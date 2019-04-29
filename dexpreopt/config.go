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
	"io/ioutil"
	"strings"

	"android/soong/android"
)

// GlobalConfig stores the configuration for dex preopting set by the product
type GlobalConfig struct {
	DefaultNoStripping bool // don't strip dex files by default

	DisablePreopt        bool     // disable preopt for all modules
	DisablePreoptModules []string // modules with preopt disabled by product-specific config

	OnlyPreoptBootImageAndSystemServer bool // only preopt jars in the boot image or system server

	GenerateApexImage bool // generate an extra boot image only containing jars from the runtime apex
	UseApexImage      bool // use the apex image by default

	HasSystemOther        bool     // store odex files that match PatternsOnSystemOther on the system_other partition
	PatternsOnSystemOther []string // patterns (using '%' to denote a prefix match) to put odex on the system_other partition

	DisableGenerateProfile bool   // don't generate profiles
	ProfileDir             string // directory to find profiles in

	BootJars []string // modules for jars that form the boot class path

	RuntimeApexJars               []string // modules for jars that are in the runtime apex
	ProductUpdatableBootModules   []string
	ProductUpdatableBootLocations []string

	SystemServerJars []string // jars that form the system server
	SystemServerApps []string // apps that are loaded into system server
	SpeedApps        []string // apps that should be speed optimized

	PreoptFlags []string // global dex2oat flags that should be used if no module-specific dex2oat flags are specified

	DefaultCompilerFilter      string // default compiler filter to pass to dex2oat, overridden by --compiler-filter= in module-specific dex2oat flags
	SystemServerCompilerFilter string // default compiler filter to pass to dex2oat for system server jars

	GenerateDMFiles     bool // generate Dex Metadata files
	NeverAllowStripping bool // whether stripping should not be done - used as build time check to make sure dex files are always available

	NoDebugInfo                 bool // don't generate debug info by default
	DontResolveStartupStrings   bool // don't resolve string literals loaded during application startup.
	AlwaysSystemServerDebugInfo bool // always generate mini debug info for system server modules (overrides NoDebugInfo=true)
	NeverSystemServerDebugInfo  bool // never generate mini debug info for system server modules (overrides NoDebugInfo=false)
	AlwaysOtherDebugInfo        bool // always generate mini debug info for non-system server modules (overrides NoDebugInfo=true)
	NeverOtherDebugInfo         bool // never generate mini debug info for non-system server modules (overrides NoDebugInfo=true)

	MissingUsesLibraries []string // libraries that may be listed in OptionalUsesLibraries but will not be installed by the product

	IsEng        bool // build is a eng variant
	SanitizeLite bool // build is the second phase of a SANITIZE_LITE build

	DefaultAppImages bool // build app images (TODO: .art files?) by default

	Dex2oatXmx string // max heap size for dex2oat
	Dex2oatXms string // initial heap size for dex2oat

	EmptyDirectory string // path to an empty directory

	CpuVariant             map[android.ArchType]string // cpu variant for each architecture
	InstructionSetFeatures map[android.ArchType]string // instruction set for each architecture

	// Only used for boot image
	DirtyImageObjects      android.OptionalPath // path to a dirty-image-objects file
	PreloadedClasses       android.OptionalPath // path to a preloaded-classes file
	BootImageProfiles      android.Paths        // path to a boot-image-profile.txt file
	UseProfileForBootImage bool                 // whether a profile should be used to compile the boot image
	BootFlags              string               // extra flags to pass to dex2oat for the boot image
	Dex2oatImageXmx        string               // max heap size for dex2oat for the boot image
	Dex2oatImageXms        string               // initial heap size for dex2oat for the boot image

	Tools Tools // paths to tools possibly used by the generated commands
}

// Tools contains paths to tools possibly used by the generated commands.  If you add a new tool here you MUST add it
// to the order-only dependency list in DEXPREOPT_GEN_DEPS.
type Tools struct {
	Profman  android.Path
	Dex2oat  android.Path
	Aapt     android.Path
	SoongZip android.Path
	Zip2zip  android.Path

	VerifyUsesLibraries android.Path
	ConstructContext    android.Path
}

