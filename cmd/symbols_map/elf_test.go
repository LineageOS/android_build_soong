// Copyright 2022 Google Inc. All rights reserved.
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
	"debug/elf"
	"encoding/binary"
	"reflect"
	"testing"
)

func Test_elfIdentifierFromReaderAt_BadElfFile(t *testing.T) {
	tests := []struct {
		name     string
		contents string
	}{
		{
			name:     "empty",
			contents: "",
		},
		{
			name:     "text",
			contents: "#!/bin/bash\necho foobar",
		},
		{
			name:     "empty elf",
			contents: emptyElfFile(),
		},
		{
			name:     "short section header",
			contents: shortSectionHeaderElfFile(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := bytes.NewReader([]byte(tt.contents))
			_, err := elfIdentifierFromReaderAt(buf, "<>", false)
			if err == nil {
				t.Errorf("expected error reading bad elf file without allowMissing")
			}
			_, err = elfIdentifierFromReaderAt(buf, "<>", true)
			if err != nil {
				t.Errorf("expected no error reading bad elf file with allowMissing, got %q", err.Error())
			}
		})
	}
}

func Test_readNote(t *testing.T) {
	note := []byte{
		0x04, 0x00, 0x00, 0x00,
		0x10, 0x00, 0x00, 0x00,
		0x03, 0x00, 0x00, 0x00,
		0x47, 0x4e, 0x55, 0x00,
		0xca, 0xaf, 0x44, 0xd2, 0x82, 0x78, 0x68, 0xfe, 0xc0, 0x90, 0xa3, 0x43, 0x85, 0x36, 0x6c, 0xc7,
	}

	descs, err := readNote(bytes.NewBuffer(note), binary.LittleEndian)
	if err != nil {
		t.Fatalf("unexpected error in readNote: %s", err)
	}

	expectedDescs := map[string][]byte{
		"GNU\x00": []byte{0xca, 0xaf, 0x44, 0xd2, 0x82, 0x78, 0x68, 0xfe, 0xc0, 0x90, 0xa3, 0x43, 0x85, 0x36, 0x6c, 0xc7},
	}

	if !reflect.DeepEqual(descs, expectedDescs) {
		t.Errorf("incorrect return, want %#v got %#v", expectedDescs, descs)
	}
}

// emptyElfFile returns an elf file header with no program headers or sections.
func emptyElfFile() string {
	ident := [elf.EI_NIDENT]byte{}
	identBuf := bytes.NewBuffer(ident[0:0:elf.EI_NIDENT])
	binary.Write(identBuf, binary.LittleEndian, []byte("\x7fELF"))
	binary.Write(identBuf, binary.LittleEndian, elf.ELFCLASS64)
	binary.Write(identBuf, binary.LittleEndian, elf.ELFDATA2LSB)
	binary.Write(identBuf, binary.LittleEndian, elf.EV_CURRENT)
	binary.Write(identBuf, binary.LittleEndian, elf.ELFOSABI_LINUX)
	binary.Write(identBuf, binary.LittleEndian, make([]byte, 8))

	header := elf.Header64{
		Ident:     ident,
		Type:      uint16(elf.ET_EXEC),
		Machine:   uint16(elf.EM_X86_64),
		Version:   uint32(elf.EV_CURRENT),
		Entry:     0,
		Phoff:     uint64(binary.Size(elf.Header64{})),
		Shoff:     uint64(binary.Size(elf.Header64{})),
		Flags:     0,
		Ehsize:    uint16(binary.Size(elf.Header64{})),
		Phentsize: 0x38,
		Phnum:     0,
		Shentsize: 0x40,
		Shnum:     0,
		Shstrndx:  0,
	}

	buf := &bytes.Buffer{}
	binary.Write(buf, binary.LittleEndian, header)
	return buf.String()
}

// shortSectionHeader returns an elf file header with a section header that extends past the end of
// the file.
func shortSectionHeaderElfFile() string {
	ident := [elf.EI_NIDENT]byte{}
	identBuf := bytes.NewBuffer(ident[0:0:elf.EI_NIDENT])
	binary.Write(identBuf, binary.LittleEndian, []byte("\x7fELF"))
	binary.Write(identBuf, binary.LittleEndian, elf.ELFCLASS64)
	binary.Write(identBuf, binary.LittleEndian, elf.ELFDATA2LSB)
	binary.Write(identBuf, binary.LittleEndian, elf.EV_CURRENT)
	binary.Write(identBuf, binary.LittleEndian, elf.ELFOSABI_LINUX)
	binary.Write(identBuf, binary.LittleEndian, make([]byte, 8))

	header := elf.Header64{
		Ident:     ident,
		Type:      uint16(elf.ET_EXEC),
		Machine:   uint16(elf.EM_X86_64),
		Version:   uint32(elf.EV_CURRENT),
		Entry:     0,
		Phoff:     uint64(binary.Size(elf.Header64{})),
		Shoff:     uint64(binary.Size(elf.Header64{})),
		Flags:     0,
		Ehsize:    uint16(binary.Size(elf.Header64{})),
		Phentsize: 0x38,
		Phnum:     0,
		Shentsize: 0x40,
		Shnum:     1,
		Shstrndx:  0,
	}

	buf := &bytes.Buffer{}
	binary.Write(buf, binary.LittleEndian, header)
	binary.Write(buf, binary.LittleEndian, []byte{0})
	return buf.String()
}
