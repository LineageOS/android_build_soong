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

// Testing support for dexpreopt config.
//
// The bootImageConfig/bootImageVariant structs returned by genBootImageConfigs are used in many
// places in the build and are currently mutated in a number of those locations. This provides
// comprehensive tests of the fields in those structs to ensure that they have been initialized
// correctly and where relevant, mutated correctly.
//
// This is used in TestBootImageConfig to verify that the

package java

import (
	"fmt"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/dexpreopt"
)

// PrepareForBootImageConfigTest is the minimal set of preparers that are needed to be able to use
// the Check*BootImageConfig methods define here.
var PrepareForBootImageConfigTest = android.GroupFixturePreparers(
	android.PrepareForTestWithArchMutator,
	android.PrepareForTestAccessingMakeVars,
	PrepareForTestWithDexpreopt,
	FixtureConfigureBootJars("com.android.art:core1", "com.android.art:core2", "platform:framework"),
	dexpreopt.FixtureSetTestOnlyArtBootImageJars("com.android.art:core1", "com.android.art:core2", "platform:extra1"),
	android.FixtureAddTextFile("extra1/Android.bp", `
		java_library {
			name: "extra1",
			srcs: ["extra1.java"],
			installable: true,
		}
	`),
	android.FixtureAddFile("extra1/extra1.java", nil),
)

var PrepareApexBootJarConfigs = FixtureConfigureApexBootJars(
	"com.android.foo:framework-foo", "com.android.bar:framework-bar")

var PrepareApexBootJarConfigsAndModules = android.GroupFixturePreparers(
	PrepareApexBootJarConfigs,
	PrepareApexBootJarModule("com.android.foo", "framework-foo"),
	PrepareApexBootJarModule("com.android.bar", "framework-bar"),
)

var ApexBootJarFragmentsForPlatformBootclasspath = fmt.Sprintf(`
	{
		apex: "%[1]s",
		module: "%[1]s-bootclasspath-fragment",
	},
	{
		apex: "%[2]s",
		module: "%[2]s-bootclasspath-fragment",
	},
`, "com.android.foo", "com.android.bar")

var ApexBootJarDexJarPaths = []string{
	"out/soong/.intermediates/packages/modules/com.android.bar/framework-bar/android_common_apex10000/aligned/framework-bar.jar",
	"out/soong/.intermediates/packages/modules/com.android.foo/framework-foo/android_common_apex10000/aligned/framework-foo.jar",
}

func PrepareApexBootJarModule(apexName string, moduleName string) android.FixturePreparer {
	moduleSourceDir := fmt.Sprintf("packages/modules/%s", apexName)
	fragmentName := apexName + "-bootclasspath-fragment"
	imageNameProp := ""
	if apexName == "com.android.art" {
		fragmentName = "art-bootclasspath-fragment"
		imageNameProp = `image_name: "art",`
	}

	return android.GroupFixturePreparers(
		android.FixtureAddTextFile(moduleSourceDir+"/Android.bp", fmt.Sprintf(`
			apex {
				name: "%[1]s",
				key: "%[1]s.key",
				bootclasspath_fragments: [
					"%[3]s",
				],
				updatable: false,
			}

			apex_key {
				name: "%[1]s.key",
				public_key: "%[1]s.avbpubkey",
				private_key: "%[1]s.pem",
			}

			bootclasspath_fragment {
				name: "%[3]s",
				%[4]s
				contents: ["%[2]s"],
				apex_available: ["%[1]s"],
				hidden_api: {
					split_packages: ["*"],
				},
			}

			java_library {
				name: "%[2]s",
				srcs: ["%[2]s.java"],
				system_modules: "none",
				sdk_version: "none",
				compile_dex: true,
				apex_available: ["%[1]s"],
			}
		`, apexName, moduleName, fragmentName, imageNameProp)),
		android.FixtureMergeMockFs(android.MockFS{
			fmt.Sprintf("%s/apex_manifest.json", moduleSourceDir):          nil,
			fmt.Sprintf("%s/%s.avbpubkey", moduleSourceDir, apexName):      nil,
			fmt.Sprintf("%s/%s.pem", moduleSourceDir, apexName):            nil,
			fmt.Sprintf("system/sepolicy/apex/%s-file_contexts", apexName): nil,
			fmt.Sprintf("%s/%s.java", moduleSourceDir, moduleName):         nil,
		}),
	)
}

// normalizedInstall represents a android.RuleBuilderInstall that has been normalized to remove
// test specific parts of the From path.
type normalizedInstall struct {
	from string
	to   string
}

// normalizeInstalls converts a slice of android.RuleBuilderInstall into a slice of
// normalizedInstall to allow them to be compared using android.AssertDeepEquals.
func normalizeInstalls(installs android.RuleBuilderInstalls) []normalizedInstall {
	var normalized []normalizedInstall
	for _, install := range installs {
		normalized = append(normalized, normalizedInstall{
			from: install.From.RelativeToTop().String(),
			to:   install.To,
		})
	}
	return normalized
}

// assertInstallsEqual normalized the android.RuleBuilderInstalls and compares against the expected
// normalizedInstalls.
func assertInstallsEqual(t *testing.T, message string, expected []normalizedInstall, actual android.RuleBuilderInstalls) {
	t.Helper()
	normalizedActual := normalizeInstalls(actual)
	android.AssertDeepEquals(t, message, expected, normalizedActual)
}

// expectedConfig encapsulates the expected properties that will be set in a bootImageConfig
//
// Each field <x> in here is compared against the corresponding field <x> in bootImageConfig.
type expectedConfig struct {
	name                     string
	stem                     string
	dir                      string
	symbolsDir               string
	installDir               string
	profileInstallPathInApex string
	modules                  android.ConfiguredJarList
	dexPaths                 []string
	dexPathsDeps             []string
	zip                      string
	variants                 []*expectedVariant

	// Mutated fields
	profileInstalls            []normalizedInstall
	profileLicenseMetadataFile string
}

// expectedVariant encapsulates the expected properties that will be set in a bootImageVariant
//
// Each field <x> in here is compared against the corresponding field <x> in bootImageVariant
// except for archType which is compared against the target.Arch.ArchType field in bootImageVariant.
type expectedVariant struct {
	archType          android.ArchType
	dexLocations      []string
	dexLocationsDeps  []string
	imagePathOnHost   string
	imagePathOnDevice string
	imagesDeps        []string
	baseImages        []string
	baseImagesDeps    []string

	// Mutated fields
	installs            []normalizedInstall
	vdexInstalls        []normalizedInstall
	unstrippedInstalls  []normalizedInstall
	licenseMetadataFile string
}

// CheckArtBootImageConfig checks the status of the fields of the bootImageConfig and
// bootImageVariant structures that are returned from artBootImageConfig.
//
// This is before any fields are mutated.
func CheckArtBootImageConfig(t *testing.T, result *android.TestResult) {
	checkArtBootImageConfig(t, result, false, "")
}

// getArtImageConfig gets the ART bootImageConfig that was created during the test.
func getArtImageConfig(result *android.TestResult) *bootImageConfig {
	pathCtx := &android.TestPathContext{TestResult: result}
	imageConfig := genBootImageConfigs(pathCtx)["art"]
	return imageConfig
}

