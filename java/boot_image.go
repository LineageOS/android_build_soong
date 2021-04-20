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
	"strings"

	"android/soong/android"
	"android/soong/dexpreopt"
	"github.com/google/blueprint/proptools"

	"github.com/google/blueprint"
)

func init() {
	RegisterBootImageBuildComponents(android.InitRegistrationContext)

	// TODO(b/177892522): Remove after has been replaced by bootclasspath_fragments
	android.RegisterSdkMemberType(&bootImageMemberType{
		SdkMemberTypeBase: android.SdkMemberTypeBase{
			PropertyName: "boot_images",
			SupportsSdk:  true,
		},
	})

	android.RegisterSdkMemberType(&bootImageMemberType{
		SdkMemberTypeBase: android.SdkMemberTypeBase{
			PropertyName: "bootclasspath_fragments",
			SupportsSdk:  true,
		},
	})
}

func RegisterBootImageBuildComponents(ctx android.RegistrationContext) {
	// TODO(b/177892522): Remove after has been replaced by bootclasspath_fragment
	ctx.RegisterModuleType("boot_image", bootImageFactory)
	ctx.RegisterModuleType("prebuilt_boot_image", prebuiltBootImageFactory)

	ctx.RegisterModuleType("bootclasspath_fragment", bootImageFactory)
	ctx.RegisterModuleType("prebuilt_bootclasspath_fragment", prebuiltBootImageFactory)
}

type bootImageContentDependencyTag struct {
	blueprint.BaseDependencyTag
}

// Avoid having to make boot image content visible to the boot image.
//
// This is a temporary workaround to make it easier to migrate to boot image modules with proper
// dependencies.
// TODO(b/177892522): Remove this and add needed visibility.
func (b bootImageContentDependencyTag) ExcludeFromVisibilityEnforcement() {
}

// The tag used for the dependency between the boot image module and its contents.
var bootImageContentDepTag = bootImageContentDependencyTag{}

var _ android.ExcludeFromVisibilityEnforcementTag = bootImageContentDepTag

func IsbootImageContentDepTag(tag blueprint.DependencyTag) bool {
	return tag == bootImageContentDepTag
}

type bootImageProperties struct {
	// The name of the image this represents.
	//
	// If specified then it must be one of "art" or "boot".
	Image_name *string

	// The contents of this boot image, could be either java_library, java_sdk_library, or boot_image.
	//
	// The order of this list matters as it is the order that is used in the bootclasspath.
	Contents []string

	Hidden_api HiddenAPIFlagFileProperties
}

type BootImageModule struct {
	android.ModuleBase
	android.ApexModuleBase
	android.SdkBase
	properties bootImageProperties
}

func bootImageFactory() android.Module {
	m := &BootImageModule{}
	m.AddProperties(&m.properties)
	android.InitApexModule(m)
	android.InitSdkAwareModule(m)
	android.InitAndroidArchModule(m, android.HostAndDeviceSupported, android.MultilibCommon)

	// Perform some consistency checking to ensure that the configuration is correct.
	android.AddLoadHook(m, func(ctx android.LoadHookContext) {
		bootImageConsistencyCheck(ctx, m)
	})
	return m
}

