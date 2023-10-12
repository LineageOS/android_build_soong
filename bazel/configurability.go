// Copyright 2021 Google Inc. All rights reserved.
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

package bazel

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	// ArchType names in arch.go
	archArm     = "arm"
	archArm64   = "arm64"
	archRiscv64 = "riscv64"
	archX86     = "x86"
	archX86_64  = "x86_64"

	// OsType names in arch.go
	OsAndroid     = "android"
	OsDarwin      = "darwin"
	OsLinux       = "linux_glibc"
	osLinuxMusl   = "linux_musl"
	osLinuxBionic = "linux_bionic"
	OsWindows     = "windows"

	// Targets in arch.go
	osArchAndroidArm        = "android_arm"
	OsArchAndroidArm64      = "android_arm64"
	osArchAndroidRiscv64    = "android_riscv64"
	osArchAndroidX86        = "android_x86"
	osArchAndroidX86_64     = "android_x86_64"
	osArchDarwinArm64       = "darwin_arm64"
	osArchDarwinX86_64      = "darwin_x86_64"
	osArchLinuxX86          = "linux_glibc_x86"
	osArchLinuxX86_64       = "linux_glibc_x86_64"
	osArchLinuxMuslArm      = "linux_musl_arm"
	osArchLinuxMuslArm64    = "linux_musl_arm64"
	osArchLinuxMuslX86      = "linux_musl_x86"
	osArchLinuxMuslX86_64   = "linux_musl_x86_64"
	osArchLinuxBionicArm64  = "linux_bionic_arm64"
	osArchLinuxBionicX86_64 = "linux_bionic_x86_64"
	osArchWindowsX86        = "windows_x86"
	osArchWindowsX86_64     = "windows_x86_64"

	// This is the string representation of the default condition wherever a
	// configurable attribute is used in a select statement, i.e.
	// //conditions:default for Bazel.
	//
	// This is consistently named "conditions_default" to mirror the Soong
	// config variable default key in an Android.bp file, although there's no
	// integration with Soong config variables (yet).
	ConditionsDefaultConfigKey = "conditions_default"

	ConditionsDefaultSelectKey = "//conditions:default"

	productVariableBazelPackage = "//build/bazel/product_config/config_settings"

	AndroidAndInApex = "android-in_apex"
	AndroidPlatform  = "system"
	Unbundled_app    = "unbundled_app"

	InApex  = "in_apex"
	NonApex = "non_apex"

	ErrorproneDisabled = "errorprone_disabled"
	// TODO: b/294868620 - Remove when completing the bug
	SanitizersEnabled = "sanitizers_enabled"
)

func PowerSetWithoutEmptySet[T any](items []T) [][]T {
	resultSize := int(math.Pow(2, float64(len(items))))
	powerSet := make([][]T, 0, resultSize-1)
	for i := 1; i < resultSize; i++ {
		combination := make([]T, 0)
		for j := 0; j < len(items); j++ {
			if (i>>j)%2 == 1 {
				combination = append(combination, items[j])
			}
		}
		powerSet = append(powerSet, combination)
	}
	return powerSet
}

func createPlatformArchMap() map[string]string {
	// Copy of archFeatures from android/arch_list.go because the bazel
	// package can't access the android package
	archFeatures := map[string][]string{
		"arm": {
			"neon",
		},
		"arm64": {
			"dotprod",
		},
		"riscv64": {},
		"x86": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"avx",
			"avx2",
			"avx512",
			"popcnt",
			"movbe",
		},
		"x86_64": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"avx",
			"avx2",
			"avx512",
			"popcnt",
		},
	}
	result := make(map[string]string)
	for arch, allFeatures := range archFeatures {
		result[arch] = "//build/bazel_common_rules/platforms/arch:" + arch
		// Sometimes we want to select on multiple features being active, so
		// add the power set of all possible features to the map. More details
		// in android.ModuleBase.GetArchVariantProperties
		for _, features := range PowerSetWithoutEmptySet(allFeatures) {
			sort.Strings(features)
			archFeaturesName := arch + "-" + strings.Join(features, "-")
			result[archFeaturesName] = "//build/bazel/platforms/arch/variants:" + archFeaturesName
		}
	}
	result[ConditionsDefaultConfigKey] = ConditionsDefaultSelectKey
	return result
}