// checkArtBootImageConfig checks the ART boot image.
//
// mutated is true if this is called after fields in the image have been mutated by the ART
// bootclasspath_fragment and false otherwise.
func checkArtBootImageConfig(t *testing.T, result *android.TestResult, mutated bool, expectedLicenseMetadataFile string) {
	imageConfig := getArtImageConfig(result)

	expected := &expectedConfig{
		name:                     "art",
		stem:                     "boot",
		dir:                      "out/soong/dexpreopt_arm64/dex_artjars",
		symbolsDir:               "out/soong/dexpreopt_arm64/dex_artjars_unstripped",
		installDir:               "apex/art_boot_images/javalib",
		profileInstallPathInApex: "etc/boot-image.prof",
		modules:                  android.CreateTestConfiguredJarList([]string{"com.android.art:core1", "com.android.art:core2", "platform:extra1"}),
		dexPaths:                 []string{"out/soong/dexpreopt_arm64/dex_artjars_input/core1.jar", "out/soong/dexpreopt_arm64/dex_artjars_input/core2.jar", "out/soong/dexpreopt_arm64/dex_artjars_input/extra1.jar"},
		dexPathsDeps:             []string{"out/soong/dexpreopt_arm64/dex_artjars_input/core1.jar", "out/soong/dexpreopt_arm64/dex_artjars_input/core2.jar", "out/soong/dexpreopt_arm64/dex_artjars_input/extra1.jar"},
		zip:                      "out/soong/dexpreopt_arm64/dex_artjars/art.zip",
		variants: []*expectedVariant{
			{
				archType:          android.Arm64,
				dexLocations:      []string{"/apex/com.android.art/javalib/core1.jar", "/apex/com.android.art/javalib/core2.jar", "/system/framework/extra1.jar"},
				dexLocationsDeps:  []string{"/apex/com.android.art/javalib/core1.jar", "/apex/com.android.art/javalib/core2.jar", "/system/framework/extra1.jar"},
				imagePathOnHost:   "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.art",
				imagePathOnDevice: "/apex/art_boot_images/javalib/arm64/boot.art",
				imagesDeps: []string{
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.art",
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.oat",
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.vdex",
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-core2.art",
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-core2.oat",
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-core2.vdex",
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-extra1.art",
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-extra1.oat",
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-extra1.vdex",
				},
				installs: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.art",
						to:   "/apex/art_boot_images/javalib/arm64/boot.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.oat",
						to:   "/apex/art_boot_images/javalib/arm64/boot.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-core2.art",
						to:   "/apex/art_boot_images/javalib/arm64/boot-core2.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-core2.oat",
						to:   "/apex/art_boot_images/javalib/arm64/boot-core2.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-extra1.art",
						to:   "/apex/art_boot_images/javalib/arm64/boot-extra1.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-extra1.oat",
						to:   "/apex/art_boot_images/javalib/arm64/boot-extra1.oat",
					},
				},
				vdexInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.vdex",
						to:   "/apex/art_boot_images/javalib/arm64/boot.vdex",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-core2.vdex",
						to:   "/apex/art_boot_images/javalib/arm64/boot-core2.vdex",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-extra1.vdex",
						to:   "/apex/art_boot_images/javalib/arm64/boot-extra1.vdex",
					},
				},
				unstrippedInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars_unstripped/android/apex/art_boot_images/javalib/arm64/boot.oat",
						to:   "/apex/art_boot_images/javalib/arm64/boot.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars_unstripped/android/apex/art_boot_images/javalib/arm64/boot-core2.oat",
						to:   "/apex/art_boot_images/javalib/arm64/boot-core2.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars_unstripped/android/apex/art_boot_images/javalib/arm64/boot-extra1.oat",
						to:   "/apex/art_boot_images/javalib/arm64/boot-extra1.oat",
					},
				},
				licenseMetadataFile: expectedLicenseMetadataFile,
			},
			{
				archType:          android.Arm,
				dexLocations:      []string{"/apex/com.android.art/javalib/core1.jar", "/apex/com.android.art/javalib/core2.jar", "/system/framework/extra1.jar"},
				dexLocationsDeps:  []string{"/apex/com.android.art/javalib/core1.jar", "/apex/com.android.art/javalib/core2.jar", "/system/framework/extra1.jar"},
				imagePathOnHost:   "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot.art",
				imagePathOnDevice: "/apex/art_boot_images/javalib/arm/boot.art",
				imagesDeps: []string{
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot.art",
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot.oat",
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot.vdex",
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-core2.art",
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-core2.oat",
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-core2.vdex",
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-extra1.art",
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-extra1.oat",
					"out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-extra1.vdex",
				},
				installs: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot.art",
						to:   "/apex/art_boot_images/javalib/arm/boot.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot.oat",
						to:   "/apex/art_boot_images/javalib/arm/boot.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-core2.art",
						to:   "/apex/art_boot_images/javalib/arm/boot-core2.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-core2.oat",
						to:   "/apex/art_boot_images/javalib/arm/boot-core2.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-extra1.art",
						to:   "/apex/art_boot_images/javalib/arm/boot-extra1.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-extra1.oat",
						to:   "/apex/art_boot_images/javalib/arm/boot-extra1.oat",
					},
				},
				vdexInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot.vdex",
						to:   "/apex/art_boot_images/javalib/arm/boot.vdex",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-core2.vdex",
						to:   "/apex/art_boot_images/javalib/arm/boot-core2.vdex",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-extra1.vdex",
						to:   "/apex/art_boot_images/javalib/arm/boot-extra1.vdex",
					},
				},
				unstrippedInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars_unstripped/android/apex/art_boot_images/javalib/arm/boot.oat",
						to:   "/apex/art_boot_images/javalib/arm/boot.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars_unstripped/android/apex/art_boot_images/javalib/arm/boot-core2.oat",
						to:   "/apex/art_boot_images/javalib/arm/boot-core2.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars_unstripped/android/apex/art_boot_images/javalib/arm/boot-extra1.oat",
						to:   "/apex/art_boot_images/javalib/arm/boot-extra1.oat",
					},
				},
				licenseMetadataFile: expectedLicenseMetadataFile,
			},
			{
				archType:          android.X86_64,
				dexLocations:      []string{"host/linux-x86/apex/com.android.art/javalib/core1.jar", "host/linux-x86/apex/com.android.art/javalib/core2.jar", "host/linux-x86/system/framework/extra1.jar"},
				dexLocationsDeps:  []string{"host/linux-x86/apex/com.android.art/javalib/core1.jar", "host/linux-x86/apex/com.android.art/javalib/core2.jar", "host/linux-x86/system/framework/extra1.jar"},
				imagePathOnHost:   "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot.art",
				imagePathOnDevice: "/apex/art_boot_images/javalib/x86_64/boot.art",
				imagesDeps: []string{
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot.art",
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot.oat",
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot.vdex",
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-core2.art",
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-core2.oat",
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-core2.vdex",
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-extra1.art",
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-extra1.oat",
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-extra1.vdex",
				},
				installs: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot.art",
						to:   "/apex/art_boot_images/javalib/x86_64/boot.art",
					}, {
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot.oat",
						to:   "/apex/art_boot_images/javalib/x86_64/boot.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-core2.art",
						to:   "/apex/art_boot_images/javalib/x86_64/boot-core2.art",
					}, {
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-core2.oat",
						to:   "/apex/art_boot_images/javalib/x86_64/boot-core2.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-extra1.art",
						to:   "/apex/art_boot_images/javalib/x86_64/boot-extra1.art",
					}, {
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-extra1.oat",
						to:   "/apex/art_boot_images/javalib/x86_64/boot-extra1.oat",
					},
				},
				vdexInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot.vdex",
						to:   "/apex/art_boot_images/javalib/x86_64/boot.vdex",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-core2.vdex",
						to:   "/apex/art_boot_images/javalib/x86_64/boot-core2.vdex",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-extra1.vdex",
						to:   "/apex/art_boot_images/javalib/x86_64/boot-extra1.vdex",
					},
				},
				unstrippedInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars_unstripped/linux_glibc/apex/art_boot_images/javalib/x86_64/boot.oat",
						to:   "/apex/art_boot_images/javalib/x86_64/boot.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars_unstripped/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-core2.oat",
						to:   "/apex/art_boot_images/javalib/x86_64/boot-core2.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars_unstripped/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-extra1.oat",
						to:   "/apex/art_boot_images/javalib/x86_64/boot-extra1.oat",
					},
				},
				licenseMetadataFile: expectedLicenseMetadataFile,
			},
			{
				archType:          android.X86,
				dexLocations:      []string{"host/linux-x86/apex/com.android.art/javalib/core1.jar", "host/linux-x86/apex/com.android.art/javalib/core2.jar", "host/linux-x86/system/framework/extra1.jar"},
				dexLocationsDeps:  []string{"host/linux-x86/apex/com.android.art/javalib/core1.jar", "host/linux-x86/apex/com.android.art/javalib/core2.jar", "host/linux-x86/system/framework/extra1.jar"},
				imagePathOnHost:   "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot.art",
				imagePathOnDevice: "/apex/art_boot_images/javalib/x86/boot.art",
				imagesDeps: []string{
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot.art",
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot.oat",
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot.vdex",
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-core2.art",
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-core2.oat",
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-core2.vdex",
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-extra1.art",
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-extra1.oat",
					"out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-extra1.vdex",
				},
				installs: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot.art",
						to:   "/apex/art_boot_images/javalib/x86/boot.art",
					}, {
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot.oat",
						to:   "/apex/art_boot_images/javalib/x86/boot.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-core2.art",
						to:   "/apex/art_boot_images/javalib/x86/boot-core2.art",
					}, {
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-core2.oat",
						to:   "/apex/art_boot_images/javalib/x86/boot-core2.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-extra1.art",
						to:   "/apex/art_boot_images/javalib/x86/boot-extra1.art",
					}, {
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-extra1.oat",
						to:   "/apex/art_boot_images/javalib/x86/boot-extra1.oat",
					},
				},
				vdexInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot.vdex",
						to:   "/apex/art_boot_images/javalib/x86/boot.vdex",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-core2.vdex",
						to:   "/apex/art_boot_images/javalib/x86/boot-core2.vdex",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-extra1.vdex",
						to:   "/apex/art_boot_images/javalib/x86/boot-extra1.vdex",
					},
				},
				unstrippedInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars_unstripped/linux_glibc/apex/art_boot_images/javalib/x86/boot.oat",
						to:   "/apex/art_boot_images/javalib/x86/boot.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars_unstripped/linux_glibc/apex/art_boot_images/javalib/x86/boot-core2.oat",
						to:   "/apex/art_boot_images/javalib/x86/boot-core2.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_artjars_unstripped/linux_glibc/apex/art_boot_images/javalib/x86/boot-extra1.oat",
						to:   "/apex/art_boot_images/javalib/x86/boot-extra1.oat",
					},
				},
				licenseMetadataFile: expectedLicenseMetadataFile,
			},
		},
	}

	checkBootImageConfig(t, result, imageConfig, mutated, expected)
}

