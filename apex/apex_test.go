// Copyright 2018 Google Inc. All rights reserved.
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

package apex

import (
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/dexpreopt"
	prebuilt_etc "android/soong/etc"
	"android/soong/java"
	"android/soong/sh"
)

var buildDir string

// names returns name list from white space separated string
func names(s string) (ns []string) {
	for _, n := range strings.Split(s, " ") {
		if len(n) > 0 {
			ns = append(ns, n)
		}
	}
	return
}

func testApexError(t *testing.T, pattern, bp string, handlers ...testCustomizer) {
	t.Helper()
	ctx, config := testApexContext(t, bp, handlers...)
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	if len(errs) > 0 {
		android.FailIfNoMatchingErrors(t, pattern, errs)
		return
	}
	_, errs = ctx.PrepareBuildActions(config)
	if len(errs) > 0 {
		android.FailIfNoMatchingErrors(t, pattern, errs)
		return
	}

	t.Fatalf("missing expected error %q (0 errors are returned)", pattern)
}

func testApex(t *testing.T, bp string, handlers ...testCustomizer) (*android.TestContext, android.Config) {
	t.Helper()
	ctx, config := testApexContext(t, bp, handlers...)
	_, errs := ctx.ParseBlueprintsFiles(".")
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)
	return ctx, config
}

type testCustomizer func(fs map[string][]byte, config android.Config)

func withFiles(files map[string][]byte) testCustomizer {
	return func(fs map[string][]byte, config android.Config) {
		for k, v := range files {
			fs[k] = v
		}
	}
}

func withTargets(targets map[android.OsType][]android.Target) testCustomizer {
	return func(fs map[string][]byte, config android.Config) {
		for k, v := range targets {
			config.Targets[k] = v
		}
	}
}

func withManifestPackageNameOverrides(specs []string) testCustomizer {
	return func(fs map[string][]byte, config android.Config) {
		config.TestProductVariables.ManifestPackageNameOverrides = specs
	}
}

func withBinder32bit(_ map[string][]byte, config android.Config) {
	config.TestProductVariables.Binder32bit = proptools.BoolPtr(true)
}

func withUnbundledBuild(_ map[string][]byte, config android.Config) {
	config.TestProductVariables.Unbundled_build = proptools.BoolPtr(true)
}

func testApexContext(_ *testing.T, bp string, handlers ...testCustomizer) (*android.TestContext, android.Config) {
	android.ClearApexDependency()

	bp = bp + `
		filegroup {
			name: "myapex-file_contexts",
			srcs: [
				"system/sepolicy/apex/myapex-file_contexts",
			],
		}
	`

	bp = bp + cc.GatherRequiredDepsForTest(android.Android)

	bp = bp + java.GatherRequiredDepsForTest()

	fs := map[string][]byte{
		"a.java":                                              nil,
		"PrebuiltAppFoo.apk":                                  nil,
		"PrebuiltAppFooPriv.apk":                              nil,
		"build/make/target/product/security":                  nil,
		"apex_manifest.json":                                  nil,
		"AndroidManifest.xml":                                 nil,
		"system/sepolicy/apex/myapex-file_contexts":           nil,
		"system/sepolicy/apex/myapex.updatable-file_contexts": nil,
		"system/sepolicy/apex/myapex2-file_contexts":          nil,
		"system/sepolicy/apex/otherapex-file_contexts":        nil,
		"system/sepolicy/apex/commonapex-file_contexts":       nil,
		"system/sepolicy/apex/com.android.vndk-file_contexts": nil,
		"mylib.cpp":                                  nil,
		"mylib_common.cpp":                           nil,
		"mytest.cpp":                                 nil,
		"mytest1.cpp":                                nil,
		"mytest2.cpp":                                nil,
		"mytest3.cpp":                                nil,
		"myprebuilt":                                 nil,
		"my_include":                                 nil,
		"foo/bar/MyClass.java":                       nil,
		"prebuilt.jar":                               nil,
		"prebuilt.so":                                nil,
		"vendor/foo/devkeys/test.x509.pem":           nil,
		"vendor/foo/devkeys/test.pk8":                nil,
		"testkey.x509.pem":                           nil,
		"testkey.pk8":                                nil,
		"testkey.override.x509.pem":                  nil,
		"testkey.override.pk8":                       nil,
		"vendor/foo/devkeys/testkey.avbpubkey":       nil,
		"vendor/foo/devkeys/testkey.pem":             nil,
		"NOTICE":                                     nil,
		"custom_notice":                              nil,
		"custom_notice_for_static_lib":               nil,
		"testkey2.avbpubkey":                         nil,
		"testkey2.pem":                               nil,
		"myapex-arm64.apex":                          nil,
		"myapex-arm.apex":                            nil,
		"myapex.apks":                                nil,
		"frameworks/base/api/current.txt":            nil,
		"framework/aidl/a.aidl":                      nil,
		"build/make/core/proguard.flags":             nil,
		"build/make/core/proguard_basic_keeps.flags": nil,
		"dummy.txt":                                  nil,
		"AppSet.apks":                                nil,
	}

	cc.GatherRequiredFilesForTest(fs)

	for _, handler := range handlers {
		// The fs now needs to be populated before creating the config, call handlers twice
		// for now, once to get any fs changes, and later after the config was created to
		// set product variables or targets.
		tempConfig := android.TestArchConfig(buildDir, nil, bp, fs)
		handler(fs, tempConfig)
	}

	config := android.TestArchConfig(buildDir, nil, bp, fs)
	config.TestProductVariables.DeviceVndkVersion = proptools.StringPtr("current")
	config.TestProductVariables.DefaultAppCertificate = proptools.StringPtr("vendor/foo/devkeys/test")
	config.TestProductVariables.CertificateOverrides = []string{"myapex_keytest:myapex.certificate.override"}
	config.TestProductVariables.Platform_sdk_codename = proptools.StringPtr("Q")
	config.TestProductVariables.Platform_sdk_final = proptools.BoolPtr(false)
	config.TestProductVariables.Platform_vndk_version = proptools.StringPtr("VER")

	for _, handler := range handlers {
		// The fs now needs to be populated before creating the config, call handlers twice
		// for now, earlier to get any fs changes, and now after the config was created to
		// set product variables or targets.
		tempFS := map[string][]byte{}
		handler(tempFS, config)
	}

	ctx := android.NewTestArchContext()

	// from android package
	android.RegisterPackageBuildComponents(ctx)
	ctx.PreArchMutators(android.RegisterBootJarMutators)
	ctx.PreArchMutators(android.RegisterVisibilityRuleChecker)

	ctx.RegisterModuleType("apex", BundleFactory)
	ctx.RegisterModuleType("apex_test", testApexBundleFactory)
	ctx.RegisterModuleType("apex_vndk", vndkApexBundleFactory)
	ctx.RegisterModuleType("apex_key", ApexKeyFactory)
	ctx.RegisterModuleType("apex_defaults", defaultsFactory)
	ctx.RegisterModuleType("prebuilt_apex", PrebuiltFactory)
	ctx.RegisterModuleType("override_apex", overrideApexFactory)
	ctx.RegisterModuleType("apex_set", apexSetFactory)

	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	ctx.PreArchMutators(android.RegisterComponentsMutator)
	ctx.PostDepsMutators(android.RegisterOverridePostDepsMutators)

	cc.RegisterRequiredBuildComponentsForTest(ctx)

	// Register this after the prebuilt mutators have been registered (in
	// cc.RegisterRequiredBuildComponentsForTest) to match what happens at runtime.
	ctx.PreArchMutators(android.RegisterVisibilityRuleGatherer)
	ctx.PostDepsMutators(android.RegisterVisibilityRuleEnforcer)

	ctx.RegisterModuleType("cc_test", cc.TestFactory)
	ctx.RegisterModuleType("vndk_prebuilt_shared", cc.VndkPrebuiltSharedFactory)
	ctx.RegisterModuleType("vndk_libraries_txt", cc.VndkLibrariesTxtFactory)
	ctx.RegisterModuleType("prebuilt_etc", prebuilt_etc.PrebuiltEtcFactory)
	ctx.RegisterModuleType("platform_compat_config", java.PlatformCompatConfigFactory)
	ctx.RegisterModuleType("sh_binary", sh.ShBinaryFactory)
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	java.RegisterJavaBuildComponents(ctx)
	java.RegisterSystemModulesBuildComponents(ctx)
	java.RegisterAppBuildComponents(ctx)
	java.RegisterSdkLibraryBuildComponents(ctx)
	ctx.RegisterSingletonType("apex_keys_text", apexKeysTextFactory)

	ctx.PreDepsMutators(RegisterPreDepsMutators)
	ctx.PostDepsMutators(RegisterPostDepsMutators)

	ctx.Register(config)

	return ctx, config
}

func setUp() {
	var err error
	buildDir, err = ioutil.TempDir("", "soong_apex_test")
	if err != nil {
		panic(err)
	}
}

func tearDown() {
	_ = os.RemoveAll(buildDir)
}

// ensure that 'result' contains 'expected'
func ensureContains(t *testing.T, result string, expected string) {
	t.Helper()
	if !strings.Contains(result, expected) {
		t.Errorf("%q is not found in %q", expected, result)
	}
}

// ensures that 'result' does not contain 'notExpected'
func ensureNotContains(t *testing.T, result string, notExpected string) {
	t.Helper()
	if strings.Contains(result, notExpected) {
		t.Errorf("%q is found in %q", notExpected, result)
	}
}

func ensureMatches(t *testing.T, result string, expectedRex string) {
	ok, err := regexp.MatchString(expectedRex, result)
	if err != nil {
		t.Fatalf("regexp failure trying to match %s against `%s` expression: %s", result, expectedRex, err)
		return
	}
	if !ok {
		t.Errorf("%s does not match regular expession %s", result, expectedRex)
	}
}

func ensureListContains(t *testing.T, result []string, expected string) {
	t.Helper()
	if !android.InList(expected, result) {
		t.Errorf("%q is not found in %v", expected, result)
	}
}

func ensureListNotContains(t *testing.T, result []string, notExpected string) {
	t.Helper()
	if android.InList(notExpected, result) {
		t.Errorf("%q is found in %v", notExpected, result)
	}
}

func ensureListEmpty(t *testing.T, result []string) {
	t.Helper()
	if len(result) > 0 {
		t.Errorf("%q is expected to be empty", result)
	}
}

// Minimal test
func TestBasicApex(t *testing.T) {
	ctx, config := testApex(t, `
		apex_defaults {
			name: "myapex-defaults",
			manifest: ":myapex.manifest",
			androidManifest: ":myapex.androidmanifest",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			multilib: {
				both: {
					binaries: ["foo",],
				}
			},
			java_libs: [
				"myjar",
				"myjar_dex",
			],
		}

		apex {
			name: "myapex",
			defaults: ["myapex-defaults"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		filegroup {
			name: "myapex.manifest",
			srcs: ["apex_manifest.json"],
		}

		filegroup {
			name: "myapex.androidmanifest",
			srcs: ["AndroidManifest.xml"],
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["mylib2"],
			system_shared_libs: [],
			stl: "none",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		cc_binary {
			name: "foo",
			srcs: ["mylib.cpp"],
			compile_multilib: "both",
			multilib: {
					lib32: {
							suffix: "32",
					},
					lib64: {
							suffix: "64",
					},
			},
			symlinks: ["foo_link_"],
			symlink_preferred_arch: true,
			system_shared_libs: [],
			static_executable: true,
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library_shared {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			notice: "custom_notice",
			static_libs: ["libstatic"],
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		cc_prebuilt_library_shared {
			name: "mylib2",
			srcs: ["prebuilt.so"],
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
    }

		cc_library_static {
			name: "libstatic",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			notice: "custom_notice_for_static_lib",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		java_library {
			name: "myjar",
			srcs: ["foo/bar/MyClass.java"],
			stem: "myjar_stem",
			sdk_version: "none",
			system_modules: "none",
			static_libs: ["myotherjar"],
			libs: ["mysharedjar"],
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		dex_import {
			name: "myjar_dex",
			jars: ["prebuilt.jar"],
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		java_library {
			name: "myotherjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		java_library {
			name: "mysharedjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
		}
	`)

	apexRule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("apexRule")

	// Make sure that Android.mk is created
	ab := ctx.ModuleForTests("myapex", "android_common_myapex_image").Module().(*apexBundle)
	data := android.AndroidMkDataForTest(t, config, "", ab)
	var builder strings.Builder
	data.Custom(&builder, ab.BaseModuleName(), "TARGET_", "", data)

	androidMk := builder.String()
	ensureContains(t, androidMk, "LOCAL_MODULE := mylib.myapex\n")
	ensureNotContains(t, androidMk, "LOCAL_MODULE := mylib.com.android.myapex\n")

	optFlags := apexRule.Args["opt_flags"]
	ensureContains(t, optFlags, "--pubkey vendor/foo/devkeys/testkey.avbpubkey")
	// Ensure that the NOTICE output is being packaged as an asset.
	ensureContains(t, optFlags, "--assets_dir "+buildDir+"/.intermediates/myapex/android_common_myapex_image/NOTICE")

	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that main rule creates an output
	ensureContains(t, apexRule.Output.String(), "myapex.apex.unsigned")

	// Ensure that apex variant is created for the direct dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_shared_apex10000")
	ensureListContains(t, ctx.ModuleVariantsForTests("myjar"), "android_common_apex10000")
	ensureListContains(t, ctx.ModuleVariantsForTests("myjar_dex"), "android_common_apex10000")

	// Ensure that apex variant is created for the indirect dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_shared_apex10000")
	ensureListContains(t, ctx.ModuleVariantsForTests("myotherjar"), "android_common_apex10000")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")
	ensureContains(t, copyCmds, "image.apex/lib64/mylib2.so")
	ensureContains(t, copyCmds, "image.apex/javalib/myjar_stem.jar")
	ensureContains(t, copyCmds, "image.apex/javalib/myjar_dex.jar")
	// .. but not for java libs
	ensureNotContains(t, copyCmds, "image.apex/javalib/myotherjar.jar")
	ensureNotContains(t, copyCmds, "image.apex/javalib/msharedjar.jar")

	// Ensure that the platform variant ends with _shared or _common
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_shared")
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_shared")
	ensureListContains(t, ctx.ModuleVariantsForTests("myjar"), "android_common")
	ensureListContains(t, ctx.ModuleVariantsForTests("myotherjar"), "android_common")
	ensureListContains(t, ctx.ModuleVariantsForTests("mysharedjar"), "android_common")

	// Ensure that dynamic dependency to java libs are not included
	ensureListNotContains(t, ctx.ModuleVariantsForTests("mysharedjar"), "android_common_myapex")

	// Ensure that all symlinks are present.
	found_foo_link_64 := false
	found_foo := false
	for _, cmd := range strings.Split(copyCmds, " && ") {
		if strings.HasPrefix(cmd, "ln -sfn foo64") {
			if strings.HasSuffix(cmd, "bin/foo") {
				found_foo = true
			} else if strings.HasSuffix(cmd, "bin/foo_link_64") {
				found_foo_link_64 = true
			}
		}
	}
	good := found_foo && found_foo_link_64
	if !good {
		t.Errorf("Could not find all expected symlinks! foo: %t, foo_link_64: %t. Command was %s", found_foo, found_foo_link_64, copyCmds)
	}

	mergeNoticesRule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("mergeNoticesRule")
	noticeInputs := mergeNoticesRule.Inputs.Strings()
	if len(noticeInputs) != 3 {
		t.Errorf("number of input notice files: expected = 3, actual = %q", len(noticeInputs))
	}
	ensureListContains(t, noticeInputs, "NOTICE")
	ensureListContains(t, noticeInputs, "custom_notice")
	ensureListContains(t, noticeInputs, "custom_notice_for_static_lib")

	fullDepsInfo := strings.Split(ctx.ModuleForTests("myapex", "android_common_myapex_image").Output("depsinfo/fulllist.txt").Args["content"], "\\n")
	ensureListContains(t, fullDepsInfo, "myjar(minSdkVersion:(no version)) <- myapex")
	ensureListContains(t, fullDepsInfo, "mylib(minSdkVersion:(no version)) <- myapex")
	ensureListContains(t, fullDepsInfo, "mylib2(minSdkVersion:(no version)) <- mylib")
	ensureListContains(t, fullDepsInfo, "myotherjar(minSdkVersion:(no version)) <- myjar")
	ensureListContains(t, fullDepsInfo, "mysharedjar(minSdkVersion:(no version)) (external) <- myjar")

	flatDepsInfo := strings.Split(ctx.ModuleForTests("myapex", "android_common_myapex_image").Output("depsinfo/flatlist.txt").Args["content"], "\\n")
	ensureListContains(t, flatDepsInfo, "  myjar(minSdkVersion:(no version))")
	ensureListContains(t, flatDepsInfo, "  mylib(minSdkVersion:(no version))")
	ensureListContains(t, flatDepsInfo, "  mylib2(minSdkVersion:(no version))")
	ensureListContains(t, flatDepsInfo, "  myotherjar(minSdkVersion:(no version))")
	ensureListContains(t, flatDepsInfo, "  mysharedjar(minSdkVersion:(no version)) (external)")
}

