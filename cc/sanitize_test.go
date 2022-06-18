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
	"runtime"
	"strings"
	"testing"

	"android/soong/android"
)

var prepareForAsanTest = android.FixtureAddFile("asan/Android.bp", []byte(`
	cc_library_shared {
		name: "libclang_rt.asan",
	}
`))

func TestAsan(t *testing.T) {
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
	`

	result := android.GroupFixturePreparers(
		prepareForCcTest,
		prepareForAsanTest,
	).RunTestWithBp(t, bp)

	check := func(t *testing.T, result *android.TestResult, variant string) {
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

		// expectSharedLinkDep verifies that the from module links against the to module as a
		// shared library.
		expectSharedLinkDep := func(from, to android.TestingModule) {
			t.Helper()
			fromLink := from.Description("link")
			toLink := to.Description("strip")

			if g, w := fromLink.OrderOnly.Strings(), toLink.Output.String(); !android.InList(w, g) {
				t.Errorf("%s should link against %s, expected %q, got %q",
					from.Module(), to.Module(), w, g)
			}
		}

		// expectStaticLinkDep verifies that the from module links against the to module as a
		// static library.
		expectStaticLinkDep := func(from, to android.TestingModule) {
			t.Helper()
			fromLink := from.Description("link")
			toLink := to.Description("static link")

			if g, w := fromLink.Implicits.Strings(), toLink.Output.String(); !android.InList(w, g) {
				t.Errorf("%s should link against %s, expected %q, got %q",
					from.Module(), to.Module(), w, g)
			}

		}

		// expectInstallDep verifies that the install rule of the from module depends on the
		// install rule of the to module.
		expectInstallDep := func(from, to android.TestingModule) {
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

		expectSharedLinkDep(binWithAsan, libShared)
		expectSharedLinkDep(binWithAsan, libAsan)
		expectSharedLinkDep(libShared, libTransitive)
		expectSharedLinkDep(libAsan, libTransitive)

		expectStaticLinkDep(binWithAsan, libStaticAsanVariant)
		expectStaticLinkDep(binWithAsan, libNoAsan)

		expectInstallDep(binWithAsan, libShared)
		expectInstallDep(binWithAsan, libAsan)
		expectInstallDep(binWithAsan, libTransitive)
		expectInstallDep(libShared, libTransitive)
		expectInstallDep(libAsan, libTransitive)

		expectSharedLinkDep(binNoAsan, libShared)
		expectSharedLinkDep(binNoAsan, libAsan)
		expectSharedLinkDep(libShared, libTransitive)
		expectSharedLinkDep(libAsan, libTransitive)

		expectStaticLinkDep(binNoAsan, libStaticNoAsanVariant)
		expectStaticLinkDep(binNoAsan, libNoAsan)

		expectInstallDep(binNoAsan, libShared)
		expectInstallDep(binNoAsan, libAsan)
		expectInstallDep(binNoAsan, libTransitive)
		expectInstallDep(libShared, libTransitive)
		expectInstallDep(libAsan, libTransitive)
	}

	t.Run("host", func(t *testing.T) { check(t, result, result.Config.BuildOSTarget.String()) })
	t.Run("device", func(t *testing.T) { check(t, result, "android_arm64_armv8-a") })
}

func TestUbsan(t *testing.T) {
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
			name: "bin_depends_ubsan",
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

		cc_library_shared {
			name: "libshared",
			host_supported: true,
			shared_libs: ["libtransitive"],
		}

		cc_library_shared {
			name: "libtransitive",
			host_supported: true,
		}

		cc_library_static {
			name: "libubsan",
			host_supported: true,
			sanitize: {
				undefined: true,
			}
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

	result := android.GroupFixturePreparers(
		prepareForCcTest,
	).RunTestWithBp(t, bp)

	check := func(t *testing.T, result *android.TestResult, variant string) {
		staticVariant := variant + "_static"

		minimalRuntime := result.ModuleForTests("libclang_rt.ubsan_minimal", staticVariant)

		// The binaries, one with ubsan and one without
		binWithUbsan := result.ModuleForTests("bin_with_ubsan", variant)
		binDependsUbsan := result.ModuleForTests("bin_depends_ubsan", variant)
		binNoUbsan := result.ModuleForTests("bin_no_ubsan", variant)

		android.AssertStringListContains(t, "missing libclang_rt.ubsan_minimal in bin_with_ubsan static libs",
			strings.Split(binWithUbsan.Rule("ld").Args["libFlags"], " "),
			minimalRuntime.OutputFiles(t, "")[0].String())

		android.AssertStringListContains(t, "missing libclang_rt.ubsan_minimal in bin_depends_ubsan static libs",
			strings.Split(binDependsUbsan.Rule("ld").Args["libFlags"], " "),
			minimalRuntime.OutputFiles(t, "")[0].String())

		android.AssertStringListDoesNotContain(t, "unexpected libclang_rt.ubsan_minimal in bin_no_ubsan static libs",
			strings.Split(binNoUbsan.Rule("ld").Args["libFlags"], " "),
			minimalRuntime.OutputFiles(t, "")[0].String())

		android.AssertStringListContains(t, "missing -Wl,--exclude-libs for minimal runtime in bin_with_ubsan",
			strings.Split(binWithUbsan.Rule("ld").Args["ldFlags"], " "),
			"-Wl,--exclude-libs="+minimalRuntime.OutputFiles(t, "")[0].Base())

		android.AssertStringListContains(t, "missing -Wl,--exclude-libs for minimal runtime in bin_depends_ubsan static libs",
			strings.Split(binDependsUbsan.Rule("ld").Args["ldFlags"], " "),
			"-Wl,--exclude-libs="+minimalRuntime.OutputFiles(t, "")[0].Base())

		android.AssertStringListDoesNotContain(t, "unexpected -Wl,--exclude-libs for minimal runtime in bin_no_ubsan static libs",
			strings.Split(binNoUbsan.Rule("ld").Args["ldFlags"], " "),
			"-Wl,--exclude-libs="+minimalRuntime.OutputFiles(t, "")[0].Base())
	}

	t.Run("host", func(t *testing.T) { check(t, result, result.Config.BuildOSTarget.String()) })
	t.Run("device", func(t *testing.T) { check(t, result, "android_arm64_armv8-a") })
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
	note_async := "note_memtag_heap_async"
	note_sync := "note_memtag_heap_sync"

	found := None
	implicits := m.Rule("ld").Implicits
	for _, lib := range implicits {
		if strings.Contains(lib.Rel(), note_async) {
			found = Async
			break
		} else if strings.Contains(lib.Rel(), note_sync) {
			found = Sync
			break
		}
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
