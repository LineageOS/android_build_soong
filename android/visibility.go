// Copyright 2019 Google Inc. All rights reserved.
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
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/google/blueprint"
)

// Enforces visibility rules between modules.
//
// Multi stage process:
// * First stage works bottom up, before defaults expansion, to check the syntax of the visibility
//   rules that have been specified.
//
// * Second stage works bottom up to extract the package info for each package and store them in a
//   map by package name. See package.go for functionality for this.
//
// * Third stage works bottom up to extract visibility information from the modules, parse it,
//   create visibilityRule structures and store them in a map keyed by the module's
//   qualifiedModuleName instance, i.e. //<pkg>:<name>. The map is stored in the context rather
//   than a global variable for testing. Each test has its own Config so they do not share a map
//   and so can be run in parallel. If a module has no visibility specified then it uses the
//   default package visibility if specified.
//
// * Fourth stage works top down and iterates over all the deps for each module. If the dep is in
//   the same package then it is automatically visible. Otherwise, for each dep it first extracts
//   its visibilityRule from the config map. If one could not be found then it assumes that it is
//   publicly visible. Otherwise, it calls the visibility rule to check that the module can see
//   the dependency. If it cannot then an error is reported.
//
// TODO(b/130631145) - Make visibility work properly with prebuilts.

// Patterns for the values that can be specified in visibility property.
const (
	packagePattern        = `//([^/:]+(?:/[^/:]+)*)`
	namePattern           = `:([^/:]+)`
	visibilityRulePattern = `^(?:` + packagePattern + `)?(?:` + namePattern + `)?$`
)

var visibilityRuleRegexp = regexp.MustCompile(visibilityRulePattern)

type visibilityModuleReference struct {
	name              qualifiedModuleName
	isPartitionModule bool
}

func createVisibilityModuleReference(name, dir, typ string) visibilityModuleReference {
	isPartitionModule := false
	switch typ {
	case "android_filesystem", "android_system_image":
		isPartitionModule = true
	}
	return visibilityModuleReference{
		name:              createQualifiedModuleName(name, dir),
		isPartitionModule: isPartitionModule,
	}
}

// A visibility rule is associated with a module and determines which other modules it is visible
// to, i.e. which other modules can depend on the rule's module.
type visibilityRule interface {
	// Check to see whether this rules matches m.
	// Returns true if it does, false otherwise.
	matches(m visibilityModuleReference) bool

	String() string
}

// Describes the properties provided by a module that contain visibility rules.
type visibilityPropertyImpl struct {
	name            string
	stringsProperty *[]string
}

type visibilityProperty interface {
	getName() string
	getStrings() []string
}

func newVisibilityProperty(name string, stringsProperty *[]string) visibilityProperty {
	return visibilityPropertyImpl{
		name:            name,
		stringsProperty: stringsProperty,
	}
}

func (p visibilityPropertyImpl) getName() string {
	return p.name
}

func (p visibilityPropertyImpl) getStrings() []string {
	return *p.stringsProperty
}

// A compositeRule is a visibility rule composed from a list of atomic visibility rules.
//
// The list corresponds to the list of strings in the visibility property after defaults expansion.
// Even though //visibility:public is not allowed together with other rules in the visibility list
// of a single module, it is allowed here to permit a module to override an inherited visibility
// spec with public visibility.
//
// //visibility:private is not allowed in the same way, since we'd need to check for it during the
// defaults expansion to make that work. No non-private visibility rules are allowed in a
// compositeRule containing a privateRule.
//
// This array will only be [] if all the rules are invalid and will behave as if visibility was
// ["//visibility:private"].
type compositeRule []visibilityRule

var _ visibilityRule = compositeRule{}

// A compositeRule matches if and only if any of its rules matches.
func (c compositeRule) matches(m visibilityModuleReference) bool {
	for _, r := range c {
		if r.matches(m) {
			return true
		}
	}
	return false
}

func (c compositeRule) String() string {
	return "[" + strings.Join(c.Strings(), ", ") + "]"
}

func (c compositeRule) Strings() []string {
	s := make([]string, 0, len(c))
	for _, r := range c {
		s = append(s, r.String())
	}
	return s
}

