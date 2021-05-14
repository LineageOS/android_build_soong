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

// The tag used for the dependency between the bootclasspath_fragment module and its contents.
var bootclasspathFragmentContentDepTag = bootclasspathFragmentContentDependencyTag{}

var _ android.ExcludeFromVisibilityEnforcementTag = bootclasspathFragmentContentDepTag
var _ android.ReplaceSourceWithPrebuilt = bootclasspathFragmentContentDepTag
var _ android.SdkMemberTypeDependencyTag = bootclasspathFragmentContentDepTag

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
	// Updates the supplied flagFileInfo with the paths to the generated files set.
	produceHiddenAPIAllFlagsFile(ctx android.ModuleContext, contents []hiddenAPIModule, stubJarsByKind map[android.SdkKind]android.Paths, flagFileInfo *hiddenAPIFlagFileInfo)
}

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
	if m.properties.Image_name == nil && len(contents) == 0 {
		ctx.ModuleErrorf(`neither of the "image_name" and "contents" properties have been supplied, please supply exactly one`)
	}

	imageName := proptools.String(m.properties.Image_name)
	if imageName == "art" {
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
	// ClasspathFragmentProtoOutput is an output path for the generated classpaths.proto config of this module.
	//
	// The file should be copied to a relevant place on device, see ClasspathFragmentProtoInstallDir
	// for more details.
	ClasspathFragmentProtoOutput android.OutputPath

	// ClasspathFragmentProtoInstallDir contains information about on device location for the generated classpaths.proto file.
	//
	// The path encodes expected sub-location within partitions, i.e. etc/classpaths/<proto-file>,
	// for ClasspathFragmentProtoOutput. To get sub-location, instead of the full output / make path
	// use android.InstallPath#Rel().
	//
	// This is only relevant for APEX modules as they perform their own installation; while regular
	// system files are installed via ClasspathFragmentBase#androidMkEntries().
	ClasspathFragmentProtoInstallDir android.InstallPath

	// The image config, internal to this module (and the dex_bootjars singleton).
	//
	// Will be nil if the BootclasspathFragmentApexContentInfo has not been provided for a specific module. That can occur
	// when SkipDexpreoptBootJars(ctx) returns true.
	imageConfig *bootImageConfig
}

func (i BootclasspathFragmentApexContentInfo) Modules() android.ConfiguredJarList {
	return i.imageConfig.modules
}

// Get a map from ArchType to the associated boot image's contents for Android.
//
// Extension boot images only return their own files, not the files of the boot images they extend.
func (i BootclasspathFragmentApexContentInfo) AndroidBootImageFilesByArchType() map[android.ArchType]android.OutputPaths {
	files := map[android.ArchType]android.OutputPaths{}
	if i.imageConfig != nil {
		for _, variant := range i.imageConfig.variants {
			// We also generate boot images for host (for testing), but we don't need those in the apex.
			// TODO(b/177892522) - consider changing this to check Os.OsClass = android.Device
			if variant.target.Os == android.Android {
				files[variant.target.Arch.ArchType] = variant.imagesDeps
			}
		}
	}
	return files
}

