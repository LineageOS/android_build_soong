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

var prepareForSingletonModuleTest = GroupFixturePreparers(
	// Enable Kati output to test SingletonModules with MakeVars.
	PrepareForTestWithAndroidMk,
	FixtureRegisterWithContext(func(ctx RegistrationContext) {
		ctx.RegisterSingletonModuleType("test_singleton_module", testSingletonModuleFactory)
	}),
	PrepareForTestWithMakevars,
)

func TestSingletonModule(t *testing.T) {
	bp := `
		test_singleton_module {
			name: "test_singleton_module",
		}
	`
	result := GroupFixturePreparers(
		prepareForSingletonModuleTest,
		FixtureWithRootAndroidBp(bp),
	).RunTest(t)

	ops := result.ModuleForTests("test_singleton_module", "").Module().(*testSingletonModule).ops
	wantOps := []string{"GenerateAndroidBuildActions", "GenerateSingletonBuildActions", "MakeVars"}
	AssertDeepEquals(t, "operations", wantOps, ops)
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

	prepareForSingletonModuleTest.
		ExtendWithErrorHandler(FixtureExpectsAllErrorsToMatchAPattern([]string{
			`\QDuplicate SingletonModule "test_singleton_module", previously used in\E`,
		})).RunTestWithBp(t, bp)
}

func TestUnusedSingletonModule(t *testing.T) {
	result := GroupFixturePreparers(
		prepareForSingletonModuleTest,
	).RunTest(t)

	singleton := result.SingletonForTests("test_singleton_module").Singleton()
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
	if testing.Short() {
		t.Skip("test fails with data race enabled")
	}
	bp := `
		test_singleton_module {
			name: "test_singleton_module",
		}
	`

	GroupFixturePreparers(
		prepareForSingletonModuleTest,
		FixtureRegisterWithContext(func(ctx RegistrationContext) {
			ctx.PreDepsMutators(func(ctx RegisterMutatorsContext) {
				ctx.BottomUp("test_singleton_module_mutator", testVariantSingletonModuleMutator)
			})
		}),
	).
		ExtendWithErrorHandler(FixtureExpectsAllErrorsToMatchAPattern([]string{
			`\QGenerateAndroidBuildActions already called for variant\E`,
		})).
		RunTestWithBp(t, bp)
}
