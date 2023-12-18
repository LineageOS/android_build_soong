// Copyright 2018 Google Inc. All rights reserved.
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

package aconfig

import (
	"testing"

	"android/soong/android"
)

func TestAconfigValueSet(t *testing.T) {
	bp := `
				aconfig_values {
					name: "one",
					srcs: [ "blah.aconfig_values" ],
					package: "foo.package"
				}

				aconfig_value_set {
					name: "module_name",
          values: [ "one" ],
				}
			`
	result := runTest(t, android.FixtureExpectsNoErrors, bp)

	module := result.ModuleForTests("module_name", "").Module().(*ValueSetModule)

	// Check that the provider has the right contents
	depData, _ := android.SingletonModuleProvider(result, module, valueSetProviderKey)
	android.AssertStringEquals(t, "AvailablePackages", "blah.aconfig_values", depData.AvailablePackages["foo.package"][0].String())
}
