// Copyright 2018 Google Inc. All rights reserved.
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
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/google/blueprint"
)

var (
	// This is the sdk version when APEX was first introduced
	SdkVersion_Android10 = uncheckedFinalApiLevel(29)
)

// ApexInfo describes the metadata about one or more apexBundles that an apex variant of a module is
// part of.  When an apex variant is created, the variant is associated with one apexBundle. But
// when multiple apex variants are merged for deduping (see mergeApexVariations), this holds the
// information about the apexBundles that are merged together.
// Accessible via `ctx.Provider(android.ApexInfoProvider).(android.ApexInfo)`
type ApexInfo struct {
	// Name of the apex variation that this module (i.e. the apex variant of the module) is
	// mutated into, or "" for a platform (i.e. non-APEX) variant. Note that this name and the
	// Soong module name of the APEX can be different. That happens when there is
	// `override_apex` that overrides `apex`. In that case, both Soong modules have the same
	// apex variation name which usually is `com.android.foo`. This name is also the `name`
	// in the path `/apex/<name>` where this apex is activated on at runtime.
	//
	// Also note that a module can be included in multiple APEXes, in which case, the module is
	// mutated into one or more variants, each of which is for an APEX. The variants then can
	// later be deduped if they don't need to be compiled differently. This is an optimization
	// done in mergeApexVariations.
	ApexVariationName string

	// ApiLevel that this module has to support at minimum.
	MinSdkVersion ApiLevel

	// True if this module comes from an updatable apexBundle.
	Updatable bool

	// True if this module can use private platform APIs. Only non-updatable APEX can set this
	// to true.
	UsePlatformApis bool

	// List of Apex variant names that this module is associated with. This initially is the
	// same as the `ApexVariationName` field.  Then when multiple apex variants are merged in
	// mergeApexVariations, ApexInfo struct of the merged variant holds the list of apexBundles
	// that are merged together.
	InApexVariants []string

	// List of APEX Soong module names that this module is part of. Note that the list includes
	// different variations of the same APEX. For example, if module `foo` is included in the
	// apex `com.android.foo`, and also if there is an override_apex module
	// `com.mycompany.android.foo` overriding `com.android.foo`, then this list contains both
	// `com.android.foo` and `com.mycompany.android.foo`.  If the APEX Soong module is a
	// prebuilt, the name here doesn't have the `prebuilt_` prefix.
	InApexModules []string

	// Pointers to the ApexContents struct each of which is for apexBundle modules that this
	// module is part of. The ApexContents gives information about which modules the apexBundle
	// has and whether a module became part of the apexBundle via a direct dependency or not.
	ApexContents []*ApexContents

	// True if this is for a prebuilt_apex.
	//
	// If true then this will customize the apex processing to make it suitable for handling
	// prebuilt_apex, e.g. it will prevent ApexInfos from being merged together.
	//
	// See Prebuilt.ApexInfoMutator for more information.
	ForPrebuiltApex bool

	// Returns the name of the test apexes that this module is included in.
	TestApexes []string
}

var ApexInfoProvider = blueprint.NewMutatorProvider[ApexInfo]("apex")

func (i ApexInfo) AddJSONData(d *map[string]interface{}) {
	(*d)["Apex"] = map[string]interface{}{
		"ApexVariationName": i.ApexVariationName,
		"MinSdkVersion":     i.MinSdkVersion,
		"InApexModules":     i.InApexModules,
		"InApexVariants":    i.InApexVariants,
		"ForPrebuiltApex":   i.ForPrebuiltApex,
	}
}

// mergedName gives the name of the alias variation that will be used when multiple apex variations
// of a module can be deduped into one variation. For example, if libfoo is included in both apex.a
// and apex.b, and if the two APEXes have the same min_sdk_version (say 29), then libfoo doesn't
// have to be built twice, but only once. In that case, the two apex variations apex.a and apex.b
// are configured to have the same alias variation named apex29. Whether platform APIs is allowed
// or not also matters; if two APEXes don't have the same allowance, they get different names and
// thus wouldn't be merged.
func (i ApexInfo) mergedName(ctx PathContext) string {
	name := "apex" + strconv.Itoa(i.MinSdkVersion.FinalOrFutureInt())
	return name
}

// IsForPlatform tells whether this module is for the platform or not. If false is returned, it
// means that this apex variant of the module is built for an APEX.
func (i ApexInfo) IsForPlatform() bool {
	return i.ApexVariationName == ""
}

// InApexVariant tells whether this apex variant of the module is part of the given apexVariant or
// not.
func (i ApexInfo) InApexVariant(apexVariant string) bool {
	for _, a := range i.InApexVariants {
		if a == apexVariant {
			return true
		}
	}
	return false
}

func (i ApexInfo) InApexModule(apexModuleName string) bool {
	for _, a := range i.InApexModules {
		if a == apexModuleName {
			return true
		}
	}
	return false
}

