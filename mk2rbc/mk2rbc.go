// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Convert makefile containing device configuration to Starlark file
// The conversion can handle the following constructs in a makefile:
//   - comments
//   - simple variable assignments
//   - $(call init-product,<file>)
//   - $(call inherit-product-if-exists
//   - if directives
//
// All other constructs are carried over to the output starlark file as comments.
package mk2rbc

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/scanner"

	mkparser "android/soong/androidmk/parser"
)

const (
	annotationCommentPrefix = "RBC#"
	baseUri                 = "//build/make/core:product_config.rbc"
	// The name of the struct exported by the product_config.rbc
	// that contains the functions and variables available to
	// product configuration Starlark files.
	baseName = "rblf"

	soongNsPrefix = "SOONG_CONFIG_"

	// And here are the functions and variables:
	cfnGetCfg         = baseName + ".cfg"
	cfnMain           = baseName + ".product_configuration"
	cfnBoardMain      = baseName + ".board_configuration"
	cfnPrintVars      = baseName + ".printvars"
	cfnInherit        = baseName + ".inherit"
	cfnSetListDefault = baseName + ".setdefault"
)

const (
	soongConfigAppend = "soong_config_append"
	soongConfigAssign = "soong_config_set"
)

var knownFunctions = map[string]interface {
	parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) starlarkExpr
}{
	"abspath":                              &simpleCallParser{name: baseName + ".abspath", returnType: starlarkTypeString},
	"add-product-dex-preopt-module-config": &simpleCallParser{name: baseName + ".add_product_dex_preopt_module_config", returnType: starlarkTypeString, addHandle: true},
	"add_soong_config_namespace":           &simpleCallParser{name: baseName + ".soong_config_namespace", returnType: starlarkTypeVoid, addGlobals: true},
	"add_soong_config_var_value":           &simpleCallParser{name: baseName + ".soong_config_set", returnType: starlarkTypeVoid, addGlobals: true},
	soongConfigAssign:                      &simpleCallParser{name: baseName + ".soong_config_set", returnType: starlarkTypeVoid, addGlobals: true},
	soongConfigAppend:                      &simpleCallParser{name: baseName + ".soong_config_append", returnType: starlarkTypeVoid, addGlobals: true},
	"soong_config_get":                     &simpleCallParser{name: baseName + ".soong_config_get", returnType: starlarkTypeString, addGlobals: true},
	"add-to-product-copy-files-if-exists":  &simpleCallParser{name: baseName + ".copy_if_exists", returnType: starlarkTypeList},
	"addprefix":                            &simpleCallParser{name: baseName + ".addprefix", returnType: starlarkTypeList},
	"addsuffix":                            &simpleCallParser{name: baseName + ".addsuffix", returnType: starlarkTypeList},
	"and":                                  &andOrParser{isAnd: true},
	"clear-var-list":                       &simpleCallParser{name: baseName + ".clear_var_list", returnType: starlarkTypeVoid, addGlobals: true, addHandle: true},
	"copy-files":                           &simpleCallParser{name: baseName + ".copy_files", returnType: starlarkTypeList},
	"dir":                                  &simpleCallParser{name: baseName + ".dir", returnType: starlarkTypeString},
	"dist-for-goals":                       &simpleCallParser{name: baseName + ".mkdist_for_goals", returnType: starlarkTypeVoid, addGlobals: true},
	"enforce-product-packages-exist":       &simpleCallParser{name: baseName + ".enforce_product_packages_exist", returnType: starlarkTypeVoid, addHandle: true},
	"error":                                &makeControlFuncParser{name: baseName + ".mkerror"},
	"findstring":                           &simpleCallParser{name: baseName + ".findstring", returnType: starlarkTypeInt},
	"find-copy-subdir-files":               &simpleCallParser{name: baseName + ".find_and_copy", returnType: starlarkTypeList},
	"filter":                               &simpleCallParser{name: baseName + ".filter", returnType: starlarkTypeList},
	"filter-out":                           &simpleCallParser{name: baseName + ".filter_out", returnType: starlarkTypeList},
	"firstword":                            &simpleCallParser{name: baseName + ".first_word", returnType: starlarkTypeString},
	"foreach":                              &foreachCallParser{},
	"if":                                   &ifCallParser{},
	"info":                                 &makeControlFuncParser{name: baseName + ".mkinfo"},
	"is-board-platform":                    &simpleCallParser{name: baseName + ".board_platform_is", returnType: starlarkTypeBool, addGlobals: true},
	"is-board-platform2":                   &simpleCallParser{name: baseName + ".board_platform_is", returnType: starlarkTypeBool, addGlobals: true},
	"is-board-platform-in-list":            &simpleCallParser{name: baseName + ".board_platform_in", returnType: starlarkTypeBool, addGlobals: true},
	"is-board-platform-in-list2":           &simpleCallParser{name: baseName + ".board_platform_in", returnType: starlarkTypeBool, addGlobals: true},
	"is-product-in-list":                   &isProductInListCallParser{},
	"is-vendor-board-platform":             &isVendorBoardPlatformCallParser{},
	"is-vendor-board-qcom":                 &isVendorBoardQcomCallParser{},
	"lastword":                             &simpleCallParser{name: baseName + ".last_word", returnType: starlarkTypeString},
	"notdir":                               &simpleCallParser{name: baseName + ".notdir", returnType: starlarkTypeString},
	"math_max":                             &mathMaxOrMinCallParser{function: "max"},
	"math_min":                             &mathMaxOrMinCallParser{function: "min"},
	"math_gt_or_eq":                        &mathComparisonCallParser{op: ">="},
	"math_gt":                              &mathComparisonCallParser{op: ">"},
	"math_lt":                              &mathComparisonCallParser{op: "<"},
	"my-dir":                               &myDirCallParser{},
	"or":                                   &andOrParser{isAnd: false},
	"patsubst":                             &substCallParser{fname: "patsubst"},
	"product-copy-files-by-pattern":        &simpleCallParser{name: baseName + ".product_copy_files_by_pattern", returnType: starlarkTypeList},
	"require-artifacts-in-path":            &simpleCallParser{name: baseName + ".require_artifacts_in_path", returnType: starlarkTypeVoid, addHandle: true},
	"require-artifacts-in-path-relaxed":    &simpleCallParser{name: baseName + ".require_artifacts_in_path_relaxed", returnType: starlarkTypeVoid, addHandle: true},
	// TODO(asmundak): remove it once all calls are removed from configuration makefiles. see b/183161002
	"shell":    &shellCallParser{},
	"sort":     &simpleCallParser{name: baseName + ".mksort", returnType: starlarkTypeList},
	"strip":    &simpleCallParser{name: baseName + ".mkstrip", returnType: starlarkTypeString},
	"subst":    &substCallParser{fname: "subst"},
	"to-lower": &lowerUpperParser{isUpper: false},
	"to-upper": &lowerUpperParser{isUpper: true},
	"warning":  &makeControlFuncParser{name: baseName + ".mkwarning"},
	"word":     &wordCallParser{},
	"words":    &wordsCallParser{},
	"wildcard": &simpleCallParser{name: baseName + ".expand_wildcard", returnType: starlarkTypeList},
}

// The same as knownFunctions, but returns a []starlarkNode instead of a starlarkExpr
var knownNodeFunctions = map[string]interface {
	parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) []starlarkNode
}{
	"eval":                      &evalNodeParser{},
	"if":                        &ifCallNodeParser{},
	"inherit-product":           &inheritProductCallParser{loadAlways: true},
	"inherit-product-if-exists": &inheritProductCallParser{loadAlways: false},
	"foreach":                   &foreachCallNodeParser{},
}

// These look like variables, but are actually functions, and would give
// undefined variable errors if we converted them as variables. Instead,
// emit an error instead of converting them.
var unsupportedFunctions = map[string]bool{
	"local-generated-sources-dir": true,
	"local-intermediates-dir":     true,
}

// These are functions that we don't implement conversions for, but
// we allow seeing their definitions in the product config files.
var ignoredDefines = map[string]bool{
	"find-word-in-list":                   true, // internal macro
	"get-vendor-board-platforms":          true, // internal macro, used by is-board-platform, etc.
	"is-android-codename":                 true, // unused by product config
	"is-android-codename-in-list":         true, // unused by product config
	"is-chipset-in-board-platform":        true, // unused by product config
	"is-chipset-prefix-in-board-platform": true, // unused by product config
	"is-not-board-platform":               true, // defined but never used
	"is-platform-sdk-version-at-least":    true, // unused by product config
	"match-prefix":                        true, // internal macro
	"match-word":                          true, // internal macro
	"match-word-in-list":                  true, // internal macro
	"tb-modules":                          true, // defined in hardware/amlogic/tb_modules/tb_detect.mk, unused
}

var identifierFullMatchRegex = regexp.MustCompile("^[a-zA-Z_][a-zA-Z0-9_]*$")

func RelativeToCwd(path string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	path, err = filepath.Rel(cwd, path)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(path, "../") {
		return "", fmt.Errorf("Could not make path relative to current working directory: " + path)
	}
	return path, nil
}

// Conversion request parameters
type Request struct {
	MkFile          string    // file to convert
	Reader          io.Reader // if set, read input from this stream instead
	OutputSuffix    string    // generated Starlark files suffix
	OutputDir       string    // if set, root of the output hierarchy
	ErrorLogger     ErrorLogger
	TracedVariables []string // trace assignment to these variables
	TraceCalls      bool
	SourceFS        fs.FS
	MakefileFinder  MakefileFinder
}

// ErrorLogger prints errors and gathers error statistics.
// Its NewError function is called on every error encountered during the conversion.
type ErrorLogger interface {
	NewError(el ErrorLocation, node mkparser.Node, text string, args ...interface{})
}

type ErrorLocation struct {
	MkFile string
	MkLine int
}

func (el ErrorLocation) String() string {
	return fmt.Sprintf("%s:%d", el.MkFile, el.MkLine)
}

// Derives module name for a given file. It is base name
// (file name without suffix), with some characters replaced to make it a Starlark identifier
func moduleNameForFile(mkFile string) string {
	base := strings.TrimSuffix(filepath.Base(mkFile), filepath.Ext(mkFile))
	// TODO(asmundak): what else can be in the product file names?
	return strings.NewReplacer("-", "_", ".", "_").Replace(base)

}

