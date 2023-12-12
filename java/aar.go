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
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"android/soong/android"
	"android/soong/dexpreopt"
	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

type AndroidLibraryDependency interface {
	ExportPackage() android.Path
	ResourcesNodeDepSet() *android.DepSet[*resourcesNode]
	RRODirsDepSet() *android.DepSet[rroDir]
	ManifestsDepSet() *android.DepSet[android.Path]
	SetRROEnforcedForDependent(enforce bool)
	IsRROEnforced(ctx android.BaseModuleContext) bool
}

func init() {
	RegisterAARBuildComponents(android.InitRegistrationContext)
}

func RegisterAARBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("android_library_import", AARImportFactory)
	ctx.RegisterModuleType("android_library", AndroidLibraryFactory)
	ctx.PostDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("propagate_rro_enforcement", propagateRROEnforcementMutator).Parallel()
	})
}

//
// AAR (android library)
//

type androidLibraryProperties struct {
	BuildAAR bool `blueprint:"mutated"`
}

type aaptProperties struct {
	// flags passed to aapt when creating the apk
	Aaptflags []string

	// include all resource configurations, not just the product-configured
	// ones.
	Aapt_include_all_resources *bool

	// list of files to use as assets.
	Assets []string `android:"path"`

	// list of directories relative to the Blueprints file containing assets.
	// Defaults to ["assets"] if a directory called assets exists.  Set to []
	// to disable the default.
	Asset_dirs []string

	// list of directories relative to the Blueprints file containing
	// Android resources.  Defaults to ["res"] if a directory called res exists.
	// Set to [] to disable the default.
	Resource_dirs []string

	// list of zip files containing Android resources.
	Resource_zips []string `android:"path"`

	// path to AndroidManifest.xml.  If unset, defaults to "AndroidManifest.xml".
	Manifest *string `android:"path"`

	// paths to additional manifest files to merge with main manifest.
	Additional_manifests []string `android:"path"`

	// do not include AndroidManifest from dependent libraries
	Dont_merge_manifests *bool

	// If use_resource_processor is set, use Bazel's resource processor instead of aapt2 to generate R.class files.
	// The resource processor produces more optimal R.class files that only list resources in the package of the
	// library that provided them, as opposed to aapt2 which produces R.java files for every package containing
	// every resource.  Using the resource processor can provide significant build time speedups, but requires
	// fixing the module to use the correct package to reference each resource, and to avoid having any other
	// libraries in the tree that use the same package name.  Defaults to false, but will default to true in the
	// future.
	Use_resource_processor *bool

	// true if RRO is enforced for any of the dependent modules
	RROEnforcedForDependent bool `blueprint:"mutated"`

	// Filter only specified product and ignore other products
	Filter_product *string `blueprint:"mutated"`
}

type aapt struct {
	aaptSrcJar                         android.Path
	transitiveAaptRJars                android.Paths
	transitiveAaptResourcePackagesFile android.Path
	exportPackage                      android.Path
	manifestPath                       android.Path
	proguardOptionsFile                android.Path
	rTxt                               android.Path
	rJar                               android.Path
	extraAaptPackagesFile              android.Path
	mergedManifestFile                 android.Path
	noticeFile                         android.OptionalPath
	assetPackage                       android.OptionalPath
	isLibrary                          bool
	defaultManifestVersion             string
	useEmbeddedNativeLibs              bool
	useEmbeddedDex                     bool
	usesNonSdkApis                     bool
	hasNoCode                          bool
	LoggingParent                      string
	resourceFiles                      android.Paths

	splitNames []string
	splits     []split

	aaptProperties aaptProperties

	resourcesNodesDepSet *android.DepSet[*resourcesNode]
	rroDirsDepSet        *android.DepSet[rroDir]
	manifestsDepSet      *android.DepSet[android.Path]

	manifestValues struct {
		applicationId string
	}
}

type split struct {
	name   string
	suffix string
	path   android.Path
}

// Propagate RRO enforcement flag to static lib dependencies transitively.
func propagateRROEnforcementMutator(ctx android.TopDownMutatorContext) {
	m := ctx.Module()
	if d, ok := m.(AndroidLibraryDependency); ok && d.IsRROEnforced(ctx) {
		ctx.VisitDirectDepsWithTag(staticLibTag, func(d android.Module) {
			if a, ok := d.(AndroidLibraryDependency); ok {
				a.SetRROEnforcedForDependent(true)
			}
		})
	}
}

func (a *aapt) useResourceProcessorBusyBox() bool {
	return BoolDefault(a.aaptProperties.Use_resource_processor, false)
}

func (a *aapt) filterProduct() string {
	return String(a.aaptProperties.Filter_product)
}

func (a *aapt) ExportPackage() android.Path {
	return a.exportPackage
}
func (a *aapt) ResourcesNodeDepSet() *android.DepSet[*resourcesNode] {
	return a.resourcesNodesDepSet
}

func (a *aapt) RRODirsDepSet() *android.DepSet[rroDir] {
	return a.rroDirsDepSet
}

func (a *aapt) ManifestsDepSet() *android.DepSet[android.Path] {
	return a.manifestsDepSet
}

func (a *aapt) SetRROEnforcedForDependent(enforce bool) {
	a.aaptProperties.RROEnforcedForDependent = enforce
}

func (a *aapt) IsRROEnforced(ctx android.BaseModuleContext) bool {
	// True if RRO is enforced for this module or...
	return ctx.Config().EnforceRROForModule(ctx.ModuleName()) ||
		// if RRO is enforced for any of its dependents.
		a.aaptProperties.RROEnforcedForDependent
}

