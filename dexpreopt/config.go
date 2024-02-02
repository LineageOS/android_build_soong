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

package dexpreopt

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
)

// GlobalConfig stores the configuration for dex preopting. The fields are set
// from product variables via dex_preopt_config.mk.
type GlobalConfig struct {
	DisablePreopt           bool     // disable preopt for all modules (excluding boot images)
	DisablePreoptBootImages bool     // disable prepot for boot images
	DisablePreoptModules    []string // modules with preopt disabled by product-specific config

	OnlyPreoptArtBootImage bool // only preopt jars in the ART boot image

	PreoptWithUpdatableBcp bool // If updatable boot jars are included in dexpreopt or not.

	HasSystemOther        bool     // store odex files that match PatternsOnSystemOther on the system_other partition
	PatternsOnSystemOther []string // patterns (using '%' to denote a prefix match) to put odex on the system_other partition

	DisableGenerateProfile bool   // don't generate profiles
	ProfileDir             string // directory to find profiles in

	BootJars     android.ConfiguredJarList // modules for jars that form the boot class path
	ApexBootJars android.ConfiguredJarList // jars within apex that form the boot class path

	ArtApexJars              android.ConfiguredJarList // modules for jars that are in the ART APEX
	TestOnlyArtBootImageJars android.ConfiguredJarList // modules for jars to be included in the ART boot image for testing

	SystemServerJars               android.ConfiguredJarList // system_server classpath jars on the platform
	SystemServerApps               []string                  // apps that are loaded into system server
	ApexSystemServerJars           android.ConfiguredJarList // system_server classpath jars delivered via apex
	StandaloneSystemServerJars     android.ConfiguredJarList // jars on the platform that system_server loads dynamically using separate classloaders
	ApexStandaloneSystemServerJars android.ConfiguredJarList // jars delivered via apex that system_server loads dynamically using separate classloaders
	SpeedApps                      []string                  // apps that should be speed optimized

	BrokenSuboptimalOrderOfSystemServerJars bool // if true, sub-optimal order does not cause a build error

	PreoptFlags []string // global dex2oat flags that should be used if no module-specific dex2oat flags are specified

	DefaultCompilerFilter      string // default compiler filter to pass to dex2oat, overridden by --compiler-filter= in module-specific dex2oat flags
	SystemServerCompilerFilter string // default compiler filter to pass to dex2oat for system server jars

	GenerateDMFiles bool // generate Dex Metadata files

	NoDebugInfo                 bool // don't generate debug info by default
	DontResolveStartupStrings   bool // don't resolve string literals loaded during application startup.
	AlwaysSystemServerDebugInfo bool // always generate mini debug info for system server modules (overrides NoDebugInfo=true)
	NeverSystemServerDebugInfo  bool // never generate mini debug info for system server modules (overrides NoDebugInfo=false)
	AlwaysOtherDebugInfo        bool // always generate mini debug info for non-system server modules (overrides NoDebugInfo=true)
	NeverOtherDebugInfo         bool // never generate mini debug info for non-system server modules (overrides NoDebugInfo=true)

	IsEng        bool // build is a eng variant
	SanitizeLite bool // build is the second phase of a SANITIZE_LITE build

	DefaultAppImages bool // build app images (TODO: .art files?) by default

	Dex2oatXmx string // max heap size for dex2oat
	Dex2oatXms string // initial heap size for dex2oat

	EmptyDirectory string // path to an empty directory

	CpuVariant             map[android.ArchType]string // cpu variant for each architecture
	InstructionSetFeatures map[android.ArchType]string // instruction set for each architecture

	BootImageProfiles android.Paths // path to a boot-image-profile.txt file
	BootFlags         string        // extra flags to pass to dex2oat for the boot image
	Dex2oatImageXmx   string        // max heap size for dex2oat for the boot image
	Dex2oatImageXms   string        // initial heap size for dex2oat for the boot image

	// If true, downgrade the compiler filter of dexpreopt to "verify" when verify_uses_libraries
	// check fails, instead of failing the build. This will disable any AOT-compilation.
	//
	// The intended use case for this flag is to have a smoother migration path for the Java
	// modules that need to add <uses-library> information in their build files. The flag allows to
	// quickly silence build errors. This flag should be used with caution and only as a temporary
	// measure, as it masks real errors and affects performance.
	RelaxUsesLibraryCheck bool

	// "true" to force preopt with CMC GC (a.k.a., UFFD GC); "false" to force preopt with CC GC;
	// "default" to determine the GC type based on the kernel version file.
	EnableUffdGc string
}