func cloneMakeString(mkString *mkparser.MakeString) *mkparser.MakeString {
	r := &mkparser.MakeString{StringPos: mkString.StringPos}
	r.Strings = append(r.Strings, mkString.Strings...)
	r.Variables = append(r.Variables, mkString.Variables...)
	return r
}

func isMakeControlFunc(s string) bool {
	return s == "error" || s == "warning" || s == "info"
}

// varAssignmentScope points to the last assignment for each variable
// in the current block. It is used during the parsing to chain
// the assignments to a variable together.
type varAssignmentScope struct {
	outer *varAssignmentScope
	vars  map[string]bool
}

// Starlark output generation context
type generationContext struct {
	buf            strings.Builder
	starScript     *StarlarkScript
	indentLevel    int
	inAssignment   bool
	tracedCount    int
	varAssignments *varAssignmentScope
}

func NewGenerateContext(ss *StarlarkScript) *generationContext {
	return &generationContext{
		starScript: ss,
		varAssignments: &varAssignmentScope{
			outer: nil,
			vars:  make(map[string]bool),
		},
	}
}

func (gctx *generationContext) pushVariableAssignments() {
	va := &varAssignmentScope{
		outer: gctx.varAssignments,
		vars:  make(map[string]bool),
	}
	gctx.varAssignments = va
}

func (gctx *generationContext) popVariableAssignments() {
	gctx.varAssignments = gctx.varAssignments.outer
}

func (gctx *generationContext) hasBeenAssigned(v variable) bool {
	for va := gctx.varAssignments; va != nil; va = va.outer {
		if _, ok := va.vars[v.name()]; ok {
			return true
		}
	}
	return false
}

func (gctx *generationContext) setHasBeenAssigned(v variable) {
	gctx.varAssignments.vars[v.name()] = true
}

// emit returns generated script
func (gctx *generationContext) emit() string {
	ss := gctx.starScript

	// The emitted code has the following layout:
	//    <initial comments>
	//    preamble, i.e.,
	//      load statement for the runtime support
	//      load statement for each unique submodule pulled in by this one
	//    def init(g, handle):
	//      cfg = rblf.cfg(handle)
	//      <statements>
	//      <warning if conversion was not clean>

	iNode := len(ss.nodes)
	for i, node := range ss.nodes {
		if _, ok := node.(*commentNode); !ok {
			iNode = i
			break
		}
		node.emit(gctx)
	}

	gctx.emitPreamble()

	gctx.newLine()
	// The arguments passed to the init function are the global dictionary
	// ('g') and the product configuration dictionary ('cfg')
	gctx.write("def init(g, handle):")
	gctx.indentLevel++
	if gctx.starScript.traceCalls {
		gctx.newLine()
		gctx.writef(`print(">%s")`, gctx.starScript.mkFile)
	}
	gctx.newLine()
	gctx.writef("cfg = %s(handle)", cfnGetCfg)
	for _, node := range ss.nodes[iNode:] {
		node.emit(gctx)
	}

	if gctx.starScript.traceCalls {
		gctx.newLine()
		gctx.writef(`print("<%s")`, gctx.starScript.mkFile)
	}
	gctx.indentLevel--
	gctx.write("\n")
	return gctx.buf.String()
}

func (gctx *generationContext) emitPreamble() {
	gctx.newLine()
	gctx.writef("load(%q, %q)", baseUri, baseName)
	// Emit exactly one load statement for each URI.
	loadedSubConfigs := make(map[string]string)
	for _, mi := range gctx.starScript.inherited {
		uri := mi.path
		if strings.HasPrefix(uri, "/") && !strings.HasPrefix(uri, "//") {
			var err error
			uri, err = RelativeToCwd(uri)
			if err != nil {
				panic(err)
			}
			uri = "//" + uri
		}
		if m, ok := loadedSubConfigs[uri]; ok {
			// No need to emit load statement, but fix module name.
			mi.moduleLocalName = m
			continue
		}
		if mi.optional || mi.missing {
			uri += "|init"
		}
		gctx.newLine()
		gctx.writef("load(%q, %s = \"init\")", uri, mi.entryName())
		loadedSubConfigs[uri] = mi.moduleLocalName
	}
	gctx.write("\n")
}

func (gctx *generationContext) emitPass() {
	gctx.newLine()
	gctx.write("pass")
}

func (gctx *generationContext) write(ss ...string) {
	for _, s := range ss {
		gctx.buf.WriteString(s)
	}
}

func (gctx *generationContext) writef(format string, args ...interface{}) {
	gctx.write(fmt.Sprintf(format, args...))
}

func (gctx *generationContext) newLine() {
	if gctx.buf.Len() == 0 {
		return
	}
	gctx.write("\n")
	gctx.writef("%*s", 2*gctx.indentLevel, "")
}

func (gctx *generationContext) emitConversionError(el ErrorLocation, message string) {
	gctx.writef(`rblf.mk2rbc_error("%s", %q)`, el, message)
}

func (gctx *generationContext) emitLoadCheck(im inheritedModule) {
	if !im.needsLoadCheck() {
		return
	}
	gctx.newLine()
	gctx.writef("if not %s:", im.entryName())
	gctx.indentLevel++
	gctx.newLine()
	gctx.write(`rblf.mkerror("`, gctx.starScript.mkFile, `", "Cannot find %s" % (`)
	im.pathExpr().emit(gctx)
	gctx.write("))")
	gctx.indentLevel--
}

type knownVariable struct {
	name      string
	class     varClass
	valueType starlarkType
}

type knownVariables map[string]knownVariable

func (pcv knownVariables) NewVariable(name string, varClass varClass, valueType starlarkType) {
	v, exists := pcv[name]
	if !exists {
		pcv[name] = knownVariable{name, varClass, valueType}
		return
	}
	// Conflict resolution:
	//    * config class trumps everything
	//    * any type trumps unknown type
	match := varClass == v.class
	if !match {
		if varClass == VarClassConfig {
			v.class = VarClassConfig
			match = true
		} else if v.class == VarClassConfig {
			match = true
		}
	}
	if valueType != v.valueType {
		if valueType != starlarkTypeUnknown {
			if v.valueType == starlarkTypeUnknown {
				v.valueType = valueType
			} else {
				match = false
			}
		}
	}
	if !match {
		fmt.Fprintf(os.Stderr, "cannot redefine %s as %v/%v (already defined as %v/%v)\n",
			name, varClass, valueType, v.class, v.valueType)
	}
}

// All known product variables.
var KnownVariables = make(knownVariables)

func init() {
	for _, kv := range []string{
		// Kernel-related variables that we know are lists.
		"BOARD_VENDOR_KERNEL_MODULES",
		"BOARD_VENDOR_RAMDISK_KERNEL_MODULES",
		"BOARD_VENDOR_RAMDISK_KERNEL_MODULES_LOAD",
		"BOARD_RECOVERY_KERNEL_MODULES",
		// Other variables we knwo are lists
		"ART_APEX_JARS",
	} {
		KnownVariables.NewVariable(kv, VarClassSoong, starlarkTypeList)
	}
}

// Information about the generated Starlark script.
type StarlarkScript struct {
	mkFile         string
	moduleName     string
	mkPos          scanner.Position
	nodes          []starlarkNode
	inherited      []*moduleInfo
	hasErrors      bool
	traceCalls     bool // print enter/exit each init function
	sourceFS       fs.FS
	makefileFinder MakefileFinder
	nodeLocator    func(pos mkparser.Pos) int
}

// parseContext holds the script we are generating and all the ephemeral data
// needed during the parsing.
type parseContext struct {
	script           *StarlarkScript
	nodes            []mkparser.Node // Makefile as parsed by mkparser
	currentNodeIndex int             // Node in it we are processing
	ifNestLevel      int
	moduleNameCount  map[string]int // count of imported modules with given basename
	fatalError       error
	outputSuffix     string
	errorLogger      ErrorLogger
	tracedVariables  map[string]bool // variables to be traced in the generated script
	variables        map[string]variable
	outputDir        string
	dependentModules map[string]*moduleInfo
	soongNamespaces  map[string]map[string]bool
	includeTops      []string
	typeHints        map[string]starlarkType
	atTopOfMakefile  bool
}

func newParseContext(ss *StarlarkScript, nodes []mkparser.Node) *parseContext {
	predefined := []struct{ name, value string }{
		{"SRC_TARGET_DIR", filepath.Join("build", "make", "target")},
		{"LOCAL_PATH", filepath.Dir(ss.mkFile)},
		{"MAKEFILE_LIST", ss.mkFile},
		{"TOPDIR", ""}, // TOPDIR is just set to an empty string in cleanbuild.mk and core.mk
		// TODO(asmundak): maybe read it from build/make/core/envsetup.mk?
		{"TARGET_COPY_OUT_SYSTEM", "system"},
		{"TARGET_COPY_OUT_SYSTEM_OTHER", "system_other"},
		{"TARGET_COPY_OUT_DATA", "data"},
		{"TARGET_COPY_OUT_ASAN", filepath.Join("data", "asan")},
		{"TARGET_COPY_OUT_OEM", "oem"},
		{"TARGET_COPY_OUT_RAMDISK", "ramdisk"},
		{"TARGET_COPY_OUT_DEBUG_RAMDISK", "debug_ramdisk"},
		{"TARGET_COPY_OUT_VENDOR_DEBUG_RAMDISK", "vendor_debug_ramdisk"},
		{"TARGET_COPY_OUT_TEST_HARNESS_RAMDISK", "test_harness_ramdisk"},
		{"TARGET_COPY_OUT_ROOT", "root"},
		{"TARGET_COPY_OUT_RECOVERY", "recovery"},
		{"TARGET_COPY_OUT_VENDOR_RAMDISK", "vendor_ramdisk"},
		// TODO(asmundak): to process internal config files, we need the following variables:
		//    TARGET_VENDOR
		//    target_base_product
		//

		// the following utility variables are set in build/make/common/core.mk:
		{"empty", ""},
		{"space", " "},
		{"comma", ","},
		{"newline", "\n"},
		{"pound", "#"},
		{"backslash", "\\"},
	}
	ctx := &parseContext{
		script:           ss,
		nodes:            nodes,
		currentNodeIndex: 0,
		ifNestLevel:      0,
		moduleNameCount:  make(map[string]int),
		variables:        make(map[string]variable),
		dependentModules: make(map[string]*moduleInfo),
		soongNamespaces:  make(map[string]map[string]bool),
		includeTops:      []string{},
		typeHints:        make(map[string]starlarkType),
		atTopOfMakefile:  true,
	}
	for _, item := range predefined {
		ctx.variables[item.name] = &predefinedVariable{
			baseVariable: baseVariable{nam: item.name, typ: starlarkTypeString},
			value:        &stringLiteralExpr{item.value},
		}
	}

	return ctx
}

