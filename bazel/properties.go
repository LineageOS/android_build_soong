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
	"strings"
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

// Return all needles in a given haystack, where needleFn is true for needles.
func FilterLabelList(haystack LabelList, needleFn func(string) bool) LabelList {
	var includes []Label
	for _, inc := range haystack.Includes {
		if needleFn(inc.Label) {
			includes = append(includes, inc)
		}
	}
	return LabelList{Includes: includes, Excludes: haystack.Excludes}
}

// Return all needles in a given haystack, where needleFn is true for needles.
func FilterLabelListAttribute(haystack LabelListAttribute, needleFn func(string) bool) LabelListAttribute {
	var result LabelListAttribute

	result.Value = FilterLabelList(haystack.Value, needleFn)

	for arch := range PlatformArchMap {
		result.SetValueForArch(arch, FilterLabelList(haystack.GetValueForArch(arch), needleFn))
	}

	for os := range PlatformOsMap {
		result.SetOsValueForTarget(os, FilterLabelList(haystack.GetOsValueForTarget(os), needleFn))

		// TODO(b/187530594): Should we handle arch=CONDITIONS_DEFAULT here? (not in ArchValues)
		for _, arch := range AllArches {
			result.SetOsArchValueForTarget(os, arch, FilterLabelList(haystack.GetOsArchValueForTarget(os, arch), needleFn))
		}
	}

	return result
}

// Subtract needle from haystack
func SubtractBazelLabelListAttribute(haystack LabelListAttribute, needle LabelListAttribute) LabelListAttribute {
	var result LabelListAttribute

	for arch := range PlatformArchMap {
		result.SetValueForArch(arch,
			SubtractBazelLabelList(haystack.GetValueForArch(arch), needle.GetValueForArch(arch)))
	}

	for os := range PlatformOsMap {
		result.SetOsValueForTarget(os, SubtractBazelLabelList(haystack.GetOsValueForTarget(os), needle.GetOsValueForTarget(os)))

		// TODO(b/187530594): Should we handle arch=CONDITIONS_DEFAULT here? (not in ArchValues)
		for _, arch := range AllArches {
			result.SetOsArchValueForTarget(os, arch, SubtractBazelLabelList(haystack.GetOsArchValueForTarget(os, arch), needle.GetOsArchValueForTarget(os, arch)))
		}
	}

	result.Value = SubtractBazelLabelList(haystack.Value, needle.Value)

	return result
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

// Appends two LabelLists, returning the combined list.
func AppendBazelLabelLists(a LabelList, b LabelList) LabelList {
	var result LabelList
	result.Includes = append(a.Includes, b.Includes...)
	result.Excludes = append(a.Excludes, b.Excludes...)
	return result
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

	// Targets in arch.go
	TARGET_ANDROID_ARM         = "android_arm"
	TARGET_ANDROID_ARM64       = "android_arm64"
	TARGET_ANDROID_X86         = "android_x86"
	TARGET_ANDROID_X86_64      = "android_x86_64"
	TARGET_DARWIN_X86_64       = "darwin_x86_64"
	TARGET_FUCHSIA_ARM64       = "fuchsia_arm64"
	TARGET_FUCHSIA_X86_64      = "fuchsia_x86_64"
	TARGET_LINUX_X86           = "linux_glibc_x86"
	TARGET_LINUX_x86_64        = "linux_glibc_x86_64"
	TARGET_LINUX_BIONIC_ARM64  = "linux_bionic_arm64"
	TARGET_LINUX_BIONIC_X86_64 = "linux_bionic_x86_64"
	TARGET_WINDOWS_X86         = "windows_x86"
	TARGET_WINDOWS_X86_64      = "windows_x86_64"

	// This is the string representation of the default condition wherever a
	// configurable attribute is used in a select statement, i.e.
	// //conditions:default for Bazel.
	//
	// This is consistently named "conditions_default" to mirror the Soong
	// config variable default key in an Android.bp file, although there's no
	// integration with Soong config variables (yet).
	CONDITIONS_DEFAULT = "conditions_default"

	ConditionsDefaultSelectKey = "//conditions:default"

	productVariableBazelPackage = "//build/bazel/product_variables"
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
		CONDITIONS_DEFAULT: ConditionsDefaultSelectKey, // The default condition of as arch select map.
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
		CONDITIONS_DEFAULT: ConditionsDefaultSelectKey, // The default condition of an os select map.
	}

	PlatformTargetMap = map[string]string{
		TARGET_ANDROID_ARM:         "//build/bazel/platforms/os_arch:android_arm",
		TARGET_ANDROID_ARM64:       "//build/bazel/platforms/os_arch:android_arm64",
		TARGET_ANDROID_X86:         "//build/bazel/platforms/os_arch:android_x86",
		TARGET_ANDROID_X86_64:      "//build/bazel/platforms/os_arch:android_x86_64",
		TARGET_DARWIN_X86_64:       "//build/bazel/platforms/os_arch:darwin_x86_64",
		TARGET_FUCHSIA_ARM64:       "//build/bazel/platforms/os_arch:fuchsia_arm64",
		TARGET_FUCHSIA_X86_64:      "//build/bazel/platforms/os_arch:fuchsia_x86_64",
		TARGET_LINUX_X86:           "//build/bazel/platforms/os_arch:linux_glibc_x86",
		TARGET_LINUX_x86_64:        "//build/bazel/platforms/os_arch:linux_glibc_x86_64",
		TARGET_LINUX_BIONIC_ARM64:  "//build/bazel/platforms/os_arch:linux_bionic_arm64",
		TARGET_LINUX_BIONIC_X86_64: "//build/bazel/platforms/os_arch:linux_bionic_x86_64",
		TARGET_WINDOWS_X86:         "//build/bazel/platforms/os_arch:windows_x86",
		TARGET_WINDOWS_X86_64:      "//build/bazel/platforms/os_arch:windows_x86_64",
		CONDITIONS_DEFAULT:         ConditionsDefaultSelectKey, // The default condition of an os select map.
	}

	// TODO(b/187530594): Should we add CONDITIONS_DEFAULT here?
	AllArches = []string{ARCH_ARM, ARCH_ARM64, ARCH_X86, ARCH_X86_64}
)

