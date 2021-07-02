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

func runApexTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	runBp2BuildTestCase(t, registerApexModuleTypes, tc)
}

func registerApexModuleTypes(ctx android.RegistrationContext) {
}

func TestApexBundleSimple(t *testing.T) {
	runApexTestCase(t, bp2buildTestCase{
		description:                        "apex - simple example",
		moduleTypeUnderTest:                "apex",
		moduleTypeUnderTestFactory:         apex.BundleFactory,
		moduleTypeUnderTestBp2BuildMutator: apex.ApexBundleBp2Build,
		filesystem:                         map[string]string{},
		blueprint: `
apex {
	name: "apogee",
	manifest: "manifest.json",
}
`,
		expectedBazelTargets: []string{`apex(
    name = "apogee",
    manifest = "manifest.json",
)`}})
}
