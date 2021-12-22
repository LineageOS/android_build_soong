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
}

func (im moduleInfo) entryName() string {
	return im.moduleLocalName + "_init"
}

type inheritedModule interface {
	name() string
	entryName() string
	emitSelect(gctx *generationContext)
	shouldExist() bool
}

type inheritedStaticModule struct {
	*moduleInfo
	loadAlways bool
}

func (im inheritedStaticModule) name() string {
	return fmt.Sprintf("%q", MakePath2ModuleName(im.originalPath))
}

func (im inheritedStaticModule) emitSelect(_ *generationContext) {
}

func (im inheritedStaticModule) shouldExist() bool {
	return im.loadAlways
}

type inheritedDynamicModule struct {
	path             interpolateExpr
	candidateModules []*moduleInfo
	loadAlways       bool
}

func (i inheritedDynamicModule) name() string {
	return "_varmod"
}

func (i inheritedDynamicModule) entryName() string {
	return i.name() + "_init"
}

func (i inheritedDynamicModule) emitSelect(gctx *generationContext) {
	gctx.newLine()
	gctx.writef("_entry = {")
	gctx.indentLevel++
	for _, mi := range i.candidateModules {
		gctx.newLine()
		gctx.writef(`"%s": (%q, %s),`, mi.originalPath, mi.moduleLocalName, mi.entryName())
	}
	gctx.indentLevel--
	gctx.newLine()
	gctx.write("}.get(")
	i.path.emit(gctx)
	gctx.write(")")
	gctx.newLine()
	gctx.writef("(%s, %s) = _entry if _entry else (None, None)", i.name(), i.entryName())
	if i.loadAlways {
		gctx.newLine()
		gctx.writef("if not %s:", i.entryName())
		gctx.indentLevel++
		gctx.newLine()
		gctx.write(`rblf.mkerror("`, gctx.starScript.mkFile, `", "Cannot find %s" % (`)
		i.path.emit(gctx)
		gctx.write("))")
		gctx.indentLevel--
	}
}

func (i inheritedDynamicModule) shouldExist() bool {
	return i.loadAlways
}

type inheritNode struct {
	module     inheritedModule
	loadAlways bool
}

func (inn *inheritNode) emit(gctx *generationContext) {
	// Unconditional case:
	//    rblf.inherit(handle, <module>, module_init)
	// Conditional case:
	//    if <module>_init != None:
	//      same as above
	inn.module.emitSelect(gctx)

	name := inn.module.name()
	entry := inn.module.entryName()
	gctx.newLine()
	if inn.loadAlways {
		gctx.writef("%s(handle, %s, %s)", cfnInherit, name, entry)
		return
	}

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
	gctx.newLine()
	if inn.loadAlways {
		gctx.writef("%s(g, handle)", entry)
		return
	}

	gctx.writef("if %s != None:", entry)
	gctx.indentLevel++
	gctx.newLine()
	gctx.writef("%s(g, handle)", entry)
	gctx.indentLevel--
}

type assignmentFlavor int

const (
	// Assignment flavors
	asgnSet         assignmentFlavor = iota // := or =
	asgnMaybeSet    assignmentFlavor = iota // ?= and variable may be unset
	asgnAppend      assignmentFlavor = iota // += and variable has been set before
	asgnMaybeAppend assignmentFlavor = iota // += and variable may be unset
)

type assignmentNode struct {
	lhs      variable
	value    starlarkExpr
	mkValue  *mkparser.MakeString
	flavor   assignmentFlavor
	location ErrorLocation
	isTraced bool
	previous *assignmentNode
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
		asgn.lhs.emitGet(gctx, true)
		gctx.writef(")")
	}
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

func (cb *switchCase) newNode(node starlarkNode) {
	cb.nodes = append(cb.nodes, node)
}

func (cb *switchCase) emit(gctx *generationContext) {
	cb.gate.emit(gctx)
	gctx.indentLevel++
	hasStatements := false
	emitNode := func(node starlarkNode) {
		if _, ok := node.(*commentNode); !ok {
			hasStatements = true
		}
		node.emit(gctx)
	}
	if len(cb.nodes) > 0 {
		emitNode(cb.nodes[0])
		for _, node := range cb.nodes[1:] {
			emitNode(node)
		}
		if !hasStatements {
			gctx.emitPass()
		}
	} else {
		gctx.emitPass()
	}
	gctx.indentLevel--
}

// A single complete if ... elseif ... else ... endif sequences
type switchNode struct {
	ssCases []*switchCase
}

func (ssw *switchNode) newNode(node starlarkNode) {
	switch br := node.(type) {
	case *switchCase:
		ssw.ssCases = append(ssw.ssCases, br)
	default:
		panic(fmt.Errorf("expected switchCase node, got %t", br))
	}
}

func (ssw *switchNode) emit(gctx *generationContext) {
	if len(ssw.ssCases) == 0 {
		gctx.emitPass()
	} else {
		ssw.ssCases[0].emit(gctx)
		for _, ssCase := range ssw.ssCases[1:] {
			ssCase.emit(gctx)
		}
	}
}
