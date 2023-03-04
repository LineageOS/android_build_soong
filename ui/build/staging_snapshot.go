// Copyright 2023 Google Inc. All rights reserved.
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
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"android/soong/shared"
	"android/soong/ui/metrics"
)

// Metadata about a staged file
type fileEntry struct {
	Name string      `json:"name"`
	Mode fs.FileMode `json:"mode"`
	Size int64       `json:"size"`
	Sha1 string      `json:"sha1"`
}

func fileEntryEqual(a fileEntry, b fileEntry) bool {
	return a.Name == b.Name && a.Mode == b.Mode && a.Size == b.Size && a.Sha1 == b.Sha1
}

func sha1_hash(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// Subdirs of PRODUCT_OUT to scan
var stagingSubdirs = []string{
	"apex",
	"cache",
	"coverage",
	"data",
	"debug_ramdisk",
	"fake_packages",
	"installer",
	"oem",
	"product",
	"ramdisk",
	"recovery",
	"root",
	"sysloader",
	"system",
	"system_dlkm",
	"system_ext",
	"system_other",
	"testcases",
	"test_harness_ramdisk",
	"vendor",
	"vendor_debug_ramdisk",
	"vendor_kernel_ramdisk",
	"vendor_ramdisk",
}

// Return an array of stagedFileEntrys, one for each file in the staging directories inside
// productOut
func takeStagingSnapshot(ctx Context, productOut string, subdirs []string) ([]fileEntry, error) {
	var outer_err error
	if !strings.HasSuffix(productOut, "/") {
		productOut += "/"
	}
	result := []fileEntry{}
	for _, subdir := range subdirs {
		filepath.WalkDir(productOut+subdir,
			func(filename string, dirent fs.DirEntry, err error) error {
				// Ignore errors. The most common one is that one of the subdirectories
				// hasn't been built, in which case we just report it as empty.
				if err != nil {
					ctx.Verbosef("scanModifiedStagingOutputs error: %s", err)
					return nil
				}
				if dirent.Type().IsRegular() {
					fileInfo, _ := dirent.Info()
					relative := strings.TrimPrefix(filename, productOut)
					sha, err := sha1_hash(filename)
					if err != nil {
						outer_err = err
					}
					result = append(result, fileEntry{
						Name: relative,
						Mode: fileInfo.Mode(),
						Size: fileInfo.Size(),
						Sha1: sha,
					})
				}
				return nil
			})
	}

	sort.Slice(result, func(l, r int) bool { return result[l].Name < result[r].Name })

	return result, outer_err
}

// Read json into an array of fileEntry. On error return empty array.
func readJson(filename string) ([]fileEntry, error) {
	buf, err := os.ReadFile(filename)
	if err != nil {
		// Not an error, just missing, which is empty.
		return []fileEntry{}, nil
	}

	var result []fileEntry
	err = json.Unmarshal(buf, &result)
	if err != nil {
		// Bad formatting. This is an error
		return []fileEntry{}, err
	}

	return result, nil
}

// Write obj to filename.
func writeJson(filename string, obj interface{}) error {
	buf, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, buf, 0660)
}

type snapshotDiff struct {
	Added   []string `json:"added"`
	Changed []string `json:"changed"`
	Removed []string `json:"removed"`
}

// Diff the two snapshots, returning a snapshotDiff.
func diffSnapshots(previous []fileEntry, current []fileEntry) snapshotDiff {
	result := snapshotDiff{
		Added:   []string{},
		Changed: []string{},
		Removed: []string{},
	}

	found := make(map[string]bool)

	prev := make(map[string]fileEntry)
	for _, pre := range previous {
		prev[pre.Name] = pre
	}

	for _, cur := range current {
		pre, ok := prev[cur.Name]
		found[cur.Name] = true
		// Added
		if !ok {
			result.Added = append(result.Added, cur.Name)
			continue
		}
		// Changed
		if !fileEntryEqual(pre, cur) {
			result.Changed = append(result.Changed, cur.Name)
		}
	}

	// Removed
	for _, pre := range previous {
		if !found[pre.Name] {
			result.Removed = append(result.Removed, pre.Name)
		}
	}

	// Sort the results
	sort.Strings(result.Added)
	sort.Strings(result.Changed)
	sort.Strings(result.Removed)

	return result
}

// Write a json files to dist:
//   - A list of which files have changed in this build.
//
// And record in out/soong:
//   - A list of all files in the staging directories, including their hashes.
func runStagingSnapshot(ctx Context, config Config) {
	ctx.BeginTrace(metrics.RunSoong, "runStagingSnapshot")
	defer ctx.EndTrace()

	snapshotFilename := shared.JoinPath(config.SoongOutDir(), "staged_files.json")

	// Read the existing snapshot file. If it doesn't exist, this is a full
	// build, so all files will be treated as new.
	previous, err := readJson(snapshotFilename)
	if err != nil {
		ctx.Fatal(err)
		return
	}

	// Take a snapshot of the current out directory
	current, err := takeStagingSnapshot(ctx, config.ProductOut(), stagingSubdirs)
	if err != nil {
		ctx.Fatal(err)
		return
	}

	// Diff the snapshots
	diff := diffSnapshots(previous, current)

	// Write the diff (use RealDistDir, not one that might have been faked for bazel)
	err = writeJson(shared.JoinPath(config.RealDistDir(), "modified_files.json"), diff)
	if err != nil {
		ctx.Fatal(err)
		return
	}

	// Update the snapshot
	err = writeJson(snapshotFilename, current)
	if err != nil {
		ctx.Fatal(err)
		return
	}
}
