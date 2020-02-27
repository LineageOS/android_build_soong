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
// TODO(b/130796911) - Make visibility work properly with defaults.

// Patterns for the values that can be specified in visibility property.
const (
	packagePattern        = `//([^/:]+(?:/[^/:]+)*)`
	namePattern           = `:([^/:]+)`
	visibilityRulePattern = `^(?:` + packagePattern + `)?(?:` + namePattern + `)?$`
)

var visibilityRuleRegexp = regexp.MustCompile(visibilityRulePattern)

// A visibility rule is associated with a module and determines which other modules it is visible
// to, i.e. which other modules can depend on the rule's module.
type visibilityRule interface {
	// Check to see whether this rules matches m.
	// Returns true if it does, false otherwise.
	matches(m qualifiedModuleName) bool

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

// A compositeRule matches if and only if any of its rules matches.
func (c compositeRule) matches(m qualifiedModuleName) bool {
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

func (r packageRule) matches(m qualifiedModuleName) bool {
	return m.pkg == r.pkg
}

func (r packageRule) String() string {
	return fmt.Sprintf("//%s:__pkg__", r.pkg)
}

// A subpackagesRule is a visibility rule that matches modules in a specific package (i.e.
// directory) or any of its subpackages (i.e. subdirectories).
type subpackagesRule struct {
	pkgPrefix string
}

func (r subpackagesRule) matches(m qualifiedModuleName) bool {
	return isAncestor(r.pkgPrefix, m.pkg)
}

func isAncestor(p1 string, p2 string) bool {
	return strings.HasPrefix(p2+"/", p1+"/")
}

func (r subpackagesRule) String() string {
	return fmt.Sprintf("//%s:__subpackages__", r.pkgPrefix)
}

// visibilityRule for //visibility:public
type publicRule struct{}

func (r publicRule) matches(_ qualifiedModuleName) bool {
	return true
}

func (r publicRule) String() string {
	return "//visibility:public"
}

// visibilityRule for //visibility:private
type privateRule struct{}

func (r privateRule) matches(_ qualifiedModuleName) bool {
	return false
}

func (r privateRule) String() string {
	return "//visibility:private"
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
	qualified := createQualifiedModuleName(ctx)
	if m, ok := ctx.Module().(Module); ok {
		visibilityProperties := m.visibilityProperties()
		for _, p := range visibilityProperties {
			if visibility := p.getStrings(); visibility != nil {
				checkRules(ctx, qualified.pkg, p.getName(), visibility)
			}
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

	for _, v := range visibility {
		ok, pkg, name := splitRule(v, currentPkg)
		if !ok {
			// Visibility rule is invalid so ignore it. Keep going rather than aborting straight away to
			// ensure all the rules on this module are checked.
			ctx.PropertyErrorf(property,
				"invalid visibility pattern %q must match"+
					" //<package>:<module>, //<package> or :<module>",
				v)
			continue
		}

		if pkg == "visibility" {
			switch name {
			case "private", "public":
			case "legacy_public":
				ctx.PropertyErrorf(property, "//visibility:legacy_public must not be used")
				continue
			default:
				ctx.PropertyErrorf(property, "unrecognized visibility rule %q", v)
				continue
			}
			if ruleCount != 1 {
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
	m, ok := ctx.Module().(Module)
	if !ok {
		return
	}

	qualifiedModuleId := m.qualifiedModuleId(ctx)
	currentPkg := qualifiedModuleId.pkg

	// Parse the visibility rules that control access to the module and store them by id
	// for use when enforcing the rules.
	if visibility := m.visibility(); visibility != nil {
		rule := parseRules(ctx, currentPkg, m.visibility())
		if rule != nil {
			moduleToVisibilityRuleMap(ctx.Config()).Store(qualifiedModuleId, rule)
		}
	}
}

func parseRules(ctx BaseModuleContext, currentPkg string, visibility []string) compositeRule {
	rules := make(compositeRule, 0, len(visibility))
	hasPrivateRule := false
	hasPublicRule := false
	hasNonPrivateRule := false
	for _, v := range visibility {
		ok, pkg, name := splitRule(v, currentPkg)
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
			}
		} else {
			switch name {
			case "__pkg__":
				r = packageRule{pkg}
			case "__subpackages__":
				r = subpackagesRule{pkg}
			default:
				continue
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
		if name == "__subpackages__" {
			return true
		}
		return false
	}

	return !isAncestor("vendor", pkg)
}

func splitRule(ruleExpression string, currentPkg string) (bool, string, string) {
	// Make sure that the rule is of the correct format.
	matches := visibilityRuleRegexp.FindStringSubmatch(ruleExpression)
	if ruleExpression == "" || matches == nil {
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
	if _, ok := ctx.Module().(Module); !ok {
		return
	}

	qualified := createQualifiedModuleName(ctx)

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
		if depQualified.pkg == qualified.pkg {
			return
		}

		rule := effectiveVisibilityRules(ctx.Config(), depQualified)
		if rule != nil && !rule.matches(qualified) {
			ctx.ModuleErrorf("depends on %s which is not visible to this module", depQualified)
		}
	})
}

func effectiveVisibilityRules(config Config, qualified qualifiedModuleName) compositeRule {
	moduleToVisibilityRule := moduleToVisibilityRuleMap(config)
	value, ok := moduleToVisibilityRule.Load(qualified)
	var rule compositeRule
	if ok {
		rule = value.(compositeRule)
	} else {
		rule = packageDefaultVisibility(config, qualified)
	}
	return rule
}

func createQualifiedModuleName(ctx BaseModuleContext) qualifiedModuleName {
	moduleName := ctx.ModuleName()
	dir := ctx.ModuleDir()
	qualified := qualifiedModuleName{dir, moduleName}
	return qualified
}

func packageDefaultVisibility(config Config, moduleId qualifiedModuleName) compositeRule {
	moduleToVisibilityRule := moduleToVisibilityRuleMap(config)
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

// Get the effective visibility rules, i.e. the actual rules that affect the visibility of the
// property irrespective of where they are defined.
//
// Includes visibility rules specified by package default_visibility and/or on defaults.
// Short hand forms, e.g. //:__subpackages__ are replaced with their full form, e.g.
// //package/containing/rule:__subpackages__.
func EffectiveVisibilityRules(ctx BaseModuleContext, module Module) []string {
	moduleName := ctx.OtherModuleName(module)
	dir := ctx.OtherModuleDir(module)
	qualified := qualifiedModuleName{dir, moduleName}

	rule := effectiveVisibilityRules(ctx.Config(), qualified)

	return rule.Strings()
}
