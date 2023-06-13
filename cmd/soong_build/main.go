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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"android/soong/android"
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
	outDir           string
	soongOutDir      string
	availableEnvFile string
	usedEnvFile      string

	runGoTests bool

	globFile    string
	globListDir string
	delveListen string
	delvePath   string

	moduleGraphFile   string
	moduleActionsFile string
	docFile           string
	bazelQueryViewDir string
	bp2buildMarker    string

	cmdlineArgs bootstrap.Args
)

func init() {
	// Flags that make sense in every mode
	flag.StringVar(&topDir, "top", "", "Top directory of the Android source tree")
	flag.StringVar(&soongOutDir, "soong_out", "", "Soong output directory (usually $TOP/out/soong)")
	flag.StringVar(&availableEnvFile, "available_env", "", "File containing available environment variables")
	flag.StringVar(&usedEnvFile, "used_env", "", "File containing used environment variables")
	flag.StringVar(&globFile, "globFile", "build-globs.ninja", "the Ninja file of globs to output")
	flag.StringVar(&globListDir, "globListDir", "", "the directory containing the glob list files")
	flag.StringVar(&outDir, "out", "", "the ninja builddir directory")
	flag.StringVar(&cmdlineArgs.ModuleListFile, "l", "", "file that lists filepaths to parse")

	// Debug flags
	flag.StringVar(&delveListen, "delve_listen", "", "Delve port to listen on for debugging")
	flag.StringVar(&delvePath, "delve_path", "", "Path to Delve. Only used if --delve_listen is set")
	flag.StringVar(&cmdlineArgs.Cpuprofile, "cpuprofile", "", "write cpu profile to file")
	flag.StringVar(&cmdlineArgs.TraceFile, "trace", "", "write trace to file")
	flag.StringVar(&cmdlineArgs.Memprofile, "memprofile", "", "write memory profile to file")
	flag.BoolVar(&cmdlineArgs.NoGC, "nogc", false, "turn off GC for debugging")

	// Flags representing various modes soong_build can run in
	flag.StringVar(&moduleGraphFile, "module_graph_file", "", "JSON module graph file to output")
	flag.StringVar(&moduleActionsFile, "module_actions_file", "", "JSON file to output inputs/outputs of actions of modules")
	flag.StringVar(&docFile, "soong_docs", "", "build documentation file to output")
	flag.StringVar(&bazelQueryViewDir, "bazel_queryview_dir", "", "path to the bazel queryview directory relative to --top")
	flag.StringVar(&bp2buildMarker, "bp2build_marker", "", "If set, run bp2build, touch the specified marker file then exit")
	flag.StringVar(&cmdlineArgs.OutFile, "o", "build.ninja", "the Ninja file to output")
	flag.BoolVar(&cmdlineArgs.EmptyNinjaFile, "empty-ninja-file", false, "write out a 0-byte ninja file")

	// Flags that probably shouldn't be flags of soong_build but we haven't found
	// the time to remove them yet
	flag.BoolVar(&runGoTests, "t", false, "build and run go tests during bootstrap")

	// Disable deterministic randomization in the protobuf package, so incremental
	// builds with unrelated Soong changes don't trigger large rebuilds (since we
	// write out text protos in command lines, and command line changes trigger
	// rebuilds).
	androidProtobuf.DisableRand()
}

func newNameResolver(config android.Config) *android.NameResolver {
	namespacePathsToExport := make(map[string]bool)

	for _, namespaceName := range config.ExportedNamespaces() {
		namespacePathsToExport[namespaceName] = true
	}

	namespacePathsToExport["."] = true // always export the root namespace

	exportFilter := func(namespace *android.Namespace) bool {
		return namespacePathsToExport[namespace.Path]
	}

	return android.NewNameResolver(exportFilter)
}

