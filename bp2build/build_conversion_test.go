// Copyright 2020 Google Inc. All rights reserved.
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
	"android/soong/genrule"
	"strings"
	"testing"
)

func TestGenerateSoongModuleTargets(t *testing.T) {
	testCases := []struct {
		bp                  string
		expectedBazelTarget string
	}{
		{
			bp: `custom {
	name: "foo",
}
		`,
			expectedBazelTarget: `soong_module(
    name = "foo",
    soong_module_name = "foo",
    soong_module_type = "custom",
    soong_module_variant = "",
    soong_module_deps = [
    ],
)`,
		},
		{
			bp: `custom {
	name: "foo",
	ramdisk: true,
}
		`,
			expectedBazelTarget: `soong_module(
    name = "foo",
    soong_module_name = "foo",
    soong_module_type = "custom",
    soong_module_variant = "",
    soong_module_deps = [
    ],
    ramdisk = True,
)`,
		},
		{
			bp: `custom {
	name: "foo",
	owner: "a_string_with\"quotes\"_and_\\backslashes\\\\",
}
		`,
			expectedBazelTarget: `soong_module(
    name = "foo",
    soong_module_name = "foo",
    soong_module_type = "custom",
    soong_module_variant = "",
    soong_module_deps = [
    ],
    owner = "a_string_with\"quotes\"_and_\\backslashes\\\\",
)`,
		},
		{
			bp: `custom {
	name: "foo",
	required: ["bar"],
}
		`,
			expectedBazelTarget: `soong_module(
    name = "foo",
    soong_module_name = "foo",
    soong_module_type = "custom",
    soong_module_variant = "",
    soong_module_deps = [
    ],
    required = [
        "bar",
    ],
)`,
		},
		{
			bp: `custom {
	name: "foo",
	target_required: ["qux", "bazqux"],
}
		`,
			expectedBazelTarget: `soong_module(
    name = "foo",
    soong_module_name = "foo",
    soong_module_type = "custom",
    soong_module_variant = "",
    soong_module_deps = [
    ],
    target_required = [
        "qux",
        "bazqux",
    ],
)`,
		},
		{
			bp: `custom {
	name: "foo",
	dist: {
		targets: ["goal_foo"],
		tag: ".foo",
	},
	dists: [
		{
			targets: ["goal_bar"],
			tag: ".bar",
		},
	],
}
		`,
			expectedBazelTarget: `soong_module(
    name = "foo",
    soong_module_name = "foo",
    soong_module_type = "custom",
    soong_module_variant = "",
    soong_module_deps = [
    ],
    dist = {
        "tag": ".foo",
        "targets": [
            "goal_foo",
        ],
    },
    dists = [
        {
            "tag": ".bar",
            "targets": [
                "goal_bar",
            ],
        },
    ],
)`,
		},
		{
			bp: `custom {
	name: "foo",
	required: ["bar"],
	target_required: ["qux", "bazqux"],
	ramdisk: true,
	owner: "custom_owner",
	dists: [
		{
			tag: ".tag",
			targets: ["my_goal"],
		},
	],
}
		`,
			expectedBazelTarget: `soong_module(
    name = "foo",
    soong_module_name = "foo",
    soong_module_type = "custom",
    soong_module_variant = "",
    soong_module_deps = [
    ],
    dists = [
        {
            "tag": ".tag",
            "targets": [
                "my_goal",
            ],
        },
    ],
    owner = "custom_owner",
    ramdisk = True,
    required = [
        "bar",
    ],
    target_required = [
        "qux",
        "bazqux",
    ],
)`,
		},
	}

	dir := "."
	for _, testCase := range testCases {
		config := android.TestConfig(buildDir, nil, testCase.bp, nil)
		ctx := android.NewTestContext(config)
		ctx.RegisterModuleType("custom", customModuleFactory)
		ctx.Register()

		_, errs := ctx.ParseFileList(dir, []string{"Android.bp"})
		android.FailIfErrored(t, errs)
		_, errs = ctx.PrepareBuildActions(config)
		android.FailIfErrored(t, errs)

		bazelTargets := GenerateBazelTargets(ctx.Context.Context, QueryView)[dir]
		if actualCount, expectedCount := len(bazelTargets), 1; actualCount != expectedCount {
			t.Fatalf("Expected %d bazel target, got %d", expectedCount, actualCount)
		}

		actualBazelTarget := bazelTargets[0]
		if actualBazelTarget.content != testCase.expectedBazelTarget {
			t.Errorf(
				"Expected generated Bazel target to be '%s', got '%s'",
				testCase.expectedBazelTarget,
				actualBazelTarget.content,
			)
		}
	}
}

