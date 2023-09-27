// Copyright 2023 Google Inc. All rights reserved.
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
	"android/soong/rust"
	"testing"
)

func runRustProtobufTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, registerRustProtobufModuleTypes, tc)
}

func registerRustProtobufModuleTypes(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("rust_protobuf_host", rust.RustProtobufHostFactory)
	ctx.RegisterModuleType("rust_protobuf", rust.RustProtobufHostFactory)
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

func TestRustProtobufTestCase(t *testing.T) {
	runRustProtobufTestCase(t, Bp2buildTestCase{
		Dir:       "external/rust/crates/foo",
		Blueprint: "",
		Filesystem: map[string]string{
			"external/rust/crates/foo/src/lib.rs":    "",
			"external/rust/crates/foo/src/helper.rs": "",
			"external/rust/crates/foo/Android.bp": `
rust_protobuf {
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
