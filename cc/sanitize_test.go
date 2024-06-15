// Copyright 2021 Google Inc. All rights reserved.
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

package cc

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"android/soong/android"
)

var prepareForAsanTest = android.FixtureAddFile("asan/Android.bp", []byte(`
	cc_library_shared {
		name: "libclang_rt.asan",
		host_supported: true,
	}
	cc_library_static {
		name: "libclang_rt.asan.static",
		host_supported: true,
	}
	cc_library_static {
		name: "libclang_rt.asan_cxx.static",
		host_supported: true,
	}
`))

var prepareForTsanTest = android.FixtureAddFile("tsan/Android.bp", []byte(`
	cc_library_shared {
		name: "libclang_rt.tsan",
		host_supported: true,
	}
`))

type providerInterface interface {
	android.SingletonModuleProviderContext
}

// expectSharedLinkDep verifies that the from module links against the to module as a
// shared library.
func expectSharedLinkDep(t *testing.T, ctx providerInterface, from, to android.TestingModule) {
	t.Helper()
	fromLink := from.Description("link")
	toInfo, _ := android.SingletonModuleProvider(ctx, to.Module(), SharedLibraryInfoProvider)

	if g, w := fromLink.OrderOnly.Strings(), toInfo.SharedLibrary.RelativeToTop().String(); !android.InList(w, g) {
		t.Errorf("%s should link against %s, expected %q, got %q",
			from.Module(), to.Module(), w, g)
	}
}

// expectNoSharedLinkDep verifies that the from module links against the to module as a
// shared library.
func expectNoSharedLinkDep(t *testing.T, ctx providerInterface, from, to android.TestingModule) {
	t.Helper()
	fromLink := from.Description("link")
	toInfo, _ := android.SingletonModuleProvider(ctx, to.Module(), SharedLibraryInfoProvider)

	if g, w := fromLink.OrderOnly.Strings(), toInfo.SharedLibrary.RelativeToTop().String(); android.InList(w, g) {
		t.Errorf("%s should not link against %s, expected %q, got %q",
			from.Module(), to.Module(), w, g)
	}
}

// expectStaticLinkDep verifies that the from module links against the to module as a
// static library.
func expectStaticLinkDep(t *testing.T, ctx providerInterface, from, to android.TestingModule) {
	t.Helper()
	fromLink := from.Description("link")
	toInfo, _ := android.SingletonModuleProvider(ctx, to.Module(), StaticLibraryInfoProvider)

	if g, w := fromLink.Implicits.Strings(), toInfo.StaticLibrary.RelativeToTop().String(); !android.InList(w, g) {
		t.Errorf("%s should link against %s, expected %q, got %q",
			from.Module(), to.Module(), w, g)
	}

}

// expectNoStaticLinkDep verifies that the from module links against the to module as a
// static library.
func expectNoStaticLinkDep(t *testing.T, ctx providerInterface, from, to android.TestingModule) {
	t.Helper()
	fromLink := from.Description("link")
	toInfo, _ := android.SingletonModuleProvider(ctx, to.Module(), StaticLibraryInfoProvider)

	if g, w := fromLink.Implicits.Strings(), toInfo.StaticLibrary.RelativeToTop().String(); android.InList(w, g) {
		t.Errorf("%s should not link against %s, expected %q, got %q",
			from.Module(), to.Module(), w, g)
	}

}

// expectInstallDep verifies that the install rule of the from module depends on the
// install rule of the to module.
func expectInstallDep(t *testing.T, from, to android.TestingModule) {
	t.Helper()
	fromInstalled := from.Description("install")
	toInstalled := to.Description("install")

	// combine implicits and order-only dependencies, host uses implicit but device uses
	// order-only.
	got := append(fromInstalled.Implicits.Strings(), fromInstalled.OrderOnly.Strings()...)
	want := toInstalled.Output.String()
	if !android.InList(want, got) {
		t.Errorf("%s installation should depend on %s, expected %q, got %q",
			from.Module(), to.Module(), want, got)
	}
}

type expectedRuntimeLinkage int

const (
	RUNTIME_LINKAGE_NONE   = expectedRuntimeLinkage(0)
	RUNTIME_LINKAGE_SHARED = iota
	RUNTIME_LINKAGE_STATIC
)

