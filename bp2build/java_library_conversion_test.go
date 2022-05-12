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
	"fmt"
	"testing"

	"android/soong/android"
	"android/soong/java"
)

func runJavaLibraryTestCaseWithRegistrationCtxFunc(t *testing.T, tc bp2buildTestCase, registrationCtxFunc func(ctx android.RegistrationContext)) {
	t.Helper()
	(&tc).moduleTypeUnderTest = "java_library"
	(&tc).moduleTypeUnderTestFactory = java.LibraryFactory
	runBp2BuildTestCase(t, registrationCtxFunc, tc)
}

func runJavaLibraryTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	runJavaLibraryTestCaseWithRegistrationCtxFunc(t, tc, func(ctx android.RegistrationContext) {})
}

func TestJavaLibrary(t *testing.T) {
	runJavaLibraryTestCase(t, bp2buildTestCase{
		description: "java_library with srcs, exclude_srcs and libs",
		blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java", "b.java"],
    exclude_srcs: ["b.java"],
    libs: ["java-lib-2"],
    bazel_module: { bp2build_available: true },
}

java_library {
    name: "java-lib-2",
    srcs: ["b.java"],
    bazel_module: { bp2build_available: true },
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("java_library", "java-lib-1", attrNameToString{
				"srcs": `["a.java"]`,
				"deps": `[":java-lib-2"]`,
			}),
			makeBazelTarget("java_library", "java-lib-2", attrNameToString{
				"srcs": `["b.java"]`,
			}),
		},
	})
}

func TestJavaLibraryConvertsStaticLibsToDepsAndExports(t *testing.T) {
	runJavaLibraryTestCase(t, bp2buildTestCase{
		blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java"],
    libs: ["java-lib-2"],
    static_libs: ["java-lib-3"],
    bazel_module: { bp2build_available: true },
}

java_library {
    name: "java-lib-2",
    srcs: ["b.java"],
    bazel_module: { bp2build_available: false },
}

java_library {
    name: "java-lib-3",
    srcs: ["c.java"],
    bazel_module: { bp2build_available: false },
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("java_library", "java-lib-1", attrNameToString{
				"srcs": `["a.java"]`,
				"deps": `[
        ":java-lib-2",
        ":java-lib-3",
    ]`,
				"exports": `[":java-lib-3"]`,
			}),
		},
	})
}

func TestJavaLibraryConvertsStaticLibsToExportsIfNoSrcs(t *testing.T) {
	runJavaLibraryTestCase(t, bp2buildTestCase{
		blueprint: `java_library {
    name: "java-lib-1",
    static_libs: ["java-lib-2"],
    bazel_module: { bp2build_available: true },
}

java_library {
    name: "java-lib-2",
    srcs: ["a.java"],
    bazel_module: { bp2build_available: false },
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("java_library", "java-lib-1", attrNameToString{
				"exports": `[":java-lib-2"]`,
			}),
		},
	})
}

func TestJavaLibraryFailsToConvertLibsWithNoSrcs(t *testing.T) {
	runJavaLibraryTestCase(t, bp2buildTestCase{
		expectedErr: fmt.Errorf("Module has direct dependencies but no sources. Bazel will not allow this."),
		blueprint: `java_library {
    name: "java-lib-1",
    libs: ["java-lib-2"],
    bazel_module: { bp2build_available: true },
}

java_library {
    name: "java-lib-2",
    srcs: ["a.java"],
    bazel_module: { bp2build_available: false },
}`,
		expectedBazelTargets: []string{},
	})
}

func TestJavaLibraryPlugins(t *testing.T) {
	runJavaLibraryTestCaseWithRegistrationCtxFunc(t, bp2buildTestCase{
		blueprint: `java_library {
    name: "java-lib-1",
    plugins: ["java-plugin-1"],
    bazel_module: { bp2build_available: true },
}

java_plugin {
    name: "java-plugin-1",
    srcs: ["a.java"],
    bazel_module: { bp2build_available: false },
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("java_library", "java-lib-1", attrNameToString{
				"plugins": `[":java-plugin-1"]`,
			}),
		},
	}, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("java_plugin", java.PluginFactory)
	})
}

func TestJavaLibraryJavaVersion(t *testing.T) {
	runJavaLibraryTestCase(t, bp2buildTestCase{
		blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java"],
    java_version: "11",
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("java_library", "java-lib-1", attrNameToString{
				"srcs":      `["a.java"]`,
				"javacopts": `["-source 11 -target 11"]`,
			}),
		},
	})
}

func TestJavaLibraryErrorproneJavacflagsEnabledManually(t *testing.T) {
	runJavaLibraryTestCase(t, bp2buildTestCase{
		blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java"],
    javacflags: ["-Xsuper-fast"],
    errorprone: {
        enabled: true,
        javacflags: ["-Xep:SpeedLimit:OFF"],
    },
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("java_library", "java-lib-1", attrNameToString{
				"javacopts": `[
        "-Xsuper-fast",
        "-Xep:SpeedLimit:OFF",
    ]`,
				"srcs": `["a.java"]`,
			}),
		},
	})
}