var (
	// These are the list of OSes and architectures with a Bazel config_setting
	// and constraint value equivalent. These exist in arch.go, but the android
	// package depends on the bazel package, so a cyclic dependency prevents
	// using those variables here.

	// A map of architectures to the Bazel label of the constraint_value
	// for the @platforms//cpu:cpu constraint_setting
	platformArchMap = createPlatformArchMap()

	// A map of target operating systems to the Bazel label of the
	// constraint_value for the @platforms//os:os constraint_setting
	platformOsMap = map[string]string{
		OsAndroid:                  "//build/bazel_common_rules/platforms/os:android",
		OsDarwin:                   "//build/bazel_common_rules/platforms/os:darwin",
		OsLinux:                    "//build/bazel_common_rules/platforms/os:linux_glibc",
		osLinuxMusl:                "//build/bazel_common_rules/platforms/os:linux_musl",
		osLinuxBionic:              "//build/bazel_common_rules/platforms/os:linux_bionic",
		OsWindows:                  "//build/bazel_common_rules/platforms/os:windows",
		ConditionsDefaultConfigKey: ConditionsDefaultSelectKey, // The default condition of an os select map.
	}

	platformOsArchMap = map[string]string{
		osArchAndroidArm:           "//build/bazel_common_rules/platforms/os_arch:android_arm",
		OsArchAndroidArm64:         "//build/bazel_common_rules/platforms/os_arch:android_arm64",
		osArchAndroidRiscv64:       "//build/bazel_common_rules/platforms/os_arch:android_riscv64",
		osArchAndroidX86:           "//build/bazel_common_rules/platforms/os_arch:android_x86",
		osArchAndroidX86_64:        "//build/bazel_common_rules/platforms/os_arch:android_x86_64",
		osArchDarwinArm64:          "//build/bazel_common_rules/platforms/os_arch:darwin_arm64",
		osArchDarwinX86_64:         "//build/bazel_common_rules/platforms/os_arch:darwin_x86_64",
		osArchLinuxX86:             "//build/bazel_common_rules/platforms/os_arch:linux_glibc_x86",
		osArchLinuxX86_64:          "//build/bazel_common_rules/platforms/os_arch:linux_glibc_x86_64",
		osArchLinuxMuslArm:         "//build/bazel_common_rules/platforms/os_arch:linux_musl_arm",
		osArchLinuxMuslArm64:       "//build/bazel_common_rules/platforms/os_arch:linux_musl_arm64",
		osArchLinuxMuslX86:         "//build/bazel_common_rules/platforms/os_arch:linux_musl_x86",
		osArchLinuxMuslX86_64:      "//build/bazel_common_rules/platforms/os_arch:linux_musl_x86_64",
		osArchLinuxBionicArm64:     "//build/bazel_common_rules/platforms/os_arch:linux_bionic_arm64",
		osArchLinuxBionicX86_64:    "//build/bazel_common_rules/platforms/os_arch:linux_bionic_x86_64",
		osArchWindowsX86:           "//build/bazel_common_rules/platforms/os_arch:windows_x86",
		osArchWindowsX86_64:        "//build/bazel_common_rules/platforms/os_arch:windows_x86_64",
		ConditionsDefaultConfigKey: ConditionsDefaultSelectKey, // The default condition of an os select map.
	}

	// Map where keys are OsType names, and values are slices containing the archs
	// that that OS supports.
	// These definitions copied from arch.go.
	// TODO(cparsons): Source from arch.go; this task is nontrivial, as it currently results
	// in a cyclic dependency.
	osToArchMap = map[string][]string{
		OsAndroid:     {archArm, archArm64, archRiscv64, archX86, archX86_64},
		OsLinux:       {archX86, archX86_64},
		osLinuxMusl:   {archX86, archX86_64},
		OsDarwin:      {archArm64, archX86_64},
		osLinuxBionic: {archArm64, archX86_64},
		// TODO(cparsons): According to arch.go, this should contain archArm, archArm64, as well.
		OsWindows: {archX86, archX86_64},
	}

	osAndInApexMap = map[string]string{
		AndroidAndInApex:           "//build/bazel/rules/apex:android-in_apex",
		AndroidPlatform:            "//build/bazel/rules/apex:system",
		Unbundled_app:              "//build/bazel/rules/apex:unbundled_app",
		OsDarwin:                   "//build/bazel_common_rules/platforms/os:darwin",
		OsLinux:                    "//build/bazel_common_rules/platforms/os:linux_glibc",
		osLinuxMusl:                "//build/bazel_common_rules/platforms/os:linux_musl",
		osLinuxBionic:              "//build/bazel_common_rules/platforms/os:linux_bionic",
		OsWindows:                  "//build/bazel_common_rules/platforms/os:windows",
		ConditionsDefaultConfigKey: ConditionsDefaultSelectKey,
	}

	inApexMap = map[string]string{
		InApex:                     "//build/bazel/rules/apex:in_apex",
		NonApex:                    "//build/bazel/rules/apex:non_apex",
		ConditionsDefaultConfigKey: ConditionsDefaultSelectKey,
	}

	errorProneMap = map[string]string{
		ErrorproneDisabled:         "//build/bazel/rules/java/errorprone:errorprone_globally_disabled",
		ConditionsDefaultConfigKey: ConditionsDefaultSelectKey,
	}

	// TODO: b/294868620 - Remove when completing the bug
	sanitizersEnabledMap = map[string]string{
		SanitizersEnabled:          "//build/bazel/rules/cc:sanitizers_enabled",
		ConditionsDefaultConfigKey: ConditionsDefaultSelectKey,
	}
)

// basic configuration types
type configurationType int

