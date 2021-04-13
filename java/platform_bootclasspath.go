// Copyright 2021 Google Inc. All rights reserved.
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
	"fmt"

	"android/soong/android"
	"android/soong/dexpreopt"
	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

func init() {
	registerPlatformBootclasspathBuildComponents(android.InitRegistrationContext)
}

func registerPlatformBootclasspathBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("platform_bootclasspath", platformBootclasspathFactory)

	ctx.FinalDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("platform_bootclasspath_deps", platformBootclasspathDepsMutator)
	})
}

type platformBootclasspathDependencyTag struct {
	blueprint.BaseDependencyTag

	name string
}

// Avoid having to make platform bootclasspath content visible to the platform bootclasspath.
//
// This is a temporary workaround to make it easier to migrate to platform bootclasspath with proper
// dependencies.
// TODO(b/177892522): Remove this and add needed visibility.
func (t platformBootclasspathDependencyTag) ExcludeFromVisibilityEnforcement() {
}

// The tag used for the dependency between the platform bootclasspath and any configured boot jars.
var platformBootclasspathModuleDepTag = platformBootclasspathDependencyTag{name: "module"}

// The tag used for the dependency between the platform bootclasspath and bootclasspath_fragments.
var platformBootclasspathFragmentDepTag = platformBootclasspathDependencyTag{name: "fragment"}

var _ android.ExcludeFromVisibilityEnforcementTag = platformBootclasspathDependencyTag{}

type platformBootclasspathModule struct {
	android.ModuleBase

	properties platformBootclasspathProperties

	// The apex:module pairs obtained from the configured modules.
	//
	// Currently only for testing.
	configuredModules []android.Module

	// The apex:module pairs obtained from the fragments.
	//
	// Currently only for testing.
	fragments []android.Module

	// Path to the monolithic hiddenapi-flags.csv file.
	hiddenAPIFlagsCSV android.Path

	// Path to the monolithic hiddenapi-index.csv file.
	hiddenAPIIndexCSV android.Path

	// Path to the monolithic hiddenapi-unsupported.csv file.
	hiddenAPIMetadataCSV android.Path
}

// ApexVariantReference specifies a particular apex variant of a module.
type ApexVariantReference struct {
	// The name of the module apex variant, i.e. the apex containing the module variant.
	//
	// If this is not specified then it defaults to "platform" which will cause a dependency to be
	// added to the module's platform variant.
	Apex *string

	// The name of the module.
	Module *string
}

type platformBootclasspathProperties struct {
	// The names of the bootclasspath_fragment modules that form part of this
	// platform_bootclasspath.
	Fragments []ApexVariantReference

	Hidden_api HiddenAPIAugmentationProperties
}

func platformBootclasspathFactory() android.Module {
	m := &platformBootclasspathModule{}
	m.AddProperties(&m.properties)
	android.InitAndroidArchModule(m, android.DeviceSupported, android.MultilibCommon)
	return m
}

var _ android.OutputFileProducer = (*platformBootclasspathModule)(nil)

// A minimal AndroidMkEntries is needed in order to support the dists property.
func (b *platformBootclasspathModule) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{
		{
			Class: "FAKE",
			// Need at least one output file in order for this to take effect.
			OutputFile: android.OptionalPathForPath(b.hiddenAPIFlagsCSV),
			Include:    "$(BUILD_PHONY_PACKAGE)",
		},
	}
}

// Make the hidden API files available from the platform-bootclasspath module.
func (b *platformBootclasspathModule) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "hiddenapi-flags.csv":
		return android.Paths{b.hiddenAPIFlagsCSV}, nil
	case "hiddenapi-index.csv":
		return android.Paths{b.hiddenAPIIndexCSV}, nil
	case "hiddenapi-metadata.csv":
		return android.Paths{b.hiddenAPIMetadataCSV}, nil
	}

	return nil, fmt.Errorf("unknown tag %s", tag)
}

func (b *platformBootclasspathModule) DepsMutator(ctx android.BottomUpMutatorContext) {
	if SkipDexpreoptBootJars(ctx) {
		return
	}

	// Add a dependency onto the dex2oat tool which is needed for creating the boot image. The
	// path is retrieved from the dependency by GetGlobalSoongConfig(ctx).
	dexpreopt.RegisterToolDeps(ctx)
}

func platformBootclasspathDepsMutator(ctx android.BottomUpMutatorContext) {
	m := ctx.Module()
	if p, ok := m.(*platformBootclasspathModule); ok {
		// Add dependencies on all the modules configured in the "art" boot image.
		artImageConfig := genBootImageConfigs(ctx)[artBootImageName]
		addDependenciesOntoBootImageModules(ctx, artImageConfig.modules)

		// Add dependencies on all the modules configured in the "boot" boot image. That does not
		// include modules configured in the "art" boot image.
		bootImageConfig := p.getImageConfig(ctx)
		addDependenciesOntoBootImageModules(ctx, bootImageConfig.modules)

		// Add dependencies on all the updatable modules.
		updatableModules := dexpreopt.GetGlobalConfig(ctx).UpdatableBootJars
		addDependenciesOntoBootImageModules(ctx, updatableModules)

		// Add dependencies on all the fragments.
		addDependencyOntoApexVariants(ctx, "fragments", p.properties.Fragments, platformBootclasspathFragmentDepTag)
	}
}

