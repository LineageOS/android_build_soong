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

package sdk

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/apex"
	"android/soong/cc"
	"android/soong/java"
)

func testSdkContext(t *testing.T, bp string) (*android.TestContext, android.Config) {
	config := android.TestArchConfig(buildDir, nil)
	ctx := android.NewTestArchContext()

	// from android package
	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	ctx.PreArchMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("prebuilts", android.PrebuiltMutator).Parallel()
	})
	ctx.PostDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("prebuilt_select", android.PrebuiltSelectModuleMutator).Parallel()
		ctx.BottomUp("prebuilt_postdeps", android.PrebuiltPostDepsMutator).Parallel()
	})

	// from java package
	ctx.RegisterModuleType("android_app_certificate", java.AndroidAppCertificateFactory)
	ctx.RegisterModuleType("java_library", java.LibraryFactory)
	ctx.RegisterModuleType("java_import", java.ImportFactory)
	ctx.RegisterModuleType("droidstubs", java.DroidstubsFactory)
	ctx.RegisterModuleType("prebuilt_stubs_sources", java.PrebuiltStubsSourcesFactory)

	// from cc package
	ctx.RegisterModuleType("cc_library", cc.LibraryFactory)
	ctx.RegisterModuleType("cc_library_shared", cc.LibrarySharedFactory)
	ctx.RegisterModuleType("cc_object", cc.ObjectFactory)
	ctx.RegisterModuleType("cc_prebuilt_library_shared", cc.PrebuiltSharedLibraryFactory)
	ctx.RegisterModuleType("cc_prebuilt_library_static", cc.PrebuiltStaticLibraryFactory)
	ctx.RegisterModuleType("llndk_library", cc.LlndkLibraryFactory)
	ctx.RegisterModuleType("toolchain_library", cc.ToolchainLibraryFactory)
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("image", android.ImageMutator).Parallel()
		ctx.BottomUp("link", cc.LinkageMutator).Parallel()
		ctx.BottomUp("vndk", cc.VndkMutator).Parallel()
		ctx.BottomUp("test_per_src", cc.TestPerSrcMutator).Parallel()
		ctx.BottomUp("version", cc.VersionMutator).Parallel()
		ctx.BottomUp("begin", cc.BeginMutator).Parallel()
	})

	// from apex package
	ctx.RegisterModuleType("apex", apex.BundleFactory)
	ctx.RegisterModuleType("apex_key", apex.ApexKeyFactory)
	ctx.PostDepsMutators(apex.RegisterPostDepsMutators)

	// from this package
	ctx.RegisterModuleType("sdk", ModuleFactory)
	ctx.RegisterModuleType("sdk_snapshot", SnapshotModuleFactory)
	ctx.PreDepsMutators(RegisterPreDepsMutators)
	ctx.PostDepsMutators(RegisterPostDepsMutators)

	ctx.Register()

	bp = bp + `
		apex_key {
			name: "myapex.key",
			public_key: "myapex.avbpubkey",
			private_key: "myapex.pem",
		}

		android_app_certificate {
			name: "myapex.cert",
			certificate: "myapex",
		}
	` + cc.GatherRequiredDepsForTest(android.Android)

	ctx.MockFileSystem(map[string][]byte{
		"Android.bp":                                 []byte(bp),
		"build/make/target/product/security":         nil,
		"apex_manifest.json":                         nil,
		"system/sepolicy/apex/myapex-file_contexts":  nil,
		"system/sepolicy/apex/myapex2-file_contexts": nil,
		"myapex.avbpubkey":                           nil,
		"myapex.pem":                                 nil,
		"myapex.x509.pem":                            nil,
		"myapex.pk8":                                 nil,
		"Test.java":                                  nil,
		"Test.cpp":                                   nil,
		"include/Test.h":                             nil,
		"aidl/foo/bar/Test.aidl":                     nil,
		"libfoo.so":                                  nil,
		"stubs-sources/foo/bar/Foo.java":             nil,
		"foo/bar/Foo.java":                           nil,
	})

	return ctx, config
}

