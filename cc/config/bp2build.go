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
	"android/soong/android"
	"fmt"
	"regexp"
	"strings"
)

// Helpers for exporting cc configuration information to Bazel.
var (
	// Map containing toolchain variables that are independent of the
	// environment variables of the build.
	exportedStringListVars = exportedStringListVariables{}
	exportedStringVars     = exportedStringVariables{}
)

type exportedStringVariables map[string]string
type exportedStringListVariables map[string][]string

func (m exportedStringVariables) Set(k string, v string) {
	m[k] = v
}

// Convenience function to declare a static variable and export it to Bazel's cc_toolchain.
func exportStringStaticVariable(name string, value string) {
	pctx.StaticVariable(name, value)
	exportedStringVars.Set(name, value)
}

func (m exportedStringListVariables) Set(k string, v []string) {
	m[k] = v
}

// Convenience function to declare a static variable and export it to Bazel's cc_toolchain.
func exportStringListStaticVariable(name string, value []string) {
	pctx.StaticVariable(name, strings.Join(value, " "))
	exportedStringListVars.Set(name, value)
}

// BazelCcToolchainVars generates bzl file content containing variables for
// Bazel's cc_toolchain configuration.
func BazelCcToolchainVars() string {
	ret := "# GENERATED FOR BAZEL FROM SOONG. DO NOT EDIT.\n\n"

	// Ensure that string s has no invalid characters to be generated into the bzl file.
	validateCharacters := func(s string) string {
		for _, c := range []string{`\n`, `"`, `\`} {
			if strings.Contains(s, c) {
				panic(fmt.Errorf("%s contains illegal character %s", s, c))
			}
		}
		return s
	}

	// For each exported variable, recursively expand elements in the variableValue
	// list to ensure that interpolated variables are expanded according to their values
	// in the variable scope.
	for _, k := range android.SortedStringKeys(exportedStringListVars) {
		variableValue := exportedStringListVars[k]
		var expandedVars []string
		for _, v := range variableValue {
			expandedVars = append(expandedVars, expandVar(v, exportedStringVars, exportedStringListVars)...)
		}
		// Build the list for this variable.
		list := "["
		for _, flag := range expandedVars {
			list += fmt.Sprintf("\n    \"%s\",", validateCharacters(flag))
		}
		list += "\n]"
		// Assign the list as a bzl-private variable; this variable will be exported
		// out through a constants struct later.
		ret += fmt.Sprintf("_%s = %s\n", k, list)
		ret += "\n"
	}

	for _, k := range android.SortedStringKeys(exportedStringVars) {
		variableValue := exportedStringVars[k]
		expandedVar := expandVar(variableValue, exportedStringVars, exportedStringListVars)
		if len(expandedVar) > 1 {
			panic(fmt.Errorf("%s expands to more than one string value: %s", variableValue, expandedVar))
		}
		ret += fmt.Sprintf("_%s = \"%s\"\n", k, validateCharacters(expandedVar[0]))
		ret += "\n"
	}

	// Build the exported constants struct.
	ret += "constants = struct(\n"
	for _, k := range android.SortedStringKeys(exportedStringVars) {
		ret += fmt.Sprintf("    %s = _%s,\n", k, k)
	}
	for _, k := range android.SortedStringKeys(exportedStringListVars) {
		ret += fmt.Sprintf("    %s = _%s,\n", k, k)
	}
	ret += ")"
	return ret
}

// expandVar recursively expand interpolated variables in the exportedVars scope.
//
// We're using a string slice to track the seen variables to avoid
// stackoverflow errors with infinite recursion. it's simpler to use a
// string slice than to handle a pass-by-referenced map, which would make it
// quite complex to track depth-first interpolations. It's also unlikely the
// interpolation stacks are deep (n > 1).
func expandVar(toExpand string, stringScope exportedStringVariables, stringListScope exportedStringListVariables) []string {
	// e.g. "${ClangExternalCflags}"
	r := regexp.MustCompile(`\${([a-zA-Z0-9_]+)}`)

	// Internal recursive function.
	var expandVarInternal func(string, map[string]bool) []string
	expandVarInternal = func(toExpand string, seenVars map[string]bool) []string {
		var ret []string
		for _, v := range strings.Split(toExpand, " ") {
			matches := r.FindStringSubmatch(v)
			if len(matches) == 0 {
				return []string{v}
			}

			if len(matches) != 2 {
				panic(fmt.Errorf(
					"Expected to only match 1 subexpression in %s, got %d",
					v,
					len(matches)-1))
			}

			// Index 1 of FindStringSubmatch contains the subexpression match
			// (variable name) of the capture group.
			variable := matches[1]
			// toExpand contains a variable.
			if _, ok := seenVars[variable]; ok {
				panic(fmt.Errorf(
					"Unbounded recursive interpolation of variable: %s", variable))
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
				for _, unexpandedVar := range unexpandedVars {
					ret = append(ret, expandVarInternal(unexpandedVar, newSeenVars)...)
				}
			} else if unexpandedVar, ok := stringScope[variable]; ok {
				ret = append(ret, expandVarInternal(unexpandedVar, newSeenVars)...)
			}
		}
		return ret
	}

	return expandVarInternal(toExpand, map[string]bool{})
}
