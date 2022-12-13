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

import "fmt"

// Starlark expression types we use
type starlarkType int

const (
	// Variable types. Initially we only know the types of the  product
	// configuration variables that are lists, and the types of some
	// hardwired variables. The remaining variables are first entered as
	// having an unknown type and treated as strings, but sometimes we
	//  can infer variable's type from the value assigned to it.
	starlarkTypeUnknown starlarkType = iota
	starlarkTypeList    starlarkType = iota
	starlarkTypeString  starlarkType = iota
	starlarkTypeInt     starlarkType = iota
	starlarkTypeBool    starlarkType = iota
	starlarkTypeVoid    starlarkType = iota
)

func (t starlarkType) String() string {
	switch t {
	case starlarkTypeList:
		return "list"
	case starlarkTypeString:
		return "string"
	case starlarkTypeInt:
		return "int"
	case starlarkTypeBool:
		return "bool"
	case starlarkTypeVoid:
		return "void"
	case starlarkTypeUnknown:
		return "unknown"
	default:
		panic(fmt.Sprintf("Unknown starlark type %d", t))
	}
}

type hiddenArgType int

const (
	// Some functions have an implicitly emitted first argument, which may be
	// a global ('g') or configuration ('cfg') variable.
	hiddenArgNone   hiddenArgType = iota
	hiddenArgGlobal hiddenArgType = iota
	hiddenArgConfig hiddenArgType = iota
)

type varClass int

const (
	VarClassConfig varClass = iota
	VarClassSoong  varClass = iota
	VarClassLocal  varClass = iota
)

type variableRegistrar interface {
	NewVariable(name string, varClass varClass, valueType starlarkType)
}

// ScopeBase is a placeholder implementation of the mkparser.Scope.
// All our scopes are read-only and resolve only simple variables.
type ScopeBase struct{}

func (s ScopeBase) Set(_, _ string) {
	panic("implement me")
}

func (s ScopeBase) Call(_ string, _ []string) []string {
	panic("implement me")
}

func (s ScopeBase) SetFunc(_ string, _ func([]string) []string) {
	panic("implement me")
}

// Used to find all makefiles in the source tree
type MakefileFinder interface {
	Find(root string) []string
}
