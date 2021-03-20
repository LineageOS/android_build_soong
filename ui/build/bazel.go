// Copyright 2020 Google Inc. All rights reserved.
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

package build

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"android/soong/bazel"
	"android/soong/shared"
	"android/soong/ui/metrics"
)

func getBazelInfo(ctx Context, config Config, bazelExecutable string, bazelEnv map[string]string, query string) string {
	infoCmd := Command(ctx, config, "bazel", bazelExecutable)

	if extraStartupArgs, ok := infoCmd.Environment.Get("BAZEL_STARTUP_ARGS"); ok {
		infoCmd.Args = append(infoCmd.Args, strings.Fields(extraStartupArgs)...)
	}

	// Obtain the output directory path in the execution root.
	infoCmd.Args = append(infoCmd.Args,
		"info",
		query,
	)

	for k, v := range bazelEnv {
		infoCmd.Environment.Set(k, v)
	}

	infoCmd.Dir = filepath.Join(config.OutDir(), "..")

	queryResult := strings.TrimSpace(string(infoCmd.OutputOrFatal()))
	return queryResult
}

// Main entry point to construct the Bazel build command line, environment
// variables and post-processing steps (e.g. converge output directories)
func runBazel(ctx Context, config Config) {
	ctx.BeginTrace(metrics.RunBazel, "bazel")
	defer ctx.EndTrace()

	// "droid" is the default ninja target.
	// TODO(b/160568333): stop hardcoding 'droid' to support building any
	// Ninja target.
	outputGroups := "droid"
	if len(config.ninjaArgs) > 0 {
		// At this stage, the residue slice of args passed to ninja
		// are the ninja targets to build, which can correspond directly
		// to ninja_build's output_groups.
		outputGroups = strings.Join(config.ninjaArgs, ",")
	}

	// Environment variables are the primary mechanism to pass information from
	// soong_ui configuration or context to Bazel.
	bazelEnv := make(map[string]string)

	// Use *_NINJA variables to pass the root-relative path of the combined,
	// kati-generated, soong-generated, and packaging Ninja files to Bazel.
	// Bazel reads these from the lunch() repository rule.
	bazelEnv["COMBINED_NINJA"] = config.CombinedNinjaFile()
	bazelEnv["KATI_NINJA"] = config.KatiBuildNinjaFile()
	bazelEnv["PACKAGE_NINJA"] = config.KatiPackageNinjaFile()
	bazelEnv["SOONG_NINJA"] = config.SoongNinjaFile()

	// NOTE: When Bazel is used, config.DistDir() is rigged to return a fake distdir under config.OutDir()
	// This is to ensure that Bazel can actually write there. See config.go for more details.
	bazelEnv["DIST_DIR"] = config.DistDir()

	bazelEnv["SHELL"] = "/bin/bash"

	// `tools/bazel` is the default entry point for executing Bazel in the AOSP
	// source tree.
	bazelExecutable := filepath.Join("tools", "bazel")
	cmd := Command(ctx, config, "bazel", bazelExecutable)

	// Append custom startup flags to the Bazel command. Startup flags affect
	// the Bazel server itself, and any changes to these flags would incur a
	// restart of the server, losing much of the in-memory incrementality.
	if extraStartupArgs, ok := cmd.Environment.Get("BAZEL_STARTUP_ARGS"); ok {
		cmd.Args = append(cmd.Args, strings.Fields(extraStartupArgs)...)
	}

	// Start constructing the `build` command.
	actionName := bazel.BazelNinjaExecRunName
	cmd.Args = append(cmd.Args,
		"build",
		// Use output_groups to select the set of outputs to produce from a
		// ninja_build target.
		"--output_groups="+outputGroups,
		// Generate a performance profile
		"--profile="+filepath.Join(shared.BazelMetricsFilename(config, actionName)),
		"--slim_profile=true",
	)

	if config.UseRBE() {
		for _, envVar := range []string{
			// RBE client
			"RBE_compare",
			"RBE_exec_strategy",
			"RBE_invocation_id",
			"RBE_log_dir",
			"RBE_num_retries_if_mismatched",
			"RBE_platform",
			"RBE_remote_accept_cache",
			"RBE_remote_update_cache",
			"RBE_server_address",
			// TODO: remove old FLAG_ variables.
			"FLAG_compare",
			"FLAG_exec_root",
			"FLAG_exec_strategy",
			"FLAG_invocation_id",
			"FLAG_log_dir",
			"FLAG_platform",
			"FLAG_remote_accept_cache",
			"FLAG_remote_update_cache",
			"FLAG_server_address",
		} {
			cmd.Args = append(cmd.Args,
				"--action_env="+envVar)
		}

		// We need to calculate --RBE_exec_root ourselves
		ctx.Println("Getting Bazel execution_root...")
		cmd.Args = append(cmd.Args, "--action_env=RBE_exec_root="+getBazelInfo(ctx, config, bazelExecutable, bazelEnv, "execution_root"))
	}

	// Ensure that the PATH environment variable value used in the action
	// environment is the restricted set computed from soong_ui, and not a
	// user-provided one, for hermeticity reasons.
	if pathEnvValue, ok := config.environ.Get("PATH"); ok {
		cmd.Environment.Set("PATH", pathEnvValue)
		cmd.Args = append(cmd.Args, "--action_env=PATH="+pathEnvValue)
	}

	// Append custom build flags to the Bazel command. Changes to these flags
	// may invalidate Bazel's analysis cache.
	// These should be appended as the final args, so that they take precedence.
	if extraBuildArgs, ok := cmd.Environment.Get("BAZEL_BUILD_ARGS"); ok {
		cmd.Args = append(cmd.Args, strings.Fields(extraBuildArgs)...)
	}

	// Append the label of the default ninja_build target.
	cmd.Args = append(cmd.Args,
		"//:"+config.TargetProduct()+"-"+config.TargetBuildVariant(),
	)

	// Execute the command at the root of the directory.
	cmd.Dir = filepath.Join(config.OutDir(), "..")

	for k, v := range bazelEnv {
		cmd.Environment.Set(k, v)
	}

	// Make a human-readable version of the bazelEnv map
	bazelEnvStringBuffer := new(bytes.Buffer)
	for k, v := range bazelEnv {
		fmt.Fprintf(bazelEnvStringBuffer, "%s=%s ", k, v)
	}

	// Print the implicit command line
	ctx.Println("Bazel implicit command line: " + strings.Join(cmd.Environment.Environ(), " ") + " " + cmd.Cmd.String() + "\n")

	// Print the explicit command line too
	ctx.Println("Bazel explicit command line: " + bazelEnvStringBuffer.String() + cmd.Cmd.String() + "\n")

	// Execute the build command.
	cmd.RunAndStreamOrFatal()

	// Post-processing steps start here. Once the Bazel build completes, the
	// output files are still stored in the execution root, not in $OUT_DIR.
	// Ensure that the $OUT_DIR contains the expected set of files by symlinking
	// the files from the execution root's output direction into $OUT_DIR.

	ctx.Println("Getting Bazel output_path...")
	outputBasePath := getBazelInfo(ctx, config, bazelExecutable, bazelEnv, "output_path")
	// TODO: Don't hardcode out/ as the bazel output directory. This is
	// currently hardcoded as ninja_build.output_root.
	bazelNinjaBuildOutputRoot := filepath.Join(outputBasePath, "..", "out")

	ctx.Println("Populating output directory...")
	populateOutdir(ctx, config, bazelNinjaBuildOutputRoot, ".")
}