func TestGenerateBazelTargetModules(t *testing.T) {
	testCases := []struct {
		bp                  string
		expectedBazelTarget string
	}{
		{
			bp: `custom {
	name: "foo",
    string_list_prop: ["a", "b"],
    string_prop: "a",
}`,
			expectedBazelTarget: `custom(
    name = "foo",
    string_list_prop = [
        "a",
        "b",
    ],
    string_prop = "a",
)`,
		},
	}

	dir := "."
	for _, testCase := range testCases {
		config := android.TestConfig(buildDir, nil, testCase.bp, nil)
		ctx := android.NewTestContext(config)
		ctx.RegisterModuleType("custom", customModuleFactory)
		ctx.RegisterBp2BuildMutator("custom", customBp2BuildMutator)
		ctx.RegisterForBazelConversion()

		_, errs := ctx.ParseFileList(dir, []string{"Android.bp"})
		if Errored(t, "", errs) {
			continue
		}
		_, errs = ctx.ResolveDependencies(config)
		if Errored(t, "", errs) {
			continue
		}

		bazelTargets := GenerateBazelTargets(ctx.Context.Context, Bp2Build)[dir]
		if actualCount, expectedCount := len(bazelTargets), 1; actualCount != expectedCount {
			t.Errorf("Expected %d bazel target, got %d", expectedCount, actualCount)
		} else {
			actualBazelTarget := bazelTargets[0]
			if actualBazelTarget.content != testCase.expectedBazelTarget {
				t.Errorf(
					"Expected generated Bazel target to be '%s', got '%s'",
					testCase.expectedBazelTarget,
					actualBazelTarget.content,
				)
			}
		}
	}
}

