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
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/cc"
)

var (
	buildDir string
)

func setUp() {
	var err error
	buildDir, err = ioutil.TempDir("", "soong_rust_test")
	if err != nil {
		panic(err)
	}
}

func tearDown() {
	os.RemoveAll(buildDir)
}

func TestMain(m *testing.M) {
	run := func() int {
		setUp()
		defer tearDown()

		return m.Run()
	}

	os.Exit(run())
}

func testConfig(bp string) android.Config {
	bp = bp + GatherRequiredDepsForTest()

	fs := map[string][]byte{
		"foo.rs":     nil,
		"src/bar.rs": nil,
		"liby.so":    nil,
		"libz.so":    nil,
	}

	cc.GatherRequiredFilesForTest(fs)

	return android.TestArchConfig(buildDir, nil, bp, fs)
}

func testRust(t *testing.T, bp string) *android.TestContext {
	// TODO (b/140435149)
	if runtime.GOOS != "linux" {
		t.Skip("Only the Linux toolchain is supported for Rust")
	}

	t.Helper()
	config := testConfig(bp)

	t.Helper()
	ctx := CreateTestContext()
	ctx.Register(config)

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	return ctx
}

func testRustError(t *testing.T, pattern string, bp string) {
	// TODO (b/140435149)
	if runtime.GOOS != "linux" {
		t.Skip("Only the Linux toolchain is supported for Rust")
	}

	t.Helper()
	config := testConfig(bp)

	ctx := CreateTestContext()
	ctx.Register(config)

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	if len(errs) > 0 {
		android.FailIfNoMatchingErrors(t, pattern, errs)
		return
	}

	_, errs = ctx.PrepareBuildActions(config)
	if len(errs) > 0 {
		android.FailIfNoMatchingErrors(t, pattern, errs)
		return
	}

	t.Fatalf("missing expected error %q (0 errors are returned)", pattern)
}

// Test that we can extract the lib name from a lib path.
func TestLibNameFromFilePath(t *testing.T) {
	libBarPath := android.PathForTesting("out/soong/.intermediates/external/libbar/libbar/linux_glibc_x86_64_shared/libbar.so.so")
	libLibPath := android.PathForTesting("out/soong/.intermediates/external/libbar/libbar/linux_glibc_x86_64_shared/liblib.dylib.so")

	libBarName := libNameFromFilePath(libBarPath)
	libLibName := libNameFromFilePath(libLibPath)

	expectedResult := "bar.so"
	if libBarName != expectedResult {
		t.Errorf("libNameFromFilePath returned the wrong name; expected '%#v', got '%#v'", expectedResult, libBarName)
	}

	expectedResult = "lib.dylib"
	if libLibName != expectedResult {
		t.Errorf("libNameFromFilePath returned the wrong name; expected '%#v', got '%#v'", expectedResult, libLibPath)
	}
}

// Test that we can extract the link path from a lib path.
func TestLinkPathFromFilePath(t *testing.T) {
	barPath := android.PathForTesting("out/soong/.intermediates/external/libbar/libbar/linux_glibc_x86_64_shared/libbar.so")
	libName := linkPathFromFilePath(barPath)
	expectedResult := "out/soong/.intermediates/external/libbar/libbar/linux_glibc_x86_64_shared/"

	if libName != expectedResult {
		t.Errorf("libNameFromFilePath returned the wrong name; expected '%#v', got '%#v'", expectedResult, libName)
	}
}

// Test to make sure dependencies are being picked up correctly.
func TestDepsTracking(t *testing.T) {
	ctx := testRust(t, `
		rust_library_host_static {
			name: "libstatic",
			srcs: ["foo.rs"],
			crate_name: "static",
		}
		rust_library_host_shared {
			name: "libshared",
			srcs: ["foo.rs"],
			crate_name: "shared",
		}
		rust_library_host_dylib {
			name: "libdylib",
			srcs: ["foo.rs"],
			crate_name: "dylib",
		}
		rust_library_host_rlib {
			name: "librlib",
			srcs: ["foo.rs"],
			crate_name: "rlib",
		}
		rust_proc_macro {
			name: "libpm",
			srcs: ["foo.rs"],
			crate_name: "pm",
		}
		rust_binary_host {
			name: "fizz-buzz",
			dylibs: ["libdylib"],
			rlibs: ["librlib"],
			proc_macros: ["libpm"],
			static_libs: ["libstatic"],
			shared_libs: ["libshared"],
			srcs: ["foo.rs"],
		}
	`)
	module := ctx.ModuleForTests("fizz-buzz", "linux_glibc_x86_64").Module().(*Module)

	// Since dependencies are added to AndroidMk* properties, we can check these to see if they've been picked up.
	if !android.InList("libdylib", module.Properties.AndroidMkDylibs) {
		t.Errorf("Dylib dependency not detected (dependency missing from AndroidMkDylibs)")
	}

	if !android.InList("librlib", module.Properties.AndroidMkRlibs) {
		t.Errorf("Rlib dependency not detected (dependency missing from AndroidMkRlibs)")
	}

	if !android.InList("libpm", module.Properties.AndroidMkProcMacroLibs) {
		t.Errorf("Proc_macro dependency not detected (dependency missing from AndroidMkProcMacroLibs)")
	}

	if !android.InList("libshared", module.Properties.AndroidMkSharedLibs) {
		t.Errorf("Shared library dependency not detected (dependency missing from AndroidMkSharedLibs)")
	}

	if !android.InList("libstatic", module.Properties.AndroidMkStaticLibs) {
		t.Errorf("Static library dependency not detected (dependency missing from AndroidMkStaticLibs)")
	}
}

// Test to make sure proc_macros use host variants when building device modules.
func TestProcMacroDeviceDeps(t *testing.T) {
	ctx := testRust(t, `
		rust_library_host_rlib {
			name: "libbar",
			srcs: ["foo.rs"],
			crate_name: "bar",
		}
		// Make a dummy libstd to let resolution go through
		rust_library_dylib {
			name: "libstd",
			crate_name: "std",
			srcs: ["foo.rs"],
			no_stdlibs: true,
		}
		rust_library_dylib {
			name: "libterm",
			crate_name: "term",
			srcs: ["foo.rs"],
			no_stdlibs: true,
		}
		rust_library_dylib {
			name: "libtest",
			crate_name: "test",
			srcs: ["foo.rs"],
			no_stdlibs: true,
		}
		rust_proc_macro {
			name: "libpm",
			rlibs: ["libbar"],
			srcs: ["foo.rs"],
			crate_name: "pm",
		}
		rust_binary {
			name: "fizz-buzz",
			proc_macros: ["libpm"],
			srcs: ["foo.rs"],
		}
	`)
	rustc := ctx.ModuleForTests("libpm", "linux_glibc_x86_64").Rule("rustc")

	if !strings.Contains(rustc.Args["libFlags"], "libbar/linux_glibc_x86_64") {
		t.Errorf("Proc_macro is not using host variant of dependent modules.")
	}
}

// Test that no_stdlibs suppresses dependencies on rust standard libraries
func TestNoStdlibs(t *testing.T) {
	ctx := testRust(t, `
		rust_binary {
			name: "fizz-buzz",
			srcs: ["foo.rs"],
                        no_stdlibs: true,
		}`)
	module := ctx.ModuleForTests("fizz-buzz", "android_arm64_armv8-a").Module().(*Module)

	if android.InList("libstd", module.Properties.AndroidMkDylibs) {
		t.Errorf("no_stdlibs did not suppress dependency on libstd")
	}
}
