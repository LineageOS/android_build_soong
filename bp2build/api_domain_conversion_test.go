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
	"android/soong/cc"
)

func registerApiDomainModuleTypes(ctx android.RegistrationContext) {
	android.RegisterApiDomainBuildComponents(ctx)
	cc.RegisterNdkModuleTypes(ctx)
	cc.RegisterLibraryBuildComponents(ctx)
}

func TestApiDomainContributionsTest(t *testing.T) {
	bp := `
	api_domain {
		name: "system",
		cc_api_contributions: [
			"libfoo.ndk",
			"libbar",
		],
	}
	`
	fs := map[string]string{
		"libfoo/Android.bp": `
		ndk_library {
			name: "libfoo",
		}
		`,
		"libbar/Android.bp": `
		cc_library {
			name: "libbar",
		}
		`,
	}
	expectedBazelTarget := MakeBazelTargetNoRestrictions(
		"api_domain",
		"system",
		AttrNameToString{
			"cc_api_contributions": `[
        "//libfoo:libfoo.ndk.contribution",
        "//libbar:libbar.contribution",
    ]`,
			"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
		},
	)
	RunApiBp2BuildTestCase(t, registerApiDomainModuleTypes, Bp2buildTestCase{
		Blueprint:            bp,
		ExpectedBazelTargets: []string{expectedBazelTarget},
		Filesystem:           fs,
	})
}