var allPlatformSystemServerJarsKey = android.NewOnceKey("allPlatformSystemServerJars")

// Returns all jars on the platform that system_server loads, including those on classpath and those
// loaded dynamically.
func (g *GlobalConfig) AllPlatformSystemServerJars(ctx android.PathContext) *android.ConfiguredJarList {
	return ctx.Config().Once(allPlatformSystemServerJarsKey, func() interface{} {
		res := g.SystemServerJars.AppendList(&g.StandaloneSystemServerJars)
		return &res
	}).(*android.ConfiguredJarList)
}

var allApexSystemServerJarsKey = android.NewOnceKey("allApexSystemServerJars")

// Returns all jars delivered via apex that system_server loads, including those on classpath and
// those loaded dynamically.
func (g *GlobalConfig) AllApexSystemServerJars(ctx android.PathContext) *android.ConfiguredJarList {
	return ctx.Config().Once(allApexSystemServerJarsKey, func() interface{} {
		res := g.ApexSystemServerJars.AppendList(&g.ApexStandaloneSystemServerJars)
		return &res
	}).(*android.ConfiguredJarList)
}

var allSystemServerClasspathJarsKey = android.NewOnceKey("allSystemServerClasspathJars")

// Returns all system_server classpath jars.
func (g *GlobalConfig) AllSystemServerClasspathJars(ctx android.PathContext) *android.ConfiguredJarList {
	return ctx.Config().Once(allSystemServerClasspathJarsKey, func() interface{} {
		res := g.SystemServerJars.AppendList(&g.ApexSystemServerJars)
		return &res
	}).(*android.ConfiguredJarList)
}

var allSystemServerJarsKey = android.NewOnceKey("allSystemServerJars")

// Returns all jars that system_server loads.
func (g *GlobalConfig) AllSystemServerJars(ctx android.PathContext) *android.ConfiguredJarList {
	return ctx.Config().Once(allSystemServerJarsKey, func() interface{} {
		res := g.AllPlatformSystemServerJars(ctx).AppendList(g.AllApexSystemServerJars(ctx))
		return &res
	}).(*android.ConfiguredJarList)
}

// GlobalSoongConfig contains the global config that is generated from Soong,
// stored in dexpreopt_soong.config.
type GlobalSoongConfig struct {
	// Paths to tools possibly used by the generated commands.
	Profman          android.Path
	Dex2oat          android.Path
	Aapt             android.Path
	SoongZip         android.Path
	Zip2zip          android.Path
	ManifestCheck    android.Path
	ConstructContext android.Path
	UffdGcFlag       android.WritablePath
}

type ModuleConfig struct {
	Name            string
	DexLocation     string // dex location on device
	BuildPath       android.OutputPath
	DexPath         android.Path
	ManifestPath    android.OptionalPath
	UncompressedDex bool
	HasApkLibraries bool
	PreoptFlags     []string

	ProfileClassListing  android.OptionalPath
	ProfileIsTextListing bool
	ProfileBootListing   android.OptionalPath

	EnforceUsesLibraries           bool         // turn on build-time verify_uses_libraries check
	EnforceUsesLibrariesStatusFile android.Path // a file with verify_uses_libraries errors (if any)
	ProvidesUsesLibrary            string       // library name (usually the same as module name)
	ClassLoaderContexts            ClassLoaderContextMap

	Archs               []android.ArchType
	DexPreoptImagesDeps []android.OutputPaths

	DexPreoptImageLocationsOnHost   []string // boot image location on host (file path without the arch subdirectory)
	DexPreoptImageLocationsOnDevice []string // boot image location on device (file path without the arch subdirectory)

	PreoptBootClassPathDexFiles     android.Paths // file paths of boot class path files
	PreoptBootClassPathDexLocations []string      // virtual locations of boot class path files

	NoCreateAppImage    bool
	ForceCreateAppImage bool

	PresignedPrebuilt bool
}

type globalSoongConfigSingleton struct{}

