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
	"reflect"
	"testing"

	"github.com/google/blueprint/proptools"
)

type defaultsTestProperties struct {
	Foo []string
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

	config := TestConfig(buildDir, nil, bp, nil)

	ctx := NewTestContext()

	ctx.RegisterModuleType("test", defaultsTestModuleFactory)
	ctx.RegisterModuleType("defaults", defaultsTestDefaultsFactory)

	ctx.PreArchMutators(RegisterDefaultsPreArchMutators)

	ctx.Register(config)

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	FailIfErrored(t, errs)

	foo := ctx.ModuleForTests("foo", "").Module().(*defaultsTestModule)

	if g, w := foo.properties.Foo, []string{"transitive", "defaults", "module"}; !reflect.DeepEqual(g, w) {
		t.Errorf("expected foo %q, got %q", w, g)
	}
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

	config := TestConfig(buildDir, nil, bp, nil)
	config.TestProductVariables.Allow_missing_dependencies = proptools.BoolPtr(true)

	ctx := NewTestContext()
	ctx.SetAllowMissingDependencies(true)

	ctx.RegisterModuleType("test", defaultsTestModuleFactory)
	ctx.RegisterModuleType("defaults", defaultsTestDefaultsFactory)

	ctx.PreArchMutators(RegisterDefaultsPreArchMutators)

	ctx.Register(config)

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	FailIfErrored(t, errs)

	missingDefaults := ctx.ModuleForTests("missing_defaults", "").Output("out")
	missingTransitiveDefaults := ctx.ModuleForTests("missing_transitive_defaults", "").Output("out")

	if missingDefaults.Rule != ErrorRule {
		t.Errorf("expected missing_defaults rule to be ErrorRule, got %#v", missingDefaults.Rule)
	}

	if g, w := missingDefaults.Args["error"], "module missing_defaults missing dependencies: missing\n"; g != w {
		t.Errorf("want error %q, got %q", w, g)
	}

	// TODO: missing transitive defaults is currently not handled
	_ = missingTransitiveDefaults
}
