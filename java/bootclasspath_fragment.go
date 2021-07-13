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

// Contents of bootclasspath fragments require files from prebuilt apex files.
func (b bootclasspathFragmentContentDependencyTag) RequiresFilesFromPrebuiltApex() {}

// The tag used for the dependency between the bootclasspath_fragment module and its contents.
var bootclasspathFragmentContentDepTag = bootclasspathFragmentContentDependencyTag{}

var _ android.ExcludeFromVisibilityEnforcementTag = bootclasspathFragmentContentDepTag
var _ android.ReplaceSourceWithPrebuilt = bootclasspathFragmentContentDepTag
var _ android.SdkMemberTypeDependencyTag = bootclasspathFragmentContentDepTag
var _ android.CopyDirectlyInAnyApexTag = bootclasspathFragmentContentDepTag
var _ android.RequiresFilesFromPrebuiltApexTag = bootclasspathFragmentContentDepTag

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

	// Hidden API related properties.
	Hidden_api HiddenAPIFlagFileProperties

	// The list of additional stub libraries which this fragment's contents use but which are not
	// provided by another bootclasspath_fragment.
	//
	// Note, "android-non-updatable" is treated specially. While no such module exists it is treated
	// as if it was a java_sdk_library. So, when public API stubs are needed then it will be replaced
	// with "android-non-updatable.stubs", with "androidn-non-updatable.system.stubs" when the system
	// stubs are needed and so on.
	Additional_stubs []string

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
	// produceHiddenAPIOutput produces the all-flags.csv and intermediate files and encodes the flags
	// into dex files.
	//
	// Returns a *HiddenAPIOutput containing the paths for the generated files. Returns nil if the
	// module cannot contribute to hidden API processing, e.g. because it is a prebuilt module in a
	// versioned sdk.
	produceHiddenAPIOutput(ctx android.ModuleContext, contents []android.Module, input HiddenAPIFlagInput) *HiddenAPIOutput

	// produceBootImageFiles will attempt to produce rules to create the boot image files at the paths
	// predefined in the bootImageConfig.
	//
	// If it could not create the files then it will return nil. Otherwise, it will return a map from
	// android.ArchType to the predefined paths of the boot image files.
	produceBootImageFiles(ctx android.ModuleContext, imageConfig *bootImageConfig) bootImageFilesByArch
}

var _ commonBootclasspathFragment = (*BootclasspathFragmentModule)(nil)

// bootImageFilesByArch is a map from android.ArchType to the paths to the boot image files.
//
// The paths include the .art, .oat and .vdex files, one for each of the modules from which the boot
// image is created.
type bootImageFilesByArch map[android.ArchType]android.Paths

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
	bootImageFilesByArch bootImageFilesByArch

	// Map from the base module name (without prebuilt_ prefix) of a fragment's contents module to the
	// hidden API encoded dex jar path.
	contentModuleDexJarPaths bootDexJarByModule
}

func (i BootclasspathFragmentApexContentInfo) Modules() android.ConfiguredJarList {
	return i.modules
}

// Get a map from ArchType to the associated boot image's contents for Android.
//
// Extension boot images only return their own files, not the files of the boot images they extend.
func (i BootclasspathFragmentApexContentInfo) AndroidBootImageFilesByArchType() bootImageFilesByArch {
	return i.bootImageFilesByArch
}

