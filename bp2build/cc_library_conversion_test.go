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
}`
)

func TestCcLibraryBp2Build(t *testing.T) {
	testCases := []struct {
		description                        string
		moduleTypeUnderTest                string
		moduleTypeUnderTestFactory         android.ModuleFactory
		moduleTypeUnderTestBp2BuildMutator func(android.TopDownMutatorContext)
		bp                                 string
		expectedBazelTargets               []string
		filesystem                         map[string]string
		dir                                string
	}{
		{
			description:                        "cc_library - simple example",
			moduleTypeUnderTest:                "cc_library",
			moduleTypeUnderTestFactory:         cc.LibraryFactory,
			moduleTypeUnderTestBp2BuildMutator: cc.CcLibraryBp2Build,
			filesystem: map[string]string{
				"android.cpp": "",
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
			bp: soongCcLibraryPreamble + `
cc_library_headers { name: "some-headers" }
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
    },
}
`,
			expectedBazelTargets: []string{`cc_library(
    name = "foo-lib",
    copts = [
        "-Wall",
        "-I.",
    ],
    deps = [":some-headers"],
    hdrs = ["foo-dir/a.h"],
    includes = ["foo-dir"],
    linkopts = ["-Wl,--exclude-libs=bar.a"] + select({
        "//build/bazel/platforms/arch:x86": ["-Wl,--exclude-libs=baz.a"],
        "//build/bazel/platforms/arch:x86_64": ["-Wl,--exclude-libs=qux.a"],
        "//conditions:default": [],
    }),
    srcs = [
        "impl.cpp",
        "header.h",
        "foo-dir/a.h",
        "header.hh",
        "header.hpp",
        "header.hxx",
        "header.h++",
        "header.inl",
        "header.inc",
        "header.ipp",
        "header.h.generic",
    ] + select({
        "//build/bazel/platforms/arch:x86": ["x86.cpp"],
        "//build/bazel/platforms/arch:x86_64": ["x86_64.cpp"],
        "//conditions:default": [],
    }) + select({
        "//build/bazel/platforms/os:android": ["android.cpp"],
        "//build/bazel/platforms/os:darwin": ["darwin.cpp"],
        "//build/bazel/platforms/os:linux": ["linux.cpp"],
        "//conditions:default": [],
    }),
)`},
		},
		{
			description:                        "cc_library - trimmed example of //bionic/linker:ld-android",
			moduleTypeUnderTest:                "cc_library",
			moduleTypeUnderTestFactory:         cc.LibraryFactory,
			moduleTypeUnderTestBp2BuildMutator: cc.CcLibraryBp2Build,
			filesystem: map[string]string{
				"ld-android.cpp":           "",
				"linked_list.h":            "",
				"linker.h":                 "",
				"linker_block_allocator.h": "",
				"linker_cfi.h":             "",
			},
			bp: soongCcLibraryPreamble + `
cc_library_headers { name: "libc_headers" }
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
}
`,
			expectedBazelTargets: []string{`cc_library(
    name = "fake-ld-android",
    copts = [
        "-Wall",
        "-Wextra",
        "-Wunused",
        "-Werror",
        "-I.",
    ],
    deps = [":libc_headers"],
    linkopts = [
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
    }),
    srcs = [
        "ld_android.cpp",
        "linked_list.h",
        "linker.h",
        "linker_block_allocator.h",
        "linker_cfi.h",
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
		ctx.RegisterModuleType("cc_library_headers", cc.LibraryHeaderFactory)
		ctx.RegisterModuleType(testCase.moduleTypeUnderTest, testCase.moduleTypeUnderTestFactory)
		ctx.RegisterBp2BuildMutator(testCase.moduleTypeUnderTest, testCase.moduleTypeUnderTestBp2BuildMutator)
		ctx.RegisterBp2BuildConfig(bp2buildConfig) // TODO(jingwen): make this the default for all tests
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
		codegenCtx := NewCodegenContext(config, *ctx.Context, Bp2Build)
		bazelTargets := generateBazelTargetsForDir(codegenCtx, checkDir)
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
