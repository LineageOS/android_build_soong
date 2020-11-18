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

package main

import (
	"android/soong/android"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap/bpdoc"
	"github.com/google/blueprint/proptools"
)

var (
	// An allowlist of prop types that are surfaced from module props to rule
	// attributes. (nested) dictionaries are notably absent here, because while
	// Soong supports multi value typed and nested dictionaries, Bazel's rule
	// attr() API supports only single-level string_dicts.
	allowedPropTypes = map[string]bool{
		"int":         true, // e.g. 42
		"bool":        true, // e.g. True
		"string_list": true, // e.g. ["a", "b"]
		"string":      true, // e.g. "a"
	}

	// Certain module property names are blocklisted/ignored here, for the reasons commented.
	ignoredPropNames = map[string]bool{
		"name":       true, // redundant, since this is explicitly generated for every target
		"from":       true, // reserved keyword
		"in":         true, // reserved keyword
		"arch":       true, // interface prop type is not supported yet.
		"multilib":   true, // interface prop type is not supported yet.
		"target":     true, // interface prop type is not supported yet.
		"visibility": true, // Bazel has native visibility semantics. Handle later.
		"features":   true, // There is already a built-in attribute 'features' which cannot be overridden.
	}
)

func targetNameWithVariant(c *blueprint.Context, logicModule blueprint.Module) string {
	name := ""
	if c.ModuleSubDir(logicModule) != "" {
		// TODO(b/162720883): Figure out a way to drop the "--" variant suffixes.
		name = c.ModuleName(logicModule) + "--" + c.ModuleSubDir(logicModule)
	} else {
		name = c.ModuleName(logicModule)
	}

	return strings.Replace(name, "//", "", 1)
}

func qualifiedTargetLabel(c *blueprint.Context, logicModule blueprint.Module) string {
	return "//" +
		packagePath(c, logicModule) +
		":" +
		targetNameWithVariant(c, logicModule)
}

func packagePath(c *blueprint.Context, logicModule blueprint.Module) string {
	return filepath.Dir(c.BlueprintFile(logicModule))
}

func escapeString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(s, "\"", "\\\"")
}

func makeIndent(indent int) string {
	if indent < 0 {
		panic(fmt.Errorf("indent column cannot be less than 0, but got %d", indent))
	}
	return strings.Repeat("    ", indent)
}

// prettyPrint a property value into the equivalent Starlark representation
// recursively.
func prettyPrint(propertyValue reflect.Value, indent int) (string, error) {
	if isZero(propertyValue) {
		// A property value being set or unset actually matters -- Soong does set default
		// values for unset properties, like system_shared_libs = ["libc", "libm", "libdl"] at
		// https://cs.android.com/android/platform/superproject/+/master:build/soong/cc/linker.go;l=281-287;drc=f70926eef0b9b57faf04c17a1062ce50d209e480
		//
		// In Bazel-parlance, we would use "attr.<type>(default = <default value>)" to set the default
		// value of unset attributes.
		return "", nil
	}

	var ret string
	switch propertyValue.Kind() {
	case reflect.String:
		ret = fmt.Sprintf("\"%v\"", escapeString(propertyValue.String()))
	case reflect.Bool:
		ret = strings.Title(fmt.Sprintf("%v", propertyValue.Interface()))
	case reflect.Int, reflect.Uint, reflect.Int64:
		ret = fmt.Sprintf("%v", propertyValue.Interface())
	case reflect.Ptr:
		return prettyPrint(propertyValue.Elem(), indent)
	case reflect.Slice:
		ret = "[\n"
		for i := 0; i < propertyValue.Len(); i++ {
			indexedValue, err := prettyPrint(propertyValue.Index(i), indent+1)
			if err != nil {
				return "", err
			}

			if indexedValue != "" {
				ret += makeIndent(indent + 1)
				ret += indexedValue
				ret += ",\n"
			}
		}
		ret += makeIndent(indent)
		ret += "]"
	case reflect.Struct:
		ret = "{\n"
		// Sort and print the struct props by the key.
		structProps := extractStructProperties(propertyValue, indent)
		for _, k := range android.SortedStringKeys(structProps) {
			ret += makeIndent(indent + 1)
			ret += fmt.Sprintf("%q: %s,\n", k, structProps[k])
		}
		ret += makeIndent(indent)
		ret += "}"
	case reflect.Interface:
		// TODO(b/164227191): implement pretty print for interfaces.
		// Interfaces are used for for arch, multilib and target properties.
		return "", nil
	default:
		return "", fmt.Errorf(
			"unexpected kind for property struct field: %s", propertyValue.Kind())
	}
	return ret, nil
}

