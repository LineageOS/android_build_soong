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
	"fmt"
	"testing"

	"android/soong/android"
)

func runFilegroupTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	(&tc).ModuleTypeUnderTest = "filegroup"
	(&tc).ModuleTypeUnderTestFactory = android.FileGroupFactory
	RunBp2BuildTestCase(t, registerFilegroupModuleTypes, tc)
}

func registerFilegroupModuleTypes(ctx android.RegistrationContext) {}

func TestFilegroupSameNameAsFile_OneFile(t *testing.T) {
	runFilegroupTestCase(t, Bp2buildTestCase{
		Description: "filegroup - same name as file, with one file",
		Filesystem:  map[string]string{},
		Blueprint: `
filegroup {
    name: "foo",
    srcs: ["foo"],
}
`,
		ExpectedBazelTargets: []string{}})
}

func TestFilegroupSameNameAsFile_MultipleFiles(t *testing.T) {
	runFilegroupTestCase(t, Bp2buildTestCase{
		Description: "filegroup - same name as file, with multiple files",
		Filesystem:  map[string]string{},
		Blueprint: `
filegroup {
	name: "foo",
	srcs: ["foo", "bar"],
}
`,
		ExpectedErr: fmt.Errorf("filegroup 'foo' cannot contain a file with the same name"),
	})
}

func TestFilegroupWithAidlSrcs(t *testing.T) {
	testcases := []struct {
		name               string
		bp                 string
		expectedBazelAttrs AttrNameToString
	}{
		{
			name: "filegroup with only aidl srcs",
			bp: `
	filegroup {
		name: "foo",
		srcs: ["aidl/foo.aidl"],
		path: "aidl",
	}`,
			expectedBazelAttrs: AttrNameToString{
				"srcs":                `["aidl/foo.aidl"]`,
				"strip_import_prefix": `"aidl"`,
			},
		},
		{
			name: "filegroup without path",
			bp: `
	filegroup {
		name: "foo",
		srcs: ["aidl/foo.aidl"],
	}`,
			expectedBazelAttrs: AttrNameToString{
				"srcs": `["aidl/foo.aidl"]`,
			},
		},
	}

	for _, test := range testcases {
		expectedBazelTargets := []string{
			MakeBazelTargetNoRestrictions("aidl_library", "foo", test.expectedBazelAttrs),
		}
		runFilegroupTestCase(t, Bp2buildTestCase{
			Description:          test.name,
			Blueprint:            test.bp,
			ExpectedBazelTargets: expectedBazelTargets,
		})
	}
}

func TestFilegroupWithAidlAndNonAidlSrcs(t *testing.T) {
	runFilegroupTestCase(t, Bp2buildTestCase{
		Description: "filegroup with aidl and non-aidl srcs",
		Filesystem:  map[string]string{},
		Blueprint: `
filegroup {
    name: "foo",
    srcs: [
		"aidl/foo.aidl",
		"buf.proto",
	],
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTargetNoRestrictions("filegroup", "foo", AttrNameToString{
				"srcs": `[
        "aidl/foo.aidl",
        "buf.proto",
    ]`}),
		}})
}

func TestFilegroupWithProtoSrcs(t *testing.T) {
	runFilegroupTestCase(t, Bp2buildTestCase{
		Description: "filegroup with proto and non-proto srcs",
		Filesystem:  map[string]string{},
		Blueprint: `
filegroup {
		name: "foo",
		srcs: ["proto/foo.proto"],
		path: "proto",
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTargetNoRestrictions("proto_library", "foo_bp2build_converted", AttrNameToString{
				"srcs":                `["proto/foo.proto"]`,
				"strip_import_prefix": `"proto"`}),
			MakeBazelTargetNoRestrictions("filegroup", "foo", AttrNameToString{
				"srcs": `["proto/foo.proto"]`}),
		}})
}

func TestFilegroupWithProtoAndNonProtoSrcs(t *testing.T) {
	runFilegroupTestCase(t, Bp2buildTestCase{
		Description: "filegroup with proto and non-proto srcs",
		Filesystem:  map[string]string{},
		Blueprint: `
filegroup {
    name: "foo",
    srcs: [
		"foo.proto",
		"buf.cpp",
	],
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTargetNoRestrictions("filegroup", "foo", AttrNameToString{
				"srcs": `[
        "foo.proto",
        "buf.cpp",
    ]`}),
		}})
}
