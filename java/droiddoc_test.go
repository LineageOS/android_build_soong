// Copyright 2021 Google Inc. All rights reserved.
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

func TestDroiddoc(t *testing.T) {
	ctx, _ := testJavaWithFS(t, `
		droiddoc_exported_dir {
		    name: "droiddoc-templates-sdk",
		    path: ".",
		}
		filegroup {
		    name: "bar-doc-aidl-srcs",
		    srcs: ["bar-doc/IBar.aidl"],
		    path: "bar-doc",
		}
		droidstubs {
		    name: "bar-stubs",
		    srcs: [
		        "bar-doc/a.java",
		    ],
		    exclude_srcs: [
		        "bar-doc/b.java"
		    ],
		    api_levels_annotations_dirs: [
		      "droiddoc-templates-sdk",
		    ],
		    api_levels_annotations_enabled: true,
		}
		droiddoc {
		    name: "bar-doc",
		    srcs: [
		        ":bar-stubs",
		        "bar-doc/IFoo.aidl",
		        ":bar-doc-aidl-srcs",
		    ],
		    custom_template: "droiddoc-templates-sdk",
		    hdf: [
		        "android.whichdoc offline",
		    ],
		    knowntags: [
		        "bar-doc/known_oj_tags.txt",
		    ],
		    proofread_file: "libcore-proofread.txt",
		    todo_file: "libcore-docs-todo.html",
		    flags: ["-offlinemode -title \"libcore\""],
		}
		`,
		map[string][]byte{
			"bar-doc/a.java": nil,
			"bar-doc/b.java": nil,
		})
	barStubs := ctx.ModuleForTests("bar-stubs", "android_common")
	barStubsOutputs, err := barStubs.Module().(*Droidstubs).OutputFiles("")
	if err != nil {
		t.Errorf("Unexpected error %q retrieving \"bar-stubs\" output file", err)
	}
	if len(barStubsOutputs) != 1 {
		t.Errorf("Expected one output from \"bar-stubs\" got %s", barStubsOutputs)
	}

	barStubsOutput := barStubsOutputs[0]
	barDoc := ctx.ModuleForTests("bar-doc", "android_common")
	javaDoc := barDoc.Rule("javadoc")
	if g, w := android.PathsRelativeToTop(javaDoc.Implicits), android.PathRelativeToTop(barStubsOutput); !inList(w, g) {
		t.Errorf("implicits of bar-doc must contain %q, but was %q.", w, g)
	}

	expected := "-sourcepath out/soong/.intermediates/bar-doc/android_common/srcjars "
	if !strings.Contains(javaDoc.RuleParams.Command, expected) {
		t.Errorf("bar-doc command does not contain flag %q, but should\n%q", expected, javaDoc.RuleParams.Command)
	}

	aidl := barDoc.Rule("aidl")
	if g, w := android.PathsRelativeToTop(javaDoc.Implicits), android.PathRelativeToTop(aidl.Output); !inList(w, g) {
		t.Errorf("implicits of bar-doc must contain %q, but was %q.", w, g)
	}

	if g, w := aidl.Implicits.Strings(), []string{"bar-doc/IBar.aidl", "bar-doc/IFoo.aidl"}; !reflect.DeepEqual(w, g) {
		t.Errorf("aidl inputs must be %q, but was %q", w, g)
	}
}

func TestDroiddocArgsAndFlagsCausesError(t *testing.T) {
	testJavaError(t, "flags is set. Cannot set args", `
		droiddoc_exported_dir {
		    name: "droiddoc-templates-sdk",
		    path: ".",
		}
		filegroup {
		    name: "bar-doc-aidl-srcs",
		    srcs: ["bar-doc/IBar.aidl"],
		    path: "bar-doc",
		}
		droidstubs {
		    name: "bar-stubs",
		    srcs: [
		        "bar-doc/a.java",
		    ],
		    exclude_srcs: [
		        "bar-doc/b.java"
		    ],
		    api_levels_annotations_dirs: [
		      "droiddoc-templates-sdk",
		    ],
		    api_levels_annotations_enabled: true,
		}
		droiddoc {
		    name: "bar-doc",
		    srcs: [
		        ":bar-stubs",
		        "bar-doc/IFoo.aidl",
		        ":bar-doc-aidl-srcs",
		    ],
		    custom_template: "droiddoc-templates-sdk",
		    hdf: [
		        "android.whichdoc offline",
		    ],
		    knowntags: [
		        "bar-doc/known_oj_tags.txt",
		    ],
		    proofread_file: "libcore-proofread.txt",
		    todo_file: "libcore-docs-todo.html",
		    flags: ["-offlinemode -title \"libcore\""],
		    args: "-offlinemode -title \"libcore\"",
		}
		`)
}
