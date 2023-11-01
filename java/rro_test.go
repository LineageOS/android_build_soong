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

package java

import (
	"reflect"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/shared"
)

func TestRuntimeResourceOverlay(t *testing.T) {
	fs := android.MockFS{
		"baz/res/res/values/strings.xml": nil,
		"bar/res/res/values/strings.xml": nil,
	}
	bp := `
		runtime_resource_overlay {
			name: "foo",
			certificate: "platform",
			lineage: "lineage.bin",
			rotationMinSdkVersion: "32",
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

	result := android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
		android.FixtureModifyConfig(android.SetKatiEnabledForTests),
		fs.AddToFixture(),
	).RunTestWithBp(t, bp)

	m := result.ModuleForTests("foo", "android_common")

	// Check AAPT2 link flags.
	aapt2Flags := m.Output("package-res.apk").Args["flags"]
	expectedFlags := []string{"--keep-raw-values", "--no-resource-deduping", "--no-resource-removal"}
	absentFlags := android.RemoveListFromList(expectedFlags, strings.Split(aapt2Flags, " "))
	if len(absentFlags) > 0 {
		t.Errorf("expected values, %q are missing in aapt2 link flags, %q", absentFlags, aapt2Flags)
	}

	// Check overlay.list output for static_libs dependency.
	overlayList := android.PathsRelativeToTop(m.Output("aapt2/overlay.list").Inputs)
	staticLibPackage := "out/soong/.intermediates/bar/android_common/package-res.apk"
	if !inList(staticLibPackage, overlayList) {
		t.Errorf("Stactic lib res package %q missing in overlay list: %q", staticLibPackage, overlayList)
	}

	// Check AAPT2 link flags for resource_libs dependency.
	resourceLibFlag := "-I " + "out/soong/.intermediates/baz/android_common/package-res.apk"
	if !strings.Contains(aapt2Flags, resourceLibFlag) {
		t.Errorf("Resource lib flag %q missing in aapt2 link flags: %q", resourceLibFlag, aapt2Flags)
	}

	// Check cert signing flags.
	signedApk := m.Output("signed/foo.apk")
	actualCertSigningFlags := signedApk.Args["flags"]
	expectedCertSigningFlags := "--lineage lineage.bin --rotation-min-sdk-version 32"
	if expectedCertSigningFlags != actualCertSigningFlags {
		t.Errorf("Incorrect cert signing flags, expected: %q, got: %q", expectedCertSigningFlags, actualCertSigningFlags)
	}

	signingFlag := signedApk.Args["certificates"]
	expected := "build/make/target/product/security/platform.x509.pem build/make/target/product/security/platform.pk8"
	if expected != signingFlag {
		t.Errorf("Incorrect signing flags, expected: %q, got: %q", expected, signingFlag)
	}
	androidMkEntries := android.AndroidMkEntriesForTest(t, result.TestContext, m.Module())[0]
	path := androidMkEntries.EntryMap["LOCAL_CERTIFICATE"]
	expectedPath := []string{"build/make/target/product/security/platform.x509.pem"}
	if !reflect.DeepEqual(path, expectedPath) {
		t.Errorf("Unexpected LOCAL_CERTIFICATE value: %v, expected: %v", path, expectedPath)
	}

	// Check device location.
	path = androidMkEntries.EntryMap["LOCAL_MODULE_PATH"]
	expectedPath = []string{shared.JoinPath("out/target/product/test_device/product/overlay")}
	android.AssertStringPathsRelativeToTopEquals(t, "LOCAL_MODULE_PATH", result.Config, expectedPath, path)

	// A themed module has a different device location
	m = result.ModuleForTests("foo_themed", "android_common")
	androidMkEntries = android.AndroidMkEntriesForTest(t, result.TestContext, m.Module())[0]
	path = androidMkEntries.EntryMap["LOCAL_MODULE_PATH"]
	expectedPath = []string{shared.JoinPath("out/target/product/test_device/product/overlay/faza")}
	android.AssertStringPathsRelativeToTopEquals(t, "LOCAL_MODULE_PATH", result.Config, expectedPath, path)

	overrides := androidMkEntries.EntryMap["LOCAL_OVERRIDES_PACKAGES"]
	expectedOverrides := []string{"foo"}
	if !reflect.DeepEqual(overrides, expectedOverrides) {
		t.Errorf("Unexpected LOCAL_OVERRIDES_PACKAGES value: %v, expected: %v", overrides, expectedOverrides)
	}
}

func TestRuntimeResourceOverlay_JavaDefaults(t *testing.T) {
	result := android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
		android.FixtureModifyConfig(android.SetKatiEnabledForTests),
	).RunTestWithBp(t, `
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
	m := result.ModuleForTests("foo_with_defaults", "android_common")

	// Check AAPT2 link flags.
	aapt2Flags := strings.Split(m.Output("package-res.apk").Args["flags"], " ")
	expectedFlags := []string{"--keep-raw-values", "--no-resource-deduping", "--no-resource-removal"}
	absentFlags := android.RemoveListFromList(expectedFlags, aapt2Flags)
	if len(absentFlags) > 0 {
		t.Errorf("expected values, %q are missing in aapt2 link flags, %q", absentFlags, aapt2Flags)
	}

	// Check device location.
	path := android.AndroidMkEntriesForTest(t, result.TestContext, m.Module())[0].EntryMap["LOCAL_MODULE_PATH"]
	expectedPath := []string{shared.JoinPath("out/target/product/test_device/product/overlay/default_theme")}
	android.AssertStringPathsRelativeToTopEquals(t, "LOCAL_MODULE_PATH", result.Config, expectedPath, path)

	//
	// RRO module without defaults
	//
	m = result.ModuleForTests("foo_barebones", "android_common")

	// Check AAPT2 link flags.
	aapt2Flags = strings.Split(m.Output("package-res.apk").Args["flags"], " ")
	unexpectedFlags := "--keep-raw-values"
	if inList(unexpectedFlags, aapt2Flags) {
		t.Errorf("unexpected value, %q is present in aapt2 link flags, %q", unexpectedFlags, aapt2Flags)
	}

	// Check device location.
	path = android.AndroidMkEntriesForTest(t, result.TestContext, m.Module())[0].EntryMap["LOCAL_MODULE_PATH"]
	expectedPath = []string{shared.JoinPath("out/target/product/test_device/product/overlay")}
	android.AssertStringPathsRelativeToTopEquals(t, "LOCAL_MODULE_PATH", result.Config, expectedPath, path)
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
			category: "mycategory",
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
		categoryFlag      string
	}{
		{
			variantName:       "android_common",
			apkPath:           "out/soong/target/product/test_device/product/overlay/foo_overlay.apk",
			overrides:         nil,
			targetVariant:     "android_common",
			packageFlag:       "",
			targetPackageFlag: "",
		},
		{
			variantName:       "android_common_bar_overlay",
			apkPath:           "out/soong/target/product/test_device/product/overlay/bar_overlay.apk",
			overrides:         []string{"foo_overlay"},
			targetVariant:     "android_common_bar",
			packageFlag:       "com.android.bar.overlay",
			targetPackageFlag: "com.android.bar",
			categoryFlag:      "mycategory",
		},
	}
	for _, expected := range expectedVariants {
		variant := ctx.ModuleForTests("foo_overlay", expected.variantName)

		// Check the final apk name
		variant.Output(expected.apkPath)

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
		checkAapt2LinkFlag(t, aapt2Flags, "rename-resources-package", "")
		checkAapt2LinkFlag(t, aapt2Flags, "rename-overlay-target-package", expected.targetPackageFlag)
		checkAapt2LinkFlag(t, aapt2Flags, "rename-overlay-category", expected.categoryFlag)
	}
}