func TestDefaults(t *testing.T) {
	ctx, _ := testApex(t, `
		apex_defaults {
			name: "myapex-defaults",
			key: "myapex.key",
			prebuilts: ["myetc"],
			native_shared_libs: ["mylib"],
			java_libs: ["myjar"],
			apps: ["AppFoo"],
		}

		prebuilt_etc {
			name: "myetc",
			src: "myprebuilt",
		}

		apex {
			name: "myapex",
			defaults: ["myapex-defaults"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		java_library {
			name: "myjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			apex_available: [ "myapex" ],
		}

		android_app {
			name: "AppFoo",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			apex_available: [ "myapex" ],
		}
	`)
	ensureExactContents(t, ctx, "myapex", "android_common_myapex_image", []string{
		"etc/myetc",
		"javalib/myjar.jar",
		"lib64/mylib.so",
		"app/AppFoo/AppFoo.apk",
	})
}

func TestApexManifest(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)

	module := ctx.ModuleForTests("myapex", "android_common_myapex_image")
	args := module.Rule("apexRule").Args
	if manifest := args["manifest"]; manifest != module.Output("apex_manifest.pb").Output.String() {
		t.Error("manifest should be apex_manifest.pb, but " + manifest)
	}
}

func TestBasicZipApex(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			payload_type: "zip",
			native_shared_libs: ["mylib"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["mylib2"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}
	`)

	zipApexRule := ctx.ModuleForTests("myapex", "android_common_myapex_zip").Rule("zipApexRule")
	copyCmds := zipApexRule.Args["copy_commands"]

	// Ensure that main rule creates an output
	ensureContains(t, zipApexRule.Output.String(), "myapex.zipapex.unsigned")

	// Ensure that APEX variant is created for the direct dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_shared_apex10000")

	// Ensure that APEX variant is created for the indirect dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_shared_apex10000")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image.zipapex/lib64/mylib.so")
	ensureContains(t, copyCmds, "image.zipapex/lib64/mylib2.so")
}

func TestApexWithStubs(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib", "mylib3"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["mylib2", "mylib3"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			cflags: ["-include mylib.h"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["1", "2", "3"],
			},
		}

		cc_library {
			name: "mylib3",
			srcs: ["mylib.cpp"],
			shared_libs: ["mylib4"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["10", "11", "12"],
			},
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "mylib4",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}
	`)

	apexRule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that direct non-stubs dep is always included
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")

	// Ensure that indirect stubs dep is not included
	ensureNotContains(t, copyCmds, "image.apex/lib64/mylib2.so")

	// Ensure that direct stubs dep is included
	ensureContains(t, copyCmds, "image.apex/lib64/mylib3.so")

	mylibLdFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_shared_apex10000").Rule("ld").Args["libFlags"]

	// Ensure that mylib is linking with the latest version of stubs for mylib2
	ensureContains(t, mylibLdFlags, "mylib2/android_arm64_armv8-a_shared_3/mylib2.so")
	// ... and not linking to the non-stub (impl) variant of mylib2
	ensureNotContains(t, mylibLdFlags, "mylib2/android_arm64_armv8-a_shared/mylib2.so")

	// Ensure that mylib is linking with the non-stub (impl) of mylib3 (because mylib3 is in the same apex)
	ensureContains(t, mylibLdFlags, "mylib3/android_arm64_armv8-a_shared_apex10000/mylib3.so")
	// .. and not linking to the stubs variant of mylib3
	ensureNotContains(t, mylibLdFlags, "mylib3/android_arm64_armv8-a_shared_12/mylib3.so")

	// Ensure that stubs libs are built without -include flags
	mylib2Cflags := ctx.ModuleForTests("mylib2", "android_arm64_armv8-a_static").Rule("cc").Args["cFlags"]
	ensureNotContains(t, mylib2Cflags, "-include ")

	// Ensure that genstub is invoked with --apex
	ensureContains(t, "--apex", ctx.ModuleForTests("mylib2", "android_arm64_armv8-a_static_3").Rule("genStubSrc").Args["flags"])

	ensureExactContents(t, ctx, "myapex", "android_common_myapex_image", []string{
		"lib64/mylib.so",
		"lib64/mylib3.so",
		"lib64/mylib4.so",
	})
}

func TestApexWithExplicitStubsDependency(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex2",
			key: "myapex2.key",
			native_shared_libs: ["mylib"],
		}

		apex_key {
			name: "myapex2.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["libfoo#10"],
			static_libs: ["libbaz"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex2" ],
		}

		cc_library {
			name: "libfoo",
			srcs: ["mylib.cpp"],
			shared_libs: ["libbar"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["10", "20", "30"],
			},
		}

		cc_library {
			name: "libbar",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
		}

		cc_library_static {
			name: "libbaz",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex2" ],
		}

	`)

	apexRule := ctx.ModuleForTests("myapex2", "android_common_myapex2_image").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that direct non-stubs dep is always included
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")

	// Ensure that indirect stubs dep is not included
	ensureNotContains(t, copyCmds, "image.apex/lib64/libfoo.so")

	// Ensure that dependency of stubs is not included
	ensureNotContains(t, copyCmds, "image.apex/lib64/libbar.so")

	mylibLdFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_shared_apex10000").Rule("ld").Args["libFlags"]

	// Ensure that mylib is linking with version 10 of libfoo
	ensureContains(t, mylibLdFlags, "libfoo/android_arm64_armv8-a_shared_10/libfoo.so")
	// ... and not linking to the non-stub (impl) variant of libfoo
	ensureNotContains(t, mylibLdFlags, "libfoo/android_arm64_armv8-a_shared/libfoo.so")

	libFooStubsLdFlags := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared_10").Rule("ld").Args["libFlags"]

	// Ensure that libfoo stubs is not linking to libbar (since it is a stubs)
	ensureNotContains(t, libFooStubsLdFlags, "libbar.so")

	fullDepsInfo := strings.Split(ctx.ModuleForTests("myapex2", "android_common_myapex2_image").Output("depsinfo/fulllist.txt").Args["content"], "\\n")
	ensureListContains(t, fullDepsInfo, "mylib(minSdkVersion:(no version)) <- myapex2")
	ensureListContains(t, fullDepsInfo, "libbaz(minSdkVersion:(no version)) <- mylib")
	ensureListContains(t, fullDepsInfo, "libfoo(minSdkVersion:(no version)) (external) <- mylib")

	flatDepsInfo := strings.Split(ctx.ModuleForTests("myapex2", "android_common_myapex2_image").Output("depsinfo/flatlist.txt").Args["content"], "\\n")
	ensureListContains(t, flatDepsInfo, "  mylib(minSdkVersion:(no version))")
	ensureListContains(t, flatDepsInfo, "  libbaz(minSdkVersion:(no version))")
	ensureListContains(t, flatDepsInfo, "  libfoo(minSdkVersion:(no version)) (external)")
}

func TestApexWithRuntimeLibsDependency(t *testing.T) {
	/*
		myapex
		  |
		  v   (runtime_libs)
		mylib ------+------> libfoo [provides stub]
			    |
			    `------> libbar
	*/
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			runtime_libs: ["libfoo", "libbar"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "libfoo",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["10", "20", "30"],
			},
		}

		cc_library {
			name: "libbar",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

	`)

	apexRule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that direct non-stubs dep is always included
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")

	// Ensure that indirect stubs dep is not included
	ensureNotContains(t, copyCmds, "image.apex/lib64/libfoo.so")

	// Ensure that runtime_libs dep in included
	ensureContains(t, copyCmds, "image.apex/lib64/libbar.so")

	apexManifestRule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("apexManifestRule")
	ensureListEmpty(t, names(apexManifestRule.Args["provideNativeLibs"]))
	ensureListContains(t, names(apexManifestRule.Args["requireNativeLibs"]), "libfoo.so")

}

func TestApexDependsOnLLNDKTransitively(t *testing.T) {
	testcases := []struct {
		name          string
		minSdkVersion string
		apexVariant   string
		shouldLink    string
		shouldNotLink []string
	}{
		{
			name:          "should link to the latest",
			minSdkVersion: "current",
			apexVariant:   "apex10000",
			shouldLink:    "30",
			shouldNotLink: []string{"29"},
		},
		{
			name:          "should link to llndk#29",
			minSdkVersion: "29",
			apexVariant:   "apex29",
			shouldLink:    "29",
			shouldNotLink: []string{"30"},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := testApex(t, `
			apex {
				name: "myapex",
				key: "myapex.key",
				use_vendor: true,
				native_shared_libs: ["mylib"],
				min_sdk_version: "`+tc.minSdkVersion+`",
			}

			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}

			cc_library {
				name: "mylib",
				srcs: ["mylib.cpp"],
				vendor_available: true,
				shared_libs: ["libbar"],
				system_shared_libs: [],
				stl: "none",
				apex_available: [ "myapex" ],
			}

			cc_library {
				name: "libbar",
				srcs: ["mylib.cpp"],
				system_shared_libs: [],
				stl: "none",
				stubs: { versions: ["29","30"] },
			}

			llndk_library {
				name: "libbar",
				symbol_file: "",
			}
			`, func(fs map[string][]byte, config android.Config) {
				setUseVendorAllowListForTest(config, []string{"myapex"})
			}, withUnbundledBuild)

			// Ensure that LLNDK dep is not included
			ensureExactContents(t, ctx, "myapex", "android_common_myapex_image", []string{
				"lib64/mylib.so",
			})

			// Ensure that LLNDK dep is required
			apexManifestRule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("apexManifestRule")
			ensureListEmpty(t, names(apexManifestRule.Args["provideNativeLibs"]))
			ensureListContains(t, names(apexManifestRule.Args["requireNativeLibs"]), "libbar.so")

			mylibLdFlags := ctx.ModuleForTests("mylib", "android_vendor.VER_arm64_armv8-a_shared_"+tc.apexVariant).Rule("ld").Args["libFlags"]
			ensureContains(t, mylibLdFlags, "libbar.llndk/android_vendor.VER_arm64_armv8-a_shared_"+tc.shouldLink+"/libbar.so")
			for _, ver := range tc.shouldNotLink {
				ensureNotContains(t, mylibLdFlags, "libbar.llndk/android_vendor.VER_arm64_armv8-a_shared_"+ver+"/libbar.so")
			}

			mylibCFlags := ctx.ModuleForTests("mylib", "android_vendor.VER_arm64_armv8-a_static_"+tc.apexVariant).Rule("cc").Args["cFlags"]
			ensureContains(t, mylibCFlags, "__LIBBAR_API__="+tc.shouldLink)
		})
	}
}

func TestApexWithSystemLibsStubs(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib", "mylib_shared", "libdl", "libm"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["libdl#27"],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library_shared {
			name: "mylib_shared",
			srcs: ["mylib.cpp"],
			shared_libs: ["libdl#27"],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "libBootstrap",
			srcs: ["mylib.cpp"],
			stl: "none",
			bootstrap: true,
		}
	`)

	apexRule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that mylib, libm, libdl are included.
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")
	ensureContains(t, copyCmds, "image.apex/lib64/bionic/libm.so")
	ensureContains(t, copyCmds, "image.apex/lib64/bionic/libdl.so")

	// Ensure that libc is not included (since it has stubs and not listed in native_shared_libs)
	ensureNotContains(t, copyCmds, "image.apex/lib64/bionic/libc.so")

	mylibLdFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_shared_apex10000").Rule("ld").Args["libFlags"]
	mylibCFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_static_apex10000").Rule("cc").Args["cFlags"]
	mylibSharedCFlags := ctx.ModuleForTests("mylib_shared", "android_arm64_armv8-a_shared_apex10000").Rule("cc").Args["cFlags"]

	// For dependency to libc
	// Ensure that mylib is linking with the latest version of stubs
	ensureContains(t, mylibLdFlags, "libc/android_arm64_armv8-a_shared_29/libc.so")
	// ... and not linking to the non-stub (impl) variant
	ensureNotContains(t, mylibLdFlags, "libc/android_arm64_armv8-a_shared/libc.so")
	// ... Cflags from stub is correctly exported to mylib
	ensureContains(t, mylibCFlags, "__LIBC_API__=29")
	ensureContains(t, mylibSharedCFlags, "__LIBC_API__=29")

	// For dependency to libm
	// Ensure that mylib is linking with the non-stub (impl) variant
	ensureContains(t, mylibLdFlags, "libm/android_arm64_armv8-a_shared_apex10000/libm.so")
	// ... and not linking to the stub variant
	ensureNotContains(t, mylibLdFlags, "libm/android_arm64_armv8-a_shared_29/libm.so")
	// ... and is not compiling with the stub
	ensureNotContains(t, mylibCFlags, "__LIBM_API__=29")
	ensureNotContains(t, mylibSharedCFlags, "__LIBM_API__=29")

	// For dependency to libdl
	// Ensure that mylib is linking with the specified version of stubs
	ensureContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_shared_27/libdl.so")
	// ... and not linking to the other versions of stubs
	ensureNotContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_shared_28/libdl.so")
	ensureNotContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_shared_29/libdl.so")
	// ... and not linking to the non-stub (impl) variant
	ensureNotContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_shared_apex10000/libdl.so")
	// ... Cflags from stub is correctly exported to mylib
	ensureContains(t, mylibCFlags, "__LIBDL_API__=27")
	ensureContains(t, mylibSharedCFlags, "__LIBDL_API__=27")

	// Ensure that libBootstrap is depending on the platform variant of bionic libs
	libFlags := ctx.ModuleForTests("libBootstrap", "android_arm64_armv8-a_shared").Rule("ld").Args["libFlags"]
	ensureContains(t, libFlags, "libc/android_arm64_armv8-a_shared/libc.so")
	ensureContains(t, libFlags, "libm/android_arm64_armv8-a_shared/libm.so")
	ensureContains(t, libFlags, "libdl/android_arm64_armv8-a_shared/libdl.so")
}

