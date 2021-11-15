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
	"android/soong/genrule"
)

var otherCcGenruleBp = map[string]string{
	"other/Android.bp": `cc_genrule {
    name: "foo.tool",
    out: ["foo_tool.out"],
    srcs: ["foo_tool.in"],
    cmd: "cp $(in) $(out)",
}
cc_genrule {
    name: "other.tool",
    out: ["other_tool.out"],
    srcs: ["other_tool.in"],
    cmd: "cp $(in) $(out)",
}`,
}

func runCcGenruleTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	(&tc).moduleTypeUnderTest = "cc_genrule"
	(&tc).moduleTypeUnderTestFactory = cc.GenRuleFactory
	(&tc).moduleTypeUnderTestBp2BuildMutator = genrule.CcGenruleBp2Build
	runBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, tc)
}

func TestCliVariableReplacement(t *testing.T) {
	runCcGenruleTestCase(t, bp2buildTestCase{
		description: "cc_genrule with command line variable replacements",
		blueprint: `cc_genrule {
    name: "foo.tool",
    out: ["foo_tool.out"],
    srcs: ["foo_tool.in"],
    cmd: "cp $(in) $(out)",
    bazel_module: { bp2build_available: true },
}

cc_genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    tools: [":foo.tool"],
    cmd: "$(location :foo.tool) --genDir=$(genDir) arg $(in) $(out)",
    bazel_module: { bp2build_available: true },
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("genrule", "foo", attrNameToString{
				"cmd":   `"$(location :foo.tool) --genDir=$(RULEDIR) arg $(SRCS) $(OUTS)"`,
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
	})
}

func TestUsingLocationsLabel(t *testing.T) {
	runCcGenruleTestCase(t, bp2buildTestCase{
		description: "cc_genrule using $(locations :label)",
		blueprint: `cc_genrule {
    name: "foo.tools",
    out: ["foo_tool.out", "foo_tool2.out"],
    srcs: ["foo_tool.in"],
    cmd: "cp $(in) $(out)",
    bazel_module: { bp2build_available: true },
}

cc_genrule {
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
	})
}

func TestUsingLocationsAbsoluteLabel(t *testing.T) {
	runCcGenruleTestCase(t, bp2buildTestCase{
		description: "cc_genrule using $(locations //absolute:label)",
		blueprint: `cc_genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    tool_files: [":foo.tool"],
    cmd: "$(locations :foo.tool) -s $(out) $(in)",
    bazel_module: { bp2build_available: true },
}`,
		filesystem: otherCcGenruleBp,
		expectedBazelTargets: []string{
			makeBazelTarget("genrule", "foo", attrNameToString{
				"cmd":   `"$(locations //other:foo.tool) -s $(OUTS) $(SRCS)"`,
				"outs":  `["foo.out"]`,
				"srcs":  `["foo.in"]`,
				"tools": `["//other:foo.tool"]`,
			}),
		},
	})
}

func TestSrcsUsingAbsoluteLabel(t *testing.T) {
	runCcGenruleTestCase(t, bp2buildTestCase{
		description: "cc_genrule srcs using $(locations //absolute:label)",
		blueprint: `cc_genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: [":other.tool"],
    tool_files: [":foo.tool"],
    cmd: "$(locations :foo.tool) -s $(out) $(location :other.tool)",
    bazel_module: { bp2build_available: true },
}`,
		filesystem: otherCcGenruleBp,
		expectedBazelTargets: []string{
			makeBazelTarget("genrule", "foo", attrNameToString{
				"cmd":   `"$(locations //other:foo.tool) -s $(OUTS) $(location //other:other.tool)"`,
				"outs":  `["foo.out"]`,
				"srcs":  `["//other:other.tool"]`,
				"tools": `["//other:foo.tool"]`,
			}),
		},
	})
}

func TestLocationsLabelUsesFirstToolFile(t *testing.T) {
	runCcGenruleTestCase(t, bp2buildTestCase{
		description: "cc_genrule using $(location) label should substitute first tool label automatically",
		blueprint: `cc_genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    tool_files: [":foo.tool", ":other.tool"],
    cmd: "$(location) -s $(out) $(in)",
    bazel_module: { bp2build_available: true },
}`,
		filesystem: otherCcGenruleBp,
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
	})
}

func TestLocationsLabelUsesFirstTool(t *testing.T) {
	runCcGenruleTestCase(t, bp2buildTestCase{
		description: "cc_genrule using $(locations) label should substitute first tool label automatically",
		blueprint: `cc_genrule {
    name: "foo",
    out: ["foo.out"],
    srcs: ["foo.in"],
    tools: [":foo.tool", ":other.tool"],
    cmd: "$(locations) -s $(out) $(in)",
    bazel_module: { bp2build_available: true },
}`,
		filesystem: otherCcGenruleBp,
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
	})
}

func TestWithoutToolsOrToolFiles(t *testing.T) {
	runCcGenruleTestCase(t, bp2buildTestCase{
		description: "cc_genrule without tools or tool_files can convert successfully",
		blueprint: `cc_genrule {
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
	})
}
