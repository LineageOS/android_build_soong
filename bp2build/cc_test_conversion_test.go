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

	"android/soong/android"
	"android/soong/cc"
	"android/soong/genrule"
)

type ccTestBp2buildTestCase struct {
	description string
	blueprint   string
	targets     []testBazelTarget
}

func registerCcTestModuleTypes(ctx android.RegistrationContext) {
	cc.RegisterCCBuildComponents(ctx)
	ctx.RegisterModuleType("cc_binary", cc.BinaryFactory)
	ctx.RegisterModuleType("cc_library_static", cc.LibraryStaticFactory)
	ctx.RegisterModuleType("cc_library", cc.LibraryFactory)
	ctx.RegisterModuleType("cc_test_library", cc.TestLibraryFactory)
	ctx.RegisterModuleType("genrule", genrule.GenRuleFactory)
}

func runCcTestTestCase(t *testing.T, testCase ccTestBp2buildTestCase) {
	t.Helper()
	moduleTypeUnderTest := "cc_test"

	description := fmt.Sprintf("%s %s", moduleTypeUnderTest, testCase.description)
	t.Run(description, func(t *testing.T) {
		t.Helper()
		RunBp2BuildTestCase(t, registerCcTestModuleTypes, Bp2buildTestCase{
			ExpectedBazelTargets:       generateBazelTargetsForTest(testCase.targets, android.HostAndDeviceSupported),
			ModuleTypeUnderTest:        moduleTypeUnderTest,
			ModuleTypeUnderTestFactory: cc.TestFactory,
			Description:                description,
			Blueprint:                  testCase.blueprint,
		})
	})
}

func TestBasicCcTest(t *testing.T) {
	runCcTestTestCase(t, ccTestBp2buildTestCase{
		description: "basic cc_test with commonly used attributes",
		blueprint: `
cc_test {
    name: "mytest",
    host_supported: true,
    srcs: ["test.cpp"],
    target: {
        android: {
            srcs: ["android.cpp"],
            shared_libs: ["foolib"],
        },
        linux: {
            srcs: ["linux.cpp"],
        },
        host: {
            static_libs: ["hostlib"],
        },
    },
    data: [":data_mod", "file.txt"],
    data_bins: [":cc_bin"],
    data_libs: [":cc_lib"],
    cflags: ["-Wall"],
}
` + simpleModuleDoNotConvertBp2build("cc_library", "foolib") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "hostlib") +
			simpleModuleDoNotConvertBp2build("genrule", "data_mod") +
			simpleModuleDoNotConvertBp2build("cc_binary", "cc_bin") +
			simpleModuleDoNotConvertBp2build("cc_test_library", "cc_lib"),
		targets: []testBazelTarget{
			{"cc_test", "mytest", AttrNameToString{
				"copts": `["-Wall"]`,
				"data": `[
        ":data_mod",
        "file.txt",
        ":cc_bin",
        ":cc_lib",
    ]`,
				"deps": `select({
        "//build/bazel/platforms/os:darwin": [":hostlib"],
        "//build/bazel/platforms/os:linux": [":hostlib"],
        "//build/bazel/platforms/os:linux_bionic": [":hostlib"],
        "//build/bazel/platforms/os:linux_musl": [":hostlib"],
        "//build/bazel/platforms/os:windows": [":hostlib"],
        "//conditions:default": [],
    })`,
				"gtest":          "True",
				"isolated":       "True",
				"local_includes": `["."]`,
				"dynamic_deps": `select({
        "//build/bazel/platforms/os:android": [":foolib"],
        "//conditions:default": [],
    })`,
				"srcs": `["test.cpp"] + select({
        "//build/bazel/platforms/os:android": [
            "linux.cpp",
            "android.cpp",
        ],
        "//build/bazel/platforms/os:linux": ["linux.cpp"],
        "//build/bazel/platforms/os:linux_bionic": ["linux.cpp"],
        "//build/bazel/platforms/os:linux_musl": ["linux.cpp"],
        "//conditions:default": [],
    })`,
			},
			},
		},
	})
}

func TestBasicCcTestGtestIsolatedDisabled(t *testing.T) {
	runCcTestTestCase(t, ccTestBp2buildTestCase{
		description: "cc test with disabled gtest and isolated props",
		blueprint: `
cc_test {
    name: "mytest",
    host_supported: true,
    srcs: ["test.cpp"],
    gtest: false,
    isolated: false,
}
`,
		targets: []testBazelTarget{
			{"cc_test", "mytest", AttrNameToString{
				"gtest":          "False",
				"isolated":       "False",
				"local_includes": `["."]`,
				"srcs":           `["test.cpp"]`,
			},
			},
		},
	})
}
