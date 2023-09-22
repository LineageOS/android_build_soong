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
	"android/soong/etc"
	"android/soong/java"
	"android/soong/sh"

	"fmt"
	"testing"
)

func runApexTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, registerApexModuleTypes, tc)
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
	ctx.RegisterModuleType("prebuilt_etc", etc.PrebuiltEtcFactory)
	ctx.RegisterModuleType("cc_test", cc.TestFactory)
}

func runOverrideApexTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, registerOverrideApexModuleTypes, tc)
}

func registerOverrideApexModuleTypes(ctx android.RegistrationContext) {
	// CC module types needed as they can be APEX dependencies
	cc.RegisterCCBuildComponents(ctx)

	ctx.RegisterModuleType("sh_binary", sh.ShBinaryFactory)
	ctx.RegisterModuleType("cc_binary", cc.BinaryFactory)
	ctx.RegisterModuleType("cc_library", cc.LibraryFactory)
	ctx.RegisterModuleType("apex_key", apex.ApexKeyFactory)
	ctx.RegisterModuleType("apex_test", apex.TestApexBundleFactory)
	ctx.RegisterModuleType("android_app_certificate", java.AndroidAppCertificateFactory)
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	ctx.RegisterModuleType("apex", apex.BundleFactory)
	ctx.RegisterModuleType("apex_defaults", apex.DefaultsFactory)
	ctx.RegisterModuleType("prebuilt_etc", etc.PrebuiltEtcFactory)
	ctx.RegisterModuleType("soong_config_module_type", android.SoongConfigModuleTypeFactory)
	ctx.RegisterModuleType("soong_config_string_variable", android.SoongConfigStringVariableDummyFactory)
}

func TestApexBundleSimple(t *testing.T) {
	runApexTestCase(t, Bp2buildTestCase{
		Description:                "apex - example with all props, file_context is a module in same Android.bp",
		ModuleTypeUnderTest:        "apex",
		ModuleTypeUnderTestFactory: apex.BundleFactory,
		Filesystem:                 map[string]string{},
		StubbedBuildDefinitions: []string{"com.android.apogee.key", "com.android.apogee.certificate", "native_shared_lib_1", "native_shared_lib_2",
			"prebuilt_1", "prebuilt_2", "com.android.apogee-file_contexts", "cc_binary_1", "sh_binary_2"},
		Blueprint: `
apex_key {
	name: "com.android.apogee.key",
	public_key: "com.android.apogee.avbpubkey",
	private_key: "com.android.apogee.pem",
}

android_app_certificate {
	name: "com.android.apogee.certificate",
	certificate: "com.android.apogee",
}

cc_library {
	name: "native_shared_lib_1",
}

cc_library {
	name: "native_shared_lib_2",
}

prebuilt_etc {
	name: "prebuilt_1",
}

prebuilt_etc {
	name: "prebuilt_2",
}

filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [
		"com.android.apogee-file_contexts",
	],
}

cc_binary { name: "cc_binary_1"}
sh_binary { name: "sh_binary_2", src: "foo.sh"}

apex {
	name: "com.android.apogee",
	manifest: "apogee_manifest.json",
	androidManifest: "ApogeeAndroidManifest.xml",
	apex_available_name: "apogee_apex_name",
	file_contexts: ":com.android.apogee-file_contexts",
	min_sdk_version: "29",
	key: "com.android.apogee.key",
	certificate: ":com.android.apogee.certificate",
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
	    "prebuilt_1",
	    "prebuilt_2",
	],
	package_name: "com.android.apogee.test.package",
	logging_parent: "logging.parent",
	variant_version: "3",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.android.apogee", AttrNameToString{
				"android_manifest":    `"ApogeeAndroidManifest.xml"`,
				"apex_available_name": `"apogee_apex_name"`,
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
				"native_shared_libs_32": `select({
        "//build/bazel/platforms/arch:arm": [
            ":native_shared_lib_1",
            ":native_shared_lib_2",
        ],
        "//build/bazel/platforms/arch:x86": [
            ":native_shared_lib_1",
            ":native_shared_lib_2",
        ],
        "//conditions:default": [],
    })`,
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
        ":prebuilt_1",
        ":prebuilt_2",
    ]`,
				"updatable":       "False",
				"compressible":    "False",
				"package_name":    `"com.android.apogee.test.package"`,
				"logging_parent":  `"logging.parent"`,
				"variant_version": `"3"`,
			}),
		}})
}

func TestApexBundleSimple_fileContextsInAnotherAndroidBp(t *testing.T) {
	runApexTestCase(t, Bp2buildTestCase{
		Description:                "apex - file contexts is a module in another Android.bp",
		ModuleTypeUnderTest:        "apex",
		ModuleTypeUnderTestFactory: apex.BundleFactory,
		StubbedBuildDefinitions:    []string{"//a/b:com.android.apogee-file_contexts"},
		Filesystem: map[string]string{
			"a/b/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [
		"com.android.apogee-file_contexts",
	],
}
`,
		},
		Blueprint: `
apex {
	name: "com.android.apogee",
	file_contexts: ":com.android.apogee-file_contexts",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.android.apogee", AttrNameToString{
				"file_contexts": `"//a/b:com.android.apogee-file_contexts"`,
				"manifest":      `"apex_manifest.json"`,
			}),
		}})
}

