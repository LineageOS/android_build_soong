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
	"debug/elf"
	"fmt"
	"io"
)

type mockableElfFile interface {
	Symbols() ([]elf.Symbol, error)
	Sections() []elf.SectionHeader
	Type() elf.Type
}

var _ mockableElfFile = elfFileWrapper{}

type elfFileWrapper struct {
	*elf.File
}

func (f elfFileWrapper) Sections() []elf.SectionHeader {
	ret := make([]elf.SectionHeader, len(f.File.Sections))
	for i, section := range f.File.Sections {
		ret[i] = section.SectionHeader
	}

	return ret
}

func (f elfFileWrapper) Type() elf.Type {
	return f.File.Type
}

type mockElfFile struct {
	symbols  []elf.Symbol
	sections []elf.SectionHeader
	t        elf.Type
}

func (f mockElfFile) Sections() []elf.SectionHeader  { return f.sections }
func (f mockElfFile) Symbols() ([]elf.Symbol, error) { return f.symbols, nil }
func (f mockElfFile) Type() elf.Type                 { return f.t }

func elfSymbolsFromFile(r io.ReaderAt) (*File, error) {
	elfFile, err := elf.NewFile(r)
	if err != nil {
		return nil, cantParseError{err}
	}
	return extractElfSymbols(elfFileWrapper{elfFile})
}

func extractElfSymbols(elfFile mockableElfFile) (*File, error) {
	symbols, err := elfFile.Symbols()
	if err != nil {
		return nil, err
	}

	file := &File{}

	for _, section := range elfFile.Sections() {
		file.Sections = append(file.Sections, &Section{
			Name:   section.Name,
			Addr:   section.Addr,
			Offset: section.Offset,
			Size:   section.Size,
		})
	}

	_ = elf.Section{}

	for _, symbol := range symbols {
		if elf.ST_TYPE(symbol.Info) != elf.STT_OBJECT {
			continue
		}
		if symbol.Section == elf.SHN_UNDEF || symbol.Section >= elf.SHN_LORESERVE {
			continue
		}
		if int(symbol.Section) >= len(file.Sections) {
			return nil, fmt.Errorf("invalid section index %d", symbol.Section)
		}

		section := file.Sections[symbol.Section]

		var addr uint64
		switch elfFile.Type() {
		case elf.ET_REL:
			// "In relocatable files, st_value holds a section offset for a defined symbol.
			// That is, st_value is an offset from the beginning of the section that st_shndx identifies."
			addr = symbol.Value
		case elf.ET_EXEC, elf.ET_DYN:
			// "In executable and shared object files, st_value holds a virtual address. To make these
			// files’ symbols more useful for the dynamic linker, the section offset (file interpretation)
			// gives way to a virtual address (memory interpretation) for which the section number is
			// irrelevant."
			if symbol.Value < section.Addr {
				return nil, fmt.Errorf("symbol starts before the start of its section")
			}
			addr = symbol.Value - section.Addr
			if addr+symbol.Size > section.Size {
				return nil, fmt.Errorf("symbol extends past the end of its section")
			}
		default:
			return nil, fmt.Errorf("unsupported elf file type %d", elfFile.Type())
		}

		file.Symbols = append(file.Symbols, &Symbol{
			Name:    symbol.Name,
			Addr:    addr,
			Size:    symbol.Size,
			Section: section,
		})
	}

	return file, nil
}

func dumpElfSymbols(r io.ReaderAt) error {
	elfFile, err := elf.NewFile(r)
	if err != nil {
		return cantParseError{err}
	}

	symbols, err := elfFile.Symbols()
	if err != nil {
		return err
	}

	fmt.Println("mockElfFile{")
	fmt.Printf("\tt: %#v,\n", elfFile.Type)

	fmt.Println("\tsections: []elf.SectionHeader{")
	for _, section := range elfFile.Sections {
		fmt.Printf("\t\t%#v,\n", section.SectionHeader)
	}
	fmt.Println("\t},")

	fmt.Println("\tsymbols: []elf.Symbol{")
	for _, symbol := range symbols {
		fmt.Printf("\t\t%#v,\n", symbol)
	}
	fmt.Println("\t},")

	fmt.Println("}")

	return nil
}
