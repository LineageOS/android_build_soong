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

	sort.Slice(peFile.Symbols, func(i, j int) bool {
		if peFile.Symbols[i].SectionNumber != peFile.Symbols[j].SectionNumber {
			return peFile.Symbols[i].SectionNumber < peFile.Symbols[j].SectionNumber
		}
		return peFile.Symbols[i].Value < peFile.Symbols[j].Value
	})

	for i, symbol := range peFile.Symbols {
		if symbol.Name == symbolName {
			var nextSymbol *pe.Symbol
			if i+1 < len(peFile.Symbols) {
				nextSymbol = peFile.Symbols[i+1]
			}
			return calculatePESymbolOffset(peFile, symbol, nextSymbol)
		}
	}

	return maxUint64, maxUint64, fmt.Errorf("symbol not found")
}

func calculatePESymbolOffset(file *pe.File, symbol *pe.Symbol, nextSymbol *pe.Symbol) (uint64, uint64, error) {
	section := file.Sections[symbol.SectionNumber-1]

	var end uint32
	if nextSymbol != nil && nextSymbol.SectionNumber != symbol.SectionNumber {
		nextSymbol = nil
	}
	if nextSymbol != nil {
		end = nextSymbol.Value
	} else {
		end = section.Size
	}

	size := end - symbol.Value - 1
	offset := section.Offset + symbol.Value

	return uint64(offset), uint64(size), nil
}
