// Copyright 2021 Google Inc. All rights reserved.
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

package config

import (
	"testing"
)

func TestExpandVars(t *testing.T) {
	testCases := []struct {
		description     string
		stringScope     exportedStringVariables
		stringListScope exportedStringListVariables
		toExpand        string
		expectedValues  []string
	}{
		{
			description:    "no expansion for non-interpolated value",
			toExpand:       "foo",
			expectedValues: []string{"foo"},
		},
		{
			description: "single level expansion for string var",
			stringScope: exportedStringVariables{
				"foo": "bar",
			},
			toExpand:       "${foo}",
			expectedValues: []string{"bar"},
		},
		{
			description: "single level expansion string list var",
			stringListScope: exportedStringListVariables{
				"foo": []string{"bar"},
			},
			toExpand:       "${foo}",
			expectedValues: []string{"bar"},
		},
		{
			description: "mixed level expansion for string list var",
			stringScope: exportedStringVariables{
				"foo": "${bar}",
				"qux": "hello",
			},
			stringListScope: exportedStringListVariables{
				"bar": []string{"baz", "${qux}"},
			},
			toExpand:       "${foo}",
			expectedValues: []string{"baz", "hello"},
		},
		{
			description: "double level expansion",
			stringListScope: exportedStringListVariables{
				"foo": []string{"${bar}"},
				"bar": []string{"baz"},
			},
			toExpand:       "${foo}",
			expectedValues: []string{"baz"},
		},
		{
			description: "double level expansion with a literal",
			stringListScope: exportedStringListVariables{
				"a": []string{"${b}", "c"},
				"b": []string{"d"},
			},
			toExpand:       "${a}",
			expectedValues: []string{"d", "c"},
		},
		{
			description: "double level expansion, with two variables in a string",
			stringListScope: exportedStringListVariables{
				"a": []string{"${b} ${c}"},
				"b": []string{"d"},
				"c": []string{"e"},
			},
			toExpand:       "${a}",
			expectedValues: []string{"d", "e"},
		},
		{
			description: "triple level expansion with two variables in a string",
			stringListScope: exportedStringListVariables{
				"a": []string{"${b} ${c}"},
				"b": []string{"${c}", "${d}"},
				"c": []string{"${d}"},
				"d": []string{"foo"},
			},
			toExpand:       "${a}",
			expectedValues: []string{"foo", "foo", "foo"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			output := expandVar(testCase.toExpand, testCase.stringScope, testCase.stringListScope)
			if len(output) != len(testCase.expectedValues) {
				t.Errorf("Expected %d values, got %d", len(testCase.expectedValues), len(output))
			}
			for i, actual := range output {
				expectedValue := testCase.expectedValues[i]
				if actual != expectedValue {
					t.Errorf("Actual value '%s' doesn't match expected value '%s'", actual, expectedValue)
				}
			}
		})
	}
}
