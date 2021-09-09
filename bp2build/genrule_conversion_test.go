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
	"android/soong/genrule"
	"strings"
	"testing"
)

func TestGenruleBp2Build(t *testing.T) {
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

	testCases := []bp2buildTestCase{
		{
			description:                        "genrule with command line variable replacements",
			moduleTypeUnderTest:                "genrule",
			moduleTypeUnderTestFactory:         genrule.GenRuleFactory,
			moduleTypeUnderTestBp2BuildMutator: genrule.GenruleBp2Build,
			blueprint: `genrule {
    name: "foo.tool",
    out: ["foo_tool.out"],
    srcs: ["foo_tool.in"],
    cmd: "cp $(in) $(out)",
    bazel_module: { bp2build_available: true },
}

genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    tools: [":foo.tool"],
    cmd: "$(location :foo.tool) --genDir=$(genDir) arg $(in) $(out)",
    bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{
				`genrule(
    name = "foo",
    cmd = "$(location :foo.tool) --genDir=$(GENDIR) arg $(SRCS) $(OUTS)",
    outs = ["foo.out"],
    srcs = ["foo.in"],
    tools = [":foo.tool"],
)`,
				`genrule(
    name = "foo.tool",
    cmd = "cp $(SRCS) $(OUTS)",
    outs = ["foo_tool.out"],
    srcs = ["foo_tool.in"],
)`,
			},
		},
		{
			description:                        "genrule using $(locations :label)",
			moduleTypeUnderTest:                "genrule",
			moduleTypeUnderTestFactory:         genrule.GenRuleFactory,
			moduleTypeUnderTestBp2BuildMutator: genrule.GenruleBp2Build,
			blueprint: `genrule {
    name: "foo.tools",
    out: ["foo_tool.out", "foo_tool2.out"],
    srcs: ["foo_tool.in"],
    cmd: "cp $(in) $(out)",
    bazel_module: { bp2build_available: true },
}

genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    tools: [":foo.tools"],
    cmd: "$(locations :foo.tools) -s $(out) $(in)",
    bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{`genrule(
    name = "foo",
    cmd = "$(locations :foo.tools) -s $(OUTS) $(SRCS)",
    outs = ["foo.out"],
    srcs = ["foo.in"],
    tools = [":foo.tools"],
)`,
				`genrule(
    name = "foo.tools",
    cmd = "cp $(SRCS) $(OUTS)",
    outs = [
        "foo_tool.out",
        "foo_tool2.out",
    ],
    srcs = ["foo_tool.in"],
)`,
			},
		},
		{
			description:                        "genrule using $(locations //absolute:label)",
			moduleTypeUnderTest:                "genrule",
			moduleTypeUnderTestFactory:         genrule.GenRuleFactory,
			moduleTypeUnderTestBp2BuildMutator: genrule.GenruleBp2Build,
			blueprint: `genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    tool_files: [":foo.tool"],
    cmd: "$(locations :foo.tool) -s $(out) $(in)",
    bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{`genrule(
    name = "foo",
    cmd = "$(locations //other:foo.tool) -s $(OUTS) $(SRCS)",
    outs = ["foo.out"],
    srcs = ["foo.in"],
    tools = ["//other:foo.tool"],
)`,
			},
			filesystem: otherGenruleBp,
		},
		{
			description:                        "genrule srcs using $(locations //absolute:label)",
			moduleTypeUnderTest:                "genrule",
			moduleTypeUnderTestFactory:         genrule.GenRuleFactory,
			moduleTypeUnderTestBp2BuildMutator: genrule.GenruleBp2Build,
			blueprint: `genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: [":other.tool"],
    tool_files: [":foo.tool"],
    cmd: "$(locations :foo.tool) -s $(out) $(location :other.tool)",
    bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{`genrule(
    name = "foo",
    cmd = "$(locations //other:foo.tool) -s $(OUTS) $(location //other:other.tool)",
    outs = ["foo.out"],
    srcs = ["//other:other.tool"],
    tools = ["//other:foo.tool"],
)`,
			},
			filesystem: otherGenruleBp,
		},
		{
			description:                        "genrule using $(location) label should substitute first tool label automatically",
			moduleTypeUnderTest:                "genrule",
			moduleTypeUnderTestFactory:         genrule.GenRuleFactory,
			moduleTypeUnderTestBp2BuildMutator: genrule.GenruleBp2Build,
			blueprint: `genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    tool_files: [":foo.tool", ":other.tool"],
    cmd: "$(location) -s $(out) $(in)",
    bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{`genrule(
    name = "foo",
    cmd = "$(location //other:foo.tool) -s $(OUTS) $(SRCS)",
    outs = ["foo.out"],
    srcs = ["foo.in"],
    tools = [
        "//other:foo.tool",
        "//other:other.tool",
    ],
)`,
			},
			filesystem: otherGenruleBp,
		},
		{
			description:                        "genrule using $(locations) label should substitute first tool label automatically",
			moduleTypeUnderTest:                "genrule",
			moduleTypeUnderTestFactory:         genrule.GenRuleFactory,
			moduleTypeUnderTestBp2BuildMutator: genrule.GenruleBp2Build,
			blueprint: `genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    tools: [":foo.tool", ":other.tool"],
    cmd: "$(locations) -s $(out) $(in)",
    bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{`genrule(
    name = "foo",
    cmd = "$(locations //other:foo.tool) -s $(OUTS) $(SRCS)",
    outs = ["foo.out"],
    srcs = ["foo.in"],
    tools = [
        "//other:foo.tool",
        "//other:other.tool",
    ],
)`,
			},
			filesystem: otherGenruleBp,
		},
		{
			description:                        "genrule without tools or tool_files can convert successfully",
			moduleTypeUnderTest:                "genrule",
			moduleTypeUnderTestFactory:         genrule.GenRuleFactory,
			moduleTypeUnderTestBp2BuildMutator: genrule.GenruleBp2Build,
			blueprint: `genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    cmd: "cp $(in) $(out)",
    bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{`genrule(
    name = "foo",
    cmd = "cp $(SRCS) $(OUTS)",
    outs = ["foo.out"],
    srcs = ["foo.in"],
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
		for f, content := range testCase.filesystem {
			if strings.HasSuffix(f, "Android.bp") {
				toParse = append(toParse, f)
			}
			fs[f] = []byte(content)
		}
		config := android.TestConfig(buildDir, nil, testCase.blueprint, fs)
		ctx := android.NewTestContext(config)
		ctx.RegisterModuleType(testCase.moduleTypeUnderTest, testCase.moduleTypeUnderTestFactory)
		ctx.RegisterBp2BuildMutator(testCase.moduleTypeUnderTest, testCase.moduleTypeUnderTestBp2BuildMutator)
		ctx.RegisterForBazelConversion()

		_, errs := ctx.ParseFileList(dir, toParse)
		if errored(t, testCase, errs) {
			continue
		}
		_, errs = ctx.ResolveDependencies(config)
		if errored(t, testCase, errs) {
			continue
		}

		checkDir := dir
		if testCase.dir != "" {
			checkDir = testCase.dir
		}

		codegenCtx := NewCodegenContext(config, *ctx.Context, Bp2Build)
		bazelTargets := generateBazelTargetsForDir(codegenCtx, checkDir)
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
    bazel_module: { bp2build_available: true },
}
`,
			expectedBazelTarget: `genrule(
    name = "gen",
    cmd = "do-something $(SRCS) $(OUTS)",
    outs = ["out"],
    srcs = ["in1"],
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
    bazel_module: { bp2build_available: true },
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
    bazel_module: { bp2build_available: true },
}
`,
			expectedBazelTarget: `genrule(
    name = "gen",
    cmd = "cp $(SRCS) $(OUTS)",
    outs = ["out"],
    srcs = ["in1"],
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
    bazel_module: { bp2build_available: true },
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

		codegenCtx := NewCodegenContext(config, *ctx.Context, Bp2Build)
		bazelTargets := generateBazelTargetsForDir(codegenCtx, dir)
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
