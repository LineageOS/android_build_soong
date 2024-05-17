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
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/genrule"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

var prepareForRustTest = android.GroupFixturePreparers(
	android.PrepareForTestWithArchMutator,
	android.PrepareForTestWithDefaults,
	android.PrepareForTestWithPrebuilts,

	genrule.PrepareForTestWithGenRuleBuildComponents,

	PrepareForIntegrationTestWithRust,
)

var rustMockedFiles = android.MockFS{
	"foo.rs":                       nil,
	"foo.c":                        nil,
	"src/bar.rs":                   nil,
	"src/any.h":                    nil,
	"c_includes/c_header.h":        nil,
	"rust_includes/rust_headers.h": nil,
	"proto.proto":                  nil,
	"proto/buf.proto":              nil,
	"buf.proto":                    nil,
	"foo.proto":                    nil,
	"liby.so":                      nil,
	"libz.so":                      nil,
	"data.txt":                     nil,
	"liblog.map.txt":               nil,
}

// testRust returns a TestContext in which a basic environment has been setup.
// This environment contains a few mocked files. See rustMockedFiles for the list of these files.
func testRust(t *testing.T, bp string) *android.TestContext {
	skipTestIfOsNotSupported(t)
	result := android.GroupFixturePreparers(
		prepareForRustTest,
		rustMockedFiles.AddToFixture(),
	).
		RunTestWithBp(t, bp)
	return result.TestContext
}

const (
	sharedVendorVariant        = "android_vendor_arm64_armv8-a_shared"
	rlibVendorVariant          = "android_vendor_arm64_armv8-a_rlib_rlib-std"
	rlibDylibStdVendorVariant  = "android_vendor_arm64_armv8-a_rlib_rlib-std"
	dylibVendorVariant         = "android_vendor_arm64_armv8-a_dylib"
	sharedRecoveryVariant      = "android_recovery_arm64_armv8-a_shared"
	rlibRecoveryVariant        = "android_recovery_arm64_armv8-a_rlib_dylib-std"
	rlibRlibStdRecoveryVariant = "android_recovery_arm64_armv8-a_rlib_rlib-std"
	dylibRecoveryVariant       = "android_recovery_arm64_armv8-a_dylib"
	binaryCoreVariant          = "android_arm64_armv8-a"
	binaryVendorVariant        = "android_vendor_arm64_armv8-a"
	binaryProductVariant       = "android_product_arm64_armv8-a"
	binaryRecoveryVariant      = "android_recovery_arm64_armv8-a"
)

// testRustCov returns a TestContext in which a basic environment has been
// setup. This environment explicitly enables coverage.
func testRustCov(t *testing.T, bp string) *android.TestContext {
	skipTestIfOsNotSupported(t)
	result := android.GroupFixturePreparers(
		prepareForRustTest,
		rustMockedFiles.AddToFixture(),
		android.FixtureModifyProductVariables(
			func(variables android.FixtureProductVariables) {
				variables.ClangCoverage = proptools.BoolPtr(true)
				variables.Native_coverage = proptools.BoolPtr(true)
				variables.NativeCoveragePaths = []string{"*"}
			},
		),
	).RunTestWithBp(t, bp)
	return result.TestContext
}

// testRustError ensures that at least one error was raised and its value
// matches the pattern provided. The error can be either in the parsing of the
// Blueprint or when generating the build actions.
func testRustError(t *testing.T, pattern string, bp string) {
	skipTestIfOsNotSupported(t)
	android.GroupFixturePreparers(
		prepareForRustTest,
		rustMockedFiles.AddToFixture(),
	).
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(pattern)).
		RunTestWithBp(t, bp)
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

