// Copyright (C) 2021 The Android Open Source Project
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
	"path/filepath"
	"reflect"
	"strings"

	"android/soong/android"
	"android/soong/dexpreopt"

	"github.com/google/blueprint/proptools"

	"github.com/google/blueprint"
)

func init() {
	registerBootclasspathFragmentBuildComponents(android.InitRegistrationContext)

	android.RegisterSdkMemberType(&bootclasspathFragmentMemberType{
		SdkMemberTypeBase: android.SdkMemberTypeBase{
			PropertyName: "bootclasspath_fragments",
			SupportsSdk:  true,
		},
	})
}

func registerBootclasspathFragmentBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("bootclasspath_fragment", bootclasspathFragmentFactory)
	ctx.RegisterModuleType("prebuilt_bootclasspath_fragment", prebuiltBootclasspathFragmentFactory)
}

type bootclasspathFragmentContentDependencyTag struct {
	blueprint.BaseDependencyTag
}

// Avoid having to make bootclasspath_fragment content visible to the bootclasspath_fragment.
//
// This is a temporary workaround to make it easier to migrate to bootclasspath_fragment modules
// with proper dependencies.
// TODO(b/177892522): Remove this and add needed visibility.
func (b bootclasspathFragmentContentDependencyTag) ExcludeFromVisibilityEnforcement() {
}

// The bootclasspath_fragment contents must never depend on prebuilts.
func (b bootclasspathFragmentContentDependencyTag) ReplaceSourceWithPrebuilt() bool {
	return false
}

// SdkMemberType causes dependencies added with this tag to be automatically added to the sdk as if
// they were specified using java_boot_libs or java_sdk_libs.
func (b bootclasspathFragmentContentDependencyTag) SdkMemberType(child android.Module) android.SdkMemberType {
	// If the module is a java_sdk_library then treat it as if it was specified in the java_sdk_libs
	// property, otherwise treat if it was specified in the java_boot_libs property.
	if javaSdkLibrarySdkMemberType.IsInstance(child) {
		return javaSdkLibrarySdkMemberType
	}

	return javaBootLibsSdkMemberType
}

func (b bootclasspathFragmentContentDependencyTag) ExportMember() bool {
	return true
}

// Contents of bootclasspath fragments in an apex are considered to be directly in the apex, as if
// they were listed in java_libs.
func (b bootclasspathFragmentContentDependencyTag) CopyDirectlyInAnyApex() {}

// The tag used for the dependency between the bootclasspath_fragment module and its contents.
var bootclasspathFragmentContentDepTag = bootclasspathFragmentContentDependencyTag{}

var _ android.ExcludeFromVisibilityEnforcementTag = bootclasspathFragmentContentDepTag
var _ android.ReplaceSourceWithPrebuilt = bootclasspathFragmentContentDepTag
var _ android.SdkMemberTypeDependencyTag = bootclasspathFragmentContentDepTag
var _ android.CopyDirectlyInAnyApexTag = bootclasspathFragmentContentDepTag

func IsBootclasspathFragmentContentDepTag(tag blueprint.DependencyTag) bool {
	return tag == bootclasspathFragmentContentDepTag
}

// Properties that can be different when coverage is enabled.
type BootclasspathFragmentCoverageAffectedProperties struct {
	// The contents of this bootclasspath_fragment, could be either java_library, or java_sdk_library.
	//
	// A java_sdk_library specified here will also be treated as if it was specified on the stub_libs
	// property.
	//
	// The order of this list matters as it is the order that is used in the bootclasspath.
	Contents []string

	// The properties for specifying the API stubs provided by this fragment.
	BootclasspathAPIProperties
}

type bootclasspathFragmentProperties struct {
	// The name of the image this represents.
	//
	// If specified then it must be one of "art" or "boot".
	Image_name *string

	// Properties whose values need to differ with and without coverage.
	BootclasspathFragmentCoverageAffectedProperties
	Coverage BootclasspathFragmentCoverageAffectedProperties

	Hidden_api HiddenAPIFlagFileProperties

	// Properties that allow a fragment to depend on other fragments. This is needed for hidden API
	// processing as it needs access to all the classes used by a fragment including those provided
	// by other fragments.
	BootclasspathFragmentsDepsProperties
}

type BootclasspathFragmentModule struct {
	android.ModuleBase
	android.ApexModuleBase
	android.SdkBase
	ClasspathFragmentBase

	properties bootclasspathFragmentProperties
}

