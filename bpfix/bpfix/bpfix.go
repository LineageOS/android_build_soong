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
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/google/blueprint/parser"
	"github.com/google/blueprint/pathtools"
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
	{
		Name: "removePdkProperty",
		Fix:  runPatchListMod(removeObsoleteProperty("product_variables.pdk")),
	},
	{
		Name: "removeScudoProperty",
		Fix:  runPatchListMod(removeObsoleteProperty("sanitize.scudo")),
	},
	{
		Name: "removeAndroidLicenseKinds",
		Fix:  runPatchListMod(removeIncorrectProperties("android_license_kinds")),
	},
	{
		Name: "removeAndroidLicenseConditions",
		Fix:  runPatchListMod(removeIncorrectProperties("android_license_conditions")),
	},
	{
		Name: "removeAndroidLicenseFiles",
		Fix:  runPatchListMod(removeIncorrectProperties("android_license_files")),
	},
	{
		Name: "formatFlagProperties",
		Fix:  runPatchListMod(formatFlagProperties),
	},
	{
		Name: "removeResourcesAndAssetsIfDefault",
		Fix:  removeResourceAndAssetsIfDefault,
	},
}

// for fix that only need to run once
var fixStepsOnce = []FixStep{
	{
		Name: "haveSameLicense",
		Fix:  haveSameLicense,
	},
	{
		Name: "rewriteLicenseProperties",
		Fix:  runPatchListMod(rewriteLicenseProperty(nil, "")),
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

	// run fix that is expected to run once first
	configOnce := NewFixRequest()
	configOnce.steps = append(configOnce.steps, fixStepsOnce...)
	if len(configOnce.steps) > 0 {
		err = f.fixTreeOnce(configOnce)
		if err != nil {
			return nil, err
		}
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
	tree, errs := parser.Parse(name, r)
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
			mod.Type = "android_test_helper_app"
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

		if !strings.HasPrefix(mod.Type, "java_") && !strings.HasPrefix(mod.Type, "android_") && mod.Type != "cc_binary" {
			continue
		}

		hasInstrumentationFor := hasNonEmptyLiteralStringProperty(mod, "instrumentation_for")
		hasTestSuites := hasNonEmptyLiteralListProperty(mod, "test_suites")
		tags, _ := getLiteralListPropertyValue(mod, "tags")

		var hasTestsTag bool
		for _, tag := range tags {
			if tag == "tests" {
				hasTestsTag = true
			}
		}

		isTest := hasInstrumentationFor || hasTestsTag || hasTestSuites

		if isTest {
			switch mod.Type {
			case "android_app":
				mod.Type = "android_test"
			case "android_app_import":
				mod.Type = "android_test_import"
			case "java_library", "java_library_installable":
				mod.Type = "java_test"
			case "java_library_host":
				mod.Type = "java_test_host"
			case "cc_binary":
				mod.Type = "cc_test"
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
//
//	LOCAL_SRC_FILES := $(LOCAL_MODULE)
//
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
//   - changing the module type from prebuilt_etc to a different one
//   - stripping the prefix of the install path based on the module type
//   - appending additional boolean properties to the prebuilt module
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
			Name:  "relative_install_path",
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
	"HOST_OUT": {
		{prefix: "/etc", modType: "prebuilt_etc_host"},
		{prefix: "/usr/share", modType: "prebuilt_usr_share_host"},
		{prefix: "", modType: "prebuilt_root_host"},
	},
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

		renameProperty(mod, "sub_dir", "relative_install_path")

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
		if !ok {
			// The definition is not a module.
			continue
		}
		if mod.Type != "android_test" && mod.Type != "android_test_helper_app" {
			// The module is not an android_test or android_test_helper_app.
			continue
		}
		// The rewriter converts LOCAL_MODULE_PATH attribute into a struct attribute
		// 'local_module_path'. For the android_test module, it should be  $(TARGET_OUT_DATA_APPS),
		// that is, `local_module_path: { var: "TARGET_OUT_DATA_APPS"}`
		// 1. if the `key: val` pair matches, (key is `local_module_path`,
		//    and val is `{ var: "TARGET_OUT_DATA_APPS"}`), this property is removed;
		// 2. o/w, an error msg is thrown.
		const local_module_path = "local_module_path"
		if prop_local_module_path, ok := mod.GetProperty(local_module_path); ok {
			removeProperty(mod, local_module_path)
			prefixVariableName := getStringProperty(prop_local_module_path, "var")
			path := getStringProperty(prop_local_module_path, "fixed")
			if prefixVariableName == "TARGET_OUT_DATA_APPS" && path == "" {
				continue
			}
			return indicateAttributeError(mod, "filename",
				"Only LOCAL_MODULE_PATH := $(TARGET_OUT_DATA_APPS) is allowed for the %s", mod.Type)
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

func RewriteRuntimeResourceOverlay(f *Fixer) error {
	for _, def := range f.tree.Defs {
		mod, ok := def.(*parser.Module)
		if !(ok && mod.Type == "runtime_resource_overlay") {
			continue
		}
		// runtime_resource_overlays are always product specific in Make.
		if _, ok := mod.GetProperty("product_specific"); !ok {
			prop := &parser.Property{
				Name: "product_specific",
				Value: &parser.Bool{
					Value: true,
				},
			}
			mod.Properties = append(mod.Properties, prop)
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

		boolVariables, ok := getLiteralListProperty(mod, "bool_variables")
		if ok {
			patchList.Add(boolVariables.RBracePos.Offset, boolVariables.RBracePos.Offset, ","+boolValues.String())
		} else {
			patchList.Add(variables.RBracePos.Offset+2, variables.RBracePos.Offset+2,
				fmt.Sprintf(`bool_variables: [%s],`, boolValues.String()))
		}

		return nil
	})(f)

	return nil
}

func removeResourceAndAssetsIfDefault(f *Fixer) error {
	for _, def := range f.tree.Defs {
		mod, ok := def.(*parser.Module)
		if !ok {
			continue
		}
		resourceDirList, resourceDirFound := getLiteralListPropertyValue(mod, "resource_dirs")
		if resourceDirFound && len(resourceDirList) == 1 && resourceDirList[0] == "res" {
			removeProperty(mod, "resource_dirs")
		}
		assetDirList, assetDirFound := getLiteralListPropertyValue(mod, "asset_dirs")
		if assetDirFound && len(assetDirList) == 1 && assetDirList[0] == "assets" {
			removeProperty(mod, "asset_dirs")
		}
	}
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

type patchListModFunction func(*parser.Module, []byte, *parser.PatchList) error

func runPatchListMod(modFunc patchListModFunction) func(*Fixer) error {
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
				mod.Type == "android_test",
				mod.Type == "android_test_import":
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

type propertyProvider interface {
	GetProperty(string) (*parser.Property, bool)
	RemoveProperty(string) bool
}

func removeNestedProperty(mod *parser.Module, patchList *parser.PatchList, propName string) error {
	propNames := strings.Split(propName, ".")

	var propProvider, toRemoveFrom propertyProvider
	propProvider = mod

	var propToRemove *parser.Property
	for i, name := range propNames {
		p, ok := propProvider.GetProperty(name)
		if !ok {
			return nil
		}
		// if this is the inner most element, it's time to delete
		if i == len(propNames)-1 {
			if propToRemove == nil {
				// if we cannot remove the properties that the current property is nested in,
				// remove only the current property
				propToRemove = p
				toRemoveFrom = propProvider
			}

			// remove the property from the list, in case we remove other properties in this list
			toRemoveFrom.RemoveProperty(propToRemove.Name)
			// only removing the property would leave blank line(s), remove with a patch
			if err := patchList.Add(propToRemove.Pos().Offset, propToRemove.End().Offset+2, ""); err != nil {
				return err
			}
		} else {
			propMap, ok := p.Value.(*parser.Map)
			if !ok {
				return nil
			}
			if len(propMap.Properties) > 1 {
				// if there are other properties in this struct, we need to keep this struct
				toRemoveFrom = nil
				propToRemove = nil
			} else if propToRemove == nil {
				// otherwise, we can remove the empty struct entirely
				toRemoveFrom = propProvider
				propToRemove = p
			}
			propProvider = propMap
		}
	}

	return nil
}

func removeObsoleteProperty(propName string) patchListModFunction {
	return func(mod *parser.Module, buf []byte, patchList *parser.PatchList) error {
		return removeNestedProperty(mod, patchList, propName)
	}
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

func formatFlagProperty(mod *parser.Module, field string, buf []byte, patchlist *parser.PatchList) error {
	// the comment or empty lines in the value of the field are skipped
	listValue, ok := getLiteralListProperty(mod, field)
	if !ok {
		// if do not find
		return nil
	}
	for i := 0; i < len(listValue.Values); i++ {
		curValue, ok := listValue.Values[i].(*parser.String)
		if !ok {
			return fmt.Errorf("Expecting string for %s.%s fields", mod.Type, field)
		}
		if !strings.HasPrefix(curValue.Value, "-") {
			return fmt.Errorf("Expecting the string `%s` starting with '-'", curValue.Value)
		}
		if i+1 < len(listValue.Values) {
			nextValue, ok := listValue.Values[i+1].(*parser.String)
			if !ok {
				return fmt.Errorf("Expecting string for %s.%s fields", mod.Type, field)
			}
			if !strings.HasPrefix(nextValue.Value, "-") {
				// delete the line
				err := patchlist.Add(curValue.Pos().Offset, curValue.End().Offset+2, "")
				if err != nil {
					return err
				}
				// replace the line
				value := "\"" + curValue.Value + " " + nextValue.Value + "\","
				err = patchlist.Add(nextValue.Pos().Offset, nextValue.End().Offset+1, value)
				if err != nil {
					return err
				}
				// combined two lines to one
				i++
			}
		}
	}
	return nil
}

func formatFlagProperties(mod *parser.Module, buf []byte, patchlist *parser.PatchList) error {
	relevantFields := []string{
		// cc flags
		"asflags",
		"cflags",
		"clang_asflags",
		"clang_cflags",
		"conlyflags",
		"cppflags",
		"ldflags",
		"tidy_flags",
		// java flags
		"aaptflags",
		"dxflags",
		"javacflags",
		"kotlincflags",
	}
	for _, field := range relevantFields {
		err := formatFlagProperty(mod, field, buf, patchlist)
		if err != nil {
			return err
		}
	}
	return nil
}

func rewriteLicenseProperty(fs pathtools.FileSystem, relativePath string) patchListModFunction {
	return func(mod *parser.Module, buf []byte, patchList *parser.PatchList) error {
		return rewriteLicenseProperties(mod, patchList, fs, relativePath)
	}
}

// rewrite the "android_license_kinds" and "android_license_files" properties to a package module
// (and a license module when needed).
func rewriteLicenseProperties(mod *parser.Module, patchList *parser.PatchList, fs pathtools.FileSystem,
	relativePath string) error {
	// if a package module has been added, no more action is needed.
	for _, patch := range *patchList {
		if strings.Contains(patch.Replacement, "package {") {
			return nil
		}
	}

	// initial the fs
	if fs == nil {
		fs = pathtools.NewOsFs(os.Getenv("ANDROID_BUILD_TOP"))
	}

	// initial the relativePath
	if len(relativePath) == 0 {
		relativePath = getModuleRelativePath()
	}
	// validate the relativePath
	ok := hasFile(relativePath+"/Android.mk", fs)
	// some modules in the existing test cases in the androidmk_test.go do not have a valid path
	if !ok && len(relativePath) > 0 {
		return fmt.Errorf("Cannot find an Android.mk file at path %q", relativePath)
	}

	licenseKindsPropertyName := "android_license_kinds"
	licenseFilesPropertyName := "android_license_files"

	androidBpFileErr := "// Error: No Android.bp file is found at path\n" +
		"// %s\n" +
		"// Please add one there with the needed license module first.\n" +
		"// Then reset the default_applicable_licenses property below with the license module name.\n"
	licenseModuleErr := "// Error: Cannot get the name of the license module in the\n" +
		"// %s file.\n" +
		"// If no such license module exists, please add one there first.\n" +
		"// Then reset the default_applicable_licenses property below with the license module name.\n"

	defaultApplicableLicense := "Android-Apache-2.0"
	var licenseModuleName, licensePatch string
	var hasFileInParentDir bool

	// when LOCAL_NOTICE_FILE is not empty
	if hasNonEmptyLiteralListProperty(mod, licenseFilesPropertyName) {
		hasFileInParentDir = hasValueStartWithTwoDotsLiteralList(mod, licenseFilesPropertyName)
		// if have LOCAL_NOTICE_FILE outside the current directory, need to find and refer to the license
		// module in the LOCAL_NOTICE_FILE location directly and no new license module needs to be created
		if hasFileInParentDir {
			bpPath, ok := getPathFromProperty(mod, licenseFilesPropertyName, fs, relativePath)
			if !ok {
				bpDir, err := getDirFromProperty(mod, licenseFilesPropertyName, fs, relativePath)
				if err != nil {
					return err
				}
				licensePatch += fmt.Sprintf(androidBpFileErr, bpDir)
				defaultApplicableLicense = ""
			} else {
				licenseModuleName, _ = getModuleName(bpPath, "license", fs)
				if len(licenseModuleName) == 0 {
					licensePatch += fmt.Sprintf(licenseModuleErr, bpPath)
				}
				defaultApplicableLicense = licenseModuleName
			}
		} else {
			// if have LOCAL_NOTICE_FILE in the current directory, need to create a new license module
			if len(relativePath) == 0 {
				return fmt.Errorf("Cannot obtain the relative path of the Android.mk file")
			}
			licenseModuleName = strings.Replace(relativePath, "/", "_", -1) + "_license"
			defaultApplicableLicense = licenseModuleName
		}
	}

	//add the package module
	if hasNonEmptyLiteralListProperty(mod, licenseKindsPropertyName) {
		licensePatch += "package {\n" +
			"    // See: http://go/android-license-faq\n" +
			"    default_applicable_licenses: [\n" +
			"         \"" + defaultApplicableLicense + "\",\n" +
			"    ],\n" +
			"}\n" +
			"\n"
	}

	// append the license module when necessary
	// when LOCAL_NOTICE_FILE is not empty and in the current directory, create a new license module
	// otherwise, use the above default license directly
	if hasNonEmptyLiteralListProperty(mod, licenseFilesPropertyName) && !hasFileInParentDir {
		licenseKinds, err := mergeLiteralListPropertyValue(mod, licenseKindsPropertyName)
		if err != nil {
			return err
		}
		licenseFiles, err := mergeLiteralListPropertyValue(mod, licenseFilesPropertyName)
		if err != nil {
			return err
		}
		licensePatch += "license {\n" +
			"    name: \"" + licenseModuleName + "\",\n" +
			"    visibility: [\":__subpackages__\"],\n" +
			"    license_kinds: [\n" +
			licenseKinds +
			"    ],\n" +
			"    license_text: [\n" +
			licenseFiles +
			"    ],\n" +
			"}\n" +
			"\n"
	}

	// add to the patchList
	pos := mod.Pos().Offset
	err := patchList.Add(pos, pos, licensePatch)
	if err != nil {
		return err
	}
	return nil
}

// merge the string vaules in a list property of a module into one string with expected format
func mergeLiteralListPropertyValue(mod *parser.Module, property string) (s string, err error) {
	listValue, ok := getLiteralListPropertyValue(mod, property)
	if !ok {
		// if do not find
		return "", fmt.Errorf("Cannot retrieve the %s.%s field", mod.Type, property)
	}
	for i := 0; i < len(listValue); i++ {
		s += "         \"" + listValue[i] + "\",\n"
	}
	return s, nil
}

// check whether a string list property has any value starting with `../`
func hasValueStartWithTwoDotsLiteralList(mod *parser.Module, property string) bool {
	listValue, ok := getLiteralListPropertyValue(mod, property)
	if ok {
		for i := 0; i < len(listValue); i++ {
			if strings.HasPrefix(listValue[i], "../") {
				return true
			}
		}
	}
	return false
}

// get the relative path from ANDROID_BUILD_TOP to the Android.mk file to be converted
func getModuleRelativePath() string {
	// get the absolute path of the top of the tree
	rootPath := os.Getenv("ANDROID_BUILD_TOP")
	// get the absolute path of the `Android.mk` file to be converted
	absPath := getModuleAbsolutePath()
	// get the relative path of the `Android.mk` file to top of the tree
	relModulePath, err := filepath.Rel(rootPath, absPath)
	if err != nil {
		return ""
	}
	return relModulePath
}

// get the absolute path of the Android.mk file to be converted
func getModuleAbsolutePath() string {
	// get the absolute path at where the `androidmk` commend is executed
	curAbsPath, err := filepath.Abs(".")
	if err != nil {
		return ""
	}
	// the argument for the `androidmk` command could be
	// 1. "./a/b/c/Android.mk"; 2. "a/b/c/Android.mk"; 3. "Android.mk"
	argPath := flag.Arg(0)
	if strings.HasPrefix(argPath, "./") {
		argPath = strings.TrimPrefix(argPath, ".")
	}
	argPath = strings.TrimSuffix(argPath, "Android.mk")
	if strings.HasSuffix(argPath, "/") {
		argPath = strings.TrimSuffix(argPath, "/")
	}
	if len(argPath) > 0 && !strings.HasPrefix(argPath, "/") {
		argPath = "/" + argPath
	}
	// get the absolute path of the `Android.mk` file to be converted
	absPath := curAbsPath + argPath
	return absPath
}

// check whether a file exists in a filesystem
func hasFile(path string, fs pathtools.FileSystem) bool {
	ok, _, _ := fs.Exists(path)
	return ok
}

// get the directory where an `Android.bp` file and the property files are expected to locate
func getDirFromProperty(mod *parser.Module, property string, fs pathtools.FileSystem, relativePath string) (string, error) {
	listValue, ok := getLiteralListPropertyValue(mod, property)
	if !ok {
		// if do not find
		return "", fmt.Errorf("Cannot retrieve the %s.%s property", mod.Type, property)
	}
	if len(listValue) == 0 {
		// if empty
		return "", fmt.Errorf("Cannot find the value of the %s.%s property", mod.Type, property)
	}
	if relativePath == "" {
		relativePath = "."
	}
	_, isDir, _ := fs.Exists(relativePath)
	if !isDir {
		return "", fmt.Errorf("Cannot find the path %q", relativePath)
	}
	path := relativePath
	for {
		if !strings.HasPrefix(listValue[0], "../") {
			break
		}
		path = filepath.Dir(path)
		listValue[0] = strings.TrimPrefix(listValue[0], "../")
	}
	_, isDir, _ = fs.Exists(path)
	if !isDir {
		return "", fmt.Errorf("Cannot find the path %q", path)
	}
	return path, nil
}

// get the path of the `Android.bp` file at the expected location where the property files locate
func getPathFromProperty(mod *parser.Module, property string, fs pathtools.FileSystem, relativePath string) (string, bool) {
	dir, err := getDirFromProperty(mod, property, fs, relativePath)
	if err != nil {
		return "", false
	}
	ok := hasFile(dir+"/Android.bp", fs)
	if !ok {
		return "", false
	}
	return dir + "/Android.bp", true
}

// parse an Android.bp file to get the name of the first module with type of moduleType
func getModuleName(path string, moduleType string, fs pathtools.FileSystem) (string, error) {
	tree, err := parserPath(path, fs)
	if err != nil {
		return "", err
	}
	for _, def := range tree.Defs {
		mod, ok := def.(*parser.Module)
		if !ok || mod.Type != moduleType {
			continue
		}
		prop, ok := mod.GetProperty("name")
		if !ok {
			return "", fmt.Errorf("Cannot get the %s."+"name property", mod.Type)
		}
		propVal, ok := prop.Value.(*parser.String)
		if ok {
			return propVal.Value, nil
		}
	}
	return "", fmt.Errorf("Cannot find the value of the %s."+"name property", moduleType)
}

// parse an Android.bp file with the specific path
func parserPath(path string, fs pathtools.FileSystem) (tree *parser.File, err error) {
	f, err := fs.Open(path)
	if err != nil {
		return tree, err
	}
	defer f.Close()
	fileContent, _ := ioutil.ReadAll(f)
	tree, err = parse(path, bytes.NewBufferString(string(fileContent)))
	if err != nil {
		return tree, err
	}
	return tree, nil
}

// remove the incorrect property that Soong does not support
func removeIncorrectProperties(propName string) patchListModFunction {
	return removeObsoleteProperty(propName)
}

// the modules on the same Android.mk file are expected to have the same license
func haveSameLicense(f *Fixer) error {
	androidLicenseProperties := []string{
		"android_license_kinds",
		"android_license_conditions",
		"android_license_files",
	}

	var prevModuleName string
	var prevLicenseKindsVals, prevLicenseConditionsVals, prevLicenseFilesVals []string
	prevLicenseVals := [][]string{
		prevLicenseKindsVals,
		prevLicenseConditionsVals,
		prevLicenseFilesVals,
	}

	for _, def := range f.tree.Defs {
		mod, ok := def.(*parser.Module)
		if !ok {
			continue
		}
		for idx, property := range androidLicenseProperties {
			curModuleName, ok := getLiteralStringPropertyValue(mod, "name")
			// some modules in the existing test cases in the androidmk_test.go do not have name property
			hasNameProperty := hasProperty(mod, "name")
			if hasNameProperty && (!ok || len(curModuleName) == 0) {
				return fmt.Errorf("Cannot retrieve the name property of a module of %s type.", mod.Type)
			}
			curVals, ok := getLiteralListPropertyValue(mod, property)
			// some modules in the existing test cases in the androidmk_test.go do not have license-related property
			hasLicenseProperty := hasProperty(mod, property)
			if hasLicenseProperty && (!ok || len(curVals) == 0) {
				// if do not find the property, or no value is found for the property
				return fmt.Errorf("Cannot retrieve the %s.%s property", mod.Type, property)
			}
			if len(prevLicenseVals[idx]) > 0 {
				if !reflect.DeepEqual(prevLicenseVals[idx], curVals) {
					return fmt.Errorf("Modules %s and %s are expected to have the same %s property.",
						prevModuleName, curModuleName, property)
				}
			}
			sort.Strings(curVals)
			prevLicenseVals[idx] = curVals
			prevModuleName = curModuleName
		}
	}
	return nil
}

func hasProperty(mod *parser.Module, propName string) bool {
	_, ok := mod.GetProperty(propName)
	return ok
}
