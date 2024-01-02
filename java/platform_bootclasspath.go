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
	ctx.RegisterParallelSingletonModuleType("platform_bootclasspath", platformBootclasspathFactory)
}

// The tags used for the dependencies between the platform bootclasspath and any configured boot
// jars.
var (
	platformBootclasspathArtBootJarDepTag  = bootclasspathDependencyTag{name: "art-boot-jar"}
	platformBootclasspathBootJarDepTag     = bootclasspathDependencyTag{name: "platform-boot-jar"}
	platformBootclasspathApexBootJarDepTag = bootclasspathDependencyTag{name: "apex-boot-jar"}
)

type platformBootclasspathModule struct {
	android.SingletonModuleBase
	ClasspathFragmentBase

	properties platformBootclasspathProperties

	// The apex:module pairs obtained from the configured modules.
	configuredModules []android.Module

	// The apex:module pairs obtained from the fragments.
	fragments []android.Module

	// Path to the monolithic hiddenapi-flags.csv file.
	hiddenAPIFlagsCSV android.OutputPath

	// Path to the monolithic hiddenapi-index.csv file.
	hiddenAPIIndexCSV android.OutputPath

	// Path to the monolithic hiddenapi-unsupported.csv file.
	hiddenAPIMetadataCSV android.OutputPath

	// Path to a srcjar containing all the transitive sources of the bootclasspath.
	srcjar android.OutputPath
}

type platformBootclasspathProperties struct {
	BootclasspathFragmentsDepsProperties

	HiddenAPIFlagFileProperties
}

func platformBootclasspathFactory() android.SingletonModule {
	m := &platformBootclasspathModule{}
	m.AddProperties(&m.properties)
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
	case ".srcjar":
		return android.Paths{b.srcjar}, nil
	}

	return nil, fmt.Errorf("unknown tag %s", tag)
}

func (b *platformBootclasspathModule) DepsMutator(ctx android.BottomUpMutatorContext) {
	// Create a dependency on all_apex_contributions to determine the selected mainline module
	ctx.AddDependency(ctx.Module(), apexContributionsMetadataDepTag, "all_apex_contributions")

	b.hiddenAPIDepsMutator(ctx)

	if !dexpreopt.IsDex2oatNeeded(ctx) {
		return
	}

	// Add a dependency onto the dex2oat tool which is needed for creating the boot image. The
	// path is retrieved from the dependency by GetGlobalSoongConfig(ctx).
	dexpreopt.RegisterToolDeps(ctx)
}

func (b *platformBootclasspathModule) hiddenAPIDepsMutator(ctx android.BottomUpMutatorContext) {
	if ctx.Config().DisableHiddenApiChecks() {
		return
	}

	// Add dependencies onto the stub lib modules.
	apiLevelToStubLibModules := hiddenAPIComputeMonolithicStubLibModules(ctx.Config())
	hiddenAPIAddStubLibDependencies(ctx, apiLevelToStubLibModules)
}

