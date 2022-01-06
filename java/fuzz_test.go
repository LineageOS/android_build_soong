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

package java

import (
	"android/soong/android"
	"path/filepath"
	"testing"
)

var prepForJavaFuzzTest = android.GroupFixturePreparers(
	PrepareForTestWithJavaDefaultModules,
	android.FixtureRegisterWithContext(RegisterJavaFuzzBuildComponents),
)

func TestJavaFuzz(t *testing.T) {
	result := prepForJavaFuzzTest.RunTestWithBp(t, `
		java_fuzz_host {
			name: "foo",
			srcs: ["a.java"],
			libs: ["bar"],
			static_libs: ["baz"],
		}

		java_library_host {
			name: "bar",
			srcs: ["b.java"],
		}

		java_library_host {
			name: "baz",
			srcs: ["c.java"],
		}`)

	osCommonTarget := result.Config.BuildOSCommonTarget.String()
	javac := result.ModuleForTests("foo", osCommonTarget).Rule("javac")
	combineJar := result.ModuleForTests("foo", osCommonTarget).Description("for javac")

	if len(javac.Inputs) != 1 || javac.Inputs[0].String() != "a.java" {
		t.Errorf(`foo inputs %v != ["a.java"]`, javac.Inputs)
	}

	baz := result.ModuleForTests("baz", osCommonTarget).Rule("javac").Output.String()
	barOut := filepath.Join("out", "soong", ".intermediates", "bar", osCommonTarget, "javac", "bar.jar")
	bazOut := filepath.Join("out", "soong", ".intermediates", "baz", osCommonTarget, "javac", "baz.jar")

	android.AssertStringDoesContain(t, "foo classpath", javac.Args["classpath"], barOut)
	android.AssertStringDoesContain(t, "foo classpath", javac.Args["classpath"], bazOut)

	if len(combineJar.Inputs) != 2 || combineJar.Inputs[1].String() != baz {
		t.Errorf("foo combineJar inputs %v does not contain %q", combineJar.Inputs, baz)
	}
}
