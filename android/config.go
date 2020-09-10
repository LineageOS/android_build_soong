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

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"
	"github.com/google/blueprint/pathtools"
	"github.com/google/blueprint/proptools"

	"android/soong/android/soongconfig"
)

var Bool = proptools.Bool
var String = proptools.String

const FutureApiLevel = 10000

// The configuration file name
const configFileName = "soong.config"
const productVariablesFileName = "soong.variables"

// A FileConfigurableOptions contains options which can be configured by the
// config file. These will be included in the config struct.
type FileConfigurableOptions struct {
	Mega_device *bool `json:",omitempty"`
	Host_bionic *bool `json:",omitempty"`
}

func (f *FileConfigurableOptions) SetDefaultConfig() {
	*f = FileConfigurableOptions{}
}

// A Config object represents the entire build configuration for Android.
type Config struct {
	*config
}

func (c Config) BuildDir() string {
	return c.buildDir
}

// A DeviceConfig object represents the configuration for a particular device being built.  For
// now there will only be one of these, but in the future there may be multiple devices being
// built
type DeviceConfig struct {
	*deviceConfig
}

type VendorConfig soongconfig.SoongConfig

type config struct {
	FileConfigurableOptions
	productVariables productVariables

	// Only available on configs created by TestConfig
	TestProductVariables *productVariables

	PrimaryBuilder           string
	ConfigFileName           string
	ProductVariablesFileName string

	Targets             map[OsType][]Target
	BuildOSTarget       Target // the Target for tools run on the build machine
	BuildOSCommonTarget Target // the Target for common (java) tools run on the build machine
	AndroidCommonTarget Target // the Target for common modules for the Android device

	// multilibConflicts for an ArchType is true if there is earlier configured device architecture with the same
	// multilib value.
	multilibConflicts map[ArchType]bool

	deviceConfig *deviceConfig

	srcDir   string // the path of the root source directory
	buildDir string // the path of the build output directory

	env       map[string]string
	envLock   sync.Mutex
	envDeps   map[string]string
	envFrozen bool

	inMake bool

	captureBuild      bool // true for tests, saves build parameters for each module
	ignoreEnvironment bool // true for tests, returns empty from all Getenv calls

	stopBefore bootstrap.StopBefore

	fs         pathtools.FileSystem
	mockBpList string

	// If testAllowNonExistentPaths is true then PathForSource and PathForModuleSrc won't error
	// in tests when a path doesn't exist.
	testAllowNonExistentPaths bool

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
	err := loadFromConfigFile(&config.FileConfigurableOptions, absolutePath(config.ConfigFileName))
	if err != nil {
		return err
	}

	return loadFromConfigFile(&config.productVariables, absolutePath(config.ProductVariablesFileName))
}

// loads configuration options from a JSON file in the cwd.
func loadFromConfigFile(configurable jsonConfigurable, filename string) error {
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

	// No error
	return nil
}

