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
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"android/soong/bazel"
	"android/soong/starlark_fmt"

	"github.com/google/blueprint"
)

// BazelVarExporter is a collection of configuration variables that can be exported for use in Bazel rules
type BazelVarExporter interface {
	// asBazel expands strings of configuration variables into their concrete values
	asBazel(Config, ExportedStringVariables, ExportedStringListVariables, ExportedConfigDependingVariables) []bazelConstant
}

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

func (ev ExportedVariables) asBazel(config Config,
	stringVars ExportedStringVariables, stringListVars ExportedStringListVariables, cfgDepVars ExportedConfigDependingVariables) []bazelConstant {
	ret := []bazelConstant{}
	ret = append(ret, ev.exportedStringVars.asBazel(config, stringVars, stringListVars, cfgDepVars)...)
	ret = append(ret, ev.exportedStringListVars.asBazel(config, stringVars, stringListVars, cfgDepVars)...)
	ret = append(ret, ev.exportedStringListDictVars.asBazel(config, stringVars, stringListVars, cfgDepVars)...)
	// Note: ExportedVariableReferenceDictVars collections can only contain references to other variables and must be printed last
	ret = append(ret, ev.exportedVariableReferenceDictVars.asBazel(config, stringVars, stringListVars, cfgDepVars)...)
	ret = append(ret, ev.exportedConfigDependingVars.asBazel(config, stringVars, stringListVars, cfgDepVars)...)
	return ret
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

func (m ExportedConfigDependingVariables) asBazel(config Config,
	stringVars ExportedStringVariables, stringListVars ExportedStringListVariables, cfgDepVars ExportedConfigDependingVariables) []bazelConstant {
	ret := make([]bazelConstant, 0, len(m))
	for variable, unevaluatedVar := range m {
		evalFunc := reflect.ValueOf(unevaluatedVar)
		validateVariableMethod(variable, evalFunc)
		evaluatedResult := evalFunc.Call([]reflect.Value{reflect.ValueOf(config)})
		evaluatedValue := evaluatedResult[0].Interface().(string)
		expandedVars, err := expandVar(config, evaluatedValue, stringVars, stringListVars, cfgDepVars)
		if err != nil {
			panic(fmt.Errorf("error expanding config variable %s: %s", variable, err))
		}
		if len(expandedVars) > 1 {
			ret = append(ret, bazelConstant{
				variableName:       variable,
				internalDefinition: starlark_fmt.PrintStringList(expandedVars, 0),
			})
		} else {
			ret = append(ret, bazelConstant{
				variableName:       variable,
				internalDefinition: fmt.Sprintf(`"%s"`, validateCharacters(expandedVars[0])),
			})
		}
	}
	return ret
}

// Ensure that string s has no invalid characters to be generated into the bzl file.
func validateCharacters(s string) string {
	for _, c := range []string{`\n`, `"`, `\`} {
		if strings.Contains(s, c) {
			panic(fmt.Errorf("%s contains illegal character %s", s, c))
		}
	}
	return s
}

type bazelConstant struct {
	variableName       string
	internalDefinition string
	sortLast           bool
}

// ExportedStringVariables is a mapping of variable names to string values
type ExportedStringVariables map[string]string

func (m ExportedStringVariables) set(k string, v string) {
	m[k] = v
}

func (m ExportedStringVariables) asBazel(config Config,
	stringVars ExportedStringVariables, stringListVars ExportedStringListVariables, cfgDepVars ExportedConfigDependingVariables) []bazelConstant {
	ret := make([]bazelConstant, 0, len(m))
	for k, variableValue := range m {
		expandedVar, err := expandVar(config, variableValue, stringVars, stringListVars, cfgDepVars)
		if err != nil {
			panic(fmt.Errorf("error expanding config variable %s: %s", k, err))
		}
		if len(expandedVar) > 1 {
			panic(fmt.Errorf("%q expands to more than one string value: %q", variableValue, expandedVar))
		}
		ret = append(ret, bazelConstant{
			variableName:       k,
			internalDefinition: fmt.Sprintf(`"%s"`, validateCharacters(expandedVar[0])),
		})
	}
	return ret
}

// ExportedStringListVariables is a mapping of variable names to a list of strings
type ExportedStringListVariables map[string][]string

func (m ExportedStringListVariables) set(k string, v []string) {
	m[k] = v
}

func (m ExportedStringListVariables) asBazel(config Config,
	stringScope ExportedStringVariables, stringListScope ExportedStringListVariables,
	exportedVars ExportedConfigDependingVariables) []bazelConstant {
	ret := make([]bazelConstant, 0, len(m))
	// For each exported variable, recursively expand elements in the variableValue
	// list to ensure that interpolated variables are expanded according to their values
	// in the variable scope.
	for k, variableValue := range m {
		var expandedVars []string
		for _, v := range variableValue {
			expandedVar, err := expandVar(config, v, stringScope, stringListScope, exportedVars)
			if err != nil {
				panic(fmt.Errorf("Error expanding config variable %s=%s: %s", k, v, err))
			}
			expandedVars = append(expandedVars, expandedVar...)
		}
		// Assign the list as a bzl-private variable; this variable will be exported
		// out through a constants struct later.
		ret = append(ret, bazelConstant{
			variableName:       k,
			internalDefinition: starlark_fmt.PrintStringList(expandedVars, 0),
		})
	}
	return ret
}

// ExportedStringListDictVariables is a mapping from variable names to a
// dictionary which maps keys to lists of strings
type ExportedStringListDictVariables map[string]map[string][]string

func (m ExportedStringListDictVariables) set(k string, v map[string][]string) {
	m[k] = v
}

// Since dictionaries are not supported in Ninja, we do not expand variables for dictionaries
func (m ExportedStringListDictVariables) asBazel(_ Config, _ ExportedStringVariables,
	_ ExportedStringListVariables, _ ExportedConfigDependingVariables) []bazelConstant {
	ret := make([]bazelConstant, 0, len(m))
	for k, dict := range m {
		ret = append(ret, bazelConstant{
			variableName:       k,
			internalDefinition: starlark_fmt.PrintStringListDict(dict, 0),
		})
	}
	return ret
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

func (m ExportedVariableReferenceDictVariables) asBazel(_ Config, _ ExportedStringVariables,
	_ ExportedStringListVariables, _ ExportedConfigDependingVariables) []bazelConstant {
	ret := make([]bazelConstant, 0, len(m))
	for n, dict := range m {
		for k, v := range dict {
			matches, err := variableReference(v)
			if err != nil {
				panic(err)
			} else if !matches.matches {
				panic(fmt.Errorf("Expected a variable reference, got %q", v))
			} else if len(matches.fullVariableReference) != len(v) {
				panic(fmt.Errorf("Expected only a variable reference, got %q", v))
			}
			dict[k] = "_" + matches.variable
		}
		ret = append(ret, bazelConstant{
			variableName:       n,
			internalDefinition: starlark_fmt.PrintDict(dict, 0),
			sortLast:           true,
		})
	}
	return ret
}

// BazelToolchainVars expands an ExportedVariables collection and returns a string
// of formatted Starlark variable definitions
func BazelToolchainVars(config Config, exportedVars ExportedVariables) string {
	results := exportedVars.asBazel(
		config,
		exportedVars.exportedStringVars,
		exportedVars.exportedStringListVars,
		exportedVars.exportedConfigDependingVars,
	)

	sort.Slice(results, func(i, j int) bool {
		if results[i].sortLast != results[j].sortLast {
			return !results[i].sortLast
		}
		return results[i].variableName < results[j].variableName
	})

	definitions := make([]string, 0, len(results))
	constants := make([]string, 0, len(results))
	for _, b := range results {
		definitions = append(definitions,
			fmt.Sprintf("_%s = %s", b.variableName, b.internalDefinition))
		constants = append(constants,
			fmt.Sprintf("%[1]s%[2]s = _%[2]s,", starlark_fmt.Indention(1), b.variableName))
	}

	// Build the exported constants struct.
	ret := bazel.GeneratedBazelFileWarning
	ret += "\n\n"
	ret += strings.Join(definitions, "\n\n")
	ret += "\n\n"
	ret += "constants = struct(\n"
	ret += strings.Join(constants, "\n")
	ret += "\n)"

	return ret
}

type match struct {
	matches               bool
	fullVariableReference string
	variable              string
}

func variableReference(input string) (match, error) {
	// e.g. "${ExternalCflags}"
	r := regexp.MustCompile(`\${(?:config\.)?([a-zA-Z0-9_]+)}`)

	matches := r.FindStringSubmatch(input)
	if len(matches) == 0 {
		return match{}, nil
	}
	if len(matches) != 2 {
		return match{}, fmt.Errorf("Expected to only match 1 subexpression in %s, got %d", input, len(matches)-1)
	}
	return match{
		matches:               true,
		fullVariableReference: matches[0],
		// Index 1 of FindStringSubmatch contains the subexpression match
		// (variable name) of the capture group.
		variable: matches[1],
	}, nil
}

// expandVar recursively expand interpolated variables in the exportedVars scope.
//
// We're using a string slice to track the seen variables to avoid
// stackoverflow errors with infinite recursion. it's simpler to use a
// string slice than to handle a pass-by-referenced map, which would make it
// quite complex to track depth-first interpolations. It's also unlikely the
// interpolation stacks are deep (n > 1).
func expandVar(config Config, toExpand string, stringScope ExportedStringVariables,
	stringListScope ExportedStringListVariables, exportedVars ExportedConfigDependingVariables) ([]string, error) {

	// Internal recursive function.
	var expandVarInternal func(string, map[string]bool) (string, error)
	expandVarInternal = func(toExpand string, seenVars map[string]bool) (string, error) {
		var ret string
		remainingString := toExpand
		for len(remainingString) > 0 {
			matches, err := variableReference(remainingString)
			if err != nil {
				panic(err)
			}
			if !matches.matches {
				return ret + remainingString, nil
			}
			matchIndex := strings.Index(remainingString, matches.fullVariableReference)
			ret += remainingString[:matchIndex]
			remainingString = remainingString[matchIndex+len(matches.fullVariableReference):]

			variable := matches.variable
			// toExpand contains a variable.
			if _, ok := seenVars[variable]; ok {
				return ret, fmt.Errorf(
					"Unbounded recursive interpolation of variable: %s", variable)
			}
			// A map is passed-by-reference. Create a new map for
			// this scope to prevent variables seen in one depth-first expansion
			// to be also treated as "seen" in other depth-first traversals.
			newSeenVars := map[string]bool{}
			for k := range seenVars {
				newSeenVars[k] = true
			}
			newSeenVars[variable] = true
			if unexpandedVars, ok := stringListScope[variable]; ok {
				expandedVars := []string{}
				for _, unexpandedVar := range unexpandedVars {
					expandedVar, err := expandVarInternal(unexpandedVar, newSeenVars)
					if err != nil {
						return ret, err
					}
					expandedVars = append(expandedVars, expandedVar)
				}
				ret += strings.Join(expandedVars, " ")
			} else if unexpandedVar, ok := stringScope[variable]; ok {
				expandedVar, err := expandVarInternal(unexpandedVar, newSeenVars)
				if err != nil {
					return ret, err
				}
				ret += expandedVar
			} else if unevaluatedVar, ok := exportedVars[variable]; ok {
				evalFunc := reflect.ValueOf(unevaluatedVar)
				validateVariableMethod(variable, evalFunc)
				evaluatedResult := evalFunc.Call([]reflect.Value{reflect.ValueOf(config)})
				evaluatedValue := evaluatedResult[0].Interface().(string)
				expandedVar, err := expandVarInternal(evaluatedValue, newSeenVars)
				if err != nil {
					return ret, err
				}
				ret += expandedVar
			} else {
				return "", fmt.Errorf("Unbound config variable %s", variable)
			}
		}
		return ret, nil
	}
	var ret []string
	stringFields := splitStringKeepingQuotedSubstring(toExpand, ' ')
	for _, v := range stringFields {
		val, err := expandVarInternal(v, map[string]bool{})
		if err != nil {
			return ret, err
		}
		ret = append(ret, val)
	}

	return ret, nil
}

// splitStringKeepingQuotedSubstring splits a string on a provided separator,
// but it will not split substrings inside unescaped double quotes. If the double
// quotes are escaped, then the returned string will only include the quote, and
// not the escape.
func splitStringKeepingQuotedSubstring(s string, delimiter byte) []string {
	var ret []string
	quote := byte('"')

	var substring []byte
	quoted := false
	escaped := false

	for i := range s {
		if !quoted && s[i] == delimiter {
			ret = append(ret, string(substring))
			substring = []byte{}
			continue
		}

		characterIsEscape := i < len(s)-1 && s[i] == '\\' && s[i+1] == quote
		if characterIsEscape {
			escaped = true
			continue
		}

		if s[i] == quote {
			if !escaped {
				quoted = !quoted
			}
			escaped = false
		}

		substring = append(substring, s[i])
	}

	ret = append(ret, string(substring))

	return ret
}

func validateVariableMethod(name string, methodValue reflect.Value) {
	methodType := methodValue.Type()
	if methodType.Kind() != reflect.Func {
		panic(fmt.Errorf("method given for variable %s is not a function",
			name))
	}
	if n := methodType.NumIn(); n != 1 {
		panic(fmt.Errorf("method for variable %s has %d inputs (should be 1)",
			name, n))
	}
	if n := methodType.NumOut(); n != 1 {
		panic(fmt.Errorf("method for variable %s has %d outputs (should be 1)",
			name, n))
	}
	if kind := methodType.Out(0).Kind(); kind != reflect.String {
		panic(fmt.Errorf("method for variable %s does not return a string",
			name))
	}
}