func newContext(configuration android.Config) *android.Context {
	ctx := android.NewContext(configuration)
	ctx.Register()
	ctx.SetNameInterface(newNameResolver(configuration))
	ctx.SetAllowMissingDependencies(configuration.AllowMissingDependencies())
	ctx.AddIncludeTags(configuration.IncludeTags()...)
	return ctx
}

func newConfig(availableEnv map[string]string) android.Config {
	configuration, err := android.NewConfig(cmdlineArgs.ModuleListFile, runGoTests, outDir, soongOutDir, availableEnv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(1)
	}
	return configuration
}

// Bazel-enabled mode. Soong runs in two passes.
// First pass: Analyze the build tree, but only store all bazel commands
// needed to correctly evaluate the tree in the second pass.
// TODO(cparsons): Don't output any ninja file, as the second pass will overwrite
// the incorrect results from the first pass, and file I/O is expensive.
func runMixedModeBuild(configuration android.Config, firstCtx *android.Context, extraNinjaDeps []string) {
	firstCtx.EventHandler.Begin("mixed_build")
	defer firstCtx.EventHandler.End("mixed_build")

	firstCtx.EventHandler.Begin("prepare")
	bootstrap.RunBlueprint(cmdlineArgs, bootstrap.StopBeforeWriteNinja, firstCtx.Context, configuration)
	firstCtx.EventHandler.End("prepare")

	firstCtx.EventHandler.Begin("bazel")
	// Invoke bazel commands and save results for second pass.
	if err := configuration.BazelContext.InvokeBazel(); err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(1)
	}
	// Second pass: Full analysis, using the bazel command results. Output ninja file.
	secondConfig, err := android.ConfigForAdditionalRun(configuration)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(1)
	}
	firstCtx.EventHandler.End("bazel")

	secondCtx := newContext(secondConfig)
	secondCtx.EventHandler = firstCtx.EventHandler
	secondCtx.EventHandler.Begin("analyze")
	ninjaDeps := bootstrap.RunBlueprint(cmdlineArgs, bootstrap.DoEverything, secondCtx.Context, secondConfig)
	ninjaDeps = append(ninjaDeps, extraNinjaDeps...)
	secondCtx.EventHandler.End("analyze")

	globListFiles := writeBuildGlobsNinjaFile(secondCtx, configuration.SoongOutDir(), configuration)
	ninjaDeps = append(ninjaDeps, globListFiles...)

	writeDepFile(cmdlineArgs.OutFile, *secondCtx.EventHandler, ninjaDeps)
}

// Run the code-generation phase to convert BazelTargetModules to BUILD files.
func runQueryView(queryviewDir, queryviewMarker string, configuration android.Config, ctx *android.Context) {
	ctx.EventHandler.Begin("queryview")
	defer ctx.EventHandler.End("queryview")
	codegenContext := bp2build.NewCodegenContext(configuration, *ctx, bp2build.QueryView)
	absoluteQueryViewDir := shared.JoinPath(topDir, queryviewDir)
	if err := createBazelQueryView(codegenContext, absoluteQueryViewDir); err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(1)
	}

	touch(shared.JoinPath(topDir, queryviewMarker))
}

func writeMetrics(configuration android.Config, eventHandler metrics.EventHandler) {
	metricsDir := configuration.Getenv("LOG_DIR")
	if len(metricsDir) < 1 {
		fmt.Fprintf(os.Stderr, "\nMissing required env var for generating soong metrics: LOG_DIR\n")
		os.Exit(1)
	}
	metricsFile := filepath.Join(metricsDir, "soong_build_metrics.pb")
	err := android.WriteMetrics(configuration, eventHandler, metricsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing soong_build metrics %s: %s", metricsFile, err)
		os.Exit(1)
	}
}

