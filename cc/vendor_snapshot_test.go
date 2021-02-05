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
		name: "libvendor_available",
		vendor_available: true,
		nocrt: true,
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

	toolchain_library {
		name: "libb",
		vendor_available: true,
		src: "libb.a",
	}

	cc_object {
		name: "obj",
		vendor_available: true,
	}

	cc_library {
		name: "libllndk",
		llndk_stubs: "libllndk.llndk",
	}

	llndk_library {
		name: "libllndk.llndk",
		symbol_file: "",
	}
`
	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	ctx := testCcWithConfig(t, config)

	// Check Vendor snapshot output.

	snapshotDir := "vendor-snapshot"
	snapshotVariantPath := filepath.Join(buildDir, snapshotDir, "arm64")
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
		sharedVariant := fmt.Sprintf("android_vendor.VER_%s_%s_shared", archType, archVariant)
		sharedDir := filepath.Join(snapshotVariantPath, archDir, "shared")
		checkSnapshot(t, ctx, snapshotSingleton, "libvendor", "libvendor.so", sharedDir, sharedVariant)
		checkSnapshot(t, ctx, snapshotSingleton, "libvendor_available", "libvendor_available.so", sharedDir, sharedVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(sharedDir, "libvendor.so.json"),
			filepath.Join(sharedDir, "libvendor_available.so.json"))

		// LLNDK modules are not captured
		checkSnapshotExclude(t, ctx, snapshotSingleton, "libllndk", "libllndk.so", sharedDir, sharedVariant)

		// For static libraries, all vendor:true and vendor_available modules (including VNDK) are captured.
		// Also cfi variants are captured, except for prebuilts like toolchain_library
		staticVariant := fmt.Sprintf("android_vendor.VER_%s_%s_static", archType, archVariant)
		staticCfiVariant := fmt.Sprintf("android_vendor.VER_%s_%s_static_cfi", archType, archVariant)
		staticDir := filepath.Join(snapshotVariantPath, archDir, "static")
		checkSnapshot(t, ctx, snapshotSingleton, "libb", "libb.a", staticDir, staticVariant)
		checkSnapshot(t, ctx, snapshotSingleton, "libvndk", "libvndk.a", staticDir, staticVariant)
		checkSnapshot(t, ctx, snapshotSingleton, "libvndk", "libvndk.cfi.a", staticDir, staticCfiVariant)
		checkSnapshot(t, ctx, snapshotSingleton, "libvendor", "libvendor.a", staticDir, staticVariant)
		checkSnapshot(t, ctx, snapshotSingleton, "libvendor", "libvendor.cfi.a", staticDir, staticCfiVariant)
		checkSnapshot(t, ctx, snapshotSingleton, "libvendor_available", "libvendor_available.a", staticDir, staticVariant)
		checkSnapshot(t, ctx, snapshotSingleton, "libvendor_available", "libvendor_available.cfi.a", staticDir, staticCfiVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(staticDir, "libb.a.json"),
			filepath.Join(staticDir, "libvndk.a.json"),
			filepath.Join(staticDir, "libvndk.cfi.a.json"),
			filepath.Join(staticDir, "libvendor.a.json"),
			filepath.Join(staticDir, "libvendor.cfi.a.json"),
			filepath.Join(staticDir, "libvendor_available.a.json"),
			filepath.Join(staticDir, "libvendor_available.cfi.a.json"))

		// For binary executables, all vendor:true and vendor_available modules are captured.
		if archType == "arm64" {
			binaryVariant := fmt.Sprintf("android_vendor.VER_%s_%s", archType, archVariant)
			binaryDir := filepath.Join(snapshotVariantPath, archDir, "binary")
			checkSnapshot(t, ctx, snapshotSingleton, "vendor_bin", "vendor_bin", binaryDir, binaryVariant)
			checkSnapshot(t, ctx, snapshotSingleton, "vendor_available_bin", "vendor_available_bin", binaryDir, binaryVariant)
			jsonFiles = append(jsonFiles,
				filepath.Join(binaryDir, "vendor_bin.json"),
				filepath.Join(binaryDir, "vendor_available_bin.json"))
		}

		// For header libraries, all vendor:true and vendor_available modules are captured.
		headerDir := filepath.Join(snapshotVariantPath, archDir, "header")
		jsonFiles = append(jsonFiles, filepath.Join(headerDir, "libvendor_headers.json"))

		// For object modules, all vendor:true and vendor_available modules are captured.
		objectVariant := fmt.Sprintf("android_vendor.VER_%s_%s", archType, archVariant)
		objectDir := filepath.Join(snapshotVariantPath, archDir, "object")
		checkSnapshot(t, ctx, snapshotSingleton, "obj", "obj.o", objectDir, objectVariant)
		jsonFiles = append(jsonFiles, filepath.Join(objectDir, "obj.o.json"))
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
	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	config.TestProductVariables.DirectedVendorSnapshot = true
	config.TestProductVariables.VendorSnapshotModules = make(map[string]bool)
	config.TestProductVariables.VendorSnapshotModules["libvendor"] = true
	config.TestProductVariables.VendorSnapshotModules["libfoo"] = true
	ctx := testCcWithConfig(t, config)

	// Check Vendor snapshot output.

	snapshotDir := "vendor-snapshot"
	snapshotVariantPath := filepath.Join(buildDir, snapshotDir, "arm64")
	snapshotSingleton := ctx.SingletonForTests("vendor-snapshot")

	var includeJsonFiles []string

	for _, arch := range [][]string{
		[]string{"arm64", "armv8-a"},
		[]string{"arm", "armv7-a-neon"},
	} {
		archType := arch[0]
		archVariant := arch[1]
		archDir := fmt.Sprintf("arch-%s-%s", archType, archVariant)

		sharedVariant := fmt.Sprintf("android_vendor.VER_%s_%s_shared", archType, archVariant)
		sharedDir := filepath.Join(snapshotVariantPath, archDir, "shared")

		// Included modules
		checkSnapshot(t, ctx, snapshotSingleton, "libvendor", "libvendor.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "libvendor.so.json"))
		// Check that snapshot captures "prefer: true" prebuilt
		checkSnapshot(t, ctx, snapshotSingleton, "prebuilt_libfoo", "libfoo.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "libfoo.so.json"))

		// Excluded modules. Modules not included in the directed vendor snapshot
		// are still include as fake modules.
		checkSnapshotRule(t, ctx, snapshotSingleton, "libvendor_available", "libvendor_available.so", sharedDir, sharedVariant)
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
		compile_multilib: "64",
	}

	cc_library {
		name: "libvendor",
		vendor: true,
		nocrt: true,
		no_libcrt: true,
		stl: "none",
		system_shared_libs: [],
		compile_multilib: "64",
	}

	cc_library {
		name: "libvendor_available",
		vendor_available: true,
		nocrt: true,
		no_libcrt: true,
		stl: "none",
		system_shared_libs: [],
		compile_multilib: "64",
	}

	cc_binary {
		name: "bin",
		vendor: true,
		nocrt: true,
		no_libcrt: true,
		stl: "none",
		system_shared_libs: [],
		compile_multilib: "64",
	}
`

	vndkBp := `
	vndk_prebuilt_shared {
		name: "libvndk",
		version: "BOARD",
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
		},
	}

	// old snapshot module which has to be ignored
	vndk_prebuilt_shared {
		name: "libvndk",
		version: "OLD",
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
		compile_multilib: "64",
	}

	cc_library_shared {
		name: "libclient",
		vendor: true,
		nocrt: true,
		no_libcrt: true,
		stl: "none",
		system_shared_libs: [],
		shared_libs: ["libvndk", "libvendor_available"],
		static_libs: ["libvendor", "libvendor_without_snapshot"],
		compile_multilib: "64",
		srcs: ["client.cpp"],
	}

	cc_binary {
		name: "bin_without_snapshot",
		vendor: true,
		nocrt: true,
		no_libcrt: true,
		stl: "none",
		system_shared_libs: [],
		static_libs: ["libvndk"],
		compile_multilib: "64",
		srcs: ["bin.cpp"],
	}

	vendor_snapshot {
		name: "vendor_snapshot",
		compile_multilib: "first",
		version: "BOARD",
		vndk_libs: [
			"libvndk",
		],
		static_libs: [
			"libvendor",
			"libvendor_available",
			"libvndk",
		],
		shared_libs: [
			"libvendor",
			"libvendor_available",
		],
		binaries: [
			"bin",
		],
	}

	vendor_snapshot_static {
		name: "libvndk",
		version: "BOARD",
		target_arch: "arm64",
		vendor: true,
		arch: {
			arm64: {
				src: "libvndk.a",
				export_include_dirs: ["include/libvndk"],
			},
		},
	}

	vendor_snapshot_shared {
		name: "libvendor",
		version: "BOARD",
		target_arch: "arm64",
		vendor: true,
		arch: {
			arm64: {
				src: "libvendor.so",
				export_include_dirs: ["include/libvendor"],
			},
		},
	}

	vendor_snapshot_static {
		name: "libvendor",
		version: "BOARD",
		target_arch: "arm64",
		vendor: true,
		arch: {
			arm64: {
				src: "libvendor.a",
				export_include_dirs: ["include/libvendor"],
			},
		},
	}

	vendor_snapshot_shared {
		name: "libvendor_available",
		androidmk_suffix: ".vendor",
		version: "BOARD",
		target_arch: "arm64",
		vendor: true,
		arch: {
			arm64: {
				src: "libvendor_available.so",
				export_include_dirs: ["include/libvendor"],
			},
		},
	}

	vendor_snapshot_static {
		name: "libvendor_available",
		androidmk_suffix: ".vendor",
		version: "BOARD",
		target_arch: "arm64",
		vendor: true,
		arch: {
			arm64: {
				src: "libvendor_available.a",
				export_include_dirs: ["include/libvendor"],
			},
		},
	}

	vendor_snapshot_binary {
		name: "bin",
		version: "BOARD",
		target_arch: "arm64",
		vendor: true,
		arch: {
			arm64: {
				src: "bin",
			},
		},
	}

	// old snapshot module which has to be ignored
	vendor_snapshot_binary {
		name: "bin",
		version: "OLD",
		target_arch: "arm64",
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
		"deps/Android.bp":              []byte(depsBp),
		"framework/Android.bp":         []byte(frameworkBp),
		"vendor/Android.bp":            []byte(vendorProprietaryBp),
		"vendor/bin":                   nil,
		"vendor/bin.cpp":               nil,
		"vendor/client.cpp":            nil,
		"vendor/include/libvndk/a.h":   nil,
		"vendor/include/libvendor/b.h": nil,
		"vendor/libvndk.a":             nil,
		"vendor/libvendor.a":           nil,
		"vendor/libvendor.so":          nil,
		"vndk/Android.bp":              []byte(vndkBp),
		"vndk/include/libvndk/a.h":     nil,
		"vndk/libvndk.so":              nil,
	}

	config := TestConfig(buildDir, android.Android, nil, "", mockFS)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("BOARD")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	ctx := CreateTestContext(config)
	ctx.Register()

	_, errs := ctx.ParseFileList(".", []string{"deps/Android.bp", "framework/Android.bp", "vendor/Android.bp", "vndk/Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	sharedVariant := "android_vendor.BOARD_arm64_armv8-a_shared"
	staticVariant := "android_vendor.BOARD_arm64_armv8-a_static"
	binaryVariant := "android_vendor.BOARD_arm64_armv8-a"

	// libclient uses libvndk.vndk.BOARD.arm64, libvendor.vendor_static.BOARD.arm64, libvendor_without_snapshot
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
		[]string{sharedVariant, "libvndk.vndk.BOARD.arm64"},
		[]string{staticVariant, "libvendor.vendor_static.BOARD.arm64"},
		[]string{staticVariant, "libvendor_without_snapshot"},
	} {
		outputPaths := getOutputPaths(ctx, input[0] /* variant */, []string{input[1]} /* module name */)
		if !strings.Contains(libclientLdFlags, outputPaths[0].String()) {
			t.Errorf("libflags for libclient must contain %#v, but was %#v", outputPaths[0], libclientLdFlags)
		}
	}

	libclientAndroidMkSharedLibs := ctx.ModuleForTests("libclient", sharedVariant).Module().(*Module).Properties.AndroidMkSharedLibs
	if g, w := libclientAndroidMkSharedLibs, []string{"libvndk.vendor", "libvendor_available.vendor"}; !reflect.DeepEqual(g, w) {
		t.Errorf("wanted libclient AndroidMkSharedLibs %q, got %q", w, g)
	}

	libclientAndroidMkStaticLibs := ctx.ModuleForTests("libclient", sharedVariant).Module().(*Module).Properties.AndroidMkStaticLibs
	if g, w := libclientAndroidMkStaticLibs, []string{"libvendor", "libvendor_without_snapshot"}; !reflect.DeepEqual(g, w) {
		t.Errorf("wanted libclient AndroidMkStaticLibs %q, got %q", w, g)
	}

	// bin_without_snapshot uses libvndk.vendor_static.BOARD.arm64
	binWithoutSnapshotCcFlags := ctx.ModuleForTests("bin_without_snapshot", binaryVariant).Rule("cc").Args["cFlags"]
	if !strings.Contains(binWithoutSnapshotCcFlags, "-Ivendor/include/libvndk") {
		t.Errorf("flags for bin_without_snapshot must contain %#v, but was %#v.",
			"-Ivendor/include/libvndk", binWithoutSnapshotCcFlags)
	}

	binWithoutSnapshotLdFlags := ctx.ModuleForTests("bin_without_snapshot", binaryVariant).Rule("ld").Args["libFlags"]
	libVndkStaticOutputPaths := getOutputPaths(ctx, staticVariant, []string{"libvndk.vendor_static.BOARD.arm64"})
	if !strings.Contains(binWithoutSnapshotLdFlags, libVndkStaticOutputPaths[0].String()) {
		t.Errorf("libflags for bin_without_snapshot must contain %#v, but was %#v",
			libVndkStaticOutputPaths[0], binWithoutSnapshotLdFlags)
	}

	// libvendor.so is installed by libvendor.vendor_shared.BOARD.arm64
	ctx.ModuleForTests("libvendor.vendor_shared.BOARD.arm64", sharedVariant).Output("libvendor.so")

	// libvendor_available.so is installed by libvendor_available.vendor_shared.BOARD.arm64
	ctx.ModuleForTests("libvendor_available.vendor_shared.BOARD.arm64", sharedVariant).Output("libvendor_available.so")

	// libvendor_without_snapshot.so is installed by libvendor_without_snapshot
	ctx.ModuleForTests("libvendor_without_snapshot", sharedVariant).Output("libvendor_without_snapshot.so")

	// bin is installed by bin.vendor_binary.BOARD.arm64
	ctx.ModuleForTests("bin.vendor_binary.BOARD.arm64", binaryVariant).Output("bin")

	// bin_without_snapshot is installed by bin_without_snapshot
	ctx.ModuleForTests("bin_without_snapshot", binaryVariant).Output("bin_without_snapshot")

	// libvendor, libvendor_available and bin don't have vendor.BOARD variant
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
}

func TestVendorSnapshotSanitizer(t *testing.T) {
	bp := `
	vendor_snapshot_static {
		name: "libsnapshot",
		vendor: true,
		target_arch: "arm64",
		version: "BOARD",
		arch: {
			arm64: {
				src: "libsnapshot.a",
				cfi: {
					src: "libsnapshot.cfi.a",
				}
			},
		},
	}
