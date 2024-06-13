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
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/dexpreopt"
)

type DexpreopterInterface interface {
	// True if the java module is to be dexed and installed on devices.
	// Structs that embed dexpreopter must implement this.
	IsInstallable() bool

	// True if dexpreopt is disabled for the java module.
	dexpreoptDisabled(ctx android.BaseModuleContext, libraryName string) bool

	// If the java module is to be installed into an APEX, this list contains information about the
	// dexpreopt outputs to be installed on devices. Note that these dexpreopt outputs are installed
	// outside of the APEX.
	DexpreoptBuiltInstalledForApex() []dexpreopterInstall

	// The Make entries to install the dexpreopt outputs. Derived from
	// `DexpreoptBuiltInstalledForApex`.
	AndroidMkEntriesForApex() []android.AndroidMkEntries

	// See `dexpreopter.outputProfilePathOnHost`.
	OutputProfilePathOnHost() android.Path
}

type dexpreopterInstall struct {
	// A unique name to distinguish an output from others for the same java library module. Usually in
	// the form of `<arch>-<encoded-path>.odex/vdex/art`.
	name string

	// The name of the input java module.
	moduleName string

	// The path to the dexpreopt output on host.
	outputPathOnHost android.Path

	// The directory on the device for the output to install to.
	installDirOnDevice android.InstallPath

	// The basename (the last segment of the path) for the output to install as.
	installFileOnDevice string
}

// The full module name of the output in the makefile.
func (install *dexpreopterInstall) FullModuleName() string {
	return install.moduleName + install.SubModuleName()
}

// The sub-module name of the output in the makefile (the name excluding the java module name).
func (install *dexpreopterInstall) SubModuleName() string {
	return "-dexpreopt-" + install.name
}

// Returns Make entries for installing the file.
//
// This function uses a value receiver rather than a pointer receiver to ensure that the object is
// safe to use in `android.AndroidMkExtraEntriesFunc`.
func (install dexpreopterInstall) ToMakeEntries() android.AndroidMkEntries {
	return android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(install.outputPathOnHost),
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE", install.FullModuleName())
				entries.SetString("LOCAL_MODULE_PATH", install.installDirOnDevice.String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", install.installFileOnDevice)
				entries.SetString("LOCAL_NOT_AVAILABLE_FOR_PLATFORM", "false")
				// Unset LOCAL_SOONG_INSTALLED_MODULE so that this does not default to the primary .apex file
				// Without this, installation of the dexpreopt artifacts get skipped
				entries.SetString("LOCAL_SOONG_INSTALLED_MODULE", "")
			},
		},
	}
}

func (install dexpreopterInstall) PackageFile(ctx android.ModuleContext) android.PackagingSpec {
	return ctx.PackageFile(install.installDirOnDevice, install.installFileOnDevice, install.outputPathOnHost)
}

type Dexpreopter struct {
	dexpreopter
}

