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
	"android/soong/genrule"
	"fmt"

	"testing"
)

const (
	// See cc/testing.go for more context
	soongCcLibraryStaticPreamble = `
cc_defaults {
    name: "linux_bionic_supported",
}`
)

func TestCcLibraryStaticLoadStatement(t *testing.T) {
	testCases := []struct {
		bazelTargets           BazelTargets
		expectedLoadStatements string
	}{
		{
			bazelTargets: BazelTargets{
				BazelTarget{
					name:      "cc_library_static_target",
					ruleClass: "cc_library_static",
					// NOTE: No bzlLoadLocation for native rules
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

func registerCcLibraryStaticModuleTypes(ctx android.RegistrationContext) {
	cc.RegisterCCBuildComponents(ctx)
	ctx.RegisterModuleType("cc_library_headers", cc.LibraryHeaderFactory)
	ctx.RegisterModuleType("genrule", genrule.GenRuleFactory)
	// Required for system_shared_libs dependencies.
	ctx.RegisterModuleType("cc_library", cc.LibraryFactory)
}

func runCcLibraryStaticTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()

	(&tc).ModuleTypeUnderTest = "cc_library_static"
	(&tc).ModuleTypeUnderTestFactory = cc.LibraryStaticFactory
	RunBp2BuildTestCase(t, registerCcLibraryStaticModuleTypes, tc)
}

func TestCcLibraryStaticSimple(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static test",
		Filesystem: map[string]string{
			// NOTE: include_dir headers *should not* appear in Bazel hdrs later (?)
			"include_dir_1/include_dir_1_a.h": "",
			"include_dir_1/include_dir_1_b.h": "",
			"include_dir_2/include_dir_2_a.h": "",
			"include_dir_2/include_dir_2_b.h": "",
			// NOTE: local_include_dir headers *should not* appear in Bazel hdrs later (?)
			"local_include_dir_1/local_include_dir_1_a.h": "",
			"local_include_dir_1/local_include_dir_1_b.h": "",
			"local_include_dir_2/local_include_dir_2_a.h": "",
			"local_include_dir_2/local_include_dir_2_b.h": "",
			// NOTE: export_include_dir headers *should* appear in Bazel hdrs later
			"export_include_dir_1/export_include_dir_1_a.h": "",
			"export_include_dir_1/export_include_dir_1_b.h": "",
			"export_include_dir_2/export_include_dir_2_a.h": "",
			"export_include_dir_2/export_include_dir_2_b.h": "",
			// NOTE: Soong implicitly includes headers in the current directory
			"implicit_include_1.h": "",
			"implicit_include_2.h": "",
		},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_headers {
    name: "header_lib_1",
    export_include_dirs: ["header_lib_1"],
    bazel_module: { bp2build_available: false },
}

cc_library_headers {
    name: "header_lib_2",
    export_include_dirs: ["header_lib_2"],
    bazel_module: { bp2build_available: false },
}

cc_library_static {
    name: "static_lib_1",
    srcs: ["static_lib_1.cc"],
    bazel_module: { bp2build_available: false },
}

cc_library_static {
    name: "static_lib_2",
    srcs: ["static_lib_2.cc"],
    bazel_module: { bp2build_available: false },
}

cc_library_static {
    name: "whole_static_lib_1",
    srcs: ["whole_static_lib_1.cc"],
    bazel_module: { bp2build_available: false },
}

cc_library_static {
    name: "whole_static_lib_2",
    srcs: ["whole_static_lib_2.cc"],
    bazel_module: { bp2build_available: false },
}

cc_library_static {
    name: "foo_static",
    srcs: [
        "foo_static1.cc",
        "foo_static2.cc",
    ],
    cflags: [
        "-Dflag1",
        "-Dflag2"
    ],
    static_libs: [
        "static_lib_1",
        "static_lib_2"
    ],
    whole_static_libs: [
        "whole_static_lib_1",
        "whole_static_lib_2"
    ],
    include_dirs: [
        "include_dir_1",
        "include_dir_2",
    ],
    local_include_dirs: [
        "local_include_dir_1",
        "local_include_dir_2",
    ],
    export_include_dirs: [
        "export_include_dir_1",
        "export_include_dir_2"
    ],
    header_libs: [
        "header_lib_1",
        "header_lib_2"
    ],
    sdk_version: "current",
    min_sdk_version: "29",

    // TODO: Also support export_header_lib_headers
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"absolute_includes": `[
        "include_dir_1",
        "include_dir_2",
    ]`,
				"copts": `[
        "-Dflag1",
        "-Dflag2",
    ]`,
				"export_includes": `[
        "export_include_dir_1",
        "export_include_dir_2",
    ]`,
				"implementation_deps": `[
        ":header_lib_1",
        ":header_lib_2",
        ":static_lib_1",
        ":static_lib_2",
    ]`,
				"local_includes": `[
        "local_include_dir_1",
        "local_include_dir_2",
        ".",
    ]`,
				"srcs": `[
        "foo_static1.cc",
        "foo_static2.cc",
    ]`,
				"whole_archive_deps": `[
        ":whole_static_lib_1",
        ":whole_static_lib_2",
    ]`,
				"sdk_version":     `"current"`,
				"min_sdk_version": `"29"`,
			}),
		},
	})
}

