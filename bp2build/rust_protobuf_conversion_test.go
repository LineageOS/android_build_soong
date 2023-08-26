package bp2build

import (
	"android/soong/android"
	"android/soong/rust"
	"testing"
)

func runRustProtobufTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, registerRustProtobufModuleTypes, tc)
}

func registerRustProtobufModuleTypes(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("rust_protobuf_host", rust.RustProtobufHostFactory)

}

func TestRustProtobufHostTestCase(t *testing.T) {
	runRustProtobufTestCase(t, Bp2buildTestCase{
		Dir:       "external/rust/crates/foo",
		Blueprint: "",
		Filesystem: map[string]string{
			"external/rust/crates/foo/src/lib.rs":    "",
			"external/rust/crates/foo/src/helper.rs": "",
			"external/rust/crates/foo/Android.bp": `
rust_protobuf_host {
	name: "libfoo",
	crate_name: "foo",
	protos: ["src/foo.proto"],
    bazel_module: { bp2build_available: true },
}
`,
		},
		ExpectedBazelTargets: []string{
			makeBazelTargetHostOrDevice("proto_library", "libfoo_proto", AttrNameToString{
				"srcs": `["src/foo.proto"]`,
			}, android.HostSupported),
			makeBazelTargetHostOrDevice("rust_proto_library", "libfoo", AttrNameToString{
				"crate_name": `"foo"`,
				"deps":       `[":libfoo_proto"]`,
			}, android.HostSupported),
		},
	},
	)
}