// ApexTestForInfo stores the contents of APEXes for which this module is a test - although this
// module is not part of the APEX - and thus has access to APEX internals.
type ApexTestForInfo struct {
	ApexContents []*ApexContents
}

var ApexTestForInfoProvider = blueprint.NewMutatorProvider[ApexTestForInfo]("apex_test_for")

// ApexBundleInfo contains information about the dependencies of an apex
type ApexBundleInfo struct {
	Contents *ApexContents
}

var ApexBundleInfoProvider = blueprint.NewMutatorProvider[ApexBundleInfo]("apex_info")

// DepIsInSameApex defines an interface that should be used to determine whether a given dependency
// should be considered as part of the same APEX as the current module or not. Note: this was
// extracted from ApexModule to make it easier to define custom subsets of the ApexModule interface
// and improve code navigation within the IDE.
type DepIsInSameApex interface {
	// DepIsInSameApex tests if the other module 'dep' is considered as part of the same APEX as
	// this module. For example, a static lib dependency usually returns true here, while a
	// shared lib dependency to a stub library returns false.
	//
	// This method must not be called directly without first ignoring dependencies whose tags
	// implement ExcludeFromApexContentsTag. Calls from within the func passed to WalkPayloadDeps()
	// are fine as WalkPayloadDeps() will ignore those dependencies automatically. Otherwise, use
	// IsDepInSameApex instead.
	DepIsInSameApex(ctx BaseModuleContext, dep Module) bool
}

func IsDepInSameApex(ctx BaseModuleContext, module, dep Module) bool {
	depTag := ctx.OtherModuleDependencyTag(dep)
	if _, ok := depTag.(ExcludeFromApexContentsTag); ok {
		// The tag defines a dependency that never requires the child module to be part of the same
		// apex as the parent.
		return false
	}
	return module.(DepIsInSameApex).DepIsInSameApex(ctx, dep)
}

// ApexModule is the interface that a module type is expected to implement if the module has to be
// built differently depending on whether the module is destined for an APEX or not (i.e., installed
// to one of the regular partitions).
//
// Native shared libraries are one such module type; when it is built for an APEX, it should depend
// only on stable interfaces such as NDK, stable AIDL, or C APIs from other APEXes.
//
// A module implementing this interface will be mutated into multiple variations by apex.apexMutator
// if it is directly or indirectly included in one or more APEXes. Specifically, if a module is
// included in apex.foo and apex.bar then three apex variants are created: platform, apex.foo and
// apex.bar. The platform variant is for the regular partitions (e.g., /system or /vendor, etc.)
// while the other two are for the APEXs, respectively. The latter two variations can be merged (see
// mergedName) when the two APEXes have the same min_sdk_version requirement.
type ApexModule interface {
	Module
	DepIsInSameApex

	apexModuleBase() *ApexModuleBase

	// Marks that this module should be built for the specified APEX. Call this BEFORE
	// apex.apexMutator is run.
	BuildForApex(apex ApexInfo)

	// Returns true if this module is present in any APEX either directly or indirectly. Call
	// this after apex.apexMutator is run.
	InAnyApex() bool

	// Returns true if this module is directly in any APEX. Call this AFTER apex.apexMutator is
	// run.
	DirectlyInAnyApex() bool

	// NotInPlatform tells whether or not this module is included in an APEX and therefore
	// shouldn't be exposed to the platform (i.e. outside of the APEX) directly. A module is
	// considered to be included in an APEX either when there actually is an APEX that
	// explicitly has the module as its dependency or the module is not available to the
	// platform, which indicates that the module belongs to at least one or more other APEXes.
	NotInPlatform() bool

	// Tests if this module could have APEX variants. Even when a module type implements
	// ApexModule interface, APEX variants are created only for the module instances that return
	// true here. This is useful for not creating APEX variants for certain types of shared
	// libraries such as NDK stubs.
	CanHaveApexVariants() bool

	// Tests if this module can be installed to APEX as a file. For example, this would return
	// true for shared libs while return false for static libs because static libs are not
	// installable module (but it can still be mutated for APEX)
	IsInstallableToApex() bool

	// Tests if this module is available for the specified APEX or ":platform". This is from the
	// apex_available property of the module.
	AvailableFor(what string) bool

	// AlwaysRequiresPlatformApexVariant allows the implementing module to determine whether an
	// APEX mutator should always be created for it.
	//
	// Returns false by default.
	AlwaysRequiresPlatformApexVariant() bool

	// Returns true if this module is not available to platform (i.e. apex_available property
	// doesn't have "//apex_available:platform"), or shouldn't be available to platform, which
	// is the case when this module depends on other module that isn't available to platform.
	NotAvailableForPlatform() bool

	// Marks that this module is not available to platform. Set by the
	// check-platform-availability mutator in the apex package.
	SetNotAvailableForPlatform()

	// Returns the list of APEXes that this module is a test for. The module has access to the
	// private part of the listed APEXes even when it is not included in the APEXes. This by
	// default returns nil. A module type should override the default implementation. For
	// example, cc_test module type returns the value of test_for here.
	TestFor() []string

	// Returns nil (success) if this module should support the given sdk version. Returns an
	// error if not. No default implementation is provided for this method. A module type
	// implementing this interface should provide an implementation. A module supports an sdk
	// version when the module's min_sdk_version is equal to or less than the given sdk version.
	ShouldSupportSdkVersion(ctx BaseModuleContext, sdkVersion ApiLevel) error

	// Returns true if this module needs a unique variation per apex, effectively disabling the
	// deduping. This is turned on when, for example if use_apex_name_macro is set so that each
	// apex variant should be built with different macro definitions.
	UniqueApexVariations() bool
}

