package bp2build

import (
	"fmt"
	"testing"

	"android/soong/cc"
)

func TestSharedPrebuiltLibrary(t *testing.T) {
	runBp2BuildTestCaseSimple(t,
		Bp2buildTestCase{
			Description:                "prebuilt library shared simple",
			ModuleTypeUnderTest:        "cc_prebuilt_library_shared",
			ModuleTypeUnderTestFactory: cc.PrebuiltSharedLibraryFactory,
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
				makeBazelTarget("prebuilt_library_shared", "libtest", AttrNameToString{
					"shared_library": `"libf.so"`,
				}),
			},
		})
}

func TestSharedPrebuiltLibraryWithArchVariance(t *testing.T) {
	runBp2BuildTestCaseSimple(t,
		Bp2buildTestCase{
			Description:                "prebuilt library shared with arch variance",
			ModuleTypeUnderTest:        "cc_prebuilt_library_shared",
			ModuleTypeUnderTestFactory: cc.PrebuiltSharedLibraryFactory,
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
				makeBazelTarget("prebuilt_library_shared", "libtest", AttrNameToString{
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
		Bp2buildTestCase{
			Description:                "prebuilt library shared with shared stanza fails because multiple sources",
			ModuleTypeUnderTest:        "cc_prebuilt_library_shared",
			ModuleTypeUnderTestFactory: cc.PrebuiltSharedLibraryFactory,
			Filesystem: map[string]string{
				"libf.so": "",
				"libg.so": "",
			},
			Blueprint: `
cc_prebuilt_library_shared {
	name: "libtest",
	srcs: ["libf.so"],
	shared: {
		srcs: ["libg.so"],
	},
	bazel_module: { bp2build_available: true},
}`,
			ExpectedErr: fmt.Errorf("Expected at most one source file"),
		})
}
