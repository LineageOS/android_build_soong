// Copyright 2019 Google Inc. All rights reserved.
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

package cc

import (
	"fmt"
	"runtime"
	"testing"

	"android/soong/android"
	"github.com/google/blueprint"
)

var prepareForPrebuiltTest = android.GroupFixturePreparers(
	prepareForCcTest,
	android.PrepareForTestWithAndroidMk,
)

func testPrebuilt(t *testing.T, bp string, fs android.MockFS, handlers ...android.FixturePreparer) *android.TestContext {
	t.Helper()
	result := android.GroupFixturePreparers(
		prepareForPrebuiltTest,
		fs.AddToFixture(),
		android.GroupFixturePreparers(handlers...),
	).RunTestWithBp(t, bp)

	return result.TestContext
}

type configCustomizer func(config android.Config)

func TestPrebuilt(t *testing.T) {
	bp := `
		cc_library {
			name: "liba",
		}

		cc_prebuilt_library_shared {
			name: "liba",
			srcs: ["liba.so"],
		}

		cc_library {
			name: "libb",
		}

		cc_prebuilt_library_static {
			name: "libb",
			srcs: ["libb.a"],
		}

		cc_library_shared {
			name: "libd",
		}

		cc_prebuilt_library_shared {
			name: "libd",
			srcs: ["libd.so"],
		}

		cc_library_static {
			name: "libe",
		}

		cc_prebuilt_library_static {
			name: "libe",
			srcs: ["libe.a"],
		}

		cc_library {
			name: "libf",
		}

		cc_prebuilt_library {
			name: "libf",
			static: {
				srcs: ["libf.a"],
			},
			shared: {
				srcs: ["libf.so"],
			},
		}

		cc_object {
			name: "crtx",
		}

		cc_prebuilt_object {
			name: "crtx",
			srcs: ["crtx.o"],
		}
	`

	ctx := testPrebuilt(t, bp, map[string][]byte{
		"liba.so": nil,
		"libb.a":  nil,
		"libd.so": nil,
		"libe.a":  nil,
		"libf.a":  nil,
		"libf.so": nil,
		"crtx.o":  nil,
	})

	// Verify that all the modules exist and that their dependencies were connected correctly
	liba := ctx.ModuleForTests("liba", "android_arm64_armv8-a_shared").Module()
	libb := ctx.ModuleForTests("libb", "android_arm64_armv8-a_static").Module()
	libd := ctx.ModuleForTests("libd", "android_arm64_armv8-a_shared").Module()
	libe := ctx.ModuleForTests("libe", "android_arm64_armv8-a_static").Module()
	libfStatic := ctx.ModuleForTests("libf", "android_arm64_armv8-a_static").Module()
	libfShared := ctx.ModuleForTests("libf", "android_arm64_armv8-a_shared").Module()
	crtx := ctx.ModuleForTests("crtx", "android_arm64_armv8-a").Module()

	prebuiltLiba := ctx.ModuleForTests("prebuilt_liba", "android_arm64_armv8-a_shared").Module()
	prebuiltLibb := ctx.ModuleForTests("prebuilt_libb", "android_arm64_armv8-a_static").Module()
	prebuiltLibd := ctx.ModuleForTests("prebuilt_libd", "android_arm64_armv8-a_shared").Module()
	prebuiltLibe := ctx.ModuleForTests("prebuilt_libe", "android_arm64_armv8-a_static").Module()
	prebuiltLibfStatic := ctx.ModuleForTests("prebuilt_libf", "android_arm64_armv8-a_static").Module()
	prebuiltLibfShared := ctx.ModuleForTests("prebuilt_libf", "android_arm64_armv8-a_shared").Module()
	prebuiltCrtx := ctx.ModuleForTests("prebuilt_crtx", "android_arm64_armv8-a").Module()

	hasDep := func(m android.Module, wantDep android.Module) bool {
		t.Helper()
		var found bool
		ctx.VisitDirectDeps(m, func(dep blueprint.Module) {
			if dep == wantDep {
				found = true
			}
		})
		return found
	}

	if !hasDep(liba, prebuiltLiba) {
		t.Errorf("liba missing dependency on prebuilt_liba")
	}

	if !hasDep(libb, prebuiltLibb) {
		t.Errorf("libb missing dependency on prebuilt_libb")
	}

	if !hasDep(libd, prebuiltLibd) {
		t.Errorf("libd missing dependency on prebuilt_libd")
	}

	if !hasDep(libe, prebuiltLibe) {
		t.Errorf("libe missing dependency on prebuilt_libe")
	}

	if !hasDep(libfStatic, prebuiltLibfStatic) {
		t.Errorf("libf static missing dependency on prebuilt_libf")
	}

	if !hasDep(libfShared, prebuiltLibfShared) {
		t.Errorf("libf shared missing dependency on prebuilt_libf")
	}

	if !hasDep(crtx, prebuiltCrtx) {
		t.Errorf("crtx missing dependency on prebuilt_crtx")
	}

	entries := android.AndroidMkEntriesForTest(t, ctx, prebuiltLiba)[0]
	android.AssertStringEquals(t, "unexpected LOCAL_SOONG_MODULE_TYPE", "cc_prebuilt_library_shared", entries.EntryMap["LOCAL_SOONG_MODULE_TYPE"][0])
	entries = android.AndroidMkEntriesForTest(t, ctx, prebuiltLibb)[0]
	android.AssertStringEquals(t, "unexpected LOCAL_SOONG_MODULE_TYPE", "cc_prebuilt_library_static", entries.EntryMap["LOCAL_SOONG_MODULE_TYPE"][0])
}

