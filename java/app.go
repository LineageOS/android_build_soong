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
	"os"
	"path/filepath"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"

	"android/soong/common"
)

// AAR prebuilts
// AndroidManifest.xml merging
// package splits

type androidAppProperties struct {
	// path to a certificate, or the name of a certificate in the default
	// certificate directory, or blank to use the default product certificate
	Certificate string

	// paths to extra certificates to sign the apk with
	Additional_certificates []string

	// If set, create package-export.apk, which other packages can
	// use to get PRODUCT-agnostic resource data like IDs and type definitions.
	Export_package_resources bool

	// flags passed to aapt when creating the apk
	Aaptflags []string

	// list of resource labels to generate individual resource packages
	Package_splits []string

	// list of directories relative to the Blueprints file containing assets.
	// Defaults to "assets"
	Asset_dirs []string

	// list of directories relative to the Blueprints file containing
	// Java resources
	Android_resource_dirs []string
}

type AndroidApp struct {
	javaBase

	appProperties androidAppProperties

	aaptJavaFileList string
	exportPackage    string
}

func (a *AndroidApp) JavaDynamicDependencies(ctx common.AndroidDynamicDependerModuleContext) []string {
	deps := a.javaBase.JavaDynamicDependencies(ctx)

	if !a.properties.No_standard_libraries {
		switch a.properties.Sdk_version { // TODO: Res_sdk_version?
		case "current", "system_current", "":
			deps = append(deps, "framework-res")
		default:
			// We'll already have a dependency on an sdk prebuilt android.jar
		}
	}

	return deps
}

