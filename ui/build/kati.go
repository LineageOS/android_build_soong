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
	"android/soong/ui/metrics"
	"android/soong/ui/status"
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

var spaceSlashReplacer = strings.NewReplacer("/", "_", " ", "_")

const katiBuildSuffix = ""
const katiCleanspecSuffix = "-cleanspec"
const katiPackageSuffix = "-package"

// genKatiSuffix creates a filename suffix for kati-generated files so that we
// can cache them based on their inputs. Such files include the generated Ninja
// files and env.sh environment variable setup files.
//
// The filename suffix should encode all common changes to Kati inputs.
// Currently that includes the TARGET_PRODUCT and kati-processed command line
// arguments.
func genKatiSuffix(ctx Context, config Config) {
	// Construct the base suffix.
	katiSuffix := "-" + config.TargetProduct()

	// Append kati arguments to the suffix.
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

func writeValueIfChanged(ctx Context, config Config, dir string, filename string, value string) {
	filePath := filepath.Join(dir, filename)
	previousValue := ""
	rawPreviousValue, err := ioutil.ReadFile(filePath)
	if err == nil {
		previousValue = string(rawPreviousValue)
	}

	if previousValue != value {
		if err = ioutil.WriteFile(filePath, []byte(value), 0666); err != nil {
			ctx.Fatalf("Failed to write: %v", err)
		}
	}
}

// Base function to construct and run the Kati command line with additional
// arguments, and a custom function closure to mutate the environment Kati runs
// in.
func runKati(ctx Context, config Config, extraSuffix string, args []string, envFunc func(*Environment)) {
	executable := config.PrebuiltBuildTool("ckati")
	// cKati arguments.
	args = append([]string{
		// Instead of executing commands directly, generate a Ninja file.
		"--ninja",
		// Generate Ninja files in the output directory.
		"--ninja_dir=" + config.OutDir(),
		// Filename suffix of the generated Ninja file.
		"--ninja_suffix=" + config.KatiSuffix() + extraSuffix,
		// Remove common parts at the beginning of a Ninja file, like build_dir,
		// local_pool and _kati_always_build_. Allows Kati to be run multiple
		// times, with generated Ninja files combined in a single invocation
		// using 'include'.
		"--no_ninja_prelude",
		// Support declaring phony outputs in AOSP Ninja.
		"--use_ninja_phony_output",
		// Support declaring symlink outputs in AOSP Ninja.
		"--use_ninja_symlink_outputs",
		// Regenerate the Ninja file if environment inputs have changed. e.g.
		// CLI flags, .mk file timestamps, env vars, $(wildcard ..) and some
		// $(shell ..) results.
		"--regen",
		// Skip '-include' directives starting with the specified path. Used to
		// ignore generated .mk files.
		"--ignore_optional_include=" + filepath.Join(config.OutDir(), "%.P"),
		// Detect the use of $(shell echo ...).
		"--detect_android_echo",
		// Colorful ANSI-based warning and error messages.
		"--color_warnings",
		// Generate all targets, not just the top level requested ones.
		"--gen_all_targets",
		// Use the built-in emulator of GNU find for better file finding
		// performance. Used with $(shell find ...).
		"--use_find_emulator",
		// Fail when the find emulator encounters problems.
		"--werror_find_emulator",
		// Do not provide any built-in rules.
		"--no_builtin_rules",
		// Fail when suffix rules are used.
		"--werror_suffix_rules",
		// Fail when a real target depends on a phony target.
		"--werror_real_to_phony",
		// Makes real_to_phony checks assume that any top-level or leaf
		// dependencies that does *not* have a '/' in it is a phony target.
		"--top_level_phony",
		// Fail when a phony target contains slashes.
		"--werror_phony_looks_real",
		// Fail when writing to a read-only directory.
		"--werror_writable",
		// Print Kati's internal statistics, such as the number of variables,
		// implicit/explicit/suffix rules, and so on.
		"--kati_stats",
	}, args...)

	// Generate a minimal Ninja file.
	//
	// Used for build_test and multiproduct_kati, which runs Kati several
	// hundred times for different configurations to test file generation logic.
	// These can result in generating Ninja files reaching ~1GB or more,
	// resulting in ~hundreds of GBs of writes.
	//
	// Since we don't care about executing the Ninja files in these test cases,
	// generating the Ninja file content wastes time, so skip writing any
	// information out with --empty_ninja_file.
	//
	// From https://github.com/google/kati/commit/87b8da7af2c8bea28b1d8ab17679453d859f96e5
	if config.EmptyNinjaFile() {
		args = append(args, "--empty_ninja_file")
	}

	// Apply 'local_pool' to to all rules that don't specify a pool.
	if config.UseRemoteBuild() {
		args = append(args, "--default_pool=local_pool")
	}

	cmd := Command(ctx, config, "ckati", executable, args...)

	// Set up the nsjail sandbox.
	cmd.Sandbox = katiSandbox

	// Set up stdout and stderr.
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		ctx.Fatalln("Error getting output pipe for ckati:", err)
	}
	cmd.Stderr = cmd.Stdout

	var username string
	// Pass on various build environment metadata to Kati.
	if usernameFromEnv, ok := cmd.Environment.Get("BUILD_USERNAME"); !ok {
		username = "unknown"
		if u, err := user.Current(); err == nil {
			username = u.Username
		} else {
			ctx.Println("Failed to get current user:", err)
		}
		cmd.Environment.Set("BUILD_USERNAME", username)
	} else {
		username = usernameFromEnv
	}

	hostname, ok := cmd.Environment.Get("BUILD_HOSTNAME")
	// Unset BUILD_HOSTNAME during kati run to avoid kati rerun, kati will use BUILD_HOSTNAME from a file.
	cmd.Environment.Unset("BUILD_HOSTNAME")
	if !ok {
		hostname, err = os.Hostname()
		if err != nil {
			ctx.Println("Failed to read hostname:", err)
			hostname = "unknown"
		}
	}
	writeValueIfChanged(ctx, config, config.SoongOutDir(), "build_hostname.txt", hostname)
	_, ok = cmd.Environment.Get("BUILD_NUMBER")
	// Unset BUILD_NUMBER during kati run to avoid kati rerun, kati will use BUILD_NUMBER from a file.
	cmd.Environment.Unset("BUILD_NUMBER")
	if ok {
		cmd.Environment.Set("HAS_BUILD_NUMBER", "true")
	} else {
		cmd.Environment.Set("HAS_BUILD_NUMBER", "false")
	}

	// Apply the caller's function closure to mutate the environment variables.
	envFunc(cmd.Environment)

	cmd.StartOrFatal()
	// Set up the ToolStatus command line reader for Kati for a consistent UI
	// for the user.
	status.KatiReader(ctx.Status.StartTool(), pipe)
	cmd.WaitOrFatal()
}