type ModuleConfig struct {
	Name            string
	DexLocation     string // dex location on device
	BuildPath       android.OutputPath
	DexPath         android.Path
	UncompressedDex bool
	HasApkLibraries bool
	PreoptFlags     []string

	ProfileClassListing  android.OptionalPath
	ProfileIsTextListing bool

	EnforceUsesLibraries  bool
	OptionalUsesLibraries []string
	UsesLibraries         []string
	LibraryPaths          map[string]android.Path

	Archs           []android.ArchType
	DexPreoptImages []android.Path

	PreoptBootClassPathDexFiles     android.Paths // file paths of boot class path files
	PreoptBootClassPathDexLocations []string      // virtual locations of boot class path files

	PreoptExtractedApk bool // Overrides OnlyPreoptModules

	NoCreateAppImage    bool
	ForceCreateAppImage bool

	PresignedPrebuilt bool

	NoStripping     bool
	StripInputPath  android.Path
	StripOutputPath android.WritablePath
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

// LoadGlobalConfig reads the global dexpreopt.config file into a GlobalConfig struct.  It is used directly in Soong
// and in dexpreopt_gen called from Make to read the $OUT/dexpreopt.config written by Make.
func LoadGlobalConfig(ctx android.PathContext, path string) (GlobalConfig, error) {
	type GlobalJSONConfig struct {
		GlobalConfig

		// Copies of entries in GlobalConfig that are not constructable without extra parameters.  They will be
		// used to construct the real value manually below.
		DirtyImageObjects string
		PreloadedClasses  string
		BootImageProfiles []string

		Tools struct {
			Profman  string
			Dex2oat  string
			Aapt     string
			SoongZip string
			Zip2zip  string

			VerifyUsesLibraries string
			ConstructContext    string
		}
	}

	config := GlobalJSONConfig{}
	err := loadConfig(ctx, path, &config)
	if err != nil {
		return config.GlobalConfig, err
	}

	// Construct paths that require a PathContext.
	config.GlobalConfig.DirtyImageObjects = android.OptionalPathForPath(constructPath(ctx, config.DirtyImageObjects))
	config.GlobalConfig.PreloadedClasses = android.OptionalPathForPath(constructPath(ctx, config.PreloadedClasses))
	config.GlobalConfig.BootImageProfiles = constructPaths(ctx, config.BootImageProfiles)

	config.GlobalConfig.Tools.Profman = constructPath(ctx, config.Tools.Profman)
	config.GlobalConfig.Tools.Dex2oat = constructPath(ctx, config.Tools.Dex2oat)
	config.GlobalConfig.Tools.Aapt = constructPath(ctx, config.Tools.Aapt)
	config.GlobalConfig.Tools.SoongZip = constructPath(ctx, config.Tools.SoongZip)
	config.GlobalConfig.Tools.Zip2zip = constructPath(ctx, config.Tools.Zip2zip)
	config.GlobalConfig.Tools.VerifyUsesLibraries = constructPath(ctx, config.Tools.VerifyUsesLibraries)
	config.GlobalConfig.Tools.ConstructContext = constructPath(ctx, config.Tools.ConstructContext)

	return config.GlobalConfig, nil
}

// LoadModuleConfig reads a per-module dexpreopt.config file into a ModuleConfig struct.  It is not used in Soong, which
// receives a ModuleConfig struct directly from java/dexpreopt.go.  It is used in dexpreopt_gen called from oMake to
// read the module dexpreopt.config written by Make.
func LoadModuleConfig(ctx android.PathContext, path string) (ModuleConfig, error) {
	type ModuleJSONConfig struct {
		ModuleConfig

		// Copies of entries in ModuleConfig that are not constructable without extra parameters.  They will be
		// used to construct the real value manually below.
		BuildPath                   string
		DexPath                     string
		ProfileClassListing         string
		LibraryPaths                map[string]string
		DexPreoptImages             []string
		PreoptBootClassPathDexFiles []string
		StripInputPath              string
		StripOutputPath             string
	}

	config := ModuleJSONConfig{}

	err := loadConfig(ctx, path, &config)
	if err != nil {
		return config.ModuleConfig, err
	}

	// Construct paths that require a PathContext.
	config.ModuleConfig.BuildPath = constructPath(ctx, config.BuildPath).(android.OutputPath)
	config.ModuleConfig.DexPath = constructPath(ctx, config.DexPath)
	config.ModuleConfig.ProfileClassListing = android.OptionalPathForPath(constructPath(ctx, config.ProfileClassListing))
	config.ModuleConfig.LibraryPaths = constructPathMap(ctx, config.LibraryPaths)
	config.ModuleConfig.DexPreoptImages = constructPaths(ctx, config.DexPreoptImages)
	config.ModuleConfig.PreoptBootClassPathDexFiles = constructPaths(ctx, config.PreoptBootClassPathDexFiles)
	config.ModuleConfig.StripInputPath = constructPath(ctx, config.StripInputPath)
	config.ModuleConfig.StripOutputPath = constructWritablePath(ctx, config.StripOutputPath)

	return config.ModuleConfig, nil
}

func loadConfig(ctx android.PathContext, path string, config interface{}) error {
	r, err := ctx.Fs().Open(path)
	if err != nil {
		return err
	}
	defer r.Close()

	data, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, config)
	if err != nil {
		return err
	}

	return nil
}

