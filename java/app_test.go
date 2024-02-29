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
	"android/soong/dexpreopt"
)

// testApp runs tests using the prepareForJavaTest
//
// See testJava for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testApp(t *testing.T, bp string) *android.TestContext {
	t.Helper()
	result := prepareForJavaTest.RunTestWithBp(t, bp)
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
			result := android.GroupFixturePreparers(
				prepareForJavaTest,
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
		"out/soong/.intermediates/foo/android_common/foo.apk",
		"out/soong/.intermediates/foo/android_common/foo_v4.apk",
		"out/soong/.intermediates/foo/android_common/foo_v7_hdpi.apk",
	}
	for _, expectedOutput := range expectedOutputs {
		foo.Output(expectedOutput)
	}

	outputFiles, err := foo.Module().(*AndroidApp).OutputFiles("")
	if err != nil {
		t.Fatal(err)
	}
	android.AssertPathsRelativeToTopEquals(t, `OutputFiles("")`, expectedOutputs, outputFiles)
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

	testJavaError(t, "This module has conflicting settings. sdk_version is empty, which means that this module is build against platform APIs. However platform_apis is not set to true", `
		android_app {
			name: "bar",
			srcs: ["b.java"],
		}
	`)

	testJavaError(t, "This module has conflicting settings. sdk_version is not empty, which means this module cannot use platform APIs. However platform_apis is set to true.", `
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
			android.GroupFixturePreparers(
				prepareForJavaTest, FixtureWithPrebuiltApis(map[string][]string{
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
			sdk_version: "current",
			min_sdk_version: "29",
		}
	`
	fs := map[string][]byte{
		"prebuilts/ndk/current/platforms/android-29/arch-arm64/usr/lib/crtbegin_so.o": nil,
		"prebuilts/ndk/current/platforms/android-29/arch-arm64/usr/lib/crtend_so.o":   nil,
		"prebuilts/ndk/current/platforms/android-29/arch-arm/usr/lib/crtbegin_so.o":   nil,
		"prebuilts/ndk/current/platforms/android-29/arch-arm/usr/lib/crtend_so.o":     nil,
	}

	ctx, _ := testJavaWithFS(t, bp, fs)

	inputs := ctx.ModuleForTests("libjni", "android_arm64_armv8-a_sdk_shared").Description("link").Implicits
	var crtbeginFound, crtendFound bool
	expectedCrtBegin := ctx.ModuleForTests("crtbegin_so",
		"android_arm64_armv8-a_sdk_29").Rule("noAddrSig").Output
	expectedCrtEnd := ctx.ModuleForTests("crtend_so",
		"android_arm64_armv8-a_sdk_29").Rule("noAddrSig").Output
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
			min_sdk_version: "current",
		}
	`
	testJavaError(t, `"libjni" .*: min_sdk_version\(current\) is higher than min_sdk_version\(29\)`, bp)
}

func TestUpdatableApps_ErrorIfDepMinSdkVersionIsHigher(t *testing.T) {
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
			min_sdk_version: "27",
		}

		cc_library {
			name: "libbar",
			stl: "none",
			system_shared_libs: [],
			sdk_version: "current",
			min_sdk_version: "current",
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

	fs := android.MockFS{
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
			result := android.GroupFixturePreparers(
				PrepareForTestWithJavaDefaultModules,
				fs.AddToFixture(),
			).RunTestWithBp(t, fmt.Sprintf(bp, testCase.prop))

			module := result.ModuleForTests("foo", "android_common")
			resourceList := module.MaybeOutput("aapt2/res.list")

			var resources []string
			if resourceList.Rule != nil {
				for _, compiledResource := range resourceList.Inputs.Strings() {
					resources = append(resources, module.Output(compiledResource).Inputs.Strings()...)
				}
			}

			android.AssertDeepEquals(t, "resource files", testCase.resources, resources)
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
				static_libs: ["lib4", "import"],
			}

			android_library {
				name: "lib4",
				sdk_version: "current",
				asset_dirs: ["assets_b"],
			}

			android_library {
				name: "lib5",
				sdk_version: "current",
				assets: [
					"path/to/asset_file_1",
					"path/to/asset_file_2",
				],
			}

			android_library_import {
				name: "import",
				sdk_version: "current",
				aars: ["import.aar"],
			}
		`

	testCases := []struct {
		name               string
		assetFlag          string
		assetPackages      []string
		tmpAssetDirInputs  []string
		tmpAssetDirOutputs []string
	}{
		{
			name: "foo",
			// lib1 has its own assets. lib3 doesn't have any, but lib4 and import have assets.
			assetPackages: []string{
				"out/soong/.intermediates/foo/android_common/aapt2/package-res.apk",
				"out/soong/.intermediates/lib1/android_common/assets.zip",
				"out/soong/.intermediates/lib4/android_common/assets.zip",
				"out/soong/.intermediates/import/android_common/assets.zip",
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
		},
		{
			name:      "lib4",
			assetFlag: "-A assets_b",
		},
		{
			name:      "lib5",
			assetFlag: "-A out/soong/.intermediates/lib5/android_common/tmp_asset_dir",
			tmpAssetDirInputs: []string{
				"path/to/asset_file_1",
				"path/to/asset_file_2",
			},
			tmpAssetDirOutputs: []string{
				"out/soong/.intermediates/lib5/android_common/tmp_asset_dir/path/to/asset_file_1",
				"out/soong/.intermediates/lib5/android_common/tmp_asset_dir/path/to/asset_file_2",
			},
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
			aapt2link = aapt2link
			aapt2Flags := aapt2link.Args["flags"]
			if test.assetFlag != "" {
				android.AssertStringDoesContain(t, "asset flag", aapt2Flags, test.assetFlag)
			} else {
				android.AssertStringDoesNotContain(t, "aapt2 link flags", aapt2Flags, " -A ")
			}

			// Check asset merge rule.
			if len(test.assetPackages) > 0 {
				mergeAssets := m.Output("package-res.apk")
				android.AssertPathsRelativeToTopEquals(t, "mergeAssets inputs", test.assetPackages, mergeAssets.Inputs)
			}

			if len(test.tmpAssetDirInputs) > 0 {
				rule := m.Rule("tmp_asset_dir")
				inputs := rule.Implicits
				outputs := append(android.WritablePaths{rule.Output}, rule.ImplicitOutputs...).Paths()
				android.AssertPathsRelativeToTopEquals(t, "tmp_asset_dir inputs", test.tmpAssetDirInputs, inputs)
				android.AssertPathsRelativeToTopEquals(t, "tmp_asset_dir outputs", test.tmpAssetDirOutputs, outputs)
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

func TestAndroidResourceProcessor(t *testing.T) {
	testCases := []struct {
		name                            string
		appUsesRP                       bool
		directLibUsesRP                 bool
		transitiveLibUsesRP             bool
		sharedLibUsesRP                 bool
		sharedTransitiveStaticLibUsesRP bool
		sharedTransitiveSharedLibUsesRP bool

		dontVerifyApp bool
		appResources  []string
		appOverlays   []string
		appImports    []string
		appSrcJars    []string
		appClasspath  []string
		appCombined   []string

		dontVerifyDirect bool
		directResources  []string
		directOverlays   []string
		directImports    []string
		directSrcJars    []string
		directClasspath  []string
		directCombined   []string

		dontVerifyTransitive bool
		transitiveResources  []string
		transitiveOverlays   []string
		transitiveImports    []string
		transitiveSrcJars    []string
		transitiveClasspath  []string
		transitiveCombined   []string

		dontVerifyDirectImport bool
		directImportResources  []string
		directImportOverlays   []string
		directImportImports    []string

		dontVerifyTransitiveImport bool
		transitiveImportResources  []string
		transitiveImportOverlays   []string
		transitiveImportImports    []string

		dontVerifyShared bool
		sharedResources  []string
		sharedOverlays   []string
		sharedImports    []string
		sharedSrcJars    []string
		sharedClasspath  []string
		sharedCombined   []string
	}{
		{
			// Test with all modules set to use_resource_processor: false (except android_library_import modules,
			// which always use resource processor).
			name:                "legacy",
			appUsesRP:           false,
			directLibUsesRP:     false,
			transitiveLibUsesRP: false,

			appResources: nil,
			appOverlays: []string{
				"out/soong/.intermediates/transitive/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import_dep/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import/android_common/package-res.apk",
				"out/soong/.intermediates/direct/android_common/package-res.apk",
				"out/soong/.intermediates/direct_import_dep/android_common/package-res.apk",
				"out/soong/.intermediates/direct_import/android_common/package-res.apk",
				"out/soong/.intermediates/app/android_common/aapt2/app/res/values_strings.arsc.flat",
			},
			appImports: []string{
				"out/soong/.intermediates/shared/android_common/package-res.apk",
				"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk",
			},
			appSrcJars: []string{"out/soong/.intermediates/app/android_common/gen/android/R.srcjar"},
			appClasspath: []string{
				"out/soong/.intermediates/default/java/android_stubs_current/android_common/turbine-combined/android_stubs_current.jar",
				"out/soong/.intermediates/shared/android_common/turbine-combined/shared.jar",
				"out/soong/.intermediates/direct/android_common/turbine-combined/direct.jar",
				"out/soong/.intermediates/direct_import/android_common/aar/classes-combined.jar",
			},
			appCombined: []string{
				"out/soong/.intermediates/app/android_common/javac/app.jar",
				"out/soong/.intermediates/direct/android_common/combined/direct.jar",
				"out/soong/.intermediates/direct_import/android_common/aar/classes-combined.jar",
			},

			directResources: nil,
			directOverlays: []string{
				"out/soong/.intermediates/transitive/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import_dep/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import/android_common/package-res.apk",
				"out/soong/.intermediates/direct/android_common/aapt2/direct/res/values_strings.arsc.flat",
			},
			directImports: []string{"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk"},
			directSrcJars: []string{"out/soong/.intermediates/direct/android_common/gen/android/R.srcjar"},
			directClasspath: []string{
				"out/soong/.intermediates/default/java/android_stubs_current/android_common/turbine-combined/android_stubs_current.jar",
				"out/soong/.intermediates/transitive/android_common/turbine-combined/transitive.jar",
				"out/soong/.intermediates/transitive_import/android_common/aar/classes-combined.jar",
			},
			directCombined: []string{
				"out/soong/.intermediates/direct/android_common/javac/direct.jar",
				"out/soong/.intermediates/transitive/android_common/javac/transitive.jar",
				"out/soong/.intermediates/transitive_import/android_common/aar/classes-combined.jar",
			},

			transitiveResources: []string{"out/soong/.intermediates/transitive/android_common/aapt2/transitive/res/values_strings.arsc.flat"},
			transitiveOverlays:  nil,
			transitiveImports:   []string{"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk"},
			transitiveSrcJars:   []string{"out/soong/.intermediates/transitive/android_common/gen/android/R.srcjar"},
			transitiveClasspath: []string{"out/soong/.intermediates/default/java/android_stubs_current/android_common/turbine-combined/android_stubs_current.jar"},
			transitiveCombined:  nil,

			sharedResources: nil,
			sharedOverlays: []string{
				"out/soong/.intermediates/shared_transitive_static/android_common/package-res.apk",
				"out/soong/.intermediates/shared/android_common/aapt2/shared/res/values_strings.arsc.flat",
			},
			sharedImports: []string{
				"out/soong/.intermediates/shared_transitive_shared/android_common/package-res.apk",
				"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk",
			},
			sharedSrcJars: []string{"out/soong/.intermediates/shared/android_common/gen/android/R.srcjar"},
			sharedClasspath: []string{
				"out/soong/.intermediates/default/java/android_stubs_current/android_common/turbine-combined/android_stubs_current.jar",
				"out/soong/.intermediates/shared_transitive_shared/android_common/turbine-combined/shared_transitive_shared.jar",
				"out/soong/.intermediates/shared_transitive_static/android_common/turbine-combined/shared_transitive_static.jar",
			},
			sharedCombined: []string{
				"out/soong/.intermediates/shared/android_common/javac/shared.jar",
				"out/soong/.intermediates/shared_transitive_static/android_common/javac/shared_transitive_static.jar",
			},

			directImportResources: nil,
			directImportOverlays:  []string{"out/soong/.intermediates/direct_import/android_common/flat-res/gen_res.flata"},
			directImportImports: []string{
				"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk",
				"out/soong/.intermediates/direct_import_dep/android_common/package-res.apk",
			},

			transitiveImportResources: nil,
			transitiveImportOverlays:  []string{"out/soong/.intermediates/transitive_import/android_common/flat-res/gen_res.flata"},
			transitiveImportImports: []string{
				"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import_dep/android_common/package-res.apk",
			},
		},
		{
			// Test with all modules set to use_resource_processor: true.
			name:                            "resource_processor",
			appUsesRP:                       true,
			directLibUsesRP:                 true,
			transitiveLibUsesRP:             true,
			sharedLibUsesRP:                 true,
			sharedTransitiveSharedLibUsesRP: true,
			sharedTransitiveStaticLibUsesRP: true,

			appResources: nil,
			appOverlays: []string{
				"out/soong/.intermediates/transitive/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import_dep/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import/android_common/package-res.apk",
				"out/soong/.intermediates/direct/android_common/package-res.apk",
				"out/soong/.intermediates/direct_import_dep/android_common/package-res.apk",
				"out/soong/.intermediates/direct_import/android_common/package-res.apk",
				"out/soong/.intermediates/app/android_common/aapt2/app/res/values_strings.arsc.flat",
			},
			appImports: []string{
				"out/soong/.intermediates/shared/android_common/package-res.apk",
				"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk",
			},
			appSrcJars: nil,
			appClasspath: []string{
				"out/soong/.intermediates/default/java/android_stubs_current/android_common/turbine-combined/android_stubs_current.jar",
				"out/soong/.intermediates/app/android_common/busybox/R.jar",
				"out/soong/.intermediates/shared/android_common/turbine-combined/shared.jar",
				"out/soong/.intermediates/direct/android_common/turbine-combined/direct.jar",
				"out/soong/.intermediates/direct_import/android_common/aar/classes-combined.jar",
			},
			appCombined: []string{
				"out/soong/.intermediates/app/android_common/javac/app.jar",
				"out/soong/.intermediates/app/android_common/busybox/R.jar",
				"out/soong/.intermediates/direct/android_common/combined/direct.jar",
				"out/soong/.intermediates/direct_import/android_common/aar/classes-combined.jar",
			},

			directResources: nil,
			directOverlays:  []string{"out/soong/.intermediates/direct/android_common/aapt2/direct/res/values_strings.arsc.flat"},
			directImports: []string{
				"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import_dep/android_common/package-res.apk",
				"out/soong/.intermediates/transitive/android_common/package-res.apk",
			},
			directSrcJars: nil,
			directClasspath: []string{
				"out/soong/.intermediates/default/java/android_stubs_current/android_common/turbine-combined/android_stubs_current.jar",
				"out/soong/.intermediates/transitive_import/android_common/busybox/R.jar",
				"out/soong/.intermediates/transitive_import_dep/android_common/busybox/R.jar",
				"out/soong/.intermediates/transitive/android_common/busybox/R.jar",
				"out/soong/.intermediates/direct/android_common/busybox/R.jar",
				"out/soong/.intermediates/transitive/android_common/turbine-combined/transitive.jar",
				"out/soong/.intermediates/transitive_import/android_common/aar/classes-combined.jar",
			},
			directCombined: []string{
				"out/soong/.intermediates/direct/android_common/javac/direct.jar",
				"out/soong/.intermediates/transitive/android_common/javac/transitive.jar",
				"out/soong/.intermediates/transitive_import/android_common/aar/classes-combined.jar",
			},

			transitiveResources: []string{"out/soong/.intermediates/transitive/android_common/aapt2/transitive/res/values_strings.arsc.flat"},
			transitiveOverlays:  nil,
			transitiveImports:   []string{"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk"},
			transitiveSrcJars:   nil,
			transitiveClasspath: []string{
				"out/soong/.intermediates/default/java/android_stubs_current/android_common/turbine-combined/android_stubs_current.jar",
				"out/soong/.intermediates/transitive/android_common/busybox/R.jar",
			},
			transitiveCombined: nil,

			sharedResources: nil,
			sharedOverlays:  []string{"out/soong/.intermediates/shared/android_common/aapt2/shared/res/values_strings.arsc.flat"},
			sharedImports: []string{
				"out/soong/.intermediates/shared_transitive_shared/android_common/package-res.apk",
				"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk",
				"out/soong/.intermediates/shared_transitive_static/android_common/package-res.apk",
			},
			sharedSrcJars: nil,
			sharedClasspath: []string{
				"out/soong/.intermediates/default/java/android_stubs_current/android_common/turbine-combined/android_stubs_current.jar",
				"out/soong/.intermediates/shared_transitive_static/android_common/busybox/R.jar",
				"out/soong/.intermediates/shared_transitive_shared/android_common/busybox/R.jar",
				"out/soong/.intermediates/shared/android_common/busybox/R.jar",
				"out/soong/.intermediates/shared_transitive_shared/android_common/turbine-combined/shared_transitive_shared.jar",
				"out/soong/.intermediates/shared_transitive_static/android_common/turbine-combined/shared_transitive_static.jar",
			},
			sharedCombined: []string{
				"out/soong/.intermediates/shared/android_common/javac/shared.jar",
				"out/soong/.intermediates/shared_transitive_static/android_common/javac/shared_transitive_static.jar",
			},

			directImportResources: nil,
			directImportOverlays:  []string{"out/soong/.intermediates/direct_import/android_common/flat-res/gen_res.flata"},
			directImportImports: []string{
				"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk",
				"out/soong/.intermediates/direct_import_dep/android_common/package-res.apk",
			},

			transitiveImportResources: nil,
			transitiveImportOverlays:  []string{"out/soong/.intermediates/transitive_import/android_common/flat-res/gen_res.flata"},
			transitiveImportImports: []string{
				"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import_dep/android_common/package-res.apk",
			},
		}, {
			// Test an app building with resource processor enabled but with dependencies built without
			// resource processor.
			name:                "app_resource_processor",
			appUsesRP:           true,
			directLibUsesRP:     false,
			transitiveLibUsesRP: false,

			appResources: nil,
			appOverlays: []string{
				"out/soong/.intermediates/transitive/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import_dep/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import/android_common/package-res.apk",
				"out/soong/.intermediates/direct/android_common/package-res.apk",
				"out/soong/.intermediates/direct_import_dep/android_common/package-res.apk",
				"out/soong/.intermediates/direct_import/android_common/package-res.apk",
				"out/soong/.intermediates/app/android_common/aapt2/app/res/values_strings.arsc.flat",
			},
			appImports: []string{
				"out/soong/.intermediates/shared/android_common/package-res.apk",
				"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk",
			},
			appSrcJars: nil,
			appClasspath: []string{
				"out/soong/.intermediates/default/java/android_stubs_current/android_common/turbine-combined/android_stubs_current.jar",
				// R.jar has to come before direct.jar
				"out/soong/.intermediates/app/android_common/busybox/R.jar",
				"out/soong/.intermediates/shared/android_common/turbine-combined/shared.jar",
				"out/soong/.intermediates/direct/android_common/turbine-combined/direct.jar",
				"out/soong/.intermediates/direct_import/android_common/aar/classes-combined.jar",
			},
			appCombined: []string{
				"out/soong/.intermediates/app/android_common/javac/app.jar",
				"out/soong/.intermediates/app/android_common/busybox/R.jar",
				"out/soong/.intermediates/direct/android_common/combined/direct.jar",
				"out/soong/.intermediates/direct_import/android_common/aar/classes-combined.jar",
			},

			dontVerifyDirect:           true,
			dontVerifyTransitive:       true,
			dontVerifyShared:           true,
			dontVerifyDirectImport:     true,
			dontVerifyTransitiveImport: true,
		},
		{
			// Test an app building without resource processor enabled but with a dependency built with
			// resource processor.
			name:                "app_dependency_lib_resource_processor",
			appUsesRP:           false,
			directLibUsesRP:     true,
			transitiveLibUsesRP: false,

			appOverlays: []string{
				"out/soong/.intermediates/transitive/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import_dep/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import/android_common/package-res.apk",
				"out/soong/.intermediates/direct/android_common/package-res.apk",
				"out/soong/.intermediates/direct_import_dep/android_common/package-res.apk",
				"out/soong/.intermediates/direct_import/android_common/package-res.apk",
				"out/soong/.intermediates/app/android_common/aapt2/app/res/values_strings.arsc.flat",
			},
			appImports: []string{
				"out/soong/.intermediates/shared/android_common/package-res.apk",
				"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk",
			},
			appSrcJars: []string{"out/soong/.intermediates/app/android_common/gen/android/R.srcjar"},
			appClasspath: []string{
				"out/soong/.intermediates/default/java/android_stubs_current/android_common/turbine-combined/android_stubs_current.jar",
				"out/soong/.intermediates/shared/android_common/turbine-combined/shared.jar",
				"out/soong/.intermediates/direct/android_common/turbine-combined/direct.jar",
				"out/soong/.intermediates/direct_import/android_common/aar/classes-combined.jar",
			},
			appCombined: []string{
				"out/soong/.intermediates/app/android_common/javac/app.jar",
				"out/soong/.intermediates/direct/android_common/combined/direct.jar",
				"out/soong/.intermediates/direct_import/android_common/aar/classes-combined.jar",
			},

			directResources: nil,
			directOverlays:  []string{"out/soong/.intermediates/direct/android_common/aapt2/direct/res/values_strings.arsc.flat"},
			directImports: []string{
				"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import_dep/android_common/package-res.apk",
				"out/soong/.intermediates/transitive/android_common/package-res.apk",
			},
			directSrcJars: nil,
			directClasspath: []string{
				"out/soong/.intermediates/default/java/android_stubs_current/android_common/turbine-combined/android_stubs_current.jar",
				"out/soong/.intermediates/transitive_import/android_common/busybox/R.jar",
				"out/soong/.intermediates/transitive_import_dep/android_common/busybox/R.jar",
				"out/soong/.intermediates/direct/android_common/busybox/R.jar",
				"out/soong/.intermediates/transitive/android_common/turbine-combined/transitive.jar",
				"out/soong/.intermediates/transitive_import/android_common/aar/classes-combined.jar",
			},
			directCombined: []string{
				"out/soong/.intermediates/direct/android_common/javac/direct.jar",
				"out/soong/.intermediates/transitive/android_common/javac/transitive.jar",
				"out/soong/.intermediates/transitive_import/android_common/aar/classes-combined.jar",
			},

			dontVerifyTransitive:       true,
			dontVerifyShared:           true,
			dontVerifyDirectImport:     true,
			dontVerifyTransitiveImport: true,
		},
		{
			// Test a library building without resource processor enabled but with a dependency built with
			// resource processor.
			name:                "lib_dependency_lib_resource_processor",
			appUsesRP:           false,
			directLibUsesRP:     false,
			transitiveLibUsesRP: true,

			appOverlays: []string{
				"out/soong/.intermediates/transitive/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import_dep/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import/android_common/package-res.apk",
				"out/soong/.intermediates/direct/android_common/package-res.apk",
				"out/soong/.intermediates/direct_import_dep/android_common/package-res.apk",
				"out/soong/.intermediates/direct_import/android_common/package-res.apk",
				"out/soong/.intermediates/app/android_common/aapt2/app/res/values_strings.arsc.flat",
			},
			appImports: []string{
				"out/soong/.intermediates/shared/android_common/package-res.apk",
				"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk",
			},
			appSrcJars: []string{"out/soong/.intermediates/app/android_common/gen/android/R.srcjar"},
			appClasspath: []string{
				"out/soong/.intermediates/default/java/android_stubs_current/android_common/turbine-combined/android_stubs_current.jar",
				"out/soong/.intermediates/shared/android_common/turbine-combined/shared.jar",
				"out/soong/.intermediates/direct/android_common/turbine-combined/direct.jar",
				"out/soong/.intermediates/direct_import/android_common/aar/classes-combined.jar",
			},
			appCombined: []string{
				"out/soong/.intermediates/app/android_common/javac/app.jar",
				"out/soong/.intermediates/direct/android_common/combined/direct.jar",
				"out/soong/.intermediates/direct_import/android_common/aar/classes-combined.jar",
			},

			directResources: nil,
			directOverlays: []string{
				"out/soong/.intermediates/transitive/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import_dep/android_common/package-res.apk",
				"out/soong/.intermediates/transitive_import/android_common/package-res.apk",
				"out/soong/.intermediates/direct/android_common/aapt2/direct/res/values_strings.arsc.flat",
			},
			directImports: []string{"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk"},
			directSrcJars: []string{"out/soong/.intermediates/direct/android_common/gen/android/R.srcjar"},
			directClasspath: []string{
				"out/soong/.intermediates/default/java/android_stubs_current/android_common/turbine-combined/android_stubs_current.jar",
				"out/soong/.intermediates/transitive/android_common/turbine-combined/transitive.jar",
				"out/soong/.intermediates/transitive_import/android_common/aar/classes-combined.jar",
			},
			directCombined: []string{
				"out/soong/.intermediates/direct/android_common/javac/direct.jar",
				"out/soong/.intermediates/transitive/android_common/javac/transitive.jar",
				"out/soong/.intermediates/transitive_import/android_common/aar/classes-combined.jar",
			},

			transitiveResources: []string{"out/soong/.intermediates/transitive/android_common/aapt2/transitive/res/values_strings.arsc.flat"},
			transitiveOverlays:  nil,
			transitiveImports:   []string{"out/soong/.intermediates/default/java/framework-res/android_common/package-res.apk"},
			transitiveSrcJars:   nil,
			transitiveClasspath: []string{
				"out/soong/.intermediates/default/java/android_stubs_current/android_common/turbine-combined/android_stubs_current.jar",
				"out/soong/.intermediates/transitive/android_common/busybox/R.jar",
			},
			transitiveCombined: nil,

			dontVerifyShared:           true,
			dontVerifyDirectImport:     true,
			dontVerifyTransitiveImport: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			bp := fmt.Sprintf(`
				android_app {
					name: "app",
					sdk_version: "current",
					srcs: ["app/app.java"],
					resource_dirs: ["app/res"],
					manifest: "app/AndroidManifest.xml",
					libs: ["shared"],
					static_libs: ["direct", "direct_import"],
					use_resource_processor: %v,
				}

				android_library {
					name: "direct",
					sdk_version: "current",
					srcs: ["direct/direct.java"],
					resource_dirs: ["direct/res"],
					manifest: "direct/AndroidManifest.xml",
					static_libs: ["transitive", "transitive_import"],
					use_resource_processor: %v,
				}

				android_library {
					name: "transitive",
					sdk_version: "current",
					srcs: ["transitive/transitive.java"],
					resource_dirs: ["transitive/res"],
					manifest: "transitive/AndroidManifest.xml",
					use_resource_processor: %v,
				}

				android_library {
					name: "shared",
					sdk_version: "current",
					srcs: ["shared/shared.java"],
					resource_dirs: ["shared/res"],
					manifest: "shared/AndroidManifest.xml",
					use_resource_processor: %v,
					libs: ["shared_transitive_shared"],
					static_libs: ["shared_transitive_static"],
				}

				android_library {
					name: "shared_transitive_shared",
					sdk_version: "current",
					srcs: ["shared_transitive_shared/shared_transitive_shared.java"],
					resource_dirs: ["shared_transitive_shared/res"],
					manifest: "shared_transitive_shared/AndroidManifest.xml",
					use_resource_processor: %v,
				}

				android_library {
					name: "shared_transitive_static",
					sdk_version: "current",
					srcs: ["shared_transitive_static/shared.java"],
					resource_dirs: ["shared_transitive_static/res"],
					manifest: "shared_transitive_static/AndroidManifest.xml",
					use_resource_processor: %v,
				}

				android_library_import {
					name: "direct_import",
					sdk_version: "current",
					aars: ["direct_import.aar"],
					static_libs: ["direct_import_dep"],
				}

				android_library_import {
					name: "direct_import_dep",
					sdk_version: "current",
					aars: ["direct_import_dep.aar"],
				}

				android_library_import {
					name: "transitive_import",
					sdk_version: "current",
					aars: ["transitive_import.aar"],
					static_libs: ["transitive_import_dep"],
				}

				android_library_import {
					name: "transitive_import_dep",
					sdk_version: "current",
					aars: ["transitive_import_dep.aar"],
				}
			`, testCase.appUsesRP, testCase.directLibUsesRP, testCase.transitiveLibUsesRP,
				testCase.sharedLibUsesRP, testCase.sharedTransitiveSharedLibUsesRP, testCase.sharedTransitiveStaticLibUsesRP)

			fs := android.MockFS{
				"app/res/values/strings.xml":                      nil,
				"direct/res/values/strings.xml":                   nil,
				"transitive/res/values/strings.xml":               nil,
				"shared/res/values/strings.xml":                   nil,
				"shared_transitive_static/res/values/strings.xml": nil,
				"shared_transitive_shared/res/values/strings.xml": nil,
			}

			result := android.GroupFixturePreparers(
				PrepareForTestWithJavaDefaultModules,
				fs.AddToFixture(),
			).RunTestWithBp(t, bp)

			type aaptInfo struct {
				resources, overlays, imports, srcJars, classpath, combined android.Paths
			}

			getAaptInfo := func(moduleName string) (aaptInfo aaptInfo) {
				mod := result.ModuleForTests(moduleName, "android_common")
				resourceListRule := mod.MaybeOutput("aapt2/res.list")
				overlayListRule := mod.MaybeOutput("aapt2/overlay.list")
				aaptRule := mod.Rule("aapt2Link")
				javacRule := mod.MaybeRule("javac")
				combinedRule := mod.MaybeOutput("combined/" + moduleName + ".jar")

				aaptInfo.resources = resourceListRule.Inputs
				aaptInfo.overlays = overlayListRule.Inputs

				aaptFlags := strings.Split(aaptRule.Args["flags"], " ")
				for i, flag := range aaptFlags {
					if flag == "-I" && i+1 < len(aaptFlags) {
						aaptInfo.imports = append(aaptInfo.imports, android.PathForTesting(aaptFlags[i+1]))
					}
				}

				if len(javacRule.Args["srcJars"]) > 0 {
					aaptInfo.srcJars = android.PathsForTesting(strings.Split(javacRule.Args["srcJars"], " ")...)
				}

				if len(javacRule.Args["classpath"]) > 0 {
					classpathArg := strings.TrimPrefix(javacRule.Args["classpath"], "-classpath ")
					aaptInfo.classpath = android.PathsForTesting(strings.Split(classpathArg, ":")...)
				}

				aaptInfo.combined = combinedRule.Inputs
				return
			}

			app := getAaptInfo("app")
			direct := getAaptInfo("direct")
			transitive := getAaptInfo("transitive")
			shared := getAaptInfo("shared")
			directImport := getAaptInfo("direct_import")
			transitiveImport := getAaptInfo("transitive_import")

			if !testCase.dontVerifyApp {
				android.AssertPathsRelativeToTopEquals(t, "app resources", testCase.appResources, app.resources)
				android.AssertPathsRelativeToTopEquals(t, "app overlays", testCase.appOverlays, app.overlays)
				android.AssertPathsRelativeToTopEquals(t, "app imports", testCase.appImports, app.imports)
				android.AssertPathsRelativeToTopEquals(t, "app srcjars", testCase.appSrcJars, app.srcJars)
				android.AssertPathsRelativeToTopEquals(t, "app classpath", testCase.appClasspath, app.classpath)
				android.AssertPathsRelativeToTopEquals(t, "app combined", testCase.appCombined, app.combined)
			}

			if !testCase.dontVerifyDirect {
				android.AssertPathsRelativeToTopEquals(t, "direct resources", testCase.directResources, direct.resources)
				android.AssertPathsRelativeToTopEquals(t, "direct overlays", testCase.directOverlays, direct.overlays)
				android.AssertPathsRelativeToTopEquals(t, "direct imports", testCase.directImports, direct.imports)
				android.AssertPathsRelativeToTopEquals(t, "direct srcjars", testCase.directSrcJars, direct.srcJars)
				android.AssertPathsRelativeToTopEquals(t, "direct classpath", testCase.directClasspath, direct.classpath)
				android.AssertPathsRelativeToTopEquals(t, "direct combined", testCase.directCombined, direct.combined)
			}

			if !testCase.dontVerifyTransitive {
				android.AssertPathsRelativeToTopEquals(t, "transitive resources", testCase.transitiveResources, transitive.resources)
				android.AssertPathsRelativeToTopEquals(t, "transitive overlays", testCase.transitiveOverlays, transitive.overlays)
				android.AssertPathsRelativeToTopEquals(t, "transitive imports", testCase.transitiveImports, transitive.imports)
				android.AssertPathsRelativeToTopEquals(t, "transitive srcjars", testCase.transitiveSrcJars, transitive.srcJars)
				android.AssertPathsRelativeToTopEquals(t, "transitive classpath", testCase.transitiveClasspath, transitive.classpath)
				android.AssertPathsRelativeToTopEquals(t, "transitive combined", testCase.transitiveCombined, transitive.combined)
			}

			if !testCase.dontVerifyShared {
				android.AssertPathsRelativeToTopEquals(t, "shared resources", testCase.sharedResources, shared.resources)
				android.AssertPathsRelativeToTopEquals(t, "shared overlays", testCase.sharedOverlays, shared.overlays)
				android.AssertPathsRelativeToTopEquals(t, "shared imports", testCase.sharedImports, shared.imports)
				android.AssertPathsRelativeToTopEquals(t, "shared srcjars", testCase.sharedSrcJars, shared.srcJars)
				android.AssertPathsRelativeToTopEquals(t, "shared classpath", testCase.sharedClasspath, shared.classpath)
				android.AssertPathsRelativeToTopEquals(t, "shared combined", testCase.sharedCombined, shared.combined)
			}

			if !testCase.dontVerifyDirectImport {
				android.AssertPathsRelativeToTopEquals(t, "direct_import resources", testCase.directImportResources, directImport.resources)
				android.AssertPathsRelativeToTopEquals(t, "direct_import overlays", testCase.directImportOverlays, directImport.overlays)
				android.AssertPathsRelativeToTopEquals(t, "direct_import imports", testCase.directImportImports, directImport.imports)
			}

			if !testCase.dontVerifyTransitiveImport {
				android.AssertPathsRelativeToTopEquals(t, "transitive_import resources", testCase.transitiveImportResources, transitiveImport.resources)
				android.AssertPathsRelativeToTopEquals(t, "transitive_import overlays", testCase.transitiveImportOverlays, transitiveImport.overlays)
				android.AssertPathsRelativeToTopEquals(t, "transitive_import imports", testCase.transitiveImportImports, transitiveImport.imports)
			}
		})
	}
}

func TestAndroidResourceOverlays(t *testing.T) {
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
					"out/soong/.intermediates/lib2/android_common/package-res.apk",
					"out/soong/.intermediates/lib/android_common/package-res.apk",
					"out/soong/.intermediates/lib3/android_common/package-res.apk",
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
					"out/soong/.intermediates/lib2/android_common/package-res.apk",
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
					"out/soong/.intermediates/lib2/android_common/package-res.apk",
					"out/soong/.intermediates/lib/android_common/package-res.apk",
					"out/soong/.intermediates/lib3/android_common/package-res.apk",
					"foo/res/res/values/strings.xml",
					"device/vendor/blah/static_overlay/foo/res/values/strings.xml",
				},
				"bar": {
					"device/vendor/blah/static_overlay/bar/res/values/strings.xml",
					"device/vendor/blah/overlay/bar/res/values/strings.xml",
				},
				"lib": {
					"out/soong/.intermediates/lib2/android_common/package-res.apk",
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
					"out/soong/.intermediates/lib2/android_common/package-res.apk",
					"out/soong/.intermediates/lib/android_common/package-res.apk",
					"out/soong/.intermediates/lib3/android_common/package-res.apk",
					"foo/res/res/values/strings.xml",
					"device/vendor/blah/static_overlay/foo/res/values/strings.xml",
				},
				"bar": {"device/vendor/blah/static_overlay/bar/res/values/strings.xml"},
				"lib": {
					"out/soong/.intermediates/lib2/android_common/package-res.apk",
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

	fs := android.MockFS{
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
			result := android.GroupFixturePreparers(
				PrepareForTestWithJavaDefaultModules,
				fs.AddToFixture(),
				android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
					variables.DeviceResourceOverlays = deviceResourceOverlays
					variables.ProductResourceOverlays = productResourceOverlays
					if testCase.enforceRROTargets != nil {
						variables.EnforceRROTargets = testCase.enforceRROTargets
					}
					if testCase.enforceRROExcludedOverlays != nil {
						variables.EnforceRROExcludedOverlays = testCase.enforceRROExcludedOverlays
					}
				}),
			).RunTestWithBp(t, bp)

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
				module := result.ModuleForTests(moduleName, "android_common")
				resourceList := module.MaybeOutput("aapt2/res.list")
				if resourceList.Rule != nil {
					resourceFiles = resourceListToFiles(module, android.PathsRelativeToTop(resourceList.Inputs))
				}
				overlayList := module.MaybeOutput("aapt2/overlay.list")
				if overlayList.Rule != nil {
					overlayFiles = resourceListToFiles(module, android.PathsRelativeToTop(overlayList.Inputs))
				}

				for _, d := range module.Module().(AndroidLibraryDependency).RRODirsDepSet().ToList() {
					var prefix string
					if d.overlayType == device {
						prefix = "device:"
					} else if d.overlayType == product {
						prefix = "product:"
					} else {
						t.Fatalf("Unexpected overlayType %d", d.overlayType)
					}
					rroDirs = append(rroDirs, prefix+android.PathRelativeToTop(d.path))
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
		minSdkVersionBp       string
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
		{
			name:                  "two active SDKs",
			sdkVersion:            "module_current",
			minSdkVersionBp:       "UpsideDownCake",
			expectedMinSdkVersion: "UpsideDownCake", // And not VanillaIceCream
			platformSdkCodename:   "VanillaIceCream",
			activeCodenames:       []string{"UpsideDownCake", "VanillaIceCream"},
		},
	}

	for _, moduleType := range []string{"android_app", "android_library"} {
		for _, test := range testCases {
			t.Run(moduleType+" "+test.name, func(t *testing.T) {
				platformApiProp := ""
				if test.platformApis {
					platformApiProp = "platform_apis: true,"
				}
				minSdkVersionProp := ""
				if test.minSdkVersionBp != "" {
					minSdkVersionProp = fmt.Sprintf(` min_sdk_version: "%s",`, test.minSdkVersionBp)
				}
				bp := fmt.Sprintf(`%s {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "%s",
					%s
					%s
				}`, moduleType, test.sdkVersion, platformApiProp, minSdkVersionProp)

				result := android.GroupFixturePreparers(
					prepareForJavaTest,
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

					result := android.GroupFixturePreparers(
						prepareForJavaTest,
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

		errorHandler := android.FixtureExpectsNoErrors
		if enforce {
			errorHandler = android.FixtureExpectsAtLeastOneErrorMatchingPattern("sdk_version must have a value when the module is located at vendor or product")
		}

		android.GroupFixturePreparers(
			PrepareForTestWithJavaDefaultModules,
			android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				variables.EnforceProductPartitionInterface = proptools.BoolPtr(enforce)
			}),
		).
			ExtendWithErrorHandler(errorHandler).
			RunTestWithBp(t, bp)
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
	vendorJNI := ctx.ModuleForTests("libvendorjni", "android_vendor_arm64_armv8-a_shared").
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
		name                     string
		bp                       string
		allowMissingDependencies bool
		certificateOverride      string
		expectedCertSigningFlags string
		expectedCertificate      string
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
			certificateOverride:      "",
			expectedCertSigningFlags: "",
			expectedCertificate:      "build/make/target/product/security/testkey",
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
			certificateOverride:      "",
			expectedCertSigningFlags: "",
			expectedCertificate:      "cert/new_cert",
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
			certificateOverride:      "",
			expectedCertSigningFlags: "",
			expectedCertificate:      "build/make/target/product/security/expiredkey",
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
			certificateOverride:      "foo:new_certificate",
			expectedCertSigningFlags: "",
			expectedCertificate:      "cert/new_cert",
		},
		{
			name: "certificate signing flags",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					certificate: ":new_certificate",
					lineage: "lineage.bin",
					rotationMinSdkVersion: "32",
					sdk_version: "current",
				}

				android_app_certificate {
					name: "new_certificate",
					certificate: "cert/new_cert",
				}
			`,
			certificateOverride:      "",
			expectedCertSigningFlags: "--lineage lineage.bin --rotation-min-sdk-version 32",
			expectedCertificate:      "cert/new_cert",
		},
		{
			name: "cert signing flags from filegroup",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					certificate: ":new_certificate",
					lineage: ":lineage_bin",
					rotationMinSdkVersion: "32",
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
			certificateOverride:      "",
			expectedCertSigningFlags: "--lineage lineage.bin --rotation-min-sdk-version 32",
			expectedCertificate:      "cert/new_cert",
		},
		{
			name: "missing with AllowMissingDependencies",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					certificate: ":new_certificate",
					sdk_version: "current",
				}
			`,
			expectedCertificate:      "out/soong/.intermediates/foo/android_common/missing",
			allowMissingDependencies: true,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := android.GroupFixturePreparers(
				PrepareForTestWithJavaDefaultModules,
				android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
					if test.certificateOverride != "" {
						variables.CertificateOverrides = []string{test.certificateOverride}
					}
					if test.allowMissingDependencies {
						variables.Allow_missing_dependencies = proptools.BoolPtr(true)
					}
				}),
				android.FixtureModifyContext(func(ctx *android.TestContext) {
					ctx.SetAllowMissingDependencies(test.allowMissingDependencies)
				}),
			).RunTestWithBp(t, test.bp)

			foo := result.ModuleForTests("foo", "android_common")

			certificate := foo.Module().(*AndroidApp).certificate
			android.AssertPathRelativeToTopEquals(t, "certificates key", test.expectedCertificate+".pk8", certificate.Key)
			// The sign_target_files_apks and check_target_files_signatures
			// tools require that certificates have a .x509.pem extension.
			android.AssertPathRelativeToTopEquals(t, "certificates pem", test.expectedCertificate+".x509.pem", certificate.Pem)

			signapk := foo.Output("foo.apk")
			if signapk.Rule != android.ErrorRule {
				signCertificateFlags := signapk.Args["certificates"]
				expectedFlags := certificate.Pem.String() + " " + certificate.Key.String()
				android.AssertStringEquals(t, "certificates flags", expectedFlags, signCertificateFlags)

				certSigningFlags := signapk.Args["flags"]
				android.AssertStringEquals(t, "cert signing flags", test.expectedCertSigningFlags, certSigningFlags)
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
			result := android.GroupFixturePreparers(
				PrepareForTestWithJavaDefaultModules,
			).RunTestWithBp(t, test.bp)

			foo := result.ModuleForTests("foo", "android_common")

			signapk := foo.Output("foo.apk")
			signFlags := signapk.Args["flags"]
			android.AssertStringEquals(t, "signing flags", test.expected, signFlags)
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
				"out/soong/.intermediates/foo/android_common/foo.apk",
				"out/soong/target/product/test_device/system/app/foo/foo.apk",
			},
		},
		{
			name: "overridden via PRODUCT_PACKAGE_NAME_OVERRIDES",
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
				"out/soong/.intermediates/foo/android_common/bar.apk",
				"out/soong/target/product/test_device/system/app/bar/bar.apk",
			},
		},
		{
			name: "overridden via stem",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "current",
					stem: "bar",
				}
			`,
			packageNameOverride: "",
			expected: []string{
				"out/soong/.intermediates/foo/android_common/bar.apk",
				"out/soong/target/product/test_device/system/app/bar/bar.apk",
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := android.GroupFixturePreparers(
				PrepareForTestWithJavaDefaultModules,
				android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
					if test.packageNameOverride != "" {
						variables.PackageNameOverrides = []string{test.packageNameOverride}
					}
				}),
			).RunTestWithBp(t, test.bp)

			foo := result.ModuleForTests("foo", "android_common")

			outSoongDir := result.Config.SoongOutDir()

			outputs := foo.AllOutputs()
			outputMap := make(map[string]bool)
			for _, o := range outputs {
				outputMap[android.StringPathRelativeToTop(outSoongDir, o)] = true
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

	result := android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.ManifestPackageNameOverrides = []string{"foo:org.dandroid.bp"}
		}),
	).RunTestWithBp(t, bp)

	bar := result.ModuleForTests("bar", "android_common")
	res := bar.Output("package-res.apk")
	aapt2Flags := res.Args["flags"]
	e := "--rename-instrumentation-target-package org.dandroid.bp"
	if !strings.Contains(aapt2Flags, e) {
		t.Errorf("target package renaming flag, %q is missing in aapt2 link flags, %q", e, aapt2Flags)
	}
}

func TestOverrideAndroidApp(t *testing.T) {
	result := PrepareForTestWithJavaDefaultModules.RunTestWithBp(
		t, `
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
			rotationMinSdkVersion: "32",
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
		name             string
		moduleName       string
		variantName      string
		apkName          string
		apkPath          string
		certFlag         string
		certSigningFlags string
		overrides        []string
		packageFlag      string
		renameResources  bool
		logging_parent   string
	}{
		{
			name:             "foo",
			moduleName:       "foo",
			variantName:      "android_common",
			apkPath:          "out/soong/target/product/test_device/system/app/foo/foo.apk",
			certFlag:         "build/make/target/product/security/expiredkey.x509.pem build/make/target/product/security/expiredkey.pk8",
			certSigningFlags: "",
			overrides:        []string{"qux"},
			packageFlag:      "",
			renameResources:  false,
			logging_parent:   "",
		},
		{
			name:             "foo",
			moduleName:       "bar",
			variantName:      "android_common_bar",
			apkPath:          "out/soong/target/product/test_device/system/app/bar/bar.apk",
			certFlag:         "cert/new_cert.x509.pem cert/new_cert.pk8",
			certSigningFlags: "--lineage lineage.bin --rotation-min-sdk-version 32",
			overrides:        []string{"qux", "foo"},
			packageFlag:      "",
			renameResources:  false,
			logging_parent:   "bah",
		},
		{
			name:             "foo",
			moduleName:       "baz",
			variantName:      "android_common_baz",
			apkPath:          "out/soong/target/product/test_device/system/app/baz/baz.apk",
			certFlag:         "build/make/target/product/security/expiredkey.x509.pem build/make/target/product/security/expiredkey.pk8",
			certSigningFlags: "",
			overrides:        []string{"qux", "foo"},
			packageFlag:      "org.dandroid.bp",
			renameResources:  true,
			logging_parent:   "",
		},
		{
			name:             "foo",
			moduleName:       "baz_no_rename_resources",
			variantName:      "android_common_baz_no_rename_resources",
			apkPath:          "out/soong/target/product/test_device/system/app/baz_no_rename_resources/baz_no_rename_resources.apk",
			certFlag:         "build/make/target/product/security/expiredkey.x509.pem build/make/target/product/security/expiredkey.pk8",
			certSigningFlags: "",
			overrides:        []string{"qux", "foo"},
			packageFlag:      "org.dandroid.bp",
			renameResources:  false,
			logging_parent:   "",
		},
		{
			name:             "foo_no_rename_resources",
			moduleName:       "baz_base_no_rename_resources",
			variantName:      "android_common_baz_base_no_rename_resources",
			apkPath:          "out/soong/target/product/test_device/system/app/baz_base_no_rename_resources/baz_base_no_rename_resources.apk",
			certFlag:         "build/make/target/product/security/expiredkey.x509.pem build/make/target/product/security/expiredkey.pk8",
			certSigningFlags: "",
			overrides:        []string{"qux", "foo_no_rename_resources"},
			packageFlag:      "org.dandroid.bp",
			renameResources:  false,
			logging_parent:   "",
		},
		{
			name:             "foo_no_rename_resources",
			moduleName:       "baz_override_base_rename_resources",
			variantName:      "android_common_baz_override_base_rename_resources",
			apkPath:          "out/soong/target/product/test_device/system/app/baz_override_base_rename_resources/baz_override_base_rename_resources.apk",
			certFlag:         "build/make/target/product/security/expiredkey.x509.pem build/make/target/product/security/expiredkey.pk8",
			certSigningFlags: "",
			overrides:        []string{"qux", "foo_no_rename_resources"},
			packageFlag:      "org.dandroid.bp",
			renameResources:  true,
			logging_parent:   "",
		},
	}
	for _, expected := range expectedVariants {
		variant := result.ModuleForTests(expected.name, expected.variantName)

		// Check the final apk name
		variant.Output(expected.apkPath)

		// Check the certificate paths
		signapk := variant.Output(expected.moduleName + ".apk")
		certFlag := signapk.Args["certificates"]
		android.AssertStringEquals(t, "certificates flags", expected.certFlag, certFlag)

		// Check the cert signing flags
		certSigningFlags := signapk.Args["flags"]
		android.AssertStringEquals(t, "cert signing flags", expected.certSigningFlags, certSigningFlags)

		// Check if the overrides field values are correctly aggregated.
		mod := variant.Module().(*AndroidApp)
		android.AssertDeepEquals(t, "overrides property", expected.overrides, mod.overridableAppProperties.Overrides)

		// Test Overridable property: Logging_parent
		logging_parent := mod.aapt.LoggingParent
		android.AssertStringEquals(t, "overrides property value for logging parent", expected.logging_parent, logging_parent)

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

func TestOverrideAndroidAppOverrides(t *testing.T) {
	ctx, _ := testJava(
		t, `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			sdk_version: "current",
			overrides: ["qux"]
		}

		android_app {
			name: "bar",
			srcs: ["b.java"],
			sdk_version: "current",
			overrides: ["foo"]
		}

		override_android_app {
			name: "foo_override",
			base: "foo",
			overrides: ["bar"]
		}
		`)

	expectedVariants := []struct {
		name        string
		moduleName  string
		variantName string
		overrides   []string
	}{
		{
			name:        "foo",
			moduleName:  "foo",
			variantName: "android_common",
			overrides:   []string{"qux"},
		},
		{
			name:        "bar",
			moduleName:  "bar",
			variantName: "android_common",
			overrides:   []string{"foo"},
		},
		{
			name:        "foo",
			moduleName:  "foo_override",
			variantName: "android_common_foo_override",
			overrides:   []string{"bar", "foo"},
		},
	}
	for _, expected := range expectedVariants {
		variant := ctx.ModuleForTests(expected.name, expected.variantName)

		// Check if the overrides field values are correctly aggregated.
		mod := variant.Module().(*AndroidApp)
		android.AssertDeepEquals(t, "overrides property", expected.overrides, mod.overridableAppProperties.Overrides)
	}
}

func TestOverrideAndroidAppWithPrebuilt(t *testing.T) {
	result := PrepareForTestWithJavaDefaultModules.RunTestWithBp(
		t, `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			sdk_version: "current",
		}

		override_android_app {
			name: "bar",
			base: "foo",
		}

		android_app_import {
			name: "bar",
			prefer: true,
			apk: "bar.apk",
			presigned: true,
		}
		`)

	// An app that has an override that also has a prebuilt should not be hidden.
	foo := result.ModuleForTests("foo", "android_common")
	if foo.Module().IsHideFromMake() {
		t.Errorf("expected foo to have HideFromMake false")
	}

	// An override that also has a prebuilt should be hidden.
	barOverride := result.ModuleForTests("foo", "android_common_bar")
	if !barOverride.Module().IsHideFromMake() {
		t.Errorf("expected bar override variant of foo to have HideFromMake true")
	}
}

func TestOverrideAndroidAppStem(t *testing.T) {
	ctx, _ := testJava(t, `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			sdk_version: "current",
		}
		override_android_app {
			name: "bar",
			base: "foo",
		}
		override_android_app {
			name: "baz",
			base: "foo",
			stem: "baz_stem",
		}
		android_app {
			name: "foo2",
			srcs: ["a.java"],
			sdk_version: "current",
			stem: "foo2_stem",
		}
		override_android_app {
			name: "bar2",
			base: "foo2",
		}
		override_android_app {
			name: "baz2",
			base: "foo2",
			stem: "baz2_stem",
		}
	`)
	for _, expected := range []struct {
		moduleName  string
		variantName string
		apkPath     string
	}{
		{
			moduleName:  "foo",
			variantName: "android_common",
			apkPath:     "out/soong/target/product/test_device/system/app/foo/foo.apk",
		},
		{
			moduleName:  "foo",
			variantName: "android_common_bar",
			apkPath:     "out/soong/target/product/test_device/system/app/bar/bar.apk",
		},
		{
			moduleName:  "foo",
			variantName: "android_common_baz",
			apkPath:     "out/soong/target/product/test_device/system/app/baz_stem/baz_stem.apk",
		},
		{
			moduleName:  "foo2",
			variantName: "android_common",
			apkPath:     "out/soong/target/product/test_device/system/app/foo2_stem/foo2_stem.apk",
		},
		{
			moduleName:  "foo2",
			variantName: "android_common_bar2",
			// Note that this may cause the duplicate output error.
			apkPath: "out/soong/target/product/test_device/system/app/foo2_stem/foo2_stem.apk",
		},
		{
			moduleName:  "foo2",
			variantName: "android_common_baz2",
			apkPath:     "out/soong/target/product/test_device/system/app/baz2_stem/baz2_stem.apk",
		},
	} {
		variant := ctx.ModuleForTests(expected.moduleName, expected.variantName)
		variant.Output(expected.apkPath)
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
	fooTurbine := "out/soong/.intermediates/foo/android_common/turbine-combined/foo.jar"
	if !strings.Contains(javac.Args["classpath"], fooTurbine) {
		t.Errorf("baz classpath %v does not contain %q", javac.Args["classpath"], fooTurbine)
	}

	// Verify qux, which depends on the overriding module bar, has the correct classpath javac arg.
	javac = ctx.ModuleForTests("qux", "android_common").Rule("javac")
	barTurbine := "out/soong/.intermediates/foo/android_common_bar/turbine-combined/foo.jar"
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
		variant.Output("out/soong" + expected.apkPath)

		// Check if the overrides field values are correctly aggregated.
		mod := variant.Module().(*AndroidTest)
		if !reflect.DeepEqual(expected.overrides, mod.overridableAppProperties.Overrides) {
			t.Errorf("Incorrect overrides property value, expected: %q, got: %q",
				expected.overrides, mod.overridableAppProperties.Overrides)
		}

		// Check if javac classpath has the correct jar file path. This checks instrumentation_for overrides.
		javac := variant.Rule("javac")
		turbine := filepath.Join("out", "soong", ".intermediates", "foo", expected.targetVariant, "turbine-combined", "foo.jar")
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
			mainline_package_name: "com.android.bar",
		}

		override_android_test {
			name: "baz_test",
			base: "foo_test",
			package_name: "com.android.baz.test",
			mainline_package_name: "com.android.baz",
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
				"--manifest out/soong/.intermediates/bar_test/android_common/manifest_fixer/AndroidManifest.xml",
				"--package-name com.android.bar.test",
				"--mainline-package-name com.android.bar",
			},
		},
		{
			moduleName:  "foo_test",
			variantName: "android_common_baz_test",
			expectedFlags: []string{
				"--manifest out/soong/.intermediates/foo_test/android_common_baz_test/manifest_fixer/AndroidManifest.xml",
				"--package-name com.android.baz.test",
				"--test-file-name baz_test.apk",
				"out/soong/.intermediates/foo_test/android_common_baz_test/test_config_fixer/AndroidTest.xml",
				"--mainline-package-name com.android.baz",
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

func TestInstrumentationTargetPrebuilt(t *testing.T) {
	bp := `
		android_app_import {
			name: "foo",
			apk: "foo.apk",
			presigned: true,
		}

		android_test {
			name: "bar",
			srcs: ["a.java"],
			instrumentation_for: "foo",
			sdk_version: "current",
		}
		`

	android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
	).ExtendWithErrorHandler(
		android.FixtureExpectsAtLeastOneErrorMatchingPattern(
			"instrumentation_for: dependency \"foo\" of type \"android_app_import\" does not provide JavaInfo so is unsuitable for use with this property")).
		RunTestWithBp(t, bp)
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

		java_library {
			name: "runtime-required-x",
			srcs: ["a.java"],
			installable: true,
			sdk_version: "current",
		}

		java_library {
			name: "runtime-optional-x",
			srcs: ["a.java"],
			installable: true,
			sdk_version: "current",
		}

		android_library {
			name: "static-x",
			uses_libs: ["runtime-required-x"],
			optional_uses_libs: ["runtime-optional-x"],
			sdk_version: "current",
		}

		java_library {
			name: "runtime-required-y",
			srcs: ["a.java"],
			installable: true,
			sdk_version: "current",
		}

		java_library {
			name: "runtime-optional-y",
			srcs: ["a.java"],
			installable: true,
			sdk_version: "current",
		}

		java_library {
			name: "static-y",
			srcs: ["a.java"],
			uses_libs: ["runtime-required-y"],
			optional_uses_libs: ["runtime-optional-y"],
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
				"static-x",
				"static-y",
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

	result := android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("runtime-library", "foo", "quuz", "qux", "bar", "fred"),
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.BuildWarningBadOptionalUsesLibsAllowlist = []string{"app", "prebuilt"}
		}),
	).RunTestWithBp(t, bp)

	app := result.ModuleForTests("app", "android_common")
	prebuilt := result.ModuleForTests("prebuilt", "android_common")

	// Test that implicit dependencies on java_sdk_library instances are passed to the manifest.
	// These also include explicit `uses_libs`/`optional_uses_libs` entries, as they may be
	// propagated from dependencies.
	actualManifestFixerArgs := app.Output("manifest_fixer/AndroidManifest.xml").Args["args"]
	expectManifestFixerArgs := `--extract-native-libs=true ` +
		`--uses-library qux ` +
		`--uses-library quuz ` +
		`--uses-library foo ` +
		`--uses-library com.non.sdk.lib ` +
		`--uses-library runtime-library ` +
		`--uses-library runtime-required-x ` +
		`--uses-library runtime-required-y ` +
		`--optional-uses-library bar ` +
		`--optional-uses-library runtime-optional-x ` +
		`--optional-uses-library runtime-optional-y`
	android.AssertStringDoesContain(t, "manifest_fixer args", actualManifestFixerArgs, expectManifestFixerArgs)

	// Test that all libraries are verified (library order matters).
	verifyCmd := app.Rule("verify_uses_libraries").RuleParams.Command
	verifyArgs := `--uses-library foo ` +
		`--uses-library com.non.sdk.lib ` +
		`--uses-library qux ` +
		`--uses-library quuz ` +
		`--uses-library runtime-library ` +
		`--uses-library runtime-required-x ` +
		`--uses-library runtime-required-y ` +
		`--optional-uses-library bar ` +
		`--optional-uses-library baz ` +
		`--optional-uses-library runtime-optional-x ` +
		`--optional-uses-library runtime-optional-y `
	android.AssertStringDoesContain(t, "verify cmd args", verifyCmd, verifyArgs)

	// Test that all libraries are verified for an APK (library order matters).
	verifyApkCmd := prebuilt.Rule("verify_uses_libraries").RuleParams.Command
	verifyApkArgs := `--uses-library foo ` +
		`--uses-library com.non.sdk.lib ` +
		`--uses-library android.test.runner ` +
		`--optional-uses-library bar ` +
		`--optional-uses-library baz `
	android.AssertStringDoesContain(t, "verify apk cmd args", verifyApkCmd, verifyApkArgs)

	// Test that necessary args are passed for constructing CLC in Ninja phase.
	cmd := app.Rule("dexpreopt").RuleParams.Command
	android.AssertStringDoesContain(t, "dexpreopt app cmd context", cmd, "--context-json=")
	android.AssertStringDoesContain(t, "dexpreopt app cmd product_packages", cmd,
		"--product-packages=out/soong/.intermediates/app/android_common/dexpreopt/app/product_packages.txt")
}

func TestDexpreoptBcp(t *testing.T) {
	bp := `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			api_packages: ["foo"],
			sdk_version: "current",
		}

		java_sdk_library {
			name: "bar",
			srcs: ["a.java"],
			api_packages: ["bar"],
			permitted_packages: ["bar"],
			sdk_version: "current",
		}

		android_app {
			name: "app",
			srcs: ["a.java"],
			sdk_version: "current",
		}
	`

	testCases := []struct {
		name   string
		with   bool
		expect string
	}{
		{
			name:   "with updatable bcp",
			with:   true,
			expect: "/system/framework/foo.jar:/system/framework/bar.jar",
		},
		{
			name:   "without updatable bcp",
			with:   false,
			expect: "/system/framework/foo.jar",
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := android.GroupFixturePreparers(
				prepareForJavaTest,
				PrepareForTestWithJavaSdkLibraryFiles,
				FixtureWithLastReleaseApis("runtime-library", "foo", "bar"),
				dexpreopt.FixtureSetBootJars("platform:foo"),
				dexpreopt.FixtureSetApexBootJars("platform:bar"),
				dexpreopt.FixtureSetPreoptWithUpdatableBcp(test.with),
			).RunTestWithBp(t, bp)

			app := result.ModuleForTests("app", "android_common")
			cmd := app.Rule("dexpreopt").RuleParams.Command
			bcp := " -Xbootclasspath-locations:" + test.expect + " " // space at the end matters
			android.AssertStringDoesContain(t, "dexpreopt app bcp", cmd, bcp)
		})
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

		result := android.GroupFixturePreparers(
			prepareForJavaTest,
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

func TestTargetSdkVersionManifestFixer(t *testing.T) {
	platform_sdk_codename := "Tiramisu"
	platform_sdk_version := 33
	testCases := []struct {
		name                     string
		targetSdkVersionInBp     string
		targetSdkVersionExpected string
		unbundledBuild           bool
		platformSdkFinal         bool
	}{
		{
			name:                     "Non-Unbundled build: Android.bp has targetSdkVersion",
			targetSdkVersionInBp:     "30",
			targetSdkVersionExpected: "30",
			unbundledBuild:           false,
		},
		{
			name:                     "Unbundled build: Android.bp has targetSdkVersion",
			targetSdkVersionInBp:     "30",
			targetSdkVersionExpected: "30",
			unbundledBuild:           true,
		},
		{
			name:                     "Non-Unbundled build: Android.bp has targetSdkVersion equal to platform_sdk_codename",
			targetSdkVersionInBp:     platform_sdk_codename,
			targetSdkVersionExpected: platform_sdk_codename,
			unbundledBuild:           false,
		},
		{
			name:                     "Unbundled build: Android.bp has targetSdkVersion equal to platform_sdk_codename",
			targetSdkVersionInBp:     platform_sdk_codename,
			targetSdkVersionExpected: "10000",
			unbundledBuild:           true,
		},

		{
			name:                     "Non-Unbundled build: Android.bp has no targetSdkVersion",
			targetSdkVersionExpected: platform_sdk_codename,
			unbundledBuild:           false,
		},
		{
			name:                     "Unbundled build: Android.bp has no targetSdkVersion",
			targetSdkVersionExpected: "10000",
			unbundledBuild:           true,
		},
		{
			name:                     "Bundled build in REL branches",
			targetSdkVersionExpected: "33",
			unbundledBuild:           false,
			platformSdkFinal:         true,
		},
	}
	for _, testCase := range testCases {
		targetSdkVersionTemplate := ""
		if testCase.targetSdkVersionInBp != "" {
			targetSdkVersionTemplate = fmt.Sprintf(`target_sdk_version: "%s",`, testCase.targetSdkVersionInBp)
		}
		bp := fmt.Sprintf(`
			android_app {
				name: "foo",
				sdk_version: "current",
				%s
			}
			`, targetSdkVersionTemplate)
		fixture := android.GroupFixturePreparers(
			prepareForJavaTest,
			android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				if testCase.platformSdkFinal {
					variables.Platform_sdk_final = proptools.BoolPtr(true)
				}
				// explicitly set platform_sdk_codename to make the test deterministic
				variables.Platform_sdk_codename = &platform_sdk_codename
				variables.Platform_sdk_version = &platform_sdk_version
				variables.Platform_version_active_codenames = []string{platform_sdk_codename}
				// create a non-empty list if unbundledBuild==true
				if testCase.unbundledBuild {
					variables.Unbundled_build_apps = []string{"apex_a", "apex_b"}
				}
			}),
		)

		result := fixture.RunTestWithBp(t, bp)
		foo := result.ModuleForTests("foo", "android_common")

		manifestFixerArgs := foo.Output("manifest_fixer/AndroidManifest.xml").Args["args"]
		android.AssertStringDoesContain(t, testCase.name, manifestFixerArgs, "--targetSdkVersion  "+testCase.targetSdkVersionExpected)
	}
}

func TestDefaultAppTargetSdkVersionForUpdatableModules(t *testing.T) {
	platform_sdk_codename := "Tiramisu"
	platform_sdk_version := 33
	testCases := []struct {
		name                     string
		platform_sdk_final       bool
		targetSdkVersionInBp     *string
		targetSdkVersionExpected *string
		updatable                bool
	}{
		{
			name:                     "Non-Updatable Module: Android.bp has older targetSdkVersion",
			targetSdkVersionInBp:     proptools.StringPtr("29"),
			targetSdkVersionExpected: proptools.StringPtr("29"),
			updatable:                false,
		},
		{
			name:                     "Updatable Module: Android.bp has older targetSdkVersion",
			targetSdkVersionInBp:     proptools.StringPtr("30"),
			targetSdkVersionExpected: proptools.StringPtr("30"),
			updatable:                true,
		},
		{
			name:                     "Updatable Module: Android.bp has no targetSdkVersion",
			targetSdkVersionExpected: proptools.StringPtr("10000"),
			updatable:                true,
		},
		{
			name:                     "[SDK finalised] Non-Updatable Module: Android.bp has older targetSdkVersion",
			platform_sdk_final:       true,
			targetSdkVersionInBp:     proptools.StringPtr("30"),
			targetSdkVersionExpected: proptools.StringPtr("30"),
			updatable:                false,
		},
		{
			name:                     "[SDK finalised] Updatable Module: Android.bp has older targetSdkVersion",
			platform_sdk_final:       true,
			targetSdkVersionInBp:     proptools.StringPtr("30"),
			targetSdkVersionExpected: proptools.StringPtr("30"),
			updatable:                true,
		},
		{
			name:                     "[SDK finalised] Updatable Module: Android.bp has targetSdkVersion as platform sdk codename",
			platform_sdk_final:       true,
			targetSdkVersionInBp:     proptools.StringPtr(platform_sdk_codename),
			targetSdkVersionExpected: proptools.StringPtr("33"),
			updatable:                true,
		},
		{
			name:                     "[SDK finalised] Updatable Module: Android.bp has no targetSdkVersion",
			platform_sdk_final:       true,
			targetSdkVersionExpected: proptools.StringPtr("33"),
			updatable:                true,
		},
	}
	for _, testCase := range testCases {
		targetSdkVersionTemplate := ""
		if testCase.targetSdkVersionInBp != nil {
			targetSdkVersionTemplate = fmt.Sprintf(`target_sdk_version: "%s",`, *testCase.targetSdkVersionInBp)
		}
		bp := fmt.Sprintf(`
			android_app {
				name: "foo",
				sdk_version: "current",
				min_sdk_version: "29",
				%s
				updatable: %t,
				enforce_default_target_sdk_version: %t
			}
			`, targetSdkVersionTemplate, testCase.updatable, testCase.updatable) // enforce default target sdk version if app is updatable

		fixture := android.GroupFixturePreparers(
			PrepareForTestWithJavaDefaultModules,
			android.PrepareForTestWithAllowMissingDependencies,
			android.PrepareForTestWithAndroidMk,
			android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				// explicitly set following platform variables to make the test deterministic
				variables.Platform_sdk_final = &testCase.platform_sdk_final
				variables.Platform_sdk_version = &platform_sdk_version
				variables.Platform_sdk_codename = &platform_sdk_codename
				variables.Platform_version_active_codenames = []string{platform_sdk_codename}
				variables.Unbundled_build = proptools.BoolPtr(true)
				variables.Unbundled_build_apps = []string{"sampleModule"}
			}),
		)

		result := fixture.RunTestWithBp(t, bp)
		foo := result.ModuleForTests("foo", "android_common")

		manifestFixerArgs := foo.Output("manifest_fixer/AndroidManifest.xml").Args["args"]
		android.AssertStringDoesContain(t, testCase.name, manifestFixerArgs, "--targetSdkVersion  "+*testCase.targetSdkVersionExpected)
	}
}

