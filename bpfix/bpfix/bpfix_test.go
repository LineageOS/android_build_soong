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

// This file implements the logic of bpfix and also provides a programmatic interface

package bpfix

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"reflect"

	"github.com/google/blueprint/parser"
)

// TODO(jeffrygaston) remove this when position is removed from ParseNode (in b/38325146) and we can directly do reflect.DeepEqual
func printListOfStrings(items []string) (text string) {
	if len(items) == 0 {
		return "[]"
	}
	return fmt.Sprintf("[\"%s\"]", strings.Join(items, "\", \""))

}

func buildTree(local_include_dirs []string, export_include_dirs []string) (file *parser.File, errs []error) {
	// TODO(jeffrygaston) use the builder class when b/38325146 is done
	input := fmt.Sprintf(`cc_library_shared {
	    name: "iAmAModule",
	    local_include_dirs: %s,
	    export_include_dirs: %s,
	}
	`,
		printListOfStrings(local_include_dirs), printListOfStrings(export_include_dirs))
	tree, errs := parser.Parse("", strings.NewReader(input), parser.NewScope(nil))
	if len(errs) > 0 {
		errs = append([]error{fmt.Errorf("failed to parse:\n%s", input)}, errs...)
	}
	return tree, errs
}

func implFilterListTest(t *testing.T, local_include_dirs []string, export_include_dirs []string, expectedResult []string) {
	// build tree
	tree, errs := buildTree(local_include_dirs, export_include_dirs)
	if len(errs) > 0 {
		t.Error("failed to build tree")
		for _, err := range errs {
			t.Error(err)
		}
		t.Fatalf("%d parse errors", len(errs))
	}

	fixer := NewFixer(tree)

	// apply simplifications
	err := runPatchListMod(simplifyKnownPropertiesDuplicatingEachOther)(fixer)
	if len(errs) > 0 {
		t.Fatal(err)
	}

	// lookup legacy property
	mod := fixer.tree.Defs[0].(*parser.Module)

	expectedResultString := fmt.Sprintf("%q", expectedResult)
	if expectedResult == nil {
		expectedResultString = "unset"
	}

	// check that the value for the legacy property was updated to the correct value
	errorHeader := fmt.Sprintf("\nFailed to correctly simplify key 'local_include_dirs' in the presence of 'export_include_dirs.'\n"+
		"original local_include_dirs: %q\n"+
		"original export_include_dirs: %q\n"+
		"expected result: %s\n"+
		"actual result: ",
		local_include_dirs, export_include_dirs, expectedResultString)
	result, found := mod.GetProperty("local_include_dirs")
	if !found {
		if expectedResult == nil {
			return
		}
		t.Fatal(errorHeader + "property not found")
	}

	listResult, ok := result.Value.(*parser.List)
	if !ok {
		t.Fatalf("%sproperty is not a list: %v", errorHeader, listResult)
	}

	if expectedResult == nil {
		t.Fatalf("%sproperty exists: %v", errorHeader, listResult)
	}

	actualExpressions := listResult.Values
	actualValues := make([]string, 0)
	for _, expr := range actualExpressions {
		str := expr.(*parser.String)
		actualValues = append(actualValues, str.Value)
	}

	if !reflect.DeepEqual(actualValues, expectedResult) {
		t.Fatalf("%s%q\nlists are different", errorHeader, actualValues)
	}
}

func TestSimplifyKnownVariablesDuplicatingEachOther(t *testing.T) {
	// TODO use []Expression{} once buildTree above can support it (which is after b/38325146 is done)
	implFilterListTest(t, []string{"include"}, []string{"include"}, nil)
	implFilterListTest(t, []string{"include1"}, []string{"include2"}, []string{"include1"})
	implFilterListTest(t, []string{"include1", "include2", "include3", "include4"}, []string{"include2"},
		[]string{"include1", "include3", "include4"})
	implFilterListTest(t, []string{}, []string{"include"}, []string{})
	implFilterListTest(t, []string{}, []string{}, []string{})
}

func runPass(t *testing.T, in, out string, innerTest func(*Fixer) error) {
	expected, err := Reformat(out)
	if err != nil {
		t.Fatal(err)
	}

	in, err = Reformat(in)
	if err != nil {
		t.Fatal(err)
	}

	tree, errs := parser.Parse("<testcase>", bytes.NewBufferString(in), parser.NewScope(nil))
	if errs != nil {
		t.Fatal(errs)
	}

	fixer := NewFixer(tree)

	got := ""
	prev := "foo"
	passes := 0
	for got != prev && passes < 10 {
		err := innerTest(fixer)
		if err != nil {
			t.Fatal(err)
		}

		out, err := parser.Print(fixer.tree)
		if err != nil {
			t.Fatal(err)
		}

		prev = got
		got = string(out)
		passes++
	}

	if got != expected {
		t.Errorf("output didn't match:\ninput:\n%s\n\nexpected:\n%s\ngot:\n%s\n",
			in, expected, got)
	}
}