var pctx = android.NewPackageContext("android/soong/dexpreopt")

func init() {
	pctx.Import("android/soong/android")
	android.RegisterParallelSingletonType("dexpreopt-soong-config", func() android.Singleton {
		return &globalSoongConfigSingleton{}
	})
}

func constructPath(ctx android.PathContext, path string) android.Path {
	buildDirPrefix := ctx.Config().SoongOutDir() + "/"
	if path == "" {
		return nil
	} else if strings.HasPrefix(path, buildDirPrefix) {
		return android.PathForOutput(ctx, strings.TrimPrefix(path, buildDirPrefix))
	} else {
		return android.PathForSource(ctx, path)
	}
}

func constructPaths(ctx android.PathContext, paths []string) android.Paths {
	var ret android.Paths
	for _, path := range paths {
		ret = append(ret, constructPath(ctx, path))
	}
	return ret
}

func constructWritablePath(ctx android.PathContext, path string) android.WritablePath {
	if path == "" {
		return nil
	}
	return constructPath(ctx, path).(android.WritablePath)
}

// ParseGlobalConfig parses the given data assumed to be read from the global
// dexpreopt.config file into a GlobalConfig struct.
func ParseGlobalConfig(ctx android.PathContext, data []byte) (*GlobalConfig, error) {
	type GlobalJSONConfig struct {
		*GlobalConfig

		// Copies of entries in GlobalConfig that are not constructable without extra parameters.  They will be
		// used to construct the real value manually below.
		BootImageProfiles []string
	}

	config := GlobalJSONConfig{}
	err := json.Unmarshal(data, &config)
	if err != nil {
		return config.GlobalConfig, err
	}

	// Construct paths that require a PathContext.
	config.GlobalConfig.BootImageProfiles = constructPaths(ctx, config.BootImageProfiles)

	return config.GlobalConfig, nil
}

type globalConfigAndRaw struct {
	global     *GlobalConfig
	data       []byte
	pathErrors []error
}

// GetGlobalConfig returns the global dexpreopt.config that's created in the
// make config phase. It is loaded once the first time it is called for any
// ctx.Config(), and returns the same data for all future calls with the same
// ctx.Config(). A value can be inserted for tests using
// setDexpreoptTestGlobalConfig.
func GetGlobalConfig(ctx android.PathContext) *GlobalConfig {
	return getGlobalConfigRaw(ctx).global
}

// GetGlobalConfigRawData is the same as GetGlobalConfig, except that it returns
// the literal content of dexpreopt.config.
func GetGlobalConfigRawData(ctx android.PathContext) []byte {
	return getGlobalConfigRaw(ctx).data
}

var globalConfigOnceKey = android.NewOnceKey("DexpreoptGlobalConfig")
var testGlobalConfigOnceKey = android.NewOnceKey("TestDexpreoptGlobalConfig")

type pathContextErrorCollector struct {
	android.PathContext
	errors []error
}

func (p *pathContextErrorCollector) Errorf(format string, args ...interface{}) {
	p.errors = append(p.errors, fmt.Errorf(format, args...))
}

func getGlobalConfigRaw(ctx android.PathContext) globalConfigAndRaw {
	config := ctx.Config().Once(globalConfigOnceKey, func() interface{} {
		if data, err := ctx.Config().DexpreoptGlobalConfig(ctx); err != nil {
			panic(err)
		} else if data != nil {
			pathErrorCollectorCtx := &pathContextErrorCollector{PathContext: ctx}
			globalConfig, err := ParseGlobalConfig(pathErrorCollectorCtx, data)
			if err != nil {
				panic(err)
			}
			return globalConfigAndRaw{globalConfig, data, pathErrorCollectorCtx.errors}
		}

		// No global config filename set, see if there is a test config set
		return ctx.Config().Once(testGlobalConfigOnceKey, func() interface{} {
			// Nope, return a config with preopting disabled
			return globalConfigAndRaw{&GlobalConfig{
				DisablePreopt:           true,
				DisablePreoptBootImages: true,
				DisableGenerateProfile:  true,
			}, nil, nil}
		})
	}).(globalConfigAndRaw)

	// Avoid non-deterministic errors by reporting cached path errors on all callers.
	for _, err := range config.pathErrors {
		if ctx.Config().AllowMissingDependencies() {
			// When AllowMissingDependencies it set, report errors through AddMissingDependencies.
			// If AddMissingDependencies doesn't exist on the current context (for example when
			// called with a SingletonContext), just swallow the errors since there is no way to
			// report them.
			if missingDepsCtx, ok := ctx.(interface {
				AddMissingDependencies(missingDeps []string)
			}); ok {
				missingDepsCtx.AddMissingDependencies([]string{err.Error()})
			}
		} else {
			android.ReportPathErrorf(ctx, "%s", err)
		}
	}

	return config
}

