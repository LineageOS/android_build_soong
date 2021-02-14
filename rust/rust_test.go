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

	"github.com/google/blueprint/proptools"

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

// testRust returns a TestContext in which a basic environment has been setup.
// This environment contains a few mocked files. See testRustCtx.useMockedFs
// for the list of these files.
func testRust(t *testing.T, bp string) *android.TestContext {
	tctx := newTestRustCtx(t, bp)
	tctx.useMockedFs()
	tctx.generateConfig()
	return tctx.parse(t)
}

func testRustVndk(t *testing.T, bp string) *android.TestContext {
	tctx := newTestRustCtx(t, bp)
	tctx.useMockedFs()
	tctx.generateConfig()
	tctx.setVndk(t)
	return tctx.parse(t)
}

// testRustCov returns a TestContext in which a basic environment has been
// setup. This environment explicitly enables coverage.
func testRustCov(t *testing.T, bp string) *android.TestContext {
	tctx := newTestRustCtx(t, bp)
	tctx.useMockedFs()
	tctx.generateConfig()
	tctx.enableCoverage(t)
	return tctx.parse(t)
}

// testRustError ensures that at least one error was raised and its value
// matches the pattern provided. The error can be either in the parsing of the
// Blueprint or when generating the build actions.
func testRustError(t *testing.T, pattern string, bp string) {
	tctx := newTestRustCtx(t, bp)
	tctx.useMockedFs()
	tctx.generateConfig()
	tctx.parseError(t, pattern)
}

// testRustCtx is used to build a particular test environment. Unless your
// tests requires a specific setup, prefer the wrapping functions: testRust,
// testRustCov or testRustError.
type testRustCtx struct {
	bp     string
	fs     map[string][]byte
	env    map[string]string
	config *android.Config
}

// newTestRustCtx returns a new testRustCtx for the Blueprint definition argument.
func newTestRustCtx(t *testing.T, bp string) *testRustCtx {
	// TODO (b/140435149)
	if runtime.GOOS != "linux" {
		t.Skip("Rust Soong tests can only be run on Linux hosts currently")
	}
	return &testRustCtx{bp: bp}
}

// useMockedFs setup a default mocked filesystem for the test environment.
func (tctx *testRustCtx) useMockedFs() {
	tctx.fs = map[string][]byte{
		"foo.rs":          nil,
		"foo.c":           nil,
		"src/bar.rs":      nil,
		"src/any.h":       nil,
		"proto.proto":     nil,
		"proto/buf.proto": nil,
		"buf.proto":       nil,
		"foo.proto":       nil,
		"liby.so":         nil,
		"libz.so":         nil,
		"data.txt":        nil,
	}
}

// generateConfig creates the android.Config based on the bp, fs and env
// attributes of the testRustCtx.
func (tctx *testRustCtx) generateConfig() {
	tctx.bp = tctx.bp + GatherRequiredDepsForTest()
	tctx.bp = tctx.bp + cc.GatherRequiredDepsForTest(android.NoOsType)
	cc.GatherRequiredFilesForTest(tctx.fs)
	config := android.TestArchConfig(buildDir, tctx.env, tctx.bp, tctx.fs)
	tctx.config = &config
}

// enableCoverage configures the test to enable coverage.
func (tctx *testRustCtx) enableCoverage(t *testing.T) {
	if tctx.config == nil {
		t.Fatalf("tctx.config not been generated yet. Please call generateConfig first.")
	}
	tctx.config.TestProductVariables.ClangCoverage = proptools.BoolPtr(true)
	tctx.config.TestProductVariables.Native_coverage = proptools.BoolPtr(true)
	tctx.config.TestProductVariables.NativeCoveragePaths = []string{"*"}
}

func (tctx *testRustCtx) setVndk(t *testing.T) {
	if tctx.config == nil {
		t.Fatalf("tctx.config not been generated yet. Please call generateConfig first.")
	}
	tctx.config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	tctx.config.TestProductVariables.ProductVndkVersion = StringPtr("current")
	tctx.config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
}

// parse validates the configuration and parses the Blueprint file. It returns
// a TestContext which can be used to retrieve the generated modules via
// ModuleForTests.
func (tctx testRustCtx) parse(t *testing.T) *android.TestContext {
	if tctx.config == nil {
		t.Fatalf("tctx.config not been generated yet. Please call generateConfig first.")
	}
	ctx := CreateTestContext(*tctx.config)
	ctx.Register()
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(*tctx.config)
	android.FailIfErrored(t, errs)
	return ctx
}