// For all files F recursively under rootPath/relativePath, creates symlinks
// such that OutDir/F resolves to rootPath/F via symlinks.
// NOTE: For distdir paths we rename files instead of creating symlinks, so that the distdir is independent.
func populateOutdir(ctx Context, config Config, rootPath string, relativePath string) {
	destDir := filepath.Join(rootPath, relativePath)
	os.MkdirAll(destDir, 0755)
	files, err := ioutil.ReadDir(destDir)
	if err != nil {
		ctx.Fatal(err)
	}

	for _, f := range files {
		// The original Bazel file path
		destPath := filepath.Join(destDir, f.Name())

		// The desired Soong file path
		srcPath := filepath.Join(config.OutDir(), relativePath, f.Name())

		destLstatResult, destLstatErr := os.Lstat(destPath)
		if destLstatErr != nil {
			ctx.Fatalf("Unable to Lstat dest %s: %s", destPath, destLstatErr)
		}

		srcLstatResult, srcLstatErr := os.Lstat(srcPath)

		if srcLstatErr == nil {
			if srcLstatResult.IsDir() && destLstatResult.IsDir() {
				// src and dest are both existing dirs - recurse on the dest dir contents...
				populateOutdir(ctx, config, rootPath, filepath.Join(relativePath, f.Name()))
			} else {
				// Ignore other pre-existing src files (could be pre-existing files, directories, symlinks, ...)
				// This can arise for files which are generated under OutDir outside of soong_build, such as .bootstrap files.
				// FIXME: This might cause a problem later e.g. if a symlink in the build graph changes...
			}
		} else {
			if !os.IsNotExist(srcLstatErr) {
				ctx.Fatalf("Unable to Lstat src %s: %s", srcPath, srcLstatErr)
			}

			if strings.Contains(destDir, config.DistDir()) {
				// We need to make a "real" file/dir instead of making a symlink (because the distdir can't have symlinks)
				// Rename instead of copy in order to save disk space.
				if err := os.Rename(destPath, srcPath); err != nil {
					ctx.Fatalf("Unable to rename %s -> %s due to error %s", srcPath, destPath, err)
				}
			} else {
				// src does not exist, so try to create a src -> dest symlink (i.e. a Soong path -> Bazel path symlink)
				if err := os.Symlink(destPath, srcPath); err != nil {
					ctx.Fatalf("Unable to create symlink %s -> %s due to error %s", srcPath, destPath, err)
				}
			}
		}
	}
}
