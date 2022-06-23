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

// This file contains the module implementations for android_app, android_test, and some more
// related module types, including their override variants.

import (
	"path/filepath"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/bazel"
	"android/soong/cc"
	"android/soong/dexpreopt"
	"android/soong/tradefed"
)

func init() {
	RegisterAppBuildComponents(android.InitRegistrationContext)
}

func RegisterAppBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("android_app", AndroidAppFactory)
	ctx.RegisterModuleType("android_test", AndroidTestFactory)
	ctx.RegisterModuleType("android_test_helper_app", AndroidTestHelperAppFactory)
	ctx.RegisterModuleType("android_app_certificate", AndroidAppCertificateFactory)
	ctx.RegisterModuleType("override_android_app", OverrideAndroidAppModuleFactory)
	ctx.RegisterModuleType("override_android_test", OverrideAndroidTestModuleFactory)
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

	// Name of the signing certificate lineage file or filegroup module.
	Lineage *string `android:"path"`

	// For overriding the --rotation-min-sdk-version property of apksig
	RotationMinSdkVersion *string

	// the package name of this app. The package name in the manifest file is used if one was not given.
	Package_name *string

	// the logging parent of this app.
	Logging_parent *string

	// Whether to rename the package in resources to the override name rather than the base name. Defaults to true.
	Rename_resources_package *bool

	// Names of modules to be overridden. Listed modules can only be other binaries
	// (in Make or Soong).
	// This does not completely prevent installation of the overridden binaries, but if both
	// binaries would be installed by default (in PRODUCT_PACKAGES) the other binary will be removed
	// from PRODUCT_PACKAGES.
	Overrides []string
}

type AndroidApp struct {
	android.BazelModuleBase
	Library
	aapt
	android.OverridableModuleBase

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

	overriddenManifestPackageName string

	android.ApexBundleDepsInfo

	javaApiUsedByOutputFile android.ModuleOutPath
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

	if String(a.appProperties.Stl) == "c++_shared" && !a.SdkVersion(ctx).Specified() {
		ctx.PropertyErrorf("stl", "sdk_version must be set in order to use c++_shared")
	}

	sdkDep := decodeSdkDep(ctx, android.SdkContext(a))
	if sdkDep.hasFrameworkLibs() {
		a.aapt.deps(ctx, sdkDep)
	}

	usesSDK := a.SdkVersion(ctx).Specified() && a.SdkVersion(ctx).Kind != android.SdkCorePlatform

	if usesSDK && Bool(a.appProperties.Jni_uses_sdk_apis) {
		ctx.PropertyErrorf("jni_uses_sdk_apis",
			"can only be set for modules that do not set sdk_version")
	} else if !usesSDK && Bool(a.appProperties.Jni_uses_platform_apis) {
		ctx.PropertyErrorf("jni_uses_platform_apis",
			"can only be set for modules that set sdk_version")
	}

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
		ctx.AddFarVariationDependencies(variation, jniLibTag, a.appProperties.Jni_libs...)
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
	a.generateJavaUsedByApex(ctx)
}

func (a *AndroidApp) checkAppSdkVersions(ctx android.ModuleContext) {
	if a.Updatable() {
		if !a.SdkVersion(ctx).Stable() {
			ctx.PropertyErrorf("sdk_version", "Updatable apps must use stable SDKs, found %v", a.SdkVersion(ctx))
		}
		if String(a.deviceProperties.Min_sdk_version) == "" {
			ctx.PropertyErrorf("updatable", "updatable apps must set min_sdk_version.")
		}

		if minSdkVersion, err := a.MinSdkVersion(ctx).EffectiveVersion(ctx); err == nil {
			a.checkJniLibsSdkVersion(ctx, minSdkVersion)
			android.CheckMinSdkVersion(ctx, minSdkVersion, a.WalkPayloadDeps)
		} else {
			ctx.PropertyErrorf("min_sdk_version", "%s", err.Error())
		}
	}

	a.checkPlatformAPI(ctx)
	a.checkSdkVersions(ctx)
}

// If an updatable APK sets min_sdk_version, min_sdk_vesion of JNI libs should match with it.
// This check is enforced for "updatable" APKs (including APK-in-APEX).
func (a *AndroidApp) checkJniLibsSdkVersion(ctx android.ModuleContext, minSdkVersion android.ApiLevel) {
	// It's enough to check direct JNI deps' sdk_version because all transitive deps from JNI deps are checked in cc.checkLinkType()
	ctx.VisitDirectDeps(func(m android.Module) {
		if !IsJniDepTag(ctx.OtherModuleDependencyTag(m)) {
			return
		}
		dep, _ := m.(*cc.Module)
		// The domain of cc.sdk_version is "current" and <number>
		// We can rely on android.SdkSpec to convert it to <number> so that "current" is
		// handled properly regardless of sdk finalization.
		jniSdkVersion, err := android.SdkSpecFrom(ctx, dep.MinSdkVersion()).EffectiveVersion(ctx)
		if err != nil || minSdkVersion.LessThan(jniSdkVersion) {
			ctx.OtherModuleErrorf(dep, "min_sdk_version(%v) is higher than min_sdk_version(%v) of the containing android_app(%v)",
				dep.MinSdkVersion(), minSdkVersion, ctx.ModuleName())
			return
		}

	})
}

