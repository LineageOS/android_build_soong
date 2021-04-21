// Copyright 2019 Google Inc. All rights reserved.
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
	"android/soong/android"
	"testing"
)

func TestMinSdkVersionsOfCrtObjects(t *testing.T) {
	ctx := testCc(t, `
		cc_object {
			name: "crt_foo",
			srcs: ["foo.c"],
			crt: true,
			stl: "none",
			min_sdk_version: "28",

		}`)

	arch := "android_arm64_armv8-a"
	for _, v := range []string{"", "28", "29", "30", "current"} {
		var variant string
		// platform variant
		if v == "" {
			variant = arch
		} else {
			variant = arch + "_sdk_" + v
		}
		cflags := ctx.ModuleForTests("crt_foo", variant).Rule("cc").Args["cFlags"]
		vNum := v
		if v == "current" || v == "" {
			vNum = "10000"
		}
		expected := "-target aarch64-linux-android" + vNum + " "
		android.AssertStringDoesContain(t, "cflag", cflags, expected)
	}
}

func TestUseCrtObjectOfCorrectVersion(t *testing.T) {
	ctx := testCc(t, `
		cc_binary {
			name: "bin",
			srcs: ["foo.c"],
			stl: "none",
			min_sdk_version: "29",
			sdk_version: "current",
		}
		`)

	// Sdk variant uses the crt object of the matching min_sdk_version
	variant := "android_arm64_armv8-a_sdk"
	crt := ctx.ModuleForTests("bin", variant).Rule("ld").Args["crtBegin"]
	android.AssertStringDoesContain(t, "crt dep of sdk variant", crt,
		variant+"_29/crtbegin_dynamic.o")

	// platform variant uses the crt object built for platform
	variant = "android_arm64_armv8-a"
	crt = ctx.ModuleForTests("bin", variant).Rule("ld").Args["crtBegin"]
	android.AssertStringDoesContain(t, "crt dep of platform variant", crt,
		variant+"/crtbegin_dynamic.o")
}

func TestLinkerScript(t *testing.T) {
	t.Run("script", func(t *testing.T) {
		testCc(t, `
		cc_object {
			name: "foo",
			srcs: ["baz.o"],
			linker_script: "foo.lds",
		}`)
	})
}

func TestCcObjectWithBazel(t *testing.T) {
	bp := `
cc_object {
	name: "foo",
	srcs: ["baz.o"],
	bazel_module: { label: "//foo/bar:bar" },
}`
	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)
	config.BazelContext = android.MockBazelContext{
		OutputBaseDir: "outputbase",
		LabelToOutputFiles: map[string][]string{
			"//foo/bar:bar": []string{"bazel_out.o"}}}
	ctx := testCcWithConfig(t, config)

	module := ctx.ModuleForTests("foo", "android_arm_armv7-a-neon").Module()
	outputFiles, err := module.(android.OutputFileProducer).OutputFiles("")
	if err != nil {
		t.Errorf("Unexpected error getting cc_object outputfiles %s", err)
	}

	expectedOutputFiles := []string{"outputbase/execroot/__main__/bazel_out.o"}
	android.AssertDeepEquals(t, "output files", expectedOutputFiles, outputFiles.Strings())
}
