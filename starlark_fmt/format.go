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
	"reflect"
	"sort"
	"strconv"
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

func PrintAny(value any, indentLevel int) string {
	return printAnyRecursive(reflect.ValueOf(value), indentLevel)
}

func printAnyRecursive(value reflect.Value, indentLevel int) string {
	switch value.Type().Kind() {
	case reflect.String:
		val := value.String()
		if strings.Contains(val, "\"") || strings.Contains(val, "\n") {
			return `'''` + val + `'''`
		}
		return `"` + val + `"`
	case reflect.Bool:
		if value.Bool() {
			return "True"
		} else {
			return "False"
		}
	case reflect.Int:
		return fmt.Sprintf("%d", value.Int())
	case reflect.Slice:
		if value.Len() == 0 {
			return "[]"
		} else if value.Len() == 1 {
			return "[" + printAnyRecursive(value.Index(0), indentLevel) + "]"
		}
		list := make([]string, 0, value.Len()+2)
		list = append(list, "[")
		innerIndent := Indention(indentLevel + 1)
		for i := 0; i < value.Len(); i++ {
			list = append(list, innerIndent+printAnyRecursive(value.Index(i), indentLevel+1)+`,`)
		}
		list = append(list, Indention(indentLevel)+"]")
		return strings.Join(list, "\n")
	case reflect.Map:
		if value.Len() == 0 {
			return "{}"
		}
		items := make([]string, 0, value.Len())
		for _, key := range value.MapKeys() {
			items = append(items, fmt.Sprintf(`%s%s: %s,`, Indention(indentLevel+1), printAnyRecursive(key, indentLevel+1), printAnyRecursive(value.MapIndex(key), indentLevel+1)))
		}
		sort.Strings(items)
		return fmt.Sprintf(`{
%s
%s}`, strings.Join(items, "\n"), Indention(indentLevel))
	case reflect.Struct:
		if value.NumField() == 0 {
			return "struct()"
		}
		items := make([]string, 0, value.NumField()+2)
		items = append(items, "struct(")
		for i := 0; i < value.NumField(); i++ {
			if value.Type().Field(i).Anonymous {
				panic("anonymous fields aren't supported")
			}
			name := value.Type().Field(i).Name
			items = append(items, fmt.Sprintf(`%s%s = %s,`, Indention(indentLevel+1), name, printAnyRecursive(value.Field(i), indentLevel+1)))
		}
		items = append(items, Indention(indentLevel)+")")
		return strings.Join(items, "\n")
	default:
		panic("Unhandled kind: " + value.Kind().String())
	}
}

// PrintBool returns a Starlark compatible bool string.
func PrintBool(item bool) string {
	if item {
		return "True"
	} else {
		return "False"
	}
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

// PrintStringIntDict returns a Starlark-compatible string formatted as dictionary with
// string keys and int values.
func PrintStringIntDict(dict map[string]int, indentLevel int) string {
	valDict := make(map[string]string, len(dict))
	for k, v := range dict {
		valDict[k] = strconv.Itoa(v)
	}
	return PrintDict(valDict, indentLevel)
}

// PrintStringStringDict returns a Starlark-compatible string formatted as dictionary with
// string keys and string values.
func PrintStringStringDict(dict map[string]string, indentLevel int) string {
	valDict := make(map[string]string, len(dict))
	for k, v := range dict {
		valDict[k] = fmt.Sprintf(`"%s"`, v)
	}
	return PrintDict(valDict, indentLevel)
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