func skipTestIfOsNotSupported(t *testing.T) {
	// TODO (b/140435149)
	if runtime.GOOS != "linux" {
		t.Skip("Rust Soong tests can only be run on Linux hosts currently")
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
		cc_library {
			host_supported: true,
			name: "cc_stubs_dep",
		}
		cc_library_host_static {
			name: "libstatic",
		}
		cc_library_host_static {
			name: "libwholestatic",
		}
		rust_ffi_host_shared {
			name: "libshared",
			srcs: ["foo.rs"],
			crate_name: "shared",
		}
		rust_library_host_rlib {
			name: "librlib",
			srcs: ["foo.rs"],
			crate_name: "rlib",
			static_libs: ["libstatic"],
			whole_static_libs: ["libwholestatic"],
			shared_libs: ["cc_stubs_dep"],
		}
		rust_proc_macro {
			name: "libpm",
			srcs: ["foo.rs"],
			crate_name: "pm",
		}
		rust_binary_host {
			name: "fizz-buzz",
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
	if !android.InList("librlib.rlib-std", module.Properties.AndroidMkRlibs) {
		t.Errorf("Rlib dependency not detected (dependency missing from AndroidMkRlibs)")
	}

	if !android.InList("libpm", module.Properties.AndroidMkProcMacroLibs) {
		t.Errorf("Proc_macro dependency not detected (dependency missing from AndroidMkProcMacroLibs)")
	}

	if !android.InList("libshared", module.transitiveAndroidMkSharedLibs.ToList()) {
		t.Errorf("Shared library dependency not detected (dependency missing from AndroidMkSharedLibs)")
	}

	if !android.InList("libstatic", module.Properties.AndroidMkStaticLibs) {
		t.Errorf("Static library dependency not detected (dependency missing from AndroidMkStaticLibs)")
	}

	if !strings.Contains(rustc.Args["rustcFlags"], "-lstatic=wholestatic") {
		t.Errorf("-lstatic flag not being passed to rustc for static library %#v", rustc.Args["rustcFlags"])
	}

	if !strings.Contains(rustc.Args["linkFlags"], "cc_stubs_dep.so") {
		t.Errorf("shared cc_library not being passed to rustc linkFlags %#v", rustc.Args["linkFlags"])
	}

	if !android.SuffixInList(rustc.OrderOnly.Strings(), "cc_stubs_dep.so") {
		t.Errorf("shared cc dep not being passed as order-only to rustc %#v", rustc.OrderOnly.Strings())
	}

	if !android.SuffixInList(rustc.Implicits.Strings(), "cc_stubs_dep.so.toc") {
		t.Errorf("shared cc dep TOC not being passed as implicit to rustc %#v", rustc.Implicits.Strings())
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
	if !android.InList("libbindings", libfooMod.Properties.AndroidMkRlibs) {
		t.Errorf("bindgen dependency not detected as a rlib dependency (dependency missing from AndroidMkRlibs)")
	}
	fizzBuzzMod := ctx.ModuleForTests("fizz-buzz-dep", "android_arm64_armv8-a").Module().(*Module)
	if !android.InList("libbindings", fizzBuzzMod.Properties.AndroidMkRlibs) {
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

// Test that library size measurements are generated.
func TestLibrarySizes(t *testing.T) {
	ctx := testRust(t, `
		rust_library_dylib {
			name: "libwaldo",
			srcs: ["foo.rs"],
			crate_name: "waldo",
		}`)

	m := ctx.SingletonForTests("file_metrics")
	m.Output("unstripped/libwaldo.dylib.so.bloaty.csv")
	m.Output("libwaldo.dylib.so.bloaty.csv")
}

// Test that aliases are respected.
func TestRustAliases(t *testing.T) {
	ctx := testRust(t, `
		rust_library {
			name: "libbar",
			crate_name: "bar",
			srcs: ["src/lib.rs"],
		}
		rust_library {
			name: "libbaz",
			crate_name: "baz",
			srcs: ["src/lib.rs"],
		}
		rust_binary {
			name: "foo",
			srcs: ["src/main.rs"],
			rustlibs: ["libbar", "libbaz"],
			aliases: ["bar:bar_renamed"],
		}`)

	fooRustc := ctx.ModuleForTests("foo", "android_arm64_armv8-a").Rule("rustc")
	if !strings.Contains(fooRustc.Args["libFlags"], "--extern bar_renamed=out/soong/.intermediates/libbar/android_arm64_armv8-a_dylib/unstripped/libbar.dylib.so") {
		t.Errorf("--extern bar_renamed=out/soong/.intermediates/libbar/android_arm64_armv8-a_dylib/unstripped/libbar.dylib.so flag not being passed to rustc for rust_binary with aliases. libFlags: %#v", fooRustc.Args["libFlags"])
	}
	if !strings.Contains(fooRustc.Args["libFlags"], "--extern baz=out/soong/.intermediates/libbaz/android_arm64_armv8-a_dylib/unstripped/libbaz.dylib.so") {
		t.Errorf("--extern baz=out/soong/.intermediates/libbaz/android_arm64_armv8-a_dylib/unstripped/libbaz.dylib.so flag not being passed to rustc for rust_binary with aliases. libFlags: %#v", fooRustc.Args["libFlags"])
	}
}

func TestRustRlibs(t *testing.T) {
	ctx := testRust(t, `
		rust_ffi_rlib {
			name: "libbar",
			crate_name: "bar",
			srcs: ["src/lib.rs"],
			export_include_dirs: ["bar_includes"]
		}

		rust_ffi_rlib {
			name: "libfoo",
			crate_name: "foo",
			srcs: ["src/lib.rs"],
			export_include_dirs: ["foo_includes"]
		}

		cc_library_shared {
			name: "libcc_shared",
			srcs:["foo.c"],
			static_rlibs: ["libbar"],
		}

		cc_library_static {
			name: "libcc_static",
			srcs:["foo.c"],
			static_rlibs: ["libfoo"],
		}

		cc_binary {
			name: "ccBin",
			srcs:["foo.c"],
			static_rlibs: ["libbar"],
			static_libs: ["libcc_static"],
		}
		`)

	libbar := ctx.ModuleForTests("libbar", "android_arm64_armv8-a_rlib_rlib-std").Rule("rustc")
	libcc_shared_rustc := ctx.ModuleForTests("libcc_shared", "android_arm64_armv8-a_shared").Rule("rustc")
	libcc_shared_ld := ctx.ModuleForTests("libcc_shared", "android_arm64_armv8-a_shared").Rule("ld")
	libcc_shared_cc := ctx.ModuleForTests("libcc_shared", "android_arm64_armv8-a_shared").Rule("cc")
	ccbin_rustc := ctx.ModuleForTests("ccBin", "android_arm64_armv8-a").Rule("rustc")
	ccbin_ld := ctx.ModuleForTests("ccBin", "android_arm64_armv8-a").Rule("ld")
	ccbin_cc := ctx.ModuleForTests("ccBin", "android_arm64_armv8-a").Rule("cc")

	if !strings.Contains(libbar.Args["rustcFlags"], "crate-type=rlib") {
		t.Errorf("missing crate-type for static variant, expecting %#v, rustcFlags: %#v", "rlib", libbar.Args["rustcFlags"])
	}

	// Make sure there's a rustc command, and it's producing a staticlib
	if !strings.Contains(libcc_shared_rustc.Args["rustcFlags"], "crate-type=staticlib") {
		t.Errorf("missing crate-type for static variant, expecting %#v, rustcFlags: %#v",
			"staticlib", libcc_shared_rustc.Args["rustcFlags"])
	}

	// Make sure the static lib is included in the ld command
	if !strings.Contains(libcc_shared_ld.Args["libFlags"], "generated_rust_staticlib/liblibcc_shared_rust_staticlib.a") {
		t.Errorf("missing generated static library in linker step libFlags %#v, libFlags: %#v",
			"libcc_shared.generated_rust_staticlib.a", libcc_shared_ld.Args["libFlags"])
	}

	// Make sure the static lib includes are in the cc command
	if !strings.Contains(libcc_shared_cc.Args["cFlags"], "-Ibar_includes") {
		t.Errorf("missing rlibs includes, expecting %#v, cFlags: %#v",
			"-Ibar_includes", libcc_shared_cc.Args["cFlags"])
	}

	// Make sure there's a rustc command, and it's producing a staticlib
	if !strings.Contains(ccbin_rustc.Args["rustcFlags"], "crate-type=staticlib") {
		t.Errorf("missing crate-type for static variant, expecting %#v, rustcFlags: %#v", "staticlib", ccbin_rustc.Args["rustcFlags"])
	}

	// Make sure the static lib is included in the cc command
	if !strings.Contains(ccbin_ld.Args["libFlags"], "generated_rust_staticlib/libccBin_rust_staticlib.a") {
		t.Errorf("missing generated static library in linker step libFlags, expecting %#v, libFlags: %#v",
			"ccBin.generated_rust_staticlib.a", ccbin_ld.Args["libFlags"])
	}

	// Make sure the static lib includes are in the ld command
	if !strings.Contains(ccbin_cc.Args["cFlags"], "-Ibar_includes") {
		t.Errorf("missing rlibs includes, expecting %#v, cFlags: %#v",
			"-Ibar_includes", ccbin_cc.Args)
	}

	// Make sure that direct dependencies and indirect dependencies are
	// propagating correctly to the generated rlib.
	if !strings.Contains(ccbin_rustc.Args["libFlags"], "--extern foo=") {
		t.Errorf("Missing indirect dependency libfoo when writing generated Rust staticlib: %#v", ccbin_rustc.Args["libFlags"])
	}
	if !strings.Contains(ccbin_rustc.Args["libFlags"], "--extern bar=") {
		t.Errorf("Missing direct dependency libbar when writing generated Rust staticlib: %#v", ccbin_rustc.Args["libFlags"])
	}

	// Test indirect includes propagation
	if !strings.Contains(ccbin_cc.Args["cFlags"], "-Ifoo_includes") {
		t.Errorf("missing rlibs includes, expecting %#v, cFlags: %#v",
			"-Ifoo_includes", ccbin_cc.Args)
	}
}

func assertString(t *testing.T, got, expected string) {
	t.Helper()
	if got != expected {
		t.Errorf("expected %q got %q", expected, got)
	}
}
