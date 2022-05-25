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

func TestStaticPrebuiltLibrary(t *testing.T) {
	runBp2BuildTestCaseSimple(t,
		bp2buildTestCase{
			description:                "prebuilt library static simple",
			moduleTypeUnderTest:        "cc_prebuilt_library_static",
			moduleTypeUnderTestFactory: cc.PrebuiltStaticLibraryFactory,
			filesystem: map[string]string{
				"libf.so": "",
			},
			blueprint: `
cc_prebuilt_library_static {
	name: "libtest",
	srcs: ["libf.so"],
	bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{
				makeBazelTarget("prebuilt_library_static", "libtest", attrNameToString{
					"static_library": `"libf.so"`,
				}),
			},
		})
}

func TestStaticPrebuiltLibraryWithArchVariance(t *testing.T) {
	runBp2BuildTestCaseSimple(t,
		bp2buildTestCase{
			description:                "prebuilt library static with arch variance",
			moduleTypeUnderTest:        "cc_prebuilt_library_static",
			moduleTypeUnderTestFactory: cc.PrebuiltStaticLibraryFactory,
			filesystem: map[string]string{
				"libf.so": "",
				"libg.so": "",
			},
			blueprint: `
cc_prebuilt_library_static {
	name: "libtest",
	arch: {
		arm64: { srcs: ["libf.so"], },
		arm: { srcs: ["libg.so"], },
	},
	bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{
				makeBazelTarget("prebuilt_library_static", "libtest", attrNameToString{
					"static_library": `select({
        "//build/bazel/platforms/arch:arm": "libg.so",
        "//build/bazel/platforms/arch:arm64": "libf.so",
        "//conditions:default": None,
    })`,
				}),
			},
		})
}

func TestStaticPrebuiltLibraryStaticStanzaFails(t *testing.T) {
	runBp2BuildTestCaseSimple(t,
		bp2buildTestCase{
			description:                "prebuilt library with static stanza fails because multiple sources",
			moduleTypeUnderTest:        "cc_prebuilt_library_static",
			moduleTypeUnderTestFactory: cc.PrebuiltStaticLibraryFactory,
			filesystem: map[string]string{
				"libf.so": "",
				"libg.so": "",
			},
			blueprint: `
cc_prebuilt_library_static {
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

func TestCcLibraryStaticConvertLex(t *testing.T) {
	runCcLibrarySharedTestCase(t, bp2buildTestCase{
		description:                "cc_library_static with lex files",
		moduleTypeUnderTest:        "cc_library_static",
		moduleTypeUnderTestFactory: cc.LibraryStaticFactory,
		filesystem: map[string]string{
			"foo.c":   "",
			"bar.cc":  "",
			"foo1.l":  "",
			"bar1.ll": "",
			"foo2.l":  "",
			"bar2.ll": "",
		},
		blueprint: `cc_library_static {
	name: "foo_lib",
	srcs: ["foo.c", "bar.cc", "foo1.l", "foo2.l", "bar1.ll", "bar2.ll"],
	lex: { flags: ["--foo_flags"] },
	include_build_directory: false,
	bazel_module: { bp2build_available: true },
}`,
		expectedBazelTargets: []string{
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
			makeBazelTarget("cc_library_static", "foo_lib", attrNameToString{
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
