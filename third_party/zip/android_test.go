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

package zip

import (
	"bytes"
	"testing"
)

var stripZip64Testcases = []struct {
	name string
	in   []byte
	out  []byte
}{
	{
		name: "empty",
		in:   []byte{},
		out:  []byte{},
	},
	{
		name: "trailing data",
		in:   []byte{1, 2, 3},
		out:  []byte{1, 2, 3},
	},
	{
		name: "valid non-zip64 extra",
		in:   []byte{2, 0, 2, 0, 1, 2},
		out:  []byte{2, 0, 2, 0, 1, 2},
	},
	{
		name: "two valid non-zip64 extras",
		in:   []byte{2, 0, 2, 0, 1, 2, 2, 0, 0, 0},
		out:  []byte{2, 0, 2, 0, 1, 2, 2, 0, 0, 0},
	},
	{
		name: "simple zip64 extra",
		in:   []byte{1, 0, 8, 0, 1, 2, 3, 4, 5, 6, 7, 8},
		out:  []byte{},
	},
	{
		name: "zip64 extra and valid non-zip64 extra",
		in:   []byte{1, 0, 8, 0, 1, 2, 3, 4, 5, 6, 7, 8, 2, 0, 0, 0},
		out:  []byte{2, 0, 0, 0},
	},
	{
		name: "invalid extra",
		in:   []byte{0, 0, 8, 0, 0, 0},
		out:  []byte{0, 0, 8, 0, 0, 0},
	},
	{
		name: "zip64 extra and extended-timestamp extra and valid non-zip64 extra",
		in:   []byte{1, 0, 8, 0, 1, 2, 3, 4, 5, 6, 7, 8, 85, 84, 5, 0, 1, 1, 2, 3, 4, 2, 0, 0, 0},
		out:  []byte{2, 0, 0, 0},
	},
}

func TestStripZip64Extras(t *testing.T) {
	for _, testcase := range stripZip64Testcases {
		got := stripExtras(testcase.in)
		if !bytes.Equal(got, testcase.out) {
			t.Errorf("Failed testcase %s\ninput: %v\n want: %v\n  got: %v\n", testcase.name, testcase.in, testcase.out, got)
		}
	}
}

func TestCopyFromZip64(t *testing.T) {
	if testing.Short() {
		t.Skip("slow test; skipping")
	}

	const size = uint32max + 1
	fromZipBytes := &bytes.Buffer{}
	fromZip := NewWriter(fromZipBytes)
	w, err := fromZip.CreateHeaderAndroid(&FileHeader{
		Name:               "large",
		Method:             Store,
		UncompressedSize64: size,
		CompressedSize64:   size,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = w.Write(make([]byte, size))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	err = fromZip.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	fromZip = nil

	fromZipReader, err := NewReader(bytes.NewReader(fromZipBytes.Bytes()), int64(fromZipBytes.Len()))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	toZipBytes := &bytes.Buffer{}
	toZip := NewWriter(toZipBytes)
	err = toZip.CopyFrom(fromZipReader.File[0], fromZipReader.File[0].Name)
	if err != nil {
		t.Fatalf("CopyFrom: %v", err)
	}

	err = toZip.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Save some memory
	fromZipReader = nil
	fromZipBytes.Reset()

	toZipReader, err := NewReader(bytes.NewReader(toZipBytes.Bytes()), int64(toZipBytes.Len()))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	if len(toZipReader.File) != 1 {
		t.Fatalf("Expected 1 file in toZip, got %d", len(toZipReader.File))
	}

	if g, w := toZipReader.File[0].CompressedSize64, uint64(size); g != w {
		t.Errorf("Expected CompressedSize64 %d, got %d", w, g)
	}

	if g, w := toZipReader.File[0].UncompressedSize64, uint64(size); g != w {
		t.Errorf("Expected UnompressedSize64 %d, got %d", w, g)
	}
}

// Test for b/187485108: zip64 output can't be read by p7zip 16.02.
func TestZip64P7ZipRecords(t *testing.T) {
	if testing.Short() {
		t.Skip("slow test; skipping")
	}

	const size = uint32max + 1
	zipBytes := &bytes.Buffer{}
	zip := NewWriter(zipBytes)
	f, err := zip.CreateHeaderAndroid(&FileHeader{
		Name:               "large",
		Method:             Store,
		UncompressedSize64: size,
		CompressedSize64:   size,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = f.Write(make([]byte, size))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	err = zip.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}

	buf := zipBytes.Bytes()
	p := findSignatureInBlock(buf)
	if p < 0 {
		t.Fatalf("Missing signature")
	}

	b := readBuf(buf[p+4:]) // skip signature
	d := &directoryEnd{
		diskNbr:            uint32(b.uint16()),
		dirDiskNbr:         uint32(b.uint16()),
		dirRecordsThisDisk: uint64(b.uint16()),
		directoryRecords:   uint64(b.uint16()),
		directorySize:      uint64(b.uint32()),
		directoryOffset:    uint64(b.uint32()),
		commentLen:         b.uint16(),
	}

	// p7zip 16.02 wants regular end record directoryRecords to be accurate.
	if g, w := d.directoryRecords, uint64(1); g != w {
		t.Errorf("wanted directoryRecords %d, got %d", w, g)
	}

	zip64ExtraBuf := 48                                                  // 4x uint16 + 5x uint64
	expectedDirSize := directoryHeaderLen + zip64ExtraBuf + len("large") // name of header
	if g, w := d.directorySize, uint64(expectedDirSize); g != w {
		t.Errorf("wanted directorySize %d, got %d", w, g)
	}

	if g, w := d.directoryOffset, uint64(uint32max); g != w {
		t.Errorf("wanted directoryOffset %d, got %d", w, g)
	}

	r := bytes.NewReader(buf)

	p64, err := findDirectory64End(r, int64(p))
	if err != nil {
		t.Fatalf("findDirectory64End: %v", err)
	}
	if p < 0 {
		t.Fatalf("findDirectory64End: not found")
	}
	err = readDirectory64End(r, p64, d)
	if err != nil {
		t.Fatalf("readDirectory64End: %v", err)
	}

	if g, w := d.directoryRecords, uint64(1); g != w {
		t.Errorf("wanted directoryRecords %d, got %d", w, g)
	}

	if g, w := d.directoryOffset, uint64(uint32max); g <= w {
		t.Errorf("wanted directoryOffset > %d, got %d", w, g)
	}
}