func writeJsonModuleGraphAndActions(ctx *android.Context, graphPath string, actionsPath string) {
	graphFile, graphErr := os.Create(shared.JoinPath(topDir, graphPath))
	actionsFile, actionsErr := os.Create(shared.JoinPath(topDir, actionsPath))
	if graphErr != nil || actionsErr != nil {
		fmt.Fprintf(os.Stderr, "Graph err: %s, actions err: %s", graphErr, actionsErr)
		os.Exit(1)
	}

	defer graphFile.Close()
	defer actionsFile.Close()
	ctx.Context.PrintJSONGraphAndActions(graphFile, actionsFile)
}

func writeBuildGlobsNinjaFile(ctx *android.Context, buildDir string, config interface{}) []string {
	ctx.EventHandler.Begin("globs_ninja_file")
	defer ctx.EventHandler.End("globs_ninja_file")

	globDir := bootstrap.GlobDirectory(buildDir, globListDir)
	bootstrap.WriteBuildGlobsNinjaFile(&bootstrap.GlobSingleton{
		GlobLister: ctx.Globs,
		GlobFile:   globFile,
		GlobDir:    globDir,
		SrcDir:     ctx.SrcDir(),
	}, config)
	return bootstrap.GlobFileListFiles(globDir)
}

func writeDepFile(outputFile string, eventHandler metrics.EventHandler, ninjaDeps []string) {
	eventHandler.Begin("ninja_deps")
	defer eventHandler.End("ninja_deps")
	depFile := shared.JoinPath(topDir, outputFile+".d")
	err := deptools.WriteDepFile(depFile, outputFile, ninjaDeps)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing depfile '%s': %s\n", depFile, err)
		os.Exit(1)
	}
}

// doChosenActivity runs Soong for a specific activity, like bp2build, queryview
// or the actual Soong build for the build.ninja file. Returns the top level
// output file of the specific activity.
func doChosenActivity(configuration android.Config, extraNinjaDeps []string) string {
	mixedModeBuild := configuration.BazelContext.BazelEnabled()
	generateBazelWorkspace := bp2buildMarker != ""
	generateQueryView := bazelQueryViewDir != ""
	generateModuleGraphFile := moduleGraphFile != ""
	generateDocFile := docFile != ""

	if generateBazelWorkspace {
		// Run the alternate pipeline of bp2build mutators and singleton to convert
		// Blueprint to BUILD files before everything else.
		runBp2Build(configuration, extraNinjaDeps)
		return bp2buildMarker
	}

	blueprintArgs := cmdlineArgs

	ctx := newContext(configuration)
	if mixedModeBuild {
		runMixedModeBuild(configuration, ctx, extraNinjaDeps)
	} else {
		var stopBefore bootstrap.StopBefore
		if generateModuleGraphFile {
			stopBefore = bootstrap.StopBeforeWriteNinja
		} else if generateQueryView {
			stopBefore = bootstrap.StopBeforePrepareBuildActions
		} else if generateDocFile {
			stopBefore = bootstrap.StopBeforePrepareBuildActions
		} else {
			stopBefore = bootstrap.DoEverything
		}

		ninjaDeps := bootstrap.RunBlueprint(blueprintArgs, stopBefore, ctx.Context, configuration)
		ninjaDeps = append(ninjaDeps, extraNinjaDeps...)

		globListFiles := writeBuildGlobsNinjaFile(ctx, configuration.SoongOutDir(), configuration)
		ninjaDeps = append(ninjaDeps, globListFiles...)

		// Convert the Soong module graph into Bazel BUILD files.
		if generateQueryView {
			queryviewMarkerFile := bazelQueryViewDir + ".marker"
			runQueryView(bazelQueryViewDir, queryviewMarkerFile, configuration, ctx)
			writeDepFile(queryviewMarkerFile, *ctx.EventHandler, ninjaDeps)
			return queryviewMarkerFile
		} else if generateModuleGraphFile {
			writeJsonModuleGraphAndActions(ctx, moduleGraphFile, moduleActionsFile)
			writeDepFile(moduleGraphFile, *ctx.EventHandler, ninjaDeps)
			return moduleGraphFile
		} else if generateDocFile {
			// TODO: we could make writeDocs() return the list of documentation files
			// written and add them to the .d file. Then soong_docs would be re-run
			// whenever one is deleted.
			if err := writeDocs(ctx, shared.JoinPath(topDir, docFile)); err != nil {
				fmt.Fprintf(os.Stderr, "error building Soong documentation: %s\n", err)
				os.Exit(1)
			}
			writeDepFile(docFile, *ctx.EventHandler, ninjaDeps)
			return docFile
		} else {
			// The actual output (build.ninja) was written in the RunBlueprint() call
			// above
			writeDepFile(cmdlineArgs.OutFile, *ctx.EventHandler, ninjaDeps)
		}
	}

	writeMetrics(configuration, *ctx.EventHandler)
	return cmdlineArgs.OutFile
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
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading available environment file '%s': %s\n", availableEnvFile, err)
		os.Exit(1)
	}

	return result
}

