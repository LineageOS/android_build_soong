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
	"fmt"
	"os"
	"regexp"
	"strconv"
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
		android.RegisterPrebuiltMutators(ctx)
		ctx.RegisterModuleType("tool", toolFactory)
		ctx.RegisterModuleType("prebuilt_tool", prebuiltToolFactory)
		ctx.RegisterModuleType("output", outputProducerFactory)
		ctx.RegisterModuleType("use_source", useSourceFactory)
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
		name       string
		moduleName string
		prop       string

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
			err: `unknown location label "missing" is not in srcs, out, tools or tool_files.`,
		},
		{
			name: "error locations",
			prop: `
					out: ["out"],
					cmd: "echo foo > $(locations missing)",
			`,
			err: `unknown locations label "missing" is not in srcs, out, tools or tool_files`,
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

			expect: "cat '***missing srcs :missing***' > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "tool allow missing dependencies",
			prop: `
				tools: [":missing"],
				out: ["out"],
				cmd: "$(location :missing) > $(out)",
			`,

			allowMissingDependencies: true,

			expect: "'***missing tool :missing***' > __SBOX_SANDBOX_DIR__/out/out",
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			moduleName := "gen"
			if test.moduleName != "" {
				moduleName = test.moduleName
			}
			bp := fmt.Sprintf(`
			genrule {
			   name: "%s",
			   %s
			}`, moduleName, test.prop)
			var expectedErrors []string
			if test.err != "" {
				expectedErrors = append(expectedErrors, regexp.QuoteMeta(test.err))
			}

			result := android.GroupFixturePreparers(
				prepareForGenRuleTest,
				android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
					variables.Allow_missing_dependencies = proptools.BoolPtr(test.allowMissingDependencies)
				}),
				android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
					variables.GenruleSandboxing = proptools.BoolPtr(true)
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

			gen := result.Module(moduleName, "").(*Module)
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
			manifest := android.RuleBuilderSboxProtoForTests(t, result.TestContext, gen.Output("genrule.sbox.textproto"))
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

		err    string
		cmds   []string
		deps   []string
		files  []string
		shards int
		inputs []string
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
		{
			name: "data",
			prop: `
				tools: ["tool"],
				srcs: ["in1.txt", "in2.txt", "in3.txt"],
				cmd: "$(location) $(in) --extra_input=$(location baz.txt) > $(out)",
				data: ["baz.txt"],
				shard_size: 2,
			`,
			cmds: []string{
				"bash -c '__SBOX_SANDBOX_DIR__/tools/out/bin/tool in1.txt --extra_input=baz.txt > __SBOX_SANDBOX_DIR__/out/in1.h' && bash -c '__SBOX_SANDBOX_DIR__/tools/out/bin/tool in2.txt --extra_input=baz.txt > __SBOX_SANDBOX_DIR__/out/in2.h'",
				"bash -c '__SBOX_SANDBOX_DIR__/tools/out/bin/tool in3.txt --extra_input=baz.txt > __SBOX_SANDBOX_DIR__/out/in3.h'",
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
			shards: 2,
			inputs: []string{
				"baz.txt",
			},
		},
	}

	checkInputs := func(t *testing.T, rule android.TestingBuildParams, inputs []string) {
		t.Helper()
		if len(inputs) == 0 {
			return
		}
		inputBaseNames := map[string]bool{}
		for _, f := range rule.Implicits {
			inputBaseNames[f.Base()] = true
		}
		for _, f := range inputs {
			if _, ok := inputBaseNames[f]; !ok {
				t.Errorf("Expected to find input file %q for %q, but did not", f, rule.Description)
			}
		}
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

			mod := result.ModuleForTests("gen", "")
			if expectedErrors != nil {
				return
			}

			if test.shards > 0 {
				for i := 0; i < test.shards; i++ {
					r := mod.Rule("generator" + strconv.Itoa(i))
					checkInputs(t, r, test.inputs)
				}
			} else {
				r := mod.Rule("generator")
				checkInputs(t, r, test.inputs)
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
	android.AssertDeepEquals(t, "srcs", expectedSrcs, gen.properties.ResolvedSrcs)
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
	result := android.GroupFixturePreparers(
		prepareForGenRuleTest,
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

func TestGenruleOutputFiles(t *testing.T) {
	bp := `
				genrule {
					name: "gen",
					out: ["foo", "sub/bar"],
					cmd: "echo foo > $(location foo) && echo bar > $(location sub/bar)",
				}
				use_source {
					name: "gen_foo",
					srcs: [":gen{foo}"],
				}
				use_source {
					name: "gen_bar",
					srcs: [":gen{sub/bar}"],
				}
				use_source {
					name: "gen_all",
					srcs: [":gen"],
				}
			`

	result := prepareForGenRuleTest.RunTestWithBp(t, testGenruleBp()+bp)
	android.AssertPathsRelativeToTopEquals(t,
		"genrule.tag with output",
		[]string{"out/soong/.intermediates/gen/gen/foo"},
		result.ModuleForTests("gen_foo", "").Module().(*useSource).srcs)
	android.AssertPathsRelativeToTopEquals(t,
		"genrule.tag with output in subdir",
		[]string{"out/soong/.intermediates/gen/gen/sub/bar"},
		result.ModuleForTests("gen_bar", "").Module().(*useSource).srcs)
	android.AssertPathsRelativeToTopEquals(t,
		"genrule.tag with all",
		[]string{"out/soong/.intermediates/gen/gen/foo", "out/soong/.intermediates/gen/gen/sub/bar"},
		result.ModuleForTests("gen_all", "").Module().(*useSource).srcs)
}

func TestGenruleInterface(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForGenRuleTest,
		android.FixtureMergeMockFs(android.MockFS{
			"package-dir/Android.bp": []byte(`
				genrule {
					name: "module-name",
					cmd: "mkdir -p $(genDir) && cat $(in) >> $(genDir)/$(out)",
					srcs: [
						"src/foo.proto",
					],
					out: ["proto.h", "bar/proto.h"],
					export_include_dirs: [".", "bar"],
				}
			`),
		}),
	).RunTest(t)

	exportedIncludeDirs := []string{
		"out/soong/.intermediates/package-dir/module-name/gen/package-dir",
		"out/soong/.intermediates/package-dir/module-name/gen",
		"out/soong/.intermediates/package-dir/module-name/gen/package-dir/bar",
		"out/soong/.intermediates/package-dir/module-name/gen/bar",
	}
	gen := result.Module("module-name", "").(*Module)

	android.AssertPathsRelativeToTopEquals(
		t,
		"include path",
		exportedIncludeDirs,
		gen.GeneratedHeaderDirs(),
	)
	android.AssertPathsRelativeToTopEquals(
		t,
		"files",
		[]string{
			"out/soong/.intermediates/package-dir/module-name/gen/proto.h",
			"out/soong/.intermediates/package-dir/module-name/gen/bar/proto.h",
		},
		gen.GeneratedSourceFiles(),
	)
}

func TestGenSrcsWithNonRootAndroidBpOutputFiles(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForGenRuleTest,
		android.FixtureMergeMockFs(android.MockFS{
			"external-protos/path/Android.bp": []byte(`
				filegroup {
					name: "external-protos",
					srcs: ["baz/baz.proto", "bar.proto"],
				}
			`),
			"package-dir/Android.bp": []byte(`
				gensrcs {
					name: "module-name",
					cmd: "mkdir -p $(genDir) && cat $(in) >> $(genDir)/$(out)",
					srcs: [
						"src/foo.proto",
						":external-protos",
					],
					output_extension: "proto.h",
				}
			`),
		}),
	).RunTest(t)

	exportedIncludeDir := "out/soong/.intermediates/package-dir/module-name/gen/gensrcs"
	gen := result.Module("module-name", "").(*Module)

	android.AssertPathsRelativeToTopEquals(
		t,
		"include path",
		[]string{exportedIncludeDir},
		gen.exportedIncludeDirs,
	)
	android.AssertPathsRelativeToTopEquals(
		t,
		"files",
		[]string{
			exportedIncludeDir + "/package-dir/src/foo.proto.h",
			exportedIncludeDir + "/external-protos/path/baz/baz.proto.h",
			exportedIncludeDir + "/external-protos/path/bar.proto.h",
		},
		gen.outputFiles,
	)
}

func TestGenSrcsWithSrcsFromExternalPackage(t *testing.T) {
	bp := `
		gensrcs {
			name: "module-name",
			cmd: "mkdir -p $(genDir) && cat $(in) >> $(genDir)/$(out)",
			srcs: [
				":external-protos",
			],
			output_extension: "proto.h",
		}
	`
	result := android.GroupFixturePreparers(
		prepareForGenRuleTest,
		android.FixtureMergeMockFs(android.MockFS{
			"external-protos/path/Android.bp": []byte(`
				filegroup {
					name: "external-protos",
					srcs: ["foo/foo.proto", "bar.proto"],
				}
			`),
		}),
	).RunTestWithBp(t, bp)

	exportedIncludeDir := "out/soong/.intermediates/module-name/gen/gensrcs"
	gen := result.Module("module-name", "").(*Module)

	android.AssertPathsRelativeToTopEquals(
		t,
		"include path",
		[]string{exportedIncludeDir},
		gen.exportedIncludeDirs,
	)
	android.AssertPathsRelativeToTopEquals(
		t,
		"files",
		[]string{
			exportedIncludeDir + "/external-protos/path/foo/foo.proto.h",
			exportedIncludeDir + "/external-protos/path/bar.proto.h",
		},
		gen.outputFiles,
	)
}

func TestGenSrcsWithTrimExtAndOutpuExtension(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForGenRuleTest,
		android.FixtureMergeMockFs(android.MockFS{
			"external-protos/path/Android.bp": []byte(`
				filegroup {
					name: "external-protos",
					srcs: [
					    "baz.a.b.c.proto/baz.a.b.c.proto",
					    "bar.a.b.c.proto",
					    "qux.ext.a.b.c.proto",
					],
				}
			`),
			"package-dir/Android.bp": []byte(`
				gensrcs {
					name: "module-name",
					cmd: "mkdir -p $(genDir) && cat $(in) >> $(genDir)/$(out)",
					srcs: [
						"src/foo.a.b.c.proto",
						":external-protos",
					],

					trim_extension: ".a.b.c.proto",
					output_extension: "proto.h",
				}
			`),
		}),
	).RunTest(t)

	exportedIncludeDir := "out/soong/.intermediates/package-dir/module-name/gen/gensrcs"
	gen := result.Module("module-name", "").(*Module)

	android.AssertPathsRelativeToTopEquals(
		t,
		"include path",
		[]string{exportedIncludeDir},
		gen.exportedIncludeDirs,
	)
	android.AssertPathsRelativeToTopEquals(
		t,
		"files",
		[]string{
			exportedIncludeDir + "/package-dir/src/foo.proto.h",
			exportedIncludeDir + "/external-protos/path/baz.a.b.c.proto/baz.proto.h",
			exportedIncludeDir + "/external-protos/path/bar.proto.h",
			exportedIncludeDir + "/external-protos/path/qux.ext.proto.h",
		},
		gen.outputFiles,
	)
}

