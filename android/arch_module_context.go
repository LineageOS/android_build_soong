// Copyright 2024 Google Inc. All rights reserved.
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

// ArchModuleContext can be embedded in other contexts to provide information about the module set by
// the archMutator.
type ArchModuleContext interface {
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

type archModuleContext struct {
	// TODO: these should eventually go through a (possibly cached) provider like any other configuration instead
	//  of being special cased.
	os            OsType
	target        Target
	targetPrimary bool
	multiTargets  []Target
	primaryArch   bool
}

func (a *archModuleContext) Target() Target {
	return a.target
}

func (a *archModuleContext) TargetPrimary() bool {
	return a.targetPrimary
}

func (a *archModuleContext) MultiTargets() []Target {
	return a.multiTargets
}

func (a *archModuleContext) Arch() Arch {
	return a.target.Arch
}

func (a *archModuleContext) Os() OsType {
	return a.os
}

func (a *archModuleContext) Host() bool {
	return a.os.Class == Host
}

func (a *archModuleContext) Device() bool {
	return a.os.Class == Device
}

func (a *archModuleContext) Darwin() bool {
	return a.os == Darwin
}

func (a *archModuleContext) Windows() bool {
	return a.os == Windows
}

func (b *archModuleContext) PrimaryArch() bool {
	return b.primaryArch
}
