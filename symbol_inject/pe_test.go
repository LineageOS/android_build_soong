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
	"strconv"
	"testing"
)

func TestPESymbolTable(t *testing.T) {
	testCases := []struct {
		file         *pe.File
		symbol       string
		offset, size uint64
	}{
		{
			file:   peSymbolTable1,
			symbol: "soong_build_number",
			offset: 0x2420,
			size:   128,
		},
		{
			file:   peSymbolTable2,
			symbol: "symbol1",
			offset: 0x2420,
			size:   128,
		},
		{
			file:   peSymbolTable2,
			symbol: "symbol2",
			offset: 0x24a0,
			size:   128,
		},
		{
			// Test when symbol has the same value as the target symbol but is located afterwards in the list
			file: &pe.File{
				FileHeader: pe.FileHeader{
					Machine: pe.IMAGE_FILE_MACHINE_I386,
				},
				Sections: []*pe.Section{
					&pe.Section{SectionHeader: pe.SectionHeader{Name: ".text", VirtualSize: 0x15e83c, VirtualAddress: 0x1000, Size: 0x15ea00, Offset: 0x600, PointerToRelocations: 0x0, PointerToLineNumbers: 0x0, NumberOfRelocations: 0x0, NumberOfLineNumbers: 0x0, Characteristics: 0x60500060}},
					&pe.Section{SectionHeader: pe.SectionHeader{Name: ".data", VirtualSize: 0x6a58, VirtualAddress: 0x160000, Size: 0x6c00, Offset: 0x15f000, PointerToRelocations: 0x0, PointerToLineNumbers: 0x0, NumberOfRelocations: 0x0, NumberOfLineNumbers: 0x0, Characteristics: 0xc0600040}},
				},
				Symbols: []*pe.Symbol{
					&pe.Symbol{Name: "_soong_build_number", Value: 0x20, SectionNumber: 2, Type: 0x0, StorageClass: 0x2},
					&pe.Symbol{Name: ".data", Value: 0x20, SectionNumber: 2, Type: 0x0, StorageClass: 0x3},
					&pe.Symbol{Name: "_adb_device_banner", Value: 0xa0, SectionNumber: 2, Type: 0x0, StorageClass: 0x2},
				},
			},
			symbol: "soong_build_number",
			offset: 0x15f020,
			size:   128,
		},
		{
			// Test when symbol has nothing after it
			file: &pe.File{
				FileHeader: pe.FileHeader{
					Machine: pe.IMAGE_FILE_MACHINE_AMD64,
				},
				Sections: []*pe.Section{
					&pe.Section{SectionHeader: pe.SectionHeader{Name: ".text", VirtualSize: 0x1cc0, VirtualAddress: 0x1000, Size: 0x1e00, Offset: 0x600, PointerToRelocations: 0x0, PointerToLineNumbers: 0x0, NumberOfRelocations: 0x0, NumberOfLineNumbers: 0x0, Characteristics: 0x60500020}},
					&pe.Section{SectionHeader: pe.SectionHeader{Name: ".data", VirtualSize: 0xa0, VirtualAddress: 0x3000, Size: 0x200, Offset: 0x2400, PointerToRelocations: 0x0, PointerToLineNumbers: 0x0, NumberOfRelocations: 0x0, NumberOfLineNumbers: 0x0, Characteristics: 0xc0600040}},
				},
				Symbols: []*pe.Symbol{
					&pe.Symbol{Name: "soong_build_number", Value: 0x20, SectionNumber: 2, Type: 0x0, StorageClass: 0x2},
				},
			},
			symbol: "soong_build_number",
			offset: 0x2420,
			size:   128,
		},
		{
			// Test when symbol has a symbol in a different section after it
			file: &pe.File{
				FileHeader: pe.FileHeader{
					Machine: pe.IMAGE_FILE_MACHINE_AMD64,
				},
				Sections: []*pe.Section{
					&pe.Section{SectionHeader: pe.SectionHeader{Name: ".text", VirtualSize: 0x1cc0, VirtualAddress: 0x1000, Size: 0x1e00, Offset: 0x600, PointerToRelocations: 0x0, PointerToLineNumbers: 0x0, NumberOfRelocations: 0x0, NumberOfLineNumbers: 0x0, Characteristics: 0x60500020}},
					&pe.Section{SectionHeader: pe.SectionHeader{Name: ".data", VirtualSize: 0xa0, VirtualAddress: 0x3000, Size: 0x200, Offset: 0x2400, PointerToRelocations: 0x0, PointerToLineNumbers: 0x0, NumberOfRelocations: 0x0, NumberOfLineNumbers: 0x0, Characteristics: 0xc0600040}},
					&pe.Section{SectionHeader: pe.SectionHeader{Name: ".rdata", VirtualSize: 0x5e0, VirtualAddress: 0x4000, Size: 0x600, Offset: 0x2600, PointerToRelocations: 0x0, PointerToLineNumbers: 0x0, NumberOfRelocations: 0x0, NumberOfLineNumbers: 0x0, Characteristics: 0x40500040}},
				},
				Symbols: []*pe.Symbol{
					&pe.Symbol{Name: "soong_build_number", Value: 0x20, SectionNumber: 2, Type: 0x0, StorageClass: 0x2},
					&pe.Symbol{Name: "_adb_device_banner", Value: 0x30, SectionNumber: 3, Type: 0x0, StorageClass: 0x2},
				},
			},
			symbol: "soong_build_number",
			offset: 0x2420,
			size:   128,
		},
		{
			// Test when symbols are out of order
			file: &pe.File{
				FileHeader: pe.FileHeader{
					Machine: pe.IMAGE_FILE_MACHINE_AMD64,
				},
				Sections: []*pe.Section{
					&pe.Section{SectionHeader: pe.SectionHeader{Name: ".text", VirtualSize: 0x1cc0, VirtualAddress: 0x1000, Size: 0x1e00, Offset: 0x600, PointerToRelocations: 0x0, PointerToLineNumbers: 0x0, NumberOfRelocations: 0x0, NumberOfLineNumbers: 0x0, Characteristics: 0x60500020}},
					&pe.Section{SectionHeader: pe.SectionHeader{Name: ".data", VirtualSize: 0x120, VirtualAddress: 0x3000, Size: 0x200, Offset: 0x2400, PointerToRelocations: 0x0, PointerToLineNumbers: 0x0, NumberOfRelocations: 0x0, NumberOfLineNumbers: 0x0, Characteristics: 0xc0600040}},
				},
				Symbols: []*pe.Symbol{
					&pe.Symbol{Name: "_adb_device_banner", Value: 0xa0, SectionNumber: 2, Type: 0x0, StorageClass: 0x2},
					&pe.Symbol{Name: "soong_build_number", Value: 0x20, SectionNumber: 2, Type: 0x0, StorageClass: 0x2},
				},
			},
			symbol: "soong_build_number",
			offset: 0x2420,
			size:   128,
		},
	}

	for i, testCase := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			file, err := extractPESymbols(testCase.file)
			if err != nil {
				t.Error(err.Error())
			}
			offset, size, err := findSymbol(file, testCase.symbol)
			if err != nil {
				t.Error(err.Error())
			}
			if offset != testCase.offset || size != testCase.size {
				t.Errorf("expected %x:%x, got %x:%x", testCase.offset, testCase.size, offset, size)
			}
		})
	}
}
