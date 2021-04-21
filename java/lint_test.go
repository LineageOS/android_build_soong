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

func TestJavaLintBypassUpdatableChecks(t *testing.T) {
	testCases := []struct {
		name  string
		bp    string
		error string
	}{
		{
			name: "warning_checks",
			bp: `
				java_library {
					name: "foo",
					srcs: [
						"a.java",
					],
					min_sdk_version: "29",
					sdk_version: "current",
					lint: {
						warning_checks: ["NewApi"],
					},
				}
			`,
			error: "lint.warning_checks: Can't treat \\[NewApi\\] checks as warnings if min_sdk_version is different from sdk_version.",
		},
		{
			name: "disable_checks",
			bp: `
				java_library {
					name: "foo",
					srcs: [
						"a.java",
					],
					min_sdk_version: "29",
					sdk_version: "current",
					lint: {
						disabled_checks: ["NewApi"],
					},
				}
			`,
			error: "lint.disabled_checks: Can't disable \\[NewApi\\] checks if min_sdk_version is different from sdk_version.",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			errorHandler := android.FixtureExpectsAtLeastOneErrorMatchingPattern(testCase.error)
			android.GroupFixturePreparers(PrepareForTestWithJavaDefaultModules).
				ExtendWithErrorHandler(errorHandler).
				RunTestWithBp(t, testCase.bp)
		})
	}
}