func TestLoadStatements(t *testing.T) {
	testCases := []struct {
		bazelTargets           BazelTargets
		expectedLoadStatements string
	}{
		{
			bazelTargets: BazelTargets{
				BazelTarget{
					name:            "foo",
					ruleClass:       "cc_library",
					bzlLoadLocation: "//build/bazel/rules:cc.bzl",
				},
			},
			expectedLoadStatements: `load("//build/bazel/rules:cc.bzl", "cc_library")`,
		},
		{
			bazelTargets: BazelTargets{
				BazelTarget{
					name:            "foo",
					ruleClass:       "cc_library",
					bzlLoadLocation: "//build/bazel/rules:cc.bzl",
				},
				BazelTarget{
					name:            "bar",
					ruleClass:       "cc_library",
					bzlLoadLocation: "//build/bazel/rules:cc.bzl",
				},
			},
			expectedLoadStatements: `load("//build/bazel/rules:cc.bzl", "cc_library")`,
		},
		{
			bazelTargets: BazelTargets{
				BazelTarget{
					name:            "foo",
					ruleClass:       "cc_library",
					bzlLoadLocation: "//build/bazel/rules:cc.bzl",
				},
				BazelTarget{
					name:            "bar",
					ruleClass:       "cc_binary",
					bzlLoadLocation: "//build/bazel/rules:cc.bzl",
				},
			},
			expectedLoadStatements: `load("//build/bazel/rules:cc.bzl", "cc_binary", "cc_library")`,
		},
		{
			bazelTargets: BazelTargets{
				BazelTarget{
					name:            "foo",
					ruleClass:       "cc_library",
					bzlLoadLocation: "//build/bazel/rules:cc.bzl",
				},
				BazelTarget{
					name:            "bar",
					ruleClass:       "cc_binary",
					bzlLoadLocation: "//build/bazel/rules:cc.bzl",
				},
				BazelTarget{
					name:            "baz",
					ruleClass:       "java_binary",
					bzlLoadLocation: "//build/bazel/rules:java.bzl",
				},
			},
			expectedLoadStatements: `load("//build/bazel/rules:cc.bzl", "cc_binary", "cc_library")
load("//build/bazel/rules:java.bzl", "java_binary")`,
		},
		{
			bazelTargets: BazelTargets{
				BazelTarget{
					name:            "foo",
					ruleClass:       "cc_binary",
					bzlLoadLocation: "//build/bazel/rules:cc.bzl",
				},
				BazelTarget{
					name:            "bar",
					ruleClass:       "java_binary",
					bzlLoadLocation: "//build/bazel/rules:java.bzl",
				},
				BazelTarget{
					name:      "baz",
					ruleClass: "genrule",
					// Note: no bzlLoadLocation for native rules
				},
			},
			expectedLoadStatements: `load("//build/bazel/rules:cc.bzl", "cc_binary")
load("//build/bazel/rules:java.bzl", "java_binary")`,
		},
	}

	for _, testCase := range testCases {
		actual := testCase.bazelTargets.LoadStatements()
		expected := testCase.expectedLoadStatements
		if actual != expected {
			t.Fatalf("Expected load statements to be %s, got %s", expected, actual)
		}
	}

}

func TestGenerateBazelTargetModules_OneToMany_LoadedFromStarlark(t *testing.T) {
	testCases := []struct {
		bp                       string
		expectedBazelTarget      string
		expectedBazelTargetCount int
		expectedLoadStatements   string
	}{
		{
			bp: `custom {
    name: "bar",
}`,
			expectedBazelTarget: `my_library(
    name = "bar",
)

my_proto_library(
    name = "bar_my_proto_library_deps",
)

proto_library(
    name = "bar_proto_library_deps",
)`,
			expectedBazelTargetCount: 3,
			expectedLoadStatements: `load("//build/bazel/rules:proto.bzl", "my_proto_library", "proto_library")
load("//build/bazel/rules:rules.bzl", "my_library")`,
		},
	}

	dir := "."
	for _, testCase := range testCases {
		config := android.TestConfig(buildDir, nil, testCase.bp, nil)
		ctx := android.NewTestContext(config)
		ctx.RegisterModuleType("custom", customModuleFactory)
		ctx.RegisterBp2BuildMutator("custom_starlark", customBp2BuildMutatorFromStarlark)
		ctx.RegisterForBazelConversion()

		_, errs := ctx.ParseFileList(dir, []string{"Android.bp"})
		android.FailIfErrored(t, errs)
		_, errs = ctx.ResolveDependencies(config)
		android.FailIfErrored(t, errs)

		bazelTargets := GenerateBazelTargets(ctx.Context.Context, Bp2Build)[dir]
		if actualCount := len(bazelTargets); actualCount != testCase.expectedBazelTargetCount {
			t.Fatalf("Expected %d bazel target, got %d", testCase.expectedBazelTargetCount, actualCount)
		}

		actualBazelTargets := bazelTargets.String()
		if actualBazelTargets != testCase.expectedBazelTarget {
			t.Errorf(
				"Expected generated Bazel target to be '%s', got '%s'",
				testCase.expectedBazelTarget,
				actualBazelTargets,
			)
		}

		actualLoadStatements := bazelTargets.LoadStatements()
		if actualLoadStatements != testCase.expectedLoadStatements {
			t.Errorf(
				"Expected generated load statements to be '%s', got '%s'",
				testCase.expectedLoadStatements,
				actualLoadStatements,
			)
		}
	}
}