// DexBootJarPathForContentModule returns the path to the dex boot jar for specified module.
//
// The dex boot jar is one which has had hidden API encoding performed on it.
func (i BootclasspathFragmentApexContentInfo) DexBootJarPathForContentModule(module android.Module) android.Path {
	j := module.(UsesLibraryDependency)
	dexJar := j.DexJarBuildPath()
	return dexJar
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
			if sdkLibrary, ok := module.(SdkLibraryDependency); ok && sdkLibrary.sharedLibrary() {
				ctx.PropertyErrorf("contents", "invalid module: %s, shared libraries cannot be on the bootclasspath", ctx.OtherModuleName(module))
			} else {
				contents = append(contents, module)
			}
		}
	})

	// Perform hidden API processing.
	b.generateHiddenAPIBuildActions(ctx, contents)

	// Construct the boot image info from the config.
	info := BootclasspathFragmentApexContentInfo{
		ClasspathFragmentProtoInstallDir: b.classpathFragmentBase().installDirPath,
		ClasspathFragmentProtoOutput:     b.classpathFragmentBase().outputFilepath,
		imageConfig:                      nil,
	}

	if !SkipDexpreoptBootJars(ctx) {
		// Force the GlobalSoongConfig to be created and cached for use by the dex_bootjars
		// GenerateSingletonBuildActions method as it cannot create it for itself.
		dexpreopt.GetGlobalSoongConfig(ctx)
		info.imageConfig = b.getImageConfig(ctx)

		// Only generate the boot image if the configuration does not skip it.
		b.generateBootImageBuildActions(ctx, contents)
	}

	// Make it available for other modules.
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
	// TODO(satayev): populate with actual content
	return android.EmptyConfiguredJarList()
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
func (b *BootclasspathFragmentModule) generateHiddenAPIBuildActions(ctx android.ModuleContext, contents []android.Module) {

	// Convert the kind specific lists of modules into kind specific lists of jars.
	stubJarsByKind := hiddenAPIGatherStubLibDexJarPaths(ctx, contents)

	// Store the information for use by other modules.
	bootclasspathApiInfo := bootclasspathApiInfo{stubJarsByKind: stubJarsByKind}
	ctx.SetProvider(bootclasspathApiInfoProvider, bootclasspathApiInfo)

	// Resolve the properties to paths.
	flagFileInfo := b.properties.Hidden_api.hiddenAPIFlagFileInfo(ctx)

	hiddenAPIModules := gatherHiddenAPIModuleFromContents(ctx, contents)

	// Delegate the production of the hidden API all flags file to a module type specific method.
	common := ctx.Module().(commonBootclasspathFragment)
	common.produceHiddenAPIAllFlagsFile(ctx, hiddenAPIModules, stubJarsByKind, &flagFileInfo)

	// Store the information for use by platform_bootclasspath.
	ctx.SetProvider(hiddenAPIFlagFileInfoProvider, flagFileInfo)
}

// produceHiddenAPIAllFlagsFile produces the hidden API all-flags.csv file (and supporting files)
// for the fragment.
func (b *BootclasspathFragmentModule) produceHiddenAPIAllFlagsFile(ctx android.ModuleContext, contents []hiddenAPIModule, stubJarsByKind map[android.SdkKind]android.Paths, flagFileInfo *hiddenAPIFlagFileInfo) {
	// If no stubs have been provided then don't perform hidden API processing. This is a temporary
	// workaround to avoid existing bootclasspath_fragments that do not provide stubs breaking the
	// build.
	// TODO(b/179354495): Remove this workaround.
	if len(stubJarsByKind) == 0 {
		// Nothing to do.
		return
	}

	// Generate the rules to create the hidden API flags and update the supplied flagFileInfo with the
	// paths to the created files.
	hiddenAPIGenerateAllFlagsForBootclasspathFragment(ctx, contents, stubJarsByKind, flagFileInfo)
}

