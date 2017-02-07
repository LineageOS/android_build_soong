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
)

func runSoongBootstrap(ctx Context, config Config) {
	ctx.BeginTrace("bootstrap soong")
	defer ctx.EndTrace()

	cmd := exec.CommandContext(ctx.Context, "./bootstrap.bash")
	env := config.Environment().Copy()
	env.Set("BUILDDIR", config.SoongOutDir())
	cmd.Env = env.Environ()
	cmd.Stdout = ctx.Stdout()
	cmd.Stderr = ctx.Stderr()
	ctx.Verboseln(cmd.Path, cmd.Args)
	if err := cmd.Run(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			ctx.Fatalln("soong bootstrap failed with:", e.ProcessState.String())
		} else {
			ctx.Fatalln("Failed to run soong bootstrap:", err)
		}
	}
}

func runSoong(ctx Context, config Config) {
	ctx.BeginTrace("soong")
	defer ctx.EndTrace()

	cmd := exec.CommandContext(ctx.Context, filepath.Join(config.SoongOutDir(), "soong"), "-w", "dupbuild=err")
	if config.IsVerbose() {
		cmd.Args = append(cmd.Args, "-v")
	}
	env := config.Environment().Copy()
	env.Set("SKIP_NINJA", "true")
	cmd.Env = env.Environ()
	cmd.Stdin = ctx.Stdin()
	cmd.Stdout = ctx.Stdout()
	cmd.Stderr = ctx.Stderr()
	ctx.Verboseln(cmd.Path, cmd.Args)
	if err := cmd.Run(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			ctx.Fatalln("soong bootstrap failed with:", e.ProcessState.String())
		} else {
			ctx.Fatalln("Failed to run soong bootstrap:", err)
		}
	}
}
