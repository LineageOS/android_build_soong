// Copyright 2015 Google Inc. All rights reserved.
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
	"github.com/google/blueprint"
	"regexp"
	"strings"
)

// BaseModuleContext is the same as blueprint.BaseModuleContext except that Config() returns
// a Config instead of an interface{}, and some methods have been wrapped to use an android.Module
// instead of a blueprint.Module, plus some extra methods that return Android-specific information
// about the current module.
type BaseModuleContext interface {
	EarlyModuleContext

	blueprintBaseModuleContext() blueprint.BaseModuleContext

	// OtherModuleName returns the name of another Module.  See BaseModuleContext.ModuleName for more information.
	// It is intended for use inside the visit functions of Visit* and WalkDeps.
	OtherModuleName(m blueprint.Module) string

	// OtherModuleDir returns the directory of another Module.  See BaseModuleContext.ModuleDir for more information.
	// It is intended for use inside the visit functions of Visit* and WalkDeps.
	OtherModuleDir(m blueprint.Module) string

	// OtherModuleErrorf reports an error on another Module.  See BaseModuleContext.ModuleErrorf for more information.
	// It is intended for use inside the visit functions of Visit* and WalkDeps.
	OtherModuleErrorf(m blueprint.Module, fmt string, args ...interface{})

	// OtherModuleDependencyTag returns the dependency tag used to depend on a module, or nil if there is no dependency
	// on the module.  When called inside a Visit* method with current module being visited, and there are multiple
	// dependencies on the module being visited, it returns the dependency tag used for the current dependency.
	OtherModuleDependencyTag(m blueprint.Module) blueprint.DependencyTag

	// OtherModuleExists returns true if a module with the specified name exists, as determined by the NameInterface
	// passed to Context.SetNameInterface, or SimpleNameInterface if it was not called.
	OtherModuleExists(name string) bool

	// OtherModuleDependencyVariantExists returns true if a module with the
	// specified name and variant exists. The variant must match the given
	// variations. It must also match all the non-local variations of the current
	// module. In other words, it checks for the module that AddVariationDependencies
	// would add a dependency on with the same arguments.
	OtherModuleDependencyVariantExists(variations []blueprint.Variation, name string) bool

	// OtherModuleFarDependencyVariantExists returns true if a module with the
	// specified name and variant exists. The variant must match the given
	// variations, but not the non-local variations of the current module. In
	// other words, it checks for the module that AddFarVariationDependencies
	// would add a dependency on with the same arguments.
	OtherModuleFarDependencyVariantExists(variations []blueprint.Variation, name string) bool

	// OtherModuleReverseDependencyVariantExists returns true if a module with the
	// specified name exists with the same variations as the current module. In
	// other words, it checks for the module that AddReverseDependency would add a
	// dependency on with the same argument.
	OtherModuleReverseDependencyVariantExists(name string) bool

	// OtherModuleType returns the type of another Module.  See BaseModuleContext.ModuleType for more information.
	// It is intended for use inside the visit functions of Visit* and WalkDeps.
	OtherModuleType(m blueprint.Module) string

	// OtherModuleProvider returns the value for a provider for the given module.  If the value is
	// not set it returns the zero value of the type of the provider, so the return value can always
	// be type asserted to the type of the provider.  The value returned may be a deep copy of the
	// value originally passed to SetProvider.
	OtherModuleProvider(m blueprint.Module, provider blueprint.ProviderKey) interface{}

	// OtherModuleHasProvider returns true if the provider for the given module has been set.
	OtherModuleHasProvider(m blueprint.Module, provider blueprint.ProviderKey) bool

	// Provider returns the value for a provider for the current module.  If the value is
	// not set it returns the zero value of the type of the provider, so the return value can always
	// be type asserted to the type of the provider.  It panics if called before the appropriate
	// mutator or GenerateBuildActions pass for the provider.  The value returned may be a deep
	// copy of the value originally passed to SetProvider.
	Provider(provider blueprint.ProviderKey) interface{}

	// HasProvider returns true if the provider for the current module has been set.
	HasProvider(provider blueprint.ProviderKey) bool

	// SetProvider sets the value for a provider for the current module.  It panics if not called
	// during the appropriate mutator or GenerateBuildActions pass for the provider, if the value
	// is not of the appropriate type, or if the value has already been set.  The value should not
	// be modified after being passed to SetProvider.
	SetProvider(provider blueprint.ProviderKey, value interface{})

	GetDirectDepsWithTag(tag blueprint.DependencyTag) []Module

	// GetDirectDepWithTag returns the Module the direct dependency with the specified name, or nil if
	// none exists.  It panics if the dependency does not have the specified tag.  It skips any
	// dependencies that are not an android.Module.
	GetDirectDepWithTag(name string, tag blueprint.DependencyTag) blueprint.Module

	// GetDirectDep returns the Module and DependencyTag for the  direct dependency with the specified
	// name, or nil if none exists.  If there are multiple dependencies on the same module it returns
	// the first DependencyTag.
	GetDirectDep(name string) (blueprint.Module, blueprint.DependencyTag)

	// VisitDirectDepsBlueprint calls visit for each direct dependency.  If there are multiple
	// direct dependencies on the same module visit will be called multiple times on that module
	// and OtherModuleDependencyTag will return a different tag for each.
	//
	// The Module passed to the visit function should not be retained outside of the visit
	// function, it may be invalidated by future mutators.
	VisitDirectDepsBlueprint(visit func(blueprint.Module))

	// VisitDirectDeps calls visit for each direct dependency.  If there are multiple
	// direct dependencies on the same module visit will be called multiple times on that module
	// and OtherModuleDependencyTag will return a different tag for each.  It raises an error if any of the
	// dependencies are not an android.Module.
	//
	// The Module passed to the visit function should not be retained outside of the visit
	// function, it may be invalidated by future mutators.
	VisitDirectDeps(visit func(Module))

	VisitDirectDepsWithTag(tag blueprint.DependencyTag, visit func(Module))

	// VisitDirectDepsIf calls pred for each direct dependency, and if pred returns true calls visit.  If there are
	// multiple direct dependencies on the same module pred and visit will be called multiple times on that module and
	// OtherModuleDependencyTag will return a different tag for each.  It skips any
	// dependencies that are not an android.Module.
	//
	// The Module passed to the visit function should not be retained outside of the visit function, it may be
	// invalidated by future mutators.
	VisitDirectDepsIf(pred func(Module) bool, visit func(Module))
	// Deprecated: use WalkDeps instead to support multiple dependency tags on the same module
	VisitDepsDepthFirst(visit func(Module))
	// Deprecated: use WalkDeps instead to support multiple dependency tags on the same module
	VisitDepsDepthFirstIf(pred func(Module) bool, visit func(Module))

	// WalkDeps calls visit for each transitive dependency, traversing the dependency tree in top down order.  visit may
	// be called multiple times for the same (child, parent) pair if there are multiple direct dependencies between the
	// child and parent with different tags.  OtherModuleDependencyTag will return the tag for the currently visited
	// (child, parent) pair.  If visit returns false WalkDeps will not continue recursing down to child.  It skips
	// any dependencies that are not an android.Module.
	//
	// The Modules passed to the visit function should not be retained outside of the visit function, they may be
	// invalidated by future mutators.
	WalkDeps(visit func(child, parent Module) bool)

	// WalkDepsBlueprint calls visit for each transitive dependency, traversing the dependency
	// tree in top down order.  visit may be called multiple times for the same (child, parent)
	// pair if there are multiple direct dependencies between the child and parent with different
	// tags.  OtherModuleDependencyTag will return the tag for the currently visited
	// (child, parent) pair.  If visit returns false WalkDeps will not continue recursing down
	// to child.
	//
	// The Modules passed to the visit function should not be retained outside of the visit function, they may be
	// invalidated by future mutators.
	WalkDepsBlueprint(visit func(blueprint.Module, blueprint.Module) bool)

	// GetWalkPath is supposed to be called in visit function passed in WalkDeps()
	// and returns a top-down dependency path from a start module to current child module.
	GetWalkPath() []Module

	// PrimaryModule returns the first variant of the current module.  Variants of a module are always visited in
	// order by mutators and GenerateBuildActions, so the data created by the current mutator can be read from the
	// Module returned by PrimaryModule without data races.  This can be used to perform singleton actions that are
	// only done once for all variants of a module.
	PrimaryModule() Module

	// FinalModule returns the last variant of the current module.  Variants of a module are always visited in
	// order by mutators and GenerateBuildActions, so the data created by the current mutator can be read from all
	// variants using VisitAllModuleVariants if the current module == FinalModule().  This can be used to perform
	// singleton actions that are only done once for all variants of a module.
	FinalModule() Module

	// VisitAllModuleVariants calls visit for each variant of the current module.  Variants of a module are always
	// visited in order by mutators and GenerateBuildActions, so the data created by the current mutator can be read
	// from all variants if the current module == FinalModule().  Otherwise, care must be taken to not access any
	// data modified by the current mutator.
	VisitAllModuleVariants(visit func(Module))

	// GetTagPath is supposed to be called in visit function passed in WalkDeps()
	// and returns a top-down dependency tags path from a start module to current child module.
	// It has one less entry than GetWalkPath() as it contains the dependency tags that
	// exist between each adjacent pair of modules in the GetWalkPath().
	// GetTagPath()[i] is the tag between GetWalkPath()[i] and GetWalkPath()[i+1]
	GetTagPath() []blueprint.DependencyTag

	// GetPathString is supposed to be called in visit function passed in WalkDeps()
	// and returns a multi-line string showing the modules and dependency tags
	// among them along the top-down dependency path from a start module to current child module.
	// skipFirst when set to true, the output doesn't include the start module,
	// which is already printed when this function is used along with ModuleErrorf().
	GetPathString(skipFirst bool) string

	AddMissingDependencies(missingDeps []string)

	// getMissingDependencies returns the list of missing dependencies.
	// Calling this function prevents adding new dependencies.
	getMissingDependencies() []string

	Target() Target
	TargetPrimary() bool

	// The additional arch specific targets (e.g. 32/64 bit) that this module variant is
	// responsible for creating.
	MultiTargets() []Target
	Arch() Arch
	Os() OsType
	Host() bool
	Device() bool
	Darwin() bool
	Windows() bool
	PrimaryArch() bool
}