func testSdk(t *testing.T, bp string) (*android.TestContext, android.Config) {
	ctx, config := testSdkContext(t, bp)
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)
	return ctx, config
}

func testSdkError(t *testing.T, pattern, bp string) {
	t.Helper()
	ctx, config := testSdkContext(t, bp)
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

func pathsToStrings(paths android.Paths) []string {
	ret := []string{}
	for _, p := range paths {
		ret = append(ret, p.String())
	}
	return ret
}

func TestBasicSdkWithJava(t *testing.T) {
	ctx, _ := testSdk(t, `
		sdk {
			name: "mysdk",
			java_libs: ["sdkmember"],
		}

		sdk_snapshot {
			name: "mysdk@1",
			java_libs: ["sdkmember_mysdk_1"],
		}

		sdk_snapshot {
			name: "mysdk@2",
			java_libs: ["sdkmember_mysdk_2"],
		}

		java_import {
			name: "sdkmember",
			prefer: false,
			host_supported: true,
		}

		java_import {
			name: "sdkmember_mysdk_1",
			sdk_member_name: "sdkmember",
			host_supported: true,
		}

		java_import {
			name: "sdkmember_mysdk_2",
			sdk_member_name: "sdkmember",
			host_supported: true,
		}

		java_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			libs: ["sdkmember"],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
			host_supported: true,
		}

		apex {
			name: "myapex",
			java_libs: ["myjavalib"],
			uses_sdks: ["mysdk@1"],
			key: "myapex.key",
			certificate: ":myapex.cert",
		}

		apex {
			name: "myapex2",
			java_libs: ["myjavalib"],
			uses_sdks: ["mysdk@2"],
			key: "myapex.key",
			certificate: ":myapex.cert",
		}
	`)

	sdkMemberV1 := ctx.ModuleForTests("sdkmember_mysdk_1", "android_common_myapex").Rule("combineJar").Output
	sdkMemberV2 := ctx.ModuleForTests("sdkmember_mysdk_2", "android_common_myapex2").Rule("combineJar").Output

	javalibForMyApex := ctx.ModuleForTests("myjavalib", "android_common_myapex")
	javalibForMyApex2 := ctx.ModuleForTests("myjavalib", "android_common_myapex2")

	// Depending on the uses_sdks value, different libs are linked
	ensureListContains(t, pathsToStrings(javalibForMyApex.Rule("javac").Implicits), sdkMemberV1.String())
	ensureListContains(t, pathsToStrings(javalibForMyApex2.Rule("javac").Implicits), sdkMemberV2.String())
}

func TestBasicSdkWithCc(t *testing.T) {
	ctx, _ := testSdk(t, `
		sdk {
			name: "mysdk",
			native_shared_libs: ["sdkmember"],
		}

		sdk_snapshot {
			name: "mysdk@1",
			native_shared_libs: ["sdkmember_mysdk_1"],
		}

		sdk_snapshot {
			name: "mysdk@2",
			native_shared_libs: ["sdkmember_mysdk_2"],
		}

		cc_prebuilt_library_shared {
			name: "sdkmember",
			srcs: ["libfoo.so"],
			prefer: false,
			system_shared_libs: [],
			stl: "none",
		}

		cc_prebuilt_library_shared {
			name: "sdkmember_mysdk_1",
			sdk_member_name: "sdkmember",
			srcs: ["libfoo.so"],
			system_shared_libs: [],
			stl: "none",
		}

		cc_prebuilt_library_shared {
			name: "sdkmember_mysdk_2",
			sdk_member_name: "sdkmember",
			srcs: ["libfoo.so"],
			system_shared_libs: [],
			stl: "none",
		}

		cc_library_shared {
			name: "mycpplib",
			srcs: ["Test.cpp"],
			shared_libs: ["sdkmember"],
			system_shared_libs: [],
			stl: "none",
		}

		apex {
			name: "myapex",
			native_shared_libs: ["mycpplib"],
			uses_sdks: ["mysdk@1"],
			key: "myapex.key",
			certificate: ":myapex.cert",
		}

		apex {
			name: "myapex2",
			native_shared_libs: ["mycpplib"],
			uses_sdks: ["mysdk@2"],
			key: "myapex.key",
			certificate: ":myapex.cert",
		}
	`)

	sdkMemberV1 := ctx.ModuleForTests("sdkmember_mysdk_1", "android_arm64_armv8-a_core_shared_myapex").Rule("toc").Output
	sdkMemberV2 := ctx.ModuleForTests("sdkmember_mysdk_2", "android_arm64_armv8-a_core_shared_myapex2").Rule("toc").Output

	cpplibForMyApex := ctx.ModuleForTests("mycpplib", "android_arm64_armv8-a_core_shared_myapex")
	cpplibForMyApex2 := ctx.ModuleForTests("mycpplib", "android_arm64_armv8-a_core_shared_myapex2")

	// Depending on the uses_sdks value, different libs are linked
	ensureListContains(t, pathsToStrings(cpplibForMyApex.Rule("ld").Implicits), sdkMemberV1.String())
	ensureListContains(t, pathsToStrings(cpplibForMyApex2.Rule("ld").Implicits), sdkMemberV2.String())
}

// Note: This test does not verify that a droidstubs can be referenced, either
// directly or indirectly from an APEX as droidstubs can never be a part of an
// apex.
func TestBasicSdkWithDroidstubs(t *testing.T) {
	testSdk(t, `
		sdk {
				name: "mysdk",
				stubs_sources: ["mystub"],
		}
		sdk_snapshot {
				name: "mysdk@10",
				stubs_sources: ["mystub_mysdk@10"],
		}
		prebuilt_stubs_sources {
				name: "mystub_mysdk@10",
				sdk_member_name: "mystub",
				srcs: ["stubs-sources/foo/bar/Foo.java"],
		}
		droidstubs {
				name: "mystub",
				srcs: ["foo/bar/Foo.java"],
				sdk_version: "none",
				system_modules: "none",
		}
		java_library {
				name: "myjavalib",
				srcs: [":mystub"],
				sdk_version: "none",
				system_modules: "none",
		}
	`)
}

func TestDepNotInRequiredSdks(t *testing.T) {
	testSdkError(t, `module "myjavalib".*depends on "otherlib".*that isn't part of the required SDKs:.*`, `
		sdk {
			name: "mysdk",
			java_libs: ["sdkmember"],
		}

		sdk_snapshot {
			name: "mysdk@1",
			java_libs: ["sdkmember_mysdk_1"],
		}

		java_import {
			name: "sdkmember",
			prefer: false,
			host_supported: true,
		}

		java_import {
			name: "sdkmember_mysdk_1",
			sdk_member_name: "sdkmember",
			host_supported: true,
		}

		java_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			libs: [
				"sdkmember",
				"otherlib",
			],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
			host_supported: true,
		}

		// this lib is no in mysdk
		java_library {
			name: "otherlib",
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
			host_supported: true,
		}

		apex {
			name: "myapex",
			java_libs: ["myjavalib"],
			uses_sdks: ["mysdk@1"],
			key: "myapex.key",
			certificate: ":myapex.cert",
		}
	`)
}

func TestSdkIsCompileMultilibBoth(t *testing.T) {
	ctx, _ := testSdk(t, `
		sdk {
			name: "mysdk",
			native_shared_libs: ["sdkmember"],
		}

		cc_library_shared {
			name: "sdkmember",
			srcs: ["Test.cpp"],
			system_shared_libs: [],
			stl: "none",
		}
	`)

	armOutput := ctx.ModuleForTests("sdkmember", "android_arm_armv7-a-neon_core_shared").Module().(*cc.Module).OutputFile()
	arm64Output := ctx.ModuleForTests("sdkmember", "android_arm64_armv8-a_core_shared").Module().(*cc.Module).OutputFile()

	var inputs []string
	buildParams := ctx.ModuleForTests("mysdk", "android_common").Module().BuildParamsForTests()
	for _, bp := range buildParams {
		if bp.Input != nil {
			inputs = append(inputs, bp.Input.String())
		}
	}

	// ensure that both 32/64 outputs are inputs of the sdk snapshot
	ensureListContains(t, inputs, armOutput.String())
	ensureListContains(t, inputs, arm64Output.String())
}

func TestSnapshot(t *testing.T) {
	ctx, config := testSdk(t, `
		sdk {
			name: "mysdk",
			java_libs: ["myjavalib"],
			native_shared_libs: ["mynativelib"],
			stubs_sources: ["myjavaapistubs"],
		}

		java_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			aidl: {
				export_include_dirs: ["aidl"],
			},
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
			host_supported: true,
		}

		cc_library_shared {
			name: "mynativelib",
			srcs: [
				"Test.cpp",
				"aidl/foo/bar/Test.aidl",
			],
			export_include_dirs: ["include"],
			aidl: {
				export_aidl_headers: true,
			},
			system_shared_libs: [],
			stl: "none",
		}

		droidstubs {
			name: "myjavaapistubs",
			srcs: ["foo/bar/Foo.java"],
			system_modules: "none",
			sdk_version: "none",
		}
	`)

	sdk := ctx.ModuleForTests("mysdk", "android_common").Module().(*sdk)

	checkSnapshotAndroidBpContents(t, sdk, `// This is auto-generated. DO NOT EDIT.

java_import {
    name: "mysdk_myjavalib@current",
    sdk_member_name: "myjavalib",
    jars: ["java/myjavalib.jar"],
}

java_import {
    name: "myjavalib",
    prefer: false,
    jars: ["java/myjavalib.jar"],
}

prebuilt_stubs_sources {
    name: "mysdk_myjavaapistubs@current",
    sdk_member_name: "myjavaapistubs",
    srcs: ["java/myjavaapistubs_stubs_sources"],
}

prebuilt_stubs_sources {
    name: "myjavaapistubs",
    prefer: false,
    srcs: ["java/myjavaapistubs_stubs_sources"],
}

cc_prebuilt_library_shared {
    name: "mysdk_mynativelib@current",
    sdk_member_name: "mynativelib",
    arch: {
        arm64: {
            srcs: ["arm64/lib/mynativelib.so"],
            export_include_dirs: [
                "arm64/include/include",
                "arm64/include_gen/mynativelib",
            ],
        },
        arm: {
            srcs: ["arm/lib/mynativelib.so"],
            export_include_dirs: [
                "arm/include/include",
                "arm/include_gen/mynativelib",
            ],
        },
    },
    stl: "none",
    system_shared_libs: [],
}

cc_prebuilt_library_shared {
    name: "mynativelib",
    prefer: false,
    arch: {
        arm64: {
            srcs: ["arm64/lib/mynativelib.so"],
            export_include_dirs: [
                "arm64/include/include",
                "arm64/include_gen/mynativelib",
            ],
        },
        arm: {
            srcs: ["arm/lib/mynativelib.so"],
            export_include_dirs: [
                "arm/include/include",
                "arm/include_gen/mynativelib",
            ],
        },
    },
    stl: "none",
    system_shared_libs: [],
}

sdk_snapshot {
    name: "mysdk@current",
    java_libs: [
        "mysdk_myjavalib@current",
    ],
    stubs_sources: [
        "mysdk_myjavaapistubs@current",
    ],
    native_shared_libs: [
        "mysdk_mynativelib@current",
    ],
}

`)

	var copySrcs []string
	var copyDests []string
	buildParams := sdk.BuildParamsForTests()
	var zipBp android.BuildParams
	for _, bp := range buildParams {
		ruleString := bp.Rule.String()
		if ruleString == "android/soong/android.Cp" {
			copySrcs = append(copySrcs, bp.Input.String())
			copyDests = append(copyDests, bp.Output.Rel()) // rooted at the snapshot root
		} else if ruleString == "<local rule>:m.mysdk_android_common.snapshot" {
			zipBp = bp
		}
	}

	buildDir := config.BuildDir()
	ensureListContains(t, copySrcs, "aidl/foo/bar/Test.aidl")
	ensureListContains(t, copySrcs, "include/Test.h")
	ensureListContains(t, copySrcs, filepath.Join(buildDir, ".intermediates/mynativelib/android_arm64_armv8-a_core_shared/gen/aidl/aidl/foo/bar/BnTest.h"))
	ensureListContains(t, copySrcs, filepath.Join(buildDir, ".intermediates/mynativelib/android_arm64_armv8-a_core_shared/gen/aidl/aidl/foo/bar/BpTest.h"))
	ensureListContains(t, copySrcs, filepath.Join(buildDir, ".intermediates/mynativelib/android_arm64_armv8-a_core_shared/gen/aidl/aidl/foo/bar/Test.h"))
	ensureListContains(t, copySrcs, filepath.Join(buildDir, ".intermediates/myjavalib/android_common/turbine-combined/myjavalib.jar"))
	ensureListContains(t, copySrcs, filepath.Join(buildDir, ".intermediates/mynativelib/android_arm64_armv8-a_core_shared/mynativelib.so"))

	ensureListContains(t, copyDests, "aidl/aidl/foo/bar/Test.aidl")
	ensureListContains(t, copyDests, "arm64/include/include/Test.h")
	ensureListContains(t, copyDests, "arm64/include_gen/mynativelib/aidl/foo/bar/BnTest.h")
	ensureListContains(t, copyDests, "arm64/include_gen/mynativelib/aidl/foo/bar/BpTest.h")
	ensureListContains(t, copyDests, "arm64/include_gen/mynativelib/aidl/foo/bar/Test.h")
	ensureListContains(t, copyDests, "java/myjavalib.jar")
	ensureListContains(t, copyDests, "arm64/lib/mynativelib.so")

	// Ensure that the droidstubs .srcjar as repackaged into a temporary zip file
	// and then merged together with the intermediate snapshot zip.
	snapshotCreationInputs := zipBp.Implicits.Strings()
	ensureListContains(t, snapshotCreationInputs,
		filepath.Join(buildDir, ".intermediates/mysdk/android_common/tmp/java/myjavaapistubs_stubs_sources.zip"))
	ensureListContains(t, snapshotCreationInputs,
		filepath.Join(buildDir, ".intermediates/mysdk/android_common/mysdk-current.unmerged.zip"))
	actual := zipBp.Output.String()
	expected := filepath.Join(buildDir, ".intermediates/mysdk/android_common/mysdk-current.zip")
	if actual != expected {
		t.Errorf("Expected snapshot output to be %q but was %q", expected, actual)
	}
}

func checkSnapshotAndroidBpContents(t *testing.T, s *sdk, expectedContents string) {
	t.Helper()
	androidBpContents := strings.NewReplacer("\\n", "\n").Replace(s.GetAndroidBpContentsForTests())
	if androidBpContents != expectedContents {
		t.Errorf("Android.bp contents do not match, expected %s, actual %s", expectedContents, androidBpContents)
	}
}

var buildDir string

func setUp() {
	var err error
	buildDir, err = ioutil.TempDir("", "soong_sdk_test")
	if err != nil {
		panic(err)
	}
}

func tearDown() {
	os.RemoveAll(buildDir)
}

func TestMain(m *testing.M) {
	run := func() int {
		setUp()
		defer tearDown()

		return m.Run()
	}

	os.Exit(run())
}
