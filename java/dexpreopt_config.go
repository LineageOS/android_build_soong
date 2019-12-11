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

// dexpreoptGlobalConfig returns the global dexpreopt.config.  It is loaded once the first time it is called for any
// ctx.Config(), and returns the same data for all future calls with the same ctx.Config().  A value can be inserted
// for tests using setDexpreoptTestGlobalConfig.
func dexpreoptGlobalConfig(ctx android.PathContext) dexpreopt.GlobalConfig {
	return dexpreoptGlobalConfigRaw(ctx).global
}

type globalConfigAndRaw struct {
	global dexpreopt.GlobalConfig
	data   []byte
}

func dexpreoptGlobalConfigRaw(ctx android.PathContext) globalConfigAndRaw {
	return ctx.Config().Once(dexpreoptGlobalConfigKey, func() interface{} {
		if f := ctx.Config().DexpreoptGlobalConfig(); f != "" {
			ctx.AddNinjaFileDeps(f)
			globalConfig, data, err := dexpreopt.LoadGlobalConfig(ctx, f)
			if err != nil {
				panic(err)
			}
			return globalConfigAndRaw{globalConfig, data}
		}

		// No global config filename set, see if there is a test config set
		return ctx.Config().Once(dexpreoptTestGlobalConfigKey, func() interface{} {
			// Nope, return a config with preopting disabled
			return globalConfigAndRaw{dexpreopt.GlobalConfig{
				DisablePreopt:          true,
				DisableGenerateProfile: true,
			}, nil}
		})
	}).(globalConfigAndRaw)
}

// setDexpreoptTestGlobalConfig sets a GlobalConfig that future calls to dexpreoptGlobalConfig will return.  It must
// be called before the first call to dexpreoptGlobalConfig for the config.
func setDexpreoptTestGlobalConfig(config android.Config, globalConfig dexpreopt.GlobalConfig) {
	config.Once(dexpreoptTestGlobalConfigKey, func() interface{} { return globalConfigAndRaw{globalConfig, nil} })
}

var dexpreoptGlobalConfigKey = android.NewOnceKey("DexpreoptGlobalConfig")
var dexpreoptTestGlobalConfigKey = android.NewOnceKey("TestDexpreoptGlobalConfig")

