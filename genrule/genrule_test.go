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
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"

	"android/soong/android"

	"github.com/google/blueprint/proptools"
)

var buildDir string

func setUp() {
	var err error
	buildDir, err = ioutil.TempDir("", "genrule_test")
	if err != nil {
		panic(err)
	}
}

func tearDown() {
	os.RemoveAll(buildDir)
}

func TestMain(m *testing.M) {
	run := func() int {
		setUp()
		defer tearDown()

		return m.Run()
	}

	os.Exit(run())
}

func testContext(config android.Config) *android.TestContext {

	ctx := android.NewTestArchContext(config)
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	ctx.RegisterModuleType("tool", toolFactory)

	RegisterGenruleBuildComponents(ctx)

	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	ctx.Register()

	return ctx
}

func testConfig(bp string, fs map[string][]byte) android.Config {
	bp += `
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

	mockFS := map[string][]byte{
		"tool":       nil,
		"tool_file1": nil,
		"tool_file2": nil,
		"in1":        nil,
		"in2":        nil,
		"in1.txt":    nil,
		"in2.txt":    nil,
		"in3.txt":    nil,
	}

	for k, v := range fs {
		mockFS[k] = v
	}

	return android.TestArchConfig(buildDir, nil, bp, mockFS)
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
			expect: "out/tool > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "empty location tool2",
			prop: `
				tools: [":tool"],
				out: ["out"],
				cmd: "$(location) > $(out)",
			`,
			expect: "out/tool > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "empty location tool file",
			prop: `
				tool_files: ["tool_file1"],
				out: ["out"],
				cmd: "$(location) > $(out)",
			`,
			expect: "tool_file1 > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "empty location tool file fg",
			prop: `
				tool_files: [":1tool_file"],
				out: ["out"],
				cmd: "$(location) > $(out)",
			`,
			expect: "tool_file1 > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "empty location tool and tool file",
			prop: `
				tools: ["tool"],
				tool_files: ["tool_file1"],
				out: ["out"],
				cmd: "$(location) > $(out)",
			`,
			expect: "out/tool > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "tool",
			prop: `
				tools: ["tool"],
				out: ["out"],
				cmd: "$(location tool) > $(out)",
			`,
			expect: "out/tool > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "tool2",
			prop: `
				tools: [":tool"],
				out: ["out"],
				cmd: "$(location :tool) > $(out)",
			`,
			expect: "out/tool > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "tool file",
			prop: `
				tool_files: ["tool_file1"],
				out: ["out"],
				cmd: "$(location tool_file1) > $(out)",
			`,
			expect: "tool_file1 > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "tool file fg",
			prop: `
				tool_files: [":1tool_file"],
				out: ["out"],
				cmd: "$(location :1tool_file) > $(out)",
			`,
			expect: "tool_file1 > __SBOX_SANDBOX_DIR__/out/out",
		},
		{
			name: "tool files",
			prop: `
				tool_files: [":tool_files"],
				out: ["out"],
				cmd: "$(locations :tool_files) > $(out)",
			`,
			expect: "tool_file1 tool_file2 > __SBOX_SANDBOX_DIR__/out/out",
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

			config := testConfig(bp, nil)
			config.TestProductVariables.Allow_missing_dependencies = proptools.BoolPtr(test.allowMissingDependencies)

			ctx := testContext(config)
			ctx.SetAllowMissingDependencies(test.allowMissingDependencies)

			_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
			if errs == nil {
				_, errs = ctx.PrepareBuildActions(config)
			}
			if errs == nil && test.err != "" {
				t.Fatalf("want error %q, got no error", test.err)
			} else if errs != nil && test.err == "" {
				android.FailIfErrored(t, errs)
			} else if test.err != "" {
				if len(errs) != 1 {
					t.Errorf("want 1 error, got %d errors:", len(errs))
					for _, err := range errs {
						t.Errorf("   %s", err.Error())
					}
					t.FailNow()
				}
				if !strings.Contains(errs[0].Error(), test.err) {
					t.Fatalf("want %q, got %q", test.err, errs[0].Error())
				}
				return
			}

			gen := ctx.ModuleForTests("gen", "").Module().(*Module)
			if g, w := gen.rawCommands[0], test.expect; w != g {
				t.Errorf("want %q, got %q", w, g)
			}
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

	config := testConfig(bp, nil)
	ctx := testContext(config)
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	if errs == nil {
		_, errs = ctx.PrepareBuildActions(config)
	}
	if errs != nil {
		t.Fatal(errs)
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			gen := ctx.ModuleForTests(test.name, "")
			manifest := android.RuleBuilderSboxProtoForTests(t, gen.Output("genrule.sbox.textproto"))
			hash := manifest.Commands[0].GetInputHash()

			if g, w := hash, test.expectedHash; g != w {
				t.Errorf("Expected has %q, got %q", w, g)
			}
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
				"bash -c 'out/tool in1.txt > __SBOX_SANDBOX_DIR__/out/in1.h' && bash -c 'out/tool in2.txt > __SBOX_SANDBOX_DIR__/out/in2.h'",
			},
			deps:  []string{buildDir + "/.intermediates/gen/gen/gensrcs/in1.h", buildDir + "/.intermediates/gen/gen/gensrcs/in2.h"},
			files: []string{buildDir + "/.intermediates/gen/gen/gensrcs/in1.h", buildDir + "/.intermediates/gen/gen/gensrcs/in2.h"},
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
				"bash -c 'out/tool in1.txt > __SBOX_SANDBOX_DIR__/out/in1.h' && bash -c 'out/tool in2.txt > __SBOX_SANDBOX_DIR__/out/in2.h'",
				"bash -c 'out/tool in3.txt > __SBOX_SANDBOX_DIR__/out/in3.h'",
			},
			deps:  []string{buildDir + "/.intermediates/gen/gen/gensrcs/in1.h", buildDir + "/.intermediates/gen/gen/gensrcs/in2.h", buildDir + "/.intermediates/gen/gen/gensrcs/in3.h"},
			files: []string{buildDir + "/.intermediates/gen/gen/gensrcs/in1.h", buildDir + "/.intermediates/gen/gen/gensrcs/in2.h", buildDir + "/.intermediates/gen/gen/gensrcs/in3.h"},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			bp := "gensrcs {\n"
			bp += `name: "gen",` + "\n"
			bp += `output_extension: "h",` + "\n"
			bp += test.prop
			bp += "}\n"

			config := testConfig(bp, nil)
			ctx := testContext(config)

			_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
			if errs == nil {
				_, errs = ctx.PrepareBuildActions(config)
			}
			if errs == nil && test.err != "" {
				t.Fatalf("want error %q, got no error", test.err)
			} else if errs != nil && test.err == "" {
				android.FailIfErrored(t, errs)
			} else if test.err != "" {
				if len(errs) != 1 {
					t.Errorf("want 1 error, got %d errors:", len(errs))
					for _, err := range errs {
						t.Errorf("   %s", err.Error())
					}
					t.FailNow()
				}
				if !strings.Contains(errs[0].Error(), test.err) {
					t.Fatalf("want %q, got %q", test.err, errs[0].Error())
				}
				return
			}

			gen := ctx.ModuleForTests("gen", "").Module().(*Module)
			if g, w := gen.rawCommands, test.cmds; !reflect.DeepEqual(w, g) {
				t.Errorf("want %q, got %q", w, g)
			}

			if g, w := gen.outputDeps.Strings(), test.deps; !reflect.DeepEqual(w, g) {
				t.Errorf("want deps %q, got %q", w, g)
			}

			if g, w := gen.outputFiles.Strings(), test.files; !reflect.DeepEqual(w, g) {
				t.Errorf("want files %q, got %q", w, g)
			}
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
	config := testConfig(bp, nil)
	ctx := testContext(config)
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	if errs == nil {
		_, errs = ctx.PrepareBuildActions(config)
	}
	if errs != nil {
		t.Fatal(errs)
	}
	gen := ctx.ModuleForTests("gen", "").Module().(*Module)

	expectedCmd := "cp in1 __SBOX_SANDBOX_DIR__/out/out"
	if gen.rawCommands[0] != expectedCmd {
		t.Errorf("Expected cmd: %q, actual: %q", expectedCmd, gen.rawCommands[0])
	}

	expectedSrcs := []string{"in1"}
	if !reflect.DeepEqual(expectedSrcs, gen.properties.Srcs) {
		t.Errorf("Expected srcs: %q, actual: %q", expectedSrcs, gen.properties.Srcs)
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

	config := testConfig(bp, nil)
	config.BazelContext = android.MockBazelContext{
		AllFiles: map[string][]string{
			"//foo/bar:bar": []string{"bazelone.txt", "bazeltwo.txt"}}}

	ctx := testContext(config)
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	if errs == nil {
		_, errs = ctx.PrepareBuildActions(config)
	}
	if errs != nil {
		t.Fatal(errs)
	}
	gen := ctx.ModuleForTests("foo", "").Module().(*Module)

	expectedOutputFiles := []string{"outputbase/execroot/__main__/bazelone.txt",
		"outputbase/execroot/__main__/bazeltwo.txt"}
	if !reflect.DeepEqual(gen.outputFiles.Strings(), expectedOutputFiles) {
		t.Errorf("Expected output files: %q, actual: %q", expectedOutputFiles, gen.outputFiles)
	}
	if !reflect.DeepEqual(gen.outputDeps.Strings(), expectedOutputFiles) {
		t.Errorf("Expected output deps: %q, actual: %q", expectedOutputFiles, gen.outputDeps)
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
	t.outputFile = android.PathForTesting("out", ctx.ModuleName())
}

func (t *testTool) HostToolPath() android.OptionalPath {
	return android.OptionalPathForPath(t.outputFile)
}

var _ android.HostToolProvider = (*testTool)(nil)
