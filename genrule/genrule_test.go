// Copyright 2018 Google Inc. All rights reserved.
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

package genrule

import (
	"os"
	"regexp"
	"testing"

	"android/soong/android"

	"github.com/google/blueprint/proptools"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

var prepareForGenRuleTest = android.GroupFixturePreparers(
	android.PrepareForTestWithArchMutator,
	android.PrepareForTestWithDefaults,

	android.PrepareForTestWithFilegroup,
	PrepareForTestWithGenRuleBuildComponents,
	android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("tool", toolFactory)
		ctx.RegisterModuleType("output", outputProducerFactory)
	}),
	android.FixtureMergeMockFs(android.MockFS{
		"tool":       nil,
		"tool_file1": nil,
		"tool_file2": nil,
		"in1":        nil,
		"in2":        nil,
		"in1.txt":    nil,
		"in2.txt":    nil,
		"in3.txt":    nil,
	}),
)

func testGenruleBp() string {
	return `
		tool {
			name: "tool",
		}

		filegroup {
			name: "tool_files",
			srcs: [
				"tool_file1",
				"tool_file2",
			],
		}

		filegroup {
			name: "1tool_file",
			srcs: [
				"tool_file1",
			],
		}

		filegroup {
			name: "ins",
			srcs: [
				"in1",
				"in2",
			],
		}

		filegroup {
			name: "1in",
			srcs: [
				"in1",
			],
		}

		filegroup {
			name: "empty",
		}
	`
}

