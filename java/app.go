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

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func init() {
	android.RegisterPreSingletonType("overlay", OverlaySingletonFactory)
	android.RegisterModuleType("android_app", AndroidAppFactory)
}

// AAR prebuilts
// AndroidManifest.xml merging
// package splits

type appProperties struct {
	// path to a certificate, or the name of a certificate in the default
	// certificate directory, or blank to use the default product certificate
	Certificate *string

	// paths to extra certificates to sign the apk with
	Additional_certificates []string

	// If set, create package-export.apk, which other packages can
	// use to get PRODUCT-agnostic resource data like IDs and type definitions.
	Export_package_resources *bool

	// flags passed to aapt when creating the apk
	Aaptflags []string

	// list of resource labels to generate individual resource packages
	Package_splits []string

	// list of directories relative to the Blueprints file containing assets.
	// Defaults to "assets"
	Asset_dirs []string

	// list of directories relative to the Blueprints file containing
	// Android resources
	Resource_dirs []string

	Instrumentation_for *string

	// Specifies that this app should be installed to the priv-app directory,
	// where the system will grant it additional privileges not available to
	// normal apps.
	Privileged *bool
}

type AndroidApp struct {
	Module

	appProperties appProperties

	aaptSrcJar    android.Path
	exportPackage android.Path
	rroDirs       android.Paths
	manifestPath  android.Path
	certificate   certificate
}

type certificate struct {
	pem, key android.Path
}

func (a *AndroidApp) DepsMutator(ctx android.BottomUpMutatorContext) {
	a.Module.deps(ctx)

	if !Bool(a.properties.No_framework_libs) && !Bool(a.properties.No_standard_libs) {
		switch String(a.deviceProperties.Sdk_version) { // TODO: Res_sdk_version?
		case "current", "system_current", "test_current", "":
			ctx.AddDependency(ctx.Module(), frameworkResTag, "framework-res")
		default:
			// We'll already have a dependency on an sdk prebuilt android.jar
		}
	}
}

func (a *AndroidApp) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	linkFlags, linkDeps, resDirs, overlayDirs, rroDirs, manifestPath := a.aapt2Flags(ctx)

	packageRes := android.PathForModuleOut(ctx, "package-res.apk")
	srcJar := android.PathForModuleGen(ctx, "R.jar")
	proguardOptionsFile := android.PathForModuleGen(ctx, "proguard.options")

	var compiledRes, compiledOverlay android.Paths
	for _, dir := range resDirs {
		compiledRes = append(compiledRes, aapt2Compile(ctx, dir.dir, dir.files).Paths()...)
	}
	for _, dir := range overlayDirs {
		compiledOverlay = append(compiledOverlay, aapt2Compile(ctx, dir.dir, dir.files).Paths()...)
	}

	aapt2Link(ctx, packageRes, srcJar, proguardOptionsFile,
		linkFlags, linkDeps, compiledRes, compiledOverlay)

	a.exportPackage = packageRes
	a.aaptSrcJar = srcJar

	ctx.CheckbuildFile(proguardOptionsFile)
	ctx.CheckbuildFile(a.exportPackage)
	ctx.CheckbuildFile(a.aaptSrcJar)

	// apps manifests are handled by aapt, don't let Module see them
	a.properties.Manifest = nil

	if String(a.appProperties.Instrumentation_for) == "" {
		a.properties.Instrument = true
	}

	a.Module.extraProguardFlagFiles = append(a.Module.extraProguardFlagFiles,
		proguardOptionsFile)

	if ctx.ModuleName() != "framework-res" {
		a.Module.compile(ctx, a.aaptSrcJar)
	}

	c := String(a.appProperties.Certificate)
	switch {
	case c == "":
		pem, key := ctx.Config().DefaultAppCertificate(ctx)
		a.certificate = certificate{pem, key}
	case strings.ContainsRune(c, '/'):
		a.certificate = certificate{
			android.PathForSource(ctx, c+".x509.pem"),
			android.PathForSource(ctx, c+".pk8"),
		}
	default:
		defaultDir := ctx.Config().DefaultAppCertificateDir(ctx)
		a.certificate = certificate{
			defaultDir.Join(ctx, c+".x509.pem"),
			defaultDir.Join(ctx, c+".pk8"),
		}
	}

	certificates := []certificate{a.certificate}
	for _, c := range a.appProperties.Additional_certificates {
		certificates = append(certificates, certificate{
			android.PathForSource(ctx, c+".x509.pem"),
			android.PathForSource(ctx, c+".pk8"),
		})
	}

	packageFile := android.PathForModuleOut(ctx, "package.apk")

	CreateAppPackage(ctx, packageFile, a.exportPackage, a.outputFile, certificates)

	a.outputFile = packageFile
	a.rroDirs = rroDirs
	a.manifestPath = manifestPath

	if ctx.ModuleName() == "framework-res" {
		// framework-res.apk is installed as system/framework/framework-res.apk
		ctx.InstallFile(android.PathForModuleInstall(ctx, "framework"), ctx.ModuleName()+".apk", a.outputFile)
	} else if Bool(a.appProperties.Privileged) {
		ctx.InstallFile(android.PathForModuleInstall(ctx, "priv-app"), ctx.ModuleName()+".apk", a.outputFile)
	} else {
		ctx.InstallFile(android.PathForModuleInstall(ctx, "app"), ctx.ModuleName()+".apk", a.outputFile)
	}
}

