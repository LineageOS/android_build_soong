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

package java

import (
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
)

var (
	resourceFiles = []string{
		"res/layout/layout.xml",
		"res/values/strings.xml",
		"res/values-en-rUS/strings.xml",
	}

	compiledResourceFiles = []string{
		"aapt2/res/layout_layout.xml.flat",
		"aapt2/res/values_strings.arsc.flat",
		"aapt2/res/values-en-rUS_strings.arsc.flat",
	}
)

func testAppConfig(env map[string]string, bp string, fs map[string][]byte) android.Config {
	appFS := map[string][]byte{}
	for k, v := range fs {
		appFS[k] = v
	}

	for _, file := range resourceFiles {
		appFS[file] = nil
	}

	return testConfig(env, bp, appFS)
}

func testApp(t *testing.T, bp string) *android.TestContext {
	config := testAppConfig(nil, bp, nil)

	ctx := testContext()

	run(t, ctx, config)

	return ctx
}

func TestApp(t *testing.T) {
	for _, moduleType := range []string{"android_app", "android_library"} {
		t.Run(moduleType, func(t *testing.T) {
			ctx := testApp(t, moduleType+` {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "current"
				}
			`)

			foo := ctx.ModuleForTests("foo", "android_common")

			var expectedLinkImplicits []string

			manifestFixer := foo.Output("manifest_fixer/AndroidManifest.xml")
			expectedLinkImplicits = append(expectedLinkImplicits, manifestFixer.Output.String())

			frameworkRes := ctx.ModuleForTests("framework-res", "android_common")
			expectedLinkImplicits = append(expectedLinkImplicits,
				frameworkRes.Output("package-res.apk").Output.String())

			// Test the mapping from input files to compiled output file names
			compile := foo.Output(compiledResourceFiles[0])
			if !reflect.DeepEqual(resourceFiles, compile.Inputs.Strings()) {
				t.Errorf("expected aapt2 compile inputs expected:\n  %#v\n got:\n  %#v",
					resourceFiles, compile.Inputs.Strings())
			}

			compiledResourceOutputs := compile.Outputs.Strings()
			sort.Strings(compiledResourceOutputs)

			expectedLinkImplicits = append(expectedLinkImplicits, compiledResourceOutputs...)

			list := foo.Output("aapt2/res.list")
			expectedLinkImplicits = append(expectedLinkImplicits, list.Output.String())

			// Check that the link rule uses
			res := ctx.ModuleForTests("foo", "android_common").Output("package-res.apk")
			if !reflect.DeepEqual(expectedLinkImplicits, res.Implicits.Strings()) {
				t.Errorf("expected aapt2 link implicits expected:\n  %#v\n got:\n  %#v",
					expectedLinkImplicits, res.Implicits.Strings())
			}
		})
	}
}

func TestAppSplits(t *testing.T) {
	ctx := testApp(t, `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					package_splits: ["v4", "v7,hdpi"],
					sdk_version: "current"
				}`)

	foo := ctx.ModuleForTests("foo", "android_common")

	expectedOutputs := []string{
		filepath.Join(buildDir, ".intermediates/foo/android_common/foo.apk"),
		filepath.Join(buildDir, ".intermediates/foo/android_common/foo_v4.apk"),
		filepath.Join(buildDir, ".intermediates/foo/android_common/foo_v7_hdpi.apk"),
	}
	for _, expectedOutput := range expectedOutputs {
		foo.Output(expectedOutput)
	}

	outputFiles, err := foo.Module().(*AndroidApp).OutputFiles("")
	if err != nil {
		t.Fatal(err)
	}
	if g, w := outputFiles.Strings(), expectedOutputs; !reflect.DeepEqual(g, w) {
		t.Errorf(`want OutputFiles("") = %q, got %q`, w, g)
	}
}

func TestAndroidAppSet(t *testing.T) {
	ctx, config := testJava(t, `
		android_app_set {
			name: "foo",
			set: "prebuilts/apks/app.apks",
			prerelease: true,
		}`)
	module := ctx.ModuleForTests("foo", "android_common")
	const packedSplitApks = "foo.zip"
	params := module.Output(packedSplitApks)
	if params.Rule == nil {
		t.Errorf("expected output %s is missing", packedSplitApks)
	}
	if s := params.Args["allow-prereleased"]; s != "true" {
		t.Errorf("wrong allow-prereleased value: '%s', expected 'true'", s)
	}
	if s := params.Args["partition"]; s != "system" {
		t.Errorf("wrong partition value: '%s', expected 'system'", s)
	}
	mkEntries := android.AndroidMkEntriesForTest(t, config, "", module.Module())[0]
	actualMaster := mkEntries.EntryMap["LOCAL_APK_SET_MASTER_FILE"]
	expectedMaster := []string{"foo.apk"}
	if !reflect.DeepEqual(actualMaster, expectedMaster) {
		t.Errorf("Unexpected LOCAL_APK_SET_MASTER_FILE value: '%s', expected: '%s',",
			actualMaster, expectedMaster)
	}
}

func TestAndroidAppSet_Variants(t *testing.T) {
	bp := `
		android_app_set {
			name: "foo",
			set: "prebuilts/apks/app.apks",
		}`
	testCases := []struct {
		name            string
		targets         []android.Target
		aaptPrebuiltDPI []string
		sdkVersion      int
		expected        map[string]string
	}{
		{
			name: "One",
			targets: []android.Target{
				{Os: android.Android, Arch: android.Arch{ArchType: android.X86}},
			},
			aaptPrebuiltDPI: []string{"ldpi", "xxhdpi"},
			sdkVersion:      29,
			expected: map[string]string{
				"abis":              "X86",
				"allow-prereleased": "false",
				"screen-densities":  "LDPI,XXHDPI",
				"sdk-version":       "29",
				"stem":              "foo",
			},
		},
		{
			name: "Two",
			targets: []android.Target{
				{Os: android.Android, Arch: android.Arch{ArchType: android.X86_64}},
				{Os: android.Android, Arch: android.Arch{ArchType: android.X86}},
			},
			aaptPrebuiltDPI: nil,
			sdkVersion:      30,
			expected: map[string]string{
				"abis":              "X86_64,X86",
				"allow-prereleased": "false",
				"screen-densities":  "all",
				"sdk-version":       "30",
				"stem":              "foo",
			},
		},
	}

	for _, test := range testCases {
		config := testAppConfig(nil, bp, nil)
		config.TestProductVariables.AAPTPrebuiltDPI = test.aaptPrebuiltDPI
		config.TestProductVariables.Platform_sdk_version = &test.sdkVersion
		config.Targets[android.Android] = test.targets
		ctx := testContext()
		run(t, ctx, config)
		module := ctx.ModuleForTests("foo", "android_common")
		const packedSplitApks = "foo.zip"
		params := module.Output(packedSplitApks)
		for k, v := range test.expected {
			if actual := params.Args[k]; actual != v {
				t.Errorf("%s: bad build arg value for '%s': '%s', expected '%s'",
					test.name, k, actual, v)
			}
		}
	}
}

func TestPlatformAPIs(t *testing.T) {
	testJava(t, `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			platform_apis: true,
		}
	`)

	testJava(t, `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			sdk_version: "current",
		}
	`)

	testJavaError(t, "platform_apis must be true when sdk_version is empty.", `
		android_app {
			name: "bar",
			srcs: ["b.java"],
		}
	`)

	testJavaError(t, "platform_apis must be false when sdk_version is not empty.", `
		android_app {
			name: "bar",
			srcs: ["b.java"],
			sdk_version: "system_current",
			platform_apis: true,
		}
	`)
}

func TestAndroidAppLinkType(t *testing.T) {
	testJava(t, `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			libs: ["bar"],
			static_libs: ["baz"],
			platform_apis: true,
		}

		java_library {
			name: "bar",
			sdk_version: "current",
			srcs: ["b.java"],
		}

		android_library {
			name: "baz",
			sdk_version: "system_current",
			srcs: ["c.java"],
		}
	`)

	testJavaError(t, "Adjust sdk_version: property of the source or target module so that target module is built with the same or smaller API set than the source.", `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			libs: ["bar"],
			sdk_version: "current",
			static_libs: ["baz"],
		}

		java_library {
			name: "bar",
			sdk_version: "current",
			srcs: ["b.java"],
		}

		android_library {
			name: "baz",
			sdk_version: "system_current",
			srcs: ["c.java"],
		}
	`)

	testJava(t, `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			libs: ["bar"],
			sdk_version: "system_current",
			static_libs: ["baz"],
		}

		java_library {
			name: "bar",
			sdk_version: "current",
			srcs: ["b.java"],
		}

		android_library {
			name: "baz",
			sdk_version: "system_current",
			srcs: ["c.java"],
		}
	`)

	testJavaError(t, "Adjust sdk_version: property of the source or target module so that target module is built with the same or smaller API set than the source.", `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			libs: ["bar"],
			sdk_version: "system_current",
			static_libs: ["baz"],
		}

		java_library {
			name: "bar",
			sdk_version: "current",
			srcs: ["b.java"],
		}

		android_library {
			name: "baz",
			srcs: ["c.java"],
		}
	`)
}

func TestUpdatableApps(t *testing.T) {
	testCases := []struct {
		name          string
		bp            string
		expectedError string
	}{
		{
			name: "Stable public SDK",
			bp: `android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "29",
					min_sdk_version: "29",
					updatable: true,
				}`,
		},
		{
			name: "Stable system SDK",
			bp: `android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "system_29",
					min_sdk_version: "29",
					updatable: true,
				}`,
		},
		{
			name: "Current public SDK",
			bp: `android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "current",
					min_sdk_version: "29",
					updatable: true,
				}`,
		},
		{
			name: "Current system SDK",
			bp: `android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "system_current",
					min_sdk_version: "29",
					updatable: true,
				}`,
		},
		{
			name: "Current module SDK",
			bp: `android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "module_current",
					min_sdk_version: "29",
					updatable: true,
				}`,
		},
		{
			name: "Current core SDK",
			bp: `android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "core_current",
					min_sdk_version: "29",
					updatable: true,
				}`,
		},
		{
			name: "No Platform APIs",
			bp: `android_app {
					name: "foo",
					srcs: ["a.java"],
					platform_apis: true,
					min_sdk_version: "29",
					updatable: true,
				}`,
			expectedError: "Updatable apps must use stable SDKs",
		},
		{
			name: "No Core Platform APIs",
			bp: `android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "core_platform",
					min_sdk_version: "29",
					updatable: true,
				}`,
			expectedError: "Updatable apps must use stable SDKs",
		},
		{
			name: "No unspecified APIs",
			bp: `android_app {
					name: "foo",
					srcs: ["a.java"],
					updatable: true,
					min_sdk_version: "29",
				}`,
			expectedError: "Updatable apps must use stable SDK",
		},
		{
			name: "Must specify min_sdk_version",
			bp: `android_app {
					name: "app_without_min_sdk_version",
					srcs: ["a.java"],
					sdk_version: "29",
					updatable: true,
				}`,
			expectedError: "updatable apps must set min_sdk_version.",
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			if test.expectedError == "" {
				testJava(t, test.bp)
			} else {
				testJavaError(t, test.expectedError, test.bp)
			}
		})
	}
}

func TestUpdatableApps_JniLibsShouldShouldSupportMinSdkVersion(t *testing.T) {
	testJava(t, cc.GatherRequiredDepsForTest(android.Android)+`
		android_app {
			name: "foo",
			srcs: ["a.java"],
			updatable: true,
			sdk_version: "current",
			min_sdk_version: "current",
			jni_libs: ["libjni"],
		}

		cc_library {
			name: "libjni",
			stl: "none",
			system_shared_libs: [],
			sdk_version: "current",
		}
	`)
}

