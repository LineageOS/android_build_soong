// Copyright 2021 The Android Open Source Project
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

package rust

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/cc"
)

func TestVendorSnapshotCapture(t *testing.T) {
	bp := `
	rust_ffi {
		name: "libffivendor_available",
		crate_name: "ffivendor_available",
		srcs: ["lib.rs"],
		vendor_available: true,
		include_dirs: ["rust_headers/"],
	}

	rust_ffi {
		name: "libffivendor",
		crate_name: "ffivendor",
		srcs: ["lib.rs"],
		vendor: true,
		include_dirs: ["rust_headers/"],
	}

	rust_library {
		name: "librustvendor_available",
		crate_name: "rustvendor_available",
		srcs: ["lib.rs"],
		vendor_available: true,
	}

	rust_library {
		name: "librustvendor",
		crate_name: "rustvendor",
		srcs: ["lib.rs"],
		vendor: true,
	}

	rust_binary {
		name: "vendor_available_bin",
		vendor_available: true,
		srcs: ["srcs/lib.rs"],
	}

	rust_binary {
		name: "vendor_bin",
		vendor: true,
		srcs: ["srcs/lib.rs"],
	}
    `
	skipTestIfOsNotSupported(t)
	result := android.GroupFixturePreparers(
		prepareForRustTest,
		rustMockedFiles.AddToFixture(),
		android.FixtureModifyProductVariables(
			func(variables android.FixtureProductVariables) {
				variables.DeviceVndkVersion = StringPtr("current")
				variables.Platform_vndk_version = StringPtr("29")
			},
		),
	).RunTestWithBp(t, bp)
	ctx := result.TestContext

	// Check Vendor snapshot output.

	snapshotDir := "vendor-snapshot"
	snapshotVariantPath := filepath.Join("out/soong", snapshotDir, "arm64")
	snapshotSingleton := ctx.SingletonForTests("vendor-snapshot")
	var jsonFiles []string
	for _, arch := range [][]string{
		[]string{"arm64", "armv8-a"},
		[]string{"arm", "armv7-a-neon"},
	} {
		archType := arch[0]
		archVariant := arch[1]
		archDir := fmt.Sprintf("arch-%s-%s", archType, archVariant)

		// For shared libraries, only non-VNDK vendor_available modules are captured
		sharedVariant := fmt.Sprintf("android_vendor.29_%s_%s_shared", archType, archVariant)
		sharedDir := filepath.Join(snapshotVariantPath, archDir, "shared")
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "libffivendor_available", "libffivendor_available.so", sharedDir, sharedVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(sharedDir, "libffivendor_available.so.json"))

		// For static libraries, all vendor:true and vendor_available modules (including VNDK) are captured.
		staticVariant := fmt.Sprintf("android_vendor.29_%s_%s_static", archType, archVariant)
		staticDir := filepath.Join(snapshotVariantPath, archDir, "static")
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "libffivendor_available", "libffivendor_available.a", staticDir, staticVariant)
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "libffivendor", "libffivendor.a", staticDir, staticVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(staticDir, "libffivendor_available.a.json"))
		jsonFiles = append(jsonFiles,
			filepath.Join(staticDir, "libffivendor.a.json"))

		// For rlib libraries, all vendor:true and vendor_available modules (including VNDK) are captured.
		rlibVariant := fmt.Sprintf("android_vendor.29_%s_%s_rlib_dylib-std", archType, archVariant)
		rlibDir := filepath.Join(snapshotVariantPath, archDir, "rlib")
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librustvendor_available", "librustvendor_available.rlib", rlibDir, rlibVariant)
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librustvendor", "librustvendor.rlib", rlibDir, rlibVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(rlibDir, "librustvendor_available.rlib.json"))
		jsonFiles = append(jsonFiles,
			filepath.Join(rlibDir, "librustvendor.rlib.json"))

		// For rlib libraries, all rlib-std variants vendor:true and vendor_available modules (including VNDK) are captured.
		rlibStdVariant := fmt.Sprintf("android_vendor.29_%s_%s_rlib_rlib-std", archType, archVariant)
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librustvendor_available", "librustvendor_available.rlib-std.rlib", rlibDir, rlibStdVariant)
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librustvendor", "librustvendor.rlib-std.rlib", rlibDir, rlibStdVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(rlibDir, "librustvendor_available.rlib.json"))
		jsonFiles = append(jsonFiles,
			filepath.Join(rlibDir, "librustvendor.rlib.json"))

		// For dylib libraries, all vendor:true and vendor_available modules (including VNDK) are captured.
		dylibVariant := fmt.Sprintf("android_vendor.29_%s_%s_dylib", archType, archVariant)
		dylibDir := filepath.Join(snapshotVariantPath, archDir, "dylib")
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librustvendor_available", "librustvendor_available.dylib.so", dylibDir, dylibVariant)
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librustvendor", "librustvendor.dylib.so", dylibDir, dylibVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(dylibDir, "librustvendor_available.dylib.so.json"))
		jsonFiles = append(jsonFiles,
			filepath.Join(dylibDir, "librustvendor.dylib.so.json"))

		// For binary executables, all vendor:true and vendor_available modules are captured.
		if archType == "arm64" {
			binaryVariant := fmt.Sprintf("android_vendor.29_%s_%s", archType, archVariant)
			binaryDir := filepath.Join(snapshotVariantPath, archDir, "binary")
			cc.CheckSnapshot(t, ctx, snapshotSingleton, "vendor_available_bin", "vendor_available_bin", binaryDir, binaryVariant)
			cc.CheckSnapshot(t, ctx, snapshotSingleton, "vendor_bin", "vendor_bin", binaryDir, binaryVariant)
			jsonFiles = append(jsonFiles,
				filepath.Join(binaryDir, "vendor_available_bin.json"))
			jsonFiles = append(jsonFiles,
				filepath.Join(binaryDir, "vendor_bin.json"))
		}
	}

	for _, jsonFile := range jsonFiles {
		// verify all json files exist
		if snapshotSingleton.MaybeOutput(jsonFile).Rule == nil {
			t.Errorf("%q expected but not found; #%v", jsonFile, jsonFiles)
		}
	}

	// fake snapshot should have all outputs in the normal snapshot.
	fakeSnapshotSingleton := ctx.SingletonForTests("vendor-fake-snapshot")

	for _, output := range snapshotSingleton.AllOutputs() {
		fakeOutput := strings.Replace(output, "/vendor-snapshot/", "/fake/vendor-snapshot/", 1)
		if fakeSnapshotSingleton.MaybeOutput(fakeOutput).Rule == nil {
			t.Errorf("%q expected but not found", fakeOutput)
		}
	}
}

