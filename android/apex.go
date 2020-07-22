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

const (
	SdkVersion_Android10 = 29
)

type ApexInfo struct {
	// Name of the apex variant that this module is mutated into
	ApexName string

	MinSdkVersion int
	Updatable     bool
}

// Extracted from ApexModule to make it easier to define custom subsets of the
// ApexModule interface and improve code navigation within the IDE.
type DepIsInSameApex interface {
	// DepIsInSameApex tests if the other module 'dep' is installed to the same
	// APEX as this module
	DepIsInSameApex(ctx BaseModuleContext, dep Module) bool
}

// ApexModule is the interface that a module type is expected to implement if
// the module has to be built differently depending on whether the module
// is destined for an apex or not (installed to one of the regular partitions).
//
// Native shared libraries are one such module type; when it is built for an
// APEX, it should depend only on stable interfaces such as NDK, stable AIDL,
// or C APIs from other APEXs.
//
// A module implementing this interface will be mutated into multiple
// variations by apex.apexMutator if it is directly or indirectly included
// in one or more APEXs. Specifically, if a module is included in apex.foo and
// apex.bar then three apex variants are created: platform, apex.foo and
// apex.bar. The platform variant is for the regular partitions
// (e.g., /system or /vendor, etc.) while the other two are for the APEXs,
// respectively.
type ApexModule interface {
	Module
	DepIsInSameApex

	apexModuleBase() *ApexModuleBase

	// Marks that this module should be built for the specified APEX.
	// Call this before apex.apexMutator is run.
	BuildForApex(apex ApexInfo)

	// Returns the APEXes that this module will be built for
	ApexVariations() []ApexInfo

	// Returns the name of APEX that this module will be built for. Empty string
	// is returned when 'IsForPlatform() == true'. Note that a module can be
	// included in multiple APEXes, in which case, the module is mutated into
	// multiple modules each of which for an APEX. This method returns the
	// name of the APEX that a variant module is for.
	// Call this after apex.apexMutator is run.
	ApexName() string

	// Tests whether this module will be built for the platform or not.
	// This is a shortcut for ApexName() == ""
	IsForPlatform() bool

	// Tests if this module could have APEX variants. APEX variants are
	// created only for the modules that returns true here. This is useful
	// for not creating APEX variants for certain types of shared libraries
	// such as NDK stubs.
	CanHaveApexVariants() bool

	// Tests if this module can be installed to APEX as a file. For example,
	// this would return true for shared libs while return false for static
	// libs.
	IsInstallableToApex() bool

	// Mutate this module into one or more variants each of which is built
	// for an APEX marked via BuildForApex().
	CreateApexVariations(mctx BottomUpMutatorContext) []Module

	// Tests if this module is available for the specified APEX or ":platform"
	AvailableFor(what string) bool

	// Return true if this module is not available to platform (i.e. apex_available
	// property doesn't have "//apex_available:platform"), or shouldn't be available
	// to platform, which is the case when this module depends on other module that
	// isn't available to platform.
	NotAvailableForPlatform() bool

	// Mark that this module is not available to platform. Set by the
	// check-platform-availability mutator in the apex package.
	SetNotAvailableForPlatform()

	// Returns the highest version which is <= maxSdkVersion.
	// For example, with maxSdkVersion is 10 and versionList is [9,11]
	// it returns 9 as string
	ChooseSdkVersion(versionList []string, maxSdkVersion int) (string, error)

	// Tests if the module comes from an updatable APEX.
	Updatable() bool

	// List of APEXes that this module tests. The module has access to
	// the private part of the listed APEXes even when it is not included in the
	// APEXes.
	TestFor() []string
}

type ApexProperties struct {
	// Availability of this module in APEXes. Only the listed APEXes can contain
	// this module. If the module has stubs then other APEXes and the platform may
	// access it through them (subject to visibility).
	//
	// "//apex_available:anyapex" is a pseudo APEX name that matches to any APEX.
	// "//apex_available:platform" refers to non-APEX partitions like "system.img".
	// Default is ["//apex_available:platform"].
	Apex_available []string

	Info ApexInfo `blueprint:"mutated"`

	NotAvailableForPlatform bool `blueprint:"mutated"`
}

// Marker interface that identifies dependencies that are excluded from APEX
// contents.
type ExcludeFromApexContentsTag interface {
	blueprint.DependencyTag

	// Method that differentiates this interface from others.
	ExcludeFromApexContents()
}

// Provides default implementation for the ApexModule interface. APEX-aware
// modules are expected to include this struct and call InitApexModule().
type ApexModuleBase struct {
	ApexProperties ApexProperties

	canHaveApexVariants bool

	apexVariationsLock sync.Mutex // protects apexVariations during parallel apexDepsMutator
	apexVariations     []ApexInfo
}