// systemServerClasspath returns the on-device locations of the modules in the system server classpath.  It is computed
// once the first time it is called for any ctx.Config(), and returns the same slice for all future calls with the same
// ctx.Config().
func systemServerClasspath(ctx android.PathContext) []string {
	return ctx.Config().OnceStringSlice(systemServerClasspathKey, func() []string {
		global := dexpreoptGlobalConfig(ctx)

		var systemServerClasspathLocations []string
		for _, m := range global.SystemServerJars {
			systemServerClasspathLocations = append(systemServerClasspathLocations,
				filepath.Join("/system/framework", m+".jar"))
		}
		for _, m := range global.UpdatableSystemServerJars {
			systemServerClasspathLocations = append(systemServerClasspathLocations,
				dexpreopt.GetJarLocationFromApexJarPair(m))
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

	return targets
}

func stemOf(moduleName string) string {
	// b/139391334: the stem of framework-minus-apex is framework
	// This is hard coded here until we find a good way to query the stem
	// of a module before any other mutators are run
	if moduleName == "framework-minus-apex" {
		return "framework"
	}
	return moduleName
}

func getJarsFromApexJarPairs(apexJarPairs []string) []string {
	modules := make([]string, len(apexJarPairs))
	for i, p := range apexJarPairs {
		_, jar := dexpreopt.SplitApexJarPair(p)
		modules[i] = jar
	}
	return modules
}

var (
	bootImageConfigKey     = android.NewOnceKey("bootImageConfig")
	artBootImageName       = "art"
	frameworkBootImageName = "boot"
	apexBootImageName      = "apex"
)

// Construct the global boot image configs.
func genBootImageConfigs(ctx android.PathContext) map[string]*bootImageConfig {
	return ctx.Config().Once(bootImageConfigKey, func() interface{} {

		global := dexpreoptGlobalConfig(ctx)
		targets := dexpreoptTargets(ctx)
		deviceDir := android.PathForOutput(ctx, ctx.Config().DeviceName())

		artModules := global.ArtApexJars
		frameworkModules := android.RemoveListFromList(global.BootJars,
			concat(artModules, getJarsFromApexJarPairs(global.UpdatableBootJars)))

		artSubdir := "apex/com.android.art/javalib"
		frameworkSubdir := "system/framework"

		var artLocations, frameworkLocations []string
		for _, m := range artModules {
			artLocations = append(artLocations, filepath.Join("/"+artSubdir, stemOf(m)+".jar"))
		}
		for _, m := range frameworkModules {
			frameworkLocations = append(frameworkLocations, filepath.Join("/"+frameworkSubdir, stemOf(m)+".jar"))
		}

		// ART config for the primary boot image in the ART apex.
		// It includes the Core Libraries.
		artCfg := bootImageConfig{
			extension:        false,
			name:             artBootImageName,
			stem:             "boot",
			installSubdir:    artSubdir,
			modules:          artModules,
			dexLocations:     artLocations,
			dexLocationsDeps: artLocations,
		}

		// Framework config for the boot image extension.
		// It includes both the Core libraries and framework.
		frameworkCfg := bootImageConfig{
			extension:        false,
			name:             frameworkBootImageName,
			stem:             "boot",
			installSubdir:    frameworkSubdir,
			modules:          concat(artModules, frameworkModules),
			dexLocations:     concat(artLocations, frameworkLocations),
			dexLocationsDeps: concat(artLocations, frameworkLocations),
		}

		// Apex config for the  boot image used in the JIT-zygote experiment.
		// It includes both the Core libraries and framework.
		apexCfg := bootImageConfig{
			extension:        false,
			name:             apexBootImageName,
			stem:             "apex",
			installSubdir:    frameworkSubdir,
			modules:          concat(artModules, frameworkModules),
			dexLocations:     concat(artLocations, frameworkLocations),
			dexLocationsDeps: concat(artLocations, frameworkLocations),
		}

		configs := map[string]*bootImageConfig{
			artBootImageName:       &artCfg,
			frameworkBootImageName: &frameworkCfg,
			apexBootImageName:      &apexCfg,
		}

		// common to all configs
		for _, c := range configs {
			c.targets = targets

			c.dir = deviceDir.Join(ctx, "dex_"+c.name+"jars")
			c.symbolsDir = deviceDir.Join(ctx, "dex_"+c.name+"jars_unstripped")

			// expands to <stem>.art for primary image and <stem>-<1st module>.art for extension
			imageName := c.firstModuleNameOrStem() + ".art"

			c.imageLocations = []string{c.dir.Join(ctx, c.installSubdir, imageName).String()}

			// The path to bootclasspath dex files needs to be known at module
			// GenerateAndroidBuildAction time, before the bootclasspath modules have been compiled.
			// Set up known paths for them, the singleton rules will copy them there.
			// TODO(b/143682396): use module dependencies instead
			inputDir := deviceDir.Join(ctx, "dex_"+c.name+"jars_input")
			for _, m := range c.modules {
				c.dexPaths = append(c.dexPaths, inputDir.Join(ctx, stemOf(m)+".jar"))
			}
			c.dexPathsDeps = c.dexPaths

			c.images = make(map[android.ArchType]android.OutputPath)
			c.imagesDeps = make(map[android.ArchType]android.OutputPaths)

			for _, target := range targets {
				arch := target.Arch.ArchType
				imageDir := c.dir.Join(ctx, c.installSubdir, arch.String())
				c.images[arch] = imageDir.Join(ctx, imageName)
				c.imagesDeps[arch] = c.moduleFiles(ctx, imageDir, ".art", ".oat", ".vdex")
			}

			c.zip = c.dir.Join(ctx, c.name+".zip")
		}

		return configs
	}).(map[string]*bootImageConfig)
}

func artBootImageConfig(ctx android.PathContext) bootImageConfig {
	return *genBootImageConfigs(ctx)[artBootImageName]
}

func defaultBootImageConfig(ctx android.PathContext) bootImageConfig {
	return *genBootImageConfigs(ctx)[frameworkBootImageName]
}

func apexBootImageConfig(ctx android.PathContext) bootImageConfig {
	return *genBootImageConfigs(ctx)[apexBootImageName]
}

func defaultBootclasspath(ctx android.PathContext) []string {
	return ctx.Config().OnceStringSlice(defaultBootclasspathKey, func() []string {
		global := dexpreoptGlobalConfig(ctx)
		image := defaultBootImageConfig(ctx)

		updatableBootclasspath := make([]string, len(global.UpdatableBootJars))
		for i, p := range global.UpdatableBootJars {
			updatableBootclasspath[i] = dexpreopt.GetJarLocationFromApexJarPair(p)
		}

		bootclasspath := append(copyOf(image.dexLocationsDeps), updatableBootclasspath...)
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
	ctx.Strict("PRODUCT_DEX2OAT_BOOTCLASSPATH", strings.Join(defaultBootImageConfig(ctx).dexLocationsDeps, ":"))
	ctx.Strict("PRODUCT_SYSTEM_SERVER_CLASSPATH", strings.Join(systemServerClasspath(ctx), ":"))

	ctx.Strict("DEXPREOPT_BOOT_JARS_MODULES", strings.Join(defaultBootImageConfig(ctx).modules, ":"))
}
