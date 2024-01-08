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
	"fmt"
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
// against assumptions, in progress code refactors, or policy to be expressed in a
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
	AddNeverAllowRules(createIncludeDirsRules()...)
	AddNeverAllowRules(createTrebleRules()...)
	AddNeverAllowRules(createJavaDeviceForHostRules()...)
	AddNeverAllowRules(createCcSdkVariantRules()...)
	AddNeverAllowRules(createUncompressDexRules()...)
	AddNeverAllowRules(createInitFirstStageRules()...)
	AddNeverAllowRules(createProhibitFrameworkAccessRules()...)
	AddNeverAllowRules(createCcStubsRule())
	AddNeverAllowRules(createJavaExcludeStaticLibsRule())
	AddNeverAllowRules(createProhibitHeaderOnlyRule())
}

// Add a NeverAllow rule to the set of rules to apply.
func AddNeverAllowRules(rules ...Rule) {
	neverallows = append(neverallows, rules...)
}

var (
	neverallowNotInIncludeDir = []string{
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
	neverallowNoUseIncludeDir = []string{
		"frameworks/av/apex",
		"frameworks/av/tools",
		"frameworks/native/cmds",
		"system/apex",
		"system/bpf",
		"system/gatekeeper",
		"system/hwservicemanager",
		"system/libbase",
		"system/libfmq",
		"system/libvintf",
	}
)

func createIncludeDirsRules() []Rule {
	rules := make([]Rule, 0, len(neverallowNotInIncludeDir)+len(neverallowNoUseIncludeDir))

	for _, path := range neverallowNotInIncludeDir {
		rule :=
			NeverAllow().
				WithMatcher("include_dirs", StartsWith(path+"/")).
				Because("include_dirs is deprecated, all usages of '" + path + "' have been migrated" +
					" to use alternate mechanisms and so can no longer be used.")

		rules = append(rules, rule)
	}

	for _, path := range neverallowNoUseIncludeDir {
		rule := NeverAllow().In(path+"/").WithMatcher("include_dirs", isSetMatcherInstance).
			Because("include_dirs is deprecated, all usages of them in '" + path + "' have been migrated" +
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

func createJavaDeviceForHostRules() []Rule {
	javaDeviceForHostProjectsAllowedList := []string{
		"development/build",
		"external/guava",
		"external/kotlinx.coroutines",
		"external/robolectric-shadows",
		"external/robolectric",
		"frameworks/base/ravenwood",
		"frameworks/base/tools/hoststubgen",
		"frameworks/layoutlib",
	}

	return []Rule{
		NeverAllow().
			NotIn(javaDeviceForHostProjectsAllowedList...).
			ModuleType("java_device_for_host", "java_host_for_device").
			Because("java_device_for_host can only be used in allowed projects"),
	}
}

func createCcSdkVariantRules() []Rule {
	sdkVersionOnlyAllowedList := []string{
		// derive_sdk_prefer32 has stem: "derive_sdk" which conflicts with the derive_sdk.
		// This sometimes works because the APEX modules that contain derive_sdk and
		// derive_sdk_prefer32 suppress the platform installation rules, but fails when
		// the APEX modules contain the SDK variant and the platform variant still exists.
		"packages/modules/SdkExtensions/derive_sdk",
		// These are for apps and shouldn't be used by non-SDK variant modules.
		"prebuilts/ndk",
		"tools/test/graphicsbenchmark/apps/sample_app",
		"tools/test/graphicsbenchmark/functional_tests/java",
		"vendor/xts/gts-tests/hostsidetests/gamedevicecert/apps/javatests",
		"external/libtextclassifier/native",
	}

	platformVariantPropertiesAllowedList := []string{
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
			NotIn(sdkVersionOnlyAllowedList...).
			WithMatcher("sdk_variant_only", isSetMatcherInstance).
			Because("sdk_variant_only can only be used in allowed projects"),
		NeverAllow().
			NotIn(platformVariantPropertiesAllowedList...).
			WithMatcher("platform.shared_libs", isSetMatcherInstance).
			Because("platform variant properties can only be used in allowed projects"),
	}
}

func createCcStubsRule() Rule {
	ccStubsImplementationInstallableProjectsAllowedList := []string{
		"packages/modules/Virtualization/vm_payload",
	}

	return NeverAllow().
		NotIn(ccStubsImplementationInstallableProjectsAllowedList...).
		WithMatcher("stubs.implementation_installable", isSetMatcherInstance).
		Because("implementation_installable can only be used in allowed projects.")
}

func createUncompressDexRules() []Rule {
	return []Rule{
		NeverAllow().
			NotIn("art").
			WithMatcher("uncompress_dex", isSetMatcherInstance).
			Because("uncompress_dex is only allowed for certain jars for test in art."),
	}
}

func createInitFirstStageRules() []Rule {
	return []Rule{
		NeverAllow().
			Without("name", "init_first_stage_defaults").
			Without("name", "init_first_stage").
			Without("name", "init_first_stage.microdroid").
			With("install_in_root", "true").
			Because("install_in_root is only for init_first_stage."),
	}
}

func createProhibitFrameworkAccessRules() []Rule {
	return []Rule{
		NeverAllow().
			With("libs", "framework").
			WithoutMatcher("sdk_version", Regexp("(core_.*|^$)")).
			Because("framework can't be used when building against SDK"),
	}
}

func createJavaExcludeStaticLibsRule() Rule {
	return NeverAllow().
		NotIn("build/soong", "libcore", "frameworks/base/api").
		ModuleType("java_library").
		WithMatcher("exclude_static_libs", isSetMatcherInstance).
		Because("exclude_static_libs property is only allowed for java modules defined in build/soong, libcore, and frameworks/base/api")
}

func createProhibitHeaderOnlyRule() Rule {
	return NeverAllow().
		Without("name", "framework-minus-apex-headers").
		With("headers_only", "true").
		Because("headers_only can only be used for generating framework-minus-apex headers for non-updatable modules")
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

type notInListMatcher struct {
	allowed []string
}

func (m *notInListMatcher) Test(value string) bool {
	return !InList(value, m.allowed)
}

func (m *notInListMatcher) String() string {
	return ".not-in-list(" + strings.Join(m.allowed, ",") + ")"
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

func (r *ruleProperty) String() string {
	return fmt.Sprintf("%q matches: %s", strings.Join(r.fields, "."), r.matcher)
}

type ruleProperties []ruleProperty

func (r ruleProperties) String() string {
	var s []string
	for _, r := range r {
		s = append(s, r.String())
	}
	return strings.Join(s, " ")
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

	props       ruleProperties
	unlessProps ruleProperties

	onlyBootclasspathJar bool
}

// Create a new NeverAllow rule.
func NeverAllow() Rule {
	return &rule{directDeps: make(map[string]bool)}
}

// In adds path(s) where this rule applies.
func (r *rule) In(path ...string) Rule {
	r.paths = append(r.paths, cleanPaths(path)...)
	return r
}

// NotIn adds path(s) to that this rule does not apply to.
func (r *rule) NotIn(path ...string) Rule {
	r.unlessPaths = append(r.unlessPaths, cleanPaths(path)...)
	return r
}

// InDirectDeps adds dep(s) that are not allowed with this rule.
func (r *rule) InDirectDeps(deps ...string) Rule {
	for _, d := range deps {
		r.directDeps[d] = true
	}
	return r
}

// WithOsClass adds osClass(es) that this rule applies to.
func (r *rule) WithOsClass(osClasses ...OsClass) Rule {
	r.osClasses = append(r.osClasses, osClasses...)
	return r
}

// ModuleType adds type(s) that this rule applies to.
func (r *rule) ModuleType(types ...string) Rule {
	r.moduleTypes = append(r.moduleTypes, types...)
	return r
}

// NotModuleType adds type(s) that this rule does not apply to..
func (r *rule) NotModuleType(types ...string) Rule {
	r.unlessModuleTypes = append(r.unlessModuleTypes, types...)
	return r
}

// With specifies property/value combinations that are restricted for this rule.
func (r *rule) With(properties, value string) Rule {
	return r.WithMatcher(properties, selectMatcher(value))
}

// WithMatcher specifies property/matcher combinations that are restricted for this rule.
func (r *rule) WithMatcher(properties string, matcher ValueMatcher) Rule {
	r.props = append(r.props, ruleProperty{
		fields:  fieldNamesForProperties(properties),
		matcher: matcher,
	})
	return r
}

// Without specifies property/value combinations that this rule does not apply to.
func (r *rule) Without(properties, value string) Rule {
	return r.WithoutMatcher(properties, selectMatcher(value))
}

// Without specifies property/matcher combinations that this rule does not apply to.
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

// Because specifies a reason for this rule.
func (r *rule) Because(reason string) Rule {
	r.reason = reason
	return r
}

func (r *rule) String() string {
	s := []string{"neverallow requirements. Not allowed:"}
	if len(r.paths) > 0 {
		s = append(s, fmt.Sprintf("in dirs: %q", r.paths))
	}
	if len(r.moduleTypes) > 0 {
		s = append(s, fmt.Sprintf("module types: %q", r.moduleTypes))
	}
	if len(r.props) > 0 {
		s = append(s, fmt.Sprintf("properties matching: %s", r.props))
	}
	if len(r.directDeps) > 0 {
		s = append(s, fmt.Sprintf("dep(s): %q", SortedKeys(r.directDeps)))
	}
	if len(r.osClasses) > 0 {
		s = append(s, fmt.Sprintf("os class(es): %q", r.osClasses))
	}
	if len(r.unlessPaths) > 0 {
		s = append(s, fmt.Sprintf("EXCEPT in dirs: %q", r.unlessPaths))
	}
	if len(r.unlessModuleTypes) > 0 {
		s = append(s, fmt.Sprintf("EXCEPT module types: %q", r.unlessModuleTypes))
	}
	if len(r.unlessProps) > 0 {
		s = append(s, fmt.Sprintf("EXCEPT properties matching: %q", r.unlessProps))
	}
	if len(r.reason) != 0 {
		s = append(s, " which is restricted because "+r.reason)
	}
	if len(s) == 1 {
		s[0] = "neverallow requirements (empty)"
	}
	return strings.Join(s, "\n\t")
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

func NotInList(allowed []string) ValueMatcher {
	return &notInListMatcher{allowed}
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
func setTestNeverallowRules(config Config, testRules []Rule) {
	config.Once(neverallowRulesKey, func() interface{} { return testRules })
}

// Prepares for a test by setting neverallow rules and enabling the mutator.
//
// If the supplied rules are nil then the default rules are used.
func PrepareForTestWithNeverallowRules(testRules []Rule) FixturePreparer {
	return GroupFixturePreparers(
		FixtureModifyConfig(func(config Config) {
			if testRules != nil {
				setTestNeverallowRules(config, testRules)
			}
		}),
		FixtureRegisterWithContext(func(ctx RegistrationContext) {
			ctx.PostDepsMutators(registerNeverallowMutator)
		}),
	)
}
