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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func runBazel(ctx Context, config Config) {
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

	bazelExecutable := filepath.Join("tools", "bazel")
	cmd := Command(ctx, config, "bazel", bazelExecutable)

	if extra_startup_args, ok := cmd.Environment.Get("BAZEL_STARTUP_ARGS"); ok {
		cmd.Args = append(cmd.Args, strings.Fields(extra_startup_args)...)
	}

	cmd.Args = append(cmd.Args,
		"build",
		"--output_groups="+outputGroups,
	)

	if extra_build_args, ok := cmd.Environment.Get("BAZEL_BUILD_ARGS"); ok {
		cmd.Args = append(cmd.Args, strings.Fields(extra_build_args)...)
	}

	cmd.Args = append(cmd.Args,
		"//:"+config.TargetProduct()+"-"+config.TargetBuildVariant(),
	)

	cmd.Environment.Set("DIST_DIR", config.DistDir())
	cmd.Environment.Set("SHELL", "/bin/bash")

	ctx.Println(cmd.Cmd)
	cmd.Dir = filepath.Join(config.OutDir(), "..")
	ctx.Status.Status("Starting Bazel..")
	cmd.RunAndStreamOrFatal()

	// Obtain the Bazel output directory for ninja_build.
	infoCmd := Command(ctx, config, "bazel", bazelExecutable)

	if extra_startup_args, ok := infoCmd.Environment.Get("BAZEL_STARTUP_ARGS"); ok {
		infoCmd.Args = append(infoCmd.Args, strings.Fields(extra_startup_args)...)
	}

	infoCmd.Args = append(infoCmd.Args,
		"info",
		"output_path",
	)

	infoCmd.Environment.Set("DIST_DIR", config.DistDir())
	infoCmd.Environment.Set("SHELL", "/bin/bash")
	infoCmd.Dir = filepath.Join(config.OutDir(), "..")
	ctx.Status.Status("Getting Bazel Info..")
	outputBasePath := string(infoCmd.OutputOrFatal())
	// TODO: Don't hardcode out/ as the bazel output directory. This is
	// currently hardcoded as ninja_build.output_root.
	bazelNinjaBuildOutputRoot := filepath.Join(outputBasePath, "..", "out")

	symlinkOutdir(ctx, config, bazelNinjaBuildOutputRoot, ".")
}

// For all files F recursively under rootPath/relativePath, creates symlinks
// such that OutDir/F resolves to rootPath/F via symlinks.
func symlinkOutdir(ctx Context, config Config, rootPath string, relativePath string) {
	destDir := filepath.Join(rootPath, relativePath)
	os.MkdirAll(destDir, 0755)
	files, err := ioutil.ReadDir(destDir)
	if err != nil {
		ctx.Fatal(err)
	}
	for _, f := range files {
		destPath := filepath.Join(destDir, f.Name())
		srcPath := filepath.Join(config.OutDir(), relativePath, f.Name())
		if statResult, err := os.Stat(srcPath); err == nil {
			if statResult.Mode().IsDir() && f.IsDir() {
				// Directory under OutDir already exists, so recurse on its contents.
				symlinkOutdir(ctx, config, rootPath, filepath.Join(relativePath, f.Name()))
			} else if !statResult.Mode().IsDir() && !f.IsDir() {
				// File exists both in source and destination, and it's not a directory
				// in either location. Do nothing.
				// This can arise for files which are generated under OutDir outside of
				// soong_build, such as .bootstrap files.
			} else {
				// File is a directory in one location but not the other. Raise an error.
				ctx.Fatalf("Could not link %s to %s due to conflict", srcPath, destPath)
			}
		} else if os.IsNotExist(err) {
			// Create symlink srcPath -> fullDestPath.
			os.Symlink(destPath, srcPath)
		} else {
			ctx.Fatalf("Unable to stat %s: %s", srcPath, err)
		}
	}
}
