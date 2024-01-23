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
	"android/soong/android/team_proto"
	"log"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestAllTeams(t *testing.T) {
	t.Parallel()
	ctx := GroupFixturePreparers(
		PrepareForTestWithTeamBuildComponents,
		FixtureRegisterWithContext(func(ctx RegistrationContext) {
			ctx.RegisterModuleType("fake", fakeModuleFactory)
			ctx.RegisterParallelSingletonType("all_teams", AllTeamsFactory)
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

		fake {
			name: "noteam",
		}
	`)

	var teams *team_proto.AllTeams
	teams = getTeamProtoOutput(t, ctx)

	// map of module name -> trendy team name.
	actualTeams := make(map[string]*string)
	for _, teamProto := range teams.Teams {
		actualTeams[teamProto.GetTargetName()] = teamProto.TrendyTeamId
	}
	expectedTeams := map[string]*string{
		"main_test": proto.String("cool_team"),
		"tool":      proto.String("22222"),
		"noteam":    nil,
	}

	AssertDeepEquals(t, "compare maps", expectedTeams, actualTeams)
}

func getTeamProtoOutput(t *testing.T, ctx *TestResult) *team_proto.AllTeams {
	teams := new(team_proto.AllTeams)
	config := ctx.SingletonForTests("all_teams")
	allOutputs := config.AllOutputs()

	protoPath := allOutputs[0]

	out := config.MaybeOutput(protoPath)
	outProto := []byte(ContentFromFileRuleForTests(t, ctx.TestContext, out))
	if err := proto.Unmarshal(outProto, teams); err != nil {
		log.Fatalln("Failed to parse teams proto:", err)
	}
	return teams
}

// Android.bp
//
//	team: team_top
//
// # dir1 has no modules with teams,
// # but has a dir with no Android.bp
// dir1/Android.bp
//
//	module_dir1
//
// # dirs without and Android.bp should be fine.
// dir1/dir2/dir3/Android.bp
//
//	package {}
//	module_dir123
//
// teams_dir/Android.bp
//
//	module_with_team1: team1
//	team1: 111
//
// # team comes from upper package default
// teams_dir/deeper/Android.bp
//
//	module2_with_team1: team1
//
// package_defaults/Android.bp
// package_defaults/pd2/Android.bp
//
//	package{ default_team: team_top}
//	module_pd2   ## should get team_top
//
// package_defaults/pd2/pd3/Android.bp
//
//	module_pd3  ## should get team_top
func TestPackageLookup(t *testing.T) {
	t.Parallel()
	rootBp := `
		team {
			name: "team_top",
			trendy_team_id: "trendy://team_top",
		} `

	dir1Bp := `
		fake {
			name: "module_dir1",
		} `
	dir3Bp := `
                package {}
		fake {
			name: "module_dir123",
		} `
	teamsDirBp := `
		fake {
			name: "module_with_team1",
                        team: "team1"

		}
		team {
			name: "team1",
			trendy_team_id: "111",
		} `
	teamsDirDeeper := `
		fake {
			name: "module2_with_team1",
                        team: "team1"
		} `
	// create an empty one.
	packageDefaultsBp := ""
	packageDefaultspd2 := `
                package { default_team: "team_top"}
		fake {
			name: "modulepd2",
		} `

	packageDefaultspd3 := `
		fake {
			name: "modulepd3",
		}
		fake {
			name: "modulepd3b",
			team: "team1"
		} `

	ctx := GroupFixturePreparers(
		PrepareForTestWithTeamBuildComponents,
		PrepareForTestWithPackageModule,
		FixtureRegisterWithContext(func(ctx RegistrationContext) {
			ctx.RegisterModuleType("fake", fakeModuleFactory)
			ctx.RegisterParallelSingletonType("all_teams", AllTeamsFactory)
		}),
		FixtureAddTextFile("Android.bp", rootBp),
		FixtureAddTextFile("dir1/Android.bp", dir1Bp),
		FixtureAddTextFile("dir1/dir2/dir3/Android.bp", dir3Bp),
		FixtureAddTextFile("teams_dir/Android.bp", teamsDirBp),
		FixtureAddTextFile("teams_dir/deeper/Android.bp", teamsDirDeeper),
		FixtureAddTextFile("package_defaults/Android.bp", packageDefaultsBp),
		FixtureAddTextFile("package_defaults/pd2/Android.bp", packageDefaultspd2),
		FixtureAddTextFile("package_defaults/pd2/pd3/Android.bp", packageDefaultspd3),
	).RunTest(t)

	var teams *team_proto.AllTeams
	teams = getTeamProtoOutput(t, ctx)

	// map of module name -> trendy team name.
	actualTeams := make(map[string]*string)
	for _, teamProto := range teams.Teams {
		actualTeams[teamProto.GetTargetName()] = teamProto.TrendyTeamId
	}
	expectedTeams := map[string]*string{
		"module_with_team1":  proto.String("111"),
		"module2_with_team1": proto.String("111"),
		"modulepd2":          proto.String("trendy://team_top"),
		"modulepd3":          proto.String("trendy://team_top"),
		"modulepd3b":         proto.String("111"),
		"module_dir1":        nil,
		"module_dir123":      nil,
	}
	AssertDeepEquals(t, "compare maps", expectedTeams, actualTeams)
}
