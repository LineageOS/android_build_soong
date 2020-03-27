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
	"strings"
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
	entries := android.AndroidMkEntriesForTest(t, config, "", mod)[0]

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
	entriesList := android.AndroidMkEntriesForTest(t, config, "", mod)
	if len(entriesList) != 2 {
		t.Errorf("two entries are expected, but got %d", len(entriesList))
	}

	mainEntries := &entriesList[0]
	expected := []string{"foo"}
	actual := mainEntries.EntryMap["LOCAL_MODULE"]
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Unexpected module name - expected: %q, actual: %q", expected, actual)
	}

	subEntries := &entriesList[1]
	expected = []string{"foo-hostdex"}
	actual = subEntries.EntryMap["LOCAL_MODULE"]
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Unexpected module name - expected: %q, actual: %q", expected, actual)
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
	entriesList := android.AndroidMkEntriesForTest(t, config, "", mod)
	if len(entriesList) != 2 {
		t.Errorf("two entries are expected, but got %d", len(entriesList))
	}

	mainEntries := &entriesList[0]
	expected := []string{"libfoo"}
	actual := mainEntries.EntryMap["LOCAL_REQUIRED_MODULES"]
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Unexpected required modules - expected: %q, actual: %q", expected, actual)
	}

	subEntries := &entriesList[1]
	expected = []string{"libfoo"}
	actual = subEntries.EntryMap["LOCAL_REQUIRED_MODULES"]
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Unexpected required modules - expected: %q, actual: %q", expected, actual)
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
	entriesList := android.AndroidMkEntriesForTest(t, config, "", mod)
	if len(entriesList) != 2 {
		t.Errorf("two entries are expected, but got %d", len(entriesList))
	}

	mainEntries := &entriesList[0]
	if r, ok := mainEntries.EntryMap["LOCAL_REQUIRED_MODULES"]; ok {
		t.Errorf("Unexpected required modules: %q", r)
	}

	subEntries := &entriesList[1]
	expected := []string{"libfoo"}
	actual := subEntries.EntryMap["LOCAL_REQUIRED_MODULES"]
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Unexpected required modules - expected: %q, actual: %q", expected, actual)
	}
}

func TestDistWithTag(t *testing.T) {
	ctx, config := testJava(t, `
		java_library {
			name: "foo_without_tag",
			srcs: ["a.java"],
			compile_dex: true,
			dist: {
				targets: ["hi"],
			},
		}
		java_library {
			name: "foo_with_tag",
			srcs: ["a.java"],
			compile_dex: true,
			dist: {
				targets: ["hi"],
				tag: ".jar",
			},
		}
	`)

	without_tag_entries := android.AndroidMkEntriesForTest(t, config, "", ctx.ModuleForTests("foo_without_tag", "android_common").Module())
	with_tag_entries := android.AndroidMkEntriesForTest(t, config, "", ctx.ModuleForTests("foo_with_tag", "android_common").Module())

	if len(without_tag_entries) != 2 || len(with_tag_entries) != 2 {
		t.Errorf("two mk entries per module expected, got %d and %d", len(without_tag_entries), len(with_tag_entries))
	}
	if !with_tag_entries[0].DistFile.Valid() || !strings.Contains(with_tag_entries[0].DistFile.String(), "/javac/foo_with_tag.jar") {
		t.Errorf("expected classes.jar DistFile, got %v", with_tag_entries[0].DistFile)
	}
	if without_tag_entries[0].DistFile.Valid() {
		t.Errorf("did not expect explicit DistFile, got %v", without_tag_entries[0].DistFile)
	}
}
