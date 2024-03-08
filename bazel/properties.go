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
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/google/blueprint"
)

// BazelTargetModuleProperties contain properties and metadata used for
// Blueprint to BUILD file conversion.
type BazelTargetModuleProperties struct {
	// The Bazel rule class for this target.
	Rule_class string `blueprint:"mutated"`

	// The target label for the bzl file containing the definition of the rule class.
	Bzl_load_location string `blueprint:"mutated"`
}

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

// MakeLabelList creates a LabelList from a list Label
func MakeLabelList(labels []Label) LabelList {
	return LabelList{
		Includes: labels,
		Excludes: nil,
	}
}

func SortedConfigurationAxes[T any](m map[ConfigurationAxis]T) []ConfigurationAxis {
	keys := make([]ConfigurationAxis, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i].less(keys[j]) })
	return keys
}

// MakeLabelListFromTargetNames creates a LabelList from unqualified target names
// This is a utiltity function for bp2build converters of Soong modules that have 1:many generated targets
func MakeLabelListFromTargetNames(targetNames []string) LabelList {
	labels := []Label{}
	for _, name := range targetNames {
		label := Label{Label: ":" + name}
		labels = append(labels, label)
	}
	return MakeLabelList(labels)
}

func (ll *LabelList) Equals(other LabelList) bool {
	if len(ll.Includes) != len(other.Includes) || len(ll.Excludes) != len(other.Excludes) {
		return false
	}
	for i, _ := range ll.Includes {
		if ll.Includes[i] != other.Includes[i] {
			return false
		}
	}
	for i, _ := range ll.Excludes {
		if ll.Excludes[i] != other.Excludes[i] {
			return false
		}
	}
	return true
}

func (ll *LabelList) IsNil() bool {
	return ll.Includes == nil && ll.Excludes == nil
}

