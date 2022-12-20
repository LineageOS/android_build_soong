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
	"android/soong/bazel/cquery"
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
					},
				},
			}
		}),
	).RunTestWithBp(t, bp)

	m := result.ModuleForTests("foo", "android_common_foo_image").Module()
	ab, ok := m.(*apexBundle)
	if !ok {
		t.Fatalf("Expected module to be an apexBundle, was not")
	}

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
	if w := "LOCAL_REQUIRED_MODULES := c d"; !strings.Contains(data, w) {
		t.Errorf("Expected %q in androidmk data, but did not find it in %q", w, data)
	}
}

func TestOverrideApexImageInMixedBuilds(t *testing.T) {
	bp := `
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
	bazel_module: { label: "//:foo" },
}
override_apex {
	name: "override_foo",
	key: "override_foo_key",
	package_name: "override_pkg_name",
	base: "foo",
	bazel_module: { label: "//:override_foo" },
}
`

	outputBaseDir := "out/bazel"
	result := android.GroupFixturePreparers(
		prepareForApexTest,
		android.FixtureModifyConfig(func(config android.Config) {
			config.BazelContext = android.MockBazelContext{
				OutputBaseDir: outputBaseDir,
				LabelToApexInfo: map[string]cquery.ApexInfo{
					"//:foo": cquery.ApexInfo{
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
					},
					"//:override_foo": cquery.ApexInfo{
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
					},
				},
			}
		}),
	).RunTestWithBp(t, bp)

	m := result.ModuleForTests("foo", "android_common_override_foo_foo_image").Module()
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

	if w, g := "out/bazel/execroot/__main__/override_container_cert", ab.containerCertificateFile.String(); w != g {
		t.Errorf("Expected public container key %q, got %q", w, g)
	}

	if w, g := "out/bazel/execroot/__main__/override_container_private", ab.containerPrivateKeyFile.String(); w != g {
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
	if w := "LOCAL_REQUIRED_MODULES := c d"; !strings.Contains(data, w) {
		t.Errorf("Expected %q in androidmk data, but did not find it in %q", w, data)
	}
}
