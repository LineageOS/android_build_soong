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
	"sort"
	"strings"
	"testing"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/java"
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
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
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

func withBinder32bit(fs map[string][]byte, config android.Config) {
	config.TestProductVariables.Binder32bit = proptools.BoolPtr(true)
}

func withUnbundledBuild(fs map[string][]byte, config android.Config) {
	config.TestProductVariables.Unbundled_build = proptools.BoolPtr(true)
}

func testApexContext(t *testing.T, bp string, handlers ...testCustomizer) (*android.TestContext, android.Config) {
	android.ClearApexDependency()

	bp = bp + `
		toolchain_library {
			name: "libcompiler_rt-extras",
			src: "",
			vendor_available: true,
			recovery_available: true,
		}

		toolchain_library {
			name: "libatomic",
			src: "",
			vendor_available: true,
			recovery_available: true,
			native_bridge_supported: true,
		}

		toolchain_library {
			name: "libgcc",
			src: "",
			vendor_available: true,
			recovery_available: true,
		}

		toolchain_library {
			name: "libgcc_stripped",
			src: "",
			vendor_available: true,
			recovery_available: true,
			native_bridge_supported: true,
		}

		toolchain_library {
			name: "libclang_rt.builtins-aarch64-android",
			src: "",
			vendor_available: true,
			recovery_available: true,
			native_bridge_supported: true,
		}

		toolchain_library {
			name: "libclang_rt.builtins-arm-android",
			src: "",
			vendor_available: true,
			recovery_available: true,
			native_bridge_supported: true,
		}

		toolchain_library {
			name: "libclang_rt.builtins-x86_64-android",
			src: "",
			vendor_available: true,
			recovery_available: true,
			native_bridge_supported: true,
		}

		toolchain_library {
			name: "libclang_rt.builtins-i686-android",
			src: "",
			vendor_available: true,
			recovery_available: true,
			native_bridge_supported: true,
		}

		cc_object {
			name: "crtbegin_so",
			stl: "none",
			vendor_available: true,
			recovery_available: true,
			native_bridge_supported: true,
		}

		cc_object {
			name: "crtend_so",
			stl: "none",
			vendor_available: true,
			recovery_available: true,
			native_bridge_supported: true,
		}

		cc_object {
			name: "crtbegin_static",
			stl: "none",
		}

		cc_object {
			name: "crtend_android",
			stl: "none",
		}

		llndk_library {
			name: "libc",
			symbol_file: "",
			native_bridge_supported: true,
		}

		llndk_library {
			name: "libm",
			symbol_file: "",
			native_bridge_supported: true,
		}

		llndk_library {
			name: "libdl",
			symbol_file: "",
			native_bridge_supported: true,
		}

		filegroup {
			name: "myapex-file_contexts",
			srcs: [
				"system/sepolicy/apex/myapex-file_contexts",
			],
		}
	`

	bp = bp + java.GatherRequiredDepsForTest()

	fs := map[string][]byte{
		"a.java":                                              nil,
		"PrebuiltAppFoo.apk":                                  nil,
		"PrebuiltAppFooPriv.apk":                              nil,
		"build/make/target/product/security":                  nil,
		"apex_manifest.json":                                  nil,
		"AndroidManifest.xml":                                 nil,
		"system/sepolicy/apex/myapex-file_contexts":           nil,
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
		"testkey2.avbpubkey":                         nil,
		"testkey2.pem":                               nil,
		"myapex-arm64.apex":                          nil,
		"myapex-arm.apex":                            nil,
		"frameworks/base/api/current.txt":            nil,
		"framework/aidl/a.aidl":                      nil,
		"build/make/core/proguard.flags":             nil,
		"build/make/core/proguard_basic_keeps.flags": nil,
		"dummy.txt":                                  nil,
	}

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
	ctx.RegisterModuleType("apex", BundleFactory)
	ctx.RegisterModuleType("apex_test", testApexBundleFactory)
	ctx.RegisterModuleType("apex_vndk", vndkApexBundleFactory)
	ctx.RegisterModuleType("apex_key", ApexKeyFactory)
	ctx.RegisterModuleType("apex_defaults", defaultsFactory)
	ctx.RegisterModuleType("prebuilt_apex", PrebuiltFactory)
	ctx.RegisterModuleType("override_apex", overrideApexFactory)

	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	ctx.PostDepsMutators(android.RegisterOverridePostDepsMutators)

	cc.RegisterRequiredBuildComponentsForTest(ctx)
	ctx.RegisterModuleType("cc_test", cc.TestFactory)
	ctx.RegisterModuleType("vndk_prebuilt_shared", cc.VndkPrebuiltSharedFactory)
	ctx.RegisterModuleType("vndk_libraries_txt", cc.VndkLibrariesTxtFactory)
	ctx.RegisterModuleType("prebuilt_etc", android.PrebuiltEtcFactory)
	ctx.RegisterModuleType("platform_compat_config", java.PlatformCompatConfigFactory)
	ctx.RegisterModuleType("sh_binary", android.ShBinaryFactory)
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	java.RegisterJavaBuildComponents(ctx)
	java.RegisterSystemModulesBuildComponents(ctx)
	java.RegisterAppBuildComponents(ctx)
	ctx.RegisterModuleType("java_sdk_library", java.SdkLibraryFactory)

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
	os.RemoveAll(buildDir)
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
	ctx, _ := testApex(t, `
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
			java_libs: ["myjar"],
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

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			notice: "custom_notice",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		java_library {
			name: "myjar",
			srcs: ["foo/bar/MyClass.java"],
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

	optFlags := apexRule.Args["opt_flags"]
	ensureContains(t, optFlags, "--pubkey vendor/foo/devkeys/testkey.avbpubkey")
	// Ensure that the NOTICE output is being packaged as an asset.
	ensureContains(t, optFlags, "--assets_dir "+buildDir+"/.intermediates/myapex/android_common_myapex_image/NOTICE")

	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that main rule creates an output
	ensureContains(t, apexRule.Output.String(), "myapex.apex.unsigned")

	// Ensure that apex variant is created for the direct dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_shared_myapex")
	ensureListContains(t, ctx.ModuleVariantsForTests("myjar"), "android_common_myapex")

	// Ensure that apex variant is created for the indirect dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_shared_myapex")
	ensureListContains(t, ctx.ModuleVariantsForTests("myotherjar"), "android_common_myapex")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")
	ensureContains(t, copyCmds, "image.apex/lib64/mylib2.so")
	ensureContains(t, copyCmds, "image.apex/javalib/myjar.jar")
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
	if len(noticeInputs) != 2 {
		t.Errorf("number of input notice files: expected = 2, actual = %q", len(noticeInputs))
	}
	ensureListContains(t, noticeInputs, "NOTICE")
	ensureListContains(t, noticeInputs, "custom_notice")

	depsInfo := strings.Split(ctx.ModuleForTests("myapex", "android_common_myapex_image").Output("myapex-deps-info.txt").Args["content"], "\\n")
	ensureListContains(t, depsInfo, "myjar <- myapex")
	ensureListContains(t, depsInfo, "mylib <- myapex")
	ensureListContains(t, depsInfo, "mylib2 <- mylib")
	ensureListContains(t, depsInfo, "myotherjar <- myjar")
	ensureListContains(t, depsInfo, "mysharedjar (external) <- myjar")
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
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_shared_myapex")

	// Ensure that APEX variant is created for the indirect dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_shared_myapex")

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

	mylibLdFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_shared_myapex").Rule("ld").Args["libFlags"]

	// Ensure that mylib is linking with the latest version of stubs for mylib2
	ensureContains(t, mylibLdFlags, "mylib2/android_arm64_armv8-a_shared_3/mylib2.so")
	// ... and not linking to the non-stub (impl) variant of mylib2
	ensureNotContains(t, mylibLdFlags, "mylib2/android_arm64_armv8-a_shared/mylib2.so")

	// Ensure that mylib is linking with the non-stub (impl) of mylib3 (because mylib3 is in the same apex)
	ensureContains(t, mylibLdFlags, "mylib3/android_arm64_armv8-a_shared_myapex/mylib3.so")
	// .. and not linking to the stubs variant of mylib3
	ensureNotContains(t, mylibLdFlags, "mylib3/android_arm64_armv8-a_shared_12_myapex/mylib3.so")

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

	mylibLdFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_shared_myapex2").Rule("ld").Args["libFlags"]

	// Ensure that mylib is linking with version 10 of libfoo
	ensureContains(t, mylibLdFlags, "libfoo/android_arm64_armv8-a_shared_10/libfoo.so")
	// ... and not linking to the non-stub (impl) variant of libfoo
	ensureNotContains(t, mylibLdFlags, "libfoo/android_arm64_armv8-a_shared/libfoo.so")

	libFooStubsLdFlags := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared_10").Rule("ld").Args["libFlags"]

	// Ensure that libfoo stubs is not linking to libbar (since it is a stubs)
	ensureNotContains(t, libFooStubsLdFlags, "libbar.so")

	depsInfo := strings.Split(ctx.ModuleForTests("myapex2", "android_common_myapex2_image").Output("myapex2-deps-info.txt").Args["content"], "\\n")

	ensureListContains(t, depsInfo, "mylib <- myapex2")
	ensureListContains(t, depsInfo, "libbaz <- mylib")
	ensureListContains(t, depsInfo, "libfoo (external) <- mylib")
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
			apex_available: [ "myapex" ],
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

func TestApexDependencyToLLNDK(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			use_vendor: true,
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
		}

		llndk_library {
			name: "libbar",
			symbol_file: "",
		}
	`, func(fs map[string][]byte, config android.Config) {
		setUseVendorWhitelistForTest(config, []string{"myapex"})
	})

	apexRule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that LLNDK dep is not included
	ensureNotContains(t, copyCmds, "image.apex/lib64/libbar.so")

	apexManifestRule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("apexManifestRule")
	ensureListEmpty(t, names(apexManifestRule.Args["provideNativeLibs"]))

	// Ensure that LLNDK dep is required
	ensureListContains(t, names(apexManifestRule.Args["requireNativeLibs"]), "libbar.so")

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
			name: "libc",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["27", "28", "29"],
			},
		}

		cc_library {
			name: "libm",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["27", "28", "29"],
			},
			apex_available: [
				"//apex_available:platform",
				"myapex"
			],
		}

		cc_library {
			name: "libdl",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["27", "28", "29"],
			},
			apex_available: [
				"//apex_available:platform",
				"myapex"
			],
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

	mylibLdFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_shared_myapex").Rule("ld").Args["libFlags"]
	mylibCFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_static_myapex").Rule("cc").Args["cFlags"]
	mylibSharedCFlags := ctx.ModuleForTests("mylib_shared", "android_arm64_armv8-a_shared_myapex").Rule("cc").Args["cFlags"]

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
	ensureContains(t, mylibLdFlags, "libm/android_arm64_armv8-a_shared_myapex/libm.so")
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
	ensureNotContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_shared_myapex/libdl.so")
	// ... Cflags from stub is correctly exported to mylib
	ensureContains(t, mylibCFlags, "__LIBDL_API__=27")
	ensureContains(t, mylibSharedCFlags, "__LIBDL_API__=27")

	// Ensure that libBootstrap is depending on the platform variant of bionic libs
	libFlags := ctx.ModuleForTests("libBootstrap", "android_arm64_armv8-a_shared").Rule("ld").Args["libFlags"]
	ensureContains(t, libFlags, "libc/android_arm64_armv8-a_shared/libc.so")
	ensureContains(t, libFlags, "libm/android_arm64_armv8-a_shared/libm.so")
	ensureContains(t, libFlags, "libdl/android_arm64_armv8-a_shared/libdl.so")
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
		setUseVendorWhitelistForTest(config, []string{"myapex"})
	})

	inputsList := []string{}
	for _, i := range ctx.ModuleForTests("myapex", "android_common_myapex_image").Module().BuildParamsForTests() {
		for _, implicit := range i.Implicits {
			inputsList = append(inputsList, implicit.String())
		}
	}
	inputsString := strings.Join(inputsList, " ")

	// ensure that the apex includes vendor variants of the direct and indirect deps
	ensureContains(t, inputsString, "android_vendor.VER_arm64_armv8-a_shared_myapex/mylib.so")
	ensureContains(t, inputsString, "android_vendor.VER_arm64_armv8-a_shared_myapex/mylib2.so")

	// ensure that the apex does not include core variants
	ensureNotContains(t, inputsString, "android_arm64_armv8-a_shared_myapex/mylib.so")
	ensureNotContains(t, inputsString, "android_arm64_armv8-a_shared_myapex/mylib2.so")
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
		setUseVendorWhitelistForTest(config, []string{""})
	})
	// no error with whitelist
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
		setUseVendorWhitelistForTest(config, []string{"myapex"})
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
			native_shared_libs: ["mylib"],
		}

		apex {
			name: "otherapex",
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
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
				"otherapex",
			],
		}
	`)

	// non-APEX variant does not have __ANDROID_APEX(_NAME)__ defined
	mylibCFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_static").Rule("cc").Args["cFlags"]
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX__")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX_MYAPEX__")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX_OTHERAPEX__")

	// APEX variant has __ANDROID_APEX(_NAME)__ defined
	mylibCFlags = ctx.ModuleForTests("mylib", "android_arm64_armv8-a_static_myapex").Rule("cc").Args["cFlags"]
	ensureContains(t, mylibCFlags, "-D__ANDROID_APEX__")
	ensureContains(t, mylibCFlags, "-D__ANDROID_APEX_MYAPEX__")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX_OTHERAPEX__")

	// APEX variant has __ANDROID_APEX(_NAME)__ defined
	mylibCFlags = ctx.ModuleForTests("mylib", "android_arm64_armv8-a_static_otherapex").Rule("cc").Args["cFlags"]
	ensureContains(t, mylibCFlags, "-D__ANDROID_APEX__")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX_MYAPEX__")
	ensureContains(t, mylibCFlags, "-D__ANDROID_APEX_OTHERAPEX__")
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
		for _, expected := range files {
			if matched, _ := path.Match(expected, file.path); matched {
				filesMatched[expected] = true
				return
			}
		}
		surplus = append(surplus, file.path)
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
		"lib64/libvndk.so",
		"lib64/libvndksp.so",
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
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common"), "android_arm64_armv8-a_shared_myapex")

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
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common_test"), "android_arm64_armv8-a_shared_myapex")

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
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_shared_myapex")
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common"), "android_arm64_armv8-a_shared_myapex")
	ensureListNotContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_shared_myapex")

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

	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_shared_myapex")
	ensureListContains(t, ctx.ModuleVariantsForTests("libcommon"), "android_arm64_armv8-a_shared_commonapex")
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
		setUseVendorWhitelistForTest(config, []string{"myapex"})
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
			sdk_version: "none",
			system_modules: "none",
			jni_libs: ["libjni"],
			apex_available: [ "myapex" ],
		}

		android_app {
			name: "AppFooPriv",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			privileged: true,
			apex_available: [ "myapex" ],
		}

		cc_library_shared {
			name: "libjni",
			srcs: ["mylib.cpp"],
			stl: "none",
			system_shared_libs: [],
			apex_available: [ "myapex" ],
		}
	`)

	module := ctx.ModuleForTests("myapex", "android_common_myapex_image")
	apexRule := module.Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	ensureContains(t, copyCmds, "image.apex/app/AppFoo/AppFoo.apk")
	ensureContains(t, copyCmds, "image.apex/priv-app/AppFooPriv/AppFooPriv.apk")

	// JNI libraries are embedded inside APK
	appZipRule := ctx.ModuleForTests("AppFoo", "android_common_myapex").Description("zip jni lib")
	libjniOutput := ctx.ModuleForTests("libjni", "android_arm64_armv8-a_shared_myapex").Module().(*cc.Module).OutputFile()
	ensureListContains(t, appZipRule.Implicits.Strings(), libjniOutput.String())
	// ... uncompressed
	if args := appZipRule.Args["jarArgs"]; !strings.Contains(args, "-L 0") {
		t.Errorf("jni lib is not uncompressed for AppFoo")
	}
	// ... and not directly inside the APEX
	ensureNotContains(t, copyCmds, "image.apex/lib64/libjni.so")
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
		}
	`)

	module := ctx.ModuleForTests("myapex", "android_common_myapex_image")
	apexRule := module.Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	ensureContains(t, copyCmds, "image.apex/app/AppFooPrebuilt/AppFooPrebuilt.apk")
	ensureContains(t, copyCmds, "image.apex/priv-app/AppFooPrivPrebuilt/AppFooPrivPrebuilt.apk")
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
	testApexError(t, `"myapex" .*: requires "libfoo" that is not available for the APEX`, `
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