func TestCcLibraryStaticSubpackage(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static subpackage test",
		Filesystem: map[string]string{
			// subpackage with subdirectory
			"subpackage/Android.bp":                         "",
			"subpackage/subpackage_header.h":                "",
			"subpackage/subdirectory/subdirectory_header.h": "",
			// subsubpackage with subdirectory
			"subpackage/subsubpackage/Android.bp":                         "",
			"subpackage/subsubpackage/subsubpackage_header.h":             "",
			"subpackage/subsubpackage/subdirectory/subdirectory_header.h": "",
			// subsubsubpackage with subdirectory
			"subpackage/subsubpackage/subsubsubpackage/Android.bp":                         "",
			"subpackage/subsubpackage/subsubsubpackage/subsubsubpackage_header.h":          "",
			"subpackage/subsubpackage/subsubsubpackage/subdirectory/subdirectory_header.h": "",
		},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    srcs: [],
    include_dirs: [
        "subpackage",
    ],
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"absolute_includes": `["subpackage"]`,
				"local_includes":    `["."]`,
			}),
		},
	})
}

func TestCcLibraryStaticExportIncludeDir(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static export include dir",
		Filesystem: map[string]string{
			// subpackage with subdirectory
			"subpackage/Android.bp":                         "",
			"subpackage/subpackage_header.h":                "",
			"subpackage/subdirectory/subdirectory_header.h": "",
		},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    export_include_dirs: ["subpackage"],
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"export_includes": `["subpackage"]`,
			}),
		},
	})
}

func TestCcLibraryStaticExportSystemIncludeDir(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static export system include dir",
		Filesystem: map[string]string{
			// subpackage with subdirectory
			"subpackage/Android.bp":                         "",
			"subpackage/subpackage_header.h":                "",
			"subpackage/subdirectory/subdirectory_header.h": "",
		},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    export_system_include_dirs: ["subpackage"],
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"export_system_includes": `["subpackage"]`,
			}),
		},
	})
}

func TestCcLibraryStaticManyIncludeDirs(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static include_dirs, local_include_dirs, export_include_dirs (b/183742505)",
		Dir:         "subpackage",
		Filesystem: map[string]string{
			// subpackage with subdirectory
			"subpackage/Android.bp": `
cc_library_static {
    name: "foo_static",
    // include_dirs are workspace/root relative
    include_dirs: [
        "subpackage/subsubpackage",
        "subpackage2",
        "subpackage3/subsubpackage"
    ],
    local_include_dirs: ["subsubpackage2"], // module dir relative
    export_include_dirs: ["./exported_subsubpackage"], // module dir relative
    include_build_directory: true,
    bazel_module: { bp2build_available: true },
}`,
			"subpackage/subsubpackage/header.h":          "",
			"subpackage/subsubpackage2/header.h":         "",
			"subpackage/exported_subsubpackage/header.h": "",
			"subpackage2/header.h":                       "",
			"subpackage3/subsubpackage/header.h":         "",
		},
		Blueprint: soongCcLibraryStaticPreamble,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"absolute_includes": `[
        "subpackage/subsubpackage",
        "subpackage2",
        "subpackage3/subsubpackage",
    ]`,
				"export_includes": `["./exported_subsubpackage"]`,
				"local_includes": `[
        "subsubpackage2",
        ".",
    ]`,
			})},
	})
}

func TestCcLibraryStaticIncludeBuildDirectoryDisabled(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static include_build_directory disabled",
		Filesystem: map[string]string{
			// subpackage with subdirectory
			"subpackage/Android.bp":                         "",
			"subpackage/subpackage_header.h":                "",
			"subpackage/subdirectory/subdirectory_header.h": "",
		},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    include_dirs: ["subpackage"], // still used, but local_include_dirs is recommended
    local_include_dirs: ["subpackage2"],
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"absolute_includes": `["subpackage"]`,
				"local_includes":    `["subpackage2"]`,
			}),
		},
	})
}

func TestCcLibraryStaticIncludeBuildDirectoryEnabled(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static include_build_directory enabled",
		Filesystem: map[string]string{
			// subpackage with subdirectory
			"subpackage/Android.bp":                         "",
			"subpackage/subpackage_header.h":                "",
			"subpackage2/Android.bp":                        "",
			"subpackage2/subpackage2_header.h":              "",
			"subpackage/subdirectory/subdirectory_header.h": "",
		},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    include_dirs: ["subpackage"], // still used, but local_include_dirs is recommended
    local_include_dirs: ["subpackage2"],
    include_build_directory: true,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"absolute_includes": `["subpackage"]`,
				"local_includes": `[
        "subpackage2",
        ".",
    ]`,
			}),
		},
	})
}

