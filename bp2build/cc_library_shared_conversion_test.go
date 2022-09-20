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
	"fmt"
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
}

func runCcLibrarySharedTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	(&tc).ModuleTypeUnderTest = "cc_library_shared"
	(&tc).ModuleTypeUnderTestFactory = cc.LibrarySharedFactory
	RunBp2BuildTestCase(t, registerCcLibrarySharedModuleTypes, tc)
}

func TestCcLibrarySharedSimple(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared simple overall test",
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
    bazel_module: { bp2build_available: false },
}

cc_library_headers {
    name: "header_lib_2",
    export_include_dirs: ["header_lib_2"],
    bazel_module: { bp2build_available: false },
}

cc_library_shared {
    name: "shared_lib_1",
    srcs: ["shared_lib_1.cc"],
    bazel_module: { bp2build_available: false },
}

cc_library_shared {
    name: "shared_lib_2",
    srcs: ["shared_lib_2.cc"],
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
			}),
		},
	})
}

func TestCcLibrarySharedArchSpecificSharedLib(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared arch-specific shared_libs with whole_static_libs",
		Filesystem:  map[string]string{},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_static {
    name: "static_dep",
    bazel_module: { bp2build_available: false },
}
cc_library_shared {
    name: "shared_dep",
    bazel_module: { bp2build_available: false },
}
cc_library_shared {
    name: "foo_shared",
    arch: { arm64: { shared_libs: ["shared_dep"], whole_static_libs: ["static_dep"] } },
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo_shared", AttrNameToString{
				"implementation_dynamic_deps": `select({
        "//build/bazel/platforms/arch:arm64": [":shared_dep"],
        "//conditions:default": [],
    })`,
				"whole_archive_deps": `select({
        "//build/bazel/platforms/arch:arm64": [":static_dep"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestCcLibrarySharedOsSpecificSharedLib(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared os-specific shared_libs",
		Filesystem:  map[string]string{},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
    name: "shared_dep",
    bazel_module: { bp2build_available: false },
}
cc_library_shared {
    name: "foo_shared",
    target: { android: { shared_libs: ["shared_dep"], } },
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo_shared", AttrNameToString{
				"implementation_dynamic_deps": `select({
        "//build/bazel/platforms/os:android": [":shared_dep"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestCcLibrarySharedBaseArchOsSpecificSharedLib(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared base, arch, and os-specific shared_libs",
		Filesystem:  map[string]string{},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
    name: "shared_dep",
    bazel_module: { bp2build_available: false },
}
cc_library_shared {
    name: "shared_dep2",
    bazel_module: { bp2build_available: false },
}
cc_library_shared {
    name: "shared_dep3",
    bazel_module: { bp2build_available: false },
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
        "//build/bazel/platforms/arch:arm64": [":shared_dep3"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel/platforms/os:android": [":shared_dep2"],
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

func TestCcLibrarySharedVersionScript(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared version script",
		Filesystem: map[string]string{
			"version_script": "",
		},
		Blueprint: soongCcLibrarySharedPreamble + `
cc_library_shared {
    name: "foo_shared",
    version_script: "version_script",
    include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo_shared", AttrNameToString{
				"additional_linker_inputs": `["version_script"]`,
				"linkopts":                 `["-Wl,--version-script,$(location version_script)"]`,
			}),
		},
	})
}

func TestCcLibrarySharedNoCrtTrue(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared - nocrt: true emits attribute",
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
				"link_crt": `False`,
				"srcs":     `["impl.cpp"]`,
			}),
		},
	})
}

func TestCcLibrarySharedNoCrtFalse(t *testing.T) {
	runCcLibrarySharedTestCase(t, Bp2buildTestCase{
		Description: "cc_library_shared - nocrt: false doesn't emit attribute",
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
		ExpectedErr: fmt.Errorf("module \"foo_shared\": nocrt is not supported for arch variants"),
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
		Blueprint: soongCcProtoPreamble + `cc_library_shared {
        name: "foo",
        use_version_lib: true,
        include_build_directory: false,
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"use_version_lib":                   "True",
				"implementation_whole_archive_deps": `["//build/soong/cc/libbuildversion:libbuildversion"]`,
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
		ExpectedBazelTargets: []string{MakeBazelTarget("cc_library_shared", "a", AttrNameToString{
			"has_stubs": `True`,
		}),
			makeCcStubSuiteTargets("a", AttrNameToString{
				"soname":            `"a.so"`,
				"source_library":    `":a"`,
				"stubs_symbol_file": `"a.map.txt"`,
				"stubs_versions": `[
        "28",
        "29",
        "current",
    ]`,
			}),
		},
	},
	)
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
  runtime_libs: ["foo"],
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_library_shared", "bar", AttrNameToString{
				"local_includes": `["."]`,
			}),
			MakeBazelTarget("cc_library_shared", "foo", AttrNameToString{
				"runtime_deps":   `[":foo"]`,
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
        "//build/bazel/platforms/arch:arm": "-32",
        "//build/bazel/platforms/arch:arm64": "-64",
        "//conditions:default": None,
    })`,
			}),
		},
	})
}
