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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func runNinja(ctx Context, config Config) {
	ctx.BeginTrace("ninja")
	defer ctx.EndTrace()

	executable := "prebuilts/build-tools/" + config.HostPrebuiltTag() + "/bin/ninja"
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

	env := config.Environment().Copy()
	env.AppendFromKati(config.KatiEnvFile())

	// Allow both NINJA_ARGS and NINJA_EXTRA_ARGS, since both have been
	// used in the past to specify extra ninja arguments.
	if extra, ok := env.Get("NINJA_ARGS"); ok {
		args = append(args, strings.Fields(extra)...)
	}
	if extra, ok := env.Get("NINJA_EXTRA_ARGS"); ok {
		args = append(args, strings.Fields(extra)...)
	}

	if _, ok := env.Get("NINJA_STATUS"); !ok {
		env.Set("NINJA_STATUS", "[%p %f/%t] ")
	}

	cmd := exec.CommandContext(ctx.Context, executable, args...)
	cmd.Env = env.Environ()
	cmd.Stdin = ctx.Stdin()
	cmd.Stdout = ctx.Stdout()
	cmd.Stderr = ctx.Stderr()
	ctx.Verboseln(cmd.Path, cmd.Args)
	startTime := time.Now()
	defer ctx.ImportNinjaLog(filepath.Join(config.OutDir(), ".ninja_log"), startTime)
	if err := cmd.Run(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			ctx.Fatalln("ninja failed with:", e.ProcessState.String())
		} else {
			ctx.Fatalln("Failed to run ninja:", err)
		}
	}
}
