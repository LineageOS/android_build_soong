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

	"android/soong/android"
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

func TestCreateBazelFiles_Bp2Build_CreatesDefaultFiles(t *testing.T) {
	testConfig := android.TestConfig("", make(map[string]string), "", make(map[string][]byte))
	codegenCtx := NewCodegenContext(testConfig, android.NewTestContext(testConfig).Context, Bp2Build, "")
	files, err := createSoongInjectionDirFiles(codegenCtx, CreateCodegenMetrics())
	if err != nil {
		t.Error(err)
	}
	expectedFilePaths := []bazelFilepath{
		{
			dir:      "android",
			basename: GeneratedBuildFileName,
		},
		{
			dir:      "android",
			basename: "constants.bzl",
		},
		{
			dir:      "cc_toolchain",
			basename: GeneratedBuildFileName,
		},
		{
			dir:      "cc_toolchain",
			basename: "config_constants.bzl",
		},
		{
			dir:      "cc_toolchain",
			basename: "ndk_libs.bzl",
		},
		{
			dir:      "cc_toolchain",
			basename: "sanitizer_constants.bzl",
		},
		{
			dir:      "java_toolchain",
			basename: GeneratedBuildFileName,
		},
		{
			dir:      "java_toolchain",
			basename: "constants.bzl",
		},
		{
			dir:      "rust_toolchain",
			basename: GeneratedBuildFileName,
		},
		{
			dir:      "rust_toolchain",
			basename: "constants.bzl",
		},
		{
			dir:      "apex_toolchain",
			basename: GeneratedBuildFileName,
		},
		{
			dir:      "apex_toolchain",
			basename: "constants.bzl",
		},
		{
			dir:      "metrics",
			basename: "converted_modules.json",
		},
		{
			dir:      "metrics",
			basename: "BUILD.bazel",
		},
		{
			dir:      "metrics",
			basename: "converted_modules_path_map.json",
		},
		{
			dir:      "metrics",
			basename: "converted_modules_path_map.bzl",
		},
		{
			dir:      "product_config",
			basename: "soong_config_variables.bzl",
		},
		{
			dir:      "product_config",
			basename: "arch_configuration.bzl",
		},
		{
			dir:      "api_levels",
			basename: GeneratedBuildFileName,
		},
		{
			dir:      "api_levels",
			basename: "api_levels.json",
		},
		{
			dir:      "api_levels",
			basename: "platform_versions.bzl",
		},
		{
			dir:      "allowlists",
			basename: GeneratedBuildFileName,
		},
		{
			dir:      "allowlists",
			basename: "mixed_build_prod_allowlist.txt",
		},
		{
			dir:      "allowlists",
			basename: "mixed_build_staging_allowlist.txt",
		},
	}

	less := func(a bazelFilepath, b bazelFilepath) bool {
		return a.dir+"/"+a.basename < b.dir+"/"+b.basename
	}

	fileToFilepath := func(a BazelFile) bazelFilepath {
		return bazelFilepath{basename: a.Basename, dir: a.Dir}
	}

	sort.Slice(expectedFilePaths, func(i, j int) bool {
		return less(expectedFilePaths[i], expectedFilePaths[j])
	})
	sort.Slice(files, func(i, j int) bool {
		return less(fileToFilepath(files[i]), fileToFilepath(files[j]))
	})

	i := 0
	j := 0
	for i < len(expectedFilePaths) && j < len(files) {
		expectedFile, actualFile := expectedFilePaths[i], files[j]

		if actualFile.Dir == expectedFile.dir && actualFile.Basename == expectedFile.basename {
			i++
			j++
		} else if less(expectedFile, fileToFilepath(actualFile)) {
			t.Errorf("Did not find expected file %s/%s", expectedFile.dir, expectedFile.basename)
			i++
		} else {
			t.Errorf("Found unexpected file %s/%s", actualFile.Dir, actualFile.Basename)
			j++
		}
	}
	for i < len(expectedFilePaths) {
		expectedFile := expectedFilePaths[i]
		t.Errorf("Did not find expected file %s/%s", expectedFile.dir, expectedFile.basename)
		i++
	}
	for j < len(files) {
		actualFile := files[j]
		t.Errorf("Found unexpected file %s/%s", actualFile.Dir, actualFile.Basename)
		j++
	}
}
