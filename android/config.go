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

package android

// This is the primary location to write and read all configuration values and
// product variables necessary for soong_build's operation.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"android/soong/shared"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"
	"github.com/google/blueprint/pathtools"
	"github.com/google/blueprint/proptools"

	"android/soong/android/soongconfig"
	"android/soong/remoteexec"
)

// Bool re-exports proptools.Bool for the android package.
var Bool = proptools.Bool

// String re-exports proptools.String for the android package.
var String = proptools.String

// StringDefault re-exports proptools.StringDefault for the android package.
var StringDefault = proptools.StringDefault

// FutureApiLevelInt is a placeholder constant for unreleased API levels.
const FutureApiLevelInt = 10000

// PrivateApiLevel represents the api level of SdkSpecPrivate (sdk_version: "")
// This api_level exists to differentiate user-provided "" from "current" sdk_version
// The differentiation is necessary to enable different validation rules for these two possible values.
var PrivateApiLevel = ApiLevel{
	value:     "current",             // The value is current since aidl expects `current` as the default (TestAidlFlagsWithMinSdkVersion)
	number:    FutureApiLevelInt + 1, // This is used to differentiate it from FutureApiLevel
	isPreview: true,
}

// FutureApiLevel represents unreleased API levels.
var FutureApiLevel = ApiLevel{
	value:     "current",
	number:    FutureApiLevelInt,
	isPreview: true,
}

// The product variables file name, containing product config from Kati.
const productVariablesFileName = "soong.variables"

// A Config object represents the entire build configuration for Android.
type Config struct {
	*config
}

type SoongBuildMode int

type CmdArgs struct {
	bootstrap.Args
	RunGoTests     bool
	OutDir         string
	SoongOutDir    string
	SoongVariables string
	KatiSuffix     string

	ModuleGraphFile   string
	ModuleActionsFile string
	DocFile           string

	BuildFromSourceStub bool

	EnsureAllowlistIntegrity bool
}

// Build modes that soong_build can run as.
const (
	// Don't use bazel at all during module analysis.
	AnalysisNoBazel SoongBuildMode = iota

	// Create a JSON representation of the module graph and exit.
	GenerateModuleGraph

	// Generate a documentation file for module type definitions and exit.
	GenerateDocFile
)

const testKeyDir = "build/make/target/product/security"

func (c Config) genericConfig() Config {
	return Config{c.config.genericConfig}
}

// SoongOutDir returns the build output directory for the configuration.
func (c Config) SoongOutDir() string {
	return c.soongOutDir
}

// tempDir returns the path to out/soong/.temp, which is cleared at the beginning of every build.
func (c Config) tempDir() string {
	return shared.TempDirForOutDir(c.soongOutDir)
}

func (c Config) OutDir() string {
	return c.outDir
}

func (c Config) RunGoTests() bool {
	return c.runGoTests
}

func (c Config) DebugCompilation() bool {
	return false // Never compile Go code in the main build for debugging
}

func (c Config) Subninjas() []string {
	return []string{}
}

func (c Config) PrimaryBuilderInvocations() []bootstrap.PrimaryBuilderInvocation {
	return []bootstrap.PrimaryBuilderInvocation{}
}

// RunningInsideUnitTest returns true if this code is being run as part of a Soong unit test.
func (c Config) RunningInsideUnitTest() bool {
	return c.config.TestProductVariables != nil
}

// DisableHiddenApiChecks returns true if hiddenapi checks have been disabled.
// For 'eng' target variant hiddenapi checks are disabled by default for performance optimisation,
// but can be enabled by setting environment variable ENABLE_HIDDENAPI_FLAGS=true.
// For other target variants hiddenapi check are enabled by default but can be disabled by
// setting environment variable UNSAFE_DISABLE_HIDDENAPI_FLAGS=true.
// If both ENABLE_HIDDENAPI_FLAGS=true and UNSAFE_DISABLE_HIDDENAPI_FLAGS=true, then
// ENABLE_HIDDENAPI_FLAGS=true will be triggered and hiddenapi checks will be considered enabled.
func (c Config) DisableHiddenApiChecks() bool {
	return !c.IsEnvTrue("ENABLE_HIDDENAPI_FLAGS") &&
		(c.IsEnvTrue("UNSAFE_DISABLE_HIDDENAPI_FLAGS") ||
			Bool(c.productVariables.Eng))
}

// DisableVerifyOverlaps returns true if verify_overlaps is skipped.
// Mismatch in version of apexes and module SDK is required for mainline prebuilts to work in
// trunk stable.
// Thus, verify_overlaps is disabled when RELEASE_DEFAULT_MODULE_BUILD_FROM_SOURCE is set to false.
// TODO(b/308174018): Re-enable verify_overlaps for both builr from source/mainline prebuilts.
func (c Config) DisableVerifyOverlaps() bool {
	if c.IsEnvFalse("DISABLE_VERIFY_OVERLAPS") && c.ReleaseDisableVerifyOverlaps() {
		panic("The current release configuration does not support verify_overlaps. DISABLE_VERIFY_OVERLAPS cannot be set to false")
	}
	return c.IsEnvTrue("DISABLE_VERIFY_OVERLAPS") || c.ReleaseDisableVerifyOverlaps() || !c.ReleaseDefaultModuleBuildFromSource()
}

func (c Config) CoverageSuffix() string {
	if v := c.IsEnvTrue("EMMA_INSTRUMENT"); v {
		return "coverage."
	}
	return ""
}

// MaxPageSizeSupported returns the max page size supported by the device. This
// value will define the ELF segment alignment for binaries (executables and
// shared libraries).
func (c Config) MaxPageSizeSupported() string {
	return String(c.config.productVariables.DeviceMaxPageSizeSupported)
}

// NoBionicPageSizeMacro returns true when AOSP is page size agnostic.
// This means that the bionic's macro PAGE_SIZE won't be defined.
// Returns false when AOSP is NOT page size agnostic.
// This means that bionic's macro PAGE_SIZE is defined.
func (c Config) NoBionicPageSizeMacro() bool {
	return Bool(c.config.productVariables.DeviceNoBionicPageSizeMacro)
}

// The release version passed to aconfig, derived from RELEASE_VERSION
func (c Config) ReleaseVersion() string {
	return c.config.productVariables.ReleaseVersion
}

// The aconfig value set passed to aconfig, derived from RELEASE_VERSION
func (c Config) ReleaseAconfigValueSets() []string {
	return c.config.productVariables.ReleaseAconfigValueSets
}

// If native modules should have symbols stripped by default. Default false, enabled for build tools
func (c Config) StripByDefault() bool {
	return proptools.Bool(c.config.productVariables.StripByDefault)
}

func (c Config) ReleaseAconfigExtraReleaseConfigs() []string {
	result := []string{}
	if val, ok := c.config.productVariables.BuildFlags["RELEASE_ACONFIG_EXTRA_RELEASE_CONFIGS"]; ok {
		if len(val) > 0 {
			// Remove any duplicates from the list.
			found := make(map[string]bool)
			for _, k := range strings.Split(val, " ") {
				if !found[k] {
					found[k] = true
					result = append(result, k)
				}
			}
		}
	}
	return result
}

func (c Config) ReleaseAconfigExtraReleaseConfigsValueSets() map[string][]string {
	result := make(map[string][]string)
	for _, rcName := range c.ReleaseAconfigExtraReleaseConfigs() {
		if value, ok := c.config.productVariables.BuildFlags["RELEASE_ACONFIG_VALUE_SETS_"+rcName]; ok {
			result[rcName] = strings.Split(value, " ")
		}
	}
	return result
}

// The flag default permission value passed to aconfig
// derived from RELEASE_ACONFIG_FLAG_DEFAULT_PERMISSION
func (c Config) ReleaseAconfigFlagDefaultPermission() string {
	return c.config.productVariables.ReleaseAconfigFlagDefaultPermission
}

// The flag indicating behavior for the tree wrt building modules or using prebuilts
// derived from RELEASE_DEFAULT_MODULE_BUILD_FROM_SOURCE
func (c Config) ReleaseDefaultModuleBuildFromSource() bool {
	return c.config.productVariables.ReleaseDefaultModuleBuildFromSource == nil ||
		Bool(c.config.productVariables.ReleaseDefaultModuleBuildFromSource)
}

func (c Config) ReleaseDefaultUpdatableModuleVersion() string {
	if val, exists := c.GetBuildFlag("RELEASE_DEFAULT_UPDATABLE_MODULE_VERSION"); exists {
		return val
	}
	panic("RELEASE_DEFAULT_UPDATABLE_MODULE_VERSION is missing from build flags.")
}

func (c Config) ReleaseDisableVerifyOverlaps() bool {
	return c.config.productVariables.GetBuildFlagBool("RELEASE_DISABLE_VERIFY_OVERLAPS_CHECK")
}

// Enables flagged apis annotated with READ_WRITE aconfig flags to be included in the stubs
// and hiddenapi flags so that they are accessible at runtime
func (c Config) ReleaseExportRuntimeApis() bool {
	return Bool(c.config.productVariables.ExportRuntimeApis)
}

// Enables ABI monitoring of NDK libraries
func (c Config) ReleaseNdkAbiMonitored() bool {
	return c.config.productVariables.GetBuildFlagBool("RELEASE_NDK_ABI_MONITORED")
}

// Enable read flag from new storage, for C/C++
func (c Config) ReleaseReadFromNewStorageCc() bool {
	return c.config.productVariables.GetBuildFlagBool("RELEASE_READ_FROM_NEW_STORAGE_CC")
}

func (c Config) ReleaseHiddenApiExportableStubs() bool {
	return c.config.productVariables.GetBuildFlagBool("RELEASE_HIDDEN_API_EXPORTABLE_STUBS") ||
		Bool(c.config.productVariables.HiddenapiExportableStubs)
}

// Enable read flag from new storage
func (c Config) ReleaseReadFromNewStorage() bool {
	return c.config.productVariables.GetBuildFlagBool("RELEASE_READ_FROM_NEW_STORAGE")
}

func (c Config) ReleaseCreateAconfigStorageFile() bool {
	return c.config.productVariables.GetBuildFlagBool("RELEASE_CREATE_ACONFIG_STORAGE_FILE")
}

func (c Config) ReleaseUseSystemFeatureBuildFlags() bool {
	return c.config.productVariables.GetBuildFlagBool("RELEASE_USE_SYSTEM_FEATURE_BUILD_FLAGS")
}

func (c Config) ReleaseFingerprintAconfigPackages() bool {
	return c.config.productVariables.GetBuildFlagBool("RELEASE_FINGERPRINT_ACONFIG_PACKAGES")
}

func (c Config) ReleaseAconfigCheckApiLevel() bool {
	return c.config.productVariables.GetBuildFlagBool("RELEASE_ACONFIG_CHECK_API_LEVEL")
}

// A DeviceConfig object represents the configuration for a particular device
// being built. For now there will only be one of these, but in the future there
// may be multiple devices being built.
type DeviceConfig struct {
	*deviceConfig
}

// VendorConfig represents the configuration for vendor-specific behavior.
type VendorConfig soongconfig.SoongConfig

