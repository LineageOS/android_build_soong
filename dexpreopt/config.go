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

	OnlyPreoptBootImageAndSystemServer bool // only preopt jars in the boot image or system server

	UseArtImage bool // use the art image (use other boot class path dex files without image)

	HasSystemOther        bool     // store odex files that match PatternsOnSystemOther on the system_other partition
	PatternsOnSystemOther []string // patterns (using '%' to denote a prefix match) to put odex on the system_other partition

	DisableGenerateProfile bool   // don't generate profiles
	ProfileDir             string // directory to find profiles in

	BootJars          android.ConfiguredJarList // modules for jars that form the boot class path
	UpdatableBootJars android.ConfiguredJarList // jars within apex that form the boot class path

	ArtApexJars android.ConfiguredJarList // modules for jars that are in the ART APEX

	SystemServerJars          []string                  // jars that form the system server
	SystemServerApps          []string                  // apps that are loaded into system server
	UpdatableSystemServerJars android.ConfiguredJarList // jars within apex that are loaded into system server
	SpeedApps                 []string                  // apps that should be speed optimized

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
}

type ModuleConfig struct {
	Name            string
	DexLocation     string // dex location on device
	BuildPath       android.OutputPath
	DexPath         android.Path
	ManifestPath    android.Path
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

	Archs                   []android.ArchType
	DexPreoptImages         []android.Path
	DexPreoptImagesDeps     []android.OutputPaths
	DexPreoptImageLocations []string

	PreoptBootClassPathDexFiles     android.Paths // file paths of boot class path files
	PreoptBootClassPathDexLocations []string      // virtual locations of boot class path files

	PreoptExtractedApk bool // Overrides OnlyPreoptModules

	NoCreateAppImage    bool
	ForceCreateAppImage bool

	PresignedPrebuilt bool
}

type globalSoongConfigSingleton struct{}

var pctx = android.NewPackageContext("android/soong/dexpreopt")

func init() {
	pctx.Import("android/soong/android")
	android.RegisterSingletonType("dexpreopt-soong-config", func() android.Singleton {
		return &globalSoongConfigSingleton{}
	})
}

func constructPath(ctx android.PathContext, path string) android.Path {
	buildDirPrefix := ctx.Config().BuildDir() + "/"
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
	global *GlobalConfig
	data   []byte
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

func getGlobalConfigRaw(ctx android.PathContext) globalConfigAndRaw {
	return ctx.Config().Once(globalConfigOnceKey, func() interface{} {
		if data, err := ctx.Config().DexpreoptGlobalConfig(ctx); err != nil {
			panic(err)
		} else if data != nil {
			globalConfig, err := ParseGlobalConfig(ctx, data)
			if err != nil {
				panic(err)
			}
			return globalConfigAndRaw{globalConfig, data}
		}

		// No global config filename set, see if there is a test config set
		return ctx.Config().Once(testGlobalConfigOnceKey, func() interface{} {
			// Nope, return a config with preopting disabled
			return globalConfigAndRaw{&GlobalConfig{
				DisablePreopt:           true,
				DisablePreoptBootImages: true,
				DisableGenerateProfile:  true,
			}, nil}
		})
	}).(globalConfigAndRaw)
}

// SetTestGlobalConfig sets a GlobalConfig that future calls to GetGlobalConfig
// will return. It must be called before the first call to GetGlobalConfig for
// the config.
func SetTestGlobalConfig(config android.Config, globalConfig *GlobalConfig) {
	config.Once(testGlobalConfigOnceKey, func() interface{} { return globalConfigAndRaw{globalConfig, nil} })
}

// ParseModuleConfig parses a per-module dexpreopt.config file into a
// ModuleConfig struct. It is not used in Soong, which receives a ModuleConfig
// struct directly from java/dexpreopt.go. It is used in dexpreopt_gen called
// from Make to read the module dexpreopt.config written in the Make config
// stage.
func ParseModuleConfig(ctx android.PathContext, data []byte) (*ModuleConfig, error) {
	type ModuleJSONConfig struct {
		*ModuleConfig

		// Copies of entries in ModuleConfig that are not constructable without extra parameters.  They will be
		// used to construct the real value manually below.
		BuildPath                      string
		DexPath                        string
		ManifestPath                   string
		ProfileClassListing            string
		EnforceUsesLibrariesStatusFile string
		ClassLoaderContexts            jsonClassLoaderContextMap
		DexPreoptImages                []string
		DexPreoptImageLocations        []string
		PreoptBootClassPathDexFiles    []string
	}

	config := ModuleJSONConfig{}

	err := json.Unmarshal(data, &config)
	if err != nil {
		return config.ModuleConfig, err
	}

	// Construct paths that require a PathContext.
	config.ModuleConfig.BuildPath = constructPath(ctx, config.BuildPath).(android.OutputPath)
	config.ModuleConfig.DexPath = constructPath(ctx, config.DexPath)
	config.ModuleConfig.ManifestPath = constructPath(ctx, config.ManifestPath)
	config.ModuleConfig.ProfileClassListing = android.OptionalPathForPath(constructPath(ctx, config.ProfileClassListing))
	config.ModuleConfig.EnforceUsesLibrariesStatusFile = constructPath(ctx, config.EnforceUsesLibrariesStatusFile)
	config.ModuleConfig.ClassLoaderContexts = fromJsonClassLoaderContext(ctx, config.ClassLoaderContexts)
	config.ModuleConfig.DexPreoptImages = constructPaths(ctx, config.DexPreoptImages)
	config.ModuleConfig.DexPreoptImageLocations = config.DexPreoptImageLocations
	config.ModuleConfig.PreoptBootClassPathDexFiles = constructPaths(ctx, config.PreoptBootClassPathDexFiles)

	// This needs to exist, but dependencies are already handled in Make, so we don't need to pass them through JSON.
	config.ModuleConfig.DexPreoptImagesDeps = make([]android.OutputPaths, len(config.ModuleConfig.DexPreoptImages))

	return config.ModuleConfig, nil
}

// WriteSlimModuleConfigForMake serializes a subset of ModuleConfig into a per-module
// dexpreopt.config JSON file. It is a way to pass dexpreopt information about Soong modules to
// Make, which is needed when a Make module has a <uses-library> dependency on a Soong module.
func WriteSlimModuleConfigForMake(ctx android.ModuleContext, config *ModuleConfig, path android.WritablePath) {
	if path == nil {
		return
	}

	// JSON representation of the slim module dexpreopt.config.
	type slimModuleJSONConfig struct {
		Name                 string
		DexLocation          string
		BuildPath            string
		EnforceUsesLibraries bool
		ProvidesUsesLibrary  string
		ClassLoaderContexts  jsonClassLoaderContextMap
	}

	jsonConfig := &slimModuleJSONConfig{
		Name:                 config.Name,
		DexLocation:          config.DexLocation,
		BuildPath:            config.BuildPath.String(),
		EnforceUsesLibraries: config.EnforceUsesLibraries,
		ProvidesUsesLibrary:  config.ProvidesUsesLibrary,
		ClassLoaderContexts:  toJsonClassLoaderContext(config.ClassLoaderContexts),
	}

	data, err := json.MarshalIndent(jsonConfig, "", "    ")
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

var Dex2oatDepTag = struct {
	blueprint.BaseDependencyTag
}{}

// RegisterToolDeps adds the necessary dependencies to binary modules for tools
// that are required later when Get(Cached)GlobalSoongConfig is called. It
// should be called from a mutator that's registered with
// android.RegistrationContext.FinalDepsMutators.
func RegisterToolDeps(ctx android.BottomUpMutatorContext) {
	dex2oatBin := dex2oatModuleName(ctx.Config())
	v := ctx.Config().BuildOSTarget.Variations()
	ctx.AddFarVariationDependencies(v, Dex2oatDepTag, dex2oatBin)
}

func dex2oatPathFromDep(ctx android.ModuleContext) android.Path {
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
			if p, ok := child.(android.PrebuiltInterface); ok && p.Prebuilt() != nil {
				return false // If it's the prebuilt we're done.
			} else {
				return true // Recurse to check if the source has a prebuilt dependency.
			}
		}
		if parent == dex2oatModule && ctx.OtherModuleDependencyTag(child) == android.PrebuiltDepTag {
			if p, ok := child.(android.PrebuiltInterface); ok && p.Prebuilt() != nil && p.Prebuilt().UsePrebuilt() {
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
		Aapt:             ctx.Config().HostToolPath(ctx, "aapt"),
		SoongZip:         ctx.Config().HostToolPath(ctx, "soong_zip"),
		Zip2zip:          ctx.Config().HostToolPath(ctx, "zip2zip"),
		ManifestCheck:    ctx.Config().HostToolPath(ctx, "manifest_check"),
		ConstructContext: ctx.Config().HostToolPath(ctx, "construct_context"),
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
	}

	return config, nil
}

func (s *globalSoongConfigSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	if GetGlobalConfig(ctx).DisablePreopt {
		return
	}

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
	}, " "))
}

