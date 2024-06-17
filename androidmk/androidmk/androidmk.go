// Copyright 2017 Google Inc. All rights reserved.
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

package androidmk

import (
	"bytes"
	"fmt"
	"strings"
	"text/scanner"

	"android/soong/bpfix/bpfix"

	mkparser "android/soong/androidmk/parser"

	bpparser "github.com/google/blueprint/parser"
)

// TODO: non-expanded variables with expressions

type bpFile struct {
	comments          []*bpparser.CommentGroup
	defs              []bpparser.Definition
	localAssignments  map[string]*bpparser.Property
	globalAssignments map[string]*bpparser.Expression
	variableRenames   map[string]string
	scope             mkparser.Scope
	module            *bpparser.Module

	mkPos scanner.Position // Position of the last handled line in the makefile
	bpPos scanner.Position // Position of the last emitted line to the blueprint file

	inModule bool
}

var invalidVariableStringToReplacement = map[string]string{
	"-": "_dash_",
}

// Fix steps that should only run in the androidmk tool, i.e. should only be applied to
// newly-converted Android.bp files.
var fixSteps = bpfix.FixStepsExtension{
	Name: "androidmk",
	Steps: []bpfix.FixStep{
		{
			Name: "RewriteRuntimeResourceOverlay",
			Fix:  bpfix.RewriteRuntimeResourceOverlay,
		},
	},
}

func init() {
	bpfix.RegisterFixStepExtension(&fixSteps)
}

func (f *bpFile) insertComment(s string) {
	f.comments = append(f.comments, &bpparser.CommentGroup{
		Comments: []*bpparser.Comment{
			&bpparser.Comment{
				Comment: []string{s},
				Slash:   f.bpPos,
			},
		},
	})
	f.bpPos.Offset += len(s)
}

func (f *bpFile) insertExtraComment(s string) {
	f.insertComment(s)
	f.bpPos.Line++
}

// records that the given node failed to be converted and includes an explanatory message
func (f *bpFile) errorf(failedNode mkparser.Node, message string, args ...interface{}) {
	orig := failedNode.Dump()
	message = fmt.Sprintf(message, args...)
	f.addErrorText(fmt.Sprintf("// ANDROIDMK TRANSLATION ERROR: %s", message))

	lines := strings.Split(orig, "\n")
	for _, l := range lines {
		f.insertExtraComment("// " + l)
	}
}

// records that something unexpected occurred
func (f *bpFile) warnf(message string, args ...interface{}) {
	message = fmt.Sprintf(message, args...)
	f.addErrorText(fmt.Sprintf("// ANDROIDMK TRANSLATION WARNING: %s", message))
}

// adds the given error message as-is to the bottom of the (in-progress) file
func (f *bpFile) addErrorText(message string) {
	f.insertExtraComment(message)
}

func (f *bpFile) setMkPos(pos, end scanner.Position) {
	// It is unusual but not forbidden for pos.Line to be smaller than f.mkPos.Line
	// For example:
	//
	// if true                       # this line is emitted 1st
	// if true                       # this line is emitted 2nd
	// some-target: some-file        # this line is emitted 3rd
	//         echo doing something  # this recipe is emitted 6th
	// endif #some comment           # this endif is emitted 4th; this comment is part of the recipe
	//         echo doing more stuff # this is part of the recipe
	// endif                         # this endif is emitted 5th
	//
	// However, if pos.Line < f.mkPos.Line, we treat it as though it were equal
	if pos.Line >= f.mkPos.Line {
		f.bpPos.Line += (pos.Line - f.mkPos.Line)
		f.mkPos = end
	}

}

type conditional struct {
	cond string
	eq   bool
}

