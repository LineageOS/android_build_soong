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
	"android/soong/android"
	"android/soong/java"
	"fmt"

	"testing"
)

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
		Blueprint: simpleModuleDoNotConvertBp2build("android_library", "static_lib_dep") + `
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
        java_version: "7",
}
`,
		ExpectedBazelTargets: []string{
			makeBazelTarget(
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
					"javacopts":      `["-source 1.7 -target 1.7"]`,
				}),
		}})
}

func TestConvertAndroidLibraryWithNoSources(t *testing.T) {
	t.Helper()
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, Bp2buildTestCase{
		Description:                "Android Library - modules with deps must have sources",
		ModuleTypeUnderTest:        "android_library",
		ModuleTypeUnderTestFactory: java.AndroidLibraryFactory,
		Filesystem: map[string]string{
			"res/res.png":         "",
			"AndroidManifest.xml": "",
		},
		Blueprint: simpleModuleDoNotConvertBp2build("android_library", "lib_dep") + `
android_library {
        name: "TestLib",
        srcs: [],
        manifest: "AndroidManifest.xml",
        libs: ["lib_dep"],
}
`,
		ExpectedErr:          fmt.Errorf("Module has direct dependencies but no sources. Bazel will not allow this."),
		ExpectedBazelTargets: []string{},
	})
}

func TestConvertAndroidLibraryImport(t *testing.T) {
	t.Helper()
	RunBp2BuildTestCase(
		t,
		func(ctx android.RegistrationContext) {
			ctx.RegisterModuleType("android_library", java.AndroidLibraryFactory)
		},
		Bp2buildTestCase{
			Description:                "Android Library Import",
			ModuleTypeUnderTest:        "android_library_import",
			ModuleTypeUnderTestFactory: java.AARImportFactory,
			Filesystem: map[string]string{
				"import.aar": "",
			},
			// Bazel's aar_import can only export *_import targets, so we expect
			// only "static_import_dep" in exports, but both "static_lib_dep" and
			// "static_import_dep" in deps
			Blueprint: simpleModuleDoNotConvertBp2build("android_library", "static_lib_dep") +
				simpleModuleDoNotConvertBp2build("android_library_import", "static_import_dep") + `
android_library_import {
        name: "TestImport",
        aars: ["import.aar"],
        static_libs: ["static_lib_dep", "static_import_dep"],
}
`,
			ExpectedBazelTargets: []string{
				makeBazelTarget(
					"aar_import",
					"TestImport",
					AttrNameToString{
						"aar": `"import.aar"`,
						"deps": `[
        ":static_lib_dep",
        ":static_import_dep",
    ]`,
						"exports": `[":static_import_dep"]`,
					},
				),
			},
		},
	)
}