func (a *aapt) aapt2Flags(ctx android.ModuleContext, sdkContext android.SdkContext,
	manifestPath android.Path) (compileFlags, linkFlags []string, linkDeps android.Paths,
	resDirs, overlayDirs []globbedResourceDir, rroDirs []rroDir, resZips android.Paths) {

	hasVersionCode := android.PrefixInList(a.aaptProperties.Aaptflags, "--version-code")
	hasVersionName := android.PrefixInList(a.aaptProperties.Aaptflags, "--version-name")

	// Flags specified in Android.bp
	linkFlags = append(linkFlags, a.aaptProperties.Aaptflags...)

	linkFlags = append(linkFlags, "--enable-compact-entries")

	// Find implicit or explicit asset and resource dirs
	assets := android.PathsRelativeToModuleSourceDir(android.SourceInput{
		Context:     ctx,
		Paths:       a.aaptProperties.Assets,
		IncludeDirs: false,
	})
	assetDirs := android.PathsWithOptionalDefaultForModuleSrc(ctx, a.aaptProperties.Asset_dirs, "assets")
	resourceDirs := android.PathsWithOptionalDefaultForModuleSrc(ctx, a.aaptProperties.Resource_dirs, "res")
	resourceZips := android.PathsForModuleSrc(ctx, a.aaptProperties.Resource_zips)

	// Glob directories into lists of paths
	for _, dir := range resourceDirs {
		resDirs = append(resDirs, globbedResourceDir{
			dir:   dir,
			files: androidResourceGlob(ctx, dir),
		})
		resOverlayDirs, resRRODirs := overlayResourceGlob(ctx, a, dir)
		overlayDirs = append(overlayDirs, resOverlayDirs...)
		rroDirs = append(rroDirs, resRRODirs...)
	}

	var assetDeps android.Paths
	for i, dir := range assetDirs {
		// Add a dependency on every file in the asset directory.  This ensures the aapt2
		// rule will be rerun if one of the files in the asset directory is modified.
		assetDeps = append(assetDeps, androidResourceGlob(ctx, dir)...)

		// Add a dependency on a file that contains a list of all the files in the asset directory.
		// This ensures the aapt2 rule will be run if a file is removed from the asset directory,
		// or a file is added whose timestamp is older than the output of aapt2.
		assetFileListFile := android.PathForModuleOut(ctx, "asset_dir_globs", strconv.Itoa(i)+".glob")
		androidResourceGlobList(ctx, dir, assetFileListFile)
		assetDeps = append(assetDeps, assetFileListFile)
	}

	assetDirStrings := assetDirs.Strings()
	if a.noticeFile.Valid() {
		assetDirStrings = append(assetDirStrings, filepath.Dir(a.noticeFile.Path().String()))
		assetDeps = append(assetDeps, a.noticeFile.Path())
	}
	if len(assets) > 0 {
		// aapt2 doesn't support adding individual asset files. Create a temp directory to hold asset
		// files and pass it to aapt2.
		tmpAssetDir := android.PathForModuleOut(ctx, "tmp_asset_dir")

		rule := android.NewRuleBuilder(pctx, ctx)
		rule.Command().
			Text("rm -rf").Text(tmpAssetDir.String()).
			Text("&&").
			Text("mkdir -p").Text(tmpAssetDir.String())

		for _, asset := range assets {
			output := tmpAssetDir.Join(ctx, asset.Rel())
			assetDeps = append(assetDeps, output)
			rule.Command().Text("mkdir -p").Text(filepath.Dir(output.String()))
			rule.Command().Text("cp").Input(asset).Output(output)
		}

		rule.Build("tmp_asset_dir", "tmp_asset_dir")

		assetDirStrings = append(assetDirStrings, tmpAssetDir.String())
	}

	linkFlags = append(linkFlags, "--manifest "+manifestPath.String())
	linkDeps = append(linkDeps, manifestPath)

	linkFlags = append(linkFlags, android.JoinWithPrefix(assetDirStrings, "-A "))
	linkDeps = append(linkDeps, assetDeps...)

	// Returns the effective version for {min|target}_sdk_version
	effectiveVersionString := func(sdkVersion android.SdkSpec, minSdkVersion android.ApiLevel) string {
		// If {min|target}_sdk_version is current, use sdk_version to determine the effective level
		// This is necessary for vendor modules.
		// The effective version does not _only_ depend on {min|target}_sdk_version(level),
		// but also on the sdk_version (kind+level)
		if minSdkVersion.IsCurrent() {
			ret, err := sdkVersion.EffectiveVersionString(ctx)
			if err != nil {
				ctx.ModuleErrorf("invalid sdk_version: %s", err)
			}
			return ret
		}
		ret, err := minSdkVersion.EffectiveVersionString(ctx)
		if err != nil {
			ctx.ModuleErrorf("invalid min_sdk_version: %s", err)
		}
		return ret
	}
	// SDK version flags
	sdkVersion := sdkContext.SdkVersion(ctx)
	minSdkVersion := effectiveVersionString(sdkVersion, sdkContext.MinSdkVersion(ctx))

	linkFlags = append(linkFlags, "--min-sdk-version "+minSdkVersion)
	// Use minSdkVersion for target-sdk-version, even if `target_sdk_version` is set
	// This behavior has been copied from Make.
	linkFlags = append(linkFlags, "--target-sdk-version "+minSdkVersion)

	// Version code
	if !hasVersionCode {
		linkFlags = append(linkFlags, "--version-code", ctx.Config().PlatformSdkVersion().String())
	}

	if !hasVersionName {
		var versionName string
		if ctx.ModuleName() == "framework-res" {
			// Some builds set AppsDefaultVersionName() to include the build number ("O-123456").  aapt2 copies the
			// version name of framework-res into app manifests as compileSdkVersionCodename, which confuses things
			// if it contains the build number.  Use the PlatformVersionName instead.
			versionName = ctx.Config().PlatformVersionName()
		} else {
			versionName = ctx.Config().AppsDefaultVersionName()
		}
		versionName = proptools.NinjaEscape(versionName)
		linkFlags = append(linkFlags, "--version-name ", versionName)
	}

	linkFlags, compileFlags = android.FilterList(linkFlags, []string{"--legacy"})

	// Always set --pseudo-localize, it will be stripped out later for release
	// builds that don't want it.
	compileFlags = append(compileFlags, "--pseudo-localize")

	return compileFlags, linkFlags, linkDeps, resDirs, overlayDirs, rroDirs, resourceZips
}

func (a *aapt) deps(ctx android.BottomUpMutatorContext, sdkDep sdkDep) {
	if sdkDep.frameworkResModule != "" {
		ctx.AddVariationDependencies(nil, frameworkResTag, sdkDep.frameworkResModule)
	}
}

var extractAssetsRule = pctx.AndroidStaticRule("extractAssets",
	blueprint.RuleParams{
		Command:     `${config.Zip2ZipCmd} -i ${in} -o ${out} "assets/**/*"`,
		CommandDeps: []string{"${config.Zip2ZipCmd}"},
	})

type aaptBuildActionOptions struct {
	sdkContext                     android.SdkContext
	classLoaderContexts            dexpreopt.ClassLoaderContextMap
	excludedLibs                   []string
	enforceDefaultTargetSdkVersion bool
	extraLinkFlags                 []string
	aconfigTextFiles               android.Paths
}

