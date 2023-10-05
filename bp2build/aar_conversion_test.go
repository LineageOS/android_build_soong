// Copyright 2022 Google Inc. All rights reserved.
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
	"testing"

	"android/soong/android"
	"android/soong/java"
)

func runAndroidLibraryImportTestWithRegistrationCtxFunc(t *testing.T, registrationCtxFunc func(ctx android.RegistrationContext), tc Bp2buildTestCase) {
	t.Helper()
	(&tc).ModuleTypeUnderTest = "android_library_import"
	(&tc).ModuleTypeUnderTestFactory = java.AARImportFactory
	RunBp2BuildTestCase(t, registrationCtxFunc, tc)
}

func runAndroidLibraryImportTest(t *testing.T, tc Bp2buildTestCase) {
	runAndroidLibraryImportTestWithRegistrationCtxFunc(t, func(ctx android.RegistrationContext) {}, tc)
}

func TestConvertAndroidLibrary(t *testing.T) {
	t.Helper()
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, Bp2buildTestCase{
		Description:                "Android Library - simple example",
		ModuleTypeUnderTest:        "android_library",
		ModuleTypeUnderTestFactory: java.AndroidLibraryFactory,
		Filesystem: map[string]string{
			"lib.java":                     "",
			"arm.java":                     "",
			"x86.java":                     "",
			"res/res.png":                  "",
			"manifest/AndroidManifest.xml": "",
		},
		StubbedBuildDefinitions: []string{"static_lib_dep"},
		Blueprint: simpleModule("android_library", "static_lib_dep") + `
android_library {
	name: "TestLib",
	srcs: ["lib.java"],
	arch: {
		arm: {
			srcs: ["arm.java"],
		},
		x86: {
			srcs: ["x86.java"],
		}
	},
	manifest: "manifest/AndroidManifest.xml",
	static_libs: ["static_lib_dep"],
	sdk_version: "current",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget(
				"android_library",
				"TestLib",
				AttrNameToString{
					"srcs": `["lib.java"] + select({
        "//build/bazel/platforms/arch:arm": ["arm.java"],
        "//build/bazel/platforms/arch:x86": ["x86.java"],
        "//conditions:default": [],
    })`,
					"manifest":       `"manifest/AndroidManifest.xml"`,
					"resource_files": `["res/res.png"]`,
					"deps":           `[":static_lib_dep"]`,
					"exports":        `[":static_lib_dep"]`,
					"sdk_version":    `"current"`, // use as default
				}),
			MakeNeverlinkDuplicateTarget("android_library", "TestLib"),
		}})
}

func TestConvertAndroidLibraryWithNoSources(t *testing.T) {
	t.Helper()
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, Bp2buildTestCase{
		Description:                "Android Library - modules will deps when there are no sources",
		ModuleTypeUnderTest:        "android_library",
		ModuleTypeUnderTestFactory: java.AndroidLibraryFactory,
		Filesystem: map[string]string{
			"res/res.png":         "",
			"AndroidManifest.xml": "",
		},
		Blueprint: simpleModule("android_library", "lib_dep") + `
android_library {
	name: "TestLib",
	srcs: [],
	manifest: "AndroidManifest.xml",
	libs: ["lib_dep"],
	sdk_version: "current",
}
`,
		StubbedBuildDefinitions: []string{"lib_dep"},
		ExpectedBazelTargets: []string{
			MakeBazelTarget(
				"android_library",
				"TestLib",
				AttrNameToString{
					"manifest":       `"AndroidManifest.xml"`,
					"resource_files": `["res/res.png"]`,
					"sdk_version":    `"current"`, // use as default
				},
			),
			MakeNeverlinkDuplicateTarget("android_library", "TestLib"),
		},
	})
}

func TestConvertAndroidLibraryImport(t *testing.T) {
	runAndroidLibraryImportTestWithRegistrationCtxFunc(t,
		func(ctx android.RegistrationContext) {
			ctx.RegisterModuleType("android_library", java.AndroidLibraryFactory)
		},
		Bp2buildTestCase{
			Description:             "Android Library Import",
			StubbedBuildDefinitions: []string{"static_lib_dep", "static_import_dep", "static_import_dep-neverlink"},
			// Bazel's aar_import can only export *_import targets, so we expect
			// only "static_import_dep" in exports, but both "static_lib_dep" and
			// "static_import_dep" in deps
			Blueprint: simpleModule("android_library", "static_lib_dep") + `
android_library_import {
        name: "TestImport",
        aars: ["import.aar"],
        static_libs: ["static_lib_dep", "static_import_dep"],
    sdk_version: "current",
}

android_library_import {
        name: "static_import_dep",
        aars: ["import.aar"],
}
`,
			ExpectedBazelTargets: []string{
				MakeBazelTarget(
					"aar_import",
					"TestImport",
					AttrNameToString{
						"aar": `"import.aar"`,
						"deps": `[
        ":static_lib_dep",
        ":static_import_dep",
    ]`,
						"exports":     `[":static_import_dep"]`,
						"sdk_version": `"current"`, // use as default
					},
				),
				MakeNeverlinkDuplicateTarget("android_library", "TestImport"),
			},
		},
	)
}

func TestConvertAndroidLibraryKotlin(t *testing.T) {
	t.Helper()
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, Bp2buildTestCase{
		Description:                "Android Library with .kt srcs and common_srcs attribute",
		ModuleTypeUnderTest:        "android_library",
		ModuleTypeUnderTestFactory: java.AndroidLibraryFactory,
		Filesystem: map[string]string{
			"AndroidManifest.xml": "",
		},
		Blueprint: `
android_library {
	name: "TestLib",
	srcs: ["a.java", "b.kt"],
	common_srcs: ["c.kt"],
	sdk_version: "current",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget(
				"android_library",
				"TestLib",
				AttrNameToString{
					"srcs": `[
        "a.java",
        "b.kt",
    ]`,
					"common_srcs":    `["c.kt"]`,
					"manifest":       `"AndroidManifest.xml"`,
					"resource_files": `[]`,
					"sdk_version":    `"current"`, // use as default
				}),
			MakeNeverlinkDuplicateTarget("android_library", "TestLib"),
		}})
}

func TestConvertAndroidLibraryKotlinCflags(t *testing.T) {
	t.Helper()
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, Bp2buildTestCase{
		Description:                "Android Library with .kt srcs and kotlincflags ",
		ModuleTypeUnderTest:        "android_library",
		ModuleTypeUnderTestFactory: java.AndroidLibraryFactory,
		Filesystem: map[string]string{
			"AndroidManifest.xml": "",
		},
		Blueprint: `
android_library {
	name: "TestLib",
	srcs: ["a.java", "b.kt"],
	kotlincflags: ["-flag1", "-flag2"],
	sdk_version: "current",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget(
				"android_library",
				"TestLib",
				AttrNameToString{
					"srcs": `[
        "a.java",
        "b.kt",
    ]`,
					"kotlincflags": `[
        "-flag1",
        "-flag2",
    ]`,
					"manifest":       `"AndroidManifest.xml"`,
					"resource_files": `[]`,
					"sdk_version":    `"current"`, // use as default
				}),
			MakeNeverlinkDuplicateTarget("android_library", "TestLib"),
		}})
}

func TestAarImportFailsToConvertNoAars(t *testing.T) {
	runAndroidLibraryImportTest(t,
		Bp2buildTestCase{
			Description: "Android Library Import with no aars does not convert.",
			Blueprint: `
android_library_import {
        name: "no_aar_import",
}
`,
			ExpectedBazelTargets: []string{},
		})
}