func TestApexBundleSimple_fileContextsIsFile(t *testing.T) {
	runApexTestCase(t, Bp2buildTestCase{
		Description:                "apex - file contexts is a file",
		ModuleTypeUnderTest:        "apex",
		ModuleTypeUnderTestFactory: apex.BundleFactory,
		Filesystem:                 map[string]string{},
		Blueprint: `
apex {
	name: "com.android.apogee",
	file_contexts: "file_contexts_file",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.android.apogee", AttrNameToString{
				"file_contexts": `"file_contexts_file"`,
				"manifest":      `"apex_manifest.json"`,
			}),
		}})
}

func TestApexBundleSimple_fileContextsIsNotSpecified(t *testing.T) {
	runApexTestCase(t, Bp2buildTestCase{
		Description:                "apex - file contexts is not specified",
		ModuleTypeUnderTest:        "apex",
		ModuleTypeUnderTestFactory: apex.BundleFactory,
		StubbedBuildDefinitions:    []string{"//system/sepolicy/apex:com.android.apogee-file_contexts"},
		Filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [
		"com.android.apogee-file_contexts",
	],
}
`,
		},
		Blueprint: `
apex {
	name: "com.android.apogee",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.android.apogee", AttrNameToString{
				"file_contexts": `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":      `"apex_manifest.json"`,
			}),
		}})
}

func TestApexBundleCompileMultilibBoth(t *testing.T) {
	runApexTestCase(t, Bp2buildTestCase{
		Description:                "apex - example with compile_multilib=both",
		ModuleTypeUnderTest:        "apex",
		ModuleTypeUnderTestFactory: apex.BundleFactory,
		StubbedBuildDefinitions:    append(multilibStubNames(), "//system/sepolicy/apex:com.android.apogee-file_contexts"),
		Filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
}
`,
		},
		Blueprint: createMultilibBlueprint(`compile_multilib: "both",`),
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.android.apogee", AttrNameToString{
				"native_shared_libs_32": `[
        ":unnested_native_shared_lib",
        ":native_shared_lib_for_both",
        ":native_shared_lib_for_lib32",
    ] + select({
        "//build/bazel/platforms/arch:arm": [":native_shared_lib_for_first"],
        "//build/bazel/platforms/arch:x86": [":native_shared_lib_for_first"],
        "//conditions:default": [],
    })`,
				"native_shared_libs_64": `select({
        "//build/bazel/platforms/arch:arm64": [
            ":unnested_native_shared_lib",
            ":native_shared_lib_for_both",
            ":native_shared_lib_for_lib64",
            ":native_shared_lib_for_first",
        ],
        "//build/bazel/platforms/arch:x86_64": [
            ":unnested_native_shared_lib",
            ":native_shared_lib_for_both",
            ":native_shared_lib_for_lib64",
            ":native_shared_lib_for_first",
        ],
        "//conditions:default": [],
    })`,
				"file_contexts": `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":      `"apex_manifest.json"`,
			}),
		}})
}

