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

	"android/soong/android"
)

func getModuleHeaderJarsAsNormalizedPaths(result *android.TestResult, moduleNames ...string) []string {
	paths := []string{}
	for _, moduleName := range moduleNames {
		module := result.Module(moduleName, "android_common")
		info := result.ModuleProvider(module, JavaInfoProvider).(JavaInfo)
		paths = append(paths, result.NormalizePathsForTesting(info.HeaderJars)...)
	}
	return paths
}

var addSourceSystemModules = android.FixtureAddTextFile("source/Android.bp", `
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

func TestJavaSystemModules(t *testing.T) {
	result := prepareForJavaTest.RunTest(t, addSourceSystemModules)

	// check the existence of the source module
	sourceSystemModules := result.ModuleForTests("system-modules", "android_common")
	sourceInputs := sourceSystemModules.Rule("jarsTosystemModules").Inputs

	// The expected paths are the header jars from the source input modules.
	expectedSourcePaths := getModuleHeaderJarsAsNormalizedPaths(result, "system-module1", "system-module2")
	android.AssertArrayString(t, "source system modules inputs", expectedSourcePaths, result.NormalizePathsForTesting(sourceInputs))
}

var addPrebuiltSystemModules = android.FixtureAddTextFile("prebuilts/Android.bp", `
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

func TestJavaSystemModulesImport(t *testing.T) {
	result := prepareForJavaTest.RunTest(t, addPrebuiltSystemModules)

	// check the existence of the renamed prebuilt module
	prebuiltSystemModules := result.ModuleForTests("system-modules", "android_common")
	prebuiltInputs := prebuiltSystemModules.Rule("jarsTosystemModules").Inputs

	// The expected paths are the header jars from the renamed prebuilt input modules.
	expectedPrebuiltPaths := getModuleHeaderJarsAsNormalizedPaths(result, "system-module1", "system-module2")
	android.AssertArrayString(t, "renamed prebuilt system modules inputs", expectedPrebuiltPaths, result.NormalizePathsForTesting(prebuiltInputs))
}

func TestJavaSystemModulesMixSourceAndPrebuilt(t *testing.T) {
	result := prepareForJavaTest.RunTest(t,
		addSourceSystemModules,
		addPrebuiltSystemModules,
	)

	// check the existence of the source module
	sourceSystemModules := result.ModuleForTests("system-modules", "android_common")
	sourceInputs := sourceSystemModules.Rule("jarsTosystemModules").Inputs

	// The expected paths are the header jars from the source input modules.
	expectedSourcePaths := getModuleHeaderJarsAsNormalizedPaths(result, "system-module1", "system-module2")
	android.AssertArrayString(t, "source system modules inputs", expectedSourcePaths, result.NormalizePathsForTesting(sourceInputs))

	// check the existence of the renamed prebuilt module
	prebuiltSystemModules := result.ModuleForTests("prebuilt_system-modules", "android_common")
	prebuiltInputs := prebuiltSystemModules.Rule("jarsTosystemModules").Inputs

	// The expected paths are the header jars from the renamed prebuilt input modules.
	expectedPrebuiltPaths := getModuleHeaderJarsAsNormalizedPaths(result, "prebuilt_system-module1", "prebuilt_system-module2")
	android.AssertArrayString(t, "prebuilt system modules inputs", expectedPrebuiltPaths, result.NormalizePathsForTesting(prebuiltInputs))
}
