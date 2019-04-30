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
)

// Enforces visibility rules between modules.
//
// Two stage process:
// * First stage works bottom up to extract visibility information from the modules, parse it,
//   create visibilityRule structures and store them in a map keyed by the module's
//   qualifiedModuleName instance, i.e. //<pkg>:<name>. The map is stored in the context rather
//   than a global variable for testing. Each test has its own Config so they do not share a map
//   and so can be run in parallel.
//
// * Second stage works top down and iterates over all the deps for each module. If the dep is in
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

// Qualified id for a module
type qualifiedModuleName struct {
	// The package (i.e. directory) in which the module is defined, without trailing /
	pkg string

	// The name of the module.
	name string
}

func (q qualifiedModuleName) String() string {
	return fmt.Sprintf("//%s:%s", q.pkg, q.name)
}

// A visibility rule is associated with a module and determines which other modules it is visible
// to, i.e. which other modules can depend on the rule's module.
type visibilityRule interface {
	// Check to see whether this rules matches m.
	// Returns true if it does, false otherwise.
	matches(m qualifiedModuleName) bool

	String() string
}

// A compositeRule is a visibility rule composed from other visibility rules.
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

func (r compositeRule) String() string {
	s := make([]string, 0, len(r))
	for _, r := range r {
		s = append(s, r.String())
	}

	return "[" + strings.Join(s, ", ") + "]"
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

var visibilityRuleMap = NewOnceKey("visibilityRuleMap")

// The map from qualifiedModuleName to visibilityRule.
func moduleToVisibilityRuleMap(ctx BaseModuleContext) *sync.Map {
	return ctx.Config().Once(visibilityRuleMap, func() interface{} {
		return &sync.Map{}
	}).(*sync.Map)
}

// Visibility is not dependent on arch so this must be registered before the arch phase to avoid
// having to process multiple variants for each module.
func registerVisibilityRuleGatherer(ctx RegisterMutatorsContext) {
	ctx.BottomUp("visibilityRuleGatherer", visibilityRuleGatherer).Parallel()
}

// This must be registered after the deps have been resolved.
func registerVisibilityRuleEnforcer(ctx RegisterMutatorsContext) {
	ctx.TopDown("visibilityRuleEnforcer", visibilityRuleEnforcer).Parallel()
}

// Gathers the visibility rules, parses the visibility properties, stores them in a map by
// qualifiedModuleName for retrieval during enforcement.
//
// See ../README.md#Visibility for information on the format of the visibility rules.

func visibilityRuleGatherer(ctx BottomUpMutatorContext) {
	m, ok := ctx.Module().(Module)
	if !ok {
		return
	}

	qualified := createQualifiedModuleName(ctx)

	visibility := m.base().commonProperties.Visibility
	if visibility != nil {
		rule := parseRules(ctx, qualified.pkg, visibility)
		if rule != nil {
			moduleToVisibilityRuleMap(ctx).Store(qualified, rule)
		}
	}
}

func parseRules(ctx BottomUpMutatorContext, currentPkg string, visibility []string) compositeRule {
	ruleCount := len(visibility)
	if ruleCount == 0 {
		// This prohibits an empty list as its meaning is unclear, e.g. it could mean no visibility and
		// it could mean public visibility. Requiring at least one rule makes the owner's intent
		// clearer.
		ctx.PropertyErrorf("visibility", "must contain at least one visibility rule")
		return nil
	}

	rules := make(compositeRule, 0, ruleCount)
	for _, v := range visibility {
		ok, pkg, name := splitRule(ctx, v, currentPkg)
		if !ok {
			// Visibility rule is invalid so ignore it. Keep going rather than aborting straight away to
			// ensure all the rules on this module are checked.
			ctx.PropertyErrorf("visibility",
				"invalid visibility pattern %q must match"+
					" //<package>:<module>, //<package> or :<module>",
				v)
			continue
		}

		if pkg == "visibility" {
			if ruleCount != 1 {
				ctx.PropertyErrorf("visibility", "cannot mix %q with any other visibility rules", v)
				continue
			}
			switch name {
			case "private":
				rules = append(rules, packageRule{currentPkg})
				continue
			case "public":
				return nil
			case "legacy_public":
				ctx.PropertyErrorf("visibility", "//visibility:legacy_public must not be used")
				return nil
			default:
				ctx.PropertyErrorf("visibility", "unrecognized visibility rule %q", v)
				continue
			}
		}

		// If the current directory is not in the vendor tree then there are some additional
		// restrictions on the rules.
		if !isAncestor("vendor", currentPkg) {
			if !isAllowedFromOutsideVendor(pkg, name) {
				ctx.PropertyErrorf("visibility",
					"%q is not allowed. Packages outside //vendor cannot make themselves visible to specific"+
						" targets within //vendor, they can only use //vendor:__subpackages__.", v)
				continue
			}
		}

		// Create the rule
		var r visibilityRule
		switch name {
		case "__pkg__":
			r = packageRule{pkg}
		case "__subpackages__":
			r = subpackagesRule{pkg}
		default:
			ctx.PropertyErrorf("visibility", "unrecognized visibility rule %q", v)
			continue
		}

		rules = append(rules, r)
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

func splitRule(ctx BaseModuleContext, ruleExpression string, currentPkg string) (bool, string, string) {
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
	_, ok := ctx.Module().(Module)
	if !ok {
		return
	}

	qualified := createQualifiedModuleName(ctx)

	moduleToVisibilityRule := moduleToVisibilityRuleMap(ctx)

	// Visit all the dependencies making sure that this module has access to them all.
	ctx.VisitDirectDeps(func(dep Module) {
		depName := ctx.OtherModuleName(dep)
		depDir := ctx.OtherModuleDir(dep)
		depQualified := qualifiedModuleName{depDir, depName}

		// Targets are always visible to other targets in their own package.
		if depQualified.pkg == qualified.pkg {
			return
		}

		rule, ok := moduleToVisibilityRule.Load(depQualified)
		if ok {
			if !rule.(compositeRule).matches(qualified) {
				ctx.ModuleErrorf(
					"depends on %s which is not visible to this module; %s is only visible to %s",
					depQualified, depQualified, rule)
			}
		}
	})
}

func createQualifiedModuleName(ctx BaseModuleContext) qualifiedModuleName {
	moduleName := ctx.ModuleName()
	dir := ctx.ModuleDir()
	qualified := qualifiedModuleName{dir, moduleName}
	return qualified
}