func main() {
	flag.Parse()

	shared.ReexecWithDelveMaybe(delveListen, delvePath)
	android.InitSandbox(topDir)

	availableEnv := parseAvailableEnv()

	configuration := newConfig(availableEnv)
	extraNinjaDeps := []string{
		configuration.ProductVariablesFileName,
		usedEnvFile,
	}

	if configuration.Getenv("ALLOW_MISSING_DEPENDENCIES") == "true" {
		configuration.SetAllowMissingDependencies()
	}

	if shared.IsDebugging() {
		// Add a non-existent file to the dependencies so that soong_build will rerun when the debugger is
		// enabled even if it completed successfully.
		extraNinjaDeps = append(extraNinjaDeps, filepath.Join(configuration.SoongOutDir(), "always_rerun_for_delve"))
	}

	finalOutputFile := doChosenActivity(configuration, extraNinjaDeps)

	writeUsedEnvironmentFile(configuration, finalOutputFile)
}

func writeUsedEnvironmentFile(configuration android.Config, finalOutputFile string) {
	if usedEnvFile == "" {
		return
	}

	path := shared.JoinPath(topDir, usedEnvFile)
	data, err := shared.EnvFileContents(configuration.EnvDeps())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing used environment file '%s': %s\n", usedEnvFile, err)
		os.Exit(1)
	}

	err = ioutil.WriteFile(path, data, 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing used environment file '%s': %s\n", usedEnvFile, err)
		os.Exit(1)
	}

	// Touch the output file so that it's not older than the file we just
	// wrote. We can't write the environment file earlier because one an access
	// new environment variables while writing it.
	touch(shared.JoinPath(topDir, finalOutputFile))
}

func touch(path string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error touching '%s': %s\n", path, err)
		os.Exit(1)
	}

	err = f.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error touching '%s': %s\n", path, err)
		os.Exit(1)
	}

	currentTime := time.Now().Local()
	err = os.Chtimes(path, currentTime, currentTime)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error touching '%s': %s\n", path, err)
		os.Exit(1)
	}
}

