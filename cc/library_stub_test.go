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

	"github.com/google/blueprint"
)

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

	vendorImplicits := ctx.ModuleForTests("vendorbin", "android_vendor.29_arm64_armv8-a").Rule("cc").OrderOnly.Strings()
	// Building the stub.so file first assembles its .h files in multi-tree out.
	// These header files are required for compiling the other API domain (vendor in this case)
	android.AssertStringListContains(t, "Vendor binary compilation should have an implicit dep on the stub .so file", vendorImplicits, "libfoo.so")
}

func TestApiLibraryWithLlndkVariant(t *testing.T) {
	bp := `
		cc_binary {
			name: "binfoo",
			vendor: true,
			srcs: ["binfoo.cc"],
			shared_libs: ["libbar"],
		}

		cc_api_library {
			name: "libbar",
			// TODO(b/244244438) Remove src property once all variants are implemented.
			src: "libbar.so",
			vendor_available: true,
			variants: [
				"llndk",
			],
		}

		cc_api_variant {
			name: "libbar",
			variant: "llndk",
			src: "libbar_llndk.so",
			export_headers: ["libbar_llndk_include"]
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

	binfoo := ctx.ModuleForTests("binfoo", "android_vendor.29_arm64_armv8-a").Module()
	libbarApiImport := ctx.ModuleForTests("libbar.apiimport", "android_vendor.29_arm64_armv8-a_shared").Module()
	libbarApiVariant := ctx.ModuleForTests("libbar.llndk.apiimport", "android_vendor.29_arm64_armv8-a").Module()

	android.AssertBoolEquals(t, "Stub library from API surface should be linked", true, hasDirectDependency(t, ctx, binfoo, libbarApiImport))
	android.AssertBoolEquals(t, "Stub library variant from API surface should be linked", true, hasDirectDependency(t, ctx, libbarApiImport, libbarApiVariant))

	binFooLibFlags := ctx.ModuleForTests("binfoo", "android_vendor.29_arm64_armv8-a").Rule("ld").Args["libFlags"]
	android.AssertStringDoesContain(t, "Vendor binary should be linked with LLNDK variant source", binFooLibFlags, "libbar_llndk.so")

	binFooCFlags := ctx.ModuleForTests("binfoo", "android_vendor.29_arm64_armv8-a").Rule("cc").Args["cFlags"]
	android.AssertStringDoesContain(t, "Vendor binary should include headers from the LLNDK variant source", binFooCFlags, "-Ilibbar_llndk_include")
}

func TestApiLibraryWithNdkVariant(t *testing.T) {
	bp := `
		cc_binary {
			name: "binfoo",
			sdk_version: "29",
			srcs: ["binfoo.cc"],
			shared_libs: ["libbar"],
			stl: "c++_shared",
		}

		cc_binary {
			name: "binbaz",
			sdk_version: "30",
			srcs: ["binbaz.cc"],
			shared_libs: ["libbar"],
			stl: "c++_shared",
		}

		cc_api_library {
			name: "libbar",
			// TODO(b/244244438) Remove src property once all variants are implemented.
			src: "libbar.so",
			variants: [
				"ndk.29",
				"ndk.30",
				"ndk.current",
			],
		}

		cc_api_variant {
			name: "libbar",
			variant: "ndk",
			version: "29",
			src: "libbar_ndk_29.so",
			export_headers: ["libbar_ndk_29_include"]
		}

		cc_api_variant {
			name: "libbar",
			variant: "ndk",
			version: "30",
			src: "libbar_ndk_30.so",
			export_headers: ["libbar_ndk_30_include"]
		}

		cc_api_variant {
			name: "libbar",
			variant: "ndk",
			version: "current",
			src: "libbar_ndk_current.so",
			export_headers: ["libbar_ndk_current_include"]
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

	binfoo := ctx.ModuleForTests("binfoo", "android_arm64_armv8-a_sdk").Module()
	libbarApiImportv29 := ctx.ModuleForTests("libbar.apiimport", "android_arm64_armv8-a_sdk_shared_29").Module()
	libbarApiVariantv29 := ctx.ModuleForTests("libbar.ndk.29.apiimport", "android_arm64_armv8-a_sdk").Module()
	libbarApiImportv30 := ctx.ModuleForTests("libbar.apiimport", "android_arm64_armv8-a_sdk_shared_30").Module()
	libbarApiVariantv30 := ctx.ModuleForTests("libbar.ndk.30.apiimport", "android_arm64_armv8-a_sdk").Module()

	android.AssertBoolEquals(t, "Stub library from API surface should be linked with target version", true, hasDirectDependency(t, ctx, binfoo, libbarApiImportv29))
	android.AssertBoolEquals(t, "Stub library variant from API surface should be linked with target version", true, hasDirectDependency(t, ctx, libbarApiImportv29, libbarApiVariantv29))
	android.AssertBoolEquals(t, "Stub library from API surface should not be linked with different version", false, hasDirectDependency(t, ctx, binfoo, libbarApiImportv30))
	android.AssertBoolEquals(t, "Stub library variant from API surface should not be linked with different version", false, hasDirectDependency(t, ctx, libbarApiImportv29, libbarApiVariantv30))

	binbaz := ctx.ModuleForTests("binbaz", "android_arm64_armv8-a_sdk").Module()

	android.AssertBoolEquals(t, "Stub library from API surface should be linked with target version", true, hasDirectDependency(t, ctx, binbaz, libbarApiImportv30))
	android.AssertBoolEquals(t, "Stub library from API surface should not be linked with different version", false, hasDirectDependency(t, ctx, binbaz, libbarApiImportv29))

	binFooLibFlags := ctx.ModuleForTests("binfoo", "android_arm64_armv8-a_sdk").Rule("ld").Args["libFlags"]
	android.AssertStringDoesContain(t, "Binary using sdk should be linked with NDK variant source", binFooLibFlags, "libbar_ndk_29.so")

	binFooCFlags := ctx.ModuleForTests("binfoo", "android_arm64_armv8-a_sdk").Rule("cc").Args["cFlags"]
	android.AssertStringDoesContain(t, "Binary using sdk should include headers from the NDK variant source", binFooCFlags, "-Ilibbar_ndk_29_include")
}

func TestApiLibraryWithMultipleVariants(t *testing.T) {
	bp := `
		cc_binary {
			name: "binfoo",
			sdk_version: "29",
			srcs: ["binfoo.cc"],
			shared_libs: ["libbar"],
			stl: "c++_shared",
		}

		cc_binary {
			name: "binbaz",
			vendor: true,
			srcs: ["binbaz.cc"],
			shared_libs: ["libbar"],
		}

		cc_api_library {
			name: "libbar",
			// TODO(b/244244438) Remove src property once all variants are implemented.
			src: "libbar.so",
			vendor_available: true,
			variants: [
				"llndk",
				"ndk.29",
				"ndk.30",
				"ndk.current",
			],
		}

		cc_api_variant {
			name: "libbar",
			variant: "ndk",
			version: "29",
			src: "libbar_ndk_29.so",
			export_headers: ["libbar_ndk_29_include"]
		}

		cc_api_variant {
			name: "libbar",
			variant: "ndk",
			version: "30",
			src: "libbar_ndk_30.so",
			export_headers: ["libbar_ndk_30_include"]
		}

		cc_api_variant {
			name: "libbar",
			variant: "ndk",
			version: "current",
			src: "libbar_ndk_current.so",
			export_headers: ["libbar_ndk_current_include"]
		}

		cc_api_variant {
			name: "libbar",
			variant: "llndk",
			src: "libbar_llndk.so",
			export_headers: ["libbar_llndk_include"]
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

	binfoo := ctx.ModuleForTests("binfoo", "android_arm64_armv8-a_sdk").Module()
	libbarApiImportv29 := ctx.ModuleForTests("libbar.apiimport", "android_arm64_armv8-a_sdk_shared_29").Module()
	libbarApiImportLlndk := ctx.ModuleForTests("libbar.apiimport", "android_vendor.29_arm64_armv8-a_shared").Module()

	android.AssertBoolEquals(t, "Binary using SDK should be linked with API library from NDK variant", true, hasDirectDependency(t, ctx, binfoo, libbarApiImportv29))
	android.AssertBoolEquals(t, "Binary using SDK should not be linked with API library from LLNDK variant", false, hasDirectDependency(t, ctx, binfoo, libbarApiImportLlndk))

	binbaz := ctx.ModuleForTests("binbaz", "android_vendor.29_arm64_armv8-a").Module()

	android.AssertBoolEquals(t, "Vendor binary should be linked with API library from LLNDK variant", true, hasDirectDependency(t, ctx, binbaz, libbarApiImportLlndk))
	android.AssertBoolEquals(t, "Vendor binary should not be linked with API library from NDK variant", false, hasDirectDependency(t, ctx, binbaz, libbarApiImportv29))

}
