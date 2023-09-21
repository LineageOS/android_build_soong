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
    sdk_version: "current",
    bazel_module: { bp2build_available: true },
}

java_library {
    name: "java-lib-2",
    srcs: ["b.java"],
    sdk_version: "current",
    bazel_module: { bp2build_available: true },
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"srcs":        `["a.java"]`,
				"deps":        `[":java-lib-2-neverlink"]`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-1"),
			MakeBazelTarget("java_library", "java-lib-2", AttrNameToString{
				"srcs":        `["b.java"]`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-2"),
		},
	})
}

func TestJavaLibraryConvertsStaticLibsToDepsAndExports(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		StubbedBuildDefinitions: []string{"java-lib-2", "java-lib-3"},
		Blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java"],
    libs: ["java-lib-2"],
    static_libs: ["java-lib-3"],
    sdk_version: "current",
    bazel_module: { bp2build_available: true },
}

java_library {
    name: "java-lib-2",
    srcs: ["b.java"],
    sdk_version: "current",
}

java_library {
    name: "java-lib-3",
    srcs: ["c.java"],
    sdk_version: "current",
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"srcs": `["a.java"]`,
				"deps": `[
        ":java-lib-2-neverlink",
        ":java-lib-3",
    ]`,
				"exports":     `[":java-lib-3"]`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-1"),
		},
	})
}

func TestJavaLibraryConvertsStaticLibsToExportsIfNoSrcs(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		StubbedBuildDefinitions: []string{"java-lib-2"},
		Blueprint: `java_library {
    name: "java-lib-1",
    static_libs: ["java-lib-2"],
    sdk_version: "current",
    bazel_module: { bp2build_available: true },
}

java_library {
    name: "java-lib-2",
    srcs: ["a.java"],
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"exports":     `[":java-lib-2"]`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-1"),
		},
	})
}

func TestJavaLibraryFailsToConvertNoSdkVersion(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Blueprint: `
java_library {
    name: "lib",
    bazel_module: { bp2build_available: true },
}
`,
		ExpectedBazelTargets: []string{}, // no targets expected because sdk_version is not set
	})
}

func TestJavaLibraryPlugins(t *testing.T) {
	runJavaLibraryTestCaseWithRegistrationCtxFunc(t, Bp2buildTestCase{
		StubbedBuildDefinitions: []string{"java-plugin-1"},
		Blueprint: `java_library {
    name: "java-lib-1",
    plugins: ["java-plugin-1"],
    sdk_version: "current",
    bazel_module: { bp2build_available: true },
}

java_plugin {
    name: "java-plugin-1",
    srcs: ["a.java"],
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"plugins":     `[":java-plugin-1"]`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-1"),
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
    sdk_version: "current",
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"srcs":         `["a.java"]`,
				"java_version": `"11"`,
				"sdk_version":  `"current"`,
			}),
			MakeNeverlinkDuplicateTargetWithAttrs(
				"java_library",
				"java-lib-1",
				AttrNameToString{
					"java_version": `"11"`,
					"sdk_version":  `"current"`,
				}),
		},
	})
}

