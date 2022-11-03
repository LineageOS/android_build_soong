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

package symbol_inject

import (
	"debug/macho"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func machoSymbolsFromFile(r io.ReaderAt) (*File, error) {
	machoFile, err := macho.NewFile(r)
	if err != nil {
		return nil, cantParseError{err}
	}

	return extractMachoSymbols(machoFile)
}

func extractMachoSymbols(machoFile *macho.File) (*File, error) {
	symbols := machoFile.Symtab.Syms
	sort.SliceStable(symbols, func(i, j int) bool {
		if symbols[i].Sect != symbols[j].Sect {
			return symbols[i].Sect < symbols[j].Sect
		}
		return symbols[i].Value < symbols[j].Value
	})

	file := &File{IsMachoFile: true}

	for _, section := range machoFile.Sections {
		file.Sections = append(file.Sections, &Section{
			Name:   section.Name,
			Addr:   section.Addr,
			Offset: uint64(section.Offset),
			Size:   section.Size,
		})
	}

	for _, symbol := range symbols {
		if symbol.Sect > 0 {
			section := file.Sections[symbol.Sect-1]
			file.Symbols = append(file.Symbols, &Symbol{
				// symbols in macho files seem to be prefixed with an underscore
				Name: strings.TrimPrefix(symbol.Name, "_"),
				// MachO symbol value is virtual address of the symbol, convert it to offset into the section.
				Addr: symbol.Value - section.Addr,
				// MachO symbols don't have size information.
				Size:    0,
				Section: section,
			})
		}
	}

	return file, nil
}

func dumpMachoSymbols(r io.ReaderAt) error {
	machoFile, err := macho.NewFile(r)
	if err != nil {
		return cantParseError{err}
	}

	fmt.Println("&macho.File{")

	fmt.Println("\tSections: []*macho.Section{")
	for _, section := range machoFile.Sections {
		fmt.Printf("\t\t&macho.Section{SectionHeader: %#v},\n", section.SectionHeader)
	}
	fmt.Println("\t},")

	fmt.Println("\tSymtab: &macho.Symtab{")
	fmt.Println("\t\tSyms: []macho.Symbol{")
	for _, symbol := range machoFile.Symtab.Syms {
		fmt.Printf("\t\t\t%#v,\n", symbol)
	}
	fmt.Println("\t\t},")
	fmt.Println("\t},")

	fmt.Println("}")

	return nil
}

func CodeSignMachoFile(path string) error {
	filename := filepath.Base(path)
	cmd := exec.Command("/usr/bin/codesign", "--force", "-s", "-", "-i", filename, path)
	if err := cmd.Run(); err != nil {
		return err
	}
	return modifyCodeSignFlags(path)
}

const LC_CODE_SIGNATURE = 0x1d
const CSSLOT_CODEDIRECTORY = 0

// To make codesign not invalidated by stripping, modify codesign flags to 0x20002
// (adhoc | linkerSigned).
func modifyCodeSignFlags(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	// Step 1: find code signature section.
	machoFile, err := macho.NewFile(f)
	if err != nil {
		return err
	}
	var codeSignSectionOffset uint32 = 0
	var codeSignSectionSize uint32 = 0
	for _, l := range machoFile.Loads {
		data := l.Raw()
		cmd := machoFile.ByteOrder.Uint32(data)
		if cmd == LC_CODE_SIGNATURE {
			codeSignSectionOffset = machoFile.ByteOrder.Uint32(data[8:])
			codeSignSectionSize = machoFile.ByteOrder.Uint32(data[12:])
		}
	}
	if codeSignSectionOffset == 0 {
		return fmt.Errorf("code signature section not found")
	}

	data := make([]byte, codeSignSectionSize)
	_, err = f.ReadAt(data, int64(codeSignSectionOffset))
	if err != nil {
		return err
	}

	// Step 2: get flags offset.
	blobCount := binary.BigEndian.Uint32(data[8:])
	off := 12
	var codeDirectoryOff uint32 = 0
	for blobCount > 0 {
		blobType := binary.BigEndian.Uint32(data[off:])
		if blobType == CSSLOT_CODEDIRECTORY {
			codeDirectoryOff = binary.BigEndian.Uint32(data[off+4:])
			break
		}
		blobCount--
		off += 8
	}
	if codeDirectoryOff == 0 {
		return fmt.Errorf("no code directory in code signature section")
	}
	flagsOff := codeSignSectionOffset + codeDirectoryOff + 12

	// Step 3: modify flags.
	flagsData := make([]byte, 4)
	_, err = f.ReadAt(flagsData, int64(flagsOff))
	if err != nil {
		return err
	}
	oldFlags := binary.BigEndian.Uint32(flagsData)
	if oldFlags != 0x2 {
		return fmt.Errorf("unexpected flags in code signature section: 0x%x", oldFlags)
	}
	binary.BigEndian.PutUint32(flagsData, 0x20002)
	_, err = f.WriteAt(flagsData, int64(flagsOff))
	return err
}