func GlobalConfigForTests(ctx android.PathContext) *GlobalConfig {
	return &GlobalConfig{
		DisablePreopt:                      false,
		DisablePreoptModules:               nil,
		OnlyPreoptBootImageAndSystemServer: false,
		HasSystemOther:                     false,
		PatternsOnSystemOther:              nil,
		DisableGenerateProfile:             false,
		ProfileDir:                         "",
		BootJars:                           android.EmptyConfiguredJarList(),
		UpdatableBootJars:                  android.EmptyConfiguredJarList(),
		ArtApexJars:                        android.EmptyConfiguredJarList(),
		SystemServerJars:                   nil,
		SystemServerApps:                   nil,
		UpdatableSystemServerJars:          android.EmptyConfiguredJarList(),
		SpeedApps:                          nil,
		PreoptFlags:                        nil,
		DefaultCompilerFilter:              "",
		SystemServerCompilerFilter:         "",
		GenerateDMFiles:                    false,
		NoDebugInfo:                        false,
		DontResolveStartupStrings:          false,
		AlwaysSystemServerDebugInfo:        false,
		NeverSystemServerDebugInfo:         false,
		AlwaysOtherDebugInfo:               false,
		NeverOtherDebugInfo:                false,
		IsEng:                              false,
		SanitizeLite:                       false,
		DefaultAppImages:                   false,
		Dex2oatXmx:                         "",
		Dex2oatXms:                         "",
		EmptyDirectory:                     "empty_dir",
		CpuVariant:                         nil,
		InstructionSetFeatures:             nil,
		BootImageProfiles:                  nil,
		BootFlags:                          "",
		Dex2oatImageXmx:                    "",
		Dex2oatImageXms:                    "",
	}
}

func globalSoongConfigForTests() *GlobalSoongConfig {
	return &GlobalSoongConfig{
		Profman:          android.PathForTesting("profman"),
		Dex2oat:          android.PathForTesting("dex2oat"),
		Aapt:             android.PathForTesting("aapt"),
		SoongZip:         android.PathForTesting("soong_zip"),
		Zip2zip:          android.PathForTesting("zip2zip"),
		ManifestCheck:    android.PathForTesting("manifest_check"),
		ConstructContext: android.PathForTesting("construct_context"),
	}
}
