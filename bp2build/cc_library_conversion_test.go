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
	soongCcLibraryPreamble = `
cc_defaults {
    name: "linux_bionic_supported",
}`

	soongCcProtoLibraries = `
cc_library {
	name: "libprotobuf-cpp-lite",
	bazel_module: { bp2build_available: false },
}

cc_library {
	name: "libprotobuf-cpp-full",
	bazel_module: { bp2build_available: false },
}`

	soongCcProtoPreamble = soongCcLibraryPreamble + soongCcProtoLibraries
)

func runCcLibraryTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	runBp2BuildTestCase(t, registerCcLibraryModuleTypes, tc)
}

func registerCcLibraryModuleTypes(ctx android.RegistrationContext) {
	cc.RegisterCCBuildComponents(ctx)
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	ctx.RegisterModuleType("cc_library_static", cc.LibraryStaticFactory)
	ctx.RegisterModuleType("cc_prebuilt_library_static", cc.PrebuiltStaticLibraryFactory)
	ctx.RegisterModuleType("cc_library_headers", cc.LibraryHeaderFactory)
}

func TestCcLibrarySimple(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library - simple example",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		filesystem: map[string]string{
			"android.cpp": "",
			"bionic.cpp":  "",
			"darwin.cpp":  "",
			// Refer to cc.headerExts for the supported header extensions in Soong.
			"header.h":         "",
			"header.hh":        "",
			"header.hpp":       "",
			"header.hxx":       "",
			"header.h++":       "",
			"header.inl":       "",
			"header.inc":       "",
			"header.ipp":       "",
			"header.h.generic": "",
			"impl.cpp":         "",
			"linux.cpp":        "",
			"x86.cpp":          "",
			"x86_64.cpp":       "",
			"foo-dir/a.h":      "",
		},
		blueprint: soongCcLibraryPreamble +
			simpleModuleDoNotConvertBp2build("cc_library_headers", "some-headers") + `
cc_library {
    name: "foo-lib",
    srcs: ["impl.cpp"],
    cflags: ["-Wall"],
    header_libs: ["some-headers"],
    export_include_dirs: ["foo-dir"],
    ldflags: ["-Wl,--exclude-libs=bar.a"],
    arch: {
        x86: {
            ldflags: ["-Wl,--exclude-libs=baz.a"],
            srcs: ["x86.cpp"],
        },
        x86_64: {
            ldflags: ["-Wl,--exclude-libs=qux.a"],
            srcs: ["x86_64.cpp"],
        },
    },
    target: {
        android: {
            srcs: ["android.cpp"],
        },
        linux_glibc: {
            srcs: ["linux.cpp"],
        },
        darwin: {
            srcs: ["darwin.cpp"],
        },
        bionic: {
          srcs: ["bionic.cpp"]
        },
    },
    include_build_directory: false,
    sdk_version: "current",
    min_sdk_version: "29",
    use_version_lib: true,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("foo-lib", attrNameToString{
			"copts":               `["-Wall"]`,
			"export_includes":     `["foo-dir"]`,
			"implementation_deps": `[":some-headers"]`,
			"linkopts": `["-Wl,--exclude-libs=bar.a"] + select({
        "//build/bazel/platforms/arch:x86": ["-Wl,--exclude-libs=baz.a"],
        "//build/bazel/platforms/arch:x86_64": ["-Wl,--exclude-libs=qux.a"],
        "//conditions:default": [],
    })`,
			"srcs": `["impl.cpp"] + select({
        "//build/bazel/platforms/arch:x86": ["x86.cpp"],
        "//build/bazel/platforms/arch:x86_64": ["x86_64.cpp"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel/platforms/os:android": [
            "bionic.cpp",
            "android.cpp",
        ],
        "//build/bazel/platforms/os:darwin": ["darwin.cpp"],
        "//build/bazel/platforms/os:linux": ["linux.cpp"],
        "//build/bazel/platforms/os:linux_bionic": ["bionic.cpp"],
        "//conditions:default": [],
    })`,
			"sdk_version":     `"current"`,
			"min_sdk_version": `"29"`,
			"use_version_lib": `True`,
		}),
	})
}

func TestCcLibraryTrimmedLdAndroid(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library - trimmed example of //bionic/linker:ld-android",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		filesystem: map[string]string{
			"ld-android.cpp":           "",
			"linked_list.h":            "",
			"linker.h":                 "",
			"linker_block_allocator.h": "",
			"linker_cfi.h":             "",
		},
		blueprint: soongCcLibraryPreamble +
			simpleModuleDoNotConvertBp2build("cc_library_headers", "libc_headers") + `
cc_library {
    name: "fake-ld-android",
    srcs: ["ld_android.cpp"],
    cflags: [
        "-Wall",
        "-Wextra",
        "-Wunused",
        "-Werror",
    ],
    header_libs: ["libc_headers"],
    ldflags: [
        "-Wl,--exclude-libs=libgcc.a",
        "-Wl,--exclude-libs=libgcc_stripped.a",
        "-Wl,--exclude-libs=libclang_rt.builtins-arm-android.a",
        "-Wl,--exclude-libs=libclang_rt.builtins-aarch64-android.a",
        "-Wl,--exclude-libs=libclang_rt.builtins-i686-android.a",
        "-Wl,--exclude-libs=libclang_rt.builtins-x86_64-android.a",
    ],
    arch: {
        x86: {
            ldflags: ["-Wl,--exclude-libs=libgcc_eh.a"],
        },
        x86_64: {
            ldflags: ["-Wl,--exclude-libs=libgcc_eh.a"],
        },
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("fake-ld-android", attrNameToString{
			"srcs": `["ld_android.cpp"]`,
			"copts": `[
        "-Wall",
        "-Wextra",
        "-Wunused",
        "-Werror",
    ]`,
			"implementation_deps": `[":libc_headers"]`,
			"linkopts": `[
        "-Wl,--exclude-libs=libgcc.a",
        "-Wl,--exclude-libs=libgcc_stripped.a",
        "-Wl,--exclude-libs=libclang_rt.builtins-arm-android.a",
        "-Wl,--exclude-libs=libclang_rt.builtins-aarch64-android.a",
        "-Wl,--exclude-libs=libclang_rt.builtins-i686-android.a",
        "-Wl,--exclude-libs=libclang_rt.builtins-x86_64-android.a",
    ] + select({
        "//build/bazel/platforms/arch:x86": ["-Wl,--exclude-libs=libgcc_eh.a"],
        "//build/bazel/platforms/arch:x86_64": ["-Wl,--exclude-libs=libgcc_eh.a"],
        "//conditions:default": [],
    })`,
		}),
	})
}

func TestCcLibraryExcludeSrcs(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library exclude_srcs - trimmed example of //external/arm-optimized-routines:libarm-optimized-routines-math",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		dir:                        "external",
		filesystem: map[string]string{
			"external/math/cosf.c":      "",
			"external/math/erf.c":       "",
			"external/math/erf_data.c":  "",
			"external/math/erff.c":      "",
			"external/math/erff_data.c": "",
			"external/Android.bp": `
cc_library {
    name: "fake-libarm-optimized-routines-math",
    exclude_srcs: [
        // Provided by:
        // bionic/libm/upstream-freebsd/lib/msun/src/s_erf.c
        // bionic/libm/upstream-freebsd/lib/msun/src/s_erff.c
        "math/erf.c",
        "math/erf_data.c",
        "math/erff.c",
        "math/erff_data.c",
    ],
    srcs: [
        "math/*.c",
    ],
    // arch-specific settings
    arch: {
        arm64: {
            cflags: [
                "-DHAVE_FAST_FMA=1",
            ],
        },
    },
    bazel_module: { bp2build_available: true },
}
`,
		},
		blueprint: soongCcLibraryPreamble,
		expectedBazelTargets: makeCcLibraryTargets("fake-libarm-optimized-routines-math", attrNameToString{
			"copts": `select({
        "//build/bazel/platforms/arch:arm64": ["-DHAVE_FAST_FMA=1"],
        "//conditions:default": [],
    })`,
			"local_includes": `["."]`,
			"srcs_c":         `["math/cosf.c"]`,
		}),
	})
}

func TestCcLibrarySharedStaticProps(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library shared/static props",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		filesystem: map[string]string{
			"both.cpp":       "",
			"sharedonly.cpp": "",
			"staticonly.cpp": "",
		},
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "a",
    srcs: ["both.cpp"],
    cflags: ["bothflag"],
    shared_libs: ["shared_dep_for_both"],
    static_libs: ["static_dep_for_both", "whole_and_static_lib_for_both"],
    whole_static_libs: ["whole_static_lib_for_both", "whole_and_static_lib_for_both"],
    static: {
        srcs: ["staticonly.cpp"],
        cflags: ["staticflag"],
        shared_libs: ["shared_dep_for_static"],
        static_libs: ["static_dep_for_static"],
        whole_static_libs: ["whole_static_lib_for_static"],
    },
    shared: {
        srcs: ["sharedonly.cpp"],
        cflags: ["sharedflag"],
        shared_libs: ["shared_dep_for_shared"],
        static_libs: ["static_dep_for_shared"],
        whole_static_libs: ["whole_static_lib_for_shared"],
    },
    include_build_directory: false,
}

