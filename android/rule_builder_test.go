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
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func pathContext() PathContext {
	return PathContextForTesting(TestConfig("out", nil),
		map[string][]byte{
			"ld":      nil,
			"a.o":     nil,
			"b.o":     nil,
			"cp":      nil,
			"a":       nil,
			"b":       nil,
			"ls":      nil,
			"turbine": nil,
			"java":    nil,
		})
}

func ExampleRuleBuilder() {
	rule := NewRuleBuilder()

	ctx := pathContext()

	rule.Command().
		Tool(PathForSource(ctx, "ld")).
		Inputs(PathsForTesting("a.o", "b.o")).
		FlagWithOutput("-o ", PathForOutput(ctx, "linked"))
	rule.Command().Text("echo success")

	// To add the command to the build graph:
	// rule.Build(pctx, ctx, "link", "link")

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

func ExampleRuleBuilder_Temporary() {
	rule := NewRuleBuilder()

	ctx := pathContext()

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
	rule := NewRuleBuilder()

	ctx := pathContext()

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
	rule := NewRuleBuilder()

	ctx := pathContext()

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
	rule := NewRuleBuilder()

	ctx := pathContext()

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
	ctx := pathContext()
	fmt.Println(NewRuleBuilder().Command().
		Tool(PathForSource(ctx, "ls")).Flag("-l"))
	// Output:
	// ls -l
}

func ExampleRuleBuilderCommand_Flags() {
	ctx := pathContext()
	fmt.Println(NewRuleBuilder().Command().
		Tool(PathForSource(ctx, "ls")).Flags([]string{"-l", "-a"}))
	// Output:
	// ls -l -a
}

func ExampleRuleBuilderCommand_FlagWithArg() {
	ctx := pathContext()
	fmt.Println(NewRuleBuilder().Command().
		Tool(PathForSource(ctx, "ls")).
		FlagWithArg("--sort=", "time"))
	// Output:
	// ls --sort=time
}

func ExampleRuleBuilderCommand_FlagForEachArg() {
	ctx := pathContext()
	fmt.Println(NewRuleBuilder().Command().
		Tool(PathForSource(ctx, "ls")).
		FlagForEachArg("--sort=", []string{"time", "size"}))
	// Output:
	// ls --sort=time --sort=size
}

func ExampleRuleBuilderCommand_FlagForEachInput() {
	ctx := pathContext()
	fmt.Println(NewRuleBuilder().Command().
		Tool(PathForSource(ctx, "turbine")).
		FlagForEachInput("--classpath ", PathsForTesting("a.jar", "b.jar")))
	// Output:
	// turbine --classpath a.jar --classpath b.jar
}

func ExampleRuleBuilderCommand_FlagWithInputList() {
	ctx := pathContext()
	fmt.Println(NewRuleBuilder().Command().
		Tool(PathForSource(ctx, "java")).
		FlagWithInputList("-classpath=", PathsForTesting("a.jar", "b.jar"), ":"))
	// Output:
	// java -classpath=a.jar:b.jar
}

func ExampleRuleBuilderCommand_FlagWithInput() {
	ctx := pathContext()
	fmt.Println(NewRuleBuilder().Command().
		Tool(PathForSource(ctx, "java")).
		FlagWithInput("-classpath=", PathForSource(ctx, "a")))
	// Output:
	// java -classpath=a
}

func ExampleRuleBuilderCommand_FlagWithList() {
	ctx := pathContext()
	fmt.Println(NewRuleBuilder().Command().
		Tool(PathForSource(ctx, "ls")).
		FlagWithList("--sort=", []string{"time", "size"}, ","))
	// Output:
	// ls --sort=time,size
}

func TestRuleBuilder(t *testing.T) {
	rule := NewRuleBuilder()

	fs := map[string][]byte{
		"dep_fixer": nil,
		"input":     nil,
		"Implicit":  nil,
		"Input":     nil,
		"Tool":      nil,
		"input2":    nil,
		"tool2":     nil,
		"input3":    nil,
	}

	ctx := PathContextForTesting(TestConfig("out", nil), fs)

	cmd := rule.Command().
		DepFile(PathForOutput(ctx, "DepFile")).
		Flag("Flag").
		FlagWithArg("FlagWithArg=", "arg").
		FlagWithDepFile("FlagWithDepFile=", PathForOutput(ctx, "depfile")).
		FlagWithInput("FlagWithInput=", PathForSource(ctx, "input")).
		FlagWithOutput("FlagWithOutput=", PathForOutput(ctx, "output")).
		Implicit(PathForSource(ctx, "Implicit")).
		ImplicitDepFile(PathForOutput(ctx, "ImplicitDepFile")).
		ImplicitOutput(PathForOutput(ctx, "ImplicitOutput")).
		Input(PathForSource(ctx, "Input")).
		Output(PathForOutput(ctx, "Output")).
		Text("Text").
		Tool(PathForSource(ctx, "Tool"))

	rule.Command().
		Text("command2").
		DepFile(PathForOutput(ctx, "depfile2")).
		Input(PathForSource(ctx, "input2")).
		Output(PathForOutput(ctx, "output2")).
		Tool(PathForSource(ctx, "tool2"))

	// Test updates to the first command after the second command has been started
	cmd.Text("after command2")
	// Test updating a command when the previous update did not replace the cmd variable
	cmd.Text("old cmd")

	// Test a command that uses the output of a previous command as an input
	rule.Command().
		Text("command3").
		Input(PathForSource(ctx, "input3")).
		Input(PathForOutput(ctx, "output2")).
		Output(PathForOutput(ctx, "output3"))

	wantCommands := []string{
		"out/DepFile Flag FlagWithArg=arg FlagWithDepFile=out/depfile FlagWithInput=input FlagWithOutput=out/output Input out/Output Text Tool after command2 old cmd",
		"command2 out/depfile2 input2 out/output2 tool2",
		"command3 input3 out/output2 out/output3",
	}

	wantDepMergerCommand := "out/host/" + ctx.Config().PrebuiltOS() + "/bin/dep_fixer out/DepFile out/depfile out/ImplicitDepFile out/depfile2"

	wantInputs := PathsForSource(ctx, []string{"Implicit", "Input", "input", "input2", "input3"})
	wantOutputs := PathsForOutput(ctx, []string{"ImplicitOutput", "Output", "output", "output2", "output3"})
	wantDepFiles := PathsForOutput(ctx, []string{"DepFile", "depfile", "ImplicitDepFile", "depfile2"})
	wantTools := PathsForSource(ctx, []string{"Tool", "tool2"})

	if g, w := rule.Commands(), wantCommands; !reflect.DeepEqual(g, w) {
		t.Errorf("\nwant rule.Commands() = %#v\n                   got %#v", w, g)
	}

	if g, w := rule.depFileMergerCmd(ctx, rule.DepFiles()).String(), wantDepMergerCommand; g != w {
		t.Errorf("\nwant rule.depFileMergerCmd() = %#v\n                   got %#v", w, g)
	}

	if g, w := rule.Inputs(), wantInputs; !reflect.DeepEqual(w, g) {
		t.Errorf("\nwant rule.Inputs() = %#v\n                 got %#v", w, g)
	}
	if g, w := rule.Outputs(), wantOutputs; !reflect.DeepEqual(w, g) {
		t.Errorf("\nwant rule.Outputs() = %#v\n                  got %#v", w, g)
	}
	if g, w := rule.DepFiles(), wantDepFiles; !reflect.DeepEqual(w, g) {
		t.Errorf("\nwant rule.DepFiles() = %#v\n                  got %#v", w, g)
	}
	if g, w := rule.Tools(), wantTools; !reflect.DeepEqual(w, g) {
		t.Errorf("\nwant rule.Tools() = %#v\n                got %#v", w, g)
	}
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
		Src string
	}
}