// getFrameworkImageConfig gets the framework bootImageConfig that was created during the test.
func getFrameworkImageConfig(result *android.TestResult) *bootImageConfig {
	pathCtx := &android.TestPathContext{TestResult: result}
	imageConfig := defaultBootImageConfig(pathCtx)
	return imageConfig
}

// CheckFrameworkBootImageConfig checks the status of the fields of the bootImageConfig and
// bootImageVariant structures that are returned from defaultBootImageConfig.
//
// This is before any fields are mutated.
func CheckFrameworkBootImageConfig(t *testing.T, result *android.TestResult) {
	checkFrameworkBootImageConfig(t, result, false, "")
}

// checkFrameworkBootImageConfig checks the framework boot image.
//
// mutated is true if this is called after fields in the image have been mutated by the
// platform_bootclasspath and false otherwise.
func checkFrameworkBootImageConfig(t *testing.T, result *android.TestResult, mutated bool, expectedLicenseMetadataFile string) {
	imageConfig := getFrameworkImageConfig(result)

	expected := &expectedConfig{
		name:                     "boot",
		stem:                     "boot",
		dir:                      "out/soong/dexpreopt_arm64/dex_bootjars",
		symbolsDir:               "out/soong/dexpreopt_arm64/dex_bootjars_unstripped",
		installDir:               "system/framework",
		profileInstallPathInApex: "",
		modules: android.CreateTestConfiguredJarList([]string{
			"com.android.art:core1",
			"com.android.art:core2",
			"platform:framework",
		}),
		dexPaths: []string{
			"out/soong/dexpreopt_arm64/dex_bootjars_input/core1.jar",
			"out/soong/dexpreopt_arm64/dex_bootjars_input/core2.jar",
			"out/soong/dexpreopt_arm64/dex_bootjars_input/framework.jar",
		},
		dexPathsDeps: []string{
			"out/soong/dexpreopt_arm64/dex_bootjars_input/core1.jar",
			"out/soong/dexpreopt_arm64/dex_bootjars_input/core2.jar",
			"out/soong/dexpreopt_arm64/dex_bootjars_input/framework.jar",
		},
		zip: "out/soong/dexpreopt_arm64/dex_bootjars/boot.zip",
		variants: []*expectedVariant{
			{
				archType: android.Arm64,
				dexLocations: []string{
					"/apex/com.android.art/javalib/core1.jar",
					"/apex/com.android.art/javalib/core2.jar",
					"/system/framework/framework.jar",
				},
				dexLocationsDeps: []string{
					"/apex/com.android.art/javalib/core1.jar",
					"/apex/com.android.art/javalib/core2.jar",
					"/system/framework/framework.jar",
				},
				imagePathOnHost:   "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.art",
				imagePathOnDevice: "/system/framework/arm64/boot.art",
				imagesDeps: []string{
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.vdex",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-core2.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-core2.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-core2.vdex",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-framework.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-framework.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-framework.vdex",
				},
				installs: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.art",
						to:   "/system/framework/arm64/boot.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.oat",
						to:   "/system/framework/arm64/boot.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-core2.art",
						to:   "/system/framework/arm64/boot-core2.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-core2.oat",
						to:   "/system/framework/arm64/boot-core2.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-framework.art",
						to:   "/system/framework/arm64/boot-framework.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-framework.oat",
						to:   "/system/framework/arm64/boot-framework.oat",
					},
				},
				vdexInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.vdex",
						to:   "/system/framework/arm64/boot.vdex",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-core2.vdex",
						to:   "/system/framework/arm64/boot-core2.vdex",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-framework.vdex",
						to:   "/system/framework/arm64/boot-framework.vdex",
					},
				},
				unstrippedInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm64/boot.oat",
						to:   "/system/framework/arm64/boot.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm64/boot-core2.oat",
						to:   "/system/framework/arm64/boot-core2.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm64/boot-framework.oat",
						to:   "/system/framework/arm64/boot-framework.oat",
					},
				},
				licenseMetadataFile: expectedLicenseMetadataFile,
			},
			{
				archType: android.Arm,
				dexLocations: []string{
					"/apex/com.android.art/javalib/core1.jar",
					"/apex/com.android.art/javalib/core2.jar",
					"/system/framework/framework.jar",
				},
				dexLocationsDeps: []string{
					"/apex/com.android.art/javalib/core1.jar",
					"/apex/com.android.art/javalib/core2.jar",
					"/system/framework/framework.jar",
				},
				imagePathOnHost:   "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.art",
				imagePathOnDevice: "/system/framework/arm/boot.art",
				imagesDeps: []string{
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.vdex",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-core2.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-core2.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-core2.vdex",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-framework.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-framework.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-framework.vdex",
				},
				installs: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.art",
						to:   "/system/framework/arm/boot.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.oat",
						to:   "/system/framework/arm/boot.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-core2.art",
						to:   "/system/framework/arm/boot-core2.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-core2.oat",
						to:   "/system/framework/arm/boot-core2.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-framework.art",
						to:   "/system/framework/arm/boot-framework.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-framework.oat",
						to:   "/system/framework/arm/boot-framework.oat",
					},
				},
				vdexInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.vdex",
						to:   "/system/framework/arm/boot.vdex",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-core2.vdex",
						to:   "/system/framework/arm/boot-core2.vdex",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-framework.vdex",
						to:   "/system/framework/arm/boot-framework.vdex",
					},
				},
				unstrippedInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm/boot.oat",
						to:   "/system/framework/arm/boot.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm/boot-core2.oat",
						to:   "/system/framework/arm/boot-core2.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm/boot-framework.oat",
						to:   "/system/framework/arm/boot-framework.oat",
					},
				},
				licenseMetadataFile: expectedLicenseMetadataFile,
			},
			{
				archType: android.X86_64,
				dexLocations: []string{
					"host/linux-x86/apex/com.android.art/javalib/core1.jar",
					"host/linux-x86/apex/com.android.art/javalib/core2.jar",
					"host/linux-x86/system/framework/framework.jar",
				},
				dexLocationsDeps: []string{
					"host/linux-x86/apex/com.android.art/javalib/core1.jar",
					"host/linux-x86/apex/com.android.art/javalib/core2.jar",
					"host/linux-x86/system/framework/framework.jar",
				},
				imagePathOnHost:   "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.art",
				imagePathOnDevice: "/system/framework/x86_64/boot.art",
				imagesDeps: []string{
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.vdex",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-core2.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-core2.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-core2.vdex",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-framework.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-framework.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-framework.vdex",
				},
				installs: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.art",
						to:   "/system/framework/x86_64/boot.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.oat",
						to:   "/system/framework/x86_64/boot.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-core2.art",
						to:   "/system/framework/x86_64/boot-core2.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-core2.oat",
						to:   "/system/framework/x86_64/boot-core2.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-framework.art",
						to:   "/system/framework/x86_64/boot-framework.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-framework.oat",
						to:   "/system/framework/x86_64/boot-framework.oat",
					},
				},
				vdexInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.vdex",
						to:   "/system/framework/x86_64/boot.vdex",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-core2.vdex",
						to:   "/system/framework/x86_64/boot-core2.vdex",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-framework.vdex",
						to:   "/system/framework/x86_64/boot-framework.vdex",
					},
				},
				unstrippedInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars_unstripped/linux_glibc/system/framework/x86_64/boot.oat",
						to:   "/system/framework/x86_64/boot.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars_unstripped/linux_glibc/system/framework/x86_64/boot-core2.oat",
						to:   "/system/framework/x86_64/boot-core2.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars_unstripped/linux_glibc/system/framework/x86_64/boot-framework.oat",
						to:   "/system/framework/x86_64/boot-framework.oat",
					},
				},
				licenseMetadataFile: expectedLicenseMetadataFile,
			},
			{
				archType: android.X86,
				dexLocations: []string{
					"host/linux-x86/apex/com.android.art/javalib/core1.jar",
					"host/linux-x86/apex/com.android.art/javalib/core2.jar",
					"host/linux-x86/system/framework/framework.jar",
				},
				dexLocationsDeps: []string{
					"host/linux-x86/apex/com.android.art/javalib/core1.jar",
					"host/linux-x86/apex/com.android.art/javalib/core2.jar",
					"host/linux-x86/system/framework/framework.jar",
				},
				imagePathOnHost:   "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.art",
				imagePathOnDevice: "/system/framework/x86/boot.art",
				imagesDeps: []string{
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.vdex",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-core2.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-core2.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-core2.vdex",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-framework.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-framework.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-framework.vdex",
				},
				installs: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.art",
						to:   "/system/framework/x86/boot.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.oat",
						to:   "/system/framework/x86/boot.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-core2.art",
						to:   "/system/framework/x86/boot-core2.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-core2.oat",
						to:   "/system/framework/x86/boot-core2.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-framework.art",
						to:   "/system/framework/x86/boot-framework.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-framework.oat",
						to:   "/system/framework/x86/boot-framework.oat",
					},
				},
				vdexInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.vdex",
						to:   "/system/framework/x86/boot.vdex",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-core2.vdex",
						to:   "/system/framework/x86/boot-core2.vdex",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-framework.vdex",
						to:   "/system/framework/x86/boot-framework.vdex",
					},
				},
				unstrippedInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars_unstripped/linux_glibc/system/framework/x86/boot.oat",
						to:   "/system/framework/x86/boot.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars_unstripped/linux_glibc/system/framework/x86/boot-core2.oat",
						to:   "/system/framework/x86/boot-core2.oat",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_bootjars_unstripped/linux_glibc/system/framework/x86/boot-framework.oat",
						to:   "/system/framework/x86/boot-framework.oat",
					},
				},
				licenseMetadataFile: expectedLicenseMetadataFile,
			},
		},
		profileInstalls: []normalizedInstall{
			{from: "out/soong/.intermediates/default/java/dex_bootjars/android_common/boot/boot.prof", to: "/system/etc/boot-image.prof"},
			{from: "out/soong/dexpreopt_arm64/dex_bootjars/boot.bprof", to: "/system/etc/boot-image.bprof"},
		},
		profileLicenseMetadataFile: expectedLicenseMetadataFile,
	}

	checkBootImageConfig(t, result, imageConfig, mutated, expected)
}

