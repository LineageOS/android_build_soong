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
	"runtime"
	"testing"

	"android/soong/android"
	"android/soong/bazel/cquery"

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

func TestPrebuiltLibraryWithBazel(t *testing.T) {
	const bp = `
cc_prebuilt_library {
	name: "foo",
	shared: {
		srcs: ["foo.so"],
	},
	static: {
		srcs: ["foo.a"],
	},
	bazel_module: { label: "//foo/bar:bar" },
}`
	outBaseDir := "outputbase"
	result := android.GroupFixturePreparers(
		prepareForPrebuiltTest,
		android.FixtureModifyConfig(func(config android.Config) {
			config.BazelContext = android.MockBazelContext{
				OutputBaseDir: outBaseDir,
				LabelToCcInfo: map[string]cquery.CcInfo{
					"//foo/bar:bar": cquery.CcInfo{
						CcSharedLibraryFiles: []string{"foo.so"},
					},
					"//foo/bar:bar_bp2build_cc_library_static": cquery.CcInfo{
						CcStaticLibraryFiles: []string{"foo.a"},
					},
				},
			}
		}),
	).RunTestWithBp(t, bp)
	sharedFoo := result.ModuleForTests("foo", "android_arm_armv7-a-neon_shared").Module()
	pathPrefix := outBaseDir + "/execroot/__main__/"

	sharedInfo := result.ModuleProvider(sharedFoo, SharedLibraryInfoProvider).(SharedLibraryInfo)
	android.AssertPathRelativeToTopEquals(t,
		"prebuilt library shared target path did not exist or did not match expected. If the base path is what does not match, it is likely that Soong built this module instead of Bazel.",
		pathPrefix+"foo.so", sharedInfo.SharedLibrary)

	outputFiles, err := sharedFoo.(android.OutputFileProducer).OutputFiles("")
	if err != nil {
		t.Errorf("Unexpected error getting cc_object outputfiles %s", err)
	}
	expectedOutputFiles := []string{pathPrefix + "foo.so"}
	android.AssertDeepEquals(t,
		"prebuilt library shared target output files did not match expected.",
		expectedOutputFiles, outputFiles.Strings())

	staticFoo := result.ModuleForTests("foo", "android_arm_armv7-a-neon_static").Module()
	staticInfo := result.ModuleProvider(staticFoo, StaticLibraryInfoProvider).(StaticLibraryInfo)
	android.AssertPathRelativeToTopEquals(t,
		"prebuilt library static target path did not exist or did not match expected. If the base path is what does not match, it is likely that Soong built this module instead of Bazel.",
		pathPrefix+"foo.a", staticInfo.StaticLibrary)

	staticOutputFiles, err := staticFoo.(android.OutputFileProducer).OutputFiles("")
	if err != nil {
		t.Errorf("Unexpected error getting cc_object staticOutputFiles %s", err)
	}
	expectedStaticOutputFiles := []string{pathPrefix + "foo.a"}
	android.AssertDeepEquals(t,
		"prebuilt library static target output files did not match expected.",
		expectedStaticOutputFiles, staticOutputFiles.Strings())
}

func TestPrebuiltLibraryWithBazelStaticDisabled(t *testing.T) {
	const bp = `
cc_prebuilt_library {
	name: "foo",
	shared: {
		srcs: ["foo.so"],
	},
	static: {
		enabled: false
	},
	bazel_module: { label: "//foo/bar:bar" },
}`
	outBaseDir := "outputbase"
	result := android.GroupFixturePreparers(
		prepareForPrebuiltTest,
		android.FixtureModifyConfig(func(config android.Config) {
			config.BazelContext = android.MockBazelContext{
				OutputBaseDir: outBaseDir,
				LabelToCcInfo: map[string]cquery.CcInfo{
					"//foo/bar:bar": cquery.CcInfo{
						CcSharedLibraryFiles: []string{"foo.so"},
					},
				},
			}
		}),
	).RunTestWithBp(t, bp)
	sharedFoo := result.ModuleForTests("foo", "android_arm_armv7-a-neon_shared").Module()
	pathPrefix := outBaseDir + "/execroot/__main__/"

	sharedInfo := result.ModuleProvider(sharedFoo, SharedLibraryInfoProvider).(SharedLibraryInfo)
	android.AssertPathRelativeToTopEquals(t,
		"prebuilt library shared target path did not exist or did not match expected. If the base path is what does not match, it is likely that Soong built this module instead of Bazel.",
		pathPrefix+"foo.so", sharedInfo.SharedLibrary)

	outputFiles, err := sharedFoo.(android.OutputFileProducer).OutputFiles("")
	if err != nil {
		t.Errorf("Unexpected error getting cc_object outputfiles %s", err)
	}
	expectedOutputFiles := []string{pathPrefix + "foo.so"}
	android.AssertDeepEquals(t,
		"prebuilt library shared target output files did not match expected.",
		expectedOutputFiles, outputFiles.Strings())
}