// Converts a reflected property struct value into a map of property names and property values,
// which each property value correctly pretty-printed and indented at the right nest level,
// since property structs can be nested. In Starlark, nested structs are represented as nested
// dicts: https://docs.bazel.build/skylark/lib/dict.html
func extractStructProperties(structValue reflect.Value, indent int) map[string]string {
	if structValue.Kind() != reflect.Struct {
		panic(fmt.Errorf("Expected a reflect.Struct type, but got %s", structValue.Kind()))
	}

	ret := map[string]string{}
	structType := structValue.Type()
	for i := 0; i < structValue.NumField(); i++ {
		field := structType.Field(i)
		if field.PkgPath != "" {
			// Skip unexported fields. Some properties are
			// internal to Soong only, and these fields do not have PkgPath.
			continue
		}
		if proptools.HasTag(field, "blueprint", "mutated") {
			continue
		}

		fieldValue := structValue.Field(i)
		if isZero(fieldValue) {
			// Ignore zero-valued fields
			continue
		}

		propertyName := proptools.PropertyNameForField(field.Name)
		prettyPrintedValue, err := prettyPrint(fieldValue, indent+1)
		if err != nil {
			panic(
				fmt.Errorf(
					"Error while parsing property: %q. %s",
					propertyName,
					err))
		}
		if prettyPrintedValue != "" {
			ret[propertyName] = prettyPrintedValue
		}
	}

	return ret
}

func isStructPtr(t reflect.Type) bool {
	return t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Struct
}

// Generically extract module properties and types into a map, keyed by the module property name.
func extractModuleProperties(aModule android.Module) map[string]string {
	ret := map[string]string{}

	// Iterate over this android.Module's property structs.
	for _, properties := range aModule.GetProperties() {
		propertiesValue := reflect.ValueOf(properties)
		// Check that propertiesValue is a pointer to the Properties struct, like
		// *cc.BaseLinkerProperties or *java.CompilerProperties.
		//
		// propertiesValue can also be type-asserted to the structs to
		// manipulate internal props, if needed.
		if isStructPtr(propertiesValue.Type()) {
			structValue := propertiesValue.Elem()
			for k, v := range extractStructProperties(structValue, 0) {
				ret[k] = v
			}
		} else {
			panic(fmt.Errorf(
				"properties must be a pointer to a struct, got %T",
				propertiesValue.Interface()))
		}

	}

	return ret
}

// FIXME(b/168089390): In Bazel, rules ending with "_test" needs to be marked as
// testonly = True, forcing other rules that depend on _test rules to also be
// marked as testonly = True. This semantic constraint is not present in Soong.
// To work around, rename "*_test" rules to "*_test_".
func canonicalizeModuleType(moduleName string) string {
	if strings.HasSuffix(moduleName, "_test") {
		return moduleName + "_"
	}

	return moduleName
}

type RuleShim struct {
	// The rule class shims contained in a bzl file. e.g. ["cc_object", "cc_library", ..]
	rules []string

	// The generated string content of the bzl file.
	content string
}

