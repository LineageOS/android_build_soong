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
	"android/soong/android"
	"android/soong/cc"
	"fmt"
	"strings"
	"testing"
)

func TestCcObjectBp2Build(t *testing.T) {
	testCases := []struct {
		description                        string
		moduleTypeUnderTest                string
		moduleTypeUnderTestFactory         android.ModuleFactory
		moduleTypeUnderTestBp2BuildMutator func(android.TopDownMutatorContext)
		blueprint                          string
		expectedBazelTargets               []string
		filesystem                         map[string]string
	}{
		{
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
        "a/b/*.h",
        "a/b/*.c"
    ],
    exclude_srcs: ["a/b/exclude.c"],

    bazel_module: { bp2build_available: true },
}
`,
			expectedBazelTargets: []string{`cc_object(
    name = "foo",
    copts = [
        "-fno-addrsig",
        "-Wno-gcc-compat",
        "-Wall",
        "-Werror",
    ],
    local_include_dirs = [
        "include",
        ".",
    ],
    srcs = [
        "a/b/bar.h",
        "a/b/foo.h",
        "a/b/c.c",
    ],
)`,
			},
		},
		{
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
    bazel_module: { bp2build_available: true },
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
    ],
    local_include_dirs = [
        "include",
        ".",
    ],
    srcs = [
        "a/b/c.c",
    ],
)`,
			},
		},
		{
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

    bazel_module: { bp2build_available: true },
}

cc_object {
    name: "bar",
    srcs: ["x/y/z.c"],

    bazel_module: { bp2build_available: true },
}
`,
			expectedBazelTargets: []string{`cc_object(
    name = "bar",
    copts = [
        "-fno-addrsig",
    ],
    local_include_dirs = [
        ".",
    ],
    srcs = [
        "x/y/z.c",
    ],
)`, `cc_object(
    name = "foo",
    copts = [
        "-fno-addrsig",
    ],
    deps = [
        ":bar",
    ],
    local_include_dirs = [
        ".",
    ],
    srcs = [
        "a/b/c.c",
    ],
)`,
			},
		},
		{
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

    bazel_module: { bp2build_available: true },
}
`,
			expectedBazelTargets: []string{`cc_object(
    name = "foo",
    copts = [
        "-fno-addrsig",
    ],
    srcs = [
        "a/b/c.c",
    ],
)`,
			},
		},
	}

	dir := "."
	for _, testCase := range testCases {
		filesystem := make(map[string][]byte)
		toParse := []string{
			"Android.bp",
		}
		for f, content := range testCase.filesystem {
			if strings.HasSuffix(f, "Android.bp") {
				toParse = append(toParse, f)
			}
			filesystem[f] = []byte(content)
		}
		config := android.TestConfig(buildDir, nil, testCase.blueprint, filesystem)
		ctx := android.NewTestContext(config)
		// Always register cc_defaults module factory
		ctx.RegisterModuleType("cc_defaults", func() android.Module { return cc.DefaultsFactory() })

		ctx.RegisterModuleType(testCase.moduleTypeUnderTest, testCase.moduleTypeUnderTestFactory)
		ctx.RegisterBp2BuildMutator(testCase.moduleTypeUnderTest, testCase.moduleTypeUnderTestBp2BuildMutator)
		ctx.RegisterForBazelConversion()

		_, errs := ctx.ParseFileList(dir, toParse)
		if Errored(t, testCase.description, errs) {
			continue
		}
		_, errs = ctx.ResolveDependencies(config)
		if Errored(t, testCase.description, errs) {
			continue
		}

		codegenCtx := NewCodegenContext(config, *ctx.Context, Bp2Build)
		bazelTargets := generateBazelTargetsForDir(codegenCtx, dir)
		if actualCount, expectedCount := len(bazelTargets), len(testCase.expectedBazelTargets); actualCount != expectedCount {
			fmt.Println(bazelTargets)
			t.Errorf("%s: Expected %d bazel target, got %d", testCase.description, expectedCount, actualCount)
		} else {
			for i, target := range bazelTargets {
				if w, g := testCase.expectedBazelTargets[i], target.content; w != g {
					t.Errorf(
						"%s: Expected generated Bazel target to be '%s', got '%s'",
						testCase.description,
						w,
						g,
					)
				}
			}
		}
	}
}