// getMainlineImageConfig gets the framework bootImageConfig that was created during the test.
func getMainlineImageConfig(result *android.TestResult) *bootImageConfig {
	pathCtx := &android.TestPathContext{TestResult: result}
	imageConfig := mainlineBootImageConfig(pathCtx)
	return imageConfig
}

// CheckMainlineBootImageConfig checks the status of the fields of the bootImageConfig and
// bootImageVariant structures that are returned from mainlineBootImageConfig.
//
// This is before any fields are mutated.
func CheckMainlineBootImageConfig(t *testing.T, result *android.TestResult) {
	expectedLicenseMetadataFile := ""
	imageConfig := getMainlineImageConfig(result)

	expected := &expectedConfig{
		name:                     "mainline",
		stem:                     "boot",
		dir:                      "out/soong/dexpreopt_arm64/dex_mainlinejars",
		symbolsDir:               "out/soong/dexpreopt_arm64/dex_mainlinejars_unstripped",
		installDir:               "system/framework",
		profileInstallPathInApex: "",
		modules: android.CreateTestConfiguredJarList([]string{
			"com.android.foo:framework-foo",
			"com.android.bar:framework-bar",
		}),
		dexPaths: []string{
			"out/soong/dexpreopt_arm64/dex_mainlinejars_input/framework-foo.jar",
			"out/soong/dexpreopt_arm64/dex_mainlinejars_input/framework-bar.jar",
		},
		dexPathsDeps: []string{
			"out/soong/dexpreopt_arm64/dex_bootjars_input/core1.jar",
			"out/soong/dexpreopt_arm64/dex_bootjars_input/core2.jar",
			"out/soong/dexpreopt_arm64/dex_bootjars_input/framework.jar",
			"out/soong/dexpreopt_arm64/dex_mainlinejars_input/framework-foo.jar",
			"out/soong/dexpreopt_arm64/dex_mainlinejars_input/framework-bar.jar",
		},
		zip: "out/soong/dexpreopt_arm64/dex_mainlinejars/mainline.zip",
		variants: []*expectedVariant{
			{
				archType: android.Arm64,
				dexLocations: []string{
					"/apex/com.android.foo/javalib/framework-foo.jar",
					"/apex/com.android.bar/javalib/framework-bar.jar",
				},
				dexLocationsDeps: []string{
					"/apex/com.android.art/javalib/core1.jar",
					"/apex/com.android.art/javalib/core2.jar",
					"/system/framework/framework.jar",
					"/apex/com.android.foo/javalib/framework-foo.jar",
					"/apex/com.android.bar/javalib/framework-bar.jar",
				},
				imagePathOnHost:   "out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm64/boot-framework-foo.art",
				imagePathOnDevice: "/system/framework/arm64/boot-framework-foo.art",
				imagesDeps: []string{
					"out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm64/boot-framework-foo.art",
					"out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm64/boot-framework-foo.oat",
					"out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm64/boot-framework-foo.vdex",
				},
				baseImages: []string{
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.art",
				},
				baseImagesDeps: []string{
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.vdex",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-core2.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-core2.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-core2.vdex",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-framework.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-framework.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-framework.vdex",
				},
				installs: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm64/boot-framework-foo.art",
						to:   "/system/framework/arm64/boot-framework-foo.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm64/boot-framework-foo.oat",
						to:   "/system/framework/arm64/boot-framework-foo.oat",
					},
				},
				vdexInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm64/boot-framework-foo.vdex",
						to:   "/system/framework/arm64/boot-framework-foo.vdex",
					},
				},
				unstrippedInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_mainlinejars_unstripped/android/system/framework/arm64/boot-framework-foo.oat",
						to:   "/system/framework/arm64/boot-framework-foo.oat",
					},
				},
				licenseMetadataFile: expectedLicenseMetadataFile,
			},
			{
				archType: android.Arm,
				dexLocations: []string{
					"/apex/com.android.foo/javalib/framework-foo.jar",
					"/apex/com.android.bar/javalib/framework-bar.jar",
				},
				dexLocationsDeps: []string{
					"/apex/com.android.art/javalib/core1.jar",
					"/apex/com.android.art/javalib/core2.jar",
					"/system/framework/framework.jar",
					"/apex/com.android.foo/javalib/framework-foo.jar",
					"/apex/com.android.bar/javalib/framework-bar.jar",
				},
				imagePathOnHost:   "out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm/boot-framework-foo.art",
				imagePathOnDevice: "/system/framework/arm/boot-framework-foo.art",
				imagesDeps: []string{
					"out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm/boot-framework-foo.art",
					"out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm/boot-framework-foo.oat",
					"out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm/boot-framework-foo.vdex",
				},
				baseImages: []string{
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.art",
				},
				baseImagesDeps: []string{
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.vdex",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-core2.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-core2.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-core2.vdex",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-framework.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-framework.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-framework.vdex",
				},
				installs: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm/boot-framework-foo.art",
						to:   "/system/framework/arm/boot-framework-foo.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm/boot-framework-foo.oat",
						to:   "/system/framework/arm/boot-framework-foo.oat",
					},
				},
				vdexInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm/boot-framework-foo.vdex",
						to:   "/system/framework/arm/boot-framework-foo.vdex",
					},
				},
				unstrippedInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_mainlinejars_unstripped/android/system/framework/arm/boot-framework-foo.oat",
						to:   "/system/framework/arm/boot-framework-foo.oat",
					},
				},
				licenseMetadataFile: expectedLicenseMetadataFile,
			},
			{
				archType: android.X86_64,
				dexLocations: []string{
					"host/linux-x86/apex/com.android.foo/javalib/framework-foo.jar",
					"host/linux-x86/apex/com.android.bar/javalib/framework-bar.jar",
				},
				dexLocationsDeps: []string{
					"host/linux-x86/apex/com.android.art/javalib/core1.jar",
					"host/linux-x86/apex/com.android.art/javalib/core2.jar",
					"host/linux-x86/system/framework/framework.jar",
					"host/linux-x86/apex/com.android.foo/javalib/framework-foo.jar",
					"host/linux-x86/apex/com.android.bar/javalib/framework-bar.jar",
				},
				imagePathOnHost:   "out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86_64/boot-framework-foo.art",
				imagePathOnDevice: "/system/framework/x86_64/boot-framework-foo.art",
				imagesDeps: []string{
					"out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86_64/boot-framework-foo.art",
					"out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86_64/boot-framework-foo.oat",
					"out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86_64/boot-framework-foo.vdex",
				},
				baseImages: []string{
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.art",
				},
				baseImagesDeps: []string{
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.vdex",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-core2.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-core2.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-core2.vdex",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-framework.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-framework.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-framework.vdex",
				},
				installs: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86_64/boot-framework-foo.art",
						to:   "/system/framework/x86_64/boot-framework-foo.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86_64/boot-framework-foo.oat",
						to:   "/system/framework/x86_64/boot-framework-foo.oat",
					},
				},
				vdexInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86_64/boot-framework-foo.vdex",
						to:   "/system/framework/x86_64/boot-framework-foo.vdex",
					},
				},
				unstrippedInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_mainlinejars_unstripped/linux_glibc/system/framework/x86_64/boot-framework-foo.oat",
						to:   "/system/framework/x86_64/boot-framework-foo.oat",
					},
				},
				licenseMetadataFile: expectedLicenseMetadataFile,
			},
			{
				archType: android.X86,
				dexLocations: []string{
					"host/linux-x86/apex/com.android.foo/javalib/framework-foo.jar",
					"host/linux-x86/apex/com.android.bar/javalib/framework-bar.jar",
				},
				dexLocationsDeps: []string{
					"host/linux-x86/apex/com.android.art/javalib/core1.jar",
					"host/linux-x86/apex/com.android.art/javalib/core2.jar",
					"host/linux-x86/system/framework/framework.jar",
					"host/linux-x86/apex/com.android.foo/javalib/framework-foo.jar",
					"host/linux-x86/apex/com.android.bar/javalib/framework-bar.jar",
				},
				imagePathOnHost:   "out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86/boot-framework-foo.art",
				imagePathOnDevice: "/system/framework/x86/boot-framework-foo.art",
				imagesDeps: []string{
					"out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86/boot-framework-foo.art",
					"out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86/boot-framework-foo.oat",
					"out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86/boot-framework-foo.vdex",
				},
				baseImages: []string{
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.art",
				},
				baseImagesDeps: []string{
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.vdex",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-core2.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-core2.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-core2.vdex",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-framework.art",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-framework.oat",
					"out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-framework.vdex",
				},
				installs: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86/boot-framework-foo.art",
						to:   "/system/framework/x86/boot-framework-foo.art",
					},
					{
						from: "out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86/boot-framework-foo.oat",
						to:   "/system/framework/x86/boot-framework-foo.oat",
					},
				},
				vdexInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86/boot-framework-foo.vdex",
						to:   "/system/framework/x86/boot-framework-foo.vdex",
					},
				},
				unstrippedInstalls: []normalizedInstall{
					{
						from: "out/soong/dexpreopt_arm64/dex_mainlinejars_unstripped/linux_glibc/system/framework/x86/boot-framework-foo.oat",
						to:   "/system/framework/x86/boot-framework-foo.oat",
					},
				},
				licenseMetadataFile: expectedLicenseMetadataFile,
			},
		},
		profileInstalls:            []normalizedInstall{},
		profileLicenseMetadataFile: expectedLicenseMetadataFile,
	}

	checkBootImageConfig(t, result, imageConfig, false, expected)
}

