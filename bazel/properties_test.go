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
	"strings"
	"testing"

	"github.com/google/blueprint/proptools"
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
				// namespaces
				{Label: "//foo:bar", OriginalModuleName: "bar"},       // when referenced from foo namespace
				{Label: "//foo:bar", OriginalModuleName: "//foo:bar"}, // when reference from root namespace
			},
			expectedUniqueLabels: []Label{
				{Label: "//foo:bar", OriginalModuleName: "bar"},
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

func TestSubtractBazelLabelListAttribute(t *testing.T) {
	testCases := []struct {
		haystack LabelListAttribute
		needle   LabelListAttribute
		expected LabelListAttribute
	}{
		{
			haystack: LabelListAttribute{
				Value: makeLabelList(
					[]string{"a", "b", "a", "c"},
					[]string{"x", "x", "y", "z"},
				),
				ConfigurableValues: configurableLabelLists{
					ArchConfigurationAxis: labelListSelectValues{
						"arm": makeLabelList([]string{"arm_1", "arm_2"}, []string{}),
						"x86": makeLabelList([]string{"x86_3", "x86_4", "x86_5"}, []string{"x86_5"}),
					},
				},
			},
			needle: LabelListAttribute{
				Value: makeLabelList(
					[]string{"d", "a"},
					[]string{"x", "y2", "z2"},
				),
				ConfigurableValues: configurableLabelLists{
					ArchConfigurationAxis: labelListSelectValues{
						"arm": makeLabelList([]string{"arm_1", "arm_3"}, []string{}),
						"x86": makeLabelList([]string{"x86_3", "x86_4"}, []string{"x86_6"}),
					},
				},
			},
			expected: LabelListAttribute{
				Value: makeLabelList(
					[]string{"b", "c"},
					[]string{"x", "x", "y", "z"},
				),
				ConfigurableValues: configurableLabelLists{
					ArchConfigurationAxis: labelListSelectValues{
						"arm": makeLabelList([]string{"arm_2"}, []string{}),
						"x86": makeLabelList([]string{"x86_5"}, []string{"x86_5"}),
					},
				},
				ForceSpecifyEmptyList: false,
				EmitEmptyList:         false,
				Prepend:               false,
			},
		},
	}
	for _, tc := range testCases {
		got := SubtractBazelLabelListAttribute(tc.haystack, tc.needle)
		if !reflect.DeepEqual(tc.expected, got) {
			t.Fatalf("Expected\n%v, but got\n%v", tc.expected, got)
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
					// namespaces
					{Label: "//foo:bar", OriginalModuleName: "bar"},       // when referenced from foo namespace
					{Label: "//foo:bar", OriginalModuleName: "//foo:bar"}, // when referenced from root namespace
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
					{Label: "//foo:bar", OriginalModuleName: "bar"},
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

func TestFirstUniqueBazelLabelListAttribute(t *testing.T) {
	testCases := []struct {
		originalLabelList       LabelListAttribute
		expectedUniqueLabelList LabelListAttribute
	}{
		{
			originalLabelList: LabelListAttribute{
				Value: makeLabelList(
					[]string{"a", "b", "a", "c"},
					[]string{"x", "x", "y", "z"},
				),
				ConfigurableValues: configurableLabelLists{
					ArchConfigurationAxis: labelListSelectValues{
						"arm": makeLabelList([]string{"1", "2", "1"}, []string{}),
						"x86": makeLabelList([]string{"3", "4", "4"}, []string{"5", "5"}),
					},
				},
			},
			expectedUniqueLabelList: LabelListAttribute{
				Value: makeLabelList(
					[]string{"a", "b", "c"},
					[]string{"x", "y", "z"},
				),
				ConfigurableValues: configurableLabelLists{
					ArchConfigurationAxis: labelListSelectValues{
						"arm": makeLabelList([]string{"1", "2"}, []string{}),
						"x86": makeLabelList([]string{"3", "4"}, []string{"5"}),
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		actualUniqueLabelList := FirstUniqueBazelLabelListAttribute(tc.originalLabelList)
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
				"product_config_exclude",
			},
			[]string{"all_exclude"},
		),
		ConfigurableValues: configurableLabelLists{
			ArchConfigurationAxis: labelListSelectValues{
				"arm":                      makeLabelList([]string{}, []string{"arm_exclude"}),
				"x86":                      makeLabelList([]string{"x86_include"}, []string{}),
				ConditionsDefaultConfigKey: makeLabelList([]string{"default_include"}, []string{}),
			},
			OsConfigurationAxis: labelListSelectValues{
				"android": makeLabelList([]string{}, []string{"android_exclude"}),
				"linux":   makeLabelList([]string{"linux_include"}, []string{}),
			},
			OsArchConfigurationAxis: labelListSelectValues{
				"linux_x86": makeLabelList([]string{"linux_x86_include"}, []string{}),
			},
			ProductVariableConfigurationAxis(false, "product_with_defaults"): labelListSelectValues{
				"a":                        makeLabelList([]string{}, []string{"not_in_value"}),
				"b":                        makeLabelList([]string{"b_val"}, []string{}),
				"c":                        makeLabelList([]string{"c_val"}, []string{}),
				ConditionsDefaultConfigKey: makeLabelList([]string{"c_val", "default", "default2", "all_exclude"}, []string{}),
			},
			ProductVariableConfigurationAxis(false, "product_only_with_excludes"): labelListSelectValues{
				"a": makeLabelList([]string{}, []string{"product_config_exclude"}),
			},
		},
	}

	attr.ResolveExcludes()

	expectedBaseIncludes := []Label{{Label: "all_include"}}
	if !reflect.DeepEqual(expectedBaseIncludes, attr.Value.Includes) {
		t.Errorf("Expected Value includes %q, got %q", attr.Value.Includes, expectedBaseIncludes)
	}
	var nilLabels []Label
	expectedConfiguredIncludes := map[ConfigurationAxis]map[string][]Label{
		ArchConfigurationAxis: {
			"arm":                      nilLabels,
			"x86":                      makeLabels("arm_exclude", "x86_include"),
			ConditionsDefaultConfigKey: makeLabels("arm_exclude", "default_include"),
		},
		OsConfigurationAxis: {
			"android":                  nilLabels,
			"linux":                    makeLabels("android_exclude", "linux_include"),
			ConditionsDefaultConfigKey: makeLabels("android_exclude"),
		},
		OsArchConfigurationAxis: {
			"linux_x86":                makeLabels("linux_x86_include"),
			ConditionsDefaultConfigKey: nilLabels,
		},
		ProductVariableConfigurationAxis(false, "product_with_defaults"): {
			"a":                        nilLabels,
			"b":                        makeLabels("b_val"),
			"c":                        makeLabels("c_val"),
			ConditionsDefaultConfigKey: makeLabels("c_val", "default", "default2"),
		},
		ProductVariableConfigurationAxis(false, "product_only_with_excludes"): {
			"a":                        nilLabels,
			ConditionsDefaultConfigKey: makeLabels("product_config_exclude"),
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
					t.Errorf("For %s,\nexpected: %#v\ngot %#v", axis, expected, value.Includes)
				}
			} else {
				t.Errorf("Got unexpected config %q for %s", config, axis)
			}
		}
	}
}

func TestLabelListAttributePartition(t *testing.T) {
	testCases := []struct {
		name         string
		input        LabelListAttribute
		predicated   LabelListAttribute
		unpredicated LabelListAttribute
		predicate    func(label Label) bool
	}{
		{
			name: "move all to predicated partition",
			input: MakeLabelListAttribute(makeLabelList(
				[]string{"keep1", "throw1", "keep2", "throw2"},
				[]string{"keep1", "throw1", "keep2", "throw2"},
			)),
			predicated: MakeLabelListAttribute(makeLabelList(
				[]string{"keep1", "throw1", "keep2", "throw2"},
				[]string{"keep1", "throw1", "keep2", "throw2"},
			)),
			unpredicated: LabelListAttribute{},
			predicate: func(label Label) bool {
				return true
			},
		},
		{
			name: "move all to unpredicated partition",
			input: MakeLabelListAttribute(makeLabelList(
				[]string{"keep1", "throw1", "keep2", "throw2"},
				[]string{"keep1", "throw1", "keep2", "throw2"},
			)),
			predicated: LabelListAttribute{},
			unpredicated: MakeLabelListAttribute(makeLabelList(
				[]string{"keep1", "throw1", "keep2", "throw2"},
				[]string{"keep1", "throw1", "keep2", "throw2"},
			)),
			predicate: func(label Label) bool {
				return false
			},
		},
		{
			name: "partition includes and excludes",
			input: MakeLabelListAttribute(makeLabelList(
				[]string{"keep1", "throw1", "keep2", "throw2"},
				[]string{"keep1", "throw1", "keep2", "throw2"},
			)),
			predicated: MakeLabelListAttribute(makeLabelList(
				[]string{"keep1", "keep2"},
				[]string{"keep1", "keep2"},
			)),
			unpredicated: MakeLabelListAttribute(makeLabelList(
				[]string{"throw1", "throw2"},
				[]string{"throw1", "throw2"},
			)),
			predicate: func(label Label) bool {
				return strings.HasPrefix(label.Label, "keep")
			},
		},
		{
			name: "partition excludes only",
			input: MakeLabelListAttribute(makeLabelList(
				[]string{},
				[]string{"keep1", "throw1", "keep2", "throw2"},
			)),
			predicated: MakeLabelListAttribute(makeLabelList(
				[]string{},
				[]string{"keep1", "keep2"},
			)),
			unpredicated: MakeLabelListAttribute(makeLabelList(
				[]string{},
				[]string{"throw1", "throw2"},
			)),
			predicate: func(label Label) bool {
				return strings.HasPrefix(label.Label, "keep")
			},
		},
		{
			name: "partition includes only",
			input: MakeLabelListAttribute(makeLabelList(
				[]string{"keep1", "throw1", "keep2", "throw2"},
				[]string{},
			)),
			predicated: MakeLabelListAttribute(makeLabelList(
				[]string{"keep1", "keep2"},
				[]string{},
			)),
			unpredicated: MakeLabelListAttribute(makeLabelList(
				[]string{"throw1", "throw2"},
				[]string{},
			)),
			predicate: func(label Label) bool {
				return strings.HasPrefix(label.Label, "keep")
			},
		},
		{
			name:         "empty partition",
			input:        MakeLabelListAttribute(makeLabelList([]string{}, []string{})),
			predicated:   LabelListAttribute{},
			unpredicated: LabelListAttribute{},
			predicate: func(label Label) bool {
				return true
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			predicated, unpredicated := tc.input.Partition(tc.predicate)
			if !predicated.Value.Equals(tc.predicated.Value) {
				t.Errorf("expected predicated labels to be %v; got %v", tc.predicated, predicated)
			}
			for axis, configs := range predicated.ConfigurableValues {
				tcConfigs, ok := tc.predicated.ConfigurableValues[axis]
				if !ok || !reflect.DeepEqual(configs, tcConfigs) {
					t.Errorf("expected predicated labels to be %v; got %v", tc.predicated, predicated)
				}
			}
			if !unpredicated.Value.Equals(tc.unpredicated.Value) {
				t.Errorf("expected unpredicated labels to be %v; got %v", tc.unpredicated, unpredicated)
			}
			for axis, configs := range unpredicated.ConfigurableValues {
				tcConfigs, ok := tc.unpredicated.ConfigurableValues[axis]
				if !ok || !reflect.DeepEqual(configs, tcConfigs) {
					t.Errorf("expected unpredicated labels to be %v; got %v", tc.unpredicated, unpredicated)
				}
			}
		})
	}
}

// labelAddSuffixForTypeMapper returns a LabelMapper that adds suffix to label name for modules of
// typ
func labelAddSuffixForTypeMapper(suffix, typ string) LabelMapper {
	return func(omc OtherModuleContext, label Label) (string, bool) {
		m, ok := omc.ModuleFromName(label.Label)
		if !ok {
			return label.Label, false
		}
		mTyp := omc.OtherModuleType(m)
		if typ == mTyp {
			return label.Label + suffix, true
		}
		return label.Label, false
	}
}

func TestPartitionLabelListAttribute(t *testing.T) {
	testCases := []struct {
		name           string
		ctx            *OtherModuleTestContext
		labelList      LabelListAttribute
		filters        LabelPartitions
		expected       PartitionToLabelListAttribute
		expectedErrMsg *string
	}{
		{
			name: "no configurable values",
			ctx:  &OtherModuleTestContext{},
			labelList: LabelListAttribute{
				Value: makeLabelList([]string{"a.a", "b.b", "c.c", "d.d", "e.e"}, []string{}),
			},
			filters: LabelPartitions{
				"A": LabelPartition{Extensions: []string{".a"}},
				"B": LabelPartition{Extensions: []string{".b"}},
				"C": LabelPartition{Extensions: []string{".c"}},
			},
			expected: PartitionToLabelListAttribute{
				"A": LabelListAttribute{Value: makeLabelList([]string{"a.a"}, []string{})},
				"B": LabelListAttribute{Value: makeLabelList([]string{"b.b"}, []string{})},
				"C": LabelListAttribute{Value: makeLabelList([]string{"c.c"}, []string{})},
			},
		},
		{
			name: "no configurable values, remainder partition",
			ctx:  &OtherModuleTestContext{},
			labelList: LabelListAttribute{
				Value: makeLabelList([]string{"a.a", "b.b", "c.c", "d.d", "e.e"}, []string{}),
			},
			filters: LabelPartitions{
				"A": LabelPartition{Extensions: []string{".a"}, Keep_remainder: true},
				"B": LabelPartition{Extensions: []string{".b"}},
				"C": LabelPartition{Extensions: []string{".c"}},
			},
			expected: PartitionToLabelListAttribute{
				"A": LabelListAttribute{Value: makeLabelList([]string{"a.a", "d.d", "e.e"}, []string{})},
				"B": LabelListAttribute{Value: makeLabelList([]string{"b.b"}, []string{})},
				"C": LabelListAttribute{Value: makeLabelList([]string{"c.c"}, []string{})},
			},
		},
		{
			name: "no configurable values, empty partition",
			ctx:  &OtherModuleTestContext{},
			labelList: LabelListAttribute{
				Value: makeLabelList([]string{"a.a", "c.c"}, []string{}),
			},
			filters: LabelPartitions{
				"A": LabelPartition{Extensions: []string{".a"}},
				"B": LabelPartition{Extensions: []string{".b"}},
				"C": LabelPartition{Extensions: []string{".c"}},
			},
			expected: PartitionToLabelListAttribute{
				"A": LabelListAttribute{Value: makeLabelList([]string{"a.a"}, []string{})},
				"C": LabelListAttribute{Value: makeLabelList([]string{"c.c"}, []string{})},
			},
		},
		{
			name: "no configurable values, has map",
			ctx: &OtherModuleTestContext{
				Modules: []TestModuleInfo{{ModuleName: "srcs", Typ: "fg", Dir: "dir"}},
			},
			labelList: LabelListAttribute{
				Value: makeLabelList([]string{"a.a", "srcs", "b.b", "c.c"}, []string{}),
			},
			filters: LabelPartitions{
				"A": LabelPartition{Extensions: []string{".a"}, LabelMapper: labelAddSuffixForTypeMapper("_a", "fg")},
				"B": LabelPartition{Extensions: []string{".b"}},
				"C": LabelPartition{Extensions: []string{".c"}},
			},
			expected: PartitionToLabelListAttribute{
				"A": LabelListAttribute{Value: makeLabelList([]string{"a.a", "srcs_a"}, []string{})},
				"B": LabelListAttribute{Value: makeLabelList([]string{"b.b"}, []string{})},
				"C": LabelListAttribute{Value: makeLabelList([]string{"c.c"}, []string{})},
			},
		},
		{
			name: "configurable values, keeps empty if excludes",
			ctx:  &OtherModuleTestContext{},
			labelList: LabelListAttribute{
				ConfigurableValues: configurableLabelLists{
					ArchConfigurationAxis: labelListSelectValues{
						"x86":    makeLabelList([]string{"a.a", "c.c"}, []string{}),
						"arm":    makeLabelList([]string{"b.b"}, []string{}),
						"x86_64": makeLabelList([]string{"b.b"}, []string{"d.d"}),
					},
				},
			},
			filters: LabelPartitions{
				"A": LabelPartition{Extensions: []string{".a"}},
				"B": LabelPartition{Extensions: []string{".b"}},
				"C": LabelPartition{Extensions: []string{".c"}},
			},
			expected: PartitionToLabelListAttribute{
				"A": LabelListAttribute{
					ConfigurableValues: configurableLabelLists{
						ArchConfigurationAxis: labelListSelectValues{
							"x86":    makeLabelList([]string{"a.a"}, []string{}),
							"x86_64": makeLabelList([]string{}, []string{"c.c"}),
						},
					},
				},
				"B": LabelListAttribute{
					ConfigurableValues: configurableLabelLists{
						ArchConfigurationAxis: labelListSelectValues{
							"arm":    makeLabelList([]string{"b.b"}, []string{}),
							"x86_64": makeLabelList([]string{"b.b"}, []string{"c.c"}),
						},
					},
				},
				"C": LabelListAttribute{
					ConfigurableValues: configurableLabelLists{
						ArchConfigurationAxis: labelListSelectValues{
							"x86":    makeLabelList([]string{"c.c"}, []string{}),
							"x86_64": makeLabelList([]string{}, []string{"c.c"}),
						},
					},
				},
			},
		},
		{
			name: "error for multiple partitions same value",
			ctx:  &OtherModuleTestContext{},
			labelList: LabelListAttribute{
				Value: makeLabelList([]string{"a.a", "b.b", "c.c", "d.d", "e.e"}, []string{}),
			},
			filters: LabelPartitions{
				"A":       LabelPartition{Extensions: []string{".a"}},
				"other A": LabelPartition{Extensions: []string{".a"}},
			},
			expected:       PartitionToLabelListAttribute{},
			expectedErrMsg: proptools.StringPtr(`"a.a" was found in multiple partitions:`),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := PartitionLabelListAttribute(tc.ctx, &tc.labelList, tc.filters)

			if hasErrors, expectsErr := len(tc.ctx.errors) > 0, tc.expectedErrMsg != nil; hasErrors != expectsErr {
				t.Errorf("Unexpected error(s): %q, expected: %q", tc.ctx.errors, *tc.expectedErrMsg)
			} else if tc.expectedErrMsg != nil {
				found := false
				for _, err := range tc.ctx.errors {
					if strings.Contains(err, *tc.expectedErrMsg) {
						found = true
						break
					}
				}

				if !found {
					t.Errorf("Expected error message: %q, got %q", *tc.expectedErrMsg, tc.ctx.errors)
				}
				return
			}

			if len(tc.expected) != len(got) {
				t.Errorf("Expected %d partitions, got %d partitions", len(tc.expected), len(got))
			}
			for partition, expectedLla := range tc.expected {
				gotLla, ok := got[partition]
				if !ok {
					t.Errorf("Expected partition %q, but it was not found %v", partition, got)
					continue
				}
				expectedLabelList := expectedLla.Value
				gotLabelList := gotLla.Value
				if !reflect.DeepEqual(expectedLabelList.Includes, gotLabelList.Includes) {
					t.Errorf("Expected no config includes %v, got %v", expectedLabelList.Includes, gotLabelList.Includes)
				}
				expectedAxes := expectedLla.SortedConfigurationAxes()
				gotAxes := gotLla.SortedConfigurationAxes()
				if !reflect.DeepEqual(expectedAxes, gotAxes) {
					t.Errorf("Expected axes %v, got %v (%#v)", expectedAxes, gotAxes, gotLla)
				}
				for _, axis := range expectedLla.SortedConfigurationAxes() {
					if _, exists := gotLla.ConfigurableValues[axis]; !exists {
						t.Errorf("Expected %s to be a supported axis, but it was not found", axis)
					}
					if expected, got := expectedLla.ConfigurableValues[axis], gotLla.ConfigurableValues[axis]; len(expected) != len(got) {
						t.Errorf("For axis %q: expected configs %v, got %v", axis, expected, got)
					}
					for config, expectedLabelList := range expectedLla.ConfigurableValues[axis] {
						gotLabelList, exists := gotLla.ConfigurableValues[axis][config]
						if !exists {
							t.Errorf("Expected %s to be a supported config, but config was not found", config)
							continue
						}
						if !reflect.DeepEqual(expectedLabelList.Includes, gotLabelList.Includes) {
							t.Errorf("Expected %s %s includes %v, got %v", axis, config, expectedLabelList.Includes, gotLabelList.Includes)
						}
					}
				}
			}
		})
	}
}

func TestDeduplicateAxesFromBase(t *testing.T) {
	attr := StringListAttribute{
		Value: []string{
			"all_include",
			"arm_include",
			"android_include",
			"linux_x86_include",
		},
		ConfigurableValues: configurableStringLists{
			ArchConfigurationAxis: stringListSelectValues{
				"arm": []string{"arm_include"},
				"x86": []string{"x86_include"},
			},
			OsConfigurationAxis: stringListSelectValues{
				"android": []string{"android_include"},
				"linux":   []string{"linux_include"},
			},
			OsArchConfigurationAxis: stringListSelectValues{
				"linux_x86": {"linux_x86_include"},
			},
			ProductVariableConfigurationAxis(false, "a"): stringListSelectValues{
				"a": []string{"not_in_value"},
			},
		},
	}

	attr.DeduplicateAxesFromBase()

	expectedBaseIncludes := []string{
		"all_include",
		"arm_include",
		"android_include",
		"linux_x86_include",
	}
	if !reflect.DeepEqual(expectedBaseIncludes, attr.Value) {
		t.Errorf("Expected Value includes %q, got %q", attr.Value, expectedBaseIncludes)
	}
	expectedConfiguredIncludes := configurableStringLists{
		ArchConfigurationAxis: stringListSelectValues{
			"x86": []string{"x86_include"},
		},
		OsConfigurationAxis: stringListSelectValues{
			"linux": []string{"linux_include"},
		},
		OsArchConfigurationAxis: stringListSelectValues{},
		ProductVariableConfigurationAxis(false, "a"): stringListSelectValues{
			"a": []string{"not_in_value"},
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
				if !reflect.DeepEqual(expected, value) {
					t.Errorf("For %s, expected: %#v, got %#v", axis, expected, value)
				}
			} else {
				t.Errorf("Got unexpected config %q for %s", config, axis)
			}
		}
	}
}
