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

func runAndroidAppTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, registerAndroidAppModuleTypes, tc)
}

func registerAndroidAppModuleTypes(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
}

func TestMinimalAndroidApp(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Android app - simple example",
		ModuleTypeUnderTest:        "android_app",
		ModuleTypeUnderTestFactory: java.AndroidAppFactory,
		Filesystem: map[string]string{
			"app.java":            "",
			"res/res.png":         "",
			"AndroidManifest.xml": "",
		},
		Blueprint: `
android_app {
        name: "TestApp",
        srcs: ["app.java"],
        sdk_version: "current",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_binary", "TestApp", AttrNameToString{
				"srcs":           `["app.java"]`,
				"manifest":       `"AndroidManifest.xml"`,
				"resource_files": `["res/res.png"]`,
			}),
		}})
}

func TestAndroidAppAllSupportedFields(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Android app - all supported fields",
		ModuleTypeUnderTest:        "android_app",
		ModuleTypeUnderTestFactory: java.AndroidAppFactory,
		Filesystem: map[string]string{
			"app.java":                     "",
			"resa/res.png":                 "",
			"resb/res.png":                 "",
			"manifest/AndroidManifest.xml": "",
		},
		Blueprint: simpleModuleDoNotConvertBp2build("android_app", "static_lib_dep") + `
android_app {
        name: "TestApp",
        srcs: ["app.java"],
        sdk_version: "current",
        package_name: "com.google",
        resource_dirs: ["resa", "resb"],
        manifest: "manifest/AndroidManifest.xml",
        static_libs: ["static_lib_dep"],
        java_version: "7",
        certificate: "foocert",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_binary", "TestApp", AttrNameToString{
				"srcs":     `["app.java"]`,
				"manifest": `"manifest/AndroidManifest.xml"`,
				"resource_files": `[
        "resa/res.png",
        "resb/res.png",
    ]`,
				"custom_package":   `"com.google"`,
				"deps":             `[":static_lib_dep"]`,
				"javacopts":        `["-source 1.7 -target 1.7"]`,
				"certificate_name": `"foocert"`,
			}),
		}})
}

func TestAndroidAppArchVariantSrcs(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Android app - arch variant srcs",
		ModuleTypeUnderTest:        "android_app",
		ModuleTypeUnderTestFactory: java.AndroidAppFactory,
		Filesystem: map[string]string{
			"arm.java":            "",
			"x86.java":            "",
			"res/res.png":         "",
			"AndroidManifest.xml": "",
		},
		Blueprint: `
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
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_binary", "TestApp", AttrNameToString{
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

func TestAndroidAppCertIsModule(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Android app - cert is module",
		ModuleTypeUnderTest:        "android_app",
		ModuleTypeUnderTestFactory: java.AndroidAppFactory,
		Filesystem:                 map[string]string{},
		Blueprint: simpleModuleDoNotConvertBp2build("filegroup", "foocert") + `
android_app {
        name: "TestApp",
        certificate: ":foocert",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_binary", "TestApp", AttrNameToString{
				"certificate":    `":foocert"`,
				"manifest":       `"AndroidManifest.xml"`,
				"resource_files": `[]`,
			}),
		}})
}

func TestAndroidAppCertIsSrcFile(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Android app - cert is src file",
		ModuleTypeUnderTest:        "android_app",
		ModuleTypeUnderTestFactory: java.AndroidAppFactory,
		Filesystem: map[string]string{
			"foocert": "",
		},
		Blueprint: `
android_app {
        name: "TestApp",
        certificate: "foocert",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_binary", "TestApp", AttrNameToString{
				"certificate":    `"foocert"`,
				"manifest":       `"AndroidManifest.xml"`,
				"resource_files": `[]`,
			}),
		}})
}

func TestAndroidAppCertIsNotSrcOrModule(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Android app - cert is not src or module",
		ModuleTypeUnderTest:        "android_app",
		ModuleTypeUnderTestFactory: java.AndroidAppFactory,
		Filesystem:                 map[string]string{
			// deliberate empty
		},
		Blueprint: `
android_app {
        name: "TestApp",
        certificate: "foocert",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_binary", "TestApp", AttrNameToString{
				"certificate_name": `"foocert"`,
				"manifest":         `"AndroidManifest.xml"`,
				"resource_files":   `[]`,
			}),
		}})
}
