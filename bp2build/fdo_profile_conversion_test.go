// Copyright 2023 Google Inc. All rights reserved.
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
	"android/soong/cc"
)

func runFdoProfileTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	(&tc).ModuleTypeUnderTest = "fdo_profile"
	(&tc).ModuleTypeUnderTestFactory = cc.FdoProfileFactory
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, tc)
}

func TestFdoProfile(t *testing.T) {
	testcases := []struct {
		name               string
		bp                 string
		expectedBazelAttrs AttrNameToString
	}{
		{
			name: "fdo_profile with arch-specific profiles",
			bp: `
fdo_profile {
	name: "foo",
	arch: {
		arm: {
			profile: "foo_arm.afdo",
		},
		arm64: {
			profile: "foo_arm64.afdo",
		}
	}
}`,
			expectedBazelAttrs: AttrNameToString{
				"profile": `select({
        "//build/bazel/platforms/arch:arm": "foo_arm.afdo",
        "//build/bazel/platforms/arch:arm64": "foo_arm64.afdo",
        "//conditions:default": None,
    })`,
			},
		},
		{
			name: "fdo_profile with arch-agnostic profile",
			bp: `
fdo_profile {
	name: "foo",
	profile: "foo.afdo",
}`,
			expectedBazelAttrs: AttrNameToString{
				"profile": `"foo.afdo"`,
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedBazelTargets := []string{
				// TODO(b/276287371): Add device-only restriction back to fdo_profile targets
				MakeBazelTargetNoRestrictions("fdo_profile", "foo", test.expectedBazelAttrs),
			}
			runFdoProfileTestCase(t, Bp2buildTestCase{
				Description:          test.name,
				Blueprint:            test.bp,
				ExpectedBazelTargets: expectedBazelTargets,
			})
		})
	}
}