func TestCcLibraryStaticArchSpecificStaticLib(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static arch-specific static_libs",
		Filesystem:  map[string]string{},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "static_dep",
    bazel_module: { bp2build_available: false },
}
cc_library_static {
    name: "static_dep2",
    bazel_module: { bp2build_available: false },
}
cc_library_static {
    name: "foo_static",
    arch: { arm64: { static_libs: ["static_dep"], whole_static_libs: ["static_dep2"] } },
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"implementation_deps": `select({
        "//build/bazel/platforms/arch:arm64": [":static_dep"],
        "//conditions:default": [],
    })`,
				"whole_archive_deps": `select({
        "//build/bazel/platforms/arch:arm64": [":static_dep2"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestCcLibraryStaticOsSpecificStaticLib(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static os-specific static_libs",
		Filesystem:  map[string]string{},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "static_dep",
    bazel_module: { bp2build_available: false },
}
cc_library_static {
    name: "static_dep2",
    bazel_module: { bp2build_available: false },
}
cc_library_static {
    name: "foo_static",
    target: { android: { static_libs: ["static_dep"], whole_static_libs: ["static_dep2"] } },
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"implementation_deps": `select({
        "//build/bazel/platforms/os:android": [":static_dep"],
        "//conditions:default": [],
    })`,
				"whole_archive_deps": `select({
        "//build/bazel/platforms/os:android": [":static_dep2"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestCcLibraryStaticBaseArchOsSpecificStaticLib(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static base, arch and os-specific static_libs",
		Filesystem:  map[string]string{},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "static_dep",
    bazel_module: { bp2build_available: false },
}
cc_library_static {
    name: "static_dep2",
    bazel_module: { bp2build_available: false },
}
cc_library_static {
    name: "static_dep3",
    bazel_module: { bp2build_available: false },
}
cc_library_static {
    name: "static_dep4",
    bazel_module: { bp2build_available: false },
}
cc_library_static {
    name: "foo_static",
    static_libs: ["static_dep"],
    whole_static_libs: ["static_dep2"],
    target: { android: { static_libs: ["static_dep3"] } },
    arch: { arm64: { static_libs: ["static_dep4"] } },
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"implementation_deps": `[":static_dep"] + select({
        "//build/bazel/platforms/arch:arm64": [":static_dep4"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel/platforms/os:android": [":static_dep3"],
        "//conditions:default": [],
    })`,
				"whole_archive_deps": `[":static_dep2"]`,
			}),
		},
	})
}

func TestCcLibraryStaticSimpleExcludeSrcs(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static simple exclude_srcs",
		Filesystem: map[string]string{
			"common.c":       "",
			"foo-a.c":        "",
			"foo-excluded.c": "",
		},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    srcs: ["common.c", "foo-*.c"],
    exclude_srcs: ["foo-excluded.c"],
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"srcs_c": `[
        "common.c",
        "foo-a.c",
    ]`,
			}),
		},
	})
}

func TestCcLibraryStaticOneArchSrcs(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static one arch specific srcs",
		Filesystem: map[string]string{
			"common.c":  "",
			"foo-arm.c": "",
		},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    srcs: ["common.c"],
    arch: { arm: { srcs: ["foo-arm.c"] } },
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"srcs_c": `["common.c"] + select({
        "//build/bazel/platforms/arch:arm": ["foo-arm.c"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestCcLibraryStaticOneArchSrcsExcludeSrcs(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static one arch specific srcs and exclude_srcs",
		Filesystem: map[string]string{
			"common.c":           "",
			"for-arm.c":          "",
			"not-for-arm.c":      "",
			"not-for-anything.c": "",
		},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    srcs: ["common.c", "not-for-*.c"],
    exclude_srcs: ["not-for-anything.c"],
    arch: {
        arm: { srcs: ["for-arm.c"], exclude_srcs: ["not-for-arm.c"] },
    },
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"srcs_c": `["common.c"] + select({
        "//build/bazel/platforms/arch:arm": ["for-arm.c"],
        "//conditions:default": ["not-for-arm.c"],
    })`,
			}),
		},
	})
}

func TestCcLibraryStaticTwoArchExcludeSrcs(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static arch specific exclude_srcs for 2 architectures",
		Filesystem: map[string]string{
			"common.c":      "",
			"for-arm.c":     "",
			"for-x86.c":     "",
			"not-for-arm.c": "",
			"not-for-x86.c": "",
		},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    srcs: ["common.c", "not-for-*.c"],
    exclude_srcs: ["not-for-everything.c"],
    arch: {
        arm: { srcs: ["for-arm.c"], exclude_srcs: ["not-for-arm.c"] },
        x86: { srcs: ["for-x86.c"], exclude_srcs: ["not-for-x86.c"] },
    },
    include_build_directory: false,
} `,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"srcs_c": `["common.c"] + select({
        "//build/bazel/platforms/arch:arm": [
            "not-for-x86.c",
            "for-arm.c",
        ],
        "//build/bazel/platforms/arch:x86": [
            "not-for-arm.c",
            "for-x86.c",
        ],
        "//conditions:default": [
            "not-for-arm.c",
            "not-for-x86.c",
        ],
    })`,
			}),
		},
	})
}