func (t *testRuleBuilderModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	in := PathForSource(ctx, t.properties.Src)
	out := PathForModuleOut(ctx, ctx.ModuleName())

	testRuleBuilder_Build(ctx, in, out)
}

type testRuleBuilderSingleton struct{}

func testRuleBuilderSingletonFactory() Singleton {
	return &testRuleBuilderSingleton{}
}

func (t *testRuleBuilderSingleton) GenerateBuildActions(ctx SingletonContext) {
	in := PathForSource(ctx, "bar")
	out := PathForOutput(ctx, "baz")
	testRuleBuilder_Build(ctx, in, out)
}

func testRuleBuilder_Build(ctx BuilderContext, in Path, out WritablePath) {
	rule := NewRuleBuilder()

	rule.Command().Tool(PathForSource(ctx, "cp")).Input(in).Output(out)

	rule.Restat()

	rule.Build(pctx, ctx, "rule", "desc")
}

func TestRuleBuilder_Build(t *testing.T) {
	buildDir, err := ioutil.TempDir("", "soong_test_rule_builder")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(buildDir)

	bp := `
		rule_builder_test {
			name: "foo",
			src: "bar",
		}
	`

	config := TestConfig(buildDir, nil)
	ctx := NewTestContext()
	ctx.MockFileSystem(map[string][]byte{
		"Android.bp": []byte(bp),
		"bar":        nil,
		"cp":         nil,
	})
	ctx.RegisterModuleType("rule_builder_test", ModuleFactoryAdaptor(testRuleBuilderFactory))
	ctx.RegisterSingletonType("rule_builder_test", SingletonFactoryAdaptor(testRuleBuilderSingletonFactory))
	ctx.Register()

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	FailIfErrored(t, errs)

	check := func(t *testing.T, params TestingBuildParams, wantOutput string) {
		if len(params.RuleParams.CommandDeps) != 1 || params.RuleParams.CommandDeps[0] != "cp" {
			t.Errorf("want RuleParams.CommandDeps = [%q], got %q", "cp", params.RuleParams.CommandDeps)
		}

		if len(params.Implicits) != 1 || params.Implicits[0].String() != "bar" {
			t.Errorf("want Implicits = [%q], got %q", "bar", params.Implicits.Strings())
		}

		if params.Output.String() != wantOutput {
			t.Errorf("want Output = %q, got %q", wantOutput, params.Output)
		}

		if !params.RuleParams.Restat {
			t.Errorf("want RuleParams.Restat = true, got %v", params.RuleParams.Restat)
		}
	}

	t.Run("module", func(t *testing.T) {
		check(t, ctx.ModuleForTests("foo", "").Rule("rule"),
			filepath.Join(buildDir, ".intermediates", "foo", "foo"))
	})
	t.Run("singleton", func(t *testing.T) {
		check(t, ctx.SingletonForTests("rule_builder_test").Rule("rule"),
			filepath.Join(buildDir, "baz"))
	})
}