func TestAsan(t *testing.T) {
	t.Parallel()
	bp := `
		cc_binary {
			name: "bin_with_asan",
			host_supported: true,
			shared_libs: [
				"libshared",
				"libasan",
			],
			static_libs: [
				"libstatic",
				"libnoasan",
				"libstatic_asan",
			],
			sanitize: {
				address: true,
			}
		}

		cc_binary {
			name: "bin_no_asan",
			host_supported: true,
			shared_libs: [
				"libshared",
				"libasan",
			],
			static_libs: [
				"libstatic",
				"libnoasan",
				"libstatic_asan",
			],
		}

		cc_library_shared {
			name: "libshared",
			host_supported: true,
			shared_libs: ["libtransitive"],
		}

		cc_library_shared {
			name: "libasan",
			host_supported: true,
			shared_libs: ["libtransitive"],
			sanitize: {
				address: true,
			}
		}

		cc_library_shared {
			name: "libtransitive",
			host_supported: true,
		}

		cc_library_static {
			name: "libstatic",
			host_supported: true,
		}

		cc_library_static {
			name: "libnoasan",
			host_supported: true,
			sanitize: {
				address: false,
			}
		}

		cc_library_static {
			name: "libstatic_asan",
			host_supported: true,
			sanitize: {
				address: true,
			}
		}

	`

	preparer := android.GroupFixturePreparers(
		prepareForCcTest,
		prepareForAsanTest,
	)
	buildOS := preparer.RunTestWithBp(t, bp).Config.BuildOSTarget.String()

	check := func(t *testing.T, variant string, runtimeLinkage expectedRuntimeLinkage, preparer android.FixturePreparer) {
		result := preparer.RunTestWithBp(t, bp)
		ctx := result.TestContext
		asanVariant := variant + "_asan"
		sharedVariant := variant + "_shared"
		sharedAsanVariant := sharedVariant + "_asan"
		staticVariant := variant + "_static"
		staticAsanVariant := staticVariant + "_asan"

		// The binaries, one with asan and one without
		binWithAsan := result.ModuleForTests("bin_with_asan", asanVariant)
		binNoAsan := result.ModuleForTests("bin_no_asan", variant)

		// Shared libraries that don't request asan
		libShared := result.ModuleForTests("libshared", sharedVariant)
		libTransitive := result.ModuleForTests("libtransitive", sharedVariant)

		// Shared library that requests asan
		libAsan := result.ModuleForTests("libasan", sharedAsanVariant)

		// Static library that uses an asan variant for bin_with_asan and a non-asan variant
		// for bin_no_asan.
		libStaticAsanVariant := result.ModuleForTests("libstatic", staticAsanVariant)
		libStaticNoAsanVariant := result.ModuleForTests("libstatic", staticVariant)

		// Static library that never uses asan.
		libNoAsan := result.ModuleForTests("libnoasan", staticVariant)

		// Static library that specifies asan
		libStaticAsan := result.ModuleForTests("libstatic_asan", staticAsanVariant)
		libStaticAsanNoAsanVariant := result.ModuleForTests("libstatic_asan", staticVariant)

		libAsanSharedRuntime := result.ModuleForTests("libclang_rt.asan", sharedVariant)
		libAsanStaticRuntime := result.ModuleForTests("libclang_rt.asan.static", staticVariant)
		libAsanStaticCxxRuntime := result.ModuleForTests("libclang_rt.asan_cxx.static", staticVariant)

		expectSharedLinkDep(t, ctx, binWithAsan, libShared)
		expectSharedLinkDep(t, ctx, binWithAsan, libAsan)
		expectSharedLinkDep(t, ctx, libShared, libTransitive)
		expectSharedLinkDep(t, ctx, libAsan, libTransitive)

		expectStaticLinkDep(t, ctx, binWithAsan, libStaticAsanVariant)
		expectStaticLinkDep(t, ctx, binWithAsan, libNoAsan)
		expectStaticLinkDep(t, ctx, binWithAsan, libStaticAsan)

		expectInstallDep(t, binWithAsan, libShared)
		expectInstallDep(t, binWithAsan, libAsan)
		expectInstallDep(t, binWithAsan, libTransitive)
		expectInstallDep(t, libShared, libTransitive)
		expectInstallDep(t, libAsan, libTransitive)

		expectSharedLinkDep(t, ctx, binNoAsan, libShared)
		expectSharedLinkDep(t, ctx, binNoAsan, libAsan)
		expectSharedLinkDep(t, ctx, libShared, libTransitive)
		expectSharedLinkDep(t, ctx, libAsan, libTransitive)

		expectStaticLinkDep(t, ctx, binNoAsan, libStaticNoAsanVariant)
		expectStaticLinkDep(t, ctx, binNoAsan, libNoAsan)
		expectStaticLinkDep(t, ctx, binNoAsan, libStaticAsanNoAsanVariant)

		expectInstallDep(t, binNoAsan, libShared)
		expectInstallDep(t, binNoAsan, libAsan)
		expectInstallDep(t, binNoAsan, libTransitive)
		expectInstallDep(t, libShared, libTransitive)
		expectInstallDep(t, libAsan, libTransitive)

		if runtimeLinkage == RUNTIME_LINKAGE_SHARED {
			expectSharedLinkDep(t, ctx, binWithAsan, libAsanSharedRuntime)
			expectNoSharedLinkDep(t, ctx, binNoAsan, libAsanSharedRuntime)
			expectSharedLinkDep(t, ctx, libAsan, libAsanSharedRuntime)
			expectNoSharedLinkDep(t, ctx, libShared, libAsanSharedRuntime)
			expectNoSharedLinkDep(t, ctx, libTransitive, libAsanSharedRuntime)
		} else {
			expectNoSharedLinkDep(t, ctx, binWithAsan, libAsanSharedRuntime)
			expectNoSharedLinkDep(t, ctx, binNoAsan, libAsanSharedRuntime)
			expectNoSharedLinkDep(t, ctx, libAsan, libAsanSharedRuntime)
			expectNoSharedLinkDep(t, ctx, libShared, libAsanSharedRuntime)
			expectNoSharedLinkDep(t, ctx, libTransitive, libAsanSharedRuntime)
		}

		if runtimeLinkage == RUNTIME_LINKAGE_STATIC {
			expectStaticLinkDep(t, ctx, binWithAsan, libAsanStaticRuntime)
			expectNoStaticLinkDep(t, ctx, binNoAsan, libAsanStaticRuntime)
			expectStaticLinkDep(t, ctx, libAsan, libAsanStaticRuntime)
			expectNoStaticLinkDep(t, ctx, libShared, libAsanStaticRuntime)
			expectNoStaticLinkDep(t, ctx, libTransitive, libAsanStaticRuntime)

			expectStaticLinkDep(t, ctx, binWithAsan, libAsanStaticCxxRuntime)
			expectNoStaticLinkDep(t, ctx, binNoAsan, libAsanStaticCxxRuntime)
			expectStaticLinkDep(t, ctx, libAsan, libAsanStaticCxxRuntime)
			expectNoStaticLinkDep(t, ctx, libShared, libAsanStaticCxxRuntime)
			expectNoStaticLinkDep(t, ctx, libTransitive, libAsanStaticCxxRuntime)
		} else {
			expectNoStaticLinkDep(t, ctx, binWithAsan, libAsanStaticRuntime)
			expectNoStaticLinkDep(t, ctx, binNoAsan, libAsanStaticRuntime)
			expectNoStaticLinkDep(t, ctx, libAsan, libAsanStaticRuntime)
			expectNoStaticLinkDep(t, ctx, libShared, libAsanStaticRuntime)
			expectNoStaticLinkDep(t, ctx, libTransitive, libAsanStaticRuntime)

			expectNoStaticLinkDep(t, ctx, binWithAsan, libAsanStaticCxxRuntime)
			expectNoStaticLinkDep(t, ctx, binNoAsan, libAsanStaticCxxRuntime)
			expectNoStaticLinkDep(t, ctx, libAsan, libAsanStaticCxxRuntime)
			expectNoStaticLinkDep(t, ctx, libShared, libAsanStaticCxxRuntime)
			expectNoStaticLinkDep(t, ctx, libTransitive, libAsanStaticCxxRuntime)
		}
	}

	t.Run("host", func(t *testing.T) { check(t, buildOS, RUNTIME_LINKAGE_NONE, preparer) })
	t.Run("device", func(t *testing.T) { check(t, "android_arm64_armv8-a", RUNTIME_LINKAGE_SHARED, preparer) })
	t.Run("host musl", func(t *testing.T) {
		check(t, "linux_musl_x86_64", RUNTIME_LINKAGE_STATIC,
			android.GroupFixturePreparers(preparer, PrepareForTestWithHostMusl))
	})
}