func TestApexBundleCompileMultilibFirstAndDefaultValue(t *testing.T) {
	expectedBazelTargets := []string{
		MakeBazelTarget("apex", "com.android.apogee", AttrNameToString{
			"native_shared_libs_32": `select({
        "//build/bazel/platforms/arch:arm": [
            ":unnested_native_shared_lib",
            ":native_shared_lib_for_both",
            ":native_shared_lib_for_lib32",
            ":native_shared_lib_for_first",
        ],
        "//build/bazel/platforms/arch:x86": [
            ":unnested_native_shared_lib",
            ":native_shared_lib_for_both",
            ":native_shared_lib_for_lib32",
            ":native_shared_lib_for_first",
        ],
        "//conditions:default": [],
    })`,
			"native_shared_libs_64": `select({
        "//build/bazel/platforms/arch:arm64": [
            ":unnested_native_shared_lib",
            ":native_shared_lib_for_both",
            ":native_shared_lib_for_lib64",
            ":native_shared_lib_for_first",
        ],
        "//build/bazel/platforms/arch:x86_64": [
            ":unnested_native_shared_lib",
            ":native_shared_lib_for_both",
            ":native_shared_lib_for_lib64",
            ":native_shared_lib_for_first",
        ],
        "//conditions:default": [],
    })`,
			"file_contexts": `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
			"manifest":      `"apex_manifest.json"`,
		}),
	}

	// "first" is the default value of compile_multilib prop so `compile_multilib_: "first"` and unset compile_multilib
	// should result to the same bp2build output
	compileMultiLibPropValues := []string{`compile_multilib: "first",`, ""}
	for _, compileMultiLibProp := range compileMultiLibPropValues {
		descriptionSuffix := compileMultiLibProp
		if descriptionSuffix == "" {
			descriptionSuffix = "compile_multilib unset"
		}
		runApexTestCase(t, Bp2buildTestCase{
			Description:                "apex - example with " + compileMultiLibProp,
			ModuleTypeUnderTest:        "apex",
			ModuleTypeUnderTestFactory: apex.BundleFactory,
			StubbedBuildDefinitions:    append(multilibStubNames(), "//system/sepolicy/apex:com.android.apogee-file_contexts"),
			Filesystem: map[string]string{
				"system/sepolicy/apex/Android.bp": `
    filegroup {
        name: "com.android.apogee-file_contexts",
        srcs: [ "apogee-file_contexts", ],
    }
    `,
			},
			Blueprint:            createMultilibBlueprint(compileMultiLibProp),
			ExpectedBazelTargets: expectedBazelTargets,
		})
	}
}

func TestApexBundleCompileMultilib32(t *testing.T) {
	runApexTestCase(t, Bp2buildTestCase{
		Description:                "apex - example with compile_multilib=32",
		ModuleTypeUnderTest:        "apex",
		ModuleTypeUnderTestFactory: apex.BundleFactory,
		StubbedBuildDefinitions:    append(multilibStubNames(), "//system/sepolicy/apex:com.android.apogee-file_contexts"),
		Filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
}
`,
		},
		Blueprint: createMultilibBlueprint(`compile_multilib: "32",`),
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.android.apogee", AttrNameToString{
				"native_shared_libs_32": `[
        ":unnested_native_shared_lib",
        ":native_shared_lib_for_both",
        ":native_shared_lib_for_lib32",
    ] + select({
        "//build/bazel/platforms/arch:arm": [":native_shared_lib_for_first"],
        "//build/bazel/platforms/arch:x86": [":native_shared_lib_for_first"],
        "//conditions:default": [],
    })`,
				"file_contexts": `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":      `"apex_manifest.json"`,
			}),
		}})
}

func TestApexBundleCompileMultilib64(t *testing.T) {
	runApexTestCase(t, Bp2buildTestCase{
		Description:                "apex - example with compile_multilib=64",
		ModuleTypeUnderTest:        "apex",
		ModuleTypeUnderTestFactory: apex.BundleFactory,
		StubbedBuildDefinitions:    append(multilibStubNames(), "//system/sepolicy/apex:com.android.apogee-file_contexts"),
		Filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
}
`,
		},
		Blueprint: createMultilibBlueprint(`compile_multilib: "64",`),
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.android.apogee", AttrNameToString{
				"native_shared_libs_64": `select({
        "//build/bazel/platforms/arch:arm64": [
            ":unnested_native_shared_lib",
            ":native_shared_lib_for_both",
            ":native_shared_lib_for_lib64",
            ":native_shared_lib_for_first",
        ],
        "//build/bazel/platforms/arch:x86_64": [
            ":unnested_native_shared_lib",
            ":native_shared_lib_for_both",
            ":native_shared_lib_for_lib64",
            ":native_shared_lib_for_first",
        ],
        "//conditions:default": [],
    })`,
				"file_contexts": `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":      `"apex_manifest.json"`,
			}),
		}})
}

func multilibStubNames() []string {
	return []string{"native_shared_lib_for_both", "native_shared_lib_for_first", "native_shared_lib_for_lib32", "native_shared_lib_for_lib64",
		"native_shared_lib_for_lib64", "unnested_native_shared_lib"}
}

func createMultilibBlueprint(compile_multilib string) string {
	return fmt.Sprintf(`
cc_library {
	name: "native_shared_lib_for_both",
}

cc_library {
	name: "native_shared_lib_for_first",
}

cc_library {
	name: "native_shared_lib_for_lib32",
}

cc_library {
	name: "native_shared_lib_for_lib64",
}

cc_library {
	name: "unnested_native_shared_lib",
}

apex {
	name: "com.android.apogee",
	%s
	native_shared_libs: ["unnested_native_shared_lib"],
	multilib: {
		both: {
			native_shared_libs: [
				"native_shared_lib_for_both",
			],
		},
		first: {
			native_shared_libs: [
				"native_shared_lib_for_first",
			],
		},
		lib32: {
			native_shared_libs: [
				"native_shared_lib_for_lib32",
			],
		},
		lib64: {
			native_shared_libs: [
				"native_shared_lib_for_lib64",
			],
		},
	},
}`, compile_multilib)
}

func TestApexBundleDefaultPropertyValues(t *testing.T) {
	runApexTestCase(t, Bp2buildTestCase{
		Description:                "apex - default property values",
		ModuleTypeUnderTest:        "apex",
		ModuleTypeUnderTestFactory: apex.BundleFactory,
		StubbedBuildDefinitions:    []string{"//system/sepolicy/apex:com.android.apogee-file_contexts"},
		Filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
}
`,
		},
		Blueprint: `
apex {
	name: "com.android.apogee",
	manifest: "apogee_manifest.json",
}
`,
		ExpectedBazelTargets: []string{MakeBazelTarget("apex", "com.android.apogee", AttrNameToString{
			"manifest":      `"apogee_manifest.json"`,
			"file_contexts": `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
		}),
		}})
}

func TestApexBundleHasBazelModuleProps(t *testing.T) {
	runApexTestCase(t, Bp2buildTestCase{
		Description:                "apex - has bazel module props",
		ModuleTypeUnderTest:        "apex",
		ModuleTypeUnderTestFactory: apex.BundleFactory,
		StubbedBuildDefinitions:    []string{"//system/sepolicy/apex:apogee-file_contexts"},
		Filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
}
`,
		},
		Blueprint: `
apex {
	name: "apogee",
	manifest: "manifest.json",
	bazel_module: { bp2build_available: true },
}
`,
		ExpectedBazelTargets: []string{MakeBazelTarget("apex", "apogee", AttrNameToString{
			"manifest":      `"manifest.json"`,
			"file_contexts": `"//system/sepolicy/apex:apogee-file_contexts"`,
		}),
		}})
}

