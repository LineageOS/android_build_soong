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
	"sort"
	"strings"
	"testing"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
)

// testAppConfig is a legacy way of creating a test Config for testing java app modules.
//
// See testJava for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testAppConfig(env map[string]string, bp string, fs map[string][]byte) android.Config {
	return testConfig(env, bp, fs)
}

// testApp runs tests using the javaFixtureFactory
//
// See testJava for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testApp(t *testing.T, bp string) *android.TestContext {
	t.Helper()
	result := javaFixtureFactory.RunTestWithBp(t, bp)
	return result.TestContext
}

func TestApp(t *testing.T) {
	resourceFiles := []string{
		"res/layout/layout.xml",
		"res/values/strings.xml",
		"res/values-en-rUS/strings.xml",
	}

	compiledResourceFiles := []string{
		"aapt2/res/layout_layout.xml.flat",
		"aapt2/res/values_strings.arsc.flat",
		"aapt2/res/values-en-rUS_strings.arsc.flat",
	}

	for _, moduleType := range []string{"android_app", "android_library"} {
		t.Run(moduleType, func(t *testing.T) {
			result := javaFixtureFactory.Extend(
				android.FixtureModifyMockFS(func(fs android.MockFS) {
					for _, file := range resourceFiles {
						fs[file] = nil
					}
				}),
			).RunTestWithBp(t, moduleType+` {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "current"
				}
			`)

			foo := result.ModuleForTests("foo", "android_common")

			var expectedLinkImplicits []string

			manifestFixer := foo.Output("manifest_fixer/AndroidManifest.xml")
			expectedLinkImplicits = append(expectedLinkImplicits, manifestFixer.Output.String())

			frameworkRes := result.ModuleForTests("framework-res", "android_common")
			expectedLinkImplicits = append(expectedLinkImplicits,
				frameworkRes.Output("package-res.apk").Output.String())

			// Test the mapping from input files to compiled output file names
			compile := foo.Output(compiledResourceFiles[0])
			android.AssertDeepEquals(t, "aapt2 compile inputs", resourceFiles, compile.Inputs.Strings())

			compiledResourceOutputs := compile.Outputs.Strings()
			sort.Strings(compiledResourceOutputs)

			expectedLinkImplicits = append(expectedLinkImplicits, compiledResourceOutputs...)

			list := foo.Output("aapt2/res.list")
			expectedLinkImplicits = append(expectedLinkImplicits, list.Output.String())

			// Check that the link rule uses
			res := result.ModuleForTests("foo", "android_common").Output("package-res.apk")
			android.AssertDeepEquals(t, "aapt2 link implicits", expectedLinkImplicits, res.Implicits.Strings())
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

	testJavaError(t, "consider adjusting sdk_version: OR platform_apis:", `
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

	testJavaError(t, "consider adjusting sdk_version: OR platform_apis:", `
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
			errorHandler := android.FixtureExpectsNoErrors
			if test.expectedError != "" {
				errorHandler = android.FixtureExpectsAtLeastOneErrorMatchingPattern(test.expectedError)
			}
			javaFixtureFactory.
				Extend(FixtureWithPrebuiltApis(map[string][]string{
					"29": {"foo"},
				})).
				ExtendWithErrorHandler(errorHandler).RunTestWithBp(t, test.bp)
		})
	}
}

func TestUpdatableApps_TransitiveDepsShouldSetMinSdkVersion(t *testing.T) {
	testJavaError(t, `module "bar".*: should support min_sdk_version\(29\)`, cc.GatherRequiredDepsForTest(android.Android)+`
		android_app {
			name: "foo",
			srcs: ["a.java"],
			updatable: true,
			sdk_version: "current",
			min_sdk_version: "29",
			static_libs: ["bar"],
		}

		java_library {
			name: "bar",
			sdk_version: "current",
		}
	`)
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
	expectedCrtBegin := ctx.ModuleForTests("crtbegin_so",
		"android_arm64_armv8-a_sdk_29").Rule("partialLd").Output
	expectedCrtEnd := ctx.ModuleForTests("crtend_so",
		"android_arm64_armv8-a_sdk_29").Rule("partialLd").Output
	implicits := []string{}
	for _, input := range inputs {
		implicits = append(implicits, input.String())
		if strings.HasSuffix(input.String(), expectedCrtBegin.String()) {
			crtbeginFound = true
		} else if strings.HasSuffix(input.String(), expectedCrtEnd.String()) {
			crtendFound = true
		}
	}
	if !crtbeginFound {
		t.Error(fmt.Sprintf(
			"expected implicit with suffix %q, have the following implicits:\n%s",
			expectedCrtBegin, strings.Join(implicits, "\n")))
	}
	if !crtendFound {
		t.Error(fmt.Sprintf(
			"expected implicit with suffix %q, have the following implicits:\n%s",
			expectedCrtEnd, strings.Join(implicits, "\n")))
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
			ctx := testContext(config)
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

func TestAppJavaResources(t *testing.T) {
	bp := `
			android_app {
				name: "foo",
				sdk_version: "current",
				java_resources: ["resources/a"],
				srcs: ["a.java"],
			}

			android_app {
				name: "bar",
				sdk_version: "current",
				java_resources: ["resources/a"],
			}
		`

	ctx := testApp(t, bp)

	foo := ctx.ModuleForTests("foo", "android_common")
	fooResources := foo.Output("res/foo.jar")
	fooDexJar := foo.Output("dex-withres/foo.jar")
	fooDexJarAligned := foo.Output("dex-withres-aligned/foo.jar")
	fooApk := foo.Rule("combineApk")

	if g, w := fooDexJar.Inputs.Strings(), fooResources.Output.String(); !android.InList(w, g) {
		t.Errorf("expected resource jar %q in foo dex jar inputs %q", w, g)
	}

	if g, w := fooDexJarAligned.Input.String(), fooDexJar.Output.String(); g != w {
		t.Errorf("expected dex jar %q in foo aligned dex jar inputs %q", w, g)
	}

	if g, w := fooApk.Inputs.Strings(), fooDexJarAligned.Output.String(); !android.InList(w, g) {
		t.Errorf("expected aligned dex jar %q in foo apk inputs %q", w, g)
	}

	bar := ctx.ModuleForTests("bar", "android_common")
	barResources := bar.Output("res/bar.jar")
	barApk := bar.Rule("combineApk")

	if g, w := barApk.Inputs.Strings(), barResources.Output.String(); !android.InList(w, g) {
		t.Errorf("expected resources jar %q in bar apk inputs %q", w, g)
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
				},
			},

			rroDirs: map[string][]string{
				"foo": {
					"device:device/vendor/blah/overlay/foo/res",
					"product:product/vendor/blah/overlay/foo/res",
					"device:device/vendor/blah/overlay/lib/res",
				},
				"bar": nil,
				"lib": {"device:device/vendor/blah/overlay/lib/res"},
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

			ctx := testContext(config)
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

func checkSdkVersion(t *testing.T, result *android.TestResult, expectedSdkVersion string) {
	foo := result.ModuleForTests("foo", "android_common")
	link := foo.Output("package-res.apk")
	linkFlags := strings.Split(link.Args["flags"], " ")
	min := android.IndexList("--min-sdk-version", linkFlags)
	target := android.IndexList("--target-sdk-version", linkFlags)

	if min == -1 || target == -1 || min == len(linkFlags)-1 || target == len(linkFlags)-1 {
		t.Fatalf("missing --min-sdk-version or --target-sdk-version in link flags: %q", linkFlags)
	}

	gotMinSdkVersion := linkFlags[min+1]
	gotTargetSdkVersion := linkFlags[target+1]

	android.AssertStringEquals(t, "incorrect --min-sdk-version", expectedSdkVersion, gotMinSdkVersion)

	android.AssertStringEquals(t, "incorrect --target-sdk-version", expectedSdkVersion, gotTargetSdkVersion)
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
		activeCodenames       []string
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
			activeCodenames:       []string{"OMR1"},
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
			activeCodenames:       []string{"OMR1"},
		},
		{
			name:                  "14",
			sdkVersion:            "14",
			expectedMinSdkVersion: "14",
			platformSdkCodename:   "S",
			activeCodenames:       []string{"S"},
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

				result := javaFixtureFactory.Extend(
					android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
						variables.Platform_sdk_version = &test.platformSdkInt
						variables.Platform_sdk_codename = &test.platformSdkCodename
						variables.Platform_version_active_codenames = test.activeCodenames
						variables.Platform_sdk_final = &test.platformSdkFinal
					}),
					FixtureWithPrebuiltApis(map[string][]string{
						"14": {"foo"},
					}),
				).RunTestWithBp(t, bp)

				checkSdkVersion(t, result, test.expectedMinSdkVersion)
			})
		}
	}
}

