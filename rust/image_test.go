// Copyright 2020 The Android Open Source Project
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
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/cc"
)

// Test that cc modules can link against vendor_available rust_ffi_rlib/rust_ffi_static libraries.
func TestVendorLinkage(t *testing.T) {
	ctx := testRust(t, `
			cc_binary {
				name: "fizz_vendor_available",
				static_libs: ["libfoo_vendor_static"],
				static_rlibs: ["libfoo_vendor"],
				vendor_available: true,
			}
			cc_binary {
				name: "fizz_soc_specific",
				static_rlibs: ["libfoo_vendor"],
				soc_specific: true,
			}
			rust_ffi_rlib {
				name: "libfoo_vendor",
				crate_name: "foo",
				srcs: ["foo.rs"],
				vendor_available: true,
			}
			rust_ffi_static {
				name: "libfoo_vendor_static",
				crate_name: "foo",
				srcs: ["foo.rs"],
				vendor_available: true,
			}
		`)

	vendorBinary := ctx.ModuleForTests("fizz_vendor_available", "android_vendor_arm64_armv8-a").Module().(*cc.Module)

	if !android.InList("libfoo_vendor_static.vendor", vendorBinary.Properties.AndroidMkStaticLibs) {
		t.Errorf("vendorBinary should have a dependency on libfoo_vendor_static.vendor: %#v", vendorBinary.Properties.AndroidMkStaticLibs)
	}
}

// Test that variants which use the vndk emit the appropriate cfg flag.
func TestImageCfgFlag(t *testing.T) {
	ctx := testRust(t, `
			rust_ffi_shared {
				name: "libfoo",
				crate_name: "foo",
				srcs: ["foo.rs"],
				vendor_available: true,
				product_available: true,
			}
		`)

	vendor := ctx.ModuleForTests("libfoo", "android_vendor_arm64_armv8-a_shared").Rule("rustc")

	if !strings.Contains(vendor.Args["rustcFlags"], "--cfg 'android_vndk'") {
		t.Errorf("missing \"--cfg 'android_vndk'\" for libfoo vendor variant, rustcFlags: %#v", vendor.Args["rustcFlags"])
	}
	if !strings.Contains(vendor.Args["rustcFlags"], "--cfg 'android_vendor'") {
		t.Errorf("missing \"--cfg 'android_vendor'\" for libfoo vendor variant, rustcFlags: %#v", vendor.Args["rustcFlags"])
	}
	if strings.Contains(vendor.Args["rustcFlags"], "--cfg 'android_product'") {
		t.Errorf("unexpected \"--cfg 'android_product'\" for libfoo vendor variant, rustcFlags: %#v", vendor.Args["rustcFlags"])
	}

	product := ctx.ModuleForTests("libfoo", "android_product_arm64_armv8-a_shared").Rule("rustc")
	if !strings.Contains(product.Args["rustcFlags"], "--cfg 'android_vndk'") {
		t.Errorf("missing \"--cfg 'android_vndk'\" for libfoo product variant, rustcFlags: %#v", product.Args["rustcFlags"])
	}
	if strings.Contains(product.Args["rustcFlags"], "--cfg 'android_vendor'") {
		t.Errorf("unexpected \"--cfg 'android_vendor'\" for libfoo product variant, rustcFlags: %#v", product.Args["rustcFlags"])
	}
	if !strings.Contains(product.Args["rustcFlags"], "--cfg 'android_product'") {
		t.Errorf("missing \"--cfg 'android_product'\" for libfoo product variant, rustcFlags: %#v", product.Args["rustcFlags"])
	}

	system := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Rule("rustc")
	if strings.Contains(system.Args["rustcFlags"], "--cfg 'android_vndk'") {
		t.Errorf("unexpected \"--cfg 'android_vndk'\" for libfoo system variant, rustcFlags: %#v", system.Args["rustcFlags"])
	}
	if strings.Contains(system.Args["rustcFlags"], "--cfg 'android_vendor'") {
		t.Errorf("unexpected \"--cfg 'android_vendor'\" for libfoo system variant, rustcFlags: %#v", system.Args["rustcFlags"])
	}
	if strings.Contains(system.Args["rustcFlags"], "--cfg 'android_product'") {
		t.Errorf("unexpected \"--cfg 'android_product'\" for libfoo system variant, rustcFlags: %#v", product.Args["rustcFlags"])
	}

}

