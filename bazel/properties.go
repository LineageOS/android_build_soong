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

func (ll *LabelList) IsNil() bool {
	return ll.Includes == nil && ll.Excludes == nil
}

func (ll *LabelList) deepCopy() LabelList {
	return LabelList{
		Includes: ll.Includes[:],
		Excludes: ll.Excludes[:],
	}
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

func FirstUniqueBazelLabels(originalLabels []Label) []Label {
	var labels []Label
	found := make(map[Label]bool, len(originalLabels))
	for _, l := range originalLabels {
		if _, ok := found[l]; ok {
			continue
		}
		labels = append(labels, l)
		found[l] = true
	}
	return labels
}

func FirstUniqueBazelLabelList(originalLabelList LabelList) LabelList {
	var uniqueLabelList LabelList
	uniqueLabelList.Includes = FirstUniqueBazelLabels(originalLabelList.Includes)
	uniqueLabelList.Excludes = FirstUniqueBazelLabels(originalLabelList.Excludes)
	return uniqueLabelList
}

func UniqueSortedBazelLabelList(originalLabelList LabelList) LabelList {
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

// Map a function over all labels in a LabelList.
func MapLabelList(mapOver LabelList, mapFn func(string) string) LabelList {
	var includes []Label
	for _, inc := range mapOver.Includes {
		mappedLabel := Label{Label: mapFn(inc.Label), OriginalModuleName: inc.OriginalModuleName}
		includes = append(includes, mappedLabel)
	}
	// mapFn is not applied over excludes, but they are propagated as-is.
	return LabelList{Includes: includes, Excludes: mapOver.Excludes}
}

// Map a function over all Labels in a LabelListAttribute
func MapLabelListAttribute(mapOver LabelListAttribute, mapFn func(string) string) LabelListAttribute {
	var result LabelListAttribute

	result.Value = MapLabelList(mapOver.Value, mapFn)

	for axis, configToLabels := range mapOver.ConfigurableValues {
		for config, value := range configToLabels {
			result.SetSelectValue(axis, config, MapLabelList(value, mapFn))
		}
	}

	return result
}

// Return all needles in a given haystack, where needleFn is true for needles.
func FilterLabelList(haystack LabelList, needleFn func(string) bool) LabelList {
	var includes []Label
	for _, inc := range haystack.Includes {
		if needleFn(inc.Label) {
			includes = append(includes, inc)
		}
	}
	// needleFn is not applied over excludes, but they are propagated as-is.
	return LabelList{Includes: includes, Excludes: haystack.Excludes}
}

// Return all needles in a given haystack, where needleFn is true for needles.
func FilterLabelListAttribute(haystack LabelListAttribute, needleFn func(string) bool) LabelListAttribute {
	result := MakeLabelListAttribute(FilterLabelList(haystack.Value, needleFn))

	for config, selects := range haystack.ConfigurableValues {
		newSelects := make(labelListSelectValues, len(selects))
		for k, v := range selects {
			newSelects[k] = FilterLabelList(v, needleFn)
		}
		result.ConfigurableValues[config] = newSelects
	}

	return result
}

// Subtract needle from haystack
func SubtractBazelLabelListAttribute(haystack LabelListAttribute, needle LabelListAttribute) LabelListAttribute {
	result := MakeLabelListAttribute(SubtractBazelLabelList(haystack.Value, needle.Value))

	for config, selects := range haystack.ConfigurableValues {
		newSelects := make(labelListSelectValues, len(selects))
		needleSelects := needle.ConfigurableValues[config]

		for k, v := range selects {
			newSelects[k] = SubtractBazelLabelList(v, needleSelects[k])
		}
		result.ConfigurableValues[config] = newSelects
	}

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

type Attribute interface {
	HasConfigurableValues() bool
}

type labelSelectValues map[string]*Label

type configurableLabels map[ConfigurationAxis]labelSelectValues

func (cl configurableLabels) setValueForAxis(axis ConfigurationAxis, config string, value *Label) {
	if cl[axis] == nil {
		cl[axis] = make(labelSelectValues)
	}
	cl[axis][config] = value
}

// Represents an attribute whose value is a single label
type LabelAttribute struct {
	Value *Label

	ConfigurableValues configurableLabels
}

// HasConfigurableValues returns whether there are configurable values set for this label.
func (la LabelAttribute) HasConfigurableValues() bool {
	return len(la.ConfigurableValues) > 0
}

// SetValue sets the base, non-configured value for the Label
func (la *LabelAttribute) SetValue(value Label) {
	la.SetSelectValue(NoConfigAxis, "", value)
}

// SetSelectValue set a value for a bazel select for the given axis, config and value.
func (la *LabelAttribute) SetSelectValue(axis ConfigurationAxis, config string, value Label) {
	axis.validateConfig(config)
	switch axis.configurationType {
	case noConfig:
		la.Value = &value
	case arch, os, osArch, productVariables:
		if la.ConfigurableValues == nil {
			la.ConfigurableValues = make(configurableLabels)
		}
		la.ConfigurableValues.setValueForAxis(axis, config, &value)
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationAxis %s", axis))
	}
}

// SelectValue gets a value for a bazel select for the given axis and config.
func (la *LabelAttribute) SelectValue(axis ConfigurationAxis, config string) Label {
	axis.validateConfig(config)
	switch axis.configurationType {
	case noConfig:
		return *la.Value
	case arch, os, osArch, productVariables:
		return *la.ConfigurableValues[axis][config]
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationAxis %s", axis))
	}
}

// SortedConfigurationAxes returns all the used ConfigurationAxis in sorted order.
func (la *LabelAttribute) SortedConfigurationAxes() []ConfigurationAxis {
	keys := make([]ConfigurationAxis, 0, len(la.ConfigurableValues))
	for k := range la.ConfigurableValues {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i].less(keys[j]) })
	return keys
}

type configToBools map[string]bool

func (ctb configToBools) setValue(config string, value *bool) {
	if value == nil {
		if _, ok := ctb[config]; ok {
			delete(ctb, config)
		}
		return
	}
	ctb[config] = *value
}

type configurableBools map[ConfigurationAxis]configToBools

func (cb configurableBools) setValueForAxis(axis ConfigurationAxis, config string, value *bool) {
	if cb[axis] == nil {
		cb[axis] = make(configToBools)
	}
	cb[axis].setValue(config, value)
}

// BoolAttribute represents an attribute whose value is a single bool but may be configurable..
type BoolAttribute struct {
	Value *bool

	ConfigurableValues configurableBools
}

// HasConfigurableValues returns whether there are configurable values for this attribute.
func (ba BoolAttribute) HasConfigurableValues() bool {
	return len(ba.ConfigurableValues) > 0
}

// SetSelectValue sets value for the given axis/config.
func (ba *BoolAttribute) SetSelectValue(axis ConfigurationAxis, config string, value *bool) {
	axis.validateConfig(config)
	switch axis.configurationType {
	case noConfig:
		ba.Value = value
	case arch, os, osArch, productVariables:
		if ba.ConfigurableValues == nil {
			ba.ConfigurableValues = make(configurableBools)
		}
		ba.ConfigurableValues.setValueForAxis(axis, config, value)
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationAxis %s", axis))
	}
}

// SelectValue gets the value for the given axis/config.
func (ba BoolAttribute) SelectValue(axis ConfigurationAxis, config string) *bool {
	axis.validateConfig(config)
	switch axis.configurationType {
	case noConfig:
		return ba.Value
	case arch, os, osArch, productVariables:
		if v, ok := ba.ConfigurableValues[axis][config]; ok {
			return &v
		} else {
			return nil
		}
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationAxis %s", axis))
	}
}

// SortedConfigurationAxes returns all the used ConfigurationAxis in sorted order.
func (ba *BoolAttribute) SortedConfigurationAxes() []ConfigurationAxis {
	keys := make([]ConfigurationAxis, 0, len(ba.ConfigurableValues))
	for k := range ba.ConfigurableValues {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i].less(keys[j]) })
	return keys
}