func TestBp2BuildOverrideApex(t *testing.T) {
	runOverrideApexTestCase(t, Bp2buildTestCase{
		Description:                "override_apex",
		ModuleTypeUnderTest:        "override_apex",
		ModuleTypeUnderTestFactory: apex.OverrideApexFactory,
		Filesystem:                 map[string]string{},
		StubbedBuildDefinitions: []string{"com.android.apogee.key", "com.android.apogee.certificate", "native_shared_lib_1",
			"native_shared_lib_2", "prebuilt_1", "prebuilt_2", "com.android.apogee-file_contexts", "cc_binary_1",
			"sh_binary_2", "com.android.apogee", "com.google.android.apogee.key", "com.google.android.apogee.certificate"},
		Blueprint: `
apex_key {
	name: "com.android.apogee.key",
	public_key: "com.android.apogee.avbpubkey",
	private_key: "com.android.apogee.pem",
}

android_app_certificate {
	name: "com.android.apogee.certificate",
	certificate: "com.android.apogee",
}

cc_library {
	name: "native_shared_lib_1",
}

cc_library {
	name: "native_shared_lib_2",
}

prebuilt_etc {
	name: "prebuilt_1",
}

prebuilt_etc {
	name: "prebuilt_2",
}

filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [
		"com.android.apogee-file_contexts",
	],
}

cc_binary { name: "cc_binary_1" }
sh_binary { name: "sh_binary_2", src: "foo.sh"}

apex {
	name: "com.android.apogee",
	manifest: "apogee_manifest.json",
	androidManifest: "ApogeeAndroidManifest.xml",
	file_contexts: ":com.android.apogee-file_contexts",
	min_sdk_version: "29",
	key: "com.android.apogee.key",
	certificate: ":com.android.apogee.certificate",
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
	    "prebuilt_1",
	    "prebuilt_2",
	],
}

apex_key {
	name: "com.google.android.apogee.key",
	public_key: "com.google.android.apogee.avbpubkey",
	private_key: "com.google.android.apogee.pem",
}

android_app_certificate {
	name: "com.google.android.apogee.certificate",
	certificate: "com.google.android.apogee",
}

override_apex {
	name: "com.google.android.apogee",
	base: ":com.android.apogee",
	key: "com.google.android.apogee.key",
	certificate: ":com.google.android.apogee.certificate",
	prebuilts: [],
	compressible: true,
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.google.android.apogee", AttrNameToString{
				"android_manifest": `"ApogeeAndroidManifest.xml"`,
				"base_apex_name":   `"com.android.apogee"`,
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
				"native_shared_libs_32": `select({
        "//build/bazel/platforms/arch:arm": [
            ":native_shared_lib_1",
            ":native_shared_lib_2",
        ],
        "//build/bazel/platforms/arch:x86": [
            ":native_shared_lib_1",
            ":native_shared_lib_2",
        ],
        "//conditions:default": [],
    })`,
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

func TestOverrideApexTest(t *testing.T) {
	runOverrideApexTestCase(t, Bp2buildTestCase{
		Description:                "override_apex",
		ModuleTypeUnderTest:        "override_apex",
		ModuleTypeUnderTestFactory: apex.OverrideApexFactory,
		Filesystem:                 map[string]string{},
		StubbedBuildDefinitions: []string{"com.android.apogee.certificate", "native_shared_lib_1",
			"prebuilt_1", "com.android.apogee-file_contexts", "cc_binary_1", "sh_binary_2",
			"com.android.apogee", "com.google.android.apogee.key", "com.google.android.apogee.certificate", "com.android.apogee.key"},
		Blueprint: `
apex_key {
	name: "com.android.apogee.key",
	public_key: "com.android.apogee.avbpubkey",
	private_key: "com.android.apogee.pem",
}

android_app_certificate {
	name: "com.android.apogee.certificate",
	certificate: "com.android.apogee",
}

cc_library {
	name: "native_shared_lib_1",
}

prebuilt_etc {
	name: "prebuilt_1",
}

filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [
		"com.android.apogee-file_contexts",
	],
}

cc_binary { name: "cc_binary_1"}
sh_binary { name: "sh_binary_2", src: "foo.sh"}

apex_test {
	name: "com.android.apogee",
	manifest: "apogee_manifest.json",
	androidManifest: "ApogeeAndroidManifest.xml",
	file_contexts: ":com.android.apogee-file_contexts",
	min_sdk_version: "29",
	key: "com.android.apogee.key",
	certificate: ":com.android.apogee.certificate",
	updatable: false,
	installable: false,
	compressible: false,
	native_shared_libs: [
	    "native_shared_lib_1",
	],
	binaries: [
		"cc_binary_1",
		"sh_binary_2",
	],
	prebuilts: [
	    "prebuilt_1",
	],
}

apex_key {
	name: "com.google.android.apogee.key",
	public_key: "com.google.android.apogee.avbpubkey",
	private_key: "com.google.android.apogee.pem",
}

android_app_certificate {
	name: "com.google.android.apogee.certificate",
	certificate: "com.google.android.apogee",
}

override_apex {
	name: "com.google.android.apogee",
	base: ":com.android.apogee",
	key: "com.google.android.apogee.key",
	certificate: ":com.google.android.apogee.certificate",
	prebuilts: [],
	compressible: true,
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.google.android.apogee", AttrNameToString{
				"android_manifest": `"ApogeeAndroidManifest.xml"`,
				"base_apex_name":   `"com.android.apogee"`,
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
				"native_shared_libs_32": `select({
        "//build/bazel/platforms/arch:arm": [":native_shared_lib_1"],
        "//build/bazel/platforms/arch:x86": [":native_shared_lib_1"],
        "//conditions:default": [],
    })`,
				"native_shared_libs_64": `select({
        "//build/bazel/platforms/arch:arm64": [":native_shared_lib_1"],
        "//build/bazel/platforms/arch:x86_64": [":native_shared_lib_1"],
        "//conditions:default": [],
    })`,
				"testonly":     "True",
				"prebuilts":    `[]`,
				"updatable":    "False",
				"compressible": "True",
			}),
		}})
}

