// Copyright 2015 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"android/soong/android"
	"android/soong/bazel"
	"android/soong/bp2build"
	"android/soong/shared"
	"android/soong/ui/metrics/bp2build_metrics_proto"

	"github.com/google/blueprint/bootstrap"
	"github.com/google/blueprint/deptools"
	"github.com/google/blueprint/metrics"
	androidProtobuf "google.golang.org/protobuf/android"
)

var (
	topDir           string
	availableEnvFile string
	usedEnvFile      string

	globFile    string
	globListDir string
	delveListen string
	delvePath   string

	cmdlineArgs android.CmdArgs
)

func init() {
	// Flags that make sense in every mode
	flag.StringVar(&topDir, "top", "", "Top directory of the Android source tree")
	flag.StringVar(&cmdlineArgs.SoongOutDir, "soong_out", "", "Soong output directory (usually $TOP/out/soong)")
	flag.StringVar(&availableEnvFile, "available_env", "", "File containing available environment variables")
	flag.StringVar(&usedEnvFile, "used_env", "", "File containing used environment variables")
	flag.StringVar(&globFile, "globFile", "build-globs.ninja", "the Ninja file of globs to output")
	flag.StringVar(&globListDir, "globListDir", "", "the directory containing the glob list files")
	flag.StringVar(&cmdlineArgs.OutDir, "out", "", "the ninja builddir directory")
	flag.StringVar(&cmdlineArgs.ModuleListFile, "l", "", "file that lists filepaths to parse")

	// Debug flags
	flag.StringVar(&delveListen, "delve_listen", "", "Delve port to listen on for debugging")
	flag.StringVar(&delvePath, "delve_path", "", "Path to Delve. Only used if --delve_listen is set")
	flag.StringVar(&cmdlineArgs.Cpuprofile, "cpuprofile", "", "write cpu profile to file")
	flag.StringVar(&cmdlineArgs.TraceFile, "trace", "", "write trace to file")
	flag.StringVar(&cmdlineArgs.Memprofile, "memprofile", "", "write memory profile to file")
	flag.BoolVar(&cmdlineArgs.NoGC, "nogc", false, "turn off GC for debugging")

	// Flags representing various modes soong_build can run in
	flag.StringVar(&cmdlineArgs.ModuleGraphFile, "module_graph_file", "", "JSON module graph file to output")
	flag.StringVar(&cmdlineArgs.ModuleActionsFile, "module_actions_file", "", "JSON file to output inputs/outputs of actions of modules")
	flag.StringVar(&cmdlineArgs.DocFile, "soong_docs", "", "build documentation file to output")
	flag.StringVar(&cmdlineArgs.BazelQueryViewDir, "bazel_queryview_dir", "", "path to the bazel queryview directory relative to --top")
	flag.StringVar(&cmdlineArgs.BazelApiBp2buildDir, "bazel_api_bp2build_dir", "", "path to the bazel api_bp2build directory relative to --top")
	flag.StringVar(&cmdlineArgs.Bp2buildMarker, "bp2build_marker", "", "If set, run bp2build, touch the specified marker file then exit")
	flag.StringVar(&cmdlineArgs.SymlinkForestMarker, "symlink_forest_marker", "", "If set, create the bp2build symlink forest, touch the specified marker file, then exit")
	flag.StringVar(&cmdlineArgs.OutFile, "o", "build.ninja", "the Ninja file to output")
	flag.StringVar(&cmdlineArgs.BazelForceEnabledModules, "bazel-force-enabled-modules", "", "additional modules to build with Bazel. Comma-delimited")
	flag.BoolVar(&cmdlineArgs.EmptyNinjaFile, "empty-ninja-file", false, "write out a 0-byte ninja file")
	flag.BoolVar(&cmdlineArgs.BazelMode, "bazel-mode", false, "use bazel for analysis of certain modules")
	flag.BoolVar(&cmdlineArgs.BazelModeStaging, "bazel-mode-staging", false, "use bazel for analysis of certain near-ready modules")
	flag.BoolVar(&cmdlineArgs.BazelModeDev, "bazel-mode-dev", false, "use bazel for analysis of a large number of modules (less stable)")

	// Flags that probably shouldn't be flags of soong_build, but we haven't found
	// the time to remove them yet
	flag.BoolVar(&cmdlineArgs.RunGoTests, "t", false, "build and run go tests during bootstrap")

	// Disable deterministic randomization in the protobuf package, so incremental
	// builds with unrelated Soong changes don't trigger large rebuilds (since we
	// write out text protos in command lines, and command line changes trigger
	// rebuilds).
	androidProtobuf.DisableRand()
}

