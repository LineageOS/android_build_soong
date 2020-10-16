// Copyright 2015 Google Inc. All rights reserved.
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

// This file contains the module types for compiling Android apps.

import (
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/tradefed"
)

var supportedDpis = []string{"ldpi", "mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi"}

func init() {
	RegisterAppBuildComponents(android.InitRegistrationContext)

	initAndroidAppImportVariantGroupTypes()
}

func RegisterAppBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("android_app", AndroidAppFactory)
	ctx.RegisterModuleType("android_test", AndroidTestFactory)
	ctx.RegisterModuleType("android_test_helper_app", AndroidTestHelperAppFactory)
	ctx.RegisterModuleType("android_app_certificate", AndroidAppCertificateFactory)
	ctx.RegisterModuleType("override_android_app", OverrideAndroidAppModuleFactory)
	ctx.RegisterModuleType("override_android_test", OverrideAndroidTestModuleFactory)
	ctx.RegisterModuleType("override_runtime_resource_overlay", OverrideRuntimeResourceOverlayModuleFactory)
	ctx.RegisterModuleType("android_app_import", AndroidAppImportFactory)
	ctx.RegisterModuleType("android_test_import", AndroidTestImportFactory)
	ctx.RegisterModuleType("runtime_resource_overlay", RuntimeResourceOverlayFactory)
	ctx.RegisterModuleType("android_app_set", AndroidApkSetFactory)
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

	properties   AndroidAppSetProperties
	packedOutput android.WritablePath
	masterFile   string
	apkcertsFile android.ModuleOutPath
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
	return as.packedOutput
}

func (as *AndroidAppSet) MasterFile() string {
	return as.masterFile
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

func SupportedAbis(ctx android.ModuleContext) []string {
	abiName := func(targetIdx int, deviceArch string) string {
		if abi, found := TargetCpuAbi[deviceArch]; found {
			return abi
		}
		ctx.ModuleErrorf("Target %d has invalid Arch: %s", targetIdx, deviceArch)
		return "BAD_ABI"
	}

	var result []string
	for i, target := range ctx.Config().Targets[android.Android] {
		result = append(result, abiName(i, target.Arch.ArchType.String()))
	}
	return result
}

func (as *AndroidAppSet) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	as.packedOutput = android.PathForModuleOut(ctx, ctx.ModuleName()+".zip")
	as.apkcertsFile = android.PathForModuleOut(ctx, "apkcerts.txt")
	// We are assuming here that the master file in the APK
	// set has `.apk` suffix. If it doesn't the build will fail.
	// APK sets containing APEX files are handled elsewhere.
	as.masterFile = as.BaseModuleName() + ".apk"
	screenDensities := "all"
	if dpis := ctx.Config().ProductAAPTPrebuiltDPI(); len(dpis) > 0 {
		screenDensities = strings.ToUpper(strings.Join(dpis, ","))
	}
	// TODO(asmundak): handle locales.
	// TODO(asmundak): do we support device features
	ctx.Build(pctx,
		android.BuildParams{
			Rule:           extractMatchingApks,
			Description:    "Extract APKs from APK set",
			Output:         as.packedOutput,
			ImplicitOutput: as.apkcertsFile,
			Inputs:         android.Paths{as.prebuilt.SingleSourcePath(ctx)},
			Args: map[string]string{
				"abis":              strings.Join(SupportedAbis(ctx), ","),
				"allow-prereleased": strconv.FormatBool(proptools.Bool(as.properties.Prerelease)),
				"screen-densities":  screenDensities,
				"sdk-version":       ctx.Config().PlatformSdkVersion(),
				"stem":              as.BaseModuleName(),
				"apkcerts":          as.apkcertsFile.String(),
				"partition":         as.PartitionTag(ctx.DeviceConfig()),
			},
		})
}

// android_app_set extracts a set of APKs based on the target device
// configuration and installs this set as "split APKs".
// The extracted set always contains 'master' APK whose name is
// _module_name_.apk and every split APK matching target device.
// The extraction of the density-specific splits depends on
// PRODUCT_AAPT_PREBUILT_DPI variable. If present (its value should
// be a list density names: LDPI, MDPI, HDPI, etc.), only listed
// splits will be extracted. Otherwise all density-specific splits
// will be extracted.
func AndroidApkSetFactory() android.Module {
	module := &AndroidAppSet{}
	module.AddProperties(&module.properties)
	InitJavaModule(module, android.DeviceSupported)
	android.InitSingleSourcePrebuiltModule(module, &module.properties, "Set")
	return module
}

// AndroidManifest.xml merging
// package splits

type appProperties struct {
	// Names of extra android_app_certificate modules to sign the apk with in the form ":module".
	Additional_certificates []string

	// If set, create package-export.apk, which other packages can
	// use to get PRODUCT-agnostic resource data like IDs and type definitions.
	Export_package_resources *bool

	// Specifies that this app should be installed to the priv-app directory,
	// where the system will grant it additional privileges not available to
	// normal apps.
	Privileged *bool

	// list of resource labels to generate individual resource packages
	Package_splits []string

	// Names of modules to be overridden. Listed modules can only be other binaries
	// (in Make or Soong).
	// This does not completely prevent installation of the overridden binaries, but if both
	// binaries would be installed by default (in PRODUCT_PACKAGES) the other binary will be removed
	// from PRODUCT_PACKAGES.
	Overrides []string

	// list of native libraries that will be provided in or alongside the resulting jar
	Jni_libs []string `android:"arch_variant"`

	// if true, use JNI libraries that link against platform APIs even if this module sets
	// sdk_version.
	Jni_uses_platform_apis *bool

	// if true, use JNI libraries that link against SDK APIs even if this module does not set
	// sdk_version.
	Jni_uses_sdk_apis *bool

	// STL library to use for JNI libraries.
	Stl *string `android:"arch_variant"`

	// Store native libraries uncompressed in the APK and set the android:extractNativeLibs="false" manifest
	// flag so that they are used from inside the APK at runtime.  Defaults to true for android_test modules unless
	// sdk_version or min_sdk_version is set to a version that doesn't support it (<23), defaults to true for
	// android_app modules that are embedded to APEXes, defaults to false for other module types where the native
	// libraries are generally preinstalled outside the APK.
	Use_embedded_native_libs *bool

	// Store dex files uncompressed in the APK and set the android:useEmbeddedDex="true" manifest attribute so that
	// they are used from inside the APK at runtime.
	Use_embedded_dex *bool

	// Forces native libraries to always be packaged into the APK,
	// Use_embedded_native_libs still selects whether they are stored uncompressed and aligned or compressed.
	// True for android_test* modules.
	AlwaysPackageNativeLibs bool `blueprint:"mutated"`

	// If set, find and merge all NOTICE files that this module and its dependencies have and store
	// it in the APK as an asset.
	Embed_notices *bool

	// cc.Coverage related properties
	PreventInstall    bool `blueprint:"mutated"`
	HideFromMake      bool `blueprint:"mutated"`
	IsCoverageVariant bool `blueprint:"mutated"`

	// Whether this app is considered mainline updatable or not. When set to true, this will enforce
	// additional rules to make sure an app can safely be updated. Default is false.
	// Prefer using other specific properties if build behaviour must be changed; avoid using this
	// flag for anything but neverallow rules (unless the behaviour change is invisible to owners).
	Updatable *bool
}

// android_app properties that can be overridden by override_android_app
type overridableAppProperties struct {
	// The name of a certificate in the default certificate directory, blank to use the default product certificate,
	// or an android_app_certificate module name in the form ":module".
	Certificate *string

	// Name of the signing certificate lineage file.
	Lineage *string

	// the package name of this app. The package name in the manifest file is used if one was not given.
	Package_name *string

	// the logging parent of this app.
	Logging_parent *string
}

// runtime_resource_overlay properties that can be overridden by override_runtime_resource_overlay
type OverridableRuntimeResourceOverlayProperties struct {
	// the package name of this app. The package name in the manifest file is used if one was not given.
	Package_name *string

	// the target package name of this overlay app. The target package name in the manifest file is used if one was not given.
	Target_package_name *string
}

type AndroidApp struct {
	Library
	aapt
	android.OverridableModuleBase

	usesLibrary usesLibrary

	certificate Certificate

	appProperties appProperties

	overridableAppProperties overridableAppProperties

	jniLibs                  []jniLib
	installPathForJNISymbols android.Path
	embeddedJniLibs          bool
	jniCoverageOutputs       android.Paths

	bundleFile android.Path

	// the install APK name is normally the same as the module name, but can be overridden with PRODUCT_PACKAGE_NAME_OVERRIDES.
	installApkName string

	installDir android.InstallPath

	onDeviceDir string

	additionalAaptFlags []string

	noticeOutputs android.NoticeOutputs

	overriddenManifestPackageName string

	android.ApexBundleDepsInfo
}

func (a *AndroidApp) IsInstallable() bool {
	return Bool(a.properties.Installable)
}

func (a *AndroidApp) ExportedProguardFlagFiles() android.Paths {
	return nil
}

func (a *AndroidApp) ExportedStaticPackages() android.Paths {
	return nil
}

func (a *AndroidApp) OutputFile() android.Path {
	return a.outputFile
}