func TestEnforceDefaultAppTargetSdkVersionFlag(t *testing.T) {
	platform_sdk_codename := "Tiramisu"
	platform_sdk_version := 33
	testCases := []struct {
		name                           string
		enforceDefaultTargetSdkVersion bool
		expectedError                  string
		platform_sdk_final             bool
		targetSdkVersionInBp           string
		targetSdkVersionExpected       string
		updatable                      bool
	}{
		{
			name:                           "Not enforcing Target SDK Version: Android.bp has older targetSdkVersion",
			enforceDefaultTargetSdkVersion: false,
			targetSdkVersionInBp:           "29",
			targetSdkVersionExpected:       "29",
			updatable:                      false,
		},
		{
			name:                           "[SDK finalised] Enforce Target SDK Version: Android.bp has current targetSdkVersion",
			enforceDefaultTargetSdkVersion: true,
			platform_sdk_final:             true,
			targetSdkVersionInBp:           "current",
			targetSdkVersionExpected:       "33",
			updatable:                      true,
		},
		{
			name:                           "Enforce Target SDK Version: Android.bp has current targetSdkVersion",
			enforceDefaultTargetSdkVersion: true,
			platform_sdk_final:             false,
			targetSdkVersionInBp:           "current",
			targetSdkVersionExpected:       "10000",
			updatable:                      false,
		},
		{
			name:                           "Not enforcing Target SDK Version for Updatable app",
			enforceDefaultTargetSdkVersion: false,
			expectedError:                  "Updatable apps must enforce default target sdk version",
			targetSdkVersionInBp:           "29",
			targetSdkVersionExpected:       "29",
			updatable:                      true,
		},
	}
	for _, testCase := range testCases {
		errExpected := testCase.expectedError != ""
		bp := fmt.Sprintf(`
			android_app {
				name: "foo",
				enforce_default_target_sdk_version: %t,
				sdk_version: "current",
				min_sdk_version: "29",
				target_sdk_version: "%v",
				updatable: %t
			}
			`, testCase.enforceDefaultTargetSdkVersion, testCase.targetSdkVersionInBp, testCase.updatable)

		fixture := android.GroupFixturePreparers(
			PrepareForTestWithJavaDefaultModules,
			android.PrepareForTestWithAllowMissingDependencies,
			android.PrepareForTestWithAndroidMk,
			android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				// explicitly set following platform variables to make the test deterministic
				variables.Platform_sdk_final = &testCase.platform_sdk_final
				variables.Platform_sdk_version = &platform_sdk_version
				variables.Platform_sdk_codename = &platform_sdk_codename
				variables.Unbundled_build = proptools.BoolPtr(true)
				variables.Unbundled_build_apps = []string{"sampleModule"}
			}),
		)

		errorHandler := android.FixtureExpectsNoErrors
		if errExpected {
			errorHandler = android.FixtureExpectsAtLeastOneErrorMatchingPattern(testCase.expectedError)
		}
		result := fixture.ExtendWithErrorHandler(errorHandler).RunTestWithBp(t, bp)

		if !errExpected {
			foo := result.ModuleForTests("foo", "android_common")
			manifestFixerArgs := foo.Output("manifest_fixer/AndroidManifest.xml").Args["args"]
			android.AssertStringDoesContain(t, testCase.name, manifestFixerArgs, "--targetSdkVersion  "+testCase.targetSdkVersionExpected)
		}
	}
}

