// Copyright 2020 Google Inc. All rights reserved.
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
	"testing"

	"android/soong/android"
)

func TestSdkMutator(t *testing.T) {
	bp := `
		cc_library {
			name: "libsdk",
			shared_libs: ["libsdkdep"],
			sdk_version: "current",
			stl: "c++_shared",
		}

		cc_library {
			name: "libsdkdep",
			sdk_version: "current",
			stl: "c++_shared",
		}

		cc_library {
			name: "libplatform",
			shared_libs: ["libsdk"],
			stl: "libc++",
		}

		cc_binary {
			name: "platformbinary",
			shared_libs: ["libplatform"],
			stl: "libc++",
		}

		cc_binary {
			name: "sdkbinary",
			shared_libs: ["libsdk"],
			sdk_version: "current",
			stl: "libc++",
		}
	`

	assertDep := func(t *testing.T, from, to android.TestingModule) {
		t.Helper()
		found := false

		var toFile android.Path
		m := to.Module().(*Module)
		if toc := m.Toc(); toc.Valid() {
			toFile = toc.Path()
		} else {
			toFile = m.outputFile.Path()
		}
		toFile = toFile.RelativeToTop()

		rule := from.Description("link")
		for _, dep := range rule.Implicits {
			if dep.String() == toFile.String() {
				found = true
			}
		}
		if !found {
			t.Errorf("expected %q in %q", toFile.String(), rule.Implicits.Strings())
		}
	}

	ctx := testCc(t, bp)

	libsdkNDK := ctx.ModuleForTests("libsdk", "android_arm64_armv8-a_sdk_shared")
	libsdkPlatform := ctx.ModuleForTests("libsdk", "android_arm64_armv8-a_shared")
	libsdkdepNDK := ctx.ModuleForTests("libsdkdep", "android_arm64_armv8-a_sdk_shared")
	libsdkdepPlatform := ctx.ModuleForTests("libsdkdep", "android_arm64_armv8-a_shared")
	libplatform := ctx.ModuleForTests("libplatform", "android_arm64_armv8-a_shared")
	platformbinary := ctx.ModuleForTests("platformbinary", "android_arm64_armv8-a")
	sdkbinary := ctx.ModuleForTests("sdkbinary", "android_arm64_armv8-a_sdk")

	libcxxNDK := ctx.ModuleForTests("ndk_libc++_shared", "android_arm64_armv8-a_sdk_shared")
	libcxxPlatform := ctx.ModuleForTests("libc++", "android_arm64_armv8-a_shared")

	assertDep(t, libsdkNDK, libsdkdepNDK)
	assertDep(t, libsdkPlatform, libsdkdepPlatform)
	assertDep(t, libplatform, libsdkPlatform)
	assertDep(t, platformbinary, libplatform)
	assertDep(t, sdkbinary, libsdkNDK)

	assertDep(t, libsdkNDK, libcxxNDK)
	assertDep(t, libsdkPlatform, libcxxPlatform)
}

func TestMakeModuleNameForSdkVariant(t *testing.T) {
	bp := `
		cc_library {
			name: "libfoo",
			srcs: ["main_test.cpp"],
			sdk_version: "current",
			stl: "none",
		}
	`
	platformVariant := "android_arm64_armv8-a_shared"
	sdkVariant := "android_arm64_armv8-a_sdk_shared"
	testCases := []struct {
		name              string
		unbundledApps     []string
		variant           string
		skipInstall       bool // soong skips install
		hideFromMake      bool // no make entry
		makeUninstallable bool // make skips install
		makeModuleName    string
	}{
		{
			name:          "platform variant in normal builds",
			unbundledApps: nil,
			variant:       platformVariant,
			// installable in soong
			skipInstall: false,
			// visiable in Make as "libfoo"
			hideFromMake:   false,
			makeModuleName: "libfoo",
			// installable in Make
			makeUninstallable: false,
		},
		{
			name:          "sdk variant in normal builds",
			unbundledApps: nil,
			variant:       sdkVariant,
			// soong doesn't install
			skipInstall: true,
			// visible in Make as "libfoo.sdk"
			hideFromMake:   false,
			makeModuleName: "libfoo.sdk",
			// but not installed
			makeUninstallable: true,
		},
		{
			name:          "platform variant in unbunded builds",
			unbundledApps: []string{"bar"},
			variant:       platformVariant,
			// installable in soong
			skipInstall: false,
			// hidden from make
			hideFromMake: true,
		},
		{
			name:          "sdk variant in unbunded builds",
			unbundledApps: []string{"bar"},
			variant:       sdkVariant,
			// soong doesn't install
			skipInstall: true,
			// visible in Make as "libfoo"
			hideFromMake:   false,
			makeModuleName: "libfoo",
			// but not installed
			makeUninstallable: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := android.GroupFixturePreparers(prepareForCcTest,
				android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
					variables.Unbundled_build_apps = tc.unbundledApps
				}),
			)
			ctx := fixture.RunTestWithBp(t, bp).TestContext
			module := ctx.ModuleForTests("libfoo", tc.variant).Module().(*Module)
			android.AssertBoolEquals(t, "IsSkipInstall", tc.skipInstall, module.IsSkipInstall())
			android.AssertBoolEquals(t, "HideFromMake", tc.hideFromMake, module.HiddenFromMake())
			if !tc.hideFromMake {
				entries := android.AndroidMkEntriesForTest(t, ctx, module)[0]
				android.AssertStringEquals(t, "LOCAL_MODULE",
					tc.makeModuleName, entries.EntryMap["LOCAL_MODULE"][0])
				actualUninstallable := false
				if actual, ok := entries.EntryMap["LOCAL_UNINSTALLABLE_MODULE"]; ok {
					actualUninstallable = "true" == actual[0]
				}
				android.AssertBoolEquals(t, "LOCAL_UNINSTALLABLE_MODULE",
					tc.makeUninstallable, actualUninstallable)
			}
		})
	}
}
