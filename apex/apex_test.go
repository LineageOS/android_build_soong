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

func testApexContext(t *testing.T, bp string, handlers ...testCustomizer) (*android.TestContext, android.Config) {
	config := android.TestArchConfig(buildDir, nil)
	config.TestProductVariables.DeviceVndkVersion = proptools.StringPtr("current")
	config.TestProductVariables.DefaultAppCertificate = proptools.StringPtr("vendor/foo/devkeys/test")
	config.TestProductVariables.CertificateOverrides = []string{"myapex_keytest:myapex.certificate.override"}
	config.TestProductVariables.Platform_sdk_codename = proptools.StringPtr("Q")
	config.TestProductVariables.Platform_sdk_final = proptools.BoolPtr(false)
	config.TestProductVariables.Platform_vndk_version = proptools.StringPtr("VER")

	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("apex", android.ModuleFactoryAdaptor(BundleFactory))
	ctx.RegisterModuleType("apex_test", android.ModuleFactoryAdaptor(testApexBundleFactory))
	ctx.RegisterModuleType("apex_vndk", android.ModuleFactoryAdaptor(vndkApexBundleFactory))
	ctx.RegisterModuleType("apex_key", android.ModuleFactoryAdaptor(ApexKeyFactory))
	ctx.RegisterModuleType("apex_defaults", android.ModuleFactoryAdaptor(defaultsFactory))
	ctx.RegisterModuleType("prebuilt_apex", android.ModuleFactoryAdaptor(PrebuiltFactory))

	ctx.RegisterModuleType("cc_library", android.ModuleFactoryAdaptor(cc.LibraryFactory))
	ctx.RegisterModuleType("cc_library_shared", android.ModuleFactoryAdaptor(cc.LibrarySharedFactory))
	ctx.RegisterModuleType("cc_library_headers", android.ModuleFactoryAdaptor(cc.LibraryHeaderFactory))
	ctx.RegisterModuleType("cc_prebuilt_library_shared", android.ModuleFactoryAdaptor(cc.PrebuiltSharedLibraryFactory))
	ctx.RegisterModuleType("cc_prebuilt_library_static", android.ModuleFactoryAdaptor(cc.PrebuiltStaticLibraryFactory))
	ctx.RegisterModuleType("cc_binary", android.ModuleFactoryAdaptor(cc.BinaryFactory))
	ctx.RegisterModuleType("cc_object", android.ModuleFactoryAdaptor(cc.ObjectFactory))
	ctx.RegisterModuleType("cc_test", android.ModuleFactoryAdaptor(cc.TestFactory))
	ctx.RegisterModuleType("llndk_library", android.ModuleFactoryAdaptor(cc.LlndkLibraryFactory))
	ctx.RegisterModuleType("vndk_prebuilt_shared", android.ModuleFactoryAdaptor(cc.VndkPrebuiltSharedFactory))
	ctx.RegisterModuleType("vndk_libraries_txt", android.ModuleFactoryAdaptor(cc.VndkLibrariesTxtFactory))
	ctx.RegisterModuleType("toolchain_library", android.ModuleFactoryAdaptor(cc.ToolchainLibraryFactory))
	ctx.RegisterModuleType("prebuilt_etc", android.ModuleFactoryAdaptor(android.PrebuiltEtcFactory))
	ctx.RegisterModuleType("sh_binary", android.ModuleFactoryAdaptor(android.ShBinaryFactory))
	ctx.RegisterModuleType("android_app_certificate", android.ModuleFactoryAdaptor(java.AndroidAppCertificateFactory))
	ctx.RegisterModuleType("filegroup", android.ModuleFactoryAdaptor(android.FileGroupFactory))
	ctx.RegisterModuleType("java_library", android.ModuleFactoryAdaptor(java.LibraryFactory))
	ctx.RegisterModuleType("java_import", android.ModuleFactoryAdaptor(java.ImportFactory))
	ctx.RegisterModuleType("java_system_modules", android.ModuleFactoryAdaptor(java.SystemModulesFactory))
	ctx.RegisterModuleType("android_app", android.ModuleFactoryAdaptor(java.AndroidAppFactory))
	ctx.RegisterModuleType("android_app_import", android.ModuleFactoryAdaptor(java.AndroidAppImportFactory))

	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	ctx.PreArchMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("prebuilts", android.PrebuiltMutator).Parallel()
	})
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("image", cc.ImageMutator).Parallel()
		ctx.BottomUp("link", cc.LinkageMutator).Parallel()
		ctx.BottomUp("vndk", cc.VndkMutator).Parallel()
		ctx.BottomUp("test_per_src", cc.TestPerSrcMutator).Parallel()
		ctx.BottomUp("version", cc.VersionMutator).Parallel()
		ctx.BottomUp("begin", cc.BeginMutator).Parallel()
	})
	ctx.PreDepsMutators(RegisterPreDepsMutators)
	ctx.PostDepsMutators(RegisterPostDepsMutators)
	ctx.PostDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("prebuilt_select", android.PrebuiltSelectModuleMutator).Parallel()
		ctx.BottomUp("prebuilt_postdeps", android.PrebuiltPostDepsMutator).Parallel()
	})

	ctx.Register()

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
	`
	bp = bp + java.GatherRequiredDepsForTest()

	fs := map[string][]byte{
		"Android.bp":                                []byte(bp),
		"a.java":                                    nil,
		"PrebuiltAppFoo.apk":                        nil,
		"PrebuiltAppFooPriv.apk":                    nil,
		"build/make/target/product/security":        nil,
		"apex_manifest.json":                        nil,
		"AndroidManifest.xml":                       nil,
		"system/sepolicy/apex/myapex-file_contexts": nil,
		"system/sepolicy/apex/myapex_keytest-file_contexts": nil,
		"system/sepolicy/apex/otherapex-file_contexts":      nil,
		"system/sepolicy/apex/commonapex-file_contexts":     nil,
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
		handler(fs, config)
	}

	ctx.MockFileSystem(fs)

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
			java_libs: ["myjar", "myprebuiltjar"],
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
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			notice: "custom_notice",
		}

		java_library {
			name: "myjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			compile_dex: true,
			static_libs: ["myotherjar"],
		}

		java_library {
			name: "myotherjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			compile_dex: true,
		}

		java_import {
			name: "myprebuiltjar",
			jars: ["prebuilt.jar"],
			installable: true,
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
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_core_shared_myapex")
	ensureListContains(t, ctx.ModuleVariantsForTests("myjar"), "android_common_myapex")
	ensureListContains(t, ctx.ModuleVariantsForTests("myprebuiltjar"), "android_common_myapex")

	// Ensure that apex variant is created for the indirect dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_core_shared_myapex")
	ensureListContains(t, ctx.ModuleVariantsForTests("myotherjar"), "android_common_myapex")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")
	ensureContains(t, copyCmds, "image.apex/lib64/mylib2.so")
	ensureContains(t, copyCmds, "image.apex/javalib/myjar.jar")
	ensureContains(t, copyCmds, "image.apex/javalib/myprebuiltjar.jar")
	// .. but not for java libs
	ensureNotContains(t, copyCmds, "image.apex/javalib/myotherjar.jar")

	// Ensure that the platform variant ends with _core_shared or _common
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_core_shared")
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_core_shared")
	ensureListContains(t, ctx.ModuleVariantsForTests("myjar"), "android_common")
	ensureListContains(t, ctx.ModuleVariantsForTests("myotherjar"), "android_common")
	ensureListContains(t, ctx.ModuleVariantsForTests("myprebuiltjar"), "android_common")

	// Ensure that all symlinks are present.
	found_foo_link_64 := false
	found_foo := false
	for _, cmd := range strings.Split(copyCmds, " && ") {
		if strings.HasPrefix(cmd, "ln -s foo64") {
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
	module.Output("apex_manifest.pb")
	module.Output("apex_manifest.json")
	module.Output("apex_manifest_full.json")
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
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
		}
	`)

	zipApexRule := ctx.ModuleForTests("myapex", "android_common_myapex_zip").Rule("zipApexRule")
	copyCmds := zipApexRule.Args["copy_commands"]

	// Ensure that main rule creates an output
	ensureContains(t, zipApexRule.Output.String(), "myapex.zipapex.unsigned")

	// Ensure that APEX variant is created for the direct dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_core_shared_myapex")

	// Ensure that APEX variant is created for the indirect dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_core_shared_myapex")

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
		}

		cc_library {
			name: "mylib4",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
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

	mylibLdFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_core_shared_myapex").Rule("ld").Args["libFlags"]

	// Ensure that mylib is linking with the latest version of stubs for mylib2
	ensureContains(t, mylibLdFlags, "mylib2/android_arm64_armv8-a_core_shared_3_myapex/mylib2.so")
	// ... and not linking to the non-stub (impl) variant of mylib2
	ensureNotContains(t, mylibLdFlags, "mylib2/android_arm64_armv8-a_core_shared_myapex/mylib2.so")

	// Ensure that mylib is linking with the non-stub (impl) of mylib3 (because mylib3 is in the same apex)
	ensureContains(t, mylibLdFlags, "mylib3/android_arm64_armv8-a_core_shared_myapex/mylib3.so")
	// .. and not linking to the stubs variant of mylib3
	ensureNotContains(t, mylibLdFlags, "mylib3/android_arm64_armv8-a_core_shared_12_myapex/mylib3.so")

	// Ensure that stubs libs are built without -include flags
	mylib2Cflags := ctx.ModuleForTests("mylib2", "android_arm64_armv8-a_core_static_myapex").Rule("cc").Args["cFlags"]
	ensureNotContains(t, mylib2Cflags, "-include ")

	// Ensure that genstub is invoked with --apex
	ensureContains(t, "--apex", ctx.ModuleForTests("mylib2", "android_arm64_armv8-a_core_static_3_myapex").Rule("genStubSrc").Args["flags"])
}