// A packageRule is a visibility rule that matches modules in a specific package (i.e. directory).
type packageRule struct {
	pkg string
}

var _ visibilityRule = packageRule{}

func (r packageRule) matches(m visibilityModuleReference) bool {
	return m.name.pkg == r.pkg
}

func (r packageRule) String() string {
	return fmt.Sprintf("//%s", r.pkg) // :__pkg__ is the default, so skip it.
}

// A subpackagesRule is a visibility rule that matches modules in a specific package (i.e.
// directory) or any of its subpackages (i.e. subdirectories).
type subpackagesRule struct {
	pkgPrefix string
}

var _ visibilityRule = subpackagesRule{}

func (r subpackagesRule) matches(m visibilityModuleReference) bool {
	return isAncestor(r.pkgPrefix, m.name.pkg)
}

func isAncestor(p1 string, p2 string) bool {
	// Equivalent to strings.HasPrefix(p2+"/", p1+"/"), but without the string copies
	// The check for a trailing slash is so that we don't consider sibling
	// directories with common prefixes to be ancestors, e.g. "fooo/bar" should not be
	// a descendant of "foo".
	return strings.HasPrefix(p2, p1) && (len(p2) == len(p1) || p2[len(p1)] == '/')
}

func (r subpackagesRule) String() string {
	return fmt.Sprintf("//%s:__subpackages__", r.pkgPrefix)
}

// visibilityRule for //visibility:public
type publicRule struct{}

var _ visibilityRule = publicRule{}

func (r publicRule) matches(_ visibilityModuleReference) bool {
	return true
}

func (r publicRule) String() string {
	return "//visibility:public"
}

// visibilityRule for //visibility:private
type privateRule struct{}

var _ visibilityRule = privateRule{}

func (r privateRule) matches(_ visibilityModuleReference) bool {
	return false
}

func (r privateRule) String() string {
	return "//visibility:private"
}

// visibilityRule for //visibility:any_partition
type anyPartitionRule struct{}

var _ visibilityRule = anyPartitionRule{}

func (r anyPartitionRule) matches(m visibilityModuleReference) bool {
	return m.isPartitionModule
}

func (r anyPartitionRule) String() string {
	return "//visibility:any_partition"
}

var visibilityRuleMap = NewOnceKey("visibilityRuleMap")

// The map from qualifiedModuleName to visibilityRule.
func moduleToVisibilityRuleMap(config Config) *sync.Map {
	return config.Once(visibilityRuleMap, func() interface{} {
		return &sync.Map{}
	}).(*sync.Map)
}

// Marker interface that identifies dependencies that are excluded from visibility
// enforcement.
type ExcludeFromVisibilityEnforcementTag interface {
	blueprint.DependencyTag

	// Method that differentiates this interface from others.
	ExcludeFromVisibilityEnforcement()
}

// The visibility mutators.
var PrepareForTestWithVisibility = FixtureRegisterWithContext(registerVisibilityMutators)

func registerVisibilityMutators(ctx RegistrationContext) {
	ctx.PreArchMutators(RegisterVisibilityRuleChecker)
	ctx.PreArchMutators(RegisterVisibilityRuleGatherer)
	ctx.PostDepsMutators(RegisterVisibilityRuleEnforcer)
}

// The rule checker needs to be registered before defaults expansion to correctly check that
// //visibility:xxx isn't combined with other packages in the same list in any one module.
func RegisterVisibilityRuleChecker(ctx RegisterMutatorsContext) {
	ctx.BottomUp("visibilityRuleChecker", visibilityRuleChecker).Parallel()
}

// Registers the function that gathers the visibility rules for each module.
//
// Visibility is not dependent on arch so this must be registered before the arch phase to avoid
// having to process multiple variants for each module. This goes after defaults expansion to gather
// the complete visibility lists from flat lists and after the package info is gathered to ensure
// that default_visibility is available.
func RegisterVisibilityRuleGatherer(ctx RegisterMutatorsContext) {
	ctx.BottomUp("visibilityRuleGatherer", visibilityRuleGatherer).Parallel()
}

