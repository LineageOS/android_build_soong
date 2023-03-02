// Copyright 2023 Google Inc. All rights reserved.
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

package build

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func assertDeepEqual(t *testing.T, expected interface{}, actual interface{}) {
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("expected:\n  %#v\n actual:\n  %#v", expected, actual)
	}
}

// Make a temp directory containing the supplied contents
func makeTempDir(files []string, directories []string, symlinks []string) string {
	temp, _ := os.MkdirTemp("", "soon_staging_snapshot_test_")

	for _, file := range files {
		os.MkdirAll(temp+"/"+filepath.Dir(file), 0700)
		os.WriteFile(temp+"/"+file, []byte(file), 0600)
	}

	for _, dir := range directories {
		os.MkdirAll(temp+"/"+dir, 0770)
	}

	for _, symlink := range symlinks {
		os.MkdirAll(temp+"/"+filepath.Dir(symlink), 0770)
		os.Symlink(temp, temp+"/"+symlink)
	}

	return temp
}

// If this is a clean build, we won't have any preexisting files, make sure we get back an empty
// list and not errors.
func TestEmptyOut(t *testing.T) {
	ctx := testContext()

	temp := makeTempDir(nil, nil, nil)
	defer os.RemoveAll(temp)

	actual, _ := takeStagingSnapshot(ctx, temp, []string{"a", "e", "g"})

	expected := []fileEntry{}

	assertDeepEqual(t, expected, actual)
}

// Make sure only the listed directories are picked up, and only regular files
func TestNoExtraSubdirs(t *testing.T) {
	ctx := testContext()

	temp := makeTempDir([]string{"a/b", "a/c", "d", "e/f"}, []string{"g/h"}, []string{"e/symlink"})
	defer os.RemoveAll(temp)

	actual, _ := takeStagingSnapshot(ctx, temp, []string{"a", "e", "g"})

	expected := []fileEntry{
		{"a/b", 0600, 3, "3ec69c85a4ff96830024afeef2d4e512181c8f7b"},
		{"a/c", 0600, 3, "592d70e4e03ee6f6780c71b0bf3b9608dbf1e201"},
		{"e/f", 0600, 3, "9e164bef74aceede0974b857170100409efe67f1"},
	}

	assertDeepEqual(t, expected, actual)
}

// Make sure diff handles empty lists
func TestDiffEmpty(t *testing.T) {
	actual := diffSnapshots(nil, []fileEntry{})

	expected := snapshotDiff{
		Added:   []string{},
		Changed: []string{},
		Removed: []string{},
	}

	assertDeepEqual(t, expected, actual)
}

// Make sure diff handles adding
func TestDiffAdd(t *testing.T) {
	actual := diffSnapshots([]fileEntry{
		{"a", 0600, 1, "1234"},
	}, []fileEntry{
		{"a", 0600, 1, "1234"},
		{"b", 0700, 2, "5678"},
	})

	expected := snapshotDiff{
		Added:   []string{"b"},
		Changed: []string{},
		Removed: []string{},
	}

	assertDeepEqual(t, expected, actual)
}

// Make sure diff handles changing mode
func TestDiffChangeMode(t *testing.T) {
	actual := diffSnapshots([]fileEntry{
		{"a", 0600, 1, "1234"},
		{"b", 0700, 2, "5678"},
	}, []fileEntry{
		{"a", 0600, 1, "1234"},
		{"b", 0600, 2, "5678"},
	})

	expected := snapshotDiff{
		Added:   []string{},
		Changed: []string{"b"},
		Removed: []string{},
	}

	assertDeepEqual(t, expected, actual)
}

// Make sure diff handles changing size
func TestDiffChangeSize(t *testing.T) {
	actual := diffSnapshots([]fileEntry{
		{"a", 0600, 1, "1234"},
		{"b", 0700, 2, "5678"},
	}, []fileEntry{
		{"a", 0600, 1, "1234"},
		{"b", 0700, 3, "5678"},
	})

	expected := snapshotDiff{
		Added:   []string{},
		Changed: []string{"b"},
		Removed: []string{},
	}

	assertDeepEqual(t, expected, actual)
}

// Make sure diff handles changing contents
func TestDiffChangeContents(t *testing.T) {
	actual := diffSnapshots([]fileEntry{
		{"a", 0600, 1, "1234"},
		{"b", 0700, 2, "5678"},
	}, []fileEntry{
		{"a", 0600, 1, "1234"},
		{"b", 0700, 2, "aaaa"},
	})

	expected := snapshotDiff{
		Added:   []string{},
		Changed: []string{"b"},
		Removed: []string{},
	}

	assertDeepEqual(t, expected, actual)
}

// Make sure diff handles removing
func TestDiffRemove(t *testing.T) {
	actual := diffSnapshots([]fileEntry{
		{"a", 0600, 1, "1234"},
		{"b", 0700, 2, "5678"},
	}, []fileEntry{
		{"a", 0600, 1, "1234"},
	})

	expected := snapshotDiff{
		Added:   []string{},
		Changed: []string{},
		Removed: []string{"b"},
	}

	assertDeepEqual(t, expected, actual)
}