func TestGenSrcsWithTrimExtButNoOutpuExtension(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForGenRuleTest,
		android.FixtureMergeMockFs(android.MockFS{
			"external-protos/path/Android.bp": []byte(`
				filegroup {
					name: "external-protos",
					srcs: [
					    "baz.a.b.c.proto/baz.a.b.c.proto",
					    "bar.a.b.c.proto",
					    "qux.ext.a.b.c.proto",
					],
				}
			`),
			"package-dir/Android.bp": []byte(`
				gensrcs {
					name: "module-name",
					cmd: "mkdir -p $(genDir) && cat $(in) >> $(genDir)/$(out)",
					srcs: [
						"src/foo.a.b.c.proto",
						":external-protos",
					],

					trim_extension: ".a.b.c.proto",
				}
			`),
		}),
	).RunTest(t)

	exportedIncludeDir := "out/soong/.intermediates/package-dir/module-name/gen/gensrcs"
	gen := result.Module("module-name", "").(*Module)

	android.AssertPathsRelativeToTopEquals(
		t,
		"include path",
		[]string{exportedIncludeDir},
		gen.exportedIncludeDirs,
	)
	android.AssertPathsRelativeToTopEquals(
		t,
		"files",
		[]string{
			exportedIncludeDir + "/package-dir/src/foo",
			exportedIncludeDir + "/external-protos/path/baz.a.b.c.proto/baz",
			exportedIncludeDir + "/external-protos/path/bar",
			exportedIncludeDir + "/external-protos/path/qux.ext",
		},
		gen.outputFiles,
	)
}

