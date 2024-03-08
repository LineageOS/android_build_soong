// Copyright 2018 Google Inc. All rights reserved.
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

package java

import (
	"testing"

	"android/soong/android"
)

func JavaGenLibTestFactory() android.Module {
	callbacks := &JavaGenLibTestCallbacks{}
	return GeneratedJavaLibraryModuleFactory("test_java_gen_lib", callbacks, &callbacks.properties)
}

type JavaGenLibTestProperties struct {
	Foo string
}

type JavaGenLibTestCallbacks struct {
	properties JavaGenLibTestProperties
}

func (callbacks *JavaGenLibTestCallbacks) DepsMutator(module *GeneratedJavaLibraryModule, ctx android.BottomUpMutatorContext) {
}

func (callbacks *JavaGenLibTestCallbacks) GenerateSourceJarBuildActions(module *GeneratedJavaLibraryModule, ctx android.ModuleContext) android.Path {
	return android.PathForOutput(ctx, "blah.srcjar")
}

func testGenLib(t *testing.T, errorHandler android.FixtureErrorHandler, bp string) *android.TestResult {
	return android.GroupFixturePreparers(
		PrepareForIntegrationTestWithJava,
		android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
			ctx.RegisterModuleType("test_java_gen_lib", JavaGenLibTestFactory)
		}),
	).
		ExtendWithErrorHandler(errorHandler).
		RunTestWithBp(t, bp)
}

func TestGenLib(t *testing.T) {
	bp := `
				test_java_gen_lib {
					name: "javagenlibtest",
                    foo: "bar",  // Note: This won't parse if the property didn't get added
				}
			`
	result := testGenLib(t, android.FixtureExpectsNoErrors, bp)

	javagenlibtest := result.ModuleForTests("javagenlibtest", "android_common").Module().(*GeneratedJavaLibraryModule)
	android.AssertPathsEndWith(t, "Generated_srcjars", []string{"/blah.srcjar"}, javagenlibtest.Library.properties.Generated_srcjars)
}
