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

package bp2build

import (
	"testing"

	"android/soong/android"
	"android/soong/sh"
)

func TestShBinaryLoadStatement(t *testing.T) {
	testCases := []struct {
		bazelTargets           BazelTargets
		expectedLoadStatements string
	}{
		{
			bazelTargets: BazelTargets{
				BazelTarget{
					name:      "sh_binary_target",
					ruleClass: "sh_binary",
					// Note: no bzlLoadLocation for native rules
					// TODO(ruperts): Could open source the existing, experimental Starlark sh_ rules?
				},
			},
			expectedLoadStatements: ``,
		},
	}

	for _, testCase := range testCases {
		actual := testCase.bazelTargets.LoadStatements()
		expected := testCase.expectedLoadStatements
		if actual != expected {
			t.Fatalf("Expected load statements to be %s, got %s", expected, actual)
		}
	}
}

func runShBinaryTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, tc)
}

func TestShBinarySimple(t *testing.T) {
	runShBinaryTestCase(t, Bp2buildTestCase{
		Description:                "sh_binary test",
		ModuleTypeUnderTest:        "sh_binary",
		ModuleTypeUnderTestFactory: sh.ShBinaryFactory,
		Blueprint: `sh_binary {
    name: "foo",
    src: "foo.sh",
    filename: "foo.exe",
    sub_dir: "sub",
    bazel_module: { bp2build_available: true },
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("sh_binary", "foo", AttrNameToString{
				"srcs":     `["foo.sh"]`,
				"filename": `"foo.exe"`,
				"sub_dir":  `"sub"`,
			})},
	})
}

func TestShBinaryDefaults(t *testing.T) {
	runShBinaryTestCase(t, Bp2buildTestCase{
		Description:                "sh_binary test",
		ModuleTypeUnderTest:        "sh_binary",
		ModuleTypeUnderTestFactory: sh.ShBinaryFactory,
		Blueprint: `sh_binary {
    name: "foo",
    src: "foo.sh",
    bazel_module: { bp2build_available: true },
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("sh_binary", "foo", AttrNameToString{
				"srcs": `["foo.sh"]`,
			})},
	})
}