func TestUpdatableApps_JniLibShouldBeBuiltAgainstMinSdkVersion(t *testing.T) {
	bp := cc.GatherRequiredDepsForTest(android.Android) + `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			updatable: true,
			sdk_version: "current",
			min_sdk_version: "29",
			jni_libs: ["libjni"],
		}

		cc_library {
			name: "libjni",
			stl: "none",
			system_shared_libs: [],
			sdk_version: "29",
		}

		ndk_prebuilt_object {
			name: "ndk_crtbegin_so.29",
			sdk_version: "29",
		}

		ndk_prebuilt_object {
			name: "ndk_crtend_so.29",
			sdk_version: "29",
		}
	`
	fs := map[string][]byte{
		"prebuilts/ndk/current/platforms/android-29/arch-arm64/usr/lib/crtbegin_so.o": nil,
		"prebuilts/ndk/current/platforms/android-29/arch-arm64/usr/lib/crtend_so.o":   nil,
		"prebuilts/ndk/current/platforms/android-29/arch-arm/usr/lib/crtbegin_so.o":   nil,
		"prebuilts/ndk/current/platforms/android-29/arch-arm/usr/lib/crtend_so.o":     nil,
	}

	ctx, _ := testJavaWithConfig(t, testConfig(nil, bp, fs))

	inputs := ctx.ModuleForTests("libjni", "android_arm64_armv8-a_sdk_shared").Description("link").Implicits
	var crtbeginFound, crtendFound bool
	for _, input := range inputs {
		switch input.String() {
		case "prebuilts/ndk/current/platforms/android-29/arch-arm64/usr/lib/crtbegin_so.o":
			crtbeginFound = true
		case "prebuilts/ndk/current/platforms/android-29/arch-arm64/usr/lib/crtend_so.o":
			crtendFound = true
		}
	}
	if !crtbeginFound || !crtendFound {
		t.Error("should link with ndk_crtbegin_so.29 and ndk_crtend_so.29")
	}
}

func TestUpdatableApps_ErrorIfJniLibDoesntSupportMinSdkVersion(t *testing.T) {
	bp := cc.GatherRequiredDepsForTest(android.Android) + `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			updatable: true,
			sdk_version: "current",
			min_sdk_version: "29",  // this APK should support 29
			jni_libs: ["libjni"],
		}

		cc_library {
			name: "libjni",
			stl: "none",
			sdk_version: "current",
		}
	`
	testJavaError(t, `"libjni" .*: sdk_version\(current\) is higher than min_sdk_version\(29\)`, bp)
}

func TestUpdatableApps_ErrorIfDepSdkVersionIsHigher(t *testing.T) {
	bp := cc.GatherRequiredDepsForTest(android.Android) + `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			updatable: true,
			sdk_version: "current",
			min_sdk_version: "29",  // this APK should support 29
			jni_libs: ["libjni"],
		}

		cc_library {
			name: "libjni",
			stl: "none",
			shared_libs: ["libbar"],
			system_shared_libs: [],
			sdk_version: "27",
		}

		cc_library {
			name: "libbar",
			stl: "none",
			system_shared_libs: [],
			sdk_version: "current",
		}
	`
	testJavaError(t, `"libjni" .*: links "libbar" built against newer API version "current"`, bp)
}

func TestResourceDirs(t *testing.T) {
	testCases := []struct {
		name      string
		prop      string
		resources []string
	}{
		{
			name:      "no resource_dirs",
			prop:      "",
			resources: []string{"res/res/values/strings.xml"},
		},
		{
			name:      "resource_dirs",
			prop:      `resource_dirs: ["res"]`,
			resources: []string{"res/res/values/strings.xml"},
		},
		{
			name:      "empty resource_dirs",
			prop:      `resource_dirs: []`,
			resources: nil,
		},
	}

	fs := map[string][]byte{
		"res/res/values/strings.xml": nil,
	}

	bp := `
			android_app {
				name: "foo",
				sdk_version: "current",
				%s
			}
		`

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			config := testConfig(nil, fmt.Sprintf(bp, testCase.prop), fs)
			ctx := testContext()
			run(t, ctx, config)

			module := ctx.ModuleForTests("foo", "android_common")
			resourceList := module.MaybeOutput("aapt2/res.list")

			var resources []string
			if resourceList.Rule != nil {
				for _, compiledResource := range resourceList.Inputs.Strings() {
					resources = append(resources, module.Output(compiledResource).Inputs.Strings()...)
				}
			}

			if !reflect.DeepEqual(resources, testCase.resources) {
				t.Errorf("expected resource files %q, got %q",
					testCase.resources, resources)
			}
		})
	}
}

func TestLibraryAssets(t *testing.T) {
	bp := `
			android_app {
				name: "foo",
				sdk_version: "current",
				static_libs: ["lib1", "lib2", "lib3"],
			}

			android_library {
				name: "lib1",
				sdk_version: "current",
				asset_dirs: ["assets_a"],
			}

			android_library {
				name: "lib2",
				sdk_version: "current",
			}

			android_library {
				name: "lib3",
				sdk_version: "current",
				static_libs: ["lib4"],
			}

			android_library {
				name: "lib4",
				sdk_version: "current",
				asset_dirs: ["assets_b"],
			}
		`

	testCases := []struct {
		name          string
		assetFlag     string
		assetPackages []string
	}{
		{
			name: "foo",
			// lib1 has its own asset. lib3 doesn't have any, but provides lib4's transitively.
			assetPackages: []string{
				buildDir + "/.intermediates/foo/android_common/aapt2/package-res.apk",
				buildDir + "/.intermediates/lib1/android_common/assets.zip",
				buildDir + "/.intermediates/lib3/android_common/assets.zip",
			},
		},
		{
			name:      "lib1",
			assetFlag: "-A assets_a",
		},
		{
			name: "lib2",
		},
		{
			name: "lib3",
			assetPackages: []string{
				buildDir + "/.intermediates/lib3/android_common/aapt2/package-res.apk",
				buildDir + "/.intermediates/lib4/android_common/assets.zip",
			},
		},
		{
			name:      "lib4",
			assetFlag: "-A assets_b",
		},
	}
	ctx := testApp(t, bp)

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			m := ctx.ModuleForTests(test.name, "android_common")

			// Check asset flag in aapt2 link flags
			var aapt2link android.TestingBuildParams
			if len(test.assetPackages) > 0 {
				aapt2link = m.Output("aapt2/package-res.apk")
			} else {
				aapt2link = m.Output("package-res.apk")
			}
			aapt2Flags := aapt2link.Args["flags"]
			if test.assetFlag != "" {
				if !strings.Contains(aapt2Flags, test.assetFlag) {
					t.Errorf("Can't find asset flag %q in aapt2 link flags %q", test.assetFlag, aapt2Flags)
				}
			} else {
				if strings.Contains(aapt2Flags, " -A ") {
					t.Errorf("aapt2 link flags %q contain unexpected asset flag", aapt2Flags)
				}
			}

			// Check asset merge rule.
			if len(test.assetPackages) > 0 {
				mergeAssets := m.Output("package-res.apk")
				if !reflect.DeepEqual(test.assetPackages, mergeAssets.Inputs.Strings()) {
					t.Errorf("Unexpected mergeAssets inputs: %v, expected: %v",
						mergeAssets.Inputs.Strings(), test.assetPackages)
				}
			}
		})
	}
}