func TestApexWithExplicitStubsDependency(t *testing.T) {
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
			shared_libs: ["libfoo#10"],
			system_shared_libs: [],
			stl: "none",
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

	`)

	apexRule := ctx.ModuleForTests("myapex", "android_common_myapex_image").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that direct non-stubs dep is always included
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")

	// Ensure that indirect stubs dep is not included
	ensureNotContains(t, copyCmds, "image.apex/lib64/libfoo.so")

	// Ensure that dependency of stubs is not included
	ensureNotContains(t, copyCmds, "image.apex/lib64/libbar.so")

	mylibLdFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_core_shared_myapex").Rule("ld").Args["libFlags"]

	// Ensure that mylib is linking with version 10 of libfoo
	ensureContains(t, mylibLdFlags, "libfoo/android_arm64_armv8-a_core_shared_10_myapex/libfoo.so")
	// ... and not linking to the non-stub (impl) variant of libfoo
	ensureNotContains(t, mylibLdFlags, "libfoo/android_arm64_armv8-a_core_shared_myapex/libfoo.so")

	libFooStubsLdFlags := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_core_shared_10_myapex").Rule("ld").Args["libFlags"]

	// Ensure that libfoo stubs is not linking to libbar (since it is a stubs)
	ensureNotContains(t, libFooStubsLdFlags, "libbar.so")
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
		}

		cc_library_shared {
			name: "mylib_shared",
			srcs: ["mylib.cpp"],
			shared_libs: ["libdl#27"],
			stl: "none",
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

	mylibLdFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_core_shared_myapex").Rule("ld").Args["libFlags"]
	mylibCFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_core_static_myapex").Rule("cc").Args["cFlags"]
	mylibSharedCFlags := ctx.ModuleForTests("mylib_shared", "android_arm64_armv8-a_core_shared_myapex").Rule("cc").Args["cFlags"]

	// For dependency to libc
	// Ensure that mylib is linking with the latest version of stubs
	ensureContains(t, mylibLdFlags, "libc/android_arm64_armv8-a_core_shared_29_myapex/libc.so")
	// ... and not linking to the non-stub (impl) variant
	ensureNotContains(t, mylibLdFlags, "libc/android_arm64_armv8-a_core_shared_myapex/libc.so")
	// ... Cflags from stub is correctly exported to mylib
	ensureContains(t, mylibCFlags, "__LIBC_API__=29")
	ensureContains(t, mylibSharedCFlags, "__LIBC_API__=29")

	// For dependency to libm
	// Ensure that mylib is linking with the non-stub (impl) variant
	ensureContains(t, mylibLdFlags, "libm/android_arm64_armv8-a_core_shared_myapex/libm.so")
	// ... and not linking to the stub variant
	ensureNotContains(t, mylibLdFlags, "libm/android_arm64_armv8-a_core_shared_29_myapex/libm.so")
	// ... and is not compiling with the stub
	ensureNotContains(t, mylibCFlags, "__LIBM_API__=29")
	ensureNotContains(t, mylibSharedCFlags, "__LIBM_API__=29")

	// For dependency to libdl
	// Ensure that mylib is linking with the specified version of stubs
	ensureContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_core_shared_27_myapex/libdl.so")
	// ... and not linking to the other versions of stubs
	ensureNotContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_core_shared_28_myapex/libdl.so")
	ensureNotContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_core_shared_29_myapex/libdl.so")
	// ... and not linking to the non-stub (impl) variant
	ensureNotContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_core_shared_myapex/libdl.so")
	// ... Cflags from stub is correctly exported to mylib
	ensureContains(t, mylibCFlags, "__LIBDL_API__=27")
	ensureContains(t, mylibSharedCFlags, "__LIBDL_API__=27")

	// Ensure that libBootstrap is depending on the platform variant of bionic libs
	libFlags := ctx.ModuleForTests("libBootstrap", "android_arm64_armv8-a_core_shared").Rule("ld").Args["libFlags"]
	ensureContains(t, libFlags, "libc/android_arm64_armv8-a_core_shared/libc.so")
	ensureContains(t, libFlags, "libm/android_arm64_armv8-a_core_shared/libm.so")
	ensureContains(t, libFlags, "libdl/android_arm64_armv8-a_core_shared/libdl.so")
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
		}

		cc_binary {
			name: "mybin",
			srcs: ["mylib.cpp"],
			relative_install_path: "foo/bar",
			system_shared_libs: [],
			static_executable: true,
			stl: "none",
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
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			vendor_available: true,
			stl: "none",
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
	ensureContains(t, inputsString, "android_arm64_armv8-a_vendor.VER_shared_myapex/mylib.so")
	ensureContains(t, inputsString, "android_arm64_armv8-a_vendor.VER_shared_myapex/mylib2.so")

	// ensure that the apex does not include core variants
	ensureNotContains(t, inputsString, "android_arm64_armv8-a_core_shared_myapex/mylib.so")
	ensureNotContains(t, inputsString, "android_arm64_armv8-a_core_shared_myapex/mylib2.so")
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

	ldFlags := ctx.ModuleForTests("not_in_apex", "android_arm64_armv8-a_core").Rule("ld").Args["libFlags"]

	// Ensure that not_in_apex is linking with the static variant of mylib
	ensureContains(t, ldFlags, "mylib/android_arm64_armv8-a_core_static/mylib.a")
}

func TestKeys(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex_keytest",
			key: "myapex.key",
			certificate: ":myapex.certificate",
			native_shared_libs: ["mylib"],
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
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
		}
	`)

	// non-APEX variant does not have __ANDROID_APEX(_NAME)__ defined
	mylibCFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_core_static").Rule("cc").Args["cFlags"]
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX__")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX_MYAPEX__")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX_OTHERAPEX__")

	// APEX variant has __ANDROID_APEX(_NAME)__ defined
	mylibCFlags = ctx.ModuleForTests("mylib", "android_arm64_armv8-a_core_static_myapex").Rule("cc").Args["cFlags"]
	ensureContains(t, mylibCFlags, "-D__ANDROID_APEX__")
	ensureContains(t, mylibCFlags, "-D__ANDROID_APEX_MYAPEX__")
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX_OTHERAPEX__")

	// APEX variant has __ANDROID_APEX(_NAME)__ defined
	mylibCFlags = ctx.ModuleForTests("mylib", "android_arm64_armv8-a_core_static_otherapex").Rule("cc").Args["cFlags"]
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
		}

		cc_library {
			name: "otherlib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			shared_libs: ["mylib"],
		}
	`)

	cFlags := ctx.ModuleForTests("otherlib", "android_arm64_armv8-a_core_static").Rule("cc").Args["cFlags"]

	// Ensure that the include path of the header lib is exported to 'otherlib'
	ensureContains(t, cFlags, "-Imy_include")
}

func ensureExactContents(t *testing.T, ctx *android.TestContext, moduleName string, files []string) {
	t.Helper()
	apexRule := ctx.ModuleForTests(moduleName, "android_common_"+moduleName+"_image").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]
	imageApexDir := "/image.apex/"
	var failed bool
	var surplus []string
	filesMatched := make(map[string]bool)
	addContent := func(content string) {
		for _, expected := range files {
			if matched, _ := path.Match(expected, content); matched {
				filesMatched[expected] = true
				return
			}
		}
		surplus = append(surplus, content)
	}
	for _, cmd := range strings.Split(copyCmds, "&&") {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		terms := strings.Split(cmd, " ")
		switch terms[0] {
		case "mkdir":
		case "cp":
			if len(terms) != 3 {
				t.Fatal("copyCmds contains invalid cp command", cmd)
			}
			dst := terms[2]
			index := strings.Index(dst, imageApexDir)
			if index == -1 {
				t.Fatal("copyCmds should copy a file to image.apex/", cmd)
			}
			dstFile := dst[index+len(imageApexDir):]
			addContent(dstFile)
		default:
			t.Fatalf("copyCmds should contain mkdir/cp commands only: %q", cmd)
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
			file_contexts: "myapex",
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
		}
	`+vndkLibrariesTxtFiles("current"))

	ensureExactContents(t, ctx, "myapex", []string{
		"lib/libvndk.so",
		"lib/libvndksp.so",
		"lib64/libvndk.so",
		"lib64/libvndksp.so",
		"etc/llndk.libraries.VER.txt",
		"etc/vndkcore.libraries.VER.txt",
		"etc/vndksp.libraries.VER.txt",
		"etc/vndkprivate.libraries.VER.txt",
		"etc/vndkcorevariant.libraries.VER.txt",
	})
}

