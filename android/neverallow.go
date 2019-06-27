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
// - it is in one of the "In" paths
// - it is not in one of the "NotIn" paths
// - it has all "With" properties matched
// - - values are matched in their entirety
// - - nil is interpreted as an empty string
// - - nested properties are separated with a '.'
// - - if the property is a list, any of the values in the list being matches
//     counts as a match
// - it has none of the "Without" properties matched (same rules as above)

func registerNeverallowMutator(ctx RegisterMutatorsContext) {
	ctx.BottomUp("neverallow", neverallowMutator).Parallel()
}

var neverallows = []Rule{}

func init() {
	AddNeverAllowRules(createTrebleRules()...)
	AddNeverAllowRules(createLibcoreRules()...)
	AddNeverAllowRules(createJavaDeviceForHostRules()...)
}

// Add a NeverAllow rule to the set of rules to apply.
func AddNeverAllowRules(rules ...Rule) {
	neverallows = append(neverallows, rules...)
}

func createTrebleRules() []Rule {
	return []Rule{
		NeverAllow().
			In("vendor", "device").
			With("vndk.enabled", "true").
			Without("vendor", "true").
			Because("the VNDK can never contain a library that is device dependent."),
		NeverAllow().
			With("vndk.enabled", "true").
			Without("vendor", "true").
			Without("owner", "").
			Because("a VNDK module can never have an owner."),

		// TODO(b/67974785): always enforce the manifest
		NeverAllow().
			Without("name", "libhidltransport").
			With("product_variables.enforce_vintf_manifest.cflags", "*").
			Because("manifest enforcement should be independent of ."),

		// TODO(b/67975799): vendor code should always use /vendor/bin/sh
		NeverAllow().
			Without("name", "libc_bionic_ndk").
			With("product_variables.treble_linker_namespaces.cflags", "*").
			Because("nothing should care if linker namespaces are enabled or not"),

		// Example:
		// *NeverAllow().with("Srcs", "main.cpp"))
	}
}

func createLibcoreRules() []Rule {
	var coreLibraryProjects = []string{
		"libcore",
		"external/apache-harmony",
		"external/apache-xml",
		"external/bouncycastle",
		"external/conscrypt",
		"external/icu",
		"external/okhttp",
		"external/wycheproof",

		// Not really a core library but still needs access to same capabilities.
		"development",
	}

	// Core library constraints. The sdk_version: "none" can only be used in core library projects.
	// Access to core library targets is restricted using visibility rules.
	rules := []Rule{
		NeverAllow().
			NotIn(coreLibraryProjects...).
			With("sdk_version", "none"),
	}

	return rules
}

func createJavaDeviceForHostRules() []Rule {
	javaDeviceForHostProjectsWhitelist := []string{
		"external/guava",
		"external/robolectric-shadows",
		"framework/layoutlib",
	}

	return []Rule{
		NeverAllow().
			NotIn(javaDeviceForHostProjectsWhitelist...).
			ModuleType("java_device_for_host", "java_host_for_device").
			Because("java_device_for_host can only be used in whitelisted projects"),
	}
}

func neverallowMutator(ctx BottomUpMutatorContext) {
	m, ok := ctx.Module().(Module)
	if !ok {
		return
	}

	dir := ctx.ModuleDir() + "/"
	properties := m.GetProperties()

	for _, r := range neverallows {
		n := r.(*rule)
		if !n.appliesToPath(dir) {
			continue
		}

		if !n.appliesToModuleType(ctx.ModuleType()) {
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

// A NeverAllow rule.
type Rule interface {
	In(path ...string) Rule

	NotIn(path ...string) Rule

	ModuleType(types ...string) Rule

	NotModuleType(types ...string) Rule

	With(properties, value string) Rule

	Without(properties, value string) Rule

	Because(reason string) Rule
}

type rule struct {
	// User string for why this is a thing.
	reason string

	paths       []string
	unlessPaths []string

	moduleTypes       []string
	unlessModuleTypes []string

	props       []ruleProperty
	unlessProps []ruleProperty
}

// Create a new NeverAllow rule.
func NeverAllow() Rule {
	return &rule{}
}

func (r *rule) In(path ...string) Rule {
	r.paths = append(r.paths, cleanPaths(path)...)
	return r
}

func (r *rule) NotIn(path ...string) Rule {
	r.unlessPaths = append(r.unlessPaths, cleanPaths(path)...)
	return r
}

func (r *rule) ModuleType(types ...string) Rule {
	r.moduleTypes = append(r.moduleTypes, types...)
	return r
}

func (r *rule) NotModuleType(types ...string) Rule {
	r.unlessModuleTypes = append(r.unlessModuleTypes, types...)
	return r
}

func (r *rule) With(properties, value string) Rule {
	r.props = append(r.props, ruleProperty{
		fields: fieldNamesForProperties(properties),
		value:  value,
	})
	return r
}

func (r *rule) Without(properties, value string) Rule {
	r.unlessProps = append(r.unlessProps, ruleProperty{
		fields: fieldNamesForProperties(properties),
		value:  value,
	})
	return r
}

func (r *rule) Because(reason string) Rule {
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
	for _, v := range r.moduleTypes {
		s += " type:" + v
	}
	for _, v := range r.unlessModuleTypes {
		s += " -type:" + v
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

func (r *rule) appliesToModuleType(moduleType string) bool {
	return (len(r.moduleTypes) == 0 || InList(moduleType, r.moduleTypes)) && !InList(moduleType, r.unlessModuleTypes)
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