// This must be registered after the deps have been resolved.
func RegisterVisibilityRuleEnforcer(ctx RegisterMutatorsContext) {
	ctx.TopDown("visibilityRuleEnforcer", visibilityRuleEnforcer).Parallel()
}

// Checks the per-module visibility rule lists before defaults expansion.
func visibilityRuleChecker(ctx BottomUpMutatorContext) {
	visibilityProperties := ctx.Module().visibilityProperties()
	for _, p := range visibilityProperties {
		if visibility := p.getStrings(); visibility != nil {
			checkRules(ctx, ctx.ModuleDir(), p.getName(), visibility)
		}
	}
}

func checkRules(ctx BaseModuleContext, currentPkg, property string, visibility []string) {
	ruleCount := len(visibility)
	if ruleCount == 0 {
		// This prohibits an empty list as its meaning is unclear, e.g. it could mean no visibility and
		// it could mean public visibility. Requiring at least one rule makes the owner's intent
		// clearer.
		ctx.PropertyErrorf(property, "must contain at least one visibility rule")
		return
	}

	for i, v := range visibility {
		ok, pkg, name := splitRule(ctx, v, currentPkg, property)
		if !ok {
			continue
		}

		if pkg == "visibility" {
			switch name {
			case "private", "public", "any_partition":
			case "legacy_public":
				ctx.PropertyErrorf(property, "//visibility:legacy_public must not be used")
				continue
			case "override":
				// This keyword does not create a rule so pretend it does not exist.
				ruleCount -= 1
			default:
				ctx.PropertyErrorf(property, "unrecognized visibility rule %q", v)
				continue
			}
			if name == "override" {
				if i != 0 {
					ctx.PropertyErrorf(property, `"%v" may only be used at the start of the visibility rules`, v)
				}
			} else if ruleCount != 1 {
				ctx.PropertyErrorf(property, "cannot mix %q with any other visibility rules", v)
				continue
			}
		}

		// If the current directory is not in the vendor tree then there are some additional
		// restrictions on the rules.
		if !isAncestor("vendor", currentPkg) {
			if !isAllowedFromOutsideVendor(pkg, name) {
				ctx.PropertyErrorf(property,
					"%q is not allowed. Packages outside //vendor cannot make themselves visible to specific"+
						" targets within //vendor, they can only use //vendor:__subpackages__.", v)
				continue
			}
		}
	}
}

// Gathers the flattened visibility rules after defaults expansion, parses the visibility
// properties, stores them in a map by qualifiedModuleName for retrieval during enforcement.
//
// See ../README.md#Visibility for information on the format of the visibility rules.
func visibilityRuleGatherer(ctx BottomUpMutatorContext) {
	m := ctx.Module()

	qualifiedModuleId := m.qualifiedModuleId(ctx)
	currentPkg := qualifiedModuleId.pkg

	// Parse the visibility rules that control access to the module and store them by id
	// for use when enforcing the rules.
	primaryProperty := m.base().primaryVisibilityProperty
	if primaryProperty != nil {
		if visibility := primaryProperty.getStrings(); visibility != nil {
			rule := parseRules(ctx, currentPkg, primaryProperty.getName(), visibility)
			if rule != nil {
				moduleToVisibilityRuleMap(ctx.Config()).Store(qualifiedModuleId, rule)
			}
		}
	}
}

func parseRules(ctx BaseModuleContext, currentPkg, property string, visibility []string) compositeRule {
	rules := make(compositeRule, 0, len(visibility))
	hasPrivateRule := false
	hasPublicRule := false
	hasNonPrivateRule := false
	for _, v := range visibility {
		ok, pkg, name := splitRule(ctx, v, currentPkg, property)
		if !ok {
			continue
		}

		var r visibilityRule
		isPrivateRule := false
		if pkg == "visibility" {
			switch name {
			case "private":
				r = privateRule{}
				isPrivateRule = true
			case "public":
				r = publicRule{}
				hasPublicRule = true
			case "override":
				// Discard all preceding rules and any state based on them.
				rules = nil
				hasPrivateRule = false
				hasPublicRule = false
				hasNonPrivateRule = false
				// This does not actually create a rule so continue onto the next rule.
				continue
			case "any_partition":
				r = anyPartitionRule{}
			}
		} else {
			switch name {
			case "__pkg__":
				r = packageRule{pkg}
			case "__subpackages__":
				r = subpackagesRule{pkg}
			default:
				ctx.PropertyErrorf(property, "invalid visibility pattern %q. Must match "+
					" //<package>:<scope>, //<package> or :<scope> "+
					"where <scope> is one of \"__pkg__\", \"__subpackages__\"",
					v)
			}
		}

		if isPrivateRule {
			hasPrivateRule = true
		} else {
			hasNonPrivateRule = true
		}

		rules = append(rules, r)
	}

	if hasPrivateRule && hasNonPrivateRule {
		ctx.PropertyErrorf("visibility",
			"cannot mix \"//visibility:private\" with any other visibility rules")
		return compositeRule{privateRule{}}
	}

	if hasPublicRule {
		// Public overrides all other rules so just return it.
		return compositeRule{publicRule{}}
	}

	return rules
}

