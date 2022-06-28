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
)

func registerCcObjectModuleTypes(ctx android.RegistrationContext) {
	// Always register cc_defaults module factory
	ctx.RegisterModuleType("cc_defaults", func() android.Module { return cc.DefaultsFactory() })
}

func runCcObjectTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	(&tc).moduleTypeUnderTest = "cc_object"
	(&tc).moduleTypeUnderTestFactory = cc.ObjectFactory
	runBp2BuildTestCase(t, registerCcObjectModuleTypes, tc)
}

func TestCcObjectSimple(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		description: "simple cc_object generates cc_object with include header dep",
		filesystem: map[string]string{
			"a/b/foo.h":     "",
			"a/b/bar.h":     "",
			"a/b/exclude.c": "",
			"a/b/c.c":       "",
		},
		blueprint: `cc_object {
    name: "foo",
    local_include_dirs: ["include"],
    system_shared_libs: [],
    cflags: [
        "-Wno-gcc-compat",
        "-Wall",
        "-Werror",
    ],
    srcs: [
        "a/b/*.c"
    ],
    exclude_srcs: ["a/b/exclude.c"],
    sdk_version: "current",
    min_sdk_version: "29",
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_object", "foo", attrNameToString{
				"copts": `[
        "-fno-addrsig",
        "-Wno-gcc-compat",
        "-Wall",
        "-Werror",
    ]`,
				"local_includes": `[
        "include",
        ".",
    ]`,
				"srcs":                `["a/b/c.c"]`,
				"system_dynamic_deps": `[]`,
        "sdk_version": `"current"`,
        "min_sdk_version": `"29"`,
			}),
		},
	})
}

func TestCcObjectDefaults(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		blueprint: `cc_object {
    name: "foo",
    system_shared_libs: [],
    srcs: [
        "a/b/*.h",
        "a/b/c.c"
    ],

    defaults: ["foo_defaults"],
}

cc_defaults {
    name: "foo_defaults",
    defaults: ["foo_bar_defaults"],
}

cc_defaults {
    name: "foo_bar_defaults",
    cflags: [
        "-Werror",
    ],
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_object", "foo", attrNameToString{
				"copts": `[
        "-Werror",
        "-fno-addrsig",
    ]`,
				"local_includes":      `["."]`,
				"srcs":                `["a/b/c.c"]`,
				"system_dynamic_deps": `[]`,
			}),
		}})
}

func TestCcObjectCcObjetDepsInObjs(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		description: "cc_object with cc_object deps in objs props",
		filesystem: map[string]string{
			"a/b/c.c": "",
			"x/y/z.c": "",
		},
		blueprint: `cc_object {
    name: "foo",
    system_shared_libs: [],
    srcs: ["a/b/c.c"],
    objs: ["bar"],
    include_build_directory: false,
}

cc_object {
    name: "bar",
    system_shared_libs: [],
    srcs: ["x/y/z.c"],
    include_build_directory: false,
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_object", "bar", attrNameToString{
				"copts":               `["-fno-addrsig"]`,
				"srcs":                `["x/y/z.c"]`,
				"system_dynamic_deps": `[]`,
			}), makeBazelTarget("cc_object", "foo", attrNameToString{
				"copts":               `["-fno-addrsig"]`,
				"deps":                `[":bar"]`,
				"srcs":                `["a/b/c.c"]`,
				"system_dynamic_deps": `[]`,
			}),
		},
	})
}

func TestCcObjectIncludeBuildDirFalse(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		description: "cc_object with include_build_dir: false",
		filesystem: map[string]string{
			"a/b/c.c": "",
			"x/y/z.c": "",
		},
		blueprint: `cc_object {
    name: "foo",
    system_shared_libs: [],
    srcs: ["a/b/c.c"],
    include_build_directory: false,
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_object", "foo", attrNameToString{
				"copts":               `["-fno-addrsig"]`,
				"srcs":                `["a/b/c.c"]`,
				"system_dynamic_deps": `[]`,
			}),
		},
	})
}

func TestCcObjectProductVariable(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		description: "cc_object with product variable",
		blueprint: `cc_object {
    name: "foo",
    system_shared_libs: [],
    include_build_directory: false,
    product_variables: {
        platform_sdk_version: {
            asflags: ["-DPLATFORM_SDK_VERSION=%d"],
        },
    },
    srcs: ["src.S"],
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_object", "foo", attrNameToString{
				"asflags": `select({
        "//build/bazel/product_variables:platform_sdk_version": ["-DPLATFORM_SDK_VERSION=$(Platform_sdk_version)"],
        "//conditions:default": [],
    })`,
				"copts":               `["-fno-addrsig"]`,
				"srcs_as":             `["src.S"]`,
				"system_dynamic_deps": `[]`,
			}),
		},
	})
}

func TestCcObjectCflagsOneArch(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		description: "cc_object setting cflags for one arch",
		blueprint: `cc_object {
    name: "foo",
    system_shared_libs: [],
    srcs: ["a.cpp"],
    arch: {
        x86: {
            cflags: ["-fPIC"], // string list
        },
        arm: {
            srcs: ["arch/arm/file.cpp"], // label list
        },
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_object", "foo", attrNameToString{
				"copts": `["-fno-addrsig"] + select({
        "//build/bazel/platforms/arch:x86": ["-fPIC"],
        "//conditions:default": [],
    })`,
				"srcs": `["a.cpp"] + select({
        "//build/bazel/platforms/arch:arm": ["arch/arm/file.cpp"],
        "//conditions:default": [],
    })`,
				"system_dynamic_deps": `[]`,
			}),
		},
	})
}