func GlobalConfigForTests(ctx android.PathContext) GlobalConfig {
	return GlobalConfig{
		DefaultNoStripping:                 false,
		DisablePreopt:                      false,
		DisablePreoptModules:               nil,
		OnlyPreoptBootImageAndSystemServer: false,
		HasSystemOther:                     false,
		PatternsOnSystemOther:              nil,
		DisableGenerateProfile:             false,
		ProfileDir:                         "",
		BootJars:                           nil,
		RuntimeApexJars:                    nil,
		ProductUpdatableBootModules:        nil,
		ProductUpdatableBootLocations:      nil,
		SystemServerJars:                   nil,
		SystemServerApps:                   nil,
		SpeedApps:                          nil,
		PreoptFlags:                        nil,
		DefaultCompilerFilter:              "",
		SystemServerCompilerFilter:         "",
		GenerateDMFiles:                    false,
		NeverAllowStripping:                false,
		NoDebugInfo:                        false,
		DontResolveStartupStrings:          false,
		AlwaysSystemServerDebugInfo:        false,
		NeverSystemServerDebugInfo:         false,
		AlwaysOtherDebugInfo:               false,
		NeverOtherDebugInfo:                false,
		MissingUsesLibraries:               nil,
		IsEng:                              false,
		SanitizeLite:                       false,
		DefaultAppImages:                   false,
		Dex2oatXmx:                         "",
		Dex2oatXms:                         "",
		EmptyDirectory:                     "empty_dir",
		CpuVariant:                         nil,
		InstructionSetFeatures:             nil,
		DirtyImageObjects:                  android.OptionalPath{},
		PreloadedClasses:                   android.OptionalPath{},
		BootImageProfiles:                  nil,
		UseProfileForBootImage:             false,
		BootFlags:                          "",
		Dex2oatImageXmx:                    "",
		Dex2oatImageXms:                    "",
		Tools: Tools{
			Profman:             android.PathForTesting("profman"),
			Dex2oat:             android.PathForTesting("dex2oat"),
			Aapt:                android.PathForTesting("aapt"),
			SoongZip:            android.PathForTesting("soong_zip"),
			Zip2zip:             android.PathForTesting("zip2zip"),
			VerifyUsesLibraries: android.PathForTesting("verify_uses_libraries.sh"),
			ConstructContext:    android.PathForTesting("construct_context.sh"),
		},
	}
}
