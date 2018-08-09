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
	"crypto/sha256"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"android/soong/android"
	"android/soong/dexpreopt"

	"github.com/google/blueprint"
	"github.com/google/blueprint/depset"
	"github.com/google/blueprint/proptools"
	"github.com/google/blueprint/uniquelist"
)

//go:generate go run ../../blueprint/gobtools/codegen

type AndroidLibraryDependency interface {
	ExportPackage() android.Path
	ResourcesNodeDepSet() depset.DepSet[resourcesNode]
	RRODirsDepSet() depset.DepSet[rroDir]
	ManifestsDepSet() depset.DepSet[android.Path]
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
		ctx.Transition("propagate_rro_enforcement", &propagateRROEnforcementTransitionMutator{})
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
	Asset_dirs proptools.Configurable[[]string]

	// list of directories relative to the Blueprints file containing
	// Android resources.  Defaults to ["res"] if a directory called res exists.
	// Set to [] to disable the default.
	Resource_dirs proptools.Configurable[[]string] `android:"path"`

	// list of zip files containing Android resources.
	Resource_zips []string `android:"path"`

	// path to AndroidManifest.xml.  If unset, defaults to "AndroidManifest.xml".
	Manifest proptools.Configurable[string] `android:"path,replace_instead_of_append"`

	// paths to additional manifest files to merge with main manifest.
	Additional_manifests []string `android:"path"`

	// do not include AndroidManifest from dependent libraries
	Dont_merge_manifests *bool

	// If use_resource_processor is set, use Bazel's resource processor instead of aapt2 to generate R.class files.
	// The resource processor produces more optimal R.class files that only list resources in the package of the
	// library that provided them, as opposed to aapt2 which produces R.java files for every package containing
	// every resource.  Using the resource processor can provide significant build time speedups, but requires
	// fixing the module to use the correct package to reference each resource, and to avoid having any other
	// libraries in the tree that use the same package name.  Defaults to true.
	Use_resource_processor *bool

	// true if RRO is enforced for any of the dependent modules
	RROEnforcedForDependent bool `blueprint:"mutated"`

	// Filter only specified product and ignore other products
	Filter_product *string `blueprint:"mutated"`

	// Names of aconfig_declarations modules that specify aconfig flags that the module depends on.
	Flags_packages []string

	// The package name of this app. May not be used if `manifest` or `additional_manifests` are provided.
	Package_name proptools.Configurable[string]
}

type aapt struct {
	aaptSrcJar                         android.Path
	transitiveAaptRJars                android.Paths
	transitiveAaptResourcePackagesFile android.Path
	exportPackage                      android.Path
	manifestPath                       android.Path
	originalManifestPath               android.Path
	proguardOptionsFile                android.Path
	rTxt                               android.Path
	rJar                               android.Path
	kSnapshotFiles                     map[string]android.Path
	extraAaptPackagesFile              android.Path
	mergedManifestFile                 android.Path
	noticeFile                         android.OptionalPath
	assetPackage                       android.OptionalPath
	resIdsFile                         android.OptionalPath
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

	resourcesNodesDepSet depset.DepSet[resourcesNode]
	rroDirsDepSet        depset.DepSet[rroDir]
	manifestsDepSet      depset.DepSet[android.Path]

	manifestValues struct {
		applicationId string
	}

	// Fields written to jdeps
	assetDirs    android.Paths
	resourceDirs android.Paths
}

type split struct {
	name   string
	suffix string
	path   android.Path
}

// Propagate RRO enforcement flag to static lib dependencies transitively.  If EnforceRROGlobally is set then
// all modules will use the "" variant.  If specific modules have RRO enforced, then modules (usually apps) with
// RRO enabled will use the "" variation for themselves, but use the "rro" variant of direct and transitive static
// android_library dependencies.
type propagateRROEnforcementTransitionMutator struct{}

func (p propagateRROEnforcementTransitionMutator) Split(ctx android.BaseModuleContext) []string {
	// Never split modules, apps with or without RRO enabled use the "" variant, static android_library dependencies
	// will use create the "rro" variant from incoming tranisitons.
	return []string{""}
}

func (p propagateRROEnforcementTransitionMutator) SplitOnDemand(ctx android.BaseModuleContext) []string {
	return nil
}

func (p propagateRROEnforcementTransitionMutator) OutgoingTransition(ctx android.OutgoingTransitionContext, sourceVariation string) string {
	// Non-static dependencies are not involved in RRO and always use the empty variant.
	if ctx.DepTag() != staticLibTag {
		return ""
	}

	m := ctx.Module()
	if _, ok := m.(AndroidLibraryDependency); ok {
		// If RRO is enforced globally don't bother using "rro" variants, the empty variant will have RRO enabled.
		if ctx.Config().EnforceRROGlobally() {
			return ""
		}

		// If RRO is enabled for this module use the "rro" variants of static dependencies.  IncomingTransition will
		// rewrite this back to "" if the dependency is not an android_library.
		if ctx.Config().EnforceRROForModule(ctx.Module().Name()) {
			return "rro"
		}
	}

	return sourceVariation
}

func (p propagateRROEnforcementTransitionMutator) IncomingTransition(ctx android.IncomingTransitionContext, incomingVariation string) string {
	// Propagate the "rro" variant to android_library modules, but use the empty variant for everything else.
	if incomingVariation == "rro" {
		m := ctx.Module()
		if _, ok := m.(AndroidLibraryDependency); ok {
			return "rro"
		}
		return ""
	}

	return ""
}

func (p propagateRROEnforcementTransitionMutator) Mutate(ctx android.BottomUpMutatorContext, variation string) {
	m := ctx.Module()
	if d, ok := m.(AndroidLibraryDependency); ok {
		if variation == "rro" {
			// This is the "rro" variant of a module that has both variants, mark this one as RRO enabled and
			// hide it from make to avoid collisions with the non-RRO empty variant.
			d.SetRROEnforcedForDependent(true)
			m.HideFromMake()
		} else if ctx.Config().EnforceRROGlobally() {
			// RRO is enabled globally, mark it enabled for this module, but there is only one variant so no
			// need to hide it from make.
			d.SetRROEnforcedForDependent(true)
		}
	}
}

func (a *aapt) useResourceProcessorBusyBox(ctx android.BaseModuleContext) bool {
	return BoolDefault(a.aaptProperties.Use_resource_processor, true) &&
		// TODO(b/331641946): remove this when ResourceProcessorBusyBox supports generating shared libraries.
		!slices.Contains(a.aaptProperties.Aaptflags, "--shared-lib")
}

func (a *aapt) filterProduct() string {
	return String(a.aaptProperties.Filter_product)
}

func (a *aapt) ExportPackage() android.Path {
	return a.exportPackage
}
func (a *aapt) ResourcesNodeDepSet() depset.DepSet[resourcesNode] {
	return a.resourcesNodesDepSet
}

func (a *aapt) RRODirsDepSet() depset.DepSet[rroDir] {
	return a.rroDirsDepSet
}

