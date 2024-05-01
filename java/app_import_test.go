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
	"fmt"
	"reflect"
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
	if variant.MaybeOutput("dexpreopt/foo/oat/arm64/package.vdex").Rule == nil ||
		variant.MaybeOutput("dexpreopt/foo/oat/arm64/package.odex").Rule == nil {
		t.Errorf("can't find dexpreopt outputs")
	}

	// Check cert signing flag.
	signedApk := variant.Output("signed/foo.apk")
	signingFlag := signedApk.Args["certificates"]
	expected := "build/make/target/product/security/platform.x509.pem build/make/target/product/security/platform.pk8"
	if expected != signingFlag {
		t.Errorf("Incorrect signing flags, expected: %q, got: %q", expected, signingFlag)
	}
	rule := variant.Rule("genProvenanceMetaData")
	android.AssertStringEquals(t, "Invalid input", "prebuilts/apk/app.apk", rule.Inputs[0].String())
	android.AssertStringEquals(t, "Invalid output", "out/soong/.intermediates/provenance_metadata/foo/provenance_metadata.textproto", rule.Output.String())
	android.AssertStringEquals(t, "Invalid args", "foo", rule.Args["module_name"])
	android.AssertStringEquals(t, "Invalid args", "/system/app/foo/foo.apk", rule.Args["install_path"])
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
	if variant.MaybeOutput("dexpreopt/foo/oat/arm64/package.vdex").Rule != nil ||
		variant.MaybeOutput("dexpreopt/foo/oat/arm64/package.odex").Rule != nil {
		t.Errorf("dexpreopt shouldn't have run.")
	}

	rule := variant.Rule("genProvenanceMetaData")
	android.AssertStringEquals(t, "Invalid input", "prebuilts/apk/app.apk", rule.Inputs[0].String())
	android.AssertStringEquals(t, "Invalid output", "out/soong/.intermediates/provenance_metadata/foo/provenance_metadata.textproto", rule.Output.String())
	android.AssertStringEquals(t, "Invalid args", "foo", rule.Args["module_name"])
	android.AssertStringEquals(t, "Invalid args", "/system/app/foo/foo.apk", rule.Args["install_path"])
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
	if variant.MaybeOutput("dexpreopt/foo/oat/arm64/package.vdex").Rule == nil ||
		variant.MaybeOutput("dexpreopt/foo/oat/arm64/package.odex").Rule == nil {
		t.Errorf("can't find dexpreopt outputs")
	}
	// Make sure signing was skipped and aligning was done.
	if variant.MaybeOutput("signed/foo.apk").Rule != nil {
		t.Errorf("signing rule shouldn't be included.")
	}
	if variant.MaybeOutput("zip-aligned/foo.apk").Rule == nil {
		t.Errorf("can't find aligning rule")
	}

	rule := variant.Rule("genProvenanceMetaData")
	android.AssertStringEquals(t, "Invalid input", "prebuilts/apk/app.apk", rule.Inputs[0].String())
	android.AssertStringEquals(t, "Invalid output", "out/soong/.intermediates/provenance_metadata/foo/provenance_metadata.textproto", rule.Output.String())
	android.AssertStringEquals(t, "Invalid args", "foo", rule.Args["module_name"])
	android.AssertStringEquals(t, "Invalid args", "/system/app/foo/foo.apk", rule.Args["install_path"])
}

