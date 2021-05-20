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
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/cc"
)

func TestVendorSnapshotCapture(t *testing.T) {
	bp := `
	rust_ffi {
		name: "librustvendor_available",
		crate_name: "rustvendor_available",
		srcs: ["lib.rs"],
		vendor_available: true,
		include_dirs: ["rust_headers/"],
	}

	rust_binary {
		name: "vendor_available_bin",
		vendor_available: true,
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
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librustvendor_available", "librustvendor_available.so", sharedDir, sharedVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(sharedDir, "librustvendor_available.so.json"))

		// For static libraries, all vendor:true and vendor_available modules (including VNDK) are captured.
		staticVariant := fmt.Sprintf("android_vendor.29_%s_%s_static", archType, archVariant)
		staticDir := filepath.Join(snapshotVariantPath, archDir, "static")
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librustvendor_available", "librustvendor_available.a", staticDir, staticVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(staticDir, "librustvendor_available.a.json"))

		// For binary executables, all vendor_available modules are captured.
		if archType == "arm64" {
			binaryVariant := fmt.Sprintf("android_vendor.29_%s_%s", archType, archVariant)
			binaryDir := filepath.Join(snapshotVariantPath, archDir, "binary")
			cc.CheckSnapshot(t, ctx, snapshotSingleton, "vendor_available_bin", "vendor_available_bin", binaryDir, binaryVariant)
			jsonFiles = append(jsonFiles,
				filepath.Join(binaryDir, "vendor_available_bin.json"))
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
		name: "librustvendor_available",
		crate_name: "rustvendor_available",
		srcs: ["lib.rs"],
		vendor_available: true,
	}

	rust_ffi_shared {
		name: "librustvendor_exclude",
		crate_name: "rustvendor_exclude",
		srcs: ["lib.rs"],
		vendor_available: true,
	}
`
	ctx := testRustVndk(t, bp)
	ctx.Config().TestProductVariables.VendorSnapshotModules = make(map[string]bool)
	ctx.Config().TestProductVariables.VendorSnapshotModules["librustvendor_available"] = true
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
		sharedDir := filepath.Join(snapshotVariantPath, archDir, "shared")

		// Included modules
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "librustvendor_available", "librustvendor_available.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "librustvendor_available.so.json"))

		// Excluded modules. Modules not included in the directed vendor snapshot
		// are still include as fake modules.
		cc.CheckSnapshotRule(t, ctx, snapshotSingleton, "librustvendor_exclude", "librustvendor_exclude.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "librustvendor_exclude.so.json"))
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

	// When vendor-specific Rust modules are available, make sure to test
	// that they're excluded by path here. See cc.TestVendorSnapshotExclude
	// for an example.

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
			vendor_available: true,
			exclude_from_vendor_snapshot: true,
		}

		rust_ffi_shared {
			name: "libavailable_exclude",
			crate_name: "available_exclude",
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
	cc.AssertExcludeFromVendorSnapshotIs(t, ctx, "libinclude", false, vendorVariant)
	cc.AssertExcludeFromVendorSnapshotIs(t, ctx, "libexclude", true, vendorVariant)
	cc.AssertExcludeFromVendorSnapshotIs(t, ctx, "libavailable_exclude", true, vendorVariant)

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
		cc.CheckSnapshot(t, ctx, snapshotSingleton, "libinclude", "libinclude.so", sharedDir, sharedVariant)
		includeJsonFiles = append(includeJsonFiles, filepath.Join(sharedDir, "libinclude.so.json"))

		// Excluded modules
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "libexclude", "libexclude.so", sharedDir, sharedVariant)
		excludeJsonFiles = append(excludeJsonFiles, filepath.Join(sharedDir, "libexclude.so.json"))
		cc.CheckSnapshotExclude(t, ctx, snapshotSingleton, "libavailable_exclude", "libavailable_exclude.so", sharedDir, sharedVariant)
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