func TestGenSrcsWithOutpuExtension(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForGenRuleTest,
		android.FixtureMergeMockFs(android.MockFS{
			"external-protos/path/Android.bp": []byte(`
				filegroup {
					name: "external-protos",
					srcs: ["baz/baz.a.b.c.proto", "bar.a.b.c.proto"],
				}
			`),
			"package-dir/Android.bp": []byte(`
				gensrcs {
					name: "module-name",
					cmd: "mkdir -p $(genDir) && cat $(in) >> $(genDir)/$(out)",
					srcs: [
						"src/foo.a.b.c.proto",
						":external-protos",
					],

					output_extension: "proto.h",
				}
			`),
		}),
	).RunTest(t)

	exportedIncludeDir := "out/soong/.intermediates/package-dir/module-name/gen/gensrcs"
	gen := result.Module("module-name", "").(*Module)

	android.AssertPathsRelativeToTopEquals(
		t,
		"include path",
		[]string{exportedIncludeDir},
		gen.exportedIncludeDirs,
	)
	android.AssertPathsRelativeToTopEquals(
		t,
		"files",
		[]string{
			exportedIncludeDir + "/package-dir/src/foo.a.b.c.proto.h",
			exportedIncludeDir + "/external-protos/path/baz/baz.a.b.c.proto.h",
			exportedIncludeDir + "/external-protos/path/bar.a.b.c.proto.h",
		},
		gen.outputFiles,
	)
}

