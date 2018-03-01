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
	"debug/pe"
	"fmt"
	"io"
	"sort"
)

func findPESymbol(r io.ReaderAt, symbolName string) (uint64, uint64, error) {
	peFile, err := pe.NewFile(r)
	if err != nil {
		return maxUint64, maxUint64, cantParseError{err}
	}

	if peFile.FileHeader.Machine == pe.IMAGE_FILE_MACHINE_I386 {
		// symbols in win32 exes seem to be prefixed with an underscore
		symbolName = "_" + symbolName
	}

	symbols := peFile.Symbols
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].SectionNumber != symbols[j].SectionNumber {
			return symbols[i].SectionNumber < symbols[j].SectionNumber
		}
		return symbols[i].Value < symbols[j].Value
	})

	for _, symbol := range symbols {
		if symbol.Name == symbolName {
			// Find the next symbol (n the same section with a higher address
			n := sort.Search(len(symbols), func(i int) bool {
				return symbols[i].SectionNumber == symbol.SectionNumber &&
					symbols[i].Value > symbol.Value
			})

			section := peFile.Sections[symbol.SectionNumber-1]

			var end uint32
			if n < len(symbols) {
				end = symbols[n].Value
			} else {
				end = section.Size
			}

			if end <= symbol.Value && end > symbol.Value+4096 {
				return maxUint64, maxUint64, fmt.Errorf("symbol end address does not seem valid, %x:%x", symbol.Value, end)
			}

			size := end - symbol.Value - 1
			offset := section.Offset + symbol.Value

			return uint64(offset), uint64(size), nil
		}
	}

	return maxUint64, maxUint64, fmt.Errorf("symbol not found")
}