// clearMutatedFields clears fields in the expectedConfig that correspond to fields in the
// bootImageConfig/bootImageVariant structs which are mutated outside the call to
// genBootImageConfigs.
//
// This allows the resulting expectedConfig struct to be compared against the values of those boot
// image structs immediately the call to genBootImageConfigs. If this is not called then the
// expectedConfig struct will expect the boot image structs to have been mutated by the ART
// bootclasspath_fragment and the platform_bootclasspath.
func clearMutatedFields(expected *expectedConfig) {
	expected.profileInstalls = nil
	expected.profileLicenseMetadataFile = ""
	for _, variant := range expected.variants {
		variant.installs = nil
		variant.vdexInstalls = nil
		variant.unstrippedInstalls = nil
		variant.licenseMetadataFile = ""
	}
}

// checkBootImageConfig checks a boot image against the expected contents.
//
// If mutated is false then this will clear any mutated fields in the expected contents back to the
// zero value so that they will match the unmodified values in the boot image.
//
// It runs the checks in an image specific subtest of the current test.
func checkBootImageConfig(t *testing.T, result *android.TestResult, imageConfig *bootImageConfig, mutated bool, expected *expectedConfig) {
	if !mutated {
		clearMutatedFields(expected)
	}

	t.Run(imageConfig.name, func(t *testing.T) {
		nestedCheckBootImageConfig(t, result, imageConfig, mutated, expected)
	})
}

// nestedCheckBootImageConfig does the work of comparing the image against the expected values and
// is run in an image specific subtest.
func nestedCheckBootImageConfig(t *testing.T, result *android.TestResult, imageConfig *bootImageConfig, mutated bool, expected *expectedConfig) {
	android.AssertStringEquals(t, "name", expected.name, imageConfig.name)
	android.AssertStringEquals(t, "stem", expected.stem, imageConfig.stem)
	android.AssertPathRelativeToTopEquals(t, "dir", expected.dir, imageConfig.dir)
	android.AssertPathRelativeToTopEquals(t, "symbolsDir", expected.symbolsDir, imageConfig.symbolsDir)
	android.AssertStringEquals(t, "installDir", expected.installDir, imageConfig.installDir)
	android.AssertDeepEquals(t, "modules", expected.modules, imageConfig.modules)
	android.AssertPathsRelativeToTopEquals(t, "dexPaths", expected.dexPaths, imageConfig.dexPaths.Paths())
	android.AssertPathsRelativeToTopEquals(t, "dexPathsDeps", expected.dexPathsDeps, imageConfig.dexPathsDeps.Paths())
	// dexPathsByModule is just a different representation of the other information in the config.
	android.AssertPathRelativeToTopEquals(t, "zip", expected.zip, imageConfig.zip)

	if !mutated {
		dexBootJarModule := result.ModuleForTests("dex_bootjars", "android_common")
		profileInstallInfo, _ := android.SingletonModuleProvider(result, dexBootJarModule.Module(), profileInstallInfoProvider)
		assertInstallsEqual(t, "profileInstalls", expected.profileInstalls, profileInstallInfo.profileInstalls)
		android.AssertStringEquals(t, "profileLicenseMetadataFile", expected.profileLicenseMetadataFile, profileInstallInfo.profileLicenseMetadataFile.RelativeToTop().String())
	}

	android.AssertIntEquals(t, "variant count", 4, len(imageConfig.variants))
	for i, variant := range imageConfig.variants {
		expectedVariant := expected.variants[i]
		t.Run(variant.target.Arch.ArchType.String(), func(t *testing.T) {
			android.AssertDeepEquals(t, "archType", expectedVariant.archType, variant.target.Arch.ArchType)
			android.AssertDeepEquals(t, "dexLocations", expectedVariant.dexLocations, variant.dexLocations)
			android.AssertDeepEquals(t, "dexLocationsDeps", expectedVariant.dexLocationsDeps, variant.dexLocationsDeps)
			android.AssertPathRelativeToTopEquals(t, "imagePathOnHost", expectedVariant.imagePathOnHost, variant.imagePathOnHost)
			android.AssertStringEquals(t, "imagePathOnDevice", expectedVariant.imagePathOnDevice, variant.imagePathOnDevice)
			android.AssertPathsRelativeToTopEquals(t, "imagesDeps", expectedVariant.imagesDeps, variant.imagesDeps.Paths())
			android.AssertPathsRelativeToTopEquals(t, "baseImages", expectedVariant.baseImages, variant.baseImages.Paths())
			android.AssertPathsRelativeToTopEquals(t, "baseImagesDeps", expectedVariant.baseImagesDeps, variant.baseImagesDeps)
			assertInstallsEqual(t, "installs", expectedVariant.installs, variant.installs)
			assertInstallsEqual(t, "vdexInstalls", expectedVariant.vdexInstalls, variant.vdexInstalls)
			assertInstallsEqual(t, "unstrippedInstalls", expectedVariant.unstrippedInstalls, variant.unstrippedInstalls)
			android.AssertStringEquals(t, "licenseMetadataFile", expectedVariant.licenseMetadataFile, variant.licenseMetadataFile.RelativeToTop().String())
		})
	}
}

