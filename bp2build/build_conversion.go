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

/*
For shareable/common functionality for conversion from soong-module to build files
for queryview/bp2build
*/

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"android/soong/android"
	"android/soong/bazel"
	"android/soong/starlark_fmt"
	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

type BazelAttributes struct {
	Attrs map[string]string
}

type BazelLoadSymbol struct {
	// The name of the symbol in the file being loaded
	symbol string
	// The name the symbol wil have in this file. Can be left blank to use the same name as symbol.
	alias string
}

type BazelLoad struct {
	file    string
	symbols []BazelLoadSymbol
}

type BazelTarget struct {
	name        string
	packageName string
	content     string
	ruleClass   string
	loads       []BazelLoad
}

// Label is the fully qualified Bazel label constructed from the BazelTarget's
// package name and target name.
func (t BazelTarget) Label() string {
	if t.packageName == "." {
		return "//:" + t.name
	} else {
		return "//" + t.packageName + ":" + t.name
	}
}

// PackageName returns the package of the Bazel target.
// Defaults to root of tree.
func (t BazelTarget) PackageName() string {
	if t.packageName == "" {
		return "."
	}
	return t.packageName
}

// BazelTargets is a typedef for a slice of BazelTarget objects.
type BazelTargets []BazelTarget

func (targets BazelTargets) packageRule() *BazelTarget {
	for _, target := range targets {
		if target.ruleClass == "package" {
			return &target
		}
	}
	return nil
}

// sort a list of BazelTargets in-place, by name, and by generated/handcrafted types.
func (targets BazelTargets) sort() {
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].name < targets[j].name
	})
}

// String returns the string representation of BazelTargets, without load
// statements (use LoadStatements for that), since the targets are usually not
// adjacent to the load statements at the top of the BUILD file.
func (targets BazelTargets) String() string {
	var res strings.Builder
	for i, target := range targets {
		if target.ruleClass != "package" {
			res.WriteString(target.content)
		}
		if i != len(targets)-1 {
			res.WriteString("\n\n")
		}
	}
	return res.String()
}

// LoadStatements return the string representation of the sorted and deduplicated
// Starlark rule load statements needed by a group of BazelTargets.
func (targets BazelTargets) LoadStatements() string {
	// First, merge all the load statements from all the targets onto one list
	bzlToLoadedSymbols := map[string][]BazelLoadSymbol{}
	for _, target := range targets {
		for _, load := range target.loads {
		outer:
			for _, symbol := range load.symbols {
				alias := symbol.alias
				if alias == "" {
					alias = symbol.symbol
				}
				for _, otherSymbol := range bzlToLoadedSymbols[load.file] {
					otherAlias := otherSymbol.alias
					if otherAlias == "" {
						otherAlias = otherSymbol.symbol
					}
					if symbol.symbol == otherSymbol.symbol && alias == otherAlias {
						continue outer
					} else if alias == otherAlias {
						panic(fmt.Sprintf("Conflicting destination (%s) for loads of %s and %s", alias, symbol.symbol, otherSymbol.symbol))
					}
				}
				bzlToLoadedSymbols[load.file] = append(bzlToLoadedSymbols[load.file], symbol)
			}
		}
	}

	var loadStatements strings.Builder
	for i, bzl := range android.SortedKeys(bzlToLoadedSymbols) {
		symbols := bzlToLoadedSymbols[bzl]
		loadStatements.WriteString("load(\"")
		loadStatements.WriteString(bzl)
		loadStatements.WriteString("\", ")
		sort.Slice(symbols, func(i, j int) bool {
			if symbols[i].symbol < symbols[j].symbol {
				return true
			}
			return symbols[i].alias < symbols[j].alias
		})
		for j, symbol := range symbols {
			if symbol.alias != "" && symbol.alias != symbol.symbol {
				loadStatements.WriteString(symbol.alias)
				loadStatements.WriteString(" = ")
			}
			loadStatements.WriteString("\"")
			loadStatements.WriteString(symbol.symbol)
			loadStatements.WriteString("\"")
			if j != len(symbols)-1 {
				loadStatements.WriteString(", ")
			}
		}
		loadStatements.WriteString(")")
		if i != len(bzlToLoadedSymbols)-1 {
			loadStatements.WriteString("\n")
		}
	}
	return loadStatements.String()
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
	config             android.Config
	context            *android.Context
	mode               CodegenMode
	additionalDeps     []string
	unconvertedDepMode unconvertedDepsMode
	topDir             string
}