func (m *ApexModuleBase) apexModuleBase() *ApexModuleBase {
	return m
}

func (m *ApexModuleBase) ApexAvailable() []string {
	return m.ApexProperties.Apex_available
}

func (m *ApexModuleBase) TestFor() []string {
	// To be implemented by concrete types inheriting ApexModuleBase
	return nil
}

func (m *ApexModuleBase) BuildForApex(apex ApexInfo) {
	m.apexVariationsLock.Lock()
	defer m.apexVariationsLock.Unlock()
	for _, v := range m.apexVariations {
		if v.ApexName == apex.ApexName {
			return
		}
	}
	m.apexVariations = append(m.apexVariations, apex)
}

func (m *ApexModuleBase) ApexVariations() []ApexInfo {
	return m.apexVariations
}

func (m *ApexModuleBase) ApexName() string {
	return m.ApexProperties.Info.ApexName
}

func (m *ApexModuleBase) IsForPlatform() bool {
	return m.ApexProperties.Info.ApexName == ""
}

func (m *ApexModuleBase) CanHaveApexVariants() bool {
	return m.canHaveApexVariants
}

func (m *ApexModuleBase) IsInstallableToApex() bool {
	// should be overriden if needed
	return false
}

const (
	AvailableToPlatform = "//apex_available:platform"
	AvailableToAnyApex  = "//apex_available:anyapex"
)

func CheckAvailableForApex(what string, apex_available []string) bool {
	if len(apex_available) == 0 {
		// apex_available defaults to ["//apex_available:platform"],
		// which means 'available to the platform but no apexes'.
		return what == AvailableToPlatform
	}
	return InList(what, apex_available) ||
		(what != AvailableToPlatform && InList(AvailableToAnyApex, apex_available))
}

func (m *ApexModuleBase) AvailableFor(what string) bool {
	return CheckAvailableForApex(what, m.ApexProperties.Apex_available)
}

func (m *ApexModuleBase) NotAvailableForPlatform() bool {
	return m.ApexProperties.NotAvailableForPlatform
}

func (m *ApexModuleBase) SetNotAvailableForPlatform() {
	m.ApexProperties.NotAvailableForPlatform = true
}

func (m *ApexModuleBase) DepIsInSameApex(ctx BaseModuleContext, dep Module) bool {
	// By default, if there is a dependency from A to B, we try to include both in the same APEX,
	// unless B is explicitly from outside of the APEX (i.e. a stubs lib). Thus, returning true.
	// This is overridden by some module types like apex.ApexBundle, cc.Module, java.Module, etc.
	return true
}

func (m *ApexModuleBase) ChooseSdkVersion(versionList []string, maxSdkVersion int) (string, error) {
	for i := range versionList {
		ver, _ := strconv.Atoi(versionList[len(versionList)-i-1])
		if ver <= maxSdkVersion {
			return versionList[len(versionList)-i-1], nil
		}
	}
	return "", fmt.Errorf("not found a version(<=%d) in versionList: %v", maxSdkVersion, versionList)
}

func (m *ApexModuleBase) checkApexAvailableProperty(mctx BaseModuleContext) {
	for _, n := range m.ApexProperties.Apex_available {
		if n == AvailableToPlatform || n == AvailableToAnyApex {
			continue
		}
		if !mctx.OtherModuleExists(n) && !mctx.Config().AllowMissingDependencies() {
			mctx.PropertyErrorf("apex_available", "%q is not a valid module name", n)
		}
	}
}

func (m *ApexModuleBase) Updatable() bool {
	return m.ApexProperties.Info.Updatable
}

type byApexName []ApexInfo

func (a byApexName) Len() int           { return len(a) }
func (a byApexName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byApexName) Less(i, j int) bool { return a[i].ApexName < a[j].ApexName }

func (m *ApexModuleBase) CreateApexVariations(mctx BottomUpMutatorContext) []Module {
	if len(m.apexVariations) > 0 {
		m.checkApexAvailableProperty(mctx)

		sort.Sort(byApexName(m.apexVariations))
		variations := []string{}
		variations = append(variations, "") // Original variation for platform
		for _, apex := range m.apexVariations {
			variations = append(variations, apex.ApexName)
		}

		defaultVariation := ""
		mctx.SetDefaultDependencyVariation(&defaultVariation)

		modules := mctx.CreateVariations(variations...)
		for i, mod := range modules {
			platformVariation := i == 0
			if platformVariation && !mctx.Host() && !mod.(ApexModule).AvailableFor(AvailableToPlatform) {
				mod.SkipInstall()
			}
			if !platformVariation {
				mod.(ApexModule).apexModuleBase().ApexProperties.Info = m.apexVariations[i-1]
			}
		}
		return modules
	}
	return nil
}

var apexData OncePer
var apexNamesMapMutex sync.Mutex
var apexNamesKey = NewOnceKey("apexNames")