// commonBootclasspathFragment defines the methods that are implemented by both source and prebuilt
// bootclasspath fragment modules.
type commonBootclasspathFragment interface {
	// produceHiddenAPIAllFlagsFile produces the all-flags.csv and intermediate files.
	//
	// Updates the supplied hiddenAPIInfo with the paths to the generated files set.
	produceHiddenAPIAllFlagsFile(ctx android.ModuleContext, contents []android.Module, input HiddenAPIFlagInput) *HiddenAPIFlagOutput
}

var _ commonBootclasspathFragment = (*BootclasspathFragmentModule)(nil)

func bootclasspathFragmentFactory() android.Module {
	m := &BootclasspathFragmentModule{}
	m.AddProperties(&m.properties)
	android.InitApexModule(m)
	android.InitSdkAwareModule(m)
	initClasspathFragment(m, BOOTCLASSPATH)
	android.InitAndroidArchModule(m, android.HostAndDeviceSupported, android.MultilibCommon)

	android.AddLoadHook(m, func(ctx android.LoadHookContext) {
		// If code coverage has been enabled for the framework then append the properties with
		// coverage specific properties.
		if ctx.Config().IsEnvTrue("EMMA_INSTRUMENT_FRAMEWORK") {
			err := proptools.AppendProperties(&m.properties.BootclasspathFragmentCoverageAffectedProperties, &m.properties.Coverage, nil)
			if err != nil {
				ctx.PropertyErrorf("coverage", "error trying to append coverage specific properties: %s", err)
				return
			}
		}

		// Initialize the contents property from the image_name.
		bootclasspathFragmentInitContentsFromImage(ctx, m)
	})
	return m
}

// bootclasspathFragmentInitContentsFromImage will initialize the contents property from the image_name if
// necessary.
func bootclasspathFragmentInitContentsFromImage(ctx android.EarlyModuleContext, m *BootclasspathFragmentModule) {
	contents := m.properties.Contents
	if len(contents) == 0 {
		ctx.PropertyErrorf("contents", "required property is missing")
		return
	}

	if m.properties.Image_name == nil {
		// Nothing to do.
		return
	}

	imageName := proptools.String(m.properties.Image_name)
	if imageName != "art" {
		ctx.PropertyErrorf("image_name", `unknown image name %q, expected "art"`, imageName)
		return
	}

	// TODO(b/177892522): Prebuilts (versioned or not) should not use the image_name property.
	if android.IsModuleInVersionedSdk(m) {
		// The module is a versioned prebuilt so ignore it. This is done for a couple of reasons:
		// 1. There is no way to use this at the moment so ignoring it is safe.
		// 2. Attempting to initialize the contents property from the configuration will end up having
		//    the versioned prebuilt depending on the unversioned prebuilt. That will cause problems
		//    as the unversioned prebuilt could end up with an APEX variant created for the source
		//    APEX which will prevent it from having an APEX variant for the prebuilt APEX which in
		//    turn will prevent it from accessing the dex implementation jar from that which will
		//    break hidden API processing, amongst others.
		return
	}

	// Get the configuration for the art apex jars. Do not use getImageConfig(ctx) here as this is
	// too early in the Soong processing for that to work.
	global := dexpreopt.GetGlobalConfig(ctx)
	modules := global.ArtApexJars

	// Make sure that the apex specified in the configuration is consistent and is one for which
	// this boot image is available.
	commonApex := ""
	for i := 0; i < modules.Len(); i++ {
		apex := modules.Apex(i)
		jar := modules.Jar(i)
		if apex == "platform" {
			ctx.ModuleErrorf("ArtApexJars is invalid as it requests a platform variant of %q", jar)
			continue
		}
		if !m.AvailableFor(apex) {
			ctx.ModuleErrorf("ArtApexJars configuration incompatible with this module, ArtApexJars expects this to be in apex %q but this is only in apexes %q",
				apex, m.ApexAvailable())
			continue
		}
		if commonApex == "" {
			commonApex = apex
		} else if commonApex != apex {
			ctx.ModuleErrorf("ArtApexJars configuration is inconsistent, expected all jars to be in the same apex but it specifies apex %q and %q",
				commonApex, apex)
		}
	}

	if len(contents) != 0 {
		// Nothing to do.
		return
	}

	// Store the jars in the Contents property so that they can be used to add dependencies.
	m.properties.Contents = modules.CopyOfJars()
}

