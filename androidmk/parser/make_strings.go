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
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A MakeString is a string that may contain variable substitutions in it.
// It can be considered as an alternating list of raw Strings and variable
// substitutions, where the first and last entries in the list must be raw
// Strings (possibly empty).  A MakeString that starts with a variable
// will have an empty first raw string, and a MakeString that ends with a
// variable will have an empty last raw string.  Two sequential Variables
// will have an empty raw string between them.
//
// The MakeString is stored as two lists, a list of raw Strings and a list
// of Variables.  The raw string list is always one longer than the variable
// list.
type MakeString struct {
	StringPos Pos
	Strings   []string
	Variables []Variable
}

func SimpleMakeString(s string, pos Pos) *MakeString {
	return &MakeString{
		StringPos: pos,
		Strings:   []string{s},
	}
}

func (ms *MakeString) Clone() (result *MakeString) {
	clone := *ms
	return &clone
}

func (ms *MakeString) Pos() Pos {
	return ms.StringPos
}

func (ms *MakeString) End() Pos {
	pos := ms.StringPos
	if len(ms.Strings) > 1 {
		pos = ms.Variables[len(ms.Variables)-1].End()
	}
	return Pos(int(pos) + len(ms.Strings[len(ms.Strings)-1]))
}

func (ms *MakeString) appendString(s string) {
	if len(ms.Strings) == 0 {
		ms.Strings = []string{s}
		return
	} else {
		ms.Strings[len(ms.Strings)-1] += s
	}
}

func (ms *MakeString) appendVariable(v Variable) {
	if len(ms.Strings) == 0 {
		ms.Strings = []string{"", ""}
		ms.Variables = []Variable{v}
	} else {
		ms.Strings = append(ms.Strings, "")
		ms.Variables = append(ms.Variables, v)
	}
}

func (ms *MakeString) appendMakeString(other *MakeString) {
	last := len(ms.Strings) - 1
	ms.Strings[last] += other.Strings[0]
	ms.Strings = append(ms.Strings, other.Strings[1:]...)
	ms.Variables = append(ms.Variables, other.Variables...)
}

func (ms *MakeString) Value(scope Scope) string {
	if len(ms.Strings) == 0 {
		return ""
	} else {
		ret := unescape(ms.Strings[0])
		for i := range ms.Strings[1:] {
			ret += ms.Variables[i].Value(scope)
			ret += unescape(ms.Strings[i+1])
		}
		return ret
	}
}

func (ms *MakeString) Dump() string {
	if len(ms.Strings) == 0 {
		return ""
	} else {
		ret := ms.Strings[0]
		for i := range ms.Strings[1:] {
			ret += ms.Variables[i].Dump()
			ret += ms.Strings[i+1]
		}
		return ret
	}
}

func (ms *MakeString) Const() bool {
	return len(ms.Strings) <= 1
}

func (ms *MakeString) Empty() bool {
	return len(ms.Strings) == 0 || (len(ms.Strings) == 1 && ms.Strings[0] == "")
}

func (ms *MakeString) Split(sep string) []*MakeString {
	return ms.SplitN(sep, -1)
}

func (ms *MakeString) SplitN(sep string, n int) []*MakeString {
	return ms.splitNFunc(n, func(s string, n int) []string {
		return splitAnyN(s, sep, n)
	})
}