cc_library_static {
    name: "static_dep_for_shared",
    bazel_module: { bp2build_available: false },
}

cc_library_static {
    name: "static_dep_for_static",
    bazel_module: { bp2build_available: false },
}

cc_library_static {
    name: "static_dep_for_both",
    bazel_module: { bp2build_available: false },
}

cc_library_static {
    name: "whole_static_lib_for_shared",
    bazel_module: { bp2build_available: false },
}

cc_library_static {
    name: "whole_static_lib_for_static",
    bazel_module: { bp2build_available: false },
}

cc_library_static {
    name: "whole_static_lib_for_both",
    bazel_module: { bp2build_available: false },
}

cc_library_static {
    name: "whole_and_static_lib_for_both",
    bazel_module: { bp2build_available: false },
}

cc_library {
    name: "shared_dep_for_shared",
    bazel_module: { bp2build_available: false },
}

cc_library {
    name: "shared_dep_for_static",
    bazel_module: { bp2build_available: false },
}

cc_library {
    name: "shared_dep_for_both",
    bazel_module: { bp2build_available: false },
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "a_bp2build_cc_library_static", attrNameToString{
				"copts": `[
        "bothflag",
        "staticflag",
    ]`,
				"implementation_deps": `[
        ":static_dep_for_both",
        ":static_dep_for_static",
    ]`,
				"implementation_dynamic_deps": `[
        ":shared_dep_for_both",
        ":shared_dep_for_static",
    ]`,
				"srcs": `[
        "both.cpp",
        "staticonly.cpp",
    ]`,
				"whole_archive_deps": `[
        ":whole_static_lib_for_both",
        ":whole_and_static_lib_for_both",
        ":whole_static_lib_for_static",
    ]`}),
			makeBazelTarget("cc_library_shared", "a", attrNameToString{
				"copts": `[
        "bothflag",
        "sharedflag",
    ]`,
				"implementation_deps": `[
        ":static_dep_for_both",
        ":static_dep_for_shared",
    ]`,
				"implementation_dynamic_deps": `[
        ":shared_dep_for_both",
        ":shared_dep_for_shared",
    ]`,
				"srcs": `[
        "both.cpp",
        "sharedonly.cpp",
    ]`,
				"whole_archive_deps": `[
        ":whole_static_lib_for_both",
        ":whole_and_static_lib_for_both",
        ":whole_static_lib_for_shared",
    ]`,
			}),
		},
	})
}

func TestCcLibraryDeps(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library shared/static props",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		filesystem: map[string]string{
			"both.cpp":       "",
			"sharedonly.cpp": "",
			"staticonly.cpp": "",
		},
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "a",
    srcs: ["both.cpp"],
    cflags: ["bothflag"],
    shared_libs: ["implementation_shared_dep_for_both", "shared_dep_for_both"],
    export_shared_lib_headers: ["shared_dep_for_both"],
    static_libs: ["implementation_static_dep_for_both", "static_dep_for_both"],
    export_static_lib_headers: ["static_dep_for_both", "whole_static_dep_for_both"],
    whole_static_libs: ["not_explicitly_exported_whole_static_dep_for_both", "whole_static_dep_for_both"],
    static: {
        srcs: ["staticonly.cpp"],
        cflags: ["staticflag"],
        shared_libs: ["implementation_shared_dep_for_static", "shared_dep_for_static"],
        export_shared_lib_headers: ["shared_dep_for_static"],
        static_libs: ["implementation_static_dep_for_static", "static_dep_for_static"],
        export_static_lib_headers: ["static_dep_for_static", "whole_static_dep_for_static"],
        whole_static_libs: ["not_explicitly_exported_whole_static_dep_for_static", "whole_static_dep_for_static"],
    },
    shared: {
        srcs: ["sharedonly.cpp"],
        cflags: ["sharedflag"],
        shared_libs: ["implementation_shared_dep_for_shared", "shared_dep_for_shared"],
        export_shared_lib_headers: ["shared_dep_for_shared"],
        static_libs: ["implementation_static_dep_for_shared", "static_dep_for_shared"],
        export_static_lib_headers: ["static_dep_for_shared", "whole_static_dep_for_shared"],
        whole_static_libs: ["not_explicitly_exported_whole_static_dep_for_shared", "whole_static_dep_for_shared"],
    },
    include_build_directory: false,
}
` + simpleModuleDoNotConvertBp2build("cc_library_static", "static_dep_for_shared") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "implementation_static_dep_for_shared") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "static_dep_for_static") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "implementation_static_dep_for_static") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "static_dep_for_both") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "implementation_static_dep_for_both") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "whole_static_dep_for_shared") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "not_explicitly_exported_whole_static_dep_for_shared") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "whole_static_dep_for_static") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "not_explicitly_exported_whole_static_dep_for_static") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "whole_static_dep_for_both") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "not_explicitly_exported_whole_static_dep_for_both") +
			simpleModuleDoNotConvertBp2build("cc_library", "shared_dep_for_shared") +
			simpleModuleDoNotConvertBp2build("cc_library", "implementation_shared_dep_for_shared") +
			simpleModuleDoNotConvertBp2build("cc_library", "shared_dep_for_static") +
			simpleModuleDoNotConvertBp2build("cc_library", "implementation_shared_dep_for_static") +
			simpleModuleDoNotConvertBp2build("cc_library", "shared_dep_for_both") +
			simpleModuleDoNotConvertBp2build("cc_library", "implementation_shared_dep_for_both"),
		expectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "a_bp2build_cc_library_static", attrNameToString{
				"copts": `[
        "bothflag",
        "staticflag",
    ]`,
				"deps": `[
        ":static_dep_for_both",
        ":static_dep_for_static",
    ]`,
				"dynamic_deps": `[
        ":shared_dep_for_both",
        ":shared_dep_for_static",
    ]`,
				"implementation_deps": `[
        ":implementation_static_dep_for_both",
        ":implementation_static_dep_for_static",
    ]`,
				"implementation_dynamic_deps": `[
        ":implementation_shared_dep_for_both",
        ":implementation_shared_dep_for_static",
    ]`,
				"srcs": `[
        "both.cpp",
        "staticonly.cpp",
    ]`,
				"whole_archive_deps": `[
        ":not_explicitly_exported_whole_static_dep_for_both",
        ":whole_static_dep_for_both",
        ":not_explicitly_exported_whole_static_dep_for_static",
        ":whole_static_dep_for_static",
    ]`,
			}),
			makeBazelTarget("cc_library_shared", "a", attrNameToString{
				"copts": `[
        "bothflag",
        "sharedflag",
    ]`,
				"deps": `[
        ":static_dep_for_both",
        ":static_dep_for_shared",
    ]`,
				"dynamic_deps": `[
        ":shared_dep_for_both",
        ":shared_dep_for_shared",
    ]`,
				"implementation_deps": `[
        ":implementation_static_dep_for_both",
        ":implementation_static_dep_for_shared",
    ]`,
				"implementation_dynamic_deps": `[
        ":implementation_shared_dep_for_both",
        ":implementation_shared_dep_for_shared",
    ]`,
				"srcs": `[
        "both.cpp",
        "sharedonly.cpp",
    ]`,
				"whole_archive_deps": `[
        ":not_explicitly_exported_whole_static_dep_for_both",
        ":whole_static_dep_for_both",
        ":not_explicitly_exported_whole_static_dep_for_shared",
        ":whole_static_dep_for_shared",
    ]`,
			})},
	},
	)
}

func TestCcLibraryWholeStaticLibsAlwaysLink(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		dir:                        "foo/bar",
		filesystem: map[string]string{
			"foo/bar/Android.bp": `
cc_library {
    name: "a",
    whole_static_libs: ["whole_static_lib_for_both"],
    static: {
        whole_static_libs: ["whole_static_lib_for_static"],
    },
    shared: {
        whole_static_libs: ["whole_static_lib_for_shared"],
    },
    bazel_module: { bp2build_available: true },
    include_build_directory: false,
}

cc_prebuilt_library_static { name: "whole_static_lib_for_shared" }

cc_prebuilt_library_static { name: "whole_static_lib_for_static" }

cc_prebuilt_library_static { name: "whole_static_lib_for_both" }
`,
		},
		blueprint: soongCcLibraryPreamble,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "a_bp2build_cc_library_static", attrNameToString{
				"whole_archive_deps": `[
        ":whole_static_lib_for_both_alwayslink",
        ":whole_static_lib_for_static_alwayslink",
    ]`,
			}),
			makeBazelTarget("cc_library_shared", "a", attrNameToString{
				"whole_archive_deps": `[
        ":whole_static_lib_for_both_alwayslink",
        ":whole_static_lib_for_shared_alwayslink",
    ]`,
			}),
		},
	},
	)
}

func TestCcLibrarySharedStaticPropsInArch(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library shared/static props in arch",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		dir:                        "foo/bar",
		filesystem: map[string]string{
			"foo/bar/arm.cpp":        "",
			"foo/bar/x86.cpp":        "",
			"foo/bar/sharedonly.cpp": "",
			"foo/bar/staticonly.cpp": "",
			"foo/bar/Android.bp": `