// Definition of general build configuration for soong_build. Some of these
// product configuration values are read from Kati-generated soong.variables.
type config struct {
	// Options configurable with soong.variables
	productVariables ProductVariables

	// Only available on configs created by TestConfig
	TestProductVariables *ProductVariables

	ProductVariablesFileName string

	// BuildOS stores the OsType for the OS that the build is running on.
	BuildOS OsType

	// BuildArch stores the ArchType for the CPU that the build is running on.
	BuildArch ArchType

	Targets                  map[OsType][]Target
	BuildOSTarget            Target // the Target for tools run on the build machine
	BuildOSCommonTarget      Target // the Target for common (java) tools run on the build machine
	AndroidCommonTarget      Target // the Target for common modules for the Android device
	AndroidFirstDeviceTarget Target // the first Target for modules for the Android device

	// Flags for Partial Compile, derived from SOONG_PARTIAL_COMPILE.
	partialCompileFlags partialCompileFlags

	// multilibConflicts for an ArchType is true if there is earlier configured
	// device architecture with the same multilib value.
	multilibConflicts map[ArchType]bool

	deviceConfig *deviceConfig

	outDir         string // The output directory (usually out/)
	soongOutDir    string
	moduleListFile string // the path to the file which lists blueprint files to parse.

	runGoTests bool

	env       map[string]string
	envLock   sync.Mutex
	envDeps   map[string]string
	envFrozen bool

	// Changes behavior based on whether Kati runs after soong_build, or if soong_build
	// runs standalone.
	katiEnabled bool
	katiSuffix  string

	captureBuild      bool // true for tests, saves build parameters for each module
	ignoreEnvironment bool // true for tests, returns empty from all Getenv calls

	fs         pathtools.FileSystem
	mockBpList string

	BuildMode SoongBuildMode

	// If testAllowNonExistentPaths is true then PathForSource and PathForModuleSrc won't error
	// in tests when a path doesn't exist.
	TestAllowNonExistentPaths bool

	// The list of files that when changed, must invalidate soong_build to
	// regenerate build.ninja.
	ninjaFileDepsSet sync.Map

	*OncePer

	// If buildFromSourceStub is true then the Java API stubs are
	// built from the source Java files, not the signature text files.
	buildFromSourceStub bool

	// If ensureAllowlistIntegrity is true, then the presence of any allowlisted
	// modules that aren't mixed-built for at least one variant will cause a build
	// failure
	ensureAllowlistIntegrity bool

	// If isGeneric is true, this config is the generic config.
	isGeneric bool

	// InstallPath requires the device name.
	// This is only for the installPath.
	deviceNameToInstall *string

	// Copy of this config struct but some product-specific variables are
	// replaced with the generic configuration values.
	genericConfig *config
}

type partialCompileFlags struct {
	// Whether to use d8 instead of r8
	Use_d8 bool

	// Whether to disable stub validation.  This is slightly more surgical
	// than DISABLE_STUB_VALIDATION, in that it only applies to partial
	// compile builds.
	Disable_stub_validation bool

	// Whether to disable api lint.
	Disable_api_lint bool

	// Add others as needed.
}

// These are the flags when `SOONG_PARTIAL_COMPILE` is empty or not set.
var defaultPartialCompileFlags = partialCompileFlags{}

// These are the flags when `SOONG_PARTIAL_COMPILE=true`.
var enabledPartialCompileFlags = partialCompileFlags{
	Use_d8:                  true,
	Disable_stub_validation: false,
	Disable_api_lint:        false,
}

type deviceConfig struct {
	config *config
	OncePer
}

type jsonConfigurable interface {
	SetDefaultConfig()
}

// Parse SOONG_PARTIAL_COMPILE.
//
// SOONG_PARTIAL_COMPILE determines which features are enabled or disabled in
// rule generation.  Changing this environment variable causes reanalysis.
//
// SOONG_USE_PARTIAL_COMPILE determines whether or not we **use** PARTIAL_COMPILE.
// Rule generation must support both cases, since changing it does not cause
// reanalysis.
//
// The user-facing documentation shows:
//
// - empty or not set: "The current default state"
// - "true" or "on": enable all stable partial compile features.
// - "false" or "off": disable partial compile completely.
//
// What we actually allow is a comma separated list of tokens, whose first
// character may be "+" (enable) or "-" (disable).  If neither is present, "+"
// is assumed.  For example, "on,+use_d8" will enable partial compilation, and
// additionally set the use_d8 flag (regardless of whether it is opt-in or
// opt-out).
//
// To add a new feature to the list, add the field in the struct
// `partialCompileFlags` above, and then add the name of the field in the
// switch statement below.
func (c *config) parsePartialCompileFlags(isEngBuild bool) (partialCompileFlags, error) {
	if !isEngBuild {
		return partialCompileFlags{}, nil
	}
	value := c.Getenv("SOONG_PARTIAL_COMPILE")
	if value == "" {
		return defaultPartialCompileFlags, nil
	}

	ret := defaultPartialCompileFlags
	tokens := strings.Split(strings.ToLower(value), ",")
	makeVal := func(state string, defaultValue bool) bool {
		switch state {
		case "":
			return defaultValue
		case "-":
			return false
		case "+":
			return true
		}
		return false
	}
	for _, tok := range tokens {
		var state string
		if len(tok) == 0 {
			continue
		}
		switch tok[0:1] {
		case "":
			// Ignore empty tokens.
			continue
		case "-", "+":
			state = tok[0:1]
			tok = tok[1:]
		default:
			// Treat `feature` as `+feature`.
			state = "+"
		}
		switch tok {
		case "all":
			// Turn on **all** of the flags.
			ret = partialCompileFlags{
				Use_d8:                  true,
				Disable_stub_validation: true,
				Disable_api_lint:        true,
			}
		case "true":
			ret = enabledPartialCompileFlags
		case "false":
			// Set everything to false.
			ret = partialCompileFlags{}

		case "api_lint", "enable_api_lint":
			ret.Disable_api_lint = !makeVal(state, !defaultPartialCompileFlags.Disable_api_lint)
		case "disable_api_lint":
			ret.Disable_api_lint = makeVal(state, defaultPartialCompileFlags.Disable_api_lint)

		case "stub_validation", "enable_stub_validation":
			ret.Disable_stub_validation = !makeVal(state, !defaultPartialCompileFlags.Disable_stub_validation)
		case "disable_stub_validation":
			ret.Disable_stub_validation = makeVal(state, defaultPartialCompileFlags.Disable_stub_validation)

		case "use_d8":
			ret.Use_d8 = makeVal(state, defaultPartialCompileFlags.Use_d8)
		default:
			return partialCompileFlags{}, fmt.Errorf("Unknown SOONG_PARTIAL_COMPILE value: %v", tok)
		}
	}
	return ret, nil
}

func loadConfig(config *config) error {
	return loadFromConfigFile(&config.productVariables, absolutePath(config.ProductVariablesFileName))
}

// Checks if the string is a valid go identifier. This is equivalent to blueprint's definition
// of an identifier, so it will match the same identifiers as those that can be used in bp files.
func isGoIdentifier(ident string) bool {
	for i, r := range ident {
		valid := r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) && i > 0
		if !valid {
			return false
		}
	}
	return len(ident) > 0
}

// loadFromConfigFile loads and decodes configuration options from a JSON file
// in the current working directory.
func loadFromConfigFile(configurable *ProductVariables, filename string) error {
	// Try to open the file
	configFileReader, err := os.Open(filename)
	defer configFileReader.Close()
	if os.IsNotExist(err) {
		// Need to create a file, so that blueprint & ninja don't get in
		// a dependency tracking loop.
		// Make a file-configurable-options with defaults, write it out using
		// a json writer.
		configurable.SetDefaultConfig()
		err = saveToConfigFile(configurable, filename)
		if err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("config file: could not open %s: %s", filename, err.Error())
	} else {
		// Make a decoder for it
		jsonDecoder := json.NewDecoder(configFileReader)
		jsonDecoder.DisallowUnknownFields()
		err = jsonDecoder.Decode(configurable)
		if err != nil {
			return fmt.Errorf("config file: %s did not parse correctly: %s", filename, err.Error())
		}
	}

	if Bool(configurable.GcovCoverage) && Bool(configurable.ClangCoverage) {
		return fmt.Errorf("GcovCoverage and ClangCoverage cannot both be set")
	}

	configurable.Native_coverage = proptools.BoolPtr(
		Bool(configurable.GcovCoverage) ||
			Bool(configurable.ClangCoverage))

	// The go scanner's definition of identifiers is c-style identifiers, but allowing unicode's
	// definition of letters and digits. This is the same scanner that blueprint uses, so it
	// will allow the same identifiers as are valid in bp files.
	for namespace := range configurable.VendorVars {
		if !isGoIdentifier(namespace) {
			return fmt.Errorf("soong config namespaces must be valid identifiers: %q", namespace)
		}
		for variable := range configurable.VendorVars[namespace] {
			if !isGoIdentifier(variable) {
				return fmt.Errorf("soong config variables must be valid identifiers: %q", variable)
			}
		}
	}

	// when Platform_sdk_final is true (or PLATFORM_VERSION_CODENAME is REL), use Platform_sdk_version;
	// if false (pre-released version, for example), use Platform_sdk_codename.
	if Bool(configurable.Platform_sdk_final) {
		if configurable.Platform_sdk_version != nil {
			configurable.Platform_sdk_version_or_codename =
				proptools.StringPtr(strconv.Itoa(*(configurable.Platform_sdk_version)))
		} else {
			return fmt.Errorf("Platform_sdk_version cannot be pointed by a NULL pointer")
		}
	} else {
		configurable.Platform_sdk_version_or_codename =
			proptools.StringPtr(String(configurable.Platform_sdk_codename))
	}

	return nil
}

// atomically writes the config file in case two copies of soong_build are running simultaneously
// (for example, docs generation and ninja manifest generation)
func saveToConfigFile(config *ProductVariables, filename string) error {
	data, err := json.MarshalIndent(&config, "", "    ")
	if err != nil {
		return fmt.Errorf("cannot marshal config data: %s", err.Error())
	}

	f, err := os.CreateTemp(filepath.Dir(filename), "config")
	if err != nil {
		return fmt.Errorf("cannot create empty config file %s: %s", filename, err.Error())
	}
	defer os.Remove(f.Name())
	defer f.Close()

	_, err = f.Write(data)
	if err != nil {
		return fmt.Errorf("default config file: %s could not be written: %s", filename, err.Error())
	}

	_, err = f.WriteString("\n")
	if err != nil {
		return fmt.Errorf("default config file: %s could not be written: %s", filename, err.Error())
	}

	f.Close()
	os.Rename(f.Name(), filename)

	return nil
}

// NullConfig returns a mostly empty Config for use by standalone tools like dexpreopt_gen that
// use the android package.
func NullConfig(outDir, soongOutDir string) Config {
	return Config{
		config: &config{
			outDir:      outDir,
			soongOutDir: soongOutDir,
			fs:          pathtools.OsFs,
		},
	}
}