// SetTestGlobalConfig sets a GlobalConfig that future calls to GetGlobalConfig
// will return. It must be called before the first call to GetGlobalConfig for
// the config.
func SetTestGlobalConfig(config android.Config, globalConfig *GlobalConfig) {
	config.Once(testGlobalConfigOnceKey, func() interface{} { return globalConfigAndRaw{globalConfig, nil, nil} })
}

// This struct is required to convert ModuleConfig from/to JSON.
// The types of fields in ModuleConfig are not convertible,
// so moduleJSONConfig has those fields as a convertible type.
type moduleJSONConfig struct {
	*ModuleConfig

	BuildPath    string
	DexPath      string
	ManifestPath string

	ProfileClassListing string
	ProfileBootListing  string

	EnforceUsesLibrariesStatusFile string
	ClassLoaderContexts            jsonClassLoaderContextMap

	DexPreoptImagesDeps [][]string

	PreoptBootClassPathDexFiles []string
}

// ParseModuleConfig parses a per-module dexpreopt.config file into a
// ModuleConfig struct. It is not used in Soong, which receives a ModuleConfig
// struct directly from java/dexpreopt.go. It is used in dexpreopt_gen called
// from Make to read the module dexpreopt.config written in the Make config
// stage.
func ParseModuleConfig(ctx android.PathContext, data []byte) (*ModuleConfig, error) {
	config := moduleJSONConfig{}

	err := json.Unmarshal(data, &config)
	if err != nil {
		return config.ModuleConfig, err
	}

	// Construct paths that require a PathContext.
	config.ModuleConfig.BuildPath = constructPath(ctx, config.BuildPath).(android.OutputPath)
	config.ModuleConfig.DexPath = constructPath(ctx, config.DexPath)
	config.ModuleConfig.ManifestPath = android.OptionalPathForPath(constructPath(ctx, config.ManifestPath))
	config.ModuleConfig.ProfileClassListing = android.OptionalPathForPath(constructPath(ctx, config.ProfileClassListing))
	config.ModuleConfig.EnforceUsesLibrariesStatusFile = constructPath(ctx, config.EnforceUsesLibrariesStatusFile)
	config.ModuleConfig.ClassLoaderContexts = fromJsonClassLoaderContext(ctx, config.ClassLoaderContexts)
	config.ModuleConfig.PreoptBootClassPathDexFiles = constructPaths(ctx, config.PreoptBootClassPathDexFiles)

	// This needs to exist, but dependencies are already handled in Make, so we don't need to pass them through JSON.
	config.ModuleConfig.DexPreoptImagesDeps = make([]android.OutputPaths, len(config.ModuleConfig.Archs))

	return config.ModuleConfig, nil
}

func pathsListToStringLists(pathsList []android.OutputPaths) [][]string {
	ret := make([][]string, 0, len(pathsList))
	for _, paths := range pathsList {
		ret = append(ret, paths.Strings())
	}
	return ret
}

func moduleConfigToJSON(config *ModuleConfig) ([]byte, error) {
	return json.MarshalIndent(&moduleJSONConfig{
		BuildPath:                      config.BuildPath.String(),
		DexPath:                        config.DexPath.String(),
		ManifestPath:                   config.ManifestPath.String(),
		ProfileClassListing:            config.ProfileClassListing.String(),
		ProfileBootListing:             config.ProfileBootListing.String(),
		EnforceUsesLibrariesStatusFile: config.EnforceUsesLibrariesStatusFile.String(),
		ClassLoaderContexts:            toJsonClassLoaderContext(config.ClassLoaderContexts),
		DexPreoptImagesDeps:            pathsListToStringLists(config.DexPreoptImagesDeps),
		PreoptBootClassPathDexFiles:    config.PreoptBootClassPathDexFiles.Strings(),
		ModuleConfig:                   config,
	}, "", "    ")
}

