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

import "debug/elf"

/* Generated from: clang -o a.out test.c

#include <unistd.h>

char soong_build_number[128] = "PLACEHOLDER";

int main() {
  write(STDOUT_FILENO, soong_build_number, sizeof(soong_build_number));
}
*/

var elfSymbolTable1 = &mockElfFile{
	t: elf.ET_EXEC,
	sections: []elf.SectionHeader{
		elf.SectionHeader{Name: "", Type: elf.SHT_NULL, Flags: 0x0, Addr: 0x0, Offset: 0x0, Size: 0x0, Link: 0x0, Info: 0x0, Addralign: 0x0, Entsize: 0x0, FileSize: 0x0},
		elf.SectionHeader{Name: ".interp", Type: elf.SHT_PROGBITS, Flags: elf.SHF_ALLOC, Addr: 0x400238, Offset: 0x238, Size: 0x1c, Link: 0x0, Info: 0x0, Addralign: 0x1, Entsize: 0x0, FileSize: 0x1c},
		elf.SectionHeader{Name: ".note.ABI-tag", Type: elf.SHT_NOTE, Flags: elf.SHF_ALLOC, Addr: 0x400254, Offset: 0x254, Size: 0x20, Link: 0x0, Info: 0x0, Addralign: 0x4, Entsize: 0x0, FileSize: 0x20},
		elf.SectionHeader{Name: ".hash", Type: elf.SHT_HASH, Flags: elf.SHF_ALLOC, Addr: 0x400278, Offset: 0x278, Size: 0x24, Link: 0x4, Info: 0x0, Addralign: 0x8, Entsize: 0x4, FileSize: 0x24},
		elf.SectionHeader{Name: ".dynsym", Type: elf.SHT_DYNSYM, Flags: elf.SHF_ALLOC, Addr: 0x4002a0, Offset: 0x2a0, Size: 0x60, Link: 0x5, Info: 0x1, Addralign: 0x8, Entsize: 0x18, FileSize: 0x60},
		elf.SectionHeader{Name: ".dynstr", Type: elf.SHT_STRTAB, Flags: elf.SHF_ALLOC, Addr: 0x400300, Offset: 0x300, Size: 0x3e, Link: 0x0, Info: 0x0, Addralign: 0x1, Entsize: 0x0, FileSize: 0x3e},
		elf.SectionHeader{Name: ".gnu.version", Type: elf.SHT_GNU_VERSYM, Flags: elf.SHF_ALLOC, Addr: 0x40033e, Offset: 0x33e, Size: 0x8, Link: 0x4, Info: 0x0, Addralign: 0x2, Entsize: 0x2, FileSize: 0x8},
		elf.SectionHeader{Name: ".gnu.version_r", Type: elf.SHT_GNU_VERNEED, Flags: elf.SHF_ALLOC, Addr: 0x400348, Offset: 0x348, Size: 0x20, Link: 0x5, Info: 0x1, Addralign: 0x8, Entsize: 0x0, FileSize: 0x20},
		elf.SectionHeader{Name: ".rela.dyn", Type: elf.SHT_RELA, Flags: elf.SHF_ALLOC, Addr: 0x400368, Offset: 0x368, Size: 0x30, Link: 0x4, Info: 0x0, Addralign: 0x8, Entsize: 0x18, FileSize: 0x30},
		elf.SectionHeader{Name: ".rela.plt", Type: elf.SHT_RELA, Flags: elf.SHF_ALLOC + elf.SHF_INFO_LINK, Addr: 0x400398, Offset: 0x398, Size: 0x18, Link: 0x4, Info: 0x16, Addralign: 0x8, Entsize: 0x18, FileSize: 0x18},
		elf.SectionHeader{Name: ".init", Type: elf.SHT_PROGBITS, Flags: elf.SHF_ALLOC + elf.SHF_EXECINSTR, Addr: 0x4003b0, Offset: 0x3b0, Size: 0x17, Link: 0x0, Info: 0x0, Addralign: 0x4, Entsize: 0x0, FileSize: 0x17},
		elf.SectionHeader{Name: ".plt", Type: elf.SHT_PROGBITS, Flags: elf.SHF_ALLOC + elf.SHF_EXECINSTR, Addr: 0x4003d0, Offset: 0x3d0, Size: 0x20, Link: 0x0, Info: 0x0, Addralign: 0x10, Entsize: 0x10, FileSize: 0x20},
		elf.SectionHeader{Name: ".text", Type: elf.SHT_PROGBITS, Flags: elf.SHF_ALLOC + elf.SHF_EXECINSTR, Addr: 0x4003f0, Offset: 0x3f0, Size: 0x1b2, Link: 0x0, Info: 0x0, Addralign: 0x10, Entsize: 0x0, FileSize: 0x1b2},
		elf.SectionHeader{Name: ".fini", Type: elf.SHT_PROGBITS, Flags: elf.SHF_ALLOC + elf.SHF_EXECINSTR, Addr: 0x4005a4, Offset: 0x5a4, Size: 0x9, Link: 0x0, Info: 0x0, Addralign: 0x4, Entsize: 0x0, FileSize: 0x9},
		elf.SectionHeader{Name: ".rodata", Type: elf.SHT_PROGBITS, Flags: elf.SHF_ALLOC + elf.SHF_MERGE, Addr: 0x4005b0, Offset: 0x5b0, Size: 0x4, Link: 0x0, Info: 0x0, Addralign: 0x4, Entsize: 0x4, FileSize: 0x4},
		elf.SectionHeader{Name: ".eh_frame_hdr", Type: elf.SHT_PROGBITS, Flags: elf.SHF_ALLOC, Addr: 0x4005b4, Offset: 0x5b4, Size: 0x34, Link: 0x0, Info: 0x0, Addralign: 0x4, Entsize: 0x0, FileSize: 0x34},
		elf.SectionHeader{Name: ".eh_frame", Type: elf.SHT_PROGBITS, Flags: elf.SHF_ALLOC, Addr: 0x4005e8, Offset: 0x5e8, Size: 0xf0, Link: 0x0, Info: 0x0, Addralign: 0x8, Entsize: 0x0, FileSize: 0xf0},
		elf.SectionHeader{Name: ".init_array", Type: elf.SHT_INIT_ARRAY, Flags: elf.SHF_WRITE + elf.SHF_ALLOC, Addr: 0x600e08, Offset: 0xe08, Size: 0x8, Link: 0x0, Info: 0x0, Addralign: 0x8, Entsize: 0x8, FileSize: 0x8},
		elf.SectionHeader{Name: ".fini_array", Type: elf.SHT_FINI_ARRAY, Flags: elf.SHF_WRITE + elf.SHF_ALLOC, Addr: 0x600e10, Offset: 0xe10, Size: 0x8, Link: 0x0, Info: 0x0, Addralign: 0x8, Entsize: 0x8, FileSize: 0x8},
		elf.SectionHeader{Name: ".jcr", Type: elf.SHT_PROGBITS, Flags: elf.SHF_WRITE + elf.SHF_ALLOC, Addr: 0x600e18, Offset: 0xe18, Size: 0x8, Link: 0x0, Info: 0x0, Addralign: 0x8, Entsize: 0x0, FileSize: 0x8},
		elf.SectionHeader{Name: ".dynamic", Type: elf.SHT_DYNAMIC, Flags: elf.SHF_WRITE + elf.SHF_ALLOC, Addr: 0x600e20, Offset: 0xe20, Size: 0x1d0, Link: 0x5, Info: 0x0, Addralign: 0x8, Entsize: 0x10, FileSize: 0x1d0},
		elf.SectionHeader{Name: ".got", Type: elf.SHT_PROGBITS, Flags: elf.SHF_WRITE + elf.SHF_ALLOC, Addr: 0x600ff0, Offset: 0xff0, Size: 0x10, Link: 0x0, Info: 0x0, Addralign: 0x8, Entsize: 0x8, FileSize: 0x10},
		elf.SectionHeader{Name: ".got.plt", Type: elf.SHT_PROGBITS, Flags: elf.SHF_WRITE + elf.SHF_ALLOC, Addr: 0x601000, Offset: 0x1000, Size: 0x20, Link: 0x0, Info: 0x0, Addralign: 0x8, Entsize: 0x8, FileSize: 0x20},
		elf.SectionHeader{Name: ".data", Type: elf.SHT_PROGBITS, Flags: elf.SHF_WRITE + elf.SHF_ALLOC, Addr: 0x601020, Offset: 0x1020, Size: 0x90, Link: 0x0, Info: 0x0, Addralign: 0x10, Entsize: 0x0, FileSize: 0x90},
		elf.SectionHeader{Name: ".bss", Type: elf.SHT_NOBITS, Flags: elf.SHF_WRITE + elf.SHF_ALLOC, Addr: 0x6010b0, Offset: 0x10b0, Size: 0x8, Link: 0x0, Info: 0x0, Addralign: 0x1, Entsize: 0x0, FileSize: 0x8},
		elf.SectionHeader{Name: ".comment", Type: elf.SHT_PROGBITS, Flags: elf.SHF_MERGE + elf.SHF_STRINGS, Addr: 0x0, Offset: 0x10b0, Size: 0x56, Link: 0x0, Info: 0x0, Addralign: 0x1, Entsize: 0x1, FileSize: 0x56},
		elf.SectionHeader{Name: ".symtab", Type: elf.SHT_SYMTAB, Flags: 0x0, Addr: 0x0, Offset: 0x1108, Size: 0x5e8, Link: 0x1b, Info: 0x2d, Addralign: 0x8, Entsize: 0x18, FileSize: 0x5e8},
		elf.SectionHeader{Name: ".strtab", Type: elf.SHT_STRTAB, Flags: 0x0, Addr: 0x0, Offset: 0x16f0, Size: 0x1dd, Link: 0x0, Info: 0x0, Addralign: 0x1, Entsize: 0x0, FileSize: 0x1dd},
		elf.SectionHeader{Name: ".shstrtab", Type: elf.SHT_STRTAB, Flags: 0x0, Addr: 0x0, Offset: 0x18cd, Size: 0xf1, Link: 0x0, Info: 0x0, Addralign: 0x1, Entsize: 0x0, FileSize: 0xf1},
	},
	symbols: []elf.Symbol{
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 1, Value: 0x400238, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 2, Value: 0x400254, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 3, Value: 0x400278, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 4, Value: 0x4002a0, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 5, Value: 0x400300, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 6, Value: 0x40033e, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 7, Value: 0x400348, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 8, Value: 0x400368, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 9, Value: 0x400398, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 10, Value: 0x4003b0, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 11, Value: 0x4003d0, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x4003f0, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 13, Value: 0x4005a4, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 14, Value: 0x4005b0, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 15, Value: 0x4005b4, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 16, Value: 0x4005e8, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 17, Value: 0x600e08, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 18, Value: 0x600e10, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 19, Value: 0x600e18, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 20, Value: 0x600e20, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 21, Value: 0x600ff0, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 22, Value: 0x601000, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 23, Value: 0x601020, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 24, Value: 0x6010b0, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 25, Value: 0x0, Size: 0x0},
		elf.Symbol{Name: "crtstuff.c", Info: 0x4, Other: 0x0, Section: elf.SHN_ABS, Value: 0x0, Size: 0x0},
		elf.Symbol{Name: "__JCR_LIST__", Info: 0x1, Other: 0x0, Section: elf.SHN_UNDEF + 19, Value: 0x600e18, Size: 0x0},
		elf.Symbol{Name: "deregister_tm_clones", Info: 0x2, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x400420, Size: 0x0},
		elf.Symbol{Name: "register_tm_clones", Info: 0x2, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x400460, Size: 0x0},
		elf.Symbol{Name: "__do_global_dtors_aux", Info: 0x2, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x4004a0, Size: 0x0},
		elf.Symbol{Name: "completed.6963", Info: 0x1, Other: 0x0, Section: elf.SHN_UNDEF + 24, Value: 0x6010b0, Size: 0x1},
		elf.Symbol{Name: "__do_global_dtors_aux_fini_array_entry", Info: 0x1, Other: 0x0, Section: elf.SHN_UNDEF + 18, Value: 0x600e10, Size: 0x0},
		elf.Symbol{Name: "frame_dummy", Info: 0x2, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x4004c0, Size: 0x0},
		elf.Symbol{Name: "__frame_dummy_init_array_entry", Info: 0x1, Other: 0x0, Section: elf.SHN_UNDEF + 17, Value: 0x600e08, Size: 0x0},
		elf.Symbol{Name: "test.c", Info: 0x4, Other: 0x0, Section: elf.SHN_ABS, Value: 0x0, Size: 0x0},
		elf.Symbol{Name: "crtstuff.c", Info: 0x4, Other: 0x0, Section: elf.SHN_ABS, Value: 0x0, Size: 0x0},
		elf.Symbol{Name: "__FRAME_END__", Info: 0x1, Other: 0x0, Section: elf.SHN_UNDEF + 16, Value: 0x4006d4, Size: 0x0},
		elf.Symbol{Name: "__JCR_END__", Info: 0x1, Other: 0x0, Section: elf.SHN_UNDEF + 19, Value: 0x600e18, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x4, Other: 0x0, Section: elf.SHN_ABS, Value: 0x0, Size: 0x0},
		elf.Symbol{Name: "__init_array_end", Info: 0x0, Other: 0x0, Section: elf.SHN_UNDEF + 17, Value: 0x600e10, Size: 0x0},
		elf.Symbol{Name: "_DYNAMIC", Info: 0x1, Other: 0x0, Section: elf.SHN_UNDEF + 20, Value: 0x600e20, Size: 0x0},
		elf.Symbol{Name: "__init_array_start", Info: 0x0, Other: 0x0, Section: elf.SHN_UNDEF + 17, Value: 0x600e08, Size: 0x0},
		elf.Symbol{Name: "__GNU_EH_FRAME_HDR", Info: 0x0, Other: 0x0, Section: elf.SHN_UNDEF + 15, Value: 0x4005b4, Size: 0x0},
		elf.Symbol{Name: "_GLOBAL_OFFSET_TABLE_", Info: 0x1, Other: 0x0, Section: elf.SHN_UNDEF + 22, Value: 0x601000, Size: 0x0},
		elf.Symbol{Name: "__libc_csu_fini", Info: 0x12, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x4005a0, Size: 0x2},
		elf.Symbol{Name: "soong_build_number", Info: 0x11, Other: 0x0, Section: elf.SHN_UNDEF + 23, Value: 0x601030, Size: 0x80},
		elf.Symbol{Name: "data_start", Info: 0x20, Other: 0x0, Section: elf.SHN_UNDEF + 23, Value: 0x601020, Size: 0x0},
		elf.Symbol{Name: "write@@GLIBC_2.2.5", Info: 0x12, Other: 0x0, Section: elf.SHN_UNDEF, Value: 0x0, Size: 0x0},
		elf.Symbol{Name: "_edata", Info: 0x10, Other: 0x0, Section: elf.SHN_UNDEF + 23, Value: 0x6010b0, Size: 0x0},
		elf.Symbol{Name: "_fini", Info: 0x12, Other: 0x0, Section: elf.SHN_UNDEF + 13, Value: 0x4005a4, Size: 0x0},
		elf.Symbol{Name: "__libc_start_main@@GLIBC_2.2.5", Info: 0x12, Other: 0x0, Section: elf.SHN_UNDEF, Value: 0x0, Size: 0x0},
		elf.Symbol{Name: "__data_start", Info: 0x10, Other: 0x0, Section: elf.SHN_UNDEF + 23, Value: 0x601020, Size: 0x0},
		elf.Symbol{Name: "__gmon_start__", Info: 0x20, Other: 0x0, Section: elf.SHN_UNDEF, Value: 0x0, Size: 0x0},
		elf.Symbol{Name: "__dso_handle", Info: 0x11, Other: 0x2, Section: elf.SHN_UNDEF + 23, Value: 0x601028, Size: 0x0},
		elf.Symbol{Name: "_IO_stdin_used", Info: 0x11, Other: 0x0, Section: elf.SHN_UNDEF + 14, Value: 0x4005b0, Size: 0x4},
		elf.Symbol{Name: "__libc_csu_init", Info: 0x12, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x400530, Size: 0x65},
		elf.Symbol{Name: "_end", Info: 0x10, Other: 0x0, Section: elf.SHN_UNDEF + 24, Value: 0x6010b8, Size: 0x0},
		elf.Symbol{Name: "_start", Info: 0x12, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x4003f0, Size: 0x2b},
		elf.Symbol{Name: "__bss_start", Info: 0x10, Other: 0x0, Section: elf.SHN_UNDEF + 24, Value: 0x6010b0, Size: 0x0},
		elf.Symbol{Name: "main", Info: 0x12, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x4004f0, Size: 0x31},
		elf.Symbol{Name: "__TMC_END__", Info: 0x11, Other: 0x2, Section: elf.SHN_UNDEF + 23, Value: 0x6010b0, Size: 0x0},
		elf.Symbol{Name: "_init", Info: 0x12, Other: 0x0, Section: elf.SHN_UNDEF + 10, Value: 0x4003b0, Size: 0x0},
	},
}

