// Copyright 2022 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package apex

import (
	"android/soong/android"
	"android/soong/android/allowlists"
	"android/soong/bazel/cquery"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestApexImageInMixedBuilds(t *testing.T) {
	bp := `
apex_key{
	name: "foo_key",
}

apex {
	name: "foo",
	key: "foo_key",
	updatable: true,
	min_sdk_version: "31",
	file_contexts: ":myapex-file_contexts",
	bazel_module: { label: "//:foo" },
}`

	outputBaseDir := "out/bazel"
	result := android.GroupFixturePreparers(
		prepareForApexTest,
		android.FixtureModifyConfig(func(config android.Config) {
			config.BazelContext = android.MockBazelContext{
				OutputBaseDir: outputBaseDir,
				LabelToApexInfo: map[string]cquery.ApexInfo{
					"//:foo": cquery.ApexInfo{
						// ApexInfo Starlark provider.
						SignedOutput:           "signed_out.apex",
						SignedCompressedOutput: "signed_out.capex",
						UnsignedOutput:         "unsigned_out.apex",
						BundleKeyInfo:          []string{"public_key", "private_key"},
						ContainerKeyInfo:       []string{"container_cert", "container_private"},
						SymbolsUsedByApex:      "foo_using.txt",
						JavaSymbolsUsedByApex:  "foo_using.xml",
						BundleFile:             "apex_bundle.zip",
						InstalledFiles:         "installed-files.txt",
						RequiresLibs:           []string{"//path/c:c", "//path/d:d"},

						// unused
						PackageName:  "pkg_name",
						ProvidesLibs: []string{"a", "b"},

						// ApexMkInfo Starlark provider
						PayloadFilesInfo: []map[string]string{
							{
								"built_file":       "bazel-out/adbd",
								"install_dir":      "bin",
								"class":            "nativeExecutable",
								"make_module_name": "adbd",
								"basename":         "adbd",
								"package":          "foo",
							},
						},
						MakeModulesToInstall: []string{"c"}, // d deliberately omitted
					},
				},
			}
		}),
	).RunTestWithBp(t, bp)

	m := result.ModuleForTests("foo", "android_common_foo").Module()
	ab, ok := m.(*apexBundle)

	if !ok {
		t.Fatalf("Expected module to be an apexBundle, was not")
	}

	// TODO: refactor to android.AssertStringEquals
	if w, g := "out/bazel/execroot/__main__/public_key", ab.publicKeyFile.String(); w != g {
		t.Errorf("Expected public key %q, got %q", w, g)
	}

	if w, g := "out/bazel/execroot/__main__/private_key", ab.privateKeyFile.String(); w != g {
		t.Errorf("Expected private key %q, got %q", w, g)
	}

	if w, g := "out/bazel/execroot/__main__/container_cert", ab.containerCertificateFile.String(); w != g {
		t.Errorf("Expected public container key %q, got %q", w, g)
	}

	if w, g := "out/bazel/execroot/__main__/container_private", ab.containerPrivateKeyFile.String(); w != g {
		t.Errorf("Expected private container key %q, got %q", w, g)
	}

	if w, g := "out/bazel/execroot/__main__/signed_out.apex", ab.outputFile.String(); w != g {
		t.Errorf("Expected output file %q, got %q", w, g)
	}

	if w, g := "out/bazel/execroot/__main__/foo_using.txt", ab.nativeApisUsedByModuleFile.String(); w != g {
		t.Errorf("Expected output file %q, got %q", w, g)
	}

	if w, g := "out/bazel/execroot/__main__/foo_using.xml", ab.javaApisUsedByModuleFile.String(); w != g {
		t.Errorf("Expected output file %q, got %q", w, g)
	}

	if w, g := "out/bazel/execroot/__main__/installed-files.txt", ab.installedFilesFile.String(); w != g {
		t.Errorf("Expected installed-files.txt %q, got %q", w, g)
	}

	mkData := android.AndroidMkDataForTest(t, result.TestContext, m)
	var builder strings.Builder
	mkData.Custom(&builder, "foo", "BAZEL_TARGET_", "", mkData)

	data := builder.String()
	if w := "ALL_MODULES.$(my_register_name).BUNDLE := out/bazel/execroot/__main__/apex_bundle.zip"; !strings.Contains(data, w) {
		t.Errorf("Expected %q in androidmk data, but did not find %q", w, data)
	}
	if w := "$(call dist-for-goals,checkbuild,out/bazel/execroot/__main__/installed-files.txt:foo-installed-files.txt)"; !strings.Contains(data, w) {
		t.Errorf("Expected %q in androidmk data, but did not find %q", w, data)
	}

	// make modules to be installed to system
	if len(ab.makeModulesToInstall) != 1 && ab.makeModulesToInstall[0] != "c" {
		t.Errorf("Expected makeModulesToInstall slice to only contain 'c', got %q", ab.makeModulesToInstall)
	}
	if w := "LOCAL_REQUIRED_MODULES := adbd.foo c"; !strings.Contains(data, w) {
		t.Errorf("Expected %q in androidmk data, but did not find it in %q", w, data)
	}
}

func TestApexImageCreatesFilesInfoForMake(t *testing.T) {
	bp := `
apex_key{
	name: "foo_key",
}

apex {
	name: "foo",
	key: "foo_key",
	updatable: true,
	min_sdk_version: "31",
	file_contexts: ":myapex-file_contexts",
	bazel_module: { label: "//:foo" },
}`

	outputBaseDir := "out/bazel"
	result := android.GroupFixturePreparers(
		prepareForApexTest,
		android.FixtureModifyConfig(func(config android.Config) {
			config.BazelContext = android.MockBazelContext{
				OutputBaseDir: outputBaseDir,
				LabelToApexInfo: map[string]cquery.ApexInfo{
					"//:foo": {
						// ApexInfo Starlark provider. Necessary for the test.
						SignedOutput:     "signed_out.apex",
						BundleKeyInfo:    []string{"public_key", "private_key"},
						ContainerKeyInfo: []string{"container_cert", "container_private"},

						// ApexMkInfo Starlark provider
						PayloadFilesInfo: []map[string]string{
							{
								"arch":                  "arm64",
								"basename":              "libcrypto.so",
								"built_file":            "bazel-out/64/libcrypto.so",
								"class":                 "nativeSharedLib",
								"install_dir":           "lib64",
								"make_module_name":      "libcrypto",
								"package":               "foo/bar",
								"unstripped_built_file": "bazel-out/64/unstripped_libcrypto.so",
							},
							{
								"arch":             "arm",
								"basename":         "libcrypto.so",
								"built_file":       "bazel-out/32/libcrypto.so",
								"class":            "nativeSharedLib",
								"install_dir":      "lib",
								"make_module_name": "libcrypto",
								"package":          "foo/bar",
							},
							{
								"arch":             "arm64",
								"basename":         "adbd",
								"built_file":       "bazel-out/adbd",
								"class":            "nativeExecutable",
								"install_dir":      "bin",
								"make_module_name": "adbd",
								"package":          "foo",
							},
						},
					},
				},
			}
		}),
	).RunTestWithBp(t, bp)

	m := result.ModuleForTests("foo", "android_common_foo").Module()
	ab, ok := m.(*apexBundle)

	if !ok {
		t.Fatalf("Expected module to be an apexBundle, was not")
	}

	expectedFilesInfo := []apexFile{
		{
			androidMkModuleName: "libcrypto",
			builtFile:           android.PathForTesting("out/bazel/execroot/__main__/bazel-out/64/libcrypto.so"),
			class:               nativeSharedLib,
			customStem:          "libcrypto.so",
			installDir:          "lib64",
			moduleDir:           "foo/bar",
			arch:                "arm64",
			unstrippedBuiltFile: android.PathForTesting("out/bazel/execroot/__main__/bazel-out/64/unstripped_libcrypto.so"),
		},
		{
			androidMkModuleName: "libcrypto",
			builtFile:           android.PathForTesting("out/bazel/execroot/__main__/bazel-out/32/libcrypto.so"),
			class:               nativeSharedLib,
			customStem:          "libcrypto.so",
			installDir:          "lib",
			moduleDir:           "foo/bar",
			arch:                "arm",
		},
		{
			androidMkModuleName: "adbd",
			builtFile:           android.PathForTesting("out/bazel/execroot/__main__/bazel-out/adbd"),
			class:               nativeExecutable,
			customStem:          "adbd",
			installDir:          "bin",
			moduleDir:           "foo",
			arch:                "arm64",
		},
	}

	if len(ab.filesInfo) != len(expectedFilesInfo) {
		t.Errorf("Expected %d entries in ab.filesInfo, but got %d", len(ab.filesInfo), len(expectedFilesInfo))
	}

	for idx, f := range ab.filesInfo {
		expected := expectedFilesInfo[idx]
		android.AssertSame(t, "different class", expected.class, f.class)
		android.AssertStringEquals(t, "different built file", expected.builtFile.String(), f.builtFile.String())
		android.AssertStringEquals(t, "different custom stem", expected.customStem, f.customStem)
		android.AssertStringEquals(t, "different install dir", expected.installDir, f.installDir)
		android.AssertStringEquals(t, "different make module name", expected.androidMkModuleName, f.androidMkModuleName)
		android.AssertStringEquals(t, "different moduleDir", expected.moduleDir, f.moduleDir)
		android.AssertStringEquals(t, "different arch", expected.arch, f.arch)
		if expected.unstrippedBuiltFile != nil {
			if f.unstrippedBuiltFile == nil {
				t.Errorf("expected an unstripped built file path.")
			}
			android.AssertStringEquals(t, "different unstripped built file", expected.unstrippedBuiltFile.String(), f.unstrippedBuiltFile.String())
		}
	}
}

func TestCompressedApexImageInMixedBuilds(t *testing.T) {
	bp := `
apex_key{
	name: "foo_key",
}
apex {
	name: "foo",
	key: "foo_key",
	updatable: true,
	min_sdk_version: "31",
	file_contexts: ":myapex-file_contexts",
	bazel_module: { label: "//:foo" },
	test_only_force_compression: true, // force compression
}`

	outputBaseDir := "out/bazel"
	result := android.GroupFixturePreparers(
		prepareForApexTest,
		android.FixtureModifyConfig(func(config android.Config) {
			config.BazelContext = android.MockBazelContext{
				OutputBaseDir: outputBaseDir,
				LabelToApexInfo: map[string]cquery.ApexInfo{
					"//:foo": cquery.ApexInfo{
						SignedOutput:           "signed_out.apex",
						SignedCompressedOutput: "signed_out.capex",
						BundleKeyInfo:          []string{"public_key", "private_key"},
						ContainerKeyInfo:       []string{"container_cert", "container_private"},
					},
				},
			}
		}),
	).RunTestWithBp(t, bp)

	m := result.ModuleForTests("foo", "android_common_foo").Module()
	ab, ok := m.(*apexBundle)
	if !ok {
		t.Fatalf("Expected module to be an apexBundle, was not")
	}

	if w, g := "out/bazel/execroot/__main__/signed_out.capex", ab.outputFile.String(); w != g {
		t.Errorf("Expected output file to be compressed apex %q, got %q", w, g)
	}

	mkData := android.AndroidMkDataForTest(t, result.TestContext, m)
	var builder strings.Builder
	mkData.Custom(&builder, "foo", "BAZEL_TARGET_", "", mkData)

	data := builder.String()

	expectedAndroidMk := []string{
		"LOCAL_PREBUILT_MODULE_FILE := out/bazel/execroot/__main__/signed_out.capex",

		// Check that the source install file is the capex. The dest is not important.
		"LOCAL_SOONG_INSTALL_PAIRS := out/bazel/execroot/__main__/signed_out.capex:",
	}
	for _, androidMk := range expectedAndroidMk {
		if !strings.Contains(data, androidMk) {
			t.Errorf("Expected %q in androidmk data, but did not find %q", androidMk, data)
		}
	}
}

func TestOverrideApexImageInMixedBuilds(t *testing.T) {
	originalBp := `
apex_key{
	name: "foo_key",
}
apex_key{
	name: "override_foo_key",
}
apex {
	name: "foo",
	key: "foo_key",
	updatable: true,
	min_sdk_version: "31",
	package_name: "pkg_name",
	file_contexts: ":myapex-file_contexts",
	%s
}`
	overrideBp := `
override_apex {
	name: "override_foo",
	key: "override_foo_key",
	package_name: "override_pkg_name",
	base: "foo",
	%s
}
`

	originalApexBpDir := "original"
	originalApexName := "foo"
	overrideApexBpDir := "override"
	overrideApexName := "override_foo"

	defaultApexLabel := fmt.Sprintf("//%s:%s", originalApexBpDir, originalApexName)
	defaultOverrideApexLabel := fmt.Sprintf("//%s:%s", overrideApexBpDir, overrideApexName)

	testCases := []struct {
		desc                    string
		bazelModuleProp         string
		apexLabel               string
		overrideBazelModuleProp string
		overrideApexLabel       string
		bp2buildConfiguration   android.Bp2BuildConversionAllowlist
	}{
		{
			desc:                    "both explicit labels",
			bazelModuleProp:         `bazel_module: { label: "//:foo" },`,
			apexLabel:               "//:foo",
			overrideBazelModuleProp: `bazel_module: { label: "//:override_foo" },`,
			overrideApexLabel:       "//:override_foo",
			bp2buildConfiguration:   android.NewBp2BuildAllowlist(),
		},
		{
			desc:                    "both explicitly allowed",
			bazelModuleProp:         `bazel_module: { bp2build_available: true },`,
			apexLabel:               defaultApexLabel,
			overrideBazelModuleProp: `bazel_module: { bp2build_available: true },`,
			overrideApexLabel:       defaultOverrideApexLabel,
			bp2buildConfiguration:   android.NewBp2BuildAllowlist(),
		},
		{
			desc:              "original allowed by dir, override allowed by name",
			apexLabel:         defaultApexLabel,
			overrideApexLabel: defaultOverrideApexLabel,
			bp2buildConfiguration: android.NewBp2BuildAllowlist().SetDefaultConfig(
				map[string]allowlists.BazelConversionConfigEntry{
					originalApexBpDir: allowlists.Bp2BuildDefaultTrue,
				}).SetModuleAlwaysConvertList([]string{
				overrideApexName,
			}),
		},
		{
			desc:              "both allowed by name",
			apexLabel:         defaultApexLabel,
			overrideApexLabel: defaultOverrideApexLabel,
			bp2buildConfiguration: android.NewBp2BuildAllowlist().SetModuleAlwaysConvertList([]string{
				originalApexName,
				overrideApexName,
			}),
		},
		{
			desc:              "override allowed by name",
			apexLabel:         defaultApexLabel,
			overrideApexLabel: defaultOverrideApexLabel,
			bp2buildConfiguration: android.NewBp2BuildAllowlist().SetModuleAlwaysConvertList([]string{
				overrideApexName,
			}),
		},
		{
			desc:              "override allowed by dir",
			apexLabel:         defaultApexLabel,
			overrideApexLabel: defaultOverrideApexLabel,
			bp2buildConfiguration: android.NewBp2BuildAllowlist().SetDefaultConfig(
				map[string]allowlists.BazelConversionConfigEntry{
					overrideApexBpDir: allowlists.Bp2BuildDefaultTrue,
				}).SetModuleAlwaysConvertList([]string{}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			outputBaseDir := "out/bazel"
			result := android.GroupFixturePreparers(
				prepareForApexTest,
				android.FixtureAddTextFile(filepath.Join(originalApexBpDir, "Android.bp"), fmt.Sprintf(originalBp, tc.bazelModuleProp)),
				android.FixtureAddTextFile(filepath.Join(overrideApexBpDir, "Android.bp"), fmt.Sprintf(overrideBp, tc.overrideBazelModuleProp)),
				android.FixtureModifyContext(func(ctx *android.TestContext) {
					ctx.RegisterBp2BuildConfig(tc.bp2buildConfiguration)
				}),
				android.FixtureModifyConfig(func(config android.Config) {
					config.BazelContext = android.MockBazelContext{
						OutputBaseDir: outputBaseDir,
						LabelToApexInfo: map[string]cquery.ApexInfo{
							tc.apexLabel: cquery.ApexInfo{
								// ApexInfo Starlark provider
								SignedOutput:          "signed_out.apex",
								UnsignedOutput:        "unsigned_out.apex",
								BundleKeyInfo:         []string{"public_key", "private_key"},
								ContainerKeyInfo:      []string{"container_cert", "container_private"},
								SymbolsUsedByApex:     "foo_using.txt",
								JavaSymbolsUsedByApex: "foo_using.xml",
								BundleFile:            "apex_bundle.zip",
								InstalledFiles:        "installed-files.txt",
								RequiresLibs:          []string{"//path/c:c", "//path/d:d"},

								// unused
								PackageName:  "pkg_name",
								ProvidesLibs: []string{"a", "b"},

								// ApexMkInfo Starlark provider
								MakeModulesToInstall: []string{"c"}, // d deliberately omitted
							},
							tc.overrideApexLabel: cquery.ApexInfo{
								// ApexInfo Starlark provider
								SignedOutput:          "override_signed_out.apex",
								UnsignedOutput:        "override_unsigned_out.apex",
								BundleKeyInfo:         []string{"override_public_key", "override_private_key"},
								ContainerKeyInfo:      []string{"override_container_cert", "override_container_private"},
								SymbolsUsedByApex:     "override_foo_using.txt",
								JavaSymbolsUsedByApex: "override_foo_using.xml",
								BundleFile:            "override_apex_bundle.zip",
								InstalledFiles:        "override_installed-files.txt",
								RequiresLibs:          []string{"//path/c:c", "//path/d:d"},

								// unused
								PackageName:  "override_pkg_name",
								ProvidesLibs: []string{"a", "b"},

								// ApexMkInfo Starlark provider
								MakeModulesToInstall: []string{"c"}, // d deliberately omitted
							},
						},
					}
				}),
			).RunTest(t)

			m := result.ModuleForTests("foo", "android_common_override_foo_foo").Module()
			ab, ok := m.(*apexBundle)
			if !ok {
				t.Fatalf("Expected module to be an apexBundle, was not")
			}

			if w, g := "out/bazel/execroot/__main__/override_public_key", ab.publicKeyFile.String(); w != g {
				t.Errorf("Expected public key %q, got %q", w, g)
			}

			if w, g := "out/bazel/execroot/__main__/override_private_key", ab.privateKeyFile.String(); w != g {
				t.Errorf("Expected private key %q, got %q", w, g)
			}

			if w, g := "out/bazel/execroot/__main__/override_container_cert", ab.containerCertificateFile; g != nil && w != g.String() {
				t.Errorf("Expected public container key %q, got %q", w, g)
			}

			if w, g := "out/bazel/execroot/__main__/override_container_private", ab.containerPrivateKeyFile; g != nil && w != g.String() {
				t.Errorf("Expected private container key %q, got %q", w, g)
			}

			if w, g := "out/bazel/execroot/__main__/override_signed_out.apex", ab.outputFile.String(); w != g {
				t.Errorf("Expected output file %q, got %q", w, g)
			}

			if w, g := "out/bazel/execroot/__main__/override_foo_using.txt", ab.nativeApisUsedByModuleFile.String(); w != g {
				t.Errorf("Expected output file %q, got %q", w, g)
			}

			if w, g := "out/bazel/execroot/__main__/override_foo_using.xml", ab.javaApisUsedByModuleFile.String(); w != g {
				t.Errorf("Expected output file %q, got %q", w, g)
			}

			if w, g := "out/bazel/execroot/__main__/override_installed-files.txt", ab.installedFilesFile.String(); w != g {
				t.Errorf("Expected installed-files.txt %q, got %q", w, g)
			}

			mkData := android.AndroidMkDataForTest(t, result.TestContext, m)
			var builder strings.Builder
			mkData.Custom(&builder, "override_foo", "BAZEL_TARGET_", "", mkData)

			data := builder.String()
			if w := "ALL_MODULES.$(my_register_name).BUNDLE := out/bazel/execroot/__main__/override_apex_bundle.zip"; !strings.Contains(data, w) {
				t.Errorf("Expected %q in androidmk data, but did not find %q", w, data)
			}
			if w := "$(call dist-for-goals,checkbuild,out/bazel/execroot/__main__/override_installed-files.txt:override_foo-installed-files.txt)"; !strings.Contains(data, w) {
				t.Errorf("Expected %q in androidmk data, but did not find %q", w, data)
			}

			// make modules to be installed to system
			if len(ab.makeModulesToInstall) != 1 || ab.makeModulesToInstall[0] != "c" {
				t.Errorf("Expected makeModulestoInstall slice to only contain 'c', got %q", ab.makeModulesToInstall)
			}
			if w := "LOCAL_REQUIRED_MODULES := c"; !strings.Contains(data, w) {
				t.Errorf("Expected %q in androidmk data, but did not find it in %q", w, data)
			}
		})
	}
}