// WriteModuleConfig serializes a ModuleConfig into a per-module dexpreopt.config JSON file.
// These config files are used for post-processing.
func WriteModuleConfig(ctx android.ModuleContext, config *ModuleConfig, path android.WritablePath) {
	if path == nil {
		return
	}

	data, err := moduleConfigToJSON(config)
	if err != nil {
		ctx.ModuleErrorf("failed to JSON marshal module dexpreopt.config: %v", err)
		return
	}

	android.WriteFileRule(ctx, path, string(data))
}

// dex2oatModuleName returns the name of the module to use for the dex2oat host
// tool. It should be a binary module with public visibility that is compiled
// and installed for host.
func dex2oatModuleName(config android.Config) string {
	// Default to the debug variant of dex2oat to help find bugs.
	// Set USE_DEX2OAT_DEBUG to false for only building non-debug versions.
	if config.Getenv("USE_DEX2OAT_DEBUG") == "false" {
		return "dex2oat"
	} else {
		return "dex2oatd"
	}
}

type dex2oatDependencyTag struct {
	blueprint.BaseDependencyTag
	android.LicenseAnnotationToolchainDependencyTag
}

func (d dex2oatDependencyTag) ExcludeFromVisibilityEnforcement() {
}

func (d dex2oatDependencyTag) ExcludeFromApexContents() {
}

func (d dex2oatDependencyTag) AllowDisabledModuleDependency(target android.Module) bool {
	// RegisterToolDeps may run after the prebuilt mutators and hence register a
	// dependency on the source module even when the prebuilt is to be used.
	// dex2oatPathFromDep takes that into account when it retrieves the path to
	// the binary, but we also need to disable the check for dependencies on
	// disabled modules.
	return target.IsReplacedByPrebuilt()
}

// Dex2oatDepTag represents the dependency onto the dex2oatd module. It is added to any module that
// needs dexpreopting and so it makes no sense for it to be checked for visibility or included in
// the apex.
var Dex2oatDepTag = dex2oatDependencyTag{}

var _ android.ExcludeFromVisibilityEnforcementTag = Dex2oatDepTag
var _ android.ExcludeFromApexContentsTag = Dex2oatDepTag
var _ android.AllowDisabledModuleDependency = Dex2oatDepTag

// RegisterToolDeps adds the necessary dependencies to binary modules for tools
// that are required later when Get(Cached)GlobalSoongConfig is called. It
// should be called from a mutator that's registered with
// android.RegistrationContext.FinalDepsMutators.
func RegisterToolDeps(ctx android.BottomUpMutatorContext) {
	dex2oatBin := dex2oatModuleName(ctx.Config())
	v := ctx.Config().BuildOSTarget.Variations()
	ctx.AddFarVariationDependencies(v, Dex2oatDepTag, dex2oatBin)
}

func IsDex2oatNeeded(ctx android.PathContext) bool {
	global := GetGlobalConfig(ctx)
	return !global.DisablePreopt || !global.DisablePreoptBootImages
}

func dex2oatPathFromDep(ctx android.ModuleContext) android.Path {
	if !IsDex2oatNeeded(ctx) {
		return nil
	}

	dex2oatBin := dex2oatModuleName(ctx.Config())

	// Find the right dex2oat module, trying to follow PrebuiltDepTag from source
	// to prebuilt if there is one. We wouldn't have to do this if the
	// prebuilt_postdeps mutator that replaces source deps with prebuilt deps was
	// run after RegisterToolDeps above, but changing that leads to ordering
	// problems between mutators (RegisterToolDeps needs to run late to act on
	// final variants, while prebuilt_postdeps needs to run before many of the
	// PostDeps mutators, like the APEX mutators). Hence we need to dig out the
	// prebuilt explicitly here instead.
	var dex2oatModule android.Module
	ctx.WalkDeps(func(child, parent android.Module) bool {
		if parent == ctx.Module() && ctx.OtherModuleDependencyTag(child) == Dex2oatDepTag {
			// Found the source module, or prebuilt module that has replaced the source.
			dex2oatModule = child
			if android.IsModulePrebuilt(child) {
				return false // If it's the prebuilt we're done.
			} else {
				return true // Recurse to check if the source has a prebuilt dependency.
			}
		}
		if parent == dex2oatModule && ctx.OtherModuleDependencyTag(child) == android.PrebuiltDepTag {
			if p := android.GetEmbeddedPrebuilt(child); p != nil && p.UsePrebuilt() {
				dex2oatModule = child // Found a prebuilt that should be used.
			}
		}
		return false
	})

	if dex2oatModule == nil {
		// If this happens there's probably a missing call to AddToolDeps in DepsMutator.
		panic(fmt.Sprintf("Failed to lookup %s dependency", dex2oatBin))
	}

	dex2oatPath := dex2oatModule.(android.HostToolProvider).HostToolPath()
	if !dex2oatPath.Valid() {
		panic(fmt.Sprintf("Failed to find host tool path in %s", dex2oatModule))
	}

	return dex2oatPath.Path()
}

