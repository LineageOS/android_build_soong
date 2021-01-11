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

func TestCreateBazelFiles_AddsTopLevelFiles(t *testing.T) {
	files := CreateBazelFiles(map[string]RuleShim{}, map[string][]BazelTarget{})
	expectedFilePaths := []struct {
		dir      string
		basename string
	}{
		{
			dir:      "",
			basename: "BUILD",
		},
		{
			dir:      "",
			basename: "WORKSPACE",
		},
		{
			dir:      bazelRulesSubDir,
			basename: "BUILD",
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

	if g, w := len(files), len(expectedFilePaths); g != w {
		t.Errorf("Expected %d files, got %d", w, g)
	}

	sort.Slice(files, func(i, j int) bool {
		if dir1, dir2 := files[i].Dir, files[j].Dir; dir1 == dir2 {
			return files[i].Basename < files[j].Basename
		} else {
			return dir1 < dir2
		}
	})

	for i := range files {
		if g, w := files[i], expectedFilePaths[i]; g.Dir != w.dir || g.Basename != w.basename {
			t.Errorf("Did not find expected file %s/%s", g.Dir, g.Basename)
		} else if g.Basename == "BUILD" || g.Basename == "WORKSPACE" {
			if g.Contents != "" {
				t.Errorf("Expected %s to have no content.", g)
			}
		} else if g.Contents == "" {
			t.Errorf("Contents of %s unexpected empty.", g)
		}
	}
}
