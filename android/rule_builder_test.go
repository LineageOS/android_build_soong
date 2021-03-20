// Copyright 2019 Google Inc. All rights reserved.
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

package android

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/blueprint"

	"android/soong/shared"
)

func builderContext() BuilderContext {
	return BuilderContextForTesting(TestConfig("out", nil, "", map[string][]byte{
		"ld":      nil,
		"a.o":     nil,
		"b.o":     nil,
		"cp":      nil,
		"a":       nil,
		"b":       nil,
		"ls":      nil,
		"ln":      nil,
		"turbine": nil,
		"java":    nil,
		"javac":   nil,
	}))
}

func ExampleRuleBuilder() {
	ctx := builderContext()

	rule := NewRuleBuilder(pctx, ctx)

	rule.Command().
		Tool(PathForSource(ctx, "ld")).
		Inputs(PathsForTesting("a.o", "b.o")).
		FlagWithOutput("-o ", PathForOutput(ctx, "linked"))
	rule.Command().Text("echo success")

	// To add the command to the build graph:
	// rule.Build("link", "link")

	fmt.Printf("commands: %q\n", strings.Join(rule.Commands(), " && "))
	fmt.Printf("tools: %q\n", rule.Tools())
	fmt.Printf("inputs: %q\n", rule.Inputs())
	fmt.Printf("outputs: %q\n", rule.Outputs())

	// Output:
	// commands: "ld a.o b.o -o out/linked && echo success"
	// tools: ["ld"]
	// inputs: ["a.o" "b.o"]
	// outputs: ["out/linked"]
}

func ExampleRuleBuilder_SymlinkOutputs() {
	ctx := builderContext()

	rule := NewRuleBuilder(pctx, ctx)

	rule.Command().
		Tool(PathForSource(ctx, "ln")).
		FlagWithInput("-s ", PathForTesting("a.o")).
		SymlinkOutput(PathForOutput(ctx, "a"))
	rule.Command().Text("cp out/a out/b").
		ImplicitSymlinkOutput(PathForOutput(ctx, "b"))

	fmt.Printf("commands: %q\n", strings.Join(rule.Commands(), " && "))
	fmt.Printf("tools: %q\n", rule.Tools())
	fmt.Printf("inputs: %q\n", rule.Inputs())
	fmt.Printf("outputs: %q\n", rule.Outputs())
	fmt.Printf("symlink_outputs: %q\n", rule.SymlinkOutputs())

	// Output:
	// commands: "ln -s a.o out/a && cp out/a out/b"
	// tools: ["ln"]
	// inputs: ["a.o"]
	// outputs: ["out/a" "out/b"]
	// symlink_outputs: ["out/a" "out/b"]
}

func ExampleRuleBuilder_Temporary() {
	ctx := builderContext()

	rule := NewRuleBuilder(pctx, ctx)

	rule.Command().
		Tool(PathForSource(ctx, "cp")).
		Input(PathForSource(ctx, "a")).
		Output(PathForOutput(ctx, "b"))
	rule.Command().
		Tool(PathForSource(ctx, "cp")).
		Input(PathForOutput(ctx, "b")).
		Output(PathForOutput(ctx, "c"))
	rule.Temporary(PathForOutput(ctx, "b"))

	fmt.Printf("commands: %q\n", strings.Join(rule.Commands(), " && "))
	fmt.Printf("tools: %q\n", rule.Tools())
	fmt.Printf("inputs: %q\n", rule.Inputs())
	fmt.Printf("outputs: %q\n", rule.Outputs())

	// Output:
	// commands: "cp a out/b && cp out/b out/c"
	// tools: ["cp"]
	// inputs: ["a"]
	// outputs: ["out/c"]
}

