// Copyright 2021 Google Inc. All rights reserved.
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
	_ "fmt"
	_ "sort"

	"testing"

	"android/soong/android"
	"android/soong/multitree"

	"github.com/google/blueprint"
)

func TestCcApiStubLibraryOutputFiles(t *testing.T) {
	bp := `
		cc_api_stub_library {
			name: "foo",
			symbol_file: "foo.map.txt",
			first_version: "29",
		}
	`
	result := prepareForCcTest.RunTestWithBp(t, bp)
	outputs := result.ModuleForTests("foo", "android_arm64_armv8-a_shared").AllOutputs()
	expected_file_suffixes := []string{".c", "stub.map", ".o", ".so"}
	for _, expected_file_suffix := range expected_file_suffixes {
		android.AssertBoolEquals(t, expected_file_suffix+" file not found in output", true, android.SuffixInList(outputs, expected_file_suffix))
	}
}

func TestCcApiStubLibraryVariants(t *testing.T) {
	bp := `
		cc_api_stub_library {
			name: "foo",
			symbol_file: "foo.map.txt",
			first_version: "29",
		}
	`
	result := prepareForCcTest.RunTestWithBp(t, bp)
	variants := result.ModuleVariantsForTests("foo")
	expected_variants := []string{"29", "30", "S", "Tiramisu"} //TODO: make this test deterministic by using fixtures
	for _, expected_variant := range expected_variants {
		android.AssertBoolEquals(t, expected_variant+" variant not found in foo", true, android.SubstringInList(variants, expected_variant))
	}
}

func TestCcLibraryUsesCcApiStubLibrary(t *testing.T) {
	bp := `
		cc_api_stub_library {
			name: "foo",
			symbol_file: "foo.map.txt",
			first_version: "29",
		}
		cc_library {
			name: "foo_user",
			shared_libs: [
				"foo#29",
			],
		}

	`
	prepareForCcTest.RunTestWithBp(t, bp)
}

func TestApiSurfaceOutputs(t *testing.T) {
	bp := `
		api_surface {
			name: "mysdk",
			contributions: [
				"foo",
			],
		}

		cc_api_contribution {
			name: "foo",
			symbol_file: "foo.map.txt",
			first_version: "29",
		}
	`
	result := android.GroupFixturePreparers(
		prepareForCcTest,
		multitree.PrepareForTestWithApiSurface,
	).RunTestWithBp(t, bp)
	mysdk := result.ModuleForTests("mysdk", "")

	actual_surface_inputs := mysdk.Rule("phony").BuildParams.Inputs.Strings()
	expected_file_suffixes := []string{"mysdk/foo/foo.map.txt"}
	for _, expected_file_suffix := range expected_file_suffixes {
		android.AssertBoolEquals(t, expected_file_suffix+" file not found in input", true, android.SuffixInList(actual_surface_inputs, expected_file_suffix))
	}

	// check args/inputs to rule
	/*api_surface_gen_rule_args := result.ModuleForTests("mysdk", "").Rule("genApiSurfaceBuildFiles").Args
	android.AssertStringEquals(t, "name", "foo.mysdk", api_surface_gen_rule_args["name"])
	android.AssertStringEquals(t, "symbol_file", "foo.map.txt", api_surface_gen_rule_args["symbol_file"])*/
}

func hasDirectDependency(t *testing.T, ctx *android.TestResult, from android.Module, to android.Module) bool {
	t.Helper()
	var found bool
	ctx.VisitDirectDeps(from, func(dep blueprint.Module) {
		if dep == to {
			found = true
		}
	})
	return found
}

func TestApiLibraryReplacesExistingModule(t *testing.T) {
	bp := `
		cc_library {
			name: "libfoo",
			shared_libs: ["libbar"],
		}

		cc_library {
			name: "libbar",
		}

		cc_api_library {
			name: "libbar",
			src: "libbar.so",
		}

		api_imports {
			name: "api_imports",
			shared_libs: [
				"libbar",
			],
			header_libs: [],
		}
	`

	ctx := prepareForCcTest.RunTestWithBp(t, bp)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
	libbar := ctx.ModuleForTests("libbar", "android_arm64_armv8-a_shared").Module()
	libbarApiImport := ctx.ModuleForTests("libbar.apiimport", "android_arm64_armv8-a_shared").Module()

	android.AssertBoolEquals(t, "original library should not be linked", false, hasDirectDependency(t, ctx, libfoo, libbar))
	android.AssertBoolEquals(t, "Stub library from API surface should be linked", true, hasDirectDependency(t, ctx, libfoo, libbarApiImport))
}

func TestApiLibraryDoNotRequireOriginalModule(t *testing.T) {
	bp := `
		cc_library {
			name: "libfoo",
			shared_libs: ["libbar"],
		}

		cc_api_library {
			name: "libbar",
			src: "libbar.so",
		}

		api_imports {
			name: "api_imports",
			shared_libs: [
				"libbar",
			],
			header_libs: [],
		}
	`

	ctx := prepareForCcTest.RunTestWithBp(t, bp)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
	libbarApiImport := ctx.ModuleForTests("libbar.apiimport", "android_arm64_armv8-a_shared").Module()

	android.AssertBoolEquals(t, "Stub library from API surface should be linked", true, hasDirectDependency(t, ctx, libfoo, libbarApiImport))
}

