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

// ApexModule is the interface that a module type is expected to implement if
// the module has to be built differently depending on whether the module
// is destined for an apex or not (installed to one of the regular partitions).
//
// Native shared libraries are one such module type; when it is built for an
// APEX, it should depend only on stable interfaces such as NDK, stable AIDL,
// or C APIs from other APEXs.
//
// A module implementing this interface will be mutated into multiple
// variations by the apex mutator if it is directly or indirectly included
// in one or more APEXs. Specifically, if a module is included in apex.foo and
// apex.bar then three apex variants are created: platform, apex.foo and
// apex.bar. The platform variant is for the regular partitions
// (e.g., /system or /vendor, etc.) while the other two are for the APEXs,
// respectively.
type ApexModule interface {
	Module
	apexModuleBase() *ApexModuleBase

	// Marks that this module should be built for the APEX of the specified name
	BuildForApex(apexName string)

	// Tests whether this module will be built for the platform or not (= APEXs)
	IsForPlatform() bool

	// Returns the name of APEX that this module will be built for. Empty string
	// is returned when 'IsForPlatform() == true'. Note that a module can be
	// included to multiple APEXs, in which case, the module is mutated into
	// multiple modules each of which for an APEX. This method returns the
	// name of the APEX that a variant module is for.
	ApexName() string

	// Tests if this module can have APEX variants. APEX variants are
	// created only for the modules that returns true here. This is useful
	// for not creating APEX variants for shared libraries such as NDK stubs.
	CanHaveApexVariants() bool

	// Tests if this module can be installed to APEX as a file. For example,
	// this would return true for shared libs while return false for static
	// libs.
	IsInstallableToApex() bool
}

type ApexProperties struct {
	ApexName string `blueprint:"mutated"`
}

// Provides default implementation for the ApexModule interface. APEX-aware
// modules are expected to include this struct and call InitApexModule().
type ApexModuleBase struct {
	ApexProperties ApexProperties

	canHaveApexVariants bool
}

func (m *ApexModuleBase) apexModuleBase() *ApexModuleBase {
	return m
}

func (m *ApexModuleBase) BuildForApex(apexName string) {
	m.ApexProperties.ApexName = apexName
}

func (m *ApexModuleBase) IsForPlatform() bool {
	return m.ApexProperties.ApexName == ""
}

func (m *ApexModuleBase) ApexName() string {
	return m.ApexProperties.ApexName
}

func (m *ApexModuleBase) CanHaveApexVariants() bool {
	return m.canHaveApexVariants
}

func (m *ApexModuleBase) IsInstallableToApex() bool {
	// should be overriden if needed
	return false
}

func InitApexModule(m ApexModule) {
	base := m.apexModuleBase()
	base.canHaveApexVariants = true

	m.AddProperties(&base.ApexProperties)
}