func initConfig(cmdArgs CmdArgs, availableEnv map[string]string) (*config, error) {
	// Make a config with default options.
	newConfig := &config{
		ProductVariablesFileName: cmdArgs.SoongVariables,

		env: availableEnv,

		outDir:            cmdArgs.OutDir,
		soongOutDir:       cmdArgs.SoongOutDir,
		runGoTests:        cmdArgs.RunGoTests,
		katiSuffix:        cmdArgs.KatiSuffix,
		multilibConflicts: make(map[ArchType]bool),

		moduleListFile: cmdArgs.ModuleListFile,
		fs:             pathtools.NewOsFs(absSrcDir),

		OncePer: &OncePer{},

		buildFromSourceStub: cmdArgs.BuildFromSourceStub,
	}
	variant, ok := os.LookupEnv("TARGET_BUILD_VARIANT")
	isEngBuild := !ok || variant == "eng"

	newConfig.deviceConfig = &deviceConfig{
		config: newConfig,
	}

	// Soundness check of the build and source directories. This won't catch strange
	// configurations with symlinks, but at least checks the obvious case.
	absBuildDir, err := filepath.Abs(cmdArgs.SoongOutDir)
	if err != nil {
		return &config{}, err
	}

	absSrcDir, err := filepath.Abs(".")
	if err != nil {
		return &config{}, err
	}

	if strings.HasPrefix(absSrcDir, absBuildDir) {
		return &config{}, fmt.Errorf("Build dir must not contain source directory")
	}

	// Load any configurable options from the configuration file
	err = loadConfig(newConfig)
	if err != nil {
		return &config{}, err
	}

	KatiEnabledMarkerFile := filepath.Join(cmdArgs.SoongOutDir, ".soong.kati_enabled")
	if _, err := os.Stat(absolutePath(KatiEnabledMarkerFile)); err == nil {
		newConfig.katiEnabled = true
	}

	determineBuildOS(newConfig)

	// Sets up the map of target OSes to the finer grained compilation targets
	// that are configured from the product variables.
	targets, err := decodeTargetProductVariables(newConfig)
	if err != nil {
		return &config{}, err
	}

	newConfig.partialCompileFlags, err = newConfig.parsePartialCompileFlags(isEngBuild)
	if err != nil {
		return &config{}, err
	}

	// Make the CommonOS OsType available for all products.
	targets[CommonOS] = []Target{commonTargetMap[CommonOS.Name]}

	var archConfig []archConfig
	if newConfig.NdkAbis() {
		archConfig = getNdkAbisConfig()
	} else if newConfig.AmlAbis() {
		archConfig = getAmlAbisConfig()
	}

	if archConfig != nil {
		androidTargets, err := decodeAndroidArchSettings(archConfig)
		if err != nil {
			return &config{}, err
		}
		targets[Android] = androidTargets
	}

	multilib := make(map[string]bool)
	for _, target := range targets[Android] {
		if seen := multilib[target.Arch.ArchType.Multilib]; seen {
			newConfig.multilibConflicts[target.Arch.ArchType] = true
		}
		multilib[target.Arch.ArchType.Multilib] = true
	}

	// Map of OS to compilation targets.
	newConfig.Targets = targets

	// Compilation targets for host tools.
	newConfig.BuildOSTarget = newConfig.Targets[newConfig.BuildOS][0]
	newConfig.BuildOSCommonTarget = getCommonTargets(newConfig.Targets[newConfig.BuildOS])[0]

	// Compilation targets for Android.
	if len(newConfig.Targets[Android]) > 0 {
		newConfig.AndroidCommonTarget = getCommonTargets(newConfig.Targets[Android])[0]
		newConfig.AndroidFirstDeviceTarget = FirstTarget(newConfig.Targets[Android], "lib64", "lib32")[0]
	}

	setBuildMode := func(arg string, mode SoongBuildMode) {
		if arg != "" {
			if newConfig.BuildMode != AnalysisNoBazel {
				fmt.Fprintf(os.Stderr, "buildMode is already set, illegal argument: %s", arg)
				os.Exit(1)
			}
			newConfig.BuildMode = mode
		}
	}
	setBuildMode(cmdArgs.ModuleGraphFile, GenerateModuleGraph)
	setBuildMode(cmdArgs.DocFile, GenerateDocFile)

	newConfig.productVariables.Build_from_text_stub = boolPtr(newConfig.BuildFromTextStub())

	newConfig.deviceNameToInstall = newConfig.productVariables.DeviceName

	return newConfig, err
}

// Replace variables in config.productVariables that have tags with "generic" key.
// A generic tag may have a string or an int value for the generic configuration.
// If the value is "unset", generic configuration will unset the variable.
func overrideGenericConfig(config *config) {
	config.genericConfig.isGeneric = true
	type_pv := reflect.TypeOf(config.genericConfig.productVariables)
	value_pv := reflect.ValueOf(&config.genericConfig.productVariables)
	for i := range type_pv.NumField() {
		type_pv_field := type_pv.Field(i)
		generic_value := type_pv_field.Tag.Get("generic")
		// If a product variable has an annotation of "generic" tag, use the
		// value of the tag to set the generic variable.
		if generic_value != "" {
			value_pv_field := value_pv.Elem().Field(i)

			if generic_value == "unset" {
				// unset the product variable
				value_pv_field.SetZero()
				continue
			}

			kind_of_type_pv := type_pv_field.Type.Kind()
			is_pointer := false
			if kind_of_type_pv == reflect.Pointer {
				is_pointer = true
				kind_of_type_pv = type_pv_field.Type.Elem().Kind()
			}

			switch kind_of_type_pv {
			case reflect.String:
				if is_pointer {
					value_pv_field.Set(reflect.ValueOf(stringPtr(generic_value)))
				} else {
					value_pv_field.Set(reflect.ValueOf(generic_value))
				}
			case reflect.Int:
				generic_int, err := strconv.Atoi(generic_value)
				if err != nil {
					panic(fmt.Errorf("Only an int value can be assigned to int variable: %s", err))
				}
				if is_pointer {
					value_pv_field.Set(reflect.ValueOf(intPtr(generic_int)))
				} else {
					value_pv_field.Set(reflect.ValueOf(generic_int))
				}
			default:
				panic(fmt.Errorf("Unknown type to replace for generic variable: %s", &kind_of_type_pv))
			}
		}
	}

	// OncePer must be a singleton.
	config.genericConfig.OncePer = config.OncePer
	// keep the device name to get the install path.
	config.genericConfig.deviceNameToInstall = config.deviceNameToInstall
}

// NewConfig creates a new Config object. It also loads the config file, if
// found. The Config object includes a duplicated Config object in it for the
// generic configuration that does not provide any product specific information.
func NewConfig(cmdArgs CmdArgs, availableEnv map[string]string) (Config, error) {
	config, err := initConfig(cmdArgs, availableEnv)
	if err != nil {
		return Config{}, err
	}

	// Initialize generic configuration.
	config.genericConfig, err = initConfig(cmdArgs, availableEnv)
	// Update product specific variables with the generic configuration.
	overrideGenericConfig(config)

	return Config{config}, err
}

// mockFileSystem replaces all reads with accesses to the provided map of
// filenames to contents stored as a byte slice.
func (c *config) mockFileSystem(bp string, fs map[string][]byte) {
	mockFS := map[string][]byte{}

	if _, exists := mockFS["Android.bp"]; !exists {
		mockFS["Android.bp"] = []byte(bp)
	}

	for k, v := range fs {
		mockFS[k] = v
	}

	// no module list file specified; find every file named Blueprints or Android.bp
	pathsToParse := []string{}
	for candidate := range mockFS {
		base := filepath.Base(candidate)
		if base == "Android.bp" {
			pathsToParse = append(pathsToParse, candidate)
		}
	}
	if len(pathsToParse) < 1 {
		panic(fmt.Sprintf("No Blueprint or Android.bp files found in mock filesystem: %v\n", mockFS))
	}
	mockFS[blueprint.MockModuleListFile] = []byte(strings.Join(pathsToParse, "\n"))

	c.fs = pathtools.MockFs(mockFS)
	c.mockBpList = blueprint.MockModuleListFile
}

func (c *config) SetAllowMissingDependencies() {
	c.productVariables.Allow_missing_dependencies = proptools.BoolPtr(true)
}

// BlueprintToolLocation returns the directory containing build system tools
// from Blueprint, like soong_zip and merge_zips.
func (c *config) HostToolDir() string {
	return filepath.Join(c.outDir, "host", c.PrebuiltOS(), "bin")
}

func (c *config) HostToolPath(ctx PathContext, tool string) Path {
	path := pathForInstall(ctx, ctx.Config().BuildOS, ctx.Config().BuildArch, "bin", tool)
	return path
}

func (c *config) HostJNIToolPath(ctx PathContext, lib string) Path {
	ext := ".so"
	if runtime.GOOS == "darwin" {
		ext = ".dylib"
	}
	path := pathForInstall(ctx, ctx.Config().BuildOS, ctx.Config().BuildArch, "lib64", lib+ext)
	return path
}

func (c *config) HostJavaToolPath(ctx PathContext, tool string) Path {
	path := pathForInstall(ctx, ctx.Config().BuildOS, ctx.Config().BuildArch, "framework", tool)
	return path
}

func (c *config) HostCcSharedLibPath(ctx PathContext, lib string) Path {
	libDir := "lib"
	if ctx.Config().BuildArch.Multilib == "lib64" {
		libDir = "lib64"
	}
	return pathForInstall(ctx, ctx.Config().BuildOS, ctx.Config().BuildArch, libDir, lib+".so")
}

// PrebuiltOS returns the name of the host OS used in prebuilts directories.
func (c *config) PrebuiltOS() string {
	switch runtime.GOOS {
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			return "linux-x86"
		case "arm64":
			return "linux-arm64"
		default:
			panic(fmt.Errorf("Unknown GOARCH %s", runtime.GOARCH))
		}
	case "darwin":
		return "darwin-x86"
	default:
		panic(fmt.Errorf("Unknown GOOS %s", runtime.GOOS))
	}
}

// GoRoot returns the path to the root directory of the Go toolchain.
func (c *config) GoRoot() string {
	return fmt.Sprintf("prebuilts/go/%s", c.PrebuiltOS())
}

// PrebuiltBuildTool returns the path to a tool in the prebuilts directory containing
// checked-in tools, like Kati, Ninja or Toybox, for the current host OS.
func (c *config) PrebuiltBuildTool(ctx PathContext, tool string) Path {
	return PathForSource(ctx, "prebuilts/build-tools", c.PrebuiltOS(), "bin", tool)
}

// CpPreserveSymlinksFlags returns the host-specific flag for the cp(1) command
// to preserve symlinks.
func (c *config) CpPreserveSymlinksFlags() string {
	switch runtime.GOOS {
	case "darwin":
		return "-R"
	case "linux":
		return "-d"
	default:
		return ""
	}
}

