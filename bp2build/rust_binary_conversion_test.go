package bp2build

import (
	"android/soong/android"
	"android/soong/rust"
	"testing"
)

func runRustBinaryTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, registerRustBinaryModuleTypes, tc)
}

func registerRustBinaryModuleTypes(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("rust_binary_host", rust.RustBinaryHostFactory)
	ctx.RegisterModuleType("rust_library_host", rust.RustLibraryHostFactory)
	ctx.RegisterModuleType("rust_proc_macro", rust.ProcMacroFactory)

}

func TestRustBinaryHost(t *testing.T) {
	runRustBinaryTestCase(t, Bp2buildTestCase{
		Dir:       "external/rust/crates/foo",
		Blueprint: "",
		Filesystem: map[string]string{
			"external/rust/crates/foo/src/lib.rs":    "",
			"external/rust/crates/foo/src/helper.rs": "",
			"external/rust/crates/foo/Android.bp": `
rust_binary_host {
	name: "libfoo",
	crate_name: "foo",
	srcs: ["src/main.rs"],
	edition: "2021",
	features: ["bah-enabled"],
	cfgs: ["baz"],
	rustlibs: ["libbar"],
	proc_macros: ["libbah"],
    bazel_module: { bp2build_available: true },
}
`,
			"external/rust/crates/bar/Android.bp": `
rust_library_host {
	name: "libbar",
	crate_name: "bar",
	srcs: ["src/lib.rs"],
    bazel_module: { bp2build_available: true },
}
`,
			"external/rust/crates/bah/Android.bp": `
rust_proc_macro {
	name: "libbah",
	crate_name: "bah",
	srcs: ["src/lib.rs"],
    bazel_module: { bp2build_available: true },
}
`,
		},
		ExpectedBazelTargets: []string{
			makeBazelTargetHostOrDevice("rust_binary", "libfoo", AttrNameToString{
				"crate_name": `"foo"`,
				"srcs": `[
        "src/helper.rs",
        "src/lib.rs",
    ]`,
				"deps":            `["//external/rust/crates/bar:libbar"]`,
				"proc_macro_deps": `["//external/rust/crates/bah:libbah"]`,
				"edition":         `"2021"`,
				"crate_features":  `["bah-enabled"]`,
				"rustc_flags":     `["--cfg=baz"]`,
			}, android.HostSupported),
		},
	},
	)
}
