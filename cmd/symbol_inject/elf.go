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
	"debug/elf"
	"fmt"
	"io"
)

func findElfSymbol(r io.ReaderAt, symbol string) (uint64, uint64, error) {
	elfFile, err := elf.NewFile(r)
	if err != nil {
		return maxUint64, maxUint64, cantParseError{err}
	}

	symbols, err := elfFile.Symbols()
	if err != nil {
		return maxUint64, maxUint64, err
	}

	for _, s := range symbols {
		if elf.ST_TYPE(s.Info) != elf.STT_OBJECT {
			continue
		}
		if s.Name == symbol {
			offset, err := calculateElfSymbolOffset(elfFile, s)
			if err != nil {
				return maxUint64, maxUint64, err
			}
			return offset, s.Size, nil
		}
	}

	return maxUint64, maxUint64, fmt.Errorf("symbol not found")
}

func calculateElfSymbolOffset(file *elf.File, symbol elf.Symbol) (uint64, error) {
	if symbol.Section == elf.SHN_UNDEF || int(symbol.Section) >= len(file.Sections) {
		return maxUint64, fmt.Errorf("invalid section index %d", symbol.Section)
	}
	section := file.Sections[symbol.Section]
	switch file.Type {
	case elf.ET_REL:
		// "In relocatable files, st_value holds a section offset for a defined symbol.
		// That is, st_value is an offset from the beginning of the section that st_shndx identifies."
		return section.Offset + symbol.Value, nil
	case elf.ET_EXEC, elf.ET_DYN:
		// "In executable and shared object files, st_value holds a virtual address. To make these
		// files’ symbols more useful for the dynamic linker, the section offset (file interpretation)
		// gives way to a virtual address (memory interpretation) for which the section number is
		// irrelevant."
		if symbol.Value < section.Addr {
			return maxUint64, fmt.Errorf("symbol starts before the start of its section")
		}
		section_offset := symbol.Value - section.Addr
		if section_offset+symbol.Size > section.Size {
			return maxUint64, fmt.Errorf("symbol extends past the end of its section")
		}
		return section.Offset + section_offset, nil
	default:
		return maxUint64, fmt.Errorf("unsupported elf file type %d", file.Type)
	}
}
