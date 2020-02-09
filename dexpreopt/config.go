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
	"strings"

	"android/soong/android"
)

// GlobalConfig stores the configuration for dex preopting. The fields are set
// from product variables via dex_preopt_config.mk, except for SoongConfig
// which come from CreateGlobalSoongConfig.
type GlobalConfig struct {
	DisablePreopt        bool     // disable preopt for all modules
	DisablePreoptModules []string // modules with preopt disabled by product-specific config

	OnlyPreoptBootImageAndSystemServer bool // only preopt jars in the boot image or system server

	UseArtImage bool // use the art image (use other boot class path dex files without image)

	HasSystemOther        bool     // store odex files that match PatternsOnSystemOther on the system_other partition
	PatternsOnSystemOther []string // patterns (using '%' to denote a prefix match) to put odex on the system_other partition

	DisableGenerateProfile bool   // don't generate profiles
	ProfileDir             string // directory to find profiles in

	BootJars          []string // modules for jars that form the boot class path
	UpdatableBootJars []string // jars within apex that form the boot class path

	ArtApexJars []string // modules for jars that are in the ART APEX

	SystemServerJars          []string // jars that form the system server
	SystemServerApps          []string // apps that are loaded into system server
	UpdatableSystemServerJars []string // jars within apex that are loaded into system server
	SpeedApps                 []string // apps that should be speed optimized

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

	// Only used for boot image
	DirtyImageObjects android.OptionalPath // path to a dirty-image-objects file
	BootImageProfiles android.Paths        // path to a boot-image-profile.txt file
	BootFlags         string               // extra flags to pass to dex2oat for the boot image
	Dex2oatImageXmx   string               // max heap size for dex2oat for the boot image
	Dex2oatImageXms   string               // initial heap size for dex2oat for the boot image

	SoongConfig GlobalSoongConfig // settings read from dexpreopt_soong.config
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

	EnforceUsesLibraries         bool
	PresentOptionalUsesLibraries []string
	UsesLibraries                []string
	LibraryPaths                 map[string]android.Path

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

func constructPathMap(ctx android.PathContext, paths map[string]string) map[string]android.Path {
	ret := map[string]android.Path{}
	for key, path := range paths {
		ret[key] = constructPath(ctx, path)
	}
	return ret
}

func constructWritablePath(ctx android.PathContext, path string) android.WritablePath {
	if path == "" {
		return nil
	}
	return constructPath(ctx, path).(android.WritablePath)
}

// LoadGlobalConfig reads the global dexpreopt.config file into a GlobalConfig
// struct, except the SoongConfig field which is set from the provided
// soongConfig argument. LoadGlobalConfig is used directly in Soong and in
// dexpreopt_gen called from Make to read the $OUT/dexpreopt.config written by
// Make.
func LoadGlobalConfig(ctx android.PathContext, data []byte, soongConfig GlobalSoongConfig) (GlobalConfig, error) {
	type GlobalJSONConfig struct {
		GlobalConfig

		// Copies of entries in GlobalConfig that are not constructable without extra parameters.  They will be
		// used to construct the real value manually below.
		DirtyImageObjects string
		BootImageProfiles []string
	}

	config := GlobalJSONConfig{}
	err := json.Unmarshal(data, &config)
	if err != nil {
		return config.GlobalConfig, err
	}

	// Construct paths that require a PathContext.
	config.GlobalConfig.DirtyImageObjects = android.OptionalPathForPath(constructPath(ctx, config.DirtyImageObjects))
	config.GlobalConfig.BootImageProfiles = constructPaths(ctx, config.BootImageProfiles)

	// Set this here to force the caller to provide a value for this struct (from
	// either CreateGlobalSoongConfig or LoadGlobalSoongConfig).
	config.GlobalConfig.SoongConfig = soongConfig

	return config.GlobalConfig, nil
}

// LoadModuleConfig reads a per-module dexpreopt.config file into a ModuleConfig struct.  It is not used in Soong, which
// receives a ModuleConfig struct directly from java/dexpreopt.go.  It is used in dexpreopt_gen called from oMake to
// read the module dexpreopt.config written by Make.
func LoadModuleConfig(ctx android.PathContext, data []byte) (ModuleConfig, error) {
	type ModuleJSONConfig struct {
		ModuleConfig

		// Copies of entries in ModuleConfig that are not constructable without extra parameters.  They will be
		// used to construct the real value manually below.
		BuildPath                   string
		DexPath                     string
		ManifestPath                string
		ProfileClassListing         string
		LibraryPaths                map[string]string
		DexPreoptImages             []string
		DexPreoptImageLocations     []string
		PreoptBootClassPathDexFiles []string
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
	config.ModuleConfig.LibraryPaths = constructPathMap(ctx, config.LibraryPaths)
	config.ModuleConfig.DexPreoptImages = constructPaths(ctx, config.DexPreoptImages)
	config.ModuleConfig.DexPreoptImageLocations = config.DexPreoptImageLocations
	config.ModuleConfig.PreoptBootClassPathDexFiles = constructPaths(ctx, config.PreoptBootClassPathDexFiles)

	// This needs to exist, but dependencies are already handled in Make, so we don't need to pass them through JSON.
	config.ModuleConfig.DexPreoptImagesDeps = make([]android.OutputPaths, len(config.ModuleConfig.DexPreoptImages))

	return config.ModuleConfig, nil
}

// CreateGlobalSoongConfig creates a GlobalSoongConfig from the current context.
// Should not be used in dexpreopt_gen.
func CreateGlobalSoongConfig(ctx android.PathContext) GlobalSoongConfig {
	// Default to debug version to help find bugs.
	// Set USE_DEX2OAT_DEBUG to false for only building non-debug versions.
	var dex2oatBinary string
	if ctx.Config().Getenv("USE_DEX2OAT_DEBUG") == "false" {
		dex2oatBinary = "dex2oat"
	} else {
		dex2oatBinary = "dex2oatd"
	}

	return GlobalSoongConfig{
		Profman:          ctx.Config().HostToolPath(ctx, "profman"),
		Dex2oat:          ctx.Config().HostToolPath(ctx, dex2oatBinary),
		Aapt:             ctx.Config().HostToolPath(ctx, "aapt"),
		SoongZip:         ctx.Config().HostToolPath(ctx, "soong_zip"),
		Zip2zip:          ctx.Config().HostToolPath(ctx, "zip2zip"),
		ManifestCheck:    ctx.Config().HostToolPath(ctx, "manifest_check"),
		ConstructContext: android.PathForSource(ctx, "build/make/core/construct_context.sh"),
	}
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

// LoadGlobalSoongConfig reads the dexpreopt_soong.config file into a
// GlobalSoongConfig struct. It is only used in dexpreopt_gen.
func LoadGlobalSoongConfig(ctx android.PathContext, data []byte) (GlobalSoongConfig, error) {
	var jc globalJsonSoongConfig

	err := json.Unmarshal(data, &jc)
	if err != nil {
		return GlobalSoongConfig{}, err
	}

	config := GlobalSoongConfig{
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
	config := CreateGlobalSoongConfig(ctx)
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

	ctx.Build(pctx, android.BuildParams{
		Rule:   android.WriteFile,
		Output: android.PathForOutput(ctx, "dexpreopt_soong.config"),
		Args: map[string]string{
			"content": string(data),
		},
	})
}

func (s *globalSoongConfigSingleton) MakeVars(ctx android.MakeVarsContext) {
	config := CreateGlobalSoongConfig(ctx)

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

func GlobalConfigForTests(ctx android.PathContext) GlobalConfig {
	return GlobalConfig{
		DisablePreopt:                      false,
		DisablePreoptModules:               nil,
		OnlyPreoptBootImageAndSystemServer: false,
		HasSystemOther:                     false,
		PatternsOnSystemOther:              nil,
		DisableGenerateProfile:             false,
		ProfileDir:                         "",
		BootJars:                           nil,
		UpdatableBootJars:                  nil,
		ArtApexJars:                        nil,
		SystemServerJars:                   nil,
		SystemServerApps:                   nil,
		UpdatableSystemServerJars:          nil,
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
		DirtyImageObjects:                  android.OptionalPath{},
		BootImageProfiles:                  nil,
		BootFlags:                          "",
		Dex2oatImageXmx:                    "",
		Dex2oatImageXms:                    "",
		SoongConfig: GlobalSoongConfig{
			Profman:          android.PathForTesting("profman"),
			Dex2oat:          android.PathForTesting("dex2oat"),
			Aapt:             android.PathForTesting("aapt"),
			SoongZip:         android.PathForTesting("soong_zip"),
			Zip2zip:          android.PathForTesting("zip2zip"),
			ManifestCheck:    android.PathForTesting("manifest_check"),
			ConstructContext: android.PathForTesting("construct_context.sh"),
		},
	}
}