func (a *aapt) buildActions(ctx android.ModuleContext, opts aaptBuildActionOptions) {

	staticResourcesNodesDepSet, sharedResourcesNodesDepSet, staticRRODirsDepSet, staticManifestsDepSet, sharedExportPackages, libFlags :=
		aaptLibs(ctx, opts.sdkContext, opts.classLoaderContexts)

	// Exclude any libraries from the supplied list.
	opts.classLoaderContexts = opts.classLoaderContexts.ExcludeLibs(opts.excludedLibs)

	// App manifest file
	manifestFile := proptools.StringDefault(a.aaptProperties.Manifest, "AndroidManifest.xml")
	manifestSrcPath := android.PathForModuleSrc(ctx, manifestFile)

	manifestPath := ManifestFixer(ctx, manifestSrcPath, ManifestFixerParams{
		SdkContext:                     opts.sdkContext,
		ClassLoaderContexts:            opts.classLoaderContexts,
		IsLibrary:                      a.isLibrary,
		DefaultManifestVersion:         a.defaultManifestVersion,
		UseEmbeddedNativeLibs:          a.useEmbeddedNativeLibs,
		UsesNonSdkApis:                 a.usesNonSdkApis,
		UseEmbeddedDex:                 a.useEmbeddedDex,
		HasNoCode:                      a.hasNoCode,
		LoggingParent:                  a.LoggingParent,
		EnforceDefaultTargetSdkVersion: opts.enforceDefaultTargetSdkVersion,
	})

	staticDeps := transitiveAarDeps(staticResourcesNodesDepSet.ToList())
	sharedDeps := transitiveAarDeps(sharedResourcesNodesDepSet.ToList())

	// Add additional manifest files to transitive manifests.
	additionalManifests := android.PathsForModuleSrc(ctx, a.aaptProperties.Additional_manifests)
	transitiveManifestPaths := append(android.Paths{manifestPath}, additionalManifests...)
	// TODO(b/288358614): Soong has historically not merged manifests from dependencies of android_library_import
	// modules.  Merging manifests from dependencies could remove the need for pom2bp to generate the "-nodeps" copies
	// of androidx libraries, but doing so triggers errors due to errors introduced by existing dependencies of
	// android_library_import modules.  If this is fixed, staticManifestsDepSet can be dropped completely in favor of
	// staticResourcesNodesDepSet.manifests()
	transitiveManifestPaths = append(transitiveManifestPaths, staticManifestsDepSet.ToList()...)

	if len(transitiveManifestPaths) > 1 && !Bool(a.aaptProperties.Dont_merge_manifests) {
		manifestMergerParams := ManifestMergerParams{
			staticLibManifests: transitiveManifestPaths[1:],
			isLibrary:          a.isLibrary,
			packageName:        a.manifestValues.applicationId,
		}
		a.mergedManifestFile = manifestMerger(ctx, transitiveManifestPaths[0], manifestMergerParams)
		if !a.isLibrary {
			// Only use the merged manifest for applications.  For libraries, the transitive closure of manifests
			// will be propagated to the final application and merged there.  The merged manifest for libraries is
			// only passed to Make, which can't handle transitive dependencies.
			manifestPath = a.mergedManifestFile
		}
	} else {
		a.mergedManifestFile = manifestPath
	}

	compileFlags, linkFlags, linkDeps, resDirs, overlayDirs, rroDirs, resZips := a.aapt2Flags(ctx, opts.sdkContext, manifestPath)

	linkFlags = append(linkFlags, libFlags...)
	linkDeps = append(linkDeps, sharedExportPackages...)
	linkDeps = append(linkDeps, staticDeps.resPackages()...)
	linkFlags = append(linkFlags, opts.extraLinkFlags...)
	if a.isLibrary {
		linkFlags = append(linkFlags, "--static-lib")
	}

	if a.isLibrary && a.useResourceProcessorBusyBox() {
		// When building an android_library using ResourceProcessorBusyBox the resources are merged into
		// package-res.apk with --merge-only, but --no-static-lib-packages is not used so that R.txt only
		// contains resources from this library.
		linkFlags = append(linkFlags, "--merge-only")
	} else {
		// When building and app or when building an android_library without ResourceProcessorBusyBox
		// --no-static-lib-packages is used to put all the resources into the app.  If ResourceProcessorBusyBox
		// is used then the app's R.txt will be post-processed along with the R.txt files from dependencies to
		// sort resources into the right packages in R.class.
		linkFlags = append(linkFlags, "--no-static-lib-packages")
	}

	packageRes := android.PathForModuleOut(ctx, "package-res.apk")
	proguardOptionsFile := android.PathForModuleGen(ctx, "proguard.options")
	rTxt := android.PathForModuleOut(ctx, "R.txt")
	// This file isn't used by Soong, but is generated for exporting
	extraPackages := android.PathForModuleOut(ctx, "extra_packages")
	var transitiveRJars android.Paths
	var srcJar android.WritablePath

	var compiledResDirs []android.Paths
	for _, dir := range resDirs {
		a.resourceFiles = append(a.resourceFiles, dir.files...)
		compiledResDirs = append(compiledResDirs, aapt2Compile(ctx, dir.dir, dir.files, compileFlags, a.filterProduct()).Paths())
	}

	for i, zip := range resZips {
		flata := android.PathForModuleOut(ctx, fmt.Sprintf("reszip.%d.flata", i))
		aapt2CompileZip(ctx, flata, zip, "", compileFlags)
		compiledResDirs = append(compiledResDirs, android.Paths{flata})
	}

	var compiledRes, compiledOverlay android.Paths

	// AAPT2 overlays are in lowest to highest priority order, reverse the topological order
	// of transitiveStaticLibs.
	transitiveStaticLibs := android.ReversePaths(staticDeps.resPackages())

	if a.isLibrary && a.useResourceProcessorBusyBox() {
		// When building an android_library with ResourceProcessorBusyBox enabled treat static library dependencies
		// as imports.  The resources from dependencies will not be merged into this module's package-res.apk, and
		// instead modules depending on this module will reference package-res.apk from all transitive static
		// dependencies.
		for _, staticDep := range staticDeps {
			linkDeps = append(linkDeps, staticDep.resPackage)
			linkFlags = append(linkFlags, "-I "+staticDep.resPackage.String())
			if staticDep.usedResourceProcessor {
				transitiveRJars = append(transitiveRJars, staticDep.rJar)
			}
		}
		for _, sharedDep := range sharedDeps {
			if sharedDep.usedResourceProcessor {
				transitiveRJars = append(transitiveRJars, sharedDep.rJar)
			}
		}
	} else {
		// When building an app or building a library without ResourceProcessorBusyBox enabled all static
		// dependencies are compiled into this module's package-res.apk as overlays.
		compiledOverlay = append(compiledOverlay, transitiveStaticLibs...)
	}

	if len(transitiveStaticLibs) > 0 {
		// If we are using static android libraries, every source file becomes an overlay.
		// This is to emulate old AAPT behavior which simulated library support.
		for _, compiledResDir := range compiledResDirs {
			compiledOverlay = append(compiledOverlay, compiledResDir...)
		}
	} else if a.isLibrary {
		// Otherwise, for a static library we treat all the resources equally with no overlay.
		for _, compiledResDir := range compiledResDirs {
			compiledRes = append(compiledRes, compiledResDir...)
		}
	} else if len(compiledResDirs) > 0 {
		// Without static libraries, the first directory is our directory, which can then be
		// overlaid by the rest.
		compiledRes = append(compiledRes, compiledResDirs[0]...)
		for _, compiledResDir := range compiledResDirs[1:] {
			compiledOverlay = append(compiledOverlay, compiledResDir...)
		}
	}

	for _, dir := range overlayDirs {
		compiledOverlay = append(compiledOverlay, aapt2Compile(ctx, dir.dir, dir.files, compileFlags, a.filterProduct()).Paths()...)
	}

	var splitPackages android.WritablePaths
	var splits []split

	for _, s := range a.splitNames {
		suffix := strings.Replace(s, ",", "_", -1)
		path := android.PathForModuleOut(ctx, "package_"+suffix+".apk")
		linkFlags = append(linkFlags, "--split", path.String()+":"+s)
		splitPackages = append(splitPackages, path)
		splits = append(splits, split{
			name:   s,
			suffix: suffix,
			path:   path,
		})
	}

	if !a.useResourceProcessorBusyBox() {
		// the subdir "android" is required to be filtered by package names
		srcJar = android.PathForModuleGen(ctx, "android", "R.srcjar")
	}

	// No need to specify assets from dependencies to aapt2Link for libraries, all transitive assets will be
	// provided to the final app aapt2Link step.
	var transitiveAssets android.Paths
	if !a.isLibrary {
		transitiveAssets = android.ReverseSliceInPlace(staticDeps.assets())
	}
	aapt2Link(ctx, packageRes, srcJar, proguardOptionsFile, rTxt,
		linkFlags, linkDeps, compiledRes, compiledOverlay, transitiveAssets, splitPackages,
		opts.aconfigTextFiles)
	// Extract assets from the resource package output so that they can be used later in aapt2link
	// for modules that depend on this one.
	if android.PrefixInList(linkFlags, "-A ") {
		assets := android.PathForModuleOut(ctx, "assets.zip")
		ctx.Build(pctx, android.BuildParams{
			Rule:        extractAssetsRule,
			Input:       packageRes,
			Output:      assets,
			Description: "extract assets from built resource file",
		})
		a.assetPackage = android.OptionalPathForPath(assets)
	}

	if a.useResourceProcessorBusyBox() {
		rJar := android.PathForModuleOut(ctx, "busybox/R.jar")
		resourceProcessorBusyBoxGenerateBinaryR(ctx, rTxt, a.mergedManifestFile, rJar, staticDeps, a.isLibrary)
		aapt2ExtractExtraPackages(ctx, extraPackages, rJar)
		transitiveRJars = append(transitiveRJars, rJar)
		a.rJar = rJar
	} else {
		aapt2ExtractExtraPackages(ctx, extraPackages, srcJar)
	}

	transitiveAaptResourcePackages := staticDeps.resPackages().Strings()
	transitiveAaptResourcePackages = slices.DeleteFunc(transitiveAaptResourcePackages, func(p string) bool {
		return p == packageRes.String()
	})
	transitiveAaptResourcePackagesFile := android.PathForModuleOut(ctx, "transitive-res-packages")
	android.WriteFileRule(ctx, transitiveAaptResourcePackagesFile, strings.Join(transitiveAaptResourcePackages, "\n"))

	a.aaptSrcJar = srcJar
	a.transitiveAaptRJars = transitiveRJars
	a.transitiveAaptResourcePackagesFile = transitiveAaptResourcePackagesFile
	a.exportPackage = packageRes
	a.manifestPath = manifestPath
	a.proguardOptionsFile = proguardOptionsFile
	a.extraAaptPackagesFile = extraPackages
	a.rTxt = rTxt
	a.splits = splits
	a.resourcesNodesDepSet = android.NewDepSetBuilder[*resourcesNode](android.TOPOLOGICAL).
		Direct(&resourcesNode{
			resPackage:          a.exportPackage,
			manifest:            a.manifestPath,
			additionalManifests: additionalManifests,
			rTxt:                a.rTxt,
			rJar:                a.rJar,
			assets:              a.assetPackage,

			usedResourceProcessor: a.useResourceProcessorBusyBox(),
		}).
		Transitive(staticResourcesNodesDepSet).Build()
	a.rroDirsDepSet = android.NewDepSetBuilder[rroDir](android.TOPOLOGICAL).
		Direct(rroDirs...).
		Transitive(staticRRODirsDepSet).Build()
	a.manifestsDepSet = android.NewDepSetBuilder[android.Path](android.TOPOLOGICAL).
		Direct(a.manifestPath).
		DirectSlice(additionalManifests).
		Transitive(staticManifestsDepSet).Build()
}

