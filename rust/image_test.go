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
	"testing"

	"android/soong/android"
	"android/soong/cc"
)

// Test that cc_binaries can link against rust_ffi_static libraries.
func TestVendorLinkage(t *testing.T) {
	ctx := testRust(t, `
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

	vendorBinary := ctx.ModuleForTests("fizz_vendor", "android_arm64_armv8-a").Module().(*cc.Module)

	if !android.InList("libfoo_vendor", vendorBinary.Properties.AndroidMkStaticLibs) {
		t.Errorf("vendorBinary should have a dependency on libfoo_vendor")
	}
}

// Test that shared libraries cannot be made vendor available until proper support is added.
func TestForbiddenVendorLinkage(t *testing.T) {
	testRustError(t, "can only be set for rust_ffi_static modules", `
		rust_ffi_shared {
			name: "libfoo_vendor",
			crate_name: "foo",
			srcs: ["foo.rs"],
			vendor_available: true,
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
}