func TestJavaLibraryErrorproneEnabledManually(t *testing.T) {
	runJavaLibraryTestCaseWithRegistrationCtxFunc(t, Bp2buildTestCase{
		StubbedBuildDefinitions: []string{"plugin2"},
		Blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java"],
    javacflags: ["-Xsuper-fast"],
    sdk_version: "current",
    errorprone: {
        enabled: true,
        javacflags: ["-Xep:SpeedLimit:OFF"],
        extra_check_modules: ["plugin2"],
    },
}
java_plugin {
    name: "plugin2",
    srcs: ["a.java"],
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"javacopts": `[
        "-Xsuper-fast",
        "-Xep:SpeedLimit:OFF",
    ]`,
				"plugins":                 `[":plugin2"]`,
				"srcs":                    `["a.java"]`,
				"errorprone_force_enable": `True`,
				"sdk_version":             `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-1"),
		},
	}, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("java_plugin", java.PluginFactory)
	})
}

func TestJavaLibraryErrorproneJavacflagsErrorproneDisabledByDefault(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java"],
    sdk_version: "current",
    javacflags: ["-Xsuper-fast"],
    errorprone: {
        javacflags: ["-Xep:SpeedLimit:OFF"],
    },
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"javacopts":   `["-Xsuper-fast"]`,
				"srcs":        `["a.java"]`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-1"),
		},
	})
}

func TestJavaLibraryErrorproneDisabledManually(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java"],
    javacflags: ["-Xsuper-fast"],
    sdk_version: "current",
    errorprone: {
    enabled: false,
    },
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"javacopts": `[
        "-Xsuper-fast",
        "-XepDisableAllChecks",
    ]`,
				"srcs":        `["a.java"]`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-1"),
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
	sdk_version: "current",
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
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "example_lib"),
		}})
}

func TestJavaLibraryResources(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Dir: "adir",
		Filesystem: map[string]string{
			"adir/res/a.res":      "",
			"adir/res/b.res":      "",
			"adir/res/dir1/b.res": "",
			"adir/Android.bp": `java_library {
	name: "java-lib-1",
	java_resources: ["res/a.res", "res/b.res"],
	sdk_version: "current",
	bazel_module: { bp2build_available: true },
}`,
		},
		Blueprint: "",
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"resources": `[
        "res/a.res",
        "res/b.res",
    ]`,
				"resource_strip_prefix": `"adir"`,
				"sdk_version":           `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-1"),
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
	sdk_version: "current",
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"resource_strip_prefix": `"res"`,
				"resources": `[
        "res/a.res",
        "res/b.res",
        "res/dir1/b.res",
    ]`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-1"),
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
	sdk_version: "current",
	exclude_java_resource_dirs: ["res/exclude"],
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"resource_strip_prefix": `"res"`,
				"resources":             `["res/a.res"]`,
				"sdk_version":           `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-1"),
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
	sdk_version: "current",
	exclude_java_resources: ["res/dir1/exclude.res"],
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"resource_strip_prefix": `"res"`,
				"resources": `[
        "res/a.res",
        "res/dir1/b.res",
    ]`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-1"),
		},
	})
}

func TestJavaLibraryResourcesWithMultipleDirs(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Filesystem: map[string]string{
			"res/a.res":  "",
			"res1/b.res": "",
		},
		Blueprint: `java_library {
	name: "java-lib-1",
	java_resource_dirs: ["res", "res1"],
	sdk_version: "current",
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_resources", "java-lib-1_resource_dir_res1", AttrNameToString{
				"resource_strip_prefix": `"res1"`,
				"resources":             `["res1/b.res"]`,
			}),
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"additional_resources":  `["java-lib-1_resource_dir_res1"]`,
				"resources":             `["res/a.res"]`,
				"resource_strip_prefix": `"res"`,
				"sdk_version":           `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-1"),
		},
	})
}

func TestJavaLibraryJavaResourcesAndResourceDirs(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Filesystem: map[string]string{
			"resdir/a.res": "",
		},
		Blueprint: `java_library {
		name: "java-lib-1",
		java_resources: ["res1", "res2"],
		java_resource_dirs: ["resdir"],
		sdk_version: "current",
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_resources", "java-lib-1_resource_dir_resdir", AttrNameToString{
				"resource_strip_prefix": `"resdir"`,
				"resources":             `["resdir/a.res"]`,
			}),
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"additional_resources":  `["java-lib-1_resource_dir_resdir"]`,
				"resource_strip_prefix": `"."`,
				"resources": `[
        "res1",
        "res2",
    ]`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-1"),
		},
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
	sdk_version: "current",
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
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "example_lib"),
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
	sdk_version: "current",
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
				"deps":        `[":example_lib_java_aidl_library"]`,
				"exports":     `[":example_lib_java_aidl_library"]`,
				"srcs":        `["a.java"]`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "example_lib"),
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
	sdk_version: "current",
	bazel_module: { bp2build_available: true },
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTargetNoRestrictions("aidl_library", "aidl_files", AttrNameToString{
				"srcs": `[
        "a.aidl",
        "b.aidl",
    ]`,
				"tags": `["apex_available=//apex_available:anyapex"]`,
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
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "example_lib"),
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
		StubbedBuildDefinitions:    []string{"A_aidl"},
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
  sdk_version: "current",
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_aidl_library", "foo_java_aidl_library", AttrNameToString{
				"deps": `["//path/to/A:A_aidl"]`,
			}),
			MakeBazelTarget("java_library", "foo", AttrNameToString{
				"exports":     `[":foo_java_aidl_library"]`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "foo"),
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
		Blueprint: simpleModule("android_library", "static_lib_dep") + `
android_library {
  name: "TestLib",
  manifest: "manifest/AndroidManifest.xml",
  srcs: ["lib.java"],
  sdk_version: "current",
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
					"sdk_version":    `"current"`, // use as default
				}),
			MakeNeverlinkDuplicateTarget("android_library", "TestLib"),
		}})
}

func TestConvertMultipleArchFeatures(t *testing.T) {
	t.Helper()
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, Bp2buildTestCase{
		Description:                "Android Library - multiple arch features",
		ModuleTypeUnderTest:        "android_library",
		ModuleTypeUnderTestFactory: java.AndroidLibraryFactory,
		Blueprint: simpleModule("android_library", "static_lib_dep") + `
android_library {
  name: "TestLib",
  manifest: "manifest/AndroidManifest.xml",
  srcs: ["lib.java"],
  sdk_version: "current",
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
					"sdk_version":    `"current"`,
				}),
			MakeNeverlinkDuplicateTarget("android_library", "TestLib"),
		}})
}

