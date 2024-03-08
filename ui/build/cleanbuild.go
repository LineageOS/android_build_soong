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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"android/soong/ui/metrics"
)

// Given a series of glob patterns, remove matching files and directories from the filesystem.
// For example, "malware*" would remove all files and directories in the current directory that begin with "malware".
func removeGlobs(ctx Context, globs ...string) {
	for _, glob := range globs {
		// Find files and directories that match this glob pattern.
		files, err := filepath.Glob(glob)
		if err != nil {
			// Only possible error is ErrBadPattern
			panic(fmt.Errorf("%q: %s", glob, err))
		}

		for _, file := range files {
			err = os.RemoveAll(file)
			if err != nil {
				ctx.Fatalf("Failed to remove file %q: %v", file, err)
			}
		}
	}
}

// Based on https://stackoverflow.com/questions/28969455/how-to-properly-instantiate-os-filemode
// Because Go doesn't provide a nice way to set bits on a filemode
const (
	FILEMODE_READ         = 04
	FILEMODE_WRITE        = 02
	FILEMODE_EXECUTE      = 01
	FILEMODE_USER_SHIFT   = 6
	FILEMODE_USER_READ    = FILEMODE_READ << FILEMODE_USER_SHIFT
	FILEMODE_USER_WRITE   = FILEMODE_WRITE << FILEMODE_USER_SHIFT
	FILEMODE_USER_EXECUTE = FILEMODE_EXECUTE << FILEMODE_USER_SHIFT
)

// Remove everything under the out directory. Don't remove the out directory
// itself in case it's a symlink.
func clean(ctx Context, config Config) {
	removeGlobs(ctx, filepath.Join(config.OutDir(), "*"))
	ctx.Println("Entire build directory removed.")
}

// Remove everything in the data directory.
func dataClean(ctx Context, config Config) {
	removeGlobs(ctx, filepath.Join(config.ProductOut(), "data", "*"))
	ctx.Println("Entire data directory removed.")
}

// installClean deletes all of the installed files -- the intent is to remove
// files that may no longer be installed, either because the user previously
// installed them, or they were previously installed by default but no longer
// are.
//
// This is faster than a full clean, since we're not deleting the
// intermediates.  Instead of recompiling, we can just copy the results.
func installClean(ctx Context, config Config) {
	dataClean(ctx, config)

	if hostCrossOutPath := config.hostCrossOut(); hostCrossOutPath != "" {
		hostCrossOut := func(path string) string {
			return filepath.Join(hostCrossOutPath, path)
		}
		removeGlobs(ctx,
			hostCrossOut("bin"),
			hostCrossOut("coverage"),
			hostCrossOut("lib*"),
			hostCrossOut("nativetest*"))
	}

	hostOutPath := config.HostOut()
	hostOut := func(path string) string {
		return filepath.Join(hostOutPath, path)
	}

	hostCommonOut := func(path string) string {
		return filepath.Join(config.hostOutRoot(), "common", path)
	}

	productOutPath := config.ProductOut()
	productOut := func(path string) string {
		return filepath.Join(productOutPath, path)
	}

	// Host bin, frameworks, and lib* are intentionally omitted, since
	// otherwise we'd have to rebuild any generated files created with
	// those tools.
	removeGlobs(ctx,
		hostOut("apex"),
		hostOut("obj/NOTICE_FILES"),
		hostOut("obj/PACKAGING"),
		hostOut("coverage"),
		hostOut("cts"),
		hostOut("nativetest*"),
		hostOut("sdk"),
		hostOut("sdk_addon"),
		hostOut("testcases"),
		hostOut("vts"),
		hostOut("vts10"),
		hostOut("vts-core"),
		hostCommonOut("obj/PACKAGING"),
		productOut("*.img"),
		productOut("*.zip"),
		productOut("*.zip.sha256sum"),
		productOut("android-info.txt"),
		productOut("misc_info.txt"),
		productOut("apex"),
		productOut("kernel"),
		productOut("kernel-*"),
		productOut("recovery_kernel"),
		productOut("data"),
		productOut("skin"),
		productOut("obj/NOTICE_FILES"),
		productOut("obj/PACKAGING"),
		productOut("ramdisk"),
		productOut("ramdisk_16k"),
		productOut("debug_ramdisk"),
		productOut("vendor_ramdisk"),
		productOut("vendor_debug_ramdisk"),
		productOut("vendor_kernel_ramdisk"),
		productOut("test_harness_ramdisk"),
		productOut("recovery"),
		productOut("root"),
		productOut("system"),
		productOut("system_dlkm"),
		productOut("system_other"),
		productOut("vendor"),
		productOut("vendor_dlkm"),
		productOut("product"),
		productOut("system_ext"),
		productOut("oem"),
		productOut("obj/FAKE"),
		productOut("breakpad"),
		productOut("cache"),
		productOut("coverage"),
		productOut("installer"),
		productOut("odm"),
		productOut("odm_dlkm"),
		productOut("sysloader"),
		productOut("testcases"),
		productOut("symbols"),
		productOut("install"))
}