func TestEnforceDefaultAppTargetSdkVersionFlagForTests(t *testing.T) {
	platform_sdk_codename := "Tiramisu"
	platform_sdk_version := 33
	testCases := []struct {
		name                           string
		enforceDefaultTargetSdkVersion bool
		expectedError                  string
		platform_sdk_final             bool
		targetSdkVersionInBp           string
		targetSdkVersionExpected       string
	}{
		{
			name:                           "Not enforcing Target SDK Version: Android.bp has older targetSdkVersion",
			enforceDefaultTargetSdkVersion: false,
			targetSdkVersionInBp:           "29",
			targetSdkVersionExpected:       "29",
		},
		{
			name:                           "[SDK finalised] Enforce Target SDK Version: Android.bp has current targetSdkVersion",
			enforceDefaultTargetSdkVersion: true,
			platform_sdk_final:             true,
			targetSdkVersionInBp:           "current",
			targetSdkVersionExpected:       "33",
		},
		{
			name:                           "Enforce Target SDK Version: Android.bp has current targetSdkVersion",
			enforceDefaultTargetSdkVersion: true,
			platform_sdk_final:             false,
			targetSdkVersionInBp:           "current",
			targetSdkVersionExpected:       "10000",
		},
	}
	for _, testCase := range testCases {
		errExpected := testCase.expectedError != ""
		bp := fmt.Sprintf(`
			android_test {
				name: "foo",
				enforce_default_target_sdk_version: %t,
				min_sdk_version: "29",
				target_sdk_version: "%v",
			}
		`, testCase.enforceDefaultTargetSdkVersion, testCase.targetSdkVersionInBp)

		fixture := android.GroupFixturePreparers(
			PrepareForTestWithJavaDefaultModules,
			android.PrepareForTestWithAllowMissingDependencies,
			android.PrepareForTestWithAndroidMk,
			android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				// explicitly set following platform variables to make the test deterministic
				variables.Platform_sdk_final = &testCase.platform_sdk_final
				variables.Platform_sdk_version = &platform_sdk_version
				variables.Platform_sdk_codename = &platform_sdk_codename
				variables.Unbundled_build = proptools.BoolPtr(true)
				variables.Unbundled_build_apps = []string{"sampleModule"}
			}),
		)

		errorHandler := android.FixtureExpectsNoErrors
		if errExpected {
			errorHandler = android.FixtureExpectsAtLeastOneErrorMatchingPattern(testCase.expectedError)
		}
		result := fixture.ExtendWithErrorHandler(errorHandler).RunTestWithBp(t, bp)

		if !errExpected {
			foo := result.ModuleForTests("foo", "android_common")
			manifestFixerArgs := foo.Output("manifest_fixer/AndroidManifest.xml").Args["args"]
			android.AssertStringDoesContain(t, testCase.name, manifestFixerArgs, "--targetSdkVersion  "+testCase.targetSdkVersionExpected)
		}
	}
}

