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

func runJavaLibraryTestCaseWithRegistrationCtxFunc(t *testing.T, tc Bp2buildTestCase, registrationCtxFunc func(ctx android.RegistrationContext)) {
	t.Helper()
	(&tc).ModuleTypeUnderTest = "java_library"
	(&tc).ModuleTypeUnderTestFactory = java.LibraryFactory
	RunBp2BuildTestCase(t, registrationCtxFunc, tc)
}

func runJavaLibraryTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	runJavaLibraryTestCaseWithRegistrationCtxFunc(t, tc, func(ctx android.RegistrationContext) {})
}

func TestJavaLibrary(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Description: "java_library with srcs, exclude_srcs and libs",
		Blueprint: `java_library {
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
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"srcs": `["a.java"]`,
				"deps": `[":java-lib-2"]`,
			}),
			MakeBazelTarget("java_library", "java-lib-2", AttrNameToString{
				"srcs": `["b.java"]`,
			}),
		},
	})
}

func TestJavaLibraryConvertsStaticLibsToDepsAndExports(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Blueprint: `java_library {
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
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
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
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Blueprint: `java_library {
    name: "java-lib-1",
    static_libs: ["java-lib-2"],
    bazel_module: { bp2build_available: true },
}

java_library {
    name: "java-lib-2",
    srcs: ["a.java"],
    bazel_module: { bp2build_available: false },
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"exports": `[":java-lib-2"]`,
			}),
		},
	})
}

func TestJavaLibraryFailsToConvertLibsWithNoSrcs(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		ExpectedErr: fmt.Errorf("Module has direct dependencies but no sources. Bazel will not allow this."),
		Blueprint: `java_library {
    name: "java-lib-1",
    libs: ["java-lib-2"],
    bazel_module: { bp2build_available: true },
}

java_library {
    name: "java-lib-2",
    srcs: ["a.java"],
    bazel_module: { bp2build_available: false },
}`,
		ExpectedBazelTargets: []string{},
	})
}

func TestJavaLibraryPlugins(t *testing.T) {
	runJavaLibraryTestCaseWithRegistrationCtxFunc(t, Bp2buildTestCase{
		Blueprint: `java_library {
    name: "java-lib-1",
    plugins: ["java-plugin-1"],
    bazel_module: { bp2build_available: true },
}

java_plugin {
    name: "java-plugin-1",
    srcs: ["a.java"],
    bazel_module: { bp2build_available: false },
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"plugins": `[":java-plugin-1"]`,
			}),
		},
	}, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("java_plugin", java.PluginFactory)
	})
}

func TestJavaLibraryJavaVersion(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java"],
    java_version: "11",
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"srcs":      `["a.java"]`,
				"javacopts": `["-source 11 -target 11"]`,
			}),
		},
	})
}

func TestJavaLibraryErrorproneJavacflagsEnabledManually(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java"],
    javacflags: ["-Xsuper-fast"],
    errorprone: {
        enabled: true,
        javacflags: ["-Xep:SpeedLimit:OFF"],
    },
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
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
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java"],
    javacflags: ["-Xsuper-fast"],
    errorprone: {
        javacflags: ["-Xep:SpeedLimit:OFF"],
    },
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"javacopts": `["-Xsuper-fast"]`,
				"srcs":      `["a.java"]`,
			}),
		},
	})
}

func TestJavaLibraryErrorproneJavacflagsErrorproneDisabledManually(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java"],
    javacflags: ["-Xsuper-fast"],
    errorprone: {
		enabled: false,
        javacflags: ["-Xep:SpeedLimit:OFF"],
    },
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"javacopts": `["-Xsuper-fast"]`,
				"srcs":      `["a.java"]`,
			}),
		},
	})
}

