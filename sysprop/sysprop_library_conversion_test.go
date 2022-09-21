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

package sysprop

import (
	"testing"

	"android/soong/bp2build"
)

func TestSyspropLibrarySimple(t *testing.T) {
	bp2build.RunBp2BuildTestCaseSimple(t, bp2build.Bp2buildTestCase{
		Description:                "sysprop_library simple",
		ModuleTypeUnderTest:        "sysprop_library",
		ModuleTypeUnderTestFactory: syspropLibraryFactory,
		Filesystem: map[string]string{
			"foo.sysprop": "",
			"bar.sysprop": "",
		},
		Blueprint: `
sysprop_library {
	name: "sysprop_foo",
	srcs: [
		"foo.sysprop",
		"bar.sysprop",
	],
	property_owner: "Platform",
}
`,
		ExpectedBazelTargets: []string{
			bp2build.MakeBazelTargetNoRestrictions("sysprop_library",
				"sysprop_foo",
				bp2build.AttrNameToString{
					"srcs": `[
        "foo.sysprop",
        "bar.sysprop",
    ]`,
				}),
			bp2build.MakeBazelTargetNoRestrictions("cc_sysprop_library_shared",
				"libsysprop_foo",
				bp2build.AttrNameToString{
					"dep": `":sysprop_foo"`,
				}),
			bp2build.MakeBazelTargetNoRestrictions("cc_sysprop_library_static",
				"libsysprop_foo_bp2build_cc_library_static",
				bp2build.AttrNameToString{
					"dep": `":sysprop_foo"`,
				}),
		},
	})
}

func TestSyspropLibraryCppMinSdkVersion(t *testing.T) {
	bp2build.RunBp2BuildTestCaseSimple(t, bp2build.Bp2buildTestCase{
		Description:                "sysprop_library with min_sdk_version",
		ModuleTypeUnderTest:        "sysprop_library",
		ModuleTypeUnderTestFactory: syspropLibraryFactory,
		Filesystem: map[string]string{
			"foo.sysprop": "",
			"bar.sysprop": "",
		},
		Blueprint: `
sysprop_library {
	name: "sysprop_foo",
	srcs: [
		"foo.sysprop",
		"bar.sysprop",
	],
	cpp: {
		min_sdk_version: "5",
	},
	property_owner: "Platform",
}
`,
		ExpectedBazelTargets: []string{
			bp2build.MakeBazelTargetNoRestrictions("sysprop_library",
				"sysprop_foo",
				bp2build.AttrNameToString{
					"srcs": `[
        "foo.sysprop",
        "bar.sysprop",
    ]`,
				}),
			bp2build.MakeBazelTargetNoRestrictions("cc_sysprop_library_shared",
				"libsysprop_foo",
				bp2build.AttrNameToString{
					"dep":             `":sysprop_foo"`,
					"min_sdk_version": `"5"`,
				}),
			bp2build.MakeBazelTargetNoRestrictions("cc_sysprop_library_static",
				"libsysprop_foo_bp2build_cc_library_static",
				bp2build.AttrNameToString{
					"dep":             `":sysprop_foo"`,
					"min_sdk_version": `"5"`,
				}),
		},
	})
}