func TestAndroidAppImport_SigningLineage(t *testing.T) {
	ctx, _ := testJava(t, `
	  android_app_import {
			name: "foo",
			apk: "prebuilts/apk/app.apk",
			certificate: "platform",
			additional_certificates: [":additional_certificate"],
			lineage: "lineage.bin",
			rotationMinSdkVersion: "32",
		}

		android_app_certificate {
			name: "additional_certificate",
			certificate: "cert/additional_cert",
		}
	`)

	variant := ctx.ModuleForTests("foo", "android_common")

	signedApk := variant.Output("signed/foo.apk")
	// Check certificates
	certificatesFlag := signedApk.Args["certificates"]
	expected := "build/make/target/product/security/platform.x509.pem " +
		"build/make/target/product/security/platform.pk8 " +
		"cert/additional_cert.x509.pem cert/additional_cert.pk8"
	if expected != certificatesFlag {
		t.Errorf("Incorrect certificates flags, expected: %q, got: %q", expected, certificatesFlag)
	}

	// Check cert signing flags.
	actualCertSigningFlags := signedApk.Args["flags"]
	expectedCertSigningFlags := "--lineage lineage.bin --rotation-min-sdk-version 32"
	if expectedCertSigningFlags != actualCertSigningFlags {
		t.Errorf("Incorrect signing flags, expected: %q, got: %q", expectedCertSigningFlags, actualCertSigningFlags)
	}

	rule := variant.Rule("genProvenanceMetaData")
	android.AssertStringEquals(t, "Invalid input", "prebuilts/apk/app.apk", rule.Inputs[0].String())
	android.AssertStringEquals(t, "Invalid output", "out/soong/.intermediates/provenance_metadata/foo/provenance_metadata.textproto", rule.Output.String())
	android.AssertStringEquals(t, "Invalid args", "foo", rule.Args["module_name"])
	android.AssertStringEquals(t, "Invalid args", "/system/app/foo/foo.apk", rule.Args["install_path"])
}

func TestAndroidAppImport_SigningLineageFilegroup(t *testing.T) {
	ctx, _ := testJava(t, `
	  android_app_import {
			name: "foo",
			apk: "prebuilts/apk/app.apk",
			certificate: "platform",
			lineage: ":lineage_bin",
		}

		filegroup {
			name: "lineage_bin",
			srcs: ["lineage.bin"],
		}
	`)

	variant := ctx.ModuleForTests("foo", "android_common")

	signedApk := variant.Output("signed/foo.apk")
	// Check cert signing lineage flag.
	signingFlag := signedApk.Args["flags"]
	expected := "--lineage lineage.bin"
	if expected != signingFlag {
		t.Errorf("Incorrect signing flags, expected: %q, got: %q", expected, signingFlag)
	}

	rule := variant.Rule("genProvenanceMetaData")
	android.AssertStringEquals(t, "Invalid input", "prebuilts/apk/app.apk", rule.Inputs[0].String())
	android.AssertStringEquals(t, "Invalid output", "out/soong/.intermediates/provenance_metadata/foo/provenance_metadata.textproto", rule.Output.String())
	android.AssertStringEquals(t, "Invalid args", "foo", rule.Args["module_name"])
	android.AssertStringEquals(t, "Invalid args", "/system/app/foo/foo.apk", rule.Args["install_path"])
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
	if variant.MaybeOutput("dexpreopt/foo/oat/arm64/package.vdex").Rule == nil ||
		variant.MaybeOutput("dexpreopt/foo/oat/arm64/package.odex").Rule == nil {
		t.Errorf("can't find dexpreopt outputs")
	}

	// Check cert signing flag.
	signedApk := variant.Output("signed/foo.apk")
	signingFlag := signedApk.Args["certificates"]
	expected := "build/make/target/product/security/testkey.x509.pem build/make/target/product/security/testkey.pk8"
	if expected != signingFlag {
		t.Errorf("Incorrect signing flags, expected: %q, got: %q", expected, signingFlag)
	}

	rule := variant.Rule("genProvenanceMetaData")
	android.AssertStringEquals(t, "Invalid input", "prebuilts/apk/app.apk", rule.Inputs[0].String())
	android.AssertStringEquals(t, "Invalid output", "out/soong/.intermediates/provenance_metadata/foo/provenance_metadata.textproto", rule.Output.String())
	android.AssertStringEquals(t, "Invalid args", "foo", rule.Args["module_name"])
	android.AssertStringEquals(t, "Invalid args", "/system/app/foo/foo.apk", rule.Args["install_path"])
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
		name                                   string
		aaptPreferredConfig                    *string
		aaptPrebuiltDPI                        []string
		expected                               string
		expectedProvenanceMetaDataArtifactPath string
	}{
		{
			name:                                   "no preferred",
			aaptPreferredConfig:                    nil,
			aaptPrebuiltDPI:                        []string{},
			expected:                               "verify_uses_libraries/apk/app.apk",
			expectedProvenanceMetaDataArtifactPath: "prebuilts/apk/app.apk",
		},
		{
			name:                                   "AAPTPreferredConfig matches",
			aaptPreferredConfig:                    proptools.StringPtr("xhdpi"),
			aaptPrebuiltDPI:                        []string{"xxhdpi", "ldpi"},
			expected:                               "verify_uses_libraries/apk/app_xhdpi.apk",
			expectedProvenanceMetaDataArtifactPath: "prebuilts/apk/app_xhdpi.apk",
		},
		{
			name:                                   "AAPTPrebuiltDPI matches",
			aaptPreferredConfig:                    proptools.StringPtr("mdpi"),
			aaptPrebuiltDPI:                        []string{"xxhdpi", "xhdpi"},
			expected:                               "verify_uses_libraries/apk/app_xxhdpi.apk",
			expectedProvenanceMetaDataArtifactPath: "prebuilts/apk/app_xxhdpi.apk",
		},
		{
			name:                                   "non-first AAPTPrebuiltDPI matches",
			aaptPreferredConfig:                    proptools.StringPtr("mdpi"),
			aaptPrebuiltDPI:                        []string{"ldpi", "xhdpi"},
			expected:                               "verify_uses_libraries/apk/app_xhdpi.apk",
			expectedProvenanceMetaDataArtifactPath: "prebuilts/apk/app_xhdpi.apk",
		},
		{
			name:                                   "no matches",
			aaptPreferredConfig:                    proptools.StringPtr("mdpi"),
			aaptPrebuiltDPI:                        []string{"ldpi", "xxxhdpi"},
			expected:                               "verify_uses_libraries/apk/app.apk",
			expectedProvenanceMetaDataArtifactPath: "prebuilts/apk/app.apk",
		},
	}

	for _, test := range testCases {
		result := android.GroupFixturePreparers(
			PrepareForTestWithJavaDefaultModules,
			android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				variables.AAPTPreferredConfig = test.aaptPreferredConfig
				variables.AAPTPrebuiltDPI = test.aaptPrebuiltDPI
			}),
		).RunTestWithBp(t, bp)

		variant := result.ModuleForTests("foo", "android_common")
		input := variant.Output("jnis-uncompressed/foo.apk").Input.String()
		if strings.HasSuffix(input, test.expected) {
			t.Errorf("wrong src apk, expected: %q got: %q", test.expected, input)
		}

		provenanceMetaDataRule := variant.Rule("genProvenanceMetaData")
		android.AssertStringEquals(t, "Invalid input", test.expectedProvenanceMetaDataArtifactPath, provenanceMetaDataRule.Inputs[0].String())
		android.AssertStringEquals(t, "Invalid output", "out/soong/.intermediates/provenance_metadata/foo/provenance_metadata.textproto", provenanceMetaDataRule.Output.String())
		android.AssertStringEquals(t, "Invalid args", "foo", provenanceMetaDataRule.Args["module_name"])
		android.AssertStringEquals(t, "Invalid args", "/system/app/foo/foo.apk", provenanceMetaDataRule.Args["install_path"])
	}
}