func ConvertFile(filename string, buffer *bytes.Buffer) (string, []error) {
	p := mkparser.NewParser(filename, buffer)

	nodes, errs := p.Parse()
	if len(errs) > 0 {
		return "", errs
	}

	file := &bpFile{
		scope:             androidScope(),
		localAssignments:  make(map[string]*bpparser.Property),
		globalAssignments: make(map[string]*bpparser.Expression),
		variableRenames:   make(map[string]string),
	}

	var conds []*conditional
	var assignmentCond *conditional
	var tree *bpparser.File

	for _, node := range nodes {
		file.setMkPos(p.Unpack(node.Pos()), p.Unpack(node.End()))

		switch x := node.(type) {
		case *mkparser.Comment:
			// Split the comment on escaped newlines and then
			// add each chunk separately.
			chunks := strings.Split(x.Comment, "\\\n")
			file.insertComment("//" + chunks[0])
			for i := 1; i < len(chunks); i++ {
				file.bpPos.Line++
				file.insertComment("//" + chunks[i])
			}
		case *mkparser.Assignment:
			handleAssignment(file, x, assignmentCond)
		case *mkparser.Directive:
			switch x.Name {
			case "include", "-include":
				module, ok := mapIncludePath(x.Args.Value(file.scope))
				if !ok {
					file.errorf(x, "unsupported include")
					continue
				}
				switch module {
				case clearVarsPath:
					resetModule(file)
				case includeIgnoredPath:
					// subdirs are already automatically included in Soong
					continue
				default:
					handleModuleConditionals(file, x, conds)
					makeModule(file, module)
				}
			case "ifeq", "ifneq", "ifdef", "ifndef":
				args := x.Args.Dump()
				eq := x.Name == "ifeq" || x.Name == "ifdef"
				if _, ok := conditionalTranslations[args]; ok {
					newCond := conditional{args, eq}
					conds = append(conds, &newCond)
					if file.inModule {
						if assignmentCond == nil {
							assignmentCond = &newCond
						} else {
							file.errorf(x, "unsupported nested conditional in module")
						}
					}
				} else {
					file.errorf(x, "unsupported conditional")
					conds = append(conds, nil)
					continue
				}
			case "else":
				if len(conds) == 0 {
					file.errorf(x, "missing if before else")
					continue
				} else if conds[len(conds)-1] == nil {
					file.errorf(x, "else from unsupported conditional")
					continue
				}
				conds[len(conds)-1].eq = !conds[len(conds)-1].eq
			case "endif":
				if len(conds) == 0 {
					file.errorf(x, "missing if before endif")
					continue
				} else if conds[len(conds)-1] == nil {
					file.errorf(x, "endif from unsupported conditional")
					conds = conds[:len(conds)-1]
				} else {
					if assignmentCond == conds[len(conds)-1] {
						assignmentCond = nil
					}
					conds = conds[:len(conds)-1]
				}
			default:
				file.errorf(x, "unsupported directive")
				continue
			}
		default:
			file.errorf(x, "unsupported line")
		}
	}

	tree = &bpparser.File{
		Defs:     file.defs,
		Comments: file.comments,
	}

	// check for common supported but undesirable structures and clean them up
	fixer := bpfix.NewFixer(tree)
	fixedTree, fixerErr := fixer.Fix(bpfix.NewFixRequest().AddAll())
	if fixerErr != nil {
		errs = append(errs, fixerErr)
	} else {
		tree = fixedTree
	}

	out, err := bpparser.Print(tree)
	if err != nil {
		errs = append(errs, err)
		return "", errs
	}

	return string(out), errs
}

func renameVariableWithInvalidCharacters(name string) string {
	renamed := ""
	for invalid, replacement := range invalidVariableStringToReplacement {
		if strings.Contains(name, invalid) {
			renamed = strings.ReplaceAll(name, invalid, replacement)
		}
	}

	return renamed
}

func invalidVariableStrings() string {
	invalidStrings := make([]string, 0, len(invalidVariableStringToReplacement))
	for s := range invalidVariableStringToReplacement {
		invalidStrings = append(invalidStrings, "\""+s+"\"")
	}
	return strings.Join(invalidStrings, ", ")
}

func handleAssignment(file *bpFile, assignment *mkparser.Assignment, c *conditional) {
	if !assignment.Name.Const() {
		file.errorf(assignment, "unsupported non-const variable name")
		return
	}

	if assignment.Target != nil {
		file.errorf(assignment, "unsupported target assignment")
		return
	}

	name := assignment.Name.Value(nil)
	prefix := ""

	if newName := renameVariableWithInvalidCharacters(name); newName != "" {
		file.warnf("Variable names cannot contain: %s. Renamed \"%s\" to \"%s\"", invalidVariableStrings(), name, newName)
		file.variableRenames[name] = newName
		name = newName
	}

	if strings.HasPrefix(name, "LOCAL_") {
		for _, x := range propertyPrefixes {
			if strings.HasSuffix(name, "_"+x.mk) {
				name = strings.TrimSuffix(name, "_"+x.mk)
				prefix = x.bp
				break
			}
		}

		if c != nil {
			if prefix != "" {
				file.errorf(assignment, "prefix assignment inside conditional, skipping conditional")
			} else {
				var ok bool
				if prefix, ok = conditionalTranslations[c.cond][c.eq]; !ok {
					panic("unknown conditional")
				}
			}
		}
	} else {
		if c != nil {
			eq := "eq"
			if !c.eq {
				eq = "neq"
			}
			file.errorf(assignment, "conditional %s %s on global assignment", eq, c.cond)
		}
	}

	appendVariable := assignment.Type == "+="

	var err error
	if prop, ok := rewriteProperties[name]; ok {
		err = prop(variableAssignmentContext{file, prefix, assignment.Value, appendVariable})
	} else {
		switch {
		case name == "LOCAL_ARM_MODE":
			// This is a hack to get the LOCAL_ARM_MODE value inside
			// of an arch: { arm: {} } block.
			armModeAssign := assignment
			armModeAssign.Name = mkparser.SimpleMakeString("LOCAL_ARM_MODE_HACK_arm", assignment.Name.Pos())
			handleAssignment(file, armModeAssign, c)
		case strings.HasPrefix(name, "LOCAL_"):
			file.errorf(assignment, "unsupported assignment to %s", name)
			return
		default:
			var val bpparser.Expression
			val, err = makeVariableToBlueprint(file, assignment.Value, bpparser.ListType)
			if err == nil {
				err = setVariable(file, appendVariable, prefix, name, val, false)
			}
		}
	}
	if err != nil {
		file.errorf(assignment, err.Error())
	}
}

