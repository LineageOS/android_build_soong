// Copyright 2020 Google Inc. All rights reserved.
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

// This file contains the module implementation for android_app_set.

import (
	"strconv"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func init() {
	RegisterAppSetBuildComponents(android.InitRegistrationContext)
}

func RegisterAppSetBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("android_app_set", AndroidAppSetFactory)
}

type AndroidAppSetProperties struct {
	// APK Set path
	Set *string

	// Specifies that this app should be installed to the priv-app directory,
	// where the system will grant it additional privileges not available to
	// normal apps.
	Privileged *bool

	// APKs in this set use prerelease SDK version
	Prerelease *bool

	// Names of modules to be overridden. Listed modules can only be other apps
	//	(in Make or Soong).
	Overrides []string
}

type AndroidAppSet struct {
	android.ModuleBase
	android.DefaultableModuleBase
	prebuilt android.Prebuilt

	properties    AndroidAppSetProperties
	packedOutput  android.WritablePath
	primaryOutput android.WritablePath
	apkcertsFile  android.ModuleOutPath
}

func (as *AndroidAppSet) Name() string {
	return as.prebuilt.Name(as.ModuleBase.Name())
}

func (as *AndroidAppSet) IsInstallable() bool {
	return true
}

func (as *AndroidAppSet) Prebuilt() *android.Prebuilt {
	return &as.prebuilt
}

func (as *AndroidAppSet) Privileged() bool {
	return Bool(as.properties.Privileged)
}

func (as *AndroidAppSet) OutputFile() android.Path {
	return as.primaryOutput
}

func (as *AndroidAppSet) PackedAdditionalOutputs() android.Path {
	return as.packedOutput
}

func (as *AndroidAppSet) APKCertsFile() android.Path {
	return as.apkcertsFile
}

var TargetCpuAbi = map[string]string{
	"arm":    "ARMEABI_V7A",
	"arm64":  "ARM64_V8A",
	"x86":    "X86",
	"x86_64": "X86_64",
}

func SupportedAbis(ctx android.ModuleContext, excludeNativeBridgeAbis bool) []string {
	abiName := func(targetIdx int, deviceArch string) string {
		if abi, found := TargetCpuAbi[deviceArch]; found {
			return abi
		}
		ctx.ModuleErrorf("Target %d has invalid Arch: %s", targetIdx, deviceArch)
		return "BAD_ABI"
	}

	var result []string
	for i, target := range ctx.Config().Targets[android.Android] {
		if target.NativeBridge == android.NativeBridgeEnabled && excludeNativeBridgeAbis {
			continue
		}
		result = append(result, abiName(i, target.Arch.ArchType.String()))
	}
	return result
}

func (as *AndroidAppSet) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	as.packedOutput = android.PathForModuleOut(ctx, ctx.ModuleName()+".zip")
	as.primaryOutput = android.PathForModuleOut(ctx, as.BaseModuleName()+".apk")
	as.apkcertsFile = android.PathForModuleOut(ctx, "apkcerts.txt")
	// We are assuming here that the install file in the APK
	// set has `.apk` suffix. If it doesn't the build will fail.
	// APK sets containing APEX files are handled elsewhere.
	screenDensities := "all"
	if dpis := ctx.Config().ProductAAPTPrebuiltDPI(); len(dpis) > 0 {
		screenDensities = strings.ToUpper(strings.Join(dpis, ","))
	}
	// TODO(asmundak): handle locales.
	// TODO(asmundak): do we support device features
	ctx.Build(pctx,
		android.BuildParams{
			Rule:            extractMatchingApks,
			Description:     "Extract APKs from APK set",
			Output:          as.primaryOutput,
			ImplicitOutputs: android.WritablePaths{as.packedOutput, as.apkcertsFile},
			Inputs:          android.Paths{as.prebuilt.SingleSourcePath(ctx)},
			Args: map[string]string{
				"abis":              strings.Join(SupportedAbis(ctx, false), ","),
				"allow-prereleased": strconv.FormatBool(proptools.Bool(as.properties.Prerelease)),
				"screen-densities":  screenDensities,
				"sdk-version":       ctx.Config().PlatformSdkVersion().String(),
				"stem":              as.BaseModuleName(),
				"apkcerts":          as.apkcertsFile.String(),
				"partition":         as.PartitionTag(ctx.DeviceConfig()),
				"zip":               as.packedOutput.String(),
			},
		})

	var installDir android.InstallPath
	if as.Privileged() {
		installDir = android.PathForModuleInstall(ctx, "priv-app", as.BaseModuleName())
	} else {
		installDir = android.PathForModuleInstall(ctx, "app", as.BaseModuleName())
	}
	ctx.InstallFileWithExtraFilesZip(installDir, as.BaseModuleName()+".apk", as.primaryOutput, as.packedOutput)
}

func (as *AndroidAppSet) InstallBypassMake() bool { return true }

// android_app_set extracts a set of APKs based on the target device
// configuration and installs this set as "split APKs".
// The extracted set always contains an APK whose name is
// _module_name_.apk and every split APK matching target device.
// The extraction of the density-specific splits depends on
// PRODUCT_AAPT_PREBUILT_DPI variable. If present (its value should
// be a list density names: LDPI, MDPI, HDPI, etc.), only listed
// splits will be extracted. Otherwise all density-specific splits
// will be extracted.
func AndroidAppSetFactory() android.Module {
	module := &AndroidAppSet{}
	module.AddProperties(&module.properties)
	InitJavaModule(module, android.DeviceSupported)
	android.InitSingleSourcePrebuiltModule(module, &module.properties, "Set")
	return module
}
