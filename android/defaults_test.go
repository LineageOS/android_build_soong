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

type defaultsTestProperties struct {
	Foo    []string
	Bar    []string
	Nested struct {
		Fizz *bool
	}
	Other struct {
		Buzz *string
	}
}

type defaultsTestModule struct {
	ModuleBase
	DefaultableModuleBase
	properties defaultsTestProperties
}

func (d *defaultsTestModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	ctx.Build(pctx, BuildParams{
		Rule:   Touch,
		Output: PathForModuleOut(ctx, "out"),
	})
}

func defaultsTestModuleFactory() Module {
	module := &defaultsTestModule{}
	module.AddProperties(&module.properties)
	InitAndroidModule(module)
	InitDefaultableModule(module)
	return module
}

type defaultsTestDefaults struct {
	ModuleBase
	DefaultsModuleBase
}

func defaultsTestDefaultsFactory() Module {
	defaults := &defaultsTestDefaults{}
	defaults.AddProperties(&defaultsTestProperties{})
	InitDefaultsModule(defaults)
	return defaults
}

var prepareForDefaultsTest = GroupFixturePreparers(
	PrepareForTestWithDefaults,
	FixtureRegisterWithContext(func(ctx RegistrationContext) {
		ctx.RegisterModuleType("test", defaultsTestModuleFactory)
		ctx.RegisterModuleType("defaults", defaultsTestDefaultsFactory)
	}),
)

func TestDefaults(t *testing.T) {
	bp := `
		defaults {
			name: "transitive",
			foo: ["transitive"],
		}

		defaults {
			name: "defaults",
			defaults: ["transitive"],
			foo: ["defaults"],
		}

		test {
			name: "foo",
			defaults: ["defaults"],
			foo: ["module"],
		}
	`

	result := GroupFixturePreparers(
		prepareForDefaultsTest,
		FixtureWithRootAndroidBp(bp),
	).RunTest(t)

	foo := result.Module("foo", "").(*defaultsTestModule)

	AssertDeepEquals(t, "foo", []string{"transitive", "defaults", "module"}, foo.properties.Foo)
}

func TestDefaultsAllowMissingDependencies(t *testing.T) {
	bp := `
		defaults {
			name: "defaults",
			defaults: ["missing"],
			foo: ["defaults"],
		}

		test {
			name: "missing_defaults",
			defaults: ["missing"],
			foo: ["module"],
		}

		test {
			name: "missing_transitive_defaults",
			defaults: ["defaults"],
			foo: ["module"],
		}
	`

	result := GroupFixturePreparers(
		prepareForDefaultsTest,
		PrepareForTestWithAllowMissingDependencies,
		FixtureWithRootAndroidBp(bp),
	).RunTest(t)

	missingDefaults := result.ModuleForTests("missing_defaults", "").Output("out")
	missingTransitiveDefaults := result.ModuleForTests("missing_transitive_defaults", "").Output("out")

	AssertSame(t, "missing_defaults rule", ErrorRule, missingDefaults.Rule)

	AssertStringEquals(t, "missing_defaults", "module missing_defaults missing dependencies: missing\n", missingDefaults.Args["error"])

	// TODO: missing transitive defaults is currently not handled
	_ = missingTransitiveDefaults
}

func TestProtectedProperties_ProtectedPropertyNotSet(t *testing.T) {
	bp := `
		defaults {
			name: "transitive",
			protected_properties: ["foo"],
		}
	`

	GroupFixturePreparers(
		prepareForDefaultsTest,
		FixtureWithRootAndroidBp(bp),
	).ExtendWithErrorHandler(FixtureExpectsAtLeastOneErrorMatchingPattern(
		"module \"transitive\": foo: is not set; protected properties must be explicitly set")).
		RunTest(t)
}