type dexpreopter struct {
	dexpreoptProperties       DexpreoptProperties
	importDexpreoptProperties ImportDexpreoptProperties

	// If true, the dexpreopt rules will not be generated
	// Unlike Dex_preopt.Enabled which is user-facing,
	// shouldDisableDexpreopt is a mutated propery.
	shouldDisableDexpreopt bool

	installPath         android.InstallPath
	uncompressedDex     bool
	isSDKLibrary        bool
	isApp               bool
	isTest              bool
	isPresignedPrebuilt bool
	preventInstall      bool

	manifestFile        android.Path
	statusFile          android.WritablePath
	enforceUsesLibs     bool
	classLoaderContexts dexpreopt.ClassLoaderContextMap

	// See the `dexpreopt` function for details.
	builtInstalled        string
	builtInstalledForApex []dexpreopterInstall

	// The config is used for two purposes:
	// - Passing dexpreopt information about libraries from Soong to Make. This is needed when
	//   a <uses-library> is defined in Android.bp, but used in Android.mk (see dex_preopt_config_merger.py).
	//   Note that dexpreopt.config might be needed even if dexpreopt is disabled for the library itself.
	// - Dexpreopt post-processing (using dexpreopt artifacts from a prebuilt system image to incrementally
	//   dexpreopt another partition).
	configPath android.WritablePath

	// The path to the profile on host that dexpreopter generates. This is used as the input for
	// dex2oat.
	outputProfilePathOnHost android.Path

	// The path to the profile that dexpreopter accepts. It must be in the binary format. If this is
	// set, it overrides the profile settings in `dexpreoptProperties`.
	inputProfilePathOnHost android.Path

	// The path to the profile that matches the dex optimized by r8/d8. It is in text format. If this is
	// set, it will be converted to a binary profile which will be subsequently used for dexpreopt.
	rewrittenProfile android.Path
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

		// If set to true, r8/d8 will use `profile` as input to generate a new profile that matches
		// the optimized dex.
		// The new profile will be subsequently used as the profile to dexpreopt the dex file.
		Enable_profile_rewriting *bool
	}

	Dex_preopt_result struct {
		// True if profile-guided optimization is actually enabled.
		Profile_guided bool
	} `blueprint:"mutated"`
}

type ImportDexpreoptProperties struct {
	Dex_preopt struct {
		// If true, use the profile in the prebuilt APEX to guide optimization. Defaults to false.
		Profile_guided *bool
	}
}

func init() {
	dexpreopt.DexpreoptRunningInSoong = true
}

func isApexVariant(ctx android.BaseModuleContext) bool {
	apexInfo, _ := android.ModuleProvider(ctx, android.ApexInfoProvider)
	return !apexInfo.IsForPlatform()
}

func forPrebuiltApex(ctx android.BaseModuleContext) bool {
	apexInfo, _ := android.ModuleProvider(ctx, android.ApexInfoProvider)
	return apexInfo.ForPrebuiltApex
}

// For apex variant of modules, this returns true on the source variant if the prebuilt apex
// has been selected using apex_contributions.
// The prebuilt apex will be responsible for generating the dexpreopt rules of the deapexed java lib.
func disableSourceApexVariant(ctx android.BaseModuleContext) bool {
	if !isApexVariant(ctx) {
		return false // platform variant
	}
	apexInfo, _ := android.ModuleProvider(ctx, android.ApexInfoProvider)
	psi := android.PrebuiltSelectionInfoMap{}
	ctx.VisitDirectDeps(func(am android.Module) {
		if prebuiltSelectionInfo, ok := android.OtherModuleProvider(ctx, am, android.PrebuiltSelectionInfoProvider); ok {
			psi = prebuiltSelectionInfo
		}
	})
	// Find the apex variant for this module
	_, apexVariantsWithoutTestApexes, _ := android.ListSetDifference(apexInfo.InApexVariants, apexInfo.TestApexes)
	disableSource := false
	// find the selected apexes
	for _, apexVariant := range apexVariantsWithoutTestApexes {
		for _, selected := range psi.GetSelectedModulesForApiDomain(apexVariant) {
			// If the apex_contribution for this api domain contains a prebuilt apex, disable the source variant
			if strings.HasPrefix(selected, "prebuilt_com.google.android") {
				disableSource = true
			}
		}
	}
	return disableSource
}