// Properties that are common to all module types implementing ApexModule interface.
type ApexProperties struct {
	// Availability of this module in APEXes. Only the listed APEXes can contain this module. If
	// the module has stubs then other APEXes and the platform may access it through them
	// (subject to visibility).
	//
	// "//apex_available:anyapex" is a pseudo APEX name that matches to any APEX.
	// "//apex_available:platform" refers to non-APEX partitions like "system.img".
	// "com.android.gki.*" matches any APEX module name with the prefix "com.android.gki.".
	// Default is ["//apex_available:platform"].
	Apex_available []string

	// See ApexModule.InAnyApex()
	InAnyApex bool `blueprint:"mutated"`

	// See ApexModule.DirectlyInAnyApex()
	DirectlyInAnyApex bool `blueprint:"mutated"`

	// AnyVariantDirectlyInAnyApex is true in the primary variant of a module if _any_ variant
	// of the module is directly in any apex. This includes host, arch, asan, etc. variants. It
	// is unused in any variant that is not the primary variant. Ideally this wouldn't be used,
	// as it incorrectly mixes arch variants if only one arch is in an apex, but a few places
	// depend on it, for example when an ASAN variant is created before the apexMutator. Call
	// this after apex.apexMutator is run.
	AnyVariantDirectlyInAnyApex bool `blueprint:"mutated"`

	// See ApexModule.NotAvailableForPlatform()
	NotAvailableForPlatform bool `blueprint:"mutated"`

	// See ApexModule.UniqueApexVariants()
	UniqueApexVariationsForDeps bool `blueprint:"mutated"`

	// The test apexes that includes this apex variant
	TestApexes []string `blueprint:"mutated"`
}

// Marker interface that identifies dependencies that are excluded from APEX contents.
//
// Unless the tag also implements the AlwaysRequireApexVariantTag this will prevent an apex variant
// from being created for the module.
//
// At the moment the sdk.sdkRequirementsMutator relies on the fact that the existing tags which
// implement this interface do not define dependencies onto members of an sdk_snapshot. If that
// changes then sdk.sdkRequirementsMutator will need fixing.
type ExcludeFromApexContentsTag interface {
	blueprint.DependencyTag

	// Method that differentiates this interface from others.
	ExcludeFromApexContents()
}

// Marker interface that identifies dependencies that always requires an APEX variant to be created.
//
// It is possible for a dependency to require an apex variant but exclude the module from the APEX
// contents. See sdk.sdkMemberDependencyTag.
type AlwaysRequireApexVariantTag interface {
	blueprint.DependencyTag

	// Return true if this tag requires that the target dependency has an apex variant.
	AlwaysRequireApexVariant() bool
}

// Marker interface that identifies dependencies that should inherit the DirectlyInAnyApex state
// from the parent to the child. For example, stubs libraries are marked as DirectlyInAnyApex if
// their implementation is in an apex.
type CopyDirectlyInAnyApexTag interface {
	blueprint.DependencyTag

	// Method that differentiates this interface from others.
	CopyDirectlyInAnyApex()
}

// Interface that identifies dependencies to skip Apex dependency check
type SkipApexAllowedDependenciesCheck interface {
	// Returns true to skip the Apex dependency check, which limits the allowed dependency in build.
	SkipApexAllowedDependenciesCheck() bool
}

// ApexModuleBase provides the default implementation for the ApexModule interface. APEX-aware
// modules are expected to include this struct and call InitApexModule().
type ApexModuleBase struct {
	ApexProperties ApexProperties

	canHaveApexVariants bool

	apexInfos     []ApexInfo
	apexInfosLock sync.Mutex // protects apexInfos during parallel apexInfoMutator
}

// Initializes ApexModuleBase struct. Not calling this (even when inheriting from ApexModuleBase)
// prevents the module from being mutated for apexBundle.
func InitApexModule(m ApexModule) {
	base := m.apexModuleBase()
	base.canHaveApexVariants = true

	m.AddProperties(&base.ApexProperties)
}

// Implements ApexModule
func (m *ApexModuleBase) apexModuleBase() *ApexModuleBase {
	return m
}

