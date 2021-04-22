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

package bazel

import (
	"reflect"
	"testing"
)

func TestUniqueBazelLabels(t *testing.T) {
	testCases := []struct {
		originalLabels       []Label
		expectedUniqueLabels []Label
	}{
		{
			originalLabels: []Label{
				{Label: "a"},
				{Label: "b"},
				{Label: "a"},
				{Label: "c"},
			},
			expectedUniqueLabels: []Label{
				{Label: "a"},
				{Label: "b"},
				{Label: "c"},
			},
		},
	}
	for _, tc := range testCases {
		actualUniqueLabels := UniqueSortedBazelLabels(tc.originalLabels)
		if !reflect.DeepEqual(tc.expectedUniqueLabels, actualUniqueLabels) {
			t.Fatalf("Expected %v, got %v", tc.expectedUniqueLabels, actualUniqueLabels)
		}
	}
}

func TestSubtractStrings(t *testing.T) {
	testCases := []struct {
		haystack       []string
		needle         []string
		expectedResult []string
	}{
		{
			haystack: []string{
				"a",
				"b",
				"c",
			},
			needle: []string{
				"a",
			},
			expectedResult: []string{
				"b", "c",
			},
		},
	}
	for _, tc := range testCases {
		actualResult := SubtractStrings(tc.haystack, tc.needle)
		if !reflect.DeepEqual(tc.expectedResult, actualResult) {
			t.Fatalf("Expected %v, got %v", tc.expectedResult, actualResult)
		}
	}
}

func TestSubtractBazelLabelList(t *testing.T) {
	testCases := []struct {
		haystack       LabelList
		needle         LabelList
		expectedResult LabelList
	}{
		{
			haystack: LabelList{
				Includes: []Label{
					{Label: "a"},
					{Label: "b"},
					{Label: "c"},
				},
				Excludes: []Label{
					{Label: "x"},
					{Label: "y"},
					{Label: "z"},
				},
			},
			needle: LabelList{
				Includes: []Label{
					{Label: "a"},
				},
				Excludes: []Label{
					{Label: "z"},
				},
			},
			// NOTE: Excludes are intentionally not subtracted
			expectedResult: LabelList{
				Includes: []Label{
					{Label: "b"},
					{Label: "c"},
				},
				Excludes: []Label{
					{Label: "x"},
					{Label: "y"},
					{Label: "z"},
				},
			},
		},
	}
	for _, tc := range testCases {
		actualResult := SubtractBazelLabelList(tc.haystack, tc.needle)
		if !reflect.DeepEqual(tc.expectedResult, actualResult) {
			t.Fatalf("Expected %v, got %v", tc.expectedResult, actualResult)
		}
	}
}
func TestUniqueBazelLabelList(t *testing.T) {
	testCases := []struct {
		originalLabelList       LabelList
		expectedUniqueLabelList LabelList
	}{
		{
			originalLabelList: LabelList{
				Includes: []Label{
					{Label: "a"},
					{Label: "b"},
					{Label: "a"},
					{Label: "c"},
				},
				Excludes: []Label{
					{Label: "x"},
					{Label: "x"},
					{Label: "y"},
					{Label: "z"},
				},
			},
			expectedUniqueLabelList: LabelList{
				Includes: []Label{
					{Label: "a"},
					{Label: "b"},
					{Label: "c"},
				},
				Excludes: []Label{
					{Label: "x"},
					{Label: "y"},
					{Label: "z"},
				},
			},
		},
	}
	for _, tc := range testCases {
		actualUniqueLabelList := UniqueBazelLabelList(tc.originalLabelList)
		if !reflect.DeepEqual(tc.expectedUniqueLabelList, actualUniqueLabelList) {
			t.Fatalf("Expected %v, got %v", tc.expectedUniqueLabelList, actualUniqueLabelList)
		}
	}
}
