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
	"strings"
	"testing"

	"android/soong/android"
)

func TestRustTest(t *testing.T) {
	ctx := testRust(t, `
		rust_test_host {
			name: "my_test",
			srcs: ["foo.rs"],
			data: ["data.txt"],
		}`)

	testingModule := ctx.ModuleForTests("my_test", "linux_glibc_x86_64")
	expectedOut := "my_test/linux_glibc_x86_64/my_test"
	outPath := testingModule.Output("my_test").Output.String()
	if !strings.Contains(outPath, expectedOut) {
		t.Errorf("wrong output path: %v;  expected: %v", outPath, expectedOut)
	}

	dataPaths := testingModule.Module().(*Module).compiler.(*testDecorator).dataPaths()
	if len(dataPaths) != 1 {
		t.Errorf("expected exactly one test data file. test data files: [%s]", dataPaths)
		return
	}
}

func TestRustTestLinkage(t *testing.T) {
	ctx := testRust(t, `
		rust_test {
			name: "my_test",
			srcs: ["foo.rs"],
			rustlibs: ["libfoo"],
            rlibs: ["libbar"],
		}
		rust_library {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
		}
		rust_library {
			name: "libbar",
			srcs: ["foo.rs"],
			crate_name: "bar",
		}`)

	testingModule := ctx.ModuleForTests("my_test", "android_arm64_armv8-a").Module().(*Module)

	if !android.InList("libfoo.rlib-std", testingModule.Properties.AndroidMkRlibs) {
		t.Errorf("rlib-std variant for libfoo not detected as a rustlib-defined rlib dependency for device rust_test module")
	}
	if !android.InList("libbar.rlib-std", testingModule.Properties.AndroidMkRlibs) {
		t.Errorf("rlib-std variant for libbar not detected as an rlib dependency for device rust_test module")
	}
	if !android.InList("libstd", testingModule.Properties.AndroidMkRlibs) {
		t.Errorf("Device rust_test module 'my_test' does not link libstd as an rlib")
	}
}
