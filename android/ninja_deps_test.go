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

import (
	"testing"
)

func init() {
	// This variable uses ExistentPathForSource on a PackageVarContext, which is a PathContext
	// that is not a PathGlobContext.  That requires the deps to be stored in the Config.
	pctx.VariableFunc("test_ninja_deps_variable", func(ctx PackageVarContext) string {
		// Using ExistentPathForSource to look for a file that does not exist in a directory that
		// does exist (test_ninja_deps) from a PackageVarContext adds a dependency from build.ninja
		// to the directory.
		if ExistentPathForSource(ctx, "test_ninja_deps/does_not_exist").Valid() {
			return "true"
		} else {
			return "false"
		}
	})
}

func testNinjaDepsSingletonFactory() Singleton {
	return testNinjaDepsSingleton{}
}

type testNinjaDepsSingleton struct{}

func (testNinjaDepsSingleton) GenerateBuildActions(ctx SingletonContext) {
	// Reference the test_ninja_deps_variable in a build statement so Blueprint is forced to
	// evaluate it.
	ctx.Build(pctx, BuildParams{
		Rule:   Cp,
		Input:  PathForTesting("foo"),
		Output: PathForOutput(ctx, "test_ninja_deps_out"),
		Args: map[string]string{
			"cpFlags": "${test_ninja_deps_variable}",
		},
	})
}

func TestNinjaDeps(t *testing.T) {
	fs := MockFS{
		"test_ninja_deps/exists": nil,
	}

	result := emptyTestFixtureFactory.RunTest(t,
		FixtureRegisterWithContext(func(ctx RegistrationContext) {
			ctx.RegisterSingletonType("test_ninja_deps_singleton", testNinjaDepsSingletonFactory)
			ctx.RegisterSingletonType("ninja_deps_singleton", ninjaDepsSingletonFactory)
		}),
		fs.AddToFixture(),
	)

	// Verify that the ninja file has a dependency on the test_ninja_deps directory.
	if g, w := result.NinjaDeps, "test_ninja_deps"; !InList(w, g) {
		t.Errorf("expected %q in %q", w, g)
	}
}