// createGlobalSoongConfig creates a GlobalSoongConfig from the current context.
// Should not be used in dexpreopt_gen.
func createGlobalSoongConfig(ctx android.ModuleContext) *GlobalSoongConfig {
	return &GlobalSoongConfig{
		Profman:          ctx.Config().HostToolPath(ctx, "profman"),
		Dex2oat:          dex2oatPathFromDep(ctx),
		Aapt:             ctx.Config().HostToolPath(ctx, "aapt2"),
		SoongZip:         ctx.Config().HostToolPath(ctx, "soong_zip"),
		Zip2zip:          ctx.Config().HostToolPath(ctx, "zip2zip"),
		ManifestCheck:    ctx.Config().HostToolPath(ctx, "manifest_check"),
		ConstructContext: ctx.Config().HostToolPath(ctx, "construct_context"),
		UffdGcFlag:       getUffdGcFlagPath(ctx),
	}
}

// The main reason for this Once cache for GlobalSoongConfig is to make the
// dex2oat path available to singletons. In ordinary modules we get it through a
// Dex2oatDepTag dependency, but in singletons there's no simple way to do the
// same thing and ensure the right variant is selected, hence this cache to make
// the resolved path available to singletons. This means we depend on there
// being at least one ordinary module with a Dex2oatDepTag dependency.
//
// TODO(b/147613152): Implement a way to deal with dependencies from singletons,
// and then possibly remove this cache altogether.
var globalSoongConfigOnceKey = android.NewOnceKey("DexpreoptGlobalSoongConfig")

// GetGlobalSoongConfig creates a GlobalSoongConfig the first time it's called,
// and later returns the same cached instance.
func GetGlobalSoongConfig(ctx android.ModuleContext) *GlobalSoongConfig {
	globalSoong := ctx.Config().Once(globalSoongConfigOnceKey, func() interface{} {
		return createGlobalSoongConfig(ctx)
	}).(*GlobalSoongConfig)

	// Always resolve the tool path from the dependency, to ensure that every
	// module has the dependency added properly.
	myDex2oat := dex2oatPathFromDep(ctx)
	if myDex2oat != globalSoong.Dex2oat {
		panic(fmt.Sprintf("Inconsistent dex2oat path in cached config: expected %s, got %s", globalSoong.Dex2oat, myDex2oat))
	}

	return globalSoong
}

// GetCachedGlobalSoongConfig returns a cached GlobalSoongConfig created by an
// earlier GetGlobalSoongConfig call. This function works with any context
// compatible with a basic PathContext, since it doesn't try to create a
// GlobalSoongConfig with the proper paths (which requires a full
// ModuleContext). If there has been no prior call to GetGlobalSoongConfig, nil
// is returned.
func GetCachedGlobalSoongConfig(ctx android.PathContext) *GlobalSoongConfig {
	return ctx.Config().Once(globalSoongConfigOnceKey, func() interface{} {
		return (*GlobalSoongConfig)(nil)
	}).(*GlobalSoongConfig)
}

type globalJsonSoongConfig struct {
	Profman          string
	Dex2oat          string
	Aapt             string
	SoongZip         string
	Zip2zip          string
	ManifestCheck    string
	ConstructContext string
	UffdGcFlag       string
}

