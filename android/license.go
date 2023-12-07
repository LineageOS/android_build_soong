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

package android

import (
	"github.com/google/blueprint"
)

type licenseKindDependencyTag struct {
	blueprint.BaseDependencyTag
}

var (
	licenseKindTag = licenseKindDependencyTag{}
)

func init() {
	RegisterLicenseBuildComponents(InitRegistrationContext)
}

// Register the license module type.
func RegisterLicenseBuildComponents(ctx RegistrationContext) {
	ctx.RegisterModuleType("license", LicenseFactory)
}

type licenseProperties struct {
	// Specifies the kinds of license that apply.
	License_kinds []string
	// Specifies a short copyright notice to use for the license.
	Copyright_notice *string
	// Specifies the path or label for the text of the license.
	License_text []string `android:"path"`
	// Specifies the package name to which the license applies.
	Package_name *string
	// Specifies where this license can be used
	Visibility []string
}

type licenseModule struct {
	ModuleBase
	DefaultableModuleBase

	properties licenseProperties
}

func (m *licenseModule) DepsMutator(ctx BottomUpMutatorContext) {
	for i, license := range m.properties.License_kinds {
		for j := i + 1; j < len(m.properties.License_kinds); j++ {
			if license == m.properties.License_kinds[j] {
				ctx.ModuleErrorf("Duplicated license kind: %q", license)
				break
			}
		}
	}
	ctx.AddVariationDependencies(nil, licenseKindTag, m.properties.License_kinds...)
}

func (m *licenseModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	// license modules have no licenses, but license_kinds must refer to license_kind modules
	mergeStringProps(&m.base().commonProperties.Effective_licenses, ctx.ModuleName())
	namePathProps(&m.base().commonProperties.Effective_license_text, m.properties.Package_name, PathsForModuleSrc(ctx, m.properties.License_text)...)
	for _, module := range ctx.GetDirectDepsWithTag(licenseKindTag) {
		if lk, ok := module.(*licenseKindModule); ok {
			mergeStringProps(&m.base().commonProperties.Effective_license_conditions, lk.properties.Conditions...)
			mergeStringProps(&m.base().commonProperties.Effective_license_kinds, ctx.OtherModuleName(module))
		} else {
			ctx.ModuleErrorf("license_kinds property %q is not a license_kind module", ctx.OtherModuleName(module))
		}
	}
}

func LicenseFactory() Module {
	module := &licenseModule{}

	base := module.base()
	module.AddProperties(&base.nameProperties, &module.properties)

	// The visibility property needs to be checked and parsed by the visibility module.
	setPrimaryVisibilityProperty(module, "visibility", &module.properties.Visibility)

	initAndroidModuleBase(module)
	InitDefaultableModule(module)

	return module
}