func ExampleRuleBuilder_DeleteTemporaryFiles() {
	ctx := builderContext()

	rule := NewRuleBuilder(pctx, ctx)

	rule.Command().
		Tool(PathForSource(ctx, "cp")).
		Input(PathForSource(ctx, "a")).
		Output(PathForOutput(ctx, "b"))
	rule.Command().
		Tool(PathForSource(ctx, "cp")).
		Input(PathForOutput(ctx, "b")).
		Output(PathForOutput(ctx, "c"))
	rule.Temporary(PathForOutput(ctx, "b"))
	rule.DeleteTemporaryFiles()

	fmt.Printf("commands: %q\n", strings.Join(rule.Commands(), " && "))
	fmt.Printf("tools: %q\n", rule.Tools())
	fmt.Printf("inputs: %q\n", rule.Inputs())
	fmt.Printf("outputs: %q\n", rule.Outputs())

	// Output:
	// commands: "cp a out/b && cp out/b out/c && rm -f out/b"
	// tools: ["cp"]
	// inputs: ["a"]
	// outputs: ["out/c"]
}

func ExampleRuleBuilder_Installs() {
	ctx := builderContext()

	rule := NewRuleBuilder(pctx, ctx)

	out := PathForOutput(ctx, "linked")

	rule.Command().
		Tool(PathForSource(ctx, "ld")).
		Inputs(PathsForTesting("a.o", "b.o")).
		FlagWithOutput("-o ", out)
	rule.Install(out, "/bin/linked")
	rule.Install(out, "/sbin/linked")

	fmt.Printf("rule.Installs().String() = %q\n", rule.Installs().String())

	// Output:
	// rule.Installs().String() = "out/linked:/bin/linked out/linked:/sbin/linked"
}

func ExampleRuleBuilderCommand() {
	ctx := builderContext()

	rule := NewRuleBuilder(pctx, ctx)

	// chained
	rule.Command().
		Tool(PathForSource(ctx, "ld")).
		Inputs(PathsForTesting("a.o", "b.o")).
		FlagWithOutput("-o ", PathForOutput(ctx, "linked"))

	// unchained
	cmd := rule.Command()
	cmd.Tool(PathForSource(ctx, "ld"))
	cmd.Inputs(PathsForTesting("a.o", "b.o"))
	cmd.FlagWithOutput("-o ", PathForOutput(ctx, "linked"))

	// mixed:
	cmd = rule.Command().Tool(PathForSource(ctx, "ld"))
	cmd.Inputs(PathsForTesting("a.o", "b.o"))
	cmd.FlagWithOutput("-o ", PathForOutput(ctx, "linked"))
}

func ExampleRuleBuilderCommand_Flag() {
	ctx := builderContext()
	fmt.Println(NewRuleBuilder(pctx, ctx).Command().
		Tool(PathForSource(ctx, "ls")).Flag("-l"))
	// Output:
	// ls -l
}

func ExampleRuleBuilderCommand_Flags() {
	ctx := builderContext()
	fmt.Println(NewRuleBuilder(pctx, ctx).Command().
		Tool(PathForSource(ctx, "ls")).Flags([]string{"-l", "-a"}))
	// Output:
	// ls -l -a
}

func ExampleRuleBuilderCommand_FlagWithArg() {
	ctx := builderContext()
	fmt.Println(NewRuleBuilder(pctx, ctx).Command().
		Tool(PathForSource(ctx, "ls")).
		FlagWithArg("--sort=", "time"))
	// Output:
	// ls --sort=time
}

func ExampleRuleBuilderCommand_FlagForEachArg() {
	ctx := builderContext()
	fmt.Println(NewRuleBuilder(pctx, ctx).Command().
		Tool(PathForSource(ctx, "ls")).
		FlagForEachArg("--sort=", []string{"time", "size"}))
	// Output:
	// ls --sort=time --sort=size
}

func ExampleRuleBuilderCommand_FlagForEachInput() {
	ctx := builderContext()
	fmt.Println(NewRuleBuilder(pctx, ctx).Command().
		Tool(PathForSource(ctx, "turbine")).
		FlagForEachInput("--classpath ", PathsForTesting("a.jar", "b.jar")))
	// Output:
	// turbine --classpath a.jar --classpath b.jar
}

func ExampleRuleBuilderCommand_FlagWithInputList() {
	ctx := builderContext()
	fmt.Println(NewRuleBuilder(pctx, ctx).Command().
		Tool(PathForSource(ctx, "java")).
		FlagWithInputList("-classpath=", PathsForTesting("a.jar", "b.jar"), ":"))
	// Output:
	// java -classpath=a.jar:b.jar
}