func TestProtectedProperties_ProtectedPropertyNotLeaf(t *testing.T) {
	bp := `
		defaults {
			name: "transitive",
			protected_properties: ["nested"],
			nested: {
				fizz: true,
			},
		}
	`

	GroupFixturePreparers(
		prepareForDefaultsTest,
		FixtureWithRootAndroidBp(bp),
	).ExtendWithErrorHandler(FixtureExpectsAtLeastOneErrorMatchingPattern(
		`\Qmodule "transitive": nested: property is not supported by this module type "defaults"\E`)).
		RunTest(t)
}

// TestProtectedProperties_ApplyDefaults makes sure that the protected_properties property has
// defaults applied.
func TestProtectedProperties_HasDefaultsApplied(t *testing.T) {

	bp := `
		defaults {
			name: "transitive",
			protected_properties: ["foo"],
			foo: ["transitive"],
		}

		defaults {
			name: "defaults",
			defaults: ["transitive"],
			protected_properties: ["bar"],
			bar: ["defaults"],
		}
	`

	result := GroupFixturePreparers(
		prepareForDefaultsTest,
		FixtureWithRootAndroidBp(bp),
	).RunTest(t)

	defaults := result.Module("defaults", "").(DefaultsModule)
	AssertDeepEquals(t, "defaults protected properties", []string{"foo", "bar"}, defaults.protectedProperties())
}

// TestProtectedProperties_ProtectAllProperties makes sure that protected_properties: ["*"] protects
// all properties.
func TestProtectedProperties_ProtectAllProperties(t *testing.T) {

	bp := `
		defaults {
			name: "transitive",
			protected_properties: ["other.buzz"],
			other: {
				buzz: "transitive",
			},
		}

		defaults {
			name: "defaults",
			defaults: ["transitive"],
			visibility: ["//visibility:private"],
			protected_properties: ["*"],
			foo: ["other"],
			bar: ["defaults"],
			nested: {
				fizz: true,
			}
		}
	`

	result := GroupFixturePreparers(
		prepareForDefaultsTest,
		FixtureWithRootAndroidBp(bp),
	).RunTest(t)

	defaults := result.Module("defaults", "").(DefaultsModule)
	AssertDeepEquals(t, "defaults protected properties", []string{"other.buzz", "bar", "foo", "nested.fizz"},
		defaults.protectedProperties())
}

func TestProtectedProperties_DetectedOverride(t *testing.T) {
	bp := `
		defaults {
			name: "defaults",
			protected_properties: ["foo", "nested.fizz"],
			foo: ["defaults"],
			nested: {
				fizz: true,
			},
		}

		test {
			name: "foo",
			defaults: ["defaults"],
			foo: ["module"],
			nested: {
				fizz: false,
			},
		}
	`

	GroupFixturePreparers(
		prepareForDefaultsTest,
		FixtureWithRootAndroidBp(bp),
	).ExtendWithErrorHandler(FixtureExpectsAllErrorsToMatchAPattern(
		[]string{
			`\Qmodule "foo": attempts to append ["module"] to protected property "foo"'s value of ["defaults"] defined in module "defaults"\E`,
			`\Qmodule "foo": attempts to override protected property "nested.fizz" defined in module "defaults" with a different value (override true with false) so removing the property may necessitate other changes.\E`,
		})).RunTest(t)
}

func TestProtectedProperties_DefaultsConflict(t *testing.T) {
	bp := `
		defaults {
			name: "defaults1",
			protected_properties: ["other.buzz"],
			other: {
				buzz: "value",
			},
		}

		defaults {
			name: "defaults2",
			protected_properties: ["other.buzz"],
			other: {
				buzz: "another",
			},
		}

		test {
			name: "foo",
			defaults: ["defaults1", "defaults2"],
		}
	`

	GroupFixturePreparers(
		prepareForDefaultsTest,
		FixtureWithRootAndroidBp(bp),
	).ExtendWithErrorHandler(FixtureExpectsAtLeastOneErrorMatchingPattern(
		`\Qmodule "foo": has conflicting default values for protected property "other.buzz":
    defaults module "defaults1" provides value "value"
    defaults module "defaults2" provides value "another"\E`,
	)).RunTest(t)
}
