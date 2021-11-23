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
//   * comments
//   * simple variable assignments
//   * $(call init-product,<file>)
//   * $(call inherit-product-if-exists
//   * if directives
// All other constructs are carried over to the output starlark file as comments.
//
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
	cfnGetCfg          = baseName + ".cfg"
	cfnMain            = baseName + ".product_configuration"
	cfnBoardMain       = baseName + ".board_configuration"
	cfnPrintVars       = baseName + ".printvars"
	cfnPrintGlobals    = baseName + ".printglobals"
	cfnWarning         = baseName + ".warning"
	cfnLocalAppend     = baseName + ".local_append"
	cfnLocalSetDefault = baseName + ".local_set_default"
	cfnInherit         = baseName + ".inherit"
	cfnSetListDefault  = baseName + ".setdefault"
)

const (
	// Phony makefile functions, they are eventually rewritten
	// according to knownFunctions map
	fileExistsPhony = "$file_exists"
	// The following two macros are obsolete, and will we deleted once
	// there are deleted from the makefiles:
	soongConfigNamespaceOld = "add_soong_config_namespace"
	soongConfigVarSetOld    = "add_soong_config_var_value"
	soongConfigAppend       = "soong_config_append"
	soongConfigAssign       = "soong_config_set"
	soongConfigGet          = "soong_config_get"
	wildcardExistsPhony     = "$wildcard_exists"
)

const (
	callLoadAlways = "inherit-product"
	callLoadIf     = "inherit-product-if-exists"
)

var knownFunctions = map[string]struct {
	// The name of the runtime function this function call in makefiles maps to.
	// If it starts with !, then this makefile function call is rewritten to
	// something else.
	runtimeName string
	returnType  starlarkType
	hiddenArg   hiddenArgType
}{
	"abspath":                             {baseName + ".abspath", starlarkTypeString, hiddenArgNone},
	fileExistsPhony:                       {baseName + ".file_exists", starlarkTypeBool, hiddenArgNone},
	wildcardExistsPhony:                   {baseName + ".file_wildcard_exists", starlarkTypeBool, hiddenArgNone},
	soongConfigNamespaceOld:               {baseName + ".soong_config_namespace", starlarkTypeVoid, hiddenArgGlobal},
	soongConfigVarSetOld:                  {baseName + ".soong_config_set", starlarkTypeVoid, hiddenArgGlobal},
	soongConfigAssign:                     {baseName + ".soong_config_set", starlarkTypeVoid, hiddenArgGlobal},
	soongConfigAppend:                     {baseName + ".soong_config_append", starlarkTypeVoid, hiddenArgGlobal},
	soongConfigGet:                        {baseName + ".soong_config_get", starlarkTypeString, hiddenArgGlobal},
	"add-to-product-copy-files-if-exists": {baseName + ".copy_if_exists", starlarkTypeList, hiddenArgNone},
	"addprefix":                           {baseName + ".addprefix", starlarkTypeList, hiddenArgNone},
	"addsuffix":                           {baseName + ".addsuffix", starlarkTypeList, hiddenArgNone},
	"copy-files":                          {baseName + ".copy_files", starlarkTypeList, hiddenArgNone},
	"dir":                                 {baseName + ".dir", starlarkTypeList, hiddenArgNone},
	"dist-for-goals":                      {baseName + ".mkdist_for_goals", starlarkTypeVoid, hiddenArgGlobal},
	"enforce-product-packages-exist":      {baseName + ".enforce_product_packages_exist", starlarkTypeVoid, hiddenArgNone},
	"error":                               {baseName + ".mkerror", starlarkTypeVoid, hiddenArgNone},
	"findstring":                          {"!findstring", starlarkTypeInt, hiddenArgNone},
	"find-copy-subdir-files":              {baseName + ".find_and_copy", starlarkTypeList, hiddenArgNone},
	"find-word-in-list":                   {"!find-word-in-list", starlarkTypeUnknown, hiddenArgNone}, // internal macro
	"filter":                              {baseName + ".filter", starlarkTypeList, hiddenArgNone},
	"filter-out":                          {baseName + ".filter_out", starlarkTypeList, hiddenArgNone},
	"firstword":                           {"!firstword", starlarkTypeString, hiddenArgNone},
	"get-vendor-board-platforms":          {"!get-vendor-board-platforms", starlarkTypeList, hiddenArgNone}, // internal macro, used by is-board-platform, etc.
	"info":                                {baseName + ".mkinfo", starlarkTypeVoid, hiddenArgNone},
	"is-android-codename":                 {"!is-android-codename", starlarkTypeBool, hiddenArgNone},         // unused by product config
	"is-android-codename-in-list":         {"!is-android-codename-in-list", starlarkTypeBool, hiddenArgNone}, // unused by product config
	"is-board-platform":                   {"!is-board-platform", starlarkTypeBool, hiddenArgNone},
	"is-board-platform2":                  {baseName + ".board_platform_is", starlarkTypeBool, hiddenArgGlobal},
	"is-board-platform-in-list":           {"!is-board-platform-in-list", starlarkTypeBool, hiddenArgNone},
	"is-board-platform-in-list2":          {baseName + ".board_platform_in", starlarkTypeBool, hiddenArgGlobal},
	"is-chipset-in-board-platform":        {"!is-chipset-in-board-platform", starlarkTypeUnknown, hiddenArgNone},     // unused by product config
	"is-chipset-prefix-in-board-platform": {"!is-chipset-prefix-in-board-platform", starlarkTypeBool, hiddenArgNone}, // unused by product config
	"is-not-board-platform":               {"!is-not-board-platform", starlarkTypeBool, hiddenArgNone},               // defined but never used
	"is-platform-sdk-version-at-least":    {"!is-platform-sdk-version-at-least", starlarkTypeBool, hiddenArgNone},    // unused by product config
	"is-product-in-list":                  {"!is-product-in-list", starlarkTypeBool, hiddenArgNone},
	"is-vendor-board-platform":            {"!is-vendor-board-platform", starlarkTypeBool, hiddenArgNone},
	"is-vendor-board-qcom":                {"!is-vendor-board-qcom", starlarkTypeBool, hiddenArgNone},
	callLoadAlways:                        {"!inherit-product", starlarkTypeVoid, hiddenArgNone},
	callLoadIf:                            {"!inherit-product-if-exists", starlarkTypeVoid, hiddenArgNone},
	"lastword":                            {"!lastword", starlarkTypeString, hiddenArgNone},
	"match-prefix":                        {"!match-prefix", starlarkTypeUnknown, hiddenArgNone},       // internal macro
	"match-word":                          {"!match-word", starlarkTypeUnknown, hiddenArgNone},         // internal macro
	"match-word-in-list":                  {"!match-word-in-list", starlarkTypeUnknown, hiddenArgNone}, // internal macro
	"notdir":                              {baseName + ".notdir", starlarkTypeString, hiddenArgNone},
	"my-dir":                              {"!my-dir", starlarkTypeString, hiddenArgNone},
	"patsubst":                            {baseName + ".mkpatsubst", starlarkTypeString, hiddenArgNone},
	"product-copy-files-by-pattern":       {baseName + ".product_copy_files_by_pattern", starlarkTypeList, hiddenArgNone},
	"require-artifacts-in-path":           {baseName + ".require_artifacts_in_path", starlarkTypeVoid, hiddenArgNone},
	"require-artifacts-in-path-relaxed":   {baseName + ".require_artifacts_in_path_relaxed", starlarkTypeVoid, hiddenArgNone},
	// TODO(asmundak): remove it once all calls are removed from configuration makefiles. see b/183161002
	"shell":      {baseName + ".shell", starlarkTypeString, hiddenArgNone},
	"strip":      {baseName + ".mkstrip", starlarkTypeString, hiddenArgNone},
	"tb-modules": {"!tb-modules", starlarkTypeUnknown, hiddenArgNone}, // defined in hardware/amlogic/tb_modules/tb_detect.mk, unused
	"subst":      {baseName + ".mksubst", starlarkTypeString, hiddenArgNone},
	"warning":    {baseName + ".mkwarning", starlarkTypeVoid, hiddenArgNone},
	"word":       {baseName + "!word", starlarkTypeString, hiddenArgNone},
	"wildcard":   {baseName + ".expand_wildcard", starlarkTypeList, hiddenArgNone},
}