// labelListSelectValues supports config-specific label_list typed Bazel attribute values.
type labelListSelectValues map[string]LabelList

func (ll labelListSelectValues) appendSelects(other labelListSelectValues) {
	for k, v := range other {
		l := ll[k]
		(&l).Append(v)
		ll[k] = l
	}
}

// HasConfigurableValues returns whether there are configurable values within this set of selects.
func (ll labelListSelectValues) HasConfigurableValues() bool {
	for _, v := range ll {
		if len(v.Includes) > 0 {
			return true
		}
	}
	return false
}

// LabelListAttribute is used to represent a list of Bazel labels as an
// attribute.
type LabelListAttribute struct {
	// The non-configured attribute label list Value. Required.
	Value LabelList

	// The configured attribute label list Values. Optional
	// a map of independent configurability axes
	ConfigurableValues configurableLabelLists
}

type configurableLabelLists map[ConfigurationAxis]labelListSelectValues

func (cll configurableLabelLists) setValueForAxis(axis ConfigurationAxis, config string, list LabelList) {
	if list.IsNil() {
		if _, ok := cll[axis][config]; ok {
			delete(cll[axis], config)
		}
		return
	}
	if cll[axis] == nil {
		cll[axis] = make(labelListSelectValues)
	}

	cll[axis][config] = list
}

