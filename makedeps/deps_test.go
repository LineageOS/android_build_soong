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

package makedeps

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"testing"
)

func TestParse(t *testing.T) {
	testCases := []struct {
		name   string
		input  string
		output Deps
		err    error
	}{
		// These come from the ninja test suite
		{
			name:  "Basic",
			input: "build/ninja.o: ninja.cc ninja.h eval_env.h manifest_parser.h",
			output: Deps{
				Output: "build/ninja.o",
				Inputs: []string{
					"ninja.cc",
					"ninja.h",
					"eval_env.h",
					"manifest_parser.h",
				},
			},
		},
		{
			name: "EarlyNewlineAndWhitespace",
			input: ` \
  out: in`,
			output: Deps{
				Output: "out",
				Inputs: []string{"in"},
			},
		},
		{
			name: "Continuation",
			input: `foo.o: \
  bar.h baz.h
`,
			output: Deps{
				Output: "foo.o",
				Inputs: []string{"bar.h", "baz.h"},
			},
		},
		{
			name:  "CarriageReturnContinuation",
			input: "foo.o: \\\r\n  bar.h baz.h\r\n",
			output: Deps{
				Output: "foo.o",
				Inputs: []string{"bar.h", "baz.h"},
			},
		},
		{
			name: "BackSlashes",
			input: `Project\Dir\Build\Release8\Foo\Foo.res : \
  Dir\Library\Foo.rc \
  Dir\Library\Version\Bar.h \
  Dir\Library\Foo.ico \
  Project\Thing\Bar.tlb \
`,
			output: Deps{
				Output: `Project\Dir\Build\Release8\Foo\Foo.res`,
				Inputs: []string{
					`Dir\Library\Foo.rc`,
					`Dir\Library\Version\Bar.h`,
					`Dir\Library\Foo.ico`,
					`Project\Thing\Bar.tlb`,
				},
			},
		},
		{
			name:  "Spaces",
			input: `a\ bc\ def:   a\ b c d`,
			output: Deps{
				Output: `a bc def`,
				Inputs: []string{"a b", "c", "d"},
			},
		},
		{
			name:  "Escapes",
			input: `\!\@\#$$\%\^\&\\:`,
			output: Deps{
				Output: `\!\@#$\%\^\&\`,
			},
		},
		{
			name: "SpecialChars",
			// Ninja includes a number of '=', but our parser can't handle that,
			// since it sees the equals and switches over to assuming it's an
			// assignment.
			//
			// We don't have any files in our tree that contain an '=' character,
			// and Kati can't handle parsing this either, so for now I'm just
			// going to remove all the '=' characters below.
			//
			// It looks like make will only do this for the first
			// dependency, but not later dependencies.
			input: `C\:/Program\ Files\ (x86)/Microsoft\ crtdefs.h: \
 en@quot.header~ t+t-x!1 \
 openldap/slapd.d/cnconfig/cnschema/cn{0}core.ldif \
 Fu` + "\303\244ball",
			output: Deps{
				Output: "C:/Program Files (x86)/Microsoft crtdefs.h",
				Inputs: []string{
					"en@quot.header~",
					"t+t-x!1",
					"openldap/slapd.d/cnconfig/cnschema/cn{0}core.ldif",
					"Fu\303\244ball",
				},
			},
		},
		// Ninja's UnifyMultipleOutputs and RejectMultipleDifferentOutputs tests have been omitted,
		// since we don't want the same behavior.

		// Our own tests
		{
			name: "Multiple outputs",
			input: `a b: c
a: d
b: e`,
			output: Deps{
				Output: "b",
				Inputs: []string{
					"c",
					"d",
					"e",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Parse("test.d", bytes.NewBufferString(tc.input))
			if err != tc.err {
				t.Fatalf("Unexpected error: %v (expected %v)", err, tc.err)
			}

			if out.Output != tc.output.Output {
				t.Errorf("output file doesn't match:\n"+
					" str: %#v\n"+
					"want: %#v\n"+
					" got: %#v", tc.input, tc.output.Output, out.Output)
			}

			matches := true
			if len(out.Inputs) != len(tc.output.Inputs) {
				matches = false
			} else {
				for i := range out.Inputs {
					if out.Inputs[i] != tc.output.Inputs[i] {
						matches = false
					}
				}
			}
			if !matches {
				t.Errorf("input files don't match:\n"+
					" str: %#v\n"+
					"want: %#v\n"+
					" got: %#v", tc.input, tc.output.Inputs, out.Inputs)
			}
		})
	}
}

func BenchmarkParsing(b *testing.B) {
	// Write it out to a file to most closely match ninja's perftest
	tmpfile, err := ioutil.TempFile("", "depfile")
	if err != nil {
		b.Fatal("Failed to create temp file:", err)
	}
	defer os.Remove(tmpfile.Name())
	_, err = io.WriteString(tmpfile, `out/soong/.intermediates/external/ninja/ninja/linux_glibc_x86_64/obj/external/ninja/src/ninja.o: \
  external/ninja/src/ninja.cc external/libcxx/include/errno.h \
  external/libcxx/include/__config \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/features.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/predefs.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/sys/cdefs.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/wordsize.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/gnu/stubs.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/gnu/stubs-64.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/errno.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/errno.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/linux/errno.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/asm/errno.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/asm-generic/errno.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/asm-generic/errno-base.h \
  external/libcxx/include/limits.h \
  prebuilts/clang/host/linux-x86/clang-4639204/lib64/clang/6.0.1/include/limits.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/limits.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/posix1_lim.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/local_lim.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/linux/limits.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/posix2_lim.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/xopen_lim.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/stdio_lim.h \
  external/libcxx/include/stdio.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/stdio.h \
  external/libcxx/include/stddef.h \
  prebuilts/clang/host/linux-x86/clang-4639204/lib64/clang/6.0.1/include/stddef.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/types.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/typesizes.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/libio.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/_G_config.h \
  external/libcxx/include/wchar.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/wchar.h \
  prebuilts/clang/host/linux-x86/clang-4639204/lib64/clang/6.0.1/include/stdarg.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/sys_errlist.h \
  external/libcxx/include/stdlib.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/stdlib.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/waitflags.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/waitstatus.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/endian.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/endian.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/byteswap.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/xlocale.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/sys/types.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/time.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/sys/select.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/select.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/sigset.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/time.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/select2.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/sys/sysmacros.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/pthreadtypes.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/alloca.h \
  external/libcxx/include/string.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/string.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/getopt.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/unistd.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/posix_opt.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/environments.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/confname.h \
  external/ninja/src/browse.h external/ninja/src/build.h \
  external/libcxx/include/cstdio external/libcxx/include/map \
  external/libcxx/include/__tree external/libcxx/include/iterator \
  external/libcxx/include/iosfwd \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/wchar.h \
  external/libcxx/include/__functional_base \
  external/libcxx/include/type_traits external/libcxx/include/cstddef \
  prebuilts/clang/host/linux-x86/clang-4639204/lib64/clang/6.0.1/include/__stddef_max_align_t.h \
  external/libcxx/include/__nullptr external/libcxx/include/typeinfo \
  external/libcxx/include/exception external/libcxx/include/cstdlib \
  external/libcxx/include/cstdint external/libcxx/include/stdint.h \
  prebuilts/clang/host/linux-x86/clang-4639204/lib64/clang/6.0.1/include/stdint.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/stdint.h \
  external/libcxx/include/new external/libcxx/include/utility \
  external/libcxx/include/__tuple \
  external/libcxx/include/initializer_list \
  external/libcxx/include/cstring external/libcxx/include/__debug \
  external/libcxx/include/memory external/libcxx/include/limits \
  external/libcxx/include/__undef_macros external/libcxx/include/tuple \
  external/libcxx/include/stdexcept external/libcxx/include/cassert \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/assert.h \
  external/libcxx/include/atomic external/libcxx/include/algorithm \
  external/libcxx/include/functional external/libcxx/include/queue \
  external/libcxx/include/deque external/libcxx/include/__split_buffer \
  external/libcxx/include/vector external/libcxx/include/__bit_reference \
  external/libcxx/include/climits external/libcxx/include/set \
  external/libcxx/include/string external/libcxx/include/string_view \
  external/libcxx/include/__string external/libcxx/include/cwchar \
  external/libcxx/include/cwctype external/libcxx/include/cctype \
  external/libcxx/include/ctype.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/ctype.h \
  external/libcxx/include/wctype.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/wctype.h \
  external/ninja/src/graph.h external/ninja/src/eval_env.h \
  external/ninja/src/string_piece.h external/ninja/src/timestamp.h \
  external/ninja/src/util.h external/ninja/src/exit_status.h \
  external/ninja/src/line_printer.h external/ninja/src/metrics.h \
  external/ninja/src/build_log.h external/ninja/src/hash_map.h \
  external/libcxx/include/unordered_map \
  external/libcxx/include/__hash_table external/libcxx/include/cmath \
  external/libcxx/include/math.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/math.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/huge_val.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/huge_valf.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/huge_vall.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/inf.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/nan.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/mathdef.h \
  prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/sysroot/usr/include/x86_64-linux-gnu/bits/mathcalls.h \
  external/ninja/src/deps_log.h external/ninja/src/clean.h \
  external/ninja/src/debug_flags.h external/ninja/src/disk_interface.h \
  external/ninja/src/graphviz.h external/ninja/src/manifest_parser.h \
  external/ninja/src/lexer.h external/ninja/src/state.h \
  external/ninja/src/version.h`)
	tmpfile.Close()
	if err != nil {
		b.Fatal("Failed to write dep file:", err)
	}
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		depfile, err := ioutil.ReadFile(tmpfile.Name())
		if err != nil {
			b.Fatal("Failed to read dep file:", err)
		}

		_, err = Parse(tmpfile.Name(), bytes.NewBuffer(depfile))
		if err != nil {
			b.Fatal("Failed to parse:", err)
		}
	}
}

func TestDepPrint(t *testing.T) {
	testCases := []struct {
		name   string
		input  Deps
		output string
	}{
		{
			name: "Empty",
			input: Deps{
				Output: "a",
			},
			output: "a:",
		},
		{
			name: "Basic",
			input: Deps{
				Output: "a",
				Inputs: []string{"b", "c"},
			},
			output: "a: b c",
		},
		{
			name: "Escapes",
			input: Deps{
				Output: `\!\@#$\%\^\&\`,
			},
			output: `\\!\\@\#$$\\%\\^\\&\\:`,
		},
		{
			name: "Spaces",
			input: Deps{
				Output: "a b",
				Inputs: []string{"c d", "e f "},
			},
			output: `a\ b: c\ d e\ f\ `,
		},
		{
			name: "SpecialChars",
			input: Deps{
				Output: "C:/Program Files (x86)/Microsoft crtdefs.h",
				Inputs: []string{
					"en@quot.header~",
					"t+t-x!1",
					"openldap/slapd.d/cnconfig/cnschema/cn{0}core.ldif",
					"Fu\303\244ball",
				},
			},
			output: `C\:/Program\ Files\ (x86)/Microsoft\ crtdefs.h: en@quot.header~ t+t-x!1 openldap/slapd.d/cnconfig/cnschema/cn{0}core.ldif Fu` + "\303\244ball",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out := tc.input.Print()
			outStr := string(out)
			want := tc.output + "\n"

			if outStr != want {
				t.Errorf("output doesn't match:\nwant:%q\n got:%q", want, outStr)
			}
		})
	}
}
