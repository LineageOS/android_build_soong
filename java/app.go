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
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/tradefed"
)

func init() {
	android.RegisterModuleType("android_app", AndroidAppFactory)
	android.RegisterModuleType("android_test", AndroidTestFactory)
	android.RegisterModuleType("android_test_helper_app", AndroidTestHelperAppFactory)
	android.RegisterModuleType("android_app_certificate", AndroidAppCertificateFactory)
	android.RegisterModuleType("override_android_app", OverrideAndroidAppModuleFactory)
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

	// Store native libraries uncompressed in the APK and set the android:extractNativeLibs="false" manifest
	// flag so that they are used from inside the APK at runtime.  Defaults to true for android_test modules unless
	// sdk_version or min_sdk_version is set to a version that doesn't support it (<23), defaults to false for other
	// module types where the native libraries are generally preinstalled outside the APK.
	Use_embedded_native_libs *bool

	// Store dex files uncompressed in the APK and set the android:useEmbeddedDex="true" manifest attribute so that
	// they are used from inside the APK at runtime.
	Use_embedded_dex *bool

	// Forces native libraries to always be packaged into the APK,
	// Use_embedded_native_libs still selects whether they are stored uncompressed and aligned or compressed.
	// True for android_test* modules.
	AlwaysPackageNativeLibs bool `blueprint:"mutated"`
}

// android_app properties that can be overridden by override_android_app
type overridableAppProperties struct {
	// The name of a certificate in the default certificate directory, blank to use the default product certificate,
	// or an android_app_certificate module name in the form ":module".
	Certificate *string

	// the package name of this app. The package name in the manifest file is used if one was not given.
	Package_name *string
}

type AndroidApp struct {
	Library
	aapt
	android.OverridableModuleBase

	certificate Certificate

	appProperties appProperties

	overridableAppProperties overridableAppProperties

	installJniLibs []jniLib

	bundleFile android.Path

	// the install APK name is normally the same as the module name, but can be overridden with PRODUCT_PACKAGE_NAME_OVERRIDES.
	installApkName string

	additionalAaptFlags []string
}

func (a *AndroidApp) ExportedProguardFlagFiles() android.Paths {
	return nil
}

func (a *AndroidApp) ExportedStaticPackages() android.Paths {
	return nil
}

var _ AndroidLibraryDependency = (*AndroidApp)(nil)

type Certificate struct {
	Pem, Key android.Path
}

func (a *AndroidApp) DepsMutator(ctx android.BottomUpMutatorContext) {
	a.Module.deps(ctx)

	if !Bool(a.properties.No_framework_libs) && !Bool(a.properties.No_standard_libs) {
		a.aapt.deps(ctx, sdkContext(a))
	}

	for _, jniTarget := range ctx.MultiTargets() {
		variation := []blueprint.Variation{
			{Mutator: "arch", Variation: jniTarget.String()},
			{Mutator: "link", Variation: "shared"},
		}
		tag := &jniDependencyTag{
			target: jniTarget,
		}
		ctx.AddFarVariationDependencies(variation, tag, a.appProperties.Jni_libs...)
	}

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

func (a *AndroidApp) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	a.aapt.uncompressedJNI = a.shouldUncompressJNI(ctx)
	a.aapt.useEmbeddedDex = Bool(a.appProperties.Use_embedded_dex)
	a.generateAndroidBuildActions(ctx)
}

// shouldUncompressJNI returns true if the native libraries should be stored in the APK uncompressed and the
// extractNativeLibs application flag should be set to false in the manifest.
func (a *AndroidApp) shouldUncompressJNI(ctx android.ModuleContext) bool {
	minSdkVersion, err := sdkVersionToNumber(ctx, a.minSdkVersion())
	if err != nil {
		ctx.PropertyErrorf("min_sdk_version", "invalid value %q: %s", a.minSdkVersion(), err)
	}

	return minSdkVersion >= 23 && Bool(a.appProperties.Use_embedded_native_libs)
}