func TestGenruleCmd(t *testing.T) {
	testcases := []struct {
		name string
		prop string

		allowMissingDependencies bool

		err    string
		expect string
	}{
		{
			name: "empty location tool",
			prop: `
				tools: ["tool"],
				out: ["out"],
				cmd: "$(location) > $(out)",
			`,
			expect: "__SBOX_SANDBOX_DIR__/tools/out/bin/tool > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "empty location tool2",
			prop: `
				tools: [":tool"],
				out: ["out"],
				cmd: "$(location) > $(out)",
			`,
			expect: "__SBOX_SANDBOX_DIR__/tools/out/bin/tool > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "empty location tool file",
			prop: `
				tool_files: ["tool_file1"],
				out: ["out"],
				cmd: "$(location) > $(out)",
			`,
			expect: "__SBOX_SANDBOX_DIR__/tools/src/tool_file1 > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "empty location tool file fg",
			prop: `
				tool_files: [":1tool_file"],
				out: ["out"],
				cmd: "$(location) > $(out)",
			`,
			expect: "__SBOX_SANDBOX_DIR__/tools/src/tool_file1 > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "empty location tool and tool file",
			prop: `
				tools: ["tool"],
				tool_files: ["tool_file1"],
				out: ["out"],
				cmd: "$(location) > $(out)",
			`,
			expect: "__SBOX_SANDBOX_DIR__/tools/out/bin/tool > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "tool",
			prop: `
				tools: ["tool"],
				out: ["out"],
				cmd: "$(location tool) > $(out)",
			`,
			expect: "__SBOX_SANDBOX_DIR__/tools/out/bin/tool > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "tool2",
			prop: `
				tools: [":tool"],
				out: ["out"],
				cmd: "$(location :tool) > $(out)",
			`,
			expect: "__SBOX_SANDBOX_DIR__/tools/out/bin/tool > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "tool file",
			prop: `
				tool_files: ["tool_file1"],
				out: ["out"],
				cmd: "$(location tool_file1) > $(out)",
			`,
			expect: "__SBOX_SANDBOX_DIR__/tools/src/tool_file1 > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "tool file fg",
			prop: `
				tool_files: [":1tool_file"],
				out: ["out"],
				cmd: "$(location :1tool_file) > $(out)",
			`,
			expect: "__SBOX_SANDBOX_DIR__/tools/src/tool_file1 > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "tool files",
			prop: `
				tool_files: [":tool_files"],
				out: ["out"],
				cmd: "$(locations :tool_files) > $(out)",
			`,
			expect: "__SBOX_SANDBOX_DIR__/tools/src/tool_file1 __SBOX_SANDBOX_DIR__/tools/src/tool_file2 > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "in1",
			prop: `
				srcs: ["in1"],
				out: ["out"],
				cmd: "cat $(in) > $(out)",
			`,
			expect: "cat in1 > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "in1 fg",
			prop: `
				srcs: [":1in"],
				out: ["out"],
				cmd: "cat $(in) > $(out)",
			`,
			expect: "cat in1 > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "ins",
			prop: `
				srcs: ["in1", "in2"],
				out: ["out"],
				cmd: "cat $(in) > $(out)",
			`,
			expect: "cat in1 in2 > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "ins fg",
			prop: `
				srcs: [":ins"],
				out: ["out"],
				cmd: "cat $(in) > $(out)",
			`,
			expect: "cat in1 in2 > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "location in1",
			prop: `
				srcs: ["in1"],
				out: ["out"],
				cmd: "cat $(location in1) > $(out)",
			`,
			expect: "cat in1 > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "location in1 fg",
			prop: `
				srcs: [":1in"],
				out: ["out"],
				cmd: "cat $(location :1in) > $(out)",
			`,
			expect: "cat in1 > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "location ins",
			prop: `
				srcs: ["in1", "in2"],
				out: ["out"],
				cmd: "cat $(location in1) > $(out)",
			`,
			expect: "cat in1 > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "location ins fg",
			prop: `
				srcs: [":ins"],
				out: ["out"],
				cmd: "cat $(locations :ins) > $(out)",
			`,
			expect: "cat in1 in2 > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "outs",
			prop: `
				out: ["out", "out2"],
				cmd: "echo foo > $(out)",
			`,
			expect: "echo foo > __SBOX_SANDBOX_DIR__/out/out __SBOX_SANDBOX_DIR__/out/out2",
		},
		{
			name: "location out",
			prop: `
				out: ["out", "out2"],
				cmd: "echo foo > $(location out2)",
			`,
			expect: "echo foo > __SBOX_SANDBOX_DIR__/out/out2",
		},
		{
			name: "depfile",
			prop: `
				out: ["out"],
				depfile: true,
				cmd: "echo foo > $(out) && touch $(depfile)",
			`,
			expect: "echo foo > __SBOX_SANDBOX_DIR__/out/out && touch __SBOX_DEPFILE__",
		},
		{
			name: "gendir",
			prop: `
				out: ["out"],
				cmd: "echo foo > $(genDir)/foo && cp $(genDir)/foo $(out)",
			`,
			expect: "echo foo > __SBOX_SANDBOX_DIR__/out/foo && cp __SBOX_SANDBOX_DIR__/out/foo __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "$",
			prop: `
				out: ["out"],
				cmd: "echo $$ > $(out)",
			`,
			expect: "echo $ > __SBOX_SANDBOX_DIR__/out/out",
		},

		{
			name: "error empty location",
			prop: `
				out: ["out"],
				cmd: "$(location) > $(out)",
			`,
			err: "at least one `tools` or `tool_files` is required if $(location) is used",
		},
		{
			name: "error empty location no files",
			prop: `
				tool_files: [":empty"],
				out: ["out"],
				cmd: "$(location) > $(out)",
			`,
			err: `default label ":empty" has no files`,
		},
		{
			name: "error empty location multiple files",
			prop: `
				tool_files: [":tool_files"],
				out: ["out"],
				cmd: "$(location) > $(out)",
			`,
			err: `default label ":tool_files" has multiple files`,
		},
		{
			name: "error location",
			prop: `
				out: ["out"],
				cmd: "echo foo > $(location missing)",
			`,
			err: `unknown location label "missing"`,
		},
		{
			name: "error locations",
			prop: `
					out: ["out"],
					cmd: "echo foo > $(locations missing)",
			`,
			err: `unknown locations label "missing"`,
		},
		{
			name: "error location no files",
			prop: `
					out: ["out"],
					srcs: [":empty"],
					cmd: "echo $(location :empty) > $(out)",
			`,
			err: `label ":empty" has no files`,
		},
		{
			name: "error locations no files",
			prop: `
					out: ["out"],
					srcs: [":empty"],
					cmd: "echo $(locations :empty) > $(out)",
			`,
			err: `label ":empty" has no files`,
		},
		{
			name: "error location multiple files",
			prop: `
					out: ["out"],
					srcs: [":ins"],
					cmd: "echo $(location :ins) > $(out)",
			`,
			err: `label ":ins" has multiple files`,
		},
		{
			name: "error variable",
			prop: `
					out: ["out"],
					srcs: ["in1"],
					cmd: "echo $(foo) > $(out)",
			`,
			err: `unknown variable '$(foo)'`,
		},
		{
			name: "error depfile",
			prop: `
				out: ["out"],
				cmd: "echo foo > $(out) && touch $(depfile)",
			`,
			err: "$(depfile) used without depfile property",
		},
		{
			name: "error no depfile",
			prop: `
				out: ["out"],
				depfile: true,
				cmd: "echo foo > $(out)",
			`,
			err: "specified depfile=true but did not include a reference to '${depfile}' in cmd",
		},
		{
			name: "error no out",
			prop: `
				cmd: "echo foo > $(out)",
			`,
			err: "must have at least one output file",
		},
		{
			name: "srcs allow missing dependencies",
			prop: `
				srcs: [":missing"],
				out: ["out"],
				cmd: "cat $(location :missing) > $(out)",
			`,

			allowMissingDependencies: true,

			expect: "cat ***missing srcs :missing*** > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "tool allow missing dependencies",
			prop: `
				tools: [":missing"],
				out: ["out"],
				cmd: "$(location :missing) > $(out)",
			`,

			allowMissingDependencies: true,

			expect: "***missing tool :missing*** > __SBOX_SANDBOX_DIR__/out/out",
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			bp := "genrule {\n"
			bp += "name: \"gen\",\n"
			bp += test.prop
			bp += "}\n"

			var expectedErrors []string
			if test.err != "" {
				expectedErrors = append(expectedErrors, regexp.QuoteMeta(test.err))
			}

			result := android.GroupFixturePreparers(
				prepareForGenRuleTest,
				android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
					variables.Allow_missing_dependencies = proptools.BoolPtr(test.allowMissingDependencies)
				}),
				android.FixtureModifyContext(func(ctx *android.TestContext) {
					ctx.SetAllowMissingDependencies(test.allowMissingDependencies)
				}),
			).
				ExtendWithErrorHandler(android.FixtureExpectsAllErrorsToMatchAPattern(expectedErrors)).
				RunTestWithBp(t, testGenruleBp()+bp)

			if expectedErrors != nil {
				return
			}

			gen := result.Module("gen", "").(*Module)
			android.AssertStringEquals(t, "raw commands", test.expect, gen.rawCommands[0])
		})
	}
}

