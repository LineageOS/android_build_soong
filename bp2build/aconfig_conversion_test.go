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

	"android/soong/aconfig"
	"android/soong/android"
)

func registerAconfigModuleTypes(ctx android.RegistrationContext) {
	aconfig.RegisterBuildComponents(ctx)
}

func TestAconfigDeclarations(t *testing.T) {
	bp := `
	aconfig_declarations {
		name: "foo",
		srcs: [
			"foo1.aconfig",
			"test/foo2.aconfig",
		],
		package: "com.android.foo",
	}
	`
	expectedBazelTarget := MakeBazelTargetNoRestrictions(
		"aconfig_declarations",
		"foo",
		AttrNameToString{
			"srcs": `[
        "foo1.aconfig",
        "test/foo2.aconfig",
    ]`,
			"package": `"com.android.foo"`,
		},
	)
	RunBp2BuildTestCase(t, registerAconfigModuleTypes, Bp2buildTestCase{
		Blueprint:            bp,
		ExpectedBazelTargets: []string{expectedBazelTarget},
	})
}

func TestAconfigValues(t *testing.T) {
	bp := `
	aconfig_values {
		name: "foo",
		srcs: [
			"foo1.textproto",
		],
		package: "com.android.foo",
	}
	aconfig_value_set {
    name: "bar",
    values: [
        "foo"
    ]
	}
	`
	expectedBazelTargets := []string{
		MakeBazelTargetNoRestrictions(
			"aconfig_values",
			"foo",
			AttrNameToString{
				"srcs":    `["foo1.textproto"]`,
				"package": `"com.android.foo"`,
			},
		),
		MakeBazelTargetNoRestrictions(
			"aconfig_value_set",
			"bar",
			AttrNameToString{
				"values": `[":foo"]`,
			},
		)}
	RunBp2BuildTestCase(t, registerAconfigModuleTypes, Bp2buildTestCase{
		Blueprint:            bp,
		ExpectedBazelTargets: expectedBazelTargets,
	})
}