var resourceProcessorBusyBox = pctx.AndroidStaticRule("resourceProcessorBusyBox",
	blueprint.RuleParams{
		Command: "${config.JavaCmd} -cp ${config.ResourceProcessorBusyBox} " +
			"com.google.devtools.build.android.ResourceProcessorBusyBox --tool=GENERATE_BINARY_R -- @${out}.args && " +
			"if cmp -s ${out}.tmp ${out} ; then rm ${out}.tmp ; else mv ${out}.tmp ${out}; fi",
		CommandDeps:    []string{"${config.ResourceProcessorBusyBox}"},
		Rspfile:        "${out}.args",
		RspfileContent: "--primaryRTxt ${rTxt} --primaryManifest ${manifest} --classJarOutput ${out}.tmp ${args}",
		Restat:         true,
	}, "rTxt", "manifest", "args")

// resourceProcessorBusyBoxGenerateBinaryR converts the R.txt file produced by aapt2 into R.class files
// using Bazel's ResourceProcessorBusyBox tool, which is faster than compiling the R.java files and
// supports producing classes for static dependencies that only include resources from that dependency.
func resourceProcessorBusyBoxGenerateBinaryR(ctx android.ModuleContext, rTxt, manifest android.Path,
	rJar android.WritablePath, transitiveDeps transitiveAarDeps, isLibrary bool) {

	var args []string
	var deps android.Paths

	if !isLibrary {
		// When compiling an app, pass all R.txt and AndroidManifest.xml from transitive static library dependencies
		// to ResourceProcessorBusyBox so that it can regenerate R.class files with the final resource IDs for each
		// package.
		args, deps = transitiveDeps.resourceProcessorDeps()
	} else {
		// When compiling a library don't pass any dependencies as it only needs to generate an R.class file for this
		// library.  Pass --finalFields=false so that the R.class file contains non-final fields so they don't get
		// inlined into the library before the final IDs are assigned during app compilation.
		args = append(args, "--finalFields=false")
	}

	deps = append(deps, rTxt, manifest)

	ctx.Build(pctx, android.BuildParams{
		Rule:        resourceProcessorBusyBox,
		Output:      rJar,
		Implicits:   deps,
		Description: "ResourceProcessorBusyBox",
		Args: map[string]string{
			"rTxt":     rTxt.String(),
			"manifest": manifest.String(),
			"args":     strings.Join(args, " "),
		},
	})
}

type resourcesNode struct {
	resPackage          android.Path
	manifest            android.Path
	additionalManifests android.Paths
	rTxt                android.Path
	rJar                android.Path
	assets              android.OptionalPath

	usedResourceProcessor bool
}

type transitiveAarDeps []*resourcesNode

func (t transitiveAarDeps) resPackages() android.Paths {
	paths := make(android.Paths, 0, len(t))
	for _, dep := range t {
		paths = append(paths, dep.resPackage)
	}
	return paths
}

