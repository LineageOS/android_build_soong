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
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/google/blueprint/proptools"
)

type mutatorTestModule struct {
	ModuleBase
	props struct {
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
	ctx.AddDependency(ctx.Module(), nil, "regular_missing_dep")
}

func addMissingDependenciesMutator(ctx TopDownMutatorContext) {
	ctx.AddMissingDependencies([]string{"added_missing_dep"})
}

func TestMutatorAddMissingDependencies(t *testing.T) {
	buildDir, err := ioutil.TempDir("", "soong_mutator_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(buildDir)

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
