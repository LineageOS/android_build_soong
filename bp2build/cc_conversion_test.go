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
	"android/soong/android"
	"android/soong/cc"
	"strings"
	"testing"
)

const (
	// See cc/testing.go for more context
	soongCcLibraryPreamble = `
cc_defaults {
	name: "linux_bionic_supported",
}

toolchain_library {
	name: "libclang_rt.builtins-x86_64-android",
	defaults: ["linux_bionic_supported"],
	vendor_available: true,
	vendor_ramdisk_available: true,
	product_available: true,
	recovery_available: true,
	native_bridge_supported: true,
	src: "",
}

toolchain_library {
	name: "libatomic",
	defaults: ["linux_bionic_supported"],
	vendor_available: true,
	vendor_ramdisk_available: true,
	product_available: true,
	recovery_available: true,
	native_bridge_supported: true,
	src: "",
}`
)

func TestCcLibraryHeadersLoadStatement(t *testing.T) {
	testCases := []struct {
		bazelTargets           BazelTargets
		expectedLoadStatements string
	}{
		{
			bazelTargets: BazelTargets{
				BazelTarget{
					name:      "cc_library_headers_target",
					ruleClass: "cc_library_headers",
					// Note: no bzlLoadLocation for native rules
				},
			},
			expectedLoadStatements: ``,
		},
	}

	for _, testCase := range testCases {
		actual := testCase.bazelTargets.LoadStatements()
		expected := testCase.expectedLoadStatements
		if actual != expected {
			t.Fatalf("Expected load statements to be %s, got %s", expected, actual)
		}
	}

}

func TestCcLibraryHeadersBp2Build(t *testing.T) {
	testCases := []struct {
		description                        string
		moduleTypeUnderTest                string
		moduleTypeUnderTestFactory         android.ModuleFactory
		moduleTypeUnderTestBp2BuildMutator func(android.TopDownMutatorContext)
		preArchMutators                    []android.RegisterMutatorFunc
		depsMutators                       []android.RegisterMutatorFunc
		bp                                 string
		expectedBazelTargets               []string
		filesystem                         map[string]string
		dir                                string
	}{
		{
			description:                        "cc_library_headers test",
			moduleTypeUnderTest:                "cc_library_headers",
			moduleTypeUnderTestFactory:         cc.LibraryHeaderFactory,
			moduleTypeUnderTestBp2BuildMutator: cc.CcLibraryHeadersBp2Build,
			filesystem: map[string]string{
				"lib-1/lib1a.h": "",
				"lib-1/lib1b.h": "",
				"lib-2/lib2a.h": "",
				"lib-2/lib2b.h": "",
				"dir-1/dir1a.h": "",
				"dir-1/dir1b.h": "",
				"dir-2/dir2a.h": "",
				"dir-2/dir2b.h": "",
			},
			bp: soongCcLibraryPreamble + `
cc_library_headers {
    name: "lib-1",
    export_include_dirs: ["lib-1"],
    bazel_module: { bp2build_available: true },
}

cc_library_headers {
    name: "lib-2",
    export_include_dirs: ["lib-2"],
    bazel_module: { bp2build_available: true },
}

cc_library_headers {
    name: "foo_headers",
    export_include_dirs: ["dir-1", "dir-2"],
    header_libs: ["lib-1", "lib-2"],
    export_header_lib_headers: ["lib-1", "lib-2"],
    bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{`cc_library_headers(
    name = "foo_headers",
    deps = [
        ":lib-1",
        ":lib-2",
    ],
    hdrs = [
        "dir-1/dir1a.h",
        "dir-1/dir1b.h",
        "dir-2/dir2a.h",
        "dir-2/dir2b.h",
    ],
    includes = [
        "dir-1",
        "dir-2",
    ],
)`, `cc_library_headers(
    name = "lib-1",
    hdrs = [
        "lib-1/lib1a.h",
        "lib-1/lib1b.h",
    ],
    includes = [
        "lib-1",
    ],
)`, `cc_library_headers(
    name = "lib-2",
    hdrs = [
        "lib-2/lib2a.h",
        "lib-2/lib2b.h",
    ],
    includes = [
        "lib-2",
    ],
)`},
		},
	}

	dir := "."
	for _, testCase := range testCases {
		filesystem := make(map[string][]byte)
		toParse := []string{
			"Android.bp",
		}
		for f, content := range testCase.filesystem {
			if strings.HasSuffix(f, "Android.bp") {
				toParse = append(toParse, f)
			}
			filesystem[f] = []byte(content)
		}
		config := android.TestConfig(buildDir, nil, testCase.bp, filesystem)
		ctx := android.NewTestContext(config)

		cc.RegisterCCBuildComponents(ctx)
		ctx.RegisterModuleType("toolchain_library", cc.ToolchainLibraryFactory)

		ctx.RegisterModuleType(testCase.moduleTypeUnderTest, testCase.moduleTypeUnderTestFactory)
		for _, m := range testCase.depsMutators {
			ctx.DepsBp2BuildMutators(m)
		}
		ctx.RegisterBp2BuildMutator(testCase.moduleTypeUnderTest, testCase.moduleTypeUnderTestBp2BuildMutator)
		ctx.RegisterForBazelConversion()

		_, errs := ctx.ParseFileList(dir, toParse)
		if Errored(t, testCase.description, errs) {
			continue
		}
		_, errs = ctx.ResolveDependencies(config)
		if Errored(t, testCase.description, errs) {
			continue
		}

		checkDir := dir
		if testCase.dir != "" {
			checkDir = testCase.dir
		}
		bazelTargets := GenerateBazelTargets(ctx.Context.Context, Bp2Build)[checkDir]
		if actualCount, expectedCount := len(bazelTargets), len(testCase.expectedBazelTargets); actualCount != expectedCount {
			t.Errorf("%s: Expected %d bazel target, got %d", testCase.description, expectedCount, actualCount)
		} else {
			for i, target := range bazelTargets {
				if w, g := testCase.expectedBazelTargets[i], target.content; w != g {
					t.Errorf(
						"%s: Expected generated Bazel target to be '%s', got '%s'",
						testCase.description,
						w,
						g,
					)
				}
			}
		}
	}
}