var (
	availableToPlatformList = []string{AvailableToPlatform}
)

// Implements ApexModule
func (m *ApexModuleBase) ApexAvailable() []string {
	aa := m.ApexProperties.Apex_available
	if len(aa) > 0 {
		return aa
	}
	// Default is availability to platform
	return CopyOf(availableToPlatformList)
}

// Implements ApexModule
func (m *ApexModuleBase) BuildForApex(apex ApexInfo) {
	m.apexInfosLock.Lock()
	defer m.apexInfosLock.Unlock()
	for i, v := range m.apexInfos {
		if v.ApexVariationName == apex.ApexVariationName {
			if len(apex.InApexModules) != 1 {
				panic(fmt.Errorf("Newly created apexInfo must be for a single APEX"))
			}
			// Even when the ApexVariantNames are the same, the given ApexInfo might
			// actually be for different APEX. This can happen when an APEX is
			// overridden via override_apex. For example, there can be two apexes
			// `com.android.foo` (from the `apex` module type) and
			// `com.mycompany.android.foo` (from the `override_apex` module type), both
			// of which has the same ApexVariantName `com.android.foo`. Add the apex
			// name to the list so that it's not lost.
			if !InList(apex.InApexModules[0], v.InApexModules) {
				m.apexInfos[i].InApexModules = append(m.apexInfos[i].InApexModules, apex.InApexModules[0])
			}
			return
		}
	}
	m.apexInfos = append(m.apexInfos, apex)
}

// Implements ApexModule
func (m *ApexModuleBase) InAnyApex() bool {
	return m.ApexProperties.InAnyApex
}

// Implements ApexModule
func (m *ApexModuleBase) DirectlyInAnyApex() bool {
	return m.ApexProperties.DirectlyInAnyApex
}

// Implements ApexModule
func (m *ApexModuleBase) NotInPlatform() bool {
	return m.ApexProperties.AnyVariantDirectlyInAnyApex || !m.AvailableFor(AvailableToPlatform)
}

// Implements ApexModule
func (m *ApexModuleBase) CanHaveApexVariants() bool {
	return m.canHaveApexVariants
}

// Implements ApexModule
func (m *ApexModuleBase) IsInstallableToApex() bool {
	// If needed, this will bel overridden by concrete types inheriting
	// ApexModuleBase
	return false
}

// Implements ApexModule
func (m *ApexModuleBase) TestFor() []string {
	// If needed, this will be overridden by concrete types inheriting
	// ApexModuleBase
	return nil
}

// Returns the test apexes that this module is included in.
func (m *ApexModuleBase) TestApexes() []string {
	return m.ApexProperties.TestApexes
}

// Implements ApexModule
func (m *ApexModuleBase) UniqueApexVariations() bool {
	// If needed, this will bel overridden by concrete types inheriting
	// ApexModuleBase
	return false
}

// Implements ApexModule
func (m *ApexModuleBase) DepIsInSameApex(ctx BaseModuleContext, dep Module) bool {
	// By default, if there is a dependency from A to B, we try to include both in the same
	// APEX, unless B is explicitly from outside of the APEX (i.e. a stubs lib). Thus, returning
	// true. This is overridden by some module types like apex.ApexBundle, cc.Module,
	// java.Module, etc.
	return true
}

const (
	AvailableToPlatform = "//apex_available:platform"
	AvailableToAnyApex  = "//apex_available:anyapex"
	AvailableToGkiApex  = "com.android.gki.*"
)

var (
	AvailableToRecognziedWildcards = []string{
		AvailableToPlatform,
		AvailableToAnyApex,
		AvailableToGkiApex,
	}
)

// CheckAvailableForApex provides the default algorithm for checking the apex availability. When the
// availability is empty, it defaults to ["//apex_available:platform"] which means "available to the
// platform but not available to any APEX". When the list is not empty, `what` is matched against
// the list. If there is any matching element in the list, thus function returns true. The special
// availability "//apex_available:anyapex" matches with anything except for
// "//apex_available:platform".
func CheckAvailableForApex(what string, apex_available []string) bool {
	if len(apex_available) == 0 {
		return what == AvailableToPlatform
	}
	return InList(what, apex_available) ||
		(what != AvailableToPlatform && InList(AvailableToAnyApex, apex_available)) ||
		(strings.HasPrefix(what, "com.android.gki.") && InList(AvailableToGkiApex, apex_available)) ||
		(what == "com.google.mainline.primary.libs") || // TODO b/248601389
		(what == "com.google.mainline.go.primary.libs") // TODO b/248601389
}

// Implements ApexModule
func (m *ApexModuleBase) AvailableFor(what string) bool {
	return CheckAvailableForApex(what, m.ApexProperties.Apex_available)
}

// Implements ApexModule
func (m *ApexModuleBase) AlwaysRequiresPlatformApexVariant() bool {
	return false
}