// Returns whether this module should have the dex file stored uncompressed in the APK.
func (a *AndroidApp) shouldUncompressDex(ctx android.ModuleContext) bool {
	if Bool(a.appProperties.Use_embedded_dex) {
		return true
	}

	if ctx.Config().UnbundledBuild() {
		return false
	}

	// Uncompress dex in APKs of privileged apps, and modules used by privileged apps.
	if ctx.Config().UncompressPrivAppDex() &&
		(Bool(a.appProperties.Privileged) ||
			inList(ctx.ModuleName(), ctx.Config().ModulesLoadedByPrivilegedModules())) {
		return true
	}

	// Uncompress if the dex files is preopted on /system.
	if !a.dexpreopter.dexpreoptDisabled(ctx) && (ctx.Host() || !odexOnSystemOther(ctx, a.dexpreopter.installPath)) {
		return true
	}

	return false
}

func (a *AndroidApp) aaptBuildActions(ctx android.ModuleContext) {
	a.aapt.usesNonSdkApis = Bool(a.Module.deviceProperties.Platform_apis)

	aaptLinkFlags := []string{}

	// Add TARGET_AAPT_CHARACTERISTICS values to AAPT link flags if they exist and --product flags were not provided.
	hasProduct := false
	for _, f := range a.aaptProperties.Aaptflags {
		if strings.HasPrefix(f, "--product") {
			hasProduct = true
			break
		}
	}
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
	}

	aaptLinkFlags = append(aaptLinkFlags, a.additionalAaptFlags...)

	a.aapt.splitNames = a.appProperties.Package_splits

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

func (a *AndroidApp) dexBuildActions(ctx android.ModuleContext) android.Path {

	var installDir string
	if ctx.ModuleName() == "framework-res" {
		// framework-res.apk is installed as system/framework/framework-res.apk
		installDir = "framework"
	} else if Bool(a.appProperties.Privileged) {
		installDir = filepath.Join("priv-app", a.installApkName)
	} else {
		installDir = filepath.Join("app", a.installApkName)
	}
	a.dexpreopter.installPath = android.PathForModuleInstall(ctx, installDir, a.installApkName+".apk")
	a.dexpreopter.isInstallable = Bool(a.properties.Installable)
	a.dexpreopter.uncompressedDex = a.shouldUncompressDex(ctx)
	a.deviceProperties.UncompressDex = a.dexpreopter.uncompressedDex

	if ctx.ModuleName() != "framework-res" {
		a.Module.compile(ctx, a.aaptSrcJar)
	}

	return a.maybeStrippedDexJarFile
}

func (a *AndroidApp) jniBuildActions(jniLibs []jniLib, ctx android.ModuleContext) android.WritablePath {
	var jniJarFile android.WritablePath
	if len(jniLibs) > 0 {
		embedJni := ctx.Config().UnbundledBuild() || Bool(a.appProperties.Use_embedded_native_libs) ||
			a.appProperties.AlwaysPackageNativeLibs
		if embedJni {
			jniJarFile = android.PathForModuleOut(ctx, "jnilibs.zip")
			TransformJniLibsToJar(ctx, jniJarFile, jniLibs, a.shouldUncompressJNI(ctx))
		} else {
			a.installJniLibs = jniLibs
		}
	}
	return jniJarFile
}

func (a *AndroidApp) certificateBuildActions(certificateDeps []Certificate, ctx android.ModuleContext) []Certificate {
	cert := a.getCertString(ctx)
	certModule := android.SrcIsModule(cert)
	if certModule != "" {
		a.certificate = certificateDeps[0]
		certificateDeps = certificateDeps[1:]
	} else if cert != "" {
		defaultDir := ctx.Config().DefaultAppCertificateDir(ctx)
		a.certificate = Certificate{
			defaultDir.Join(ctx, cert+".x509.pem"),
			defaultDir.Join(ctx, cert+".pk8"),
		}
	} else {
		pem, key := ctx.Config().DefaultAppCertificate(ctx)
		a.certificate = Certificate{pem, key}
	}

	if !a.Module.Platform() {
		certPath := a.certificate.Pem.String()
		systemCertPath := ctx.Config().DefaultAppCertificateDir(ctx).String()
		if strings.HasPrefix(certPath, systemCertPath) {
			enforceSystemCert := ctx.Config().EnforceSystemCertificate()
			whitelist := ctx.Config().EnforceSystemCertificateWhitelist()

			if enforceSystemCert && !inList(a.Module.Name(), whitelist) {
				ctx.PropertyErrorf("certificate", "The module in product partition cannot be signed with certificate in system.")
			}
		}
	}

	return append([]Certificate{a.certificate}, certificateDeps...)
}