func TestMergeMatchingProperties(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
	}{
		{
			name: "empty",
			in: `
				java_library {
					name: "foo",
					static_libs: [],
					static_libs: [],
				}
			`,
			out: `
				java_library {
					name: "foo",
					static_libs: [],
				}
			`,
		},
		{
			name: "single line into multiline",
			in: `
				java_library {
					name: "foo",
					static_libs: [
						"a",
						"b",
					],
					//c1
					static_libs: ["c" /*c2*/],
				}
			`,
			out: `
				java_library {
					name: "foo",
					static_libs: [
						"a",
						"b",
						"c", /*c2*/
					],
					//c1
				}
			`,
		},
		{
			name: "multiline into multiline",
			in: `
				java_library {
					name: "foo",
					static_libs: [
						"a",
						"b",
					],
					//c1
					static_libs: [
						//c2
						"c", //c3
						"d",
					],
				}
			`,
			out: `
				java_library {
					name: "foo",
					static_libs: [
						"a",
						"b",
						//c2
						"c", //c3
						"d",
					],
					//c1
				}
			`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runPass(t, test.in, test.out, func(fixer *Fixer) error {
				return runPatchListMod(mergeMatchingModuleProperties)(fixer)
			})
		})
	}
}

func TestReorderCommonProperties(t *testing.T) {
	var tests = []struct {
		name string
		in   string
		out  string
	}{
		{
			name: "empty",
			in:   `cc_library {}`,
			out:  `cc_library {}`,
		},
		{
			name: "only priority",
			in: `
				cc_library {
					name: "foo",
				}
			`,
			out: `
				cc_library {
					name: "foo",
				}
			`,
		},
		{
			name: "already in order",
			in: `
				cc_library {
					name: "foo",
					defaults: ["bar"],
				}
			`,
			out: `
				cc_library {
					name: "foo",
					defaults: ["bar"],
				}
			`,
		},
		{
			name: "reorder only priority",
			in: `
				cc_library {
					defaults: ["bar"],
					name: "foo",
				}
			`,
			out: `
				cc_library {
					name: "foo",
					defaults: ["bar"],
				}
			`,
		},
		{
			name: "reorder",
			in: `
				cc_library {
					name: "foo",
					srcs: ["a.c"],
					host_supported: true,
					defaults: ["bar"],
					shared_libs: ["baz"],
				}
			`,
			out: `
				cc_library {
					name: "foo",
					defaults: ["bar"],
					host_supported: true,
					srcs: ["a.c"],
					shared_libs: ["baz"],
				}
			`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runPass(t, test.in, test.out, func(fixer *Fixer) error {
				return runPatchListMod(reorderCommonProperties)(fixer)
			})
		})
	}
}

func TestRemoveMatchingModuleListProperties(t *testing.T) {
	var tests = []struct {
		name string
		in   string
		out  string
	}{
		{
			name: "simple",
			in: `
				cc_library {
					name: "foo",
					foo: ["a"],
					bar: ["a"],
				}
			`,
			out: `
				cc_library {
					name: "foo",
					bar: ["a"],
				}
			`,
		},
		{
			name: "long",
			in: `
				cc_library {
					name: "foo",
					foo: [
						"a",
						"b",
					],
					bar: ["a"],
				}
			`,
			out: `
				cc_library {
					name: "foo",
					foo: [
						"b",
					],
					bar: ["a"],
				}
			`,
		},
		{
			name: "long fully removed",
			in: `
				cc_library {
					name: "foo",
					foo: [
						"a",
					],
					bar: ["a"],
				}
			`,
			out: `
				cc_library {
					name: "foo",
					bar: ["a"],
				}
			`,
		},
		{
			name: "comment",
			in: `
				cc_library {
					name: "foo",

					// comment
					foo: ["a"],

					bar: ["a"],
				}
			`,
			out: `
				cc_library {
					name: "foo",

					// comment

					bar: ["a"],
				}
			`,
		},
		{
			name: "inner comment",
			in: `
				cc_library {
					name: "foo",
					foo: [
						// comment
						"a",
					],
					bar: ["a"],
				}
			`,
			out: `
				cc_library {
					name: "foo",
					bar: ["a"],
				}
			`,
		},
		{
			name: "eol comment",
			in: `
				cc_library {
					name: "foo",
					foo: ["a"], // comment
					bar: ["a"],
				}
			`,
			out: `
				cc_library {
					name: "foo",
					// comment
					bar: ["a"],
				}
			`,
		},
		{
			name: "eol comment with blank lines",
			in: `
				cc_library {
					name: "foo",

					foo: ["a"], // comment

					// bar
					bar: ["a"],
				}
			`,
			out: `
				cc_library {
					name: "foo",

					// comment

					// bar
					bar: ["a"],
				}
			`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runPass(t, test.in, test.out, func(fixer *Fixer) error {
				return runPatchListMod(func(mod *parser.Module, buf []byte, patchList *parser.PatchList) error {
					return removeMatchingModuleListProperties(mod, patchList, "bar", "foo")
				})(fixer)
			})
		})
	}
}