func (ctx *parseContext) hasNodes() bool {
	return ctx.currentNodeIndex < len(ctx.nodes)
}

func (ctx *parseContext) getNode() mkparser.Node {
	if !ctx.hasNodes() {
		return nil
	}
	node := ctx.nodes[ctx.currentNodeIndex]
	ctx.currentNodeIndex++
	return node
}

func (ctx *parseContext) backNode() {
	if ctx.currentNodeIndex <= 0 {
		panic("Cannot back off")
	}
	ctx.currentNodeIndex--
}

func (ctx *parseContext) handleAssignment(a *mkparser.Assignment) []starlarkNode {
	// Handle only simple variables
	if !a.Name.Const() || a.Target != nil {
		return []starlarkNode{ctx.newBadNode(a, "Only simple variables are handled")}
	}
	name := a.Name.Strings[0]
	// The `override` directive
	//      override FOO :=
	// is parsed as an assignment to a variable named `override FOO`.
	// There are very few places where `override` is used, just flag it.
	if strings.HasPrefix(name, "override ") {
		return []starlarkNode{ctx.newBadNode(a, "cannot handle override directive")}
	}
	if name == ".KATI_READONLY" {
		// Skip assignments to .KATI_READONLY. If it was in the output file, it
		// would be an error because it would be sorted before the definition of
		// the variable it's trying to make readonly.
		return []starlarkNode{}
	}

	// Soong configuration
	if strings.HasPrefix(name, soongNsPrefix) {
		return ctx.handleSoongNsAssignment(strings.TrimPrefix(name, soongNsPrefix), a)
	}
	lhs := ctx.addVariable(name)
	if lhs == nil {
		return []starlarkNode{ctx.newBadNode(a, "unknown variable %s", name)}
	}
	_, isTraced := ctx.tracedVariables[lhs.name()]
	asgn := &assignmentNode{lhs: lhs, mkValue: a.Value, isTraced: isTraced, location: ctx.errorLocation(a)}
	if lhs.valueType() == starlarkTypeUnknown {
		// Try to divine variable type from the RHS
		asgn.value = ctx.parseMakeString(a, a.Value)
		inferred_type := asgn.value.typ()
		if inferred_type != starlarkTypeUnknown {
			lhs.setValueType(inferred_type)
		}
	}
	if lhs.valueType() == starlarkTypeList {
		xConcat, xBad := ctx.buildConcatExpr(a)
		if xBad != nil {
			asgn.value = xBad
		} else {
			switch len(xConcat.items) {
			case 0:
				asgn.value = &listExpr{}
			case 1:
				asgn.value = xConcat.items[0]
			default:
				asgn.value = xConcat
			}
		}
	} else {
		asgn.value = ctx.parseMakeString(a, a.Value)
	}

	if asgn.lhs.valueType() == starlarkTypeString &&
		asgn.value.typ() != starlarkTypeUnknown &&
		asgn.value.typ() != starlarkTypeString {
		asgn.value = &toStringExpr{expr: asgn.value}
	}

	switch a.Type {
	case "=", ":=":
		asgn.flavor = asgnSet
	case "+=":
		asgn.flavor = asgnAppend
	case "?=":
		if _, ok := lhs.(*productConfigVariable); ok {
			// Make sets all product configuration variables to empty strings before running product
			// config makefiles. ?= will have no effect on a variable that has been assigned before,
			// even if assigned to an empty string. So just skip emitting any code for this
			// assignment.
			return nil
		}
		asgn.flavor = asgnMaybeSet
	default:
		panic(fmt.Errorf("unexpected assignment type %s", a.Type))
	}

	return []starlarkNode{asgn}
}

func (ctx *parseContext) handleSoongNsAssignment(name string, asgn *mkparser.Assignment) []starlarkNode {
	val := ctx.parseMakeString(asgn, asgn.Value)
	if xBad, ok := val.(*badExpr); ok {
		return []starlarkNode{&exprNode{expr: xBad}}
	}

	// Unfortunately, Soong namespaces can be set up by directly setting corresponding Make
	// variables instead of via add_soong_config_namespace + add_soong_config_var_value.
	// Try to divine the call from the assignment as follows:
	if name == "NAMESPACES" {
		// Upon seeng
		//      SOONG_CONFIG_NAMESPACES += foo
		//    remember that there is a namespace `foo` and act as we saw
		//      $(call add_soong_config_namespace,foo)
		s, ok := maybeString(val)
		if !ok {
			return []starlarkNode{ctx.newBadNode(asgn, "cannot handle variables in SOONG_CONFIG_NAMESPACES assignment, please use add_soong_config_namespace instead")}
		}
		result := make([]starlarkNode, 0)
		for _, ns := range strings.Fields(s) {
			ctx.addSoongNamespace(ns)
			result = append(result, &exprNode{&callExpr{
				name:       baseName + ".soong_config_namespace",
				args:       []starlarkExpr{&globalsExpr{}, &stringLiteralExpr{ns}},
				returnType: starlarkTypeVoid,
			}})
		}
		return result
	} else {
		// Upon seeing
		//      SOONG_CONFIG_x_y = v
		// find a namespace called `x` and act as if we encountered
		//      $(call soong_config_set,x,y,v)
		// or check that `x_y` is a namespace, and then add the RHS of this assignment as variables in
		// it.
		// Emit an error in the ambiguous situation (namespaces `foo_bar` with a variable `baz`
		// and `foo` with a variable `bar_baz`.
		namespaceName := ""
		if ctx.hasSoongNamespace(name) {
			namespaceName = name
		}
		var varName string
		for pos, ch := range name {
			if !(ch == '_' && ctx.hasSoongNamespace(name[0:pos])) {
				continue
			}
			if namespaceName != "" {
				return []starlarkNode{ctx.newBadNode(asgn, "ambiguous soong namespace (may be either `%s` or  `%s`)", namespaceName, name[0:pos])}
			}
			namespaceName = name[0:pos]
			varName = name[pos+1:]
		}
		if namespaceName == "" {
			return []starlarkNode{ctx.newBadNode(asgn, "cannot figure out Soong namespace, please use add_soong_config_var_value macro instead")}
		}
		if varName == "" {
			// Remember variables in this namespace
			s, ok := maybeString(val)
			if !ok {
				return []starlarkNode{ctx.newBadNode(asgn, "cannot handle variables in SOONG_CONFIG_ assignment, please use add_soong_config_var_value instead")}
			}
			ctx.updateSoongNamespace(asgn.Type != "+=", namespaceName, strings.Fields(s))
			return []starlarkNode{}
		}

		// Finally, handle assignment to a namespace variable
		if !ctx.hasNamespaceVar(namespaceName, varName) {
			return []starlarkNode{ctx.newBadNode(asgn, "no %s variable in %s namespace, please use add_soong_config_var_value instead", varName, namespaceName)}
		}
		fname := baseName + "." + soongConfigAssign
		if asgn.Type == "+=" {
			fname = baseName + "." + soongConfigAppend
		}
		return []starlarkNode{&exprNode{&callExpr{
			name:       fname,
			args:       []starlarkExpr{&globalsExpr{}, &stringLiteralExpr{namespaceName}, &stringLiteralExpr{varName}, val},
			returnType: starlarkTypeVoid,
		}}}
	}
}

func (ctx *parseContext) buildConcatExpr(a *mkparser.Assignment) (*concatExpr, *badExpr) {
	xConcat := &concatExpr{}
	var xItemList *listExpr
	addToItemList := func(x ...starlarkExpr) {
		if xItemList == nil {
			xItemList = &listExpr{[]starlarkExpr{}}
		}
		xItemList.items = append(xItemList.items, x...)
	}
	finishItemList := func() {
		if xItemList != nil {
			xConcat.items = append(xConcat.items, xItemList)
			xItemList = nil
		}
	}

	items := a.Value.Words()
	for _, item := range items {
		// A function call in RHS is supposed to return a list, all other item
		// expressions return individual elements.
		switch x := ctx.parseMakeString(a, item).(type) {
		case *badExpr:
			return nil, x
		case *stringLiteralExpr:
			addToItemList(maybeConvertToStringList(x).(*listExpr).items...)
		default:
			switch x.typ() {
			case starlarkTypeList:
				finishItemList()
				xConcat.items = append(xConcat.items, x)
			case starlarkTypeString:
				finishItemList()
				xConcat.items = append(xConcat.items, &callExpr{
					object:     x,
					name:       "split",
					args:       nil,
					returnType: starlarkTypeList,
				})
			default:
				addToItemList(x)
			}
		}
	}
	if xItemList != nil {
		xConcat.items = append(xConcat.items, xItemList)
	}
	return xConcat, nil
}

func (ctx *parseContext) newDependentModule(path string, optional bool) *moduleInfo {
	modulePath := ctx.loadedModulePath(path)
	if mi, ok := ctx.dependentModules[modulePath]; ok {
		mi.optional = mi.optional && optional
		return mi
	}
	moduleName := moduleNameForFile(path)
	moduleLocalName := "_" + moduleName
	n, found := ctx.moduleNameCount[moduleName]
	if found {
		moduleLocalName += fmt.Sprintf("%d", n)
	}
	ctx.moduleNameCount[moduleName] = n + 1
	_, err := fs.Stat(ctx.script.sourceFS, path)
	mi := &moduleInfo{
		path:            modulePath,
		originalPath:    path,
		moduleLocalName: moduleLocalName,
		optional:        optional,
		missing:         err != nil,
	}
	ctx.dependentModules[modulePath] = mi
	ctx.script.inherited = append(ctx.script.inherited, mi)
	return mi
}