func ExampleRuleBuilderCommand_FlagWithInput() {
	ctx := builderContext()
	fmt.Println(NewRuleBuilder(pctx, ctx).Command().
		Tool(PathForSource(ctx, "java")).
		FlagWithInput("-classpath=", PathForSource(ctx, "a")))
	// Output:
	// java -classpath=a
}

func ExampleRuleBuilderCommand_FlagWithList() {
	ctx := builderContext()
	fmt.Println(NewRuleBuilder(pctx, ctx).Command().
		Tool(PathForSource(ctx, "ls")).
		FlagWithList("--sort=", []string{"time", "size"}, ","))
	// Output:
	// ls --sort=time,size
}

func ExampleRuleBuilderCommand_FlagWithRspFileInputList() {
	ctx := builderContext()
	fmt.Println(NewRuleBuilder(pctx, ctx).Command().
		Tool(PathForSource(ctx, "javac")).
		FlagWithRspFileInputList("@", PathForOutput(ctx, "foo.rsp"), PathsForTesting("a.java", "b.java")).
		String())
	// Output:
	// javac @out/foo.rsp
}

func ExampleRuleBuilderCommand_String() {
	ctx := builderContext()
	fmt.Println(NewRuleBuilder(pctx, ctx).Command().
		Text("FOO=foo").
		Text("echo $FOO").
		String())
	// Output:
	// FOO=foo echo $FOO
}

