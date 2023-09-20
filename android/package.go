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
	"path/filepath"

	"android/soong/bazel"
	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

func init() {
	RegisterPackageBuildComponents(InitRegistrationContext)
}

var PrepareForTestWithPackageModule = FixtureRegisterWithContext(RegisterPackageBuildComponents)

// Register the package module type.
func RegisterPackageBuildComponents(ctx RegistrationContext) {
	ctx.RegisterModuleType("package", PackageFactory)
}

type packageProperties struct {
	// Specifies the default visibility for all modules defined in this package.
	Default_visibility []string
	// Specifies the default license terms for all modules defined in this package.
	Default_applicable_licenses []string
}

type bazelPackageAttributes struct {
	Default_visibility       []string
	Default_package_metadata bazel.LabelListAttribute
}

type packageModule struct {
	ModuleBase
	BazelModuleBase

	properties packageProperties
}

var _ Bazelable = &packageModule{}

func (p *packageModule) ConvertWithBp2build(ctx Bp2buildMutatorContext) {
	defaultPackageMetadata := bazel.MakeLabelListAttribute(BazelLabelForModuleDeps(ctx, p.properties.Default_applicable_licenses))
	// If METADATA file exists in the package, add it to package(default_package_metadata=) using a
	// filegroup(name="default_metadata_file") which can be accessed later on each module in Bazel
	// using attribute "applicable_licenses".
	// Attribute applicable_licenses of filegroup "default_metadata_file" has to be set to [],
	// otherwise Bazel reports cyclic reference error.
	if existed, _, _ := ctx.Config().fs.Exists(filepath.Join(ctx.ModuleDir(), "METADATA")); existed {
		ctx.CreateBazelTargetModule(
			bazel.BazelTargetModuleProperties{
				Rule_class: "filegroup",
			},
			CommonAttributes{Name: "default_metadata_file"},
			&bazelFilegroupAttributes{
				Srcs:                bazel.MakeLabelListAttribute(BazelLabelForModuleSrc(ctx, []string{"METADATA"})),
				Applicable_licenses: bazel.LabelListAttribute{Value: bazel.LabelList{Includes: []bazel.Label{}}, EmitEmptyList: true},
			})
		defaultPackageMetadata.Value.Add(&bazel.Label{Label: ":default_metadata_file"})
	}

	ctx.CreateBazelTargetModule(
		bazel.BazelTargetModuleProperties{
			Rule_class: "package",
		},
		CommonAttributes{},
		&bazelPackageAttributes{
			Default_package_metadata: defaultPackageMetadata,
			// FIXME(asmundak): once b/221436821 is resolved
			Default_visibility: []string{"//visibility:public"},
		})
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

	module.AddProperties(&module.properties, &module.commonProperties.BazelConversionStatus)

	// The name is the relative path from build root to the directory containing this
	// module. Set that name at the earliest possible moment that information is available
	// which is in a LoadHook.
	AddLoadHook(module, func(ctx LoadHookContext) {
		module.nameProperties.Name = proptools.StringPtr("//" + ctx.ModuleDir())
	})

	// The default_visibility property needs to be checked and parsed by the visibility module during
	// its checking and parsing phases so make it the primary visibility property.
	setPrimaryVisibilityProperty(module, "default_visibility", &module.properties.Default_visibility)

	// The default_applicable_licenses property needs to be checked and parsed by the licenses module during
	// its checking and parsing phases so make it the primary licenses property.
	setPrimaryLicensesProperty(module, "default_applicable_licenses", &module.properties.Default_applicable_licenses)

	InitBazelModule(module)

	return module
}
