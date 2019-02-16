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

package main

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"android/soong/jar"
	"android/soong/third_party/zip"
)

type testZipEntry struct {
	name string
	mode os.FileMode
	data []byte
}

var (
	A     = testZipEntry{"A", 0755, []byte("foo")}
	a     = testZipEntry{"a", 0755, []byte("foo")}
	a2    = testZipEntry{"a", 0755, []byte("FOO2")}
	a3    = testZipEntry{"a", 0755, []byte("Foo3")}
	bDir  = testZipEntry{"b/", os.ModeDir | 0755, nil}
	bbDir = testZipEntry{"b/b/", os.ModeDir | 0755, nil}
	bbb   = testZipEntry{"b/b/b", 0755, nil}
	ba    = testZipEntry{"b/a", 0755, []byte("foob")}
	bc    = testZipEntry{"b/c", 0755, []byte("bar")}
	bd    = testZipEntry{"b/d", 0700, []byte("baz")}
	be    = testZipEntry{"b/e", 0700, []byte("")}

	metainfDir     = testZipEntry{jar.MetaDir, os.ModeDir | 0755, nil}
	manifestFile   = testZipEntry{jar.ManifestFile, 0755, []byte("manifest")}
	manifestFile2  = testZipEntry{jar.ManifestFile, 0755, []byte("manifest2")}
	moduleInfoFile = testZipEntry{jar.ModuleInfoClass, 0755, []byte("module-info")}
)

func TestMergeZips(t *testing.T) {
	testCases := []struct {
		name             string
		in               [][]testZipEntry
		stripFiles       []string
		stripDirs        []string
		jar              bool
		sort             bool
		ignoreDuplicates bool
		stripDirEntries  bool
		zipsToNotStrip   map[string]bool

		out []testZipEntry
		err string
	}{
		{
			name: "duplicates error",
			in: [][]testZipEntry{
				{a},
				{a2},
				{a3},
			},
			out: []testZipEntry{a},
			err: "duplicate",
		},
		{
			name: "duplicates take first",
			in: [][]testZipEntry{
				{a},
				{a2},
				{a3},
			},
			out: []testZipEntry{a},

			ignoreDuplicates: true,
		},
		{
			name: "duplicates identical",
			in: [][]testZipEntry{
				{a},
				{a},
			},
			out: []testZipEntry{a},
		},
		{
			name: "sort",
			in: [][]testZipEntry{
				{be, bc, bDir, bbDir, bbb, A, metainfDir, manifestFile},
			},
			out: []testZipEntry{A, metainfDir, manifestFile, bDir, bbDir, bbb, bc, be},

			sort: true,
		},
		{
			name: "jar sort",
			in: [][]testZipEntry{
				{be, bc, bDir, A, metainfDir, manifestFile},
			},
			out: []testZipEntry{metainfDir, manifestFile, A, bDir, bc, be},

			jar: true,
		},
		{
			name: "jar merge",
			in: [][]testZipEntry{
				{metainfDir, manifestFile, bDir, be},
				{metainfDir, manifestFile2, bDir, bc},
				{metainfDir, manifestFile2, A},
			},
			out: []testZipEntry{metainfDir, manifestFile, A, bDir, bc, be},

			jar: true,
		},
		{
			name: "merge",
			in: [][]testZipEntry{
				{bDir, be},
				{bDir, bc},
				{A},
			},
			out: []testZipEntry{bDir, be, bc, A},
		},
		{
			name: "strip dir entries",
			in: [][]testZipEntry{
				{a, bDir, bbDir, bbb, bc, bd, be},
			},
			out: []testZipEntry{a, bbb, bc, bd, be},

			stripDirEntries: true,
		},
		{
			name: "strip files",
			in: [][]testZipEntry{
				{a, bDir, bbDir, bbb, bc, bd, be},
			},
			out: []testZipEntry{a, bDir, bbDir, bbb, bc},

			stripFiles: []string{"b/d", "b/e"},
		},
		{
			// merge_zips used to treat -stripFile a as stripping any file named a, it now only strips a in the
			// root of the zip.
			name: "strip file name",
			in: [][]testZipEntry{
				{a, bDir, ba},
			},
			out: []testZipEntry{bDir, ba},

			stripFiles: []string{"a"},
		},
		{
			name: "strip files glob",
			in: [][]testZipEntry{
				{a, bDir, ba},
			},
			out: []testZipEntry{bDir},

			stripFiles: []string{"**/a"},
		},
		{
			name: "strip dirs",
			in: [][]testZipEntry{
				{a, bDir, bbDir, bbb, bc, bd, be},
			},
			out: []testZipEntry{a},

			stripDirs: []string{"b"},
		},
		{
			name: "strip dirs glob",
			in: [][]testZipEntry{
				{a, bDir, bbDir, bbb, bc, bd, be},
			},
			out: []testZipEntry{a, bDir, bc, bd, be},

			stripDirs: []string{"b/*"},
		},
		{
			name: "zips to not strip",
			in: [][]testZipEntry{
				{a, bDir, bc},
				{bDir, bd},
				{bDir, be},
			},
			out: []testZipEntry{a, bDir, bd},

			stripDirs: []string{"b"},
			zipsToNotStrip: map[string]bool{
				"in1": true,
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			var readers []namedZipReader
			for i, in := range test.in {
				r := testZipEntriesToZipReader(in)
				readers = append(readers, namedZipReader{
					path:   "in" + strconv.Itoa(i),
					reader: r,
				})
			}

			want := testZipEntriesToBuf(test.out)

			out := &bytes.Buffer{}
			writer := zip.NewWriter(out)

			err := mergeZips(readers, writer, "", "",
				test.sort, test.jar, false, test.stripDirEntries, test.ignoreDuplicates,
				test.stripFiles, test.stripDirs, test.zipsToNotStrip)

			closeErr := writer.Close()
			if closeErr != nil {
				t.Fatal(err)
			}

			if test.err != "" {
				if err == nil {
					t.Fatal("missing err, expected: ", test.err)
				} else if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.err)) {
					t.Fatal("incorrect err, want:", test.err, "got:", err)
				}
				return
			}

			if !bytes.Equal(want, out.Bytes()) {
				t.Error("incorrect zip output")
				t.Errorf("want:\n%s", dumpZip(want))
				t.Errorf("got:\n%s", dumpZip(out.Bytes()))
			}
		})
	}
}

func testZipEntriesToBuf(entries []testZipEntry) []byte {
	b := &bytes.Buffer{}
	zw := zip.NewWriter(b)

	for _, e := range entries {
		fh := zip.FileHeader{
			Name: e.name,
		}
		fh.SetMode(e.mode)

		w, err := zw.CreateHeader(&fh)
		if err != nil {
			panic(err)
		}

		_, err = w.Write(e.data)
		if err != nil {
			panic(err)
		}
	}

	err := zw.Close()
	if err != nil {
		panic(err)
	}

	return b.Bytes()
}

func testZipEntriesToZipReader(entries []testZipEntry) *zip.Reader {
	b := testZipEntriesToBuf(entries)
	r := bytes.NewReader(b)

	zr, err := zip.NewReader(r, int64(len(b)))
	if err != nil {
		panic(err)
	}

	return zr
}

func dumpZip(buf []byte) string {
	r := bytes.NewReader(buf)
	zr, err := zip.NewReader(r, int64(len(buf)))
	if err != nil {
		panic(err)
	}

	var ret string

	for _, f := range zr.File {
		ret += fmt.Sprintf("%v: %v %v %08x\n", f.Name, f.Mode(), f.UncompressedSize64, f.CRC32)
	}

	return ret
}
