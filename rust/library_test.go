// Copyright 2019 The Android Open Source Project
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
)

// Test that variants are being generated correctly, and that crate-types are correct.
func TestLibraryVariants(t *testing.T) {

	ctx := testRust(t, `
		rust_library_host {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
		}`)

	// Test all variants are being built.
	libfooRlib := ctx.ModuleForTests("libfoo", "linux_glibc_x86_64_rlib").Output("libfoo.rlib")
	libfooDylib := ctx.ModuleForTests("libfoo", "linux_glibc_x86_64_dylib").Output("libfoo.dylib.so")
	libfooStatic := ctx.ModuleForTests("libfoo", "linux_glibc_x86_64_static").Output("libfoo.a")
	libfooShared := ctx.ModuleForTests("libfoo", "linux_glibc_x86_64_shared").Output("libfoo.so")

	rlibCrateType := "rlib"
	dylibCrateType := "dylib"
	sharedCrateType := "cdylib"
	staticCrateType := "static"

	// Test crate type for rlib is correct.
	if !strings.Contains(libfooRlib.Args["rustcFlags"], "crate-type="+rlibCrateType) {
		t.Errorf("missing crate-type for static variant, expecting %#v, rustcFlags: %#v", rlibCrateType, libfooRlib.Args["rustcFlags"])
	}

	// Test crate type for dylib is correct.
	if !strings.Contains(libfooDylib.Args["rustcFlags"], "crate-type="+dylibCrateType) {
		t.Errorf("missing crate-type for static variant, expecting %#v, rustcFlags: %#v", dylibCrateType, libfooDylib.Args["rustcFlags"])
	}

	// Test crate type for C static libraries is correct.
	if !strings.Contains(libfooStatic.Args["rustcFlags"], "crate-type="+staticCrateType) {
		t.Errorf("missing crate-type for static variant, expecting %#v, rustcFlags: %#v", staticCrateType, libfooStatic.Args["rustcFlags"])
	}

	// Test crate type for C shared libraries is correct.
	if !strings.Contains(libfooShared.Args["rustcFlags"], "crate-type="+sharedCrateType) {
		t.Errorf("missing crate-type for shared variant, expecting %#v, got rustcFlags: %#v", sharedCrateType, libfooShared.Args["rustcFlags"])
	}

}

// Test that dylibs are not statically linking the standard library.
func TestDylibPreferDynamic(t *testing.T) {
	ctx := testRust(t, `
		rust_library_host_dylib {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
		}`)

	libfooDylib := ctx.ModuleForTests("libfoo", "linux_glibc_x86_64_dylib").Output("libfoo.dylib.so")

	if !strings.Contains(libfooDylib.Args["rustcFlags"], "prefer-dynamic") {
		t.Errorf("missing prefer-dynamic flag for libfoo dylib, rustcFlags: %#v", libfooDylib.Args["rustcFlags"])
	}
}

func TestValidateLibraryStem(t *testing.T) {
	testRustError(t, "crate_name must be defined.", `
			rust_library_host {
				name: "libfoo",
				srcs: ["foo.rs"],
			}`)

	testRustError(t, "library crate_names must be alphanumeric with underscores allowed", `
			rust_library_host {
				name: "libfoo-bar",
				srcs: ["foo.rs"],
				crate_name: "foo-bar"
			}`)

	testRustError(t, "Invalid name or stem property; library filenames must start with lib<crate_name>", `
			rust_library_host {
				name: "foobar",
				srcs: ["foo.rs"],
				crate_name: "foo_bar"
			}`)
	testRustError(t, "Invalid name or stem property; library filenames must start with lib<crate_name>", `
			rust_library_host {
				name: "foobar",
				stem: "libfoo",
				srcs: ["foo.rs"],
				crate_name: "foo_bar"
			}`)
	testRustError(t, "Invalid name or stem property; library filenames must start with lib<crate_name>", `
			rust_library_host {
				name: "foobar",
				stem: "foo_bar",
				srcs: ["foo.rs"],
				crate_name: "foo_bar"
			}`)

}
