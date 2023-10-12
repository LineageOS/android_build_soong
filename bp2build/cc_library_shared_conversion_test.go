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
	"testing"

	"android/soong/android"
	"android/soong/cc"
)

const (
	// See cc/testing.go for more context
	// TODO(alexmarquez): Split out the preamble into common code?
	soongCcLibrarySharedPreamble = soongCcLibraryStaticPreamble
)

func registerCcLibrarySharedModuleTypes(ctx android.RegistrationContext) {
	cc.RegisterCCBuildComponents(ctx)
	ctx.RegisterModuleType("cc_library_headers", cc.LibraryHeaderFactory)
	ctx.RegisterModuleType("cc_library_static", cc.LibraryStaticFactory)
	ctx.RegisterModuleType("cc_library", cc.LibraryFactory)
	ctx.RegisterModuleType("ndk_library", cc.NdkLibraryFactory)
}

func runCcLibrarySharedTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	t.Parallel()
	tc.StubbedBuildDefinitions = append(tc.StubbedBuildDefinitions, "libbuildversion", "libprotobuf-cpp-lite", "libprotobuf-cpp-full")
	(&tc).ModuleTypeUnderTest = "cc_library_shared"
	(&tc).ModuleTypeUnderTestFactory = cc.LibrarySharedFactory
	RunBp2BuildTestCase(t, registerCcLibrarySharedModuleTypes, tc)
}

