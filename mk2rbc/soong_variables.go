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
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	mkparser "android/soong/androidmk/parser"
)

type context struct {
	includeFileScope mkparser.Scope
	registrar        variableRegistrar
}

// Scans the makefile Soong uses to generate soong.variables file,
// collecting variable names and types from the lines that look like this:
//
//	$(call add_json_XXX,  <...>,             $(VAR))
func FindSoongVariables(mkFile string, includeFileScope mkparser.Scope, registrar variableRegistrar) error {
	ctx := context{includeFileScope, registrar}
	return ctx.doFind(mkFile)
}

func (ctx *context) doFind(mkFile string) error {
	mkContents, err := ioutil.ReadFile(mkFile)
	if err != nil {
		return err
	}
	parser := mkparser.NewParser(mkFile, bytes.NewBuffer(mkContents))
	nodes, errs := parser.Parse()
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "ERROR:", e)
		}
		return fmt.Errorf("cannot parse %s", mkFile)
	}
	for _, node := range nodes {
		switch t := node.(type) {
		case *mkparser.Variable:
			ctx.handleVariable(t)
		case *mkparser.Directive:
			ctx.handleInclude(t)
		}
	}
	return nil
}

func (ctx context) NewSoongVariable(name, typeString string) {
	var valueType starlarkType
	switch typeString {
	case "bool":
		valueType = starlarkTypeBool
	case "csv":
		// Only PLATFORM_VERSION_ALL_CODENAMES, and it's a list
		valueType = starlarkTypeList
	case "list":
		valueType = starlarkTypeList
	case "str":
		valueType = starlarkTypeString
	case "val":
		// Only PLATFORM_SDK_VERSION uses this, and it's integer
		valueType = starlarkTypeInt
	default:
		panic(fmt.Errorf("unknown Soong variable type %s", typeString))
	}

	ctx.registrar.NewVariable(name, VarClassSoong, valueType)
}

func (ctx context) handleInclude(t *mkparser.Directive) {
	if t.Name != "include" && t.Name != "-include" {
		return
	}
	includedPath := t.Args.Value(ctx.includeFileScope)
	err := ctx.doFind(includedPath)
	if err != nil && t.Name == "include" {
		fmt.Fprintf(os.Stderr, "cannot include %s: %s", includedPath, err)
	}
}

var callFuncRex = regexp.MustCompile("^call +add_json_(str|val|bool|csv|list) *,")

func (ctx context) handleVariable(t *mkparser.Variable) {
	// From the variable reference looking as follows:
	//  $(call json_add_TYPE,arg1,$(VAR))
	// we infer that the type of $(VAR) is TYPE
	// VAR can be a simple variable name, or another call
	// (e.g., $(call invert_bool, $(X)), from which we can infer
	// that the type of X is bool
	if prefix, v, ok := prefixedVariable(t.Name); ok && strings.HasPrefix(prefix, "call add_json") {
		if match := callFuncRex.FindStringSubmatch(prefix); match != nil {
			ctx.inferSoongVariableType(match[1], v)
			// NOTE(asmundak): sometimes arg1 (the name of the Soong variable defined
			// in this statement) may indicate that there is a Make counterpart. E.g, from
			//     $(call add_json_bool, DisablePreopt, $(call invert_bool,$(ENABLE_PREOPT)))
			// it may be inferred that there is a Make boolean variable DISABLE_PREOPT.
			// Unfortunately, Soong variable names have no 1:1 correspondence to Make variables,
			// for instance,
			//       $(call add_json_list, PatternsOnSystemOther, $(SYSTEM_OTHER_ODEX_FILTER))
			// does not mean that there is PATTERNS_ON_SYSTEM_OTHER
			// Our main interest lies in finding the variables whose values are lists, and
			// so far there are none that can be found this way, so it is not important.
		} else {
			panic(fmt.Errorf("cannot match the call: %s", prefix))
		}
	}
}

var (
	callInvertBoolRex = regexp.MustCompile("^call +invert_bool *, *$")
	callFilterBoolRex = regexp.MustCompile("^(filter|filter-out) +(true|false), *$")
)

func (ctx context) inferSoongVariableType(vType string, n *mkparser.MakeString) {
	if n.Const() {
		ctx.NewSoongVariable(n.Strings[0], vType)
		return
	}
	if prefix, v, ok := prefixedVariable(n); ok {
		if callInvertBoolRex.MatchString(prefix) || callFilterBoolRex.MatchString(prefix) {
			// It is $(call invert_bool, $(VAR)) or $(filter[-out] [false|true],$(VAR))
			ctx.inferSoongVariableType("bool", v)
		}
	}
}

// If MakeString is foo$(BAR), returns 'foo', BAR(as *MakeString) and true
func prefixedVariable(s *mkparser.MakeString) (string, *mkparser.MakeString, bool) {
	if len(s.Strings) != 2 || s.Strings[1] != "" {
		return "", nil, false
	}
	return s.Strings[0], s.Variables[0].Name, true
}
