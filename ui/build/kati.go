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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

	executable := "prebuilts/build-tools/" + config.HostPrebuiltTag() + "/bin/ckati"
	args := []string{
		"--ninja",
		"--ninja_dir=" + config.OutDir(),
		"--ninja_suffix=" + config.KatiSuffix(),
		"--regen",
		"--ignore_optional_include=" + filepath.Join(config.OutDir(), "%.P"),
		"--detect_android_echo",
	}

	if !config.Environment().IsFalse("KATI_EMULATE_FIND") {
		args = append(args, "--use_find_emulator")
	}

	// The argument order could be simplified, but currently this matches
	// the ordering in Make
	args = append(args, "-f", "build/core/main.mk")

	args = append(args, config.KatiArgs()...)

	args = append(args,
		"--gen_all_targets",
		"BUILDING_WITH_NINJA=true",
		"SOONG_ANDROID_MK="+config.SoongAndroidMk(),
		"SOONG_MAKEVARS_MK="+config.SoongMakeVarsMk())

	if config.UseGoma() {
		args = append(args, "-j"+strconv.Itoa(config.Parallel()))
	}

	cmd := exec.CommandContext(ctx.Context, executable, args...)
	cmd.Env = config.Environment().Environ()
	cmd.Stdout = ctx.Stdout()
	cmd.Stderr = ctx.Stderr()
	ctx.Verboseln(cmd.Path, cmd.Args)
	if err := cmd.Run(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			ctx.Fatalln("ckati failed with:", e.ProcessState.String())
		} else {
			ctx.Fatalln("Failed to run ckati:", err)
		}
	}
}