type baseModuleContext struct {
	bp blueprint.BaseModuleContext
	earlyModuleContext
	os            OsType
	target        Target
	multiTargets  []Target
	targetPrimary bool

	walkPath []Module
	tagPath  []blueprint.DependencyTag

	strictVisitDeps bool // If true, enforce that all dependencies are enabled

}

func (b *baseModuleContext) OtherModuleName(m blueprint.Module) string {
	return b.bp.OtherModuleName(m)
}
func (b *baseModuleContext) OtherModuleDir(m blueprint.Module) string { return b.bp.OtherModuleDir(m) }
func (b *baseModuleContext) OtherModuleErrorf(m blueprint.Module, fmt string, args ...interface{}) {
	b.bp.OtherModuleErrorf(m, fmt, args...)
}
func (b *baseModuleContext) OtherModuleDependencyTag(m blueprint.Module) blueprint.DependencyTag {
	return b.bp.OtherModuleDependencyTag(m)
}
func (b *baseModuleContext) OtherModuleExists(name string) bool { return b.bp.OtherModuleExists(name) }
func (b *baseModuleContext) OtherModuleDependencyVariantExists(variations []blueprint.Variation, name string) bool {
	return b.bp.OtherModuleDependencyVariantExists(variations, name)
}
func (b *baseModuleContext) OtherModuleFarDependencyVariantExists(variations []blueprint.Variation, name string) bool {
	return b.bp.OtherModuleFarDependencyVariantExists(variations, name)
}
func (b *baseModuleContext) OtherModuleReverseDependencyVariantExists(name string) bool {
	return b.bp.OtherModuleReverseDependencyVariantExists(name)
}
func (b *baseModuleContext) OtherModuleType(m blueprint.Module) string {
	return b.bp.OtherModuleType(m)
}
func (b *baseModuleContext) OtherModuleProvider(m blueprint.Module, provider blueprint.ProviderKey) interface{} {
	return b.bp.OtherModuleProvider(m, provider)
}
func (b *baseModuleContext) OtherModuleHasProvider(m blueprint.Module, provider blueprint.ProviderKey) bool {
	return b.bp.OtherModuleHasProvider(m, provider)
}
func (b *baseModuleContext) Provider(provider blueprint.ProviderKey) interface{} {
	return b.bp.Provider(provider)
}
func (b *baseModuleContext) HasProvider(provider blueprint.ProviderKey) bool {
	return b.bp.HasProvider(provider)
}
func (b *baseModuleContext) SetProvider(provider blueprint.ProviderKey, value interface{}) {
	b.bp.SetProvider(provider, value)
}

