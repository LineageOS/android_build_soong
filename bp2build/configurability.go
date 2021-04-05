package bp2build

import (
	"android/soong/android"
	"android/soong/bazel"
	"fmt"
	"reflect"
)

// Configurability support for bp2build.

// prettyPrintStringListAttribute converts a StringListAttribute to its Bazel
// syntax. May contain a select statement.
func prettyPrintStringListAttribute(stringList bazel.StringListAttribute, indent int) (string, error) {
	ret, err := prettyPrint(reflect.ValueOf(stringList.Value), indent)
	if err != nil {
		return ret, err
	}

	if !stringList.HasConfigurableValues() {
		// Select statement not needed.
		return ret, nil
	}

	// Create the selects for arch specific values.
	selects := map[string]reflect.Value{}
	for arch, selectKey := range bazel.PlatformArchMap {
		selects[selectKey] = reflect.ValueOf(stringList.GetValueForArch(arch))
	}

	selectMap, err := prettyPrintSelectMap(selects, "[]", indent)
	return ret + selectMap, err
}

// prettyPrintLabelListAttribute converts a LabelListAttribute to its Bazel
// syntax. May contain select statements.
func prettyPrintLabelListAttribute(labels bazel.LabelListAttribute, indent int) (string, error) {
	// TODO(b/165114590): convert glob syntax
	ret, err := prettyPrint(reflect.ValueOf(labels.Value.Includes), indent)
	if err != nil {
		return ret, err
	}

	if !labels.HasConfigurableValues() {
		// Select statements not needed.
		return ret, nil
	}

	// Create the selects for arch specific values.
	archSelects := map[string]reflect.Value{}
	for arch, selectKey := range bazel.PlatformArchMap {
		archSelects[selectKey] = reflect.ValueOf(labels.GetValueForArch(arch).Includes)
	}
	selectMap, err := prettyPrintSelectMap(archSelects, "[]", indent)
	if err != nil {
		return "", err
	}
	ret += selectMap

	// Create the selects for target os specific values.
	osSelects := map[string]reflect.Value{}
	for os, selectKey := range bazel.PlatformOsMap {
		osSelects[selectKey] = reflect.ValueOf(labels.GetValueForOS(os).Includes)
	}
	selectMap, err = prettyPrintSelectMap(osSelects, "[]", indent)
	return ret + selectMap, err
}

// prettyPrintSelectMap converts a map of select keys to reflected Values as a generic way
// to construct a select map for any kind of attribute type.
func prettyPrintSelectMap(selectMap map[string]reflect.Value, defaultValue string, indent int) (string, error) {
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
	ret := " + select({\n"
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
