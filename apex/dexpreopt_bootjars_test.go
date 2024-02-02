// Copyright 2019 Google Inc. All rights reserved.
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

package apex

import (
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	"android/soong/android"
	"android/soong/java"
)

func testDexpreoptBoot(t *testing.T, ruleFile string, expectedInputs, expectedOutputs []string, preferPrebuilt bool) {
	bp := `
		// Platform.

		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			api_packages: ["foo"],
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
			installable: true,
			system_ext_specific: true,
		}

		dex_import {
			name: "baz",
			jars: ["a.jar"],
		}

		platform_bootclasspath {
			name: "platform-bootclasspath",
			fragments: [
				{
					apex: "com.android.art",
					module: "art-bootclasspath-fragment",
				},
			],
		}

		// Source ART APEX.

		java_library {
			name: "core-oj",
			srcs: ["core-oj.java"],
			installable: true,
			apex_available: [
				"com.android.art",
			],
		}

		bootclasspath_fragment {
			name: "art-bootclasspath-fragment",
			image_name: "art",
			contents: ["core-oj"],
			apex_available: [
				"com.android.art",
			],
			hidden_api: {
				split_packages: ["*"],
			},
		}

		apex_key {
			name: "com.android.art.key",
			public_key: "com.android.art.avbpubkey",
			private_key: "com.android.art.pem",
		}

		apex {
			name: "com.android.art",
			key: "com.android.art.key",
			bootclasspath_fragments: ["art-bootclasspath-fragment"],
			updatable: false,
		}

		// Prebuilt ART APEX.

		java_import {
			name: "core-oj",
			prefer: %[1]t,
			jars: ["core-oj.jar"],
			apex_available: [
				"com.android.art",
			],
		}

		prebuilt_bootclasspath_fragment {
			name: "art-bootclasspath-fragment",
			prefer: %[1]t,
			image_name: "art",
			contents: ["core-oj"],
			hidden_api: {
				annotation_flags: "my-bootclasspath-fragment/annotation-flags.csv",
				metadata: "my-bootclasspath-fragment/metadata.csv",
				index: "my-bootclasspath-fragment/index.csv",
				stub_flags: "my-bootclasspath-fragment/stub-flags.csv",
				all_flags: "my-bootclasspath-fragment/all-flags.csv",
			},
			apex_available: [
				"com.android.art",
			],
		}

		prebuilt_apex {
			name: "com.android.art",
			prefer: %[1]t,
			apex_name: "com.android.art",
			src: "com.android.art-arm.apex",
			exported_bootclasspath_fragments: ["art-bootclasspath-fragment"],
		}
	`

	result := android.GroupFixturePreparers(
		java.PrepareForTestWithDexpreopt,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("foo"),
		java.FixtureConfigureBootJars("com.android.art:core-oj", "platform:foo", "system_ext:bar", "platform:baz"),
		PrepareForTestWithApexBuildComponents,
		prepareForTestWithArtApex,
	).RunTestWithBp(t, fmt.Sprintf(bp, preferPrebuilt))

	dexBootJars := result.ModuleForTests("dex_bootjars", "android_common")
	rule := dexBootJars.Output(ruleFile)

	inputs := rule.Implicits.Strings()
	sort.Strings(inputs)
	sort.Strings(expectedInputs)

	outputs := append(android.WritablePaths{rule.Output}, rule.ImplicitOutputs...).Strings()
	sort.Strings(outputs)
	sort.Strings(expectedOutputs)

	android.AssertStringPathsRelativeToTopEquals(t, "inputs", result.Config, expectedInputs, inputs)

	android.AssertStringPathsRelativeToTopEquals(t, "outputs", result.Config, expectedOutputs, outputs)
}

