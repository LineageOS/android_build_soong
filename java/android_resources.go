// Copyright 2018 Google Inc. All rights reserved.
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
	"path/filepath"
	"strings"

	"android/soong/android"
)

var androidResourceIgnoreFilenames = []string{
	".svn",
	".git",
	".ds_store",
	"*.scc",
	".*",
	"CVS",
	"thumbs.db",
	"picasa.ini",
	"*~",
}

// androidResourceGlob returns the list of files in the given directory, using the standard
// exclusion patterns for Android resources.
func androidResourceGlob(ctx android.EarlyModuleContext, dir android.Path) android.Paths {
	return ctx.GlobFiles(filepath.Join(dir.String(), "**/*"), androidResourceIgnoreFilenames)
}

// androidResourceGlobList creates a rule to write the list of files in the given directory, using
// the standard exclusion patterns for Android resources, to the given output file.
func androidResourceGlobList(ctx android.ModuleContext, dir android.Path,
	fileListFile android.WritablePath) {

	android.GlobToListFileRule(ctx, filepath.Join(dir.String(), "**/*"),
		androidResourceIgnoreFilenames, fileListFile)
}

type overlayType int

const (
	device overlayType = iota + 1
	product
)

type rroDir struct {
	path        android.Path
	overlayType overlayType
}

type overlayGlobResult struct {
	dir         string
	paths       android.DirectorySortedPaths
	overlayType overlayType
}

var overlayDataKey = android.NewOnceKey("overlayDataKey")

type globbedResourceDir struct {
	dir   android.Path
	files android.Paths
}

func overlayResourceGlob(ctx android.ModuleContext, a *aapt, dir android.Path) (res []globbedResourceDir,
	rroDirs []rroDir) {

	overlayData := ctx.Config().Once(overlayDataKey, func() interface{} {
		var overlayData []overlayGlobResult

		appendOverlayData := func(overlayDirs []string, t overlayType) {
			for i := range overlayDirs {
				// Iterate backwards through the list of overlay directories so that the later, lower-priority
				// directories in the list show up earlier in the command line to aapt2.
				overlay := overlayDirs[len(overlayDirs)-1-i]
				var result overlayGlobResult
				result.dir = overlay
				result.overlayType = t

				files, err := ctx.GlobWithDeps(filepath.Join(overlay, "**/*"), androidResourceIgnoreFilenames)
				if err != nil {
					ctx.ModuleErrorf("failed to glob resource dir %q: %s", overlay, err.Error())
					continue
				}
				var paths android.Paths
				for _, f := range files {
					if !strings.HasSuffix(f, "/") {
						paths = append(paths, android.PathForSource(ctx, f))
					}
				}
				result.paths = android.PathsToDirectorySortedPaths(paths)
				overlayData = append(overlayData, result)
			}
		}

		appendOverlayData(ctx.Config().DeviceResourceOverlays(), device)
		appendOverlayData(ctx.Config().ProductResourceOverlays(), product)
		return overlayData
	}).([]overlayGlobResult)

	// Runtime resource overlays (RRO) may be turned on by the product config for some modules
	rroEnabled := a.IsRROEnforced(ctx)

	for _, data := range overlayData {
		files := data.paths.PathsInDirectory(filepath.Join(data.dir, dir.String()))
		if len(files) > 0 {
			overlayModuleDir := android.PathForSource(ctx, data.dir, dir.String())

			// If enforce RRO is enabled for this module and this overlay is not in the
			// exclusion list, ignore the overlay.  The list of ignored overlays will be
			// passed to Make to be turned into an RRO package.
			if rroEnabled && !ctx.Config().EnforceRROExcludedOverlay(overlayModuleDir.String()) {
				rroDirs = append(rroDirs, rroDir{overlayModuleDir, data.overlayType})
			} else {
				res = append(res, globbedResourceDir{
					dir:   overlayModuleDir,
					files: files,
				})
			}
		}
	}

	return res, rroDirs
}