// DexBootJarPathForContentModule returns the path to the dex boot jar for specified module.
//
// The dex boot jar is one which has had hidden API encoding performed on it.
func (i BootclasspathFragmentApexContentInfo) DexBootJarPathForContentModule(module android.Module) (android.Path, error) {
	// A bootclasspath_fragment cannot use a prebuilt library so Name() will return the base name
	// without a prebuilt_ prefix so is safe to use as the key for the contentModuleDexJarPaths.
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
	hiddenAPIAddStubLibDependencies(ctx, b.properties.apiScopeToStubLibs())

	for _, additionalStubModule := range b.properties.Additional_stubs {
		for _, apiScope := range hiddenAPISdkLibrarySupportedScopes {
			// Add a dependency onto a possibly scope specific stub library.
			scopeSpecificDependency := apiScope.scopeSpecificStubModule(ctx, additionalStubModule)
			tag := hiddenAPIStubsDependencyTag{apiScope: apiScope, fromAdditionalDependency: true}
			ctx.AddVariationDependencies(nil, tag, scopeSpecificDependency)
		}
	}

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

	// Verify that the image_name specified on a bootclasspath_fragment is valid even if this is a
	// prebuilt which will not use the image config.
	imageConfig := b.getImageConfig(ctx)

	// A versioned prebuilt_bootclasspath_fragment cannot and does not need to perform hidden API
	// processing. It cannot do it because it is not part of a prebuilt_apex and so has no access to
	// the correct dex implementation jar. It does not need to because the platform-bootclasspath
	// always references the latest bootclasspath_fragments.
	if !android.IsModuleInVersionedSdk(ctx.Module()) {
		// Perform hidden API processing.
		hiddenAPIOutput := b.generateHiddenAPIBuildActions(ctx, contents, fragments)

		var bootImageFilesByArch bootImageFilesByArch
		if imageConfig != nil {
			// Delegate the production of the boot image files to a module type specific method.
			common := ctx.Module().(commonBootclasspathFragment)
			bootImageFilesByArch = common.produceBootImageFiles(ctx, imageConfig)

			if shouldCopyBootFilesToPredefinedLocations(ctx, imageConfig) {
				// Zip the boot image files up, if available. This will generate the zip file in a
				// predefined location.
				buildBootImageZipInPredefinedLocation(ctx, imageConfig, bootImageFilesByArch)

				// Copy the dex jars of this fragment's content modules to their predefined locations.
				copyBootJarsToPredefinedLocations(ctx, hiddenAPIOutput.EncodedBootDexFilesByModule, imageConfig.dexPathsByModule)
			}
		}

		// A prebuilt fragment cannot contribute to an apex.
		if !android.IsModulePrebuilt(ctx.Module()) {
			// Provide the apex content info.
			b.provideApexContentInfo(ctx, imageConfig, hiddenAPIOutput, bootImageFilesByArch)
		}
	}
}

// shouldCopyBootFilesToPredefinedLocations determines whether the current module should copy boot
// files, e.g. boot dex jars or boot image files, to the predefined location expected by the rest
// of the build.
//
// This ensures that only a single module will copy its files to the image configuration.
func shouldCopyBootFilesToPredefinedLocations(ctx android.ModuleContext, imageConfig *bootImageConfig) bool {
	// Bootclasspath fragment modules that are for the platform do not produce boot related files.
	apexInfo := ctx.Provider(android.ApexInfoProvider).(android.ApexInfo)
	if apexInfo.IsForPlatform() {
		return false
	}

	// If the image configuration has no modules specified then it means that the build has been
	// configured to build something other than a boot image, e.g. an sdk, so do not try and copy the
	// files.
	if imageConfig.modules.Len() == 0 {
		return false
	}

	// Only copy files from the module that is preferred.
	return isActiveModule(ctx.Module())
}

