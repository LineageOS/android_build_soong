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
	"path/filepath"
	"regexp"
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

var productVariableSubstitutionPattern = regexp.MustCompile("%(d|s)")

// Label is used to represent a Bazel compatible Label. Also stores the original
// bp text to support string replacement.
type Label struct {
	// The string representation of a Bazel target label. This can be a relative
	// or fully qualified label. These labels are used for generating BUILD
	// files with bp2build.
	Label string

	// The original Soong/Blueprint module name that the label was derived from.
	// This is used for replacing references to the original name with the new
	// label, for example in genrule cmds.
	//
	// While there is a reversible 1:1 mapping from the module name to Bazel
	// label with bp2build that could make computing the original module name
	// from the label automatic, it is not the case for handcrafted targets,
	// where modules can have a custom label mapping through the { bazel_module:
	// { label: <label> } } property.
	//
	// With handcrafted labels, those modules don't go through bp2build
	// conversion, but relies on handcrafted targets in the source tree.
	OriginalModuleName string
}

// LabelList is used to represent a list of Bazel labels.
type LabelList struct {
	Includes []Label
	Excludes []Label
}

// uniqueParentDirectories returns a list of the unique parent directories for
// all files in ll.Includes.
func (ll *LabelList) uniqueParentDirectories() []string {
	dirMap := map[string]bool{}
	for _, label := range ll.Includes {
		dirMap[filepath.Dir(label.Label)] = true
	}
	dirs := []string{}
	for dir := range dirMap {
		dirs = append(dirs, dir)
	}
	return dirs
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

// UniqueSortedBazelLabels takes a []Label and deduplicates the labels, and returns
// the slice in a sorted order.
func UniqueSortedBazelLabels(originalLabels []Label) []Label {
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
	uniqueLabelList.Includes = UniqueSortedBazelLabels(originalLabelList.Includes)
	uniqueLabelList.Excludes = UniqueSortedBazelLabels(originalLabelList.Excludes)
	return uniqueLabelList
}

// Subtract needle from haystack
func SubtractStrings(haystack []string, needle []string) []string {
	// This is really a set
	remainder := make(map[string]bool)

	for _, s := range haystack {
		remainder[s] = true
	}
	for _, s := range needle {
		delete(remainder, s)
	}

	var strings []string
	for s, _ := range remainder {
		strings = append(strings, s)
	}

	sort.SliceStable(strings, func(i, j int) bool {
		return strings[i] < strings[j]
	})

	return strings
}

// Subtract needle from haystack
func SubtractBazelLabels(haystack []Label, needle []Label) []Label {
	// This is really a set
	remainder := make(map[Label]bool)

	for _, label := range haystack {
		remainder[label] = true
	}
	for _, label := range needle {
		delete(remainder, label)
	}

	var labels []Label
	for label, _ := range remainder {
		labels = append(labels, label)
	}

	sort.SliceStable(labels, func(i, j int) bool {
		return labels[i].Label < labels[j].Label
	})

	return labels
}

// Subtract needle from haystack
func SubtractBazelLabelList(haystack LabelList, needle LabelList) LabelList {
	var result LabelList
	result.Includes = SubtractBazelLabels(haystack.Includes, needle.Includes)
	// NOTE: Excludes are intentionally not subtracted
	result.Excludes = haystack.Excludes
	return result
}

const (
	// ArchType names in arch.go
	ARCH_ARM    = "arm"
	ARCH_ARM64  = "arm64"
	ARCH_X86    = "x86"
	ARCH_X86_64 = "x86_64"

	// OsType names in arch.go
	OS_ANDROID      = "android"
	OS_DARWIN       = "darwin"
	OS_FUCHSIA      = "fuchsia"
	OS_LINUX        = "linux_glibc"
	OS_LINUX_BIONIC = "linux_bionic"
	OS_WINDOWS      = "windows"

	// This is the string representation of the default condition wherever a
	// configurable attribute is used in a select statement, i.e.
	// //conditions:default for Bazel.
	//
	// This is consistently named "conditions_default" to mirror the Soong
	// config variable default key in an Android.bp file, although there's no
	// integration with Soong config variables (yet).
	CONDITIONS_DEFAULT = "conditions_default"
)

var (
	// These are the list of OSes and architectures with a Bazel config_setting
	// and constraint value equivalent. These exist in arch.go, but the android
	// package depends on the bazel package, so a cyclic dependency prevents
	// using those variables here.

	// A map of architectures to the Bazel label of the constraint_value
	// for the @platforms//cpu:cpu constraint_setting
	PlatformArchMap = map[string]string{
		ARCH_ARM:           "//build/bazel/platforms/arch:arm",
		ARCH_ARM64:         "//build/bazel/platforms/arch:arm64",
		ARCH_X86:           "//build/bazel/platforms/arch:x86",
		ARCH_X86_64:        "//build/bazel/platforms/arch:x86_64",
		CONDITIONS_DEFAULT: "//conditions:default", // The default condition of as arch select map.
	}

	// A map of target operating systems to the Bazel label of the
	// constraint_value for the @platforms//os:os constraint_setting
	PlatformOsMap = map[string]string{
		OS_ANDROID:         "//build/bazel/platforms/os:android",
		OS_DARWIN:          "//build/bazel/platforms/os:darwin",
		OS_FUCHSIA:         "//build/bazel/platforms/os:fuchsia",
		OS_LINUX:           "//build/bazel/platforms/os:linux",
		OS_LINUX_BIONIC:    "//build/bazel/platforms/os:linux_bionic",
		OS_WINDOWS:         "//build/bazel/platforms/os:windows",
		CONDITIONS_DEFAULT: "//conditions:default", // The default condition of an os select map.
	}
)

type Attribute interface {
	HasConfigurableValues() bool
}

// Represents an attribute whose value is a single label
type LabelAttribute struct {
	Value  Label
	X86    Label
	X86_64 Label
	Arm    Label
	Arm64  Label
}

func (attr *LabelAttribute) GetValueForArch(arch string) Label {
	switch arch {
	case ARCH_ARM:
		return attr.Arm
	case ARCH_ARM64:
		return attr.Arm64
	case ARCH_X86:
		return attr.X86
	case ARCH_X86_64:
		return attr.X86_64
	case CONDITIONS_DEFAULT:
		return attr.Value
	default:
		panic("Invalid arch type")
	}
}

func (attr *LabelAttribute) SetValueForArch(arch string, value Label) {
	switch arch {
	case ARCH_ARM:
		attr.Arm = value
	case ARCH_ARM64:
		attr.Arm64 = value
	case ARCH_X86:
		attr.X86 = value
	case ARCH_X86_64:
		attr.X86_64 = value
	default:
		panic("Invalid arch type")
	}
}

func (attr LabelAttribute) HasConfigurableValues() bool {
	return attr.Arm.Label != "" || attr.Arm64.Label != "" || attr.X86.Label != "" || attr.X86_64.Label != ""
}

// Arch-specific label_list typed Bazel attribute values. This should correspond
// to the types of architectures supported for compilation in arch.go.
type labelListArchValues struct {
	X86    LabelList
	X86_64 LabelList
	Arm    LabelList
	Arm64  LabelList
	Common LabelList

	ConditionsDefault LabelList
}

type labelListOsValues struct {
	Android     LabelList
	Darwin      LabelList
	Fuchsia     LabelList
	Linux       LabelList
	LinuxBionic LabelList
	Windows     LabelList

	ConditionsDefault LabelList
}

// LabelListAttribute is used to represent a list of Bazel labels as an
// attribute.
type LabelListAttribute struct {
	// The non-arch specific attribute label list Value. Required.
	Value LabelList

	// The arch-specific attribute label list values. Optional. If used, these
	// are generated in a select statement and appended to the non-arch specific
	// label list Value.
	ArchValues labelListArchValues

	// The os-specific attribute label list values. Optional. If used, these
	// are generated in a select statement and appended to the non-os specific
	// label list Value.
	OsValues labelListOsValues
}

// MakeLabelListAttribute initializes a LabelListAttribute with the non-arch specific value.
func MakeLabelListAttribute(value LabelList) LabelListAttribute {
	return LabelListAttribute{Value: UniqueBazelLabelList(value)}
}

// Append all values, including os and arch specific ones, from another
// LabelListAttribute to this LabelListAttribute.
func (attrs *LabelListAttribute) Append(other LabelListAttribute) {
	for arch := range PlatformArchMap {
		this := attrs.GetValueForArch(arch)
		that := other.GetValueForArch(arch)
		this.Append(that)
		attrs.SetValueForArch(arch, this)
	}

	for os := range PlatformOsMap {
		this := attrs.GetValueForOS(os)
		that := other.GetValueForOS(os)
		this.Append(that)
		attrs.SetValueForOS(os, this)
	}

	attrs.Value.Append(other.Value)
}

// HasArchSpecificValues returns true if the attribute contains
// architecture-specific label_list values.
func (attrs LabelListAttribute) HasConfigurableValues() bool {
	for arch := range PlatformArchMap {
		if len(attrs.GetValueForArch(arch).Includes) > 0 {
			return true
		}
	}

	for os := range PlatformOsMap {
		if len(attrs.GetValueForOS(os).Includes) > 0 {
			return true
		}
	}
	return false
}

func (attrs *LabelListAttribute) archValuePtrs() map[string]*LabelList {
	return map[string]*LabelList{
		ARCH_X86:           &attrs.ArchValues.X86,
		ARCH_X86_64:        &attrs.ArchValues.X86_64,
		ARCH_ARM:           &attrs.ArchValues.Arm,
		ARCH_ARM64:         &attrs.ArchValues.Arm64,
		CONDITIONS_DEFAULT: &attrs.ArchValues.ConditionsDefault,
	}
}

// GetValueForArch returns the label_list attribute value for an architecture.
func (attrs *LabelListAttribute) GetValueForArch(arch string) LabelList {
	var v *LabelList
	if v = attrs.archValuePtrs()[arch]; v == nil {
		panic(fmt.Errorf("Unknown arch: %s", arch))
	}
	return *v
}

// SetValueForArch sets the label_list attribute value for an architecture.
func (attrs *LabelListAttribute) SetValueForArch(arch string, value LabelList) {
	var v *LabelList
	if v = attrs.archValuePtrs()[arch]; v == nil {
		panic(fmt.Errorf("Unknown arch: %s", arch))
	}
	*v = value
}

func (attrs *LabelListAttribute) osValuePtrs() map[string]*LabelList {
	return map[string]*LabelList{
		OS_ANDROID:         &attrs.OsValues.Android,
		OS_DARWIN:          &attrs.OsValues.Darwin,
		OS_FUCHSIA:         &attrs.OsValues.Fuchsia,
		OS_LINUX:           &attrs.OsValues.Linux,
		OS_LINUX_BIONIC:    &attrs.OsValues.LinuxBionic,
		OS_WINDOWS:         &attrs.OsValues.Windows,
		CONDITIONS_DEFAULT: &attrs.OsValues.ConditionsDefault,
	}
}

// GetValueForOS returns the label_list attribute value for an OS target.
func (attrs *LabelListAttribute) GetValueForOS(os string) LabelList {
	var v *LabelList
	if v = attrs.osValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	return *v
}

// SetValueForArch sets the label_list attribute value for an OS target.
func (attrs *LabelListAttribute) SetValueForOS(os string, value LabelList) {
	var v *LabelList
	if v = attrs.osValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	*v = value
}

// StringListAttribute corresponds to the string_list Bazel attribute type with
// support for additional metadata, like configurations.
type StringListAttribute struct {
	// The base value of the string list attribute.
	Value []string

	// The arch-specific attribute string list values. Optional. If used, these
	// are generated in a select statement and appended to the non-arch specific
	// label list Value.
	ArchValues stringListArchValues

	// The os-specific attribute string list values. Optional. If used, these
	// are generated in a select statement and appended to the non-os specific
	// label list Value.
	OsValues stringListOsValues
}

// MakeStringListAttribute initializes a StringListAttribute with the non-arch specific value.
func MakeStringListAttribute(value []string) StringListAttribute {
	// NOTE: These strings are not necessarily unique or sorted.
	return StringListAttribute{Value: value}
}

// Arch-specific string_list typed Bazel attribute values. This should correspond
// to the types of architectures supported for compilation in arch.go.
type stringListArchValues struct {
	X86    []string
	X86_64 []string
	Arm    []string
	Arm64  []string
	Common []string

	ConditionsDefault []string
}

type stringListOsValues struct {
	Android     []string
	Darwin      []string
	Fuchsia     []string
	Linux       []string
	LinuxBionic []string
	Windows     []string

	ConditionsDefault []string
}

// HasConfigurableValues returns true if the attribute contains
// architecture-specific string_list values.
func (attrs StringListAttribute) HasConfigurableValues() bool {
	for arch := range PlatformArchMap {
		if len(attrs.GetValueForArch(arch)) > 0 {
			return true
		}
	}

	for os := range PlatformOsMap {
		if len(attrs.GetValueForOS(os)) > 0 {
			return true
		}
	}
	return false
}

func (attrs *StringListAttribute) archValuePtrs() map[string]*[]string {
	return map[string]*[]string{
		ARCH_X86:           &attrs.ArchValues.X86,
		ARCH_X86_64:        &attrs.ArchValues.X86_64,
		ARCH_ARM:           &attrs.ArchValues.Arm,
		ARCH_ARM64:         &attrs.ArchValues.Arm64,
		CONDITIONS_DEFAULT: &attrs.ArchValues.ConditionsDefault,
	}
}

// GetValueForArch returns the string_list attribute value for an architecture.
func (attrs *StringListAttribute) GetValueForArch(arch string) []string {
	var v *[]string
	if v = attrs.archValuePtrs()[arch]; v == nil {
		panic(fmt.Errorf("Unknown arch: %s", arch))
	}
	return *v
}

// SetValueForArch sets the string_list attribute value for an architecture.
func (attrs *StringListAttribute) SetValueForArch(arch string, value []string) {
	var v *[]string
	if v = attrs.archValuePtrs()[arch]; v == nil {
		panic(fmt.Errorf("Unknown arch: %s", arch))
	}
	*v = value
}

func (attrs *StringListAttribute) osValuePtrs() map[string]*[]string {
	return map[string]*[]string{
		OS_ANDROID:         &attrs.OsValues.Android,
		OS_DARWIN:          &attrs.OsValues.Darwin,
		OS_FUCHSIA:         &attrs.OsValues.Fuchsia,
		OS_LINUX:           &attrs.OsValues.Linux,
		OS_LINUX_BIONIC:    &attrs.OsValues.LinuxBionic,
		OS_WINDOWS:         &attrs.OsValues.Windows,
		CONDITIONS_DEFAULT: &attrs.OsValues.ConditionsDefault,
	}
}

// GetValueForOS returns the string_list attribute value for an OS target.
func (attrs *StringListAttribute) GetValueForOS(os string) []string {
	var v *[]string
	if v = attrs.osValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	return *v
}

// SetValueForArch sets the string_list attribute value for an OS target.
func (attrs *StringListAttribute) SetValueForOS(os string, value []string) {
	var v *[]string
	if v = attrs.osValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	*v = value
}

// Append appends all values, including os and arch specific ones, from another
// StringListAttribute to this StringListAttribute
func (attrs *StringListAttribute) Append(other StringListAttribute) {
	for arch := range PlatformArchMap {
		this := attrs.GetValueForArch(arch)
		that := other.GetValueForArch(arch)
		this = append(this, that...)
		attrs.SetValueForArch(arch, this)
	}

	for os := range PlatformOsMap {
		this := attrs.GetValueForOS(os)
		that := other.GetValueForOS(os)
		this = append(this, that...)
		attrs.SetValueForOS(os, this)
	}

	attrs.Value = append(attrs.Value, other.Value...)
}

// TryVariableSubstitution, replace string substitution formatting within each string in slice with
// Starlark string.format compatible tag for productVariable.
func TryVariableSubstitutions(slice []string, productVariable string) ([]string, bool) {
	ret := make([]string, 0, len(slice))
	changesMade := false
	for _, s := range slice {
		newS, changed := TryVariableSubstitution(s, productVariable)
		ret = append(ret, newS)
		changesMade = changesMade || changed
	}
	return ret, changesMade
}

// TryVariableSubstitution, replace string substitution formatting within s with Starlark
// string.format compatible tag for productVariable.
func TryVariableSubstitution(s string, productVariable string) (string, bool) {
	sub := productVariableSubstitutionPattern.ReplaceAllString(s, "{"+productVariable+"}")
	return sub, s != sub
}
