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
}

// AndroidManifest.xml merging
// package splits

type appProperties struct {
	// The name of a certificate in the default certificate directory, blank to use the default product certificate,
	// or an android_app_certificate module name in the form ":module".
	Certificate *string

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

	EmbedJNI bool `blueprint:"mutated"`
}

type AndroidApp struct {
	Library
	aapt

	certificate Certificate

	appProperties appProperties

	extraLinkFlags []string

	installJniLibs []jniLib

	bundleFile android.Path
}

func (a *AndroidApp) ExportedProguardFlagFiles() android.Paths {
	return nil
}

func (a *AndroidApp) ExportedStaticPackages() android.Paths {
	return nil
}

func (a *AndroidApp) ExportedManifest() android.Path {
	return a.manifestPath
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

	cert := android.SrcIsModule(String(a.appProperties.Certificate))
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
	a.generateAndroidBuildActions(ctx)
}

// Returns whether this module should have the dex file stored uncompressed in the APK.
func (a *AndroidApp) shouldUncompressDex(ctx android.ModuleContext) bool {
	if ctx.Config().UnbundledBuild() {
		return false
	}

	// Uncompress dex in APKs of privileged apps, and modules used by privileged apps.
	return ctx.Config().UncompressPrivAppDex() &&
		(Bool(a.appProperties.Privileged) ||
			inList(ctx.ModuleName(), ctx.Config().ModulesLoadedByPrivilegedModules()))
}

