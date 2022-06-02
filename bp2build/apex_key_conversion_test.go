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
	"android/soong/apex"

	"testing"
)

func runApexKeyTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	runBp2BuildTestCase(t, registerApexKeyModuleTypes, tc)
}

func registerApexKeyModuleTypes(ctx android.RegistrationContext) {
}

func TestApexKeySimple(t *testing.T) {
	runApexKeyTestCase(t, bp2buildTestCase{
		description:                "apex key - simple example",
		moduleTypeUnderTest:        "apex_key",
		moduleTypeUnderTestFactory: apex.ApexKeyFactory,
		filesystem:                 map[string]string{},
		blueprint: `
apex_key {
        name: "com.android.apogee.key",
        public_key: "com.android.apogee.avbpubkey",
        private_key: "com.android.apogee.pem",
}
`,
		expectedBazelTargets: []string{makeBazelTargetNoRestrictions("apex_key", "com.android.apogee.key", attrNameToString{
			"private_key": `"com.android.apogee.pem"`,
			"public_key":  `"com.android.apogee.avbpubkey"`,
		}),
		}})
}
