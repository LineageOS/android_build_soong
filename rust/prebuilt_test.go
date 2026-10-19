// Copyright (C) 2026 The LineageOS Project
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

func TestPrebuiltCheckElfFile(t *testing.T) {
	ctx := testRust(t, `
		rust_prebuilt_binary {
			name: "fizz",
			srcs: ["fizz"],
			rustlibs: ["libbuzz"],
			shared_libs: ["libbar"],
		}
		rust_prebuilt_dylib {
			name: "libbuzz",
			crate_name: "buzz",
			srcs: ["libbuzz.dylib.so"],
		}
		cc_library_shared {
			name: "libbar",
			srcs: ["foo.c"],
		}
	`, android.FixtureMergeMockFs(android.MockFS{
		"fizz":             nil,
		"libbuzz.dylib.so": nil,
	}))

	fizz := ctx.ModuleForTests(t, "fizz", "android_arm64_armv8-a")
	check := fizz.Output("fizz.check_elf_file")
	for _, arg := range []string{"libbuzz.dylib.so", "libbar.so"} {
		if !strings.Contains(check.Args["args"], arg) {
			t.Errorf("fizz check_elf_file args %q do not contain %s", check.Args["args"], arg)
		}
	}
	android.AssertPathsRelativeToTopEquals(t, "fizz validations",
		[]string{check.Output.String()}, fizz.Output("unstripped/fizz").Validations)

	buzz := ctx.ModuleForTests(t, "libbuzz", "android_arm64_armv8-a_dylib")
	check = buzz.Output("libbuzz.dylib.so.check_elf_file")
	android.AssertStringDoesContain(t, "libbuzz check_elf_file args",
		check.Args["args"], "--soname libbuzz.dylib.so")
	android.AssertPathRelativeToTopEquals(t, "libbuzz validation",
		check.Output.String(), buzz.Output("libbuzz.dylib.so").Validation)
}
