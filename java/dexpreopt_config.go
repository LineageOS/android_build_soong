// Copyright 2019 Google Inc. All rights reserved.
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
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/dexpreopt"
)

// dexpreoptTargets returns the list of targets that are relevant to dexpreopting, which excludes architectures
// supported through native bridge.
func dexpreoptTargets(ctx android.PathContext) []android.Target {
	var targets []android.Target
	for _, target := range ctx.Config().Targets[android.Android] {
		if target.NativeBridge == android.NativeBridgeDisabled {
			targets = append(targets, target)
		}
	}
	// We may also need the images on host in order to run host-based tests.
	for _, target := range ctx.Config().Targets[android.BuildOs] {
		targets = append(targets, target)
	}

	return targets
}

var (
	bootImageConfigKey     = android.NewOnceKey("bootImageConfig")
	artBootImageName       = "art"
	frameworkBootImageName = "boot"
)

// Construct the global boot image configs.
func genBootImageConfigs(ctx android.PathContext) map[string]*bootImageConfig {
	return ctx.Config().Once(bootImageConfigKey, func() interface{} {

		global := dexpreopt.GetGlobalConfig(ctx)
		targets := dexpreoptTargets(ctx)
		deviceDir := android.PathForOutput(ctx, ctx.Config().DeviceName())

		artModules := global.ArtApexJars
		frameworkModules := global.BootJars.RemoveList(artModules)

		artDirOnHost := "apex/art_boot_images/javalib"
		artDirOnDevice := "apex/com.android.art/javalib"
		frameworkSubdir := "system/framework"

		// ART config for the primary boot image in the ART apex.
		// It includes the Core Libraries.
		artCfg := bootImageConfig{
			name:               artBootImageName,
			stem:               "boot",
			installDirOnHost:   artDirOnHost,
			installDirOnDevice: artDirOnDevice,
			modules:            artModules,
		}

		// Framework config for the boot image extension.
		// It includes framework libraries and depends on the ART config.
		frameworkCfg := bootImageConfig{
			extends:            &artCfg,
			name:               frameworkBootImageName,
			stem:               "boot",
			installDirOnHost:   frameworkSubdir,
			installDirOnDevice: frameworkSubdir,
			modules:            frameworkModules,
		}

		configs := map[string]*bootImageConfig{
			artBootImageName:       &artCfg,
			frameworkBootImageName: &frameworkCfg,
		}

		// common to all configs
		for _, c := range configs {
			c.dir = deviceDir.Join(ctx, "dex_"+c.name+"jars")
			c.symbolsDir = deviceDir.Join(ctx, "dex_"+c.name+"jars_unstripped")

			// expands to <stem>.art for primary image and <stem>-<1st module>.art for extension
			imageName := c.firstModuleNameOrStem(ctx) + ".art"

			// The path to bootclasspath dex files needs to be known at module
			// GenerateAndroidBuildAction time, before the bootclasspath modules have been compiled.
			// Set up known paths for them, the singleton rules will copy them there.
			// TODO(b/143682396): use module dependencies instead
			inputDir := deviceDir.Join(ctx, "dex_"+c.name+"jars_input")
			c.dexPaths = c.modules.BuildPaths(ctx, inputDir)
			c.dexPathsByModule = c.modules.BuildPathsByModule(ctx, inputDir)
			c.dexPathsDeps = c.dexPaths

			// Create target-specific variants.
			for _, target := range targets {
				arch := target.Arch.ArchType
				imageDir := c.dir.Join(ctx, target.Os.String(), c.installDirOnHost, arch.String())
				variant := &bootImageVariant{
					bootImageConfig:   c,
					target:            target,
					imagePathOnHost:   imageDir.Join(ctx, imageName),
					imagePathOnDevice: filepath.Join("/", c.installDirOnDevice, arch.String(), imageName),
					imagesDeps:        c.moduleFiles(ctx, imageDir, ".art", ".oat", ".vdex"),
					dexLocations:      c.modules.DevicePaths(ctx.Config(), target.Os),
				}
				variant.dexLocationsDeps = variant.dexLocations
				c.variants = append(c.variants, variant)
			}

			c.zip = c.dir.Join(ctx, c.name+".zip")
		}

		// specific to the framework config
		frameworkCfg.dexPathsDeps = append(artCfg.dexPathsDeps, frameworkCfg.dexPathsDeps...)
		for i := range targets {
			frameworkCfg.variants[i].primaryImages = artCfg.variants[i].imagePathOnHost
			frameworkCfg.variants[i].primaryImagesDeps = artCfg.variants[i].imagesDeps.Paths()
			frameworkCfg.variants[i].dexLocationsDeps = append(artCfg.variants[i].dexLocations, frameworkCfg.variants[i].dexLocationsDeps...)
		}

		return configs
	}).(map[string]*bootImageConfig)
}

