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

	// Test both variants are being built.
	libfooRlib := ctx.ModuleForTests("libfoo", "linux_glibc_x86_64_rlib").Output("libfoo.rlib")
	libfooDylib := ctx.ModuleForTests("libfoo", "linux_glibc_x86_64_dylib").Output("libfoo.so")

	// Test crate type for rlib is correct.
	if !strings.Contains(libfooRlib.Args["rustcFlags"], "crate-type=rlib") {
		t.Errorf("missing crate-type for libfoo rlib, rustcFlags: %#v", libfooRlib.Args["rustcFlags"])
	}

	// Test crate type for dylib is correct.
	if !strings.Contains(libfooDylib.Args["rustcFlags"], "crate-type=dylib") {
		t.Errorf("missing crate-type for libfoo dylib, rustcFlags: %#v", libfooDylib.Args["rustcFlags"])
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

	libfooDylib := ctx.ModuleForTests("libfoo", "linux_glibc_x86_64_dylib").Output("libfoo.so")

	if !strings.Contains(libfooDylib.Args["rustcFlags"], "prefer-dynamic") {
		t.Errorf("missing prefer-dynamic flag for libfoo dylib, rustcFlags: %#v", libfooDylib.Args["rustcFlags"])
	}
}