func TestTsan(t *testing.T) {
	t.Parallel()
	bp := `
	cc_binary {
		name: "bin_with_tsan",
		host_supported: true,
		shared_libs: [
			"libshared",
			"libtsan",
		],
		sanitize: {
			thread: true,
		}
	}

	cc_binary {
		name: "bin_no_tsan",
		host_supported: true,
		shared_libs: [
			"libshared",
			"libtsan",
		],
	}

	cc_library_shared {
		name: "libshared",
		host_supported: true,
		shared_libs: ["libtransitive"],
	}

	cc_library_shared {
		name: "libtsan",
		host_supported: true,
		shared_libs: ["libtransitive"],
		sanitize: {
			thread: true,
		}
	}

	cc_library_shared {
		name: "libtransitive",
		host_supported: true,
	}
`

	preparer := android.GroupFixturePreparers(
		prepareForCcTest,
		prepareForTsanTest,
	)
	buildOS := preparer.RunTestWithBp(t, bp).Config.BuildOSTarget.String()

	check := func(t *testing.T, variant string, preparer android.FixturePreparer) {
		result := preparer.RunTestWithBp(t, bp)
		ctx := result.TestContext
		tsanVariant := variant + "_tsan"
		sharedVariant := variant + "_shared"
		sharedTsanVariant := sharedVariant + "_tsan"

		// The binaries, one with tsan and one without
		binWithTsan := result.ModuleForTests("bin_with_tsan", tsanVariant)
		binNoTsan := result.ModuleForTests("bin_no_tsan", variant)

		// Shared libraries that don't request tsan
		libShared := result.ModuleForTests("libshared", sharedVariant)
		libTransitive := result.ModuleForTests("libtransitive", sharedVariant)

		// Shared library that requests tsan
		libTsan := result.ModuleForTests("libtsan", sharedTsanVariant)

		expectSharedLinkDep(t, ctx, binWithTsan, libShared)
		expectSharedLinkDep(t, ctx, binWithTsan, libTsan)
		expectSharedLinkDep(t, ctx, libShared, libTransitive)
		expectSharedLinkDep(t, ctx, libTsan, libTransitive)

		expectSharedLinkDep(t, ctx, binNoTsan, libShared)
		expectSharedLinkDep(t, ctx, binNoTsan, libTsan)
		expectSharedLinkDep(t, ctx, libShared, libTransitive)
		expectSharedLinkDep(t, ctx, libTsan, libTransitive)
	}

	t.Run("host", func(t *testing.T) { check(t, buildOS, preparer) })
	t.Run("device", func(t *testing.T) { check(t, "android_arm64_armv8-a", preparer) })
	t.Run("host musl", func(t *testing.T) {
		check(t, "linux_musl_x86_64", android.GroupFixturePreparers(preparer, PrepareForTestWithHostMusl))
	})
}