const (
	noConfig configurationType = iota
	arch
	os
	osArch
	productVariables
	osAndInApex
	inApex
	errorProneDisabled
	// TODO: b/294868620 - Remove when completing the bug
	sanitizersEnabled
)

func osArchString(os string, arch string) string {
	return fmt.Sprintf("%s_%s", os, arch)
}

func (ct configurationType) String() string {
	return map[configurationType]string{
		noConfig:           "no_config",
		arch:               "arch",
		os:                 "os",
		osArch:             "arch_os",
		productVariables:   "product_variables",
		osAndInApex:        "os_in_apex",
		inApex:             "in_apex",
		errorProneDisabled: "errorprone_disabled",
		// TODO: b/294868620 - Remove when completing the bug
		sanitizersEnabled: "sanitizers_enabled",
	}[ct]
}

func (ct configurationType) validateConfig(config string) {
	switch ct {
	case noConfig:
		if config != "" {
			panic(fmt.Errorf("Cannot specify config with %s, but got %s", ct, config))
		}
	case arch:
		if _, ok := platformArchMap[config]; !ok {
			panic(fmt.Errorf("Unknown arch: %s", config))
		}
	case os:
		if _, ok := platformOsMap[config]; !ok {
			panic(fmt.Errorf("Unknown os: %s", config))
		}
	case osArch:
		if _, ok := platformOsArchMap[config]; !ok {
			panic(fmt.Errorf("Unknown os+arch: %s", config))
		}
	case productVariables:
		// do nothing
	case osAndInApex:
		// do nothing
		// this axis can contain additional per-apex keys
	case inApex:
		if _, ok := inApexMap[config]; !ok {
			panic(fmt.Errorf("Unknown in_apex config: %s", config))
		}
	case errorProneDisabled:
		if _, ok := errorProneMap[config]; !ok {
			panic(fmt.Errorf("Unknown errorprone config: %s", config))
		}
	// TODO: b/294868620 - Remove when completing the bug
	case sanitizersEnabled:
		if _, ok := sanitizersEnabledMap[config]; !ok {
			panic(fmt.Errorf("Unknown sanitizers_enabled config: %s", config))
		}
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationType %d", ct))
	}
}

// SelectKey returns the Bazel select key for a given configurationType and config string.
func (ca ConfigurationAxis) SelectKey(config string) string {
	ca.validateConfig(config)
	switch ca.configurationType {
	case noConfig:
		panic(fmt.Errorf("SelectKey is unnecessary for noConfig ConfigurationType "))
	case arch:
		return platformArchMap[config]
	case os:
		return platformOsMap[config]
	case osArch:
		return platformOsArchMap[config]
	case productVariables:
		if config == ConditionsDefaultConfigKey {
			return ConditionsDefaultSelectKey
		}
		return fmt.Sprintf("%s:%s", productVariableBazelPackage, config)
	case osAndInApex:
		if ret, exists := osAndInApexMap[config]; exists {
			return ret
		}
		return config
	case inApex:
		return inApexMap[config]
	case errorProneDisabled:
		return errorProneMap[config]
	// TODO: b/294868620 - Remove when completing the bug
	case sanitizersEnabled:
		return sanitizersEnabledMap[config]
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationType %d", ca.configurationType))
	}
}

var (
	// Indicating there is no configuration axis
	NoConfigAxis = ConfigurationAxis{configurationType: noConfig}
	// An axis for architecture-specific configurations
	ArchConfigurationAxis = ConfigurationAxis{configurationType: arch}
	// An axis for os-specific configurations
	OsConfigurationAxis = ConfigurationAxis{configurationType: os}
	// An axis for arch+os-specific configurations
	OsArchConfigurationAxis = ConfigurationAxis{configurationType: osArch}
	// An axis for os+in_apex-specific configurations
	OsAndInApexAxis = ConfigurationAxis{configurationType: osAndInApex}
	// An axis for in_apex-specific configurations
	InApexAxis = ConfigurationAxis{configurationType: inApex}

	ErrorProneAxis = ConfigurationAxis{configurationType: errorProneDisabled}

	// TODO: b/294868620 - Remove when completing the bug
	SanitizersEnabledAxis = ConfigurationAxis{configurationType: sanitizersEnabled}
)

// ProductVariableConfigurationAxis returns an axis for the given product variable
func ProductVariableConfigurationAxis(archVariant bool, variable string) ConfigurationAxis {
	return ConfigurationAxis{
		configurationType: productVariables,
		subType:           variable,
		archVariant:       archVariant,
	}
}

// ConfigurationAxis is an independent axis for configuration, there should be no overlap between
// elements within an axis.
type ConfigurationAxis struct {
	configurationType
	// some configuration types (e.g. productVariables) have multiple independent axes, subType helps
	// distinguish between them without needing to list all 17 product variables.
	subType string

	archVariant bool
}

func (ca *ConfigurationAxis) less(other ConfigurationAxis) bool {
	if ca.configurationType == other.configurationType {
		return ca.subType < other.subType
	}
	return ca.configurationType < other.configurationType
}