func TestAppMissingCertificateAllowMissingDependencies(t *testing.T) {
	result := android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
		android.PrepareForTestWithAllowMissingDependencies,
		android.PrepareForTestWithAndroidMk,
	).RunTestWithBp(t, `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			certificate: ":missing_certificate",
			sdk_version: "current",
		}

		android_app {
			name: "bar",
			srcs: ["a.java"],
			certificate: ":missing_certificate",
			product_specific: true,
			sdk_version: "current",
		}`)

	foo := result.ModuleForTests("foo", "android_common")
	fooApk := foo.Output("foo.apk")
	if fooApk.Rule != android.ErrorRule {
		t.Fatalf("expected ErrorRule for foo.apk, got %s", fooApk.Rule.String())
	}
	android.AssertStringDoesContain(t, "expected error rule message", fooApk.Args["error"], "missing dependencies: missing_certificate\n")
}

func TestAppIncludesJniPackages(t *testing.T) {
	ctx := android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
	).RunTestWithBp(t, `
		android_library_import {
			name: "aary-nodeps",
			aars: ["aary.aar"],
			extract_jni: true,
		}

		android_library {
			name: "aary-lib",
			sdk_version: "current",
			min_sdk_version: "21",
			static_libs: ["aary-nodeps"],
		}

		android_app {
			name: "aary-lib-dep",
			sdk_version: "current",
			min_sdk_version: "21",
			manifest: "AndroidManifest.xml",
			static_libs: ["aary-lib"],
			use_embedded_native_libs: true,
		}

		android_app {
			name: "aary-import-dep",
			sdk_version: "current",
			min_sdk_version: "21",
			manifest: "AndroidManifest.xml",
			static_libs: ["aary-nodeps"],
			use_embedded_native_libs: true,
		}

		android_app {
			name: "aary-no-use-embedded",
			sdk_version: "current",
			min_sdk_version: "21",
			manifest: "AndroidManifest.xml",
			static_libs: ["aary-nodeps"],
		}`)

	testCases := []struct {
		name       string
		hasPackage bool
	}{
		{
			name:       "aary-import-dep",
			hasPackage: true,
		},
		{
			name:       "aary-lib-dep",
			hasPackage: true,
		},
		{
			name:       "aary-no-use-embedded",
			hasPackage: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			app := ctx.ModuleForTests(tc.name, "android_common")

			outputFile := "jnilibs.zip"
			jniOutputLibZip := app.MaybeOutput(outputFile)
			if jniOutputLibZip.Rule == nil && !tc.hasPackage {
				return
			}

			jniPackage := "arm64-v8a_jni.zip"
			inputs := jniOutputLibZip.Inputs
			foundPackage := false
			for i := 0; i < len(inputs); i++ {
				if strings.Contains(inputs[i].String(), jniPackage) {
					foundPackage = true
				}
			}
			if foundPackage != tc.hasPackage {
				t.Errorf("expected to find %v in %v inputs; inputs = %v", jniPackage, outputFile, inputs)
			}
		})
	}
}

