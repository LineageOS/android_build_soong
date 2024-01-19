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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/blueprint"

	"android/soong/shared"
)

var (
	pctx_ruleBuilderTest           = NewPackageContext("android/soong/rule_builder")
	pctx_ruleBuilderTestSubContext = NewPackageContext("android/soong/rule_builder/config")
)

func init() {
	pctx_ruleBuilderTest.Import("android/soong/rule_builder/config")
	pctx_ruleBuilderTest.StaticVariable("cmdFlags", "${config.ConfigFlags}")
	pctx_ruleBuilderTestSubContext.StaticVariable("ConfigFlags", "--some-clang-flag")
}

func builderContext() BuilderContext {
	return BuilderContextForTesting(TestConfig("out", nil, "", map[string][]byte{
		"ld":      nil,
		"a.o":     nil,
		"b.o":     nil,
		"cp":      nil,
		"a":       nil,
		"b":       nil,
		"ls":      nil,
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
	// commands: "ld a.o b.o -o out/soong/linked && echo success"
	// tools: ["ld"]
	// inputs: ["a.o" "b.o"]
	// outputs: ["out/soong/linked"]
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
	// commands: "cp a out/soong/b && cp out/soong/b out/soong/c"
	// tools: ["cp"]
	// inputs: ["a"]
	// outputs: ["out/soong/c"]
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
	// commands: "cp a out/soong/b && cp out/soong/b out/soong/c && rm -f out/soong/b"
	// tools: ["cp"]
	// inputs: ["a"]
	// outputs: ["out/soong/c"]
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
	// rule.Installs().String() = "out/soong/linked:/bin/linked out/soong/linked:/sbin/linked"
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
	// javac @out/soong/foo.rsp
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
			Validation(PathForSource(ctx, "Validation")).
			Text("Text").
			Tool(PathForSource(ctx, "Tool"))

		rule.Command().
			Text("command2").
			DepFile(PathForOutput(ctx, "module/depfile2")).
			Input(PathForSource(ctx, "input2")).
			Output(PathForOutput(ctx, "module/output2")).
			OrderOnlys(PathsForSource(ctx, []string{"OrderOnlys"})).
			Validations(PathsForSource(ctx, []string{"Validations"})).
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
		"module/ImplicitOutput", "module/Output", "module/output", "module/output2",
		"module/output3"})
	wantDepFiles := PathsForOutput(ctx, []string{
		"module/DepFile", "module/depfile", "module/ImplicitDepFile", "module/depfile2"})
	wantTools := PathsForSource(ctx, []string{"Tool", "tool2"})
	wantOrderOnlys := PathsForSource(ctx, []string{"OrderOnly", "OrderOnlys"})
	wantValidations := PathsForSource(ctx, []string{"Validation", "Validations"})

	t.Run("normal", func(t *testing.T) {
		rule := NewRuleBuilder(pctx, ctx)
		addCommands(rule)

		wantCommands := []string{
			"out_local/soong/module/DepFile Flag FlagWithArg=arg FlagWithDepFile=out_local/soong/module/depfile " +
				"FlagWithInput=input FlagWithOutput=out_local/soong/module/output FlagWithRspFileInputList=out_local/soong/rsp " +
				"Input out_local/soong/module/Output Text Tool after command2 old cmd",
			"command2 out_local/soong/module/depfile2 input2 out_local/soong/module/output2 tool2",
			"command3 input3 out_local/soong/module/output2 out_local/soong/module/output3 input3 out_local/soong/module/output2",
		}

		wantDepMergerCommand := "out_local/soong/host/" + ctx.Config().PrebuiltOS() + "/bin/dep_fixer " +
			"out_local/soong/module/DepFile out_local/soong/module/depfile out_local/soong/module/ImplicitDepFile out_local/soong/module/depfile2"

		AssertDeepEquals(t, "rule.Commands()", wantCommands, rule.Commands())

		AssertDeepEquals(t, "rule.Inputs()", wantInputs, rule.Inputs())
		AssertDeepEquals(t, "rule.RspfileInputs()", wantRspFileInputs, rule.RspFileInputs())
		AssertDeepEquals(t, "rule.Outputs()", wantOutputs, rule.Outputs())
		AssertDeepEquals(t, "rule.DepFiles()", wantDepFiles, rule.DepFiles())
		AssertDeepEquals(t, "rule.Tools()", wantTools, rule.Tools())
		AssertDeepEquals(t, "rule.OrderOnlys()", wantOrderOnlys, rule.OrderOnlys())
		AssertDeepEquals(t, "rule.Validations()", wantValidations, rule.Validations())

		AssertSame(t, "rule.depFileMergerCmd()", wantDepMergerCommand, rule.depFileMergerCmd(rule.DepFiles()).String())
	})

	t.Run("sbox", func(t *testing.T) {
		rule := NewRuleBuilder(pctx, ctx).Sbox(PathForOutput(ctx, "module"),
			PathForOutput(ctx, "sbox.textproto"))
		addCommands(rule)

		wantCommands := []string{
			"__SBOX_SANDBOX_DIR__/out/DepFile Flag FlagWithArg=arg FlagWithDepFile=__SBOX_SANDBOX_DIR__/out/depfile " +
				"FlagWithInput=input FlagWithOutput=__SBOX_SANDBOX_DIR__/out/output " +
				"FlagWithRspFileInputList=out_local/soong/rsp Input __SBOX_SANDBOX_DIR__/out/Output " +
				"Text Tool after command2 old cmd",
			"command2 __SBOX_SANDBOX_DIR__/out/depfile2 input2 __SBOX_SANDBOX_DIR__/out/output2 tool2",
			"command3 input3 __SBOX_SANDBOX_DIR__/out/output2 __SBOX_SANDBOX_DIR__/out/output3 input3 __SBOX_SANDBOX_DIR__/out/output2",
		}

		wantDepMergerCommand := "out_local/soong/host/" + ctx.Config().PrebuiltOS() + "/bin/dep_fixer __SBOX_SANDBOX_DIR__/out/DepFile __SBOX_SANDBOX_DIR__/out/depfile __SBOX_SANDBOX_DIR__/out/ImplicitDepFile __SBOX_SANDBOX_DIR__/out/depfile2"

		AssertDeepEquals(t, "rule.Commands()", wantCommands, rule.Commands())

		AssertDeepEquals(t, "rule.Inputs()", wantInputs, rule.Inputs())
		AssertDeepEquals(t, "rule.RspfileInputs()", wantRspFileInputs, rule.RspFileInputs())
		AssertDeepEquals(t, "rule.Outputs()", wantOutputs, rule.Outputs())
		AssertDeepEquals(t, "rule.DepFiles()", wantDepFiles, rule.DepFiles())
		AssertDeepEquals(t, "rule.Tools()", wantTools, rule.Tools())
		AssertDeepEquals(t, "rule.OrderOnlys()", wantOrderOnlys, rule.OrderOnlys())
		AssertDeepEquals(t, "rule.Validations()", wantValidations, rule.Validations())

		AssertSame(t, "rule.depFileMergerCmd()", wantDepMergerCommand, rule.depFileMergerCmd(rule.DepFiles()).String())
	})

	t.Run("sbox tools", func(t *testing.T) {
		rule := NewRuleBuilder(pctx, ctx).Sbox(PathForOutput(ctx, "module"),
			PathForOutput(ctx, "sbox.textproto")).SandboxTools()
		addCommands(rule)

		wantCommands := []string{
			"__SBOX_SANDBOX_DIR__/out/DepFile Flag FlagWithArg=arg FlagWithDepFile=__SBOX_SANDBOX_DIR__/out/depfile " +
				"FlagWithInput=input FlagWithOutput=__SBOX_SANDBOX_DIR__/out/output " +
				"FlagWithRspFileInputList=out_local/soong/rsp Input __SBOX_SANDBOX_DIR__/out/Output " +
				"Text __SBOX_SANDBOX_DIR__/tools/src/Tool after command2 old cmd",
			"command2 __SBOX_SANDBOX_DIR__/out/depfile2 input2 __SBOX_SANDBOX_DIR__/out/output2 __SBOX_SANDBOX_DIR__/tools/src/tool2",
			"command3 input3 __SBOX_SANDBOX_DIR__/out/output2 __SBOX_SANDBOX_DIR__/out/output3 input3 __SBOX_SANDBOX_DIR__/out/output2",
		}

		wantDepMergerCommand := "__SBOX_SANDBOX_DIR__/tools/out/bin/dep_fixer __SBOX_SANDBOX_DIR__/out/DepFile __SBOX_SANDBOX_DIR__/out/depfile __SBOX_SANDBOX_DIR__/out/ImplicitDepFile __SBOX_SANDBOX_DIR__/out/depfile2"

		AssertDeepEquals(t, "rule.Commands()", wantCommands, rule.Commands())

		AssertDeepEquals(t, "rule.Inputs()", wantInputs, rule.Inputs())
		AssertDeepEquals(t, "rule.RspfileInputs()", wantRspFileInputs, rule.RspFileInputs())
		AssertDeepEquals(t, "rule.Outputs()", wantOutputs, rule.Outputs())
		AssertDeepEquals(t, "rule.DepFiles()", wantDepFiles, rule.DepFiles())
		AssertDeepEquals(t, "rule.Tools()", wantTools, rule.Tools())
		AssertDeepEquals(t, "rule.OrderOnlys()", wantOrderOnlys, rule.OrderOnlys())
		AssertDeepEquals(t, "rule.Validations()", wantValidations, rule.Validations())

		AssertSame(t, "rule.depFileMergerCmd()", wantDepMergerCommand, rule.depFileMergerCmd(rule.DepFiles()).String())
	})

	t.Run("sbox inputs", func(t *testing.T) {
		rule := NewRuleBuilder(pctx, ctx).Sbox(PathForOutput(ctx, "module"),
			PathForOutput(ctx, "sbox.textproto")).SandboxInputs()
		addCommands(rule)

		wantCommands := []string{
			"__SBOX_SANDBOX_DIR__/out/DepFile Flag FlagWithArg=arg FlagWithDepFile=__SBOX_SANDBOX_DIR__/out/depfile " +
				"FlagWithInput=input FlagWithOutput=__SBOX_SANDBOX_DIR__/out/output " +
				"FlagWithRspFileInputList=__SBOX_SANDBOX_DIR__/out/soong/rsp Input __SBOX_SANDBOX_DIR__/out/Output " +
				"Text __SBOX_SANDBOX_DIR__/tools/src/Tool after command2 old cmd",
			"command2 __SBOX_SANDBOX_DIR__/out/depfile2 input2 __SBOX_SANDBOX_DIR__/out/output2 __SBOX_SANDBOX_DIR__/tools/src/tool2",
			"command3 input3 __SBOX_SANDBOX_DIR__/out/output2 __SBOX_SANDBOX_DIR__/out/output3 input3 __SBOX_SANDBOX_DIR__/out/output2",
		}

		wantDepMergerCommand := "__SBOX_SANDBOX_DIR__/tools/out/bin/dep_fixer __SBOX_SANDBOX_DIR__/out/DepFile __SBOX_SANDBOX_DIR__/out/depfile __SBOX_SANDBOX_DIR__/out/ImplicitDepFile __SBOX_SANDBOX_DIR__/out/depfile2"

		AssertDeepEquals(t, "rule.Commands()", wantCommands, rule.Commands())

		AssertDeepEquals(t, "rule.Inputs()", wantInputs, rule.Inputs())
		AssertDeepEquals(t, "rule.RspfileInputs()", wantRspFileInputs, rule.RspFileInputs())
		AssertDeepEquals(t, "rule.Outputs()", wantOutputs, rule.Outputs())
		AssertDeepEquals(t, "rule.DepFiles()", wantDepFiles, rule.DepFiles())
		AssertDeepEquals(t, "rule.Tools()", wantTools, rule.Tools())
		AssertDeepEquals(t, "rule.OrderOnlys()", wantOrderOnlys, rule.OrderOnlys())
		AssertDeepEquals(t, "rule.Validations()", wantValidations, rule.Validations())

		AssertSame(t, "rule.depFileMergerCmd()", wantDepMergerCommand, rule.depFileMergerCmd(rule.DepFiles()).String())
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
		Srcs  []string
		Flags []string

		Restat              bool
		Sbox                bool
		Sbox_inputs         bool
		Unescape_ninja_vars bool
	}
}

func (t *testRuleBuilderModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	in := PathsForSource(ctx, t.properties.Srcs)
	implicit := PathForSource(ctx, "implicit")
	orderOnly := PathForSource(ctx, "orderonly")
	validation := PathForSource(ctx, "validation")
	out := PathForModuleOut(ctx, "gen", ctx.ModuleName())
	outDep := PathForModuleOut(ctx, "gen", ctx.ModuleName()+".d")
	outDir := PathForModuleOut(ctx, "gen")
	rspFile := PathForModuleOut(ctx, "rsp")
	rspFile2 := PathForModuleOut(ctx, "rsp2")
	rspFileContents := PathsForSource(ctx, []string{"rsp_in"})
	rspFileContents2 := PathsForSource(ctx, []string{"rsp_in2"})
	manifestPath := PathForModuleOut(ctx, "sbox.textproto")

	testRuleBuilder_Build(ctx, in, implicit, orderOnly, validation, t.properties.Flags,
		out, outDep, outDir,
		manifestPath, t.properties.Restat, t.properties.Sbox, t.properties.Sbox_inputs, t.properties.Unescape_ninja_vars,
		rspFile, rspFileContents, rspFile2, rspFileContents2)
}

type testRuleBuilderSingleton struct{}

func testRuleBuilderSingletonFactory() Singleton {
	return &testRuleBuilderSingleton{}
}

func (t *testRuleBuilderSingleton) GenerateBuildActions(ctx SingletonContext) {
	in := PathsForSource(ctx, []string{"in"})
	implicit := PathForSource(ctx, "implicit")
	orderOnly := PathForSource(ctx, "orderonly")
	validation := PathForSource(ctx, "validation")
	out := PathForOutput(ctx, "singleton/gen/baz")
	outDep := PathForOutput(ctx, "singleton/gen/baz.d")
	outDir := PathForOutput(ctx, "singleton/gen")
	rspFile := PathForOutput(ctx, "singleton/rsp")
	rspFile2 := PathForOutput(ctx, "singleton/rsp2")
	rspFileContents := PathsForSource(ctx, []string{"rsp_in"})
	rspFileContents2 := PathsForSource(ctx, []string{"rsp_in2"})
	manifestPath := PathForOutput(ctx, "singleton/sbox.textproto")

	testRuleBuilder_Build(ctx, in, implicit, orderOnly, validation, nil, out, outDep, outDir,
		manifestPath, true, false, false, false,
		rspFile, rspFileContents, rspFile2, rspFileContents2)
}

func testRuleBuilder_Build(ctx BuilderContext, in Paths, implicit, orderOnly, validation Path,
	flags []string,
	out, outDep, outDir, manifestPath WritablePath,
	restat, sbox, sboxInputs, unescapeNinjaVars bool,
	rspFile WritablePath, rspFileContents Paths, rspFile2 WritablePath, rspFileContents2 Paths) {

	rule := NewRuleBuilder(pctx_ruleBuilderTest, ctx)

	if sbox {
		rule.Sbox(outDir, manifestPath)
		if sboxInputs {
			rule.SandboxInputs()
		}
	}

	rule.Command().
		Tool(PathForSource(ctx, "cp")).
		Flags(flags).
		Inputs(in).
		Implicit(implicit).
		OrderOnly(orderOnly).
		Validation(validation).
		Output(out).
		ImplicitDepFile(outDep).
		FlagWithRspFileInputList("@", rspFile, rspFileContents).
		FlagWithRspFileInputList("@", rspFile2, rspFileContents2)

	if restat {
		rule.Restat()
	}

	if unescapeNinjaVars {
		rule.BuildWithUnescapedNinjaVars("rule", "desc")
	} else {
		rule.Build("rule", "desc")
	}
}

var prepareForRuleBuilderTest = FixtureRegisterWithContext(func(ctx RegistrationContext) {
	ctx.RegisterModuleType("rule_builder_test", testRuleBuilderFactory)
	ctx.RegisterSingletonType("rule_builder_test", testRuleBuilderSingletonFactory)
})

func TestRuleBuilder_Build(t *testing.T) {
	fs := MockFS{
		"in": nil,
		"cp": nil,
	}

	bp := `
		rule_builder_test {
			name: "foo",
			srcs: ["in"],
			restat: true,
		}
		rule_builder_test {
			name: "foo_sbox",
			srcs: ["in"],
			sbox: true,
		}
		rule_builder_test {
			name: "foo_sbox_inputs",
			srcs: ["in"],
			sbox: true,
			sbox_inputs: true,
		}
	`

	result := GroupFixturePreparers(
		prepareForRuleBuilderTest,
		FixtureWithRootAndroidBp(bp),
		fs.AddToFixture(),
	).RunTest(t)

	check := func(t *testing.T, params TestingBuildParams, rspFile2Params TestingBuildParams,
		wantCommand, wantOutput, wantDepfile, wantRspFile, wantRspFile2 string,
		wantRestat bool, extraImplicits, extraCmdDeps []string) {

		t.Helper()
		command := params.RuleParams.Command
		re := regexp.MustCompile(" # hash of input list: [a-z0-9]*$")
		command = re.ReplaceAllLiteralString(command, "")

		AssertStringEquals(t, "RuleParams.Command", wantCommand, command)

		wantDeps := append([]string{"cp"}, extraCmdDeps...)
		AssertArrayString(t, "RuleParams.CommandDeps", wantDeps, params.RuleParams.CommandDeps)

		AssertBoolEquals(t, "RuleParams.Restat", wantRestat, params.RuleParams.Restat)

		wantInputs := []string{"rsp_in"}
		AssertArrayString(t, "Inputs", wantInputs, params.Inputs.Strings())

		wantImplicits := append([]string{"implicit", "in"}, extraImplicits...)
		// The second rsp file and the files listed in it should be in implicits
		wantImplicits = append(wantImplicits, "rsp_in2", wantRspFile2)
		AssertPathsRelativeToTopEquals(t, "Implicits", wantImplicits, params.Implicits)

		wantOrderOnlys := []string{"orderonly"}
		AssertPathsRelativeToTopEquals(t, "OrderOnly", wantOrderOnlys, params.OrderOnly)

		wantValidations := []string{"validation"}
		AssertPathsRelativeToTopEquals(t, "Validations", wantValidations, params.Validations)

		wantRspFileContent := "$in"
		AssertStringEquals(t, "RspfileContent", wantRspFileContent, params.RuleParams.RspfileContent)

		AssertStringEquals(t, "Rspfile", wantRspFile, params.RuleParams.Rspfile)

		AssertPathRelativeToTopEquals(t, "Output", wantOutput, params.Output)

		if len(params.ImplicitOutputs) != 0 {
			t.Errorf("want ImplicitOutputs = [], got %q", params.ImplicitOutputs.Strings())
		}

		AssertPathRelativeToTopEquals(t, "Depfile", wantDepfile, params.Depfile)

		if params.Deps != blueprint.DepsGCC {
			t.Errorf("want Deps = %q, got %q", blueprint.DepsGCC, params.Deps)
		}

		rspFile2Content := ContentFromFileRuleForTests(t, result.TestContext, rspFile2Params)
		AssertStringEquals(t, "rspFile2 content", "rsp_in2\n", rspFile2Content)
	}

	t.Run("module", func(t *testing.T) {
		outFile := "out/soong/.intermediates/foo/gen/foo"
		rspFile := "out/soong/.intermediates/foo/rsp"
		rspFile2 := "out/soong/.intermediates/foo/rsp2"
		module := result.ModuleForTests("foo", "")
		check(t, module.Rule("rule"), module.Output(rspFile2),
			"cp in "+outFile+" @"+rspFile+" @"+rspFile2,
			outFile, outFile+".d", rspFile, rspFile2, true, nil, nil)
	})
	t.Run("sbox", func(t *testing.T) {
		outDir := "out/soong/.intermediates/foo_sbox"
		sboxOutDir := filepath.Join(outDir, "gen")
		outFile := filepath.Join(sboxOutDir, "foo_sbox")
		depFile := filepath.Join(sboxOutDir, "foo_sbox.d")
		rspFile := filepath.Join(outDir, "rsp")
		rspFile2 := filepath.Join(outDir, "rsp2")
		manifest := filepath.Join(outDir, "sbox.textproto")
		sbox := filepath.Join("out", "soong", "host", result.Config.PrebuiltOS(), "bin/sbox")
		sandboxPath := shared.TempDirForOutDir("out/soong")

		cmd := sbox + ` --sandbox-path ` + sandboxPath + ` --output-dir ` + sboxOutDir + ` --manifest ` + manifest
		module := result.ModuleForTests("foo_sbox", "")
		check(t, module.Output("gen/foo_sbox"), module.Output(rspFile2),
			cmd, outFile, depFile, rspFile, rspFile2, false, []string{manifest}, []string{sbox})
	})
	t.Run("sbox_inputs", func(t *testing.T) {
		outDir := "out/soong/.intermediates/foo_sbox_inputs"
		sboxOutDir := filepath.Join(outDir, "gen")
		outFile := filepath.Join(sboxOutDir, "foo_sbox_inputs")
		depFile := filepath.Join(sboxOutDir, "foo_sbox_inputs.d")
		rspFile := filepath.Join(outDir, "rsp")
		rspFile2 := filepath.Join(outDir, "rsp2")
		manifest := filepath.Join(outDir, "sbox.textproto")
		sbox := filepath.Join("out", "soong", "host", result.Config.PrebuiltOS(), "bin/sbox")
		sandboxPath := shared.TempDirForOutDir("out/soong")

		cmd := sbox + ` --sandbox-path ` + sandboxPath + ` --output-dir ` + sboxOutDir + ` --manifest ` + manifest

		module := result.ModuleForTests("foo_sbox_inputs", "")
		check(t, module.Output("gen/foo_sbox_inputs"), module.Output(rspFile2),
			cmd, outFile, depFile, rspFile, rspFile2, false, []string{manifest}, []string{sbox})
	})
	t.Run("singleton", func(t *testing.T) {
		outFile := filepath.Join("out/soong/singleton/gen/baz")
		rspFile := filepath.Join("out/soong/singleton/rsp")
		rspFile2 := filepath.Join("out/soong/singleton/rsp2")
		singleton := result.SingletonForTests("rule_builder_test")
		check(t, singleton.Rule("rule"), singleton.Output(rspFile2),
			"cp in "+outFile+" @"+rspFile+" @"+rspFile2,
			outFile, outFile+".d", rspFile, rspFile2, true, nil, nil)
	})
}

func TestRuleBuilderHashInputs(t *testing.T) {
	// The basic idea here is to verify that the command (in the case of a
	// non-sbox rule) or the sbox textproto manifest contain a hash of the
	// inputs.

	// By including a hash of the inputs, we cause the rule to re-run if
	// the list of inputs changes because the command line or a dependency
	// changes.

	hashOf := func(s string) string {
		sum := sha256.Sum256([]byte(s))
		return hex.EncodeToString(sum[:])
	}

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
			name:         "hash0",
			expectedHash: hashOf("implicit\nin1.txt\nin2.txt"),
		},
		{
			name:         "hash1",
			expectedHash: hashOf("implicit\nin1.txt\nin2.txt\nin3.txt"),
		},
	}

	result := GroupFixturePreparers(
		prepareForRuleBuilderTest,
		FixtureWithRootAndroidBp(bp),
	).RunTest(t)

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			t.Run("sbox", func(t *testing.T) {
				gen := result.ModuleForTests(test.name+"_sbox", "")
				manifest := RuleBuilderSboxProtoForTests(t, result.TestContext, gen.Output("sbox.textproto"))
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

func TestRuleBuilderWithNinjaVarEscaping(t *testing.T) {
	bp := `
		rule_builder_test {
			name: "foo_sbox_escaped",
			flags: ["${cmdFlags}"],
			sbox: true,
			sbox_inputs: true,
		}
		rule_builder_test {
			name: "foo_sbox_unescaped",
			flags: ["${cmdFlags}"],
			sbox: true,
			sbox_inputs: true,
			unescape_ninja_vars: true,
		}
	`
	result := GroupFixturePreparers(
		prepareForRuleBuilderTest,
		FixtureWithRootAndroidBp(bp),
	).RunTest(t)

	escapedNinjaMod := result.ModuleForTests("foo_sbox_escaped", "").Output("sbox.textproto")
	AssertStringEquals(t, "expected rule", "android/soong/android.rawFileCopy", escapedNinjaMod.Rule.String())
	AssertStringDoesContain(
		t,
		"",
		ContentFromFileRuleForTests(t, result.TestContext, escapedNinjaMod),
		"${cmdFlags}",
	)

	unescapedNinjaMod := result.ModuleForTests("foo_sbox_unescaped", "").Rule("unescapedWriteFile")
	AssertStringDoesContain(
		t,
		"",
		unescapedNinjaMod.BuildParams.Args["content"],
		"${cmdFlags}",
	)
	AssertStringDoesNotContain(
		t,
		"",
		unescapedNinjaMod.BuildParams.Args["content"],
		"$${cmdFlags}",
	)
}
