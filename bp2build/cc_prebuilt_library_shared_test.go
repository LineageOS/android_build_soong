package bp2build

import (
	"fmt"
	"testing"

	"android/soong/cc"
)

func TestSharedPrebuiltLibrary(t *testing.T) {
	runBp2BuildTestCaseSimple(t,
		bp2buildTestCase{
			description:                "prebuilt library shared simple",
			moduleTypeUnderTest:        "cc_prebuilt_library_shared",
			moduleTypeUnderTestFactory: cc.PrebuiltSharedLibraryFactory,
			filesystem: map[string]string{
				"libf.so": "",
			},
			blueprint: `
cc_prebuilt_library_shared {
	name: "libtest",
	srcs: ["libf.so"],
	bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{
				makeBazelTarget("prebuilt_library_shared", "libtest", attrNameToString{
					"shared_library": `"libf.so"`,
				}),
			},
		})
}

func TestSharedPrebuiltLibraryWithArchVariance(t *testing.T) {
	runBp2BuildTestCaseSimple(t,
		bp2buildTestCase{
			description:                "prebuilt library shared with arch variance",
			moduleTypeUnderTest:        "cc_prebuilt_library_shared",
			moduleTypeUnderTestFactory: cc.PrebuiltSharedLibraryFactory,
			filesystem: map[string]string{
				"libf.so": "",
				"libg.so": "",
			},
			blueprint: `
cc_prebuilt_library_shared {
	name: "libtest",
	arch: {
		arm64: { srcs: ["libf.so"], },
		arm: { srcs: ["libg.so"], },
	},
	bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []string{
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

func TestSharedPrebuiltLibrarySharedStanzaFails(t *testing.T) {
	runBp2BuildTestCaseSimple(t,
		bp2buildTestCase{
			description:                "prebuilt library shared with shared stanza fails because multiple sources",
			moduleTypeUnderTest:        "cc_prebuilt_library_shared",
			moduleTypeUnderTestFactory: cc.PrebuiltSharedLibraryFactory,
			filesystem: map[string]string{
				"libf.so": "",
				"libg.so": "",
			},
			blueprint: `
cc_prebuilt_library_shared {
	name: "libtest",
	srcs: ["libf.so"],
	shared: {
		srcs: ["libg.so"],
	},
	bazel_module: { bp2build_available: true},
}`,
			expectedErr: fmt.Errorf("Expected at most one source file"),
		})
}