func newNameResolver(config android.Config) *android.NameResolver {
	return android.NewNameResolver(config)
}

func newContext(configuration android.Config) *android.Context {
	ctx := android.NewContext(configuration)
	ctx.SetNameInterface(newNameResolver(configuration))
	ctx.SetAllowMissingDependencies(configuration.AllowMissingDependencies())
	ctx.AddIncludeTags(configuration.IncludeTags()...)
	return ctx
}

// Bazel-enabled mode. Attaches a mutator to queue Bazel requests, adds a
// BeforePrepareBuildActionsHook to invoke Bazel, and then uses Bazel metadata
// for modules that should be handled by Bazel.
func runMixedModeBuild(ctx *android.Context, extraNinjaDeps []string) string {
	ctx.EventHandler.Begin("mixed_build")
	defer ctx.EventHandler.End("mixed_build")

	bazelHook := func() error {
		return ctx.Config().BazelContext.InvokeBazel(ctx.Config(), ctx)
	}
	ctx.SetBeforePrepareBuildActionsHook(bazelHook)
	ninjaDeps := bootstrap.RunBlueprint(cmdlineArgs.Args, bootstrap.DoEverything, ctx.Context, ctx.Config())
	ninjaDeps = append(ninjaDeps, extraNinjaDeps...)

	bazelPaths, err := readFileLines(ctx.Config().Getenv("BAZEL_DEPS_FILE"))
	if err != nil {
		panic("Bazel deps file not found: " + err.Error())
	}
	ninjaDeps = append(ninjaDeps, bazelPaths...)
	ninjaDeps = append(ninjaDeps, writeBuildGlobsNinjaFile(ctx)...)

	writeDepFile(cmdlineArgs.OutFile, ctx.EventHandler, ninjaDeps)
	return cmdlineArgs.OutFile
}

// Run the code-generation phase to convert BazelTargetModules to BUILD files.
func runQueryView(queryviewDir, queryviewMarker string, ctx *android.Context) {
	ctx.EventHandler.Begin("queryview")
	defer ctx.EventHandler.End("queryview")
	codegenContext := bp2build.NewCodegenContext(ctx.Config(), ctx, bp2build.QueryView)
	err := createBazelWorkspace(codegenContext, shared.JoinPath(topDir, queryviewDir))
	maybeQuit(err, "")
	touch(shared.JoinPath(topDir, queryviewMarker))
}

// Run the code-generation phase to convert API contributions to BUILD files.
// Return marker file for the new synthetic workspace
func runApiBp2build(ctx *android.Context, extraNinjaDeps []string) string {
	ctx.EventHandler.Begin("api_bp2build")
	defer ctx.EventHandler.End("api_bp2build")
	// Do not allow missing dependencies.
	ctx.SetAllowMissingDependencies(false)
	ctx.RegisterForApiBazelConversion()

	// Register the Android.bp files in the tree
	// Add them to the workspace's .d file
	ctx.SetModuleListFile(cmdlineArgs.ModuleListFile)
	if paths, err := ctx.ListModulePaths("."); err == nil {
		extraNinjaDeps = append(extraNinjaDeps, paths...)
	} else {
		panic(err)
	}

	// Run the loading and analysis phase
	ninjaDeps := bootstrap.RunBlueprint(cmdlineArgs.Args,
		bootstrap.StopBeforePrepareBuildActions,
		ctx.Context,
		ctx.Config())
	ninjaDeps = append(ninjaDeps, extraNinjaDeps...)

	// Add the globbed dependencies
	ninjaDeps = append(ninjaDeps, writeBuildGlobsNinjaFile(ctx)...)

	// Run codegen to generate BUILD files
	codegenContext := bp2build.NewCodegenContext(ctx.Config(), ctx, bp2build.ApiBp2build)
	absoluteApiBp2buildDir := shared.JoinPath(topDir, cmdlineArgs.BazelApiBp2buildDir)
	err := createBazelWorkspace(codegenContext, absoluteApiBp2buildDir)
	maybeQuit(err, "")
	ninjaDeps = append(ninjaDeps, codegenContext.AdditionalNinjaDeps()...)

	// Create soong_injection repository
	soongInjectionFiles := bp2build.CreateSoongInjectionFiles(ctx.Config(), bp2build.CreateCodegenMetrics())
	absoluteSoongInjectionDir := shared.JoinPath(topDir, ctx.Config().SoongOutDir(), bazel.SoongInjectionDirName)
	for _, file := range soongInjectionFiles {
		// The API targets in api_bp2build workspace do not have any dependency on api_bp2build.
		// But we need to create these files to prevent errors during Bazel analysis.
		// These need to be created in Read-Write mode.
		// This is because the subsequent step (bp2build in api domain analysis) creates them in Read-Write mode
		// to allow users to edit/experiment in the synthetic workspace.
		writeReadWriteFile(absoluteSoongInjectionDir, file)
	}

	workspace := shared.JoinPath(ctx.Config().SoongOutDir(), "api_bp2build")
	// Create the symlink forest
	symlinkDeps := bp2build.PlantSymlinkForest(
		ctx.Config().IsEnvTrue("BP2BUILD_VERBOSE"),
		topDir,
		workspace,
		cmdlineArgs.BazelApiBp2buildDir,
		apiBuildFileExcludes(ctx))
	ninjaDeps = append(ninjaDeps, symlinkDeps...)

	workspaceMarkerFile := workspace + ".marker"
	writeDepFile(workspaceMarkerFile, ctx.EventHandler, ninjaDeps)
	touch(shared.JoinPath(topDir, workspaceMarkerFile))
	return workspaceMarkerFile
}