func (a *AndroidApp) Certificate() Certificate {
	return a.certificate
}

func (a *AndroidApp) JniCoverageOutputs() android.Paths {
	return a.jniCoverageOutputs
}

var _ AndroidLibraryDependency = (*AndroidApp)(nil)

type Certificate struct {
	Pem, Key  android.Path
	presigned bool
}

var PresignedCertificate = Certificate{presigned: true}

func (c Certificate) AndroidMkString() string {
	if c.presigned {
		return "PRESIGNED"
	} else {
		return c.Pem.String()
	}
}

func (a *AndroidApp) DepsMutator(ctx android.BottomUpMutatorContext) {
	a.Module.deps(ctx)

	if String(a.appProperties.Stl) == "c++_shared" && !a.sdkVersion().specified() {
		ctx.PropertyErrorf("stl", "sdk_version must be set in order to use c++_shared")
	}

	sdkDep := decodeSdkDep(ctx, sdkContext(a))
	if sdkDep.hasFrameworkLibs() {
		a.aapt.deps(ctx, sdkDep)
	}

	usesSDK := a.sdkVersion().specified() && a.sdkVersion().kind != sdkCorePlatform

	if usesSDK && Bool(a.appProperties.Jni_uses_sdk_apis) {
		ctx.PropertyErrorf("jni_uses_sdk_apis",
			"can only be set for modules that do not set sdk_version")
	} else if !usesSDK && Bool(a.appProperties.Jni_uses_platform_apis) {
		ctx.PropertyErrorf("jni_uses_platform_apis",
			"can only be set for modules that set sdk_version")
	}

	tag := &jniDependencyTag{}
	for _, jniTarget := range ctx.MultiTargets() {
		variation := append(jniTarget.Variations(),
			blueprint.Variation{Mutator: "link", Variation: "shared"})

		// If the app builds against an Android SDK use the SDK variant of JNI dependencies
		// unless jni_uses_platform_apis is set.
		// Don't require the SDK variant for apps that are shipped on vendor, etc., as they already
		// have stable APIs through the VNDK.
		if (usesSDK && !a.RequiresStableAPIs(ctx) &&
			!Bool(a.appProperties.Jni_uses_platform_apis)) ||
			Bool(a.appProperties.Jni_uses_sdk_apis) {
			variation = append(variation, blueprint.Variation{Mutator: "sdk", Variation: "sdk"})
		}
		ctx.AddFarVariationDependencies(variation, tag, a.appProperties.Jni_libs...)
	}

	a.usesLibrary.deps(ctx, sdkDep.hasFrameworkLibs())
}

func (a *AndroidApp) OverridablePropertiesDepsMutator(ctx android.BottomUpMutatorContext) {
	cert := android.SrcIsModule(a.getCertString(ctx))
	if cert != "" {
		ctx.AddDependency(ctx.Module(), certificateTag, cert)
	}

	for _, cert := range a.appProperties.Additional_certificates {
		cert = android.SrcIsModule(cert)
		if cert != "" {
			ctx.AddDependency(ctx.Module(), certificateTag, cert)
		} else {
			ctx.PropertyErrorf("additional_certificates",
				`must be names of android_app_certificate modules in the form ":module"`)
		}
	}
}

func (a *AndroidTestHelperApp) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	a.generateAndroidBuildActions(ctx)
}

func (a *AndroidApp) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	a.checkAppSdkVersions(ctx)
	a.generateAndroidBuildActions(ctx)
}

func (a *AndroidApp) checkAppSdkVersions(ctx android.ModuleContext) {
	if a.Updatable() {
		if !a.sdkVersion().stable() {
			ctx.PropertyErrorf("sdk_version", "Updatable apps must use stable SDKs, found %v", a.sdkVersion())
		}
		if String(a.deviceProperties.Min_sdk_version) == "" {
			ctx.PropertyErrorf("updatable", "updatable apps must set min_sdk_version.")
		}
		if minSdkVersion, err := a.minSdkVersion().effectiveVersion(ctx); err == nil {
			a.checkJniLibsSdkVersion(ctx, minSdkVersion)
		} else {
			ctx.PropertyErrorf("min_sdk_version", "%s", err.Error())
		}
	}

	a.checkPlatformAPI(ctx)
	a.checkSdkVersions(ctx)
}

// If an updatable APK sets min_sdk_version, min_sdk_vesion of JNI libs should match with it.
// This check is enforced for "updatable" APKs (including APK-in-APEX).
// b/155209650: until min_sdk_version is properly supported, use sdk_version instead.
// because, sdk_version is overridden by min_sdk_version (if set as smaller)
// and linkType is checked with dependencies so we can be sure that the whole dependency tree
// will meet the requirements.
func (a *AndroidApp) checkJniLibsSdkVersion(ctx android.ModuleContext, minSdkVersion sdkVersion) {
	// It's enough to check direct JNI deps' sdk_version because all transitive deps from JNI deps are checked in cc.checkLinkType()
	ctx.VisitDirectDeps(func(m android.Module) {
		if !IsJniDepTag(ctx.OtherModuleDependencyTag(m)) {
			return
		}
		dep, _ := m.(*cc.Module)
		// The domain of cc.sdk_version is "current" and <number>
		// We can rely on sdkSpec to convert it to <number> so that "current" is handled
		// properly regardless of sdk finalization.
		jniSdkVersion, err := sdkSpecFrom(dep.SdkVersion()).effectiveVersion(ctx)
		if err != nil || minSdkVersion < jniSdkVersion {
			ctx.OtherModuleErrorf(dep, "sdk_version(%v) is higher than min_sdk_version(%v) of the containing android_app(%v)",
				dep.SdkVersion(), minSdkVersion, ctx.ModuleName())
			return
		}

	})
}

// Returns true if the native libraries should be stored in the APK uncompressed and the
// extractNativeLibs application flag should be set to false in the manifest.
func (a *AndroidApp) useEmbeddedNativeLibs(ctx android.ModuleContext) bool {
	minSdkVersion, err := a.minSdkVersion().effectiveVersion(ctx)
	if err != nil {
		ctx.PropertyErrorf("min_sdk_version", "invalid value %q: %s", a.minSdkVersion(), err)
	}

	return (minSdkVersion >= 23 && Bool(a.appProperties.Use_embedded_native_libs)) ||
		!a.IsForPlatform()
}

// Returns whether this module should have the dex file stored uncompressed in the APK.
func (a *AndroidApp) shouldUncompressDex(ctx android.ModuleContext) bool {
	if Bool(a.appProperties.Use_embedded_dex) {
		return true
	}

	// Uncompress dex in APKs of privileged apps (even for unbundled builds, they may
	// be preinstalled as prebuilts).
	if ctx.Config().UncompressPrivAppDex() && a.Privileged() {
		return true
	}

	if ctx.Config().UnbundledBuild() {
		return false
	}

	return shouldUncompressDex(ctx, &a.dexpreopter)
}

func (a *AndroidApp) shouldEmbedJnis(ctx android.BaseModuleContext) bool {
	return ctx.Config().UnbundledBuild() || Bool(a.appProperties.Use_embedded_native_libs) ||
		!a.IsForPlatform() || a.appProperties.AlwaysPackageNativeLibs
}

func (a *AndroidApp) OverriddenManifestPackageName() string {
	return a.overriddenManifestPackageName
}

func (a *AndroidApp) aaptBuildActions(ctx android.ModuleContext) {
	a.aapt.usesNonSdkApis = Bool(a.Module.deviceProperties.Platform_apis)

	// Ask manifest_fixer to add or update the application element indicating this app has no code.
	a.aapt.hasNoCode = !a.hasCode(ctx)

	aaptLinkFlags := []string{}

	// Add TARGET_AAPT_CHARACTERISTICS values to AAPT link flags if they exist and --product flags were not provided.
	hasProduct := android.PrefixInList(a.aaptProperties.Aaptflags, "--product")
	if !hasProduct && len(ctx.Config().ProductAAPTCharacteristics()) > 0 {
		aaptLinkFlags = append(aaptLinkFlags, "--product", ctx.Config().ProductAAPTCharacteristics())
	}

	if !Bool(a.aaptProperties.Aapt_include_all_resources) {
		// Product AAPT config
		for _, aaptConfig := range ctx.Config().ProductAAPTConfig() {
			aaptLinkFlags = append(aaptLinkFlags, "-c", aaptConfig)
		}

		// Product AAPT preferred config
		if len(ctx.Config().ProductAAPTPreferredConfig()) > 0 {
			aaptLinkFlags = append(aaptLinkFlags, "--preferred-density", ctx.Config().ProductAAPTPreferredConfig())
		}
	}

	manifestPackageName, overridden := ctx.DeviceConfig().OverrideManifestPackageNameFor(ctx.ModuleName())
	if overridden || a.overridableAppProperties.Package_name != nil {
		// The product override variable has a priority over the package_name property.
		if !overridden {
			manifestPackageName = *a.overridableAppProperties.Package_name
		}
		aaptLinkFlags = append(aaptLinkFlags, "--rename-manifest-package "+manifestPackageName)
		a.overriddenManifestPackageName = manifestPackageName
	}

	aaptLinkFlags = append(aaptLinkFlags, a.additionalAaptFlags...)

	a.aapt.splitNames = a.appProperties.Package_splits
	a.aapt.sdkLibraries = a.exportedSdkLibs
	a.aapt.LoggingParent = String(a.overridableAppProperties.Logging_parent)
	a.aapt.buildActions(ctx, sdkContext(a), aaptLinkFlags...)

	// apps manifests are handled by aapt, don't let Module see them
	a.properties.Manifest = nil
}