func (a *AndroidApp) generateAndroidBuildActions(ctx android.ModuleContext) {
	// Check if the install APK name needs to be overridden.
	a.installApkName = ctx.DeviceConfig().OverridePackageNameFor(a.Name())

	// Process all building blocks, from AAPT to certificates.
	a.aaptBuildActions(ctx)

	a.proguardBuildActions(ctx)

	dexJarFile := a.dexBuildActions(ctx)

	jniLibs, certificateDeps := a.collectAppDeps(ctx)
	jniJarFile := a.jniBuildActions(jniLibs, ctx)

	if ctx.Failed() {
		return
	}

	certificates := a.certificateBuildActions(certificateDeps, ctx)

	// Build a final signed app package.
	// TODO(jungjw): Consider changing this to installApkName.
	packageFile := android.PathForModuleOut(ctx, ctx.ModuleName()+".apk")
	CreateAppPackage(ctx, packageFile, a.exportPackage, jniJarFile, dexJarFile, certificates)
	a.outputFile = packageFile

	for _, split := range a.aapt.splits {
		// Sign the split APKs
		packageFile := android.PathForModuleOut(ctx, ctx.ModuleName()+"_"+split.suffix+".apk")
		CreateAppPackage(ctx, packageFile, split.path, nil, nil, certificates)
		a.extraOutputFiles = append(a.extraOutputFiles, packageFile)
	}

	// Build an app bundle.
	bundleFile := android.PathForModuleOut(ctx, "base.zip")
	BuildBundleModule(ctx, bundleFile, a.exportPackage, jniJarFile, dexJarFile)
	a.bundleFile = bundleFile

	// Install the app package.
	var installDir android.OutputPath
	if ctx.ModuleName() == "framework-res" {
		// framework-res.apk is installed as system/framework/framework-res.apk
		installDir = android.PathForModuleInstall(ctx, "framework")
	} else if Bool(a.appProperties.Privileged) {
		installDir = android.PathForModuleInstall(ctx, "priv-app", a.installApkName)
	} else {
		installDir = android.PathForModuleInstall(ctx, "app", a.installApkName)
	}

	ctx.InstallFile(installDir, a.installApkName+".apk", a.outputFile)
	for _, split := range a.aapt.splits {
		ctx.InstallFile(installDir, a.installApkName+"_"+split.suffix+".apk", split.path)
	}
}

func (a *AndroidApp) collectAppDeps(ctx android.ModuleContext) ([]jniLib, []Certificate) {
	var jniLibs []jniLib
	var certificates []Certificate

	ctx.VisitDirectDeps(func(module android.Module) {
		otherName := ctx.OtherModuleName(module)
		tag := ctx.OtherModuleDependencyTag(module)

		if jniTag, ok := tag.(*jniDependencyTag); ok {
			if dep, ok := module.(*cc.Module); ok {
				lib := dep.OutputFile()
				if lib.Valid() {
					jniLibs = append(jniLibs, jniLib{
						name:   ctx.OtherModuleName(module),
						path:   lib.Path(),
						target: jniTag.target,
					})
				} else {
					ctx.ModuleErrorf("dependency %q missing output file", otherName)
				}
			} else {
				ctx.ModuleErrorf("jni_libs dependency %q must be a cc library", otherName)

			}
		} else if tag == certificateTag {
			if dep, ok := module.(*AndroidAppCertificate); ok {
				certificates = append(certificates, dep.Certificate)
			} else {
				ctx.ModuleErrorf("certificate dependency %q must be an android_app_certificate module", otherName)
			}
		}
	})

	return jniLibs, certificates
}

func (a *AndroidApp) getCertString(ctx android.BaseContext) string {
	certificate, overridden := ctx.DeviceConfig().OverrideCertificateFor(ctx.ModuleName())
	if overridden {
		return ":" + certificate
	}
	return String(a.overridableAppProperties.Certificate)
}