// With some exceptions, api_bp2build does not have any dependencies on the checked-in BUILD files
// Exclude them from the generated workspace to prevent unrelated errors during the loading phase
func apiBuildFileExcludes(ctx *android.Context) []string {
	ret := bazelArtifacts()
	srcs, err := getExistingBazelRelatedFiles(topDir)
	maybeQuit(err, "Error determining existing Bazel-related files")
	for _, src := range srcs {
		// Exclude all src BUILD files
		if src != "WORKSPACE" &&
			src != "BUILD" &&
			src != "BUILD.bazel" &&
			!strings.HasPrefix(src, "build/bazel") &&
			!strings.HasPrefix(src, "external/bazel-skylib") &&
			!strings.HasPrefix(src, "prebuilts/clang") {
			ret = append(ret, src)
		}
	}
	// Android.bp files for api surfaces are mounted to out/, but out/ should not be a
	// dep for api_bp2build. Otherwise, api_bp2build will be run every single time
	ret = append(ret, ctx.Config().OutDir())
	return ret
}

func writeMetrics(configuration android.Config, eventHandler *metrics.EventHandler, metricsDir string) {
	if len(metricsDir) < 1 {
		fmt.Fprintf(os.Stderr, "\nMissing required env var for generating soong metrics: LOG_DIR\n")
		os.Exit(1)
	}
	metricsFile := filepath.Join(metricsDir, "soong_build_metrics.pb")
	err := android.WriteMetrics(configuration, eventHandler, metricsFile)
	maybeQuit(err, "error writing soong_build metrics %s", metricsFile)
}

func writeJsonModuleGraphAndActions(ctx *android.Context, cmdArgs android.CmdArgs) {
	graphFile, graphErr := os.Create(shared.JoinPath(topDir, cmdArgs.ModuleGraphFile))
	maybeQuit(graphErr, "graph err")
	defer graphFile.Close()
	actionsFile, actionsErr := os.Create(shared.JoinPath(topDir, cmdArgs.ModuleActionsFile))
	maybeQuit(actionsErr, "actions err")
	defer actionsFile.Close()
	ctx.Context.PrintJSONGraphAndActions(graphFile, actionsFile)
}

func writeBuildGlobsNinjaFile(ctx *android.Context) []string {
	ctx.EventHandler.Begin("globs_ninja_file")
	defer ctx.EventHandler.End("globs_ninja_file")

	globDir := bootstrap.GlobDirectory(ctx.Config().SoongOutDir(), globListDir)
	bootstrap.WriteBuildGlobsNinjaFile(&bootstrap.GlobSingleton{
		GlobLister: ctx.Globs,
		GlobFile:   globFile,
		GlobDir:    globDir,
		SrcDir:     ctx.SrcDir(),
	}, ctx.Config())
	return bootstrap.GlobFileListFiles(globDir)
}

