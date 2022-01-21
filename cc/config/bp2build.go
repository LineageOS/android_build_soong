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

package config

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"android/soong/android"

	"github.com/google/blueprint"
)

const (
	bazelIndent = 4
)

type bazelVarExporter interface {
	asBazel(android.Config, exportedStringVariables, exportedStringListVariables, exportedConfigDependingVariables) []bazelConstant
}

// Helpers for exporting cc configuration information to Bazel.
var (
	// Maps containing toolchain variables that are independent of the
	// environment variables of the build.
	exportedStringListVars     = exportedStringListVariables{}
	exportedStringVars         = exportedStringVariables{}
	exportedStringListDictVars = exportedStringListDictVariables{}

	/// Maps containing variables that are dependent on the build config.
	exportedConfigDependingVars = exportedConfigDependingVariables{}
)

type exportedConfigDependingVariables map[string]interface{}

func (m exportedConfigDependingVariables) Set(k string, v interface{}) {
	m[k] = v
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
}

type exportedStringVariables map[string]string

func (m exportedStringVariables) Set(k string, v string) {
	m[k] = v
}

func bazelIndention(level int) string {
	return strings.Repeat(" ", level*bazelIndent)
}

func printBazelList(items []string, indentLevel int) string {
	list := make([]string, 0, len(items)+2)
	list = append(list, "[")
	innerIndent := bazelIndention(indentLevel + 1)
	for _, item := range items {
		list = append(list, fmt.Sprintf(`%s"%s",`, innerIndent, item))
	}
	list = append(list, bazelIndention(indentLevel)+"]")
	return strings.Join(list, "\n")
}

func (m exportedStringVariables) asBazel(config android.Config,
	stringVars exportedStringVariables, stringListVars exportedStringListVariables, cfgDepVars exportedConfigDependingVariables) []bazelConstant {
	ret := make([]bazelConstant, 0, len(m))
	for k, variableValue := range m {
		expandedVar, err := expandVar(config, variableValue, stringVars, stringListVars, cfgDepVars)
		if err != nil {
			panic(fmt.Errorf("error expanding config variable %s: %s", k, err))
		}
		if len(expandedVar) > 1 {
			panic(fmt.Errorf("%s expands to more than one string value: %s", variableValue, expandedVar))
		}
		ret = append(ret, bazelConstant{
			variableName:       k,
			internalDefinition: fmt.Sprintf(`"%s"`, validateCharacters(expandedVar[0])),
		})
	}
	return ret
}

// Convenience function to declare a static variable and export it to Bazel's cc_toolchain.
func exportStringStaticVariable(name string, value string) {
	pctx.StaticVariable(name, value)
	exportedStringVars.Set(name, value)
}

type exportedStringListVariables map[string][]string

func (m exportedStringListVariables) Set(k string, v []string) {
	m[k] = v
}

func (m exportedStringListVariables) asBazel(config android.Config,
	stringScope exportedStringVariables, stringListScope exportedStringListVariables,
	exportedVars exportedConfigDependingVariables) []bazelConstant {
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
			internalDefinition: printBazelList(expandedVars, 0),
		})
	}
	return ret
}

// Convenience function to declare a static "source path" variable and export it to Bazel's cc_toolchain.
func exportVariableConfigMethod(name string, method interface{}) blueprint.Variable {
	exportedConfigDependingVars.Set(name, method)
	return pctx.VariableConfigMethod(name, method)
}

// Convenience function to declare a static "source path" variable and export it to Bazel's cc_toolchain.
func exportSourcePathVariable(name string, value string) {
	pctx.SourcePathVariable(name, value)
	exportedStringVars.Set(name, value)
}

// Convenience function to declare a static variable and export it to Bazel's cc_toolchain.
func exportStringListStaticVariable(name string, value []string) {
	pctx.StaticVariable(name, strings.Join(value, " "))
	exportedStringListVars.Set(name, value)
}

func ExportStringList(name string, value []string) {
	exportedStringListVars.Set(name, value)
}

type exportedStringListDictVariables map[string]map[string][]string

func (m exportedStringListDictVariables) Set(k string, v map[string][]string) {
	m[k] = v
}

