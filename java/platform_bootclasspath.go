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
	hiddenAPIFlagsCSV android.OutputPath

	// Path to the monolithic hiddenapi-index.csv file.
	hiddenAPIIndexCSV android.OutputPath

	// Path to the monolithic hiddenapi-unsupported.csv file.
	hiddenAPIMetadataCSV android.OutputPath
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
		// Add dependency on the unprefixed (i.e. source or renamed prebuilt) module which we know does
		// not exist. The resulting error message will contain useful information about the available
		// variants.
		reportMissingVariationDependency(ctx, variations, name)

		// Add dependency on the missing prefixed prebuilt variant too if a module with that name exists
		// so that information about its available variants will be reported too.
		if ctx.OtherModuleExists(prebuiltName) {
			reportMissingVariationDependency(ctx, variations, prebuiltName)
		}
	}
}

// reportMissingVariationDependency intentionally adds a dependency on a missing variation in order
// to generate an appropriate error message with information about the available variations.
func reportMissingVariationDependency(ctx android.BottomUpMutatorContext, variations []blueprint.Variation, name string) {
	modules := ctx.AddFarVariationDependencies(variations, nil, name)
	if len(modules) != 1 {
		panic(fmt.Errorf("Internal Error: expected one module, found %d", len(modules)))
		return
	}
	if modules[0] != nil {
		panic(fmt.Errorf("Internal Error: expected module to be missing but was found: %q", modules[0]))
		return
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

	// Save the paths to the monolithic files for retrieval via OutputFiles().
	b.hiddenAPIFlagsCSV = hiddenAPISingletonPaths(ctx).flags
	b.hiddenAPIIndexCSV = hiddenAPISingletonPaths(ctx).index
	b.hiddenAPIMetadataCSV = hiddenAPISingletonPaths(ctx).metadata

	// Don't run any hiddenapi rules if UNSAFE_DISABLE_HIDDENAPI_FLAGS=true. This is a performance
	// optimization that can be used to reduce the incremental build time but as its name suggests it
	// can be unsafe to use, e.g. when the changes affect anything that goes on the bootclasspath.
	if ctx.Config().IsEnvTrue("UNSAFE_DISABLE_HIDDENAPI_FLAGS") {
		paths := android.OutputPaths{b.hiddenAPIFlagsCSV, b.hiddenAPIIndexCSV, b.hiddenAPIMetadataCSV}
		for _, path := range paths {
			ctx.Build(pctx, android.BuildParams{
				Rule:   android.Touch,
				Output: path,
			})
		}
		return
	}

	hiddenAPISupportingModules := []hiddenAPISupportingModule{}
	for _, module := range modules {
		if h, ok := module.(hiddenAPISupportingModule); ok {
			if h.bootDexJar() == nil {
				ctx.ModuleErrorf("module %s does not provide a bootDexJar file", module)
			}
			if h.flagsCSV() == nil {
				ctx.ModuleErrorf("module %s does not provide a flagsCSV file", module)
			}
			if h.indexCSV() == nil {
				ctx.ModuleErrorf("module %s does not provide an indexCSV file", module)
			}
			if h.metadataCSV() == nil {
				ctx.ModuleErrorf("module %s does not provide a metadataCSV file", module)
			}

			if ctx.Failed() {
				continue
			}

			hiddenAPISupportingModules = append(hiddenAPISupportingModules, h)
		} else {
			ctx.ModuleErrorf("module %s of type %s does not support hidden API processing", module, ctx.OtherModuleType(module))
		}
	}

	moduleSpecificFlagsPaths := android.Paths{}
	for _, module := range hiddenAPISupportingModules {
		moduleSpecificFlagsPaths = append(moduleSpecificFlagsPaths, module.flagsCSV())
	}

	augmentationInfo := b.properties.Hidden_api.hiddenAPIAugmentationInfo(ctx)

	outputPath := hiddenAPISingletonPaths(ctx).flags
	baseFlagsPath := hiddenAPISingletonPaths(ctx).stubFlags
	ruleToGenerateHiddenApiFlags(ctx, outputPath, baseFlagsPath, moduleSpecificFlagsPaths, augmentationInfo)

	b.generateHiddenAPIIndexRules(ctx, hiddenAPISupportingModules)
	b.generatedHiddenAPIMetadataRules(ctx, hiddenAPISupportingModules)
}

func (b *platformBootclasspathModule) generateHiddenAPIIndexRules(ctx android.ModuleContext, modules []hiddenAPISupportingModule) {
	indexes := android.Paths{}
	for _, module := range modules {
		indexes = append(indexes, module.indexCSV())
	}

	rule := android.NewRuleBuilder(pctx, ctx)
	rule.Command().
		BuiltTool("merge_csv").
		Flag("--key_field signature").
		FlagWithArg("--header=", "signature,file,startline,startcol,endline,endcol,properties").
		FlagWithOutput("--output=", hiddenAPISingletonPaths(ctx).index).
		Inputs(indexes)
	rule.Build("platform-bootclasspath-monolithic-hiddenapi-index", "monolithic hidden API index")
}

func (b *platformBootclasspathModule) generatedHiddenAPIMetadataRules(ctx android.ModuleContext, modules []hiddenAPISupportingModule) {
	metadataCSVFiles := android.Paths{}
	for _, module := range modules {
		metadataCSVFiles = append(metadataCSVFiles, module.metadataCSV())
	}

	rule := android.NewRuleBuilder(pctx, ctx)

	outputPath := hiddenAPISingletonPaths(ctx).metadata

	rule.Command().
		BuiltTool("merge_csv").
		Flag("--key_field signature").
		FlagWithOutput("--output=", outputPath).
		Inputs(metadataCSVFiles)

	rule.Build("platform-bootclasspath-monolithic-hiddenapi-metadata", "monolithic hidden API metadata")
}