func TestAndroidResources(t *testing.T) {
	testCases := []struct {
		name                       string
		enforceRROTargets          []string
		enforceRROExcludedOverlays []string
		resourceFiles              map[string][]string
		overlayFiles               map[string][]string
		rroDirs                    map[string][]string
	}{
		{
			name:                       "no RRO",
			enforceRROTargets:          nil,
			enforceRROExcludedOverlays: nil,
			resourceFiles: map[string][]string{
				"foo":  nil,
				"bar":  {"bar/res/res/values/strings.xml"},
				"lib":  nil,
				"lib2": {"lib2/res/res/values/strings.xml"},
			},
			overlayFiles: map[string][]string{
				"foo": {
					buildDir + "/.intermediates/lib2/android_common/package-res.apk",
					buildDir + "/.intermediates/lib/android_common/package-res.apk",
					buildDir + "/.intermediates/lib3/android_common/package-res.apk",
					"foo/res/res/values/strings.xml",
					"device/vendor/blah/static_overlay/foo/res/values/strings.xml",
					"device/vendor/blah/overlay/foo/res/values/strings.xml",
					"product/vendor/blah/overlay/foo/res/values/strings.xml",
				},
				"bar": {
					"device/vendor/blah/static_overlay/bar/res/values/strings.xml",
					"device/vendor/blah/overlay/bar/res/values/strings.xml",
				},
				"lib": {
					buildDir + "/.intermediates/lib2/android_common/package-res.apk",
					"lib/res/res/values/strings.xml",
					"device/vendor/blah/overlay/lib/res/values/strings.xml",
				},
			},
			rroDirs: map[string][]string{
				"foo": nil,
				"bar": nil,
			},
		},
		{
			name:                       "enforce RRO on foo",
			enforceRROTargets:          []string{"foo"},
			enforceRROExcludedOverlays: []string{"device/vendor/blah/static_overlay"},
			resourceFiles: map[string][]string{
				"foo":  nil,
				"bar":  {"bar/res/res/values/strings.xml"},
				"lib":  nil,
				"lib2": {"lib2/res/res/values/strings.xml"},
			},
			overlayFiles: map[string][]string{
				"foo": {
					buildDir + "/.intermediates/lib2/android_common/package-res.apk",
					buildDir + "/.intermediates/lib/android_common/package-res.apk",
					buildDir + "/.intermediates/lib3/android_common/package-res.apk",
					"foo/res/res/values/strings.xml",
					"device/vendor/blah/static_overlay/foo/res/values/strings.xml",
				},
				"bar": {
					"device/vendor/blah/static_overlay/bar/res/values/strings.xml",
					"device/vendor/blah/overlay/bar/res/values/strings.xml",
				},
				"lib": {
					buildDir + "/.intermediates/lib2/android_common/package-res.apk",
					"lib/res/res/values/strings.xml",
					"device/vendor/blah/overlay/lib/res/values/strings.xml",
				},
			},

			rroDirs: map[string][]string{
				"foo": {
					"device:device/vendor/blah/overlay/foo/res",
					// Enforce RRO on "foo" could imply RRO on static dependencies, but for now it doesn't.
					// "device/vendor/blah/overlay/lib/res",
					"product:product/vendor/blah/overlay/foo/res",
				},
				"bar": nil,
				"lib": nil,
			},
		},
		{
			name:              "enforce RRO on all",
			enforceRROTargets: []string{"*"},
			enforceRROExcludedOverlays: []string{
				// Excluding specific apps/res directories also allowed.
				"device/vendor/blah/static_overlay/foo",
				"device/vendor/blah/static_overlay/bar/res",
			},
			resourceFiles: map[string][]string{
				"foo":  nil,
				"bar":  {"bar/res/res/values/strings.xml"},
				"lib":  nil,
				"lib2": {"lib2/res/res/values/strings.xml"},
			},
			overlayFiles: map[string][]string{
				"foo": {
					buildDir + "/.intermediates/lib2/android_common/package-res.apk",
					buildDir + "/.intermediates/lib/android_common/package-res.apk",
					buildDir + "/.intermediates/lib3/android_common/package-res.apk",
					"foo/res/res/values/strings.xml",
					"device/vendor/blah/static_overlay/foo/res/values/strings.xml",
				},
				"bar": {"device/vendor/blah/static_overlay/bar/res/values/strings.xml"},
				"lib": {
					buildDir + "/.intermediates/lib2/android_common/package-res.apk",
					"lib/res/res/values/strings.xml",
				},
			},
			rroDirs: map[string][]string{
				"foo": {
					"device:device/vendor/blah/overlay/foo/res",
					"product:product/vendor/blah/overlay/foo/res",
					// Lib dep comes after the direct deps
					"device:device/vendor/blah/overlay/lib/res",
				},
				"bar": {"device:device/vendor/blah/overlay/bar/res"},
				"lib": {"device:device/vendor/blah/overlay/lib/res"},
			},
		},
	}

	deviceResourceOverlays := []string{
		"device/vendor/blah/overlay",
		"device/vendor/blah/overlay2",
		"device/vendor/blah/static_overlay",
	}

	productResourceOverlays := []string{
		"product/vendor/blah/overlay",
	}

	fs := map[string][]byte{
		"foo/res/res/values/strings.xml":                               nil,
		"bar/res/res/values/strings.xml":                               nil,
		"lib/res/res/values/strings.xml":                               nil,
		"lib2/res/res/values/strings.xml":                              nil,
		"device/vendor/blah/overlay/foo/res/values/strings.xml":        nil,
		"device/vendor/blah/overlay/bar/res/values/strings.xml":        nil,
		"device/vendor/blah/overlay/lib/res/values/strings.xml":        nil,
		"device/vendor/blah/static_overlay/foo/res/values/strings.xml": nil,
		"device/vendor/blah/static_overlay/bar/res/values/strings.xml": nil,
		"device/vendor/blah/overlay2/res/values/strings.xml":           nil,
		"product/vendor/blah/overlay/foo/res/values/strings.xml":       nil,
	}

	bp := `
			android_app {
				name: "foo",
				sdk_version: "current",
				resource_dirs: ["foo/res"],
				static_libs: ["lib", "lib3"],
			}

			android_app {
				name: "bar",
				sdk_version: "current",
				resource_dirs: ["bar/res"],
			}

			android_library {
				name: "lib",
				sdk_version: "current",
				resource_dirs: ["lib/res"],
				static_libs: ["lib2"],
			}

			android_library {
				name: "lib2",
				sdk_version: "current",
				resource_dirs: ["lib2/res"],
			}

			// This library has the same resources as lib (should not lead to dupe RROs)
			android_library {
				name: "lib3",
				sdk_version: "current",
				resource_dirs: ["lib/res"]
			}
		`

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			config := testAppConfig(nil, bp, fs)
			config.TestProductVariables.DeviceResourceOverlays = deviceResourceOverlays
			config.TestProductVariables.ProductResourceOverlays = productResourceOverlays
			if testCase.enforceRROTargets != nil {
				config.TestProductVariables.EnforceRROTargets = testCase.enforceRROTargets
			}
			if testCase.enforceRROExcludedOverlays != nil {
				config.TestProductVariables.EnforceRROExcludedOverlays = testCase.enforceRROExcludedOverlays
			}

			ctx := testContext()
			run(t, ctx, config)

			resourceListToFiles := func(module android.TestingModule, list []string) (files []string) {
				for _, o := range list {
					res := module.MaybeOutput(o)
					if res.Rule != nil {
						// If the overlay is compiled as part of this module (i.e. a .arsc.flat file),
						// verify the inputs to the .arsc.flat rule.
						files = append(files, res.Inputs.Strings()...)
					} else {
						// Otherwise, verify the full path to the output of the other module
						files = append(files, o)
					}
				}
				return files
			}

			getResources := func(moduleName string) (resourceFiles, overlayFiles, rroDirs []string) {
				module := ctx.ModuleForTests(moduleName, "android_common")
				resourceList := module.MaybeOutput("aapt2/res.list")
				if resourceList.Rule != nil {
					resourceFiles = resourceListToFiles(module, resourceList.Inputs.Strings())
				}
				overlayList := module.MaybeOutput("aapt2/overlay.list")
				if overlayList.Rule != nil {
					overlayFiles = resourceListToFiles(module, overlayList.Inputs.Strings())
				}

				for _, d := range module.Module().(AndroidLibraryDependency).ExportedRRODirs() {
					var prefix string
					if d.overlayType == device {
						prefix = "device:"
					} else if d.overlayType == product {
						prefix = "product:"
					} else {
						t.Fatalf("Unexpected overlayType %d", d.overlayType)
					}
					rroDirs = append(rroDirs, prefix+d.path.String())
				}

				return resourceFiles, overlayFiles, rroDirs
			}

			modules := []string{"foo", "bar", "lib", "lib2"}
			for _, module := range modules {
				resourceFiles, overlayFiles, rroDirs := getResources(module)

				if !reflect.DeepEqual(resourceFiles, testCase.resourceFiles[module]) {
					t.Errorf("expected %s resource files:\n  %#v\n got:\n  %#v",
						module, testCase.resourceFiles[module], resourceFiles)
				}
				if !reflect.DeepEqual(overlayFiles, testCase.overlayFiles[module]) {
					t.Errorf("expected %s overlay files:\n  %#v\n got:\n  %#v",
						module, testCase.overlayFiles[module], overlayFiles)
				}
				if !reflect.DeepEqual(rroDirs, testCase.rroDirs[module]) {
					t.Errorf("expected %s rroDirs:  %#v\n got:\n  %#v",
						module, testCase.rroDirs[module], rroDirs)
				}
			}
		})
	}
}

func TestAppSdkVersion(t *testing.T) {
	testCases := []struct {
		name                  string
		sdkVersion            string
		platformSdkInt        int
		platformSdkCodename   string
		platformSdkFinal      bool
		expectedMinSdkVersion string
		platformApis          bool
	}{
		{
			name:                  "current final SDK",
			sdkVersion:            "current",
			platformSdkInt:        27,
			platformSdkCodename:   "REL",
			platformSdkFinal:      true,
			expectedMinSdkVersion: "27",
		},
		{
			name:                  "current non-final SDK",
			sdkVersion:            "current",
			platformSdkInt:        27,
			platformSdkCodename:   "OMR1",
			platformSdkFinal:      false,
			expectedMinSdkVersion: "OMR1",
		},
		{
			name:                  "default final SDK",
			sdkVersion:            "",
			platformApis:          true,
			platformSdkInt:        27,
			platformSdkCodename:   "REL",
			platformSdkFinal:      true,
			expectedMinSdkVersion: "27",
		},
		{
			name:                  "default non-final SDK",
			sdkVersion:            "",
			platformApis:          true,
			platformSdkInt:        27,
			platformSdkCodename:   "OMR1",
			platformSdkFinal:      false,
			expectedMinSdkVersion: "OMR1",
		},
		{
			name:                  "14",
			sdkVersion:            "14",
			expectedMinSdkVersion: "14",
		},
	}

	for _, moduleType := range []string{"android_app", "android_library"} {
		for _, test := range testCases {
			t.Run(moduleType+" "+test.name, func(t *testing.T) {
				platformApiProp := ""
				if test.platformApis {
					platformApiProp = "platform_apis: true,"
				}
				bp := fmt.Sprintf(`%s {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "%s",
					%s
				}`, moduleType, test.sdkVersion, platformApiProp)

				config := testAppConfig(nil, bp, nil)
				config.TestProductVariables.Platform_sdk_version = &test.platformSdkInt
				config.TestProductVariables.Platform_sdk_codename = &test.platformSdkCodename
				config.TestProductVariables.Platform_sdk_final = &test.platformSdkFinal

				ctx := testContext()

				run(t, ctx, config)

				foo := ctx.ModuleForTests("foo", "android_common")
				link := foo.Output("package-res.apk")
				linkFlags := strings.Split(link.Args["flags"], " ")
				min := android.IndexList("--min-sdk-version", linkFlags)
				target := android.IndexList("--target-sdk-version", linkFlags)

				if min == -1 || target == -1 || min == len(linkFlags)-1 || target == len(linkFlags)-1 {
					t.Fatalf("missing --min-sdk-version or --target-sdk-version in link flags: %q", linkFlags)
				}

				gotMinSdkVersion := linkFlags[min+1]
				gotTargetSdkVersion := linkFlags[target+1]

				if gotMinSdkVersion != test.expectedMinSdkVersion {
					t.Errorf("incorrect --min-sdk-version, expected %q got %q",
						test.expectedMinSdkVersion, gotMinSdkVersion)
				}

				if gotTargetSdkVersion != test.expectedMinSdkVersion {
					t.Errorf("incorrect --target-sdk-version, expected %q got %q",
						test.expectedMinSdkVersion, gotTargetSdkVersion)
				}
			})
		}
	}
}

func TestJNIABI(t *testing.T) {
	ctx, _ := testJava(t, cc.GatherRequiredDepsForTest(android.Android)+`
		cc_library {
			name: "libjni",
			system_shared_libs: [],
			sdk_version: "current",
			stl: "none",
		}

		android_test {
			name: "test",
			sdk_version: "core_platform",
			jni_libs: ["libjni"],
		}

		android_test {
			name: "test_first",
			sdk_version: "core_platform",
			compile_multilib: "first",
			jni_libs: ["libjni"],
		}

		android_test {
			name: "test_both",
			sdk_version: "core_platform",
			compile_multilib: "both",
			jni_libs: ["libjni"],
		}

		android_test {
			name: "test_32",
			sdk_version: "core_platform",
			compile_multilib: "32",
			jni_libs: ["libjni"],
		}

		android_test {
			name: "test_64",
			sdk_version: "core_platform",
			compile_multilib: "64",
			jni_libs: ["libjni"],
		}
		`)

	testCases := []struct {
		name string
		abis []string
	}{
		{"test", []string{"arm64-v8a"}},
		{"test_first", []string{"arm64-v8a"}},
		{"test_both", []string{"arm64-v8a", "armeabi-v7a"}},
		{"test_32", []string{"armeabi-v7a"}},
		{"test_64", []string{"arm64-v8a"}},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			app := ctx.ModuleForTests(test.name, "android_common")
			jniLibZip := app.Output("jnilibs.zip")
			var abis []string
			args := strings.Fields(jniLibZip.Args["jarArgs"])
			for i := 0; i < len(args); i++ {
				if args[i] == "-P" {
					abis = append(abis, filepath.Base(args[i+1]))
					i++
				}
			}
			if !reflect.DeepEqual(abis, test.abis) {
				t.Errorf("want abis %v, got %v", test.abis, abis)
			}
		})
	}
}

func TestAppSdkVersionByPartition(t *testing.T) {
	testJavaError(t, "sdk_version must have a value when the module is located at vendor or product", `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			vendor: true,
			platform_apis: true,
		}
	`)

	testJava(t, `
		android_app {
			name: "bar",
			srcs: ["b.java"],
			platform_apis: true,
		}
	`)

	for _, enforce := range []bool{true, false} {
		bp := `
			android_app {
				name: "foo",
				srcs: ["a.java"],
				product_specific: true,
				platform_apis: true,
			}
		`

		config := testAppConfig(nil, bp, nil)
		config.TestProductVariables.EnforceProductPartitionInterface = proptools.BoolPtr(enforce)
		if enforce {
			testJavaErrorWithConfig(t, "sdk_version must have a value when the module is located at vendor or product", config)
		} else {
			testJavaWithConfig(t, config)
		}
	}
}

