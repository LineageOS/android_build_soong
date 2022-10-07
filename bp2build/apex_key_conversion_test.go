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

func runApexKeyTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, registerApexKeyModuleTypes, tc)
}

func registerApexKeyModuleTypes(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
}

func TestApexKeySimple_KeysAreSrcFiles(t *testing.T) {
	runApexKeyTestCase(t, Bp2buildTestCase{
		Description:                "apex key - keys are src files, use key_name attributes",
		ModuleTypeUnderTest:        "apex_key",
		ModuleTypeUnderTestFactory: apex.ApexKeyFactory,
		Filesystem:                 map[string]string{},
		Blueprint: `
apex_key {
        name: "com.android.apogee.key",
        public_key: "com.android.apogee.avbpubkey",
        private_key: "com.android.apogee.pem",
}
`,
		ExpectedBazelTargets: []string{MakeBazelTargetNoRestrictions("apex_key", "com.android.apogee.key", AttrNameToString{
			"private_key_name": `"com.android.apogee.pem"`,
			"public_key_name":  `"com.android.apogee.avbpubkey"`,
		}),
		}})
}

func TestApexKey_KeysAreModules(t *testing.T) {
	runApexKeyTestCase(t, Bp2buildTestCase{
		Description:                "apex key - keys are modules, use key attributes",
		ModuleTypeUnderTest:        "apex_key",
		ModuleTypeUnderTestFactory: apex.ApexKeyFactory,
		Filesystem:                 map[string]string{},
		Blueprint: `
apex_key {
        name: "com.android.apogee.key",
        public_key: ":com.android.apogee.avbpubkey",
        private_key: ":com.android.apogee.pem",
}
` + simpleModuleDoNotConvertBp2build("filegroup", "com.android.apogee.avbpubkey") +
			simpleModuleDoNotConvertBp2build("filegroup", "com.android.apogee.pem"),
		ExpectedBazelTargets: []string{MakeBazelTargetNoRestrictions("apex_key", "com.android.apogee.key", AttrNameToString{
			"private_key": `":com.android.apogee.pem"`,
			"public_key":  `":com.android.apogee.avbpubkey"`,
		}),
		}})
}
