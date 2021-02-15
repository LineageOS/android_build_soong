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

// Test that cc modules can link against vendor_available rust_ffi_static libraries.
func TestVendorLinkage(t *testing.T) {
	ctx := testRustVndk(t, `
			cc_binary {
				name: "fizz_vendor",
				static_libs: ["libfoo_vendor"],
				soc_specific: true,
			}
			rust_ffi_static {
				name: "libfoo_vendor",
				crate_name: "foo",
				srcs: ["foo.rs"],
				vendor_available: true,
			}
		`)

	vendorBinary := ctx.ModuleForTests("fizz_vendor", "android_vendor.VER_arm64_armv8-a").Module().(*cc.Module)

	if !android.InList("libfoo_vendor", vendorBinary.Properties.AndroidMkStaticLibs) {
		t.Errorf("vendorBinary should have a dependency on libfoo_vendor")
	}
}

// Test that variants which use the vndk emit the appropriate cfg flag.
func TestImageVndkCfgFlag(t *testing.T) {
	ctx := testRustVndk(t, `
			rust_ffi_static {
				name: "libfoo",
				crate_name: "foo",
				srcs: ["foo.rs"],
				vendor_available: true,
			}
		`)

	vendor := ctx.ModuleForTests("libfoo", "android_vendor.VER_arm64_armv8-a_static").Rule("rustc")

	if !strings.Contains(vendor.Args["rustcFlags"], "--cfg 'android_vndk'") {
		t.Errorf("missing \"--cfg 'android_vndk'\" for libfoo vendor variant, rustcFlags: %#v", vendor.Args["rustcFlags"])
	}
}

// Test that cc modules can link against vendor_ramdisk_available rust_ffi_static libraries.
func TestVendorRamdiskLinkage(t *testing.T) {
	ctx := testRustVndk(t, `
			cc_library_static {
				name: "libcc_vendor_ramdisk",
				static_libs: ["libfoo_vendor_ramdisk"],
				system_shared_libs: [],
				vendor_ramdisk_available: true,
			}
			rust_ffi_static {
				name: "libfoo_vendor_ramdisk",
				crate_name: "foo",
				srcs: ["foo.rs"],
				vendor_ramdisk_available: true,
			}
		`)

	vendorRamdiskLibrary := ctx.ModuleForTests("libcc_vendor_ramdisk", "android_vendor_ramdisk_arm64_armv8-a_static").Module().(*cc.Module)

	if !android.InList("libfoo_vendor_ramdisk.vendor_ramdisk", vendorRamdiskLibrary.Properties.AndroidMkStaticLibs) {
		t.Errorf("libcc_vendor_ramdisk should have a dependency on libfoo_vendor_ramdisk")
	}
}

// Test that shared libraries cannot be made vendor available until proper support is added.
func TestForbiddenVendorLinkage(t *testing.T) {
	testRustError(t, "cannot be set for rust_ffi or rust_ffi_shared modules.", `
		rust_ffi_shared {
			name: "libfoo_vendor",
			crate_name: "foo",
			srcs: ["foo.rs"],
			vendor_available: true,
		}
	`)
	testRustError(t, "cannot be set for rust_ffi or rust_ffi_shared modules.", `
		rust_ffi_shared {
			name: "libfoo_vendor",
			crate_name: "foo",
			srcs: ["foo.rs"],
			vendor_ramdisk_available: true,
		}
	`)
	testRustError(t, "Rust vendor specific modules are currently only supported for rust_ffi_static modules.", `
		rust_ffi {
			name: "libfoo_vendor",
			crate_name: "foo",
			srcs: ["foo.rs"],
			vendor: true,
		}
	`)
	testRustError(t, "Rust vendor specific modules are currently only supported for rust_ffi_static modules.", `
		rust_library {
			name: "libfoo_vendor",
			crate_name: "foo",
			srcs: ["foo.rs"],
			vendor: true,
		}
	`)
	testRustError(t, "Rust vendor specific modules are currently only supported for rust_ffi_static modules.", `
		rust_binary {
			name: "foo_vendor",
			crate_name: "foo",
			srcs: ["foo.rs"],
			vendor: true,
		}
	`)

}