func TestJNIPackaging(t *testing.T) {
	ctx, _ := testJava(t, cc.GatherRequiredDepsForTest(android.Android)+`
		cc_library {
			name: "libjni",
			system_shared_libs: [],
			stl: "none",
			sdk_version: "current",
		}

		android_app {
			name: "app",
			jni_libs: ["libjni"],
			sdk_version: "current",
		}

		android_app {
			name: "app_noembed",
			jni_libs: ["libjni"],
			use_embedded_native_libs: false,
			sdk_version: "current",
		}

		android_app {
			name: "app_embed",
			jni_libs: ["libjni"],
			use_embedded_native_libs: true,
			sdk_version: "current",
		}

		android_test {
			name: "test",
			sdk_version: "current",
			jni_libs: ["libjni"],
		}

		android_test {
			name: "test_noembed",
			sdk_version: "current",
			jni_libs: ["libjni"],
			use_embedded_native_libs: false,
		}

		android_test_helper_app {
			name: "test_helper",
			sdk_version: "current",
			jni_libs: ["libjni"],
		}

		android_test_helper_app {
			name: "test_helper_noembed",
			sdk_version: "current",
			jni_libs: ["libjni"],
			use_embedded_native_libs: false,
		}
		`)

	testCases := []struct {
		name       string
		packaged   bool
		compressed bool
	}{
		{"app", false, false},
		{"app_noembed", false, false},
		{"app_embed", true, false},
		{"test", true, false},
		{"test_noembed", true, true},
		{"test_helper", true, false},
		{"test_helper_noembed", true, true},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			app := ctx.ModuleForTests(test.name, "android_common")
			jniLibZip := app.MaybeOutput("jnilibs.zip")
			if g, w := (jniLibZip.Rule != nil), test.packaged; g != w {
				t.Errorf("expected jni packaged %v, got %v", w, g)
			}

			if jniLibZip.Rule != nil {
				if g, w := !strings.Contains(jniLibZip.Args["jarArgs"], "-L 0"), test.compressed; g != w {
					t.Errorf("expected jni compressed %v, got %v", w, g)
				}

				if !strings.Contains(jniLibZip.Implicits[0].String(), "_sdk_") {
					t.Errorf("expected input %q to use sdk variant", jniLibZip.Implicits[0].String())
				}
			}
		})
	}
}

func TestJNISDK(t *testing.T) {
	ctx, _ := testJava(t, cc.GatherRequiredDepsForTest(android.Android)+`
		cc_library {
			name: "libjni",
			system_shared_libs: [],
			stl: "none",
			sdk_version: "current",
		}

		android_test {
			name: "app_platform",
			jni_libs: ["libjni"],
			platform_apis: true,
		}

		android_test {
			name: "app_sdk",
			jni_libs: ["libjni"],
			sdk_version: "current",
		}

		android_test {
			name: "app_force_platform",
			jni_libs: ["libjni"],
			sdk_version: "current",
			jni_uses_platform_apis: true,
		}

		android_test {
			name: "app_force_sdk",
			jni_libs: ["libjni"],
			platform_apis: true,
			jni_uses_sdk_apis: true,
		}

		cc_library {
			name: "libvendorjni",
			system_shared_libs: [],
			stl: "none",
			vendor: true,
		}

		android_test {
			name: "app_vendor",
			jni_libs: ["libvendorjni"],
			sdk_version: "current",
			vendor: true,
		}
	`)

	testCases := []struct {
		name      string
		sdkJNI    bool
		vendorJNI bool
	}{
		{name: "app_platform"},
		{name: "app_sdk", sdkJNI: true},
		{name: "app_force_platform"},
		{name: "app_force_sdk", sdkJNI: true},
		{name: "app_vendor", vendorJNI: true},
	}

	platformJNI := ctx.ModuleForTests("libjni", "android_arm64_armv8-a_shared").
		Output("libjni.so").Output.String()
	sdkJNI := ctx.ModuleForTests("libjni", "android_arm64_armv8-a_sdk_shared").
		Output("libjni.so").Output.String()
	vendorJNI := ctx.ModuleForTests("libvendorjni", "android_arm64_armv8-a_shared").
		Output("libvendorjni.so").Output.String()

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			app := ctx.ModuleForTests(test.name, "android_common")

			jniLibZip := app.MaybeOutput("jnilibs.zip")
			if len(jniLibZip.Implicits) != 1 {
				t.Fatalf("expected exactly one jni library, got %q", jniLibZip.Implicits.Strings())
			}
			gotJNI := jniLibZip.Implicits[0].String()

			if test.sdkJNI {
				if gotJNI != sdkJNI {
					t.Errorf("expected SDK JNI library %q, got %q", sdkJNI, gotJNI)
				}
			} else if test.vendorJNI {
				if gotJNI != vendorJNI {
					t.Errorf("expected platform JNI library %q, got %q", vendorJNI, gotJNI)
				}
			} else {
				if gotJNI != platformJNI {
					t.Errorf("expected platform JNI library %q, got %q", platformJNI, gotJNI)
				}
			}
		})
	}

	t.Run("jni_uses_platform_apis_error", func(t *testing.T) {
		testJavaError(t, `jni_uses_platform_apis: can only be set for modules that set sdk_version`, `
			android_test {
				name: "app_platform",
				platform_apis: true,
				jni_uses_platform_apis: true,
			}
		`)
	})

	t.Run("jni_uses_sdk_apis_error", func(t *testing.T) {
		testJavaError(t, `jni_uses_sdk_apis: can only be set for modules that do not set sdk_version`, `
			android_test {
				name: "app_sdk",
				sdk_version: "current",
				jni_uses_sdk_apis: true,
			}
		`)
	})

}

func TestCertificates(t *testing.T) {
	testCases := []struct {
		name                string
		bp                  string
		certificateOverride string
		expectedLineage     string
		expectedCertificate string
	}{
		{
			name: "default",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "current",
				}
			`,
			certificateOverride: "",
			expectedLineage:     "",
			expectedCertificate: "build/make/target/product/security/testkey.x509.pem build/make/target/product/security/testkey.pk8",
		},
		{
			name: "module certificate property",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					certificate: ":new_certificate",
					sdk_version: "current",
				}

				android_app_certificate {
					name: "new_certificate",
					certificate: "cert/new_cert",
				}
			`,
			certificateOverride: "",
			expectedLineage:     "",
			expectedCertificate: "cert/new_cert.x509.pem cert/new_cert.pk8",
		},
		{
			name: "path certificate property",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					certificate: "expiredkey",
					sdk_version: "current",
				}
			`,
			certificateOverride: "",
			expectedLineage:     "",
			expectedCertificate: "build/make/target/product/security/expiredkey.x509.pem build/make/target/product/security/expiredkey.pk8",
		},
		{
			name: "certificate overrides",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					certificate: "expiredkey",
					sdk_version: "current",
				}

				android_app_certificate {
					name: "new_certificate",
					certificate: "cert/new_cert",
				}
			`,
			certificateOverride: "foo:new_certificate",
			expectedLineage:     "",
			expectedCertificate: "cert/new_cert.x509.pem cert/new_cert.pk8",
		},
		{
			name: "certificate lineage",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					certificate: ":new_certificate",
					lineage: "lineage.bin",
					sdk_version: "current",
				}

				android_app_certificate {
					name: "new_certificate",
					certificate: "cert/new_cert",
				}
			`,
			certificateOverride: "",
			expectedLineage:     "--lineage lineage.bin",
			expectedCertificate: "cert/new_cert.x509.pem cert/new_cert.pk8",
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			config := testAppConfig(nil, test.bp, nil)
			if test.certificateOverride != "" {
				config.TestProductVariables.CertificateOverrides = []string{test.certificateOverride}
			}
			ctx := testContext()

			run(t, ctx, config)
			foo := ctx.ModuleForTests("foo", "android_common")

			signapk := foo.Output("foo.apk")
			signCertificateFlags := signapk.Args["certificates"]
			if test.expectedCertificate != signCertificateFlags {
				t.Errorf("Incorrect signing flags, expected: %q, got: %q", test.expectedCertificate, signCertificateFlags)
			}

			signFlags := signapk.Args["flags"]
			if test.expectedLineage != signFlags {
				t.Errorf("Incorrect signing flags, expected: %q, got: %q", test.expectedLineage, signFlags)
			}
		})
	}
}

func TestRequestV4SigningFlag(t *testing.T) {
	testCases := []struct {
		name     string
		bp       string
		expected string
	}{
		{
			name: "default",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "current",
				}
			`,
			expected: "",
		},
		{
			name: "default",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "current",
					v4_signature: false,
				}
			`,
			expected: "",
		},
		{
			name: "module certificate property",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "current",
					v4_signature: true,
				}
			`,
			expected: "--enable-v4",
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			config := testAppConfig(nil, test.bp, nil)
			ctx := testContext()

			run(t, ctx, config)
			foo := ctx.ModuleForTests("foo", "android_common")

			signapk := foo.Output("foo.apk")
			signFlags := signapk.Args["flags"]
			if test.expected != signFlags {
				t.Errorf("Incorrect signing flags, expected: %q, got: %q", test.expected, signFlags)
			}
		})
	}
}

func TestPackageNameOverride(t *testing.T) {
	testCases := []struct {
		name                string
		bp                  string
		packageNameOverride string
		expected            []string
	}{
		{
			name: "default",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "current",
				}
			`,
			packageNameOverride: "",
			expected: []string{
				buildDir + "/.intermediates/foo/android_common/foo.apk",
				buildDir + "/target/product/test_device/system/app/foo/foo.apk",
			},
		},
		{
			name: "overridden",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "current",
				}
			`,
			packageNameOverride: "foo:bar",
			expected: []string{
				// The package apk should be still be the original name for test dependencies.
				buildDir + "/.intermediates/foo/android_common/bar.apk",
				buildDir + "/target/product/test_device/system/app/bar/bar.apk",
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			config := testAppConfig(nil, test.bp, nil)
			if test.packageNameOverride != "" {
				config.TestProductVariables.PackageNameOverrides = []string{test.packageNameOverride}
			}
			ctx := testContext()

			run(t, ctx, config)
			foo := ctx.ModuleForTests("foo", "android_common")

			outputs := foo.AllOutputs()
			outputMap := make(map[string]bool)
			for _, o := range outputs {
				outputMap[o] = true
			}
			for _, e := range test.expected {
				if _, exist := outputMap[e]; !exist {
					t.Errorf("Can't find %q in output files.\nAll outputs:%v", e, outputs)
				}
			}
		})
	}
}

func TestInstrumentationTargetOverridden(t *testing.T) {
	bp := `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			sdk_version: "current",
		}

		android_test {
			name: "bar",
			instrumentation_for: "foo",
			sdk_version: "current",
		}
		`
	config := testAppConfig(nil, bp, nil)
	config.TestProductVariables.ManifestPackageNameOverrides = []string{"foo:org.dandroid.bp"}
	ctx := testContext()

	run(t, ctx, config)

	bar := ctx.ModuleForTests("bar", "android_common")
	res := bar.Output("package-res.apk")
	aapt2Flags := res.Args["flags"]
	e := "--rename-instrumentation-target-package org.dandroid.bp"
	if !strings.Contains(aapt2Flags, e) {
		t.Errorf("target package renaming flag, %q is missing in aapt2 link flags, %q", e, aapt2Flags)
	}
}