func (ctx *parseContext) handleSubConfig(
	v mkparser.Node, pathExpr starlarkExpr, loadAlways bool, processModule func(inheritedModule) starlarkNode) []starlarkNode {

	// Allow seeing $(sort $(wildcard realPathExpr)) or $(wildcard realPathExpr)
	// because those are functionally the same as not having the sort/wildcard calls.
	if ce, ok := pathExpr.(*callExpr); ok && ce.name == "rblf.mksort" && len(ce.args) == 1 {
		if ce2, ok2 := ce.args[0].(*callExpr); ok2 && ce2.name == "rblf.expand_wildcard" && len(ce2.args) == 1 {
			pathExpr = ce2.args[0]
		}
	} else if ce2, ok2 := pathExpr.(*callExpr); ok2 && ce2.name == "rblf.expand_wildcard" && len(ce2.args) == 1 {
		pathExpr = ce2.args[0]
	}

	// In a simple case, the name of a module to inherit/include is known statically.
	if path, ok := maybeString(pathExpr); ok {
		// Note that even if this directive loads a module unconditionally, a module may be
		// absent without causing any harm if this directive is inside an if/else block.
		moduleShouldExist := loadAlways && ctx.ifNestLevel == 0
		if strings.Contains(path, "*") {
			if paths, err := fs.Glob(ctx.script.sourceFS, path); err == nil {
				sort.Strings(paths)
				result := make([]starlarkNode, 0)
				for _, p := range paths {
					mi := ctx.newDependentModule(p, !moduleShouldExist)
					result = append(result, processModule(inheritedStaticModule{mi, loadAlways}))
				}
				return result
			} else {
				return []starlarkNode{ctx.newBadNode(v, "cannot glob wildcard argument")}
			}
		} else {
			mi := ctx.newDependentModule(path, !moduleShouldExist)
			return []starlarkNode{processModule(inheritedStaticModule{mi, loadAlways})}
		}
	}

	// If module path references variables (e.g., $(v1)/foo/$(v2)/device-config.mk), find all the paths in the
	// source tree that may be a match and the corresponding variable values. For instance, if the source tree
	// contains vendor1/foo/abc/dev.mk and vendor2/foo/def/dev.mk, the first one will be inherited when
	// (v1, v2) == ('vendor1', 'abc'), and the second one when (v1, v2) == ('vendor2', 'def').
	// We then emit the code that loads all of them, e.g.:
	//    load("//vendor1/foo/abc:dev.rbc", _dev1_init="init")
	//    load("//vendor2/foo/def/dev.rbc", _dev2_init="init")
	// And then inherit it as follows:
	//    _e = {
	//       "vendor1/foo/abc/dev.mk": ("vendor1/foo/abc/dev", _dev1_init),
	//       "vendor2/foo/def/dev.mk": ("vendor2/foo/def/dev", _dev_init2) }.get("%s/foo/%s/dev.mk" % (v1, v2))
	//    if _e:
	//       rblf.inherit(handle, _e[0], _e[1])
	//
	var matchingPaths []string
	var needsWarning = false
	if interpolate, ok := pathExpr.(*interpolateExpr); ok {
		pathPattern := []string{interpolate.chunks[0]}
		for _, chunk := range interpolate.chunks[1:] {
			if chunk != "" {
				pathPattern = append(pathPattern, chunk)
			}
		}
		if len(pathPattern) == 1 {
			pathPattern = append(pathPattern, "")
		}
		matchingPaths = ctx.findMatchingPaths(pathPattern)
		needsWarning = pathPattern[0] == "" && len(ctx.includeTops) == 0
	} else if len(ctx.includeTops) > 0 {
		matchingPaths = append(matchingPaths, ctx.findMatchingPaths([]string{"", ""})...)
	} else {
		return []starlarkNode{ctx.newBadNode(v, "inherit-product/include argument is too complex")}
	}

	// Safeguard against $(call inherit-product,$(PRODUCT_PATH))
	const maxMatchingFiles = 150
	if len(matchingPaths) > maxMatchingFiles {
		return []starlarkNode{ctx.newBadNode(v, "there are >%d files matching the pattern, please rewrite it", maxMatchingFiles)}
	}

	res := inheritedDynamicModule{pathExpr, []*moduleInfo{}, loadAlways, ctx.errorLocation(v), needsWarning}
	for _, p := range matchingPaths {
		// A product configuration files discovered dynamically may attempt to inherit
		// from another one which does not exist in this source tree. Prevent load errors
		// by always loading the dynamic files as optional.
		res.candidateModules = append(res.candidateModules, ctx.newDependentModule(p, true))
	}
	return []starlarkNode{processModule(res)}
}

func (ctx *parseContext) findMatchingPaths(pattern []string) []string {
	files := ctx.script.makefileFinder.Find(".")
	if len(pattern) == 0 {
		return files
	}

	// Create regular expression from the pattern
	regexString := "^" + regexp.QuoteMeta(pattern[0])
	for _, s := range pattern[1:] {
		regexString += ".*" + regexp.QuoteMeta(s)
	}
	regexString += "$"
	rex := regexp.MustCompile(regexString)

	includeTopRegexString := ""
	if len(ctx.includeTops) > 0 {
		for i, top := range ctx.includeTops {
			if i > 0 {
				includeTopRegexString += "|"
			}
			includeTopRegexString += "^" + regexp.QuoteMeta(top)
		}
	} else {
		includeTopRegexString = ".*"
	}

	includeTopRegex := regexp.MustCompile(includeTopRegexString)

	// Now match
	var res []string
	for _, p := range files {
		if rex.MatchString(p) && includeTopRegex.MatchString(p) {
			res = append(res, p)
		}
	}
	return res
}

type inheritProductCallParser struct {
	loadAlways bool
}

func (p *inheritProductCallParser) parse(ctx *parseContext, v mkparser.Node, args *mkparser.MakeString) []starlarkNode {
	args.TrimLeftSpaces()
	args.TrimRightSpaces()
	pathExpr := ctx.parseMakeString(v, args)
	if _, ok := pathExpr.(*badExpr); ok {
		return []starlarkNode{ctx.newBadNode(v, "Unable to parse argument to inherit")}
	}
	return ctx.handleSubConfig(v, pathExpr, p.loadAlways, func(im inheritedModule) starlarkNode {
		return &inheritNode{im, p.loadAlways}
	})
}

func (ctx *parseContext) handleInclude(v *mkparser.Directive) []starlarkNode {
	loadAlways := v.Name[0] != '-'
	v.Args.TrimRightSpaces()
	v.Args.TrimLeftSpaces()
	return ctx.handleSubConfig(v, ctx.parseMakeString(v, v.Args), loadAlways, func(im inheritedModule) starlarkNode {
		return &includeNode{im, loadAlways}
	})
}

func (ctx *parseContext) handleVariable(v *mkparser.Variable) []starlarkNode {
	// Handle:
	//   $(call inherit-product,...)
	//   $(call inherit-product-if-exists,...)
	//   $(info xxx)
	//   $(warning xxx)
	//   $(error xxx)
	//   $(call other-custom-functions,...)

	if name, args, ok := ctx.maybeParseFunctionCall(v, v.Name); ok {
		if kf, ok := knownNodeFunctions[name]; ok {
			return kf.parse(ctx, v, args)
		}
	}

	return []starlarkNode{&exprNode{expr: ctx.parseReference(v, v.Name)}}
}

func (ctx *parseContext) maybeHandleDefine(directive *mkparser.Directive) starlarkNode {
	macro_name := strings.Fields(directive.Args.Strings[0])[0]
	// Ignore the macros that we handle
	_, ignored := ignoredDefines[macro_name]
	_, known := knownFunctions[macro_name]
	if !ignored && !known {
		return ctx.newBadNode(directive, "define is not supported: %s", macro_name)
	}
	return nil
}

func (ctx *parseContext) handleIfBlock(ifDirective *mkparser.Directive) starlarkNode {
	ssSwitch := &switchNode{
		ssCases: []*switchCase{ctx.processBranch(ifDirective)},
	}
	for ctx.hasNodes() && ctx.fatalError == nil {
		node := ctx.getNode()
		switch x := node.(type) {
		case *mkparser.Directive:
			switch x.Name {
			case "else", "elifdef", "elifndef", "elifeq", "elifneq":
				ssSwitch.ssCases = append(ssSwitch.ssCases, ctx.processBranch(x))
			case "endif":
				return ssSwitch
			default:
				return ctx.newBadNode(node, "unexpected directive %s", x.Name)
			}
		default:
			return ctx.newBadNode(ifDirective, "unexpected statement")
		}
	}
	if ctx.fatalError == nil {
		ctx.fatalError = fmt.Errorf("no matching endif for %s", ifDirective.Dump())
	}
	return ctx.newBadNode(ifDirective, "no matching endif for %s", ifDirective.Dump())
}

// processBranch processes a single branch (if/elseif/else) until the next directive
// on the same level.
func (ctx *parseContext) processBranch(check *mkparser.Directive) *switchCase {
	block := &switchCase{gate: ctx.parseCondition(check)}
	defer func() {
		ctx.ifNestLevel--
	}()
	ctx.ifNestLevel++

	for ctx.hasNodes() {
		node := ctx.getNode()
		if d, ok := node.(*mkparser.Directive); ok {
			switch d.Name {
			case "else", "elifdef", "elifndef", "elifeq", "elifneq", "endif":
				ctx.backNode()
				return block
			}
		}
		block.nodes = append(block.nodes, ctx.handleSimpleStatement(node)...)
	}
	ctx.fatalError = fmt.Errorf("no matching endif for %s", check.Dump())
	return block
}

func (ctx *parseContext) parseCondition(check *mkparser.Directive) starlarkNode {
	switch check.Name {
	case "ifdef", "ifndef", "elifdef", "elifndef":
		if !check.Args.Const() {
			return ctx.newBadNode(check, "ifdef variable ref too complex: %s", check.Args.Dump())
		}
		v := NewVariableRefExpr(ctx.addVariable(check.Args.Strings[0]))
		if strings.HasSuffix(check.Name, "ndef") {
			v = &notExpr{v}
		}
		return &ifNode{
			isElif: strings.HasPrefix(check.Name, "elif"),
			expr:   v,
		}
	case "ifeq", "ifneq", "elifeq", "elifneq":
		return &ifNode{
			isElif: strings.HasPrefix(check.Name, "elif"),
			expr:   ctx.parseCompare(check),
		}
	case "else":
		return &elseNode{}
	default:
		panic(fmt.Errorf("%s: unknown directive: %s", ctx.script.mkFile, check.Dump()))
	}
}

