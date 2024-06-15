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
	for _, target := range ctx.Config().Targets[ctx.Config().BuildOS] {
		targets = append(targets, target)
	}

	return targets
}

var (
	bootImageConfigKey       = android.NewOnceKey("bootImageConfig")
	bootImageConfigRawKey    = android.NewOnceKey("bootImageConfigRaw")
	frameworkBootImageName   = "boot"
	mainlineBootImageName    = "mainline"
	bootImageStem            = "boot"
	ProfileInstallPathInApex = "etc/boot-image.prof"
)

// getImageNames returns an ordered list of image names. The order doesn't matter but needs to be
// deterministic. The names listed here must match the map keys returned by genBootImageConfigs.
func getImageNames() []string {
	return []string{"art", "boot", "mainline"}
}

func genBootImageConfigRaw(ctx android.PathContext) map[string]*bootImageConfig {
	return ctx.Config().Once(bootImageConfigRawKey, func() interface{} {
		global := dexpreopt.GetGlobalConfig(ctx)

		artBootImageName := "art"           // Keep this local to avoid accidental references.
		frameworkModules := global.BootJars // This includes `global.ArtApexJars`.
		mainlineBcpModules := global.ApexBootJars
		frameworkSubdir := "system/framework"

		profileImports := []string{"com.android.art"}

		// ART boot image for testing only. Do not rely on it to make any build-time decision.
		artCfg := bootImageConfig{
			name:                 artBootImageName,
			enabledIfExists:      "art-bootclasspath-fragment",
			stem:                 bootImageStem,
			installDir:           "apex/art_boot_images/javalib",
			modules:              global.TestOnlyArtBootImageJars,
			preloadedClassesFile: "art/build/boot/preloaded-classes",
			compilerFilter:       "speed-profile",
			singleImage:          false,
			profileImports:       profileImports,
		}

		// Framework config for the boot image extension.
		// It includes framework libraries and depends on the ART config.
		frameworkCfg := bootImageConfig{
			name:                 frameworkBootImageName,
			enabledIfExists:      "platform-bootclasspath",
			stem:                 bootImageStem,
			installDir:           frameworkSubdir,
			modules:              frameworkModules,
			preloadedClassesFile: "frameworks/base/config/preloaded-classes",
			compilerFilter:       "speed-profile",
			singleImage:          false,
			profileImports:       profileImports,
		}

		mainlineCfg := bootImageConfig{
			extends:         &frameworkCfg,
			name:            mainlineBootImageName,
			enabledIfExists: "platform-bootclasspath",
			stem:            bootImageStem,
			installDir:      frameworkSubdir,
			modules:         mainlineBcpModules,
			compilerFilter:  "verify",
			singleImage:     true,
		}

		return map[string]*bootImageConfig{
			artBootImageName:       &artCfg,
			frameworkBootImageName: &frameworkCfg,
			mainlineBootImageName:  &mainlineCfg,
		}
	}).(map[string]*bootImageConfig)
}

// Construct the global boot image configs.
func genBootImageConfigs(ctx android.PathContext) map[string]*bootImageConfig {
	return ctx.Config().Once(bootImageConfigKey, func() interface{} {
		targets := dexpreoptTargets(ctx)
		deviceDir := android.PathForOutput(ctx, dexpreopt.GetDexpreoptDirName(ctx))

		configs := genBootImageConfigRaw(ctx)

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
				imageDir := c.dir.Join(ctx, target.Os.String(), c.installDir, arch.String())
				variant := &bootImageVariant{
					bootImageConfig:   c,
					target:            target,
					imagePathOnHost:   imageDir.Join(ctx, imageName),
					imagePathOnDevice: filepath.Join("/", c.installDir, arch.String(), imageName),
					imagesDeps:        c.moduleFiles(ctx, imageDir, ".art", ".oat", ".vdex"),
					dexLocations:      c.modules.DevicePaths(ctx.Config(), target.Os),
				}
				variant.dexLocationsDeps = variant.dexLocations
				c.variants = append(c.variants, variant)
			}

			c.zip = c.dir.Join(ctx, c.name+".zip")
		}

		visited := make(map[string]bool)
		for _, c := range configs {
			calculateDepsRecursive(c, targets, visited)
		}

		return configs
	}).(map[string]*bootImageConfig)
}

// calculateDepsRecursive calculates the dependencies of the given boot image config and all its
// ancestors, if they are not visited.
// The boot images are supposed to form a tree, where the root is the primary boot image. We do not
// expect loops (e.g., A extends B, B extends C, C extends A), and we let them crash soong with a
// stack overflow.
// Note that a boot image config only has a pointer to the parent, not to children. Therefore, we
// first go up through the parent chain, and then go back down to visit every code along the path.
// `visited` is a map where a key is a boot image name and the value indicates whether the boot
// image config is visited. The boot image names are guaranteed to be unique because they come from
// `genBootImageConfigRaw` above, which also returns a map and would fail in the first place if the
// names were not unique.
func calculateDepsRecursive(c *bootImageConfig, targets []android.Target, visited map[string]bool) {
	if c.extends == nil || visited[c.name] {
		return
	}
	if c.extends.extends != nil {
		calculateDepsRecursive(c.extends, targets, visited)
	}
	visited[c.name] = true
	c.dexPathsDeps = android.Concat(c.extends.dexPathsDeps, c.dexPathsDeps)
	for i := range targets {
		c.variants[i].baseImages = android.Concat(c.extends.variants[i].baseImages, android.OutputPaths{c.extends.variants[i].imagePathOnHost})
		c.variants[i].baseImagesDeps = android.Concat(c.extends.variants[i].baseImagesDeps, c.extends.variants[i].imagesDeps.Paths())
		c.variants[i].dexLocationsDeps = android.Concat(c.extends.variants[i].dexLocationsDeps, c.variants[i].dexLocationsDeps)
	}
}

func defaultBootImageConfig(ctx android.PathContext) *bootImageConfig {
	return genBootImageConfigs(ctx)[frameworkBootImageName]
}

func mainlineBootImageConfig(ctx android.PathContext) *bootImageConfig {
	return genBootImageConfigs(ctx)[mainlineBootImageName]
}

// isProfileProviderApex returns true if this apex provides a boot image profile.
func isProfileProviderApex(ctx android.PathContext, apexName string) bool {
	for _, config := range genBootImageConfigs(ctx) {
		for _, profileImport := range config.profileImports {
			if profileImport == apexName {
				return true
			}
		}
	}
	return false
}

// Returns a list of paths and a list of locations for the boot jars used in dexpreopt (to be
// passed in -Xbootclasspath and -Xbootclasspath-locations arguments for dex2oat).
func bcpForDexpreopt(ctx android.PathContext, withUpdatable bool) (android.WritablePaths, []string) {
	bootImage := defaultBootImageConfig(ctx)
	if withUpdatable {
		bootImage = mainlineBootImageConfig(ctx)
	}

	dexPaths := bootImage.dexPathsDeps
	// The dex locations for all Android variants are identical.
	dexLocations := bootImage.getAnyAndroidVariant().dexLocationsDeps

	return dexPaths, dexLocations
}

var defaultBootclasspathKey = android.NewOnceKey("defaultBootclasspath")

func init() {
	android.RegisterMakeVarsProvider(pctx, dexpreoptConfigMakevars)
}

func dexpreoptConfigMakevars(ctx android.MakeVarsContext) {
	ctx.Strict("DEXPREOPT_BOOT_JARS_MODULES", strings.Join(defaultBootImageConfig(ctx).modules.CopyOfApexJarPairs(), ":"))
}
