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

package apex

import (
	"fmt"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/java"
)

// Contains tests for bootclasspath_fragment logic from java/bootclasspath_fragment.go as the ART
// bootclasspath_fragment requires modules from the ART apex.

var prepareForTestWithBootclasspathFragment = android.GroupFixturePreparers(
	java.PrepareForTestWithDexpreopt,
	PrepareForTestWithApexBuildComponents,
)

// Some additional files needed for the art apex.
var prepareForTestWithArtApex = android.FixtureMergeMockFs(android.MockFS{
	"com.android.art.avbpubkey":                          nil,
	"com.android.art.pem":                                nil,
	"system/sepolicy/apex/com.android.art-file_contexts": nil,
})

func TestBootclasspathFragments(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithBootclasspathFragment,
		// Configure some libraries in the art bootclasspath_fragment and platform_bootclasspath.
		java.FixtureConfigureBootJars("com.android.art:baz", "com.android.art:quuz", "platform:foo", "platform:bar"),
		prepareForTestWithArtApex,

		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("foo"),
	).RunTestWithBp(t, `
		java_sdk_library {
			name: "foo",
			srcs: ["b.java"],
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
			installable: true,
		}

		apex {
			name: "com.android.art",
			key: "com.android.art.key",
 			java_libs: [
				"baz",
				"quuz",
			],
			updatable: false,
		}

		apex_key {
			name: "com.android.art.key",
			public_key: "com.android.art.avbpubkey",
			private_key: "com.android.art.pem",
		}

		java_library {
			name: "baz",
			apex_available: [
				"com.android.art",
			],
			srcs: ["b.java"],
		}

		java_library {
			name: "quuz",
			apex_available: [
				"com.android.art",
			],
			srcs: ["b.java"],
		}

		bootclasspath_fragment {
			name: "art-bootclasspath-fragment",
			image_name: "art",
			// Must match the "com.android.art:" entries passed to FixtureConfigureBootJars above.
			contents: ["baz", "quuz"],
			apex_available: [
				"com.android.art",
			],
		}

		bootclasspath_fragment {
			name: "framework-bootclasspath-fragment",
			image_name: "boot",
		}
`,
	)

	// Make sure that the framework-bootclasspath-fragment is using the correct configuration.
	checkBootclasspathFragment(t, result, "framework-bootclasspath-fragment", "platform:foo,platform:bar", `
test_device/dex_bootjars/android/system/framework/arm/boot-foo.art
test_device/dex_bootjars/android/system/framework/arm/boot-foo.oat
test_device/dex_bootjars/android/system/framework/arm/boot-foo.vdex
test_device/dex_bootjars/android/system/framework/arm/boot-bar.art
test_device/dex_bootjars/android/system/framework/arm/boot-bar.oat
test_device/dex_bootjars/android/system/framework/arm/boot-bar.vdex
test_device/dex_bootjars/android/system/framework/arm64/boot-foo.art
test_device/dex_bootjars/android/system/framework/arm64/boot-foo.oat
test_device/dex_bootjars/android/system/framework/arm64/boot-foo.vdex
test_device/dex_bootjars/android/system/framework/arm64/boot-bar.art
test_device/dex_bootjars/android/system/framework/arm64/boot-bar.oat
test_device/dex_bootjars/android/system/framework/arm64/boot-bar.vdex
`)

	// Make sure that the art-bootclasspath-fragment is using the correct configuration.
	checkBootclasspathFragment(t, result, "art-bootclasspath-fragment", "com.android.art:baz,com.android.art:quuz", `
test_device/dex_artjars/android/apex/art_boot_images/javalib/arm/boot.art
test_device/dex_artjars/android/apex/art_boot_images/javalib/arm/boot.oat
test_device/dex_artjars/android/apex/art_boot_images/javalib/arm/boot.vdex
test_device/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-quuz.art
test_device/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-quuz.oat
test_device/dex_artjars/android/apex/art_boot_images/javalib/arm/boot-quuz.vdex
test_device/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.art
test_device/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.oat
test_device/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.vdex
test_device/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-quuz.art
test_device/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-quuz.oat
test_device/dex_artjars/android/apex/art_boot_images/javalib/arm64/boot-quuz.vdex
`)
}