func TestCcLibraryStaticFourArchExcludeSrcs(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static arch specific exclude_srcs for 4 architectures",
		Filesystem: map[string]string{
			"common.c":             "",
			"for-arm.c":            "",
			"for-arm64.c":          "",
			"for-x86.c":            "",
			"for-x86_64.c":         "",
			"not-for-arm.c":        "",
			"not-for-arm64.c":      "",
			"not-for-x86.c":        "",
			"not-for-x86_64.c":     "",
			"not-for-everything.c": "",
		},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    srcs: ["common.c", "not-for-*.c"],
    exclude_srcs: ["not-for-everything.c"],
    arch: {
        arm: { srcs: ["for-arm.c"], exclude_srcs: ["not-for-arm.c"] },
        arm64: { srcs: ["for-arm64.c"], exclude_srcs: ["not-for-arm64.c"] },
        x86: { srcs: ["for-x86.c"], exclude_srcs: ["not-for-x86.c"] },
        x86_64: { srcs: ["for-x86_64.c"], exclude_srcs: ["not-for-x86_64.c"] },
  },
    include_build_directory: false,
} `,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"srcs_c": `["common.c"] + select({
        "//build/bazel/platforms/arch:arm": [
            "not-for-arm64.c",
            "not-for-x86.c",
            "not-for-x86_64.c",
            "for-arm.c",
        ],
        "//build/bazel/platforms/arch:arm64": [
            "not-for-arm.c",
            "not-for-x86.c",
            "not-for-x86_64.c",
            "for-arm64.c",
        ],
        "//build/bazel/platforms/arch:x86": [
            "not-for-arm.c",
            "not-for-arm64.c",
            "not-for-x86_64.c",
            "for-x86.c",
        ],
        "//build/bazel/platforms/arch:x86_64": [
            "not-for-arm.c",
            "not-for-arm64.c",
            "not-for-x86.c",
            "for-x86_64.c",
        ],
        "//conditions:default": [
            "not-for-arm.c",
            "not-for-arm64.c",
            "not-for-x86.c",
            "not-for-x86_64.c",
        ],
    })`,
			}),
		},
	})
}

func TestCcLibraryStaticOneArchEmpty(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static one arch empty",
		Filesystem: map[string]string{
			"common.cc":       "",
			"foo-no-arm.cc":   "",
			"foo-excluded.cc": "",
		},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    srcs: ["common.cc", "foo-*.cc"],
    exclude_srcs: ["foo-excluded.cc"],
    arch: {
        arm: { exclude_srcs: ["foo-no-arm.cc"] },
    },
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"srcs": `["common.cc"] + select({
        "//build/bazel/platforms/arch:arm": [],
        "//conditions:default": ["foo-no-arm.cc"],
    })`,
			}),
		},
	})
}

func TestCcLibraryStaticOneArchEmptyOtherSet(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static one arch empty other set",
		Filesystem: map[string]string{
			"common.cc":       "",
			"foo-no-arm.cc":   "",
			"x86-only.cc":     "",
			"foo-excluded.cc": "",
		},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    srcs: ["common.cc", "foo-*.cc"],
    exclude_srcs: ["foo-excluded.cc"],
    arch: {
        arm: { exclude_srcs: ["foo-no-arm.cc"] },
        x86: { srcs: ["x86-only.cc"] },
    },
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"srcs": `["common.cc"] + select({
        "//build/bazel/platforms/arch:arm": [],
        "//build/bazel/platforms/arch:x86": [
            "foo-no-arm.cc",
            "x86-only.cc",
        ],
        "//conditions:default": ["foo-no-arm.cc"],
    })`,
			}),
		},
	})
}

func TestCcLibraryStaticMultipleDepSameName(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static multiple dep same name panic",
		Filesystem:  map[string]string{},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "static_dep",
    bazel_module: { bp2build_available: false },
}
cc_library_static {
    name: "foo_static",
    static_libs: ["static_dep", "static_dep"],
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"implementation_deps": `[":static_dep"]`,
			}),
		},
	})
}

func TestCcLibraryStaticOneMultilibSrcsExcludeSrcs(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static 1 multilib srcs and exclude_srcs",
		Filesystem: map[string]string{
			"common.c":        "",
			"for-lib32.c":     "",
			"not-for-lib32.c": "",
		},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    srcs: ["common.c", "not-for-*.c"],
    multilib: {
        lib32: { srcs: ["for-lib32.c"], exclude_srcs: ["not-for-lib32.c"] },
    },
    include_build_directory: false,
} `,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"srcs_c": `["common.c"] + select({
        "//build/bazel/platforms/arch:arm": ["for-lib32.c"],
        "//build/bazel/platforms/arch:x86": ["for-lib32.c"],
        "//conditions:default": ["not-for-lib32.c"],
    })`,
			}),
		},
	})
}

