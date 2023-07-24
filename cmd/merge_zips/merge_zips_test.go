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
	"hash/crc32"
	"os"
	"strconv"
	"strings"
	"testing"

	"android/soong/jar"
	"android/soong/third_party/zip"
)

type testZipEntry struct {
	name   string
	mode   os.FileMode
	data   []byte
	method uint16
}

var (
	A     = testZipEntry{"A", 0755, []byte("foo"), zip.Deflate}
	a     = testZipEntry{"a", 0755, []byte("foo"), zip.Deflate}
	a2    = testZipEntry{"a", 0755, []byte("FOO2"), zip.Deflate}
	a3    = testZipEntry{"a", 0755, []byte("Foo3"), zip.Deflate}
	bDir  = testZipEntry{"b/", os.ModeDir | 0755, nil, zip.Deflate}
	bbDir = testZipEntry{"b/b/", os.ModeDir | 0755, nil, zip.Deflate}
	bbb   = testZipEntry{"b/b/b", 0755, nil, zip.Deflate}
	ba    = testZipEntry{"b/a", 0755, []byte("foo"), zip.Deflate}
	bc    = testZipEntry{"b/c", 0755, []byte("bar"), zip.Deflate}
	bd    = testZipEntry{"b/d", 0700, []byte("baz"), zip.Deflate}
	be    = testZipEntry{"b/e", 0700, []byte(""), zip.Deflate}

	service1a        = testZipEntry{"META-INF/services/service1", 0755, []byte("class1\nclass2\n"), zip.Store}
	service1b        = testZipEntry{"META-INF/services/service1", 0755, []byte("class1\nclass3\n"), zip.Deflate}
	service1combined = testZipEntry{"META-INF/services/service1", 0755, []byte("class1\nclass2\nclass3\n"), zip.Store}
	service2         = testZipEntry{"META-INF/services/service2", 0755, []byte("class1\nclass2\n"), zip.Deflate}

	metainfDir     = testZipEntry{jar.MetaDir, os.ModeDir | 0755, nil, zip.Deflate}
	manifestFile   = testZipEntry{jar.ManifestFile, 0755, []byte("manifest"), zip.Deflate}
	manifestFile2  = testZipEntry{jar.ManifestFile, 0755, []byte("manifest2"), zip.Deflate}
	moduleInfoFile = testZipEntry{jar.ModuleInfoClass, 0755, []byte("module-info"), zip.Deflate}
)

type testInputZip struct {
	name    string
	entries []testZipEntry
	reader  *zip.Reader
}

func (tiz *testInputZip) Name() string {
	return tiz.name
}

func (tiz *testInputZip) Open() error {
	if tiz.reader == nil {
		tiz.reader = testZipEntriesToZipReader(tiz.entries)
	}
	return nil
}

func (tiz *testInputZip) Close() error {
	tiz.reader = nil
	return nil
}

func (tiz *testInputZip) Entries() []*zip.File {
	if tiz.reader == nil {
		panic(fmt.Errorf("%s: should be open to get entries", tiz.Name()))
	}
	return tiz.reader.File
}

func (tiz *testInputZip) IsOpen() bool {
	return tiz.reader != nil
}

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
		{
			name: "services",
			in: [][]testZipEntry{
				{service1a, service2},
				{service1b},
			},
			jar: true,
			out: []testZipEntry{service1combined, service2},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			inputZips := make([]InputZip, len(test.in))
			for i, in := range test.in {
				inputZips[i] = &testInputZip{name: "in" + strconv.Itoa(i), entries: in}
			}

			want := testZipEntriesToBuf(test.out)

			out := &bytes.Buffer{}
			writer := zip.NewWriter(out)

			err := mergeZips(inputZips, writer, "", "",
				test.sort, test.jar, false, test.stripDirEntries, test.ignoreDuplicates,
				test.stripFiles, test.stripDirs, test.zipsToNotStrip)

			closeErr := writer.Close()
			if closeErr != nil {
				t.Fatal(closeErr)
			}

			if test.err != "" {
				if err == nil {
					t.Fatal("missing err, expected: ", test.err)
				} else if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.err)) {
					t.Fatal("incorrect err, want:", test.err, "got:", err)
				}
				return
			} else if err != nil {
				t.Fatal("unexpected err: ", err)
			}

			if !bytes.Equal(want, out.Bytes()) {
				t.Error("incorrect zip output")
				t.Errorf("want:\n%s", dumpZip(want))
				t.Errorf("got:\n%s", dumpZip(out.Bytes()))
				os.WriteFile("/tmp/got.zip", out.Bytes(), 0755)
				os.WriteFile("/tmp/want.zip", want, 0755)
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
		fh.Method = e.method
		fh.UncompressedSize64 = uint64(len(e.data))
		fh.CRC32 = crc32.ChecksumIEEE(e.data)
		if fh.Method == zip.Store {
			fh.CompressedSize64 = fh.UncompressedSize64
		}

		w, err := zw.CreateHeaderAndroid(&fh)
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

type DummyInpuZip struct {
	isOpen bool
}

func (diz *DummyInpuZip) Name() string {
	return "dummy"
}

func (diz *DummyInpuZip) Open() error {
	diz.isOpen = true
	return nil
}

func (diz *DummyInpuZip) Close() error {
	diz.isOpen = false
	return nil
}

func (DummyInpuZip) Entries() []*zip.File {
	panic("implement me")
}

func (diz *DummyInpuZip) IsOpen() bool {
	return diz.isOpen
}

func TestInputZipsManager(t *testing.T) {
	const nInputZips = 20
	const nMaxOpenZips = 10
	izm := NewInputZipsManager(20, 10)
	managedZips := make([]InputZip, nInputZips)
	for i := 0; i < nInputZips; i++ {
		managedZips[i] = izm.Manage(&DummyInpuZip{})
	}

	t.Run("InputZipsManager", func(t *testing.T) {
		for i, iz := range managedZips {
			if err := iz.Open(); err != nil {
				t.Fatalf("Step %d: open failed: %s", i, err)
				return
			}
			if izm.nOpenZips > nMaxOpenZips {
				t.Errorf("Step %d: should be <=%d open zips", i, nMaxOpenZips)
			}
		}
		if !managedZips[nInputZips-1].IsOpen() {
			t.Error("The last input should stay open")
		}
		for _, iz := range managedZips {
			iz.Close()
		}
		if izm.nOpenZips > 0 {
			t.Error("Some input zips are still open")
		}
	})
}
