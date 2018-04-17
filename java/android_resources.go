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

func init() {
	android.RegisterPreSingletonType("overlay", OverlaySingletonFactory)
}

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

func androidResourceGlob(ctx android.ModuleContext, dir android.Path) android.Paths {
	return ctx.GlobFiles(filepath.Join(dir.String(), "**/*"), androidResourceIgnoreFilenames)
}

type overlayGlobResult struct {
	dir   string
	paths android.DirectorySortedPaths

	// Set to true of the product has selected that values in this overlay should not be moved to
	// Runtime Resource Overlay (RRO) packages.
	excludeFromRRO bool
}

const overlayDataKey = "overlayDataKey"

type globbedResourceDir struct {
	dir   android.Path
	files android.Paths
}

func overlayResourceGlob(ctx android.ModuleContext, dir android.Path) (res []globbedResourceDir,
	rroDirs android.Paths) {

	overlayData := ctx.Config().Get(overlayDataKey).([]overlayGlobResult)

	// Runtime resource overlays (RRO) may be turned on by the product config for some modules
	rroEnabled := ctx.Config().EnforceRROForModule(ctx.ModuleName())

	for _, data := range overlayData {
		files := data.paths.PathsInDirectory(filepath.Join(data.dir, dir.String()))
		if len(files) > 0 {
			overlayModuleDir := android.PathForSource(ctx, data.dir, dir.String())
			// If enforce RRO is enabled for this module and this overlay is not in the
			// exclusion list, ignore the overlay.  The list of ignored overlays will be
			// passed to Make to be turned into an RRO package.
			if rroEnabled && !data.excludeFromRRO {
				rroDirs = append(rroDirs, overlayModuleDir)
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

func OverlaySingletonFactory() android.Singleton {
	return overlaySingleton{}
}

type overlaySingleton struct{}

func (overlaySingleton) GenerateBuildActions(ctx android.SingletonContext) {
	var overlayData []overlayGlobResult
	overlayDirs := ctx.Config().ResourceOverlays()
	for i := range overlayDirs {
		// Iterate backwards through the list of overlay directories so that the later, lower-priority
		// directories in the list show up earlier in the command line to aapt2.
		overlay := overlayDirs[len(overlayDirs)-1-i]
		var result overlayGlobResult
		result.dir = overlay

		// Mark overlays that will not have Runtime Resource Overlays enforced on them
		// based on the product config
		result.excludeFromRRO = ctx.Config().EnforceRROExcludedOverlay(overlay)

		files, err := ctx.GlobWithDeps(filepath.Join(overlay, "**/*"), androidResourceIgnoreFilenames)
		if err != nil {
			ctx.Errorf("failed to glob resource dir %q: %s", overlay, err.Error())
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

	ctx.Config().Once(overlayDataKey, func() interface{} {
		return overlayData
	})
}