// CheckMutatedArtBootImageConfig checks the mutated fields in the bootImageConfig/Variant for ART.
func CheckMutatedArtBootImageConfig(t *testing.T, result *android.TestResult, expectedLicenseMetadataFile string) {
	checkArtBootImageConfig(t, result, true, expectedLicenseMetadataFile)

	// Check the dexpreopt make vars. Do it in here as it depends on the expected license metadata
	// file at the moment and it
	checkDexpreoptMakeVars(t, result, expectedLicenseMetadataFile)
}

// CheckMutatedFrameworkBootImageConfig checks the mutated fields in the bootImageConfig/Variant for framework.
func CheckMutatedFrameworkBootImageConfig(t *testing.T, result *android.TestResult, expectedLicenseMetadataFile string) {
	checkFrameworkBootImageConfig(t, result, true, expectedLicenseMetadataFile)
}

// checkDexpreoptMakeVars checks the DEXPREOPT_ prefixed make vars produced by dexpreoptBootJars
// singleton.
func checkDexpreoptMakeVars(t *testing.T, result *android.TestResult, expectedLicenseMetadataFile string) {
	vars := result.MakeVarsForTesting(func(variable android.MakeVarVariable) bool {
		return strings.HasPrefix(variable.Name(), "DEXPREOPT_")
	})

	out := &strings.Builder{}
	for _, v := range vars {
		fmt.Fprintf(out, "%s=%s\n", v.Name(), android.StringRelativeToTop(result.Config, v.Value()))
	}
	format := `
DEXPREOPT_BOOTCLASSPATH_DEX_FILES=out/soong/dexpreopt_arm64/dex_bootjars_input/core1.jar out/soong/dexpreopt_arm64/dex_bootjars_input/core2.jar out/soong/dexpreopt_arm64/dex_bootjars_input/framework.jar
DEXPREOPT_BOOTCLASSPATH_DEX_LOCATIONS=/apex/com.android.art/javalib/core1.jar /apex/com.android.art/javalib/core2.jar /system/framework/framework.jar
DEXPREOPT_BOOT_JARS_MODULES=com.android.art:core1:com.android.art:core2:platform:framework
DEXPREOPT_GEN=out/host/linux-x86/bin/dexpreopt_gen
DEXPREOPT_IMAGE_BUILT_INSTALLED_art_arm=out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot.art:/apex/art_boot_images/javalib/arm/boot.art out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot.oat:/apex/art_boot_images/javalib/arm/boot.oat out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-core2.art:/apex/art_boot_images/javalib/arm/boot-core2.art out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-core2.oat:/apex/art_boot_images/javalib/arm/boot-core2.oat out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-extra1.art:/apex/art_boot_images/javalib/arm/boot-extra1.art out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-extra1.oat:/apex/art_boot_images/javalib/arm/boot-extra1.oat
DEXPREOPT_IMAGE_BUILT_INSTALLED_art_arm64=out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.art:/apex/art_boot_images/javalib/arm64/boot.art out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.oat:/apex/art_boot_images/javalib/arm64/boot.oat out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-core2.art:/apex/art_boot_images/javalib/arm64/boot-core2.art out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-core2.oat:/apex/art_boot_images/javalib/arm64/boot-core2.oat out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-extra1.art:/apex/art_boot_images/javalib/arm64/boot-extra1.art out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-extra1.oat:/apex/art_boot_images/javalib/arm64/boot-extra1.oat
DEXPREOPT_IMAGE_BUILT_INSTALLED_art_host_x86=out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot.art:/apex/art_boot_images/javalib/x86/boot.art out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot.oat:/apex/art_boot_images/javalib/x86/boot.oat out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-core2.art:/apex/art_boot_images/javalib/x86/boot-core2.art out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-core2.oat:/apex/art_boot_images/javalib/x86/boot-core2.oat out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-extra1.art:/apex/art_boot_images/javalib/x86/boot-extra1.art out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-extra1.oat:/apex/art_boot_images/javalib/x86/boot-extra1.oat
DEXPREOPT_IMAGE_BUILT_INSTALLED_art_host_x86_64=out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot.art:/apex/art_boot_images/javalib/x86_64/boot.art out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot.oat:/apex/art_boot_images/javalib/x86_64/boot.oat out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-core2.art:/apex/art_boot_images/javalib/x86_64/boot-core2.art out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-core2.oat:/apex/art_boot_images/javalib/x86_64/boot-core2.oat out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-extra1.art:/apex/art_boot_images/javalib/x86_64/boot-extra1.art out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-extra1.oat:/apex/art_boot_images/javalib/x86_64/boot-extra1.oat
DEXPREOPT_IMAGE_BUILT_INSTALLED_boot_arm=out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.art:/system/framework/arm/boot.art out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.oat:/system/framework/arm/boot.oat out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-core2.art:/system/framework/arm/boot-core2.art out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-core2.oat:/system/framework/arm/boot-core2.oat out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-framework.art:/system/framework/arm/boot-framework.art out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-framework.oat:/system/framework/arm/boot-framework.oat
DEXPREOPT_IMAGE_BUILT_INSTALLED_boot_arm64=out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.art:/system/framework/arm64/boot.art out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.oat:/system/framework/arm64/boot.oat out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-core2.art:/system/framework/arm64/boot-core2.art out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-core2.oat:/system/framework/arm64/boot-core2.oat out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-framework.art:/system/framework/arm64/boot-framework.art out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-framework.oat:/system/framework/arm64/boot-framework.oat
DEXPREOPT_IMAGE_BUILT_INSTALLED_boot_host_x86=out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.art:/system/framework/x86/boot.art out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.oat:/system/framework/x86/boot.oat out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-core2.art:/system/framework/x86/boot-core2.art out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-core2.oat:/system/framework/x86/boot-core2.oat out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-framework.art:/system/framework/x86/boot-framework.art out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-framework.oat:/system/framework/x86/boot-framework.oat
DEXPREOPT_IMAGE_BUILT_INSTALLED_boot_host_x86_64=out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.art:/system/framework/x86_64/boot.art out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.oat:/system/framework/x86_64/boot.oat out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-core2.art:/system/framework/x86_64/boot-core2.art out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-core2.oat:/system/framework/x86_64/boot-core2.oat out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-framework.art:/system/framework/x86_64/boot-framework.art out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-framework.oat:/system/framework/x86_64/boot-framework.oat
DEXPREOPT_IMAGE_BUILT_INSTALLED_mainline_arm=out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm/boot-framework-foo.art:/system/framework/arm/boot-framework-foo.art out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm/boot-framework-foo.oat:/system/framework/arm/boot-framework-foo.oat
DEXPREOPT_IMAGE_BUILT_INSTALLED_mainline_arm64=out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm64/boot-framework-foo.art:/system/framework/arm64/boot-framework-foo.art out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm64/boot-framework-foo.oat:/system/framework/arm64/boot-framework-foo.oat
DEXPREOPT_IMAGE_BUILT_INSTALLED_mainline_host_x86=out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86/boot-framework-foo.art:/system/framework/x86/boot-framework-foo.art out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86/boot-framework-foo.oat:/system/framework/x86/boot-framework-foo.oat
DEXPREOPT_IMAGE_BUILT_INSTALLED_mainline_host_x86_64=out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86_64/boot-framework-foo.art:/system/framework/x86_64/boot-framework-foo.art out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86_64/boot-framework-foo.oat:/system/framework/x86_64/boot-framework-foo.oat
DEXPREOPT_IMAGE_DEPS_art_arm=out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot.art out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot.oat out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot.vdex out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-core2.art out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-core2.oat out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-core2.vdex out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-extra1.art out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-extra1.oat out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-extra1.vdex
DEXPREOPT_IMAGE_DEPS_art_arm64=out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.art out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.oat out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.vdex out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-core2.art out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-core2.oat out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-core2.vdex out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-extra1.art out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-extra1.oat out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-extra1.vdex
DEXPREOPT_IMAGE_DEPS_art_host_x86=out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot.art out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot.oat out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot.vdex out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-core2.art out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-core2.oat out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-core2.vdex out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-extra1.art out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-extra1.oat out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-extra1.vdex
DEXPREOPT_IMAGE_DEPS_art_host_x86_64=out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot.art out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot.oat out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot.vdex out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-core2.art out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-core2.oat out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-core2.vdex out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-extra1.art out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-extra1.oat out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-extra1.vdex
DEXPREOPT_IMAGE_DEPS_boot_arm=out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.art out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.oat out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.vdex out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-core2.art out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-core2.oat out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-core2.vdex out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-framework.art out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-framework.oat out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-framework.vdex
DEXPREOPT_IMAGE_DEPS_boot_arm64=out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.art out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.oat out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.vdex out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-core2.art out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-core2.oat out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-core2.vdex out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-framework.art out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-framework.oat out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-framework.vdex
DEXPREOPT_IMAGE_DEPS_boot_host_x86=out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.art out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.oat out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.vdex out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-core2.art out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-core2.oat out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-core2.vdex out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-framework.art out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-framework.oat out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-framework.vdex
DEXPREOPT_IMAGE_DEPS_boot_host_x86_64=out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.art out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.oat out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.vdex out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-core2.art out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-core2.oat out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-core2.vdex out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-framework.art out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-framework.oat out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-framework.vdex
DEXPREOPT_IMAGE_DEPS_mainline_arm=out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm/boot-framework-foo.art out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm/boot-framework-foo.oat out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm/boot-framework-foo.vdex
DEXPREOPT_IMAGE_DEPS_mainline_arm64=out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm64/boot-framework-foo.art out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm64/boot-framework-foo.oat out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm64/boot-framework-foo.vdex
DEXPREOPT_IMAGE_DEPS_mainline_host_x86=out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86/boot-framework-foo.art out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86/boot-framework-foo.oat out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86/boot-framework-foo.vdex
DEXPREOPT_IMAGE_DEPS_mainline_host_x86_64=out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86_64/boot-framework-foo.art out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86_64/boot-framework-foo.oat out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86_64/boot-framework-foo.vdex
DEXPREOPT_IMAGE_LICENSE_METADATA_art_arm=%[1]s
DEXPREOPT_IMAGE_LICENSE_METADATA_art_arm64=%[1]s
DEXPREOPT_IMAGE_LICENSE_METADATA_art_host_x86=%[1]s
DEXPREOPT_IMAGE_LICENSE_METADATA_art_host_x86_64=%[1]s
DEXPREOPT_IMAGE_LICENSE_METADATA_boot_arm=out/soong/.intermediates/default/java/dex_bootjars/android_common/meta_lic
DEXPREOPT_IMAGE_LICENSE_METADATA_boot_arm64=out/soong/.intermediates/default/java/dex_bootjars/android_common/meta_lic
DEXPREOPT_IMAGE_LICENSE_METADATA_boot_host_x86=out/soong/.intermediates/default/java/dex_bootjars/android_common/meta_lic
DEXPREOPT_IMAGE_LICENSE_METADATA_boot_host_x86_64=out/soong/.intermediates/default/java/dex_bootjars/android_common/meta_lic
DEXPREOPT_IMAGE_LICENSE_METADATA_mainline_arm=out/soong/.intermediates/default/java/dex_bootjars/android_common/meta_lic
DEXPREOPT_IMAGE_LICENSE_METADATA_mainline_arm64=out/soong/.intermediates/default/java/dex_bootjars/android_common/meta_lic
DEXPREOPT_IMAGE_LICENSE_METADATA_mainline_host_x86=out/soong/.intermediates/default/java/dex_bootjars/android_common/meta_lic
DEXPREOPT_IMAGE_LICENSE_METADATA_mainline_host_x86_64=out/soong/.intermediates/default/java/dex_bootjars/android_common/meta_lic
DEXPREOPT_IMAGE_LOCATIONS_ON_DEVICEart=/apex/art_boot_images/javalib/boot.art
DEXPREOPT_IMAGE_LOCATIONS_ON_DEVICEboot=/system/framework/boot.art
DEXPREOPT_IMAGE_LOCATIONS_ON_DEVICEmainline=/system/framework/boot.art:/system/framework/boot-framework-foo.art
DEXPREOPT_IMAGE_LOCATIONS_ON_HOSTart=out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/boot.art
DEXPREOPT_IMAGE_LOCATIONS_ON_HOSTboot=out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/boot.art
DEXPREOPT_IMAGE_LOCATIONS_ON_HOSTmainline=out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/boot.art:out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/boot-framework-foo.art
DEXPREOPT_IMAGE_NAMES=art boot mainline
DEXPREOPT_IMAGE_PROFILE_BUILT_INSTALLED=out/soong/.intermediates/default/java/dex_bootjars/android_common/boot/boot.prof:/system/etc/boot-image.prof out/soong/dexpreopt_arm64/dex_bootjars/boot.bprof:/system/etc/boot-image.bprof
DEXPREOPT_IMAGE_PROFILE_LICENSE_METADATA=out/soong/.intermediates/default/java/dex_bootjars/android_common/meta_lic
DEXPREOPT_IMAGE_UNSTRIPPED_BUILT_INSTALLED_art_arm=out/soong/dexpreopt_arm64/dex_artjars_unstripped/android/apex/art_boot_images/javalib/arm/boot.oat:/apex/art_boot_images/javalib/arm/boot.oat out/soong/dexpreopt_arm64/dex_artjars_unstripped/android/apex/art_boot_images/javalib/arm/boot-core2.oat:/apex/art_boot_images/javalib/arm/boot-core2.oat out/soong/dexpreopt_arm64/dex_artjars_unstripped/android/apex/art_boot_images/javalib/arm/boot-extra1.oat:/apex/art_boot_images/javalib/arm/boot-extra1.oat
DEXPREOPT_IMAGE_UNSTRIPPED_BUILT_INSTALLED_art_arm64=out/soong/dexpreopt_arm64/dex_artjars_unstripped/android/apex/art_boot_images/javalib/arm64/boot.oat:/apex/art_boot_images/javalib/arm64/boot.oat out/soong/dexpreopt_arm64/dex_artjars_unstripped/android/apex/art_boot_images/javalib/arm64/boot-core2.oat:/apex/art_boot_images/javalib/arm64/boot-core2.oat out/soong/dexpreopt_arm64/dex_artjars_unstripped/android/apex/art_boot_images/javalib/arm64/boot-extra1.oat:/apex/art_boot_images/javalib/arm64/boot-extra1.oat
DEXPREOPT_IMAGE_UNSTRIPPED_BUILT_INSTALLED_art_host_x86=out/soong/dexpreopt_arm64/dex_artjars_unstripped/linux_glibc/apex/art_boot_images/javalib/x86/boot.oat:/apex/art_boot_images/javalib/x86/boot.oat out/soong/dexpreopt_arm64/dex_artjars_unstripped/linux_glibc/apex/art_boot_images/javalib/x86/boot-core2.oat:/apex/art_boot_images/javalib/x86/boot-core2.oat out/soong/dexpreopt_arm64/dex_artjars_unstripped/linux_glibc/apex/art_boot_images/javalib/x86/boot-extra1.oat:/apex/art_boot_images/javalib/x86/boot-extra1.oat
DEXPREOPT_IMAGE_UNSTRIPPED_BUILT_INSTALLED_art_host_x86_64=out/soong/dexpreopt_arm64/dex_artjars_unstripped/linux_glibc/apex/art_boot_images/javalib/x86_64/boot.oat:/apex/art_boot_images/javalib/x86_64/boot.oat out/soong/dexpreopt_arm64/dex_artjars_unstripped/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-core2.oat:/apex/art_boot_images/javalib/x86_64/boot-core2.oat out/soong/dexpreopt_arm64/dex_artjars_unstripped/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-extra1.oat:/apex/art_boot_images/javalib/x86_64/boot-extra1.oat
DEXPREOPT_IMAGE_UNSTRIPPED_BUILT_INSTALLED_boot_arm=out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm/boot.oat:/system/framework/arm/boot.oat out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm/boot-core2.oat:/system/framework/arm/boot-core2.oat out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm/boot-framework.oat:/system/framework/arm/boot-framework.oat
DEXPREOPT_IMAGE_UNSTRIPPED_BUILT_INSTALLED_boot_arm64=out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm64/boot.oat:/system/framework/arm64/boot.oat out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm64/boot-core2.oat:/system/framework/arm64/boot-core2.oat out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm64/boot-framework.oat:/system/framework/arm64/boot-framework.oat
DEXPREOPT_IMAGE_UNSTRIPPED_BUILT_INSTALLED_boot_host_x86=out/soong/dexpreopt_arm64/dex_bootjars_unstripped/linux_glibc/system/framework/x86/boot.oat:/system/framework/x86/boot.oat out/soong/dexpreopt_arm64/dex_bootjars_unstripped/linux_glibc/system/framework/x86/boot-core2.oat:/system/framework/x86/boot-core2.oat out/soong/dexpreopt_arm64/dex_bootjars_unstripped/linux_glibc/system/framework/x86/boot-framework.oat:/system/framework/x86/boot-framework.oat
DEXPREOPT_IMAGE_UNSTRIPPED_BUILT_INSTALLED_boot_host_x86_64=out/soong/dexpreopt_arm64/dex_bootjars_unstripped/linux_glibc/system/framework/x86_64/boot.oat:/system/framework/x86_64/boot.oat out/soong/dexpreopt_arm64/dex_bootjars_unstripped/linux_glibc/system/framework/x86_64/boot-core2.oat:/system/framework/x86_64/boot-core2.oat out/soong/dexpreopt_arm64/dex_bootjars_unstripped/linux_glibc/system/framework/x86_64/boot-framework.oat:/system/framework/x86_64/boot-framework.oat
DEXPREOPT_IMAGE_UNSTRIPPED_BUILT_INSTALLED_mainline_arm=out/soong/dexpreopt_arm64/dex_mainlinejars_unstripped/android/system/framework/arm/boot-framework-foo.oat:/system/framework/arm/boot-framework-foo.oat
DEXPREOPT_IMAGE_UNSTRIPPED_BUILT_INSTALLED_mainline_arm64=out/soong/dexpreopt_arm64/dex_mainlinejars_unstripped/android/system/framework/arm64/boot-framework-foo.oat:/system/framework/arm64/boot-framework-foo.oat
DEXPREOPT_IMAGE_UNSTRIPPED_BUILT_INSTALLED_mainline_host_x86=out/soong/dexpreopt_arm64/dex_mainlinejars_unstripped/linux_glibc/system/framework/x86/boot-framework-foo.oat:/system/framework/x86/boot-framework-foo.oat
DEXPREOPT_IMAGE_UNSTRIPPED_BUILT_INSTALLED_mainline_host_x86_64=out/soong/dexpreopt_arm64/dex_mainlinejars_unstripped/linux_glibc/system/framework/x86_64/boot-framework-foo.oat:/system/framework/x86_64/boot-framework-foo.oat
DEXPREOPT_IMAGE_VDEX_BUILT_INSTALLED_art_arm=out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot.vdex:/apex/art_boot_images/javalib/arm/boot.vdex out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-core2.vdex:/apex/art_boot_images/javalib/arm/boot-core2.vdex out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-extra1.vdex:/apex/art_boot_images/javalib/arm/boot-extra1.vdex
DEXPREOPT_IMAGE_VDEX_BUILT_INSTALLED_art_arm64=out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.vdex:/apex/art_boot_images/javalib/arm64/boot.vdex out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-core2.vdex:/apex/art_boot_images/javalib/arm64/boot-core2.vdex out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-extra1.vdex:/apex/art_boot_images/javalib/arm64/boot-extra1.vdex
DEXPREOPT_IMAGE_VDEX_BUILT_INSTALLED_art_host_x86=out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot.vdex:/apex/art_boot_images/javalib/x86/boot.vdex out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-core2.vdex:/apex/art_boot_images/javalib/x86/boot-core2.vdex out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot-extra1.vdex:/apex/art_boot_images/javalib/x86/boot-extra1.vdex
DEXPREOPT_IMAGE_VDEX_BUILT_INSTALLED_art_host_x86_64=out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot.vdex:/apex/art_boot_images/javalib/x86_64/boot.vdex out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-core2.vdex:/apex/art_boot_images/javalib/x86_64/boot-core2.vdex out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot-extra1.vdex:/apex/art_boot_images/javalib/x86_64/boot-extra1.vdex
DEXPREOPT_IMAGE_VDEX_BUILT_INSTALLED_boot_arm=out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.vdex:/system/framework/arm/boot.vdex out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-core2.vdex:/system/framework/arm/boot-core2.vdex out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot-framework.vdex:/system/framework/arm/boot-framework.vdex
DEXPREOPT_IMAGE_VDEX_BUILT_INSTALLED_boot_arm64=out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.vdex:/system/framework/arm64/boot.vdex out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-core2.vdex:/system/framework/arm64/boot-core2.vdex out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-framework.vdex:/system/framework/arm64/boot-framework.vdex
DEXPREOPT_IMAGE_VDEX_BUILT_INSTALLED_boot_host_x86=out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.vdex:/system/framework/x86/boot.vdex out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-core2.vdex:/system/framework/x86/boot-core2.vdex out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot-framework.vdex:/system/framework/x86/boot-framework.vdex
DEXPREOPT_IMAGE_VDEX_BUILT_INSTALLED_boot_host_x86_64=out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.vdex:/system/framework/x86_64/boot.vdex out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-core2.vdex:/system/framework/x86_64/boot-core2.vdex out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot-framework.vdex:/system/framework/x86_64/boot-framework.vdex
DEXPREOPT_IMAGE_VDEX_BUILT_INSTALLED_mainline_arm=out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm/boot-framework-foo.vdex:/system/framework/arm/boot-framework-foo.vdex
DEXPREOPT_IMAGE_VDEX_BUILT_INSTALLED_mainline_arm64=out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm64/boot-framework-foo.vdex:/system/framework/arm64/boot-framework-foo.vdex
DEXPREOPT_IMAGE_VDEX_BUILT_INSTALLED_mainline_host_x86=out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86/boot-framework-foo.vdex:/system/framework/x86/boot-framework-foo.vdex
DEXPREOPT_IMAGE_VDEX_BUILT_INSTALLED_mainline_host_x86_64=out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86_64/boot-framework-foo.vdex:/system/framework/x86_64/boot-framework-foo.vdex
DEXPREOPT_IMAGE_ZIP_art=out/soong/dexpreopt_arm64/dex_artjars/art.zip
DEXPREOPT_IMAGE_ZIP_boot=out/soong/dexpreopt_arm64/dex_bootjars/boot.zip
DEXPREOPT_IMAGE_ZIP_mainline=out/soong/dexpreopt_arm64/dex_mainlinejars/mainline.zip
DEXPREOPT_IMAGE_art_arm=out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm/boot.art
DEXPREOPT_IMAGE_art_arm64=out/soong/dexpreopt_arm64/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.art
DEXPREOPT_IMAGE_art_host_x86=out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86/boot.art
DEXPREOPT_IMAGE_art_host_x86_64=out/soong/dexpreopt_arm64/dex_artjars/linux_glibc/apex/art_boot_images/javalib/x86_64/boot.art
DEXPREOPT_IMAGE_boot_arm=out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm/boot.art
DEXPREOPT_IMAGE_boot_arm64=out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.art
DEXPREOPT_IMAGE_boot_host_x86=out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86/boot.art
DEXPREOPT_IMAGE_boot_host_x86_64=out/soong/dexpreopt_arm64/dex_bootjars/linux_glibc/system/framework/x86_64/boot.art
DEXPREOPT_IMAGE_mainline_arm=out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm/boot-framework-foo.art
DEXPREOPT_IMAGE_mainline_arm64=out/soong/dexpreopt_arm64/dex_mainlinejars/android/system/framework/arm64/boot-framework-foo.art
DEXPREOPT_IMAGE_mainline_host_x86=out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86/boot-framework-foo.art
DEXPREOPT_IMAGE_mainline_host_x86_64=out/soong/dexpreopt_arm64/dex_mainlinejars/linux_glibc/system/framework/x86_64/boot-framework-foo.art
`
	expected := strings.TrimSpace(fmt.Sprintf(format, expectedLicenseMetadataFile))
	actual := strings.TrimSpace(out.String())
	android.AssertStringEquals(t, "vars", expected, actual)
}