func isAllowedFromOutsideVendor(pkg string, name string) bool {
	if pkg == "vendor" {
		return name == "__subpackages__"
	}

	return !isAncestor("vendor", pkg)
}

func splitRule(ctx BaseModuleContext, ruleExpression string, currentPkg, property string) (bool, string, string) {
	// Make sure that the rule is of the correct format.
	matches := visibilityRuleRegexp.FindStringSubmatch(ruleExpression)
	if ruleExpression == "" || matches == nil {
		// Visibility rule is invalid so ignore it. Keep going rather than aborting straight away to
		// ensure all the rules on this module are checked.
		ctx.PropertyErrorf(property,
			"invalid visibility pattern %q must match"+
				" //<package>:<scope>, //<package> or :<scope> "+
				"where <scope> is one of \"__pkg__\", \"__subpackages__\"",
			ruleExpression)
		return false, "", ""
	}

	// Extract the package and name.
	pkg := matches[1]
	name := matches[2]

	// Normalize the short hands
	if pkg == "" {
		pkg = currentPkg
	}
	if name == "" {
		name = "__pkg__"
	}

	return true, pkg, name
}

func visibilityRuleEnforcer(ctx TopDownMutatorContext) {
	qualified := createVisibilityModuleReference(ctx.ModuleName(), ctx.ModuleDir(), ctx.ModuleType())

	// Visit all the dependencies making sure that this module has access to them all.
	ctx.VisitDirectDeps(func(dep Module) {
		// Ignore dependencies that have an ExcludeFromVisibilityEnforcementTag
		tag := ctx.OtherModuleDependencyTag(dep)
		if _, ok := tag.(ExcludeFromVisibilityEnforcementTag); ok {
			return
		}

		depName := ctx.OtherModuleName(dep)
		depDir := ctx.OtherModuleDir(dep)
		depQualified := qualifiedModuleName{depDir, depName}

		// Targets are always visible to other targets in their own package.
		if depQualified.pkg == qualified.name.pkg {
			return
		}

		rule := effectiveVisibilityRules(ctx.Config(), depQualified)
		if !rule.matches(qualified) {
			ctx.ModuleErrorf("depends on %s which is not visible to this module\nYou may need to add %q to its visibility", depQualified, "//"+ctx.ModuleDir())
		}
	})
}

// Default visibility is public.
var defaultVisibility = compositeRule{publicRule{}}

// Return the effective visibility rules.
//
// If no rules have been specified this will return the default visibility rule
// which is currently //visibility:public.
func effectiveVisibilityRules(config Config, qualified qualifiedModuleName) compositeRule {
	moduleToVisibilityRule := moduleToVisibilityRuleMap(config)
	value, ok := moduleToVisibilityRule.Load(qualified)
	var rule compositeRule
	if ok {
		rule = value.(compositeRule)
	} else {
		rule = packageDefaultVisibility(moduleToVisibilityRule, qualified)
	}

	// If no rule is specified then return the default visibility rule to avoid
	// every caller having to treat nil as public.
	if rule == nil {
		rule = defaultVisibility
	}
	return rule
}

func createQualifiedModuleName(moduleName, dir string) qualifiedModuleName {
	qualified := qualifiedModuleName{dir, moduleName}
	return qualified
}

