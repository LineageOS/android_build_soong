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

// Test that coverage flags are being correctly generated.
func TestCoverageFlags(t *testing.T) {
	ctx := testRustCov(t, `
		rust_library {
			name: "libfoo_cov",
			srcs: ["foo.rs"],
			crate_name: "foo",
		}
		rust_binary {
			name: "fizz_cov",
			srcs: ["foo.rs"],
		}
        rust_binary {
			name: "buzzNoCov",
			srcs: ["foo.rs"],
			native_coverage: false,
		}
		rust_library {
			name: "libbar_nocov",
			srcs: ["foo.rs"],
			crate_name: "bar",
			native_coverage: false,
		}`)

	// Make sure native_coverage: false isn't creating a coverage variant.
	if android.InList("android_arm64_armv8-a_dylib_cov", ctx.ModuleVariantsForTests("libbar_nocov")) {
		t.Fatalf("coverage variant created for module 'libbar_nocov' with native coverage disabled")
	}

	// Just test the dylib variants unless the library coverage logic changes to distinguish between the types.
	libfooCov := ctx.ModuleForTests("libfoo_cov", "android_arm64_armv8-a_dylib_cov").Rule("rustc")
	libbarNoCov := ctx.ModuleForTests("libbar_nocov", "android_arm64_armv8-a_dylib").Rule("rustc")
	fizzCov := ctx.ModuleForTests("fizz_cov", "android_arm64_armv8-a_cov").Rule("rustc")
	buzzNoCov := ctx.ModuleForTests("buzzNoCov", "android_arm64_armv8-a").Rule("rustc")

	rustcCoverageFlags := []string{"-C instrument-coverage", " -g "}
	for _, flag := range rustcCoverageFlags {
		missingErrorStr := "missing rustc flag '%s' for '%s' module with coverage enabled; rustcFlags: %#v"
		containsErrorStr := "contains rustc flag '%s' for '%s' module with coverage disabled; rustcFlags: %#v"

		if !strings.Contains(fizzCov.Args["rustcFlags"], flag) {
			t.Fatalf(missingErrorStr, flag, "fizz_cov", fizzCov.Args["rustcFlags"])
		}
		if !strings.Contains(libfooCov.Args["rustcFlags"], flag) {
			t.Fatalf(missingErrorStr, flag, "libfoo_cov dylib", libfooCov.Args["rustcFlags"])
		}
		if strings.Contains(buzzNoCov.Args["rustcFlags"], flag) {
			t.Fatalf(containsErrorStr, flag, "buzzNoCov", buzzNoCov.Args["rustcFlags"])
		}
		if strings.Contains(libbarNoCov.Args["rustcFlags"], flag) {
			t.Fatalf(containsErrorStr, flag, "libbar_cov", libbarNoCov.Args["rustcFlags"])
		}
	}

	linkCoverageFlags := []string{"-fprofile-instr-generate=/data/misc/trace/clang-%p-%m.profraw", " -g "}
	for _, flag := range linkCoverageFlags {
		missingErrorStr := "missing rust linker flag '%s' for '%s' module with coverage enabled; rustcFlags: %#v"
		containsErrorStr := "contains rust linker flag '%s' for '%s' module with coverage disabled; rustcFlags: %#v"

		if !strings.Contains(fizzCov.Args["linkFlags"], flag) {
			t.Fatalf(missingErrorStr, flag, "fizz_cov", fizzCov.Args["linkFlags"])
		}
		if !strings.Contains(libfooCov.Args["linkFlags"], flag) {
			t.Fatalf(missingErrorStr, flag, "libfoo_cov dylib", libfooCov.Args["linkFlags"])
		}
		if strings.Contains(buzzNoCov.Args["linkFlags"], flag) {
			t.Fatalf(containsErrorStr, flag, "buzzNoCov", buzzNoCov.Args["linkFlags"])
		}
		if strings.Contains(libbarNoCov.Args["linkFlags"], flag) {
			t.Fatalf(containsErrorStr, flag, "libbar_cov", libbarNoCov.Args["linkFlags"])
		}
	}

}

func TestCoverageDeps(t *testing.T) {
	ctx := testRustCov(t, `
		rust_binary {
			name: "fizz",
			srcs: ["foo.rs"],
		}`)

	fizz := ctx.ModuleForTests("fizz", "android_arm64_armv8-a_cov").Rule("rustc")
	if !strings.Contains(fizz.Args["linkFlags"], "libprofile-clang-extras.a") {
		t.Fatalf("missing expected coverage 'libprofile-clang-extras' dependency in linkFlags: %#v", fizz.Args["linkFlags"])
	}
}