func (a *aapt) ManifestsDepSet() depset.DepSet[android.Path] {
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
	manifestPath android.Path, doNotIncludeAssetDirImplicitly bool) (compileFlags, linkFlags []string, linkDeps android.Paths,
	resDirs, overlayDirs []globbedResourceDir, rroDirs []rroDir, resZips android.Paths) {

	hasVersionCode := android.PrefixInList(a.aaptProperties.Aaptflags, "--version-code")
	hasVersionName := android.PrefixInList(a.aaptProperties.Aaptflags, "--version-name")

	// Flags specified in Android.bp
	linkFlags = append(linkFlags, a.aaptProperties.Aaptflags...)

	linkFlags = append(linkFlags, "--enable-compact-entries")
	if ctx.Config().ReleaseUseSparseEncoding() {
		linkFlags = append(linkFlags, "--enable-sparse-encoding")
	}

	if ctx.Config().ReleaseUseUncompressedFonts() {
		linkFlags = append(linkFlags, "--no-compress-fonts")
	}

	// Find implicit or explicit asset and resource dirs
	assets := android.PathsRelativeToModuleSourceDir(android.SourceInput{
		Context:     ctx,
		Paths:       a.aaptProperties.Assets,
		IncludeDirs: false,
	})
	var assetDirs android.Paths
	if doNotIncludeAssetDirImplicitly {
		assetDirs = android.PathsForModuleSrc(ctx, a.aaptProperties.Asset_dirs.GetOrDefault(ctx, nil))
	} else {
		assetDirs = android.PathsWithOptionalDefaultForModuleSrc(ctx, a.aaptProperties.Asset_dirs.GetOrDefault(ctx, nil), "assets")
	}
	a.assetDirs = assetDirs

	resourceDirs := android.PathsWithOptionalDefaultForModuleSrc(ctx, a.aaptProperties.Resource_dirs.GetOrDefault(ctx, nil), "res")
	a.resourceDirs = resourceDirs
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

	var allOverlays android.Paths
	for _, overlayDir := range overlayDirs {
		allOverlays = append(allOverlays, overlayDir.dir)
	}
	for _, rro := range rroDirs {
		allOverlays = append(allOverlays, rro.path)
	}
	android.SetProvider(ctx, android.RROInfoProvider, android.RROInfo{
		Paths: allOverlays,
	})

	assetDirsHasher := sha256.New()
	var assetDeps android.Paths
	for _, dir := range assetDirs {
		// Add a dependency on every file in the asset directory.  This ensures the aapt2
		// rule will be rerun if one of the files in the asset directory is modified.
		dirContents := androidResourceGlob(ctx, dir)
		assetDeps = append(assetDeps, dirContents...)

		// Add a hash of all the files in the asset directory to the command line.
		// This ensures the aapt2 rule will be run if a file is removed from the asset directory,
		// or a file is added whose timestamp is older than the output of aapt2.
		for _, path := range dirContents.Strings() {
			assetDirsHasher.Write([]byte(path))
		}
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

		rule := android.NewRuleBuilder(pctx, ctx).SandboxDisabled()
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
	linkFlags = append(linkFlags, fmt.Sprintf("$$(: %x)", assetDirsHasher.Sum(nil)))
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
		if ctx.ModuleName() == "framework-res" || ctx.ModuleName() == "org.lineageos.platform-res" {
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
	// Split the flags by prefix, as --png-compression-level has the "=value" suffix.
	linkFlags, compileFlags = android.FilterListByPrefix(linkFlags,
		[]string{"--legacy", "--png-compression-level"})

	// Always set --pseudo-localize, it will be stripped out later for release
	// builds that don't want it.
	compileFlags = append(compileFlags, "--pseudo-localize")

	return compileFlags, linkFlags, linkDeps, resDirs, overlayDirs, rroDirs, resourceZips
}

func (a *aapt) deps(ctx android.BottomUpMutatorContext, sdkDep sdkDep) {
	if sdkDep.frameworkResModule != "" {
		ctx.AddVariationDependencies(nil, frameworkResTag, sdkDep.frameworkResModule)
	}
	if sdkDep.lineageResModule != "" {
		ctx.AddVariationDependencies(nil, lineageResTag, sdkDep.lineageResModule)
	}
}

var extractAssetsRule = pctx.AndroidStaticRule("extractAssets",
	blueprint.RuleParams{
		Command2: blueprint.NewCommand(
			android.Zip2zip, ` -i ${in} -o ${out} "assets/**/*"`,
		),
	})

type aaptBuildActionOptions struct {
	sdkContext                     android.SdkContext
	classLoaderContexts            dexpreopt.ClassLoaderContextMap
	excludedLibs                   []string
	enforceDefaultTargetSdkVersion bool
	forceNonFinalResourceIDs       bool
	emitResIds                     bool
	extraLinkFlags                 []string
	aconfigTextFiles               android.Paths
	usesLibrary                    *usesLibrary
	// If rroDirs is provided, it will be used to generate package-res.apk
	rroDirs *android.Paths
	// If manifestForAapt is not nil, it will be used for aapt instead of the default source manifest.
	manifestForAapt android.Path
}

func filterRRO(rroDirsDepSet depset.DepSet[rroDir], filter overlayType) android.Paths {
	var paths android.Paths
	seen := make(map[android.Path]bool)
	for _, d := range rroDirsDepSet.ToList() {
		if d.overlayType == filter {
			if seen[d.path] {
				continue
			}
			seen[d.path] = true
			paths = append(paths, d.path)
		}
	}
	return paths
}

func (a *aapt) buildActions(ctx android.ModuleContext, opts aaptBuildActionOptions) {

	a.kSnapshotFiles = make(map[string]android.Path)

	staticResourcesNodesDepSet, sharedResourcesNodesDepSet, staticRRODirsDepSet, staticManifestsDepSet, sharedExportPackages, libFlags :=
		aaptLibs(ctx, opts.sdkContext, opts.classLoaderContexts, opts.usesLibrary)

	// Exclude any libraries from the supplied list.
	opts.classLoaderContexts = opts.classLoaderContexts.ExcludeLibs(opts.excludedLibs)

	// App manifest file
	packageNameProp := a.aaptProperties.Package_name.Get(ctx)
	var manifestFilePath android.Path
	if opts.manifestForAapt != nil {
		manifestFilePath = opts.manifestForAapt
	} else if a.isLibrary && packageNameProp.IsPresent() && a.aaptProperties.Manifest.GetOrDefault(ctx, "") == "" && a.aaptProperties.Additional_manifests == nil {
		// If the only reason that a library needs a manifest file is to give the package name, allow them to do that in
		// the module declaration.  If they are already supplying a manifest, then do not autogenerate a manifest file.
		generatedManifestPath := android.PathForModuleOut(ctx, "GeneratedManifest.xml")
		manifestString := `<?xml version="1.0" encoding="utf-8"?>
<!-- Automatically generated by Soong. -->
<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="` + packageNameProp.Get() + `" />
`
		android.WriteFileRule(ctx, generatedManifestPath, manifestString)
		ctx.SetOutputFiles([]android.Path{generatedManifestPath}, ".gen_xml")
		manifestFilePath = generatedManifestPath
	} else {
		manifestFile := a.aaptProperties.Manifest.GetOrDefault(ctx, "AndroidManifest.xml")
		manifestFilePath = android.PathForModuleSrc(ctx, manifestFile)
	}
	a.originalManifestPath = manifestFilePath

	manifestPath := ManifestFixer(ctx, manifestFilePath, ManifestFixerParams{
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
	transitiveManifestPaths = append(transitiveManifestPaths, staticManifestsDepSet.ToList()...)

	if len(transitiveManifestPaths) > 1 && !Bool(a.aaptProperties.Dont_merge_manifests) {
		manifestMergerParams := ManifestMergerParams{
			staticLibManifests: transitiveManifestPaths[1:],
			isLibrary:          a.isLibrary,
			packageName:        a.manifestValues.applicationId,
		}
		a.mergedManifestFile = manifestMerger(ctx, transitiveManifestPaths[0], manifestMergerParams)
		ctx.CheckbuildFile(a.mergedManifestFile)
		if !a.isLibrary {
			// Only use the merged manifest for applications.  For libraries, the transitive closure of manifests
			// will be propagated to the final application and merged there.  The merged manifest for libraries is
			// only passed to Make, which can't handle transitive dependencies.
			manifestPath = a.mergedManifestFile
		}
	} else {
		a.mergedManifestFile = manifestPath
	}

	// do not include assets in autogenerated RRO.
	compileFlags, linkFlags, linkDeps, resDirs, overlayDirs, rroDirs, resZips := a.aapt2Flags(ctx, opts.sdkContext, manifestPath, opts.rroDirs != nil)

	a.rroDirsDepSet = depset.NewBuilder[rroDir](depset.TOPOLOGICAL).
		Direct(rroDirs...).
		Transitive(staticRRODirsDepSet).Build()

	linkFlags = append(linkFlags, libFlags...)
	linkDeps = append(linkDeps, sharedExportPackages...)
	linkDeps = append(linkDeps, staticDeps.resPackages()...)
	linkFlags = append(linkFlags, opts.extraLinkFlags...)
	if a.isLibrary {
		linkFlags = append(linkFlags, "--static-lib")
	}
	if opts.forceNonFinalResourceIDs {
		linkFlags = append(linkFlags, "--non-final-ids")
	}

	linkFlags = append(linkFlags, "--no-static-lib-packages")
	if a.isLibrary {
		// Pass --merge-only to skip resource references validation until the final
		// app link step when when all static libraries are present.
		linkFlags = append(linkFlags, "--merge-only")
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
		compiledResDirs = append(compiledResDirs, aapt2Compile(ctx, dir.dir, dir.files,
			compileFlags, a.filterProduct(), opts.aconfigTextFiles).Paths())
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

	if a.isLibrary && a.useResourceProcessorBusyBox(ctx) {
		// When building an android_library with ResourceProcessorBusyBox enabled treat static library dependencies
		// as imports.  The resources from dependencies will not be merged into this module's package-res.apk, and
		// instead modules depending on this module will reference package-res.apk from all transitive static
		// dependencies.
		for _, sharedDep := range sharedDeps {
			if sharedDep.usedResourceProcessor {
				transitiveRJars = append(transitiveRJars, sharedDep.rJar)
			}
		}
		for _, staticDep := range staticDeps {
			linkDeps = append(linkDeps, staticDep.resPackage)
			linkFlags = append(linkFlags, "-I "+staticDep.resPackage.String())
			if staticDep.usedResourceProcessor {
				transitiveRJars = append(transitiveRJars, staticDep.rJar)
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

	var compiledRro, compiledRroOverlay android.Paths
	if opts.rroDirs != nil {
		compiledRro, compiledRroOverlay = a.compileResInDir(ctx, *opts.rroDirs, compileFlags, opts.aconfigTextFiles)
	} else {
		// RRO enforcement is done based on module name. Compile the overlayDirs only if rroDirs is nil.
		// This ensures that the autogenerated RROs do not compile the overlay dirs twice.
		for _, dir := range overlayDirs {
			compiledOverlay = append(compiledOverlay, aapt2Compile(ctx, dir.dir, dir.files,
				compileFlags, a.filterProduct(), opts.aconfigTextFiles).Paths()...)
		}
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

	if !a.useResourceProcessorBusyBox(ctx) {
		// the subdir "android" is required to be filtered by package names
		srcJar = android.PathForModuleGen(ctx, "android", "R.srcjar")
	}

	var resIds android.WritablePath
	if opts.emitResIds {
		resIds = android.PathForModuleOut(ctx, "res-ids.txt")
		a.resIdsFile = android.OptionalPathForPath(resIds)
	}

	// No need to specify assets from dependencies to aapt2Link for libraries, all transitive assets will be
	// provided to the final app aapt2Link step.
	var transitiveAssets android.Paths
	if !a.isLibrary {
		transitiveAssets = android.ReverseSliceInPlace(staticDeps.assets())
	}
	if opts.rroDirs == nil { // link resources and overlay
		aapt2Link(ctx, packageRes, srcJar, proguardOptionsFile, rTxt, resIds,
			linkFlags, linkDeps, compiledRes, compiledOverlay, transitiveAssets, splitPackages,
			opts.aconfigTextFiles)
		ctx.CheckbuildFile(packageRes)
	} else { // link autogenerated rro
		if len(compiledRro) == 0 {
			return
		}
		aapt2Link(ctx, packageRes, srcJar, proguardOptionsFile, rTxt, resIds,
			linkFlags, linkDeps, compiledRro, compiledRroOverlay, nil, nil,
			opts.aconfigTextFiles)
		ctx.CheckbuildFile(packageRes)
	}

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

	if a.useResourceProcessorBusyBox(ctx) {
		rJar := android.PathForModuleOut(ctx, "busybox/R.jar")
		resourceProcessorBusyBoxGenerateBinaryR(ctx, rTxt, a.mergedManifestFile, rJar, staticDeps, a.isLibrary, a.aaptProperties.Aaptflags,
			opts.forceNonFinalResourceIDs)
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

	// Reverse the list of R.jar files so that the current module comes first, and direct dependencies come before
	// transitive dependencies.
	transitiveRJars = android.ReversePaths(transitiveRJars)

	a.aaptSrcJar = srcJar
	a.transitiveAaptRJars = transitiveRJars
	a.transitiveAaptResourcePackagesFile = transitiveAaptResourcePackagesFile
	a.exportPackage = packageRes
	a.manifestPath = manifestPath
	a.proguardOptionsFile = proguardOptionsFile
	a.extraAaptPackagesFile = extraPackages
	a.rTxt = rTxt
	a.splits = splits
	a.resourcesNodesDepSet = depset.NewBuilder[resourcesNode](depset.TOPOLOGICAL).
		Direct(resourcesNode{
			resPackage:          a.exportPackage,
			manifest:            a.manifestPath,
			additionalManifests: uniquelist.Make(additionalManifests),
			rTxt:                a.rTxt,
			rJar:                a.rJar,
			assets:              a.assetPackage,

			usedResourceProcessor: a.useResourceProcessorBusyBox(ctx),
		}).
		Transitive(staticResourcesNodesDepSet).Build()
	a.manifestsDepSet = depset.NewBuilder[android.Path](depset.TOPOLOGICAL).
		Direct(a.manifestPath).
		DirectSlice(additionalManifests).
		Transitive(staticManifestsDepSet).Build()
}

func (a *aapt) addKSnapshot(ctx android.ModuleContext, jarFile android.Path) {
	if jarFile == nil {
		return
	}
	if _, exists := a.kSnapshotFiles[jarFile.String()]; !exists {
		snapshot := SnapshotJarForKotlin(ctx, jarFile.(android.WritablePath))
		a.kSnapshotFiles[jarFile.String()] = snapshot
	}
}

// comileResInDir finds the resource files in dirs by globbing and then compiles them using aapt2
// returns the file paths of compiled resources
// dirs[0] is used as compileRes
// dirs[1:] is used as compileOverlay
func (a *aapt) compileResInDir(ctx android.ModuleContext, dirs android.Paths, compileFlags []string, aconfig android.Paths) (android.Paths, android.Paths) {
	filesInDir := func(dir android.Path) android.Paths {
		files, err := ctx.GlobWithDeps(filepath.Join(dir.String(), "**/*"), androidResourceIgnoreFilenames)
		if err != nil {
			ctx.ModuleErrorf("failed to glob overlay resource dir %q: %s", dir, err.Error())
			return nil
		}
		var filePaths android.Paths
		for _, file := range files {
			if strings.HasSuffix(file, "/") {
				continue // ignore directories
			}
			filePaths = append(filePaths, android.PathForSource(ctx, file))
		}
		return filePaths
	}

	var compiledRes, compiledOverlay android.Paths
	if len(dirs) == 0 {
		return nil, nil
	}
	compiledRes = append(compiledRes, aapt2Compile(ctx, dirs[0], filesInDir(dirs[0]), compileFlags, a.filterProduct(), aconfig).Paths()...)
	if len(dirs) > 0 {
		for _, dir := range dirs[1:] {
			compiledOverlay = append(compiledOverlay, aapt2Compile(ctx, dir, filesInDir(dir), compileFlags, a.filterProduct(), aconfig).Paths()...)
		}
	}
	return compiledRes, compiledOverlay
}

var resourceProcessorBusyBox = pctx.AndroidStaticRule("resourceProcessorBusyBox",
	blueprint.RuleParams{
		Command: "${config.JavaCmd} ${config.ResourceProcessorBusyBoxSuppressJDKWarnings} " +
			"-cp ${config.ResourceProcessorBusyBox} " +
			"com.google.devtools.build.android.ResourceProcessorBusyBox --tool=GENERATE_BINARY_R -- @${out}.args && " +
			"if cmp -s ${out}.tmp ${out} ; then rm ${out}.tmp ; else mv ${out}.tmp ${out}; fi",
		CommandDeps:     []string{"${config.ResourceProcessorBusyBox}"},
		Rspfile:         "${out}.args",
		RspfileContent:  "--primaryRTxt ${rTxt} --primaryManifest ${manifest} --classJarOutput ${out}.tmp ${args}",
		Restat:          true,
		SandboxDisabled: true,
	}, "rTxt", "manifest", "args")

// resourceProcessorBusyBoxGenerateBinaryR converts the R.txt file produced by aapt2 into R.class files
// using Bazel's ResourceProcessorBusyBox tool, which is faster than compiling the R.java files and
// supports producing classes for static dependencies that only include resources from that dependency.
func resourceProcessorBusyBoxGenerateBinaryR(ctx android.ModuleContext, rTxt, manifest android.Path,
	rJar android.WritablePath, transitiveDeps transitiveAarDeps, isLibrary bool, aaptFlags []string,
	forceNonFinalIds bool) {

	var args []string
	var deps android.Paths

	if !isLibrary {
		// When compiling an app, pass all R.txt and AndroidManifest.xml from transitive static library dependencies
		// to ResourceProcessorBusyBox so that it can regenerate R.class files with the final resource IDs for each
		// package.
		args, deps = transitiveDeps.resourceProcessorDeps()
		if forceNonFinalIds {
			args = append(args, "--finalFields=false")
		}
	} else {
		// When compiling a library don't pass any dependencies as it only needs to generate an R.class file for this
		// library.  Pass --finalFields=false so that the R.class file contains non-final fields so they don't get
		// inlined into the library before the final IDs are assigned during app compilation.
		args = append(args, "--finalFields=false")
	}

	for i, arg := range aaptFlags {
		const AAPT_CUSTOM_PACKAGE = "--custom-package"
		if strings.HasPrefix(arg, AAPT_CUSTOM_PACKAGE) {
			pkg := strings.TrimSpace(strings.TrimPrefix(arg, AAPT_CUSTOM_PACKAGE))
			if pkg == "" && i+1 < len(aaptFlags) {
				pkg = aaptFlags[i+1]
			}
			args = append(args, "--packageForR "+pkg)
		}
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

// @auto-generate: gob
type resourcesNode struct {
	resPackage          android.Path
	manifest            android.Path
	additionalManifests uniquelist.UniqueList[android.Path]
	rTxt                android.Path
	rJar                android.Path
	assets              android.OptionalPath

	usedResourceProcessor bool
}

type transitiveAarDeps []resourcesNode

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
		paths = dep.additionalManifests.AppendTo(paths)
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
func aaptLibs(ctx android.ModuleContext, sdkContext android.SdkContext,
	classLoaderContexts dexpreopt.ClassLoaderContextMap, usesLibrary *usesLibrary) (
	staticResourcesNodes, sharedResourcesNodes depset.DepSet[resourcesNode], staticRRODirs depset.DepSet[rroDir],
	staticManifests depset.DepSet[android.Path], sharedLibs android.Paths, flags []string) {

	if classLoaderContexts == nil {
		// Not all callers need to compute class loader context, those who don't just pass nil.
		// Create a temporary class loader context here (it will be computed, but not used).
		classLoaderContexts = make(dexpreopt.ClassLoaderContextMap)
	}

	sdkDep := decodeSdkDep(ctx, sdkContext)
	if sdkDep.useFiles {
		sharedLibs = append(sharedLibs, sdkDep.jars...)
	}

	var staticResourcesNodeDepSets []depset.DepSet[resourcesNode]
	var sharedResourcesNodeDepSets []depset.DepSet[resourcesNode]
	rroDirsDepSetBuilder := depset.NewBuilder[rroDir](depset.TOPOLOGICAL)
	manifestsDepSetBuilder := depset.NewBuilder[android.Path](depset.TOPOLOGICAL)

	ctx.VisitDirectDepsProxy(func(module android.ModuleProxy) {
		depTag := ctx.OtherModuleDependencyTag(module)

		var exportPackage android.Path
		var aarDep *AndroidLibraryDependencyInfo
		javaInfo, ok := android.OtherModuleProvider(ctx, module, JavaInfoProvider)
		if ok && javaInfo.AndroidLibraryDependencyInfo != nil {
			aarDep = javaInfo.AndroidLibraryDependencyInfo
			exportPackage = aarDep.ExportPackage
		}

		switch depTag {
		case instrumentationForTag:
			// Nothing, instrumentationForTag is treated as libTag for javac but not for aapt2.
		case sdkLibTag, libTag, rroDepTag:
			if exportPackage != nil {
				sharedResourcesNodeDepSets = append(sharedResourcesNodeDepSets, aarDep.ResourcesNodeDepSet)
				sharedLibs = append(sharedLibs, exportPackage)
			}
		case frameworkResTag, lineageResTag:
			if exportPackage != nil {
				sharedLibs = append(sharedLibs, exportPackage)
			}
		case staticLibTag:
			if exportPackage != nil {
				staticResourcesNodeDepSets = append(staticResourcesNodeDepSets, aarDep.ResourcesNodeDepSet)
				rroDirsDepSetBuilder.Transitive(aarDep.RRODirsDepSet)
				manifestsDepSetBuilder.Transitive(aarDep.ManifestsDepSet)
			}
		}

		addCLCFromDep(ctx, module, classLoaderContexts)
		if usesLibrary != nil {
			addMissingOptionalUsesLibsFromDep(ctx, module, usesLibrary)
		}
	})

	// AAPT2 overlays are in lowest to highest priority order, the topological order will be reversed later.
	// Reverse the dependency order now going into the depset so that it comes out in order after the second
	// reverse later.
	// NOTE: this is legacy and probably incorrect behavior, for most other cases (e.g. conflicting classes in
	// dependencies) the highest priority dependency is listed first, but for resources the highest priority
	// dependency has to be listed last.  This is also inconsistent with the way manifests from the same
	// transitive dependencies are merged.
	staticResourcesNodes = depset.New(depset.TOPOLOGICAL, nil,
		android.ReverseSliceInPlace(staticResourcesNodeDepSets))
	sharedResourcesNodes = depset.New(depset.TOPOLOGICAL, nil,
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

// @auto-generate: gob
type AndroidLibraryInfo struct {
	// Empty for now
}

var AndroidLibraryInfoProvider = blueprint.NewProvider[AndroidLibraryInfo]()

// @auto-generate: gob
type AARImportInfo struct {
	// Empty for now
}

var AARImportInfoProvider = blueprint.NewProvider[AARImportInfo]()

type AndroidLibrary struct {
	Library
	aapt

	androidLibraryProperties androidLibraryProperties

	aarFile android.WritablePath
}

var _ AndroidLibraryDependency = (*AndroidLibrary)(nil)
var _ KSnapshotContainer = (*AndroidLibrary)(nil)

func (a *AndroidLibrary) JarToSnapshotMap() map[string]android.Path {
	return a.kSnapshotFiles
}

func (a *AndroidLibrary) DepsMutator(ctx android.BottomUpMutatorContext) {
	a.usesLibrary.deps(ctx, false)
	a.Module.deps(ctx)
	sdkDep := decodeSdkDep(ctx, android.SdkContext(a))
	if sdkDep.hasFrameworkLibs() {
		a.aapt.deps(ctx, sdkDep)
	}

	for _, aconfig_declaration := range a.aaptProperties.Flags_packages {
		ctx.AddDependency(ctx.Module(), aconfigDeclarationTag, aconfig_declaration)
	}
}

func (a *AndroidLibrary) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if checkSdkViolation, ok := ctx.Config().GetBuildFlag("RELEASE_AAR_SDK_VIOLATION_CHECK"); ok && checkSdkViolation == "true" {
		a.checkSdkVersions(ctx)
	}
	packageNameProp := a.aaptProperties.Package_name.Get(ctx)
	if packageNameProp.IsPresent() {
		if a.aaptProperties.Manifest.GetOrDefault(ctx, "") != "" {
			ctx.PropertyErrorf("package_name", "cannot be used with `manifest`")
			return
		}
		if a.aaptProperties.Additional_manifests != nil {
			ctx.PropertyErrorf("package_name", "cannot be used with `additional_manifests`")
			return
		}
	}
	a.aapt.isLibrary = true
	a.classLoaderContexts = a.usesLibrary.classLoaderContextForUsesLibDeps(ctx)
	if a.usesLibrary.shouldDisableDexpreopt {
		a.dexpreopter.disableDexpreopt()
	}
	aconfigTextFilePaths := getAconfigFilePaths(ctx)
	a.aapt.buildActions(ctx,
		aaptBuildActionOptions{
			sdkContext:                     android.SdkContext(a),
			classLoaderContexts:            a.classLoaderContexts,
			enforceDefaultTargetSdkVersion: false,
			aconfigTextFiles:               aconfigTextFilePaths,
			usesLibrary:                    &a.usesLibrary,
		},
	)

	maps.Copy(a.kSnapshotFiles, a.aapt.kSnapshotFiles)

	apexInfo, _ := android.ModuleProvider(ctx, android.ApexInfoProvider)
	if !apexInfo.IsForPlatform() {
		a.HideFromMake()
	}

	a.stem = proptools.StringDefault(a.overridableProperties.Stem, ctx.ModuleName())

	ctx.CheckbuildFile(a.aapt.proguardOptionsFile)
	ctx.CheckbuildFile(a.aapt.exportPackage)
	if a.useResourceProcessorBusyBox(ctx) {
		ctx.CheckbuildFile(a.aapt.rJar)
	} else {
		ctx.CheckbuildFile(a.aapt.aaptSrcJar)
	}

	// apps manifests are handled by aapt, don't let Module see them
	a.properties.Manifest = proptools.Configurable[string]{}

	a.linter.mergedManifest = a.aapt.mergedManifestFile
	a.linter.manifest = a.aapt.manifestPath
	a.linter.resources = a.aapt.resourceFiles

	proguardSpecInfo := a.collectProguardSpecInfo(ctx)
	android.SetProvider(ctx, ProguardSpecInfoProvider, proguardSpecInfo)
	exportedProguardFlagsFiles := proguardSpecInfo.ProguardFlagsFiles.ToList()
	a.extraProguardFlagsFiles = append(a.extraProguardFlagsFiles, exportedProguardFlagsFiles...)
	a.extraProguardFlagsFiles = append(a.extraProguardFlagsFiles, a.proguardOptionsFile)
	a.extraIncludedProguardFlagsFiles = append(a.extraIncludedProguardFlagsFiles, proguardSpecInfo.IncludedProguardFlagsFiles.ToList()...)

	combinedExportedProguardFlagFile := android.PathForModuleOut(ctx, "export_proguard_flags")
	writeCombinedProguardFlagsFile(ctx, combinedExportedProguardFlagFile, exportedProguardFlagsFiles)
	a.combinedExportedProguardFlagsFile = combinedExportedProguardFlagFile

	if a.useResourceProcessorBusyBox(ctx) {
		// When building a library with ResourceProcessorBusyBox enabled ResourceProcessorBusyBox for this
		// library and each of the transitive static android_library dependencies has already created an
		// R.class file for the appropriate package.  Add all of those R.class files to the classpath.
		a.Module.extraClasspathJars = append(a.Module.extraClasspathJars, a.transitiveAaptRJars...)
	} else {
		// When building a library without ResourceProcessorBusyBox the aapt2 rule creates R.srcjar containing
		// R.java files for the library's package and the packages from all transitive static android_library
		// dependencies.  Compile the srcjar alongside the rest of the sources.
		a.Module.extraSrcJars = append(a.Module.extraSrcJars, a.aapt.aaptSrcJar)
	}

	javaInfo := a.Module.compile(ctx)

	a.aarFile = android.PathForModuleOut(ctx, ctx.ModuleName()+".aar")
	var res android.Paths
	if a.androidLibraryProperties.BuildAAR {
		BuildAAR(ctx, a.aarFile, a.outputFile, a.manifestPath, a.rTxt, res)
		android.SetProvider(ctx, AARProvider, AARInfo{Aar: a.aarFile})
	}

	prebuiltJniPackages := android.Paths{}
	ctx.VisitDirectDepsProxy(func(module android.ModuleProxy) {
		if info, ok := android.OtherModuleProvider(ctx, module, JniPackageProvider); ok {
			prebuiltJniPackages = append(prebuiltJniPackages, info.JniPackages...)
		}
	})
	if len(prebuiltJniPackages) > 0 {
		android.SetProvider(ctx, JniPackageProvider, JniPackageInfo{
			JniPackages: prebuiltJniPackages,
		})
	}

	android.SetProvider(ctx, FlagsPackagesProvider, FlagsPackages{
		AconfigTextFiles: aconfigTextFilePaths,
	})

	android.SetProvider(ctx, AndroidLibraryInfoProvider, AndroidLibraryInfo{})

	if javaInfo != nil {
		setExtraJavaInfo(ctx, a, javaInfo)
		android.SetProvider(ctx, JavaInfoProvider, javaInfo)
	} else {
		fmt.Printf("%s\n", ctx.ModuleName())
		for k, v := range a.kSnapshotFiles {
			fmt.Printf("%s: %s\n", k, v)
		}
	}

	a.setOutputFiles(ctx)

	buildComplianceMetadata(ctx)

	if javaInfo != nil {
		moduleInfoJSON := ctx.ModuleInfoJSON()
		moduleInfoJSON.Class = []string{"JAVA_LIBRARIES"}
		moduleInfoJSON.ClassesJar = []string{a.Library.implementationAndResourcesJar.String()}
		moduleInfoJSON.SystemSharedLibs = []string{"none"}
	}
}

func (a *AndroidLibrary) setOutputFiles(ctx android.ModuleContext) {
	ctx.SetOutputFiles([]android.Path{a.aarFile}, ".aar")
	setOutputFiles(ctx, &a.Library.Module)
}

func (a *AndroidLibrary) IDEInfo(ctx android.BaseModuleContext, dpInfo *android.IdeInfo) {
	a.Library.IDEInfo(ctx, dpInfo)
	a.aapt.IDEInfo(ctx, dpInfo)
}

func (a *aapt) IDEInfo(ctx android.BaseModuleContext, dpInfo *android.IdeInfo) {
	if a.rJar != nil {
		dpInfo.Jars = append(dpInfo.Jars, a.rJar.String())
	}
	dpInfo.Asset_dirs = append(dpInfo.Asset_dirs, a.assetDirs.Strings()...)
	dpInfo.Resource_dirs = append(dpInfo.Resource_dirs, a.resourceDirs.Strings()...)
	if a.originalManifestPath != nil {
		dpInfo.Manifest = a.originalManifestPath.String()
	}
	if packageNameProp := a.aaptProperties.Package_name.Get(ctx); packageNameProp.IsPresent() {
		dpInfo.PackageName = packageNameProp.Get()
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
		&module.androidLibraryProperties,
		&module.sourceProperties)

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
	Min_sdk_version proptools.Configurable[string] `android:"replace_instead_of_append"`
	// List of java static libraries that the included ARR (android library prebuilts) has dependencies to.
	Static_libs proptools.Configurable[[]string]
	// List of java libraries that the included ARR (android library prebuilts) has dependencies to.
	Libs proptools.Configurable[[]string]
	// If set to true, run Jetifier against .aar file. Defaults to false.
	Jetifier *bool
	// If true, extract JNI libs from AAR archive. These libs will be accessible to android_app modules and
	// will be passed transitively through android_libraries to an android_app.
	//TODO(b/241138093) evaluate whether we can have this flag default to true for Bazel conversion
	Extract_jni *bool

	// If set, overrides the manifest extracted from the AAR with the provided path.
	Manifest proptools.Configurable[string] `android:"path,replace_instead_of_append"`
}

type AARImport struct {
	android.ModuleBase
	android.DefaultableModuleBase
	android.ApexModuleBase
	prebuilt android.Prebuilt

	// Functionality common to Module and Import.
	embeddableInModuleAndImport

	providesTransitiveHeaderJarsForR8

	properties AARImportProperties

	headerJarFile                      android.Path
	implementationJarFile              android.Path
	implementationAndResourcesJarFile  android.Path
	proguardFlags                      android.Path
	exportPackage                      android.Path
	transitiveAaptResourcePackagesFile android.Path
	extraAaptPackagesFile              android.Path
	manifest                           android.Path
	assetsPackage                      android.Path
	rTxt                               android.Path
	rJar                               android.Path
	kSnapshotFiles                     map[string]android.Path

	resourcesNodesDepSet depset.DepSet[resourcesNode]
	manifestsDepSet      depset.DepSet[android.Path]

	aarPath     android.Path
	jniPackages android.Paths

	sdkVersion    android.SdkSpec
	minSdkVersion android.ApiLevel

	usesLibrary
	classLoaderContexts dexpreopt.ClassLoaderContextMap
}

func (a *AARImport) SdkVersion(ctx android.ConfigContext) android.SdkSpec {
	return android.SdkSpecFrom(ctx, String(a.properties.Sdk_version))
}

func (a *AARImport) SystemModules() string {
	return ""
}

func (a *AARImport) MinSdkVersion(ctx android.MinSdkVersionFromValueContext) android.ApiLevel {
	minSdkVersion := a.properties.Min_sdk_version.Get(a.ConfigurableEvaluator(ctx))
	if minSdkVersion.IsPresent() {
		return android.ApiLevelFrom(ctx, minSdkVersion.Get())
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
func (a *AARImport) ResourcesNodeDepSet() depset.DepSet[resourcesNode] {
	return a.resourcesNodesDepSet
}

func (a *AARImport) RRODirsDepSet() depset.DepSet[rroDir] {
	return depset.New[rroDir](depset.TOPOLOGICAL, nil, nil)
}

func (a *AARImport) ManifestsDepSet() depset.DepSet[android.Path] {
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

func (a *AARImport) DepsMutator(ctx android.BottomUpMutatorContext) {
	if !ctx.Config().AlwaysUsePrebuiltSdks() {
		sdkDep := decodeSdkDep(ctx, android.SdkContext(a))
		if sdkDep.useModule && sdkDep.frameworkResModule != "" {
			ctx.AddVariationDependencies(nil, frameworkResTag, sdkDep.frameworkResModule)
		}
		if sdkDep.useModule && sdkDep.lineageResModule != "" {
			ctx.AddVariationDependencies(nil, lineageResTag, sdkDep.lineageResModule)
		}
	}

	ctx.AddVariationDependencies(nil, libTag, a.properties.Libs.GetOrDefault(ctx, nil)...)
	ctx.AddVariationDependencies(nil, staticLibTag, a.properties.Static_libs.GetOrDefault(ctx, nil)...)

	a.usesLibrary.deps(ctx, false)
	a.EmbeddableSdkLibraryComponent.setComponentDependencyInfoProvider(ctx)
}

// @auto-generate: gob
type JniPackageInfo struct {
	// List of zip files containing JNI libraries
	// Zip files should have directory structure jni/<arch>/*.so
	JniPackages android.Paths
}

var JniPackageProvider = blueprint.NewProvider[JniPackageInfo]()

// Unzip an AAR and extract the JNI libs for $archString.
var extractJNI = pctx.AndroidStaticRule("extractJNI",
	blueprint.RuleParams{
		Command2: blueprint.NewCommand(
			android.Rm, ` -rf $out && `, android.Touch, ` $out && `,
			android.ZipSync, ` -d $outDir $in && `,
			`jni_files=$$(`, android.Find, ` $outDir/jni/${archString} -type f 2>/dev/null) && `,
			// print error message if there are no JNI libs for this arch
			`[ -n "$$jni_files" ] || (`, android.Echo, ` "ERROR: no JNI libs found for arch ${archString}" && exit 1) && `,
			android.SoongZip, ` -o $out -L 0 -P 'lib/${archString}' `,
			`-C $outDir/jni/${archString} $$(`, android.Echo, ` $$jni_files | `, android.Xargs, ` -n1 `, android.Printf, ` " -f %s")`,
		),
	},
	"outDir", "archString")

// Unzip an AAR into its constituent files and directories.  Any files in Outputs that don't exist in the AAR will be
// touched to create an empty file. The res directory is not extracted, as it will be extracted in its own rule.
var unzipAAR = pctx.AndroidStaticRule("unzipAAR",
	blueprint.RuleParams{
		Command2: blueprint.NewCommand(
			android.ZipSync, ` -d $outDir $in && `,
			android.Rm, ` -rf $outDir/res && `,
			android.Touch, ` $out && `,
			android.Zip2zip, ` -i $in -o $assetsPackage 'assets/**/*' && `,
			android.MergeZips, ` $combinedClassesJar $$(`,
			android.Ls, ` $outDir/classes.jar 2> /dev/null) $$(`,
			android.Ls, ` $outDir/libs/*.jar 2> /dev/null)`,
		),
	},
	"outDir", "combinedClassesJar", "assetsPackage")

func (a *AARImport) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if len(a.properties.Aars) != 1 {
		ctx.PropertyErrorf("aars", "exactly one aar is required")
		return
	}

	a.kSnapshotFiles = make(map[string]android.Path)
	a.sdkVersion = a.SdkVersion(ctx)
	a.minSdkVersion = a.MinSdkVersion(ctx)

	apexInfo, _ := android.ModuleProvider(ctx, android.ApexInfoProvider)
	if !apexInfo.IsForPlatform() {
		a.HideFromMake()
	}

	aarName := ctx.ModuleName() + ".aar"
	a.aarPath = android.PathForModuleSrc(ctx, a.properties.Aars[0])

	if Bool(a.properties.Jetifier) {
		inputFile := a.aarPath
		jetifierPath := android.PathForModuleOut(ctx, "jetifier", aarName)
		TransformJetifier(ctx, jetifierPath, inputFile)
		a.aarPath = jetifierPath
	}

	jarName := ctx.ModuleName() + ".jar"
	extractedAARDir := android.PathForModuleOut(ctx, "aar")
	classpathFile := extractedAARDir.Join(ctx, jarName)

	extractedManifest := extractedAARDir.Join(ctx, "AndroidManifest.xml")
	if manifestFromProp := a.properties.Manifest.GetOrDefault(ctx, ""); manifestFromProp != "" {
		a.manifest = android.OptionalPathForModuleSrc(ctx, &manifestFromProp).Path()
	} else {
		a.manifest = extractedManifest
	}

	rTxt := extractedAARDir.Join(ctx, "R.txt")
	assetsPackage := android.PathForModuleOut(ctx, "assets.zip")
	proguardFlags := extractedAARDir.Join(ctx, "proguard.txt")
	proguardSpecInfo := collectDepProguardSpecInfo(ctx)
	android.SetProvider(ctx, ProguardSpecInfoProvider, ProguardSpecInfo{
		ProguardFlagsFiles: depset.New[android.Path](
			depset.POSTORDER,
			android.Paths{proguardFlags},
			proguardSpecInfo.transitiveProguardFlags,
		),
		IncludedProguardFlagsFiles: depset.New[android.Path](
			depset.POSTORDER,
			nil,
			proguardSpecInfo.transitiveIncludedProguardFlags,
		),
		UnconditionallyExportedProguardFlags: depset.New[android.Path](
			depset.POSTORDER,
			nil,
			proguardSpecInfo.transitiveUnconditionalExportedFlags,
		),
		IncludedUnconditionallyExportedProguardFlags: depset.New[android.Path](
			depset.POSTORDER,
			nil,
			proguardSpecInfo.transitiveIncludedUnconditionalExportedFlags,
		),
	})

	ctx.Build(pctx, android.BuildParams{
		Rule:        unzipAAR,
		Input:       a.aarPath,
		Outputs:     android.WritablePaths{classpathFile, proguardFlags, extractedManifest, assetsPackage, rTxt},
		Description: "unzip AAR",
		Args: map[string]string{
			"outDir":             extractedAARDir.String(),
			"combinedClassesJar": classpathFile.String(),
			"assetsPackage":      assetsPackage.String(),
		},
	})

	a.addKSnapshot(ctx, classpathFile)

	a.proguardFlags = proguardFlags
	a.assetsPackage = assetsPackage
	a.rTxt = rTxt

	// Always set --pseudo-localize, it will be stripped out later for release
	// builds that don't want it.
	compileFlags := []string{"--pseudo-localize"}
	compiledResDir := android.PathForModuleOut(ctx, "flat-res")
	flata := compiledResDir.Join(ctx, "gen_res.flata")
	aapt2CompileZip(ctx, flata, a.aarPath, "res", compileFlags)

	exportPackage := android.PathForModuleOut(ctx, "package-res.apk")
	proguardOptionsFile := android.PathForModuleGen(ctx, "proguard.options")
	aaptRTxt := android.PathForModuleOut(ctx, "R.txt")
	extraAaptPackagesFile := android.PathForModuleOut(ctx, "extra_packages")

	var linkDeps android.Paths

	linkFlags := []string{
		"--static-lib",
		"--merge-only",
		"--auto-add-overlay",
		"--no-static-lib-packages",
	}

	linkFlags = append(linkFlags, "--manifest "+a.manifest.String())
	linkDeps = append(linkDeps, a.manifest)

	staticResourcesNodesDepSet, sharedResourcesNodesDepSet, staticRRODirsDepSet, staticManifestsDepSet, sharedLibs, libFlags :=
		aaptLibs(ctx, android.SdkContext(a), nil, nil)

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
	aapt2Link(ctx, exportPackage, nil, proguardOptionsFile, aaptRTxt, nil,
		linkFlags, linkDeps, nil, overlayRes, transitiveAssets, nil, nil)
	ctx.CheckbuildFile(exportPackage)
	a.exportPackage = exportPackage

	rJar := android.PathForModuleOut(ctx, "busybox/R.jar")
	resourceProcessorBusyBoxGenerateBinaryR(ctx, a.rTxt, a.manifest, rJar, nil, true, nil, false)
	ctx.CheckbuildFile(rJar)
	a.rJar = rJar
	a.addKSnapshot(ctx, a.rJar)

	aapt2ExtractExtraPackages(ctx, extraAaptPackagesFile, a.rJar)
	a.extraAaptPackagesFile = extraAaptPackagesFile

	resourcesNodesDepSetBuilder := depset.NewBuilder[resourcesNode](depset.TOPOLOGICAL)
	resourcesNodesDepSetBuilder.Direct(resourcesNode{
		resPackage: a.exportPackage,
		manifest:   a.manifest,
		rTxt:       a.rTxt,
		rJar:       a.rJar,
		assets:     android.OptionalPathForPath(a.assetsPackage),

		usedResourceProcessor: true,
	})
	resourcesNodesDepSetBuilder.Transitive(staticResourcesNodesDepSet)
	a.resourcesNodesDepSet = resourcesNodesDepSetBuilder.Build()

	manifestDepSetBuilder := depset.NewBuilder[android.Path](depset.TOPOLOGICAL).Direct(a.manifest)
	manifestDepSetBuilder.Transitive(staticManifestsDepSet)
	a.manifestsDepSet = manifestDepSetBuilder.Build()

	transitiveAaptResourcePackages := staticDeps.resPackages().Strings()
	transitiveAaptResourcePackages = slices.DeleteFunc(transitiveAaptResourcePackages, func(p string) bool {
		return p == a.exportPackage.String()
	})
	transitiveAaptResourcePackagesFile := android.PathForModuleOut(ctx, "transitive-res-packages")
	android.WriteFileRule(ctx, transitiveAaptResourcePackagesFile, strings.Join(transitiveAaptResourcePackages, "\n"))
	a.transitiveAaptResourcePackagesFile = transitiveAaptResourcePackagesFile

	a.collectTransitiveHeaderJarsForR8(ctx)

	a.classLoaderContexts = a.usesLibrary.classLoaderContextForUsesLibDeps(ctx)

	var staticJars android.Paths
	var staticHeaderJars android.Paths
	var staticResourceJars android.Paths
	var transitiveStaticLibsHeaderJars []depset.DepSet[android.Path]
	var transitiveStaticLibsImplementationJars []depset.DepSet[android.Path]
	var transitiveStaticLibsResourceJars []depset.DepSet[android.Path]

	ctx.VisitDirectDepsProxy(func(module android.ModuleProxy) {
		if dep, ok := android.OtherModuleProvider(ctx, module, JavaInfoProvider); ok {
			tag := ctx.OtherModuleDependencyTag(module)
			switch tag {
			case staticLibTag:
				staticJars = append(staticJars, dep.ImplementationJars...)
				staticHeaderJars = append(staticHeaderJars, dep.HeaderJars...)
				staticResourceJars = append(staticResourceJars, dep.ResourceJars...)
				transitiveStaticLibsHeaderJars = append(transitiveStaticLibsHeaderJars, dep.TransitiveStaticLibsHeaderJars)
				transitiveStaticLibsImplementationJars = append(transitiveStaticLibsImplementationJars, dep.TransitiveStaticLibsImplementationJars)
				transitiveStaticLibsResourceJars = append(transitiveStaticLibsResourceJars, dep.TransitiveStaticLibsResourceJars)
				maps.Copy(a.kSnapshotFiles, dep.KSnapshotFiles)
			}
		}
		addCLCFromDep(ctx, module, a.classLoaderContexts)
		addMissingOptionalUsesLibsFromDep(ctx, module, &a.usesLibrary)
	})

	completeStaticLibsHeaderJars := depset.New(depset.PREORDER, android.Paths{classpathFile}, transitiveStaticLibsHeaderJars)
	completeStaticLibsImplementationJars := depset.New(depset.PREORDER, android.Paths{classpathFile}, transitiveStaticLibsImplementationJars)
	completeStaticLibsResourceJars := depset.New(depset.PREORDER, nil, transitiveStaticLibsResourceJars)

	var implementationJarFile android.Path
	combineJars := completeStaticLibsImplementationJars.ToList()

	if len(combineJars) > 1 {
		implementationJarOutputPath := android.PathForModuleOut(ctx, "combined", jarName)
		TransformJarsToJar(ctx, implementationJarOutputPath, "combine", combineJars, android.OptionalPath{}, false, nil, nil)
		implementationJarFile = implementationJarOutputPath
	} else {
		implementationJarFile = classpathFile
	}

	var resourceJarFile android.Path
	resourceJars := completeStaticLibsResourceJars.ToList()

	if len(resourceJars) > 1 {
		combinedJar := android.PathForModuleOut(ctx, "res-combined", jarName)
		TransformJarsToJar(ctx, combinedJar, "for resources", resourceJars, android.OptionalPath{},
			false, nil, nil)
		resourceJarFile = combinedJar
	} else if len(resourceJars) == 1 {
		resourceJarFile = resourceJars[0]
	}

	// merge implementation jar with resources if necessary
	implementationAndResourcesJars := append(slices.Clone(resourceJars), combineJars...)

	var implementationAndResourcesJar android.Path
	if len(implementationAndResourcesJars) > 1 {
		combinedJar := android.PathForModuleOut(ctx, "withres", jarName)
		TransformJarsToJar(ctx, combinedJar, "for resources", implementationAndResourcesJars, android.OptionalPath{},
			false, nil, nil)
		implementationAndResourcesJar = combinedJar
	} else {
		implementationAndResourcesJar = implementationAndResourcesJars[0]
	}

	a.implementationJarFile = implementationJarFile
	// Save the output file with no relative path so that it doesn't end up in a subdirectory when used as a resource
	a.implementationAndResourcesJarFile = implementationAndResourcesJar.WithoutRel()

	headerJars := completeStaticLibsHeaderJars.ToList()
	if len(headerJars) > 1 {
		headerJarFile := android.PathForModuleOut(ctx, "turbine-combined", jarName)
		TransformJarsToJar(ctx, headerJarFile, "combine header jars", headerJars, android.OptionalPath{}, false, nil, nil)
		a.headerJarFile = headerJarFile
	} else {
		a.headerJarFile = headerJars[0]
	}

	ctx.CheckbuildFile(classpathFile)

	javaInfo := &JavaInfo{
		HeaderJars:                             android.PathsIfNonNil(a.headerJarFile),
		LocalHeaderJars:                        android.PathsIfNonNil(classpathFile),
		TransitiveStaticLibsHeaderJars:         completeStaticLibsHeaderJars,
		TransitiveStaticLibsImplementationJars: completeStaticLibsImplementationJars,
		TransitiveStaticLibsResourceJars:       completeStaticLibsResourceJars,
		ResourceJars:                           android.PathsIfNonNil(resourceJarFile),
		TransitiveLibsHeaderJarsForR8:          a.transitiveLibsHeaderJarsForR8,
		TransitiveStaticLibsHeaderJarsForR8:    a.transitiveStaticLibsHeaderJarsForR8,
		ImplementationAndResourcesJars:         android.PathsIfNonNil(a.implementationAndResourcesJarFile),
		ImplementationJars:                     android.PathsIfNonNil(a.implementationJarFile),
		StubsLinkType:                          Implementation,
		KSnapshotFiles:                         a.kSnapshotFiles,
		// TransitiveAconfigFiles: // TODO(b/289117800): LOCAL_ACONFIG_FILES for prebuilts
	}
	setExtraJavaInfo(ctx, a, javaInfo)
	android.SetProvider(ctx, JavaInfoProvider, javaInfo)

	if proptools.Bool(a.properties.Extract_jni) {
		for _, t := range ctx.MultiTargets() {
			// Don't attempt to extract JNI libraries for riscv64, we don't have any AARs with riscv64 JNI library support yet.
			if t.Arch.ArchType == android.Riscv64 {
				continue
			}
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
	}

	android.SetProvider(ctx, JniPackageProvider, JniPackageInfo{
		JniPackages: a.jniPackages,
	})

	android.SetProvider(ctx, AARImportInfoProvider, AARImportInfo{})

	ctx.SetOutputFiles([]android.Path{a.implementationAndResourcesJarFile}, "")
	ctx.SetOutputFiles([]android.Path{a.aarPath}, ".aar")

	buildComplianceMetadata(ctx)

	moduleInfoJSON := ctx.ModuleInfoJSON()
	moduleInfoJSON.Class = []string{"JAVA_LIBRARIES"}
	moduleInfoJSON.ClassesJar = []string{a.implementationAndResourcesJarFile.String()}
	moduleInfoJSON.SystemSharedLibs = []string{"none"}
}

func (a *AARImport) addKSnapshot(ctx android.ModuleContext, jarFile android.Path) {
	if jarFile == nil {
		return
	}
	if _, exists := a.kSnapshotFiles[jarFile.String()]; !exists {
		snapshot := SnapshotJarForKotlin(ctx, jarFile.(android.WritablePath))
		a.kSnapshotFiles[jarFile.String()] = snapshot
	}
}

func (a *AARImport) JarToSnapshotMap() map[string]android.Path {
	return a.kSnapshotFiles
}

var _ KSnapshotContainer = (*AARImport)(nil)

func (a *AARImport) HeaderJars() android.Paths {
	return android.Paths{a.headerJarFile}
}

func (a *AARImport) ImplementationAndResourcesJars() android.Paths {
	return android.Paths{a.implementationAndResourcesJarFile}
}

func (a *AARImport) DexJarBuildPath(ctx android.ModuleErrorfContext) OptionalDexJarPath {
	return OptionalDexJarPath{}
}

func (a *AARImport) DexJarInstallPath() android.Path {
	return nil
}

func (a *AARImport) ClassLoaderContexts() dexpreopt.ClassLoaderContextMap {
	return a.classLoaderContexts
}

var _ UsesLibraryDependency = (*AARImport)(nil)

var _ android.ApexModule = (*AARImport)(nil)

// Implements android.ApexModule
func (m *AARImport) GetDepInSameApexChecker() android.DepInSameApexChecker {
	return AARImportDepInSameApexChecker{}
}

// @auto-generate: gob
type AARImportDepInSameApexChecker struct {
	android.BaseDepInSameApexChecker
}

func (m AARImportDepInSameApexChecker) OutgoingDepIsInSameApex(tag blueprint.DependencyTag) bool {
	return depIsInSameApex(tag)
}

// Implements android.ApexModule
func (a *AARImport) MinSdkVersionSupported(ctx android.BaseModuleContext) android.ApiLevel {
	return android.MinApiLevel
}

var _ android.PrebuiltInterface = (*AARImport)(nil)

func (a *AARImport) UsesLibrary() *usesLibrary {
	return &a.usesLibrary
}

var _ ModuleWithUsesLibrary = (*AARImport)(nil)

// android_library_import imports an `.aar` file into the build graph as if it was built with android_library.
//
// This module is not suitable for installing on a device, but can be used as a `static_libs` dependency of
// an android_app module.
func AARImportFactory() android.Module {
	module := &AARImport{}

	module.AddProperties(
		&module.properties,
		&module.usesLibrary.usesLibraryProperties,
	)

	android.InitPrebuiltModule(module, &module.properties.Aars)
	android.InitApexModule(module)
	InitJavaModuleMultiTargets(module, android.DeviceSupported)
	return module
}

func (a *AARImport) IDEInfo(ctx android.BaseModuleContext, dpInfo *android.IdeInfo) {
	dpInfo.Jars = append(dpInfo.Jars, a.implementationJarFile.String(), a.rJar.String(), a.aarPath.String())
	dpInfo.Static_libs = append(dpInfo.Static_libs, a.properties.Static_libs.GetOrDefault(ctx, nil)...)
	dpInfo.Imported_aars = append(dpInfo.Imported_aars, android.PathsForModuleSrc(ctx, a.properties.Aars).Strings()...)
}

// @auto-generate: gob
type AARInfo struct {
	Aar android.Path
}

var AARProvider = blueprint.NewProvider[AARInfo]()
