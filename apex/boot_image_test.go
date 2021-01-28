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
	checkBootImage(t, ctx, "framework-boot-image", "platform:foo,platform:bar")

	// Make sure that the art-boot-image is using the correct configuration.
	checkBootImage(t, ctx, "art-boot-image", "com.android.art:baz,com.android.art:quuz")
}

func checkBootImage(t *testing.T, ctx *android.TestContext, moduleName string, expectedConfiguredModules string) {
	t.Helper()

	bootImage := ctx.ModuleForTests(moduleName, "android_common").Module().(*java.BootImageModule)

	bootImageInfo := ctx.ModuleProvider(bootImage, java.BootImageInfoProvider).(java.BootImageInfo)
	modules := bootImageInfo.Modules()
	if actual := modules.String(); actual != expectedConfiguredModules {
		t.Errorf("invalid modules for %s: expected %q, actual %q", moduleName, expectedConfiguredModules, actual)
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