// Returns whether dexpreopt is applicable to the module.
// When it returns true, neither profile nor dexpreopt artifacts will be generated.
func (d *dexpreopter) dexpreoptDisabled(ctx android.BaseModuleContext, libName string) bool {
	if !ctx.Device() {
		return true
	}

	if d.isTest {
		return true
	}

	if !BoolDefault(d.dexpreoptProperties.Dex_preopt.Enabled, true) {
		return true
	}

	if d.shouldDisableDexpreopt {
		return true
	}

	// If the module is from a prebuilt APEX, it shouldn't be installable, but it can still be
	// dexpreopted.
	if !ctx.Module().(DexpreopterInterface).IsInstallable() && !forPrebuiltApex(ctx) {
		return true
	}

	if !android.IsModulePreferred(ctx.Module()) {
		return true
	}

	if _, isApex := android.ModuleProvider(ctx, android.ApexBundleInfoProvider); isApex {
		// dexpreopt rules for system server jars can be generated in the ModuleCtx of prebuilt apexes
		return false
	}

	global := dexpreopt.GetGlobalConfig(ctx)

	// Use the libName argument to determine if the library being dexpreopt'd is a system server jar
	// ctx.ModuleName() is not safe. In case of prebuilt apexes, the dexpreopt rules of system server jars
	// are created in the ctx object of the top-level prebuilt apex.
	isApexSystemServerJar := global.AllApexSystemServerJars(ctx).ContainsJar(libName)

	if _, isApex := android.ModuleProvider(ctx, android.ApexBundleInfoProvider); isApex || isApexVariant(ctx) {
		// dexpreopt rules for system server jars can be generated in the ModuleCtx of prebuilt apexes
		if !isApexSystemServerJar {
			return true
		}
		ai, _ := android.ModuleProvider(ctx, android.ApexInfoProvider)
		allApexInfos := []android.ApexInfo{}
		if allApexInfosProvider, ok := android.ModuleProvider(ctx, android.AllApexInfoProvider); ok {
			allApexInfos = allApexInfosProvider.ApexInfos
		}
		if len(allApexInfos) > 0 && !ai.MinSdkVersion.EqualTo(allApexInfos[0].MinSdkVersion) {
			// Apex system server jars are dexpreopted and installed on to the system image.
			// Since we can have BigAndroid and Go variants of system server jar providing apexes,
			// and these two variants can have different min_sdk_versions, hide one of the apex variants
			// from make to prevent collisions.
			//
			// Unlike cc, min_sdk_version does not have an effect on the build actions of java libraries.
			ctx.Module().MakeUninstallable()
		}
	} else {
		// Don't preopt the platform variant of an APEX system server jar to avoid conflicts.
		if isApexSystemServerJar {
			return true
		}
	}

	// TODO: contains no java code

	return false
}

func dexpreoptToolDepsMutator(ctx android.BottomUpMutatorContext) {
	if _, isApex := android.ModuleProvider(ctx, android.ApexBundleInfoProvider); isApex && dexpreopt.IsDex2oatNeeded(ctx) {
		// prebuilt apexes can genererate rules to dexpreopt deapexed jars
		// Add a dex2oat dep aggressively on _every_ apex module
		dexpreopt.RegisterToolDeps(ctx)
		return
	}
	if d, ok := ctx.Module().(DexpreopterInterface); !ok || d.dexpreoptDisabled(ctx, android.RemoveOptionalPrebuiltPrefix(ctx.ModuleName())) || !dexpreopt.IsDex2oatNeeded(ctx) {
		return
	}
	dexpreopt.RegisterToolDeps(ctx)
}

// Returns the install path of the dex jar of a module.
//
// Do not rely on `ApexInfo.ApexVariationName` because it can be something like "apex1000", rather
// than the `name` in the path `/apex/<name>` as suggested in its comment.
//
// This function is on a best-effort basis. It cannot handle the case where an APEX jar is not a
// system server jar, which is fine because we currently only preopt system server jars for APEXes.
func (d *dexpreopter) getInstallPath(
	ctx android.ModuleContext, libName string, defaultInstallPath android.InstallPath) android.InstallPath {
	global := dexpreopt.GetGlobalConfig(ctx)
	if global.AllApexSystemServerJars(ctx).ContainsJar(libName) {
		dexLocation := dexpreopt.GetSystemServerDexLocation(ctx, global, libName)
		return android.PathForModuleInPartitionInstall(ctx, "", strings.TrimPrefix(dexLocation, "/"))
	}
	if !d.dexpreoptDisabled(ctx, libName) && isApexVariant(ctx) &&
		filepath.Base(defaultInstallPath.PartitionDir()) != "apex" {
		ctx.ModuleErrorf("unable to get the install path of the dex jar for dexpreopt")
	}
	return defaultInstallPath
}

