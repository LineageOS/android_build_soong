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

	switch list := v.(type) {
	case bazel.StringListAttribute:
		value, archSelects, osSelects = getStringListValues(list)
	case bazel.LabelListAttribute:
		value, archSelects, osSelects = getLabelListValues(list)
	default:
		return "", fmt.Errorf("Not a supported Bazel attribute type: %s", v)
	}

	ret, err := prettyPrint(value, indent)
	if err != nil {
		return ret, err
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

	ret, err = appendSelects(archSelects, "[]", ret)
	if err != nil {
		return "", err
	}

	ret, err = appendSelects(osSelects, "[]", ret)
	return ret, err
}

// prettyPrintSelectMap converts a map of select keys to reflected Values as a generic way
// to construct a select map for any kind of attribute type.
func prettyPrintSelectMap(selectMap map[string]reflect.Value, defaultValue string, indent int) (string, error) {
	if selectMap == nil {
		return "", nil
	}

	var selects string
	for _, selectKey := range android.SortedStringKeys(selectMap) {
		value := selectMap[selectKey]
		if isZero(value) {
			// Ignore zero values to not generate empty lists.
			continue
		}
		s, err := prettyPrintSelectEntry(value, selectKey, indent)
		if err != nil {
			return "", err
		}
		selects += s + ",\n"
	}

	if len(selects) == 0 {
		// No conditions (or all values are empty lists), so no need for a map.
		return "", nil
	}

	// Create the map.
	ret := "select({\n"
	ret += selects
	// default condition comes last.
	ret += fmt.Sprintf("%s\"%s\": %s,\n", makeIndent(indent+1), "//conditions:default", defaultValue)
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
	s += fmt.Sprintf("\"%s\": %s", key, v)
	return s, nil
}