`
	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("BOARD")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	ctx := testCcWithConfig(t, config)

	// Check non-cfi and cfi variant.
	staticVariant := "android_vendor.BOARD_arm64_armv8-a_static"
	staticCfiVariant := "android_vendor.BOARD_arm64_armv8-a_static_cfi"

	staticModule := ctx.ModuleForTests("libsnapshot.vendor_static.BOARD.arm64", staticVariant).Module().(*Module)
	assertString(t, staticModule.outputFile.Path().Base(), "libsnapshot.a")

	staticCfiModule := ctx.ModuleForTests("libsnapshot.vendor_static.BOARD.arm64", staticCfiVariant).Module().(*Module)
	assertString(t, staticCfiModule.outputFile.Path().Base(), "libsnapshot.cfi.a")
}

func assertExcludeFromVendorSnapshotIs(t *testing.T, ctx *android.TestContext, name string, expected bool) {
	t.Helper()
	m := ctx.ModuleForTests(name, vendorVariant).Module().(*Module)
	if m.ExcludeFromVendorSnapshot() != expected {
		t.Errorf("expected %q ExcludeFromVendorSnapshot to be %t", m.String(), expected)
	}
}

func assertExcludeFromRecoverySnapshotIs(t *testing.T, ctx *android.TestContext, name string, expected bool) {
	t.Helper()
	m := ctx.ModuleForTests(name, recoveryVariant).Module().(*Module)
	if m.ExcludeFromRecoverySnapshot() != expected {
		t.Errorf("expected %q ExcludeFromRecoverySnapshot to be %t", m.String(), expected)
	}
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

	config := TestConfig(buildDir, android.Android, nil, "", mockFS)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	ctx := CreateTestContext(config)
	ctx.Register()

	_, errs := ctx.ParseFileList(".", []string{"deps/Android.bp", "framework/Android.bp", "device/Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	// Test an include and exclude framework module.
	assertExcludeFromVendorSnapshotIs(t, ctx, "libinclude", false)
	assertExcludeFromVendorSnapshotIs(t, ctx, "libexclude", true)
	assertExcludeFromVendorSnapshotIs(t, ctx, "libavailable_exclude", true)

	// A vendor module is excluded, but by its path, not the
	// exclude_from_vendor_snapshot property.
	assertExcludeFromVendorSnapshotIs(t, ctx, "libvendor", false)

	// Verify the content of the vendor snapshot.

	snapshotDir := "vendor-snapshot"
	snapshotVariantPath := filepath.Join(buildDir, snapshotDir, "arm64")
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

		sharedVariant := fmt.Sprintf("android_vendor.VER_%s_%s_shared", archType, archVariant)
		sharedDir := filepath.Join(snapshotVariantPath, archDir, "shared")

		// Included modules
		checkSnapshot(t, ctx, snapshotSingleton, "libinclude", "libinclude.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "libinclude.so.json"))

		// Excluded modules
		checkSnapshotExclude(t, ctx, snapshotSingleton, "libexclude", "libexclude.so", sharedDir, sharedVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(sharedDir, "libexclude.so.json"))
		checkSnapshotExclude(t, ctx, snapshotSingleton, "libvendor", "libvendor.so", sharedDir, sharedVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(sharedDir, "libvendor.so.json"))
		checkSnapshotExclude(t, ctx, snapshotSingleton, "libavailable_exclude", "libavailable_exclude.so", sharedDir, sharedVariant)
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

	config := TestConfig(buildDir, android.Android, nil, "", mockFS)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
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

	toolchain_library {
		name: "libb",
		recovery_available: true,
		src: "libb.a",
	}

	cc_object {
		name: "obj",
		recovery_available: true,
	}
`
	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.RecoverySnapshotVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	ctx := testCcWithConfig(t, config)

	// Check Recovery snapshot output.

	snapshotDir := "recovery-snapshot"
	snapshotVariantPath := filepath.Join(buildDir, snapshotDir, "arm64")
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
		checkSnapshot(t, ctx, snapshotSingleton, "libvndk", "libvndk.so", sharedDir, sharedVariant)
		checkSnapshot(t, ctx, snapshotSingleton, "librecovery", "librecovery.so", sharedDir, sharedVariant)
		checkSnapshot(t, ctx, snapshotSingleton, "librecovery_available", "librecovery_available.so", sharedDir, sharedVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(sharedDir, "libvndk.so.json"),
			filepath.Join(sharedDir, "librecovery.so.json"),
			filepath.Join(sharedDir, "librecovery_available.so.json"))

		// For static libraries, all recovery:true and recovery_available modules are captured.
		staticVariant := fmt.Sprintf("android_recovery_%s_%s_static", archType, archVariant)
		staticDir := filepath.Join(snapshotVariantPath, archDir, "static")
		checkSnapshot(t, ctx, snapshotSingleton, "libb", "libb.a", staticDir, staticVariant)
		checkSnapshot(t, ctx, snapshotSingleton, "librecovery", "librecovery.a", staticDir, staticVariant)
		checkSnapshot(t, ctx, snapshotSingleton, "librecovery_available", "librecovery_available.a", staticDir, staticVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(staticDir, "libb.a.json"),
			filepath.Join(staticDir, "librecovery.a.json"),
			filepath.Join(staticDir, "librecovery_available.a.json"))

		// For binary executables, all recovery:true and recovery_available modules are captured.
		if archType == "arm64" {
			binaryVariant := fmt.Sprintf("android_recovery_%s_%s", archType, archVariant)
			binaryDir := filepath.Join(snapshotVariantPath, archDir, "binary")
			checkSnapshot(t, ctx, snapshotSingleton, "recovery_bin", "recovery_bin", binaryDir, binaryVariant)
			checkSnapshot(t, ctx, snapshotSingleton, "recovery_available_bin", "recovery_available_bin", binaryDir, binaryVariant)
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
		checkSnapshot(t, ctx, snapshotSingleton, "obj", "obj.o", objectDir, objectVariant)
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

	config := TestConfig(buildDir, android.Android, nil, "", mockFS)
	config.TestProductVariables.RecoverySnapshotVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	ctx := CreateTestContext(config)
	ctx.Register()

	_, errs := ctx.ParseFileList(".", []string{"deps/Android.bp", "framework/Android.bp", "device/Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	// Test an include and exclude framework module.
	assertExcludeFromRecoverySnapshotIs(t, ctx, "libinclude", false)
	assertExcludeFromRecoverySnapshotIs(t, ctx, "libexclude", true)
	assertExcludeFromRecoverySnapshotIs(t, ctx, "libavailable_exclude", true)

	// A recovery module is excluded, but by its path, not the
	// exclude_from_recovery_snapshot property.
	assertExcludeFromRecoverySnapshotIs(t, ctx, "librecovery", false)

	// Verify the content of the recovery snapshot.

	snapshotDir := "recovery-snapshot"
	snapshotVariantPath := filepath.Join(buildDir, snapshotDir, "arm64")
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
		checkSnapshot(t, ctx, snapshotSingleton, "libinclude", "libinclude.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "libinclude.so.json"))

		// Excluded modules
		checkSnapshotExclude(t, ctx, snapshotSingleton, "libexclude", "libexclude.so", sharedDir, sharedVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(sharedDir, "libexclude.so.json"))
		checkSnapshotExclude(t, ctx, snapshotSingleton, "librecovery", "librecovery.so", sharedDir, sharedVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(sharedDir, "librecovery.so.json"))
		checkSnapshotExclude(t, ctx, snapshotSingleton, "libavailable_exclude", "libavailable_exclude.so", sharedDir, sharedVariant)
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
