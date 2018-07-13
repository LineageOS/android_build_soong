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
	"path/filepath"
	"strconv"
	"strings"

	"android/soong/ui/status"
)

var spaceSlashReplacer = strings.NewReplacer("/", "_", " ", "_")

// genKatiSuffix creates a suffix for kati-generated files so that we can cache
// them based on their inputs. So this should encode all common changes to Kati
// inputs. Currently that includes the TARGET_PRODUCT, kati-processed command
// line arguments, and the directories specified by mm/mmm.
func genKatiSuffix(ctx Context, config Config) {
	katiSuffix := "-" + config.TargetProduct()
	if args := config.KatiArgs(); len(args) > 0 {
		katiSuffix += "-" + spaceSlashReplacer.Replace(strings.Join(args, "_"))
	}
	if oneShot, ok := config.Environment().Get("ONE_SHOT_MAKEFILE"); ok {
		katiSuffix += "-" + spaceSlashReplacer.Replace(oneShot)
	}

	// If the suffix is too long, replace it with a md5 hash and write a
	// file that contains the original suffix.
	if len(katiSuffix) > 64 {
		shortSuffix := "-" + fmt.Sprintf("%x", md5.Sum([]byte(katiSuffix)))
		config.SetKatiSuffix(shortSuffix)

		ctx.Verbosef("Kati ninja suffix too long: %q", katiSuffix)
		ctx.Verbosef("Replacing with: %q", shortSuffix)

		if err := ioutil.WriteFile(strings.TrimSuffix(config.KatiNinjaFile(), "ninja")+"suf", []byte(katiSuffix), 0777); err != nil {
			ctx.Println("Error writing suffix file:", err)
		}
	} else {
		config.SetKatiSuffix(katiSuffix)
	}
}

func runKati(ctx Context, config Config) {
	genKatiSuffix(ctx, config)

	runKatiCleanSpec(ctx, config)

	ctx.BeginTrace("kati")
	defer ctx.EndTrace()

	executable := config.PrebuiltBuildTool("ckati")
	args := []string{
		"--ninja",
		"--ninja_dir=" + config.OutDir(),
		"--ninja_suffix=" + config.KatiSuffix(),
		"--regen",
		"--ignore_optional_include=" + filepath.Join(config.OutDir(), "%.P"),
		"--detect_android_echo",
		"--color_warnings",
		"--gen_all_targets",
		"--werror_find_emulator",
		"--no_builtin_rules",
		"--werror_suffix_rules",
		"--kati_stats",
		"-f", "build/make/core/main.mk",
	}

	// PDK builds still uses a few implicit rules
	if !config.IsPdkBuild() {
		args = append(args, "--werror_implicit_rules")
	}

	if !config.BuildBrokenDupRules() {
		args = append(args, "--werror_overriding_commands")
	}

	if !config.Environment().IsFalse("KATI_EMULATE_FIND") {
		args = append(args, "--use_find_emulator")
	}

	args = append(args, config.KatiArgs()...)

	args = append(args,
		"BUILDING_WITH_NINJA=true",
		"SOONG_ANDROID_MK="+config.SoongAndroidMk(),
		"SOONG_MAKEVARS_MK="+config.SoongMakeVarsMk(),
		"TARGET_DEVICE_DIR="+config.TargetDeviceDir())

	if config.UseGoma() {
		args = append(args, "-j"+strconv.Itoa(config.Parallel()))
	}

	cmd := Command(ctx, config, "ckati", executable, args...)
	cmd.Sandbox = katiSandbox
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		ctx.Fatalln("Error getting output pipe for ckati:", err)
	}
	cmd.Stderr = cmd.Stdout

	cmd.StartOrFatal()
	status.KatiReader(ctx.Status.StartTool(), pipe)
	cmd.WaitOrFatal()
}

func runKatiCleanSpec(ctx Context, config Config) {
	ctx.BeginTrace("kati cleanspec")
	defer ctx.EndTrace()

	executable := config.PrebuiltBuildTool("ckati")
	args := []string{
		"--ninja",
		"--ninja_dir=" + config.OutDir(),
		"--ninja_suffix=" + config.KatiSuffix() + "-cleanspec",
		"--regen",
		"--detect_android_echo",
		"--color_warnings",
		"--gen_all_targets",
		"--werror_find_emulator",
		"--werror_overriding_commands",
		"--use_find_emulator",
		"--kati_stats",
		"-f", "build/make/core/cleanbuild.mk",
		"BUILDING_WITH_NINJA=true",
		"SOONG_MAKEVARS_MK=" + config.SoongMakeVarsMk(),
		"TARGET_DEVICE_DIR=" + config.TargetDeviceDir(),
	}

	cmd := Command(ctx, config, "ckati", executable, args...)
	cmd.Sandbox = katiCleanSpecSandbox
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		ctx.Fatalln("Error getting output pipe for ckati:", err)
	}
	cmd.Stderr = cmd.Stdout

	cmd.StartOrFatal()
	status.KatiReader(ctx.Status.StartTool(), pipe)
	cmd.WaitOrFatal()
}