// DexpreoptPrebuiltApexSystemServerJars generates the dexpreopt artifacts from a jar file that has been deapexed from a prebuilt apex
func (d *Dexpreopter) DexpreoptPrebuiltApexSystemServerJars(ctx android.ModuleContext, libraryName string, di *android.DeapexerInfo) {
	// A single prebuilt apex can have multiple apex system jars
	// initialize the output path for this dex jar
	dc := dexpreopt.GetGlobalConfig(ctx)
	d.installPath = android.PathForModuleInPartitionInstall(ctx, "", strings.TrimPrefix(dexpreopt.GetSystemServerDexLocation(ctx, dc, libraryName), "/"))
	// generate the rules for creating the .odex and .vdex files for this system server jar
	dexJarFile := di.PrebuiltExportPath(ApexRootRelativePathToJavaLib(libraryName))

	d.inputProfilePathOnHost = nil // reset: TODO(spandandas): Make dexpreopter stateless
	if android.InList(libraryName, di.GetDexpreoptProfileGuidedExportedModuleNames()) {
		// Set the profile path to guide optimization
		prof := di.PrebuiltExportPath(ApexRootRelativePathToJavaLib(libraryName) + ".prof")
		if prof == nil {
			ctx.ModuleErrorf("Could not find a .prof file in this prebuilt apex")
		}
		d.inputProfilePathOnHost = prof
	}

	d.dexpreopt(ctx, libraryName, dexJarFile)
}

