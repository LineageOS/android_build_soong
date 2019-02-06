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

func ExampleRuleBuilder() {
	rule := NewRuleBuilder()

	rule.Command().Tool("ld").Inputs([]string{"a.o", "b.o"}).FlagWithOutput("-o ", "linked")
	rule.Command().Text("echo success")

	// To add the command to the build graph:
	// rule.Build(pctx, ctx, "link", "link")

	fmt.Printf("commands: %q\n", strings.Join(rule.Commands(), " && "))
	fmt.Printf("tools: %q\n", rule.Tools())
	fmt.Printf("inputs: %q\n", rule.Inputs())
	fmt.Printf("outputs: %q\n", rule.Outputs())

	// Output:
	// commands: "ld a.o b.o -o linked && echo success"
	// tools: ["ld"]
	// inputs: ["a.o" "b.o"]
	// outputs: ["linked"]
}

func ExampleRuleBuilder_Temporary() {
	rule := NewRuleBuilder()

	rule.Command().Tool("cp").Input("a").Output("b")
	rule.Command().Tool("cp").Input("b").Output("c")
	rule.Temporary("b")

	fmt.Printf("commands: %q\n", strings.Join(rule.Commands(), " && "))
	fmt.Printf("tools: %q\n", rule.Tools())
	fmt.Printf("inputs: %q\n", rule.Inputs())
	fmt.Printf("outputs: %q\n", rule.Outputs())

	// Output:
	// commands: "cp a b && cp b c"
	// tools: ["cp"]
	// inputs: ["a"]
	// outputs: ["c"]
}

func ExampleRuleBuilder_DeleteTemporaryFiles() {
	rule := NewRuleBuilder()

	rule.Command().Tool("cp").Input("a").Output("b")
	rule.Command().Tool("cp").Input("b").Output("c")
	rule.Temporary("b")
	rule.DeleteTemporaryFiles()

	fmt.Printf("commands: %q\n", strings.Join(rule.Commands(), " && "))
	fmt.Printf("tools: %q\n", rule.Tools())
	fmt.Printf("inputs: %q\n", rule.Inputs())
	fmt.Printf("outputs: %q\n", rule.Outputs())

	// Output:
	// commands: "cp a b && cp b c && rm -f b"
	// tools: ["cp"]
	// inputs: ["a"]
	// outputs: ["c"]
}

func ExampleRuleBuilderCommand() {
	rule := NewRuleBuilder()

	// chained
	rule.Command().Tool("ld").Inputs([]string{"a.o", "b.o"}).FlagWithOutput("-o ", "linked")

	// unchained
	cmd := rule.Command()
	cmd.Tool("ld")
	cmd.Inputs([]string{"a.o", "b.o"})
	cmd.FlagWithOutput("-o ", "linked")

	// mixed:
	cmd = rule.Command().Tool("ld")
	cmd.Inputs([]string{"a.o", "b.o"})
	cmd.FlagWithOutput("-o ", "linked")
}

func ExampleRuleBuilderCommand_Flag() {
	fmt.Println(NewRuleBuilder().Command().
		Tool("ls").Flag("-l"))
	// Output:
	// ls -l
}

func ExampleRuleBuilderCommand_FlagWithArg() {
	fmt.Println(NewRuleBuilder().Command().
		Tool("ls").
		FlagWithArg("--sort=", "time"))
	// Output:
	// ls --sort=time
}

func ExampleRuleBuilderCommand_FlagForEachInput() {
	fmt.Println(NewRuleBuilder().Command().
		Tool("turbine").
		FlagForEachInput("--classpath ", []string{"a.jar", "b.jar"}))
	// Output:
	// turbine --classpath a.jar --classpath b.jar
}

func ExampleRuleBuilderCommand_FlagWithInputList() {
	fmt.Println(NewRuleBuilder().Command().
		Tool("java").
		FlagWithInputList("-classpath=", []string{"a.jar", "b.jar"}, ":"))
	// Output:
	// java -classpath=a.jar:b.jar
}

func ExampleRuleBuilderCommand_FlagWithInput() {
	fmt.Println(NewRuleBuilder().Command().
		Tool("java").
		FlagWithInput("-classpath=", "a"))
	// Output:
	// java -classpath=a
}

func ExampleRuleBuilderCommand_FlagWithList() {
	fmt.Println(NewRuleBuilder().Command().
		Tool("ls").
		FlagWithList("--sort=", []string{"time", "size"}, ","))
	// Output:
	// ls --sort=time,size
}

func TestRuleBuilder(t *testing.T) {
	rule := NewRuleBuilder()

	cmd := rule.Command().
		Flag("Flag").
		FlagWithArg("FlagWithArg=", "arg").
		FlagWithInput("FlagWithInput=", "input").
		FlagWithOutput("FlagWithOutput=", "output").
		Implicit("Implicit").
		ImplicitOutput("ImplicitOutput").
		Input("Input").
		Output("Output").
		Text("Text").
		Tool("Tool")

	rule.Command().
		Text("command2").
		Input("input2").
		Output("output2").
		Tool("tool2")

	// Test updates to the first command after the second command has been started
	cmd.Text("after command2")
	// Test updating a command when the previous update did not replace the cmd variable
	cmd.Text("old cmd")

	// Test a command that uses the output of a previous command as an input
	rule.Command().
		Text("command3").
		Input("input3").
		Input("output2").
		Output("output3")

	wantCommands := []string{
		"Flag FlagWithArg=arg FlagWithInput=input FlagWithOutput=output Input Output Text Tool after command2 old cmd",
		"command2 input2 output2 tool2",
		"command3 input3 output2 output3",
	}
	wantInputs := []string{"Implicit", "Input", "input", "input2", "input3"}
	wantOutputs := []string{"ImplicitOutput", "Output", "output", "output2", "output3"}
	wantTools := []string{"Tool", "tool2"}

	if !reflect.DeepEqual(rule.Commands(), wantCommands) {
		t.Errorf("\nwant rule.Commands() = %#v\n                   got %#v", wantCommands, rule.Commands())
	}
	if !reflect.DeepEqual(rule.Inputs(), wantInputs) {
		t.Errorf("\nwant rule.Inputs() = %#v\n                 got %#v", wantInputs, rule.Inputs())
	}
	if !reflect.DeepEqual(rule.Outputs(), wantOutputs) {
		t.Errorf("\nwant rule.Outputs() = %#v\n                  got %#v", wantOutputs, rule.Outputs())
	}
	if !reflect.DeepEqual(rule.Tools(), wantTools) {
		t.Errorf("\nwant rule.Tools() = %#v\n                got %#v", wantTools, rule.Tools())
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

	rule.Command().Tool("cp").Input(in.String()).Output(out.String())

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

	foo := ctx.ModuleForTests("foo", "").Rule("rule")

	// TODO: make RuleParams accessible to tests and verify rule.Command().Tools() ends up in CommandDeps

	if len(foo.Implicits) != 1 || foo.Implicits[0].String() != "bar" {
		t.Errorf("want foo.Implicits = [%q], got %q", "bar", foo.Implicits.Strings())
	}

	wantOutput := filepath.Join(buildDir, ".intermediates", "foo", "foo")
	if len(foo.Outputs) != 1 || foo.Outputs[0].String() != wantOutput {
		t.Errorf("want foo.Outputs = [%q], got %q", wantOutput, foo.Outputs.Strings())
	}

}
