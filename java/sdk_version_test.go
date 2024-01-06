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

package java

import (
	"testing"

	"android/soong/android"
)

func stringPtr(v string) *string {
	return &v
}

func TestSystemSdkFromVendor(t *testing.T) {
	fixtures := android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.Platform_sdk_version = intPtr(34)
			variables.Platform_sdk_codename = stringPtr("VanillaIceCream")
			variables.Platform_version_active_codenames = []string{"VanillaIceCream"}
			variables.Platform_systemsdk_versions = []string{"33", "34", "VanillaIceCream"}
			variables.DeviceSystemSdkVersions = []string{"VanillaIceCream"}
		}),
		FixtureWithPrebuiltApis(map[string][]string{
			"33": {},
			"34": {},
			"35": {},
		}),
	)

	fixtures.ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern("incompatible sdk version")).
		RunTestWithBp(t, `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			vendor: true,
			sdk_version: "system_35",
		}`)

	result := fixtures.RunTestWithBp(t, `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			vendor: true,
			sdk_version: "system_current",
		}`)
	fooModule := result.ModuleForTests("foo", "android_common")
	fooClasspath := fooModule.Rule("javac").Args["classpath"]

	android.AssertStringDoesContain(t, "foo classpath", fooClasspath, "prebuilts/sdk/34/system/android.jar")
	android.AssertStringDoesNotContain(t, "foo classpath", fooClasspath, "prebuilts/sdk/35/system/android.jar")
	android.AssertStringDoesNotContain(t, "foo classpath", fooClasspath, "prebuilts/sdk/current/system/android.jar")
}