/* Generated from: clang -o a.out test2.c

#include <unistd.h>

char symbol1[128] = "PLACEHOLDER1";
char symbol2[128] = "PLACEHOLDER2";

int main() {
  write(STDOUT_FILENO, symbol1, sizeof(symbol1));
  write(STDOUT_FILENO, symbol2, sizeof(symbol2));
}

*/

var elfSymbolTable2 = &mockElfFile{
	t: elf.ET_EXEC,
	sections: []elf.SectionHeader{
		elf.SectionHeader{Name: "", Type: elf.SHT_NULL, Flags: 0x0, Addr: 0x0, Offset: 0x0, Size: 0x0, Link: 0x0, Info: 0x0, Addralign: 0x0, Entsize: 0x0, FileSize: 0x0},
		elf.SectionHeader{Name: ".interp", Type: elf.SHT_PROGBITS, Flags: elf.SHF_ALLOC, Addr: 0x400238, Offset: 0x238, Size: 0x1c, Link: 0x0, Info: 0x0, Addralign: 0x1, Entsize: 0x0, FileSize: 0x1c},
		elf.SectionHeader{Name: ".note.ABI-tag", Type: elf.SHT_NOTE, Flags: elf.SHF_ALLOC, Addr: 0x400254, Offset: 0x254, Size: 0x20, Link: 0x0, Info: 0x0, Addralign: 0x4, Entsize: 0x0, FileSize: 0x20},
		elf.SectionHeader{Name: ".hash", Type: elf.SHT_HASH, Flags: elf.SHF_ALLOC, Addr: 0x400278, Offset: 0x278, Size: 0x24, Link: 0x4, Info: 0x0, Addralign: 0x8, Entsize: 0x4, FileSize: 0x24},
		elf.SectionHeader{Name: ".dynsym", Type: elf.SHT_DYNSYM, Flags: elf.SHF_ALLOC, Addr: 0x4002a0, Offset: 0x2a0, Size: 0x60, Link: 0x5, Info: 0x1, Addralign: 0x8, Entsize: 0x18, FileSize: 0x60},
		elf.SectionHeader{Name: ".dynstr", Type: elf.SHT_STRTAB, Flags: elf.SHF_ALLOC, Addr: 0x400300, Offset: 0x300, Size: 0x3e, Link: 0x0, Info: 0x0, Addralign: 0x1, Entsize: 0x0, FileSize: 0x3e},
		elf.SectionHeader{Name: ".gnu.version", Type: elf.SHT_GNU_VERSYM, Flags: elf.SHF_ALLOC, Addr: 0x40033e, Offset: 0x33e, Size: 0x8, Link: 0x4, Info: 0x0, Addralign: 0x2, Entsize: 0x2, FileSize: 0x8},
		elf.SectionHeader{Name: ".gnu.version_r", Type: elf.SHT_GNU_VERNEED, Flags: elf.SHF_ALLOC, Addr: 0x400348, Offset: 0x348, Size: 0x20, Link: 0x5, Info: 0x1, Addralign: 0x8, Entsize: 0x0, FileSize: 0x20},
		elf.SectionHeader{Name: ".rela.dyn", Type: elf.SHT_RELA, Flags: elf.SHF_ALLOC, Addr: 0x400368, Offset: 0x368, Size: 0x30, Link: 0x4, Info: 0x0, Addralign: 0x8, Entsize: 0x18, FileSize: 0x30},
		elf.SectionHeader{Name: ".rela.plt", Type: elf.SHT_RELA, Flags: elf.SHF_ALLOC + elf.SHF_INFO_LINK, Addr: 0x400398, Offset: 0x398, Size: 0x18, Link: 0x4, Info: 0x16, Addralign: 0x8, Entsize: 0x18, FileSize: 0x18},
		elf.SectionHeader{Name: ".init", Type: elf.SHT_PROGBITS, Flags: elf.SHF_ALLOC + elf.SHF_EXECINSTR, Addr: 0x4003b0, Offset: 0x3b0, Size: 0x17, Link: 0x0, Info: 0x0, Addralign: 0x4, Entsize: 0x0, FileSize: 0x17},
		elf.SectionHeader{Name: ".plt", Type: elf.SHT_PROGBITS, Flags: elf.SHF_ALLOC + elf.SHF_EXECINSTR, Addr: 0x4003d0, Offset: 0x3d0, Size: 0x20, Link: 0x0, Info: 0x0, Addralign: 0x10, Entsize: 0x10, FileSize: 0x20},
		elf.SectionHeader{Name: ".text", Type: elf.SHT_PROGBITS, Flags: elf.SHF_ALLOC + elf.SHF_EXECINSTR, Addr: 0x4003f0, Offset: 0x3f0, Size: 0x1c2, Link: 0x0, Info: 0x0, Addralign: 0x10, Entsize: 0x0, FileSize: 0x1c2},
		elf.SectionHeader{Name: ".fini", Type: elf.SHT_PROGBITS, Flags: elf.SHF_ALLOC + elf.SHF_EXECINSTR, Addr: 0x4005b4, Offset: 0x5b4, Size: 0x9, Link: 0x0, Info: 0x0, Addralign: 0x4, Entsize: 0x0, FileSize: 0x9},
		elf.SectionHeader{Name: ".rodata", Type: elf.SHT_PROGBITS, Flags: elf.SHF_ALLOC + elf.SHF_MERGE, Addr: 0x4005c0, Offset: 0x5c0, Size: 0x4, Link: 0x0, Info: 0x0, Addralign: 0x4, Entsize: 0x4, FileSize: 0x4},
		elf.SectionHeader{Name: ".eh_frame_hdr", Type: elf.SHT_PROGBITS, Flags: elf.SHF_ALLOC, Addr: 0x4005c4, Offset: 0x5c4, Size: 0x34, Link: 0x0, Info: 0x0, Addralign: 0x4, Entsize: 0x0, FileSize: 0x34},
		elf.SectionHeader{Name: ".eh_frame", Type: elf.SHT_PROGBITS, Flags: elf.SHF_ALLOC, Addr: 0x4005f8, Offset: 0x5f8, Size: 0xf0, Link: 0x0, Info: 0x0, Addralign: 0x8, Entsize: 0x0, FileSize: 0xf0},
		elf.SectionHeader{Name: ".init_array", Type: elf.SHT_INIT_ARRAY, Flags: elf.SHF_WRITE + elf.SHF_ALLOC, Addr: 0x600e08, Offset: 0xe08, Size: 0x8, Link: 0x0, Info: 0x0, Addralign: 0x8, Entsize: 0x8, FileSize: 0x8},
		elf.SectionHeader{Name: ".fini_array", Type: elf.SHT_FINI_ARRAY, Flags: elf.SHF_WRITE + elf.SHF_ALLOC, Addr: 0x600e10, Offset: 0xe10, Size: 0x8, Link: 0x0, Info: 0x0, Addralign: 0x8, Entsize: 0x8, FileSize: 0x8},
		elf.SectionHeader{Name: ".jcr", Type: elf.SHT_PROGBITS, Flags: elf.SHF_WRITE + elf.SHF_ALLOC, Addr: 0x600e18, Offset: 0xe18, Size: 0x8, Link: 0x0, Info: 0x0, Addralign: 0x8, Entsize: 0x0, FileSize: 0x8},
		elf.SectionHeader{Name: ".dynamic", Type: elf.SHT_DYNAMIC, Flags: elf.SHF_WRITE + elf.SHF_ALLOC, Addr: 0x600e20, Offset: 0xe20, Size: 0x1d0, Link: 0x5, Info: 0x0, Addralign: 0x8, Entsize: 0x10, FileSize: 0x1d0},
		elf.SectionHeader{Name: ".got", Type: elf.SHT_PROGBITS, Flags: elf.SHF_WRITE + elf.SHF_ALLOC, Addr: 0x600ff0, Offset: 0xff0, Size: 0x10, Link: 0x0, Info: 0x0, Addralign: 0x8, Entsize: 0x8, FileSize: 0x10},
		elf.SectionHeader{Name: ".got.plt", Type: elf.SHT_PROGBITS, Flags: elf.SHF_WRITE + elf.SHF_ALLOC, Addr: 0x601000, Offset: 0x1000, Size: 0x20, Link: 0x0, Info: 0x0, Addralign: 0x8, Entsize: 0x8, FileSize: 0x20},
		elf.SectionHeader{Name: ".data", Type: elf.SHT_PROGBITS, Flags: elf.SHF_WRITE + elf.SHF_ALLOC, Addr: 0x601020, Offset: 0x1020, Size: 0x110, Link: 0x0, Info: 0x0, Addralign: 0x10, Entsize: 0x0, FileSize: 0x110},
		elf.SectionHeader{Name: ".bss", Type: elf.SHT_NOBITS, Flags: elf.SHF_WRITE + elf.SHF_ALLOC, Addr: 0x601130, Offset: 0x1130, Size: 0x8, Link: 0x0, Info: 0x0, Addralign: 0x1, Entsize: 0x0, FileSize: 0x8},
		elf.SectionHeader{Name: ".comment", Type: elf.SHT_PROGBITS, Flags: elf.SHF_MERGE + elf.SHF_STRINGS, Addr: 0x0, Offset: 0x1130, Size: 0x56, Link: 0x0, Info: 0x0, Addralign: 0x1, Entsize: 0x1, FileSize: 0x56},
		elf.SectionHeader{Name: ".symtab", Type: elf.SHT_SYMTAB, Flags: 0x0, Addr: 0x0, Offset: 0x1188, Size: 0x600, Link: 0x1b, Info: 0x2d, Addralign: 0x8, Entsize: 0x18, FileSize: 0x600},
		elf.SectionHeader{Name: ".strtab", Type: elf.SHT_STRTAB, Flags: 0x0, Addr: 0x0, Offset: 0x1788, Size: 0x1db, Link: 0x0, Info: 0x0, Addralign: 0x1, Entsize: 0x0, FileSize: 0x1db},
		elf.SectionHeader{Name: ".shstrtab", Type: elf.SHT_STRTAB, Flags: 0x0, Addr: 0x0, Offset: 0x1963, Size: 0xf1, Link: 0x0, Info: 0x0, Addralign: 0x1, Entsize: 0x0, FileSize: 0xf1},
	},
	symbols: []elf.Symbol{
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 1, Value: 0x400238, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 2, Value: 0x400254, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 3, Value: 0x400278, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 4, Value: 0x4002a0, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 5, Value: 0x400300, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 6, Value: 0x40033e, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 7, Value: 0x400348, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 8, Value: 0x400368, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 9, Value: 0x400398, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 10, Value: 0x4003b0, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 11, Value: 0x4003d0, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x4003f0, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 13, Value: 0x4005b4, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 14, Value: 0x4005c0, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 15, Value: 0x4005c4, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 16, Value: 0x4005f8, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 17, Value: 0x600e08, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 18, Value: 0x600e10, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 19, Value: 0x600e18, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 20, Value: 0x600e20, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 21, Value: 0x600ff0, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 22, Value: 0x601000, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 23, Value: 0x601020, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 24, Value: 0x601130, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x3, Other: 0x0, Section: elf.SHN_UNDEF + 25, Value: 0x0, Size: 0x0},
		elf.Symbol{Name: "crtstuff.c", Info: 0x4, Other: 0x0, Section: elf.SHN_ABS, Value: 0x0, Size: 0x0},
		elf.Symbol{Name: "__JCR_LIST__", Info: 0x1, Other: 0x0, Section: elf.SHN_UNDEF + 19, Value: 0x600e18, Size: 0x0},
		elf.Symbol{Name: "deregister_tm_clones", Info: 0x2, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x400420, Size: 0x0},
		elf.Symbol{Name: "register_tm_clones", Info: 0x2, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x400460, Size: 0x0},
		elf.Symbol{Name: "__do_global_dtors_aux", Info: 0x2, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x4004a0, Size: 0x0},
		elf.Symbol{Name: "completed.6963", Info: 0x1, Other: 0x0, Section: elf.SHN_UNDEF + 24, Value: 0x601130, Size: 0x1},
		elf.Symbol{Name: "__do_global_dtors_aux_fini_array_entry", Info: 0x1, Other: 0x0, Section: elf.SHN_UNDEF + 18, Value: 0x600e10, Size: 0x0},
		elf.Symbol{Name: "frame_dummy", Info: 0x2, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x4004c0, Size: 0x0},
		elf.Symbol{Name: "__frame_dummy_init_array_entry", Info: 0x1, Other: 0x0, Section: elf.SHN_UNDEF + 17, Value: 0x600e08, Size: 0x0},
		elf.Symbol{Name: "test2.c", Info: 0x4, Other: 0x0, Section: elf.SHN_ABS, Value: 0x0, Size: 0x0},
		elf.Symbol{Name: "crtstuff.c", Info: 0x4, Other: 0x0, Section: elf.SHN_ABS, Value: 0x0, Size: 0x0},
		elf.Symbol{Name: "__FRAME_END__", Info: 0x1, Other: 0x0, Section: elf.SHN_UNDEF + 16, Value: 0x4006e4, Size: 0x0},
		elf.Symbol{Name: "__JCR_END__", Info: 0x1, Other: 0x0, Section: elf.SHN_UNDEF + 19, Value: 0x600e18, Size: 0x0},
		elf.Symbol{Name: "", Info: 0x4, Other: 0x0, Section: elf.SHN_ABS, Value: 0x0, Size: 0x0},
		elf.Symbol{Name: "__init_array_end", Info: 0x0, Other: 0x0, Section: elf.SHN_UNDEF + 17, Value: 0x600e10, Size: 0x0},
		elf.Symbol{Name: "_DYNAMIC", Info: 0x1, Other: 0x0, Section: elf.SHN_UNDEF + 20, Value: 0x600e20, Size: 0x0},
		elf.Symbol{Name: "__init_array_start", Info: 0x0, Other: 0x0, Section: elf.SHN_UNDEF + 17, Value: 0x600e08, Size: 0x0},
		elf.Symbol{Name: "__GNU_EH_FRAME_HDR", Info: 0x0, Other: 0x0, Section: elf.SHN_UNDEF + 15, Value: 0x4005c4, Size: 0x0},
		elf.Symbol{Name: "_GLOBAL_OFFSET_TABLE_", Info: 0x1, Other: 0x0, Section: elf.SHN_UNDEF + 22, Value: 0x601000, Size: 0x0},
		elf.Symbol{Name: "__libc_csu_fini", Info: 0x12, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x4005b0, Size: 0x2},
		elf.Symbol{Name: "symbol1", Info: 0x11, Other: 0x0, Section: elf.SHN_UNDEF + 23, Value: 0x601030, Size: 0x80},
		elf.Symbol{Name: "data_start", Info: 0x20, Other: 0x0, Section: elf.SHN_UNDEF + 23, Value: 0x601020, Size: 0x0},
		elf.Symbol{Name: "write@@GLIBC_2.2.5", Info: 0x12, Other: 0x0, Section: elf.SHN_UNDEF, Value: 0x0, Size: 0x0},
		elf.Symbol{Name: "_edata", Info: 0x10, Other: 0x0, Section: elf.SHN_UNDEF + 23, Value: 0x601130, Size: 0x0},
		elf.Symbol{Name: "_fini", Info: 0x12, Other: 0x0, Section: elf.SHN_UNDEF + 13, Value: 0x4005b4, Size: 0x0},
		elf.Symbol{Name: "__libc_start_main@@GLIBC_2.2.5", Info: 0x12, Other: 0x0, Section: elf.SHN_UNDEF, Value: 0x0, Size: 0x0},
		elf.Symbol{Name: "__data_start", Info: 0x10, Other: 0x0, Section: elf.SHN_UNDEF + 23, Value: 0x601020, Size: 0x0},
		elf.Symbol{Name: "__gmon_start__", Info: 0x20, Other: 0x0, Section: elf.SHN_UNDEF, Value: 0x0, Size: 0x0},
		elf.Symbol{Name: "__dso_handle", Info: 0x11, Other: 0x2, Section: elf.SHN_UNDEF + 23, Value: 0x601028, Size: 0x0},
		elf.Symbol{Name: "_IO_stdin_used", Info: 0x11, Other: 0x0, Section: elf.SHN_UNDEF + 14, Value: 0x4005c0, Size: 0x4},
		elf.Symbol{Name: "symbol2", Info: 0x11, Other: 0x0, Section: elf.SHN_UNDEF + 23, Value: 0x6010b0, Size: 0x80},
		elf.Symbol{Name: "__libc_csu_init", Info: 0x12, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x400540, Size: 0x65},
		elf.Symbol{Name: "_end", Info: 0x10, Other: 0x0, Section: elf.SHN_UNDEF + 24, Value: 0x601138, Size: 0x0},
		elf.Symbol{Name: "_start", Info: 0x12, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x4003f0, Size: 0x2b},
		elf.Symbol{Name: "__bss_start", Info: 0x10, Other: 0x0, Section: elf.SHN_UNDEF + 24, Value: 0x601130, Size: 0x0},
		elf.Symbol{Name: "main", Info: 0x12, Other: 0x0, Section: elf.SHN_UNDEF + 12, Value: 0x4004f0, Size: 0x50},
		elf.Symbol{Name: "__TMC_END__", Info: 0x11, Other: 0x2, Section: elf.SHN_UNDEF + 23, Value: 0x601130, Size: 0x0},
		elf.Symbol{Name: "_init", Info: 0x12, Other: 0x0, Section: elf.SHN_UNDEF + 10, Value: 0x4003b0, Size: 0x0},
	},
}