func (a *AndroidApp) proguardBuildActions(ctx android.ModuleContext) {
	var staticLibProguardFlagFiles android.Paths
	ctx.VisitDirectDeps(func(m android.Module) {
		if lib, ok := m.(AndroidLibraryDependency); ok && ctx.OtherModuleDependencyTag(m) == staticLibTag {
			staticLibProguardFlagFiles = append(staticLibProguardFlagFiles, lib.ExportedProguardFlagFiles()...)
		}
	})

	staticLibProguardFlagFiles = android.FirstUniquePaths(staticLibProguardFlagFiles)

	a.Module.extraProguardFlagFiles = append(a.Module.extraProguardFlagFiles, staticLibProguardFlagFiles...)
	a.Module.extraProguardFlagFiles = append(a.Module.extraProguardFlagFiles, a.proguardOptionsFile)
}

func (a *AndroidApp) installPath(ctx android.ModuleContext) android.InstallPath {
	var installDir string
	if ctx.ModuleName() == "framework-res" {
		// framework-res.apk is installed as system/framework/framework-res.apk
		installDir = "framework"
	} else if a.Privileged() {
		installDir = filepath.Join("priv-app", a.installApkName)
	} else {
		installDir = filepath.Join("app", a.installApkName)
	}

	return android.PathForModuleInstall(ctx, installDir, a.installApkName+".apk")
}

func (a *AndroidApp) dexBuildActions(ctx android.ModuleContext) android.Path {
	a.dexpreopter.installPath = a.installPath(ctx)
	if a.dexProperties.Uncompress_dex == nil {
		// If the value was not force-set by the user, use reasonable default based on the module.
		a.dexProperties.Uncompress_dex = proptools.BoolPtr(a.shouldUncompressDex(ctx))
	}
	a.dexpreopter.uncompressedDex = *a.dexProperties.Uncompress_dex
	a.dexpreopter.enforceUsesLibs = a.usesLibrary.enforceUsesLibraries()
	a.dexpreopter.usesLibs = a.usesLibrary.usesLibraryProperties.Uses_libs
	a.dexpreopter.optionalUsesLibs = a.usesLibrary.presentOptionalUsesLibs(ctx)
	a.dexpreopter.libraryPaths = a.usesLibrary.usesLibraryPaths(ctx)
	a.dexpreopter.manifestFile = a.mergedManifestFile

	if ctx.ModuleName() != "framework-res" {
		a.Module.compile(ctx, a.aaptSrcJar)
	}

	return a.maybeStrippedDexJarFile
}

func (a *AndroidApp) jniBuildActions(jniLibs []jniLib, ctx android.ModuleContext) android.WritablePath {
	var jniJarFile android.WritablePath
	if len(jniLibs) > 0 {
		a.jniLibs = jniLibs
		if a.shouldEmbedJnis(ctx) {
			jniJarFile = android.PathForModuleOut(ctx, "jnilibs.zip")
			a.installPathForJNISymbols = a.installPath(ctx).ToMakePath()
			TransformJniLibsToJar(ctx, jniJarFile, jniLibs, a.useEmbeddedNativeLibs(ctx))
			for _, jni := range jniLibs {
				if jni.coverageFile.Valid() {
					// Only collect coverage for the first target arch if this is a multilib target.
					// TODO(jungjw): Ideally, we want to collect both reports, but that would cause coverage
					// data file path collisions since the current coverage file path format doesn't contain
					// arch-related strings. This is fine for now though; the code coverage team doesn't use
					// multi-arch targets such as test_suite_* for coverage collections yet.
					//
					// Work with the team to come up with a new format that handles multilib modules properly
					// and change this.
					if len(ctx.Config().Targets[android.Android]) == 1 ||
						ctx.Config().Targets[android.Android][0].Arch.ArchType == jni.target.Arch.ArchType {
						a.jniCoverageOutputs = append(a.jniCoverageOutputs, jni.coverageFile.Path())
					}
				}
			}
			a.embeddedJniLibs = true
		}
	}
	return jniJarFile
}

func (a *AndroidApp) JNISymbolsInstalls(installPath string) android.RuleBuilderInstalls {
	var jniSymbols android.RuleBuilderInstalls
	for _, jniLib := range a.jniLibs {
		if jniLib.unstrippedFile != nil {
			jniSymbols = append(jniSymbols, android.RuleBuilderInstall{
				From: jniLib.unstrippedFile,
				To:   filepath.Join(installPath, targetToJniDir(jniLib.target), jniLib.unstrippedFile.Base()),
			})
		}
	}
	return jniSymbols
}

func (a *AndroidApp) noticeBuildActions(ctx android.ModuleContext) {
	// Collect NOTICE files from all dependencies.
	seenModules := make(map[android.Module]bool)
	noticePathSet := make(map[android.Path]bool)

	ctx.WalkDeps(func(child android.Module, parent android.Module) bool {
		// Have we already seen this?
		if _, ok := seenModules[child]; ok {
			return false
		}
		seenModules[child] = true

		// Skip host modules.
		if child.Target().Os.Class == android.Host || child.Target().Os.Class == android.HostCross {
			return false
		}

		path := child.(android.Module).NoticeFile()
		if path.Valid() {
			noticePathSet[path.Path()] = true
		}
		return true
	})

	// If the app has one, add it too.
	if a.NoticeFile().Valid() {
		noticePathSet[a.NoticeFile().Path()] = true
	}

	if len(noticePathSet) == 0 {
		return
	}
	var noticePaths []android.Path
	for path := range noticePathSet {
		noticePaths = append(noticePaths, path)
	}
	sort.Slice(noticePaths, func(i, j int) bool {
		return noticePaths[i].String() < noticePaths[j].String()
	})

	a.noticeOutputs = android.BuildNoticeOutput(ctx, a.installDir, a.installApkName+".apk", noticePaths)
}

// Reads and prepends a main cert from the default cert dir if it hasn't been set already, i.e. it
// isn't a cert module reference. Also checks and enforces system cert restriction if applicable.
func processMainCert(m android.ModuleBase, certPropValue string, certificates []Certificate, ctx android.ModuleContext) []Certificate {
	if android.SrcIsModule(certPropValue) == "" {
		var mainCert Certificate
		if certPropValue != "" {
			defaultDir := ctx.Config().DefaultAppCertificateDir(ctx)
			mainCert = Certificate{
				Pem: defaultDir.Join(ctx, certPropValue+".x509.pem"),
				Key: defaultDir.Join(ctx, certPropValue+".pk8"),
			}
		} else {
			pem, key := ctx.Config().DefaultAppCertificate(ctx)
			mainCert = Certificate{
				Pem: pem,
				Key: key,
			}
		}
		certificates = append([]Certificate{mainCert}, certificates...)
	}

	if !m.Platform() {
		certPath := certificates[0].Pem.String()
		systemCertPath := ctx.Config().DefaultAppCertificateDir(ctx).String()
		if strings.HasPrefix(certPath, systemCertPath) {
			enforceSystemCert := ctx.Config().EnforceSystemCertificate()
			allowed := ctx.Config().EnforceSystemCertificateAllowList()

			if enforceSystemCert && !inList(m.Name(), allowed) {
				ctx.PropertyErrorf("certificate", "The module in product partition cannot be signed with certificate in system.")
			}
		}
	}

	return certificates
}

func (a *AndroidApp) InstallApkName() string {
	return a.installApkName
}

