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
	runBp2BuildTestCase(t, registerCcObjectModuleTypes, tc)
}

func TestCcObjectSimple(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		description:                        "simple cc_object generates cc_object with include header dep",
		moduleTypeUnderTest:                "cc_object",
		moduleTypeUnderTestFactory:         cc.ObjectFactory,
		moduleTypeUnderTestBp2BuildMutator: cc.ObjectBp2Build,
		filesystem: map[string]string{
			"a/b/foo.h":     "",
			"a/b/bar.h":     "",
			"a/b/exclude.c": "",
			"a/b/c.c":       "",
		},
		blueprint: `cc_object {
    name: "foo",
    local_include_dirs: ["include"],
    cflags: [
        "-Wno-gcc-compat",
        "-Wall",
        "-Werror",
    ],
    srcs: [
        "a/b/*.c"
    ],
    exclude_srcs: ["a/b/exclude.c"],
}
`,
		expectedBazelTargets: []string{`cc_object(
    name = "foo",
    copts = [
        "-fno-addrsig",
        "-Wno-gcc-compat",
        "-Wall",
        "-Werror",
        "-Iinclude",
        "-I$(BINDIR)/include",
        "-I.",
        "-I$(BINDIR)/.",
    ],
    srcs = ["a/b/c.c"],
)`,
		},
	})
}

func TestCcObjectDefaults(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		description:                        "simple cc_object with defaults",
		moduleTypeUnderTest:                "cc_object",
		moduleTypeUnderTestFactory:         cc.ObjectFactory,
		moduleTypeUnderTestBp2BuildMutator: cc.ObjectBp2Build,
		blueprint: `cc_object {
    name: "foo",
    local_include_dirs: ["include"],
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
        "-Wno-gcc-compat",
        "-Wall",
        "-Werror",
    ],
}
`,
		expectedBazelTargets: []string{`cc_object(
    name = "foo",
    copts = [
        "-Wno-gcc-compat",
        "-Wall",
        "-Werror",
        "-fno-addrsig",
        "-Iinclude",
        "-I$(BINDIR)/include",
        "-I.",
        "-I$(BINDIR)/.",
    ],
    srcs = ["a/b/c.c"],
)`,
		}})
}

func TestCcObjectCcObjetDepsInObjs(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		description:                        "cc_object with cc_object deps in objs props",
		moduleTypeUnderTest:                "cc_object",
		moduleTypeUnderTestFactory:         cc.ObjectFactory,
		moduleTypeUnderTestBp2BuildMutator: cc.ObjectBp2Build,
		filesystem: map[string]string{
			"a/b/c.c": "",
			"x/y/z.c": "",
		},
		blueprint: `cc_object {
    name: "foo",
    srcs: ["a/b/c.c"],
    objs: ["bar"],
}

cc_object {
    name: "bar",
    srcs: ["x/y/z.c"],
}
`,
		expectedBazelTargets: []string{`cc_object(
    name = "bar",
    copts = [
        "-fno-addrsig",
        "-I.",
        "-I$(BINDIR)/.",
    ],
    srcs = ["x/y/z.c"],
)`, `cc_object(
    name = "foo",
    copts = [
        "-fno-addrsig",
        "-I.",
        "-I$(BINDIR)/.",
    ],
    deps = [":bar"],
    srcs = ["a/b/c.c"],
)`,
		},
	})
}

func TestCcObjectIncludeBuildDirFalse(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		description:                        "cc_object with include_build_dir: false",
		moduleTypeUnderTest:                "cc_object",
		moduleTypeUnderTestFactory:         cc.ObjectFactory,
		moduleTypeUnderTestBp2BuildMutator: cc.ObjectBp2Build,
		filesystem: map[string]string{
			"a/b/c.c": "",
			"x/y/z.c": "",
		},
		blueprint: `cc_object {
    name: "foo",
    srcs: ["a/b/c.c"],
    include_build_directory: false,
}
`,
		expectedBazelTargets: []string{`cc_object(
    name = "foo",
    copts = ["-fno-addrsig"],
    srcs = ["a/b/c.c"],
)`,
		},
	})
}