func TestDexpreoptBootJarsWithSourceArtApex(t *testing.T) {
	ruleFile := "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.art"

	expectedInputs := []string{
		"out/soong/dexpreopt_arm64/dex_bootjars_input/core-oj.jar",
		"out/soong/dexpreopt_arm64/dex_bootjars_input/foo.jar",
		"out/soong/dexpreopt_arm64/dex_bootjars_input/bar.jar",
		"out/soong/dexpreopt_arm64/dex_bootjars_input/baz.jar",
		"out/soong/.intermediates/art-bootclasspath-fragment/android_common_apex10000/art-bootclasspath-fragment/boot.prof",
		"out/soong/.intermediates/default/java/dex_bootjars/android_common/boot/boot.prof",
		"out/soong/dexpreopt/uffd_gc_flag.txt",
	}

	expectedOutputs := []string{
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.invocation",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.art",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-foo.art",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-bar.art",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-baz.art",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.oat",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-foo.oat",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-bar.oat",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-baz.oat",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.vdex",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-foo.vdex",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-bar.vdex",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-baz.vdex",
		"out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm64/boot.oat",
		"out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm64/boot-foo.oat",
		"out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm64/boot-bar.oat",
		"out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm64/boot-baz.oat",
	}

	testDexpreoptBoot(t, ruleFile, expectedInputs, expectedOutputs, false)
}

// The only difference is that the ART profile should be deapexed from the prebuilt APEX. Other
// inputs and outputs should be the same as above.
func TestDexpreoptBootJarsWithPrebuiltArtApex(t *testing.T) {
	ruleFile := "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.art"

	expectedInputs := []string{
		"out/soong/dexpreopt_arm64/dex_bootjars_input/core-oj.jar",
		"out/soong/dexpreopt_arm64/dex_bootjars_input/foo.jar",
		"out/soong/dexpreopt_arm64/dex_bootjars_input/bar.jar",
		"out/soong/dexpreopt_arm64/dex_bootjars_input/baz.jar",
		"out/soong/.intermediates/prebuilt_com.android.art.deapexer/android_common/deapexer/etc/boot-image.prof",
		"out/soong/.intermediates/default/java/dex_bootjars/android_common/boot/boot.prof",
		"out/soong/dexpreopt/uffd_gc_flag.txt",
	}

	expectedOutputs := []string{
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.invocation",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.art",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-foo.art",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-bar.art",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-baz.art",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.oat",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-foo.oat",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-bar.oat",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-baz.oat",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.vdex",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-foo.vdex",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-bar.vdex",
		"out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot-baz.vdex",
		"out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm64/boot.oat",
		"out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm64/boot-foo.oat",
		"out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm64/boot-bar.oat",
		"out/soong/dexpreopt_arm64/dex_bootjars_unstripped/android/system/framework/arm64/boot-baz.oat",
	}

	testDexpreoptBoot(t, ruleFile, expectedInputs, expectedOutputs, true)
}

// Changes to the boot.zip structure may break the ART APK scanner.
func TestDexpreoptBootZip(t *testing.T) {
	ruleFile := "boot.zip"

	ctx := android.PathContextForTesting(android.TestArchConfig("", nil, "", nil))
	expectedInputs := []string{}
	for _, target := range ctx.Config().Targets[android.Android] {
		for _, ext := range []string{".art", ".oat", ".vdex"} {
			for _, suffix := range []string{"", "-foo", "-bar", "-baz"} {
				expectedInputs = append(expectedInputs,
					filepath.Join(
						"out/soong/dexpreopt_arm64/dex_bootjars",
						target.Os.String(),
						"system/framework",
						target.Arch.ArchType.String(),
						"boot"+suffix+ext))
			}
		}
	}

	expectedOutputs := []string{
		"out/soong/dexpreopt_arm64/dex_bootjars/boot.zip",
	}

	testDexpreoptBoot(t, ruleFile, expectedInputs, expectedOutputs, false)
}