func (ctx *CodegenContext) Mode() CodegenMode {
	return ctx.mode
}

// CodegenMode is an enum to differentiate code-generation modes.
type CodegenMode int

const (
	// QueryView - generate BUILD files with targets representing fully mutated
	// Soong modules, representing the fully configured Soong module graph with
	// variants and dependency edges.
	//
	// This mode is used for discovering and introspecting the existing Soong
	// module graph.
	QueryView CodegenMode = iota
)

type unconvertedDepsMode int

const (
	// Include a warning in conversion metrics about converted modules with unconverted direct deps
	warnUnconvertedDeps unconvertedDepsMode = iota
	// Error and fail conversion if encountering a module with unconverted direct deps
	// Enabled by setting environment variable `BP2BUILD_ERROR_UNCONVERTED`
	errorModulesUnconvertedDeps
)

func (mode CodegenMode) String() string {
	switch mode {
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

func (ctx *CodegenContext) Config() android.Config    { return ctx.config }
func (ctx *CodegenContext) Context() *android.Context { return ctx.context }

// NewCodegenContext creates a wrapper context that conforms to PathContext for
// writing BUILD files in the output directory.
func NewCodegenContext(config android.Config, context *android.Context, mode CodegenMode, topDir string) *CodegenContext {
	var unconvertedDeps unconvertedDepsMode
	return &CodegenContext{
		context:            context,
		config:             config,
		mode:               mode,
		unconvertedDepMode: unconvertedDeps,
		topDir:             topDir,
	}
}

// props is an unsorted map. This function ensures that
// the generated attributes are sorted to ensure determinism.
func propsToAttributes(props map[string]string) string {
	var attributes string
	for _, propName := range android.SortedKeys(props) {
		attributes += fmt.Sprintf("    %s = %s,\n", propName, props[propName])
	}
	return attributes
}

type conversionResults struct {
	buildFileToTargets    map[string]BazelTargets
	moduleNameToPartition map[string]string
}

func (r conversionResults) BuildDirToTargets() map[string]BazelTargets {
	return r.buildFileToTargets
}

func GenerateBazelTargets(ctx *CodegenContext, generateFilegroups bool) (conversionResults, []error) {
	ctx.Context().BeginEvent("GenerateBazelTargets")
	defer ctx.Context().EndEvent("GenerateBazelTargets")
	buildFileToTargets := make(map[string]BazelTargets)

	dirs := make(map[string]bool)
	moduleNameToPartition := make(map[string]string)

	var errs []error

	bpCtx := ctx.Context()
	bpCtx.VisitAllModules(func(m blueprint.Module) {
		dir := bpCtx.ModuleDir(m)
		dirs[dir] = true

		var targets []BazelTarget

		switch ctx.Mode() {
		case QueryView:
			// Blocklist certain module types from being generated.
			if canonicalizeModuleType(bpCtx.ModuleType(m)) == "package" {
				// package module name contain slashes, and thus cannot
				// be mapped cleanly to a bazel label.
				return
			}
			t, err := generateSoongModuleTarget(bpCtx, m)
			if err != nil {
				errs = append(errs, err)
			}
			targets = append(targets, t)
		default:
			errs = append(errs, fmt.Errorf("Unknown code-generation mode: %s", ctx.Mode()))
			return
		}

		for _, target := range targets {
			targetDir := target.PackageName()
			buildFileToTargets[targetDir] = append(buildFileToTargets[targetDir], target)
		}
	})

	if len(errs) > 0 {
		return conversionResults{}, errs
	}

	if generateFilegroups {
		// Add a filegroup target that exposes all sources in the subtree of this package
		// NOTE: This also means we generate a BUILD file for every Android.bp file (as long as it has at least one module)
		//
		// This works because: https://bazel.build/reference/be/functions#exports_files
		// "As a legacy behaviour, also files mentioned as input to a rule are exported with the
		// default visibility until the flag --incompatible_no_implicit_file_export is flipped. However, this behavior
		// should not be relied upon and actively migrated away from."
		//
		// TODO(b/198619163): We should change this to export_files(glob(["**/*"])) instead, but doing that causes these errors:
		// "Error in exports_files: generated label '//external/avb:avbtool' conflicts with existing py_binary rule"
		// So we need to solve all the "target ... is both a rule and a file" warnings first.
		for dir := range dirs {
			buildFileToTargets[dir] = append(buildFileToTargets[dir], BazelTarget{
				name:      "bp2build_all_srcs",
				content:   `filegroup(name = "bp2build_all_srcs", srcs = glob(["**/*"]), tags = ["manual"])`,
				ruleClass: "filegroup",
			})
		}
	}

	return conversionResults{
		buildFileToTargets:    buildFileToTargets,
		moduleNameToPartition: moduleNameToPartition,
	}, errs
}

// Convert a module and its deps and props into a Bazel macro/rule
// representation in the BUILD file.
func generateSoongModuleTarget(ctx bpToBuildContext, m blueprint.Module) (BazelTarget, error) {
	props, err := getBuildProperties(ctx, m)

	// TODO(b/163018919): DirectDeps can have duplicate (module, variant)
	// items, if the modules are added using different DependencyTag. Figure
	// out the implications of that.
	depLabels := map[string]bool{}
	if aModule, ok := m.(android.Module); ok {
		ctx.VisitDirectDeps(aModule, func(depModule blueprint.Module) {
			depLabels[qualifiedTargetLabel(ctx, depModule)] = true
		})
	}

	for p := range ignoredPropNames {
		delete(props.Attrs, p)
	}
	attributes := propsToAttributes(props.Attrs)

	depLabelList := "[\n"
	for depLabel := range depLabels {
		depLabelList += fmt.Sprintf("        %q,\n", depLabel)
	}
	depLabelList += "    ]"

	targetName := targetNameWithVariant(ctx, m)
	return BazelTarget{
		name:        targetName,
		packageName: ctx.ModuleDir(m),
		content: fmt.Sprintf(
			soongModuleTargetTemplate,
			targetName,
			ctx.ModuleName(m),
			canonicalizeModuleType(ctx.ModuleType(m)),
			ctx.ModuleSubDir(m),
			depLabelList,
			attributes),
	}, err
}

func getBuildProperties(ctx bpToBuildContext, m blueprint.Module) (BazelAttributes, error) {
	// TODO: this omits properties for blueprint modules (blueprint_go_binary,
	// bootstrap_go_binary, bootstrap_go_package), which will have to be handled separately.
	if aModule, ok := m.(android.Module); ok {
		return extractModuleProperties(aModule.GetProperties(), false)
	}

	return BazelAttributes{}, nil
}

// Generically extract module properties and types into a map, keyed by the module property name.
func extractModuleProperties(props []interface{}, checkForDuplicateProperties bool) (BazelAttributes, error) {
	ret := map[string]string{}

	// Iterate over this android.Module's property structs.
	for _, properties := range props {
		propertiesValue := reflect.ValueOf(properties)
		// Check that propertiesValue is a pointer to the Properties struct, like
		// *cc.BaseLinkerProperties or *java.CompilerProperties.
		//
		// propertiesValue can also be type-asserted to the structs to
		// manipulate internal props, if needed.
		if isStructPtr(propertiesValue.Type()) {
			structValue := propertiesValue.Elem()
			ok, err := extractStructProperties(structValue, 0)
			if err != nil {
				return BazelAttributes{}, err
			}
			for k, v := range ok {
				if existing, exists := ret[k]; checkForDuplicateProperties && exists {
					return BazelAttributes{}, fmt.Errorf(
						"%s (%v) is present in properties whereas it should be consolidated into a commonAttributes",
						k, existing)
				}
				ret[k] = v
			}
		} else {
			return BazelAttributes{},
				fmt.Errorf(
					"properties must be a pointer to a struct, got %T",
					propertiesValue.Interface())
		}
	}

	return BazelAttributes{
		Attrs: ret,
	}, nil
}

func isStructPtr(t reflect.Type) bool {
	return t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Struct
}

// prettyPrint a property value into the equivalent Starlark representation
// recursively.
func prettyPrint(propertyValue reflect.Value, indent int, emitZeroValues bool) (string, error) {
	if !emitZeroValues && isZero(propertyValue) {
		// A property value being set or unset actually matters -- Soong does set default
		// values for unset properties, like system_shared_libs = ["libc", "libm", "libdl"] at
		// https://cs.android.com/android/platform/superproject/+/main:build/soong/cc/linker.go;l=281-287;drc=f70926eef0b9b57faf04c17a1062ce50d209e480
		//
		// In Bazel-parlance, we would use "attr.<type>(default = <default
		// value>)" to set the default value of unset attributes. In the cases
		// where the bp2build converter didn't set the default value within the
		// mutator when creating the BazelTargetModule, this would be a zero
		// value. For those cases, we return an empty string so we don't
		// unnecessarily generate empty values.
		return "", nil
	}

	switch propertyValue.Kind() {
	case reflect.String:
		return fmt.Sprintf("\"%v\"", escapeString(propertyValue.String())), nil
	case reflect.Bool:
		return starlark_fmt.PrintBool(propertyValue.Bool()), nil
	case reflect.Int, reflect.Uint, reflect.Int64:
		return fmt.Sprintf("%v", propertyValue.Interface()), nil
	case reflect.Ptr:
		return prettyPrint(propertyValue.Elem(), indent, emitZeroValues)
	case reflect.Slice:
		elements := make([]string, 0, propertyValue.Len())
		for i := 0; i < propertyValue.Len(); i++ {
			val, err := prettyPrint(propertyValue.Index(i), indent, emitZeroValues)
			if err != nil {
				return "", err
			}
			if val != "" {
				elements = append(elements, val)
			}
		}
		return starlark_fmt.PrintList(elements, indent, func(s string) string {
			return "%s"
		}), nil

	case reflect.Struct:
		// Special cases where the bp2build sends additional information to the codegenerator
		// by wrapping the attributes in a custom struct type.
		if attr, ok := propertyValue.Interface().(bazel.Attribute); ok {
			return prettyPrintAttribute(attr, indent)
		} else if label, ok := propertyValue.Interface().(bazel.Label); ok {
			return fmt.Sprintf("%q", label.Label), nil
		}

		// Sort and print the struct props by the key.
		structProps, err := extractStructProperties(propertyValue, indent)

		if err != nil {
			return "", err
		}

		if len(structProps) == 0 {
			return "", nil
		}
		return starlark_fmt.PrintDict(structProps, indent), nil
	case reflect.Interface:
		// TODO(b/164227191): implement pretty print for interfaces.
		// Interfaces are used for for arch, multilib and target properties.
		return "", nil
	case reflect.Map:
		if v, ok := propertyValue.Interface().(bazel.StringMapAttribute); ok {
			return starlark_fmt.PrintStringStringDict(v, indent), nil
		}
		return "", fmt.Errorf("bp2build expects map of type map[string]string for field: %s", propertyValue)
	default:
		return "", fmt.Errorf(
			"unexpected kind for property struct field: %s", propertyValue.Kind())
	}
}

// Converts a reflected property struct value into a map of property names and property values,
// which each property value correctly pretty-printed and indented at the right nest level,
// since property structs can be nested. In Starlark, nested structs are represented as nested
// dicts: https://docs.bazel.build/skylark/lib/dict.html
func extractStructProperties(structValue reflect.Value, indent int) (map[string]string, error) {
	if structValue.Kind() != reflect.Struct {
		return map[string]string{}, fmt.Errorf("Expected a reflect.Struct type, but got %s", structValue.Kind())
	}

	var err error

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

		// if the struct is embedded (anonymous), flatten the properties into the containing struct
		if field.Anonymous {
			if field.Type.Kind() == reflect.Ptr {
				fieldValue = fieldValue.Elem()
			}
			if fieldValue.Type().Kind() == reflect.Struct {
				propsToMerge, err := extractStructProperties(fieldValue, indent)
				if err != nil {
					return map[string]string{}, err
				}
				for prop, value := range propsToMerge {
					ret[prop] = value
				}
				continue
			}
		}

		propertyName := proptools.PropertyNameForField(field.Name)
		var prettyPrintedValue string
		prettyPrintedValue, err = prettyPrint(fieldValue, indent+1, false)
		if err != nil {
			return map[string]string{}, fmt.Errorf(
				"Error while parsing property: %q. %s",
				propertyName,
				err)
		}
		if prettyPrintedValue != "" {
			ret[propertyName] = prettyPrintedValue
		}
	}

	return ret, nil
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
	// Always print bool/strings, if you want a bool/string attribute to be able to take the default value, use a
	// pointer instead
	case reflect.Bool, reflect.String:
		return false
	default:
		if !value.IsValid() {
			return true
		}
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
