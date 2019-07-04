// Copyright 2015 Google Inc. All rights reserved.
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

type mutatorTestModule struct {
	ModuleBase
	props struct {
		Deps_missing_deps    []string
		Mutator_missing_deps []string
	}

	missingDeps []string
}

func mutatorTestModuleFactory() Module {
	module := &mutatorTestModule{}
	module.AddProperties(&module.props)
	InitAndroidModule(module)
	return module
}

func (m *mutatorTestModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	ctx.Build(pctx, BuildParams{
		Rule:   Touch,
		Output: PathForModuleOut(ctx, "output"),
	})

	m.missingDeps = ctx.GetMissingDependencies()
}

func (m *mutatorTestModule) DepsMutator(ctx BottomUpMutatorContext) {
	ctx.AddDependency(ctx.Module(), nil, m.props.Deps_missing_deps...)
}

func addMissingDependenciesMutator(ctx TopDownMutatorContext) {
	ctx.AddMissingDependencies(ctx.Module().(*mutatorTestModule).props.Mutator_missing_deps)
}

func TestMutatorAddMissingDependencies(t *testing.T) {
	config := TestConfig(buildDir, nil)
	config.TestProductVariables.Allow_missing_dependencies = proptools.BoolPtr(true)

	ctx := NewTestContext()
	ctx.SetAllowMissingDependencies(true)

	ctx.RegisterModuleType("test", ModuleFactoryAdaptor(mutatorTestModuleFactory))
	ctx.PreDepsMutators(func(ctx RegisterMutatorsContext) {
		ctx.TopDown("add_missing_dependencies", addMissingDependenciesMutator)
	})

	bp := `
		test {
			name: "foo",
			deps_missing_deps: ["regular_missing_dep"],
			mutator_missing_deps: ["added_missing_dep"],
		}
	`

	mockFS := map[string][]byte{
		"Android.bp": []byte(bp),
	}

	ctx.MockFileSystem(mockFS)

	ctx.Register()
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	FailIfErrored(t, errs)

	foo := ctx.ModuleForTests("foo", "").Module().(*mutatorTestModule)

	if g, w := foo.missingDeps, []string{"added_missing_dep", "regular_missing_dep"}; !reflect.DeepEqual(g, w) {
		t.Errorf("want foo missing deps %q, got %q", w, g)
	}
}

func TestModuleString(t *testing.T) {
	ctx := NewTestContext()

	var moduleStrings []string

	ctx.PreArchMutators(func(ctx RegisterMutatorsContext) {
		ctx.BottomUp("pre_arch", func(ctx BottomUpMutatorContext) {
			moduleStrings = append(moduleStrings, ctx.Module().String())
			ctx.CreateVariations("a", "b")
		})
		ctx.TopDown("rename_top_down", func(ctx TopDownMutatorContext) {
			moduleStrings = append(moduleStrings, ctx.Module().String())
			ctx.Rename(ctx.Module().base().Name() + "_renamed1")
		})
	})

	ctx.PreDepsMutators(func(ctx RegisterMutatorsContext) {
		ctx.BottomUp("pre_deps", func(ctx BottomUpMutatorContext) {
			moduleStrings = append(moduleStrings, ctx.Module().String())
			ctx.CreateVariations("c", "d")
		})
	})

	ctx.PostDepsMutators(func(ctx RegisterMutatorsContext) {
		ctx.BottomUp("post_deps", func(ctx BottomUpMutatorContext) {
			moduleStrings = append(moduleStrings, ctx.Module().String())
			ctx.CreateLocalVariations("e", "f")
		})
		ctx.BottomUp("rename_bottom_up", func(ctx BottomUpMutatorContext) {
			moduleStrings = append(moduleStrings, ctx.Module().String())
			ctx.Rename(ctx.Module().base().Name() + "_renamed2")
		})
		ctx.BottomUp("final", func(ctx BottomUpMutatorContext) {
			moduleStrings = append(moduleStrings, ctx.Module().String())
		})
	})

	ctx.RegisterModuleType("test", ModuleFactoryAdaptor(mutatorTestModuleFactory))

	bp := `
		test {
			name: "foo",
		}
	`

	mockFS := map[string][]byte{
		"Android.bp": []byte(bp),
	}

	ctx.MockFileSystem(mockFS)

	ctx.Register()

	config := TestConfig(buildDir, nil)

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	FailIfErrored(t, errs)

	want := []string{
		// Initial name.
		"foo{}",

		// After pre_arch (reversed because rename_top_down is TopDown so it visits in reverse order).
		"foo{pre_arch:b}",
		"foo{pre_arch:a}",

		// After rename_top_down.
		"foo_renamed1{pre_arch:a}",
		"foo_renamed1{pre_arch:b}",

		// After pre_deps.
		"foo_renamed1{pre_arch:a,pre_deps:c}",
		"foo_renamed1{pre_arch:a,pre_deps:d}",
		"foo_renamed1{pre_arch:b,pre_deps:c}",
		"foo_renamed1{pre_arch:b,pre_deps:d}",

		// After post_deps.
		"foo_renamed1{pre_arch:a,pre_deps:c,post_deps:e}",
		"foo_renamed1{pre_arch:a,pre_deps:c,post_deps:f}",
		"foo_renamed1{pre_arch:a,pre_deps:d,post_deps:e}",
		"foo_renamed1{pre_arch:a,pre_deps:d,post_deps:f}",
		"foo_renamed1{pre_arch:b,pre_deps:c,post_deps:e}",
		"foo_renamed1{pre_arch:b,pre_deps:c,post_deps:f}",
		"foo_renamed1{pre_arch:b,pre_deps:d,post_deps:e}",
		"foo_renamed1{pre_arch:b,pre_deps:d,post_deps:f}",

		// After rename_bottom_up.
		"foo_renamed2{pre_arch:a,pre_deps:c,post_deps:e}",
		"foo_renamed2{pre_arch:a,pre_deps:c,post_deps:f}",
		"foo_renamed2{pre_arch:a,pre_deps:d,post_deps:e}",
		"foo_renamed2{pre_arch:a,pre_deps:d,post_deps:f}",
		"foo_renamed2{pre_arch:b,pre_deps:c,post_deps:e}",
		"foo_renamed2{pre_arch:b,pre_deps:c,post_deps:f}",
		"foo_renamed2{pre_arch:b,pre_deps:d,post_deps:e}",
		"foo_renamed2{pre_arch:b,pre_deps:d,post_deps:f}",
	}

	if !reflect.DeepEqual(moduleStrings, want) {
		t.Errorf("want module String() values:\n%q\ngot:\n%q", want, moduleStrings)
	}
}