func TestRuleBuilder(t *testing.T) {
	fs := map[string][]byte{
		"dep_fixer":  nil,
		"input":      nil,
		"Implicit":   nil,
		"Input":      nil,
		"OrderOnly":  nil,
		"OrderOnlys": nil,
		"Tool":       nil,
		"input2":     nil,
		"tool2":      nil,
		"input3":     nil,
	}

	pathCtx := PathContextForTesting(TestConfig("out_local", nil, "", fs))
	ctx := builderContextForTests{
		PathContext: pathCtx,
	}

	addCommands := func(rule *RuleBuilder) {
		cmd := rule.Command().
			DepFile(PathForOutput(ctx, "module/DepFile")).
			Flag("Flag").
			FlagWithArg("FlagWithArg=", "arg").
			FlagWithDepFile("FlagWithDepFile=", PathForOutput(ctx, "module/depfile")).
			FlagWithInput("FlagWithInput=", PathForSource(ctx, "input")).
			FlagWithOutput("FlagWithOutput=", PathForOutput(ctx, "module/output")).
			FlagWithRspFileInputList("FlagWithRspFileInputList=", PathForOutput(ctx, "rsp"),
				Paths{
					PathForSource(ctx, "RspInput"),
					PathForOutput(ctx, "other/RspOutput2"),
				}).
			Implicit(PathForSource(ctx, "Implicit")).
			ImplicitDepFile(PathForOutput(ctx, "module/ImplicitDepFile")).
			ImplicitOutput(PathForOutput(ctx, "module/ImplicitOutput")).
			Input(PathForSource(ctx, "Input")).
			Output(PathForOutput(ctx, "module/Output")).
			OrderOnly(PathForSource(ctx, "OrderOnly")).
			SymlinkOutput(PathForOutput(ctx, "module/SymlinkOutput")).
			ImplicitSymlinkOutput(PathForOutput(ctx, "module/ImplicitSymlinkOutput")).
			Text("Text").
			Tool(PathForSource(ctx, "Tool"))

		rule.Command().
			Text("command2").
			DepFile(PathForOutput(ctx, "module/depfile2")).
			Input(PathForSource(ctx, "input2")).
			Output(PathForOutput(ctx, "module/output2")).
			OrderOnlys(PathsForSource(ctx, []string{"OrderOnlys"})).
			Tool(PathForSource(ctx, "tool2"))

		// Test updates to the first command after the second command has been started
		cmd.Text("after command2")
		// Test updating a command when the previous update did not replace the cmd variable
		cmd.Text("old cmd")

		// Test a command that uses the output of a previous command as an input
		rule.Command().
			Text("command3").
			Input(PathForSource(ctx, "input3")).
			Input(PathForOutput(ctx, "module/output2")).
			Output(PathForOutput(ctx, "module/output3")).
			Text(cmd.PathForInput(PathForSource(ctx, "input3"))).
			Text(cmd.PathForOutput(PathForOutput(ctx, "module/output2")))
	}

	wantInputs := PathsForSource(ctx, []string{"Implicit", "Input", "input", "input2", "input3"})
	wantRspFileInputs := Paths{PathForSource(ctx, "RspInput"),
		PathForOutput(ctx, "other/RspOutput2")}
	wantOutputs := PathsForOutput(ctx, []string{
		"module/ImplicitOutput", "module/ImplicitSymlinkOutput", "module/Output", "module/SymlinkOutput",
		"module/output", "module/output2", "module/output3"})
	wantDepFiles := PathsForOutput(ctx, []string{
		"module/DepFile", "module/depfile", "module/ImplicitDepFile", "module/depfile2"})
	wantTools := PathsForSource(ctx, []string{"Tool", "tool2"})
	wantOrderOnlys := PathsForSource(ctx, []string{"OrderOnly", "OrderOnlys"})
	wantSymlinkOutputs := PathsForOutput(ctx, []string{
		"module/ImplicitSymlinkOutput", "module/SymlinkOutput"})

	t.Run("normal", func(t *testing.T) {
		rule := NewRuleBuilder(pctx, ctx)
		addCommands(rule)

		wantCommands := []string{
			"out_local/module/DepFile Flag FlagWithArg=arg FlagWithDepFile=out_local/module/depfile " +
				"FlagWithInput=input FlagWithOutput=out_local/module/output FlagWithRspFileInputList=out_local/rsp " +
				"Input out_local/module/Output out_local/module/SymlinkOutput Text Tool after command2 old cmd",
			"command2 out_local/module/depfile2 input2 out_local/module/output2 tool2",
			"command3 input3 out_local/module/output2 out_local/module/output3 input3 out_local/module/output2",
		}

		wantDepMergerCommand := "out_local/host/" + ctx.Config().PrebuiltOS() + "/bin/dep_fixer " +
			"out_local/module/DepFile out_local/module/depfile out_local/module/ImplicitDepFile out_local/module/depfile2"

		wantRspFileContent := "$in"

		AssertDeepEquals(t, "rule.Commands()", wantCommands, rule.Commands())

		AssertDeepEquals(t, "rule.Inputs()", wantInputs, rule.Inputs())
		AssertDeepEquals(t, "rule.RspfileInputs()", wantRspFileInputs, rule.RspFileInputs())
		AssertDeepEquals(t, "rule.Outputs()", wantOutputs, rule.Outputs())
		AssertDeepEquals(t, "rule.SymlinkOutputs()", wantSymlinkOutputs, rule.SymlinkOutputs())
		AssertDeepEquals(t, "rule.DepFiles()", wantDepFiles, rule.DepFiles())
		AssertDeepEquals(t, "rule.Tools()", wantTools, rule.Tools())
		AssertDeepEquals(t, "rule.OrderOnlys()", wantOrderOnlys, rule.OrderOnlys())

		AssertSame(t, "rule.depFileMergerCmd()", wantDepMergerCommand, rule.depFileMergerCmd(rule.DepFiles()).String())

		AssertSame(t, "rule.composeRspFileContent()", wantRspFileContent, rule.composeRspFileContent(rule.RspFileInputs()))
	})

	t.Run("sbox", func(t *testing.T) {
		rule := NewRuleBuilder(pctx, ctx).Sbox(PathForOutput(ctx, "module"),
			PathForOutput(ctx, "sbox.textproto"))
		addCommands(rule)

		wantCommands := []string{
			"__SBOX_SANDBOX_DIR__/out/DepFile Flag FlagWithArg=arg FlagWithDepFile=__SBOX_SANDBOX_DIR__/out/depfile " +
				"FlagWithInput=input FlagWithOutput=__SBOX_SANDBOX_DIR__/out/output " +
				"FlagWithRspFileInputList=out_local/rsp Input __SBOX_SANDBOX_DIR__/out/Output " +
				"__SBOX_SANDBOX_DIR__/out/SymlinkOutput Text Tool after command2 old cmd",
			"command2 __SBOX_SANDBOX_DIR__/out/depfile2 input2 __SBOX_SANDBOX_DIR__/out/output2 tool2",
			"command3 input3 __SBOX_SANDBOX_DIR__/out/output2 __SBOX_SANDBOX_DIR__/out/output3 input3 __SBOX_SANDBOX_DIR__/out/output2",
		}

		wantDepMergerCommand := "out_local/host/" + ctx.Config().PrebuiltOS() + "/bin/dep_fixer __SBOX_SANDBOX_DIR__/out/DepFile __SBOX_SANDBOX_DIR__/out/depfile __SBOX_SANDBOX_DIR__/out/ImplicitDepFile __SBOX_SANDBOX_DIR__/out/depfile2"

		wantRspFileContent := "$in"

		AssertDeepEquals(t, "rule.Commands()", wantCommands, rule.Commands())

		AssertDeepEquals(t, "rule.Inputs()", wantInputs, rule.Inputs())
		AssertDeepEquals(t, "rule.RspfileInputs()", wantRspFileInputs, rule.RspFileInputs())
		AssertDeepEquals(t, "rule.Outputs()", wantOutputs, rule.Outputs())
		AssertDeepEquals(t, "rule.SymlinkOutputs()", wantSymlinkOutputs, rule.SymlinkOutputs())
		AssertDeepEquals(t, "rule.DepFiles()", wantDepFiles, rule.DepFiles())
		AssertDeepEquals(t, "rule.Tools()", wantTools, rule.Tools())
		AssertDeepEquals(t, "rule.OrderOnlys()", wantOrderOnlys, rule.OrderOnlys())

		AssertSame(t, "rule.depFileMergerCmd()", wantDepMergerCommand, rule.depFileMergerCmd(rule.DepFiles()).String())

		AssertSame(t, "rule.composeRspFileContent()", wantRspFileContent, rule.composeRspFileContent(rule.RspFileInputs()))
	})

	t.Run("sbox tools", func(t *testing.T) {
		rule := NewRuleBuilder(pctx, ctx).Sbox(PathForOutput(ctx, "module"),
			PathForOutput(ctx, "sbox.textproto")).SandboxTools()
		addCommands(rule)

		wantCommands := []string{
			"__SBOX_SANDBOX_DIR__/out/DepFile Flag FlagWithArg=arg FlagWithDepFile=__SBOX_SANDBOX_DIR__/out/depfile " +
				"FlagWithInput=input FlagWithOutput=__SBOX_SANDBOX_DIR__/out/output " +
				"FlagWithRspFileInputList=out_local/rsp Input __SBOX_SANDBOX_DIR__/out/Output " +
				"__SBOX_SANDBOX_DIR__/out/SymlinkOutput Text __SBOX_SANDBOX_DIR__/tools/src/Tool after command2 old cmd",
			"command2 __SBOX_SANDBOX_DIR__/out/depfile2 input2 __SBOX_SANDBOX_DIR__/out/output2 __SBOX_SANDBOX_DIR__/tools/src/tool2",
			"command3 input3 __SBOX_SANDBOX_DIR__/out/output2 __SBOX_SANDBOX_DIR__/out/output3 input3 __SBOX_SANDBOX_DIR__/out/output2",
		}

		wantDepMergerCommand := "__SBOX_SANDBOX_DIR__/tools/out/bin/dep_fixer __SBOX_SANDBOX_DIR__/out/DepFile __SBOX_SANDBOX_DIR__/out/depfile __SBOX_SANDBOX_DIR__/out/ImplicitDepFile __SBOX_SANDBOX_DIR__/out/depfile2"

		wantRspFileContent := "$in"

		AssertDeepEquals(t, "rule.Commands()", wantCommands, rule.Commands())

		AssertDeepEquals(t, "rule.Inputs()", wantInputs, rule.Inputs())
		AssertDeepEquals(t, "rule.RspfileInputs()", wantRspFileInputs, rule.RspFileInputs())
		AssertDeepEquals(t, "rule.Outputs()", wantOutputs, rule.Outputs())
		AssertDeepEquals(t, "rule.SymlinkOutputs()", wantSymlinkOutputs, rule.SymlinkOutputs())
		AssertDeepEquals(t, "rule.DepFiles()", wantDepFiles, rule.DepFiles())
		AssertDeepEquals(t, "rule.Tools()", wantTools, rule.Tools())
		AssertDeepEquals(t, "rule.OrderOnlys()", wantOrderOnlys, rule.OrderOnlys())

		AssertSame(t, "rule.depFileMergerCmd()", wantDepMergerCommand, rule.depFileMergerCmd(rule.DepFiles()).String())

		AssertSame(t, "rule.composeRspFileContent()", wantRspFileContent, rule.composeRspFileContent(rule.RspFileInputs()))
	})
}

