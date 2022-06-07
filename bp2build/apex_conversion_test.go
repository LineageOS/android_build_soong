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
	"android/soong/apex"
	"android/soong/cc"
	"android/soong/java"
	"android/soong/sh"

	"testing"
)

func runApexTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	runBp2BuildTestCase(t, registerApexModuleTypes, tc)
}

func registerApexModuleTypes(ctx android.RegistrationContext) {
	// CC module types needed as they can be APEX dependencies
	cc.RegisterCCBuildComponents(ctx)

	ctx.RegisterModuleType("sh_binary", sh.ShBinaryFactory)
	ctx.RegisterModuleType("cc_binary", cc.BinaryFactory)
	ctx.RegisterModuleType("cc_library", cc.LibraryFactory)
	ctx.RegisterModuleType("apex_key", apex.ApexKeyFactory)
	ctx.RegisterModuleType("android_app_certificate", java.AndroidAppCertificateFactory)
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
}

func runOverrideApexTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	runBp2BuildTestCase(t, registerOverrideApexModuleTypes, tc)
}

func registerOverrideApexModuleTypes(ctx android.RegistrationContext) {
	// CC module types needed as they can be APEX dependencies
	cc.RegisterCCBuildComponents(ctx)

	ctx.RegisterModuleType("sh_binary", sh.ShBinaryFactory)
	ctx.RegisterModuleType("cc_binary", cc.BinaryFactory)
	ctx.RegisterModuleType("cc_library", cc.LibraryFactory)
	ctx.RegisterModuleType("apex_key", apex.ApexKeyFactory)
	ctx.RegisterModuleType("android_app_certificate", java.AndroidAppCertificateFactory)
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	ctx.RegisterModuleType("apex", apex.BundleFactory)
}

func TestApexBundleSimple(t *testing.T) {
	runApexTestCase(t, bp2buildTestCase{
		description:                "apex - example with all props, file_context is a module in same Android.bp",
		moduleTypeUnderTest:        "apex",
		moduleTypeUnderTestFactory: apex.BundleFactory,
		filesystem:                 map[string]string{},
		blueprint: `
apex_key {
	name: "com.android.apogee.key",
	public_key: "com.android.apogee.avbpubkey",
	private_key: "com.android.apogee.pem",
	bazel_module: { bp2build_available: false },
}

android_app_certificate {
	name: "com.android.apogee.certificate",
	certificate: "com.android.apogee",
	bazel_module: { bp2build_available: false },
}

cc_library {
	name: "native_shared_lib_1",
	bazel_module: { bp2build_available: false },
}

cc_library {
	name: "native_shared_lib_2",
	bazel_module: { bp2build_available: false },
}

cc_library {
	name: "pretend_prebuilt_1",
	bazel_module: { bp2build_available: false },
}

cc_library {
	name: "pretend_prebuilt_2",
	bazel_module: { bp2build_available: false },
}

filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [
		"com.android.apogee-file_contexts",
	],
	bazel_module: { bp2build_available: false },
}

cc_binary { name: "cc_binary_1", bazel_module: { bp2build_available: false } }
sh_binary { name: "sh_binary_2", bazel_module: { bp2build_available: false } }

apex {
	name: "com.android.apogee",
	manifest: "apogee_manifest.json",
	androidManifest: "ApogeeAndroidManifest.xml",
	file_contexts: ":com.android.apogee-file_contexts",
	min_sdk_version: "29",
	key: "com.android.apogee.key",
	certificate: "com.android.apogee.certificate",
	updatable: false,
	installable: false,
	compressible: false,
	native_shared_libs: [
	    "native_shared_lib_1",
	    "native_shared_lib_2",
	],
	binaries: [
		"cc_binary_1",
		"sh_binary_2",
	],
	prebuilts: [
	    "pretend_prebuilt_1",
	    "pretend_prebuilt_2",
	],
	package_name: "com.android.apogee.test.package",
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("apex", "com.android.apogee", attrNameToString{
				"android_manifest": `"ApogeeAndroidManifest.xml"`,
				"binaries": `[
        ":cc_binary_1",
        ":sh_binary_2",
    ]`,
				"certificate":     `":com.android.apogee.certificate"`,
				"file_contexts":   `":com.android.apogee-file_contexts"`,
				"installable":     "False",
				"key":             `":com.android.apogee.key"`,
				"manifest":        `"apogee_manifest.json"`,
				"min_sdk_version": `"29"`,
				"native_shared_libs_32": `[
        ":native_shared_lib_1",
        ":native_shared_lib_2",
    ]`,
				"native_shared_libs_64": `select({
        "//build/bazel/platforms/arch:arm64": [
            ":native_shared_lib_1",
            ":native_shared_lib_2",
        ],
        "//build/bazel/platforms/arch:x86_64": [
            ":native_shared_lib_1",
            ":native_shared_lib_2",
        ],
        "//conditions:default": [],
    })`,
				"prebuilts": `[
        ":pretend_prebuilt_1",
        ":pretend_prebuilt_2",
    ]`,
				"updatable":    "False",
				"compressible": "False",
				"package_name": `"com.android.apogee.test.package"`,
			}),
		}})
}

func TestApexBundleSimple_fileContextsInAnotherAndroidBp(t *testing.T) {
	runApexTestCase(t, bp2buildTestCase{
		description:                "apex - file contexts is a module in another Android.bp",
		moduleTypeUnderTest:        "apex",
		moduleTypeUnderTestFactory: apex.BundleFactory,
		filesystem: map[string]string{
			"a/b/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [
		"com.android.apogee-file_contexts",
	],
	bazel_module: { bp2build_available: false },
}
`,
		},
		blueprint: `
apex {
	name: "com.android.apogee",
	file_contexts: ":com.android.apogee-file_contexts",
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("apex", "com.android.apogee", attrNameToString{
				"file_contexts": `"//a/b:com.android.apogee-file_contexts"`,
				"manifest":      `"apex_manifest.json"`,
			}),
		}})
}

func TestApexBundleSimple_fileContextsIsFile(t *testing.T) {
	runApexTestCase(t, bp2buildTestCase{
		description:                "apex - file contexts is a file",
		moduleTypeUnderTest:        "apex",
		moduleTypeUnderTestFactory: apex.BundleFactory,
		filesystem:                 map[string]string{},
		blueprint: `
apex {
	name: "com.android.apogee",
	file_contexts: "file_contexts_file",
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("apex", "com.android.apogee", attrNameToString{
				"file_contexts": `"file_contexts_file"`,
				"manifest":      `"apex_manifest.json"`,
			}),
		}})
}

