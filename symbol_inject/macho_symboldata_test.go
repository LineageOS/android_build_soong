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
	"debug/macho"
)

/* Generated from: clang -o a.out test.c

#include <unistd.h>

char soong_build_number[128] = "PLACEHOLDER";

int main() {
  write(STDOUT_FILENO, soong_build_number, sizeof(soong_build_number));
}
*/

var machoSymbolTable1 = &macho.File{
	Sections: []*macho.Section{
		&macho.Section{SectionHeader: macho.SectionHeader{Name: "__text", Seg: "__TEXT", Addr: 0x100000f50, Size: 0x2e, Offset: 0xf50, Align: 0x4, Reloff: 0x0, Nreloc: 0x0, Flags: 0x80000400}},
		&macho.Section{SectionHeader: macho.SectionHeader{Name: "__stubs", Seg: "__TEXT", Addr: 0x100000f7e, Size: 0x6, Offset: 0xf7e, Align: 0x1, Reloff: 0x0, Nreloc: 0x0, Flags: 0x80000408}},
		&macho.Section{SectionHeader: macho.SectionHeader{Name: "__stub_helper", Seg: "__TEXT", Addr: 0x100000f84, Size: 0x1a, Offset: 0xf84, Align: 0x2, Reloff: 0x0, Nreloc: 0x0, Flags: 0x80000400}},
		&macho.Section{SectionHeader: macho.SectionHeader{Name: "__unwind_info", Seg: "__TEXT", Addr: 0x100000fa0, Size: 0x48, Offset: 0xfa0, Align: 0x2, Reloff: 0x0, Nreloc: 0x0, Flags: 0x0}},
		&macho.Section{SectionHeader: macho.SectionHeader{Name: "__eh_frame", Seg: "__TEXT", Addr: 0x100000fe8, Size: 0x18, Offset: 0xfe8, Align: 0x3, Reloff: 0x0, Nreloc: 0x0, Flags: 0x0}},
		&macho.Section{SectionHeader: macho.SectionHeader{Name: "__nl_symbol_ptr", Seg: "__DATA", Addr: 0x100001000, Size: 0x10, Offset: 0x1000, Align: 0x3, Reloff: 0x0, Nreloc: 0x0, Flags: 0x6}},
		&macho.Section{SectionHeader: macho.SectionHeader{Name: "__la_symbol_ptr", Seg: "__DATA", Addr: 0x100001010, Size: 0x8, Offset: 0x1010, Align: 0x3, Reloff: 0x0, Nreloc: 0x0, Flags: 0x7}},
		&macho.Section{SectionHeader: macho.SectionHeader{Name: "__data", Seg: "__DATA", Addr: 0x100001020, Size: 0x80, Offset: 0x1020, Align: 0x4, Reloff: 0x0, Nreloc: 0x0, Flags: 0x0}},
	},
	Symtab: &macho.Symtab{
		Syms: []macho.Symbol{
			macho.Symbol{Name: "__mh_execute_header", Type: 0xf, Sect: 0x1, Desc: 0x10, Value: 0x100000000},
			macho.Symbol{Name: "_main", Type: 0xf, Sect: 0x1, Desc: 0x0, Value: 0x100000f50},
			macho.Symbol{Name: "_soong_build_number", Type: 0xf, Sect: 0x8, Desc: 0x0, Value: 0x100001020},
			macho.Symbol{Name: "_write", Type: 0x1, Sect: 0x0, Desc: 0x100, Value: 0x0},
			macho.Symbol{Name: "dyld_stub_binder", Type: 0x1, Sect: 0x0, Desc: 0x100, Value: 0x0},
		},
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

var machoSymbolTable2 = &macho.File{
	Sections: []*macho.Section{
		&macho.Section{SectionHeader: macho.SectionHeader{Name: "__text", Seg: "__TEXT", Addr: 0x100000f30, Size: 0x4a, Offset: 0xf30, Align: 0x4, Reloff: 0x0, Nreloc: 0x0, Flags: 0x80000400}},
		&macho.Section{SectionHeader: macho.SectionHeader{Name: "__stubs", Seg: "__TEXT", Addr: 0x100000f7a, Size: 0x6, Offset: 0xf7a, Align: 0x1, Reloff: 0x0, Nreloc: 0x0, Flags: 0x80000408}},
		&macho.Section{SectionHeader: macho.SectionHeader{Name: "__stub_helper", Seg: "__TEXT", Addr: 0x100000f80, Size: 0x1a, Offset: 0xf80, Align: 0x2, Reloff: 0x0, Nreloc: 0x0, Flags: 0x80000400}},
		&macho.Section{SectionHeader: macho.SectionHeader{Name: "__unwind_info", Seg: "__TEXT", Addr: 0x100000f9c, Size: 0x48, Offset: 0xf9c, Align: 0x2, Reloff: 0x0, Nreloc: 0x0, Flags: 0x0}},
		&macho.Section{SectionHeader: macho.SectionHeader{Name: "__eh_frame", Seg: "__TEXT", Addr: 0x100000fe8, Size: 0x18, Offset: 0xfe8, Align: 0x3, Reloff: 0x0, Nreloc: 0x0, Flags: 0x0}},
		&macho.Section{SectionHeader: macho.SectionHeader{Name: "__nl_symbol_ptr", Seg: "__DATA", Addr: 0x100001000, Size: 0x10, Offset: 0x1000, Align: 0x3, Reloff: 0x0, Nreloc: 0x0, Flags: 0x6}},
		&macho.Section{SectionHeader: macho.SectionHeader{Name: "__la_symbol_ptr", Seg: "__DATA", Addr: 0x100001010, Size: 0x8, Offset: 0x1010, Align: 0x3, Reloff: 0x0, Nreloc: 0x0, Flags: 0x7}},
		&macho.Section{SectionHeader: macho.SectionHeader{Name: "__data", Seg: "__DATA", Addr: 0x100001020, Size: 0x100, Offset: 0x1020, Align: 0x4, Reloff: 0x0, Nreloc: 0x0, Flags: 0x0}},
	},
	Symtab: &macho.Symtab{
		Syms: []macho.Symbol{
			macho.Symbol{Name: "__mh_execute_header", Type: 0xf, Sect: 0x1, Desc: 0x10, Value: 0x100000000},
			macho.Symbol{Name: "_main", Type: 0xf, Sect: 0x1, Desc: 0x0, Value: 0x100000f30},
			macho.Symbol{Name: "_symbol1", Type: 0xf, Sect: 0x8, Desc: 0x0, Value: 0x100001020},
			macho.Symbol{Name: "_symbol2", Type: 0xf, Sect: 0x8, Desc: 0x0, Value: 0x1000010a0},
			macho.Symbol{Name: "_write", Type: 0x1, Sect: 0x0, Desc: 0x100, Value: 0x0},
			macho.Symbol{Name: "dyld_stub_binder", Type: 0x1, Sect: 0x0, Desc: 0x100, Value: 0x0},
		},
	},
}