func TestEnforceRRO_propagatesToDependencies(t *testing.T) {
	testCases := []struct {
		name              string
		enforceRROTargets []string
		rroDirs           map[string][]string
	}{
		{
			name:              "no RRO",
			enforceRROTargets: nil,
			rroDirs: map[string][]string{
				"foo": nil,
				"bar": nil,
			},
		},
		{
			name:              "enforce RRO on all",
			enforceRROTargets: []string{"*"},
			rroDirs: map[string][]string{
				"foo": {"product/vendor/blah/overlay/lib2/res"},
				"bar": {"product/vendor/blah/overlay/lib2/res"},
			},
		},
		{
			name:              "enforce RRO on foo",
			enforceRROTargets: []string{"foo"},
			rroDirs: map[string][]string{
				"foo": {"product/vendor/blah/overlay/lib2/res"},
				"bar": {"product/vendor/blah/overlay/lib2/res"},
			},
		},
	}

	productResourceOverlays := []string{
		"product/vendor/blah/overlay",
	}

	fs := android.MockFS{
		"lib2/res/values/strings.xml":                             nil,
		"product/vendor/blah/overlay/lib2/res/values/strings.xml": nil,
	}

	bp := `
			android_app {
				name: "foo",
				sdk_version: "current",
				resource_dirs: [],
				static_libs: ["lib"],
			}

			android_app {
				name: "bar",
				sdk_version: "current",
				resource_dirs: [],
				static_libs: ["lib"],
			}

			android_library {
				name: "lib",
				sdk_version: "current",
				resource_dirs: [],
				static_libs: ["lib2"],
			}

			android_library {
				name: "lib2",
				sdk_version: "current",
				resource_dirs: ["lib2/res"],
			}
		`

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := android.GroupFixturePreparers(
				PrepareForTestWithJavaDefaultModules,
				fs.AddToFixture(),
				android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
					variables.ProductResourceOverlays = productResourceOverlays
					if testCase.enforceRROTargets != nil {
						variables.EnforceRROTargets = testCase.enforceRROTargets
					}
				}),
			).RunTestWithBp(t, bp)

			modules := []string{"foo", "bar"}
			for _, moduleName := range modules {
				module := result.ModuleForTests(moduleName, "android_common")
				mkEntries := android.AndroidMkEntriesForTest(t, result.TestContext, module.Module())[0]
				actualRRODirs := mkEntries.EntryMap["LOCAL_SOONG_PRODUCT_RRO_DIRS"]
				if !reflect.DeepEqual(actualRRODirs, testCase.rroDirs[moduleName]) {
					t.Errorf("exected %s LOCAL_SOONG_PRODUCT_RRO_DIRS entry: %v\ngot:%q",
						moduleName, testCase.rroDirs[moduleName], actualRRODirs)
				}
			}
		})
	}
}

