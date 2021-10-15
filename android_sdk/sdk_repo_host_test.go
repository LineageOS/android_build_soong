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

package android_sdk

import (
	"fmt"
	"runtime"
	"sort"
	"testing"

	"android/soong/android"
	"android/soong/cc"

	"github.com/google/blueprint/pathtools"
)

var fixture = android.GroupFixturePreparers(
	android.PrepareForIntegrationTestWithAndroid,
	cc.PrepareForIntegrationTestWithCc,
	android.FixtureRegisterWithContext(registerBuildComponents),
)

func TestSdkRepoHostDeps(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("Skipping sdk_repo_host testing that is only supported on linux not %s", runtime.GOOS)
	}

	result := fixture.RunTestWithBp(t, `
		android_sdk_repo_host {
			name: "platform-tools",
		}
	`)

	// produces "sdk-repo-{OS}-platform-tools.zip"
	result.ModuleForTests("platform-tools", "linux_glibc_common").Output("sdk-repo-linux-platform-tools.zip")
}

func TestRemapPackageSpecs(t *testing.T) {
	testcases := []struct {
		name   string
		input  []string
		remaps []remapProperties
		output []string
		err    string
	}{
		{
			name:  "basic remap",
			input: []string{"a", "c"},
			remaps: []remapProperties{
				{From: "a", To: "b"},
			},
			output: []string{"b", "c"},
		},
		{
			name:  "non-matching remap",
			input: []string{"a"},
			remaps: []remapProperties{
				{From: "b", To: "c"},
			},
			output: []string{"a"},
		},
		{
			name:  "glob",
			input: []string{"bin/d", "liba.so", "libb.so", "lib/c.so"},
			remaps: []remapProperties{
				{From: "lib*.so", To: "lib/"},
			},
			output: []string{"bin/d", "lib/c.so", "lib/liba.so", "lib/libb.so"},
		},
		{
			name:  "bad glob",
			input: []string{"a"},
			remaps: []remapProperties{
				{From: "**", To: "./"},
			},
			err: fmt.Sprintf("Error parsing \"**\": %v", pathtools.GlobLastRecursiveErr.Error()),
		},
		{
			name:  "globbed dirs",
			input: []string{"a/b/c"},
			remaps: []remapProperties{
				{From: "a/*/c", To: "./"},
			},
			output: []string{"b/c"},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			specs := map[string]android.PackagingSpec{}
			for _, input := range test.input {
				spec := android.PackagingSpec{}
				spec.SetRelPathInPackage(input)
				specs[input] = spec
			}

			err := remapPackageSpecs(specs, test.remaps)

			if test.err != "" {
				android.AssertErrorMessageEquals(t, "", test.err, err)
			} else {
				outputs := []string{}
				for path, spec := range specs {
					android.AssertStringEquals(t, "path does not match rel path", path, spec.RelPathInPackage())
					outputs = append(outputs, path)
				}
				sort.Strings(outputs)
				android.AssertArrayString(t, "outputs mismatch", test.output, outputs)
			}
		})
	}
}
