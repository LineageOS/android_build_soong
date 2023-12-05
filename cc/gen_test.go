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

package cc

import (
	"path/filepath"
	"strings"
	"testing"

	"android/soong/android"
)

func TestGen(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ctx := testCc(t, `
		cc_library_shared {
			name: "libfoo",
			srcs: [
				"foo.c",
				"b.aidl",
			],
		}`)

		aidl := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_shared").Rule("aidl")
		libfoo := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_shared").Module().(*Module)

		expected := "-I" + filepath.Dir(aidl.Output.String())
		actual := android.StringsRelativeToTop(ctx.Config(), libfoo.flags.Local.CommonFlags)
		if !inList(expected, actual) {
			t.Errorf("missing aidl includes in global flags, expected %q, actual %q", expected, actual)
		}
	})

	t.Run("filegroup", func(t *testing.T) {
		ctx := testCc(t, `
		filegroup {
			name: "fg",
			srcs: ["sub/c.aidl"],
			path: "sub",
		}

		cc_library_shared {
			name: "libfoo",
			srcs: [
				"foo.c",
				":fg",
			],
		}`)

		aidl := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_shared").Rule("aidl")
		aidlManifest := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_shared").Output("aidl.sbox.textproto")
		libfoo := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_shared").Module().(*Module)

		if !inList("-I"+filepath.Dir(aidl.Output.String()), android.StringsRelativeToTop(ctx.Config(), libfoo.flags.Local.CommonFlags)) {
			t.Errorf("missing aidl includes in global flags")
		}

		aidlCommand := android.RuleBuilderSboxProtoForTests(t, ctx, aidlManifest).Commands[0].GetCommand()
		if !strings.Contains(aidlCommand, "-Isub") {
			t.Errorf("aidl command for c.aidl should contain \"-Isub\", but was %q", aidlCommand)
		}

	})

	t.Run("sysprop", func(t *testing.T) {
		ctx := testCc(t, `
		cc_library {
			name: "libsysprop",
			srcs: [
				"path/to/foo.sysprop",
			],
		}`)

		outDir := "out/soong/.intermediates/libsysprop/android_arm64_armv8-a_static/gen"
		syspropBuildParams := ctx.ModuleForTests("libsysprop", "android_arm64_armv8-a_static").Rule("sysprop")

		android.AssertStringEquals(t, "header output directory does not match", outDir+"/sysprop/include/path/to", syspropBuildParams.Args["headerOutDir"])
		android.AssertStringEquals(t, "public output directory does not match", outDir+"/sysprop/public/include/path/to", syspropBuildParams.Args["publicOutDir"])
		android.AssertStringEquals(t, "src output directory does not match", outDir+"/sysprop/path/to", syspropBuildParams.Args["srcOutDir"])
		android.AssertStringEquals(t, "output include name does not match", "path/to/foo.sysprop.h", syspropBuildParams.Args["includeName"])
		android.AssertStringEquals(t, "Input file does not match", "path/to/foo.sysprop", syspropBuildParams.Input.String())
		android.AssertStringEquals(t, "Output file does not match", outDir+"/sysprop/path/to/foo.sysprop.cpp", syspropBuildParams.Output.String())
		android.AssertStringListContains(t, "Implicit outputs does not contain header file", syspropBuildParams.ImplicitOutputs.Strings(), outDir+"/sysprop/include/path/to/foo.sysprop.h")
		android.AssertStringListContains(t, "Implicit outputs does not contain public header file", syspropBuildParams.ImplicitOutputs.Strings(), outDir+"/sysprop/public/include/path/to/foo.sysprop.h")
		android.AssertIntEquals(t, "Implicit outputs contains the incorrect number of elements", 2, len(syspropBuildParams.ImplicitOutputs.Strings()))
	})
}