func checkBootclasspathFragment(t *testing.T, result *android.TestResult, moduleName string, expectedConfiguredModules string, expectedBootclasspathFragmentFiles string) {
	t.Helper()

	bootclasspathFragment := result.ModuleForTests(moduleName, "android_common").Module().(*java.BootclasspathFragmentModule)

	bootclasspathFragmentInfo := result.ModuleProvider(bootclasspathFragment, java.BootclasspathFragmentApexContentInfoProvider).(java.BootclasspathFragmentApexContentInfo)
	modules := bootclasspathFragmentInfo.Modules()
	android.AssertStringEquals(t, "invalid modules for "+moduleName, expectedConfiguredModules, modules.String())

	// Get a list of all the paths in the boot image sorted by arch type.
	allPaths := []string{}
	bootImageFilesByArchType := bootclasspathFragmentInfo.AndroidBootImageFilesByArchType()
	for _, archType := range android.ArchTypeList() {
		if paths, ok := bootImageFilesByArchType[archType]; ok {
			for _, path := range paths {
				allPaths = append(allPaths, android.NormalizePathForTesting(path))
			}
		}
	}

	android.AssertTrimmedStringEquals(t, "invalid paths for "+moduleName, expectedBootclasspathFragmentFiles, strings.Join(allPaths, "\n"))
}

func TestBootclasspathFragmentInArtApex(t *testing.T) {
	commonPreparer := android.GroupFixturePreparers(
		prepareForTestWithBootclasspathFragment,
		prepareForTestWithArtApex,

		android.FixtureWithRootAndroidBp(`
		apex {
			name: "com.android.art",
			key: "com.android.art.key",
			bootclasspath_fragments: [
				"mybootclasspathfragment",
			],
			// bar (like foo) should be transitively included in this apex because it is part of the
			// mybootclasspathfragment bootclasspath_fragment. However, it is kept here to ensure that the
			// apex dedups the files correctly.
			java_libs: [
				"bar",
			],
			updatable: false,
		}

		apex_key {
			name: "com.android.art.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "foo",
			srcs: ["b.java"],
			installable: true,
			apex_available: [
				"com.android.art",
			],
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
			installable: true,
			apex_available: [
				"com.android.art",
			],
		}

		java_import {
			name: "foo",
			jars: ["foo.jar"],
			apex_available: [
				"com.android.art",
			],
		}

		java_import {
			name: "bar",
			jars: ["bar.jar"],
			apex_available: [
				"com.android.art",
			],
		}
	`),
	)

	contentsInsert := func(contents []string) string {
		insert := ""
		if contents != nil {
			insert = fmt.Sprintf(`contents: ["%s"],`, strings.Join(contents, `", "`))
		}
		return insert
	}

	addSource := func(contents ...string) android.FixturePreparer {
		text := fmt.Sprintf(`
			bootclasspath_fragment {
				name: "mybootclasspathfragment",
				image_name: "art",
				%s
				apex_available: [
					"com.android.art",
				],
			}
		`, contentsInsert(contents))

		return android.FixtureAddTextFile("art/build/boot/Android.bp", text)
	}

	addPrebuilt := func(prefer bool, contents ...string) android.FixturePreparer {
		text := fmt.Sprintf(`
			prebuilt_bootclasspath_fragment {
				name: "mybootclasspathfragment",
				image_name: "art",
				%s
				prefer: %t,
				apex_available: [
					"com.android.art",
				],
			}
		`, contentsInsert(contents), prefer)
		return android.FixtureAddTextFile("prebuilts/module_sdk/art/Android.bp", text)
	}

	t.Run("boot image files", func(t *testing.T) {
		result := android.GroupFixturePreparers(
			commonPreparer,

			// Configure some libraries in the art bootclasspath_fragment that match the source
			// bootclasspath_fragment's contents property.
			java.FixtureConfigureBootJars("com.android.art:foo", "com.android.art:bar"),
			addSource("foo", "bar"),

			// Make sure that a preferred prebuilt with consistent contents doesn't affect the apex.
			addPrebuilt(true, "foo", "bar"),
		).RunTest(t)

		ensureExactContents(t, result.TestContext, "com.android.art", "android_common_com.android.art_image", []string{
			"javalib/arm/boot.art",
			"javalib/arm/boot.oat",
			"javalib/arm/boot.vdex",
			"javalib/arm/boot-bar.art",
			"javalib/arm/boot-bar.oat",
			"javalib/arm/boot-bar.vdex",
			"javalib/arm64/boot.art",
			"javalib/arm64/boot.oat",
			"javalib/arm64/boot.vdex",
			"javalib/arm64/boot-bar.art",
			"javalib/arm64/boot-bar.oat",
			"javalib/arm64/boot-bar.vdex",
			"javalib/bar.jar",
			"javalib/foo.jar",
		})

		java.CheckModuleDependencies(t, result.TestContext, "com.android.art", "android_common_com.android.art_image", []string{
			`bar`,
			`com.android.art.key`,
			`mybootclasspathfragment`,
		})
	})

	t.Run("source with inconsistency between config and contents", func(t *testing.T) {
		android.GroupFixturePreparers(
			commonPreparer,

			// Create an inconsistency between the ArtApexJars configuration and the art source
			// bootclasspath_fragment module's contents property.
			java.FixtureConfigureBootJars("com.android.art:foo"),
			addSource("foo", "bar"),
		).
			ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(`\QArtApexJars configuration specifies []string{"foo"}, contents property specifies []string{"foo", "bar"}\E`)).
			RunTest(t)
	})

	t.Run("prebuilt with inconsistency between config and contents", func(t *testing.T) {
		android.GroupFixturePreparers(
			commonPreparer,

			// Create an inconsistency between the ArtApexJars configuration and the art
			// prebuilt_bootclasspath_fragment module's contents property.
			java.FixtureConfigureBootJars("com.android.art:foo"),
			addPrebuilt(false, "foo", "bar"),
		).
			ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(`\QArtApexJars configuration specifies []string{"foo"}, contents property specifies []string{"foo", "bar"}\E`)).
			RunTest(t)
	})

	t.Run("preferred prebuilt with inconsistency between config and contents", func(t *testing.T) {
		android.GroupFixturePreparers(
			commonPreparer,

			// Create an inconsistency between the ArtApexJars configuration and the art
			// prebuilt_bootclasspath_fragment module's contents property.
			java.FixtureConfigureBootJars("com.android.art:foo"),
			addPrebuilt(true, "foo", "bar"),

			// Source contents property is consistent with the config.
			addSource("foo"),
		).
			ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(`\QArtApexJars configuration specifies []string{"foo"}, contents property specifies []string{"foo", "bar"}\E`)).
			RunTest(t)
	})

	t.Run("source preferred and prebuilt with inconsistency between config and contents", func(t *testing.T) {
		android.GroupFixturePreparers(
			commonPreparer,

			// Create an inconsistency between the ArtApexJars configuration and the art
			// prebuilt_bootclasspath_fragment module's contents property.
			java.FixtureConfigureBootJars("com.android.art:foo"),
			addPrebuilt(false, "foo", "bar"),

			// Source contents property is consistent with the config.
			addSource("foo"),

			// This should pass because while the prebuilt is inconsistent with the configuration it is
			// not actually used.
		).RunTest(t)
	})
}

