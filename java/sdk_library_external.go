// Copyright 2020 Google Inc. All rights reserved.
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

package java

import (
	"android/soong/android"
)

type partitionGroup int

// Representation of partition group for checking inter-partition library dependencies.
// Between system and system_ext, there are no restrictions of dependencies,
// so we can treat these partitions as the same in terms of inter-partition dependency.
// Same policy is applied between vendor and odm partiton.
const (
	partitionGroupNone partitionGroup = iota
	// group for system, and system_ext partition
	partitionGroupSystem
	// group for vendor and odm partition
	partitionGroupVendor
	// product partition
	partitionGroupProduct
)

func (g partitionGroup) String() string {
	switch g {
	case partitionGroupSystem:
		return "system"
	case partitionGroupVendor:
		return "vendor"
	case partitionGroupProduct:
		return "product"
	}

	return ""
}

// Get partition group of java module that can be used at inter-partition dependency check.
// We currently have three groups
//   (system, system_ext) => system partition group
//   (vendor, odm) => vendor partition group
//   (product) => product partition group
func (j *Module) partitionGroup(ctx android.EarlyModuleContext) partitionGroup {
	// system and system_ext partition can be treated as the same in terms of inter-partition dependency.
	if j.Platform() || j.SystemExtSpecific() {
		return partitionGroupSystem
	}

	// vendor and odm partition can be treated as the same in terms of inter-partition dependency.
	if j.SocSpecific() || j.DeviceSpecific() {
		return partitionGroupVendor
	}

	// product partition is independent.
	if j.ProductSpecific() {
		return partitionGroupProduct
	}

	panic("Cannot determine partition type")
}

func (j *Module) allowListedInterPartitionJavaLibrary(ctx android.EarlyModuleContext) bool {
	return inList(j.Name(), ctx.Config().InterPartitionJavaLibraryAllowList())
}

func (j *Module) syspropWithPublicStubs() bool {
	return j.deviceProperties.SyspropPublicStub != ""
}

type javaSdkLibraryEnforceContext interface {
	Name() string
	allowListedInterPartitionJavaLibrary(ctx android.EarlyModuleContext) bool
	partitionGroup(ctx android.EarlyModuleContext) partitionGroup
	syspropWithPublicStubs() bool
}

var _ javaSdkLibraryEnforceContext = (*Module)(nil)

func (j *Module) checkPartitionsForJavaDependency(ctx android.EarlyModuleContext, propName string, dep javaSdkLibraryEnforceContext) {
	if dep.allowListedInterPartitionJavaLibrary(ctx) {
		return
	}

	if dep.syspropWithPublicStubs() {
		return
	}

	// If product interface is not enforced, skip check between system and product partition.
	// But still need to check between product and vendor partition because product interface flag
	// just represents enforcement between product and system, and vendor interface enforcement
	// that is enforced here by precondition is representing enforcement between vendor and other partitions.
	if !ctx.Config().EnforceProductPartitionInterface() {
		productToSystem := j.partitionGroup(ctx) == partitionGroupProduct && dep.partitionGroup(ctx) == partitionGroupSystem
		systemToProduct := j.partitionGroup(ctx) == partitionGroupSystem && dep.partitionGroup(ctx) == partitionGroupProduct

		if productToSystem || systemToProduct {
			return
		}
	}

	// If module and dependency library is inter-partition
	if j.partitionGroup(ctx) != dep.partitionGroup(ctx) {
		errorFormat := "dependency on java_library (%q) is not allowed across the partitions (%s -> %s), use java_sdk_library instead"
		ctx.PropertyErrorf(propName, errorFormat, dep.Name(), j.partitionGroup(ctx), dep.partitionGroup(ctx))
	}
}