func (cll configurableLabelLists) Append(other configurableLabelLists) {
	for axis, otherSelects := range other {
		selects := cll[axis]
		if selects == nil {
			selects = make(labelListSelectValues, len(otherSelects))
		}
		selects.appendSelects(otherSelects)
		cll[axis] = selects
	}
}

// MakeLabelListAttribute initializes a LabelListAttribute with the non-arch specific value.
func MakeLabelListAttribute(value LabelList) LabelListAttribute {
	return LabelListAttribute{
		Value:              value,
		ConfigurableValues: make(configurableLabelLists),
	}
}

func (lla *LabelListAttribute) SetValue(list LabelList) {
	lla.SetSelectValue(NoConfigAxis, "", list)
}

// SetSelectValue set a value for a bazel select for the given axis, config and value.
func (lla *LabelListAttribute) SetSelectValue(axis ConfigurationAxis, config string, list LabelList) {
	axis.validateConfig(config)
	switch axis.configurationType {
	case noConfig:
		lla.Value = list
	case arch, os, osArch, productVariables:
		if lla.ConfigurableValues == nil {
			lla.ConfigurableValues = make(configurableLabelLists)
		}
		lla.ConfigurableValues.setValueForAxis(axis, config, list)
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationAxis %s", axis))
	}
}

// SelectValue gets a value for a bazel select for the given axis and config.
func (lla *LabelListAttribute) SelectValue(axis ConfigurationAxis, config string) LabelList {
	axis.validateConfig(config)
	switch axis.configurationType {
	case noConfig:
		return lla.Value
	case arch, os, osArch, productVariables:
		return lla.ConfigurableValues[axis][config]
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationAxis %s", axis))
	}
}

// SortedConfigurationAxes returns all the used ConfigurationAxis in sorted order.
func (lla *LabelListAttribute) SortedConfigurationAxes() []ConfigurationAxis {
	keys := make([]ConfigurationAxis, 0, len(lla.ConfigurableValues))
	for k := range lla.ConfigurableValues {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i].less(keys[j]) })
	return keys
}

// Append all values, including os and arch specific ones, from another
// LabelListAttribute to this LabelListAttribute.
func (lla *LabelListAttribute) Append(other LabelListAttribute) {
	lla.Value.Append(other.Value)
	if lla.ConfigurableValues == nil {
		lla.ConfigurableValues = make(configurableLabelLists)
	}
	lla.ConfigurableValues.Append(other.ConfigurableValues)
}

// HasConfigurableValues returns true if the attribute contains axis-specific label list values.
func (lla LabelListAttribute) HasConfigurableValues() bool {
	return len(lla.ConfigurableValues) > 0
}

// ResolveExcludes handles excludes across the various axes, ensuring that items are removed from
// the base value and included in default values as appropriate.
func (lla *LabelListAttribute) ResolveExcludes() {
	for axis, configToLabels := range lla.ConfigurableValues {
		baseLabels := lla.Value.deepCopy()
		for config, val := range configToLabels {
			// Exclude config-specific excludes from base value
			lla.Value = SubtractBazelLabelList(lla.Value, LabelList{Includes: val.Excludes})

			// add base values to config specific to add labels excluded by others in this axis
			// then remove all config-specific excludes
			allLabels := baseLabels.deepCopy()
			allLabels.Append(val)
			lla.ConfigurableValues[axis][config] = SubtractBazelLabelList(allLabels, LabelList{Includes: val.Excludes})
		}

		// After going through all configs, delete the duplicates in the config
		// values that are already in the base Value.
		for config, val := range configToLabels {
			lla.ConfigurableValues[axis][config] = SubtractBazelLabelList(val, lla.Value)
		}

		// Now that the Value list is finalized for this axis, compare it with the original
		// list, and put the difference into the default condition for the axis.
		lla.ConfigurableValues[axis][conditionsDefault] = SubtractBazelLabelList(baseLabels, lla.Value)

		// if everything ends up without includes, just delete the axis
		if !lla.ConfigurableValues[axis].HasConfigurableValues() {
			delete(lla.ConfigurableValues, axis)
		}
	}
}

