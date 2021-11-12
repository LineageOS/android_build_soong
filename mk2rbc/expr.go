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
	"strconv"
	"strings"
)

// Represents an expression in the Starlark code. An expression has
// a type, and it can be evaluated.
type starlarkExpr interface {
	starlarkNode
	typ() starlarkType
	// Try to substitute variable values. Return substitution result
	// and whether it is the same as the original expression.
	eval(valueMap map[string]starlarkExpr) (res starlarkExpr, same bool)
	// Emit the code to copy the expression, otherwise we will end up
	// with source and target pointing to the same list.
	emitListVarCopy(gctx *generationContext)
}

func maybeString(expr starlarkExpr) (string, bool) {
	if x, ok := expr.(*stringLiteralExpr); ok {
		return x.literal, true
	}
	return "", false
}

type stringLiteralExpr struct {
	literal string
}

func (s *stringLiteralExpr) eval(_ map[string]starlarkExpr) (res starlarkExpr, same bool) {
	res = s
	same = true
	return
}

func (s *stringLiteralExpr) emit(gctx *generationContext) {
	gctx.writef("%q", s.literal)
}

func (_ *stringLiteralExpr) typ() starlarkType {
	return starlarkTypeString
}

func (s *stringLiteralExpr) emitListVarCopy(gctx *generationContext) {
	s.emit(gctx)
}

// Integer literal
type intLiteralExpr struct {
	literal int
}

func (s *intLiteralExpr) eval(_ map[string]starlarkExpr) (res starlarkExpr, same bool) {
	res = s
	same = true
	return
}

func (s *intLiteralExpr) emit(gctx *generationContext) {
	gctx.writef("%d", s.literal)
}

func (_ *intLiteralExpr) typ() starlarkType {
	return starlarkTypeInt
}

func (s *intLiteralExpr) emitListVarCopy(gctx *generationContext) {
	s.emit(gctx)
}

// interpolateExpr represents Starlark's interpolation operator <string> % list
// we break <string> into a list of chunks, i.e., "first%second%third" % (X, Y)
// will have chunks = ["first", "second", "third"] and args = [X, Y]
type interpolateExpr struct {
	chunks []string // string chunks, separated by '%'
	args   []starlarkExpr
}

func (xi *interpolateExpr) emit(gctx *generationContext) {
	if len(xi.chunks) != len(xi.args)+1 {
		panic(fmt.Errorf("malformed interpolateExpr: #chunks(%d) != #args(%d)+1",
			len(xi.chunks), len(xi.args)))
	}
	// Generate format as join of chunks, but first escape '%' in them
	format := strings.ReplaceAll(xi.chunks[0], "%", "%%")
	for _, chunk := range xi.chunks[1:] {
		format += "%s" + strings.ReplaceAll(chunk, "%", "%%")
	}
	gctx.writef("%q %% ", format)
	emitArg := func(arg starlarkExpr) {
		if arg.typ() == starlarkTypeList {
			gctx.write(`" ".join(`)
			arg.emit(gctx)
			gctx.write(`)`)
		} else {
			arg.emit(gctx)
		}
	}
	if len(xi.args) == 1 {
		emitArg(xi.args[0])
	} else {
		sep := "("
		for _, arg := range xi.args {
			gctx.write(sep)
			emitArg(arg)
			sep = ", "
		}
		gctx.write(")")
	}
}

func (xi *interpolateExpr) eval(valueMap map[string]starlarkExpr) (res starlarkExpr, same bool) {
	same = true
	newChunks := []string{xi.chunks[0]}
	var newArgs []starlarkExpr
	for i, arg := range xi.args {
		newArg, sameArg := arg.eval(valueMap)
		same = same && sameArg
		switch x := newArg.(type) {
		case *stringLiteralExpr:
			newChunks[len(newChunks)-1] += x.literal + xi.chunks[i+1]
			same = false
			continue
		case *intLiteralExpr:
			newChunks[len(newChunks)-1] += strconv.Itoa(x.literal) + xi.chunks[i+1]
			same = false
			continue
		default:
			newChunks = append(newChunks, xi.chunks[i+1])
			newArgs = append(newArgs, newArg)
		}
	}
	if same {
		res = xi
	} else if len(newChunks) == 1 {
		res = &stringLiteralExpr{newChunks[0]}
	} else {
		res = &interpolateExpr{chunks: newChunks, args: newArgs}
	}
	return
}

