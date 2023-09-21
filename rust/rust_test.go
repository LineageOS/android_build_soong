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
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/google/blueprint/proptools"
	"google.golang.org/protobuf/encoding/prototext"

	"android/soong/android"
	"android/soong/cmd/sbox/sbox_proto"
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

	PrepareForTestWithRustIncludeVndk,
	android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
		variables.DeviceVndkVersion = StringPtr("current")
		variables.ProductVndkVersion = StringPtr("current")
		variables.Platform_vndk_version = StringPtr("29")
	}),
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
func testRust(t *testing.T, bp string, preparers ...android.FixturePreparer) *android.TestContext {
	skipTestIfOsNotSupported(t)
	result := android.GroupFixturePreparers(
		prepareForRustTest,
		rustMockedFiles.AddToFixture(),
		android.GroupFixturePreparers(
			preparers...,
		),
	).
		RunTestWithBp(t, bp)
	return result.TestContext
}

func testRustVndk(t *testing.T, bp string) *android.TestContext {
	return testRustVndkFs(t, bp, rustMockedFiles)
}

const (
	sharedVendorVariant        = "android_vendor.29_arm64_armv8-a_shared"
	rlibVendorVariant          = "android_vendor.29_arm64_armv8-a_rlib_rlib-std"
	rlibDylibStdVendorVariant  = "android_vendor.29_arm64_armv8-a_rlib_rlib-std"
	dylibVendorVariant         = "android_vendor.29_arm64_armv8-a_dylib"
	sharedRecoveryVariant      = "android_recovery_arm64_armv8-a_shared"
	rlibRecoveryVariant        = "android_recovery_arm64_armv8-a_rlib_dylib-std"
	rlibRlibStdRecoveryVariant = "android_recovery_arm64_armv8-a_rlib_rlib-std"
	dylibRecoveryVariant       = "android_recovery_arm64_armv8-a_dylib"
	binaryCoreVariant          = "android_arm64_armv8-a"
	binaryVendorVariant        = "android_vendor.29_arm64_armv8-a"
	binaryProductVariant       = "android_product.29_arm64_armv8-a"
	binaryRecoveryVariant      = "android_recovery_arm64_armv8-a"
)

func testRustVndkFs(t *testing.T, bp string, fs android.MockFS) *android.TestContext {
	return testRustVndkFsVersions(t, bp, fs, "current", "current", "29")
}

func testRustVndkFsVersions(t *testing.T, bp string, fs android.MockFS, device_version, product_version, vndk_version string) *android.TestContext {
	skipTestIfOsNotSupported(t)
	result := android.GroupFixturePreparers(
		prepareForRustTest,
		fs.AddToFixture(),
		android.FixtureModifyProductVariables(
			func(variables android.FixtureProductVariables) {
				variables.DeviceVndkVersion = StringPtr(device_version)
				variables.ProductVndkVersion = StringPtr(product_version)
				variables.Platform_vndk_version = StringPtr(vndk_version)
			},
		),
	).RunTestWithBp(t, bp)
	return result.TestContext
}

func testRustRecoveryFsVersions(t *testing.T, bp string, fs android.MockFS, device_version, vndk_version, recovery_version string) *android.TestContext {
	skipTestIfOsNotSupported(t)
	result := android.GroupFixturePreparers(
		prepareForRustTest,
		fs.AddToFixture(),
		android.FixtureModifyProductVariables(
			func(variables android.FixtureProductVariables) {
				variables.DeviceVndkVersion = StringPtr(device_version)
				variables.RecoverySnapshotVersion = StringPtr(recovery_version)
				variables.Platform_vndk_version = StringPtr(vndk_version)
			},
		),
	).RunTestWithBp(t, bp)
	return result.TestContext
}

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

// testRustVndkError is similar to testRustError, but can be used to test VNDK-related errors.
func testRustVndkError(t *testing.T, pattern string, bp string) {
	testRustVndkFsError(t, pattern, bp, rustMockedFiles)
}

