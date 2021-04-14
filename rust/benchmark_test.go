// Copyright 2021 The Android Open Source Project
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

func TestRustBenchmark(t *testing.T) {
	ctx := testRust(t, `
		rust_benchmark_host {
			name: "my_bench",
			srcs: ["foo.rs"],
		}`)

	testingModule := ctx.ModuleForTests("my_bench", "linux_glibc_x86_64")
	expectedOut := "my_bench/linux_glibc_x86_64/my_bench"
	outPath := testingModule.Output("my_bench").Output.String()
	if !strings.Contains(outPath, expectedOut) {
		t.Errorf("wrong output path: %v;  expected: %v", outPath, expectedOut)
	}
}

func TestRustBenchmarkLinkage(t *testing.T) {
	ctx := testRust(t, `
		rust_benchmark {
			name: "my_bench",
			srcs: ["foo.rs"],
		}`)

	testingModule := ctx.ModuleForTests("my_bench", "android_arm64_armv8-a").Module().(*Module)

	if !android.InList("libcriterion.rlib-std", testingModule.Properties.AndroidMkRlibs) {
		t.Errorf("rlib-std variant for libcriterion not detected as a rustlib-defined rlib dependency for device rust_benchmark module")
	}
	if !android.InList("libstd", testingModule.Properties.AndroidMkRlibs) {
		t.Errorf("Device rust_benchmark module 'my_bench' does not link libstd as an rlib")
	}
}