func (b *baseModuleContext) GetDirectDepWithTag(name string, tag blueprint.DependencyTag) blueprint.Module {
	return b.bp.GetDirectDepWithTag(name, tag)
}

func (b *baseModuleContext) blueprintBaseModuleContext() blueprint.BaseModuleContext {
	return b.bp
}

func (b *baseModuleContext) AddMissingDependencies(deps []string) {
	if deps != nil {
		missingDeps := &b.Module().base().commonProperties.MissingDeps
		*missingDeps = append(*missingDeps, deps...)
		*missingDeps = FirstUniqueStrings(*missingDeps)
	}
}

func (b *baseModuleContext) checkedMissingDeps() bool {
	return b.Module().base().commonProperties.CheckedMissingDeps
}

func (b *baseModuleContext) getMissingDependencies() []string {
	checked := &b.Module().base().commonProperties.CheckedMissingDeps
	*checked = true
	var missingDeps []string
	missingDeps = append(missingDeps, b.Module().base().commonProperties.MissingDeps...)
	missingDeps = append(missingDeps, b.bp.EarlyGetMissingDependencies()...)
	missingDeps = FirstUniqueStrings(missingDeps)
	return missingDeps
}

type AllowDisabledModuleDependency interface {
	blueprint.DependencyTag
	AllowDisabledModuleDependency(target Module) bool
}