func TestCcLibrarySharedSimple(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description:             "cc_library_shared simple overall test",
		StubbedBuildDefinitions: []string{"header_lib_1", "header_lib_2", "whole_static_lib_1", "whole_static_lib_2", "shared_lib_1", "shared_lib_2"},
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
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_headers {
    name: "header_lib_1",
    export_include_dirs: ["header_lib_1"],
}

cc_library_headers {
    name: "header_lib_2",
    export_include_dirs: ["header_lib_2"],
}

cc_library_shared {
    name: "shared_lib_1",
    srcs: ["shared_lib_1.cc"],
}

cc_library_shared {
    name: "shared_lib_2",
    srcs: ["shared_lib_2.cc"],
}

cc_library_static {
    name: "whole_static_lib_1",
    srcs: ["whole_static_lib_1.cc"],
}

cc_library_static {
    name: "whole_static_lib_2",
    srcs: ["whole_static_lib_2.cc"],
}

cc_library_shared {
    name: "foo_shared",
    srcs: [
        "foo_shared1.cc",
        "foo_shared2.cc",
    ],
    cflags: [
        "-Dflag1",
        "-Dflag2"
    ],
    shared_libs: [
        "shared_lib_1",
        "shared_lib_2"
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
			MakeBazelTarget("cc_library_shared", "foo_shared", AttrNameToString{
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
    ]`,
				"implementation_dynamic_deps": `[
        ":shared_lib_1",
        ":shared_lib_2",
    ]`,
				"local_includes": `[
        "local_include_dir_1",
        "local_include_dir_2",
        ".",
    ]`,
				"srcs": `[
        "foo_shared1.cc",
        "foo_shared2.cc",
    ]`,
				"whole_archive_deps": `[
        ":whole_static_lib_1",
        ":whole_static_lib_2",
    ]`,
				"sdk_version":     `"current"`,
				"min_sdk_version": `"29"`,
				"deps": `select({
        "//build/bazel/rules/apex:unbundled_app": ["//build/bazel/rules/cc:ndk_sysroot"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestCcLibrarySharedArchSpecificSharedLib(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description:             "cc_library_shared arch-specific shared_libs with whole_static_libs",
		Filesystem:              map[string]string{},
		StubbedBuildDefinitions: []string{"static_dep", "shared_dep"},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_static {
    name: "static_dep",
}
cc_library_shared {
    name: "shared_dep",
}
cc_library_shared {
    name: "foo_shared",
    arch: { arm64: { shared_libs: ["shared_dep"], whole_static_libs: ["static_dep"] } },
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo_shared", AttrNameToString{
				"implementation_dynamic_deps": `select({
        "//build/bazel_common_rules/platforms/arch:arm64": [":shared_dep"],
        "//conditions:default": [],
    })`,
				"whole_archive_deps": `select({
        "//build/bazel_common_rules/platforms/arch:arm64": [":static_dep"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestCcLibrarySharedOsSpecificSharedLib(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		StubbedBuildDefinitions: []string{"shared_dep"},
		Description:             "cc_library_shared os-specific shared_libs",
		Filesystem:              map[string]string{},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
    name: "shared_dep",
}
cc_library_shared {
    name: "foo_shared",
    target: { android: { shared_libs: ["shared_dep"], } },
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo_shared", AttrNameToString{
				"implementation_dynamic_deps": `select({
        "//build/bazel_common_rules/platforms/os:android": [":shared_dep"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestCcLibrarySharedBaseArchOsSpecificSharedLib(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		StubbedBuildDefinitions: []string{"shared_dep", "shared_dep2", "shared_dep3"},
		Description:             "cc_library_shared base, arch, and os-specific shared_libs",
		Filesystem:              map[string]string{},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
    name: "shared_dep",
}
cc_library_shared {
    name: "shared_dep2",
}
cc_library_shared {
    name: "shared_dep3",
}
cc_library_shared {
    name: "foo_shared",
    shared_libs: ["shared_dep"],
    target: { android: { shared_libs: ["shared_dep2"] } },
    arch: { arm64: { shared_libs: ["shared_dep3"] } },
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo_shared", AttrNameToString{
				"implementation_dynamic_deps": `[":shared_dep"] + select({
        "//build/bazel_common_rules/platforms/arch:arm64": [":shared_dep3"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel_common_rules/platforms/os:android": [":shared_dep2"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestCcLibrarySharedSimpleExcludeSrcs(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared simple exclude_srcs",
		Filesystem: map[string]string{
			"common.c":       "",
			"foo-a.c":        "",
			"foo-excluded.c": "",
		},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
    name: "foo_shared",
    srcs: ["common.c", "foo-*.c"],
    exclude_srcs: ["foo-excluded.c"],
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo_shared", AttrNameToString{
				"srcs_c": `[
        "common.c",
        "foo-a.c",
    ]`,
			}),
		},
	})
}

func TestCcLibrarySharedStrip(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared stripping",
		Filesystem:  map[string]string{},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
    name: "foo_shared",
    strip: {
        keep_symbols: false,
        keep_symbols_and_debug_frame: true,
        keep_symbols_list: ["sym", "sym2"],
        all: true,
        none: false,
    },
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo_shared", AttrNameToString{
				"strip": `{
        "all": True,
        "keep_symbols": False,
        "keep_symbols_and_debug_frame": True,
        "keep_symbols_list": [
            "sym",
            "sym2",
        ],
        "none": False,
    }`,
			}),
		},
	})
}

func TestCcLibrarySharedVersionScriptAndDynamicList(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared version script and dynamic list",
		Filesystem: map[string]string{
			"version_script": "",
			"dynamic.list":   "",
		},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
    name: "foo_shared",
    version_script: "version_script",
    dynamic_list: "dynamic.list",
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo_shared", AttrNameToString{
				"additional_linker_inputs": `[
        "version_script",
        "dynamic.list",
    ]`,
				"linkopts": `[
        "-Wl,--version-script,$(location version_script)",
        "-Wl,--dynamic-list,$(location dynamic.list)",
    ]`,
				"features": `["android_cfi_exports_map"]`,
			}),
		},
	})
}

func TestCcLibraryLdflagsSplitBySpaceSoongAdded(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "ldflags are split by spaces except for the ones added by soong (version script and dynamic list)",
		Filesystem: map[string]string{
			"version_script": "",
			"dynamic.list":   "",
		},
		Blueprint: `
cc_library_shared {
    name: "foo",
    ldflags: [
        "--nospace_flag",
        "-z spaceflag",
    ],
    version_script: "version_script",
    dynamic_list: "dynamic.list",
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"additional_linker_inputs": `[
        "version_script",
        "dynamic.list",
    ]`,
				"linkopts": `[
        "--nospace_flag",
        "-z",
        "spaceflag",
        "-Wl,--version-script,$(location version_script)",
        "-Wl,--dynamic-list,$(location dynamic.list)",
    ]`,
				"features": `["android_cfi_exports_map"]`,
			}),
		},
	})
}

func TestCcLibrarySharedNoCrtTrue(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared - nocrt: true disables feature",
		Filesystem: map[string]string{
			"impl.cpp": "",
		},
		Blueprint: soongCcLibraryPreamble + `
cc_library_shared {
    name: "foo_shared",
    srcs: ["impl.cpp"],
    nocrt: true,
    include_build_directory: false,
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo_shared", AttrNameToString{
				"features": `["-link_crt"]`,
				"srcs":     `["impl.cpp"]`,
			}),
		},
	})
}

func TestCcLibrarySharedNoCrtFalse(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared - nocrt: false doesn't disable feature",
		Filesystem: map[string]string{
			"impl.cpp": "",
		},
		Blueprint: soongCcLibraryPreamble + `
cc_library_shared {
    name: "foo_shared",
    srcs: ["impl.cpp"],
    nocrt: false,
    include_build_directory: false,
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo_shared", AttrNameToString{
				"srcs": `["impl.cpp"]`,
			}),
		},
	})
}

func TestCcLibrarySharedNoCrtArchVariant(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared - nocrt in select",
		Filesystem: map[string]string{
			"impl.cpp": "",
		},
		Blueprint: soongCcLibraryPreamble + `
cc_library_shared {
    name: "foo_shared",
    srcs: ["impl.cpp"],
    arch: {
        arm: {
            nocrt: true,
        },
        x86: {
            nocrt: false,
        },
    },
    include_build_directory: false,
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo_shared", AttrNameToString{
				"features": `select({
        "//build/bazel_common_rules/platforms/arch:arm": ["-link_crt"],
        "//conditions:default": [],
    })`,
				"srcs": `["impl.cpp"]`,
			}),
		},
	})
}

func TestCcLibrarySharedProto(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Blueprint: soongCcProtoPreamble + `cc_library_shared {
	name: "foo",
	srcs: ["foo.proto"],
	proto: {
		export_proto_headers: true,
	},
	include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("proto_library", "foo_proto", AttrNameToString{
				"srcs": `["foo.proto"]`,
			}), MakeBazelTarget("cc_lite_proto_library", "foo_cc_proto_lite", AttrNameToString{
				"deps": `[":foo_proto"]`,
			}), MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"dynamic_deps":       `[":libprotobuf-cpp-lite"]`,
				"whole_archive_deps": `[":foo_cc_proto_lite"]`,
			}),
		},
	})
}

