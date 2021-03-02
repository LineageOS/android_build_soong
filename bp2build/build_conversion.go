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
	"android/soong/bazel"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

type BazelAttributes struct {
	Attrs map[string]string
}

type BazelTarget struct {
	name            string
	content         string
	ruleClass       string
	bzlLoadLocation string
}

// IsLoadedFromStarlark determines if the BazelTarget's rule class is loaded from a .bzl file,
// as opposed to a native rule built into Bazel.
func (t BazelTarget) IsLoadedFromStarlark() bool {
	return t.bzlLoadLocation != ""
}

// BazelTargets is a typedef for a slice of BazelTarget objects.
type BazelTargets []BazelTarget

// String returns the string representation of BazelTargets, without load
// statements (use LoadStatements for that), since the targets are usually not
// adjacent to the load statements at the top of the BUILD file.
func (targets BazelTargets) String() string {
	var res string
	for i, target := range targets {
		res += target.content
		if i != len(targets)-1 {
			res += "\n\n"
		}
	}
	return res
}

// LoadStatements return the string representation of the sorted and deduplicated
// Starlark rule load statements needed by a group of BazelTargets.
func (targets BazelTargets) LoadStatements() string {
	bzlToLoadedSymbols := map[string][]string{}
	for _, target := range targets {
		if target.IsLoadedFromStarlark() {
			bzlToLoadedSymbols[target.bzlLoadLocation] =
				append(bzlToLoadedSymbols[target.bzlLoadLocation], target.ruleClass)
		}
	}

	var loadStatements []string
	for bzl, ruleClasses := range bzlToLoadedSymbols {
		loadStatement := "load(\""
		loadStatement += bzl
		loadStatement += "\", "
		ruleClasses = android.SortedUniqueStrings(ruleClasses)
		for i, ruleClass := range ruleClasses {
			loadStatement += "\"" + ruleClass + "\""
			if i != len(ruleClasses)-1 {
				loadStatement += ", "
			}
		}
		loadStatement += ")"
		loadStatements = append(loadStatements, loadStatement)
	}
	return strings.Join(android.SortedUniqueStrings(loadStatements), "\n")
}

type bpToBuildContext interface {
	ModuleName(module blueprint.Module) string
	ModuleDir(module blueprint.Module) string
	ModuleSubDir(module blueprint.Module) string
	ModuleType(module blueprint.Module) string

	VisitAllModules(visit func(blueprint.Module))
	VisitDirectDeps(module blueprint.Module, visit func(blueprint.Module))
}

type CodegenContext struct {
	config  android.Config
	context android.Context
	mode    CodegenMode
}

func (c *CodegenContext) Mode() CodegenMode {
	return c.mode
}

// CodegenMode is an enum to differentiate code-generation modes.
type CodegenMode int

const (
	// Bp2Build: generate BUILD files with targets buildable by Bazel directly.
	//
	// This mode is used for the Soong->Bazel build definition conversion.
	Bp2Build CodegenMode = iota

	// QueryView: generate BUILD files with targets representing fully mutated
	// Soong modules, representing the fully configured Soong module graph with
	// variants and dependency endges.
	//
	// This mode is used for discovering and introspecting the existing Soong
	// module graph.
	QueryView
)

func (mode CodegenMode) String() string {
	switch mode {
	case Bp2Build:
		return "Bp2Build"
	case QueryView:
		return "QueryView"
	default:
		return fmt.Sprintf("%d", mode)
	}
}

func (ctx CodegenContext) AddNinjaFileDeps(...string) {}
func (ctx CodegenContext) Config() android.Config     { return ctx.config }
func (ctx CodegenContext) Context() android.Context   { return ctx.context }