func TestConvertExcludeSrcsArchFeature(t *testing.T) {
	t.Helper()
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, Bp2buildTestCase{
		Description:                "Android Library - exclude_srcs with arch feature",
		ModuleTypeUnderTest:        "android_library",
		ModuleTypeUnderTestFactory: java.AndroidLibraryFactory,
		Blueprint: simpleModule("android_library", "static_lib_dep") + `
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
  sdk_version: "current",
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
					"sdk_version":    `"current"`, // use as default
				}),
			MakeNeverlinkDuplicateTarget("android_library", "TestLib"),
		}})
}

func TestJavaLibraryKotlinSrcs(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Description: "java_library with kotlin srcs",
		Blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java", "b.java", "c.kt"],
    bazel_module: { bp2build_available: true },
    sdk_version: "current",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("kt_jvm_library", "java-lib-1", AttrNameToString{
				"srcs": `[
        "a.java",
        "b.java",
        "c.kt",
    ]`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("kt_jvm_library", "java-lib-1"),
		},
	})
}

func TestJavaLibraryKotlincflags(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Description: "java_library with kotlincfalgs",
		Blueprint: `java_library {
    name: "java-lib-1",
    srcs: [ "a.kt"],
    kotlincflags: ["-flag1", "-flag2"],
    bazel_module: { bp2build_available: true },
    sdk_version: "current",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("kt_jvm_library", "java-lib-1", AttrNameToString{
				"srcs": `["a.kt"]`,
				"kotlincflags": `[
        "-flag1",
        "-flag2",
    ]`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("kt_jvm_library", "java-lib-1"),
		},
	})
}

func TestJavaLibraryKotlinCommonSrcs(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Description: "java_library with kotlin common_srcs",
		Blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java", "b.java"],
    common_srcs: ["c.kt"],
    bazel_module: { bp2build_available: true },
    sdk_version: "current",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("kt_jvm_library", "java-lib-1", AttrNameToString{
				"srcs": `[
        "a.java",
        "b.java",
    ]`,
				"common_srcs": `["c.kt"]`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("kt_jvm_library", "java-lib-1"),
		},
	})
}