cc_library {
    name: "a",
    arch: {
        arm: {
            shared: {
                srcs: ["arm_shared.cpp"],
                cflags: ["-DARM_SHARED"],
                static_libs: ["arm_static_dep_for_shared"],
                whole_static_libs: ["arm_whole_static_dep_for_shared"],
                shared_libs: ["arm_shared_dep_for_shared"],
            },
        },
        x86: {
            static: {
                srcs: ["x86_static.cpp"],
                cflags: ["-DX86_STATIC"],
                static_libs: ["x86_dep_for_static"],
            },
        },
    },
    target: {
        android: {
            shared: {
                srcs: ["android_shared.cpp"],
                cflags: ["-DANDROID_SHARED"],
                static_libs: ["android_dep_for_shared"],
            },
        },
        android_arm: {
            shared: {
                cflags: ["-DANDROID_ARM_SHARED"],
            },
        },
    },
    srcs: ["both.cpp"],
    cflags: ["bothflag"],
    static_libs: ["static_dep_for_both"],
    static: {
        srcs: ["staticonly.cpp"],
        cflags: ["staticflag"],
        static_libs: ["static_dep_for_static"],
    },
    shared: {
        srcs: ["sharedonly.cpp"],
        cflags: ["sharedflag"],
        static_libs: ["static_dep_for_shared"],
    },
    bazel_module: { bp2build_available: true },
}

cc_library_static { name: "static_dep_for_shared" }
cc_library_static { name: "static_dep_for_static" }
cc_library_static { name: "static_dep_for_both" }

cc_library_static { name: "arm_static_dep_for_shared" }
cc_library_static { name: "arm_whole_static_dep_for_shared" }
cc_library_static { name: "arm_shared_dep_for_shared" }

cc_library_static { name: "x86_dep_for_static" }

cc_library_static { name: "android_dep_for_shared" }
`,
		},
		blueprint: soongCcLibraryPreamble,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "a_bp2build_cc_library_static", attrNameToString{
				"copts": `[
        "bothflag",
        "staticflag",
    ] + select({
        "//build/bazel/platforms/arch:x86": ["-DX86_STATIC"],
        "//conditions:default": [],
    })`,
				"implementation_deps": `[
        ":static_dep_for_both",
        ":static_dep_for_static",
    ] + select({
        "//build/bazel/platforms/arch:x86": [":x86_dep_for_static"],
        "//conditions:default": [],
    })`,
				"local_includes": `["."]`,
				"srcs": `[
        "both.cpp",
        "staticonly.cpp",
    ] + select({
        "//build/bazel/platforms/arch:x86": ["x86_static.cpp"],
        "//conditions:default": [],
    })`,
			}),
			makeBazelTarget("cc_library_shared", "a", attrNameToString{
				"copts": `[
        "bothflag",
        "sharedflag",
    ] + select({
        "//build/bazel/platforms/arch:arm": ["-DARM_SHARED"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel/platforms/os:android": ["-DANDROID_SHARED"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel/platforms/os_arch:android_arm": ["-DANDROID_ARM_SHARED"],
        "//conditions:default": [],
    })`,
				"implementation_deps": `[
        ":static_dep_for_both",
        ":static_dep_for_shared",
    ] + select({
        "//build/bazel/platforms/arch:arm": [":arm_static_dep_for_shared"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel/platforms/os:android": [":android_dep_for_shared"],
        "//conditions:default": [],
    })`,
				"implementation_dynamic_deps": `select({
        "//build/bazel/platforms/arch:arm": [":arm_shared_dep_for_shared"],
        "//conditions:default": [],
    })`,
				"local_includes": `["."]`,
				"srcs": `[
        "both.cpp",
        "sharedonly.cpp",
    ] + select({
        "//build/bazel/platforms/arch:arm": ["arm_shared.cpp"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel/platforms/os:android": ["android_shared.cpp"],
        "//conditions:default": [],
    })`,
				"whole_archive_deps": `select({
        "//build/bazel/platforms/arch:arm": [":arm_whole_static_dep_for_shared"],
        "//conditions:default": [],
    })`,
			}),
		},
	},
	)
}

func TestCcLibrarySharedStaticPropsWithMixedSources(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library shared/static props with c/cpp/s mixed sources",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		dir:                        "foo/bar",
		filesystem: map[string]string{
			"foo/bar/both_source.cpp":   "",
			"foo/bar/both_source.cc":    "",
			"foo/bar/both_source.c":     "",
			"foo/bar/both_source.s":     "",
			"foo/bar/both_source.S":     "",
			"foo/bar/shared_source.cpp": "",
			"foo/bar/shared_source.cc":  "",
			"foo/bar/shared_source.c":   "",
			"foo/bar/shared_source.s":   "",
			"foo/bar/shared_source.S":   "",
			"foo/bar/static_source.cpp": "",
			"foo/bar/static_source.cc":  "",
			"foo/bar/static_source.c":   "",
			"foo/bar/static_source.s":   "",
			"foo/bar/static_source.S":   "",
			"foo/bar/Android.bp": `
cc_library {
    name: "a",
    srcs: [
    "both_source.cpp",
    "both_source.cc",
    "both_source.c",
    "both_source.s",
    "both_source.S",
    ":both_filegroup",
  ],
    static: {
        srcs: [
          "static_source.cpp",
          "static_source.cc",
          "static_source.c",
          "static_source.s",
          "static_source.S",
          ":static_filegroup",
        ],
    },
    shared: {
        srcs: [
          "shared_source.cpp",
          "shared_source.cc",
          "shared_source.c",
          "shared_source.s",
          "shared_source.S",
          ":shared_filegroup",
        ],
    },
    bazel_module: { bp2build_available: true },
}

filegroup {
    name: "both_filegroup",
    srcs: [
        // Not relevant, handled by filegroup macro
  ],
}

filegroup {
    name: "shared_filegroup",
    srcs: [
        // Not relevant, handled by filegroup macro
  ],
}

filegroup {
    name: "static_filegroup",
    srcs: [
        // Not relevant, handled by filegroup macro
  ],
}
`,
		},
		blueprint: soongCcLibraryPreamble,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "a_bp2build_cc_library_static", attrNameToString{
				"local_includes": `["."]`,
				"srcs": `[
        "both_source.cpp",
        "both_source.cc",
        ":both_filegroup_cpp_srcs",
        "static_source.cpp",
        "static_source.cc",
        ":static_filegroup_cpp_srcs",
    ]`,
				"srcs_as": `[
        "both_source.s",
        "both_source.S",
        ":both_filegroup_as_srcs",
        "static_source.s",
        "static_source.S",
        ":static_filegroup_as_srcs",
    ]`,
				"srcs_c": `[
        "both_source.c",
        ":both_filegroup_c_srcs",
        "static_source.c",
        ":static_filegroup_c_srcs",
    ]`,
			}),
			makeBazelTarget("cc_library_shared", "a", attrNameToString{
				"local_includes": `["."]`,
				"srcs": `[
        "both_source.cpp",
        "both_source.cc",
        ":both_filegroup_cpp_srcs",
        "shared_source.cpp",
        "shared_source.cc",
        ":shared_filegroup_cpp_srcs",
    ]`,
				"srcs_as": `[
        "both_source.s",
        "both_source.S",
        ":both_filegroup_as_srcs",
        "shared_source.s",
        "shared_source.S",
        ":shared_filegroup_as_srcs",
    ]`,
				"srcs_c": `[
        "both_source.c",
        ":both_filegroup_c_srcs",
        "shared_source.c",
        ":shared_filegroup_c_srcs",
    ]`,
			})}})
}

func TestCcLibraryNonConfiguredVersionScript(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library non-configured version script",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		dir:                        "foo/bar",
		filesystem: map[string]string{
			"foo/bar/Android.bp": `
cc_library {
    name: "a",
    srcs: ["a.cpp"],
    version_script: "v.map",
    bazel_module: { bp2build_available: true },
    include_build_directory: false,
}
`,
		},
		blueprint: soongCcLibraryPreamble,
		expectedBazelTargets: makeCcLibraryTargets("a", attrNameToString{
			"additional_linker_inputs": `["v.map"]`,
			"linkopts":                 `["-Wl,--version-script,$(location v.map)"]`,
			"srcs":                     `["a.cpp"]`,
		}),
	},
	)
}

func TestCcLibraryConfiguredVersionScript(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library configured version script",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		dir:                        "foo/bar",
		filesystem: map[string]string{
			"foo/bar/Android.bp": `
cc_library {
   name: "a",
   srcs: ["a.cpp"],
   arch: {
     arm: {
       version_script: "arm.map",
     },
     arm64: {
       version_script: "arm64.map",
     },
   },

   bazel_module: { bp2build_available: true },
    include_build_directory: false,
}
    `,
		},
		blueprint: soongCcLibraryPreamble,
		expectedBazelTargets: makeCcLibraryTargets("a", attrNameToString{
			"additional_linker_inputs": `select({
        "//build/bazel/platforms/arch:arm": ["arm.map"],
        "//build/bazel/platforms/arch:arm64": ["arm64.map"],
        "//conditions:default": [],
    })`,
			"linkopts": `select({
        "//build/bazel/platforms/arch:arm": ["-Wl,--version-script,$(location arm.map)"],
        "//build/bazel/platforms/arch:arm64": ["-Wl,--version-script,$(location arm64.map)"],
        "//conditions:default": [],
    })`,
			"srcs": `["a.cpp"]`,
		}),
	},
	)
}

func TestCcLibrarySharedLibs(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library shared_libs",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "mylib",
    bazel_module: { bp2build_available: false },
}

cc_library {
    name: "a",
    shared_libs: ["mylib",],
    include_build_directory: false,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("a", attrNameToString{
			"implementation_dynamic_deps": `[":mylib"]`,
		}),
	},
	)
}

func TestCcLibraryFeatures(t *testing.T) {
	expected_targets := []string{}
	expected_targets = append(expected_targets, makeCcLibraryTargets("a", attrNameToString{
		"features": `[
        "disable_pack_relocations",
        "-no_undefined_symbols",
    ]`,
		"srcs": `["a.cpp"]`,
	})...)
	expected_targets = append(expected_targets, makeCcLibraryTargets("b", attrNameToString{
		"features": `select({
        "//build/bazel/platforms/arch:x86_64": [
            "disable_pack_relocations",
            "-no_undefined_symbols",
        ],
        "//conditions:default": [],
    })`,
		"srcs": `["b.cpp"]`,
	})...)
	expected_targets = append(expected_targets, makeCcLibraryTargets("c", attrNameToString{
		"features": `select({
        "//build/bazel/platforms/os:darwin": [
            "disable_pack_relocations",
            "-no_undefined_symbols",
        ],
        "//conditions:default": [],
    })`,
		"srcs": `["c.cpp"]`,
	})...)

	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library pack_relocations test",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "a",
    srcs: ["a.cpp"],
    pack_relocations: false,
    allow_undefined_symbols: true,
    include_build_directory: false,
}

cc_library {
    name: "b",
    srcs: ["b.cpp"],
    arch: {
        x86_64: {
            pack_relocations: false,
            allow_undefined_symbols: true,
        },
    },
    include_build_directory: false,
}

cc_library {
    name: "c",
    srcs: ["c.cpp"],
    target: {
        darwin: {
            pack_relocations: false,
            allow_undefined_symbols: true,
        },
    },
    include_build_directory: false,
}`,
		expectedBazelTargets: expected_targets,
	})
}