func TestJavaLibraryLogTags(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Description:                "Java library - logtags creates separate dependency",
		ModuleTypeUnderTest:        "java_library",
		ModuleTypeUnderTestFactory: java.LibraryFactory,
		Blueprint: `java_library {
        name: "example_lib",
        srcs: [
			"a.java",
			"b.java",
			"a.logtag",
			"b.logtag",
		],
        bazel_module: { bp2build_available: true },
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("event_log_tags", "example_lib_logtags", AttrNameToString{
				"srcs": `[
        "a.logtag",
        "b.logtag",
    ]`,
			}),
			MakeBazelTarget("java_library", "example_lib", AttrNameToString{
				"srcs": `[
        "a.java",
        "b.java",
        ":example_lib_logtags",
    ]`,
			}),
		}})
}

func TestJavaLibraryResources(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Filesystem: map[string]string{
			"res/a.res":      "",
			"res/b.res":      "",
			"res/dir1/b.res": "",
		},
		Blueprint: `java_library {
    name: "java-lib-1",
	java_resources: ["res/a.res", "res/b.res"],
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"resources": `[
        "res/a.res",
        "res/b.res",
    ]`,
			}),
		},
	})
}

func TestJavaLibraryResourceDirs(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Filesystem: map[string]string{
			"res/a.res":      "",
			"res/b.res":      "",
			"res/dir1/b.res": "",
		},
		Blueprint: `java_library {
    name: "java-lib-1",
	java_resource_dirs: ["res"],
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
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
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Filesystem: map[string]string{
			"res/a.res":         "",
			"res/exclude/b.res": "",
		},
		Blueprint: `java_library {
    name: "java-lib-1",
	java_resource_dirs: ["res"],
	exclude_java_resource_dirs: ["res/exclude"],
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"resource_strip_prefix": `"res"`,
				"resources":             `["res/a.res"]`,
			}),
		},
	})
}

func TestJavaLibraryResourcesExcludeFile(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Filesystem: map[string]string{
			"res/a.res":            "",
			"res/dir1/b.res":       "",
			"res/dir1/exclude.res": "",
		},
		Blueprint: `java_library {
    name: "java-lib-1",
	java_resource_dirs: ["res"],
	exclude_java_resources: ["res/dir1/exclude.res"],
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
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
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Filesystem: map[string]string{
			"res/a.res":  "",
			"res1/a.res": "",
		},
		Blueprint: `java_library {
    name: "java-lib-1",
	java_resource_dirs: ["res", "res1"],
}`,
		ExpectedErr:          fmt.Errorf("bp2build does not support more than one directory in java_resource_dirs (b/226423379)"),
		ExpectedBazelTargets: []string{},
	})
}

func TestJavaLibraryAidl(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Description:                "Java library - aidl creates separate dependency",
		ModuleTypeUnderTest:        "java_library",
		ModuleTypeUnderTestFactory: java.LibraryFactory,
		Blueprint: `java_library {
        name: "example_lib",
        srcs: [
			"a.java",
			"b.java",
			"a.aidl",
			"b.aidl",
		],
        bazel_module: { bp2build_available: true },
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("aidl_library", "example_lib_aidl_library", AttrNameToString{
				"srcs": `[
        "a.aidl",
        "b.aidl",
    ]`,
			}),
			MakeBazelTarget("java_aidl_library", "example_lib_java_aidl_library", AttrNameToString{
				"deps": `[":example_lib_aidl_library"]`,
			}),
			MakeBazelTarget("java_library", "example_lib", AttrNameToString{
				"deps":    `[":example_lib_java_aidl_library"]`,
				"exports": `[":example_lib_java_aidl_library"]`,
				"srcs": `[
        "a.java",
        "b.java",
    ]`,
			}),
		}})
}