// Test that cc modules can link against vendor_ramdisk_available rust_ffi_rlib and rust_ffi_static libraries.
func TestVendorRamdiskLinkage(t *testing.T) {
	ctx := testRust(t, `
			cc_library_shared {
				name: "libcc_vendor_ramdisk",
				static_rlibs: ["libfoo_vendor_ramdisk"],
				static_libs: ["libfoo_static_vendor_ramdisk"],
				system_shared_libs: [],
				vendor_ramdisk_available: true,
			}
			rust_ffi_rlib {
				name: "libfoo_vendor_ramdisk",
				crate_name: "foo",
				srcs: ["foo.rs"],
				vendor_ramdisk_available: true,
			}
			rust_ffi_static {
				name: "libfoo_static_vendor_ramdisk",
				crate_name: "foo",
				srcs: ["foo.rs"],
				vendor_ramdisk_available: true,
			}
		`)

	vendorRamdiskLibrary := ctx.ModuleForTests("libcc_vendor_ramdisk", "android_vendor_ramdisk_arm64_armv8-a_shared").Module().(*cc.Module)

	if !android.InList("libfoo_static_vendor_ramdisk.vendor_ramdisk", vendorRamdiskLibrary.Properties.AndroidMkStaticLibs) {
		t.Errorf("libcc_vendor_ramdisk should have a dependency on libfoo_static_vendor_ramdisk")
	}
}

// Test that prebuilt libraries cannot be made vendor available.
func TestForbiddenVendorLinkage(t *testing.T) {
	testRustError(t, "Rust prebuilt modules not supported for non-system images.", `
		rust_prebuilt_library {
			name: "librust_prebuilt",
			crate_name: "rust_prebuilt",
			rlib: {
				srcs: ["libtest.rlib"],
			},
			dylib: {
				srcs: ["libtest.so"],
			},
			vendor: true,
		}
       `)
}

func checkInstallPartition(t *testing.T, ctx *android.TestContext, name, variant, expected string) {
	mod := ctx.ModuleForTests(name, variant).Module().(*Module)
	partitionDefined := false
	checkPartition := func(specific bool, partition string) {
		if specific {
			if expected != partition && !partitionDefined {
				// The variant is installed to the 'partition'
				t.Errorf("%s variant of %q must not be installed to %s partition", variant, name, partition)
			}
			partitionDefined = true
		} else {
			// The variant is not installed to the 'partition'
			if expected == partition {
				t.Errorf("%s variant of %q must be installed to %s partition", variant, name, partition)
			}
		}
	}
	socSpecific := func(m *Module) bool {
		return m.SocSpecific()
	}
	deviceSpecific := func(m *Module) bool {
		return m.DeviceSpecific()
	}
	productSpecific := func(m *Module) bool {
		return m.ProductSpecific() || m.productSpecificModuleContext()
	}
	systemExtSpecific := func(m *Module) bool {
		return m.SystemExtSpecific()
	}
	checkPartition(socSpecific(mod), "vendor")
	checkPartition(deviceSpecific(mod), "odm")
	checkPartition(productSpecific(mod), "product")
	checkPartition(systemExtSpecific(mod), "system_ext")
	if !partitionDefined && expected != "system" {
		t.Errorf("%s variant of %q is expected to be installed to %s partition,"+
			" but installed to system partition", variant, name, expected)
	}
}

func TestInstallPartition(t *testing.T) {
	t.Parallel()
	t.Helper()
	ctx := testRust(t, `
		rust_binary {
			name: "sample_system",
			crate_name: "sample",
			srcs: ["foo.rs"],
		}
		rust_binary {
			name: "sample_system_ext",
			crate_name: "sample",
			srcs: ["foo.rs"],
			system_ext_specific: true,
		}
		rust_binary {
			name: "sample_product",
			crate_name: "sample",
			srcs: ["foo.rs"],
			product_specific: true,
		}
		rust_binary {
			name: "sample_vendor",
			crate_name: "sample",
			srcs: ["foo.rs"],
			vendor: true,
		}
		rust_binary {
			name: "sample_odm",
			crate_name: "sample",
			srcs: ["foo.rs"],
			device_specific: true,
		}
		rust_binary {
			name: "sample_all_available",
			crate_name: "sample",
			srcs: ["foo.rs"],
			vendor_available: true,
			product_available: true,
		}
	`)

	checkInstallPartition(t, ctx, "sample_system", binaryCoreVariant, "system")
	checkInstallPartition(t, ctx, "sample_system_ext", binaryCoreVariant, "system_ext")
	checkInstallPartition(t, ctx, "sample_product", binaryProductVariant, "product")
	checkInstallPartition(t, ctx, "sample_vendor", binaryVendorVariant, "vendor")
	checkInstallPartition(t, ctx, "sample_odm", binaryVendorVariant, "odm")

	checkInstallPartition(t, ctx, "sample_all_available", binaryCoreVariant, "system")
}
