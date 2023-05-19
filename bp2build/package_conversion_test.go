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
	"android/soong/genrule"
)

func registerDependentModules(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("license", android.LicenseFactory)
	ctx.RegisterModuleType("genrule", genrule.GenRuleFactory)
}

func TestPackage(t *testing.T) {
	tests := []struct {
		description string
		modules     string
		fs          map[string]string
		expected    []ExpectedRuleTarget
	}{
		{
			description: "with default applicable licenses",
			modules: `
license {
  name: "my_license",
  visibility: [":__subpackages__"],
  license_kinds: ["SPDX-license-identifier-Apache-2.0"],
  license_text: ["NOTICE"],
}

package {
  default_applicable_licenses: ["my_license"],
}
`,
			expected: []ExpectedRuleTarget{
				{
					"package",
					"",
					AttrNameToString{
						"default_package_metadata": `[":my_license"]`,
						"default_visibility":       `["//visibility:public"]`,
					},
					android.HostAndDeviceDefault,
				},
				{
					"android_license",
					"my_license",
					AttrNameToString{
						"license_kinds": `["SPDX-license-identifier-Apache-2.0"]`,
						"license_text":  `"NOTICE"`,
						"visibility":    `[":__subpackages__"]`,
					},
					android.HostAndDeviceDefault,
				},
			},
		},
		{
			description: "package has METADATA file",
			fs: map[string]string{
				"METADATA": ``,
			},
			modules: `
license {
  name: "my_license",
  visibility: [":__subpackages__"],
  license_kinds: ["SPDX-license-identifier-Apache-2.0"],
  license_text: ["NOTICE"],
}

package {
  default_applicable_licenses: ["my_license"],
}
`,
			expected: []ExpectedRuleTarget{
				{
					"package",
					"",
					AttrNameToString{
						"default_package_metadata": `[
        ":my_license",
        ":default_metadata_file",
    ]`,
						"default_visibility": `["//visibility:public"]`,
					},
					android.HostAndDeviceDefault,
				},
				{
					"android_license",
					"my_license",
					AttrNameToString{
						"license_kinds": `["SPDX-license-identifier-Apache-2.0"]`,
						"license_text":  `"NOTICE"`,
						"visibility":    `[":__subpackages__"]`,
					},
					android.HostAndDeviceDefault,
				},
				{
					"filegroup",
					"default_metadata_file",
					AttrNameToString{
						"applicable_licenses": `[]`,
						"srcs":                `["METADATA"]`,
					},
					android.HostAndDeviceDefault,
				},
			},
		},
	}
	for _, test := range tests {
		expected := make([]string, 0, len(test.expected))
		for _, e := range test.expected {
			expected = append(expected, e.String())
		}
		RunBp2BuildTestCase(t, registerDependentModules,
			Bp2buildTestCase{
				Description:                test.description,
				ModuleTypeUnderTest:        "package",
				ModuleTypeUnderTestFactory: android.PackageFactory,
				Blueprint:                  test.modules,
				ExpectedBazelTargets:       expected,
				Filesystem:                 test.fs,
			})
	}
}
