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
		description    string
		exportedVars   map[string]variableValue
		toExpand       string
		expectedValues []string
	}{
		{
			description: "single level expansion",
			exportedVars: map[string]variableValue{
				"foo": variableValue([]string{"bar"}),
			},
			toExpand:       "${foo}",
			expectedValues: []string{"bar"},
		},
		{
			description: "double level expansion",
			exportedVars: map[string]variableValue{
				"foo": variableValue([]string{"${bar}"}),
				"bar": variableValue([]string{"baz"}),
			},
			toExpand:       "${foo}",
			expectedValues: []string{"baz"},
		},
		{
			description: "double level expansion with a literal",
			exportedVars: map[string]variableValue{
				"a": variableValue([]string{"${b}", "c"}),
				"b": variableValue([]string{"d"}),
			},
			toExpand:       "${a}",
			expectedValues: []string{"d", "c"},
		},
		{
			description: "double level expansion, with two variables in a string",
			exportedVars: map[string]variableValue{
				"a": variableValue([]string{"${b} ${c}"}),
				"b": variableValue([]string{"d"}),
				"c": variableValue([]string{"e"}),
			},
			toExpand:       "${a}",
			expectedValues: []string{"d", "e"},
		},
		{
			description: "triple level expansion with two variables in a string",
			exportedVars: map[string]variableValue{
				"a": variableValue([]string{"${b} ${c}"}),
				"b": variableValue([]string{"${c}", "${d}"}),
				"c": variableValue([]string{"${d}"}),
				"d": variableValue([]string{"foo"}),
			},
			toExpand:       "${a}",
			expectedValues: []string{"foo", "foo", "foo"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			output := expandVar(testCase.toExpand, testCase.exportedVars)
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
