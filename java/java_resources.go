// Copyright 2015 Google Inc. All rights reserved.
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

package java

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/blueprint/pathtools"

	"android/soong/android"
)

var resourceExcludes = []string{
	"**/*.java",
	"**/package.html",
	"**/overview.html",
	"**/.*.swp",
	"**/.DS_Store",
	"**/*~",
}

func ResourceDirsToJarArgs(ctx android.ModuleContext,
	resourceDirs, excludeResourceDirs, excludeResourceFiles []string) (args []string, deps android.Paths) {
	var excludeDirs []string
	var excludeFiles []string

	for _, exclude := range excludeResourceDirs {
		dirs := ctx.Glob(android.PathForSource(ctx, ctx.ModuleDir()).Join(ctx, exclude).String(), nil)
		for _, dir := range dirs {
			excludeDirs = append(excludeDirs, dir.String())
			excludeFiles = append(excludeFiles, dir.(android.SourcePath).Join(ctx, "**/*").String())
		}
	}

	excludeFiles = append(excludeFiles, android.PathsForModuleSrc(ctx, excludeResourceFiles).Strings()...)

	excludeFiles = append(excludeFiles, resourceExcludes...)

	for _, resourceDir := range resourceDirs {
		// resourceDir may be a glob, resolve it first
		dirs := ctx.Glob(android.PathForSource(ctx, ctx.ModuleDir()).Join(ctx, resourceDir).String(), excludeDirs)
		for _, dir := range dirs {
			files := ctx.GlobFiles(filepath.Join(dir.String(), "**/*"), excludeFiles)

			deps = append(deps, files...)

			if len(files) > 0 {
				args = append(args, "-C", dir.String())

				for _, f := range files {
					path := f.String()
					if !strings.HasPrefix(path, dir.String()) {
						panic(fmt.Errorf("path %q does not start with %q", path, dir))
					}
					args = append(args, "-f", pathtools.MatchEscape(path))
				}
			}
		}
	}

	return args, deps
}

// Convert java_resources properties to arguments to soong_zip -jar, ignoring common patterns
// that should not be treated as resources (including *.java).
func ResourceFilesToJarArgs(ctx android.ModuleContext,
	res, exclude []string) (args []string, deps android.Paths) {

	exclude = append([]string(nil), exclude...)
	exclude = append(exclude, resourceExcludes...)
	return resourceFilesToJarArgs(ctx, res, exclude)
}

func resourceFilesToJarArgs(ctx android.ModuleContext,
	res, exclude []string) (args []string, deps android.Paths) {

	files := android.PathsForModuleSrcExcludes(ctx, res, exclude)

	args = resourcePathsToJarArgs(files)

	return args, files
}

func resourcePathsToJarArgs(files android.Paths) []string {
	var args []string

	lastDir := ""
	for i, f := range files {
		rel := f.Rel()
		path := f.String()
		if !strings.HasSuffix(path, rel) {
			panic(fmt.Errorf("path %q does not end with %q", path, rel))
		}
		dir := filepath.Clean(strings.TrimSuffix(path, rel))
		if i == 0 || dir != lastDir {
			args = append(args, "-C", dir)
		}
		args = append(args, "-f", pathtools.MatchEscape(path))
		lastDir = dir
	}

	return args
}
