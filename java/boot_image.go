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
	"strings"

	"android/soong/android"
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

	properties bootImageProperties
}

func bootImageFactory() android.Module {
	m := &BootImageModule{}
	m.AddProperties(&m.properties)
	android.InitAndroidArchModule(m, android.HostAndDeviceDefault, android.MultilibCommon)
	return m
}

var BootImageInfoProvider = blueprint.NewProvider(BootImageInfo{})

type BootImageInfo struct {
	// The image config, internal to this module (and the dex_bootjars singleton).
	imageConfig *bootImageConfig
}

func (i BootImageInfo) Modules() android.ConfiguredJarList {
	return i.imageConfig.modules
}

func (b *BootImageModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Nothing to do if skipping the dexpreopt of boot image jars.
	if SkipDexpreoptBootJars(ctx) {
		return
	}

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
