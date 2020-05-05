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
	"sync/atomic"

	"github.com/google/blueprint"
)

func init() {
	RegisterPackageBuildComponents(InitRegistrationContext)
}

// Register the package module type and supporting mutators.
//
// This must be called in the correct order (relative to other methods that also
// register mutators) to match the order of mutator registration in mutator.go.
// Failing to do so will result in an unrealistic test environment.
func RegisterPackageBuildComponents(ctx RegistrationContext) {
	ctx.RegisterModuleType("package", PackageFactory)

	// Register mutators that are hard coded in to mutator.go.
	ctx.HardCodedPreArchMutators(RegisterPackageRenamer)
}

// The information maintained about each package.
type packageInfo struct {
	// The module from which this information was populated. If `duplicated` = true then this is the
	// module that has been renamed and must be used to report errors.
	module *packageModule

	// If true this indicates that there are two package statements in the same package which is not
	// allowed and will cause the build to fail. This flag is set by packageRenamer and checked in
	// packageErrorReporter
	duplicated bool
}

type packageProperties struct {
	Name string `blueprint:"mutated"`

	// Specifies the default visibility for all modules defined in this package.
	Default_visibility []string
}

type packageModule struct {
	ModuleBase

	properties  packageProperties
	packageInfo *packageInfo
}

func (p *packageModule) GenerateAndroidBuildActions(ModuleContext) {
	// Nothing to do.
}

func (p *packageModule) GenerateBuildActions(ctx blueprint.ModuleContext) {
	// Nothing to do.
}

func (p *packageModule) qualifiedModuleId(ctx BaseModuleContext) qualifiedModuleName {
	// Override to create a package id.
	return newPackageId(ctx.ModuleDir())
}

func (p *packageModule) Name() string {
	return p.properties.Name
}

func (p *packageModule) setName(name string) {
	p.properties.Name = name
}

// Counter to ensure package modules are created with a unique name within whatever namespace they
// belong.
var packageCount uint32 = 0

func PackageFactory() Module {
	module := &packageModule{}

	// Get a unique if for the package. Has to be done atomically as the creation of the modules are
	// done in parallel.
	id := atomic.AddUint32(&packageCount, 1)
	name := fmt.Sprintf("soong_package_%d", id)

	module.properties.Name = name

	module.AddProperties(&module.properties)

	// The default_visibility property needs to be checked and parsed by the visibility module during
	// its checking and parsing phases so make it the primary visibility property.
	setPrimaryVisibilityProperty(module, "default_visibility", &module.properties.Default_visibility)

	return module
}

// Registers the function that renames the packages.
func RegisterPackageRenamer(ctx RegisterMutatorsContext) {
	ctx.BottomUp("packageRenamer", packageRenamer).Parallel()
	ctx.BottomUp("packageErrorReporter", packageErrorReporter).Parallel()
}

// Renames the package to match the package directory.
//
// This also creates a PackageInfo object for each package and uses that to detect and remember
// duplicates for later error reporting.
func packageRenamer(ctx BottomUpMutatorContext) {
	m, ok := ctx.Module().(*packageModule)
	if !ok {
		return
	}

	packageName := "//" + ctx.ModuleDir()

	pi := newPackageInfo(ctx, packageName, m)
	if pi.module != m {
		// Remember that the package was duplicated but do not rename as that will cause an error to
		// be logged with the generated name. Similarly, reporting the error here will use the generated
		// name as renames are only processed after this phase.
		pi.duplicated = true
	} else {
		// This is the first package module in this package so rename it to match the package name.
		m.setName(packageName)
		ctx.Rename(packageName)

		// Store a package info reference in the module.
		m.packageInfo = pi
	}
}

// Logs any deferred errors.
func packageErrorReporter(ctx BottomUpMutatorContext) {
	m, ok := ctx.Module().(*packageModule)
	if !ok {
		return
	}

	packageDir := ctx.ModuleDir()
	packageName := "//" + packageDir

	// Get the PackageInfo for the package. Should have been populated in the packageRenamer phase.
	pi := findPackageInfo(ctx, packageName)
	if pi == nil {
		ctx.ModuleErrorf("internal error, expected package info to be present for package '%s'",
			packageName)
		return
	}

	if pi.module != m {
		// The package module has been duplicated but this is not the module that has been renamed so
		// ignore it. An error will be logged for the renamed module which will ensure that the error
		// message uses the correct name.
		return
	}

	// Check to see whether there are duplicate package modules in the package.
	if pi.duplicated {
		ctx.ModuleErrorf("package {...} specified multiple times")
		return
	}
}

type defaultPackageInfoKey string

func newPackageInfo(
	ctx BaseModuleContext, packageName string, module *packageModule) *packageInfo {
	key := NewCustomOnceKey(defaultPackageInfoKey(packageName))

	return ctx.Config().Once(key, func() interface{} {
		return &packageInfo{module: module}
	}).(*packageInfo)
}

// Get the PackageInfo for the package name (starts with //, no trailing /), is nil if no package
// module type was specified.
func findPackageInfo(ctx BaseModuleContext, packageName string) *packageInfo {
	key := NewCustomOnceKey(defaultPackageInfoKey(packageName))

	pi := ctx.Config().Once(key, func() interface{} {
		return nil
	})

	if pi == nil {
		return nil
	} else {
		return pi.(*packageInfo)
	}
}
