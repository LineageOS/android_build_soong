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

	"android/soong/android"
)

// Test that variants are being generated correctly, and that crate-types are correct.
func TestLibraryVariants(t *testing.T) {

	ctx := testRust(t, `
		rust_library_host {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
		}
                rust_ffi_host {
                        name: "libfoo.ffi",
                        srcs: ["foo.rs"],
                        crate_name: "foo"
                }`)

	// Test all variants are being built.
	libfooRlib := ctx.ModuleForTests("libfoo", "linux_glibc_x86_64_rlib_rlib-std").Output("libfoo.rlib")
	libfooDylib := ctx.ModuleForTests("libfoo", "linux_glibc_x86_64_dylib").Output("libfoo.dylib.so")
	libfooStatic := ctx.ModuleForTests("libfoo.ffi", "linux_glibc_x86_64_static").Output("libfoo.ffi.a")
	libfooShared := ctx.ModuleForTests("libfoo.ffi", "linux_glibc_x86_64_shared").Output("libfoo.ffi.so")

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

func TestSharedLibrary(t *testing.T) {
	ctx := testRust(t, `
		rust_ffi_shared {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
		}`)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared")

	libfooOutput := libfoo.Output("libfoo.so")
	if !strings.Contains(libfooOutput.Args["linkFlags"], "-Wl,-soname=libfoo.so") {
		t.Errorf("missing expected -Wl,-soname linker flag for libfoo shared lib, linkFlags: %#v",
			libfooOutput.Args["linkFlags"])
	}

	if !android.InList("libstd", libfoo.Module().(*Module).Properties.AndroidMkDylibs) {
		t.Errorf("Non-static libstd dylib expected to be a dependency of Rust shared libraries. Dylib deps are: %#v",
			libfoo.Module().(*Module).Properties.AndroidMkDylibs)
	}
}

func TestStaticLibraryLinkage(t *testing.T) {
	ctx := testRust(t, `
		rust_ffi_static {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
		}`)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_static")

	if !android.InList("libstd", libfoo.Module().(*Module).Properties.AndroidMkRlibs) {
		t.Errorf("Static libstd rlib expected to be a dependency of Rust static libraries. Rlib deps are: %#v",
			libfoo.Module().(*Module).Properties.AndroidMkDylibs)
	}
}

// Test that variants pull in the right type of rustlib autodep
func TestAutoDeps(t *testing.T) {

	ctx := testRust(t, `
                rust_library_host {
                        name: "libbar",
                        srcs: ["bar.rs"],
                        crate_name: "bar",
                }
		rust_library_host {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
                        rustlibs: ["libbar"],
		}
                rust_ffi_host {
                        name: "libfoo.ffi",
                        srcs: ["foo.rs"],
                        crate_name: "foo",
                        rustlibs: ["libbar"],
                }`)

	libfooRlib := ctx.ModuleForTests("libfoo", "linux_glibc_x86_64_rlib_rlib-std")
	libfooDylib := ctx.ModuleForTests("libfoo", "linux_glibc_x86_64_dylib")
	libfooStatic := ctx.ModuleForTests("libfoo.ffi", "linux_glibc_x86_64_static")
	libfooShared := ctx.ModuleForTests("libfoo.ffi", "linux_glibc_x86_64_shared")

	for _, static := range []android.TestingModule{libfooRlib, libfooStatic} {
		if !android.InList("libbar.rlib-std", static.Module().(*Module).Properties.AndroidMkRlibs) {
			t.Errorf("libbar not present as rlib dependency in static lib")
		}
		if android.InList("libbar", static.Module().(*Module).Properties.AndroidMkDylibs) {
			t.Errorf("libbar present as dynamic dependency in static lib")
		}
	}

	for _, dyn := range []android.TestingModule{libfooDylib, libfooShared} {
		if !android.InList("libbar", dyn.Module().(*Module).Properties.AndroidMkDylibs) {
			t.Errorf("libbar not present as dynamic dependency in dynamic lib")
		}
		if android.InList("libbar.dylib-std", dyn.Module().(*Module).Properties.AndroidMkRlibs) {
			t.Errorf("libbar present as rlib dependency in dynamic lib")
		}

	}
}

// Test that stripped versions are correctly generated and used.
func TestStrippedLibrary(t *testing.T) {
	ctx := testRust(t, `
		rust_library_dylib {
			name: "libfoo",
			crate_name: "foo",
			srcs: ["foo.rs"],
		}
		rust_library_dylib {
			name: "libbar",
			crate_name: "bar",
			srcs: ["foo.rs"],
			strip: {
				none: true
			}
		}
	`)

	foo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_dylib")
	foo.Output("stripped/libfoo.dylib.so")
	// Check that the `cp` rule is using the stripped version as input.
	cp := foo.Rule("android.Cp")
	if !strings.HasSuffix(cp.Input.String(), "stripped/libfoo.dylib.so") {
		t.Errorf("installed binary not based on stripped version: %v", cp.Input)
	}

	fizzBar := ctx.ModuleForTests("libbar", "android_arm64_armv8-a_dylib").MaybeOutput("stripped/libbar.dylib.so")
	if fizzBar.Rule != nil {
		t.Errorf("stripped version of bar has been generated")
	}
}

func TestLibstdLinkage(t *testing.T) {
	ctx := testRust(t, `
		rust_library {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
		}
		rust_ffi {
			name: "libbar",
			srcs: ["foo.rs"],
			crate_name: "bar",
			rustlibs: ["libfoo"],
		}
		rust_ffi {
			name: "libbar.prefer_rlib",
			srcs: ["foo.rs"],
			crate_name: "bar",
			rustlibs: ["libfoo"],
			prefer_rlib: true,
		}`)

	libfooDylib := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_dylib").Module().(*Module)
	libfooRlibStatic := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_rlib_rlib-std").Module().(*Module)
	libfooRlibDynamic := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_rlib_dylib-std").Module().(*Module)

	libbarShared := ctx.ModuleForTests("libbar", "android_arm64_armv8-a_shared").Module().(*Module)
	libbarStatic := ctx.ModuleForTests("libbar", "android_arm64_armv8-a_static").Module().(*Module)

	// prefer_rlib works the same for both rust_library and rust_ffi, so a single check is sufficient here.
	libbarRlibStd := ctx.ModuleForTests("libbar.prefer_rlib", "android_arm64_armv8-a_shared").Module().(*Module)

	if !android.InList("libstd", libfooRlibStatic.Properties.AndroidMkRlibs) {
		t.Errorf("rlib-std variant for device rust_library_rlib does not link libstd as an rlib")
	}
	if !android.InList("libstd", libfooRlibDynamic.Properties.AndroidMkDylibs) {
		t.Errorf("dylib-std variant for device rust_library_rlib does not link libstd as an dylib")
	}
	if !android.InList("libstd", libfooDylib.Properties.AndroidMkDylibs) {
		t.Errorf("Device rust_library_dylib does not link libstd as an dylib")
	}

	if !android.InList("libstd", libbarShared.Properties.AndroidMkDylibs) {
		t.Errorf("Device rust_ffi_shared does not link libstd as an dylib")
	}
	if !android.InList("libstd", libbarStatic.Properties.AndroidMkRlibs) {
		t.Errorf("Device rust_ffi_static does not link libstd as an rlib")
	}
	if !android.InList("libfoo.rlib-std", libbarStatic.Properties.AndroidMkRlibs) {
		t.Errorf("Device rust_ffi_static does not link dependent rustlib rlib-std variant")
	}
	if !android.InList("libstd", libbarRlibStd.Properties.AndroidMkRlibs) {
		t.Errorf("rust_ffi with prefer_rlib does not link libstd as an rlib")
	}

}
