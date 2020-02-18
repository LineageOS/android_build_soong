// Copyright 2018 Google Inc. All rights reserved.
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
)

type dexpreopterInterface interface {
	IsInstallable() bool // Structs that embed dexpreopter must implement this.
	dexpreoptDisabled(ctx android.BaseModuleContext) bool
}

type dexpreopter struct {
	dexpreoptProperties DexpreoptProperties

	installPath         android.InstallPath
	uncompressedDex     bool
	isSDKLibrary        bool
	isTest              bool
	isPresignedPrebuilt bool

	manifestFile     android.Path
	usesLibs         []string
	optionalUsesLibs []string
	enforceUsesLibs  bool
	libraryPaths     map[string]android.Path

	builtInstalled string
}

type DexpreoptProperties struct {
	Dex_preopt struct {
		// If false, prevent dexpreopting.  Defaults to true.
		Enabled *bool

		// If true, generate an app image (.art file) for this module.
		App_image *bool

		// If true, use a checked-in profile to guide optimization.  Defaults to false unless
		// a matching profile is set or a profile is found in PRODUCT_DEX_PREOPT_PROFILE_DIR
		// that matches the name of this module, in which case it is defaulted to true.
		Profile_guided *bool

		// If set, provides the path to profile relative to the Android.bp file.  If not set,
		// defaults to searching for a file that matches the name of this module in the default
		// profile location set by PRODUCT_DEX_PREOPT_PROFILE_DIR, or empty if not found.
		Profile *string `android:"path"`
	}
}

func (d *dexpreopter) dexpreoptDisabled(ctx android.BaseModuleContext) bool {
	global := dexpreopt.GetGlobalConfig(ctx)

	if global.DisablePreopt {
		return true
	}

	if inList(ctx.ModuleName(), global.DisablePreoptModules) {
		return true
	}

	if ctx.Config().UnbundledBuild() {
		return true
	}

	if d.isTest {
		return true
	}

	if !BoolDefault(d.dexpreoptProperties.Dex_preopt.Enabled, true) {
		return true
	}

	if !ctx.Module().(dexpreopterInterface).IsInstallable() {
		return true
	}

	if ctx.Host() {
		return true
	}

	// Don't preopt APEX variant module
	if am, ok := ctx.Module().(android.ApexModule); ok && !am.IsForPlatform() {
		return true
	}

	// TODO: contains no java code

	return false
}

func dexpreoptToolDepsMutator(ctx android.BottomUpMutatorContext) {
	if d, ok := ctx.Module().(dexpreopterInterface); !ok || d.dexpreoptDisabled(ctx) {
		return
	}
	dexpreopt.RegisterToolDeps(ctx)
}

func odexOnSystemOther(ctx android.ModuleContext, installPath android.InstallPath) bool {
	return dexpreopt.OdexOnSystemOtherByName(ctx.ModuleName(), android.InstallPathToOnDevicePath(ctx, installPath), dexpreopt.GetGlobalConfig(ctx))
}