// provideApexContentInfo creates, initializes and stores the apex content info for use by other
// modules.
func (b *BootclasspathFragmentModule) provideApexContentInfo(ctx android.ModuleContext, imageConfig *bootImageConfig, hiddenAPIOutput *HiddenAPIOutput, bootImageFilesByArch bootImageFilesByArch) {
	// Construct the apex content info from the config.
	info := BootclasspathFragmentApexContentInfo{
		// Populate the apex content info with paths to the dex jars.
		contentModuleDexJarPaths: hiddenAPIOutput.EncodedBootDexFilesByModule,
	}

	if imageConfig != nil {
		info.modules = imageConfig.modules
	}

	info.bootImageFilesByArch = bootImageFilesByArch

	// Make the apex content info available for other modules.
	ctx.SetProvider(BootclasspathFragmentApexContentInfoProvider, info)
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

	possibleUpdatableModules := gatherPossibleUpdatableModuleNamesAndStems(ctx, b.properties.Contents, bootclasspathFragmentContentDepTag)

	// Only create configs for updatable boot jars. Non-updatable boot jars must be part of the
	// platform_bootclasspath's classpath proto config to guarantee that they come before any
	// updatable jars at runtime.
	jars := global.UpdatableBootJars.Filter(possibleUpdatableModules)

	// TODO(satayev): for apex_test we want to include all contents unconditionally to classpaths
	// config. However, any test specific jars would not be present in UpdatableBootJars. Instead,
	// we should check if we are creating a config for apex_test via ApexInfo and amend the values.
	// This is an exception to support end-to-end test for SdkExtensions, until such support exists.
	if android.InList("test_framework-sdkextensions", possibleUpdatableModules) {
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
func (b *BootclasspathFragmentModule) generateHiddenAPIBuildActions(ctx android.ModuleContext, contents []android.Module, fragments []android.Module) *HiddenAPIOutput {

	// Create hidden API input structure.
	input := b.createHiddenAPIFlagInput(ctx, contents, fragments)

	// Delegate the production of the hidden API all-flags.csv file to a module type specific method.
	common := ctx.Module().(commonBootclasspathFragment)
	output := common.produceHiddenAPIOutput(ctx, contents, input)

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
		TransitiveStubDexJarsByScope: input.transitiveStubDexJarsByScope(),
	}

	// The monolithic hidden API processing also needs access to all the output files produced by
	// hidden API processing of this fragment.
	hiddenAPIInfo.HiddenAPIFlagOutput = (*output).HiddenAPIFlagOutput

	//  Provide it for use by other modules.
	ctx.SetProvider(HiddenAPIInfoProvider, hiddenAPIInfo)

	return output
}

// retrieveLegacyEncodedBootDexFiles attempts to retrieve the legacy encoded boot dex jar files.
func retrieveLegacyEncodedBootDexFiles(ctx android.ModuleContext, contents []android.Module) bootDexJarByModule {
	// If the current bootclasspath_fragment is the active module or a source module then retrieve the
	// encoded dex files, otherwise return an empty map.
	//
	// An inactive (i.e. not preferred) bootclasspath_fragment needs to retrieve the encoded dex jars
	// as they are still needed by an apex. An inactive prebuilt_bootclasspath_fragment does not need
	// to do so and may not yet have access to dex boot jars from a prebuilt_apex/apex_set.
	if isActiveModule(ctx.Module()) || !android.IsModulePrebuilt(ctx.Module()) {
		return extractEncodedDexJarsFromModules(ctx, contents)
	} else {
		return nil
	}
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

	// Add the stub dex jars from this module's fragment dependencies.
	input.DependencyStubDexJarsByScope.addStubDexJarsByModule(dependencyHiddenApiInfo.TransitiveStubDexJarsByScope)

	return input
}

// produceHiddenAPIOutput produces the hidden API all-flags.csv file (and supporting files)
// for the fragment as well as encoding the flags in the boot dex jars.
func (b *BootclasspathFragmentModule) produceHiddenAPIOutput(ctx android.ModuleContext, contents []android.Module, input HiddenAPIFlagInput) *HiddenAPIOutput {
	// Generate the rules to create the hidden API flags and update the supplied hiddenAPIInfo with the
	// paths to the created files.
	return hiddenAPIRulesForBootclasspathFragment(ctx, contents, input)
}

// produceBootImageFiles builds the boot image files from the source if it is required.
func (b *BootclasspathFragmentModule) produceBootImageFiles(ctx android.ModuleContext, imageConfig *bootImageConfig) bootImageFilesByArch {
	if SkipDexpreoptBootJars(ctx) {
		return nil
	}

	// Only generate the boot image if the configuration does not skip it.
	return b.generateBootImageBuildActions(ctx, imageConfig)
}

// generateBootImageBuildActions generates ninja rules to create the boot image if required for this
// module.
//
// If it could not create the files then it will return nil. Otherwise, it will return a map from
// android.ArchType to the predefined paths of the boot image files.
func (b *BootclasspathFragmentModule) generateBootImageBuildActions(ctx android.ModuleContext, imageConfig *bootImageConfig) bootImageFilesByArch {
	global := dexpreopt.GetGlobalConfig(ctx)
	if !shouldBuildBootImages(ctx.Config(), global) {
		return nil
	}

	// Bootclasspath fragment modules that are for the platform do not produce a boot image.
	apexInfo := ctx.Provider(android.ApexInfoProvider).(android.ApexInfo)
	if apexInfo.IsForPlatform() {
		return nil
	}

	// Bootclasspath fragment modules that are versioned do not produce a boot image.
	if android.IsModuleInVersionedSdk(ctx.Module()) {
		return nil
	}

	// Build a profile for the image config and then use that to build the boot image.
	profile := bootImageProfileRule(ctx, imageConfig)

	// Build boot image files for the host variants.
	buildBootImageVariantsForBuildOs(ctx, imageConfig, profile)

	// Build boot image files for the android variants.
	androidBootImageFilesByArch := buildBootImageVariantsForAndroidOs(ctx, imageConfig, profile)

	// Return the boot image files for the android variants for inclusion in an APEX and to be zipped
	// up for the dist.
	return androidBootImageFilesByArch
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

// produceHiddenAPIOutput returns a path to the prebuilt all-flags.csv or nil if none is specified.
func (module *prebuiltBootclasspathFragmentModule) produceHiddenAPIOutput(ctx android.ModuleContext, contents []android.Module, input HiddenAPIFlagInput) *HiddenAPIOutput {
	pathForOptionalSrc := func(src *string) android.Path {
		if src == nil {
			// TODO(b/179354495): Fail if this is not provided once prebuilts have been updated.
			return nil
		}
		return android.PathForModuleSrc(ctx, *src)
	}

	// Retrieve the dex files directly from the content modules. They in turn should retrieve the
	// encoded dex jars from the prebuilt .apex files.
	encodedBootDexJarsByModule := extractEncodedDexJarsFromModules(ctx, contents)

	output := HiddenAPIOutput{
		HiddenAPIFlagOutput: HiddenAPIFlagOutput{
			StubFlagsPath:       pathForOptionalSrc(module.prebuiltProperties.Hidden_api.Stub_flags),
			AnnotationFlagsPath: pathForOptionalSrc(module.prebuiltProperties.Hidden_api.Annotation_flags),
			MetadataPath:        pathForOptionalSrc(module.prebuiltProperties.Hidden_api.Metadata),
			IndexPath:           pathForOptionalSrc(module.prebuiltProperties.Hidden_api.Index),
			AllFlagsPath:        pathForOptionalSrc(module.prebuiltProperties.Hidden_api.All_flags),
		},
		EncodedBootDexFilesByModule: encodedBootDexJarsByModule,
	}

	return &output
}

// produceBootImageFiles extracts the boot image files from the APEX if available.
func (module *prebuiltBootclasspathFragmentModule) produceBootImageFiles(ctx android.ModuleContext, imageConfig *bootImageConfig) bootImageFilesByArch {
	if !shouldCopyBootFilesToPredefinedLocations(ctx, imageConfig) {
		return nil
	}

	var deapexerModule android.Module
	ctx.VisitDirectDeps(func(to android.Module) {
		tag := ctx.OtherModuleDependencyTag(to)
		// Save away the `deapexer` module on which this depends, if any.
		if tag == android.DeapexerTag {
			if deapexerModule != nil {
				ctx.ModuleErrorf("Ambiguous duplicate deapexer module dependencies %q and %q",
					deapexerModule.Name(), to.Name())
			}
			deapexerModule = to
		}
	})

	if deapexerModule == nil {
		// This should never happen as a variant for a prebuilt_apex is only created if the
		// deapexer module has been configured to export the dex implementation jar for this module.
		ctx.ModuleErrorf("internal error: module does not depend on a `deapexer` module")
		return nil
	}

	di := ctx.OtherModuleProvider(deapexerModule, android.DeapexerProvider).(android.DeapexerInfo)
	files := bootImageFilesByArch{}
	for _, variant := range imageConfig.apexVariants() {
		arch := variant.target.Arch.ArchType
		for _, toPath := range variant.imagesDeps {
			apexRelativePath := apexRootRelativePathToBootImageFile(arch, toPath.Base())
			// Get the path to the file that the deapexer extracted from the prebuilt apex file.
			fromPath := di.PrebuiltExportPath(apexRelativePath)

			// Return the toPath as the calling code expects the paths in the returned map to be the
			// paths predefined in the bootImageConfig.
			files[arch] = append(files[arch], toPath)

			// Copy the file to the predefined location.
			ctx.Build(pctx, android.BuildParams{
				Rule:   android.Cp,
				Input:  fromPath,
				Output: toPath,
			})
		}
	}

	// Build the boot image files for the host variants. These are built from the dex files provided
	// by the contents of this module as prebuilt versions of the host boot image files are not
	// available, i.e. there is no host specific prebuilt apex containing them. This has to be built
	// without a profile as the prebuilt modules do not provide a profile.
	buildBootImageVariantsForBuildOs(ctx, imageConfig, nil)

	return files
}

var _ commonBootclasspathFragment = (*prebuiltBootclasspathFragmentModule)(nil)

// createBootImageTag creates the tag to uniquely identify the boot image file among all of the
// files that a module requires from the prebuilt .apex file.
func createBootImageTag(arch android.ArchType, baseName string) string {
	tag := fmt.Sprintf(".bootimage-%s-%s", arch, baseName)
	return tag
}

// RequiredFilesFromPrebuiltApex returns the list of all files the prebuilt_bootclasspath_fragment
// requires from a prebuilt .apex file.
//
// If there is no image config associated with this fragment then it returns nil. Otherwise, it
// returns the files that are listed in the image config.
func (module *prebuiltBootclasspathFragmentModule) RequiredFilesFromPrebuiltApex(ctx android.BaseModuleContext) []string {
	imageConfig := module.getImageConfig(ctx)
	if imageConfig != nil {
		// Add the boot image files, e.g. .art, .oat and .vdex files.
		files := []string{}
		for _, variant := range imageConfig.apexVariants() {
			arch := variant.target.Arch.ArchType
			for _, path := range variant.imagesDeps.Paths() {
				base := path.Base()
				files = append(files, apexRootRelativePathToBootImageFile(arch, base))
			}
		}
		return files
	}
	return nil
}

func apexRootRelativePathToBootImageFile(arch android.ArchType, base string) string {
	return filepath.Join("javalib", arch.String(), base)
}

var _ android.RequiredFilesFromPrebuiltApex = (*prebuiltBootclasspathFragmentModule)(nil)

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
