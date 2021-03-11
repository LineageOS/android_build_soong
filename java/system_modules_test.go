// Copyright 2021 Google Inc. All rights reserved.
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
)

func TestJavaSystemModules(t *testing.T) {
	result := javaFixtureFactory.RunTestWithBp(t, `
		java_system_modules {
			name: "system-modules",
			libs: ["system-module1", "system-module2"],
		}
		java_library {
			name: "system-module1",
			srcs: ["a.java"],
			sdk_version: "none",
			system_modules: "none",
		}
		java_library {
			name: "system-module2",
			srcs: ["b.java"],
			sdk_version: "none",
			system_modules: "none",
		}
		`)

	// check the existence of the module
	systemModules := result.ModuleForTests("system-modules", "android_common")

	cmd := systemModules.Rule("jarsTosystemModules")

	// make sure the command compiles against the supplied modules.
	for _, module := range []string{"system-module1.jar", "system-module2.jar"} {
		result.AssertStringDoesContain("system modules classpath", cmd.Args["classpath"], module)
	}
}

func TestJavaSystemModulesImport(t *testing.T) {
	result := javaFixtureFactory.RunTestWithBp(t, `
		java_system_modules_import {
			name: "system-modules",
			libs: ["system-module1", "system-module2"],
		}
		java_import {
			name: "system-module1",
			jars: ["a.jar"],
		}
		java_import {
			name: "system-module2",
			jars: ["b.jar"],
		}
		`)

	// check the existence of the module
	systemModules := result.ModuleForTests("system-modules", "android_common")

	cmd := systemModules.Rule("jarsTosystemModules")

	// make sure the command compiles against the supplied modules.
	for _, module := range []string{"system-module1.jar", "system-module2.jar"} {
		result.AssertStringDoesContain("system modules classpath", cmd.Args["classpath"], module)
	}
}
