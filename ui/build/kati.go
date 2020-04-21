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
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"android/soong/ui/metrics"
	"android/soong/ui/status"
)

var spaceSlashReplacer = strings.NewReplacer("/", "_", " ", "_")

const katiBuildSuffix = ""
const katiCleanspecSuffix = "-cleanspec"
const katiPackageSuffix = "-package"

// genKatiSuffix creates a suffix for kati-generated files so that we can cache
// them based on their inputs. So this should encode all common changes to Kati
// inputs. Currently that includes the TARGET_PRODUCT, kati-processed command
// line arguments, and the directories specified by mm/mmm.
func genKatiSuffix(ctx Context, config Config) {
	katiSuffix := "-" + config.TargetProduct()
	if args := config.KatiArgs(); len(args) > 0 {
		katiSuffix += "-" + spaceSlashReplacer.Replace(strings.Join(args, "_"))
	}

	// If the suffix is too long, replace it with a md5 hash and write a
	// file that contains the original suffix.
	if len(katiSuffix) > 64 {
		shortSuffix := "-" + fmt.Sprintf("%x", md5.Sum([]byte(katiSuffix)))
		config.SetKatiSuffix(shortSuffix)

		ctx.Verbosef("Kati ninja suffix too long: %q", katiSuffix)
		ctx.Verbosef("Replacing with: %q", shortSuffix)

		if err := ioutil.WriteFile(strings.TrimSuffix(config.KatiBuildNinjaFile(), "ninja")+"suf", []byte(katiSuffix), 0777); err != nil {
			ctx.Println("Error writing suffix file:", err)
		}
	} else {
		config.SetKatiSuffix(katiSuffix)
	}
}

func runKati(ctx Context, config Config, extraSuffix string, args []string, envFunc func(*Environment)) {
	executable := config.PrebuiltBuildTool("ckati")
	args = append([]string{
		"--ninja",
		"--ninja_dir=" + config.OutDir(),
		"--ninja_suffix=" + config.KatiSuffix() + extraSuffix,
		"--no_ninja_prelude",
		"--use_ninja_phony_output",
		"--regen",
		"--ignore_optional_include=" + filepath.Join(config.OutDir(), "%.P"),
		"--detect_android_echo",
		"--color_warnings",
		"--gen_all_targets",
		"--use_find_emulator",
		"--werror_find_emulator",
		"--no_builtin_rules",
		"--werror_suffix_rules",
		"--warn_real_to_phony",
		"--warn_phony_looks_real",
		"--werror_real_to_phony",
		"--werror_phony_looks_real",
		"--werror_writable",
		"--top_level_phony",
		"--kati_stats",
	}, args...)

	if config.Environment().IsEnvTrue("EMPTY_NINJA_FILE") {
		args = append(args, "--empty_ninja_file")
	}

	if config.UseRemoteBuild() {
		args = append(args, "--default_pool=local_pool")
	}

	cmd := Command(ctx, config, "ckati", executable, args...)
	cmd.Sandbox = katiSandbox
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		ctx.Fatalln("Error getting output pipe for ckati:", err)
	}
	cmd.Stderr = cmd.Stdout

	envFunc(cmd.Environment)

	if _, ok := cmd.Environment.Get("BUILD_USERNAME"); !ok {
		username := "unknown"
		if u, err := user.Current(); err == nil {
			username = u.Username
		} else {
			ctx.Println("Failed to get current user:", err)
		}
		cmd.Environment.Set("BUILD_USERNAME", username)
	}

	if _, ok := cmd.Environment.Get("BUILD_HOSTNAME"); !ok {
		hostname, err := os.Hostname()
		if err != nil {
			ctx.Println("Failed to read hostname:", err)
			hostname = "unknown"
		}
		cmd.Environment.Set("BUILD_HOSTNAME", hostname)
	}

	cmd.StartOrFatal()
	status.KatiReader(ctx.Status.StartTool(), pipe)
	cmd.WaitOrFatal()
}