func TestVendorSnapshotDirected(t *testing.T) {
	bp := `
	rust_ffi_shared {
		name: "libffivendor_available",
		crate_name: "ffivendor_available",
		srcs: ["lib.rs"],
		vendor_available: true,
	}

	rust_library {
		name: "librustvendor_available",
		crate_name: "rustvendor_available",
		srcs: ["lib.rs"],
		vendor_available: true,
	}

	rust_ffi_shared {
		name: "libffivendor_exclude",
		crate_name: "ffivendor_exclude",
		srcs: ["lib.rs"],
		vendor_available: true,
	}

	rust_library {
		name: "librustvendor_exclude",
		crate_name: "rustvendor_exclude",
		srcs: ["lib.rs"],
		vendor_available: true,
	}
`
	ctx := testRustVndk(t, bp)
	ctx.Config().TestProductVariables.VendorSnapshotModules = make(map[string]bool)
	ctx.Config().TestProductVariables.VendorSnapshotModules["librustvendor_available"] = true
	ctx.Config().TestProductVariables.VendorSnapshotModules["libffivendor_available"] = true
	ctx.Config().TestProductVariables.DirectedVendorSnapshot = true

	// Check Vendor snapshot output.

	snapshotDir := "vendor-snapshot"
	snapshotVariantPath := filepath.Join("out/soong", snapshotDir, "arm64")
	snapshotSingleton := ctx.SingletonForTests("vendor-snapshot")

	var includeJsonFiles []string

	for _, arch := range [][]string{
		[]string{"arm64", "armv8-a"},
		[]string{"arm", "armv7-a-neon"},
	} {
		archType := arch[0]
		archVariant := arch[1]
		archDir := fmt.Sprintf("arch-%s-%s", archType, archVariant)

		sharedVariant := fmt.Sprintf("android_vendor.29_%s_%s_shared", archType, archVariant)
		rlibVariant := fmt.Sprintf("android_vendor.29_%s_%s_rlib_dylib-std", archType, archVariant)
		rlibRlibStdVariant := fmt.Sprintf("android_vendor.29_%s_%s_rlib_rlib-std", archType, archVariant)
		sharedDir := filepath.Join(snapshotVariantPath, archDir, "shared")
		rlibDir := filepath.Join(snapshotVariantPath, archDir, "rlib")
		dylibVariant := fmt.Sprintf("android_vendor.29_%s_%s_dylib", archType, archVariant)
		dylibDir := filepath.Join(snapshotVariantPath, archDir, "dylib")

		// Included modules
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librustvendor_available", "librustvendor_available.rlib", rlibDir, rlibVariant)
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librustvendor_available", "librustvendor_available.rlib-std.rlib", rlibDir, rlibRlibStdVariant)
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librustvendor_available", "librustvendor_available.dylib.so", dylibDir, dylibVariant)
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "libffivendor_available", "libffivendor_available.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(rlibDir, "librustvendor_available.rlib.json"))
		includeJsonFiles = append(includeJsonFiles, filepath.Join(rlibDir, "librustvendor_available.rlib-std.rlib.json"))
		includeJsonFiles = append(includeJsonFiles, filepath.Join(dylibDir, "librustvendor_available.dylib.so.json"))
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "libffivendor_available.so.json"))

		// Excluded modules. Modules not included in the directed vendor snapshot
		// are still include as fake modules.
		cc.CheckSnapshotRule(t, ctx, snapshotSingleton, "librustvendor_exclude", "librustvendor_exclude.rlib", rlibDir, rlibVariant)
		cc.CheckSnapshotRule(t, ctx, snapshotSingleton, "librustvendor_exclude", "librustvendor_exclude.rlib-std.rlib", rlibDir, rlibRlibStdVariant)
		cc.CheckSnapshotRule(t, ctx, snapshotSingleton, "librustvendor_exclude", "librustvendor_exclude.dylib.so", dylibDir, dylibVariant)
		cc.CheckSnapshotRule(t, ctx, snapshotSingleton, "libffivendor_exclude", "libffivendor_exclude.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(rlibDir, "librustvendor_exclude.rlib.json"))
		includeJsonFiles = append(includeJsonFiles, filepath.Join(rlibDir, "librustvendor_exclude.rlib-std.rlib.json"))
		includeJsonFiles = append(includeJsonFiles, filepath.Join(dylibDir, "librustvendor_exclude.dylib.so.json"))
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "libffivendor_exclude.so.json"))
	}

	// Verify that each json file for an included module has a rule.
	for _, jsonFile := range includeJsonFiles {
		if snapshotSingleton.MaybeOutput(jsonFile).Rule == nil {
			t.Errorf("include json file %q not found", jsonFile)
		}
	}
}

func TestVendorSnapshotExclude(t *testing.T) {

	// This test verifies that the exclude_from_vendor_snapshot property
	// makes its way from the Android.bp source file into the module data
	// structure. It also verifies that modules are correctly included or
	// excluded in the vendor snapshot based on their path (framework or
	// vendor) and the exclude_from_vendor_snapshot property.

	frameworkBp := `
		rust_ffi_shared {
			name: "libinclude",
			crate_name: "include",
			srcs: ["include.rs"],
			vendor_available: true,
		}

		rust_ffi_shared {
			name: "libexclude",
			crate_name: "exclude",
			srcs: ["exclude.rs"],
			vendor: true,
			exclude_from_vendor_snapshot: true,
		}

		rust_ffi_shared {
			name: "libavailable_exclude",
			crate_name: "available_exclude",
			srcs: ["lib.rs"],
			vendor_available: true,
			exclude_from_vendor_snapshot: true,
		}

		rust_library {
			name: "librust_include",
			crate_name: "rust_include",
			srcs: ["include.rs"],
			vendor_available: true,
		}

		rust_library {
			name: "librust_exclude",
			crate_name: "rust_exclude",
			srcs: ["exclude.rs"],
			vendor: true,
			exclude_from_vendor_snapshot: true,
		}

		rust_library {
			name: "librust_available_exclude",
			crate_name: "rust_available_exclude",
			srcs: ["lib.rs"],
			vendor_available: true,
			exclude_from_vendor_snapshot: true,
		}
	`

	mockFS := map[string][]byte{
		"framework/Android.bp": []byte(frameworkBp),
		"framework/include.rs": nil,
		"framework/exclude.rs": nil,
	}

	ctx := testRustVndkFs(t, "", mockFS)

	// Test an include and exclude framework module.
	cc.AssertExcludeFromVendorSnapshotIs(t, ctx, "libinclude", false, sharedVendorVariant)
	cc.AssertExcludeFromVendorSnapshotIs(t, ctx, "libexclude", true, sharedVendorVariant)
	cc.AssertExcludeFromVendorSnapshotIs(t, ctx, "libavailable_exclude", true, sharedVendorVariant)

	cc.AssertExcludeFromVendorSnapshotIs(t, ctx, "librust_include", false, rlibVendorVariant)
	cc.AssertExcludeFromVendorSnapshotIs(t, ctx, "librust_exclude", true, rlibVendorVariant)
	cc.AssertExcludeFromVendorSnapshotIs(t, ctx, "librust_available_exclude", true, rlibVendorVariant)

	cc.AssertExcludeFromVendorSnapshotIs(t, ctx, "librust_include", false, rlibDylibStdVendorVariant)
	cc.AssertExcludeFromVendorSnapshotIs(t, ctx, "librust_exclude", true, rlibDylibStdVendorVariant)
	cc.AssertExcludeFromVendorSnapshotIs(t, ctx, "librust_available_exclude", true, rlibDylibStdVendorVariant)

	cc.AssertExcludeFromVendorSnapshotIs(t, ctx, "librust_include", false, dylibVendorVariant)
	cc.AssertExcludeFromVendorSnapshotIs(t, ctx, "librust_exclude", true, dylibVendorVariant)
	cc.AssertExcludeFromVendorSnapshotIs(t, ctx, "librust_available_exclude", true, dylibVendorVariant)

	// Verify the content of the vendor snapshot.

	snapshotDir := "vendor-snapshot"
	snapshotVariantPath := filepath.Join("out/soong", snapshotDir, "arm64")
	snapshotSingleton := ctx.SingletonForTests("vendor-snapshot")

	var includeJsonFiles []string
	var excludeJsonFiles []string

	for _, arch := range [][]string{
		[]string{"arm64", "armv8-a"},
		[]string{"arm", "armv7-a-neon"},
	} {
		archType := arch[0]
		archVariant := arch[1]
		archDir := fmt.Sprintf("arch-%s-%s", archType, archVariant)

		sharedVariant := fmt.Sprintf("android_vendor.29_%s_%s_shared", archType, archVariant)
		sharedDir := filepath.Join(snapshotVariantPath, archDir, "shared")

		rlibVariant := fmt.Sprintf("android_vendor.29_%s_%s_rlib_dylib-std", archType, archVariant)
		rlibRlibStdVariant := fmt.Sprintf("android_vendor.29_%s_%s_rlib_rlib-std", archType, archVariant)
		rlibDir := filepath.Join(snapshotVariantPath, archDir, "rlib")
		dylibVariant := fmt.Sprintf("android_vendor.29_%s_%s_dylib", archType, archVariant)
		dylibDir := filepath.Join(snapshotVariantPath, archDir, "dylib")

		// Included modules
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "libinclude", "libinclude.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "libinclude.so.json"))
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librust_include", "librust_include.rlib", rlibDir, rlibVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(rlibDir, "librust_include.rlib.json"))
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librust_include", "librust_include.rlib-std.rlib", rlibDir, rlibRlibStdVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(rlibDir, "librust_include.rlib-std.rlib.json"))
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librust_include", "librust_include.dylib.so", dylibDir, dylibVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(dylibDir, "librust_include.dylib.so.json"))

		// Excluded modules
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "libexclude", "libexclude.so", sharedDir, sharedVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(sharedDir, "libexclude.so.json"))
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "libavailable_exclude", "libavailable_exclude.so", sharedDir, sharedVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(sharedDir, "libavailable_exclude.so.json"))
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "librust_exclude", "librust_exclude.rlib", rlibDir, rlibVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(rlibDir, "librust_exclude.rlib.json"))
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "librust_available_exclude", "librust_available_exclude.rlib", rlibDir, rlibVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(rlibDir, "librust_available_exclude.rlib.json"))
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "librust_available_exclude", "librust_available_exclude.rlib-std.rlib", rlibDir, rlibRlibStdVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(rlibDir, "librust_available_exclude.rlib.rlib-std.json"))
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "librust_exclude", "librust_exclude.dylib.so", dylibDir, dylibVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(dylibDir, "librust_exclude.dylib.so.json"))
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "librust_available_exclude", "librust_available_exclude.dylib.so", dylibDir, dylibVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(dylibDir, "librust_available_exclude.dylib.so.json"))
	}

	// Verify that each json file for an included module has a rule.
	for _, jsonFile := range includeJsonFiles {
		if snapshotSingleton.MaybeOutput(jsonFile).Rule == nil {
			t.Errorf("include json file %q not found", jsonFile)
		}
	}

	// Verify that each json file for an excluded module has no rule.
	for _, jsonFile := range excludeJsonFiles {
		if snapshotSingleton.MaybeOutput(jsonFile).Rule != nil {
			t.Errorf("exclude json file %q found", jsonFile)
		}
	}
}