// Implements ApexModule
func (m *ApexModuleBase) NotAvailableForPlatform() bool {
	return m.ApexProperties.NotAvailableForPlatform
}

// Implements ApexModule
func (m *ApexModuleBase) SetNotAvailableForPlatform() {
	m.ApexProperties.NotAvailableForPlatform = true
}

// This function makes sure that the apex_available property is valid
func (m *ApexModuleBase) checkApexAvailableProperty(mctx BaseModuleContext) {
	for _, n := range m.ApexProperties.Apex_available {
		if n == AvailableToPlatform || n == AvailableToAnyApex || n == AvailableToGkiApex {
			continue
		}
		if !mctx.OtherModuleExists(n) && !mctx.Config().AllowMissingDependencies() {
			mctx.PropertyErrorf("apex_available", "%q is not a valid module name", n)
		}
	}
}

// AvailableToSameApexes returns true if the two modules are apex_available to
// exactly the same set of APEXes (and platform), i.e. if their apex_available
// properties have the same elements.
func AvailableToSameApexes(mod1, mod2 ApexModule) bool {
	mod1ApexAvail := SortedUniqueStrings(mod1.apexModuleBase().ApexProperties.Apex_available)
	mod2ApexAvail := SortedUniqueStrings(mod2.apexModuleBase().ApexProperties.Apex_available)
	if len(mod1ApexAvail) != len(mod2ApexAvail) {
		return false
	}
	for i, v := range mod1ApexAvail {
		if v != mod2ApexAvail[i] {
			return false
		}
	}
	return true
}

type byApexName []ApexInfo

func (a byApexName) Len() int           { return len(a) }
func (a byApexName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byApexName) Less(i, j int) bool { return a[i].ApexVariationName < a[j].ApexVariationName }

// mergeApexVariations deduplicates apex variations that would build identically into a common
// variation. It returns the reduced list of variations and a list of aliases from the original
// variation names to the new variation names.
func mergeApexVariations(ctx PathContext, apexInfos []ApexInfo) (merged []ApexInfo, aliases [][2]string) {
	sort.Sort(byApexName(apexInfos))
	seen := make(map[string]int)
	for _, apexInfo := range apexInfos {
		// If this is for a prebuilt apex then use the actual name of the apex variation to prevent this
		// from being merged with other ApexInfo. See Prebuilt.ApexInfoMutator for more information.
		if apexInfo.ForPrebuiltApex {
			merged = append(merged, apexInfo)
			continue
		}

		// Merge the ApexInfo together. If a compatible ApexInfo exists then merge the information from
		// this one into it, otherwise create a new merged ApexInfo from this one and save it away so
		// other ApexInfo instances can be merged into it.
		variantName := apexInfo.ApexVariationName
		mergedName := apexInfo.mergedName(ctx)
		if index, exists := seen[mergedName]; exists {
			// Variants having the same mergedName are deduped
			merged[index].InApexVariants = append(merged[index].InApexVariants, variantName)
			merged[index].InApexModules = append(merged[index].InApexModules, apexInfo.InApexModules...)
			merged[index].ApexContents = append(merged[index].ApexContents, apexInfo.ApexContents...)
			merged[index].Updatable = merged[index].Updatable || apexInfo.Updatable
			// Platform APIs is allowed for this module only when all APEXes containing
			// the module are with `use_platform_apis: true`.
			merged[index].UsePlatformApis = merged[index].UsePlatformApis && apexInfo.UsePlatformApis
			merged[index].TestApexes = append(merged[index].TestApexes, apexInfo.TestApexes...)
		} else {
			seen[mergedName] = len(merged)
			apexInfo.ApexVariationName = mergedName
			apexInfo.InApexVariants = CopyOf(apexInfo.InApexVariants)
			apexInfo.InApexModules = CopyOf(apexInfo.InApexModules)
			apexInfo.ApexContents = append([]*ApexContents(nil), apexInfo.ApexContents...)
			apexInfo.TestApexes = CopyOf(apexInfo.TestApexes)
			merged = append(merged, apexInfo)
		}
		aliases = append(aliases, [2]string{variantName, mergedName})
	}
	return merged, aliases
}

