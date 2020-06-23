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
	"testing"
)

func TestClippy(t *testing.T) {
	ctx := testRust(t, `
		rust_library {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
		}
		rust_library {
			name: "libfoobar",
			srcs: ["foo.rs"],
			crate_name: "foobar",
			clippy: false,
		}`)

	ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_dylib").Output("libfoo.dylib.so")
	fooClippy := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_dylib").MaybeRule("clippy")
	if fooClippy.Rule.String() != "android/soong/rust.clippy" {
		t.Errorf("Clippy output (default) for libfoo was not generated: %+v", fooClippy)
	}

	ctx.ModuleForTests("libfoobar", "android_arm64_armv8-a_dylib").Output("libfoobar.dylib.so")
	foobarClippy := ctx.ModuleForTests("libfoobar", "android_arm64_armv8-a_dylib").MaybeRule("clippy")
	if foobarClippy.Rule != nil {
		t.Errorf("Clippy output for libfoobar is not empty")
	}
}
