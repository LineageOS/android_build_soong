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
				"exports":   `[":example_import"]`,
				"neverlink": `True`,
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
        "//build/bazel/platforms/os:android": ["android.jar"],
        "//build/bazel/platforms/os:linux_glibc": ["linux.jar"],
        "//conditions:default": [],
    })`,
			}),
			MakeBazelTarget("java_library", "example_import-neverlink", AttrNameToString{
				"exports":   `[":example_import"]`,
				"neverlink": `True`,
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
				"exports":   `[":example_import"]`,
				"neverlink": `True`,
			}),
		}})
}