// Create <module>.bzl containing Bazel rule shims for every module type available in Soong and
// user-specified Go plugins.
//
// This function reuses documentation generation APIs to ensure parity between modules-as-docs
// and modules-as-code, including the names and types of module properties.
func createRuleShims(packages []*bpdoc.Package) (map[string]RuleShim, error) {
	var propToAttr func(prop bpdoc.Property, propName string) string
	propToAttr = func(prop bpdoc.Property, propName string) string {
		// dots are not allowed in Starlark attribute names. Substitute them with double underscores.
		propName = strings.ReplaceAll(propName, ".", "__")
		if !shouldGenerateAttribute(propName) {
			return ""
		}

		// Canonicalize and normalize module property types to Bazel attribute types
		starlarkAttrType := prop.Type
		if starlarkAttrType == "list of string" {
			starlarkAttrType = "string_list"
		} else if starlarkAttrType == "int64" {
			starlarkAttrType = "int"
		} else if starlarkAttrType == "" {
			var attr string
			for _, nestedProp := range prop.Properties {
				nestedAttr := propToAttr(nestedProp, propName+"__"+nestedProp.Name)
				if nestedAttr != "" {
					// TODO(b/167662930): Fix nested props resulting in too many attributes.
					// Let's still generate these, but comment them out.
					attr += "# " + nestedAttr
				}
			}
			return attr
		}

		if !allowedPropTypes[starlarkAttrType] {
			return ""
		}

		return fmt.Sprintf("        %q: attr.%s(),\n", propName, starlarkAttrType)
	}

	ruleShims := map[string]RuleShim{}
	for _, pkg := range packages {
		content := "load(\"//build/bazel/queryview_rules:providers.bzl\", \"SoongModuleInfo\")\n"

		bzlFileName := strings.ReplaceAll(pkg.Path, "android/soong/", "")
		bzlFileName = strings.ReplaceAll(bzlFileName, ".", "_")
		bzlFileName = strings.ReplaceAll(bzlFileName, "/", "_")

		rules := []string{}

		for _, moduleTypeTemplate := range moduleTypeDocsToTemplates(pkg.ModuleTypes) {
			attrs := `{
        "module_name": attr.string(mandatory = True),
        "module_variant": attr.string(),
        "module_deps": attr.label_list(providers = [SoongModuleInfo]),
`
			for _, prop := range moduleTypeTemplate.Properties {
				attrs += propToAttr(prop, prop.Name)
			}

			moduleTypeName := moduleTypeTemplate.Name

			// Certain SDK-related module types dynamically inject properties, instead of declaring
			// them as structs. These properties are registered in an SdkMemberTypesRegistry. If
			// the module type name matches, add these properties into the rule definition.
			var registeredTypes []android.SdkMemberType
			if moduleTypeName == "module_exports" || moduleTypeName == "module_exports_snapshot" {
				registeredTypes = android.ModuleExportsMemberTypes.RegisteredTypes()
			} else if moduleTypeName == "sdk" || moduleTypeName == "sdk_snapshot" {
				registeredTypes = android.SdkMemberTypes.RegisteredTypes()
			}
			for _, memberType := range registeredTypes {
				attrs += fmt.Sprintf("        %q: attr.string_list(),\n", memberType.SdkPropertyName())
			}

			attrs += "    },"

			rule := canonicalizeModuleType(moduleTypeTemplate.Name)
			content += fmt.Sprintf(moduleRuleShim, rule, attrs)
			rules = append(rules, rule)
		}

		ruleShims[bzlFileName] = RuleShim{content: content, rules: rules}
	}
	return ruleShims, nil
}

func createBazelQueryView(ctx *android.Context, bazelQueryViewDir string) error {
	blueprintCtx := ctx.Context
	blueprintCtx.VisitAllModules(func(module blueprint.Module) {
		buildFile, err := buildFileForModule(blueprintCtx, module, bazelQueryViewDir)
		if err != nil {
			panic(err)
		}

		buildFile.Write([]byte(generateSoongModuleTarget(blueprintCtx, module) + "\n\n"))
		buildFile.Close()
	})
	var err error

	// Write top level files: WORKSPACE and BUILD. These files are empty.
	if err = writeReadOnlyFile(bazelQueryViewDir, "WORKSPACE", ""); err != nil {
		return err
	}

	// Used to denote that the top level directory is a package.
	if err = writeReadOnlyFile(bazelQueryViewDir, "BUILD", ""); err != nil {
		return err
	}

	packages, err := getPackages(ctx)
	if err != nil {
		return err
	}
	ruleShims, err := createRuleShims(packages)
	if err != nil {
		return err
	}

	// Write .bzl Starlark files into the bazel_rules top level directory (provider and rule definitions)
	bazelRulesDir := bazelQueryViewDir + "/build/bazel/queryview_rules"
	if err = writeReadOnlyFile(bazelRulesDir, "BUILD", ""); err != nil {
		return err
	}
	if err = writeReadOnlyFile(bazelRulesDir, "providers.bzl", providersBzl); err != nil {
		return err
	}

	for bzlFileName, ruleShim := range ruleShims {
		if err = writeReadOnlyFile(bazelRulesDir, bzlFileName+".bzl", ruleShim.content); err != nil {
			return err
		}
	}

	return writeReadOnlyFile(bazelRulesDir, "soong_module.bzl", generateSoongModuleBzl(ruleShims))
}