func (b *baseModuleContext) validateAndroidModule(module blueprint.Module, tag blueprint.DependencyTag, strict bool) Module {
	aModule, _ := module.(Module)

	if !strict {
		return aModule
	}

	if aModule == nil {
		b.ModuleErrorf("module %q (%#v) not an android module", b.OtherModuleName(module), tag)
		return nil
	}

	if !aModule.Enabled() {
		if t, ok := tag.(AllowDisabledModuleDependency); !ok || !t.AllowDisabledModuleDependency(aModule) {
			if b.Config().AllowMissingDependencies() {
				b.AddMissingDependencies([]string{b.OtherModuleName(aModule)})
			} else {
				b.ModuleErrorf("depends on disabled module %q", b.OtherModuleName(aModule))
			}
		}
		return nil
	}
	return aModule
}

type dep struct {
	mod blueprint.Module
	tag blueprint.DependencyTag
}

func (b *baseModuleContext) getDirectDepsInternal(name string, tag blueprint.DependencyTag) []dep {
	var deps []dep
	b.VisitDirectDepsBlueprint(func(module blueprint.Module) {
		if aModule, _ := module.(Module); aModule != nil {
			if aModule.base().BaseModuleName() == name {
				returnedTag := b.bp.OtherModuleDependencyTag(aModule)
				if tag == nil || returnedTag == tag {
					deps = append(deps, dep{aModule, returnedTag})
				}
			}
		} else if b.bp.OtherModuleName(module) == name {
			returnedTag := b.bp.OtherModuleDependencyTag(module)
			if tag == nil || returnedTag == tag {
				deps = append(deps, dep{module, returnedTag})
			}
		}
	})
	return deps
}

func (b *baseModuleContext) getDirectDepInternal(name string, tag blueprint.DependencyTag) (blueprint.Module, blueprint.DependencyTag) {
	deps := b.getDirectDepsInternal(name, tag)
	if len(deps) == 1 {
		return deps[0].mod, deps[0].tag
	} else if len(deps) >= 2 {
		panic(fmt.Errorf("Multiple dependencies having same BaseModuleName() %q found from %q",
			name, b.ModuleName()))
	} else {
		return nil, nil
	}
}