func TestCcLibrarySpacesInCopts(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library spaces in copts",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "a",
    cflags: ["-include header.h",],
    include_build_directory: false,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("a", attrNameToString{
			"copts": `[
        "-include",
        "header.h",
    ]`,
		}),
	},
	)
}

func TestCcLibraryCppFlagsGoesIntoCopts(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library cppflags usage",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcLibraryPreamble + `cc_library {
    name: "a",
    srcs: ["a.cpp"],
    cflags: ["-Wall"],
    cppflags: [
        "-fsigned-char",
        "-pedantic",
    ],
    arch: {
        arm64: {
            cppflags: ["-DARM64=1"],
        },
    },
    target: {
        android: {
            cppflags: ["-DANDROID=1"],
        },
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("a", attrNameToString{
			"copts": `["-Wall"]`,
			"cppflags": `[
        "-fsigned-char",
        "-pedantic",
    ] + select({
        "//build/bazel/platforms/arch:arm64": ["-DARM64=1"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel/platforms/os:android": ["-DANDROID=1"],
        "//conditions:default": [],
    })`,
			"srcs": `["a.cpp"]`,
		}),
	},
	)
}

func TestCcLibraryExcludeLibs(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		filesystem:                 map[string]string{},
		blueprint: soongCcLibraryStaticPreamble + `
cc_library {
    name: "foo_static",
    srcs: ["common.c"],
    whole_static_libs: [
        "arm_whole_static_lib_excludes",
        "malloc_not_svelte_whole_static_lib_excludes"
    ],
    static_libs: [
        "arm_static_lib_excludes",
        "malloc_not_svelte_static_lib_excludes"
    ],
    shared_libs: [
        "arm_shared_lib_excludes",
    ],
    arch: {
        arm: {
            exclude_shared_libs: [
                 "arm_shared_lib_excludes",
            ],
            exclude_static_libs: [
                "arm_static_lib_excludes",
                "arm_whole_static_lib_excludes",
            ],
        },
    },
    product_variables: {
        malloc_not_svelte: {
            shared_libs: ["malloc_not_svelte_shared_lib"],
            whole_static_libs: ["malloc_not_svelte_whole_static_lib"],
            exclude_static_libs: [
                "malloc_not_svelte_static_lib_excludes",
                "malloc_not_svelte_whole_static_lib_excludes",
            ],
        },
    },
    include_build_directory: false,
}

cc_library {
    name: "arm_whole_static_lib_excludes",
    bazel_module: { bp2build_available: false },
}

cc_library {
    name: "malloc_not_svelte_whole_static_lib",
    bazel_module: { bp2build_available: false },
}

cc_library {
    name: "malloc_not_svelte_whole_static_lib_excludes",
    bazel_module: { bp2build_available: false },
}

cc_library {
    name: "arm_static_lib_excludes",
    bazel_module: { bp2build_available: false },
}

cc_library {
    name: "malloc_not_svelte_static_lib_excludes",
    bazel_module: { bp2build_available: false },
}

cc_library {
    name: "arm_shared_lib_excludes",
    bazel_module: { bp2build_available: false },
}

cc_library {
    name: "malloc_not_svelte_shared_lib",
    bazel_module: { bp2build_available: false },
}
`,
		expectedBazelTargets: makeCcLibraryTargets("foo_static", attrNameToString{
			"implementation_deps": `select({
        "//build/bazel/platforms/arch:arm": [],
        "//conditions:default": [":arm_static_lib_excludes_bp2build_cc_library_static"],
    }) + select({
        "//build/bazel/product_variables:malloc_not_svelte": [],
        "//conditions:default": [":malloc_not_svelte_static_lib_excludes_bp2build_cc_library_static"],
    })`,
			"implementation_dynamic_deps": `select({
        "//build/bazel/platforms/arch:arm": [],
        "//conditions:default": [":arm_shared_lib_excludes"],
    }) + select({
        "//build/bazel/product_variables:malloc_not_svelte": [":malloc_not_svelte_shared_lib"],
        "//conditions:default": [],
    })`,
			"srcs_c": `["common.c"]`,
			"whole_archive_deps": `select({
        "//build/bazel/platforms/arch:arm": [],
        "//conditions:default": [":arm_whole_static_lib_excludes_bp2build_cc_library_static"],
    }) + select({
        "//build/bazel/product_variables:malloc_not_svelte": [":malloc_not_svelte_whole_static_lib_bp2build_cc_library_static"],
        "//conditions:default": [":malloc_not_svelte_whole_static_lib_excludes_bp2build_cc_library_static"],
    })`,
		}),
	},
	)
}

func TestCCLibraryNoCrtTrue(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library - nocrt: true emits attribute",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		filesystem: map[string]string{
			"impl.cpp": "",
		},
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "foo-lib",
    srcs: ["impl.cpp"],
    nocrt: true,
    include_build_directory: false,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("foo-lib", attrNameToString{
			"link_crt": `False`,
			"srcs":     `["impl.cpp"]`,
		}),
	},
	)
}

func TestCCLibraryNoCrtFalse(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library - nocrt: false - does not emit attribute",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		filesystem: map[string]string{
			"impl.cpp": "",
		},
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "foo-lib",
    srcs: ["impl.cpp"],
    nocrt: false,
    include_build_directory: false,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("foo-lib", attrNameToString{
			"srcs": `["impl.cpp"]`,
		}),
	})
}

func TestCCLibraryNoCrtArchVariant(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library - nocrt in select",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		filesystem: map[string]string{
			"impl.cpp": "",
		},
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "foo-lib",
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
		expectedErr: fmt.Errorf("module \"foo-lib\": nocrt is not supported for arch variants"),
	})
}

func TestCCLibraryNoLibCrtTrue(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		filesystem: map[string]string{
			"impl.cpp": "",
		},
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "foo-lib",
    srcs: ["impl.cpp"],
    no_libcrt: true,
    include_build_directory: false,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("foo-lib", attrNameToString{
			"srcs":       `["impl.cpp"]`,
			"use_libcrt": `False`,
		}),
	})
}

func makeCcLibraryTargets(name string, attrs attrNameToString) []string {
	STATIC_ONLY_ATTRS := map[string]bool{}
	SHARED_ONLY_ATTRS := map[string]bool{
		"link_crt":                 true,
		"additional_linker_inputs": true,
		"linkopts":                 true,
		"strip":                    true,
		"stubs_symbol_file":        true,
		"stubs_versions":           true,
		"inject_bssl_hash":         true,
	}
	sharedAttrs := attrNameToString{}
	staticAttrs := attrNameToString{}
	for key, val := range attrs {
		if _, staticOnly := STATIC_ONLY_ATTRS[key]; !staticOnly {
			sharedAttrs[key] = val
		}
		if _, sharedOnly := SHARED_ONLY_ATTRS[key]; !sharedOnly {
			staticAttrs[key] = val
		}
	}
	sharedTarget := makeBazelTarget("cc_library_shared", name, sharedAttrs)
	staticTarget := makeBazelTarget("cc_library_static", name+"_bp2build_cc_library_static", staticAttrs)

	return []string{staticTarget, sharedTarget}
}

