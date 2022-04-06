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
)

// Represents an expression in the Starlark code. An expression has a type.
type starlarkExpr interface {
	starlarkNode
	typ() starlarkType
	// Emit the code to copy the expression, otherwise we will end up
	// with source and target pointing to the same list.
	emitListVarCopy(gctx *generationContext)
	// Return the expression, calling the transformer func for
	// every expression in the tree. If the transformer func returns non-nil,
	// its result is used in place of the expression it was called with in the
	// resulting expression. The resulting starlarkExpr will contain as many
	// of the same objects from the original expression as possible.
	transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr
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

func (s *stringLiteralExpr) emit(gctx *generationContext) {
	gctx.writef("%q", s.literal)
}

func (_ *stringLiteralExpr) typ() starlarkType {
	return starlarkTypeString
}

func (s *stringLiteralExpr) emitListVarCopy(gctx *generationContext) {
	s.emit(gctx)
}

func (s *stringLiteralExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	if replacement := transformer(s); replacement != nil {
		return replacement
	} else {
		return s
	}
}

// Integer literal
type intLiteralExpr struct {
	literal int
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

func (s *intLiteralExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	if replacement := transformer(s); replacement != nil {
		return replacement
	} else {
		return s
	}
}

// Boolean literal
type boolLiteralExpr struct {
	literal bool
}

func (b *boolLiteralExpr) emit(gctx *generationContext) {
	if b.literal {
		gctx.write("True")
	} else {
		gctx.write("False")
	}
}

func (_ *boolLiteralExpr) typ() starlarkType {
	return starlarkTypeBool
}

func (b *boolLiteralExpr) emitListVarCopy(gctx *generationContext) {
	b.emit(gctx)
}

func (b *boolLiteralExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	if replacement := transformer(b); replacement != nil {
		return replacement
	} else {
		return b
	}
}

type globalsExpr struct {
}

func (g *globalsExpr) emit(gctx *generationContext) {
	gctx.write("g")
}

func (g *globalsExpr) typ() starlarkType {
	return starlarkTypeUnknown
}

func (g *globalsExpr) emitListVarCopy(gctx *generationContext) {
	g.emit(gctx)
}

func (g *globalsExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	if replacement := transformer(g); replacement != nil {
		return replacement
	} else {
		return g
	}
}

// interpolateExpr represents Starlark's interpolation operator <string> % list
// we break <string> into a list of chunks, i.e., "first%second%third" % (X, Y)
// will have chunks = ["first", "second", "third"] and args = [X, Y]
type interpolateExpr struct {
	chunks []string // string chunks, separated by '%'
	args   []starlarkExpr
}

func NewInterpolateExpr(parts []starlarkExpr) starlarkExpr {
	result := &interpolateExpr{}
	needString := true
	for _, part := range parts {
		if needString {
			if strLit, ok := part.(*stringLiteralExpr); ok {
				result.chunks = append(result.chunks, strLit.literal)
			} else {
				result.chunks = append(result.chunks, "")
			}
			needString = false
		} else {
			if strLit, ok := part.(*stringLiteralExpr); ok {
				result.chunks[len(result.chunks)-1] += strLit.literal
			} else {
				result.args = append(result.args, part)
				needString = true
			}
		}
	}
	if len(result.chunks) == len(result.args) {
		result.chunks = append(result.chunks, "")
	}
	if len(result.args) == 0 {
		return &stringLiteralExpr{literal: strings.Join(result.chunks, "")}
	}
	return result
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

func (_ *interpolateExpr) typ() starlarkType {
	return starlarkTypeString
}

func (xi *interpolateExpr) emitListVarCopy(gctx *generationContext) {
	xi.emit(gctx)
}

func (xi *interpolateExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	for i := range xi.args {
		xi.args[i] = xi.args[i].transform(transformer)
	}
	if replacement := transformer(xi); replacement != nil {
		return replacement
	} else {
		return xi
	}
}

type variableRefExpr struct {
	ref       variable
	isDefined bool
}

func NewVariableRefExpr(ref variable, isDefined bool) starlarkExpr {
	if predefined, ok := ref.(*predefinedVariable); ok {
		return predefined.value
	}
	return &variableRefExpr{ref, isDefined}
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

func (v *variableRefExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	if replacement := transformer(v); replacement != nil {
		return replacement
	} else {
		return v
	}
}

type toStringExpr struct {
	expr starlarkExpr
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
		ctx.write(`("true" if (`)
		s.expr.emit(ctx)
		ctx.write(`) else "")`)
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

func (s *toStringExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	s.expr = s.expr.transform(transformer)
	if replacement := transformer(s); replacement != nil {
		return replacement
	} else {
		return s
	}
}

type notExpr struct {
	expr starlarkExpr
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

func (n *notExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	n.expr = n.expr.transform(transformer)
	if replacement := transformer(n); replacement != nil {
		return replacement
	} else {
		return n
	}
}

type eqExpr struct {
	left, right starlarkExpr
	isEq        bool // if false, it's !=
}

func (eq *eqExpr) emit(gctx *generationContext) {
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

func (eq *eqExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	eq.left = eq.left.transform(transformer)
	eq.right = eq.right.transform(transformer)
	if replacement := transformer(eq); replacement != nil {
		return replacement
	} else {
		return eq
	}
}

type listExpr struct {
	items []starlarkExpr
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

func (l *listExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	itemsCopy := make([]starlarkExpr, len(l.items))
	for i, item := range l.items {
		itemsCopy[i] = item.transform(transformer)
	}
	l.items = itemsCopy
	if replacement := transformer(l); replacement != nil {
		return replacement
	} else {
		return l
	}
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

func (_ *concatExpr) typ() starlarkType {
	return starlarkTypeList
}

func (c *concatExpr) emitListVarCopy(gctx *generationContext) {
	c.emit(gctx)
}

func (c *concatExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	itemsCopy := make([]starlarkExpr, len(c.items))
	for i, item := range c.items {
		itemsCopy[i] = item.transform(transformer)
	}
	c.items = itemsCopy
	if replacement := transformer(c); replacement != nil {
		return replacement
	} else {
		return c
	}
}

// inExpr generates <expr> [not] in <list>
type inExpr struct {
	expr  starlarkExpr
	list  starlarkExpr
	isNot bool
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

func (i *inExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	i.expr = i.expr.transform(transformer)
	i.list = i.list.transform(transformer)
	if replacement := transformer(i); replacement != nil {
		return replacement
	} else {
		return i
	}
}

type indexExpr struct {
	array starlarkExpr
	index starlarkExpr
}

func (ix *indexExpr) emit(gctx *generationContext) {
	ix.array.emit(gctx)
	gctx.write("[")
	ix.index.emit(gctx)
	gctx.write("]")
}

func (ix *indexExpr) typ() starlarkType {
	return starlarkTypeString
}

func (ix *indexExpr) emitListVarCopy(gctx *generationContext) {
	ix.emit(gctx)
}

func (ix *indexExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	ix.array = ix.array.transform(transformer)
	ix.index = ix.index.transform(transformer)
	if replacement := transformer(ix); replacement != nil {
		return replacement
	} else {
		return ix
	}
}

type callExpr struct {
	object     starlarkExpr // nil if static call
	name       string
	args       []starlarkExpr
	returnType starlarkType
}

func (cx *callExpr) emit(gctx *generationContext) {
	if cx.object != nil {
		gctx.write("(")
		cx.object.emit(gctx)
		gctx.write(")")
		gctx.write(".", cx.name, "(")
	} else {
		gctx.write(cx.name, "(")
	}
	sep := ""
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

func (cx *callExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	if cx.object != nil {
		cx.object = cx.object.transform(transformer)
	}
	for i := range cx.args {
		cx.args[i] = cx.args[i].transform(transformer)
	}
	if replacement := transformer(cx); replacement != nil {
		return replacement
	} else {
		return cx
	}
}

type ifExpr struct {
	condition starlarkExpr
	ifTrue    starlarkExpr
	ifFalse   starlarkExpr
}

func (i *ifExpr) emit(gctx *generationContext) {
	gctx.write("(")
	i.ifTrue.emit(gctx)
	gctx.write(" if ")
	i.condition.emit(gctx)
	gctx.write(" else ")
	i.ifFalse.emit(gctx)
	gctx.write(")")
}

func (i *ifExpr) typ() starlarkType {
	tType := i.ifTrue.typ()
	fType := i.ifFalse.typ()
	if tType != fType && tType != starlarkTypeUnknown && fType != starlarkTypeUnknown {
		panic("Conflicting types in if expression")
	}
	if tType != starlarkTypeUnknown {
		return tType
	} else {
		return fType
	}
}

func (i *ifExpr) emitListVarCopy(gctx *generationContext) {
	i.emit(gctx)
}

func (i *ifExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	i.condition = i.condition.transform(transformer)
	i.ifTrue = i.ifTrue.transform(transformer)
	i.ifFalse = i.ifFalse.transform(transformer)
	if replacement := transformer(i); replacement != nil {
		return replacement
	} else {
		return i
	}
}

type identifierExpr struct {
	name string
}

func (i *identifierExpr) emit(gctx *generationContext) {
	gctx.write(i.name)
}

func (i *identifierExpr) typ() starlarkType {
	return starlarkTypeUnknown
}

func (i *identifierExpr) emitListVarCopy(gctx *generationContext) {
	i.emit(gctx)
}

func (i *identifierExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	if replacement := transformer(i); replacement != nil {
		return replacement
	} else {
		return i
	}
}

type foreachExpr struct {
	varName string
	list    starlarkExpr
	action  starlarkExpr
}

func (f *foreachExpr) emit(gctx *generationContext) {
	gctx.write("[")
	f.action.emit(gctx)
	gctx.write(" for " + f.varName + " in ")
	f.list.emit(gctx)
	gctx.write("]")
}

func (f *foreachExpr) typ() starlarkType {
	return starlarkTypeList
}

func (f *foreachExpr) emitListVarCopy(gctx *generationContext) {
	f.emit(gctx)
}

func (f *foreachExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	f.list = f.list.transform(transformer)
	f.action = f.action.transform(transformer)
	if replacement := transformer(f); replacement != nil {
		return replacement
	} else {
		return f
	}
}

type binaryOpExpr struct {
	left, right starlarkExpr
	op          string
	returnType  starlarkType
}

func (b *binaryOpExpr) emit(gctx *generationContext) {
	b.left.emit(gctx)
	gctx.write(" " + b.op + " ")
	b.right.emit(gctx)
}

func (b *binaryOpExpr) typ() starlarkType {
	return b.returnType
}

func (b *binaryOpExpr) emitListVarCopy(gctx *generationContext) {
	b.emit(gctx)
}

func (b *binaryOpExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	b.left = b.left.transform(transformer)
	b.right = b.right.transform(transformer)
	if replacement := transformer(b); replacement != nil {
		return replacement
	} else {
		return b
	}
}

type badExpr struct {
	errorLocation ErrorLocation
	message       string
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

func (b *badExpr) transform(transformer func(expr starlarkExpr) starlarkExpr) starlarkExpr {
	if replacement := transformer(b); replacement != nil {
		return replacement
	} else {
		return b
	}
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

func negateExpr(expr starlarkExpr) starlarkExpr {
	switch typedExpr := expr.(type) {
	case *notExpr:
		return typedExpr.expr
	case *inExpr:
		typedExpr.isNot = !typedExpr.isNot
		return typedExpr
	case *eqExpr:
		typedExpr.isEq = !typedExpr.isEq
		return typedExpr
	case *binaryOpExpr:
		switch typedExpr.op {
		case ">":
			typedExpr.op = "<="
			return typedExpr
		case "<":
			typedExpr.op = ">="
			return typedExpr
		case ">=":
			typedExpr.op = "<"
			return typedExpr
		case "<=":
			typedExpr.op = ">"
			return typedExpr
		default:
			return &notExpr{expr: expr}
		}
	default:
		return &notExpr{expr: expr}
	}
}