func (d *dexpreopter) dexpreopt(ctx android.ModuleContext, libName string, dexJarFile android.WritablePath) {
	global := dexpreopt.GetGlobalConfig(ctx)

	// TODO(b/148690468): The check on d.installPath is to bail out in cases where
	// the dexpreopter struct hasn't been fully initialized before we're called,
	// e.g. in aar.go. This keeps the behaviour that dexpreopting is effectively
	// disabled, even if installable is true.
	if d.installPath.Base() == "." {
		return
	}

	dexLocation := android.InstallPathToOnDevicePath(ctx, d.installPath)

	providesUsesLib := libName
	if ulib, ok := ctx.Module().(ProvidesUsesLib); ok {
		name := ulib.ProvidesUsesLib()
		if name != nil {
			providesUsesLib = *name
		}
	}

	// If it is test, make config files regardless of its dexpreopt setting.
	// The config files are required for apps defined in make which depend on the lib.
	if d.isTest && d.dexpreoptDisabled(ctx, libName) {
		return
	}

	isSystemServerJar := global.AllSystemServerJars(ctx).ContainsJar(libName)

	bootImage := defaultBootImageConfig(ctx)
	// When `global.PreoptWithUpdatableBcp` is true, `bcpForDexpreopt` below includes the mainline
	// boot jars into bootclasspath, so we should include the mainline boot image as well because it's
	// generated from those jars.
	if global.PreoptWithUpdatableBcp {
		bootImage = mainlineBootImageConfig(ctx)
	}
	dexFiles, dexLocations := bcpForDexpreopt(ctx, global.PreoptWithUpdatableBcp)

	targets := ctx.MultiTargets()
	if len(targets) == 0 {
		// assume this is a java library, dexpreopt for all arches for now
		for _, target := range ctx.Config().Targets[android.Android] {
			if target.NativeBridge == android.NativeBridgeDisabled {
				targets = append(targets, target)
			}
		}
		if isSystemServerJar && libName != "com.android.location.provider" {
			// If the module is a system server jar, only preopt for the primary arch because the jar can
			// only be loaded by system server. "com.android.location.provider" is a special case because
			// it's also used by apps as a shared library.
			targets = targets[:1]
		}
	}

	var archs []android.ArchType
	var images android.Paths
	var imagesDeps []android.OutputPaths
	for _, target := range targets {
		archs = append(archs, target.Arch.ArchType)
		variant := bootImage.getVariant(target)
		images = append(images, variant.imagePathOnHost)
		imagesDeps = append(imagesDeps, variant.imagesDeps)
	}
	// The image locations for all Android variants are identical.
	hostImageLocations, deviceImageLocations := bootImage.getAnyAndroidVariant().imageLocations()

	var profileClassListing android.OptionalPath
	var profileBootListing android.OptionalPath
	profileIsTextListing := false

	if d.inputProfilePathOnHost != nil {
		profileClassListing = android.OptionalPathForPath(d.inputProfilePathOnHost)
	} else if BoolDefault(d.dexpreoptProperties.Dex_preopt.Profile_guided, true) && !forPrebuiltApex(ctx) {
		// If enable_profile_rewriting is set, use the rewritten profile instead of the checked-in profile
		if d.EnableProfileRewriting() {
			profileClassListing = android.OptionalPathForPath(d.GetRewrittenProfile())
			profileIsTextListing = true
		} else if profile := d.GetProfile(); profile != "" {
			// If dex_preopt.profile_guided is not set, default it based on the existence of the
			// dexprepot.profile option or the profile class listing.
			profileClassListing = android.OptionalPathForPath(
				android.PathForModuleSrc(ctx, profile))
			profileBootListing = android.ExistentPathForSource(ctx,
				ctx.ModuleDir(), profile+"-boot")
			profileIsTextListing = true
		} else if global.ProfileDir != "" {
			profileClassListing = android.ExistentPathForSource(ctx,
				global.ProfileDir, libName+".prof")
		}
	}

	d.dexpreoptProperties.Dex_preopt_result.Profile_guided = profileClassListing.Valid()

	// A single apex can have multiple system server jars
	// Use the dexJar to create a unique scope for each
	dexJarStem := strings.TrimSuffix(dexJarFile.Base(), dexJarFile.Ext())

	// Full dexpreopt config, used to create dexpreopt build rules.
	dexpreoptConfig := &dexpreopt.ModuleConfig{
		Name:            libName,
		DexLocation:     dexLocation,
		BuildPath:       android.PathForModuleOut(ctx, "dexpreopt", dexJarStem, libName+".jar").OutputPath,
		DexPath:         dexJarFile,
		ManifestPath:    android.OptionalPathForPath(d.manifestFile),
		UncompressedDex: d.uncompressedDex,
		HasApkLibraries: false,
		PreoptFlags:     nil,

		ProfileClassListing:  profileClassListing,
		ProfileIsTextListing: profileIsTextListing,
		ProfileBootListing:   profileBootListing,

		EnforceUsesLibrariesStatusFile: dexpreopt.UsesLibrariesStatusFile(ctx),
		EnforceUsesLibraries:           d.enforceUsesLibs,
		ProvidesUsesLibrary:            providesUsesLib,
		ClassLoaderContexts:            d.classLoaderContexts,

		Archs:                           archs,
		DexPreoptImagesDeps:             imagesDeps,
		DexPreoptImageLocationsOnHost:   hostImageLocations,
		DexPreoptImageLocationsOnDevice: deviceImageLocations,

		PreoptBootClassPathDexFiles:     dexFiles.Paths(),
		PreoptBootClassPathDexLocations: dexLocations,

		NoCreateAppImage:    !BoolDefault(d.dexpreoptProperties.Dex_preopt.App_image, true),
		ForceCreateAppImage: BoolDefault(d.dexpreoptProperties.Dex_preopt.App_image, false),

		PresignedPrebuilt: d.isPresignedPrebuilt,
	}

	d.configPath = android.PathForModuleOut(ctx, "dexpreopt", dexJarStem, "dexpreopt.config")
	dexpreopt.WriteModuleConfig(ctx, dexpreoptConfig, d.configPath)

	if d.dexpreoptDisabled(ctx, libName) {
		return
	}

	globalSoong := dexpreopt.GetGlobalSoongConfig(ctx)

	// The root "product_packages.txt" is generated by `build/make/core/Makefile`. It contains a list
	// of all packages that are installed on the device. We use `grep` to filter the list by the app's
	// dependencies to create a per-app list, and use `rsync --checksum` to prevent the file's mtime
	// from being changed if the contents don't change. This avoids unnecessary dexpreopt reruns.
	productPackages := android.PathForModuleInPartitionInstall(ctx, "", "product_packages.txt")
	appProductPackages := android.PathForModuleOut(ctx, "dexpreopt", dexJarStem, "product_packages.txt")
	appProductPackagesStaging := appProductPackages.ReplaceExtension(ctx, "txt.tmp")
	clcNames, _ := dexpreopt.ComputeClassLoaderContextDependencies(dexpreoptConfig.ClassLoaderContexts)
	sort.Strings(clcNames) // The order needs to be deterministic.
	productPackagesRule := android.NewRuleBuilder(pctx, ctx)
	if len(clcNames) > 0 {
		productPackagesRule.Command().
			Text("grep -F -x").
			FlagForEachArg("-e ", clcNames).
			Input(productPackages).
			FlagWithOutput("> ", appProductPackagesStaging).
			Text("|| true")
	} else {
		productPackagesRule.Command().
			Text("rm -f").Output(appProductPackagesStaging).
			Text("&&").
			Text("touch").Output(appProductPackagesStaging)
	}
	productPackagesRule.Command().
		Text("rsync --checksum").
		Input(appProductPackagesStaging).
		Output(appProductPackages)
	productPackagesRule.Restat().Build("product_packages."+dexJarStem, "dexpreopt product_packages")

	// Prebuilts are active, do not copy the dexpreopt'd source javalib to out/soong/system_server_dexjars
	// The javalib from the deapexed prebuilt will be copied to this location.
	// TODO (b/331665856): Implement a principled solution for this.
	copyApexSystemServerJarDex := !disableSourceApexVariant(ctx) && !ctx.Module().IsHideFromMake()
	dexpreoptRule, err := dexpreopt.GenerateDexpreoptRule(
		ctx, globalSoong, global, dexpreoptConfig, appProductPackages, copyApexSystemServerJarDex)
	if err != nil {
		ctx.ModuleErrorf("error generating dexpreopt rule: %s", err.Error())
		return
	}

	dexpreoptRule.Build("dexpreopt"+"."+dexJarStem, "dexpreopt")

	// The current ctx might be of a deapexer module created by a prebuilt apex
	// Use the path of the dex file to determine the library name
	isApexSystemServerJar := global.AllApexSystemServerJars(ctx).ContainsJar(dexJarStem)

	dexpreoptPartition := d.installPath.Partition()
	// dexpreoptPartition is set to empty for dexpreopts of system APEX and system_other.
	// In case of system APEX, however, we can set it to "system" manually.
	// TODO(b/346662300): Let dexpreopter generate the installPath for dexpreopt files instead of
	// using the dex location to generate the installPath.
	if isApexSystemServerJar {
		dexpreoptPartition = "system"
	}
	for _, install := range dexpreoptRule.Installs() {
		// Remove the "/" prefix because the path should be relative to $ANDROID_PRODUCT_OUT.
		installDir := strings.TrimPrefix(filepath.Dir(install.To), "/")
		partition := dexpreoptPartition
		if strings.HasPrefix(installDir, partition+"/") {
			installDir = strings.TrimPrefix(installDir, partition+"/")
		} else {
			// If the partition for the installDir is different from the install partition, set the
			// partition empty to install the dexpreopt files to the desired partition.
			// TODO(b/346439786): Define and use the dexpreopt module type to avoid this mismatch.
			partition = ""
		}
		installBase := filepath.Base(install.To)
		arch := filepath.Base(installDir)
		installPath := android.PathForModuleInPartitionInstall(ctx, partition, installDir)
		isProfile := strings.HasSuffix(installBase, ".prof")

		if isProfile {
			d.outputProfilePathOnHost = install.From
		}

		if isApexSystemServerJar {
			// Profiles are handled separately because they are installed into the APEX.
			if !isProfile {
				// APEX variants of java libraries are hidden from Make, so their dexpreopt
				// outputs need special handling. Currently, for APEX variants of java
				// libraries, only those in the system server classpath are handled here.
				// Preopting of boot classpath jars in the ART APEX are handled in
				// java/dexpreopt_bootjars.go, and other APEX jars are not preopted.
				// The installs will be handled by Make as sub-modules of the java library.
				d.builtInstalledForApex = append(d.builtInstalledForApex, dexpreopterInstall{
					name:                arch + "-" + installBase,
					moduleName:          libName,
					outputPathOnHost:    install.From,
					installDirOnDevice:  installPath,
					installFileOnDevice: installBase,
				})
			}
		} else if !d.preventInstall {
			ctx.InstallFile(installPath, installBase, install.From)
		}
	}

	if !isApexSystemServerJar {
		d.builtInstalled = dexpreoptRule.Installs().String()
	}
}

