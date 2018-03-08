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

package android

import (
	"reflect"
	"testing"
)

var firstUniqueStringsTestCases = []struct {
	in  []string
	out []string
}{
	{
		in:  []string{"a"},
		out: []string{"a"},
	},
	{
		in:  []string{"a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"a", "a"},
		out: []string{"a"},
	},
	{
		in:  []string{"a", "b", "a"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"b", "a", "a"},
		out: []string{"b", "a"},
	},
	{
		in:  []string{"a", "a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"a", "b", "a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"liblog", "libdl", "libc++", "libdl", "libc", "libm"},
		out: []string{"liblog", "libdl", "libc++", "libc", "libm"},
	},
}

func TestFirstUniqueStrings(t *testing.T) {
	for _, testCase := range firstUniqueStringsTestCases {
		out := FirstUniqueStrings(testCase.in)
		if !reflect.DeepEqual(out, testCase.out) {
			t.Errorf("incorrect output:")
			t.Errorf("     input: %#v", testCase.in)
			t.Errorf("  expected: %#v", testCase.out)
			t.Errorf("       got: %#v", out)
		}
	}
}

var lastUniqueStringsTestCases = []struct {
	in  []string
	out []string
}{
	{
		in:  []string{"a"},
		out: []string{"a"},
	},
	{
		in:  []string{"a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"a", "a"},
		out: []string{"a"},
	},
	{
		in:  []string{"a", "b", "a"},
		out: []string{"b", "a"},
	},
	{
		in:  []string{"b", "a", "a"},
		out: []string{"b", "a"},
	},
	{
		in:  []string{"a", "a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"a", "b", "a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"liblog", "libdl", "libc++", "libdl", "libc", "libm"},
		out: []string{"liblog", "libc++", "libdl", "libc", "libm"},
	},
}

func TestLastUniqueStrings(t *testing.T) {
	for _, testCase := range lastUniqueStringsTestCases {
		out := LastUniqueStrings(testCase.in)
		if !reflect.DeepEqual(out, testCase.out) {
			t.Errorf("incorrect output:")
			t.Errorf("     input: %#v", testCase.in)
			t.Errorf("  expected: %#v", testCase.out)
			t.Errorf("       got: %#v", out)
		}
	}
}

func TestJoinWithPrefix(t *testing.T) {
	testcases := []struct {
		name     string
		input    []string
		expected string
	}{
		{
			name:     "zero_inputs",
			input:    []string{},
			expected: "",
		},
		{
			name:     "one_input",
			input:    []string{"a"},
			expected: "prefix:a",
		},
		{
			name:     "two_inputs",
			input:    []string{"a", "b"},
			expected: "prefix:a prefix:b",
		},
	}

	prefix := "prefix:"

	for _, testCase := range testcases {
		t.Run(testCase.name, func(t *testing.T) {
			out := JoinWithPrefix(testCase.input, prefix)
			if out != testCase.expected {
				t.Errorf("incorrect output:")
				t.Errorf("     input: %#v", testCase.input)
				t.Errorf("    prefix: %#v", prefix)
				t.Errorf("  expected: %#v", testCase.expected)
				t.Errorf("       got: %#v", out)
			}
		})
	}
}

func TestIndexList(t *testing.T) {
	input := []string{"a", "b", "c"}

	testcases := []struct {
		key      string
		expected int
	}{
		{
			key:      "a",
			expected: 0,
		},
		{
			key:      "b",
			expected: 1,
		},
		{
			key:      "c",
			expected: 2,
		},
		{
			key:      "X",
			expected: -1,
		},
	}

	for _, testCase := range testcases {
		t.Run(testCase.key, func(t *testing.T) {
			out := IndexList(testCase.key, input)
			if out != testCase.expected {
				t.Errorf("incorrect output:")
				t.Errorf("       key: %#v", testCase.key)
				t.Errorf("     input: %#v", input)
				t.Errorf("  expected: %#v", testCase.expected)
				t.Errorf("       got: %#v", out)
			}
		})
	}
}

func TestInList(t *testing.T) {
	input := []string{"a"}

	testcases := []struct {
		key      string
		expected bool
	}{
		{
			key:      "a",
			expected: true,
		},
		{
			key:      "X",
			expected: false,
		},
	}

	for _, testCase := range testcases {
		t.Run(testCase.key, func(t *testing.T) {
			out := InList(testCase.key, input)
			if out != testCase.expected {
				t.Errorf("incorrect output:")
				t.Errorf("       key: %#v", testCase.key)
				t.Errorf("     input: %#v", input)
				t.Errorf("  expected: %#v", testCase.expected)
				t.Errorf("       got: %#v", out)
			}
		})
	}
}

func TestPrefixInList(t *testing.T) {
	prefixes := []string{"a", "b"}

	testcases := []struct {
		str      string
		expected bool
	}{
		{
			str:      "a-example",
			expected: true,
		},
		{
			str:      "b-example",
			expected: true,
		},
		{
			str:      "X-example",
			expected: false,
		},
	}

	for _, testCase := range testcases {
		t.Run(testCase.str, func(t *testing.T) {
			out := PrefixInList(testCase.str, prefixes)
			if out != testCase.expected {
				t.Errorf("incorrect output:")
				t.Errorf("       str: %#v", testCase.str)
				t.Errorf("  prefixes: %#v", prefixes)
				t.Errorf("  expected: %#v", testCase.expected)
				t.Errorf("       got: %#v", out)
			}
		})
	}
}

func TestFilterList(t *testing.T) {
	input := []string{"a", "b", "c", "c", "b", "d", "a"}
	filter := []string{"a", "c"}
	remainder, filtered := FilterList(input, filter)

	expected := []string{"b", "b", "d"}
	if !reflect.DeepEqual(remainder, expected) {
		t.Errorf("incorrect remainder output:")
		t.Errorf("     input: %#v", input)
		t.Errorf("    filter: %#v", filter)
		t.Errorf("  expected: %#v", expected)
		t.Errorf("       got: %#v", remainder)
	}

	expected = []string{"a", "c", "c", "a"}
	if !reflect.DeepEqual(filtered, expected) {
		t.Errorf("incorrect filtered output:")
		t.Errorf("     input: %#v", input)
		t.Errorf("    filter: %#v", filter)
		t.Errorf("  expected: %#v", expected)
		t.Errorf("       got: %#v", filtered)
	}
}

func TestRemoveListFromList(t *testing.T) {
	input := []string{"a", "b", "c", "d", "a", "c", "d"}
	filter := []string{"a", "c"}
	expected := []string{"b", "d", "d"}
	out := RemoveListFromList(input, filter)
	if !reflect.DeepEqual(out, expected) {
		t.Errorf("incorrect output:")
		t.Errorf("     input: %#v", input)
		t.Errorf("    filter: %#v", filter)
		t.Errorf("  expected: %#v", expected)
		t.Errorf("       got: %#v", out)
	}
}