func TestCcObjectConfigurableAttributesBp2Build(t *testing.T) {
	testCases := []struct {
		description                        string
		moduleTypeUnderTest                string
		moduleTypeUnderTestFactory         android.ModuleFactory
		moduleTypeUnderTestBp2BuildMutator func(android.TopDownMutatorContext)
		blueprint                          string
		expectedBazelTargets               []string
		filesystem                         map[string]string
	}{
		{
			description:                        "cc_object setting cflags for one arch",
			moduleTypeUnderTest:                "cc_object",
			moduleTypeUnderTestFactory:         cc.ObjectFactory,
			moduleTypeUnderTestBp2BuildMutator: cc.ObjectBp2Build,
			blueprint: `cc_object {
    name: "foo",
    arch: {
        x86: {
            cflags: ["-fPIC"],
        },
    },
    bazel_module: { bp2build_available: true },
}
`,
			expectedBazelTargets: []string{
				`cc_object(
    name = "foo",
    copts = [
        "-fno-addrsig",
    ] + select({
        "@bazel_tools//platforms:x86_32": [
            "-fPIC",
        ],
        "//conditions:default": [
        ],
    }),
    local_include_dirs = [
        ".",
    ],
)`,
			},
		},
		{
			description:                        "cc_object setting cflags for 4 architectures",
			moduleTypeUnderTest:                "cc_object",
			moduleTypeUnderTestFactory:         cc.ObjectFactory,
			moduleTypeUnderTestBp2BuildMutator: cc.ObjectBp2Build,
			blueprint: `cc_object {
    name: "foo",
    arch: {
        x86: {
            cflags: ["-fPIC"],
        },
        x86_64: {
            cflags: ["-fPIC"],
        },
        arm: {
            cflags: ["-Wall"],
        },
        arm64: {
            cflags: ["-Wall"],
        },
    },
    bazel_module: { bp2build_available: true },
}
`,
			expectedBazelTargets: []string{
				`cc_object(
    name = "foo",
    copts = [
        "-fno-addrsig",
    ] + select({
        "@bazel_tools//platforms:arm": [
            "-Wall",
        ],
        "@bazel_tools//platforms:aarch64": [
            "-Wall",
        ],
        "@bazel_tools//platforms:x86_32": [
            "-fPIC",
        ],
        "@bazel_tools//platforms:x86_64": [
            "-fPIC",
        ],
        "//conditions:default": [
        ],
    }),
    local_include_dirs = [
        ".",
    ],
)`,
			},
		},
	}

	dir := "."
	for _, testCase := range testCases {
		filesystem := make(map[string][]byte)
		toParse := []string{
			"Android.bp",
		}
		config := android.TestConfig(buildDir, nil, testCase.blueprint, filesystem)
		ctx := android.NewTestContext(config)
		// Always register cc_defaults module factory
		ctx.RegisterModuleType("cc_defaults", func() android.Module { return cc.DefaultsFactory() })

		ctx.RegisterModuleType(testCase.moduleTypeUnderTest, testCase.moduleTypeUnderTestFactory)
		ctx.RegisterBp2BuildMutator(testCase.moduleTypeUnderTest, testCase.moduleTypeUnderTestBp2BuildMutator)
		ctx.RegisterForBazelConversion()

		_, errs := ctx.ParseFileList(dir, toParse)
		if Errored(t, testCase.description, errs) {
			continue
		}
		_, errs = ctx.ResolveDependencies(config)
		if Errored(t, testCase.description, errs) {
			continue
		}

		codegenCtx := NewCodegenContext(config, *ctx.Context, Bp2Build)
		bazelTargets := generateBazelTargetsForDir(codegenCtx, dir)
		if actualCount, expectedCount := len(bazelTargets), len(testCase.expectedBazelTargets); actualCount != expectedCount {
			fmt.Println(bazelTargets)
			t.Errorf("%s: Expected %d bazel target, got %d", testCase.description, expectedCount, actualCount)
		} else {
			for i, target := range bazelTargets {
				if w, g := testCase.expectedBazelTargets[i], target.content; w != g {
					t.Errorf(
						"%s: Expected generated Bazel target to be '%s', got '%s'",
						testCase.description,
						w,
						g,
					)
				}
			}
		}
	}
}