type Attribute interface {
	HasConfigurableValues() bool
}

type labelArchValues struct {
	X86    Label
	X86_64 Label
	Arm    Label
	Arm64  Label

	ConditionsDefault Label
}

type labelTargetValue struct {
	// E.g. for android
	OsValue Label

	// E.g. for android_arm, android_arm64, ...
	ArchValues labelArchValues
}

type labelTargetValues struct {
	Android     labelTargetValue
	Darwin      labelTargetValue
	Fuchsia     labelTargetValue
	Linux       labelTargetValue
	LinuxBionic labelTargetValue
	Windows     labelTargetValue

	ConditionsDefault labelTargetValue
}

// Represents an attribute whose value is a single label
type LabelAttribute struct {
	Value Label

	ArchValues labelArchValues

	TargetValues labelTargetValues
}

func (attr *LabelAttribute) GetValueForArch(arch string) Label {
	var v *Label
	if v = attr.archValuePtrs()[arch]; v == nil {
		panic(fmt.Errorf("Unknown arch: %s", arch))
	}
	return *v
}

func (attr *LabelAttribute) SetValueForArch(arch string, value Label) {
	var v *Label
	if v = attr.archValuePtrs()[arch]; v == nil {
		panic(fmt.Errorf("Unknown arch: %s", arch))
	}
	*v = value
}

func (attr *LabelAttribute) archValuePtrs() map[string]*Label {
	return map[string]*Label{
		ARCH_X86:           &attr.ArchValues.X86,
		ARCH_X86_64:        &attr.ArchValues.X86_64,
		ARCH_ARM:           &attr.ArchValues.Arm,
		ARCH_ARM64:         &attr.ArchValues.Arm64,
		CONDITIONS_DEFAULT: &attr.ArchValues.ConditionsDefault,
	}
}

func (attr LabelAttribute) HasConfigurableValues() bool {
	for arch := range PlatformArchMap {
		if attr.GetValueForArch(arch).Label != "" {
			return true
		}
	}

	for os := range PlatformOsMap {
		if attr.GetOsValueForTarget(os).Label != "" {
			return true
		}
		// TODO(b/187530594): Should we also check arch=CONDITIONS_DEFAULT (not in AllArches)
		for _, arch := range AllArches {
			if attr.GetOsArchValueForTarget(os, arch).Label != "" {
				return true
			}
		}
	}
	return false
}