// Returns true if the native libraries should be stored in the APK uncompressed and the
// extractNativeLibs application flag should be set to false in the manifest.
func (a *AndroidApp) useEmbeddedNativeLibs(ctx android.ModuleContext) bool {
	minSdkVersion, err := a.MinSdkVersion(ctx).EffectiveVersion(ctx)
	if err != nil {
		ctx.PropertyErrorf("min_sdk_version", "invalid value %q: %s", a.MinSdkVersion(ctx), err)
	}

	apexInfo := ctx.Provider(android.ApexInfoProvider).(android.ApexInfo)
	return (minSdkVersion.FinalOrFutureInt() >= 23 && Bool(a.appProperties.Use_embedded_native_libs)) ||
		!apexInfo.IsForPlatform()
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
	apexInfo := ctx.Provider(android.ApexInfoProvider).(android.ApexInfo)
	return ctx.Config().UnbundledBuild() || Bool(a.appProperties.Use_embedded_native_libs) ||
		!apexInfo.IsForPlatform() || a.appProperties.AlwaysPackageNativeLibs
}

func generateAaptRenamePackageFlags(packageName string, renameResourcesPackage bool) []string {
	aaptFlags := []string{"--rename-manifest-package " + packageName}
	if renameResourcesPackage {
		// Required to rename the package name in the resources table.
		aaptFlags = append(aaptFlags, "--rename-resources-package "+packageName)
	}
	return aaptFlags
}

func (a *AndroidApp) OverriddenManifestPackageName() string {
	return a.overriddenManifestPackageName
}

func (a *AndroidApp) renameResourcesPackage() bool {
	return proptools.BoolDefault(a.overridableAppProperties.Rename_resources_package, true)
}

func (a *AndroidApp) aaptBuildActions(ctx android.ModuleContext) {
	usePlatformAPI := proptools.Bool(a.Module.deviceProperties.Platform_apis)
	if ctx.Module().(android.SdkContext).SdkVersion(ctx).Kind == android.SdkModule {
		usePlatformAPI = true
	}
	a.aapt.usesNonSdkApis = usePlatformAPI

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
		aaptLinkFlags = append(aaptLinkFlags, generateAaptRenamePackageFlags(manifestPackageName, a.renameResourcesPackage())...)
		a.overriddenManifestPackageName = manifestPackageName
	}

	aaptLinkFlags = append(aaptLinkFlags, a.additionalAaptFlags...)

	a.aapt.splitNames = a.appProperties.Package_splits
	a.aapt.LoggingParent = String(a.overridableAppProperties.Logging_parent)
	a.aapt.buildActions(ctx, android.SdkContext(a), a.classLoaderContexts,
		a.usesLibraryProperties.Exclude_uses_libs, aaptLinkFlags...)

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
	a.dexpreopter.isApp = true
	if a.dexProperties.Uncompress_dex == nil {
		// If the value was not force-set by the user, use reasonable default based on the module.
		a.dexProperties.Uncompress_dex = proptools.BoolPtr(a.shouldUncompressDex(ctx))
	}
	a.dexpreopter.uncompressedDex = *a.dexProperties.Uncompress_dex
	a.dexpreopter.enforceUsesLibs = a.usesLibrary.enforceUsesLibraries()
	a.dexpreopter.classLoaderContexts = a.classLoaderContexts
	a.dexpreopter.manifestFile = a.mergedManifestFile
	a.dexpreopter.preventInstall = a.appProperties.PreventInstall

	if ctx.ModuleName() != "framework-res" {
		a.Module.compile(ctx, a.aaptSrcJar)
	}

	return a.dexJarFile.PathOrNil()
}