func TestCCLibraryNoLibCrtFalse(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		filesystem: map[string]string{
			"impl.cpp": "",
		},
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "foo-lib",
    srcs: ["impl.cpp"],
    no_libcrt: false,
    include_build_directory: false,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("foo-lib", attrNameToString{
			"srcs":       `["impl.cpp"]`,
			"use_libcrt": `True`,
		}),
	})
}

func TestCCLibraryNoLibCrtArchVariant(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		filesystem: map[string]string{
			"impl.cpp": "",
		},
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "foo-lib",
    srcs: ["impl.cpp"],
    arch: {
        arm: {
            no_libcrt: true,
        },
        x86: {
            no_libcrt: true,
        },
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("foo-lib", attrNameToString{
			"srcs": `["impl.cpp"]`,
			"use_libcrt": `select({
        "//build/bazel/platforms/arch:arm": False,
        "//build/bazel/platforms/arch:x86": False,
        "//conditions:default": None,
    })`,
		}),
	})
}

func TestCCLibraryNoLibCrtArchAndTargetVariant(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		filesystem: map[string]string{
			"impl.cpp": "",
		},
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "foo-lib",
    srcs: ["impl.cpp"],
    arch: {
        arm: {
            no_libcrt: true,
        },
        x86: {
            no_libcrt: true,
        },
    },
    target: {
        darwin: {
            no_libcrt: true,
        }
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("foo-lib", attrNameToString{
			"srcs": `["impl.cpp"]`,
			"use_libcrt": `select({
        "//build/bazel/platforms/os_arch:android_arm": False,
        "//build/bazel/platforms/os_arch:android_x86": False,
        "//build/bazel/platforms/os_arch:darwin_arm64": False,
        "//build/bazel/platforms/os_arch:darwin_x86_64": False,
        "//build/bazel/platforms/os_arch:linux_glibc_x86": False,
        "//build/bazel/platforms/os_arch:linux_musl_x86": False,
        "//build/bazel/platforms/os_arch:windows_x86": False,
        "//conditions:default": None,
    })`,
		}),
	})
}

func TestCCLibraryNoLibCrtArchAndTargetVariantConflict(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		filesystem: map[string]string{
			"impl.cpp": "",
		},
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "foo-lib",
    srcs: ["impl.cpp"],
    arch: {
        arm: {
            no_libcrt: true,
        },
        // This is expected to override the value for darwin_x86_64.
        x86_64: {
            no_libcrt: true,
        },
    },
    target: {
        darwin: {
            no_libcrt: false,
        }
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("foo-lib", attrNameToString{
			"srcs": `["impl.cpp"]`,
			"use_libcrt": `select({
        "//build/bazel/platforms/os_arch:android_arm": False,
        "//build/bazel/platforms/os_arch:android_x86_64": False,
        "//build/bazel/platforms/os_arch:darwin_arm64": True,
        "//build/bazel/platforms/os_arch:darwin_x86_64": False,
        "//build/bazel/platforms/os_arch:linux_bionic_x86_64": False,
        "//build/bazel/platforms/os_arch:linux_glibc_x86_64": False,
        "//build/bazel/platforms/os_arch:linux_musl_x86_64": False,
        "//build/bazel/platforms/os_arch:windows_x86_64": False,
        "//conditions:default": None,
    })`,
		}),
	})
}

func TestCcLibraryStrip(t *testing.T) {
	expectedTargets := []string{}
	expectedTargets = append(expectedTargets, makeCcLibraryTargets("all", attrNameToString{
		"strip": `{
        "all": True,
    }`,
	})...)
	expectedTargets = append(expectedTargets, makeCcLibraryTargets("keep_symbols", attrNameToString{
		"strip": `{
        "keep_symbols": True,
    }`,
	})...)
	expectedTargets = append(expectedTargets, makeCcLibraryTargets("keep_symbols_and_debug_frame", attrNameToString{
		"strip": `{
        "keep_symbols_and_debug_frame": True,
    }`,
	})...)
	expectedTargets = append(expectedTargets, makeCcLibraryTargets("keep_symbols_list", attrNameToString{
		"strip": `{
        "keep_symbols_list": ["symbol"],
    }`,
	})...)
	expectedTargets = append(expectedTargets, makeCcLibraryTargets("none", attrNameToString{
		"strip": `{
        "none": True,
    }`,
	})...)
	expectedTargets = append(expectedTargets, makeCcLibraryTargets("nothing", attrNameToString{})...)

	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library strip args",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "nothing",
    include_build_directory: false,
}
cc_library {
    name: "keep_symbols",
    strip: {
        keep_symbols: true,
    },
    include_build_directory: false,
}
cc_library {
    name: "keep_symbols_and_debug_frame",
    strip: {
        keep_symbols_and_debug_frame: true,
    },
    include_build_directory: false,
}
cc_library {
    name: "none",
    strip: {
        none: true,
    },
    include_build_directory: false,
}
cc_library {
    name: "keep_symbols_list",
    strip: {
        keep_symbols_list: ["symbol"],
    },
    include_build_directory: false,
}
cc_library {
    name: "all",
    strip: {
        all: true,
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: expectedTargets,
	})
}

func TestCcLibraryStripWithArch(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library strip args",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "multi-arch",
    target: {
        darwin: {
            strip: {
                keep_symbols_list: ["foo", "bar"]
            }
        },
    },
    arch: {
        arm: {
            strip: {
                keep_symbols_and_debug_frame: true,
            },
        },
        arm64: {
            strip: {
                keep_symbols: true,
            },
        },
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("multi-arch", attrNameToString{
			"strip": `{
        "keep_symbols": select({
            "//build/bazel/platforms/arch:arm64": True,
            "//conditions:default": None,
        }),
        "keep_symbols_and_debug_frame": select({
            "//build/bazel/platforms/arch:arm": True,
            "//conditions:default": None,
        }),
        "keep_symbols_list": select({
            "//build/bazel/platforms/os:darwin": [
                "foo",
                "bar",
            ],
            "//conditions:default": [],
        }),
    }`,
		}),
	},
	)
}

func TestCcLibrary_SystemSharedLibsRootEmpty(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library system_shared_libs empty at root",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "root_empty",
    system_shared_libs: [],
    include_build_directory: false,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("root_empty", attrNameToString{
			"system_dynamic_deps": `[]`,
		}),
	},
	)
}

func TestCcLibrary_SystemSharedLibsStaticEmpty(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library system_shared_libs empty for static variant",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "static_empty",
    static: {
        system_shared_libs: [],
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "static_empty_bp2build_cc_library_static", attrNameToString{
				"system_dynamic_deps": "[]",
			}),
			makeBazelTarget("cc_library_shared", "static_empty", attrNameToString{}),
		},
	})
}

func TestCcLibrary_SystemSharedLibsSharedEmpty(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library system_shared_libs empty for shared variant",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "shared_empty",
    shared: {
        system_shared_libs: [],
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "shared_empty_bp2build_cc_library_static", attrNameToString{}),
			makeBazelTarget("cc_library_shared", "shared_empty", attrNameToString{
				"system_dynamic_deps": "[]",
			}),
		},
	})
}

func TestCcLibrary_SystemSharedLibsSharedBionicEmpty(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library system_shared_libs empty for shared, bionic variant",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "shared_empty",
    target: {
        bionic: {
            shared: {
                system_shared_libs: [],
            }
        }
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "shared_empty_bp2build_cc_library_static", attrNameToString{}),
			makeBazelTarget("cc_library_shared", "shared_empty", attrNameToString{
				"system_dynamic_deps": "[]",
			}),
		},
	})
}

func TestCcLibrary_SystemSharedLibsLinuxBionicEmpty(t *testing.T) {
	// Note that this behavior is technically incorrect (it's a simplification).
	// The correct behavior would be if bp2build wrote `system_dynamic_deps = []`
	// only for linux_bionic, but `android` had `["libc", "libdl", "libm"].
	// b/195791252 tracks the fix.
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library system_shared_libs empty for linux_bionic variant",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "target_linux_bionic_empty",
    target: {
        linux_bionic: {
            system_shared_libs: [],
        },
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("target_linux_bionic_empty", attrNameToString{
			"system_dynamic_deps": `[]`,
		}),
	},
	)
}

func TestCcLibrary_SystemSharedLibsBionicEmpty(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library system_shared_libs empty for bionic variant",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "target_bionic_empty",
    target: {
        bionic: {
            system_shared_libs: [],
        },
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("target_bionic_empty", attrNameToString{
			"system_dynamic_deps": `[]`,
		}),
	},
	)
}

func TestCcLibrary_SystemSharedLibsSharedAndRoot(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library system_shared_libs set for shared and root",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "libc",
    bazel_module: { bp2build_available: false },
}
cc_library {
    name: "libm",
    bazel_module: { bp2build_available: false },
}

cc_library {
    name: "foo",
    system_shared_libs: ["libc"],
    shared: {
        system_shared_libs: ["libm"],
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: []string{
			makeBazelTarget("cc_library_static", "foo_bp2build_cc_library_static", attrNameToString{
				"system_dynamic_deps": `[":libc"]`,
			}),
			makeBazelTarget("cc_library_shared", "foo", attrNameToString{
				"system_dynamic_deps": `[
        ":libc",
        ":libm",
    ]`,
			}),
		},
	})
}