func (ctx *parseContext) newBadExpr(node mkparser.Node, text string, args ...interface{}) starlarkExpr {
	if ctx.errorLogger != nil {
		ctx.errorLogger.NewError(ctx.errorLocation(node), node, text, args...)
	}
	ctx.script.hasErrors = true
	return &badExpr{errorLocation: ctx.errorLocation(node), message: fmt.Sprintf(text, args...)}
}

// records that the given node failed to be converted and includes an explanatory message
func (ctx *parseContext) newBadNode(failedNode mkparser.Node, message string, args ...interface{}) starlarkNode {
	return &exprNode{ctx.newBadExpr(failedNode, message, args...)}
}

func (ctx *parseContext) parseCompare(cond *mkparser.Directive) starlarkExpr {
	// Strip outer parentheses
	mkArg := cloneMakeString(cond.Args)
	mkArg.Strings[0] = strings.TrimLeft(mkArg.Strings[0], "( ")
	n := len(mkArg.Strings)
	mkArg.Strings[n-1] = strings.TrimRight(mkArg.Strings[n-1], ") ")
	args := mkArg.Split(",")
	// TODO(asmundak): handle the case where the arguments are in quotes and space-separated
	if len(args) != 2 {
		return ctx.newBadExpr(cond, "ifeq/ifneq len(args) != 2 %s", cond.Dump())
	}
	args[0].TrimRightSpaces()
	args[1].TrimLeftSpaces()

	isEq := !strings.HasSuffix(cond.Name, "neq")
	xLeft := ctx.parseMakeString(cond, args[0])
	xRight := ctx.parseMakeString(cond, args[1])
	if bad, ok := xLeft.(*badExpr); ok {
		return bad
	}
	if bad, ok := xRight.(*badExpr); ok {
		return bad
	}

	if expr, ok := ctx.parseCompareSpecialCases(cond, xLeft, xRight); ok {
		return expr
	}

	var stringOperand string
	var otherOperand starlarkExpr
	if s, ok := maybeString(xLeft); ok {
		stringOperand = s
		otherOperand = xRight
	} else if s, ok := maybeString(xRight); ok {
		stringOperand = s
		otherOperand = xLeft
	}

	// If we've identified one of the operands as being a string literal, check
	// for some special cases we can do to simplify the resulting expression.
	if otherOperand != nil {
		if stringOperand == "" {
			if isEq {
				return negateExpr(otherOperand)
			} else {
				return otherOperand
			}
		}
		if stringOperand == "true" && otherOperand.typ() == starlarkTypeBool {
			if !isEq {
				return negateExpr(otherOperand)
			} else {
				return otherOperand
			}
		}
		if otherOperand.typ() == starlarkTypeList {
			fields := strings.Fields(stringOperand)
			elements := make([]starlarkExpr, len(fields))
			for i, s := range fields {
				elements[i] = &stringLiteralExpr{literal: s}
			}
			return &eqExpr{
				left:  otherOperand,
				right: &listExpr{elements},
				isEq:  isEq,
			}
		}
		if intOperand, err := strconv.Atoi(strings.TrimSpace(stringOperand)); err == nil && otherOperand.typ() == starlarkTypeInt {
			return &eqExpr{
				left:  otherOperand,
				right: &intLiteralExpr{literal: intOperand},
				isEq:  isEq,
			}
		}
	}

	return &eqExpr{left: xLeft, right: xRight, isEq: isEq}
}

// Given an if statement's directive and the left/right starlarkExprs,
// check if the starlarkExprs are one of a few hardcoded special cases
// that can be converted to a simpler equality expression than simply comparing
// the two.
func (ctx *parseContext) parseCompareSpecialCases(directive *mkparser.Directive, left starlarkExpr,
	right starlarkExpr) (starlarkExpr, bool) {
	isEq := !strings.HasSuffix(directive.Name, "neq")

	// All the special cases require a call on one side and a
	// string literal/variable on the other. Turn the left/right variables into
	// call/value variables, and return false if that's not possible.
	var value starlarkExpr = nil
	call, ok := left.(*callExpr)
	if ok {
		switch right.(type) {
		case *stringLiteralExpr, *variableRefExpr:
			value = right
		}
	} else {
		call, _ = right.(*callExpr)
		switch left.(type) {
		case *stringLiteralExpr, *variableRefExpr:
			value = left
		}
	}

	if call == nil || value == nil {
		return nil, false
	}

	switch call.name {
	case baseName + ".filter":
		return ctx.parseCompareFilterFuncResult(directive, call, value, isEq)
	case baseName + ".findstring":
		return ctx.parseCheckFindstringFuncResult(directive, call, value, !isEq), true
	case baseName + ".strip":
		return ctx.parseCompareStripFuncResult(directive, call, value, !isEq), true
	}
	return nil, false
}

func (ctx *parseContext) parseCompareFilterFuncResult(cond *mkparser.Directive,
	filterFuncCall *callExpr, xValue starlarkExpr, negate bool) (starlarkExpr, bool) {
	// We handle:
	// *  ifeq/ifneq (,$(filter v1 v2 ..., EXPR) becomes if EXPR not in/in ["v1", "v2", ...]
	// *  ifeq/ifneq (,$(filter EXPR, v1 v2 ...) becomes if EXPR not in/in ["v1", "v2", ...]
	if x, ok := xValue.(*stringLiteralExpr); !ok || x.literal != "" {
		return nil, false
	}
	xPattern := filterFuncCall.args[0]
	xText := filterFuncCall.args[1]
	var xInList *stringLiteralExpr
	var expr starlarkExpr
	var ok bool
	if xInList, ok = xPattern.(*stringLiteralExpr); ok && !strings.ContainsRune(xInList.literal, '%') && xText.typ() == starlarkTypeList {
		expr = xText
	} else if xInList, ok = xText.(*stringLiteralExpr); ok {
		expr = xPattern
	} else {
		return nil, false
	}
	slExpr := newStringListExpr(strings.Fields(xInList.literal))
	// Generate simpler code for the common cases:
	if expr.typ() == starlarkTypeList {
		if len(slExpr.items) == 1 {
			// Checking that a string belongs to list
			return &inExpr{isNot: negate, list: expr, expr: slExpr.items[0]}, true
		} else {
			return nil, false
		}
	} else if len(slExpr.items) == 1 {
		return &eqExpr{left: expr, right: slExpr.items[0], isEq: !negate}, true
	} else {
		return &inExpr{isNot: negate, list: newStringListExpr(strings.Fields(xInList.literal)), expr: expr}, true
	}
}

func (ctx *parseContext) parseCheckFindstringFuncResult(directive *mkparser.Directive,
	xCall *callExpr, xValue starlarkExpr, negate bool) starlarkExpr {
	if isEmptyString(xValue) {
		return &eqExpr{
			left: &callExpr{
				object:     xCall.args[1],
				name:       "find",
				args:       []starlarkExpr{xCall.args[0]},
				returnType: starlarkTypeInt,
			},
			right: &intLiteralExpr{-1},
			isEq:  !negate,
		}
	} else if s, ok := maybeString(xValue); ok {
		if s2, ok := maybeString(xCall.args[0]); ok && s == s2 {
			return &eqExpr{
				left: &callExpr{
					object:     xCall.args[1],
					name:       "find",
					args:       []starlarkExpr{xCall.args[0]},
					returnType: starlarkTypeInt,
				},
				right: &intLiteralExpr{-1},
				isEq:  negate,
			}
		}
	}
	return ctx.newBadExpr(directive, "$(findstring) can only be compared to nothing or its first argument")
}

func (ctx *parseContext) parseCompareStripFuncResult(directive *mkparser.Directive,
	xCall *callExpr, xValue starlarkExpr, negate bool) starlarkExpr {
	if _, ok := xValue.(*stringLiteralExpr); !ok {
		return ctx.newBadExpr(directive, "strip result can be compared only to string: %s", xValue)
	}
	return &eqExpr{
		left: &callExpr{
			name:       "strip",
			args:       xCall.args,
			returnType: starlarkTypeString,
		},
		right: xValue, isEq: !negate}
}

func (ctx *parseContext) maybeParseFunctionCall(node mkparser.Node, ref *mkparser.MakeString) (name string, args *mkparser.MakeString, ok bool) {
	ref.TrimLeftSpaces()
	ref.TrimRightSpaces()

	words := ref.SplitN(" ", 2)
	if !words[0].Const() {
		return "", nil, false
	}

	name = words[0].Dump()
	args = mkparser.SimpleMakeString("", words[0].Pos())
	if len(words) >= 2 {
		args = words[1]
	}
	args.TrimLeftSpaces()
	if name == "call" {
		words = args.SplitN(",", 2)
		if words[0].Empty() || !words[0].Const() {
			return "", nil, false
		}
		name = words[0].Dump()
		if len(words) < 2 {
			args = mkparser.SimpleMakeString("", words[0].Pos())
		} else {
			args = words[1]
		}
	}
	ok = true
	return
}

