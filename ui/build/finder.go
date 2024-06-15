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
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"android/soong/finder"
	"android/soong/finder/fs"
	"android/soong/ui/logger"

	"android/soong/ui/metrics"
)

// This file provides an interface to the Finder type for soong_ui. Finder is
// used to recursively traverse the source tree to gather paths of files, such
// as Android.bp or Android.mk, and store the lists/database of paths in files
// under `$OUT_DIR/.module_paths`. This directory can also be dist'd.

// NewSourceFinder returns a new Finder configured to search for source files.
// Callers of NewSourceFinder should call <f.Shutdown()> when done
func NewSourceFinder(ctx Context, config Config) (f *finder.Finder) {
	ctx.BeginTrace(metrics.RunSetupTool, "find modules")
	defer ctx.EndTrace()

	// Set up the working directory for the Finder.
	dir, err := os.Getwd()
	if err != nil {
		ctx.Fatalf("No working directory for module-finder: %v", err.Error())
	}
	filesystem := fs.OsFs

	// .out-dir and .find-ignore are markers for Finder to ignore siblings and
	// subdirectories of the directory Finder finds them in, hence stopping the
	// search recursively down those branches. It's possible that these files
	// are in the root directory, and if they are, then the subsequent error
	// messages are very confusing, so check for that here.
	pruneFiles := []string{".out-dir", ".find-ignore"}
	for _, name := range pruneFiles {
		prunePath := filepath.Join(dir, name)
		_, statErr := filesystem.Lstat(prunePath)
		if statErr == nil {
			ctx.Fatalf("%v must not exist", prunePath)
		}
	}

	// Set up configuration parameters for the Finder cache.
	cacheParams := finder.CacheParams{
		WorkingDirectory: dir,
		RootDirs:         androidBpSearchDirs(config),
		FollowSymlinks:   config.environ.IsEnvTrue("ALLOW_BP_UNDER_SYMLINKS"),
		ExcludeDirs:      []string{".git", ".repo"},
		PruneFiles:       pruneFiles,
		IncludeFiles: []string{
			// Kati build definitions.
			"Android.mk",
			// Product configuration files.
			"AndroidProducts.mk",
			// General Soong build definitions, using the Blueprint syntax.
			"Android.bp",
			// Kati clean definitions.
			"CleanSpec.mk",
			// Ownership definition.
			"OWNERS",
			// Test configuration for modules in directories that contain this
			// file.
			"TEST_MAPPING",
			// METADATA file of packages
			"METADATA",
		},
		// .mk files for product/board configuration.
		IncludeSuffixes: []string{".mk"},
	}
	dumpDir := config.FileListDir()
	f, err = finder.New(cacheParams, filesystem, logger.New(ioutil.Discard),
		filepath.Join(dumpDir, "files.db"))
	if err != nil {
		ctx.Fatalf("Could not create module-finder: %v", err)
	}
	return f
}

func androidBpSearchDirs(config Config) []string {
	dirs := []string{"."} // always search from root of source tree.
	if config.searchApiDir {
		// Search in out/api_surfaces
		dirs = append(dirs, config.ApiSurfacesOutDir())
	}
	return dirs
}

func findProductAndBoardConfigFiles(entries finder.DirEntries) (dirNames []string, fileNames []string) {
	matches := []string{}
	for _, foundName := range entries.FileNames {
		if foundName != "Android.mk" &&
			foundName != "AndroidProducts.mk" &&
			foundName != "CleanSpec.mk" &&
			strings.HasSuffix(foundName, ".mk") {
			matches = append(matches, foundName)
		}
	}
	return entries.DirNames, matches
}