func TestApiLibraryShouldNotReplaceWithoutApiImport(t *testing.T) {
	bp := `
		cc_library {
			name: "libfoo",
			shared_libs: ["libbar"],
		}

		cc_library {
			name: "libbar",
		}

		cc_api_library {
			name: "libbar",
			src: "libbar.so",
		}

		api_imports {
			name: "api_imports",
			shared_libs: [],
			header_libs: [],
		}
	`

	ctx := prepareForCcTest.RunTestWithBp(t, bp)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
	libbar := ctx.ModuleForTests("libbar", "android_arm64_armv8-a_shared").Module()
	libbarApiImport := ctx.ModuleForTests("libbar.apiimport", "android_arm64_armv8-a_shared").Module()

	android.AssertBoolEquals(t, "original library should be linked", true, hasDirectDependency(t, ctx, libfoo, libbar))
	android.AssertBoolEquals(t, "Stub library from API surface should not be linked", false, hasDirectDependency(t, ctx, libfoo, libbarApiImport))
}

func TestApiHeaderReplacesExistingModule(t *testing.T) {
	bp := `
		cc_library {
			name: "libfoo",
			header_libs: ["libfoo_headers"],
		}

		cc_api_library {
			name: "libfoo",
			header_libs: ["libfoo_headers"],
			src: "libfoo.so",
		}

		cc_library_headers {
			name: "libfoo_headers",
		}

		cc_api_headers {
			name: "libfoo_headers",
		}

		api_imports {
			name: "api_imports",
			shared_libs: [
				"libfoo",
			],
			header_libs: [
				"libfoo_headers",
			],
		}
	`

	ctx := prepareForCcTest.RunTestWithBp(t, bp)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
	libfooApiImport := ctx.ModuleForTests("libfoo.apiimport", "android_arm64_armv8-a_shared").Module()
	libfooHeader := ctx.ModuleForTests("libfoo_headers", "android_arm64_armv8-a").Module()
	libfooHeaderApiImport := ctx.ModuleForTests("libfoo_headers.apiimport", "android_arm64_armv8-a").Module()

	android.AssertBoolEquals(t, "original header should not be used for original library", false, hasDirectDependency(t, ctx, libfoo, libfooHeader))
	android.AssertBoolEquals(t, "Header from API surface should be used for original library", true, hasDirectDependency(t, ctx, libfoo, libfooHeaderApiImport))
	android.AssertBoolEquals(t, "original header should not be used for library imported from API surface", false, hasDirectDependency(t, ctx, libfooApiImport, libfooHeader))
	android.AssertBoolEquals(t, "Header from API surface should be used for library imported from API surface", true, hasDirectDependency(t, ctx, libfooApiImport, libfooHeaderApiImport))
}

func TestApiHeadersDoNotRequireOriginalModule(t *testing.T) {
	bp := `
	cc_library {
		name: "libfoo",
		header_libs: ["libfoo_headers"],
	}

	cc_api_headers {
		name: "libfoo_headers",
	}

	api_imports {
		name: "api_imports",
		shared_libs: [
			"libfoo",
		],
		header_libs: [
			"libfoo_headers",
		],
	}
	`

	ctx := prepareForCcTest.RunTestWithBp(t, bp)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
	libfooHeaderApiImport := ctx.ModuleForTests("libfoo_headers.apiimport", "android_arm64_armv8-a").Module()

	android.AssertBoolEquals(t, "Header from API surface should be used for original library", true, hasDirectDependency(t, ctx, libfoo, libfooHeaderApiImport))
}

func TestApiHeadersShouldNotReplaceWithoutApiImport(t *testing.T) {
	bp := `
		cc_library {
			name: "libfoo",
			header_libs: ["libfoo_headers"],
		}

		cc_library_headers {
			name: "libfoo_headers",
		}

		cc_api_headers {
			name: "libfoo_headers",
		}

		api_imports {
			name: "api_imports",
			shared_libs: [
				"libfoo",
			],
			header_libs: [],
		}
	`

	ctx := prepareForCcTest.RunTestWithBp(t, bp)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
	libfooHeader := ctx.ModuleForTests("libfoo_headers", "android_arm64_armv8-a").Module()
	libfooHeaderApiImport := ctx.ModuleForTests("libfoo_headers.apiimport", "android_arm64_armv8-a").Module()

	android.AssertBoolEquals(t, "original header should be used for original library", true, hasDirectDependency(t, ctx, libfoo, libfooHeader))
	android.AssertBoolEquals(t, "Header from API surface should not be used for original library", false, hasDirectDependency(t, ctx, libfoo, libfooHeaderApiImport))
}

func TestExportDirFromStubLibrary(t *testing.T) {
	bp := `
		cc_library {
			name: "libfoo",
			export_include_dirs: ["source_include_dir"],
			export_system_include_dirs: ["source_system_include_dir"],
			vendor_available: true,
		}
		cc_api_library {
			name: "libfoo",
			export_include_dirs: ["stub_include_dir"],
			export_system_include_dirs: ["stub_system_include_dir"],
			vendor_available: true,
			src: "libfoo.so",
		}
		api_imports {
			name: "api_imports",
			shared_libs: [
				"libfoo",
			],
			header_libs: [],
		}
		// vendor binary
		cc_binary {
			name: "vendorbin",
			vendor: true,
			srcs: ["vendor.cc"],
			shared_libs: ["libfoo"],
		}
	`
	ctx := prepareForCcTest.RunTestWithBp(t, bp)
	vendorCFlags := ctx.ModuleForTests("vendorbin", "android_vendor.29_arm64_armv8-a").Rule("cc").Args["cFlags"]
	android.AssertStringDoesContain(t, "Vendor binary should compile using headers provided by stub", vendorCFlags, "-Istub_include_dir")
	android.AssertStringDoesNotContain(t, "Vendor binary should not compile using headers of source", vendorCFlags, "-Isource_include_dir")
	android.AssertStringDoesContain(t, "Vendor binary should compile using system headers provided by stub", vendorCFlags, "-isystem stub_system_include_dir")
	android.AssertStringDoesNotContain(t, "Vendor binary should not compile using system headers of source", vendorCFlags, "-isystem source_system_include_dir")
}
