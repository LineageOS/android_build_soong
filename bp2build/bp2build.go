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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/bazel"
	"android/soong/shared"
	"android/soong/starlark_import"
)

func deleteFilesExcept(ctx *CodegenContext, rootOutputPath android.OutputPath, except []BazelFile) {
	// Delete files that should no longer be present.
	bp2buildDirAbs := shared.JoinPath(ctx.topDir, rootOutputPath.String())

	filesToDelete := make(map[string]struct{})
	err := filepath.Walk(bp2buildDirAbs,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				relPath, err := filepath.Rel(bp2buildDirAbs, path)
				if err != nil {
					return err
				}
				filesToDelete[relPath] = struct{}{}
			}
			return nil
		})
	if err != nil {
		fmt.Printf("ERROR reading %s: %s", bp2buildDirAbs, err)
		os.Exit(1)
	}

	for _, bazelFile := range except {
		filePath := filepath.Join(bazelFile.Dir, bazelFile.Basename)
		delete(filesToDelete, filePath)
	}
	for f, _ := range filesToDelete {
		absPath := shared.JoinPath(bp2buildDirAbs, f)
		if err := os.RemoveAll(absPath); err != nil {
			fmt.Printf("ERROR deleting %s: %s", absPath, err)
			os.Exit(1)
		}
	}
}

// Codegen is the backend of bp2build. The code generator is responsible for
// writing .bzl files that are equivalent to Android.bp files that are capable
// of being built with Bazel.
func Codegen(ctx *CodegenContext) *CodegenMetrics {
	ctx.Context().BeginEvent("Codegen")
	defer ctx.Context().EndEvent("Codegen")
	// This directory stores BUILD files that could be eventually checked-in.
	bp2buildDir := android.PathForOutput(ctx, "bp2build")

	res, errs := GenerateBazelTargets(ctx, true)
	if len(errs) > 0 {
		errMsgs := make([]string, len(errs))
		for i, err := range errs {
			errMsgs[i] = fmt.Sprintf("%q", err)
		}
		fmt.Printf("ERROR: Encountered %d error(s): \nERROR: %s", len(errs), strings.Join(errMsgs, "\n"))
		os.Exit(1)
	}
	var bp2buildFiles []BazelFile
	productConfig, err := createProductConfigFiles(ctx, res.metrics)
	ctx.Context().EventHandler.Do("CreateBazelFile", func() {
		allTargets := make(map[string]BazelTargets)
		for k, v := range res.buildFileToTargets {
			allTargets[k] = append(allTargets[k], v...)
		}
		for k, v := range productConfig.bp2buildTargets {
			allTargets[k] = append(allTargets[k], v...)
		}
		bp2buildFiles = CreateBazelFiles(nil, allTargets, ctx.mode)
	})
	bp2buildFiles = append(bp2buildFiles, productConfig.bp2buildFiles...)
	injectionFiles, err := createSoongInjectionDirFiles(ctx, res.metrics)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		os.Exit(1)
	}
	injectionFiles = append(injectionFiles, productConfig.injectionFiles...)

	writeFiles(ctx, bp2buildDir, bp2buildFiles)
	// Delete files under the bp2build root which weren't just written. An
	// alternative would have been to delete the whole directory and write these
	// files. However, this would regenerate files which were otherwise unchanged
	// since the last bp2build run, which would have negative incremental
	// performance implications.
	deleteFilesExcept(ctx, bp2buildDir, bp2buildFiles)

	writeFiles(ctx, android.PathForOutput(ctx, bazel.SoongInjectionDirName), injectionFiles)
	starlarkDeps, err := starlark_import.GetNinjaDeps()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
	ctx.AddNinjaFileDeps(starlarkDeps...)
	return &res.metrics
}

// Get the output directory and create it if it doesn't exist.
func getOrCreateOutputDir(outputDir android.OutputPath, ctx android.PathContext, dir string) android.OutputPath {
	dirPath := outputDir.Join(ctx, dir)
	if err := android.CreateOutputDirIfNonexistent(dirPath, os.ModePerm); err != nil {
		fmt.Printf("ERROR: path %s: %s", dirPath, err.Error())
	}
	return dirPath
}

// writeFiles materializes a list of BazelFile rooted at outputDir.
func writeFiles(ctx android.PathContext, outputDir android.OutputPath, files []BazelFile) {
	for _, f := range files {
		p := getOrCreateOutputDir(outputDir, ctx, f.Dir).Join(ctx, f.Basename)
		if err := writeFile(p, f.Contents); err != nil {
			panic(fmt.Errorf("Failed to write %q (dir %q) due to %q", f.Basename, f.Dir, err))
		}
	}
}

func writeFile(pathToFile android.OutputPath, content string) error {
	// These files are made editable to allow users to modify and iterate on them
	// in the source tree.
	return android.WriteFileToOutputDir(pathToFile, []byte(content), 0644)
}
