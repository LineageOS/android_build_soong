// Copyright 2020 Google Inc. All rights reserved.
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
	"android/soong/genrule"
)

func registerModulesForGensrcsTests(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
}

func TestGensrcs(t *testing.T) {
	testcases := []struct {
		name                    string
		bp                      string
		expectedBazelAttrs      AttrNameToString
		stubbedBuildDefinitions []string
	}{
		{
			name: "gensrcs with common usage of properties",
			bp: `
			gensrcs {
                name: "foo",
                srcs: ["test/input.txt", ":external_files"],
                tool_files: ["program.py"],
                cmd: "$(location program.py) $(in) $(out) $(location foo/file.txt) $(location :external_files)",
                data: ["foo/file.txt", ":external_files"],
                output_extension: "out",
                bazel_module: { bp2build_available: true },
			}
      filegroup {
                name: "external_files",
			}`,
			stubbedBuildDefinitions: []string{"external_files"},
			expectedBazelAttrs: AttrNameToString{
				"srcs": `[
        "test/input.txt",
        ":external_files",
    ]`,
				"tools":            `["program.py"]`,
				"output_extension": `"out"`,
				"cmd":              `"$(location program.py) $(SRC) $(OUT) $(location foo/file.txt) $(location :external_files)"`,
				"data": `[
        "foo/file.txt",
        ":external_files",
    ]`,
			},
		},
		{
			name: "gensrcs with out_extension unset",
			bp: `
			gensrcs {
                name: "foo",
                srcs: ["input.txt"],
                cmd: "cat $(in) > $(out)",
                bazel_module: { bp2build_available: true },
			}`,
			expectedBazelAttrs: AttrNameToString{
				"srcs": `["input.txt"]`,
				"cmd":  `"cat $(SRC) > $(OUT)"`,
			},
		},
	}

	for _, test := range testcases {
		expectedBazelTargets := []string{
			MakeBazelTargetNoRestrictions("gensrcs", "foo", test.expectedBazelAttrs),
		}
		t.Run(test.name, func(t *testing.T) {
			RunBp2BuildTestCase(t, registerModulesForGensrcsTests,
				Bp2buildTestCase{
					ModuleTypeUnderTest:        "gensrcs",
					ModuleTypeUnderTestFactory: genrule.GenSrcsFactory,
					Blueprint:                  test.bp,
					ExpectedBazelTargets:       expectedBazelTargets,
					StubbedBuildDefinitions:    test.stubbedBuildDefinitions,
				})
		})
	}
}