// bootclasspathImageNameContentsConsistencyCheck checks that the configuration that applies to this
// module (if any) matches the contents.
//
// This should be a noop as if image_name="art" then the contents will be set from the ArtApexJars
// config by bootclasspathFragmentInitContentsFromImage so it will be guaranteed to match. However,
// in future this will not be the case.
func (b *BootclasspathFragmentModule) bootclasspathImageNameContentsConsistencyCheck(ctx android.BaseModuleContext) {
	imageName := proptools.String(b.properties.Image_name)
	if imageName == "art" {
		// TODO(b/177892522): Prebuilts (versioned or not) should not use the image_name property.
		if android.IsModuleInVersionedSdk(b) {
			// The module is a versioned prebuilt so ignore it. This is done for a couple of reasons:
			// 1. There is no way to use this at the moment so ignoring it is safe.
			// 2. Attempting to initialize the contents property from the configuration will end up having
			//    the versioned prebuilt depending on the unversioned prebuilt. That will cause problems
			//    as the unversioned prebuilt could end up with an APEX variant created for the source
			//    APEX which will prevent it from having an APEX variant for the prebuilt APEX which in
			//    turn will prevent it from accessing the dex implementation jar from that which will
			//    break hidden API processing, amongst others.
			return
		}

		// Get the configuration for the art apex jars.
		modules := b.getImageConfig(ctx).modules
		configuredJars := modules.CopyOfJars()

		// Skip the check if the configured jars list is empty as that is a common configuration when
		// building targets that do not result in a system image.
		if len(configuredJars) == 0 {
			return
		}

		contents := b.properties.Contents
		if !reflect.DeepEqual(configuredJars, contents) {
			ctx.ModuleErrorf("inconsistency in specification of contents. ArtApexJars configuration specifies %#v, contents property specifies %#v",
				configuredJars, contents)
		}
	}
}

var BootclasspathFragmentApexContentInfoProvider = blueprint.NewProvider(BootclasspathFragmentApexContentInfo{})

// BootclasspathFragmentApexContentInfo contains the bootclasspath_fragments contributions to the
// apex contents.
type BootclasspathFragmentApexContentInfo struct {
	// The configured modules, will be empty if this is from a bootclasspath_fragment that does not
	// set image_name: "art".
	modules android.ConfiguredJarList

	// Map from arch type to the boot image files.
	bootImageFilesByArch map[android.ArchType]android.OutputPaths

	// Map from the name of the context module (as returned by Name()) to the hidden API encoded dex
	// jar path.
	contentModuleDexJarPaths map[string]android.Path
}

func (i BootclasspathFragmentApexContentInfo) Modules() android.ConfiguredJarList {
	return i.modules
}

// Get a map from ArchType to the associated boot image's contents for Android.
//
// Extension boot images only return their own files, not the files of the boot images they extend.
func (i BootclasspathFragmentApexContentInfo) AndroidBootImageFilesByArchType() map[android.ArchType]android.OutputPaths {
	return i.bootImageFilesByArch
}

// DexBootJarPathForContentModule returns the path to the dex boot jar for specified module.
//
// The dex boot jar is one which has had hidden API encoding performed on it.
func (i BootclasspathFragmentApexContentInfo) DexBootJarPathForContentModule(module android.Module) (android.Path, error) {
	name := module.Name()
	if dexJar, ok := i.contentModuleDexJarPaths[name]; ok {
		return dexJar, nil
	} else {
		return nil, fmt.Errorf("unknown bootclasspath_fragment content module %s, expected one of %s",
			name, strings.Join(android.SortedStringKeys(i.contentModuleDexJarPaths), ", "))
	}
}

func (b *BootclasspathFragmentModule) DepIsInSameApex(ctx android.BaseModuleContext, dep android.Module) bool {
	tag := ctx.OtherModuleDependencyTag(dep)
	if IsBootclasspathFragmentContentDepTag(tag) {
		// Boot image contents are automatically added to apex.
		return true
	}
	if android.IsMetaDependencyTag(tag) {
		// Cross-cutting metadata dependencies are metadata.
		return false
	}
	panic(fmt.Errorf("boot_image module %q should not have a dependency on %q via tag %s", b, dep, android.PrettyPrintTag(tag)))
}

func (b *BootclasspathFragmentModule) ShouldSupportSdkVersion(ctx android.BaseModuleContext, sdkVersion android.ApiLevel) error {
	return nil
}

// ComponentDepsMutator adds dependencies onto modules before any prebuilt modules without a
// corresponding source module are renamed. This means that adding a dependency using a name without
// a prebuilt_ prefix will always resolve to a source module and when using a name with that prefix
// it will always resolve to a prebuilt module.
func (b *BootclasspathFragmentModule) ComponentDepsMutator(ctx android.BottomUpMutatorContext) {
	module := ctx.Module()
	_, isSourceModule := module.(*BootclasspathFragmentModule)

	for _, name := range b.properties.Contents {
		// A bootclasspath_fragment must depend only on other source modules, while the
		// prebuilt_bootclasspath_fragment must only depend on other prebuilt modules.
		//
		// TODO(b/177892522) - avoid special handling of jacocoagent.
		if !isSourceModule && name != "jacocoagent" {
			name = android.PrebuiltNameFromSource(name)
		}
		ctx.AddDependency(module, bootclasspathFragmentContentDepTag, name)
	}

}

