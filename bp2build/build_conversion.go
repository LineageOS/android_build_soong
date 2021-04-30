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
	config         android.Config
	context        android.Context
	mode           CodegenMode
	additionalDeps []string
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

// AddNinjaFileDeps adds dependencies on the specified files to be added to the ninja manifest. The
// primary builder will be rerun whenever the specified files are modified. Allows us to fulfill the
// PathContext interface in order to add dependencies on hand-crafted BUILD files. Note: must also
// call AdditionalNinjaDeps and add them manually to the ninja file.
func (ctx *CodegenContext) AddNinjaFileDeps(deps ...string) {
	ctx.additionalDeps = append(ctx.additionalDeps, deps...)
}

// AdditionalNinjaDeps returns additional ninja deps added by CodegenContext
func (ctx *CodegenContext) AdditionalNinjaDeps() []string {
	return ctx.additionalDeps
}

func (ctx *CodegenContext) Config() android.Config   { return ctx.config }
func (ctx *CodegenContext) Context() android.Context { return ctx.context }

// NewCodegenContext creates a wrapper context that conforms to PathContext for
// writing BUILD files in the output directory.
func NewCodegenContext(config android.Config, context android.Context, mode CodegenMode) *CodegenContext {
	return &CodegenContext{
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

func GenerateBazelTargets(ctx *CodegenContext, generateFilegroups bool) (map[string]BazelTargets, CodegenMetrics) {
	buildFileToTargets := make(map[string]BazelTargets)
	buildFileToAppend := make(map[string]bool)

	// Simple metrics tracking for bp2build
	metrics := CodegenMetrics{
		RuleClassCount: make(map[string]int),
	}

	dirs := make(map[string]bool)

	bpCtx := ctx.Context()
	bpCtx.VisitAllModules(func(m blueprint.Module) {
		dir := bpCtx.ModuleDir(m)
		dirs[dir] = true

		var t BazelTarget

		switch ctx.Mode() {
		case Bp2Build:
			if b, ok := m.(android.Bazelable); ok && b.HasHandcraftedLabel() {
				metrics.handCraftedTargetCount += 1
				metrics.TotalModuleCount += 1
				pathToBuildFile := getBazelPackagePath(b)
				// We are using the entire contents of handcrafted build file, so if multiple targets within
				// a package have handcrafted targets, we only want to include the contents one time.
				if _, exists := buildFileToAppend[pathToBuildFile]; exists {
					return
				}
				var err error
				t, err = getHandcraftedBuildContent(ctx, b, pathToBuildFile)
				if err != nil {
					panic(fmt.Errorf("Error converting %s: %s", bpCtx.ModuleName(m), err))
				}
				// TODO(b/181575318): currently we append the whole BUILD file, let's change that to do
				// something more targeted based on the rule type and target
				buildFileToAppend[pathToBuildFile] = true
			} else if btm, ok := m.(android.BazelTargetModule); ok {
				t = generateBazelTarget(bpCtx, m, btm)
				metrics.RuleClassCount[t.ruleClass] += 1
			} else {
				metrics.TotalModuleCount += 1
				return
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
	if generateFilegroups {
		// Add a filegroup target that exposes all sources in the subtree of this package
		// NOTE: This also means we generate a BUILD file for every Android.bp file (as long as it has at least one module)
		for dir, _ := range dirs {
			buildFileToTargets[dir] = append(buildFileToTargets[dir], BazelTarget{
				name:      "bp2build_all_srcs",
				content:   `filegroup(name = "bp2build_all_srcs", srcs = glob(["**/*"]))`,
				ruleClass: "filegroup",
			})
		}
	}

	return buildFileToTargets, metrics
}

func getBazelPackagePath(b android.Bazelable) string {
	label := b.HandcraftedLabel()
	pathToBuildFile := strings.TrimPrefix(label, "//")
	pathToBuildFile = strings.Split(pathToBuildFile, ":")[0]
	return pathToBuildFile
}

func getHandcraftedBuildContent(ctx *CodegenContext, b android.Bazelable, pathToBuildFile string) (BazelTarget, error) {
	p := android.ExistentPathForSource(ctx, pathToBuildFile, HandcraftedBuildFileName)
	if !p.Valid() {
		return BazelTarget{}, fmt.Errorf("Could not find file %q for handcrafted target.", pathToBuildFile)
	}
	c, err := b.GetBazelBuildFileContents(ctx.Config(), pathToBuildFile, HandcraftedBuildFileName)
	if err != nil {
		return BazelTarget{}, err
	}
	// TODO(b/181575318): once this is more targeted, we need to include name, rule class, etc
	return BazelTarget{
		content: c,
	}, nil
}

func generateBazelTarget(ctx bpToBuildContext, m blueprint.Module, btm android.BazelTargetModule) BazelTarget {
	ruleClass := btm.RuleClass()
	bzlLoadLocation := btm.BzlLoadLocation()

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
		// In Bazel-parlance, we would use "attr.<type>(default = <default
		// value>)" to set the default value of unset attributes. In the cases
		// where the bp2build converter didn't set the default value within the
		// mutator when creating the BazelTargetModule, this would be a zero
		// value. For those cases, we return an empty string so we don't
		// unnecessarily generate empty values.
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
		if propertyValue.Len() == 0 {
			return "", nil
		}

		if propertyValue.Len() == 1 {
			// Single-line list for list with only 1 element
			ret += "["
			indexedValue, err := prettyPrint(propertyValue.Index(0), indent)
			if err != nil {
				return "", err
			}
			ret += indexedValue
			ret += "]"
		} else {
			// otherwise, use a multiline list.
			ret += "[\n"
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
		}

	case reflect.Struct:
		// Special cases where the bp2build sends additional information to the codegenerator
		// by wrapping the attributes in a custom struct type.
		if attr, ok := propertyValue.Interface().(bazel.Attribute); ok {
			return prettyPrintAttribute(attr, indent)
		} else if label, ok := propertyValue.Interface().(bazel.Label); ok {
			return fmt.Sprintf("%q", label.Label), nil
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
			valueIsZero = valueIsZero && isZero(value.Field(i))
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

	// b/184026959: Reverse the application of some common control sequences.
	// These must be generated literally in the BUILD file.
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")

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
