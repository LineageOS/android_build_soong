// Copyright 2022 Google Inc. All rights reserved.
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

package starlark_fmt

import (
	"testing"
)

func simpleFormat(s string) string {
	return "%s"
}

func TestPrintEmptyStringList(t *testing.T) {
	in := []string{}
	indentLevel := 0
	out := PrintStringList(in, indentLevel)
	expectedOut := "[]"
	if out != expectedOut {
		t.Errorf("Expected %q, got %q", expectedOut, out)
	}
}

func TestPrintSingleElementStringList(t *testing.T) {
	in := []string{"a"}
	indentLevel := 0
	out := PrintStringList(in, indentLevel)
	expectedOut := `["a"]`
	if out != expectedOut {
		t.Errorf("Expected %q, got %q", expectedOut, out)
	}
}

func TestPrintMultiElementStringList(t *testing.T) {
	in := []string{"a", "b"}
	indentLevel := 0
	out := PrintStringList(in, indentLevel)
	expectedOut := `[
    "a",
    "b",
]`
	if out != expectedOut {
		t.Errorf("Expected %q, got %q", expectedOut, out)
	}
}

func TestPrintEmptyList(t *testing.T) {
	in := []string{}
	indentLevel := 0
	out := PrintList(in, indentLevel, simpleFormat)
	expectedOut := "[]"
	if out != expectedOut {
		t.Errorf("Expected %q, got %q", expectedOut, out)
	}
}

func TestPrintSingleElementList(t *testing.T) {
	in := []string{"1"}
	indentLevel := 0
	out := PrintList(in, indentLevel, simpleFormat)
	expectedOut := `[1]`
	if out != expectedOut {
		t.Errorf("Expected %q, got %q", expectedOut, out)
	}
}

func TestPrintMultiElementList(t *testing.T) {
	in := []string{"1", "2"}
	indentLevel := 0
	out := PrintList(in, indentLevel, simpleFormat)
	expectedOut := `[
    1,
    2,
]`
	if out != expectedOut {
		t.Errorf("Expected %q, got %q", expectedOut, out)
	}
}

func TestListWithNonZeroIndent(t *testing.T) {
	in := []string{"1", "2"}
	indentLevel := 1
	out := PrintList(in, indentLevel, simpleFormat)
	expectedOut := `[
        1,
        2,
    ]`
	if out != expectedOut {
		t.Errorf("Expected %q, got %q", expectedOut, out)
	}
}

func TestStringListDictEmpty(t *testing.T) {
	in := map[string][]string{}
	indentLevel := 0
	out := PrintStringListDict(in, indentLevel)
	expectedOut := `{}`
	if out != expectedOut {
		t.Errorf("Expected %q, got %q", expectedOut, out)
	}
}

func TestStringListDict(t *testing.T) {
	in := map[string][]string{
		"key1": []string{},
		"key2": []string{"a"},
		"key3": []string{"1", "2"},
	}
	indentLevel := 0
	out := PrintStringListDict(in, indentLevel)
	expectedOut := `{
    "key1": [],
    "key2": ["a"],
    "key3": [
        "1",
        "2",
    ],
}`
	if out != expectedOut {
		t.Errorf("Expected %q, got %q", expectedOut, out)
	}
}

func TestPrintDict(t *testing.T) {
	in := map[string]string{
		"key1": `""`,
		"key2": `"a"`,
		"key3": `[
        1,
        2,
    ]`,
	}
	indentLevel := 0
	out := PrintDict(in, indentLevel)
	expectedOut := `{
    "key1": "",
    "key2": "a",
    "key3": [
        1,
        2,
    ],
}`
	if out != expectedOut {
		t.Errorf("Expected %q, got %q", expectedOut, out)
	}
}

func TestPrintDictWithIndent(t *testing.T) {
	in := map[string]string{
		"key1": `""`,
		"key2": `"a"`,
	}
	indentLevel := 1
	out := PrintDict(in, indentLevel)
	expectedOut := `{
        "key1": "",
        "key2": "a",
    }`
	if out != expectedOut {
		t.Errorf("Expected %q, got %q", expectedOut, out)
	}
}
