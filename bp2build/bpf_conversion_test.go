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
	"android/soong/bpf"

	"testing"
)

func runBpfTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	(&tc).ModuleTypeUnderTest = "bpf"
	(&tc).ModuleTypeUnderTestFactory = bpf.BpfFactory
	RunBp2BuildTestCase(t, registerBpfModuleTypes, tc)
}

func registerBpfModuleTypes(ctx android.RegistrationContext) {}

func TestBpfSupportedAttrs(t *testing.T) {
	runBpfTestCase(t, Bp2buildTestCase{
		Description: "Bpf module only converts supported attributes",
		Filesystem:  map[string]string{},
		Blueprint: `
bpf {
    name: "bpfTestOut.o",
    srcs: ["bpfTestSrcOne.c",
           "bpfTestSrcTwo.c"],
    btf: true,
    cflags: ["-bpfCflagOne",
             "-bpfCflagTwo"],
    include_dirs: ["ia/ib/ic"],
    sub_dir: "sa/ab",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("bpf", "bpfTestOut.o", AttrNameToString{
				"absolute_includes": `["ia/ib/ic"]`,
				"btf":               `True`,
				"copts": `[
        "-bpfCflagOne",
        "-bpfCflagTwo",
    ]`,
				"srcs": `[
        "bpfTestSrcOne.c",
        "bpfTestSrcTwo.c",
    ]`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
			}),
		},
	})
}