// parses $(...), returning an expression
func (ctx *parseContext) parseReference(node mkparser.Node, ref *mkparser.MakeString) starlarkExpr {
	ref.TrimLeftSpaces()
	ref.TrimRightSpaces()
	refDump := ref.Dump()

	// Handle only the case where the first (or only) word is constant
	words := ref.SplitN(" ", 2)
	if !words[0].Const() {
		if len(words) == 1 {
			expr := ctx.parseMakeString(node, ref)
			return &callExpr{
				object: &identifierExpr{"cfg"},
				name:   "get",
				args: []starlarkExpr{
					expr,
					&callExpr{
						object: &identifierExpr{"g"},
						name:   "get",
						args: []starlarkExpr{
							expr,
							&stringLiteralExpr{literal: ""},
						},
						returnType: starlarkTypeUnknown,
					},
				},
				returnType: starlarkTypeUnknown,
			}
		} else {
			return ctx.newBadExpr(node, "reference is too complex: %s", refDump)
		}
	}

	if name, _, ok := ctx.maybeParseFunctionCall(node, ref); ok {
		if _, unsupported := unsupportedFunctions[name]; unsupported {
			return ctx.newBadExpr(node, "%s is not supported", refDump)
		}
	}

	// If it is a single word, it can be a simple variable
	// reference or a function call
	if len(words) == 1 && !isMakeControlFunc(refDump) && refDump != "shell" && refDump != "eval" {
		if strings.HasPrefix(refDump, soongNsPrefix) {
			// TODO (asmundak): if we find many, maybe handle them.
			return ctx.newBadExpr(node, "SOONG_CONFIG_ variables cannot be referenced, use soong_config_get instead: %s", refDump)
		}
		// Handle substitution references: https://www.gnu.org/software/make/manual/html_node/Substitution-Refs.html
		if strings.Contains(refDump, ":") {
			parts := strings.SplitN(refDump, ":", 2)
			substParts := strings.SplitN(parts[1], "=", 2)
			if len(substParts) < 2 || strings.Count(substParts[0], "%") > 1 {
				return ctx.newBadExpr(node, "Invalid substitution reference")
			}
			if !strings.Contains(substParts[0], "%") {
				if strings.Contains(substParts[1], "%") {
					return ctx.newBadExpr(node, "A substitution reference must have a %% in the \"before\" part of the substitution if it has one in the \"after\" part.")
				}
				substParts[0] = "%" + substParts[0]
				substParts[1] = "%" + substParts[1]
			}
			v := ctx.addVariable(parts[0])
			if v == nil {
				return ctx.newBadExpr(node, "unknown variable %s", refDump)
			}
			return &callExpr{
				name:       baseName + ".mkpatsubst",
				returnType: starlarkTypeString,
				args: []starlarkExpr{
					&stringLiteralExpr{literal: substParts[0]},
					&stringLiteralExpr{literal: substParts[1]},
					NewVariableRefExpr(v),
				},
			}
		}
		if v := ctx.addVariable(refDump); v != nil {
			return NewVariableRefExpr(v)
		}
		return ctx.newBadExpr(node, "unknown variable %s", refDump)
	}

	if name, args, ok := ctx.maybeParseFunctionCall(node, ref); ok {
		if kf, found := knownFunctions[name]; found {
			return kf.parse(ctx, node, args)
		} else {
			return ctx.newBadExpr(node, "cannot handle invoking %s", name)
		}
	}
	return ctx.newBadExpr(node, "cannot handle %s", refDump)
}

type simpleCallParser struct {
	name       string
	returnType starlarkType
	addGlobals bool
	addHandle  bool
}

func (p *simpleCallParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) starlarkExpr {
	expr := &callExpr{name: p.name, returnType: p.returnType}
	if p.addGlobals {
		expr.args = append(expr.args, &globalsExpr{})
	}
	if p.addHandle {
		expr.args = append(expr.args, &identifierExpr{name: "handle"})
	}
	for _, arg := range args.Split(",") {
		arg.TrimLeftSpaces()
		arg.TrimRightSpaces()
		x := ctx.parseMakeString(node, arg)
		if xBad, ok := x.(*badExpr); ok {
			return xBad
		}
		expr.args = append(expr.args, x)
	}
	return expr
}

type makeControlFuncParser struct {
	name string
}

func (p *makeControlFuncParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) starlarkExpr {
	// Make control functions need special treatment as everything
	// after the name is a single text argument
	x := ctx.parseMakeString(node, args)
	if xBad, ok := x.(*badExpr); ok {
		return xBad
	}
	return &callExpr{
		name: p.name,
		args: []starlarkExpr{
			&stringLiteralExpr{ctx.script.mkFile},
			x,
		},
		returnType: starlarkTypeUnknown,
	}
}

type shellCallParser struct{}

func (p *shellCallParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) starlarkExpr {
	// Shell functions need special treatment as everything
	// after the name is a single text argument
	x := ctx.parseMakeString(node, args)
	if xBad, ok := x.(*badExpr); ok {
		return xBad
	}
	return &callExpr{
		name:       baseName + ".shell",
		args:       []starlarkExpr{x},
		returnType: starlarkTypeUnknown,
	}
}

type myDirCallParser struct{}

func (p *myDirCallParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) starlarkExpr {
	if !args.Empty() {
		return ctx.newBadExpr(node, "my-dir function cannot have any arguments passed to it.")
	}
	return &stringLiteralExpr{literal: filepath.Dir(ctx.script.mkFile)}
}

type andOrParser struct {
	isAnd bool
}

func (p *andOrParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) starlarkExpr {
	if args.Empty() {
		return ctx.newBadExpr(node, "and/or function must have at least 1 argument")
	}
	op := "or"
	if p.isAnd {
		op = "and"
	}

	argsParsed := make([]starlarkExpr, 0)

	for _, arg := range args.Split(",") {
		arg.TrimLeftSpaces()
		arg.TrimRightSpaces()
		x := ctx.parseMakeString(node, arg)
		if xBad, ok := x.(*badExpr); ok {
			return xBad
		}
		argsParsed = append(argsParsed, x)
	}
	typ := starlarkTypeUnknown
	for _, arg := range argsParsed {
		if typ != arg.typ() && arg.typ() != starlarkTypeUnknown && typ != starlarkTypeUnknown {
			return ctx.newBadExpr(node, "Expected all arguments to $(or) or $(and) to have the same type, found %q and %q", typ.String(), arg.typ().String())
		}
		if arg.typ() != starlarkTypeUnknown {
			typ = arg.typ()
		}
	}
	result := argsParsed[0]
	for _, arg := range argsParsed[1:] {
		result = &binaryOpExpr{
			left:       result,
			right:      arg,
			op:         op,
			returnType: typ,
		}
	}
	return result
}

type isProductInListCallParser struct{}

func (p *isProductInListCallParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) starlarkExpr {
	if args.Empty() {
		return ctx.newBadExpr(node, "is-product-in-list requires an argument")
	}
	return &inExpr{
		expr:  NewVariableRefExpr(ctx.addVariable("TARGET_PRODUCT")),
		list:  maybeConvertToStringList(ctx.parseMakeString(node, args)),
		isNot: false,
	}
}

type isVendorBoardPlatformCallParser struct{}

func (p *isVendorBoardPlatformCallParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) starlarkExpr {
	if args.Empty() || !identifierFullMatchRegex.MatchString(args.Dump()) {
		return ctx.newBadExpr(node, "cannot handle non-constant argument to is-vendor-board-platform")
	}
	return &inExpr{
		expr:  NewVariableRefExpr(ctx.addVariable("TARGET_BOARD_PLATFORM")),
		list:  NewVariableRefExpr(ctx.addVariable(args.Dump() + "_BOARD_PLATFORMS")),
		isNot: false,
	}
}

type isVendorBoardQcomCallParser struct{}

func (p *isVendorBoardQcomCallParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) starlarkExpr {
	if !args.Empty() {
		return ctx.newBadExpr(node, "is-vendor-board-qcom does not accept any arguments")
	}
	return &inExpr{
		expr:  NewVariableRefExpr(ctx.addVariable("TARGET_BOARD_PLATFORM")),
		list:  NewVariableRefExpr(ctx.addVariable("QCOM_BOARD_PLATFORMS")),
		isNot: false,
	}
}

type substCallParser struct {
	fname string
}

func (p *substCallParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) starlarkExpr {
	words := args.Split(",")
	if len(words) != 3 {
		return ctx.newBadExpr(node, "%s function should have 3 arguments", p.fname)
	}
	from := ctx.parseMakeString(node, words[0])
	if xBad, ok := from.(*badExpr); ok {
		return xBad
	}
	to := ctx.parseMakeString(node, words[1])
	if xBad, ok := to.(*badExpr); ok {
		return xBad
	}
	words[2].TrimLeftSpaces()
	words[2].TrimRightSpaces()
	obj := ctx.parseMakeString(node, words[2])
	typ := obj.typ()
	if typ == starlarkTypeString && p.fname == "subst" {
		// Optimization: if it's $(subst from, to, string), emit string.replace(from, to)
		return &callExpr{
			object:     obj,
			name:       "replace",
			args:       []starlarkExpr{from, to},
			returnType: typ,
		}
	}
	return &callExpr{
		name:       baseName + ".mk" + p.fname,
		args:       []starlarkExpr{from, to, obj},
		returnType: obj.typ(),
	}
}

type ifCallParser struct{}

func (p *ifCallParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) starlarkExpr {
	words := args.Split(",")
	if len(words) != 2 && len(words) != 3 {
		return ctx.newBadExpr(node, "if function should have 2 or 3 arguments, found "+strconv.Itoa(len(words)))
	}
	condition := ctx.parseMakeString(node, words[0])
	ifTrue := ctx.parseMakeString(node, words[1])
	var ifFalse starlarkExpr
	if len(words) == 3 {
		ifFalse = ctx.parseMakeString(node, words[2])
	} else {
		switch ifTrue.typ() {
		case starlarkTypeList:
			ifFalse = &listExpr{items: []starlarkExpr{}}
		case starlarkTypeInt:
			ifFalse = &intLiteralExpr{literal: 0}
		case starlarkTypeBool:
			ifFalse = &boolLiteralExpr{literal: false}
		default:
			ifFalse = &stringLiteralExpr{literal: ""}
		}
	}
	return &ifExpr{
		condition,
		ifTrue,
		ifFalse,
	}
}

type ifCallNodeParser struct{}

func (p *ifCallNodeParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) []starlarkNode {
	words := args.Split(",")
	if len(words) != 2 && len(words) != 3 {
		return []starlarkNode{ctx.newBadNode(node, "if function should have 2 or 3 arguments, found "+strconv.Itoa(len(words)))}
	}

	ifn := &ifNode{expr: ctx.parseMakeString(node, words[0])}
	cases := []*switchCase{
		{
			gate:  ifn,
			nodes: ctx.parseNodeMakeString(node, words[1]),
		},
	}
	if len(words) == 3 {
		cases = append(cases, &switchCase{
			gate:  &elseNode{},
			nodes: ctx.parseNodeMakeString(node, words[2]),
		})
	}
	if len(cases) == 2 {
		if len(cases[1].nodes) == 0 {
			// Remove else branch if it has no contents
			cases = cases[:1]
		} else if len(cases[0].nodes) == 0 {
			// If the if branch has no contents but the else does,
			// move them to the if and negate its condition
			ifn.expr = negateExpr(ifn.expr)
			cases[0].nodes = cases[1].nodes
			cases = cases[:1]
		}
	}

	return []starlarkNode{&switchNode{ssCases: cases}}
}

type foreachCallParser struct{}

