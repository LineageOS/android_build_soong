// Copyright 2021 Google Inc. All rights reserved.
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

package android

import (
	"strings"

	"github.com/google/blueprint"
)

// ExportedVariables is a collection of interdependent configuration variables
type ExportedVariables struct {
	// Maps containing toolchain variables that are independent of the
	// environment variables of the build.
	exportedStringVars         ExportedStringVariables
	exportedStringListVars     ExportedStringListVariables
	exportedStringListDictVars ExportedStringListDictVariables

	exportedVariableReferenceDictVars ExportedVariableReferenceDictVariables

	/// Maps containing variables that are dependent on the build config.
	exportedConfigDependingVars ExportedConfigDependingVariables

	pctx PackageContext
}

// NewExportedVariables creats an empty ExportedVariables struct with non-nil maps
func NewExportedVariables(pctx PackageContext) ExportedVariables {
	return ExportedVariables{
		exportedStringVars:                ExportedStringVariables{},
		exportedStringListVars:            ExportedStringListVariables{},
		exportedStringListDictVars:        ExportedStringListDictVariables{},
		exportedVariableReferenceDictVars: ExportedVariableReferenceDictVariables{},
		exportedConfigDependingVars:       ExportedConfigDependingVariables{},
		pctx:                              pctx,
	}
}

// ExportStringStaticVariable declares a static string variable and exports it to
// Bazel's toolchain.
func (ev ExportedVariables) ExportStringStaticVariable(name string, value string) {
	ev.pctx.StaticVariable(name, value)
	ev.exportedStringVars.set(name, value)
}

// ExportStringListStaticVariable declares a static variable and exports it to
// Bazel's toolchain.
func (ev ExportedVariables) ExportStringListStaticVariable(name string, value []string) {
	ev.pctx.StaticVariable(name, strings.Join(value, " "))
	ev.exportedStringListVars.set(name, value)
}

// ExportVariableConfigMethod declares a variable whose value is evaluated at
// runtime via a function with access to the Config and exports it to Bazel's
// toolchain.
func (ev ExportedVariables) ExportVariableConfigMethod(name string, method interface{}) blueprint.Variable {
	ev.exportedConfigDependingVars.set(name, method)
	return ev.pctx.VariableConfigMethod(name, method)
}

// ExportSourcePathVariable declares a static "source path" variable and exports
// it to Bazel's toolchain.
func (ev ExportedVariables) ExportSourcePathVariable(name string, value string) {
	ev.pctx.SourcePathVariable(name, value)
	ev.exportedStringVars.set(name, value)
}

// ExportVariableFuncVariable declares a variable whose value is evaluated at
// runtime via a function and exports it to Bazel's toolchain.
func (ev ExportedVariables) ExportVariableFuncVariable(name string, f func() string) {
	ev.exportedConfigDependingVars.set(name, func(config Config) string {
		return f()
	})
	ev.pctx.VariableFunc(name, func(PackageVarContext) string {
		return f()
	})
}

// ExportString only exports a variable to Bazel, but does not declare it in Soong
func (ev ExportedVariables) ExportString(name string, value string) {
	ev.exportedStringVars.set(name, value)
}

// ExportStringList only exports a variable to Bazel, but does not declare it in Soong
func (ev ExportedVariables) ExportStringList(name string, value []string) {
	ev.exportedStringListVars.set(name, value)
}

// ExportStringListDict only exports a variable to Bazel, but does not declare it in Soong
func (ev ExportedVariables) ExportStringListDict(name string, value map[string][]string) {
	ev.exportedStringListDictVars.set(name, value)
}

// ExportVariableReferenceDict only exports a variable to Bazel, but does not declare it in Soong
func (ev ExportedVariables) ExportVariableReferenceDict(name string, value map[string]string) {
	ev.exportedVariableReferenceDictVars.set(name, value)
}

// ExportedConfigDependingVariables is a mapping of variable names to functions
// of type func(config Config) string which return the runtime-evaluated string
// value of a particular variable
type ExportedConfigDependingVariables map[string]interface{}

func (m ExportedConfigDependingVariables) set(k string, v interface{}) {
	m[k] = v
}

// ExportedStringVariables is a mapping of variable names to string values
type ExportedStringVariables map[string]string

func (m ExportedStringVariables) set(k string, v string) {
	m[k] = v
}

// ExportedStringListVariables is a mapping of variable names to a list of strings
type ExportedStringListVariables map[string][]string

func (m ExportedStringListVariables) set(k string, v []string) {
	m[k] = v
}

// ExportedStringListDictVariables is a mapping from variable names to a
// dictionary which maps keys to lists of strings
type ExportedStringListDictVariables map[string]map[string][]string

func (m ExportedStringListDictVariables) set(k string, v map[string][]string) {
	m[k] = v
}

// ExportedVariableReferenceDictVariables is a mapping from variable names to a
// dictionary which references previously defined variables. This is used to
// create a Starlark output such as:
//
//	string_var1 = "string1
//	var_ref_dict_var1 = {
//		"key1": string_var1
//	}
//
// This type of variable collection must be expanded last so that it recognizes
// previously defined variables.
type ExportedVariableReferenceDictVariables map[string]map[string]string

func (m ExportedVariableReferenceDictVariables) set(k string, v map[string]string) {
	m[k] = v
}