// Find BUILD files in the srcDir which...
//
// - are not on the allow list (android/bazel.go#ShouldKeepExistingBuildFileForDir())
//
// - won't be overwritten by corresponding bp2build generated files
//
// And return their paths so they can be left out of the Bazel workspace dir (i.e. ignored)
func getPathsToIgnoredBuildFiles(topDir string, generatedRoot string, srcDirBazelFiles []string, verbose bool) []string {
	paths := make([]string, 0)

	for _, srcDirBazelFileRelativePath := range srcDirBazelFiles {
		srcDirBazelFileFullPath := shared.JoinPath(topDir, srcDirBazelFileRelativePath)
		fileInfo, err := os.Stat(srcDirBazelFileFullPath)
		if err != nil {
			// Warn about error, but continue trying to check files
			fmt.Fprintf(os.Stderr, "WARNING: Error accessing path '%s', err: %s\n", srcDirBazelFileFullPath, err)
			continue
		}
		if fileInfo.IsDir() {
			// Don't ignore entire directories
			continue
		}
		if !(fileInfo.Name() == "BUILD" || fileInfo.Name() == "BUILD.bazel") {
			// Don't ignore this file - it is not a build file
			continue
		}
		srcDirBazelFileDir := filepath.Dir(srcDirBazelFileRelativePath)
		if android.ShouldKeepExistingBuildFileForDir(srcDirBazelFileDir) {
			// Don't ignore this existing build file
			continue
		}
		correspondingBp2BuildFile := shared.JoinPath(topDir, generatedRoot, srcDirBazelFileRelativePath)
		if _, err := os.Stat(correspondingBp2BuildFile); err == nil {
			// If bp2build generated an alternate BUILD file, don't exclude this workspace path
			// BUILD file clash resolution happens later in the symlink forest creation
			continue
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "Ignoring existing BUILD file: %s\n", srcDirBazelFileRelativePath)
		}
		paths = append(paths, srcDirBazelFileRelativePath)
	}

	return paths
}

// Returns temporary symlink forest excludes necessary for bazel build //external/... (and bazel build //frameworks/...) to work
func getTemporaryExcludes() []string {
	excludes := make([]string, 0)

	// FIXME: 'autotest_lib' is a symlink back to external/autotest, and this causes an infinite symlink expansion error for Bazel
	excludes = append(excludes, "external/autotest/venv/autotest_lib")

	// FIXME: The external/google-fruit/extras/bazel_root/third_party/fruit dir is poison
	// It contains several symlinks back to real source dirs, and those source dirs contain BUILD files we want to ignore
	excludes = append(excludes, "external/google-fruit/extras/bazel_root/third_party/fruit")

	// FIXME: 'frameworks/compile/slang' has a filegroup error due to an escaping issue
	excludes = append(excludes, "frameworks/compile/slang")

	return excludes
}

// Read the bazel.list file that the Soong Finder already dumped earlier (hopefully)
// It contains the locations of BUILD files, BUILD.bazel files, etc. in the source dir
func getExistingBazelRelatedFiles(topDir string) ([]string, error) {
	bazelFinderFile := filepath.Join(filepath.Dir(cmdlineArgs.ModuleListFile), "bazel.list")
	if !filepath.IsAbs(bazelFinderFile) {
		// Assume this was a relative path under topDir
		bazelFinderFile = filepath.Join(topDir, bazelFinderFile)
	}
	data, err := ioutil.ReadFile(bazelFinderFile)
	if err != nil {
		return nil, err
	}
	files := strings.Split(strings.TrimSpace(string(data)), "\n")
	return files, nil
}