func TestCcObjectCflagsFourArch(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		description: "cc_object setting cflags for 4 architectures",
		blueprint: `cc_object {
    name: "foo",
    system_shared_libs: [],
    srcs: ["base.cpp"],
    arch: {
        x86: {
            srcs: ["x86.cpp"],
            cflags: ["-fPIC"],
        },
        x86_64: {
            srcs: ["x86_64.cpp"],
            cflags: ["-fPIC"],
        },
        arm: {
            srcs: ["arm.cpp"],
            cflags: ["-Wall"],
        },
        arm64: {
            srcs: ["arm64.cpp"],
            cflags: ["-Wall"],
        },
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_object", "foo", attrNameToString{
				"copts": `["-fno-addrsig"] + select({
        "//build/bazel/platforms/arch:arm": ["-Wall"],
        "//build/bazel/platforms/arch:arm64": ["-Wall"],
        "//build/bazel/platforms/arch:x86": ["-fPIC"],
        "//build/bazel/platforms/arch:x86_64": ["-fPIC"],
        "//conditions:default": [],
    })`,
				"srcs": `["base.cpp"] + select({
        "//build/bazel/platforms/arch:arm": ["arm.cpp"],
        "//build/bazel/platforms/arch:arm64": ["arm64.cpp"],
        "//build/bazel/platforms/arch:x86": ["x86.cpp"],
        "//build/bazel/platforms/arch:x86_64": ["x86_64.cpp"],
        "//conditions:default": [],
    })`,
				"system_dynamic_deps": `[]`,
			}),
		},
	})
}

func TestCcObjectLinkerScript(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		description: "cc_object setting linker_script",
		blueprint: `cc_object {
    name: "foo",
    srcs: ["base.cpp"],
    linker_script: "bunny.lds",
    include_build_directory: false,
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_object", "foo", attrNameToString{
				"copts":         `["-fno-addrsig"]`,
				"linker_script": `"bunny.lds"`,
				"srcs":          `["base.cpp"]`,
			}),
		},
	})
}

func TestCcObjectDepsAndLinkerScriptSelects(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		description: "cc_object setting deps and linker_script across archs",
		blueprint: `cc_object {
    name: "foo",
    srcs: ["base.cpp"],
    arch: {
        x86: {
            objs: ["x86_obj"],
            linker_script: "x86.lds",
        },
        x86_64: {
            objs: ["x86_64_obj"],
            linker_script: "x86_64.lds",
        },
        arm: {
            objs: ["arm_obj"],
            linker_script: "arm.lds",
        },
    },
    include_build_directory: false,
}

cc_object {
    name: "x86_obj",
    system_shared_libs: [],
    srcs: ["x86.cpp"],
    include_build_directory: false,
    bazel_module: { bp2build_available: false },
}

cc_object {
    name: "x86_64_obj",
    system_shared_libs: [],
    srcs: ["x86_64.cpp"],
    include_build_directory: false,
    bazel_module: { bp2build_available: false },
}

cc_object {
    name: "arm_obj",
    system_shared_libs: [],
    srcs: ["arm.cpp"],
    include_build_directory: false,
    bazel_module: { bp2build_available: false },
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_object", "foo", attrNameToString{
				"copts": `["-fno-addrsig"]`,
				"deps": `select({
        "//build/bazel/platforms/arch:arm": [":arm_obj"],
        "//build/bazel/platforms/arch:x86": [":x86_obj"],
        "//build/bazel/platforms/arch:x86_64": [":x86_64_obj"],
        "//conditions:default": [],
    })`,
				"linker_script": `select({
        "//build/bazel/platforms/arch:arm": "arm.lds",
        "//build/bazel/platforms/arch:x86": "x86.lds",
        "//build/bazel/platforms/arch:x86_64": "x86_64.lds",
        "//conditions:default": None,
    })`,
				"srcs": `["base.cpp"]`,
			}),
		},
	})
}

func TestCcObjectSelectOnLinuxAndBionicArchs(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		description: "cc_object setting srcs based on linux and bionic archs",
		blueprint: `cc_object {
    name: "foo",
    srcs: ["base.cpp"],
    target: {
        linux_arm64: {
            srcs: ["linux_arm64.cpp",]
        },
        linux_x86: {
            srcs: ["linux_x86.cpp",]
        },
        bionic_arm64: {
            srcs: ["bionic_arm64.cpp",]
        },
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_object", "foo", attrNameToString{
				"copts": `["-fno-addrsig"]`,
				"srcs": `["base.cpp"] + select({
        "//build/bazel/platforms/os_arch:android_arm64": [
            "linux_arm64.cpp",
            "bionic_arm64.cpp",
        ],
        "//build/bazel/platforms/os_arch:android_x86": ["linux_x86.cpp"],
        "//build/bazel/platforms/os_arch:linux_bionic_arm64": [
            "linux_arm64.cpp",
            "bionic_arm64.cpp",
        ],
        "//build/bazel/platforms/os_arch:linux_glibc_x86": ["linux_x86.cpp"],
        "//build/bazel/platforms/os_arch:linux_musl_arm64": ["linux_arm64.cpp"],
        "//build/bazel/platforms/os_arch:linux_musl_x86": ["linux_x86.cpp"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}