func TestVendorSnapshotUse(t *testing.T) {
	frameworkBp := `
	cc_library {
		name: "libvndk",
		vendor_available: true,
		product_available: true,
		vndk: {
			enabled: true,
		},
		nocrt: true,
	}

	cc_library {
		name: "libvendor",
		vendor: true,
		nocrt: true,
		no_libcrt: true,
		stl: "none",
		system_shared_libs: [],
	}

	cc_library {
		name: "libvendor_available",
		vendor_available: true,
		nocrt: true,
		no_libcrt: true,
		stl: "none",
		system_shared_libs: [],
	}

	cc_library {
		name: "lib32",
		vendor: true,
		nocrt: true,
		no_libcrt: true,
		stl: "none",
		system_shared_libs: [],
		compile_multilib: "32",
	}

	cc_library {
		name: "lib64",
		vendor: true,
		nocrt: true,
		no_libcrt: true,
		stl: "none",
		system_shared_libs: [],
		compile_multilib: "64",
	}

	rust_binary {
		name: "bin",
		vendor: true,
		srcs: ["bin.rs"],
	}

	rust_binary {
		name: "bin32",
		vendor: true,
		compile_multilib: "32",
		srcs: ["bin.rs"],
	}

	rust_library {
		name: "librust_vendor_available",
		crate_name: "rust_vendor",
		vendor_available: true,
		srcs: ["client.rs"],
	}

`

	vndkBp := `
	vndk_prebuilt_shared {
		name: "libvndk",
		version: "30",
		target_arch: "arm64",
		vendor_available: true,
		product_available: true,
		vndk: {
			enabled: true,
		},
		arch: {
			arm64: {
				srcs: ["libvndk.so"],
				export_include_dirs: ["include/libvndk"],
			},
			arm: {
				srcs: ["libvndk.so"],
				export_include_dirs: ["include/libvndk"],
			},
		},
	}

	// old snapshot module which has to be ignored
	vndk_prebuilt_shared {
		name: "libvndk",
		version: "26",
		target_arch: "arm64",
		vendor_available: true,
		product_available: true,
		vndk: {
			enabled: true,
		},
		arch: {
			arm64: {
				srcs: ["libvndk.so"],
				export_include_dirs: ["include/libvndk"],
			},
			arm: {
				srcs: ["libvndk.so"],
				export_include_dirs: ["include/libvndk"],
			},
		},
	}

	// different arch snapshot which has to be ignored
	vndk_prebuilt_shared {
		name: "libvndk",
		version: "30",
		target_arch: "arm",
		vendor_available: true,
		product_available: true,
		vndk: {
			enabled: true,
		},
		arch: {
			arm: {
				srcs: ["libvndk.so"],
				export_include_dirs: ["include/libvndk"],
			},
		},
	}
`

	vendorProprietaryBp := `
	cc_library {
		name: "libvendor_without_snapshot",
		vendor: true,
		nocrt: true,
		no_libcrt: true,
		no_crt_pad_segment: true,
		stl: "none",
		system_shared_libs: [],
	}

	rust_ffi_shared {
		name: "libclient",
		crate_name: "client",
		vendor: true,
		shared_libs: ["libvndk", "libvendor_available"],
		static_libs: ["libvendor", "libvendor_without_snapshot"],
		rustlibs: ["librust_vendor_available"],
		arch: {
			arm64: {
				shared_libs: ["lib64"],
			},
			arm: {
				shared_libs: ["lib32"],
			},
		},
		srcs: ["client.rs"],
	}

	rust_library {
		name: "libclient_rust",
		crate_name: "client_rust",
		vendor: true,
		shared_libs: ["libvndk", "libvendor_available"],
		static_libs: ["libvendor", "libvendor_without_snapshot"],
		rustlibs: ["librust_vendor_available"],
		arch: {
			arm64: {
				shared_libs: ["lib64"],
			},
			arm: {
				shared_libs: ["lib32"],
			},
		},
		srcs: ["client.rs"],
	}

	rust_binary {
		name: "bin_without_snapshot",
		vendor: true,
		static_libs: ["libvndk"],
		srcs: ["bin.rs"],
		rustlibs: ["librust_vendor_available"],
	}

	vendor_snapshot {
		name: "vendor_snapshot",
		version: "30",
		arch: {
			arm64: {
				vndk_libs: [
					"libvndk",
				],
				static_libs: [
					"libvendor",
					"libvndk",
					"libclang_rt.builtins",
					"note_memtag_heap_sync",
				],
				shared_libs: [
					"libvendor_available",
					"lib64",
				],
				rlibs: [
					"libstd",
					"librust_vendor_available",
					"librust_vendor_available.rlib-std"
				],
				dylibs: [
					"libstd",
					"librust_vendor_available",
				],
				binaries: [
					"bin",
				],
                objects: [
				    "crtend_so",
					"crtbegin_so",
					"crtbegin_dynamic",
					"crtend_android"
				],
			},
			arm: {
				vndk_libs: [
					"libvndk",
				],
				static_libs: [
					"libvendor",
					"libvndk",
					"libclang_rt.builtins",
				],
				shared_libs: [
					"libvendor_available",
					"lib32",
				],
				rlibs: [
					"libstd",
					"librust_vendor_available",
				],
				dylibs: [
					"libstd",
					"librust_vendor_available",
				],
				binaries: [
					"bin32",
				],
                objects: [
				    "crtend_so",
					"crtbegin_so",
					"crtbegin_dynamic",
					"crtend_android"
				],

			},
		}
	}

	vendor_snapshot_object {
		name: "crtend_so",
		version: "30",
		target_arch: "arm64",
		vendor: true,
		stl: "none",
		crt: true,
		arch: {
			arm64: {
				src: "crtend_so.o",
			},
			arm: {
				src: "crtend_so.o",
			},
		},
	}

	vendor_snapshot_object {
		name: "crtbegin_so",
		version: "30",
		target_arch: "arm64",
		vendor: true,
		stl: "none",
		crt: true,
		arch: {
			arm64: {
				src: "crtbegin_so.o",
			},
			arm: {
				src: "crtbegin_so.o",
			},
		},
	}

	vendor_snapshot_rlib {
		name: "libstd",
		version: "30",
		target_arch: "arm64",
		vendor: true,
		sysroot: true,
		arch: {
			arm64: {
				src: "libstd.rlib",
			},
			arm: {
				src: "libstd.rlib",
			},
		},
	}

	vendor_snapshot_rlib {
		name: "librust_vendor_available",
		version: "30",
		target_arch: "arm64",
		vendor: true,
		arch: {
			arm64: {
				src: "librust_vendor_available.rlib",
			},
			arm: {
				src: "librust_vendor_available.rlib",
			},
		},
	}

	vendor_snapshot_rlib {
		name: "librust_vendor_available.rlib-std",
		version: "30",
		target_arch: "arm64",
		vendor: true,
		arch: {
			arm64: {
				src: "librust_vendor_available.rlib-std.rlib",
			},
			arm: {
				src: "librust_vendor_available.rlib-std.rlib",
			},
		},
	}

	vendor_snapshot_dylib {
		name: "libstd",
		version: "30",
		target_arch: "arm64",
		vendor: true,
		sysroot: true,
		arch: {
			arm64: {
				src: "libstd.dylib.so",
			},
			arm: {
				src: "libstd.dylib.so",
			},
		},
	}

	vendor_snapshot_dylib {
		name: "librust_vendor_available",
		version: "30",
		target_arch: "arm64",
		vendor: true,
		arch: {
			arm64: {
				src: "librust_vendor_available.dylib.so",
			},
			arm: {
				src: "librust_vendor_available.dylib.so",
			},
		},
	}

	vendor_snapshot_object {
		name: "crtend_android",
		version: "30",
		target_arch: "arm64",
		vendor: true,
		stl: "none",
		crt: true,
		arch: {
			arm64: {
				src: "crtend_so.o",
			},
			arm: {
				src: "crtend_so.o",
			},
		},
	}

	vendor_snapshot_object {
		name: "crtbegin_dynamic",
		version: "30",
		target_arch: "arm64",
		vendor: true,
		stl: "none",
		crt: true,
		arch: {
			arm64: {
				src: "crtbegin_so.o",
			},
			arm: {
				src: "crtbegin_so.o",
			},
		},
	}

	vendor_snapshot_static {
		name: "libvndk",
		version: "30",
		target_arch: "arm64",
		compile_multilib: "both",
		vendor: true,
		arch: {
			arm64: {
				src: "libvndk.a",
			},
			arm: {
				src: "libvndk.a",
			},
		},
		shared_libs: ["libvndk"],
		export_shared_lib_headers: ["libvndk"],
	}

	vendor_snapshot_static {
		name: "libclang_rt.builtins",
		version: "30",
		target_arch: "arm64",
		vendor: true,
		arch: {
			arm: {
				src: "libclang_rt.builtins-arm-android.a",
			},
			arm64: {
				src: "libclang_rt.builtins-aarch64-android.a",
			},
		},
    }

	vendor_snapshot_shared {
		name: "lib32",
		version: "30",
		target_arch: "arm64",
		compile_multilib: "32",
		vendor: true,
		no_crt_pad_segment: true,
		arch: {
			arm: {
				src: "lib32.so",
			},
		},
	}

	vendor_snapshot_shared {
		name: "lib64",
		version: "30",
		target_arch: "arm64",
		compile_multilib: "64",
		vendor: true,
		no_crt_pad_segment: true,
		arch: {
			arm64: {
				src: "lib64.so",
			},
		},
	}
	vendor_snapshot_shared {
		name: "liblog",
		version: "30",
		target_arch: "arm64",
		compile_multilib: "64",
		vendor: true,
		no_crt_pad_segment: true,
		arch: {
			arm64: {
				src: "liblog.so",
			},
		},
	}

	vendor_snapshot_static {
		name: "libvendor",
		version: "30",
		target_arch: "arm64",
		compile_multilib: "both",
		vendor: true,
		arch: {
			arm64: {
				src: "libvendor.a",
				export_include_dirs: ["include/libvendor"],
			},
			arm: {
				src: "libvendor.a",
				export_include_dirs: ["include/libvendor"],
			},
		},
	}

	vendor_snapshot_shared {
		name: "libvendor_available",
		version: "30",
		target_arch: "arm64",
		compile_multilib: "both",
		vendor: true,
		no_crt_pad_segment: true,
		arch: {
			arm64: {
				src: "libvendor_available.so",
				export_include_dirs: ["include/libvendor"],
			},
			arm: {
				src: "libvendor_available.so",
				export_include_dirs: ["include/libvendor"],
			},
		},
	}

	vendor_snapshot_binary {
		name: "bin",
		version: "30",
		target_arch: "arm64",
		compile_multilib: "64",
		vendor: true,
		arch: {
			arm64: {
				src: "bin",
			},
		},
	}

	vendor_snapshot_binary {
		name: "bin32",
		version: "30",
		target_arch: "arm64",
		compile_multilib: "32",
		vendor: true,
		arch: {
			arm: {
				src: "bin32",
			},
		},
	}

	// Test sanitizers use the snapshot libraries
	rust_binary {
		name: "memtag_binary",
		srcs: ["vendor/bin.rs"],
		vendor: true,
		compile_multilib: "64",
		sanitize: {
			memtag_heap: true,
			diag: {
				memtag_heap: true,
			}
		},
	}

	// old snapshot module which has to be ignored
	vendor_snapshot_binary {
		name: "bin",
		version: "26",
		target_arch: "arm64",
		compile_multilib: "first",
		vendor: true,
		arch: {
			arm64: {
				src: "bin",
			},
		},
	}

	// different arch snapshot which has to be ignored
	vendor_snapshot_binary {
		name: "bin",
		version: "30",
		target_arch: "arm",
		compile_multilib: "first",
		vendor: true,
		arch: {
			arm64: {
				src: "bin",
			},
		},
	}

	vendor_snapshot_static {
		name: "note_memtag_heap_sync",
		vendor: true,
		target_arch: "arm64",
		version: "30",
		arch: {
			arm64: {
				src: "note_memtag_heap_sync.a",
			},
		},
	}

`

	mockFS := android.MockFS{
		"framework/Android.bp":                          []byte(frameworkBp),
		"framework/bin.rs":                              nil,
		"note_memtag_heap_sync.a":                       nil,
		"vendor/Android.bp":                             []byte(vendorProprietaryBp),
		"vendor/bin":                                    nil,
		"vendor/bin32":                                  nil,
		"vendor/bin.rs":                                 nil,
		"vendor/client.rs":                              nil,
		"vendor/include/libvndk/a.h":                    nil,
		"vendor/include/libvendor/b.h":                  nil,
		"vendor/libvndk.a":                              nil,
		"vendor/libvendor.a":                            nil,
		"vendor/libvendor.so":                           nil,
		"vendor/lib32.so":                               nil,
		"vendor/lib64.so":                               nil,
		"vendor/liblog.so":                              nil,
		"vendor/libstd.rlib":                            nil,
		"vendor/librust_vendor_available.rlib":          nil,
		"vendor/librust_vendor_available.rlib-std.rlib": nil,
		"vendor/libstd.dylib.so":                        nil,
		"vendor/librust_vendor_available.dylib.so":      nil,
		"vendor/crtbegin_so.o":                          nil,
		"vendor/crtend_so.o":                            nil,
		"vendor/libclang_rt.builtins-aarch64-android.a": nil,
		"vendor/libclang_rt.builtins-arm-android.a":     nil,
		"vndk/Android.bp":                               []byte(vndkBp),
		"vndk/include/libvndk/a.h":                      nil,
		"vndk/libvndk.so":                               nil,
	}

	sharedVariant := "android_vendor.30_arm64_armv8-a_shared"
	rlibVariant := "android_vendor.30_arm64_armv8-a_rlib_dylib-std"
	rlibRlibStdVariant := "android_vendor.30_arm64_armv8-a_rlib_rlib-std"
	dylibVariant := "android_vendor.30_arm64_armv8-a_dylib"
	staticVariant := "android_vendor.30_arm64_armv8-a_static"
	binaryVariant := "android_vendor.30_arm64_armv8-a"

	shared32Variant := "android_vendor.30_arm_armv7-a-neon_shared"
	binary32Variant := "android_vendor.30_arm_armv7-a-neon"

	ctx := testRustVndkFsVersions(t, "", mockFS, "30", "current", "31")

	// libclient uses libvndk.vndk.30.arm64, libvendor.vendor_static.30.arm64, libvendor_without_snapshot
	libclientLdFlags := ctx.ModuleForTests("libclient", sharedVariant).Rule("rustc").Args["linkFlags"]
	for _, input := range [][]string{
		[]string{sharedVariant, "libvndk.vndk.30.arm64"},
		[]string{staticVariant, "libvendor.vendor_static.30.arm64"},
		[]string{staticVariant, "libvendor_without_snapshot"},
	} {
		outputPaths := cc.GetOutputPaths(ctx, input[0] /* variant */, []string{input[1]} /* module name */)
		if !strings.Contains(libclientLdFlags, outputPaths[0].String()) {
			t.Errorf("libflags for libclient must contain %#v, but was %#v", outputPaths[0], libclientLdFlags)
		}
	}

	libclientAndroidMkSharedLibs := ctx.ModuleForTests("libclient", sharedVariant).Module().(*Module).transitiveAndroidMkSharedLibs.ToList()
	if g, w := libclientAndroidMkSharedLibs, []string{"libvndk.vendor", "libvendor_available.vendor", "lib64", "liblog.vendor", "libc.vendor", "libm.vendor", "libdl.vendor"}; !reflect.DeepEqual(g, w) {
		t.Errorf("wanted libclient AndroidMkSharedLibs %q, got %q", w, g)
	}

	libclientAndroidMkStaticLibs := ctx.ModuleForTests("libclient", sharedVariant).Module().(*Module).Properties.AndroidMkStaticLibs
	if g, w := libclientAndroidMkStaticLibs, []string{"libvendor", "libvendor_without_snapshot", "libclang_rt.builtins.vendor"}; !reflect.DeepEqual(g, w) {
		t.Errorf("wanted libclient AndroidMkStaticLibs %q, got %q", w, g)
	}

	libclientAndroidMkDylibs := ctx.ModuleForTests("libclient", sharedVariant).Module().(*Module).Properties.AndroidMkDylibs
	if g, w := libclientAndroidMkDylibs, []string{"librust_vendor_available.vendor", "libstd.vendor"}; !reflect.DeepEqual(g, w) {
		t.Errorf("wanted libclient libclientAndroidMkDylibs %q, got %q", w, libclientAndroidMkDylibs)
	}

	libclient32AndroidMkSharedLibs := ctx.ModuleForTests("libclient", shared32Variant).Module().(*Module).transitiveAndroidMkSharedLibs.ToList()
	if g, w := libclient32AndroidMkSharedLibs, []string{"libvndk.vendor", "libvendor_available.vendor", "lib32", "liblog.vendor", "libc.vendor", "libm.vendor", "libdl.vendor"}; !reflect.DeepEqual(g, w) {
		t.Errorf("wanted libclient32 AndroidMkSharedLibs %q, got %q", w, g)
	}

	libclientRustAndroidMkRlibs := ctx.ModuleForTests("libclient_rust", rlibVariant).Module().(*Module).Properties.AndroidMkRlibs
	if g, w := libclientRustAndroidMkRlibs, []string{"librust_vendor_available.vendor"}; !reflect.DeepEqual(g, w) {
		t.Errorf("wanted rlib libclient libclientAndroidMkRlibs %q, got %q", w, g)
	}

	libclientRlibStdRustAndroidMkRlibs := ctx.ModuleForTests("libclient_rust", rlibRlibStdVariant).Module().(*Module).Properties.AndroidMkRlibs
	if g, w := libclientRlibStdRustAndroidMkRlibs, []string{"librust_vendor_available.vendor.rlib-std", "libstd.vendor"}; !reflect.DeepEqual(g, w) {
		t.Errorf("wanted rlib libclient libclientAndroidMkRlibs %q, got %q", w, g)
	}

	libclientRustDylibAndroidMkDylibs := ctx.ModuleForTests("libclient_rust", dylibVariant).Module().(*Module).Properties.AndroidMkDylibs
	if g, w := libclientRustDylibAndroidMkDylibs, []string{"librust_vendor_available.vendor", "libstd.vendor"}; !reflect.DeepEqual(g, w) {
		t.Errorf("wanted dylib libclient libclientRustDylibAndroidMkDylibs %q, got %q", w, g)
	}

	// rust vendor snapshot must have ".vendor" suffix in AndroidMk
	librustVendorAvailableSnapshotModule := ctx.ModuleForTests("librust_vendor_available.vendor_rlib.30.arm64", rlibVariant).Module()
	librustVendorSnapshotMkName := android.AndroidMkEntriesForTest(t, ctx, librustVendorAvailableSnapshotModule)[0].EntryMap["LOCAL_MODULE"][0]
	expectedRustVendorSnapshotName := "librust_vendor_available.vendor"
	if librustVendorSnapshotMkName != expectedRustVendorSnapshotName {
		t.Errorf("Unexpected rust vendor snapshot name in AndroidMk: %q, expected: %q\n", librustVendorSnapshotMkName, expectedRustVendorSnapshotName)
	}

	librustVendorAvailableDylibSnapshotModule := ctx.ModuleForTests("librust_vendor_available.vendor_dylib.30.arm64", dylibVariant).Module()
	librustVendorSnapshotDylibMkName := android.AndroidMkEntriesForTest(t, ctx, librustVendorAvailableDylibSnapshotModule)[0].EntryMap["LOCAL_MODULE"][0]
	expectedRustVendorDylibSnapshotName := "librust_vendor_available.vendor"
	if librustVendorSnapshotDylibMkName != expectedRustVendorDylibSnapshotName {
		t.Errorf("Unexpected rust vendor snapshot name in AndroidMk: %q, expected: %q\n", librustVendorSnapshotDylibMkName, expectedRustVendorDylibSnapshotName)
	}

	rustVendorBinModule := ctx.ModuleForTests("bin_without_snapshot", binaryVariant).Module()
	rustVendorBinMkDylibName := android.AndroidMkEntriesForTest(t, ctx, rustVendorBinModule)[0].EntryMap["LOCAL_DYLIB_LIBRARIES"][0]
	if rustVendorBinMkDylibName != expectedRustVendorSnapshotName {
		t.Errorf("Unexpected rust rlib name in AndroidMk: %q, expected: %q\n", rustVendorBinMkDylibName, expectedRustVendorSnapshotName)
	}

	binWithoutSnapshotLdFlags := ctx.ModuleForTests("bin_without_snapshot", binaryVariant).Rule("rustc").Args["linkFlags"]
	libVndkStaticOutputPaths := cc.GetOutputPaths(ctx, staticVariant, []string{"libvndk.vendor_static.30.arm64"})
	if !strings.Contains(binWithoutSnapshotLdFlags, libVndkStaticOutputPaths[0].String()) {
		t.Errorf("libflags for bin_without_snapshot must contain %#v, but was %#v",
			libVndkStaticOutputPaths[0], binWithoutSnapshotLdFlags)
	}

	// bin is installed by bin.vendor_binary.30.arm64
	ctx.ModuleForTests("bin.vendor_binary.30.arm64", binaryVariant).Output("bin")

	// bin32 is installed by bin32.vendor_binary.30.arm64
	ctx.ModuleForTests("bin32.vendor_binary.30.arm64", binary32Variant).Output("bin32")

	// bin_without_snapshot is installed by bin_without_snapshot
	ctx.ModuleForTests("bin_without_snapshot", binaryVariant).Output("bin_without_snapshot")

	// libvendor, libvendor_available and bin don't have vendor.30 variant
	libvendorVariants := ctx.ModuleVariantsForTests("libvendor")
	if android.InList(sharedVariant, libvendorVariants) {
		t.Errorf("libvendor must not have variant %#v, but it does", sharedVariant)
	}

	libvendorAvailableVariants := ctx.ModuleVariantsForTests("libvendor_available")
	if android.InList(sharedVariant, libvendorAvailableVariants) {
		t.Errorf("libvendor_available must not have variant %#v, but it does", sharedVariant)
	}

	binVariants := ctx.ModuleVariantsForTests("bin")
	if android.InList(binaryVariant, binVariants) {
		t.Errorf("bin must not have variant %#v, but it does", sharedVariant)
	}

	memtagStaticLibs := ctx.ModuleForTests("memtag_binary", "android_vendor.30_arm64_armv8-a").Module().(*Module).Properties.AndroidMkStaticLibs
	if g, w := memtagStaticLibs, []string{"libclang_rt.builtins.vendor", "note_memtag_heap_sync.vendor"}; !reflect.DeepEqual(g, w) {
		t.Errorf("wanted memtag_binary AndroidMkStaticLibs %q, got %q", w, g)
	}
}

