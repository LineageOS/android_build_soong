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

package bp2build

import (
	"sort"
	"testing"
)

type bazelFilepath struct {
	dir      string
	basename string
}

func TestCreateBazelFiles_QueryView_AddsTopLevelFiles(t *testing.T) {
	files := CreateBazelFiles(map[string]RuleShim{}, map[string]BazelTargets{}, QueryView)
	expectedFilePaths := []bazelFilepath{
		{
			dir:      "",
			basename: "BUILD.bazel",
		},
		{
			dir:      "",
			basename: "WORKSPACE",
		},
		{
			dir:      bazelRulesSubDir,
			basename: "BUILD.bazel",
		},
		{
			dir:      bazelRulesSubDir,
			basename: "providers.bzl",
		},
		{
			dir:      bazelRulesSubDir,
			basename: "soong_module.bzl",
		},
	}

	// Compare number of files
	if a, e := len(files), len(expectedFilePaths); a != e {
		t.Errorf("Expected %d files, got %d", e, a)
	}

	// Sort the files to be deterministic
	sort.Slice(files, func(i, j int) bool {
		if dir1, dir2 := files[i].Dir, files[j].Dir; dir1 == dir2 {
			return files[i].Basename < files[j].Basename
		} else {
			return dir1 < dir2
		}
	})

	// Compare the file contents
	for i := range files {
		actualFile, expectedFile := files[i], expectedFilePaths[i]

		if actualFile.Dir != expectedFile.dir || actualFile.Basename != expectedFile.basename {
			t.Errorf("Did not find expected file %s/%s", actualFile.Dir, actualFile.Basename)
		} else if actualFile.Basename == "BUILD.bazel" || actualFile.Basename == "WORKSPACE" {
			if actualFile.Contents != "" {
				t.Errorf("Expected %s to have no content.", actualFile)
			}
		} else if actualFile.Contents == "" {
			t.Errorf("Contents of %s unexpected empty.", actualFile)
		}
	}
}