func TestJavaLibraryAidlSrcsNoFileGroup(t *testing.T) {
	runJavaLibraryTestCaseWithRegistrationCtxFunc(t, Bp2buildTestCase{
		Description:                "Java library - aidl filegroup is parsed",
		ModuleTypeUnderTest:        "java_library",
		ModuleTypeUnderTestFactory: java.LibraryFactory,
		Blueprint: `
java_library {
        name: "example_lib",
        srcs: [
			"a.java",
			"b.aidl",
		],
        bazel_module: { bp2build_available: true },
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("aidl_library", "example_lib_aidl_library", AttrNameToString{
				"srcs": `["b.aidl"]`,
			}),
			MakeBazelTarget("java_aidl_library", "example_lib_java_aidl_library", AttrNameToString{
				"deps": `[":example_lib_aidl_library"]`,
			}),
			MakeBazelTarget("java_library", "example_lib", AttrNameToString{
				"deps":    `[":example_lib_java_aidl_library"]`,
				"exports": `[":example_lib_java_aidl_library"]`,
				"srcs":    `["a.java"]`,
			}),
		},
	}, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	})
}

func TestJavaLibraryAidlFilegroup(t *testing.T) {
	runJavaLibraryTestCaseWithRegistrationCtxFunc(t, Bp2buildTestCase{
		Description:                "Java library - aidl filegroup is parsed",
		ModuleTypeUnderTest:        "java_library",
		ModuleTypeUnderTestFactory: java.LibraryFactory,
		Blueprint: `
filegroup {
	name: "random_other_files",
	srcs: [
		"a.java",
		"b.java",
	],
}
filegroup {
	name: "aidl_files",
	srcs: [
		"a.aidl",
		"b.aidl",
	],
}
java_library {
        name: "example_lib",
        srcs: [
			"a.java",
			"b.java",
			":aidl_files",
			":random_other_files",
		],
        bazel_module: { bp2build_available: true },
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTargetNoRestrictions("aidl_library", "aidl_files", AttrNameToString{
				"srcs": `[
        "a.aidl",
        "b.aidl",
    ]`,
			}),
			MakeBazelTarget("java_aidl_library", "example_lib_java_aidl_library", AttrNameToString{
				"deps": `[":aidl_files"]`,
			}),
			MakeBazelTarget("java_library", "example_lib", AttrNameToString{
				"deps":    `[":example_lib_java_aidl_library"]`,
				"exports": `[":example_lib_java_aidl_library"]`,
				"srcs": `[
        "a.java",
        "b.java",
        ":random_other_files",
    ]`,
			}),
			MakeBazelTargetNoRestrictions("filegroup", "random_other_files", AttrNameToString{
				"srcs": `[
        "a.java",
        "b.java",
    ]`,
			}),
		},
	}, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	})
}

func TestJavaLibraryAidlNonAdjacentAidlFilegroup(t *testing.T) {
	runJavaLibraryTestCaseWithRegistrationCtxFunc(t, Bp2buildTestCase{
		Description:                "java_library with non adjacent aidl filegroup",
		ModuleTypeUnderTest:        "java_library",
		ModuleTypeUnderTestFactory: java.LibraryFactory,
		Filesystem: map[string]string{
			"path/to/A/Android.bp": `
filegroup {
	name: "A_aidl",
	srcs: ["aidl/A.aidl"],
	path: "aidl",
}`,
		},
		Blueprint: `
java_library {
	name: "foo",
	srcs: [
		":A_aidl",
	],
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_aidl_library", "foo_java_aidl_library", AttrNameToString{
				"deps": `["//path/to/A:A_aidl"]`,
			}),
			MakeBazelTarget("java_library", "foo", AttrNameToString{
				"exports": `[":foo_java_aidl_library"]`,
			}),
		},
	}, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	})
}

func TestConvertArmNeonVariant(t *testing.T) {
	t.Helper()
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, Bp2buildTestCase{
		Description:                "Android Library - simple arch feature",
		ModuleTypeUnderTest:        "android_library",
		ModuleTypeUnderTestFactory: java.AndroidLibraryFactory,
		Blueprint: simpleModuleDoNotConvertBp2build("android_library", "static_lib_dep") + `
android_library {
	name: "TestLib",
	manifest: "manifest/AndroidManifest.xml",
	srcs: ["lib.java"],
	arch: {
		arm: {
			neon: {
				srcs: ["arm_neon.java"],
			},
		},
	},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget(
				"android_library",
				"TestLib",
				AttrNameToString{
					"srcs": `["lib.java"] + select({
        "//build/bazel/platforms/arch/variants:arm-neon": ["arm_neon.java"],
        "//conditions:default": [],
    })`,
					"manifest":       `"manifest/AndroidManifest.xml"`,
					"resource_files": `[]`,
				}),
		}})
}

