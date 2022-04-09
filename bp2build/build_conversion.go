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

type BazelTarget struct {
	name            string
	packageName     string
	content         string
	ruleClass       string
	bzlLoadLocation string
	handcrafted     bool
}

// IsLoadedFromStarlark determines if the BazelTarget's rule class is loaded from a .bzl file,
// as opposed to a native rule built into Bazel.
func (t BazelTarget) IsLoadedFromStarlark() bool {
	return t.bzlLoadLocation != ""
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

// BazelTargets is a typedef for a slice of BazelTarget objects.
type BazelTargets []BazelTarget

// HasHandcraftedTargetsreturns true if a set of bazel targets contain
// handcrafted ones.
func (targets BazelTargets) hasHandcraftedTargets() bool {
	for _, target := range targets {
		if target.handcrafted {
			return true
		}
	}
	return false
}

// sort a list of BazelTargets in-place, by name, and by generated/handcrafted types.
func (targets BazelTargets) sort() {
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].handcrafted != targets[j].handcrafted {
			// Handcrafted targets will be generated after the bp2build generated targets.
			return targets[j].handcrafted
		}
		// This will cover all bp2build generated targets.
		return targets[i].name < targets[j].name
	})
}

// String returns the string representation of BazelTargets, without load
// statements (use LoadStatements for that), since the targets are usually not
// adjacent to the load statements at the top of the BUILD file.
func (targets BazelTargets) String() string {
	var res string
	for i, target := range targets {
		// There is only at most 1 handcrafted "target", because its contents
		// represent the entire BUILD file content from the tree. See
		// build_conversion.go#getHandcraftedBuildContent for more information.
		//
		// Add a header to make it easy to debug where the handcrafted targets
		// are in a generated BUILD file.
		if target.handcrafted {
			res += "# -----------------------------\n"
			res += "# Section: Handcrafted targets. \n"
			res += "# -----------------------------\n\n"
		}

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
	config             android.Config
	context            android.Context
	mode               CodegenMode
	additionalDeps     []string
	unconvertedDepMode unconvertedDepsMode
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
	var unconvertedDeps unconvertedDepsMode
	if config.IsEnvTrue("BP2BUILD_ERROR_UNCONVERTED") {
		unconvertedDeps = errorModulesUnconvertedDeps
	}
	return &CodegenContext{
		context:            context,
		config:             config,
		mode:               mode,
		unconvertedDepMode: unconvertedDeps,
	}
}

// props is an unsorted map. This function ensures that
// the generated attributes are sorted to ensure determinism.
func propsToAttributes(props map[string]string) string {
	var attributes string
	for _, propName := range android.SortedStringKeys(props) {
		attributes += fmt.Sprintf("    %s = %s,\n", propName, props[propName])
	}
	return attributes
}

type conversionResults struct {
	buildFileToTargets map[string]BazelTargets
	metrics            CodegenMetrics
}

func (r conversionResults) BuildDirToTargets() map[string]BazelTargets {
	return r.buildFileToTargets
}

func GenerateBazelTargets(ctx *CodegenContext, generateFilegroups bool) (conversionResults, []error) {
	buildFileToTargets := make(map[string]BazelTargets)
	buildFileToAppend := make(map[string]bool)

	// Simple metrics tracking for bp2build
	metrics := CodegenMetrics{
		ruleClassCount:           make(map[string]uint64),
		convertedModuleTypeCount: make(map[string]uint64),
		totalModuleTypeCount:     make(map[string]uint64),
	}

	dirs := make(map[string]bool)

	var errs []error

	bpCtx := ctx.Context()
	bpCtx.VisitAllModules(func(m blueprint.Module) {
		dir := bpCtx.ModuleDir(m)
		moduleType := bpCtx.ModuleType(m)
		dirs[dir] = true

		var targets []BazelTarget

		switch ctx.Mode() {
		case Bp2Build:
			// There are two main ways of converting a Soong module to Bazel:
			// 1) Manually handcrafting a Bazel target and associating the module with its label
			// 2) Automatically generating with bp2build converters
			//
			// bp2build converters are used for the majority of modules.
			if b, ok := m.(android.Bazelable); ok && b.HasHandcraftedLabel() {
				// Handle modules converted to handcrafted targets.
				//
				// Since these modules are associated with some handcrafted
				// target in a BUILD file, we simply append the entire contents
				// of that BUILD file to the generated BUILD file.
				//
				// The append operation is only done once, even if there are
				// multiple modules from the same directory associated to
				// targets in the same BUILD file (or package).

				// Log the module.
				metrics.AddConvertedModule(m, moduleType, Handcrafted)

				pathToBuildFile := getBazelPackagePath(b)
				if _, exists := buildFileToAppend[pathToBuildFile]; exists {
					// Append the BUILD file content once per package, at most.
					return
				}
				t, err := getHandcraftedBuildContent(ctx, b, pathToBuildFile)
				if err != nil {
					errs = append(errs, fmt.Errorf("Error converting %s: %s", bpCtx.ModuleName(m), err))
					return
				}
				targets = append(targets, t)
				// TODO(b/181575318): currently we append the whole BUILD file, let's change that to do
				// something more targeted based on the rule type and target
				buildFileToAppend[pathToBuildFile] = true
			} else if aModule, ok := m.(android.Module); ok && aModule.IsConvertedByBp2build() {
				// Handle modules converted to generated targets.

				// Log the module.
				metrics.AddConvertedModule(aModule, moduleType, Generated)

				// Handle modules with unconverted deps. By default, emit a warning.
				if unconvertedDeps := aModule.GetUnconvertedBp2buildDeps(); len(unconvertedDeps) > 0 {
					msg := fmt.Sprintf("%q depends on unconverted modules: %s", m.Name(), strings.Join(unconvertedDeps, ", "))
					if ctx.unconvertedDepMode == warnUnconvertedDeps {
						metrics.moduleWithUnconvertedDepsMsgs = append(metrics.moduleWithUnconvertedDepsMsgs, msg)
					} else if ctx.unconvertedDepMode == errorModulesUnconvertedDeps {
						errs = append(errs, fmt.Errorf(msg))
						return
					}
				}
				if unconvertedDeps := aModule.GetMissingBp2buildDeps(); len(unconvertedDeps) > 0 {
					msg := fmt.Sprintf("%q depends on missing modules: %s", m.Name(), strings.Join(unconvertedDeps, ", "))
					if ctx.unconvertedDepMode == warnUnconvertedDeps {
						metrics.moduleWithMissingDepsMsgs = append(metrics.moduleWithMissingDepsMsgs, msg)
					} else if ctx.unconvertedDepMode == errorModulesUnconvertedDeps {
						errs = append(errs, fmt.Errorf(msg))
						return
					}
				}
				targets = generateBazelTargets(bpCtx, aModule)
				for _, t := range targets {
					// A module can potentially generate more than 1 Bazel
					// target, each of a different rule class.
					metrics.IncrementRuleClassCount(t.ruleClass)
				}
			} else {
				metrics.AddUnconvertedModule(moduleType)
				return
			}
		case QueryView:
			// Blocklist certain module types from being generated.
			if canonicalizeModuleType(bpCtx.ModuleType(m)) == "package" {
				// package module name contain slashes, and thus cannot
				// be mapped cleanly to a bazel label.
				return
			}
			t := generateSoongModuleTarget(bpCtx, m)
			targets = append(targets, t)
		default:
			errs = append(errs, fmt.Errorf("Unknown code-generation mode: %s", ctx.Mode()))
			return
		}

		buildFileToTargets[dir] = append(buildFileToTargets[dir], targets...)
	})

	if len(errs) > 0 {
		return conversionResults{}, errs
	}

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

	return conversionResults{
		buildFileToTargets: buildFileToTargets,
		metrics:            metrics,
	}, errs
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
		content:     c,
		handcrafted: true,
	}, nil
}

