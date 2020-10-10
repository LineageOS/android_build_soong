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

package build

import (
	"android/soong/ui/logger"
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestCleanOldFiles(t *testing.T) {
	dir, err := ioutil.TempDir("", "testcleanoldfiles")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	ctx := testContext()
	logBuf := &bytes.Buffer{}
	ctx.Logger = logger.New(logBuf)

	touch := func(names ...string) {
		for _, name := range names {
			if f, err := os.Create(filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			} else {
				f.Close()
			}
		}
	}
	runCleanOldFiles := func(names ...string) {
		data := []byte(strings.Join(names, " "))
		if err := ioutil.WriteFile(filepath.Join(dir, ".installed"), data, 0666); err != nil {
			t.Fatal(err)
		}

		cleanOldFiles(ctx, dir, ".installed")
	}

	assertFileList := func(names ...string) {
		t.Helper()

		sort.Strings(names)

		var foundNames []string
		if foundFiles, err := ioutil.ReadDir(dir); err == nil {
			for _, fi := range foundFiles {
				foundNames = append(foundNames, fi.Name())
			}
		} else {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(names, foundNames) {
			t.Errorf("Expected a different list of files:\nwant: %v\n got: %v", names, foundNames)
			t.Error("Log: ", logBuf.String())
			logBuf.Reset()
		}
	}

	// Initial list of potential files
	runCleanOldFiles("foo", "bar")
	touch("foo", "bar", "baz")
	assertFileList("foo", "bar", "baz", ".installed.previous")

	// This should be a no-op, as the list hasn't changed
	runCleanOldFiles("foo", "bar")
	assertFileList("foo", "bar", "baz", ".installed", ".installed.previous")

	// This should be a no-op, as only a file was added
	runCleanOldFiles("foo", "bar", "foo2")
	assertFileList("foo", "bar", "baz", ".installed.previous")

	// "bar" should be removed, foo2 should be ignored as it was never there
	runCleanOldFiles("foo")
	assertFileList("foo", "baz", ".installed.previous")

	// Recreate bar, and create foo2. Ensure that they aren't removed
	touch("bar", "foo2")
	runCleanOldFiles("foo", "baz")
	assertFileList("foo", "bar", "baz", "foo2", ".installed.previous")
}
