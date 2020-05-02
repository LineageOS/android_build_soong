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
	"regexp"
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

func RegisterNeverallowMutator(ctx RegisterMutatorsContext) {
	ctx.BottomUp("neverallow", neverallowMutator).Parallel()
}

var neverallows = []Rule{}

func init() {
	AddNeverAllowRules(createIncludeDirsRules()...)
	AddNeverAllowRules(createTrebleRules()...)
	AddNeverAllowRules(createLibcoreRules()...)
	AddNeverAllowRules(createMediaRules()...)
	AddNeverAllowRules(createJavaDeviceForHostRules()...)
	AddNeverAllowRules(createCcSdkVariantRules()...)
}

// Add a NeverAllow rule to the set of rules to apply.
func AddNeverAllowRules(rules ...Rule) {
	neverallows = append(neverallows, rules...)
}

func createIncludeDirsRules() []Rule {
	// The list of paths that cannot be referenced using include_dirs
	paths := []string{
		"art",
		"art/libnativebridge",
		"art/libnativeloader",
		"libcore",
		"libnativehelper",
		"external/apache-harmony",
		"external/apache-xml",
		"external/boringssl",
		"external/bouncycastle",
		"external/conscrypt",
		"external/icu",
		"external/okhttp",
		"external/vixl",
		"external/wycheproof",
	}

	// Create a composite matcher that will match if the value starts with any of the restricted
	// paths. A / is appended to the prefix to ensure that restricting path X does not affect paths
	// XY.
	rules := make([]Rule, 0, len(paths))
	for _, path := range paths {
		rule :=
			NeverAllow().
				WithMatcher("include_dirs", StartsWith(path+"/")).
				Because("include_dirs is deprecated, all usages of '" + path + "' have been migrated" +
					" to use alternate mechanisms and so can no longer be used.")

		rules = append(rules, rule)
	}

	return rules
}

func createTrebleRules() []Rule {
	return []Rule{
		NeverAllow().
			In("vendor", "device").
			With("vndk.enabled", "true").
			Without("vendor", "true").
			Without("product_specific", "true").
			Because("the VNDK can never contain a library that is device dependent."),
		NeverAllow().
			With("vndk.enabled", "true").
			Without("vendor", "true").
			Without("owner", "").
			Because("a VNDK module can never have an owner."),

		// TODO(b/67974785): always enforce the manifest
		NeverAllow().
			Without("name", "libhidlbase-combined-impl").
			Without("name", "libhidlbase").
			Without("name", "libhidlbase_pgo").
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
	}

	// Core library constraints. The sdk_version: "none" can only be used in core library projects.
	// Access to core library targets is restricted using visibility rules.
	rules := []Rule{
		NeverAllow().
			NotIn(coreLibraryProjects...).
			With("sdk_version", "none").
			WithoutMatcher("name", Regexp("^android_.*stubs_current$")),
	}

	return rules
}

