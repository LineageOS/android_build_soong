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

	"android/soong/android"
	"android/soong/cc"
)

func runCcPrebuiltLibraryTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	(&tc).ModuleTypeUnderTest = "cc_prebuilt_library"
	(&tc).ModuleTypeUnderTestFactory = cc.PrebuiltLibraryFactory
	RunBp2BuildTestCaseSimple(t, tc)
}

func TestPrebuiltLibraryStaticAndSharedSimple(t *testing.T) {
	runCcPrebuiltLibraryTestCase(t,
		Bp2buildTestCase{
			Description: "prebuilt library static and shared simple",
			Filesystem: map[string]string{
				"libf.so": "",
			},
			Blueprint: `
cc_prebuilt_library {
	name: "libtest",
	srcs: ["libf.so"],
	bazel_module: { bp2build_available: true },
}`,
			ExpectedBazelTargets: []string{
				MakeBazelTarget("cc_prebuilt_library_static", "libtest_bp2build_cc_library_static", AttrNameToString{
					"static_library": `"libf.so"`,
				}),
				MakeBazelTarget("cc_prebuilt_library_static", "libtest_bp2build_cc_library_static_alwayslink", AttrNameToString{
					"static_library": `"libf.so"`,
					"alwayslink":     "True",
				}),
				MakeBazelTarget("cc_prebuilt_library_shared", "libtest", AttrNameToString{
					"shared_library": `"libf.so"`,
				}),
			},
		})
}

func TestPrebuiltLibraryWithArchVariance(t *testing.T) {
	runCcPrebuiltLibraryTestCase(t,
		Bp2buildTestCase{
			Description: "prebuilt library with arch variance",
			Filesystem: map[string]string{
				"libf.so": "",
				"libg.so": "",
			},
			Blueprint: `
cc_prebuilt_library {
	name: "libtest",
	arch: {
		arm64: { srcs: ["libf.so"], },
		arm: { srcs: ["libg.so"], },
	},
	bazel_module: { bp2build_available: true },
}`,
			ExpectedBazelTargets: []string{
				MakeBazelTarget("cc_prebuilt_library_static", "libtest_bp2build_cc_library_static", AttrNameToString{
					"static_library": `select({
        "//build/bazel/platforms/arch:arm": "libg.so",
        "//build/bazel/platforms/arch:arm64": "libf.so",
        "//conditions:default": None,
    })`}),
				MakeBazelTarget("cc_prebuilt_library_static", "libtest_bp2build_cc_library_static_alwayslink", AttrNameToString{
					"alwayslink": "True",
					"static_library": `select({
        "//build/bazel/platforms/arch:arm": "libg.so",
        "//build/bazel/platforms/arch:arm64": "libf.so",
        "//conditions:default": None,
    })`}),
				MakeBazelTarget("cc_prebuilt_library_shared", "libtest", AttrNameToString{
					"shared_library": `select({
        "//build/bazel/platforms/arch:arm": "libg.so",
        "//build/bazel/platforms/arch:arm64": "libf.so",
        "//conditions:default": None,
    })`,
				}),
			},
		})
}

func TestPrebuiltLibraryAdditionalAttrs(t *testing.T) {
	runCcPrebuiltLibraryTestCase(t,
		Bp2buildTestCase{
			Description: "prebuilt library additional attributes",
			Filesystem: map[string]string{
				"libf.so":             "",
				"testdir/1/include.h": "",
				"testdir/2/other.h":   "",
			},
			Blueprint: `
cc_prebuilt_library {
	name: "libtest",
	srcs: ["libf.so"],
	export_include_dirs: ["testdir/1/"],
	export_system_include_dirs: ["testdir/2/"],
	bazel_module: { bp2build_available: true },
}`,
			ExpectedBazelTargets: []string{
				MakeBazelTarget("cc_prebuilt_library_static", "libtest_bp2build_cc_library_static", AttrNameToString{
					"static_library":         `"libf.so"`,
					"export_includes":        `["testdir/1/"]`,
					"export_system_includes": `["testdir/2/"]`,
				}),
				MakeBazelTarget("cc_prebuilt_library_static", "libtest_bp2build_cc_library_static_alwayslink", AttrNameToString{
					"static_library":         `"libf.so"`,
					"export_includes":        `["testdir/1/"]`,
					"export_system_includes": `["testdir/2/"]`,
					"alwayslink":             "True",
				}),
				MakeBazelTarget("cc_prebuilt_library_shared", "libtest", AttrNameToString{
					"shared_library":         `"libf.so"`,
					"export_includes":        `["testdir/1/"]`,
					"export_system_includes": `["testdir/2/"]`,
				}),
			},
		})
}