func (p *foreachCallParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) starlarkExpr {
	words := args.Split(",")
	if len(words) != 3 {
		return ctx.newBadExpr(node, "foreach function should have 3 arguments, found "+strconv.Itoa(len(words)))
	}
	if !words[0].Const() || words[0].Empty() || !identifierFullMatchRegex.MatchString(words[0].Strings[0]) {
		return ctx.newBadExpr(node, "first argument to foreach function must be a simple string identifier")
	}
	loopVarName := words[0].Strings[0]
	list := ctx.parseMakeString(node, words[1])
	action := ctx.parseMakeString(node, words[2]).transform(func(expr starlarkExpr) starlarkExpr {
		if varRefExpr, ok := expr.(*variableRefExpr); ok && varRefExpr.ref.name() == loopVarName {
			return &identifierExpr{loopVarName}
		}
		return nil
	})

	if list.typ() != starlarkTypeList {
		list = &callExpr{
			name:       baseName + ".words",
			returnType: starlarkTypeList,
			args:       []starlarkExpr{list},
		}
	}

	var result starlarkExpr = &foreachExpr{
		varName: loopVarName,
		list:    list,
		action:  action,
	}

	if action.typ() == starlarkTypeList {
		result = &callExpr{
			name:       baseName + ".flatten_2d_list",
			args:       []starlarkExpr{result},
			returnType: starlarkTypeList,
		}
	}

	return result
}

func transformNode(node starlarkNode, transformer func(expr starlarkExpr) starlarkExpr) {
	switch a := node.(type) {
	case *ifNode:
		a.expr = a.expr.transform(transformer)
	case *switchCase:
		transformNode(a.gate, transformer)
		for _, n := range a.nodes {
			transformNode(n, transformer)
		}
	case *switchNode:
		for _, n := range a.ssCases {
			transformNode(n, transformer)
		}
	case *exprNode:
		a.expr = a.expr.transform(transformer)
	case *assignmentNode:
		a.value = a.value.transform(transformer)
	case *foreachNode:
		a.list = a.list.transform(transformer)
		for _, n := range a.actions {
			transformNode(n, transformer)
		}
	case *inheritNode:
		if b, ok := a.module.(inheritedDynamicModule); ok {
			b.path = b.path.transform(transformer)
			a.module = b
		}
	case *includeNode:
		if b, ok := a.module.(inheritedDynamicModule); ok {
			b.path = b.path.transform(transformer)
			a.module = b
		}
	}
}

type foreachCallNodeParser struct{}

func (p *foreachCallNodeParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) []starlarkNode {
	words := args.Split(",")
	if len(words) != 3 {
		return []starlarkNode{ctx.newBadNode(node, "foreach function should have 3 arguments, found "+strconv.Itoa(len(words)))}
	}
	if !words[0].Const() || words[0].Empty() || !identifierFullMatchRegex.MatchString(words[0].Strings[0]) {
		return []starlarkNode{ctx.newBadNode(node, "first argument to foreach function must be a simple string identifier")}
	}

	loopVarName := words[0].Strings[0]

	list := ctx.parseMakeString(node, words[1])
	if list.typ() != starlarkTypeList {
		list = &callExpr{
			name:       baseName + ".words",
			returnType: starlarkTypeList,
			args:       []starlarkExpr{list},
		}
	}

	actions := ctx.parseNodeMakeString(node, words[2])
	// TODO(colefaust): Replace transforming code with something more elegant
	for _, action := range actions {
		transformNode(action, func(expr starlarkExpr) starlarkExpr {
			if varRefExpr, ok := expr.(*variableRefExpr); ok && varRefExpr.ref.name() == loopVarName {
				return &identifierExpr{loopVarName}
			}
			return nil
		})
	}

	return []starlarkNode{&foreachNode{
		varName: loopVarName,
		list:    list,
		actions: actions,
	}}
}

type wordCallParser struct{}

func (p *wordCallParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) starlarkExpr {
	words := args.Split(",")
	if len(words) != 2 {
		return ctx.newBadExpr(node, "word function should have 2 arguments")
	}
	var index = 0
	if words[0].Const() {
		if i, err := strconv.Atoi(strings.TrimSpace(words[0].Strings[0])); err == nil {
			index = i
		}
	}
	if index < 1 {
		return ctx.newBadExpr(node, "word index should be constant positive integer")
	}
	words[1].TrimLeftSpaces()
	words[1].TrimRightSpaces()
	array := ctx.parseMakeString(node, words[1])
	if bad, ok := array.(*badExpr); ok {
		return bad
	}
	if array.typ() != starlarkTypeList {
		array = &callExpr{
			name:       baseName + ".words",
			args:       []starlarkExpr{array},
			returnType: starlarkTypeList,
		}
	}
	return &indexExpr{array, &intLiteralExpr{index - 1}}
}

type wordsCallParser struct{}

func (p *wordsCallParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) starlarkExpr {
	args.TrimLeftSpaces()
	args.TrimRightSpaces()
	array := ctx.parseMakeString(node, args)
	if bad, ok := array.(*badExpr); ok {
		return bad
	}
	if array.typ() != starlarkTypeList {
		array = &callExpr{
			name:       baseName + ".words",
			args:       []starlarkExpr{array},
			returnType: starlarkTypeList,
		}
	}
	return &callExpr{
		name:       "len",
		args:       []starlarkExpr{array},
		returnType: starlarkTypeInt,
	}
}

func parseIntegerArguments(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString, expectedArgs int) ([]starlarkExpr, error) {
	parsedArgs := make([]starlarkExpr, 0)
	for _, arg := range args.Split(",") {
		expr := ctx.parseMakeString(node, arg)
		if expr.typ() == starlarkTypeList {
			return nil, fmt.Errorf("argument to math argument has type list, which cannot be converted to int")
		}
		if s, ok := maybeString(expr); ok {
			intVal, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil {
				return nil, err
			}
			expr = &intLiteralExpr{literal: intVal}
		} else if expr.typ() != starlarkTypeInt {
			expr = &callExpr{
				name:       "int",
				args:       []starlarkExpr{expr},
				returnType: starlarkTypeInt,
			}
		}
		parsedArgs = append(parsedArgs, expr)
	}
	if len(parsedArgs) != expectedArgs {
		return nil, fmt.Errorf("function should have %d arguments", expectedArgs)
	}
	return parsedArgs, nil
}

type mathComparisonCallParser struct {
	op string
}

func (p *mathComparisonCallParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) starlarkExpr {
	parsedArgs, err := parseIntegerArguments(ctx, node, args, 2)
	if err != nil {
		return ctx.newBadExpr(node, err.Error())
	}
	return &binaryOpExpr{
		left:       parsedArgs[0],
		right:      parsedArgs[1],
		op:         p.op,
		returnType: starlarkTypeBool,
	}
}

type mathMaxOrMinCallParser struct {
	function string
}

func (p *mathMaxOrMinCallParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) starlarkExpr {
	parsedArgs, err := parseIntegerArguments(ctx, node, args, 2)
	if err != nil {
		return ctx.newBadExpr(node, err.Error())
	}
	return &callExpr{
		object:     nil,
		name:       p.function,
		args:       parsedArgs,
		returnType: starlarkTypeInt,
	}
}

type evalNodeParser struct{}

func (p *evalNodeParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) []starlarkNode {
	parser := mkparser.NewParser("Eval expression", strings.NewReader(args.Dump()))
	nodes, errs := parser.Parse()
	if errs != nil {
		return []starlarkNode{ctx.newBadNode(node, "Unable to parse eval statement")}
	}

	if len(nodes) == 0 {
		return []starlarkNode{}
	} else if len(nodes) == 1 {
		// Replace the nodeLocator with one that just returns the location of
		// the $(eval) node. Otherwise, statements inside an $(eval) will show as
		// being on line 1 of the file, because they're on line 1 of
		// strings.NewReader(args.Dump())
		oldNodeLocator := ctx.script.nodeLocator
		ctx.script.nodeLocator = func(pos mkparser.Pos) int {
			return oldNodeLocator(node.Pos())
		}
		defer func() {
			ctx.script.nodeLocator = oldNodeLocator
		}()

		switch n := nodes[0].(type) {
		case *mkparser.Assignment:
			if n.Name.Const() {
				return ctx.handleAssignment(n)
			}
		case *mkparser.Comment:
			return []starlarkNode{&commentNode{strings.TrimSpace("#" + n.Comment)}}
		case *mkparser.Directive:
			if n.Name == "include" || n.Name == "-include" {
				return ctx.handleInclude(n)
			}
		case *mkparser.Variable:
			// Technically inherit-product(-if-exists) don't need to be put inside
			// an eval, but some makefiles do it, presumably because they copy+pasted
			// from a $(eval include ...)
			if name, _, ok := ctx.maybeParseFunctionCall(n, n.Name); ok {
				if name == "inherit-product" || name == "inherit-product-if-exists" {
					return ctx.handleVariable(n)
				}
			}
		}
	}

	return []starlarkNode{ctx.newBadNode(node, "Eval expression too complex; only assignments, comments, includes, and inherit-products are supported")}
}

type lowerUpperParser struct {
	isUpper bool
}

func (p *lowerUpperParser) parse(ctx *parseContext, node mkparser.Node, args *mkparser.MakeString) starlarkExpr {
	fn := "lower"
	if p.isUpper {
		fn = "upper"
	}
	arg := ctx.parseMakeString(node, args)

	return &callExpr{
		object:     arg,
		name:       fn,
		returnType: starlarkTypeString,
	}
}

func (ctx *parseContext) parseMakeString(node mkparser.Node, mk *mkparser.MakeString) starlarkExpr {
	if mk.Const() {
		return &stringLiteralExpr{mk.Dump()}
	}
	if mkRef, ok := mk.SingleVariable(); ok {
		return ctx.parseReference(node, mkRef)
	}
	// If we reached here, it's neither string literal nor a simple variable,
	// we need a full-blown interpolation node that will generate
	// "a%b%c" % (X, Y) for a$(X)b$(Y)c
	parts := make([]starlarkExpr, len(mk.Variables)+len(mk.Strings))
	for i := 0; i < len(parts); i++ {
		if i%2 == 0 {
			parts[i] = &stringLiteralExpr{literal: mk.Strings[i/2]}
		} else {
			parts[i] = ctx.parseReference(node, mk.Variables[i/2].Name)
			if x, ok := parts[i].(*badExpr); ok {
				return x
			}
		}
	}
	return NewInterpolateExpr(parts)
}

func (ctx *parseContext) parseNodeMakeString(node mkparser.Node, mk *mkparser.MakeString) []starlarkNode {
	// Discard any constant values in the make string, as they would be top level
	// string literals and do nothing.
	result := make([]starlarkNode, 0, len(mk.Variables))
	for i := range mk.Variables {
		result = append(result, ctx.handleVariable(&mk.Variables[i])...)
	}
	return result
}

