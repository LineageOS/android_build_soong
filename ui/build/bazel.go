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
	"path/filepath"
	"strings"
)

func runBazel(ctx Context, config Config) {
	// "droid" is the default ninja target.
	outputGroups := "droid"
	if len(config.ninjaArgs) > 0 {
		// At this stage, the residue slice of args passed to ninja
		// are the ninja targets to build, which can correspond directly
		// to ninja_build's output_groups.
		outputGroups = strings.Join(config.ninjaArgs, ",")
	}

	bazelExecutable := filepath.Join("tools", "bazel")
	args := []string{
		"build",
		"--verbose_failures",
		"--show_progress_rate_limit=0.05",
		"--color=yes",
		"--curses=yes",
		"--show_timestamps",
		"--announce_rc",
		"--output_groups=" + outputGroups,
		"//:" + config.TargetProduct() + "-" + config.TargetBuildVariant(),
	}

	cmd := Command(ctx, config, "bazel", bazelExecutable, args...)

	cmd.Environment.Set("DIST_DIR", config.DistDir())
	cmd.Environment.Set("SHELL", "/bin/bash")

	ctx.Println(cmd.Cmd)
	cmd.Dir = filepath.Join(config.OutDir(), "..")
	ctx.Status.Status("Starting Bazel..")
	cmd.RunAndStreamOrFatal()
}
