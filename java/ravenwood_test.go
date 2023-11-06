// Copyright 2022 Google Inc. All rights reserved.
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
	"runtime"
	"testing"

	"android/soong/android"
)

var prepareRavenwoodRuntime = android.GroupFixturePreparers(
	android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
		RegisterRavenwoodBuildComponents(ctx)
	}),
	android.FixtureAddTextFile("ravenwood/Android.bp", `
		java_library_static {
			name: "framework-minus-apex.ravenwood",
			srcs: ["Framework.java"],
		}
		java_library_static {
			name: "framework-services.ravenwood",
			srcs: ["Services.java"],
		}
		java_library_static {
			name: "framework-rules.ravenwood",
			srcs: ["Rules.java"],
		}
		android_ravenwood_libgroup {
			name: "ravenwood-runtime",
			libs: [
				"framework-minus-apex.ravenwood",
				"framework-services.ravenwood",
			],
		}
		android_ravenwood_libgroup {
			name: "ravenwood-utils",
			libs: [
				"framework-rules.ravenwood",
			],
		}
	`),
)

var installPathPrefix = "out/soong/host/linux-x86/testcases"

func TestRavenwoodRuntime(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires linux")
	}

	ctx := android.GroupFixturePreparers(
		PrepareForIntegrationTestWithJava,
		prepareRavenwoodRuntime,
	).RunTest(t)

	// Verify that our runtime depends on underlying libs
	CheckModuleHasDependency(t, ctx.TestContext, "ravenwood-runtime", "android_common", "framework-minus-apex.ravenwood")
	CheckModuleHasDependency(t, ctx.TestContext, "ravenwood-runtime", "android_common", "framework-services.ravenwood")
	CheckModuleHasDependency(t, ctx.TestContext, "ravenwood-utils", "android_common", "framework-rules.ravenwood")

	// Verify that we've emitted artifacts in expected location
	runtime := ctx.ModuleForTests("ravenwood-runtime", "android_common")
	runtime.Output(installPathPrefix + "/ravenwood-runtime/framework-minus-apex.ravenwood.jar")
	runtime.Output(installPathPrefix + "/ravenwood-runtime/framework-services.ravenwood.jar")
	utils := ctx.ModuleForTests("ravenwood-utils", "android_common")
	utils.Output(installPathPrefix + "/ravenwood-utils/framework-rules.ravenwood.jar")
}

func TestRavenwoodTest(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires linux")
	}

	ctx := android.GroupFixturePreparers(
		PrepareForIntegrationTestWithJava,
		prepareRavenwoodRuntime,
	).RunTestWithBp(t, `
		android_ravenwood_test {
			name: "ravenwood-test",
			srcs: ["Test.java"],
			sdk_version: "test_current",
		}
	`)

	// Verify that our test depends on underlying libs
	CheckModuleHasDependency(t, ctx.TestContext, "ravenwood-test", "android_common", "ravenwood-buildtime")
	CheckModuleHasDependency(t, ctx.TestContext, "ravenwood-test", "android_common", "ravenwood-utils")

	module := ctx.ModuleForTests("ravenwood-test", "android_common")
	classpath := module.Rule("javac").Args["classpath"]

	// Verify that we're linking against test_current
	android.AssertStringDoesContain(t, "classpath", classpath, "android_test_stubs_current.jar")
	// Verify that we're linking against utils
	android.AssertStringDoesContain(t, "classpath", classpath, "framework-rules.ravenwood.jar")
	// Verify that we're *NOT* linking against runtime
	android.AssertStringDoesNotContain(t, "classpath", classpath, "framework-minus-apex.ravenwood.jar")
	android.AssertStringDoesNotContain(t, "classpath", classpath, "framework-services.ravenwood.jar")

	// Verify that we've emitted test artifacts in expected location
	outputJar := module.Output(installPathPrefix + "/ravenwood-test/ravenwood-test.jar")
	module.Output(installPathPrefix + "/ravenwood-test/ravenwood-test.config")

	// Verify that we're going to install underlying libs
	orderOnly := outputJar.OrderOnly.Strings()
	android.AssertStringListContains(t, "orderOnly", orderOnly, installPathPrefix+"/ravenwood-runtime/framework-minus-apex.ravenwood.jar")
	android.AssertStringListContains(t, "orderOnly", orderOnly, installPathPrefix+"/ravenwood-runtime/framework-services.ravenwood.jar")
	android.AssertStringListContains(t, "orderOnly", orderOnly, installPathPrefix+"/ravenwood-utils/framework-rules.ravenwood.jar")
}
