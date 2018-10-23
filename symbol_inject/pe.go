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
	"debug/pe"
	"fmt"
	"io"
	"sort"
	"strings"
)

func peSymbolsFromFile(r io.ReaderAt) (*File, error) {
	peFile, err := pe.NewFile(r)
	if err != nil {
		return nil, cantParseError{err}
	}

	return extractPESymbols(peFile)
}

func extractPESymbols(peFile *pe.File) (*File, error) {
	var prefix string
	if peFile.FileHeader.Machine == pe.IMAGE_FILE_MACHINE_I386 {
		// symbols in win32 exes seem to be prefixed with an underscore
		prefix = "_"
	}

	symbols := peFile.Symbols
	sort.SliceStable(symbols, func(i, j int) bool {
		if symbols[i].SectionNumber != symbols[j].SectionNumber {
			return symbols[i].SectionNumber < symbols[j].SectionNumber
		}
		return symbols[i].Value < symbols[j].Value
	})

	file := &File{}

	for _, section := range peFile.Sections {
		file.Sections = append(file.Sections, &Section{
			Name:   section.Name,
			Addr:   uint64(section.VirtualAddress),
			Offset: uint64(section.Offset),
			Size:   uint64(section.VirtualSize),
		})
	}

	for _, symbol := range symbols {
		if symbol.SectionNumber > 0 {
			file.Symbols = append(file.Symbols, &Symbol{
				Name: strings.TrimPrefix(symbol.Name, prefix),
				// PE symbol value is the offset of the symbol into the section
				Addr: uint64(symbol.Value),
				// PE symbols don't have size information
				Size:    0,
				Section: file.Sections[symbol.SectionNumber-1],
			})
		}
	}

	return file, nil
}

func dumpPESymbols(r io.ReaderAt) error {
	peFile, err := pe.NewFile(r)
	if err != nil {
		return cantParseError{err}
	}

	fmt.Println("&pe.File{")
	fmt.Println("\tFileHeader: pe.FileHeader{")
	fmt.Printf("\t\tMachine: %#v,\n", peFile.FileHeader.Machine)
	fmt.Println("\t},")

	fmt.Println("\tSections: []*pe.Section{")
	for _, section := range peFile.Sections {
		fmt.Printf("\t\t&pe.Section{SectionHeader: %#v},\n", section.SectionHeader)
	}
	fmt.Println("\t},")

	fmt.Println("\tSymbols: []*pe.Symbol{")
	for _, symbol := range peFile.Symbols {
		fmt.Printf("\t\t%#v,\n", symbol)
	}
	fmt.Println("\t},")

	fmt.Println("}")

	return nil
}