func writeDepFile(outputFile string, eventHandler *metrics.EventHandler, ninjaDeps []string) {
	eventHandler.Begin("ninja_deps")
	defer eventHandler.End("ninja_deps")
	depFile := shared.JoinPath(topDir, outputFile+".d")
	err := deptools.WriteDepFile(depFile, outputFile, ninjaDeps)
	maybeQuit(err, "error writing depfile '%s'", depFile)
}

// runSoongOnlyBuild runs the standard Soong build in a number of different modes.
func runSoongOnlyBuild(ctx *android.Context, extraNinjaDeps []string) string {
	ctx.EventHandler.Begin("soong_build")
	defer ctx.EventHandler.End("soong_build")

	var stopBefore bootstrap.StopBefore
	switch ctx.Config().BuildMode {
	case android.GenerateModuleGraph:
		stopBefore = bootstrap.StopBeforeWriteNinja
	case android.GenerateQueryView | android.GenerateDocFile:
		stopBefore = bootstrap.StopBeforePrepareBuildActions
	default:
		stopBefore = bootstrap.DoEverything
	}

	ninjaDeps := bootstrap.RunBlueprint(cmdlineArgs.Args, stopBefore, ctx.Context, ctx.Config())
	ninjaDeps = append(ninjaDeps, extraNinjaDeps...)

	globListFiles := writeBuildGlobsNinjaFile(ctx)
	ninjaDeps = append(ninjaDeps, globListFiles...)

	// Convert the Soong module graph into Bazel BUILD files.
	switch ctx.Config().BuildMode {
	case android.GenerateQueryView:
		queryviewMarkerFile := cmdlineArgs.BazelQueryViewDir + ".marker"
		runQueryView(cmdlineArgs.BazelQueryViewDir, queryviewMarkerFile, ctx)
		writeDepFile(queryviewMarkerFile, ctx.EventHandler, ninjaDeps)
		return queryviewMarkerFile
	case android.GenerateModuleGraph:
		writeJsonModuleGraphAndActions(ctx, cmdlineArgs)
		writeDepFile(cmdlineArgs.ModuleGraphFile, ctx.EventHandler, ninjaDeps)
		return cmdlineArgs.ModuleGraphFile
	case android.GenerateDocFile:
		// TODO: we could make writeDocs() return the list of documentation files
		// written and add them to the .d file. Then soong_docs would be re-run
		// whenever one is deleted.
		err := writeDocs(ctx, shared.JoinPath(topDir, cmdlineArgs.DocFile))
		maybeQuit(err, "error building Soong documentation")
		writeDepFile(cmdlineArgs.DocFile, ctx.EventHandler, ninjaDeps)
		return cmdlineArgs.DocFile
	default:
		// The actual output (build.ninja) was written in the RunBlueprint() call
		// above
		writeDepFile(cmdlineArgs.OutFile, ctx.EventHandler, ninjaDeps)
		return cmdlineArgs.OutFile
	}
}

// soong_ui dumps the available environment variables to
// soong.environment.available . Then soong_build itself is run with an empty
// environment so that the only way environment variables can be accessed is
// using Config, which tracks access to them.

// At the end of the build, a file called soong.environment.used is written
// containing the current value of all used environment variables. The next
// time soong_ui is run, it checks whether any environment variables that was
// used had changed and if so, it deletes soong.environment.used to cause a
// rebuild.
//
// The dependency of build.ninja on soong.environment.used is declared in
// build.ninja.d
func parseAvailableEnv() map[string]string {
	if availableEnvFile == "" {
		fmt.Fprintf(os.Stderr, "--available_env not set\n")
		os.Exit(1)
	}
	result, err := shared.EnvFromFile(shared.JoinPath(topDir, availableEnvFile))
	maybeQuit(err, "error reading available environment file '%s'", availableEnvFile)
	return result
}