func (t transitiveAarDeps) manifests() android.Paths {
	paths := make(android.Paths, 0, len(t))
	for _, dep := range t {
		paths = append(paths, dep.manifest)
		paths = append(paths, dep.additionalManifests...)
	}
	return paths
}

func (t transitiveAarDeps) resourceProcessorDeps() (args []string, deps android.Paths) {
	for _, dep := range t {
		args = append(args, "--library="+dep.rTxt.String()+","+dep.manifest.String())
		deps = append(deps, dep.rTxt, dep.manifest)
	}
	return args, deps
}

func (t transitiveAarDeps) assets() android.Paths {
	paths := make(android.Paths, 0, len(t))
	for _, dep := range t {
		if dep.assets.Valid() {
			paths = append(paths, dep.assets.Path())
		}
	}
	return paths
}

// aaptLibs collects libraries from dependencies and sdk_version and converts them into paths
func aaptLibs(ctx android.ModuleContext, sdkContext android.SdkContext, classLoaderContexts dexpreopt.ClassLoaderContextMap) (
	staticResourcesNodes, sharedResourcesNodes *android.DepSet[*resourcesNode], staticRRODirs *android.DepSet[rroDir],
	staticManifests *android.DepSet[android.Path], sharedLibs android.Paths, flags []string) {

	if classLoaderContexts == nil {
		// Not all callers need to compute class loader context, those who don't just pass nil.
		// Create a temporary class loader context here (it will be computed, but not used).
		classLoaderContexts = make(dexpreopt.ClassLoaderContextMap)
	}

	sdkDep := decodeSdkDep(ctx, sdkContext)
	if sdkDep.useFiles {
		sharedLibs = append(sharedLibs, sdkDep.jars...)
	}

	var staticResourcesNodeDepSets []*android.DepSet[*resourcesNode]
	var sharedResourcesNodeDepSets []*android.DepSet[*resourcesNode]
	rroDirsDepSetBuilder := android.NewDepSetBuilder[rroDir](android.TOPOLOGICAL)
	manifestsDepSetBuilder := android.NewDepSetBuilder[android.Path](android.TOPOLOGICAL)

	ctx.VisitDirectDeps(func(module android.Module) {
		depTag := ctx.OtherModuleDependencyTag(module)

		var exportPackage android.Path
		aarDep, _ := module.(AndroidLibraryDependency)
		if aarDep != nil {
			exportPackage = aarDep.ExportPackage()
		}

		switch depTag {
		case instrumentationForTag:
			// Nothing, instrumentationForTag is treated as libTag for javac but not for aapt2.
		case sdkLibTag, libTag:
			if exportPackage != nil {
				sharedResourcesNodeDepSets = append(sharedResourcesNodeDepSets, aarDep.ResourcesNodeDepSet())
				sharedLibs = append(sharedLibs, exportPackage)
			}
		case frameworkResTag:
			if exportPackage != nil {
				sharedLibs = append(sharedLibs, exportPackage)
			}
		case staticLibTag:
			if exportPackage != nil {
				staticResourcesNodeDepSets = append(staticResourcesNodeDepSets, aarDep.ResourcesNodeDepSet())
				rroDirsDepSetBuilder.Transitive(aarDep.RRODirsDepSet())
				manifestsDepSetBuilder.Transitive(aarDep.ManifestsDepSet())
			}
		}

		addCLCFromDep(ctx, module, classLoaderContexts)
	})

	// AAPT2 overlays are in lowest to highest priority order, the topological order will be reversed later.
	// Reverse the dependency order now going into the depset so that it comes out in order after the second
	// reverse later.
	// NOTE: this is legacy and probably incorrect behavior, for most other cases (e.g. conflicting classes in
	// dependencies) the highest priority dependency is listed first, but for resources the highest priority
	// dependency has to be listed last.
	staticResourcesNodes = android.NewDepSet(android.TOPOLOGICAL, nil,
		android.ReverseSliceInPlace(staticResourcesNodeDepSets))
	sharedResourcesNodes = android.NewDepSet(android.TOPOLOGICAL, nil,
		android.ReverseSliceInPlace(sharedResourcesNodeDepSets))

	staticRRODirs = rroDirsDepSetBuilder.Build()
	staticManifests = manifestsDepSetBuilder.Build()

	if len(staticResourcesNodes.ToList()) > 0 {
		flags = append(flags, "--auto-add-overlay")
	}

	for _, sharedLib := range sharedLibs {
		flags = append(flags, "-I "+sharedLib.String())
	}

	return staticResourcesNodes, sharedResourcesNodes, staticRRODirs, staticManifests, sharedLibs, flags
}

type AndroidLibrary struct {
	Library
	aapt

	androidLibraryProperties androidLibraryProperties

	aarFile android.WritablePath
}

var _ android.OutputFileProducer = (*AndroidLibrary)(nil)

// For OutputFileProducer interface
func (a *AndroidLibrary) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case ".aar":
		return []android.Path{a.aarFile}, nil
	default:
		return a.Library.OutputFiles(tag)
	}
}

var _ AndroidLibraryDependency = (*AndroidLibrary)(nil)

func (a *AndroidLibrary) DepsMutator(ctx android.BottomUpMutatorContext) {
	a.Module.deps(ctx)
	sdkDep := decodeSdkDep(ctx, android.SdkContext(a))
	if sdkDep.hasFrameworkLibs() {
		a.aapt.deps(ctx, sdkDep)
	}
	a.usesLibrary.deps(ctx, false)
}

