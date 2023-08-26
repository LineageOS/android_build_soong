package bp2build

import (
	"android/soong/android"
	"android/soong/rust"
	"testing"
)

func rustRustProcMacroTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, registerRustProcMacroModuleTypes, tc)
}

func registerRustProcMacroModuleTypes(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("rust_library_host", rust.RustLibraryHostFactory)
	ctx.RegisterModuleType("rust_proc_macro", rust.ProcMacroFactory)
}

func TestRustProcMacroLibrary(t *testing.T) {
	runRustLibraryTestCase(t, Bp2buildTestCase{
		Dir:       "external/rust/crates/foo",
		Blueprint: "",
		Filesystem: map[string]string{
			"external/rust/crates/foo/src/lib.rs":    "",
			"external/rust/crates/foo/src/helper.rs": "",
			"external/rust/crates/foo/Android.bp": `
rust_proc_macro {
	name: "libfoo",
	crate_name: "foo",
	srcs: ["src/lib.rs"],
	edition: "2021",
	features: ["bah-enabled"],
	cfgs: ["baz"],
	rustlibs: ["libbar"],
    bazel_module: { bp2build_available: true },
}
`,
			"external/rust/crates/bar/src/lib.rs": "",
			"external/rust/crates/bar/Android.bp": `
rust_library_host {
    name: "libbar",
    crate_name: "bar",
    srcs: ["src/lib.rs"],
    bazel_module: { bp2build_available: true },
}`,
		},
		ExpectedBazelTargets: []string{
			makeBazelTargetHostOrDevice("rust_proc_macro", "libfoo", AttrNameToString{
				"crate_name": `"foo"`,
				"srcs": `[
        "src/helper.rs",
        "src/lib.rs",
    ]`,
				"crate_features": `["bah-enabled"]`,
				"edition":        `"2021"`,
				"rustc_flags":    `["--cfg=baz"]`,
				"deps":           `["//external/rust/crates/bar:libbar"]`,
			}, android.HostSupported),
		},
	},
	)
}