func (b *platformBootclasspathModule) BootclasspathDepsMutator(ctx android.BottomUpMutatorContext) {
	// Add dependencies on all the ART jars.
	global := dexpreopt.GetGlobalConfig(ctx)
	addDependenciesOntoSelectedBootImageApexes(ctx, "com.android.art")
	// TODO: b/308174306 - Remove the mechanism of depending on the java_sdk_library(_import) directly
	addDependenciesOntoBootImageModules(ctx, global.ArtApexJars, platformBootclasspathArtBootJarDepTag)

	// Add dependencies on all the non-updatable jars, which are on the platform or in non-updatable
	// APEXes.
	addDependenciesOntoBootImageModules(ctx, b.platformJars(ctx), platformBootclasspathBootJarDepTag)

	// Add dependencies on all the updatable jars, except the ART jars.
	apexJars := dexpreopt.GetGlobalConfig(ctx).ApexBootJars
	apexes := []string{}
	for i := 0; i < apexJars.Len(); i++ {
		apexes = append(apexes, apexJars.Apex(i))
	}
	addDependenciesOntoSelectedBootImageApexes(ctx, android.FirstUniqueStrings(apexes)...)
	// TODO: b/308174306 - Remove the mechanism of depending on the java_sdk_library(_import) directly
	addDependenciesOntoBootImageModules(ctx, apexJars, platformBootclasspathApexBootJarDepTag)

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
	// Gather all the dependencies from the art, platform, and apex boot jars.
	artModules := gatherApexModulePairDepsWithTag(ctx, platformBootclasspathArtBootJarDepTag)
	platformModules := gatherApexModulePairDepsWithTag(ctx, platformBootclasspathBootJarDepTag)
	apexModules := gatherApexModulePairDepsWithTag(ctx, platformBootclasspathApexBootJarDepTag)

	// Concatenate them all, in order as they would appear on the bootclasspath.
	var allModules []android.Module
	allModules = append(allModules, artModules...)
	allModules = append(allModules, platformModules...)
	allModules = append(allModules, apexModules...)
	b.configuredModules = allModules

	var transitiveSrcFiles android.Paths
	for _, module := range allModules {
		depInfo, _ := android.OtherModuleProvider(ctx, module, JavaInfoProvider)
		if depInfo.TransitiveSrcFiles != nil {
			transitiveSrcFiles = append(transitiveSrcFiles, depInfo.TransitiveSrcFiles.ToList()...)
		}
	}
	jarArgs := resourcePathsToJarArgs(transitiveSrcFiles)
	jarArgs = append(jarArgs, "-srcjar") // Move srcfiles to the right package
	b.srcjar = android.PathForModuleOut(ctx, ctx.ModuleName()+"-transitive.srcjar").OutputPath
	TransformResourcesToJar(ctx, b.srcjar, jarArgs, transitiveSrcFiles)

	// Gather all the fragments dependencies.
	b.fragments = gatherApexModulePairDepsWithTag(ctx, bootclasspathFragmentDepTag)

	// Check the configuration of the boot modules.
	// ART modules are checked by the art-bootclasspath-fragment.
	b.checkPlatformModules(ctx, platformModules)
	b.checkApexModules(ctx, apexModules)

	b.generateClasspathProtoBuildActions(ctx)

	bootDexJarByModule := b.generateHiddenAPIBuildActions(ctx, b.configuredModules, b.fragments)
	buildRuleForBootJarsPackageCheck(ctx, bootDexJarByModule)
}

// Generate classpaths.proto config
func (b *platformBootclasspathModule) generateClasspathProtoBuildActions(ctx android.ModuleContext) {
	configuredJars := b.configuredJars(ctx)
	// ART and platform boot jars must have a corresponding entry in DEX2OATBOOTCLASSPATH
	classpathJars := configuredJarListToClasspathJars(ctx, configuredJars, BOOTCLASSPATH, DEX2OATBOOTCLASSPATH)
	b.classpathFragmentBase().generateClasspathProtoBuildActions(ctx, configuredJars, classpathJars)
}

func (b *platformBootclasspathModule) configuredJars(ctx android.ModuleContext) android.ConfiguredJarList {
	// Include all non APEX jars
	jars := b.platformJars(ctx)

	// Include jars from APEXes that don't populate their classpath proto config.
	remainingJars := dexpreopt.GetGlobalConfig(ctx).ApexBootJars
	for _, fragment := range b.fragments {
		info, _ := android.OtherModuleProvider(ctx, fragment, ClasspathFragmentProtoContentInfoProvider)
		if info.ClasspathFragmentProtoGenerated {
			remainingJars = remainingJars.RemoveList(info.ClasspathFragmentProtoContents)
		}
	}
	for i := 0; i < remainingJars.Len(); i++ {
		jars = jars.Append(remainingJars.Apex(i), remainingJars.Jar(i))
	}

	return jars
}

func (b *platformBootclasspathModule) platformJars(ctx android.PathContext) android.ConfiguredJarList {
	global := dexpreopt.GetGlobalConfig(ctx)
	return global.BootJars.RemoveList(global.ArtApexJars)
}

