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
	"android/soong/android"
	"android/soong/dexpreopt"
	"path/filepath"
	"strings"
)

// dexpreoptGlobalConfig returns the global dexpreopt.config.  It is loaded once the first time it is called for any
// ctx.Config(), and returns the same data for all future calls with the same ctx.Config().  A value can be inserted
// for tests using setDexpreoptTestGlobalConfig.
func dexpreoptGlobalConfig(ctx android.PathContext) dexpreopt.GlobalConfig {
	return ctx.Config().Once(dexpreoptGlobalConfigKey, func() interface{} {
		if f := ctx.Config().DexpreoptGlobalConfig(); f != "" {
			ctx.AddNinjaFileDeps(f)
			globalConfig, err := dexpreopt.LoadGlobalConfig(ctx, f)
			if err != nil {
				panic(err)
			}
			return globalConfig
		}

		// No global config filename set, see if there is a test config set
		return ctx.Config().Once(dexpreoptTestGlobalConfigKey, func() interface{} {
			// Nope, return a config with preopting disabled
			return dexpreopt.GlobalConfig{
				DisablePreopt: true,
			}
		})
	}).(dexpreopt.GlobalConfig)
}

// setDexpreoptTestGlobalConfig sets a GlobalConfig that future calls to dexpreoptGlobalConfig will return.  It must
// be called before the first call to dexpreoptGlobalConfig for the config.
func setDexpreoptTestGlobalConfig(config android.Config, globalConfig dexpreopt.GlobalConfig) {
	config.Once(dexpreoptTestGlobalConfigKey, func() interface{} { return globalConfig })
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
		return systemServerClasspathLocations
	})
}

var systemServerClasspathKey = android.NewOnceKey("systemServerClasspath")

// defaultBootImageConfig returns the bootImageConfig that will be used to dexpreopt modules.  It is computed once the
// first time it is called for any ctx.Config(), and returns the same slice for all future calls with the same
// ctx.Config().
func defaultBootImageConfig(ctx android.PathContext) bootImageConfig {
	return ctx.Config().Once(defaultBootImageConfigKey, func() interface{} {
		global := dexpreoptGlobalConfig(ctx)

		runtimeModules := global.RuntimeApexJars
		nonFrameworkModules := concat(runtimeModules, global.ProductUpdatableBootModules)
		frameworkModules := android.RemoveListFromList(global.BootJars, nonFrameworkModules)

		var nonUpdatableBootModules []string
		var nonUpdatableBootLocations []string

		for _, m := range runtimeModules {
			nonUpdatableBootModules = append(nonUpdatableBootModules, m)
			nonUpdatableBootLocations = append(nonUpdatableBootLocations,
				filepath.Join("/apex/com.android.runtime/javalib", m+".jar"))
		}

		for _, m := range frameworkModules {
			nonUpdatableBootModules = append(nonUpdatableBootModules, m)
			nonUpdatableBootLocations = append(nonUpdatableBootLocations,
				filepath.Join("/system/framework", m+".jar"))
		}

		// The path to bootclasspath dex files needs to be known at module GenerateAndroidBuildAction time, before
		// the bootclasspath modules have been compiled.  Set up known paths for them, the singleton rules will copy
		// them there.
		// TODO: use module dependencies instead
		var nonUpdatableBootDexPaths android.WritablePaths
		for _, m := range nonUpdatableBootModules {
			nonUpdatableBootDexPaths = append(nonUpdatableBootDexPaths,
				android.PathForOutput(ctx, ctx.Config().DeviceName(), "dex_bootjars_input", m+".jar"))
		}

		dir := android.PathForOutput(ctx, ctx.Config().DeviceName(), "dex_bootjars")
		symbolsDir := android.PathForOutput(ctx, ctx.Config().DeviceName(), "dex_bootjars_unstripped")
		images := make(map[android.ArchType]android.OutputPath)

		for _, target := range ctx.Config().Targets[android.Android] {
			images[target.Arch.ArchType] = dir.Join(ctx,
				"system/framework", target.Arch.ArchType.String()).Join(ctx, "boot.art")
		}

		return bootImageConfig{
			name:         "boot",
			modules:      nonUpdatableBootModules,
			dexLocations: nonUpdatableBootLocations,
			dexPaths:     nonUpdatableBootDexPaths,
			dir:          dir,
			symbolsDir:   symbolsDir,
			images:       images,
		}
	}).(bootImageConfig)
}

var defaultBootImageConfigKey = android.NewOnceKey("defaultBootImageConfig")

func apexBootImageConfig(ctx android.PathContext) bootImageConfig {
	return ctx.Config().Once(apexBootImageConfigKey, func() interface{} {
		global := dexpreoptGlobalConfig(ctx)

		runtimeModules := global.RuntimeApexJars
		nonFrameworkModules := concat(runtimeModules, global.ProductUpdatableBootModules)
		frameworkModules := android.RemoveListFromList(global.BootJars, nonFrameworkModules)
		imageModules := concat(runtimeModules, frameworkModules)

		var bootLocations []string

		for _, m := range runtimeModules {
			bootLocations = append(bootLocations,
				filepath.Join("/apex/com.android.runtime/javalib", m+".jar"))
		}

		for _, m := range frameworkModules {
			bootLocations = append(bootLocations,
				filepath.Join("/system/framework", m+".jar"))
		}

		// The path to bootclasspath dex files needs to be known at module GenerateAndroidBuildAction time, before
		// the bootclasspath modules have been compiled.  Set up known paths for them, the singleton rules will copy
		// them there.
		// TODO: use module dependencies instead
		var bootDexPaths android.WritablePaths
		for _, m := range imageModules {
			bootDexPaths = append(bootDexPaths,
				android.PathForOutput(ctx, ctx.Config().DeviceName(), "dex_apexjars_input", m+".jar"))
		}

		dir := android.PathForOutput(ctx, ctx.Config().DeviceName(), "dex_apexjars")
		symbolsDir := android.PathForOutput(ctx, ctx.Config().DeviceName(), "dex_apexjars_unstripped")
		images := make(map[android.ArchType]android.OutputPath)

		for _, target := range ctx.Config().Targets[android.Android] {
			images[target.Arch.ArchType] = dir.Join(ctx,
				"system/framework", target.Arch.ArchType.String(), "apex.art")
		}

		return bootImageConfig{
			name:         "apex",
			modules:      imageModules,
			dexLocations: bootLocations,
			dexPaths:     bootDexPaths,
			dir:          dir,
			symbolsDir:   symbolsDir,
			images:       images,
		}
	}).(bootImageConfig)
}

var apexBootImageConfigKey = android.NewOnceKey("apexBootImageConfig")

func defaultBootclasspath(ctx android.PathContext) []string {
	return ctx.Config().OnceStringSlice(defaultBootclasspathKey, func() []string {
		global := dexpreoptGlobalConfig(ctx)
		image := defaultBootImageConfig(ctx)
		bootclasspath := append(copyOf(image.dexLocations), global.ProductUpdatableBootLocations...)
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
	ctx.Strict("PRODUCT_DEX2OAT_BOOTCLASSPATH", strings.Join(defaultBootImageConfig(ctx).dexLocations, ":"))
	ctx.Strict("PRODUCT_SYSTEM_SERVER_CLASSPATH", strings.Join(systemServerClasspath(ctx), ":"))

	ctx.Strict("DEXPREOPT_BOOT_JARS_MODULES", strings.Join(defaultBootImageConfig(ctx).modules, ":"))
}