func (c *config) Getenv(key string) string {
	var val string
	var exists bool
	c.envLock.Lock()
	defer c.envLock.Unlock()
	if c.envDeps == nil {
		c.envDeps = make(map[string]string)
	}
	if val, exists = c.envDeps[key]; !exists {
		if c.envFrozen {
			panic("Cannot access new environment variables after envdeps are frozen")
		}
		val, _ = c.env[key]
		c.envDeps[key] = val
	}
	return val
}

func (c *config) GetenvWithDefault(key string, defaultValue string) string {
	ret := c.Getenv(key)
	if ret == "" {
		return defaultValue
	}
	return ret
}

func (c *config) IsEnvTrue(key string) bool {
	value := strings.ToLower(c.Getenv(key))
	return value == "1" || value == "y" || value == "yes" || value == "on" || value == "true"
}

func (c *config) IsEnvFalse(key string) bool {
	value := strings.ToLower(c.Getenv(key))
	return value == "0" || value == "n" || value == "no" || value == "off" || value == "false"
}

func (c *config) TargetsJava21() bool {
	return c.productVariables.GetBuildFlagBool("RELEASE_TARGET_JAVA_21")
}

// EnvDeps returns the environment variables this build depends on. The first
// call to this function blocks future reads from the environment.
func (c *config) EnvDeps() map[string]string {
	c.envLock.Lock()
	defer c.envLock.Unlock()
	c.envFrozen = true
	return c.envDeps
}

func (c *config) KatiEnabled() bool {
	return c.katiEnabled
}

func (c *config) ProductVariables() ProductVariables {
	return c.productVariables
}

func (c *config) BuildId() string {
	return String(c.productVariables.BuildId)
}

func (c *config) DisplayBuildNumber() bool {
	return Bool(c.productVariables.DisplayBuildNumber)
}

// BuildFingerprintFile returns the path to a text file containing metadata
// representing the current build's fingerprint.
//
// Rules that want to reference the build fingerprint should read from this file
// without depending on it. They will run whenever their other dependencies
// require them to run and get the current build fingerprint. This ensures they
// don't rebuild on every incremental build when the build number changes.
func (c *config) BuildFingerprintFile(ctx PathContext) Path {
	return PathForArbitraryOutput(ctx, "target", "product", *c.deviceNameToInstall, String(c.productVariables.BuildFingerprintFile))
}

// BuildNumberFile returns the path to a text file containing metadata
// representing the current build's number.
//
// Rules that want to reference the build number should read from this file
// without depending on it. They will run whenever their other dependencies
// require them to run and get the current build number. This ensures they don't
// rebuild on every incremental build when the build number changes.
func (c *config) BuildNumberFile(ctx PathContext) Path {
	return PathForOutput(ctx, String(c.productVariables.BuildNumberFile))
}

// BuildHostnameFile returns the path to a text file containing metadata
// representing the current build's host name.
func (c *config) BuildHostnameFile(ctx PathContext) Path {
	return PathForOutput(ctx, String(c.productVariables.BuildHostnameFile))
}

// BuildThumbprintFile returns the path to a text file containing metadata
// representing the current build's thumbprint.
//
// Rules that want to reference the build thumbprint should read from this file
// without depending on it. They will run whenever their other dependencies
// require them to run and get the current build thumbprint. This ensures they
// don't rebuild on every incremental build when the build thumbprint changes.
func (c *config) BuildThumbprintFile(ctx PathContext) Path {
	return PathForArbitraryOutput(ctx, "target", "product", *c.deviceNameToInstall, String(c.productVariables.BuildThumbprintFile))
}

// DeviceName returns the name of the current device target.
// TODO: take an AndroidModuleContext to select the device name for multi-device builds
func (c *config) DeviceName() string {
	return *c.productVariables.DeviceName
}

// DeviceProduct returns the current product target. There could be multiple of
// these per device type.
//
// NOTE: Do not base conditional logic on this value. It may break product inheritance.
func (c *config) DeviceProduct() string {
	return *c.productVariables.DeviceProduct
}

// HasDeviceProduct returns if the build has a product. A build will not
// necessarily have a product when --skip-config is passed to soong, like it is
// in prebuilts/build-tools/build-prebuilts.sh
func (c *config) HasDeviceProduct() bool {
	return c.productVariables.DeviceProduct != nil
}

func (c *config) DeviceAbi() []string {
	return c.productVariables.DeviceAbi
}

func (c *config) DeviceResourceOverlays() []string {
	return c.productVariables.DeviceResourceOverlays
}

func (c *config) ProductResourceOverlays() []string {
	return c.productVariables.ProductResourceOverlays
}

func (c *config) PlatformDisplayVersionName() string {
	return String(c.productVariables.Platform_display_version_name)
}

func (c *config) PlatformVersionName() string {
	return String(c.productVariables.Platform_version_name)
}

func (c *config) PlatformSdkVersion() ApiLevel {
	return uncheckedFinalApiLevel(*c.productVariables.Platform_sdk_version)
}

func (c *config) PlatformSdkVersionFull() string {
	return proptools.StringDefault(c.productVariables.Platform_sdk_version_full, "")
}

func (c *config) RawPlatformSdkVersion() *int {
	return c.productVariables.Platform_sdk_version
}

func (c *config) PlatformSdkFinal() bool {
	return Bool(c.productVariables.Platform_sdk_final)
}

func (c *config) PlatformSdkCodename() string {
	return String(c.productVariables.Platform_sdk_codename)
}

func (c *config) PlatformSdkExtensionVersion() int {
	return *c.productVariables.Platform_sdk_extension_version
}

func (c *config) PlatformBaseSdkExtensionVersion() int {
	return *c.productVariables.Platform_base_sdk_extension_version
}

func (c *config) PlatformSecurityPatch() string {
	return String(c.productVariables.Platform_security_patch)
}

func (c *config) PlatformPreviewSdkVersion() string {
	return String(c.productVariables.Platform_preview_sdk_version)
}

func (c *config) PlatformMinSupportedTargetSdkVersion() string {
	var val, ok = c.productVariables.BuildFlags["RELEASE_PLATFORM_MIN_SUPPORTED_TARGET_SDK_VERSION"]
	if !ok {
		return ""
	}
	return val
}

func (c *config) PlatformBaseOS() string {
	return String(c.productVariables.Platform_base_os)
}

func (c *config) PlatformVersionLastStable() string {
	return String(c.productVariables.Platform_version_last_stable)
}

func (c *config) PlatformVersionKnownCodenames() string {
	return String(c.productVariables.Platform_version_known_codenames)
}

func (c *config) MinSupportedSdkVersion() ApiLevel {
	return uncheckedFinalApiLevel(21)
}

func (c *config) FinalApiLevels() []ApiLevel {
	var levels []ApiLevel
	for i := 1; i <= c.PlatformSdkVersion().FinalOrFutureInt(); i++ {
		levels = append(levels, uncheckedFinalApiLevel(i))
	}
	return levels
}

func (c *config) PreviewApiLevels() []ApiLevel {
	var levels []ApiLevel
	i := 0
	for _, codename := range c.PlatformVersionActiveCodenames() {
		if codename == "REL" {
			continue
		}

		levels = append(levels, ApiLevel{
			value:     codename,
			number:    i,
			isPreview: true,
		})
		i++
	}
	return levels
}

func (c *config) LatestPreviewApiLevel() ApiLevel {
	level := NoneApiLevel
	for _, l := range c.PreviewApiLevels() {
		if l.GreaterThan(level) {
			level = l
		}
	}
	return level
}

func (c *config) AllSupportedApiLevels() []ApiLevel {
	var levels []ApiLevel
	levels = append(levels, c.FinalApiLevels()...)
	return append(levels, c.PreviewApiLevels()...)
}

// DefaultAppTargetSdk returns the API level that platform apps are targeting.
// This converts a codename to the exact ApiLevel it represents.
func (c *config) DefaultAppTargetSdk(ctx EarlyModuleContext) ApiLevel {
	if Bool(c.productVariables.Platform_sdk_final) {
		return c.PlatformSdkVersion()
	}
	codename := c.PlatformSdkCodename()
	hostOnlyBuild := c.productVariables.DeviceArch == nil
	if codename == "" {
		// There are some host-only builds (those are invoked by build-prebuilts.sh) which
		// don't set platform sdk codename. Platform sdk codename makes sense only when we
		// are building the platform. So we don't enforce the below panic for the host-only
		// builds.
		if hostOnlyBuild {
			return NoneApiLevel
		}
		panic("Platform_sdk_codename must be set")
	}
	if codename == "REL" {
		panic("Platform_sdk_codename should not be REL when Platform_sdk_final is true")
	}
	return ApiLevelOrPanic(ctx, codename)
}

func (c *config) PartialCompileFlags() partialCompileFlags {
	return c.partialCompileFlags
}

func (c *config) AppsDefaultVersionName() string {
	return String(c.productVariables.AppsDefaultVersionName)
}

// Codenames that are active in the current lunch target.
func (c *config) PlatformVersionActiveCodenames() []string {
	return c.productVariables.Platform_version_active_codenames
}

// All unreleased codenames.
func (c *config) PlatformVersionAllPreviewCodenames() []string {
	return c.productVariables.Platform_version_all_preview_codenames
}

func (c *config) ProductAAPTConfig() []string {
	return c.productVariables.AAPTConfig
}

func (c *config) ProductAAPTPreferredConfig() string {
	return String(c.productVariables.AAPTPreferredConfig)
}

func (c *config) ProductAAPTCharacteristics() string {
	return String(c.productVariables.AAPTCharacteristics)
}

func (c *config) ProductAAPTPrebuiltDPI() []string {
	return c.productVariables.AAPTPrebuiltDPI
}

func (c *config) DefaultAppCertificateDir(ctx PathContext) SourcePath {
	defaultCert := String(c.productVariables.DefaultAppCertificate)
	if defaultCert != "" {
		return PathForSource(ctx, filepath.Dir(defaultCert))
	}
	return PathForSource(ctx, testKeyDir)
}

func (c *config) DefaultAppCertificate(ctx PathContext) (pem, key SourcePath) {
	defaultCert := String(c.productVariables.DefaultAppCertificate)
	if defaultCert != "" {
		return PathForSource(ctx, defaultCert+".x509.pem"), PathForSource(ctx, defaultCert+".pk8")
	}
	defaultDir := c.DefaultAppCertificateDir(ctx)
	return defaultDir.Join(ctx, "testkey.x509.pem"), defaultDir.Join(ctx, "testkey.pk8")
}

func (c *config) ExtraOtaKeys(ctx PathContext, recovery bool) []SourcePath {
	var otaKeys []string
	if recovery {
		otaKeys = c.productVariables.ExtraOtaRecoveryKeys
	} else {
		otaKeys = c.productVariables.ExtraOtaKeys
	}

	otaPaths := make([]SourcePath, len(otaKeys))
	for i, key := range otaKeys {
		otaPaths[i] = PathForSource(ctx, key+".x509.pem")
	}

	return otaPaths
}

func (c *config) ExtraOtaRecoveryKeys() []string {
	return c.productVariables.ExtraOtaRecoveryKeys
}

