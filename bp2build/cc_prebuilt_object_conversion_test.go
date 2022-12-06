// Copyright 2022 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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

func runCcPrebuiltObjectTestCase(t *testing.T, testCase Bp2buildTestCase) {
	t.Helper()
	description := fmt.Sprintf("cc_prebuilt_object: %s", testCase.Description)
	testCase.ModuleTypeUnderTest = "cc_prebuilt_object"
	testCase.ModuleTypeUnderTestFactory = cc.PrebuiltObjectFactory
	testCase.Description = description
	t.Run(description, func(t *testing.T) {
		t.Helper()
		RunBp2BuildTestCaseSimple(t, testCase)
	})
}

func TestPrebuiltObject(t *testing.T) {
	runCcPrebuiltObjectTestCase(t,
		Bp2buildTestCase{
			Description: "simple",
			Filesystem: map[string]string{
				"obj.o": "",
			},
			Blueprint: `
cc_prebuilt_object {
	name: "objtest",
	srcs: ["obj.o"],
	bazel_module: { bp2build_available: true },
}`,
			ExpectedBazelTargets: []string{
				MakeBazelTarget("cc_prebuilt_object", "objtest", AttrNameToString{
					"src": `"obj.o"`,
				})},
		})
}

func TestPrebuiltObjectWithArchVariance(t *testing.T) {
	runCcPrebuiltObjectTestCase(t,
		Bp2buildTestCase{
			Description: "with arch variance",
			Filesystem: map[string]string{
				"obja.o": "",
				"objb.o": "",
			},
			Blueprint: `
cc_prebuilt_object {
	name: "objtest",
	arch: {
		arm64: { srcs: ["obja.o"], },
		arm: { srcs: ["objb.o"], },
	},
	bazel_module: { bp2build_available: true },
}`, ExpectedBazelTargets: []string{
				MakeBazelTarget("cc_prebuilt_object", "objtest", AttrNameToString{
					"src": `select({
        "//build/bazel/platforms/arch:arm": "objb.o",
        "//build/bazel/platforms/arch:arm64": "obja.o",
        "//conditions:default": None,
    })`,
				}),
			},
		})
}

func TestPrebuiltObjectMultipleSrcsFails(t *testing.T) {
	runCcPrebuiltObjectTestCase(t,
		Bp2buildTestCase{
			Description: "fails because multiple sources",
			Filesystem: map[string]string{
				"obja": "",
				"objb": "",
			},
			Blueprint: `
cc_prebuilt_object {
	name: "objtest",
	srcs: ["obja.o", "objb.o"],
	bazel_module: { bp2build_available: true },
}`,
			ExpectedErr: fmt.Errorf("Expected at most one source file"),
		})
}

// TODO: nosrcs test