func TestPrebuiltLibraryShared(t *testing.T) {
	ctx := testPrebuilt(t, `
	cc_prebuilt_library_shared {
		name: "libtest",
		srcs: ["libf.so"],
    strip: {
        none: true,
    },
	}
	`, map[string][]byte{
		"libf.so": nil,
	})

	shared := ctx.ModuleForTests("libtest", "android_arm64_armv8-a_shared").Module().(*Module)
	assertString(t, shared.OutputFile().Path().Base(), "libtest.so")
}

func TestPrebuiltLibraryStatic(t *testing.T) {
	ctx := testPrebuilt(t, `
	cc_prebuilt_library_static {
		name: "libtest",
		srcs: ["libf.a"],
	}
	`, map[string][]byte{
		"libf.a": nil,
	})

	static := ctx.ModuleForTests("libtest", "android_arm64_armv8-a_static").Module().(*Module)
	assertString(t, static.OutputFile().Path().Base(), "libf.a")
}

func TestPrebuiltLibrary(t *testing.T) {
	ctx := testPrebuilt(t, `
	cc_prebuilt_library {
		name: "libtest",
		static: {
			srcs: ["libf.a"],
		},
		shared: {
			srcs: ["libf.so"],
		},
    strip: {
        none: true,
    },
	}
	`, map[string][]byte{
		"libf.a":  nil,
		"libf.so": nil,
	})

	shared := ctx.ModuleForTests("libtest", "android_arm64_armv8-a_shared").Module().(*Module)
	assertString(t, shared.OutputFile().Path().Base(), "libtest.so")

	static := ctx.ModuleForTests("libtest", "android_arm64_armv8-a_static").Module().(*Module)
	assertString(t, static.OutputFile().Path().Base(), "libf.a")
}

func TestPrebuiltLibraryStem(t *testing.T) {
	ctx := testPrebuilt(t, `
	cc_prebuilt_library {
		name: "libfoo",
		stem: "libbar",
		static: {
			srcs: ["libfoo.a"],
		},
		shared: {
			srcs: ["libfoo.so"],
		},
		strip: {
			none: true,
		},
	}
	`, map[string][]byte{
		"libfoo.a":  nil,
		"libfoo.so": nil,
	})

	static := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_static").Module().(*Module)
	assertString(t, static.OutputFile().Path().Base(), "libfoo.a")

	shared := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module().(*Module)
	assertString(t, shared.OutputFile().Path().Base(), "libbar.so")
}

func TestPrebuiltLibrarySharedStem(t *testing.T) {
	ctx := testPrebuilt(t, `
	cc_prebuilt_library_shared {
		name: "libfoo",
		stem: "libbar",
		srcs: ["libfoo.so"],
		strip: {
			none: true,
		},
	}
	`, map[string][]byte{
		"libfoo.so": nil,
	})

	shared := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module().(*Module)
	assertString(t, shared.OutputFile().Path().Base(), "libbar.so")
}