func TestApexBundleSimple_manifestIsEmpty_baseApexOverrideApexInDifferentAndroidBp(t *testing.T) {
	runOverrideApexTestCase(t, Bp2buildTestCase{
		Description:                "override_apex - manifest of base apex is empty, base apex and override_apex is in different Android.bp",
		ModuleTypeUnderTest:        "override_apex",
		ModuleTypeUnderTestFactory: apex.OverrideApexFactory,
		StubbedBuildDefinitions:    []string{"//a/b:com.android.apogee", "//system/sepolicy/apex:com.android.apogee-file_contexts"},
		Filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
}`,
			"a/b/Android.bp": `
apex {
	name: "com.android.apogee",
}
`,
		},
		Blueprint: `
override_apex {
	name: "com.google.android.apogee",
	base: ":com.android.apogee",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.google.android.apogee", AttrNameToString{
				"base_apex_name": `"com.android.apogee"`,
				"file_contexts":  `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":       `"//a/b:apex_manifest.json"`,
			}),
		}})
}

func TestApexBundleSimple_manifestIsSet_baseApexOverrideApexInDifferentAndroidBp(t *testing.T) {
	runOverrideApexTestCase(t, Bp2buildTestCase{
		Description:                "override_apex - manifest of base apex is set, base apex and override_apex is in different Android.bp",
		ModuleTypeUnderTest:        "override_apex",
		ModuleTypeUnderTestFactory: apex.OverrideApexFactory,
		StubbedBuildDefinitions:    []string{"//a/b:com.android.apogee", "//system/sepolicy/apex:com.android.apogee-file_contexts"},
		Filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
}`,
			"a/b/Android.bp": `
apex {
	name: "com.android.apogee",
  manifest: "apogee_manifest.json",
}
`,
		},
		Blueprint: `
override_apex {
	name: "com.google.android.apogee",
  base: ":com.android.apogee",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.google.android.apogee", AttrNameToString{
				"base_apex_name": `"com.android.apogee"`,
				"file_contexts":  `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":       `"//a/b:apogee_manifest.json"`,
			}),
		}})
}

func TestApexBundleSimple_manifestIsEmpty_baseApexOverrideApexInSameAndroidBp(t *testing.T) {
	runOverrideApexTestCase(t, Bp2buildTestCase{
		Description:                "override_apex - manifest of base apex is empty, base apex and override_apex is in same Android.bp",
		ModuleTypeUnderTest:        "override_apex",
		ModuleTypeUnderTestFactory: apex.OverrideApexFactory,
		StubbedBuildDefinitions:    []string{"com.android.apogee", "//system/sepolicy/apex:com.android.apogee-file_contexts"},
		Filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
}`,
		},
		Blueprint: `
apex {
	name: "com.android.apogee",
}

override_apex {
	name: "com.google.android.apogee",
  base: ":com.android.apogee",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.google.android.apogee", AttrNameToString{
				"base_apex_name": `"com.android.apogee"`,
				"file_contexts":  `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":       `"apex_manifest.json"`,
			}),
		}})
}

func TestApexBundleSimple_manifestIsSet_baseApexOverrideApexInSameAndroidBp(t *testing.T) {
	runOverrideApexTestCase(t, Bp2buildTestCase{
		Description:                "override_apex - manifest of base apex is set, base apex and override_apex is in same Android.bp",
		ModuleTypeUnderTest:        "override_apex",
		ModuleTypeUnderTestFactory: apex.OverrideApexFactory,
		StubbedBuildDefinitions:    []string{"com.android.apogee", "//system/sepolicy/apex:com.android.apogee-file_contexts"},
		Filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
}`,
		},
		Blueprint: `
apex {
	name: "com.android.apogee",
  manifest: "apogee_manifest.json",
}

override_apex {
	name: "com.google.android.apogee",
  base: ":com.android.apogee",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.google.android.apogee", AttrNameToString{
				"base_apex_name": `"com.android.apogee"`,
				"file_contexts":  `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":       `"apogee_manifest.json"`,
			}),
		}})
}

func TestApexBundleSimple_packageNameOverride(t *testing.T) {
	runOverrideApexTestCase(t, Bp2buildTestCase{
		Description:                "override_apex - override package name",
		ModuleTypeUnderTest:        "override_apex",
		ModuleTypeUnderTestFactory: apex.OverrideApexFactory,
		StubbedBuildDefinitions:    []string{"com.android.apogee", "//system/sepolicy/apex:com.android.apogee-file_contexts"},
		Filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
}`,
		},
		Blueprint: `
apex {
	name: "com.android.apogee",
}

override_apex {
	name: "com.google.android.apogee",
	base: ":com.android.apogee",
	package_name: "com.google.android.apogee",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.google.android.apogee", AttrNameToString{
				"base_apex_name": `"com.android.apogee"`,
				"file_contexts":  `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":       `"apex_manifest.json"`,
				"package_name":   `"com.google.android.apogee"`,
			}),
		}})
}

func TestApexBundleSimple_NoPrebuiltsOverride(t *testing.T) {
	runOverrideApexTestCase(t, Bp2buildTestCase{
		Description:                "override_apex - no override",
		ModuleTypeUnderTest:        "override_apex",
		ModuleTypeUnderTestFactory: apex.OverrideApexFactory,
		StubbedBuildDefinitions:    []string{"prebuilt_file", "com.android.apogee", "//system/sepolicy/apex:com.android.apogee-file_contexts"},
		Filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
}`,
		},
		Blueprint: `
prebuilt_etc {
	name: "prebuilt_file",
}

apex {
	name: "com.android.apogee",
	prebuilts: ["prebuilt_file"]
}

override_apex {
	name: "com.google.android.apogee",
	base: ":com.android.apogee",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.google.android.apogee", AttrNameToString{
				"base_apex_name": `"com.android.apogee"`,
				"file_contexts":  `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":       `"apex_manifest.json"`,
				"prebuilts":      `[":prebuilt_file"]`,
			}),
		}})
}

