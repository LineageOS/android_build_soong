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
	"fmt"
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/dexpreopt"
)

// systemServerClasspath returns the on-device locations of the modules in the system server classpath.  It is computed
// once the first time it is called for any ctx.Config(), and returns the same slice for all future calls with the same
// ctx.Config().
func systemServerClasspath(ctx android.MakeVarsContext) []string {
	return ctx.Config().OnceStringSlice(systemServerClasspathKey, func() []string {
		global := dexpreopt.GetGlobalConfig(ctx)
		var systemServerClasspathLocations []string
		nonUpdatable := dexpreopt.NonUpdatableSystemServerJars(ctx, global)
		// 1) Non-updatable jars.
		for _, m := range nonUpdatable {
			systemServerClasspathLocations = append(systemServerClasspathLocations,
				filepath.Join("/system/framework", m+".jar"))
		}
		// 2) The jars that are from an updatable apex.
		systemServerClasspathLocations = append(systemServerClasspathLocations,
			global.UpdatableSystemServerJars.DevicePaths(ctx.Config(), android.Android)...)
		if len(systemServerClasspathLocations) != len(global.SystemServerJars)+global.UpdatableSystemServerJars.Len() {
			panic(fmt.Errorf("Wrong number of system server jars, got %d, expected %d",
				len(systemServerClasspathLocations),
				len(global.SystemServerJars)+global.UpdatableSystemServerJars.Len()))
		}
		return systemServerClasspathLocations
	})
}

var systemServerClasspathKey = android.NewOnceKey("systemServerClasspath")

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

		artSubdir := "apex/art_boot_images/javalib"
		frameworkSubdir := "system/framework"

		// ART config for the primary boot image in the ART apex.
		// It includes the Core Libraries.
		artCfg := bootImageConfig{
			name:          artBootImageName,
			stem:          "boot",
			installSubdir: artSubdir,
			modules:       artModules,
		}

		// Framework config for the boot image extension.
		// It includes framework libraries and depends on the ART config.
		frameworkCfg := bootImageConfig{
			extends:       &artCfg,
			name:          frameworkBootImageName,
			stem:          "boot",
			installSubdir: frameworkSubdir,
			modules:       frameworkModules,
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
			c.dexPathsDeps = c.dexPaths

			// Create target-specific variants.
			for _, target := range targets {
				arch := target.Arch.ArchType
				imageDir := c.dir.Join(ctx, target.Os.String(), c.installSubdir, arch.String())
				variant := &bootImageVariant{
					bootImageConfig: c,
					target:          target,
					images:          imageDir.Join(ctx, imageName),
					imagesDeps:      c.moduleFiles(ctx, imageDir, ".art", ".oat", ".vdex"),
					dexLocations:    c.modules.DevicePaths(ctx.Config(), target.Os),
				}
				variant.dexLocationsDeps = variant.dexLocations
				c.variants = append(c.variants, variant)
			}

			c.zip = c.dir.Join(ctx, c.name+".zip")
		}

		// specific to the framework config
		frameworkCfg.dexPathsDeps = append(artCfg.dexPathsDeps, frameworkCfg.dexPathsDeps...)
		for i := range targets {
			frameworkCfg.variants[i].primaryImages = artCfg.variants[i].images
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

func defaultBootclasspath(ctx android.PathContext) []string {
	return ctx.Config().OnceStringSlice(defaultBootclasspathKey, func() []string {
		global := dexpreopt.GetGlobalConfig(ctx)
		image := defaultBootImageConfig(ctx)

		updatableBootclasspath := global.UpdatableBootJars.DevicePaths(ctx.Config(), android.Android)

		bootclasspath := append(copyOf(image.getAnyAndroidVariant().dexLocationsDeps), updatableBootclasspath...)
		return bootclasspath
	})
}

var defaultBootclasspathKey = android.NewOnceKey("defaultBootclasspath")

var copyOf = android.CopyOf

func init() {
	android.RegisterMakeVarsProvider(pctx, dexpreoptConfigMakevars)
}

func dexpreoptConfigMakevars(ctx android.MakeVarsContext) {
	ctx.Strict("PRODUCT_BOOTCLASSPATH", strings.Join(defaultBootclasspath(ctx), ":"))
	ctx.Strict("PRODUCT_DEX2OAT_BOOTCLASSPATH", strings.Join(defaultBootImageConfig(ctx).getAnyAndroidVariant().dexLocationsDeps, ":"))
	ctx.Strict("PRODUCT_SYSTEM_SERVER_CLASSPATH", strings.Join(systemServerClasspath(ctx), ":"))

	ctx.Strict("DEXPREOPT_BOOT_JARS_MODULES", strings.Join(defaultBootImageConfig(ctx).modules.CopyOfApexJarPairs(), ":"))
}
