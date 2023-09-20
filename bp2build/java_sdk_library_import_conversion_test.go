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

	"android/soong/java"
)

func runJavaSdkLibraryImportTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, java.RegisterSdkLibraryBuildComponents, tc)
}

func TestJavaSdkLibraryImport(t *testing.T) {
	runJavaSdkLibraryImportTestCase(t, Bp2buildTestCase{
		Blueprint: `
java_sdk_library_import {
	name : "foo",
	public: {
		current_api: "foo_current.txt",
	},
	system: {
		current_api: "system_foo_current.txt",
	},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_sdk_library", "foo", AttrNameToString{
				"public": `"foo_current.txt"`,
				"system": `"system_foo_current.txt"`,
			}),
		},
	})
}

func TestJavaSdkLibraryImportPrebuiltPrefixRemoved(t *testing.T) {
	runJavaSdkLibraryImportTestCase(t, Bp2buildTestCase{
		Filesystem: map[string]string{
			"foobar/Android.bp": `
java_sdk_library {
	name: "foo",
	srcs: ["**/*.java"],
}
`,
			"foobar/api/current.txt":        "",
			"foobar/api/system-current.txt": "",
			"foobar/api/test-current.txt":   "",
			"foobar/api/removed.txt":        "",
			"foobar/api/system-removed.txt": "",
			"foobar/api/test-removed.txt":   "",
		},
		Blueprint: `
java_sdk_library_import {
	name : "foo",
	public: {
		current_api: "foo_current.txt",
	},
	system: {
		current_api: "system_foo_current.txt",
	},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_sdk_library", "foo", AttrNameToString{
				"public": `"foo_current.txt"`,
				"system": `"system_foo_current.txt"`,
			}),
		},
	})
}
