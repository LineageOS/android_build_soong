package bp2build

import (
	"android/soong/android"
	"android/soong/bazel"
	"fmt"
	"reflect"
)

// Configurability support for bp2build.

type selects map[string]reflect.Value

func getStringListValues(list bazel.StringListAttribute) (reflect.Value, selects, selects) {
	value := reflect.ValueOf(list.Value)
	if !list.HasConfigurableValues() {
		return value, nil, nil
	}

	archSelects := map[string]reflect.Value{}
	for arch, selectKey := range bazel.PlatformArchMap {
		archSelects[selectKey] = reflect.ValueOf(list.GetValueForArch(arch))
	}

	osSelects := map[string]reflect.Value{}
	for os, selectKey := range bazel.PlatformOsMap {
		osSelects[selectKey] = reflect.ValueOf(list.GetValueForOS(os))
	}

	return value, archSelects, osSelects
}

func getLabelValue(label bazel.LabelAttribute) (reflect.Value, selects, selects) {
	var value reflect.Value
	var archSelects selects

	if label.HasConfigurableValues() {
		archSelects = map[string]reflect.Value{}
		for arch, selectKey := range bazel.PlatformArchMap {
			archSelects[selectKey] = reflect.ValueOf(label.GetValueForArch(arch))
		}
	} else {
		value = reflect.ValueOf(label.Value)
	}

	return value, archSelects, nil
}

func getLabelListValues(list bazel.LabelListAttribute) (reflect.Value, selects, selects) {
	value := reflect.ValueOf(list.Value.Includes)
	if !list.HasConfigurableValues() {
		return value, nil, nil
	}

	archSelects := map[string]reflect.Value{}
	for arch, selectKey := range bazel.PlatformArchMap {
		archSelects[selectKey] = reflect.ValueOf(list.GetValueForArch(arch).Includes)
	}

	osSelects := map[string]reflect.Value{}
	for os, selectKey := range bazel.PlatformOsMap {
		osSelects[selectKey] = reflect.ValueOf(list.GetValueForOS(os).Includes)
	}

	return value, archSelects, osSelects
}

// prettyPrintAttribute converts an Attribute to its Bazel syntax. May contain
// select statements.
func prettyPrintAttribute(v bazel.Attribute, indent int) (string, error) {
	var value reflect.Value
	var archSelects, osSelects selects
	var defaultSelectValue string
	switch list := v.(type) {
	case bazel.StringListAttribute:
		value, archSelects, osSelects = getStringListValues(list)
		defaultSelectValue = "[]"
	case bazel.LabelListAttribute:
		value, archSelects, osSelects = getLabelListValues(list)
		defaultSelectValue = "[]"
	case bazel.LabelAttribute:
		value, archSelects, osSelects = getLabelValue(list)
		defaultSelectValue = "None"
	default:
		return "", fmt.Errorf("Not a supported Bazel attribute type: %s", v)
	}

	ret := ""
	if value.Kind() != reflect.Invalid {
		s, err := prettyPrint(value, indent)
		if err != nil {
			return ret, err
		}

		ret += s
	}
	// Convenience function to append selects components to an attribute value.
	appendSelects := func(selectsData selects, defaultValue, s string) (string, error) {
		selectMap, err := prettyPrintSelectMap(selectsData, defaultValue, indent)
		if err != nil {
			return "", err
		}
		if s != "" && selectMap != "" {
			s += " + "
		}
		s += selectMap

		return s, nil
	}

	ret, err := appendSelects(archSelects, defaultSelectValue, ret)
	if err != nil {
		return "", err
	}

	ret, err = appendSelects(osSelects, defaultSelectValue, ret)
	return ret, err
}

// prettyPrintSelectMap converts a map of select keys to reflected Values as a generic way
// to construct a select map for any kind of attribute type.
func prettyPrintSelectMap(selectMap map[string]reflect.Value, defaultValue string, indent int) (string, error) {
	if selectMap == nil {
		return "", nil
	}

	// addConditionsDefault := false
	conditionsDefaultKey := bazel.PlatformArchMap[bazel.CONDITIONS_DEFAULT]

	var selects string
	for _, selectKey := range android.SortedStringKeys(selectMap) {
		if selectKey == conditionsDefaultKey {
			// Handle default condition later.
			continue
		}
		value := selectMap[selectKey]
		if isZero(value) {
			// Ignore zero values to not generate empty lists.
			continue
		}
		s, err := prettyPrintSelectEntry(value, selectKey, indent)
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
		// No conditions (or all values are empty lists), so no need for a map.
		return "", nil
	}

	// Create the map.
	ret := "select({\n"
	ret += selects

	// Handle the default condition
	s, err := prettyPrintSelectEntry(selectMap[conditionsDefaultKey], conditionsDefaultKey, indent)
	if err != nil {
		return "", err
	}
	if s == "" {
		// Print an explicit empty list (the default value) even if the value is
		// empty, to avoid errors about not finding a configuration that matches.
		ret += fmt.Sprintf("%s\"%s\": %s,\n", makeIndent(indent+1), "//conditions:default", defaultValue)
	} else {
		// Print the custom default value.
		ret += s
		ret += ",\n"
	}

	ret += makeIndent(indent)
	ret += "})"

	return ret, nil
}

// prettyPrintSelectEntry converts a reflect.Value into an entry in a select map
// with a provided key.
func prettyPrintSelectEntry(value reflect.Value, key string, indent int) (string, error) {
	s := makeIndent(indent + 1)
	v, err := prettyPrint(value, indent+1)
	if err != nil {
		return "", err
	}
	if v == "" {
		return "", nil
	}
	s += fmt.Sprintf("\"%s\": %s", key, v)
	return s, nil
}
