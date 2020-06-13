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

func TestRustTest(t *testing.T) {
	ctx := testRust(t, `
		rust_test_host {
			name: "my_test",
			srcs: ["foo.rs"],
		}`)

	testingModule := ctx.ModuleForTests("my_test", "linux_glibc_x86_64")
	expectedOut := "my_test/linux_glibc_x86_64/my_test"
	outPath := testingModule.Output("my_test").Output.String()
	if !strings.Contains(outPath, expectedOut) {
		t.Errorf("wrong output path: %v;  expected: %v", outPath, expectedOut)
	}
}
