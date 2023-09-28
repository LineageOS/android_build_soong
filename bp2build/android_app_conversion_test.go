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
	"android/soong/cc"
	"android/soong/java"

	"testing"
)

func runAndroidAppTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, registerAndroidAppModuleTypes, tc)
}

func registerAndroidAppModuleTypes(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	ctx.RegisterModuleType("java_library", java.LibraryFactory)
	ctx.RegisterModuleType("cc_library_shared", cc.LibrarySharedFactory)
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
			"assets/asset.png":    "",
		},
		Blueprint: `
android_app {
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
			"assets_/asset.png":            "",
		},
		StubbedBuildDefinitions: []string{"static_lib_dep", "jni_lib"},
		Blueprint: simpleModule("android_app", "static_lib_dep") +
			simpleModule("cc_library_shared", "jni_lib") + `
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
	required: ["static_lib_dep"],
	asset_dirs: ["assets_"],
	optimize: {
		enabled: true,
		optimize: false,
		proguard_flags_files: ["proguard.flags"],
		shrink: false,
		obfuscate: false,
		ignore_warnings: true,
	},
	jni_libs: ["jni_lib"],
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
				"assets":         `["assets_/asset.png"]`,
				"assets_dir":     `"assets_"`,
				"custom_package": `"com.google"`,
				"deps": `[
        ":static_lib_dep",
        ":jni_lib",
    ]`,
				"java_version":     `"7"`,
				"sdk_version":      `"current"`,
				"certificate_name": `"foocert"`,
				"proguard_specs": `[
        "proguard.flags",
        ":TestApp_proguard_flags",
    ]`,
			}),
			MakeBazelTarget("genrule", "TestApp_proguard_flags", AttrNameToString{
				"outs": `["TestApp_proguard.flags"]`,
				"cmd":  `"echo -ignorewarning -dontshrink -dontoptimize -dontobfuscate > $(OUTS)"`,
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
	},
	optimize: {
		enabled: false,
	},
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
				"sdk_version":    `"current"`,
				"optimize":       `False`,
			}),
		}})
}

func TestAndroidAppCertIsModule(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Android app - cert is module",
		ModuleTypeUnderTest:        "android_app",
		ModuleTypeUnderTestFactory: java.AndroidAppFactory,
		Filesystem:                 map[string]string{},
		StubbedBuildDefinitions:    []string{"foocert"},
		Blueprint: simpleModule("filegroup", "foocert") + `
android_app {
	name: "TestApp",
	certificate: ":foocert",
	sdk_version: "current",
	optimize: {
		enabled: false,
	},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_binary", "TestApp", AttrNameToString{
				"certificate":    `":foocert"`,
				"manifest":       `"AndroidManifest.xml"`,
				"resource_files": `[]`,
				"sdk_version":    `"current"`, // use as default
				"optimize":       `False`,
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
	sdk_version: "current",
	optimize: {
		enabled: false,
	},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_binary", "TestApp", AttrNameToString{
				"certificate":    `"foocert"`,
				"manifest":       `"AndroidManifest.xml"`,
				"resource_files": `[]`,
				"sdk_version":    `"current"`, // use as default
				"optimize":       `False`,
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
	sdk_version: "current",
	optimize: {
		enabled: false,
	},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_binary", "TestApp", AttrNameToString{
				"certificate_name": `"foocert"`,
				"manifest":         `"AndroidManifest.xml"`,
				"resource_files":   `[]`,
				"sdk_version":      `"current"`, // use as default
				"optimize":         `False`,
			}),
		}})
}

func TestAndroidAppLibs(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Android app with libs",
		ModuleTypeUnderTest:        "android_app",
		ModuleTypeUnderTestFactory: java.AndroidAppFactory,
		Filesystem:                 map[string]string{},
		StubbedBuildDefinitions:    []string{"barLib"},
		Blueprint: simpleModule("java_library", "barLib") + `
android_app {
	name: "foo",
	libs: ["barLib"],
	sdk_version: "current",
	optimize: {
		enabled: false,
	},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_binary", "foo", AttrNameToString{
				"manifest":       `"AndroidManifest.xml"`,
				"resource_files": `[]`,
				"deps":           `[":barLib-neverlink"]`,
				"sdk_version":    `"current"`, // use as default
				"optimize":       `False`,
			}),
		}})
}