func TestCcLibraryStaticTwoMultilibSrcsExcludeSrcs(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static 2 multilib srcs and exclude_srcs",
		Filesystem: map[string]string{
			"common.c":        "",
			"for-lib32.c":     "",
			"for-lib64.c":     "",
			"not-for-lib32.c": "",
			"not-for-lib64.c": "",
		},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    srcs: ["common.c", "not-for-*.c"],
    multilib: {
        lib32: { srcs: ["for-lib32.c"], exclude_srcs: ["not-for-lib32.c"] },
        lib64: { srcs: ["for-lib64.c"], exclude_srcs: ["not-for-lib64.c"] },
    },
    include_build_directory: false,
} `,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"srcs_c": `["common.c"] + select({
        "//build/bazel/platforms/arch:arm": [
            "not-for-lib64.c",
            "for-lib32.c",
        ],
        "//build/bazel/platforms/arch:arm64": [
            "not-for-lib32.c",
            "for-lib64.c",
        ],
        "//build/bazel/platforms/arch:x86": [
            "not-for-lib64.c",
            "for-lib32.c",
        ],
        "//build/bazel/platforms/arch:x86_64": [
            "not-for-lib32.c",
            "for-lib64.c",
        ],
        "//conditions:default": [
            "not-for-lib32.c",
            "not-for-lib64.c",
        ],
    })`,
			}),
		},
	})
}

func TestCcLibrarySTaticArchMultilibSrcsExcludeSrcs(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static arch and multilib srcs and exclude_srcs",
		Filesystem: map[string]string{
			"common.c":             "",
			"for-arm.c":            "",
			"for-arm64.c":          "",
			"for-x86.c":            "",
			"for-x86_64.c":         "",
			"for-lib32.c":          "",
			"for-lib64.c":          "",
			"not-for-arm.c":        "",
			"not-for-arm64.c":      "",
			"not-for-x86.c":        "",
			"not-for-x86_64.c":     "",
			"not-for-lib32.c":      "",
			"not-for-lib64.c":      "",
			"not-for-everything.c": "",
		},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
   name: "foo_static",
   srcs: ["common.c", "not-for-*.c"],
   exclude_srcs: ["not-for-everything.c"],
   arch: {
       arm: { srcs: ["for-arm.c"], exclude_srcs: ["not-for-arm.c"] },
       arm64: { srcs: ["for-arm64.c"], exclude_srcs: ["not-for-arm64.c"] },
       x86: { srcs: ["for-x86.c"], exclude_srcs: ["not-for-x86.c"] },
       x86_64: { srcs: ["for-x86_64.c"], exclude_srcs: ["not-for-x86_64.c"] },
   },
   multilib: {
       lib32: { srcs: ["for-lib32.c"], exclude_srcs: ["not-for-lib32.c"] },
       lib64: { srcs: ["for-lib64.c"], exclude_srcs: ["not-for-lib64.c"] },
   },
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"srcs_c": `["common.c"] + select({
        "//build/bazel/platforms/arch:arm": [
            "not-for-arm64.c",
            "not-for-lib64.c",
            "not-for-x86.c",
            "not-for-x86_64.c",
            "for-arm.c",
            "for-lib32.c",
        ],
        "//build/bazel/platforms/arch:arm64": [
            "not-for-arm.c",
            "not-for-lib32.c",
            "not-for-x86.c",
            "not-for-x86_64.c",
            "for-arm64.c",
            "for-lib64.c",
        ],
        "//build/bazel/platforms/arch:x86": [
            "not-for-arm.c",
            "not-for-arm64.c",
            "not-for-lib64.c",
            "not-for-x86_64.c",
            "for-x86.c",
            "for-lib32.c",
        ],
        "//build/bazel/platforms/arch:x86_64": [
            "not-for-arm.c",
            "not-for-arm64.c",
            "not-for-lib32.c",
            "not-for-x86.c",
            "for-x86_64.c",
            "for-lib64.c",
        ],
        "//conditions:default": [
            "not-for-arm.c",
            "not-for-arm64.c",
            "not-for-lib32.c",
            "not-for-lib64.c",
            "not-for-x86.c",
            "not-for-x86_64.c",
        ],
    })`,
			}),
		},
	})
}

func TestCcLibraryStaticGeneratedHeadersAllPartitions(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Blueprint: soongCcLibraryStaticPreamble + `
genrule {
    name: "generated_hdr",
    cmd: "nothing to see here",
    bazel_module: { bp2build_available: false },
}

genrule {
    name: "export_generated_hdr",
    cmd: "nothing to see here",
    bazel_module: { bp2build_available: false },
}

cc_library_static {
    name: "foo_static",
    srcs: ["cpp_src.cpp", "as_src.S", "c_src.c"],
    generated_headers: ["generated_hdr", "export_generated_hdr"],
    export_generated_headers: ["export_generated_hdr"],
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"export_includes": `["."]`,
				"local_includes":  `["."]`,
				"hdrs":            `[":export_generated_hdr"]`,
				"srcs": `[
        "cpp_src.cpp",
        ":generated_hdr",
    ]`,
				"srcs_as": `[
        "as_src.S",
        ":generated_hdr",
    ]`,
				"srcs_c": `[
        "c_src.c",
        ":generated_hdr",
    ]`,
			}),
		},
	})
}

