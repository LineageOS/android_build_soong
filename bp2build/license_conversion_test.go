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
	"android/soong/android"
	"testing"
)

func registerLicenseModuleTypes(_ android.RegistrationContext) {}

func TestLicenseBp2Build(t *testing.T) {
	tests := []struct {
		description string
		module      string
		expected    ExpectedRuleTarget
	}{
		{
			description: "license kind and text notice",
			module: `
license {
    name: "my_license",
    license_kinds: [ "SPDX-license-identifier-Apache-2.0"],
    license_text: [ "NOTICE"],
}`,
			expected: ExpectedRuleTarget{
				"android_license",
				"my_license",
				AttrNameToString{
					"license_kinds": `["SPDX-license-identifier-Apache-2.0"]`,
					"license_text":  `"NOTICE"`,
				},
				android.HostAndDeviceDefault,
			},
		},
		{
			description: "visibility, package_name, copyright_notice",
			module: `
license {
	name: "my_license",
    package_name: "my_package",
    visibility: [":__subpackages__"],
    copyright_notice: "Copyright © 2022",
}`,
			expected: ExpectedRuleTarget{
				"android_license",
				"my_license",
				AttrNameToString{
					"copyright_notice": `"Copyright © 2022"`,
					"package_name":     `"my_package"`,
					"visibility":       `[":__subpackages__"]`,
				},
				android.HostAndDeviceDefault,
			},
		},
	}

	for _, test := range tests {
		RunBp2BuildTestCase(t,
			registerLicenseModuleTypes,
			Bp2buildTestCase{
				Description:                test.description,
				ModuleTypeUnderTest:        "license",
				ModuleTypeUnderTestFactory: android.LicenseFactory,
				Blueprint:                  test.module,
				ExpectedBazelTargets:       []string{test.expected.String()},
			})
	}
}