func (b *BootclasspathFragmentModule) DepsMutator(ctx android.BottomUpMutatorContext) {
	// Add dependencies onto all the modules that provide the API stubs for classes on this
	// bootclasspath fragment.
	hiddenAPIAddStubLibDependencies(ctx, b.properties.sdkKindToStubLibs())

	if SkipDexpreoptBootJars(ctx) {
		return
	}

	// Add a dependency onto the dex2oat tool which is needed for creating the boot image. The
	// path is retrieved from the dependency by GetGlobalSoongConfig(ctx).
	dexpreopt.RegisterToolDeps(ctx)
}

func (b *BootclasspathFragmentModule) BootclasspathDepsMutator(ctx android.BottomUpMutatorContext) {
	// Add dependencies on all the fragments.
	b.properties.BootclasspathFragmentsDepsProperties.addDependenciesOntoFragments(ctx)
}

func (b *BootclasspathFragmentModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Only perform a consistency check if this module is the active module. That will prevent an
	// unused prebuilt that was created without instrumentation from breaking an instrumentation
	// build.
	if isActiveModule(ctx.Module()) {
		b.bootclasspathImageNameContentsConsistencyCheck(ctx)
	}

	// Generate classpaths.proto config
	b.generateClasspathProtoBuildActions(ctx)

	// Gather the bootclasspath fragment's contents.
	var contents []android.Module
	ctx.VisitDirectDeps(func(module android.Module) {
		tag := ctx.OtherModuleDependencyTag(module)
		if IsBootclasspathFragmentContentDepTag(tag) {
			contents = append(contents, module)
		}
	})

	fragments := gatherApexModulePairDepsWithTag(ctx, bootclasspathFragmentDepTag)

	// Perform hidden API processing.
	hiddenAPIFlagOutput := b.generateHiddenAPIBuildActions(ctx, contents, fragments)

	// Verify that the image_name specified on a bootclasspath_fragment is valid even if this is a
	// prebuilt which will not use the image config.
	imageConfig := b.getImageConfig(ctx)

	// A prebuilt fragment cannot contribute to the apex.
	if !android.IsModulePrebuilt(ctx.Module()) {
		// Provide the apex content info.
		b.provideApexContentInfo(ctx, imageConfig, contents, hiddenAPIFlagOutput)
	}
}

// provideApexContentInfo creates, initializes and stores the apex content info for use by other
// modules.
func (b *BootclasspathFragmentModule) provideApexContentInfo(ctx android.ModuleContext, imageConfig *bootImageConfig, contents []android.Module, hiddenAPIFlagOutput *HiddenAPIFlagOutput) {
	// Construct the apex content info from the config.
	info := BootclasspathFragmentApexContentInfo{}

	// Populate the apex content info with paths to the dex jars.
	b.populateApexContentInfoDexJars(ctx, &info, contents, hiddenAPIFlagOutput)

	if imageConfig != nil {
		info.modules = imageConfig.modules

		if !SkipDexpreoptBootJars(ctx) {
			// Force the GlobalSoongConfig to be created and cached for use by the dex_bootjars
			// GenerateSingletonBuildActions method as it cannot create it for itself.
			dexpreopt.GetGlobalSoongConfig(ctx)

			// Only generate the boot image if the configuration does not skip it.
			if b.generateBootImageBuildActions(ctx, contents, imageConfig) {
				// Allow the apex to access the boot image files.
				files := map[android.ArchType]android.OutputPaths{}
				for _, variant := range imageConfig.variants {
					// We also generate boot images for host (for testing), but we don't need those in the apex.
					// TODO(b/177892522) - consider changing this to check Os.OsClass = android.Device
					if variant.target.Os == android.Android {
						files[variant.target.Arch.ArchType] = variant.imagesDeps
					}
				}
				info.bootImageFilesByArch = files
			}
		}
	}

	// Make the apex content info available for other modules.
	ctx.SetProvider(BootclasspathFragmentApexContentInfoProvider, info)
}

