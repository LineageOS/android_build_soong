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

func runJavaPluginTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	(&tc).moduleTypeUnderTest = "java_plugin"
	(&tc).moduleTypeUnderTestFactory = java.PluginFactory
	runBp2BuildTestCase(t, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("java_library", java.LibraryFactory)
	}, tc)
}

func TestJavaPlugin(t *testing.T) {
	runJavaPluginTestCase(t, bp2buildTestCase{
		description: "java_plugin with srcs, libs, static_libs",
		blueprint: `java_plugin {
    name: "java-plug-1",
    srcs: ["a.java", "b.java"],
    libs: ["java-lib-1"],
    static_libs: ["java-lib-2"],
    bazel_module: { bp2build_available: true },
    java_version: "7",
}

java_library {
    name: "java-lib-1",
    srcs: ["b.java"],
    bazel_module: { bp2build_available: false },
}

java_library {
    name: "java-lib-2",
    srcs: ["c.java"],
    bazel_module: { bp2build_available: false },
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("java_plugin", "java-plug-1", attrNameToString{
				"target_compatible_with": `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
				"deps": `[
        ":java-lib-1",
        ":java-lib-2",
    ]`,
				"srcs": `[
        "a.java",
        "b.java",
    ]`,
				"javacopts": `["-source 1.7 -target 1.7"]`,
			}),
		},
	})
}

func TestJavaPluginNoSrcs(t *testing.T) {
	runJavaPluginTestCase(t, bp2buildTestCase{
		description: "java_plugin without srcs converts (static) libs to deps",
		blueprint: `java_plugin {
    name: "java-plug-1",
    libs: ["java-lib-1"],
    static_libs: ["java-lib-2"],
    bazel_module: { bp2build_available: true },
}

java_library {
    name: "java-lib-1",
    srcs: ["b.java"],
    bazel_module: { bp2build_available: false },
}

java_library {
    name: "java-lib-2",
    srcs: ["c.java"],
    bazel_module: { bp2build_available: false },
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("java_plugin", "java-plug-1", attrNameToString{
				"target_compatible_with": `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
				"deps": `[
        ":java-lib-1",
        ":java-lib-2",
    ]`,
			}),
		},
	})
}