func TestPrebuiltSymlinkedHostBinary(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("Skipping host prebuilt testing that is only supported on linux not %s", runtime.GOOS)
	}

	ctx := testPrebuilt(t, `
	cc_prebuilt_library_shared {
		name: "libfoo",
		device_supported: false,
		host_supported: true,
		target: {
			linux_glibc_x86_64: {
				srcs: ["linux_glibc_x86_64/lib64/libfoo.so"],
			},
		},
	}

	cc_prebuilt_binary {
		name: "foo",
		device_supported: false,
		host_supported: true,
		shared_libs: ["libfoo"],
		target: {
			linux_glibc_x86_64: {
				srcs: ["linux_glibc_x86_64/bin/foo"],
			},
		},
	}
	`, map[string][]byte{
		"libfoo.so": nil,
		"foo":       nil,
	})

	fooRule := ctx.ModuleForTests("foo", "linux_glibc_x86_64").Rule("Symlink")
	assertString(t, fooRule.Output.String(), "out/soong/.intermediates/foo/linux_glibc_x86_64/foo")
	assertString(t, fooRule.Args["fromPath"], "$$PWD/linux_glibc_x86_64/bin/foo")

	var libfooDep android.Path
	for _, dep := range fooRule.Implicits {
		if dep.Base() == "libfoo.so" {
			libfooDep = dep
			break
		}
	}
	assertString(t, libfooDep.String(), "out/soong/.intermediates/libfoo/linux_glibc_x86_64_shared/libfoo.so")
}

func TestPrebuiltLibrarySanitized(t *testing.T) {
	bp := `cc_prebuilt_library {
	name: "libtest",
		static: {
                        sanitized: { none: { srcs: ["libf.a"], }, hwaddress: { srcs: ["libf.hwasan.a"], }, },
		},
		shared: {
                        sanitized: { none: { srcs: ["libf.so"], }, hwaddress: { srcs: ["hwasan/libf.so"], }, },
		},
	}
	cc_prebuilt_library_static {
		name: "libtest_static",
                sanitized: { none: { srcs: ["libf.a"], }, hwaddress: { srcs: ["libf.hwasan.a"], }, },
	}
	cc_prebuilt_library_shared {
		name: "libtest_shared",
                sanitized: { none: { srcs: ["libf.so"], }, hwaddress: { srcs: ["hwasan/libf.so"], }, },
	}`

	fs := map[string][]byte{
		"libf.a":         nil,
		"libf.hwasan.a":  nil,
		"libf.so":        nil,
		"hwasan/libf.so": nil,
	}

	// Without SANITIZE_TARGET.
	ctx := testPrebuilt(t, bp, fs)

	shared_rule := ctx.ModuleForTests("libtest", "android_arm64_armv8-a_shared").Rule("android/soong/cc.strip")
	assertString(t, shared_rule.Input.String(), "libf.so")

	static := ctx.ModuleForTests("libtest", "android_arm64_armv8-a_static").Module().(*Module)
	assertString(t, static.OutputFile().Path().Base(), "libf.a")

	shared_rule2 := ctx.ModuleForTests("libtest_shared", "android_arm64_armv8-a_shared").Rule("android/soong/cc.strip")
	assertString(t, shared_rule2.Input.String(), "libf.so")

	static2 := ctx.ModuleForTests("libtest_static", "android_arm64_armv8-a_static").Module().(*Module)
	assertString(t, static2.OutputFile().Path().Base(), "libf.a")

	// With SANITIZE_TARGET=hwaddress
	ctx = testPrebuilt(t, bp, fs,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.SanitizeDevice = []string{"hwaddress"}
		}),
	)

	shared_rule = ctx.ModuleForTests("libtest", "android_arm64_armv8-a_shared_hwasan").Rule("android/soong/cc.strip")
	assertString(t, shared_rule.Input.String(), "hwasan/libf.so")

	static = ctx.ModuleForTests("libtest", "android_arm64_armv8-a_static_hwasan").Module().(*Module)
	assertString(t, static.OutputFile().Path().Base(), "libf.hwasan.a")

	shared_rule2 = ctx.ModuleForTests("libtest_shared", "android_arm64_armv8-a_shared_hwasan").Rule("android/soong/cc.strip")
	assertString(t, shared_rule2.Input.String(), "hwasan/libf.so")

	static2 = ctx.ModuleForTests("libtest_static", "android_arm64_armv8-a_static_hwasan").Module().(*Module)
	assertString(t, static2.OutputFile().Path().Base(), "libf.hwasan.a")
}