func TestCcLibraryStaticArchSrcsExcludeSrcsGeneratedFiles(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static arch srcs/exclude_srcs with generated files",
		Filesystem: map[string]string{
			"common.cpp":             "",
			"for-x86.cpp":            "",
			"not-for-x86.cpp":        "",
			"not-for-everything.cpp": "",
			"dep/Android.bp": simpleModuleDoNotConvertBp2build("genrule", "generated_src_other_pkg") +
				simpleModuleDoNotConvertBp2build("genrule", "generated_hdr_other_pkg") +
				simpleModuleDoNotConvertBp2build("genrule", "generated_src_other_pkg_x86") +
				simpleModuleDoNotConvertBp2build("genrule", "generated_hdr_other_pkg_x86") +
				simpleModuleDoNotConvertBp2build("genrule", "generated_hdr_other_pkg_android"),
		},
		Blueprint: soongCcLibraryStaticPreamble +
			simpleModuleDoNotConvertBp2build("genrule", "generated_src") +
			simpleModuleDoNotConvertBp2build("genrule", "generated_src_not_x86") +
			simpleModuleDoNotConvertBp2build("genrule", "generated_src_android") +
			simpleModuleDoNotConvertBp2build("genrule", "generated_hdr") + `
cc_library_static {
    name: "foo_static",
    srcs: ["common.cpp", "not-for-*.cpp"],
    exclude_srcs: ["not-for-everything.cpp"],
    generated_sources: ["generated_src", "generated_src_other_pkg", "generated_src_not_x86"],
    generated_headers: ["generated_hdr", "generated_hdr_other_pkg"],
    export_generated_headers: ["generated_hdr_other_pkg"],
    arch: {
        x86: {
          srcs: ["for-x86.cpp"],
          exclude_srcs: ["not-for-x86.cpp"],
          generated_headers: ["generated_hdr_other_pkg_x86"],
          exclude_generated_sources: ["generated_src_not_x86"],
    export_generated_headers: ["generated_hdr_other_pkg_x86"],
        },
    },
    target: {
        android: {
            generated_sources: ["generated_src_android"],
            generated_headers: ["generated_hdr_other_pkg_android"],
    export_generated_headers: ["generated_hdr_other_pkg_android"],
        },
    },

    include_build_directory: false,
}
`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"srcs": `[
        "common.cpp",
        ":generated_src",
        "//dep:generated_src_other_pkg",
        ":generated_hdr",
    ] + select({
        "//build/bazel/platforms/arch:x86": ["for-x86.cpp"],
        "//conditions:default": [
            "not-for-x86.cpp",
            ":generated_src_not_x86",
        ],
    }) + select({
        "//build/bazel/platforms/os:android": [":generated_src_android"],
        "//conditions:default": [],
    })`,
				"hdrs": `["//dep:generated_hdr_other_pkg"] + select({
        "//build/bazel/platforms/arch:x86": ["//dep:generated_hdr_other_pkg_x86"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel/platforms/os:android": ["//dep:generated_hdr_other_pkg_android"],
        "//conditions:default": [],
    })`,
				"local_includes":           `["."]`,
				"export_absolute_includes": `["dep"]`,
			}),
		},
	})
}

func TestCcLibraryStaticGetTargetProperties(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{

		Description: "cc_library_static complex GetTargetProperties",
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    target: {
        android: {
            srcs: ["android_src.c"],
        },
        android_arm: {
            srcs: ["android_arm_src.c"],
        },
        android_arm64: {
            srcs: ["android_arm64_src.c"],
        },
        android_x86: {
            srcs: ["android_x86_src.c"],
        },
        android_x86_64: {
            srcs: ["android_x86_64_src.c"],
        },
        linux_bionic_arm64: {
            srcs: ["linux_bionic_arm64_src.c"],
        },
        linux_bionic_x86_64: {
            srcs: ["linux_bionic_x86_64_src.c"],
        },
    },
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"srcs_c": `select({
        "//build/bazel/platforms/os:android": ["android_src.c"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel/platforms/os_arch:android_arm": ["android_arm_src.c"],
        "//build/bazel/platforms/os_arch:android_arm64": ["android_arm64_src.c"],
        "//build/bazel/platforms/os_arch:android_x86": ["android_x86_src.c"],
        "//build/bazel/platforms/os_arch:android_x86_64": ["android_x86_64_src.c"],
        "//build/bazel/platforms/os_arch:linux_bionic_arm64": ["linux_bionic_arm64_src.c"],
        "//build/bazel/platforms/os_arch:linux_bionic_x86_64": ["linux_bionic_x86_64_src.c"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestCcLibraryStaticProductVariableSelects(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static product variable selects",
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    srcs: ["common.c"],
    product_variables: {
      malloc_not_svelte: {
        cflags: ["-Wmalloc_not_svelte"],
      },
      malloc_zero_contents: {
        cflags: ["-Wmalloc_zero_contents"],
      },
      binder32bit: {
        cflags: ["-Wbinder32bit"],
      },
    },
    include_build_directory: false,
} `,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"copts": `select({
        "//build/bazel/product_variables:binder32bit": ["-Wbinder32bit"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel/product_variables:malloc_not_svelte": ["-Wmalloc_not_svelte"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel/product_variables:malloc_zero_contents": ["-Wmalloc_zero_contents"],
        "//conditions:default": [],
    })`,
				"srcs_c": `["common.c"]`,
			}),
		},
	})
}

