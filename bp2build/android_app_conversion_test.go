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

package bp2build

import (
	"android/soong/android"
	"android/soong/java"

	"testing"
)

func runAndroidAppTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	runBp2BuildTestCase(t, registerAndroidAppModuleTypes, tc)
}

func registerAndroidAppModuleTypes(ctx android.RegistrationContext) {
}

func TestMinimalAndroidApp(t *testing.T) {
	runAndroidAppTestCase(t, bp2buildTestCase{
		description:                "Android app - simple example",
		moduleTypeUnderTest:        "android_app",
		moduleTypeUnderTestFactory: java.AndroidAppFactory,
		filesystem: map[string]string{
			"app.java":            "",
			"res/res.png":         "",
			"AndroidManifest.xml": "",
		},
		blueprint: `
android_app {
        name: "TestApp",
        srcs: ["app.java"],
        sdk_version: "current",
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("android_binary", "TestApp", attrNameToString{
				"srcs":           `["app.java"]`,
				"manifest":       `"AndroidManifest.xml"`,
				"resource_files": `["res/res.png"]`,
			}),
		}})
}

func TestAndroidAppAllSupportedFields(t *testing.T) {
	runAndroidAppTestCase(t, bp2buildTestCase{
		description:                "Android app - all supported fields",
		moduleTypeUnderTest:        "android_app",
		moduleTypeUnderTestFactory: java.AndroidAppFactory,
		filesystem: map[string]string{
			"app.java":                     "",
			"resa/res.png":                 "",
			"resb/res.png":                 "",
			"manifest/AndroidManifest.xml": "",
		},
		blueprint: simpleModuleDoNotConvertBp2build("android_app", "static_lib_dep") + `
android_app {
        name: "TestApp",
        srcs: ["app.java"],
        sdk_version: "current",
        package_name: "com.google",
        resource_dirs: ["resa", "resb"],
        manifest: "manifest/AndroidManifest.xml",
        static_libs: ["static_lib_dep"],
        java_version: "7",
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("android_binary", "TestApp", attrNameToString{
				"srcs":     `["app.java"]`,
				"manifest": `"manifest/AndroidManifest.xml"`,
				"resource_files": `[
        "resa/res.png",
        "resb/res.png",
    ]`,
				"custom_package": `"com.google"`,
				"deps":           `[":static_lib_dep"]`,
				"javacopts":      `["-source 1.7 -target 1.7"]`,
			}),
		}})
}

func TestAndroidAppArchVariantSrcs(t *testing.T) {
	runAndroidAppTestCase(t, bp2buildTestCase{
		description:                "Android app - arch variant srcs",
		moduleTypeUnderTest:        "android_app",
		moduleTypeUnderTestFactory: java.AndroidAppFactory,
		filesystem: map[string]string{
			"arm.java":            "",
			"x86.java":            "",
			"res/res.png":         "",
			"AndroidManifest.xml": "",
		},
		blueprint: `
android_app {
        name: "TestApp",
        sdk_version: "current",
        arch: {
			arm: {
				srcs: ["arm.java"],
			},
			x86: {
				srcs: ["x86.java"],
			}
		}
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("android_binary", "TestApp", attrNameToString{
				"srcs": `select({
        "//build/bazel/platforms/arch:arm": ["arm.java"],
        "//build/bazel/platforms/arch:x86": ["x86.java"],
        "//conditions:default": [],
    })`,
				"manifest":       `"AndroidManifest.xml"`,
				"resource_files": `["res/res.png"]`,
			}),
		}})
}