// populateApexContentInfoDexJars adds paths to the dex jars provided by this fragment to the
// apex content info.
func (b *BootclasspathFragmentModule) populateApexContentInfoDexJars(ctx android.ModuleContext, info *BootclasspathFragmentApexContentInfo, contents []android.Module, hiddenAPIFlagOutput *HiddenAPIFlagOutput) {

	info.contentModuleDexJarPaths = map[string]android.Path{}
	if hiddenAPIFlagOutput != nil {
		// Hidden API encoding has been performed.
		flags := hiddenAPIFlagOutput.AllFlagsPath
		for _, m := range contents {
			h := m.(hiddenAPIModule)
			unencodedDex := h.bootDexJar()
			if unencodedDex == nil {
				// This is an error. Sometimes Soong will report the error directly, other times it will
				// defer the error reporting to happen only when trying to use the missing file in ninja.
				// Either way it is handled by extractBootDexJarsFromModules which must have been
				// called before this as it generates the flags that are used to encode these files.
				continue
			}

			outputDir := android.PathForModuleOut(ctx, "hiddenapi-modular/encoded").OutputPath
			encodedDex := hiddenAPIEncodeDex(ctx, unencodedDex, flags, *h.uncompressDex(), outputDir)
			info.contentModuleDexJarPaths[m.Name()] = encodedDex
		}
	} else {
		for _, m := range contents {
			j := m.(UsesLibraryDependency)
			dexJar := j.DexJarBuildPath()
			info.contentModuleDexJarPaths[m.Name()] = dexJar
		}
	}
}

// generateClasspathProtoBuildActions generates all required build actions for classpath.proto config
func (b *BootclasspathFragmentModule) generateClasspathProtoBuildActions(ctx android.ModuleContext) {
	var classpathJars []classpathJar
	if "art" == proptools.String(b.properties.Image_name) {
		// ART and platform boot jars must have a corresponding entry in DEX2OATBOOTCLASSPATH
		classpathJars = configuredJarListToClasspathJars(ctx, b.ClasspathFragmentToConfiguredJarList(ctx), BOOTCLASSPATH, DEX2OATBOOTCLASSPATH)
	} else {
		classpathJars = configuredJarListToClasspathJars(ctx, b.ClasspathFragmentToConfiguredJarList(ctx), b.classpathType)
	}
	b.classpathFragmentBase().generateClasspathProtoBuildActions(ctx, classpathJars)
}

func (b *BootclasspathFragmentModule) ClasspathFragmentToConfiguredJarList(ctx android.ModuleContext) android.ConfiguredJarList {
	if "art" == proptools.String(b.properties.Image_name) {
		return b.getImageConfig(ctx).modules
	}

	global := dexpreopt.GetGlobalConfig(ctx)

	// Convert content names to their appropriate stems, in case a test library is overriding an actual boot jar
	var stems []string
	for _, name := range b.properties.Contents {
		dep := ctx.GetDirectDepWithTag(name, bootclasspathFragmentContentDepTag)
		if m, ok := dep.(ModuleWithStem); ok {
			stems = append(stems, m.Stem())
		} else {
			ctx.PropertyErrorf("contents", "%v is not a ModuleWithStem", name)
		}
	}

	// Only create configs for updatable boot jars. Non-updatable boot jars must be part of the
	// platform_bootclasspath's classpath proto config to guarantee that they come before any
	// updatable jars at runtime.
	jars := global.UpdatableBootJars.Filter(stems)

	// TODO(satayev): for apex_test we want to include all contents unconditionally to classpaths
	// config. However, any test specific jars would not be present in UpdatableBootJars. Instead,
	// we should check if we are creating a config for apex_test via ApexInfo and amend the values.
	// This is an exception to support end-to-end test for SdkExtensions, until such support exists.
	if android.InList("test_framework-sdkextensions", stems) {
		jars = jars.Append("com.android.sdkext", "test_framework-sdkextensions")
	}
	return jars
}

func (b *BootclasspathFragmentModule) getImageConfig(ctx android.EarlyModuleContext) *bootImageConfig {
	// Get a map of the image configs that are supported.
	imageConfigs := genBootImageConfigs(ctx)

	// Retrieve the config for this image.
	imageNamePtr := b.properties.Image_name
	if imageNamePtr == nil {
		return nil
	}

	imageName := *imageNamePtr
	imageConfig := imageConfigs[imageName]
	if imageConfig == nil {
		ctx.PropertyErrorf("image_name", "Unknown image name %q, expected one of %s", imageName, strings.Join(android.SortedStringKeys(imageConfigs), ", "))
		return nil
	}
	return imageConfig
}