func main() {
	flag.Parse()

	shared.ReexecWithDelveMaybe(delveListen, delvePath)
	android.InitSandbox(topDir)

	availableEnv := parseAvailableEnv()
	configuration, err := android.NewConfig(cmdlineArgs, availableEnv)
	maybeQuit(err, "")
	if configuration.Getenv("ALLOW_MISSING_DEPENDENCIES") == "true" {
		configuration.SetAllowMissingDependencies()
	}

	extraNinjaDeps := []string{configuration.ProductVariablesFileName, usedEnvFile}
	if shared.IsDebugging() {
		// Add a non-existent file to the dependencies so that soong_build will rerun when the debugger is
		// enabled even if it completed successfully.
		extraNinjaDeps = append(extraNinjaDeps, filepath.Join(configuration.SoongOutDir(), "always_rerun_for_delve"))
	}

	// Bypass configuration.Getenv, as LOG_DIR does not need to be dependency tracked. By definition, it will
	// change between every CI build, so tracking it would require re-running Soong for every build.
	metricsDir := availableEnv["LOG_DIR"]

	ctx := newContext(configuration)

	var finalOutputFile string

	// Run Soong for a specific activity, like bp2build, queryview
	// or the actual Soong build for the build.ninja file.
	switch configuration.BuildMode {
	case android.SymlinkForest:
		finalOutputFile = runSymlinkForestCreation(ctx, extraNinjaDeps, metricsDir)
	case android.Bp2build:
		// Run the alternate pipeline of bp2build mutators and singleton to convert
		// Blueprint to BUILD files before everything else.
		finalOutputFile = runBp2Build(ctx, extraNinjaDeps, metricsDir)
	case android.ApiBp2build:
		finalOutputFile = runApiBp2build(ctx, extraNinjaDeps)
		writeMetrics(configuration, ctx.EventHandler, metricsDir)
	default:
		ctx.Register()
		if configuration.IsMixedBuildsEnabled() {
			finalOutputFile = runMixedModeBuild(ctx, extraNinjaDeps)
		} else {
			finalOutputFile = runSoongOnlyBuild(ctx, extraNinjaDeps)
		}
		writeMetrics(configuration, ctx.EventHandler, metricsDir)
	}
	writeUsedEnvironmentFile(configuration, finalOutputFile)
}

func writeUsedEnvironmentFile(configuration android.Config, finalOutputFile string) {
	if usedEnvFile == "" {
		return
	}

	path := shared.JoinPath(topDir, usedEnvFile)
	data, err := shared.EnvFileContents(configuration.EnvDeps())
	maybeQuit(err, "error writing used environment file '%s'\n", usedEnvFile)

	if preexistingData, err := os.ReadFile(path); err != nil {
		if !os.IsNotExist(err) {
			maybeQuit(err, "error reading used environment file '%s'", usedEnvFile)
		}
	} else if bytes.Equal(preexistingData, data) {
		// used environment file is unchanged
		return
	}
	err = os.WriteFile(path, data, 0666)
	maybeQuit(err, "error writing used environment file '%s'", usedEnvFile)

	// Touch the output file so that it's not older than the file we just
	// wrote. We can't write the environment file earlier because one an access
	// new environment variables while writing it.
	touch(shared.JoinPath(topDir, finalOutputFile))
}

func touch(path string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	maybeQuit(err, "Error touching '%s'", path)
	err = f.Close()
	maybeQuit(err, "Error touching '%s'", path)

	currentTime := time.Now().Local()
	err = os.Chtimes(path, currentTime, currentTime)
	maybeQuit(err, "error touching '%s'", path)
}

// Read the bazel.list file that the Soong Finder already dumped earlier (hopefully)
// It contains the locations of BUILD files, BUILD.bazel files, etc. in the source dir
func getExistingBazelRelatedFiles(topDir string) ([]string, error) {
	bazelFinderFile := filepath.Join(filepath.Dir(cmdlineArgs.ModuleListFile), "bazel.list")
	if !filepath.IsAbs(bazelFinderFile) {
		// Assume this was a relative path under topDir
		bazelFinderFile = filepath.Join(topDir, bazelFinderFile)
	}
	return readFileLines(bazelFinderFile)
}

func bazelArtifacts() []string {
	return []string{
		"bazel-bin",
		"bazel-genfiles",
		"bazel-out",
		"bazel-testlogs",
		"bazel-" + filepath.Base(topDir),
	}
}