func TestAndroidAppKotlinSrcs(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Android app with kotlin sources and common_srcs",
		ModuleTypeUnderTest:        "android_app",
		ModuleTypeUnderTestFactory: java.AndroidAppFactory,
		Filesystem: map[string]string{
			"res/res.png": "",
		},
		StubbedBuildDefinitions: []string{"foocert", "barLib"},
		Blueprint: simpleModule("filegroup", "foocert") +
			simpleModule("java_library", "barLib") + `
android_app {
	name: "foo",
	srcs: ["a.java", "b.kt"],
	certificate: ":foocert",
	manifest: "fooManifest.xml",
	libs: ["barLib"],
	sdk_version: "current",
	optimize: {
		enabled: false,
	},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_library", "foo_kt", AttrNameToString{
				"srcs": `[
        "a.java",
        "b.kt",
    ]`,
				"manifest":       `"fooManifest.xml"`,
				"resource_files": `["res/res.png"]`,
				"deps":           `[":barLib-neverlink"]`,
				"sdk_version":    `"current"`, // use as default
			}),
			MakeBazelTarget("android_binary", "foo", AttrNameToString{
				"deps":        `[":foo_kt"]`,
				"certificate": `":foocert"`,
				"manifest":    `"fooManifest.xml"`,
				"sdk_version": `"current"`, // use as default
				"optimize":    `False`,
			}),
		}})
}

func TestAndroidAppCommonSrcs(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Android app with common_srcs",
		ModuleTypeUnderTest:        "android_app",
		ModuleTypeUnderTestFactory: java.AndroidAppFactory,
		Filesystem: map[string]string{
			"res/res.png": "",
		},
		StubbedBuildDefinitions: []string{"barLib"},
		Blueprint: `
android_app {
	name: "foo",
	srcs: ["a.java"],
	common_srcs: ["b.kt"],
	manifest: "fooManifest.xml",
	libs:        ["barLib"],
	sdk_version: "current",
	optimize: {
		enabled: false,
	},
}
java_library{
	name:   "barLib",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_library", "foo_kt", AttrNameToString{
				"srcs":           `["a.java"]`,
				"common_srcs":    `["b.kt"]`,
				"manifest":       `"fooManifest.xml"`,
				"resource_files": `["res/res.png"]`,
				"deps":           `[":barLib-neverlink"]`,
				"sdk_version":    `"current"`, // use as default
			}),
			MakeBazelTarget("android_binary", "foo", AttrNameToString{
				"deps":        `[":foo_kt"]`,
				"manifest":    `"fooManifest.xml"`,
				"sdk_version": `"current"`, // use as default
				"optimize":    `False`,
			}),
		}})
}

func TestAndroidAppKotlinCflags(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Android app with kotlincflags",
		ModuleTypeUnderTest:        "android_app",
		ModuleTypeUnderTestFactory: java.AndroidAppFactory,
		Filesystem: map[string]string{
			"res/res.png": "",
		},
		Blueprint: `
android_app {
	name: "foo",
	srcs: ["a.java", "b.kt"],
	manifest: "fooManifest.xml",
	kotlincflags: ["-flag1", "-flag2"],
	sdk_version: "current",
	optimize: {
		enabled: false,
	},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_library", "foo_kt", AttrNameToString{
				"srcs": `[
        "a.java",
        "b.kt",
    ]`,
				"manifest":       `"fooManifest.xml"`,
				"resource_files": `["res/res.png"]`,
				"kotlincflags": `[
        "-flag1",
        "-flag2",
    ]`,
				"sdk_version": `"current"`, // use as default
			}),
			MakeBazelTarget("android_binary", "foo", AttrNameToString{
				"deps":        `[":foo_kt"]`,
				"manifest":    `"fooManifest.xml"`,
				"sdk_version": `"current"`,
				"optimize":    `False`,
			}),
		}})
}

func TestAndroidAppManifestSdkVersionsProvided(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Android app with value for min_sdk_version",
		ModuleTypeUnderTest:        "android_app",
		ModuleTypeUnderTestFactory: java.AndroidAppFactory,
		Filesystem:                 map[string]string{},
		Blueprint: `
android_app {
	name: "foo",
	sdk_version: "current",
	min_sdk_version: "24",
	target_sdk_version: "29",
	optimize: {
		enabled: false,
	},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_binary", "foo", AttrNameToString{
				"manifest":       `"AndroidManifest.xml"`,
				"resource_files": `[]`,
				"manifest_values": `{
        "minSdkVersion": "24",
        "targetSdkVersion": "29",
    }`,
				"sdk_version": `"current"`,
				"optimize":    `False`,
			}),
		}})
}

func TestAndroidAppMinAndTargetSdkDefaultToSdkVersion(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Android app with value for sdk_version",
		ModuleTypeUnderTest:        "android_app",
		ModuleTypeUnderTestFactory: java.AndroidAppFactory,
		Filesystem:                 map[string]string{},
		Blueprint: `
android_app {
	name: "foo",
	sdk_version: "30",
	optimize: {
		enabled: false,
	},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("android_binary", "foo", AttrNameToString{
				"manifest":       `"AndroidManifest.xml"`,
				"resource_files": `[]`,
				"sdk_version":    `"30"`,
				"optimize":       `False`,
			}),
		}})
}

func TestFrameworkResConversion(t *testing.T) {
	runAndroidAppTestCase(t, Bp2buildTestCase{
		Description:                "Framework Res custom conversion",
		ModuleTypeUnderTest:        "android_app",
		ModuleTypeUnderTestFactory: java.AndroidAppFactory,
		Filesystem: map[string]string{
			"res/values/attrs.xml": "",
			"resource_zip.zip":     "",
		},
		Blueprint: `
android_app {
	name: "framework-res",
	resource_zips: [
		"resource_zip.zip",
	],
	certificate: "platform",
}

filegroup {
	name: "framework-res-package-jar",
	srcs: [":framework-res{.export-package.apk}"],
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("framework_resources", "framework-res", AttrNameToString{
				"certificate_name":       `"platform"`,
				"manifest":               `"AndroidManifest.xml"`,
				"resource_files":         `["res/values/attrs.xml"]`,
				"resource_zips":          `["resource_zip.zip"]`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
			}),
			MakeBazelTargetNoRestrictions("filegroup", "framework-res-package-jar", AttrNameToString{
				"srcs": `[":framework-res.export-package.apk"]`,
			}),
		}})

}