func TestVndkApexWithPrebuilt(t *testing.T) {
	ctx, _ := testApex(t, `
		apex_vndk {
			name: "myapex",
			key: "myapex.key",
			file_contexts: "myapex",
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
		}
		`+vndkLibrariesTxtFiles("current"),
		withFiles(map[string][]byte{
			"libvndk.so":     nil,
			"libvndk.arm.so": nil,
		}))

	ensureExactContents(t, ctx, "myapex", []string{
		"lib/libvndk.so",
		"lib/libvndk.arm.so",
		"lib64/libvndk.so",
		"etc/*",
	})
}

func vndkLibrariesTxtFiles(vers ...string) (result string) {
	for _, v := range vers {
		if v == "current" {
			for _, txt := range []string{"llndk", "vndkcore", "vndksp", "vndkprivate", "vndkcorevariant"} {
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
			file_contexts: "myapex",
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

	ensureExactContents(t, ctx, "myapex_v27", []string{
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
			file_contexts: "myapex",
			vndk_version: "27",
		}
		apex_vndk {
			name: "myapex_v27_other",
			key: "myapex.key",
			file_contexts: "myapex",
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
			file_contexts: "myapex",
		}
		apex_vndk {
			name: "myapex_v28",
			key: "myapex.key",
			file_contexts: "myapex",
			vndk_version: "28",
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}`+vndkLibrariesTxtFiles("28", "current"))

	assertApexName := func(expected, moduleName string) {
		bundle := ctx.ModuleForTests(moduleName, "android_common_"+moduleName+"_image").Module().(*apexBundle)
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
			file_contexts: "myapex",
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
		`+vndkLibrariesTxtFiles("current"),
		withTargets(map[android.OsType][]android.Target{
			android.Android: []android.Target{
				{Os: android.Android, Arch: android.Arch{ArchType: android.Arm64, ArchVariant: "armv8-a", Abi: []string{"arm64-v8a"}}, NativeBridge: android.NativeBridgeDisabled, NativeBridgeHostArchName: "", NativeBridgeRelativePath: ""},
				{Os: android.Android, Arch: android.Arch{ArchType: android.Arm, ArchVariant: "armv7-a-neon", Abi: []string{"armeabi-v7a"}}, NativeBridge: android.NativeBridgeDisabled, NativeBridgeHostArchName: "", NativeBridgeRelativePath: ""},
				{Os: android.Android, Arch: android.Arch{ArchType: android.X86_64, ArchVariant: "silvermont", Abi: []string{"arm64-v8a"}}, NativeBridge: android.NativeBridgeEnabled, NativeBridgeHostArchName: "arm64", NativeBridgeRelativePath: "x86_64"},
				{Os: android.Android, Arch: android.Arch{ArchType: android.X86, ArchVariant: "silvermont", Abi: []string{"armeabi-v7a"}}, NativeBridge: android.NativeBridgeEnabled, NativeBridgeHostArchName: "arm", NativeBridgeRelativePath: "x86"},
			},
		}))

	ensureExactContents(t, ctx, "myapex", []string{
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
			file_contexts: "myapex",
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
			file_contexts: "myapex",
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

	ensureExactContents(t, ctx, "myapex_v27", []string{
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
			file_contexts: "myapex",
		}

		apex {
			name: "myapex_dep",
			key: "myapex.key",
			native_shared_libs: ["lib_dep"],
			compile_multilib: "both",
			file_contexts: "myapex",
		}

		apex {
			name: "myapex_provider",
			key: "myapex.key",
			native_shared_libs: ["libfoo"],
			compile_multilib: "both",
			file_contexts: "myapex",
		}

		apex {
			name: "myapex_selfcontained",
			key: "myapex.key",
			native_shared_libs: ["lib_dep", "libfoo"],
			compile_multilib: "both",
			file_contexts: "myapex",
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
		}

		cc_library {
			name: "lib_dep",
			srcs: ["mylib.cpp"],
			shared_libs: ["libfoo"],
			system_shared_libs: [],
			stl: "none",
		}

		cc_library {
			name: "libfoo",
			srcs: ["mytest.cpp"],
			stubs: {
				versions: ["1"],
			},
			system_shared_libs: [],
			stl: "none",
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
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apex_name: "com.android.myapex",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)

	module := ctx.ModuleForTests("myapex", "android_common_myapex_image")
	apexManifestRule := module.Rule("apexManifestRule")
	ensureContains(t, apexManifestRule.Args["opt"], "-v name com.android.myapex")
	apexRule := module.Rule("apexRule")
	ensureContains(t, apexRule.Args["opt_flags"], "--do_not_check_keyname")
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
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common"), "android_arm64_armv8-a_core_shared_myapex")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image.apex/lib64/mylib_common.so")

	// Ensure that the platform variant ends with _core_shared
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common"), "android_arm64_armv8-a_core_shared")

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
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common_test"), "android_arm64_armv8-a_core_shared_myapex")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image.apex/lib64/mylib_common_test.so")

	// Ensure that the platform variant ends with _core_shared
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common_test"), "android_arm64_armv8-a_core_shared")

	if android.InAnyApex("mylib_common_test") {
		t.Log("Found mylib_common_test in some apex!")
		t.Fail()
	}
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
		}

		cc_library {
			name: "mylib_common",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			compile_multilib: "first",
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
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_core_shared_myapex")
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common"), "android_arm64_armv8-a_core_shared_myapex")
	ensureListNotContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_core_shared_myapex")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")
	ensureContains(t, copyCmds, "image.apex/lib64/mylib_common.so")
	ensureNotContains(t, copyCmds, "image.apex/lib64/mylib2.so")

	// Ensure that the platform variant ends with _core_shared
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_core_shared")
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common"), "android_arm64_armv8-a_core_shared")
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_core_shared")
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

func TestApexInProductPartition(t *testing.T) {
	ctx, _ := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			product_specific: true,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
			product_specific: true,
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
		}
	`)

	apex := ctx.ModuleForTests("myapex", "android_common_myapex_image").Module().(*apexBundle)
	expected := buildDir + "/target/product/test_device/product/apex"
	actual := apex.installDir.String()
	if actual != expected {
		t.Errorf("wrong install path. expected %q. actual %q", expected, actual)
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
	actual := android.AndroidMkEntriesForTest(t, config, "", p).EntryMap["LOCAL_OVERRIDES_PACKAGES"]
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Incorrect LOCAL_OVERRIDES_PACKAGES value '%s', expected '%s'", actual, expected)
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
	ensureContains(t, androidMk, "LOCAL_MODULE := apex_manifest.json.myapex\n")
	ensureContains(t, androidMk, "LOCAL_MODULE := apex_pubkey.myapex\n")
	ensureContains(t, androidMk, "LOCAL_MODULE := myapex\n")
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
		}

		cc_library {
			name: "libcommon",
			srcs: ["mylib_common.cpp"],
			system_shared_libs: [],
			stl: "none",
		}
	`)

	module1 := ctx.ModuleForTests("myapex", "android_common_myapex_image")
	apexRule1 := module1.Rule("apexRule")
	copyCmds1 := apexRule1.Args["copy_commands"]

	module2 := ctx.ModuleForTests("commonapex", "android_common_commonapex_image")
	apexRule2 := module2.Rule("apexRule")
	copyCmds2 := apexRule2.Args["copy_commands"]

	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_core_shared_myapex")
	ensureListContains(t, ctx.ModuleVariantsForTests("libcommon"), "android_arm64_armv8-a_core_shared_commonapex")
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
			compile_dex: true,
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
		}

		android_app {
			name: "AppFooPriv",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			privileged: true,
		}

		cc_library_shared {
			name: "libjni",
			srcs: ["mylib.cpp"],
			stl: "none",
			system_shared_libs: [],
		}
	`)

	module := ctx.ModuleForTests("myapex", "android_common_myapex_image")
	apexRule := module.Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	ensureContains(t, copyCmds, "image.apex/app/AppFoo/AppFoo.apk")
	ensureContains(t, copyCmds, "image.apex/priv-app/AppFooPriv/AppFooPriv.apk")
	ensureContains(t, copyCmds, "image.apex/lib64/libjni.so")
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
	ensureListContains(t, ctx.ModuleVariantsForTests("libfoo"), "android_arm64_armv8-a_core_shared_myapex")
	ensureListNotContains(t, ctx.ModuleVariantsForTests("libfoo"), "android_arm64_armv8-a_core_shared")
	ensureListContains(t, ctx.ModuleVariantsForTests("libbar"), "android_arm64_armv8-a_core_shared_myapex")
	ensureListNotContains(t, ctx.ModuleVariantsForTests("libbar"), "android_arm64_armv8-a_core_shared")

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
	ensureListNotContains(t, ctx.ModuleVariantsForTests("libfoo"), "android_arm64_armv8-a_core_shared_myapex")
	ensureListContains(t, ctx.ModuleVariantsForTests("libfoo"), "android_arm64_armv8-a_core_shared")

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
	ensureListContains(t, ctx.ModuleVariantsForTests("libfoo"), "android_arm64_armv8-a_core_shared_myapex")
	ensureListNotContains(t, ctx.ModuleVariantsForTests("libfoo"), "android_arm64_armv8-a_core_shared")
	// but the static variant is available to both myapex and the platform
	ensureListContains(t, ctx.ModuleVariantsForTests("libfoo"), "android_arm64_armv8-a_core_static_myapex")
	ensureListContains(t, ctx.ModuleVariantsForTests("libfoo"), "android_arm64_armv8-a_core_static")
}

func TestMain(m *testing.M) {
	run := func() int {
		setUp()
		defer tearDown()

		return m.Run()
	}

	os.Exit(run())
}
