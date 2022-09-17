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

import "android/soong/bazel"

func init() {
	RegisterLicenseKindBuildComponents(InitRegistrationContext)
}

// Register the license_kind module type.
func RegisterLicenseKindBuildComponents(ctx RegistrationContext) {
	ctx.RegisterModuleType("license_kind", LicenseKindFactory)
}

type licenseKindProperties struct {
	// Specifies the conditions for all licenses of the kind.
	Conditions []string
	// Specifies the url to the canonical license definition.
	Url string
	// Specifies where this license can be used
	Visibility []string
}

var _ Bazelable = &licenseKindModule{}

type licenseKindModule struct {
	ModuleBase
	DefaultableModuleBase
	BazelModuleBase

	properties licenseKindProperties
}

type bazelLicenseKindAttributes struct {
	Conditions []string
	Url        string
	Visibility []string
}

func (m *licenseKindModule) ConvertWithBp2build(ctx TopDownMutatorContext) {
	attrs := &bazelLicenseKindAttributes{
		Conditions: m.properties.Conditions,
		Url:        m.properties.Url,
		Visibility: m.properties.Visibility,
	}
	ctx.CreateBazelTargetModule(
		bazel.BazelTargetModuleProperties{
			Rule_class:        "license_kind",
			Bzl_load_location: "@rules_license//rules:license_kind.bzl",
		},
		CommonAttributes{
			Name: m.Name(),
		},
		attrs)
}

func (m *licenseKindModule) DepsMutator(ctx BottomUpMutatorContext) {
	// Nothing to do.
}

func (m *licenseKindModule) GenerateAndroidBuildActions(ModuleContext) {
	// Nothing to do.
}

func LicenseKindFactory() Module {
	module := &licenseKindModule{}

	base := module.base()
	module.AddProperties(&base.nameProperties, &module.properties, &base.commonProperties.BazelConversionStatus)

	// The visibility property needs to be checked and parsed by the visibility module.
	setPrimaryVisibilityProperty(module, "visibility", &module.properties.Visibility)

	initAndroidModuleBase(module)
	InitDefaultableModule(module)
	InitBazelModule(module)

	return module
}