func TestPrebuiltLibrarySharedStanzaFails(t *testing.T) {
	runCcPrebuiltLibraryTestCase(t,
		Bp2buildTestCase{
			Description: "prebuilt library with shared stanza fails because multiple sources",
			Filesystem: map[string]string{
				"libf.so": "",
				"libg.so": "",
			},
			Blueprint: `
cc_prebuilt_library {
	name: "libtest",
	srcs: ["libf.so"],
	shared: {
		srcs: ["libg.so"],
	},
	bazel_module: { bp2build_available: true },
}`,
			ExpectedErr: fmt.Errorf("Expected at most one source file"),
		})
}

func TestPrebuiltLibraryStaticStanzaFails(t *testing.T) {
	RunBp2BuildTestCaseSimple(t,
		Bp2buildTestCase{
			Description:                "prebuilt library with static stanza fails because multiple sources",
			ModuleTypeUnderTest:        "cc_prebuilt_library",
			ModuleTypeUnderTestFactory: cc.PrebuiltLibraryFactory,
			Filesystem: map[string]string{
				"libf.so": "",
				"libg.so": "",
			},
			Blueprint: `
cc_prebuilt_library {
	name: "libtest",
	srcs: ["libf.so"],
	static: {
		srcs: ["libg.so"],
	},
	bazel_module: { bp2build_available: true },
}`,
			ExpectedErr: fmt.Errorf("Expected at most one source file"),
		})
}

func TestPrebuiltLibrarySharedAndStaticStanzas(t *testing.T) {
	runCcPrebuiltLibraryTestCase(t,
		Bp2buildTestCase{
			Description: "prebuilt library with both shared and static stanzas",
			Filesystem: map[string]string{
				"libf.so": "",
				"libg.so": "",
			},
			Blueprint: `
cc_prebuilt_library {
	name: "libtest",
	static: {
		srcs: ["libf.so"],
	},
	shared: {
		srcs: ["libg.so"],
	},
	bazel_module: { bp2build_available: true },
}`,
			ExpectedBazelTargets: []string{
				MakeBazelTarget("cc_prebuilt_library_static", "libtest_bp2build_cc_library_static", AttrNameToString{
					"static_library": `"libf.so"`,
				}),
				MakeBazelTarget("cc_prebuilt_library_static", "libtest_bp2build_cc_library_static_alwayslink", AttrNameToString{
					"static_library": `"libf.so"`,
					"alwayslink":     "True",
				}),
				MakeBazelTarget("cc_prebuilt_library_shared", "libtest", AttrNameToString{
					"shared_library": `"libg.so"`,
				}),
			},
		})
}

// TODO(b/228623543): When this bug is fixed, enable this test
//func TestPrebuiltLibraryOnlyShared(t *testing.T) {
//	runCcPrebuiltLibraryTestCase(t,
//		bp2buildTestCase{
//			description:                "prebuilt library shared only",
//			filesystem: map[string]string{
//				"libf.so": "",
//			},
//			blueprint: `
//cc_prebuilt_library {
//	name: "libtest",
//	srcs: ["libf.so"],
//	static: {
//		enabled: false,
//	},
//	bazel_module: { bp2build_available: true },
//}`,
//			expectedBazelTargets: []string{
//				makeBazelTarget("cc_prebuilt_library_shared", "libtest", attrNameToString{
//					"shared_library": `"libf.so"`,
//				}),
//			},
//		})
//}

// TODO(b/228623543): When this bug is fixed, enable this test
//func TestPrebuiltLibraryOnlyStatic(t *testing.T) {
//	runCcPrebuiltLibraryTestCase(t,
//		bp2buildTestCase{
//			description:                "prebuilt library static only",
//			filesystem: map[string]string{
//				"libf.so": "",
//			},
//			blueprint: `
//cc_prebuilt_library {
//	name: "libtest",
//	srcs: ["libf.so"],
//	shared: {
//		enabled: false,
//	},
//	bazel_module: { bp2build_available: true },
//}`,
//			expectedBazelTargets: []string{
//				makeBazelTarget("cc_prebuilt_library_static", "libtest_bp2build_cc_library_static", attrNameToString{
//					"static_library": `"libf.so"`,
//				}),
//				makeBazelTarget("cc_prebuilt_library_static", "libtest_bp2build_cc_library_static_always", attrNameToString{
//					"static_library": `"libf.so"`,
//					"alwayslink": "True",
//				}),
//			},
//		})
//}

