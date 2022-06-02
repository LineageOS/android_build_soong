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
	"android/soong/android"
	"android/soong/java"

	"testing"
)

func runAndroidAppCertificateTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	runBp2BuildTestCase(t, registerAndroidAppCertificateModuleTypes, tc)
}

func registerAndroidAppCertificateModuleTypes(ctx android.RegistrationContext) {
}

func TestAndroidAppCertificateSimple(t *testing.T) {
	runAndroidAppCertificateTestCase(t, bp2buildTestCase{
		description:                "Android app certificate - simple example",
		moduleTypeUnderTest:        "android_app_certificate",
		moduleTypeUnderTestFactory: java.AndroidAppCertificateFactory,
		filesystem:                 map[string]string{},
		blueprint: `
android_app_certificate {
        name: "com.android.apogee.cert",
        certificate: "chamber_of_secrets_dir",
}
`,
		expectedBazelTargets: []string{
			makeBazelTargetNoRestrictions("android_app_certificate", "com.android.apogee.cert", attrNameToString{
				"certificate": `"chamber_of_secrets_dir"`,
			}),
		}})
}
