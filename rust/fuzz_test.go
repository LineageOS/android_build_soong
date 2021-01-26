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
	`)

	// Check that appropriate dependencies are added and that the rustlib linkage is correct.
	fuzz_libtest_mod := ctx.ModuleForTests("fuzz_libtest", "android_arm64_armv8-a_fuzzer").Module().(*Module)
	if !android.InList("libclang_rt.asan-aarch64-android", fuzz_libtest_mod.Properties.AndroidMkSharedLibs) {
		t.Errorf("libclang_rt.asan-aarch64-android shared library dependency missing for rust_fuzz module.")
	}
	if !android.InList("liblibfuzzer_sys.rlib-std", fuzz_libtest_mod.Properties.AndroidMkRlibs) {
		t.Errorf("liblibfuzzer_sys rlib library dependency missing for rust_fuzz module. %#v", fuzz_libtest_mod.Properties.AndroidMkRlibs)
	}
	if !android.InList("libtest_fuzzing.rlib-std", fuzz_libtest_mod.Properties.AndroidMkRlibs) {
		t.Errorf("rustlibs not linked as rlib for rust_fuzz module.")
	}

	// Check that compiler flags are set appropriately .
	fuzz_libtest := ctx.ModuleForTests("fuzz_libtest", "android_arm64_armv8-a_fuzzer").Output("fuzz_libtest")
	if !strings.Contains(fuzz_libtest.Args["rustcFlags"], "-Z sanitizer=address") ||
		!strings.Contains(fuzz_libtest.Args["rustcFlags"], "-C passes='sancov'") ||
		!strings.Contains(fuzz_libtest.Args["rustcFlags"], "--cfg fuzzing") {
		t.Errorf("rust_fuzz module does not contain the expected flags (sancov, cfg fuzzing, address sanitizer).")

	}

	// Check that dependencies have 'fuzzer' variants produced for them as well.
	libtest_fuzzer := ctx.ModuleForTests("libtest_fuzzing", "android_arm64_armv8-a_rlib_rlib-std_fuzzer").Output("libtest_fuzzing.rlib")
	if !strings.Contains(libtest_fuzzer.Args["rustcFlags"], "-Z sanitizer=address") ||
		!strings.Contains(libtest_fuzzer.Args["rustcFlags"], "-C passes='sancov'") ||
		!strings.Contains(libtest_fuzzer.Args["rustcFlags"], "--cfg fuzzing") {
		t.Errorf("rust_fuzz dependent library does not contain the expected flags (sancov, cfg fuzzing, address sanitizer).")
	}
}