func (c *config) BuildKeys() string {
	defaultCert := String(c.productVariables.DefaultAppCertificate)
	if defaultCert == "" || defaultCert == filepath.Join(testKeyDir, "testkey") {
		return "test-keys"
	}
	return "dev-keys"
}

func (c *config) ApexKeyDir(ctx ModuleContext) SourcePath {
	// TODO(b/121224311): define another variable such as TARGET_APEX_KEY_OVERRIDE
	defaultCert := String(c.productVariables.DefaultAppCertificate)
	if defaultCert == "" || filepath.Dir(defaultCert) == testKeyDir {
		// When defaultCert is unset or is set to the testkeys path, use the APEX keys
		// that is under the module dir
		return pathForModuleSrc(ctx)
	}
	// If not, APEX keys are under the specified directory
	return PathForSource(ctx, filepath.Dir(defaultCert))
}

// Certificate for the NetworkStack sepolicy context
func (c *config) MainlineSepolicyDevCertificatesDir(ctx ModuleContext) SourcePath {
	cert := String(c.productVariables.MainlineSepolicyDevCertificates)
	if cert != "" {
		return PathForSource(ctx, cert)
	}
	return c.DefaultAppCertificateDir(ctx)
}

// AllowMissingDependencies configures Blueprint/Soong to not fail when modules
// are configured to depend on non-existent modules. Note that this does not
// affect missing input dependencies at the Ninja level.
func (c *config) AllowMissingDependencies() bool {
	return Bool(c.productVariables.Allow_missing_dependencies)
}

// Returns true if a full platform source tree cannot be assumed.
func (c *config) UnbundledBuild() bool {
	return Bool(c.productVariables.Unbundled_build)
}

// Returns true if building apps that aren't bundled with the platform.
// UnbundledBuild() is always true when this is true.
func (c *config) UnbundledBuildApps() bool {
	return len(c.productVariables.Unbundled_build_apps) > 0
}

// Returns true if building image that aren't bundled with the platform.
// UnbundledBuild() is always true when this is true.
func (c *config) UnbundledBuildImage() bool {
	return Bool(c.productVariables.Unbundled_build_image)
}

// Returns true if building modules against prebuilt SDKs.
func (c *config) AlwaysUsePrebuiltSdks() bool {
	return Bool(c.productVariables.Always_use_prebuilt_sdks)
}

func (c *config) MinimizeJavaDebugInfo() bool {
	return Bool(c.productVariables.MinimizeJavaDebugInfo) && !Bool(c.productVariables.Eng)
}

func (c *config) Debuggable() bool {
	return Bool(c.productVariables.Debuggable)
}

func (c *config) Eng() bool {
	return Bool(c.productVariables.Eng)
}

func (c *config) BuildType() string {
	return String(c.productVariables.BuildType)
}

// DevicePrimaryArchType returns the ArchType for the first configured device architecture, or
// Common if there are no device architectures.
func (c *config) DevicePrimaryArchType() ArchType {
	if androidTargets := c.Targets[Android]; len(androidTargets) > 0 {
		return androidTargets[0].Arch.ArchType
	}
	return Common
}

func (c *config) SanitizeHost() []string {
	return append([]string(nil), c.productVariables.SanitizeHost...)
}

func (c *config) SanitizeDevice() []string {
	return append([]string(nil), c.productVariables.SanitizeDevice...)
}

func (c *config) SanitizeDeviceDiag() []string {
	return append([]string(nil), c.productVariables.SanitizeDeviceDiag...)
}

func (c *config) SanitizeDeviceArch() []string {
	return append([]string(nil), c.productVariables.SanitizeDeviceArch...)
}

func (c *config) EnableCFI() bool {
	if c.productVariables.EnableCFI == nil {
		return true
	}
	return *c.productVariables.EnableCFI
}

func (c *config) DisableScudo() bool {
	return Bool(c.productVariables.DisableScudo)
}

func (c *config) Android64() bool {
	for _, t := range c.Targets[Android] {
		if t.Arch.ArchType.Multilib == "lib64" {
			return true
		}
	}

	return false
}

func (c *config) UseGoma() bool {
	return Bool(c.productVariables.UseGoma)
}

func (c *config) UseABFS() bool {
	return Bool(c.productVariables.UseABFS)
}

func (c *config) UseRBE() bool {
	return Bool(c.productVariables.UseRBE)
}

func (c *config) UseRBEJAVAC() bool {
	return Bool(c.productVariables.UseRBEJAVAC)
}

func (c *config) UseRBER8() bool {
	return Bool(c.productVariables.UseRBER8)
}

func (c *config) UseRBED8() bool {
	return Bool(c.productVariables.UseRBED8)
}

func (c *config) UseRemoteBuild() bool {
	return c.UseGoma() || c.UseRBE()
}

func (c *config) RunErrorProne() bool {
	return c.IsEnvTrue("RUN_ERROR_PRONE") || c.RunErrorProneInline()
}

// Returns if the errorprone build should be run "inline", that is, using errorprone as part
// of the main javac compilation instead of its own separate compilation. This is good for CI
// but bad for local development, because if you toggle errorprone+inline on/off it will repeatedly
// clobber java files from the old configuration.
func (c *config) RunErrorProneInline() bool {
	value := strings.ToLower(c.Getenv("RUN_ERROR_PRONE"))
	return c.IsEnvTrue("RUN_ERROR_PRONE_INLINE") || value == "inline"
}

// XrefCorpusName returns the Kythe cross-reference corpus name.
func (c *config) XrefCorpusName() string {
	return c.Getenv("XREF_CORPUS")
}

// XrefCuEncoding returns the compilation unit encoding to use for Kythe code
// xrefs. Can be 'json' (default), 'proto' or 'all'.
func (c *config) XrefCuEncoding() string {
	if enc := c.Getenv("KYTHE_KZIP_ENCODING"); enc != "" {
		return enc
	}
	return "json"
}

// XrefCuJavaSourceMax returns the maximum number of the Java source files
// in a single compilation unit
const xrefJavaSourceFileMaxDefault = "1000"

func (c Config) XrefCuJavaSourceMax() string {
	v := c.Getenv("KYTHE_JAVA_SOURCE_BATCH_SIZE")
	if v == "" {
		return xrefJavaSourceFileMaxDefault
	}
	if _, err := strconv.ParseUint(v, 0, 0); err != nil {
		fmt.Fprintf(os.Stderr,
			"bad KYTHE_JAVA_SOURCE_BATCH_SIZE value: %s, will use %s",
			err, xrefJavaSourceFileMaxDefault)
		return xrefJavaSourceFileMaxDefault
	}
	return v

}

func (c *config) EmitXrefRules() bool {
	return c.XrefCorpusName() != ""
}

func (c *config) ClangTidy() bool {
	return Bool(c.productVariables.ClangTidy)
}

func (c *config) TidyChecks() string {
	if c.productVariables.TidyChecks == nil {
		return ""
	}
	return *c.productVariables.TidyChecks
}

func (c *config) LibartImgHostBaseAddress() string {
	return "0x70000000"
}

func (c *config) LibartImgDeviceBaseAddress() string {
	return "0x70000000"
}

func (c *config) ArtUseReadBarrier() bool {
	return Bool(c.productVariables.ArtUseReadBarrier)
}

// Enforce Runtime Resource Overlays for a module. RROs supersede static RROs,
// but some modules still depend on it.
//
// More info: https://source.android.com/devices/architecture/rros
func (c *config) EnforceRROForModule(name string) bool {
	enforceList := c.productVariables.EnforceRROTargets

	if len(enforceList) > 0 {
		if InList("*", enforceList) {
			return true
		}
		return InList(name, enforceList)
	}
	return false
}

func (c *config) EnforceRROExcludedOverlay(path string) bool {
	excluded := c.productVariables.EnforceRROExcludedOverlays
	if len(excluded) > 0 {
		return HasAnyPrefix(path, excluded)
	}
	return false
}

func (c *config) EnforceRROGlobally() bool {
	enforceList := c.productVariables.EnforceRROTargets
	return InList("*", enforceList)
}

func (c *config) ExportedNamespaces() []string {
	return append([]string(nil), c.productVariables.NamespacesToExport...)
}

func (c *config) SourceRootDirs() []string {
	return c.productVariables.SourceRootDirs
}

func (c *config) HostStaticBinaries() bool {
	return Bool(c.productVariables.HostStaticBinaries)
}

func (c *config) UncompressPrivAppDex() bool {
	return Bool(c.productVariables.UncompressPrivAppDex)
}

func (c *config) ModulesLoadedByPrivilegedModules() []string {
	return c.productVariables.ModulesLoadedByPrivilegedModules
}

// DexpreoptGlobalConfigPath returns the path to the dexpreopt.config file in
// the output directory, if it was created during the product configuration
// phase by Kati.
func (c *config) DexpreoptGlobalConfigPath(ctx PathContext) OptionalPath {
	if c.productVariables.DexpreoptGlobalConfig == nil {
		return OptionalPathForPath(nil)
	}
	return OptionalPathForPath(
		pathForBuildToolDep(ctx, *c.productVariables.DexpreoptGlobalConfig))
}

// DexpreoptGlobalConfig returns the raw byte contents of the dexpreopt global
// configuration. Since the configuration file was created by Kati during
// product configuration (externally of soong_build), it's not tracked, so we
// also manually add a Ninja file dependency on the configuration file to the
// rule that creates the main build.ninja file. This ensures that build.ninja is
// regenerated correctly if dexpreopt.config changes.
func (c *config) DexpreoptGlobalConfig(ctx PathContext) ([]byte, error) {
	path := c.DexpreoptGlobalConfigPath(ctx)
	if !path.Valid() {
		return nil, nil
	}
	ctx.AddNinjaFileDeps(path.String())
	return os.ReadFile(absolutePath(path.String()))
}

func (c *deviceConfig) WithDexpreopt() bool {
	return c.config.productVariables.WithDexpreopt
}

func (c *config) FrameworksBaseDirExists(ctx PathGlobContext) bool {
	return ExistentPathForSource(ctx, "frameworks", "base", "Android.bp").Valid()
}

func (c *config) HasMultilibConflict(arch ArchType) bool {
	return c.multilibConflicts[arch]
}

func (c *config) PrebuiltHiddenApiDir(_ PathContext) string {
	return String(c.productVariables.PrebuiltHiddenApiDir)
}

func (c *config) VendorApiLevel() string {
	return String(c.productVariables.VendorApiLevel)
}

func (c *config) PrevVendorApiLevel() string {
	vendorApiLevel, err := strconv.Atoi(c.VendorApiLevel())
	if err != nil {
		panic(fmt.Errorf("Cannot parse vendor API level %s to an integer: %s",
			c.VendorApiLevel(), err))
	}
	// The version before trunk stable is 34.
	if vendorApiLevel == 202404 {
		return "34"
	}
	if vendorApiLevel >= 1 && vendorApiLevel <= 34 {
		return strconv.Itoa(vendorApiLevel - 1)
	}
	if vendorApiLevel < 202404 || vendorApiLevel%100 != 4 {
		panic("Unknown vendor API level " + c.VendorApiLevel())
	}
	return strconv.Itoa(vendorApiLevel - 100)
}