func TestCcLibrarySharedUseVersionLib(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Filesystem: map[string]string{
			soongCcVersionLibBpPath: soongCcVersionLibBp,
		},
		StubbedBuildDefinitions: []string{"//build/soong/cc/libbuildversion:libbuildversion"},
		Blueprint: soongCcProtoPreamble + `cc_library_shared {
        name: "foo",
        use_version_lib: true,
        include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"use_version_lib":    "True",
				"whole_archive_deps": `["//build/soong/cc/libbuildversion:libbuildversion"]`,
			}),
		},
	})
}

func TestCcLibrarySharedStubs(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description:                "cc_library_shared stubs",
		ModuleTypeUnderTest:        "cc_library_shared",
		ModuleTypeUnderTestFactory: cc.LibrarySharedFactory,
		Dir:                        "foo/bar",
		Filesystem: map[string]string{
			"foo/bar/Android.bp": `
cc_library_shared {
	name: "a",
	stubs: { symbol_file: "a.map.txt", versions: ["28", "29", "current"] },
	bazel_module: { bp2build_available: true },
	include_build_directory: false,
}
`,
		},
		Blueprint: soongCcLibraryPreamble,
		ExpectedBazelTargets: []string{makeCcStubSuiteTargets("a", AttrNameToString{
			"api_surface":          `"module-libapi"`,
			"soname":               `"a.so"`,
			"source_library_label": `"//foo/bar:a"`,
			"stubs_symbol_file":    `"a.map.txt"`,
			"stubs_versions": `[
        "28",
        "29",
        "current",
    ]`,
		}),
			MakeBazelTarget("cc_library_shared", "a", AttrNameToString{
				"stubs_symbol_file": `"a.map.txt"`,
			}),
		},
	})
}

func TestCcLibrarySharedStubs_UseImplementationInSameApex(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description:                "cc_library_shared stubs",
		ModuleTypeUnderTest:        "cc_library_shared",
		ModuleTypeUnderTestFactory: cc.LibrarySharedFactory,
		StubbedBuildDefinitions:    []string{"a"},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
	name: "a",
	stubs: { symbol_file: "a.map.txt", versions: ["28", "29", "current"] },
	include_build_directory: false,
	apex_available: ["made_up_apex"],
}
cc_library_shared {
	name: "b",
	shared_libs: [":a"],
	include_build_directory: false,
	apex_available: ["made_up_apex"],
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "b", AttrNameToString{
				"implementation_dynamic_deps": `[":a"]`,
				"tags":                        `["apex_available=made_up_apex"]`,
			}),
		},
	})
}

func TestCcLibrarySharedStubs_UseStubsInDifferentApex(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description:                "cc_library_shared stubs",
		ModuleTypeUnderTest:        "cc_library_shared",
		ModuleTypeUnderTestFactory: cc.LibrarySharedFactory,
		StubbedBuildDefinitions:    []string{"a"},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
	name: "a",
	stubs: { symbol_file: "a.map.txt", versions: ["28", "29", "current"] },
	include_build_directory: false,
	apex_available: ["apex_a"],
}
cc_library_shared {
	name: "b",
	shared_libs: [":a"],
	include_build_directory: false,
	apex_available: ["apex_b"],
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "b", AttrNameToString{
				"implementation_dynamic_deps": `select({
        "//build/bazel/rules/apex:apex_b": ["@api_surfaces//module-libapi/current:a"],
        "//build/bazel/rules/apex:system": ["@api_surfaces//module-libapi/current:a"],
        "//conditions:default": [":a"],
    })`,
				"tags": `["apex_available=apex_b"]`,
			}),
		},
	})
}

// Tests that library in apexfoo links against stubs of platform_lib and otherapex_lib
func TestCcLibrarySharedStubs_UseStubsFromMultipleApiDomains(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description:                "cc_library_shared stubs",
		ModuleTypeUnderTest:        "cc_library_shared",
		ModuleTypeUnderTestFactory: cc.LibrarySharedFactory,
		StubbedBuildDefinitions:    []string{"libplatform_stable", "libapexfoo_stable"},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
	name: "libplatform_stable",
	stubs: { symbol_file: "libplatform_stable.map.txt", versions: ["28", "29", "current"] },
	apex_available: ["//apex_available:platform"],
	include_build_directory: false,
}
cc_library_shared {
	name: "libapexfoo_stable",
	stubs: { symbol_file: "libapexfoo_stable.map.txt", versions: ["28", "29", "current"] },
	apex_available: ["apexfoo"],
	include_build_directory: false,
}
cc_library_shared {
	name: "libutils",
	shared_libs: ["libplatform_stable", "libapexfoo_stable",],
	apex_available: ["//apex_available:platform", "apexfoo", "apexbar"],
	include_build_directory: false,
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "libutils", AttrNameToString{
				"implementation_dynamic_deps": `select({
        "//build/bazel/rules/apex:apexbar": [
            "@api_surfaces//module-libapi/current:libplatform_stable",
            "@api_surfaces//module-libapi/current:libapexfoo_stable",
        ],
        "//build/bazel/rules/apex:apexfoo": [
            "@api_surfaces//module-libapi/current:libplatform_stable",
            ":libapexfoo_stable",
        ],
        "//build/bazel/rules/apex:system": [
            ":libplatform_stable",
            "@api_surfaces//module-libapi/current:libapexfoo_stable",
        ],
        "//conditions:default": [
            ":libplatform_stable",
            ":libapexfoo_stable",
        ],
    })`,
				"tags": `[
        "apex_available=//apex_available:platform",
        "apex_available=apexfoo",
        "apex_available=apexbar",
    ]`,
			}),
		},
	})
}

func TestCcLibrarySharedStubs_IgnorePlatformAvailable(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description:                "cc_library_shared stubs",
		ModuleTypeUnderTest:        "cc_library_shared",
		ModuleTypeUnderTestFactory: cc.LibrarySharedFactory,
		StubbedBuildDefinitions:    []string{"a"},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
	name: "a",
	stubs: { symbol_file: "a.map.txt", versions: ["28", "29", "current"] },
	include_build_directory: false,
	apex_available: ["//apex_available:platform", "apex_a"],
}
cc_library_shared {
	name: "b",
	shared_libs: [":a"],
	include_build_directory: false,
	apex_available: ["//apex_available:platform", "apex_b"],
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "b", AttrNameToString{
				"implementation_dynamic_deps": `select({
        "//build/bazel/rules/apex:apex_b": ["@api_surfaces//module-libapi/current:a"],
        "//build/bazel/rules/apex:system": ["@api_surfaces//module-libapi/current:a"],
        "//conditions:default": [":a"],
    })`,
				"tags": `[
        "apex_available=//apex_available:platform",
        "apex_available=apex_b",
    ]`,
			}),
		},
	})
}

func TestCcLibraryDoesNotDropStubDepIfNoVariationAcrossAxis(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description:                "cc_library depeends on impl for all configurations",
		ModuleTypeUnderTest:        "cc_library_shared",
		ModuleTypeUnderTestFactory: cc.LibrarySharedFactory,
		StubbedBuildDefinitions:    []string{"a"},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
	name: "a",
	stubs: { symbol_file: "a.map.txt", versions: ["28", "29", "current"] },
	apex_available: ["//apex_available:platform"],
}
cc_library_shared {
	name: "b",
	shared_libs: [":a"],
	include_build_directory: false,
	apex_available: ["//apex_available:platform"],
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "b", AttrNameToString{
				"implementation_dynamic_deps": `[":a"]`,
				"tags":                        `["apex_available=//apex_available:platform"]`,
			}),
		},
	})
}

func TestCcLibrarySharedStubs_MultipleApexAvailable(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		ModuleTypeUnderTest:        "cc_library_shared",
		ModuleTypeUnderTestFactory: cc.LibrarySharedFactory,
		StubbedBuildDefinitions:    []string{"a"},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
	name: "a",
	stubs: { symbol_file: "a.map.txt", versions: ["28", "29", "current"] },
	include_build_directory: false,
	apex_available: ["//apex_available:platform", "apex_a", "apex_b"],
}
cc_library_shared {
	name: "b",
	shared_libs: [":a"],
	include_build_directory: false,
	apex_available: ["//apex_available:platform", "apex_b"],
}

cc_library_shared {
	name: "c",
	shared_libs: [":a"],
	include_build_directory: false,
	apex_available: ["//apex_available:platform", "apex_a", "apex_b"],
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "b", AttrNameToString{
				"implementation_dynamic_deps": `select({
        "//build/bazel/rules/apex:system": ["@api_surfaces//module-libapi/current:a"],
        "//conditions:default": [":a"],
    })`,
				"tags": `[
        "apex_available=//apex_available:platform",
        "apex_available=apex_b",
    ]`,
			}),
			MakeBazelTarget("cc_library_shared", "c", AttrNameToString{
				"implementation_dynamic_deps": `[":a"]`,
				"tags": `[
        "apex_available=//apex_available:platform",
        "apex_available=apex_a",
        "apex_available=apex_b",
    ]`,
			}),
		},
	})
}

func TestCcLibrarySharedSystemSharedLibsSharedEmpty(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description:                "cc_library_shared system_shared_libs empty shared default",
		ModuleTypeUnderTest:        "cc_library_shared",
		ModuleTypeUnderTestFactory: cc.LibrarySharedFactory,
		Blueprint: soongCcLibrarySharedPreamble + `
cc_defaults {
    name: "empty_defaults",
    shared: {
        system_shared_libs: [],
    },
    include_build_directory: false,
}
cc_library_shared {
    name: "empty",
    defaults: ["empty_defaults"],
}
`,
		ExpectedBazelTargets: []string{MakeBazelTarget("cc_library_shared", "empty", AttrNameToString{
			"system_dynamic_deps": "[]",
		})},
	})
}

func TestCcLibrarySharedConvertLex(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description:                "cc_library_shared with lex files",
		ModuleTypeUnderTest:        "cc_library_shared",
		ModuleTypeUnderTestFactory: cc.LibrarySharedFactory,
		Filesystem: map[string]string{
			"foo.c":   "",
			"bar.cc":  "",
			"foo1.l":  "",
			"bar1.ll": "",
			"foo2.l":  "",
			"bar2.ll": "",
		},
		Blueprint: `cc_library_shared {
	name: "foo_lib",
	srcs: ["foo.c", "bar.cc", "foo1.l", "foo2.l", "bar1.ll", "bar2.ll"],
	lex: { flags: ["--foo_flags"] },
	include_build_directory: false,
	bazel_module: { bp2build_available: true },
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("genlex", "foo_lib_genlex_l", AttrNameToString{
				"srcs": `[
        "foo1.l",
        "foo2.l",
    ]`,
				"lexopts": `["--foo_flags"]`,
			}),
			MakeBazelTarget("genlex", "foo_lib_genlex_ll", AttrNameToString{
				"srcs": `[
        "bar1.ll",
        "bar2.ll",
    ]`,
				"lexopts": `["--foo_flags"]`,
			}),
			MakeBazelTarget("cc_library_shared", "foo_lib", AttrNameToString{
				"srcs": `[
        "bar.cc",
        ":foo_lib_genlex_ll",
    ]`,
				"srcs_c": `[
        "foo.c",
        ":foo_lib_genlex_l",
    ]`,
			}),
		},
	})
}