func testRuleBuilderFactory() Module {
	module := &testRuleBuilderModule{}
	module.AddProperties(&module.properties)
	InitAndroidModule(module)
	return module
}

type testRuleBuilderModule struct {
	ModuleBase
	properties struct {
		Srcs []string

		Restat bool
		Sbox   bool
	}
}

func (t *testRuleBuilderModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	in := PathsForSource(ctx, t.properties.Srcs)
	out := PathForModuleOut(ctx, "gen", ctx.ModuleName())
	outDep := PathForModuleOut(ctx, "gen", ctx.ModuleName()+".d")
	outDir := PathForModuleOut(ctx, "gen")
	manifestPath := PathForModuleOut(ctx, "sbox.textproto")

	testRuleBuilder_Build(ctx, in, out, outDep, outDir, manifestPath, t.properties.Restat, t.properties.Sbox)
}

type testRuleBuilderSingleton struct{}

func testRuleBuilderSingletonFactory() Singleton {
	return &testRuleBuilderSingleton{}
}

func (t *testRuleBuilderSingleton) GenerateBuildActions(ctx SingletonContext) {
	in := PathForSource(ctx, "bar")
	out := PathForOutput(ctx, "singleton/gen/baz")
	outDep := PathForOutput(ctx, "singleton/gen/baz.d")
	outDir := PathForOutput(ctx, "singleton/gen")
	manifestPath := PathForOutput(ctx, "singleton/sbox.textproto")
	testRuleBuilder_Build(ctx, Paths{in}, out, outDep, outDir, manifestPath, true, false)
}