func TestOverrideAndroidApp(t *testing.T) {
	ctx, _ := testJava(t, `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			certificate: "expiredkey",
			overrides: ["qux"],
			sdk_version: "current",
		}

		override_android_app {
			name: "bar",
			base: "foo",
			certificate: ":new_certificate",
			lineage: "lineage.bin",
			logging_parent: "bah",
		}

		android_app_certificate {
			name: "new_certificate",
			certificate: "cert/new_cert",
		}

		override_android_app {
			name: "baz",
			base: "foo",
			package_name: "org.dandroid.bp",
		}
		`)

	expectedVariants := []struct {
		moduleName     string
		variantName    string
		apkName        string
		apkPath        string
		certFlag       string
		lineageFlag    string
		overrides      []string
		aaptFlag       string
		logging_parent string
	}{
		{
			moduleName:     "foo",
			variantName:    "android_common",
			apkPath:        "/target/product/test_device/system/app/foo/foo.apk",
			certFlag:       "build/make/target/product/security/expiredkey.x509.pem build/make/target/product/security/expiredkey.pk8",
			lineageFlag:    "",
			overrides:      []string{"qux"},
			aaptFlag:       "",
			logging_parent: "",
		},
		{
			moduleName:     "bar",
			variantName:    "android_common_bar",
			apkPath:        "/target/product/test_device/system/app/bar/bar.apk",
			certFlag:       "cert/new_cert.x509.pem cert/new_cert.pk8",
			lineageFlag:    "--lineage lineage.bin",
			overrides:      []string{"qux", "foo"},
			aaptFlag:       "",
			logging_parent: "bah",
		},
		{
			moduleName:     "baz",
			variantName:    "android_common_baz",
			apkPath:        "/target/product/test_device/system/app/baz/baz.apk",
			certFlag:       "build/make/target/product/security/expiredkey.x509.pem build/make/target/product/security/expiredkey.pk8",
			lineageFlag:    "",
			overrides:      []string{"qux", "foo"},
			aaptFlag:       "--rename-manifest-package org.dandroid.bp",
			logging_parent: "",
		},
	}
	for _, expected := range expectedVariants {
		variant := ctx.ModuleForTests("foo", expected.variantName)

		// Check the final apk name
		outputs := variant.AllOutputs()
		expectedApkPath := buildDir + expected.apkPath
		found := false
		for _, o := range outputs {
			if o == expectedApkPath {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Can't find %q in output files.\nAll outputs:%v", expectedApkPath, outputs)
		}

		// Check the certificate paths
		signapk := variant.Output(expected.moduleName + ".apk")
		certFlag := signapk.Args["certificates"]
		if expected.certFlag != certFlag {
			t.Errorf("Incorrect signing flags, expected: %q, got: %q", expected.certFlag, certFlag)
		}

		// Check the lineage flags
		lineageFlag := signapk.Args["flags"]
		if expected.lineageFlag != lineageFlag {
			t.Errorf("Incorrect signing flags, expected: %q, got: %q", expected.lineageFlag, lineageFlag)
		}

		// Check if the overrides field values are correctly aggregated.
		mod := variant.Module().(*AndroidApp)
		if !reflect.DeepEqual(expected.overrides, mod.appProperties.Overrides) {
			t.Errorf("Incorrect overrides property value, expected: %q, got: %q",
				expected.overrides, mod.appProperties.Overrides)
		}

		// Test Overridable property: Logging_parent
		logging_parent := mod.aapt.LoggingParent
		if expected.logging_parent != logging_parent {
			t.Errorf("Incorrect overrides property value for logging parent, expected: %q, got: %q",
				expected.logging_parent, logging_parent)
		}

		// Check the package renaming flag, if exists.
		res := variant.Output("package-res.apk")
		aapt2Flags := res.Args["flags"]
		if !strings.Contains(aapt2Flags, expected.aaptFlag) {
			t.Errorf("package renaming flag, %q is missing in aapt2 link flags, %q", expected.aaptFlag, aapt2Flags)
		}
	}
}

func TestOverrideAndroidAppDependency(t *testing.T) {
	ctx, _ := testJava(t, `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			sdk_version: "current",
		}

		override_android_app {
			name: "bar",
			base: "foo",
			package_name: "org.dandroid.bp",
		}

		android_test {
			name: "baz",
			srcs: ["b.java"],
			instrumentation_for: "foo",
		}

		android_test {
			name: "qux",
			srcs: ["b.java"],
			instrumentation_for: "bar",
		}
		`)

	// Verify baz, which depends on the overridden module foo, has the correct classpath javac arg.
	javac := ctx.ModuleForTests("baz", "android_common").Rule("javac")
	fooTurbine := filepath.Join(buildDir, ".intermediates", "foo", "android_common", "turbine-combined", "foo.jar")
	if !strings.Contains(javac.Args["classpath"], fooTurbine) {
		t.Errorf("baz classpath %v does not contain %q", javac.Args["classpath"], fooTurbine)
	}

	// Verify qux, which depends on the overriding module bar, has the correct classpath javac arg.
	javac = ctx.ModuleForTests("qux", "android_common").Rule("javac")
	barTurbine := filepath.Join(buildDir, ".intermediates", "foo", "android_common_bar", "turbine-combined", "foo.jar")
	if !strings.Contains(javac.Args["classpath"], barTurbine) {
		t.Errorf("qux classpath %v does not contain %q", javac.Args["classpath"], barTurbine)
	}
}

func TestOverrideAndroidTest(t *testing.T) {
	ctx, _ := testJava(t, `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			package_name: "com.android.foo",
			sdk_version: "current",
		}

		override_android_app {
			name: "bar",
			base: "foo",
			package_name: "com.android.bar",
		}

		android_test {
			name: "foo_test",
			srcs: ["b.java"],
			instrumentation_for: "foo",
		}

		override_android_test {
			name: "bar_test",
			base: "foo_test",
			package_name: "com.android.bar.test",
			instrumentation_for: "bar",
			instrumentation_target_package: "com.android.bar",
		}
		`)

	expectedVariants := []struct {
		moduleName        string
		variantName       string
		apkPath           string
		overrides         []string
		targetVariant     string
		packageFlag       string
		targetPackageFlag string
	}{
		{
			variantName:       "android_common",
			apkPath:           "/target/product/test_device/testcases/foo_test/arm64/foo_test.apk",
			overrides:         nil,
			targetVariant:     "android_common",
			packageFlag:       "",
			targetPackageFlag: "",
		},
		{
			variantName:       "android_common_bar_test",
			apkPath:           "/target/product/test_device/testcases/bar_test/arm64/bar_test.apk",
			overrides:         []string{"foo_test"},
			targetVariant:     "android_common_bar",
			packageFlag:       "com.android.bar.test",
			targetPackageFlag: "com.android.bar",
		},
	}
	for _, expected := range expectedVariants {
		variant := ctx.ModuleForTests("foo_test", expected.variantName)

		// Check the final apk name
		outputs := variant.AllOutputs()
		expectedApkPath := buildDir + expected.apkPath
		found := false
		for _, o := range outputs {
			if o == expectedApkPath {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Can't find %q in output files.\nAll outputs:%v", expectedApkPath, outputs)
		}

		// Check if the overrides field values are correctly aggregated.
		mod := variant.Module().(*AndroidTest)
		if !reflect.DeepEqual(expected.overrides, mod.appProperties.Overrides) {
			t.Errorf("Incorrect overrides property value, expected: %q, got: %q",
				expected.overrides, mod.appProperties.Overrides)
		}

		// Check if javac classpath has the correct jar file path. This checks instrumentation_for overrides.
		javac := variant.Rule("javac")
		turbine := filepath.Join(buildDir, ".intermediates", "foo", expected.targetVariant, "turbine-combined", "foo.jar")
		if !strings.Contains(javac.Args["classpath"], turbine) {
			t.Errorf("classpath %q does not contain %q", javac.Args["classpath"], turbine)
		}

		// Check aapt2 flags.
		res := variant.Output("package-res.apk")
		aapt2Flags := res.Args["flags"]
		checkAapt2LinkFlag(t, aapt2Flags, "rename-manifest-package", expected.packageFlag)
		checkAapt2LinkFlag(t, aapt2Flags, "rename-instrumentation-target-package", expected.targetPackageFlag)
	}
}

func TestAndroidTest_FixTestConfig(t *testing.T) {
	ctx, _ := testJava(t, `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			package_name: "com.android.foo",
			sdk_version: "current",
		}

		android_test {
			name: "foo_test",
			srcs: ["b.java"],
			instrumentation_for: "foo",
		}

		android_test {
			name: "bar_test",
			srcs: ["b.java"],
			package_name: "com.android.bar.test",
			instrumentation_for: "foo",
		}

		override_android_test {
			name: "baz_test",
			base: "foo_test",
			package_name: "com.android.baz.test",
		}
		`)

	testCases := []struct {
		moduleName    string
		variantName   string
		expectedFlags []string
	}{
		{
			moduleName:  "foo_test",
			variantName: "android_common",
		},
		{
			moduleName:  "bar_test",
			variantName: "android_common",
			expectedFlags: []string{
				"--manifest " + buildDir + "/.intermediates/bar_test/android_common/manifest_fixer/AndroidManifest.xml",
				"--package-name com.android.bar.test",
			},
		},
		{
			moduleName:  "foo_test",
			variantName: "android_common_baz_test",
			expectedFlags: []string{
				"--manifest " + buildDir +
					"/.intermediates/foo_test/android_common_baz_test/manifest_fixer/AndroidManifest.xml",
				"--package-name com.android.baz.test",
				"--test-file-name baz_test.apk",
			},
		},
	}

	for _, test := range testCases {
		variant := ctx.ModuleForTests(test.moduleName, test.variantName)
		params := variant.MaybeOutput("test_config_fixer/AndroidTest.xml")

		if len(test.expectedFlags) > 0 {
			if params.Rule == nil {
				t.Errorf("test_config_fixer was expected to run, but didn't")
			} else {
				for _, flag := range test.expectedFlags {
					if !strings.Contains(params.RuleParams.Command, flag) {
						t.Errorf("Flag %q was not found in command: %q", flag, params.RuleParams.Command)
					}
				}
			}
		} else {
			if params.Rule != nil {
				t.Errorf("test_config_fixer was not expected to run, but did: %q", params.RuleParams.Command)
			}
		}

	}
}

func TestAndroidAppImport(t *testing.T) {
	ctx, _ := testJava(t, `
		android_app_import {
			name: "foo",
			apk: "prebuilts/apk/app.apk",
			certificate: "platform",
			dex_preopt: {
				enabled: true,
			},
		}
		`)

	variant := ctx.ModuleForTests("foo", "android_common")

	// Check dexpreopt outputs.
	if variant.MaybeOutput("dexpreopt/oat/arm64/package.vdex").Rule == nil ||
		variant.MaybeOutput("dexpreopt/oat/arm64/package.odex").Rule == nil {
		t.Errorf("can't find dexpreopt outputs")
	}

	// Check cert signing flag.
	signedApk := variant.Output("signed/foo.apk")
	signingFlag := signedApk.Args["certificates"]
	expected := "build/make/target/product/security/platform.x509.pem build/make/target/product/security/platform.pk8"
	if expected != signingFlag {
		t.Errorf("Incorrect signing flags, expected: %q, got: %q", expected, signingFlag)
	}
}

func TestAndroidAppImport_NoDexPreopt(t *testing.T) {
	ctx, _ := testJava(t, `
		android_app_import {
			name: "foo",
			apk: "prebuilts/apk/app.apk",
			certificate: "platform",
			dex_preopt: {
				enabled: false,
			},
		}
		`)

	variant := ctx.ModuleForTests("foo", "android_common")

	// Check dexpreopt outputs. They shouldn't exist.
	if variant.MaybeOutput("dexpreopt/oat/arm64/package.vdex").Rule != nil ||
		variant.MaybeOutput("dexpreopt/oat/arm64/package.odex").Rule != nil {
		t.Errorf("dexpreopt shouldn't have run.")
	}
}

func TestAndroidAppImport_Presigned(t *testing.T) {
	ctx, _ := testJava(t, `
		android_app_import {
			name: "foo",
			apk: "prebuilts/apk/app.apk",
			presigned: true,
			dex_preopt: {
				enabled: true,
			},
		}
		`)

	variant := ctx.ModuleForTests("foo", "android_common")

	// Check dexpreopt outputs.
	if variant.MaybeOutput("dexpreopt/oat/arm64/package.vdex").Rule == nil ||
		variant.MaybeOutput("dexpreopt/oat/arm64/package.odex").Rule == nil {
		t.Errorf("can't find dexpreopt outputs")
	}
	// Make sure signing was skipped and aligning was done.
	if variant.MaybeOutput("signed/foo.apk").Rule != nil {
		t.Errorf("signing rule shouldn't be included.")
	}
	if variant.MaybeOutput("zip-aligned/foo.apk").Rule == nil {
		t.Errorf("can't find aligning rule")
	}
}

func TestAndroidAppImport_SigningLineage(t *testing.T) {
	ctx, _ := testJava(t, `
	  android_app_import {
			name: "foo",
			apk: "prebuilts/apk/app.apk",
			certificate: "platform",
			lineage: "lineage.bin",
		}
	`)

	variant := ctx.ModuleForTests("foo", "android_common")

	// Check cert signing lineage flag.
	signedApk := variant.Output("signed/foo.apk")
	signingFlag := signedApk.Args["flags"]
	expected := "--lineage lineage.bin"
	if expected != signingFlag {
		t.Errorf("Incorrect signing flags, expected: %q, got: %q", expected, signingFlag)
	}
}

func TestAndroidAppImport_DefaultDevCert(t *testing.T) {
	ctx, _ := testJava(t, `
		android_app_import {
			name: "foo",
			apk: "prebuilts/apk/app.apk",
			default_dev_cert: true,
			dex_preopt: {
				enabled: true,
			},
		}
		`)

	variant := ctx.ModuleForTests("foo", "android_common")

	// Check dexpreopt outputs.
	if variant.MaybeOutput("dexpreopt/oat/arm64/package.vdex").Rule == nil ||
		variant.MaybeOutput("dexpreopt/oat/arm64/package.odex").Rule == nil {
		t.Errorf("can't find dexpreopt outputs")
	}

	// Check cert signing flag.
	signedApk := variant.Output("signed/foo.apk")
	signingFlag := signedApk.Args["certificates"]
	expected := "build/make/target/product/security/testkey.x509.pem build/make/target/product/security/testkey.pk8"
	if expected != signingFlag {
		t.Errorf("Incorrect signing flags, expected: %q, got: %q", expected, signingFlag)
	}
}

func TestAndroidAppImport_DpiVariants(t *testing.T) {
	bp := `
		android_app_import {
			name: "foo",
			apk: "prebuilts/apk/app.apk",
			dpi_variants: {
				xhdpi: {
					apk: "prebuilts/apk/app_xhdpi.apk",
				},
				xxhdpi: {
					apk: "prebuilts/apk/app_xxhdpi.apk",
				},
			},
			presigned: true,
			dex_preopt: {
				enabled: true,
			},
		}
		`
	testCases := []struct {
		name                string
		aaptPreferredConfig *string
		aaptPrebuiltDPI     []string
		expected            string
	}{
		{
			name:                "no preferred",
			aaptPreferredConfig: nil,
			aaptPrebuiltDPI:     []string{},
			expected:            "prebuilts/apk/app.apk",
		},
		{
			name:                "AAPTPreferredConfig matches",
			aaptPreferredConfig: proptools.StringPtr("xhdpi"),
			aaptPrebuiltDPI:     []string{"xxhdpi", "ldpi"},
			expected:            "prebuilts/apk/app_xhdpi.apk",
		},
		{
			name:                "AAPTPrebuiltDPI matches",
			aaptPreferredConfig: proptools.StringPtr("mdpi"),
			aaptPrebuiltDPI:     []string{"xxhdpi", "xhdpi"},
			expected:            "prebuilts/apk/app_xxhdpi.apk",
		},
		{
			name:                "non-first AAPTPrebuiltDPI matches",
			aaptPreferredConfig: proptools.StringPtr("mdpi"),
			aaptPrebuiltDPI:     []string{"ldpi", "xhdpi"},
			expected:            "prebuilts/apk/app_xhdpi.apk",
		},
		{
			name:                "no matches",
			aaptPreferredConfig: proptools.StringPtr("mdpi"),
			aaptPrebuiltDPI:     []string{"ldpi", "xxxhdpi"},
			expected:            "prebuilts/apk/app.apk",
		},
	}

	jniRuleRe := regexp.MustCompile("^if \\(zipinfo (\\S+)")
	for _, test := range testCases {
		config := testAppConfig(nil, bp, nil)
		config.TestProductVariables.AAPTPreferredConfig = test.aaptPreferredConfig
		config.TestProductVariables.AAPTPrebuiltDPI = test.aaptPrebuiltDPI
		ctx := testContext()

		run(t, ctx, config)

		variant := ctx.ModuleForTests("foo", "android_common")
		jniRuleCommand := variant.Output("jnis-uncompressed/foo.apk").RuleParams.Command
		matches := jniRuleRe.FindStringSubmatch(jniRuleCommand)
		if len(matches) != 2 {
			t.Errorf("failed to extract the src apk path from %q", jniRuleCommand)
		}
		if test.expected != matches[1] {
			t.Errorf("wrong src apk, expected: %q got: %q", test.expected, matches[1])
		}
	}
}

func TestAndroidAppImport_Filename(t *testing.T) {
	ctx, config := testJava(t, `
		android_app_import {
			name: "foo",
			apk: "prebuilts/apk/app.apk",
			presigned: true,
		}

		android_app_import {
			name: "bar",
			apk: "prebuilts/apk/app.apk",
			presigned: true,
			filename: "bar_sample.apk"
		}
		`)

	testCases := []struct {
		name     string
		expected string
	}{
		{
			name:     "foo",
			expected: "foo.apk",
		},
		{
			name:     "bar",
			expected: "bar_sample.apk",
		},
	}

	for _, test := range testCases {
		variant := ctx.ModuleForTests(test.name, "android_common")
		if variant.MaybeOutput(test.expected).Rule == nil {
			t.Errorf("can't find output named %q - all outputs: %v", test.expected, variant.AllOutputs())
		}

		a := variant.Module().(*AndroidAppImport)
		expectedValues := []string{test.expected}
		actualValues := android.AndroidMkEntriesForTest(
			t, config, "", a)[0].EntryMap["LOCAL_INSTALLED_MODULE_STEM"]
		if !reflect.DeepEqual(actualValues, expectedValues) {
			t.Errorf("Incorrect LOCAL_INSTALLED_MODULE_STEM value '%s', expected '%s'",
				actualValues, expectedValues)
		}
	}
}

func TestAndroidAppImport_ArchVariants(t *testing.T) {
	// The test config's target arch is ARM64.
	testCases := []struct {
		name     string
		bp       string
		expected string
	}{
		{
			name: "matching arch",
			bp: `
				android_app_import {
					name: "foo",
					apk: "prebuilts/apk/app.apk",
					arch: {
						arm64: {
							apk: "prebuilts/apk/app_arm64.apk",
						},
					},
					presigned: true,
					dex_preopt: {
						enabled: true,
					},
				}
			`,
			expected: "prebuilts/apk/app_arm64.apk",
		},
		{
			name: "no matching arch",
			bp: `
				android_app_import {
					name: "foo",
					apk: "prebuilts/apk/app.apk",
					arch: {
						arm: {
							apk: "prebuilts/apk/app_arm.apk",
						},
					},
					presigned: true,
					dex_preopt: {
						enabled: true,
					},
				}
			`,
			expected: "prebuilts/apk/app.apk",
		},
	}

	jniRuleRe := regexp.MustCompile("^if \\(zipinfo (\\S+)")
	for _, test := range testCases {
		ctx, _ := testJava(t, test.bp)

		variant := ctx.ModuleForTests("foo", "android_common")
		jniRuleCommand := variant.Output("jnis-uncompressed/foo.apk").RuleParams.Command
		matches := jniRuleRe.FindStringSubmatch(jniRuleCommand)
		if len(matches) != 2 {
			t.Errorf("failed to extract the src apk path from %q", jniRuleCommand)
		}
		if test.expected != matches[1] {
			t.Errorf("wrong src apk, expected: %q got: %q", test.expected, matches[1])
		}
	}
}

func TestAndroidTestImport(t *testing.T) {
	ctx, config := testJava(t, `
		android_test_import {
			name: "foo",
			apk: "prebuilts/apk/app.apk",
			presigned: true,
			data: [
				"testdata/data",
			],
		}
		`)

	test := ctx.ModuleForTests("foo", "android_common").Module().(*AndroidTestImport)

	// Check android mks.
	entries := android.AndroidMkEntriesForTest(t, config, "", test)[0]
	expected := []string{"tests"}
	actual := entries.EntryMap["LOCAL_MODULE_TAGS"]
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Unexpected module tags - expected: %q, actual: %q", expected, actual)
	}
	expected = []string{"testdata/data:testdata/data"}
	actual = entries.EntryMap["LOCAL_COMPATIBILITY_SUPPORT_FILES"]
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Unexpected test data - expected: %q, actual: %q", expected, actual)
	}
}

