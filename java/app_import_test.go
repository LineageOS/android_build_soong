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
	"regexp"
	"strings"
	"testing"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

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
			expected:            "verify_uses_libraries/apk/app.apk",
		},
		{
			name:                "AAPTPreferredConfig matches",
			aaptPreferredConfig: proptools.StringPtr("xhdpi"),
			aaptPrebuiltDPI:     []string{"xxhdpi", "ldpi"},
			expected:            "verify_uses_libraries/apk/app_xhdpi.apk",
		},
		{
			name:                "AAPTPrebuiltDPI matches",
			aaptPreferredConfig: proptools.StringPtr("mdpi"),
			aaptPrebuiltDPI:     []string{"xxhdpi", "xhdpi"},
			expected:            "verify_uses_libraries/apk/app_xxhdpi.apk",
		},
		{
			name:                "non-first AAPTPrebuiltDPI matches",
			aaptPreferredConfig: proptools.StringPtr("mdpi"),
			aaptPrebuiltDPI:     []string{"ldpi", "xhdpi"},
			expected:            "verify_uses_libraries/apk/app_xhdpi.apk",
		},
		{
			name:                "no matches",
			aaptPreferredConfig: proptools.StringPtr("mdpi"),
			aaptPrebuiltDPI:     []string{"ldpi", "xxxhdpi"},
			expected:            "verify_uses_libraries/apk/app.apk",
		},
	}

	jniRuleRe := regexp.MustCompile("^if \\(zipinfo (\\S+)")
	for _, test := range testCases {
		config := testAppConfig(nil, bp, nil)
		config.TestProductVariables.AAPTPreferredConfig = test.aaptPreferredConfig
		config.TestProductVariables.AAPTPrebuiltDPI = test.aaptPrebuiltDPI
		ctx := testContext(config)

		run(t, ctx, config)

		variant := ctx.ModuleForTests("foo", "android_common")
		jniRuleCommand := variant.Output("jnis-uncompressed/foo.apk").RuleParams.Command
		matches := jniRuleRe.FindStringSubmatch(jniRuleCommand)
		if len(matches) != 2 {
			t.Errorf("failed to extract the src apk path from %q", jniRuleCommand)
		}
		if strings.HasSuffix(matches[1], test.expected) {
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
			expected: "verify_uses_libraries/apk/app_arm64.apk",
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
			expected: "verify_uses_libraries/apk/app.apk",
		},
		{
			name: "no matching arch without default",
			bp: `
				android_app_import {
					name: "foo",
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
			expected: "",
		},
	}

	jniRuleRe := regexp.MustCompile("^if \\(zipinfo (\\S+)")
	for _, test := range testCases {
		ctx, _ := testJava(t, test.bp)

		variant := ctx.ModuleForTests("foo", "android_common")
		if test.expected == "" {
			if variant.Module().Enabled() {
				t.Error("module should have been disabled, but wasn't")
			}
			continue
		}
		jniRuleCommand := variant.Output("jnis-uncompressed/foo.apk").RuleParams.Command
		matches := jniRuleRe.FindStringSubmatch(jniRuleCommand)
		if len(matches) != 2 {
			t.Errorf("failed to extract the src apk path from %q", jniRuleCommand)
		}
		if strings.HasSuffix(matches[1], test.expected) {
			t.Errorf("wrong src apk, expected: %q got: %q", test.expected, matches[1])
		}
	}
}

func TestAndroidAppImport_overridesDisabledAndroidApp(t *testing.T) {
	ctx, _ := testJava(t, `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			enabled: false,
		}

 		android_app_import {
			name: "foo",
			apk: "prebuilts/apk/app.apk",
			certificate: "platform",
			prefer: true,
		}
		`)

	variant := ctx.ModuleForTests("prebuilt_foo", "android_common")
	a := variant.Module().(*AndroidAppImport)
	// The prebuilt module should still be enabled and active even if the source-based counterpart
	// is disabled.
	if !a.prebuilt.UsePrebuilt() {
		t.Errorf("prebuilt foo module is not active")
	}
	if !a.Enabled() {
		t.Errorf("prebuilt foo module is disabled")
	}
}

func TestAndroidAppImport_frameworkRes(t *testing.T) {
	ctx, config := testJava(t, `
		android_app_import {
			name: "framework-res",
			certificate: "platform",
			apk: "package-res.apk",
			prefer: true,
			export_package_resources: true,
			// Disable dexpreopt and verify_uses_libraries check as the app
			// contains no Java code to be dexpreopted.
			enforce_uses_libs: false,
			dex_preopt: {
				enabled: false,
			},
		}
		`)

	mod := ctx.ModuleForTests("prebuilt_framework-res", "android_common").Module()
	a := mod.(*AndroidAppImport)

	if !a.preprocessed {
		t.Errorf("prebuilt framework-res is not preprocessed")
	}

	expectedInstallPath := buildDir + "/target/product/test_device/system/framework/framework-res.apk"

	if a.dexpreopter.installPath.String() != expectedInstallPath {
		t.Errorf("prebuilt framework-res installed to incorrect location, actual: %s, expected: %s", a.dexpreopter.installPath, expectedInstallPath)

	}

	entries := android.AndroidMkEntriesForTest(t, config, "", mod)[0]

	expectedPath := "."
	// From apk property above, in the root of the source tree.
	expectedPrebuiltModuleFile := "package-res.apk"
	// Verify that the apk is preprocessed: The export package is the same
	// as the prebuilt.
	expectedSoongResourceExportPackage := expectedPrebuiltModuleFile

	actualPath := entries.EntryMap["LOCAL_PATH"]
	actualPrebuiltModuleFile := entries.EntryMap["LOCAL_PREBUILT_MODULE_FILE"]
	actualSoongResourceExportPackage := entries.EntryMap["LOCAL_SOONG_RESOURCE_EXPORT_PACKAGE"]

	if len(actualPath) != 1 {
		t.Errorf("LOCAL_PATH incorrect len %d", len(actualPath))
	} else if actualPath[0] != expectedPath {
		t.Errorf("LOCAL_PATH mismatch, actual: %s, expected: %s", actualPath[0], expectedPath)
	}

	if len(actualPrebuiltModuleFile) != 1 {
		t.Errorf("LOCAL_PREBUILT_MODULE_FILE incorrect len %d", len(actualPrebuiltModuleFile))
	} else if actualPrebuiltModuleFile[0] != expectedPrebuiltModuleFile {
		t.Errorf("LOCAL_PREBUILT_MODULE_FILE mismatch, actual: %s, expected: %s", actualPrebuiltModuleFile[0], expectedPrebuiltModuleFile)
	}

	if len(actualSoongResourceExportPackage) != 1 {
		t.Errorf("LOCAL_SOONG_RESOURCE_EXPORT_PACKAGE incorrect len %d", len(actualSoongResourceExportPackage))
	} else if actualSoongResourceExportPackage[0] != expectedSoongResourceExportPackage {
		t.Errorf("LOCAL_SOONG_RESOURCE_EXPORT_PACKAGE mismatch, actual: %s, expected: %s", actualSoongResourceExportPackage[0], expectedSoongResourceExportPackage)
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