// Generate the content of soong_module.bzl with the rule shim load statements
// and mapping of module_type to rule shim map for every module type in Soong.
func generateSoongModuleBzl(bzlLoads map[string]RuleShim) string {
	var loadStmts string
	var moduleRuleMap string
	for bzlFileName, ruleShim := range bzlLoads {
		loadStmt := "load(\"//build/bazel/queryview_rules:"
		loadStmt += bzlFileName
		loadStmt += ".bzl\""
		for _, rule := range ruleShim.rules {
			loadStmt += fmt.Sprintf(", %q", rule)
			moduleRuleMap += "    \"" + rule + "\": " + rule + ",\n"
		}
		loadStmt += ")\n"
		loadStmts += loadStmt
	}

	return fmt.Sprintf(soongModuleBzl, loadStmts, moduleRuleMap)
}

func shouldGenerateAttribute(prop string) bool {
	return !ignoredPropNames[prop]
}

// props is an unsorted map. This function ensures that
// the generated attributes are sorted to ensure determinism.
func propsToAttributes(props map[string]string) string {
	var attributes string
	for _, propName := range android.SortedStringKeys(props) {
		if shouldGenerateAttribute(propName) {
			attributes += fmt.Sprintf("    %s = %s,\n", propName, props[propName])
		}
	}
	return attributes
}

// Convert a module and its deps and props into a Bazel macro/rule
// representation in the BUILD file.
func generateSoongModuleTarget(
	blueprintCtx *blueprint.Context,
	module blueprint.Module) string {

	var props map[string]string
	if aModule, ok := module.(android.Module); ok {
		props = extractModuleProperties(aModule)
	}
	attributes := propsToAttributes(props)

	// TODO(b/163018919): DirectDeps can have duplicate (module, variant)
	// items, if the modules are added using different DependencyTag. Figure
	// out the implications of that.
	depLabels := map[string]bool{}
	blueprintCtx.VisitDirectDeps(module, func(depModule blueprint.Module) {
		depLabels[qualifiedTargetLabel(blueprintCtx, depModule)] = true
	})

	depLabelList := "[\n"
	for depLabel, _ := range depLabels {
		depLabelList += fmt.Sprintf("        %q,\n", depLabel)
	}
	depLabelList += "    ]"

	return fmt.Sprintf(
		soongModuleTarget,
		targetNameWithVariant(blueprintCtx, module),
		blueprintCtx.ModuleName(module),
		canonicalizeModuleType(blueprintCtx.ModuleType(module)),
		blueprintCtx.ModuleSubDir(module),
		depLabelList,
		attributes)
}

func buildFileForModule(
	ctx *blueprint.Context, module blueprint.Module, bazelQueryViewDir string) (*os.File, error) {
	// Create nested directories for the BUILD file
	dirPath := filepath.Join(bazelQueryViewDir, packagePath(ctx, module))
	createDirectoryIfNonexistent(dirPath)
	// Open the file for appending, and create it if it doesn't exist
	f, err := os.OpenFile(
		filepath.Join(dirPath, "BUILD.bazel"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644)
	if err != nil {
		return nil, err
	}

	// If the file is empty, add the load statement for the `soong_module` rule
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if fi.Size() == 0 {
		f.Write([]byte(soongModuleLoad + "\n"))
	}

	return f, nil
}

func createDirectoryIfNonexistent(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, os.ModePerm)
	}
}

// The QueryView directory should be read-only, sufficient for bazel query. The files
// are not intended to be edited by end users.
func writeReadOnlyFile(dir string, baseName string, content string) error {
	createDirectoryIfNonexistent(dir)
	pathToFile := filepath.Join(dir, baseName)
	// 0444 is read-only
	return ioutil.WriteFile(pathToFile, []byte(content), 0444)
}

func isZero(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Func, reflect.Map, reflect.Slice:
		return value.IsNil()
	case reflect.Array:
		valueIsZero := true
		for i := 0; i < value.Len(); i++ {
			valueIsZero = valueIsZero && isZero(value.Index(i))
		}
		return valueIsZero
	case reflect.Struct:
		valueIsZero := true
		for i := 0; i < value.NumField(); i++ {
			if value.Field(i).CanSet() {
				valueIsZero = valueIsZero && isZero(value.Field(i))
			}
		}
		return valueIsZero
	case reflect.Ptr:
		if !value.IsNil() {
			return isZero(reflect.Indirect(value))
		} else {
			return true
		}
	default:
		zeroValue := reflect.Zero(value.Type())
		result := value.Interface() == zeroValue.Interface()
		return result
	}
}
