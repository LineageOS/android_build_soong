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

// Verifies a host bionic executable with an embedded linker.
package main

import (
	"debug/elf"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	var inputFile, linkerFile string

	flag.StringVar(&inputFile, "i", "", "Input file")
	flag.StringVar(&linkerFile, "l", "", "Linker file")
	flag.Parse()

	if inputFile == "" || linkerFile == "" || flag.NArg() != 0 {
		flag.Usage()
		os.Exit(1)
	}

	r, err := os.Open(inputFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	defer r.Close()

	linker, err := elf.Open(linkerFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(4)
	}

	err = checkElf(r, linker)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(5)
	}
}

// Check the ELF file, and return the address to the _start function
func checkElf(r io.ReaderAt, linker *elf.File) error {
	file, err := elf.NewFile(r)
	if err != nil {
		return err
	}

	symbols, err := file.Symbols()
	if err != nil {
		return err
	}

	for _, prog := range file.Progs {
		if prog.Type == elf.PT_INTERP {
			return fmt.Errorf("File should not have a PT_INTERP header")
		}
	}

	if dlwrap_start, err := findSymbol(symbols, "__dlwrap__start"); err != nil {
		return err
	} else if dlwrap_start.Value != file.Entry {
		return fmt.Errorf("Expected file entry(0x%x) to point to __dlwrap_start(0x%x)",
			file.Entry, dlwrap_start.Value)
	}

	err = checkLinker(file, linker, symbols)
	if err != nil {
		return fmt.Errorf("Linker executable failed verification against app embedded linker: %s\n"+
			"linker might not be in sync with crtbegin_dynamic.o.",
			err)
	}

	return nil
}

func findSymbol(symbols []elf.Symbol, name string) (elf.Symbol, error) {
	for _, sym := range symbols {
		if sym.Name == name {
			return sym, nil
		}
	}
	return elf.Symbol{}, fmt.Errorf("Failed to find symbol %q", name)
}

// Check that all of the PT_LOAD segments have been embedded properly
func checkLinker(file, linker *elf.File, fileSyms []elf.Symbol) error {
	dlwrapLinkerOffset, err := findSymbol(fileSyms, "__dlwrap_linker_offset")
	if err != nil {
		return err
	}

	for i, lprog := range linker.Progs {
		if lprog.Type != elf.PT_LOAD {
			continue
		}

		laddr := lprog.Vaddr + dlwrapLinkerOffset.Value

		found := false
		for _, prog := range file.Progs {
			if prog.Type != elf.PT_LOAD {
				continue
			}

			if laddr < prog.Vaddr || laddr > prog.Vaddr+prog.Memsz {
				continue
			}
			found = true

			if lprog.Flags != prog.Flags {
				return fmt.Errorf("Linker prog %d (0x%x) flags (%s) do not match (%s)",
					i, lprog.Vaddr, lprog.Flags, prog.Flags)
			}

			if laddr+lprog.Memsz > prog.Vaddr+prog.Filesz {
				return fmt.Errorf("Linker prog %d (0x%x) not fully present (0x%x > 0x%x)",
					i, lprog.Vaddr, laddr+lprog.Memsz, prog.Vaddr+prog.Filesz)
			}
		}
		if !found {
			return fmt.Errorf("Linker prog %d (0x%x) not found at offset 0x%x",
				i, lprog.Vaddr, dlwrapLinkerOffset.Value)
		}
	}

	return nil
}