func TestCcLibraryOsSelects(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library - selects for all os targets",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		filesystem:                 map[string]string{},
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "foo-lib",
    srcs: ["base.cpp"],
    target: {
        android: {
            srcs: ["android.cpp"],
        },
        linux: {
            srcs: ["linux.cpp"],
        },
        linux_glibc: {
            srcs: ["linux_glibc.cpp"],
        },
        darwin: {
            srcs: ["darwin.cpp"],
        },
        bionic: {
            srcs: ["bionic.cpp"],
        },
        linux_musl: {
            srcs: ["linux_musl.cpp"],
        },
        windows: {
            srcs: ["windows.cpp"],
        },
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("foo-lib", attrNameToString{
			"srcs": `["base.cpp"] + select({
        "//build/bazel/platforms/os:android": [
            "linux.cpp",
            "bionic.cpp",
            "android.cpp",
        ],
        "//build/bazel/platforms/os:darwin": ["darwin.cpp"],
        "//build/bazel/platforms/os:linux": [
            "linux.cpp",
            "linux_glibc.cpp",
        ],
        "//build/bazel/platforms/os:linux_bionic": [
            "linux.cpp",
            "bionic.cpp",
        ],
        "//build/bazel/platforms/os:linux_musl": [
            "linux.cpp",
            "linux_musl.cpp",
        ],
        "//build/bazel/platforms/os:windows": ["windows.cpp"],
        "//conditions:default": [],
    })`,
		}),
	},
	)
}

func TestLibcryptoHashInjection(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library - libcrypto hash injection",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		filesystem:                 map[string]string{},
		blueprint: soongCcLibraryPreamble + `
cc_library {
    name: "libcrypto",
    target: {
        android: {
            inject_bssl_hash: true,
        },
    },
    include_build_directory: false,
}
`,
		expectedBazelTargets: makeCcLibraryTargets("libcrypto", attrNameToString{
			"inject_bssl_hash": `select({
        "//build/bazel/platforms/os:android": True,
        "//conditions:default": None,
    })`,
		}),
	},
	)
}

func TestCcLibraryCppStdWithGnuExtensions_ConvertsToFeatureAttr(t *testing.T) {
	type testCase struct {
		cpp_std        string
		c_std          string
		gnu_extensions string
		bazel_cpp_std  string
		bazel_c_std    string
	}

	testCases := []testCase{
		// Existing usages of cpp_std in AOSP are:
		// experimental, c++11, c++17, c++2a, c++98, gnu++11, gnu++17
		//
		// not set, only emit if gnu_extensions is disabled. the default (gnu+17
		// is set in the toolchain.)
		{cpp_std: "", gnu_extensions: "", bazel_cpp_std: ""},
		{cpp_std: "", gnu_extensions: "false", bazel_cpp_std: "c++17", bazel_c_std: "c99"},
		{cpp_std: "", gnu_extensions: "true", bazel_cpp_std: ""},
		// experimental defaults to gnu++2a
		{cpp_std: "experimental", gnu_extensions: "", bazel_cpp_std: "gnu++2a"},
		{cpp_std: "experimental", gnu_extensions: "false", bazel_cpp_std: "c++2a", bazel_c_std: "c99"},
		{cpp_std: "experimental", gnu_extensions: "true", bazel_cpp_std: "gnu++2a"},
		// Explicitly setting a c++ std does not use replace gnu++ std even if
		// gnu_extensions is true.
		// "c++11",
		{cpp_std: "c++11", gnu_extensions: "", bazel_cpp_std: "c++11"},
		{cpp_std: "c++11", gnu_extensions: "false", bazel_cpp_std: "c++11", bazel_c_std: "c99"},
		{cpp_std: "c++11", gnu_extensions: "true", bazel_cpp_std: "c++11"},
		// "c++17",
		{cpp_std: "c++17", gnu_extensions: "", bazel_cpp_std: "c++17"},
		{cpp_std: "c++17", gnu_extensions: "false", bazel_cpp_std: "c++17", bazel_c_std: "c99"},
		{cpp_std: "c++17", gnu_extensions: "true", bazel_cpp_std: "c++17"},
		// "c++2a",
		{cpp_std: "c++2a", gnu_extensions: "", bazel_cpp_std: "c++2a"},
		{cpp_std: "c++2a", gnu_extensions: "false", bazel_cpp_std: "c++2a", bazel_c_std: "c99"},
		{cpp_std: "c++2a", gnu_extensions: "true", bazel_cpp_std: "c++2a"},
		// "c++98",
		{cpp_std: "c++98", gnu_extensions: "", bazel_cpp_std: "c++98"},
		{cpp_std: "c++98", gnu_extensions: "false", bazel_cpp_std: "c++98", bazel_c_std: "c99"},
		{cpp_std: "c++98", gnu_extensions: "true", bazel_cpp_std: "c++98"},
		// gnu++ is replaced with c++ if gnu_extensions is explicitly false.
		// "gnu++11",
		{cpp_std: "gnu++11", gnu_extensions: "", bazel_cpp_std: "gnu++11"},
		{cpp_std: "gnu++11", gnu_extensions: "false", bazel_cpp_std: "c++11", bazel_c_std: "c99"},
		{cpp_std: "gnu++11", gnu_extensions: "true", bazel_cpp_std: "gnu++11"},
		// "gnu++17",
		{cpp_std: "gnu++17", gnu_extensions: "", bazel_cpp_std: "gnu++17"},
		{cpp_std: "gnu++17", gnu_extensions: "false", bazel_cpp_std: "c++17", bazel_c_std: "c99"},
		{cpp_std: "gnu++17", gnu_extensions: "true", bazel_cpp_std: "gnu++17"},

		// some c_std test cases
		{c_std: "experimental", gnu_extensions: "", bazel_c_std: "gnu17"},
		{c_std: "experimental", gnu_extensions: "false", bazel_cpp_std: "c++17", bazel_c_std: "c17"},
		{c_std: "experimental", gnu_extensions: "true", bazel_c_std: "gnu17"},
		{c_std: "gnu11", cpp_std: "gnu++17", gnu_extensions: "", bazel_cpp_std: "gnu++17", bazel_c_std: "gnu11"},
		{c_std: "gnu11", cpp_std: "gnu++17", gnu_extensions: "false", bazel_cpp_std: "c++17", bazel_c_std: "c11"},
		{c_std: "gnu11", cpp_std: "gnu++17", gnu_extensions: "true", bazel_cpp_std: "gnu++17", bazel_c_std: "gnu11"},
	}
	for i, tc := range testCases {
		name_prefix := fmt.Sprintf("a_%v", i)
		cppStdProp := ""
		if tc.cpp_std != "" {
			cppStdProp = fmt.Sprintf("    cpp_std: \"%s\",", tc.cpp_std)
		}
		cStdProp := ""
		if tc.c_std != "" {
			cStdProp = fmt.Sprintf("    c_std: \"%s\",", tc.c_std)
		}
		gnuExtensionsProp := ""
		if tc.gnu_extensions != "" {
			gnuExtensionsProp = fmt.Sprintf("    gnu_extensions: %s,", tc.gnu_extensions)
		}
		attrs := attrNameToString{}
		if tc.bazel_cpp_std != "" {
			attrs["cpp_std"] = fmt.Sprintf(`"%s"`, tc.bazel_cpp_std)
		}
		if tc.bazel_c_std != "" {
			attrs["c_std"] = fmt.Sprintf(`"%s"`, tc.bazel_c_std)
		}

		runCcLibraryTestCase(t, bp2buildTestCase{
			description: fmt.Sprintf(
				"cc_library with cpp_std: %s and gnu_extensions: %s", tc.cpp_std, tc.gnu_extensions),
			moduleTypeUnderTest:        "cc_library",
			moduleTypeUnderTestFactory: cc.LibraryFactory,
			blueprint: soongCcLibraryPreamble + fmt.Sprintf(`
cc_library {
	name: "%s_full",
%s // cpp_std: *string
%s // c_std: *string
%s // gnu_extensions: *bool
	include_build_directory: false,
}
`, name_prefix, cppStdProp, cStdProp, gnuExtensionsProp),
			expectedBazelTargets: makeCcLibraryTargets(name_prefix+"_full", attrs),
		})

		runCcLibraryStaticTestCase(t, bp2buildTestCase{
			description: fmt.Sprintf(
				"cc_library_static with cpp_std: %s and gnu_extensions: %s", tc.cpp_std, tc.gnu_extensions),
			moduleTypeUnderTest:        "cc_library_static",
			moduleTypeUnderTestFactory: cc.LibraryStaticFactory,
			blueprint: soongCcLibraryPreamble + fmt.Sprintf(`
cc_library_static {
	name: "%s_static",
%s // cpp_std: *string
%s // c_std: *string
%s // gnu_extensions: *bool
	include_build_directory: false,
}
`, name_prefix, cppStdProp, cStdProp, gnuExtensionsProp),
			expectedBazelTargets: []string{
				makeBazelTarget("cc_library_static", name_prefix+"_static", attrs),
			},
		})

		runCcLibrarySharedTestCase(t, bp2buildTestCase{
			description: fmt.Sprintf(
				"cc_library_shared with cpp_std: %s and gnu_extensions: %s", tc.cpp_std, tc.gnu_extensions),
			moduleTypeUnderTest:        "cc_library_shared",
			moduleTypeUnderTestFactory: cc.LibrarySharedFactory,
			blueprint: soongCcLibraryPreamble + fmt.Sprintf(`
cc_library_shared {
	name: "%s_shared",
%s // cpp_std: *string
%s // c_std: *string
%s // gnu_extensions: *bool
	include_build_directory: false,
}
`, name_prefix, cppStdProp, cStdProp, gnuExtensionsProp),
			expectedBazelTargets: []string{
				makeBazelTarget("cc_library_shared", name_prefix+"_shared", attrs),
			},
		})
	}
}

