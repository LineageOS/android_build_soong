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

func runRustLibraryTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, registerRustLibraryModuleTypes, tc)
}

func registerRustLibraryModuleTypes(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("rust_library", rust.RustLibraryFactory)
	ctx.RegisterModuleType("rust_library_host", rust.RustLibraryHostFactory)
}

func TestLibProtobuf(t *testing.T) {
	runRustLibraryTestCase(t, Bp2buildTestCase{
		Dir:       "external/rust/crates/foo",
		Blueprint: "",
		Filesystem: map[string]string{
			"external/rust/crates/foo/src/lib.rs": "",
			"external/rust/crates/foo/Android.bp": `
rust_library_host {
	name: "libprotobuf",
	crate_name: "protobuf",
	srcs: ["src/lib.rs"],
    bazel_module: { bp2build_available: true },
}
`,
		},
		ExpectedBazelTargets: []string{
			// TODO(b/290790800): Remove the restriction when rust toolchain for android is implemented
			makeBazelTargetHostOrDevice("rust_library", "libprotobuf", AttrNameToString{
				"crate_name": `"protobuf"`,
				"srcs":       `["src/lib.rs"]`,
				"deps":       `[":libprotobuf_build_script"]`,
			}, android.HostSupported),
			makeBazelTargetHostOrDevice("cargo_build_script", "libprotobuf_build_script", AttrNameToString{
				"srcs": `["build.rs"]`,
			}, android.HostSupported),
		},
	},
	)
}

func TestRustLibrary(t *testing.T) {
	expectedAttrs := AttrNameToString{
		"crate_name": `"foo"`,
		"srcs": `[
        "src/helper.rs",
        "src/lib.rs",
    ]`,
		"crate_features": `["bah-enabled"]`,
		"edition":        `"2021"`,
		"rustc_flags":    `["--cfg=baz"]`,
	}

	runRustLibraryTestCase(t, Bp2buildTestCase{
		Dir:       "external/rust/crates/foo",
		Blueprint: "",
		Filesystem: map[string]string{
			"external/rust/crates/foo/src/lib.rs":    "",
			"external/rust/crates/foo/src/helper.rs": "",
			"external/rust/crates/foo/Android.bp": `
rust_library {
	name: "libfoo",
	crate_name: "foo",
    host_supported: true,
	srcs: ["src/lib.rs"],
	edition: "2021",
	features: ["bah-enabled"],
	cfgs: ["baz"],
    bazel_module: { bp2build_available: true },
}
rust_library_host {
    name: "libfoo_host",
    crate_name: "foo",
    srcs: ["src/lib.rs"],
    edition: "2021",
    features: ["bah-enabled"],
    cfgs: ["baz"],
    bazel_module: { bp2build_available: true },
}
`,
		},
		ExpectedBazelTargets: []string{
			// TODO(b/290790800): Remove the restriction when rust toolchain for android is implemented
			makeBazelTargetHostOrDevice("rust_library", "libfoo", expectedAttrs, android.HostSupported),
			makeBazelTargetHostOrDevice("rust_library", "libfoo_host", expectedAttrs, android.HostSupported),
		},
	},
	)
}