func TestRecoverySnapshotCapture(t *testing.T) {
	bp := `
	rust_ffi {
		name: "librecovery",
		recovery: true,
		srcs: ["foo.rs"],
		crate_name: "recovery",
	}

	rust_ffi {
		name: "librecovery_available",
		recovery_available: true,
		srcs: ["foo.rs"],
		crate_name: "recovery_available",
	}

	rust_library {
		name: "librecovery_rustlib",
		recovery: true,
		srcs: ["foo.rs"],
		crate_name: "recovery_rustlib",
	}

	rust_library {
		name: "librecovery_available_rustlib",
		recovery_available: true,
		srcs: ["foo.rs"],
		crate_name: "recovery_available_rustlib",
	}

	rust_binary {
		name: "recovery_bin",
		recovery: true,
		srcs: ["foo.rs"],
	}

	rust_binary {
		name: "recovery_available_bin",
		recovery_available: true,
		srcs: ["foo.rs"],
	}

`
	// Check Recovery snapshot output.

	ctx := testRustRecoveryFsVersions(t, bp, rustMockedFiles, "", "29", "current")
	snapshotDir := "recovery-snapshot"
	snapshotVariantPath := filepath.Join("out/soong", snapshotDir, "arm64")
	snapshotSingleton := ctx.SingletonForTests("recovery-snapshot")

	var jsonFiles []string

	for _, arch := range [][]string{
		[]string{"arm64", "armv8-a"},
	} {
		archType := arch[0]
		archVariant := arch[1]
		archDir := fmt.Sprintf("arch-%s-%s", archType, archVariant)

		// For shared libraries, all recovery:true and recovery_available modules are captured.
		sharedVariant := fmt.Sprintf("android_recovery_%s_%s_shared", archType, archVariant)
		sharedDir := filepath.Join(snapshotVariantPath, archDir, "shared")
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librecovery", "librecovery.so", sharedDir, sharedVariant)
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librecovery_available", "librecovery_available.so", sharedDir, sharedVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(sharedDir, "librecovery.so.json"),
			filepath.Join(sharedDir, "librecovery_available.so.json"))

		// For static libraries, all recovery:true and recovery_available modules are captured.
		staticVariant := fmt.Sprintf("android_recovery_%s_%s_static", archType, archVariant)
		staticDir := filepath.Join(snapshotVariantPath, archDir, "static")
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librecovery", "librecovery.a", staticDir, staticVariant)
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librecovery_available", "librecovery_available.a", staticDir, staticVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(staticDir, "librecovery.a.json"),
			filepath.Join(staticDir, "librecovery_available.a.json"))

		// For rlib libraries, all recovery:true and recovery_available modules are captured.
		rlibVariant := fmt.Sprintf("android_recovery_%s_%s_rlib_dylib-std", archType, archVariant)
		rlibDir := filepath.Join(snapshotVariantPath, archDir, "rlib")
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librecovery_rustlib", "librecovery_rustlib.rlib", rlibDir, rlibVariant)
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librecovery_available_rustlib", "librecovery_available_rustlib.rlib", rlibDir, rlibVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(rlibDir, "librecovery_rustlib.rlib.json"),
			filepath.Join(rlibDir, "librecovery_available_rustlib.rlib.json"))

		rlibRlibStdVariant := fmt.Sprintf("android_recovery_%s_%s_rlib_rlib-std", archType, archVariant)
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librecovery_rustlib", "librecovery_rustlib.rlib-std.rlib", rlibDir, rlibRlibStdVariant)
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librecovery_available_rustlib", "librecovery_available_rustlib.rlib-std.rlib", rlibDir, rlibRlibStdVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(rlibDir, "librecovery_rustlib.rlib-std.rlib.json"),
			filepath.Join(rlibDir, "librecovery_available_rustlib.rlib-std.rlib.json"))

		// For dylib libraries, all recovery:true and recovery_available modules are captured.
		dylibVariant := fmt.Sprintf("android_recovery_%s_%s_dylib", archType, archVariant)
		dylibDir := filepath.Join(snapshotVariantPath, archDir, "dylib")
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librecovery_rustlib", "librecovery_rustlib.dylib.so", dylibDir, dylibVariant)
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librecovery_available_rustlib", "librecovery_available_rustlib.dylib.so", dylibDir, dylibVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(dylibDir, "librecovery_rustlib.dylib.so.json"),
			filepath.Join(dylibDir, "librecovery_available_rustlib.dylib.so.json"))

		// For binary executables, all recovery:true and recovery_available modules are captured.
		if archType == "arm64" {
			binaryVariant := fmt.Sprintf("android_recovery_%s_%s", archType, archVariant)
			binaryDir := filepath.Join(snapshotVariantPath, archDir, "binary")
			cc.CheckSnapshot(t, ctx, snapshotSingleton, "recovery_bin", "recovery_bin", binaryDir, binaryVariant)
			cc.CheckSnapshot(t, ctx, snapshotSingleton, "recovery_available_bin", "recovery_available_bin", binaryDir, binaryVariant)
			jsonFiles = append(jsonFiles,
				filepath.Join(binaryDir, "recovery_bin.json"),
				filepath.Join(binaryDir, "recovery_available_bin.json"))
		}
	}

	for _, jsonFile := range jsonFiles {
		// verify all json files exist
		if snapshotSingleton.MaybeOutput(jsonFile).Rule == nil {
			t.Errorf("%q expected but not found", jsonFile)
		}
	}
}