// Run Soong in the bp2build mode. This creates a standalone context that registers
// an alternate pipeline of mutators and singletons specifically for generating
// Bazel BUILD files instead of Ninja files.
func runBp2Build(configuration android.Config, extraNinjaDeps []string) {
	eventHandler := metrics.EventHandler{}
	eventHandler.Begin("bp2build")

	// Register an alternate set of singletons and mutators for bazel
	// conversion for Bazel conversion.
	bp2buildCtx := android.NewContext(configuration)

	// Soong internals like LoadHooks behave differently when running as
	// bp2build. This is the bit to differentiate between Soong-as-Soong and
	// Soong-as-bp2build.
	bp2buildCtx.SetRunningAsBp2build()

	// Propagate "allow misssing dependencies" bit. This is normally set in
	// newContext(), but we create bp2buildCtx without calling that method.
	bp2buildCtx.SetAllowMissingDependencies(configuration.AllowMissingDependencies())
	bp2buildCtx.SetNameInterface(newNameResolver(configuration))
	bp2buildCtx.RegisterForBazelConversion()

	// The bp2build process is a purely functional process that only depends on
	// Android.bp files. It must not depend on the values of per-build product
	// configurations or variables, since those will generate different BUILD
	// files based on how the user has configured their tree.
	bp2buildCtx.SetModuleListFile(cmdlineArgs.ModuleListFile)
	modulePaths, err := bp2buildCtx.ListModulePaths(".")
	if err != nil {
		panic(err)
	}

	extraNinjaDeps = append(extraNinjaDeps, modulePaths...)

	// Run the loading and analysis pipeline to prepare the graph of regular
	// Modules parsed from Android.bp files, and the BazelTargetModules mapped
	// from the regular Modules.
	blueprintArgs := cmdlineArgs
	ninjaDeps := bootstrap.RunBlueprint(blueprintArgs, bootstrap.StopBeforePrepareBuildActions, bp2buildCtx.Context, configuration)
	ninjaDeps = append(ninjaDeps, extraNinjaDeps...)

	globListFiles := writeBuildGlobsNinjaFile(bp2buildCtx, configuration.SoongOutDir(), configuration)
	ninjaDeps = append(ninjaDeps, globListFiles...)

	// Run the code-generation phase to convert BazelTargetModules to BUILD files
	// and print conversion metrics to the user.
	codegenContext := bp2build.NewCodegenContext(configuration, *bp2buildCtx, bp2build.Bp2Build)
	metrics := bp2build.Codegen(codegenContext)

	generatedRoot := shared.JoinPath(configuration.SoongOutDir(), "bp2build")
	workspaceRoot := shared.JoinPath(configuration.SoongOutDir(), "workspace")

	excludes := []string{
		"bazel-bin",
		"bazel-genfiles",
		"bazel-out",
		"bazel-testlogs",
		"bazel-" + filepath.Base(topDir),
	}

	if outDir[0] != '/' {
		excludes = append(excludes, outDir)
	}

	existingBazelRelatedFiles, err := getExistingBazelRelatedFiles(topDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error determining existing Bazel-related files: %s\n", err)
		os.Exit(1)
	}

	pathsToIgnoredBuildFiles := getPathsToIgnoredBuildFiles(topDir, generatedRoot, existingBazelRelatedFiles, configuration.IsEnvTrue("BP2BUILD_VERBOSE"))
	excludes = append(excludes, pathsToIgnoredBuildFiles...)

	excludes = append(excludes, getTemporaryExcludes()...)

	symlinkForestDeps := bp2build.PlantSymlinkForest(
		topDir, workspaceRoot, generatedRoot, ".", excludes)

	ninjaDeps = append(ninjaDeps, codegenContext.AdditionalNinjaDeps()...)
	ninjaDeps = append(ninjaDeps, symlinkForestDeps...)

	writeDepFile(bp2buildMarker, eventHandler, ninjaDeps)

	// Create an empty bp2build marker file.
	touch(shared.JoinPath(topDir, bp2buildMarker))

	eventHandler.End("bp2build")

	// Only report metrics when in bp2build mode. The metrics aren't relevant
	// for queryview, since that's a total repo-wide conversion and there's a
	// 1:1 mapping for each module.
	metrics.Print()
	writeBp2BuildMetrics(&metrics, configuration, eventHandler)
}

// Write Bp2Build metrics into $LOG_DIR
func writeBp2BuildMetrics(codegenMetrics *bp2build.CodegenMetrics,
	configuration android.Config, eventHandler metrics.EventHandler) {
	for _, event := range eventHandler.CompletedEvents() {
		codegenMetrics.Events = append(codegenMetrics.Events,
			&bp2build_metrics_proto.Event{
				Name:      event.Id,
				StartTime: uint64(event.Start.UnixNano()),
				RealTime:  event.RuntimeNanoseconds(),
			})
	}
	metricsDir := configuration.Getenv("LOG_DIR")
	if len(metricsDir) < 1 {
		fmt.Fprintf(os.Stderr, "\nMissing required env var for generating bp2build metrics: LOG_DIR\n")
		os.Exit(1)
	}
	codegenMetrics.Write(metricsDir)
}
