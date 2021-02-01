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
	"reflect"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/dexpreopt"
	"android/soong/java"
)

// Contains tests for boot_image logic from java/boot_image.go as the ART boot image requires
// modules from the ART apex.

func TestBootImages(t *testing.T) {
	ctx, _ := testApex(t, `
		java_sdk_library {
			name: "foo",
			srcs: ["b.java"],
			unsafe_ignore_missing_latest_api: true,
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
		}

		boot_image {
			name: "framework-boot-image",
			image_name: "boot",
		}
`,
		// Configure some libraries in the art and framework boot images.
		withArtBootImageJars("com.android.art:baz", "com.android.art:quuz"),
		withFrameworkBootImageJars("platform:foo", "platform:bar"),
		withFiles(filesForSdkLibrary),
		// Some additional files needed for the art apex.
		withFiles(map[string][]byte{
			"com.android.art.avbpubkey":                          nil,
			"com.android.art.pem":                                nil,
			"system/sepolicy/apex/com.android.art-file_contexts": nil,
		}),
	)

	// Make sure that the framework-boot-image is using the correct configuration.
	checkBootImage(t, ctx, "framework-boot-image", "platform:foo,platform:bar", `
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
	checkBootImage(t, ctx, "art-boot-image", "com.android.art:baz,com.android.art:quuz", `
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

func checkBootImage(t *testing.T, ctx *android.TestContext, moduleName string, expectedConfiguredModules string, expectedBootImageFiles string) {
	t.Helper()

	bootImage := ctx.ModuleForTests(moduleName, "android_common").Module().(*java.BootImageModule)

	bootImageInfo := ctx.ModuleProvider(bootImage, java.BootImageInfoProvider).(java.BootImageInfo)
	modules := bootImageInfo.Modules()
	if actual := modules.String(); actual != expectedConfiguredModules {
		t.Errorf("invalid modules for %s: expected %q, actual %q", moduleName, expectedConfiguredModules, actual)
	}

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
	if expected, actual := strings.TrimSpace(expectedBootImageFiles), strings.TrimSpace(strings.Join(allPaths, "\n")); !reflect.DeepEqual(expected, actual) {
		t.Errorf("invalid paths for %s: expected \n%s, actual \n%s", moduleName, expected, actual)
	}
}

func modifyDexpreoptConfig(configModifier func(dexpreoptConfig *dexpreopt.GlobalConfig)) func(fs map[string][]byte, config android.Config) {
	return func(fs map[string][]byte, config android.Config) {
		// Initialize the dexpreopt GlobalConfig to an empty structure. This has no effect if it has
		// already been set.
		pathCtx := android.PathContextForTesting(config)
		dexpreoptConfig := dexpreopt.GlobalConfigForTests(pathCtx)
		dexpreopt.SetTestGlobalConfig(config, dexpreoptConfig)

		// Retrieve the existing configuration and modify it.
		dexpreoptConfig = dexpreopt.GetGlobalConfig(pathCtx)
		configModifier(dexpreoptConfig)
	}
}

func withArtBootImageJars(bootJars ...string) func(fs map[string][]byte, config android.Config) {
	return modifyDexpreoptConfig(func(dexpreoptConfig *dexpreopt.GlobalConfig) {
		dexpreoptConfig.ArtApexJars = android.CreateTestConfiguredJarList(bootJars)
	})
}

func withFrameworkBootImageJars(bootJars ...string) func(fs map[string][]byte, config android.Config) {
	return modifyDexpreoptConfig(func(dexpreoptConfig *dexpreopt.GlobalConfig) {
		dexpreoptConfig.BootJars = android.CreateTestConfiguredJarList(bootJars)
	})
}

func TestBootImageInApex(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			boot_images: [
				"mybootimage",
			],
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
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
			installable: true,
		}

		boot_image {
			name: "mybootimage",
			image_name: "boot",
			apex_available: [
				"myapex",
			],
		}
`,
		// Configure some libraries in the framework boot image.
		withFrameworkBootImageJars("platform:foo", "platform:bar"),
	)

	ensureExactContents(t, ctx, "myapex", "android_common_myapex_image", []string{
		"javalib/arm/boot-bar.art",
		"javalib/arm/boot-bar.oat",
		"javalib/arm/boot-bar.vdex",
		"javalib/arm/boot-foo.art",
		"javalib/arm/boot-foo.oat",
		"javalib/arm/boot-foo.vdex",
		"javalib/arm64/boot-bar.art",
		"javalib/arm64/boot-bar.oat",
		"javalib/arm64/boot-bar.vdex",
		"javalib/arm64/boot-foo.art",
		"javalib/arm64/boot-foo.oat",
		"javalib/arm64/boot-foo.vdex",
	})
}

// TODO(b/177892522) - add test for host apex.