// This could in theory easily be separated into a binary that generically
// merges two directories into a symlink tree. The main obstacle is that this
// function currently depends on both Bazel-specific knowledge (the existence
// of bazel-* symlinks) and configuration (the set of BUILD.bazel files that
// should and should not be kept)
//
// Ideally, bp2build would write a file that contains instructions to the
// symlink tree creation binary. Then the latter would not need to depend on
// the very heavy-weight machinery of soong_build .
func runSymlinkForestCreation(ctx *android.Context, extraNinjaDeps []string, metricsDir string) string {
	ctx.EventHandler.Do("symlink_forest", func() {
		var ninjaDeps []string
		ninjaDeps = append(ninjaDeps, extraNinjaDeps...)
		verbose := ctx.Config().IsEnvTrue("BP2BUILD_VERBOSE")

		// PlantSymlinkForest() returns all the directories that were readdir()'ed.
		// Such a directory SHOULD be added to `ninjaDeps` so that a child directory
		// or file created/deleted under it would trigger an update of the symlink forest.
		generatedRoot := shared.JoinPath(ctx.Config().SoongOutDir(), "bp2build")
		workspaceRoot := shared.JoinPath(ctx.Config().SoongOutDir(), "workspace")
		ctx.EventHandler.Do("plant", func() {
			symlinkForestDeps := bp2build.PlantSymlinkForest(
				verbose, topDir, workspaceRoot, generatedRoot, excludedFromSymlinkForest(ctx, verbose))
			ninjaDeps = append(ninjaDeps, symlinkForestDeps...)
		})

		writeDepFile(cmdlineArgs.SymlinkForestMarker, ctx.EventHandler, ninjaDeps)
		touch(shared.JoinPath(topDir, cmdlineArgs.SymlinkForestMarker))
	})
	codegenMetrics := bp2build.ReadCodegenMetrics(metricsDir)
	if codegenMetrics == nil {
		m := bp2build.CreateCodegenMetrics()
		codegenMetrics = &m
	} else {
		//TODO (usta) we cannot determine if we loaded a stale file, i.e. from an unrelated prior
		//invocation of codegen. We should simply use a separate .pb file
	}
	writeBp2BuildMetrics(codegenMetrics, ctx.EventHandler, metricsDir)
	return cmdlineArgs.SymlinkForestMarker
}

func excludedFromSymlinkForest(ctx *android.Context, verbose bool) []string {
	excluded := bazelArtifacts()
	if cmdlineArgs.OutDir[0] != '/' {
		excluded = append(excluded, cmdlineArgs.OutDir)
	}

	// Find BUILD files in the srcDir which are not in the allowlist
	// (android.Bp2BuildConversionAllowlist#ShouldKeepExistingBuildFileForDir)
	// and return their paths so they can be left out of the Bazel workspace dir (i.e. ignored)
	existingBazelFiles, err := getExistingBazelRelatedFiles(topDir)
	maybeQuit(err, "Error determining existing Bazel-related files")

	for _, path := range existingBazelFiles {
		fullPath := shared.JoinPath(topDir, path)
		fileInfo, err2 := os.Stat(fullPath)
		if err2 != nil {
			// Warn about error, but continue trying to check files
			fmt.Fprintf(os.Stderr, "WARNING: Error accessing path '%s', err: %s\n", fullPath, err2)
			continue
		}
		// Exclude only files named 'BUILD' or 'BUILD.bazel' and unless forcibly kept
		if fileInfo.IsDir() ||
			(fileInfo.Name() != "BUILD" && fileInfo.Name() != "BUILD.bazel") ||
			ctx.Config().Bp2buildPackageConfig.ShouldKeepExistingBuildFileForDir(filepath.Dir(path)) {
			// Don't ignore this existing build file
			continue
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "Ignoring existing BUILD file: %s\n", path)
		}
		excluded = append(excluded, path)
	}

	// Temporarily exclude stuff to make `bazel build //external/...` (and `bazel build //frameworks/...`)  work
	excluded = append(excluded,
		// FIXME: 'autotest_lib' is a symlink back to external/autotest, and this causes an infinite
		// symlink expansion error for Bazel
		"external/autotest/venv/autotest_lib",
		"external/autotest/autotest_lib",
		"external/autotest/client/autotest_lib/client",

		// FIXME: The external/google-fruit/extras/bazel_root/third_party/fruit dir is poison
		// It contains several symlinks back to real source dirs, and those source dirs contain
		// BUILD files we want to ignore
		"external/google-fruit/extras/bazel_root/third_party/fruit",

		// FIXME: 'frameworks/compile/slang' has a filegroup error due to an escaping issue
		"frameworks/compile/slang",

		// FIXME(b/260809113): 'prebuilts/clang/host/linux-x86/clang-dev' is a tool-generated symlink
		// directory that contains a BUILD file. The bazel files finder code doesn't traverse into symlink dirs,
		// and hence is not aware of this BUILD file and exclude it accordingly during symlink forest generation
		// when checking against keepExistingBuildFiles allowlist.
		//
		// This is necessary because globs in //prebuilts/clang/host/linux-x86/BUILD
		// currently assume no subpackages (keepExistingBuildFile is not recursive for that directory).
		//
		// This is a bandaid until we the symlink forest logic can intelligently exclude BUILD files found in
		// source symlink dirs according to the keepExistingBuildFile allowlist.
		"prebuilts/clang/host/linux-x86/clang-dev",
	)
	return excluded
}