func TestMiscUndefined(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires linux")
	}

	t.Parallel()
	bp := `
	cc_binary {
		name: "bin_with_ubsan",
		srcs: ["src.cc"],
		host_supported: true,
		static_libs: [
			"libstatic",
			"libubsan",
		],
		sanitize: {
			misc_undefined: ["integer"],
		}
	}

	cc_binary {
		name: "bin_no_ubsan",
		host_supported: true,
		srcs: ["src.cc"],
		static_libs: [
			"libstatic",
			"libubsan",
		],
	}

	cc_library_static {
		name: "libstatic",
		host_supported: true,
		srcs: ["src.cc"],
		static_libs: ["libtransitive"],
	}

	cc_library_static {
		name: "libubsan",
		host_supported: true,
		srcs: ["src.cc"],
		whole_static_libs: ["libtransitive"],
		sanitize: {
			misc_undefined: ["integer"],
		}
	}

	cc_library_static {
		name: "libtransitive",
		host_supported: true,
		srcs: ["src.cc"],
	}
`

	preparer := android.GroupFixturePreparers(
		prepareForCcTest,
	)
	buildOS := preparer.RunTestWithBp(t, bp).Config.BuildOSTarget.String()

	check := func(t *testing.T, variant string, preparer android.FixturePreparer) {
		result := preparer.RunTestWithBp(t, bp)
		ctx := result.TestContext
		staticVariant := variant + "_static"

		// The binaries, one with ubsan and one without
		binWithUbsan := result.ModuleForTests("bin_with_ubsan", variant)
		binNoUbsan := result.ModuleForTests("bin_no_ubsan", variant)

		// Static libraries that don't request ubsan
		libStatic := result.ModuleForTests("libstatic", staticVariant)
		libTransitive := result.ModuleForTests("libtransitive", staticVariant)

		libUbsan := result.ModuleForTests("libubsan", staticVariant)

		libUbsanMinimal := result.ModuleForTests("libclang_rt.ubsan_minimal", staticVariant)

		expectStaticLinkDep(t, ctx, binWithUbsan, libStatic)
		expectStaticLinkDep(t, ctx, binWithUbsan, libUbsan)
		expectStaticLinkDep(t, ctx, binWithUbsan, libUbsanMinimal)

		miscUndefinedSanFlag := "-fsanitize=integer"
		binWithUbsanCflags := binWithUbsan.Rule("cc").Args["cFlags"]
		if !strings.Contains(binWithUbsanCflags, miscUndefinedSanFlag) {
			t.Errorf("'bin_with_ubsan' Expected %q to be in flags %q, was not", miscUndefinedSanFlag, binWithUbsanCflags)
		}
		libStaticCflags := libStatic.Rule("cc").Args["cFlags"]
		if strings.Contains(libStaticCflags, miscUndefinedSanFlag) {
			t.Errorf("'libstatic' Expected %q to NOT be in flags %q, was", miscUndefinedSanFlag, binWithUbsanCflags)
		}
		libUbsanCflags := libUbsan.Rule("cc").Args["cFlags"]
		if !strings.Contains(libUbsanCflags, miscUndefinedSanFlag) {
			t.Errorf("'libubsan' Expected %q to be in flags %q, was not", miscUndefinedSanFlag, binWithUbsanCflags)
		}
		libTransitiveCflags := libTransitive.Rule("cc").Args["cFlags"]
		if strings.Contains(libTransitiveCflags, miscUndefinedSanFlag) {
			t.Errorf("'libtransitive': Expected %q to NOT be in flags %q, was", miscUndefinedSanFlag, binWithUbsanCflags)
		}

		expectStaticLinkDep(t, ctx, binNoUbsan, libStatic)
		expectStaticLinkDep(t, ctx, binNoUbsan, libUbsan)
	}

	t.Run("host", func(t *testing.T) { check(t, buildOS, preparer) })
	t.Run("device", func(t *testing.T) { check(t, "android_arm64_armv8-a", preparer) })
	t.Run("host musl", func(t *testing.T) {
		check(t, "linux_musl_x86_64", android.GroupFixturePreparers(preparer, PrepareForTestWithHostMusl))
	})
}