func generateBazelTargets(ctx bpToBuildContext, m android.Module) []BazelTarget {
	var targets []BazelTarget
	for _, m := range m.Bp2buildTargets() {
		targets = append(targets, generateBazelTarget(ctx, m))
	}
	return targets
}

type bp2buildModule interface {
	TargetName() string
	TargetPackage() string
	BazelRuleClass() string
	BazelRuleLoadLocation() string
	BazelAttributes() []interface{}
}

func generateBazelTarget(ctx bpToBuildContext, m bp2buildModule) BazelTarget {
	ruleClass := m.BazelRuleClass()
	bzlLoadLocation := m.BazelRuleLoadLocation()

	// extract the bazel attributes from the module.
	attrs := m.BazelAttributes()
	props := extractModuleProperties(attrs, true)

	// name is handled in a special manner
	delete(props.Attrs, "name")

	// Return the Bazel target with rule class and attributes, ready to be
	// code-generated.
	attributes := propsToAttributes(props.Attrs)
	targetName := m.TargetName()
	return BazelTarget{
		name:            targetName,
		packageName:     m.TargetPackage(),
		ruleClass:       ruleClass,
		bzlLoadLocation: bzlLoadLocation,
		content: fmt.Sprintf(
			bazelTarget,
			ruleClass,
			targetName,
			attributes,
		),
		handcrafted: false,
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

	for p, _ := range ignoredPropNames {
		delete(props.Attrs, p)
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
	// TODO: this omits properties for blueprint modules (blueprint_go_binary,
	// bootstrap_go_binary, bootstrap_go_package), which will have to be handled separately.
	if aModule, ok := m.(android.Module); ok {
		return extractModuleProperties(aModule.GetProperties(), false)
	}

	return BazelAttributes{}
}

// Generically extract module properties and types into a map, keyed by the module property name.
func extractModuleProperties(props []interface{}, checkForDuplicateProperties bool) BazelAttributes {
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
			for k, v := range extractStructProperties(structValue, 0) {
				if existing, exists := ret[k]; checkForDuplicateProperties && exists {
					panic(fmt.Errorf(
						"%s (%v) is present in properties whereas it should be consolidated into a commonAttributes",
						k, existing))
				}
				ret[k] = v
			}
		} else {
			panic(fmt.Errorf(
				"properties must be a pointer to a struct, got %T",
				propertiesValue.Interface()))
		}
	}

	return BazelAttributes{
		Attrs: ret,
	}
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
		structProps := extractStructProperties(propertyValue, indent)
		if len(structProps) == 0 {
			return "", nil
		}
		return starlark_fmt.PrintDict(structProps, indent), nil
	case reflect.Interface:
		// TODO(b/164227191): implement pretty print for interfaces.
		// Interfaces are used for for arch, multilib and target properties.
		return "", nil
	default:
		return "", fmt.Errorf(
			"unexpected kind for property struct field: %s", propertyValue.Kind())
	}
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

		// if the struct is embedded (anonymous), flatten the properties into the containing struct
		if field.Anonymous {
			if field.Type.Kind() == reflect.Ptr {
				fieldValue = fieldValue.Elem()
			}
			if fieldValue.Type().Kind() == reflect.Struct {
				propsToMerge := extractStructProperties(fieldValue, indent)
				for prop, value := range propsToMerge {
					ret[prop] = value
				}
				continue
			}
		}

		propertyName := proptools.PropertyNameForField(field.Name)
		prettyPrintedValue, err := prettyPrint(fieldValue, indent+1, false)
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