func TestJavaLibraryErrorproneJavacflagsErrorproneDisabledByDefault(t *testing.T) {
	runJavaLibraryTestCase(t, bp2buildTestCase{
		blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java"],
    javacflags: ["-Xsuper-fast"],
    errorprone: {
        javacflags: ["-Xep:SpeedLimit:OFF"],
    },
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("java_library", "java-lib-1", attrNameToString{
				"javacopts": `["-Xsuper-fast"]`,
				"srcs":      `["a.java"]`,
			}),
		},
	})
}

func TestJavaLibraryErrorproneJavacflagsErrorproneDisabledManually(t *testing.T) {
	runJavaLibraryTestCase(t, bp2buildTestCase{
		blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java"],
    javacflags: ["-Xsuper-fast"],
    errorprone: {
		enabled: false,
        javacflags: ["-Xep:SpeedLimit:OFF"],
    },
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("java_library", "java-lib-1", attrNameToString{
				"javacopts": `["-Xsuper-fast"]`,
				"srcs":      `["a.java"]`,
			}),
		},
	})
}

func TestJavaLibraryLogTags(t *testing.T) {
	runJavaLibraryTestCase(t, bp2buildTestCase{
		description:                "Java library - logtags creates separate dependency",
		moduleTypeUnderTest:        "java_library",
		moduleTypeUnderTestFactory: java.LibraryFactory,
		blueprint: `java_library {
        name: "example_lib",
        srcs: [
			"a.java",
			"b.java",
			"a.logtag",
			"b.logtag",
		],
        bazel_module: { bp2build_available: true },
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("event_log_tags", "example_lib_logtags", attrNameToString{
				"srcs": `[
        "a.logtag",
        "b.logtag",
    ]`,
			}),
			makeBazelTarget("java_library", "example_lib", attrNameToString{
				"srcs": `[
        "a.java",
        "b.java",
        ":example_lib_logtags",
    ]`,
			}),
		}})
}

func TestJavaLibraryResources(t *testing.T) {
	runJavaLibraryTestCase(t, bp2buildTestCase{
		filesystem: map[string]string{
			"res/a.res":      "",
			"res/b.res":      "",
			"res/dir1/b.res": "",
		},
		blueprint: `java_library {
    name: "java-lib-1",
	java_resources: ["res/a.res", "res/b.res"],
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("java_library", "java-lib-1", attrNameToString{
				"resources": `[
        "res/a.res",
        "res/b.res",
    ]`,
			}),
		},
	})
}

func TestJavaLibraryResourceDirs(t *testing.T) {
	runJavaLibraryTestCase(t, bp2buildTestCase{
		filesystem: map[string]string{
			"res/a.res":      "",
			"res/b.res":      "",
			"res/dir1/b.res": "",
		},
		blueprint: `java_library {
    name: "java-lib-1",
	java_resource_dirs: ["res"],
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("java_library", "java-lib-1", attrNameToString{
				"resource_strip_prefix": `"res"`,
				"resources": `[
        "res/a.res",
        "res/b.res",
        "res/dir1/b.res",
    ]`,
			}),
		},
	})
}

func TestJavaLibraryResourcesExcludeDir(t *testing.T) {
	runJavaLibraryTestCase(t, bp2buildTestCase{
		filesystem: map[string]string{
			"res/a.res":         "",
			"res/exclude/b.res": "",
		},
		blueprint: `java_library {
    name: "java-lib-1",
	java_resource_dirs: ["res"],
	exclude_java_resource_dirs: ["res/exclude"],
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("java_library", "java-lib-1", attrNameToString{
				"resource_strip_prefix": `"res"`,
				"resources":             `["res/a.res"]`,
			}),
		},
	})
}

func TestJavaLibraryResourcesExcludeFile(t *testing.T) {
	runJavaLibraryTestCase(t, bp2buildTestCase{
		filesystem: map[string]string{
			"res/a.res":            "",
			"res/dir1/b.res":       "",
			"res/dir1/exclude.res": "",
		},
		blueprint: `java_library {
    name: "java-lib-1",
	java_resource_dirs: ["res"],
	exclude_java_resources: ["res/dir1/exclude.res"],
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("java_library", "java-lib-1", attrNameToString{
				"resource_strip_prefix": `"res"`,
				"resources": `[
        "res/a.res",
        "res/dir1/b.res",
    ]`,
			}),
		},
	})
}

func TestJavaLibraryResourcesFailsWithMultipleDirs(t *testing.T) {
	runJavaLibraryTestCase(t, bp2buildTestCase{
		filesystem: map[string]string{
			"res/a.res":  "",
			"res1/a.res": "",
		},
		blueprint: `java_library {
    name: "java-lib-1",
	java_resource_dirs: ["res", "res1"],
}`,
		expectedErr:          fmt.Errorf("bp2build does not support more than one directory in java_resource_dirs (b/226423379)"),
		expectedBazelTargets: []string{},
	})
}