func TestGenruleHashInputs(t *testing.T) {

	// The basic idea here is to verify that the sbox command (which is
	// in the Command field of the generate rule) contains a hash of the
	// inputs, but only if $(in) is not referenced in the genrule cmd
	// property.

	// By including a hash of the inputs, we cause the rule to re-run if
	// the list of inputs changes (because the sbox command changes).

	// However, if the genrule cmd property already contains $(in), then
	// the dependency is already expressed, so we don't need to include the
	// hash in that case.

	bp := `
			genrule {
				name: "hash0",
				srcs: ["in1.txt", "in2.txt"],
				out: ["out"],
				cmd: "echo foo > $(out)",
			}
			genrule {
				name: "hash1",
				srcs: ["*.txt"],
				out: ["out"],
				cmd: "echo bar > $(out)",
			}
			genrule {
				name: "hash2",
				srcs: ["*.txt"],
				out: ["out"],
				cmd: "echo $(in) > $(out)",
			}
		`
	testcases := []struct {
		name         string
		expectedHash string
	}{
		{
			name: "hash0",
			// sha256 value obtained from: echo -en 'in1.txt\nin2.txt' | sha256sum
			expectedHash: "18da75b9b1cc74b09e365b4ca2e321b5d618f438cc632b387ad9dc2ab4b20e9d",
		},
		{
			name: "hash1",
			// sha256 value obtained from: echo -en 'in1.txt\nin2.txt\nin3.txt' | sha256sum
			expectedHash: "a38d432a4b19df93140e1f1fe26c97ff0387dae01fe506412b47208f0595fb45",
		},
		{
			name: "hash2",
			// sha256 value obtained from: echo -en 'in1.txt\nin2.txt\nin3.txt' | sha256sum
			expectedHash: "a38d432a4b19df93140e1f1fe26c97ff0387dae01fe506412b47208f0595fb45",
		},
	}

	result := prepareForGenRuleTest.RunTestWithBp(t, testGenruleBp()+bp)

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			gen := result.ModuleForTests(test.name, "")
			manifest := android.RuleBuilderSboxProtoForTests(t, gen.Output("genrule.sbox.textproto"))
			hash := manifest.Commands[0].GetInputHash()

			android.AssertStringEquals(t, "hash", test.expectedHash, hash)
		})
	}
}