func bootImageConsistencyCheck(ctx android.EarlyModuleContext, m *BootImageModule) {
	contents := m.properties.Contents
	if m.properties.Image_name == nil && len(contents) == 0 {
		ctx.ModuleErrorf(`neither of the "image_name" and "contents" properties have been supplied, please supply exactly one`)
	}
	if m.properties.Image_name != nil && len(contents) != 0 {
		ctx.ModuleErrorf(`both of the "image_name" and "contents" properties have been supplied, please supply exactly one`)
	}
	imageName := proptools.String(m.properties.Image_name)
	if imageName == "art" {
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

var BootImageInfoProvider = blueprint.NewProvider(BootImageInfo{})

type BootImageInfo struct {
	// The image config, internal to this module (and the dex_bootjars singleton).
	//
	// Will be nil if the BootImageInfo has not been provided for a specific module. That can occur
	// when SkipDexpreoptBootJars(ctx) returns true.
	imageConfig *bootImageConfig
}

func (i BootImageInfo) Modules() android.ConfiguredJarList {
	return i.imageConfig.modules
}

// Get a map from ArchType to the associated boot image's contents for Android.
//
// Extension boot images only return their own files, not the files of the boot images they extend.
func (i BootImageInfo) AndroidBootImageFilesByArchType() map[android.ArchType]android.OutputPaths {
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

func (b *BootImageModule) DepIsInSameApex(ctx android.BaseModuleContext, dep android.Module) bool {
	tag := ctx.OtherModuleDependencyTag(dep)
	if tag == bootImageContentDepTag {
		// Boot image contents are automatically added to apex.
		return true
	}
	if android.IsMetaDependencyTag(tag) {
		// Cross-cutting metadata dependencies are metadata.
		return false
	}
	panic(fmt.Errorf("boot_image module %q should not have a dependency on %q via tag %s", b, dep, android.PrettyPrintTag(tag)))
}

func (b *BootImageModule) ShouldSupportSdkVersion(ctx android.BaseModuleContext, sdkVersion android.ApiLevel) error {
	return nil
}

func (b *BootImageModule) DepsMutator(ctx android.BottomUpMutatorContext) {
	ctx.AddDependency(ctx.Module(), bootImageContentDepTag, b.properties.Contents...)

	if SkipDexpreoptBootJars(ctx) {
		return
	}

	// Add a dependency onto the dex2oat tool which is needed for creating the boot image. The
	// path is retrieved from the dependency by GetGlobalSoongConfig(ctx).
	dexpreopt.RegisterToolDeps(ctx)
}

func (b *BootImageModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
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
	info := BootImageInfo{imageConfig: imageConfig}

	// Make it available for other modules.
	ctx.SetProvider(BootImageInfoProvider, info)
}

func (b *BootImageModule) getImageConfig(ctx android.EarlyModuleContext) *bootImageConfig {
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
func (b *BootImageModule) generateHiddenAPIBuildActions(ctx android.ModuleContext) {
	// Resolve the properties to paths.
	flagFileInfo := b.properties.Hidden_api.hiddenAPIFlagFileInfo(ctx)

	// Store the information for use by platform_bootclasspath.
	ctx.SetProvider(hiddenAPIFlagFileInfoProvider, flagFileInfo)
}

type bootImageMemberType struct {
	android.SdkMemberTypeBase
}

func (b *bootImageMemberType) AddDependencies(mctx android.BottomUpMutatorContext, dependencyTag blueprint.DependencyTag, names []string) {
	mctx.AddVariationDependencies(nil, dependencyTag, names...)
}

func (b *bootImageMemberType) IsInstance(module android.Module) bool {
	_, ok := module.(*BootImageModule)
	return ok
}

func (b *bootImageMemberType) AddPrebuiltModule(ctx android.SdkMemberContext, member android.SdkMember) android.BpModule {
	if b.PropertyName == "boot_images" {
		return ctx.SnapshotBuilder().AddPrebuiltModule(member, "prebuilt_boot_image")
	} else {
		return ctx.SnapshotBuilder().AddPrebuiltModule(member, "prebuilt_bootclasspath_fragment")
	}
}

func (b *bootImageMemberType) CreateVariantPropertiesStruct() android.SdkMemberProperties {
	return &bootImageSdkMemberProperties{}
}

type bootImageSdkMemberProperties struct {
	android.SdkMemberPropertiesBase

	Image_name *string
}

func (b *bootImageSdkMemberProperties) PopulateFromVariant(ctx android.SdkMemberContext, variant android.Module) {
	module := variant.(*BootImageModule)

	b.Image_name = module.properties.Image_name
}

func (b *bootImageSdkMemberProperties) AddToPropertySet(ctx android.SdkMemberContext, propertySet android.BpPropertySet) {
	if b.Image_name != nil {
		propertySet.AddProperty("image_name", *b.Image_name)
	}
}

var _ android.SdkMemberType = (*bootImageMemberType)(nil)

// A prebuilt version of the boot image module.
//
// At the moment this is basically just a boot image module that can be used as a prebuilt.
// Eventually as more functionality is migrated into the boot image module from the singleton then
// this will diverge.
type prebuiltBootImageModule struct {
	BootImageModule
	prebuilt android.Prebuilt
}

func (module *prebuiltBootImageModule) Prebuilt() *android.Prebuilt {
	return &module.prebuilt
}

func (module *prebuiltBootImageModule) Name() string {
	return module.prebuilt.Name(module.ModuleBase.Name())
}

func prebuiltBootImageFactory() android.Module {
	m := &prebuiltBootImageModule{}
	m.AddProperties(&m.properties)
	// This doesn't actually have any prebuilt files of its own so pass a placeholder for the srcs
	// array.
	android.InitPrebuiltModule(m, &[]string{"placeholder"})
	android.InitApexModule(m)
	android.InitSdkAwareModule(m)
	android.InitAndroidArchModule(m, android.HostAndDeviceSupported, android.MultilibCommon)

	// Perform some consistency checking to ensure that the configuration is correct.
	android.AddLoadHook(m, func(ctx android.LoadHookContext) {
		bootImageConsistencyCheck(ctx, &m.BootImageModule)
	})
	return m
}
