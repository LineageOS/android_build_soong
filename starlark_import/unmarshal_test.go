// Copyright 2023 Google Inc. All rights reserved.
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

package starlark_import

import (
	"reflect"
	"testing"

	"go.starlark.net/starlark"
)

func createStarlarkValue(t *testing.T, code string) starlark.Value {
	t.Helper()
	result, err := starlark.ExecFile(&starlark.Thread{}, "main.bzl", "x = "+code, builtins)
	if err != nil {
		panic(err)
	}
	return result["x"]
}

func TestUnmarshalConcreteType(t *testing.T) {
	x, err := Unmarshal[string](createStarlarkValue(t, `"foo"`))
	if err != nil {
		t.Error(err)
		return
	}
	if x != "foo" {
		t.Errorf(`Expected "foo", got %q`, x)
	}
}

func TestUnmarshalConcreteTypeWithInterfaces(t *testing.T) {
	x, err := Unmarshal[map[string]map[string]interface{}](createStarlarkValue(t,
		`{"foo": {"foo2": "foo3"}, "bar": {"bar2": ["bar3"]}}`))
	if err != nil {
		t.Error(err)
		return
	}
	expected := map[string]map[string]interface{}{
		"foo": {"foo2": "foo3"},
		"bar": {"bar2": []string{"bar3"}},
	}
	if !reflect.DeepEqual(x, expected) {
		t.Errorf(`Expected %v, got %v`, expected, x)
	}
}

func TestUnmarshalToStarlarkValue(t *testing.T) {
	x, err := Unmarshal[map[string]starlark.Value](createStarlarkValue(t,
		`{"foo": "Hi", "bar": None}`))
	if err != nil {
		t.Error(err)
		return
	}
	if x["foo"].(starlark.String).GoString() != "Hi" {
		t.Errorf("Expected \"Hi\", got: %q", x["foo"].(starlark.String).GoString())
	}
	if x["bar"].Type() != "NoneType" {
		t.Errorf("Expected \"NoneType\", got: %q", x["bar"].Type())
	}
}

func TestUnmarshal(t *testing.T) {
	testCases := []struct {
		input    string
		expected interface{}
	}{
		{
			input:    `"foo"`,
			expected: "foo",
		},
		{
			input:    `5`,
			expected: 5,
		},
		{
			input:    `["foo", "bar"]`,
			expected: []string{"foo", "bar"},
		},
		{
			input:    `("foo", "bar")`,
			expected: []string{"foo", "bar"},
		},
		{
			input:    `("foo",5)`,
			expected: []interface{}{"foo", 5},
		},
		{
			input:    `{"foo": 5, "bar": 10}`,
			expected: map[string]int{"foo": 5, "bar": 10},
		},
		{
			input:    `{"foo": ["qux"], "bar": []}`,
			expected: map[string][]string{"foo": {"qux"}, "bar": nil},
		},
		{
			input: `struct(Foo="foo", Bar=5)`,
			expected: struct {
				Foo string
				Bar int
			}{Foo: "foo", Bar: 5},
		},
		{
			// Unexported fields version of the above
			input: `struct(foo="foo", bar=5)`,
			expected: struct {
				foo string
				bar int
			}{foo: "foo", bar: 5},
		},
		{
			input: `{"foo": "foo2", "bar": ["bar2"], "baz": 5, "qux": {"qux2": "qux3"}, "quux": {"quux2": "quux3", "quux4": 5}}`,
			expected: map[string]interface{}{
				"foo": "foo2",
				"bar": []string{"bar2"},
				"baz": 5,
				"qux": map[string]string{"qux2": "qux3"},
				"quux": map[string]interface{}{
					"quux2": "quux3",
					"quux4": 5,
				},
			},
		},
	}

	for _, tc := range testCases {
		x, err := UnmarshalReflect(createStarlarkValue(t, tc.input), reflect.TypeOf(tc.expected))
		if err != nil {
			t.Error(err)
			continue
		}
		if !reflect.DeepEqual(x.Interface(), tc.expected) {
			t.Errorf(`Expected %#v, got %#v`, tc.expected, x.Interface())
		}
	}
}