func (a *AndroidApp) generateAndroidBuildActions(ctx android.ModuleContext) {
	var apkDeps android.Paths

	a.aapt.useEmbeddedNativeLibs = a.useEmbeddedNativeLibs(ctx)
	a.aapt.useEmbeddedDex = Bool(a.appProperties.Use_embedded_dex)

	// Check if the install APK name needs to be overridden.
	a.installApkName = ctx.DeviceConfig().OverridePackageNameFor(a.Name())

	if ctx.ModuleName() == "framework-res" {
		// framework-res.apk is installed as system/framework/framework-res.apk
		a.installDir = android.PathForModuleInstall(ctx, "framework")
	} else if a.Privileged() {
		a.installDir = android.PathForModuleInstall(ctx, "priv-app", a.installApkName)
	} else if ctx.InstallInTestcases() {
		a.installDir = android.PathForModuleInstall(ctx, a.installApkName, ctx.DeviceConfig().DeviceArch())
	} else {
		a.installDir = android.PathForModuleInstall(ctx, "app", a.installApkName)
	}
	a.onDeviceDir = android.InstallPathToOnDevicePath(ctx, a.installDir)

	a.noticeBuildActions(ctx)
	if Bool(a.appProperties.Embed_notices) || ctx.Config().IsEnvTrue("ALWAYS_EMBED_NOTICES") {
		a.aapt.noticeFile = a.noticeOutputs.HtmlGzOutput
	}

	// Process all building blocks, from AAPT to certificates.
	a.aaptBuildActions(ctx)

	if a.usesLibrary.enforceUsesLibraries() {
		manifestCheckFile := a.usesLibrary.verifyUsesLibrariesManifest(ctx, a.mergedManifestFile)
		apkDeps = append(apkDeps, manifestCheckFile)
	}

	a.proguardBuildActions(ctx)

	a.linter.mergedManifest = a.aapt.mergedManifestFile
	a.linter.manifest = a.aapt.manifestPath
	a.linter.resources = a.aapt.resourceFiles
	a.linter.buildModuleReportZip = ctx.Config().UnbundledBuild()

	dexJarFile := a.dexBuildActions(ctx)

	jniLibs, certificateDeps := collectAppDeps(ctx, a, a.shouldEmbedJnis(ctx), !Bool(a.appProperties.Jni_uses_platform_apis))
	jniJarFile := a.jniBuildActions(jniLibs, ctx)

	if ctx.Failed() {
		return
	}

	certificates := processMainCert(a.ModuleBase, a.getCertString(ctx), certificateDeps, ctx)
	a.certificate = certificates[0]

	// Build a final signed app package.
	packageFile := android.PathForModuleOut(ctx, a.installApkName+".apk")
	v4SigningRequested := Bool(a.Module.deviceProperties.V4_signature)
	var v4SignatureFile android.WritablePath = nil
	if v4SigningRequested {
		v4SignatureFile = android.PathForModuleOut(ctx, a.installApkName+".apk.idsig")
	}
	var lineageFile android.Path
	if lineage := String(a.overridableAppProperties.Lineage); lineage != "" {
		lineageFile = android.PathForModuleSrc(ctx, lineage)
	}
	CreateAndSignAppPackage(ctx, packageFile, a.exportPackage, jniJarFile, dexJarFile, certificates, apkDeps, v4SignatureFile, lineageFile)
	a.outputFile = packageFile
	if v4SigningRequested {
		a.extraOutputFiles = append(a.extraOutputFiles, v4SignatureFile)
	}

	for _, split := range a.aapt.splits {
		// Sign the split APKs
		packageFile := android.PathForModuleOut(ctx, a.installApkName+"_"+split.suffix+".apk")
		if v4SigningRequested {
			v4SignatureFile = android.PathForModuleOut(ctx, a.installApkName+"_"+split.suffix+".apk.idsig")
		}
		CreateAndSignAppPackage(ctx, packageFile, split.path, nil, nil, certificates, apkDeps, v4SignatureFile, lineageFile)
		a.extraOutputFiles = append(a.extraOutputFiles, packageFile)
		if v4SigningRequested {
			a.extraOutputFiles = append(a.extraOutputFiles, v4SignatureFile)
		}
	}

	// Build an app bundle.
	bundleFile := android.PathForModuleOut(ctx, "base.zip")
	BuildBundleModule(ctx, bundleFile, a.exportPackage, jniJarFile, dexJarFile)
	a.bundleFile = bundleFile

	// Install the app package.
	if (Bool(a.Module.properties.Installable) || ctx.Host()) && a.IsForPlatform() {
		ctx.InstallFile(a.installDir, a.outputFile.Base(), a.outputFile)
		for _, extra := range a.extraOutputFiles {
			ctx.InstallFile(a.installDir, extra.Base(), extra)
		}
	}

	a.buildAppDependencyInfo(ctx)
}

type appDepsInterface interface {
	sdkVersion() sdkSpec
	minSdkVersion() sdkSpec
	RequiresStableAPIs(ctx android.BaseModuleContext) bool
}

func collectAppDeps(ctx android.ModuleContext, app appDepsInterface,
	shouldCollectRecursiveNativeDeps bool,
	checkNativeSdkVersion bool) ([]jniLib, []Certificate) {

	var jniLibs []jniLib
	var certificates []Certificate
	seenModulePaths := make(map[string]bool)

	if checkNativeSdkVersion {
		checkNativeSdkVersion = app.sdkVersion().specified() &&
			app.sdkVersion().kind != sdkCorePlatform && !app.RequiresStableAPIs(ctx)
	}

	ctx.WalkDeps(func(module android.Module, parent android.Module) bool {
		otherName := ctx.OtherModuleName(module)
		tag := ctx.OtherModuleDependencyTag(module)

		if IsJniDepTag(tag) || tag == cc.SharedDepTag {
			if dep, ok := module.(*cc.Module); ok {
				if dep.IsNdk() || dep.IsStubs() {
					return false
				}

				lib := dep.OutputFile()
				path := lib.Path()
				if seenModulePaths[path.String()] {
					return false
				}
				seenModulePaths[path.String()] = true

				if checkNativeSdkVersion && dep.SdkVersion() == "" {
					ctx.PropertyErrorf("jni_libs", "JNI dependency %q uses platform APIs, but this module does not",
						otherName)
				}

				if lib.Valid() {
					jniLibs = append(jniLibs, jniLib{
						name:           ctx.OtherModuleName(module),
						path:           path,
						target:         module.Target(),
						coverageFile:   dep.CoverageOutputFile(),
						unstrippedFile: dep.UnstrippedOutputFile(),
					})
				} else {
					ctx.ModuleErrorf("dependency %q missing output file", otherName)
				}
			} else {
				ctx.ModuleErrorf("jni_libs dependency %q must be a cc library", otherName)
			}

			return shouldCollectRecursiveNativeDeps
		}

		if tag == certificateTag {
			if dep, ok := module.(*AndroidAppCertificate); ok {
				certificates = append(certificates, dep.Certificate)
			} else {
				ctx.ModuleErrorf("certificate dependency %q must be an android_app_certificate module", otherName)
			}
		}

		return false
	})

	return jniLibs, certificates
}

func (a *AndroidApp) walkPayloadDeps(ctx android.ModuleContext,
	do func(ctx android.ModuleContext, from blueprint.Module, to android.ApexModule, externalDep bool)) {

	ctx.WalkDeps(func(child, parent android.Module) bool {
		isExternal := !a.DepIsInSameApex(ctx, child)
		if am, ok := child.(android.ApexModule); ok {
			do(ctx, parent, am, isExternal)
		}
		return !isExternal
	})
}

func (a *AndroidApp) buildAppDependencyInfo(ctx android.ModuleContext) {
	if ctx.Host() {
		return
	}

	depsInfo := android.DepNameToDepInfoMap{}
	a.walkPayloadDeps(ctx, func(ctx android.ModuleContext, from blueprint.Module, to android.ApexModule, externalDep bool) {
		depName := to.Name()
		if info, exist := depsInfo[depName]; exist {
			info.From = append(info.From, from.Name())
			info.IsExternal = info.IsExternal && externalDep
			depsInfo[depName] = info
		} else {
			toMinSdkVersion := "(no version)"
			if m, ok := to.(interface{ MinSdkVersion() string }); ok {
				if v := m.MinSdkVersion(); v != "" {
					toMinSdkVersion = v
				}
			}
			depsInfo[depName] = android.ApexModuleDepInfo{
				To:            depName,
				From:          []string{from.Name()},
				IsExternal:    externalDep,
				MinSdkVersion: toMinSdkVersion,
			}
		}
	})

	a.ApexBundleDepsInfo.BuildDepsInfoLists(ctx, a.MinSdkVersion(), depsInfo)
}

func (a *AndroidApp) Updatable() bool {
	return Bool(a.appProperties.Updatable) || a.ApexModuleBase.Updatable()
}

func (a *AndroidApp) getCertString(ctx android.BaseModuleContext) string {
	certificate, overridden := ctx.DeviceConfig().OverrideCertificateFor(ctx.ModuleName())
	if overridden {
		return ":" + certificate
	}
	return String(a.overridableAppProperties.Certificate)
}

func (a *AndroidApp) DepIsInSameApex(ctx android.BaseModuleContext, dep android.Module) bool {
	if IsJniDepTag(ctx.OtherModuleDependencyTag(dep)) {
		return true
	}
	return a.Library.DepIsInSameApex(ctx, dep)
}

// For OutputFileProducer interface
func (a *AndroidApp) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case ".aapt.srcjar":
		return []android.Path{a.aaptSrcJar}, nil
	case ".export-package.apk":
		return []android.Path{a.exportPackage}, nil
	}
	return a.Library.OutputFiles(tag)
}

func (a *AndroidApp) Privileged() bool {
	return Bool(a.appProperties.Privileged)
}

func (a *AndroidApp) IsNativeCoverageNeeded(ctx android.BaseModuleContext) bool {
	return ctx.Device() && ctx.DeviceConfig().NativeCoverageEnabled()
}

func (a *AndroidApp) PreventInstall() {
	a.appProperties.PreventInstall = true
}

func (a *AndroidApp) HideFromMake() {
	a.appProperties.HideFromMake = true
}

func (a *AndroidApp) MarkAsCoverageVariant(coverage bool) {
	a.appProperties.IsCoverageVariant = coverage
}

var _ cc.Coverage = (*AndroidApp)(nil)