var builtinFuncRex = regexp.MustCompile(
	"^(addprefix|addsuffix|abspath|and|basename|call|dir|error|eval" +
		"|flavor|foreach|file|filter|filter-out|findstring|firstword|guile" +
		"|if|info|join|lastword|notdir|or|origin|patsubst|realpath" +
		"|shell|sort|strip|subst|suffix|value|warning|word|wordlist|words" +
		"|wildcard)")

// Conversion request parameters
type Request struct {
	MkFile          string    // file to convert
	Reader          io.Reader // if set, read input from this stream instead
	RootDir         string    // root directory path used to resolve included files
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

// Starlark output generation context
type generationContext struct {
	buf          strings.Builder
	starScript   *StarlarkScript
	indentLevel  int
	inAssignment bool
	tracedCount  int
}

func NewGenerateContext(ss *StarlarkScript) *generationContext {
	return &generationContext{starScript: ss}
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
	for _, sc := range gctx.starScript.inherited {
		uri := sc.path
		if m, ok := loadedSubConfigs[uri]; ok {
			// No need to emit load statement, but fix module name.
			sc.moduleLocalName = m
			continue
		}
		if sc.optional {
			uri += "|init"
		}
		gctx.newLine()
		gctx.writef("load(%q, %s = \"init\")", uri, sc.entryName())
		loadedSubConfigs[uri] = sc.moduleLocalName
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

type nodeReceiver interface {
	newNode(node starlarkNode)
}

// Information about the generated Starlark script.
type StarlarkScript struct {
	mkFile         string
	moduleName     string
	mkPos          scanner.Position
	nodes          []starlarkNode
	inherited      []*moduleInfo
	hasErrors      bool
	topDir         string
	traceCalls     bool // print enter/exit each init function
	sourceFS       fs.FS
	makefileFinder MakefileFinder
	nodeLocator    func(pos mkparser.Pos) int
}

func (ss *StarlarkScript) newNode(node starlarkNode) {
	ss.nodes = append(ss.nodes, node)
}

// varAssignmentScope points to the last assignment for each variable
// in the current block. It is used during the parsing to chain
// the assignments to a variable together.
type varAssignmentScope struct {
	outer *varAssignmentScope
	vars  map[string]*assignmentNode
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
	builtinMakeVars  map[string]starlarkExpr
	outputSuffix     string
	errorLogger      ErrorLogger
	tracedVariables  map[string]bool // variables to be traced in the generated script
	variables        map[string]variable
	varAssignments   *varAssignmentScope
	receiver         nodeReceiver // receptacle for the generated starlarkNode's
	receiverStack    []nodeReceiver
	outputDir        string
	dependentModules map[string]*moduleInfo
	soongNamespaces  map[string]map[string]bool
	includeTops      []string
}

func newParseContext(ss *StarlarkScript, nodes []mkparser.Node) *parseContext {
	topdir, _ := filepath.Split(filepath.Join(ss.topDir, "foo"))
	predefined := []struct{ name, value string }{
		{"SRC_TARGET_DIR", filepath.Join("build", "make", "target")},
		{"LOCAL_PATH", filepath.Dir(ss.mkFile)},
		{"TOPDIR", topdir},
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
		//    BOARD_CONFIG_VENDOR_PATH
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
		builtinMakeVars:  map[string]starlarkExpr{},
		variables:        make(map[string]variable),
		dependentModules: make(map[string]*moduleInfo),
		soongNamespaces:  make(map[string]map[string]bool),
		includeTops:      []string{"vendor/google-devices"},
	}
	ctx.pushVarAssignments()
	for _, item := range predefined {
		ctx.variables[item.name] = &predefinedVariable{
			baseVariable: baseVariable{nam: item.name, typ: starlarkTypeString},
			value:        &stringLiteralExpr{item.value},
		}
	}

	return ctx
}

func (ctx *parseContext) lastAssignment(name string) *assignmentNode {
	for va := ctx.varAssignments; va != nil; va = va.outer {
		if v, ok := va.vars[name]; ok {
			return v
		}
	}
	return nil
}

func (ctx *parseContext) setLastAssignment(name string, asgn *assignmentNode) {
	ctx.varAssignments.vars[name] = asgn
}

func (ctx *parseContext) pushVarAssignments() {
	va := &varAssignmentScope{
		outer: ctx.varAssignments,
		vars:  make(map[string]*assignmentNode),
	}
	ctx.varAssignments = va
}

func (ctx *parseContext) popVarAssignments() {
	ctx.varAssignments = ctx.varAssignments.outer
}

func (ctx *parseContext) pushReceiver(rcv nodeReceiver) {
	ctx.receiverStack = append(ctx.receiverStack, ctx.receiver)
	ctx.receiver = rcv
}

func (ctx *parseContext) popReceiver() {
	last := len(ctx.receiverStack) - 1
	if last < 0 {
		panic(fmt.Errorf("popReceiver: receiver stack empty"))
	}
	ctx.receiver = ctx.receiverStack[last]
	ctx.receiverStack = ctx.receiverStack[0:last]
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

func (ctx *parseContext) handleAssignment(a *mkparser.Assignment) {
	// Handle only simple variables
	if !a.Name.Const() {
		ctx.errorf(a, "Only simple variables are handled")
		return
	}
	name := a.Name.Strings[0]
	// The `override` directive
	//      override FOO :=
	// is parsed as an assignment to a variable named `override FOO`.
	// There are very few places where `override` is used, just flag it.
	if strings.HasPrefix(name, "override ") {
		ctx.errorf(a, "cannot handle override directive")
	}

	// Soong configuration
	if strings.HasPrefix(name, soongNsPrefix) {
		ctx.handleSoongNsAssignment(strings.TrimPrefix(name, soongNsPrefix), a)
		return
	}
	lhs := ctx.addVariable(name)
	if lhs == nil {
		ctx.errorf(a, "unknown variable %s", name)
		return
	}
	_, isTraced := ctx.tracedVariables[name]
	asgn := &assignmentNode{lhs: lhs, mkValue: a.Value, isTraced: isTraced, location: ctx.errorLocation(a)}
	if lhs.valueType() == starlarkTypeUnknown {
		// Try to divine variable type from the RHS
		asgn.value = ctx.parseMakeString(a, a.Value)
		if xBad, ok := asgn.value.(*badExpr); ok {
			ctx.wrapBadExpr(xBad)
			return
		}
		inferred_type := asgn.value.typ()
		if inferred_type != starlarkTypeUnknown {
			lhs.setValueType(inferred_type)
		}
	}
	if lhs.valueType() == starlarkTypeList {
		xConcat := ctx.buildConcatExpr(a)
		if xConcat == nil {
			return
		}
		switch len(xConcat.items) {
		case 0:
			asgn.value = &listExpr{}
		case 1:
			asgn.value = xConcat.items[0]
		default:
			asgn.value = xConcat
		}
	} else {
		asgn.value = ctx.parseMakeString(a, a.Value)
		if xBad, ok := asgn.value.(*badExpr); ok {
			ctx.wrapBadExpr(xBad)
			return
		}
	}

	// TODO(asmundak): move evaluation to a separate pass
	asgn.value, _ = asgn.value.eval(ctx.builtinMakeVars)

	asgn.previous = ctx.lastAssignment(name)
	ctx.setLastAssignment(name, asgn)
	switch a.Type {
	case "=", ":=":
		asgn.flavor = asgnSet
	case "+=":
		if asgn.previous == nil && !asgn.lhs.isPreset() {
			asgn.flavor = asgnMaybeAppend
		} else {
			asgn.flavor = asgnAppend
		}
	case "?=":
		asgn.flavor = asgnMaybeSet
	default:
		panic(fmt.Errorf("unexpected assignment type %s", a.Type))
	}

	ctx.receiver.newNode(asgn)
}

func (ctx *parseContext) handleSoongNsAssignment(name string, asgn *mkparser.Assignment) {
	val := ctx.parseMakeString(asgn, asgn.Value)
	if xBad, ok := val.(*badExpr); ok {
		ctx.wrapBadExpr(xBad)
		return
	}
	val, _ = val.eval(ctx.builtinMakeVars)

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
			ctx.errorf(asgn, "cannot handle variables in SOONG_CONFIG_NAMESPACES assignment, please use add_soong_config_namespace instead")
			return
		}
		for _, ns := range strings.Fields(s) {
			ctx.addSoongNamespace(ns)
			ctx.receiver.newNode(&exprNode{&callExpr{
				name:       soongConfigNamespaceOld,
				args:       []starlarkExpr{&stringLiteralExpr{ns}},
				returnType: starlarkTypeVoid,
			}})
		}
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
				ctx.errorf(asgn, "ambiguous soong namespace (may be either `%s` or  `%s`)", namespaceName, name[0:pos])
				return
			}
			namespaceName = name[0:pos]
			varName = name[pos+1:]
		}
		if namespaceName == "" {
			ctx.errorf(asgn, "cannot figure out Soong namespace, please use add_soong_config_var_value macro instead")
			return
		}
		if varName == "" {
			// Remember variables in this namespace
			s, ok := maybeString(val)
			if !ok {
				ctx.errorf(asgn, "cannot handle variables in SOONG_CONFIG_ assignment, please use add_soong_config_var_value instead")
				return
			}
			ctx.updateSoongNamespace(asgn.Type != "+=", namespaceName, strings.Fields(s))
			return
		}

		// Finally, handle assignment to a namespace variable
		if !ctx.hasNamespaceVar(namespaceName, varName) {
			ctx.errorf(asgn, "no %s variable in %s namespace, please use add_soong_config_var_value instead", varName, namespaceName)
			return
		}
		fname := soongConfigAssign
		if asgn.Type == "+=" {
			fname = soongConfigAppend
		}
		ctx.receiver.newNode(&exprNode{&callExpr{
			name:       fname,
			args:       []starlarkExpr{&stringLiteralExpr{namespaceName}, &stringLiteralExpr{varName}, val},
			returnType: starlarkTypeVoid,
		}})
	}
}

