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

type licenseKindModule struct {
	ModuleBase
	DefaultableModuleBase

	properties licenseKindProperties
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
	module.AddProperties(&base.nameProperties, &module.properties)

	// The visibility property needs to be checked and parsed by the visibility module.
	setPrimaryVisibilityProperty(module, "visibility", &module.properties.Visibility)

	initAndroidModuleBase(module)
	InitDefaultableModule(module)

	return module
}