// ParseGlobalSoongConfig parses the given data assumed to be read from the
// global dexpreopt_soong.config file into a GlobalSoongConfig struct. It is
// only used in dexpreopt_gen.
func ParseGlobalSoongConfig(ctx android.PathContext, data []byte) (*GlobalSoongConfig, error) {
	var jc globalJsonSoongConfig

	err := json.Unmarshal(data, &jc)
	if err != nil {
		return &GlobalSoongConfig{}, err
	}

	config := &GlobalSoongConfig{
		Profman:          constructPath(ctx, jc.Profman),
		Dex2oat:          constructPath(ctx, jc.Dex2oat),
		Aapt:             constructPath(ctx, jc.Aapt),
		SoongZip:         constructPath(ctx, jc.SoongZip),
		Zip2zip:          constructPath(ctx, jc.Zip2zip),
		ManifestCheck:    constructPath(ctx, jc.ManifestCheck),
		ConstructContext: constructPath(ctx, jc.ConstructContext),
		UffdGcFlag:       constructWritablePath(ctx, jc.UffdGcFlag),
	}

	return config, nil
}

// checkBootJarsConfigConsistency checks the consistency of BootJars and ApexBootJars fields in
// DexpreoptGlobalConfig and Config.productVariables.
func checkBootJarsConfigConsistency(ctx android.SingletonContext, dexpreoptConfig *GlobalConfig, config android.Config) {
	compareBootJars := func(property string, dexpreoptJars, variableJars android.ConfiguredJarList) {
		dexpreoptPairs := dexpreoptJars.CopyOfApexJarPairs()
		variablePairs := variableJars.CopyOfApexJarPairs()
		if !reflect.DeepEqual(dexpreoptPairs, variablePairs) {
			ctx.Errorf("Inconsistent configuration of %[1]s\n"+
				"    dexpreopt.GlobalConfig.%[1]s = %[2]s\n"+
				"    productVariables.%[1]s       = %[3]s",
				property, dexpreoptPairs, variablePairs)
		}
	}

	compareBootJars("BootJars", dexpreoptConfig.BootJars, config.NonApexBootJars())
	compareBootJars("ApexBootJars", dexpreoptConfig.ApexBootJars, config.ApexBootJars())
}

func (s *globalSoongConfigSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	global := GetGlobalConfig(ctx)
	checkBootJarsConfigConsistency(ctx, global, ctx.Config())

	if global.DisablePreopt {
		return
	}

	buildUffdGcFlag(ctx, global)

	config := GetCachedGlobalSoongConfig(ctx)
	if config == nil {
		// No module has enabled dexpreopting, so we assume there will be no calls
		// to dexpreopt_gen.
		return
	}

	jc := globalJsonSoongConfig{
		Profman:          config.Profman.String(),
		Dex2oat:          config.Dex2oat.String(),
		Aapt:             config.Aapt.String(),
		SoongZip:         config.SoongZip.String(),
		Zip2zip:          config.Zip2zip.String(),
		ManifestCheck:    config.ManifestCheck.String(),
		ConstructContext: config.ConstructContext.String(),
		UffdGcFlag:       config.UffdGcFlag.String(),
	}

	data, err := json.Marshal(jc)
	if err != nil {
		ctx.Errorf("failed to JSON marshal GlobalSoongConfig: %v", err)
		return
	}

	android.WriteFileRule(ctx, android.PathForOutput(ctx, "dexpreopt_soong.config"), string(data))
}

func (s *globalSoongConfigSingleton) MakeVars(ctx android.MakeVarsContext) {
	if GetGlobalConfig(ctx).DisablePreopt {
		return
	}

	config := GetCachedGlobalSoongConfig(ctx)
	if config == nil {
		return
	}

	ctx.Strict("DEX2OAT", config.Dex2oat.String())
	ctx.Strict("DEXPREOPT_GEN_DEPS", strings.Join([]string{
		config.Profman.String(),
		config.Dex2oat.String(),
		config.Aapt.String(),
		config.SoongZip.String(),
		config.Zip2zip.String(),
		config.ManifestCheck.String(),
		config.ConstructContext.String(),
		config.UffdGcFlag.String(),
	}, " "))
}

