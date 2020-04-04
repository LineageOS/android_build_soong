// Copyright 2017 Google Inc. All rights reserved.
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

// This file implements the logic of bpfix and also provides a programmatic interface

package bpfix

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/google/blueprint/parser"
)

// Reformat takes a blueprint file as a string and returns a formatted version
func Reformat(input string) (string, error) {
	tree, err := parse("<string>", bytes.NewBufferString(input))
	if err != nil {
		return "", err
	}

	res, err := parser.Print(tree)
	if err != nil {
		return "", err
	}

	return string(res), nil
}

// A FixRequest specifies the details of which fixes to apply to an individual file
// A FixRequest doesn't specify whether to do a dry run or where to write the results; that's in cmd/bpfix.go
type FixRequest struct {
	steps []FixStep
}
type FixStepsExtension struct {
	Name  string
	Steps []FixStep
}

type FixStep struct {
	Name string
	Fix  func(f *Fixer) error
}

var fixStepsExtensions = []*FixStepsExtension(nil)

func RegisterFixStepExtension(extension *FixStepsExtension) {
	fixStepsExtensions = append(fixStepsExtensions, extension)
}

var fixSteps = []FixStep{
	{
		Name: "simplifyKnownRedundantVariables",
		Fix:  runPatchListMod(simplifyKnownPropertiesDuplicatingEachOther),
	},
	{
		Name: "rewriteIncorrectAndroidmkPrebuilts",
		Fix:  rewriteIncorrectAndroidmkPrebuilts,
	},
	{
		Name: "rewriteCtsModuleTypes",
		Fix:  rewriteCtsModuleTypes,
	},
	{
		Name: "rewriteIncorrectAndroidmkAndroidLibraries",
		Fix:  rewriteIncorrectAndroidmkAndroidLibraries,
	},
	{
		Name: "rewriteTestModuleTypes",
		Fix:  rewriteTestModuleTypes,
	},
	{
		Name: "rewriteAndroidmkJavaLibs",
		Fix:  rewriteAndroidmkJavaLibs,
	},
	{
		Name: "rewriteJavaStaticLibs",
		Fix:  rewriteJavaStaticLibs,
	},
	{
		Name: "rewritePrebuiltEtc",
		Fix:  rewriteAndroidmkPrebuiltEtc,
	},
	{
		Name: "mergeMatchingModuleProperties",
		Fix:  runPatchListMod(mergeMatchingModuleProperties),
	},
	{
		Name: "reorderCommonProperties",
		Fix:  runPatchListMod(reorderCommonProperties),
	},
	{
		Name: "removeTags",
		Fix:  runPatchListMod(removeTags),
	},
	{
		Name: "rewriteAndroidTest",
		Fix:  rewriteAndroidTest,
	},
	{
		Name: "rewriteAndroidAppImport",
		Fix:  rewriteAndroidAppImport,
	},
	{
		Name: "removeEmptyLibDependencies",
		Fix:  removeEmptyLibDependencies,
	},
	{
		Name: "removeHidlInterfaceTypes",
		Fix:  removeHidlInterfaceTypes,
	},
	{
		Name: "removeSoongConfigBoolVariable",
		Fix:  removeSoongConfigBoolVariable,
	},
}

func NewFixRequest() FixRequest {
	return FixRequest{}
}

func (r FixRequest) AddAll() (result FixRequest) {
	result.steps = append([]FixStep(nil), r.steps...)
	result.steps = append(result.steps, fixSteps...)
	for _, extension := range fixStepsExtensions {
		result.steps = append(result.steps, extension.Steps...)
	}
	return result
}

func (r FixRequest) AddBase() (result FixRequest) {
	result.steps = append([]FixStep(nil), r.steps...)
	result.steps = append(result.steps, fixSteps...)
	return result
}

func (r FixRequest) AddMatchingExtensions(pattern string) (result FixRequest) {
	result.steps = append([]FixStep(nil), r.steps...)
	for _, extension := range fixStepsExtensions {
		if match, _ := filepath.Match(pattern, extension.Name); match {
			result.steps = append(result.steps, extension.Steps...)
		}
	}
	return result
}

type Fixer struct {
	tree *parser.File
}

func (f Fixer) Tree() *parser.File {
	return f.tree
}

func NewFixer(tree *parser.File) *Fixer {
	fixer := &Fixer{tree}

	// make a copy of the tree
	fixer.reparse()

	return fixer
}

