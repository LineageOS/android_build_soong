// Copyright 2023 Google Inc. All rights reserved.
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

package bp2build

import (
	"testing"

	"android/soong/android"
	"android/soong/java"
)

func runPlatformCompatConfigTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("java_library", java.LibraryFactory)
		ctx.RegisterModuleType("platform_compat_config", java.PlatformCompatConfigFactory)
	}, tc)
}

func TestPlatformCompatConfig(t *testing.T) {
	runPlatformCompatConfigTestCase(t, Bp2buildTestCase{
		Description: "platform_compat_config - conversion test",
		Blueprint: `
		platform_compat_config {
			name: "foo",
			src: ":lib",
		}`,
		StubbedBuildDefinitions: []string{"//a/b:lib"},
		Filesystem: map[string]string{
			"a/b/Android.bp": `
			java_library {
				name: "lib",
				srcs: ["a.java"],
			}`,
		},
		ExpectedBazelTargets: []string{
			MakeBazelTarget("platform_compat_config", "foo", AttrNameToString{
				"src": `"//a/b:lib"`,
			}),
		},
	})
}
