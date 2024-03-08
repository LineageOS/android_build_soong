package bp2build

import (
	"fmt"
	"reflect"

	"android/soong/android"
	"android/soong/bazel"
	"android/soong/starlark_fmt"
)

// Configurability support for bp2build.

type selects map[string]reflect.Value

func getStringValue(str bazel.StringAttribute) (reflect.Value, []selects) {
	value := reflect.ValueOf(str.Value)

	if !str.HasConfigurableValues() {
		return value, []selects{}
	}

	ret := selects{}
	for _, axis := range str.SortedConfigurationAxes() {
		configToStrs := str.ConfigurableValues[axis]
		for config, strs := range configToStrs {
			selectKey := axis.SelectKey(config)
			ret[selectKey] = reflect.ValueOf(strs)
		}
	}

	// if there is a select, use the base value as the conditions default value
	if len(ret) > 0 {
		if _, ok := ret[bazel.ConditionsDefaultSelectKey]; !ok {
			ret[bazel.ConditionsDefaultSelectKey] = value
			value = reflect.Zero(value.Type())
		}
	}

	return value, []selects{ret}
}

func getStringListValues(list bazel.StringListAttribute) (reflect.Value, []selects, bool) {
	value := reflect.ValueOf(list.Value)
	prepend := list.Prepend
	if !list.HasConfigurableValues() {
		return value, []selects{}, prepend
	}

	var ret []selects
	for _, axis := range list.SortedConfigurationAxes() {
		configToLists := list.ConfigurableValues[axis]
		archSelects := map[string]reflect.Value{}
		for config, labels := range configToLists {
			selectKey := axis.SelectKey(config)
			archSelects[selectKey] = reflect.ValueOf(labels)
		}
		if len(archSelects) > 0 {
			ret = append(ret, archSelects)
		}
	}

	return value, ret, prepend
}

func getLabelValue(label bazel.LabelAttribute) (reflect.Value, []selects) {
	value := reflect.ValueOf(label.Value)
	if !label.HasConfigurableValues() {
		return value, []selects{}
	}

	ret := selects{}
	for _, axis := range label.SortedConfigurationAxes() {
		configToLabels := label.ConfigurableValues[axis]
		for config, labels := range configToLabels {
			selectKey := axis.SelectKey(config)
			ret[selectKey] = reflect.ValueOf(labels)
		}
	}

	// if there is a select, use the base value as the conditions default value
	if len(ret) > 0 {
		ret[bazel.ConditionsDefaultSelectKey] = value
		value = reflect.Zero(value.Type())
	}

	return value, []selects{ret}
}

func getBoolValue(boolAttr bazel.BoolAttribute) (reflect.Value, []selects) {
	value := reflect.ValueOf(boolAttr.Value)
	if !boolAttr.HasConfigurableValues() {
		return value, []selects{}
	}

	ret := selects{}
	for _, axis := range boolAttr.SortedConfigurationAxes() {
		configToBools := boolAttr.ConfigurableValues[axis]
		for config, bools := range configToBools {
			selectKey := axis.SelectKey(config)
			ret[selectKey] = reflect.ValueOf(bools)
		}
	}
	// if there is a select, use the base value as the conditions default value
	if len(ret) > 0 {
		ret[bazel.ConditionsDefaultSelectKey] = value
		value = reflect.Zero(value.Type())
	}

	return value, []selects{ret}
}
func getLabelListValues(list bazel.LabelListAttribute) (reflect.Value, []selects, bool) {
	value := reflect.ValueOf(list.Value.Includes)
	prepend := list.Prepend
	var ret []selects
	for _, axis := range list.SortedConfigurationAxes() {
		configToLabels := list.ConfigurableValues[axis]
		if !configToLabels.HasConfigurableValues() {
			continue
		}
		archSelects := map[string]reflect.Value{}
		defaultVal := configToLabels[bazel.ConditionsDefaultConfigKey]
		// Skip empty list values unless ether EmitEmptyList is true, or these values differ from the default.
		emitEmptyList := list.EmitEmptyList || len(defaultVal.Includes) > 0
		for config, labels := range configToLabels {
			// Omit any entries in the map which match the default value, for brevity.
			if config != bazel.ConditionsDefaultConfigKey && labels.Equals(defaultVal) {
				continue
			}
			selectKey := axis.SelectKey(config)
			if use, value := labelListSelectValue(selectKey, labels, emitEmptyList); use {
				archSelects[selectKey] = value
			}
		}
		if len(archSelects) > 0 {
			ret = append(ret, archSelects)
		}
	}

	return value, ret, prepend
}

func labelListSelectValue(selectKey string, list bazel.LabelList, emitEmptyList bool) (bool, reflect.Value) {
	if selectKey == bazel.ConditionsDefaultSelectKey || emitEmptyList || len(list.Includes) > 0 {
		return true, reflect.ValueOf(list.Includes)
	} else if len(list.Excludes) > 0 {
		// if there is still an excludes -- we need to have an empty list for this select & use the
		// value in conditions default Includes
		return true, reflect.ValueOf([]string{})
	}
	return false, reflect.Zero(reflect.TypeOf([]string{}))
}

var (
	emptyBazelList = "[]"
	bazelNone      = "None"
)

