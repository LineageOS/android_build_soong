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
	// Do nothing.
}

func (m *licenseModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	// Nothing to do.
}

func LicenseFactory() Module {
	module := &licenseModule{}

	base := module.base()
	module.AddProperties(&base.nameProperties, &module.properties)

	base.generalProperties = module.GetProperties()
	base.customizableProperties = module.GetProperties()

	// The visibility property needs to be checked and parsed by the visibility module.
	setPrimaryVisibilityProperty(module, "visibility", &module.properties.Visibility)

	initAndroidModuleBase(module)
	InitDefaultableModule(module)

	return module
}