func TestPrebuiltStubNoinstall(t *testing.T) {
	testFunc := func(t *testing.T, expectLibfooOnSystemLib bool, fs android.MockFS) {
		result := android.GroupFixturePreparers(
			prepareForPrebuiltTest,
			android.PrepareForTestWithMakevars,
			android.FixtureMergeMockFs(fs),
		).RunTest(t)

		ldRule := result.ModuleForTests("installedlib", "android_arm64_armv8-a_shared").Rule("ld")
		android.AssertStringDoesContain(t, "", ldRule.Args["libFlags"], "android_arm64_armv8-a_shared/libfoo.so")

		installRules := result.InstallMakeRulesForTesting(t)
		var installedlibRule *android.InstallMakeRule
		for i, rule := range installRules {
			if rule.Target == "out/target/product/test_device/system/lib/installedlib.so" {
				if installedlibRule != nil {
					t.Errorf("Duplicate install rules for %s", rule.Target)
				}
				installedlibRule = &installRules[i]
			}
		}
		if installedlibRule == nil {
			t.Errorf("No install rule found for installedlib")
			return
		}

		if expectLibfooOnSystemLib {
			android.AssertStringListContains(t,
				"installedlib doesn't have install dependency on libfoo impl",
				installedlibRule.OrderOnlyDeps,
				"out/target/product/test_device/system/lib/libfoo.so")
		} else {
			android.AssertStringListDoesNotContain(t,
				"installedlib has install dependency on libfoo stub",
				installedlibRule.Deps,
				"out/target/product/test_device/system/lib/libfoo.so")
			android.AssertStringListDoesNotContain(t,
				"installedlib has order-only install dependency on libfoo stub",
				installedlibRule.OrderOnlyDeps,
				"out/target/product/test_device/system/lib/libfoo.so")
		}
	}

	prebuiltLibfooBp := []byte(`
		cc_prebuilt_library {
			name: "libfoo",
			prefer: true,
			srcs: ["libfoo.so"],
			stubs: {
				versions: ["1"],
			},
		}
	`)

	installedlibBp := []byte(`
		cc_library {
			name: "installedlib",
			shared_libs: ["libfoo"],
		}
	`)

	t.Run("prebuilt stub (without source): no install", func(t *testing.T) {
		testFunc(
			t,
			/*expectLibfooOnSystemLib=*/ false,
			android.MockFS{
				"prebuilts/module_sdk/art/current/Android.bp": prebuiltLibfooBp,
				"Android.bp": installedlibBp,
			},
		)
	})

	disabledSourceLibfooBp := []byte(`
		cc_library {
			name: "libfoo",
			enabled: false,
			stubs: {
				versions: ["1"],
			},
		}
	`)

	t.Run("prebuilt stub (with disabled source): no install", func(t *testing.T) {
		testFunc(
			t,
			/*expectLibfooOnSystemLib=*/ false,
			android.MockFS{
				"prebuilts/module_sdk/art/current/Android.bp": prebuiltLibfooBp,
				"impl/Android.bp": disabledSourceLibfooBp,
				"Android.bp":      installedlibBp,
			},
		)
	})

	t.Run("prebuilt impl (with `stubs` property set): install", func(t *testing.T) {
		testFunc(
			t,
			/*expectLibfooOnSystemLib=*/ true,
			android.MockFS{
				"impl/Android.bp": prebuiltLibfooBp,
				"Android.bp":      installedlibBp,
			},
		)
	})
}

func TestPrebuiltBinaryNoSrcsNoError(t *testing.T) {
	const bp = `
cc_prebuilt_binary {
	name: "bintest",
	srcs: [],
}`
	ctx := testPrebuilt(t, bp, map[string][]byte{})
	mod := ctx.ModuleForTests("bintest", "android_arm64_armv8-a").Module().(*Module)
	android.AssertBoolEquals(t, `expected no srcs to yield no output file`, false, mod.OutputFile().Valid())
}

func TestPrebuiltBinaryMultipleSrcs(t *testing.T) {
	const bp = `
cc_prebuilt_binary {
	name: "bintest",
	srcs: ["foo", "bar"],
}`
	testCcError(t, `Android.bp:4:6: module "bintest" variant "android_arm64_armv8-a": srcs: multiple prebuilt source files`, bp)
}