func TestCcLibraryProtoSimple(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcProtoPreamble + `cc_library {
	name: "foo",
	srcs: ["foo.proto"],
	include_build_directory: false,
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("proto_library", "foo_proto", attrNameToString{
				"srcs": `["foo.proto"]`,
			}), makeBazelTarget("cc_lite_proto_library", "foo_cc_proto_lite", attrNameToString{
				"deps": `[":foo_proto"]`,
			}), makeBazelTarget("cc_library_static", "foo_bp2build_cc_library_static", attrNameToString{
				"implementation_whole_archive_deps": `[":foo_cc_proto_lite"]`,
				"deps":                              `[":libprotobuf-cpp-lite"]`,
			}), makeBazelTarget("cc_library_shared", "foo", attrNameToString{
				"dynamic_deps": `[":libprotobuf-cpp-lite"]`,
			}),
		},
	})
}

func TestCcLibraryProtoNoCanonicalPathFromRoot(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcProtoPreamble + `cc_library {
	name: "foo",
	srcs: ["foo.proto"],
	proto: { canonical_path_from_root: false},
	include_build_directory: false,
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("proto_library", "foo_proto", attrNameToString{
				"srcs":                `["foo.proto"]`,
				"strip_import_prefix": `""`,
			}), makeBazelTarget("cc_lite_proto_library", "foo_cc_proto_lite", attrNameToString{
				"deps": `[":foo_proto"]`,
			}), makeBazelTarget("cc_library_static", "foo_bp2build_cc_library_static", attrNameToString{
				"implementation_whole_archive_deps": `[":foo_cc_proto_lite"]`,
				"deps":                              `[":libprotobuf-cpp-lite"]`,
			}), makeBazelTarget("cc_library_shared", "foo", attrNameToString{
				"dynamic_deps": `[":libprotobuf-cpp-lite"]`,
			}),
		},
	})
}

func TestCcLibraryProtoExplicitCanonicalPathFromRoot(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcProtoPreamble + `cc_library {
	name: "foo",
	srcs: ["foo.proto"],
	proto: { canonical_path_from_root: true},
	include_build_directory: false,
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("proto_library", "foo_proto", attrNameToString{
				"srcs": `["foo.proto"]`,
			}), makeBazelTarget("cc_lite_proto_library", "foo_cc_proto_lite", attrNameToString{
				"deps": `[":foo_proto"]`,
			}), makeBazelTarget("cc_library_static", "foo_bp2build_cc_library_static", attrNameToString{
				"implementation_whole_archive_deps": `[":foo_cc_proto_lite"]`,
				"deps":                              `[":libprotobuf-cpp-lite"]`,
			}), makeBazelTarget("cc_library_shared", "foo", attrNameToString{
				"dynamic_deps": `[":libprotobuf-cpp-lite"]`,
			}),
		},
	})
}

func TestCcLibraryProtoFull(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcProtoPreamble + `cc_library {
	name: "foo",
	srcs: ["foo.proto"],
	proto: {
		type: "full",
	},
	include_build_directory: false,
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("proto_library", "foo_proto", attrNameToString{
				"srcs": `["foo.proto"]`,
			}), makeBazelTarget("cc_proto_library", "foo_cc_proto", attrNameToString{
				"deps": `[":foo_proto"]`,
			}), makeBazelTarget("cc_library_static", "foo_bp2build_cc_library_static", attrNameToString{
				"implementation_whole_archive_deps": `[":foo_cc_proto"]`,
				"deps":                              `[":libprotobuf-cpp-full"]`,
			}), makeBazelTarget("cc_library_shared", "foo", attrNameToString{
				"dynamic_deps": `[":libprotobuf-cpp-full"]`,
			}),
		},
	})
}

func TestCcLibraryProtoLite(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcProtoPreamble + `cc_library {
	name: "foo",
	srcs: ["foo.proto"],
	proto: {
		type: "lite",
	},
	include_build_directory: false,
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("proto_library", "foo_proto", attrNameToString{
				"srcs": `["foo.proto"]`,
			}), makeBazelTarget("cc_lite_proto_library", "foo_cc_proto_lite", attrNameToString{
				"deps": `[":foo_proto"]`,
			}), makeBazelTarget("cc_library_static", "foo_bp2build_cc_library_static", attrNameToString{
				"implementation_whole_archive_deps": `[":foo_cc_proto_lite"]`,
				"deps":                              `[":libprotobuf-cpp-lite"]`,
			}), makeBazelTarget("cc_library_shared", "foo", attrNameToString{
				"dynamic_deps": `[":libprotobuf-cpp-lite"]`,
			}),
		},
	})
}

func TestCcLibraryProtoExportHeaders(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcProtoPreamble + `cc_library {
	name: "foo",
	srcs: ["foo.proto"],
	proto: {
		export_proto_headers: true,
	},
	include_build_directory: false,
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("proto_library", "foo_proto", attrNameToString{
				"srcs": `["foo.proto"]`,
			}), makeBazelTarget("cc_lite_proto_library", "foo_cc_proto_lite", attrNameToString{
				"deps": `[":foo_proto"]`,
			}), makeBazelTarget("cc_library_static", "foo_bp2build_cc_library_static", attrNameToString{
				"deps":               `[":libprotobuf-cpp-lite"]`,
				"whole_archive_deps": `[":foo_cc_proto_lite"]`,
			}), makeBazelTarget("cc_library_shared", "foo", attrNameToString{
				"dynamic_deps":       `[":libprotobuf-cpp-lite"]`,
				"whole_archive_deps": `[":foo_cc_proto_lite"]`,
			}),
		},
	})
}

func TestCcLibraryProtoFilegroups(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcProtoPreamble +
			simpleModuleDoNotConvertBp2build("filegroup", "a_fg_proto") +
			simpleModuleDoNotConvertBp2build("filegroup", "b_protos") +
			simpleModuleDoNotConvertBp2build("filegroup", "c-proto-srcs") +
			simpleModuleDoNotConvertBp2build("filegroup", "proto-srcs-d") + `
cc_library {
	name: "a",
	srcs: [":a_fg_proto"],
	proto: {
		export_proto_headers: true,
	},
	include_build_directory: false,
}

cc_library {
	name: "b",
	srcs: [":b_protos"],
	proto: {
		export_proto_headers: true,
	},
	include_build_directory: false,
}

cc_library {
	name: "c",
	srcs: [":c-proto-srcs"],
	proto: {
		export_proto_headers: true,
	},
	include_build_directory: false,
}

cc_library {
	name: "d",
	srcs: [":proto-srcs-d"],
	proto: {
		export_proto_headers: true,
	},
	include_build_directory: false,
}`,
		expectedBazelTargets: []string{
			makeBazelTarget("proto_library", "a_proto", attrNameToString{
				"srcs": `[":a_fg_proto"]`,
			}), makeBazelTarget("cc_lite_proto_library", "a_cc_proto_lite", attrNameToString{
				"deps": `[":a_proto"]`,
			}), makeBazelTarget("cc_library_static", "a_bp2build_cc_library_static", attrNameToString{
				"deps":               `[":libprotobuf-cpp-lite"]`,
				"whole_archive_deps": `[":a_cc_proto_lite"]`,
				"srcs":               `[":a_fg_proto_cpp_srcs"]`,
				"srcs_as":            `[":a_fg_proto_as_srcs"]`,
				"srcs_c":             `[":a_fg_proto_c_srcs"]`,
			}), makeBazelTarget("cc_library_shared", "a", attrNameToString{
				"dynamic_deps":       `[":libprotobuf-cpp-lite"]`,
				"whole_archive_deps": `[":a_cc_proto_lite"]`,
				"srcs":               `[":a_fg_proto_cpp_srcs"]`,
				"srcs_as":            `[":a_fg_proto_as_srcs"]`,
				"srcs_c":             `[":a_fg_proto_c_srcs"]`,
			}), makeBazelTarget("proto_library", "b_proto", attrNameToString{
				"srcs": `[":b_protos"]`,
			}), makeBazelTarget("cc_lite_proto_library", "b_cc_proto_lite", attrNameToString{
				"deps": `[":b_proto"]`,
			}), makeBazelTarget("cc_library_static", "b_bp2build_cc_library_static", attrNameToString{
				"deps":               `[":libprotobuf-cpp-lite"]`,
				"whole_archive_deps": `[":b_cc_proto_lite"]`,
				"srcs":               `[":b_protos_cpp_srcs"]`,
				"srcs_as":            `[":b_protos_as_srcs"]`,
				"srcs_c":             `[":b_protos_c_srcs"]`,
			}), makeBazelTarget("cc_library_shared", "b", attrNameToString{
				"dynamic_deps":       `[":libprotobuf-cpp-lite"]`,
				"whole_archive_deps": `[":b_cc_proto_lite"]`,
				"srcs":               `[":b_protos_cpp_srcs"]`,
				"srcs_as":            `[":b_protos_as_srcs"]`,
				"srcs_c":             `[":b_protos_c_srcs"]`,
			}), makeBazelTarget("proto_library", "c_proto", attrNameToString{
				"srcs": `[":c-proto-srcs"]`,
			}), makeBazelTarget("cc_lite_proto_library", "c_cc_proto_lite", attrNameToString{
				"deps": `[":c_proto"]`,
			}), makeBazelTarget("cc_library_static", "c_bp2build_cc_library_static", attrNameToString{
				"deps":               `[":libprotobuf-cpp-lite"]`,
				"whole_archive_deps": `[":c_cc_proto_lite"]`,
				"srcs":               `[":c-proto-srcs_cpp_srcs"]`,
				"srcs_as":            `[":c-proto-srcs_as_srcs"]`,
				"srcs_c":             `[":c-proto-srcs_c_srcs"]`,
			}), makeBazelTarget("cc_library_shared", "c", attrNameToString{
				"dynamic_deps":       `[":libprotobuf-cpp-lite"]`,
				"whole_archive_deps": `[":c_cc_proto_lite"]`,
				"srcs":               `[":c-proto-srcs_cpp_srcs"]`,
				"srcs_as":            `[":c-proto-srcs_as_srcs"]`,
				"srcs_c":             `[":c-proto-srcs_c_srcs"]`,
			}), makeBazelTarget("proto_library", "d_proto", attrNameToString{
				"srcs": `[":proto-srcs-d"]`,
			}), makeBazelTarget("cc_lite_proto_library", "d_cc_proto_lite", attrNameToString{
				"deps": `[":d_proto"]`,
			}), makeBazelTarget("cc_library_static", "d_bp2build_cc_library_static", attrNameToString{
				"deps":               `[":libprotobuf-cpp-lite"]`,
				"whole_archive_deps": `[":d_cc_proto_lite"]`,
				"srcs":               `[":proto-srcs-d_cpp_srcs"]`,
				"srcs_as":            `[":proto-srcs-d_as_srcs"]`,
				"srcs_c":             `[":proto-srcs-d_c_srcs"]`,
			}), makeBazelTarget("cc_library_shared", "d", attrNameToString{
				"dynamic_deps":       `[":libprotobuf-cpp-lite"]`,
				"whole_archive_deps": `[":d_cc_proto_lite"]`,
				"srcs":               `[":proto-srcs-d_cpp_srcs"]`,
				"srcs_as":            `[":proto-srcs-d_as_srcs"]`,
				"srcs_c":             `[":proto-srcs-d_c_srcs"]`,
			}),
		},
	})
}