func TestCcLibrarySharedClangUnknownFlags(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Blueprint: soongCcProtoPreamble + `cc_library_shared {
	name: "foo",
	conlyflags: ["-a", "-finline-functions"],
	cflags: ["-b","-finline-functions"],
	cppflags: ["-c", "-finline-functions"],
	ldflags: ["-d","-finline-functions", "-e"],
	include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"conlyflags": `["-a"]`,
				"copts":      `["-b"]`,
				"cppflags":   `["-c"]`,
				"linkopts": `[
        "-d",
        "-e",
    ]`,
			}),
		},
	})
}

func TestCCLibraryFlagSpaceSplitting(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Blueprint: soongCcProtoPreamble + `cc_library_shared {
	name: "foo",
	conlyflags: [ "-include header.h"],
	cflags: ["-include header.h"],
	cppflags: ["-include header.h"],
	version_script: "version_script",
	include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"additional_linker_inputs": `["version_script"]`,
				"conlyflags": `[
        "-include",
        "header.h",
    ]`,
				"copts": `[
        "-include",
        "header.h",
    ]`,
				"cppflags": `[
        "-include",
        "header.h",
    ]`,
				"linkopts": `["-Wl,--version-script,$(location version_script)"]`,
				"features": `["android_cfi_exports_map"]`,
			}),
		},
	})
}

