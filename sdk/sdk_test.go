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
	ctx.RegisterModuleType("android_app_certificate", android.ModuleFactoryAdaptor(java.AndroidAppCertificateFactory))
	ctx.RegisterModuleType("java_library", android.ModuleFactoryAdaptor(java.LibraryFactory))
	ctx.RegisterModuleType("java_import", android.ModuleFactoryAdaptor(java.ImportFactory))

	// from cc package
	ctx.RegisterModuleType("cc_library", android.ModuleFactoryAdaptor(cc.LibraryFactory))
	ctx.RegisterModuleType("cc_library_shared", android.ModuleFactoryAdaptor(cc.LibrarySharedFactory))
	ctx.RegisterModuleType("cc_object", android.ModuleFactoryAdaptor(cc.ObjectFactory))
	ctx.RegisterModuleType("cc_prebuilt_library_shared", android.ModuleFactoryAdaptor(cc.PrebuiltSharedLibraryFactory))
	ctx.RegisterModuleType("cc_prebuilt_library_static", android.ModuleFactoryAdaptor(cc.PrebuiltStaticLibraryFactory))
	ctx.RegisterModuleType("llndk_library", android.ModuleFactoryAdaptor(cc.LlndkLibraryFactory))
	ctx.RegisterModuleType("toolchain_library", android.ModuleFactoryAdaptor(cc.ToolchainLibraryFactory))
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("image", cc.ImageMutator).Parallel()
		ctx.BottomUp("link", cc.LinkageMutator).Parallel()
		ctx.BottomUp("vndk", cc.VndkMutator).Parallel()
		ctx.BottomUp("test_per_src", cc.TestPerSrcMutator).Parallel()
		ctx.BottomUp("version", cc.VersionMutator).Parallel()
		ctx.BottomUp("begin", cc.BeginMutator).Parallel()
	})

	// from apex package
	ctx.RegisterModuleType("apex", android.ModuleFactoryAdaptor(apex.BundleFactory))
	ctx.RegisterModuleType("apex_key", android.ModuleFactoryAdaptor(apex.ApexKeyFactory))
	ctx.PostDepsMutators(apex.RegisterPostDepsMutators)

	// from this package
	ctx.RegisterModuleType("sdk", android.ModuleFactoryAdaptor(ModuleFactory))
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
		"libfoo.so":                                  nil,
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
			name: "mysdk#1",
			java_libs: ["sdkmember_mysdk_1"],
		}

		sdk {
			name: "mysdk#2",
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
			uses_sdks: ["mysdk#1"],
			key: "myapex.key",
			certificate: ":myapex.cert",
		}

		apex {
			name: "myapex2",
			java_libs: ["myjavalib"],
			uses_sdks: ["mysdk#2"],
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
			name: "mysdk#1",
			native_shared_libs: ["sdkmember_mysdk_1"],
		}

		sdk {
			name: "mysdk#2",
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
			uses_sdks: ["mysdk#1"],
			key: "myapex.key",
			certificate: ":myapex.cert",
		}

		apex {
			name: "myapex2",
			native_shared_libs: ["mycpplib"],
			uses_sdks: ["mysdk#2"],
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