func TestRecoverySnapshotExclude(t *testing.T) {
	// This test verifies that the exclude_from_recovery_snapshot property
	// makes its way from the Android.bp source file into the module data
	// structure. It also verifies that modules are correctly included or
	// excluded in the recovery snapshot based on their path (framework or
	// vendor) and the exclude_from_recovery_snapshot property.

	frameworkBp := `
		rust_ffi_shared {
			name: "libinclude",
			srcs: ["src/include.rs"],
			recovery_available: true,
			crate_name: "include",
		}
		rust_ffi_shared {
			name: "libexclude",
			srcs: ["src/exclude.rs"],
			recovery: true,
			exclude_from_recovery_snapshot: true,
			crate_name: "exclude",
		}
		rust_ffi_shared {
			name: "libavailable_exclude",
			srcs: ["src/exclude.rs"],
			recovery_available: true,
			exclude_from_recovery_snapshot: true,
			crate_name: "available_exclude",
		}
		rust_library {
			name: "libinclude_rustlib",
			srcs: ["src/include.rs"],
			recovery_available: true,
			crate_name: "include_rustlib",
		}
		rust_library {
			name: "libexclude_rustlib",
			srcs: ["src/exclude.rs"],
			recovery: true,
			exclude_from_recovery_snapshot: true,
			crate_name: "exclude_rustlib",
		}
		rust_library {
			name: "libavailable_exclude_rustlib",
			srcs: ["src/exclude.rs"],
			recovery_available: true,
			exclude_from_recovery_snapshot: true,
			crate_name: "available_exclude_rustlib",
		}
	`

	vendorProprietaryBp := `
		rust_ffi_shared {
			name: "librecovery",
			srcs: ["recovery.rs"],
			recovery: true,
			crate_name: "recovery",
		}
		rust_library {
			name: "librecovery_rustlib",
			srcs: ["recovery.rs"],
			recovery: true,
			crate_name: "recovery_rustlib",
		}
	`

	mockFS := map[string][]byte{
		"framework/Android.bp": []byte(frameworkBp),
		"framework/include.rs": nil,
		"framework/exclude.rs": nil,
		"device/Android.bp":    []byte(vendorProprietaryBp),
		"device/recovery.rs":   nil,
	}

	ctx := testRustRecoveryFsVersions(t, "", mockFS, "", "29", "current")

	// Test an include and exclude framework module.
	cc.AssertExcludeFromRecoverySnapshotIs(t, ctx, "libinclude", false, sharedRecoveryVariant)
	cc.AssertExcludeFromRecoverySnapshotIs(t, ctx, "libexclude", true, sharedRecoveryVariant)
	cc.AssertExcludeFromRecoverySnapshotIs(t, ctx, "libavailable_exclude", true, sharedRecoveryVariant)

	cc.AssertExcludeFromRecoverySnapshotIs(t, ctx, "libinclude_rustlib", false, rlibRecoveryVariant)
	cc.AssertExcludeFromRecoverySnapshotIs(t, ctx, "libexclude_rustlib", true, rlibRecoveryVariant)
	cc.AssertExcludeFromRecoverySnapshotIs(t, ctx, "libavailable_exclude_rustlib", true, rlibRlibStdRecoveryVariant)

	cc.AssertExcludeFromRecoverySnapshotIs(t, ctx, "libinclude_rustlib", false, rlibRlibStdRecoveryVariant)
	cc.AssertExcludeFromRecoverySnapshotIs(t, ctx, "libexclude_rustlib", true, rlibRlibStdRecoveryVariant)
	cc.AssertExcludeFromRecoverySnapshotIs(t, ctx, "libavailable_exclude_rustlib", true, rlibRlibStdRecoveryVariant)

	cc.AssertExcludeFromRecoverySnapshotIs(t, ctx, "libinclude_rustlib", false, dylibRecoveryVariant)
	cc.AssertExcludeFromRecoverySnapshotIs(t, ctx, "libexclude_rustlib", true, dylibRecoveryVariant)
	cc.AssertExcludeFromRecoverySnapshotIs(t, ctx, "libavailable_exclude_rustlib", true, dylibRecoveryVariant)

	// A recovery module is excluded, but by its path not the exclude_from_recovery_snapshot property
	// ('device/' and 'vendor/' are default excluded). See snapshot/recovery_snapshot.go for more detail.
	cc.AssertExcludeFromRecoverySnapshotIs(t, ctx, "librecovery", false, sharedRecoveryVariant)
	cc.AssertExcludeFromRecoverySnapshotIs(t, ctx, "librecovery_rustlib", false, rlibRecoveryVariant)
	cc.AssertExcludeFromRecoverySnapshotIs(t, ctx, "librecovery_rustlib", false, rlibRlibStdRecoveryVariant)
	cc.AssertExcludeFromRecoverySnapshotIs(t, ctx, "librecovery_rustlib", false, dylibRecoveryVariant)

	// Verify the content of the recovery snapshot.

	snapshotDir := "recovery-snapshot"
	snapshotVariantPath := filepath.Join("out/soong", snapshotDir, "arm64")
	snapshotSingleton := ctx.SingletonForTests("recovery-snapshot")

	var includeJsonFiles []string
	var excludeJsonFiles []string

	for _, arch := range [][]string{
		[]string{"arm64", "armv8-a"},
	} {
		archType := arch[0]
		archVariant := arch[1]
		archDir := fmt.Sprintf("arch-%s-%s", archType, archVariant)

		sharedVariant := fmt.Sprintf("android_recovery_%s_%s_shared", archType, archVariant)
		rlibVariant := fmt.Sprintf("android_recovery_%s_%s_rlib_dylib-std", archType, archVariant)
		rlibRlibStdVariant := fmt.Sprintf("android_recovery_%s_%s_rlib_rlib-std", archType, archVariant)
		dylibVariant := fmt.Sprintf("android_recovery_%s_%s_dylib", archType, archVariant)
		sharedDir := filepath.Join(snapshotVariantPath, archDir, "shared")
		rlibDir := filepath.Join(snapshotVariantPath, archDir, "rlib")
		dylibDir := filepath.Join(snapshotVariantPath, archDir, "dylib")

		// Included modules

		cc.CheckSnapshot(t, ctx, snapshotSingleton, "libinclude", "libinclude.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "libinclude.so.json"))
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "libinclude_rustlib", "libinclude_rustlib.rlib", rlibDir, rlibVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(rlibDir, "libinclude_rustlib.rlib.json"))
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "libinclude_rustlib", "libinclude_rustlib.rlib-std.rlib", rlibDir, rlibRlibStdVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(rlibDir, "libinclude_rustlib.rlib-std.rlib.json"))

		// Excluded modules
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "libexclude", "libexclude.so", sharedDir, sharedVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(sharedDir, "libexclude.so.json"))
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "librecovery", "librecovery.so", sharedDir, sharedVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(sharedDir, "librecovery.so.json"))
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "libavailable_exclude", "libavailable_exclude.so", sharedDir, sharedVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(sharedDir, "libavailable_exclude.so.json"))

		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "libexclude_rustlib", "libexclude_rustlib.rlib", rlibDir, rlibVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(rlibDir, "libexclude_rustlib.rlib.json"))
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "librecovery_rustlib", "librecovery_rustlib.rlib", rlibDir, rlibVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(rlibDir, "librecovery_rustlib.rlib.json"))
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "libavailable_exclude_rustlib", "libavailable_exclude_rustlib.rlib", rlibDir, rlibVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(rlibDir, "libavailable_exclude_rustlib.rlib.json"))

		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "libexclude_rustlib", "libexclude_rustlib.rlib-std.rlib", rlibDir, rlibRlibStdVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(rlibDir, "libexclude_rustlib.rlib-std.rlib.json"))
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "librecovery_rustlib", "librecovery_rustlib.rlib-std.rlib", rlibDir, rlibRlibStdVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(rlibDir, "librecovery_rustlib.rlib-std.rlib.json"))
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "libavailable_exclude_rustlib", "libavailable_exclude_rustlib.rlib-std.rlib", rlibDir, rlibRlibStdVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(rlibDir, "libavailable_exclude_rustlib.rlib-std.rlib.json"))

		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "libexclude_rustlib", "libexclude_rustlib.dylib.so", dylibDir, dylibVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(rlibDir, "libexclude_rustlib.dylib.so.json"))
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "librecovery_rustlib", "librecovery_rustlib.dylib.so", dylibDir, dylibVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(rlibDir, "librecovery_rustlib.dylib.so.json"))
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "libavailable_exclude_rustlib", "libavailable_exclude_rustlib.dylib.so", dylibDir, dylibVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(rlibDir, "libavailable_exclude_rustlib.dylib.so.json"))
	}

	// Verify that each json file for an included module has a rule.
	for _, jsonFile := range includeJsonFiles {
		if snapshotSingleton.MaybeOutput(jsonFile).Rule == nil {
			t.Errorf("include json file %q not found", jsonFile)
		}
	}

	// Verify that each json file for an excluded module has no rule.
	for _, jsonFile := range excludeJsonFiles {
		if snapshotSingleton.MaybeOutput(jsonFile).Rule != nil {
			t.Errorf("exclude json file %q found", jsonFile)
		}
	}
}

