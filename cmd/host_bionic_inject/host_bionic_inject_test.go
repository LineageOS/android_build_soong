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
	"testing"
)

// prog is a shortcut to fill out a elf.Prog structure
func prog(flags elf.ProgFlag, offset, addr, filesz, memsz uint64) *elf.Prog {
	return &elf.Prog{
		ProgHeader: elf.ProgHeader{
			Type:   elf.PT_LOAD,
			Flags:  flags,
			Off:    offset,
			Vaddr:  addr,
			Paddr:  addr,
			Filesz: filesz,
			Memsz:  memsz,
		},
	}
}

// linkerGold returns an example elf.File from a linker binary that was linked
// with gold.
func linkerGold() *elf.File {
	return &elf.File{
		Progs: []*elf.Prog{
			prog(elf.PF_R|elf.PF_X, 0, 0, 0xd0fac, 0xd0fac),
			prog(elf.PF_R|elf.PF_W, 0xd1050, 0xd2050, 0x6890, 0xd88c),
		},
	}
}

// fileGold returns an example elf binary with a properly embedded linker. The
// embedded linker was the one returned by linkerGold.
func fileGold() *elf.File {
	return &elf.File{
		Progs: []*elf.Prog{
			prog(elf.PF_R, 0, 0, 0x2e0, 0x2e0),
			prog(elf.PF_R|elf.PF_X, 0x1000, 0x1000, 0xd0fac, 0xd0fac),
			prog(elf.PF_R|elf.PF_W, 0xd2050, 0xd3050, 0xd88c, 0xd88c),
			prog(elf.PF_R, 0xe0000, 0xe1000, 0x10e4, 0x10e4),
			prog(elf.PF_R|elf.PF_X, 0xe2000, 0xe3000, 0x1360, 0x1360),
			prog(elf.PF_R|elf.PF_W, 0xe4000, 0xe5000, 0x1358, 0x1358),
		},
	}
}

// linkerLld returns an example elf.File from a linker binary that was linked
// with lld.
func linkerLld() *elf.File {
	return &elf.File{
		Progs: []*elf.Prog{
			prog(elf.PF_R, 0, 0, 0x3c944, 0x3c944),
			prog(elf.PF_R|elf.PF_X, 0x3d000, 0x3d000, 0x946fa, 0x946fa),
			prog(elf.PF_R|elf.PF_W, 0xd2000, 0xd2000, 0x7450, 0xf778),
		},
	}
}

// fileGold returns an example elf binary with a properly embedded linker. The
// embedded linker was the one returned by linkerLld.
func fileLld() *elf.File {
	return &elf.File{
		Progs: []*elf.Prog{
			prog(elf.PF_R, 0, 0, 0x3d944, 0x3d944),
			prog(elf.PF_R|elf.PF_X, 0x3e000, 0x3e000, 0x946fa, 0x946fa),
			prog(elf.PF_R|elf.PF_W, 0xd3000, 0xd3000, 0xf778, 0xf778),
			prog(elf.PF_R, 0xe3000, 0xe3000, 0x10e4, 0x10e4),
			prog(elf.PF_R|elf.PF_X, 0xe5000, 0xe5000, 0x1360, 0x1360),
			prog(elf.PF_R|elf.PF_W, 0xe7000, 0xe7000, 0x1358, 0x1358),
		},
	}
}

// linkerOffset returns the symbol representing the linker offset used by both
// fileGold and fileLld
func linkerOffset() []elf.Symbol {
	return []elf.Symbol{
		elf.Symbol{
			Name:  "__dlwrap_linker_offset",
			Value: 0x1000,
		},
	}
}

func TestCheckLinker(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		file   func() *elf.File
		linker func() *elf.File
	}{
		{
			name:   "good gold-linked linker",
			file:   fileGold,
			linker: linkerGold,
		},
		{
			name:   "good lld-linked linker",
			file:   fileLld,
			linker: linkerLld,
		},
		{
			name: "truncated RO section",
			err:  fmt.Errorf("Linker prog 0 (0x0) not fully present (0x3d944 > 0x3d943)"),
			file: func() *elf.File {
				f := fileLld()
				f.Progs[0].Filesz -= 1
				f.Progs[0].Memsz -= 1
				return f
			},
			linker: linkerLld,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkLinker(tc.file(), tc.linker(), linkerOffset())
			if tc.err == nil {
				if err != nil {
					t.Fatalf("No error expected, but got: %v", err)
				}
			} else if err == nil {
				t.Fatalf("Returned no error, but wanted: %v", tc.err)
			} else if err.Error() != tc.err.Error() {
				t.Fatalf("Different error found:\nwant: %v\n got: %v", tc.err, err)
			}
		})
	}
}