func (a *AndroidApp) generateAndroidBuildActions(ctx android.ModuleContext) {
	linkFlags := append([]string(nil), a.extraLinkFlags...)

	hasProduct := false
	for _, f := range a.aaptProperties.Aaptflags {
		if strings.HasPrefix(f, "--product") {
			hasProduct = true
		}
	}

	// Product characteristics
	if !hasProduct && len(ctx.Config().ProductAAPTCharacteristics()) > 0 {
		linkFlags = append(linkFlags, "--product", ctx.Config().ProductAAPTCharacteristics())
	}

	if !Bool(a.aaptProperties.Aapt_include_all_resources) {
		// Product AAPT config
		for _, aaptConfig := range ctx.Config().ProductAAPTConfig() {
			linkFlags = append(linkFlags, "-c", aaptConfig)
		}

		// Product AAPT preferred config
		if len(ctx.Config().ProductAAPTPreferredConfig()) > 0 {
			linkFlags = append(linkFlags, "--preferred-density", ctx.Config().ProductAAPTPreferredConfig())
		}
	}

	// TODO: LOCAL_PACKAGE_OVERRIDES
	//    $(addprefix --rename-manifest-package , $(PRIVATE_MANIFEST_PACKAGE_NAME)) \

	manifestPackageName, overridden := ctx.DeviceConfig().OverrideManifestPackageNameFor(ctx.ModuleName())
	if overridden {
		linkFlags = append(linkFlags, "--rename-manifest-package "+manifestPackageName)
	}

	a.aapt.buildActions(ctx, sdkContext(a), linkFlags...)

	// apps manifests are handled by aapt, don't let Module see them
	a.properties.Manifest = nil

	var staticLibProguardFlagFiles android.Paths
	ctx.VisitDirectDeps(func(m android.Module) {
		if lib, ok := m.(AndroidLibraryDependency); ok && ctx.OtherModuleDependencyTag(m) == staticLibTag {
			staticLibProguardFlagFiles = append(staticLibProguardFlagFiles, lib.ExportedProguardFlagFiles()...)
		}
	})

	staticLibProguardFlagFiles = android.FirstUniquePaths(staticLibProguardFlagFiles)

	a.Module.extraProguardFlagFiles = append(a.Module.extraProguardFlagFiles, staticLibProguardFlagFiles...)
	a.Module.extraProguardFlagFiles = append(a.Module.extraProguardFlagFiles, a.proguardOptionsFile)

	a.deviceProperties.UncompressDex = a.shouldUncompressDex(ctx)

	var installDir string
	if ctx.ModuleName() == "framework-res" {
		// framework-res.apk is installed as system/framework/framework-res.apk
		installDir = "framework"
	} else if Bool(a.appProperties.Privileged) {
		installDir = filepath.Join("priv-app", ctx.ModuleName())
	} else {
		installDir = filepath.Join("app", ctx.ModuleName())
	}
	a.dexpreopter.installPath = android.PathForModuleInstall(ctx, installDir, ctx.ModuleName()+".apk")

	if ctx.ModuleName() != "framework-res" {
		a.Module.compile(ctx, a.aaptSrcJar)
	}

	dexJarFile := a.maybeStrippedDexJarFile

	var certificates []Certificate

	var jniJarFile android.WritablePath
	jniLibs, certificateDeps := a.collectAppDeps(ctx)
	if len(jniLibs) > 0 {
		embedJni := ctx.Config().UnbundledBuild() || a.appProperties.EmbedJNI
		if embedJni {
			jniJarFile = android.PathForModuleOut(ctx, "jnilibs.zip")
			TransformJniLibsToJar(ctx, jniJarFile, jniLibs)
		} else {
			a.installJniLibs = jniLibs
		}
	}

	if ctx.Failed() {
		return
	}

	cert := String(a.appProperties.Certificate)
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

	certificates = append([]Certificate{a.certificate}, certificateDeps...)

	packageFile := android.PathForModuleOut(ctx, "package.apk")
	CreateAppPackage(ctx, packageFile, a.exportPackage, jniJarFile, dexJarFile, certificates)

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

	a.outputFile = packageFile

	bundleFile := android.PathForModuleOut(ctx, "base.zip")
	BuildBundleModule(ctx, bundleFile, a.exportPackage, jniJarFile, dexJarFile)
	a.bundleFile = bundleFile

	if ctx.ModuleName() == "framework-res" {
		// framework-res.apk is installed as system/framework/framework-res.apk
		ctx.InstallFile(android.PathForModuleInstall(ctx, "framework"), ctx.ModuleName()+".apk", a.outputFile)
	} else if Bool(a.appProperties.Privileged) {
		ctx.InstallFile(android.PathForModuleInstall(ctx, "priv-app", ctx.ModuleName()), ctx.ModuleName()+".apk", a.outputFile)
	} else {
		ctx.InstallFile(android.PathForModuleInstall(ctx, "app", ctx.ModuleName()), ctx.ModuleName()+".apk", a.outputFile)
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

func AndroidAppFactory() android.Module {
	module := &AndroidApp{}

	module.Module.deviceProperties.Optimize.Enabled = proptools.BoolPtr(true)
	module.Module.deviceProperties.Optimize.Shrink = proptools.BoolPtr(true)

	module.Module.properties.Instrument = true
	module.Module.properties.Installable = proptools.BoolPtr(true)

	module.AddProperties(
		&module.Module.properties,
		&module.Module.deviceProperties,
		&module.Module.dexpreoptProperties,
		&module.Module.protoProperties,
		&module.aaptProperties,
		&module.appProperties)

	module.Prefer32(func(ctx android.BaseModuleContext, base *android.ModuleBase, class android.OsClass) bool {
		return class == android.Device && ctx.Config().DevicePrefer32BitApps()
	})

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)

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
	a.generateAndroidBuildActions(ctx)

	a.testConfig = tradefed.AutoGenInstrumentationTestConfig(ctx, a.testProperties.Test_config, a.testProperties.Test_config_template, a.manifestPath)
	a.data = ctx.ExpandSources(a.testProperties.Data, nil)
}

func (a *AndroidTest) DepsMutator(ctx android.BottomUpMutatorContext) {
	android.ExtractSourceDeps(ctx, a.testProperties.Test_config)
	android.ExtractSourceDeps(ctx, a.testProperties.Test_config_template)
	android.ExtractSourcesDeps(ctx, a.testProperties.Data)
	a.AndroidApp.DepsMutator(ctx)
	if a.appTestProperties.Instrumentation_for != nil {
		// The android_app dependency listed in instrumentation_for needs to be added to the classpath for javac,
		// but not added to the aapt2 link includes like a normal android_app or android_library dependency, so
		// use instrumentationForTag instead of libTag.
		ctx.AddVariationDependencies(nil, instrumentationForTag, String(a.appTestProperties.Instrumentation_for))
	}
}

func AndroidTestFactory() android.Module {
	module := &AndroidTest{}

	module.Module.deviceProperties.Optimize.Enabled = proptools.BoolPtr(true)

	module.Module.properties.Instrument = true
	module.Module.properties.Installable = proptools.BoolPtr(true)
	module.appProperties.EmbedJNI = true
	module.Module.dexpreopter.isTest = true

	module.AddProperties(
		&module.Module.properties,
		&module.Module.deviceProperties,
		&module.Module.dexpreoptProperties,
		&module.Module.protoProperties,
		&module.aaptProperties,
		&module.appProperties,
		&module.appTestProperties,
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

func AndroidTestHelperAppFactory() android.Module {
	module := &AndroidTestHelperApp{}

	module.Module.deviceProperties.Optimize.Enabled = proptools.BoolPtr(true)

	module.Module.properties.Installable = proptools.BoolPtr(true)
	module.appProperties.EmbedJNI = true
	module.Module.dexpreopter.isTest = true

	module.AddProperties(
		&module.Module.properties,
		&module.Module.deviceProperties,
		&module.Module.dexpreoptProperties,
		&module.Module.protoProperties,
		&module.aaptProperties,
		&module.appProperties,
		&module.appTestHelperAppProperties)

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

func AndroidAppCertificateFactory() android.Module {
	module := &AndroidAppCertificate{}
	module.AddProperties(&module.properties)
	android.InitAndroidModule(module)
	return module
}

func (c *AndroidAppCertificate) DepsMutator(ctx android.BottomUpMutatorContext) {
}

func (c *AndroidAppCertificate) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	cert := String(c.properties.Certificate)
	c.Certificate = Certificate{
		android.PathForModuleSrc(ctx, cert+".x509.pem"),
		android.PathForModuleSrc(ctx, cert+".pk8"),
	}
}