func TestPrebuiltLibraryStaticWithBazel(t *testing.T) {
	const bp = `
cc_prebuilt_library_static {
	name: "foo",
	srcs: ["foo.so"],
	bazel_module: { label: "//foo/bar:bar" },
}`
	outBaseDir := "outputbase"
	result := android.GroupFixturePreparers(
		prepareForPrebuiltTest,
		android.FixtureModifyConfig(func(config android.Config) {
			config.BazelContext = android.MockBazelContext{
				OutputBaseDir: outBaseDir,
				LabelToCcInfo: map[string]cquery.CcInfo{
					"//foo/bar:bar": cquery.CcInfo{
						CcStaticLibraryFiles: []string{"foo.so"},
					},
				},
			}
		}),
	).RunTestWithBp(t, bp)
	staticFoo := result.ModuleForTests("foo", "android_arm_armv7-a-neon_static").Module()
	pathPrefix := outBaseDir + "/execroot/__main__/"

	info := result.ModuleProvider(staticFoo, StaticLibraryInfoProvider).(StaticLibraryInfo)
	android.AssertPathRelativeToTopEquals(t,
		"prebuilt library static path did not match expected. If the base path is what does not match, it is likely that Soong built this module instead of Bazel.",
		pathPrefix+"foo.so", info.StaticLibrary)

	outputFiles, err := staticFoo.(android.OutputFileProducer).OutputFiles("")
	if err != nil {
		t.Errorf("Unexpected error getting cc_object outputfiles %s", err)
	}
	expectedOutputFiles := []string{pathPrefix + "foo.so"}
	android.AssertDeepEquals(t, "prebuilt library static output files did not match expected.", expectedOutputFiles, outputFiles.Strings())
}

func TestPrebuiltLibrarySharedWithBazelWithoutToc(t *testing.T) {
	const bp = `
cc_prebuilt_library_shared {
	name: "foo",
	srcs: ["foo.so"],
	bazel_module: { label: "//foo/bar:bar" },
}`
	outBaseDir := "outputbase"
	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)
	config.BazelContext = android.MockBazelContext{
		OutputBaseDir: outBaseDir,
		LabelToCcInfo: map[string]cquery.CcInfo{
			"//foo/bar:bar": cquery.CcInfo{
				CcSharedLibraryFiles: []string{"foo.so"},
			},
		},
	}
	ctx := testCcWithConfig(t, config)
	sharedFoo := ctx.ModuleForTests("foo", "android_arm_armv7-a-neon_shared").Module()
	pathPrefix := outBaseDir + "/execroot/__main__/"

	info := ctx.ModuleProvider(sharedFoo, SharedLibraryInfoProvider).(SharedLibraryInfo)
	android.AssertPathRelativeToTopEquals(t, "prebuilt shared library",
		pathPrefix+"foo.so", info.SharedLibrary)
	android.AssertPathRelativeToTopEquals(t, "prebuilt's 'nullary' ToC",
		pathPrefix+"foo.so", info.TableOfContents.Path())

	outputFiles, err := sharedFoo.(android.OutputFileProducer).OutputFiles("")
	if err != nil {
		t.Errorf("Unexpected error getting cc_object outputfiles %s", err)
	}
	expectedOutputFiles := []string{pathPrefix + "foo.so"}
	android.AssertDeepEquals(t, "output files", expectedOutputFiles, outputFiles.Strings())
}

func TestPrebuiltLibrarySharedWithBazelWithToc(t *testing.T) {
	const bp = `
cc_prebuilt_library_shared {
	name: "foo",
	srcs: ["foo.so"],
	bazel_module: { label: "//foo/bar:bar" },
}`
	outBaseDir := "outputbase"
	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)
	config.BazelContext = android.MockBazelContext{
		OutputBaseDir: outBaseDir,
		LabelToCcInfo: map[string]cquery.CcInfo{
			"//foo/bar:bar": cquery.CcInfo{
				CcSharedLibraryFiles: []string{"foo.so"},
				TocFile:              "toc",
			},
		},
	}
	ctx := testCcWithConfig(t, config)
	sharedFoo := ctx.ModuleForTests("foo", "android_arm_armv7-a-neon_shared").Module()
	pathPrefix := outBaseDir + "/execroot/__main__/"

	info := ctx.ModuleProvider(sharedFoo, SharedLibraryInfoProvider).(SharedLibraryInfo)
	android.AssertPathRelativeToTopEquals(t, "prebuilt shared library's ToC",
		pathPrefix+"toc", info.TableOfContents.Path())
	android.AssertPathRelativeToTopEquals(t, "prebuilt shared library",
		pathPrefix+"foo.so", info.SharedLibrary)

	outputFiles, err := sharedFoo.(android.OutputFileProducer).OutputFiles("")
	if err != nil {
		t.Errorf("Unexpected error getting cc_object outputfiles %s", err)
	}
	expectedOutputFiles := []string{pathPrefix + "foo.so"}
	android.AssertDeepEquals(t, "output files", expectedOutputFiles, outputFiles.Strings())
}

func TestPrebuiltStubNoinstall(t *testing.T) {
	testFunc := func(t *testing.T, bp string) {
		result := android.GroupFixturePreparers(
			prepareForPrebuiltTest,
			android.PrepareForTestWithMakevars,
		).RunTestWithBp(t, bp)

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

		android.AssertStringListDoesNotContain(t,
			"installedlib has install dependency on stub",
			installedlibRule.Deps,
			"out/target/product/test_device/system/lib/stublib.so")
		android.AssertStringListDoesNotContain(t,
			"installedlib has order-only install dependency on stub",
			installedlibRule.OrderOnlyDeps,
			"out/target/product/test_device/system/lib/stublib.so")
	}

	const prebuiltStublibBp = `
		cc_prebuilt_library {
			name: "stublib",
			prefer: true,
			srcs: ["foo.so"],
			stubs: {
				versions: ["1"],
			},
		}
	`

	const installedlibBp = `
		cc_library {
			name: "installedlib",
			shared_libs: ["stublib"],
		}
	`

	t.Run("prebuilt without source", func(t *testing.T) {
		testFunc(t, prebuiltStublibBp+installedlibBp)
	})

	const disabledSourceStublibBp = `
		cc_library {
			name: "stublib",
			enabled: false,
			stubs: {
				versions: ["1"],
			},
		}
	`

	t.Run("prebuilt with disabled source", func(t *testing.T) {
		testFunc(t, disabledSourceStublibBp+prebuiltStublibBp+installedlibBp)
	})
}