func (a *AndroidLibrary) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	a.aapt.isLibrary = true
	a.classLoaderContexts = a.usesLibrary.classLoaderContextForUsesLibDeps(ctx)
	a.aapt.buildActions(ctx,
		aaptBuildActionOptions{
			sdkContext:                     android.SdkContext(a),
			classLoaderContexts:            a.classLoaderContexts,
			enforceDefaultTargetSdkVersion: false,
		},
	)

	a.hideApexVariantFromMake = !ctx.Provider(android.ApexInfoProvider).(android.ApexInfo).IsForPlatform()

	a.stem = proptools.StringDefault(a.overridableDeviceProperties.Stem, ctx.ModuleName())

	ctx.CheckbuildFile(a.aapt.proguardOptionsFile)
	ctx.CheckbuildFile(a.aapt.exportPackage)
	if a.useResourceProcessorBusyBox() {
		ctx.CheckbuildFile(a.aapt.rJar)
	} else {
		ctx.CheckbuildFile(a.aapt.aaptSrcJar)
	}

	// apps manifests are handled by aapt, don't let Module see them
	a.properties.Manifest = nil

	a.linter.mergedManifest = a.aapt.mergedManifestFile
	a.linter.manifest = a.aapt.manifestPath
	a.linter.resources = a.aapt.resourceFiles

	proguardSpecInfo := a.collectProguardSpecInfo(ctx)
	ctx.SetProvider(ProguardSpecInfoProvider, proguardSpecInfo)
	exportedProguardFlagsFiles := proguardSpecInfo.ProguardFlagsFiles.ToList()
	a.extraProguardFlagsFiles = append(a.extraProguardFlagsFiles, exportedProguardFlagsFiles...)
	a.extraProguardFlagsFiles = append(a.extraProguardFlagsFiles, a.proguardOptionsFile)

	combinedExportedProguardFlagFile := android.PathForModuleOut(ctx, "export_proguard_flags")
	writeCombinedProguardFlagsFile(ctx, combinedExportedProguardFlagFile, exportedProguardFlagsFiles)
	a.combinedExportedProguardFlagsFile = combinedExportedProguardFlagFile

	var extraSrcJars android.Paths
	var extraCombinedJars android.Paths
	var extraClasspathJars android.Paths
	if a.useResourceProcessorBusyBox() {
		// When building a library with ResourceProcessorBusyBox enabled ResourceProcessorBusyBox for this
		// library and each of the transitive static android_library dependencies has already created an
		// R.class file for the appropriate package.  Add all of those R.class files to the classpath.
		extraClasspathJars = a.transitiveAaptRJars
	} else {
		// When building a library without ResourceProcessorBusyBox the aapt2 rule creates R.srcjar containing
		// R.java files for the library's package and the packages from all transitive static android_library
		// dependencies.  Compile the srcjar alongside the rest of the sources.
		extraSrcJars = android.Paths{a.aapt.aaptSrcJar}
	}

	a.Module.compile(ctx, extraSrcJars, extraClasspathJars, extraCombinedJars)

	a.aarFile = android.PathForModuleOut(ctx, ctx.ModuleName()+".aar")
	var res android.Paths
	if a.androidLibraryProperties.BuildAAR {
		BuildAAR(ctx, a.aarFile, a.outputFile, a.manifestPath, a.rTxt, res)
		ctx.CheckbuildFile(a.aarFile)
	}

	prebuiltJniPackages := android.Paths{}
	ctx.VisitDirectDeps(func(module android.Module) {
		if info, ok := ctx.OtherModuleProvider(module, JniPackageProvider).(JniPackageInfo); ok {
			prebuiltJniPackages = append(prebuiltJniPackages, info.JniPackages...)
		}
	})
	if len(prebuiltJniPackages) > 0 {
		ctx.SetProvider(JniPackageProvider, JniPackageInfo{
			JniPackages: prebuiltJniPackages,
		})
	}
}

func (a *AndroidLibrary) IDEInfo(dpInfo *android.IdeInfo) {
	a.Library.IDEInfo(dpInfo)
	a.aapt.IDEInfo(dpInfo)
}

func (a *aapt) IDEInfo(dpInfo *android.IdeInfo) {
	if a.useResourceProcessorBusyBox() {
		dpInfo.Jars = append(dpInfo.Jars, a.rJar.String())
	}
}

// android_library builds and links sources into a `.jar` file for the device along with Android resources.
//
// An android_library has a single variant that produces a `.jar` file containing `.class` files that were
// compiled against the device bootclasspath, along with a `package-res.apk` file containing Android resources compiled
// with aapt2.  This module is not suitable for installing on a device, but can be used as a `static_libs` dependency of
// an android_app module.
func AndroidLibraryFactory() android.Module {
	module := &AndroidLibrary{}

	module.Module.addHostAndDeviceProperties()
	module.AddProperties(
		&module.aaptProperties,
		&module.androidLibraryProperties)

	module.androidLibraryProperties.BuildAAR = true
	module.Module.linter.library = true

	android.InitApexModule(module)
	InitJavaModule(module, android.DeviceSupported)
	return module
}

//
// AAR (android library) prebuilts
//

// Properties for android_library_import
type AARImportProperties struct {
	// ARR (android library prebuilt) filepath. Exactly one ARR is required.
	Aars []string `android:"path"`
	// If not blank, set to the version of the sdk to compile against.
	// Defaults to private.
	// Values are of one of the following forms:
	// 1) numerical API level, "current", "none", or "core_platform"
	// 2) An SDK kind with an API level: "<sdk kind>_<API level>"
	// See build/soong/android/sdk_version.go for the complete and up to date list of SDK kinds.
	// If the SDK kind is empty, it will be set to public
	Sdk_version *string
	// If not blank, set the minimum version of the sdk that the compiled artifacts will run against.
	// Defaults to sdk_version if not set. See sdk_version for possible values.
	Min_sdk_version *string
	// List of java static libraries that the included ARR (android library prebuilts) has dependencies to.
	Static_libs []string
	// List of java libraries that the included ARR (android library prebuilts) has dependencies to.
	Libs []string
	// If set to true, run Jetifier against .aar file. Defaults to false.
	Jetifier *bool
	// If true, extract JNI libs from AAR archive. These libs will be accessible to android_app modules and
	// will be passed transitively through android_libraries to an android_app.
	//TODO(b/241138093) evaluate whether we can have this flag default to true for Bazel conversion
	Extract_jni *bool
}

type AARImport struct {
	android.ModuleBase
	android.DefaultableModuleBase
	android.ApexModuleBase
	prebuilt android.Prebuilt

	// Functionality common to Module and Import.
	embeddableInModuleAndImport

	providesTransitiveHeaderJars

	properties AARImportProperties

	classpathFile                      android.WritablePath
	proguardFlags                      android.WritablePath
	exportPackage                      android.WritablePath
	transitiveAaptResourcePackagesFile android.Path
	extraAaptPackagesFile              android.WritablePath
	manifest                           android.WritablePath
	assetsPackage                      android.WritablePath
	rTxt                               android.WritablePath
	rJar                               android.WritablePath

	resourcesNodesDepSet *android.DepSet[*resourcesNode]
	manifestsDepSet      *android.DepSet[android.Path]

	hideApexVariantFromMake bool

	aarPath     android.Path
	jniPackages android.Paths

	sdkVersion    android.SdkSpec
	minSdkVersion android.ApiLevel
}

var _ android.OutputFileProducer = (*AARImport)(nil)

// For OutputFileProducer interface
func (a *AARImport) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case ".aar":
		return []android.Path{a.aarPath}, nil
	case "":
		return []android.Path{a.classpathFile}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

func (a *AARImport) SdkVersion(ctx android.EarlyModuleContext) android.SdkSpec {
	return android.SdkSpecFrom(ctx, String(a.properties.Sdk_version))
}

func (a *AARImport) SystemModules() string {
	return ""
}

func (a *AARImport) MinSdkVersion(ctx android.EarlyModuleContext) android.ApiLevel {
	if a.properties.Min_sdk_version != nil {
		return android.ApiLevelFrom(ctx, *a.properties.Min_sdk_version)
	}
	return a.SdkVersion(ctx).ApiLevel
}

func (a *AARImport) ReplaceMaxSdkVersionPlaceholder(ctx android.EarlyModuleContext) android.ApiLevel {
	return android.SdkSpecFrom(ctx, "").ApiLevel
}