// android_app compiles sources and Android resources into an Android application package `.apk` file.
func AndroidAppFactory() android.Module {
	module := &AndroidApp{}

	module.Module.dexProperties.Optimize.EnabledByDefault = true
	module.Module.dexProperties.Optimize.Shrink = proptools.BoolPtr(true)

	module.Module.properties.Instrument = true
	module.Module.properties.Installable = proptools.BoolPtr(true)

	module.addHostAndDeviceProperties()
	module.AddProperties(
		&module.aaptProperties,
		&module.appProperties,
		&module.overridableAppProperties,
		&module.usesLibrary.usesLibraryProperties)

	module.Prefer32(func(ctx android.BaseModuleContext, base *android.ModuleBase, class android.OsClass) bool {
		return class == android.Device && ctx.Config().DevicePrefer32BitApps()
	})

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitOverridableModule(module, &module.appProperties.Overrides)
	android.InitApexModule(module)

	return module
}

type appTestProperties struct {
	Instrumentation_for *string

	// if specified, the instrumentation target package name in the manifest is overwritten by it.
	Instrumentation_target_package *string
}

type AndroidTest struct {
	AndroidApp

	appTestProperties appTestProperties

	testProperties testProperties

	testConfig android.Path
	data       android.Paths
}

func (a *AndroidTest) InstallInTestcases() bool {
	return true
}

func (a *AndroidTest) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	var configs []tradefed.Config
	if a.appTestProperties.Instrumentation_target_package != nil {
		a.additionalAaptFlags = append(a.additionalAaptFlags,
			"--rename-instrumentation-target-package "+*a.appTestProperties.Instrumentation_target_package)
	} else if a.appTestProperties.Instrumentation_for != nil {
		// Check if the instrumentation target package is overridden.
		manifestPackageName, overridden := ctx.DeviceConfig().OverrideManifestPackageNameFor(*a.appTestProperties.Instrumentation_for)
		if overridden {
			a.additionalAaptFlags = append(a.additionalAaptFlags, "--rename-instrumentation-target-package "+manifestPackageName)
		}
	}
	a.generateAndroidBuildActions(ctx)

	for _, module := range a.testProperties.Test_mainline_modules {
		configs = append(configs, tradefed.Option{Name: "config-descriptor:metadata", Key: "mainline-param", Value: module})
	}

	testConfig := tradefed.AutoGenInstrumentationTestConfig(ctx, a.testProperties.Test_config,
		a.testProperties.Test_config_template, a.manifestPath, a.testProperties.Test_suites, a.testProperties.Auto_gen_config, configs)
	a.testConfig = a.FixTestConfig(ctx, testConfig)
	a.data = android.PathsForModuleSrc(ctx, a.testProperties.Data)
}

func (a *AndroidTest) FixTestConfig(ctx android.ModuleContext, testConfig android.Path) android.Path {
	if testConfig == nil {
		return nil
	}

	fixedConfig := android.PathForModuleOut(ctx, "test_config_fixer", "AndroidTest.xml")
	rule := android.NewRuleBuilder()
	command := rule.Command().BuiltTool(ctx, "test_config_fixer").Input(testConfig).Output(fixedConfig)
	fixNeeded := false

	if ctx.ModuleName() != a.installApkName {
		fixNeeded = true
		command.FlagWithArg("--test-file-name ", a.installApkName+".apk")
	}

	if a.overridableAppProperties.Package_name != nil {
		fixNeeded = true
		command.FlagWithInput("--manifest ", a.manifestPath).
			FlagWithArg("--package-name ", *a.overridableAppProperties.Package_name)
	}

	if fixNeeded {
		rule.Build(pctx, ctx, "fix_test_config", "fix test config")
		return fixedConfig
	}
	return testConfig
}

func (a *AndroidTest) DepsMutator(ctx android.BottomUpMutatorContext) {
	a.AndroidApp.DepsMutator(ctx)
}

func (a *AndroidTest) OverridablePropertiesDepsMutator(ctx android.BottomUpMutatorContext) {
	a.AndroidApp.OverridablePropertiesDepsMutator(ctx)
	if a.appTestProperties.Instrumentation_for != nil {
		// The android_app dependency listed in instrumentation_for needs to be added to the classpath for javac,
		// but not added to the aapt2 link includes like a normal android_app or android_library dependency, so
		// use instrumentationForTag instead of libTag.
		ctx.AddVariationDependencies(nil, instrumentationForTag, String(a.appTestProperties.Instrumentation_for))
	}
}

// android_test compiles test sources and Android resources into an Android application package `.apk` file and
// creates an `AndroidTest.xml` file to allow running the test with `atest` or a `TEST_MAPPING` file.
func AndroidTestFactory() android.Module {
	module := &AndroidTest{}

	module.Module.dexProperties.Optimize.EnabledByDefault = true

	module.Module.properties.Instrument = true
	module.Module.properties.Installable = proptools.BoolPtr(true)
	module.appProperties.Use_embedded_native_libs = proptools.BoolPtr(true)
	module.appProperties.AlwaysPackageNativeLibs = true
	module.Module.dexpreopter.isTest = true
	module.Module.linter.test = true

	module.addHostAndDeviceProperties()
	module.AddProperties(
		&module.aaptProperties,
		&module.appProperties,
		&module.appTestProperties,
		&module.overridableAppProperties,
		&module.usesLibrary.usesLibraryProperties,
		&module.testProperties)

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitOverridableModule(module, &module.appProperties.Overrides)
	return module
}

type appTestHelperAppProperties struct {
	// list of compatibility suites (for example "cts", "vts") that the module should be
	// installed into.
	Test_suites []string `android:"arch_variant"`

	// Flag to indicate whether or not to create test config automatically. If AndroidTest.xml
	// doesn't exist next to the Android.bp, this attribute doesn't need to be set to true
	// explicitly.
	Auto_gen_config *bool
}

type AndroidTestHelperApp struct {
	AndroidApp

	appTestHelperAppProperties appTestHelperAppProperties
}

func (a *AndroidTestHelperApp) InstallInTestcases() bool {
	return true
}

// android_test_helper_app compiles sources and Android resources into an Android application package `.apk` file that
// will be used by tests, but does not produce an `AndroidTest.xml` file so the module will not be run directly as a
// test.
func AndroidTestHelperAppFactory() android.Module {
	module := &AndroidTestHelperApp{}

	module.Module.dexProperties.Optimize.EnabledByDefault = true

	module.Module.properties.Installable = proptools.BoolPtr(true)
	module.appProperties.Use_embedded_native_libs = proptools.BoolPtr(true)
	module.appProperties.AlwaysPackageNativeLibs = true
	module.Module.dexpreopter.isTest = true
	module.Module.linter.test = true

	module.addHostAndDeviceProperties()
	module.AddProperties(
		&module.aaptProperties,
		&module.appProperties,
		&module.appTestHelperAppProperties,
		&module.overridableAppProperties,
		&module.usesLibrary.usesLibraryProperties)

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitApexModule(module)
	return module
}

type AndroidAppCertificate struct {
	android.ModuleBase
	properties  AndroidAppCertificateProperties
	Certificate Certificate
}

type AndroidAppCertificateProperties struct {
	// Name of the certificate files.  Extensions .x509.pem and .pk8 will be added to the name.
	Certificate *string
}

// android_app_certificate modules can be referenced by the certificates property of android_app modules to select
// the signing key.
func AndroidAppCertificateFactory() android.Module {
	module := &AndroidAppCertificate{}
	module.AddProperties(&module.properties)
	android.InitAndroidModule(module)
	return module
}

func (c *AndroidAppCertificate) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	cert := String(c.properties.Certificate)
	c.Certificate = Certificate{
		Pem: android.PathForModuleSrc(ctx, cert+".x509.pem"),
		Key: android.PathForModuleSrc(ctx, cert+".pk8"),
	}
}

type OverrideAndroidApp struct {
	android.ModuleBase
	android.OverrideModuleBase
}

func (i *OverrideAndroidApp) GenerateAndroidBuildActions(_ android.ModuleContext) {
	// All the overrides happen in the base module.
	// TODO(jungjw): Check the base module type.
}

// override_android_app is used to create an android_app module based on another android_app by overriding
// some of its properties.
func OverrideAndroidAppModuleFactory() android.Module {
	m := &OverrideAndroidApp{}
	m.AddProperties(&overridableAppProperties{})

	android.InitAndroidMultiTargetsArchModule(m, android.DeviceSupported, android.MultilibCommon)
	android.InitOverrideModule(m)
	return m
}

type OverrideAndroidTest struct {
	android.ModuleBase
	android.OverrideModuleBase
}

func (i *OverrideAndroidTest) GenerateAndroidBuildActions(_ android.ModuleContext) {
	// All the overrides happen in the base module.
	// TODO(jungjw): Check the base module type.
}

// override_android_test is used to create an android_app module based on another android_test by overriding
// some of its properties.
func OverrideAndroidTestModuleFactory() android.Module {
	m := &OverrideAndroidTest{}
	m.AddProperties(&overridableAppProperties{})
	m.AddProperties(&appTestProperties{})

	android.InitAndroidMultiTargetsArchModule(m, android.DeviceSupported, android.MultilibCommon)
	android.InitOverrideModule(m)
	return m
}

