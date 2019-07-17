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

package java

import (
	"android/soong/android"
	"testing"
)

func TestNoPlugin(t *testing.T) {
	ctx, _ := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
		}
	`)

	javac := ctx.ModuleForTests("foo", "android_common").Rule("javac")
	turbine := ctx.ModuleForTests("foo", "android_common").MaybeRule("turbine")

	if turbine.Rule == nil {
		t.Errorf("expected turbine to be enabled")
	}

	if javac.Args["processsorpath"] != "" {
		t.Errorf("want empty processorpath, got %q", javac.Args["processorpath"])
	}

	if javac.Args["processor"] != "-proc:none" {
		t.Errorf("want '-proc:none' argument, got %q", javac.Args["processor"])
	}
}

func TestPlugin(t *testing.T) {
	ctx, _ := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			plugins: ["bar"],
		}

		java_plugin {
			name: "bar",
			processor_class: "com.bar",
			srcs: ["b.java"],
		}
	`)

	buildOS := android.BuildOs.String()

	javac := ctx.ModuleForTests("foo", "android_common").Rule("javac")
	turbine := ctx.ModuleForTests("foo", "android_common").MaybeRule("turbine")

	if turbine.Rule == nil {
		t.Errorf("expected turbine to be enabled")
	}

	bar := ctx.ModuleForTests("bar", buildOS+"_common").Rule("javac").Output.String()

	if !inList(bar, javac.Implicits.Strings()) {
		t.Errorf("foo implicits %v does not contain %q", javac.Implicits.Strings(), bar)
	}

	if javac.Args["processorpath"] != "-processorpath "+bar {
		t.Errorf("foo processorpath %q != '-processorpath %s'", javac.Args["processorpath"], bar)
	}

	if javac.Args["processor"] != "-processor com.bar" {
		t.Errorf("foo processor %q != '-processor com.bar'", javac.Args["processor"])
	}
}

func TestPluginGeneratesApi(t *testing.T) {
	ctx, _ := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			plugins: ["bar"],
		}

		java_plugin {
			name: "bar",
			processor_class: "com.bar",
			generates_api: true,
			srcs: ["b.java"],
		}
	`)

	buildOS := android.BuildOs.String()

	javac := ctx.ModuleForTests("foo", "android_common").Rule("javac")
	turbine := ctx.ModuleForTests("foo", "android_common").MaybeRule("turbine")

	if turbine.Rule != nil {
		t.Errorf("expected turbine to be disabled")
	}

	bar := ctx.ModuleForTests("bar", buildOS+"_common").Rule("javac").Output.String()

	if !inList(bar, javac.Implicits.Strings()) {
		t.Errorf("foo implicits %v does not contain %q", javac.Implicits.Strings(), bar)
	}

	if javac.Args["processorpath"] != "-processorpath "+bar {
		t.Errorf("foo processorpath %q != '-processorpath %s'", javac.Args["processorpath"], bar)
	}

	if javac.Args["processor"] != "-processor com.bar" {
		t.Errorf("foo processor %q != '-processor com.bar'", javac.Args["processor"])
	}
}