func TestAndroidAppImport_Filename(t *testing.T) {
	ctx, _ := testJava(t, `
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
		name                 string
		expected             string
		onDevice             string
		expectedArtifactPath string
		expectedMetaDataPath string
	}{
		{
			name:                 "foo",
			expected:             "foo.apk",
			onDevice:             "/system/app/foo/foo.apk",
			expectedArtifactPath: "prebuilts/apk/app.apk",
			expectedMetaDataPath: "out/soong/.intermediates/provenance_metadata/foo/provenance_metadata.textproto",
		},
		{
			name:                 "bar",
			expected:             "bar_sample.apk",
			onDevice:             "/system/app/bar/bar_sample.apk",
			expectedArtifactPath: "prebuilts/apk/app.apk",
			expectedMetaDataPath: "out/soong/.intermediates/provenance_metadata/bar/provenance_metadata.textproto",
		},
	}

	for _, test := range testCases {
		variant := ctx.ModuleForTests(test.name, "android_common")
		if variant.MaybeOutput(test.expected).Rule == nil {
			t.Errorf("can't find output named %q - all outputs: %v", test.expected, variant.AllOutputs())
		}

		a := variant.Module().(*AndroidAppImport)
		expectedValues := []string{test.expected}
		entries := android.AndroidMkEntriesForTest(t, ctx, a)[0]
		actualValues := entries.EntryMap["LOCAL_INSTALLED_MODULE_STEM"]
		if !reflect.DeepEqual(actualValues, expectedValues) {
			t.Errorf("Incorrect LOCAL_INSTALLED_MODULE_STEM value '%s', expected '%s'",
				actualValues, expectedValues)
		}
		android.AssertStringEquals(t, "unexpected LOCAL_SOONG_MODULE_TYPE", "android_app_import", entries.EntryMap["LOCAL_SOONG_MODULE_TYPE"][0])

		rule := variant.Rule("genProvenanceMetaData")
		android.AssertStringEquals(t, "Invalid input", test.expectedArtifactPath, rule.Inputs[0].String())
		android.AssertStringEquals(t, "Invalid output", test.expectedMetaDataPath, rule.Output.String())
		android.AssertStringEquals(t, "Invalid args", test.name, rule.Args["module_name"])
		android.AssertStringEquals(t, "Invalid args", test.onDevice, rule.Args["install_path"])
	}
}

func TestAndroidAppImport_ArchVariants(t *testing.T) {
	// The test config's target arch is ARM64.
	testCases := []struct {
		name         string
		bp           string
		expected     string
		artifactPath string
		metaDataPath string
		installPath  string
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
			expected:     "verify_uses_libraries/apk/app_arm64.apk",
			artifactPath: "prebuilts/apk/app_arm64.apk",
			installPath:  "/system/app/foo/foo.apk",
		},
		{
			name: "matching arch without default",
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
			expected:     "verify_uses_libraries/apk/app_arm64.apk",
			artifactPath: "prebuilts/apk/app_arm64.apk",
			installPath:  "/system/app/foo/foo.apk",
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
			expected:     "verify_uses_libraries/apk/app.apk",
			artifactPath: "prebuilts/apk/app.apk",
			installPath:  "/system/app/foo/foo.apk",
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
			expected:     "",
			artifactPath: "prebuilts/apk/app_arm.apk",
			installPath:  "/system/app/foo/foo.apk",
		},
		{
			name: "matching arch and dpi_variants",
			bp: `
				android_app_import {
					name: "foo",
					apk: "prebuilts/apk/app.apk",
					arch: {
						arm64: {
							apk: "prebuilts/apk/app_arm64.apk",
							dpi_variants: {
								mdpi: {
									apk: "prebuilts/apk/app_arm64_mdpi.apk",
								},
								xhdpi: {
									apk: "prebuilts/apk/app_arm64_xhdpi.apk",
								},
							},
						},
					},
					presigned: true,
					dex_preopt: {
						enabled: true,
					},
				}
			`,
			expected:     "verify_uses_libraries/apk/app_arm64_xhdpi.apk",
			artifactPath: "prebuilts/apk/app_arm64_xhdpi.apk",
			installPath:  "/system/app/foo/foo.apk",
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := testJava(t, test.bp)

			variant := ctx.ModuleForTests("foo", "android_common")
			if test.expected == "" {
				if variant.Module().Enabled() {
					t.Error("module should have been disabled, but wasn't")
				}
				rule := variant.MaybeRule("genProvenanceMetaData")
				android.AssertDeepEquals(t, "Provenance metadata is not empty", android.TestingBuildParams{}, rule)
				return
			}
			input := variant.Output("jnis-uncompressed/foo.apk").Input.String()
			if strings.HasSuffix(input, test.expected) {
				t.Errorf("wrong src apk, expected: %q got: %q", test.expected, input)
			}
			rule := variant.Rule("genProvenanceMetaData")
			android.AssertStringEquals(t, "Invalid input", test.artifactPath, rule.Inputs[0].String())
			android.AssertStringEquals(t, "Invalid output", "out/soong/.intermediates/provenance_metadata/foo/provenance_metadata.textproto", rule.Output.String())
			android.AssertStringEquals(t, "Invalid args", "foo", rule.Args["module_name"])
			android.AssertStringEquals(t, "Invalid args", test.installPath, rule.Args["install_path"])
		})
	}
}

func TestAndroidAppImport_SoongConfigVariables(t *testing.T) {
	testCases := []struct {
		name         string
		bp           string
		expected     string
		artifactPath string
		metaDataPath string
		installPath  string
	}{
		{
			name: "matching arch",
			bp: `
				soong_config_module_type {
					name: "my_android_app_import",
					module_type: "android_app_import",
					config_namespace: "my_namespace",
					value_variables: ["my_apk_var"],
					properties: ["apk"],
				}
				soong_config_value_variable {
					name: "my_apk_var",
				}
				my_android_app_import {
					name: "foo",
					soong_config_variables: {
						my_apk_var: {
							apk: "prebuilts/apk/%s.apk",
						},
					},
					presigned: true,
					dex_preopt: {
						enabled: true,
					},
				}
			`,
			expected:     "verify_uses_libraries/apk/name_from_soong_config.apk",
			artifactPath: "prebuilts/apk/name_from_soong_config.apk",
			installPath:  "/system/app/foo/foo.apk",
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			ctx := android.GroupFixturePreparers(
				prepareForJavaTest,
				android.PrepareForTestWithSoongConfigModuleBuildComponents,
				android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
					variables.VendorVars = map[string]map[string]string{
						"my_namespace": {
							"my_apk_var": "name_from_soong_config",
						},
					}
				}),
			).RunTestWithBp(t, test.bp).TestContext

			variant := ctx.ModuleForTests("foo", "android_common")
			if test.expected == "" {
				if variant.Module().Enabled() {
					t.Error("module should have been disabled, but wasn't")
				}
				rule := variant.MaybeRule("genProvenanceMetaData")
				android.AssertDeepEquals(t, "Provenance metadata is not empty", android.TestingBuildParams{}, rule)
				return
			}
			input := variant.Output("jnis-uncompressed/foo.apk").Input.String()
			if strings.HasSuffix(input, test.expected) {
				t.Errorf("wrong src apk, expected: %q got: %q", test.expected, input)
			}
			rule := variant.Rule("genProvenanceMetaData")
			android.AssertStringEquals(t, "Invalid input", test.artifactPath, rule.Inputs[0].String())
			android.AssertStringEquals(t, "Invalid output", "out/soong/.intermediates/provenance_metadata/foo/provenance_metadata.textproto", rule.Output.String())
			android.AssertStringEquals(t, "Invalid args", "foo", rule.Args["module_name"])
			android.AssertStringEquals(t, "Invalid args", test.installPath, rule.Args["install_path"])
		})
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

func TestAndroidAppImport_relativeInstallPath(t *testing.T) {
	bp := `
		android_app_import {
			name: "no_relative_install_path",
			apk: "prebuilts/apk/app.apk",
			presigned: true,
		}

		android_app_import {
			name: "relative_install_path",
			apk: "prebuilts/apk/app.apk",
			presigned: true,
			relative_install_path: "my/path",
		}

		android_app_import {
			name: "privileged_relative_install_path",
			apk: "prebuilts/apk/app.apk",
			presigned: true,
			privileged: true,
			relative_install_path: "my/path"
		}
		`
	testCases := []struct {
		name                string
		expectedInstallPath string
		errorMessage        string
	}{
		{
			name:                "no_relative_install_path",
			expectedInstallPath: "out/soong/target/product/test_device/system/app/no_relative_install_path/no_relative_install_path.apk",
			errorMessage:        "Install path is not correct when relative_install_path is missing",
		},
		{
			name:                "relative_install_path",
			expectedInstallPath: "out/soong/target/product/test_device/system/app/my/path/relative_install_path/relative_install_path.apk",
			errorMessage:        "Install path is not correct for app when relative_install_path is present",
		},
		{
			name:                "privileged_relative_install_path",
			expectedInstallPath: "out/soong/target/product/test_device/system/priv-app/my/path/privileged_relative_install_path/privileged_relative_install_path.apk",
			errorMessage:        "Install path is not correct for privileged app when relative_install_path is present",
		},
	}
	for _, testCase := range testCases {
		ctx, _ := testJava(t, bp)
		mod := ctx.ModuleForTests(testCase.name, "android_common").Module().(*AndroidAppImport)
		android.AssertPathRelativeToTopEquals(t, testCase.errorMessage, testCase.expectedInstallPath, mod.installPath)
	}
}

func TestAndroidTestImport(t *testing.T) {
	ctx, _ := testJava(t, `
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
	entries := android.AndroidMkEntriesForTest(t, ctx, test)[0]
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
	jniRule := variant.Output("jnis-uncompressed/foo.apk").BuildParams.Rule.String()
	if jniRule == android.Cp.String() {
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
		`)

	apkName := "foo.apk"
	variant := ctx.ModuleForTests("foo", "android_common")
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

func TestAndroidAppImport_Preprocessed(t *testing.T) {
	ctx, _ := testJava(t, `
		android_app_import {
			name: "foo",
			apk: "prebuilts/apk/app.apk",
			presigned: true,
			preprocessed: true,
		}
		`)

	apkName := "foo.apk"
	variant := ctx.ModuleForTests("foo", "android_common")
	outputBuildParams := variant.Output(apkName).BuildParams
	if outputBuildParams.Rule.String() != android.Cp.String() {
		t.Errorf("Unexpected prebuilt android_app_import rule: " + outputBuildParams.Rule.String())
	}

	// Make sure compression and aligning were validated.
	if outputBuildParams.Validation == nil {
		t.Errorf("Expected validation rule, but was not found")
	}

	validationBuildParams := variant.Output("validated-prebuilt/check.stamp").BuildParams
	if validationBuildParams.Rule.String() != checkPresignedApkRule.String() {
		t.Errorf("Unexpected validation rule: " + validationBuildParams.Rule.String())
	}
}

func TestAndroidTestImport_UncompressDex(t *testing.T) {
	testCases := []struct {
		name string
		bp   string
	}{
		{
			name: "normal",
			bp: `
				android_app_import {
					name: "foo",
					presigned: true,
					apk: "prebuilts/apk/app.apk",
				}
			`,
		},
		{
			name: "privileged",
			bp: `
				android_app_import {
					name: "foo",
					presigned: true,
					privileged: true,
					apk: "prebuilts/apk/app.apk",
				}
			`,
		},
	}

	test := func(t *testing.T, bp string, unbundled bool, dontUncompressPrivAppDexs bool) {
		t.Helper()

		result := android.GroupFixturePreparers(
			prepareForJavaTest,
			android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				if unbundled {
					variables.Unbundled_build = proptools.BoolPtr(true)
				}
				variables.UncompressPrivAppDex = proptools.BoolPtr(!dontUncompressPrivAppDexs)
			}),
		).RunTestWithBp(t, bp)

		foo := result.ModuleForTests("foo", "android_common")
		actual := foo.MaybeRule("uncompress-dex").Rule != nil

		expect := !unbundled
		if strings.Contains(bp, "privileged: true") {
			if dontUncompressPrivAppDexs {
				expect = false
			} else {
				// TODO(b/194504107): shouldn't priv-apps be always uncompressed unless
				// DONT_UNCOMPRESS_PRIV_APPS_DEXS is true (regardless of unbundling)?
				// expect = true
			}
		}

		android.AssertBoolEquals(t, "uncompress dex", expect, actual)
	}

	for _, unbundled := range []bool{false, true} {
		for _, dontUncompressPrivAppDexs := range []bool{false, true} {
			for _, tt := range testCases {
				name := fmt.Sprintf("%s,unbundled:%t,dontUncompressPrivAppDexs:%t",
					tt.name, unbundled, dontUncompressPrivAppDexs)
				t.Run(name, func(t *testing.T) {
					test(t, tt.bp, unbundled, dontUncompressPrivAppDexs)
				})
			}
		}
	}
}

func TestAppImportMissingCertificateAllowMissingDependencies(t *testing.T) {
	result := android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
		android.PrepareForTestWithAllowMissingDependencies,
		android.PrepareForTestWithAndroidMk,
	).RunTestWithBp(t, `
		android_app_import {
			name: "foo",
			apk: "a.apk",
			certificate: ":missing_certificate",
		}`)

	foo := result.ModuleForTests("foo", "android_common")
	fooApk := foo.Output("signed/foo.apk")
	if fooApk.Rule != android.ErrorRule {
		t.Fatalf("expected ErrorRule for foo.apk, got %s", fooApk.Rule.String())
	}
	android.AssertStringDoesContain(t, "expected error rule message", fooApk.Args["error"], "missing dependencies: missing_certificate\n")
}