func (a *AARImport) TargetSdkVersion(ctx android.EarlyModuleContext) android.ApiLevel {
	return a.SdkVersion(ctx).ApiLevel
}

func (a *AARImport) javaVersion() string {
	return ""
}

var _ AndroidLibraryDependency = (*AARImport)(nil)

func (a *AARImport) ExportPackage() android.Path {
	return a.exportPackage
}
func (a *AARImport) ResourcesNodeDepSet() *android.DepSet[*resourcesNode] {
	return a.resourcesNodesDepSet
}

func (a *AARImport) RRODirsDepSet() *android.DepSet[rroDir] {
	return android.NewDepSet[rroDir](android.TOPOLOGICAL, nil, nil)
}

func (a *AARImport) ManifestsDepSet() *android.DepSet[android.Path] {
	return a.manifestsDepSet
}

// RRO enforcement is not available on aar_import since its RRO dirs are not
// exported.
func (a *AARImport) SetRROEnforcedForDependent(enforce bool) {
}

// RRO enforcement is not available on aar_import since its RRO dirs are not
// exported.
func (a *AARImport) IsRROEnforced(ctx android.BaseModuleContext) bool {
	return false
}

func (a *AARImport) Prebuilt() *android.Prebuilt {
	return &a.prebuilt
}

func (a *AARImport) Name() string {
	return a.prebuilt.Name(a.ModuleBase.Name())
}

func (a *AARImport) JacocoReportClassesFile() android.Path {
	return nil
}

func (a *AARImport) DepsMutator(ctx android.BottomUpMutatorContext) {
	if !ctx.Config().AlwaysUsePrebuiltSdks() {
		sdkDep := decodeSdkDep(ctx, android.SdkContext(a))
		if sdkDep.useModule && sdkDep.frameworkResModule != "" {
			ctx.AddVariationDependencies(nil, frameworkResTag, sdkDep.frameworkResModule)
		}
	}

	ctx.AddVariationDependencies(nil, libTag, a.properties.Libs...)
	ctx.AddVariationDependencies(nil, staticLibTag, a.properties.Static_libs...)
}

type JniPackageInfo struct {
	// List of zip files containing JNI libraries
	// Zip files should have directory structure jni/<arch>/*.so
	JniPackages android.Paths
}

var JniPackageProvider = blueprint.NewProvider(JniPackageInfo{})

// Unzip an AAR and extract the JNI libs for $archString.
var extractJNI = pctx.AndroidStaticRule("extractJNI",
	blueprint.RuleParams{
		Command: `rm -rf $out $outDir && touch $out && ` +
			`unzip -qoDD -d $outDir $in "jni/${archString}/*" && ` +
			`jni_files=$$(find $outDir/jni -type f) && ` +
			// print error message if there are no JNI libs for this arch
			`[ -n "$$jni_files" ] || (echo "ERROR: no JNI libs found for arch ${archString}" && exit 1) && ` +
			`${config.SoongZipCmd} -o $out -L 0 -P 'lib/${archString}' ` +
			`-C $outDir/jni/${archString} $$(echo $$jni_files | xargs -n1 printf " -f %s")`,
		CommandDeps: []string{"${config.SoongZipCmd}"},
	},
	"outDir", "archString")

// Unzip an AAR into its constituent files and directories.  Any files in Outputs that don't exist in the AAR will be
// touched to create an empty file. The res directory is not extracted, as it will be extracted in its own rule.
var unzipAAR = pctx.AndroidStaticRule("unzipAAR",
	blueprint.RuleParams{
		Command: `rm -rf $outDir && mkdir -p $outDir && ` +
			`unzip -qoDD -d $outDir $in && rm -rf $outDir/res && touch $out && ` +
			`${config.Zip2ZipCmd} -i $in -o $assetsPackage 'assets/**/*' && ` +
			`${config.MergeZipsCmd} $combinedClassesJar $$(ls $outDir/classes.jar 2> /dev/null) $$(ls $outDir/libs/*.jar 2> /dev/null)`,
		CommandDeps: []string{"${config.MergeZipsCmd}", "${config.Zip2ZipCmd}"},
	},
	"outDir", "combinedClassesJar", "assetsPackage")

