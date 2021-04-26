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

// The tag used for the dependency between the bootclasspath_fragment module and its contents.
var bootclasspathFragmentContentDepTag = bootclasspathFragmentContentDependencyTag{}

var _ android.ExcludeFromVisibilityEnforcementTag = bootclasspathFragmentContentDepTag
var _ android.ReplaceSourceWithPrebuilt = bootclasspathFragmentContentDepTag

func IsBootclasspathFragmentContentDepTag(tag blueprint.DependencyTag) bool {
	return tag == bootclasspathFragmentContentDepTag
}

type bootclasspathFragmentProperties struct {
	// The name of the image this represents.
	//
	// If specified then it must be one of "art" or "boot".
	Image_name *string

	// The contents of this bootclasspath_fragment, could be either java_library, java_sdk_library, or boot_image.
	//
	// The order of this list matters as it is the order that is used in the bootclasspath.
	Contents []string

	Hidden_api HiddenAPIFlagFileProperties
}

type BootclasspathFragmentModule struct {
	android.ModuleBase
	android.ApexModuleBase
	android.SdkBase
	properties bootclasspathFragmentProperties
}

func bootclasspathFragmentFactory() android.Module {
	m := &BootclasspathFragmentModule{}
	m.AddProperties(&m.properties)
	android.InitApexModule(m)
	android.InitSdkAwareModule(m)
	android.InitAndroidArchModule(m, android.HostAndDeviceSupported, android.MultilibCommon)

	// Initialize the contents property from the image_name.
	android.AddLoadHook(m, func(ctx android.LoadHookContext) {
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
	if m.properties.Image_name != nil && len(contents) != 0 {
		ctx.ModuleErrorf(`both of the "image_name" and "contents" properties have been supplied, please supply exactly one`)
	}

	imageName := proptools.String(m.properties.Image_name)
	if imageName == "art" {
		// TODO(b/177892522): Prebuilts (versioned or not) should not use the image_name property.
		if m.MemberName() != "" {
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
		jars := []string{}
		commonApex := ""
		for i := 0; i < modules.Len(); i++ {
			apex := modules.Apex(i)
			jar := modules.Jar(i)
			if apex == "platform" {
				ctx.ModuleErrorf("ArtApexJars is invalid as it requests a platform variant of %q", jar)
				continue
			}
			if !m.AvailableFor(apex) {
				ctx.ModuleErrorf("incompatible with ArtApexJars which expects this to be in apex %q but this is only in apexes %q",
					apex, m.ApexAvailable())
				continue
			}
			if commonApex == "" {
				commonApex = apex
			} else if commonApex != apex {
				ctx.ModuleErrorf("ArtApexJars configuration is inconsistent, expected all jars to be in the same apex but it specifies apex %q and %q",
					commonApex, apex)
			}
			jars = append(jars, jar)
		}

		// Store the jars in the Contents property so that they can be used to add dependencies.
		m.properties.Contents = jars
	}
}

var BootclasspathFragmentApexContentInfoProvider = blueprint.NewProvider(BootclasspathFragmentApexContentInfo{})

// BootclasspathFragmentApexContentInfo contains the bootclasspath_fragments contributions to the
// apex contents.
type BootclasspathFragmentApexContentInfo struct {
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

	if SkipDexpreoptBootJars(ctx) {
		return
	}

	// Add a dependency onto the dex2oat tool which is needed for creating the boot image. The
	// path is retrieved from the dependency by GetGlobalSoongConfig(ctx).
	dexpreopt.RegisterToolDeps(ctx)
}

func (b *BootclasspathFragmentModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Perform hidden API processing.
	b.generateHiddenAPIBuildActions(ctx)

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
	info := BootclasspathFragmentApexContentInfo{imageConfig: imageConfig}

	// Make it available for other modules.
	ctx.SetProvider(BootclasspathFragmentApexContentInfoProvider, info)
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
func (b *BootclasspathFragmentModule) generateHiddenAPIBuildActions(ctx android.ModuleContext) {
	// Resolve the properties to paths.
	flagFileInfo := b.properties.Hidden_api.hiddenAPIFlagFileInfo(ctx)

	// Store the information for use by platform_bootclasspath.
	ctx.SetProvider(hiddenAPIFlagFileInfoProvider, flagFileInfo)
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

	// Flag files by *hiddenAPIFlagFileCategory
	Flag_files_by_category map[*hiddenAPIFlagFileCategory]android.Paths
}

func (b *bootclasspathFragmentSdkMemberProperties) PopulateFromVariant(ctx android.SdkMemberContext, variant android.Module) {
	module := variant.(*BootclasspathFragmentModule)

	b.Image_name = module.properties.Image_name
	if b.Image_name == nil {
		// Only one of image_name or contents can be specified. However, if image_name is set then the
		// contents property is updated to match the configuration used to create the corresponding
		// boot image. Therefore, contents property is only copied if the image name is not specified.
		b.Contents = module.properties.Contents
	}

	// Get the flag file information from the module.
	mctx := ctx.SdkModuleContext()
	flagFileInfo := mctx.OtherModuleProvider(module, hiddenAPIFlagFileInfoProvider).(hiddenAPIFlagFileInfo)
	b.Flag_files_by_category = flagFileInfo.categoryToPaths
}

func (b *bootclasspathFragmentSdkMemberProperties) AddToPropertySet(ctx android.SdkMemberContext, propertySet android.BpPropertySet) {
	if b.Image_name != nil {
		propertySet.AddProperty("image_name", *b.Image_name)
	}

	if len(b.Contents) > 0 {
		propertySet.AddPropertyWithTag("contents", b.Contents, ctx.SnapshotBuilder().SdkMemberReferencePropertyTag(true))
	}

	builder := ctx.SnapshotBuilder()
	if b.Flag_files_by_category != nil {
		hiddenAPISet := propertySet.AddPropertySet("hidden_api")
		for _, category := range hiddenAPIFlagFileCategories {
			paths := b.Flag_files_by_category[category]
			if len(paths) > 0 {
				dests := []string{}
				for _, p := range paths {
					dest := filepath.Join("hiddenapi", p.Base())
					builder.CopyToSnapshot(p, dest)
					dests = append(dests, dest)
				}
				hiddenAPISet.AddProperty(category.propertyName, dests)
			}
		}
	}
}

var _ android.SdkMemberType = (*bootclasspathFragmentMemberType)(nil)

// A prebuilt version of the bootclasspath_fragment module.
//
// At the moment this is basically just a bootclasspath_fragment module that can be used as a
// prebuilt. Eventually as more functionality is migrated into the bootclasspath_fragment module
// type from the various singletons then this will diverge.
type prebuiltBootclasspathFragmentModule struct {
	BootclasspathFragmentModule
	prebuilt android.Prebuilt
}

func (module *prebuiltBootclasspathFragmentModule) Prebuilt() *android.Prebuilt {
	return &module.prebuilt
}

func (module *prebuiltBootclasspathFragmentModule) Name() string {
	return module.prebuilt.Name(module.ModuleBase.Name())
}

func prebuiltBootclasspathFragmentFactory() android.Module {
	m := &prebuiltBootclasspathFragmentModule{}
	m.AddProperties(&m.properties)
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