type OverrideRuntimeResourceOverlay struct {
	android.ModuleBase
	android.OverrideModuleBase
}

func (i *OverrideRuntimeResourceOverlay) GenerateAndroidBuildActions(_ android.ModuleContext) {
	// All the overrides happen in the base module.
	// TODO(jungjw): Check the base module type.
}

// override_runtime_resource_overlay is used to create a module based on another
// runtime_resource_overlay module by overriding some of its properties.
func OverrideRuntimeResourceOverlayModuleFactory() android.Module {
	m := &OverrideRuntimeResourceOverlay{}
	m.AddProperties(&OverridableRuntimeResourceOverlayProperties{})

	android.InitAndroidMultiTargetsArchModule(m, android.DeviceSupported, android.MultilibCommon)
	android.InitOverrideModule(m)
	return m
}

type AndroidAppImport struct {
	android.ModuleBase
	android.DefaultableModuleBase
	prebuilt android.Prebuilt

	properties   AndroidAppImportProperties
	dpiVariants  interface{}
	archVariants interface{}

	outputFile  android.Path
	certificate Certificate

	dexpreopter

	usesLibrary usesLibrary

	preprocessed bool

	installPath android.InstallPath
}

type AndroidAppImportProperties struct {
	// A prebuilt apk to import
	Apk *string

	// The name of a certificate in the default certificate directory or an android_app_certificate
	// module name in the form ":module". Should be empty if presigned or default_dev_cert is set.
	Certificate *string

	// Set this flag to true if the prebuilt apk is already signed. The certificate property must not
	// be set for presigned modules.
	Presigned *bool

	// Name of the signing certificate lineage file.
	Lineage *string

	// Sign with the default system dev certificate. Must be used judiciously. Most imported apps
	// need to either specify a specific certificate or be presigned.
	Default_dev_cert *bool

	// Specifies that this app should be installed to the priv-app directory,
	// where the system will grant it additional privileges not available to
	// normal apps.
	Privileged *bool

	// Names of modules to be overridden. Listed modules can only be other binaries
	// (in Make or Soong).
	// This does not completely prevent installation of the overridden binaries, but if both
	// binaries would be installed by default (in PRODUCT_PACKAGES) the other binary will be removed
	// from PRODUCT_PACKAGES.
	Overrides []string

	// Optional name for the installed app. If unspecified, it is derived from the module name.
	Filename *string
}

func (a *AndroidAppImport) IsInstallable() bool {
	return true
}

// Updates properties with variant-specific values.
func (a *AndroidAppImport) processVariants(ctx android.LoadHookContext) {
	config := ctx.Config()

	dpiProps := reflect.ValueOf(a.dpiVariants).Elem().FieldByName("Dpi_variants")
	// Try DPI variant matches in the reverse-priority order so that the highest priority match
	// overwrites everything else.
	// TODO(jungjw): Can we optimize this by making it priority order?
	for i := len(config.ProductAAPTPrebuiltDPI()) - 1; i >= 0; i-- {
		MergePropertiesFromVariant(ctx, &a.properties, dpiProps, config.ProductAAPTPrebuiltDPI()[i])
	}
	if config.ProductAAPTPreferredConfig() != "" {
		MergePropertiesFromVariant(ctx, &a.properties, dpiProps, config.ProductAAPTPreferredConfig())
	}

	archProps := reflect.ValueOf(a.archVariants).Elem().FieldByName("Arch")
	archType := ctx.Config().Targets[android.Android][0].Arch.ArchType
	MergePropertiesFromVariant(ctx, &a.properties, archProps, archType.Name)
}

func MergePropertiesFromVariant(ctx android.EarlyModuleContext,
	dst interface{}, variantGroup reflect.Value, variant string) {
	src := variantGroup.FieldByName(proptools.FieldNameForProperty(variant))
	if !src.IsValid() {
		return
	}

	err := proptools.ExtendMatchingProperties([]interface{}{dst}, src.Interface(), nil, proptools.OrderAppend)
	if err != nil {
		if propertyErr, ok := err.(*proptools.ExtendPropertyError); ok {
			ctx.PropertyErrorf(propertyErr.Property, "%s", propertyErr.Err.Error())
		} else {
			panic(err)
		}
	}
}

func (a *AndroidAppImport) DepsMutator(ctx android.BottomUpMutatorContext) {
	cert := android.SrcIsModule(String(a.properties.Certificate))
	if cert != "" {
		ctx.AddDependency(ctx.Module(), certificateTag, cert)
	}

	a.usesLibrary.deps(ctx, true)
}

func (a *AndroidAppImport) uncompressEmbeddedJniLibs(
	ctx android.ModuleContext, inputPath android.Path, outputPath android.OutputPath) {
	// Test apps don't need their JNI libraries stored uncompressed. As a matter of fact, messing
	// with them may invalidate pre-existing signature data.
	if ctx.InstallInTestcases() && (Bool(a.properties.Presigned) || a.preprocessed) {
		ctx.Build(pctx, android.BuildParams{
			Rule:   android.Cp,
			Output: outputPath,
			Input:  inputPath,
		})
		return
	}
	rule := android.NewRuleBuilder()
	rule.Command().
		Textf(`if (zipinfo %s 'lib/*.so' 2>/dev/null | grep -v ' stor ' >/dev/null) ; then`, inputPath).
		BuiltTool(ctx, "zip2zip").
		FlagWithInput("-i ", inputPath).
		FlagWithOutput("-o ", outputPath).
		FlagWithArg("-0 ", "'lib/**/*.so'").
		Textf(`; else cp -f %s %s; fi`, inputPath, outputPath)
	rule.Build(pctx, ctx, "uncompress-embedded-jni-libs", "Uncompress embedded JIN libs")
}

// Returns whether this module should have the dex file stored uncompressed in the APK.
func (a *AndroidAppImport) shouldUncompressDex(ctx android.ModuleContext) bool {
	if ctx.Config().UnbundledBuild() || a.preprocessed {
		return false
	}

	// Uncompress dex in APKs of privileged apps
	if ctx.Config().UncompressPrivAppDex() && a.Privileged() {
		return true
	}

	return shouldUncompressDex(ctx, &a.dexpreopter)
}

func (a *AndroidAppImport) uncompressDex(
	ctx android.ModuleContext, inputPath android.Path, outputPath android.OutputPath) {
	rule := android.NewRuleBuilder()
	rule.Command().
		Textf(`if (zipinfo %s '*.dex' 2>/dev/null | grep -v ' stor ' >/dev/null) ; then`, inputPath).
		BuiltTool(ctx, "zip2zip").
		FlagWithInput("-i ", inputPath).
		FlagWithOutput("-o ", outputPath).
		FlagWithArg("-0 ", "'classes*.dex'").
		Textf(`; else cp -f %s %s; fi`, inputPath, outputPath)
	rule.Build(pctx, ctx, "uncompress-dex", "Uncompress dex files")
}

func (a *AndroidAppImport) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	a.generateAndroidBuildActions(ctx)
}

func (a *AndroidAppImport) InstallApkName() string {
	return a.BaseModuleName()
}

