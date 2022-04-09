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
	"fmt"
	"sort"
	"strings"
)

const (
	indent = 4
)

// Indention returns an indent string of the specified level.
func Indention(level int) string {
	if level < 0 {
		panic(fmt.Errorf("indent level cannot be less than 0, but got %d", level))
	}
	return strings.Repeat(" ", level*indent)
}

// PrintBool returns a Starlark compatible bool string.
func PrintBool(item bool) string {
	return strings.Title(fmt.Sprintf("%t", item))
}

// PrintsStringList returns a Starlark-compatible string of a list of Strings/Labels.
func PrintStringList(items []string, indentLevel int) string {
	return PrintList(items, indentLevel, func(s string) string {
		if strings.Contains(s, "\"") {
			return `'''%s'''`
		}
		return `"%s"`
	})
}

// PrintList returns a Starlark-compatible string of list formmated as requested.
func PrintList(items []string, indentLevel int, formatString func(string) string) string {
	if len(items) == 0 {
		return "[]"
	} else if len(items) == 1 {
		return fmt.Sprintf("["+formatString(items[0])+"]", items[0])
	}
	list := make([]string, 0, len(items)+2)
	list = append(list, "[")
	innerIndent := Indention(indentLevel + 1)
	for _, item := range items {
		list = append(list, fmt.Sprintf(`%s`+formatString(item)+`,`, innerIndent, item))
	}
	list = append(list, Indention(indentLevel)+"]")
	return strings.Join(list, "\n")
}

// PrintStringListDict returns a Starlark-compatible string formatted as dictionary with
// string keys and list of string values.
func PrintStringListDict(dict map[string][]string, indentLevel int) string {
	formattedValueDict := make(map[string]string, len(dict))
	for k, v := range dict {
		formattedValueDict[k] = PrintStringList(v, indentLevel+1)
	}
	return PrintDict(formattedValueDict, indentLevel)
}

// PrintBoolDict returns a starlark-compatible string containing a dictionary with string keys and
// values printed with no additional formatting.
func PrintBoolDict(dict map[string]bool, indentLevel int) string {
	formattedValueDict := make(map[string]string, len(dict))
	for k, v := range dict {
		formattedValueDict[k] = PrintBool(v)
	}
	return PrintDict(formattedValueDict, indentLevel)
}

// PrintDict returns a starlark-compatible string containing a dictionary with string keys and
// values printed with no additional formatting.
func PrintDict(dict map[string]string, indentLevel int) string {
	if len(dict) == 0 {
		return "{}"
	}
	items := make([]string, 0, len(dict))
	for k, v := range dict {
		items = append(items, fmt.Sprintf(`%s"%s": %s,`, Indention(indentLevel+1), k, v))
	}
	sort.Strings(items)
	return fmt.Sprintf(`{
%s
%s}`, strings.Join(items, "\n"), Indention(indentLevel))
}