func TestGenSrcs(t *testing.T) {
	testcases := []struct {
		name string
		prop string

		allowMissingDependencies bool

		err   string
		cmds  []string
		deps  []string
		files []string
	}{
		{
			name: "gensrcs",
			prop: `
				tools: ["tool"],
				srcs: ["in1.txt", "in2.txt"],
				cmd: "$(location) $(in) > $(out)",
			`,
			cmds: []string{
				"bash -c '__SBOX_SANDBOX_DIR__/tools/out/bin/tool in1.txt > __SBOX_SANDBOX_DIR__/out/in1.h' && bash -c '__SBOX_SANDBOX_DIR__/tools/out/bin/tool in2.txt > __SBOX_SANDBOX_DIR__/out/in2.h'",
			},
			deps: []string{
				"out/soong/.intermediates/gen/gen/gensrcs/in1.h",
				"out/soong/.intermediates/gen/gen/gensrcs/in2.h",
			},
			files: []string{
				"out/soong/.intermediates/gen/gen/gensrcs/in1.h",
				"out/soong/.intermediates/gen/gen/gensrcs/in2.h",
			},
		},
		{
			name: "shards",
			prop: `
				tools: ["tool"],
				srcs: ["in1.txt", "in2.txt", "in3.txt"],
				cmd: "$(location) $(in) > $(out)",
				shard_size: 2,
			`,
			cmds: []string{
				"bash -c '__SBOX_SANDBOX_DIR__/tools/out/bin/tool in1.txt > __SBOX_SANDBOX_DIR__/out/in1.h' && bash -c '__SBOX_SANDBOX_DIR__/tools/out/bin/tool in2.txt > __SBOX_SANDBOX_DIR__/out/in2.h'",
				"bash -c '__SBOX_SANDBOX_DIR__/tools/out/bin/tool in3.txt > __SBOX_SANDBOX_DIR__/out/in3.h'",
			},
			deps: []string{
				"out/soong/.intermediates/gen/gen/gensrcs/in1.h",
				"out/soong/.intermediates/gen/gen/gensrcs/in2.h",
				"out/soong/.intermediates/gen/gen/gensrcs/in3.h",
			},
			files: []string{
				"out/soong/.intermediates/gen/gen/gensrcs/in1.h",
				"out/soong/.intermediates/gen/gen/gensrcs/in2.h",
				"out/soong/.intermediates/gen/gen/gensrcs/in3.h",
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			bp := "gensrcs {\n"
			bp += `name: "gen",` + "\n"
			bp += `output_extension: "h",` + "\n"
			bp += test.prop
			bp += "}\n"

			var expectedErrors []string
			if test.err != "" {
				expectedErrors = append(expectedErrors, regexp.QuoteMeta(test.err))
			}

			result := prepareForGenRuleTest.
				ExtendWithErrorHandler(android.FixtureExpectsAllErrorsToMatchAPattern(expectedErrors)).
				RunTestWithBp(t, testGenruleBp()+bp)

			if expectedErrors != nil {
				return
			}

			gen := result.Module("gen", "").(*Module)
			android.AssertDeepEquals(t, "cmd", test.cmds, gen.rawCommands)

			android.AssertPathsRelativeToTopEquals(t, "deps", test.deps, gen.outputDeps)

			android.AssertPathsRelativeToTopEquals(t, "files", test.files, gen.outputFiles)
		})
	}
}

