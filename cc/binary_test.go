// Copyright 2022 Google Inc. All rights reserved.
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

package cc

import (
	"testing"

	"android/soong/android"
)

func TestBinaryLinkerScripts(t *testing.T) {
	t.Parallel()
	result := PrepareForIntegrationTestWithCc.RunTestWithBp(t, `
		cc_binary {
			name: "foo",
			srcs: ["foo.cc"],
			linker_scripts: ["foo.ld", "bar.ld"],
		}`)

	binFoo := result.ModuleForTests("foo", "android_arm64_armv8-a").Rule("ld")

	android.AssertStringListContains(t, "missing dependency on linker_scripts",
		binFoo.Implicits.Strings(), "foo.ld")
	android.AssertStringListContains(t, "missing dependency on linker_scripts",
		binFoo.Implicits.Strings(), "bar.ld")
	android.AssertStringDoesContain(t, "missing flag for linker_scripts",
		binFoo.Args["ldFlags"], "-Wl,--script,foo.ld")
	android.AssertStringDoesContain(t, "missing flag for linker_scripts",
		binFoo.Args["ldFlags"], "-Wl,--script,bar.ld")
}