func (_ *interpolateExpr) typ() starlarkType {
	return starlarkTypeString
}

func (xi *interpolateExpr) emitListVarCopy(gctx *generationContext) {
	xi.emit(gctx)
}

type variableRefExpr struct {
	ref       variable
	isDefined bool
}

func (v *variableRefExpr) eval(map[string]starlarkExpr) (res starlarkExpr, same bool) {
	predefined, ok := v.ref.(*predefinedVariable)
	if same = !ok; same {
		res = v
	} else {
		res = predefined.value
	}
	return
}

func (v *variableRefExpr) emit(gctx *generationContext) {
	v.ref.emitGet(gctx, v.isDefined)
}

func (v *variableRefExpr) typ() starlarkType {
	return v.ref.valueType()
}

func (v *variableRefExpr) emitListVarCopy(gctx *generationContext) {
	v.emit(gctx)
	if v.typ() == starlarkTypeList {
		gctx.write("[:]") // this will copy the list
	}
}

type toStringExpr struct {
	expr starlarkExpr
}

func (s *toStringExpr) eval(valueMap map[string]starlarkExpr) (res starlarkExpr, same bool) {
	if x, same := s.expr.eval(valueMap); same {
		res = s
	} else {
		res = &toStringExpr{expr: x}
	}
	return
}

func (s *toStringExpr) emit(ctx *generationContext) {
	switch s.expr.typ() {
	case starlarkTypeString, starlarkTypeUnknown:
		// Assume unknown types are strings already.
		s.expr.emit(ctx)
	case starlarkTypeList:
		ctx.write(`" ".join(`)
		s.expr.emit(ctx)
		ctx.write(")")
	case starlarkTypeInt:
		ctx.write(`("%d" % (`)
		s.expr.emit(ctx)
		ctx.write("))")
	case starlarkTypeBool:
		ctx.write("((")
		s.expr.emit(ctx)
		ctx.write(`) ? "true" : "")`)
	case starlarkTypeVoid:
		ctx.write(`""`)
	default:
		panic("Unknown starlark type!")
	}
}

func (s *toStringExpr) typ() starlarkType {
	return starlarkTypeString
}

func (s *toStringExpr) emitListVarCopy(gctx *generationContext) {
	s.emit(gctx)
}

type notExpr struct {
	expr starlarkExpr
}

func (n *notExpr) eval(valueMap map[string]starlarkExpr) (res starlarkExpr, same bool) {
	if x, same := n.expr.eval(valueMap); same {
		res = n
	} else {
		res = &notExpr{expr: x}
	}
	return
}

func (n *notExpr) emit(ctx *generationContext) {
	ctx.write("not ")
	n.expr.emit(ctx)
}

func (_ *notExpr) typ() starlarkType {
	return starlarkTypeBool
}

func (n *notExpr) emitListVarCopy(gctx *generationContext) {
	n.emit(gctx)
}

type eqExpr struct {
	left, right starlarkExpr
	isEq        bool // if false, it's !=
}

func (eq *eqExpr) eval(valueMap map[string]starlarkExpr) (res starlarkExpr, same bool) {
	xLeft, sameLeft := eq.left.eval(valueMap)
	xRight, sameRight := eq.right.eval(valueMap)
	if same = sameLeft && sameRight; same {
		res = eq
	} else {
		res = &eqExpr{left: xLeft, right: xRight, isEq: eq.isEq}
	}
	return
}

func (eq *eqExpr) emit(gctx *generationContext) {
	emitSimple := func(expr starlarkExpr) {
		if eq.isEq {
			gctx.write("not ")
		}
		expr.emit(gctx)
	}
	// Are we checking that a variable is empty?
	if isEmptyString(eq.left) {
		emitSimple(eq.right)
		return
	} else if isEmptyString(eq.right) {
		emitSimple(eq.left)
		return

	}

	if eq.left.typ() != eq.right.typ() {
		eq.left = &toStringExpr{expr: eq.left}
		eq.right = &toStringExpr{expr: eq.right}
	}

	// General case
	eq.left.emit(gctx)
	if eq.isEq {
		gctx.write(" == ")
	} else {
		gctx.write(" != ")
	}
	eq.right.emit(gctx)
}

