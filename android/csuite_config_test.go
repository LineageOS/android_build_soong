// Copyright 2019 Google Inc. All rights reserved.
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

package android

import (
	"testing"
)

func TestCSuiteConfig(t *testing.T) {
	result := emptyTestFixtureFactory.RunTest(t,
		PrepareForTestWithArchMutator,
		FixtureRegisterWithContext(registerCSuiteBuildComponents),
		FixtureWithRootAndroidBp(`
			csuite_config { name: "plain"}
			csuite_config { name: "with_manifest", test_config: "manifest.xml" }
		`),
	)

	variants := result.ModuleVariantsForTests("plain")
	if len(variants) > 1 {
		t.Errorf("expected 1, got %d", len(variants))
	}
	outputFilename := result.ModuleForTests(
		"plain", variants[0]).Module().(*CSuiteConfig).OutputFilePath.Base()
	AssertStringEquals(t, "output file name", "plain", outputFilename)
}