// checkPlatformModules ensures that the non-updatable modules supplied are not part of an
// apex module.
func (b *platformBootclasspathModule) checkPlatformModules(ctx android.ModuleContext, modules []android.Module) {
	// TODO(satayev): change this check to only allow core-icu4j, all apex jars should not be here.
	for _, m := range modules {
		apexInfo, _ := android.OtherModuleProvider(ctx, m, android.ApexInfoProvider)
		fromUpdatableApex := apexInfo.Updatable
		if fromUpdatableApex {
			// error: this jar is part of an updatable apex
			ctx.ModuleErrorf("module %q from updatable apexes %q is not allowed in the platform bootclasspath", ctx.OtherModuleName(m), apexInfo.InApexVariants)
		} else {
			// ok: this jar is part of the platform or a non-updatable apex
		}
	}
}

// checkApexModules ensures that the apex modules supplied are not from the platform.
func (b *platformBootclasspathModule) checkApexModules(ctx android.ModuleContext, modules []android.Module) {
	for _, m := range modules {
		apexInfo, _ := android.OtherModuleProvider(ctx, m, android.ApexInfoProvider)
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
					ctx.ModuleErrorf("module %q from platform is not allowed in the apex boot jars list", name)
				}
			} else {
				// TODO(b/177892522): Treat this as an error.
				// Cannot do that at the moment because framework-wifi and framework-tethering are in the
				// PRODUCT_APEX_BOOT_JARS but not marked as updatable in AOSP.
			}
		}
	}
}