func TestTargetSdkVersionMtsTests(t *testing.T) {
	platformSdkCodename := "Tiramisu"
	android_test := "android_test"
	android_test_helper_app := "android_test_helper_app"
	bpTemplate := `
	%v {
		name: "mytest",
		target_sdk_version: "%v",
		test_suites: ["othersuite", "%v"],
	}
	`
	testCases := []struct {
		desc                     string
		moduleType               string
		targetSdkVersionInBp     string
		targetSdkVersionExpected string
		testSuites               string
	}{
		{
			desc:                     "Non-MTS android_test_apps targeting current should not be upgraded to 10000",
			moduleType:               android_test,
			targetSdkVersionInBp:     "current",
			targetSdkVersionExpected: platformSdkCodename,
			testSuites:               "non-mts-suite",
		},
		{
			desc:                     "MTS android_test_apps targeting released sdks should not be upgraded to 10000",
			moduleType:               android_test,
			targetSdkVersionInBp:     "29",
			targetSdkVersionExpected: "29",
			testSuites:               "mts-suite",
		},
		{
			desc:                     "MTS android_test_apps targeting current should be upgraded to 10000",
			moduleType:               android_test,
			targetSdkVersionInBp:     "current",
			targetSdkVersionExpected: "10000",
			testSuites:               "mts-suite",
		},
		{
			desc:                     "MTS android_test_helper_apps targeting current should be upgraded to 10000",
			moduleType:               android_test_helper_app,
			targetSdkVersionInBp:     "current",
			targetSdkVersionExpected: "10000",
			testSuites:               "mts-suite",
		},
	}
	fixture := android.GroupFixturePreparers(
		prepareForJavaTest,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.Platform_sdk_codename = &platformSdkCodename
			variables.Platform_version_active_codenames = []string{platformSdkCodename}
		}),
	)
	for _, testCase := range testCases {
		result := fixture.RunTestWithBp(t, fmt.Sprintf(bpTemplate, testCase.moduleType, testCase.targetSdkVersionInBp, testCase.testSuites))
		mytest := result.ModuleForTests("mytest", "android_common")
		manifestFixerArgs := mytest.Output("manifest_fixer/AndroidManifest.xml").Args["args"]
		android.AssertStringDoesContain(t, testCase.desc, manifestFixerArgs, "--targetSdkVersion  "+testCase.targetSdkVersionExpected)
	}
}