func TestModuleTypeBp2Build(t *testing.T) {
	otherGenruleBp := map[string]string{
		"other/Android.bp": `genrule {
    name: "foo.tool",
    out: ["foo_tool.out"],
    srcs: ["foo_tool.in"],
    cmd: "cp $(in) $(out)",
}
genrule {
    name: "other.tool",
    out: ["other_tool.out"],
    srcs: ["other_tool.in"],
    cmd: "cp $(in) $(out)",
}`,
	}

	testCases := []struct {
		description                        string
		moduleTypeUnderTest                string
		moduleTypeUnderTestFactory         android.ModuleFactory
		moduleTypeUnderTestBp2BuildMutator func(android.TopDownMutatorContext)
		preArchMutators                    []android.RegisterMutatorFunc
		depsMutators                       []android.RegisterMutatorFunc
		bp                                 string
		expectedBazelTargets               []string
		fs                                 map[string]string
		dir                                string
	}{
		{
			description:                        "filegroup with no srcs",
			moduleTypeUnderTest:                "filegroup",
			moduleTypeUnderTestFactory:         android.FileGroupFactory,
			moduleTypeUnderTestBp2BuildMutator: android.FilegroupBp2Build,
			bp: `filegroup {
    name: "fg_foo",
    srcs: [],
}`,
			expectedBazelTargets: []string{
				`filegroup(
    name = "fg_foo",
    srcs = [
    ],
)`,
			},
		},
		{
			description:                        "filegroup with srcs",
			moduleTypeUnderTest:                "filegroup",
			moduleTypeUnderTestFactory:         android.FileGroupFactory,
			moduleTypeUnderTestBp2BuildMutator: android.FilegroupBp2Build,
			bp: `filegroup {
    name: "fg_foo",
    srcs: ["a", "b"],
}`,
			expectedBazelTargets: []string{`filegroup(
    name = "fg_foo",
    srcs = [
        "a",
        "b",
    ],
)`,
			},
		},
		{
			description:                        "filegroup with excludes srcs",
			moduleTypeUnderTest:                "filegroup",
			moduleTypeUnderTestFactory:         android.FileGroupFactory,
			moduleTypeUnderTestBp2BuildMutator: android.FilegroupBp2Build,
			bp: `filegroup {
    name: "fg_foo",
    srcs: ["a", "b"],
    exclude_srcs: ["a"],
}`,
			expectedBazelTargets: []string{`filegroup(
    name = "fg_foo",
    srcs = [
        "b",
    ],
)`,
			},
		},
		{
			description:                        "filegroup with glob",
			moduleTypeUnderTest:                "filegroup",
			moduleTypeUnderTestFactory:         android.FileGroupFactory,
			moduleTypeUnderTestBp2BuildMutator: android.FilegroupBp2Build,
			bp: `filegroup {
    name: "foo",
    srcs: ["**/*.txt"],
}`,
			expectedBazelTargets: []string{`filegroup(
    name = "foo",
    srcs = [
        "other/a.txt",
        "other/b.txt",
        "other/subdir/a.txt",
    ],
)`,
			},
			fs: map[string]string{
				"other/a.txt":        "",
				"other/b.txt":        "",
				"other/subdir/a.txt": "",
				"other/file":         "",
			},
		},
		{
			description:                        "filegroup with glob in subdir",
			moduleTypeUnderTest:                "filegroup",
			moduleTypeUnderTestFactory:         android.FileGroupFactory,
			moduleTypeUnderTestBp2BuildMutator: android.FilegroupBp2Build,
			bp: `filegroup {
    name: "foo",
    srcs: ["a.txt"],
}`,
			dir: "other",
			expectedBazelTargets: []string{`filegroup(
    name = "fg_foo",
    srcs = [
        "a.txt",
        "b.txt",
        "subdir/a.txt",
    ],
)`,
			},
			fs: map[string]string{
				"other/Android.bp": `filegroup {
    name: "fg_foo",
    srcs: ["**/*.txt"],
}`,
				"other/a.txt":        "",
				"other/b.txt":        "",
				"other/subdir/a.txt": "",
				"other/file":         "",
			},
		},
		{
			description:                        "depends_on_other_dir_module",
			moduleTypeUnderTest:                "filegroup",
			moduleTypeUnderTestFactory:         android.FileGroupFactory,
			moduleTypeUnderTestBp2BuildMutator: android.FilegroupBp2Build,
			bp: `filegroup {
    name: "foobar",
    srcs: [
      ":foo",
        "c",
    ],
}`,
			expectedBazelTargets: []string{`filegroup(
    name = "foobar",
    srcs = [
        "//other:foo",
        "c",
    ],
)`,
			},
			fs: map[string]string{
				"other/Android.bp": `filegroup {
    name: "foo",
    srcs: ["a", "b"],
}`,
			},
		},
		{
			description:                        "genrule with command line variable replacements",
			moduleTypeUnderTest:                "genrule",
			moduleTypeUnderTestFactory:         genrule.GenRuleFactory,
			moduleTypeUnderTestBp2BuildMutator: genrule.GenruleBp2Build,
			depsMutators:                       []android.RegisterMutatorFunc{genrule.RegisterGenruleBp2BuildDeps},
			bp: `genrule {
    name: "foo.tool",
    out: ["foo_tool.out"],
    srcs: ["foo_tool.in"],
    cmd: "cp $(in) $(out)",
}

genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    tools: [":foo.tool"],
    cmd: "$(location :foo.tool) --genDir=$(genDir) arg $(in) $(out)",
}`,
			expectedBazelTargets: []string{
				`genrule(
    name = "foo",
    cmd = "$(location :foo.tool) --genDir=$(GENDIR) arg $(SRCS) $(OUTS)",
    outs = [
        "foo.out",
    ],
    srcs = [
        "foo.in",
    ],
    tools = [
        ":foo.tool",
    ],
)`,
				`genrule(
    name = "foo.tool",
    cmd = "cp $(SRCS) $(OUTS)",
    outs = [
        "foo_tool.out",
    ],
    srcs = [
        "foo_tool.in",
    ],
)`,
			},
		},
		{
			description:                        "genrule using $(locations :label)",
			moduleTypeUnderTest:                "genrule",
			moduleTypeUnderTestFactory:         genrule.GenRuleFactory,
			moduleTypeUnderTestBp2BuildMutator: genrule.GenruleBp2Build,
			depsMutators:                       []android.RegisterMutatorFunc{genrule.RegisterGenruleBp2BuildDeps},
			bp: `genrule {
    name: "foo.tools",
    out: ["foo_tool.out", "foo_tool2.out"],
    srcs: ["foo_tool.in"],
    cmd: "cp $(in) $(out)",
   }

genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    tools: [":foo.tools"],
    cmd: "$(locations :foo.tools) -s $(out) $(in)",
}`,
			expectedBazelTargets: []string{`genrule(
    name = "foo",
    cmd = "$(locations :foo.tools) -s $(OUTS) $(SRCS)",
    outs = [
        "foo.out",
    ],
    srcs = [
        "foo.in",
    ],
    tools = [
        ":foo.tools",
    ],
)`,
				`genrule(
    name = "foo.tools",
    cmd = "cp $(SRCS) $(OUTS)",
    outs = [
        "foo_tool.out",
        "foo_tool2.out",
    ],
    srcs = [
        "foo_tool.in",
    ],
)`,
			},
		},
		{
			description:                        "genrule using $(locations //absolute:label)",
			moduleTypeUnderTest:                "genrule",
			moduleTypeUnderTestFactory:         genrule.GenRuleFactory,
			moduleTypeUnderTestBp2BuildMutator: genrule.GenruleBp2Build,
			depsMutators:                       []android.RegisterMutatorFunc{genrule.RegisterGenruleBp2BuildDeps},
			bp: `genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    tool_files: [":foo.tool"],
    cmd: "$(locations :foo.tool) -s $(out) $(in)",
}`,
			expectedBazelTargets: []string{`genrule(
    name = "foo",
    cmd = "$(locations //other:foo.tool) -s $(OUTS) $(SRCS)",
    outs = [
        "foo.out",
    ],
    srcs = [
        "foo.in",
    ],
    tools = [
        "//other:foo.tool",
    ],
)`,
			},
			fs: otherGenruleBp,
		},
		{
			description:                        "genrule srcs using $(locations //absolute:label)",
			moduleTypeUnderTest:                "genrule",
			moduleTypeUnderTestFactory:         genrule.GenRuleFactory,
			moduleTypeUnderTestBp2BuildMutator: genrule.GenruleBp2Build,
			depsMutators:                       []android.RegisterMutatorFunc{genrule.RegisterGenruleBp2BuildDeps},
			bp: `genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: [":other.tool"],
    tool_files: [":foo.tool"],
    cmd: "$(locations :foo.tool) -s $(out) $(location :other.tool)",
}`,
			expectedBazelTargets: []string{`genrule(
    name = "foo",
    cmd = "$(locations //other:foo.tool) -s $(OUTS) $(location //other:other.tool)",
    outs = [
        "foo.out",
    ],
    srcs = [
        "//other:other.tool",
    ],
    tools = [
        "//other:foo.tool",
    ],
)`,
			},
			fs: otherGenruleBp,
		},
		{
			description:                        "genrule using $(location) label should substitute first tool label automatically",
			moduleTypeUnderTest:                "genrule",
			moduleTypeUnderTestFactory:         genrule.GenRuleFactory,
			moduleTypeUnderTestBp2BuildMutator: genrule.GenruleBp2Build,
			depsMutators:                       []android.RegisterMutatorFunc{genrule.RegisterGenruleBp2BuildDeps},
			bp: `genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    tool_files: [":foo.tool", ":other.tool"],
    cmd: "$(location) -s $(out) $(in)",
}`,
			expectedBazelTargets: []string{`genrule(
    name = "foo",
    cmd = "$(location //other:foo.tool) -s $(OUTS) $(SRCS)",
    outs = [
        "foo.out",
    ],
    srcs = [
        "foo.in",
    ],
    tools = [
        "//other:foo.tool",
        "//other:other.tool",
    ],
)`,
			},
			fs: otherGenruleBp,
		},
		{
			description:                        "genrule using $(locations) label should substitute first tool label automatically",
			moduleTypeUnderTest:                "genrule",
			moduleTypeUnderTestFactory:         genrule.GenRuleFactory,
			moduleTypeUnderTestBp2BuildMutator: genrule.GenruleBp2Build,
			depsMutators:                       []android.RegisterMutatorFunc{genrule.RegisterGenruleBp2BuildDeps},
			bp: `genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    tools: [":foo.tool", ":other.tool"],
    cmd: "$(locations) -s $(out) $(in)",
}`,
			expectedBazelTargets: []string{`genrule(
    name = "foo",
    cmd = "$(locations //other:foo.tool) -s $(OUTS) $(SRCS)",
    outs = [
        "foo.out",
    ],
    srcs = [
        "foo.in",
    ],
    tools = [
        "//other:foo.tool",
        "//other:other.tool",
    ],
)`,
			},
			fs: otherGenruleBp,
		},
		{
			description:                        "genrule without tools or tool_files can convert successfully",
			moduleTypeUnderTest:                "genrule",
			moduleTypeUnderTestFactory:         genrule.GenRuleFactory,
			moduleTypeUnderTestBp2BuildMutator: genrule.GenruleBp2Build,
			depsMutators:                       []android.RegisterMutatorFunc{genrule.RegisterGenruleBp2BuildDeps},
			bp: `genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    cmd: "cp $(in) $(out)",
}`,
			expectedBazelTargets: []string{`genrule(
    name = "foo",
    cmd = "cp $(SRCS) $(OUTS)",
    outs = [
        "foo.out",
    ],
    srcs = [
        "foo.in",
    ],
)`,
			},
		},
	}

	dir := "."
	for _, testCase := range testCases {
		fs := make(map[string][]byte)
		toParse := []string{
			"Android.bp",
		}
		for f, content := range testCase.fs {
			if strings.HasSuffix(f, "Android.bp") {
				toParse = append(toParse, f)
			}
			fs[f] = []byte(content)
		}
		config := android.TestConfig(buildDir, nil, testCase.bp, fs)
		ctx := android.NewTestContext(config)
		ctx.RegisterModuleType(testCase.moduleTypeUnderTest, testCase.moduleTypeUnderTestFactory)
		for _, m := range testCase.depsMutators {
			ctx.DepsBp2BuildMutators(m)
		}
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

		checkDir := dir
		if testCase.dir != "" {
			checkDir = testCase.dir
		}
		bazelTargets := GenerateBazelTargets(ctx.Context.Context, Bp2Build)[checkDir]
		if actualCount, expectedCount := len(bazelTargets), len(testCase.expectedBazelTargets); actualCount != expectedCount {
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

func Errored(t *testing.T, desc string, errs []error) bool {
	t.Helper()
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("%s: %s", desc, err)
		}
		return true
	}
	return false
}

type bp2buildMutator = func(android.TopDownMutatorContext)

func TestBp2BuildInlinesDefaults(t *testing.T) {
	testCases := []struct {
		moduleTypesUnderTest      map[string]android.ModuleFactory
		bp2buildMutatorsUnderTest map[string]bp2buildMutator
		bp                        string
		expectedBazelTarget       string
		description               string
	}{
		{
			moduleTypesUnderTest: map[string]android.ModuleFactory{
				"genrule":          genrule.GenRuleFactory,
				"genrule_defaults": func() android.Module { return genrule.DefaultsFactory() },
			},
			bp2buildMutatorsUnderTest: map[string]bp2buildMutator{
				"genrule": genrule.GenruleBp2Build,
			},
			bp: `genrule_defaults {
    name: "gen_defaults",
    cmd: "do-something $(in) $(out)",
}
genrule {
    name: "gen",
    out: ["out"],
    srcs: ["in1"],
    defaults: ["gen_defaults"],
}
`,
			expectedBazelTarget: `genrule(
    name = "gen",
    cmd = "do-something $(SRCS) $(OUTS)",
    outs = [
        "out",
    ],
    srcs = [
        "in1",
    ],
)`,
			description: "genrule applies properties from a genrule_defaults dependency if not specified",
		},
		{
			moduleTypesUnderTest: map[string]android.ModuleFactory{
				"genrule":          genrule.GenRuleFactory,
				"genrule_defaults": func() android.Module { return genrule.DefaultsFactory() },
			},
			bp2buildMutatorsUnderTest: map[string]bp2buildMutator{
				"genrule": genrule.GenruleBp2Build,
			},
			bp: `genrule_defaults {
    name: "gen_defaults",
    out: ["out-from-defaults"],
    srcs: ["in-from-defaults"],
    cmd: "cmd-from-defaults",
}
genrule {
    name: "gen",
    out: ["out"],
    srcs: ["in1"],
    defaults: ["gen_defaults"],
    cmd: "do-something $(in) $(out)",
}
`,
			expectedBazelTarget: `genrule(
    name = "gen",
    cmd = "do-something $(SRCS) $(OUTS)",
    outs = [
        "out-from-defaults",
        "out",
    ],
    srcs = [
        "in-from-defaults",
        "in1",
    ],
)`,
			description: "genrule does merges properties from a genrule_defaults dependency, latest-first",
		},
		{
			moduleTypesUnderTest: map[string]android.ModuleFactory{
				"genrule":          genrule.GenRuleFactory,
				"genrule_defaults": func() android.Module { return genrule.DefaultsFactory() },
			},
			bp2buildMutatorsUnderTest: map[string]bp2buildMutator{
				"genrule": genrule.GenruleBp2Build,
			},
			bp: `genrule_defaults {
    name: "gen_defaults1",
    cmd: "cp $(in) $(out)",
}

genrule_defaults {
    name: "gen_defaults2",
    srcs: ["in1"],
}

genrule {
    name: "gen",
    out: ["out"],
    defaults: ["gen_defaults1", "gen_defaults2"],
}
`,
			expectedBazelTarget: `genrule(
    name = "gen",
    cmd = "cp $(SRCS) $(OUTS)",
    outs = [
        "out",
    ],
    srcs = [
        "in1",
    ],
)`,
			description: "genrule applies properties from list of genrule_defaults",
		},
		{
			moduleTypesUnderTest: map[string]android.ModuleFactory{
				"genrule":          genrule.GenRuleFactory,
				"genrule_defaults": func() android.Module { return genrule.DefaultsFactory() },
			},
			bp2buildMutatorsUnderTest: map[string]bp2buildMutator{
				"genrule": genrule.GenruleBp2Build,
			},
			bp: `genrule_defaults {
    name: "gen_defaults1",
    defaults: ["gen_defaults2"],
    cmd: "cmd1 $(in) $(out)", // overrides gen_defaults2's cmd property value.
}

genrule_defaults {
    name: "gen_defaults2",
    defaults: ["gen_defaults3"],
    cmd: "cmd2 $(in) $(out)",
    out: ["out-from-2"],
    srcs: ["in1"],
}

genrule_defaults {
    name: "gen_defaults3",
    out: ["out-from-3"],
    srcs: ["srcs-from-3"],
}

genrule {
    name: "gen",
    out: ["out"],
    defaults: ["gen_defaults1"],
}
`,
			expectedBazelTarget: `genrule(
    name = "gen",
    cmd = "cmd1 $(SRCS) $(OUTS)",
    outs = [
        "out-from-3",
        "out-from-2",
        "out",
    ],
    srcs = [
        "srcs-from-3",
        "in1",
    ],
)`,
			description: "genrule applies properties from genrule_defaults transitively",
		},
	}

	dir := "."
	for _, testCase := range testCases {
		config := android.TestConfig(buildDir, nil, testCase.bp, nil)
		ctx := android.NewTestContext(config)
		for m, factory := range testCase.moduleTypesUnderTest {
			ctx.RegisterModuleType(m, factory)
		}
		for mutator, f := range testCase.bp2buildMutatorsUnderTest {
			ctx.RegisterBp2BuildMutator(mutator, f)
		}
		ctx.RegisterForBazelConversion()

		_, errs := ctx.ParseFileList(dir, []string{"Android.bp"})
		android.FailIfErrored(t, errs)
		_, errs = ctx.ResolveDependencies(config)
		android.FailIfErrored(t, errs)

		bazelTargets := GenerateBazelTargets(ctx.Context.Context, Bp2Build)[dir]
		if actualCount := len(bazelTargets); actualCount != 1 {
			t.Fatalf("%s: Expected 1 bazel target, got %d", testCase.description, actualCount)
		}

		actualBazelTarget := bazelTargets[0]
		if actualBazelTarget.content != testCase.expectedBazelTarget {
			t.Errorf(
				"%s: Expected generated Bazel target to be '%s', got '%s'",
				testCase.description,
				testCase.expectedBazelTarget,
				actualBazelTarget.content,
			)
		}
	}
}