// Handles the statements whose treatment is the same in all contexts: comment,
// assignment, variable (which is a macro call in reality) and all constructs that
// do not handle in any context ('define directive and any unrecognized stuff).
func (ctx *parseContext) handleSimpleStatement(node mkparser.Node) []starlarkNode {
	var result []starlarkNode
	switch x := node.(type) {
	case *mkparser.Comment:
		if n, handled := ctx.maybeHandleAnnotation(x); handled && n != nil {
			result = []starlarkNode{n}
		} else if !handled {
			result = []starlarkNode{&commentNode{strings.TrimSpace("#" + x.Comment)}}
		}
	case *mkparser.Assignment:
		result = ctx.handleAssignment(x)
	case *mkparser.Variable:
		result = ctx.handleVariable(x)
	case *mkparser.Directive:
		switch x.Name {
		case "define":
			if res := ctx.maybeHandleDefine(x); res != nil {
				result = []starlarkNode{res}
			}
		case "include", "-include":
			result = ctx.handleInclude(x)
		case "ifeq", "ifneq", "ifdef", "ifndef":
			result = []starlarkNode{ctx.handleIfBlock(x)}
		default:
			result = []starlarkNode{ctx.newBadNode(x, "unexpected directive %s", x.Name)}
		}
	default:
		result = []starlarkNode{ctx.newBadNode(x, "unsupported line %s", strings.ReplaceAll(x.Dump(), "\n", "\n#"))}
	}

	// Clear the includeTops after each non-comment statement
	// so that include annotations placed on certain statements don't apply
	// globally for the rest of the makefile was well.
	if _, wasComment := node.(*mkparser.Comment); !wasComment {
		ctx.atTopOfMakefile = false
		ctx.includeTops = []string{}
	}

	if result == nil {
		result = []starlarkNode{}
	}

	return result
}

// The types allowed in a type_hint
var typeHintMap = map[string]starlarkType{
	"string": starlarkTypeString,
	"list":   starlarkTypeList,
}

// Processes annotation. An annotation is a comment that starts with #RBC# and provides
// a conversion hint -- say, where to look for the dynamically calculated inherit/include
// paths. Returns true if the comment was a successfully-handled annotation.
func (ctx *parseContext) maybeHandleAnnotation(cnode *mkparser.Comment) (starlarkNode, bool) {
	maybeTrim := func(s, prefix string) (string, bool) {
		if strings.HasPrefix(s, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(s, prefix)), true
		}
		return s, false
	}
	annotation, ok := maybeTrim(cnode.Comment, annotationCommentPrefix)
	if !ok {
		return nil, false
	}
	if p, ok := maybeTrim(annotation, "include_top"); ok {
		// Don't allow duplicate include tops, because then we will generate
		// invalid starlark code. (duplicate keys in the _entry dictionary)
		for _, top := range ctx.includeTops {
			if top == p {
				return nil, true
			}
		}
		ctx.includeTops = append(ctx.includeTops, p)
		return nil, true
	} else if p, ok := maybeTrim(annotation, "type_hint"); ok {
		// Type hints must come at the beginning the file, to avoid confusion
		// if a type hint was specified later and thus only takes effect for half
		// of the file.
		if !ctx.atTopOfMakefile {
			return ctx.newBadNode(cnode, "type_hint annotations must come before the first Makefile statement"), true
		}

		parts := strings.Fields(p)
		if len(parts) <= 1 {
			return ctx.newBadNode(cnode, "Invalid type_hint annotation: %s. Must be a variable type followed by a list of variables of that type", p), true
		}

		var varType starlarkType
		if varType, ok = typeHintMap[parts[0]]; !ok {
			varType = starlarkTypeUnknown
		}
		if varType == starlarkTypeUnknown {
			return ctx.newBadNode(cnode, "Invalid type_hint annotation. Only list/string types are accepted, found %s", parts[0]), true
		}

		for _, name := range parts[1:] {
			// Don't allow duplicate type hints
			if _, ok := ctx.typeHints[name]; ok {
				return ctx.newBadNode(cnode, "Duplicate type hint for variable %s", name), true
			}
			ctx.typeHints[name] = varType
		}
		return nil, true
	}
	return ctx.newBadNode(cnode, "unsupported annotation %s", cnode.Comment), true
}

func (ctx *parseContext) loadedModulePath(path string) string {
	// During the transition to Roboleaf some of the product configuration files
	// will be converted and checked in while the others will be generated on the fly
	// and run. The runner  (rbcrun application) accommodates this by allowing three
	// different ways to specify the loaded file location:
	//  1) load(":<file>",...) loads <file> from the same directory
	//  2) load("//path/relative/to/source/root:<file>", ...) loads <file> source tree
	//  3) load("/absolute/path/to/<file> absolute path
	// If the file being generated and the file it wants to load are in the same directory,
	// generate option 1.
	// Otherwise, if output directory is not specified, generate 2)
	// Finally, if output directory has been specified and the file being generated and
	// the file it wants to load from are in the different directories, generate 2) or 3):
	//  * if the file being loaded exists in the source tree, generate 2)
	//  * otherwise, generate 3)
	// Finally, figure out the loaded module path and name and create a node for it
	loadedModuleDir := filepath.Dir(path)
	base := filepath.Base(path)
	loadedModuleName := strings.TrimSuffix(base, filepath.Ext(base)) + ctx.outputSuffix
	if loadedModuleDir == filepath.Dir(ctx.script.mkFile) {
		return ":" + loadedModuleName
	}
	if ctx.outputDir == "" {
		return fmt.Sprintf("//%s:%s", loadedModuleDir, loadedModuleName)
	}
	if _, err := os.Stat(filepath.Join(loadedModuleDir, loadedModuleName)); err == nil {
		return fmt.Sprintf("//%s:%s", loadedModuleDir, loadedModuleName)
	}
	return filepath.Join(ctx.outputDir, loadedModuleDir, loadedModuleName)
}

func (ctx *parseContext) addSoongNamespace(ns string) {
	if _, ok := ctx.soongNamespaces[ns]; ok {
		return
	}
	ctx.soongNamespaces[ns] = make(map[string]bool)
}

func (ctx *parseContext) hasSoongNamespace(name string) bool {
	_, ok := ctx.soongNamespaces[name]
	return ok
}

func (ctx *parseContext) updateSoongNamespace(replace bool, namespaceName string, varNames []string) {
	ctx.addSoongNamespace(namespaceName)
	vars := ctx.soongNamespaces[namespaceName]
	if replace {
		vars = make(map[string]bool)
		ctx.soongNamespaces[namespaceName] = vars
	}
	for _, v := range varNames {
		vars[v] = true
	}
}

func (ctx *parseContext) hasNamespaceVar(namespaceName string, varName string) bool {
	vars, ok := ctx.soongNamespaces[namespaceName]
	if ok {
		_, ok = vars[varName]
	}
	return ok
}

func (ctx *parseContext) errorLocation(node mkparser.Node) ErrorLocation {
	return ErrorLocation{ctx.script.mkFile, ctx.script.nodeLocator(node.Pos())}
}

func (ss *StarlarkScript) String() string {
	return NewGenerateContext(ss).emit()
}

func (ss *StarlarkScript) SubConfigFiles() []string {

	var subs []string
	for _, src := range ss.inherited {
		subs = append(subs, src.originalPath)
	}
	return subs
}

func (ss *StarlarkScript) HasErrors() bool {
	return ss.hasErrors
}

// Convert reads and parses a makefile. If successful, parsed tree
// is returned and then can be passed to String() to get the generated
// Starlark file.
func Convert(req Request) (*StarlarkScript, error) {
	reader := req.Reader
	if reader == nil {
		mkContents, err := ioutil.ReadFile(req.MkFile)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewBuffer(mkContents)
	}
	parser := mkparser.NewParser(req.MkFile, reader)
	nodes, errs := parser.Parse()
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "ERROR:", e)
		}
		return nil, fmt.Errorf("bad makefile %s", req.MkFile)
	}
	starScript := &StarlarkScript{
		moduleName:     moduleNameForFile(req.MkFile),
		mkFile:         req.MkFile,
		traceCalls:     req.TraceCalls,
		sourceFS:       req.SourceFS,
		makefileFinder: req.MakefileFinder,
		nodeLocator:    func(pos mkparser.Pos) int { return parser.Unpack(pos).Line },
		nodes:          make([]starlarkNode, 0),
	}
	ctx := newParseContext(starScript, nodes)
	ctx.outputSuffix = req.OutputSuffix
	ctx.outputDir = req.OutputDir
	ctx.errorLogger = req.ErrorLogger
	if len(req.TracedVariables) > 0 {
		ctx.tracedVariables = make(map[string]bool)
		for _, v := range req.TracedVariables {
			ctx.tracedVariables[v] = true
		}
	}
	for ctx.hasNodes() && ctx.fatalError == nil {
		starScript.nodes = append(starScript.nodes, ctx.handleSimpleStatement(ctx.getNode())...)
	}
	if ctx.fatalError != nil {
		return nil, ctx.fatalError
	}
	return starScript, nil
}

func Launcher(mainModuleUri, inputVariablesUri, mainModuleName string) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "load(%q, %q)\n", baseUri, baseName)
	fmt.Fprintf(&buf, "load(%q, input_variables_init = \"init\")\n", inputVariablesUri)
	fmt.Fprintf(&buf, "load(%q, \"init\")\n", mainModuleUri)
	fmt.Fprintf(&buf, "%s(%s(%q, init, input_variables_init))\n", cfnPrintVars, cfnMain, mainModuleName)
	return buf.String()
}

func BoardLauncher(mainModuleUri string, inputVariablesUri string) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "load(%q, %q)\n", baseUri, baseName)
	fmt.Fprintf(&buf, "load(%q, \"init\")\n", mainModuleUri)
	fmt.Fprintf(&buf, "load(%q, input_variables_init = \"init\")\n", inputVariablesUri)
	fmt.Fprintf(&buf, "%s(%s(init, input_variables_init))\n", cfnPrintVars, cfnBoardMain)
	return buf.String()
}

func MakePath2ModuleName(mkPath string) string {
	return strings.TrimSuffix(mkPath, filepath.Ext(mkPath))
}
