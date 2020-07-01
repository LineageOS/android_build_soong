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
	"os"
	"testing"
)

func TestMockFs_LstatStatSymlinks(t *testing.T) {
	// setup filesystem
	filesystem := NewMockFs(nil)
	Create(t, "/tmp/realdir/hi.txt", filesystem)
	Create(t, "/tmp/realdir/ignoreme.txt", filesystem)

	Link(t, "/tmp/links/dir", "../realdir", filesystem)
	Link(t, "/tmp/links/file", "../realdir/hi.txt", filesystem)
	Link(t, "/tmp/links/broken", "nothingHere", filesystem)
	Link(t, "/tmp/links/recursive", "recursive", filesystem)

	assertStat := func(t *testing.T, stat os.FileInfo, err error, wantName string, wantMode os.FileMode) {
		t.Helper()
		if err != nil {
			t.Error(err)
			return
		}
		if g, w := stat.Name(), wantName; g != w {
			t.Errorf("want name %q, got %q", w, g)
		}
		if g, w := stat.Mode(), wantMode; g != w {
			t.Errorf("%s: want mode %q, got %q", wantName, w, g)
		}
	}

	assertErr := func(t *testing.T, err error, wantErr string) {
		if err == nil || err.Error() != wantErr {
			t.Errorf("want error %q, got %q", wantErr, err)
		}
	}

	stat, err := filesystem.Lstat("/tmp/links/dir")
	assertStat(t, stat, err, "dir", os.ModeSymlink)

	stat, err = filesystem.Stat("/tmp/links/dir")
	assertStat(t, stat, err, "realdir", os.ModeDir)

	stat, err = filesystem.Lstat("/tmp/links/file")
	assertStat(t, stat, err, "file", os.ModeSymlink)

	stat, err = filesystem.Stat("/tmp/links/file")
	assertStat(t, stat, err, "hi.txt", 0)

	stat, err = filesystem.Lstat("/tmp/links/broken")
	assertStat(t, stat, err, "broken", os.ModeSymlink)

	stat, err = filesystem.Stat("/tmp/links/broken")
	assertErr(t, err, "stat /tmp/links/nothingHere: file does not exist")

	stat, err = filesystem.Lstat("/tmp/links/recursive")
	assertStat(t, stat, err, "recursive", os.ModeSymlink)

	stat, err = filesystem.Stat("/tmp/links/recursive")
	assertErr(t, err, "read /tmp/links/recursive: too many levels of symbolic links")
}