func TestPrivappAllowlist(t *testing.T) {
	testJavaError(t, "privileged must be set in order to use privapp_allowlist", `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			privapp_allowlist: "perms.xml",
		}
	`)

	result := PrepareForTestWithJavaDefaultModules.RunTestWithBp(
		t,
		`
		android_app {
			name: "foo",
			srcs: ["a.java"],
			privapp_allowlist: "privapp_allowlist_com.android.foo.xml",
			privileged: true,
			sdk_version: "current",
		}
		override_android_app {
			name: "bar",
			base: "foo",
			package_name: "com.google.android.foo",
		}
		`,
	)
	app := result.ModuleForTests("foo", "android_common")
	overrideApp := result.ModuleForTests("foo", "android_common_bar")

	// verify that privapp allowlist is created for override apps
	overrideApp.Output("out/soong/.intermediates/foo/android_common_bar/privapp_allowlist_com.google.android.foo.xml")
	expectedAllowlistInput := "privapp_allowlist_com.android.foo.xml"
	overrideActualAllowlistInput := overrideApp.Rule("modifyAllowlist").Input.String()
	if expectedAllowlistInput != overrideActualAllowlistInput {
		t.Errorf("expected override allowlist to be %q; got %q", expectedAllowlistInput, overrideActualAllowlistInput)
	}

	// verify that permissions are copied to device
	app.Output("out/soong/target/product/test_device/system/etc/permissions/foo.xml")
	overrideApp.Output("out/soong/target/product/test_device/system/etc/permissions/bar.xml")
}

