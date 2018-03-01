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
	"debug/macho"
	"fmt"
	"io"
	"sort"
)

func findMachoSymbol(r io.ReaderAt, symbolName string) (uint64, uint64, error) {
	machoFile, err := macho.NewFile(r)
	if err != nil {
		return maxUint64, maxUint64, cantParseError{err}
	}

	// symbols in macho files seem to be prefixed with an underscore
	symbolName = "_" + symbolName

	symbols := machoFile.Symtab.Syms
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].Sect != symbols[j].Sect {
			return symbols[i].Sect < symbols[j].Sect
		}
		return symbols[i].Value < symbols[j].Value
	})

	for _, symbol := range symbols {
		if symbol.Name == symbolName && symbol.Sect != 0 {
			// Find the next symbol in the same section with a higher address
			n := sort.Search(len(symbols), func(i int) bool {
				return symbols[i].Sect == symbol.Sect &&
					symbols[i].Value > symbol.Value
			})

			section := machoFile.Sections[symbol.Sect-1]

			var end uint64
			if n < len(symbols) {
				end = symbols[n].Value
			} else {
				end = section.Addr + section.Size
			}

			if end <= symbol.Value && end > symbol.Value+4096 {
				return maxUint64, maxUint64, fmt.Errorf("symbol end address does not seem valid, %x:%x", symbol.Value, end)
			}

			size := end - symbol.Value - 1
			offset := uint64(section.Offset) + (symbol.Value - section.Addr)

			return offset, size, nil
		}
	}

	return maxUint64, maxUint64, fmt.Errorf("symbol not found")
}