func TestCcObjectProductVariable(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		description:                        "cc_object with product variable",
		moduleTypeUnderTest:                "cc_object",
		moduleTypeUnderTestFactory:         cc.ObjectFactory,
		moduleTypeUnderTestBp2BuildMutator: cc.ObjectBp2Build,
		blueprint: `cc_object {
    name: "foo",
    include_build_directory: false,
    product_variables: {
        platform_sdk_version: {
            asflags: ["-DPLATFORM_SDK_VERSION=%d"],
        },
    },
}
`,
		expectedBazelTargets: []string{`cc_object(
    name = "foo",
    asflags = select({
        "//build/bazel/product_variables:platform_sdk_version": ["-DPLATFORM_SDK_VERSION={Platform_sdk_version}"],
        "//conditions:default": [],
    }),
    copts = ["-fno-addrsig"],
)`,
		},
	})
}

func TestCcObjectCflagsOneArch(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		description:                        "cc_object setting cflags for one arch",
		moduleTypeUnderTest:                "cc_object",
		moduleTypeUnderTestFactory:         cc.ObjectFactory,
		moduleTypeUnderTestBp2BuildMutator: cc.ObjectBp2Build,
		blueprint: `cc_object {
    name: "foo",
    srcs: ["a.cpp"],
    arch: {
        x86: {
            cflags: ["-fPIC"], // string list
        },
        arm: {
            srcs: ["arch/arm/file.S"], // label list
        },
    },
}
`,
		expectedBazelTargets: []string{
			`cc_object(
    name = "foo",
    copts = [
        "-fno-addrsig",
        "-I.",
        "-I$(BINDIR)/.",
    ] + select({
        "//build/bazel/platforms/arch:x86": ["-fPIC"],
        "//conditions:default": [],
    }),
    srcs = ["a.cpp"] + select({
        "//build/bazel/platforms/arch:arm": ["arch/arm/file.S"],
        "//conditions:default": [],
    }),
)`,
		},
	})
}

func TestCcObjectCflagsFourArch(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		description:                        "cc_object setting cflags for 4 architectures",
		moduleTypeUnderTest:                "cc_object",
		moduleTypeUnderTestFactory:         cc.ObjectFactory,
		moduleTypeUnderTestBp2BuildMutator: cc.ObjectBp2Build,
		blueprint: `cc_object {
    name: "foo",
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
}
`,
		expectedBazelTargets: []string{
			`cc_object(
    name = "foo",
    copts = [
        "-fno-addrsig",
        "-I.",
        "-I$(BINDIR)/.",
    ] + select({
        "//build/bazel/platforms/arch:arm": ["-Wall"],
        "//build/bazel/platforms/arch:arm64": ["-Wall"],
        "//build/bazel/platforms/arch:x86": ["-fPIC"],
        "//build/bazel/platforms/arch:x86_64": ["-fPIC"],
        "//conditions:default": [],
    }),
    srcs = ["base.cpp"] + select({
        "//build/bazel/platforms/arch:arm": ["arm.cpp"],
        "//build/bazel/platforms/arch:arm64": ["arm64.cpp"],
        "//build/bazel/platforms/arch:x86": ["x86.cpp"],
        "//build/bazel/platforms/arch:x86_64": ["x86_64.cpp"],
        "//conditions:default": [],
    }),
)`,
		},
	})
}

func TestCcObjectCflagsMultiOs(t *testing.T) {
	runCcObjectTestCase(t, bp2buildTestCase{
		description:                        "cc_object setting cflags for multiple OSes",
		moduleTypeUnderTest:                "cc_object",
		moduleTypeUnderTestFactory:         cc.ObjectFactory,
		moduleTypeUnderTestBp2BuildMutator: cc.ObjectBp2Build,
		blueprint: `cc_object {
    name: "foo",
    srcs: ["base.cpp"],
    target: {
        android: {
            cflags: ["-fPIC"],
        },
        windows: {
            cflags: ["-fPIC"],
        },
        darwin: {
            cflags: ["-Wall"],
        },
    },
}
`,
		expectedBazelTargets: []string{
			`cc_object(
    name = "foo",
    copts = [
        "-fno-addrsig",
        "-I.",
        "-I$(BINDIR)/.",
    ] + select({
        "//build/bazel/platforms/os:android": ["-fPIC"],
        "//build/bazel/platforms/os:darwin": ["-Wall"],
        "//build/bazel/platforms/os:windows": ["-fPIC"],
        "//conditions:default": [],
    }),
    srcs = ["base.cpp"],
)`,
		},
	})
}