func TestPrivappAllowlistAndroidMk(t *testing.T) {
	result := android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
		android.PrepareForTestWithAndroidMk,
	).RunTestWithBp(
		t,
		`
		android_app {
			name: "foo",
			srcs: ["a.java"],
			privapp_allowlist: "privapp_allowlist_com.android.foo.xml",
			privileged: true,
			sdk_version: "current",
		}
		override_android_app {
			name: "bar",
			base: "foo",
			package_name: "com.google.android.foo",
		}
		`,
	)
	baseApp := result.ModuleForTests("foo", "android_common")
	overrideApp := result.ModuleForTests("foo", "android_common_bar")

	baseAndroidApp := baseApp.Module().(*AndroidApp)
	baseEntries := android.AndroidMkEntriesForTest(t, result.TestContext, baseAndroidApp)[0]
	android.AssertStringMatches(
		t,
		"androidmk has incorrect LOCAL_SOONG_INSTALLED_MODULE; expected to find foo.apk",
		baseEntries.EntryMap["LOCAL_SOONG_INSTALLED_MODULE"][0],
		"\\S+foo.apk",
	)
	android.AssertStringMatches(
		t,
		"androidmk has incorrect LOCAL_SOONG_INSTALL_PAIRS; expected to it to include foo.apk",
		baseEntries.EntryMap["LOCAL_SOONG_INSTALL_PAIRS"][0],
		"\\S+foo.apk",
	)
	android.AssertStringMatches(
		t,
		"androidmk has incorrect LOCAL_SOONG_INSTALL_PAIRS; expected to it to include app",
		baseEntries.EntryMap["LOCAL_SOONG_INSTALL_PAIRS"][0],
		"\\S+foo.apk:\\S+/target/product/test_device/system/priv-app/foo/foo.apk",
	)
	android.AssertStringMatches(
		t,
		"androidmk has incorrect LOCAL_SOONG_INSTALL_PAIRS; expected to it to include privapp_allowlist",
		baseEntries.EntryMap["LOCAL_SOONG_INSTALL_PAIRS"][0],
		"privapp_allowlist_com.android.foo.xml:\\S+/target/product/test_device/system/etc/permissions/foo.xml",
	)

	overrideAndroidApp := overrideApp.Module().(*AndroidApp)
	overrideEntries := android.AndroidMkEntriesForTest(t, result.TestContext, overrideAndroidApp)[0]
	android.AssertStringMatches(
		t,
		"androidmk has incorrect LOCAL_SOONG_INSTALLED_MODULE; expected to find bar.apk",
		overrideEntries.EntryMap["LOCAL_SOONG_INSTALLED_MODULE"][0],
		"\\S+bar.apk",
	)
	android.AssertStringMatches(
		t,
		"androidmk has incorrect LOCAL_SOONG_INSTALL_PAIRS; expected to it to include bar.apk",
		overrideEntries.EntryMap["LOCAL_SOONG_INSTALL_PAIRS"][0],
		"\\S+bar.apk",
	)
	android.AssertStringMatches(
		t,
		"androidmk has incorrect LOCAL_SOONG_INSTALL_PAIRS; expected to it to include app",
		overrideEntries.EntryMap["LOCAL_SOONG_INSTALL_PAIRS"][0],
		"\\S+bar.apk:\\S+/target/product/test_device/system/priv-app/bar/bar.apk",
	)
	android.AssertStringMatches(
		t,
		"androidmk has incorrect LOCAL_SOONG_INSTALL_PAIRS; expected to it to include privapp_allowlist",
		overrideEntries.EntryMap["LOCAL_SOONG_INSTALL_PAIRS"][0],
		"\\S+soong/.intermediates/foo/android_common_bar/privapp_allowlist_com.google.android.foo.xml:\\S+/target/product/test_device/system/etc/permissions/bar.xml",
	)
}