func (a *AndroidAppImport) generateAndroidBuildActions(ctx android.ModuleContext) {
	numCertPropsSet := 0
	if String(a.properties.Certificate) != "" {
		numCertPropsSet++
	}
	if Bool(a.properties.Presigned) {
		numCertPropsSet++
	}
	if Bool(a.properties.Default_dev_cert) {
		numCertPropsSet++
	}
	if numCertPropsSet != 1 {
		ctx.ModuleErrorf("One and only one of certficate, presigned, and default_dev_cert properties must be set")
	}

	_, certificates := collectAppDeps(ctx, a, false, false)

	// TODO: LOCAL_EXTRACT_APK/LOCAL_EXTRACT_DPI_APK
	// TODO: LOCAL_PACKAGE_SPLITS

	srcApk := a.prebuilt.SingleSourcePath(ctx)

	if a.usesLibrary.enforceUsesLibraries() {
		srcApk = a.usesLibrary.verifyUsesLibrariesAPK(ctx, srcApk)
	}

	// TODO: Install or embed JNI libraries

	// Uncompress JNI libraries in the apk
	jnisUncompressed := android.PathForModuleOut(ctx, "jnis-uncompressed", ctx.ModuleName()+".apk")
	a.uncompressEmbeddedJniLibs(ctx, srcApk, jnisUncompressed.OutputPath)

	var installDir android.InstallPath
	if Bool(a.properties.Privileged) {
		installDir = android.PathForModuleInstall(ctx, "priv-app", a.BaseModuleName())
	} else if ctx.InstallInTestcases() {
		installDir = android.PathForModuleInstall(ctx, a.BaseModuleName(), ctx.DeviceConfig().DeviceArch())
	} else {
		installDir = android.PathForModuleInstall(ctx, "app", a.BaseModuleName())
	}

	a.dexpreopter.installPath = installDir.Join(ctx, a.BaseModuleName()+".apk")
	a.dexpreopter.isPresignedPrebuilt = Bool(a.properties.Presigned)
	a.dexpreopter.uncompressedDex = a.shouldUncompressDex(ctx)

	a.dexpreopter.enforceUsesLibs = a.usesLibrary.enforceUsesLibraries()
	a.dexpreopter.usesLibs = a.usesLibrary.usesLibraryProperties.Uses_libs
	a.dexpreopter.optionalUsesLibs = a.usesLibrary.presentOptionalUsesLibs(ctx)
	a.dexpreopter.libraryPaths = a.usesLibrary.usesLibraryPaths(ctx)

	dexOutput := a.dexpreopter.dexpreopt(ctx, jnisUncompressed)
	if a.dexpreopter.uncompressedDex {
		dexUncompressed := android.PathForModuleOut(ctx, "dex-uncompressed", ctx.ModuleName()+".apk")
		a.uncompressDex(ctx, dexOutput, dexUncompressed.OutputPath)
		dexOutput = dexUncompressed
	}

	apkFilename := proptools.StringDefault(a.properties.Filename, a.BaseModuleName()+".apk")

	// TODO: Handle EXTERNAL

	// Sign or align the package if package has not been preprocessed
	if a.preprocessed {
		a.outputFile = srcApk
		a.certificate = PresignedCertificate
	} else if !Bool(a.properties.Presigned) {
		// If the certificate property is empty at this point, default_dev_cert must be set to true.
		// Which makes processMainCert's behavior for the empty cert string WAI.
		certificates = processMainCert(a.ModuleBase, String(a.properties.Certificate), certificates, ctx)
		if len(certificates) != 1 {
			ctx.ModuleErrorf("Unexpected number of certificates were extracted: %q", certificates)
		}
		a.certificate = certificates[0]
		signed := android.PathForModuleOut(ctx, "signed", apkFilename)
		var lineageFile android.Path
		if lineage := String(a.properties.Lineage); lineage != "" {
			lineageFile = android.PathForModuleSrc(ctx, lineage)
		}
		SignAppPackage(ctx, signed, dexOutput, certificates, nil, lineageFile)
		a.outputFile = signed
	} else {
		alignedApk := android.PathForModuleOut(ctx, "zip-aligned", apkFilename)
		TransformZipAlign(ctx, alignedApk, dexOutput)
		a.outputFile = alignedApk
		a.certificate = PresignedCertificate
	}

	// TODO: Optionally compress the output apk.

	a.installPath = ctx.InstallFile(installDir, apkFilename, a.outputFile)

	// TODO: androidmk converter jni libs
}

func (a *AndroidAppImport) Prebuilt() *android.Prebuilt {
	return &a.prebuilt
}

func (a *AndroidAppImport) Name() string {
	return a.prebuilt.Name(a.ModuleBase.Name())
}

func (a *AndroidAppImport) OutputFile() android.Path {
	return a.outputFile
}

func (a *AndroidAppImport) JacocoReportClassesFile() android.Path {
	return nil
}

func (a *AndroidAppImport) Certificate() Certificate {
	return a.certificate
}

var dpiVariantGroupType reflect.Type
var archVariantGroupType reflect.Type

func initAndroidAppImportVariantGroupTypes() {
	dpiVariantGroupType = createVariantGroupType(supportedDpis, "Dpi_variants")

	archNames := make([]string, len(android.ArchTypeList()))
	for i, archType := range android.ArchTypeList() {
		archNames[i] = archType.Name
	}
	archVariantGroupType = createVariantGroupType(archNames, "Arch")
}

// Populates all variant struct properties at creation time.
func (a *AndroidAppImport) populateAllVariantStructs() {
	a.dpiVariants = reflect.New(dpiVariantGroupType).Interface()
	a.AddProperties(a.dpiVariants)

	a.archVariants = reflect.New(archVariantGroupType).Interface()
	a.AddProperties(a.archVariants)
}

func (a *AndroidAppImport) Privileged() bool {
	return Bool(a.properties.Privileged)
}

func (a *AndroidAppImport) sdkVersion() sdkSpec {
	return sdkSpecFrom("")
}

func (a *AndroidAppImport) minSdkVersion() sdkSpec {
	return sdkSpecFrom("")
}

func createVariantGroupType(variants []string, variantGroupName string) reflect.Type {
	props := reflect.TypeOf((*AndroidAppImportProperties)(nil))

	variantFields := make([]reflect.StructField, len(variants))
	for i, variant := range variants {
		variantFields[i] = reflect.StructField{
			Name: proptools.FieldNameForProperty(variant),
			Type: props,
		}
	}

	variantGroupStruct := reflect.StructOf(variantFields)
	return reflect.StructOf([]reflect.StructField{
		{
			Name: variantGroupName,
			Type: variantGroupStruct,
		},
	})
}

// android_app_import imports a prebuilt apk with additional processing specified in the module.
// DPI-specific apk source files can be specified using dpi_variants. Example:
//
//     android_app_import {
//         name: "example_import",
//         apk: "prebuilts/example.apk",
//         dpi_variants: {
//             mdpi: {
//                 apk: "prebuilts/example_mdpi.apk",
//             },
//             xhdpi: {
//                 apk: "prebuilts/example_xhdpi.apk",
//             },
//         },
//         certificate: "PRESIGNED",
//     }
func AndroidAppImportFactory() android.Module {
	module := &AndroidAppImport{}
	module.AddProperties(&module.properties)
	module.AddProperties(&module.dexpreoptProperties)
	module.AddProperties(&module.usesLibrary.usesLibraryProperties)
	module.populateAllVariantStructs()
	android.AddLoadHook(module, func(ctx android.LoadHookContext) {
		module.processVariants(ctx)
	})

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitSingleSourcePrebuiltModule(module, &module.properties, "Apk")

	return module
}

type androidTestImportProperties struct {
	// Whether the prebuilt apk can be installed without additional processing. Default is false.
	Preprocessed *bool
}

type AndroidTestImport struct {
	AndroidAppImport

	testProperties testProperties

	testImportProperties androidTestImportProperties

	data android.Paths
}

func (a *AndroidTestImport) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	a.preprocessed = Bool(a.testImportProperties.Preprocessed)

	a.generateAndroidBuildActions(ctx)

	a.data = android.PathsForModuleSrc(ctx, a.testProperties.Data)
}

func (a *AndroidTestImport) InstallInTestcases() bool {
	return true
}

// android_test_import imports a prebuilt test apk with additional processing specified in the
// module. DPI or arch variant configurations can be made as with android_app_import.
func AndroidTestImportFactory() android.Module {
	module := &AndroidTestImport{}
	module.AddProperties(&module.properties)
	module.AddProperties(&module.dexpreoptProperties)
	module.AddProperties(&module.usesLibrary.usesLibraryProperties)
	module.AddProperties(&module.testProperties)
	module.AddProperties(&module.testImportProperties)
	module.populateAllVariantStructs()
	android.AddLoadHook(module, func(ctx android.LoadHookContext) {
		module.processVariants(ctx)
	})

	module.dexpreopter.isTest = true

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitSingleSourcePrebuiltModule(module, &module.properties, "Apk")

	return module
}

type RuntimeResourceOverlay struct {
	android.ModuleBase
	android.DefaultableModuleBase
	android.OverridableModuleBase
	aapt

	properties            RuntimeResourceOverlayProperties
	overridableProperties OverridableRuntimeResourceOverlayProperties

	certificate Certificate

	outputFile android.Path
	installDir android.InstallPath
}

type RuntimeResourceOverlayProperties struct {
	// the name of a certificate in the default certificate directory or an android_app_certificate
	// module name in the form ":module".
	Certificate *string

	// Name of the signing certificate lineage file.
	Lineage *string

	// optional theme name. If specified, the overlay package will be applied
	// only when the ro.boot.vendor.overlay.theme system property is set to the same value.
	Theme *string

	// if not blank, set to the version of the sdk to compile against.
	// Defaults to compiling against the current platform.
	Sdk_version *string

	// if not blank, set the minimum version of the sdk that the compiled artifacts will run against.
	// Defaults to sdk_version if not set.
	Min_sdk_version *string

	// list of android_library modules whose resources are extracted and linked against statically
	Static_libs []string

	// list of android_app modules whose resources are extracted and linked against
	Resource_libs []string

	// Names of modules to be overridden. Listed modules can only be other overlays
	// (in Make or Soong).
	// This does not completely prevent installation of the overridden overlays, but if both
	// overlays would be installed by default (in PRODUCT_PACKAGES) the other overlay will be removed
	// from PRODUCT_PACKAGES.
	Overrides []string
}

func (r *RuntimeResourceOverlay) DepsMutator(ctx android.BottomUpMutatorContext) {
	sdkDep := decodeSdkDep(ctx, sdkContext(r))
	if sdkDep.hasFrameworkLibs() {
		r.aapt.deps(ctx, sdkDep)
	}

	cert := android.SrcIsModule(String(r.properties.Certificate))
	if cert != "" {
		ctx.AddDependency(ctx.Module(), certificateTag, cert)
	}

	ctx.AddVariationDependencies(nil, staticLibTag, r.properties.Static_libs...)
	ctx.AddVariationDependencies(nil, libTag, r.properties.Resource_libs...)
}

