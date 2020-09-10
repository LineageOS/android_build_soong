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
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"
	"github.com/google/blueprint/pathtools"
	"github.com/google/blueprint/proptools"

	"android/soong/android/soongconfig"
	"android/soong/bazel"
	"android/soong/remoteexec"
	"android/soong/starlark_fmt"
)

// Bool re-exports proptools.Bool for the android package.
var Bool = proptools.Bool

// String re-exports proptools.String for the android package.
var String = proptools.String

// StringDefault re-exports proptools.StringDefault for the android package.
var StringDefault = proptools.StringDefault

// FutureApiLevelInt is a placeholder constant for unreleased API levels.
const FutureApiLevelInt = 10000

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

// SoongOutDir returns the build output directory for the configuration.
func (c Config) SoongOutDir() string {
	return c.soongOutDir
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
	productVariables productVariables

	// Only available on configs created by TestConfig
	TestProductVariables *productVariables

	// A specialized context object for Bazel/Soong mixed builds and migration
	// purposes.
	BazelContext BazelContext

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

	captureBuild      bool // true for tests, saves build parameters for each module
	ignoreEnvironment bool // true for tests, returns empty from all Getenv calls

	fs         pathtools.FileSystem
	mockBpList string

	runningAsBp2Build              bool
	bp2buildPackageConfig          bp2BuildConversionAllowlist
	Bp2buildSoongConfigDefinitions soongconfig.Bp2BuildSoongConfigDefinitions

	// If testAllowNonExistentPaths is true then PathForSource and PathForModuleSrc won't error
	// in tests when a path doesn't exist.
	TestAllowNonExistentPaths bool

	// The list of files that when changed, must invalidate soong_build to
	// regenerate build.ninja.
	ninjaFileDepsSet sync.Map

	OncePer
}

type deviceConfig struct {
	config *config
	OncePer
}

type jsonConfigurable interface {
	SetDefaultConfig()
}

func loadConfig(config *config) error {
	return loadFromConfigFile(&config.productVariables, absolutePath(config.ProductVariablesFileName))
}

// loadFromConfigFile loads and decodes configuration options from a JSON file
// in the current working directory.
func loadFromConfigFile(configurable *productVariables, filename string) error {
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

	return saveToBazelConfigFile(configurable, filepath.Dir(filename))
}