func (ctx *parseContext) buildConcatExpr(a *mkparser.Assignment) *concatExpr {
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
			ctx.wrapBadExpr(x)
			return nil
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
	return xConcat
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
	mi := &moduleInfo{
		path:            modulePath,
		originalPath:    path,
		moduleLocalName: moduleLocalName,
		optional:        optional,
	}
	ctx.dependentModules[modulePath] = mi
	ctx.script.inherited = append(ctx.script.inherited, mi)
	return mi
}

func (ctx *parseContext) handleSubConfig(
	v mkparser.Node, pathExpr starlarkExpr, loadAlways bool, processModule func(inheritedModule)) {
	pathExpr, _ = pathExpr.eval(ctx.builtinMakeVars)

	// In a simple case, the name of a module to inherit/include is known statically.
	if path, ok := maybeString(pathExpr); ok {
		// Note that even if this directive loads a module unconditionally, a module may be
		// absent without causing any harm if this directive is inside an if/else block.
		moduleShouldExist := loadAlways && ctx.ifNestLevel == 0
		if strings.Contains(path, "*") {
			if paths, err := fs.Glob(ctx.script.sourceFS, path); err == nil {
				for _, p := range paths {
					mi := ctx.newDependentModule(p, !moduleShouldExist)
					processModule(inheritedStaticModule{mi, loadAlways})
				}
			} else {
				ctx.errorf(v, "cannot glob wildcard argument")
			}
		} else {
			mi := ctx.newDependentModule(path, !moduleShouldExist)
			processModule(inheritedStaticModule{mi, loadAlways})
		}
		return
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
	varPath, ok := pathExpr.(*interpolateExpr)
	if !ok {
		ctx.errorf(v, "inherit-product/include argument is too complex")
		return
	}

	pathPattern := []string{varPath.chunks[0]}
	for _, chunk := range varPath.chunks[1:] {
		if chunk != "" {
			pathPattern = append(pathPattern, chunk)
		}
	}
	if pathPattern[0] == "" {
		// If pattern starts from the top. restrict it to the directories where
		// we know inherit-product uses dynamically calculated path.
		for _, p := range ctx.includeTops {
			pathPattern[0] = p
			matchingPaths = append(matchingPaths, ctx.findMatchingPaths(pathPattern)...)
		}
	} else {
		matchingPaths = ctx.findMatchingPaths(pathPattern)
	}
	// Safeguard against $(call inherit-product,$(PRODUCT_PATH))
	const maxMatchingFiles = 150
	if len(matchingPaths) > maxMatchingFiles {
		ctx.errorf(v, "there are >%d files matching the pattern, please rewrite it", maxMatchingFiles)
		return
	}
	res := inheritedDynamicModule{*varPath, []*moduleInfo{}, loadAlways}
	for _, p := range matchingPaths {
		// A product configuration files discovered dynamically may attempt to inherit
		// from another one which does not exist in this source tree. Prevent load errors
		// by always loading the dynamic files as optional.
		res.candidateModules = append(res.candidateModules, ctx.newDependentModule(p, true))
	}
	processModule(res)
}

func (ctx *parseContext) findMatchingPaths(pattern []string) []string {
	files := ctx.script.makefileFinder.Find(ctx.script.topDir)
	if len(pattern) == 0 {
		return files
	}

	// Create regular expression from the pattern
	s_regexp := "^" + regexp.QuoteMeta(pattern[0])
	for _, s := range pattern[1:] {
		s_regexp += ".*" + regexp.QuoteMeta(s)
	}
	s_regexp += "$"
	rex := regexp.MustCompile(s_regexp)

	// Now match
	var res []string
	for _, p := range files {
		if rex.MatchString(p) {
			res = append(res, p)
		}
	}
	return res
}

func (ctx *parseContext) handleInheritModule(v mkparser.Node, pathExpr starlarkExpr, loadAlways bool) {
	ctx.handleSubConfig(v, pathExpr, loadAlways, func(im inheritedModule) {
		ctx.receiver.newNode(&inheritNode{im, loadAlways})
	})
}

func (ctx *parseContext) handleInclude(v mkparser.Node, pathExpr starlarkExpr, loadAlways bool) {
	ctx.handleSubConfig(v, pathExpr, loadAlways, func(im inheritedModule) {
		ctx.receiver.newNode(&includeNode{im, loadAlways})
	})
}

func (ctx *parseContext) handleVariable(v *mkparser.Variable) {
	// Handle:
	//   $(call inherit-product,...)
	//   $(call inherit-product-if-exists,...)
	//   $(info xxx)
	//   $(warning xxx)
	//   $(error xxx)
	expr := ctx.parseReference(v, v.Name)
	switch x := expr.(type) {
	case *callExpr:
		if x.name == callLoadAlways || x.name == callLoadIf {
			ctx.handleInheritModule(v, x.args[0], x.name == callLoadAlways)
		} else if isMakeControlFunc(x.name) {
			// File name is the first argument
			args := []starlarkExpr{
				&stringLiteralExpr{ctx.script.mkFile},
				x.args[0],
			}
			ctx.receiver.newNode(&exprNode{
				&callExpr{name: x.name, args: args, returnType: starlarkTypeUnknown},
			})
		} else {
			ctx.receiver.newNode(&exprNode{expr})
		}
	case *badExpr:
		ctx.wrapBadExpr(x)
		return
	default:
		ctx.errorf(v, "cannot handle %s", v.Dump())
		return
	}
}

func (ctx *parseContext) handleDefine(directive *mkparser.Directive) {
	macro_name := strings.Fields(directive.Args.Strings[0])[0]
	// Ignore the macros that we handle
	if _, ok := knownFunctions[macro_name]; !ok {
		ctx.errorf(directive, "define is not supported: %s", macro_name)
	}
}

func (ctx *parseContext) handleIfBlock(ifDirective *mkparser.Directive) {
	ssSwitch := &switchNode{}
	ctx.pushReceiver(ssSwitch)
	for ctx.processBranch(ifDirective); ctx.hasNodes() && ctx.fatalError == nil; {
		node := ctx.getNode()
		switch x := node.(type) {
		case *mkparser.Directive:
			switch x.Name {
			case "else", "elifdef", "elifndef", "elifeq", "elifneq":
				ctx.processBranch(x)
			case "endif":
				ctx.popReceiver()
				ctx.receiver.newNode(ssSwitch)
				return
			default:
				ctx.errorf(node, "unexpected directive %s", x.Name)
			}
		default:
			ctx.errorf(ifDirective, "unexpected statement")
		}
	}
	if ctx.fatalError == nil {
		ctx.fatalError = fmt.Errorf("no matching endif for %s", ifDirective.Dump())
	}
	ctx.popReceiver()
}

// processBranch processes a single branch (if/elseif/else) until the next directive
// on the same level.
func (ctx *parseContext) processBranch(check *mkparser.Directive) {
	block := switchCase{gate: ctx.parseCondition(check)}
	defer func() {
		ctx.popVarAssignments()
		ctx.ifNestLevel--

	}()
	ctx.pushVarAssignments()
	ctx.ifNestLevel++

	ctx.pushReceiver(&block)
	for ctx.hasNodes() {
		node := ctx.getNode()
		if d, ok := node.(*mkparser.Directive); ok {
			switch d.Name {
			case "else", "elifdef", "elifndef", "elifeq", "elifneq", "endif":
				ctx.popReceiver()
				ctx.receiver.newNode(&block)
				ctx.backNode()
				return
			}
		}
		ctx.handleSimpleStatement(node)
	}
	ctx.fatalError = fmt.Errorf("no matching endif for %s", check.Dump())
	ctx.popReceiver()
}

func (ctx *parseContext) newIfDefinedNode(check *mkparser.Directive) (starlarkExpr, bool) {
	if !check.Args.Const() {
		return ctx.newBadExpr(check, "ifdef variable ref too complex: %s", check.Args.Dump()), false
	}
	v := ctx.addVariable(check.Args.Strings[0])
	return &variableDefinedExpr{v}, true
}

func (ctx *parseContext) parseCondition(check *mkparser.Directive) starlarkNode {
	switch check.Name {
	case "ifdef", "ifndef", "elifdef", "elifndef":
		v, ok := ctx.newIfDefinedNode(check)
		if ok && strings.HasSuffix(check.Name, "ndef") {
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
	message := fmt.Sprintf(text, args...)
	if ctx.errorLogger != nil {
		ctx.errorLogger.NewError(ctx.errorLocation(node), node, text, args...)
	}
	ctx.script.hasErrors = true
	return &badExpr{errorLocation: ctx.errorLocation(node), message: message}
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

	return &eqExpr{left: xLeft, right: xRight, isEq: isEq}
}

// Given an if statement's directive and the left/right starlarkExprs,
// check if the starlarkExprs are one of a few hardcoded special cases
// that can be converted to a simpler equalify expression than simply comparing
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

	checkIsSomethingFunction := func(xCall *callExpr) starlarkExpr {
		s, ok := maybeString(value)
		if !ok || s != "true" {
			return ctx.newBadExpr(directive,
				fmt.Sprintf("the result of %s can be compared only to 'true'", xCall.name))
		}
		if len(xCall.args) < 1 {
			return ctx.newBadExpr(directive, "%s requires an argument", xCall.name)
		}
		return nil
	}

	switch call.name {
	case "filter":
		return ctx.parseCompareFilterFuncResult(directive, call, value, isEq), true
	case "filter-out":
		return ctx.parseCompareFilterFuncResult(directive, call, value, !isEq), true
	case "wildcard":
		return ctx.parseCompareWildcardFuncResult(directive, call, value, !isEq), true
	case "findstring":
		return ctx.parseCheckFindstringFuncResult(directive, call, value, !isEq), true
	case "strip":
		return ctx.parseCompareStripFuncResult(directive, call, value, !isEq), true
	case "is-board-platform":
		if xBad := checkIsSomethingFunction(call); xBad != nil {
			return xBad, true
		}
		return &eqExpr{
			left:  &variableRefExpr{ctx.addVariable("TARGET_BOARD_PLATFORM"), false},
			right: call.args[0],
			isEq:  isEq,
		}, true
	case "is-board-platform-in-list":
		if xBad := checkIsSomethingFunction(call); xBad != nil {
			return xBad, true
		}
		return &inExpr{
			expr:  &variableRefExpr{ctx.addVariable("TARGET_BOARD_PLATFORM"), false},
			list:  maybeConvertToStringList(call.args[0]),
			isNot: !isEq,
		}, true
	case "is-product-in-list":
		if xBad := checkIsSomethingFunction(call); xBad != nil {
			return xBad, true
		}
		return &inExpr{
			expr:  &variableRefExpr{ctx.addVariable("TARGET_PRODUCT"), true},
			list:  maybeConvertToStringList(call.args[0]),
			isNot: !isEq,
		}, true
	case "is-vendor-board-platform":
		if xBad := checkIsSomethingFunction(call); xBad != nil {
			return xBad, true
		}
		s, ok := maybeString(call.args[0])
		if !ok {
			return ctx.newBadExpr(directive, "cannot handle non-constant argument to is-vendor-board-platform"), true
		}
		return &inExpr{
			expr:  &variableRefExpr{ctx.addVariable("TARGET_BOARD_PLATFORM"), false},
			list:  &variableRefExpr{ctx.addVariable(s + "_BOARD_PLATFORMS"), true},
			isNot: !isEq,
		}, true

	case "is-board-platform2", "is-board-platform-in-list2":
		if s, ok := maybeString(value); !ok || s != "" {
			return ctx.newBadExpr(directive,
				fmt.Sprintf("the result of %s can be compared only to empty", call.name)), true
		}
		if len(call.args) != 1 {
			return ctx.newBadExpr(directive, "%s requires an argument", call.name), true
		}
		cc := &callExpr{
			name:       call.name,
			args:       []starlarkExpr{call.args[0]},
			returnType: starlarkTypeBool,
		}
		if isEq {
			return &notExpr{cc}, true
		}
		return cc, true
	case "is-vendor-board-qcom":
		if s, ok := maybeString(value); !ok || s != "" {
			return ctx.newBadExpr(directive,
				fmt.Sprintf("the result of %s can be compared only to empty", call.name)), true
		}
		// if the expression is ifneq (,$(call is-vendor-board-platform,...)), negate==true,
		// so we should set inExpr.isNot to false
		return &inExpr{
			expr:  &variableRefExpr{ctx.addVariable("TARGET_BOARD_PLATFORM"), false},
			list:  &variableRefExpr{ctx.addVariable("QCOM_BOARD_PLATFORMS"), true},
			isNot: isEq,
		}, true
	}
	return nil, false
}

func (ctx *parseContext) parseCompareFilterFuncResult(cond *mkparser.Directive,
	filterFuncCall *callExpr, xValue starlarkExpr, negate bool) starlarkExpr {
	// We handle:
	// *  ifeq/ifneq (,$(filter v1 v2 ..., EXPR) becomes if EXPR not in/in ["v1", "v2", ...]
	// *  ifeq/ifneq (,$(filter EXPR, v1 v2 ...) becomes if EXPR not in/in ["v1", "v2", ...]
	// *  ifeq/ifneq ($(VAR),$(filter $(VAR), v1 v2 ...) becomes if VAR in/not in ["v1", "v2"]
	// TODO(Asmundak): check the last case works for filter-out, too.
	xPattern := filterFuncCall.args[0]
	xText := filterFuncCall.args[1]
	var xInList *stringLiteralExpr
	var expr starlarkExpr
	var ok bool
	switch x := xValue.(type) {
	case *stringLiteralExpr:
		if x.literal != "" {
			return ctx.newBadExpr(cond, "filter comparison to non-empty value: %s", xValue)
		}
		// Either pattern or text should be const, and the
		// non-const one should be varRefExpr
		if xInList, ok = xPattern.(*stringLiteralExpr); ok && !strings.ContainsRune(xInList.literal, '%') && xText.typ() == starlarkTypeList {
			expr = xText
		} else if xInList, ok = xText.(*stringLiteralExpr); ok {
			expr = xPattern
		} else {
			expr = &callExpr{
				object:     nil,
				name:       filterFuncCall.name,
				args:       filterFuncCall.args,
				returnType: starlarkTypeBool,
			}
			if negate {
				expr = &notExpr{expr: expr}
			}
			return expr
		}
	case *variableRefExpr:
		if v, ok := xPattern.(*variableRefExpr); ok {
			if xInList, ok = xText.(*stringLiteralExpr); ok && v.ref.name() == x.ref.name() {
				// ifeq/ifneq ($(VAR),$(filter $(VAR), v1 v2 ...), flip negate,
				// it's the opposite to what is done when comparing to empty.
				expr = xPattern
				negate = !negate
			}
		}
	}
	if expr != nil && xInList != nil {
		slExpr := newStringListExpr(strings.Fields(xInList.literal))
		// Generate simpler code for the common cases:
		if expr.typ() == starlarkTypeList {
			if len(slExpr.items) == 1 {
				// Checking that a string belongs to list
				return &inExpr{isNot: negate, list: expr, expr: slExpr.items[0]}
			} else {
				// TODO(asmundak):
				panic("TBD")
			}
		} else if len(slExpr.items) == 1 {
			return &eqExpr{left: expr, right: slExpr.items[0], isEq: !negate}
		} else {
			return &inExpr{isNot: negate, list: newStringListExpr(strings.Fields(xInList.literal)), expr: expr}
		}
	}
	return ctx.newBadExpr(cond, "filter arguments are too complex: %s", cond.Dump())
}

func (ctx *parseContext) parseCompareWildcardFuncResult(directive *mkparser.Directive,
	xCall *callExpr, xValue starlarkExpr, negate bool) starlarkExpr {
	if !isEmptyString(xValue) {
		return ctx.newBadExpr(directive, "wildcard result can be compared only to empty: %s", xValue)
	}
	callFunc := wildcardExistsPhony
	if s, ok := xCall.args[0].(*stringLiteralExpr); ok && !strings.ContainsAny(s.literal, "*?{[") {
		callFunc = fileExistsPhony
	}
	var cc starlarkExpr = &callExpr{name: callFunc, args: xCall.args, returnType: starlarkTypeBool}
	if !negate {
		cc = &notExpr{cc}
	}
	return cc
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
	}
	return ctx.newBadExpr(directive, "findstring result can be compared only to empty: %s", xValue)
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

// parses $(...), returning an expression
func (ctx *parseContext) parseReference(node mkparser.Node, ref *mkparser.MakeString) starlarkExpr {
	ref.TrimLeftSpaces()
	ref.TrimRightSpaces()
	refDump := ref.Dump()

	// Handle only the case where the first (or only) word is constant
	words := ref.SplitN(" ", 2)
	if !words[0].Const() {
		return ctx.newBadExpr(node, "reference is too complex: %s", refDump)
	}

	// If it is a single word, it can be a simple variable
	// reference or a function call
	if len(words) == 1 {
		if isMakeControlFunc(refDump) || refDump == "shell" {
			return &callExpr{
				name:       refDump,
				args:       []starlarkExpr{&stringLiteralExpr{""}},
				returnType: starlarkTypeUnknown,
			}
		}
		if strings.HasPrefix(refDump, soongNsPrefix) {
			// TODO (asmundak): if we find many, maybe handle them.
			return ctx.newBadExpr(node, "SOONG_CONFIG_ variables cannot be referenced, use soong_config_get instead: %s", refDump)
		}
		if v := ctx.addVariable(refDump); v != nil {
			return &variableRefExpr{v, ctx.lastAssignment(v.name()) != nil}
		}
		return ctx.newBadExpr(node, "unknown variable %s", refDump)
	}

	expr := &callExpr{name: words[0].Dump(), returnType: starlarkTypeUnknown}
	args := words[1]
	args.TrimLeftSpaces()
	// Make control functions and shell need special treatment as everything
	// after the name is a single text argument
	if isMakeControlFunc(expr.name) || expr.name == "shell" {
		x := ctx.parseMakeString(node, args)
		if xBad, ok := x.(*badExpr); ok {
			return xBad
		}
		expr.args = []starlarkExpr{x}
		return expr
	}
	if expr.name == "call" {
		words = args.SplitN(",", 2)
		if words[0].Empty() || !words[0].Const() {
			return ctx.newBadExpr(node, "cannot handle %s", refDump)
		}
		expr.name = words[0].Dump()
		if len(words) < 2 {
			args = &mkparser.MakeString{}
		} else {
			args = words[1]
		}
	}
	if kf, found := knownFunctions[expr.name]; found {
		expr.returnType = kf.returnType
	} else {
		return ctx.newBadExpr(node, "cannot handle invoking %s", expr.name)
	}
	switch expr.name {
	case "word":
		return ctx.parseWordFunc(node, args)
	case "firstword", "lastword":
		return ctx.parseFirstOrLastwordFunc(node, expr.name, args)
	case "my-dir":
		return &variableRefExpr{ctx.addVariable("LOCAL_PATH"), true}
	case "subst", "patsubst":
		return ctx.parseSubstFunc(node, expr.name, args)
	default:
		for _, arg := range args.Split(",") {
			arg.TrimLeftSpaces()
			arg.TrimRightSpaces()
			x := ctx.parseMakeString(node, arg)
			if xBad, ok := x.(*badExpr); ok {
				return xBad
			}
			expr.args = append(expr.args, x)
		}
	}
	return expr
}

func (ctx *parseContext) parseSubstFunc(node mkparser.Node, fname string, args *mkparser.MakeString) starlarkExpr {
	words := args.Split(",")
	if len(words) != 3 {
		return ctx.newBadExpr(node, "%s function should have 3 arguments", fname)
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
	if typ == starlarkTypeString && fname == "subst" {
		// Optimization: if it's $(subst from, to, string), emit string.replace(from, to)
		return &callExpr{
			object:     obj,
			name:       "replace",
			args:       []starlarkExpr{from, to},
			returnType: typ,
		}
	}
	return &callExpr{
		name:       fname,
		args:       []starlarkExpr{from, to, obj},
		returnType: obj.typ(),
	}
}

func (ctx *parseContext) parseWordFunc(node mkparser.Node, args *mkparser.MakeString) starlarkExpr {
	words := args.Split(",")
	if len(words) != 2 {
		return ctx.newBadExpr(node, "word function should have 2 arguments")
	}
	var index uint64 = 0
	if words[0].Const() {
		index, _ = strconv.ParseUint(strings.TrimSpace(words[0].Strings[0]), 10, 64)
	}
	if index < 1 {
		return ctx.newBadExpr(node, "word index should be constant positive integer")
	}
	words[1].TrimLeftSpaces()
	words[1].TrimRightSpaces()
	array := ctx.parseMakeString(node, words[1])
	if xBad, ok := array.(*badExpr); ok {
		return xBad
	}
	if array.typ() != starlarkTypeList {
		array = &callExpr{object: array, name: "split", returnType: starlarkTypeList}
	}
	return indexExpr{array, &intLiteralExpr{int(index - 1)}}
}

func (ctx *parseContext) parseFirstOrLastwordFunc(node mkparser.Node, name string, args *mkparser.MakeString) starlarkExpr {
	arg := ctx.parseMakeString(node, args)
	if bad, ok := arg.(*badExpr); ok {
		return bad
	}
	index := &intLiteralExpr{0}
	if name == "lastword" {
		if v, ok := arg.(*variableRefExpr); ok && v.ref.name() == "MAKEFILE_LIST" {
			return &stringLiteralExpr{ctx.script.mkFile}
		}
		index.literal = -1
	}
	if arg.typ() == starlarkTypeList {
		return &indexExpr{arg, index}
	}
	return &indexExpr{&callExpr{object: arg, name: "split", returnType: starlarkTypeList}, index}
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
	xInterp := &interpolateExpr{args: make([]starlarkExpr, len(mk.Variables))}
	for i, ref := range mk.Variables {
		arg := ctx.parseReference(node, ref.Name)
		if x, ok := arg.(*badExpr); ok {
			return x
		}
		xInterp.args[i] = arg
	}
	xInterp.chunks = append(xInterp.chunks, mk.Strings...)
	return xInterp
}

// Handles the statements whose treatment is the same in all contexts: comment,
// assignment, variable (which is a macro call in reality) and all constructs that
// do not handle in any context ('define directive and any unrecognized stuff).
func (ctx *parseContext) handleSimpleStatement(node mkparser.Node) {
	switch x := node.(type) {
	case *mkparser.Comment:
		ctx.maybeHandleAnnotation(x)
		ctx.insertComment("#" + x.Comment)
	case *mkparser.Assignment:
		ctx.handleAssignment(x)
	case *mkparser.Variable:
		ctx.handleVariable(x)
	case *mkparser.Directive:
		switch x.Name {
		case "define":
			ctx.handleDefine(x)
		case "include", "-include":
			ctx.handleInclude(node, ctx.parseMakeString(node, x.Args), x.Name[0] != '-')
		case "ifeq", "ifneq", "ifdef", "ifndef":
			ctx.handleIfBlock(x)
		default:
			ctx.errorf(x, "unexpected directive %s", x.Name)
		}
	default:
		ctx.errorf(x, "unsupported line %s", strings.ReplaceAll(x.Dump(), "\n", "\n#"))
	}
}

// Processes annotation. An annotation is a comment that starts with #RBC# and provides
// a conversion hint -- say, where to look for the dynamically calculated inherit/include
// paths.
func (ctx *parseContext) maybeHandleAnnotation(cnode *mkparser.Comment) {
	maybeTrim := func(s, prefix string) (string, bool) {
		if strings.HasPrefix(s, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(s, prefix)), true
		}
		return s, false
	}
	annotation, ok := maybeTrim(cnode.Comment, annotationCommentPrefix)
	if !ok {
		return
	}
	if p, ok := maybeTrim(annotation, "include_top"); ok {
		ctx.includeTops = append(ctx.includeTops, p)
		return
	}
	ctx.errorf(cnode, "unsupported annotation %s", cnode.Comment)

}

func (ctx *parseContext) insertComment(s string) {
	ctx.receiver.newNode(&commentNode{strings.TrimSpace(s)})
}

func (ctx *parseContext) carryAsComment(failedNode mkparser.Node) {
	for _, line := range strings.Split(failedNode.Dump(), "\n") {
		ctx.insertComment("# " + line)
	}
}

// records that the given node failed to be converted and includes an explanatory message
func (ctx *parseContext) errorf(failedNode mkparser.Node, message string, args ...interface{}) {
	if ctx.errorLogger != nil {
		ctx.errorLogger.NewError(ctx.errorLocation(failedNode), failedNode, message, args...)
	}
	ctx.receiver.newNode(&exprNode{ctx.newBadExpr(failedNode, message, args...)})
	ctx.script.hasErrors = true
}

func (ctx *parseContext) wrapBadExpr(xBad *badExpr) {
	ctx.receiver.newNode(&exprNode{xBad})
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
		topDir:         req.RootDir,
		traceCalls:     req.TraceCalls,
		sourceFS:       req.SourceFS,
		makefileFinder: req.MakefileFinder,
		nodeLocator:    func(pos mkparser.Pos) int { return parser.Unpack(pos).Line },
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
	ctx.pushReceiver(starScript)
	for ctx.hasNodes() && ctx.fatalError == nil {
		ctx.handleSimpleStatement(ctx.getNode())
	}
	if ctx.fatalError != nil {
		return nil, ctx.fatalError
	}
	return starScript, nil
}

func Launcher(mainModuleUri, versionDefaultsUri, mainModuleName string) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "load(%q, %q)\n", baseUri, baseName)
	fmt.Fprintf(&buf, "load(%q, \"version_defaults\")\n", versionDefaultsUri)
	fmt.Fprintf(&buf, "load(%q, \"init\")\n", mainModuleUri)
	fmt.Fprintf(&buf, "%s(%s(%q, init, version_defaults))\n", cfnPrintVars, cfnMain, mainModuleName)
	return buf.String()
}

func BoardLauncher(mainModuleUri string, inputVariablesUri string) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "load(%q, %q)\n", baseUri, baseName)
	fmt.Fprintf(&buf, "load(%q, \"init\")\n", mainModuleUri)
	fmt.Fprintf(&buf, "load(%q, input_variables_init = \"init\")\n", inputVariablesUri)
	fmt.Fprintf(&buf, "globals, cfg, globals_base = %s(init, input_variables_init)\n", cfnBoardMain)
	fmt.Fprintf(&buf, "# TODO: Some product config variables need to be printed, but most are readonly so we can't just print cfg here.\n")
	fmt.Fprintf(&buf, "%s((globals, cfg, globals_base))\n", cfnPrintVars)
	return buf.String()
}

func MakePath2ModuleName(mkPath string) string {
	return strings.TrimSuffix(mkPath, filepath.Ext(mkPath))
}
