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
	"os"
	"path/filepath"
	"strings"

	"github.com/google/blueprint/bootstrap"

	"android/soong/android"
	"android/soong/bp2build"
)

var (
	docFile           string
	bazelQueryViewDir string
)

func init() {
	flag.StringVar(&docFile, "soong_docs", "", "build documentation file to output")
	flag.StringVar(&bazelQueryViewDir, "bazel_queryview_dir", "", "path to the bazel queryview directory")
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

// bazelConversionRequested checks that the user is intending to convert
// Blueprint to Bazel BUILD files.
func bazelConversionRequested(configuration android.Config) bool {
	return configuration.IsEnvTrue("GENERATE_BAZEL_FILES")
}

func newContext(configuration android.Config) *android.Context {
	ctx := android.NewContext(configuration)
	ctx.Register()
	if !shouldPrepareBuildActions(configuration) {
		configuration.SetStopBefore(bootstrap.StopBeforePrepareBuildActions)
	}
	ctx.SetNameInterface(newNameResolver(configuration))
	ctx.SetAllowMissingDependencies(configuration.AllowMissingDependencies())
	return ctx
}

func newConfig(srcDir string) android.Config {
	configuration, err := android.NewConfig(srcDir, bootstrap.BuildDir, bootstrap.ModuleListFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(1)
	}
	return configuration
}

func main() {
	android.ReexecWithDelveMaybe()
	flag.Parse()

	// The top-level Blueprints file is passed as the first argument.
	srcDir := filepath.Dir(flag.Arg(0))
	var ctx *android.Context
	configuration := newConfig(srcDir)
	extraNinjaDeps := []string{configuration.ProductVariablesFileName}

	// Read the SOONG_DELVE again through configuration so that there is a dependency on the environment variable
	// and soong_build will rerun when it is set for the first time.
	if listen := configuration.Getenv("SOONG_DELVE"); listen != "" {
		// Add a non-existent file to the dependencies so that soong_build will rerun when the debugger is
		// enabled even if it completed successfully.
		extraNinjaDeps = append(extraNinjaDeps, filepath.Join(configuration.BuildDir(), "always_rerun_for_delve"))
	}

	if bazelConversionRequested(configuration) {
		// Run the alternate pipeline of bp2build mutators and singleton to convert Blueprint to BUILD files
		// before everything else.
		runBp2Build(srcDir, configuration)
		// Short-circuit and return.
		return
	}

	if configuration.BazelContext.BazelEnabled() {
		// Bazel-enabled mode. Soong runs in two passes.
		// First pass: Analyze the build tree, but only store all bazel commands
		// needed to correctly evaluate the tree in the second pass.
		// TODO(cparsons): Don't output any ninja file, as the second pass will overwrite
		// the incorrect results from the first pass, and file I/O is expensive.
		firstCtx := newContext(configuration)
		configuration.SetStopBefore(bootstrap.StopBeforeWriteNinja)
		bootstrap.Main(firstCtx.Context, configuration, extraNinjaDeps...)
		// Invoke bazel commands and save results for second pass.
		if err := configuration.BazelContext.InvokeBazel(); err != nil {
			fmt.Fprintf(os.Stderr, "%s", err)
			os.Exit(1)
		}
		// Second pass: Full analysis, using the bazel command results. Output ninja file.
		secondPassConfig, err := android.ConfigForAdditionalRun(configuration)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", err)
			os.Exit(1)
		}
		ctx = newContext(secondPassConfig)
		bootstrap.Main(ctx.Context, secondPassConfig, extraNinjaDeps...)
	} else {
		ctx = newContext(configuration)
		bootstrap.Main(ctx.Context, configuration, extraNinjaDeps...)
	}

	// Convert the Soong module graph into Bazel BUILD files.
	if bazelQueryViewDir != "" {
		// Run the code-generation phase to convert BazelTargetModules to BUILD files.
		codegenContext := bp2build.NewCodegenContext(configuration, *ctx, bp2build.QueryView)
		if err := createBazelQueryView(codegenContext, bazelQueryViewDir); err != nil {
			fmt.Fprintf(os.Stderr, "%s", err)
			os.Exit(1)
		}
	}

	if docFile != "" {
		if err := writeDocs(ctx, docFile); err != nil {
			fmt.Fprintf(os.Stderr, "%s", err)
			os.Exit(1)
		}
	}

	// TODO(ccross): make this a command line argument.  Requires plumbing through blueprint
	//  to affect the command line of the primary builder.
	if shouldPrepareBuildActions(configuration) {
		metricsFile := filepath.Join(bootstrap.BuildDir, "soong_build_metrics.pb")
		err := android.WriteMetrics(configuration, metricsFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error writing soong_build metrics %s: %s", metricsFile, err)
			os.Exit(1)
		}
	}
}