// Words splits MakeString into multiple makeStrings separated by whitespace.
// Thus, " a $(X)b  c " will be split into ["a", "$(X)b", "c"].
// Splitting a MakeString consisting solely of whitespace yields empty array.
func (ms *MakeString) Words() []*MakeString {
	var ch rune    // current character
	const EOF = -1 // no more characters
	const EOS = -2 // at the end of a string chunk

	// Next character's chunk and position
	iString := 0
	iChar := 0

	var words []*MakeString
	word := SimpleMakeString("", ms.Pos())

	nextChar := func() {
		if iString >= len(ms.Strings) {
			ch = EOF
		} else if iChar >= len(ms.Strings[iString]) {
			iString++
			iChar = 0
			ch = EOS
		} else {
			var w int
			ch, w = utf8.DecodeRuneInString(ms.Strings[iString][iChar:])
			iChar += w
		}
	}

	appendVariableAndAdvance := func() {
		if iString-1 < len(ms.Variables) {
			word.appendVariable(ms.Variables[iString-1])
		}
		nextChar()
	}

	appendCharAndAdvance := func(c rune) {
		if c != EOF {
			word.appendString(string(c))
		}
		nextChar()
	}

	nextChar()
	for ch != EOF {
		// Skip whitespace
		for ch == ' ' || ch == '\t' {
			nextChar()
		}
		if ch == EOS {
			// "... $(X)... " case. The current word should be empty.
			if !word.Empty() {
				panic(fmt.Errorf("%q: EOS while current word %q is not empty, iString=%d",
					ms.Dump(), word.Dump(), iString))
			}
			appendVariableAndAdvance()
		}
		// Copy word
		for ch != EOF {
			if ch == ' ' || ch == '\t' {
				words = append(words, word)
				word = SimpleMakeString("", ms.Pos())
				break
			}
			if ch == EOS {
				// "...a$(X)..." case. Append variable to the current word
				appendVariableAndAdvance()
			} else {
				if ch == '\\' {
					appendCharAndAdvance('\\')
				}
				appendCharAndAdvance(ch)
			}
		}
	}
	if !word.Empty() {
		words = append(words, word)
	}
	return words
}

func (ms *MakeString) splitNFunc(n int, splitFunc func(s string, n int) []string) []*MakeString {
	ret := []*MakeString{}

	curMs := SimpleMakeString("", ms.Pos())

	var i int
	var s string
	for i, s = range ms.Strings {
		if n != 0 {
			split := splitFunc(s, n)
			if n != -1 {
				if len(split) > n {
					panic("oops!")
				} else {
					n -= len(split)
				}
			}
			curMs.appendString(split[0])

			for _, r := range split[1:] {
				ret = append(ret, curMs)
				curMs = SimpleMakeString(r, ms.Pos())
			}
		} else {
			curMs.appendString(s)
		}

		if i < len(ms.Strings)-1 {
			curMs.appendVariable(ms.Variables[i])
		}
	}

	ret = append(ret, curMs)
	return ret
}

func (ms *MakeString) TrimLeftSpaces() {
	l := len(ms.Strings[0])
	ms.Strings[0] = strings.TrimLeftFunc(ms.Strings[0], unicode.IsSpace)
	ms.StringPos += Pos(len(ms.Strings[0]) - l)
}

func (ms *MakeString) TrimRightSpaces() {
	last := len(ms.Strings) - 1
	ms.Strings[last] = strings.TrimRightFunc(ms.Strings[last], unicode.IsSpace)
}

func (ms *MakeString) TrimRightOne() {
	last := len(ms.Strings) - 1
	if len(ms.Strings[last]) > 1 {
		ms.Strings[last] = ms.Strings[last][0 : len(ms.Strings[last])-1]
	}
}

func (ms *MakeString) EndsWith(ch rune) bool {
	s := ms.Strings[len(ms.Strings)-1]
	return s[len(s)-1] == uint8(ch)
}

func (ms *MakeString) ReplaceLiteral(input string, output string) {
	for i := range ms.Strings {
		ms.Strings[i] = strings.Replace(ms.Strings[i], input, output, -1)
	}
}

func splitAnyN(s, sep string, n int) []string {
	ret := []string{}
	for n == -1 || n > 1 {
		index := strings.IndexAny(s, sep)
		if index >= 0 {
			ret = append(ret, s[0:index])
			s = s[index+1:]
			if n > 0 {
				n--
			}
		} else {
			break
		}
	}
	ret = append(ret, s)
	return ret
}

func unescape(s string) string {
	ret := ""
	for {
		index := strings.IndexByte(s, '\\')
		if index < 0 {
			break
		}

		if index+1 == len(s) {
			break
		}

		switch s[index+1] {
		case ' ', '\\', '#', ':', '*', '[', '|', '\t', '\n', '\r':
			ret += s[:index] + s[index+1:index+2]
		default:
			ret += s[:index+2]
		}
		s = s[index+2:]
	}
	return ret + s
}
