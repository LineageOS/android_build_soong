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

func rustRustProcMacroTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, registerRustProcMacroModuleTypes, tc)
}

func registerRustProcMacroModuleTypes(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("rust_library_host", rust.RustLibraryHostFactory)
	ctx.RegisterModuleType("rust_proc_macro", rust.ProcMacroFactory)
}

func TestRustProcMacroLibrary(t *testing.T) {
	rustRustProcMacroTestCase(t, Bp2buildTestCase{
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
