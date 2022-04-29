// Copyright 2021 Google Inc. All rights reserved.
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

func runJavaLibraryHostTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	(&tc).moduleTypeUnderTest = "java_library_host"
	(&tc).moduleTypeUnderTestFactory = java.LibraryHostFactory
	runBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, tc)
}

func TestJavaLibraryHost(t *testing.T) {
	runJavaLibraryHostTestCase(t, bp2buildTestCase{
		description: "java_library_host with srcs, exclude_srcs and libs",
		blueprint: `java_library_host {
    name: "java-lib-host-1",
    srcs: ["a.java", "b.java"],
    exclude_srcs: ["b.java"],
    libs: ["java-lib-host-2"],
    bazel_module: { bp2build_available: true },
}

java_library_host {
    name: "java-lib-host-2",
    srcs: ["c.java"],
    bazel_module: { bp2build_available: true },
    java_version: "9",
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("java_library", "java-lib-host-1", attrNameToString{
				"srcs": `["a.java"]`,
				"deps": `[":java-lib-host-2"]`,
				"target_compatible_with": `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
			}),
			makeBazelTarget("java_library", "java-lib-host-2", attrNameToString{
				"javacopts": `["-source 1.9 -target 1.9"]`,
				"srcs":      `["c.java"]`,
				"target_compatible_with": `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}