func printBazelStringListDict(dict map[string][]string) string {
	bazelDict := make([]string, 0, len(dict)+2)
	bazelDict = append(bazelDict, "{")
	for k, v := range dict {
		bazelDict = append(bazelDict,
			fmt.Sprintf(`%s"%s": %s,`, bazelIndention(1), k, printBazelList(v, 1)))
	}
	bazelDict = append(bazelDict, "}")
	return strings.Join(bazelDict, "\n")
}

// Since dictionaries are not supported in Ninja, we do not expand variables for dictionaries
func (m exportedStringListDictVariables) asBazel(_ android.Config, _ exportedStringVariables,
	_ exportedStringListVariables, _ exportedConfigDependingVariables) []bazelConstant {
	ret := make([]bazelConstant, 0, len(m))
	for k, dict := range m {
		ret = append(ret, bazelConstant{
			variableName:       k,
			internalDefinition: printBazelStringListDict(dict),
		})
	}
	return ret
}

// BazelCcToolchainVars generates bzl file content containing variables for
// Bazel's cc_toolchain configuration.
func BazelCcToolchainVars(config android.Config) string {
	return bazelToolchainVars(
		config,
		exportedStringListDictVars,
		exportedStringListVars,
		exportedStringVars)
}

func bazelToolchainVars(config android.Config, vars ...bazelVarExporter) string {
	ret := "# GENERATED FOR BAZEL FROM SOONG. DO NOT EDIT.\n\n"

	results := []bazelConstant{}
	for _, v := range vars {
		results = append(results, v.asBazel(config, exportedStringVars, exportedStringListVars, exportedConfigDependingVars)...)
	}

	sort.Slice(results, func(i, j int) bool { return results[i].variableName < results[j].variableName })

	definitions := make([]string, 0, len(results))
	constants := make([]string, 0, len(results))
	for _, b := range results {
		definitions = append(definitions,
			fmt.Sprintf("_%s = %s", b.variableName, b.internalDefinition))
		constants = append(constants,
			fmt.Sprintf("%[1]s%[2]s = _%[2]s,", bazelIndention(1), b.variableName))
	}

	// Build the exported constants struct.
	ret += strings.Join(definitions, "\n\n")
	ret += "\n\n"
	ret += "constants = struct(\n"
	ret += strings.Join(constants, "\n")
	ret += "\n)"

	return ret
}

// expandVar recursively expand interpolated variables in the exportedVars scope.
//
// We're using a string slice to track the seen variables to avoid
// stackoverflow errors with infinite recursion. it's simpler to use a
// string slice than to handle a pass-by-referenced map, which would make it
// quite complex to track depth-first interpolations. It's also unlikely the
// interpolation stacks are deep (n > 1).
func expandVar(config android.Config, toExpand string, stringScope exportedStringVariables,
	stringListScope exportedStringListVariables, exportedVars exportedConfigDependingVariables) ([]string, error) {
	// e.g. "${ExternalCflags}"
	r := regexp.MustCompile(`\${([a-zA-Z0-9_]+)}`)

	// Internal recursive function.
	var expandVarInternal func(string, map[string]bool) (string, error)
	expandVarInternal = func(toExpand string, seenVars map[string]bool) (string, error) {
		var ret string
		remainingString := toExpand
		for len(remainingString) > 0 {
			matches := r.FindStringSubmatch(remainingString)
			if len(matches) == 0 {
				return ret + remainingString, nil
			}
			if len(matches) != 2 {
				panic(fmt.Errorf("Expected to only match 1 subexpression in %s, got %d", remainingString, len(matches)-1))
			}
			matchIndex := strings.Index(remainingString, matches[0])
			ret += remainingString[:matchIndex]
			remainingString = remainingString[matchIndex+len(matches[0]):]

			// Index 1 of FindStringSubmatch contains the subexpression match
			// (variable name) of the capture group.
			variable := matches[1]
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
	for _, v := range strings.Split(toExpand, " ") {
		val, err := expandVarInternal(v, map[string]bool{})
		if err != nil {
			return ret, err
		}
		ret = append(ret, val)
	}

	return ret, nil
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