func addDependencyOntoApexVariants(ctx android.BottomUpMutatorContext, propertyName string, refs []ApexVariantReference, tag blueprint.DependencyTag) {
	for i, ref := range refs {
		apex := proptools.StringDefault(ref.Apex, "platform")

		if ref.Module == nil {
			ctx.PropertyErrorf(propertyName, "missing module name at position %d", i)
			continue
		}
		name := proptools.String(ref.Module)

		addDependencyOntoApexModulePair(ctx, apex, name, tag)
	}
}

func addDependencyOntoApexModulePair(ctx android.BottomUpMutatorContext, apex string, name string, tag blueprint.DependencyTag) {
	var variations []blueprint.Variation
	if apex != "platform" {
		// Pick the correct apex variant.
		variations = []blueprint.Variation{
			{Mutator: "apex", Variation: apex},
		}
	}

	addedDep := false
	if ctx.OtherModuleDependencyVariantExists(variations, name) {
		ctx.AddFarVariationDependencies(variations, tag, name)
		addedDep = true
	}

	// Add a dependency on the prebuilt module if it exists.
	prebuiltName := android.PrebuiltNameFromSource(name)
	if ctx.OtherModuleDependencyVariantExists(variations, prebuiltName) {
		ctx.AddVariationDependencies(variations, tag, prebuiltName)
		addedDep = true
	}

	// If no appropriate variant existing for this, so no dependency could be added, then it is an
	// error, unless missing dependencies are allowed. The simplest way to handle that is to add a
	// dependency that will not be satisfied and the default behavior will handle it.
	if !addedDep {
		ctx.AddFarVariationDependencies(variations, tag, name)
	}
}

func addDependenciesOntoBootImageModules(ctx android.BottomUpMutatorContext, modules android.ConfiguredJarList) {
	for i := 0; i < modules.Len(); i++ {
		apex := modules.Apex(i)
		name := modules.Jar(i)

		addDependencyOntoApexModulePair(ctx, apex, name, platformBootclasspathModuleDepTag)
	}
}

func (b *platformBootclasspathModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	ctx.VisitDirectDepsIf(isActiveModule, func(module android.Module) {
		tag := ctx.OtherModuleDependencyTag(module)
		if tag == platformBootclasspathModuleDepTag {
			b.configuredModules = append(b.configuredModules, module)
		} else if tag == platformBootclasspathFragmentDepTag {
			b.fragments = append(b.fragments, module)
		}
	})

	b.generateHiddenAPIBuildActions(ctx, b.configuredModules)

	// Nothing to do if skipping the dexpreopt of boot image jars.
	if SkipDexpreoptBootJars(ctx) {
		return
	}

	// Force the GlobalSoongConfig to be created and cached for use by the dex_bootjars
	// GenerateSingletonBuildActions method as it cannot create it for itself.
	dexpreopt.GetGlobalSoongConfig(ctx)

	imageConfig := b.getImageConfig(ctx)
	if imageConfig == nil {
		return
	}

	// Construct the boot image info from the config.
	info := BootImageInfo{imageConfig: imageConfig}

	// Make it available for other modules.
	ctx.SetProvider(BootImageInfoProvider, info)
}

func (b *platformBootclasspathModule) getImageConfig(ctx android.EarlyModuleContext) *bootImageConfig {
	return defaultBootImageConfig(ctx)
}

// generateHiddenAPIBuildActions generates all the hidden API related build rules.
func (b *platformBootclasspathModule) generateHiddenAPIBuildActions(ctx android.ModuleContext, modules []android.Module) {

	// Save the paths to the monolithic files for retrieval via OutputFiles()
	// Make the paths relative to the out/soong/hiddenapi directory instead of to the out/soong/
	// directory. This ensures that if they are used as java_resources they do not end up in a
	// hiddenapi directory in the resulting APK.
	relToHiddenapiDir := func(path android.OutputPath) android.Path {
		return path
	}
	b.hiddenAPIFlagsCSV = relToHiddenapiDir(hiddenAPISingletonPaths(ctx).flags)
	b.hiddenAPIIndexCSV = relToHiddenapiDir(hiddenAPISingletonPaths(ctx).index)
	b.hiddenAPIMetadataCSV = relToHiddenapiDir(hiddenAPISingletonPaths(ctx).metadata)

	moduleSpecificFlagsPaths := android.Paths{}
	for _, module := range modules {
		if h, ok := module.(hiddenAPIIntf); ok {
			if csv := h.flagsCSV(); csv != nil {
				moduleSpecificFlagsPaths = append(moduleSpecificFlagsPaths, csv)
			}
		} else {
			ctx.ModuleErrorf("module %s of type %s does not implement hiddenAPIIntf", module, ctx.OtherModuleType(module))
		}
	}

	augmentationInfo := b.properties.Hidden_api.hiddenAPIAugmentationInfo(ctx)

	outputPath := hiddenAPISingletonPaths(ctx).flags
	baseFlagsPath := hiddenAPISingletonPaths(ctx).stubFlags
	ruleToGenerateHiddenApiFlags(ctx, outputPath, baseFlagsPath, moduleSpecificFlagsPaths, augmentationInfo)
}