func TestCcLibraryStaticProductVariableArchSpecificSelects(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static arch-specific product variable selects",
		Filesystem:  map[string]string{},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    srcs: ["common.c"],
    product_variables: {
      malloc_not_svelte: {
        cflags: ["-Wmalloc_not_svelte"],
      },
    },
    arch: {
        arm64: {
            product_variables: {
                malloc_not_svelte: {
                    cflags: ["-Warm64_malloc_not_svelte"],
                },
            },
        },
    },
    multilib: {
        lib32: {
            product_variables: {
                malloc_not_svelte: {
                    cflags: ["-Wlib32_malloc_not_svelte"],
                },
            },
        },
    },
    target: {
        android: {
            product_variables: {
                malloc_not_svelte: {
                    cflags: ["-Wandroid_malloc_not_svelte"],
                },
            },
        }
    },
    include_build_directory: false,
} `,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"copts": `select({
        "//build/bazel/product_variables:malloc_not_svelte": ["-Wmalloc_not_svelte"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel/product_variables:malloc_not_svelte-android": ["-Wandroid_malloc_not_svelte"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel/product_variables:malloc_not_svelte-arm": ["-Wlib32_malloc_not_svelte"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel/product_variables:malloc_not_svelte-arm64": ["-Warm64_malloc_not_svelte"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel/product_variables:malloc_not_svelte-x86": ["-Wlib32_malloc_not_svelte"],
        "//conditions:default": [],
    })`,
				"srcs_c": `["common.c"]`,
			}),
		},
	})
}

func TestCcLibraryStaticProductVariableStringReplacement(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static product variable string replacement",
		Filesystem:  map[string]string{},
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "foo_static",
    srcs: ["common.S"],
    product_variables: {
      platform_sdk_version: {
          asflags: ["-DPLATFORM_SDK_VERSION=%d"],
      },
    },
    include_build_directory: false,
} `,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_static", AttrNameToString{
				"asflags": `select({
        "//build/bazel/product_variables:platform_sdk_version": ["-DPLATFORM_SDK_VERSION=$(Platform_sdk_version)"],
        "//conditions:default": [],
    })`,
				"srcs_as": `["common.S"]`,
			}),
		},
	})
}

func TestStaticLibrary_SystemSharedLibsRootEmpty(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static system_shared_lib empty root",
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "root_empty",
    system_shared_libs: [],
    include_build_directory: false,
}
`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "root_empty", AttrNameToString{
				"system_dynamic_deps": `[]`,
			}),
		},
	})
}

func TestStaticLibrary_SystemSharedLibsStaticEmpty(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static system_shared_lib empty static default",
		Blueprint: soongCcLibraryStaticPreamble + `
cc_defaults {
    name: "static_empty_defaults",
    static: {
        system_shared_libs: [],
    },
    include_build_directory: false,
}
cc_library_static {
    name: "static_empty",
    defaults: ["static_empty_defaults"],
}
`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "static_empty", AttrNameToString{
				"system_dynamic_deps": `[]`,
			}),
		},
	})
}

func TestStaticLibrary_SystemSharedLibsBionicEmpty(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static system_shared_lib empty for bionic variant",
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "target_bionic_empty",
    target: {
        bionic: {
            system_shared_libs: [],
        },
    },
    include_build_directory: false,
}
`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "target_bionic_empty", AttrNameToString{
				"system_dynamic_deps": `[]`,
			}),
		},
	})
}

func TestStaticLibrary_SystemSharedLibsLinuxBionicEmpty(t *testing.T) {
	// Note that this behavior is technically incorrect (it's a simplification).
	// The correct behavior would be if bp2build wrote `system_dynamic_deps = []`
	// only for linux_bionic, but `android` had `["libc", "libdl", "libm"].
	// b/195791252 tracks the fix.
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static system_shared_lib empty for linux_bionic variant",
		Blueprint: soongCcLibraryStaticPreamble + `
cc_library_static {
    name: "target_linux_bionic_empty",
    target: {
        linux_bionic: {
            system_shared_libs: [],
        },
    },
    include_build_directory: false,
}
`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "target_linux_bionic_empty", AttrNameToString{
				"system_dynamic_deps": `[]`,
			}),
		},
	})
}

