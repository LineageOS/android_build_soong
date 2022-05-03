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

package mk2rbc

import (
	"fmt"
	"strings"

	mkparser "android/soong/androidmk/parser"
)

// A parsed node for which starlark code will be generated
// by calling emit().
type starlarkNode interface {
	emit(ctx *generationContext)
}

// Types used to keep processed makefile data:
type commentNode struct {
	text string
}

func (c *commentNode) emit(gctx *generationContext) {
	chunks := strings.Split(c.text, "\\\n")
	gctx.newLine()
	gctx.write(chunks[0]) // It has '#' at the beginning already.
	for _, chunk := range chunks[1:] {
		gctx.newLine()
		gctx.write("#", chunk)
	}
}

type moduleInfo struct {
	path            string // Converted Starlark file path
	originalPath    string // Makefile file path
	moduleLocalName string
	optional        bool
	missing         bool // a module may not exist if a module that depends on it is loaded dynamically
}

func (im moduleInfo) entryName() string {
	return im.moduleLocalName + "_init"
}

func (mi moduleInfo) name() string {
	return fmt.Sprintf("%q", MakePath2ModuleName(mi.originalPath))
}

type inheritedModule interface {
	name() string
	entryName() string
	emitSelect(gctx *generationContext)
	pathExpr() starlarkExpr
	needsLoadCheck() bool
}

type inheritedStaticModule struct {
	*moduleInfo
	loadAlways bool
}

func (im inheritedStaticModule) emitSelect(_ *generationContext) {
}

func (im inheritedStaticModule) pathExpr() starlarkExpr {
	return &stringLiteralExpr{im.path}
}

func (im inheritedStaticModule) needsLoadCheck() bool {
	return im.missing
}

type inheritedDynamicModule struct {
	path             starlarkExpr
	candidateModules []*moduleInfo
	loadAlways       bool
	location         ErrorLocation
	needsWarning     bool
}

func (i inheritedDynamicModule) name() string {
	return "_varmod"
}

func (i inheritedDynamicModule) entryName() string {
	return i.name() + "_init"
}

func (i inheritedDynamicModule) emitSelect(gctx *generationContext) {
	if i.needsWarning {
		gctx.newLine()
		gctx.writef("%s.mkwarning(%q, %q)", baseName, i.location, "Please avoid starting an include path with a variable. See https://source.android.com/setup/build/bazel/product_config/issues/includes for details.")
	}
	gctx.newLine()
	gctx.writef("_entry = {")
	gctx.indentLevel++
	for _, mi := range i.candidateModules {
		gctx.newLine()
		gctx.writef(`"%s": (%s, %s),`, mi.originalPath, mi.name(), mi.entryName())
	}
	gctx.indentLevel--
	gctx.newLine()
	gctx.write("}.get(")
	i.path.emit(gctx)
	gctx.write(")")
	gctx.newLine()
	gctx.writef("(%s, %s) = _entry if _entry else (None, None)", i.name(), i.entryName())
}

func (i inheritedDynamicModule) pathExpr() starlarkExpr {
	return i.path
}

func (i inheritedDynamicModule) needsLoadCheck() bool {
	return true
}

type inheritNode struct {
	module     inheritedModule
	loadAlways bool
}

func (inn *inheritNode) emit(gctx *generationContext) {
	// Unconditional case:
	//    maybe check that loaded
	//    rblf.inherit(handle, <module>, module_init)
	// Conditional case:
	//    if <module>_init != None:
	//      same as above
	inn.module.emitSelect(gctx)
	name := inn.module.name()
	entry := inn.module.entryName()
	if inn.loadAlways {
		gctx.emitLoadCheck(inn.module)
		gctx.newLine()
		gctx.writef("%s(handle, %s, %s)", cfnInherit, name, entry)
		return
	}

	gctx.newLine()
	gctx.writef("if %s:", entry)
	gctx.indentLevel++
	gctx.newLine()
	gctx.writef("%s(handle, %s, %s)", cfnInherit, name, entry)
	gctx.indentLevel--
}

type includeNode struct {
	module     inheritedModule
	loadAlways bool
}