func runKatiBuild(ctx Context, config Config) {
	ctx.BeginTrace(metrics.RunKati, "kati build")
	defer ctx.EndTrace()

	args := []string{
		"--writable", config.OutDir() + "/",
		"-f", "build/make/core/main.mk",
	}

	// PDK builds still uses a few implicit rules
	if !config.IsPdkBuild() {
		args = append(args, "--werror_implicit_rules")
	}

	if !config.BuildBrokenDupRules() {
		args = append(args, "--werror_overriding_commands")
	}

	args = append(args, config.KatiArgs()...)

	args = append(args,
		"SOONG_MAKEVARS_MK="+config.SoongMakeVarsMk(),
		"SOONG_ANDROID_MK="+config.SoongAndroidMk(),
		"TARGET_DEVICE_DIR="+config.TargetDeviceDir(),
		"KATI_PACKAGE_MK_DIR="+config.KatiPackageMkDir())

	runKati(ctx, config, katiBuildSuffix, args, func(env *Environment) {})

	cleanCopyHeaders(ctx, config)
	cleanOldInstalledFiles(ctx, config)
}

func cleanCopyHeaders(ctx Context, config Config) {
	ctx.BeginTrace("clean", "clean copy headers")
	defer ctx.EndTrace()

	data, err := ioutil.ReadFile(filepath.Join(config.ProductOut(), ".copied_headers_list"))
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		ctx.Fatalf("Failed to read copied headers list: %v", err)
	}

	headers := strings.Fields(string(data))
	if len(headers) < 1 {
		ctx.Fatal("Failed to parse copied headers list: %q", string(data))
	}
	headerDir := headers[0]
	headers = headers[1:]

	filepath.Walk(headerDir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return nil
			}
			if !inList(path, headers) {
				ctx.Printf("Removing obsolete header %q", path)
				if err := os.Remove(path); err != nil {
					ctx.Fatalf("Failed to remove obsolete header %q: %v", path, err)
				}
			}
			return nil
		})
}

func cleanOldInstalledFiles(ctx Context, config Config) {
	ctx.BeginTrace("clean", "clean old installed files")
	defer ctx.EndTrace()

	// We shouldn't be removing files from one side of the two-step asan builds
	var suffix string
	if v, ok := config.Environment().Get("SANITIZE_TARGET"); ok {
		if sanitize := strings.Fields(v); inList("address", sanitize) {
			suffix = "_asan"
		}
	}

	cleanOldFiles(ctx, config.ProductOut(), ".installable_files"+suffix)

	cleanOldFiles(ctx, config.HostOut(), ".installable_test_files")
}

func runKatiPackage(ctx Context, config Config) {
	ctx.BeginTrace(metrics.RunKati, "kati package")
	defer ctx.EndTrace()

	args := []string{
		"--writable", config.DistDir() + "/",
		"--werror_implicit_rules",
		"--werror_overriding_commands",
		"-f", "build/make/packaging/main.mk",
		"KATI_PACKAGE_MK_DIR=" + config.KatiPackageMkDir(),
	}

	runKati(ctx, config, katiPackageSuffix, args, func(env *Environment) {
		env.Allow([]string{
			// Some generic basics
			"LANG",
			"LC_MESSAGES",
			"PATH",
			"PWD",
			"TMPDIR",

			// Tool configs
			"ASAN_SYMBOLIZER_PATH",
			"JAVA_HOME",
			"PYTHONDONTWRITEBYTECODE",

			// Build configuration
			"ANDROID_BUILD_SHELL",
			"DIST_DIR",
			"OUT_DIR",
		}...)

		if config.Dist() {
			env.Set("DIST", "true")
			env.Set("DIST_DIR", config.DistDir())
		}
	})
}

func runKatiCleanSpec(ctx Context, config Config) {
	ctx.BeginTrace(metrics.RunKati, "kati cleanspec")
	defer ctx.EndTrace()

	runKati(ctx, config, katiCleanspecSuffix, []string{
		"--werror_implicit_rules",
		"--werror_overriding_commands",
		"-f", "build/make/core/cleanbuild.mk",
		"SOONG_MAKEVARS_MK=" + config.SoongMakeVarsMk(),
		"TARGET_DEVICE_DIR=" + config.TargetDeviceDir(),
	}, func(env *Environment) {})
}
