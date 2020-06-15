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

	withoutTagEntries := android.AndroidMkEntriesForTest(t, config, "", ctx.ModuleForTests("foo_without_tag", "android_common").Module())
	withTagEntries := android.AndroidMkEntriesForTest(t, config, "", ctx.ModuleForTests("foo_with_tag", "android_common").Module())

	if len(withoutTagEntries) != 2 || len(withTagEntries) != 2 {
		t.Errorf("two mk entries per module expected, got %d and %d", len(withoutTagEntries), len(withTagEntries))
	}
	if len(withTagEntries[0].DistFiles[".jar"]) != 1 ||
		!strings.Contains(withTagEntries[0].DistFiles[".jar"][0].String(), "/javac/foo_with_tag.jar") {
		t.Errorf("expected DistFiles to contain classes.jar, got %v", withTagEntries[0].DistFiles)
	}
	if len(withoutTagEntries[0].DistFiles[".jar"]) > 0 {
		t.Errorf("did not expect explicit DistFile for .jar tag, got %v", withoutTagEntries[0].DistFiles[".jar"])
	}
}

func TestDistWithDest(t *testing.T) {
	ctx, config := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			compile_dex: true,
			dist: {
				targets: ["my_goal"],
				dest: "my/custom/dest/dir",
			},
		}
	`)

	module := ctx.ModuleForTests("foo", "android_common").Module()
	entries := android.AndroidMkEntriesForTest(t, config, "", module)
	if len(entries) != 2 {
		t.Errorf("Expected 2 AndroidMk entries, got %d", len(entries))
	}

	distStrings := entries[0].GetDistForGoals(module)

	if len(distStrings) != 2 {
		t.Errorf("Expected 2 entries for dist: PHONY and dist-for-goals, but got %q", distStrings)
	}

	if distStrings[0] != ".PHONY: my_goal\n" {
		t.Errorf("Expected .PHONY entry to declare my_goal, but got: %s", distStrings[0])
	}

	if !strings.Contains(distStrings[1], "$(call dist-for-goals,my_goal") ||
		!strings.Contains(distStrings[1], ".intermediates/foo/android_common/dex/foo.jar:my/custom/dest/dir") {
		t.Errorf(
			"Expected dist-for-goals entry to contain my_goal and new dest dir, but got: %s", distStrings[1])
	}
}

func TestDistsWithAllProperties(t *testing.T) {
	ctx, config := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			compile_dex: true,
			dist: {
				targets: ["baz"],
			},
			dists: [
				{
					targets: ["bar"],
					tag: ".jar",
					dest: "bar.jar",
					dir: "bar/dir",
					suffix: ".qux",
				},
			]
		}
	`)

	module := ctx.ModuleForTests("foo", "android_common").Module()
	entries := android.AndroidMkEntriesForTest(t, config, "", module)
	if len(entries) != 2 {
		t.Errorf("Expected 2 AndroidMk entries, got %d", len(entries))
	}

	distStrings := entries[0].GetDistForGoals(module)

	if len(distStrings) != 4 {
		t.Errorf("Expected 4 entries for dist: PHONY and dist-for-goals, but got %d", len(distStrings))
	}

	if distStrings[0] != ".PHONY: bar\n" {
		t.Errorf("Expected .PHONY entry to declare bar, but got: %s", distStrings[0])
	}

	if !strings.Contains(distStrings[1], "$(call dist-for-goals,bar") ||
		!strings.Contains(
			distStrings[1],
			".intermediates/foo/android_common/javac/foo.jar:bar/dir/bar.qux.jar") {
		t.Errorf(
			"Expected dist-for-goals entry to contain bar and new dest dir, but got: %s", distStrings[1])
	}

	if distStrings[2] != ".PHONY: baz\n" {
		t.Errorf("Expected .PHONY entry to declare baz, but got: %s", distStrings[2])
	}

	if !strings.Contains(distStrings[3], "$(call dist-for-goals,baz") ||
		!strings.Contains(distStrings[3], ".intermediates/foo/android_common/dex/foo.jar:foo.jar") {
		t.Errorf(
			"Expected dist-for-goals entry to contain my_other_goal and new dest dir, but got: %s",
			distStrings[3])
	}
}

func TestDistsWithTag(t *testing.T) {
	ctx, config := testJava(t, `
		java_library {
			name: "foo_without_tag",
			srcs: ["a.java"],
			compile_dex: true,
			dists: [
				{
					targets: ["hi"],
				},
			],
		}
		java_library {
			name: "foo_with_tag",
			srcs: ["a.java"],
			compile_dex: true,
			dists: [
				{
					targets: ["hi"],
					tag: ".jar",
				},
			],
		}
	`)

	moduleWithoutTag := ctx.ModuleForTests("foo_without_tag", "android_common").Module()
	moduleWithTag := ctx.ModuleForTests("foo_with_tag", "android_common").Module()

	withoutTagEntries := android.AndroidMkEntriesForTest(t, config, "", moduleWithoutTag)
	withTagEntries := android.AndroidMkEntriesForTest(t, config, "", moduleWithTag)

	if len(withoutTagEntries) != 2 || len(withTagEntries) != 2 {
		t.Errorf("two mk entries per module expected, got %d and %d", len(withoutTagEntries), len(withTagEntries))
	}

	distFilesWithoutTag := withoutTagEntries[0].DistFiles
	distFilesWithTag := withTagEntries[0].DistFiles

	if len(distFilesWithTag[".jar"]) != 1 ||
		!strings.Contains(distFilesWithTag[".jar"][0].String(), "/javac/foo_with_tag.jar") {
		t.Errorf("expected foo_with_tag's .jar-tagged DistFiles to contain classes.jar, got %v", distFilesWithTag[".jar"])
	}
	if len(distFilesWithoutTag[".jar"]) > 0 {
		t.Errorf("did not expect foo_without_tag's .jar-tagged DistFiles to contain files, but got %v", distFilesWithoutTag[".jar"])
	}
}