func (inn *includeNode) emit(gctx *generationContext) {
	inn.module.emitSelect(gctx)
	entry := inn.module.entryName()
	if inn.loadAlways {
		gctx.emitLoadCheck(inn.module)
		gctx.newLine()
		gctx.writef("%s(g, handle)", entry)
		return
	}

	gctx.newLine()
	gctx.writef("if %s != None:", entry)
	gctx.indentLevel++
	gctx.newLine()
	gctx.writef("%s(g, handle)", entry)
	gctx.indentLevel--
}

type assignmentFlavor int

const (
	// Assignment flavors
	asgnSet      assignmentFlavor = iota // := or =
	asgnMaybeSet assignmentFlavor = iota // ?=
	asgnAppend   assignmentFlavor = iota // +=
)

type assignmentNode struct {
	lhs      variable
	value    starlarkExpr
	mkValue  *mkparser.MakeString
	flavor   assignmentFlavor
	location ErrorLocation
	isTraced bool
}

func (asgn *assignmentNode) emit(gctx *generationContext) {
	gctx.newLine()
	gctx.inAssignment = true
	asgn.lhs.emitSet(gctx, asgn)
	gctx.inAssignment = false

	if asgn.isTraced {
		gctx.newLine()
		gctx.tracedCount++
		gctx.writef(`print("%s.%d: %s := ", `, gctx.starScript.mkFile, gctx.tracedCount, asgn.lhs.name())
		asgn.lhs.emitGet(gctx)
		gctx.writef(")")
	}
}

func (asgn *assignmentNode) isSelfReferential() bool {
	if asgn.flavor == asgnAppend {
		return true
	}
	isSelfReferential := false
	asgn.value.transform(func(expr starlarkExpr) starlarkExpr {
		if ref, ok := expr.(*variableRefExpr); ok && ref.ref.name() == asgn.lhs.name() {
			isSelfReferential = true
		}
		return nil
	})
	return isSelfReferential
}

type exprNode struct {
	expr starlarkExpr
}

func (exn *exprNode) emit(gctx *generationContext) {
	gctx.newLine()
	exn.expr.emit(gctx)
}

type ifNode struct {
	isElif bool // true if this is 'elif' statement
	expr   starlarkExpr
}

func (in *ifNode) emit(gctx *generationContext) {
	ifElif := "if "
	if in.isElif {
		ifElif = "elif "
	}

	gctx.newLine()
	gctx.write(ifElif)
	in.expr.emit(gctx)
	gctx.write(":")
}

type elseNode struct{}

func (br *elseNode) emit(gctx *generationContext) {
	gctx.newLine()
	gctx.write("else:")
}

// switchCase represents as single if/elseif/else branch. All the necessary
// info about flavor (if/elseif/else) is supposed to be kept in `gate`.
type switchCase struct {
	gate  starlarkNode
	nodes []starlarkNode
}

func (cb *switchCase) emit(gctx *generationContext) {
	cb.gate.emit(gctx)
	gctx.indentLevel++
	gctx.pushVariableAssignments()
	hasStatements := false
	for _, node := range cb.nodes {
		if _, ok := node.(*commentNode); !ok {
			hasStatements = true
		}
		node.emit(gctx)
	}
	if !hasStatements {
		gctx.emitPass()
	}
	gctx.indentLevel--
	gctx.popVariableAssignments()
}

// A single complete if ... elseif ... else ... endif sequences
type switchNode struct {
	ssCases []*switchCase
}

func (ssw *switchNode) emit(gctx *generationContext) {
	for _, ssCase := range ssw.ssCases {
		ssCase.emit(gctx)
	}
}

type foreachNode struct {
	varName string
	list    starlarkExpr
	actions []starlarkNode
}

func (f *foreachNode) emit(gctx *generationContext) {
	gctx.pushVariableAssignments()
	gctx.newLine()
	gctx.writef("for %s in ", f.varName)
	f.list.emit(gctx)
	gctx.write(":")
	gctx.indentLevel++
	hasStatements := false
	for _, a := range f.actions {
		if _, ok := a.(*commentNode); !ok {
			hasStatements = true
		}
		a.emit(gctx)
	}
	if !hasStatements {
		gctx.emitPass()
	}
	gctx.indentLevel--
	gctx.popVariableAssignments()
}