func (d *dexpreopter) dexpreopt(ctx android.ModuleContext, dexJarFile android.ModuleOutPath) android.ModuleOutPath {
	// TODO(b/148690468): The check on d.installPath is to bail out in cases where
	// the dexpreopter struct hasn't been fully initialized before we're called,
	// e.g. in aar.go. This keeps the behaviour that dexpreopting is effectively
	// disabled, even if installable is true.
	if d.dexpreoptDisabled(ctx) || d.installPath.Base() == "." {
		return dexJarFile
	}

	globalSoong := dexpreopt.GetGlobalSoongConfig(ctx)
	global := dexpreopt.GetGlobalConfig(ctx)
	bootImage := defaultBootImageConfig(ctx)
	dexFiles := bootImage.dexPathsDeps.Paths()
	dexLocations := bootImage.dexLocationsDeps
	if global.UseArtImage {
		bootImage = artBootImageConfig(ctx)
	}

	targets := ctx.MultiTargets()
	if len(targets) == 0 {
		// assume this is a java library, dexpreopt for all arches for now
		for _, target := range ctx.Config().Targets[android.Android] {
			if target.NativeBridge == android.NativeBridgeDisabled {
				targets = append(targets, target)
			}
		}
		if inList(ctx.ModuleName(), global.SystemServerJars) && !d.isSDKLibrary {
			// If the module is not an SDK library and it's a system server jar, only preopt the primary arch.
			targets = targets[:1]
		}
	}

	var archs []android.ArchType
	var images android.Paths
	var imagesDeps []android.OutputPaths
	for _, target := range targets {
		archs = append(archs, target.Arch.ArchType)
		variant := bootImage.getVariant(target)
		images = append(images, variant.images)
		imagesDeps = append(imagesDeps, variant.imagesDeps)
	}

	dexLocation := android.InstallPathToOnDevicePath(ctx, d.installPath)

	var profileClassListing android.OptionalPath
	var profileBootListing android.OptionalPath
	profileIsTextListing := false
	if BoolDefault(d.dexpreoptProperties.Dex_preopt.Profile_guided, true) {
		// If dex_preopt.profile_guided is not set, default it based on the existence of the
		// dexprepot.profile option or the profile class listing.
		if String(d.dexpreoptProperties.Dex_preopt.Profile) != "" {
			profileClassListing = android.OptionalPathForPath(
				android.PathForModuleSrc(ctx, String(d.dexpreoptProperties.Dex_preopt.Profile)))
			profileBootListing = android.ExistentPathForSource(ctx,
				ctx.ModuleDir(), String(d.dexpreoptProperties.Dex_preopt.Profile)+"-boot")
			profileIsTextListing = true
		} else {
			profileClassListing = android.ExistentPathForSource(ctx,
				global.ProfileDir, ctx.ModuleName()+".prof")
		}
	}

	dexpreoptConfig := &dexpreopt.ModuleConfig{
		Name:            ctx.ModuleName(),
		DexLocation:     dexLocation,
		BuildPath:       android.PathForModuleOut(ctx, "dexpreopt", ctx.ModuleName()+".jar").OutputPath,
		DexPath:         dexJarFile,
		ManifestPath:    d.manifestFile,
		UncompressedDex: d.uncompressedDex,
		HasApkLibraries: false,
		PreoptFlags:     nil,

		ProfileClassListing:  profileClassListing,
		ProfileIsTextListing: profileIsTextListing,
		ProfileBootListing:   profileBootListing,

		EnforceUsesLibraries:         d.enforceUsesLibs,
		PresentOptionalUsesLibraries: d.optionalUsesLibs,
		UsesLibraries:                d.usesLibs,
		LibraryPaths:                 d.libraryPaths,

		Archs:                   archs,
		DexPreoptImages:         images,
		DexPreoptImagesDeps:     imagesDeps,
		DexPreoptImageLocations: bootImage.imageLocations,

		PreoptBootClassPathDexFiles:     dexFiles,
		PreoptBootClassPathDexLocations: dexLocations,

		PreoptExtractedApk: false,

		NoCreateAppImage:    !BoolDefault(d.dexpreoptProperties.Dex_preopt.App_image, true),
		ForceCreateAppImage: BoolDefault(d.dexpreoptProperties.Dex_preopt.App_image, false),

		PresignedPrebuilt: d.isPresignedPrebuilt,
	}

	dexpreoptRule, err := dexpreopt.GenerateDexpreoptRule(ctx, globalSoong, global, dexpreoptConfig)
	if err != nil {
		ctx.ModuleErrorf("error generating dexpreopt rule: %s", err.Error())
		return dexJarFile
	}

	dexpreoptRule.Build(pctx, ctx, "dexpreopt", "dexpreopt")

	d.builtInstalled = dexpreoptRule.Installs().String()

	return dexJarFile
}
