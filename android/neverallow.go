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

package android

import (
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/google/blueprint/proptools"
)

// "neverallow" rules for the build system.
//
// This allows things which aren't related to the build system and are enforced
// for sanity, in progress code refactors, or policy to be expressed in a
// straightforward away disjoint from implementations and tests which should
// work regardless of these restrictions.
//
// A module is disallowed if all of the following are true:
// - it is in one of the "in" paths
// - it is not in one of the "notIn" paths
// - it has all "with" properties matched
// - - values are matched in their entirety
// - - nil is interpreted as an empty string
// - - nested properties are separated with a '.'
// - - if the property is a list, any of the values in the list being matches
//     counts as a match
// - it has none of the "without" properties matched (same rules as above)

func registerNeverallowMutator(ctx RegisterMutatorsContext) {
	ctx.BottomUp("neverallow", neverallowMutator).Parallel()
}

var neverallows = []*rule{
	neverallow().
		in("vendor", "device").
		with("vndk.enabled", "true").
		without("vendor", "true").
		because("the VNDK can never contain a library that is device dependent."),
	neverallow().
		with("vndk.enabled", "true").
		without("vendor", "true").
		without("owner", "").
		because("a VNDK module can never have an owner."),
	neverallow().notIn("libcore").with("no_standard_libs", "true"),

	// TODO(b/67974785): always enforce the manifest
	neverallow().
		without("name", "libhidltransport").
		with("product_variables.enforce_vintf_manifest.cflags", "*").
		because("manifest enforcement should be independent of ."),

	// TODO(b/67975799): vendor code should always use /vendor/bin/sh
	neverallow().
		without("name", "libc_bionic_ndk").
		with("product_variables.treble_linker_namespaces.cflags", "*").
		because("nothing should care if linker namespaces are enabled or not"),

	// Example:
	// *neverallow().with("Srcs", "main.cpp"),
}

func neverallowMutator(ctx BottomUpMutatorContext) {
	m, ok := ctx.Module().(Module)
	if !ok {
		return
	}

	dir := ctx.ModuleDir() + "/"
	properties := m.GetProperties()

	for _, n := range neverallows {
		if !n.appliesToPath(dir) {
			continue
		}

		if !n.appliesToProperties(properties) {
			continue
		}

		ctx.ModuleErrorf("violates " + n.String())
	}
}

type ruleProperty struct {
	fields []string // e.x.: Vndk.Enabled
	value  string   // e.x.: true
}

type rule struct {
	// User string for why this is a thing.
	reason string

	paths       []string
	unlessPaths []string

	props       []ruleProperty
	unlessProps []ruleProperty
}

func neverallow() *rule {
	return &rule{}
}
func (r *rule) in(path ...string) *rule {
	r.paths = append(r.paths, cleanPaths(path)...)
	return r
}
func (r *rule) notIn(path ...string) *rule {
	r.unlessPaths = append(r.unlessPaths, cleanPaths(path)...)
	return r
}
func (r *rule) with(properties, value string) *rule {
	r.props = append(r.props, ruleProperty{
		fields: fieldNamesForProperties(properties),
		value:  value,
	})
	return r
}
func (r *rule) without(properties, value string) *rule {
	r.unlessProps = append(r.unlessProps, ruleProperty{
		fields: fieldNamesForProperties(properties),
		value:  value,
	})
	return r
}
func (r *rule) because(reason string) *rule {
	r.reason = reason
	return r
}

func (r *rule) String() string {
	s := "neverallow"
	for _, v := range r.paths {
		s += " dir:" + v + "*"
	}
	for _, v := range r.unlessPaths {
		s += " -dir:" + v + "*"
	}
	for _, v := range r.props {
		s += " " + strings.Join(v.fields, ".") + "=" + v.value
	}
	for _, v := range r.unlessProps {
		s += " -" + strings.Join(v.fields, ".") + "=" + v.value
	}
	if len(r.reason) != 0 {
		s += " which is restricted because " + r.reason
	}
	return s
}

func (r *rule) appliesToPath(dir string) bool {
	includePath := len(r.paths) == 0 || hasAnyPrefix(dir, r.paths)
	excludePath := hasAnyPrefix(dir, r.unlessPaths)
	return includePath && !excludePath
}

func (r *rule) appliesToProperties(properties []interface{}) bool {
	includeProps := hasAllProperties(properties, r.props)
	excludeProps := hasAnyProperty(properties, r.unlessProps)
	return includeProps && !excludeProps
}

// assorted utils

func cleanPaths(paths []string) []string {
	res := make([]string, len(paths))
	for i, v := range paths {
		res[i] = filepath.Clean(v) + "/"
	}
	return res
}

func fieldNamesForProperties(propertyNames string) []string {
	names := strings.Split(propertyNames, ".")
	for i, v := range names {
		names[i] = proptools.FieldNameForProperty(v)
	}
	return names
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func hasAnyProperty(properties []interface{}, props []ruleProperty) bool {
	for _, v := range props {
		if hasProperty(properties, v) {
			return true
		}
	}
	return false
}

func hasAllProperties(properties []interface{}, props []ruleProperty) bool {
	for _, v := range props {
		if !hasProperty(properties, v) {
			return false
		}
	}
	return true
}

func hasProperty(properties []interface{}, prop ruleProperty) bool {
	for _, propertyStruct := range properties {
		propertiesValue := reflect.ValueOf(propertyStruct).Elem()
		for _, v := range prop.fields {
			if !propertiesValue.IsValid() {
				break
			}
			propertiesValue = propertiesValue.FieldByName(v)
		}
		if !propertiesValue.IsValid() {
			continue
		}

		check := func(v string) bool {
			return prop.value == "*" || prop.value == v
		}

		if matchValue(propertiesValue, check) {
			return true
		}
	}
	return false
}

func matchValue(value reflect.Value, check func(string) bool) bool {
	if !value.IsValid() {
		return false
	}

	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return check("")
		}
		value = value.Elem()
	}

	switch value.Kind() {
	case reflect.String:
		return check(value.String())
	case reflect.Bool:
		return check(strconv.FormatBool(value.Bool()))
	case reflect.Int:
		return check(strconv.FormatInt(value.Int(), 10))
	case reflect.Slice:
		slice, ok := value.Interface().([]string)
		if !ok {
			panic("Can only handle slice of string")
		}
		for _, v := range slice {
			if check(v) {
				return true
			}
		}
		return false
	}

	panic("Can't handle type: " + value.Kind().String())
}
