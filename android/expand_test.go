// Copyright 2016 Google Inc. All rights reserved.
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

package android

import (
	"fmt"
	"testing"
)

var vars = map[string]string{
	"var1":   "abc",
	"var2":   "",
	"var3":   "def",
	"💩":      "😃",
	"escape": "${in}",
}

func expander(s string) (string, bool, error) {
	if val, ok := vars[s]; ok {
		return val, s == "escape", nil
	} else {
		return "", false, fmt.Errorf("unknown variable %q", s)
	}
}

var expandTestCases = []struct {
	in          string
	out         string
	out_escaped string
	err         bool
}{
	{
		in:          "$(var1)",
		out:         "abc",
		out_escaped: "abc",
	},
	{
		in:          "$( var1 )",
		out:         "abc",
		out_escaped: "abc",
	},
	{
		in:          "def$(var1)",
		out:         "defabc",
		out_escaped: "defabc",
	},
	{
		in:          "$(var1)def",
		out:         "abcdef",
		out_escaped: "abcdef",
	},
	{
		in:          "def$(var1)def",
		out:         "defabcdef",
		out_escaped: "defabcdef",
	},
	{
		in:          "$(var2)",
		out:         "",
		out_escaped: "",
	},
	{
		in:          "def$(var2)",
		out:         "def",
		out_escaped: "def",
	},
	{
		in:          "$(var2)def",
		out:         "def",
		out_escaped: "def",
	},
	{
		in:          "def$(var2)def",
		out:         "defdef",
		out_escaped: "defdef",
	},
	{
		in:          "$(var1)$(var3)",
		out:         "abcdef",
		out_escaped: "abcdef",
	},
	{
		in:          "$(var1)g$(var3)",
		out:         "abcgdef",
		out_escaped: "abcgdef",
	},
	{
		in:          "$$",
		out:         "$",
		out_escaped: "$$",
	},
	{
		in:          "$$(var1)",
		out:         "$(var1)",
		out_escaped: "$$(var1)",
	},
	{
		in:          "$$$(var1)",
		out:         "$abc",
		out_escaped: "$$abc",
	},
	{
		in:          "$(var1)$$",
		out:         "abc$",
		out_escaped: "abc$$",
	},
	{
		in:          "$(💩)",
		out:         "😃",
		out_escaped: "😃",
	},
	{
		in:          "$$a$(escape)$$b",
		out:         "$a${in}$b",
		out_escaped: "$$a${in}$$b",
	},

	// Errors
	{
		in:  "$",
		err: true,
	},
	{
		in:  "$$$",
		err: true,
	},
	{
		in:  "$(var1)$",
		err: true,
	},
	{
		in:  "$(var1)$",
		err: true,
	},
	{
		in:  "$(var4)",
		err: true,
	},
	{
		in:  "$var1",
		err: true,
	},
	{
		in:  "$(var1",
		err: true,
	},
	{
		in:  "$a💩c",
		err: true,
	},
}

func TestExpand(t *testing.T) {
	for _, test := range expandTestCases {
		got, err := Expand(test.in, func(s string) (string, error) {
			s, _, err := expander(s)
			return s, err
		})
		if err != nil && !test.err {
			t.Errorf("%q: unexpected error %s", test.in, err.Error())
		} else if err == nil && test.err {
			t.Errorf("%q: expected error, got %q", test.in, got)
		} else if !test.err && got != test.out {
			t.Errorf("%q: expected %q, got %q", test.in, test.out, got)
		}
	}
}

func TestExpandNinjaEscaped(t *testing.T) {
	for _, test := range expandTestCases {
		got, err := ExpandNinjaEscaped(test.in, expander)
		if err != nil && !test.err {
			t.Errorf("%q: unexpected error %s", test.in, err.Error())
		} else if err == nil && test.err {
			t.Errorf("%q: expected error, got %q", test.in, got)
		} else if !test.err && got != test.out_escaped {
			t.Errorf("%q: expected %q, got %q", test.in, test.out, got)
		}
	}
}
