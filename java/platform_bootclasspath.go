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
)

func init() {
	registerPlatformBootclasspathBuildComponents(android.InitRegistrationContext)
}

func registerPlatformBootclasspathBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterSingletonModuleType("platform_bootclasspath", platformBootclasspathFactory)
}

// The tags used for the dependencies between the platform bootclasspath and any configured boot
// jars.
var (
	platformBootclasspathArtBootJarDepTag          = bootclasspathDependencyTag{name: "art-boot-jar"}
	platformBootclasspathNonUpdatableBootJarDepTag = bootclasspathDependencyTag{name: "non-updatable-boot-jar"}
	platformBootclasspathUpdatableBootJarDepTag    = bootclasspathDependencyTag{name: "updatable-boot-jar"}
)

type platformBootclasspathModule struct {
	android.SingletonModuleBase
	ClasspathFragmentBase

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

type platformBootclasspathProperties struct {
	BootclasspathFragmentsDepsProperties

	Hidden_api HiddenAPIFlagFileProperties
}

func platformBootclasspathFactory() android.SingletonModule {
	m := &platformBootclasspathModule{}
	m.AddProperties(&m.properties)
	// TODO(satayev): split apex jars into separate configs.
	initClasspathFragment(m, BOOTCLASSPATH)
	android.InitAndroidArchModule(m, android.DeviceSupported, android.MultilibCommon)
	return m
}

var _ android.OutputFileProducer = (*platformBootclasspathModule)(nil)

func (b *platformBootclasspathModule) AndroidMkEntries() (entries []android.AndroidMkEntries) {
	entries = append(entries, android.AndroidMkEntries{
		Class: "FAKE",
		// Need at least one output file in order for this to take effect.
		OutputFile: android.OptionalPathForPath(b.hiddenAPIFlagsCSV),
		Include:    "$(BUILD_PHONY_PACKAGE)",
	})
	entries = append(entries, b.classpathFragmentBase().androidMkEntries()...)
	return
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
	b.hiddenAPIDepsMutator(ctx)

	if SkipDexpreoptBootJars(ctx) {
		return
	}

	// Add a dependency onto the dex2oat tool which is needed for creating the boot image. The
	// path is retrieved from the dependency by GetGlobalSoongConfig(ctx).
	dexpreopt.RegisterToolDeps(ctx)
}

func (b *platformBootclasspathModule) hiddenAPIDepsMutator(ctx android.BottomUpMutatorContext) {
	if ctx.Config().IsEnvTrue("UNSAFE_DISABLE_HIDDENAPI_FLAGS") {
		return
	}

	// Add dependencies onto the stub lib modules.
	sdkKindToStubLibModules := hiddenAPIComputeMonolithicStubLibModules(ctx.Config())
	hiddenAPIAddStubLibDependencies(ctx, sdkKindToStubLibModules)
}

func (b *platformBootclasspathModule) BootclasspathDepsMutator(ctx android.BottomUpMutatorContext) {
	// Add dependencies on all the modules configured in the "art" boot image.
	artImageConfig := genBootImageConfigs(ctx)[artBootImageName]
	addDependenciesOntoBootImageModules(ctx, artImageConfig.modules, platformBootclasspathArtBootJarDepTag)

	// Add dependencies on all the non-updatable module configured in the "boot" boot image. That does
	// not include modules configured in the "art" boot image.
	bootImageConfig := b.getImageConfig(ctx)
	addDependenciesOntoBootImageModules(ctx, bootImageConfig.modules, platformBootclasspathNonUpdatableBootJarDepTag)

	// Add dependencies on all the updatable modules.
	updatableModules := dexpreopt.GetGlobalConfig(ctx).UpdatableBootJars
	addDependenciesOntoBootImageModules(ctx, updatableModules, platformBootclasspathUpdatableBootJarDepTag)

	// Add dependencies on all the fragments.
	b.properties.BootclasspathFragmentsDepsProperties.addDependenciesOntoFragments(ctx)
}

func addDependenciesOntoBootImageModules(ctx android.BottomUpMutatorContext, modules android.ConfiguredJarList, tag bootclasspathDependencyTag) {
	for i := 0; i < modules.Len(); i++ {
		apex := modules.Apex(i)
		name := modules.Jar(i)

		addDependencyOntoApexModulePair(ctx, apex, name, tag)
	}
}

// GenerateSingletonBuildActions does nothing and must never do anything.
//
// This module only implements android.SingletonModule so that it can implement
// android.SingletonMakeVarsProvider.
func (b *platformBootclasspathModule) GenerateSingletonBuildActions(android.SingletonContext) {
	// Keep empty
}

func (d *platformBootclasspathModule) MakeVars(ctx android.MakeVarsContext) {
	d.generateHiddenApiMakeVars(ctx)
}

func (b *platformBootclasspathModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Gather all the dependencies from the art, updatable and non-updatable boot jars.
	artModules := gatherApexModulePairDepsWithTag(ctx, platformBootclasspathArtBootJarDepTag)
	nonUpdatableModules := gatherApexModulePairDepsWithTag(ctx, platformBootclasspathNonUpdatableBootJarDepTag)
	updatableModules := gatherApexModulePairDepsWithTag(ctx, platformBootclasspathUpdatableBootJarDepTag)

	// Concatenate them all, in order as they would appear on the bootclasspath.
	var allModules []android.Module
	allModules = append(allModules, artModules...)
	allModules = append(allModules, nonUpdatableModules...)
	allModules = append(allModules, updatableModules...)
	b.configuredModules = allModules

	// Gather all the fragments dependencies.
	b.fragments = gatherApexModulePairDepsWithTag(ctx, bootclasspathFragmentDepTag)

	// Check the configuration of the boot modules.
	// ART modules are checked by the art-bootclasspath-fragment.
	b.checkNonUpdatableModules(ctx, nonUpdatableModules)
	b.checkUpdatableModules(ctx, updatableModules)

	b.generateClasspathProtoBuildActions(ctx)

	b.generateHiddenAPIBuildActions(ctx, b.configuredModules, b.fragments)

	// Nothing to do if skipping the dexpreopt of boot image jars.
	if SkipDexpreoptBootJars(ctx) {
		return
	}

	b.generateBootImageBuildActions(ctx, nonUpdatableModules, updatableModules)
}

// Generate classpaths.proto config
func (b *platformBootclasspathModule) generateClasspathProtoBuildActions(ctx android.ModuleContext) {
	// ART and platform boot jars must have a corresponding entry in DEX2OATBOOTCLASSPATH
	classpathJars := configuredJarListToClasspathJars(ctx, b.ClasspathFragmentToConfiguredJarList(ctx), BOOTCLASSPATH, DEX2OATBOOTCLASSPATH)
	b.classpathFragmentBase().generateClasspathProtoBuildActions(ctx, classpathJars)
}

func (b *platformBootclasspathModule) ClasspathFragmentToConfiguredJarList(ctx android.ModuleContext) android.ConfiguredJarList {
	return b.getImageConfig(ctx).modules
}

// checkNonUpdatableModules ensures that the non-updatable modules supplied are not part of an
// updatable module.
func (b *platformBootclasspathModule) checkNonUpdatableModules(ctx android.ModuleContext, modules []android.Module) {
	for _, m := range modules {
		apexInfo := ctx.OtherModuleProvider(m, android.ApexInfoProvider).(android.ApexInfo)
		fromUpdatableApex := apexInfo.Updatable
		if fromUpdatableApex {
			// error: this jar is part of an updatable apex
			ctx.ModuleErrorf("module %q from updatable apexes %q is not allowed in the framework boot image", ctx.OtherModuleName(m), apexInfo.InApexVariants)
		} else {
			// ok: this jar is part of the platform or a non-updatable apex
		}
	}
}

// checkUpdatableModules ensures that the updatable modules supplied are not from the platform.
func (b *platformBootclasspathModule) checkUpdatableModules(ctx android.ModuleContext, modules []android.Module) {
	for _, m := range modules {
		apexInfo := ctx.OtherModuleProvider(m, android.ApexInfoProvider).(android.ApexInfo)
		fromUpdatableApex := apexInfo.Updatable
		if fromUpdatableApex {
			// ok: this jar is part of an updatable apex
		} else {
			name := ctx.OtherModuleName(m)
			if apexInfo.IsForPlatform() {
				// If AlwaysUsePrebuiltSdks() returns true then it is possible that the updatable list will
				// include platform variants of a prebuilt module due to workarounds elsewhere. In that case
				// do not treat this as an error.
				// TODO(b/179354495): Always treat this as an error when migration to bootclasspath_fragment
				//  modules is complete.
				if !ctx.Config().AlwaysUsePrebuiltSdks() {
					// error: this jar is part of the platform
					ctx.ModuleErrorf("module %q from platform is not allowed in the updatable boot jars list", name)
				}
			} else {
				// TODO(b/177892522): Treat this as an error.
				// Cannot do that at the moment because framework-wifi and framework-tethering are in the
				// PRODUCT_UPDATABLE_BOOT_JARS but not marked as updatable in AOSP.
			}
		}
	}
}

func (b *platformBootclasspathModule) getImageConfig(ctx android.EarlyModuleContext) *bootImageConfig {
	return defaultBootImageConfig(ctx)
}

// generateHiddenAPIBuildActions generates all the hidden API related build rules.
func (b *platformBootclasspathModule) generateHiddenAPIBuildActions(ctx android.ModuleContext, modules []android.Module, fragments []android.Module) {

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

	monolithicInfo := b.createAndProvideMonolithicHiddenAPIInfo(ctx, fragments)
	// Create the input to pass to ruleToGenerateHiddenAPIStubFlagsFile
	input := newHiddenAPIFlagInput()

	// Gather stub library information from the dependencies on modules provided by
	// hiddenAPIComputeMonolithicStubLibModules.
	input.gatherStubLibInfo(ctx, nil)

	// Use the flag files from this module and all the fragments.
	input.FlagFilesByCategory = monolithicInfo.FlagsFilesByCategory

	// Generate the monolithic stub-flags.csv file.
	bootDexJars := extractBootDexJarsFromModules(ctx, modules)
	stubFlags := hiddenAPISingletonPaths(ctx).stubFlags
	rule := ruleToGenerateHiddenAPIStubFlagsFile(ctx, stubFlags, bootDexJars, input)
	rule.Build("platform-bootclasspath-monolithic-hiddenapi-stub-flags", "monolithic hidden API stub flags")

	// Extract the classes jars from the contents.
	classesJars := extractClassesJarsFromModules(modules)

	// Generate the annotation-flags.csv file from all the module annotations.
	annotationFlags := android.PathForModuleOut(ctx, "hiddenapi-monolithic", "annotation-flags.csv")
	buildRuleToGenerateAnnotationFlags(ctx, "monolithic hiddenapi flags", classesJars, stubFlags, annotationFlags)

	// Generate the monotlithic hiddenapi-flags.csv file.
	allFlags := hiddenAPISingletonPaths(ctx).flags
	buildRuleToGenerateHiddenApiFlags(ctx, "hiddenAPIFlagsFile", "hiddenapi flags", allFlags, stubFlags, annotationFlags, monolithicInfo.FlagsFilesByCategory, monolithicInfo.AllFlagsPaths, android.OptionalPath{})

	// Generate an intermediate monolithic hiddenapi-metadata.csv file directly from the annotations
	// in the source code.
	intermediateMetadataCSV := android.PathForModuleOut(ctx, "hiddenapi-monolithic", "intermediate-metadata.csv")
	buildRuleToGenerateMetadata(ctx, "monolithic hidden API metadata", classesJars, stubFlags, intermediateMetadataCSV)

	// Reformat the intermediate file to add | quotes just in case that is important for the tools
	// that consume the metadata file.
	// TODO(b/179354495): Investigate whether it is possible to remove this reformatting step.
	metadataCSV := hiddenAPISingletonPaths(ctx).metadata
	b.buildRuleMergeCSV(ctx, "reformat monolithic hidden API metadata", android.Paths{intermediateMetadataCSV}, metadataCSV)

	// Generate the monolithic hiddenapi-index.csv file directly from the CSV files in the classes
	// jars.
	indexCSV := hiddenAPISingletonPaths(ctx).index
	buildRuleToGenerateIndex(ctx, "monolithic hidden API index", classesJars, indexCSV)
}

// createAndProvideMonolithicHiddenAPIInfo creates a MonolithicHiddenAPIInfo and provides it for
// testing.
func (b *platformBootclasspathModule) createAndProvideMonolithicHiddenAPIInfo(ctx android.ModuleContext, fragments []android.Module) MonolithicHiddenAPIInfo {
	// Create a temporary input structure in which to collate information provided directly by this
	// module, either through properties or direct dependencies.
	temporaryInput := newHiddenAPIFlagInput()

	// Create paths to the flag files specified in the properties.
	temporaryInput.extractFlagFilesFromProperties(ctx, &b.properties.Hidden_api)

	// Create the monolithic info, by starting with the flag files specified on this and then merging
	// in information from all the fragment dependencies of this.
	monolithicInfo := newMonolithicHiddenAPIInfo(ctx, temporaryInput.FlagFilesByCategory, fragments)

	// Store the information for testing.
	ctx.SetProvider(MonolithicHiddenAPIInfoProvider, monolithicInfo)
	return monolithicInfo
}

func (b *platformBootclasspathModule) buildRuleMergeCSV(ctx android.ModuleContext, desc string, inputPaths android.Paths, outputPath android.WritablePath) {
	rule := android.NewRuleBuilder(pctx, ctx)
	rule.Command().
		BuiltTool("merge_csv").
		Flag("--key_field signature").
		FlagWithOutput("--output=", outputPath).
		Inputs(inputPaths)

	rule.Build(desc, desc)
}

// generateHiddenApiMakeVars generates make variables needed by hidden API related make rules, e.g.
// veridex and run-appcompat.
func (b *platformBootclasspathModule) generateHiddenApiMakeVars(ctx android.MakeVarsContext) {
	if ctx.Config().IsEnvTrue("UNSAFE_DISABLE_HIDDENAPI_FLAGS") {
		return
	}
	// INTERNAL_PLATFORM_HIDDENAPI_FLAGS is used by Make rules in art/ and cts/.
	ctx.Strict("INTERNAL_PLATFORM_HIDDENAPI_FLAGS", b.hiddenAPIFlagsCSV.String())
}

// generateBootImageBuildActions generates ninja rules related to the boot image creation.
func (b *platformBootclasspathModule) generateBootImageBuildActions(ctx android.ModuleContext, nonUpdatableModules, updatableModules []android.Module) {
	// Force the GlobalSoongConfig to be created and cached for use by the dex_bootjars
	// GenerateSingletonBuildActions method as it cannot create it for itself.
	dexpreopt.GetGlobalSoongConfig(ctx)

	imageConfig := b.getImageConfig(ctx)
	if imageConfig == nil {
		return
	}

	global := dexpreopt.GetGlobalConfig(ctx)
	if !shouldBuildBootImages(ctx.Config(), global) {
		return
	}

	// Generate the framework profile rule
	bootFrameworkProfileRule(ctx, imageConfig)

	// Generate the updatable bootclasspath packages rule.
	generateUpdatableBcpPackagesRule(ctx, imageConfig, updatableModules)

	// Copy non-updatable module dex jars to their predefined locations.
	nonUpdatableBootDexJarsByModule := extractEncodedDexJarsFromModules(ctx, nonUpdatableModules)
	copyBootJarsToPredefinedLocations(ctx, nonUpdatableBootDexJarsByModule, imageConfig.dexPathsByModule)

	// Copy updatable module dex jars to their predefined locations.
	config := GetUpdatableBootConfig(ctx)
	updatableBootDexJarsByModule := extractEncodedDexJarsFromModules(ctx, updatableModules)
	copyBootJarsToPredefinedLocations(ctx, updatableBootDexJarsByModule, config.dexPathsByModule)

	// Build a profile for the image config and then use that to build the boot image.
	profile := bootImageProfileRule(ctx, imageConfig)
	buildBootImage(ctx, imageConfig, profile)

	dumpOatRules(ctx, imageConfig)
}