var aaptIgnoreFilenames = []string{
	".svn",
	".git",
	".ds_store",
	"*.scc",
	".*",
	"CVS",
	"thumbs.db",
	"picasa.ini",
	"*~",
}

type globbedResourceDir struct {
	dir   android.Path
	files android.Paths
}

func (a *AndroidApp) aapt2Flags(ctx android.ModuleContext) (flags []string, deps android.Paths,
	resDirs, overlayDirs []globbedResourceDir, rroDirs android.Paths, manifestPath android.Path) {

	hasVersionCode := false
	hasVersionName := false
	hasProduct := false
	for _, f := range a.appProperties.Aaptflags {
		if strings.HasPrefix(f, "--version-code") {
			hasVersionCode = true
		} else if strings.HasPrefix(f, "--version-name") {
			hasVersionName = true
		} else if strings.HasPrefix(f, "--product") {
			hasProduct = true
		}
	}

	var linkFlags []string

	// Flags specified in Android.bp
	linkFlags = append(linkFlags, a.appProperties.Aaptflags...)

	linkFlags = append(linkFlags, "--no-static-lib-packages")

	// Find implicit or explicit asset and resource dirs
	assetDirs := android.PathsWithOptionalDefaultForModuleSrc(ctx, a.appProperties.Asset_dirs, "assets")
	resourceDirs := android.PathsWithOptionalDefaultForModuleSrc(ctx, a.appProperties.Resource_dirs, "res")

	var linkDeps android.Paths

	// Glob directories into lists of paths
	for _, dir := range resourceDirs {
		resDirs = append(resDirs, globbedResourceDir{
			dir:   dir,
			files: resourceGlob(ctx, dir),
		})
		resOverlayDirs, resRRODirs := overlayResourceGlob(ctx, dir)
		overlayDirs = append(overlayDirs, resOverlayDirs...)
		rroDirs = append(rroDirs, resRRODirs...)
	}

	var assetFiles android.Paths
	for _, dir := range assetDirs {
		assetFiles = append(assetFiles, resourceGlob(ctx, dir)...)
	}

	// App manifest file
	var manifestFile string
	if a.properties.Manifest == nil {
		manifestFile = "AndroidManifest.xml"
	} else {
		manifestFile = *a.properties.Manifest
	}

	manifestPath = android.PathForModuleSrc(ctx, manifestFile)
	linkFlags = append(linkFlags, "--manifest "+manifestPath.String())
	linkDeps = append(linkDeps, manifestPath)

	linkFlags = append(linkFlags, android.JoinWithPrefix(assetDirs.Strings(), "-A "))
	linkDeps = append(linkDeps, assetFiles...)

	// Include dirs
	ctx.VisitDirectDeps(func(module android.Module) {
		var depFiles android.Paths
		if javaDep, ok := module.(Dependency); ok {
			// TODO: shared android libraries
			if ctx.OtherModuleName(module) == "framework-res" {
				depFiles = android.Paths{javaDep.(*AndroidApp).exportPackage}
			}
		}

		for _, dep := range depFiles {
			linkFlags = append(linkFlags, "-I "+dep.String())
		}
		linkDeps = append(linkDeps, depFiles...)
	})

	sdkDep := decodeSdkDep(ctx, String(a.deviceProperties.Sdk_version))
	if sdkDep.useFiles {
		linkFlags = append(linkFlags, "-I "+sdkDep.jar.String())
		linkDeps = append(linkDeps, sdkDep.jar)
	}

	// SDK version flags
	sdkVersion := String(a.deviceProperties.Sdk_version)
	switch sdkVersion {
	case "", "current", "system_current", "test_current":
		sdkVersion = proptools.NinjaEscape([]string{ctx.Config().DefaultAppTargetSdk()})[0]
	}

	linkFlags = append(linkFlags, "--min-sdk-version "+sdkVersion)
	linkFlags = append(linkFlags, "--target-sdk-version "+sdkVersion)

	// Product characteristics
	if !hasProduct && len(ctx.Config().ProductAAPTCharacteristics()) > 0 {
		linkFlags = append(linkFlags, "--product", ctx.Config().ProductAAPTCharacteristics())
	}

	// Product AAPT config
	for _, aaptConfig := range ctx.Config().ProductAAPTConfig() {
		linkFlags = append(linkFlags, "-c", aaptConfig)
	}

	// Product AAPT preferred config
	if len(ctx.Config().ProductAAPTPreferredConfig()) > 0 {
		linkFlags = append(linkFlags, "--preferred-density", ctx.Config().ProductAAPTPreferredConfig())
	}

	// Version code
	if !hasVersionCode {
		linkFlags = append(linkFlags, "--version-code", ctx.Config().PlatformSdkVersion())
	}

	if !hasVersionName {
		var versionName string
		if ctx.ModuleName() == "framework-res" {
			// Some builds set AppsDefaultVersionName() to include the build number ("O-123456").  aapt2 copies the
			// version name of framework-res into app manifests as compileSdkVersionCodename, which confuses things
			// if it contains the build number.  Use the DefaultAppTargetSdk instead.
			versionName = ctx.Config().DefaultAppTargetSdk()
		} else {
			versionName = ctx.Config().AppsDefaultVersionName()
		}
		versionName = proptools.NinjaEscape([]string{versionName})[0]
		linkFlags = append(linkFlags, "--version-name ", versionName)
	}

	if String(a.appProperties.Instrumentation_for) != "" {
		linkFlags = append(linkFlags,
			"--rename-instrumentation-target-package",
			String(a.appProperties.Instrumentation_for))
	}

	// TODO: LOCAL_PACKAGE_OVERRIDES
	//    $(addprefix --rename-manifest-package , $(PRIVATE_MANIFEST_PACKAGE_NAME)) \

	return linkFlags, linkDeps, resDirs, overlayDirs, rroDirs, manifestPath
}