func IsTrunkStableVendorApiLevel(level string) bool {
	levelInt, err := strconv.Atoi(level)
	return err == nil && levelInt >= 202404
}

func (c *config) VendorApiLevelFrozen() bool {
	return c.productVariables.GetBuildFlagBool("RELEASE_BOARD_API_LEVEL_FROZEN")
}

func (c *config) katiPackageMkDir() string {
	return filepath.Join(c.soongOutDir, "kati_packaging"+c.katiSuffix)
}

func (c *deviceConfig) Arches() []Arch {
	var arches []Arch
	for _, target := range c.config.Targets[Android] {
		arches = append(arches, target.Arch)
	}
	return arches
}

func (c *deviceConfig) BinderBitness() string {
	is32BitBinder := c.config.productVariables.Binder32bit
	if is32BitBinder != nil && *is32BitBinder {
		return "32"
	}
	return "64"
}

func (c *deviceConfig) RecoveryPath() string {
	if c.config.productVariables.RecoveryPath != nil {
		return *c.config.productVariables.RecoveryPath
	}
	return "recovery"
}

func (c *deviceConfig) VendorPath() string {
	if c.config.productVariables.VendorPath != nil {
		return *c.config.productVariables.VendorPath
	}
	return "vendor"
}

func (c *deviceConfig) VendorDlkmPath() string {
	if c.config.productVariables.VendorDlkmPath != nil {
		return *c.config.productVariables.VendorDlkmPath
	}
	return "vendor_dlkm"
}

func (c *deviceConfig) BuildingVendorImage() bool {
	return proptools.Bool(c.config.productVariables.BuildingVendorImage)
}

func (c *deviceConfig) CurrentApiLevelForVendorModules() string {
	return StringDefault(c.config.productVariables.DeviceCurrentApiLevelForVendorModules, "current")
}

func (c *deviceConfig) ExtraVndkVersions() []string {
	return c.config.productVariables.ExtraVndkVersions
}

func (c *deviceConfig) SystemSdkVersions() []string {
	return c.config.productVariables.DeviceSystemSdkVersions
}

func (c *deviceConfig) PlatformSystemSdkVersions() []string {
	return c.config.productVariables.Platform_systemsdk_versions
}

func (c *deviceConfig) OdmPath() string {
	if c.config.productVariables.OdmPath != nil {
		return *c.config.productVariables.OdmPath
	}
	return "odm"
}

func (c *deviceConfig) BuildingOdmImage() bool {
	return proptools.Bool(c.config.productVariables.BuildingOdmImage)
}

func (c *deviceConfig) OdmDlkmPath() string {
	if c.config.productVariables.OdmDlkmPath != nil {
		return *c.config.productVariables.OdmDlkmPath
	}
	return "odm_dlkm"
}

func (c *deviceConfig) ProductPath() string {
	if c.config.productVariables.ProductPath != nil {
		return *c.config.productVariables.ProductPath
	}
	return "product"
}

func (c *deviceConfig) BuildingProductImage() bool {
	return proptools.Bool(c.config.productVariables.BuildingProductImage)
}

func (c *deviceConfig) SystemExtPath() string {
	if c.config.productVariables.SystemExtPath != nil {
		return *c.config.productVariables.SystemExtPath
	}
	return "system_ext"
}

func (c *deviceConfig) SystemDlkmPath() string {
	if c.config.productVariables.SystemDlkmPath != nil {
		return *c.config.productVariables.SystemDlkmPath
	}
	return "system_dlkm"
}

func (c *deviceConfig) OemPath() string {
	if c.config.productVariables.OemPath != nil {
		return *c.config.productVariables.OemPath
	}
	return "oem"
}

func (c *deviceConfig) UserdataPath() string {
	if c.config.productVariables.UserdataPath != nil {
		return *c.config.productVariables.UserdataPath
	}
	return "data"
}

func (c *deviceConfig) BuildingUserdataImage() bool {
	return proptools.Bool(c.config.productVariables.BuildingUserdataImage)
}

func (c *deviceConfig) BuildingRecoveryImage() bool {
	return proptools.Bool(c.config.productVariables.BuildingRecoveryImage)
}

func (c *deviceConfig) BtConfigIncludeDir() string {
	return String(c.config.productVariables.BtConfigIncludeDir)
}

func (c *deviceConfig) DeviceKernelHeaderDirs() []string {
	return c.config.productVariables.DeviceKernelHeaders
}

func (c *deviceConfig) TargetSpecificHeaderPath() string {
	return String(c.config.productVariables.TargetSpecificHeaderPath)
}

// JavaCoverageEnabledForPath returns whether Java code coverage is enabled for
// path. Coverage is enabled by default when the product variable
// JavaCoveragePaths is empty. If JavaCoveragePaths is not empty, coverage is
// enabled for any path which is part of this variable (and not part of the
// JavaCoverageExcludePaths product variable). Value "*" in JavaCoveragePaths
// represents any path.
func (c *deviceConfig) JavaCoverageEnabledForPath(path string) bool {
	coverage := false
	if len(c.config.productVariables.JavaCoveragePaths) == 0 ||
		InList("*", c.config.productVariables.JavaCoveragePaths) ||
		HasAnyPrefix(path, c.config.productVariables.JavaCoveragePaths) {
		coverage = true
	}
	if coverage && len(c.config.productVariables.JavaCoverageExcludePaths) > 0 {
		if HasAnyPrefix(path, c.config.productVariables.JavaCoverageExcludePaths) {
			coverage = false
		}
	}
	return coverage
}

// Returns true if gcov or clang coverage is enabled.
func (c *deviceConfig) NativeCoverageEnabled() bool {
	return Bool(c.config.productVariables.GcovCoverage) ||
		Bool(c.config.productVariables.ClangCoverage)
}

func (c *deviceConfig) ClangCoverageEnabled() bool {
	return Bool(c.config.productVariables.ClangCoverage)
}

func (c *deviceConfig) ClangCoverageContinuousMode() bool {
	return Bool(c.config.productVariables.ClangCoverageContinuousMode)
}

func (c *deviceConfig) GcovCoverageEnabled() bool {
	return Bool(c.config.productVariables.GcovCoverage)
}

// NativeCoverageEnabledForPath returns whether (GCOV- or Clang-based) native
// code coverage is enabled for path. By default, coverage is not enabled for a
// given path unless it is part of the NativeCoveragePaths product variable (and
// not part of the NativeCoverageExcludePaths product variable). Value "*" in
// NativeCoveragePaths represents any path.
func (c *deviceConfig) NativeCoverageEnabledForPath(path string) bool {
	coverage := false
	if len(c.config.productVariables.NativeCoveragePaths) > 0 {
		if InList("*", c.config.productVariables.NativeCoveragePaths) || HasAnyPrefix(path, c.config.productVariables.NativeCoveragePaths) {
			coverage = true
		}
	}
	if coverage && len(c.config.productVariables.NativeCoverageExcludePaths) > 0 {
		if HasAnyPrefix(path, c.config.productVariables.NativeCoverageExcludePaths) {
			coverage = false
		}
	}
	return coverage
}

func (c *deviceConfig) PgoAdditionalProfileDirs() []string {
	return c.config.productVariables.PgoAdditionalProfileDirs
}

// AfdoProfile returns fully qualified path associated to the given module name
func (c *deviceConfig) AfdoProfile(name string) (string, error) {
	for _, afdoProfile := range c.config.productVariables.AfdoProfiles {
		split := strings.Split(afdoProfile, ":")
		if len(split) != 3 {
			return "", fmt.Errorf("AFDO_PROFILES has invalid value: %s. "+
				"The expected format is <module>:<fully-qualified-path-to-fdo_profile>", afdoProfile)
		}
		if split[0] == name {
			return strings.Join([]string{split[1], split[2]}, ":"), nil
		}
	}
	return "", nil
}

func (c *deviceConfig) VendorSepolicyDirs() []string {
	return c.config.productVariables.BoardVendorSepolicyDirs
}

func (c *deviceConfig) OdmSepolicyDirs() []string {
	return c.config.productVariables.BoardOdmSepolicyDirs
}

func (c *deviceConfig) SystemExtPublicSepolicyDirs() []string {
	return c.config.productVariables.SystemExtPublicSepolicyDirs
}

func (c *deviceConfig) SystemExtPrivateSepolicyDirs() []string {
	return c.config.productVariables.SystemExtPrivateSepolicyDirs
}

func (c *deviceConfig) SepolicyM4Defs() []string {
	return c.config.productVariables.BoardSepolicyM4Defs
}

func (c *deviceConfig) OverrideManifestPackageNameFor(name string) (manifestName string, overridden bool) {
	return findOverrideValue(c.config.productVariables.ManifestPackageNameOverrides, name,
		"invalid override rule %q in PRODUCT_MANIFEST_PACKAGE_NAME_OVERRIDES should be <module_name>:<manifest_name>")
}

func (c *deviceConfig) OverrideCertificateFor(name string) (certificatePath string, overridden bool) {
	return findOverrideValue(c.config.productVariables.CertificateOverrides, name,
		"invalid override rule %q in PRODUCT_CERTIFICATE_OVERRIDES should be <module_name>:<certificate_module_name>")
}

func (c *deviceConfig) OverridePackageNameFor(name string) string {
	newName, overridden := findOverrideValue(
		c.config.productVariables.PackageNameOverrides,
		name,
		"invalid override rule %q in PRODUCT_PACKAGE_NAME_OVERRIDES should be <module_name>:<package_name>")
	if overridden {
		return newName
	}
	return name
}

func findOverrideValue(overrides []string, name string, errorMsg string) (newValue string, overridden bool) {
	if overrides == nil || len(overrides) == 0 {
		return "", false
	}
	for _, o := range overrides {
		split := strings.Split(o, ":")
		if len(split) != 2 {
			// This shouldn't happen as this is first checked in make, but just in case.
			panic(fmt.Errorf(errorMsg, o))
		}
		if matchPattern(split[0], name) {
			return substPattern(split[0], split[1], name), true
		}
	}
	return "", false
}

func (c *deviceConfig) ApexGlobalMinSdkVersionOverride() string {
	return String(c.config.productVariables.ApexGlobalMinSdkVersionOverride)
}

func (c *config) IntegerOverflowDisabledForPath(path string) bool {
	if len(c.productVariables.IntegerOverflowExcludePaths) == 0 {
		return false
	}
	return HasAnyPrefix(path, c.productVariables.IntegerOverflowExcludePaths)
}

func (c *config) CFIDisabledForPath(path string) bool {
	if len(c.productVariables.CFIExcludePaths) == 0 {
		return false
	}
	return HasAnyPrefix(path, c.productVariables.CFIExcludePaths)
}

func (c *config) CFIEnabledForPath(path string) bool {
	if len(c.productVariables.CFIIncludePaths) == 0 {
		return false
	}
	return HasAnyPrefix(path, c.productVariables.CFIIncludePaths) && !c.CFIDisabledForPath(path)
}

func (c *config) MemtagHeapDisabledForPath(path string) bool {
	if len(c.productVariables.MemtagHeapExcludePaths) == 0 {
		return false
	}
	return HasAnyPrefix(path, c.productVariables.MemtagHeapExcludePaths)
}

