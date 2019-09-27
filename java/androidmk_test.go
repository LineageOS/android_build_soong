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
	"reflect"
	"testing"

	"android/soong/android"
)

func TestRequired(t *testing.T) {
	ctx, config := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			required: ["libfoo"],
		}
	`)

	mod := ctx.ModuleForTests("foo", "android_common").Module()
	entries := android.AndroidMkEntriesForTest(t, config, "", mod)

	expected := []string{"libfoo"}
	actual := entries.EntryMap["LOCAL_REQUIRED_MODULES"]
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Unexpected required modules - expected: %q, actual: %q", expected, actual)
	}
}

func TestHostdex(t *testing.T) {
	ctx, config := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			hostdex: true,
		}
	`)

	mod := ctx.ModuleForTests("foo", "android_common").Module()
	entries := android.AndroidMkEntriesForTest(t, config, "", mod)

	expected := []string{"foo"}
	actual := entries.EntryMap["LOCAL_MODULE"]
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Unexpected module name - expected: %q, actual: %q", expected, actual)
	}

	footerLines := entries.FooterLinesForTests()
	if !android.InList("LOCAL_MODULE := foo-hostdex", footerLines) {
		t.Errorf("foo-hostdex is not found in the footers: %q", footerLines)
	}
}

func TestHostdexRequired(t *testing.T) {
	ctx, config := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			hostdex: true,
			required: ["libfoo"],
		}
	`)

	mod := ctx.ModuleForTests("foo", "android_common").Module()
	entries := android.AndroidMkEntriesForTest(t, config, "", mod)

	expected := []string{"libfoo"}
	actual := entries.EntryMap["LOCAL_REQUIRED_MODULES"]
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Unexpected required modules - expected: %q, actual: %q", expected, actual)
	}

	footerLines := entries.FooterLinesForTests()
	if !android.InList("LOCAL_REQUIRED_MODULES := libfoo", footerLines) {
		t.Errorf("Wrong or missing required line for foo-hostdex in the footers: %q", footerLines)
	}
}

func TestHostdexSpecificRequired(t *testing.T) {
	ctx, config := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			hostdex: true,
			target: {
				hostdex: {
					required: ["libfoo"],
				},
			},
		}
	`)

	mod := ctx.ModuleForTests("foo", "android_common").Module()
	entries := android.AndroidMkEntriesForTest(t, config, "", mod)

	if r, ok := entries.EntryMap["LOCAL_REQUIRED_MODULES"]; ok {
		t.Errorf("Unexpected required modules: %q", r)
	}

	footerLines := entries.FooterLinesForTests()
	if !android.InList("LOCAL_REQUIRED_MODULES += libfoo", footerLines) {
		t.Errorf("Wrong or missing required line for foo-hostdex in the footers: %q", footerLines)
	}
}
