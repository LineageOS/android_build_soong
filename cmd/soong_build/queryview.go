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

package main

import (
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"

	"android/soong/android"
	"android/soong/bp2build"
	"android/soong/starlark_import"
)

// A helper function to generate a Read-only Bazel workspace in outDir
func createBazelWorkspace(ctx *bp2build.CodegenContext, outDir string, generateFilegroups bool) error {
	os.RemoveAll(outDir)
	ruleShims := bp2build.CreateRuleShims(android.ModuleTypeFactories())

	res, err := bp2build.GenerateBazelTargets(ctx, generateFilegroups)
	if err != nil {
		panic(err)
	}

	filesToWrite := bp2build.CreateBazelFiles(ruleShims, res.BuildDirToTargets(), ctx.Mode())
	bazelRcFiles, err2 := CopyBazelRcFiles()
	if err2 != nil {
		return err2
	}
	filesToWrite = append(filesToWrite, bazelRcFiles...)
	for _, f := range filesToWrite {
		if err := writeReadOnlyFile(outDir, f); err != nil {
			return err
		}
	}

	// Add starlark deps here, so that they apply to both queryview and apibp2build which
	// both run this function.
	starlarkDeps, err2 := starlark_import.GetNinjaDeps()
	if err2 != nil {
		return err2
	}
	ctx.AddNinjaFileDeps(starlarkDeps...)

	return nil
}

// CopyBazelRcFiles creates BazelFiles for all the bazelrc files under
// build/bazel. They're needed because the rc files are still read when running
// queryview, so they have to be in the queryview workspace.
func CopyBazelRcFiles() ([]bp2build.BazelFile, error) {
	result := make([]bp2build.BazelFile, 0)
	err := filepath.WalkDir(filepath.Join(topDir, "build/bazel"), func(path string, info fs.DirEntry, err error) error {
		if filepath.Ext(path) == ".bazelrc" {
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			path, err = filepath.Rel(topDir, path)
			if err != nil {
				return err
			}
			result = append(result, bp2build.BazelFile{
				Dir:      filepath.Dir(path),
				Basename: filepath.Base(path),
				Contents: string(contents),
			})
		}
		return nil
	})
	return result, err
}

// The auto-conversion directory should be read-only, sufficient for bazel query. The files
// are not intended to be edited by end users.
func writeReadOnlyFile(dir string, f bp2build.BazelFile) error {
	dir = filepath.Join(dir, f.Dir)
	if err := createDirectoryIfNonexistent(dir); err != nil {
		return err
	}
	pathToFile := filepath.Join(dir, f.Basename)

	// 0444 is read-only
	err := ioutil.WriteFile(pathToFile, []byte(f.Contents), 0444)

	return err
}

func writeReadWriteFile(dir string, f bp2build.BazelFile) error {
	dir = filepath.Join(dir, f.Dir)
	if err := createDirectoryIfNonexistent(dir); err != nil {
		return err
	}
	pathToFile := filepath.Join(dir, f.Basename)

	// 0644 is read-write
	err := ioutil.WriteFile(pathToFile, []byte(f.Contents), 0644)

	return err
}

func createDirectoryIfNonexistent(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, os.ModePerm)
	} else {
		return err
	}
}
