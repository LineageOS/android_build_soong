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
	"github.com/google/blueprint"
)

func init() {
	RegisterBootImageBuildComponents(android.InitRegistrationContext)
}

func RegisterBootImageBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("boot_image", bootImageFactory)
}

type bootImageProperties struct {
	// The name of the image this represents.
	//
	// Must be one of "art" or "boot".
	Image_name string
}

type BootImageModule struct {
	android.ModuleBase
	android.ApexModuleBase

	properties bootImageProperties
}

func bootImageFactory() android.Module {
	m := &BootImageModule{}
	m.AddProperties(&m.properties)
	android.InitAndroidArchModule(m, android.HostAndDeviceSupported, android.MultilibCommon)
	android.InitApexModule(m)
	return m
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
	if tag == dexpreopt.Dex2oatDepTag {
		// The dex2oat tool is only needed for building and is not required in the apex.
		return false
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
	if SkipDexpreoptBootJars(ctx) {
		return
	}

	// Add a dependency onto the dex2oat tool which is needed for creating the boot image. The
	// path is retrieved from the dependency by GetGlobalSoongConfig(ctx).
	dexpreopt.RegisterToolDeps(ctx)
}

func (b *BootImageModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Nothing to do if skipping the dexpreopt of boot image jars.
	if SkipDexpreoptBootJars(ctx) {
		return
	}

	// Force the GlobalSoongConfig to be created and cached for use by the dex_bootjars
	// GenerateSingletonBuildActions method as it cannot create it for itself.
	dexpreopt.GetGlobalSoongConfig(ctx)

	// Get a map of the image configs that are supported.
	imageConfigs := genBootImageConfigs(ctx)

	// Retrieve the config for this image.
	imageName := b.properties.Image_name
	imageConfig := imageConfigs[imageName]
	if imageConfig == nil {
		ctx.PropertyErrorf("image_name", "Unknown image name %q, expected one of %s", imageName, strings.Join(android.SortedStringKeys(imageConfigs), ", "))
		return
	}

	// Construct the boot image info from the config.
	info := BootImageInfo{imageConfig: imageConfig}

	// Make it available for other modules.
	ctx.SetProvider(BootImageInfoProvider, info)
}