func TestCCLibrarySharedRuntimeDeps(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Blueprint: `cc_library_shared {
	name: "bar",
}

cc_library_shared {
  name: "foo",
  runtime_libs: ["bar"],
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "bar", AttrNameToString{
				"local_includes": `["."]`,
			}),
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"runtime_deps":   `[":bar"]`,
				"local_includes": `["."]`,
			}),
		},
	})
}

func TestCcLibrarySharedEmptySuffix(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared with empty suffix",
		Filesystem: map[string]string{
			"foo.c": "",
		},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
    name: "foo_shared",
    suffix: "",
    srcs: ["foo.c"],
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo_shared", AttrNameToString{
				"srcs_c": `["foo.c"]`,
				"suffix": `""`,
			}),
		},
	})
}

func TestCcLibrarySharedSuffix(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared with suffix",
		Filesystem: map[string]string{
			"foo.c": "",
		},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
    name: "foo_shared",
    suffix: "-suf",
    srcs: ["foo.c"],
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo_shared", AttrNameToString{
				"srcs_c": `["foo.c"]`,
				"suffix": `"-suf"`,
			}),
		},
	})
}

func TestCcLibrarySharedArchVariantSuffix(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared with arch-variant suffix",
		Filesystem: map[string]string{
			"foo.c": "",
		},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
    name: "foo_shared",
    arch: {
        arm64: { suffix: "-64" },
        arm:   { suffix: "-32" },
		},
    srcs: ["foo.c"],
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo_shared", AttrNameToString{
				"srcs_c": `["foo.c"]`,
				"suffix": `select({
        "//build/bazel_common_rules/platforms/arch:arm": "-32",
        "//build/bazel_common_rules/platforms/arch:arm64": "-64",
        "//conditions:default": None,
    })`,
			}),
		},
	})
}

func TestCcLibrarySharedWithSyspropSrcs(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared with sysprop sources",
		Blueprint: `
cc_library_shared {
	name: "foo",
	srcs: [
		"bar.sysprop",
		"baz.sysprop",
		"blah.cpp",
	],
	min_sdk_version: "5",
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("sysprop_library", "foo_sysprop_library", AttrNameToString{
				"srcs": `[
        "bar.sysprop",
        "baz.sysprop",
    ]`,
			}),
			MakeBazelTarget("cc_sysprop_library_static", "foo_cc_sysprop_library_static", AttrNameToString{
				"dep":             `":foo_sysprop_library"`,
				"min_sdk_version": `"5"`,
			}),
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"srcs":               `["blah.cpp"]`,
				"local_includes":     `["."]`,
				"min_sdk_version":    `"5"`,
				"whole_archive_deps": `[":foo_cc_sysprop_library_static"]`,
			}),
		},
	})
}