func TestFuzz(t *testing.T) {
	t.Parallel()
	bp := `
		cc_binary {
			name: "bin_with_fuzzer",
			host_supported: true,
			shared_libs: [
				"libshared",
				"libfuzzer",
			],
			static_libs: [
				"libstatic",
				"libnofuzzer",
				"libstatic_fuzzer",
			],
			sanitize: {
				fuzzer: true,
			}
		}

		cc_binary {
			name: "bin_no_fuzzer",
			host_supported: true,
			shared_libs: [
				"libshared",
				"libfuzzer",
			],
			static_libs: [
				"libstatic",
				"libnofuzzer",
				"libstatic_fuzzer",
			],
		}

		cc_library_shared {
			name: "libshared",
			host_supported: true,
			shared_libs: ["libtransitive"],
		}

		cc_library_shared {
			name: "libfuzzer",
			host_supported: true,
			shared_libs: ["libtransitive"],
			sanitize: {
				fuzzer: true,
			}
		}

		cc_library_shared {
			name: "libtransitive",
			host_supported: true,
		}

		cc_library_static {
			name: "libstatic",
			host_supported: true,
		}

		cc_library_static {
			name: "libnofuzzer",
			host_supported: true,
			sanitize: {
				fuzzer: false,
			}
		}

		cc_library_static {
			name: "libstatic_fuzzer",
			host_supported: true,
		}

	`

	result := android.GroupFixturePreparers(
		prepareForCcTest,
	).RunTestWithBp(t, bp)

	check := func(t *testing.T, result *android.TestResult, variant string) {
		ctx := result.TestContext
		fuzzerVariant := variant + "_fuzzer"
		sharedVariant := variant + "_shared"
		sharedFuzzerVariant := sharedVariant + "_fuzzer"
		staticVariant := variant + "_static"
		staticFuzzerVariant := staticVariant + "_fuzzer"

		// The binaries, one with fuzzer and one without
		binWithFuzzer := result.ModuleForTests("bin_with_fuzzer", fuzzerVariant)
		binNoFuzzer := result.ModuleForTests("bin_no_fuzzer", variant)

		// Shared libraries that don't request fuzzer
		libShared := result.ModuleForTests("libshared", sharedVariant)
		libTransitive := result.ModuleForTests("libtransitive", sharedVariant)

		// Shared libraries that don't request fuzzer
		libSharedFuzzer := result.ModuleForTests("libshared", sharedFuzzerVariant)
		libTransitiveFuzzer := result.ModuleForTests("libtransitive", sharedFuzzerVariant)

		// Shared library that requests fuzzer
		libFuzzer := result.ModuleForTests("libfuzzer", sharedFuzzerVariant)

		// Static library that uses an fuzzer variant for bin_with_fuzzer and a non-fuzzer variant
		// for bin_no_fuzzer.
		libStaticFuzzerVariant := result.ModuleForTests("libstatic", staticFuzzerVariant)
		libStaticNoFuzzerVariant := result.ModuleForTests("libstatic", staticVariant)

		// Static library that never uses fuzzer.
		libNoFuzzer := result.ModuleForTests("libnofuzzer", staticVariant)

		// Static library that specifies fuzzer
		libStaticFuzzer := result.ModuleForTests("libstatic_fuzzer", staticFuzzerVariant)
		libStaticFuzzerNoFuzzerVariant := result.ModuleForTests("libstatic_fuzzer", staticVariant)

		expectSharedLinkDep(t, ctx, binWithFuzzer, libSharedFuzzer)
		expectSharedLinkDep(t, ctx, binWithFuzzer, libFuzzer)
		expectSharedLinkDep(t, ctx, libSharedFuzzer, libTransitiveFuzzer)
		expectSharedLinkDep(t, ctx, libFuzzer, libTransitiveFuzzer)

		expectStaticLinkDep(t, ctx, binWithFuzzer, libStaticFuzzerVariant)
		expectStaticLinkDep(t, ctx, binWithFuzzer, libNoFuzzer)
		expectStaticLinkDep(t, ctx, binWithFuzzer, libStaticFuzzer)

		expectSharedLinkDep(t, ctx, binNoFuzzer, libShared)
		expectSharedLinkDep(t, ctx, binNoFuzzer, libFuzzer)
		expectSharedLinkDep(t, ctx, libShared, libTransitive)
		expectSharedLinkDep(t, ctx, libFuzzer, libTransitiveFuzzer)

		expectStaticLinkDep(t, ctx, binNoFuzzer, libStaticNoFuzzerVariant)
		expectStaticLinkDep(t, ctx, binNoFuzzer, libNoFuzzer)
		expectStaticLinkDep(t, ctx, binNoFuzzer, libStaticFuzzerNoFuzzerVariant)
	}

	t.Run("device", func(t *testing.T) { check(t, result, "android_arm64_armv8-a") })
}