// generateHiddenAPIBuildActions generates all the hidden API related build rules.
func (b *BootclasspathFragmentModule) generateHiddenAPIBuildActions(ctx android.ModuleContext, contents []android.Module, fragments []android.Module) *HiddenAPIFlagOutput {

	// Create hidden API input structure.
	input := b.createHiddenAPIFlagInput(ctx, contents, fragments)

	var output *HiddenAPIFlagOutput

	// Hidden API processing is conditional as a temporary workaround as not all
	// bootclasspath_fragments provide the appropriate information needed for hidden API processing
	// which leads to breakages of the build.
	// TODO(b/179354495): Stop hidden API processing being conditional once all bootclasspath_fragment
	//  modules have been updated to support it.
	if input.canPerformHiddenAPIProcessing(ctx, b.properties) {
		// Delegate the production of the hidden API all-flags.csv file to a module type specific method.
		common := ctx.Module().(commonBootclasspathFragment)
		output = common.produceHiddenAPIAllFlagsFile(ctx, contents, input)
	}

	// Initialize a HiddenAPIInfo structure.
	hiddenAPIInfo := HiddenAPIInfo{
		// The monolithic hidden API processing needs access to the flag files that override the default
		// flags from all the fragments whether or not they actually perform their own hidden API flag
		// generation. That is because the monolithic hidden API processing uses those flag files to
		// perform its own flag generation.
		FlagFilesByCategory: input.FlagFilesByCategory,

		// Other bootclasspath_fragments that depend on this need the transitive set of stub dex jars
		// from this to resolve any references from their code to classes provided by this fragment
		// and the fragments this depends upon.
		TransitiveStubDexJarsByKind: input.transitiveStubDexJarsByKind(),
	}

	if output != nil {
		// The monolithic hidden API processing also needs access to all the output files produced by
		// hidden API processing of this fragment.
		hiddenAPIInfo.HiddenAPIFlagOutput = *output
	}

	//  Provide it for use by other modules.
	ctx.SetProvider(HiddenAPIInfoProvider, hiddenAPIInfo)

	return output
}

// createHiddenAPIFlagInput creates a HiddenAPIFlagInput struct and initializes it with information derived
// from the properties on this module and its dependencies.
func (b *BootclasspathFragmentModule) createHiddenAPIFlagInput(ctx android.ModuleContext, contents []android.Module, fragments []android.Module) HiddenAPIFlagInput {

	// Merge the HiddenAPIInfo from all the fragment dependencies.
	dependencyHiddenApiInfo := newHiddenAPIInfo()
	dependencyHiddenApiInfo.mergeFromFragmentDeps(ctx, fragments)

	// Create hidden API flag input structure.
	input := newHiddenAPIFlagInput()

	// Update the input structure with information obtained from the stub libraries.
	input.gatherStubLibInfo(ctx, contents)

	// Populate with flag file paths from the properties.
	input.extractFlagFilesFromProperties(ctx, &b.properties.Hidden_api)

	// Store the stub dex jars from this module's fragment dependencies.
	input.DependencyStubDexJarsByKind = dependencyHiddenApiInfo.TransitiveStubDexJarsByKind

	return input
}

// produceHiddenAPIAllFlagsFile produces the hidden API all-flags.csv file (and supporting files)
// for the fragment.
func (b *BootclasspathFragmentModule) produceHiddenAPIAllFlagsFile(ctx android.ModuleContext, contents []android.Module, input HiddenAPIFlagInput) *HiddenAPIFlagOutput {
	// Generate the rules to create the hidden API flags and update the supplied hiddenAPIInfo with the
	// paths to the created files.
	return hiddenAPIGenerateAllFlagsForBootclasspathFragment(ctx, contents, input)
}

// generateBootImageBuildActions generates ninja rules to create the boot image if required for this
// module.
//
// Returns true if the boot image is created, false otherwise.
func (b *BootclasspathFragmentModule) generateBootImageBuildActions(ctx android.ModuleContext, contents []android.Module, imageConfig *bootImageConfig) bool {
	global := dexpreopt.GetGlobalConfig(ctx)
	if !shouldBuildBootImages(ctx.Config(), global) {
		return false
	}

	// Bootclasspath fragment modules that are for the platform do not produce a boot image.
	apexInfo := ctx.Provider(android.ApexInfoProvider).(android.ApexInfo)
	if apexInfo.IsForPlatform() {
		return false
	}

	// Bootclasspath fragment modules that are versioned do not produce a boot image.
	if android.IsModuleInVersionedSdk(ctx.Module()) {
		return false
	}

	// Copy the dex jars of this fragment's content modules to their predefined locations.
	bootDexJarByModule := extractEncodedDexJarsFromModules(ctx, contents)
	copyBootJarsToPredefinedLocations(ctx, bootDexJarByModule, imageConfig.dexPathsByModule)

	// Build a profile for the image config and then use that to build the boot image.
	profile := bootImageProfileRule(ctx, imageConfig)
	buildBootImage(ctx, imageConfig, profile)

	return true
}

type bootclasspathFragmentMemberType struct {
	android.SdkMemberTypeBase
}

