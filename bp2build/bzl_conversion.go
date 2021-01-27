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

package bp2build

import (
	"android/soong/android"
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strings"

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
)

type rule struct {
	name  string
	attrs string
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
// and modules-as-code, including the names and types of morule properties.
func CreateRuleShims(moduleTypeFactories map[string]android.ModuleFactory) map[string]RuleShim {
	ruleShims := map[string]RuleShim{}
	for pkg, rules := range generateRules(moduleTypeFactories) {
		shim := RuleShim{
			rules: make([]string, 0, len(rules)),
		}
		shim.content = "load(\"//build/bazel/queryview_rules:providers.bzl\", \"SoongModuleInfo\")\n"

		bzlFileName := strings.ReplaceAll(pkg, "android/soong/", "")
		bzlFileName = strings.ReplaceAll(bzlFileName, ".", "_")
		bzlFileName = strings.ReplaceAll(bzlFileName, "/", "_")

		for _, r := range rules {
			shim.content += fmt.Sprintf(moduleRuleShim, r.name, r.attrs)
			shim.rules = append(shim.rules, r.name)
		}
		sort.Strings(shim.rules)
		ruleShims[bzlFileName] = shim
	}
	return ruleShims
}

// Generate the content of soong_module.bzl with the rule shim load statements
// and mapping of module_type to rule shim map for every module type in Soong.
func generateSoongModuleBzl(bzlLoads map[string]RuleShim) string {
	var loadStmts string
	var moduleRuleMap string
	for _, bzlFileName := range android.SortedStringKeys(bzlLoads) {
		loadStmt := "load(\"//build/bazel/queryview_rules:"
		loadStmt += bzlFileName
		loadStmt += ".bzl\""
		ruleShim := bzlLoads[bzlFileName]
		for _, rule := range ruleShim.rules {
			loadStmt += fmt.Sprintf(", %q", rule)
			moduleRuleMap += "    \"" + rule + "\": " + rule + ",\n"
		}
		loadStmt += ")\n"
		loadStmts += loadStmt
	}

	return fmt.Sprintf(soongModuleBzl, loadStmts, moduleRuleMap)
}

func generateRules(moduleTypeFactories map[string]android.ModuleFactory) map[string][]rule {
	// TODO: add shims for bootstrap/blueprint go modules types

	rules := make(map[string][]rule)
	// TODO: allow registration of a bzl rule when registring a factory
	for _, moduleType := range android.SortedStringKeys(moduleTypeFactories) {
		factory := moduleTypeFactories[moduleType]
		factoryName := runtime.FuncForPC(reflect.ValueOf(factory).Pointer()).Name()
		pkg := strings.Split(factoryName, ".")[0]
		attrs := `{
        "soong_module_name": attr.string(mandatory = True),
        "soong_module_variant": attr.string(),
        "soong_module_deps": attr.label_list(providers = [SoongModuleInfo]),
`
		attrs += getAttributes(factory)
		attrs += "    },"

		r := rule{
			name:  canonicalizeModuleType(moduleType),
			attrs: attrs,
		}

		rules[pkg] = append(rules[pkg], r)
	}
	return rules
}

type property struct {
	name             string
	starlarkAttrType string
	properties       []property
}

const (
	attributeIndent = "        "
)

func (p *property) attributeString() string {
	if !shouldGenerateAttribute(p.name) {
		return ""
	}

	if _, ok := allowedPropTypes[p.starlarkAttrType]; !ok {
		// a struct -- let's just comment out sub-props
		s := fmt.Sprintf(attributeIndent+"# %s start\n", p.name)
		for _, nestedP := range p.properties {
			s += "# " + nestedP.attributeString()
		}
		s += fmt.Sprintf(attributeIndent+"# %s end\n", p.name)
		return s
	}
	return fmt.Sprintf(attributeIndent+"%q: attr.%s(),\n", p.name, p.starlarkAttrType)
}

func extractPropertyDescriptionsFromStruct(structType reflect.Type) []property {
	properties := make([]property, 0)
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		if shouldSkipStructField(field) {
			continue
		}

		properties = append(properties, extractPropertyDescriptions(field.Name, field.Type)...)
	}
	return properties
}

func extractPropertyDescriptions(name string, t reflect.Type) []property {
	name = proptools.PropertyNameForField(name)

	// TODO: handle android:paths tags, they should be changed to label types

	starlarkAttrType := fmt.Sprintf("%s", t.Name())
	props := make([]property, 0)

	switch t.Kind() {
	case reflect.Bool, reflect.String:
		// do nothing
	case reflect.Uint, reflect.Int, reflect.Int64:
		starlarkAttrType = "int"
	case reflect.Slice:
		if t.Elem().Kind() != reflect.String {
			// TODO: handle lists of non-strings (currently only list of Dist)
			return []property{}
		}
		starlarkAttrType = "string_list"
	case reflect.Struct:
		props = extractPropertyDescriptionsFromStruct(t)
	case reflect.Ptr:
		return extractPropertyDescriptions(name, t.Elem())
	case reflect.Interface:
		// Interfaces are used for for arch, multilib and target properties, which are handled at runtime.
		// These will need to be handled in a bazel-specific version of the arch mutator.
		return []property{}
	}

	prop := property{
		name:             name,
		starlarkAttrType: starlarkAttrType,
		properties:       props,
	}

	return []property{prop}
}

func getPropertyDescriptions(props []interface{}) []property {
	// there may be duplicate properties, e.g. from defaults libraries
	propertiesByName := make(map[string]property)
	for _, p := range props {
		for _, prop := range extractPropertyDescriptionsFromStruct(reflect.ValueOf(p).Elem().Type()) {
			propertiesByName[prop.name] = prop
		}
	}

	properties := make([]property, 0, len(propertiesByName))
	for _, key := range android.SortedStringKeys(propertiesByName) {
		properties = append(properties, propertiesByName[key])
	}

	return properties
}

func getAttributes(factory android.ModuleFactory) string {
	attrs := ""
	for _, p := range getPropertyDescriptions(factory().GetProperties()) {
		attrs += p.attributeString()
	}
	return attrs
}
