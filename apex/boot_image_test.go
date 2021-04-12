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
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/java"
)

// Contains tests for boot_image logic from java/boot_image.go as the ART boot image requires
// modules from the ART apex.

var prepareForTestWithBootImage = android.GroupFixturePreparers(
	java.PrepareForTestWithDexpreopt,
	PrepareForTestWithApexBuildComponents,
)

// Some additional files needed for the art apex.
var prepareForTestWithArtApex = android.FixtureMergeMockFs(android.MockFS{
	"com.android.art.avbpubkey":                          nil,
	"com.android.art.pem":                                nil,
	"system/sepolicy/apex/com.android.art-file_contexts": nil,
})

func TestBootImages(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithBootImage,
		// Configure some libraries in the art and framework boot images.
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

		boot_image {
			name: "art-boot-image",
			image_name: "art",
			apex_available: [
				"com.android.art",
			],
		}

		boot_image {
			name: "framework-boot-image",
			image_name: "boot",
		}
`,
	)

	// Make sure that the framework-boot-image is using the correct configuration.
	checkBootImage(t, result, "framework-boot-image", "platform:foo,platform:bar", `
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

	// Make sure that the art-boot-image is using the correct configuration.
	checkBootImage(t, result, "art-boot-image", "com.android.art:baz,com.android.art:quuz", `
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

func checkBootImage(t *testing.T, result *android.TestResult, moduleName string, expectedConfiguredModules string, expectedBootImageFiles string) {
	t.Helper()

	bootImage := result.ModuleForTests(moduleName, "android_common").Module().(*java.BootImageModule)

	bootImageInfo := result.ModuleProvider(bootImage, java.BootImageInfoProvider).(java.BootImageInfo)
	modules := bootImageInfo.Modules()
	android.AssertStringEquals(t, "invalid modules for "+moduleName, expectedConfiguredModules, modules.String())

	// Get a list of all the paths in the boot image sorted by arch type.
	allPaths := []string{}
	bootImageFilesByArchType := bootImageInfo.AndroidBootImageFilesByArchType()
	for _, archType := range android.ArchTypeList() {
		if paths, ok := bootImageFilesByArchType[archType]; ok {
			for _, path := range paths {
				allPaths = append(allPaths, android.NormalizePathForTesting(path))
			}
		}
	}

	android.AssertTrimmedStringEquals(t, "invalid paths for "+moduleName, expectedBootImageFiles, strings.Join(allPaths, "\n"))
}

func TestBootImageInArtApex(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithBootImage,
		prepareForTestWithArtApex,

		// Configure some libraries in the art boot image.
		java.FixtureConfigureBootJars("com.android.art:foo", "com.android.art:bar"),
	).RunTestWithBp(t, `
		apex {
			name: "com.android.art",
			key: "com.android.art.key",
			boot_images: [
				"mybootimage",
			],
			// bar (like foo) should be transitively included in this apex because it is part of the
			// mybootimage boot_image. However, it is kept here to ensure that the apex dedups the files
			// correctly.
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

		boot_image {
			name: "mybootimage",
			image_name: "art",
			apex_available: [
				"com.android.art",
			],
		}

		// Make sure that a preferred prebuilt doesn't affect the apex.
		prebuilt_boot_image {
			name: "mybootimage",
			image_name: "art",
			prefer: true,
			apex_available: [
				"com.android.art",
			],
		}
	`)

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
		`mybootimage`,
	})
}

func TestBootImageInPrebuiltArtApex(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithBootImage,
		prepareForTestWithArtApex,

		android.FixtureMergeMockFs(android.MockFS{
			"com.android.art-arm64.apex": nil,
			"com.android.art-arm.apex":   nil,
		}),

		// Configure some libraries in the art boot image.
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

		prebuilt_boot_image {
			name: "mybootimage",
			image_name: "art",
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

	java.CheckModuleDependencies(t, result.TestContext, "mybootimage", "android_common", []string{
		`dex2oatd`,
		`prebuilt_bar`,
		`prebuilt_foo`,
	})
}

func TestBootImageContentsNoName(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithBootImage,
		prepareForTestWithMyapex,
	).RunTestWithBp(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			boot_images: [
				"mybootimage",
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

		boot_image {
			name: "mybootimage",
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
		`mybootimage`,
	})
}

// TODO(b/177892522) - add test for host apex.
