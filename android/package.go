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
	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

func init() {
	RegisterPackageBuildComponents(InitRegistrationContext)
}

// Register the package module type.
func RegisterPackageBuildComponents(ctx RegistrationContext) {
	ctx.RegisterModuleType("package", PackageFactory)
}

type packageProperties struct {
	// Specifies the default visibility for all modules defined in this package.
	Default_visibility []string
	// Specifies the names of the default licenses for all modules defined in this package.
	Default_applicable_licenses []string
}

type packageModule struct {
	ModuleBase

	properties packageProperties
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

func PackageFactory() Module {
	module := &packageModule{}

	module.AddProperties(&module.properties)

	// The name is the relative path from build root to the directory containing this
	// module. Set that name at the earliest possible moment that information is available
	// which is in a LoadHook.
	AddLoadHook(module, func(ctx LoadHookContext) {
		module.nameProperties.Name = proptools.StringPtr("//" + ctx.ModuleDir())
	})

	// The default_visibility property needs to be checked and parsed by the visibility module during
	// its checking and parsing phases so make it the primary visibility property.
	setPrimaryVisibilityProperty(module, "default_visibility", &module.properties.Default_visibility)

	return module
}
