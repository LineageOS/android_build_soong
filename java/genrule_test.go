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

package java

import (
	"reflect"
	"strings"
	"testing"

	"android/soong/android"
)

func testGenruleContext(config android.Config) *android.TestContext {
	ctx := android.NewTestArchContext(config)
	ctx.RegisterModuleType("java_genrule", GenRuleFactory)
	ctx.Register()

	return ctx
}

func TestGenruleCmd(t *testing.T) {
	fs := map[string][]byte{
		"tool": nil,
		"foo":  nil,
	}
	bp := `
				java_genrule {
					name: "gen",
					tool_files: ["tool"],
					cmd: "$(location tool) $(in) $(out)",
					srcs: ["foo"],
					out: ["out"],
				}
			`
	config := android.TestArchConfig(t.TempDir(), nil, bp, fs)

	ctx := testGenruleContext(config)

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	if errs == nil {
		_, errs = ctx.PrepareBuildActions(config)
	}
	if errs != nil {
		t.Fatal(errs)
	}

	gen := ctx.ModuleForTests("gen", "android_common").Output("out")
	expected := []string{"foo"}
	if !reflect.DeepEqual(expected, gen.Implicits.Strings()[:len(expected)]) {
		t.Errorf(`want arm inputs %v, got %v`, expected, gen.Implicits.Strings())
	}
}

func TestJarGenrules(t *testing.T) {
	ctx, _ := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
		}

		java_genrule {
			name: "jargen",
			tool_files: ["b.java"],
			cmd: "$(location b.java) $(in) $(out)",
			out: ["jargen.jar"],
			srcs: [":foo"],
		}

		java_library {
			name: "bar",
			static_libs: ["jargen"],
			srcs: ["c.java"],
		}

		java_library {
			name: "baz",
			libs: ["jargen"],
			srcs: ["c.java"],
		}
	`)

	foo := ctx.ModuleForTests("foo", "android_common").Output("javac/foo.jar")
	jargen := ctx.ModuleForTests("jargen", "android_common").Output("jargen.jar")
	bar := ctx.ModuleForTests("bar", "android_common").Output("javac/bar.jar")
	baz := ctx.ModuleForTests("baz", "android_common").Output("javac/baz.jar")
	barCombined := ctx.ModuleForTests("bar", "android_common").Output("combined/bar.jar")

	if g, w := jargen.Implicits.Strings(), foo.Output.String(); !android.InList(w, g) {
		t.Errorf("expected jargen inputs [%q], got %q", w, g)
	}

	if !strings.Contains(bar.Args["classpath"], jargen.Output.String()) {
		t.Errorf("bar classpath %v does not contain %q", bar.Args["classpath"], jargen.Output.String())
	}

	if !strings.Contains(baz.Args["classpath"], jargen.Output.String()) {
		t.Errorf("baz classpath %v does not contain %q", baz.Args["classpath"], jargen.Output.String())
	}

	if len(barCombined.Inputs) != 2 ||
		barCombined.Inputs[0].String() != bar.Output.String() ||
		barCombined.Inputs[1].String() != jargen.Output.String() {
		t.Errorf("bar combined jar inputs %v is not [%q, %q]",
			barCombined.Inputs.Strings(), bar.Output.String(), jargen.Output.String())
	}
}
