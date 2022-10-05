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
	"fmt"
	"testing"

	"android/soong/cc"
)

func TestNdkHeaderFilepaths(t *testing.T) {
	bpTemplate := `
	ndk_headers {
		name: "foo",
		srcs: %v,
		exclude_srcs: %v,
	}
	`
	testCases := []struct {
		desc         string
		srcs         string
		excludeSrcs  string
		expectedHdrs string
	}{
		{
			desc:         "Single header file",
			srcs:         `["foo.h"]`,
			excludeSrcs:  `[]`,
			expectedHdrs: `["foo.h"]`,
		},
		{
			desc:        "Multiple header files",
			srcs:        `["foo.h", "foo_other.h"]`,
			excludeSrcs: `[]`,
			expectedHdrs: `[
        "foo.h",
        "foo_other.h",
    ]`,
		},
		{
			desc:         "Multiple header files with excludes",
			srcs:         `["foo.h", "foo_other.h"]`,
			excludeSrcs:  `["foo_other.h"]`,
			expectedHdrs: `["foo.h"]`,
		},
		{
			desc:        "Multiple header files via Soong-supported globs",
			srcs:        `["*.h"]`,
			excludeSrcs: `[]`,
			expectedHdrs: `[
        "foo.h",
        "foo_other.h",
    ]`,
		},
	}
	for _, testCase := range testCases {
		fs := map[string]string{
			"foo.h":       "",
			"foo_other.h": "",
		}
		expectedApiContributionTargetName := "foo.contribution"
		expectedBazelTarget := MakeBazelTargetNoRestrictions(
			"cc_api_headers",
			expectedApiContributionTargetName,
			AttrNameToString{
				"hdrs": testCase.expectedHdrs,
			},
		)
		RunApiBp2BuildTestCase(t, cc.RegisterNdkModuleTypes, Bp2buildTestCase{
			Description:          testCase.desc,
			Blueprint:            fmt.Sprintf(bpTemplate, testCase.srcs, testCase.excludeSrcs),
			ExpectedBazelTargets: []string{expectedBazelTarget},
			Filesystem:           fs,
		})
	}
}

func TestNdkHeaderIncludeDir(t *testing.T) {
	bpTemplate := `
	ndk_headers {
		name: "foo",
		from: %v,
		to: "this/value/is/ignored",
	}
	`
	testCases := []struct {
		desc               string
		from               string
		expectedIncludeDir string
	}{
		{
			desc:               "Empty `from` value",
			from:               `""`,
			expectedIncludeDir: `""`,
		},
		{
			desc:               "Non-Empty `from` value",
			from:               `"include"`,
			expectedIncludeDir: `"include"`,
		},
	}
	for _, testCase := range testCases {
		expectedApiContributionTargetName := "foo.contribution"
		expectedBazelTarget := MakeBazelTargetNoRestrictions(
			"cc_api_headers",
			expectedApiContributionTargetName,
			AttrNameToString{
				"include_dir": testCase.expectedIncludeDir,
			},
		)
		RunApiBp2BuildTestCase(t, cc.RegisterNdkModuleTypes, Bp2buildTestCase{
			Description:          testCase.desc,
			Blueprint:            fmt.Sprintf(bpTemplate, testCase.from),
			ExpectedBazelTargets: []string{expectedBazelTarget},
		})
	}
}

func TestVersionedNdkHeaderFilepaths(t *testing.T) {
	bp := `
	versioned_ndk_headers {
		name: "common_libc",
		from: "include"
	}
	`
	fs := map[string]string{
		"include/math.h":    "",
		"include/stdio.h":   "",
		"include/arm/arm.h": "",
		"include/x86/x86.h": "",
	}
	expectedApiContributionTargetName := "common_libc.contribution"
	expectedBazelTarget := MakeBazelTargetNoRestrictions(
		"cc_api_headers",
		expectedApiContributionTargetName,
		AttrNameToString{
			"include_dir": `"include"`,
			"hdrs": `[
        "include/math.h",
        "include/stdio.h",
        "include/arm/arm.h",
        "include/x86/x86.h",
    ]`,
		},
	)
	RunApiBp2BuildTestCase(t, cc.RegisterNdkModuleTypes, Bp2buildTestCase{
		Blueprint:            bp,
		Filesystem:           fs,
		ExpectedBazelTargets: []string{expectedBazelTarget},
	})
}
