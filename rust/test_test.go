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
)

// Check if rust_test_host accepts multiple source files and applies --test flag.
func TestRustTest(t *testing.T) {
	ctx := testRust(t, `
		rust_test_host {
			name: "my_test",
			srcs: ["foo.rs", "src/bar.rs"],
			crate_name: "new_test", // not used for multiple source files
			relative_install_path: "rust/my-test",
		}`)

	for _, name := range []string{"foo", "bar"} {
		testingModule := ctx.ModuleForTests("my_test", "linux_glibc_x86_64_"+name)
		testingBuildParams := testingModule.Output(name)
		rustcFlags := testingBuildParams.Args["rustcFlags"]
		if !strings.Contains(rustcFlags, "--test") {
			t.Errorf("%v missing --test flag, rustcFlags: %#v", name, rustcFlags)
		}
		outPath := "/my_test/linux_glibc_x86_64_" + name + "/" + name
		if !strings.Contains(testingBuildParams.Output.String(), outPath) {
			t.Errorf("wrong output: %v  expect: %v", testingBuildParams.Output, outPath)
		}
	}
}

// crate_name is output file name, when there is only one source file.
func TestRustTestSingleFile(t *testing.T) {
	ctx := testRust(t, `
		rust_test_host {
			name: "my-test",
			srcs: ["foo.rs"],
			crate_name: "new_test",
			relative_install_path: "my-pkg",
		}`)

	name := "new_test"
	testingModule := ctx.ModuleForTests("my-test", "linux_glibc_x86_64_"+name)
	outPath := "/my-test/linux_glibc_x86_64_" + name + "/" + name
	testingBuildParams := testingModule.Output(name)
	if !strings.Contains(testingBuildParams.Output.String(), outPath) {
		t.Errorf("wrong output: %v  expect: %v", testingBuildParams.Output, outPath)
	}
}