// This structure maintains the global mapping in between modules and APEXes.
// Examples:
//
// apexNamesMap()["foo"]["bar"] == true: module foo is directly depended on by APEX bar
// apexNamesMap()["foo"]["bar"] == false: module foo is indirectly depended on by APEX bar
// apexNamesMap()["foo"]["bar"] doesn't exist: foo is not built for APEX bar
func apexNamesMap() map[string]map[string]bool {
	return apexData.Once(apexNamesKey, func() interface{} {
		return make(map[string]map[string]bool)
	}).(map[string]map[string]bool)
}

// Update the map to mark that a module named moduleName is directly or indirectly
// depended on by the specified APEXes. Directly depending means that a module
// is explicitly listed in the build definition of the APEX via properties like
// native_shared_libs, java_libs, etc.
func UpdateApexDependency(apex ApexInfo, moduleName string, directDep bool) {
	apexNamesMapMutex.Lock()
	defer apexNamesMapMutex.Unlock()
	apexesForModule, ok := apexNamesMap()[moduleName]
	if !ok {
		apexesForModule = make(map[string]bool)
		apexNamesMap()[moduleName] = apexesForModule
	}
	apexesForModule[apex.ApexName] = apexesForModule[apex.ApexName] || directDep
}

// TODO(b/146393795): remove this when b/146393795 is fixed
func ClearApexDependency() {
	m := apexNamesMap()
	for k := range m {
		delete(m, k)
	}
}

// Tests whether a module named moduleName is directly depended on by an APEX
// named apexName.
func DirectlyInApex(apexName string, moduleName string) bool {
	apexNamesMapMutex.Lock()
	defer apexNamesMapMutex.Unlock()
	if apexNames, ok := apexNamesMap()[moduleName]; ok {
		return apexNames[apexName]
	}
	return false
}

type hostContext interface {
	Host() bool
}

// Tests whether a module named moduleName is directly depended on by any APEX.
func DirectlyInAnyApex(ctx hostContext, moduleName string) bool {
	if ctx.Host() {
		// Host has no APEX.
		return false
	}
	apexNamesMapMutex.Lock()
	defer apexNamesMapMutex.Unlock()
	if apexNames, ok := apexNamesMap()[moduleName]; ok {
		for an := range apexNames {
			if apexNames[an] {
				return true
			}
		}
	}
	return false
}

// Tests whether a module named module is depended on (including both
// direct and indirect dependencies) by any APEX.
func InAnyApex(moduleName string) bool {
	apexNamesMapMutex.Lock()
	defer apexNamesMapMutex.Unlock()
	apexNames, ok := apexNamesMap()[moduleName]
	return ok && len(apexNames) > 0
}

func GetApexesForModule(moduleName string) []string {
	ret := []string{}
	apexNamesMapMutex.Lock()
	defer apexNamesMapMutex.Unlock()
	if apexNames, ok := apexNamesMap()[moduleName]; ok {
		for an := range apexNames {
			ret = append(ret, an)
		}
	}
	return ret
}

func InitApexModule(m ApexModule) {
	base := m.apexModuleBase()
	base.canHaveApexVariants = true

	m.AddProperties(&base.ApexProperties)
}

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
func (d *ApexBundleDepsInfo) BuildDepsInfoLists(ctx ModuleContext, minSdkVersion string, depInfos DepNameToDepInfoMap) {
	var fullContent strings.Builder
	var flatContent strings.Builder

	fmt.Fprintf(&flatContent, "%s(minSdkVersion:%s):\\n", ctx.ModuleName(), minSdkVersion)
	for _, key := range FirstUniqueStrings(SortedStringKeys(depInfos)) {
		info := depInfos[key]
		toName := fmt.Sprintf("%s(minSdkVersion:%s)", info.To, info.MinSdkVersion)
		if info.IsExternal {
			toName = toName + " (external)"
		}
		fmt.Fprintf(&fullContent, "%s <- %s\\n", toName, strings.Join(SortedUniqueStrings(info.From), ", "))
		fmt.Fprintf(&flatContent, "  %s\\n", toName)
	}

	d.fullListPath = PathForModuleOut(ctx, "depsinfo", "fulllist.txt").OutputPath
	ctx.Build(pctx, BuildParams{
		Rule:        WriteFile,
		Description: "Full Dependency Info",
		Output:      d.fullListPath,
		Args: map[string]string{
			"content": fullContent.String(),
		},
	})

	d.flatListPath = PathForModuleOut(ctx, "depsinfo", "flatlist.txt").OutputPath
	ctx.Build(pctx, BuildParams{
		Rule:        WriteFile,
		Description: "Flat Dependency Info",
		Output:      d.flatListPath,
		Args: map[string]string{
			"content": flatContent.String(),
		},
	})
}