func (ll *LabelList) IsEmpty() bool {
	return len(ll.Includes) == 0 && len(ll.Excludes) == 0
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

// Add inserts the label Label at the end of the LabelList.Includes.
func (ll *LabelList) Add(label *Label) {
	if label == nil {
		return
	}
	ll.Includes = append(ll.Includes, *label)
}

// AddExclude inserts the label Label at the end of the LabelList.Excludes.
func (ll *LabelList) AddExclude(label *Label) {
	if label == nil {
		return
	}
	ll.Excludes = append(ll.Excludes, *label)
}

// Append appends the fields of other labelList to the corresponding fields of ll.
func (ll *LabelList) Append(other LabelList) {
	if len(ll.Includes) > 0 || len(other.Includes) > 0 {
		ll.Includes = append(ll.Includes, other.Includes...)
	}
	if len(ll.Excludes) > 0 || len(other.Excludes) > 0 {
		ll.Excludes = append(ll.Excludes, other.Excludes...)
	}
}

// Partition splits a LabelList into two LabelLists depending on the return value
// of the predicate.
// This function preserves the Includes and Excludes, but it does not provide
// that information to the partition function.
func (ll *LabelList) Partition(predicate func(label Label) bool) (LabelList, LabelList) {
	predicated := LabelList{}
	unpredicated := LabelList{}
	for _, include := range ll.Includes {
		if predicate(include) {
			predicated.Add(&include)
		} else {
			unpredicated.Add(&include)
		}
	}
	for _, exclude := range ll.Excludes {
		if predicate(exclude) {
			predicated.AddExclude(&exclude)
		} else {
			unpredicated.AddExclude(&exclude)
		}
	}
	return predicated, unpredicated
}

// UniqueSortedBazelLabels takes a []Label and deduplicates the labels, and returns
// the slice in a sorted order.
func UniqueSortedBazelLabels(originalLabels []Label) []Label {
	uniqueLabels := FirstUniqueBazelLabels(originalLabels)
	sort.SliceStable(uniqueLabels, func(i, j int) bool {
		return uniqueLabels[i].Label < uniqueLabels[j].Label
	})
	return uniqueLabels
}

func FirstUniqueBazelLabels(originalLabels []Label) []Label {
	var labels []Label
	found := make(map[string]bool, len(originalLabels))
	for _, l := range originalLabels {
		if _, ok := found[l.Label]; ok {
			continue
		}
		labels = append(labels, l)
		found[l.Label] = true
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
	needleMap := make(map[string]bool)
	for _, s := range needle {
		needleMap[s] = true
	}

	var strings []string
	for _, s := range haystack {
		if exclude := needleMap[s]; !exclude {
			strings = append(strings, s)
		}
	}

	return strings
}

// Subtract needle from haystack
func SubtractBazelLabels(haystack []Label, needle []Label) []Label {
	// This is really a set
	needleMap := make(map[Label]bool)
	for _, s := range needle {
		needleMap[s] = true
	}

	var labels []Label
	for _, label := range haystack {
		if exclude := needleMap[label]; !exclude {
			labels = append(labels, label)
		}
	}

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

// FirstUniqueBazelLabelListAttribute takes a LabelListAttribute and makes the LabelList for
// each axis/configuration by keeping the first instance of a Label and omitting all subsequent
// repetitions.
func FirstUniqueBazelLabelListAttribute(attr LabelListAttribute) LabelListAttribute {
	var result LabelListAttribute
	result.Value = FirstUniqueBazelLabelList(attr.Value)
	if attr.HasConfigurableValues() {
		result.ConfigurableValues = make(configurableLabelLists)
	}
	for axis, configToLabels := range attr.ConfigurableValues {
		for c, l := range configToLabels {
			result.SetSelectValue(axis, c, FirstUniqueBazelLabelList(l))
		}
	}

	return result
}

// SubtractBazelLabelListAttribute subtract needle from haystack for LabelList in each
// axis/configuration.
func SubtractBazelLabelListAttribute(haystack LabelListAttribute, needle LabelListAttribute) LabelListAttribute {
	var result LabelListAttribute
	result.Value = SubtractBazelLabelList(haystack.Value, needle.Value)
	if haystack.HasConfigurableValues() {
		result.ConfigurableValues = make(configurableLabelLists)
	}
	for axis, configToLabels := range haystack.ConfigurableValues {
		for haystackConfig, haystackLabels := range configToLabels {
			result.SetSelectValue(axis, haystackConfig, SubtractBazelLabelList(haystackLabels, needle.SelectValue(axis, haystackConfig)))
		}
	}

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

func (la *LabelAttribute) axisTypes() map[configurationType]bool {
	types := map[configurationType]bool{}
	for k := range la.ConfigurableValues {
		if len(la.ConfigurableValues[k]) > 0 {
			types[k.configurationType] = true
		}
	}
	return types
}

// Collapse reduces the configurable axes of the label attribute to a single axis.
// This is necessary for final writing to bp2build, as a configurable label
// attribute can only be comprised by a single select.
func (la *LabelAttribute) Collapse() error {
	axisTypes := la.axisTypes()
	_, containsOs := axisTypes[os]
	_, containsArch := axisTypes[arch]
	_, containsOsArch := axisTypes[osArch]
	_, containsProductVariables := axisTypes[productVariables]
	if containsProductVariables {
		if containsOs || containsArch || containsOsArch {
			if containsArch {
				allProductVariablesAreArchVariant := true
				for k := range la.ConfigurableValues {
					if k.configurationType == productVariables && !k.archVariant {
						allProductVariablesAreArchVariant = false
					}
				}
				if !allProductVariablesAreArchVariant {
					return fmt.Errorf("label attribute could not be collapsed as it has two or more unrelated axes")
				}
			} else {
				return fmt.Errorf("label attribute could not be collapsed as it has two or more unrelated axes")
			}
		}
	}
	if (containsOs && containsArch) || (containsOsArch && (containsOs || containsArch)) {
		// If a bool attribute has both os and arch configuration axes, the only
		// way to successfully union their values is to increase the granularity
		// of the configuration criteria to os_arch.
		for osType, supportedArchs := range osToArchMap {
			for _, supportedArch := range supportedArchs {
				osArch := osArchString(osType, supportedArch)
				if archOsVal := la.SelectValue(OsArchConfigurationAxis, osArch); archOsVal != nil {
					// Do nothing, as the arch_os is explicitly defined already.
				} else {
					archVal := la.SelectValue(ArchConfigurationAxis, supportedArch)
					osVal := la.SelectValue(OsConfigurationAxis, osType)
					if osVal != nil && archVal != nil {
						// In this case, arch takes precedence. (This fits legacy Soong behavior, as arch mutator
						// runs after os mutator.
						la.SetSelectValue(OsArchConfigurationAxis, osArch, *archVal)
					} else if osVal != nil && archVal == nil {
						la.SetSelectValue(OsArchConfigurationAxis, osArch, *osVal)
					} else if osVal == nil && archVal != nil {
						la.SetSelectValue(OsArchConfigurationAxis, osArch, *archVal)
					}
				}
			}
		}
		// All os_arch values are now set. Clear os and arch axes.
		delete(la.ConfigurableValues, ArchConfigurationAxis)
		delete(la.ConfigurableValues, OsConfigurationAxis)
	}
	return nil
}

// HasConfigurableValues returns whether there are configurable values set for this label.
func (la LabelAttribute) HasConfigurableValues() bool {
	for _, selectValues := range la.ConfigurableValues {
		if len(selectValues) > 0 {
			return true
		}
	}
	return false
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
	case arch, os, osArch, productVariables, osAndInApex, sanitizersEnabled:
		if la.ConfigurableValues == nil {
			la.ConfigurableValues = make(configurableLabels)
		}
		la.ConfigurableValues.setValueForAxis(axis, config, &value)
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationAxis %s", axis))
	}
}

// SelectValue gets a value for a bazel select for the given axis and config.
func (la *LabelAttribute) SelectValue(axis ConfigurationAxis, config string) *Label {
	axis.validateConfig(config)
	switch axis.configurationType {
	case noConfig:
		return la.Value
	case arch, os, osArch, productVariables, osAndInApex, sanitizersEnabled:
		return la.ConfigurableValues[axis][config]
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationAxis %s", axis))
	}
}

// SortedConfigurationAxes returns all the used ConfigurationAxis in sorted order.
func (la *LabelAttribute) SortedConfigurationAxes() []ConfigurationAxis {
	return SortedConfigurationAxes(la.ConfigurableValues)
}

// MakeLabelAttribute turns a string into a LabelAttribute
func MakeLabelAttribute(label string) *LabelAttribute {
	return &LabelAttribute{
		Value: &Label{
			Label: label,
		},
	}
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
	for _, cfgToBools := range ba.ConfigurableValues {
		if len(cfgToBools) > 0 {
			return true
		}
	}
	return false
}

// SetValue sets value for the no config axis
func (ba *BoolAttribute) SetValue(value *bool) {
	ba.SetSelectValue(NoConfigAxis, "", value)
}

// SetSelectValue sets value for the given axis/config.
func (ba *BoolAttribute) SetSelectValue(axis ConfigurationAxis, config string, value *bool) {
	axis.validateConfig(config)
	switch axis.configurationType {
	case noConfig:
		ba.Value = value
	case arch, os, osArch, productVariables, osAndInApex, sanitizersEnabled:
		if ba.ConfigurableValues == nil {
			ba.ConfigurableValues = make(configurableBools)
		}
		ba.ConfigurableValues.setValueForAxis(axis, config, value)
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationAxis %s", axis))
	}
}

// ToLabelListAttribute creates and returns a LabelListAttribute from this
// bool attribute, where each bool in this attribute corresponds to a
// label list value in the resultant attribute.
func (ba *BoolAttribute) ToLabelListAttribute(falseVal LabelList, trueVal LabelList) (LabelListAttribute, error) {
	getLabelList := func(boolPtr *bool) LabelList {
		if boolPtr == nil {
			return LabelList{nil, nil}
		} else if *boolPtr {
			return trueVal
		} else {
			return falseVal
		}
	}

	mainVal := getLabelList(ba.Value)
	if !ba.HasConfigurableValues() {
		return MakeLabelListAttribute(mainVal), nil
	}

	result := LabelListAttribute{}
	if err := ba.Collapse(); err != nil {
		return result, err
	}

	for axis, configToBools := range ba.ConfigurableValues {
		if len(configToBools) < 1 {
			continue
		}
		for config, boolPtr := range configToBools {
			val := getLabelList(&boolPtr)
			if !val.Equals(mainVal) {
				result.SetSelectValue(axis, config, val)
			}
		}
		result.SetSelectValue(axis, ConditionsDefaultConfigKey, mainVal)
	}

	return result, nil
}

// ToStringListAttribute creates a StringListAttribute from this BoolAttribute,
// where each bool corresponds to a string list value generated by the provided
// function.
// TODO(b/271425661): Generalize this
func (ba *BoolAttribute) ToStringListAttribute(valueFunc func(boolPtr *bool, axis ConfigurationAxis, config string) []string) (StringListAttribute, error) {
	mainVal := valueFunc(ba.Value, NoConfigAxis, "")
	if !ba.HasConfigurableValues() {
		return MakeStringListAttribute(mainVal), nil
	}

	result := StringListAttribute{}
	if err := ba.Collapse(); err != nil {
		return result, err
	}

	for axis, configToBools := range ba.ConfigurableValues {
		if len(configToBools) < 1 {
			continue
		}
		for config, boolPtr := range configToBools {
			val := valueFunc(&boolPtr, axis, config)
			if !reflect.DeepEqual(val, mainVal) {
				result.SetSelectValue(axis, config, val)
			}
		}
		result.SetSelectValue(axis, ConditionsDefaultConfigKey, mainVal)
	}

	return result, nil
}

// Collapse reduces the configurable axes of the boolean attribute to a single axis.
// This is necessary for final writing to bp2build, as a configurable boolean
// attribute can only be comprised by a single select.
func (ba *BoolAttribute) Collapse() error {
	axisTypes := ba.axisTypes()
	_, containsOs := axisTypes[os]
	_, containsArch := axisTypes[arch]
	_, containsOsArch := axisTypes[osArch]
	_, containsProductVariables := axisTypes[productVariables]
	if containsProductVariables {
		if containsOs || containsArch || containsOsArch {
			return fmt.Errorf("boolean attribute could not be collapsed as it has two or more unrelated axes")
		}
	}
	if (containsOs && containsArch) || (containsOsArch && (containsOs || containsArch)) {
		// If a bool attribute has both os and arch configuration axes, the only
		// way to successfully union their values is to increase the granularity
		// of the configuration criteria to os_arch.
		for osType, supportedArchs := range osToArchMap {
			for _, supportedArch := range supportedArchs {
				osArch := osArchString(osType, supportedArch)
				if archOsVal := ba.SelectValue(OsArchConfigurationAxis, osArch); archOsVal != nil {
					// Do nothing, as the arch_os is explicitly defined already.
				} else {
					archVal := ba.SelectValue(ArchConfigurationAxis, supportedArch)
					osVal := ba.SelectValue(OsConfigurationAxis, osType)
					if osVal != nil && archVal != nil {
						// In this case, arch takes precedence. (This fits legacy Soong behavior, as arch mutator
						// runs after os mutator.
						ba.SetSelectValue(OsArchConfigurationAxis, osArch, archVal)
					} else if osVal != nil && archVal == nil {
						ba.SetSelectValue(OsArchConfigurationAxis, osArch, osVal)
					} else if osVal == nil && archVal != nil {
						ba.SetSelectValue(OsArchConfigurationAxis, osArch, archVal)
					}
				}
			}
		}
		// All os_arch values are now set. Clear os and arch axes.
		delete(ba.ConfigurableValues, ArchConfigurationAxis)
		delete(ba.ConfigurableValues, OsConfigurationAxis)
		// Verify post-condition; this should never fail, provided no additional
		// axes are introduced.
		if len(ba.ConfigurableValues) > 1 {
			panic(fmt.Errorf("error in collapsing attribute: %#v", ba))
		}
	}
	return nil
}

func (ba *BoolAttribute) axisTypes() map[configurationType]bool {
	types := map[configurationType]bool{}
	for k := range ba.ConfigurableValues {
		if len(ba.ConfigurableValues[k]) > 0 {
			types[k.configurationType] = true
		}
	}
	return types
}

// SelectValue gets the value for the given axis/config.
func (ba BoolAttribute) SelectValue(axis ConfigurationAxis, config string) *bool {
	axis.validateConfig(config)
	switch axis.configurationType {
	case noConfig:
		return ba.Value
	case arch, os, osArch, productVariables, osAndInApex, sanitizersEnabled:
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
	return SortedConfigurationAxes(ba.ConfigurableValues)
}

// labelListSelectValues supports config-specific label_list typed Bazel attribute values.
type labelListSelectValues map[string]LabelList

func (ll labelListSelectValues) addSelects(label labelSelectValues) {
	for k, v := range label {
		if label == nil {
			continue
		}
		l := ll[k]
		(&l).Add(v)
		ll[k] = l
	}
}

func (ll labelListSelectValues) appendSelects(other labelListSelectValues, forceSpecifyEmptyList bool) {
	for k, v := range other {
		l := ll[k]
		if forceSpecifyEmptyList && l.IsNil() && !v.IsNil() {
			l.Includes = []Label{}
		}
		(&l).Append(v)
		ll[k] = l
	}
}

// HasConfigurableValues returns whether there are configurable values within this set of selects.
func (ll labelListSelectValues) HasConfigurableValues() bool {
	for _, v := range ll {
		if v.Includes != nil {
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

	// If true, differentiate between "nil" and "empty" list. nil means that
	// this attribute should not be specified at all, and "empty" means that
	// the attribute should be explicitly specified as an empty list.
	// This mode facilitates use of attribute defaults: an empty list should
	// override the default.
	ForceSpecifyEmptyList bool

	// If true, signal the intent to the code generator to emit all select keys,
	// even if the Includes list for that key is empty. This mode facilitates
	// specific select statements where an empty list for a non-default select
	// key has a meaning.
	EmitEmptyList bool

	// If a property has struct tag "variant_prepend", this value should
	// be set to True, so that when bp2build generates BUILD.bazel, variant
	// properties(select ...) come before general properties.
	Prepend bool
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

func (cll configurableLabelLists) Append(other configurableLabelLists, forceSpecifyEmptyList bool) {
	for axis, otherSelects := range other {
		selects := cll[axis]
		if selects == nil {
			selects = make(labelListSelectValues, len(otherSelects))
		}
		selects.appendSelects(otherSelects, forceSpecifyEmptyList)
		cll[axis] = selects
	}
}

func (lla *LabelListAttribute) Clone() *LabelListAttribute {
	result := &LabelListAttribute{ForceSpecifyEmptyList: lla.ForceSpecifyEmptyList}
	return result.Append(*lla)
}

// MakeLabelListAttribute initializes a LabelListAttribute with the non-arch specific value.
func MakeLabelListAttribute(value LabelList) LabelListAttribute {
	return LabelListAttribute{
		Value:              value,
		ConfigurableValues: make(configurableLabelLists),
	}
}

// MakeSingleLabelListAttribute initializes a LabelListAttribute as a non-arch specific list with 1 element, the given Label.
func MakeSingleLabelListAttribute(value Label) LabelListAttribute {
	return MakeLabelListAttribute(MakeLabelList([]Label{value}))
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
	case arch, os, osArch, productVariables, osAndInApex, inApex, errorProneDisabled, sanitizersEnabled:
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
	case arch, os, osArch, productVariables, osAndInApex, inApex, errorProneDisabled, sanitizersEnabled:
		return lla.ConfigurableValues[axis][config]
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationAxis %s", axis))
	}
}

// SortedConfigurationAxes returns all the used ConfigurationAxis in sorted order.
func (lla *LabelListAttribute) SortedConfigurationAxes() []ConfigurationAxis {
	return SortedConfigurationAxes(lla.ConfigurableValues)
}

// Append all values, including os and arch specific ones, from another
// LabelListAttribute to this LabelListAttribute. Returns this LabelListAttribute.
func (lla *LabelListAttribute) Append(other LabelListAttribute) *LabelListAttribute {
	forceSpecifyEmptyList := lla.ForceSpecifyEmptyList || other.ForceSpecifyEmptyList
	if forceSpecifyEmptyList && lla.Value.IsNil() && !other.Value.IsNil() {
		lla.Value.Includes = []Label{}
	}
	lla.Value.Append(other.Value)
	if lla.ConfigurableValues == nil {
		lla.ConfigurableValues = make(configurableLabelLists)
	}
	lla.ConfigurableValues.Append(other.ConfigurableValues, forceSpecifyEmptyList)
	return lla
}

// Add inserts the labels for each axis of LabelAttribute at the end of corresponding axis's
// LabelList within the LabelListAttribute
func (lla *LabelListAttribute) Add(label *LabelAttribute) {
	if label == nil {
		return
	}

	lla.Value.Add(label.Value)
	if lla.ConfigurableValues == nil && label.ConfigurableValues != nil {
		lla.ConfigurableValues = make(configurableLabelLists)
	}
	for axis, _ := range label.ConfigurableValues {
		if _, exists := lla.ConfigurableValues[axis]; !exists {
			lla.ConfigurableValues[axis] = make(labelListSelectValues)
		}
		lla.ConfigurableValues[axis].addSelects(label.ConfigurableValues[axis])
	}
}

// HasConfigurableValues returns true if the attribute contains axis-specific label list values.
func (lla LabelListAttribute) HasConfigurableValues() bool {
	for _, selectValues := range lla.ConfigurableValues {
		if len(selectValues) > 0 {
			return true
		}
	}
	return false
}

// HasAxisSpecificValues returns true if the attribute contains axis specific label list values from a given axis
func (lla LabelListAttribute) HasAxisSpecificValues(axis ConfigurationAxis) bool {
	for _, values := range lla.ConfigurableValues[axis] {
		if !values.IsNil() {
			return true
		}
	}
	return false
}

// IsEmpty returns true if the attribute has no values under any configuration.
func (lla LabelListAttribute) IsEmpty() bool {
	if len(lla.Value.Includes) > 0 {
		return false
	}
	for axis, _ := range lla.ConfigurableValues {
		if lla.ConfigurableValues[axis].HasConfigurableValues() {
			return false
		}
	}
	return true
}

// IsNil returns true if the attribute has not been set for any configuration.
func (lla LabelListAttribute) IsNil() bool {
	if lla.Value.Includes != nil {
		return false
	}
	return !lla.HasConfigurableValues()
}

// Exclude for the given axis, config, removes Includes in labelList from Includes and appends them
// to Excludes. This is to special case any excludes that are not specified in a bp file but need to
// be removed, e.g. if they could cause duplicate element failures.
func (lla *LabelListAttribute) Exclude(axis ConfigurationAxis, config string, labelList LabelList) {
	val := lla.SelectValue(axis, config)
	newList := SubtractBazelLabelList(val, labelList)
	newList.Excludes = append(newList.Excludes, labelList.Includes...)
	lla.SetSelectValue(axis, config, newList)
}

// ResolveExcludes handles excludes across the various axes, ensuring that items are removed from
// the base value and included in default values as appropriate.
func (lla *LabelListAttribute) ResolveExcludes() {
	// If there are OsAndInApexAxis, we need to use
	//   * includes from the OS & in APEX Axis for non-Android configs for libraries that need to be
	//     included in non-Android OSes
	//   * excludes from the OS Axis for non-Android configs, to exclude libraries that should _not_
	//     be included in the non-Android OSes
	if _, ok := lla.ConfigurableValues[OsAndInApexAxis]; ok {
		inApexLabels := lla.ConfigurableValues[OsAndInApexAxis][ConditionsDefaultConfigKey]
		for config, labels := range lla.ConfigurableValues[OsConfigurationAxis] {
			// OsAndroid has already handled its excludes.
			// We only need to copy the excludes from other arches, so if there are none, skip it.
			if config == OsAndroid || len(labels.Excludes) == 0 {
				continue
			}
			lla.ConfigurableValues[OsAndInApexAxis][config] = LabelList{
				Includes: inApexLabels.Includes,
				Excludes: labels.Excludes,
			}
		}
	}

	for axis, configToLabels := range lla.ConfigurableValues {
		baseLabels := lla.Value.deepCopy()
		for config, val := range configToLabels {
			// Exclude config-specific excludes from base value
			lla.Value = SubtractBazelLabelList(lla.Value, LabelList{Includes: val.Excludes})

			// add base values to config specific to add labels excluded by others in this axis
			// then remove all config-specific excludes
			allLabels := baseLabels.deepCopy()
			allLabels.Append(val)
			lla.ConfigurableValues[axis][config] = SubtractBazelLabelList(allLabels, LabelList{Includes: allLabels.Excludes})
		}

		// After going through all configs, delete the duplicates in the config
		// values that are already in the base Value.
		for config, val := range configToLabels {
			lla.ConfigurableValues[axis][config] = SubtractBazelLabelList(val, lla.Value)
		}

		// Now that the Value list is finalized for this axis, compare it with
		// the original list, and union the difference with the default
		// condition for the axis.
		difference := SubtractBazelLabelList(baseLabels, lla.Value)
		existingDefaults := lla.ConfigurableValues[axis][ConditionsDefaultConfigKey]
		existingDefaults.Append(difference)
		lla.ConfigurableValues[axis][ConditionsDefaultConfigKey] = FirstUniqueBazelLabelList(existingDefaults)

		// if everything ends up without includes, just delete the axis
		if !lla.ConfigurableValues[axis].HasConfigurableValues() {
			delete(lla.ConfigurableValues, axis)
		}
	}
}

// Partition splits a LabelListAttribute into two LabelListAttributes depending
// on the return value of the predicate.
// This function preserves the Includes and Excludes, but it does not provide
// that information to the partition function.
func (lla LabelListAttribute) Partition(predicate func(label Label) bool) (LabelListAttribute, LabelListAttribute) {
	predicated := LabelListAttribute{}
	unpredicated := LabelListAttribute{}

	valuePartitionTrue, valuePartitionFalse := lla.Value.Partition(predicate)
	predicated.SetValue(valuePartitionTrue)
	unpredicated.SetValue(valuePartitionFalse)

	for axis, selectValueLabelLists := range lla.ConfigurableValues {
		for config, labelList := range selectValueLabelLists {
			configPredicated, configUnpredicated := labelList.Partition(predicate)
			predicated.SetSelectValue(axis, config, configPredicated)
			unpredicated.SetSelectValue(axis, config, configUnpredicated)
		}
	}

	return predicated, unpredicated
}

// OtherModuleContext is a limited context that has methods with information about other modules.
type OtherModuleContext interface {
	ModuleFromName(name string) (blueprint.Module, bool)
	OtherModuleType(m blueprint.Module) string
	OtherModuleName(m blueprint.Module) string
	OtherModuleDir(m blueprint.Module) string
	ModuleErrorf(fmt string, args ...interface{})
}

// LabelMapper is a function that takes a OtherModuleContext and returns a (potentially changed)
// label and whether it was changed.
type LabelMapper func(OtherModuleContext, Label) (string, bool)

// LabelPartition contains descriptions of a partition for labels
type LabelPartition struct {
	// Extensions to include in this partition
	Extensions []string
	// LabelMapper is a function that can map a label to a new label, and indicate whether to include
	// the mapped label in the partition
	LabelMapper LabelMapper
	// Whether to store files not included in any other partition in a group of LabelPartitions
	// Only one partition in a group of LabelPartitions can enabled Keep_remainder
	Keep_remainder bool
}

// LabelPartitions is a map of partition name to a LabelPartition describing the elements of the
// partition
type LabelPartitions map[string]LabelPartition

// filter returns a pointer to a label if the label should be included in the partition or nil if
// not.
func (lf LabelPartition) filter(ctx OtherModuleContext, label Label) *Label {
	if lf.LabelMapper != nil {
		if newLabel, changed := lf.LabelMapper(ctx, label); changed {
			return &Label{newLabel, label.OriginalModuleName}
		}
	}
	for _, ext := range lf.Extensions {
		if strings.HasSuffix(label.Label, ext) {
			return &label
		}
	}

	return nil
}

// PartitionToLabelListAttribute is map of partition name to a LabelListAttribute
type PartitionToLabelListAttribute map[string]LabelListAttribute

type partitionToLabelList map[string]*LabelList

func (p partitionToLabelList) appendIncludes(partition string, label Label) {
	if _, ok := p[partition]; !ok {
		p[partition] = &LabelList{}
	}
	p[partition].Includes = append(p[partition].Includes, label)
}

func (p partitionToLabelList) excludes(partition string, excludes []Label) {
	if _, ok := p[partition]; !ok {
		p[partition] = &LabelList{}
	}
	p[partition].Excludes = excludes
}

// PartitionLabelListAttribute partitions a LabelListAttribute into the requested partitions
func PartitionLabelListAttribute(ctx OtherModuleContext, lla *LabelListAttribute, partitions LabelPartitions) PartitionToLabelListAttribute {
	ret := PartitionToLabelListAttribute{}
	var partitionNames []string
	// Stored as a pointer to distinguish nil (no remainder partition) from empty string partition
	var remainderPartition *string
	for p, f := range partitions {
		partitionNames = append(partitionNames, p)
		if f.Keep_remainder {
			if remainderPartition != nil {
				panic("only one partition can store the remainder")
			}
			// If we take the address of p in a loop, we'll end up with the last value of p in
			// remainderPartition, we want the requested partition
			capturePartition := p
			remainderPartition = &capturePartition
		}
	}

	partitionLabelList := func(axis ConfigurationAxis, config string) {
		value := lla.SelectValue(axis, config)
		partitionToLabels := partitionToLabelList{}
		for _, item := range value.Includes {
			wasFiltered := false
			var inPartition *string
			for partition, f := range partitions {
				filtered := f.filter(ctx, item)
				if filtered == nil {
					// did not match this filter, keep looking
					continue
				}
				wasFiltered = true
				partitionToLabels.appendIncludes(partition, *filtered)
				// don't need to check other partitions if this filter used the item,
				// continue checking if mapped to another name
				if *filtered == item {
					if inPartition != nil {
						ctx.ModuleErrorf("%q was found in multiple partitions: %q, %q", item.Label, *inPartition, partition)
					}
					capturePartition := partition
					inPartition = &capturePartition
				}
			}

			// if not specified in a partition, add to remainder partition if one exists
			if !wasFiltered && remainderPartition != nil {
				partitionToLabels.appendIncludes(*remainderPartition, item)
			}
		}

		// ensure empty lists are maintained
		if value.Excludes != nil {
			for _, partition := range partitionNames {
				partitionToLabels.excludes(partition, value.Excludes)
			}
		}

		for partition, list := range partitionToLabels {
			val := ret[partition]
			(&val).SetSelectValue(axis, config, *list)
			ret[partition] = val
		}
	}

	partitionLabelList(NoConfigAxis, "")
	for axis, configToList := range lla.ConfigurableValues {
		for config, _ := range configToList {
			partitionLabelList(axis, config)
		}
	}
	return ret
}

// StringAttribute corresponds to the string Bazel attribute type with
// support for additional metadata, like configurations.
type StringAttribute struct {
	// The base value of the string attribute.
	Value *string

	// The configured attribute label list Values. Optional
	// a map of independent configurability axes
	ConfigurableValues configurableStrings
}

type configurableStrings map[ConfigurationAxis]stringSelectValues

func (cs configurableStrings) setValueForAxis(axis ConfigurationAxis, config string, str *string) {
	if cs[axis] == nil {
		cs[axis] = make(stringSelectValues)
	}
	cs[axis][config] = str
}

type stringSelectValues map[string]*string

// HasConfigurableValues returns true if the attribute contains axis-specific string values.
func (sa StringAttribute) HasConfigurableValues() bool {
	for _, selectValues := range sa.ConfigurableValues {
		if len(selectValues) > 0 {
			return true
		}
	}
	return false
}

// SetValue sets the base, non-configured value for the Label
func (sa *StringAttribute) SetValue(value string) {
	sa.SetSelectValue(NoConfigAxis, "", &value)
}

// SetSelectValue set a value for a bazel select for the given axis, config and value.
func (sa *StringAttribute) SetSelectValue(axis ConfigurationAxis, config string, str *string) {
	axis.validateConfig(config)
	switch axis.configurationType {
	case noConfig:
		sa.Value = str
	case arch, os, osArch, productVariables, sanitizersEnabled:
		if sa.ConfigurableValues == nil {
			sa.ConfigurableValues = make(configurableStrings)
		}
		sa.ConfigurableValues.setValueForAxis(axis, config, str)
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationAxis %s", axis))
	}
}

// SelectValue gets a value for a bazel select for the given axis and config.
func (sa *StringAttribute) SelectValue(axis ConfigurationAxis, config string) *string {
	axis.validateConfig(config)
	switch axis.configurationType {
	case noConfig:
		return sa.Value
	case arch, os, osArch, productVariables, sanitizersEnabled:
		if v, ok := sa.ConfigurableValues[axis][config]; ok {
			return v
		} else {
			return nil
		}
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationAxis %s", axis))
	}
}

// SortedConfigurationAxes returns all the used ConfigurationAxis in sorted order.
func (sa *StringAttribute) SortedConfigurationAxes() []ConfigurationAxis {
	return SortedConfigurationAxes(sa.ConfigurableValues)
}

// Collapse reduces the configurable axes of the string attribute to a single axis.
// This is necessary for final writing to bp2build, as a configurable string
// attribute can only be comprised by a single select.
func (sa *StringAttribute) Collapse() error {
	axisTypes := sa.axisTypes()
	_, containsOs := axisTypes[os]
	_, containsArch := axisTypes[arch]
	_, containsOsArch := axisTypes[osArch]
	_, containsProductVariables := axisTypes[productVariables]
	if containsProductVariables {
		if containsOs || containsArch || containsOsArch {
			return fmt.Errorf("string attribute could not be collapsed as it has two or more unrelated axes")
		}
	}
	if (containsOs && containsArch) || (containsOsArch && (containsOs || containsArch)) {
		// If a bool attribute has both os and arch configuration axes, the only
		// way to successfully union their values is to increase the granularity
		// of the configuration criteria to os_arch.
		for osType, supportedArchs := range osToArchMap {
			for _, supportedArch := range supportedArchs {
				osArch := osArchString(osType, supportedArch)
				if archOsVal := sa.SelectValue(OsArchConfigurationAxis, osArch); archOsVal != nil {
					// Do nothing, as the arch_os is explicitly defined already.
				} else {
					archVal := sa.SelectValue(ArchConfigurationAxis, supportedArch)
					osVal := sa.SelectValue(OsConfigurationAxis, osType)
					if osVal != nil && archVal != nil {
						// In this case, arch takes precedence. (This fits legacy Soong behavior, as arch mutator
						// runs after os mutator.
						sa.SetSelectValue(OsArchConfigurationAxis, osArch, archVal)
					} else if osVal != nil && archVal == nil {
						sa.SetSelectValue(OsArchConfigurationAxis, osArch, osVal)
					} else if osVal == nil && archVal != nil {
						sa.SetSelectValue(OsArchConfigurationAxis, osArch, archVal)
					}
				}
			}
		}
		/// All os_arch values are now set. Clear os and arch axes.
		delete(sa.ConfigurableValues, ArchConfigurationAxis)
		delete(sa.ConfigurableValues, OsConfigurationAxis)
		// Verify post-condition; this should never fail, provided no additional
		// axes are introduced.
		if len(sa.ConfigurableValues) > 1 {
			panic(fmt.Errorf("error in collapsing attribute: %#v", sa))
		}
	} else if containsProductVariables {
		usedBaseValue := false
		for a, configToProp := range sa.ConfigurableValues {
			if a.configurationType == productVariables {
				for c, p := range configToProp {
					if p == nil {
						sa.SetSelectValue(a, c, sa.Value)
						usedBaseValue = true
					}
				}
			}
		}
		if usedBaseValue {
			sa.Value = nil
		}
	}
	return nil
}

func (sa *StringAttribute) axisTypes() map[configurationType]bool {
	types := map[configurationType]bool{}
	for k := range sa.ConfigurableValues {
		if strs := sa.ConfigurableValues[k]; len(strs) > 0 {
			types[k.configurationType] = true
		}
	}
	return types
}

// StringListAttribute corresponds to the string_list Bazel attribute type with
// support for additional metadata, like configurations.
type StringListAttribute struct {
	// The base value of the string list attribute.
	Value []string

	// The configured attribute label list Values. Optional
	// a map of independent configurability axes
	ConfigurableValues configurableStringLists

	// If a property has struct tag "variant_prepend", this value should
	// be set to True, so that when bp2build generates BUILD.bazel, variant
	// properties(select ...) come before general properties.
	Prepend bool
}

// IsEmpty returns true if the attribute has no values under any configuration.
func (sla StringListAttribute) IsEmpty() bool {
	return len(sla.Value) == 0 && !sla.HasConfigurableValues()
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
	for _, selectValues := range sla.ConfigurableValues {
		if len(selectValues) > 0 {
			return true
		}
	}
	return false
}

// Append appends all values, including os and arch specific ones, from another
// StringListAttribute to this StringListAttribute
func (sla *StringListAttribute) Append(other StringListAttribute) *StringListAttribute {
	sla.Value = append(sla.Value, other.Value...)
	if sla.ConfigurableValues == nil {
		sla.ConfigurableValues = make(configurableStringLists)
	}
	sla.ConfigurableValues.Append(other.ConfigurableValues)
	return sla
}

func (sla *StringListAttribute) Clone() *StringListAttribute {
	result := &StringListAttribute{}
	return result.Append(*sla)
}

// SetSelectValue set a value for a bazel select for the given axis, config and value.
func (sla *StringListAttribute) SetSelectValue(axis ConfigurationAxis, config string, list []string) {
	axis.validateConfig(config)
	switch axis.configurationType {
	case noConfig:
		sla.Value = list
	case arch, os, osArch, productVariables, osAndInApex, errorProneDisabled, sanitizersEnabled:
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
	case arch, os, osArch, productVariables, osAndInApex, errorProneDisabled, sanitizersEnabled:
		return sla.ConfigurableValues[axis][config]
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationAxis %s", axis))
	}
}

// SortedConfigurationAxes returns all the used ConfigurationAxis in sorted order.
func (sla *StringListAttribute) SortedConfigurationAxes() []ConfigurationAxis {
	return SortedConfigurationAxes(sla.ConfigurableValues)
}

// DeduplicateAxesFromBase ensures no duplication of items between the no-configuration value and
// configuration-specific values. For example, if we would convert this StringListAttribute as:
//
//	["a", "b", "c"] + select({
//	   "//condition:one": ["a", "d"],
//	   "//conditions:default": [],
//	})
//
// after this function, we would convert this StringListAttribute as:
//
//	["a", "b", "c"] + select({
//	   "//condition:one": ["d"],
//	   "//conditions:default": [],
//	})
func (sla *StringListAttribute) DeduplicateAxesFromBase() {
	base := sla.Value
	for axis, configToList := range sla.ConfigurableValues {
		for config, list := range configToList {
			remaining := SubtractStrings(list, base)
			if len(remaining) == 0 {
				delete(sla.ConfigurableValues[axis], config)
			} else {
				sla.ConfigurableValues[axis][config] = remaining
			}
		}
	}
}

// TryVariableSubstitution, replace string substitution formatting within each string in slice with
// Starlark string.format compatible tag for productVariable.
func TryVariableSubstitutions(slice []string, productVariable string) ([]string, bool) {
	if len(slice) == 0 {
		return slice, false
	}
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

// StringMapAttribute is a map of strings.
// The use case for this is storing the flag_values in a config_setting object.
// Bazel rules do not support map attributes, and this should NOT be used in Bazel rules.
type StringMapAttribute map[string]string

// ConfigSettingAttributes stores the keys of a config_setting object.
type ConfigSettingAttributes struct {
	// Each key in Flag_values is a label to a custom string_setting
	Flag_values StringMapAttribute
	// Each element in Constraint_values is a label to a constraint_value
	Constraint_values LabelListAttribute
}