func TestRuntimeResourceOverlayPartition(t *testing.T) {
	bp := `
		runtime_resource_overlay {
			name: "device_specific",
			device_specific: true,
		}
		runtime_resource_overlay {
			name: "soc_specific",
			soc_specific: true,
		}
		runtime_resource_overlay {
			name: "system_ext_specific",
			system_ext_specific: true,
		}
		runtime_resource_overlay {
			name: "product_specific",
			product_specific: true,
		}
		runtime_resource_overlay {
			name: "default"
		}
	`
	testCases := []struct {
		name         string
		expectedPath string
	}{
		{
			name:         "device_specific",
			expectedPath: "out/soong/target/product/test_device/odm/overlay",
		},
		{
			name:         "soc_specific",
			expectedPath: "out/soong/target/product/test_device/vendor/overlay",
		},
		{
			name:         "system_ext_specific",
			expectedPath: "out/soong/target/product/test_device/system_ext/overlay",
		},
		{
			name:         "product_specific",
			expectedPath: "out/soong/target/product/test_device/product/overlay",
		},
		{
			name:         "default",
			expectedPath: "out/soong/target/product/test_device/product/overlay",
		},
	}
	for _, testCase := range testCases {
		ctx, _ := testJava(t, bp)
		mod := ctx.ModuleForTests(testCase.name, "android_common").Module().(*RuntimeResourceOverlay)
		android.AssertPathRelativeToTopEquals(t, "Install dir is not correct for "+testCase.name, testCase.expectedPath, mod.installDir)
	}
}
