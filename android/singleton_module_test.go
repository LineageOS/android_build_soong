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

package android

import (
	"reflect"
	"strings"
	"testing"
)

type testSingletonModule struct {
	SingletonModuleBase
	ops []string
}

func (tsm *testSingletonModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	tsm.ops = append(tsm.ops, "GenerateAndroidBuildActions")
}

func (tsm *testSingletonModule) GenerateSingletonBuildActions(ctx SingletonContext) {
	tsm.ops = append(tsm.ops, "GenerateSingletonBuildActions")
}

func (tsm *testSingletonModule) MakeVars(ctx MakeVarsContext) {
	tsm.ops = append(tsm.ops, "MakeVars")
}

func testSingletonModuleFactory() SingletonModule {
	tsm := &testSingletonModule{}
	InitAndroidSingletonModule(tsm)
	return tsm
}

func runSingletonModuleTest(bp string) (*TestContext, []error) {
	config := TestConfig(buildDir, nil, bp, nil)
	// Enable Kati output to test SingletonModules with MakeVars.
	config.katiEnabled = true
	ctx := NewTestContext(config)
	ctx.RegisterSingletonModuleType("test_singleton_module", testSingletonModuleFactory)
	ctx.RegisterSingletonType("makevars", makeVarsSingletonFunc)
	ctx.Register()

	_, errs := ctx.ParseBlueprintsFiles("Android.bp")
	if len(errs) > 0 {
		return ctx, errs
	}

	_, errs = ctx.PrepareBuildActions(config)
	return ctx, errs
}

func TestSingletonModule(t *testing.T) {
	bp := `
		test_singleton_module {
			name: "test_singleton_module",
		}
	`
	ctx, errs := runSingletonModuleTest(bp)
	if len(errs) > 0 {
		t.Fatal(errs)
	}

	ops := ctx.ModuleForTests("test_singleton_module", "").Module().(*testSingletonModule).ops
	wantOps := []string{"GenerateAndroidBuildActions", "GenerateSingletonBuildActions", "MakeVars"}
	if !reflect.DeepEqual(ops, wantOps) {
		t.Errorf("Expected operations %q, got %q", wantOps, ops)
	}
}

func TestDuplicateSingletonModule(t *testing.T) {
	bp := `
		test_singleton_module {
			name: "test_singleton_module",
		}

		test_singleton_module {
			name: "test_singleton_module2",
		}
	`
	_, errs := runSingletonModuleTest(bp)
	if len(errs) == 0 {
		t.Fatal("expected duplicate SingletonModule error")
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), `Duplicate SingletonModule "test_singleton_module", previously used in`) {
		t.Fatalf("expected duplicate SingletonModule error, got %q", errs)
	}
}

func TestUnusedSingletonModule(t *testing.T) {
	bp := ``
	ctx, errs := runSingletonModuleTest(bp)
	if len(errs) > 0 {
		t.Fatal(errs)
	}

	singleton := ctx.SingletonForTests("test_singleton_module").Singleton()
	sm := singleton.(*singletonModuleSingletonAdaptor).sm
	ops := sm.(*testSingletonModule).ops
	if ops != nil {
		t.Errorf("Expected no operations, got %q", ops)
	}
}

func testVariantSingletonModuleMutator(ctx BottomUpMutatorContext) {
	if _, ok := ctx.Module().(*testSingletonModule); ok {
		ctx.CreateVariations("a", "b")
	}
}

func TestVariantSingletonModule(t *testing.T) {
	bp := `
		test_singleton_module {
			name: "test_singleton_module",
		}
	`

	config := TestConfig(buildDir, nil, bp, nil)
	ctx := NewTestContext(config)
	ctx.PreDepsMutators(func(ctx RegisterMutatorsContext) {
		ctx.BottomUp("test_singleton_module_mutator", testVariantSingletonModuleMutator)
	})
	ctx.RegisterSingletonModuleType("test_singleton_module", testSingletonModuleFactory)
	ctx.Register()

	_, errs := ctx.ParseBlueprintsFiles("Android.bp")

	if len(errs) == 0 {
		_, errs = ctx.PrepareBuildActions(config)
	}

	if len(errs) == 0 {
		t.Fatal("expected duplicate SingletonModule error")
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), `GenerateAndroidBuildActions already called for variant`) {
		t.Fatalf("expected duplicate SingletonModule error, got %q", errs)
	}
}