func testRustVndkFsError(t *testing.T, pattern string, bp string, fs android.MockFS) {
	skipTestIfOsNotSupported(t)
	android.GroupFixturePreparers(
		prepareForRustTest,
		fs.AddToFixture(),
		android.FixtureModifyProductVariables(
			func(variables android.FixtureProductVariables) {
				variables.DeviceVndkVersion = StringPtr("current")
				variables.ProductVndkVersion = StringPtr("current")
				variables.Platform_vndk_version = StringPtr("VER")
			},
		),
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
	libName := barPath.Dir()
	expectedResult := "out/soong/.intermediates/external/libbar/libbar/linux_glibc_x86_64_shared"

	if libName.String() != expectedResult {
		t.Errorf("libNameFromFilePath returned the wrong name; expected '%#v', got '%#v'", expectedResult, libName.String())
	}
}

// Test to make sure dependencies are being picked up correctly.
func TestDepsTracking(t *testing.T) {
	ctx := testRust(t, `
		cc_library {
			host_supported: true,
			name: "cc_stubs_dep",
		}
		rust_ffi_host_static {
			name: "libstatic",
			srcs: ["foo.rs"],
			crate_name: "static",
		}
		rust_ffi_host_static {
			name: "libwholestatic",
			srcs: ["foo.rs"],
			crate_name: "wholestatic",
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
	rustLink := ctx.ModuleForTests("fizz-buzz", "linux_glibc_x86_64").Rule("rustc")

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

	if !strings.Contains(rustc.RuleParams.Command, "-lstatic=wholestatic") {
		t.Errorf("-lstatic flag not being passed to rustc for static library %#v", rustc.RuleParams.Command)
	}

	if !strings.Contains(rustLink.RuleParams.Command, "cc_stubs_dep.so") {
		t.Errorf("shared cc_library not being passed to rustc linkFlags %#v", rustLink.RuleParams.Command)
	}

	if !android.SuffixInList(rustLink.Implicits.Strings(), "cc_stubs_dep.so") {
		t.Errorf("shared cc dep not being passed as implicit to rustc %#v", rustLink.OrderOnly.Strings())
	}

	if !android.SuffixInList(rustLink.Implicits.Strings(), "cc_stubs_dep.so.toc") {
		t.Errorf("shared cc dep TOC not being passed as implicit to rustc %#v", rustLink.Implicits.Strings())
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

	if !strings.Contains(rustc.RuleParams.Command, "libbar/linux_glibc_x86_64") {
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

func assertString(t *testing.T, got, expected string) {
	t.Helper()
	if got != expected {
		t.Errorf("expected %q got %q", expected, got)
	}
}

var (
	sboxCompilationFiles = []string{
		"out/soong/.intermediates/defaults/rust/libaddr2line/android_arm64_armv8-a_rlib/libaddr2line.rlib",
		"out/soong/.intermediates/defaults/rust/libadler/android_arm64_armv8-a_rlib/libadler.rlib",
		"out/soong/.intermediates/defaults/rust/liballoc/android_arm64_armv8-a_rlib/liballoc.rlib",
		"out/soong/.intermediates/defaults/rust/libcfg_if/android_arm64_armv8-a_rlib/libcfg_if.rlib",
		"out/soong/.intermediates/defaults/rust/libcompiler_builtins/android_arm64_armv8-a_rlib/libcompiler_builtins.rlib",
		"out/soong/.intermediates/defaults/rust/libcore/android_arm64_armv8-a_rlib/libcore.rlib",
		"out/soong/.intermediates/defaults/rust/libgimli/android_arm64_armv8-a_rlib/libgimli.rlib",
		"out/soong/.intermediates/defaults/rust/libhashbrown/android_arm64_armv8-a_rlib/libhashbrown.rlib",
		"out/soong/.intermediates/defaults/rust/liblibc/android_arm64_armv8-a_rlib/liblibc.rlib",
		"out/soong/.intermediates/defaults/rust/libmemchr/android_arm64_armv8-a_rlib/libmemchr.rlib",
		"out/soong/.intermediates/defaults/rust/libminiz_oxide/android_arm64_armv8-a_rlib/libminiz_oxide.rlib",
		"out/soong/.intermediates/defaults/rust/libobject/android_arm64_armv8-a_rlib/libobject.rlib",
		"out/soong/.intermediates/defaults/rust/libpanic_unwind/android_arm64_armv8-a_rlib/libpanic_unwind.rlib",
		"out/soong/.intermediates/defaults/rust/librustc_demangle/android_arm64_armv8-a_rlib/librustc_demangle.rlib",
		"out/soong/.intermediates/defaults/rust/librustc_std_workspace_alloc/android_arm64_armv8-a_rlib/librustc_std_workspace_alloc.rlib",
		"out/soong/.intermediates/defaults/rust/librustc_std_workspace_core/android_arm64_armv8-a_rlib/librustc_std_workspace_core.rlib",
		"out/soong/.intermediates/defaults/rust/libstd_detect/android_arm64_armv8-a_rlib/libstd_detect.rlib",
		"build/soong/scripts/mkcratersp.py",
		"defaults/rust/linux-x86/1.69.0/bin/rustc",
		"defaults/rust/linux-x86/1.69.0/lib/libstd.so",
		"defaults/rust/linux-x86/1.69.0/lib64/libc++.so.1",
	}
	sboxCompilationFilesWithCc = []string{
		"defaults/cc/common",
		"out/soong/.intermediates/defaults/cc/common/libc/android_arm64_armv8-a_shared/libc.so",
		"out/soong/.intermediates/defaults/cc/common/libc/android_arm64_armv8-a_shared/libc.so.toc",
		"out/soong/.intermediates/defaults/cc/common/libdl/android_arm64_armv8-a_shared/libdl.so",
		"out/soong/.intermediates/defaults/cc/common/libdl/android_arm64_armv8-a_shared/libdl.so.toc",
		"out/soong/.intermediates/defaults/cc/common/libm/android_arm64_armv8-a_shared/libm.so",
		"out/soong/.intermediates/defaults/cc/common/libm/android_arm64_armv8-a_shared/libm.so.toc",
		"out/soong/.intermediates/defaults/rust/liblog/android_arm64_armv8-a_shared/liblog.so",
		"out/soong/.intermediates/defaults/rust/liblog/android_arm64_armv8-a_shared/liblog.so.toc",
	}
)

func TestSandboxCompilation(t *testing.T) {
	ctx := testRust(t, `
		filegroup {
			name: "libsrcs1",
			srcs: ["src_filegroup1.rs"],
		}
		filegroup {
			name: "libsrcs2",
			srcs: ["src_filegroup2.rs"],
		}
		rust_library {
			name: "libfizz_buzz",
			crate_name:"fizz_buzz",
			crate_root: "foo.rs",
			srcs: [
				"src_lib*.rs",
				":libsrcs1",
				":libsrcs2",
			],
			compile_data: [
				"compile_data1.txt",
				"compile_data2.txt",
			],
			dylib: {
				srcs: ["dylib_only.rs"],
			},
			rlib: {
				srcs: ["rlib_only.rs"],
			},
		}
		rust_binary {
			name: "fizz_buzz",
			crate_name:"fizz_buzz",
			crate_root: "foo.rs",
			srcs: [
				"src_lib*.rs",
				":libsrcs1",
				":libsrcs2",
			],
		}
		rust_ffi {
			name: "librust_ffi",
			crate_name: "rust_ffi",
			crate_root: "foo.rs",
			static: {
				srcs: ["static_only.rs"],
			},
			shared: {
				srcs: ["shared_only.rs"],
			},
			srcs: ["src1.rs"],
		}
		cc_library_static {
			name: "cc_dep_static",
		}
		cc_library_shared {
			name: "cc_dep_shared",
		}
		rust_library {
			name: "libfizz_buzz_cc_deps",
			crate_name:"fizz_buzz",
			crate_root: "foo.rs",
			srcs: ["src*.rs"],
			shared_libs: ["cc_dep_shared"],
			static_libs: ["cc_dep_static"],
		}
		rust_library {
			name: "libfizz_buzz_intermediate_cc_deps",
			crate_name:"fizz_buzz",
			crate_root: "foo.rs",
			srcs: ["src*.rs"],
			rustlibs: ["libfizz_buzz_cc_deps"],
		}
		rust_library {
			name: "libfizz_buzz_transitive_cc_deps",
			crate_name:"fizz_buzz",
			crate_root: "foo.rs",
			srcs: ["src*.rs"],
			rustlibs: ["libfizz_buzz_intermediate_cc_deps"],
		}
	`,
		android.FixtureMergeMockFs(android.MockFS{
			"src_lib1.rs":       nil,
			"src_lib2.rs":       nil,
			"src_lib3.rs":       nil,
			"src_lib4.rs":       nil,
			"src_filegroup1.rs": nil,
			"src_filegroup2.rs": nil,
			"static_only.rs":    nil,
			"shared_only.rs":    nil,
		}),
	)

	testcases := []struct {
		name                     string
		moduleName               string
		variant                  string
		rustcExpectedFilesToCopy []string
		expectedFlags            []string
	}{
		{
			name:       "rust_library (dylib)",
			moduleName: "libfizz_buzz",
			variant:    "android_arm64_armv8-a_dylib",
			rustcExpectedFilesToCopy: android.Concat(sboxCompilationFiles, sboxCompilationFilesWithCc, []string{
				"foo.rs",
				"src_lib1.rs",
				"src_lib2.rs",
				"src_lib3.rs",
				"src_lib4.rs",
				"src_filegroup1.rs",
				"src_filegroup2.rs",
				"compile_data1.txt",
				"compile_data2.txt",
				"dylib_only.rs",
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_dylib/out/src_filegroup1.rs",
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_dylib/out/src_filegroup2.rs",

				"out/soong/.intermediates/defaults/cc/common/libc/android_arm64_armv8-a_shared/libc.so",
				"out/soong/.intermediates/defaults/cc/common/libc/android_arm64_armv8-a_shared/libc.so.toc",
				"out/soong/.intermediates/defaults/cc/common/libm/android_arm64_armv8-a_shared/libm.so",
				"out/soong/.intermediates/defaults/cc/common/libm/android_arm64_armv8-a_shared/libm.so.toc",
				"out/soong/.intermediates/defaults/cc/common/libdl/android_arm64_armv8-a_shared/libdl.so",
				"out/soong/.intermediates/defaults/cc/common/libdl/android_arm64_armv8-a_shared/libdl.so.toc",
				"out/soong/.intermediates/defaults/rust/libstd/android_arm64_armv8-a_dylib/unstripped/libstd.dylib.so",
				"out/soong/.intermediates/defaults/cc/common/crtbegin_so/android_arm64_armv8-a/crtbegin_so.o",
				"out/soong/.intermediates/defaults/cc/common/crtend_so/android_arm64_armv8-a/crtend_so.o",
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_dylib/libfizz_buzz.dylib.so.clippy",
			}),
			expectedFlags: []string{
				"-C linker=build/soong/scripts/mkcratersp.py",
				"--emit link",
				"-o __SBOX_SANDBOX_DIR__/out/libfizz_buzz.dylib.so.rsp",
				"--emit dep-info=__SBOX_SANDBOX_DIR__/out/libfizz_buzz.dylib.so.d.raw",
				"foo.rs", // this is the entry point
			},
		},
		{
			name:       "rust_library (rlib dylib-std)",
			moduleName: "libfizz_buzz",
			variant:    "android_arm64_armv8-a_rlib_dylib-std",
			rustcExpectedFilesToCopy: android.Concat(sboxCompilationFiles, []string{
				"foo.rs",
				"src_lib1.rs",
				"src_lib2.rs",
				"src_lib3.rs",
				"src_lib4.rs",
				"src_filegroup1.rs",
				"src_filegroup2.rs",
				"rlib_only.rs",
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_rlib_dylib-std/out/src_filegroup1.rs",
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_rlib_dylib-std/out/src_filegroup2.rs",
				"out/soong/.intermediates/defaults/rust/libstd/android_arm64_armv8-a_dylib/unstripped/libstd.dylib.so",
			}),
			expectedFlags: []string{
				"--emit link",
				"-o __SBOX_SANDBOX_DIR__/out/libfizz_buzz.rlib",
				"--emit dep-info=__SBOX_SANDBOX_DIR__/out/libfizz_buzz.rlib.d.raw",
				"foo.rs", // this is the entry point
			},
		},
		{
			name:       "rust_library (rlib rlib-std)",
			moduleName: "libfizz_buzz",
			variant:    "android_arm64_armv8-a_rlib_rlib-std",
			rustcExpectedFilesToCopy: android.Concat(sboxCompilationFiles, []string{
				"foo.rs",
				"src_lib1.rs",
				"src_lib2.rs",
				"src_lib3.rs",
				"src_lib4.rs",
				"src_filegroup1.rs",
				"src_filegroup2.rs",
				"rlib_only.rs",
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_rlib_rlib-std/out/src_filegroup1.rs",
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_rlib_rlib-std/out/src_filegroup2.rs",
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_rlib_rlib-std/libfizz_buzz.rlib.clippy",
				"out/soong/.intermediates/defaults/rust/libstd/android_arm64_armv8-a_rlib/libstd.rlib",
			}),
			expectedFlags: []string{
				"--emit link",
				"-o __SBOX_SANDBOX_DIR__/out/libfizz_buzz.rlib",
				"--emit dep-info=__SBOX_SANDBOX_DIR__/out/libfizz_buzz.rlib.d.raw",
				"foo.rs", // this is the entry point
			},
		},
		{
			name:       "rust_binary",
			moduleName: "fizz_buzz",
			variant:    "android_arm64_armv8-a",
			rustcExpectedFilesToCopy: android.Concat(sboxCompilationFiles, sboxCompilationFilesWithCc, []string{
				"foo.rs",
				"src_lib1.rs",
				"src_lib2.rs",
				"src_lib3.rs",
				"src_lib4.rs",
				"src_filegroup1.rs",
				"src_filegroup2.rs",
				"out/soong/.intermediates/fizz_buzz/android_arm64_armv8-a/out/src_filegroup1.rs",
				"out/soong/.intermediates/fizz_buzz/android_arm64_armv8-a/out/src_filegroup2.rs",

				"out/soong/.intermediates/defaults/rust/libstd/android_arm64_armv8-a_dylib/unstripped/libstd.dylib.so",
				"out/soong/.intermediates/defaults/cc/common/crtbegin_dynamic/android_arm64_armv8-a/crtbegin_dynamic.o",
				"out/soong/.intermediates/defaults/cc/common/crtend_android/android_arm64_armv8-a/crtend_android.o",
				"out/soong/.intermediates/fizz_buzz/android_arm64_armv8-a/fizz_buzz.clippy",
			}),
			expectedFlags: []string{
				"--emit link",
				"-o __SBOX_SANDBOX_DIR__/out/fizz_buzz",
				"--emit dep-info=__SBOX_SANDBOX_DIR__/out/fizz_buzz.d.raw",
				"foo.rs", // this is the entry point
			},
		},
		{
			name:       "rust_ffi static lib variant",
			moduleName: "librust_ffi",
			variant:    "android_arm64_armv8-a_static",
			rustcExpectedFilesToCopy: android.Concat(sboxCompilationFiles, []string{
				"foo.rs",
				"src1.rs",
				"static_only.rs",
				"out/soong/.intermediates/librust_ffi/android_arm64_armv8-a_static/librust_ffi.a.clippy",
				"out/soong/.intermediates/defaults/rust/libstd/android_arm64_armv8-a_rlib/libstd.rlib",
			}),
			expectedFlags: []string{
				"--emit link",
				"-o __SBOX_SANDBOX_DIR__/out/librust_ffi.a",
				"--emit dep-info=__SBOX_SANDBOX_DIR__/out/librust_ffi.a.d.raw",
				"foo.rs", // this is the entry point
			},
		},
		{
			name:       "rust_ffi shared lib variant",
			moduleName: "librust_ffi",
			variant:    "android_arm64_armv8-a_shared",
			rustcExpectedFilesToCopy: android.Concat(sboxCompilationFiles, sboxCompilationFilesWithCc, []string{
				"foo.rs",
				"src1.rs",
				"shared_only.rs",

				"out/soong/.intermediates/defaults/rust/libstd/android_arm64_armv8-a_dylib/unstripped/libstd.dylib.so",
				"out/soong/.intermediates/defaults/cc/common/crtbegin_so/android_arm64_armv8-a/crtbegin_so.o",
				"out/soong/.intermediates/defaults/cc/common/crtend_so/android_arm64_armv8-a/crtend_so.o",
				"out/soong/.intermediates/librust_ffi/android_arm64_armv8-a_shared/librust_ffi.so.clippy",
			}),
			expectedFlags: []string{
				"--emit link",
				"-o __SBOX_SANDBOX_DIR__/out/librust_ffi.so",
				"--emit dep-info=__SBOX_SANDBOX_DIR__/out/librust_ffi.so.d.raw",
				"foo.rs", // this is the entry point
			},
		},
		{
			name:       "rust_library with cc deps (dylib)",
			moduleName: "libfizz_buzz_cc_deps",
			variant:    "android_arm64_armv8-a_dylib",
			rustcExpectedFilesToCopy: []string{
				"out/soong/.intermediates/cc_dep_static/android_arm64_armv8-a_static/cc_dep_static.a",
				"out/soong/.intermediates/cc_dep_shared/android_arm64_armv8-a_shared/cc_dep_shared.so",
				"out/soong/.intermediates/cc_dep_shared/android_arm64_armv8-a_shared/cc_dep_shared.so.toc",
			},
		},
		{
			name:       "rust_library with cc deps (rlib rlib-std)",
			moduleName: "libfizz_buzz_cc_deps",
			variant:    "android_arm64_armv8-a_rlib_rlib-std",
			rustcExpectedFilesToCopy: []string{
				"out/soong/.intermediates/cc_dep_static/android_arm64_armv8-a_static/cc_dep_static.a",
				"out/soong/.intermediates/cc_dep_shared/android_arm64_armv8-a_shared/cc_dep_shared.so",
				"out/soong/.intermediates/cc_dep_shared/android_arm64_armv8-a_shared/cc_dep_shared.so.toc",
			},
		},
		{
			name:       "rust_library with cc deps (rlib dylib-std)",
			moduleName: "libfizz_buzz_cc_deps",
			variant:    "android_arm64_armv8-a_rlib_dylib-std",
			rustcExpectedFilesToCopy: []string{
				"out/soong/.intermediates/cc_dep_static/android_arm64_armv8-a_static/cc_dep_static.a",
				"out/soong/.intermediates/cc_dep_shared/android_arm64_armv8-a_shared/cc_dep_shared.so",
				"out/soong/.intermediates/cc_dep_shared/android_arm64_armv8-a_shared/cc_dep_shared.so.toc",
			},
		},
		{
			name:       "rust_library with transitive cc deps (dylib)",
			moduleName: "libfizz_buzz_transitive_cc_deps",
			variant:    "android_arm64_armv8-a_dylib",
			rustcExpectedFilesToCopy: []string{
				"out/soong/.intermediates/cc_dep_static/android_arm64_armv8-a_static/cc_dep_static.a",
				"out/soong/.intermediates/cc_dep_shared/android_arm64_armv8-a_shared/cc_dep_shared.so",
				"out/soong/.intermediates/cc_dep_shared/android_arm64_armv8-a_shared/cc_dep_shared.so.toc",
			},
		},
		{
			name:       "rust_library with transitive cc deps (rlib rlib-std)",
			moduleName: "libfizz_buzz_transitive_cc_deps",
			variant:    "android_arm64_armv8-a_rlib_rlib-std",
			rustcExpectedFilesToCopy: []string{
				"out/soong/.intermediates/cc_dep_static/android_arm64_armv8-a_static/cc_dep_static.a",
				"out/soong/.intermediates/cc_dep_shared/android_arm64_armv8-a_shared/cc_dep_shared.so",
				"out/soong/.intermediates/cc_dep_shared/android_arm64_armv8-a_shared/cc_dep_shared.so.toc",
			},
		},
		{
			name:       "rust_library with transitive cc deps (rlib dylib-std)",
			moduleName: "libfizz_buzz_transitive_cc_deps",
			variant:    "android_arm64_armv8-a_rlib_dylib-std",
			rustcExpectedFilesToCopy: []string{
				"out/soong/.intermediates/cc_dep_static/android_arm64_armv8-a_static/cc_dep_static.a",
				"out/soong/.intermediates/cc_dep_shared/android_arm64_armv8-a_shared/cc_dep_shared.so",
				"out/soong/.intermediates/cc_dep_shared/android_arm64_armv8-a_shared/cc_dep_shared.so.toc",
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			writeFile := ctx.ModuleForTests(tc.moduleName, tc.variant).Rule("unescapedWriteFile")
			contents := writeFile.BuildParams.Args["content"]
			manifestProto := sbox_proto.Manifest{}
			err := prototext.Unmarshal([]byte(contents), &manifestProto)
			if err != nil {
				t.Errorf("expected no errors unmarshaling manifest proto; got %v", err)
			}

			if len(manifestProto.Commands) != 1 {
				t.Errorf("expected 1 command; got %v", len(manifestProto.Commands))
			}

			// check that sandbox contains correct files
			rustc := manifestProto.Commands[0]
			actualFilesToCopy := []string{}
			for _, copy := range rustc.CopyBefore {
				actualFilesToCopy = append(actualFilesToCopy, copy.GetFrom())
			}
			_, expectedFilesNotCopied, _ := android.ListSetDifference(tc.rustcExpectedFilesToCopy, actualFilesToCopy)
			if len(expectedFilesNotCopied) > 0 {
				t.Errorf("did not copy expected files to sbox: %v;\n files copied: %v", expectedFilesNotCopied, actualFilesToCopy)
			}

			rustcCmd := proptools.String(rustc.Command)
			for _, flag := range tc.expectedFlags {
				android.AssertStringDoesContain(
					t,
					fmt.Sprintf(
						"missing flag in rustc invocation; expected to find substring %q; got %q",
						flag,
						rustcCmd,
					),
					rustcCmd,
					flag,
				)
			}
		})
	}
}