func (a *AARImport) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if len(a.properties.Aars) != 1 {
		ctx.PropertyErrorf("aars", "exactly one aar is required")
		return
	}

	a.sdkVersion = a.SdkVersion(ctx)
	a.minSdkVersion = a.MinSdkVersion(ctx)

	a.hideApexVariantFromMake = !ctx.Provider(android.ApexInfoProvider).(android.ApexInfo).IsForPlatform()

	aarName := ctx.ModuleName() + ".aar"
	a.aarPath = android.PathForModuleSrc(ctx, a.properties.Aars[0])

	if Bool(a.properties.Jetifier) {
		inputFile := a.aarPath
		a.aarPath = android.PathForModuleOut(ctx, "jetifier", aarName)
		TransformJetifier(ctx, a.aarPath.(android.WritablePath), inputFile)
	}

	extractedAARDir := android.PathForModuleOut(ctx, "aar")
	a.classpathFile = extractedAARDir.Join(ctx, "classes-combined.jar")
	a.manifest = extractedAARDir.Join(ctx, "AndroidManifest.xml")
	aarRTxt := extractedAARDir.Join(ctx, "R.txt")
	a.assetsPackage = android.PathForModuleOut(ctx, "assets.zip")
	a.proguardFlags = extractedAARDir.Join(ctx, "proguard.txt")
	ctx.SetProvider(ProguardSpecInfoProvider, ProguardSpecInfo{
		ProguardFlagsFiles: android.NewDepSet[android.Path](
			android.POSTORDER,
			android.Paths{a.proguardFlags},
			nil,
		),
	})

	ctx.Build(pctx, android.BuildParams{
		Rule:        unzipAAR,
		Input:       a.aarPath,
		Outputs:     android.WritablePaths{a.classpathFile, a.proguardFlags, a.manifest, a.assetsPackage, aarRTxt},
		Description: "unzip AAR",
		Args: map[string]string{
			"outDir":             extractedAARDir.String(),
			"combinedClassesJar": a.classpathFile.String(),
			"assetsPackage":      a.assetsPackage.String(),
		},
	})

	// Always set --pseudo-localize, it will be stripped out later for release
	// builds that don't want it.
	compileFlags := []string{"--pseudo-localize"}
	compiledResDir := android.PathForModuleOut(ctx, "flat-res")
	flata := compiledResDir.Join(ctx, "gen_res.flata")
	aapt2CompileZip(ctx, flata, a.aarPath, "res", compileFlags)

	a.exportPackage = android.PathForModuleOut(ctx, "package-res.apk")
	proguardOptionsFile := android.PathForModuleGen(ctx, "proguard.options")
	a.rTxt = android.PathForModuleOut(ctx, "R.txt")
	a.extraAaptPackagesFile = android.PathForModuleOut(ctx, "extra_packages")

	var linkDeps android.Paths

	linkFlags := []string{
		"--static-lib",
		"--merge-only",
		"--auto-add-overlay",
	}

	linkFlags = append(linkFlags, "--manifest "+a.manifest.String())
	linkDeps = append(linkDeps, a.manifest)

	staticResourcesNodesDepSet, sharedResourcesNodesDepSet, staticRRODirsDepSet, staticManifestsDepSet, sharedLibs, libFlags :=
		aaptLibs(ctx, android.SdkContext(a), nil)

	_ = sharedResourcesNodesDepSet
	_ = staticRRODirsDepSet

	staticDeps := transitiveAarDeps(staticResourcesNodesDepSet.ToList())

	linkDeps = append(linkDeps, sharedLibs...)
	linkDeps = append(linkDeps, staticDeps.resPackages()...)
	linkFlags = append(linkFlags, libFlags...)

	overlayRes := android.Paths{flata}

	// Treat static library dependencies of static libraries as imports.
	transitiveStaticLibs := staticDeps.resPackages()
	linkDeps = append(linkDeps, transitiveStaticLibs...)
	for _, staticLib := range transitiveStaticLibs {
		linkFlags = append(linkFlags, "-I "+staticLib.String())
	}

	transitiveAssets := android.ReverseSliceInPlace(staticDeps.assets())
	aapt2Link(ctx, a.exportPackage, nil, proguardOptionsFile, a.rTxt,
		linkFlags, linkDeps, nil, overlayRes, transitiveAssets, nil, nil)

	a.rJar = android.PathForModuleOut(ctx, "busybox/R.jar")
	resourceProcessorBusyBoxGenerateBinaryR(ctx, a.rTxt, a.manifest, a.rJar, nil, true)

	aapt2ExtractExtraPackages(ctx, a.extraAaptPackagesFile, a.rJar)

	resourcesNodesDepSetBuilder := android.NewDepSetBuilder[*resourcesNode](android.TOPOLOGICAL)
	resourcesNodesDepSetBuilder.Direct(&resourcesNode{
		resPackage: a.exportPackage,
		manifest:   a.manifest,
		rTxt:       a.rTxt,
		rJar:       a.rJar,
		assets:     android.OptionalPathForPath(a.assetsPackage),

		usedResourceProcessor: true,
	})
	resourcesNodesDepSetBuilder.Transitive(staticResourcesNodesDepSet)
	a.resourcesNodesDepSet = resourcesNodesDepSetBuilder.Build()

	manifestDepSetBuilder := android.NewDepSetBuilder[android.Path](android.TOPOLOGICAL).Direct(a.manifest)
	// TODO(b/288358614): Soong has historically not merged manifests from dependencies of android_library_import
	// modules.  Merging manifests from dependencies could remove the need for pom2bp to generate the "-nodeps" copies
	// of androidx libraries, but doing so triggers errors due to errors introduced by existing dependencies of
	// android_library_import modules.  If this is fixed, AndroidLibraryDependency.ManifestsDepSet can be dropped
	// completely in favor of AndroidLibraryDependency.ResourceNodesDepSet.manifest
	//manifestDepSetBuilder.Transitive(transitiveStaticDeps.manifests)
	_ = staticManifestsDepSet
	a.manifestsDepSet = manifestDepSetBuilder.Build()

	transitiveAaptResourcePackages := staticDeps.resPackages().Strings()
	transitiveAaptResourcePackages = slices.DeleteFunc(transitiveAaptResourcePackages, func(p string) bool {
		return p == a.exportPackage.String()
	})
	transitiveAaptResourcePackagesFile := android.PathForModuleOut(ctx, "transitive-res-packages")
	android.WriteFileRule(ctx, transitiveAaptResourcePackagesFile, strings.Join(transitiveAaptResourcePackages, "\n"))
	a.transitiveAaptResourcePackagesFile = transitiveAaptResourcePackagesFile

	a.collectTransitiveHeaderJars(ctx)
	ctx.SetProvider(JavaInfoProvider, JavaInfo{
		HeaderJars:                     android.PathsIfNonNil(a.classpathFile),
		TransitiveLibsHeaderJars:       a.transitiveLibsHeaderJars,
		TransitiveStaticLibsHeaderJars: a.transitiveStaticLibsHeaderJars,
		ImplementationAndResourcesJars: android.PathsIfNonNil(a.classpathFile),
		ImplementationJars:             android.PathsIfNonNil(a.classpathFile),
		// TransitiveAconfigFiles: // TODO(b/289117800): LOCAL_ACONFIG_FILES for prebuilts
	})

	if proptools.Bool(a.properties.Extract_jni) {
		for _, t := range ctx.MultiTargets() {
			arch := t.Arch.Abi[0]
			path := android.PathForModuleOut(ctx, arch+"_jni.zip")
			a.jniPackages = append(a.jniPackages, path)

			outDir := android.PathForModuleOut(ctx, "aarForJni")
			aarPath := android.PathForModuleSrc(ctx, a.properties.Aars[0])
			ctx.Build(pctx, android.BuildParams{
				Rule:        extractJNI,
				Input:       aarPath,
				Outputs:     android.WritablePaths{path},
				Description: "extract JNI from AAR",
				Args: map[string]string{
					"outDir":     outDir.String(),
					"archString": arch,
				},
			})
		}

		ctx.SetProvider(JniPackageProvider, JniPackageInfo{
			JniPackages: a.jniPackages,
		})
	}
}

func (a *AARImport) HeaderJars() android.Paths {
	return android.Paths{a.classpathFile}
}

func (a *AARImport) ImplementationAndResourcesJars() android.Paths {
	return android.Paths{a.classpathFile}
}

func (a *AARImport) DexJarBuildPath() android.Path {
	return nil
}

func (a *AARImport) DexJarInstallPath() android.Path {
	return nil
}

func (a *AARImport) ClassLoaderContexts() dexpreopt.ClassLoaderContextMap {
	return nil
}

var _ android.ApexModule = (*AARImport)(nil)

// Implements android.ApexModule
func (a *AARImport) DepIsInSameApex(ctx android.BaseModuleContext, dep android.Module) bool {
	return a.depIsInSameApex(ctx, dep)
}

// Implements android.ApexModule
func (g *AARImport) ShouldSupportSdkVersion(ctx android.BaseModuleContext,
	sdkVersion android.ApiLevel) error {
	return nil
}

var _ android.PrebuiltInterface = (*AARImport)(nil)

// android_library_import imports an `.aar` file into the build graph as if it was built with android_library.
//
// This module is not suitable for installing on a device, but can be used as a `static_libs` dependency of
// an android_app module.
func AARImportFactory() android.Module {
	module := &AARImport{}

	module.AddProperties(&module.properties)

	android.InitPrebuiltModule(module, &module.properties.Aars)
	android.InitApexModule(module)
	InitJavaModuleMultiTargets(module, android.DeviceSupported)
	return module
}
