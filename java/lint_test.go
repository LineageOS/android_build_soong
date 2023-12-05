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
	"strings"
	"testing"

	"android/soong/android"
)

func TestJavaLint(t *testing.T) {
	ctx, _ := testJavaWithFS(t, `
		java_library {
			name: "foo",
			srcs: [
				"a.java",
				"b.java",
				"c.java",
			],
			min_sdk_version: "29",
			sdk_version: "system_current",
		}
       `, map[string][]byte{
		"lint-baseline.xml": nil,
	})

	foo := ctx.ModuleForTests("foo", "android_common")

	sboxProto := android.RuleBuilderSboxProtoForTests(t, ctx, foo.Output("lint.sbox.textproto"))
	if !strings.Contains(*sboxProto.Commands[0].Command, "--baseline lint-baseline.xml") {
		t.Error("did not pass --baseline flag")
	}
}

func TestJavaLintWithoutBaseline(t *testing.T) {
	ctx, _ := testJavaWithFS(t, `
		java_library {
			name: "foo",
			srcs: [
				"a.java",
				"b.java",
				"c.java",
			],
			min_sdk_version: "29",
			sdk_version: "system_current",
		}
       `, map[string][]byte{})

	foo := ctx.ModuleForTests("foo", "android_common")

	sboxProto := android.RuleBuilderSboxProtoForTests(t, ctx, foo.Output("lint.sbox.textproto"))
	if strings.Contains(*sboxProto.Commands[0].Command, "--baseline") {
		t.Error("passed --baseline flag for non existent file")
	}
}

func TestJavaLintRequiresCustomLintFileToExist(t *testing.T) {
	android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
		android.PrepareForTestDisallowNonExistentPaths,
	).ExtendWithErrorHandler(android.FixtureExpectsAllErrorsToMatchAPattern([]string{`source path "mybaseline.xml" does not exist`})).
		RunTestWithBp(t, `
			java_library {
				name: "foo",
				srcs: [
				],
				min_sdk_version: "29",
				sdk_version: "system_current",
				lint: {
					baseline_filename: "mybaseline.xml",
				},
			}
	 `)
}

func TestJavaLintUsesCorrectBpConfig(t *testing.T) {
	ctx, _ := testJavaWithFS(t, `
		java_library {
			name: "foo",
			srcs: [
				"a.java",
				"b.java",
				"c.java",
			],
			min_sdk_version: "29",
			sdk_version: "system_current",
			lint: {
				error_checks: ["SomeCheck"],
				baseline_filename: "mybaseline.xml",
			},
		}
       `, map[string][]byte{
		"mybaseline.xml": nil,
	})

	foo := ctx.ModuleForTests("foo", "android_common")

	sboxProto := android.RuleBuilderSboxProtoForTests(t, ctx, foo.Output("lint.sbox.textproto"))
	if !strings.Contains(*sboxProto.Commands[0].Command, "--baseline mybaseline.xml") {
		t.Error("did not use the correct file for baseline")
	}

	if !strings.Contains(*sboxProto.Commands[0].Command, "--warning_check NewApi") {
		// TODO(b/268261262): Change this to check for --error_check
		t.Error("should check NewApi warnings")
	}

	if !strings.Contains(*sboxProto.Commands[0].Command, "--error_check SomeCheck") {
		t.Error("should combine NewApi errors with SomeCheck errors")
	}
}

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

// TODO(b/193460475): Re-enable this test
//func TestJavaLintStrictUpdatabilityLinting(t *testing.T) {
//	bp := `
//		java_library {
//			name: "foo",
//			srcs: [
//				"a.java",
//			],
//			static_libs: ["bar"],
//			min_sdk_version: "29",
//			sdk_version: "current",
//			lint: {
//				strict_updatability_linting: true,
//			},
//		}
//
//		java_library {
//			name: "bar",
//			srcs: [
//				"a.java",
//			],
//			min_sdk_version: "29",
//			sdk_version: "current",
//		}
//	`
//	fs := android.MockFS{
//		"lint-baseline.xml": nil,
//	}
//
//	result := android.GroupFixturePreparers(PrepareForTestWithJavaDefaultModules, fs.AddToFixture()).
//		RunTestWithBp(t, bp)
//
//	foo := result.ModuleForTests("foo", "android_common")
//	sboxProto := android.RuleBuilderSboxProtoForTests(t, foo.Output("lint.sbox.textproto"))
//	if !strings.Contains(*sboxProto.Commands[0].Command,
//		"--baseline lint-baseline.xml --disallowed_issues NewApi") {
//		t.Error("did not restrict baselining NewApi")
//	}
//
//	bar := result.ModuleForTests("bar", "android_common")
//	sboxProto = android.RuleBuilderSboxProtoForTests(t, bar.Output("lint.sbox.textproto"))
//	if !strings.Contains(*sboxProto.Commands[0].Command,
//		"--baseline lint-baseline.xml --disallowed_issues NewApi") {
//		t.Error("did not restrict baselining NewApi")
//	}
//}

func TestJavaLintDatabaseSelectionFull(t *testing.T) {
	testCases := []struct {
		sdk_version   string
		expected_file string
	}{
		{
			"current",
			"api_versions_public.xml",
		}, {
			"core_platform",
			"api_versions_public.xml",
		}, {
			"system_current",
			"api_versions_system.xml",
		}, {
			"module_current",
			"api_versions_module_lib.xml",
		}, {
			"system_server_current",
			"api_versions_system_server.xml",
		}, {
			"S",
			"api_versions_public.xml",
		}, {
			"30",
			"api_versions_public.xml",
		}, {
			"10000",
			"api_versions_public.xml",
		},
	}
	bp := `
		java_library {
			name: "foo",
			srcs: [
				"a.java",
			],
			min_sdk_version: "29",
			sdk_version: "XXX",
			lint: {
				strict_updatability_linting: true,
			},
		}
`
	for _, testCase := range testCases {
		thisBp := strings.Replace(bp, "XXX", testCase.sdk_version, 1)

		result := android.GroupFixturePreparers(PrepareForTestWithJavaDefaultModules, FixtureWithPrebuiltApis(map[string][]string{
			"30":    {"foo"},
			"10000": {"foo"},
		})).
			RunTestWithBp(t, thisBp)

		foo := result.ModuleForTests("foo", "android_common")
		sboxProto := android.RuleBuilderSboxProtoForTests(t, result.TestContext, foo.Output("lint.sbox.textproto"))
		if !strings.Contains(*sboxProto.Commands[0].Command, "/"+testCase.expected_file) {
			t.Error("did not use full api database for case", testCase)
		}
	}
}