// Multiple ART apexes might exist in the tree.
// The profile should correspond to the apex selected using release build flags
func TestDexpreoptProfileWithMultiplePrebuiltArtApexes(t *testing.T) {
	ruleFile := "out/soong/dexpreopt_arm64/dex_bootjars/android/system/framework/arm64/boot.art"
	bp := `
		// Platform.

		platform_bootclasspath {
			name: "platform-bootclasspath",
			fragments: [
				{
					apex: "com.android.art",
					module: "art-bootclasspath-fragment",
				},
			],
		}

		// Source ART APEX.

		java_library {
			name: "core-oj",
			srcs: ["core-oj.java"],
			installable: true,
			apex_available: [
				"com.android.art",
			],
		}

		bootclasspath_fragment {
			name: "art-bootclasspath-fragment",
			image_name: "art",
			contents: ["core-oj"],
			apex_available: [
				"com.android.art",
			],
			hidden_api: {
				split_packages: ["*"],
			},
		}

		apex_key {
			name: "com.android.art.key",
			public_key: "com.android.art.avbpubkey",
			private_key: "com.android.art.pem",
		}

		apex {
			name: "com.android.art",
			key: "com.android.art.key",
			bootclasspath_fragments: ["art-bootclasspath-fragment"],
			updatable: false,
		}

		// Prebuilt ART APEX.

		java_import {
			name: "core-oj",
			jars: ["core-oj.jar"],
			apex_available: [
				"com.android.art",
			],
		}

		prebuilt_bootclasspath_fragment {
			name: "art-bootclasspath-fragment",
			image_name: "art",
			contents: ["core-oj"],
			hidden_api: {
				annotation_flags: "my-bootclasspath-fragment/annotation-flags.csv",
				metadata: "my-bootclasspath-fragment/metadata.csv",
				index: "my-bootclasspath-fragment/index.csv",
				stub_flags: "my-bootclasspath-fragment/stub-flags.csv",
				all_flags: "my-bootclasspath-fragment/all-flags.csv",
			},
			apex_available: [
				"com.android.art",
			],
		}

		prebuilt_apex {
			name: "com.android.art",
			apex_name: "com.android.art",
			src: "com.android.art-arm.apex",
			exported_bootclasspath_fragments: ["art-bootclasspath-fragment"],
		}

		// Another Prebuilt ART APEX
		prebuilt_apex {
			name: "com.android.art.v2",
			apex_name: "com.android.art", // Used to determine the API domain
			src: "com.android.art-arm.apex",
			exported_bootclasspath_fragments: ["art-bootclasspath-fragment"],
		}

		// APEX contribution modules

		apex_contributions {
			name: "art.source.contributions",
			api_domain: "com.android.art",
			contents: ["com.android.art"],
		}

		apex_contributions {
			name: "art.prebuilt.contributions",
			api_domain: "com.android.art",
			contents: ["prebuilt_com.android.art"],
		}

		apex_contributions {
			name: "art.prebuilt.v2.contributions",
			api_domain: "com.android.art",
			contents: ["com.android.art.v2"], // prebuilt_ prefix is missing because of prebuilt_rename mutator
		}

	`

	testCases := []struct {
		desc                         string
		selectedArtApexContributions string
		expectedProfile              string
	}{
		{
			desc:                         "Source apex com.android.art is selected, profile should come from source java library",
			selectedArtApexContributions: "art.source.contributions",
			expectedProfile:              "out/soong/.intermediates/art-bootclasspath-fragment/android_common_apex10000/art-bootclasspath-fragment/boot.prof",
		},
		{
			desc:                         "Prebuilt apex prebuilt_com.android.art is selected, profile should come from .prof deapexed from the prebuilt",
			selectedArtApexContributions: "art.prebuilt.contributions",
			expectedProfile:              "out/soong/.intermediates/prebuilt_com.android.art.deapexer/android_common/deapexer/etc/boot-image.prof",
		},
		{
			desc:                         "Prebuilt apex prebuilt_com.android.art.v2 is selected, profile should come from .prof deapexed from the prebuilt",
			selectedArtApexContributions: "art.prebuilt.v2.contributions",
			expectedProfile:              "out/soong/.intermediates/prebuilt_com.android.art.v2.deapexer/android_common/deapexer/etc/boot-image.prof",
		},
	}
	for _, tc := range testCases {
		result := android.GroupFixturePreparers(
			java.PrepareForTestWithDexpreopt,
			java.PrepareForTestWithJavaSdkLibraryFiles,
			java.FixtureConfigureBootJars("com.android.art:core-oj"),
			PrepareForTestWithApexBuildComponents,
			prepareForTestWithArtApex,
			android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				variables.BuildFlags = map[string]string{
					"RELEASE_APEX_CONTRIBUTIONS_ART": tc.selectedArtApexContributions,
				}
			}),
		).RunTestWithBp(t, bp)

		dexBootJars := result.ModuleForTests("dex_bootjars", "android_common")
		rule := dexBootJars.Output(ruleFile)

		inputs := rule.Implicits.Strings()
		android.AssertStringListContains(t, tc.desc, inputs, tc.expectedProfile)
	}
}