func (b *bootclasspathFragmentMemberType) AddDependencies(mctx android.BottomUpMutatorContext, dependencyTag blueprint.DependencyTag, names []string) {
	mctx.AddVariationDependencies(nil, dependencyTag, names...)
}

func (b *bootclasspathFragmentMemberType) IsInstance(module android.Module) bool {
	_, ok := module.(*BootclasspathFragmentModule)
	return ok
}

func (b *bootclasspathFragmentMemberType) AddPrebuiltModule(ctx android.SdkMemberContext, member android.SdkMember) android.BpModule {
	if b.PropertyName == "boot_images" {
		return ctx.SnapshotBuilder().AddPrebuiltModule(member, "prebuilt_boot_image")
	} else {
		return ctx.SnapshotBuilder().AddPrebuiltModule(member, "prebuilt_bootclasspath_fragment")
	}
}

func (b *bootclasspathFragmentMemberType) CreateVariantPropertiesStruct() android.SdkMemberProperties {
	return &bootclasspathFragmentSdkMemberProperties{}
}

type bootclasspathFragmentSdkMemberProperties struct {
	android.SdkMemberPropertiesBase

	// The image name
	Image_name *string

	// Contents of the bootclasspath fragment
	Contents []string

	// Stub_libs properties.
	Stub_libs               []string
	Core_platform_stub_libs []string

	// Flag files by *hiddenAPIFlagFileCategory
	Flag_files_by_category FlagFilesByCategory

	// The path to the generated stub-flags.csv file.
	Stub_flags_path android.OptionalPath

	// The path to the generated annotation-flags.csv file.
	Annotation_flags_path android.OptionalPath

	// The path to the generated metadata.csv file.
	Metadata_path android.OptionalPath

	// The path to the generated index.csv file.
	Index_path android.OptionalPath

	// The path to the generated all-flags.csv file.
	All_flags_path android.OptionalPath
}

func (b *bootclasspathFragmentSdkMemberProperties) PopulateFromVariant(ctx android.SdkMemberContext, variant android.Module) {
	module := variant.(*BootclasspathFragmentModule)

	b.Image_name = module.properties.Image_name
	b.Contents = module.properties.Contents

	// Get the hidden API information from the module.
	mctx := ctx.SdkModuleContext()
	hiddenAPIInfo := mctx.OtherModuleProvider(module, HiddenAPIInfoProvider).(HiddenAPIInfo)
	b.Flag_files_by_category = hiddenAPIInfo.FlagFilesByCategory

	// Copy all the generated file paths.
	b.Stub_flags_path = android.OptionalPathForPath(hiddenAPIInfo.StubFlagsPath)
	b.Annotation_flags_path = android.OptionalPathForPath(hiddenAPIInfo.AnnotationFlagsPath)
	b.Metadata_path = android.OptionalPathForPath(hiddenAPIInfo.MetadataPath)
	b.Index_path = android.OptionalPathForPath(hiddenAPIInfo.IndexPath)
	b.All_flags_path = android.OptionalPathForPath(hiddenAPIInfo.AllFlagsPath)

	// Copy stub_libs properties.
	b.Stub_libs = module.properties.Api.Stub_libs
	b.Core_platform_stub_libs = module.properties.Core_platform_api.Stub_libs
}

func (b *bootclasspathFragmentSdkMemberProperties) AddToPropertySet(ctx android.SdkMemberContext, propertySet android.BpPropertySet) {
	if b.Image_name != nil {
		propertySet.AddProperty("image_name", *b.Image_name)
	}

	builder := ctx.SnapshotBuilder()
	requiredMemberDependency := builder.SdkMemberReferencePropertyTag(true)

	if len(b.Contents) > 0 {
		propertySet.AddPropertyWithTag("contents", b.Contents, requiredMemberDependency)
	}

	if len(b.Stub_libs) > 0 {
		apiPropertySet := propertySet.AddPropertySet("api")
		apiPropertySet.AddPropertyWithTag("stub_libs", b.Stub_libs, requiredMemberDependency)
	}
	if len(b.Core_platform_stub_libs) > 0 {
		corePlatformApiPropertySet := propertySet.AddPropertySet("core_platform_api")
		corePlatformApiPropertySet.AddPropertyWithTag("stub_libs", b.Core_platform_stub_libs, requiredMemberDependency)
	}

	hiddenAPISet := propertySet.AddPropertySet("hidden_api")
	hiddenAPIDir := "hiddenapi"

	// Copy manually curated flag files specified on the bootclasspath_fragment.
	if b.Flag_files_by_category != nil {
		for _, category := range HiddenAPIFlagFileCategories {
			paths := b.Flag_files_by_category[category]
			if len(paths) > 0 {
				dests := []string{}
				for _, p := range paths {
					dest := filepath.Join(hiddenAPIDir, p.Base())
					builder.CopyToSnapshot(p, dest)
					dests = append(dests, dest)
				}
				hiddenAPISet.AddProperty(category.PropertyName, dests)
			}
		}
	}

	copyOptionalPath := func(path android.OptionalPath, property string) {
		if path.Valid() {
			p := path.Path()
			dest := filepath.Join(hiddenAPIDir, p.Base())
			builder.CopyToSnapshot(p, dest)
			hiddenAPISet.AddProperty(property, dest)
		}
	}

	// Copy all the generated files, if available.
	copyOptionalPath(b.Stub_flags_path, "stub_flags")
	copyOptionalPath(b.Annotation_flags_path, "annotation_flags")
	copyOptionalPath(b.Metadata_path, "metadata")
	copyOptionalPath(b.Index_path, "index")
	copyOptionalPath(b.All_flags_path, "all_flags")
}

