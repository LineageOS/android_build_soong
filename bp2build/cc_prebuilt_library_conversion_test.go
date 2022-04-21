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

	"android/soong/cc"
)

func TestPrebuiltLibraryStaticAndSharedSimple(t *testing.T) {
	runBp2BuildTestCaseSimple(t,
		bp2buildTestCase{
			description:                "prebuilt library static and shared simple",
			moduleTypeUnderTest:        "cc_prebuilt_library",
			moduleTypeUnderTestFactory: cc.PrebuiltLibraryFactory,
			filesystem: map[string]string{
				"libf.so": "",
			},
			blueprint: `
cc_prebuilt_library {
	name: "libtest",
	srcs: ["libf.so"],
	bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{
				makeBazelTarget("prebuilt_library_static", "libtest_bp2build_cc_library_static", attrNameToString{
					"static_library": `"libf.so"`,
				}),
				makeBazelTarget("prebuilt_library_shared", "libtest", attrNameToString{
					"shared_library": `"libf.so"`,
				}),
			},
		})
}

func TestPrebuiltLibraryWithArchVariance(t *testing.T) {
	runBp2BuildTestCaseSimple(t,
		bp2buildTestCase{
			description:                "prebuilt library with arch variance",
			moduleTypeUnderTest:        "cc_prebuilt_library",
			moduleTypeUnderTestFactory: cc.PrebuiltLibraryFactory,
			filesystem: map[string]string{
				"libf.so": "",
				"libg.so": "",
			},
			blueprint: `
cc_prebuilt_library {
	name: "libtest",
	arch: {
		arm64: { srcs: ["libf.so"], },
		arm: { srcs: ["libg.so"], },
	},
	bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{
				makeBazelTarget("prebuilt_library_static", "libtest_bp2build_cc_library_static", attrNameToString{
					"static_library": `select({
        "//build/bazel/platforms/arch:arm": "libg.so",
        "//build/bazel/platforms/arch:arm64": "libf.so",
        "//conditions:default": None,
    })`,
				}),
				makeBazelTarget("prebuilt_library_shared", "libtest", attrNameToString{
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
	runBp2BuildTestCaseSimple(t,
		bp2buildTestCase{
			description:                "prebuilt library additional attributes",
			moduleTypeUnderTest:        "cc_prebuilt_library",
			moduleTypeUnderTestFactory: cc.PrebuiltLibraryFactory,
			filesystem: map[string]string{
				"libf.so":    "",
				"testdir/1/": "",
				"testdir/2/": "",
			},
			blueprint: `
cc_prebuilt_library {
	name: "libtest",
	srcs: ["libf.so"],
	export_include_dirs: ["testdir/1/"],
	export_system_include_dirs: ["testdir/2/"],
	bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{
				makeBazelTarget("prebuilt_library_static", "libtest_bp2build_cc_library_static", attrNameToString{
					"static_library":         `"libf.so"`,
					"export_includes":        `["testdir/1/"]`,
					"export_system_includes": `["testdir/2/"]`,
				}),
				// TODO(b/229374533): When fixed, update this test
				makeBazelTarget("prebuilt_library_shared", "libtest", attrNameToString{
					"shared_library": `"libf.so"`,
				}),
			},
		})
}

func TestPrebuiltLibrarySharedStanzaFails(t *testing.T) {
	runBp2BuildTestCaseSimple(t,
		bp2buildTestCase{
			description:                "prebuilt library with shared stanza fails because multiple sources",
			moduleTypeUnderTest:        "cc_prebuilt_library",
			moduleTypeUnderTestFactory: cc.PrebuiltLibraryFactory,
			filesystem: map[string]string{
				"libf.so": "",
				"libg.so": "",
			},
			blueprint: `
cc_prebuilt_library {
	name: "libtest",
	srcs: ["libf.so"],
	shared: {
		srcs: ["libg.so"],
	},
	bazel_module: { bp2build_available: true },
}`,
			expectedErr: fmt.Errorf("Expected at most one source file"),
		})
}

func TestPrebuiltLibraryStaticStanzaFails(t *testing.T) {
	runBp2BuildTestCaseSimple(t,
		bp2buildTestCase{
			description:                "prebuilt library with static stanza fails because multiple sources",
			moduleTypeUnderTest:        "cc_prebuilt_library",
			moduleTypeUnderTestFactory: cc.PrebuiltLibraryFactory,
			filesystem: map[string]string{
				"libf.so": "",
				"libg.so": "",
			},
			blueprint: `
cc_prebuilt_library {
	name: "libtest",
	srcs: ["libf.so"],
	static: {
		srcs: ["libg.so"],
	},
	bazel_module: { bp2build_available: true },
}`,
			expectedErr: fmt.Errorf("Expected at most one source file"),
		})
}

func TestPrebuiltLibrarySharedAndStaticStanzas(t *testing.T) {
	runBp2BuildTestCaseSimple(t,
		bp2buildTestCase{
			description:                "prebuilt library with both shared and static stanzas",
			moduleTypeUnderTest:        "cc_prebuilt_library",
			moduleTypeUnderTestFactory: cc.PrebuiltLibraryFactory,
			filesystem: map[string]string{
				"libf.so": "",
				"libg.so": "",
			},
			blueprint: `
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
			expectedBazelTargets: []string{
				makeBazelTarget("prebuilt_library_static", "libtest_bp2build_cc_library_static", attrNameToString{
					"static_library": `"libf.so"`,
				}),
				makeBazelTarget("prebuilt_library_shared", "libtest", attrNameToString{
					"shared_library": `"libg.so"`,
				}),
			},
		})
}

// TODO(b/228623543): When this bug is fixed, enable this test
//func TestPrebuiltLibraryOnlyShared(t *testing.T) {
//	runBp2BuildTestCaseSimple(t,
//		bp2buildTestCase{
//			description:                "prebuilt library shared only",
//			moduleTypeUnderTest:        "cc_prebuilt_library",
//			moduleTypeUnderTestFactory: cc.PrebuiltLibraryFactory,
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
//				makeBazelTarget("prebuilt_library_shared", "libtest", attrNameToString{
//					"shared_library": `"libf.so"`,
//				}),
//			},
//		})
//}

// TODO(b/228623543): When this bug is fixed, enable this test
//func TestPrebuiltLibraryOnlyStatic(t *testing.T) {
//	runBp2BuildTestCaseSimple(t,
//		bp2buildTestCase{
//			description:                "prebuilt library static only",
//			moduleTypeUnderTest:        "cc_prebuilt_library",
//			moduleTypeUnderTestFactory: cc.PrebuiltLibraryFactory,
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
//				makeBazelTarget("prebuilt_library_static", "libtest_bp2build_cc_library_static", attrNameToString{
//					"static_library": `"libf.so"`,
//				}),
//			},
//		})
//}