// parseError parses the Blueprint file and ensure that at least one error
// matching the provided pattern is observed.
func (tctx testRustCtx) parseError(t *testing.T, pattern string) {
	if tctx.config == nil {
		t.Fatalf("tctx.config not been generated yet. Please call generateConfig first.")
	}
	ctx := CreateTestContext(*tctx.config)
	ctx.Register()

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	if len(errs) > 0 {
		android.FailIfNoMatchingErrors(t, pattern, errs)
		return
	}

	_, errs = ctx.PrepareBuildActions(*tctx.config)
	if len(errs) > 0 {
		android.FailIfNoMatchingErrors(t, pattern, errs)
		return
	}

	t.Fatalf("missing expected error %q (0 errors are returned)", pattern)
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
		rust_ffi_host_static {
			name: "libstatic",
			srcs: ["foo.rs"],
			crate_name: "static",
		}
		rust_ffi_host_shared {
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
			static_libs: ["libstatic"],
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
	rustc := ctx.ModuleForTests("librlib", "linux_glibc_x86_64_rlib_rlib-std").Rule("rustc")

	// Since dependencies are added to AndroidMk* properties, we can check these to see if they've been picked up.
	if !android.InList("libdylib", module.Properties.AndroidMkDylibs) {
		t.Errorf("Dylib dependency not detected (dependency missing from AndroidMkDylibs)")
	}

	if !android.InList("librlib.rlib-std", module.Properties.AndroidMkRlibs) {
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

	if !strings.Contains(rustc.Args["rustcFlags"], "-lstatic=static") {
		t.Errorf("-lstatic flag not being passed to rustc for static library")
	}

}

func TestSourceProviderDeps(t *testing.T) {
	ctx := testRust(t, `
		rust_binary {
			name: "fizz-buzz-dep",
			srcs: [
				"foo.rs",
				":my_generator",
				":libbindings",
			],
			rlibs: ["libbindings"],
		}
		rust_proc_macro {
			name: "libprocmacro",
			srcs: [
				"foo.rs",
				":my_generator",
				":libbindings",
			],
			rlibs: ["libbindings"],
			crate_name: "procmacro",
		}
		rust_library {
			name: "libfoo",
			srcs: [
				"foo.rs",
				":my_generator",
				":libbindings",
			],
			rlibs: ["libbindings"],
			crate_name: "foo",
		}
		genrule {
			name: "my_generator",
			tools: ["any_rust_binary"],
			cmd: "$(location) -o $(out) $(in)",
			srcs: ["src/any.h"],
			out: ["src/any.rs"],
		}
		rust_binary_host {
			name: "any_rust_binary",
			srcs: [
				"foo.rs",
			],
		}
		rust_bindgen {
			name: "libbindings",
			crate_name: "bindings",
			source_stem: "bindings",
			host_supported: true,
			wrapper_src: "src/any.h",
        }
	`)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_rlib_dylib-std").Rule("rustc")
	if !android.SuffixInList(libfoo.Implicits.Strings(), "/out/bindings.rs") {
		t.Errorf("rust_bindgen generated source not included as implicit input for libfoo; Implicits %#v", libfoo.Implicits.Strings())
	}
	if !android.SuffixInList(libfoo.Implicits.Strings(), "/out/any.rs") {
		t.Errorf("genrule generated source not included as implicit input for libfoo; Implicits %#v", libfoo.Implicits.Strings())
	}

	fizzBuzz := ctx.ModuleForTests("fizz-buzz-dep", "android_arm64_armv8-a").Rule("rustc")
	if !android.SuffixInList(fizzBuzz.Implicits.Strings(), "/out/bindings.rs") {
		t.Errorf("rust_bindgen generated source not included as implicit input for fizz-buzz-dep; Implicits %#v", libfoo.Implicits.Strings())
	}
	if !android.SuffixInList(fizzBuzz.Implicits.Strings(), "/out/any.rs") {
		t.Errorf("genrule generated source not included as implicit input for fizz-buzz-dep; Implicits %#v", libfoo.Implicits.Strings())
	}

	libprocmacro := ctx.ModuleForTests("libprocmacro", "linux_glibc_x86_64").Rule("rustc")
	if !android.SuffixInList(libprocmacro.Implicits.Strings(), "/out/bindings.rs") {
		t.Errorf("rust_bindgen generated source not included as implicit input for libprocmacro; Implicits %#v", libfoo.Implicits.Strings())
	}
	if !android.SuffixInList(libprocmacro.Implicits.Strings(), "/out/any.rs") {
		t.Errorf("genrule generated source not included as implicit input for libprocmacro; Implicits %#v", libfoo.Implicits.Strings())
	}

	// Check that our bindings are picked up as crate dependencies as well
	libfooMod := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_dylib").Module().(*Module)
	if !android.InList("libbindings.dylib-std", libfooMod.Properties.AndroidMkRlibs) {
		t.Errorf("bindgen dependency not detected as a rlib dependency (dependency missing from AndroidMkRlibs)")
	}
	fizzBuzzMod := ctx.ModuleForTests("fizz-buzz-dep", "android_arm64_armv8-a").Module().(*Module)
	if !android.InList("libbindings.dylib-std", fizzBuzzMod.Properties.AndroidMkRlibs) {
		t.Errorf("bindgen dependency not detected as a rlib dependency (dependency missing from AndroidMkRlibs)")
	}
	libprocmacroMod := ctx.ModuleForTests("libprocmacro", "linux_glibc_x86_64").Module().(*Module)
	if !android.InList("libbindings.rlib-std", libprocmacroMod.Properties.AndroidMkRlibs) {
		t.Errorf("bindgen dependency not detected as a rlib dependency (dependency missing from AndroidMkRlibs)")
	}

}

func TestSourceProviderTargetMismatch(t *testing.T) {
	// This might error while building the dependency tree or when calling depsToPaths() depending on the lunched
	// target, which results in two different errors. So don't check the error, just confirm there is one.
	testRustError(t, ".*", `
		rust_proc_macro {
			name: "libprocmacro",
			srcs: [
				"foo.rs",
				":libbindings",
			],
			crate_name: "procmacro",
		}
		rust_bindgen {
			name: "libbindings",
			crate_name: "bindings",
			source_stem: "bindings",
			wrapper_src: "src/any.h",
		}
	`)
}

// Test to make sure proc_macros use host variants when building device modules.
func TestProcMacroDeviceDeps(t *testing.T) {
	ctx := testRust(t, `
		rust_library_host_rlib {
			name: "libbar",
			srcs: ["foo.rs"],
			crate_name: "bar",
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

// Test that libraries provide both 32-bit and 64-bit variants.
func TestMultilib(t *testing.T) {
	ctx := testRust(t, `
		rust_library_rlib {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
		}`)

	_ = ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_rlib_dylib-std")
	_ = ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_rlib_dylib-std")
}
