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

package jar

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"android/soong/third_party/zip"
)

const (
	MetaDir         = "META-INF/"
	ManifestFile    = MetaDir + "MANIFEST.MF"
	ModuleInfoClass = "module-info.class"
)

var DefaultTime = time.Date(2008, 1, 1, 0, 0, 0, 0, time.UTC)

var MetaDirExtra = [2]byte{0xca, 0xfe}

// EntryNamesLess tells whether <filepathA> should precede <filepathB> in
// the order of files with a .jar
func EntryNamesLess(filepathA string, filepathB string) (less bool) {
	diff := index(filepathA) - index(filepathB)
	if diff == 0 {
		return filepathA < filepathB
	}
	return diff < 0
}

// Treats trailing * as a prefix match
func patternMatch(pattern, name string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(name, strings.TrimSuffix(pattern, "*"))
	} else {
		return name == pattern
	}
}

var jarOrder = []string{
	MetaDir,
	ManifestFile,
	MetaDir + "*",
	"*",
}

func index(name string) int {
	for i, pattern := range jarOrder {
		if patternMatch(pattern, name) {
			return i
		}
	}
	panic(fmt.Errorf("file %q did not match any pattern", name))
}

func MetaDirFileHeader() *zip.FileHeader {
	dirHeader := &zip.FileHeader{
		Name:  MetaDir,
		Extra: []byte{MetaDirExtra[1], MetaDirExtra[0], 0, 0},
	}
	dirHeader.SetMode(0700 | os.ModeDir)
	dirHeader.SetModTime(DefaultTime)

	return dirHeader
}

// Create a manifest zip header and contents using the provided contents if any.
func ManifestFileContents(contents []byte) (*zip.FileHeader, []byte, error) {
	b, err := manifestContents(contents)
	if err != nil {
		return nil, nil, err
	}

	fh := &zip.FileHeader{
		Name:               ManifestFile,
		Method:             zip.Store,
		UncompressedSize64: uint64(len(b)),
	}
	fh.SetMode(0700)
	fh.SetModTime(DefaultTime)

	return fh, b, nil
}

// Create manifest contents, using the provided contents if any.
func manifestContents(contents []byte) ([]byte, error) {
	manifestMarker := []byte("Manifest-Version:")
	header := append(manifestMarker, []byte(" 1.0\nCreated-By: soong_zip\n")...)

	var finalBytes []byte
	if !bytes.Contains(contents, manifestMarker) {
		finalBytes = append(append(header, contents...), byte('\n'))
	} else {
		finalBytes = contents
	}

	return finalBytes, nil
}