// StringListAttribute corresponds to the string_list Bazel attribute type with
// support for additional metadata, like configurations.
type StringListAttribute struct {
	// The base value of the string list attribute.
	Value []string

	// The configured attribute label list Values. Optional
	// a map of independent configurability axes
	ConfigurableValues configurableStringLists
}

type configurableStringLists map[ConfigurationAxis]stringListSelectValues

func (csl configurableStringLists) Append(other configurableStringLists) {
	for axis, otherSelects := range other {
		selects := csl[axis]
		if selects == nil {
			selects = make(stringListSelectValues, len(otherSelects))
		}
		selects.appendSelects(otherSelects)
		csl[axis] = selects
	}
}

func (csl configurableStringLists) setValueForAxis(axis ConfigurationAxis, config string, list []string) {
	if csl[axis] == nil {
		csl[axis] = make(stringListSelectValues)
	}
	csl[axis][config] = list
}

type stringListSelectValues map[string][]string

func (sl stringListSelectValues) appendSelects(other stringListSelectValues) {
	for k, v := range other {
		sl[k] = append(sl[k], v...)
	}
}

func (sl stringListSelectValues) hasConfigurableValues(other stringListSelectValues) bool {
	for _, val := range sl {
		if len(val) > 0 {
			return true
		}
	}
	return false
}

// MakeStringListAttribute initializes a StringListAttribute with the non-arch specific value.
func MakeStringListAttribute(value []string) StringListAttribute {
	// NOTE: These strings are not necessarily unique or sorted.
	return StringListAttribute{
		Value:              value,
		ConfigurableValues: make(configurableStringLists),
	}
}

// HasConfigurableValues returns true if the attribute contains axis-specific string_list values.
func (sla StringListAttribute) HasConfigurableValues() bool {
	return len(sla.ConfigurableValues) > 0
}

// Append appends all values, including os and arch specific ones, from another
// StringListAttribute to this StringListAttribute
func (sla *StringListAttribute) Append(other StringListAttribute) {
	sla.Value = append(sla.Value, other.Value...)
	if sla.ConfigurableValues == nil {
		sla.ConfigurableValues = make(configurableStringLists)
	}
	sla.ConfigurableValues.Append(other.ConfigurableValues)
}

// SetSelectValue set a value for a bazel select for the given axis, config and value.
func (sla *StringListAttribute) SetSelectValue(axis ConfigurationAxis, config string, list []string) {
	axis.validateConfig(config)
	switch axis.configurationType {
	case noConfig:
		sla.Value = list
	case arch, os, osArch, productVariables:
		if sla.ConfigurableValues == nil {
			sla.ConfigurableValues = make(configurableStringLists)
		}
		sla.ConfigurableValues.setValueForAxis(axis, config, list)
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationAxis %s", axis))
	}
}

// SelectValue gets a value for a bazel select for the given axis and config.
func (sla *StringListAttribute) SelectValue(axis ConfigurationAxis, config string) []string {
	axis.validateConfig(config)
	switch axis.configurationType {
	case noConfig:
		return sla.Value
	case arch, os, osArch, productVariables:
		return sla.ConfigurableValues[axis][config]
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationAxis %s", axis))
	}
}

// SortedConfigurationAxes returns all the used ConfigurationAxis in sorted order.
func (sla *StringListAttribute) SortedConfigurationAxes() []ConfigurationAxis {
	keys := make([]ConfigurationAxis, 0, len(sla.ConfigurableValues))
	for k := range sla.ConfigurableValues {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i].less(keys[j]) })
	return keys
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