func getModuleInstallPathInfo(ctx android.ModuleContext, fullInstallPath string) (android.InstallPath, string, string) {
	installPath := android.PathForModuleInstall(ctx)
	installDir, installBase := filepath.Split(strings.TrimPrefix(fullInstallPath, "/"))

	if !strings.HasPrefix(installDir, installPath.Partition()+"/") {
		// Return empty filename if the install partition is not for the target image.
		return installPath, "", ""
	}
	relDir, err := filepath.Rel(installPath.Partition(), installDir)
	if err != nil {
		panic(err)
	}
	return installPath, relDir, installBase
}

// RuleBuilder.Install() adds output-to-install copy pairs to a list for Make. To share this
// information with PackagingSpec in soong, call PackageFile for them.
// The install path and the target install partition of the module must be the same.
func packageFile(ctx android.ModuleContext, install android.RuleBuilderInstall) {
	installPath, relDir, name := getModuleInstallPathInfo(ctx, install.To)
	// Empty name means the install partition is not for the target image.
	// For the system image, files for "apex" and "system_other" are skipped here.
	// The skipped "apex" files are for testing only, for example,
	// "/apex/art_boot_images/javalib/x86/boot.vdex".
	// TODO(b/320196894): Files for "system_other" are skipped because soong creates the system
	// image only for now.
	if name != "" {
		ctx.PackageFile(installPath.Join(ctx, relDir), name, install.From)
	}
}