// CreateApexVariations mutates a given module into multiple apex variants each of which is for an
// apexBundle (and/or the platform) where the module is part of.
func CreateApexVariations(mctx BottomUpMutatorContext, module ApexModule) []Module {
	base := module.apexModuleBase()

	// Shortcut
	if len(base.apexInfos) == 0 {
		return nil
	}

	// Do some validity checks.
	// TODO(jiyong): is this the right place?
	base.checkApexAvailableProperty(mctx)

	var apexInfos []ApexInfo
	var aliases [][2]string
	if !mctx.Module().(ApexModule).UniqueApexVariations() && !base.ApexProperties.UniqueApexVariationsForDeps {
		apexInfos, aliases = mergeApexVariations(mctx, base.apexInfos)
	} else {
		apexInfos = base.apexInfos
	}
	// base.apexInfos is only needed to propagate the list of apexes from apexInfoMutator to
	// apexMutator. It is no longer accurate after mergeApexVariations, and won't be copied to
	// all but the first created variant. Clear it so it doesn't accidentally get used later.
	base.apexInfos = nil
	sort.Sort(byApexName(apexInfos))

	var inApex ApexMembership
	for _, a := range apexInfos {
		for _, apexContents := range a.ApexContents {
			inApex = inApex.merge(apexContents.contents[mctx.ModuleName()])
		}
	}
	base.ApexProperties.InAnyApex = true
	base.ApexProperties.DirectlyInAnyApex = inApex == directlyInApex

	defaultVariation := ""
	mctx.SetDefaultDependencyVariation(&defaultVariation)

	variations := []string{defaultVariation}
	testApexes := []string{}
	for _, a := range apexInfos {
		variations = append(variations, a.ApexVariationName)
		testApexes = append(testApexes, a.TestApexes...)
	}
	modules := mctx.CreateVariations(variations...)
	for i, mod := range modules {
		platformVariation := i == 0
		if platformVariation && !mctx.Host() && !mod.(ApexModule).AvailableFor(AvailableToPlatform) {
			// Do not install the module for platform, but still allow it to output
			// uninstallable AndroidMk entries in certain cases when they have side
			// effects.  TODO(jiyong): move this routine to somewhere else
			mod.MakeUninstallable()
		}
		if !platformVariation {
			mctx.SetVariationProvider(mod, ApexInfoProvider, apexInfos[i-1])
		}
		// Set the value of TestApexes in every single apex variant.
		// This allows each apex variant to be aware of the test apexes in the user provided apex_available.
		mod.(ApexModule).apexModuleBase().ApexProperties.TestApexes = testApexes
	}

	for _, alias := range aliases {
		mctx.CreateAliasVariation(alias[0], alias[1])
	}

	return modules
}

// UpdateUniqueApexVariationsForDeps sets UniqueApexVariationsForDeps if any dependencies that are
// in the same APEX have unique APEX variations so that the module can link against the right
// variant.
func UpdateUniqueApexVariationsForDeps(mctx BottomUpMutatorContext, am ApexModule) {
	// anyInSameApex returns true if the two ApexInfo lists contain any values in an
	// InApexVariants list in common. It is used instead of DepIsInSameApex because it needs to
	// determine if the dep is in the same APEX due to being directly included, not only if it
	// is included _because_ it is a dependency.
	anyInSameApex := func(a, b []ApexInfo) bool {
		collectApexes := func(infos []ApexInfo) []string {
			var ret []string
			for _, info := range infos {
				ret = append(ret, info.InApexVariants...)
			}
			return ret
		}

		aApexes := collectApexes(a)
		bApexes := collectApexes(b)
		sort.Strings(bApexes)
		for _, aApex := range aApexes {
			index := sort.SearchStrings(bApexes, aApex)
			if index < len(bApexes) && bApexes[index] == aApex {
				return true
			}
		}
		return false
	}

	// If any of the dependencies requires unique apex variations, so does this module.
	mctx.VisitDirectDeps(func(dep Module) {
		if depApexModule, ok := dep.(ApexModule); ok {
			if anyInSameApex(depApexModule.apexModuleBase().apexInfos, am.apexModuleBase().apexInfos) &&
				(depApexModule.UniqueApexVariations() ||
					depApexModule.apexModuleBase().ApexProperties.UniqueApexVariationsForDeps) {
				am.apexModuleBase().ApexProperties.UniqueApexVariationsForDeps = true
			}
		}
	})
}

// UpdateDirectlyInAnyApex uses the final module to store if any variant of this module is directly
// in any APEX, and then copies the final value to all the modules. It also copies the
// DirectlyInAnyApex value to any direct dependencies with a CopyDirectlyInAnyApexTag dependency
// tag.
func UpdateDirectlyInAnyApex(mctx BottomUpMutatorContext, am ApexModule) {
	base := am.apexModuleBase()
	// Copy DirectlyInAnyApex and InAnyApex from any direct dependencies with a
	// CopyDirectlyInAnyApexTag dependency tag.
	mctx.VisitDirectDeps(func(dep Module) {
		if _, ok := mctx.OtherModuleDependencyTag(dep).(CopyDirectlyInAnyApexTag); ok {
			depBase := dep.(ApexModule).apexModuleBase()
			depBase.ApexProperties.DirectlyInAnyApex = base.ApexProperties.DirectlyInAnyApex
			depBase.ApexProperties.InAnyApex = base.ApexProperties.InAnyApex
		}
	})

	if base.ApexProperties.DirectlyInAnyApex {
		// Variants of a module are always visited sequentially in order, so it is safe to
		// write to another variant of this module. For a BottomUpMutator the
		// PrimaryModule() is visited first and FinalModule() is visited last.
		mctx.FinalModule().(ApexModule).apexModuleBase().ApexProperties.AnyVariantDirectlyInAnyApex = true
	}

	// If this is the FinalModule (last visited module) copy
	// AnyVariantDirectlyInAnyApex to all the other variants
	if am == mctx.FinalModule().(ApexModule) {
		mctx.VisitAllModuleVariants(func(variant Module) {
			variant.(ApexModule).apexModuleBase().ApexProperties.AnyVariantDirectlyInAnyApex =
				base.ApexProperties.AnyVariantDirectlyInAnyApex
		})
	}
}

