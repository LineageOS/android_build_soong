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

func testNinjaDepsSingletonFactory() Singleton {
	return testNinjaDepsSingleton{}
}

type testNinjaDepsSingleton struct{}

func (testNinjaDepsSingleton) GenerateBuildActions(ctx SingletonContext) {
	ctx.Config().addNinjaFileDeps("foo")
}

func TestNinjaDeps(t *testing.T) {
	result := GroupFixturePreparers(
		FixtureRegisterWithContext(func(ctx RegistrationContext) {
			ctx.RegisterSingletonType("test_ninja_deps_singleton", testNinjaDepsSingletonFactory)
			ctx.RegisterSingletonType("ninja_deps_singleton", ninjaDepsSingletonFactory)
		}),
	).RunTest(t)

	// Verify that the ninja file has a dependency on the test_ninja_deps directory.
	if g, w := result.NinjaDeps, "foo"; !InList(w, g) {
		t.Errorf("expected %q in %q", w, g)
	}
}