func TestConvertMultipleArchFeatures(t *testing.T) {
	t.Helper()
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, Bp2buildTestCase{
		Description:                "Android Library - multiple arch features",
		ModuleTypeUnderTest:        "android_library",
		ModuleTypeUnderTestFactory: java.AndroidLibraryFactory,
		Blueprint: simpleModuleDoNotConvertBp2build("android_library", "static_lib_dep") + `
android_library {
	name: "TestLib",
	manifest: "manifest/AndroidManifest.xml",
	srcs: ["lib.java"],
	arch: {
		x86: {
			ssse3: {
				srcs: ["ssse3.java"],
			},
			sse4_1: {
				srcs: ["sse4_1.java"],
			},
		},
	},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget(
				"android_library",
				"TestLib",
				AttrNameToString{
					"srcs": `["lib.java"] + select({
        "//build/bazel/platforms/arch/variants:x86-sse4_1": ["sse4_1.java"],
        "//build/bazel/platforms/arch/variants:x86-sse4_1-ssse3": [
            "sse4_1.java",
            "ssse3.java",
        ],
        "//build/bazel/platforms/arch/variants:x86-ssse3": ["ssse3.java"],
        "//conditions:default": [],
    })`,
					"manifest":       `"manifest/AndroidManifest.xml"`,
					"resource_files": `[]`,
				}),
		}})
}

func TestConvertExcludeSrcsArchFeature(t *testing.T) {
	t.Helper()
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, Bp2buildTestCase{
		Description:                "Android Library - exclude_srcs with arch feature",
		ModuleTypeUnderTest:        "android_library",
		ModuleTypeUnderTestFactory: java.AndroidLibraryFactory,
		Blueprint: simpleModuleDoNotConvertBp2build("android_library", "static_lib_dep") + `
android_library {
	name: "TestLib",
	manifest: "manifest/AndroidManifest.xml",
	srcs: ["lib.java"],
	arch: {
		arm: {
			srcs: ["arm_non_neon.java"],
			neon: {
				exclude_srcs: ["arm_non_neon.java"],
			},
		},
	},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget(
				"android_library",
				"TestLib",
				AttrNameToString{
					"srcs": `["lib.java"] + select({
        "//build/bazel/platforms/arch/variants:arm-neon": [],
        "//build/bazel/platforms/arch:arm": ["arm_non_neon.java"],
        "//conditions:default": [],
    })`,
					"manifest":       `"manifest/AndroidManifest.xml"`,
					"resource_files": `[]`,
				}),
		}})
}

func TestJavaLibraryKotlinSrcs(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Description: "java_library with kotlin  srcs",
		Blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java", "b.java", "c.kt"],
    bazel_module: { bp2build_available: true },
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("kt_jvm_library", "java-lib-1", AttrNameToString{
				"srcs": `[
        "a.java",
        "b.java",
        "c.kt",
    ]`,
			}),
		},
	})
}

func TestJavaLibraryKotlinCommonSrcs(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Description: "java_library with kotlin  common_srcs",
		Blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java", "b.java"],
    common_srcs: ["c.kt"],
    bazel_module: { bp2build_available: true },
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("kt_jvm_library", "java-lib-1", AttrNameToString{
				"srcs": `[
        "a.java",
        "b.java",
    ]`,
				"common_srcs": `["c.kt"]`,
			}),
		},
	})
}
