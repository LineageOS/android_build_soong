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

	"github.com/google/blueprint/bootstrap"
	"github.com/google/blueprint/proptools"
)

var Bool = proptools.Bool
var String = proptools.String
var FutureApiLevel = 10000

// The configuration file name
const configFileName = "soong.config"
const productVariablesFileName = "soong.variables"

// A FileConfigurableOptions contains options which can be configured by the
// config file. These will be included in the config struct.
type FileConfigurableOptions struct {
	Mega_device *bool `json:",omitempty"`
	Ndk_abis    *bool `json:",omitempty"`
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

type VendorConfig interface {
	// Bool interprets the variable named `name` as a boolean, returning true if, after
	// lowercasing, it matches one of "1", "y", "yes", "on", or "true". Unset, or any other
	// value will return false.
	Bool(name string) bool

	// String returns the string value of `name`. If the variable was not set, it will
	// return the empty string.
	String(name string) string

	// IsSet returns whether the variable `name` was set by Make.
	IsSet(name string) bool
}

type config struct {
	FileConfigurableOptions
	productVariables productVariables

	// Only available on configs created by TestConfig
	TestProductVariables *productVariables

	PrimaryBuilder           string
	ConfigFileName           string
	ProductVariablesFileName string

	Targets        map[OsClass][]Target
	BuildOsVariant string

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

	useOpenJDK9    bool // Use OpenJDK9, but possibly target 1.8
	targetOpenJDK9 bool // Use OpenJDK9 and target 1.9

	stopBefore bootstrap.StopBefore

	OncePer
}

type deviceConfig struct {
	config *config
	OncePer
}

type vendorConfig map[string]string

type jsonConfigurable interface {
	SetDefaultConfig()
}

func loadConfig(config *config) error {
	err := loadFromConfigFile(&config.FileConfigurableOptions, config.ConfigFileName)
	if err != nil {
		return err
	}

	return loadFromConfigFile(&config.productVariables, config.ProductVariablesFileName)
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

// TestConfig returns a Config object suitable for using for tests
func TestConfig(buildDir string, env map[string]string) Config {
	config := &config{
		productVariables: productVariables{
			DeviceName:           stringPtr("test_device"),
			Platform_sdk_version: intPtr(26),
			AAPTConfig:           &[]string{"normal", "large", "xlarge", "hdpi", "xhdpi", "xxhdpi"},
			AAPTPreferredConfig:  stringPtr("xhdpi"),
			AAPTCharacteristics:  stringPtr("nosdcard"),
			AAPTPrebuiltDPI:      &[]string{"xhdpi", "xxhdpi"},
		},

		buildDir:     buildDir,
		captureBuild: true,
		env:          env,
	}
	config.deviceConfig = &deviceConfig{
		config: config,
	}
	config.TestProductVariables = &config.productVariables

	if err := config.fromEnv(); err != nil {
		panic(err)
	}

	return Config{config}
}

// TestConfig returns a Config object suitable for using for tests that need to run the arch mutator
func TestArchConfig(buildDir string, env map[string]string) Config {
	testConfig := TestConfig(buildDir, env)
	config := testConfig.config

	config.Targets = map[OsClass][]Target{
		Device: []Target{
			{Android, Arch{ArchType: Arm64, ArchVariant: "armv8-a", Native: true}},
			{Android, Arch{ArchType: Arm, ArchVariant: "armv7-a-neon", Native: true}},
		},
		Host: []Target{
			{BuildOs, Arch{ArchType: X86_64}},
			{BuildOs, Arch{ArchType: X86}},
		},
	}

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

		srcDir:   srcDir,
		buildDir: buildDir,
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
	if _, err := os.Stat(inMakeFile); err == nil {
		config.inMake = true
	}

	targets, err := decodeTargetProductVariables(config)
	if err != nil {
		return Config{}, err
	}

	var archConfig []archConfig
	if Bool(config.Mega_device) {
		archConfig = getMegaDeviceConfig()
	} else if Bool(config.Ndk_abis) {
		archConfig = getNdkAbisConfig()
	}

	if archConfig != nil {
		deviceTargets, err := decodeArchSettings(archConfig)
		if err != nil {
			return Config{}, err
		}
		targets[Device] = deviceTargets
	}

	config.Targets = targets
	config.BuildOsVariant = targets[Host][0].String()

	if err := config.fromEnv(); err != nil {
		return Config{}, err
	}

	return Config{config}, nil
}

func (c *config) fromEnv() error {
	switch c.Getenv("EXPERIMENTAL_USE_OPENJDK9") {
	case "":
		if c.Getenv("RUN_ERROR_PRONE") != "true" {
			// Use OpenJDK9, but target 1.8
			c.useOpenJDK9 = true
		}
	case "false":
		// Use OpenJDK8
	case "1.8":
		// Use OpenJDK9, but target 1.8
		c.useOpenJDK9 = true
	case "true":
		// Use OpenJDK9 and target 1.9
		c.useOpenJDK9 = true
		c.targetOpenJDK9 = true
	default:
		return fmt.Errorf(`Invalid value for EXPERIMENTAL_USE_OPENJDK9, should be "", "false", "1.8", or "true"`)
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

func (c *config) BuildNumberFromFile() string {
	return String(c.productVariables.BuildNumberFromFile)
}

// DeviceName returns the name of the current device target
// TODO: take an AndroidModuleContext to select the device name for multi-device builds
func (c *config) DeviceName() string {
	return *c.productVariables.DeviceName
}

func (c *config) ResourceOverlays() []string {
	if c.productVariables.ResourceOverlays == nil {
		return nil
	}
	return *c.productVariables.ResourceOverlays
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

func (c *config) MinSupportedSdkVersion() int {
	return 14
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

// Codenames that are available in the branch but not included in the current
// lunch target.
func (c *config) PlatformVersionFutureCodenames() []string {
	return c.productVariables.Platform_version_future_codenames
}

// All possible codenames in the current branch. NB: Not named AllCodenames
// because "all" has historically meant "active" in make, and still does in
// build.prop.
func (c *config) PlatformVersionCombinedCodenames() []string {
	combined := []string{}
	combined = append(combined, c.PlatformVersionActiveCodenames()...)
	combined = append(combined, c.PlatformVersionFutureCodenames()...)
	return combined
}

func (c *config) ProductAAPTConfig() []string {
	return stringSlice(c.productVariables.AAPTConfig)
}

func (c *config) ProductAAPTPreferredConfig() string {
	return String(c.productVariables.AAPTPreferredConfig)
}

func (c *config) ProductAAPTCharacteristics() string {
	return String(c.productVariables.AAPTCharacteristics)
}

func (c *config) ProductAAPTPrebuiltDPI() []string {
	return stringSlice(c.productVariables.AAPTPrebuiltDPI)
}

func (c *config) DefaultAppCertificateDir(ctx PathContext) SourcePath {
	defaultCert := String(c.productVariables.DefaultAppCertificate)
	if defaultCert != "" {
		return PathForSource(ctx, filepath.Dir(defaultCert))
	} else {
		return PathForSource(ctx, "build/target/product/security")
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

func (c *config) AllowMissingDependencies() bool {
	return Bool(c.productVariables.Allow_missing_dependencies)
}

func (c *config) UnbundledBuild() bool {
	return Bool(c.productVariables.Unbundled_build)
}

func (c *config) IsPdkBuild() bool {
	return Bool(c.productVariables.Pdk)
}

func (c *config) MinimizeJavaDebugInfo() bool {
	return Bool(c.productVariables.MinimizeJavaDebugInfo) && !Bool(c.productVariables.Eng)
}

func (c *config) DevicePrefer32BitExecutables() bool {
	return Bool(c.productVariables.DevicePrefer32BitExecutables)
}

func (c *config) SkipDeviceInstall() bool {
	return c.EmbeddedInMake()
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

func (c *config) Android64() bool {
	for _, t := range c.Targets[Device] {
		if t.Arch.ArchType.Multilib == "lib64" {
			return true
		}
	}

	return false
}

func (c *config) UseD8Desugar() bool {
	return !c.IsEnvFalse("USE_D8_DESUGAR")
}

func (c *config) UseGoma() bool {
	return Bool(c.productVariables.UseGoma)
}

// Returns true if OpenJDK9 prebuilts are being used
func (c *config) UseOpenJDK9() bool {
	return c.useOpenJDK9
}

// Returns true if -source 1.9 -target 1.9 is being passed to javac
func (c *config) TargetOpenJDK9() bool {
	return c.targetOpenJDK9
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
	if len(c.Targets[Device]) > 0 {
		archType = c.Targets[Device][0].Arch.ArchType
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
	if enforceList != nil {
		if len(*enforceList) == 1 && (*enforceList)[0] == "*" {
			return true
		}
		return InList(name, *enforceList)
	}
	return false
}

func (c *config) EnforceRROExcludedOverlay(path string) bool {
	excluded := c.productVariables.EnforceRROExcludedOverlays
	if excluded != nil {
		for _, exclude := range *excluded {
			if strings.HasPrefix(path, exclude) {
				return true
			}
		}
	}
	return false
}

func (c *config) ExportedNamespaces() []string {
	return append([]string(nil), c.productVariables.NamespacesToExport...)
}

func (c *config) HostStaticBinaries() bool {
	return Bool(c.productVariables.HostStaticBinaries)
}

func (c *deviceConfig) Arches() []Arch {
	var arches []Arch
	for _, target := range c.config.Targets[Device] {
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

func (c *deviceConfig) ExtraVndkVersions() []string {
	return c.config.productVariables.ExtraVndkVersions
}

func (c *deviceConfig) SystemSdkVersions() []string {
	if c.config.productVariables.DeviceSystemSdkVersions == nil {
		return nil
	}
	return *c.config.productVariables.DeviceSystemSdkVersions
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

func (c *deviceConfig) BtConfigIncludeDir() string {
	return String(c.config.productVariables.BtConfigIncludeDir)
}

func (c *deviceConfig) DeviceKernelHeaderDirs() []string {
	return c.config.productVariables.DeviceKernelHeaders
}

func (c *deviceConfig) NativeCoverageEnabled() bool {
	return Bool(c.config.productVariables.NativeCoverage)
}

func (c *deviceConfig) CoverageEnabledForPath(path string) bool {
	coverage := false
	if c.config.productVariables.CoveragePaths != nil {
		if PrefixInList(path, *c.config.productVariables.CoveragePaths) {
			coverage = true
		}
	}
	if coverage && c.config.productVariables.CoverageExcludePaths != nil {
		if PrefixInList(path, *c.config.productVariables.CoverageExcludePaths) {
			coverage = false
		}
	}
	return coverage
}

func (c *deviceConfig) PgoAdditionalProfileDirs() []string {
	return c.config.productVariables.PgoAdditionalProfileDirs
}

func (c *config) IntegerOverflowDisabledForPath(path string) bool {
	if c.productVariables.IntegerOverflowExcludePaths == nil {
		return false
	}
	return PrefixInList(path, *c.productVariables.IntegerOverflowExcludePaths)
}

func (c *config) CFIDisabledForPath(path string) bool {
	if c.productVariables.CFIExcludePaths == nil {
		return false
	}
	return PrefixInList(path, *c.productVariables.CFIExcludePaths)
}

func (c *config) CFIEnabledForPath(path string) bool {
	if c.productVariables.CFIIncludePaths == nil {
		return false
	}
	return PrefixInList(path, *c.productVariables.CFIIncludePaths)
}

func (c *config) VendorConfig(name string) VendorConfig {
	return vendorConfig(c.productVariables.VendorVars[name])
}

func (c vendorConfig) Bool(name string) bool {
	v := strings.ToLower(c[name])
	return v == "1" || v == "y" || v == "yes" || v == "on" || v == "true"
}

func (c vendorConfig) String(name string) string {
	return c[name]
}

func (c vendorConfig) IsSet(name string) bool {
	_, ok := c[name]
	return ok
}

func stringSlice(s *[]string) []string {
	if s != nil {
		return *s
	} else {
		return nil
	}
}