func TestAndroidTestImport_NoJinUncompressForPresigned(t *testing.T) {
	ctx, _ := testJava(t, `
		android_test_import {
			name: "foo",
			apk: "prebuilts/apk/app.apk",
			certificate: "cert/new_cert",
			data: [
				"testdata/data",
			],
		}

		android_test_import {
			name: "foo_presigned",
			apk: "prebuilts/apk/app.apk",
			presigned: true,
			data: [
				"testdata/data",
			],
		}
		`)

	variant := ctx.ModuleForTests("foo", "android_common")
	jniRule := variant.Output("jnis-uncompressed/foo.apk").RuleParams.Command
	if !strings.HasPrefix(jniRule, "if (zipinfo") {
		t.Errorf("Unexpected JNI uncompress rule command: " + jniRule)
	}

	variant = ctx.ModuleForTests("foo_presigned", "android_common")
	jniRule = variant.Output("jnis-uncompressed/foo_presigned.apk").BuildParams.Rule.String()
	if jniRule != android.Cp.String() {
		t.Errorf("Unexpected JNI uncompress rule: " + jniRule)
	}
	if variant.MaybeOutput("zip-aligned/foo_presigned.apk").Rule == nil {
		t.Errorf("Presigned test apk should be aligned")
	}
}

func TestAndroidTestImport_Preprocessed(t *testing.T) {
	ctx, _ := testJava(t, `
		android_test_import {
			name: "foo",
			apk: "prebuilts/apk/app.apk",
			presigned: true,
			preprocessed: true,
		}

		android_test_import {
			name: "foo_cert",
			apk: "prebuilts/apk/app.apk",
			certificate: "cert/new_cert",
			preprocessed: true,
		}
		`)

	testModules := []string{"foo", "foo_cert"}
	for _, m := range testModules {
		apkName := m + ".apk"
		variant := ctx.ModuleForTests(m, "android_common")
		jniRule := variant.Output("jnis-uncompressed/" + apkName).BuildParams.Rule.String()
		if jniRule != android.Cp.String() {
			t.Errorf("Unexpected JNI uncompress rule: " + jniRule)
		}

		// Make sure signing and aligning were skipped.
		if variant.MaybeOutput("signed/"+apkName).Rule != nil {
			t.Errorf("signing rule shouldn't be included for preprocessed.")
		}
		if variant.MaybeOutput("zip-aligned/"+apkName).Rule != nil {
			t.Errorf("aligning rule shouldn't be for preprocessed")
		}
	}
}