func (b *baseModuleContext) getDirectDepFirstTag(name string) (blueprint.Module, blueprint.DependencyTag) {
	foundDeps := b.getDirectDepsInternal(name, nil)
	deps := map[blueprint.Module]bool{}
	for _, dep := range foundDeps {
		deps[dep.mod] = true
	}
	if len(deps) == 1 {
		return foundDeps[0].mod, foundDeps[0].tag
	} else if len(deps) >= 2 {
		// this could happen if two dependencies have the same name in different namespaces
		// TODO(b/186554727): this should not occur if namespaces are handled within
		// getDirectDepsInternal.
		panic(fmt.Errorf("Multiple dependencies having same BaseModuleName() %q found from %q",
			name, b.ModuleName()))
	} else {
		return nil, nil
	}
}

func (b *baseModuleContext) GetDirectDepsWithTag(tag blueprint.DependencyTag) []Module {
	var deps []Module
	b.VisitDirectDepsBlueprint(func(module blueprint.Module) {
		if aModule, _ := module.(Module); aModule != nil {
			if b.bp.OtherModuleDependencyTag(aModule) == tag {
				deps = append(deps, aModule)
			}
		}
	})
	return deps
}

// GetDirectDep returns the Module and DependencyTag for the direct dependency with the specified
// name, or nil if none exists. If there are multiple dependencies on the same module it returns the
// first DependencyTag.
func (b *baseModuleContext) GetDirectDep(name string) (blueprint.Module, blueprint.DependencyTag) {
	return b.getDirectDepFirstTag(name)
}

func (b *baseModuleContext) VisitDirectDepsBlueprint(visit func(blueprint.Module)) {
	b.bp.VisitDirectDeps(visit)
}

func (b *baseModuleContext) VisitDirectDeps(visit func(Module)) {
	b.bp.VisitDirectDeps(func(module blueprint.Module) {
		if aModule := b.validateAndroidModule(module, b.bp.OtherModuleDependencyTag(module), b.strictVisitDeps); aModule != nil {
			visit(aModule)
		}
	})
}

func (b *baseModuleContext) VisitDirectDepsWithTag(tag blueprint.DependencyTag, visit func(Module)) {
	b.bp.VisitDirectDeps(func(module blueprint.Module) {
		if b.bp.OtherModuleDependencyTag(module) == tag {
			if aModule := b.validateAndroidModule(module, b.bp.OtherModuleDependencyTag(module), b.strictVisitDeps); aModule != nil {
				visit(aModule)
			}
		}
	})
}

func (b *baseModuleContext) VisitDirectDepsIf(pred func(Module) bool, visit func(Module)) {
	b.bp.VisitDirectDepsIf(
		// pred
		func(module blueprint.Module) bool {
			if aModule := b.validateAndroidModule(module, b.bp.OtherModuleDependencyTag(module), b.strictVisitDeps); aModule != nil {
				return pred(aModule)
			} else {
				return false
			}
		},
		// visit
		func(module blueprint.Module) {
			visit(module.(Module))
		})
}

func (b *baseModuleContext) VisitDepsDepthFirst(visit func(Module)) {
	b.bp.VisitDepsDepthFirst(func(module blueprint.Module) {
		if aModule := b.validateAndroidModule(module, b.bp.OtherModuleDependencyTag(module), b.strictVisitDeps); aModule != nil {
			visit(aModule)
		}
	})
}

func (b *baseModuleContext) VisitDepsDepthFirstIf(pred func(Module) bool, visit func(Module)) {
	b.bp.VisitDepsDepthFirstIf(
		// pred
		func(module blueprint.Module) bool {
			if aModule := b.validateAndroidModule(module, b.bp.OtherModuleDependencyTag(module), b.strictVisitDeps); aModule != nil {
				return pred(aModule)
			} else {
				return false
			}
		},
		// visit
		func(module blueprint.Module) {
			visit(module.(Module))
		})
}

func (b *baseModuleContext) WalkDepsBlueprint(visit func(blueprint.Module, blueprint.Module) bool) {
	b.bp.WalkDeps(visit)
}

