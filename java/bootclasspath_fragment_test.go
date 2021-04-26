// Copyright (C) 2021 The Android Open Source Project
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

package java

import (
	"testing"

	"android/soong/android"
	"android/soong/dexpreopt"
)

// Contains some simple tests for bootclasspath_fragment logic, additional tests can be found in
// apex/bootclasspath_fragment_test.go as the ART boot image requires modules from the ART apex.

var prepareForTestWithBootclasspathFragment = android.GroupFixturePreparers(
	PrepareForTestWithJavaDefaultModules,
	dexpreopt.PrepareForTestByEnablingDexpreopt,
)

func TestUnknownBootclasspathFragment(t *testing.T) {
	prepareForTestWithBootclasspathFragment.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(
			`\Qimage_name: Unknown image name "unknown", expected one of art, boot\E`)).
		RunTestWithBp(t, `
			bootclasspath_fragment {
				name: "unknown-bootclasspath-fragment",
				image_name: "unknown",
			}
		`)
}

func TestUnknownBootclasspathFragmentImageName(t *testing.T) {
	prepareForTestWithBootclasspathFragment.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(
			`\Qimage_name: Unknown image name "unknown", expected one of art, boot\E`)).
		RunTestWithBp(t, `
			bootclasspath_fragment {
				name: "unknown-bootclasspath-fragment",
				image_name: "unknown",
			}
		`)
}

func TestUnknownPrebuiltBootclasspathFragment(t *testing.T) {
	prepareForTestWithBootclasspathFragment.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(
			`\Qimage_name: Unknown image name "unknown", expected one of art, boot\E`)).
		RunTestWithBp(t, `
			prebuilt_bootclasspath_fragment {
				name: "unknown-bootclasspath-fragment",
				image_name: "unknown",
			}
		`)
}

func TestBootclasspathFragmentInconsistentArtConfiguration_Platform(t *testing.T) {
	android.GroupFixturePreparers(
		prepareForTestWithBootclasspathFragment,
		dexpreopt.FixtureSetArtBootJars("platform:foo", "apex:bar"),
	).
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(
			`\QArtApexJars is invalid as it requests a platform variant of "foo"\E`)).
		RunTestWithBp(t, `
			bootclasspath_fragment {
				name: "bootclasspath-fragment",
				image_name: "art",
				apex_available: [
					"apex",
				],
			}
		`)
}

func TestBootclasspathFragmentInconsistentArtConfiguration_ApexMixture(t *testing.T) {
	android.GroupFixturePreparers(
		prepareForTestWithBootclasspathFragment,
		dexpreopt.FixtureSetArtBootJars("apex1:foo", "apex2:bar"),
	).
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(
			`\QArtApexJars configuration is inconsistent, expected all jars to be in the same apex but it specifies apex "apex1" and "apex2"\E`)).
		RunTestWithBp(t, `
			bootclasspath_fragment {
				name: "bootclasspath-fragment",
				image_name: "art",
				apex_available: [
					"apex1",
					"apex2",
				],
			}
		`)
}

func TestBootclasspathFragmentWithoutImageNameOrContents(t *testing.T) {
	prepareForTestWithBootclasspathFragment.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(
			`\Qneither of the "image_name" and "contents" properties\E`)).
		RunTestWithBp(t, `
			bootclasspath_fragment {
				name: "bootclasspath-fragment",
			}
		`)
}

func TestBootclasspathFragmentWithImageNameAndContents(t *testing.T) {
	prepareForTestWithBootclasspathFragment.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(
			`\Qboth of the "image_name" and "contents" properties\E`)).
		RunTestWithBp(t, `
			bootclasspath_fragment {
				name: "bootclasspath-fragment",
				image_name: "boot",
				contents: ["other"],
			}
		`)
}

func TestBootclasspathFragment_Coverage(t *testing.T) {
	prepareForTestWithFrameworkCoverage := android.FixtureMergeEnv(map[string]string{
		"EMMA_INSTRUMENT":           "true",
		"EMMA_INSTRUMENT_FRAMEWORK": "true",
	})

	prepareWithBp := android.FixtureWithRootAndroidBp(`
		bootclasspath_fragment {
			name: "myfragment",
			contents: [
				"mybootlib",
			],
			coverage: {
				contents: [
					"coveragelib",
				],
			},
		}

		java_library {
			name: "mybootlib",
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
		}

		java_library {
			name: "coveragelib",
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
		}
	`)

	checkContents := func(t *testing.T, result *android.TestResult, expected ...string) {
		module := result.Module("myfragment", "android_common").(*BootclasspathFragmentModule)
		android.AssertArrayString(t, "contents property", expected, module.properties.Contents)
	}

	t.Run("without coverage", func(t *testing.T) {
		result := android.GroupFixturePreparers(
			prepareForTestWithBootclasspathFragment,
			prepareWithBp,
		).RunTest(t)
		checkContents(t, result, "mybootlib")
	})

	t.Run("with coverage", func(t *testing.T) {
		result := android.GroupFixturePreparers(
			prepareForTestWithBootclasspathFragment,
			prepareForTestWithFrameworkCoverage,
			prepareWithBp,
		).RunTest(t)
		checkContents(t, result, "mybootlib", "coveragelib")
	})
}
