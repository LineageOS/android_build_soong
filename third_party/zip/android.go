// Copyright 2016 Google Inc. All rights reserved.
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
	"io"
)

func (w *Writer) CopyFrom(orig *File, newName string) error {
	if w.last != nil && !w.last.closed {
		if err := w.last.close(); err != nil {
			return err
		}
		w.last = nil
	}

	fileHeader := orig.FileHeader
	fileHeader.Name = newName
	fh := &fileHeader
	fh.Flags |= 0x8

	h := &header{
		FileHeader: fh,
		offset:     uint64(w.cw.count),
	}
	w.dir = append(w.dir, h)

	if err := writeHeader(w.cw, fh); err != nil {
		return err
	}

	// Copy data
	dataOffset, err := orig.DataOffset()
	if err != nil {
		return err
	}
	io.Copy(w.cw, io.NewSectionReader(orig.zipr, dataOffset, int64(orig.CompressedSize64)))

	// Write data descriptor.
	var buf []byte
	if fh.isZip64() {
		buf = make([]byte, dataDescriptor64Len)
	} else {
		buf = make([]byte, dataDescriptorLen)
	}
	b := writeBuf(buf)
	b.uint32(dataDescriptorSignature)
	b.uint32(fh.CRC32)
	if fh.isZip64() {
		b.uint64(fh.CompressedSize64)
		b.uint64(fh.UncompressedSize64)
	} else {
		b.uint32(fh.CompressedSize)
		b.uint32(fh.UncompressedSize)
	}
	_, err = w.cw.Write(buf)
	return err
}