// Since products and build variants (unfortunately) shared the same
// PRODUCT_OUT staging directory, things can get out of sync if different
// build configurations are built in the same tree. This function will
// notice when the configuration has changed and call installClean to
// remove the files necessary to keep things consistent.
func installCleanIfNecessary(ctx Context, config Config) {
	configFile := config.DevicePreviousProductConfig()
	prefix := "PREVIOUS_BUILD_CONFIG := "
	suffix := "\n"
	currentConfig := prefix + config.TargetProduct() + "-" + config.TargetBuildVariant() + suffix

	ensureDirectoriesExist(ctx, filepath.Dir(configFile))

	writeConfig := func() {
		err := ioutil.WriteFile(configFile, []byte(currentConfig), 0666) // a+rw
		if err != nil {
			ctx.Fatalln("Failed to write product config:", err)
		}
	}

	previousConfigBytes, err := ioutil.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Just write the new config file, no old config file to worry about.
			writeConfig()
			return
		} else {
			ctx.Fatalln("Failed to read previous product config:", err)
		}
	}

	previousConfig := string(previousConfigBytes)
	if previousConfig == currentConfig {
		// Same config as before - nothing to clean.
		return
	}

	if config.Environment().IsEnvTrue("DISABLE_AUTO_INSTALLCLEAN") {
		ctx.Println("DISABLE_AUTO_INSTALLCLEAN is set and true; skipping auto-clean. Your tree may be in an inconsistent state.")
		return
	}

	ctx.BeginTrace(metrics.PrimaryNinja, "installclean")
	defer ctx.EndTrace()

	previousProductAndVariant := strings.TrimPrefix(strings.TrimSuffix(previousConfig, suffix), prefix)
	currentProductAndVariant := strings.TrimPrefix(strings.TrimSuffix(currentConfig, suffix), prefix)

	ctx.Printf("Build configuration changed: %q -> %q, forcing installclean\n", previousProductAndVariant, currentProductAndVariant)

	installClean(ctx, config)

	writeConfig()
}

// cleanOldFiles takes an input file (with all paths relative to basePath), and removes files from
// the filesystem if they were removed from the input file since the last execution.
func cleanOldFiles(ctx Context, basePath, newFile string) {
	newFile = filepath.Join(basePath, newFile)
	oldFile := newFile + ".previous"

	if _, err := os.Stat(newFile); os.IsNotExist(err) {
		// If the file doesn't exist, assume no installed files exist either
		return
	} else if err != nil {
		ctx.Fatalf("Expected %q to be readable", newFile)
	}

	if _, err := os.Stat(oldFile); os.IsNotExist(err) {
		if err := os.Rename(newFile, oldFile); err != nil {
			ctx.Fatalf("Failed to rename file list (%q->%q): %v", newFile, oldFile, err)
		}
		return
	}

	var newData, oldData []byte
	if data, err := ioutil.ReadFile(newFile); err == nil {
		newData = data
	} else {
		ctx.Fatalf("Failed to read list of installable files (%q): %v", newFile, err)
	}
	if data, err := ioutil.ReadFile(oldFile); err == nil {
		oldData = data
	} else {
		ctx.Fatalf("Failed to read list of installable files (%q): %v", oldFile, err)
	}

	// Common case: nothing has changed
	if bytes.Equal(newData, oldData) {
		return
	}

	var newPaths, oldPaths []string
	newPaths = strings.Fields(string(newData))
	oldPaths = strings.Fields(string(oldData))

	// These should be mostly sorted by make already, but better make sure Go concurs
	sort.Strings(newPaths)
	sort.Strings(oldPaths)

	for len(oldPaths) > 0 {
		if len(newPaths) > 0 {
			if oldPaths[0] == newPaths[0] {
				// Same file; continue
				newPaths = newPaths[1:]
				oldPaths = oldPaths[1:]
				continue
			} else if oldPaths[0] > newPaths[0] {
				// New file; ignore
				newPaths = newPaths[1:]
				continue
			}
		}

		// File only exists in the old list; remove if it exists
		oldPath := filepath.Join(basePath, oldPaths[0])
		oldPaths = oldPaths[1:]

		if oldFile, err := os.Stat(oldPath); err == nil {
			if oldFile.IsDir() {
				if err := os.Remove(oldPath); err == nil {
					ctx.Println("Removed directory that is no longer installed: ", oldPath)
					cleanEmptyDirs(ctx, filepath.Dir(oldPath))
				} else {
					ctx.Println("Failed to remove directory that is no longer installed (%q): %v", oldPath, err)
					ctx.Println("It's recommended to run `m installclean`")
				}
			} else {
				// Removing a file, not a directory.
				if err := os.Remove(oldPath); err == nil {
					ctx.Println("Removed file that is no longer installed: ", oldPath)
					cleanEmptyDirs(ctx, filepath.Dir(oldPath))
				} else if !os.IsNotExist(err) {
					ctx.Fatalf("Failed to remove file that is no longer installed (%q): %v", oldPath, err)
				}
			}
		}
	}

	// Use the new list as the base for the next build
	os.Rename(newFile, oldFile)
}

// cleanEmptyDirs will delete a directory if it contains no files.
// If a deletion occurs, then it also recurses upwards to try and delete empty parent directories.
func cleanEmptyDirs(ctx Context, dir string) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		ctx.Println("Could not read directory while trying to clean empty dirs: ", dir)
		return
	}
	if len(files) > 0 {
		// Directory is not empty.
		return
	}

	if err := os.Remove(dir); err == nil {
		ctx.Println("Removed empty directory (may no longer be installed?): ", dir)
	} else {
		ctx.Fatalf("Failed to remove empty directory (which may no longer be installed?) %q: (%v)", dir, err)
	}

	// Try and delete empty parent directories too.
	cleanEmptyDirs(ctx, filepath.Dir(dir))
}
