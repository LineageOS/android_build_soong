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

	"github.com/google/blueprint/bootstrap"

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

func isStringInSlice(str string, slice []string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func ResourceDirsToJarArgs(ctx android.ModuleContext,
	resourceDirs, excludeDirs []string) (args []string, deps android.Paths) {
	var excludes []string

	for _, exclude := range excludeDirs {
		excludes = append(excludes,
			filepath.Join(android.PathForModuleSrc(ctx, exclude).String(), "**/*"))
	}

	excludes = append(excludes, resourceExcludes...)

	for _, resourceDir := range resourceDirs {
		if isStringInSlice(resourceDir, excludeDirs) {
			continue
		}
		resourceDir := android.PathForModuleSrc(ctx, resourceDir)
		dirs := ctx.Glob(resourceDir.String(), nil)
		for _, dir := range dirs {
			fileListFile := android.ResPathWithName(ctx, dir, "resources.list")
			depFile := fileListFile.String() + ".d"

			pattern := filepath.Join(dir.String(), "**/*")
			bootstrap.GlobFile(ctx, pattern, excludes, fileListFile.String(), depFile)
			args = append(args,
				"-C", dir.String(),
				"-l", fileListFile.String())
			deps = append(deps, fileListFile)
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

// Convert java_resources properties to arguments to soong_zip -jar, keeping files that should
// normally not used as resources like *.java
func SourceFilesToJarArgs(ctx android.ModuleContext,
	res, exclude []string) (args []string, deps android.Paths) {

	return resourceFilesToJarArgs(ctx, res, exclude)
}

func resourceFilesToJarArgs(ctx android.ModuleContext,
	res, exclude []string) (args []string, deps android.Paths) {

	files := ctx.ExpandSources(res, exclude)

	for _, f := range files {
		rel := f.Rel()
		path := f.String()
		if !strings.HasSuffix(path, rel) {
			panic(fmt.Errorf("path %q does not end with %q", path, rel))
		}
		path = filepath.Clean(strings.TrimSuffix(path, rel))
		args = append(args, "-C", filepath.Clean(path), "-f", f.String())
	}

	return args, files
}