func TestCcLibrarySharedWithSyspropSrcsSomeConfigs(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared with sysprop sources in some configs but not others",
		Blueprint: `
cc_library_shared {
	name: "foo",
	srcs: [
		"blah.cpp",
	],
	target: {
		android: {
			srcs: ["bar.sysprop"],
		},
	},
	min_sdk_version: "5",
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("sysprop_library", "foo_sysprop_library", AttrNameToString{
				"srcs": `select({
        "//build/bazel_common_rules/platforms/os:android": ["bar.sysprop"],
        "//conditions:default": [],
    })`,
			}),
			MakeBazelTarget("cc_sysprop_library_static", "foo_cc_sysprop_library_static", AttrNameToString{
				"dep":             `":foo_sysprop_library"`,
				"min_sdk_version": `"5"`,
			}),
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"srcs":            `["blah.cpp"]`,
				"local_includes":  `["."]`,
				"min_sdk_version": `"5"`,
				"whole_archive_deps": `select({
        "//build/bazel_common_rules/platforms/os:android": [":foo_cc_sysprop_library_static"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestCcLibrarySharedHeaderAbiChecker(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared with header abi checker",
		Blueprint: `cc_library_shared {
    name: "foo",
    header_abi_checker: {
        enabled: true,
        symbol_file: "a.map.txt",
        exclude_symbol_versions: [
						"29",
						"30",
				],
        exclude_symbol_tags: [
						"tag1",
						"tag2",
				],
        check_all_apis: true,
        diff_flags: ["-allow-adding-removing-weak-symbols"],
    },
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"abi_checker_enabled":     `True`,
				"abi_checker_symbol_file": `"a.map.txt"`,
				"abi_checker_exclude_symbol_versions": `[
        "29",
        "30",
    ]`,
				"abi_checker_exclude_symbol_tags": `[
        "tag1",
        "tag2",
    ]`,
				"abi_checker_check_all_apis": `True`,
				"abi_checker_diff_flags":     `["-allow-adding-removing-weak-symbols"]`,
			}),
		},
	})
}

func TestCcLibrarySharedWithIntegerOverflowProperty(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared has correct features when integer_overflow property is provided",
		Blueprint: `
cc_library_shared {
		name: "foo",
		sanitize: {
				integer_overflow: true,
		},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"features":       `["ubsan_integer_overflow"]`,
				"local_includes": `["."]`,
			}),
		},
	})
}

func TestCcLibrarySharedWithMiscUndefinedProperty(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared has correct features when misc_undefined property is provided",
		Blueprint: `
cc_library_shared {
		name: "foo",
		sanitize: {
				misc_undefined: ["undefined", "nullability"],
		},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"features": `[
        "ubsan_undefined",
        "ubsan_nullability",
    ]`,
				"local_includes": `["."]`,
			}),
		},
	})
}

func TestCcLibrarySharedWithUBSanPropertiesArchSpecific(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared has correct feature select when UBSan props are specified in arch specific blocks",
		Blueprint: `
cc_library_shared {
		name: "foo",
		sanitize: {
				misc_undefined: ["undefined", "nullability"],
		},
		target: {
				android: {
						sanitize: {
								misc_undefined: ["alignment"],
						},
				},
				linux_glibc: {
						sanitize: {
								integer_overflow: true,
						},
				},
		},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"features": `[
        "ubsan_undefined",
        "ubsan_nullability",
    ] + select({
        "//build/bazel_common_rules/platforms/os:android": ["ubsan_alignment"],
        "//build/bazel_common_rules/platforms/os:linux_glibc": ["ubsan_integer_overflow"],
        "//conditions:default": [],
    })`,
				"local_includes": `["."]`,
			}),
		},
	})
}