// prettyPrintAttribute converts an Attribute to its Bazel syntax. May contain
// select statements.
func prettyPrintAttribute(v bazel.Attribute, indent int) (string, error) {
	var value reflect.Value
	// configurableAttrs is the list of individual select statements to be
	// concatenated together. These select statements should be along different
	// axes. For example, one element may be
	// `select({"//color:red": "one", "//color:green": "two"})`, and the second
	// element may be `select({"//animal:cat": "three", "//animal:dog": "four"}).
	// These selects should be sorted by axis identifier.
	var configurableAttrs []selects
	var prepend bool
	var defaultSelectValue *string
	var emitZeroValues bool
	// If true, print the default attribute value, even if the attribute is zero.
	shouldPrintDefault := false
	switch list := v.(type) {
	case bazel.StringAttribute:
		if err := list.Collapse(); err != nil {
			return "", err
		}
		value, configurableAttrs = getStringValue(list)
		defaultSelectValue = &bazelNone
	case bazel.StringListAttribute:
		value, configurableAttrs, prepend = getStringListValues(list)
		defaultSelectValue = &emptyBazelList
	case bazel.LabelListAttribute:
		value, configurableAttrs, prepend = getLabelListValues(list)
		emitZeroValues = list.EmitEmptyList
		defaultSelectValue = &emptyBazelList
		if list.ForceSpecifyEmptyList && (!value.IsNil() || list.HasConfigurableValues()) {
			shouldPrintDefault = true
		}
	case bazel.LabelAttribute:
		if err := list.Collapse(); err != nil {
			return "", err
		}
		value, configurableAttrs = getLabelValue(list)
		defaultSelectValue = &bazelNone
	case bazel.BoolAttribute:
		if err := list.Collapse(); err != nil {
			return "", err
		}
		value, configurableAttrs = getBoolValue(list)
		defaultSelectValue = &bazelNone
	default:
		return "", fmt.Errorf("Not a supported Bazel attribute type: %s", v)
	}

	var err error
	ret := ""
	if value.Kind() != reflect.Invalid {
		s, err := prettyPrint(value, indent, false) // never emit zero values for the base value
		if err != nil {
			return ret, err
		}

		ret += s
	}
	// Convenience function to prepend/append selects components to an attribute value.
	concatenateSelects := func(selectsData selects, defaultValue *string, s string, prepend bool) (string, error) {
		selectMap, err := prettyPrintSelectMap(selectsData, defaultValue, indent, emitZeroValues)
		if err != nil {
			return "", err
		}
		var left, right string
		if prepend {
			left, right = selectMap, s
		} else {
			left, right = s, selectMap
		}
		if left != "" && right != "" {
			left += " + "
		}
		left += right

		return left, nil
	}

	for _, configurableAttr := range configurableAttrs {
		ret, err = concatenateSelects(configurableAttr, defaultSelectValue, ret, prepend)
		if err != nil {
			return "", err
		}
	}

	if ret == "" && shouldPrintDefault {
		return *defaultSelectValue, nil
	}
	return ret, nil
}

// prettyPrintSelectMap converts a map of select keys to reflected Values as a generic way
// to construct a select map for any kind of attribute type.
func prettyPrintSelectMap(selectMap map[string]reflect.Value, defaultValue *string, indent int, emitZeroValues bool) (string, error) {
	if selectMap == nil {
		return "", nil
	}

	var selects string
	for _, selectKey := range android.SortedKeys(selectMap) {
		if selectKey == bazel.ConditionsDefaultSelectKey {
			// Handle default condition later.
			continue
		}
		value := selectMap[selectKey]
		if isZero(value) && !emitZeroValues && isZero(selectMap[bazel.ConditionsDefaultSelectKey]) {
			// Ignore zero values to not generate empty lists. However, always note zero values if
			// the default value is non-zero.
			continue
		}
		s, err := prettyPrintSelectEntry(value, selectKey, indent, true)
		if err != nil {
			return "", err
		}
		// s could still be an empty string, e.g. unset slices of structs with
		// length of 0.
		if s != "" {
			selects += s + ",\n"
		}
	}

	if len(selects) == 0 {
		// If there is a default value, and there are no selects for this axis, print that without any selects.
		if val, exists := selectMap[bazel.ConditionsDefaultSelectKey]; exists {
			return prettyPrint(val, indent, emitZeroValues)
		}
		// No conditions (or all values are empty lists), so no need for a map.
		return "", nil
	}

	// Create the map.
	ret := "select({\n"
	ret += selects

	// Handle the default condition
	s, err := prettyPrintSelectEntry(selectMap[bazel.ConditionsDefaultSelectKey], bazel.ConditionsDefaultSelectKey, indent, emitZeroValues)
	if err != nil {
		return "", err
	}
	if s != "" {
		// Print the custom default value.
		ret += s
		ret += ",\n"
	} else if defaultValue != nil {
		// Print an explicit empty list (the default value) even if the value is
		// empty, to avoid errors about not finding a configuration that matches.
		ret += fmt.Sprintf("%s\"%s\": %s,\n", starlark_fmt.Indention(indent+1), bazel.ConditionsDefaultSelectKey, *defaultValue)
	}

	ret += starlark_fmt.Indention(indent)
	ret += "})"

	return ret, nil
}

// prettyPrintSelectEntry converts a reflect.Value into an entry in a select map
// with a provided key.
func prettyPrintSelectEntry(value reflect.Value, key string, indent int, emitZeroValues bool) (string, error) {
	s := starlark_fmt.Indention(indent + 1)
	v, err := prettyPrint(value, indent+1, emitZeroValues)
	if err != nil {
		return "", err
	}
	if v == "" {
		return "", nil
	}
	s += fmt.Sprintf("\"%s\": %s", key, v)
	return s, nil
}