func TestApexBundleSimple_PrebuiltsOverride(t *testing.T) {
	runOverrideApexTestCase(t, Bp2buildTestCase{
		Description:                "override_apex - ooverride",
		ModuleTypeUnderTest:        "override_apex",
		ModuleTypeUnderTestFactory: apex.OverrideApexFactory,
		StubbedBuildDefinitions:    []string{"prebuilt_file", "prebuilt_file2", "com.android.apogee", "//system/sepolicy/apex:com.android.apogee-file_contexts"},
		Filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
}`,
		},
		Blueprint: `
prebuilt_etc {
	name: "prebuilt_file",
}

prebuilt_etc {
	name: "prebuilt_file2",
}

apex {
	name: "com.android.apogee",
	prebuilts: ["prebuilt_file"]
}

override_apex {
	name: "com.google.android.apogee",
	base: ":com.android.apogee",
	prebuilts: ["prebuilt_file2"]
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.google.android.apogee", AttrNameToString{
				"base_apex_name": `"com.android.apogee"`,
				"file_contexts":  `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":       `"apex_manifest.json"`,
				"prebuilts":      `[":prebuilt_file2"]`,
			}),
		}})
}

func TestApexBundleSimple_PrebuiltsOverrideEmptyList(t *testing.T) {
	runOverrideApexTestCase(t, Bp2buildTestCase{
		Description:                "override_apex - override with empty list",
		ModuleTypeUnderTest:        "override_apex",
		ModuleTypeUnderTestFactory: apex.OverrideApexFactory,
		StubbedBuildDefinitions:    []string{"prebuilt_file", "com.android.apogee", "//system/sepolicy/apex:com.android.apogee-file_contexts"},
		Filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
}`,
		},
		Blueprint: `
prebuilt_etc {
	name: "prebuilt_file",
}

apex {
	name: "com.android.apogee",
	prebuilts: ["prebuilt_file"]
}

override_apex {
	name: "com.google.android.apogee",
	base: ":com.android.apogee",
    prebuilts: [],
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.google.android.apogee", AttrNameToString{
				"base_apex_name": `"com.android.apogee"`,
				"file_contexts":  `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":       `"apex_manifest.json"`,
				"prebuilts":      `[]`,
			}),
		}})
}

func TestApexBundleSimple_NoLoggingParentOverride(t *testing.T) {
	runOverrideApexTestCase(t, Bp2buildTestCase{
		Description:                "override_apex - logging_parent - no override",
		ModuleTypeUnderTest:        "override_apex",
		ModuleTypeUnderTestFactory: apex.OverrideApexFactory,
		StubbedBuildDefinitions:    []string{"com.android.apogee", "//system/sepolicy/apex:com.android.apogee-file_contexts"},
		Filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
}`,
		},
		Blueprint: `
apex {
	name: "com.android.apogee",
	logging_parent: "foo.bar.baz",
}

override_apex {
	name: "com.google.android.apogee",
	base: ":com.android.apogee",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.google.android.apogee", AttrNameToString{
				"base_apex_name": `"com.android.apogee"`,
				"file_contexts":  `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":       `"apex_manifest.json"`,
				"logging_parent": `"foo.bar.baz"`,
			}),
		}})
}

func TestApexBundleSimple_LoggingParentOverride(t *testing.T) {
	runOverrideApexTestCase(t, Bp2buildTestCase{
		Description:                "override_apex - logging_parent - override",
		ModuleTypeUnderTest:        "override_apex",
		ModuleTypeUnderTestFactory: apex.OverrideApexFactory,
		StubbedBuildDefinitions:    []string{"com.android.apogee", "//system/sepolicy/apex:com.android.apogee-file_contexts"},
		Filesystem: map[string]string{
			"system/sepolicy/apex/Android.bp": `
filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [ "apogee-file_contexts", ],
}`,
		},
		Blueprint: `
apex {
	name: "com.android.apogee",
	logging_parent: "foo.bar.baz",
}

override_apex {
	name: "com.google.android.apogee",
	base: ":com.android.apogee",
	logging_parent: "foo.bar.baz.override",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.google.android.apogee", AttrNameToString{
				"base_apex_name": `"com.android.apogee"`,
				"file_contexts":  `"//system/sepolicy/apex:com.android.apogee-file_contexts"`,
				"manifest":       `"apex_manifest.json"`,
				"logging_parent": `"foo.bar.baz.override"`,
			}),
		}})
}

