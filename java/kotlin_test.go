// Copyright 2017 Google Inc. All rights reserved.
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
	"strings"
	"testing"
)

func TestKotlin(t *testing.T) {
	ctx := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java", "b.kt"],
		}

		java_library {
			name: "bar",
			srcs: ["b.kt"],
			libs: ["foo"],
			static_libs: ["baz"],
		}

		java_library {
			name: "baz",
			srcs: ["c.java"],
		}
		`)

	fooKotlinc := ctx.ModuleForTests("foo", "android_common").Rule("kotlinc")
	fooJavac := ctx.ModuleForTests("foo", "android_common").Rule("javac")
	fooJar := ctx.ModuleForTests("foo", "android_common").Output("combined/foo.jar")

	if len(fooKotlinc.Inputs) != 2 || fooKotlinc.Inputs[0].String() != "a.java" ||
		fooKotlinc.Inputs[1].String() != "b.kt" {
		t.Errorf(`foo kotlinc inputs %v != ["a.java", "b.kt"]`, fooKotlinc.Inputs)
	}

	if len(fooJavac.Inputs) != 1 || fooJavac.Inputs[0].String() != "a.java" {
		t.Errorf(`foo inputs %v != ["a.java"]`, fooJavac.Inputs)
	}

	if !strings.Contains(fooJavac.Args["classpath"], fooKotlinc.Output.String()) {
		t.Errorf("foo classpath %v does not contain %q",
			fooJavac.Args["classpath"], fooKotlinc.Output.String())
	}

	if !inList(fooKotlinc.Output.String(), fooJar.Inputs.Strings()) {
		t.Errorf("foo jar inputs %v does not contain %q",
			fooJar.Inputs.Strings(), fooKotlinc.Output.String())
	}

	fooHeaderJar := ctx.ModuleForTests("foo", "android_common").Output("turbine-combined/foo.jar")
	bazHeaderJar := ctx.ModuleForTests("baz", "android_common").Output("turbine-combined/baz.jar")
	barKotlinc := ctx.ModuleForTests("bar", "android_common").Rule("kotlinc")

	if len(barKotlinc.Inputs) != 1 || barKotlinc.Inputs[0].String() != "b.kt" {
		t.Errorf(`bar kotlinc inputs %v != ["b.kt"]`, barKotlinc.Inputs)
	}

	if !inList(fooHeaderJar.Output.String(), barKotlinc.Implicits.Strings()) {
		t.Errorf(`expected %q in bar implicits %v`,
			fooHeaderJar.Output.String(), barKotlinc.Implicits.Strings())
	}

	if !inList(bazHeaderJar.Output.String(), barKotlinc.Implicits.Strings()) {
		t.Errorf(`expected %q in bar implicits %v`,
			bazHeaderJar.Output.String(), barKotlinc.Implicits.Strings())
	}
}