func TestApexUseStubsAccordingToMinSdkVersionInUnbundledBuild(t *testing.T) {
	// there are three links between liba --> libz
	// 1) myapex -> libx -> liba -> libz    : this should be #2 link, but fallback to #1
	// 2) otherapex -> liby -> liba -> libz : this should be #3 link
	// 3) (platform) -> liba -> libz        : this should be non-stub link
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libx"],
			min_sdk_version: "2",
		}

		apex {
			name: "otherapex",
			key: "myapex.key",
			native_shared_libs: ["liby"],
			min_sdk_version: "3",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libx",
			shared_libs: ["liba"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "liby",
			shared_libs: ["liba"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "otherapex" ],
		}

		cc_library {
			name: "liba",
			shared_libs: ["libz"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"//apex_available:anyapex",
				"//apex_available:platform",
			],
		}

		cc_library {
			name: "libz",
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["1", "3"],
			},
		}
	`, withUnbundledBuild)

	expectLink := func(from, from_variant, to, to_variant string) {
		ldArgs := ctx.ModuleForTests(from, "android_arm64_armv8-a_"+from_variant).Rule("ld").Args["libFlags"]
		ensureContains(t, ldArgs, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	expectNoLink := func(from, from_variant, to, to_variant string) {
		ldArgs := ctx.ModuleForTests(from, "android_arm64_armv8-a_"+from_variant).Rule("ld").Args["libFlags"]
		ensureNotContains(t, ldArgs, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	// platform liba is linked to non-stub version
	expectLink("liba", "shared", "libz", "shared")
	// liba in myapex is linked to #1
	expectLink("liba", "shared_apex2", "libz", "shared_1")
	expectNoLink("liba", "shared_apex2", "libz", "shared_3")
	expectNoLink("liba", "shared_apex2", "libz", "shared")
	// liba in otherapex is linked to #3
	expectLink("liba", "shared_apex3", "libz", "shared_3")
	expectNoLink("liba", "shared_apex3", "libz", "shared_1")
	expectNoLink("liba", "shared_apex3", "libz", "shared")
}

func TestApexMinSdkVersion_SupportsCodeNames(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libx"],
			min_sdk_version: "R",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libx",
			shared_libs: ["libz"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "libz",
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["29", "R"],
			},
		}
	`, func(fs map[string][]byte, config android.Config) {
		config.TestProductVariables.Platform_version_active_codenames = []string{"R"}
	})

	expectLink := func(from, from_variant, to, to_variant string) {
		ldArgs := ctx.ModuleForTests(from, "android_arm64_armv8-a_"+from_variant).Rule("ld").Args["libFlags"]
		ensureContains(t, ldArgs, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	expectNoLink := func(from, from_variant, to, to_variant string) {
		ldArgs := ctx.ModuleForTests(from, "android_arm64_armv8-a_"+from_variant).Rule("ld").Args["libFlags"]
		ensureNotContains(t, ldArgs, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	// 9000 is quite a magic number.
	// Finalized SDK codenames are mapped as P(28), Q(29), ...
	// And, codenames which are not finalized yet(active_codenames + future_codenames) are numbered from 9000, 9001, ...
	// to distinguish them from finalized and future_api(10000)
	// In this test, "R" is assumed not finalized yet( listed in Platform_version_active_codenames) and translated into 9000
	// (refer android/api_levels.go)
	expectLink("libx", "shared_apex9000", "libz", "shared_9000")
	expectNoLink("libx", "shared_apex9000", "libz", "shared_29")
	expectNoLink("libx", "shared_apex9000", "libz", "shared")
}

func TestApexMinSdkVersionDefaultsToLatest(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libx"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libx",
			shared_libs: ["libz"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "libz",
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["1", "2"],
			},
		}
	`)

	expectLink := func(from, from_variant, to, to_variant string) {
		ldArgs := ctx.ModuleForTests(from, "android_arm64_armv8-a_"+from_variant).Rule("ld").Args["libFlags"]
		ensureContains(t, ldArgs, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	expectNoLink := func(from, from_variant, to, to_variant string) {
		ldArgs := ctx.ModuleForTests(from, "android_arm64_armv8-a_"+from_variant).Rule("ld").Args["libFlags"]
		ensureNotContains(t, ldArgs, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	expectLink("libx", "shared_apex10000", "libz", "shared_2")
	expectNoLink("libx", "shared_apex10000", "libz", "shared_1")
	expectNoLink("libx", "shared_apex10000", "libz", "shared")
}

func TestPlatformUsesLatestStubsFromApexes(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libx"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libx",
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
			stubs: {
				versions: ["1", "2"],
			},
		}

		cc_library {
			name: "libz",
			shared_libs: ["libx"],
			system_shared_libs: [],
			stl: "none",
		}
	`)

	expectLink := func(from, from_variant, to, to_variant string) {
		ldArgs := ctx.ModuleForTests(from, "android_arm64_armv8-a_"+from_variant).Rule("ld").Args["libFlags"]
		ensureContains(t, ldArgs, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	expectNoLink := func(from, from_variant, to, to_variant string) {
		ldArgs := ctx.ModuleForTests(from, "android_arm64_armv8-a_"+from_variant).Rule("ld").Args["libFlags"]
		ensureNotContains(t, ldArgs, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	expectLink("libz", "shared", "libx", "shared_2")
	expectNoLink("libz", "shared", "libz", "shared_1")
	expectNoLink("libz", "shared", "libz", "shared")
}

func TestQApexesUseLatestStubsInBundledBuildsAndHWASAN(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libx"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libx",
			shared_libs: ["libbar"],
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "libbar",
			stubs: {
				versions: ["29", "30"],
			},
		}
	`, func(fs map[string][]byte, config android.Config) {
		config.TestProductVariables.SanitizeDevice = []string{"hwaddress"}
	})
	expectLink := func(from, from_variant, to, to_variant string) {
		ld := ctx.ModuleForTests(from, "android_arm64_armv8-a_"+from_variant).Rule("ld")
		libFlags := ld.Args["libFlags"]
		ensureContains(t, libFlags, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	expectLink("libx", "shared_hwasan_apex29", "libbar", "shared_30")
}

func TestQTargetApexUsesStaticUnwinder(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libx"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libx",
			apex_available: [ "myapex" ],
		}
	`)

	// ensure apex variant of c++ is linked with static unwinder
	cm := ctx.ModuleForTests("libc++", "android_arm64_armv8-a_shared_apex29").Module().(*cc.Module)
	ensureListContains(t, cm.Properties.AndroidMkStaticLibs, "libgcc_stripped")
	// note that platform variant is not.
	cm = ctx.ModuleForTests("libc++", "android_arm64_armv8-a_shared").Module().(*cc.Module)
	ensureListNotContains(t, cm.Properties.AndroidMkStaticLibs, "libgcc_stripped")
}

func TestInvalidMinSdkVersion(t *testing.T) {
	testApexError(t, `"libz" .*: not found a version\(<=29\)`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libx"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libx",
			shared_libs: ["libz"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "libz",
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["30"],
			},
		}
	`)

	testApexError(t, `"myapex" .*: min_sdk_version: SDK version should be .*`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			min_sdk_version: "abc",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)
}

func TestJavaStableSdkVersion(t *testing.T) {
	testCases := []struct {
		name          string
		expectedError string
		bp            string
	}{
		{
			name: "Non-updatable apex with non-stable dep",
			bp: `
				apex {
					name: "myapex",
					java_libs: ["myjar"],
					key: "myapex.key",
				}
				apex_key {
					name: "myapex.key",
					public_key: "testkey.avbpubkey",
					private_key: "testkey.pem",
				}
				java_library {
					name: "myjar",
					srcs: ["foo/bar/MyClass.java"],
					sdk_version: "core_platform",
					apex_available: ["myapex"],
				}
			`,
		},
		{
			name: "Updatable apex with stable dep",
			bp: `
				apex {
					name: "myapex",
					java_libs: ["myjar"],
					key: "myapex.key",
					updatable: true,
					min_sdk_version: "29",
				}
				apex_key {
					name: "myapex.key",
					public_key: "testkey.avbpubkey",
					private_key: "testkey.pem",
				}
				java_library {
					name: "myjar",
					srcs: ["foo/bar/MyClass.java"],
					sdk_version: "current",
					apex_available: ["myapex"],
				}
			`,
		},
		{
			name:          "Updatable apex with non-stable dep",
			expectedError: "cannot depend on \"myjar\"",
			bp: `
				apex {
					name: "myapex",
					java_libs: ["myjar"],
					key: "myapex.key",
					updatable: true,
				}
				apex_key {
					name: "myapex.key",
					public_key: "testkey.avbpubkey",
					private_key: "testkey.pem",
				}
				java_library {
					name: "myjar",
					srcs: ["foo/bar/MyClass.java"],
					sdk_version: "core_platform",
					apex_available: ["myapex"],
				}
			`,
		},
		{
			name:          "Updatable apex with non-stable transitive dep",
			expectedError: "compiles against Android API, but dependency \"transitive-jar\" is compiling against non-public Android API.",
			bp: `
				apex {
					name: "myapex",
					java_libs: ["myjar"],
					key: "myapex.key",
					updatable: true,
				}
				apex_key {
					name: "myapex.key",
					public_key: "testkey.avbpubkey",
					private_key: "testkey.pem",
				}
				java_library {
					name: "myjar",
					srcs: ["foo/bar/MyClass.java"],
					sdk_version: "current",
					apex_available: ["myapex"],
					static_libs: ["transitive-jar"],
				}
				java_library {
					name: "transitive-jar",
					srcs: ["foo/bar/MyClass.java"],
					sdk_version: "core_platform",
					apex_available: ["myapex"],
				}
			`,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			if test.expectedError == "" {
				testApex(t, test.bp)
			} else {
				testApexError(t, test.expectedError, test.bp)
			}
		})
	}
}

func TestFilesInSubDir(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			binaries: ["mybin"],
			prebuilts: ["myetc"],
			compile_multilib: "both",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		prebuilt_etc {
			name: "myetc",
			src: "myprebuilt",
			sub_dir: "foo/bar",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			relative_install_path: "foo/bar",
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_binary {
			name: "mybin",
			srcs: ["mylib.cpp"],
			relative_install_path: "foo/bar",
			system_shared_libs: [],
			static_executable: true,
			stl: "none",
			apex_available: [ "myapex" ],
		}
	`)

	generateFsRule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("generateFsConfig")
	dirs := strings.Split(generateFsRule.Args["exec_paths"], " ")

	// Ensure that the subdirectories are all listed
	ensureListContains(t, dirs, "etc")
	ensureListContains(t, dirs, "etc/foo")
	ensureListContains(t, dirs, "etc/foo/bar")
	ensureListContains(t, dirs, "lib64")
	ensureListContains(t, dirs, "lib64/foo")
	ensureListContains(t, dirs, "lib64/foo/bar")
	ensureListContains(t, dirs, "lib")
	ensureListContains(t, dirs, "lib/foo")
	ensureListContains(t, dirs, "lib/foo/bar")

	ensureListContains(t, dirs, "bin")
	ensureListContains(t, dirs, "bin/foo")
	ensureListContains(t, dirs, "bin/foo/bar")
}

func TestUseVendor(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			use_vendor: true,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["mylib2"],
			system_shared_libs: [],
			vendor_available: true,
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			vendor_available: true,
			stl: "none",
			apex_available: [ "myapex" ],
		}
	`, func(fs map[string][]byte, config android.Config) {
		setUseVendorAllowListForTest(config, []string{"myapex"})
	})

	inputsList := []string{}
	for _, i := range ctx.ModuleForTests("myapex", "android_common_myapex_image").Module().BuildParamsForTests() {
		for _, implicit := range i.Implicits {
			inputsList = append(inputsList, implicit.String())
		}
	}
	inputsString := strings.Join(inputsList, " ")

	// ensure that the apex includes vendor variants of the direct and indirect deps
	ensureContains(t, inputsString, "android_vendor.VER_arm64_armv8-a_shared_apex10000/mylib.so")
	ensureContains(t, inputsString, "android_vendor.VER_arm64_armv8-a_shared_apex10000/mylib2.so")

	// ensure that the apex does not include core variants
	ensureNotContains(t, inputsString, "android_arm64_armv8-a_shared_apex10000/mylib.so")
	ensureNotContains(t, inputsString, "android_arm64_armv8-a_shared_apex10000/mylib2.so")
}

func TestUseVendorRestriction(t *testing.T) {
	testApexError(t, `module "myapex" .*: use_vendor: not allowed`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			use_vendor: true,
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`, func(fs map[string][]byte, config android.Config) {
		setUseVendorAllowListForTest(config, []string{""})
	})
	// no error with allow list
	testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			use_vendor: true,
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`, func(fs map[string][]byte, config android.Config) {
		setUseVendorAllowListForTest(config, []string{"myapex"})
	})
}

func TestUseVendorFailsIfNotVendorAvailable(t *testing.T) {
	testApexError(t, `dependency "mylib" of "myapex" missing variant:\n.*image:vendor`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			use_vendor: true,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
		}
	`)
}

func TestStaticLinking(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["1", "2", "3"],
			},
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		cc_binary {
			name: "not_in_apex",
			srcs: ["mylib.cpp"],
			static_libs: ["mylib"],
			static_executable: true,
			system_shared_libs: [],
			stl: "none",
		}
	`)

	ldFlags := ctx.ModuleForTests("not_in_apex", "android_arm64_armv8-a").Rule("ld").Args["libFlags"]

	// Ensure that not_in_apex is linking with the static variant of mylib
	ensureContains(t, ldFlags, "mylib/android_arm64_armv8-a_static/mylib.a")
}

func TestKeys(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex_keytest",
			key: "myapex.key",
			certificate: ":myapex.certificate",
			native_shared_libs: ["mylib"],
			file_contexts: ":myapex-file_contexts",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex_keytest" ],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_app_certificate {
			name: "myapex.certificate",
			certificate: "testkey",
		}

		android_app_certificate {
			name: "myapex.certificate.override",
			certificate: "testkey.override",
		}

	`)

	// check the APEX keys
	keys := ctx.ModuleForTests("myapex.key", "android_common").Module().(*apexKey)

	if keys.public_key_file.String() != "vendor/foo/devkeys/testkey.avbpubkey" {
		t.Errorf("public key %q is not %q", keys.public_key_file.String(),
			"vendor/foo/devkeys/testkey.avbpubkey")
	}
	if keys.private_key_file.String() != "vendor/foo/devkeys/testkey.pem" {
		t.Errorf("private key %q is not %q", keys.private_key_file.String(),
			"vendor/foo/devkeys/testkey.pem")
	}

	// check the APK certs. It should be overridden to myapex.certificate.override
	certs := ctx.ModuleForTests("myapex_keytest", "android_common_myapex_keytest_image").Rule("signapk").Args["certificates"]
	if certs != "testkey.override.x509.pem testkey.override.pk8" {
		t.Errorf("cert and private key %q are not %q", certs,
			"testkey.override.509.pem testkey.override.pk8")
	}
}

func TestCertificate(t *testing.T) {
	t.Run("if unspecified, it defaults to DefaultAppCertificate", func(t *testing.T) {
		ctx, _ := testApex(t, `
			apex {
				name: "myapex",
				key: "myapex.key",
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}`)
		rule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("signapk")
		expected := "vendor/foo/devkeys/test.x509.pem vendor/foo/devkeys/test.pk8"
		if actual := rule.Args["certificates"]; actual != expected {
			t.Errorf("certificates should be %q, not %q", expected, actual)
		}
	})
	t.Run("override when unspecified", func(t *testing.T) {
		ctx, _ := testApex(t, `
			apex {
				name: "myapex_keytest",
				key: "myapex.key",
				file_contexts: ":myapex-file_contexts",
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}
			android_app_certificate {
				name: "myapex.certificate.override",
				certificate: "testkey.override",
			}`)
		rule := ctx.ModuleForTests("myapex_keytest", "android_common_myapex_keytest_image").Rule("signapk")
		expected := "testkey.override.x509.pem testkey.override.pk8"
		if actual := rule.Args["certificates"]; actual != expected {
			t.Errorf("certificates should be %q, not %q", expected, actual)
		}
	})
	t.Run("if specified as :module, it respects the prop", func(t *testing.T) {
		ctx, _ := testApex(t, `
			apex {
				name: "myapex",
				key: "myapex.key",
				certificate: ":myapex.certificate",
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}
			android_app_certificate {
				name: "myapex.certificate",
				certificate: "testkey",
			}`)
		rule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("signapk")
		expected := "testkey.x509.pem testkey.pk8"
		if actual := rule.Args["certificates"]; actual != expected {
			t.Errorf("certificates should be %q, not %q", expected, actual)
		}
	})
	t.Run("override when specifiec as <:module>", func(t *testing.T) {
		ctx, _ := testApex(t, `
			apex {
				name: "myapex_keytest",
				key: "myapex.key",
				file_contexts: ":myapex-file_contexts",
				certificate: ":myapex.certificate",
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}
			android_app_certificate {
				name: "myapex.certificate.override",
				certificate: "testkey.override",
			}`)
		rule := ctx.ModuleForTests("myapex_keytest", "android_common_myapex_keytest_image").Rule("signapk")
		expected := "testkey.override.x509.pem testkey.override.pk8"
		if actual := rule.Args["certificates"]; actual != expected {
			t.Errorf("certificates should be %q, not %q", expected, actual)
		}
	})
	t.Run("if specified as name, finds it from DefaultDevKeyDir", func(t *testing.T) {
		ctx, _ := testApex(t, `
			apex {
				name: "myapex",
				key: "myapex.key",
				certificate: "testkey",
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}`)
		rule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("signapk")
		expected := "vendor/foo/devkeys/testkey.x509.pem vendor/foo/devkeys/testkey.pk8"
		if actual := rule.Args["certificates"]; actual != expected {
			t.Errorf("certificates should be %q, not %q", expected, actual)
		}
	})
	t.Run("override when specified as <name>", func(t *testing.T) {
		ctx, _ := testApex(t, `
			apex {
				name: "myapex_keytest",
				key: "myapex.key",
				file_contexts: ":myapex-file_contexts",
				certificate: "testkey",
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}
			android_app_certificate {
				name: "myapex.certificate.override",
				certificate: "testkey.override",
			}`)
		rule := ctx.ModuleForTests("myapex_keytest", "android_common_myapex_keytest_image").Rule("signapk")
		expected := "testkey.override.x509.pem testkey.override.pk8"
		if actual := rule.Args["certificates"]; actual != expected {
			t.Errorf("certificates should be %q, not %q", expected, actual)
		}
	})
}

