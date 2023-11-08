// Copyright 2023 Google Inc. All rights reserved.
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
	"github.com/google/blueprint/proptools"
)

func init() {
	RegisterApexContributionsBuildComponents(InitRegistrationContext)
}

func RegisterApexContributionsBuildComponents(ctx RegistrationContext) {
	ctx.RegisterModuleType("apex_contributions", apexContributionsFactory)
}

type apexContributions struct {
	ModuleBase
	properties contributionProps
}

type contributionProps struct {
	// Name of the mainline module
	Api_domain *string
	// A list of module names that should be used when this contribution
	// is selected via product_config
	// The name should be explicit (foo or prebuilt_foo)
	Contents []string
}

func (m *apexContributions) ApiDomain() string {
	return proptools.String(m.properties.Api_domain)
}

func (m *apexContributions) Contents() []string {
	return m.properties.Contents
}

// apex_contributions contains a list of module names (source or
// prebuilt) belonging to the mainline module
// An apex can have multiple apex_contributions modules
// with different combinations of source or prebuilts, but only one can be
// selected via product_config.
func apexContributionsFactory() Module {
	module := &apexContributions{}
	module.AddProperties(&module.properties)
	InitAndroidModule(module)
	return module
}

// This module type does not have any build actions.
// It provides metadata that is used in post-deps mutator phase for source vs
// prebuilts selection.
func (m *apexContributions) GenerateAndroidBuildActions(ctx ModuleContext) {
}