func TestPrebuiltLibraryWithExportIncludesArchVariant(t *testing.T) {
	runCcPrebuiltLibraryTestCase(t, Bp2buildTestCase{
		Description: "cc_prebuilt_library correctly translates export_includes with arch variance",
		Filesystem: map[string]string{
			"libf.so": "",
			"libg.so": "",
		},
		Blueprint: `
cc_prebuilt_library {
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
        "//build/bazel/platforms/arch:arm": ["testdir/1/"],
        "//build/bazel/platforms/arch:arm64": ["testdir/2/"],
        "//conditions:default": [],
    })`,
			}),
			MakeBazelTarget("cc_prebuilt_library_static", "libtest_bp2build_cc_library_static", AttrNameToString{
				"static_library": `"libf.so"`,
				"export_includes": `select({
        "//build/bazel/platforms/arch:arm": ["testdir/1/"],
        "//build/bazel/platforms/arch:arm64": ["testdir/2/"],
        "//conditions:default": [],
    })`,
			}),
			MakeBazelTarget("cc_prebuilt_library_static", "libtest_bp2build_cc_library_static_alwayslink", AttrNameToString{
				"alwayslink":     "True",
				"static_library": `"libf.so"`,
				"export_includes": `select({
        "//build/bazel/platforms/arch:arm": ["testdir/1/"],
        "//build/bazel/platforms/arch:arm64": ["testdir/2/"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestPrebuiltLibraryWithExportSystemIncludesArchVariant(t *testing.T) {
	runCcPrebuiltLibraryTestCase(t, Bp2buildTestCase{
		Description: "cc_prebuilt_ibrary correctly translates export_system_includes with arch variance",
		Filesystem: map[string]string{
			"libf.so": "",
			"libg.so": "",
		},
		Blueprint: `
cc_prebuilt_library {
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
        "//build/bazel/platforms/arch:arm": ["testdir/1/"],
        "//build/bazel/platforms/arch:arm64": ["testdir/2/"],
        "//conditions:default": [],
    })`,
			}),
			MakeBazelTarget("cc_prebuilt_library_static", "libtest_bp2build_cc_library_static", AttrNameToString{
				"static_library": `"libf.so"`,
				"export_system_includes": `select({
        "//build/bazel/platforms/arch:arm": ["testdir/1/"],
        "//build/bazel/platforms/arch:arm64": ["testdir/2/"],
        "//conditions:default": [],
    })`,
			}),
			MakeBazelTarget("cc_prebuilt_library_static", "libtest_bp2build_cc_library_static_alwayslink", AttrNameToString{
				"alwayslink":     "True",
				"static_library": `"libf.so"`,
				"export_system_includes": `select({
        "//build/bazel/platforms/arch:arm": ["testdir/1/"],
        "//build/bazel/platforms/arch:arm64": ["testdir/2/"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestPrebuiltNdkStlConversion(t *testing.T) {
	registerNdkStlModuleTypes := func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("ndk_prebuilt_static_stl", cc.NdkPrebuiltStaticStlFactory)
		ctx.RegisterModuleType("ndk_prebuilt_shared_stl", cc.NdkPrebuiltSharedStlFactory)
	}
	RunBp2BuildTestCase(t, registerNdkStlModuleTypes, Bp2buildTestCase{
		Description: "TODO",
		Blueprint: `
ndk_prebuilt_static_stl {
	name: "ndk_libfoo_static",
	export_include_dirs: ["dir1", "dir2"],
}
ndk_prebuilt_shared_stl {
	name: "ndk_libfoo_shared",
	export_include_dirs: ["dir1", "dir2"],
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("cc_prebuilt_library_static", "ndk_libfoo_static", AttrNameToString{
				"static_library": `select({
        "//build/bazel/platforms/os_arch:android_arm": "current/sources/cxx-stl/llvm-libc++/libs/armeabi-v7a/libfoo_static.a",
        "//build/bazel/platforms/os_arch:android_arm64": "current/sources/cxx-stl/llvm-libc++/libs/arm64-v8a/libfoo_static.a",
        "//build/bazel/platforms/os_arch:android_riscv64": "current/sources/cxx-stl/llvm-libc++/libs/riscv64/libfoo_static.a",
        "//build/bazel/platforms/os_arch:android_x86": "current/sources/cxx-stl/llvm-libc++/libs/x86/libfoo_static.a",
        "//build/bazel/platforms/os_arch:android_x86_64": "current/sources/cxx-stl/llvm-libc++/libs/x86_64/libfoo_static.a",
        "//conditions:default": None,
    })`,
				"export_system_includes": `[
        "dir1",
        "dir2",
    ]`,
			}),
			MakeBazelTarget("cc_prebuilt_library_shared", "ndk_libfoo_shared", AttrNameToString{
				"shared_library": `select({
        "//build/bazel/platforms/os_arch:android_arm": "current/sources/cxx-stl/llvm-libc++/libs/armeabi-v7a/libfoo_shared.so",
        "//build/bazel/platforms/os_arch:android_arm64": "current/sources/cxx-stl/llvm-libc++/libs/arm64-v8a/libfoo_shared.so",
        "//build/bazel/platforms/os_arch:android_riscv64": "current/sources/cxx-stl/llvm-libc++/libs/riscv64/libfoo_shared.so",
        "//build/bazel/platforms/os_arch:android_x86": "current/sources/cxx-stl/llvm-libc++/libs/x86/libfoo_shared.so",
        "//build/bazel/platforms/os_arch:android_x86_64": "current/sources/cxx-stl/llvm-libc++/libs/x86_64/libfoo_shared.so",
        "//conditions:default": None,
    })`,
				"export_system_includes": `[
        "dir1",
        "dir2",
    ]`,
			}),
		},
	})
}