func TestStaticLibrary_SystemSharedLibsBionic(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static system_shared_libs set for bionic variant",
		Blueprint: soongCcLibraryStaticPreamble +
			simpleModuleDoNotConvertBp2build("cc_library", "libc") + `
cc_library_static {
    name: "target_bionic",
    target: {
        bionic: {
            system_shared_libs: ["libc"],
        },
    },
    include_build_directory: false,
}
`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "target_bionic", AttrNameToString{
				"system_dynamic_deps": `select({
        "//build/bazel/platforms/os:android": [":libc"],
        "//build/bazel/platforms/os:linux_bionic": [":libc"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestStaticLibrary_SystemSharedLibsLinuxRootAndLinuxBionic(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static system_shared_libs set for root and linux_bionic variant",
		Blueprint: soongCcLibraryStaticPreamble +
			simpleModuleDoNotConvertBp2build("cc_library", "libc") +
			simpleModuleDoNotConvertBp2build("cc_library", "libm") + `
cc_library_static {
    name: "target_linux_bionic",
    system_shared_libs: ["libc"],
    target: {
        linux_bionic: {
            system_shared_libs: ["libm"],
        },
    },
    include_build_directory: false,
}
`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "target_linux_bionic", AttrNameToString{
				"system_dynamic_deps": `[":libc"] + select({
        "//build/bazel/platforms/os:linux_bionic": [":libm"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestCcLibrarystatic_SystemSharedLibUsedAsDep(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Description: "cc_library_static system_shared_lib empty for linux_bionic variant",
		Blueprint: soongCcLibraryStaticPreamble +
			simpleModuleDoNotConvertBp2build("cc_library", "libc") + `
cc_library_static {
    name: "used_in_bionic_oses",
    target: {
        android: {
            shared_libs: ["libc"],
        },
        linux_bionic: {
            shared_libs: ["libc"],
        },
    },
    include_build_directory: false,
}

cc_library_static {
    name: "all",
    shared_libs: ["libc"],
    include_build_directory: false,
}

cc_library_static {
    name: "keep_for_empty_system_shared_libs",
    shared_libs: ["libc"],
		system_shared_libs: [],
    include_build_directory: false,
}
`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "all", AttrNameToString{
				"implementation_dynamic_deps": `select({
        "//build/bazel/platforms/os:android": [],
        "//build/bazel/platforms/os:linux_bionic": [],
        "//conditions:default": [":libc"],
    })`,
			}),
			makeBazelTarget("cc_library_static", "keep_for_empty_system_shared_libs", AttrNameToString{
				"implementation_dynamic_deps": `[":libc"]`,
				"system_dynamic_deps":         `[]`,
			}),
			makeBazelTarget("cc_library_static", "used_in_bionic_oses", AttrNameToString{}),
		},
	})
}

func TestCcLibraryStaticProto(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Blueprint: soongCcProtoPreamble + `cc_library_static {
	name: "foo",
	srcs: ["foo.proto"],
	proto: {
		export_proto_headers: true,
	},
	include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("proto_library", "foo_proto", AttrNameToString{
				"srcs": `["foo.proto"]`,
			}), makeBazelTarget("cc_lite_proto_library", "foo_cc_proto_lite", AttrNameToString{
				"deps": `[":foo_proto"]`,
			}), makeBazelTarget("cc_library_static", "foo", AttrNameToString{
				"deps":               `[":libprotobuf-cpp-lite"]`,
				"whole_archive_deps": `[":foo_cc_proto_lite"]`,
			}),
		},
	})
}

func TestCcLibraryStaticUseVersionLib(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Blueprint: soongCcProtoPreamble + `cc_library_static {
	name: "foo",
	use_version_lib: true,
	include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo", AttrNameToString{
				"use_version_lib": "True",
			}),
		},
	})
}

func TestCcLibraryStaticStdInFlags(t *testing.T) {
	runCcLibraryStaticTestCase(t, Bp2buildTestCase{
		Blueprint: soongCcProtoPreamble + `cc_library_static {
	name: "foo",
	cflags: ["-std=candcpp"],
	conlyflags: ["-std=conly"],
	cppflags: ["-std=cpp"],
	include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo", AttrNameToString{
				"conlyflags": `["-std=conly"]`,
				"cppflags":   `["-std=cpp"]`,
			}),
		},
	})
}

func TestCcLibraryStaticStl(t *testing.T) {
	testCases := []struct {
		desc string
		prop string
		attr AttrNameToString
	}{
		{
			desc: "c++_shared deduped to libc++",
			prop: `stl: "c++_shared",`,
			attr: AttrNameToString{
				"stl": `"libc++"`,
			},
		},
		{
			desc: "libc++ to libc++",
			prop: `stl: "libc++",`,
			attr: AttrNameToString{
				"stl": `"libc++"`,
			},
		},
		{
			desc: "c++_static to libc++_static",
			prop: `stl: "c++_static",`,
			attr: AttrNameToString{
				"stl": `"libc++_static"`,
			},
		},
		{
			desc: "libc++_static to libc++_static",
			prop: `stl: "libc++_static",`,
			attr: AttrNameToString{
				"stl": `"libc++_static"`,
			},
		},
		{
			desc: "system to system",
			prop: `stl: "system",`,
			attr: AttrNameToString{
				"stl": `"system"`,
			},
		},
		{
			desc: "none to none",
			prop: `stl: "none",`,
			attr: AttrNameToString{
				"stl": `"none"`,
			},
		},
		{
			desc: "empty to empty",
			attr: AttrNameToString{},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(*testing.T) {
			runCcLibraryStaticTestCase(t, Bp2buildTestCase{
				Blueprint: fmt.Sprintf(`cc_library_static {
	name: "foo",
	include_build_directory: false,
	%s
}`, tc.prop),
				ExpectedBazelTargets: []string{
					makeBazelTarget("cc_library_static", "foo", tc.attr),
				},
			})
		})
	}
}
