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
	"path/filepath"
	"runtime"
	"testing"

	"android/soong/android"
	"android/soong/cc"
)

var prepForJavaFuzzTest = android.GroupFixturePreparers(
	PrepareForTestWithJavaDefaultModules,
	cc.PrepareForTestWithCcBuildComponents,
	android.FixtureRegisterWithContext(RegisterJavaFuzzBuildComponents),
)

func TestJavaFuzz(t *testing.T) {
	result := prepForJavaFuzzTest.RunTestWithBp(t, `
		java_fuzz {
			name: "foo",
			srcs: ["a.java"],
			host_supported: true,
			device_supported: false,
			libs: ["bar"],
			static_libs: ["baz"],
            jni_libs: [
                "libjni",
            ],
		}

		java_library_host {
			name: "bar",
			srcs: ["b.java"],
		}

		java_library_host {
			name: "baz",
			srcs: ["c.java"],
		}

		cc_library_shared {
			name: "libjni",
			host_supported: true,
			device_supported: false,
			stl: "none",
		}
		`)

	osCommonTarget := result.Config.BuildOSCommonTarget.String()

	javac := result.ModuleForTests("foo", osCommonTarget).Rule("javac")
	combineJar := result.ModuleForTests("foo", osCommonTarget).Description("for javac")

	if len(javac.Inputs) != 1 || javac.Inputs[0].String() != "a.java" {
		t.Errorf(`foo inputs %v != ["a.java"]`, javac.Inputs)
	}

	baz := result.ModuleForTests("baz", osCommonTarget).Rule("javac").Output.String()
	barOut := filepath.Join("out", "soong", ".intermediates", "bar", osCommonTarget, "javac-header", "bar.jar")
	bazOut := filepath.Join("out", "soong", ".intermediates", "baz", osCommonTarget, "javac-header", "baz.jar")

	android.AssertStringDoesContain(t, "foo classpath", javac.Args["classpath"], barOut)
	android.AssertStringDoesContain(t, "foo classpath", javac.Args["classpath"], bazOut)

	if len(combineJar.Inputs) != 2 || combineJar.Inputs[1].String() != baz {
		t.Errorf("foo combineJar inputs %v does not contain %q", combineJar.Inputs, baz)
	}

	ctx := result.TestContext
	foo := ctx.ModuleForTests("foo", osCommonTarget).Module().(*JavaFuzzTest)

	expected := "lib64/libjni.so"
	if runtime.GOOS == "darwin" {
		expected = "lib64/libjni.dylib"
	}

	fooJniFilePaths := foo.jniFilePaths
	if len(fooJniFilePaths) != 1 || fooJniFilePaths[0].Rel() != expected {
		t.Errorf(`expected foo test data relative path [%q], got %q`,
			expected, fooJniFilePaths.Strings())
	}
}