func runKatiBuild(ctx Context, config Config) {
	ctx.BeginTrace(metrics.RunKati, "kati build")
	defer ctx.EndTrace()

	args := []string{
		// Mark the output directory as writable.
		"--writable", config.OutDir() + "/",
		// Fail when encountering implicit rules. e.g.
		// %.foo: %.bar
		//   cp $< $@
		"--werror_implicit_rules",
		// Entry point for the Kati Ninja file generation.
		"-f", "build/make/core/main.mk",
	}

	if !config.BuildBrokenDupRules() {
		// Fail when redefining / duplicating a target.
		args = append(args, "--werror_overriding_commands")
	}

	args = append(args, config.KatiArgs()...)

	args = append(args,
		// Location of the Make vars .mk file generated by Soong.
		"SOONG_MAKEVARS_MK="+config.SoongMakeVarsMk(),
		// Location of the Android.mk file generated by Soong. This
		// file contains Soong modules represented as Kati modules,
		// allowing Kati modules to depend on Soong modules.
		"SOONG_ANDROID_MK="+config.SoongAndroidMk(),
		// Directory containing outputs for the target device.
		"TARGET_DEVICE_DIR="+config.TargetDeviceDir(),
		// Directory containing .mk files for packaging purposes, such as
		// the dist.mk file, containing dist-for-goals data.
		"KATI_PACKAGE_MK_DIR="+config.KatiPackageMkDir())

	runKati(ctx, config, katiBuildSuffix, args, func(env *Environment) {})

	// compress and dist the main build ninja file.
	distGzipFile(ctx, config, config.KatiBuildNinjaFile())

	// Cleanup steps.
	cleanCopyHeaders(ctx, config)
	cleanOldInstalledFiles(ctx, config)
}

// Clean out obsolete header files on the disk that were *not copied* during the
// build with BUILD_COPY_HEADERS and LOCAL_COPY_HEADERS.
//
// These should be increasingly uncommon, as it's a deprecated feature and there
// isn't an equivalent feature in Soong.
func cleanCopyHeaders(ctx Context, config Config) {
	ctx.BeginTrace("clean", "clean copy headers")
	defer ctx.EndTrace()

	// Read and parse the list of copied headers from a file in the product
	// output directory.
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

	// Walk the tree and remove any headers that are not in the list of copied
	// headers in the current build.
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

// Clean out any previously installed files from the disk that are not installed
// in the current build.
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

// Generate the Ninja file containing the packaging command lines for the dist
// dir.
func runKatiPackage(ctx Context, config Config) {
	ctx.BeginTrace(metrics.RunKati, "kati package")
	defer ctx.EndTrace()

	args := []string{
		// Mark the dist dir as writable.
		"--writable", config.DistDir() + "/",
		// Fail when encountering implicit rules. e.g.
		"--werror_implicit_rules",
		// Fail when redefining / duplicating a target.
		"--werror_overriding_commands",
		// Entry point.
		"-f", "build/make/packaging/main.mk",
		// Directory containing .mk files for packaging purposes, such as
		// the dist.mk file, containing dist-for-goals data.
		"KATI_PACKAGE_MK_DIR=" + config.KatiPackageMkDir(),
	}

	// Run Kati against a restricted set of environment variables.
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
			"FILE_NAME_TAG",
		}...)

		if config.Dist() {
			env.Set("DIST", "true")
			env.Set("DIST_DIR", config.DistDir())
		}
	})

	// Compress and dist the packaging Ninja file.
	distGzipFile(ctx, config, config.KatiPackageNinjaFile())
}

// Run Kati on the cleanspec files to clean the build.
func runKatiCleanSpec(ctx Context, config Config) {
	ctx.BeginTrace(metrics.RunKati, "kati cleanspec")
	defer ctx.EndTrace()

	runKati(ctx, config, katiCleanspecSuffix, []string{
		// Fail when encountering implicit rules. e.g.
		"--werror_implicit_rules",
		// Fail when redefining / duplicating a target.
		"--werror_overriding_commands",
		// Entry point.
		"-f", "build/make/core/cleanbuild.mk",
		"SOONG_MAKEVARS_MK=" + config.SoongMakeVarsMk(),
		"TARGET_DEVICE_DIR=" + config.TargetDeviceDir(),
	}, func(env *Environment) {})
}
