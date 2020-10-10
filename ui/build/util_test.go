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

package build

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"android/soong/ui/logger"
)

func TestEnsureEmptyDirs(t *testing.T) {
	ctx := testContext()
	defer logger.Recover(func(err error) {
		t.Error(err)
	})

	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			t.Errorf("Error removing tmpDir: %v", err)
		}
	}()

	ensureEmptyDirectoriesExist(ctx, filepath.Join(tmpDir, "a/b"))

	err = os.Chmod(filepath.Join(tmpDir, "a"), 0555)
	if err != nil {
		t.Fatalf("Failed to chown: %v", err)
	}

	ensureEmptyDirectoriesExist(ctx, filepath.Join(tmpDir, "a"))
}

func TestCopyFile(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "test_copy_file")
	if err != nil {
		t.Fatalf("failed to create temporary directory to hold test text files: %v", err)
	}
	defer os.Remove(tmpDir)

	data := []byte("fake data")
	src := filepath.Join(tmpDir, "src.txt")
	if err := ioutil.WriteFile(src, data, 0755); err != nil {
		t.Fatalf("failed to create a src file %q for copying: %v", src, err)
	}

	dst := filepath.Join(tmpDir, "dst.txt")

	l, err := copyFile(src, dst)
	if err != nil {
		t.Fatalf("got %v, expecting nil error on copyFile operation", err)
	}

	if l != int64(len(data)) {
		t.Errorf("got %d, expecting %d for copied bytes", l, len(data))
	}

	dstData, err := ioutil.ReadFile(dst)
	if err != nil {
		t.Fatalf("got %v, expecting nil error reading dst %q file", err, dst)
	}

	if bytes.Compare(data, dstData) != 0 {
		t.Errorf("got %q, expecting data %q from dst %q text file", string(data), string(dstData), dst)
	}
}

func TestCopyFileErrors(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "test_copy_file_errors")
	if err != nil {
		t.Fatalf("failed to create temporary directory to hold test text files: %v", err)
	}
	defer os.Remove(tmpDir)

	srcExists := filepath.Join(tmpDir, "src_exist.txt")
	if err := ioutil.WriteFile(srcExists, []byte("fake data"), 0755); err != nil {
		t.Fatalf("failed to create a src file %q for copying: %v", srcExists, err)
	}

	tests := []struct {
		description string
		src         string
		dst         string
	}{{
		description: "src file does not exist",
		src:         "/src/not/exist",
		dst:         "/dst/not/exist",
	}, {
		description: "dst directory does not exist",
		src:         srcExists,
		dst:         "/dst/not/exist",
	}}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			if _, err := copyFile(tt.src, tt.dst); err == nil {
				t.Errorf("got nil, expecting error")
			}
		})
	}
}
