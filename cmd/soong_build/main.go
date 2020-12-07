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

	"github.com/google/blueprint/bootstrap"

	"android/soong/android"
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
	return configuration.IsEnvTrue("CONVERT_TO_BAZEL")
}

func newContext(srcDir string, configuration android.Config) *android.Context {
	ctx := android.NewContext(configuration)
	if bazelConversionRequested(configuration) {
		// Register an alternate set of singletons and mutators for bazel
		// conversion for Bazel conversion.
		ctx.RegisterForBazelConversion()
	} else {
		ctx.Register()
	}
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
	if configuration.BazelContext.BazelEnabled() {
		// Bazel-enabled mode. Soong runs in two passes.
		// First pass: Analyze the build tree, but only store all bazel commands
		// needed to correctly evaluate the tree in the second pass.
		// TODO(cparsons): Don't output any ninja file, as the second pass will overwrite
		// the incorrect results from the first pass, and file I/O is expensive.
		firstCtx := newContext(srcDir, configuration)
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
		ctx = newContext(srcDir, secondPassConfig)
		bootstrap.Main(ctx.Context, secondPassConfig, extraNinjaDeps...)
	} else {
		ctx = newContext(srcDir, configuration)
		bootstrap.Main(ctx.Context, configuration, extraNinjaDeps...)
	}

	// Convert the Soong module graph into Bazel BUILD files.
	if bazelQueryViewDir != "" {
		if err := createBazelQueryView(ctx, bazelQueryViewDir); err != nil {
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