func (_ *eqExpr) typ() starlarkType {
	return starlarkTypeBool
}

func (eq *eqExpr) emitListVarCopy(gctx *generationContext) {
	eq.emit(gctx)
}

// variableDefinedExpr corresponds to Make's ifdef VAR
type variableDefinedExpr struct {
	v variable
}

func (v *variableDefinedExpr) eval(_ map[string]starlarkExpr) (res starlarkExpr, same bool) {
	res = v
	same = true
	return

}

func (v *variableDefinedExpr) emit(gctx *generationContext) {
	if v.v != nil {
		v.v.emitDefined(gctx)
		return
	}
	gctx.writef("%s(%q)", cfnWarning, "TODO(VAR)")
}

func (_ *variableDefinedExpr) typ() starlarkType {
	return starlarkTypeBool
}

func (v *variableDefinedExpr) emitListVarCopy(gctx *generationContext) {
	v.emit(gctx)
}

type listExpr struct {
	items []starlarkExpr
}

func (l *listExpr) eval(valueMap map[string]starlarkExpr) (res starlarkExpr, same bool) {
	newItems := make([]starlarkExpr, len(l.items))
	same = true
	for i, item := range l.items {
		var sameItem bool
		newItems[i], sameItem = item.eval(valueMap)
		same = same && sameItem
	}
	if same {
		res = l
	} else {
		res = &listExpr{newItems}
	}
	return
}

func (l *listExpr) emit(gctx *generationContext) {
	if !gctx.inAssignment || len(l.items) < 2 {
		gctx.write("[")
		sep := ""
		for _, item := range l.items {
			gctx.write(sep)
			item.emit(gctx)
			sep = ", "
		}
		gctx.write("]")
		return
	}

	gctx.write("[")
	gctx.indentLevel += 2

	for _, item := range l.items {
		gctx.newLine()
		item.emit(gctx)
		gctx.write(",")
	}
	gctx.indentLevel -= 2
	gctx.newLine()
	gctx.write("]")
}

func (_ *listExpr) typ() starlarkType {
	return starlarkTypeList
}

func (l *listExpr) emitListVarCopy(gctx *generationContext) {
	l.emit(gctx)
}

func newStringListExpr(items []string) *listExpr {
	v := listExpr{}
	for _, item := range items {
		v.items = append(v.items, &stringLiteralExpr{item})
	}
	return &v
}

// concatExpr generates expr1 + expr2 + ... + exprN in Starlark.
type concatExpr struct {
	items []starlarkExpr
}

func (c *concatExpr) emit(gctx *generationContext) {
	if len(c.items) == 1 {
		c.items[0].emit(gctx)
		return
	}

	if !gctx.inAssignment {
		c.items[0].emit(gctx)
		for _, item := range c.items[1:] {
			gctx.write(" + ")
			item.emit(gctx)
		}
		return
	}
	gctx.write("(")
	c.items[0].emit(gctx)
	gctx.indentLevel += 2
	for _, item := range c.items[1:] {
		gctx.write(" +")
		gctx.newLine()
		item.emit(gctx)
	}
	gctx.write(")")
	gctx.indentLevel -= 2
}

func (c *concatExpr) eval(valueMap map[string]starlarkExpr) (res starlarkExpr, same bool) {
	same = true
	xConcat := &concatExpr{items: make([]starlarkExpr, len(c.items))}
	for i, item := range c.items {
		var sameItem bool
		xConcat.items[i], sameItem = item.eval(valueMap)
		same = same && sameItem
	}
	if same {
		res = c
	} else {
		res = xConcat
	}
	return
}

func (_ *concatExpr) typ() starlarkType {
	return starlarkTypeList
}

func (c *concatExpr) emitListVarCopy(gctx *generationContext) {
	c.emit(gctx)
}

// inExpr generates <expr> [not] in <list>
type inExpr struct {
	expr  starlarkExpr
	list  starlarkExpr
	isNot bool
}

func (i *inExpr) eval(valueMap map[string]starlarkExpr) (res starlarkExpr, same bool) {
	x := &inExpr{isNot: i.isNot}
	var sameExpr, sameList bool
	x.expr, sameExpr = i.expr.eval(valueMap)
	x.list, sameList = i.list.eval(valueMap)
	if same = sameExpr && sameList; same {
		res = i
	} else {
		res = x
	}
	return
}

