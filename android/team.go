// Copyright 2020 Google Inc. All rights reserved.
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

func init() {
	RegisterTeamBuildComponents(InitRegistrationContext)
}

func RegisterTeamBuildComponents(ctx RegistrationContext) {
	ctx.RegisterModuleType("team", TeamFactory)
}

var PrepareForTestWithTeamBuildComponents = GroupFixturePreparers(
	FixtureRegisterWithContext(RegisterTeamBuildComponents),
)

type teamProperties struct {
	Trendy_team_id *string `json:"trendy_team_id"`
}

type teamModule struct {
	ModuleBase
	DefaultableModuleBase

	properties teamProperties
}

// Real work is done for the module that depends on us.
// If needed, the team can serialize the config to json/proto file as well.
func (t *teamModule) GenerateAndroidBuildActions(ctx ModuleContext) {}

func (t *teamModule) TrendyTeamId(ctx ModuleContext) string {
	return *t.properties.Trendy_team_id
}

func TeamFactory() Module {
	module := &teamModule{}

	base := module.base()
	module.AddProperties(&base.nameProperties, &module.properties)

	InitAndroidModule(module)
	InitDefaultableModule(module)

	return module
}
