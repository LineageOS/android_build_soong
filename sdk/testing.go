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

func testSdkContext(bp string) (*android.TestContext, android.Config) {
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
	ctx, config := testSdkContext(bp)
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)
	return ctx, config
}

func testSdkError(t *testing.T, pattern, bp string) {
	t.Helper()
	ctx, config := testSdkContext(bp)
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

func ensureListContains(t *testing.T, result []string, expected string) {
	t.Helper()
	if !android.InList(expected, result) {
		t.Errorf("%q is not found in %v", expected, result)
	}
}

func pathsToStrings(paths android.Paths) []string {
	var ret []string
	for _, p := range paths {
		ret = append(ret, p.String())
	}
	return ret
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
	_ = os.RemoveAll(buildDir)
}

func runTestWithBuildDir(m *testing.M) {
	run := func() int {
		setUp()
		defer tearDown()

		return m.Run()
	}

	os.Exit(run())
}

func SkipIfNotLinux(t *testing.T) {
	t.Helper()
	if android.BuildOs != android.Linux {
		t.Skipf("Skipping as sdk snapshot generation is only supported on %s not %s", android.Linux, android.BuildOs)
	}
}
