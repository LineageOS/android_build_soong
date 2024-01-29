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

	"github.com/google/blueprint"
)

type defaultsTestProperties struct {
	Foo       []string
	Path_prop []string `android:"path"`
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

func TestDefaultsPathProperties(t *testing.T) {
	bp := `
		defaults {
			name: "defaults",
			path_prop: [":gen"],
		}

		test {
			name: "foo",
			defaults: ["defaults"],
		}

		test {
			name: "gen",
		}
	`

	result := GroupFixturePreparers(
		prepareForDefaultsTest,
		FixtureWithRootAndroidBp(bp),
	).RunTest(t)

	collectDeps := func(m Module) []string {
		var deps []string
		result.VisitDirectDeps(m, func(dep blueprint.Module) {
			deps = append(deps, result.ModuleName(dep))
		})
		return deps
	}

	foo := result.Module("foo", "")
	defaults := result.Module("defaults", "")

	AssertStringListContains(t, "foo should depend on gen", collectDeps(foo), "gen")
	AssertStringListDoesNotContain(t, "defaults should not depend on gen", collectDeps(defaults), "gen")
}
