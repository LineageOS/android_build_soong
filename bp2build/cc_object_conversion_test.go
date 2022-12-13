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
	ctx.RegisterModuleType("cc_library_headers", cc.LibraryHeaderFactory)
}

func runCcObjectTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	(&tc).ModuleTypeUnderTest = "cc_object"
	(&tc).ModuleTypeUnderTestFactory = cc.ObjectFactory
	RunBp2BuildTestCase(t, registerCcObjectModuleTypes, tc)
}

func TestCcObjectSimple(t *testing.T) {
	runCcObjectTestCase(t, Bp2buildTestCase{
		Description: "simple cc_object generates cc_object with include header dep",
		Filesystem: map[string]string{
			"a/b/foo.h":     "",
			"a/b/bar.h":     "",
			"a/b/exclude.c": "",
			"a/b/c.c":       "",
		},
		Blueprint: `cc_object {
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
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_object", "foo", AttrNameToString{
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
				"sdk_version":         `"current"`,
				"min_sdk_version":     `"29"`,
			}),
		},
	})
}

func TestCcObjectDefaults(t *testing.T) {
	runCcObjectTestCase(t, Bp2buildTestCase{
		Blueprint: `cc_object {
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
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_object", "foo", AttrNameToString{
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
	runCcObjectTestCase(t, Bp2buildTestCase{
		Description: "cc_object with cc_object deps in objs props",
		Filesystem: map[string]string{
			"a/b/c.c": "",
			"x/y/z.c": "",
		},
		Blueprint: `cc_object {
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
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_object", "bar", AttrNameToString{
				"copts":               `["-fno-addrsig"]`,
				"srcs":                `["x/y/z.c"]`,
				"system_dynamic_deps": `[]`,
			}), MakeBazelTarget("cc_object", "foo", AttrNameToString{
				"copts":               `["-fno-addrsig"]`,
				"objs":                `[":bar"]`,
				"srcs":                `["a/b/c.c"]`,
				"system_dynamic_deps": `[]`,
			}),
		},
	})
}

func TestCcObjectIncludeBuildDirFalse(t *testing.T) {
	runCcObjectTestCase(t, Bp2buildTestCase{
		Description: "cc_object with include_build_dir: false",
		Filesystem: map[string]string{
			"a/b/c.c": "",
			"x/y/z.c": "",
		},
		Blueprint: `cc_object {
    name: "foo",
    system_shared_libs: [],
    srcs: ["a/b/c.c"],
    include_build_directory: false,
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_object", "foo", AttrNameToString{
				"copts":               `["-fno-addrsig"]`,
				"srcs":                `["a/b/c.c"]`,
				"system_dynamic_deps": `[]`,
			}),
		},
	})
}

func TestCcObjectProductVariable(t *testing.T) {
	runCcObjectTestCase(t, Bp2buildTestCase{
		Description: "cc_object with product variable",
		Blueprint: `cc_object {
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
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_object", "foo", AttrNameToString{
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
	runCcObjectTestCase(t, Bp2buildTestCase{
		Description: "cc_object setting cflags for one arch",
		Blueprint: `cc_object {
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
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_object", "foo", AttrNameToString{
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
	runCcObjectTestCase(t, Bp2buildTestCase{
		Description: "cc_object setting cflags for 4 architectures",
		Blueprint: `cc_object {
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
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_object", "foo", AttrNameToString{
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
	runCcObjectTestCase(t, Bp2buildTestCase{
		Description: "cc_object setting linker_script",
		Blueprint: `cc_object {
    name: "foo",
    srcs: ["base.cpp"],
    linker_script: "bunny.lds",
    include_build_directory: false,
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_object", "foo", AttrNameToString{
				"copts":         `["-fno-addrsig"]`,
				"linker_script": `"bunny.lds"`,
				"srcs":          `["base.cpp"]`,
			}),
		},
	})
}

func TestCcObjectDepsAndLinkerScriptSelects(t *testing.T) {
	runCcObjectTestCase(t, Bp2buildTestCase{
		Description: "cc_object setting deps and linker_script across archs",
		Blueprint: `cc_object {
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
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_object", "foo", AttrNameToString{
				"copts": `["-fno-addrsig"]`,
				"objs": `select({
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
	runCcObjectTestCase(t, Bp2buildTestCase{
		Description: "cc_object setting srcs based on linux and bionic archs",
		Blueprint: `cc_object {
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
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_object", "foo", AttrNameToString{
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

func TestCcObjectHeaderLib(t *testing.T) {
	runCcObjectTestCase(t, Bp2buildTestCase{
		Description: "simple cc_object generates cc_object with include header dep",
		Filesystem: map[string]string{
			"a/b/foo.h":     "",
			"a/b/bar.h":     "",
			"a/b/exclude.c": "",
			"a/b/c.c":       "",
		},
		Blueprint: `cc_object {
    name: "foo",
	header_libs: ["libheaders"],
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

cc_library_headers {
    name: "libheaders",
	export_include_dirs: ["include"],
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_object", "foo", AttrNameToString{
				"copts": `[
        "-fno-addrsig",
        "-Wno-gcc-compat",
        "-Wall",
        "-Werror",
    ]`,
				"deps":                `[":libheaders"]`,
				"local_includes":      `["."]`,
				"srcs":                `["a/b/c.c"]`,
				"system_dynamic_deps": `[]`,
				"sdk_version":         `"current"`,
				"min_sdk_version":     `"29"`,
			}),
			MakeBazelTarget("cc_library_headers", "libheaders", AttrNameToString{
				"export_includes": `["include"]`,
			}),
		},
	})
}
