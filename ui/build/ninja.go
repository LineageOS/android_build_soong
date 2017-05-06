// Copyright 2017 Google Inc. All rights reserved.
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
	"strconv"
	"strings"
	"time"
)

func runNinja(ctx Context, config Config) {
	ctx.BeginTrace("ninja")
	defer ctx.EndTrace()

	executable := config.PrebuiltBuildTool("ninja")
	args := []string{
		"-d", "keepdepfile",
	}

	args = append(args, config.NinjaArgs()...)

	var parallel int
	if config.UseGoma() {
		parallel = config.RemoteParallel()
	} else {
		parallel = config.Parallel()
	}
	args = append(args, "-j", strconv.Itoa(parallel))
	if config.keepGoing != 1 {
		args = append(args, "-k", strconv.Itoa(config.keepGoing))
	}

	args = append(args, "-f", config.CombinedNinjaFile())

	if config.IsVerbose() {
		args = append(args, "-v")
	}
	args = append(args, "-w", "dupbuild=err")

	cmd := Command(ctx, config, "ninja", executable, args...)
	cmd.Environment.AppendFromKati(config.KatiEnvFile())

	// Allow both NINJA_ARGS and NINJA_EXTRA_ARGS, since both have been
	// used in the past to specify extra ninja arguments.
	if extra, ok := cmd.Environment.Get("NINJA_ARGS"); ok {
		cmd.Args = append(cmd.Args, strings.Fields(extra)...)
	}
	if extra, ok := cmd.Environment.Get("NINJA_EXTRA_ARGS"); ok {
		cmd.Args = append(cmd.Args, strings.Fields(extra)...)
	}

	if _, ok := cmd.Environment.Get("NINJA_STATUS"); !ok {
		cmd.Environment.Set("NINJA_STATUS", "[%p %f/%t] ")
	}

	cmd.Stdin = ctx.Stdin()
	cmd.Stdout = ctx.Stdout()
	cmd.Stderr = ctx.Stderr()
	startTime := time.Now()
	defer ctx.ImportNinjaLog(filepath.Join(config.OutDir(), ".ninja_log"), startTime)
	cmd.RunOrFatal()
}
