// Copyright 2017 Google Inc. All rights reserved.
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

package parser

import (
	"strings"
	"testing"
)

var splitNTestCases = []struct {
	in       *MakeString
	expected []*MakeString
	sep      string
	n        int
}{
	{
		// "a b c$(var1)d e f$(var2) h i j"
		in:  genMakeString("a b c", "var1", "d e f", "var2", " h i j"),
		sep: " ",
		n:   -1,
		expected: []*MakeString{
			genMakeString("a"),
			genMakeString("b"),
			genMakeString("c", "var1", "d"),
			genMakeString("e"),
			genMakeString("f", "var2", ""),
			genMakeString("h"),
			genMakeString("i"),
			genMakeString("j"),
		},
	},
	{
		// "a b c$(var1)d e f$(var2) h i j"
		in:  genMakeString("a b c", "var1", "d e f", "var2", " h i j"),
		sep: " ",
		n:   3,
		expected: []*MakeString{
			genMakeString("a"),
			genMakeString("b"),
			genMakeString("c", "var1", "d e f", "var2", " h i j"),
		},
	},
	{
		// "$(var1) $(var2)"
		in:  genMakeString("", "var1", " ", "var2", ""),
		sep: " ",
		n:   -1,
		expected: []*MakeString{
			genMakeString("", "var1", ""),
			genMakeString("", "var2", ""),
		},
	},
	{
		// "a,,b,c,"
		in:  genMakeString("a,,b,c,"),
		sep: ",",
		n:   -1,
		expected: []*MakeString{
			genMakeString("a"),
			genMakeString(""),
			genMakeString("b"),
			genMakeString("c"),
			genMakeString(""),
		},
	},
}

func TestMakeStringSplitN(t *testing.T) {
	for _, test := range splitNTestCases {
		got := test.in.SplitN(test.sep, test.n)
		gotString := dumpArray(got)
		expectedString := dumpArray(test.expected)
		if gotString != expectedString {
			t.Errorf("expected:\n%s\ngot:\n%s", expectedString, gotString)
		}
	}
}

var valueTestCases = []struct {
	in       *MakeString
	expected string
}{
	{
		in:       genMakeString("a b"),
		expected: "a b",
	},
	{
		in:       genMakeString("a\\ \\\tb\\\\"),
		expected: "a \tb\\",
	},
	{
		in:       genMakeString("a\\b\\"),
		expected: "a\\b\\",
	},
}

func TestMakeStringValue(t *testing.T) {
	for _, test := range valueTestCases {
		got := test.in.Value(nil)
		if got != test.expected {
			t.Errorf("\nwith: %q\nwant: %q\n got: %q", test.in.Dump(), test.expected, got)
		}
	}
}

var splitWordsTestCases = []struct {
	in       *MakeString
	expected []*MakeString
}{
	{
		in:       genMakeString(""),
		expected: []*MakeString{},
	},
	{
		in: genMakeString(` a b\ c d`),
		expected: []*MakeString{
			genMakeString("a"),
			genMakeString(`b\ c`),
			genMakeString("d"),
		},
	},
	{
		in: SimpleMakeString("  a\tb"+`\`+"\t"+`\ c d  `, NoPos),
		expected: []*MakeString{
			genMakeString("a"),
			genMakeString("b" + `\` + "\t" + `\ c`),
			genMakeString("d"),
		},
	},
	{
		in: genMakeString(`a\\ b\\\ c d`),
		expected: []*MakeString{
			genMakeString(`a\\`),
			genMakeString(`b\\\ c`),
			genMakeString("d"),
		},
	},
	{
		in: genMakeString(`\\ a`),
		expected: []*MakeString{
			genMakeString(`\\`),
			genMakeString("a"),
		},
	},
	{
		// "  "
		in: &MakeString{
			Strings:   []string{" \t \t"},
			Variables: nil,
		},
		expected: []*MakeString{},
	},
	{
		// " a $(X)b c "
		in: genMakeString(" a ", "X", "b c "),
		expected: []*MakeString{
			genMakeString("a"),
			genMakeString("", "X", "b"),
			genMakeString("c"),
		},
	},
	{
		// " a b$(X)c d"
		in: genMakeString(" a b", "X", "c d"),
		expected: []*MakeString{
			genMakeString("a"),
			genMakeString("b", "X", "c"),
			genMakeString("d"),
		},
	},
	{
		// "$(X) $(Y)"
		in: genMakeString("", "X", " ", "Y", ""),
		expected: []*MakeString{
			genMakeString("", "X", ""),
			genMakeString("", "Y", ""),
		},
	},
	{
		// " a$(X) b"
		in: genMakeString(" a", "X", " b"),
		expected: []*MakeString{
			genMakeString("a", "X", ""),
			genMakeString("b"),
		},
	},
	{
		// "a$(X) b$(Y) "
		in: genMakeString("a", "X", " b", "Y", " "),
		expected: []*MakeString{
			genMakeString("a", "X", ""),
			genMakeString("b", "Y", ""),
		},
	},
}

func TestMakeStringWords(t *testing.T) {
	for _, test := range splitWordsTestCases {
		got := test.in.Words()
		gotString := dumpArray(got)
		expectedString := dumpArray(test.expected)
		if gotString != expectedString {
			t.Errorf("with:\n%q\nexpected:\n%s\ngot:\n%s", test.in.Dump(), expectedString, gotString)
		}
	}
}

func dumpArray(a []*MakeString) string {
	ret := make([]string, len(a))

	for i, s := range a {
		ret[i] = s.Dump()
	}

	return strings.Join(ret, "|||")
}

// generates MakeString from alternating string chunks and variable names,
// e.g., genMakeString("a", "X", "b") returns MakeString for "a$(X)b"
func genMakeString(items ...string) *MakeString {
	n := len(items) / 2
	if len(items) != (2*n + 1) {
		panic("genMakeString expects odd number of arguments")
	}

	ms := &MakeString{Strings: make([]string, n+1), Variables: make([]Variable, n)}
	ms.Strings[0] = items[0]
	for i := 1; i <= n; i++ {
		ms.Variables[i-1] = Variable{Name: SimpleMakeString(items[2*i-1], NoPos)}
		ms.Strings[i] = items[2*i]
	}
	return ms
}