func TestJavaLibraryArchVariantDeps(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Description:             "java_library with arch variant libs",
		StubbedBuildDefinitions: []string{"java-lib-2", "java-lib-3", "java-lib-4"},
		Blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java"],
    libs: ["java-lib-2"],
    sdk_version: "current",
    target: {
        android: {
            libs: ["java-lib-3"],
            static_libs: ["java-lib-4"],
        },
    },
    bazel_module: { bp2build_available: true },
}

  java_library{
    name: "java-lib-2",
}

  java_library{
    name: "java-lib-3",
}

  java_library{
    name: "java-lib-4",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"srcs": `["a.java"]`,
				"exports": `select({
        "//build/bazel/platforms/os:android": [":java-lib-4"],
        "//conditions:default": [],
    })`,
				"deps": `[":java-lib-2-neverlink"] + select({
        "//build/bazel/platforms/os:android": [
            ":java-lib-3-neverlink",
            ":java-lib-4",
        ],
        "//conditions:default": [],
    })`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-1"),
		},
	})
}

func TestJavaLibraryArchVariantSrcsWithExcludes(t *testing.T) {
	runJavaLibraryTestCase(t, Bp2buildTestCase{
		Description: "java_library with arch variant libs",
		Blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java", "b.java"],
    sdk_version: "current",
    target: {
        android: {
            exclude_srcs: ["a.java"],
        },
    },
    bazel_module: { bp2build_available: true },
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"srcs": `["b.java"] + select({
        "//build/bazel/platforms/os:android": [],
        "//conditions:default": ["a.java"],
    })`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-1"),
		},
	})
}

func TestJavaLibraryJavaResourcesSingleFilegroup(t *testing.T) {
	runJavaLibraryTestCaseWithRegistrationCtxFunc(t, Bp2buildTestCase{
		Filesystem: map[string]string{
			"res/a.res":      "",
			"res/b.res":      "",
			"res/dir1/b.res": "",
		},
		Description: "java_library",
		Blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java"],
    sdk_version: "current",
    java_resources: [":filegroup1"],
    bazel_module: { bp2build_available: true },
}

filegroup {
    name: "filegroup1",
    path: "foo",
    srcs: ["foo/a", "foo/b"],
}

`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"srcs":                  `["a.java"]`,
				"resources":             `[":filegroup1"]`,
				"resource_strip_prefix": `"foo"`,
				"sdk_version":           `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-1"),
			MakeBazelTargetNoRestrictions("filegroup", "filegroup1", AttrNameToString{
				"srcs": `[
        "foo/a",
        "foo/b",
    ]`}),
		},
	}, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	})
}

func TestJavaLibraryJavaResourcesMultipleFilegroup(t *testing.T) {
	runJavaLibraryTestCaseWithRegistrationCtxFunc(t, Bp2buildTestCase{
		Filesystem: map[string]string{
			"a.res": "",
		},
		Description: "with java_resources that has multiple filegroups",
		Blueprint: `java_library {
    name: "java-lib-1",
    srcs: ["a.java"],
    java_resources: ["a.res", ":filegroup1", ":filegroup2"],
    sdk_version: "current",
    bazel_module: { bp2build_available: true },
}

filegroup {
    name: "filegroup1",
    path: "foo",
    srcs: ["foo/a"],
}

filegroup {
    name: "filegroup2",
    path: "bar",
    srcs: ["bar/a"],
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_resources", "java-lib-1_filegroup_resources_filegroup1", AttrNameToString{
				"resource_strip_prefix": `"foo"`,
				"resources":             `[":filegroup1"]`,
			}),
			MakeBazelTarget("java_resources", "java-lib-1_filegroup_resources_filegroup2", AttrNameToString{
				"resource_strip_prefix": `"bar"`,
				"resources":             `[":filegroup2"]`,
			}),
			MakeBazelTarget("java_library", "java-lib-1", AttrNameToString{
				"srcs":                  `["a.java"]`,
				"resources":             `["a.res"]`,
				"resource_strip_prefix": `"."`,
				"additional_resources": `[
        "java-lib-1_filegroup_resources_filegroup1",
        "java-lib-1_filegroup_resources_filegroup2",
    ]`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-1"),
			MakeBazelTargetNoRestrictions("filegroup", "filegroup1", AttrNameToString{
				"srcs": `["foo/a"]`}),
			MakeBazelTargetNoRestrictions("filegroup", "filegroup2", AttrNameToString{
				"srcs": `["bar/a"]`}),
		},
	}, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	})
}
