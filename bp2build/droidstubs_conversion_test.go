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

package bp2build

import (
	"testing"

	"android/soong/android"
	"android/soong/java"
)

func registerJavaApiModules(ctx android.RegistrationContext) {
	java.RegisterSdkLibraryBuildComponents(ctx)
	java.RegisterStubsBuildComponents(ctx)
}

func TestDroidstubsApiContributions(t *testing.T) {
	bp := `
	droidstubs {
		name: "framework-stubs",
		check_api: {
			current: {
				api_file: "framework.current.txt",
			},
		},
	}

	// Modules without check_api should not generate a Bazel API target
	droidstubs {
		name: "framework-docs",
	}

	// java_sdk_library is a macro that creates droidstubs
	java_sdk_library {
		name: "module-stubs",
		srcs: ["A.java"],

		// These api surfaces are added by default, but add them explicitly to make
		// this test hermetic
		public: {
			enabled: true,
		},
		system: {
			enabled: true,
		},

		// Disable other api surfaces to keep unit test scope limited
		module_lib: {
			enabled: false,
		},
		test: {
			enabled: false,
		},
	}
	`
	expectedBazelTargets := []string{
		MakeBazelTargetNoRestrictions(
			"java_api_contribution",
			"framework-stubs.contribution",
			AttrNameToString{
				"api":                    `"framework.current.txt"`,
				"api_surface":            `"publicapi"`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
			}),
		MakeBazelTargetNoRestrictions(
			"java_api_contribution",
			"module-stubs.stubs.source.contribution",
			AttrNameToString{
				"api":                    `"api/current.txt"`,
				"api_surface":            `"publicapi"`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
			}),
		MakeBazelTargetNoRestrictions(
			"java_api_contribution",
			"module-stubs.stubs.source.system.contribution",
			AttrNameToString{
				"api":                    `"api/system-current.txt"`,
				"api_surface":            `"systemapi"`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
			}),
	}
	RunApiBp2BuildTestCase(t, registerJavaApiModules, Bp2buildTestCase{
		Blueprint:            bp,
		ExpectedBazelTargets: expectedBazelTargets,
		Filesystem: map[string]string{
			"api/current.txt":        "",
			"api/removed.txt":        "",
			"api/system-current.txt": "",
			"api/system-removed.txt": "",
		},
	})
}