// generateBootImageBuildActions generates ninja rules to create the boot image if required for this
// module.
func (b *BootclasspathFragmentModule) generateBootImageBuildActions(ctx android.ModuleContext, contents []android.Module) {
	global := dexpreopt.GetGlobalConfig(ctx)
	if !shouldBuildBootImages(ctx.Config(), global) {
		return
	}

	// Bootclasspath fragment modules that are not preferred do not produce a boot image.
	if !isActiveModule(ctx.Module()) {
		return
	}

	// Bootclasspath fragment modules that have no image_name property do not produce a boot image.
	imageConfig := b.getImageConfig(ctx)
	if imageConfig == nil {
		return
	}

	// Bootclasspath fragment modules that are for the platform do not produce a boot image.
	apexInfo := ctx.Provider(android.ApexInfoProvider).(android.ApexInfo)
	if apexInfo.IsForPlatform() {
		return
	}

	// Bootclasspath fragment modules that are versioned do not produce a boot image.
	if android.IsModuleInVersionedSdk(ctx.Module()) {
		return
	}

	// Copy the dex jars of this fragment's content modules to their predefined locations.
	copyBootJarsToPredefinedLocations(ctx, contents, imageConfig.modules, imageConfig.dexPaths)

	// Build a profile for the image config and then use that to build the boot image.
	profile := bootImageProfileRule(ctx, imageConfig)
	buildBootImage(ctx, imageConfig, profile)
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
	Flag_files_by_category map[*hiddenAPIFlagFileCategory]android.Paths

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

func pathsToOptionalPath(paths android.Paths) android.OptionalPath {
	switch len(paths) {
	case 0:
		return android.OptionalPath{}
	case 1:
		return android.OptionalPathForPath(paths[0])
	default:
		panic(fmt.Errorf("expected 0 or 1 paths, found %q", paths))
	}
}

func (b *bootclasspathFragmentSdkMemberProperties) PopulateFromVariant(ctx android.SdkMemberContext, variant android.Module) {
	module := variant.(*BootclasspathFragmentModule)

	b.Image_name = module.properties.Image_name
	b.Contents = module.properties.Contents

	// Get the flag file information from the module.
	mctx := ctx.SdkModuleContext()
	flagFileInfo := mctx.OtherModuleProvider(module, hiddenAPIFlagFileInfoProvider).(hiddenAPIFlagFileInfo)
	b.Flag_files_by_category = flagFileInfo.categoryToPaths

	// Copy all the generated file paths.
	b.Stub_flags_path = pathsToOptionalPath(flagFileInfo.StubFlagsPaths)
	b.Annotation_flags_path = pathsToOptionalPath(flagFileInfo.AnnotationFlagsPaths)
	b.Metadata_path = pathsToOptionalPath(flagFileInfo.MetadataPaths)
	b.Index_path = pathsToOptionalPath(flagFileInfo.IndexPaths)
	b.All_flags_path = pathsToOptionalPath(flagFileInfo.AllFlagsPaths)

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
		for _, category := range hiddenAPIFlagFileCategories {
			paths := b.Flag_files_by_category[category]
			if len(paths) > 0 {
				dests := []string{}
				for _, p := range paths {
					dest := filepath.Join(hiddenAPIDir, p.Base())
					builder.CopyToSnapshot(p, dest)
					dests = append(dests, dest)
				}
				hiddenAPISet.AddProperty(category.propertyName, dests)
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
func (module *prebuiltBootclasspathFragmentModule) produceHiddenAPIAllFlagsFile(ctx android.ModuleContext, contents []hiddenAPIModule, stubJarsByKind map[android.SdkKind]android.Paths, flagFileInfo *hiddenAPIFlagFileInfo) {
	pathsForOptionalSrc := func(src *string) android.Paths {
		if src == nil {
			// TODO(b/179354495): Fail if this is not provided once prebuilts have been updated.
			return nil
		}
		return android.Paths{android.PathForModuleSrc(ctx, *src)}
	}

	flagFileInfo.StubFlagsPaths = pathsForOptionalSrc(module.prebuiltProperties.Hidden_api.Stub_flags)
	flagFileInfo.AnnotationFlagsPaths = pathsForOptionalSrc(module.prebuiltProperties.Hidden_api.Annotation_flags)
	flagFileInfo.MetadataPaths = pathsForOptionalSrc(module.prebuiltProperties.Hidden_api.Metadata)
	flagFileInfo.IndexPaths = pathsForOptionalSrc(module.prebuiltProperties.Hidden_api.Index)
	flagFileInfo.AllFlagsPaths = pathsForOptionalSrc(module.prebuiltProperties.Hidden_api.All_flags)
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