func (r *RuntimeResourceOverlay) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Compile and link resources
	r.aapt.hasNoCode = true
	// Do not remove resources without default values nor dedupe resource configurations with the same value
	aaptLinkFlags := []string{"--no-resource-deduping", "--no-resource-removal"}
	// Allow the override of "package name" and "overlay target package name"
	manifestPackageName, overridden := ctx.DeviceConfig().OverrideManifestPackageNameFor(ctx.ModuleName())
	if overridden || r.overridableProperties.Package_name != nil {
		// The product override variable has a priority over the package_name property.
		if !overridden {
			manifestPackageName = *r.overridableProperties.Package_name
		}
		aaptLinkFlags = append(aaptLinkFlags, "--rename-manifest-package "+manifestPackageName)
	}
	if r.overridableProperties.Target_package_name != nil {
		aaptLinkFlags = append(aaptLinkFlags,
			"--rename-overlay-target-package "+*r.overridableProperties.Target_package_name)
	}
	r.aapt.buildActions(ctx, r, aaptLinkFlags...)

	// Sign the built package
	_, certificates := collectAppDeps(ctx, r, false, false)
	certificates = processMainCert(r.ModuleBase, String(r.properties.Certificate), certificates, ctx)
	signed := android.PathForModuleOut(ctx, "signed", r.Name()+".apk")
	var lineageFile android.Path
	if lineage := String(r.properties.Lineage); lineage != "" {
		lineageFile = android.PathForModuleSrc(ctx, lineage)
	}
	SignAppPackage(ctx, signed, r.aapt.exportPackage, certificates, nil, lineageFile)
	r.certificate = certificates[0]

	r.outputFile = signed
	r.installDir = android.PathForModuleInstall(ctx, "overlay", String(r.properties.Theme))
	ctx.InstallFile(r.installDir, r.outputFile.Base(), r.outputFile)
}

func (r *RuntimeResourceOverlay) sdkVersion() sdkSpec {
	return sdkSpecFrom(String(r.properties.Sdk_version))
}

func (r *RuntimeResourceOverlay) systemModules() string {
	return ""
}

func (r *RuntimeResourceOverlay) minSdkVersion() sdkSpec {
	if r.properties.Min_sdk_version != nil {
		return sdkSpecFrom(*r.properties.Min_sdk_version)
	}
	return r.sdkVersion()
}

func (r *RuntimeResourceOverlay) targetSdkVersion() sdkSpec {
	return r.sdkVersion()
}

// runtime_resource_overlay generates a resource-only apk file that can overlay application and
// system resources at run time.
func RuntimeResourceOverlayFactory() android.Module {
	module := &RuntimeResourceOverlay{}
	module.AddProperties(
		&module.properties,
		&module.aaptProperties,
		&module.overridableProperties)

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitOverridableModule(module, &module.properties.Overrides)
	return module
}

type UsesLibraryProperties struct {
	// A list of shared library modules that will be listed in uses-library tags in the AndroidManifest.xml file.
	Uses_libs []string

	// A list of shared library modules that will be listed in uses-library tags in the AndroidManifest.xml file with
	// required=false.
	Optional_uses_libs []string

	// If true, the list of uses_libs and optional_uses_libs modules must match the AndroidManifest.xml file.  Defaults
	// to true if either uses_libs or optional_uses_libs is set.  Will unconditionally default to true in the future.
	Enforce_uses_libs *bool
}

// usesLibrary provides properties and helper functions for AndroidApp and AndroidAppImport to verify that the
// <uses-library> tags that end up in the manifest of an APK match the ones known to the build system through the
// uses_libs and optional_uses_libs properties.  The build system's values are used by dexpreopt to preopt apps
// with knowledge of their shared libraries.
type usesLibrary struct {
	usesLibraryProperties UsesLibraryProperties
}

func (u *usesLibrary) deps(ctx android.BottomUpMutatorContext, hasFrameworkLibs bool) {
	if !ctx.Config().UnbundledBuild() {
		ctx.AddVariationDependencies(nil, usesLibTag, u.usesLibraryProperties.Uses_libs...)
		ctx.AddVariationDependencies(nil, usesLibTag, u.presentOptionalUsesLibs(ctx)...)
		// Only add these extra dependencies if the module depends on framework libs. This avoids
		// creating a cyclic dependency:
		//     e.g. framework-res -> org.apache.http.legacy -> ... -> framework-res.
		if hasFrameworkLibs {
			// dexpreopt/dexpreopt.go needs the paths to the dex jars of these libraries in case construct_context.sh needs
			// to pass them to dex2oat.  Add them as a dependency so we can determine the path to the dex jar of each
			// library to dexpreopt.
			ctx.AddVariationDependencies(nil, usesLibTag,
				"org.apache.http.legacy",
				"android.hidl.base-V1.0-java",
				"android.hidl.manager-V1.0-java")
		}
	}
}

// presentOptionalUsesLibs returns optional_uses_libs after filtering out MissingUsesLibraries, which don't exist in the
// build.
func (u *usesLibrary) presentOptionalUsesLibs(ctx android.BaseModuleContext) []string {
	optionalUsesLibs, _ := android.FilterList(u.usesLibraryProperties.Optional_uses_libs, ctx.Config().MissingUsesLibraries())
	return optionalUsesLibs
}

// usesLibraryPaths returns a map of module names of shared library dependencies to the paths to their dex jars.
func (u *usesLibrary) usesLibraryPaths(ctx android.ModuleContext) map[string]android.Path {
	usesLibPaths := make(map[string]android.Path)

	if !ctx.Config().UnbundledBuild() {
		ctx.VisitDirectDepsWithTag(usesLibTag, func(m android.Module) {
			if lib, ok := m.(Dependency); ok {
				if dexJar := lib.DexJar(); dexJar != nil {
					usesLibPaths[ctx.OtherModuleName(m)] = dexJar
				} else {
					ctx.ModuleErrorf("module %q in uses_libs or optional_uses_libs must produce a dex jar, does it have installable: true?",
						ctx.OtherModuleName(m))
				}
			} else if ctx.Config().AllowMissingDependencies() {
				ctx.AddMissingDependencies([]string{ctx.OtherModuleName(m)})
			} else {
				ctx.ModuleErrorf("module %q in uses_libs or optional_uses_libs must be a java library",
					ctx.OtherModuleName(m))
			}
		})
	}

	return usesLibPaths
}

// enforceUsesLibraries returns true of <uses-library> tags should be checked against uses_libs and optional_uses_libs
// properties.  Defaults to true if either of uses_libs or optional_uses_libs is specified.  Will default to true
// unconditionally in the future.
func (u *usesLibrary) enforceUsesLibraries() bool {
	defaultEnforceUsesLibs := len(u.usesLibraryProperties.Uses_libs) > 0 ||
		len(u.usesLibraryProperties.Optional_uses_libs) > 0
	return BoolDefault(u.usesLibraryProperties.Enforce_uses_libs, defaultEnforceUsesLibs)
}

// verifyUsesLibrariesManifest checks the <uses-library> tags in an AndroidManifest.xml against the ones specified
// in the uses_libs and optional_uses_libs properties.  It returns the path to a copy of the manifest.
func (u *usesLibrary) verifyUsesLibrariesManifest(ctx android.ModuleContext, manifest android.Path) android.Path {
	outputFile := android.PathForModuleOut(ctx, "manifest_check", "AndroidManifest.xml")

	rule := android.NewRuleBuilder()
	cmd := rule.Command().BuiltTool(ctx, "manifest_check").
		Flag("--enforce-uses-libraries").
		Input(manifest).
		FlagWithOutput("-o ", outputFile)

	for _, lib := range u.usesLibraryProperties.Uses_libs {
		cmd.FlagWithArg("--uses-library ", lib)
	}

	for _, lib := range u.usesLibraryProperties.Optional_uses_libs {
		cmd.FlagWithArg("--optional-uses-library ", lib)
	}

	rule.Build(pctx, ctx, "verify_uses_libraries", "verify <uses-library>")

	return outputFile
}

// verifyUsesLibrariesAPK checks the <uses-library> tags in the manifest of an APK against the ones specified
// in the uses_libs and optional_uses_libs properties.  It returns the path to a copy of the APK.
func (u *usesLibrary) verifyUsesLibrariesAPK(ctx android.ModuleContext, apk android.Path) android.Path {
	outputFile := android.PathForModuleOut(ctx, "verify_uses_libraries", apk.Base())

	rule := android.NewRuleBuilder()
	aapt := ctx.Config().HostToolPath(ctx, "aapt")
	rule.Command().
		Textf("aapt_binary=%s", aapt.String()).Implicit(aapt).
		Textf(`uses_library_names="%s"`, strings.Join(u.usesLibraryProperties.Uses_libs, " ")).
		Textf(`optional_uses_library_names="%s"`, strings.Join(u.usesLibraryProperties.Optional_uses_libs, " ")).
		Tool(android.PathForSource(ctx, "build/make/core/verify_uses_libraries.sh")).Input(apk)
	rule.Command().Text("cp -f").Input(apk).Output(outputFile)

	rule.Build(pctx, ctx, "verify_uses_libraries", "verify <uses-library>")

	return outputFile
}