func TestBp2BuildOverrideApex_CertificateNil(t *testing.T) {
	runOverrideApexTestCase(t, Bp2buildTestCase{
		Description:                "override_apex - don't set default certificate",
		ModuleTypeUnderTest:        "override_apex",
		ModuleTypeUnderTestFactory: apex.OverrideApexFactory,
		Filesystem:                 map[string]string{},
		StubbedBuildDefinitions:    []string{"com.android.apogee.certificate", "com.android.apogee-file_contexts", "com.android.apogee"},
		Blueprint: `
android_app_certificate {
	name: "com.android.apogee.certificate",
	certificate: "com.android.apogee",
}

filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [
		"com.android.apogee-file_contexts",
	],
}

apex {
	name: "com.android.apogee",
	manifest: "apogee_manifest.json",
	file_contexts: ":com.android.apogee-file_contexts",
	certificate: ":com.android.apogee.certificate",
}

override_apex {
	name: "com.google.android.apogee",
	base: ":com.android.apogee",
	// certificate is deliberately omitted, and not converted to bazel,
	// because the overridden apex shouldn't be using the base apex's cert.
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.google.android.apogee", AttrNameToString{
				"base_apex_name": `"com.android.apogee"`,
				"file_contexts":  `":com.android.apogee-file_contexts"`,
				"manifest":       `"apogee_manifest.json"`,
			}),
		}})
}

func TestApexCertificateIsModule(t *testing.T) {
	runApexTestCase(t, Bp2buildTestCase{
		Description:                "apex - certificate is module",
		ModuleTypeUnderTest:        "apex",
		ModuleTypeUnderTestFactory: apex.BundleFactory,
		Filesystem:                 map[string]string{},
		StubbedBuildDefinitions:    []string{"com.android.apogee-file_contexts", "com.android.apogee.certificate"},
		Blueprint: `
android_app_certificate {
	name: "com.android.apogee.certificate",
	certificate: "com.android.apogee",
}

apex {
	name: "com.android.apogee",
	manifest: "apogee_manifest.json",
	file_contexts: ":com.android.apogee-file_contexts",
	certificate: ":com.android.apogee.certificate",
}
` + simpleModule("filegroup", "com.android.apogee-file_contexts"),
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.android.apogee", AttrNameToString{
				"certificate":   `":com.android.apogee.certificate"`,
				"file_contexts": `":com.android.apogee-file_contexts"`,
				"manifest":      `"apogee_manifest.json"`,
			}),
		}})
}

func TestApexWithStubLib(t *testing.T) {
	runApexTestCase(t, Bp2buildTestCase{
		Description:                "apex - static variant of stub lib should not have apex_available tag",
		ModuleTypeUnderTest:        "apex",
		ModuleTypeUnderTestFactory: apex.BundleFactory,
		Filesystem:                 map[string]string{},
		StubbedBuildDefinitions:    []string{"myapex-file_contexts"},
		Blueprint: `
cc_library{
	name: "foo",
	stubs: { symbol_file: "foo.map.txt", versions: ["28", "29", "current"] },
	apex_available: ["myapex"],
}

cc_binary{
	name: "bar",
	static_libs: ["foo"],
	apex_available: ["myapex"],
}

apex {
	name: "myapex",
	manifest: "myapex_manifest.json",
	file_contexts: ":myapex-file_contexts",
	binaries: ["bar"],
	native_shared_libs: ["foo"],
}
` + simpleModule("filegroup", "myapex-file_contexts"),
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_binary", "bar", AttrNameToString{
				"local_includes": `["."]`,
				"deps":           `[":foo_bp2build_cc_library_static"]`,
				"tags":           `["apex_available=myapex"]`,
			}),
			MakeBazelTarget("cc_library_static", "foo_bp2build_cc_library_static", AttrNameToString{
				"local_includes": `["."]`,
			}),
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"local_includes":    `["."]`,
				"stubs_symbol_file": `"foo.map.txt"`,
				"tags":              `["apex_available=myapex"]`,
			}),
			MakeBazelTarget("cc_stub_suite", "foo_stub_libs", AttrNameToString{
				"api_surface":          `"module-libapi"`,
				"soname":               `"foo.so"`,
				"source_library_label": `"//:foo"`,
				"symbol_file":          `"foo.map.txt"`,
				"versions": `[
        "28",
        "29",
        "current",
    ]`,
			}),
			MakeBazelTarget("apex", "myapex", AttrNameToString{
				"file_contexts": `":myapex-file_contexts"`,
				"manifest":      `"myapex_manifest.json"`,
				"binaries":      `[":bar"]`,
				"native_shared_libs_32": `select({
        "//build/bazel/platforms/arch:arm": [":foo"],
        "//build/bazel/platforms/arch:x86": [":foo"],
        "//conditions:default": [],
    })`,
				"native_shared_libs_64": `select({
        "//build/bazel/platforms/arch:arm64": [":foo"],
        "//build/bazel/platforms/arch:x86_64": [":foo"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestApexCertificateIsSrc(t *testing.T) {
	runApexTestCase(t, Bp2buildTestCase{
		Description:                "apex - certificate is src",
		ModuleTypeUnderTest:        "apex",
		ModuleTypeUnderTestFactory: apex.BundleFactory,
		Filesystem:                 map[string]string{},
		StubbedBuildDefinitions:    []string{"com.android.apogee-file_contexts"},
		Blueprint: `
apex {
	name: "com.android.apogee",
	manifest: "apogee_manifest.json",
	file_contexts: ":com.android.apogee-file_contexts",
	certificate: "com.android.apogee.certificate",
}
` + simpleModule("filegroup", "com.android.apogee-file_contexts"),
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.android.apogee", AttrNameToString{
				"certificate_name": `"com.android.apogee.certificate"`,
				"file_contexts":    `":com.android.apogee-file_contexts"`,
				"manifest":         `"apogee_manifest.json"`,
			}),
		}})
}

func TestBp2BuildOverrideApex_CertificateIsModule(t *testing.T) {
	runOverrideApexTestCase(t, Bp2buildTestCase{
		Description:                "override_apex - certificate is module",
		ModuleTypeUnderTest:        "override_apex",
		ModuleTypeUnderTestFactory: apex.OverrideApexFactory,
		Filesystem:                 map[string]string{},
		StubbedBuildDefinitions: []string{"com.android.apogee.certificate", "com.android.apogee-file_contexts",
			"com.android.apogee", "com.google.android.apogee.certificate"},
		Blueprint: `
android_app_certificate {
	name: "com.android.apogee.certificate",
	certificate: "com.android.apogee",
}

filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [
		"com.android.apogee-file_contexts",
	],
}

apex {
	name: "com.android.apogee",
	manifest: "apogee_manifest.json",
	file_contexts: ":com.android.apogee-file_contexts",
	certificate: ":com.android.apogee.certificate",
}

android_app_certificate {
	name: "com.google.android.apogee.certificate",
	certificate: "com.google.android.apogee",
}

override_apex {
	name: "com.google.android.apogee",
	base: ":com.android.apogee",
	certificate: ":com.google.android.apogee.certificate",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.google.android.apogee", AttrNameToString{
				"base_apex_name": `"com.android.apogee"`,
				"file_contexts":  `":com.android.apogee-file_contexts"`,
				"certificate":    `":com.google.android.apogee.certificate"`,
				"manifest":       `"apogee_manifest.json"`,
			}),
		}})
}

func TestBp2BuildOverrideApex_CertificateIsSrc(t *testing.T) {
	runOverrideApexTestCase(t, Bp2buildTestCase{
		Description:                "override_apex - certificate is src",
		ModuleTypeUnderTest:        "override_apex",
		ModuleTypeUnderTestFactory: apex.OverrideApexFactory,
		Filesystem:                 map[string]string{},
		StubbedBuildDefinitions:    []string{"com.android.apogee", "com.android.apogee.certificate", "com.android.apogee", "com.android.apogee-file_contexts"},
		Blueprint: `
android_app_certificate {
	name: "com.android.apogee.certificate",
	certificate: "com.android.apogee",
}

filegroup {
	name: "com.android.apogee-file_contexts",
	srcs: [
		"com.android.apogee-file_contexts",
	],
}

apex {
	name: "com.android.apogee",
	manifest: "apogee_manifest.json",
	file_contexts: ":com.android.apogee-file_contexts",
	certificate: ":com.android.apogee.certificate",
}

override_apex {
	name: "com.google.android.apogee",
	base: ":com.android.apogee",
	certificate: "com.google.android.apogee.certificate",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.google.android.apogee", AttrNameToString{
				"base_apex_name":   `"com.android.apogee"`,
				"file_contexts":    `":com.android.apogee-file_contexts"`,
				"certificate_name": `"com.google.android.apogee.certificate"`,
				"manifest":         `"apogee_manifest.json"`,
			}),
		}})
}

func TestApexTestBundleSimple(t *testing.T) {
	runApexTestCase(t, Bp2buildTestCase{
		Description:                "apex_test - simple",
		ModuleTypeUnderTest:        "apex_test",
		ModuleTypeUnderTestFactory: apex.TestApexBundleFactory,
		Filesystem:                 map[string]string{},
		StubbedBuildDefinitions:    []string{"cc_test_1"},
		Blueprint: `
cc_test { name: "cc_test_1"}

apex_test {
	name: "test_com.android.apogee",
	file_contexts: "file_contexts_file",
	tests: ["cc_test_1"],
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "test_com.android.apogee", AttrNameToString{
				"file_contexts":  `"file_contexts_file"`,
				"base_apex_name": `"com.android.apogee"`,
				"manifest":       `"apex_manifest.json"`,
				"testonly":       `True`,
				"tests":          `[":cc_test_1"]`,
			}),
		}})
}

func TestApexBundle_overridePlusProductVars(t *testing.T) {
	// Reproduction of b/271424349
	// Tests that overriding an apex that uses product variables correctly copies the product var
	// selects over to the override.
	runOverrideApexTestCase(t, Bp2buildTestCase{
		Description:                "apex - overriding a module that uses product vars",
		ModuleTypeUnderTest:        "override_apex",
		ModuleTypeUnderTestFactory: apex.OverrideApexFactory,
		StubbedBuildDefinitions:    []string{"foo-file_contexts"},
		Blueprint: `
soong_config_string_variable {
    name: "library_linking_strategy",
    values: [
        "prefer_static",
    ],
}

soong_config_module_type {
    name: "library_linking_strategy_apex_defaults",
    module_type: "apex_defaults",
    config_namespace: "ANDROID",
    variables: ["library_linking_strategy"],
    properties: [
        "manifest",
        "min_sdk_version",
    ],
}

library_linking_strategy_apex_defaults {
    name: "higher_min_sdk_when_prefer_static",
    soong_config_variables: {
        library_linking_strategy: {
            // Use the R min_sdk_version
            prefer_static: {},
            // Override the R min_sdk_version to min_sdk_version that supports dcla
            conditions_default: {
                min_sdk_version: "31",
            },
        },
    },
}

filegroup {
	name: "foo-file_contexts",
	srcs: [
		"com.android.apogee-file_contexts",
	],
}

apex {
	name: "foo",
	defaults: ["higher_min_sdk_when_prefer_static"],
	min_sdk_version: "30",
	package_name: "pkg_name",
	file_contexts: ":foo-file_contexts",
}
override_apex {
	name: "override_foo",
	base: ":foo",
	package_name: "override_pkg_name",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "foo", AttrNameToString{
				"file_contexts": `":foo-file_contexts"`,
				"manifest":      `"apex_manifest.json"`,
				"min_sdk_version": `select({
        "//build/bazel/product_config/config_settings:android__library_linking_strategy__prefer_static": "30",
        "//conditions:default": "31",
    })`,
				"package_name": `"pkg_name"`,
			}), MakeBazelTarget("apex", "override_foo", AttrNameToString{
				"base_apex_name": `"foo"`,
				"file_contexts":  `":foo-file_contexts"`,
				"manifest":       `"apex_manifest.json"`,
				"min_sdk_version": `select({
        "//build/bazel/product_config/config_settings:android__library_linking_strategy__prefer_static": "30",
        "//conditions:default": "31",
    })`,
				"package_name": `"override_pkg_name"`,
			}),
		}})
}

func TestApexBundleSimple_customCannedFsConfig(t *testing.T) {
	runApexTestCase(t, Bp2buildTestCase{
		Description:                "apex - custom canned_fs_config",
		ModuleTypeUnderTest:        "apex",
		ModuleTypeUnderTestFactory: apex.BundleFactory,
		Filesystem:                 map[string]string{},
		Blueprint: `
apex {
	name: "com.android.apogee",
	canned_fs_config: "custom.canned_fs_config",
	file_contexts: "file_contexts_file",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("apex", "com.android.apogee", AttrNameToString{
				"canned_fs_config": `"custom.canned_fs_config"`,
				"file_contexts":    `"file_contexts_file"`,
				"manifest":         `"apex_manifest.json"`,
			}),
		}})
}