func buildUffdGcFlag(ctx android.BuilderContext, global *GlobalConfig) {
	uffdGcFlag := getUffdGcFlagPath(ctx)

	if global.EnableUffdGc == "true" {
		android.WriteFileRuleVerbatim(ctx, uffdGcFlag, "--runtime-arg -Xgc:CMC")
	} else if global.EnableUffdGc == "false" {
		android.WriteFileRuleVerbatim(ctx, uffdGcFlag, "")
	} else if global.EnableUffdGc == "default" {
		// Generated by `build/make/core/Makefile`.
		kernelVersionFile := android.PathForOutput(ctx, "dexpreopt/kernel_version_for_uffd_gc.txt")
		// Determine the UFFD GC flag by the kernel version file.
		rule := android.NewRuleBuilder(pctx, ctx)
		rule.Command().
			Tool(ctx.Config().HostToolPath(ctx, "construct_uffd_gc_flag")).
			Input(kernelVersionFile).
			Output(uffdGcFlag)
		rule.Restat().Build("dexpreopt_uffd_gc_flag", "dexpreopt_uffd_gc_flag")
	} else {
		panic(fmt.Sprintf("Unknown value of PRODUCT_ENABLE_UFFD_GC: %s", global.EnableUffdGc))
	}
}

func GlobalConfigForTests(ctx android.PathContext) *GlobalConfig {
	return &GlobalConfig{
		DisablePreopt:                  false,
		DisablePreoptModules:           nil,
		OnlyPreoptArtBootImage:         false,
		HasSystemOther:                 false,
		PatternsOnSystemOther:          nil,
		DisableGenerateProfile:         false,
		ProfileDir:                     "",
		BootJars:                       android.EmptyConfiguredJarList(),
		ApexBootJars:                   android.EmptyConfiguredJarList(),
		ArtApexJars:                    android.EmptyConfiguredJarList(),
		TestOnlyArtBootImageJars:       android.EmptyConfiguredJarList(),
		SystemServerJars:               android.EmptyConfiguredJarList(),
		SystemServerApps:               nil,
		ApexSystemServerJars:           android.EmptyConfiguredJarList(),
		StandaloneSystemServerJars:     android.EmptyConfiguredJarList(),
		ApexStandaloneSystemServerJars: android.EmptyConfiguredJarList(),
		SpeedApps:                      nil,
		PreoptFlags:                    nil,
		DefaultCompilerFilter:          "",
		SystemServerCompilerFilter:     "",
		GenerateDMFiles:                false,
		NoDebugInfo:                    false,
		DontResolveStartupStrings:      false,
		AlwaysSystemServerDebugInfo:    false,
		NeverSystemServerDebugInfo:     false,
		AlwaysOtherDebugInfo:           false,
		NeverOtherDebugInfo:            false,
		IsEng:                          false,
		SanitizeLite:                   false,
		DefaultAppImages:               false,
		Dex2oatXmx:                     "",
		Dex2oatXms:                     "",
		EmptyDirectory:                 "empty_dir",
		CpuVariant:                     nil,
		InstructionSetFeatures:         nil,
		BootImageProfiles:              nil,
		BootFlags:                      "",
		Dex2oatImageXmx:                "",
		Dex2oatImageXms:                "",
	}
}

func globalSoongConfigForTests(ctx android.BuilderContext) *GlobalSoongConfig {
	return &GlobalSoongConfig{
		Profman:          android.PathForTesting("profman"),
		Dex2oat:          android.PathForTesting("dex2oat"),
		Aapt:             android.PathForTesting("aapt2"),
		SoongZip:         android.PathForTesting("soong_zip"),
		Zip2zip:          android.PathForTesting("zip2zip"),
		ManifestCheck:    android.PathForTesting("manifest_check"),
		ConstructContext: android.PathForTesting("construct_context"),
		UffdGcFlag:       android.PathForOutput(ctx, "dexpreopt_test", "uffd_gc_flag.txt"),
	}
}

func GetDexpreoptDirName(ctx android.PathContext) string {
	prefix := "dexpreopt_"
	targets := ctx.Config().Targets[android.Android]
	if len(targets) > 0 {
		return prefix + targets[0].Arch.ArchType.String()
	}
	return prefix + "unknown_target"
}

func getUffdGcFlagPath(ctx android.PathContext) android.WritablePath {
	return android.PathForOutput(ctx, "dexpreopt/uffd_gc_flag.txt")
}