func TestApexAvailable(t *testing.T) {
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

	// libbar is an indirect dep
	testApexError(t, "requires \"libbar\" that is not available for the APEX", `
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
		shared_libs: ["libbar"],
		system_shared_libs: [],
		apex_available: ["myapex", "otherapex"],
	}

	cc_library {
		name: "libbar",
		stl: "none",
		system_shared_libs: [],
		apex_available: ["otherapex"],
	}`)

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

	ctx, _ := testApex(t, `
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
		apex_available: ["myapex"],
	}

	cc_library {
		name: "libbar",
		stl: "none",
		system_shared_libs: [],
		apex_available: ["//apex_available:anyapex"],
	}`)

	// check that libfoo and libbar are created only for myapex, but not for the platform
	// TODO(jiyong) the checks for the platform variant are removed because we now create
	// the platform variant regardless of the apex_availability. Instead, we will make sure that
	// the platform variants are not used from other platform modules. When that is done,
	// these checks will be replaced by expecting a specific error message that will be
	// emitted when the platform variant is used.
	//	ensureListContains(t, ctx.ModuleVariantsForTests("libfoo"), "android_arm64_armv8-a_shared_myapex")
	//	ensureListNotContains(t, ctx.ModuleVariantsForTests("libfoo"), "android_arm64_armv8-a_shared")
	//	ensureListContains(t, ctx.ModuleVariantsForTests("libbar"), "android_arm64_armv8-a_shared_myapex")
	//	ensureListNotContains(t, ctx.ModuleVariantsForTests("libbar"), "android_arm64_armv8-a_shared")

	ctx, _ = testApex(t, `
	apex {
		name: "myapex",
		key: "myapex.key",
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
		apex_available: ["//apex_available:platform"],
	}`)

	// check that libfoo is created only for the platform
	ensureListNotContains(t, ctx.ModuleVariantsForTests("libfoo"), "android_arm64_armv8-a_shared_myapex")
	ensureListContains(t, ctx.ModuleVariantsForTests("libfoo"), "android_arm64_armv8-a_shared")

	ctx, _ = testApex(t, `
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

	// shared variant of libfoo is only available to myapex
	// TODO(jiyong) the checks for the platform variant are removed because we now create
	// the platform variant regardless of the apex_availability. Instead, we will make sure that
	// the platform variants are not used from other platform modules. When that is done,
	// these checks will be replaced by expecting a specific error message that will be
	// emitted when the platform variant is used.
	//	ensureListContains(t, ctx.ModuleVariantsForTests("libfoo"), "android_arm64_armv8-a_shared_myapex")
	//	ensureListNotContains(t, ctx.ModuleVariantsForTests("libfoo"), "android_arm64_armv8-a_shared")
	//	// but the static variant is available to both myapex and the platform
	//	ensureListContains(t, ctx.ModuleVariantsForTests("libfoo"), "android_arm64_armv8-a_static_myapex")
	//	ensureListContains(t, ctx.ModuleVariantsForTests("libfoo"), "android_arm64_armv8-a_static")
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
	`)

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
	ensureContains(t, copyCmds, "image.apex/app/app/override_app.apk")

	apexBundle := module.Module().(*apexBundle)
	name := apexBundle.Name()
	if name != "override_myapex" {
		t.Errorf("name should be \"override_myapex\", but was %q", name)
	}

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
			legacy_android10_support: true,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)

	module := ctx.ModuleForTests("myapex", "android_common_myapex_image")
	args := module.Rule("apexRule").Args
	ensureContains(t, args["opt_flags"], "--manifest_json "+module.Output("apex_manifest.json").Output.String())
	ensureNotContains(t, args["opt_flags"], "--no_hashtree")
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
	`, withFiles(map[string][]byte{
		"api/current.txt":        nil,
		"api/removed.txt":        nil,
		"api/system-current.txt": nil,
		"api/system-removed.txt": nil,
		"api/test-current.txt":   nil,
		"api/test-removed.txt":   nil,
	}))

	// java_sdk_library installs both impl jar and permission XML
	ensureExactContents(t, ctx, "myapex", "android_common_myapex_image", []string{
		"javalib/foo.jar",
		"etc/permissions/foo.xml",
	})
	// Permission XML should point to the activated path of impl jar of java_sdk_library
	sdkLibrary := ctx.ModuleForTests("foo", "android_common_myapex").Module().(*java.SdkLibrary)
	xml := sdkLibrary.XmlPermissionsFileContent()
	ensureContains(t, xml, `<library name="foo" file="/apex/myapex/javalib/foo.jar"`)
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

	ctx, _ := testApex(t, bp, withUnbundledBuild)
	files := getFiles(t, ctx, "myapex", "android_common_myapex_image")
	ensureRealfileExists(t, files, "javalib/myjar.jar")
	ensureRealfileExists(t, files, "lib64/mylib.so")
	ensureRealfileExists(t, files, "lib64/myotherlib.so")

	ctx, _ = testApex(t, bp)
	files = getFiles(t, ctx, "myapex", "android_common_myapex_image")
	ensureRealfileExists(t, files, "javalib/myjar.jar")
	ensureRealfileExists(t, files, "lib64/mylib.so")
	ensureSymlinkExists(t, files, "lib64/myotherlib.so") // this is symlink
}

func TestMain(m *testing.M) {
	run := func() int {
		setUp()
		defer tearDown()

		return m.Run()
	}

	os.Exit(run())
}
