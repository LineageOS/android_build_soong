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
)

func findMachoSymbol(r io.ReaderAt, symbolName string) (uint64, uint64, error) {
	machoFile, err := macho.NewFile(r)
	if err != nil {
		return maxUint64, maxUint64, cantParseError{err}
	}

	// TODO(ccross): why?
	symbolName = "_" + symbolName

	for i, symbol := range machoFile.Symtab.Syms {
		if symbol.Sect == 0 {
			continue
		}
		if symbol.Name == symbolName {
			var nextSymbol *macho.Symbol
			if i+1 < len(machoFile.Symtab.Syms) {
				nextSymbol = &machoFile.Symtab.Syms[i+1]
			}
			return calculateMachoSymbolOffset(machoFile, symbol, nextSymbol)
		}
	}

	return maxUint64, maxUint64, fmt.Errorf("symbol not found")
}

func calculateMachoSymbolOffset(file *macho.File, symbol macho.Symbol, nextSymbol *macho.Symbol) (uint64, uint64, error) {
	section := file.Sections[symbol.Sect-1]

	var end uint64
	if nextSymbol != nil && nextSymbol.Sect != symbol.Sect {
		nextSymbol = nil
	}
	if nextSymbol != nil {
		end = nextSymbol.Value
	} else {
		end = section.Addr + section.Size
	}

	size := end - symbol.Value - 1
	offset := uint64(section.Offset) + (symbol.Value - section.Addr)

	return offset, size, nil
}