func TestBootclasspathFragmentInPrebuiltArtApex(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithBootclasspathFragment,
		prepareForTestWithArtApex,

		android.FixtureMergeMockFs(android.MockFS{
			"com.android.art-arm64.apex": nil,
			"com.android.art-arm.apex":   nil,
		}),

		// Configure some libraries in the art bootclasspath_fragment.
		java.FixtureConfigureBootJars("com.android.art:foo", "com.android.art:bar"),
	).RunTestWithBp(t, `
		prebuilt_apex {
			name: "com.android.art",
			arch: {
				arm64: {
					src: "com.android.art-arm64.apex",
				},
				arm: {
					src: "com.android.art-arm.apex",
				},
			},
			exported_java_libs: ["foo", "bar"],
		}

		java_import {
			name: "foo",
			jars: ["foo.jar"],
			apex_available: [
				"com.android.art",
			],
		}

		java_import {
			name: "bar",
			jars: ["bar.jar"],
			apex_available: [
				"com.android.art",
			],
		}

		prebuilt_bootclasspath_fragment {
			name: "mybootclasspathfragment",
			image_name: "art",
			// Must match the "com.android.art:" entries passed to FixtureConfigureBootJars above.
			contents: ["foo", "bar"],
			apex_available: [
				"com.android.art",
			],
		}
	`)

	java.CheckModuleDependencies(t, result.TestContext, "com.android.art", "android_common", []string{
		`com.android.art.apex.selector`,
		`prebuilt_bar`,
		`prebuilt_foo`,
	})

	java.CheckModuleDependencies(t, result.TestContext, "mybootclasspathfragment", "android_common", []string{
		`dex2oatd`,
		`prebuilt_bar`,
		`prebuilt_foo`,
	})
}

func TestBootclasspathFragmentContentsNoName(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithBootclasspathFragment,
		prepareForTestWithMyapex,
	).RunTestWithBp(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			bootclasspath_fragments: [
				"mybootclasspathfragment",
			],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "foo",
			srcs: ["b.java"],
			installable: true,
			apex_available: [
				"myapex",
			],
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
			installable: true,
			apex_available: [
				"myapex",
			],
		}

		bootclasspath_fragment {
			name: "mybootclasspathfragment",
			contents: [
				"foo",
				"bar",
			],
			apex_available: [
				"myapex",
			],
		}
	`)

	ensureExactContents(t, result.TestContext, "myapex", "android_common_myapex_image", []string{
		// This does not include art, oat or vdex files as they are only included for the art boot
		// image.
		"javalib/bar.jar",
		"javalib/foo.jar",
	})

	java.CheckModuleDependencies(t, result.TestContext, "myapex", "android_common_myapex_image", []string{
		`myapex.key`,
		`mybootclasspathfragment`,
	})
}

// TODO(b/177892522) - add test for host apex.
