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
	"android/soong/cc"
	"android/soong/java"
)

func registerAconfigModuleTypes(ctx android.RegistrationContext) {
	aconfig.RegisterBuildComponents(ctx)
	ctx.RegisterModuleType("cc_library", cc.LibraryFactory)
	ctx.RegisterModuleType("java_library", java.LibraryFactory)
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

func TestCcAconfigLibrary(t *testing.T) {
	bp := `
	aconfig_declarations {
		name: "foo_aconfig_declarations",
		srcs: [
			"foo1.aconfig",
		],
		package: "com.android.foo",
	}
	cc_library {
			name: "server_configurable_flags",
			srcs: ["bar.cc"],
	}
	cc_aconfig_library {
			name: "foo",
			aconfig_declarations: "foo_aconfig_declarations",
	}
	`
	expectedBazelTargets := []string{
		MakeBazelTargetNoRestrictions(
			"aconfig_declarations",
			"foo_aconfig_declarations",
			AttrNameToString{
				"srcs":    `["foo1.aconfig"]`,
				"package": `"com.android.foo"`,
			},
		),
		MakeBazelTargetNoRestrictions(
			"cc_aconfig_library",
			"foo",
			AttrNameToString{
				"aconfig_declarations":   `":foo_aconfig_declarations"`,
				"dynamic_deps":           `[":server_configurable_flags"]`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
			},
		)}
	RunBp2BuildTestCase(t, registerAconfigModuleTypes, Bp2buildTestCase{
		Blueprint:               bp,
		ExpectedBazelTargets:    expectedBazelTargets,
		StubbedBuildDefinitions: []string{"server_configurable_flags"},
	})
}

func TestJavaAconfigLibrary(t *testing.T) {
	bp := `
	aconfig_declarations {
		name: "foo_aconfig_declarations",
		srcs: [
			"foo1.aconfig",
		],
		package: "com.android.foo",
	}
	java_aconfig_library {
			name: "foo",
			aconfig_declarations: "foo_aconfig_declarations",
			test: true,
	}
	`
	expectedBazelTargets := []string{
		MakeBazelTargetNoRestrictions(
			"aconfig_declarations",
			"foo_aconfig_declarations",
			AttrNameToString{
				"srcs":    `["foo1.aconfig"]`,
				"package": `"com.android.foo"`,
			},
		),
		MakeBazelTargetNoRestrictions(
			"java_aconfig_library",
			"foo",
			AttrNameToString{
				"aconfig_declarations":   `":foo_aconfig_declarations"`,
				"test":                   `True`,
				"sdk_version":            `"system_current"`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
			},
		)}
	RunBp2BuildTestCase(t, registerAconfigModuleTypes, Bp2buildTestCase{
		Blueprint:            bp,
		ExpectedBazelTargets: expectedBazelTargets,
	})
}

func TestJavaAconfigLibraryAsTaggedOutput(t *testing.T) {
	bp := `
	aconfig_declarations {
		name: "foo_aconfig_declarations",
		srcs: [
			"foo.aconfig",
		],
		package: "com.android.foo",
	}
	java_library {
			name: "foo_library",
			srcs: [":foo_aconfig_library{.generated_srcjars}"],
			sdk_version: "current",
			bazel_module: { bp2build_available: true },
	}
	java_aconfig_library {
			name: "foo_aconfig_library",
			aconfig_declarations: "foo_aconfig_declarations",
			test: true,
	}
	`
	expectedBazelTargets := []string{
		MakeBazelTargetNoRestrictions(
			"aconfig_declarations",
			"foo_aconfig_declarations",
			AttrNameToString{
				"srcs":    `["foo.aconfig"]`,
				"package": `"com.android.foo"`,
			},
		),
		MakeBazelTargetNoRestrictions(
			"java_aconfig_library",
			"foo_aconfig_library",
			AttrNameToString{
				"aconfig_declarations":   `":foo_aconfig_declarations"`,
				"test":                   `True`,
				"sdk_version":            `"system_current"`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
			},
		),
		MakeBazelTargetNoRestrictions(
			"java_library",
			"foo_library",
			AttrNameToString{
				"srcs":                   `[":foo_aconfig_library.generated_srcjars"]`,
				"sdk_version":            `"current"`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
			},
		),
		MakeNeverlinkDuplicateTarget("java_library", "foo_library"),
	}

	RunBp2BuildTestCase(t, registerAconfigModuleTypes, Bp2buildTestCase{
		Blueprint:            bp,
		ExpectedBazelTargets: expectedBazelTargets,
	})
}