func TestPrebuiltTool(t *testing.T) {
	testcases := []struct {
		name             string
		bp               string
		expectedToolName string
	}{
		{
			name: "source only",
			bp: `
				tool { name: "tool" }
			`,
			expectedToolName: "bin/tool",
		},
		{
			name: "prebuilt only",
			bp: `
				prebuilt_tool { name: "tool" }
			`,
			expectedToolName: "prebuilt_bin/tool",
		},
		{
			name: "source preferred",
			bp: `
				tool { name: "tool" }
				prebuilt_tool { name: "tool" }
			`,
			expectedToolName: "bin/tool",
		},
		{
			name: "prebuilt preferred",
			bp: `
				tool { name: "tool" }
				prebuilt_tool { name: "tool", prefer: true }
			`,
			expectedToolName: "prebuilt_bin/prebuilt_tool",
		},
		{
			name: "source disabled",
			bp: `
				tool { name: "tool", enabled: false }
				prebuilt_tool { name: "tool" }
      `,
			expectedToolName: "prebuilt_bin/prebuilt_tool",
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			result := prepareForGenRuleTest.RunTestWithBp(t, test.bp+`
				genrule {
					name: "gen",
					tools: ["tool"],
					out: ["foo"],
					cmd: "$(location tool)",
				}
			`)
			gen := result.Module("gen", "").(*Module)
			expectedCmd := "__SBOX_SANDBOX_DIR__/tools/out/" + test.expectedToolName
			android.AssertStringEquals(t, "command", expectedCmd, gen.rawCommands[0])
		})
	}
}

