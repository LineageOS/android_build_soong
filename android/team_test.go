// Copyright 2024 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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

type fakeModuleForTests struct {
	ModuleBase
}

func fakeModuleFactory() Module {
	module := &fakeModuleForTests{}
	InitAndroidModule(module)
	return module
}

func (*fakeModuleForTests) GenerateAndroidBuildActions(ModuleContext) {}

func TestTeam(t *testing.T) {
	t.Parallel()
	ctx := GroupFixturePreparers(
		PrepareForTestWithTeamBuildComponents,
		FixtureRegisterWithContext(func(ctx RegistrationContext) {
			ctx.RegisterModuleType("fake", fakeModuleFactory)
		}),
	).RunTestWithBp(t, `
		fake {
			name: "main_test",
			team: "someteam",
		}
		team {
			name: "someteam",
			trendy_team_id: "cool_team",
		}

		team {
			name: "team2",
			trendy_team_id: "22222",
		}

		fake {
			name: "tool",
			team: "team2",
		}
	`)

	// Assert the rule from GenerateAndroidBuildActions exists.
	m := ctx.ModuleForTests("main_test", "")
	AssertStringEquals(t, "msg", m.Module().base().Team(), "someteam")
	m = ctx.ModuleForTests("tool", "")
	AssertStringEquals(t, "msg", m.Module().base().Team(), "team2")
}

func TestMissingTeamFails(t *testing.T) {
	t.Parallel()
	GroupFixturePreparers(
		PrepareForTestWithTeamBuildComponents,
		FixtureRegisterWithContext(func(ctx RegistrationContext) {
			ctx.RegisterModuleType("fake", fakeModuleFactory)
		}),
	).
		ExtendWithErrorHandler(FixtureExpectsAtLeastOneErrorMatchingPattern("depends on undefined module \"ring-bearer")).
		RunTestWithBp(t, `
		fake {
			name: "you_cannot_pass",
			team: "ring-bearer",
		}
	`)
}

func TestPackageBadTeamNameFails(t *testing.T) {
	t.Parallel()
	GroupFixturePreparers(
		PrepareForTestWithTeamBuildComponents,
		PrepareForTestWithPackageModule,
		FixtureRegisterWithContext(func(ctx RegistrationContext) {
			ctx.RegisterModuleType("fake", fakeModuleFactory)
		}),
	).
		ExtendWithErrorHandler(FixtureExpectsAtLeastOneErrorMatchingPattern("depends on undefined module \"ring-bearer")).
		RunTestWithBp(t, `
		package {
			default_team: "ring-bearer",
		}
	`)
}
