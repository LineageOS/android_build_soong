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
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/blueprint/microfactory"
)

func runSoong(ctx Context, config Config) {
	ctx.BeginTrace("soong")
	defer ctx.EndTrace()

	func() {
		ctx.BeginTrace("blueprint bootstrap")
		defer ctx.EndTrace()

		cmd := Command(ctx, config, "blueprint bootstrap", "build/blueprint/bootstrap.bash", "-t")
		cmd.Environment.Set("BLUEPRINTDIR", "./build/blueprint")
		cmd.Environment.Set("BOOTSTRAP", "./build/blueprint/bootstrap.bash")
		cmd.Environment.Set("BUILDDIR", config.SoongOutDir())
		cmd.Environment.Set("GOROOT", "./"+filepath.Join("prebuilts/go", config.HostPrebuiltTag()))
		cmd.Environment.Set("BLUEPRINT_LIST_FILE", filepath.Join(config.FileListDir(), "Android.bp.list"))
		cmd.Environment.Set("NINJA_BUILDDIR", config.OutDir())
		cmd.Environment.Set("SRCDIR", ".")
		cmd.Environment.Set("TOPNAME", "Android.bp")
		cmd.Sandbox = soongSandbox
		cmd.Stdout = ctx.Stdout()
		cmd.Stderr = ctx.Stderr()
		cmd.RunOrFatal()
	}()

	func() {
		ctx.BeginTrace("environment check")
		defer ctx.EndTrace()

		envFile := filepath.Join(config.SoongOutDir(), ".soong.environment")
		envTool := filepath.Join(config.SoongOutDir(), ".bootstrap/bin/soong_env")
		if _, err := os.Stat(envFile); err == nil {
			if _, err := os.Stat(envTool); err == nil {
				cmd := Command(ctx, config, "soong_env", envTool, envFile)
				cmd.Sandbox = soongSandbox
				cmd.Stdout = ctx.Stdout()
				cmd.Stderr = ctx.Stderr()
				if err := cmd.Run(); err != nil {
					ctx.Verboseln("soong_env failed, forcing manifest regeneration")
					os.Remove(envFile)
				}
			} else {
				ctx.Verboseln("Missing soong_env tool, forcing manifest regeneration")
				os.Remove(envFile)
			}
		} else if !os.IsNotExist(err) {
			ctx.Fatalf("Failed to stat %f: %v", envFile, err)
		}
	}()

	func() {
		ctx.BeginTrace("minibp")
		defer ctx.EndTrace()

		var cfg microfactory.Config
		cfg.Map("github.com/google/blueprint", "build/blueprint")

		cfg.TrimPath = absPath(ctx, ".")

		minibp := filepath.Join(config.SoongOutDir(), ".minibootstrap/minibp")
		if _, err := microfactory.Build(&cfg, minibp, "github.com/google/blueprint/bootstrap/minibp"); err != nil {
			ctx.Fatalln("Failed to build minibp:", err)
		}
	}()

	ninja := func(name, file string) {
		ctx.BeginTrace(name)
		defer ctx.EndTrace()

		cmd := Command(ctx, config, "soong "+name,
			config.PrebuiltBuildTool("ninja"),
			"-d", "keepdepfile",
			"-w", "dupbuild=err",
			"-j", strconv.Itoa(config.Parallel()),
			"-f", filepath.Join(config.SoongOutDir(), file))
		if config.IsVerbose() {
			cmd.Args = append(cmd.Args, "-v")
		}
		cmd.Sandbox = soongSandbox
		cmd.Stdin = ctx.Stdin()
		cmd.Stdout = ctx.Stdout()
		cmd.Stderr = ctx.Stderr()

		defer ctx.ImportNinjaLog(filepath.Join(config.OutDir(), ".ninja_log"), time.Now())
		cmd.RunOrFatal()
	}

	ninja("minibootstrap", ".minibootstrap/build.ninja")
	ninja("bootstrap", ".bootstrap/build.ninja")
}