func TestVendorAppSdkVersion(t *testing.T) {
	testCases := []struct {
		name                                  string
		sdkVersion                            string
		platformSdkInt                        int
		platformSdkCodename                   string
		platformSdkFinal                      bool
		deviceCurrentApiLevelForVendorModules string
		expectedMinSdkVersion                 string
	}{
		{
			name:                                  "current final SDK",
			sdkVersion:                            "current",
			platformSdkInt:                        29,
			platformSdkCodename:                   "REL",
			platformSdkFinal:                      true,
			deviceCurrentApiLevelForVendorModules: "29",
			expectedMinSdkVersion:                 "29",
		},
		{
			name:                                  "current final SDK",
			sdkVersion:                            "current",
			platformSdkInt:                        29,
			platformSdkCodename:                   "REL",
			platformSdkFinal:                      true,
			deviceCurrentApiLevelForVendorModules: "28",
			expectedMinSdkVersion:                 "28",
		},
		{
			name:                                  "current final SDK",
			sdkVersion:                            "current",
			platformSdkInt:                        29,
			platformSdkCodename:                   "Q",
			platformSdkFinal:                      false,
			deviceCurrentApiLevelForVendorModules: "28",
			expectedMinSdkVersion:                 "28",
		},
	}

	for _, moduleType := range []string{"android_app", "android_library"} {
		for _, sdkKind := range []string{"", "system_"} {
			for _, test := range testCases {
				t.Run(moduleType+" "+test.name, func(t *testing.T) {
					bp := fmt.Sprintf(`%s {
						name: "foo",
						srcs: ["a.java"],
						sdk_version: "%s%s",
						vendor: true,
					}`, moduleType, sdkKind, test.sdkVersion)

					result := javaFixtureFactory.Extend(
						android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
							variables.Platform_sdk_version = &test.platformSdkInt
							variables.Platform_sdk_codename = &test.platformSdkCodename
							variables.Platform_sdk_final = &test.platformSdkFinal
							variables.DeviceCurrentApiLevelForVendorModules = &test.deviceCurrentApiLevelForVendorModules
							variables.DeviceSystemSdkVersions = []string{"28", "29"}
						}),
						FixtureWithPrebuiltApis(map[string][]string{
							"28":      {"foo"},
							"29":      {"foo"},
							"current": {"foo"},
						}),
					).RunTestWithBp(t, bp)

					checkSdkVersion(t, result, test.expectedMinSdkVersion)
				})
			}
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
		{
			name: "lineage from filegroup",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					certificate: ":new_certificate",
					lineage: ":lineage_bin",
					sdk_version: "current",
				}

				android_app_certificate {
					name: "new_certificate",
					certificate: "cert/new_cert",
				}

				filegroup {
					name: "lineage_bin",
					srcs: ["lineage.bin"],
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
			ctx := testContext(config)

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
			ctx := testContext(config)

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
			ctx := testContext(config)

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
	ctx := testContext(config)

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

		override_android_app {
			name: "baz_no_rename_resources",
			base: "foo",
			package_name: "org.dandroid.bp",
			rename_resources_package: false,
		}

		android_app {
			name: "foo_no_rename_resources",
			srcs: ["a.java"],
			certificate: "expiredkey",
			overrides: ["qux"],
			rename_resources_package: false,
			sdk_version: "current",
		}

		override_android_app {
			name: "baz_base_no_rename_resources",
			base: "foo_no_rename_resources",
			package_name: "org.dandroid.bp",
		}

		override_android_app {
			name: "baz_override_base_rename_resources",
			base: "foo_no_rename_resources",
			package_name: "org.dandroid.bp",
			rename_resources_package: true,
		}
		`)

	expectedVariants := []struct {
		name            string
		moduleName      string
		variantName     string
		apkName         string
		apkPath         string
		certFlag        string
		lineageFlag     string
		overrides       []string
		packageFlag     string
		renameResources bool
		logging_parent  string
	}{
		{
			name:            "foo",
			moduleName:      "foo",
			variantName:     "android_common",
			apkPath:         "/target/product/test_device/system/app/foo/foo.apk",
			certFlag:        "build/make/target/product/security/expiredkey.x509.pem build/make/target/product/security/expiredkey.pk8",
			lineageFlag:     "",
			overrides:       []string{"qux"},
			packageFlag:     "",
			renameResources: false,
			logging_parent:  "",
		},
		{
			name:            "foo",
			moduleName:      "bar",
			variantName:     "android_common_bar",
			apkPath:         "/target/product/test_device/system/app/bar/bar.apk",
			certFlag:        "cert/new_cert.x509.pem cert/new_cert.pk8",
			lineageFlag:     "--lineage lineage.bin",
			overrides:       []string{"qux", "foo"},
			packageFlag:     "",
			renameResources: false,
			logging_parent:  "bah",
		},
		{
			name:            "foo",
			moduleName:      "baz",
			variantName:     "android_common_baz",
			apkPath:         "/target/product/test_device/system/app/baz/baz.apk",
			certFlag:        "build/make/target/product/security/expiredkey.x509.pem build/make/target/product/security/expiredkey.pk8",
			lineageFlag:     "",
			overrides:       []string{"qux", "foo"},
			packageFlag:     "org.dandroid.bp",
			renameResources: true,
			logging_parent:  "",
		},
		{
			name:            "foo",
			moduleName:      "baz_no_rename_resources",
			variantName:     "android_common_baz_no_rename_resources",
			apkPath:         "/target/product/test_device/system/app/baz_no_rename_resources/baz_no_rename_resources.apk",
			certFlag:        "build/make/target/product/security/expiredkey.x509.pem build/make/target/product/security/expiredkey.pk8",
			lineageFlag:     "",
			overrides:       []string{"qux", "foo"},
			packageFlag:     "org.dandroid.bp",
			renameResources: false,
			logging_parent:  "",
		},
		{
			name:            "foo_no_rename_resources",
			moduleName:      "baz_base_no_rename_resources",
			variantName:     "android_common_baz_base_no_rename_resources",
			apkPath:         "/target/product/test_device/system/app/baz_base_no_rename_resources/baz_base_no_rename_resources.apk",
			certFlag:        "build/make/target/product/security/expiredkey.x509.pem build/make/target/product/security/expiredkey.pk8",
			lineageFlag:     "",
			overrides:       []string{"qux", "foo_no_rename_resources"},
			packageFlag:     "org.dandroid.bp",
			renameResources: false,
			logging_parent:  "",
		},
		{
			name:            "foo_no_rename_resources",
			moduleName:      "baz_override_base_rename_resources",
			variantName:     "android_common_baz_override_base_rename_resources",
			apkPath:         "/target/product/test_device/system/app/baz_override_base_rename_resources/baz_override_base_rename_resources.apk",
			certFlag:        "build/make/target/product/security/expiredkey.x509.pem build/make/target/product/security/expiredkey.pk8",
			lineageFlag:     "",
			overrides:       []string{"qux", "foo_no_rename_resources"},
			packageFlag:     "org.dandroid.bp",
			renameResources: true,
			logging_parent:  "",
		},
	}
	for _, expected := range expectedVariants {
		variant := ctx.ModuleForTests(expected.name, expected.variantName)

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
		checkAapt2LinkFlag(t, aapt2Flags, "rename-manifest-package", expected.packageFlag)
		expectedPackage := expected.packageFlag
		if !expected.renameResources {
			expectedPackage = ""
		}
		checkAapt2LinkFlag(t, aapt2Flags, "rename-resources-package", expectedPackage)
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
		checkAapt2LinkFlag(t, aapt2Flags, "rename-resources-package", expected.packageFlag)
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
			name: "fred",
			srcs: ["a.java"],
			api_packages: ["fred"],
			sdk_version: "current",
		}

		java_sdk_library {
			name: "bar",
			srcs: ["a.java"],
			api_packages: ["bar"],
			sdk_version: "current",
		}

		java_sdk_library {
			name: "runtime-library",
			srcs: ["a.java"],
			sdk_version: "current",
		}

		java_library {
			name: "static-runtime-helper",
			srcs: ["a.java"],
			libs: ["runtime-library"],
			sdk_version: "current",
		}

		// A library that has to use "provides_uses_lib", because:
		//    - it is not an SDK library
		//    - its library name is different from its module name
		java_library {
			name: "non-sdk-lib",
			provides_uses_lib: "com.non.sdk.lib",
			installable: true,
			srcs: ["a.java"],
		}

		android_app {
			name: "app",
			srcs: ["a.java"],
			libs: [
				"qux",
				"quuz.stubs"
			],
			static_libs: [
				"static-runtime-helper",
				// statically linked component libraries should not pull their SDK libraries,
				// so "fred" should not be added to class loader context
				"fred.stubs",
			],
			uses_libs: [
				"foo",
				"non-sdk-lib"
			],
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
			uses_libs: [
				"foo",
				"non-sdk-lib",
				"android.test.runner"
			],
			optional_uses_libs: [
				"bar",
				"baz",
			],
		}
	`

	result := javaFixtureFactory.Extend(
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("runtime-library", "foo", "quuz", "qux", "bar", "fred"),
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.MissingUsesLibraries = []string{"baz"}
		}),
	).RunTestWithBp(t, bp)

	app := result.ModuleForTests("app", "android_common")
	prebuilt := result.ModuleForTests("prebuilt", "android_common")

	// Test that implicit dependencies on java_sdk_library instances are passed to the manifest.
	// This should not include explicit `uses_libs`/`optional_uses_libs` entries.
	actualManifestFixerArgs := app.Output("manifest_fixer/AndroidManifest.xml").Args["args"]
	expectManifestFixerArgs := `--extract-native-libs=true ` +
		`--uses-library qux ` +
		`--uses-library quuz ` +
		`--uses-library foo ` + // TODO(b/132357300): "foo" should not be passed to manifest_fixer
		`--uses-library com.non.sdk.lib ` + // TODO(b/132357300): "com.non.sdk.lib" should not be passed to manifest_fixer
		`--uses-library bar ` + // TODO(b/132357300): "bar" should not be passed to manifest_fixer
		`--uses-library runtime-library`
	android.AssertStringEquals(t, "manifest_fixer args", expectManifestFixerArgs, actualManifestFixerArgs)

	// Test that all libraries are verified (library order matters).
	verifyCmd := app.Rule("verify_uses_libraries").RuleParams.Command
	verifyArgs := `--uses-library foo ` +
		`--uses-library com.non.sdk.lib ` +
		`--uses-library qux ` +
		`--uses-library quuz ` +
		`--uses-library runtime-library ` +
		`--optional-uses-library bar ` +
		`--optional-uses-library baz `
	android.AssertStringDoesContain(t, "verify cmd args", verifyCmd, verifyArgs)

	// Test that all libraries are verified for an APK (library order matters).
	verifyApkCmd := prebuilt.Rule("verify_uses_libraries").RuleParams.Command
	verifyApkArgs := `--uses-library foo ` +
		`--uses-library com.non.sdk.lib ` +
		`--uses-library android.test.runner ` +
		`--optional-uses-library bar ` +
		`--optional-uses-library baz `
	android.AssertStringDoesContain(t, "verify apk cmd args", verifyApkCmd, verifyApkArgs)

	// Test that all present libraries are preopted, including implicit SDK dependencies, possibly stubs
	cmd := app.Rule("dexpreopt").RuleParams.Command
	w := `--target-context-for-sdk any ` +
		`PCL[/system/framework/qux.jar]#` +
		`PCL[/system/framework/quuz.jar]#` +
		`PCL[/system/framework/foo.jar]#` +
		`PCL[/system/framework/non-sdk-lib.jar]#` +
		`PCL[/system/framework/bar.jar]#` +
		`PCL[/system/framework/runtime-library.jar]`
	android.AssertStringDoesContain(t, "dexpreopt app cmd args", cmd, w)

	// Test conditional context for target SDK version 28.
	android.AssertStringDoesContain(t, "dexpreopt app cmd 28", cmd,
		`--target-context-for-sdk 28`+
			` PCL[/system/framework/org.apache.http.legacy.jar] `)

	// Test conditional context for target SDK version 29.
	android.AssertStringDoesContain(t, "dexpreopt app cmd 29", cmd,
		`--target-context-for-sdk 29`+
			` PCL[/system/framework/android.hidl.manager-V1.0-java.jar]`+
			`#PCL[/system/framework/android.hidl.base-V1.0-java.jar] `)

	// Test conditional context for target SDK version 30.
	// "android.test.mock" is absent because "android.test.runner" is not used.
	android.AssertStringDoesContain(t, "dexpreopt app cmd 30", cmd,
		`--target-context-for-sdk 30`+
			` PCL[/system/framework/android.test.base.jar] `)

	cmd = prebuilt.Rule("dexpreopt").RuleParams.Command
	android.AssertStringDoesContain(t, "dexpreopt prebuilt cmd", cmd,
		`--target-context-for-sdk any`+
			` PCL[/system/framework/foo.jar]`+
			`#PCL[/system/framework/non-sdk-lib.jar]`+
			`#PCL[/system/framework/android.test.runner.jar]`+
			`#PCL[/system/framework/bar.jar] `)

	// Test conditional context for target SDK version 30.
	// "android.test.mock" is present because "android.test.runner" is used.
	android.AssertStringDoesContain(t, "dexpreopt prebuilt cmd 30", cmd,
		`--target-context-for-sdk 30`+
			` PCL[/system/framework/android.test.base.jar]`+
			`#PCL[/system/framework/android.test.mock.jar] `)
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

		result := javaFixtureFactory.Extend(
			PrepareForTestWithPrebuiltsOfCurrentApi,
			android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				if unbundled {
					variables.Unbundled_build = proptools.BoolPtr(true)
					variables.Always_use_prebuilt_sdks = proptools.BoolPtr(true)
				}
			}),
		).RunTestWithBp(t, bp)

		foo := result.ModuleForTests("foo", "android_common")
		dex := foo.Rule("r8")
		uncompressedInDexJar := strings.Contains(dex.Args["zipFlags"], "-L 0")
		aligned := foo.MaybeRule("zipalign").Rule != nil

		android.AssertBoolEquals(t, "uncompressed in dex", want, uncompressedInDexJar)

		android.AssertBoolEquals(t, "aligne", want, aligned)
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

func TestExportedProguardFlagFiles(t *testing.T) {
	ctx, _ := testJava(t, `
		android_app {
			name: "foo",
			sdk_version: "current",
			static_libs: ["lib1"],
		}

		android_library {
			name: "lib1",
			sdk_version: "current",
			optimize: {
				proguard_flags_files: ["lib1proguard.cfg"],
			}
		}
	`)

	m := ctx.ModuleForTests("foo", "android_common")
	hasLib1Proguard := false
	for _, s := range m.Rule("java.r8").Implicits.Strings() {
		if s == "lib1proguard.cfg" {
			hasLib1Proguard = true
			break
		}
	}

	if !hasLib1Proguard {
		t.Errorf("App does not use library proguard config")
	}
}