func (a *AndroidApp) GenerateJavaBuildActions(ctx common.AndroidModuleContext) {
	aaptFlags, aaptDeps, hasResources := a.aaptFlags(ctx)

	if hasResources {
		// First generate R.java so we can build the .class files
		aaptRJavaFlags := append([]string(nil), aaptFlags...)

		publicResourcesFile, proguardOptionsFile, aaptJavaFileList :=
			CreateResourceJavaFiles(ctx, aaptRJavaFlags, aaptDeps)
		a.aaptJavaFileList = aaptJavaFileList
		a.ExtraSrcLists = append(a.ExtraSrcLists, aaptJavaFileList)

		if a.appProperties.Export_package_resources {
			aaptPackageFlags := append([]string(nil), aaptFlags...)
			var hasProduct bool
			for _, f := range aaptPackageFlags {
				if strings.HasPrefix(f, "--product") {
					hasProduct = true
					break
				}
			}

			if !hasProduct {
				aaptPackageFlags = append(aaptPackageFlags,
					"--product "+ctx.AConfig().ProductAaptCharacteristics())
			}
			a.exportPackage = CreateExportPackage(ctx, aaptPackageFlags, aaptDeps)
			ctx.CheckbuildFile(a.exportPackage)
		}
		ctx.CheckbuildFile(publicResourcesFile)
		ctx.CheckbuildFile(proguardOptionsFile)
		ctx.CheckbuildFile(aaptJavaFileList)
	}

	// apps manifests are handled by aapt, don't let javaBase see them
	a.properties.Manifest = ""

	//if !ctx.ContainsProperty("proguard.enabled") {
	//	a.properties.Proguard.Enabled = true
	//}

	a.javaBase.GenerateJavaBuildActions(ctx)

	aaptPackageFlags := append([]string(nil), aaptFlags...)
	var hasProduct bool
	for _, f := range aaptPackageFlags {
		if strings.HasPrefix(f, "--product") {
			hasProduct = true
			break
		}
	}

	if !hasProduct {
		aaptPackageFlags = append(aaptPackageFlags,
			"--product "+ctx.AConfig().ProductAaptCharacteristics())
	}

	certificate := a.appProperties.Certificate
	if certificate == "" {
		certificate = ctx.AConfig().DefaultAppCertificate()
	} else if dir, _ := filepath.Split(certificate); dir == "" {
		certificate = filepath.Join(ctx.AConfig().DefaultAppCertificateDir(), certificate)
	} else {
		certificate = filepath.Join(ctx.AConfig().SrcDir(), certificate)
	}

	certificates := []string{certificate}
	for _, c := range a.appProperties.Additional_certificates {
		certificates = append(certificates, filepath.Join(ctx.AConfig().SrcDir(), c))
	}

	a.outputFile = CreateAppPackage(ctx, aaptPackageFlags, a.outputFile, certificates)
	ctx.InstallFileName("app", ctx.ModuleName()+".apk", a.outputFile)
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

func (a *AndroidApp) aaptFlags(ctx common.AndroidModuleContext) ([]string, []string, bool) {
	aaptFlags := a.appProperties.Aaptflags
	hasVersionCode := false
	hasVersionName := false
	for _, f := range aaptFlags {
		if strings.HasPrefix(f, "--version-code") {
			hasVersionCode = true
		} else if strings.HasPrefix(f, "--version-name") {
			hasVersionName = true
		}
	}

	if true /* is not a test */ {
		aaptFlags = append(aaptFlags, "-z")
	}

	assetDirs := a.appProperties.Asset_dirs
	if len(assetDirs) == 0 {
		defaultAssetDir := filepath.Join(common.ModuleSrcDir(ctx), "assets")
		if _, err := os.Stat(defaultAssetDir); err == nil {
			assetDirs = []string{defaultAssetDir}
		} else {
			// Default asset directory doesn't exist, add a dep on the parent directory to
			// regenerate the manifest if it is created later
			// TODO: use glob to avoid rerunning whole regenerate if a different file is created?
			ctx.AddNinjaFileDeps(common.ModuleSrcDir(ctx))
		}
	} else {
		assetDirs = pathtools.PrefixPaths(assetDirs, common.ModuleSrcDir(ctx))
	}

	resourceDirs := a.appProperties.Android_resource_dirs
	if len(resourceDirs) == 0 {
		defaultResourceDir := filepath.Join(common.ModuleSrcDir(ctx), "res")
		if _, err := os.Stat(defaultResourceDir); err == nil {
			resourceDirs = []string{defaultResourceDir}
		} else {
			// Default resource directory doesn't exist, add a dep on the parent directory to
			// regenerate the manifest if it is created later
			// TODO: use glob to avoid rerunning whole regenerate if a different file is created?
			ctx.AddNinjaFileDeps(common.ModuleSrcDir(ctx))
		}
	} else {
		resourceDirs = pathtools.PrefixPaths(resourceDirs, common.ModuleSrcDir(ctx))
	}

	rootSrcDir := ctx.AConfig().SrcDir()
	var overlayResourceDirs []string
	// For every resource directory, check if there is an overlay directory with the same path.
	// If found, it will be prepended to the list of resource directories.
	for _, overlayDir := range ctx.AConfig().ResourceOverlays() {
		for _, resourceDir := range resourceDirs {
			relResourceDir, err := filepath.Rel(rootSrcDir, resourceDir)
			if err != nil {
				ctx.ModuleErrorf("resource directory %q is not in source tree", resourceDir)
				continue
			}
			overlayResourceDir := filepath.Join(overlayDir, relResourceDir)
			if _, err := os.Stat(overlayResourceDir); err == nil {
				overlayResourceDirs = append(overlayResourceDirs, overlayResourceDir)
			} else {
				// Overlay resource directory doesn't exist, add a dep to regenerate the manifest if
				// it is created later
				ctx.AddNinjaFileDeps(overlayResourceDir)
			}
		}
	}

	if len(overlayResourceDirs) > 0 {
		resourceDirs = append(overlayResourceDirs, resourceDirs...)
	}

	// aapt needs to rerun if any files are added or modified in the assets or resource directories,
	// use glob to create a filelist.
	var aaptDeps []string
	var hasResources bool
	for _, d := range resourceDirs {
		newDeps := ctx.Glob(filepath.Join(d, "**/*"), aaptIgnoreFilenames)
		aaptDeps = append(aaptDeps, newDeps...)
		if len(newDeps) > 0 {
			hasResources = true
		}
	}
	for _, d := range assetDirs {
		newDeps := ctx.Glob(filepath.Join(d, "**/*"), aaptIgnoreFilenames)
		aaptDeps = append(aaptDeps, newDeps...)
	}

	manifestFile := a.properties.Manifest
	if manifestFile == "" {
		manifestFile = "AndroidManifest.xml"
	}

	manifestFile = filepath.Join(common.ModuleSrcDir(ctx), manifestFile)
	aaptDeps = append(aaptDeps, manifestFile)

	aaptFlags = append(aaptFlags, "-M "+manifestFile)
	aaptFlags = append(aaptFlags, common.JoinWithPrefix(assetDirs, "-A "))
	aaptFlags = append(aaptFlags, common.JoinWithPrefix(resourceDirs, "-S "))

	ctx.VisitDirectDeps(func(module blueprint.Module) {
		var depFile string
		if sdkDep, ok := module.(sdkDependency); ok {
			depFile = sdkDep.ClasspathFile()
		} else if javaDep, ok := module.(JavaDependency); ok {
			if ctx.OtherModuleName(module) == "framework-res" {
				depFile = javaDep.(*javaBase).module.(*AndroidApp).exportPackage
			}
		}
		if depFile != "" {
			aaptFlags = append(aaptFlags, "-I "+depFile)
			aaptDeps = append(aaptDeps, depFile)
		}
	})

	sdkVersion := a.properties.Sdk_version
	if sdkVersion == "" {
		sdkVersion = ctx.AConfig().PlatformSdkVersion()
	}

	aaptFlags = append(aaptFlags, "--min-sdk-version "+sdkVersion)
	aaptFlags = append(aaptFlags, "--target-sdk-version "+sdkVersion)

	if !hasVersionCode {
		aaptFlags = append(aaptFlags, "--version-code "+ctx.AConfig().PlatformSdkVersion())
	}

	if !hasVersionName {
		aaptFlags = append(aaptFlags,
			"--version-name "+ctx.AConfig().PlatformVersion()+"-"+ctx.AConfig().BuildNumber())
	}

	// TODO: LOCAL_PACKAGE_OVERRIDES
	//    $(addprefix --rename-manifest-package , $(PRIVATE_MANIFEST_PACKAGE_NAME)) \

	// TODO: LOCAL_INSTRUMENTATION_FOR
	//    $(addprefix --rename-instrumentation-target-package , $(PRIVATE_MANIFEST_INSTRUMENTATION_FOR))

	return aaptFlags, aaptDeps, hasResources
}

func AndroidAppFactory() (blueprint.Module, []interface{}) {
	module := &AndroidApp{}

	module.properties.Dex = true

	return NewJavaBase(&module.javaBase, module, common.DeviceSupported, &module.appProperties)
}