func TestRecoverySnapshotDirected(t *testing.T) {
	bp := `
	rust_ffi_shared {
		name: "librecovery",
		recovery: true,
		crate_name: "recovery",
		srcs: ["foo.rs"],
	}

	rust_ffi_shared {
		name: "librecovery_available",
		recovery_available: true,
		crate_name: "recovery_available",
		srcs: ["foo.rs"],
	}

	rust_library {
		name: "librecovery_rustlib",
		recovery: true,
		crate_name: "recovery",
		srcs: ["foo.rs"],
	}

	rust_library {
		name: "librecovery_available_rustlib",
		recovery_available: true,
		crate_name: "recovery_available",
		srcs: ["foo.rs"],
	}

	/* TODO: Uncomment when Rust supports the "prefer" property for prebuilts
	rust_library_rlib {
		name: "libfoo_rlib",
		recovery: true,
		crate_name: "foo",
	}

	rust_prebuilt_rlib {
		name: "libfoo_rlib",
		recovery: true,
		prefer: true,
		srcs: ["libfoo.rlib"],
		crate_name: "foo",
	}
	*/
`
	ctx := testRustRecoveryFsVersions(t, bp, rustMockedFiles, "current", "29", "current")
	ctx.Config().TestProductVariables.RecoverySnapshotModules = make(map[string]bool)
	ctx.Config().TestProductVariables.RecoverySnapshotModules["librecovery"] = true
	ctx.Config().TestProductVariables.RecoverySnapshotModules["librecovery_rustlib"] = true
	ctx.Config().TestProductVariables.DirectedRecoverySnapshot = true

	// Check recovery snapshot output.
	snapshotDir := "recovery-snapshot"
	snapshotVariantPath := filepath.Join("out/soong", snapshotDir, "arm64")
	snapshotSingleton := ctx.SingletonForTests("recovery-snapshot")

	var includeJsonFiles []string

	for _, arch := range [][]string{
		[]string{"arm64", "armv8-a"},
	} {
		archType := arch[0]
		archVariant := arch[1]
		archDir := fmt.Sprintf("arch-%s-%s", archType, archVariant)

		sharedVariant := fmt.Sprintf("android_recovery_%s_%s_shared", archType, archVariant)
		rlibVariant := fmt.Sprintf("android_recovery_%s_%s_rlib_dylib-std", archType, archVariant)
		rlibRlibStdVariant := fmt.Sprintf("android_recovery_%s_%s_rlib_rlib-std", archType, archVariant)
		dylibVariant := fmt.Sprintf("android_recovery_%s_%s_dylib", archType, archVariant)
		sharedDir := filepath.Join(snapshotVariantPath, archDir, "shared")
		rlibDir := filepath.Join(snapshotVariantPath, archDir, "rlib")
		dylibDir := filepath.Join(snapshotVariantPath, archDir, "dylib")

		// Included modules
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librecovery", "librecovery.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "librecovery.so.json"))
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librecovery_rustlib", "librecovery_rustlib.rlib", rlibDir, rlibVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(rlibDir, "librecovery_rustlib.rlib.json"))
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librecovery_rustlib", "librecovery_rustlib.rlib-std.rlib", rlibDir, rlibRlibStdVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(rlibDir, "librecovery_rustlib.rlib-std.rlib.json"))
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librecovery_rustlib", "librecovery_rustlib.dylib.so", dylibDir, dylibVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(dylibDir, "librecovery_rustlib.dylib.so.json"))

		// TODO: When Rust supports the "prefer" property for prebuilts, perform this check.
		/*
			// Check that snapshot captures "prefer: true" prebuilt
			cc.CheckSnapshot(t, ctx, snapshotSingleton, "prebuilt_libfoo_rlib", "libfoo_rlib.rlib", rlibDir, rlibVariant)
			includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "libfoo_rlib.rlib.json"))
		*/

		// Excluded modules. Modules not included in the directed recovery snapshot
		// are still included as fake modules.
		cc.CheckSnapshotRule(t, ctx, snapshotSingleton, "librecovery_available", "librecovery_available.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "librecovery_available.so.json"))
		cc.CheckSnapshotRule(t, ctx, snapshotSingleton, "librecovery_available_rustlib", "librecovery_available_rustlib.rlib", rlibDir, rlibVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(rlibDir, "librecovery_available_rustlib.rlib.json"))
		cc.CheckSnapshotRule(t, ctx, snapshotSingleton, "librecovery_available_rustlib", "librecovery_available_rustlib.rlib-std.rlib", rlibDir, rlibRlibStdVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(rlibDir, "librecovery_available_rustlib.rlib-std.rlib.json"))
		cc.CheckSnapshotRule(t, ctx, snapshotSingleton, "librecovery_available_rustlib", "librecovery_available_rustlib.dylib.so", dylibDir, dylibVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(dylibDir, "librecovery_available_rustlib.dylib.so.json"))
	}

	// Verify that each json file for an included module has a rule.
	for _, jsonFile := range includeJsonFiles {
		if snapshotSingleton.MaybeOutput(jsonFile).Rule == nil {
			t.Errorf("include json file %q not found, %#v", jsonFile, includeJsonFiles)
		}
	}
}
