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
	"android/soong/cc"
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

// Test that we reject multiple source files.
func TestEnforceSingleSourceFile(t *testing.T) {

	singleSrcError := "srcs can only contain one path for a rust file and source providers prefixed by \":\""

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
	testRustError(t, singleSrcError, `
		rust_prebuilt_dylib {
			name: "foo-bar-prebuilt",
			srcs: ["liby.so", "libz.so"],
		  host_supported: true,
		}`)
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

	bp = bp + GatherRequiredDepsForTest()
	bp = bp + cc.GatherRequiredDepsForTest(android.NoOsType)

	fs := map[string][]byte{
		// Reuse the same blueprint file for subdirectories.
		"external/Android.bp": []byte(bp),
		"hardware/Android.bp": []byte(bp),
	}

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

			config := android.TestArchConfig(t.TempDir(), nil, bp, fs)
			ctx := CreateTestContext(config)
			ctx.Register()
			_, errs := ctx.ParseFileList(".", []string{tc.modulePath + "Android.bp"})
			android.FailIfErrored(t, errs)
			_, errs = ctx.PrepareBuildActions(config)
			android.FailIfErrored(t, errs)

			r := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_dylib").MaybeRule("rustc")
			if !strings.Contains(r.Args["rustcFlags"], tc.fooFlags) {
				t.Errorf("Incorrect flags for libfoo: %q, want %q", r.Args["rustcFlags"], tc.fooFlags)
			}

			r = ctx.ModuleForTests("libbar", "android_arm64_armv8-a_dylib").MaybeRule("rustc")
			if !strings.Contains(r.Args["rustcFlags"], "${config.RustDefaultLints}") {
				t.Errorf("Incorrect flags for libbar: %q, want %q", r.Args["rustcFlags"], "${config.RustDefaultLints}")
			}

			r = ctx.ModuleForTests("libfoobar", "android_arm64_armv8-a_dylib").MaybeRule("rustc")
			if !strings.Contains(r.Args["rustcFlags"], "${config.RustAllowAllLints}") {
				t.Errorf("Incorrect flags for libfoobar: %q, want %q", r.Args["rustcFlags"], "${config.RustAllowAllLints}")
			}

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