// ApexMembership tells how a module became part of an APEX.
type ApexMembership int

const (
	notInApex        ApexMembership = 0
	indirectlyInApex                = iota
	directlyInApex
)

// ApexContents gives an information about member modules of an apexBundle.  Each apexBundle has an
// apexContents, and modules in that apex have a provider containing the apexContents of each
// apexBundle they are part of.
type ApexContents struct {
	// map from a module name to its membership in this apexBundle
	contents map[string]ApexMembership
}

// NewApexContents creates and initializes an ApexContents that is suitable
// for use with an apex module.
//   - contents is a map from a module name to information about its membership within
//     the apex.
func NewApexContents(contents map[string]ApexMembership) *ApexContents {
	return &ApexContents{
		contents: contents,
	}
}

// Updates an existing membership by adding a new direct (or indirect) membership
func (i ApexMembership) Add(direct bool) ApexMembership {
	if direct || i == directlyInApex {
		return directlyInApex
	}
	return indirectlyInApex
}

// Merges two membership into one. Merging is needed because a module can be a part of an apexBundle
// in many different paths. For example, it could be dependend on by the apexBundle directly, but at
// the same time, there might be an indirect dependency to the module. In that case, the more
// specific dependency (the direct one) is chosen.
func (i ApexMembership) merge(other ApexMembership) ApexMembership {
	if other == directlyInApex || i == directlyInApex {
		return directlyInApex
	}

	if other == indirectlyInApex || i == indirectlyInApex {
		return indirectlyInApex
	}
	return notInApex
}

// Tests whether a module named moduleName is directly included in the apexBundle where this
// ApexContents is tagged.
func (ac *ApexContents) DirectlyInApex(moduleName string) bool {
	return ac.contents[moduleName] == directlyInApex
}

// Tests whether a module named moduleName is included in the apexBundle where this ApexContent is
// tagged.
func (ac *ApexContents) InApex(moduleName string) bool {
	return ac.contents[moduleName] != notInApex
}

// Tests whether a module named moduleName is directly depended on by all APEXes in an ApexInfo.
func DirectlyInAllApexes(apexInfo ApexInfo, moduleName string) bool {
	for _, contents := range apexInfo.ApexContents {
		if !contents.DirectlyInApex(moduleName) {
			return false
		}
	}
	return true
}

////////////////////////////////////////////////////////////////////////////////////////////////////
//Below are routines for extra safety checks.
//
// BuildDepsInfoLists is to flatten the dependency graph for an apexBundle into a text file
// (actually two in slightly different formats). The files are mostly for debugging, for example to
// see why a certain module is included in an APEX via which dependency path.
//
// CheckMinSdkVersion is to make sure that all modules in an apexBundle satisfy the min_sdk_version
// requirement of the apexBundle.

// A dependency info for a single ApexModule, either direct or transitive.
type ApexModuleDepInfo struct {
	// Name of the dependency
	To string
	// List of dependencies To belongs to. Includes APEX itself, if a direct dependency.
	From []string
	// Whether the dependency belongs to the final compiled APEX.
	IsExternal bool
	// min_sdk_version of the ApexModule
	MinSdkVersion string
}

// A map of a dependency name to its ApexModuleDepInfo
type DepNameToDepInfoMap map[string]ApexModuleDepInfo

type ApexBundleDepsInfo struct {
	flatListPath OutputPath
	fullListPath OutputPath
}

type ApexBundleDepsInfoIntf interface {
	Updatable() bool
	FlatListPath() Path
	FullListPath() Path
}

func (d *ApexBundleDepsInfo) FlatListPath() Path {
	return d.flatListPath
}

func (d *ApexBundleDepsInfo) FullListPath() Path {
	return d.fullListPath
}

