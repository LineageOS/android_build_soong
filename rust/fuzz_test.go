// Copyright 2020 The Android Open Source Project
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

func TestRustFuzz(t *testing.T) {
	ctx := testRust(t, `
			rust_library {
				name: "libtest_fuzzing",
				crate_name: "test_fuzzing",
				srcs: ["foo.rs"],
			}
			rust_fuzz {
				name: "fuzz_libtest",
				srcs: ["foo.rs"],
				rustlibs: ["libtest_fuzzing"],
			}
			rust_fuzz_host {
				name: "host_fuzzer",
				srcs: ["foo.rs"],
			}
	`)

	// Check that appropriate dependencies are added and that the rustlib linkage is correct.
	fuzz_libtest_mod := ctx.ModuleForTests("fuzz_libtest", "android_arm64_armv8-a_fuzzer").Module().(*Module)
	if !android.InList("liblibfuzzer_sys.rlib-std", fuzz_libtest_mod.Properties.AndroidMkRlibs) {
		t.Errorf("liblibfuzzer_sys rlib library dependency missing for rust_fuzz module. %#v", fuzz_libtest_mod.Properties.AndroidMkRlibs)
	}
	if !android.InList("libtest_fuzzing.rlib-std", fuzz_libtest_mod.Properties.AndroidMkRlibs) {
		t.Errorf("rustlibs not linked as rlib for rust_fuzz module.")
	}

	// Check that compiler flags are set appropriately .
	fuzz_libtest := ctx.ModuleForTests("fuzz_libtest", "android_arm64_armv8-a_fuzzer").Rule("rustc")
	if !strings.Contains(fuzz_libtest.Args["rustcFlags"], "-C passes='sancov-module'") ||
		!strings.Contains(fuzz_libtest.Args["rustcFlags"], "--cfg fuzzing") {
		t.Errorf("rust_fuzz module does not contain the expected flags (sancov-module, cfg fuzzing).")
	}

	// Check that host modules support fuzzing.
	host_fuzzer := ctx.ModuleForTests("fuzz_libtest", "android_arm64_armv8-a_fuzzer").Rule("rustc")
	if !strings.Contains(host_fuzzer.Args["rustcFlags"], "-C passes='sancov-module'") ||
		!strings.Contains(host_fuzzer.Args["rustcFlags"], "--cfg fuzzing") {
		t.Errorf("rust_fuzz_host module does not contain the expected flags (sancov-module, cfg fuzzing).")
	}

	// Check that dependencies have 'fuzzer' variants produced for them as well.
	libtest_fuzzer := ctx.ModuleForTests("libtest_fuzzing", "android_arm64_armv8-a_rlib_rlib-std_fuzzer").Output("libtest_fuzzing.rlib")
	if !strings.Contains(libtest_fuzzer.Args["rustcFlags"], "-C passes='sancov-module'") ||
		!strings.Contains(libtest_fuzzer.Args["rustcFlags"], "--cfg fuzzing") {
		t.Errorf("rust_fuzz dependent library does not contain the expected flags (sancov-module, cfg fuzzing).")
	}
}

func TestRustFuzzDepBundling(t *testing.T) {
	ctx := testRust(t, `
			cc_library {
				name: "libcc_transitive_dep",
			}
			cc_library {
				name: "libcc_direct_dep",
			}
			rust_library {
				name: "libtest_fuzzing",
				crate_name: "test_fuzzing",
				srcs: ["foo.rs"],
				shared_libs: ["libcc_transitive_dep"],
			}
			rust_fuzz {
				name: "fuzz_libtest",
				srcs: ["foo.rs"],
				rustlibs: ["libtest_fuzzing"],
				shared_libs: ["libcc_direct_dep"],
			}
	`)

	fuzz_libtest := ctx.ModuleForTests("fuzz_libtest", "android_arm64_armv8-a_fuzzer").Module().(*Module)

	if !strings.Contains(fuzz_libtest.FuzzSharedLibraries().String(), ":libcc_direct_dep.so") {
		t.Errorf("rust_fuzz does not contain the expected bundled direct shared libs ('libcc_direct_dep'): %#v", fuzz_libtest.FuzzSharedLibraries().String())
	}
	if !strings.Contains(fuzz_libtest.FuzzSharedLibraries().String(), ":libcc_transitive_dep.so") {
		t.Errorf("rust_fuzz does not contain the expected bundled transitive shared libs ('libcc_transitive_dep'): %#v", fuzz_libtest.FuzzSharedLibraries().String())
	}
}

func TestCCFuzzDepBundling(t *testing.T) {
	ctx := testRust(t, `
			cc_library {
				name: "libcc_transitive_dep",
			}
			rust_ffi {
				name: "libtest_fuzzing",
				crate_name: "test_fuzzing",
				srcs: ["foo.rs"],
				shared_libs: ["libcc_transitive_dep"],
			}
			cc_fuzz {
				name: "fuzz_shared_libtest",
				shared_libs: ["libtest_fuzzing"],
			}
			cc_fuzz {
				name: "fuzz_static_libtest",
				static_libs: ["libtest_fuzzing"],
			}

	`)

	fuzz_shared_libtest := ctx.ModuleForTests("fuzz_shared_libtest", "android_arm64_armv8-a_fuzzer").Module().(cc.LinkableInterface)
	fuzz_static_libtest := ctx.ModuleForTests("fuzz_static_libtest", "android_arm64_armv8-a_fuzzer").Module().(cc.LinkableInterface)

	if !strings.Contains(fuzz_shared_libtest.FuzzSharedLibraries().String(), ":libcc_transitive_dep.so") {
		t.Errorf("cc_fuzz does not contain the expected bundled transitive shared libs from rust_ffi_shared ('libcc_transitive_dep'): %#v", fuzz_shared_libtest.FuzzSharedLibraries().String())
	}
	if !strings.Contains(fuzz_static_libtest.FuzzSharedLibraries().String(), ":libcc_transitive_dep.so") {
		t.Errorf("cc_fuzz does not contain the expected bundled transitive shared libs from rust_ffi_static ('libcc_transitive_dep'): %#v", fuzz_static_libtest.FuzzSharedLibraries().String())
	}
}
