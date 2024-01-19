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
	"fmt"
	"testing"

	"android/soong/android"
)

func TestMinSdkVersionsOfCrtObjects(t *testing.T) {
	bp := `
		cc_object {
			name: "crt_foo",
			srcs: ["foo.c"],
			crt: true,
			stl: "none",
			min_sdk_version: "28",
			vendor_available: true,
		}
	`
	variants := []struct {
		variant string
		num     string
	}{
		{"android_arm64_armv8-a", "10000"},
		{"android_arm64_armv8-a_sdk_28", "28"},
		{"android_arm64_armv8-a_sdk_29", "29"},
		{"android_arm64_armv8-a_sdk_30", "30"},
		{"android_arm64_armv8-a_sdk_current", "10000"},
		{"android_vendor.29_arm64_armv8-a", "29"},
	}

	ctx := prepareForCcTest.RunTestWithBp(t, bp)
	for _, v := range variants {
		cflags := ctx.ModuleForTests("crt_foo", v.variant).Rule("cc").Args["cFlags"]
		expected := "-target aarch64-linux-android" + v.num + " "
		android.AssertStringDoesContain(t, "cflag", cflags, expected)
	}
	ctx = prepareForCcTestWithoutVndk.RunTestWithBp(t, bp)
	android.AssertStringDoesContain(t, "cflag",
		ctx.ModuleForTests("crt_foo", "android_vendor_arm64_armv8-a").Rule("cc").Args["cFlags"],
		"-target aarch64-linux-android10000 ")
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
		"29/crtbegin_dynamic.o")

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

func TestCcObjectOutputFile(t *testing.T) {
	testcases := []struct {
		name       string
		moduleName string
		bp         string
	}{
		{
			name:       "normal",
			moduleName: "foo",
			bp: `
				srcs: ["bar.c"],
			`,
		},
		{
			name:       "suffix",
			moduleName: "foo.o",
			bp: `
				srcs: ["bar.c"],
			`,
		},
		{
			name:       "keep symbols",
			moduleName: "foo",
			bp: `
				srcs: ["bar.c"],
				prefix_symbols: "foo_",
			`,
		},
		{
			name:       "partial linking",
			moduleName: "foo",
			bp: `
				srcs: ["bar.c", "baz.c"],
			`,
		},
		{
			name:       "partial linking and prefix symbols",
			moduleName: "foo",
			bp: `
				srcs: ["bar.c", "baz.c"],
				prefix_symbols: "foo_",
			`,
		},
	}

	for _, testcase := range testcases {
		bp := fmt.Sprintf(`
			cc_object {
				name: "%s",
				%s
			}
		`, testcase.moduleName, testcase.bp)
		t.Run(testcase.name, func(t *testing.T) {
			ctx := PrepareForIntegrationTestWithCc.RunTestWithBp(t, bp)
			android.AssertPathRelativeToTopEquals(t, "expected output file foo.o",
				fmt.Sprintf("out/soong/.intermediates/%s/android_arm64_armv8-a/foo.o", testcase.moduleName),
				ctx.ModuleForTests(testcase.moduleName, "android_arm64_armv8-a").Output("foo.o").Output)
		})
	}

}
