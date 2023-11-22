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

// Test that feature flags are being correctly generated.
func TestFeaturesToFlags(t *testing.T) {
	ctx := testRust(t, `
		rust_library_host_dylib {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
			features: [
				"fizz",
				"buzz"
			],
		}`)

	libfooDylib := ctx.ModuleForTests("libfoo", "linux_glibc_x86_64_dylib").Rule("rustc")

	if !strings.Contains(libfooDylib.Args["rustcFlags"], "cfg 'feature=\"fizz\"'") ||
		!strings.Contains(libfooDylib.Args["rustcFlags"], "cfg 'feature=\"buzz\"'") {
		t.Fatalf("missing fizz and buzz feature flags for libfoo dylib, rustcFlags: %#v", libfooDylib.Args["rustcFlags"])
	}
}

// Test that cfgs flags are being correctly generated.
func TestCfgsToFlags(t *testing.T) {
	ctx := testRust(t, `
		rust_library_host {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
			cfgs: [
				"std",
				"cfg1=\"one\""
			],
		}`)

	libfooDylib := ctx.ModuleForTests("libfoo", "linux_glibc_x86_64_dylib").Rule("rustc")

	if !strings.Contains(libfooDylib.Args["rustcFlags"], "cfg 'std'") ||
		!strings.Contains(libfooDylib.Args["rustcFlags"], "cfg 'cfg1=\"one\"'") {
		t.Fatalf("missing std and cfg1 flags for libfoo dylib, rustcFlags: %#v", libfooDylib.Args["rustcFlags"])
	}
}

// Test that we reject multiple source files.
func TestEnforceSingleSourceFile(t *testing.T) {

	singleSrcError := "srcs can only contain one path for a rust file and source providers prefixed by \":\""
	prebuiltSingleSrcError := "prebuilt libraries can only have one entry in srcs"

	// Test libraries
	testRustError(t, singleSrcError, `
		rust_library_host {
			name: "foo-bar-library",
			srcs: ["foo.rs", "src/bar.rs"],
		}`)

	// Test binaries
	testRustError(t, singleSrcError, `
			rust_binary_host {
				name: "foo-bar-binary",
				srcs: ["foo.rs", "src/bar.rs"],
			}`)

	// Test proc_macros
	testRustError(t, singleSrcError, `
		rust_proc_macro {
			name: "foo-bar-proc-macro",
			srcs: ["foo.rs", "src/bar.rs"],
		}`)

	// Test prebuilts
	testRustError(t, prebuiltSingleSrcError, `
		rust_prebuilt_dylib {
			name: "foo-bar-prebuilt",
			srcs: ["liby.so", "libz.so"],
		  host_supported: true,
		}`)
}

// Test that we reject _no_ source files.
func TestEnforceMissingSourceFiles(t *testing.T) {

	singleSrcError := "srcs must not be empty"

	// Test libraries
	testRustError(t, singleSrcError, `
		rust_library_host {
			name: "foo-bar-library",
			crate_name: "foo",
		}`)

	// Test binaries
	testRustError(t, singleSrcError, `
		rust_binary_host {
			name: "foo-bar-binary",
			crate_name: "foo",
		}`)

	// Test proc_macros
	testRustError(t, singleSrcError, `
		rust_proc_macro {
			name: "foo-bar-proc-macro",
			crate_name: "foo",
		}`)

	// Test prebuilts
	testRustError(t, singleSrcError, `
		rust_prebuilt_dylib {
			name: "foo-bar-prebuilt",
			crate_name: "foo",
		  host_supported: true,
		}`)
}

// Test environment vars for Cargo compat are set.
func TestCargoCompat(t *testing.T) {
	ctx := testRust(t, `
		rust_binary {
			name: "fizz",
			srcs: ["foo.rs"],
			crate_name: "foo",
			cargo_env_compat: true,
			cargo_pkg_version: "1.0.0"
		}`)

	fizz := ctx.ModuleForTests("fizz", "android_arm64_armv8-a").Rule("rustc")

	if !strings.Contains(fizz.Args["envVars"], "CARGO_BIN_NAME=fizz") {
		t.Fatalf("expected 'CARGO_BIN_NAME=fizz' in envVars, actual envVars: %#v", fizz.Args["envVars"])
	}
	if !strings.Contains(fizz.Args["envVars"], "CARGO_CRATE_NAME=foo") {
		t.Fatalf("expected 'CARGO_CRATE_NAME=foo' in envVars, actual envVars: %#v", fizz.Args["envVars"])
	}
	if !strings.Contains(fizz.Args["envVars"], "CARGO_PKG_VERSION=1.0.0") {
		t.Fatalf("expected 'CARGO_PKG_VERSION=1.0.0' in envVars, actual envVars: %#v", fizz.Args["envVars"])
	}
}