func TestUbsan(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("requires linux")
	}

	bp := `
		cc_binary {
			name: "bin_with_ubsan",
			host_supported: true,
			shared_libs: [
				"libshared",
			],
			static_libs: [
				"libstatic",
				"libnoubsan",
			],
			sanitize: {
				undefined: true,
			}
		}

		cc_binary {
			name: "bin_depends_ubsan_static",
			host_supported: true,
			shared_libs: [
				"libshared",
			],
			static_libs: [
				"libstatic",
				"libubsan",
				"libnoubsan",
			],
		}

		cc_binary {
			name: "bin_depends_ubsan_shared",
			host_supported: true,
			shared_libs: [
				"libsharedubsan",
			],
		}

		cc_binary {
			name: "bin_no_ubsan",
			host_supported: true,
			shared_libs: [
				"libshared",
			],
			static_libs: [
				"libstatic",
				"libnoubsan",
			],
		}

		cc_binary {
			name: "static_bin_with_ubsan_dep",
			static_executable: true,
			host_supported: true,
			static_libs: [
				"libubsan_diag",
			],
		}

		cc_library_shared {
			name: "libshared",
			host_supported: true,
			shared_libs: ["libtransitive"],
		}

		cc_library_shared {
			name: "libtransitive",
			host_supported: true,
		}

		cc_library_shared {
			name: "libsharedubsan",
			host_supported: true,
			sanitize: {
				undefined: true,
			}
		}

		cc_library_static {
			name: "libubsan",
			host_supported: true,
			sanitize: {
				undefined: true,
			}
		}

		cc_library_static {
			name: "libubsan_diag",
			host_supported: true,
			sanitize: {
				undefined: true,
				diag: {
					undefined: true,
				},
			},
		}

		cc_library_static {
			name: "libstatic",
			host_supported: true,
		}

		cc_library_static {
			name: "libnoubsan",
			host_supported: true,
		}
	`

	preparer := android.GroupFixturePreparers(
		prepareForCcTest,
	)
	buildOS := preparer.RunTestWithBp(t, bp).Config.BuildOSTarget.String()

	check := func(t *testing.T, variant string, preparer android.FixturePreparer) {
		result := preparer.RunTestWithBp(t, bp)
		staticVariant := variant + "_static"
		sharedVariant := variant + "_shared"

		minimalRuntime := result.ModuleForTests("libclang_rt.ubsan_minimal", staticVariant)
		standaloneRuntime := result.ModuleForTests("libclang_rt.ubsan_standalone.static", staticVariant)

		// The binaries, one with ubsan and one without
		binWithUbsan := result.ModuleForTests("bin_with_ubsan", variant)
		binDependsUbsan := result.ModuleForTests("bin_depends_ubsan_static", variant)
		libSharedUbsan := result.ModuleForTests("libsharedubsan", sharedVariant)
		binDependsUbsanShared := result.ModuleForTests("bin_depends_ubsan_shared", variant)
		binNoUbsan := result.ModuleForTests("bin_no_ubsan", variant)
		staticBin := result.ModuleForTests("static_bin_with_ubsan_dep", variant)

		android.AssertStringListContains(t, "missing libclang_rt.ubsan_minimal in bin_with_ubsan static libs",
			strings.Split(binWithUbsan.Rule("ld").Args["libFlags"], " "),
			minimalRuntime.OutputFiles(t, "")[0].String())

		android.AssertStringListContains(t, "missing libclang_rt.ubsan_minimal in bin_depends_ubsan_static static libs",
			strings.Split(binDependsUbsan.Rule("ld").Args["libFlags"], " "),
			minimalRuntime.OutputFiles(t, "")[0].String())

		android.AssertStringListContains(t, "missing libclang_rt.ubsan_minimal in libsharedubsan static libs",
			strings.Split(libSharedUbsan.Rule("ld").Args["libFlags"], " "),
			minimalRuntime.OutputFiles(t, "")[0].String())

		android.AssertStringListDoesNotContain(t, "unexpected libclang_rt.ubsan_minimal in bin_depends_ubsan_shared static libs",
			strings.Split(binDependsUbsanShared.Rule("ld").Args["libFlags"], " "),
			minimalRuntime.OutputFiles(t, "")[0].String())

		android.AssertStringListDoesNotContain(t, "unexpected libclang_rt.ubsan_minimal in bin_no_ubsan static libs",
			strings.Split(binNoUbsan.Rule("ld").Args["libFlags"], " "),
			minimalRuntime.OutputFiles(t, "")[0].String())

		android.AssertStringListContains(t, "missing -Wl,--exclude-libs for minimal runtime in bin_with_ubsan",
			strings.Split(binWithUbsan.Rule("ld").Args["ldFlags"], " "),
			"-Wl,--exclude-libs="+minimalRuntime.OutputFiles(t, "")[0].Base())

		android.AssertStringListContains(t, "missing -Wl,--exclude-libs for minimal runtime in bin_depends_ubsan_static static libs",
			strings.Split(binDependsUbsan.Rule("ld").Args["ldFlags"], " "),
			"-Wl,--exclude-libs="+minimalRuntime.OutputFiles(t, "")[0].Base())

		android.AssertStringListContains(t, "missing -Wl,--exclude-libs for minimal runtime in libsharedubsan static libs",
			strings.Split(libSharedUbsan.Rule("ld").Args["ldFlags"], " "),
			"-Wl,--exclude-libs="+minimalRuntime.OutputFiles(t, "")[0].Base())

		android.AssertStringListDoesNotContain(t, "unexpected -Wl,--exclude-libs for minimal runtime in bin_depends_ubsan_shared static libs",
			strings.Split(binDependsUbsanShared.Rule("ld").Args["ldFlags"], " "),
			"-Wl,--exclude-libs="+minimalRuntime.OutputFiles(t, "")[0].Base())

		android.AssertStringListDoesNotContain(t, "unexpected -Wl,--exclude-libs for minimal runtime in bin_no_ubsan static libs",
			strings.Split(binNoUbsan.Rule("ld").Args["ldFlags"], " "),
			"-Wl,--exclude-libs="+minimalRuntime.OutputFiles(t, "")[0].Base())

		android.AssertStringListContains(t, "missing libclang_rt.ubsan_standalone.static in static_bin_with_ubsan_dep static libs",
			strings.Split(staticBin.Rule("ld").Args["libFlags"], " "),
			standaloneRuntime.OutputFiles(t, "")[0].String())

	}

	t.Run("host", func(t *testing.T) { check(t, buildOS, preparer) })
	t.Run("device", func(t *testing.T) { check(t, "android_arm64_armv8-a", preparer) })
	t.Run("host musl", func(t *testing.T) {
		check(t, "linux_musl_x86_64", android.GroupFixturePreparers(preparer, PrepareForTestWithHostMusl))
	})
}

type MemtagNoteType int

const (
	None MemtagNoteType = iota + 1
	Sync
	Async
)

func (t MemtagNoteType) str() string {
	switch t {
	case None:
		return "none"
	case Sync:
		return "sync"
	case Async:
		return "async"
	default:
		panic("type_note_invalid")
	}
}

func checkHasMemtagNote(t *testing.T, m android.TestingModule, expected MemtagNoteType) {
	t.Helper()

	found := None
	ldFlags := m.Rule("ld").Args["ldFlags"]
	if strings.Contains(ldFlags, "-fsanitize-memtag-mode=async") {
		found = Async
	} else if strings.Contains(ldFlags, "-fsanitize-memtag-mode=sync") {
		found = Sync
	}

	if found != expected {
		t.Errorf("Wrong Memtag note in target %q: found %q, expected %q", m.Module().(*Module).Name(), found.str(), expected.str())
	}
}

