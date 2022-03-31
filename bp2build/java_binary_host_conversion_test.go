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
	"android/soong/cc"
	"android/soong/java"
)

func runJavaBinaryHostTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	(&tc).moduleTypeUnderTest = "java_binary_host"
	(&tc).moduleTypeUnderTestFactory = java.BinaryHostFactory
	runBp2BuildTestCase(t, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("cc_library_host_shared", cc.LibraryHostSharedFactory)
		ctx.RegisterModuleType("java_library", java.LibraryFactory)
	}, tc)
}

var fs = map[string]string{
	"test.mf": "Main-Class: com.android.test.MainClass",
	"other/Android.bp": `cc_library_host_shared {
    name: "jni-lib-1",
    stl: "none",
}`,
}

func TestJavaBinaryHost(t *testing.T) {
	runJavaBinaryHostTestCase(t, bp2buildTestCase{
		description: "java_binary_host with srcs, exclude_srcs, jni_libs, javacflags, and manifest.",
		filesystem:  fs,
		blueprint: `java_binary_host {
    name: "java-binary-host-1",
    srcs: ["a.java", "b.java"],
    exclude_srcs: ["b.java"],
    manifest: "test.mf",
    jni_libs: ["jni-lib-1"],
    javacflags: ["-Xdoclint:all/protected"],
    bazel_module: { bp2build_available: true },
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("java_binary", "java-binary-host-1", attrNameToString{
				"srcs":       `["a.java"]`,
				"main_class": `"com.android.test.MainClass"`,
				"deps":       `["//other:jni-lib-1"]`,
				"jvm_flags":  `["-Djava.library.path=$${RUNPATH}other"]`,
				"javacopts":  `["-Xdoclint:all/protected"]`,
				"target_compatible_with": `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestJavaBinaryHostRuntimeDeps(t *testing.T) {
	runJavaBinaryHostTestCase(t, bp2buildTestCase{
		description: "java_binary_host with srcs, exclude_srcs, jni_libs, javacflags, and manifest.",
		filesystem:  fs,
		blueprint: `java_binary_host {
    name: "java-binary-host-1",
    static_libs: ["java-dep-1"],
    manifest: "test.mf",
    bazel_module: { bp2build_available: true },
}

java_library {
    name: "java-dep-1",
    srcs: ["a.java"],
    bazel_module: { bp2build_available: false },
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("java_binary", "java-binary-host-1", attrNameToString{
				"main_class":   `"com.android.test.MainClass"`,
				"runtime_deps": `[":java-dep-1"]`,
				"target_compatible_with": `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}