// generateHiddenAPIBuildActions generates all the hidden API related build rules.
func (b *platformBootclasspathModule) generateHiddenAPIBuildActions(ctx android.ModuleContext, modules []android.Module, fragments []android.Module) bootDexJarByModule {

	// Save the paths to the monolithic files for retrieval via OutputFiles().
	b.hiddenAPIFlagsCSV = hiddenAPISingletonPaths(ctx).flags
	b.hiddenAPIIndexCSV = hiddenAPISingletonPaths(ctx).index
	b.hiddenAPIMetadataCSV = hiddenAPISingletonPaths(ctx).metadata

	bootDexJarByModule := extractBootDexJarsFromModules(ctx, modules)

	// Don't run any hiddenapi rules if hidden api checks are disabled. This is a performance
	// optimization that can be used to reduce the incremental build time but as its name suggests it
	// can be unsafe to use, e.g. when the changes affect anything that goes on the bootclasspath.
	if ctx.Config().DisableHiddenApiChecks() {
		paths := android.OutputPaths{b.hiddenAPIFlagsCSV, b.hiddenAPIIndexCSV, b.hiddenAPIMetadataCSV}
		for _, path := range paths {
			ctx.Build(pctx, android.BuildParams{
				Rule:   android.Touch,
				Output: path,
			})
		}
		return bootDexJarByModule
	}

	// Construct a list of ClasspathElement objects from the modules and fragments.
	classpathElements := CreateClasspathElements(ctx, modules, fragments)

	monolithicInfo := b.createAndProvideMonolithicHiddenAPIInfo(ctx, classpathElements)

	// Extract the classes jars only from those libraries that do not have corresponding fragments as
	// the fragments will have already provided the flags that are needed.
	classesJars := monolithicInfo.ClassesJars

	// Create the input to pass to buildRuleToGenerateHiddenAPIStubFlagsFile
	input := newHiddenAPIFlagInput()

	// Gather stub library information from the dependencies on modules provided by
	// hiddenAPIComputeMonolithicStubLibModules.
	input.gatherStubLibInfo(ctx, nil)

	// Use the flag files from this module and all the fragments.
	input.FlagFilesByCategory = monolithicInfo.FlagsFilesByCategory

	// Generate the monolithic stub-flags.csv file.
	stubFlags := hiddenAPISingletonPaths(ctx).stubFlags
	buildRuleToGenerateHiddenAPIStubFlagsFile(ctx, "platform-bootclasspath-monolithic-hiddenapi-stub-flags", "monolithic hidden API stub flags", stubFlags, bootDexJarByModule.bootDexJars(), input, monolithicInfo.StubFlagSubsets)

	// Generate the annotation-flags.csv file from all the module annotations.
	annotationFlags := android.PathForModuleOut(ctx, "hiddenapi-monolithic", "annotation-flags-from-classes.csv")
	buildRuleToGenerateAnnotationFlags(ctx, "intermediate hidden API flags", classesJars, stubFlags, annotationFlags)

	// Generate the monolithic hiddenapi-flags.csv file.
	//
	// Use annotation flags generated directly from the classes jars as well as annotation flag files
	// provided by prebuilts.
	allAnnotationFlagFiles := android.Paths{annotationFlags}
	allAnnotationFlagFiles = append(allAnnotationFlagFiles, monolithicInfo.AnnotationFlagsPaths...)
	allFlags := hiddenAPISingletonPaths(ctx).flags
	buildRuleToGenerateHiddenApiFlags(ctx, "hiddenAPIFlagsFile", "monolithic hidden API flags", allFlags, stubFlags, allAnnotationFlagFiles, monolithicInfo.FlagsFilesByCategory, monolithicInfo.FlagSubsets, android.OptionalPath{})

	// Generate an intermediate monolithic hiddenapi-metadata.csv file directly from the annotations
	// in the source code.
	intermediateMetadataCSV := android.PathForModuleOut(ctx, "hiddenapi-monolithic", "metadata-from-classes.csv")
	buildRuleToGenerateMetadata(ctx, "intermediate hidden API metadata", classesJars, stubFlags, intermediateMetadataCSV)

	// Generate the monolithic hiddenapi-metadata.csv file.
	//
	// Use metadata files generated directly from the classes jars as well as metadata files provided
	// by prebuilts.
	//
	// This has the side effect of ensuring that the output file uses | quotes just in case that is
	// important for the tools that consume the metadata file.
	allMetadataFlagFiles := android.Paths{intermediateMetadataCSV}
	allMetadataFlagFiles = append(allMetadataFlagFiles, monolithicInfo.MetadataPaths...)
	metadataCSV := hiddenAPISingletonPaths(ctx).metadata
	b.buildRuleMergeCSV(ctx, "monolithic hidden API metadata", allMetadataFlagFiles, metadataCSV)

	// Generate an intermediate monolithic hiddenapi-index.csv file directly from the CSV files in the
	// classes jars.
	intermediateIndexCSV := android.PathForModuleOut(ctx, "hiddenapi-monolithic", "index-from-classes.csv")
	buildRuleToGenerateIndex(ctx, "intermediate hidden API index", classesJars, intermediateIndexCSV)

	// Generate the monolithic hiddenapi-index.csv file.
	//
	// Use index files generated directly from the classes jars as well as index files provided
	// by prebuilts.
	allIndexFlagFiles := android.Paths{intermediateIndexCSV}
	allIndexFlagFiles = append(allIndexFlagFiles, monolithicInfo.IndexPaths...)
	indexCSV := hiddenAPISingletonPaths(ctx).index
	b.buildRuleMergeCSV(ctx, "monolithic hidden API index", allIndexFlagFiles, indexCSV)

	return bootDexJarByModule
}

// createAndProvideMonolithicHiddenAPIInfo creates a MonolithicHiddenAPIInfo and provides it for
// testing.
func (b *platformBootclasspathModule) createAndProvideMonolithicHiddenAPIInfo(ctx android.ModuleContext, classpathElements ClasspathElements) MonolithicHiddenAPIInfo {
	// Create a temporary input structure in which to collate information provided directly by this
	// module, either through properties or direct dependencies.
	temporaryInput := newHiddenAPIFlagInput()

	// Create paths to the flag files specified in the properties.
	temporaryInput.extractFlagFilesFromProperties(ctx, &b.properties.HiddenAPIFlagFileProperties)

	// Create the monolithic info, by starting with the flag files specified on this and then merging
	// in information from all the fragment dependencies of this.
	monolithicInfo := newMonolithicHiddenAPIInfo(ctx, temporaryInput.FlagFilesByCategory, classpathElements)

	// Store the information for testing.
	android.SetProvider(ctx, MonolithicHiddenAPIInfoProvider, monolithicInfo)
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