func testRuleBuilder_Build(ctx BuilderContext, in Paths, out, outDep, outDir, manifestPath WritablePath, restat, sbox bool) {
	rule := NewRuleBuilder(pctx, ctx)

	if sbox {
		rule.Sbox(outDir, manifestPath)
	}

	rule.Command().Tool(PathForSource(ctx, "cp")).Inputs(in).Output(out).ImplicitDepFile(outDep)

	if restat {
		rule.Restat()
	}

	rule.Build("rule", "desc")
}

var prepareForRuleBuilderTest = FixtureRegisterWithContext(func(ctx RegistrationContext) {
	ctx.RegisterModuleType("rule_builder_test", testRuleBuilderFactory)
	ctx.RegisterSingletonType("rule_builder_test", testRuleBuilderSingletonFactory)
})

func TestRuleBuilder_Build(t *testing.T) {
	fs := MockFS{
		"bar": nil,
		"cp":  nil,
	}

	bp := `
		rule_builder_test {
			name: "foo",
			srcs: ["bar"],
			restat: true,
		}
		rule_builder_test {
			name: "foo_sbox",
			srcs: ["bar"],
			sbox: true,
		}
	`

	result := emptyTestFixtureFactory.RunTest(t,
		prepareForRuleBuilderTest,
		FixtureWithRootAndroidBp(bp),
		fs.AddToFixture(),
	)

	check := func(t *testing.T, params TestingBuildParams, wantCommand, wantOutput, wantDepfile string, wantRestat bool, extraImplicits, extraCmdDeps []string) {
		t.Helper()
		command := params.RuleParams.Command
		re := regexp.MustCompile(" # hash of input list: [a-z0-9]*$")
		command = re.ReplaceAllLiteralString(command, "")

		AssertStringEquals(t, "RuleParams.Command", wantCommand, command)

		wantDeps := append([]string{"cp"}, extraCmdDeps...)
		AssertArrayString(t, "RuleParams.CommandDeps", wantDeps, params.RuleParams.CommandDeps)

		AssertBoolEquals(t, "RuleParams.Restat", wantRestat, params.RuleParams.Restat)

		wantImplicits := append([]string{"bar"}, extraImplicits...)
		AssertArrayString(t, "Implicits", wantImplicits, params.Implicits.Strings())

		AssertStringEquals(t, "Output", wantOutput, params.Output.String())

		if len(params.ImplicitOutputs) != 0 {
			t.Errorf("want ImplicitOutputs = [], got %q", params.ImplicitOutputs.Strings())
		}

		AssertStringEquals(t, "Depfile", wantDepfile, params.Depfile.String())

		if params.Deps != blueprint.DepsGCC {
			t.Errorf("want Deps = %q, got %q", blueprint.DepsGCC, params.Deps)
		}
	}

	buildDir := result.Config.BuildDir()

	t.Run("module", func(t *testing.T) {
		outFile := filepath.Join(buildDir, ".intermediates", "foo", "gen", "foo")
		check(t, result.ModuleForTests("foo", "").Rule("rule"),
			"cp bar "+outFile,
			outFile, outFile+".d", true, nil, nil)
	})
	t.Run("sbox", func(t *testing.T) {
		outDir := filepath.Join(buildDir, ".intermediates", "foo_sbox")
		outFile := filepath.Join(outDir, "gen/foo_sbox")
		depFile := filepath.Join(outDir, "gen/foo_sbox.d")
		manifest := filepath.Join(outDir, "sbox.textproto")
		sbox := filepath.Join(buildDir, "host", result.Config.PrebuiltOS(), "bin/sbox")
		sandboxPath := shared.TempDirForOutDir(buildDir)

		cmd := `rm -rf ` + outDir + `/gen && ` +
			sbox + ` --sandbox-path ` + sandboxPath + ` --manifest ` + manifest

		check(t, result.ModuleForTests("foo_sbox", "").Output("gen/foo_sbox"),
			cmd, outFile, depFile, false, []string{manifest}, []string{sbox})
	})
	t.Run("singleton", func(t *testing.T) {
		outFile := filepath.Join(buildDir, "singleton/gen/baz")
		check(t, result.SingletonForTests("rule_builder_test").Rule("rule"),
			"cp bar "+outFile, outFile, outFile+".d", true, nil, nil)
	})
}

