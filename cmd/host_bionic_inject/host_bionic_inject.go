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

// Verifies a host bionic executable with an embedded linker, then injects
// the address of the _start function for the linker_wrapper to use.
package main

import (
	"debug/elf"
	"flag"
	"fmt"
	"io"
	"os"

	"android/soong/symbol_inject"
)

func main() {
	var inputFile, linkerFile, outputFile string

	flag.StringVar(&inputFile, "i", "", "Input file")
	flag.StringVar(&linkerFile, "l", "", "Linker file")
	flag.StringVar(&outputFile, "o", "", "Output file")
	flag.Parse()

	if inputFile == "" || linkerFile == "" || outputFile == "" || flag.NArg() != 0 {
		flag.Usage()
		os.Exit(1)
	}

	r, err := os.Open(inputFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	defer r.Close()

	file, err := symbol_inject.OpenFile(r)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(3)
	}

	linker, err := elf.Open(linkerFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(4)
	}

	start_addr, err := parseElf(r, linker)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(5)
	}

	w, err := os.OpenFile(outputFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0777)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(6)
	}
	defer w.Close()

	err = symbol_inject.InjectUint64Symbol(file, w, "__dlwrap_original_start", start_addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(7)
	}
}

// Check the ELF file, and return the address to the _start function
func parseElf(r io.ReaderAt, linker *elf.File) (uint64, error) {
	file, err := elf.NewFile(r)
	if err != nil {
		return 0, err
	}

	symbols, err := file.Symbols()
	if err != nil {
		return 0, err
	}

	for _, prog := range file.Progs {
		if prog.Type == elf.PT_INTERP {
			return 0, fmt.Errorf("File should not have a PT_INTERP header")
		}
	}

	if dlwrap_start, err := findSymbol(symbols, "__dlwrap__start"); err != nil {
		return 0, err
	} else if dlwrap_start.Value != file.Entry {
		return 0, fmt.Errorf("Expected file entry(0x%x) to point to __dlwrap_start(0x%x)",
			file.Entry, dlwrap_start.Value)
	}

	err = checkLinker(file, linker, symbols)
	if err != nil {
		return 0, err
	}

	start, err := findSymbol(symbols, "_start")
	if err != nil {
		return 0, fmt.Errorf("Failed to find _start symbol")
	}
	return start.Value, nil
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
	dlwrap_linker_offset, err := findSymbol(fileSyms, "__dlwrap_linker_offset")
	if err != nil {
		return err
	}

	for i, lprog := range linker.Progs {
		if lprog.Type != elf.PT_LOAD {
			continue
		}

		laddr := lprog.Vaddr + dlwrap_linker_offset.Value

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
				i, lprog.Vaddr, dlwrap_linker_offset.Value)
		}
	}

	return nil
}