func (c *config) MemtagHeapAsyncEnabledForPath(path string) bool {
	if len(c.productVariables.MemtagHeapAsyncIncludePaths) == 0 {
		return false
	}
	return HasAnyPrefix(path, c.productVariables.MemtagHeapAsyncIncludePaths) && !c.MemtagHeapDisabledForPath(path)
}

func (c *config) MemtagHeapSyncEnabledForPath(path string) bool {
	if len(c.productVariables.MemtagHeapSyncIncludePaths) == 0 {
		return false
	}
	return HasAnyPrefix(path, c.productVariables.MemtagHeapSyncIncludePaths) && !c.MemtagHeapDisabledForPath(path)
}

func (c *config) HWASanDisabledForPath(path string) bool {
	if len(c.productVariables.HWASanExcludePaths) == 0 {
		return false
	}
	return HasAnyPrefix(path, c.productVariables.HWASanExcludePaths)
}

func (c *config) HWASanEnabledForPath(path string) bool {
	if len(c.productVariables.HWASanIncludePaths) == 0 {
		return false
	}
	return HasAnyPrefix(path, c.productVariables.HWASanIncludePaths) && !c.HWASanDisabledForPath(path)
}

func (c *config) VendorConfig(name string) VendorConfig {
	return soongconfig.Config(c.productVariables.VendorVars[name])
}

func (c *config) NdkAbis() bool {
	return Bool(c.productVariables.Ndk_abis)
}

func (c *config) AmlAbis() bool {
	return Bool(c.productVariables.Aml_abis)
}

func (c *config) ForceApexSymlinkOptimization() bool {
	return Bool(c.productVariables.ForceApexSymlinkOptimization)
}

func (c *config) ApexCompressionEnabled() bool {
	return Bool(c.productVariables.CompressedApex) && !c.UnbundledBuildApps()
}

func (c *config) DefaultApexPayloadType() string {
	return StringDefault(c.productVariables.DefaultApexPayloadType, "ext4")
}

func (c *config) UseSoongSystemImage() bool {
	return Bool(c.productVariables.UseSoongSystemImage)
}

func (c *config) SoongDefinedSystemImage() string {
	if c.UseSoongSystemImage() {
		return String(c.productVariables.ProductSoongDefinedSystemImage)
	}
	return ""
}

func (c *config) EnforceSystemCertificate() bool {
	return Bool(c.productVariables.EnforceSystemCertificate)
}

func (c *config) EnforceSystemCertificateAllowList() []string {
	return c.productVariables.EnforceSystemCertificateAllowList
}

func (c *config) EnforceProductPartitionInterface() bool {
	return Bool(c.productVariables.EnforceProductPartitionInterface)
}

func (c *config) ProductHiddenAPIStubs() []string {
	return c.productVariables.ProductHiddenAPIStubs
}

func (c *config) ProductHiddenAPIStubsSystem() []string {
	return c.productVariables.ProductHiddenAPIStubsSystem
}

func (c *config) ProductHiddenAPIStubsTest() []string {
	return c.productVariables.ProductHiddenAPIStubsTest
}

func (c *deviceConfig) TargetFSConfigGen() []string {
	return c.config.productVariables.TargetFSConfigGen
}

func (c *config) ProductPublicSepolicyDirs() []string {
	return c.productVariables.ProductPublicSepolicyDirs
}

func (c *config) ProductPrivateSepolicyDirs() []string {
	return c.productVariables.ProductPrivateSepolicyDirs
}

func (c *config) TargetMultitreeUpdateMeta() bool {
	return c.productVariables.MultitreeUpdateMeta
}

func (c *deviceConfig) DeviceArch() string {
	return String(c.config.productVariables.DeviceArch)
}

func (c *deviceConfig) DeviceArchVariant() string {
	return String(c.config.productVariables.DeviceArchVariant)
}

func (c *deviceConfig) DeviceSecondaryArch() string {
	return String(c.config.productVariables.DeviceSecondaryArch)
}

func (c *deviceConfig) DeviceSecondaryArchVariant() string {
	return String(c.config.productVariables.DeviceSecondaryArchVariant)
}

func (c *deviceConfig) BoardUsesRecoveryAsBoot() bool {
	return Bool(c.config.productVariables.BoardUsesRecoveryAsBoot)
}

func (c *deviceConfig) BoardKernelBinaries() []string {
	return c.config.productVariables.BoardKernelBinaries
}

func (c *deviceConfig) BoardKernelModuleInterfaceVersions() []string {
	return c.config.productVariables.BoardKernelModuleInterfaceVersions
}

func (c *deviceConfig) BoardMoveRecoveryResourcesToVendorBoot() bool {
	return Bool(c.config.productVariables.BoardMoveRecoveryResourcesToVendorBoot)
}

func (c *deviceConfig) PlatformSepolicyVersion() string {
	return String(c.config.productVariables.PlatformSepolicyVersion)
}

func (c *deviceConfig) PlatformSepolicyCompatVersions() []string {
	return c.config.productVariables.PlatformSepolicyCompatVersions
}

func (c *deviceConfig) BoardSepolicyVers() string {
	if ver := String(c.config.productVariables.BoardSepolicyVers); ver != "" {
		return ver
	}
	return c.PlatformSepolicyVersion()
}

func (c *deviceConfig) SystemExtSepolicyPrebuiltApiDir() string {
	return String(c.config.productVariables.SystemExtSepolicyPrebuiltApiDir)
}

func (c *deviceConfig) ProductSepolicyPrebuiltApiDir() string {
	return String(c.config.productVariables.ProductSepolicyPrebuiltApiDir)
}

func (c *deviceConfig) IsPartnerTrebleSepolicyTestEnabled() bool {
	return c.SystemExtSepolicyPrebuiltApiDir() != "" || c.ProductSepolicyPrebuiltApiDir() != ""
}

func createDirsMap(previous map[string]bool, dirs []string) (map[string]bool, error) {
	var ret = make(map[string]bool)
	for _, dir := range dirs {
		clean := filepath.Clean(dir)
		if previous[clean] || ret[clean] {
			return nil, fmt.Errorf("Duplicate entry %s", dir)
		}
		ret[clean] = true
	}
	return ret, nil
}

func (c *deviceConfig) createDirsMapOnce(onceKey OnceKey, previous map[string]bool, dirs []string) map[string]bool {
	dirMap := c.Once(onceKey, func() interface{} {
		ret, err := createDirsMap(previous, dirs)
		if err != nil {
			panic(fmt.Errorf("%s: %w", onceKey.key, err))
		}
		return ret
	})
	if dirMap == nil {
		return nil
	}
	return dirMap.(map[string]bool)
}

func (c *deviceConfig) ShippingApiLevel() ApiLevel {
	if c.config.productVariables.Shipping_api_level == nil {
		return NoneApiLevel
	}
	apiLevel, _ := strconv.Atoi(*c.config.productVariables.Shipping_api_level)
	return uncheckedFinalApiLevel(apiLevel)
}

func (c *deviceConfig) BuildBrokenPluginValidation() []string {
	return c.config.productVariables.BuildBrokenPluginValidation
}

func (c *deviceConfig) BuildBrokenClangAsFlags() bool {
	return c.config.productVariables.BuildBrokenClangAsFlags
}

func (c *deviceConfig) BuildBrokenClangCFlags() bool {
	return c.config.productVariables.BuildBrokenClangCFlags
}

func (c *deviceConfig) BuildBrokenClangProperty() bool {
	return c.config.productVariables.BuildBrokenClangProperty
}

func (c *deviceConfig) BuildBrokenEnforceSyspropOwner() bool {
	return c.config.productVariables.BuildBrokenEnforceSyspropOwner
}

func (c *deviceConfig) BuildBrokenTrebleSyspropNeverallow() bool {
	return c.config.productVariables.BuildBrokenTrebleSyspropNeverallow
}

func (c *deviceConfig) BuildDebugfsRestrictionsEnabled() bool {
	return c.config.productVariables.BuildDebugfsRestrictionsEnabled
}

func (c *deviceConfig) BuildBrokenVendorPropertyNamespace() bool {
	return c.config.productVariables.BuildBrokenVendorPropertyNamespace
}

func (c *deviceConfig) BuildBrokenInputDir(name string) bool {
	return InList(name, c.config.productVariables.BuildBrokenInputDirModules)
}

func (c *deviceConfig) BuildBrokenDontCheckSystemSdk() bool {
	return c.config.productVariables.BuildBrokenDontCheckSystemSdk
}

func (c *deviceConfig) BuildBrokenDupSysprop() bool {
	return c.config.productVariables.BuildBrokenDupSysprop
}

func (c *config) BuildWarningBadOptionalUsesLibsAllowlist() []string {
	return c.productVariables.BuildWarningBadOptionalUsesLibsAllowlist
}

func (c *deviceConfig) GenruleSandboxing() bool {
	return Bool(c.config.productVariables.GenruleSandboxing)
}

func (c *deviceConfig) RequiresInsecureExecmemForSwiftshader() bool {
	return c.config.productVariables.RequiresInsecureExecmemForSwiftshader
}

func (c *deviceConfig) Release_aidl_use_unfrozen() bool {
	return Bool(c.config.productVariables.Release_aidl_use_unfrozen)
}

func (c *config) SelinuxIgnoreNeverallows() bool {
	return c.productVariables.SelinuxIgnoreNeverallows
}

func (c *deviceConfig) SepolicyFreezeTestExtraDirs() []string {
	return c.config.productVariables.SepolicyFreezeTestExtraDirs
}

func (c *deviceConfig) SepolicyFreezeTestExtraPrebuiltDirs() []string {
	return c.config.productVariables.SepolicyFreezeTestExtraPrebuiltDirs
}

func (c *deviceConfig) GenerateAidlNdkPlatformBackend() bool {
	return c.config.productVariables.GenerateAidlNdkPlatformBackend
}

func (c *deviceConfig) AconfigContainerValidation() string {
	return c.config.productVariables.AconfigContainerValidation
}

func (c *config) IgnorePrefer32OnDevice() bool {
	return c.productVariables.IgnorePrefer32OnDevice
}

func (c *config) BootJars() []string {
	return c.Once(earlyBootJarsKey, func() interface{} {
		list := c.productVariables.BootJars.CopyOfJars()
		return append(list, c.productVariables.ApexBootJars.CopyOfJars()...)
	}).([]string)
}

func (c *config) NonApexBootJars() ConfiguredJarList {
	return c.productVariables.BootJars
}

func (c *config) ApexBootJars() ConfiguredJarList {
	return c.productVariables.ApexBootJars
}

func (c *config) RBEWrapper() string {
	return c.GetenvWithDefault("RBE_WRAPPER", remoteexec.DefaultWrapperPath)
}

// UseHostMusl returns true if the host target has been configured to build against musl libc.
func (c *config) UseHostMusl() bool {
	return Bool(c.productVariables.HostMusl)
}