var prepareForTestWithMemtagHeap = android.GroupFixturePreparers(
	android.FixtureModifyMockFS(func(fs android.MockFS) {
		templateBp := `
		cc_test {
			name: "unset_test_%[1]s",
			gtest: false,
		}

		cc_test {
			name: "no_memtag_test_%[1]s",
			gtest: false,
			sanitize: { memtag_heap: false },
		}

		cc_test {
			name: "set_memtag_test_%[1]s",
			gtest: false,
			sanitize: { memtag_heap: true },
		}

		cc_test {
			name: "set_memtag_set_async_test_%[1]s",
			gtest: false,
			sanitize: { memtag_heap: true, diag: { memtag_heap: false }  },
		}

		cc_test {
			name: "set_memtag_set_sync_test_%[1]s",
			gtest: false,
			sanitize: { memtag_heap: true, diag: { memtag_heap: true }  },
		}

		cc_test {
			name: "unset_memtag_set_sync_test_%[1]s",
			gtest: false,
			sanitize: { diag: { memtag_heap: true }  },
		}

		cc_binary {
			name: "unset_binary_%[1]s",
		}

		cc_binary {
			name: "no_memtag_binary_%[1]s",
			sanitize: { memtag_heap: false },
		}

		cc_binary {
			name: "set_memtag_binary_%[1]s",
			sanitize: { memtag_heap: true },
		}

		cc_binary {
			name: "set_memtag_set_async_binary_%[1]s",
			sanitize: { memtag_heap: true, diag: { memtag_heap: false }  },
		}

		cc_binary {
			name: "set_memtag_set_sync_binary_%[1]s",
			sanitize: { memtag_heap: true, diag: { memtag_heap: true }  },
		}

		cc_binary {
			name: "unset_memtag_set_sync_binary_%[1]s",
			sanitize: { diag: { memtag_heap: true }  },
		}
		`
		subdirNoOverrideBp := fmt.Sprintf(templateBp, "no_override")
		subdirOverrideDefaultDisableBp := fmt.Sprintf(templateBp, "override_default_disable")
		subdirSyncBp := fmt.Sprintf(templateBp, "override_default_sync")
		subdirAsyncBp := fmt.Sprintf(templateBp, "override_default_async")

		fs.Merge(android.MockFS{
			"subdir_no_override/Android.bp":              []byte(subdirNoOverrideBp),
			"subdir_override_default_disable/Android.bp": []byte(subdirOverrideDefaultDisableBp),
			"subdir_sync/Android.bp":                     []byte(subdirSyncBp),
			"subdir_async/Android.bp":                    []byte(subdirAsyncBp),
		})
	}),
	android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
		variables.MemtagHeapExcludePaths = []string{"subdir_override_default_disable"}
		// "subdir_override_default_disable" is covered by both include and override_default_disable paths. override_default_disable wins.
		variables.MemtagHeapSyncIncludePaths = []string{"subdir_sync", "subdir_override_default_disable"}
		variables.MemtagHeapAsyncIncludePaths = []string{"subdir_async", "subdir_override_default_disable"}
	}),
)

func TestSanitizeMemtagHeap(t *testing.T) {
	t.Parallel()
	variant := "android_arm64_armv8-a"

	result := android.GroupFixturePreparers(
		prepareForCcTest,
		prepareForTestWithMemtagHeap,
	).RunTest(t)
	ctx := result.TestContext

	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_no_override", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_async", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_sync", variant), None)

	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_no_override", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_async", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_sync", variant), None)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_disable", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_disable", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_sync", variant), Async)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_disable", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_sync", variant), Async)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_sync", variant), Sync)

	// should sanitize: { diag: { memtag: true } } result in Sync instead of None here?
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_no_override", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_async", variant), Sync)
	// should sanitize: { diag: { memtag: true } } result in Sync instead of None here?
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_no_override", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_sync", variant), Sync)
}

func TestSanitizeMemtagHeapWithSanitizeDevice(t *testing.T) {
	t.Parallel()
	variant := "android_arm64_armv8-a"

	result := android.GroupFixturePreparers(
		prepareForCcTest,
		prepareForTestWithMemtagHeap,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.SanitizeDevice = []string{"memtag_heap"}
		}),
	).RunTest(t)
	ctx := result.TestContext

	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_no_override", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_async", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_sync", variant), None)

	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_no_override", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_async", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_sync", variant), None)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_disable", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_disable", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_sync", variant), Async)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_disable", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_sync", variant), Async)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_async", variant), Sync)
	// should sanitize: { diag: { memtag: true } } result in Sync instead of None here?
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_sync", variant), Sync)
}

func TestSanitizeMemtagHeapWithSanitizeDeviceDiag(t *testing.T) {
	t.Parallel()
	variant := "android_arm64_armv8-a"

	result := android.GroupFixturePreparers(
		prepareForCcTest,
		prepareForTestWithMemtagHeap,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.SanitizeDevice = []string{"memtag_heap"}
			variables.SanitizeDeviceDiag = []string{"memtag_heap"}
		}),
	).RunTest(t)
	ctx := result.TestContext

	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_no_override", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_async", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_sync", variant), None)

	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_no_override", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_async", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_sync", variant), None)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_disable", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_sync", variant), Async)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_disable", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_sync", variant), Async)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_async", variant), Sync)
	// should sanitize: { diag: { memtag: true } } result in Sync instead of None here?
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_sync", variant), Sync)
}