func artBootImageConfig(ctx android.PathContext) *bootImageConfig {
	return genBootImageConfigs(ctx)[artBootImageName]
}

func defaultBootImageConfig(ctx android.PathContext) *bootImageConfig {
	return genBootImageConfigs(ctx)[frameworkBootImageName]
}

// Updatable boot config allows to access build/install paths of updatable boot jars without going
// through the usual trouble of registering dependencies on those modules and extracting build paths
// from those dependencies.
type updatableBootConfig struct {
	// A list of updatable boot jars.
	modules android.ConfiguredJarList

	// A list of predefined build paths to updatable boot jars. They are configured very early,
	// before the modules for these jars are processed and the actual paths are generated, and
	// later on a singleton adds commands to copy actual jars to the predefined paths.
	dexPaths android.WritablePaths

	// Map from module name (without prebuilt_ prefix) to the predefined build path.
	dexPathsByModule map[string]android.WritablePath

	// A list of dex locations (a.k.a. on-device paths) to the boot jars.
	dexLocations []string
}

var updatableBootConfigKey = android.NewOnceKey("updatableBootConfig")

// Returns updatable boot config.
func GetUpdatableBootConfig(ctx android.PathContext) updatableBootConfig {
	return ctx.Config().Once(updatableBootConfigKey, func() interface{} {
		updatableBootJars := dexpreopt.GetGlobalConfig(ctx).UpdatableBootJars

		dir := android.PathForOutput(ctx, ctx.Config().DeviceName(), "updatable_bootjars")
		dexPaths := updatableBootJars.BuildPaths(ctx, dir)
		dexPathsByModuleName := updatableBootJars.BuildPathsByModule(ctx, dir)

		dexLocations := updatableBootJars.DevicePaths(ctx.Config(), android.Android)

		return updatableBootConfig{updatableBootJars, dexPaths, dexPathsByModuleName, dexLocations}
	}).(updatableBootConfig)
}

// Returns a list of paths and a list of locations for the boot jars used in dexpreopt (to be
// passed in -Xbootclasspath and -Xbootclasspath-locations arguments for dex2oat).
func bcpForDexpreopt(ctx android.PathContext, withUpdatable bool) (android.WritablePaths, []string) {
	// Non-updatable boot jars (they are used both in the boot image and in dexpreopt).
	bootImage := defaultBootImageConfig(ctx)
	dexPaths := bootImage.dexPathsDeps
	// The dex locations for all Android variants are identical.
	dexLocations := bootImage.getAnyAndroidVariant().dexLocationsDeps

	if withUpdatable {
		// Updatable boot jars (they are used only in dexpreopt, but not in the boot image).
		updBootConfig := GetUpdatableBootConfig(ctx)
		dexPaths = append(dexPaths, updBootConfig.dexPaths...)
		dexLocations = append(dexLocations, updBootConfig.dexLocations...)
	}

	return dexPaths, dexLocations
}

var defaultBootclasspathKey = android.NewOnceKey("defaultBootclasspath")

var copyOf = android.CopyOf

func init() {
	android.RegisterMakeVarsProvider(pctx, dexpreoptConfigMakevars)
}

func dexpreoptConfigMakevars(ctx android.MakeVarsContext) {
	ctx.Strict("DEXPREOPT_BOOT_JARS_MODULES", strings.Join(defaultBootImageConfig(ctx).modules.CopyOfApexJarPairs(), ":"))
}
