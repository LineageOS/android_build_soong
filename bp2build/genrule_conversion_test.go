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
	"testing"
)

func registerGenruleModuleTypes(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("genrule_defaults", func() android.Module { return genrule.DefaultsFactory() })
}

func runGenruleTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	(&tc).moduleTypeUnderTest = "genrule"
	(&tc).moduleTypeUnderTestFactory = genrule.GenRuleFactory
	runBp2BuildTestCase(t, registerGenruleModuleTypes, tc)
}

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
			description: "genrule with command line variable replacements",
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
				makeBazelTarget("genrule", "foo", attrNameToString{
					"cmd":   `"$(location :foo.tool) --genDir=$(GENDIR) arg $(SRCS) $(OUTS)"`,
					"outs":  `["foo.out"]`,
					"srcs":  `["foo.in"]`,
					"tools": `[":foo.tool"]`,
				}),
				makeBazelTarget("genrule", "foo.tool", attrNameToString{
					"cmd":  `"cp $(SRCS) $(OUTS)"`,
					"outs": `["foo_tool.out"]`,
					"srcs": `["foo_tool.in"]`,
				}),
			},
		},
		{
			description: "genrule using $(locations :label)",
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
			expectedBazelTargets: []string{
				makeBazelTarget("genrule", "foo", attrNameToString{
					"cmd":   `"$(locations :foo.tools) -s $(OUTS) $(SRCS)"`,
					"outs":  `["foo.out"]`,
					"srcs":  `["foo.in"]`,
					"tools": `[":foo.tools"]`,
				}),
				makeBazelTarget("genrule", "foo.tools", attrNameToString{
					"cmd": `"cp $(SRCS) $(OUTS)"`,
					"outs": `[
        "foo_tool.out",
        "foo_tool2.out",
    ]`,
					"srcs": `["foo_tool.in"]`,
				}),
			},
		},
		{
			description: "genrule using $(locations //absolute:label)",
			blueprint: `genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    tool_files: [":foo.tool"],
    cmd: "$(locations :foo.tool) -s $(out) $(in)",
    bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{
				makeBazelTarget("genrule", "foo", attrNameToString{
					"cmd":   `"$(locations //other:foo.tool) -s $(OUTS) $(SRCS)"`,
					"outs":  `["foo.out"]`,
					"srcs":  `["foo.in"]`,
					"tools": `["//other:foo.tool"]`,
				}),
			},
			filesystem: otherGenruleBp,
		},
		{
			description: "genrule srcs using $(locations //absolute:label)",
			blueprint: `genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: [":other.tool"],
    tool_files: [":foo.tool"],
    cmd: "$(locations :foo.tool) -s $(out) $(location :other.tool)",
    bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{
				makeBazelTarget("genrule", "foo", attrNameToString{
					"cmd":   `"$(locations //other:foo.tool) -s $(OUTS) $(location //other:other.tool)"`,
					"outs":  `["foo.out"]`,
					"srcs":  `["//other:other.tool"]`,
					"tools": `["//other:foo.tool"]`,
				}),
			},
			filesystem: otherGenruleBp,
		},
		{
			description: "genrule using $(location) label should substitute first tool label automatically",
			blueprint: `genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    tool_files: [":foo.tool", ":other.tool"],
    cmd: "$(location) -s $(out) $(in)",
    bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{
				makeBazelTarget("genrule", "foo", attrNameToString{
					"cmd":  `"$(location //other:foo.tool) -s $(OUTS) $(SRCS)"`,
					"outs": `["foo.out"]`,
					"srcs": `["foo.in"]`,
					"tools": `[
        "//other:foo.tool",
        "//other:other.tool",
    ]`,
				}),
			},
			filesystem: otherGenruleBp,
		},
		{
			description: "genrule using $(locations) label should substitute first tool label automatically",
			blueprint: `genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    tools: [":foo.tool", ":other.tool"],
    cmd: "$(locations) -s $(out) $(in)",
    bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{
				makeBazelTarget("genrule", "foo", attrNameToString{
					"cmd":  `"$(locations //other:foo.tool) -s $(OUTS) $(SRCS)"`,
					"outs": `["foo.out"]`,
					"srcs": `["foo.in"]`,
					"tools": `[
        "//other:foo.tool",
        "//other:other.tool",
    ]`,
				}),
			},
			filesystem: otherGenruleBp,
		},
		{
			description: "genrule without tools or tool_files can convert successfully",
			blueprint: `genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    cmd: "cp $(in) $(out)",
    bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{
				makeBazelTarget("genrule", "foo", attrNameToString{
					"cmd":  `"cp $(SRCS) $(OUTS)"`,
					"outs": `["foo.out"]`,
					"srcs": `["foo.in"]`,
				}),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			runGenruleTestCase(t, testCase)
		})
	}
}

func TestBp2BuildInlinesDefaults(t *testing.T) {
	testCases := []bp2buildTestCase{
		{
			description: "genrule applies properties from a genrule_defaults dependency if not specified",
			blueprint: `genrule_defaults {
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
			expectedBazelTargets: []string{
				makeBazelTarget("genrule", "gen", attrNameToString{
					"cmd":  `"do-something $(SRCS) $(OUTS)"`,
					"outs": `["out"]`,
					"srcs": `["in1"]`,
				}),
			},
		},
		{
			description: "genrule does merges properties from a genrule_defaults dependency, latest-first",
			blueprint: `genrule_defaults {
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
			expectedBazelTargets: []string{
				makeBazelTarget("genrule", "gen", attrNameToString{
					"cmd": `"do-something $(SRCS) $(OUTS)"`,
					"outs": `[
        "out-from-defaults",
        "out",
    ]`,
					"srcs": `[
        "in-from-defaults",
        "in1",
    ]`,
				}),
			},
		},
		{
			description: "genrule applies properties from list of genrule_defaults",
			blueprint: `genrule_defaults {
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
			expectedBazelTargets: []string{
				makeBazelTarget("genrule", "gen", attrNameToString{
					"cmd":  `"cp $(SRCS) $(OUTS)"`,
					"outs": `["out"]`,
					"srcs": `["in1"]`,
				}),
			},
		},
		{
			description: "genrule applies properties from genrule_defaults transitively",
			blueprint: `genrule_defaults {
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
			expectedBazelTargets: []string{
				makeBazelTarget("genrule", "gen", attrNameToString{
					"cmd": `"cmd1 $(SRCS) $(OUTS)"`,
					"outs": `[
        "out-from-3",
        "out-from-2",
        "out",
    ]`,
					"srcs": `[
        "srcs-from-3",
        "in1",
    ]`,
				}),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			runGenruleTestCase(t, testCase)
		})
	}
}