func TestCcLibrarySharedWithSanitizerBlocklist(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared has correct features when sanitize.blocklist is provided",
		Blueprint: `
cc_library_shared {
	name: "foo",
	sanitize: {
		blocklist: "foo_blocklist.txt",
	},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"copts": `select({
        "//build/bazel/rules/cc:sanitizers_enabled": ["-fsanitize-ignorelist=$(location foo_blocklist.txt)"],
        "//conditions:default": [],
    })`,
				"additional_compiler_inputs": `select({
        "//build/bazel/rules/cc:sanitizers_enabled": [":foo_blocklist.txt"],
        "//conditions:default": [],
    })`,
				"local_includes": `["."]`,
			}),
		},
	})
}

func TestCcLibrarySharedWithThinLto(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared has correct features when thin lto is enabled",
		Blueprint: `
cc_library_shared {
	name: "foo",
	lto: {
		thin: true,
	},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"features":       `["android_thin_lto"]`,
				"local_includes": `["."]`,
			}),
		},
	})
}

func TestCcLibrarySharedWithLtoNever(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared has correct features when thin lto is enabled",
		Blueprint: `
cc_library_shared {
	name: "foo",
	lto: {
		never: true,
	},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"features":       `["-android_thin_lto"]`,
				"local_includes": `["."]`,
			}),
		},
	})
}

func TestCcLibrarySharedWithThinLtoArchSpecific(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared has correct features when LTO differs across arch and os variants",
		Blueprint: `
cc_library_shared {
	name: "foo",
	target: {
		android: {
			lto: {
				thin: true,
			},
		},
	},
	arch: {
		riscv64: {
			lto: {
				thin: false,
			},
		},
	},
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"local_includes": `["."]`,
				"features": `select({
        "//build/bazel_common_rules/platforms/os_arch:android_arm": ["android_thin_lto"],
        "//build/bazel_common_rules/platforms/os_arch:android_arm64": ["android_thin_lto"],
        "//build/bazel_common_rules/platforms/os_arch:android_riscv64": ["-android_thin_lto"],
        "//build/bazel_common_rules/platforms/os_arch:android_x86": ["android_thin_lto"],
        "//build/bazel_common_rules/platforms/os_arch:android_x86_64": ["android_thin_lto"],
        "//conditions:default": [],
    })`}),
		},
	})
}

func TestCcLibrarySharedWithThinLtoDisabledDefaultEnabledVariant(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared with thin lto disabled by default but enabled on a particular variant",
		Blueprint: `
cc_library_shared {
	name: "foo",
	lto: {
		never: true,
	},
	target: {
		android: {
			lto: {
				thin: true,
				never: false,
			},
		},
	},
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"local_includes": `["."]`,
				"features": `select({
        "//build/bazel_common_rules/platforms/os:android": ["android_thin_lto"],
        "//conditions:default": ["-android_thin_lto"],
    })`,
			}),
		},
	})
}

func TestCcLibrarySharedWithThinLtoAndWholeProgramVtables(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared has correct features when thin LTO is enabled with whole_program_vtables",
		Blueprint: `
cc_library_shared {
	name: "foo",
	lto: {
		thin: true,
	},
	whole_program_vtables: true,
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"features": `[
        "android_thin_lto",
        "android_thin_lto_whole_program_vtables",
    ]`,
				"local_includes": `["."]`,
			}),
		},
	})
}

func TestCcLibrarySharedHiddenVisibilityConvertedToFeature(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared changes hidden visibility flag to feature",
		Blueprint: `
cc_library_shared{
	name: "foo",
	cflags: ["-fvisibility=hidden"],
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"features":       `["visibility_hidden"]`,
				"local_includes": `["."]`,
			}),
		},
	})
}

func TestCcLibrarySharedHiddenVisibilityConvertedToFeatureOsSpecific(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared changes hidden visibility flag to feature for specific os",
		Blueprint: `
cc_library_shared{
	name: "foo",
	target: {
		android: {
			cflags: ["-fvisibility=hidden"],
		},
	},
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"features": `select({
        "//build/bazel_common_rules/platforms/os:android": ["visibility_hidden"],
        "//conditions:default": [],
    })`,
				"local_includes": `["."]`,
			}),
		},
	})
}

func TestCcLibrarySharedStubsDessertVersionConversion(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared converts dessert codename versions to numerical versions",
		Blueprint: `
cc_library_shared {
	name: "a",
	include_build_directory: false,
	stubs: {
		symbol_file: "a.map.txt",
		versions: [
			"Q",
			"R",
			"31",
		],
	},
}
cc_library_shared {
	name: "b",
	include_build_directory: false,
	stubs: {
		symbol_file: "b.map.txt",
		versions: [
			"Q",
			"R",
			"31",
			"current",
		],
	},
}
`,
		ExpectedBazelTargets: []string{
			makeCcStubSuiteTargets("a", AttrNameToString{
				"api_surface":          `"module-libapi"`,
				"soname":               `"a.so"`,
				"source_library_label": `"//:a"`,
				"stubs_symbol_file":    `"a.map.txt"`,
				"stubs_versions": `[
        "29",
        "30",
        "31",
        "current",
    ]`,
			}),
			MakeBazelTarget("cc_library_shared", "a", AttrNameToString{
				"stubs_symbol_file": `"a.map.txt"`,
			}),
			makeCcStubSuiteTargets("b", AttrNameToString{
				"api_surface":          `"module-libapi"`,
				"soname":               `"b.so"`,
				"source_library_label": `"//:b"`,
				"stubs_symbol_file":    `"b.map.txt"`,
				"stubs_versions": `[
        "29",
        "30",
        "31",
        "current",
    ]`,
			}),
			MakeBazelTarget("cc_library_shared", "b", AttrNameToString{
				"stubs_symbol_file": `"b.map.txt"`,
			}),
		},
	})
}

