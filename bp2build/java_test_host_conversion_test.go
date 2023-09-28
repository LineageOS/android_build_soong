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

func runJavaTestHostTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	(&tc).ModuleTypeUnderTest = "java_test_host"
	(&tc).ModuleTypeUnderTestFactory = java.TestHostFactory
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("java_library", java.LibraryFactory)
	}, tc)
}

func TestJavaTestHostGeneral(t *testing.T) {
	runJavaTestHostTestCase(t, Bp2buildTestCase{
		Description:             "java_test_host general",
		Filesystem:              map[string]string{},
		StubbedBuildDefinitions: []string{"lib_a", "lib_b"},
		Blueprint: `
java_test_host {
    name: "java_test_host-1",
    srcs: ["a.java", "b.java"],
    libs: ["lib_a"],
    static_libs: ["static_libs_a"],
    exclude_srcs: ["b.java"],
    javacflags: ["-Xdoclint:all/protected"],
    java_version: "8",
}

java_library {
    name: "lib_a",
}

java_library {
    name: "static_libs_a",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java_test_host-1_lib", AttrNameToString{
				"deps": `[
        ":lib_a-neverlink",
        ":static_libs_a",
    ]`,
				"java_version": `"8"`,
				"javacopts":    `["-Xdoclint:all/protected"]`,
				"srcs":         `["a.java"]`,
				"target_compatible_with": `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
			}),
			MakeBazelTarget("java_test", "java_test_host-1", AttrNameToString{
				"runtime_deps": `[":java_test_host-1_lib"]`,
				"deps": `[
        ":lib_a-neverlink",
        ":static_libs_a",
    ]`,
				"srcs": `["a.java"]`,
				"target_compatible_with": `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestJavaTestHostNoSrcs(t *testing.T) {
	runJavaTestHostTestCase(t, Bp2buildTestCase{
		Description: "java_test_host without srcs",
		Filesystem:  map[string]string{},
		Blueprint: `
java_test_host {
    name: "java_test_host-1",
    libs: ["lib_a"],
    static_libs: ["static_libs_a"],
}

java_library {
    name: "lib_a",
}

java_library {
    name: "static_libs_a",
}
`,
		StubbedBuildDefinitions: []string{"lib_a", "static_libs_a"},
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_test", "java_test_host-1", AttrNameToString{
				"runtime_deps": `[
        ":lib_a-neverlink",
        ":static_libs_a",
    ]`,
				"target_compatible_with": `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestJavaTestHostKotlinSrcs(t *testing.T) {
	runJavaTestHostTestCase(t, Bp2buildTestCase{
		Description: "java_test_host with .kt in srcs",
		Filesystem:  map[string]string{},
		Blueprint: `
java_test_host {
    name: "java_test_host-1",
    srcs: ["a.java", "b.kt"],
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_test", "java_test_host-1", AttrNameToString{
				"srcs": `[
        "a.java",
        "b.kt",
    ]`,
				"runtime_deps": `[":java_test_host-1_lib"]`,
				"target_compatible_with": `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
			}),
			MakeBazelTarget("kt_jvm_library", "java_test_host-1_lib", AttrNameToString{
				"srcs": `[
        "a.java",
        "b.kt",
    ]`,
				"target_compatible_with": `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}