func TestInstallDir(t *testing.T) {
	ctx := testRust(t, `
		rust_library_dylib {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
		}
		rust_binary {
			name: "fizzbuzz",
			srcs: ["foo.rs"],
		}`)

	install_path_lib64 := ctx.ModuleForTests("libfoo",
		"android_arm64_armv8-a_dylib").Module().(*Module).compiler.(*libraryDecorator).path.String()
	install_path_lib32 := ctx.ModuleForTests("libfoo",
		"android_arm_armv7-a-neon_dylib").Module().(*Module).compiler.(*libraryDecorator).path.String()
	install_path_bin := ctx.ModuleForTests("fizzbuzz",
		"android_arm64_armv8-a").Module().(*Module).compiler.(*binaryDecorator).path.String()

	if !strings.HasSuffix(install_path_lib64, "system/lib64/libfoo.dylib.so") {
		t.Fatalf("unexpected install path for 64-bit library: %#v", install_path_lib64)
	}
	if !strings.HasSuffix(install_path_lib32, "system/lib/libfoo.dylib.so") {
		t.Fatalf("unexpected install path for 32-bit library: %#v", install_path_lib32)
	}
	if !strings.HasSuffix(install_path_bin, "system/bin/fizzbuzz") {
		t.Fatalf("unexpected install path for binary: %#v", install_path_bin)
	}
}

func TestLints(t *testing.T) {

	bp := `
		// foo uses the default value of lints
		rust_library {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
		}
		// bar forces the use of the "android" lint set
		rust_library {
			name: "libbar",
			srcs: ["foo.rs"],
			crate_name: "bar",
			lints: "android",
		}
		// foobar explicitly disable all lints
		rust_library {
			name: "libfoobar",
			srcs: ["foo.rs"],
			crate_name: "foobar",
			lints: "none",
		}`

	var lintTests = []struct {
		modulePath string
		fooFlags   string
	}{
		{"", "${config.RustDefaultLints}"},
		{"external/", "${config.RustAllowAllLints}"},
		{"hardware/", "${config.RustVendorLints}"},
	}

	for _, tc := range lintTests {
		t.Run("path="+tc.modulePath, func(t *testing.T) {

			result := android.GroupFixturePreparers(
				prepareForRustTest,
				// Test with the blueprint file in different directories.
				android.FixtureAddTextFile(tc.modulePath+"Android.bp", bp),
			).RunTest(t)

			r := result.ModuleForTests("libfoo", "android_arm64_armv8-a_dylib").MaybeRule("rustc")
			android.AssertStringDoesContain(t, "libfoo flags", r.Args["rustcFlags"], tc.fooFlags)

			r = result.ModuleForTests("libbar", "android_arm64_armv8-a_dylib").MaybeRule("rustc")
			android.AssertStringDoesContain(t, "libbar flags", r.Args["rustcFlags"], "${config.RustDefaultLints}")

			r = result.ModuleForTests("libfoobar", "android_arm64_armv8-a_dylib").MaybeRule("rustc")
			android.AssertStringDoesContain(t, "libfoobar flags", r.Args["rustcFlags"], "${config.RustAllowAllLints}")
		})
	}
}

// Test that devices are linking the stdlib dynamically
func TestStdDeviceLinkage(t *testing.T) {
	ctx := testRust(t, `
		rust_binary {
			name: "fizz",
			srcs: ["foo.rs"],
		}
		rust_library {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
		}`)
	fizz := ctx.ModuleForTests("fizz", "android_arm64_armv8-a").Module().(*Module)
	fooRlib := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_rlib_dylib-std").Module().(*Module)
	fooDylib := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_dylib").Module().(*Module)

	if !android.InList("libstd", fizz.Properties.AndroidMkDylibs) {
		t.Errorf("libstd is not linked dynamically for device binaries")
	}
	if !android.InList("libstd", fooRlib.Properties.AndroidMkDylibs) {
		t.Errorf("libstd is not linked dynamically for rlibs")
	}
	if !android.InList("libstd", fooDylib.Properties.AndroidMkDylibs) {
		t.Errorf("libstd is not linked dynamically for dylibs")
	}
}

// Ensure that manual link flags are disallowed.
func TestManualLinkageRejection(t *testing.T) {
	// rustc flags
	testRustError(t, ".* cannot be manually specified", `
		rust_binary {
			name: "foo",
			srcs: [
				"foo.rs",
			],
			flags: ["-lbar"],
		}
	`)
	testRustError(t, ".* cannot be manually specified", `
		rust_binary {
			name: "foo",
			srcs: [
				"foo.rs",
			],
			flags: ["--extern=foo"],
		}
	`)
	testRustError(t, ".* cannot be manually specified", `
		rust_binary {
			name: "foo",
			srcs: [
				"foo.rs",
			],
			flags: ["-Clink-args=foo"],
		}
	`)
	testRustError(t, ".* cannot be manually specified", `
		rust_binary {
			name: "foo",
			srcs: [
				"foo.rs",
			],
			flags: ["-C link-args=foo"],
		}
	`)
	testRustError(t, ".* cannot be manually specified", `
		rust_binary {
			name: "foo",
			srcs: [
				"foo.rs",
			],
			flags: ["-L foo/"],
		}
	`)

	// lld flags
	testRustError(t, ".* cannot be manually specified", `
		rust_binary {
			name: "foo",
			srcs: [
				"foo.rs",
			],
			ld_flags: ["-Wl,-L bar/"],
		}
	`)
	testRustError(t, ".* cannot be manually specified", `
		rust_binary {
			name: "foo",
			srcs: [
				"foo.rs",
			],
			ld_flags: ["-Wl,-lbar"],
		}
	`)
}