// NewCodegenContext creates a wrapper context that conforms to PathContext for
// writing BUILD files in the output directory.
func NewCodegenContext(config android.Config, context android.Context, mode CodegenMode) CodegenContext {
	return CodegenContext{
		context: context,
		config:  config,
		mode:    mode,
	}
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

func GenerateBazelTargets(ctx CodegenContext) (map[string]BazelTargets, CodegenMetrics) {
	buildFileToTargets := make(map[string]BazelTargets)

	// Simple metrics tracking for bp2build
	totalModuleCount := 0
	ruleClassCount := make(map[string]int)

	bpCtx := ctx.Context()
	bpCtx.VisitAllModules(func(m blueprint.Module) {
		dir := bpCtx.ModuleDir(m)
		var t BazelTarget

		switch ctx.Mode() {
		case Bp2Build:
			if b, ok := m.(android.BazelTargetModule); !ok {
				// Only include regular Soong modules (non-BazelTargetModules) into the total count.
				totalModuleCount += 1
				return
			} else {
				t = generateBazelTarget(bpCtx, m, b)
				ruleClassCount[t.ruleClass] += 1
			}
		case QueryView:
			// Blocklist certain module types from being generated.
			if canonicalizeModuleType(bpCtx.ModuleType(m)) == "package" {
				// package module name contain slashes, and thus cannot
				// be mapped cleanly to a bazel label.
				return
			}
			t = generateSoongModuleTarget(bpCtx, m)
		default:
			panic(fmt.Errorf("Unknown code-generation mode: %s", ctx.Mode()))
		}

		buildFileToTargets[dir] = append(buildFileToTargets[dir], t)
	})

	metrics := CodegenMetrics{
		TotalModuleCount: totalModuleCount,
		RuleClassCount:   ruleClassCount,
	}

	return buildFileToTargets, metrics
}

func generateBazelTarget(ctx bpToBuildContext, m blueprint.Module, b android.BazelTargetModule) BazelTarget {
	ruleClass := b.RuleClass()
	bzlLoadLocation := b.BzlLoadLocation()

	// extract the bazel attributes from the module.
	props := getBuildProperties(ctx, m)

	delete(props.Attrs, "bp2build_available")

	// Return the Bazel target with rule class and attributes, ready to be
	// code-generated.
	attributes := propsToAttributes(props.Attrs)
	targetName := targetNameForBp2Build(ctx, m)
	return BazelTarget{
		name:            targetName,
		ruleClass:       ruleClass,
		bzlLoadLocation: bzlLoadLocation,
		content: fmt.Sprintf(
			bazelTarget,
			ruleClass,
			targetName,
			attributes,
		),
	}
}

// Convert a module and its deps and props into a Bazel macro/rule
// representation in the BUILD file.
func generateSoongModuleTarget(ctx bpToBuildContext, m blueprint.Module) BazelTarget {
	props := getBuildProperties(ctx, m)

	// TODO(b/163018919): DirectDeps can have duplicate (module, variant)
	// items, if the modules are added using different DependencyTag. Figure
	// out the implications of that.
	depLabels := map[string]bool{}
	if aModule, ok := m.(android.Module); ok {
		ctx.VisitDirectDeps(aModule, func(depModule blueprint.Module) {
			depLabels[qualifiedTargetLabel(ctx, depModule)] = true
		})
	}
	attributes := propsToAttributes(props.Attrs)

	depLabelList := "[\n"
	for depLabel, _ := range depLabels {
		depLabelList += fmt.Sprintf("        %q,\n", depLabel)
	}
	depLabelList += "    ]"

	targetName := targetNameWithVariant(ctx, m)
	return BazelTarget{
		name: targetName,
		content: fmt.Sprintf(
			soongModuleTarget,
			targetName,
			ctx.ModuleName(m),
			canonicalizeModuleType(ctx.ModuleType(m)),
			ctx.ModuleSubDir(m),
			depLabelList,
			attributes),
	}
}

func getBuildProperties(ctx bpToBuildContext, m blueprint.Module) BazelAttributes {
	var allProps map[string]string
	// TODO: this omits properties for blueprint modules (blueprint_go_binary,
	// bootstrap_go_binary, bootstrap_go_package), which will have to be handled separately.
	if aModule, ok := m.(android.Module); ok {
		allProps = ExtractModuleProperties(aModule)
	}

	return BazelAttributes{
		Attrs: allProps,
	}
}

// Generically extract module properties and types into a map, keyed by the module property name.
func ExtractModuleProperties(aModule android.Module) map[string]string {
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

func isStructPtr(t reflect.Type) bool {
	return t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Struct
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
		// Special cases where the bp2build sends additional information to the codegenerator
		// by wrapping the attributes in a custom struct type.
		if labels, ok := propertyValue.Interface().(bazel.LabelList); ok {
			// TODO(b/165114590): convert glob syntax
			return prettyPrint(reflect.ValueOf(labels.Includes), indent)
		} else if label, ok := propertyValue.Interface().(bazel.Label); ok {
			return fmt.Sprintf("%q", label.Label), nil
		} else if stringList, ok := propertyValue.Interface().(bazel.StringListAttribute); ok {
			// A Bazel string_list attribute that may contain a select statement.
			ret, err := prettyPrint(reflect.ValueOf(stringList.Value), indent)
			if err != nil {
				return ret, err
			}

			if !stringList.HasArchSpecificValues() {
				// Select statement not needed.
				return ret, nil
			}

			ret += " + " + "select({\n"
			for _, arch := range android.ArchTypeList() {
				value := stringList.GetValueForArch(arch.Name)
				if len(value) > 0 {
					ret += makeIndent(indent + 1)
					list, _ := prettyPrint(reflect.ValueOf(value), indent+1)
					ret += fmt.Sprintf("\"%s\": %s,\n", platformArchMap[arch], list)
				}
			}

			ret += makeIndent(indent + 1)
			list, _ := prettyPrint(reflect.ValueOf(stringList.GetValueForArch("default")), indent+1)
			ret += fmt.Sprintf("\"%s\": %s,\n", "//conditions:default", list)

			ret += makeIndent(indent)
			ret += "})"
			return ret, err
		}

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
		if shouldSkipStructField(field) {
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

func targetNameForBp2Build(c bpToBuildContext, logicModule blueprint.Module) string {
	return strings.Replace(c.ModuleName(logicModule), bazel.BazelTargetModuleNamePrefix, "", 1)
}

func targetNameWithVariant(c bpToBuildContext, logicModule blueprint.Module) string {
	name := ""
	if c.ModuleSubDir(logicModule) != "" {
		// TODO(b/162720883): Figure out a way to drop the "--" variant suffixes.
		name = c.ModuleName(logicModule) + "--" + c.ModuleSubDir(logicModule)
	} else {
		name = c.ModuleName(logicModule)
	}

	return strings.Replace(name, "//", "", 1)
}

func qualifiedTargetLabel(c bpToBuildContext, logicModule blueprint.Module) string {
	return fmt.Sprintf("//%s:%s", c.ModuleDir(logicModule), targetNameWithVariant(c, logicModule))
}