// android_app compiles sources and Android resources into an Android application package `.apk` file.
func AndroidAppFactory() android.Module {
	module := &AndroidApp{}

	module.Module.deviceProperties.Optimize.EnabledByDefault = true
	module.Module.deviceProperties.Optimize.Shrink = proptools.BoolPtr(true)

	module.Module.properties.Instrument = true
	module.Module.properties.Installable = proptools.BoolPtr(true)

	module.AddProperties(
		&module.Module.properties,
		&module.Module.deviceProperties,
		&module.Module.dexpreoptProperties,
		&module.Module.protoProperties,
		&module.aaptProperties,
		&module.appProperties,
		&module.overridableAppProperties)

	module.Prefer32(func(ctx android.BaseModuleContext, base *android.ModuleBase, class android.OsClass) bool {
		return class == android.Device && ctx.Config().DevicePrefer32BitApps()
	})

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitOverridableModule(module, &module.appProperties.Overrides)

	return module
}

type appTestProperties struct {
	Instrumentation_for *string
}

type AndroidTest struct {
	AndroidApp

	appTestProperties appTestProperties

	testProperties testProperties

	testConfig android.Path
	data       android.Paths
}

func (a *AndroidTest) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Check if the instrumentation target package is overridden before generating build actions.
	if a.appTestProperties.Instrumentation_for != nil {
		manifestPackageName, overridden := ctx.DeviceConfig().OverrideManifestPackageNameFor(*a.appTestProperties.Instrumentation_for)
		if overridden {
			a.additionalAaptFlags = append(a.additionalAaptFlags, "--rename-instrumentation-target-package "+manifestPackageName)
		}
	}
	a.generateAndroidBuildActions(ctx)

	a.testConfig = tradefed.AutoGenInstrumentationTestConfig(ctx, a.testProperties.Test_config, a.testProperties.Test_config_template, a.manifestPath, a.testProperties.Test_suites)
	a.data = android.PathsForModuleSrc(ctx, a.testProperties.Data)
}

func (a *AndroidTest) DepsMutator(ctx android.BottomUpMutatorContext) {
	a.AndroidApp.DepsMutator(ctx)
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

	module.Module.deviceProperties.Optimize.EnabledByDefault = true

	module.Module.properties.Instrument = true
	module.Module.properties.Installable = proptools.BoolPtr(true)
	module.appProperties.Use_embedded_native_libs = proptools.BoolPtr(true)
	module.appProperties.AlwaysPackageNativeLibs = true
	module.Module.dexpreopter.isTest = true

	module.AddProperties(
		&module.Module.properties,
		&module.Module.deviceProperties,
		&module.Module.dexpreoptProperties,
		&module.Module.protoProperties,
		&module.aaptProperties,
		&module.appProperties,
		&module.appTestProperties,
		&module.overridableAppProperties,
		&module.testProperties)

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

type appTestHelperAppProperties struct {
	// list of compatibility suites (for example "cts", "vts") that the module should be
	// installed into.
	Test_suites []string `android:"arch_variant"`
}

type AndroidTestHelperApp struct {
	AndroidApp

	appTestHelperAppProperties appTestHelperAppProperties
}

// android_test_helper_app compiles sources and Android resources into an Android application package `.apk` file that
// will be used by tests, but does not produce an `AndroidTest.xml` file so the module will not be run directly as a
// test.
func AndroidTestHelperAppFactory() android.Module {
	module := &AndroidTestHelperApp{}

	module.Module.deviceProperties.Optimize.EnabledByDefault = true

	module.Module.properties.Installable = proptools.BoolPtr(true)
	module.appProperties.Use_embedded_native_libs = proptools.BoolPtr(true)
	module.appProperties.AlwaysPackageNativeLibs = true
	module.Module.dexpreopter.isTest = true

	module.AddProperties(
		&module.Module.properties,
		&module.Module.deviceProperties,
		&module.Module.dexpreoptProperties,
		&module.Module.protoProperties,
		&module.aaptProperties,
		&module.appProperties,
		&module.appTestHelperAppProperties,
		&module.overridableAppProperties)

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
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
		android.PathForModuleSrc(ctx, cert+".x509.pem"),
		android.PathForModuleSrc(ctx, cert+".pk8"),
	}
}

type OverrideAndroidApp struct {
	android.ModuleBase
	android.OverrideModuleBase
}

func (i *OverrideAndroidApp) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// All the overrides happen in the base module.
	// TODO(jungjw): Check the base module type.
}

// override_android_app is used to create an android_app module based on another android_app by overriding
// some of its properties.
func OverrideAndroidAppModuleFactory() android.Module {
	m := &OverrideAndroidApp{}
	m.AddProperties(&overridableAppProperties{})

	android.InitAndroidModule(m)
	android.InitOverrideModule(m)
	return m
}
