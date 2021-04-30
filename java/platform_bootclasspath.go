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
	// TODO(satayev): split systemserver and apex jars into separate configs.
	initClasspathFragment(m)
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
	entries = append(entries, b.classpathFragmentBase().getAndroidMkEntries()...)
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
	b.classpathFragmentBase().generateAndroidBuildActions(ctx)

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

	b.generateHiddenAPIBuildActions(ctx, b.configuredModules, b.fragments)

	// Nothing to do if skipping the dexpreopt of boot image jars.
	if SkipDexpreoptBootJars(ctx) {
		return
	}

	b.generateBootImageBuildActions(ctx, updatableModules)
}

func (b *platformBootclasspathModule) getImageConfig(ctx android.EarlyModuleContext) *bootImageConfig {
	return defaultBootImageConfig(ctx)
}

// hiddenAPISupportingModule encapsulates the information provided by any module that contributes to
// the hidden API processing.
type hiddenAPISupportingModule struct {
	module android.Module

	bootDexJar  android.Path
	flagsCSV    android.Path
	indexCSV    android.Path
	metadataCSV android.Path
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

	// nilPathHandler will check the supplied path and if it is nil then it will either immediately
	// report an error, or it will defer the error reporting until it is actually used, depending
	// whether missing dependencies are allowed.
	var nilPathHandler func(path android.Path, name string, module android.Module) android.Path
	if ctx.Config().AllowMissingDependencies() {
		nilPathHandler = func(path android.Path, name string, module android.Module) android.Path {
			if path == nil {
				outputPath := android.PathForModuleOut(ctx, "missing", module.Name(), name)
				path = outputPath

				// Create an error rule that pretends to create the output file but will actually fail if it
				// is run.
				ctx.Build(pctx, android.BuildParams{
					Rule:   android.ErrorRule,
					Output: outputPath,
					Args: map[string]string{
						"error": fmt.Sprintf("missing hidden API file: %s for %s", name, module),
					},
				})
			}
			return path
		}
	} else {
		nilPathHandler = func(path android.Path, name string, module android.Module) android.Path {
			if path == nil {
				ctx.ModuleErrorf("module %s does not provide a %s file", module, name)
			}
			return path
		}
	}

	hiddenAPISupportingModules := []hiddenAPISupportingModule{}
	for _, module := range modules {
		if h, ok := module.(hiddenAPIIntf); ok {
			hiddenAPISupportingModule := hiddenAPISupportingModule{
				module:      module,
				bootDexJar:  nilPathHandler(h.bootDexJar(), "bootDexJar", module),
				flagsCSV:    nilPathHandler(h.flagsCSV(), "flagsCSV", module),
				indexCSV:    nilPathHandler(h.indexCSV(), "indexCSV", module),
				metadataCSV: nilPathHandler(h.metadataCSV(), "metadataCSV", module),
			}

			// If any errors were reported when trying to populate the hiddenAPISupportingModule struct
			// then don't add it to the list.
			if ctx.Failed() {
				continue
			}

			hiddenAPISupportingModules = append(hiddenAPISupportingModules, hiddenAPISupportingModule)
		} else {
			ctx.ModuleErrorf("module %s of type %s does not support hidden API processing", module, ctx.OtherModuleType(module))
		}
	}

	moduleSpecificFlagsPaths := android.Paths{}
	for _, module := range hiddenAPISupportingModules {
		moduleSpecificFlagsPaths = append(moduleSpecificFlagsPaths, module.flagsCSV)
	}

	flagFileInfo := b.properties.Hidden_api.hiddenAPIFlagFileInfo(ctx)
	for _, fragment := range fragments {
		if ctx.OtherModuleHasProvider(fragment, hiddenAPIFlagFileInfoProvider) {
			info := ctx.OtherModuleProvider(fragment, hiddenAPIFlagFileInfoProvider).(hiddenAPIFlagFileInfo)
			flagFileInfo.append(info)
		}
	}

	// Store the information for testing.
	ctx.SetProvider(hiddenAPIFlagFileInfoProvider, flagFileInfo)

	outputPath := hiddenAPISingletonPaths(ctx).flags
	baseFlagsPath := hiddenAPISingletonPaths(ctx).stubFlags
	ruleToGenerateHiddenApiFlags(ctx, outputPath, baseFlagsPath, moduleSpecificFlagsPaths, flagFileInfo)

	b.generateHiddenAPIStubFlagsRules(ctx, hiddenAPISupportingModules)
	b.generateHiddenAPIIndexRules(ctx, hiddenAPISupportingModules)
	b.generatedHiddenAPIMetadataRules(ctx, hiddenAPISupportingModules)
}

func (b *platformBootclasspathModule) generateHiddenAPIStubFlagsRules(ctx android.ModuleContext, modules []hiddenAPISupportingModule) {
	bootDexJars := android.Paths{}
	for _, module := range modules {
		bootDexJars = append(bootDexJars, module.bootDexJar)
	}

	sdkKindToStubPaths := hiddenAPIGatherStubLibDexJarPaths(ctx)

	outputPath := hiddenAPISingletonPaths(ctx).stubFlags
	rule := ruleToGenerateHiddenAPIStubFlagsFile(ctx, outputPath, bootDexJars, sdkKindToStubPaths)
	rule.Build("platform-bootclasspath-monolithic-hiddenapi-stub-flags", "monolithic hidden API stub flags")
}

func (b *platformBootclasspathModule) generateHiddenAPIIndexRules(ctx android.ModuleContext, modules []hiddenAPISupportingModule) {
	indexes := android.Paths{}
	for _, module := range modules {
		indexes = append(indexes, module.indexCSV)
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
		metadataCSVFiles = append(metadataCSVFiles, module.metadataCSV)
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
func (b *platformBootclasspathModule) generateBootImageBuildActions(ctx android.ModuleContext, updatableModules []android.Module) {
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

	dumpOatRules(ctx, imageConfig)
}
