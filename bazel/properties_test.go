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
func TestFirstUniqueBazelLabelList(t *testing.T) {
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
		actualUniqueLabelList := FirstUniqueBazelLabelList(tc.originalLabelList)
		if !reflect.DeepEqual(tc.expectedUniqueLabelList, actualUniqueLabelList) {
			t.Fatalf("Expected %v, got %v", tc.expectedUniqueLabelList, actualUniqueLabelList)
		}
	}
}

func TestUniqueSortedBazelLabelList(t *testing.T) {
	testCases := []struct {
		originalLabelList       LabelList
		expectedUniqueLabelList LabelList
	}{
		{
			originalLabelList: LabelList{
				Includes: []Label{
					{Label: "c"},
					{Label: "a"},
					{Label: "a"},
					{Label: "b"},
				},
				Excludes: []Label{
					{Label: "y"},
					{Label: "z"},
					{Label: "x"},
					{Label: "x"},
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
		actualUniqueLabelList := UniqueSortedBazelLabelList(tc.originalLabelList)
		if !reflect.DeepEqual(tc.expectedUniqueLabelList, actualUniqueLabelList) {
			t.Fatalf("Expected %v, got %v", tc.expectedUniqueLabelList, actualUniqueLabelList)
		}
	}
}

func makeLabels(labels ...string) []Label {
	var ret []Label
	for _, l := range labels {
		ret = append(ret, Label{Label: l})
	}
	return ret
}

func makeLabelList(includes, excludes []string) LabelList {
	return LabelList{
		Includes: makeLabels(includes...),
		Excludes: makeLabels(excludes...),
	}
}

func TestResolveExcludes(t *testing.T) {
	attr := LabelListAttribute{
		Value: makeLabelList(
			[]string{
				"all_include",
				"arm_exclude",
				"android_exclude",
			},
			[]string{"all_exclude"},
		),
		ConfigurableValues: configurableLabelLists{
			ArchConfigurationAxis: labelListSelectValues{
				"arm": makeLabelList([]string{}, []string{"arm_exclude"}),
				"x86": makeLabelList([]string{"x86_include"}, []string{}),
			},
			OsConfigurationAxis: labelListSelectValues{
				"android": makeLabelList([]string{}, []string{"android_exclude"}),
				"linux":   makeLabelList([]string{"linux_include"}, []string{}),
			},
			OsArchConfigurationAxis: labelListSelectValues{
				"linux_x86": makeLabelList([]string{"linux_x86_include"}, []string{}),
			},
			ProductVariableConfigurationAxis("a"): labelListSelectValues{
				"a": makeLabelList([]string{}, []string{"not_in_value"}),
			},
		},
	}

	attr.ResolveExcludes()

	expectedBaseIncludes := []Label{Label{Label: "all_include"}}
	if !reflect.DeepEqual(expectedBaseIncludes, attr.Value.Includes) {
		t.Errorf("Expected Value includes %q, got %q", attr.Value.Includes, expectedBaseIncludes)
	}
	var nilLabels []Label
	expectedConfiguredIncludes := map[ConfigurationAxis]map[string][]Label{
		ArchConfigurationAxis: map[string][]Label{
			"arm":                nilLabels,
			"x86":                makeLabels("arm_exclude", "x86_include"),
			"conditions_default": makeLabels("arm_exclude"),
		},
		OsConfigurationAxis: map[string][]Label{
			"android":            nilLabels,
			"linux":              makeLabels("android_exclude", "linux_include"),
			"conditions_default": makeLabels("android_exclude"),
		},
		OsArchConfigurationAxis: map[string][]Label{
			"linux_x86":          makeLabels("linux_x86_include"),
			"conditions_default": nilLabels,
		},
	}
	for _, axis := range attr.SortedConfigurationAxes() {
		if _, ok := expectedConfiguredIncludes[axis]; !ok {
			t.Errorf("Found unexpected axis %s", axis)
			continue
		}
		expectedForAxis := expectedConfiguredIncludes[axis]
		gotForAxis := attr.ConfigurableValues[axis]
		if len(expectedForAxis) != len(gotForAxis) {
			t.Errorf("Expected %d configs for %s, got %d: %s", len(expectedForAxis), axis, len(gotForAxis), gotForAxis)
		}
		for config, value := range gotForAxis {
			if expected, ok := expectedForAxis[config]; ok {
				if !reflect.DeepEqual(expected, value.Includes) {
					t.Errorf("For %s, expected: %#v, got %#v", axis, expected, value.Includes)
				}
			} else {
				t.Errorf("Got unexpected config %q for %s", config, axis)
			}
		}
	}
}
