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

func testRust(t *testing.T, bp string) *android.TestContext {
	// TODO (b/140435149)
	if runtime.GOOS != "linux" {
		t.Skip("Only the Linux toolchain is supported for Rust")
	}

	t.Helper()
	config := android.TestArchConfig(buildDir, nil)

	t.Helper()
	ctx := CreateTestContext(bp)
	ctx.Register()

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
	config := android.TestArchConfig(buildDir, nil)

	ctx := CreateTestContext(bp)
	ctx.Register()

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
	barPath := android.PathForTesting("out/soong/.intermediates/external/libbar/libbar/linux_glibc_x86_64_shared/libbar.so")
	libName := libNameFromFilePath(barPath)
	expectedResult := "bar"

	if libName != expectedResult {
		t.Errorf("libNameFromFilePath returned the wrong name; expected '%#v', got '%#v'", expectedResult, libName)
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

// Test default crate names from module names are generated correctly.
func TestDefaultCrateName(t *testing.T) {
	ctx := testRust(t, `
		rust_library_host_dylib {
			name: "fizz-buzz",
			srcs: ["foo.rs"],
		}`)
	module := ctx.ModuleForTests("fizz-buzz", "linux_glibc_x86_64_dylib").Module().(*Module)
	crateName := module.CrateName()
	expectedResult := "fizz_buzz"

	if crateName != expectedResult {
		t.Errorf("CrateName() returned the wrong default crate name; expected '%#v', got '%#v'", expectedResult, crateName)
	}
}

// Test to make sure dependencies are being picked up correctly.
func TestDepsTracking(t *testing.T) {
	ctx := testRust(t, `
		rust_library_host_dylib {
			name: "libfoo",
			srcs: ["foo.rs"],
		}
		rust_library_host_rlib {
			name: "libbar",
			srcs: ["foo.rs"],
		}
		rust_proc_macro {
			name: "libpm",
			srcs: ["foo.rs"],
		}
		rust_binary_host {
			name: "fizz-buzz",
			dylibs: ["libfoo"],
			rlibs: ["libbar"],
			proc_macros: ["libpm"],
			srcs: ["foo.rs"],
		}
	`)
	module := ctx.ModuleForTests("fizz-buzz", "linux_glibc_x86_64").Module().(*Module)

	// Since dependencies are added to AndroidMk* properties, we can check these to see if they've been picked up.
	if !android.InList("libfoo", module.Properties.AndroidMkDylibs) {
		t.Errorf("Dylib dependency not detected (dependency missing from AndroidMkDylibs)")
	}

	if !android.InList("libbar", module.Properties.AndroidMkRlibs) {
		t.Errorf("Rlib dependency not detected (dependency missing from AndroidMkRlibs)")
	}

	if !android.InList("libpm", module.Properties.AndroidMkProcMacroLibs) {
		t.Errorf("Proc_macro dependency not detected (dependency missing from AndroidMkProcMacroLibs)")
	}

}

// Test to make sure proc_macros use host variants when building device modules.
func TestProcMacroDeviceDeps(t *testing.T) {
	ctx := testRust(t, `
		rust_library_host_rlib {
			name: "libbar",
			srcs: ["foo.rs"],
		}
		rust_proc_macro {
			name: "libpm",
			rlibs: ["libbar"],
			srcs: ["foo.rs"],
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