func TestMacro(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib", "mylib2"],
		}

		apex {
			name: "otherapex",
			key: "myapex.key",
			native_shared_libs: ["mylib", "mylib2"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex",
				"otherapex",
			],
			recovery_available: true,
		}
		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex",
				"otherapex",
			],
			static_libs: ["mylib3"],
			recovery_available: true,
		}
		cc_library {
			name: "mylib3",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex",
				"otherapex",
			],
			use_apex_name_macro: true,
			recovery_available: true,
		}
	`)

	// non-APEX variant does not have __ANDROID_APEX__ defined
	mylibCFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_static").Rule("cc").Args["cFlags"]
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX__")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_SDK_VERSION__")

	// APEX variant has __ANDROID_APEX__ and __ANDROID_APEX_SDK__ defined
	mylibCFlags = ctx.ModuleForTests("mylib", "android_arm64_armv8-a_static_apex10000").Rule("cc").Args["cFlags"]
	ensureContains(t, mylibCFlags, "-D__ANDROID_APEX__")
	ensureContains(t, mylibCFlags, "-D__ANDROID_SDK_VERSION__=10000")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX_MYAPEX__")

	// APEX variant has __ANDROID_APEX__ and __ANDROID_APEX_SDK__ defined
	mylibCFlags = ctx.ModuleForTests("mylib", "android_arm64_armv8-a_static_apex29").Rule("cc").Args["cFlags"]
	ensureContains(t, mylibCFlags, "-D__ANDROID_APEX__")
	ensureContains(t, mylibCFlags, "-D__ANDROID_SDK_VERSION__=29")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX_OTHERAPEX__")

	// When a cc_library sets use_apex_name_macro: true each apex gets a unique variant and
	// each variant defines additional macros to distinguish which apex variant it is built for

	// non-APEX variant does not have __ANDROID_APEX__ defined
	mylibCFlags = ctx.ModuleForTests("mylib3", "android_arm64_armv8-a_static").Rule("cc").Args["cFlags"]
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX__")

	// APEX variant has __ANDROID_APEX__ defined
	mylibCFlags = ctx.ModuleForTests("mylib3", "android_arm64_armv8-a_static_myapex").Rule("cc").Args["cFlags"]
	ensureContains(t, mylibCFlags, "-D__ANDROID_APEX__")
	ensureContains(t, mylibCFlags, "-D__ANDROID_APEX_MYAPEX__")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX_OTHERAPEX__")

	// APEX variant has __ANDROID_APEX__ defined
	mylibCFlags = ctx.ModuleForTests("mylib3", "android_arm64_armv8-a_static_otherapex").Rule("cc").Args["cFlags"]
	ensureContains(t, mylibCFlags, "-D__ANDROID_APEX__")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX_MYAPEX__")
	ensureContains(t, mylibCFlags, "-D__ANDROID_APEX_OTHERAPEX__")

	// recovery variant does not set __ANDROID_SDK_VERSION__
	mylibCFlags = ctx.ModuleForTests("mylib3", "android_recovery_arm64_armv8-a_static").Rule("cc").Args["cFlags"]
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX__")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_SDK_VERSION__")

	// When a dependency of a cc_library sets use_apex_name_macro: true each apex gets a unique
	// variant.

	// non-APEX variant does not have __ANDROID_APEX__ defined
	mylibCFlags = ctx.ModuleForTests("mylib2", "android_arm64_armv8-a_static").Rule("cc").Args["cFlags"]
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX__")

	// APEX variant has __ANDROID_APEX__ defined
	mylibCFlags = ctx.ModuleForTests("mylib2", "android_arm64_armv8-a_static_myapex").Rule("cc").Args["cFlags"]
	ensureContains(t, mylibCFlags, "-D__ANDROID_APEX__")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX_MYAPEX__")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX_OTHERAPEX__")

	// APEX variant has __ANDROID_APEX__ defined
	mylibCFlags = ctx.ModuleForTests("mylib2", "android_arm64_armv8-a_static_otherapex").Rule("cc").Args["cFlags"]
	ensureContains(t, mylibCFlags, "-D__ANDROID_APEX__")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX_MYAPEX__")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX_OTHERAPEX__")

	// recovery variant does not set __ANDROID_SDK_VERSION__
	mylibCFlags = ctx.ModuleForTests("mylib2", "android_recovery_arm64_armv8-a_static").Rule("cc").Args["cFlags"]
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX__")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_SDK_VERSION__")
}

func TestHeaderLibsDependency(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library_headers {
			name: "mylib_headers",
			export_include_dirs: ["my_include"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			header_libs: ["mylib_headers"],
			export_header_lib_headers: ["mylib_headers"],
			stubs: {
				versions: ["1", "2", "3"],
			},
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "otherlib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			shared_libs: ["mylib"],
		}
	`)

	cFlags := ctx.ModuleForTests("otherlib", "android_arm64_armv8-a_static").Rule("cc").Args["cFlags"]

	// Ensure that the include path of the header lib is exported to 'otherlib'
	ensureContains(t, cFlags, "-Imy_include")
}

type fileInApex struct {
	path   string // path in apex
	src    string // src path
	isLink bool
}

func getFiles(t *testing.T, ctx *android.TestContext, moduleName, variant string) []fileInApex {
	t.Helper()
	apexRule := ctx.ModuleForTests(moduleName, variant).Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]
	imageApexDir := "/image.apex/"
	var ret []fileInApex
	for _, cmd := range strings.Split(copyCmds, "&&") {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		terms := strings.Split(cmd, " ")
		var dst, src string
		var isLink bool
		switch terms[0] {
		case "mkdir":
		case "cp":
			if len(terms) != 3 && len(terms) != 4 {
				t.Fatal("copyCmds contains invalid cp command", cmd)
			}
			dst = terms[len(terms)-1]
			src = terms[len(terms)-2]
			isLink = false
		case "ln":
			if len(terms) != 3 && len(terms) != 4 {
				// ln LINK TARGET or ln -s LINK TARGET
				t.Fatal("copyCmds contains invalid ln command", cmd)
			}
			dst = terms[len(terms)-1]
			src = terms[len(terms)-2]
			isLink = true
		default:
			t.Fatalf("copyCmds should contain mkdir/cp commands only: %q", cmd)
		}
		if dst != "" {
			index := strings.Index(dst, imageApexDir)
			if index == -1 {
				t.Fatal("copyCmds should copy a file to image.apex/", cmd)
			}
			dstFile := dst[index+len(imageApexDir):]
			ret = append(ret, fileInApex{path: dstFile, src: src, isLink: isLink})
		}
	}
	return ret
}

func ensureExactContents(t *testing.T, ctx *android.TestContext, moduleName, variant string, files []string) {
	t.Helper()
	var failed bool
	var surplus []string
	filesMatched := make(map[string]bool)
	for _, file := range getFiles(t, ctx, moduleName, variant) {
		mactchFound := false
		for _, expected := range files {
			if matched, _ := path.Match(expected, file.path); matched {
				filesMatched[expected] = true
				mactchFound = true
				break
			}
		}
		if !mactchFound {
			surplus = append(surplus, file.path)
		}
	}

	if len(surplus) > 0 {
		sort.Strings(surplus)
		t.Log("surplus files", surplus)
		failed = true
	}

	if len(files) > len(filesMatched) {
		var missing []string
		for _, expected := range files {
			if !filesMatched[expected] {
				missing = append(missing, expected)
			}
		}
		sort.Strings(missing)
		t.Log("missing files", missing)
		failed = true
	}
	if failed {
		t.Fail()
	}
}

func TestVndkApexCurrent(t *testing.T) {
	ctx, _ := testApex(t, `
		apex_vndk {
			name: "myapex",
			key: "myapex.key",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libvndk",
			srcs: ["mylib.cpp"],
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "libvndksp",
			srcs: ["mylib.cpp"],
			vendor_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}
	`+vndkLibrariesTxtFiles("current"))

	ensureExactContents(t, ctx, "myapex", "android_common_image", []string{
		"lib/libvndk.so",
		"lib/libvndksp.so",
		"lib/libc++.so",
		"lib64/libvndk.so",
		"lib64/libvndksp.so",
		"lib64/libc++.so",
		"etc/llndk.libraries.VER.txt",
		"etc/vndkcore.libraries.VER.txt",
		"etc/vndksp.libraries.VER.txt",
		"etc/vndkprivate.libraries.VER.txt",
	})
}

func TestVndkApexWithPrebuilt(t *testing.T) {
	ctx, _ := testApex(t, `
		apex_vndk {
			name: "myapex",
			key: "myapex.key",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_prebuilt_library_shared {
			name: "libvndk",
			srcs: ["libvndk.so"],
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_prebuilt_library_shared {
			name: "libvndk.arm",
			srcs: ["libvndk.arm.so"],
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			enabled: false,
			arch: {
				arm: {
					enabled: true,
				},
			},
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}
		`+vndkLibrariesTxtFiles("current"),
		withFiles(map[string][]byte{
			"libvndk.so":     nil,
			"libvndk.arm.so": nil,
		}))

	ensureExactContents(t, ctx, "myapex", "android_common_image", []string{
		"lib/libvndk.so",
		"lib/libvndk.arm.so",
		"lib64/libvndk.so",
		"lib/libc++.so",
		"lib64/libc++.so",
		"etc/*",
	})
}

func vndkLibrariesTxtFiles(vers ...string) (result string) {
	for _, v := range vers {
		if v == "current" {
			for _, txt := range []string{"llndk", "vndkcore", "vndksp", "vndkprivate"} {
				result += `
					vndk_libraries_txt {
						name: "` + txt + `.libraries.txt",
					}
				`
			}
		} else {
			for _, txt := range []string{"llndk", "vndkcore", "vndksp", "vndkprivate"} {
				result += `
					prebuilt_etc {
						name: "` + txt + `.libraries.` + v + `.txt",
						src: "dummy.txt",
					}
				`
			}
		}
	}
	return
}

func TestVndkApexVersion(t *testing.T) {
	ctx, _ := testApex(t, `
		apex_vndk {
			name: "myapex_v27",
			key: "myapex.key",
			file_contexts: ":myapex-file_contexts",
			vndk_version: "27",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		vndk_prebuilt_shared {
			name: "libvndk27",
			version: "27",
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			target_arch: "arm64",
			arch: {
				arm: {
					srcs: ["libvndk27_arm.so"],
				},
				arm64: {
					srcs: ["libvndk27_arm64.so"],
				},
			},
			apex_available: [ "myapex_v27" ],
		}

		vndk_prebuilt_shared {
			name: "libvndk27",
			version: "27",
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			target_arch: "x86_64",
			arch: {
				x86: {
					srcs: ["libvndk27_x86.so"],
				},
				x86_64: {
					srcs: ["libvndk27_x86_64.so"],
				},
			},
		}
		`+vndkLibrariesTxtFiles("27"),
		withFiles(map[string][]byte{
			"libvndk27_arm.so":    nil,
			"libvndk27_arm64.so":  nil,
			"libvndk27_x86.so":    nil,
			"libvndk27_x86_64.so": nil,
		}))

	ensureExactContents(t, ctx, "myapex_v27", "android_common_image", []string{
		"lib/libvndk27_arm.so",
		"lib64/libvndk27_arm64.so",
		"etc/*",
	})
}

func TestVndkApexErrorWithDuplicateVersion(t *testing.T) {
	testApexError(t, `module "myapex_v27.*" .*: vndk_version: 27 is already defined in "myapex_v27.*"`, `
		apex_vndk {
			name: "myapex_v27",
			key: "myapex.key",
			file_contexts: ":myapex-file_contexts",
			vndk_version: "27",
		}
		apex_vndk {
			name: "myapex_v27_other",
			key: "myapex.key",
			file_contexts: ":myapex-file_contexts",
			vndk_version: "27",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libvndk",
			srcs: ["mylib.cpp"],
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			system_shared_libs: [],
			stl: "none",
		}

		vndk_prebuilt_shared {
			name: "libvndk",
			version: "27",
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			srcs: ["libvndk.so"],
		}
	`, withFiles(map[string][]byte{
		"libvndk.so": nil,
	}))
}

func TestVndkApexNameRule(t *testing.T) {
	ctx, _ := testApex(t, `
		apex_vndk {
			name: "myapex",
			key: "myapex.key",
			file_contexts: ":myapex-file_contexts",
		}
		apex_vndk {
			name: "myapex_v28",
			key: "myapex.key",
			file_contexts: ":myapex-file_contexts",
			vndk_version: "28",
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}`+vndkLibrariesTxtFiles("28", "current"))

	assertApexName := func(expected, moduleName string) {
		bundle := ctx.ModuleForTests(moduleName, "android_common_image").Module().(*apexBundle)
		actual := proptools.String(bundle.properties.Apex_name)
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("Got '%v', expected '%v'", actual, expected)
		}
	}

	assertApexName("com.android.vndk.vVER", "myapex")
	assertApexName("com.android.vndk.v28", "myapex_v28")
}

func TestVndkApexSkipsNativeBridgeSupportedModules(t *testing.T) {
	ctx, _ := testApex(t, `
		apex_vndk {
			name: "myapex",
			key: "myapex.key",
			file_contexts: ":myapex-file_contexts",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libvndk",
			srcs: ["mylib.cpp"],
			vendor_available: true,
			native_bridge_supported: true,
			host_supported: true,
			vndk: {
				enabled: true,
			},
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}
		`+vndkLibrariesTxtFiles("current"),
		withTargets(map[android.OsType][]android.Target{
			android.Android: []android.Target{
				{Os: android.Android, Arch: android.Arch{ArchType: android.Arm64, ArchVariant: "armv8-a", Abi: []string{"arm64-v8a"}}, NativeBridge: android.NativeBridgeDisabled, NativeBridgeHostArchName: "", NativeBridgeRelativePath: ""},
				{Os: android.Android, Arch: android.Arch{ArchType: android.Arm, ArchVariant: "armv7-a-neon", Abi: []string{"armeabi-v7a"}}, NativeBridge: android.NativeBridgeDisabled, NativeBridgeHostArchName: "", NativeBridgeRelativePath: ""},
				{Os: android.Android, Arch: android.Arch{ArchType: android.X86_64, ArchVariant: "silvermont", Abi: []string{"arm64-v8a"}}, NativeBridge: android.NativeBridgeEnabled, NativeBridgeHostArchName: "arm64", NativeBridgeRelativePath: "x86_64"},
				{Os: android.Android, Arch: android.Arch{ArchType: android.X86, ArchVariant: "silvermont", Abi: []string{"armeabi-v7a"}}, NativeBridge: android.NativeBridgeEnabled, NativeBridgeHostArchName: "arm", NativeBridgeRelativePath: "x86"},
			},
		}))

	ensureExactContents(t, ctx, "myapex", "android_common_image", []string{
		"lib/libvndk.so",
		"lib64/libvndk.so",
		"lib/libc++.so",
		"lib64/libc++.so",
		"etc/*",
	})
}