func (b *baseModuleContext) WalkDeps(visit func(Module, Module) bool) {
	b.walkPath = []Module{b.Module()}
	b.tagPath = []blueprint.DependencyTag{}
	b.bp.WalkDeps(func(child, parent blueprint.Module) bool {
		childAndroidModule, _ := child.(Module)
		parentAndroidModule, _ := parent.(Module)
		if childAndroidModule != nil && parentAndroidModule != nil {
			// record walkPath before visit
			for b.walkPath[len(b.walkPath)-1] != parentAndroidModule {
				b.walkPath = b.walkPath[0 : len(b.walkPath)-1]
				b.tagPath = b.tagPath[0 : len(b.tagPath)-1]
			}
			b.walkPath = append(b.walkPath, childAndroidModule)
			b.tagPath = append(b.tagPath, b.OtherModuleDependencyTag(childAndroidModule))
			return visit(childAndroidModule, parentAndroidModule)
		} else {
			return false
		}
	})
}

func (b *baseModuleContext) GetWalkPath() []Module {
	return b.walkPath
}

func (b *baseModuleContext) GetTagPath() []blueprint.DependencyTag {
	return b.tagPath
}

func (b *baseModuleContext) VisitAllModuleVariants(visit func(Module)) {
	b.bp.VisitAllModuleVariants(func(module blueprint.Module) {
		visit(module.(Module))
	})
}

func (b *baseModuleContext) PrimaryModule() Module {
	return b.bp.PrimaryModule().(Module)
}

func (b *baseModuleContext) FinalModule() Module {
	return b.bp.FinalModule().(Module)
}

// IsMetaDependencyTag returns true for cross-cutting metadata dependencies.
func IsMetaDependencyTag(tag blueprint.DependencyTag) bool {
	if tag == licenseKindTag {
		return true
	} else if tag == licensesTag {
		return true
	} else if tag == acDepTag {
		return true
	}
	return false
}

// A regexp for removing boilerplate from BaseDependencyTag from the string representation of
// a dependency tag.
var tagCleaner = regexp.MustCompile(`\QBaseDependencyTag:{}\E(, )?`)

// PrettyPrintTag returns string representation of the tag, but prefers
// custom String() method if available.
func PrettyPrintTag(tag blueprint.DependencyTag) string {
	// Use tag's custom String() method if available.
	if stringer, ok := tag.(fmt.Stringer); ok {
		return stringer.String()
	}

	// Otherwise, get a default string representation of the tag's struct.
	tagString := fmt.Sprintf("%T: %+v", tag, tag)

	// Remove the boilerplate from BaseDependencyTag as it adds no value.
	tagString = tagCleaner.ReplaceAllString(tagString, "")
	return tagString
}

func (b *baseModuleContext) GetPathString(skipFirst bool) string {
	sb := strings.Builder{}
	tagPath := b.GetTagPath()
	walkPath := b.GetWalkPath()
	if !skipFirst {
		sb.WriteString(walkPath[0].String())
	}
	for i, m := range walkPath[1:] {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("           via tag %s\n", PrettyPrintTag(tagPath[i])))
		sb.WriteString(fmt.Sprintf("    -> %s", m.String()))
	}
	return sb.String()
}

func (b *baseModuleContext) Target() Target {
	return b.target
}

func (b *baseModuleContext) TargetPrimary() bool {
	return b.targetPrimary
}

func (b *baseModuleContext) MultiTargets() []Target {
	return b.multiTargets
}

func (b *baseModuleContext) Arch() Arch {
	return b.target.Arch
}

func (b *baseModuleContext) Os() OsType {
	return b.os
}

func (b *baseModuleContext) Host() bool {
	return b.os.Class == Host
}

func (b *baseModuleContext) Device() bool {
	return b.os.Class == Device
}

func (b *baseModuleContext) Darwin() bool {
	return b.os == Darwin
}

func (b *baseModuleContext) Windows() bool {
	return b.os == Windows
}

func (b *baseModuleContext) PrimaryArch() bool {
	if len(b.config.Targets[b.target.Os]) <= 1 {
		return true
	}
	return b.target.Arch.ArchType == b.config.Targets[b.target.Os][0].Arch.ArchType
}