func (attr *LabelAttribute) getValueForTarget(os string) labelTargetValue {
	var v *labelTargetValue
	if v = attr.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	return *v
}

func (attr *LabelAttribute) GetOsValueForTarget(os string) Label {
	var v *labelTargetValue
	if v = attr.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	return v.OsValue
}

func (attr *LabelAttribute) GetOsArchValueForTarget(os string, arch string) Label {
	var v *labelTargetValue
	if v = attr.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	switch arch {
	case ARCH_X86:
		return v.ArchValues.X86
	case ARCH_X86_64:
		return v.ArchValues.X86_64
	case ARCH_ARM:
		return v.ArchValues.Arm
	case ARCH_ARM64:
		return v.ArchValues.Arm64
	case CONDITIONS_DEFAULT:
		return v.ArchValues.ConditionsDefault
	default:
		panic(fmt.Errorf("Unknown arch: %s\n", arch))
	}
}

func (attr *LabelAttribute) setValueForTarget(os string, value labelTargetValue) {
	var v *labelTargetValue
	if v = attr.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	*v = value
}

func (attr *LabelAttribute) SetOsValueForTarget(os string, value Label) {
	var v *labelTargetValue
	if v = attr.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	v.OsValue = value
}

func (attr *LabelAttribute) SetOsArchValueForTarget(os string, arch string, value Label) {
	var v *labelTargetValue
	if v = attr.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	switch arch {
	case ARCH_X86:
		v.ArchValues.X86 = value
	case ARCH_X86_64:
		v.ArchValues.X86_64 = value
	case ARCH_ARM:
		v.ArchValues.Arm = value
	case ARCH_ARM64:
		v.ArchValues.Arm64 = value
	case CONDITIONS_DEFAULT:
		v.ArchValues.ConditionsDefault = value
	default:
		panic(fmt.Errorf("Unknown arch: %s\n", arch))
	}
}

func (attr *LabelAttribute) targetValuePtrs() map[string]*labelTargetValue {
	return map[string]*labelTargetValue{
		OS_ANDROID:         &attr.TargetValues.Android,
		OS_DARWIN:          &attr.TargetValues.Darwin,
		OS_FUCHSIA:         &attr.TargetValues.Fuchsia,
		OS_LINUX:           &attr.TargetValues.Linux,
		OS_LINUX_BIONIC:    &attr.TargetValues.LinuxBionic,
		OS_WINDOWS:         &attr.TargetValues.Windows,
		CONDITIONS_DEFAULT: &attr.TargetValues.ConditionsDefault,
	}
}

// Arch-specific label_list typed Bazel attribute values. This should correspond
// to the types of architectures supported for compilation in arch.go.
type labelListArchValues struct {
	X86    LabelList
	X86_64 LabelList
	Arm    LabelList
	Arm64  LabelList

	ConditionsDefault LabelList
}

type labelListTargetValue struct {
	// E.g. for android
	OsValue LabelList

	// E.g. for android_arm, android_arm64, ...
	ArchValues labelListArchValues
}

func (target *labelListTargetValue) Append(other labelListTargetValue) {
	target.OsValue.Append(other.OsValue)
	target.ArchValues.X86.Append(other.ArchValues.X86)
	target.ArchValues.X86_64.Append(other.ArchValues.X86_64)
	target.ArchValues.Arm.Append(other.ArchValues.Arm)
	target.ArchValues.Arm64.Append(other.ArchValues.Arm64)
	target.ArchValues.ConditionsDefault.Append(other.ArchValues.ConditionsDefault)
}