func TestVndkApexDoesntSupportNativeBridgeSupported(t *testing.T) {
	testApexError(t, `module "myapex" .*: native_bridge_supported: .* doesn't support native bridge binary`, `
		apex_vndk {
			name: "myapex",
			key: "myapex.key",
			file_contexts: ":myapex-file_contexts",
			native_bridge_supported: true,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libvndk",
			srcs: ["mylib.cpp"],
			vendor_available: true,
			native_bridge_supported: true,
			host_supported: true,
			vndk: {
				enabled: true,
			},
			system_shared_libs: [],
			stl: "none",
		}
	`)
}

func TestVndkApexWithBinder32(t *testing.T) {
	ctx, _ := testApex(t, `
		apex_vndk {
			name: "myapex_v27",
			key: "myapex.key",
			file_contexts: ":myapex-file_contexts",
			vndk_version: "27",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		vndk_prebuilt_shared {
			name: "libvndk27",
			version: "27",
			target_arch: "arm",
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			arch: {
				arm: {
					srcs: ["libvndk27.so"],
				}
			},
		}

		vndk_prebuilt_shared {
			name: "libvndk27",
			version: "27",
			target_arch: "arm",
			binder32bit: true,
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			arch: {
				arm: {
					srcs: ["libvndk27binder32.so"],
				}
			},
			apex_available: [ "myapex_v27" ],
		}
		`+vndkLibrariesTxtFiles("27"),
		withFiles(map[string][]byte{
			"libvndk27.so":         nil,
			"libvndk27binder32.so": nil,
		}),
		withBinder32bit,
		withTargets(map[android.OsType][]android.Target{
			android.Android: []android.Target{
				{Os: android.Android, Arch: android.Arch{ArchType: android.Arm, ArchVariant: "armv7-a-neon", Abi: []string{"armeabi-v7a"}}, NativeBridge: android.NativeBridgeDisabled, NativeBridgeHostArchName: "", NativeBridgeRelativePath: ""},
			},
		}),
	)

	ensureExactContents(t, ctx, "myapex_v27", "android_common_image", []string{
		"lib/libvndk27binder32.so",
		"etc/*",
	})
}

func TestDependenciesInApexManifest(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex_nodep",
			key: "myapex.key",
			native_shared_libs: ["lib_nodep"],
			compile_multilib: "both",
			file_contexts: ":myapex-file_contexts",
		}

		apex {
			name: "myapex_dep",
			key: "myapex.key",
			native_shared_libs: ["lib_dep"],
			compile_multilib: "both",
			file_contexts: ":myapex-file_contexts",
		}

		apex {
			name: "myapex_provider",
			key: "myapex.key",
			native_shared_libs: ["libfoo"],
			compile_multilib: "both",
			file_contexts: ":myapex-file_contexts",
		}

		apex {
			name: "myapex_selfcontained",
			key: "myapex.key",
			native_shared_libs: ["lib_dep", "libfoo"],
			compile_multilib: "both",
			file_contexts: ":myapex-file_contexts",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "lib_nodep",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex_nodep" ],
		}

		cc_library {
			name: "lib_dep",
			srcs: ["mylib.cpp"],
			shared_libs: ["libfoo"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex_dep",
				"myapex_provider",
				"myapex_selfcontained",
			],
		}

		cc_library {
			name: "libfoo",
			srcs: ["mytest.cpp"],
			stubs: {
				versions: ["1"],
			},
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex_provider",
				"myapex_selfcontained",
			],
		}
	`)

	var apexManifestRule android.TestingBuildParams
	var provideNativeLibs, requireNativeLibs []string

	apexManifestRule = ctx.ModuleForTests("myapex_nodep", "android_common_myapex_nodep_image").Rule("apexManifestRule")
	provideNativeLibs = names(apexManifestRule.Args["provideNativeLibs"])
	requireNativeLibs = names(apexManifestRule.Args["requireNativeLibs"])
	ensureListEmpty(t, provideNativeLibs)
	ensureListEmpty(t, requireNativeLibs)

	apexManifestRule = ctx.ModuleForTests("myapex_dep", "android_common_myapex_dep_image").Rule("apexManifestRule")
	provideNativeLibs = names(apexManifestRule.Args["provideNativeLibs"])
	requireNativeLibs = names(apexManifestRule.Args["requireNativeLibs"])
	ensureListEmpty(t, provideNativeLibs)
	ensureListContains(t, requireNativeLibs, "libfoo.so")

	apexManifestRule = ctx.ModuleForTests("myapex_provider", "android_common_myapex_provider_image").Rule("apexManifestRule")
	provideNativeLibs = names(apexManifestRule.Args["provideNativeLibs"])
	requireNativeLibs = names(apexManifestRule.Args["requireNativeLibs"])
	ensureListContains(t, provideNativeLibs, "libfoo.so")
	ensureListEmpty(t, requireNativeLibs)

	apexManifestRule = ctx.ModuleForTests("myapex_selfcontained", "android_common_myapex_selfcontained_image").Rule("apexManifestRule")
	provideNativeLibs = names(apexManifestRule.Args["provideNativeLibs"])
	requireNativeLibs = names(apexManifestRule.Args["requireNativeLibs"])
	ensureListContains(t, provideNativeLibs, "libfoo.so")
	ensureListEmpty(t, requireNativeLibs)
}

func TestApexName(t *testing.T) {
	ctx, config := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apex_name: "com.android.myapex",
			native_shared_libs: ["mylib"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}
	`)

	module := ctx.ModuleForTests("myapex", "android_common_myapex_image")
	apexManifestRule := module.Rule("apexManifestRule")
	ensureContains(t, apexManifestRule.Args["opt"], "-v name com.android.myapex")
	apexRule := module.Rule("apexRule")
	ensureContains(t, apexRule.Args["opt_flags"], "--do_not_check_keyname")

	apexBundle := ctx.ModuleForTests("myapex", "android_common_myapex_image").Module().(*apexBundle)
	data := android.AndroidMkDataForTest(t, config, "", apexBundle)
	name := apexBundle.BaseModuleName()
	prefix := "TARGET_"
	var builder strings.Builder
	data.Custom(&builder, name, prefix, "", data)
	androidMk := builder.String()
	ensureContains(t, androidMk, "LOCAL_MODULE := mylib.myapex\n")
	ensureNotContains(t, androidMk, "LOCAL_MODULE := mylib.com.android.myapex\n")
}

func TestNonTestApex(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib_common"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib_common",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
					"//apex_available:platform",
				  "myapex",
		  ],
		}
	`)

	module := ctx.ModuleForTests("myapex", "android_common_myapex_image")
	apexRule := module.Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	if apex, ok := module.Module().(*apexBundle); !ok || apex.testApex {
		t.Log("Apex was a test apex!")
		t.Fail()
	}
	// Ensure that main rule creates an output
	ensureContains(t, apexRule.Output.String(), "myapex.apex.unsigned")

	// Ensure that apex variant is created for the direct dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common"), "android_arm64_armv8-a_shared_apex10000")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image.apex/lib64/mylib_common.so")

	// Ensure that the platform variant ends with _shared
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common"), "android_arm64_armv8-a_shared")

	if !android.InAnyApex("mylib_common") {
		t.Log("Found mylib_common not in any apex!")
		t.Fail()
	}
}

func TestTestApex(t *testing.T) {
	if android.InAnyApex("mylib_common_test") {
		t.Fatal("mylib_common_test must not be used in any other tests since this checks that global state is not updated in an illegal way!")
	}
	ctx, _ := testApex(t, `
		apex_test {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib_common_test"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib_common_test",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}
	`)

	module := ctx.ModuleForTests("myapex", "android_common_myapex_image")
	apexRule := module.Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	if apex, ok := module.Module().(*apexBundle); !ok || !apex.testApex {
		t.Log("Apex was not a test apex!")
		t.Fail()
	}
	// Ensure that main rule creates an output
	ensureContains(t, apexRule.Output.String(), "myapex.apex.unsigned")

	// Ensure that apex variant is created for the direct dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common_test"), "android_arm64_armv8-a_shared_apex10000")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image.apex/lib64/mylib_common_test.so")

	// Ensure that the platform variant ends with _shared
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common_test"), "android_arm64_armv8-a_shared")
}

func TestApexWithTarget(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			multilib: {
				first: {
					native_shared_libs: ["mylib_common"],
				}
			},
			target: {
				android: {
					multilib: {
						first: {
							native_shared_libs: ["mylib"],
						}
					}
				},
				host: {
					multilib: {
						first: {
							native_shared_libs: ["mylib2"],
						}
					}
				}
			}
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		cc_library {
			name: "mylib_common",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			compile_multilib: "first",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			compile_multilib: "first",
		}
	`)

	apexRule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that main rule creates an output
	ensureContains(t, apexRule.Output.String(), "myapex.apex.unsigned")

	// Ensure that apex variant is created for the direct dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_shared_apex10000")
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common"), "android_arm64_armv8-a_shared_apex10000")
	ensureListNotContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_shared_apex10000")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")
	ensureContains(t, copyCmds, "image.apex/lib64/mylib_common.so")
	ensureNotContains(t, copyCmds, "image.apex/lib64/mylib2.so")

	// Ensure that the platform variant ends with _shared
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_shared")
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common"), "android_arm64_armv8-a_shared")
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_shared")
}

func TestApexWithShBinary(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			binaries: ["myscript"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		sh_binary {
			name: "myscript",
			src: "mylib.cpp",
			filename: "myscript.sh",
			sub_dir: "script",
		}
	`)

	apexRule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	ensureContains(t, copyCmds, "image.apex/bin/script/myscript.sh")
}

func TestApexInVariousPartition(t *testing.T) {
	testcases := []struct {
		propName, parition, flattenedPartition string
	}{
		{"", "system", "system_ext"},
		{"product_specific: true", "product", "product"},
		{"soc_specific: true", "vendor", "vendor"},
		{"proprietary: true", "vendor", "vendor"},
		{"vendor: true", "vendor", "vendor"},
		{"system_ext_specific: true", "system_ext", "system_ext"},
	}
	for _, tc := range testcases {
		t.Run(tc.propName+":"+tc.parition, func(t *testing.T) {
			ctx, _ := testApex(t, `
				apex {
					name: "myapex",
					key: "myapex.key",
					`+tc.propName+`
				}

				apex_key {
					name: "myapex.key",
					public_key: "testkey.avbpubkey",
					private_key: "testkey.pem",
				}
			`)

			apex := ctx.ModuleForTests("myapex", "android_common_myapex_image").Module().(*apexBundle)
			expected := buildDir + "/target/product/test_device/" + tc.parition + "/apex"
			actual := apex.installDir.String()
			if actual != expected {
				t.Errorf("wrong install path. expected %q. actual %q", expected, actual)
			}

			flattened := ctx.ModuleForTests("myapex", "android_common_myapex_flattened").Module().(*apexBundle)
			expected = buildDir + "/target/product/test_device/" + tc.flattenedPartition + "/apex"
			actual = flattened.installDir.String()
			if actual != expected {
				t.Errorf("wrong install path. expected %q. actual %q", expected, actual)
			}
		})
	}
}

func TestFileContexts(t *testing.T) {
	ctx, _ := testApex(t, `
	apex {
		name: "myapex",
		key: "myapex.key",
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}
	`)
	module := ctx.ModuleForTests("myapex", "android_common_myapex_image")
	apexRule := module.Rule("apexRule")
	actual := apexRule.Args["file_contexts"]
	expected := "system/sepolicy/apex/myapex-file_contexts"
	if actual != expected {
		t.Errorf("wrong file_contexts. expected %q. actual %q", expected, actual)
	}

	testApexError(t, `"myapex" .*: file_contexts: should be under system/sepolicy`, `
	apex {
		name: "myapex",
		key: "myapex.key",
		file_contexts: "my_own_file_contexts",
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}
	`, withFiles(map[string][]byte{
		"my_own_file_contexts": nil,
	}))

	testApexError(t, `"myapex" .*: file_contexts: cannot find`, `
	apex {
		name: "myapex",
		key: "myapex.key",
		product_specific: true,
		file_contexts: "product_specific_file_contexts",
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}
	`)

	ctx, _ = testApex(t, `
	apex {
		name: "myapex",
		key: "myapex.key",
		product_specific: true,
		file_contexts: "product_specific_file_contexts",
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}
	`, withFiles(map[string][]byte{
		"product_specific_file_contexts": nil,
	}))
	module = ctx.ModuleForTests("myapex", "android_common_myapex_image")
	apexRule = module.Rule("apexRule")
	actual = apexRule.Args["file_contexts"]
	expected = "product_specific_file_contexts"
	if actual != expected {
		t.Errorf("wrong file_contexts. expected %q. actual %q", expected, actual)
	}

	ctx, _ = testApex(t, `
	apex {
		name: "myapex",
		key: "myapex.key",
		product_specific: true,
		file_contexts: ":my-file-contexts",
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	filegroup {
		name: "my-file-contexts",
		srcs: ["product_specific_file_contexts"],
	}
	`, withFiles(map[string][]byte{
		"product_specific_file_contexts": nil,
	}))
	module = ctx.ModuleForTests("myapex", "android_common_myapex_image")
	apexRule = module.Rule("apexRule")
	actual = apexRule.Args["file_contexts"]
	expected = "product_specific_file_contexts"
	if actual != expected {
		t.Errorf("wrong file_contexts. expected %q. actual %q", expected, actual)
	}
}

func TestApexKeyFromOtherModule(t *testing.T) {
	ctx, _ := testApex(t, `
		apex_key {
			name: "myapex.key",
			public_key: ":my.avbpubkey",
			private_key: ":my.pem",
			product_specific: true,
		}

		filegroup {
			name: "my.avbpubkey",
			srcs: ["testkey2.avbpubkey"],
		}

		filegroup {
			name: "my.pem",
			srcs: ["testkey2.pem"],
		}
	`)

	apex_key := ctx.ModuleForTests("myapex.key", "android_common").Module().(*apexKey)
	expected_pubkey := "testkey2.avbpubkey"
	actual_pubkey := apex_key.public_key_file.String()
	if actual_pubkey != expected_pubkey {
		t.Errorf("wrong public key path. expected %q. actual %q", expected_pubkey, actual_pubkey)
	}
	expected_privkey := "testkey2.pem"
	actual_privkey := apex_key.private_key_file.String()
	if actual_privkey != expected_privkey {
		t.Errorf("wrong private key path. expected %q. actual %q", expected_privkey, actual_privkey)
	}
}

func TestPrebuilt(t *testing.T) {
	ctx, _ := testApex(t, `
		prebuilt_apex {
			name: "myapex",
			arch: {
				arm64: {
					src: "myapex-arm64.apex",
				},
				arm: {
					src: "myapex-arm.apex",
				},
			},
		}
	`)

	prebuilt := ctx.ModuleForTests("myapex", "android_common").Module().(*Prebuilt)

	expectedInput := "myapex-arm64.apex"
	if prebuilt.inputApex.String() != expectedInput {
		t.Errorf("inputApex invalid. expected: %q, actual: %q", expectedInput, prebuilt.inputApex.String())
	}
}

func TestPrebuiltFilenameOverride(t *testing.T) {
	ctx, _ := testApex(t, `
		prebuilt_apex {
			name: "myapex",
			src: "myapex-arm.apex",
			filename: "notmyapex.apex",
		}
	`)

	p := ctx.ModuleForTests("myapex", "android_common").Module().(*Prebuilt)

	expected := "notmyapex.apex"
	if p.installFilename != expected {
		t.Errorf("installFilename invalid. expected: %q, actual: %q", expected, p.installFilename)
	}
}

func TestPrebuiltOverrides(t *testing.T) {
	ctx, config := testApex(t, `
		prebuilt_apex {
			name: "myapex.prebuilt",
			src: "myapex-arm.apex",
			overrides: [
				"myapex",
			],
		}
	`)

	p := ctx.ModuleForTests("myapex.prebuilt", "android_common").Module().(*Prebuilt)

	expected := []string{"myapex"}
	actual := android.AndroidMkEntriesForTest(t, config, "", p)[0].EntryMap["LOCAL_OVERRIDES_MODULES"]
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Incorrect LOCAL_OVERRIDES_MODULES value '%s', expected '%s'", actual, expected)
	}
}

func TestApexWithTests(t *testing.T) {
	ctx, config := testApex(t, `
		apex_test {
			name: "myapex",
			key: "myapex.key",
			tests: [
				"mytest",
				"mytests",
			],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_test {
			name: "mytest",
			gtest: false,
			srcs: ["mytest.cpp"],
			relative_install_path: "test",
			system_shared_libs: [],
			static_executable: true,
			stl: "none",
		}

		cc_test {
			name: "mytests",
			gtest: false,
			srcs: [
				"mytest1.cpp",
				"mytest2.cpp",
				"mytest3.cpp",
			],
			test_per_src: true,
			relative_install_path: "test",
			system_shared_libs: [],
			static_executable: true,
			stl: "none",
		}
	`)

	apexRule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that test dep is copied into apex.
	ensureContains(t, copyCmds, "image.apex/bin/test/mytest")

	// Ensure that test deps built with `test_per_src` are copied into apex.
	ensureContains(t, copyCmds, "image.apex/bin/test/mytest1")
	ensureContains(t, copyCmds, "image.apex/bin/test/mytest2")
	ensureContains(t, copyCmds, "image.apex/bin/test/mytest3")

	// Ensure the module is correctly translated.
	apexBundle := ctx.ModuleForTests("myapex", "android_common_myapex_image").Module().(*apexBundle)
	data := android.AndroidMkDataForTest(t, config, "", apexBundle)
	name := apexBundle.BaseModuleName()
	prefix := "TARGET_"
	var builder strings.Builder
	data.Custom(&builder, name, prefix, "", data)
	androidMk := builder.String()
	ensureContains(t, androidMk, "LOCAL_MODULE := mytest.myapex\n")
	ensureContains(t, androidMk, "LOCAL_MODULE := mytest1.myapex\n")
	ensureContains(t, androidMk, "LOCAL_MODULE := mytest2.myapex\n")
	ensureContains(t, androidMk, "LOCAL_MODULE := mytest3.myapex\n")
	ensureContains(t, androidMk, "LOCAL_MODULE := apex_manifest.pb.myapex\n")
	ensureContains(t, androidMk, "LOCAL_MODULE := apex_pubkey.myapex\n")
	ensureContains(t, androidMk, "LOCAL_MODULE := myapex\n")
}

func TestInstallExtraFlattenedApexes(t *testing.T) {
	ctx, config := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`, func(fs map[string][]byte, config android.Config) {
		config.TestProductVariables.InstallExtraFlattenedApexes = proptools.BoolPtr(true)
	})
	ab := ctx.ModuleForTests("myapex", "android_common_myapex_image").Module().(*apexBundle)
	ensureListContains(t, ab.requiredDeps, "myapex.flattened")
	mk := android.AndroidMkDataForTest(t, config, "", ab)
	var builder strings.Builder
	mk.Custom(&builder, ab.Name(), "TARGET_", "", mk)
	androidMk := builder.String()
	ensureContains(t, androidMk, "LOCAL_REQUIRED_MODULES += myapex.flattened")
}

func TestApexUsesOtherApex(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			uses: ["commonapex"],
		}

		apex {
			name: "commonapex",
			key: "myapex.key",
			native_shared_libs: ["libcommon"],
			provide_cpp_shared_libs: true,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["libcommon"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "libcommon",
			srcs: ["mylib_common.cpp"],
			system_shared_libs: [],
			stl: "none",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"commonapex",
				"myapex",
			],
		}
	`)

	module1 := ctx.ModuleForTests("myapex", "android_common_myapex_image")
	apexRule1 := module1.Rule("apexRule")
	copyCmds1 := apexRule1.Args["copy_commands"]

	module2 := ctx.ModuleForTests("commonapex", "android_common_commonapex_image")
	apexRule2 := module2.Rule("apexRule")
	copyCmds2 := apexRule2.Args["copy_commands"]

	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_shared_apex10000")
	ensureListContains(t, ctx.ModuleVariantsForTests("libcommon"), "android_arm64_armv8-a_shared_apex10000")
	ensureContains(t, copyCmds1, "image.apex/lib64/mylib.so")
	ensureContains(t, copyCmds2, "image.apex/lib64/libcommon.so")
	ensureNotContains(t, copyCmds1, "image.apex/lib64/libcommon.so")
}

func TestApexUsesFailsIfNotProvided(t *testing.T) {
	testApexError(t, `uses: "commonapex" does not provide native_shared_libs`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			uses: ["commonapex"],
		}

		apex {
			name: "commonapex",
			key: "myapex.key",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)
	testApexError(t, `uses: "commonapex" is not a provider`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			uses: ["commonapex"],
		}

		cc_library {
			name: "commonapex",
			system_shared_libs: [],
			stl: "none",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)
}

func TestApexUsesFailsIfUseVenderMismatch(t *testing.T) {
	testApexError(t, `use_vendor: "commonapex" has different value of use_vendor`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			use_vendor: true,
			uses: ["commonapex"],
		}

		apex {
			name: "commonapex",
			key: "myapex.key",
			provide_cpp_shared_libs: true,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`, func(fs map[string][]byte, config android.Config) {
		setUseVendorAllowListForTest(config, []string{"myapex"})
	})
}