func TestCcLibrarySharedWithCfi(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared has correct features when cfi is enabled for specific variants",
		Blueprint: `
cc_library_shared {
	name: "foo",
	sanitize: {
		cfi: true,
	},
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"features":       `["android_cfi"]`,
				"local_includes": `["."]`,
			}),
		},
	})
}

func TestCcLibrarySharedWithCfiOsSpecific(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared has correct features when cfi is enabled",
		Blueprint: `
cc_library_shared {
	name: "foo",
	target: {
		android: {
			sanitize: {
				cfi: true,
			},
		},
	},
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"features": `select({
        "//build/bazel_common_rules/platforms/os:android": ["android_cfi"],
        "//conditions:default": [],
    })`,
				"local_includes": `["."]`,
			}),
		},
	})
}

func TestCcLibrarySharedWithCfiAndCfiAssemblySupport(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared has correct features when cfi is enabled with cfi assembly support",
		Blueprint: `
cc_library_static {
	name: "foo",
	sanitize: {
		cfi: true,
		config: {
			cfi_assembly_support: true,
		},
	},
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_static", "foo", AttrNameToString{
				"features": `[
        "android_cfi",
        "android_cfi_assembly_support",
    ]`,
				"local_includes": `["."]`,
			}),
		},
	})
}

func TestCcLibrarySharedExplicitlyDisablesCfiWhenFalse(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared disables cfi when explciitly set to false in the bp",
		Blueprint: `
cc_library_shared {
	name: "foo",
	sanitize: {
		cfi: false,
	},
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"features":       `["-android_cfi"]`,
				"local_includes": `["."]`,
			}),
		},
	})
}

func TestCCLibrarySharedRscriptSrc(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: ``,
		Blueprint: `
cc_library_shared{
    name : "foo",
    srcs : [
        "ccSrc.cc",
        "rsSrc.rscript",
    ],
    include_build_directory: false,
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("rscript_to_cpp", "foo_renderscript", AttrNameToString{
				"srcs": `["rsSrc.rscript"]`,
			}),
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"absolute_includes": `[
        "frameworks/rs",
        "frameworks/rs/cpp",
    ]`,
				"local_includes": `["."]`,
				"srcs": `[
        "ccSrc.cc",
        "foo_renderscript",
    ]`,
			})}})
}

func TestCcLibrarySdkVariantUsesStubs(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description:                "cc_library_shared stubs",
		ModuleTypeUnderTest:        "cc_library_shared",
		ModuleTypeUnderTestFactory: cc.LibrarySharedFactory,
		StubbedBuildDefinitions:    []string{"libNoStubs", "libHasApexStubs", "libHasApexAndNdkStubs", "libHasApexAndNdkStubs.ndk_stub_libs"},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
	name: "libUsesSdk",
	sdk_version: "current",
	shared_libs: [
		"libNoStubs",
		"libHasApexStubs",
		"libHasApexAndNdkStubs",
	]
}
cc_library_shared {
	name: "libNoStubs",
}
cc_library_shared {
	name: "libHasApexStubs",
	stubs: { symbol_file: "a.map.txt", versions: ["28", "29", "current"] },
	apex_available: ["apex_a"],
}
cc_library_shared {
	name: "libHasApexAndNdkStubs",
	stubs: { symbol_file: "b.map.txt", versions: ["28", "29", "current"] },
	apex_available: ["apex_b"],
}
ndk_library {
	name: "libHasApexAndNdkStubs",
	first_version: "28",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "libUsesSdk", AttrNameToString{
				"implementation_dynamic_deps": `[":libNoStubs"] + select({
        "//build/bazel/rules/apex:system": [
            "@api_surfaces//module-libapi/current:libHasApexStubs",
            "@api_surfaces//module-libapi/current:libHasApexAndNdkStubs",
        ],
        "//build/bazel/rules/apex:unbundled_app": [
            ":libHasApexStubs",
            "//.:libHasApexAndNdkStubs.ndk_stub_libs-current",
        ],
        "//conditions:default": [
            ":libHasApexStubs",
            ":libHasApexAndNdkStubs",
        ],
    })`,
				"local_includes": `["."]`,
				"sdk_version":    `"current"`,
				"deps": `select({
        "//build/bazel/rules/apex:unbundled_app": ["//build/bazel/rules/cc:ndk_sysroot"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}
