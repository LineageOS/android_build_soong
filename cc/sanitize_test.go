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
	"testing"

	"android/soong/android"
)

var prepareForAsanTest = android.FixtureAddFile("asan/Android.bp", []byte(`
	cc_library_shared {
		name: "libclang_rt.asan-aarch64-android",
	}

	cc_library_shared {
		name: "libclang_rt.asan-arm-android",
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