func TestMultiplePrebuilts(t *testing.T) {
	bp := `
		// an rdep
		cc_library {
			name: "libfoo",
			shared_libs: ["libbar"],
		}

		// multiple variations of dep
		// source
		cc_library {
			name: "libbar",
		}
		// prebuilt "v1"
		cc_prebuilt_library_shared {
			name: "libbar",
			srcs: ["libbar.so"],
		}
		// prebuilt "v2"
		cc_prebuilt_library_shared {
			name: "libbar.v2",
			stem: "libbar",
			source_module_name: "libbar",
			srcs: ["libbar.so"],
		}

		// selectors
		apex_contributions {
			name: "myapex_contributions",
			contents: ["%v"],
		}
		all_apex_contributions {name: "all_apex_contributions"}
	`
	hasDep := func(ctx *android.TestContext, m android.Module, wantDep android.Module) bool {
		t.Helper()
		var found bool
		ctx.VisitDirectDeps(m, func(dep blueprint.Module) {
			if dep == wantDep {
				found = true
			}
		})
		return found
	}

	testCases := []struct {
		desc                   string
		selectedDependencyName string
		expectedDependencyName string
	}{
		{
			desc:                   "Source library is selected using apex_contributions",
			selectedDependencyName: "libbar",
			expectedDependencyName: "libbar",
		},
		{
			desc:                   "Prebuilt library v1 is selected using apex_contributions",
			selectedDependencyName: "prebuilt_libbar",
			expectedDependencyName: "prebuilt_libbar",
		},
		{
			desc:                   "Prebuilt library v2 is selected using apex_contributions",
			selectedDependencyName: "prebuilt_libbar.v2",
			expectedDependencyName: "prebuilt_libbar.v2",
		},
	}

	for _, tc := range testCases {
		preparer := android.GroupFixturePreparers(
			android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
				android.RegisterApexContributionsBuildComponents(ctx)
			}),
			android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				variables.BuildFlags = map[string]string{
					"RELEASE_APEX_CONTRIBUTIONS_ADSERVICES": "myapex_contributions",
				}
			}),
		)
		ctx := testPrebuilt(t, fmt.Sprintf(bp, tc.selectedDependencyName), map[string][]byte{
			"libbar.so": nil,
			"crtx.o":    nil,
		}, preparer)
		libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
		expectedDependency := ctx.ModuleForTests(tc.expectedDependencyName, "android_arm64_armv8-a_shared").Module()
		android.AssertBoolEquals(t, fmt.Sprintf("expected dependency from %s to %s\n", libfoo.Name(), tc.expectedDependencyName), true, hasDep(ctx, libfoo, expectedDependency))
		// check that LOCAL_SHARED_LIBRARIES contains libbar and not libbar.v<N>
		entries := android.AndroidMkEntriesForTest(t, ctx, libfoo)[0]
		android.AssertStringListContains(t, "Version should not be present in LOCAL_SHARED_LIBRARIES", entries.EntryMap["LOCAL_SHARED_LIBRARIES"], "libbar")

		// check installation rules
		// the selected soong module should be exported to make
		libbar := ctx.ModuleForTests(tc.expectedDependencyName, "android_arm64_armv8-a_shared").Module()
		android.AssertBoolEquals(t, fmt.Sprintf("dependency %s should be exported to make\n", expectedDependency), true, !libbar.IsHideFromMake())

		// check LOCAL_MODULE of the selected module name
		// the prebuilt should have the same LOCAL_MODULE when exported to make
		entries = android.AndroidMkEntriesForTest(t, ctx, libbar)[0]
		android.AssertStringEquals(t, "unexpected LOCAL_MODULE", "libbar", entries.EntryMap["LOCAL_MODULE"][0])
	}
}