func TestApexGlobalMinSdkVersionOverride(t *testing.T) {
	result := android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.ApexGlobalMinSdkVersionOverride = proptools.StringPtr("Tiramisu")
		}),
	).RunTestWithBp(t, `
		android_app {
			name: "com.android.bar",
			srcs: ["a.java"],
			sdk_version: "current",
		}
		android_app {
			name: "com.android.foo",
			srcs: ["a.java"],
			sdk_version: "current",
			min_sdk_version: "S",
			updatable: true,
		}
		override_android_app {
			name: "com.android.go.foo",
			base: "com.android.foo",
		}
	`)
	foo := result.ModuleForTests("com.android.foo", "android_common").Rule("manifestFixer")
	fooOverride := result.ModuleForTests("com.android.foo", "android_common_com.android.go.foo").Rule("manifestFixer")
	bar := result.ModuleForTests("com.android.bar", "android_common").Rule("manifestFixer")

	android.AssertStringDoesContain(t,
		"expected manifest fixer to set com.android.bar minSdkVersion to S",
		bar.BuildParams.Args["args"],
		"--minSdkVersion  S",
	)
	android.AssertStringDoesContain(t,
		"com.android.foo: expected manifest fixer to set minSdkVersion to T",
		foo.BuildParams.Args["args"],
		"--minSdkVersion  T",
	)
	android.AssertStringDoesContain(t,
		"com.android.go.foo: expected manifest fixer to set minSdkVersion to T",
		fooOverride.BuildParams.Args["args"],
		"--minSdkVersion  T",
	)

}

func TestAppFlagsPackages(t *testing.T) {
	ctx := testApp(t, `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			sdk_version: "current",
			flags_packages: [
				"bar",
				"baz",
			],
		}
		aconfig_declarations {
			name: "bar",
			package: "com.example.package.bar",
			srcs: [
				"bar.aconfig",
			],
		}
		aconfig_declarations {
			name: "baz",
			package: "com.example.package.baz",
			srcs: [
				"baz.aconfig",
			],
		}
	`)

	foo := ctx.ModuleForTests("foo", "android_common")

	// android_app module depends on aconfig_declarations listed in flags_packages
	android.AssertBoolEquals(t, "foo expected to depend on bar", true,
		CheckModuleHasDependency(t, ctx, "foo", "android_common", "bar"))

	android.AssertBoolEquals(t, "foo expected to depend on baz", true,
		CheckModuleHasDependency(t, ctx, "foo", "android_common", "baz"))

	aapt2LinkRule := foo.Rule("android/soong/java.aapt2Link")
	linkInFlags := aapt2LinkRule.Args["inFlags"]
	android.AssertStringDoesContain(t,
		"aapt2 link command expected to pass feature flags arguments",
		linkInFlags,
		"--feature-flags @out/soong/.intermediates/bar/intermediate.txt --feature-flags @out/soong/.intermediates/baz/intermediate.txt",
	)
}

// Test that dexpreopt is disabled if an optional_uses_libs exists, but does not provide an implementation.
func TestNoDexpreoptOptionalUsesLibDoesNotHaveImpl(t *testing.T) {
	bp := `
		java_sdk_library_import {
			name: "sdklib_noimpl",
			public: {
				jars: ["stub.jar"],
			},
		}
		android_app {
			name: "app",
			srcs: ["a.java"],
			sdk_version: "current",
			optional_uses_libs: [
				"sdklib_noimpl",
			],
		}
	`
	result := prepareForJavaTest.RunTestWithBp(t, bp)
	dexpreopt := result.ModuleForTests("app", "android_common").MaybeRule("dexpreopt").Rule
	android.AssertBoolEquals(t, "dexpreopt should be disabled if optional_uses_libs does not have an implementation", true, dexpreopt == nil)
}
