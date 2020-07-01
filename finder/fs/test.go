// Copyright 2020 Google Inc. All rights reserved.
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

package fs

import (
	"io/ioutil"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

func Write(t *testing.T, path string, content string, filesystem *MockFs) {
	parent := filepath.Dir(path)
	filesystem.MkDirs(parent)
	err := filesystem.WriteFile(path, []byte(content), 0777)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func Create(t *testing.T, path string, filesystem *MockFs) {
	Write(t, path, "hi", filesystem)
}

func Delete(t *testing.T, path string, filesystem *MockFs) {
	err := filesystem.Remove(path)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func RemoveAll(t *testing.T, path string, filesystem *MockFs) {
	err := filesystem.RemoveAll(path)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func Move(t *testing.T, oldPath string, newPath string, filesystem *MockFs) {
	err := filesystem.Rename(oldPath, newPath)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func Link(t *testing.T, newPath string, oldPath string, filesystem *MockFs) {
	parentPath := filepath.Dir(newPath)
	err := filesystem.MkDirs(parentPath)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = filesystem.Symlink(oldPath, newPath)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func Read(t *testing.T, path string, filesystem *MockFs) string {
	reader, err := filesystem.Open(path)
	if err != nil {
		t.Fatalf(err.Error())
	}
	bytes, err := ioutil.ReadAll(reader)
	if err != nil {
		t.Fatal(err.Error())
	}
	return string(bytes)
}

func ModTime(t *testing.T, path string, filesystem *MockFs) time.Time {
	stats, err := filesystem.Lstat(path)
	if err != nil {
		t.Fatal(err.Error())
	}
	return stats.ModTime()
}

func SetReadable(t *testing.T, path string, readable bool, filesystem *MockFs) {
	err := filesystem.SetReadable(path, readable)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func SetReadErr(t *testing.T, path string, readErr error, filesystem *MockFs) {
	err := filesystem.SetReadErr(path, readErr)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func AssertSameResponse(t *testing.T, actual []string, expected []string) {
	t.Helper()
	sort.Strings(actual)
	sort.Strings(expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("Expected Finder to return these %v paths:\n  %v,\ninstead returned these %v paths:  %v\n",
			len(expected), expected, len(actual), actual)
	}
}

func AssertSameStatCalls(t *testing.T, actual []string, expected []string) {
	t.Helper()
	sort.Strings(actual)
	sort.Strings(expected)

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("Finder made incorrect Stat calls.\n"+
			"Actual:\n"+
			"%v\n"+
			"Expected:\n"+
			"%v\n"+
			"\n",
			actual, expected)
	}
}

func AssertSameReadDirCalls(t *testing.T, actual []string, expected []string) {
	t.Helper()
	sort.Strings(actual)
	sort.Strings(expected)

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("Finder made incorrect ReadDir calls.\n"+
			"Actual:\n"+
			"%v\n"+
			"Expected:\n"+
			"%v\n"+
			"\n",
			actual, expected)
	}
}
