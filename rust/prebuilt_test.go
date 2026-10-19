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

func TestPrebuiltBinaryCheckElfFile(t *testing.T) {
	ctx := testRust(t, `
		rust_prebuilt_binary {
			name: "fizz-buzz",
			srcs: ["fizz-buzz"],
			rustlibs: ["libfoo"],
			shared_libs: ["libbar"],
		}
		rust_prebuilt_binary {
			name: "unchecked",
			srcs: ["unchecked"],
			check_elf_files: false,
		}
		rust_library {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
		}
		cc_library_shared {
			name: "libbar",
			srcs: ["foo.c"],
		}
	`, android.FixtureMergeMockFs(android.MockFS{
		"fizz-buzz": nil,
		"unchecked": nil,
	}))

	fizzBuzz := ctx.ModuleForTests(t, "fizz-buzz", "android_arm64_armv8-a")
	check := fizzBuzz.Output("fizz-buzz.check_elf_file")
	args := check.Args["args"]
	for _, lib := range []string{"libfoo.dylib.so", "libbar.so"} {
		if !strings.Contains(args, lib) {
			t.Errorf("check_elf_file args %q do not pass %s", args, lib)
		}
	}
	android.AssertPathsRelativeToTopEquals(t, "validations",
		[]string{check.Output.String()}, fizzBuzz.Output("unstripped/fizz-buzz").Validations)

	unchecked := ctx.ModuleForTests(t, "unchecked", "android_arm64_armv8-a")
	if unchecked.MaybeOutput("unchecked.check_elf_file").Rule != nil {
		t.Errorf("check_elf_files: false still runs check_elf_file")
	}
}

func TestPrebuiltDylibCheckElfFile(t *testing.T) {
	ctx := testRust(t, `
		rust_prebuilt_dylib {
			name: "libfizz",
			crate_name: "fizz",
			srcs: ["libfizz.dylib.so"],
			rustlibs: ["libbuzz"],
			shared_libs: ["libbar"],
		}
		rust_prebuilt_dylib {
			name: "libbuzz",
			crate_name: "buzz",
			srcs: ["libbuzz.dylib.so"],
			check_elf_files: false,
		}
		cc_library_shared {
			name: "libbar",
			srcs: ["foo.c"],
		}
	`, android.FixtureMergeMockFs(android.MockFS{
		"libfizz.dylib.so": nil,
		"libbuzz.dylib.so": nil,
	}))

	fizz := ctx.ModuleForTests(t, "libfizz", "android_arm64_armv8-a_dylib")
	check := fizz.Output("libfizz.dylib.so.check_elf_file")
	args := check.Args["args"]
	for _, arg := range []string{"--soname libfizz.dylib.so", "libbuzz.dylib.so", "libbar.so"} {
		if !strings.Contains(args, arg) {
			t.Errorf("check_elf_file args %q do not contain %s", args, arg)
		}
	}
	android.AssertPathRelativeToTopEquals(t, "validation",
		check.Output.String(), fizz.Output("libfizz.dylib.so").Validation)

	buzz := ctx.ModuleForTests(t, "libbuzz", "android_arm64_armv8-a_dylib")
	if buzz.MaybeOutput("libbuzz.dylib.so.check_elf_file").Rule != nil {
		t.Errorf("check_elf_files: false still runs check_elf_file")
	}
}