var _ android.SdkMemberType = (*bootclasspathFragmentMemberType)(nil)

// prebuiltBootclasspathFragmentProperties contains additional prebuilt_bootclasspath_fragment
// specific properties.
type prebuiltBootclasspathFragmentProperties struct {
	Hidden_api struct {
		// The path to the stub-flags.csv file created by the bootclasspath_fragment.
		Stub_flags *string `android:"path"`

		// The path to the annotation-flags.csv file created by the bootclasspath_fragment.
		Annotation_flags *string `android:"path"`

		// The path to the metadata.csv file created by the bootclasspath_fragment.
		Metadata *string `android:"path"`

		// The path to the index.csv file created by the bootclasspath_fragment.
		Index *string `android:"path"`

		// The path to the all-flags.csv file created by the bootclasspath_fragment.
		All_flags *string `android:"path"`
	}
}

// A prebuilt version of the bootclasspath_fragment module.
//
// At the moment this is basically just a bootclasspath_fragment module that can be used as a
// prebuilt. Eventually as more functionality is migrated into the bootclasspath_fragment module
// type from the various singletons then this will diverge.
type prebuiltBootclasspathFragmentModule struct {
	BootclasspathFragmentModule
	prebuilt android.Prebuilt

	// Additional prebuilt specific properties.
	prebuiltProperties prebuiltBootclasspathFragmentProperties
}

func (module *prebuiltBootclasspathFragmentModule) Prebuilt() *android.Prebuilt {
	return &module.prebuilt
}

func (module *prebuiltBootclasspathFragmentModule) Name() string {
	return module.prebuilt.Name(module.ModuleBase.Name())
}

// produceHiddenAPIAllFlagsFile returns a path to the prebuilt all-flags.csv or nil if none is
// specified.
func (module *prebuiltBootclasspathFragmentModule) produceHiddenAPIAllFlagsFile(ctx android.ModuleContext, contents []android.Module, _ HiddenAPIFlagInput) *HiddenAPIFlagOutput {
	pathForOptionalSrc := func(src *string) android.Path {
		if src == nil {
			// TODO(b/179354495): Fail if this is not provided once prebuilts have been updated.
			return nil
		}
		return android.PathForModuleSrc(ctx, *src)
	}

	output := HiddenAPIFlagOutput{
		StubFlagsPath:       pathForOptionalSrc(module.prebuiltProperties.Hidden_api.Stub_flags),
		AnnotationFlagsPath: pathForOptionalSrc(module.prebuiltProperties.Hidden_api.Annotation_flags),
		MetadataPath:        pathForOptionalSrc(module.prebuiltProperties.Hidden_api.Metadata),
		IndexPath:           pathForOptionalSrc(module.prebuiltProperties.Hidden_api.Index),
		AllFlagsPath:        pathForOptionalSrc(module.prebuiltProperties.Hidden_api.All_flags),
	}

	return &output
}

var _ commonBootclasspathFragment = (*prebuiltBootclasspathFragmentModule)(nil)

func prebuiltBootclasspathFragmentFactory() android.Module {
	m := &prebuiltBootclasspathFragmentModule{}
	m.AddProperties(&m.properties, &m.prebuiltProperties)
	// This doesn't actually have any prebuilt files of its own so pass a placeholder for the srcs
	// array.
	android.InitPrebuiltModule(m, &[]string{"placeholder"})
	android.InitApexModule(m)
	android.InitSdkAwareModule(m)
	android.InitAndroidArchModule(m, android.HostAndDeviceSupported, android.MultilibCommon)

	// Initialize the contents property from the image_name.
	android.AddLoadHook(m, func(ctx android.LoadHookContext) {
		bootclasspathFragmentInitContentsFromImage(ctx, &m.BootclasspathFragmentModule)
	})
	return m
}
