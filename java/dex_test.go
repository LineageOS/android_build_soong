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

package java

import (
	"testing"

	"android/soong/android"
)

func TestR8(t *testing.T) {
	result := PrepareForTestWithJavaDefaultModulesWithoutFakeDex2oatd.RunTestWithBp(t, `
		android_app {
			name: "app",
			srcs: ["foo.java"],
			libs: ["lib"],
			static_libs: ["static_lib"],
			platform_apis: true,
		}

		java_library {
			name: "lib",
			srcs: ["foo.java"],
		}

		java_library {
			name: "static_lib",
			srcs: ["foo.java"],
		}
	`)

	app := result.ModuleForTests("app", "android_common")
	lib := result.ModuleForTests("lib", "android_common")
	staticLib := result.ModuleForTests("static_lib", "android_common")

	appJavac := app.Rule("javac")
	appR8 := app.Rule("r8")
	libHeader := lib.Output("turbine-combined/lib.jar").Output
	staticLibHeader := staticLib.Output("turbine-combined/static_lib.jar").Output

	android.AssertStringDoesContain(t, "expected lib header jar in app javac classpath",
		appJavac.Args["classpath"], libHeader.String())
	android.AssertStringDoesContain(t, "expected static_lib header jar in app javac classpath",
		appJavac.Args["classpath"], staticLibHeader.String())

	android.AssertStringDoesContain(t, "expected lib header jar in app r8 classpath",
		appR8.Args["r8Flags"], libHeader.String())
	android.AssertStringDoesNotContain(t, "expected no  static_lib header jar in app javac classpath",
		appR8.Args["r8Flags"], staticLibHeader.String())
}

func TestD8(t *testing.T) {
	result := PrepareForTestWithJavaDefaultModulesWithoutFakeDex2oatd.RunTestWithBp(t, `
		java_library {
			name: "foo",
			srcs: ["foo.java"],
			libs: ["lib"],
			static_libs: ["static_lib"],
			installable: true,
		}

		java_library {
			name: "lib",
			srcs: ["foo.java"],
		}

		java_library {
			name: "static_lib",
			srcs: ["foo.java"],
		}
	`)

	foo := result.ModuleForTests("foo", "android_common")
	lib := result.ModuleForTests("lib", "android_common")
	staticLib := result.ModuleForTests("static_lib", "android_common")

	fooJavac := foo.Rule("javac")
	fooD8 := foo.Rule("d8")
	libHeader := lib.Output("turbine-combined/lib.jar").Output
	staticLibHeader := staticLib.Output("turbine-combined/static_lib.jar").Output

	android.AssertStringDoesContain(t, "expected lib header jar in foo javac classpath",
		fooJavac.Args["classpath"], libHeader.String())
	android.AssertStringDoesContain(t, "expected static_lib header jar in foo javac classpath",
		fooJavac.Args["classpath"], staticLibHeader.String())

	android.AssertStringDoesContain(t, "expected lib header jar in foo d8 classpath",
		fooD8.Args["d8Flags"], libHeader.String())
	android.AssertStringDoesNotContain(t, "expected no  static_lib header jar in foo javac classpath",
		fooD8.Args["d8Flags"], staticLibHeader.String())
}