// atomically writes the config file in case two copies of soong_build are running simultaneously
// (for example, docs generation and ninja manifest generation)
func saveToConfigFile(config jsonConfigurable, filename string) error {
	data, err := json.MarshalIndent(&config, "", "    ")
	if err != nil {
		return fmt.Errorf("cannot marshal config data: %s", err.Error())
	}

	f, err := ioutil.TempFile(filepath.Dir(filename), "config")
	if err != nil {
		return fmt.Errorf("cannot create empty config file %s: %s\n", filename, err.Error())
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
func NullConfig(buildDir string) Config {
	return Config{
		config: &config{
			buildDir: buildDir,
			fs:       pathtools.OsFs,
		},
	}
}

// TestConfig returns a Config object suitable for using for tests
func TestConfig(buildDir string, env map[string]string, bp string, fs map[string][]byte) Config {
	envCopy := make(map[string]string)
	for k, v := range env {
		envCopy[k] = v
	}

	// Copy the real PATH value to the test environment, it's needed by HostSystemTool() used in x86_darwin_host.go
	envCopy["PATH"] = originalEnv["PATH"]

	config := &config{
		productVariables: productVariables{
			DeviceName:                  stringPtr("test_device"),
			Platform_sdk_version:        intPtr(30),
			DeviceSystemSdkVersions:     []string{"14", "15"},
			Platform_systemsdk_versions: []string{"29", "30"},
			AAPTConfig:                  []string{"normal", "large", "xlarge", "hdpi", "xhdpi", "xxhdpi"},
			AAPTPreferredConfig:         stringPtr("xhdpi"),
			AAPTCharacteristics:         stringPtr("nosdcard"),
			AAPTPrebuiltDPI:             []string{"xhdpi", "xxhdpi"},
			UncompressPrivAppDex:        boolPtr(true),
		},

		buildDir:     buildDir,
		captureBuild: true,
		env:          envCopy,

		// Set testAllowNonExistentPaths so that test contexts don't need to specify every path
		// passed to PathForSource or PathForModuleSrc.
		testAllowNonExistentPaths: true,
	}
	config.deviceConfig = &deviceConfig{
		config: config,
	}
	config.TestProductVariables = &config.productVariables

	config.mockFileSystem(bp, fs)

	if err := config.fromEnv(); err != nil {
		panic(err)
	}

	return Config{config}
}

func TestArchConfigNativeBridge(buildDir string, env map[string]string, bp string, fs map[string][]byte) Config {
	testConfig := TestArchConfig(buildDir, env, bp, fs)
	config := testConfig.config

	config.Targets[Android] = []Target{
		{Android, Arch{ArchType: X86_64, ArchVariant: "silvermont", Abi: []string{"arm64-v8a"}}, NativeBridgeDisabled, "", ""},
		{Android, Arch{ArchType: X86, ArchVariant: "silvermont", Abi: []string{"armeabi-v7a"}}, NativeBridgeDisabled, "", ""},
		{Android, Arch{ArchType: Arm64, ArchVariant: "armv8-a", Abi: []string{"arm64-v8a"}}, NativeBridgeEnabled, "x86_64", "arm64"},
		{Android, Arch{ArchType: Arm, ArchVariant: "armv7-a-neon", Abi: []string{"armeabi-v7a"}}, NativeBridgeEnabled, "x86", "arm"},
	}

	return testConfig
}

func TestArchConfigFuchsia(buildDir string, env map[string]string, bp string, fs map[string][]byte) Config {
	testConfig := TestConfig(buildDir, env, bp, fs)
	config := testConfig.config

	config.Targets = map[OsType][]Target{
		Fuchsia: []Target{
			{Fuchsia, Arch{ArchType: Arm64, ArchVariant: "", Abi: []string{"arm64-v8a"}}, NativeBridgeDisabled, "", ""},
		},
		BuildOs: []Target{
			{BuildOs, Arch{ArchType: X86_64}, NativeBridgeDisabled, "", ""},
		},
	}

	return testConfig
}

// TestConfig returns a Config object suitable for using for tests that need to run the arch mutator
func TestArchConfig(buildDir string, env map[string]string, bp string, fs map[string][]byte) Config {
	testConfig := TestConfig(buildDir, env, bp, fs)
	config := testConfig.config

	config.Targets = map[OsType][]Target{
		Android: []Target{
			{Android, Arch{ArchType: Arm64, ArchVariant: "armv8-a", Abi: []string{"arm64-v8a"}}, NativeBridgeDisabled, "", ""},
			{Android, Arch{ArchType: Arm, ArchVariant: "armv7-a-neon", Abi: []string{"armeabi-v7a"}}, NativeBridgeDisabled, "", ""},
		},
		BuildOs: []Target{
			{BuildOs, Arch{ArchType: X86_64}, NativeBridgeDisabled, "", ""},
			{BuildOs, Arch{ArchType: X86}, NativeBridgeDisabled, "", ""},
		},
	}

	if runtime.GOOS == "darwin" {
		config.Targets[BuildOs] = config.Targets[BuildOs][:1]
	}

	config.BuildOSTarget = config.Targets[BuildOs][0]
	config.BuildOSCommonTarget = getCommonTargets(config.Targets[BuildOs])[0]
	config.AndroidCommonTarget = getCommonTargets(config.Targets[Android])[0]
	config.TestProductVariables.DeviceArch = proptools.StringPtr("arm64")
	config.TestProductVariables.DeviceArchVariant = proptools.StringPtr("armv8-a")
	config.TestProductVariables.DeviceSecondaryArch = proptools.StringPtr("arm")
	config.TestProductVariables.DeviceSecondaryArchVariant = proptools.StringPtr("armv7-a-neon")

	return testConfig
}

// New creates a new Config object.  The srcDir argument specifies the path to
// the root source directory. It also loads the config file, if found.
func NewConfig(srcDir, buildDir string) (Config, error) {
	// Make a config with default options
	config := &config{
		ConfigFileName:           filepath.Join(buildDir, configFileName),
		ProductVariablesFileName: filepath.Join(buildDir, productVariablesFileName),

		env: originalEnv,

		srcDir:            srcDir,
		buildDir:          buildDir,
		multilibConflicts: make(map[ArchType]bool),

		fs: pathtools.NewOsFs(absSrcDir),
	}

	config.deviceConfig = &deviceConfig{
		config: config,
	}

	// Sanity check the build and source directories. This won't catch strange
	// configurations with symlinks, but at least checks the obvious cases.
	absBuildDir, err := filepath.Abs(buildDir)
	if err != nil {
		return Config{}, err
	}

	absSrcDir, err := filepath.Abs(srcDir)
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

	inMakeFile := filepath.Join(buildDir, ".soong.in_make")
	if _, err := os.Stat(absolutePath(inMakeFile)); err == nil {
		config.inMake = true
	}

	targets, err := decodeTargetProductVariables(config)
	if err != nil {
		return Config{}, err
	}

	// Make the CommonOS OsType available for all products.
	targets[CommonOS] = []Target{commonTargetMap[CommonOS.Name]}

	var archConfig []archConfig
	if Bool(config.Mega_device) {
		archConfig = getMegaDeviceConfig()
	} else if config.NdkAbis() {
		archConfig = getNdkAbisConfig()
	} else if config.AmlAbis() {
		archConfig = getAmlAbisConfig()
	}

	if archConfig != nil {
		androidTargets, err := decodeArchSettings(Android, archConfig)
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

	config.Targets = targets
	config.BuildOSTarget = config.Targets[BuildOs][0]
	config.BuildOSCommonTarget = getCommonTargets(config.Targets[BuildOs])[0]
	if len(config.Targets[Android]) > 0 {
		config.AndroidCommonTarget = getCommonTargets(config.Targets[Android])[0]
	}

	if err := config.fromEnv(); err != nil {
		return Config{}, err
	}

	if Bool(config.productVariables.GcovCoverage) && Bool(config.productVariables.ClangCoverage) {
		return Config{}, fmt.Errorf("GcovCoverage and ClangCoverage cannot both be set")
	}

	config.productVariables.Native_coverage = proptools.BoolPtr(
		Bool(config.productVariables.GcovCoverage) ||
			Bool(config.productVariables.ClangCoverage))

	return Config{config}, nil
}

var TestConfigOsFs = map[string][]byte{}

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
		if base == "Blueprints" || base == "Android.bp" {
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

func (c *config) fromEnv() error {
	switch c.Getenv("EXPERIMENTAL_JAVA_LANGUAGE_LEVEL_9") {
	case "", "true":
		// Do nothing
	default:
		return fmt.Errorf("The environment variable EXPERIMENTAL_JAVA_LANGUAGE_LEVEL_9 is no longer supported. Java language level 9 is now the global default.")
	}

	return nil
}

func (c *config) StopBefore() bootstrap.StopBefore {
	return c.stopBefore
}

func (c *config) SetStopBefore(stopBefore bootstrap.StopBefore) {
	c.stopBefore = stopBefore
}

var _ bootstrap.ConfigStopBefore = (*config)(nil)

func (c *config) BlueprintToolLocation() string {
	return filepath.Join(c.buildDir, "host", c.PrebuiltOS(), "bin")
}

var _ bootstrap.ConfigBlueprintToolLocation = (*config)(nil)

func (c *config) HostToolPath(ctx PathContext, tool string) Path {
	return PathForOutput(ctx, "host", c.PrebuiltOS(), "bin", tool)
}

func (c *config) HostJNIToolPath(ctx PathContext, path string) Path {
	ext := ".so"
	if runtime.GOOS == "darwin" {
		ext = ".dylib"
	}
	return PathForOutput(ctx, "host", c.PrebuiltOS(), "lib64", path+ext)
}

func (c *config) HostJavaToolPath(ctx PathContext, path string) Path {
	return PathForOutput(ctx, "host", c.PrebuiltOS(), "framework", path)
}

// HostSystemTool looks for non-hermetic tools from the system we're running on.
// Generally shouldn't be used, but useful to find the XCode SDK, etc.
func (c *config) HostSystemTool(name string) string {
	for _, dir := range filepath.SplitList(c.Getenv("PATH")) {
		path := filepath.Join(dir, name)
		if s, err := os.Stat(path); err != nil {
			continue
		} else if m := s.Mode(); !s.IsDir() && m&0111 != 0 {
			return path
		}
	}
	return name
}

// PrebuiltOS returns the name of the host OS used in prebuilts directories
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
	return fmt.Sprintf("%s/prebuilts/go/%s", c.srcDir, c.PrebuiltOS())
}

func (c *config) PrebuiltBuildTool(ctx PathContext, tool string) Path {
	return PathForSource(ctx, "prebuilts/build-tools", c.PrebuiltOS(), "bin", tool)
}

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

func (c *config) EnvDeps() map[string]string {
	c.envLock.Lock()
	defer c.envLock.Unlock()
	c.envFrozen = true
	return c.envDeps
}

func (c *config) EmbeddedInMake() bool {
	return c.inMake
}

func (c *config) BuildId() string {
	return String(c.productVariables.BuildId)
}

func (c *config) BuildNumberFile(ctx PathContext) Path {
	return PathForOutput(ctx, String(c.productVariables.BuildNumberFile))
}

// DeviceName returns the name of the current device target
// TODO: take an AndroidModuleContext to select the device name for multi-device builds
func (c *config) DeviceName() string {
	return *c.productVariables.DeviceName
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

func (c *config) PlatformSdkVersionInt() int {
	return *c.productVariables.Platform_sdk_version
}

func (c *config) PlatformSdkVersion() string {
	return strconv.Itoa(c.PlatformSdkVersionInt())
}

func (c *config) PlatformSdkCodename() string {
	return String(c.productVariables.Platform_sdk_codename)
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

func (c *config) MinSupportedSdkVersion() int {
	return 16
}

func (c *config) DefaultAppTargetSdkInt() int {
	if Bool(c.productVariables.Platform_sdk_final) {
		return c.PlatformSdkVersionInt()
	} else {
		return FutureApiLevel
	}
}

func (c *config) DefaultAppTargetSdk() string {
	if Bool(c.productVariables.Platform_sdk_final) {
		return c.PlatformSdkVersion()
	} else {
		return c.PlatformSdkCodename()
	}
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
	} else {
		return PathForSource(ctx, "build/make/target/product/security")
	}
}

func (c *config) DefaultAppCertificate(ctx PathContext) (pem, key SourcePath) {
	defaultCert := String(c.productVariables.DefaultAppCertificate)
	if defaultCert != "" {
		return PathForSource(ctx, defaultCert+".x509.pem"), PathForSource(ctx, defaultCert+".pk8")
	} else {
		defaultDir := c.DefaultAppCertificateDir(ctx)
		return defaultDir.Join(ctx, "testkey.x509.pem"), defaultDir.Join(ctx, "testkey.pk8")
	}
}

func (c *config) ApexKeyDir(ctx ModuleContext) SourcePath {
	// TODO(b/121224311): define another variable such as TARGET_APEX_KEY_OVERRIDE
	defaultCert := String(c.productVariables.DefaultAppCertificate)
	if defaultCert == "" || filepath.Dir(defaultCert) == "build/make/target/product/security" {
		// When defaultCert is unset or is set to the testkeys path, use the APEX keys
		// that is under the module dir
		return pathForModuleSrc(ctx)
	} else {
		// If not, APEX keys are under the specified directory
		return PathForSource(ctx, filepath.Dir(defaultCert))
	}
}

func (c *config) AllowMissingDependencies() bool {
	return Bool(c.productVariables.Allow_missing_dependencies)
}

func (c *config) UnbundledBuild() bool {
	return Bool(c.productVariables.Unbundled_build)
}

func (c *config) UnbundledBuildUsePrebuiltSdks() bool {
	return Bool(c.productVariables.Unbundled_build) && !Bool(c.productVariables.Unbundled_build_sdks_from_source)
}

func (c *config) Fuchsia() bool {
	return Bool(c.productVariables.Fuchsia)
}

func (c *config) IsPdkBuild() bool {
	return Bool(c.productVariables.Pdk)
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

func (c *config) DevicePrefer32BitApps() bool {
	return Bool(c.productVariables.DevicePrefer32BitApps)
}

func (c *config) DevicePrefer32BitExecutables() bool {
	return Bool(c.productVariables.DevicePrefer32BitExecutables)
}

func (c *config) DevicePrimaryArchType() ArchType {
	return c.Targets[Android][0].Arch.ArchType
}

func (c *config) SkipMegaDeviceInstall(path string) bool {
	return Bool(c.Mega_device) &&
		strings.HasPrefix(path, filepath.Join(c.buildDir, "target", "product"))
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
	} else {
		return *c.productVariables.EnableCFI
	}
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

func (c *config) XrefCorpusName() string {
	return c.Getenv("XREF_CORPUS")
}

// Returns Compilation Unit encoding to use. Can be 'json' (default), 'proto' or 'all'.
func (c *config) XrefCuEncoding() string {
	if enc := c.Getenv("KYTHE_KZIP_ENCODING"); enc != "" {
		return enc
	}
	return "json"
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
	archType := Common
	if len(c.Targets[Android]) > 0 {
		archType = c.Targets[Android][0].Arch.ArchType
	}
	switch archType {
	default:
		return "0x70000000"
	case Mips, Mips64:
		return "0x5C000000"
	}
}

func (c *config) ArtUseReadBarrier() bool {
	return Bool(c.productVariables.ArtUseReadBarrier)
}

func (c *config) EnforceRROForModule(name string) bool {
	enforceList := c.productVariables.EnforceRROTargets
	// TODO(b/150820813) Some modules depend on static overlay, remove this after eliminating the dependency.
	exemptedList := c.productVariables.EnforceRROExemptedTargets
	if exemptedList != nil {
		if InList(name, exemptedList) {
			return false
		}
	}
	if enforceList != nil {
		if InList("*", enforceList) {
			return true
		}
		return InList(name, enforceList)
	}
	return false
}

func (c *config) EnforceRROExcludedOverlay(path string) bool {
	excluded := c.productVariables.EnforceRROExcludedOverlays
	if excluded != nil {
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

// Expected format for apexJarValue = <apex name>:<jar name>
func SplitApexJarPair(apexJarValue string) (string, string) {
	var apexJarPair []string = strings.SplitN(apexJarValue, ":", 2)
	if apexJarPair == nil || len(apexJarPair) != 2 {
		panic(fmt.Errorf("malformed apexJarValue: %q, expected format: <apex>:<jar>",
			apexJarValue))
	}
	return apexJarPair[0], apexJarPair[1]
}

func (c *config) BootJars() []string {
	jars := c.productVariables.BootJars
	for _, p := range c.productVariables.UpdatableBootJars {
		_, jar := SplitApexJarPair(p)
		jars = append(jars, jar)
	}
	return jars
}

func (c *config) DexpreoptGlobalConfig(ctx PathContext) ([]byte, error) {
	if c.productVariables.DexpreoptGlobalConfig == nil {
		return nil, nil
	}
	path := absolutePath(*c.productVariables.DexpreoptGlobalConfig)
	ctx.AddNinjaFileDeps(path)
	return ioutil.ReadFile(path)
}

func (c *config) FrameworksBaseDirExists(ctx PathContext) bool {
	return ExistentPathForSource(ctx, "frameworks", "base").Valid()
}

func (c *config) VndkSnapshotBuildArtifacts() bool {
	return Bool(c.productVariables.VndkSnapshotBuildArtifacts)
}

func (c *config) HasMultilibConflict(arch ArchType) bool {
	return c.multilibConflicts[arch]
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

func (c *deviceConfig) SamplingPGO() bool {
	return Bool(c.config.productVariables.SamplingPGO)
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
	if coverage && c.config.productVariables.JavaCoverageExcludePaths != nil {
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
	if c.config.productVariables.NativeCoveragePaths != nil {
		if InList("*", c.config.productVariables.NativeCoveragePaths) || HasAnyPrefix(path, c.config.productVariables.NativeCoveragePaths) {
			coverage = true
		}
	}
	if coverage && c.config.productVariables.NativeCoverageExcludePaths != nil {
		if HasAnyPrefix(path, c.config.productVariables.NativeCoverageExcludePaths) {
			coverage = false
		}
	}
	return coverage
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

func (c *deviceConfig) PlatPublicSepolicyDirs() []string {
	return c.config.productVariables.BoardPlatPublicSepolicyDirs
}

func (c *deviceConfig) PlatPrivateSepolicyDirs() []string {
	return c.config.productVariables.BoardPlatPrivateSepolicyDirs
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

func (c *config) IntegerOverflowDisabledForPath(path string) bool {
	if c.productVariables.IntegerOverflowExcludePaths == nil {
		return false
	}
	return HasAnyPrefix(path, c.productVariables.IntegerOverflowExcludePaths)
}

func (c *config) CFIDisabledForPath(path string) bool {
	if c.productVariables.CFIExcludePaths == nil {
		return false
	}
	return HasAnyPrefix(path, c.productVariables.CFIExcludePaths)
}

func (c *config) CFIEnabledForPath(path string) bool {
	if c.productVariables.CFIIncludePaths == nil {
		return false
	}
	return HasAnyPrefix(path, c.productVariables.CFIIncludePaths)
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

func (c *config) ExcludeDraftNdkApis() bool {
	return Bool(c.productVariables.Exclude_draft_ndk_apis)
}

func (c *config) FlattenApex() bool {
	return Bool(c.productVariables.Flatten_apex)
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

func (c *config) ProductCompatibleProperty() bool {
	return Bool(c.productVariables.ProductCompatibleProperty)
}

func (c *config) MissingUsesLibraries() []string {
	return c.productVariables.MissingUsesLibraries
}

func (c *deviceConfig) BoardVndkRuntimeDisable() bool {
	return Bool(c.config.productVariables.BoardVndkRuntimeDisable)
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