func TestGenruleWithGlobPaths(t *testing.T) {
	testcases := []struct {
		name            string
		bp              string
		additionalFiles android.MockFS
		expectedCmd     string
	}{
		{
			name: "single file in directory with $ sign",
			bp: `
				genrule {
					name: "gen",
					srcs: ["inn*.txt"],
					out: ["out.txt"],
					cmd: "cp $(in) $(out)",
				}
				`,
			additionalFiles: android.MockFS{"inn$1.txt": nil},
			expectedCmd:     "cp 'inn$1.txt' __SBOX_SANDBOX_DIR__/out/out.txt",
		},
		{
			name: "multiple file in directory with $ sign",
			bp: `
				genrule {
					name: "gen",
					srcs: ["inn*.txt"],
					out: ["."],
					cmd: "cp $(in) $(out)",
				}
				`,
			additionalFiles: android.MockFS{"inn$1.txt": nil, "inn$2.txt": nil},
			expectedCmd:     "cp 'inn$1.txt' 'inn$2.txt' __SBOX_SANDBOX_DIR__/out",
		},
		{
			name: "file in directory with other shell unsafe character",
			bp: `
				genrule {
					name: "gen",
					srcs: ["inn*.txt"],
					out: ["out.txt"],
					cmd: "cp $(in) $(out)",
				}
				`,
			additionalFiles: android.MockFS{"inn@1.txt": nil},
			expectedCmd:     "cp 'inn@1.txt' __SBOX_SANDBOX_DIR__/out/out.txt",
		},
		{
			name: "glob location param with filepath containing $",
			bp: `
				genrule {
					name: "gen",
					srcs: ["**/inn*"],
					out: ["."],
					cmd: "cp $(in) $(location **/inn*)",
				}
				`,
			additionalFiles: android.MockFS{"a/inn$1.txt": nil},
			expectedCmd:     "cp 'a/inn$1.txt' 'a/inn$1.txt'",
		},
		{
			name: "glob locations param with filepath containing $",
			bp: `
				genrule {
					name: "gen",
					tool_files: ["**/inn*"],
					out: ["out.txt"],
					cmd: "cp $(locations  **/inn*) $(out)",
				}
				`,
			additionalFiles: android.MockFS{"a/inn$1.txt": nil},
			expectedCmd:     "cp '__SBOX_SANDBOX_DIR__/tools/src/a/inn$1.txt' __SBOX_SANDBOX_DIR__/out/out.txt",
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			result := android.GroupFixturePreparers(
				prepareForGenRuleTest,
				android.FixtureMergeMockFs(test.additionalFiles),
			).RunTestWithBp(t, test.bp)
			gen := result.Module("gen", "").(*Module)
			android.AssertStringEquals(t, "command", test.expectedCmd, gen.rawCommands[0])
		})
	}
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

type prebuiltTestTool struct {
	android.ModuleBase
	prebuilt android.Prebuilt
	testTool
}

func (p *prebuiltTestTool) Name() string {
	return p.prebuilt.Name(p.ModuleBase.Name())
}

func (p *prebuiltTestTool) Prebuilt() *android.Prebuilt {
	return &p.prebuilt
}

func (t *prebuiltTestTool) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	t.outputFile = ctx.InstallFile(android.PathForModuleInstall(ctx, "prebuilt_bin"), ctx.ModuleName(), android.PathForOutput(ctx, ctx.ModuleName()))
}

func prebuiltToolFactory() android.Module {
	module := &prebuiltTestTool{}
	android.InitPrebuiltModuleWithoutSrcs(module)
	android.InitAndroidArchModule(module, android.HostSupported, android.MultilibFirst)
	return module
}

var _ android.HostToolProvider = (*testTool)(nil)
var _ android.HostToolProvider = (*prebuiltTestTool)(nil)

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

type useSource struct {
	android.ModuleBase
	props struct {
		Srcs []string `android:"path"`
	}
	srcs android.Paths
}

func (s *useSource) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	s.srcs = android.PathsForModuleSrc(ctx, s.props.Srcs)
}

func useSourceFactory() android.Module {
	module := &useSource{}
	module.AddProperties(&module.props)
	android.InitAndroidModule(module)
	return module
}