func AndroidAppFactory() android.Module {
	module := &AndroidApp{}

	module.Module.deviceProperties.Optimize.Enabled = proptools.BoolPtr(true)
	module.Module.deviceProperties.Optimize.Shrink = proptools.BoolPtr(true)

	module.AddProperties(
		&module.Module.properties,
		&module.Module.deviceProperties,
		&module.appProperties)

	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	return module
}

func resourceGlob(ctx android.ModuleContext, dir android.Path) android.Paths {
	return ctx.GlobFiles(filepath.Join(dir.String(), "**/*"), aaptIgnoreFilenames)
}

type overlayGlobResult struct {
	dir   string
	paths android.DirectorySortedPaths

	// Set to true of the product has selected that values in this overlay should not be moved to
	// Runtime Resource Overlay (RRO) packages.
	excludeFromRRO bool
}

const overlayDataKey = "overlayDataKey"

func overlayResourceGlob(ctx android.ModuleContext, dir android.Path) (res []globbedResourceDir,
	rroDirs android.Paths) {

	overlayData := ctx.Config().Get(overlayDataKey).([]overlayGlobResult)

	// Runtime resource overlays (RRO) may be turned on by the product config for some modules
	rroEnabled := ctx.Config().EnforceRROForModule(ctx.ModuleName())

	for _, data := range overlayData {
		files := data.paths.PathsInDirectory(filepath.Join(data.dir, dir.String()))
		if len(files) > 0 {
			overlayModuleDir := android.PathForSource(ctx, data.dir, dir.String())
			// If enforce RRO is enabled for this module and this overlay is not in the
			// exclusion list, ignore the overlay.  The list of ignored overlays will be
			// passed to Make to be turned into an RRO package.
			if rroEnabled && !data.excludeFromRRO {
				rroDirs = append(rroDirs, overlayModuleDir)
			} else {
				res = append(res, globbedResourceDir{
					dir:   overlayModuleDir,
					files: files,
				})
			}
		}
	}

	return res, rroDirs
}

func OverlaySingletonFactory() android.Singleton {
	return overlaySingleton{}
}

type overlaySingleton struct{}

func (overlaySingleton) GenerateBuildActions(ctx android.SingletonContext) {
	var overlayData []overlayGlobResult
	overlayDirs := ctx.Config().ResourceOverlays()
	for i := range overlayDirs {
		// Iterate backwards through the list of overlay directories so that the later, lower-priority
		// directories in the list show up earlier in the command line to aapt2.
		overlay := overlayDirs[len(overlayDirs)-1-i]
		var result overlayGlobResult
		result.dir = overlay

		// Mark overlays that will not have Runtime Resource Overlays enforced on them
		// based on the product config
		result.excludeFromRRO = ctx.Config().EnforceRROExcludedOverlay(overlay)

		files, err := ctx.GlobWithDeps(filepath.Join(overlay, "**/*"), aaptIgnoreFilenames)
		if err != nil {
			ctx.Errorf("failed to glob resource dir %q: %s", overlay, err.Error())
			continue
		}
		var paths android.Paths
		for _, f := range files {
			if !strings.HasSuffix(f, "/") {
				paths = append(paths, android.PathForSource(ctx, f))
			}
		}
		result.paths = android.PathsToDirectorySortedPaths(paths)
		overlayData = append(overlayData, result)
	}

	ctx.Config().Once(overlayDataKey, func() interface{} {
		return overlayData
	})
}