func TestApexBundleSimple_fileContextsIsNotSpecified(t *testing.T) {
	runApexTestCase(t, bp2buildTestCase{
		description:                "apex - file contexts is not specified",
		moduleTypeUnderTest:        "apex",
		moduleTypeUnderTestFactory: apex.BundleFactory,
		filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [
		"com.android.apogee-file_contexts",
	],
	bazel_module: { bp2build_available: false },
}
`,
		},
		blueprint: `
apex {
	name: "com.android.apogee",
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("apex", "com.android.apogee", attrNameToString{
				"file_contexts": `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":      `"apex_manifest.json"`,
			}),
		}})
}

func TestApexBundleCompileMultilibBoth(t *testing.T) {
	runApexTestCase(t, bp2buildTestCase{
		description:                "apex - example with compile_multilib=both",
		moduleTypeUnderTest:        "apex",
		moduleTypeUnderTestFactory: apex.BundleFactory,
		filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
	bazel_module: { bp2build_available: false },
}
`,
		},
		blueprint: createMultilibBlueprint("both"),
		expectedBazelTargets: []string{
			makeBazelTarget("apex", "com.android.apogee", attrNameToString{
				"native_shared_libs_32": `[
        ":native_shared_lib_1",
        ":native_shared_lib_3",
    ] + select({
        "//build/bazel/platforms/arch:arm": [":native_shared_lib_2"],
        "//build/bazel/platforms/arch:x86": [":native_shared_lib_2"],
        "//conditions:default": [],
    })`,
				"native_shared_libs_64": `select({
        "//build/bazel/platforms/arch:arm64": [
            ":native_shared_lib_1",
            ":native_shared_lib_4",
            ":native_shared_lib_2",
        ],
        "//build/bazel/platforms/arch:x86_64": [
            ":native_shared_lib_1",
            ":native_shared_lib_4",
            ":native_shared_lib_2",
        ],
        "//conditions:default": [],
    })`,
				"file_contexts": `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":      `"apex_manifest.json"`,
			}),
		}})
}

func TestApexBundleCompileMultilibFirst(t *testing.T) {
	runApexTestCase(t, bp2buildTestCase{
		description:                "apex - example with compile_multilib=first",
		moduleTypeUnderTest:        "apex",
		moduleTypeUnderTestFactory: apex.BundleFactory,
		filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
	bazel_module: { bp2build_available: false },
}
`,
		},
		blueprint: createMultilibBlueprint("first"),
		expectedBazelTargets: []string{
			makeBazelTarget("apex", "com.android.apogee", attrNameToString{
				"native_shared_libs_32": `select({
        "//build/bazel/platforms/arch:arm": [
            ":native_shared_lib_1",
            ":native_shared_lib_3",
            ":native_shared_lib_2",
        ],
        "//build/bazel/platforms/arch:x86": [
            ":native_shared_lib_1",
            ":native_shared_lib_3",
            ":native_shared_lib_2",
        ],
        "//conditions:default": [],
    })`,
				"native_shared_libs_64": `select({
        "//build/bazel/platforms/arch:arm64": [
            ":native_shared_lib_1",
            ":native_shared_lib_4",
            ":native_shared_lib_2",
        ],
        "//build/bazel/platforms/arch:x86_64": [
            ":native_shared_lib_1",
            ":native_shared_lib_4",
            ":native_shared_lib_2",
        ],
        "//conditions:default": [],
    })`,
				"file_contexts": `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":      `"apex_manifest.json"`,
			}),
		}})
}

func TestApexBundleCompileMultilib32(t *testing.T) {
	runApexTestCase(t, bp2buildTestCase{
		description:                "apex - example with compile_multilib=32",
		moduleTypeUnderTest:        "apex",
		moduleTypeUnderTestFactory: apex.BundleFactory,
		filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
	bazel_module: { bp2build_available: false },
}
`,
		},
		blueprint: createMultilibBlueprint("32"),
		expectedBazelTargets: []string{
			makeBazelTarget("apex", "com.android.apogee", attrNameToString{
				"native_shared_libs_32": `[
        ":native_shared_lib_1",
        ":native_shared_lib_3",
    ] + select({
        "//build/bazel/platforms/arch:arm": [":native_shared_lib_2"],
        "//build/bazel/platforms/arch:x86": [":native_shared_lib_2"],
        "//conditions:default": [],
    })`,
				"file_contexts": `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":      `"apex_manifest.json"`,
			}),
		}})
}

func TestApexBundleCompileMultilib64(t *testing.T) {
	runApexTestCase(t, bp2buildTestCase{
		description:                "apex - example with compile_multilib=64",
		moduleTypeUnderTest:        "apex",
		moduleTypeUnderTestFactory: apex.BundleFactory,
		filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
	bazel_module: { bp2build_available: false },
}
`,
		},
		blueprint: createMultilibBlueprint("64"),
		expectedBazelTargets: []string{
			makeBazelTarget("apex", "com.android.apogee", attrNameToString{
				"native_shared_libs_64": `select({
        "//build/bazel/platforms/arch:arm64": [
            ":native_shared_lib_1",
            ":native_shared_lib_4",
            ":native_shared_lib_2",
        ],
        "//build/bazel/platforms/arch:x86_64": [
            ":native_shared_lib_1",
            ":native_shared_lib_4",
            ":native_shared_lib_2",
        ],
        "//conditions:default": [],
    })`,
				"file_contexts": `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":      `"apex_manifest.json"`,
			}),
		}})
}

func TestApexBundleDefaultPropertyValues(t *testing.T) {
	runApexTestCase(t, bp2buildTestCase{
		description:                "apex - default property values",
		moduleTypeUnderTest:        "apex",
		moduleTypeUnderTestFactory: apex.BundleFactory,
		filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
	bazel_module: { bp2build_available: false },
}
`,
		},
		blueprint: `
apex {
	name: "com.android.apogee",
	manifest: "apogee_manifest.json",
}
`,
		expectedBazelTargets: []string{makeBazelTarget("apex", "com.android.apogee", attrNameToString{
			"manifest":      `"apogee_manifest.json"`,
			"file_contexts": `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
		}),
		}})
}

func TestApexBundleHasBazelModuleProps(t *testing.T) {
	runApexTestCase(t, bp2buildTestCase{
		description:                "apex - has bazel module props",
		moduleTypeUnderTest:        "apex",
		moduleTypeUnderTestFactory: apex.BundleFactory,
		filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
	bazel_module: { bp2build_available: false },
}
`,
		},
		blueprint: `
apex {
	name: "apogee",
	manifest: "manifest.json",
	bazel_module: { bp2build_available: true },
}
`,
		expectedBazelTargets: []string{makeBazelTarget("apex", "apogee", attrNameToString{
			"manifest":      `"manifest.json"`,
			"file_contexts": `"//system/sepolicy/apex:apogee-file_contexts"`,
		}),
		}})
}

func createMultilibBlueprint(compile_multilib string) string {
	return `
cc_library {
	name: "native_shared_lib_1",
	bazel_module: { bp2build_available: false },
}

cc_library {
	name: "native_shared_lib_2",
	bazel_module: { bp2build_available: false },
}

cc_library {
	name: "native_shared_lib_3",
	bazel_module: { bp2build_available: false },
}

cc_library {
	name: "native_shared_lib_4",
	bazel_module: { bp2build_available: false },
}

apex {
	name: "com.android.apogee",
	compile_multilib: "` + compile_multilib + `",
	multilib: {
		both: {
			native_shared_libs: [
				"native_shared_lib_1",
			],
		},
		first: {
			native_shared_libs: [
				"native_shared_lib_2",
			],
		},
		lib32: {
			native_shared_libs: [
				"native_shared_lib_3",
			],
		},
		lib64: {
			native_shared_libs: [
				"native_shared_lib_4",
			],
		},
	},
}`
}

func TestBp2BuildOverrideApex(t *testing.T) {
	runOverrideApexTestCase(t, bp2buildTestCase{
		description:                "override_apex",
		moduleTypeUnderTest:        "override_apex",
		moduleTypeUnderTestFactory: apex.OverrideApexFactory,
		filesystem:                 map[string]string{},
		blueprint: `
apex_key {
	name: "com.android.apogee.key",
	public_key: "com.android.apogee.avbpubkey",
	private_key: "com.android.apogee.pem",
	bazel_module: { bp2build_available: false },
}

android_app_certificate {
	name: "com.android.apogee.certificate",
	certificate: "com.android.apogee",
	bazel_module: { bp2build_available: false },
}

cc_library {
	name: "native_shared_lib_1",
	bazel_module: { bp2build_available: false },
}

cc_library {
	name: "native_shared_lib_2",
	bazel_module: { bp2build_available: false },
}

cc_library {
	name: "pretend_prebuilt_1",
	bazel_module: { bp2build_available: false },
}

cc_library {
	name: "pretend_prebuilt_2",
	bazel_module: { bp2build_available: false },
}

filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [
		"com.android.apogee-file_contexts",
	],
	bazel_module: { bp2build_available: false },
}

cc_binary { name: "cc_binary_1", bazel_module: { bp2build_available: false } }
sh_binary { name: "sh_binary_2", bazel_module: { bp2build_available: false } }

apex {
	name: "com.android.apogee",
	manifest: "apogee_manifest.json",
	androidManifest: "ApogeeAndroidManifest.xml",
	file_contexts: ":com.android.apogee-file_contexts",
	min_sdk_version: "29",
	key: "com.android.apogee.key",
	certificate: "com.android.apogee.certificate",
	updatable: false,
	installable: false,
	compressible: false,
	native_shared_libs: [
	    "native_shared_lib_1",
	    "native_shared_lib_2",
	],
	binaries: [
		"cc_binary_1",
		"sh_binary_2",
	],
	prebuilts: [
	    "pretend_prebuilt_1",
	    "pretend_prebuilt_2",
	],
	bazel_module: { bp2build_available: false },
}

apex_key {
	name: "com.google.android.apogee.key",
	public_key: "com.google.android.apogee.avbpubkey",
	private_key: "com.google.android.apogee.pem",
	bazel_module: { bp2build_available: false },
}

android_app_certificate {
	name: "com.google.android.apogee.certificate",
	certificate: "com.google.android.apogee",
	bazel_module: { bp2build_available: false },
}

override_apex {
	name: "com.google.android.apogee",
	base: ":com.android.apogee",
	key: "com.google.android.apogee.key",
	certificate: "com.google.android.apogee.certificate",
	prebuilts: [],
	compressible: true,
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("apex", "com.google.android.apogee", attrNameToString{
				"android_manifest": `"ApogeeAndroidManifest.xml"`,
				"binaries": `[
        ":cc_binary_1",
        ":sh_binary_2",
    ]`,
				"certificate":     `":com.google.android.apogee.certificate"`,
				"file_contexts":   `":com.android.apogee-file_contexts"`,
				"installable":     "False",
				"key":             `":com.google.android.apogee.key"`,
				"manifest":        `"apogee_manifest.json"`,
				"min_sdk_version": `"29"`,
				"native_shared_libs_32": `[
        ":native_shared_lib_1",
        ":native_shared_lib_2",
    ]`,
				"native_shared_libs_64": `select({
        "//build/bazel/platforms/arch:arm64": [
            ":native_shared_lib_1",
            ":native_shared_lib_2",
        ],
        "//build/bazel/platforms/arch:x86_64": [
            ":native_shared_lib_1",
            ":native_shared_lib_2",
        ],
        "//conditions:default": [],
    })`,
				"prebuilts":    `[]`,
				"updatable":    "False",
				"compressible": "True",
			}),
		}})
}

func TestApexBundleSimple_manifestIsEmpty_baseApexOverrideApexInDifferentAndroidBp(t *testing.T) {
	runOverrideApexTestCase(t, bp2buildTestCase{
		description:                "override_apex - manifest of base apex is empty, base apex and override_apex is in different Android.bp",
		moduleTypeUnderTest:        "override_apex",
		moduleTypeUnderTestFactory: apex.OverrideApexFactory,
		filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
	bazel_module: { bp2build_available: false },
}`,
			"a/b/Android.bp": `
apex {
	name: "com.android.apogee",
	bazel_module: { bp2build_available: false },
}
`,
		},
		blueprint: `
override_apex {
	name: "com.google.android.apogee",
	base: ":com.android.apogee",
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("apex", "com.google.android.apogee", attrNameToString{
				"file_contexts": `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":      `"//a/b:apex_manifest.json"`,
			}),
		}})
}

func TestApexBundleSimple_manifestIsSet_baseApexOverrideApexInDifferentAndroidBp(t *testing.T) {
	runOverrideApexTestCase(t, bp2buildTestCase{
		description:                "override_apex - manifest of base apex is set, base apex and override_apex is in different Android.bp",
		moduleTypeUnderTest:        "override_apex",
		moduleTypeUnderTestFactory: apex.OverrideApexFactory,
		filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
	bazel_module: { bp2build_available: false },
}`,
			"a/b/Android.bp": `
apex {
	name: "com.android.apogee",
  manifest: "apogee_manifest.json",
	bazel_module: { bp2build_available: false },
}
`,
		},
		blueprint: `
override_apex {
	name: "com.google.android.apogee",
  base: ":com.android.apogee",
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("apex", "com.google.android.apogee", attrNameToString{
				"file_contexts": `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":      `"//a/b:apogee_manifest.json"`,
			}),
		}})
}

func TestApexBundleSimple_manifestIsEmpty_baseApexOverrideApexInSameAndroidBp(t *testing.T) {
	runOverrideApexTestCase(t, bp2buildTestCase{
		description:                "override_apex - manifest of base apex is empty, base apex and override_apex is in same Android.bp",
		moduleTypeUnderTest:        "override_apex",
		moduleTypeUnderTestFactory: apex.OverrideApexFactory,
		filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
	bazel_module: { bp2build_available: false },
}`,
		},
		blueprint: `
apex {
	name: "com.android.apogee",
	bazel_module: { bp2build_available: false },
}

override_apex {
	name: "com.google.android.apogee",
  base: ":com.android.apogee",
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("apex", "com.google.android.apogee", attrNameToString{
				"file_contexts": `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":      `"apex_manifest.json"`,
			}),
		}})
}

func TestApexBundleSimple_manifestIsSet_baseApexOverrideApexInSameAndroidBp(t *testing.T) {
	runOverrideApexTestCase(t, bp2buildTestCase{
		description:                "override_apex - manifest of base apex is set, base apex and override_apex is in same Android.bp",
		moduleTypeUnderTest:        "override_apex",
		moduleTypeUnderTestFactory: apex.OverrideApexFactory,
		filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
	bazel_module: { bp2build_available: false },
}`,
		},
		blueprint: `
apex {
	name: "com.android.apogee",
  manifest: "apogee_manifest.json",
	bazel_module: { bp2build_available: false },
}

override_apex {
	name: "com.google.android.apogee",
  base: ":com.android.apogee",
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("apex", "com.google.android.apogee", attrNameToString{
				"file_contexts": `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":      `"apogee_manifest.json"`,
			}),
		}})
}

func TestApexBundleSimple_packageNameOverride(t *testing.T) {
	runOverrideApexTestCase(t, bp2buildTestCase{
		description:                "override_apex - override package name",
		moduleTypeUnderTest:        "override_apex",
		moduleTypeUnderTestFactory: apex.OverrideApexFactory,
		filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
	bazel_module: { bp2build_available: false },
}`,
		},
		blueprint: `
apex {
	name: "com.android.apogee",
	bazel_module: { bp2build_available: false },
}

override_apex {
	name: "com.google.android.apogee",
	base: ":com.android.apogee",
	package_name: "com.google.android.apogee",
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("apex", "com.google.android.apogee", attrNameToString{
				"file_contexts": `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":      `"apex_manifest.json"`,
				"package_name":  `"com.google.android.apogee"`,
			}),
		}})
}
