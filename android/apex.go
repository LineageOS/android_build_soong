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
	"sort"
	"sync"
)

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
	apexModuleBase() *ApexModuleBase

	// Marks that this module should be built for the APEXes of the specified names.
	// Call this before apex.apexMutator is run.
	BuildForApexes(apexNames []string)

	// Returns the name of the APEXes that this modoule will be built for
	ApexVariations() []string

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
	// for an APEX marked via BuildForApexes().
	CreateApexVariations(mctx BottomUpMutatorContext) []Module

	// Sets the name of the apex variant of this module. Called inside
	// CreateApexVariations.
	setApexName(apexName string)

	// Tests if this module is available for the specified APEX or ":platform"
	AvailableFor(what string) bool

	// DepIsInSameApex tests if the other module 'dep' is installed to the same
	// APEX as this module
	DepIsInSameApex(ctx BaseModuleContext, dep Module) bool
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

	// Name of the apex variant that this module is mutated into
	ApexName string `blueprint:"mutated"`
}

// Provides default implementation for the ApexModule interface. APEX-aware
// modules are expected to include this struct and call InitApexModule().
type ApexModuleBase struct {
	ApexProperties ApexProperties

	canHaveApexVariants bool

	apexVariationsLock sync.Mutex // protects apexVariations during parallel apexDepsMutator
	apexVariations     []string
}

func (m *ApexModuleBase) apexModuleBase() *ApexModuleBase {
	return m
}

func (m *ApexModuleBase) BuildForApexes(apexNames []string) {
	m.apexVariationsLock.Lock()
	defer m.apexVariationsLock.Unlock()
	for _, apexName := range apexNames {
		if !InList(apexName, m.apexVariations) {
			m.apexVariations = append(m.apexVariations, apexName)
		}
	}
}

func (m *ApexModuleBase) ApexVariations() []string {
	return m.apexVariations
}

func (m *ApexModuleBase) ApexName() string {
	return m.ApexProperties.ApexName
}

func (m *ApexModuleBase) IsForPlatform() bool {
	return m.ApexProperties.ApexName == ""
}

func (m *ApexModuleBase) setApexName(apexName string) {
	m.ApexProperties.ApexName = apexName
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
	availableToAnyApex  = "//apex_available:anyapex"
)

func CheckAvailableForApex(what string, apex_available []string) bool {
	if len(apex_available) == 0 {
		// apex_available defaults to ["//apex_available:platform"],
		// which means 'available to the platform but no apexes'.
		return what == AvailableToPlatform
	}
	return InList(what, apex_available) ||
		(what != AvailableToPlatform && InList(availableToAnyApex, apex_available))
}

func (m *ApexModuleBase) AvailableFor(what string) bool {
	return CheckAvailableForApex(what, m.ApexProperties.Apex_available)
}

func (m *ApexModuleBase) DepIsInSameApex(ctx BaseModuleContext, dep Module) bool {
	// By default, if there is a dependency from A to B, we try to include both in the same APEX,
	// unless B is explicitly from outside of the APEX (i.e. a stubs lib). Thus, returning true.
	// This is overridden by some module types like apex.ApexBundle, cc.Module, java.Module, etc.
	return true
}

func (m *ApexModuleBase) checkApexAvailableProperty(mctx BaseModuleContext) {
	for _, n := range m.ApexProperties.Apex_available {
		if n == AvailableToPlatform || n == availableToAnyApex {
			continue
		}
		if !mctx.OtherModuleExists(n) && !mctx.Config().AllowMissingDependencies() {
			mctx.PropertyErrorf("apex_available", "%q is not a valid module name", n)
		}
	}
}

func (m *ApexModuleBase) CreateApexVariations(mctx BottomUpMutatorContext) []Module {
	if len(m.apexVariations) > 0 {
		m.checkApexAvailableProperty(mctx)

		sort.Strings(m.apexVariations)
		variations := []string{}
		variations = append(variations, "") // Original variation for platform
		variations = append(variations, m.apexVariations...)

		defaultVariation := ""
		mctx.SetDefaultDependencyVariation(&defaultVariation)

		modules := mctx.CreateVariations(variations...)
		for i, m := range modules {
			platformVariation := i == 0
			if platformVariation && !mctx.Host() && !m.(ApexModule).AvailableFor(AvailableToPlatform) {
				m.SkipInstall()
			}
			m.(ApexModule).setApexName(variations[i])
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
func UpdateApexDependency(apexNames []string, moduleName string, directDep bool) {
	apexNamesMapMutex.Lock()
	defer apexNamesMapMutex.Unlock()
	for _, apexName := range apexNames {
		apexesForModule, ok := apexNamesMap()[moduleName]
		if !ok {
			apexesForModule = make(map[string]bool)
			apexNamesMap()[moduleName] = apexesForModule
		}
		apexesForModule[apexName] = apexesForModule[apexName] || directDep
	}
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