// ApiSurfaces directory returns the source path inside the api_surfaces repo
// (relative to workspace root).
func (c *config) ApiSurfacesDir(s ApiSurface, version string) string {
	return filepath.Join(
		"build",
		"bazel",
		"api_surfaces",
		s.String(),
		version)
}

func (c *config) JavaCoverageEnabled() bool {
	return c.IsEnvTrue("EMMA_INSTRUMENT") || c.IsEnvTrue("EMMA_INSTRUMENT_STATIC") || c.IsEnvTrue("EMMA_INSTRUMENT_FRAMEWORK")
}

func (c *deviceConfig) BuildFromSourceStub() bool {
	return Bool(c.config.productVariables.BuildFromSourceStub)
}

func (c *config) BuildFromTextStub() bool {
	// TODO: b/302320354 - Remove the coverage build specific logic once the
	// robust solution for handling native properties in from-text stub build
	// is implemented.
	return !c.buildFromSourceStub &&
		!c.JavaCoverageEnabled() &&
		!c.deviceConfig.BuildFromSourceStub()
}

func (c *config) SetBuildFromTextStub(b bool) {
	c.buildFromSourceStub = !b
	c.productVariables.Build_from_text_stub = boolPtr(b)
}

func (c *deviceConfig) CheckVendorSeappViolations() bool {
	return Bool(c.config.productVariables.CheckVendorSeappViolations)
}

func (c *config) GetBuildFlag(name string) (string, bool) {
	val, ok := c.productVariables.BuildFlags[name]
	return val, ok
}

func (c *config) UseOptimizedResourceShrinkingByDefault() bool {
	return c.productVariables.GetBuildFlagBool("RELEASE_USE_OPTIMIZED_RESOURCE_SHRINKING_BY_DEFAULT")
}

func (c *config) UseR8FullModeByDefault() bool {
	return c.productVariables.GetBuildFlagBool("RELEASE_R8_FULL_MODE_BY_DEFAULT")
}

func (c *config) UseR8OnlyRuntimeVisibleAnnotations() bool {
	return c.productVariables.GetBuildFlagBool("RELEASE_R8_ONLY_RUNTIME_VISIBLE_ANNOTATIONS")
}

func (c *config) UseR8StoreStoreFenceConstructorInlining() bool {
	return c.productVariables.GetBuildFlagBool("RELEASE_R8_STORE_STORE_FENCE_CONSTRUCTOR_INLINING")
}

func (c *config) UseR8GlobalCheckNotNullFlags() bool {
	return c.productVariables.GetBuildFlagBool("RELEASE_R8_GLOBAL_CHECK_NOT_NULL_FLAGS")
}

func (c *config) UseDexV41() bool {
	return c.productVariables.GetBuildFlagBool("RELEASE_USE_DEX_V41")
}

var (
	mainlineApexContributionBuildFlagsToApexNames = map[string]string{
		"RELEASE_APEX_CONTRIBUTIONS_ADBD":                    "com.android.adbd",
		"RELEASE_APEX_CONTRIBUTIONS_ADSERVICES":              "com.android.adservices",
		"RELEASE_APEX_CONTRIBUTIONS_APPSEARCH":               "com.android.appsearch",
		"RELEASE_APEX_CONTRIBUTIONS_ART":                     "com.android.art",
		"RELEASE_APEX_CONTRIBUTIONS_BLUETOOTH":               "com.android.bt",
		"RELEASE_APEX_CONTRIBUTIONS_CAPTIVEPORTALLOGIN":      "",
		"RELEASE_APEX_CONTRIBUTIONS_CELLBROADCAST":           "com.android.cellbroadcast",
		"RELEASE_APEX_CONTRIBUTIONS_CONFIGINFRASTRUCTURE":    "com.android.configinfrastructure",
		"RELEASE_APEX_CONTRIBUTIONS_CONNECTIVITY":            "com.android.tethering",
		"RELEASE_APEX_CONTRIBUTIONS_CONSCRYPT":               "com.android.conscrypt",
		"RELEASE_APEX_CONTRIBUTIONS_CRASHRECOVERY":           "com.android.crashrecovery",
		"RELEASE_APEX_CONTRIBUTIONS_DEVICELOCK":              "com.android.devicelock",
		"RELEASE_APEX_CONTRIBUTIONS_DOCUMENTSUIGOOGLE":       "",
		"RELEASE_APEX_CONTRIBUTIONS_EXTSERVICES":             "com.android.extservices",
		"RELEASE_APEX_CONTRIBUTIONS_HEALTHFITNESS":           "com.android.healthfitness",
		"RELEASE_APEX_CONTRIBUTIONS_IPSEC":                   "com.android.ipsec",
		"RELEASE_APEX_CONTRIBUTIONS_MEDIA":                   "com.android.media",
		"RELEASE_APEX_CONTRIBUTIONS_MEDIAPROVIDER":           "com.android.mediaprovider",
		"RELEASE_APEX_CONTRIBUTIONS_MODULE_METADATA":         "",
		"RELEASE_APEX_CONTRIBUTIONS_NETWORKSTACKGOOGLE":      "",
		"RELEASE_APEX_CONTRIBUTIONS_NEURALNETWORKS":          "com.android.neuralnetworks",
		"RELEASE_APEX_CONTRIBUTIONS_NFC":                     "com.android.nfcservices",
		"RELEASE_APEX_CONTRIBUTIONS_ONDEVICEPERSONALIZATION": "com.android.ondevicepersonalization",
		"RELEASE_APEX_CONTRIBUTIONS_PERMISSION":              "com.android.permission",
		"RELEASE_APEX_CONTRIBUTIONS_PROFILING":               "com.android.profiling",
		"RELEASE_APEX_CONTRIBUTIONS_REMOTEKEYPROVISIONING":   "com.android.rkpd",
		"RELEASE_APEX_CONTRIBUTIONS_RESOLV":                  "com.android.resolv",
		"RELEASE_APEX_CONTRIBUTIONS_SCHEDULING":              "com.android.scheduling",
		"RELEASE_APEX_CONTRIBUTIONS_SDKEXTENSIONS":           "com.android.sdkext",
		"RELEASE_APEX_CONTRIBUTIONS_SWCODEC":                 "com.android.media.swcodec",
		"RELEASE_APEX_CONTRIBUTIONS_STATSD":                  "com.android.os.statsd",
		"RELEASE_APEX_CONTRIBUTIONS_TELEMETRY_TVP":           "",
		"RELEASE_APEX_CONTRIBUTIONS_TZDATA":                  "com.android.tzdata",
		"RELEASE_APEX_CONTRIBUTIONS_UPROBESTATS":             "com.android.uprobestats",
		"RELEASE_APEX_CONTRIBUTIONS_UWB":                     "com.android.uwb",
		"RELEASE_APEX_CONTRIBUTIONS_WIFI":                    "com.android.wifi",
	}
)

// Returns the list of _selected_ apex_contributions
// Each mainline module will have one entry in the list
func (c *config) AllApexContributions() []string {
	ret := []string{}
	for _, f := range SortedKeys(mainlineApexContributionBuildFlagsToApexNames) {
		if val, exists := c.GetBuildFlag(f); exists && val != "" {
			ret = append(ret, val)
		}
	}
	return ret
}

func (c *config) AllMainlineApexNames() []string {
	return SortedStringValues(mainlineApexContributionBuildFlagsToApexNames)
}

func (c *config) BuildIgnoreApexContributionContents() *bool {
	return c.productVariables.BuildIgnoreApexContributionContents
}

func (c *config) ProductLocales() []string {
	return c.productVariables.ProductLocales
}

func (c *config) ProductDefaultWifiChannels() []string {
	return c.productVariables.ProductDefaultWifiChannels
}

func (c *config) BoardUseVbmetaDigestInFingerprint() bool {
	return Bool(c.productVariables.BoardUseVbmetaDigestInFingerprint)
}

func (c *config) OemProperties() []string {
	return c.productVariables.OemProperties
}

func (c *config) UseDebugArt() bool {
	// If the ArtTargetIncludeDebugBuild product variable is set then return its value.
	if c.productVariables.ArtTargetIncludeDebugBuild != nil {
		return Bool(c.productVariables.ArtTargetIncludeDebugBuild)
	}

	// If the RELEASE_APEX_CONTRIBUTIONS_ART build flag is set to use a prebuilt ART apex
	// then don't use the debug apex.
	if val, ok := c.GetBuildFlag("RELEASE_APEX_CONTRIBUTIONS_ART"); ok && val != "" {
		return false
	}

	// Default to the debug apex for eng builds.
	return Bool(c.productVariables.Eng)
}

func (c *config) SystemPropFiles(ctx PathContext) Paths {
	return PathsForSource(ctx, c.productVariables.SystemPropFiles)
}

func (c *config) SystemExtPropFiles(ctx PathContext) Paths {
	return PathsForSource(ctx, c.productVariables.SystemExtPropFiles)
}

func (c *config) ProductPropFiles(ctx PathContext) Paths {
	return PathsForSource(ctx, c.productVariables.ProductPropFiles)
}

func (c *config) OdmPropFiles(ctx PathContext) Paths {
	return PathsForSource(ctx, c.productVariables.OdmPropFiles)
}

func (c *config) VendorPropFiles(ctx PathContext) Paths {
	return PathsForSource(ctx, c.productVariables.VendorPropFiles)
}

func (c *config) EnableUffdGc() string {
	return String(c.productVariables.EnableUffdGc)
}

func (c *config) DeviceFrameworkCompatibilityMatrixFile() []string {
	return c.productVariables.DeviceFrameworkCompatibilityMatrixFile
}

func (c *config) DeviceProductCompatibilityMatrixFile() []string {
	return c.productVariables.DeviceProductCompatibilityMatrixFile
}

func (c *config) BoardAvbEnable() bool {
	return Bool(c.productVariables.BoardAvbEnable)
}

func (c *config) BoardAvbSystemAddHashtreeFooterArgs() []string {
	return c.productVariables.BoardAvbSystemAddHashtreeFooterArgs
}

// Returns true if RELEASE_INSTALL_APEX_SYSTEMSERVER_DEXPREOPT_SAME_PARTITION is set to true.
// If true, dexpreopt files of apex system server jars will be installed in the same partition as the parent apex.
// If false, all these files will be installed in /system partition.
func (c Config) InstallApexSystemServerDexpreoptSamePartition() bool {
	return c.config.productVariables.GetBuildFlagBool("RELEASE_INSTALL_APEX_SYSTEMSERVER_DEXPREOPT_SAME_PARTITION")
}

func (c *config) DeviceMatrixFile() []string {
	return c.productVariables.DeviceMatrixFile
}

func (c *config) ProductManifestFiles() []string {
	return c.productVariables.ProductManifestFiles
}

func (c *config) SystemManifestFile() []string {
	return c.productVariables.SystemManifestFile
}

func (c *config) SystemExtManifestFiles() []string {
	return c.productVariables.SystemExtManifestFiles
}

func (c *config) DeviceManifestFiles() []string {
	return c.productVariables.DeviceManifestFiles
}

func (c *config) OdmManifestFiles() []string {
	return c.productVariables.OdmManifestFiles
}