func TestCfi(t *testing.T) {
	t.Parallel()

	bp := `
	cc_library_shared {
		name: "shared_with_cfi",
		static_libs: [
			"static_dep_with_cfi",
			"static_dep_no_cfi",
		],
		sanitize: {
			cfi: true,
		},
	}

	cc_library_shared {
		name: "shared_no_cfi",
		static_libs: [
			"static_dep_with_cfi",
			"static_dep_no_cfi",
		],
	}

	cc_library_static {
		name: "static_dep_with_cfi",
		sanitize: {
			cfi: true,
		},
	}

	cc_library_static {
		name: "static_dep_no_cfi",
	}

	cc_library_shared {
		name: "shared_rdep_no_cfi",
		static_libs: ["static_dep_with_cfi_2"],
	}

	cc_library_static {
		name: "static_dep_with_cfi_2",
		sanitize: {
			cfi: true,
		},
	}
`
	preparer := android.GroupFixturePreparers(
		prepareForCcTest,
	)
	result := preparer.RunTestWithBp(t, bp)
	ctx := result.TestContext

	buildOs := "android_arm64_armv8-a"
	shared_suffix := "_shared"
	cfi_suffix := "_cfi"
	static_suffix := "_static"

	sharedWithCfiLib := result.ModuleForTests("shared_with_cfi", buildOs+shared_suffix+cfi_suffix)
	sharedNoCfiLib := result.ModuleForTests("shared_no_cfi", buildOs+shared_suffix)
	staticWithCfiLib := result.ModuleForTests("static_dep_with_cfi", buildOs+static_suffix)
	staticWithCfiLibCfiVariant := result.ModuleForTests("static_dep_with_cfi", buildOs+static_suffix+cfi_suffix)
	staticNoCfiLib := result.ModuleForTests("static_dep_no_cfi", buildOs+static_suffix)
	staticNoCfiLibCfiVariant := result.ModuleForTests("static_dep_no_cfi", buildOs+static_suffix+cfi_suffix)
	sharedRdepNoCfi := result.ModuleForTests("shared_rdep_no_cfi", buildOs+shared_suffix)
	staticDepWithCfi2Lib := result.ModuleForTests("static_dep_with_cfi_2", buildOs+static_suffix)

	// Confirm assumptions about propagation of CFI enablement
	expectStaticLinkDep(t, ctx, sharedWithCfiLib, staticWithCfiLibCfiVariant)
	expectStaticLinkDep(t, ctx, sharedNoCfiLib, staticWithCfiLib)
	expectStaticLinkDep(t, ctx, sharedWithCfiLib, staticNoCfiLibCfiVariant)
	expectStaticLinkDep(t, ctx, sharedNoCfiLib, staticNoCfiLib)
	expectStaticLinkDep(t, ctx, sharedRdepNoCfi, staticDepWithCfi2Lib)

	// Confirm that non-CFI variants do not add CFI flags
	bazLibCflags := staticWithCfiLib.Rule("cc").Args["cFlags"]
	if strings.Contains(bazLibCflags, "-fsanitize-cfi-cross-dso") {
		t.Errorf("non-CFI variant of baz not expected to contain CFI flags ")
	}
}

func TestHwasan(t *testing.T) {
	t.Parallel()

	bp := `
	cc_library_shared {
		name: "shared_with_hwaddress",
		static_libs: [
			"static_dep_with_hwaddress",
			"static_dep_no_hwaddress",
		],
		sanitize: {
			hwaddress: true,
		},
		sdk_version: "current",
			stl: "c++_shared",
	}

	cc_library_static {
		name: "static_dep_with_hwaddress",
		sanitize: {
			hwaddress: true,
		},
		sdk_version: "current",
			stl: "c++_shared",
	}

	cc_library_static {
		name: "static_dep_no_hwaddress",
		sdk_version: "current",
			stl: "c++_shared",
	}
`

	androidArm := "android_arm_armv7-a-neon"
	androidArm64 := "android_arm64_armv8-a"
	androidX86 := "android_x86_silvermont"
	sharedSuffix := "_shared"
	hwasanSuffix := "_hwasan"
	staticSuffix := "_static"
	sdkSuffix := "_sdk"

	sharedWithHwasanVariant := sharedSuffix + hwasanSuffix
	sharedWithSdkVariant := sdkSuffix + sharedSuffix
	staticWithHwasanVariant := staticSuffix + hwasanSuffix
	staticWithSdkVariant := sdkSuffix + staticSuffix

	testCases := []struct {
		buildOs          string
		extraPreparer    android.FixturePreparer
		expectedVariants map[string][]string
	}{
		{
			buildOs: androidArm64,
			expectedVariants: map[string][]string{
				"shared_with_hwaddress": []string{
					androidArm64 + sharedWithHwasanVariant,
					androidArm64 + sharedWithSdkVariant,
					androidArm + sharedSuffix,
					androidArm + sharedWithSdkVariant,
				},
				"static_dep_with_hwaddress": []string{
					androidArm64 + staticSuffix,
					androidArm64 + staticWithHwasanVariant,
					androidArm64 + staticWithSdkVariant,
					androidArm + staticSuffix,
					androidArm + staticWithSdkVariant,
				},
				"static_dep_no_hwaddress": []string{
					androidArm64 + staticSuffix,
					androidArm64 + staticWithHwasanVariant,
					androidArm64 + staticWithSdkVariant,
					androidArm + staticSuffix,
					androidArm + staticWithSdkVariant,
				},
			},
		},
		{
			buildOs: androidX86,
			extraPreparer: android.FixtureModifyConfig(func(config android.Config) {
				config.Targets[android.Android] = []android.Target{
					{
						android.Android,
						android.Arch{
							ArchType: android.X86, ArchVariant: "silvermont", Abi: []string{"armeabi-v7a"}}, android.NativeBridgeDisabled, "", "", false},
				}
			}),
			expectedVariants: map[string][]string{
				"shared_with_hwaddress": []string{
					androidX86 + sharedSuffix,
					androidX86 + sharedWithSdkVariant,
				},
				"static_dep_with_hwaddress": []string{
					androidX86 + staticSuffix,
					androidX86 + staticWithSdkVariant,
				},
				"static_dep_no_hwaddress": []string{
					androidX86 + staticSuffix,
					androidX86 + staticWithSdkVariant,
				},
			},
		},
	}

	for _, tc := range testCases {
		preparer := android.GroupFixturePreparers(
			prepareForCcTest,
			android.OptionalFixturePreparer(tc.extraPreparer),
		)
		result := preparer.RunTestWithBp(t, bp)

		for m, v := range tc.expectedVariants {
			variants := result.ModuleVariantsForTests(m)
			if !reflect.DeepEqual(variants, v) {
				t.Errorf("Expected variants of %q to be %q, but got %q", m, v, variants)
			}
		}
	}
}