// atomically writes the config file in case two copies of soong_build are running simultaneously
// (for example, docs generation and ninja manifest generation)
func saveToConfigFile(config *productVariables, filename string) error {
	data, err := json.MarshalIndent(&config, "", "    ")
	if err != nil {
		return fmt.Errorf("cannot marshal config data: %s", err.Error())
	}

	f, err := ioutil.TempFile(filepath.Dir(filename), "config")
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

func saveToBazelConfigFile(config *productVariables, outDir string) error {
	dir := filepath.Join(outDir, bazel.SoongInjectionDirName, "product_config")
	err := createDirIfNonexistent(dir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("Could not create dir %s: %s", dir, err)
	}

	nonArchVariantProductVariables := []string{}
	archVariantProductVariables := []string{}
	p := variableProperties{}
	t := reflect.TypeOf(p.Product_variables)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		nonArchVariantProductVariables = append(nonArchVariantProductVariables, strings.ToLower(f.Name))
		if proptools.HasTag(f, "android", "arch_variant") {
			archVariantProductVariables = append(archVariantProductVariables, strings.ToLower(f.Name))
		}
	}

	nonArchVariantProductVariablesJson := starlark_fmt.PrintStringList(nonArchVariantProductVariables, 0)
	if err != nil {
		return fmt.Errorf("cannot marshal product variable data: %s", err.Error())
	}

	archVariantProductVariablesJson := starlark_fmt.PrintStringList(archVariantProductVariables, 0)
	if err != nil {
		return fmt.Errorf("cannot marshal arch variant product variable data: %s", err.Error())
	}

	configJson, err := json.MarshalIndent(&config, "", "    ")
	if err != nil {
		return fmt.Errorf("cannot marshal config data: %s", err.Error())
	}

	bzl := []string{
		bazel.GeneratedBazelFileWarning,
		fmt.Sprintf(`_product_vars = json.decode("""%s""")`, configJson),
		fmt.Sprintf(`_product_var_constraints = %s`, nonArchVariantProductVariablesJson),
		fmt.Sprintf(`_arch_variant_product_var_constraints = %s`, archVariantProductVariablesJson),
		"\n", `
product_vars = _product_vars
product_var_constraints = _product_var_constraints
arch_variant_product_var_constraints = _arch_variant_product_var_constraints
`,
	}
	err = ioutil.WriteFile(filepath.Join(dir, "product_variables.bzl"), []byte(strings.Join(bzl, "\n")), 0644)
	if err != nil {
		return fmt.Errorf("Could not write .bzl config file %s", err)
	}
	err = ioutil.WriteFile(filepath.Join(dir, "BUILD"), []byte(bazel.GeneratedBazelFileWarning), 0644)
	if err != nil {
		return fmt.Errorf("Could not write BUILD config file %s", err)
	}

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

// TestConfig returns a Config object for testing.
func TestConfig(buildDir string, env map[string]string, bp string, fs map[string][]byte) Config {
	envCopy := make(map[string]string)
	for k, v := range env {
		envCopy[k] = v
	}

	// Copy the real PATH value to the test environment, it's needed by
	// NonHermeticHostSystemTool() used in x86_darwin_host.go
	envCopy["PATH"] = os.Getenv("PATH")

	config := &config{
		productVariables: productVariables{
			DeviceName:                          stringPtr("test_device"),
			DeviceProduct:                       stringPtr("test_product"),
			Platform_sdk_version:                intPtr(30),
			Platform_sdk_codename:               stringPtr("S"),
			Platform_base_sdk_extension_version: intPtr(1),
			Platform_version_active_codenames:   []string{"S", "Tiramisu"},
			DeviceSystemSdkVersions:             []string{"14", "15"},
			Platform_systemsdk_versions:         []string{"29", "30"},
			AAPTConfig:                          []string{"normal", "large", "xlarge", "hdpi", "xhdpi", "xxhdpi"},
			AAPTPreferredConfig:                 stringPtr("xhdpi"),
			AAPTCharacteristics:                 stringPtr("nosdcard"),
			AAPTPrebuiltDPI:                     []string{"xhdpi", "xxhdpi"},
			UncompressPrivAppDex:                boolPtr(true),
			ShippingApiLevel:                    stringPtr("30"),
		},

		outDir:       buildDir,
		soongOutDir:  filepath.Join(buildDir, "soong"),
		captureBuild: true,
		env:          envCopy,

		// Set testAllowNonExistentPaths so that test contexts don't need to specify every path
		// passed to PathForSource or PathForModuleSrc.
		TestAllowNonExistentPaths: true,

		BazelContext: noopBazelContext{},
	}
	config.deviceConfig = &deviceConfig{
		config: config,
	}
	config.TestProductVariables = &config.productVariables

	config.mockFileSystem(bp, fs)

	determineBuildOS(config)

	return Config{config}
}

func modifyTestConfigToSupportArchMutator(testConfig Config) {
	config := testConfig.config

	config.Targets = map[OsType][]Target{
		Android: []Target{
			{Android, Arch{ArchType: Arm64, ArchVariant: "armv8-a", Abi: []string{"arm64-v8a"}}, NativeBridgeDisabled, "", "", false},
			{Android, Arch{ArchType: Arm, ArchVariant: "armv7-a-neon", Abi: []string{"armeabi-v7a"}}, NativeBridgeDisabled, "", "", false},
		},
		config.BuildOS: []Target{
			{config.BuildOS, Arch{ArchType: X86_64}, NativeBridgeDisabled, "", "", false},
			{config.BuildOS, Arch{ArchType: X86}, NativeBridgeDisabled, "", "", false},
		},
	}

	if runtime.GOOS == "darwin" {
		config.Targets[config.BuildOS] = config.Targets[config.BuildOS][:1]
	}

	config.BuildOSTarget = config.Targets[config.BuildOS][0]
	config.BuildOSCommonTarget = getCommonTargets(config.Targets[config.BuildOS])[0]
	config.AndroidCommonTarget = getCommonTargets(config.Targets[Android])[0]
	config.AndroidFirstDeviceTarget = FirstTarget(config.Targets[Android], "lib64", "lib32")[0]
	config.TestProductVariables.DeviceArch = proptools.StringPtr("arm64")
	config.TestProductVariables.DeviceArchVariant = proptools.StringPtr("armv8-a")
	config.TestProductVariables.DeviceSecondaryArch = proptools.StringPtr("arm")
	config.TestProductVariables.DeviceSecondaryArchVariant = proptools.StringPtr("armv7-a-neon")
}

func modifyTestConfigForMusl(config Config) {
	delete(config.Targets, config.BuildOS)
	config.productVariables.HostMusl = boolPtr(true)
	determineBuildOS(config.config)
	config.Targets[config.BuildOS] = []Target{
		{config.BuildOS, Arch{ArchType: X86_64}, NativeBridgeDisabled, "", "", false},
		{config.BuildOS, Arch{ArchType: X86}, NativeBridgeDisabled, "", "", false},
	}

	config.BuildOSTarget = config.Targets[config.BuildOS][0]
	config.BuildOSCommonTarget = getCommonTargets(config.Targets[config.BuildOS])[0]
}

// TestArchConfig returns a Config object suitable for using for tests that
// need to run the arch mutator.
func TestArchConfig(buildDir string, env map[string]string, bp string, fs map[string][]byte) Config {
	testConfig := TestConfig(buildDir, env, bp, fs)
	modifyTestConfigToSupportArchMutator(testConfig)
	return testConfig
}

// ConfigForAdditionalRun is a config object which is "reset" for another
// bootstrap run. Only per-run data is reset. Data which needs to persist across
// multiple runs in the same program execution is carried over (such as Bazel
// context or environment deps).
func ConfigForAdditionalRun(c Config) (Config, error) {
	newConfig, err := NewConfig(c.moduleListFile, c.runGoTests, c.outDir, c.soongOutDir, c.env)
	if err != nil {
		return Config{}, err
	}
	newConfig.BazelContext = c.BazelContext
	newConfig.envDeps = c.envDeps
	return newConfig, nil
}

// NewConfig creates a new Config object. The srcDir argument specifies the path
// to the root source directory. It also loads the config file, if found.
func NewConfig(moduleListFile string, runGoTests bool, outDir, soongOutDir string, availableEnv map[string]string) (Config, error) {
	// Make a config with default options.
	config := &config{
		ProductVariablesFileName: filepath.Join(soongOutDir, productVariablesFileName),

		env: availableEnv,

		outDir:            outDir,
		soongOutDir:       soongOutDir,
		runGoTests:        runGoTests,
		multilibConflicts: make(map[ArchType]bool),

		moduleListFile: moduleListFile,
		fs:             pathtools.NewOsFs(absSrcDir),
	}

	config.deviceConfig = &deviceConfig{
		config: config,
	}

	// Soundness check of the build and source directories. This won't catch strange
	// configurations with symlinks, but at least checks the obvious case.
	absBuildDir, err := filepath.Abs(soongOutDir)
	if err != nil {
		return Config{}, err
	}

	absSrcDir, err := filepath.Abs(".")
	if err != nil {
		return Config{}, err
	}

	if strings.HasPrefix(absSrcDir, absBuildDir) {
		return Config{}, fmt.Errorf("Build dir must not contain source directory")
	}

	// Load any configurable options from the configuration file
	err = loadConfig(config)
	if err != nil {
		return Config{}, err
	}

	KatiEnabledMarkerFile := filepath.Join(soongOutDir, ".soong.kati_enabled")
	if _, err := os.Stat(absolutePath(KatiEnabledMarkerFile)); err == nil {
		config.katiEnabled = true
	}

	determineBuildOS(config)

	// Sets up the map of target OSes to the finer grained compilation targets
	// that are configured from the product variables.
	targets, err := decodeTargetProductVariables(config)
	if err != nil {
		return Config{}, err
	}

	// Make the CommonOS OsType available for all products.
	targets[CommonOS] = []Target{commonTargetMap[CommonOS.Name]}

	var archConfig []archConfig
	if config.NdkAbis() {
		archConfig = getNdkAbisConfig()
	} else if config.AmlAbis() {
		archConfig = getAmlAbisConfig()
	}

	if archConfig != nil {
		androidTargets, err := decodeAndroidArchSettings(archConfig)
		if err != nil {
			return Config{}, err
		}
		targets[Android] = androidTargets
	}

	multilib := make(map[string]bool)
	for _, target := range targets[Android] {
		if seen := multilib[target.Arch.ArchType.Multilib]; seen {
			config.multilibConflicts[target.Arch.ArchType] = true
		}
		multilib[target.Arch.ArchType.Multilib] = true
	}

	// Map of OS to compilation targets.
	config.Targets = targets

	// Compilation targets for host tools.
	config.BuildOSTarget = config.Targets[config.BuildOS][0]
	config.BuildOSCommonTarget = getCommonTargets(config.Targets[config.BuildOS])[0]

	// Compilation targets for Android.
	if len(config.Targets[Android]) > 0 {
		config.AndroidCommonTarget = getCommonTargets(config.Targets[Android])[0]
		config.AndroidFirstDeviceTarget = FirstTarget(config.Targets[Android], "lib64", "lib32")[0]
	}

	config.BazelContext, err = NewBazelContext(config)
	config.bp2buildPackageConfig = bp2buildAllowlist

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
	if c.KatiEnabled() {
		return filepath.Join(c.outDir, "host", c.PrebuiltOS(), "bin")
	} else {
		return filepath.Join(c.soongOutDir, "host", c.PrebuiltOS(), "bin")
	}
}

func (c *config) HostToolPath(ctx PathContext, tool string) Path {
	path := pathForInstall(ctx, ctx.Config().BuildOS, ctx.Config().BuildArch, "bin", false, tool)
	return path
}

func (c *config) HostJNIToolPath(ctx PathContext, lib string) Path {
	ext := ".so"
	if runtime.GOOS == "darwin" {
		ext = ".dylib"
	}
	path := pathForInstall(ctx, ctx.Config().BuildOS, ctx.Config().BuildArch, "lib64", false, lib+ext)
	return path
}

func (c *config) HostJavaToolPath(ctx PathContext, tool string) Path {
	path := pathForInstall(ctx, ctx.Config().BuildOS, ctx.Config().BuildArch, "framework", false, tool)
	return path
}

// PrebuiltOS returns the name of the host OS used in prebuilts directories.
func (c *config) PrebuiltOS() string {
	switch runtime.GOOS {
	case "linux":
		return "linux-x86"
	case "darwin":
		return "darwin-x86"
	default:
		panic("Unknown GOOS")
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
	value := c.Getenv(key)
	return value == "1" || value == "y" || value == "yes" || value == "on" || value == "true"
}

func (c *config) IsEnvFalse(key string) bool {
	value := c.Getenv(key)
	return value == "0" || value == "n" || value == "no" || value == "off" || value == "false"
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

func (c *config) BuildId() string {
	return String(c.productVariables.BuildId)
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

// DeviceName returns the name of the current device target.
// TODO: take an AndroidModuleContext to select the device name for multi-device builds
func (c *config) DeviceName() string {
	return *c.productVariables.DeviceName
}

// DeviceProduct returns the current product target. There could be multiple of
// these per device type.
//
// NOTE: Do not base conditional logic on this value. It may break product
//       inheritance.
func (c *config) DeviceProduct() string {
	return *c.productVariables.DeviceProduct
}

func (c *config) DeviceResourceOverlays() []string {
	return c.productVariables.DeviceResourceOverlays
}

func (c *config) ProductResourceOverlays() []string {
	return c.productVariables.ProductResourceOverlays
}

func (c *config) PlatformVersionName() string {
	return String(c.productVariables.Platform_version_name)
}

func (c *config) PlatformSdkVersion() ApiLevel {
	return uncheckedFinalApiLevel(*c.productVariables.Platform_sdk_version)
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
	return String(c.productVariables.Platform_min_supported_target_sdk_version)
}

func (c *config) PlatformBaseOS() string {
	return String(c.productVariables.Platform_base_os)
}

func (c *config) PlatformVersionLastStable() string {
	return String(c.productVariables.Platform_version_last_stable)
}

func (c *config) MinSupportedSdkVersion() ApiLevel {
	return uncheckedFinalApiLevel(19)
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
	for i, codename := range c.PlatformVersionActiveCodenames() {
		levels = append(levels, ApiLevel{
			value:     codename,
			number:    i,
			isPreview: true,
		})
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
	if codename == "" {
		return NoneApiLevel
	}
	if codename == "REL" {
		panic("Platform_sdk_codename should not be REL when Platform_sdk_final is true")
	}
	return ApiLevelOrPanic(ctx, codename)
}

func (c *config) AppsDefaultVersionName() string {
	return String(c.productVariables.AppsDefaultVersionName)
}

// Codenames that are active in the current lunch target.
func (c *config) PlatformVersionActiveCodenames() []string {
	return c.productVariables.Platform_version_active_codenames
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
	return PathForSource(ctx, "build/make/target/product/security")
}

func (c *config) DefaultAppCertificate(ctx PathContext) (pem, key SourcePath) {
	defaultCert := String(c.productVariables.DefaultAppCertificate)
	if defaultCert != "" {
		return PathForSource(ctx, defaultCert+".x509.pem"), PathForSource(ctx, defaultCert+".pk8")
	}
	defaultDir := c.DefaultAppCertificateDir(ctx)
	return defaultDir.Join(ctx, "testkey.x509.pem"), defaultDir.Join(ctx, "testkey.pk8")
}

func (c *config) ApexKeyDir(ctx ModuleContext) SourcePath {
	// TODO(b/121224311): define another variable such as TARGET_APEX_KEY_OVERRIDE
	defaultCert := String(c.productVariables.DefaultAppCertificate)
	if defaultCert == "" || filepath.Dir(defaultCert) == "build/make/target/product/security" {
		// When defaultCert is unset or is set to the testkeys path, use the APEX keys
		// that is under the module dir
		return pathForModuleSrc(ctx)
	}
	// If not, APEX keys are under the specified directory
	return PathForSource(ctx, filepath.Dir(defaultCert))
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
	return c.IsEnvTrue("RUN_ERROR_PRONE")
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
	return "0x60000000"
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

func (c *config) ExportedNamespaces() []string {
	return append([]string(nil), c.productVariables.NamespacesToExport...)
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
	return ioutil.ReadFile(absolutePath(path.String()))
}

func (c *deviceConfig) WithDexpreopt() bool {
	return c.config.productVariables.WithDexpreopt
}

func (c *config) FrameworksBaseDirExists(ctx PathContext) bool {
	return ExistentPathForSource(ctx, "frameworks", "base", "Android.bp").Valid()
}

func (c *config) VndkSnapshotBuildArtifacts() bool {
	return Bool(c.productVariables.VndkSnapshotBuildArtifacts)
}

func (c *config) HasMultilibConflict(arch ArchType) bool {
	return c.multilibConflicts[arch]
}

func (c *config) PrebuiltHiddenApiDir(ctx PathContext) string {
	return String(c.productVariables.PrebuiltHiddenApiDir)
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

func (c *deviceConfig) VendorPath() string {
	if c.config.productVariables.VendorPath != nil {
		return *c.config.productVariables.VendorPath
	}
	return "vendor"
}

func (c *deviceConfig) VndkVersion() string {
	return String(c.config.productVariables.DeviceVndkVersion)
}

func (c *deviceConfig) RecoverySnapshotVersion() string {
	return String(c.config.productVariables.RecoverySnapshotVersion)
}

func (c *deviceConfig) CurrentApiLevelForVendorModules() string {
	return StringDefault(c.config.productVariables.DeviceCurrentApiLevelForVendorModules, "current")
}

func (c *deviceConfig) PlatformVndkVersion() string {
	return String(c.config.productVariables.Platform_vndk_version)
}

func (c *deviceConfig) ProductVndkVersion() string {
	return String(c.config.productVariables.ProductVndkVersion)
}

func (c *deviceConfig) ExtraVndkVersions() []string {
	return c.config.productVariables.ExtraVndkVersions
}

func (c *deviceConfig) VndkUseCoreVariant() bool {
	return Bool(c.config.productVariables.VndkUseCoreVariant)
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

func (c *deviceConfig) ProductPath() string {
	if c.config.productVariables.ProductPath != nil {
		return *c.config.productVariables.ProductPath
	}
	return "product"
}

func (c *deviceConfig) SystemExtPath() string {
	if c.config.productVariables.SystemExtPath != nil {
		return *c.config.productVariables.SystemExtPath
	}
	return "system_ext"
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

func (c *deviceConfig) AfdoAdditionalProfileDirs() []string {
	return c.config.productVariables.AfdoAdditionalProfileDirs
}

func (c *deviceConfig) PgoAdditionalProfileDirs() []string {
	return c.config.productVariables.PgoAdditionalProfileDirs
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

func (c *config) VendorConfig(name string) VendorConfig {
	return soongconfig.Config(c.productVariables.VendorVars[name])
}

func (c *config) NdkAbis() bool {
	return Bool(c.productVariables.Ndk_abis)
}

func (c *config) AmlAbis() bool {
	return Bool(c.productVariables.Aml_abis)
}

func (c *config) FlattenApex() bool {
	return Bool(c.productVariables.Flatten_apex)
}

func (c *config) ForceApexSymlinkOptimization() bool {
	return Bool(c.productVariables.ForceApexSymlinkOptimization)
}

func (c *config) CompressedApex() bool {
	return Bool(c.productVariables.CompressedApex)
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

func (c *config) EnforceInterPartitionJavaSdkLibrary() bool {
	return Bool(c.productVariables.EnforceInterPartitionJavaSdkLibrary)
}

func (c *config) InterPartitionJavaLibraryAllowList() []string {
	return c.productVariables.InterPartitionJavaLibraryAllowList
}

func (c *config) InstallExtraFlattenedApexes() bool {
	return Bool(c.productVariables.InstallExtraFlattenedApexes)
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

func (c *config) MissingUsesLibraries() []string {
	return c.productVariables.MissingUsesLibraries
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

func (c *deviceConfig) TotSepolicyVersion() string {
	return String(c.config.productVariables.TotSepolicyVersion)
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

func (c *deviceConfig) BoardPlatVendorPolicy() []string {
	return c.config.productVariables.BoardPlatVendorPolicy
}

func (c *deviceConfig) BoardReqdMaskPolicy() []string {
	return c.config.productVariables.BoardReqdMaskPolicy
}

func (c *deviceConfig) BoardSystemExtPublicPrebuiltDirs() []string {
	return c.config.productVariables.BoardSystemExtPublicPrebuiltDirs
}

func (c *deviceConfig) BoardSystemExtPrivatePrebuiltDirs() []string {
	return c.config.productVariables.BoardSystemExtPrivatePrebuiltDirs
}

func (c *deviceConfig) BoardProductPublicPrebuiltDirs() []string {
	return c.config.productVariables.BoardProductPublicPrebuiltDirs
}

func (c *deviceConfig) BoardProductPrivatePrebuiltDirs() []string {
	return c.config.productVariables.BoardProductPrivatePrebuiltDirs
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

func (c *deviceConfig) DirectedVendorSnapshot() bool {
	return c.config.productVariables.DirectedVendorSnapshot
}

func (c *deviceConfig) VendorSnapshotModules() map[string]bool {
	return c.config.productVariables.VendorSnapshotModules
}

func (c *deviceConfig) DirectedRecoverySnapshot() bool {
	return c.config.productVariables.DirectedRecoverySnapshot
}

func (c *deviceConfig) RecoverySnapshotModules() map[string]bool {
	return c.config.productVariables.RecoverySnapshotModules
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

var vendorSnapshotDirsExcludedKey = NewOnceKey("VendorSnapshotDirsExcludedMap")

func (c *deviceConfig) VendorSnapshotDirsExcludedMap() map[string]bool {
	return c.createDirsMapOnce(vendorSnapshotDirsExcludedKey, nil,
		c.config.productVariables.VendorSnapshotDirsExcluded)
}

var vendorSnapshotDirsIncludedKey = NewOnceKey("VendorSnapshotDirsIncludedMap")

func (c *deviceConfig) VendorSnapshotDirsIncludedMap() map[string]bool {
	excludedMap := c.VendorSnapshotDirsExcludedMap()
	return c.createDirsMapOnce(vendorSnapshotDirsIncludedKey, excludedMap,
		c.config.productVariables.VendorSnapshotDirsIncluded)
}

var recoverySnapshotDirsExcludedKey = NewOnceKey("RecoverySnapshotDirsExcludedMap")

func (c *deviceConfig) RecoverySnapshotDirsExcludedMap() map[string]bool {
	return c.createDirsMapOnce(recoverySnapshotDirsExcludedKey, nil,
		c.config.productVariables.RecoverySnapshotDirsExcluded)
}

var recoverySnapshotDirsIncludedKey = NewOnceKey("RecoverySnapshotDirsIncludedMap")

func (c *deviceConfig) RecoverySnapshotDirsIncludedMap() map[string]bool {
	excludedMap := c.RecoverySnapshotDirsExcludedMap()
	return c.createDirsMapOnce(recoverySnapshotDirsIncludedKey, excludedMap,
		c.config.productVariables.RecoverySnapshotDirsIncluded)
}

func (c *deviceConfig) HostFakeSnapshotEnabled() bool {
	return c.config.productVariables.HostFakeSnapshotEnabled
}

func (c *deviceConfig) ShippingApiLevel() ApiLevel {
	if c.config.productVariables.ShippingApiLevel == nil {
		return NoneApiLevel
	}
	apiLevel, _ := strconv.Atoi(*c.config.productVariables.ShippingApiLevel)
	return uncheckedFinalApiLevel(apiLevel)
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

func (c *deviceConfig) RequiresInsecureExecmemForSwiftshader() bool {
	return c.config.productVariables.RequiresInsecureExecmemForSwiftshader
}

func (c *config) SelinuxIgnoreNeverallows() bool {
	return c.productVariables.SelinuxIgnoreNeverallows
}

func (c *deviceConfig) SepolicySplit() bool {
	return c.config.productVariables.SepolicySplit
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

// The ConfiguredJarList struct provides methods for handling a list of (apex, jar) pairs.
// Such lists are used in the build system for things like bootclasspath jars or system server jars.
// The apex part is either an apex name, or a special names "platform" or "system_ext". Jar is a
// module name. The pairs come from Make product variables as a list of colon-separated strings.
//
// Examples:
//   - "com.android.art:core-oj"
//   - "platform:framework"
//   - "system_ext:foo"
//
type ConfiguredJarList struct {
	// A list of apex components, which can be an apex name,
	// or special names like "platform" or "system_ext".
	apexes []string

	// A list of jar module name components.
	jars []string
}

// Len returns the length of the list of jars.
func (l *ConfiguredJarList) Len() int {
	return len(l.jars)
}

// Jar returns the idx-th jar component of (apex, jar) pairs.
func (l *ConfiguredJarList) Jar(idx int) string {
	return l.jars[idx]
}

// Apex returns the idx-th apex component of (apex, jar) pairs.
func (l *ConfiguredJarList) Apex(idx int) string {
	return l.apexes[idx]
}

// ContainsJar returns true if the (apex, jar) pairs contains a pair with the
// given jar module name.
func (l *ConfiguredJarList) ContainsJar(jar string) bool {
	return InList(jar, l.jars)
}

// If the list contains the given (apex, jar) pair.
func (l *ConfiguredJarList) containsApexJarPair(apex, jar string) bool {
	for i := 0; i < l.Len(); i++ {
		if apex == l.apexes[i] && jar == l.jars[i] {
			return true
		}
	}
	return false
}

// ApexOfJar returns the apex component of the first pair with the given jar name on the list, or
// an empty string if not found.
func (l *ConfiguredJarList) ApexOfJar(jar string) string {
	if idx := IndexList(jar, l.jars); idx != -1 {
		return l.Apex(IndexList(jar, l.jars))
	}
	return ""
}

// IndexOfJar returns the first pair with the given jar name on the list, or -1
// if not found.
func (l *ConfiguredJarList) IndexOfJar(jar string) int {
	return IndexList(jar, l.jars)
}

func copyAndAppend(list []string, item string) []string {
	// Create the result list to be 1 longer than the input.
	result := make([]string, len(list)+1)

	// Copy the whole input list into the result.
	count := copy(result, list)

	// Insert the extra item at the end.
	result[count] = item

	return result
}

// Append an (apex, jar) pair to the list.
func (l *ConfiguredJarList) Append(apex string, jar string) ConfiguredJarList {
	// Create a copy of the backing arrays before appending to avoid sharing backing
	// arrays that are mutated across instances.
	apexes := copyAndAppend(l.apexes, apex)
	jars := copyAndAppend(l.jars, jar)

	return ConfiguredJarList{apexes, jars}
}

// Append a list of (apex, jar) pairs to the list.
func (l *ConfiguredJarList) AppendList(other *ConfiguredJarList) ConfiguredJarList {
	apexes := make([]string, 0, l.Len()+other.Len())
	jars := make([]string, 0, l.Len()+other.Len())

	apexes = append(apexes, l.apexes...)
	jars = append(jars, l.jars...)

	apexes = append(apexes, other.apexes...)
	jars = append(jars, other.jars...)

	return ConfiguredJarList{apexes, jars}
}

// RemoveList filters out a list of (apex, jar) pairs from the receiving list of pairs.
func (l *ConfiguredJarList) RemoveList(list ConfiguredJarList) ConfiguredJarList {
	apexes := make([]string, 0, l.Len())
	jars := make([]string, 0, l.Len())

	for i, jar := range l.jars {
		apex := l.apexes[i]
		if !list.containsApexJarPair(apex, jar) {
			apexes = append(apexes, apex)
			jars = append(jars, jar)
		}
	}

	return ConfiguredJarList{apexes, jars}
}

// Filter keeps the entries if a jar appears in the given list of jars to keep. Returns a new list
// and any remaining jars that are not on this list.
func (l *ConfiguredJarList) Filter(jarsToKeep []string) (ConfiguredJarList, []string) {
	var apexes []string
	var jars []string

	for i, jar := range l.jars {
		if InList(jar, jarsToKeep) {
			apexes = append(apexes, l.apexes[i])
			jars = append(jars, jar)
		}
	}

	return ConfiguredJarList{apexes, jars}, RemoveListFromList(jarsToKeep, jars)
}

// CopyOfJars returns a copy of the list of strings containing jar module name
// components.
func (l *ConfiguredJarList) CopyOfJars() []string {
	return CopyOf(l.jars)
}

// CopyOfApexJarPairs returns a copy of the list of strings with colon-separated
// (apex, jar) pairs.
func (l *ConfiguredJarList) CopyOfApexJarPairs() []string {
	pairs := make([]string, 0, l.Len())

	for i, jar := range l.jars {
		apex := l.apexes[i]
		pairs = append(pairs, apex+":"+jar)
	}

	return pairs
}

// BuildPaths returns a list of build paths based on the given directory prefix.
func (l *ConfiguredJarList) BuildPaths(ctx PathContext, dir OutputPath) WritablePaths {
	paths := make(WritablePaths, l.Len())
	for i, jar := range l.jars {
		paths[i] = dir.Join(ctx, ModuleStem(jar)+".jar")
	}
	return paths
}

// BuildPathsByModule returns a map from module name to build paths based on the given directory
// prefix.
func (l *ConfiguredJarList) BuildPathsByModule(ctx PathContext, dir OutputPath) map[string]WritablePath {
	paths := map[string]WritablePath{}
	for _, jar := range l.jars {
		paths[jar] = dir.Join(ctx, ModuleStem(jar)+".jar")
	}
	return paths
}

// UnmarshalJSON converts JSON configuration from raw bytes into a
// ConfiguredJarList structure.
func (l *ConfiguredJarList) UnmarshalJSON(b []byte) error {
	// Try and unmarshal into a []string each item of which contains a pair
	// <apex>:<jar>.
	var list []string
	err := json.Unmarshal(b, &list)
	if err != nil {
		// Did not work so return
		return err
	}

	apexes, jars, err := splitListOfPairsIntoPairOfLists(list)
	if err != nil {
		return err
	}
	l.apexes = apexes
	l.jars = jars
	return nil
}

func (l *ConfiguredJarList) MarshalJSON() ([]byte, error) {
	if len(l.apexes) != len(l.jars) {
		return nil, errors.New(fmt.Sprintf("Inconsistent ConfiguredJarList: apexes: %q, jars: %q", l.apexes, l.jars))
	}

	list := make([]string, 0, len(l.apexes))

	for i := 0; i < len(l.apexes); i++ {
		list = append(list, l.apexes[i]+":"+l.jars[i])
	}

	return json.Marshal(list)
}

// ModuleStem hardcodes the stem of framework-minus-apex to return "framework".
//
// TODO(b/139391334): hard coded until we find a good way to query the stem of a
// module before any other mutators are run.
func ModuleStem(module string) string {
	if module == "framework-minus-apex" {
		return "framework"
	}
	return module
}

// DevicePaths computes the on-device paths for the list of (apex, jar) pairs,
// based on the operating system.
func (l *ConfiguredJarList) DevicePaths(cfg Config, ostype OsType) []string {
	paths := make([]string, l.Len())
	for i, jar := range l.jars {
		apex := l.apexes[i]
		name := ModuleStem(jar) + ".jar"

		var subdir string
		if apex == "platform" {
			subdir = "system/framework"
		} else if apex == "system_ext" {
			subdir = "system_ext/framework"
		} else {
			subdir = filepath.Join("apex", apex, "javalib")
		}

		if ostype.Class == Host {
			paths[i] = filepath.Join(cfg.Getenv("OUT_DIR"), "host", cfg.PrebuiltOS(), subdir, name)
		} else {
			paths[i] = filepath.Join("/", subdir, name)
		}
	}
	return paths
}

func (l *ConfiguredJarList) String() string {
	var pairs []string
	for i := 0; i < l.Len(); i++ {
		pairs = append(pairs, l.apexes[i]+":"+l.jars[i])
	}
	return strings.Join(pairs, ",")
}

func splitListOfPairsIntoPairOfLists(list []string) ([]string, []string, error) {
	// Now we need to populate this list by splitting each item in the slice of
	// pairs and appending them to the appropriate list of apexes or jars.
	apexes := make([]string, len(list))
	jars := make([]string, len(list))

	for i, apexjar := range list {
		apex, jar, err := splitConfiguredJarPair(apexjar)
		if err != nil {
			return nil, nil, err
		}
		apexes[i] = apex
		jars[i] = jar
	}

	return apexes, jars, nil
}

// Expected format for apexJarValue = <apex name>:<jar name>
func splitConfiguredJarPair(str string) (string, string, error) {
	pair := strings.SplitN(str, ":", 2)
	if len(pair) == 2 {
		apex := pair[0]
		jar := pair[1]
		if apex == "" {
			return apex, jar, fmt.Errorf("invalid apex '%s' in <apex>:<jar> pair '%s', expected format: <apex>:<jar>", apex, str)
		}
		return apex, jar, nil
	} else {
		return "error-apex", "error-jar", fmt.Errorf("malformed (apex, jar) pair: '%s', expected format: <apex>:<jar>", str)
	}
}

// CreateTestConfiguredJarList is a function to create ConfiguredJarList for tests.
func CreateTestConfiguredJarList(list []string) ConfiguredJarList {
	// Create the ConfiguredJarList in as similar way as it is created at runtime by marshalling to
	// a json list of strings and then unmarshalling into a ConfiguredJarList instance.
	b, err := json.Marshal(list)
	if err != nil {
		panic(err)
	}

	var jarList ConfiguredJarList
	err = json.Unmarshal(b, &jarList)
	if err != nil {
		panic(err)
	}

	return jarList
}

// EmptyConfiguredJarList returns an empty jar list.
func EmptyConfiguredJarList() ConfiguredJarList {
	return ConfiguredJarList{}
}

var earlyBootJarsKey = NewOnceKey("earlyBootJars")

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