func createMediaRules() []Rule {
	return []Rule{
		NeverAllow().
			With("libs", "updatable-media").
			Because("updatable-media includes private APIs. Use updatable_media_stubs instead."),
	}
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

func createCcSdkVariantRules() []Rule {
	sdkVersionOnlyWhitelist := []string{
		// derive_sdk_prefer32 has stem: "derive_sdk" which conflicts with the derive_sdk.
		// This sometimes works because the APEX modules that contain derive_sdk and
		// derive_sdk_prefer32 suppress the platform installation rules, but fails when
		// the APEX modules contain the SDK variant and the platform variant still exists.
		"frameworks/base/apex/sdkextensions/derive_sdk",
	}

	platformVariantPropertiesWhitelist := []string{
		// android_native_app_glue and libRSSupport use native_window.h but target old
		// sdk versions (minimum and 9 respectively) where libnativewindow didn't exist,
		// so they can't add libnativewindow to shared_libs to get the header directory
		// for the platform variant.  Allow them to use the platform variant
		// property to set shared_libs.
		"prebuilts/ndk",
		"frameworks/rs",
	}

	return []Rule{
		NeverAllow().
			NotIn(sdkVersionOnlyWhitelist...).
			WithMatcher("sdk_variant_only", isSetMatcherInstance).
			Because("sdk_variant_only can only be used in whitelisted projects"),
		NeverAllow().
			NotIn(platformVariantPropertiesWhitelist...).
			WithMatcher("platform.shared_libs", isSetMatcherInstance).
			Because("platform variant properties can only be used in whitelisted projects"),
	}
}

func neverallowMutator(ctx BottomUpMutatorContext) {
	m, ok := ctx.Module().(Module)
	if !ok {
		return
	}

	dir := ctx.ModuleDir() + "/"
	properties := m.GetProperties()

	osClass := ctx.Module().Target().Os.Class

	for _, r := range neverallowRules(ctx.Config()) {
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

		if !n.appliesToOsClass(osClass) {
			continue
		}

		if !n.appliesToDirectDeps(ctx) {
			continue
		}

		ctx.ModuleErrorf("violates " + n.String())
	}
}

type ValueMatcher interface {
	Test(string) bool
	String() string
}

type equalMatcher struct {
	expected string
}

func (m *equalMatcher) Test(value string) bool {
	return m.expected == value
}

func (m *equalMatcher) String() string {
	return "=" + m.expected
}

type anyMatcher struct {
}

func (m *anyMatcher) Test(value string) bool {
	return true
}

func (m *anyMatcher) String() string {
	return "=*"
}

var anyMatcherInstance = &anyMatcher{}

type startsWithMatcher struct {
	prefix string
}

func (m *startsWithMatcher) Test(value string) bool {
	return strings.HasPrefix(value, m.prefix)
}

func (m *startsWithMatcher) String() string {
	return ".starts-with(" + m.prefix + ")"
}

type regexMatcher struct {
	re *regexp.Regexp
}

func (m *regexMatcher) Test(value string) bool {
	return m.re.MatchString(value)
}

func (m *regexMatcher) String() string {
	return ".regexp(" + m.re.String() + ")"
}

type isSetMatcher struct{}

func (m *isSetMatcher) Test(value string) bool {
	return value != ""
}

func (m *isSetMatcher) String() string {
	return ".is-set"
}

var isSetMatcherInstance = &isSetMatcher{}

type ruleProperty struct {
	fields  []string // e.x.: Vndk.Enabled
	matcher ValueMatcher
}

// A NeverAllow rule.
type Rule interface {
	In(path ...string) Rule

	NotIn(path ...string) Rule

	InDirectDeps(deps ...string) Rule

	WithOsClass(osClasses ...OsClass) Rule

	ModuleType(types ...string) Rule

	NotModuleType(types ...string) Rule

	With(properties, value string) Rule

	WithMatcher(properties string, matcher ValueMatcher) Rule

	Without(properties, value string) Rule

	WithoutMatcher(properties string, matcher ValueMatcher) Rule

	Because(reason string) Rule
}

type rule struct {
	// User string for why this is a thing.
	reason string

	paths       []string
	unlessPaths []string

	directDeps map[string]bool

	osClasses []OsClass

	moduleTypes       []string
	unlessModuleTypes []string

	props       []ruleProperty
	unlessProps []ruleProperty
}

// Create a new NeverAllow rule.
func NeverAllow() Rule {
	return &rule{directDeps: make(map[string]bool)}
}

func (r *rule) In(path ...string) Rule {
	r.paths = append(r.paths, cleanPaths(path)...)
	return r
}

func (r *rule) NotIn(path ...string) Rule {
	r.unlessPaths = append(r.unlessPaths, cleanPaths(path)...)
	return r
}

func (r *rule) InDirectDeps(deps ...string) Rule {
	for _, d := range deps {
		r.directDeps[d] = true
	}
	return r
}

func (r *rule) WithOsClass(osClasses ...OsClass) Rule {
	r.osClasses = append(r.osClasses, osClasses...)
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
	return r.WithMatcher(properties, selectMatcher(value))
}

func (r *rule) WithMatcher(properties string, matcher ValueMatcher) Rule {
	r.props = append(r.props, ruleProperty{
		fields:  fieldNamesForProperties(properties),
		matcher: matcher,
	})
	return r
}

func (r *rule) Without(properties, value string) Rule {
	return r.WithoutMatcher(properties, selectMatcher(value))
}

func (r *rule) WithoutMatcher(properties string, matcher ValueMatcher) Rule {
	r.unlessProps = append(r.unlessProps, ruleProperty{
		fields:  fieldNamesForProperties(properties),
		matcher: matcher,
	})
	return r
}

func selectMatcher(expected string) ValueMatcher {
	if expected == "*" {
		return anyMatcherInstance
	}
	return &equalMatcher{expected: expected}
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
		s += " " + strings.Join(v.fields, ".") + v.matcher.String()
	}
	for _, v := range r.unlessProps {
		s += " -" + strings.Join(v.fields, ".") + v.matcher.String()
	}
	for k := range r.directDeps {
		s += " deps:" + k
	}
	for _, v := range r.osClasses {
		s += " os:" + v.String()
	}
	if len(r.reason) != 0 {
		s += " which is restricted because " + r.reason
	}
	return s
}

func (r *rule) appliesToPath(dir string) bool {
	includePath := len(r.paths) == 0 || HasAnyPrefix(dir, r.paths)
	excludePath := HasAnyPrefix(dir, r.unlessPaths)
	return includePath && !excludePath
}

func (r *rule) appliesToDirectDeps(ctx BottomUpMutatorContext) bool {
	if len(r.directDeps) == 0 {
		return true
	}

	matches := false
	ctx.VisitDirectDeps(func(m Module) {
		if !matches {
			name := ctx.OtherModuleName(m)
			matches = r.directDeps[name]
		}
	})

	return matches
}

func (r *rule) appliesToOsClass(osClass OsClass) bool {
	if len(r.osClasses) == 0 {
		return true
	}

	for _, c := range r.osClasses {
		if c == osClass {
			return true
		}
	}

	return false
}

func (r *rule) appliesToModuleType(moduleType string) bool {
	return (len(r.moduleTypes) == 0 || InList(moduleType, r.moduleTypes)) && !InList(moduleType, r.unlessModuleTypes)
}

func (r *rule) appliesToProperties(properties []interface{}) bool {
	includeProps := hasAllProperties(properties, r.props)
	excludeProps := hasAnyProperty(properties, r.unlessProps)
	return includeProps && !excludeProps
}

func StartsWith(prefix string) ValueMatcher {
	return &startsWithMatcher{prefix}
}

func Regexp(re string) ValueMatcher {
	r, err := regexp.Compile(re)
	if err != nil {
		panic(err)
	}
	return &regexMatcher{r}
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

		check := func(value string) bool {
			return prop.matcher.Test(value)
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

var neverallowRulesKey = NewOnceKey("neverallowRules")

func neverallowRules(config Config) []Rule {
	return config.Once(neverallowRulesKey, func() interface{} {
		// No test rules were set by setTestNeverallowRules, use the global rules
		return neverallows
	}).([]Rule)
}

// Overrides the default neverallow rules for the supplied config.
//
// For testing only.
func SetTestNeverallowRules(config Config, testRules []Rule) {
	config.Once(neverallowRulesKey, func() interface{} { return testRules })
}
