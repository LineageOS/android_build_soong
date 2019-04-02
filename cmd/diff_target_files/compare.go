// Copyright 2019 Google Inc. All rights reserved.
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
	"bytes"
	"fmt"
)

// compareTargetFiles takes two ZipArtifacts and compares the files they contain by examining
// the path, size, and CRC of each file.
func compareTargetFiles(priZip, refZip ZipArtifact, artifact string, whitelists []whitelist, filters []string) (zipDiff, error) {
	priZipFiles, err := priZip.Files()
	if err != nil {
		return zipDiff{}, fmt.Errorf("error fetching target file lists from primary zip %v", err)
	}

	refZipFiles, err := refZip.Files()
	if err != nil {
		return zipDiff{}, fmt.Errorf("error fetching target file lists from reference zip %v", err)
	}

	priZipFiles, err = filterTargetZipFiles(priZipFiles, artifact, filters)
	if err != nil {
		return zipDiff{}, err
	}

	refZipFiles, err = filterTargetZipFiles(refZipFiles, artifact, filters)
	if err != nil {
		return zipDiff{}, err
	}

	// Compare the file lists from both builds
	diff := diffTargetFilesLists(refZipFiles, priZipFiles)

	return applyWhitelists(diff, whitelists)
}

// zipDiff contains the list of files that differ between two zip files.
type zipDiff struct {
	modified         [][2]*ZipArtifactFile
	onlyInA, onlyInB []*ZipArtifactFile
}

// String pretty-prints the list of files that differ between two zip files.
func (d *zipDiff) String() string {
	buf := &bytes.Buffer{}

	must := func(n int, err error) {
		if err != nil {
			panic(err)
		}
	}

	var sizeChange int64

	if len(d.modified) > 0 {
		must(fmt.Fprintln(buf, "files modified:"))
		for _, f := range d.modified {
			must(fmt.Fprintf(buf, "   %v (%v bytes -> %v bytes)\n", f[0].Name, f[0].UncompressedSize64, f[1].UncompressedSize64))
			sizeChange += int64(f[1].UncompressedSize64) - int64(f[0].UncompressedSize64)
		}
	}

	if len(d.onlyInA) > 0 {
		must(fmt.Fprintln(buf, "files removed:"))
		for _, f := range d.onlyInA {
			must(fmt.Fprintf(buf, " - %v (%v bytes)\n", f.Name, f.UncompressedSize64))
			sizeChange -= int64(f.UncompressedSize64)
		}
	}

	if len(d.onlyInB) > 0 {
		must(fmt.Fprintln(buf, "files added:"))
		for _, f := range d.onlyInB {
			must(fmt.Fprintf(buf, " + %v (%v bytes)\n", f.Name, f.UncompressedSize64))
			sizeChange += int64(f.UncompressedSize64)
		}
	}

	if len(d.modified) > 0 || len(d.onlyInA) > 0 || len(d.onlyInB) > 0 {
		must(fmt.Fprintf(buf, "total size change: %v bytes\n", sizeChange))
	}

	return buf.String()
}

func diffTargetFilesLists(a, b []*ZipArtifactFile) zipDiff {
	i := 0
	j := 0

	diff := zipDiff{}

	for i < len(a) && j < len(b) {
		if a[i].Name == b[j].Name {
			if a[i].UncompressedSize64 != b[j].UncompressedSize64 || a[i].CRC32 != b[j].CRC32 {
				diff.modified = append(diff.modified, [2]*ZipArtifactFile{a[i], b[j]})
			}
			i++
			j++
		} else if a[i].Name < b[j].Name {
			// a[i] is not present in b
			diff.onlyInA = append(diff.onlyInA, a[i])
			i++
		} else {
			// b[j] is not present in a
			diff.onlyInB = append(diff.onlyInB, b[j])
			j++
		}
	}
	for i < len(a) {
		diff.onlyInA = append(diff.onlyInA, a[i])
		i++
	}
	for j < len(b) {
		diff.onlyInB = append(diff.onlyInB, b[j])
		j++
	}

	return diff
}