func packageDefaultVisibility(moduleToVisibilityRule *sync.Map, moduleId qualifiedModuleName) compositeRule {
	packageQualifiedId := moduleId.getContainingPackageId()
	for {
		value, ok := moduleToVisibilityRule.Load(packageQualifiedId)
		if ok {
			return value.(compositeRule)
		}

		if packageQualifiedId.isRootPackage() {
			return nil
		}

		packageQualifiedId = packageQualifiedId.getContainingPackageId()
	}
}

type VisibilityRuleSet interface {
	// Widen the visibility with some extra rules.
	Widen(extra []string) error

	Strings() []string
}

type visibilityRuleSet struct {
	rules []string
}

var _ VisibilityRuleSet = (*visibilityRuleSet)(nil)

func (v *visibilityRuleSet) Widen(extra []string) error {
	// Check the extra rules first just in case they are invalid. Otherwise, if
	// the current visibility is public then the extra rules will just be ignored.
	if len(extra) == 1 {
		singularRule := extra[0]
		switch singularRule {
		case "//visibility:public":
			// Public overrides everything so just discard any existing rules.
			v.rules = extra
			return nil
		case "//visibility:private":
			// Extending rule with private is an error.
			return fmt.Errorf("%q does not widen the visibility", singularRule)
		}
	}

	if len(v.rules) == 1 {
		switch v.rules[0] {
		case "//visibility:public":
			// No point in adding rules to something which is already public.
			return nil
		case "//visibility:private":
			// Adding any rules to private means it is no longer private so the
			// private can be discarded.
			v.rules = nil
		}
	}

	v.rules = FirstUniqueStrings(append(v.rules, extra...))
	sort.Strings(v.rules)
	return nil
}

func (v *visibilityRuleSet) Strings() []string {
	return v.rules
}

// Get the effective visibility rules, i.e. the actual rules that affect the visibility of the
// property irrespective of where they are defined.
//
// Includes visibility rules specified by package default_visibility and/or on defaults.
// Short hand forms, e.g. //:__subpackages__ are replaced with their full form, e.g.
// //package/containing/rule:__subpackages__.
func EffectiveVisibilityRules(ctx BaseModuleContext, module Module) VisibilityRuleSet {
	moduleName := ctx.OtherModuleName(module)
	dir := ctx.OtherModuleDir(module)
	qualified := qualifiedModuleName{dir, moduleName}

	rule := effectiveVisibilityRules(ctx.Config(), qualified)

	currentModule := createVisibilityModuleReference(moduleName, dir, ctx.OtherModuleType(module))

	// Modules are implicitly visible to other modules in the same package,
	// without checking the visibility rules. Here we need to add that visibility
	// explicitly.
	if !rule.matches(currentModule) {
		if len(rule) == 1 {
			if _, ok := rule[0].(privateRule); ok {
				// If the rule is //visibility:private we can't append another
				// visibility to it. Semantically we need to convert it to a package
				// visibility rule for the location where the result is used, but since
				// modules are implicitly visible within the package we get the same
				// result without any rule at all, so just make it an empty list to be
				// appended below.
				rule = nil
			}
		}
		rule = append(rule, packageRule{dir})
	}

	return &visibilityRuleSet{rule.Strings()}
}

// Clear the default visibility properties so they can be replaced.
func clearVisibilityProperties(module Module) {
	module.base().visibilityPropertyInfo = nil
}

// Add a property that contains visibility rules so that they are checked for
// correctness.
func AddVisibilityProperty(module Module, name string, stringsProperty *[]string) {
	addVisibilityProperty(module, name, stringsProperty)
}

func addVisibilityProperty(module Module, name string, stringsProperty *[]string) visibilityProperty {
	base := module.base()
	property := newVisibilityProperty(name, stringsProperty)
	base.visibilityPropertyInfo = append(base.visibilityPropertyInfo, property)
	return property
}

// Set the primary visibility property.
//
// Also adds the property to the list of properties to be validated.
func setPrimaryVisibilityProperty(module Module, name string, stringsProperty *[]string) {
	module.base().primaryVisibilityProperty = addVisibilityProperty(module, name, stringsProperty)
}
