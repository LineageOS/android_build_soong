// Copyright 2024 Google Inc. All rights reserved.
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

package cc

import (
	"android/soong/android"
	"android/soong/android/team_proto"
	"log"
	"strings"
	"testing"

	"github.com/google/blueprint"
	"google.golang.org/protobuf/proto"
)

func TestTestOnlyProvider(t *testing.T) {
	t.Parallel()
	ctx := android.GroupFixturePreparers(
		prepareForCcTest,
		android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
			ctx.RegisterModuleType("cc_test_host", TestHostFactory)
		}),
	).RunTestWithBp(t, `
                // These should be test-only
                cc_fuzz { name: "cc-fuzz" }
                cc_test { name: "cc-test", gtest:false }
                cc_benchmark { name: "cc-benchmark" }
                cc_library { name: "cc-library-forced",
                             test_only: true }
                cc_test_library {name: "cc-test-library", gtest: false}
                cc_test_host {name: "cc-test-host", gtest: false}

                // These should not be.
                cc_genrule { name: "cc_genrule", cmd: "echo foo", out: ["out"] }
                cc_library { name: "cc_library" }
                cc_library { name: "cc_library_false", test_only: false }
                cc_library_static { name: "cc_static" }
                cc_library_shared { name: "cc_library_shared" }

                cc_object { name: "cc-object" }
	`)

	// Visit all modules and ensure only the ones that should
	// marked as test-only are marked as test-only.

	actualTestOnly := []string{}
	ctx.VisitAllModules(func(m blueprint.Module) {
		if provider, ok := android.OtherModuleProvider(ctx.TestContext.OtherModuleProviderAdaptor(), m, android.TestOnlyProviderKey); ok {
			if provider.TestOnly {
				actualTestOnly = append(actualTestOnly, m.Name())
			}
		}
	})
	expectedTestOnlyModules := []string{
		"cc-test",
		"cc-library-forced",
		"cc-fuzz",
		"cc-benchmark",
		"cc-test-library",
		"cc-test-host",
	}

	notEqual, left, right := android.ListSetDifference(expectedTestOnlyModules, actualTestOnly)
	if notEqual {
		t.Errorf("test-only: Expected but not found: %v, Found but not expected: %v", left, right)
	}
}

func TestTestOnlyInTeamsProto(t *testing.T) {
	t.Parallel()
	ctx := android.GroupFixturePreparers(
		android.PrepareForTestWithTeamBuildComponents,
		prepareForCcTest,
		android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
			ctx.RegisterParallelSingletonType("all_teams", android.AllTeamsFactory)
			ctx.RegisterModuleType("cc_test_host", TestHostFactory)

		}),
	).RunTestWithBp(t, `
                package { default_team: "someteam"}

                // These should be test-only
                cc_fuzz { name: "cc-fuzz" }
                cc_test { name: "cc-test", gtest:false }
                cc_benchmark { name: "cc-benchmark" }
                cc_library { name: "cc-library-forced",
                             test_only: true }
                cc_test_library {name: "cc-test-library", gtest: false}
                cc_test_host {name: "cc-test-host", gtest: false}

                // These should not be.
                cc_genrule { name: "cc_genrule", cmd: "echo foo", out: ["out"] }
                cc_library { name: "cc_library" }
                cc_library_static { name: "cc_static" }
                cc_library_shared { name: "cc_library_shared" }

                cc_object { name: "cc-object" }
		team {
			name: "someteam",
			trendy_team_id: "cool_team",
		}
	`)

	var teams *team_proto.AllTeams
	teams = getTeamProtoOutput(t, ctx)

	// map of module name -> trendy team name.
	actualTrueModules := []string{}
	for _, teamProto := range teams.Teams {
		if Bool(teamProto.TestOnly) {
			actualTrueModules = append(actualTrueModules, teamProto.GetTargetName())
		}
	}
	expectedTestOnlyModules := []string{
		"cc-test",
		"cc-library-forced",
		"cc-fuzz",
		"cc-benchmark",
		"cc-test-library",
		"cc-test-host",
	}

	notEqual, left, right := android.ListSetDifference(expectedTestOnlyModules, actualTrueModules)
	if notEqual {
		t.Errorf("test-only: Expected but not found: %v, Found but not expected: %v", left, right)
	}
}

// Don't allow setting test-only on things that are always tests or never tests.
func TestInvalidTestOnlyTargets(t *testing.T) {
	testCases := []string{
		` cc_test {  name: "cc-test", test_only: true, gtest: false, srcs: ["foo.cc"],  } `,
		` cc_binary {  name: "cc-binary", test_only: true, srcs: ["foo.cc"],  } `,
		` cc_test_library {name: "cc-test-library", test_only: true, gtest: false} `,
		` cc_test_host {name: "cc-test-host", test_only: true, gtest: false} `,
		` cc_defaults {name: "cc-defaults", test_only: true} `,
	}

	for i, bp := range testCases {
		ctx := android.GroupFixturePreparers(
			prepareForCcTest,
			android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
				ctx.RegisterModuleType("cc_test_host", TestHostFactory)
			})).
			ExtendWithErrorHandler(android.FixtureIgnoreErrors).
			RunTestWithBp(t, bp)
		if len(ctx.Errs) == 0 {
			t.Errorf("Expected err setting test_only in testcase #%d", i)
		}
		if len(ctx.Errs) > 1 {
			t.Errorf("Too many errs: [%s] %v", bp, ctx.Errs)
		}

		if len(ctx.Errs) == 1 {
			if !strings.Contains(ctx.Errs[0].Error(), "unrecognized property \"test_only\"") {
				t.Errorf("ERR: %s bad bp: %s", ctx.Errs[0], bp)
			}
		}
	}
}

func getTeamProtoOutput(t *testing.T, ctx *android.TestResult) *team_proto.AllTeams {
	teams := new(team_proto.AllTeams)
	config := ctx.SingletonForTests("all_teams")
	allOutputs := config.AllOutputs()

	protoPath := allOutputs[0]

	out := config.MaybeOutput(protoPath)
	outProto := []byte(android.ContentFromFileRuleForTests(t, ctx.TestContext, out))
	if err := proto.Unmarshal(outProto, teams); err != nil {
		log.Fatalln("Failed to parse teams proto:", err)
	}
	return teams
}