func TestErrorsIfDepsAreNotEnabled(t *testing.T) {
	testApexError(t, `module "myapex" .* depends on disabled module "libfoo"`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libfoo"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libfoo",
			stl: "none",
			system_shared_libs: [],
			enabled: false,
			apex_available: ["myapex"],
		}
	`)
	testApexError(t, `module "myapex" .* depends on disabled module "myjar"`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["myjar"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "myjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			enabled: false,
			apex_available: ["myapex"],
		}
	`)
}

func TestApexWithApps(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apps: [
				"AppFoo",
				"AppFooPriv",
			],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_app {
			name: "AppFoo",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "current",
			system_modules: "none",
			jni_libs: ["libjni"],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		android_app {
			name: "AppFooPriv",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "current",
			system_modules: "none",
			privileged: true,
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library_shared {
			name: "libjni",
			srcs: ["mylib.cpp"],
			shared_libs: ["libfoo"],
			stl: "none",
			system_shared_libs: [],
			apex_available: [ "myapex" ],
			sdk_version: "current",
		}

		cc_library_shared {
			name: "libfoo",
			stl: "none",
			system_shared_libs: [],
			apex_available: [ "myapex" ],
			sdk_version: "current",
		}
	`)

	module := ctx.ModuleForTests("myapex", "android_common_myapex_image")
	apexRule := module.Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	ensureContains(t, copyCmds, "image.apex/app/AppFoo/AppFoo.apk")
	ensureContains(t, copyCmds, "image.apex/priv-app/AppFooPriv/AppFooPriv.apk")

	appZipRule := ctx.ModuleForTests("AppFoo", "android_common_apex10000").Description("zip jni libs")
	// JNI libraries are uncompressed
	if args := appZipRule.Args["jarArgs"]; !strings.Contains(args, "-L 0") {
		t.Errorf("jni libs are not uncompressed for AppFoo")
	}
	// JNI libraries including transitive deps are
	for _, jni := range []string{"libjni", "libfoo"} {
		jniOutput := ctx.ModuleForTests(jni, "android_arm64_armv8-a_sdk_shared_apex10000").Module().(*cc.Module).OutputFile()
		// ... embedded inside APK (jnilibs.zip)
		ensureListContains(t, appZipRule.Implicits.Strings(), jniOutput.String())
		// ... and not directly inside the APEX
		ensureNotContains(t, copyCmds, "image.apex/lib64/"+jni+".so")
	}
}

func TestApexWithAppImports(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apps: [
				"AppFooPrebuilt",
				"AppFooPrivPrebuilt",
			],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_app_import {
			name: "AppFooPrebuilt",
			apk: "PrebuiltAppFoo.apk",
			presigned: true,
			dex_preopt: {
				enabled: false,
			},
		}

		android_app_import {
			name: "AppFooPrivPrebuilt",
			apk: "PrebuiltAppFooPriv.apk",
			privileged: true,
			presigned: true,
			dex_preopt: {
				enabled: false,
			},
			filename: "AwesomePrebuiltAppFooPriv.apk",
		}
	`)

	module := ctx.ModuleForTests("myapex", "android_common_myapex_image")
	apexRule := module.Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	ensureContains(t, copyCmds, "image.apex/app/AppFooPrebuilt/AppFooPrebuilt.apk")
	ensureContains(t, copyCmds, "image.apex/priv-app/AppFooPrivPrebuilt/AwesomePrebuiltAppFooPriv.apk")
}

func TestApexWithAppImportsPrefer(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apps: [
				"AppFoo",
			],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_app {
			name: "AppFoo",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			apex_available: [ "myapex" ],
		}

		android_app_import {
			name: "AppFoo",
			apk: "AppFooPrebuilt.apk",
			filename: "AppFooPrebuilt.apk",
			presigned: true,
			prefer: true,
		}
	`, withFiles(map[string][]byte{
		"AppFooPrebuilt.apk": nil,
	}))

	ensureExactContents(t, ctx, "myapex", "android_common_myapex_image", []string{
		"app/AppFoo/AppFooPrebuilt.apk",
	})
}

func TestApexWithTestHelperApp(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apps: [
				"TesterHelpAppFoo",
			],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_test_helper_app {
			name: "TesterHelpAppFoo",
			srcs: ["foo/bar/MyClass.java"],
			apex_available: [ "myapex" ],
		}

	`)

	module := ctx.ModuleForTests("myapex", "android_common_myapex_image")
	apexRule := module.Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	ensureContains(t, copyCmds, "image.apex/app/TesterHelpAppFoo/TesterHelpAppFoo.apk")
}

func TestApexPropertiesShouldBeDefaultable(t *testing.T) {
	// libfoo's apex_available comes from cc_defaults
	testApexError(t, `requires "libfoo" that is not available for the APEX`, `
	apex {
		name: "myapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo"],
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	apex {
		name: "otherapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo"],
	}

	cc_defaults {
		name: "libfoo-defaults",
		apex_available: ["otherapex"],
	}

	cc_library {
		name: "libfoo",
		defaults: ["libfoo-defaults"],
		stl: "none",
		system_shared_libs: [],
	}`)
}

func TestApexAvailable_DirectDep(t *testing.T) {
	// libfoo is not available to myapex, but only to otherapex
	testApexError(t, "requires \"libfoo\" that is not available for the APEX", `
	apex {
		name: "myapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo"],
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	apex {
		name: "otherapex",
		key: "otherapex.key",
		native_shared_libs: ["libfoo"],
	}

	apex_key {
		name: "otherapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	cc_library {
		name: "libfoo",
		stl: "none",
		system_shared_libs: [],
		apex_available: ["otherapex"],
	}`)
}

func TestApexAvailable_IndirectDep(t *testing.T) {
	// libbbaz is an indirect dep
	testApexError(t, `requires "libbaz" that is not available for the APEX. Dependency path:
.*via tag apex\.dependencyTag.*"sharedLib".*
.*-> libfoo.*link:shared.*
.*via tag cc\.DependencyTag.*"shared".*
.*-> libbar.*link:shared.*
.*via tag cc\.DependencyTag.*"shared".*
.*-> libbaz.*link:shared.*`, `
	apex {
		name: "myapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo"],
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	cc_library {
		name: "libfoo",
		stl: "none",
		shared_libs: ["libbar"],
		system_shared_libs: [],
		apex_available: ["myapex"],
	}

	cc_library {
		name: "libbar",
		stl: "none",
		shared_libs: ["libbaz"],
		system_shared_libs: [],
		apex_available: ["myapex"],
	}

	cc_library {
		name: "libbaz",
		stl: "none",
		system_shared_libs: [],
	}`)
}

func TestApexAvailable_InvalidApexName(t *testing.T) {
	testApexError(t, "\"otherapex\" is not a valid module name", `
	apex {
		name: "myapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo"],
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	cc_library {
		name: "libfoo",
		stl: "none",
		system_shared_libs: [],
		apex_available: ["otherapex"],
	}`)

	testApex(t, `
	apex {
		name: "myapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo", "libbar"],
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	cc_library {
		name: "libfoo",
		stl: "none",
		system_shared_libs: [],
		runtime_libs: ["libbaz"],
		apex_available: ["myapex"],
	}

	cc_library {
		name: "libbar",
		stl: "none",
		system_shared_libs: [],
		apex_available: ["//apex_available:anyapex"],
	}

	cc_library {
		name: "libbaz",
		stl: "none",
		system_shared_libs: [],
		stubs: {
			versions: ["10", "20", "30"],
		},
	}`)
}

func TestApexAvailable_CheckForPlatform(t *testing.T) {
	ctx, _ := testApex(t, `
	apex {
		name: "myapex",
		key: "myapex.key",
		native_shared_libs: ["libbar", "libbaz"],
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	cc_library {
		name: "libfoo",
		stl: "none",
		system_shared_libs: [],
		shared_libs: ["libbar"],
		apex_available: ["//apex_available:platform"],
	}

	cc_library {
		name: "libfoo2",
		stl: "none",
		system_shared_libs: [],
		shared_libs: ["libbaz"],
		apex_available: ["//apex_available:platform"],
	}

	cc_library {
		name: "libbar",
		stl: "none",
		system_shared_libs: [],
		apex_available: ["myapex"],
	}

	cc_library {
		name: "libbaz",
		stl: "none",
		system_shared_libs: [],
		apex_available: ["myapex"],
		stubs: {
			versions: ["1"],
		},
	}`)

	// libfoo shouldn't be available to platform even though it has "//apex_available:platform",
	// because it depends on libbar which isn't available to platform
	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module().(*cc.Module)
	if libfoo.NotAvailableForPlatform() != true {
		t.Errorf("%q shouldn't be available to platform", libfoo.String())
	}

	// libfoo2 however can be available to platform because it depends on libbaz which provides
	// stubs
	libfoo2 := ctx.ModuleForTests("libfoo2", "android_arm64_armv8-a_shared").Module().(*cc.Module)
	if libfoo2.NotAvailableForPlatform() == true {
		t.Errorf("%q should be available to platform", libfoo2.String())
	}
}

func TestApexAvailable_CreatedForApex(t *testing.T) {
	ctx, _ := testApex(t, `
	apex {
		name: "myapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo"],
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	cc_library {
		name: "libfoo",
		stl: "none",
		system_shared_libs: [],
		apex_available: ["myapex"],
		static: {
			apex_available: ["//apex_available:platform"],
		},
	}`)

	libfooShared := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module().(*cc.Module)
	if libfooShared.NotAvailableForPlatform() != true {
		t.Errorf("%q shouldn't be available to platform", libfooShared.String())
	}
	libfooStatic := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_static").Module().(*cc.Module)
	if libfooStatic.NotAvailableForPlatform() != false {
		t.Errorf("%q should be available to platform", libfooStatic.String())
	}
}

