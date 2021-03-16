// Copyright 2020 Google Inc. All rights reserved.
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
	"fmt"
	"sort"
)

// BazelTargetModuleProperties contain properties and metadata used for
// Blueprint to BUILD file conversion.
type BazelTargetModuleProperties struct {
	// The Bazel rule class for this target.
	Rule_class string `blueprint:"mutated"`

	// The target label for the bzl file containing the definition of the rule class.
	Bzl_load_location string `blueprint:"mutated"`
}

const BazelTargetModuleNamePrefix = "__bp2build__"

// Label is used to represent a Bazel compatible Label. Also stores the original bp text to support
// string replacement.
type Label struct {
	Bp_text string
	Label   string
}

// LabelList is used to represent a list of Bazel labels.
type LabelList struct {
	Includes []Label
	Excludes []Label
}

// Append appends the fields of other labelList to the corresponding fields of ll.
func (ll *LabelList) Append(other LabelList) {
	if len(ll.Includes) > 0 || len(other.Includes) > 0 {
		ll.Includes = append(ll.Includes, other.Includes...)
	}
	if len(ll.Excludes) > 0 || len(other.Excludes) > 0 {
		ll.Excludes = append(other.Excludes, other.Excludes...)
	}
}

func UniqueBazelLabels(originalLabels []Label) []Label {
	uniqueLabelsSet := make(map[Label]bool)
	for _, l := range originalLabels {
		uniqueLabelsSet[l] = true
	}
	var uniqueLabels []Label
	for l, _ := range uniqueLabelsSet {
		uniqueLabels = append(uniqueLabels, l)
	}
	sort.SliceStable(uniqueLabels, func(i, j int) bool {
		return uniqueLabels[i].Label < uniqueLabels[j].Label
	})
	return uniqueLabels
}

func UniqueBazelLabelList(originalLabelList LabelList) LabelList {
	var uniqueLabelList LabelList
	uniqueLabelList.Includes = UniqueBazelLabels(originalLabelList.Includes)
	uniqueLabelList.Excludes = UniqueBazelLabels(originalLabelList.Excludes)
	return uniqueLabelList
}

// StringListAttribute corresponds to the string_list Bazel attribute type with
// support for additional metadata, like configurations.
type StringListAttribute struct {
	// The base value of the string list attribute.
	Value []string

	// Optional additive set of list values to the base value.
	ArchValues stringListArchValues
}

// Arch-specific string_list typed Bazel attribute values. This should correspond
// to the types of architectures supported for compilation in arch.go.
type stringListArchValues struct {
	X86     []string
	X86_64  []string
	Arm     []string
	Arm64   []string
	Default []string
	// TODO(b/181299724): this is currently missing the "common" arch, which
	// doesn't have an equivalent platform() definition yet.
}

// HasArchSpecificValues returns true if the attribute contains
// architecture-specific string_list values.
func (attrs *StringListAttribute) HasArchSpecificValues() bool {
	for _, arch := range []string{"x86", "x86_64", "arm", "arm64", "default"} {
		if len(attrs.GetValueForArch(arch)) > 0 {
			return true
		}
	}
	return false
}

// GetValueForArch returns the string_list attribute value for an architecture.
func (attrs *StringListAttribute) GetValueForArch(arch string) []string {
	switch arch {
	case "x86":
		return attrs.ArchValues.X86
	case "x86_64":
		return attrs.ArchValues.X86_64
	case "arm":
		return attrs.ArchValues.Arm
	case "arm64":
		return attrs.ArchValues.Arm64
	case "default":
		return attrs.ArchValues.Default
	default:
		panic(fmt.Errorf("Unknown arch: %s", arch))
	}
}

// SetValueForArch sets the string_list attribute value for an architecture.
func (attrs *StringListAttribute) SetValueForArch(arch string, value []string) {
	switch arch {
	case "x86":
		attrs.ArchValues.X86 = value
	case "x86_64":
		attrs.ArchValues.X86_64 = value
	case "arm":
		attrs.ArchValues.Arm = value
	case "arm64":
		attrs.ArchValues.Arm64 = value
	case "default":
		attrs.ArchValues.Default = value
	default:
		panic(fmt.Errorf("Unknown arch: %s", arch))
	}
}