func (a *AndroidApp) jniBuildActions(jniLibs []jniLib, ctx android.ModuleContext) android.WritablePath {
	var jniJarFile android.WritablePath
	if len(jniLibs) > 0 {
		a.jniLibs = jniLibs
		if a.shouldEmbedJnis(ctx) {
			jniJarFile = android.PathForModuleOut(ctx, "jnilibs.zip")
			a.installPathForJNISymbols = a.installPath(ctx)
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
						ctx.Config().AndroidFirstDeviceTarget.Arch.ArchType == jni.target.Arch.ArchType {
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

	if !ctx.Provider(android.ApexInfoProvider).(android.ApexInfo).IsForPlatform() {
		a.hideApexVariantFromMake = true
	}

	a.aapt.useEmbeddedNativeLibs = a.useEmbeddedNativeLibs(ctx)
	a.aapt.useEmbeddedDex = Bool(a.appProperties.Use_embedded_dex)

	// Check if the install APK name needs to be overridden.
	a.installApkName = ctx.DeviceConfig().OverridePackageNameFor(a.Stem())

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

	a.classLoaderContexts = a.usesLibrary.classLoaderContextForUsesLibDeps(ctx)

	var noticeAssetPath android.WritablePath
	if Bool(a.appProperties.Embed_notices) || ctx.Config().IsEnvTrue("ALWAYS_EMBED_NOTICES") {
		// The rule to create the notice file can't be generated yet, as the final output path
		// for the apk isn't known yet.  Add the path where the notice file will be generated to the
		// aapt rules now before calling aaptBuildActions, the rule to create the notice file will
		// be generated later.
		noticeAssetPath = android.PathForModuleOut(ctx, "NOTICE", "NOTICE.html.gz")
		a.aapt.noticeFile = android.OptionalPathForPath(noticeAssetPath)
	}

	// Process all building blocks, from AAPT to certificates.
	a.aaptBuildActions(ctx)

	// The decision to enforce <uses-library> checks is made before adding implicit SDK libraries.
	a.usesLibrary.freezeEnforceUsesLibraries()

	// Add implicit SDK libraries to <uses-library> list.
	requiredUsesLibs, optionalUsesLibs := a.classLoaderContexts.UsesLibs()
	for _, usesLib := range requiredUsesLibs {
		a.usesLibrary.addLib(usesLib, false)
	}
	for _, usesLib := range optionalUsesLibs {
		a.usesLibrary.addLib(usesLib, true)
	}

	// Check that the <uses-library> list is coherent with the manifest.
	if a.usesLibrary.enforceUsesLibraries() {
		manifestCheckFile := a.usesLibrary.verifyUsesLibrariesManifest(ctx, a.mergedManifestFile)
		apkDeps = append(apkDeps, manifestCheckFile)
	}

	a.proguardBuildActions(ctx)

	a.linter.mergedManifest = a.aapt.mergedManifestFile
	a.linter.manifest = a.aapt.manifestPath
	a.linter.resources = a.aapt.resourceFiles
	a.linter.buildModuleReportZip = ctx.Config().UnbundledBuildApps()

	dexJarFile := a.dexBuildActions(ctx)

	jniLibs, certificateDeps := collectAppDeps(ctx, a, a.shouldEmbedJnis(ctx), !Bool(a.appProperties.Jni_uses_platform_apis))
	jniJarFile := a.jniBuildActions(jniLibs, ctx)

	if ctx.Failed() {
		return
	}

	certificates := processMainCert(a.ModuleBase, a.getCertString(ctx), certificateDeps, ctx)

	// This can be reached with an empty certificate list if AllowMissingDependencies is set
	// and the certificate property for this module is a module reference to a missing module.
	if len(certificates) > 0 {
		a.certificate = certificates[0]
	} else {
		if !ctx.Config().AllowMissingDependencies() && len(ctx.GetMissingDependencies()) > 0 {
			panic("Should only get here if AllowMissingDependencies set and there are missing dependencies")
		}
		// Set a certificate to avoid panics later when accessing it.
		a.certificate = Certificate{
			Key: android.PathForModuleOut(ctx, "missing.pk8"),
			Pem: android.PathForModuleOut(ctx, "missing.pem"),
		}
	}

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

	rotationMinSdkVersion := String(a.overridableAppProperties.RotationMinSdkVersion)

	CreateAndSignAppPackage(ctx, packageFile, a.exportPackage, jniJarFile, dexJarFile, certificates, apkDeps, v4SignatureFile, lineageFile, rotationMinSdkVersion)
	a.outputFile = packageFile
	if v4SigningRequested {
		a.extraOutputFiles = append(a.extraOutputFiles, v4SignatureFile)
	}

	if a.aapt.noticeFile.Valid() {
		// Generating the notice file rule has to be here after a.outputFile is known.
		noticeFile := android.PathForModuleOut(ctx, "NOTICE.html.gz")
		android.BuildNoticeHtmlOutputFromLicenseMetadata(
			ctx, noticeFile, "", "",
			[]string{
				a.installDir.String() + "/",
				android.PathForModuleInstall(ctx).String() + "/",
				a.outputFile.String(),
			})
		builder := android.NewRuleBuilder(pctx, ctx)
		builder.Command().Text("cp").
			Input(noticeFile).
			Output(noticeAssetPath)
		builder.Build("notice_dir", "Building notice dir")
	}

	for _, split := range a.aapt.splits {
		// Sign the split APKs
		packageFile := android.PathForModuleOut(ctx, a.installApkName+"_"+split.suffix+".apk")
		if v4SigningRequested {
			v4SignatureFile = android.PathForModuleOut(ctx, a.installApkName+"_"+split.suffix+".apk.idsig")
		}
		CreateAndSignAppPackage(ctx, packageFile, split.path, nil, nil, certificates, apkDeps, v4SignatureFile, lineageFile, rotationMinSdkVersion)
		a.extraOutputFiles = append(a.extraOutputFiles, packageFile)
		if v4SigningRequested {
			a.extraOutputFiles = append(a.extraOutputFiles, v4SignatureFile)
		}
	}

	// Build an app bundle.
	bundleFile := android.PathForModuleOut(ctx, "base.zip")
	BuildBundleModule(ctx, bundleFile, a.exportPackage, jniJarFile, dexJarFile)
	a.bundleFile = bundleFile

	apexInfo := ctx.Provider(android.ApexInfoProvider).(android.ApexInfo)

	// Install the app package.
	if (Bool(a.Module.properties.Installable) || ctx.Host()) && apexInfo.IsForPlatform() &&
		!a.appProperties.PreventInstall {

		var extraInstalledPaths android.Paths
		for _, extra := range a.extraOutputFiles {
			installed := ctx.InstallFile(a.installDir, extra.Base(), extra)
			extraInstalledPaths = append(extraInstalledPaths, installed)
		}
		ctx.InstallFile(a.installDir, a.outputFile.Base(), a.outputFile, extraInstalledPaths...)
	}

	a.buildAppDependencyInfo(ctx)
}

type appDepsInterface interface {
	SdkVersion(ctx android.EarlyModuleContext) android.SdkSpec
	MinSdkVersion(ctx android.EarlyModuleContext) android.SdkSpec
	RequiresStableAPIs(ctx android.BaseModuleContext) bool
}

func collectAppDeps(ctx android.ModuleContext, app appDepsInterface,
	shouldCollectRecursiveNativeDeps bool,
	checkNativeSdkVersion bool) ([]jniLib, []Certificate) {

	var jniLibs []jniLib
	var certificates []Certificate
	seenModulePaths := make(map[string]bool)

	if checkNativeSdkVersion {
		checkNativeSdkVersion = app.SdkVersion(ctx).Specified() &&
			app.SdkVersion(ctx).Kind != android.SdkCorePlatform && !app.RequiresStableAPIs(ctx)
	}

	ctx.WalkDeps(func(module android.Module, parent android.Module) bool {
		otherName := ctx.OtherModuleName(module)
		tag := ctx.OtherModuleDependencyTag(module)

		if IsJniDepTag(tag) || cc.IsSharedDepTag(tag) {
			if dep, ok := module.(*cc.Module); ok {
				if dep.IsNdk(ctx.Config()) || dep.IsStubs() {
					return false
				}

				lib := dep.OutputFile()
				if lib.Valid() {
					path := lib.Path()
					if seenModulePaths[path.String()] {
						return false
					}
					seenModulePaths[path.String()] = true

					if checkNativeSdkVersion && dep.SdkVersion() == "" {
						ctx.PropertyErrorf("jni_libs", "JNI dependency %q uses platform APIs, but this module does not",
							otherName)
					}

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

func (a *AndroidApp) WalkPayloadDeps(ctx android.ModuleContext, do android.PayloadDepsCallback) {
	ctx.WalkDeps(func(child, parent android.Module) bool {
		isExternal := !a.DepIsInSameApex(ctx, child)
		if am, ok := child.(android.ApexModule); ok {
			if !do(ctx, parent, am, isExternal) {
				return false
			}
		}
		return !isExternal
	})
}

func (a *AndroidApp) buildAppDependencyInfo(ctx android.ModuleContext) {
	if ctx.Host() {
		return
	}

	depsInfo := android.DepNameToDepInfoMap{}
	a.WalkPayloadDeps(ctx, func(ctx android.ModuleContext, from blueprint.Module, to android.ApexModule, externalDep bool) bool {
		depName := to.Name()

		// Skip dependencies that are only available to APEXes; they are developed with updatability
		// in mind and don't need manual approval.
		if to.(android.ApexModule).NotAvailableForPlatform() {
			return true
		}

		if info, exist := depsInfo[depName]; exist {
			info.From = append(info.From, from.Name())
			info.IsExternal = info.IsExternal && externalDep
			depsInfo[depName] = info
		} else {
			toMinSdkVersion := "(no version)"
			if m, ok := to.(interface {
				MinSdkVersion(ctx android.EarlyModuleContext) android.SdkSpec
			}); ok {
				if v := m.MinSdkVersion(ctx); !v.ApiLevel.IsNone() {
					toMinSdkVersion = v.ApiLevel.String()
				}
			} else if m, ok := to.(interface{ MinSdkVersion() string }); ok {
				// TODO(b/175678607) eliminate the use of MinSdkVersion returning
				// string
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
		return true
	})

	a.ApexBundleDepsInfo.BuildDepsInfoLists(ctx, a.MinSdkVersion(ctx).String(), depsInfo)
}

func (a *AndroidApp) Updatable() bool {
	return Bool(a.appProperties.Updatable)
}

func (a *AndroidApp) SetUpdatable(val bool) {
	a.appProperties.Updatable = &val
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

func (a *AndroidApp) SetPreventInstall() {
	a.appProperties.PreventInstall = true
}

func (a *AndroidApp) MarkAsCoverageVariant(coverage bool) {
	a.appProperties.IsCoverageVariant = coverage
}

func (a *AndroidApp) EnableCoverageIfNeeded() {}

var _ cc.Coverage = (*AndroidApp)(nil)

// android_app compiles sources and Android resources into an Android application package `.apk` file.
func AndroidAppFactory() android.Module {
	module := &AndroidApp{}

	module.Module.dexProperties.Optimize.EnabledByDefault = true
	module.Module.dexProperties.Optimize.Shrink = proptools.BoolPtr(true)

	module.Module.properties.Instrument = true
	module.Module.properties.Supports_static_instrumentation = true
	module.Module.properties.Installable = proptools.BoolPtr(true)

	module.addHostAndDeviceProperties()
	module.AddProperties(
		&module.aaptProperties,
		&module.appProperties,
		&module.overridableAppProperties)

	module.usesLibrary.enforce = true

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitOverridableModule(module, &module.overridableAppProperties.Overrides)
	android.InitApexModule(module)
	android.InitBazelModule(module)

	return module
}

type appTestProperties struct {
	// The name of the android_app module that the tests will run against.
	Instrumentation_for *string

	// if specified, the instrumentation target package name in the manifest is overwritten by it.
	Instrumentation_target_package *string
}

type AndroidTest struct {
	AndroidApp

	appTestProperties appTestProperties

	testProperties testProperties

	testConfig       android.Path
	extraTestConfigs android.Paths
	data             android.Paths
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
	a.extraTestConfigs = android.PathsForModuleSrc(ctx, a.testProperties.Test_options.Extra_test_configs)
	a.data = android.PathsForModuleSrc(ctx, a.testProperties.Data)
}

func (a *AndroidTest) FixTestConfig(ctx android.ModuleContext, testConfig android.Path) android.Path {
	if testConfig == nil {
		return nil
	}

	fixedConfig := android.PathForModuleOut(ctx, "test_config_fixer", "AndroidTest.xml")
	rule := android.NewRuleBuilder(pctx, ctx)
	command := rule.Command().BuiltTool("test_config_fixer").Input(testConfig).Output(fixedConfig)
	fixNeeded := false

	// Auto-generated test config uses `ModuleName` as the APK name. So fix it if it is not the case.
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
		rule.Build("fix_test_config", "fix test config")
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
	module.Module.properties.Supports_static_instrumentation = true
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
		&module.testProperties)

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitOverridableModule(module, &module.overridableAppProperties.Overrides)
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

	// Install the test into a folder named for the module in all test suites.
	Per_testcase_directory *bool
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
		&module.overridableAppProperties)

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitApexModule(module)
	return module
}

type AndroidAppCertificate struct {
	android.ModuleBase
	android.BazelModuleBase

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
	android.InitBazelModule(module)
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
	m.AddProperties(
		&OverridableDeviceProperties{},
		&overridableAppProperties{},
	)

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

type UsesLibraryProperties struct {
	// A list of shared library modules that will be listed in uses-library tags in the AndroidManifest.xml file.
	Uses_libs []string

	// A list of shared library modules that will be listed in uses-library tags in the AndroidManifest.xml file with
	// required=false.
	Optional_uses_libs []string

	// If true, the list of uses_libs and optional_uses_libs modules must match the AndroidManifest.xml file.  Defaults
	// to true if either uses_libs or optional_uses_libs is set.  Will unconditionally default to true in the future.
	Enforce_uses_libs *bool

	// Optional name of the <uses-library> provided by this module. This is needed for non-SDK
	// libraries, because SDK ones are automatically picked up by Soong. The <uses-library> name
	// normally is the same as the module name, but there are exceptions.
	Provides_uses_lib *string

	// A list of shared library names to exclude from the classpath of the APK. Adding a library here
	// will prevent it from being used when precompiling the APK and prevent it from being implicitly
	// added to the APK's manifest's <uses-library> elements.
	//
	// Care must be taken when using this as it could result in runtime errors if the APK actually
	// uses classes provided by the library and which are not provided in any other way.
	//
	// This is primarily intended for use by various CTS tests that check the runtime handling of the
	// android.test.base shared library (and related libraries) but which depend on some common
	// libraries that depend on the android.test.base library. Without this those tests will end up
	// with a <uses-library android:name="android.test.base"/> in their manifest which would either
	// render the tests worthless (as they would be testing the wrong behavior), or would break the
	// test altogether by providing access to classes that the tests were not expecting. Those tests
	// provide the android.test.base statically and use jarjar to rename them so they do not collide
	// with the classes provided by the android.test.base library.
	Exclude_uses_libs []string
}

// usesLibrary provides properties and helper functions for AndroidApp and AndroidAppImport to verify that the
// <uses-library> tags that end up in the manifest of an APK match the ones known to the build system through the
// uses_libs and optional_uses_libs properties.  The build system's values are used by dexpreopt to preopt apps
// with knowledge of their shared libraries.
type usesLibrary struct {
	usesLibraryProperties UsesLibraryProperties

	// Whether to enforce verify_uses_library check.
	enforce bool
}

func (u *usesLibrary) addLib(lib string, optional bool) {
	if !android.InList(lib, u.usesLibraryProperties.Uses_libs) && !android.InList(lib, u.usesLibraryProperties.Optional_uses_libs) {
		if optional {
			u.usesLibraryProperties.Optional_uses_libs = append(u.usesLibraryProperties.Optional_uses_libs, lib)
		} else {
			u.usesLibraryProperties.Uses_libs = append(u.usesLibraryProperties.Uses_libs, lib)
		}
	}
}

func (u *usesLibrary) deps(ctx android.BottomUpMutatorContext, hasFrameworkLibs bool) {
	if !ctx.Config().UnbundledBuild() || ctx.Config().UnbundledBuildImage() {
		reqTag := makeUsesLibraryDependencyTag(dexpreopt.AnySdkVersion, false, false)
		ctx.AddVariationDependencies(nil, reqTag, u.usesLibraryProperties.Uses_libs...)

		optTag := makeUsesLibraryDependencyTag(dexpreopt.AnySdkVersion, true, false)
		ctx.AddVariationDependencies(nil, optTag, u.presentOptionalUsesLibs(ctx)...)

		// Only add these extra dependencies if the module depends on framework libs. This avoids
		// creating a cyclic dependency:
		//     e.g. framework-res -> org.apache.http.legacy -> ... -> framework-res.
		if hasFrameworkLibs {
			// Add implicit <uses-library> dependencies on compatibility libraries. Some of them are
			// optional, and some required --- this depends on the most common usage of the library
			// and may be wrong for some apps (they need explicit `uses_libs`/`optional_uses_libs`).

			compat28OptTag := makeUsesLibraryDependencyTag(28, true, true)
			ctx.AddVariationDependencies(nil, compat28OptTag, dexpreopt.OptionalCompatUsesLibs28...)

			compat29ReqTag := makeUsesLibraryDependencyTag(29, false, true)
			ctx.AddVariationDependencies(nil, compat29ReqTag, dexpreopt.CompatUsesLibs29...)

			compat30OptTag := makeUsesLibraryDependencyTag(30, true, true)
			ctx.AddVariationDependencies(nil, compat30OptTag, dexpreopt.OptionalCompatUsesLibs30...)
		}
	}
}

// presentOptionalUsesLibs returns optional_uses_libs after filtering out MissingUsesLibraries, which don't exist in the
// build.
func (u *usesLibrary) presentOptionalUsesLibs(ctx android.BaseModuleContext) []string {
	optionalUsesLibs, _ := android.FilterList(u.usesLibraryProperties.Optional_uses_libs, ctx.Config().MissingUsesLibraries())
	return optionalUsesLibs
}

// Helper function to replace string in a list.
func replaceInList(list []string, oldstr, newstr string) {
	for i, str := range list {
		if str == oldstr {
			list[i] = newstr
		}
	}
}

// Returns a map of module names of shared library dependencies to the paths to their dex jars on
// host and on device.
func (u *usesLibrary) classLoaderContextForUsesLibDeps(ctx android.ModuleContext) dexpreopt.ClassLoaderContextMap {
	clcMap := make(dexpreopt.ClassLoaderContextMap)

	// Skip when UnbundledBuild() is true, but UnbundledBuildImage() is false. With
	// UnbundledBuildImage() it is necessary to generate dexpreopt.config for post-dexpreopting.
	if ctx.Config().UnbundledBuild() && !ctx.Config().UnbundledBuildImage() {
		return clcMap
	}

	ctx.VisitDirectDeps(func(m android.Module) {
		tag, isUsesLibTag := ctx.OtherModuleDependencyTag(m).(usesLibraryDependencyTag)
		if !isUsesLibTag {
			return
		}

		dep := android.RemoveOptionalPrebuiltPrefix(ctx.OtherModuleName(m))

		// Skip stub libraries. A dependency on the implementation library has been added earlier,
		// so it will be added to CLC, but the stub shouldn't be. Stub libraries can be distingushed
		// from implementation libraries by their name, which is different as it has a suffix.
		if comp, ok := m.(SdkLibraryComponentDependency); ok {
			if impl := comp.OptionalSdkLibraryImplementation(); impl != nil && *impl != dep {
				return
			}
		}

		if lib, ok := m.(UsesLibraryDependency); ok {
			libName := dep
			if ulib, ok := m.(ProvidesUsesLib); ok && ulib.ProvidesUsesLib() != nil {
				libName = *ulib.ProvidesUsesLib()
				// Replace module name with library name in `uses_libs`/`optional_uses_libs` in
				// order to pass verify_uses_libraries check (which compares these properties
				// against library names written in the manifest).
				replaceInList(u.usesLibraryProperties.Uses_libs, dep, libName)
				replaceInList(u.usesLibraryProperties.Optional_uses_libs, dep, libName)
			}
			clcMap.AddContext(ctx, tag.sdkVersion, libName, tag.optional, tag.implicit,
				lib.DexJarBuildPath().PathOrNil(), lib.DexJarInstallPath(),
				lib.ClassLoaderContexts())
		} else if ctx.Config().AllowMissingDependencies() {
			ctx.AddMissingDependencies([]string{dep})
		} else {
			ctx.ModuleErrorf("module %q in uses_libs or optional_uses_libs must be a java library", dep)
		}
	})
	return clcMap
}

// enforceUsesLibraries returns true of <uses-library> tags should be checked against uses_libs and optional_uses_libs
// properties.  Defaults to true if either of uses_libs or optional_uses_libs is specified.  Will default to true
// unconditionally in the future.
func (u *usesLibrary) enforceUsesLibraries() bool {
	defaultEnforceUsesLibs := len(u.usesLibraryProperties.Uses_libs) > 0 ||
		len(u.usesLibraryProperties.Optional_uses_libs) > 0
	return BoolDefault(u.usesLibraryProperties.Enforce_uses_libs, u.enforce || defaultEnforceUsesLibs)
}

// Freeze the value of `enforce_uses_libs` based on the current values of `uses_libs` and `optional_uses_libs`.
func (u *usesLibrary) freezeEnforceUsesLibraries() {
	enforce := u.enforceUsesLibraries()
	u.usesLibraryProperties.Enforce_uses_libs = &enforce
}

// verifyUsesLibraries checks the <uses-library> tags in the manifest against the ones specified
// in the `uses_libs`/`optional_uses_libs` properties. The input can be either an XML manifest, or
// an APK with the manifest embedded in it (manifest_check will know which one it is by the file
// extension: APKs are supposed to end with '.apk').
func (u *usesLibrary) verifyUsesLibraries(ctx android.ModuleContext, inputFile android.Path,
	outputFile android.WritablePath) android.Path {

	statusFile := dexpreopt.UsesLibrariesStatusFile(ctx)

	// Disable verify_uses_libraries check if dexpreopt is globally disabled. Without dexpreopt the
	// check is not necessary, and although it is good to have, it is difficult to maintain on
	// non-linux build platforms where dexpreopt is generally disabled (the check may fail due to
	// various unrelated reasons, such as a failure to get manifest from an APK).
	global := dexpreopt.GetGlobalConfig(ctx)
	if global.DisablePreopt || global.OnlyPreoptBootImageAndSystemServer {
		return inputFile
	}

	rule := android.NewRuleBuilder(pctx, ctx)
	cmd := rule.Command().BuiltTool("manifest_check").
		Flag("--enforce-uses-libraries").
		Input(inputFile).
		FlagWithOutput("--enforce-uses-libraries-status ", statusFile).
		FlagWithInput("--aapt ", ctx.Config().HostToolPath(ctx, "aapt"))

	if outputFile != nil {
		cmd.FlagWithOutput("-o ", outputFile)
	}

	if dexpreopt.GetGlobalConfig(ctx).RelaxUsesLibraryCheck {
		cmd.Flag("--enforce-uses-libraries-relax")
	}

	for _, lib := range u.usesLibraryProperties.Uses_libs {
		cmd.FlagWithArg("--uses-library ", lib)
	}

	for _, lib := range u.usesLibraryProperties.Optional_uses_libs {
		cmd.FlagWithArg("--optional-uses-library ", lib)
	}

	rule.Build("verify_uses_libraries", "verify <uses-library>")
	return outputFile
}

// verifyUsesLibrariesManifest checks the <uses-library> tags in an AndroidManifest.xml against
// the build system and returns the path to a copy of the manifest.
func (u *usesLibrary) verifyUsesLibrariesManifest(ctx android.ModuleContext, manifest android.Path) android.Path {
	outputFile := android.PathForModuleOut(ctx, "manifest_check", "AndroidManifest.xml")
	return u.verifyUsesLibraries(ctx, manifest, outputFile)
}

// verifyUsesLibrariesAPK checks the <uses-library> tags in the manifest of an APK against the build
// system and returns the path to a copy of the APK.
func (u *usesLibrary) verifyUsesLibrariesAPK(ctx android.ModuleContext, apk android.Path) android.Path {
	u.verifyUsesLibraries(ctx, apk, nil) // for APKs manifest_check does not write output file
	outputFile := android.PathForModuleOut(ctx, "verify_uses_libraries", apk.Base())
	return outputFile
}

// For Bazel / bp2build

type bazelAndroidAppCertificateAttributes struct {
	Certificate string
}

func (m *AndroidAppCertificate) ConvertWithBp2build(ctx android.TopDownMutatorContext) {
	androidAppCertificateBp2Build(ctx, m)
}

func androidAppCertificateBp2Build(ctx android.TopDownMutatorContext, module *AndroidAppCertificate) {
	var certificate string
	if module.properties.Certificate != nil {
		certificate = *module.properties.Certificate
	}

	attrs := &bazelAndroidAppCertificateAttributes{
		Certificate: certificate,
	}

	props := bazel.BazelTargetModuleProperties{
		Rule_class:        "android_app_certificate",
		Bzl_load_location: "//build/bazel/rules/android:android_app_certificate.bzl",
	}

	ctx.CreateBazelTargetModule(props, android.CommonAttributes{Name: module.Name()}, attrs)
}

type bazelAndroidAppAttributes struct {
	*javaCommonAttributes
	Deps             bazel.LabelListAttribute
	Manifest         bazel.Label
	Custom_package   *string
	Resource_files   bazel.LabelListAttribute
	Certificate      *bazel.Label
	Certificate_name *string
}

// ConvertWithBp2build is used to convert android_app to Bazel.
func (a *AndroidApp) ConvertWithBp2build(ctx android.TopDownMutatorContext) {
	commonAttrs, depLabels := a.convertLibraryAttrsBp2Build(ctx)

	deps := depLabels.Deps
	if !commonAttrs.Srcs.IsEmpty() {
		deps.Append(depLabels.StaticDeps) // we should only append these if there are sources to use them
	} else if !deps.IsEmpty() || !depLabels.StaticDeps.IsEmpty() {
		ctx.ModuleErrorf("android_app has dynamic or static dependencies but no sources." +
			" Bazel does not allow direct dependencies without sources nor exported" +
			" dependencies on android_binary rule.")
	}

	manifest := proptools.StringDefault(a.aaptProperties.Manifest, "AndroidManifest.xml")

	resourceFiles := bazel.LabelList{
		Includes: []bazel.Label{},
	}
	for _, dir := range android.PathsWithOptionalDefaultForModuleSrc(ctx, a.aaptProperties.Resource_dirs, "res") {
		files := android.RootToModuleRelativePaths(ctx, androidResourceGlob(ctx, dir))
		resourceFiles.Includes = append(resourceFiles.Includes, files...)
	}

	var certificate *bazel.Label
	certificateNamePtr := a.overridableAppProperties.Certificate
	certificateName := proptools.StringDefault(certificateNamePtr, "")
	certModule := android.SrcIsModule(certificateName)
	if certModule != "" {
		c := android.BazelLabelForModuleDepSingle(ctx, certificateName)
		certificate = &c
		certificateNamePtr = nil
	}

	attrs := &bazelAndroidAppAttributes{
		commonAttrs,
		deps,
		android.BazelLabelForModuleSrcSingle(ctx, manifest),
		// TODO(b/209576404): handle package name override by product variable PRODUCT_MANIFEST_PACKAGE_NAME_OVERRIDES
		a.overridableAppProperties.Package_name,
		bazel.MakeLabelListAttribute(resourceFiles),
		certificate,
		certificateNamePtr,
	}

	props := bazel.BazelTargetModuleProperties{
		Rule_class:        "android_binary",
		Bzl_load_location: "//build/bazel/rules/android:android_binary.bzl",
	}

	ctx.CreateBazelTargetModule(props, android.CommonAttributes{Name: a.Name()}, attrs)

}