func TestStl(t *testing.T) {
	ctx, _ := testJava(t, cc.GatherRequiredDepsForTest(android.Android)+`
		cc_library {
			name: "libjni",
			sdk_version: "current",
			stl: "c++_shared",
		}

		android_test {
			name: "stl",
			jni_libs: ["libjni"],
			compile_multilib: "both",
			sdk_version: "current",
			stl: "c++_shared",
		}

		android_test {
			name: "system",
			jni_libs: ["libjni"],
			compile_multilib: "both",
			sdk_version: "current",
		}
		`)

	testCases := []struct {
		name string
		jnis []string
	}{
		{"stl",
			[]string{
				"libjni.so",
				"libc++_shared.so",
			},
		},
		{"system",
			[]string{
				"libjni.so",
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			app := ctx.ModuleForTests(test.name, "android_common")
			jniLibZip := app.Output("jnilibs.zip")
			var jnis []string
			args := strings.Fields(jniLibZip.Args["jarArgs"])
			for i := 0; i < len(args); i++ {
				if args[i] == "-f" {
					jnis = append(jnis, args[i+1])
					i += 1
				}
			}
			jnisJoined := strings.Join(jnis, " ")
			for _, jni := range test.jnis {
				if !strings.Contains(jnisJoined, jni) {
					t.Errorf("missing jni %q in %q", jni, jnis)
				}
			}
		})
	}
}

func TestUsesLibraries(t *testing.T) {
	bp := `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			api_packages: ["foo"],
			sdk_version: "current",
		}

		java_sdk_library {
			name: "qux",
			srcs: ["a.java"],
			api_packages: ["qux"],
			sdk_version: "current",
		}

		java_sdk_library {
			name: "quuz",
			srcs: ["a.java"],
			api_packages: ["quuz"],
			sdk_version: "current",
		}

		java_sdk_library {
			name: "bar",
			srcs: ["a.java"],
			api_packages: ["bar"],
			sdk_version: "current",
		}

		android_app {
			name: "app",
			srcs: ["a.java"],
			libs: ["qux", "quuz.stubs"],
			uses_libs: ["foo"],
			sdk_version: "current",
			optional_uses_libs: [
				"bar",
				"baz",
			],
		}

		android_app_import {
			name: "prebuilt",
			apk: "prebuilts/apk/app.apk",
			certificate: "platform",
			uses_libs: ["foo"],
			optional_uses_libs: [
				"bar",
				"baz",
			],
		}
	`

	config := testAppConfig(nil, bp, nil)
	config.TestProductVariables.MissingUsesLibraries = []string{"baz"}

	ctx := testContext()

	run(t, ctx, config)

	app := ctx.ModuleForTests("app", "android_common")
	prebuilt := ctx.ModuleForTests("prebuilt", "android_common")

	// Test that implicit dependencies on java_sdk_library instances are passed to the manifest.
	manifestFixerArgs := app.Output("manifest_fixer/AndroidManifest.xml").Args["args"]
	if w := "--uses-library qux"; !strings.Contains(manifestFixerArgs, w) {
		t.Errorf("unexpected manifest_fixer args: wanted %q in %q", w, manifestFixerArgs)
	}
	if w := "--uses-library quuz"; !strings.Contains(manifestFixerArgs, w) {
		t.Errorf("unexpected manifest_fixer args: wanted %q in %q", w, manifestFixerArgs)
	}

	// Test that all libraries are verified
	cmd := app.Rule("verify_uses_libraries").RuleParams.Command
	if w := "--uses-library foo"; !strings.Contains(cmd, w) {
		t.Errorf("wanted %q in %q", w, cmd)
	}

	if w := "--optional-uses-library bar --optional-uses-library baz"; !strings.Contains(cmd, w) {
		t.Errorf("wanted %q in %q", w, cmd)
	}

	cmd = prebuilt.Rule("verify_uses_libraries").RuleParams.Command

	if w := `uses_library_names="foo"`; !strings.Contains(cmd, w) {
		t.Errorf("wanted %q in %q", w, cmd)
	}

	if w := `optional_uses_library_names="bar baz"`; !strings.Contains(cmd, w) {
		t.Errorf("wanted %q in %q", w, cmd)
	}

	// Test that only present libraries are preopted
	cmd = app.Rule("dexpreopt").RuleParams.Command

	if w := `dex_preopt_target_libraries="/system/framework/foo.jar /system/framework/bar.jar"`; !strings.Contains(cmd, w) {
		t.Errorf("wanted %q in %q", w, cmd)
	}

	cmd = prebuilt.Rule("dexpreopt").RuleParams.Command

	if w := `dex_preopt_target_libraries="/system/framework/foo.jar /system/framework/bar.jar"`; !strings.Contains(cmd, w) {
		t.Errorf("wanted %q in %q", w, cmd)
	}
}

func TestCodelessApp(t *testing.T) {
	testCases := []struct {
		name   string
		bp     string
		noCode bool
	}{
		{
			name: "normal",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "current",
				}
			`,
			noCode: false,
		},
		{
			name: "app without sources",
			bp: `
				android_app {
					name: "foo",
					sdk_version: "current",
				}
			`,
			noCode: true,
		},
		{
			name: "app with libraries",
			bp: `
				android_app {
					name: "foo",
					static_libs: ["lib"],
					sdk_version: "current",
				}

				java_library {
					name: "lib",
					srcs: ["a.java"],
					sdk_version: "current",
				}
			`,
			noCode: false,
		},
		{
			name: "app with sourceless libraries",
			bp: `
				android_app {
					name: "foo",
					static_libs: ["lib"],
					sdk_version: "current",
				}

				java_library {
					name: "lib",
					sdk_version: "current",
				}
			`,
			// TODO(jungjw): this should probably be true
			noCode: false,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			ctx := testApp(t, test.bp)

			foo := ctx.ModuleForTests("foo", "android_common")
			manifestFixerArgs := foo.Output("manifest_fixer/AndroidManifest.xml").Args["args"]
			if strings.Contains(manifestFixerArgs, "--has-no-code") != test.noCode {
				t.Errorf("unexpected manifest_fixer args: %q", manifestFixerArgs)
			}
		})
	}
}

func TestEmbedNotice(t *testing.T) {
	ctx, _ := testJavaWithFS(t, cc.GatherRequiredDepsForTest(android.Android)+`
		android_app {
			name: "foo",
			srcs: ["a.java"],
			static_libs: ["javalib"],
			jni_libs: ["libjni"],
			notice: "APP_NOTICE",
			embed_notices: true,
			sdk_version: "current",
		}

		// No embed_notice flag
		android_app {
			name: "bar",
			srcs: ["a.java"],
			jni_libs: ["libjni"],
			notice: "APP_NOTICE",
			sdk_version: "current",
		}

		// No NOTICE files
		android_app {
			name: "baz",
			srcs: ["a.java"],
			embed_notices: true,
			sdk_version: "current",
		}

		cc_library {
			name: "libjni",
			system_shared_libs: [],
			stl: "none",
			notice: "LIB_NOTICE",
			sdk_version: "current",
		}

		java_library {
			name: "javalib",
			srcs: [
				":gen",
			],
			sdk_version: "current",
		}

		genrule {
			name: "gen",
			tools: ["gentool"],
			out: ["gen.java"],
			notice: "GENRULE_NOTICE",
		}

		java_binary_host {
			name: "gentool",
			srcs: ["b.java"],
			notice: "TOOL_NOTICE",
		}
	`, map[string][]byte{
		"APP_NOTICE":     nil,
		"GENRULE_NOTICE": nil,
		"LIB_NOTICE":     nil,
		"TOOL_NOTICE":    nil,
	})

	// foo has NOTICE files to process, and embed_notices is true.
	foo := ctx.ModuleForTests("foo", "android_common")
	// verify merge notices rule.
	mergeNotices := foo.Rule("mergeNoticesRule")
	noticeInputs := mergeNotices.Inputs.Strings()
	// TOOL_NOTICE should be excluded as it's a host module.
	if len(mergeNotices.Inputs) != 3 {
		t.Errorf("number of input notice files: expected = 3, actual = %q", noticeInputs)
	}
	if !inList("APP_NOTICE", noticeInputs) {
		t.Errorf("APP_NOTICE is missing from notice files, %q", noticeInputs)
	}
	if !inList("LIB_NOTICE", noticeInputs) {
		t.Errorf("LIB_NOTICE is missing from notice files, %q", noticeInputs)
	}
	if !inList("GENRULE_NOTICE", noticeInputs) {
		t.Errorf("GENRULE_NOTICE is missing from notice files, %q", noticeInputs)
	}
	// aapt2 flags should include -A <NOTICE dir> so that its contents are put in the APK's /assets.
	res := foo.Output("package-res.apk")
	aapt2Flags := res.Args["flags"]
	e := "-A " + buildDir + "/.intermediates/foo/android_common/NOTICE"
	if !strings.Contains(aapt2Flags, e) {
		t.Errorf("asset dir flag for NOTICE, %q is missing in aapt2 link flags, %q", e, aapt2Flags)
	}

	// bar has NOTICE files to process, but embed_notices is not set.
	bar := ctx.ModuleForTests("bar", "android_common")
	res = bar.Output("package-res.apk")
	aapt2Flags = res.Args["flags"]
	e = "-A " + buildDir + "/.intermediates/bar/android_common/NOTICE"
	if strings.Contains(aapt2Flags, e) {
		t.Errorf("bar shouldn't have the asset dir flag for NOTICE: %q", e)
	}

	// baz's embed_notice is true, but it doesn't have any NOTICE files.
	baz := ctx.ModuleForTests("baz", "android_common")
	res = baz.Output("package-res.apk")
	aapt2Flags = res.Args["flags"]
	e = "-A " + buildDir + "/.intermediates/baz/android_common/NOTICE"
	if strings.Contains(aapt2Flags, e) {
		t.Errorf("baz shouldn't have the asset dir flag for NOTICE: %q", e)
	}
}

func TestUncompressDex(t *testing.T) {
	testCases := []struct {
		name string
		bp   string

		uncompressedPlatform  bool
		uncompressedUnbundled bool
	}{
		{
			name: "normal",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "current",
				}
			`,
			uncompressedPlatform:  true,
			uncompressedUnbundled: false,
		},
		{
			name: "use_embedded_dex",
			bp: `
				android_app {
					name: "foo",
					use_embedded_dex: true,
					srcs: ["a.java"],
					sdk_version: "current",
				}
			`,
			uncompressedPlatform:  true,
			uncompressedUnbundled: true,
		},
		{
			name: "privileged",
			bp: `
				android_app {
					name: "foo",
					privileged: true,
					srcs: ["a.java"],
					sdk_version: "current",
				}
			`,
			uncompressedPlatform:  true,
			uncompressedUnbundled: true,
		},
		{
			name: "normal_uncompress_dex_true",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "current",
					uncompress_dex: true,
				}
			`,
			uncompressedPlatform:  true,
			uncompressedUnbundled: true,
		},
		{
			name: "normal_uncompress_dex_false",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "current",
					uncompress_dex: false,
				}
			`,
			uncompressedPlatform:  false,
			uncompressedUnbundled: false,
		},
	}

	test := func(t *testing.T, bp string, want bool, unbundled bool) {
		t.Helper()

		config := testAppConfig(nil, bp, nil)
		if unbundled {
			config.TestProductVariables.Unbundled_build = proptools.BoolPtr(true)
		}

		ctx := testContext()

		run(t, ctx, config)

		foo := ctx.ModuleForTests("foo", "android_common")
		dex := foo.Rule("r8")
		uncompressedInDexJar := strings.Contains(dex.Args["zipFlags"], "-L 0")
		aligned := foo.MaybeRule("zipalign").Rule != nil

		if uncompressedInDexJar != want {
			t.Errorf("want uncompressed in dex %v, got %v", want, uncompressedInDexJar)
		}

		if aligned != want {
			t.Errorf("want aligned %v, got %v", want, aligned)
		}
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("platform", func(t *testing.T) {
				test(t, tt.bp, tt.uncompressedPlatform, false)
			})
			t.Run("unbundled", func(t *testing.T) {
				test(t, tt.bp, tt.uncompressedUnbundled, true)
			})
		})
	}
}

