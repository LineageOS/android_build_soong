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
	"testing"

	"android/soong/android"
	"android/soong/java"
)

func runJavaSdkLibraryTestCaseWithRegistrationCtxFunc(t *testing.T, tc Bp2buildTestCase, registrationCtxFunc func(ctx android.RegistrationContext)) {
	t.Helper()
	(&tc).ModuleTypeUnderTest = "java_sdk_library"
	(&tc).ModuleTypeUnderTestFactory = java.SdkLibraryFactory
	RunBp2BuildTestCase(t, registrationCtxFunc, tc)
}

func runJavaSdkLibraryTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	runJavaSdkLibraryTestCaseWithRegistrationCtxFunc(t, tc, func(ctx android.RegistrationContext) {})
}

func TestJavaSdkLibraryApiSurfaceGeneral(t *testing.T) {
	runJavaSdkLibraryTestCase(t, Bp2buildTestCase{
		Description: "limited java_sdk_library for api surfaces, general conversion",
		Filesystem: map[string]string{
			"build/soong/scripts/gen-java-current-api-files.sh": "",
			"api/current.txt":               "",
			"api/system-current.txt":        "",
			"api/test-current.txt":          "",
			"api/module-lib-current.txt":    "",
			"api/system-server-current.txt": "",
			"api/removed.txt":               "",
			"api/system-removed.txt":        "",
			"api/test-removed.txt":          "",
			"api/module-lib-removed.txt":    "",
			"api/system-server-removed.txt": "",
		},
		Blueprint: `java_sdk_library {
    name: "java-sdk-lib",
    srcs: ["a.java"],
    public: {enabled: true},
    system: {enabled: true},
    test: {enabled: true},
    module_lib: {enabled: true},
    system_server: {enabled: true},
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_sdk_library", "java-sdk-lib", AttrNameToString{
				"public":        `"api/current.txt"`,
				"system":        `"api/system-current.txt"`,
				"test":          `"api/test-current.txt"`,
				"module_lib":    `"api/module-lib-current.txt"`,
				"system_server": `"api/system-server-current.txt"`,
			}),
		},
	})
}

func TestJavaSdkLibraryApiSurfacePublicDefault(t *testing.T) {
	runJavaSdkLibraryTestCase(t, Bp2buildTestCase{
		Description: "limited java_sdk_library for api surfaces, public prop uses default value",
		Filesystem: map[string]string{
			"build/soong/scripts/gen-java-current-api-files.sh": "",
			"api/current.txt":               "",
			"api/system-current.txt":        "",
			"api/test-current.txt":          "",
			"api/module-lib-current.txt":    "",
			"api/system-server-current.txt": "",
			"api/removed.txt":               "",
			"api/system-removed.txt":        "",
			"api/test-removed.txt":          "",
			"api/module-lib-removed.txt":    "",
			"api/system-server-removed.txt": "",
		},
		Blueprint: `java_sdk_library {
    name: "java-sdk-lib",
    srcs: ["a.java"],
    system: {enabled: false},
    test: {enabled: false},
    module_lib: {enabled: false},
    system_server: {enabled: false},
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_sdk_library", "java-sdk-lib", AttrNameToString{
				"public": `"api/current.txt"`,
			}),
		},
	})
}

func TestJavaSdkLibraryApiSurfacePublicNotEnabled(t *testing.T) {
	runJavaSdkLibraryTestCase(t, Bp2buildTestCase{
		Description: "limited java_sdk_library for api surfaces, public enable is false",
		Filesystem: map[string]string{
			"build/soong/scripts/gen-java-current-api-files.sh": "",
			"api/current.txt": "",
			"api/removed.txt": "",
		},
		Blueprint: `java_sdk_library {
   name: "java-sdk-lib",
   srcs: ["a.java"],
   public: {enabled: false},
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_sdk_library", "java-sdk-lib", AttrNameToString{}),
		},
	})
}

func TestJavaSdkLibraryApiSurfaceNoScopeIsSet(t *testing.T) {
	runJavaSdkLibraryTestCase(t, Bp2buildTestCase{
		Description: "limited java_sdk_library for api surfaces, none of the api scopes is set",
		Filesystem: map[string]string{
			"build/soong/scripts/gen-java-current-api-files.sh": "",
			"api/current.txt":        "",
			"api/system-current.txt": "",
			"api/test-current.txt":   "",
			"api/removed.txt":        "",
			"api/system-removed.txt": "",
			"api/test-removed.txt":   "",
		},
		Blueprint: `java_sdk_library {
   name: "java-sdk-lib",
   srcs: ["a.java"],
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_sdk_library", "java-sdk-lib", AttrNameToString{
				"public": `"api/current.txt"`,
				"system": `"api/system-current.txt"`,
				"test":   `"api/test-current.txt"`,
			}),
		},
	})
}