// FindSources searches for source files known to <f> and writes them to the filesystem for
// use later.
func FindSources(ctx Context, config Config, f *finder.Finder) {
	// note that dumpDir in FindSources may be different than dumpDir in NewSourceFinder
	// if a caller such as multiproduct_kati wants to share one Finder among several builds
	dumpDir := config.FileListDir()
	os.MkdirAll(dumpDir, 0777)

	// Stop searching a subdirectory recursively after finding an Android.mk.
	androidMks := f.FindFirstNamedAt(".", "Android.mk")
	blockAndroidMks(ctx, androidMks)
	err := dumpListToFile(ctx, config, androidMks, filepath.Join(dumpDir, "Android.mk.list"))
	if err != nil {
		ctx.Fatalf("Could not export module list: %v", err)
	}

	// Gate collecting/reporting mk metrics on builds that specifically request
	// it, as identifying the total number of mk files adds 4-5ms onto null
	// builds.
	if config.reportMkMetrics {
		androidMksTotal := f.FindNamedAt(".", "Android.mk")

		ctx.Metrics.SetToplevelMakefiles(len(androidMks))
		ctx.Metrics.SetTotalMakefiles(len(androidMksTotal))
		ctx.Metrics.DumpMkMetrics(config.MkMetrics())
	}

	// Stop searching a subdirectory recursively after finding a CleanSpec.mk.
	cleanSpecs := f.FindFirstNamedAt(".", "CleanSpec.mk")
	err = dumpListToFile(ctx, config, cleanSpecs, filepath.Join(dumpDir, "CleanSpec.mk.list"))
	if err != nil {
		ctx.Fatalf("Could not export module list: %v", err)
	}

	// Only consider AndroidProducts.mk in device/, vendor/ and product/, recursively in these directories.
	androidProductsMks := f.FindNamedAt("device", "AndroidProducts.mk")
	androidProductsMks = append(androidProductsMks, f.FindNamedAt("vendor", "AndroidProducts.mk")...)
	androidProductsMks = append(androidProductsMks, f.FindNamedAt("product", "AndroidProducts.mk")...)
	err = dumpListToFile(ctx, config, androidProductsMks, filepath.Join(dumpDir, "AndroidProducts.mk.list"))
	if err != nil {
		ctx.Fatalf("Could not export product list: %v", err)
	}

	// Recursively look for all OWNERS files.
	owners := f.FindNamedAt(".", "OWNERS")
	err = dumpListToFile(ctx, config, owners, filepath.Join(dumpDir, "OWNERS.list"))
	if err != nil {
		ctx.Fatalf("Could not find OWNERS: %v", err)
	}

	// Recursively look for all METADATA files.
	metadataFiles := f.FindNamedAt(".", "METADATA")
	err = dumpListToFile(ctx, config, metadataFiles, filepath.Join(dumpDir, "METADATA.list"))
	if err != nil {
		ctx.Fatalf("Could not find METADATA: %v", err)
	}

	// Recursively look for all TEST_MAPPING files.
	testMappings := f.FindNamedAt(".", "TEST_MAPPING")
	err = dumpListToFile(ctx, config, testMappings, filepath.Join(dumpDir, "TEST_MAPPING.list"))
	if err != nil {
		ctx.Fatalf("Could not find TEST_MAPPING: %v", err)
	}

	// Recursively look for all Android.bp files
	androidBps := f.FindNamedAt(".", "Android.bp")
	if len(androidBps) == 0 {
		ctx.Fatalf("No Android.bp found")
	}
	err = dumpListToFile(ctx, config, androidBps, filepath.Join(dumpDir, "Android.bp.list"))
	if err != nil {
		ctx.Fatalf("Could not find modules: %v", err)
	}

	// Recursively look for all product/board config files.
	configurationFiles := f.FindMatching(".", findProductAndBoardConfigFiles)
	err = dumpListToFile(ctx, config, configurationFiles, filepath.Join(dumpDir, "configuration.list"))
	if err != nil {
		ctx.Fatalf("Could not export product/board configuration list: %v", err)
	}

	if config.Dist() {
		f.WaitForDbDump()
		// Dist the files.db plain text database.
		distFile(ctx, config, f.DbPath, "module_paths")
	}
}

// Write the .list files to disk.
func dumpListToFile(ctx Context, config Config, list []string, filePath string) (err error) {
	desiredText := strings.Join(list, "\n")
	desiredBytes := []byte(desiredText)
	actualBytes, readErr := ioutil.ReadFile(filePath)
	if readErr != nil || !bytes.Equal(desiredBytes, actualBytes) {
		err = ioutil.WriteFile(filePath, desiredBytes, 0777)
		if err != nil {
			return err
		}
	}

	distFile(ctx, config, filePath, "module_paths")

	return nil
}
