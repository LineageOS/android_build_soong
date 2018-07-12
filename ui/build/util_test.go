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