// Fix repeatedly applies the fixes listed in the given FixRequest to the given File
// until there is no fix that affects the tree
func (f *Fixer) Fix(config FixRequest) (*parser.File, error) {
	prevIdentifier, err := f.fingerprint()
	if err != nil {
		return nil, err
	}

	maxNumIterations := 20
	i := 0
	for {
		err = f.fixTreeOnce(config)
		newIdentifier, err := f.fingerprint()
		if err != nil {
			return nil, err
		}
		if bytes.Equal(newIdentifier, prevIdentifier) {
			break
		}
		prevIdentifier = newIdentifier
		// any errors from a previous iteration generally get thrown away and overwritten by errors on the next iteration

		// detect infinite loop
		i++
		if i >= maxNumIterations {
			return nil, fmt.Errorf("Applied fixes %d times and yet the tree continued to change. Is there an infinite loop?", i)
		}
	}
	return f.tree, err
}

// returns a unique identifier for the given tree that can be used to determine whether the tree changed
func (f *Fixer) fingerprint() (fingerprint []byte, err error) {
	bytes, err := parser.Print(f.tree)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func (f *Fixer) reparse() ([]byte, error) {
	buf, err := parser.Print(f.tree)
	if err != nil {
		return nil, err
	}
	newTree, err := parse(f.tree.Name, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	f.tree = newTree
	return buf, nil
}

func parse(name string, r io.Reader) (*parser.File, error) {
	tree, errs := parser.Parse(name, r, parser.NewScope(nil))
	if errs != nil {
		s := "parse error: "
		for _, err := range errs {
			s += "\n" + err.Error()
		}
		return nil, errors.New(s)
	}
	return tree, nil
}

func (f *Fixer) fixTreeOnce(config FixRequest) error {
	for _, fix := range config.steps {
		err := fix.Fix(f)
		if err != nil {
			return err
		}
	}
	return nil
}

func simplifyKnownPropertiesDuplicatingEachOther(mod *parser.Module, buf []byte, patchList *parser.PatchList) error {
	// remove from local_include_dirs anything in export_include_dirs
	return removeMatchingModuleListProperties(mod, patchList,
		"export_include_dirs", "local_include_dirs")
}

func rewriteIncorrectAndroidmkPrebuilts(f *Fixer) error {
	for _, def := range f.tree.Defs {
		mod, ok := def.(*parser.Module)
		if !ok {
			continue
		}
		if mod.Type != "java_import" {
			continue
		}
		host, _ := getLiteralBoolPropertyValue(mod, "host")
		if host {
			mod.Type = "java_import_host"
			removeProperty(mod, "host")
		}
		srcs, ok := getLiteralListProperty(mod, "srcs")
		if !ok {
			continue
		}
		if len(srcs.Values) == 0 {
			continue
		}
		src, ok := srcs.Values[0].(*parser.String)
		if !ok {
			continue
		}
		switch filepath.Ext(src.Value) {
		case ".jar":
			renameProperty(mod, "srcs", "jars")

		case ".aar":
			renameProperty(mod, "srcs", "aars")
			mod.Type = "android_library_import"

			// An android_library_import doesn't get installed, so setting "installable = false" isn't supported
			removeProperty(mod, "installable")
		}
	}

	return nil
}

func rewriteCtsModuleTypes(f *Fixer) error {
	for _, def := range f.tree.Defs {
		mod, ok := def.(*parser.Module)
		if !ok {
			continue
		}

		if mod.Type != "cts_support_package" && mod.Type != "cts_package" &&
			mod.Type != "cts_target_java_library" &&
			mod.Type != "cts_host_java_library" {

			continue
		}

		var defStr string
		switch mod.Type {
		case "cts_support_package":
			mod.Type = "android_test"
			defStr = "cts_support_defaults"
		case "cts_package":
			mod.Type = "android_test"
			defStr = "cts_defaults"
		case "cts_target_java_library":
			mod.Type = "java_library"
			defStr = "cts_defaults"
		case "cts_host_java_library":
			mod.Type = "java_library_host"
			defStr = "cts_defaults"
		}

		defaults := &parser.Property{
			Name: "defaults",
			Value: &parser.List{
				Values: []parser.Expression{
					&parser.String{
						Value: defStr,
					},
				},
			},
		}
		mod.Properties = append(mod.Properties, defaults)
	}

	return nil
}

func rewriteIncorrectAndroidmkAndroidLibraries(f *Fixer) error {
	for _, def := range f.tree.Defs {
		mod, ok := def.(*parser.Module)
		if !ok {
			continue
		}

		if !strings.HasPrefix(mod.Type, "java_") && !strings.HasPrefix(mod.Type, "android_") {
			continue
		}

		hasAndroidLibraries := hasNonEmptyLiteralListProperty(mod, "android_libs")
		hasStaticAndroidLibraries := hasNonEmptyLiteralListProperty(mod, "android_static_libs")
		hasResourceDirs := hasNonEmptyLiteralListProperty(mod, "resource_dirs")

		if hasAndroidLibraries || hasStaticAndroidLibraries || hasResourceDirs {
			if mod.Type == "java_library_static" || mod.Type == "java_library" {
				mod.Type = "android_library"
			}
		}

		if mod.Type == "java_import" && !hasStaticAndroidLibraries {
			removeProperty(mod, "android_static_libs")
		}

		// These may conflict with existing libs and static_libs properties, but the
		// mergeMatchingModuleProperties pass will fix it.
		renameProperty(mod, "shared_libs", "libs")
		renameProperty(mod, "android_libs", "libs")
		renameProperty(mod, "android_static_libs", "static_libs")
	}

	return nil
}

// rewriteTestModuleTypes looks for modules that are identifiable as tests but for which Make doesn't have a separate
// module class, and moves them to the appropriate Soong module type.
func rewriteTestModuleTypes(f *Fixer) error {
	for _, def := range f.tree.Defs {
		mod, ok := def.(*parser.Module)
		if !ok {
			continue
		}

		if !strings.HasPrefix(mod.Type, "java_") && !strings.HasPrefix(mod.Type, "android_") {
			continue
		}

		hasInstrumentationFor := hasNonEmptyLiteralStringProperty(mod, "instrumentation_for")
		tags, _ := getLiteralListPropertyValue(mod, "tags")

		var hasTestsTag bool
		for _, tag := range tags {
			if tag == "tests" {
				hasTestsTag = true
			}
		}

		isTest := hasInstrumentationFor || hasTestsTag

		if isTest {
			switch mod.Type {
			case "android_app":
				mod.Type = "android_test"
			case "java_library", "java_library_installable":
				mod.Type = "java_test"
			case "java_library_host":
				mod.Type = "java_test_host"
			}
		}
	}

	return nil
}

// rewriteJavaStaticLibs rewrites java_library_static into java_library
func rewriteJavaStaticLibs(f *Fixer) error {
	for _, def := range f.tree.Defs {
		mod, ok := def.(*parser.Module)
		if !ok {
			continue
		}

		if mod.Type == "java_library_static" {
			mod.Type = "java_library"
		}
	}

	return nil
}

// rewriteAndroidmkJavaLibs rewrites java_library_installable into java_library plus installable: true
func rewriteAndroidmkJavaLibs(f *Fixer) error {
	for _, def := range f.tree.Defs {
		mod, ok := def.(*parser.Module)
		if !ok {
			continue
		}

		if mod.Type != "java_library_installable" {
			continue
		}

		mod.Type = "java_library"

		_, hasInstallable := mod.GetProperty("installable")
		if !hasInstallable {
			prop := &parser.Property{
				Name: "installable",
				Value: &parser.Bool{
					Value: true,
				},
			}
			mod.Properties = append(mod.Properties, prop)
		}
	}

	return nil
}

// Helper function to get the value of a string-valued property in a given compound property.
func getStringProperty(prop *parser.Property, fieldName string) string {
	if propsAsMap, ok := prop.Value.(*parser.Map); ok {
		for _, propField := range propsAsMap.Properties {
			if fieldName == propField.Name {
				if propFieldAsString, ok := propField.Value.(*parser.String); ok {
					return propFieldAsString.Value
				} else {
					return ""
				}
			}
		}
	}
	return ""
}

// Set the value of the given attribute to the error message
func indicateAttributeError(mod *parser.Module, attributeName string, format string, a ...interface{}) error {
	msg := fmt.Sprintf(format, a...)
	mod.Properties = append(mod.Properties, &parser.Property{
		Name:  attributeName,
		Value: &parser.String{Value: "ERROR: " + msg},
	})
	return errors.New(msg)
}

// If a variable is LOCAL_MODULE, get its value from the 'name' attribute.
// This handles the statement
//    LOCAL_SRC_FILES := $(LOCAL_MODULE)
// which occurs often.
func resolveLocalModule(mod *parser.Module, val parser.Expression) parser.Expression {
	if varLocalName, ok := val.(*parser.Variable); ok {
		if varLocalName.Name == "LOCAL_MODULE" {
			if v, ok := getLiteralStringProperty(mod, "name"); ok {
				return v
			}
		}
	}
	return val
}

// etcPrebuiltModuleUpdate contains information on updating certain parts of a defined module such as:
//    * changing the module type from prebuilt_etc to a different one
//    * stripping the prefix of the install path based on the module type
//    * appending additional boolean properties to the prebuilt module
type etcPrebuiltModuleUpdate struct {
	// The prefix of the install path defined in local_module_path. The prefix is removed from local_module_path
	// before setting the 'filename' attribute.
	prefix string

	// There is only one prebuilt module type in makefiles. In Soong, there are multiple versions  of
	// prebuilts based on local_module_path. By default, it is "prebuilt_etc" if modType is blank. An
	// example is if the local_module_path contains $(TARGET_OUT)/usr/share, the module type is
	// considered as prebuilt_usr_share.
	modType string

	// Additional boolean attributes to be added in the prebuilt module. Each added boolean attribute
	// has a value of true.
	flags []string
}

func (f etcPrebuiltModuleUpdate) update(m *parser.Module, path string) bool {
	updated := false
	if path == f.prefix {
		updated = true
	} else if trimmedPath := strings.TrimPrefix(path, f.prefix+"/"); trimmedPath != path {
		m.Properties = append(m.Properties, &parser.Property{
			Name:  "sub_dir",
			Value: &parser.String{Value: trimmedPath},
		})
		updated = true
	}
	if updated {
		for _, flag := range f.flags {
			m.Properties = append(m.Properties, &parser.Property{Name: flag, Value: &parser.Bool{Value: true, Token: "true"}})
		}
		if f.modType != "" {
			m.Type = f.modType
		}
	}
	return updated
}

var localModuleUpdate = map[string][]etcPrebuiltModuleUpdate{
	"HOST_OUT":    {{prefix: "/etc", modType: "prebuilt_etc_host"}, {prefix: "/usr/share", modType: "prebuilt_usr_share_host"}},
	"PRODUCT_OUT": {{prefix: "/system/etc"}, {prefix: "/vendor/etc", flags: []string{"proprietary"}}},
	"TARGET_OUT": {{prefix: "/usr/share", modType: "prebuilt_usr_share"}, {prefix: "/fonts", modType: "prebuilt_font"},
		{prefix: "/etc/firmware", modType: "prebuilt_firmware"}, {prefix: "/vendor/firmware", modType: "prebuilt_firmware", flags: []string{"proprietary"}},
		{prefix: "/etc"}},
	"TARGET_OUT_ETC":            {{prefix: "/firmware", modType: "prebuilt_firmware"}, {prefix: ""}},
	"TARGET_OUT_PRODUCT":        {{prefix: "/etc", flags: []string{"product_specific"}}, {prefix: "/fonts", modType: "prebuilt_font", flags: []string{"product_specific"}}},
	"TARGET_OUT_PRODUCT_ETC":    {{prefix: "", flags: []string{"product_specific"}}},
	"TARGET_OUT_ODM":            {{prefix: "/etc", flags: []string{"device_specific"}}},
	"TARGET_OUT_SYSTEM_EXT":     {{prefix: "/etc", flags: []string{"system_ext_specific"}}},
	"TARGET_OUT_SYSTEM_EXT_ETC": {{prefix: "", flags: []string{"system_ext_specific"}}},
	"TARGET_OUT_VENDOR":         {{prefix: "/etc", flags: []string{"proprietary"}}, {prefix: "/firmware", modType: "prebuilt_firmware", flags: []string{"proprietary"}}},
	"TARGET_OUT_VENDOR_ETC":     {{prefix: "", flags: []string{"proprietary"}}},
	"TARGET_RECOVERY_ROOT_OUT":  {{prefix: "/system/etc", flags: []string{"recovery"}}},
}

// rewriteAndroidPrebuiltEtc fixes prebuilt_etc rule
func rewriteAndroidmkPrebuiltEtc(f *Fixer) error {
	for _, def := range f.tree.Defs {
		mod, ok := def.(*parser.Module)
		if !ok {
			continue
		}

		if mod.Type != "prebuilt_etc" && mod.Type != "prebuilt_etc_host" {
			continue
		}

		// 'srcs' --> 'src' conversion
		convertToSingleSource(mod, "src")

		// The rewriter converts LOCAL_MODULE_PATH attribute into a struct attribute
		// 'local_module_path'. Analyze its contents and create the correct sub_dir:,
		// filename: and boolean attributes combination
		const local_module_path = "local_module_path"
		if prop_local_module_path, ok := mod.GetProperty(local_module_path); ok {
			removeProperty(mod, local_module_path)
			prefixVariableName := getStringProperty(prop_local_module_path, "var")
			if moduleUpdates, ok := localModuleUpdate[prefixVariableName]; ok {
				path := getStringProperty(prop_local_module_path, "fixed")
				updated := false
				for i := 0; i < len(moduleUpdates) && !updated; i++ {
					updated = moduleUpdates[i].update(mod, path)
				}
				if !updated {
					expectedPrefices := ""
					sep := ""
					for _, moduleUpdate := range moduleUpdates {
						expectedPrefices += sep
						sep = ", "
						expectedPrefices += moduleUpdate.prefix
					}
					return indicateAttributeError(mod, "filename",
						"LOCAL_MODULE_PATH value under $(%s) should start with %s", prefixVariableName, expectedPrefices)
				}
			} else {
				return indicateAttributeError(mod, "filename", "Cannot handle $(%s) for the prebuilt_etc", prefixVariableName)
			}
		}
	}
	return nil
}

func rewriteAndroidTest(f *Fixer) error {
	for _, def := range f.tree.Defs {
		mod, ok := def.(*parser.Module)
		if !(ok && mod.Type == "android_test") {
			continue
		}
		// The rewriter converts LOCAL_MODULE_PATH attribute into a struct attribute
		// 'local_module_path'. For the android_test module, it should be  $(TARGET_OUT_DATA_APPS),
		// that is, `local_module_path: { var: "TARGET_OUT_DATA_APPS"}`
		const local_module_path = "local_module_path"
		if prop_local_module_path, ok := mod.GetProperty(local_module_path); ok {
			removeProperty(mod, local_module_path)
			prefixVariableName := getStringProperty(prop_local_module_path, "var")
			path := getStringProperty(prop_local_module_path, "fixed")
			if prefixVariableName == "TARGET_OUT_DATA_APPS" && path == "" {
				continue
			}
			return indicateAttributeError(mod, "filename",
				"Only LOCAL_MODULE_PATH := $(TARGET_OUT_DATA_APPS) is allowed for the android_test")
		}
	}
	return nil
}

func rewriteAndroidAppImport(f *Fixer) error {
	for _, def := range f.tree.Defs {
		mod, ok := def.(*parser.Module)
		if !(ok && mod.Type == "android_app_import") {
			continue
		}
		// 'srcs' --> 'apk' conversion
		convertToSingleSource(mod, "apk")
		// Handle special certificate value, "PRESIGNED".
		if cert, ok := mod.GetProperty("certificate"); ok {
			if certStr, ok := cert.Value.(*parser.String); ok {
				if certStr.Value == "PRESIGNED" {
					removeProperty(mod, "certificate")
					prop := &parser.Property{
						Name: "presigned",
						Value: &parser.Bool{
							Value: true,
						},
					}
					mod.Properties = append(mod.Properties, prop)
				}
			}
		}
	}
	return nil
}

// Removes library dependencies which are empty (and restricted from usage in Soong)
func removeEmptyLibDependencies(f *Fixer) error {
	emptyLibraries := []string{
		"libhidltransport",
		"libhwbinder",
	}
	relevantFields := []string{
		"export_shared_lib_headers",
		"export_static_lib_headers",
		"static_libs",
		"whole_static_libs",
		"shared_libs",
	}
	for _, def := range f.tree.Defs {
		mod, ok := def.(*parser.Module)
		if !ok {
			continue
		}
		for _, field := range relevantFields {
			listValue, ok := getLiteralListProperty(mod, field)
			if !ok {
				continue
			}
			newValues := []parser.Expression{}
			for _, v := range listValue.Values {
				stringValue, ok := v.(*parser.String)
				if !ok {
					return fmt.Errorf("Expecting string for %s.%s fields", mod.Type, field)
				}
				if inList(stringValue.Value, emptyLibraries) {
					continue
				}
				newValues = append(newValues, stringValue)
			}
			if len(newValues) == 0 && len(listValue.Values) != 0 {
				removeProperty(mod, field)
			} else {
				listValue.Values = newValues
			}
		}
	}
	return nil
}

// Removes hidl_interface 'types' which are no longer needed
func removeHidlInterfaceTypes(f *Fixer) error {
	for _, def := range f.tree.Defs {
		mod, ok := def.(*parser.Module)
		if !(ok && mod.Type == "hidl_interface") {
			continue
		}
		removeProperty(mod, "types")
	}
	return nil
}

func removeSoongConfigBoolVariable(f *Fixer) error {
	found := map[string]bool{}
	newDefs := make([]parser.Definition, 0, len(f.tree.Defs))
	for _, def := range f.tree.Defs {
		if mod, ok := def.(*parser.Module); ok && mod.Type == "soong_config_bool_variable" {
			if name, ok := getLiteralStringPropertyValue(mod, "name"); ok {
				found[name] = true
			} else {
				return fmt.Errorf("Found soong_config_bool_variable without a name")
			}
		} else {
			newDefs = append(newDefs, def)
		}
	}
	f.tree.Defs = newDefs

	if len(found) == 0 {
		return nil
	}

	return runPatchListMod(func(mod *parser.Module, buf []byte, patchList *parser.PatchList) error {
		if mod.Type != "soong_config_module_type" {
			return nil
		}

		variables, ok := getLiteralListProperty(mod, "variables")
		if !ok {
			return nil
		}

		boolValues := strings.Builder{}
		empty := true
		for _, item := range variables.Values {
			nameValue, ok := item.(*parser.String)
			if !ok {
				empty = false
				continue
			}
			if found[nameValue.Value] {
				patchList.Add(item.Pos().Offset, item.End().Offset+2, "")

				boolValues.WriteString(`"`)
				boolValues.WriteString(nameValue.Value)
				boolValues.WriteString(`",`)
			} else {
				empty = false
			}
		}
		if empty {
			*patchList = parser.PatchList{}

			prop, _ := mod.GetProperty("variables")
			patchList.Add(prop.Pos().Offset, prop.End().Offset+2, "")
		}
		if boolValues.Len() == 0 {
			return nil
		}

		bool_variables, ok := getLiteralListProperty(mod, "bool_variables")
		if ok {
			patchList.Add(bool_variables.RBracePos.Offset, bool_variables.RBracePos.Offset, ","+boolValues.String())
		} else {
			patchList.Add(variables.RBracePos.Offset+2, variables.RBracePos.Offset+2,
				fmt.Sprintf(`bool_variables: [%s],`, boolValues.String()))
		}

		return nil
	})(f)

	return nil
}

// Converts the default source list property, 'srcs', to a single source property with a given name.
// "LOCAL_MODULE" reference is also resolved during the conversion process.
func convertToSingleSource(mod *parser.Module, srcPropertyName string) {
	if srcs, ok := mod.GetProperty("srcs"); ok {
		if srcList, ok := srcs.Value.(*parser.List); ok {
			removeProperty(mod, "srcs")
			if len(srcList.Values) == 1 {
				mod.Properties = append(mod.Properties,
					&parser.Property{
						Name:     srcPropertyName,
						NamePos:  srcs.NamePos,
						ColonPos: srcs.ColonPos,
						Value:    resolveLocalModule(mod, srcList.Values[0])})
			} else if len(srcList.Values) > 1 {
				indicateAttributeError(mod, srcPropertyName, "LOCAL_SRC_FILES should contain at most one item")
			}
		} else if _, ok = srcs.Value.(*parser.Variable); ok {
			removeProperty(mod, "srcs")
			mod.Properties = append(mod.Properties,
				&parser.Property{Name: srcPropertyName,
					NamePos:  srcs.NamePos,
					ColonPos: srcs.ColonPos,
					Value:    resolveLocalModule(mod, srcs.Value)})
		} else {
			renameProperty(mod, "srcs", "apk")
		}
	}
}

func runPatchListMod(modFunc func(mod *parser.Module, buf []byte, patchlist *parser.PatchList) error) func(*Fixer) error {
	return func(f *Fixer) error {
		// Make sure all the offsets are accurate
		buf, err := f.reparse()
		if err != nil {
			return err
		}

		var patchlist parser.PatchList
		for _, def := range f.tree.Defs {
			mod, ok := def.(*parser.Module)
			if !ok {
				continue
			}

			err := modFunc(mod, buf, &patchlist)
			if err != nil {
				return err
			}
		}

		newBuf := new(bytes.Buffer)
		err = patchlist.Apply(bytes.NewReader(buf), newBuf)
		if err != nil {
			return err
		}

		// Save a copy of the buffer to print for errors below
		bufCopy := append([]byte(nil), newBuf.Bytes()...)

		newTree, err := parse(f.tree.Name, newBuf)
		if err != nil {
			return fmt.Errorf("Failed to parse: %v\nBuffer:\n%s", err, string(bufCopy))
		}

		f.tree = newTree

		return nil
	}
}

var commonPropertyPriorities = []string{
	"name",
	"defaults",
	"device_supported",
	"host_supported",
	"installable",
}

func reorderCommonProperties(mod *parser.Module, buf []byte, patchlist *parser.PatchList) error {
	if len(mod.Properties) == 0 {
		return nil
	}

	pos := mod.LBracePos.Offset + 1
	stage := ""

	for _, name := range commonPropertyPriorities {
		idx := propertyIndex(mod.Properties, name)
		if idx == -1 {
			continue
		}
		if idx == 0 {
			err := patchlist.Add(pos, pos, stage)
			if err != nil {
				return err
			}
			stage = ""

			pos = mod.Properties[0].End().Offset + 1
			mod.Properties = mod.Properties[1:]
			continue
		}

		prop := mod.Properties[idx]
		mod.Properties = append(mod.Properties[:idx], mod.Properties[idx+1:]...)

		stage += string(buf[prop.Pos().Offset : prop.End().Offset+1])

		err := patchlist.Add(prop.Pos().Offset, prop.End().Offset+2, "")
		if err != nil {
			return err
		}
	}

	if stage != "" {
		err := patchlist.Add(pos, pos, stage)
		if err != nil {
			return err
		}
	}

	return nil
}

func removeTags(mod *parser.Module, buf []byte, patchlist *parser.PatchList) error {
	prop, ok := mod.GetProperty("tags")
	if !ok {
		return nil
	}
	list, ok := prop.Value.(*parser.List)
	if !ok {
		return nil
	}

	replaceStr := ""

	for _, item := range list.Values {
		str, ok := item.(*parser.String)
		if !ok {
			replaceStr += fmt.Sprintf("// ERROR: Unable to parse tag %q\n", item)
			continue
		}

		switch str.Value {
		case "optional":
			continue
		case "debug":
			replaceStr += `// WARNING: Module tags are not supported in Soong.
				// Add this module to PRODUCT_PACKAGES_DEBUG in your product file if you want to
				// force installation for -userdebug and -eng builds.
				`
		case "eng":
			replaceStr += `// WARNING: Module tags are not supported in Soong.
				// Add this module to PRODUCT_PACKAGES_ENG in your product file if you want to
				// force installation for -eng builds.
				`
		case "tests":
			switch {
			case strings.Contains(mod.Type, "cc_test"),
				strings.Contains(mod.Type, "cc_library_static"),
				strings.Contains(mod.Type, "java_test"),
				mod.Type == "android_test":
				continue
			case strings.Contains(mod.Type, "cc_lib"):
				replaceStr += `// WARNING: Module tags are not supported in Soong.
					// To make a shared library only for tests, use the "cc_test_library" module
					// type. If you don't use gtest, set "gtest: false".
					`
			case strings.Contains(mod.Type, "cc_bin"):
				replaceStr += `// WARNING: Module tags are not supported in Soong.
					// For native test binaries, use the "cc_test" module type. Some differences:
					//  - If you don't use gtest, set "gtest: false"
					//  - Binaries will be installed into /data/nativetest[64]/<name>/<name>
					//  - Both 32 & 64 bit versions will be built (as appropriate)
					`
			case strings.Contains(mod.Type, "java_lib"):
				replaceStr += `// WARNING: Module tags are not supported in Soong.
					// For JUnit or similar tests, use the "java_test" module type. A dependency on
					// Junit will be added by default, if it is using some other runner, set "junit: false".
					`
			case mod.Type == "android_app":
				replaceStr += `// WARNING: Module tags are not supported in Soong.
					// For JUnit or instrumentataion app tests, use the "android_test" module type.
					`
			default:
				replaceStr += `// WARNING: Module tags are not supported in Soong.
					// In most cases, tests are now identified by their module type:
					// cc_test, java_test, python_test
					`
			}
		default:
			replaceStr += fmt.Sprintf("// WARNING: Unknown module tag %q\n", str.Value)
		}
	}

	return patchlist.Add(prop.Pos().Offset, prop.End().Offset+2, replaceStr)
}

func mergeMatchingModuleProperties(mod *parser.Module, buf []byte, patchlist *parser.PatchList) error {
	return mergeMatchingProperties(&mod.Properties, buf, patchlist)
}

func mergeMatchingProperties(properties *[]*parser.Property, buf []byte, patchlist *parser.PatchList) error {
	seen := make(map[string]*parser.Property)
	for i := 0; i < len(*properties); i++ {
		property := (*properties)[i]
		if prev, exists := seen[property.Name]; exists {
			err := mergeProperties(prev, property, buf, patchlist)
			if err != nil {
				return err
			}
			*properties = append((*properties)[:i], (*properties)[i+1:]...)
		} else {
			seen[property.Name] = property
			if mapProperty, ok := property.Value.(*parser.Map); ok {
				err := mergeMatchingProperties(&mapProperty.Properties, buf, patchlist)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func mergeProperties(a, b *parser.Property, buf []byte, patchlist *parser.PatchList) error {
	// The value of one of the properties may be a variable reference with no type assigned
	// Bail out in this case. Soong will notice duplicate entries and will tell to merge them.
	if _, isVar := a.Value.(*parser.Variable); isVar {
		return nil
	}
	if _, isVar := b.Value.(*parser.Variable); isVar {
		return nil
	}
	if a.Value.Type() != b.Value.Type() {
		return fmt.Errorf("type mismatch when merging properties %q: %s and %s", a.Name, a.Value.Type(), b.Value.Type())
	}

	switch a.Value.Type() {
	case parser.StringType:
		return fmt.Errorf("conflicting definitions of string property %q", a.Name)
	case parser.ListType:
		return mergeListProperties(a, b, buf, patchlist)
	}

	return nil
}

func mergeListProperties(a, b *parser.Property, buf []byte, patchlist *parser.PatchList) error {
	aval, oka := a.Value.(*parser.List)
	bval, okb := b.Value.(*parser.List)
	if !oka || !okb {
		// Merging expressions not supported yet
		return nil
	}

	s := string(buf[bval.LBracePos.Offset+1 : bval.RBracePos.Offset])
	if bval.LBracePos.Line != bval.RBracePos.Line {
		if s[0] != '\n' {
			panic("expected \n")
		}
		// If B is a multi line list, skip the first "\n" in case A already has a trailing "\n"
		s = s[1:]
	}
	if aval.LBracePos.Line == aval.RBracePos.Line {
		// A is a single line list with no trailing comma
		if len(aval.Values) > 0 {
			s = "," + s
		}
	}

	err := patchlist.Add(aval.RBracePos.Offset, aval.RBracePos.Offset, s)
	if err != nil {
		return err
	}
	err = patchlist.Add(b.NamePos.Offset, b.End().Offset+2, "")
	if err != nil {
		return err
	}

	return nil
}

// removes from <items> every item present in <removals>
func filterExpressionList(patchList *parser.PatchList, items *parser.List, removals *parser.List) {
	writeIndex := 0
	for _, item := range items.Values {
		included := true
		for _, removal := range removals.Values {
			equal, err := parser.ExpressionsAreSame(item, removal)
			if err != nil {
				continue
			}
			if equal {
				included = false
				break
			}
		}
		if included {
			items.Values[writeIndex] = item
			writeIndex++
		} else {
			patchList.Add(item.Pos().Offset, item.End().Offset+2, "")
		}
	}
	items.Values = items.Values[:writeIndex]
}

// Remove each modules[i].Properties[<legacyName>][j] that matches a modules[i].Properties[<canonicalName>][k]
func removeMatchingModuleListProperties(mod *parser.Module, patchList *parser.PatchList, canonicalName string, legacyName string) error {
	legacyProp, ok := mod.GetProperty(legacyName)
	if !ok {
		return nil
	}
	legacyList, ok := legacyProp.Value.(*parser.List)
	if !ok || len(legacyList.Values) == 0 {
		return nil
	}
	canonicalList, ok := getLiteralListProperty(mod, canonicalName)
	if !ok {
		return nil
	}

	localPatches := parser.PatchList{}
	filterExpressionList(&localPatches, legacyList, canonicalList)

	if len(legacyList.Values) == 0 {
		patchList.Add(legacyProp.Pos().Offset, legacyProp.End().Offset+2, "")
	} else {
		for _, p := range localPatches {
			patchList.Add(p.Start, p.End, p.Replacement)
		}
	}

	return nil
}

func hasNonEmptyLiteralListProperty(mod *parser.Module, name string) bool {
	list, found := getLiteralListProperty(mod, name)
	return found && len(list.Values) > 0
}

func hasNonEmptyLiteralStringProperty(mod *parser.Module, name string) bool {
	s, found := getLiteralStringPropertyValue(mod, name)
	return found && len(s) > 0
}

func getLiteralListProperty(mod *parser.Module, name string) (list *parser.List, found bool) {
	prop, ok := mod.GetProperty(name)
	if !ok {
		return nil, false
	}
	list, ok = prop.Value.(*parser.List)
	return list, ok
}

func getLiteralListPropertyValue(mod *parser.Module, name string) (list []string, found bool) {
	listValue, ok := getLiteralListProperty(mod, name)
	if !ok {
		return nil, false
	}
	for _, v := range listValue.Values {
		stringValue, ok := v.(*parser.String)
		if !ok {
			return nil, false
		}
		list = append(list, stringValue.Value)
	}

	return list, true
}

func getLiteralStringProperty(mod *parser.Module, name string) (s *parser.String, found bool) {
	prop, ok := mod.GetProperty(name)
	if !ok {
		return nil, false
	}
	s, ok = prop.Value.(*parser.String)
	return s, ok
}

func getLiteralStringPropertyValue(mod *parser.Module, name string) (s string, found bool) {
	stringValue, ok := getLiteralStringProperty(mod, name)
	if !ok {
		return "", false
	}

	return stringValue.Value, true
}

func getLiteralBoolProperty(mod *parser.Module, name string) (b *parser.Bool, found bool) {
	prop, ok := mod.GetProperty(name)
	if !ok {
		return nil, false
	}
	b, ok = prop.Value.(*parser.Bool)
	return b, ok
}

func getLiteralBoolPropertyValue(mod *parser.Module, name string) (s bool, found bool) {
	boolValue, ok := getLiteralBoolProperty(mod, name)
	if !ok {
		return false, false
	}

	return boolValue.Value, true
}

func propertyIndex(props []*parser.Property, propertyName string) int {
	for i, prop := range props {
		if prop.Name == propertyName {
			return i
		}
	}
	return -1
}

func renameProperty(mod *parser.Module, from, to string) {
	for _, prop := range mod.Properties {
		if prop.Name == from {
			prop.Name = to
		}
	}
}

func removeProperty(mod *parser.Module, propertyName string) {
	newList := make([]*parser.Property, 0, len(mod.Properties))
	for _, prop := range mod.Properties {
		if prop.Name != propertyName {
			newList = append(newList, prop)
		}
	}
	mod.Properties = newList
}

func inList(s string, list []string) bool {
	for _, v := range list {
		if s == v {
			return true
		}
	}
	return false
}
