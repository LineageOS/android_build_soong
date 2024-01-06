// Copyright 2022 Google Inc. All rights reserved.
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
	"os"
	"path/filepath"
	"runtime"

	"github.com/google/blueprint/proptools"
)

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
		productVariables: ProductVariables{
			DeviceName:                          stringPtr("test_device"),
			DeviceProduct:                       stringPtr("test_product"),
			Platform_sdk_version:                intPtr(30),
			Platform_sdk_version_or_codename:    stringPtr("S"),
			Platform_sdk_codename:               stringPtr("S"),
			Platform_base_sdk_extension_version: intPtr(1),
			Platform_version_active_codenames:   []string{"S", "Tiramisu"},
			DeviceSystemSdkVersions:             []string{"29", "30", "S"},
			Platform_systemsdk_versions:         []string{"29", "30", "S", "Tiramisu"},
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

		BuildMode: AnalysisNoBazel,
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

	// Make the CommonOS OsType available for all products.
	config.Targets[CommonOS] = []Target{commonTargetMap[CommonOS.Name]}

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

// ModifyTestConfigForMusl takes a Config returned by TestConfig and changes the host targets from glibc to musl.
func ModifyTestConfigForMusl(config Config) {
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

func modifyTestConfigForMuslArm64HostCross(config Config) {
	config.Targets[LinuxMusl] = append(config.Targets[LinuxMusl],
		Target{config.BuildOS, Arch{ArchType: Arm64}, NativeBridgeDisabled, "", "", true})
}

// TestArchConfig returns a Config object suitable for using for tests that
// need to run the arch mutator.
func TestArchConfig(buildDir string, env map[string]string, bp string, fs map[string][]byte) Config {
	testConfig := TestConfig(buildDir, env, bp, fs)
	modifyTestConfigToSupportArchMutator(testConfig)
	return testConfig
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