func handleModuleConditionals(file *bpFile, directive *mkparser.Directive, conds []*conditional) {
	for _, c := range conds {
		if c == nil {
			continue
		}

		if _, ok := conditionalTranslations[c.cond]; !ok {
			panic("unknown conditional " + c.cond)
		}

		disabledPrefix := conditionalTranslations[c.cond][!c.eq]

		// Create a fake assignment with enabled = false
		val, err := makeVariableToBlueprint(file, mkparser.SimpleMakeString("false", mkparser.NoPos), bpparser.BoolType)
		if err == nil {
			err = setVariable(file, false, disabledPrefix, "enabled", val, true)
		}
		if err != nil {
			file.errorf(directive, err.Error())
		}
	}
}

func makeModule(file *bpFile, t string) {
	file.module.Type = t
	file.module.TypePos = file.module.LBracePos
	file.module.RBracePos = file.bpPos
	file.defs = append(file.defs, file.module)
	file.inModule = false
}

func resetModule(file *bpFile) {
	file.module = &bpparser.Module{}
	file.module.LBracePos = file.bpPos
	file.localAssignments = make(map[string]*bpparser.Property)
	file.inModule = true
}

func makeVariableToBlueprint(file *bpFile, val *mkparser.MakeString,
	typ bpparser.Type) (bpparser.Expression, error) {

	var exp bpparser.Expression
	var err error
	switch typ {
	case bpparser.ListType:
		exp, err = makeToListExpression(val, file)
	case bpparser.StringType:
		exp, err = makeToStringExpression(val, file)
	case bpparser.BoolType:
		exp, err = makeToBoolExpression(val, file)
	default:
		panic("unknown type")
	}

	if err != nil {
		return nil, err
	}

	return exp, nil
}

// If local is set to true, then the variable will be added as a part of the
// variable at file.bpPos. For example, if file.bpPos references a module,
// then calling this method will set a property on that module if local is set
// to true. Otherwise, the Variable will be created at the root of the file.
//
// prefix should be populated with the top level value to be assigned, and
// name with a sub-value. If prefix is empty, then name is the top level value.
// For example, if prefix is "foo" and name is "bar" with a value of "baz", then
// the following variable will be generated:
//
//	foo {
//	  bar: "baz"
//	}
//
// If prefix is the empty string and name is "foo" with a value of "bar", the
// following variable will be generated (if it is a property):
//
// foo: "bar"
func setVariable(file *bpFile, plusequals bool, prefix, name string, value bpparser.Expression, local bool) error {
	if prefix != "" {
		name = prefix + "." + name
	}

	pos := file.bpPos

	var oldValue *bpparser.Expression
	if local {
		oldProp := file.localAssignments[name]
		if oldProp != nil {
			oldValue = &oldProp.Value
		}
	} else {
		oldValue = file.globalAssignments[name]
	}

	if local {
		if oldValue != nil && plusequals {
			val, err := addValues(*oldValue, value)
			if err != nil {
				return fmt.Errorf("unsupported addition: %s", err.Error())
			}
			val.(*bpparser.Operator).OperatorPos = pos
			*oldValue = val
		} else {
			names := strings.Split(name, ".")
			if file.module == nil {
				file.warnf("No 'include $(CLEAR_VARS)' detected before first assignment; clearing vars now")
				resetModule(file)
			}
			container := &file.module.Properties

			for i, n := range names[:len(names)-1] {
				fqn := strings.Join(names[0:i+1], ".")
				prop := file.localAssignments[fqn]
				if prop == nil {
					prop = &bpparser.Property{
						Name:    n,
						NamePos: pos,
						Value: &bpparser.Map{
							Properties: []*bpparser.Property{},
						},
					}
					file.localAssignments[fqn] = prop
					*container = append(*container, prop)
				}
				container = &prop.Value.(*bpparser.Map).Properties
			}

			prop := &bpparser.Property{
				Name:    names[len(names)-1],
				NamePos: pos,
				Value:   value,
			}
			file.localAssignments[name] = prop
			*container = append(*container, prop)
		}
	} else {
		if oldValue != nil && plusequals {
			a := &bpparser.Assignment{
				Name:      name,
				NamePos:   pos,
				Value:     value,
				EqualsPos: pos,
				Assigner:  "+=",
			}
			file.defs = append(file.defs, a)
		} else {
			if _, ok := file.globalAssignments[name]; ok {
				return fmt.Errorf("cannot assign a variable multiple times: \"%s\"", name)
			}
			a := &bpparser.Assignment{
				Name:      name,
				NamePos:   pos,
				Value:     value,
				EqualsPos: pos,
				Assigner:  "=",
			}
			file.globalAssignments[name] = &a.Value
			file.defs = append(file.defs, a)
		}
	}
	return nil
}