func checkAapt2LinkFlag(t *testing.T, aapt2Flags, flagName, expectedValue string) {
	if expectedValue != "" {
		expectedFlag := "--" + flagName + " " + expectedValue
		if !strings.Contains(aapt2Flags, expectedFlag) {
			t.Errorf("%q is missing in aapt2 link flags, %q", expectedFlag, aapt2Flags)
		}
	} else {
		unexpectedFlag := "--" + flagName
		if strings.Contains(aapt2Flags, unexpectedFlag) {
			t.Errorf("unexpected flag, %q is found in aapt2 link flags, %q", unexpectedFlag, aapt2Flags)
		}
	}
}

func TestRuntimeResourceOverlay(t *testing.T) {
	fs := map[string][]byte{
		"baz/res/res/values/strings.xml": nil,
		"bar/res/res/values/strings.xml": nil,
	}
	bp := `
		runtime_resource_overlay {
			name: "foo",
			certificate: "platform",
			lineage: "lineage.bin",
			product_specific: true,
			static_libs: ["bar"],
			resource_libs: ["baz"],
			aaptflags: ["--keep-raw-values"],
		}

		runtime_resource_overlay {
			name: "foo_themed",
			certificate: "platform",
			product_specific: true,
			theme: "faza",
			overrides: ["foo"],
		}

		android_library {
			name: "bar",
			resource_dirs: ["bar/res"],
		}

		android_app {
			name: "baz",
			sdk_version: "current",
			resource_dirs: ["baz/res"],
		}
		`
	config := testAppConfig(nil, bp, fs)
	ctx := testContext()
	run(t, ctx, config)

	m := ctx.ModuleForTests("foo", "android_common")

	// Check AAPT2 link flags.
	aapt2Flags := m.Output("package-res.apk").Args["flags"]
	expectedFlags := []string{"--keep-raw-values", "--no-resource-deduping", "--no-resource-removal"}
	absentFlags := android.RemoveListFromList(expectedFlags, strings.Split(aapt2Flags, " "))
	if len(absentFlags) > 0 {
		t.Errorf("expected values, %q are missing in aapt2 link flags, %q", absentFlags, aapt2Flags)
	}

	// Check overlay.list output for static_libs dependency.
	overlayList := m.Output("aapt2/overlay.list").Inputs.Strings()
	staticLibPackage := buildDir + "/.intermediates/bar/android_common/package-res.apk"
	if !inList(staticLibPackage, overlayList) {
		t.Errorf("Stactic lib res package %q missing in overlay list: %q", staticLibPackage, overlayList)
	}

	// Check AAPT2 link flags for resource_libs dependency.
	resourceLibFlag := "-I " + buildDir + "/.intermediates/baz/android_common/package-res.apk"
	if !strings.Contains(aapt2Flags, resourceLibFlag) {
		t.Errorf("Resource lib flag %q missing in aapt2 link flags: %q", resourceLibFlag, aapt2Flags)
	}

	// Check cert signing flag.
	signedApk := m.Output("signed/foo.apk")
	lineageFlag := signedApk.Args["flags"]
	expectedLineageFlag := "--lineage lineage.bin"
	if expectedLineageFlag != lineageFlag {
		t.Errorf("Incorrect signing lineage flags, expected: %q, got: %q", expectedLineageFlag, lineageFlag)
	}
	signingFlag := signedApk.Args["certificates"]
	expected := "build/make/target/product/security/platform.x509.pem build/make/target/product/security/platform.pk8"
	if expected != signingFlag {
		t.Errorf("Incorrect signing flags, expected: %q, got: %q", expected, signingFlag)
	}
	androidMkEntries := android.AndroidMkEntriesForTest(t, config, "", m.Module())[0]
	path := androidMkEntries.EntryMap["LOCAL_CERTIFICATE"]
	expectedPath := []string{"build/make/target/product/security/platform.x509.pem"}
	if !reflect.DeepEqual(path, expectedPath) {
		t.Errorf("Unexpected LOCAL_CERTIFICATE value: %v, expected: %v", path, expectedPath)
	}

	// Check device location.
	path = androidMkEntries.EntryMap["LOCAL_MODULE_PATH"]
	expectedPath = []string{"/tmp/target/product/test_device/product/overlay"}
	if !reflect.DeepEqual(path, expectedPath) {
		t.Errorf("Unexpected LOCAL_MODULE_PATH value: %v, expected: %v", path, expectedPath)
	}

	// A themed module has a different device location
	m = ctx.ModuleForTests("foo_themed", "android_common")
	androidMkEntries = android.AndroidMkEntriesForTest(t, config, "", m.Module())[0]
	path = androidMkEntries.EntryMap["LOCAL_MODULE_PATH"]
	expectedPath = []string{"/tmp/target/product/test_device/product/overlay/faza"}
	if !reflect.DeepEqual(path, expectedPath) {
		t.Errorf("Unexpected LOCAL_MODULE_PATH value: %v, expected: %v", path, expectedPath)
	}

	overrides := androidMkEntries.EntryMap["LOCAL_OVERRIDES_PACKAGES"]
	expectedOverrides := []string{"foo"}
	if !reflect.DeepEqual(overrides, expectedOverrides) {
		t.Errorf("Unexpected LOCAL_OVERRIDES_PACKAGES value: %v, expected: %v", overrides, expectedOverrides)
	}
}

func TestRuntimeResourceOverlay_JavaDefaults(t *testing.T) {
	ctx, config := testJava(t, `
		java_defaults {
			name: "rro_defaults",
			theme: "default_theme",
			product_specific: true,
			aaptflags: ["--keep-raw-values"],
		}

		runtime_resource_overlay {
			name: "foo_with_defaults",
			defaults: ["rro_defaults"],
		}

		runtime_resource_overlay {
			name: "foo_barebones",
		}
		`)

	//
	// RRO module with defaults
	//
	m := ctx.ModuleForTests("foo_with_defaults", "android_common")

	// Check AAPT2 link flags.
	aapt2Flags := strings.Split(m.Output("package-res.apk").Args["flags"], " ")
	expectedFlags := []string{"--keep-raw-values", "--no-resource-deduping", "--no-resource-removal"}
	absentFlags := android.RemoveListFromList(expectedFlags, aapt2Flags)
	if len(absentFlags) > 0 {
		t.Errorf("expected values, %q are missing in aapt2 link flags, %q", absentFlags, aapt2Flags)
	}

	// Check device location.
	path := android.AndroidMkEntriesForTest(t, config, "", m.Module())[0].EntryMap["LOCAL_MODULE_PATH"]
	expectedPath := []string{"/tmp/target/product/test_device/product/overlay/default_theme"}
	if !reflect.DeepEqual(path, expectedPath) {
		t.Errorf("Unexpected LOCAL_MODULE_PATH value: %q, expected: %q", path, expectedPath)
	}

	//
	// RRO module without defaults
	//
	m = ctx.ModuleForTests("foo_barebones", "android_common")

	// Check AAPT2 link flags.
	aapt2Flags = strings.Split(m.Output("package-res.apk").Args["flags"], " ")
	unexpectedFlags := "--keep-raw-values"
	if inList(unexpectedFlags, aapt2Flags) {
		t.Errorf("unexpected value, %q is present in aapt2 link flags, %q", unexpectedFlags, aapt2Flags)
	}

	// Check device location.
	path = android.AndroidMkEntriesForTest(t, config, "", m.Module())[0].EntryMap["LOCAL_MODULE_PATH"]
	expectedPath = []string{"/tmp/target/product/test_device/system/overlay"}
	if !reflect.DeepEqual(path, expectedPath) {
		t.Errorf("Unexpected LOCAL_MODULE_PATH value: %v, expected: %v", path, expectedPath)
	}
}

func TestOverrideRuntimeResourceOverlay(t *testing.T) {
	ctx, _ := testJava(t, `
		runtime_resource_overlay {
			name: "foo_overlay",
			certificate: "platform",
			product_specific: true,
			sdk_version: "current",
		}

		override_runtime_resource_overlay {
			name: "bar_overlay",
			base: "foo_overlay",
			package_name: "com.android.bar.overlay",
			target_package_name: "com.android.bar",
		}
		`)

	expectedVariants := []struct {
		moduleName        string
		variantName       string
		apkPath           string
		overrides         []string
		targetVariant     string
		packageFlag       string
		targetPackageFlag string
	}{
		{
			variantName:       "android_common",
			apkPath:           "/target/product/test_device/product/overlay/foo_overlay.apk",
			overrides:         nil,
			targetVariant:     "android_common",
			packageFlag:       "",
			targetPackageFlag: "",
		},
		{
			variantName:       "android_common_bar_overlay",
			apkPath:           "/target/product/test_device/product/overlay/bar_overlay.apk",
			overrides:         []string{"foo_overlay"},
			targetVariant:     "android_common_bar",
			packageFlag:       "com.android.bar.overlay",
			targetPackageFlag: "com.android.bar",
		},
	}
	for _, expected := range expectedVariants {
		variant := ctx.ModuleForTests("foo_overlay", expected.variantName)

		// Check the final apk name
		outputs := variant.AllOutputs()
		expectedApkPath := buildDir + expected.apkPath
		found := false
		for _, o := range outputs {
			if o == expectedApkPath {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Can't find %q in output files.\nAll outputs:%v", expectedApkPath, outputs)
		}

		// Check if the overrides field values are correctly aggregated.
		mod := variant.Module().(*RuntimeResourceOverlay)
		if !reflect.DeepEqual(expected.overrides, mod.properties.Overrides) {
			t.Errorf("Incorrect overrides property value, expected: %q, got: %q",
				expected.overrides, mod.properties.Overrides)
		}

		// Check aapt2 flags.
		res := variant.Output("package-res.apk")
		aapt2Flags := res.Args["flags"]
		checkAapt2LinkFlag(t, aapt2Flags, "rename-manifest-package", expected.packageFlag)
		checkAapt2LinkFlag(t, aapt2Flags, "rename-overlay-target-package", expected.targetPackageFlag)
	}
}
