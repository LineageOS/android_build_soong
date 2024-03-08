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
	"android/soong/android"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func checkJsonContents(t *testing.T, ctx *android.TestContext, snapshotSingleton android.TestingSingleton, jsonPath string, key string, value string) {
	jsonOut := snapshotSingleton.MaybeOutput(jsonPath)
	if jsonOut.Rule == nil {
		t.Errorf("%q expected but not found", jsonPath)
		return
	}
	content := android.ContentFromFileRuleForTests(t, ctx, jsonOut)
	if !strings.Contains(content, fmt.Sprintf("%q:%q", key, value)) {
		t.Errorf("%q must include %q:%q but it only has %v", jsonPath, key, value, jsonOut.Args["content"])
	}
}

func TestVendorSnapshotCapture(t *testing.T) {
	bp := `
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
	}

	cc_library {
		name: "libvendor_override",
		vendor: true,
		nocrt: true,
		overrides: ["libvendor"],
	}

	cc_library {
		name: "libvendor_available",
		vendor_available: true,
		nocrt: true,
		min_sdk_version: "29",
	}

	cc_library_headers {
		name: "libvendor_headers",
		vendor_available: true,
		nocrt: true,
	}

	cc_binary {
		name: "vendor_bin",
		vendor: true,
		nocrt: true,
	}

	cc_binary {
		name: "vendor_available_bin",
		vendor_available: true,
		nocrt: true,
	}

	cc_binary {
		name: "vendor_bin_override",
		vendor: true,
		nocrt: true,
		overrides: ["vendor_bin"],
	}

	cc_prebuilt_library_static {
		name: "libb",
		vendor_available: true,
		srcs: ["libb.a"],
		nocrt: true,
		no_libcrt: true,
		stl: "none",
	}

	cc_object {
		name: "obj",
		vendor_available: true,
	}

	cc_library {
		name: "libllndk",
		llndk: {
			symbol_file: "libllndk.map.txt",
		},
	}
`

	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("29")
	ctx := testCcWithConfig(t, config)

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
		CheckSnapshot(t, ctx, snapshotSingleton, "libvendor", "libvendor.so", sharedDir, sharedVariant)
		CheckSnapshot(t, ctx, snapshotSingleton, "libvendor_available", "libvendor_available.so", sharedDir, sharedVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(sharedDir, "libvendor.so.json"),
			filepath.Join(sharedDir, "libvendor_available.so.json"))

		// LLNDK modules are not captured
		CheckSnapshotExclude(t, ctx, snapshotSingleton, "libllndk", "libllndk.so", sharedDir, sharedVariant)

		// For static libraries, all vendor:true and vendor_available modules (including VNDK) are captured.
		// Also cfi variants are captured, except for prebuilts like toolchain_library
		staticVariant := fmt.Sprintf("android_vendor.29_%s_%s_static", archType, archVariant)
		staticCfiVariant := fmt.Sprintf("android_vendor.29_%s_%s_static_cfi", archType, archVariant)
		staticDir := filepath.Join(snapshotVariantPath, archDir, "static")
		CheckSnapshot(t, ctx, snapshotSingleton, "libb", "libb.a", staticDir, staticVariant)
		CheckSnapshot(t, ctx, snapshotSingleton, "libvndk", "libvndk.a", staticDir, staticVariant)
		CheckSnapshot(t, ctx, snapshotSingleton, "libvndk", "libvndk.cfi.a", staticDir, staticCfiVariant)
		CheckSnapshot(t, ctx, snapshotSingleton, "libvendor", "libvendor.a", staticDir, staticVariant)
		CheckSnapshot(t, ctx, snapshotSingleton, "libvendor", "libvendor.cfi.a", staticDir, staticCfiVariant)
		CheckSnapshot(t, ctx, snapshotSingleton, "libvendor_available", "libvendor_available.a", staticDir, staticVariant)
		CheckSnapshot(t, ctx, snapshotSingleton, "libvendor_available", "libvendor_available.cfi.a", staticDir, staticCfiVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(staticDir, "libb.a.json"),
			filepath.Join(staticDir, "libvndk.a.json"),
			filepath.Join(staticDir, "libvndk.cfi.a.json"),
			filepath.Join(staticDir, "libvendor.a.json"),
			filepath.Join(staticDir, "libvendor.cfi.a.json"),
			filepath.Join(staticDir, "libvendor_available.a.json"),
			filepath.Join(staticDir, "libvendor_available.cfi.a.json"))

		checkJsonContents(t, ctx, snapshotSingleton, filepath.Join(staticDir, "libb.a.json"), "MinSdkVersion", "apex_inherit")
		checkJsonContents(t, ctx, snapshotSingleton, filepath.Join(staticDir, "libvendor_available.a.json"), "MinSdkVersion", "29")

		// For binary executables, all vendor:true and vendor_available modules are captured.
		if archType == "arm64" {
			binaryVariant := fmt.Sprintf("android_vendor.29_%s_%s", archType, archVariant)
			binaryDir := filepath.Join(snapshotVariantPath, archDir, "binary")
			CheckSnapshot(t, ctx, snapshotSingleton, "vendor_bin", "vendor_bin", binaryDir, binaryVariant)
			CheckSnapshot(t, ctx, snapshotSingleton, "vendor_available_bin", "vendor_available_bin", binaryDir, binaryVariant)
			jsonFiles = append(jsonFiles,
				filepath.Join(binaryDir, "vendor_bin.json"),
				filepath.Join(binaryDir, "vendor_available_bin.json"))

			checkOverrides(t, ctx, snapshotSingleton, filepath.Join(binaryDir, "vendor_bin_override.json"), []string{"vendor_bin"})
		}

		// For header libraries, all vendor:true and vendor_available modules are captured.
		headerDir := filepath.Join(snapshotVariantPath, archDir, "header")
		jsonFiles = append(jsonFiles, filepath.Join(headerDir, "libvendor_headers.json"))

		// For object modules, all vendor:true and vendor_available modules are captured.
		objectVariant := fmt.Sprintf("android_vendor.29_%s_%s", archType, archVariant)
		objectDir := filepath.Join(snapshotVariantPath, archDir, "object")
		CheckSnapshot(t, ctx, snapshotSingleton, "obj", "obj.o", objectDir, objectVariant)
		jsonFiles = append(jsonFiles, filepath.Join(objectDir, "obj.o.json"))

		checkOverrides(t, ctx, snapshotSingleton, filepath.Join(sharedDir, "libvendor_override.so.json"), []string{"libvendor"})
	}

	for _, jsonFile := range jsonFiles {
		// verify all json files exist
		if snapshotSingleton.MaybeOutput(jsonFile).Rule == nil {
			t.Errorf("%q expected but not found", jsonFile)
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
	cc_library_shared {
		name: "libvendor",
		vendor: true,
		nocrt: true,
	}

	cc_library_shared {
		name: "libvendor_available",
		vendor_available: true,
		nocrt: true,
	}

	genrule {
		name: "libfoo_gen",
		cmd: "",
		out: ["libfoo.so"],
	}

	cc_prebuilt_library_shared {
		name: "libfoo",
		vendor: true,
		prefer: true,
		srcs: [":libfoo_gen"],
	}

	cc_library_shared {
		name: "libfoo",
		vendor: true,
		nocrt: true,
	}
`
	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("29")
	config.TestProductVariables.DirectedVendorSnapshot = true
	config.TestProductVariables.VendorSnapshotModules = make(map[string]bool)
	config.TestProductVariables.VendorSnapshotModules["libvendor"] = true
	config.TestProductVariables.VendorSnapshotModules["libfoo"] = true
	ctx := testCcWithConfig(t, config)

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
		sharedDir := filepath.Join(snapshotVariantPath, archDir, "shared")

		// Included modules
		CheckSnapshot(t, ctx, snapshotSingleton, "libvendor", "libvendor.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "libvendor.so.json"))
		// Check that snapshot captures "prefer: true" prebuilt
		CheckSnapshot(t, ctx, snapshotSingleton, "prebuilt_libfoo", "libfoo.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "libfoo.so.json"))

		// Excluded modules. Modules not included in the directed vendor snapshot
		// are still include as fake modules.
		CheckSnapshotRule(t, ctx, snapshotSingleton, "libvendor_available", "libvendor_available.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "libvendor_available.so.json"))
	}

	// Verify that each json file for an included module has a rule.
	for _, jsonFile := range includeJsonFiles {
		if snapshotSingleton.MaybeOutput(jsonFile).Rule == nil {
			t.Errorf("include json file %q not found", jsonFile)
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

	cc_library {
		name: "libllndk",
		llndk: {
			symbol_file: "libllndk.map.txt",
		},
	}

	cc_binary {
		name: "bin",
		vendor: true,
		nocrt: true,
		no_libcrt: true,
		stl: "none",
		system_shared_libs: [],
	}

	cc_binary {
		name: "bin32",
		vendor: true,
		nocrt: true,
		no_libcrt: true,
		stl: "none",
		system_shared_libs: [],
		compile_multilib: "32",
	}
`

	vndkBp := `
	vndk_prebuilt_shared {
		name: "libvndk",
		version: "31",
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
		version: "31",
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

	vndk_prebuilt_shared {
		name: "libllndk",
		version: "31",
		target_arch: "arm64",
		vendor_available: true,
		product_available: true,
		arch: {
			arm64: {
				srcs: ["libllndk.so"],
			},
			arm: {
				srcs: ["libllndk.so"],
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
		stl: "none",
		system_shared_libs: [],
	}

	cc_library_shared {
		name: "libclient",
		vendor: true,
		nocrt: true,
		no_libcrt: true,
		stl: "none",
		system_shared_libs: [],
		shared_libs: ["libvndk", "libvendor_available", "libllndk"],
		static_libs: ["libvendor", "libvendor_without_snapshot"],
		arch: {
			arm64: {
				shared_libs: ["lib64"],
			},
			arm: {
				shared_libs: ["lib32"],
			},
		},
		srcs: ["client.cpp"],
	}

	cc_library_shared {
		name: "libclient_cfi",
		vendor: true,
		nocrt: true,
		no_libcrt: true,
		stl: "none",
		system_shared_libs: [],
		static_libs: ["libvendor"],
		sanitize: {
			cfi: true,
		},
		srcs: ["client.cpp"],
	}

	cc_library_shared {
		name: "libvndkext",
		vendor: true,
		nocrt: true,
		no_libcrt: true,
		stl: "none",
		system_shared_libs: [],
		vndk: {
			extends: "libvndk",
			enabled: true,
		}
	}

	cc_binary {
		name: "bin_without_snapshot",
		vendor: true,
		nocrt: true,
		no_libcrt: true,
		stl: "libc++_static",
		system_shared_libs: [],
		static_libs: ["libvndk"],
		srcs: ["bin.cpp"],
	}

	vendor_snapshot {
		name: "vendor_snapshot",
		version: "31",
		arch: {
			arm64: {
				vndk_libs: [
					"libvndk",
					"libllndk",
				],
				static_libs: [
					"libc++_static",
					"libc++demangle",
					"libunwind",
					"libvendor",
					"libvendor_available",
					"libvndk",
					"lib64",
				],
				shared_libs: [
					"libvendor",
					"libvendor_override",
					"libvendor_available",
					"lib64",
				],
				binaries: [
					"bin",
					"bin_override",
				],
			},
			arm: {
				vndk_libs: [
					"libvndk",
					"libllndk",
				],
				static_libs: [
					"libvendor",
					"libvendor_available",
					"libvndk",
					"lib32",
				],
				shared_libs: [
					"libvendor",
					"libvendor_override",
					"libvendor_available",
					"lib32",
				],
				binaries: [
					"bin32",
				],
			},
		}
	}

	vendor_snapshot_static {
		name: "libvndk",
		version: "31",
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

	vendor_snapshot_shared {
		name: "libvendor",
		version: "31",
		target_arch: "arm64",
		compile_multilib: "both",
		vendor: true,
		shared_libs: [
			"libvendor_without_snapshot",
			"libvendor_available",
			"libvndk",
		],
		arch: {
			arm64: {
				src: "libvendor.so",
				export_include_dirs: ["include/libvendor"],
			},
			arm: {
				src: "libvendor.so",
				export_include_dirs: ["include/libvendor"],
			},
		},
	}

	vendor_snapshot_shared {
		name: "libvendor_override",
		version: "31",
		target_arch: "arm64",
		compile_multilib: "both",
		vendor: true,
		overrides: ["libvendor"],
		shared_libs: [
			"libvendor_without_snapshot",
			"libvendor_available",
			"libvndk",
		],
		arch: {
			arm64: {
				src: "override/libvendor.so",
				export_include_dirs: ["include/libvendor"],
			},
			arm: {
				src: "override/libvendor.so",
				export_include_dirs: ["include/libvendor"],
			},
		},
	}

	vendor_snapshot_static {
		name: "lib32",
		version: "31",
		target_arch: "arm64",
		compile_multilib: "32",
		vendor: true,
		arch: {
			arm: {
				src: "lib32.a",
			},
		},
	}

	vendor_snapshot_shared {
		name: "lib32",
		version: "31",
		target_arch: "arm64",
		compile_multilib: "32",
		vendor: true,
		arch: {
			arm: {
				src: "lib32.so",
			},
		},
	}

	vendor_snapshot_static {
		name: "lib64",
		version: "31",
		target_arch: "arm64",
		compile_multilib: "64",
		vendor: true,
		arch: {
			arm64: {
				src: "lib64.a",
			},
		},
	}

	vendor_snapshot_shared {
		name: "lib64",
		version: "31",
		target_arch: "arm64",
		compile_multilib: "64",
		vendor: true,
		arch: {
			arm64: {
				src: "lib64.so",
			},
		},
	}

	vendor_snapshot_static {
		name: "libvendor",
		version: "31",
		target_arch: "arm64",
		compile_multilib: "both",
		vendor: true,
		arch: {
			arm64: {
				cfi: {
					src: "libvendor.cfi.a",
					export_include_dirs: ["include/libvendor_cfi"],
				},
				src: "libvendor.a",
				export_include_dirs: ["include/libvendor"],
			},
			arm: {
				cfi: {
					src: "libvendor.cfi.a",
					export_include_dirs: ["include/libvendor_cfi"],
				},
				src: "libvendor.a",
				export_include_dirs: ["include/libvendor"],
			},
		},
	}

	vendor_snapshot_shared {
		name: "libvendor_available",
		version: "31",
		target_arch: "arm64",
		compile_multilib: "both",
		vendor: true,
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

	vendor_snapshot_static {
		name: "libvendor_available",
		version: "31",
		target_arch: "arm64",
		compile_multilib: "both",
		vendor: true,
		arch: {
			arm64: {
				src: "libvendor_available.a",
				export_include_dirs: ["include/libvendor"],
			},
			arm: {
				src: "libvendor_available.so",
				export_include_dirs: ["include/libvendor"],
			},
		},
	}

	vendor_snapshot_static {
		name: "libc++_static",
		version: "31",
		target_arch: "arm64",
		compile_multilib: "64",
		vendor: true,
		arch: {
			arm64: {
				src: "libc++_static.a",
			},
		},
	}

	vendor_snapshot_static {
		name: "libc++demangle",
		version: "31",
		target_arch: "arm64",
		compile_multilib: "64",
		vendor: true,
		arch: {
			arm64: {
				src: "libc++demangle.a",
			},
		},
	}

	vendor_snapshot_static {
		name: "libunwind",
		version: "31",
		target_arch: "arm64",
		compile_multilib: "64",
		vendor: true,
		arch: {
			arm64: {
				src: "libunwind.a",
			},
		},
	}

	vendor_snapshot_binary {
		name: "bin",
		version: "31",
		target_arch: "arm64",
		compile_multilib: "64",
		vendor: true,
		arch: {
			arm64: {
				src: "bin",
			},
		},
		symlinks: ["binfoo", "binbar"],
	}

	vendor_snapshot_binary {
		name: "bin_override",
		version: "31",
		target_arch: "arm64",
		compile_multilib: "64",
		vendor: true,
		overrides: ["bin"],
		arch: {
			arm64: {
				src: "override/bin",
			},
		},
		symlinks: ["binfoo", "binbar"],
	}

	vendor_snapshot_binary {
		name: "bin32",
		version: "31",
		target_arch: "arm64",
		compile_multilib: "32",
		vendor: true,
		arch: {
			arm: {
				src: "bin32",
			},
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
		version: "31",
		target_arch: "arm",
		compile_multilib: "first",
		vendor: true,
		arch: {
			arm64: {
				src: "bin",
			},
		},
	}
`
	depsBp := GatherRequiredDepsForTest(android.Android)

	mockFS := map[string][]byte{
		"deps/Android.bp":                  []byte(depsBp),
		"framework/Android.bp":             []byte(frameworkBp),
		"framework/symbol.txt":             nil,
		"vendor/Android.bp":                []byte(vendorProprietaryBp),
		"vendor/bin":                       nil,
		"vendor/override/bin":              nil,
		"vendor/bin32":                     nil,
		"vendor/bin.cpp":                   nil,
		"vendor/client.cpp":                nil,
		"vendor/include/libvndk/a.h":       nil,
		"vendor/include/libvendor/b.h":     nil,
		"vendor/include/libvendor_cfi/c.h": nil,
		"vendor/libc++_static.a":           nil,
		"vendor/libc++demangle.a":          nil,
		"vendor/libunwind.a":               nil,
		"vendor/libvndk.a":                 nil,
		"vendor/libvendor.a":               nil,
		"vendor/libvendor.cfi.a":           nil,
		"vendor/libvendor.so":              nil,
		"vendor/override/libvendor.so":     nil,
		"vendor/lib32.a":                   nil,
		"vendor/lib32.so":                  nil,
		"vendor/lib64.a":                   nil,
		"vendor/lib64.so":                  nil,
		"vndk/Android.bp":                  []byte(vndkBp),
		"vndk/include/libvndk/a.h":         nil,
		"vndk/libvndk.so":                  nil,
		"vndk/libllndk.so":                 nil,
	}

	config := TestConfig(t.TempDir(), android.Android, nil, "", mockFS)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("31")
	config.TestProductVariables.Platform_vndk_version = StringPtr("32")
	ctx := CreateTestContext(config)
	ctx.Register()

	_, errs := ctx.ParseFileList(".", []string{"deps/Android.bp", "framework/Android.bp", "vendor/Android.bp", "vndk/Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	sharedVariant := "android_vendor.31_arm64_armv8-a_shared"
	staticVariant := "android_vendor.31_arm64_armv8-a_static"
	binaryVariant := "android_vendor.31_arm64_armv8-a"

	sharedCfiVariant := "android_vendor.31_arm64_armv8-a_shared_cfi"
	staticCfiVariant := "android_vendor.31_arm64_armv8-a_static_cfi"

	shared32Variant := "android_vendor.31_arm_armv7-a-neon_shared"
	binary32Variant := "android_vendor.31_arm_armv7-a-neon"

	// libclient uses libvndk.vndk.31.arm64, libvendor.vendor_static.31.arm64, libvendor_without_snapshot
	libclientCcFlags := ctx.ModuleForTests("libclient", sharedVariant).Rule("cc").Args["cFlags"]
	for _, includeFlags := range []string{
		"-Ivndk/include/libvndk",     // libvndk
		"-Ivendor/include/libvendor", // libvendor
	} {
		if !strings.Contains(libclientCcFlags, includeFlags) {
			t.Errorf("flags for libclient must contain %#v, but was %#v.",
				includeFlags, libclientCcFlags)
		}
	}

	libclientLdFlags := ctx.ModuleForTests("libclient", sharedVariant).Rule("ld").Args["libFlags"]
	for _, input := range [][]string{
		[]string{sharedVariant, "libvndk.vndk.31.arm64"},
		[]string{sharedVariant, "libllndk.vndk.31.arm64"},
		[]string{staticVariant, "libvendor.vendor_static.31.arm64"},
		[]string{staticVariant, "libvendor_without_snapshot"},
	} {
		outputPaths := GetOutputPaths(ctx, input[0] /* variant */, []string{input[1]} /* module name */)
		if !strings.Contains(libclientLdFlags, outputPaths[0].String()) {
			t.Errorf("libflags for libclient must contain %#v, but was %#v", outputPaths[0], libclientLdFlags)
		}

	}

	libclientAndroidMkSharedLibs := ctx.ModuleForTests("libclient", sharedVariant).Module().(*Module).Properties.AndroidMkSharedLibs
	if g, w := libclientAndroidMkSharedLibs, []string{"libvndk.vendor", "libvendor_available.vendor", "libllndk.vendor", "lib64"}; !reflect.DeepEqual(g, w) {
		t.Errorf("wanted libclient AndroidMkSharedLibs %q, got %q", w, g)
	}

	libclientAndroidMkStaticLibs := ctx.ModuleForTests("libclient", sharedVariant).Module().(*Module).Properties.AndroidMkStaticLibs
	if g, w := libclientAndroidMkStaticLibs, []string{"libvendor", "libvendor_without_snapshot"}; !reflect.DeepEqual(g, w) {
		t.Errorf("wanted libclient AndroidMkStaticLibs %q, got %q", w, g)
	}

	libclient32AndroidMkSharedLibs := ctx.ModuleForTests("libclient", shared32Variant).Module().(*Module).Properties.AndroidMkSharedLibs
	if g, w := libclient32AndroidMkSharedLibs, []string{"libvndk.vendor", "libvendor_available.vendor", "libllndk.vendor", "lib32"}; !reflect.DeepEqual(g, w) {
		t.Errorf("wanted libclient32 AndroidMkSharedLibs %q, got %q", w, g)
	}

	// libclient_cfi uses libvendor.vendor_static.31.arm64's cfi variant
	libclientCfiCcFlags := ctx.ModuleForTests("libclient_cfi", sharedCfiVariant).Rule("cc").Args["cFlags"]
	if !strings.Contains(libclientCfiCcFlags, "-Ivendor/include/libvendor_cfi") {
		t.Errorf("flags for libclient_cfi must contain %#v, but was %#v.",
			"-Ivendor/include/libvendor_cfi", libclientCfiCcFlags)
	}

	libclientCfiLdFlags := ctx.ModuleForTests("libclient_cfi", sharedCfiVariant).Rule("ld").Args["libFlags"]
	libvendorCfiOutputPaths := GetOutputPaths(ctx, staticCfiVariant, []string{"libvendor.vendor_static.31.arm64"})
	if !strings.Contains(libclientCfiLdFlags, libvendorCfiOutputPaths[0].String()) {
		t.Errorf("libflags for libclientCfi must contain %#v, but was %#v", libvendorCfiOutputPaths[0], libclientCfiLdFlags)
	}

	// bin_without_snapshot uses libvndk.vendor_static.31.arm64 (which reexports vndk's exported headers)
	binWithoutSnapshotCcFlags := ctx.ModuleForTests("bin_without_snapshot", binaryVariant).Rule("cc").Args["cFlags"]
	if !strings.Contains(binWithoutSnapshotCcFlags, "-Ivndk/include/libvndk") {
		t.Errorf("flags for bin_without_snapshot must contain %#v, but was %#v.",
			"-Ivendor/include/libvndk", binWithoutSnapshotCcFlags)
	}

	binWithoutSnapshotLdFlags := ctx.ModuleForTests("bin_without_snapshot", binaryVariant).Rule("ld").Args["libFlags"]
	libVndkStaticOutputPaths := GetOutputPaths(ctx, staticVariant, []string{"libvndk.vendor_static.31.arm64"})
	if !strings.Contains(binWithoutSnapshotLdFlags, libVndkStaticOutputPaths[0].String()) {
		t.Errorf("libflags for bin_without_snapshot must contain %#v, but was %#v",
			libVndkStaticOutputPaths[0], binWithoutSnapshotLdFlags)
	}

	// libvendor.so is installed by libvendor.vendor_shared.31.arm64
	ctx.ModuleForTests("libvendor.vendor_shared.31.arm64", sharedVariant).Output("libvendor.so")

	// lib64.so is installed by lib64.vendor_shared.31.arm64
	ctx.ModuleForTests("lib64.vendor_shared.31.arm64", sharedVariant).Output("lib64.so")

	// lib32.so is installed by lib32.vendor_shared.31.arm64
	ctx.ModuleForTests("lib32.vendor_shared.31.arm64", shared32Variant).Output("lib32.so")

	// libvendor_available.so is installed by libvendor_available.vendor_shared.31.arm64
	ctx.ModuleForTests("libvendor_available.vendor_shared.31.arm64", sharedVariant).Output("libvendor_available.so")

	// libvendor_without_snapshot.so is installed by libvendor_without_snapshot
	ctx.ModuleForTests("libvendor_without_snapshot", sharedVariant).Output("libvendor_without_snapshot.so")

	// bin is installed by bin.vendor_binary.31.arm64
	bin64Module := ctx.ModuleForTests("bin.vendor_binary.31.arm64", binaryVariant)
	bin64Module.Output("bin")

	// also test symlinks
	bin64MkEntries := android.AndroidMkEntriesForTest(t, ctx, bin64Module.Module())
	bin64KatiSymlinks := bin64MkEntries[0].EntryMap["LOCAL_SOONG_INSTALL_SYMLINKS"]

	// Either AndroidMk entries contain symlinks, or symlinks should be installed by Soong
	for _, symlink := range []string{"binfoo", "binbar"} {
		if inList(symlink, bin64KatiSymlinks) {
			continue
		}

		bin64Module.Output(symlink)
	}

	// bin32 is installed by bin32.vendor_binary.31.arm64
	ctx.ModuleForTests("bin32.vendor_binary.31.arm64", binary32Variant).Output("bin32")

	// bin_without_snapshot is installed by bin_without_snapshot
	ctx.ModuleForTests("bin_without_snapshot", binaryVariant).Output("bin_without_snapshot")

	// libvendor, libvendor_available and bin don't have vendor.31 variant
	libvendorVariants := ctx.ModuleVariantsForTests("libvendor")
	if inList(sharedVariant, libvendorVariants) {
		t.Errorf("libvendor must not have variant %#v, but it does", sharedVariant)
	}

	libvendorAvailableVariants := ctx.ModuleVariantsForTests("libvendor_available")
	if inList(sharedVariant, libvendorAvailableVariants) {
		t.Errorf("libvendor_available must not have variant %#v, but it does", sharedVariant)
	}

	binVariants := ctx.ModuleVariantsForTests("bin")
	if inList(binaryVariant, binVariants) {
		t.Errorf("bin must not have variant %#v, but it does", sharedVariant)
	}

	// test overrides property
	binOverrideModule := ctx.ModuleForTests("bin_override.vendor_binary.31.arm64", binaryVariant)
	binOverrideModule.Output("bin")
	binOverrideMkEntries := android.AndroidMkEntriesForTest(t, ctx, binOverrideModule.Module())
	binOverrideEntry := binOverrideMkEntries[0].EntryMap["LOCAL_OVERRIDES_MODULES"]
	if !inList("bin", binOverrideEntry) {
		t.Errorf("bin_override must override bin but was %q\n", binOverrideEntry)
	}

	libvendorOverrideModule := ctx.ModuleForTests("libvendor_override.vendor_shared.31.arm64", sharedVariant)
	libvendorOverrideModule.Output("libvendor.so")
	libvendorOverrideMkEntries := android.AndroidMkEntriesForTest(t, ctx, libvendorOverrideModule.Module())
	libvendorOverrideEntry := libvendorOverrideMkEntries[0].EntryMap["LOCAL_OVERRIDES_MODULES"]
	if !inList("libvendor", libvendorOverrideEntry) {
		t.Errorf("libvendor_override must override libvendor but was %q\n", libvendorOverrideEntry)
	}
}

func TestVendorSnapshotSanitizer(t *testing.T) {
	bp := `
	vendor_snapshot {
		name: "vendor_snapshot",
		version: "28",
		arch: {
			arm64: {
				static_libs: [
					"libsnapshot",
					"note_memtag_heap_sync",
				],
				objects: [
					"snapshot_object",
				],
				vndk_libs: [
					"libclang_rt.hwasan",
				],
			},
		},
	}

	vendor_snapshot_static {
		name: "libsnapshot",
		vendor: true,
		target_arch: "arm64",
		version: "28",
		arch: {
			arm64: {
				src: "libsnapshot.a",
				cfi: {
					src: "libsnapshot.cfi.a",
				},
				hwasan: {
					src: "libsnapshot.hwasan.a",
				},
			},
		},
	}

	vendor_snapshot_static {
		name: "note_memtag_heap_sync",
		vendor: true,
		target_arch: "arm64",
		version: "28",
		arch: {
			arm64: {
				src: "note_memtag_heap_sync.a",
			},
		},
	}

	vndk_prebuilt_shared {
		name: "libclang_rt.hwasan",
		version: "28",
		target_arch: "arm64",
		vendor_available: true,
		product_available: true,
		vndk: {
			enabled: true,
		},
		arch: {
			arm64: {
				srcs: ["libclang_rt.hwasan.so"],
			},
		},
	}

	vendor_snapshot_object {
		name: "snapshot_object",
		vendor: true,
		target_arch: "arm64",
		version: "28",
		arch: {
			arm64: {
				src: "snapshot_object.o",
			},
		},
		stl: "none",
	}

	cc_test {
		name: "vstest",
		gtest: false,
		vendor: true,
		compile_multilib: "64",
		nocrt: true,
		no_libcrt: true,
		stl: "none",
		static_libs: ["libsnapshot"],
		system_shared_libs: [],
	}
`

	mockFS := map[string][]byte{
		"vendor/Android.bp":              []byte(bp),
		"vendor/libc++demangle.a":        nil,
		"vendor/libclang_rt.hwasan.so":   nil,
		"vendor/libsnapshot.a":           nil,
		"vendor/libsnapshot.cfi.a":       nil,
		"vendor/libsnapshot.hwasan.a":    nil,
		"vendor/note_memtag_heap_sync.a": nil,
		"vendor/snapshot_object.o":       nil,
	}

	config := TestConfig(t.TempDir(), android.Android, nil, "", mockFS)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("28")
	config.TestProductVariables.Platform_vndk_version = StringPtr("29")
	config.TestProductVariables.SanitizeDevice = []string{"hwaddress"}
	ctx := testCcWithConfig(t, config)

	// Check non-cfi, cfi and hwasan variant.
	staticVariant := "android_vendor.28_arm64_armv8-a_static"
	staticCfiVariant := "android_vendor.28_arm64_armv8-a_static_cfi"
	staticHwasanVariant := "android_vendor.28_arm64_armv8-a_static_hwasan"
	staticHwasanCfiVariant := "android_vendor.28_arm64_armv8-a_static_hwasan_cfi"

	staticModule := ctx.ModuleForTests("libsnapshot.vendor_static.28.arm64", staticVariant).Module().(*Module)
	assertString(t, staticModule.outputFile.Path().Base(), "libsnapshot.a")

	staticCfiModule := ctx.ModuleForTests("libsnapshot.vendor_static.28.arm64", staticCfiVariant).Module().(*Module)
	assertString(t, staticCfiModule.outputFile.Path().Base(), "libsnapshot.cfi.a")

	staticHwasanModule := ctx.ModuleForTests("libsnapshot.vendor_static.28.arm64", staticHwasanVariant).Module().(*Module)
	assertString(t, staticHwasanModule.outputFile.Path().Base(), "libsnapshot.hwasan.a")

	staticHwasanCfiModule := ctx.ModuleForTests("libsnapshot.vendor_static.28.arm64", staticHwasanCfiVariant).Module().(*Module)
	if !staticHwasanCfiModule.HiddenFromMake() || !staticHwasanCfiModule.PreventInstall() {
		t.Errorf("Hwasan and Cfi cannot enabled at the same time.")
	}

	snapshotObjModule := ctx.ModuleForTests("snapshot_object.vendor_object.28.arm64", "android_vendor.28_arm64_armv8-a").Module()
	snapshotObjMkEntries := android.AndroidMkEntriesForTest(t, ctx, snapshotObjModule)
	// snapshot object must not add ".hwasan" suffix
	assertString(t, snapshotObjMkEntries[0].EntryMap["LOCAL_MODULE"][0], "snapshot_object")
}

func TestVendorSnapshotExclude(t *testing.T) {

	// This test verifies that the exclude_from_vendor_snapshot property
	// makes its way from the Android.bp source file into the module data
	// structure. It also verifies that modules are correctly included or
	// excluded in the vendor snapshot based on their path (framework or
	// vendor) and the exclude_from_vendor_snapshot property.

	frameworkBp := `
		cc_library_shared {
			name: "libinclude",
			srcs: ["src/include.cpp"],
			vendor_available: true,
		}
		cc_library_shared {
			name: "libexclude",
			srcs: ["src/exclude.cpp"],
			vendor: true,
			exclude_from_vendor_snapshot: true,
		}
		cc_library_shared {
			name: "libavailable_exclude",
			srcs: ["src/exclude.cpp"],
			vendor_available: true,
			exclude_from_vendor_snapshot: true,
		}
	`

	vendorProprietaryBp := `
		cc_library_shared {
			name: "libvendor",
			srcs: ["vendor.cpp"],
			vendor: true,
		}
	`

	depsBp := GatherRequiredDepsForTest(android.Android)

	mockFS := map[string][]byte{
		"deps/Android.bp":       []byte(depsBp),
		"framework/Android.bp":  []byte(frameworkBp),
		"framework/include.cpp": nil,
		"framework/exclude.cpp": nil,
		"device/Android.bp":     []byte(vendorProprietaryBp),
		"device/vendor.cpp":     nil,
	}

	config := TestConfig(t.TempDir(), android.Android, nil, "", mockFS)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("29")
	ctx := CreateTestContext(config)
	ctx.Register()

	_, errs := ctx.ParseFileList(".", []string{"deps/Android.bp", "framework/Android.bp", "device/Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	// Test an include and exclude framework module.
	AssertExcludeFromVendorSnapshotIs(t, ctx, "libinclude", false, vendorVariant)
	AssertExcludeFromVendorSnapshotIs(t, ctx, "libexclude", true, vendorVariant)
	AssertExcludeFromVendorSnapshotIs(t, ctx, "libavailable_exclude", true, vendorVariant)

	// A vendor module is excluded, but by its path, not the
	// exclude_from_vendor_snapshot property.
	AssertExcludeFromVendorSnapshotIs(t, ctx, "libvendor", false, vendorVariant)

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

		// Included modules
		CheckSnapshot(t, ctx, snapshotSingleton, "libinclude", "libinclude.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "libinclude.so.json"))

		// Excluded modules
		CheckSnapshotExclude(t, ctx, snapshotSingleton, "libexclude", "libexclude.so", sharedDir, sharedVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(sharedDir, "libexclude.so.json"))
		CheckSnapshotExclude(t, ctx, snapshotSingleton, "libvendor", "libvendor.so", sharedDir, sharedVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(sharedDir, "libvendor.so.json"))
		CheckSnapshotExclude(t, ctx, snapshotSingleton, "libavailable_exclude", "libavailable_exclude.so", sharedDir, sharedVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(sharedDir, "libavailable_exclude.so.json"))
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

func TestVendorSnapshotExcludeInVendorProprietaryPathErrors(t *testing.T) {

	// This test verifies that using the exclude_from_vendor_snapshot
	// property on a module in a vendor proprietary path generates an
	// error. These modules are already excluded, so we prohibit using the
	// property in this way, which could add to confusion.

	vendorProprietaryBp := `
		cc_library_shared {
			name: "libvendor",
			srcs: ["vendor.cpp"],
			vendor: true,
			exclude_from_vendor_snapshot: true,
		}
	`

	depsBp := GatherRequiredDepsForTest(android.Android)

	mockFS := map[string][]byte{
		"deps/Android.bp":   []byte(depsBp),
		"device/Android.bp": []byte(vendorProprietaryBp),
		"device/vendor.cpp": nil,
	}

	config := TestConfig(t.TempDir(), android.Android, nil, "", mockFS)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("29")
	ctx := CreateTestContext(config)
	ctx.Register()

	_, errs := ctx.ParseFileList(".", []string{"deps/Android.bp", "device/Android.bp"})
	android.FailIfErrored(t, errs)

	_, errs = ctx.PrepareBuildActions(config)
	android.CheckErrorsAgainstExpectations(t, errs, []string{
		`module "libvendor\{.+,image:vendor.+,arch:arm64_.+\}" in vendor proprietary path "device" may not use "exclude_from_vendor_snapshot: true"`,
		`module "libvendor\{.+,image:vendor.+,arch:arm_.+\}" in vendor proprietary path "device" may not use "exclude_from_vendor_snapshot: true"`,
		`module "libvendor\{.+,image:vendor.+,arch:arm64_.+\}" in vendor proprietary path "device" may not use "exclude_from_vendor_snapshot: true"`,
		`module "libvendor\{.+,image:vendor.+,arch:arm_.+\}" in vendor proprietary path "device" may not use "exclude_from_vendor_snapshot: true"`,
		`module "libvendor\{.+,image:vendor.+,arch:arm64_.+\}" in vendor proprietary path "device" may not use "exclude_from_vendor_snapshot: true"`,
		`module "libvendor\{.+,image:vendor.+,arch:arm_.+\}" in vendor proprietary path "device" may not use "exclude_from_vendor_snapshot: true"`,
	})
}

func TestRecoverySnapshotCapture(t *testing.T) {
	bp := `
	cc_library {
		name: "libvndk",
		vendor_available: true,
		recovery_available: true,
		product_available: true,
		vndk: {
			enabled: true,
		},
		nocrt: true,
	}

	cc_library {
		name: "librecovery",
		recovery: true,
		nocrt: true,
	}

	cc_library {
		name: "librecovery_available",
		recovery_available: true,
		nocrt: true,
	}

	cc_library_headers {
		name: "librecovery_headers",
		recovery_available: true,
		nocrt: true,
	}

	cc_binary {
		name: "recovery_bin",
		recovery: true,
		nocrt: true,
	}

	cc_binary {
		name: "recovery_available_bin",
		recovery_available: true,
		nocrt: true,
	}

	cc_prebuilt_library_static {
		name: "libb",
		recovery_available: true,
		srcs: ["libb.a"],
		nocrt: true,
		no_libcrt: true,
		stl: "none",
	}

	cc_object {
		name: "obj",
		recovery_available: true,
	}
`
	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)
	config.TestProductVariables.RecoverySnapshotVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("29")
	ctx := testCcWithConfig(t, config)

	// Check Recovery snapshot output.

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

		// For shared libraries, only recovery_available modules are captured.
		sharedVariant := fmt.Sprintf("android_recovery_%s_%s_shared", archType, archVariant)
		sharedDir := filepath.Join(snapshotVariantPath, archDir, "shared")
		CheckSnapshot(t, ctx, snapshotSingleton, "libvndk", "libvndk.so", sharedDir, sharedVariant)
		CheckSnapshot(t, ctx, snapshotSingleton, "librecovery", "librecovery.so", sharedDir, sharedVariant)
		CheckSnapshot(t, ctx, snapshotSingleton, "librecovery_available", "librecovery_available.so", sharedDir, sharedVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(sharedDir, "libvndk.so.json"),
			filepath.Join(sharedDir, "librecovery.so.json"),
			filepath.Join(sharedDir, "librecovery_available.so.json"))

		// For static libraries, all recovery:true and recovery_available modules are captured.
		staticVariant := fmt.Sprintf("android_recovery_%s_%s_static", archType, archVariant)
		staticDir := filepath.Join(snapshotVariantPath, archDir, "static")
		CheckSnapshot(t, ctx, snapshotSingleton, "libb", "libb.a", staticDir, staticVariant)
		CheckSnapshot(t, ctx, snapshotSingleton, "librecovery", "librecovery.a", staticDir, staticVariant)
		CheckSnapshot(t, ctx, snapshotSingleton, "librecovery_available", "librecovery_available.a", staticDir, staticVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(staticDir, "libb.a.json"),
			filepath.Join(staticDir, "librecovery.a.json"),
			filepath.Join(staticDir, "librecovery_available.a.json"))

		// For binary executables, all recovery:true and recovery_available modules are captured.
		if archType == "arm64" {
			binaryVariant := fmt.Sprintf("android_recovery_%s_%s", archType, archVariant)
			binaryDir := filepath.Join(snapshotVariantPath, archDir, "binary")
			CheckSnapshot(t, ctx, snapshotSingleton, "recovery_bin", "recovery_bin", binaryDir, binaryVariant)
			CheckSnapshot(t, ctx, snapshotSingleton, "recovery_available_bin", "recovery_available_bin", binaryDir, binaryVariant)
			jsonFiles = append(jsonFiles,
				filepath.Join(binaryDir, "recovery_bin.json"),
				filepath.Join(binaryDir, "recovery_available_bin.json"))
		}

		// For header libraries, all vendor:true and vendor_available modules are captured.
		headerDir := filepath.Join(snapshotVariantPath, archDir, "header")
		jsonFiles = append(jsonFiles, filepath.Join(headerDir, "librecovery_headers.json"))

		// For object modules, all vendor:true and vendor_available modules are captured.
		objectVariant := fmt.Sprintf("android_recovery_%s_%s", archType, archVariant)
		objectDir := filepath.Join(snapshotVariantPath, archDir, "object")
		CheckSnapshot(t, ctx, snapshotSingleton, "obj", "obj.o", objectDir, objectVariant)
		jsonFiles = append(jsonFiles, filepath.Join(objectDir, "obj.o.json"))
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
		cc_library_shared {
			name: "libinclude",
			srcs: ["src/include.cpp"],
			recovery_available: true,
		}
		cc_library_shared {
			name: "libexclude",
			srcs: ["src/exclude.cpp"],
			recovery: true,
			exclude_from_recovery_snapshot: true,
		}
		cc_library_shared {
			name: "libavailable_exclude",
			srcs: ["src/exclude.cpp"],
			recovery_available: true,
			exclude_from_recovery_snapshot: true,
		}
	`

	vendorProprietaryBp := `
		cc_library_shared {
			name: "librecovery",
			srcs: ["recovery.cpp"],
			recovery: true,
		}
	`

	depsBp := GatherRequiredDepsForTest(android.Android)

	mockFS := map[string][]byte{
		"deps/Android.bp":       []byte(depsBp),
		"framework/Android.bp":  []byte(frameworkBp),
		"framework/include.cpp": nil,
		"framework/exclude.cpp": nil,
		"device/Android.bp":     []byte(vendorProprietaryBp),
		"device/recovery.cpp":   nil,
	}

	config := TestConfig(t.TempDir(), android.Android, nil, "", mockFS)
	config.TestProductVariables.RecoverySnapshotVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("29")
	ctx := CreateTestContext(config)
	ctx.Register()

	_, errs := ctx.ParseFileList(".", []string{"deps/Android.bp", "framework/Android.bp", "device/Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	// Test an include and exclude framework module.
	AssertExcludeFromRecoverySnapshotIs(t, ctx, "libinclude", false, recoveryVariant)
	AssertExcludeFromRecoverySnapshotIs(t, ctx, "libexclude", true, recoveryVariant)
	AssertExcludeFromRecoverySnapshotIs(t, ctx, "libavailable_exclude", true, recoveryVariant)

	// A recovery module is excluded, but by its path, not the
	// exclude_from_recovery_snapshot property.
	AssertExcludeFromRecoverySnapshotIs(t, ctx, "librecovery", false, recoveryVariant)

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
		sharedDir := filepath.Join(snapshotVariantPath, archDir, "shared")

		// Included modules
		CheckSnapshot(t, ctx, snapshotSingleton, "libinclude", "libinclude.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "libinclude.so.json"))

		// Excluded modules
		CheckSnapshotExclude(t, ctx, snapshotSingleton, "libexclude", "libexclude.so", sharedDir, sharedVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(sharedDir, "libexclude.so.json"))
		CheckSnapshotExclude(t, ctx, snapshotSingleton, "librecovery", "librecovery.so", sharedDir, sharedVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(sharedDir, "librecovery.so.json"))
		CheckSnapshotExclude(t, ctx, snapshotSingleton, "libavailable_exclude", "libavailable_exclude.so", sharedDir, sharedVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(sharedDir, "libavailable_exclude.so.json"))
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
	cc_library_shared {
		name: "librecovery",
		recovery: true,
		nocrt: true,
	}

	cc_library_shared {
		name: "librecovery_available",
		recovery_available: true,
		nocrt: true,
	}

	genrule {
		name: "libfoo_gen",
		cmd: "",
		out: ["libfoo.so"],
	}

	cc_prebuilt_library_shared {
		name: "libfoo",
		recovery: true,
		prefer: true,
		srcs: [":libfoo_gen"],
	}

	cc_library_shared {
		name: "libfoo",
		recovery: true,
		nocrt: true,
	}
`
	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.RecoverySnapshotVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("29")
	config.TestProductVariables.DirectedRecoverySnapshot = true
	config.TestProductVariables.RecoverySnapshotModules = make(map[string]bool)
	config.TestProductVariables.RecoverySnapshotModules["librecovery"] = true
	config.TestProductVariables.RecoverySnapshotModules["libfoo"] = true
	ctx := testCcWithConfig(t, config)

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
		sharedDir := filepath.Join(snapshotVariantPath, archDir, "shared")

		// Included modules
		CheckSnapshot(t, ctx, snapshotSingleton, "librecovery", "librecovery.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "librecovery.so.json"))
		// Check that snapshot captures "prefer: true" prebuilt
		CheckSnapshot(t, ctx, snapshotSingleton, "prebuilt_libfoo", "libfoo.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "libfoo.so.json"))

		// Excluded modules. Modules not included in the directed recovery snapshot
		// are still include as fake modules.
		CheckSnapshotRule(t, ctx, snapshotSingleton, "librecovery_available", "librecovery_available.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "librecovery_available.so.json"))
	}

	// Verify that each json file for an included module has a rule.
	for _, jsonFile := range includeJsonFiles {
		if snapshotSingleton.MaybeOutput(jsonFile).Rule == nil {
			t.Errorf("include json file %q not found", jsonFile)
		}
	}
}

func TestSnapshotInRelativeInstallPath(t *testing.T) {
	bp := `
	cc_library {
		name: "libvendor_available",
		vendor_available: true,
		nocrt: true,
	}

	cc_library {
		name: "libvendor_available_var",
		vendor_available: true,
		stem: "libvendor_available",
		relative_install_path: "var",
		nocrt: true,
	}
`

	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("29")
	ctx := testCcWithConfig(t, config)

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
		sharedDirVar := filepath.Join(sharedDir, "var")
		CheckSnapshot(t, ctx, snapshotSingleton, "libvendor_available", "libvendor_available.so", sharedDir, sharedVariant)
		CheckSnapshot(t, ctx, snapshotSingleton, "libvendor_available_var", "libvendor_available.so", sharedDirVar, sharedVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(sharedDir, "libvendor_available.so.json"),
			filepath.Join(sharedDirVar, "libvendor_available.so.json"))
	}

	for _, jsonFile := range jsonFiles {
		// verify all json files exist
		if snapshotSingleton.MaybeOutput(jsonFile).Rule == nil {
			t.Errorf("%q expected but not found", jsonFile)
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
