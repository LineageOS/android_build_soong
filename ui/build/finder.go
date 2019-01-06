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
	"android/soong/finder"
	"android/soong/finder/fs"
	"android/soong/ui/logger"
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"android/soong/ui/metrics"
)

// This file provides an interface to the Finder for use in Soong UI
// This file stores configuration information about which files to find

// NewSourceFinder returns a new Finder configured to search for source files.
// Callers of NewSourceFinder should call <f.Shutdown()> when done
func NewSourceFinder(ctx Context, config Config) (f *finder.Finder) {
	ctx.BeginTrace(metrics.RunSetupTool, "find modules")
	defer ctx.EndTrace()

	dir, err := os.Getwd()
	if err != nil {
		ctx.Fatalf("No working directory for module-finder: %v", err.Error())
	}
	filesystem := fs.OsFs

	// if the root dir is ignored, then the subsequent error messages are very confusing,
	// so check for that upfront
	pruneFiles := []string{".out-dir", ".find-ignore"}
	for _, name := range pruneFiles {
		prunePath := filepath.Join(dir, name)
		_, statErr := filesystem.Lstat(prunePath)
		if statErr == nil {
			ctx.Fatalf("%v must not exist", prunePath)
		}
	}

	cacheParams := finder.CacheParams{
		WorkingDirectory: dir,
		RootDirs:         []string{"."},
		ExcludeDirs:      []string{".git", ".repo"},
		PruneFiles:       pruneFiles,
		IncludeFiles: []string{
			"Android.mk",
			"AndroidProducts.mk",
			"Android.bp",
			"Blueprints",
			"CleanSpec.mk",
			"OWNERS",
			"TEST_MAPPING",
		},
	}
	dumpDir := config.FileListDir()
	f, err = finder.New(cacheParams, filesystem, logger.New(ioutil.Discard),
		filepath.Join(dumpDir, "files.db"))
	if err != nil {
		ctx.Fatalf("Could not create module-finder: %v", err)
	}
	return f
}

// FindSources searches for source files known to <f> and writes them to the filesystem for
// use later.
func FindSources(ctx Context, config Config, f *finder.Finder) {
	// note that dumpDir in FindSources may be different than dumpDir in NewSourceFinder
	// if a caller such as multiproduct_kati wants to share one Finder among several builds
	dumpDir := config.FileListDir()
	os.MkdirAll(dumpDir, 0777)

	androidMks := f.FindFirstNamedAt(".", "Android.mk")
	err := dumpListToFile(androidMks, filepath.Join(dumpDir, "Android.mk.list"))
	if err != nil {
		ctx.Fatalf("Could not export module list: %v", err)
	}

	androidProductsMks := f.FindNamedAt("device", "AndroidProducts.mk")
	androidProductsMks = append(androidProductsMks, f.FindNamedAt("vendor", "AndroidProducts.mk")...)
	androidProductsMks = append(androidProductsMks, f.FindNamedAt("product", "AndroidProducts.mk")...)
	err = dumpListToFile(androidProductsMks, filepath.Join(dumpDir, "AndroidProducts.mk.list"))
	if err != nil {
		ctx.Fatalf("Could not export product list: %v", err)
	}

	cleanSpecs := f.FindFirstNamedAt(".", "CleanSpec.mk")
	err = dumpListToFile(cleanSpecs, filepath.Join(dumpDir, "CleanSpec.mk.list"))
	if err != nil {
		ctx.Fatalf("Could not export module list: %v", err)
	}

	owners := f.FindNamedAt(".", "OWNERS")
	err = dumpListToFile(owners, filepath.Join(dumpDir, "OWNERS.list"))
	if err != nil {
		ctx.Fatalf("Could not find OWNERS: %v", err)
	}

	testMappings := f.FindNamedAt(".", "TEST_MAPPING")
	err = dumpListToFile(testMappings, filepath.Join(dumpDir, "TEST_MAPPING.list"))
	if err != nil {
		ctx.Fatalf("Could not find TEST_MAPPING: %v", err)
	}

	androidBps := f.FindNamedAt(".", "Android.bp")
	androidBps = append(androidBps, f.FindNamedAt("build/blueprint", "Blueprints")...)
	if len(androidBps) == 0 {
		ctx.Fatalf("No Android.bp found")
	}
	err = dumpListToFile(androidBps, filepath.Join(dumpDir, "Android.bp.list"))
	if err != nil {
		ctx.Fatalf("Could not find modules: %v", err)
	}
}

func dumpListToFile(list []string, filePath string) (err error) {
	desiredText := strings.Join(list, "\n")
	desiredBytes := []byte(desiredText)
	actualBytes, readErr := ioutil.ReadFile(filePath)
	if readErr != nil || !bytes.Equal(desiredBytes, actualBytes) {
		err = ioutil.WriteFile(filePath, desiredBytes, 0777)
	}
	return err
}