func TestReplaceJavaStaticLibs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
	}{
		{
			name: "static lib",
			in: `
				java_library_static {
					name: "foo",
				}
			`,
			out: `
				java_library {
					name: "foo",
				}
			`,
		},
		{
			name: "java lib",
			in: `
				java_library {
					name: "foo",
				}
			`,
			out: `
				java_library {
					name: "foo",
				}
			`,
		},
		{
			name: "java installable lib",
			in: `
				java_library {
					name: "foo",
					installable: true,
				}
			`,
			out: `
				java_library {
					name: "foo",
					installable: true,
				}
			`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runPass(t, test.in, test.out, func(fixer *Fixer) error {
				return rewriteJavaStaticLibs(fixer)
			})
		})
	}
}

func TestRewritePrebuilts(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
	}{
		{
			name: "jar srcs",
			in: `
				java_import {
					name: "foo",
					srcs: ["foo.jar"],
				}
			`,
			out: `
				java_import {
					name: "foo",
					jars: ["foo.jar"],
				}
			`,
		},
		{
			name: "aar srcs",
			in: `
				java_import {
					name: "foo",
					srcs: ["foo.aar"],
					installable: true,
				}
			`,
			out: `
				android_library_import {
					name: "foo",
					aars: ["foo.aar"],

				}
			`,
		},
		{
			name: "host prebuilt",
			in: `
				java_import {
					name: "foo",
					srcs: ["foo.jar"],
					host: true,
				}
			`,
			out: `
				java_import_host {
					name: "foo",
					jars: ["foo.jar"],

				}
			`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runPass(t, test.in, test.out, func(fixer *Fixer) error {
				return rewriteIncorrectAndroidmkPrebuilts(fixer)
			})
		})
	}
}

func TestRewriteCtsModuleTypes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
	}{
		{
			name: "cts_support_package",
			in: `
				cts_support_package {
					name: "foo",
				}
			`,
			out: `
				android_test {
					name: "foo",
					defaults: ["cts_support_defaults"],
				}
			`,
		},
		{
			name: "cts_package",
			in: `
				cts_package {
					name: "foo",
				}
			`,
			out: `
				android_test {
					name: "foo",
					defaults: ["cts_defaults"],
				}
			`,
		},
		{
			name: "cts_target_java_library",
			in: `
				cts_target_java_library {
					name: "foo",
				}
			`,
			out: `
				java_library {
					name: "foo",
					defaults: ["cts_defaults"],
				}
			`,
		},
		{
			name: "cts_host_java_library",
			in: `
				cts_host_java_library {
					name: "foo",
				}
			`,
			out: `
				java_library_host {
					name: "foo",
					defaults: ["cts_defaults"],
				}
			`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runPass(t, test.in, test.out, rewriteCtsModuleTypes)
		})
	}
}

func TestRewritePrebuiltEtc(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
	}{
		{
			name: "prebuilt_etc src",
			in: `
			prebuilt_etc {
			name: "foo",
			srcs: ["bar"],
		}
		`,
			out: `prebuilt_etc {
			name: "foo",
			src: "bar",
		}
		`,
		},
		{
			name: "prebuilt_etc src",
			in: `
			prebuilt_etc {
			name: "foo",
			srcs: FOO,
		}
		`,
			out: `prebuilt_etc {
			name: "foo",
			src: FOO,
		}
		`,
		},
		{
			name: "prebuilt_etc src",
			in: `
			prebuilt_etc {
			name: "foo",
			srcs: ["bar", "baz"],
		}
		`,
			out: `prebuilt_etc {
			name: "foo",
			src: "ERROR: LOCAL_SRC_FILES should contain at most one item",

		}
		`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runPass(t, test.in, test.out, func(fixer *Fixer) error {
				return rewriteAndroidmkPrebuiltEtc(fixer)
			})
		})
	}
}

func TestRewriteAndroidTest(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
	}{
		{
			name: "android_test valid module path",
			in: `
				android_test {
					name: "foo",
					local_module_path: {
						var: "TARGET_OUT_DATA_APPS",
					},
				}
			`,
			out: `
				android_test {
					name: "foo",

				}
			`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runPass(t, test.in, test.out, func(fixer *Fixer) error {
				return rewriteAndroidTest(fixer)
			})
		})
	}
}

func TestRewriteAndroidAppImport(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
	}{
		{
			name: "android_app_import apk",
			in: `
				android_app_import {
					name: "foo",
					srcs: ["package.apk"],
				}
			`,
			out: `
				android_app_import {
					name: "foo",
					apk: "package.apk",
				}
			`,
		},
		{
			name: "android_app_import presigned",
			in: `
				android_app_import {
					name: "foo",
					apk: "package.apk",
					certificate: "PRESIGNED",
				}
			`,
			out: `
				android_app_import {
					name: "foo",
					apk: "package.apk",
					presigned: true,

				}
			`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runPass(t, test.in, test.out, func(fixer *Fixer) error {
				return rewriteAndroidAppImport(fixer)
			})
		})
	}
}
