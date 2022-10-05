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

	"android/soong/cc"
)

func TestNdkLibraryContributionSymbolFile(t *testing.T) {
	bp := `
	ndk_library {
		name: "libfoo",
		symbol_file: "libfoo.map.txt",
	}
	`
	expectedBazelTarget := MakeBazelTargetNoRestrictions(
		"cc_api_contribution",
		"libfoo.ndk.contribution",
		AttrNameToString{
			"api":                    `"libfoo.map.txt"`,
			"api_surfaces":           `["publicapi"]`,
			"library_name":           `"libfoo"`,
			"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
		},
	)
	RunApiBp2BuildTestCase(t, cc.RegisterNdkModuleTypes, Bp2buildTestCase{
		Blueprint:            bp,
		ExpectedBazelTargets: []string{expectedBazelTarget},
	})
}

func TestNdkLibraryContributionHeaders(t *testing.T) {
	bp := `
	ndk_library {
		name: "libfoo",
		symbol_file: "libfoo.map.txt",
		export_header_libs: ["libfoo_headers"],
	}
	`
	fs := map[string]string{
		"header_directory/Android.bp": `
		ndk_headers {
			name: "libfoo_headers",
		}
		`,
	}
	expectedBazelTarget := MakeBazelTargetNoRestrictions(
		"cc_api_contribution",
		"libfoo.ndk.contribution",
		AttrNameToString{
			"api":                    `"libfoo.map.txt"`,
			"api_surfaces":           `["publicapi"]`,
			"library_name":           `"libfoo"`,
			"hdrs":                   `["//header_directory:libfoo_headers.contribution"]`,
			"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
		},
	)
	RunApiBp2BuildTestCase(t, cc.RegisterNdkModuleTypes, Bp2buildTestCase{
		Blueprint:            bp,
		Filesystem:           fs,
		ExpectedBazelTargets: []string{expectedBazelTarget},
	})
}