func TestCcLibraryDisabledArchAndTarget(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcProtoPreamble + `cc_library {
	name: "foo",
	srcs: ["foo.cpp"],
	target: {
		darwin: {
			enabled: false,
		},
		windows: {
			enabled: false,
		},
		linux_glibc_x86: {
			enabled: false,
		},
	},
	include_build_directory: false,
}`,
		expectedBazelTargets: makeCcLibraryTargets("foo", attrNameToString{
			"srcs": `["foo.cpp"]`,
			"target_compatible_with": `select({
        "//build/bazel/platforms/os_arch:darwin_arm64": ["@platforms//:incompatible"],
        "//build/bazel/platforms/os_arch:darwin_x86_64": ["@platforms//:incompatible"],
        "//build/bazel/platforms/os_arch:linux_glibc_x86": ["@platforms//:incompatible"],
        "//build/bazel/platforms/os_arch:windows_x86": ["@platforms//:incompatible"],
        "//build/bazel/platforms/os_arch:windows_x86_64": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
		}),
	})
}

func TestCcLibraryDisabledArchAndTargetWithDefault(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcProtoPreamble + `cc_library {
	name: "foo",
	srcs: ["foo.cpp"],
  enabled: false,
	target: {
		darwin: {
			enabled: true,
		},
		windows: {
			enabled: false,
		},
		linux_glibc_x86: {
			enabled: false,
		},
	},
	include_build_directory: false,
}`,
		expectedBazelTargets: makeCcLibraryTargets("foo", attrNameToString{
			"srcs": `["foo.cpp"]`,
			"target_compatible_with": `select({
        "//build/bazel/platforms/os_arch:darwin_arm64": [],
        "//build/bazel/platforms/os_arch:darwin_x86_64": [],
        "//conditions:default": ["@platforms//:incompatible"],
    })`,
		}),
	})
}

func TestCcLibrarySharedDisabled(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcProtoPreamble + `cc_library {
	name: "foo",
	srcs: ["foo.cpp"],
	enabled: false,
	shared: {
		enabled: true,
	},
	target: {
		android: {
			shared: {
				enabled: false,
			},
		}
  },
	include_build_directory: false,
}`,
		expectedBazelTargets: []string{makeBazelTarget("cc_library_static", "foo_bp2build_cc_library_static", attrNameToString{
			"srcs":                   `["foo.cpp"]`,
			"target_compatible_with": `["@platforms//:incompatible"]`,
		}), makeBazelTarget("cc_library_shared", "foo", attrNameToString{
			"srcs": `["foo.cpp"]`,
			"target_compatible_with": `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
		}),
		},
	})
}

func TestCcLibraryStaticDisabledForSomeArch(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcProtoPreamble + `cc_library {
	name: "foo",
	srcs: ["foo.cpp"],
	shared: {
		enabled: false
	},
	target: {
		darwin: {
			enabled: true,
		},
		windows: {
			enabled: false,
		},
		linux_glibc_x86: {
			shared: {
				enabled: true,
			},
		},
	},
	include_build_directory: false,
}`,
		expectedBazelTargets: []string{makeBazelTarget("cc_library_static", "foo_bp2build_cc_library_static", attrNameToString{
			"srcs": `["foo.cpp"]`,
			"target_compatible_with": `select({
        "//build/bazel/platforms/os:windows": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
		}), makeBazelTarget("cc_library_shared", "foo", attrNameToString{
			"srcs": `["foo.cpp"]`,
			"target_compatible_with": `select({
        "//build/bazel/platforms/os_arch:darwin_arm64": [],
        "//build/bazel/platforms/os_arch:darwin_x86_64": [],
        "//build/bazel/platforms/os_arch:linux_glibc_x86": [],
        "//conditions:default": ["@platforms//:incompatible"],
    })`,
		}),
		}})
}

func TestCcLibraryStubs(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		description:                "cc_library stubs",
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		dir:                        "foo/bar",
		filesystem: map[string]string{
			"foo/bar/Android.bp": `
cc_library {
    name: "a",
    stubs: { symbol_file: "a.map.txt", versions: ["28", "29", "current"] },
    bazel_module: { bp2build_available: true },
    include_build_directory: false,
}
`,
		},
		blueprint: soongCcLibraryPreamble,
		expectedBazelTargets: makeCcLibraryTargets("a", attrNameToString{
			"stubs_symbol_file": `"a.map.txt"`,
			"stubs_versions": `[
        "28",
        "29",
        "current",
    ]`,
		}),
	},
	)
}

func TestCcLibraryEscapeLdflags(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		blueprint: soongCcProtoPreamble + `cc_library {
	name: "foo",
	ldflags: ["-Wl,--rpath,${ORIGIN}"],
	include_build_directory: false,
}`,
		expectedBazelTargets: makeCcLibraryTargets("foo", attrNameToString{
			"linkopts": `["-Wl,--rpath,$${ORIGIN}"]`,
		}),
	})
}

func TestCcLibraryConvertLex(t *testing.T) {
	runCcLibraryTestCase(t, bp2buildTestCase{
		moduleTypeUnderTest:        "cc_library",
		moduleTypeUnderTestFactory: cc.LibraryFactory,
		filesystem: map[string]string{
			"foo.c":   "",
			"bar.cc":  "",
			"foo1.l":  "",
			"bar1.ll": "",
			"foo2.l":  "",
			"bar2.ll": "",
		},
		blueprint: `cc_library {
	name: "foo_lib",
	srcs: ["foo.c", "bar.cc", "foo1.l", "foo2.l", "bar1.ll", "bar2.ll"],
	lex: { flags: ["--foo_flags"] },
	include_build_directory: false,
	bazel_module: { bp2build_available: true },
}`,
		expectedBazelTargets: append([]string{
			makeBazelTarget("genlex", "foo_lib_genlex_l", attrNameToString{
				"srcs": `[
        "foo1.l",
        "foo2.l",
    ]`,
				"lexopts": `["--foo_flags"]`,
			}),
			makeBazelTarget("genlex", "foo_lib_genlex_ll", attrNameToString{
				"srcs": `[
        "bar1.ll",
        "bar2.ll",
    ]`,
				"lexopts": `["--foo_flags"]`,
			}),
		},
			makeCcLibraryTargets("foo_lib", attrNameToString{
				"srcs": `[
        "bar.cc",
        ":foo_lib_genlex_ll",
    ]`,
				"srcs_c": `[
        "foo.c",
        ":foo_lib_genlex_l",
    ]`,
			})...),
	})
}
