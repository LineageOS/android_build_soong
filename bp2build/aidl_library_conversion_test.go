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

	"android/soong/aidl_library"
	"android/soong/android"
)

func runAidlLibraryTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	(&tc).ModuleTypeUnderTest = "aidl_library"
	(&tc).ModuleTypeUnderTestFactory = aidl_library.AidlLibraryFactory
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, tc)
}

func TestAidlLibrary(t *testing.T) {
	testcases := []struct {
		name               string
		bp                 string
		expectedBazelAttrs AttrNameToString
	}{
		{
			name: "aidl_library with strip_import_prefix",
			bp: `
	aidl_library {
		name: "foo",
		srcs: ["aidl/foo.aidl"],
		hdrs: ["aidl/header.aidl"],
		strip_import_prefix: "aidl",
	}`,
			expectedBazelAttrs: AttrNameToString{
				"srcs":                `["aidl/foo.aidl"]`,
				"hdrs":                `["aidl/header.aidl"]`,
				"strip_import_prefix": `"aidl"`,
				"tags":                `["apex_available=//apex_available:anyapex"]`,
			},
		},
		{
			name: "aidl_library without strip_import_prefix",
			bp: `
	aidl_library {
		name: "foo",
		srcs: ["aidl/foo.aidl"],
		hdrs: ["aidl/header.aidl"],
	}`,
			expectedBazelAttrs: AttrNameToString{
				"srcs": `["aidl/foo.aidl"]`,
				"hdrs": `["aidl/header.aidl"]`,
				"tags": `["apex_available=//apex_available:anyapex"]`,
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedBazelTargets := []string{
				MakeBazelTargetNoRestrictions("aidl_library", "foo", test.expectedBazelAttrs),
			}
			runAidlLibraryTestCase(t, Bp2buildTestCase{
				Description:          test.name,
				Blueprint:            test.bp,
				ExpectedBazelTargets: expectedBazelTargets,
			})
		})
	}
}

func TestAidlLibraryWithDeps(t *testing.T) {
	bp := `
	aidl_library {
		name: "bar",
		srcs: ["Bar.aidl"],
		hdrs: ["aidl/BarHeader.aidl"],
	}
	aidl_library {
		name: "foo",
		srcs: ["aidl/Foo.aidl"],
		hdrs: ["aidl/FooHeader.aidl"],
		strip_import_prefix: "aidl",
		deps: ["bar"],
	}`

	t.Run("aidl_library with deps", func(t *testing.T) {
		expectedBazelTargets := []string{
			MakeBazelTargetNoRestrictions("aidl_library", "bar", AttrNameToString{
				"srcs": `["Bar.aidl"]`,
				"hdrs": `["aidl/BarHeader.aidl"]`,
				"tags": `["apex_available=//apex_available:anyapex"]`,
			}),
			MakeBazelTargetNoRestrictions("aidl_library", "foo", AttrNameToString{
				"srcs":                `["aidl/Foo.aidl"]`,
				"hdrs":                `["aidl/FooHeader.aidl"]`,
				"strip_import_prefix": `"aidl"`,
				"deps":                `[":bar"]`,
				"tags":                `["apex_available=//apex_available:anyapex"]`,
			}),
		}
		runAidlLibraryTestCase(t, Bp2buildTestCase{
			Description:          "aidl_library with deps",
			Blueprint:            bp,
			ExpectedBazelTargets: expectedBazelTargets,
		})
	})
}