// Generate two module out files:
// 1. FullList with transitive deps and their parents in the dep graph
// 2. FlatList with a flat list of transitive deps
// In both cases transitive deps of external deps are not included. Neither are deps that are only
// available to APEXes; they are developed with updatability in mind and don't need manual approval.
func (d *ApexBundleDepsInfo) BuildDepsInfoLists(ctx ModuleContext, minSdkVersion string, depInfos DepNameToDepInfoMap) {
	var fullContent strings.Builder
	var flatContent strings.Builder

	fmt.Fprintf(&fullContent, "%s(minSdkVersion:%s):\n", ctx.ModuleName(), minSdkVersion)
	for _, key := range FirstUniqueStrings(SortedKeys(depInfos)) {
		info := depInfos[key]
		toName := fmt.Sprintf("%s(minSdkVersion:%s)", info.To, info.MinSdkVersion)
		if info.IsExternal {
			toName = toName + " (external)"
		}
		fmt.Fprintf(&fullContent, "  %s <- %s\n", toName, strings.Join(SortedUniqueStrings(info.From), ", "))
		fmt.Fprintf(&flatContent, "%s\n", toName)
	}

	d.fullListPath = PathForModuleOut(ctx, "depsinfo", "fulllist.txt").OutputPath
	WriteFileRule(ctx, d.fullListPath, fullContent.String())

	d.flatListPath = PathForModuleOut(ctx, "depsinfo", "flatlist.txt").OutputPath
	WriteFileRule(ctx, d.flatListPath, flatContent.String())

	ctx.Phony(fmt.Sprintf("%s-depsinfo", ctx.ModuleName()), d.fullListPath, d.flatListPath)
}

// Function called while walking an APEX's payload dependencies.
//
// Return true if the `to` module should be visited, false otherwise.
type PayloadDepsCallback func(ctx ModuleContext, from blueprint.Module, to ApexModule, externalDep bool) bool
type WalkPayloadDepsFunc func(ctx ModuleContext, do PayloadDepsCallback)

// ModuleWithMinSdkVersionCheck represents a module that implements min_sdk_version checks
type ModuleWithMinSdkVersionCheck interface {
	Module
	MinSdkVersion(ctx EarlyModuleContext) ApiLevel
	CheckMinSdkVersion(ctx ModuleContext)
}

// CheckMinSdkVersion checks if every dependency of an updatable module sets min_sdk_version
// accordingly
func CheckMinSdkVersion(ctx ModuleContext, minSdkVersion ApiLevel, walk WalkPayloadDepsFunc) {
	// do not enforce min_sdk_version for host
	if ctx.Host() {
		return
	}

	// do not enforce for coverage build
	if ctx.Config().IsEnvTrue("EMMA_INSTRUMENT") || ctx.DeviceConfig().NativeCoverageEnabled() || ctx.DeviceConfig().ClangCoverageEnabled() {
		return
	}

	// do not enforce deps.min_sdk_version if APEX/APK doesn't set min_sdk_version
	if minSdkVersion.IsNone() {
		return
	}

	walk(ctx, func(ctx ModuleContext, from blueprint.Module, to ApexModule, externalDep bool) bool {
		if externalDep {
			// external deps are outside the payload boundary, which is "stable"
			// interface. We don't have to check min_sdk_version for external
			// dependencies.
			return false
		}
		if am, ok := from.(DepIsInSameApex); ok && !am.DepIsInSameApex(ctx, to) {
			return false
		}
		if m, ok := to.(ModuleWithMinSdkVersionCheck); ok {
			// This dependency performs its own min_sdk_version check, just make sure it sets min_sdk_version
			// to trigger the check.
			if !m.MinSdkVersion(ctx).Specified() {
				ctx.OtherModuleErrorf(m, "must set min_sdk_version")
			}
			return false
		}
		if err := to.ShouldSupportSdkVersion(ctx, minSdkVersion); err != nil {
			toName := ctx.OtherModuleName(to)
			ctx.OtherModuleErrorf(to, "should support min_sdk_version(%v) for %q: %v."+
				"\n\nDependency path: %s\n\n"+
				"Consider adding 'min_sdk_version: %q' to %q",
				minSdkVersion, ctx.ModuleName(), err.Error(),
				ctx.GetPathString(false),
				minSdkVersion, toName)
			return false
		}
		return true
	})
}

// Construct ApiLevel object from min_sdk_version string value
func MinSdkVersionFromValue(ctx EarlyModuleContext, value string) ApiLevel {
	if value == "" {
		return NoneApiLevel
	}
	apiLevel, err := ApiLevelFromUser(ctx, value)
	if err != nil {
		ctx.PropertyErrorf("min_sdk_version", "%s", err.Error())
		return NoneApiLevel
	}
	return apiLevel
}

// Implemented by apexBundle.
type ApexTestInterface interface {
	// Return true if the apex bundle is an apex_test
	IsTestApex() bool
}

var ApexExportsInfoProvider = blueprint.NewProvider[ApexExportsInfo]()

// ApexExportsInfo contains information about the artifacts provided by apexes to dexpreopt and hiddenapi
type ApexExportsInfo struct {
	// Canonical name of this APEX. Used to determine the path to the activated APEX on
	// device (/apex/<apex_name>)
	ApexName string

	// Path to the image profile file on host (or empty, if profile is not generated).
	ProfilePathOnHost Path

	// Map from the apex library name (without prebuilt_ prefix) to the dex file path on host
	LibraryNameToDexJarPathOnHost map[string]Path
}