func (i *inExpr) emit(gctx *generationContext) {
	i.expr.emit(gctx)
	if i.isNot {
		gctx.write(" not in ")
	} else {
		gctx.write(" in ")
	}
	i.list.emit(gctx)
}

func (_ *inExpr) typ() starlarkType {
	return starlarkTypeBool
}

func (i *inExpr) emitListVarCopy(gctx *generationContext) {
	i.emit(gctx)
}

type indexExpr struct {
	array starlarkExpr
	index starlarkExpr
}

func (ix indexExpr) emit(gctx *generationContext) {
	ix.array.emit(gctx)
	gctx.write("[")
	ix.index.emit(gctx)
	gctx.write("]")
}

func (ix indexExpr) typ() starlarkType {
	return starlarkTypeString
}

func (ix indexExpr) eval(valueMap map[string]starlarkExpr) (res starlarkExpr, same bool) {
	newArray, isSameArray := ix.array.eval(valueMap)
	newIndex, isSameIndex := ix.index.eval(valueMap)
	if same = isSameArray && isSameIndex; same {
		res = ix
	} else {
		res = &indexExpr{newArray, newIndex}
	}
	return
}

func (ix indexExpr) emitListVarCopy(gctx *generationContext) {
	ix.emit(gctx)
}

type callExpr struct {
	object     starlarkExpr // nil if static call
	name       string
	args       []starlarkExpr
	returnType starlarkType
}

func (cx *callExpr) eval(valueMap map[string]starlarkExpr) (res starlarkExpr, same bool) {
	newCallExpr := &callExpr{name: cx.name, args: make([]starlarkExpr, len(cx.args)),
		returnType: cx.returnType}
	if cx.object != nil {
		newCallExpr.object, same = cx.object.eval(valueMap)
	} else {
		same = true
	}
	for i, args := range cx.args {
		var s bool
		newCallExpr.args[i], s = args.eval(valueMap)
		same = same && s
	}
	if same {
		res = cx
	} else {
		res = newCallExpr
	}
	return
}

func (cx *callExpr) emit(gctx *generationContext) {
	sep := ""
	if cx.object != nil {
		gctx.write("(")
		cx.object.emit(gctx)
		gctx.write(")")
		gctx.write(".", cx.name, "(")
	} else {
		kf, found := knownFunctions[cx.name]
		if !found {
			panic(fmt.Errorf("callExpr with unknown function %q", cx.name))
		}
		if kf.runtimeName[0] == '!' {
			panic(fmt.Errorf("callExpr for %q should not be there", cx.name))
		}
		gctx.write(kf.runtimeName, "(")
		if kf.hiddenArg == hiddenArgGlobal {
			gctx.write("g")
			sep = ", "
		} else if kf.hiddenArg == hiddenArgConfig {
			gctx.write("cfg")
			sep = ", "
		}
	}
	for _, arg := range cx.args {
		gctx.write(sep)
		arg.emit(gctx)
		sep = ", "
	}
	gctx.write(")")
}

func (cx *callExpr) typ() starlarkType {
	return cx.returnType
}

func (cx *callExpr) emitListVarCopy(gctx *generationContext) {
	cx.emit(gctx)
}

type badExpr struct {
	errorLocation ErrorLocation
	message       string
}

func (b *badExpr) eval(_ map[string]starlarkExpr) (res starlarkExpr, same bool) {
	res = b
	same = true
	return
}

func (b *badExpr) emit(gctx *generationContext) {
	gctx.emitConversionError(b.errorLocation, b.message)
}

func (_ *badExpr) typ() starlarkType {
	return starlarkTypeUnknown
}

func (_ *badExpr) emitListVarCopy(_ *generationContext) {
	panic("implement me")
}

func maybeConvertToStringList(expr starlarkExpr) starlarkExpr {
	if xString, ok := expr.(*stringLiteralExpr); ok {
		return newStringListExpr(strings.Fields(xString.literal))
	}
	return expr
}

func isEmptyString(expr starlarkExpr) bool {
	x, ok := expr.(*stringLiteralExpr)
	return ok && x.literal == ""
}