// Run Soong in the bp2build mode. This creates a standalone context that registers
// an alternate pipeline of mutators and singletons specifically for generating
// Bazel BUILD files instead of Ninja files.
func runBp2Build(srcDir string, configuration android.Config) {
	// Register an alternate set of singletons and mutators for bazel
	// conversion for Bazel conversion.
	bp2buildCtx := android.NewContext(configuration)
	bp2buildCtx.RegisterForBazelConversion()

	// No need to generate Ninja build rules/statements from Modules and Singletons.
	configuration.SetStopBefore(bootstrap.StopBeforePrepareBuildActions)
	bp2buildCtx.SetNameInterface(newNameResolver(configuration))

	// The bp2build process is a purely functional process that only depends on
	// Android.bp files. It must not depend on the values of per-build product
	// configurations or variables, since those will generate different BUILD
	// files based on how the user has configured their tree.
	bp2buildCtx.SetModuleListFile(bootstrap.ModuleListFile)
	extraNinjaDeps, err := bp2buildCtx.ListModulePaths(srcDir)
	if err != nil {
		panic(err)
	}
	extraNinjaDepsString := strings.Join(extraNinjaDeps, " \\\n ")

	// Run the loading and analysis pipeline to prepare the graph of regular
	// Modules parsed from Android.bp files, and the BazelTargetModules mapped
	// from the regular Modules.
	bootstrap.Main(bp2buildCtx.Context, configuration, extraNinjaDeps...)

	// Run the code-generation phase to convert BazelTargetModules to BUILD files
	// and print conversion metrics to the user.
	codegenContext := bp2build.NewCodegenContext(configuration, *bp2buildCtx, bp2build.Bp2Build)
	metrics := bp2build.Codegen(codegenContext)

	// Only report metrics when in bp2build mode. The metrics aren't relevant
	// for queryview, since that's a total repo-wide conversion and there's a
	// 1:1 mapping for each module.
	metrics.Print()

	// Workarounds to support running bp2build in a clean AOSP checkout with no
	// prior builds, and exiting early as soon as the BUILD files get generated,
	// therefore not creating build.ninja files that soong_ui and callers of
	// soong_build expects.
	//
	// These files are: build.ninja and build.ninja.d. Since Kati hasn't been
	// ran as well, and `nothing` is defined in a .mk file, there isn't a ninja
	// target called `nothing`, so we manually create it here.
	//
	// Even though outFile (build.ninja) and depFile (build.ninja.d) are values
	// passed into bootstrap.Main, they are package-private fields in bootstrap.
	// Short of modifying Blueprint to add an exported getter, inlining them
	// here is the next-best practical option.
	ninjaFileName := "build.ninja"
	ninjaFile := android.PathForOutput(codegenContext, ninjaFileName)
	ninjaFileD := android.PathForOutput(codegenContext, ninjaFileName+".d")
	// A workaround to create the 'nothing' ninja target so `m nothing` works,
	// since bp2build runs without Kati, and the 'nothing' target is declared in
	// a Makefile.
	android.WriteFileToOutputDir(ninjaFile, []byte("build nothing: phony\n  phony_output = true\n"), 0666)
	android.WriteFileToOutputDir(
		ninjaFileD,
		[]byte(fmt.Sprintf("%s: \\\n %s\n", ninjaFileName, extraNinjaDepsString)),
		0666)
}

// shouldPrepareBuildActions reads configuration and flags if build actions
// should be generated.
func shouldPrepareBuildActions(configuration android.Config) bool {
	// Generating Soong docs
	if docFile != "" {
		return false
	}

	// Generating a directory for Soong query (queryview)
	if bazelQueryViewDir != "" {
		return false
	}

	// Generating a directory for converted Bazel BUILD files
	return !bazelConversionRequested(configuration)
}
