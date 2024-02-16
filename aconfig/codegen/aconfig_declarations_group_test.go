// Copyright 2024 Google Inc. All rights reserved.
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

package codegen

import (
	"android/soong/android"
	"android/soong/java"
	"testing"
)

func TestAconfigDeclarationsGroup(t *testing.T) {
	result := android.GroupFixturePreparers(
		PrepareForTestWithAconfigBuildComponents,
		java.PrepareForTestWithJavaDefaultModules,
	).RunTestWithBp(t, `
		aconfig_declarations {
			name: "foo-aconfig",
			package: "com.example.package",
			srcs: ["foo.aconfig"],
		}

		java_aconfig_library {
			name: "foo-java",
			aconfig_declarations: "foo-aconfig",
		}

		aconfig_declarations {
			name: "bar-aconfig",
			package: "com.example.package",
			srcs: ["foo.aconfig"],
		}

		java_aconfig_library {
			name: "bar-java",
			aconfig_declarations: "bar-aconfig",
		}

		aconfig_declarations_group {
			name: "my_group",
			java_aconfig_libraries: [
				"foo-java",
				"bar-java",
			],
		}

		java_library {
			name: "baz",
			srcs: [
				":my_group{.srcjars}",
			],
		}
	`)

	// Check if aconfig_declarations_group module depends on the aconfig_library modules
	java.CheckModuleDependencies(t, result.TestContext, "my_group", "", []string{
		`bar-java`,
		`foo-java`,
	})

	// Check if srcjar files are correctly passed to the reverse dependency of
	// aconfig_declarations_group module
	bazModule := result.ModuleForTests("baz", "android_common")
	bazJavacSrcjars := bazModule.Rule("javac").Args["srcJars"]
	errorMessage := "baz javac argument expected to contain srcjar provided by aconfig_declrations_group"
	android.AssertStringDoesContain(t, errorMessage, bazJavacSrcjars, "foo-java.srcjar")
	android.AssertStringDoesContain(t, errorMessage, bazJavacSrcjars, "bar-java.srcjar")
}
