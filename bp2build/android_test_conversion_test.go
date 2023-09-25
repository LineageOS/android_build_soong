// Copyright 2023 Google Inc. All rights reserved.
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

package bp2build

import (
	"android/soong/android"
	"android/soong/java"

	"testing"
)

func runAndroidTestTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, registerAndroidTestModuleTypes, tc)
}

func registerAndroidTestModuleTypes(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	ctx.RegisterModuleType("java_library", java.LibraryFactory)
}

func TestMinimalAndroidTest(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Android test - simple example",
		ModuleTypeUnderTest:        "android_test",
		ModuleTypeUnderTestFactory: java.AndroidTestFactory,
		Filesystem: map[string]string{
			"app.java":            "",
			"res/res.png":         "",
			"AndroidManifest.xml": "",
			"assets/asset.png":    "",
		},
		Blueprint: `
android_test {
		name: "TestApp",
		srcs: ["app.java"],
		sdk_version: "current",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_test", "TestApp", AttrNameToString{
				"srcs":           `["app.java"]`,
				"manifest":       `"AndroidManifest.xml"`,
				"resource_files": `["res/res.png"]`,
				"sdk_version":    `"current"`,
				"assets":         `["assets/asset.png"]`,
				"assets_dir":     `"assets"`,
				// no need for optimize = False because it's false for
				// android_test by default
			}),
		}})
}

func TestAndroidTest_OptimizationEnabled(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Android test - simple example",
		ModuleTypeUnderTest:        "android_test",
		ModuleTypeUnderTestFactory: java.AndroidTestFactory,
		Filesystem: map[string]string{
			"app.java":            "",
			"res/res.png":         "",
			"AndroidManifest.xml": "",
			"assets/asset.png":    "",
		},
		Blueprint: `
android_test {
		name: "TestApp",
		srcs: ["app.java"],
		sdk_version: "current",
		optimize: {
			enabled: true,
			shrink: true,
			optimize: true,
			obfuscate: true,
		},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_test", "TestApp", AttrNameToString{
				"srcs":           `["app.java"]`,
				"manifest":       `"AndroidManifest.xml"`,
				"resource_files": `["res/res.png"]`,
				"sdk_version":    `"current"`,
				"assets":         `["assets/asset.png"]`,
				"assets_dir":     `"assets"`,
				// optimize = True because it's false for android_test by
				// default
				"optimize": `True`,
			}),
		}})
}

func TestMinimalAndroidTestHelperApp(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Android test helper app - simple example",
		ModuleTypeUnderTest:        "android_test_helper_app",
		ModuleTypeUnderTestFactory: java.AndroidTestHelperAppFactory,
		Filesystem: map[string]string{
			"app.java":            "",
			"res/res.png":         "",
			"AndroidManifest.xml": "",
			"assets/asset.png":    "",
		},
		Blueprint: `
android_test_helper_app {
		name: "TestApp",
		srcs: ["app.java"],
		sdk_version: "current",
		optimize: {
			shrink: true,
			optimize: true,
			obfuscate: true,
		},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_binary", "TestApp", AttrNameToString{
				"srcs":           `["app.java"]`,
				"manifest":       `"AndroidManifest.xml"`,
				"resource_files": `["res/res.png"]`,
				"sdk_version":    `"current"`,
				"assets":         `["assets/asset.png"]`,
				"assets_dir":     `"assets"`,
				"testonly":       `True`,
				// no need for optimize = True because it's true for
				// android_test_helper_app by default
			}),
		}})
}

func TestAndroidTestHelperApp_OptimizationDisabled(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Android test helper app - simple example",
		ModuleTypeUnderTest:        "android_test_helper_app",
		ModuleTypeUnderTestFactory: java.AndroidTestHelperAppFactory,
		Filesystem: map[string]string{
			"app.java":            "",
			"res/res.png":         "",
			"AndroidManifest.xml": "",
			"assets/asset.png":    "",
		},
		Blueprint: `
android_test_helper_app {
		name: "TestApp",
		srcs: ["app.java"],
		sdk_version: "current",
		optimize: {
			enabled: false,
		},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_binary", "TestApp", AttrNameToString{
				"srcs":           `["app.java"]`,
				"manifest":       `"AndroidManifest.xml"`,
				"resource_files": `["res/res.png"]`,
				"sdk_version":    `"current"`,
				"assets":         `["assets/asset.png"]`,
				"assets_dir":     `"assets"`,
				"testonly":       `True`,
				// optimize = False because it's true for
				// android_test_helper_app by default
				"optimize": `False`,
			}),
		}})
}