// Setting prefer on multiple prebuilts is an error, unless one of them is also listed in apex_contributions
func TestMultiplePrebuiltsPreferredUsingLegacyFlags(t *testing.T) {
	bp := `
		// an rdep
		cc_library {
			name: "libfoo",
			shared_libs: ["libbar"],
		}

		// multiple variations of dep
		// source
		cc_library {
			name: "libbar",
		}
		// prebuilt "v1"
		cc_prebuilt_library_shared {
			name: "libbar",
			srcs: ["libbar.so"],
			prefer: true,
		}
		// prebuilt "v2"
		cc_prebuilt_library_shared {
			name: "libbar.v2",
			stem: "libbar",
			source_module_name: "libbar",
			srcs: ["libbar.so"],
			prefer: true,
		}

		// selectors
		apex_contributions {
			name: "myapex_contributions",
			contents: [%v],
		}
		all_apex_contributions {name: "all_apex_contributions"}
	`
	hasDep := func(ctx *android.TestContext, m android.Module, wantDep android.Module) bool {
		t.Helper()
		var found bool
		ctx.VisitDirectDeps(m, func(dep blueprint.Module) {
			if dep == wantDep {
				found = true
			}
		})
		return found
	}

	testCases := []struct {
		desc                   string
		selectedDependencyName string
		expectedDependencyName string
		expectedErr            string
	}{
		{
			desc:        "Multiple prebuilts have prefer: true",
			expectedErr: "Multiple prebuilt modules prebuilt_libbar and prebuilt_libbar.v2 have been marked as preferred for this source module",
		},
		{
			desc:                   "Multiple prebuilts have prefer: true. The prebuilt listed in apex_contributions wins.",
			selectedDependencyName: `"prebuilt_libbar"`,
			expectedDependencyName: "prebuilt_libbar",
		},
	}

	for _, tc := range testCases {
		preparer := android.GroupFixturePreparers(
			android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
				android.RegisterApexContributionsBuildComponents(ctx)
			}),
			android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				variables.BuildFlags = map[string]string{
					"RELEASE_APEX_CONTRIBUTIONS_ADSERVICES": "myapex_contributions",
				}
			}),
		)
		if tc.expectedErr != "" {
			preparer = preparer.ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(tc.expectedErr))
		}

		ctx := testPrebuilt(t, fmt.Sprintf(bp, tc.selectedDependencyName), map[string][]byte{
			"libbar.so": nil,
			"crtx.o":    nil,
		}, preparer)
		if tc.expectedErr != "" {
			return // the fixture will assert that the excepted err has been raised
		}
		libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
		expectedDependency := ctx.ModuleForTests(tc.expectedDependencyName, "android_arm64_armv8-a_shared").Module()
		android.AssertBoolEquals(t, fmt.Sprintf("expected dependency from %s to %s\n", libfoo.Name(), tc.expectedDependencyName), true, hasDep(ctx, libfoo, expectedDependency))
	}
}

// If module sdk cannot provide a cc module variant (e.g. static), then the module variant from source should be used
func TestMissingVariantInModuleSdk(t *testing.T) {
	bp := `
		// an rdep
		cc_library {
			name: "libfoo",
			static_libs: ["libbar"],
		}

		// source
		cc_library {
			name: "libbar",
		}
		// prebuilt
		// libbar only exists as a shared library
		cc_prebuilt_library_shared {
			name: "libbar",
			srcs: ["libbar.so"],
		}
		// selectors
		apex_contributions {
			name: "myapex_contributions",
			contents: ["prebuilt_libbar"],
		}
		all_apex_contributions {name: "all_apex_contributions"}
	`
	hasDep := func(ctx *android.TestContext, m android.Module, wantDep android.Module) bool {
		t.Helper()
		var found bool
		ctx.VisitDirectDeps(m, func(dep blueprint.Module) {
			if dep == wantDep {
				found = true
			}
		})
		return found
	}

	preparer := android.GroupFixturePreparers(
		android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
			android.RegisterApexContributionsBuildComponents(ctx)
		}),
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.BuildFlags = map[string]string{
				"RELEASE_APEX_CONTRIBUTIONS_ADSERVICES": "myapex_contributions",
			}
		}),
	)
	ctx := testPrebuilt(t, bp, map[string][]byte{
		"libbar.so": nil,
		"crtx.o":    nil,
	}, preparer)
	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
	sourceLibBar := ctx.ModuleForTests("libbar", "android_arm64_armv8-a_static").Module()
	// Even though the prebuilt is listed in apex_contributions, the prebuilt does not have a static variant.
	// Therefore source of libbar should be used.
	android.AssertBoolEquals(t, fmt.Sprintf("expected dependency from libfoo to source libbar"), true, hasDep(ctx, libfoo, sourceLibBar))
}