func TestGenruleDefaults(t *testing.T) {
	bp := `
				genrule_defaults {
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
			`

	result := prepareForGenRuleTest.RunTestWithBp(t, testGenruleBp()+bp)

	gen := result.Module("gen", "").(*Module)

	expectedCmd := "cp in1 __SBOX_SANDBOX_DIR__/out/out"
	android.AssertStringEquals(t, "cmd", expectedCmd, gen.rawCommands[0])

	expectedSrcs := []string{"in1"}
	android.AssertDeepEquals(t, "srcs", expectedSrcs, gen.properties.Srcs)
}

func TestGenruleAllowMissingDependencies(t *testing.T) {
	bp := `
		output {
			name: "disabled",
			enabled: false,
		}

		genrule {
			name: "gen",
			srcs: [
				":disabled",
			],
			out: ["out"],
			cmd: "cat $(in) > $(out)",
		}
       `
	result := prepareForGenRuleTest.Extend(
		android.FixtureModifyConfigAndContext(
			func(config android.Config, ctx *android.TestContext) {
				config.TestProductVariables.Allow_missing_dependencies = proptools.BoolPtr(true)
				ctx.SetAllowMissingDependencies(true)
			})).RunTestWithBp(t, bp)

	gen := result.ModuleForTests("gen", "").Output("out")
	if gen.Rule != android.ErrorRule {
		t.Errorf("Expected missing dependency error rule for gen, got %q", gen.Rule.String())
	}
}

func TestGenruleWithBazel(t *testing.T) {
	bp := `
		genrule {
				name: "foo",
				out: ["one.txt", "two.txt"],
				bazel_module: { label: "//foo/bar:bar" },
		}
	`

	result := android.GroupFixturePreparers(
		prepareForGenRuleTest, android.FixtureModifyConfig(func(config android.Config) {
			config.BazelContext = android.MockBazelContext{
				AllFiles: map[string][]string{
					"//foo/bar:bar": []string{"bazelone.txt", "bazeltwo.txt"}}}
		})).RunTestWithBp(t, testGenruleBp()+bp)

	gen := result.Module("foo", "").(*Module)

	expectedOutputFiles := []string{"outputbase/execroot/__main__/bazelone.txt",
		"outputbase/execroot/__main__/bazeltwo.txt"}
	android.AssertDeepEquals(t, "output files", expectedOutputFiles, gen.outputFiles.Strings())
	android.AssertDeepEquals(t, "output deps", expectedOutputFiles, gen.outputDeps.Strings())
}

type testTool struct {
	android.ModuleBase
	outputFile android.Path
}

func toolFactory() android.Module {
	module := &testTool{}
	android.InitAndroidArchModule(module, android.HostSupported, android.MultilibFirst)
	return module
}

func (t *testTool) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	t.outputFile = ctx.InstallFile(android.PathForModuleInstall(ctx, "bin"), ctx.ModuleName(), android.PathForOutput(ctx, ctx.ModuleName()))
}

func (t *testTool) HostToolPath() android.OptionalPath {
	return android.OptionalPathForPath(t.outputFile)
}

var _ android.HostToolProvider = (*testTool)(nil)

type testOutputProducer struct {
	android.ModuleBase
	outputFile android.Path
}

func outputProducerFactory() android.Module {
	module := &testOutputProducer{}
	android.InitAndroidArchModule(module, android.HostSupported, android.MultilibFirst)
	return module
}

func (t *testOutputProducer) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	t.outputFile = ctx.InstallFile(android.PathForModuleInstall(ctx, "bin"), ctx.ModuleName(), android.PathForOutput(ctx, ctx.ModuleName()))
}

func (t *testOutputProducer) OutputFiles(tag string) (android.Paths, error) {
	return android.Paths{t.outputFile}, nil
}

var _ android.OutputFileProducer = (*testOutputProducer)(nil)