func TestOverrideApex(t *testing.T) {
	ctx, config := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apps: ["app"],
			overrides: ["oldapex"],
		}

		override_apex {
			name: "override_myapex",
			base: "myapex",
			apps: ["override_app"],
			overrides: ["unknownapex"],
			logging_parent: "com.foo.bar",
			package_name: "test.overridden.package",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_app {
			name: "app",
			srcs: ["foo/bar/MyClass.java"],
			package_name: "foo",
			sdk_version: "none",
			system_modules: "none",
			apex_available: [ "myapex" ],
		}

		override_android_app {
			name: "override_app",
			base: "app",
			package_name: "bar",
		}
	`, withManifestPackageNameOverrides([]string{"myapex:com.android.myapex"}))

	originalVariant := ctx.ModuleForTests("myapex", "android_common_myapex_image").Module().(android.OverridableModule)
	overriddenVariant := ctx.ModuleForTests("myapex", "android_common_override_myapex_myapex_image").Module().(android.OverridableModule)
	if originalVariant.GetOverriddenBy() != "" {
		t.Errorf("GetOverriddenBy should be empty, but was %q", originalVariant.GetOverriddenBy())
	}
	if overriddenVariant.GetOverriddenBy() != "override_myapex" {
		t.Errorf("GetOverriddenBy should be \"override_myapex\", but was %q", overriddenVariant.GetOverriddenBy())
	}

	module := ctx.ModuleForTests("myapex", "android_common_override_myapex_myapex_image")
	apexRule := module.Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	ensureNotContains(t, copyCmds, "image.apex/app/app/app.apk")
	ensureContains(t, copyCmds, "image.apex/app/override_app/override_app.apk")

	apexBundle := module.Module().(*apexBundle)
	name := apexBundle.Name()
	if name != "override_myapex" {
		t.Errorf("name should be \"override_myapex\", but was %q", name)
	}

	if apexBundle.overridableProperties.Logging_parent != "com.foo.bar" {
		t.Errorf("override_myapex should have logging parent (com.foo.bar), but was %q.", apexBundle.overridableProperties.Logging_parent)
	}

	optFlags := apexRule.Args["opt_flags"]
	ensureContains(t, optFlags, "--override_apk_package_name test.overridden.package")

	data := android.AndroidMkDataForTest(t, config, "", apexBundle)
	var builder strings.Builder
	data.Custom(&builder, name, "TARGET_", "", data)
	androidMk := builder.String()
	ensureContains(t, androidMk, "LOCAL_MODULE := override_app.override_myapex")
	ensureContains(t, androidMk, "LOCAL_MODULE := apex_manifest.pb.override_myapex")
	ensureContains(t, androidMk, "LOCAL_MODULE_STEM := override_myapex.apex")
	ensureContains(t, androidMk, "LOCAL_OVERRIDES_MODULES := unknownapex myapex")
	ensureNotContains(t, androidMk, "LOCAL_MODULE := app.myapex")
	ensureNotContains(t, androidMk, "LOCAL_MODULE := override_app.myapex")
	ensureNotContains(t, androidMk, "LOCAL_MODULE := apex_manifest.pb.myapex")
	ensureNotContains(t, androidMk, "LOCAL_MODULE_STEM := myapex.apex")
}

func TestLegacyAndroid10Support(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			stl: "libc++",
			system_shared_libs: [],
			apex_available: [ "myapex" ],
		}
	`, withUnbundledBuild)

	module := ctx.ModuleForTests("myapex", "android_common_myapex_image")
	args := module.Rule("apexRule").Args
	ensureContains(t, args["opt_flags"], "--manifest_json "+module.Output("apex_manifest.json").Output.String())
	ensureNotContains(t, args["opt_flags"], "--no_hashtree")

	// The copies of the libraries in the apex should have one more dependency than
	// the ones outside the apex, namely the unwinder. Ideally we should check
	// the dependency names directly here but for some reason the names are blank in
	// this test.
	for _, lib := range []string{"libc++", "mylib"} {
		apexImplicits := ctx.ModuleForTests(lib, "android_arm64_armv8-a_shared_apex29").Rule("ld").Implicits
		nonApexImplicits := ctx.ModuleForTests(lib, "android_arm64_armv8-a_shared").Rule("ld").Implicits
		if len(apexImplicits) != len(nonApexImplicits)+1 {
			t.Errorf("%q missing unwinder dep", lib)
		}
	}
}

var filesForSdkLibrary = map[string][]byte{
	"api/current.txt":        nil,
	"api/removed.txt":        nil,
	"api/system-current.txt": nil,
	"api/system-removed.txt": nil,
	"api/test-current.txt":   nil,
	"api/test-removed.txt":   nil,

	// For java_sdk_library_import
	"a.jar": nil,
}

func TestJavaSDKLibrary(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["foo"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			api_packages: ["foo"],
			apex_available: [ "myapex" ],
		}
	`, withFiles(filesForSdkLibrary))

	// java_sdk_library installs both impl jar and permission XML
	ensureExactContents(t, ctx, "myapex", "android_common_myapex_image", []string{
		"javalib/foo.jar",
		"etc/permissions/foo.xml",
	})
	// Permission XML should point to the activated path of impl jar of java_sdk_library
	sdkLibrary := ctx.ModuleForTests("foo.xml", "android_common_myapex").Rule("java_sdk_xml")
	ensureContains(t, sdkLibrary.RuleParams.Command, `<library name=\"foo\" file=\"/apex/myapex/javalib/foo.jar\"`)
}

func TestJavaSDKLibrary_WithinApex(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["foo", "bar"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			api_packages: ["foo"],
			apex_available: ["myapex"],
			sdk_version: "none",
			system_modules: "none",
		}

		java_library {
			name: "bar",
			srcs: ["a.java"],
			libs: ["foo"],
			apex_available: ["myapex"],
			sdk_version: "none",
			system_modules: "none",
		}
	`, withFiles(filesForSdkLibrary))

	// java_sdk_library installs both impl jar and permission XML
	ensureExactContents(t, ctx, "myapex", "android_common_myapex_image", []string{
		"javalib/bar.jar",
		"javalib/foo.jar",
		"etc/permissions/foo.xml",
	})

	// The bar library should depend on the implementation jar.
	barLibrary := ctx.ModuleForTests("bar", "android_common_myapex").Rule("javac")
	if expected, actual := `^-classpath /[^:]*/turbine-combined/foo\.jar$`, barLibrary.Args["classpath"]; !regexp.MustCompile(expected).MatchString(actual) {
		t.Errorf("expected %q, found %#q", expected, actual)
	}
}

func TestJavaSDKLibrary_CrossBoundary(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["foo"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			api_packages: ["foo"],
			apex_available: ["myapex"],
			sdk_version: "none",
			system_modules: "none",
		}

		java_library {
			name: "bar",
			srcs: ["a.java"],
			libs: ["foo"],
			sdk_version: "none",
			system_modules: "none",
		}
	`, withFiles(filesForSdkLibrary))

	// java_sdk_library installs both impl jar and permission XML
	ensureExactContents(t, ctx, "myapex", "android_common_myapex_image", []string{
		"javalib/foo.jar",
		"etc/permissions/foo.xml",
	})

	// The bar library should depend on the stubs jar.
	barLibrary := ctx.ModuleForTests("bar", "android_common").Rule("javac")
	if expected, actual := `^-classpath /[^:]*/turbine-combined/foo\.stubs\.jar$`, barLibrary.Args["classpath"]; !regexp.MustCompile(expected).MatchString(actual) {
		t.Errorf("expected %q, found %#q", expected, actual)
	}
}

func TestJavaSDKLibrary_ImportPreferred(t *testing.T) {
	ctx, _ := testApex(t, ``,
		withFiles(map[string][]byte{
			"apex/a.java":             nil,
			"apex/apex_manifest.json": nil,
			"apex/Android.bp": []byte(`
		package {
			default_visibility: ["//visibility:private"],
		}

		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["foo", "bar"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "bar",
			srcs: ["a.java"],
			libs: ["foo"],
			apex_available: ["myapex"],
			sdk_version: "none",
			system_modules: "none",
		}
`),
			"source/a.java":          nil,
			"source/api/current.txt": nil,
			"source/api/removed.txt": nil,
			"source/Android.bp": []byte(`
		package {
			default_visibility: ["//visibility:private"],
		}

		java_sdk_library {
			name: "foo",
			visibility: ["//apex"],
			srcs: ["a.java"],
			api_packages: ["foo"],
			apex_available: ["myapex"],
			sdk_version: "none",
			system_modules: "none",
			public: {
				enabled: true,
			},
		}
`),
			"prebuilt/a.jar": nil,
			"prebuilt/Android.bp": []byte(`
		package {
			default_visibility: ["//visibility:private"],
		}

		java_sdk_library_import {
			name: "foo",
			visibility: ["//apex", "//source"],
			apex_available: ["myapex"],
			prefer: true,
			public: {
				jars: ["a.jar"],
			},
		}
`),
		}),
	)

	// java_sdk_library installs both impl jar and permission XML
	ensureExactContents(t, ctx, "myapex", "android_common_myapex_image", []string{
		"javalib/bar.jar",
		"javalib/foo.jar",
		"etc/permissions/foo.xml",
	})

	// The bar library should depend on the implementation jar.
	barLibrary := ctx.ModuleForTests("bar", "android_common_myapex").Rule("javac")
	if expected, actual := `^-classpath /[^:]*/turbine-combined/foo\.impl\.jar$`, barLibrary.Args["classpath"]; !regexp.MustCompile(expected).MatchString(actual) {
		t.Errorf("expected %q, found %#q", expected, actual)
	}
}

func TestJavaSDKLibrary_ImportOnly(t *testing.T) {
	testApexError(t, `java_libs: "foo" is not configured to be compiled into dex`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["foo"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_sdk_library_import {
			name: "foo",
			apex_available: ["myapex"],
			prefer: true,
			public: {
				jars: ["a.jar"],
			},
		}

	`, withFiles(filesForSdkLibrary))
}

func TestCompatConfig(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			prebuilts: ["myjar-platform-compat-config"],
			java_libs: ["myjar"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		platform_compat_config {
		    name: "myjar-platform-compat-config",
		    src: ":myjar",
		}

		java_library {
			name: "myjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			apex_available: [ "myapex" ],
		}
	`)
	ensureExactContents(t, ctx, "myapex", "android_common_myapex_image", []string{
		"etc/compatconfig/myjar-platform-compat-config.xml",
		"javalib/myjar.jar",
	})
}

func TestRejectNonInstallableJavaLibrary(t *testing.T) {
	testApexError(t, `"myjar" is not configured to be compiled into dex`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["myjar"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "myjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			compile_dex: false,
			apex_available: ["myapex"],
		}
	`)
}

func TestCarryRequiredModuleNames(t *testing.T) {
	ctx, config := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			required: ["a", "b"],
			host_required: ["c", "d"],
			target_required: ["e", "f"],
			apex_available: [ "myapex" ],
		}
	`)

	apexBundle := ctx.ModuleForTests("myapex", "android_common_myapex_image").Module().(*apexBundle)
	data := android.AndroidMkDataForTest(t, config, "", apexBundle)
	name := apexBundle.BaseModuleName()
	prefix := "TARGET_"
	var builder strings.Builder
	data.Custom(&builder, name, prefix, "", data)
	androidMk := builder.String()
	ensureContains(t, androidMk, "LOCAL_REQUIRED_MODULES += a b\n")
	ensureContains(t, androidMk, "LOCAL_HOST_REQUIRED_MODULES += c d\n")
	ensureContains(t, androidMk, "LOCAL_TARGET_REQUIRED_MODULES += e f\n")
}

func TestSymlinksFromApexToSystem(t *testing.T) {
	bp := `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			java_libs: ["myjar"],
		}

		apex {
			name: "myapex.updatable",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			java_libs: ["myjar"],
			updatable: true,
			min_sdk_version: "current",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["myotherlib"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex",
				"myapex.updatable",
				"//apex_available:platform",
			],
		}

		cc_library {
			name: "myotherlib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex",
				"myapex.updatable",
				"//apex_available:platform",
			],
		}

		java_library {
			name: "myjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			libs: ["myotherjar"],
			apex_available: [
				"myapex",
				"myapex.updatable",
				"//apex_available:platform",
			],
		}

		java_library {
			name: "myotherjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			apex_available: [
				"myapex",
				"myapex.updatable",
				"//apex_available:platform",
			],
		}
	`

	ensureRealfileExists := func(t *testing.T, files []fileInApex, file string) {
		for _, f := range files {
			if f.path == file {
				if f.isLink {
					t.Errorf("%q is not a real file", file)
				}
				return
			}
		}
		t.Errorf("%q is not found", file)
	}

	ensureSymlinkExists := func(t *testing.T, files []fileInApex, file string) {
		for _, f := range files {
			if f.path == file {
				if !f.isLink {
					t.Errorf("%q is not a symlink", file)
				}
				return
			}
		}
		t.Errorf("%q is not found", file)
	}

	// For unbundled build, symlink shouldn't exist regardless of whether an APEX
	// is updatable or not
	ctx, _ := testApex(t, bp, withUnbundledBuild)
	files := getFiles(t, ctx, "myapex", "android_common_myapex_image")
	ensureRealfileExists(t, files, "javalib/myjar.jar")
	ensureRealfileExists(t, files, "lib64/mylib.so")
	ensureRealfileExists(t, files, "lib64/myotherlib.so")

	files = getFiles(t, ctx, "myapex.updatable", "android_common_myapex.updatable_image")
	ensureRealfileExists(t, files, "javalib/myjar.jar")
	ensureRealfileExists(t, files, "lib64/mylib.so")
	ensureRealfileExists(t, files, "lib64/myotherlib.so")

	// For bundled build, symlink to the system for the non-updatable APEXes only
	ctx, _ = testApex(t, bp)
	files = getFiles(t, ctx, "myapex", "android_common_myapex_image")
	ensureRealfileExists(t, files, "javalib/myjar.jar")
	ensureRealfileExists(t, files, "lib64/mylib.so")
	ensureSymlinkExists(t, files, "lib64/myotherlib.so") // this is symlink

	files = getFiles(t, ctx, "myapex.updatable", "android_common_myapex.updatable_image")
	ensureRealfileExists(t, files, "javalib/myjar.jar")
	ensureRealfileExists(t, files, "lib64/mylib.so")
	ensureRealfileExists(t, files, "lib64/myotherlib.so") // this is a real file
}

func TestApexMutatorsDontRunIfDisabled(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`, func(fs map[string][]byte, config android.Config) {
		delete(config.Targets, android.Android)
		config.AndroidCommonTarget = android.Target{}
	})

	if expected, got := []string{""}, ctx.ModuleVariantsForTests("myapex"); !reflect.DeepEqual(expected, got) {
		t.Errorf("Expected variants: %v, but got: %v", expected, got)
	}
}

func TestAppBundle(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apps: ["AppFoo"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_app {
			name: "AppFoo",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			apex_available: [ "myapex" ],
		}
		`, withManifestPackageNameOverrides([]string{"AppFoo:com.android.foo"}))

	bundleConfigRule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Description("Bundle Config")
	content := bundleConfigRule.Args["content"]

	ensureContains(t, content, `"compression":{"uncompressed_glob":["apex_payload.img","apex_manifest.*"]}`)
	ensureContains(t, content, `"apex_config":{"apex_embedded_apk_config":[{"package_name":"com.android.foo","path":"app/AppFoo/AppFoo.apk"}]}`)
}

