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
	"testing"

	"github.com/google/blueprint/bootstrap"

	"android/soong/android"
)

func runGoTests(t *testing.T, tc Bp2buildTestCase) {
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {
		tCtx := ctx.(*android.TestContext)
		bootstrap.RegisterGoModuleTypes(tCtx.Context.Context) // android.TestContext --> android.Context --> blueprint.Context
	}, tc)
}

func TestConvertGoPackage(t *testing.T) {
	bp := `
bootstrap_go_package {
	name: "foo",
	pkgPath: "android/foo",
	deps: [
		"bar",
	],
	srcs: [
		"foo1.go",
		"foo2.go",
	],
	linux: {
		srcs: [
			"foo_linux.go",
		],
		testSrcs: [
			"foo_linux_test.go",
		],
	},
	darwin: {
		srcs: [
			"foo_darwin.go",
		],
		testSrcs: [
			"foo_darwin_test.go",
		],
	},
	testSrcs: [
		"foo1_test.go",
		"foo2_test.go",
	],
}
`
	depBp := `
bootstrap_go_package {
	name: "bar",
}
`
	t.Parallel()
	runGoTests(t, Bp2buildTestCase{
		Description:         "Convert bootstrap_go_package to go_library",
		ModuleTypeUnderTest: "bootrstap_go_package",
		Blueprint:           bp,
		Filesystem: map[string]string{
			"bar/Android.bp": depBp, // Put dep in Android.bp to reduce boilerplate in ExpectedBazelTargets
		},
		ExpectedBazelTargets: []string{makeBazelTargetHostOrDevice("go_library", "foo",
			AttrNameToString{
				"deps":       `["//bar:bar"]`,
				"importpath": `"android/foo"`,
				"srcs": `[
        "foo1.go",
        "foo2.go",
    ] + select({
        "//build/bazel_common_rules/platforms/os:darwin": ["foo_darwin.go"],
        "//build/bazel_common_rules/platforms/os:linux_glibc": ["foo_linux.go"],
        "//conditions:default": [],
    })`,
			},
			android.HostSupported,
		),
			makeBazelTargetHostOrDevice("go_test", "foo-test",
				AttrNameToString{
					"embed": `[":foo"]`,
					"srcs": `[
        "foo1_test.go",
        "foo2_test.go",
    ] + select({
        "//build/bazel_common_rules/platforms/os:darwin": ["foo_darwin_test.go"],
        "//build/bazel_common_rules/platforms/os:linux_glibc": ["foo_linux_test.go"],
        "//conditions:default": [],
    })`,
				},
				android.HostSupported,
			)},
	})
}

func TestConvertGoBinaryWithTransitiveDeps(t *testing.T) {
	bp := `
blueprint_go_binary {
	name: "foo",
	srcs: ["main.go"],
	deps: ["bar"],
}
`
	depBp := `
bootstrap_go_package {
	name: "bar",
	deps: ["baz"],
}
bootstrap_go_package {
	name: "baz",
}
`
	t.Parallel()
	runGoTests(t, Bp2buildTestCase{
		Description: "Convert blueprint_go_binary to go_binary",
		Blueprint:   bp,
		Filesystem: map[string]string{
			"bar/Android.bp": depBp, // Put dep in Android.bp to reduce boilerplate in ExpectedBazelTargets
		},
		ExpectedBazelTargets: []string{makeBazelTargetHostOrDevice("go_binary", "foo",
			AttrNameToString{
				"deps": `[
        "//bar:bar",
        "//bar:baz",
    ]`,
				"srcs": `["main.go"]`,
			},
			android.HostSupported,
		)},
	})
}

func TestConvertGoBinaryWithTestSrcs(t *testing.T) {
	bp := `
blueprint_go_binary {
	name: "foo",
	srcs: ["main.go"],
	testSrcs: ["main_test.go"],
}
`
	t.Parallel()
	runGoTests(t, Bp2buildTestCase{
		Description: "Convert blueprint_go_binary with testSrcs",
		Blueprint:   bp,
		ExpectedBazelTargets: []string{
			makeBazelTargetHostOrDevice("go_binary", "foo",
				AttrNameToString{
					"deps":  `[]`,
					"embed": `[":foo-source"]`,
				},
				android.HostSupported,
			),
			makeBazelTargetHostOrDevice("go_source", "foo-source",
				AttrNameToString{
					"deps": `[]`,
					"srcs": `["main.go"]`,
				},
				android.HostSupported,
			),
			makeBazelTargetHostOrDevice("go_test", "foo-test",
				AttrNameToString{
					"embed": `[":foo-source"]`,
					"srcs":  `["main_test.go"]`,
				},
				android.HostSupported,
			),
		},
	})
}

func TestConvertGoBinaryWithSrcInDifferentPackage(t *testing.T) {
	bp := `
blueprint_go_binary {
	name: "foo",
	srcs: ["subdir/main.go"],
}
`
	t.Parallel()
	runGoTests(t, Bp2buildTestCase{
		Description: "Convert blueprint_go_binary with src in different package",
		Blueprint:   bp,
		Filesystem: map[string]string{
			"subdir/Android.bp": "",
		},
		ExpectedBazelTargets: []string{makeBazelTargetHostOrDevice("go_binary", "foo",
			AttrNameToString{
				"deps": `[]`,
				"srcs": `["//subdir:main.go"]`,
			},
			android.HostSupported,
		)},
	})
}