func TestRuleBuilderHashInputs(t *testing.T) {
	// The basic idea here is to verify that the command (in the case of a
	// non-sbox rule) or the sbox textproto manifest contain a hash of the
	// inputs.

	// By including a hash of the inputs, we cause the rule to re-run if
	// the list of inputs changes because the command line or a dependency
	// changes.

	bp := `
			rule_builder_test {
				name: "hash0",
				srcs: ["in1.txt", "in2.txt"],
			}
			rule_builder_test {
				name: "hash0_sbox",
				srcs: ["in1.txt", "in2.txt"],
				sbox: true,
			}
			rule_builder_test {
				name: "hash1",
				srcs: ["in1.txt", "in2.txt", "in3.txt"],
			}
			rule_builder_test {
				name: "hash1_sbox",
				srcs: ["in1.txt", "in2.txt", "in3.txt"],
				sbox: true,
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
	}

	result := emptyTestFixtureFactory.RunTest(t,
		prepareForRuleBuilderTest,
		FixtureWithRootAndroidBp(bp),
	)

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			t.Run("sbox", func(t *testing.T) {
				gen := result.ModuleForTests(test.name+"_sbox", "")
				manifest := RuleBuilderSboxProtoForTests(t, gen.Output("sbox.textproto"))
				hash := manifest.Commands[0].GetInputHash()

				AssertStringEquals(t, "hash", test.expectedHash, hash)
			})
			t.Run("", func(t *testing.T) {
				gen := result.ModuleForTests(test.name+"", "")
				command := gen.Output("gen/" + test.name).RuleParams.Command
				if g, w := command, " # hash of input list: "+test.expectedHash; !strings.HasSuffix(g, w) {
					t.Errorf("Expected command line to end with %q, got %q", w, g)
				}
			})
		})
	}
}