func TestAppSetBundle(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apps: ["AppSet"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_app_set {
			name: "AppSet",
			set: "AppSet.apks",
		}`)
	mod := ctx.ModuleForTests("myapex", "android_common_myapex_image")
	bundleConfigRule := mod.Description("Bundle Config")
	content := bundleConfigRule.Args["content"]
	ensureContains(t, content, `"compression":{"uncompressed_glob":["apex_payload.img","apex_manifest.*"]}`)
	s := mod.Rule("apexRule").Args["copy_commands"]
	copyCmds := regexp.MustCompile(" *&& *").Split(s, -1)
	if len(copyCmds) != 3 {
		t.Fatalf("Expected 3 commands, got %d in:\n%s", len(copyCmds), s)
	}
	ensureMatches(t, copyCmds[0], "^rm -rf .*/app/AppSet$")
	ensureMatches(t, copyCmds[1], "^mkdir -p .*/app/AppSet$")
	ensureMatches(t, copyCmds[2], "^unzip .*-d .*/app/AppSet .*/AppSet.zip$")
}

func testNoUpdatableJarsInBootImage(t *testing.T, errmsg, bp string, transformDexpreoptConfig func(*dexpreopt.GlobalConfig)) {
	t.Helper()

	bp = bp + `
		filegroup {
			name: "some-updatable-apex-file_contexts",
			srcs: [
				"system/sepolicy/apex/some-updatable-apex-file_contexts",
			],
		}

		filegroup {
			name: "some-non-updatable-apex-file_contexts",
			srcs: [
				"system/sepolicy/apex/some-non-updatable-apex-file_contexts",
			],
		}
	`
	bp += cc.GatherRequiredDepsForTest(android.Android)
	bp += java.GatherRequiredDepsForTest()
	bp += dexpreopt.BpToolModulesForTest()

	fs := map[string][]byte{
		"a.java":                             nil,
		"a.jar":                              nil,
		"build/make/target/product/security": nil,
		"apex_manifest.json":                 nil,
		"AndroidManifest.xml":                nil,
		"system/sepolicy/apex/some-updatable-apex-file_contexts":       nil,
		"system/sepolicy/apex/some-non-updatable-apex-file_contexts":   nil,
		"system/sepolicy/apex/com.android.art.something-file_contexts": nil,
		"framework/aidl/a.aidl": nil,
	}
	cc.GatherRequiredFilesForTest(fs)

	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("apex", BundleFactory)
	ctx.RegisterModuleType("apex_key", ApexKeyFactory)
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	ctx.PreArchMutators(android.RegisterBootJarMutators)
	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	cc.RegisterRequiredBuildComponentsForTest(ctx)
	java.RegisterJavaBuildComponents(ctx)
	java.RegisterSystemModulesBuildComponents(ctx)
	java.RegisterAppBuildComponents(ctx)
	java.RegisterDexpreoptBootJarsComponents(ctx)
	ctx.PostDepsMutators(android.RegisterOverridePostDepsMutators)
	ctx.PreDepsMutators(RegisterPreDepsMutators)
	ctx.PostDepsMutators(RegisterPostDepsMutators)

	config := android.TestArchConfig(buildDir, nil, bp, fs)
	ctx.Register(config)

	_ = dexpreopt.GlobalSoongConfigForTests(config)
	dexpreopt.RegisterToolModulesForTest(ctx)
	pathCtx := android.PathContextForTesting(config)
	dexpreoptConfig := dexpreopt.GlobalConfigForTests(pathCtx)
	transformDexpreoptConfig(dexpreoptConfig)
	dexpreopt.SetTestGlobalConfig(config, dexpreoptConfig)

	_, errs := ctx.ParseBlueprintsFiles("Android.bp")
	android.FailIfErrored(t, errs)

	_, errs = ctx.PrepareBuildActions(config)
	if errmsg == "" {
		android.FailIfErrored(t, errs)
	} else if len(errs) > 0 {
		android.FailIfNoMatchingErrors(t, errmsg, errs)
		return
	} else {
		t.Fatalf("missing expected error %q (0 errors are returned)", errmsg)
	}
}

func TestUpdatable_should_set_min_sdk_version(t *testing.T) {
	testApexError(t, `"myapex" .*: updatable: updatable APEXes should set min_sdk_version`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: true,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)
}

func TestNoUpdatableJarsInBootImage(t *testing.T) {
	bp := `
		java_library {
			name: "some-updatable-apex-lib",
			srcs: ["a.java"],
			sdk_version: "current",
			apex_available: [
				"some-updatable-apex",
			],
		}

		java_library {
			name: "some-non-updatable-apex-lib",
			srcs: ["a.java"],
			apex_available: [
				"some-non-updatable-apex",
			],
		}

		java_library {
			name: "some-platform-lib",
			srcs: ["a.java"],
			sdk_version: "current",
			installable: true,
		}

		java_library {
			name: "some-art-lib",
			srcs: ["a.java"],
			sdk_version: "current",
			apex_available: [
				"com.android.art.something",
			],
			hostdex: true,
		}

		apex {
			name: "some-updatable-apex",
			key: "some-updatable-apex.key",
			java_libs: ["some-updatable-apex-lib"],
			updatable: true,
			min_sdk_version: "current",
		}

		apex {
			name: "some-non-updatable-apex",
			key: "some-non-updatable-apex.key",
			java_libs: ["some-non-updatable-apex-lib"],
		}

		apex_key {
			name: "some-updatable-apex.key",
		}

		apex_key {
			name: "some-non-updatable-apex.key",
		}

		apex {
			name: "com.android.art.something",
			key: "com.android.art.something.key",
			java_libs: ["some-art-lib"],
			updatable: true,
			min_sdk_version: "current",
		}

		apex_key {
			name: "com.android.art.something.key",
		}
	`

	var error string
	var transform func(*dexpreopt.GlobalConfig)

	// updatable jar from ART apex in the ART boot image => ok
	transform = func(config *dexpreopt.GlobalConfig) {
		config.ArtApexJars = []string{"some-art-lib"}
	}
	testNoUpdatableJarsInBootImage(t, "", bp, transform)

	// updatable jar from ART apex in the framework boot image => error
	error = `module "some-art-lib" from updatable apexes \["com.android.art.something"\] is not allowed in the framework boot image`
	transform = func(config *dexpreopt.GlobalConfig) {
		config.BootJars = []string{"some-art-lib"}
	}
	testNoUpdatableJarsInBootImage(t, error, bp, transform)

	// updatable jar from some other apex in the ART boot image => error
	error = `module "some-updatable-apex-lib" from updatable apexes \["some-updatable-apex"\] is not allowed in the ART boot image`
	transform = func(config *dexpreopt.GlobalConfig) {
		config.ArtApexJars = []string{"some-updatable-apex-lib"}
	}
	testNoUpdatableJarsInBootImage(t, error, bp, transform)

	// non-updatable jar from some other apex in the ART boot image => error
	error = `module "some-non-updatable-apex-lib" is not allowed in the ART boot image`
	transform = func(config *dexpreopt.GlobalConfig) {
		config.ArtApexJars = []string{"some-non-updatable-apex-lib"}
	}
	testNoUpdatableJarsInBootImage(t, error, bp, transform)

	// updatable jar from some other apex in the framework boot image => error
	error = `module "some-updatable-apex-lib" from updatable apexes \["some-updatable-apex"\] is not allowed in the framework boot image`
	transform = func(config *dexpreopt.GlobalConfig) {
		config.BootJars = []string{"some-updatable-apex-lib"}
	}
	testNoUpdatableJarsInBootImage(t, error, bp, transform)

	// non-updatable jar from some other apex in the framework boot image => ok
	transform = func(config *dexpreopt.GlobalConfig) {
		config.BootJars = []string{"some-non-updatable-apex-lib"}
	}
	testNoUpdatableJarsInBootImage(t, "", bp, transform)

	// nonexistent jar in the ART boot image => error
	error = "failed to find a dex jar path for module 'nonexistent'"
	transform = func(config *dexpreopt.GlobalConfig) {
		config.ArtApexJars = []string{"nonexistent"}
	}
	testNoUpdatableJarsInBootImage(t, error, bp, transform)

	// nonexistent jar in the framework boot image => error
	error = "failed to find a dex jar path for module 'nonexistent'"
	transform = func(config *dexpreopt.GlobalConfig) {
		config.BootJars = []string{"nonexistent"}
	}
	testNoUpdatableJarsInBootImage(t, error, bp, transform)

	// platform jar in the ART boot image => error
	error = `module "some-platform-lib" is not allowed in the ART boot image`
	transform = func(config *dexpreopt.GlobalConfig) {
		config.ArtApexJars = []string{"some-platform-lib"}
	}
	testNoUpdatableJarsInBootImage(t, error, bp, transform)

	// platform jar in the framework boot image => ok
	transform = func(config *dexpreopt.GlobalConfig) {
		config.BootJars = []string{"some-platform-lib"}
	}
	testNoUpdatableJarsInBootImage(t, "", bp, transform)
}

func testApexPermittedPackagesRules(t *testing.T, errmsg, bp string, apexBootJars []string, rules []android.Rule) {
	t.Helper()
	android.ClearApexDependency()
	bp += `
	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}`
	fs := map[string][]byte{
		"lib1/src/A.java": nil,
		"lib2/src/B.java": nil,
		"system/sepolicy/apex/myapex-file_contexts": nil,
	}

	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("apex", BundleFactory)
	ctx.RegisterModuleType("apex_key", ApexKeyFactory)
	ctx.PreArchMutators(android.RegisterBootJarMutators)
	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	cc.RegisterRequiredBuildComponentsForTest(ctx)
	java.RegisterJavaBuildComponents(ctx)
	java.RegisterSystemModulesBuildComponents(ctx)
	java.RegisterDexpreoptBootJarsComponents(ctx)
	ctx.PostDepsMutators(android.RegisterOverridePostDepsMutators)
	ctx.PreDepsMutators(RegisterPreDepsMutators)
	ctx.PostDepsMutators(RegisterPostDepsMutators)
	ctx.PostDepsMutators(android.RegisterNeverallowMutator)

	config := android.TestArchConfig(buildDir, nil, bp, fs)
	android.SetTestNeverallowRules(config, rules)
	updatableBootJars := make([]string, 0, len(apexBootJars))
	for _, apexBootJar := range apexBootJars {
		updatableBootJars = append(updatableBootJars, "myapex:"+apexBootJar)
	}
	config.TestProductVariables.UpdatableBootJars = updatableBootJars

	ctx.Register(config)

	_, errs := ctx.ParseBlueprintsFiles("Android.bp")
	android.FailIfErrored(t, errs)

	_, errs = ctx.PrepareBuildActions(config)
	if errmsg == "" {
		android.FailIfErrored(t, errs)
	} else if len(errs) > 0 {
		android.FailIfNoMatchingErrors(t, errmsg, errs)
		return
	} else {
		t.Fatalf("missing expected error %q (0 errors are returned)", errmsg)
	}
}

func TestApexPermittedPackagesRules(t *testing.T) {
	testcases := []struct {
		name            string
		expectedError   string
		bp              string
		bootJars        []string
		modulesPackages map[string][]string
	}{

		{
			name:          "Non-Bootclasspath apex jar not satisfying allowed module packages.",
			expectedError: "",
			bp: `
				java_library {
					name: "bcp_lib1",
					srcs: ["lib1/src/*.java"],
					permitted_packages: ["foo.bar"],
					apex_available: ["myapex"],
					sdk_version: "none",
					system_modules: "none",
				}
				java_library {
					name: "nonbcp_lib2",
					srcs: ["lib2/src/*.java"],
					apex_available: ["myapex"],
					permitted_packages: ["a.b"],
					sdk_version: "none",
					system_modules: "none",
				}
				apex {
					name: "myapex",
					key: "myapex.key",
					java_libs: ["bcp_lib1", "nonbcp_lib2"],
				}`,
			bootJars: []string{"bcp_lib1"},
			modulesPackages: map[string][]string{
				"myapex": []string{
					"foo.bar",
				},
			},
		},
		{
			name:          "Bootclasspath apex jar not satisfying allowed module packages.",
			expectedError: `module "bcp_lib2" .* which is restricted because jars that are part of the myapex module may only allow these packages: foo.bar. Please jarjar or move code around.`,
			bp: `
				java_library {
					name: "bcp_lib1",
					srcs: ["lib1/src/*.java"],
					apex_available: ["myapex"],
					permitted_packages: ["foo.bar"],
					sdk_version: "none",
					system_modules: "none",
				}
				java_library {
					name: "bcp_lib2",
					srcs: ["lib2/src/*.java"],
					apex_available: ["myapex"],
					permitted_packages: ["foo.bar", "bar.baz"],
					sdk_version: "none",
					system_modules: "none",
				}
				apex {
					name: "myapex",
					key: "myapex.key",
					java_libs: ["bcp_lib1", "bcp_lib2"],
				}
			`,
			bootJars: []string{"bcp_lib1", "bcp_lib2"},
			modulesPackages: map[string][]string{
				"myapex": []string{
					"foo.bar",
				},
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			rules := createApexPermittedPackagesRules(tc.modulesPackages)
			testApexPermittedPackagesRules(t, tc.expectedError, tc.bp, tc.bootJars, rules)
		})
	}
}

func TestTestFor(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib", "myprivlib"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["1"],
			},
			apex_available: ["myapex"],
		}

		cc_library {
			name: "myprivlib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: ["myapex"],
		}


		cc_test {
			name: "mytest",
			gtest: false,
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			shared_libs: ["mylib", "myprivlib"],
			test_for: ["myapex"]
		}
	`)

	// the test 'mytest' is a test for the apex, therefore is linked to the
	// actual implementation of mylib instead of its stub.
	ldFlags := ctx.ModuleForTests("mytest", "android_arm64_armv8-a").Rule("ld").Args["libFlags"]
	ensureContains(t, ldFlags, "mylib/android_arm64_armv8-a_shared/mylib.so")
	ensureNotContains(t, ldFlags, "mylib/android_arm64_armv8-a_shared_1/mylib.so")
}

// TODO(jungjw): Move this to proptools
func intPtr(i int) *int {
	return &i
}

func TestApexSet(t *testing.T) {
	ctx, config := testApex(t, `
		apex_set {
			name: "myapex",
			set: "myapex.apks",
			filename: "foo_v2.apex",
			overrides: ["foo"],
		}
	`, func(fs map[string][]byte, config android.Config) {
		config.TestProductVariables.Platform_sdk_version = intPtr(30)
		config.Targets[android.Android] = []android.Target{
			{Os: android.Android, Arch: android.Arch{ArchType: android.Arm, ArchVariant: "armv7-a-neon", Abi: []string{"armeabi-v7a"}}},
			{Os: android.Android, Arch: android.Arch{ArchType: android.Arm64, ArchVariant: "armv8-a", Abi: []string{"arm64-v8a"}}},
		}
	})

	m := ctx.ModuleForTests("myapex", "android_common")

	// Check extract_apks tool parameters.
	extractedApex := m.Output(buildDir + "/.intermediates/myapex/android_common/foo_v2.apex")
	actual := extractedApex.Args["abis"]
	expected := "ARMEABI_V7A,ARM64_V8A"
	if actual != expected {
		t.Errorf("Unexpected abis parameter - expected %q vs actual %q", expected, actual)
	}
	actual = extractedApex.Args["sdk-version"]
	expected = "30"
	if actual != expected {
		t.Errorf("Unexpected abis parameter - expected %q vs actual %q", expected, actual)
	}

	a := m.Module().(*ApexSet)
	expectedOverrides := []string{"foo"}
	actualOverrides := android.AndroidMkEntriesForTest(t, config, "", a)[0].EntryMap["LOCAL_OVERRIDES_MODULES"]
	if !reflect.DeepEqual(actualOverrides, expectedOverrides) {
		t.Errorf("Incorrect LOCAL_OVERRIDES_MODULES - expected %q vs actual %q", expectedOverrides, actualOverrides)
	}
}

func TestApexKeysTxt(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		prebuilt_apex {
			name: "myapex",
			prefer: true,
			arch: {
				arm64: {
					src: "myapex-arm64.apex",
				},
				arm: {
					src: "myapex-arm.apex",
				},
			},
		}

		apex_set {
			name: "myapex_set",
			set: "myapex.apks",
			filename: "myapex_set.apex",
			overrides: ["myapex"],
		}
	`)

	apexKeysText := ctx.SingletonForTests("apex_keys_text")
	content := apexKeysText.MaybeDescription("apexkeys.txt").BuildParams.Args["content"]
	ensureContains(t, content, `name="myapex_set.apex" public_key="PRESIGNED" private_key="PRESIGNED" container_certificate="PRESIGNED" container_private_key="PRESIGNED" partition="system"`)
	ensureContains(t, content, `name="myapex.apex" public_key="PRESIGNED" private_key="PRESIGNED" container_certificate="PRESIGNED" container_private_key="PRESIGNED" partition="system"`)
}

func TestAllowedFiles(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apps: ["app"],
			allowed_files: "allowed.txt",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_app {
			name: "app",
			srcs: ["foo/bar/MyClass.java"],
			package_name: "foo",
			sdk_version: "none",
			system_modules: "none",
			apex_available: [ "myapex" ],
		}
	`, withFiles(map[string][]byte{
		"sub/Android.bp": []byte(`
			override_apex {
				name: "override_myapex",
				base: "myapex",
				apps: ["override_app"],
				allowed_files: ":allowed",
			}
			// Overridable "path" property should be referenced indirectly
			filegroup {
				name: "allowed",
				srcs: ["allowed.txt"],
			}
			override_android_app {
				name: "override_app",
				base: "app",
				package_name: "bar",
			}
			`),
	}))

	rule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("diffApexContentRule")
	if expected, actual := "allowed.txt", rule.Args["allowed_files_file"]; expected != actual {
		t.Errorf("allowed_files_file: expected %q but got %q", expected, actual)
	}

	rule2 := ctx.ModuleForTests("myapex", "android_common_override_myapex_myapex_image").Rule("diffApexContentRule")
	if expected, actual := "sub/allowed.txt", rule2.Args["allowed_files_file"]; expected != actual {
		t.Errorf("allowed_files_file: expected %q but got %q", expected, actual)
	}
}

func TestMain(m *testing.M) {
	run := func() int {
		setUp()
		defer tearDown()

		return m.Run()
	}

	os.Exit(run())
}
