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
	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

func init() {
	RegisterApexContributionsBuildComponents(InitRegistrationContext)
}

func RegisterApexContributionsBuildComponents(ctx RegistrationContext) {
	ctx.RegisterModuleType("apex_contributions", apexContributionsFactory)
	ctx.RegisterSingletonModuleType("all_apex_contributions", allApexContributionsFactory)
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

// A container for apex_contributions.
// Based on product_config, it will create a dependency on the selected
// apex_contributions per mainline module
type allApexContributions struct {
	SingletonModuleBase
}

func allApexContributionsFactory() SingletonModule {
	module := &allApexContributions{}
	InitAndroidModule(module)
	return module
}

type apexContributionsDepTag struct {
	blueprint.BaseDependencyTag
}

var (
	acDepTag = apexContributionsDepTag{}
)

// Creates a dep to each selected apex_contributions
func (a *allApexContributions) DepsMutator(ctx BottomUpMutatorContext) {
	ctx.AddDependency(ctx.Module(), acDepTag, ctx.Config().AllApexContributions()...)
}

// Set PrebuiltSelectionInfoProvider in post deps phase
func (a *allApexContributions) SetPrebuiltSelectionInfoProvider(ctx BaseModuleContext) {
	addContentsToProvider := func(p *PrebuiltSelectionInfoMap, m *apexContributions) {
		for _, content := range m.Contents() {
			if !ctx.OtherModuleExists(content) && !ctx.Config().AllowMissingDependencies() {
				ctx.ModuleErrorf("%s listed in apex_contributions %s does not exist\n", content, m.Name())
			}
			pi := &PrebuiltSelectionInfo{
				baseModuleName:     RemoveOptionalPrebuiltPrefix(content),
				selectedModuleName: content,
				metadataModuleName: m.Name(),
				apiDomain:          m.ApiDomain(),
			}
			p.Add(ctx, pi)
		}
	}

	p := PrebuiltSelectionInfoMap{}
	ctx.VisitDirectDepsWithTag(acDepTag, func(child Module) {
		if m, ok := child.(*apexContributions); ok {
			addContentsToProvider(&p, m)
		} else {
			ctx.ModuleErrorf("%s is not an apex_contributions module\n", child.Name())
		}
	})
	ctx.SetProvider(PrebuiltSelectionInfoProvider, p)
}

// A provider containing metadata about whether source or prebuilt should be used
// This provider will be used in prebuilt_select mutator to redirect deps
var PrebuiltSelectionInfoProvider = blueprint.NewMutatorProvider(PrebuiltSelectionInfoMap{}, "prebuilt_select")

// Map of baseModuleName to the selected source or prebuilt
type PrebuiltSelectionInfoMap map[string]PrebuiltSelectionInfo

// Add a new entry to the map with some validations
func (pm *PrebuiltSelectionInfoMap) Add(ctx BaseModuleContext, p *PrebuiltSelectionInfo) {
	if p == nil {
		return
	}
	// Do not allow dups. If the base module (without the prebuilt_) has been added before, raise an exception.
	if old, exists := (*pm)[p.baseModuleName]; exists {
		ctx.ModuleErrorf("Cannot use Soong module: %s from apex_contributions: %s because it has been added previously as: %s from apex_contributions: %s\n",
			p.selectedModuleName, p.metadataModuleName, old.selectedModuleName, old.metadataModuleName,
		)
	}
	(*pm)[p.baseModuleName] = *p
}

type PrebuiltSelectionInfo struct {
	// e.g. libc
	baseModuleName string
	// e.g. (libc|prebuilt_libc)
	selectedModuleName string
	// Name of the apex_contributions module
	metadataModuleName string
	// e.g. com.android.runtime
	apiDomain string
}

// Returns true if `name` is explicitly requested using one of the selected
// apex_contributions metadata modules.
func (p *PrebuiltSelectionInfoMap) IsSelected(baseModuleName, name string) bool {
	if i, exists := (*p)[baseModuleName]; exists {
		return i.selectedModuleName == name
	} else {
		return false
	}
}

// This module type does not have any build actions.
func (a *allApexContributions) GenerateAndroidBuildActions(ctx ModuleContext) {
}

func (a *allApexContributions) GenerateSingletonBuildActions(ctx SingletonContext) {
}
