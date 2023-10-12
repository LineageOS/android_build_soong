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

	"testing"
)

func runJavaImportTestCaseWithRegistrationCtxFunc(t *testing.T, tc Bp2buildTestCase, registrationCtxFunc func(ctx android.RegistrationContext)) {
	t.Helper()
	(&tc).ModuleTypeUnderTest = "java_import"
	(&tc).ModuleTypeUnderTestFactory = java.ImportFactory
	RunBp2BuildTestCase(t, registrationCtxFunc, tc)
}

func runJavaImportTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, registerJavaImportModuleTypes, tc)
}

func registerJavaImportModuleTypes(ctx android.RegistrationContext) {
}

func TestJavaImportMinimal(t *testing.T) {
	runJavaImportTestCase(t, Bp2buildTestCase{
		Description:                "Java import - simple example",
		ModuleTypeUnderTest:        "java_import",
		ModuleTypeUnderTestFactory: java.ImportFactory,
		Filesystem: map[string]string{
			"import.jar": "",
		},
		Blueprint: `
java_import {
        name: "example_import",
        jars: ["import.jar"],
        bazel_module: { bp2build_available: true },
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_import", "example_import", AttrNameToString{
				"jars": `["import.jar"]`,
			}),
			MakeBazelTarget("java_library", "example_import-neverlink", AttrNameToString{
				"exports":     `[":example_import"]`,
				"neverlink":   `True`,
				"sdk_version": `"none"`,
			}),
		}})
}

func TestJavaImportArchVariant(t *testing.T) {
	runJavaImportTestCase(t, Bp2buildTestCase{
		Description:                "Java import - simple example",
		ModuleTypeUnderTest:        "java_import",
		ModuleTypeUnderTestFactory: java.ImportFactory,
		Filesystem: map[string]string{
			"import.jar": "",
		},
		Blueprint: `
java_import {
        name: "example_import",
		target: {
			android: {
				jars: ["android.jar"],
			},
			linux_glibc: {
				jars: ["linux.jar"],
			},
		},
        bazel_module: { bp2build_available: true },
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_import", "example_import", AttrNameToString{
				"jars": `select({
        "//build/bazel_common_rules/platforms/os:android": ["android.jar"],
        "//build/bazel_common_rules/platforms/os:linux_glibc": ["linux.jar"],
        "//conditions:default": [],
    })`,
			}),
			MakeBazelTarget("java_library", "example_import-neverlink", AttrNameToString{
				"exports":     `[":example_import"]`,
				"neverlink":   `True`,
				"sdk_version": `"none"`,
			}),
		}})
}

func TestJavaImportHost(t *testing.T) {
	runJavaImportTestCase(t, Bp2buildTestCase{
		Description:                "Java import host- simple example",
		ModuleTypeUnderTest:        "java_import_host",
		ModuleTypeUnderTestFactory: java.ImportFactory,
		Filesystem: map[string]string{
			"import.jar": "",
		},
		Blueprint: `
java_import_host {
        name: "example_import",
        jars: ["import.jar"],
        bazel_module: { bp2build_available: true },
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_import", "example_import", AttrNameToString{
				"jars": `["import.jar"]`,
			}),
			MakeBazelTarget("java_library", "example_import-neverlink", AttrNameToString{
				"exports":     `[":example_import"]`,
				"neverlink":   `True`,
				"sdk_version": `"none"`,
			}),
		}})
}

func TestJavaImportSameNameAsJavaLibrary(t *testing.T) {
	runJavaImportTestCaseWithRegistrationCtxFunc(t, Bp2buildTestCase{
		Description: "java_import has the same name as other package java_library's",
		Filesystem: map[string]string{
			"foo/bar/Android.bp": simpleModule("java_library", "test_lib"),
			"test.jar":           "",
		},
		Blueprint: `java_import {
    name: "test_lib",
    jars: ["test.jar"],
    bazel_module: { bp2build_available: true },
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_import", "test_lib", AttrNameToString{
				"jars": `["test.jar"]`,
			}),
			MakeBazelTarget("java_library", "test_lib-neverlink", AttrNameToString{
				"exports":     `[":test_lib"]`,
				"neverlink":   `True`,
				"sdk_version": `"none"`,
			}),
		},
	}, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("java_library", java.LibraryFactory)
	})
}