func (d *dexpreopter) DexpreoptBuiltInstalledForApex() []dexpreopterInstall {
	return d.builtInstalledForApex
}

func (d *dexpreopter) AndroidMkEntriesForApex() []android.AndroidMkEntries {
	var entries []android.AndroidMkEntries
	for _, install := range d.builtInstalledForApex {
		entries = append(entries, install.ToMakeEntries())
	}
	return entries
}

func (d *dexpreopter) OutputProfilePathOnHost() android.Path {
	return d.outputProfilePathOnHost
}

func (d *dexpreopter) disableDexpreopt() {
	d.shouldDisableDexpreopt = true
}

func (d *dexpreopter) EnableProfileRewriting() bool {
	return proptools.Bool(d.dexpreoptProperties.Dex_preopt.Enable_profile_rewriting)
}

func (d *dexpreopter) GetProfile() string {
	return proptools.String(d.dexpreoptProperties.Dex_preopt.Profile)
}

func (d *dexpreopter) GetProfileGuided() bool {
	return proptools.Bool(d.dexpreoptProperties.Dex_preopt.Profile_guided)
}

func (d *dexpreopter) GetRewrittenProfile() android.Path {
	return d.rewrittenProfile
}

func (d *dexpreopter) SetRewrittenProfile(p android.Path) {
	d.rewrittenProfile = p
}