// Run Soong in the bp2build mode. This creates a standalone context that registers
// an alternate pipeline of mutators and singletons specifically for generating
// Bazel BUILD files instead of Ninja files.
func runBp2Build(ctx *android.Context, extraNinjaDeps []string, metricsDir string) string {
	var codegenMetrics *bp2build.CodegenMetrics
	ctx.EventHandler.Do("bp2build", func() {

		// Propagate "allow misssing dependencies" bit. This is normally set in
		// newContext(), but we create ctx without calling that method.
		ctx.SetAllowMissingDependencies(ctx.Config().AllowMissingDependencies())
		ctx.SetNameInterface(newNameResolver(ctx.Config()))
		ctx.RegisterForBazelConversion()
		ctx.SetModuleListFile(cmdlineArgs.ModuleListFile)

		var ninjaDeps []string
		ninjaDeps = append(ninjaDeps, extraNinjaDeps...)

		// Run the loading and analysis pipeline to prepare the graph of regular
		// Modules parsed from Android.bp files, and the BazelTargetModules mapped
		// from the regular Modules.
		ctx.EventHandler.Do("bootstrap", func() {
			blueprintArgs := cmdlineArgs
			bootstrapDeps := bootstrap.RunBlueprint(blueprintArgs.Args,
				bootstrap.StopBeforePrepareBuildActions, ctx.Context, ctx.Config())
			ninjaDeps = append(ninjaDeps, bootstrapDeps...)
		})

		globListFiles := writeBuildGlobsNinjaFile(ctx)
		ninjaDeps = append(ninjaDeps, globListFiles...)

		// Run the code-generation phase to convert BazelTargetModules to BUILD files
		// and print conversion codegenMetrics to the user.
		codegenContext := bp2build.NewCodegenContext(ctx.Config(), ctx, bp2build.Bp2Build)
		ctx.EventHandler.Do("codegen", func() {
			codegenMetrics = bp2build.Codegen(codegenContext)
		})

		ninjaDeps = append(ninjaDeps, codegenContext.AdditionalNinjaDeps()...)

		writeDepFile(cmdlineArgs.Bp2buildMarker, ctx.EventHandler, ninjaDeps)
		touch(shared.JoinPath(topDir, cmdlineArgs.Bp2buildMarker))
	})

	// Only report metrics when in bp2build mode. The metrics aren't relevant
	// for queryview, since that's a total repo-wide conversion and there's a
	// 1:1 mapping for each module.
	if ctx.Config().IsEnvTrue("BP2BUILD_VERBOSE") {
		codegenMetrics.Print()
	}
	writeBp2BuildMetrics(codegenMetrics, ctx.EventHandler, metricsDir)
	return cmdlineArgs.Bp2buildMarker
}

// Write Bp2Build metrics into $LOG_DIR
func writeBp2BuildMetrics(codegenMetrics *bp2build.CodegenMetrics, eventHandler *metrics.EventHandler, metricsDir string) {
	for _, event := range eventHandler.CompletedEvents() {
		codegenMetrics.AddEvent(&bp2build_metrics_proto.Event{
			Name:      event.Id,
			StartTime: uint64(event.Start.UnixNano()),
			RealTime:  event.RuntimeNanoseconds(),
		})
	}
	if len(metricsDir) < 1 {
		fmt.Fprintf(os.Stderr, "\nMissing required env var for generating bp2build metrics: LOG_DIR\n")
		os.Exit(1)
	}
	codegenMetrics.Write(metricsDir)
}

func readFileLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return strings.Split(strings.TrimSpace(string(data)), "\n"), nil
	}
	return nil, err

}
func maybeQuit(err error, format string, args ...interface{}) {
	if err == nil {
		return
	}
	if format != "" {
		fmt.Fprintln(os.Stderr, fmt.Sprintf(format, args...)+": "+err.Error())
	} else {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(1)
}