type labelListTargetValues struct {
	Android     labelListTargetValue
	Darwin      labelListTargetValue
	Fuchsia     labelListTargetValue
	Linux       labelListTargetValue
	LinuxBionic labelListTargetValue
	Windows     labelListTargetValue

	ConditionsDefault labelListTargetValue
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
	TargetValues labelListTargetValues
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
		this := attrs.getValueForTarget(os)
		that := other.getValueForTarget(os)
		this.Append(that)
		attrs.setValueForTarget(os, this)
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
		if len(attrs.GetOsValueForTarget(os).Includes) > 0 {
			return true
		}
		// TODO(b/187530594): Should we also check arch=CONDITIONS_DEFAULT (not in AllArches)
		for _, arch := range AllArches {
			if len(attrs.GetOsArchValueForTarget(os, arch).Includes) > 0 {
				return true
			}
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

func (attrs *LabelListAttribute) targetValuePtrs() map[string]*labelListTargetValue {
	return map[string]*labelListTargetValue{
		OS_ANDROID:         &attrs.TargetValues.Android,
		OS_DARWIN:          &attrs.TargetValues.Darwin,
		OS_FUCHSIA:         &attrs.TargetValues.Fuchsia,
		OS_LINUX:           &attrs.TargetValues.Linux,
		OS_LINUX_BIONIC:    &attrs.TargetValues.LinuxBionic,
		OS_WINDOWS:         &attrs.TargetValues.Windows,
		CONDITIONS_DEFAULT: &attrs.TargetValues.ConditionsDefault,
	}
}

func (attrs *LabelListAttribute) getValueForTarget(os string) labelListTargetValue {
	var v *labelListTargetValue
	if v = attrs.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	return *v
}

func (attrs *LabelListAttribute) GetOsValueForTarget(os string) LabelList {
	var v *labelListTargetValue
	if v = attrs.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	return v.OsValue
}

func (attrs *LabelListAttribute) GetOsArchValueForTarget(os string, arch string) LabelList {
	var v *labelListTargetValue
	if v = attrs.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	switch arch {
	case ARCH_X86:
		return v.ArchValues.X86
	case ARCH_X86_64:
		return v.ArchValues.X86_64
	case ARCH_ARM:
		return v.ArchValues.Arm
	case ARCH_ARM64:
		return v.ArchValues.Arm64
	case CONDITIONS_DEFAULT:
		return v.ArchValues.ConditionsDefault
	default:
		panic(fmt.Errorf("Unknown arch: %s\n", arch))
	}
}

func (attrs *LabelListAttribute) setValueForTarget(os string, value labelListTargetValue) {
	var v *labelListTargetValue
	if v = attrs.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	*v = value
}

func (attrs *LabelListAttribute) SetOsValueForTarget(os string, value LabelList) {
	var v *labelListTargetValue
	if v = attrs.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	v.OsValue = value
}

func (attrs *LabelListAttribute) SetOsArchValueForTarget(os string, arch string, value LabelList) {
	var v *labelListTargetValue
	if v = attrs.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	switch arch {
	case ARCH_X86:
		v.ArchValues.X86 = value
	case ARCH_X86_64:
		v.ArchValues.X86_64 = value
	case ARCH_ARM:
		v.ArchValues.Arm = value
	case ARCH_ARM64:
		v.ArchValues.Arm64 = value
	case CONDITIONS_DEFAULT:
		v.ArchValues.ConditionsDefault = value
	default:
		panic(fmt.Errorf("Unknown arch: %s\n", arch))
	}
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
	TargetValues stringListTargetValues

	// list of product-variable string list values. Optional. if used, each will generate a select
	// statement appended to the label list Value.
	ProductValues []ProductVariableValues
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

	ConditionsDefault []string
}

type stringListTargetValue struct {
	// E.g. for android
	OsValue []string

	// E.g. for android_arm, android_arm64, ...
	ArchValues stringListArchValues
}

func (target *stringListTargetValue) Append(other stringListTargetValue) {
	target.OsValue = append(target.OsValue, other.OsValue...)
	target.ArchValues.X86 = append(target.ArchValues.X86, other.ArchValues.X86...)
	target.ArchValues.X86_64 = append(target.ArchValues.X86_64, other.ArchValues.X86_64...)
	target.ArchValues.Arm = append(target.ArchValues.Arm, other.ArchValues.Arm...)
	target.ArchValues.Arm64 = append(target.ArchValues.Arm64, other.ArchValues.Arm64...)
	target.ArchValues.ConditionsDefault = append(target.ArchValues.ConditionsDefault, other.ArchValues.ConditionsDefault...)
}

type stringListTargetValues struct {
	Android     stringListTargetValue
	Darwin      stringListTargetValue
	Fuchsia     stringListTargetValue
	Linux       stringListTargetValue
	LinuxBionic stringListTargetValue
	Windows     stringListTargetValue

	ConditionsDefault stringListTargetValue
}

// Product Variable values for StringListAttribute
type ProductVariableValues struct {
	ProductVariable string

	Values []string
}

// SelectKey returns the appropriate select key for the receiving ProductVariableValues.
func (p ProductVariableValues) SelectKey() string {
	return fmt.Sprintf("%s:%s", productVariableBazelPackage, strings.ToLower(p.ProductVariable))
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
		if len(attrs.GetOsValueForTarget(os)) > 0 {
			return true
		}
		// TODO(b/187530594): Should we also check arch=CONDITIONS_DEFAULT? (Not in AllArches)
		for _, arch := range AllArches {
			if len(attrs.GetOsArchValueForTarget(os, arch)) > 0 {
				return true
			}

		}
	}

	return len(attrs.ProductValues) > 0
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

func (attrs *StringListAttribute) targetValuePtrs() map[string]*stringListTargetValue {
	return map[string]*stringListTargetValue{
		OS_ANDROID:         &attrs.TargetValues.Android,
		OS_DARWIN:          &attrs.TargetValues.Darwin,
		OS_FUCHSIA:         &attrs.TargetValues.Fuchsia,
		OS_LINUX:           &attrs.TargetValues.Linux,
		OS_LINUX_BIONIC:    &attrs.TargetValues.LinuxBionic,
		OS_WINDOWS:         &attrs.TargetValues.Windows,
		CONDITIONS_DEFAULT: &attrs.TargetValues.ConditionsDefault,
	}
}

func (attrs *StringListAttribute) getValueForTarget(os string) stringListTargetValue {
	var v *stringListTargetValue
	if v = attrs.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	return *v
}

func (attrs *StringListAttribute) GetOsValueForTarget(os string) []string {
	var v *stringListTargetValue
	if v = attrs.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	return v.OsValue
}

func (attrs *StringListAttribute) GetOsArchValueForTarget(os string, arch string) []string {
	var v *stringListTargetValue
	if v = attrs.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	switch arch {
	case ARCH_X86:
		return v.ArchValues.X86
	case ARCH_X86_64:
		return v.ArchValues.X86_64
	case ARCH_ARM:
		return v.ArchValues.Arm
	case ARCH_ARM64:
		return v.ArchValues.Arm64
	case CONDITIONS_DEFAULT:
		return v.ArchValues.ConditionsDefault
	default:
		panic(fmt.Errorf("Unknown arch: %s\n", arch))
	}
}

func (attrs *StringListAttribute) setValueForTarget(os string, value stringListTargetValue) {
	var v *stringListTargetValue
	if v = attrs.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	*v = value
}

func (attrs *StringListAttribute) SortedProductVariables() []ProductVariableValues {
	vals := attrs.ProductValues[:]
	sort.Slice(vals, func(i, j int) bool { return vals[i].ProductVariable < vals[j].ProductVariable })
	return vals
}

func (attrs *StringListAttribute) SetOsValueForTarget(os string, value []string) {
	var v *stringListTargetValue
	if v = attrs.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	v.OsValue = value
}

func (attrs *StringListAttribute) SetOsArchValueForTarget(os string, arch string, value []string) {
	var v *stringListTargetValue
	if v = attrs.targetValuePtrs()[os]; v == nil {
		panic(fmt.Errorf("Unknown os: %s", os))
	}
	switch arch {
	case ARCH_X86:
		v.ArchValues.X86 = value
	case ARCH_X86_64:
		v.ArchValues.X86_64 = value
	case ARCH_ARM:
		v.ArchValues.Arm = value
	case ARCH_ARM64:
		v.ArchValues.Arm64 = value
	case CONDITIONS_DEFAULT:
		v.ArchValues.ConditionsDefault = value
	default:
		panic(fmt.Errorf("Unknown arch: %s\n", arch))
	}
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
		this := attrs.getValueForTarget(os)
		that := other.getValueForTarget(os)
		this.Append(that)
		attrs.setValueForTarget(os, this)
	}

	productValues := make(map[string][]string, 0)
	for _, pv := range attrs.ProductValues {
		productValues[pv.ProductVariable] = pv.Values
	}
	for _, pv := range other.ProductValues {
		productValues[pv.ProductVariable] = append(productValues[pv.ProductVariable], pv.Values...)
	}
	attrs.ProductValues = make([]ProductVariableValues, 0, len(productValues))
	for pv, vals := range productValues {
		attrs.ProductValues = append(attrs.ProductValues, ProductVariableValues{
			ProductVariable: pv,
			Values:          vals,
		})
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
	sub := productVariableSubstitutionPattern.ReplaceAllString(s, "$("+productVariable+")")
	return sub, s != sub
}
