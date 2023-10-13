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
	"testing"

	"android/soong/cc"
)

func runCcPrebuiltLibrarySharedTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Parallel()
	t.Helper()
	(&tc).ModuleTypeUnderTest = "cc_prebuilt_library_shared"
	(&tc).ModuleTypeUnderTestFactory = cc.PrebuiltSharedLibraryFactory
	RunBp2BuildTestCaseSimple(t, tc)
}

func TestPrebuiltLibrarySharedSimple(t *testing.T) {
	runCcPrebuiltLibrarySharedTestCase(t,
		Bp2buildTestCase{
			Description: "prebuilt library shared simple",
			Filesystem: map[string]string{
				"libf.so": "",
			},
			Blueprint: `
cc_prebuilt_library_shared {
	name: "libtest",
	srcs: ["libf.so"],
	bazel_module: { bp2build_available: true },
}`,
			ExpectedBazelTargets: []string{
				MakeBazelTarget("cc_prebuilt_library_shared", "libtest", AttrNameToString{
					"shared_library": `"libf.so"`,
				}),
			},
		})
}

func TestPrebuiltLibrarySharedWithArchVariance(t *testing.T) {
	runCcPrebuiltLibrarySharedTestCase(t,
		Bp2buildTestCase{
			Description: "prebuilt library shared with arch variance",
			Filesystem: map[string]string{
				"libf.so": "",
				"libg.so": "",
			},
			Blueprint: `
cc_prebuilt_library_shared {
	name: "libtest",
	arch: {
		arm64: { srcs: ["libf.so"], },
		arm: { srcs: ["libg.so"], },
	},
	bazel_module: { bp2build_available: true },
}`,
			ExpectedBazelTargets: []string{
				MakeBazelTarget("cc_prebuilt_library_shared", "libtest", AttrNameToString{
					"shared_library": `select({
        "//build/bazel_common_rules/platforms/arch:arm": "libg.so",
        "//build/bazel_common_rules/platforms/arch:arm64": "libf.so",
        "//conditions:default": None,
    })`,
				}),
			},
		})
}

func TestPrebuiltLibrarySharedAdditionalAttrs(t *testing.T) {
	runCcPrebuiltLibrarySharedTestCase(t,
		Bp2buildTestCase{
			Description: "prebuilt library shared additional attributes",
			Filesystem: map[string]string{
				"libf.so":             "",
				"testdir/1/include.h": "",
				"testdir/2/other.h":   "",
			},
			Blueprint: `
cc_prebuilt_library_shared {
	name: "libtest",
	srcs: ["libf.so"],
	export_include_dirs: ["testdir/1/"],
	export_system_include_dirs: ["testdir/2/"],
	bazel_module: { bp2build_available: true },
}`,
			ExpectedBazelTargets: []string{
				MakeBazelTarget("cc_prebuilt_library_shared", "libtest", AttrNameToString{
					"shared_library":         `"libf.so"`,
					"export_includes":        `["testdir/1/"]`,
					"export_system_includes": `["testdir/2/"]`,
				}),
			},
		})
}

func TestPrebuiltLibrarySharedWithExportIncludesArchVariant(t *testing.T) {
	runCcPrebuiltLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_prebuilt_library_shared correctly translates export_includes with arch variance",
		Filesystem: map[string]string{
			"libf.so": "",
			"libg.so": "",
		},
		Blueprint: `
cc_prebuilt_library_shared {
	name: "libtest",
	srcs: ["libf.so"],
	arch: {
		arm: { export_include_dirs: ["testdir/1/"], },
		arm64: { export_include_dirs: ["testdir/2/"], },
	},
	bazel_module: { bp2build_available: true },
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_prebuilt_library_shared", "libtest", AttrNameToString{
				"shared_library": `"libf.so"`,
				"export_includes": `select({
        "//build/bazel_common_rules/platforms/arch:arm": ["testdir/1/"],
        "//build/bazel_common_rules/platforms/arch:arm64": ["testdir/2/"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestPrebuiltLibrarySharedWithExportSystemIncludesArchVariant(t *testing.T) {
	runCcPrebuiltLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_prebuilt_library_shared correctly translates export_system_includes with arch variance",
		Filesystem: map[string]string{
			"libf.so": "",
			"libg.so": "",
		},
		Blueprint: `
cc_prebuilt_library_shared {
	name: "libtest",
	srcs: ["libf.so"],
	arch: {
		arm: { export_system_include_dirs: ["testdir/1/"], },
		arm64: { export_system_include_dirs: ["testdir/2/"], },
	},
	bazel_module: { bp2build_available: true },
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_prebuilt_library_shared", "libtest", AttrNameToString{
				"shared_library": `"libf.so"`,
				"export_system_includes": `select({
        "//build/bazel_common_rules/platforms/arch:arm": ["testdir/1/"],
        "//build/bazel_common_rules/platforms/arch:arm64": ["testdir/2/"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}
