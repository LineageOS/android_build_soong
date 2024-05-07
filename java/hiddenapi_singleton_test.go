// Copyright 2020 Google Inc. All rights reserved.
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

package java

import (
	"fmt"
	"path/filepath"
	"testing"

	"android/soong/android"

	"github.com/google/blueprint/proptools"
)

// TODO(b/177892522): Move these tests into a more appropriate place.

func fixtureSetPrebuiltHiddenApiDirProductVariable(prebuiltHiddenApiDir *string) android.FixturePreparer {
	return android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
		variables.PrebuiltHiddenApiDir = prebuiltHiddenApiDir
	})
}

var prepareForTestWithDefaultPlatformBootclasspath = android.FixtureAddTextFile("frameworks/base/boot/Android.bp", `
	platform_bootclasspath {
		name: "platform-bootclasspath",
	}
`)

var hiddenApiFixtureFactory = android.GroupFixturePreparers(
	PrepareForTestWithJavaDefaultModules,
	PrepareForTestWithHiddenApiBuildComponents,
)

func TestHiddenAPISingleton(t *testing.T) {
	result := android.GroupFixturePreparers(
		hiddenApiFixtureFactory,
		FixtureConfigureBootJars("platform:foo"),
		prepareForTestWithDefaultPlatformBootclasspath,
	).RunTestWithBp(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			compile_dex: true,
		}
	`)

	hiddenAPI := result.ModuleForTests("platform-bootclasspath", "android_common")
	hiddenapiRule := hiddenAPI.Rule("platform-bootclasspath-monolithic-hiddenapi-stub-flags")
	want := "--boot-dex=out/soong/.intermediates/foo/android_common/aligned/foo.jar"
	android.AssertStringDoesContain(t, "hiddenapi command", hiddenapiRule.RuleParams.Command, want)
}

func TestHiddenAPISingletonWithSourceAndPrebuiltPreferredButNoDex(t *testing.T) {
	expectedErrorMessage := "module prebuilt_foo{os:android,arch:common} does not provide a dex jar"

	android.GroupFixturePreparers(
		hiddenApiFixtureFactory,
		FixtureConfigureBootJars("platform:foo"),
		prepareForTestWithDefaultPlatformBootclasspath,
	).ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(expectedErrorMessage)).
		RunTestWithBp(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			compile_dex: true,
		}

		java_import {
			name: "foo",
			jars: ["a.jar"],
			prefer: true,
		}
	`)
}

func TestHiddenAPISingletonWithPrebuilt(t *testing.T) {
	result := android.GroupFixturePreparers(
		hiddenApiFixtureFactory,
		FixtureConfigureBootJars("platform:foo"),
		prepareForTestWithDefaultPlatformBootclasspath,
	).RunTestWithBp(t, `
		java_import {
			name: "foo",
			jars: ["a.jar"],
			compile_dex: true,
	}
	`)

	hiddenAPI := result.ModuleForTests("platform-bootclasspath", "android_common")
	hiddenapiRule := hiddenAPI.Rule("platform-bootclasspath-monolithic-hiddenapi-stub-flags")
	want := "--boot-dex=out/soong/.intermediates/foo/android_common/aligned/foo.jar"
	android.AssertStringDoesContain(t, "hiddenapi command", hiddenapiRule.RuleParams.Command, want)
}

func TestHiddenAPISingletonWithPrebuiltUseSource(t *testing.T) {
	result := android.GroupFixturePreparers(
		hiddenApiFixtureFactory,
		FixtureConfigureBootJars("platform:foo"),
		prepareForTestWithDefaultPlatformBootclasspath,
	).RunTestWithBp(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			compile_dex: true,
		}

		java_import {
			name: "foo",
			jars: ["a.jar"],
			compile_dex: true,
			prefer: false,
		}
	`)

	hiddenAPI := result.ModuleForTests("platform-bootclasspath", "android_common")
	hiddenapiRule := hiddenAPI.Rule("platform-bootclasspath-monolithic-hiddenapi-stub-flags")
	fromSourceJarArg := "--boot-dex=out/soong/.intermediates/foo/android_common/aligned/foo.jar"
	android.AssertStringDoesContain(t, "hiddenapi command", hiddenapiRule.RuleParams.Command, fromSourceJarArg)

	prebuiltJarArg := "--boot-dex=out/soong/.intermediates/foo/android_common/dex/foo.jar"
	android.AssertStringDoesNotContain(t, "hiddenapi command", hiddenapiRule.RuleParams.Command, prebuiltJarArg)
}

func TestHiddenAPISingletonWithPrebuiltOverrideSource(t *testing.T) {
	result := android.GroupFixturePreparers(
		hiddenApiFixtureFactory,
		FixtureConfigureBootJars("platform:foo"),
		prepareForTestWithDefaultPlatformBootclasspath,
	).RunTestWithBp(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			compile_dex: true,
		}

		java_import {
			name: "foo",
			jars: ["a.jar"],
			compile_dex: true,
			prefer: true,
		}
	`)

	hiddenAPI := result.ModuleForTests("platform-bootclasspath", "android_common")
	hiddenapiRule := hiddenAPI.Rule("platform-bootclasspath-monolithic-hiddenapi-stub-flags")
	prebuiltJarArg := "--boot-dex=out/soong/.intermediates/prebuilt_foo/android_common/dex/foo.jar"
	android.AssertStringDoesContain(t, "hiddenapi command", hiddenapiRule.RuleParams.Command, prebuiltJarArg)

	fromSourceJarArg := "--boot-dex=out/soong/.intermediates/foo/android_common/aligned/foo.jar"
	android.AssertStringDoesNotContain(t, "hiddenapi command", hiddenapiRule.RuleParams.Command, fromSourceJarArg)
}

func TestHiddenAPISingletonSdks(t *testing.T) {
	testCases := []struct {
		name             string
		unbundledBuild   bool
		publicStub       string
		systemStub       string
		testStub         string
		corePlatformStub string

		// Additional test preparer
		preparer android.FixturePreparer
	}{
		{
			name:             "testBundled",
			unbundledBuild:   false,
			publicStub:       "android_stubs_current_exportable",
			systemStub:       "android_system_stubs_current_exportable",
			testStub:         "android_test_stubs_current_exportable",
			corePlatformStub: "legacy.core.platform.api.stubs.exportable",
			preparer:         android.GroupFixturePreparers(),
		}, {
			name:             "testUnbundled",
			unbundledBuild:   true,
			publicStub:       "sdk_public_current_android",
			systemStub:       "sdk_system_current_android",
			testStub:         "sdk_test_current_android",
			corePlatformStub: "legacy.core.platform.api.stubs.exportable",
			preparer:         PrepareForTestWithPrebuiltsOfCurrentApi,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := android.GroupFixturePreparers(
				hiddenApiFixtureFactory,
				tc.preparer,
				prepareForTestWithDefaultPlatformBootclasspath,
				android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
					variables.Always_use_prebuilt_sdks = proptools.BoolPtr(tc.unbundledBuild)
					variables.BuildFlags = map[string]string{
						"RELEASE_HIDDEN_API_EXPORTABLE_STUBS": "true",
					}
				}),
			).RunTest(t)

			hiddenAPI := result.ModuleForTests("platform-bootclasspath", "android_common")
			hiddenapiRule := hiddenAPI.Rule("platform-bootclasspath-monolithic-hiddenapi-stub-flags")
			wantPublicStubs := "--public-stub-classpath=" + generateSdkDexPath(tc.publicStub, tc.unbundledBuild)
			android.AssertStringDoesContain(t, "hiddenapi command", hiddenapiRule.RuleParams.Command, wantPublicStubs)

			wantSystemStubs := "--system-stub-classpath=" + generateSdkDexPath(tc.systemStub, tc.unbundledBuild)
			android.AssertStringDoesContain(t, "hiddenapi command", hiddenapiRule.RuleParams.Command, wantSystemStubs)

			wantTestStubs := "--test-stub-classpath=" + generateSdkDexPath(tc.testStub, tc.unbundledBuild)
			android.AssertStringDoesContain(t, "hiddenapi command", hiddenapiRule.RuleParams.Command, wantTestStubs)

			wantCorePlatformStubs := "--core-platform-stub-classpath=" + generateDexPath(defaultJavaDir, tc.corePlatformStub)
			android.AssertStringDoesContain(t, "hiddenapi command", hiddenapiRule.RuleParams.Command, wantCorePlatformStubs)
		})
	}
}

func generateDexedPath(subDir, dex, module string) string {
	return fmt.Sprintf("out/soong/.intermediates/%s/android_common/%s/%s.jar", subDir, dex, module)
}

func generateDexPath(moduleDir string, module string) string {
	return generateDexedPath(filepath.Join(moduleDir, module), "dex", module)
}

func generateSdkDexPath(module string, unbundled bool) string {
	if unbundled {
		return generateDexedPath("prebuilts/sdk/"+module, "dex", module)
	}
	return generateDexPath(defaultJavaDir, module)
}

func TestHiddenAPISingletonWithPrebuiltCsvFile(t *testing.T) {

	// The idea behind this test is to ensure that when the build is
	// confugured with a PrebuiltHiddenApiDir that the rules for the
	// hiddenapi singleton copy the prebuilts to the typical output
	// location, and then use that output location for the hiddenapi encode
	// dex step.

	// Where to find the prebuilt hiddenapi files:
	prebuiltHiddenApiDir := "path/to/prebuilt/hiddenapi"

	result := android.GroupFixturePreparers(
		hiddenApiFixtureFactory,
		FixtureConfigureBootJars("platform:foo"),
		fixtureSetPrebuiltHiddenApiDirProductVariable(&prebuiltHiddenApiDir),
	).RunTestWithBp(t, `
		java_import {
			name: "foo",
			jars: ["a.jar"],
			compile_dex: true,
	}
	`)

	expectedCpInput := prebuiltHiddenApiDir + "/hiddenapi-flags.csv"
	expectedCpOutput := "out/soong/hiddenapi/hiddenapi-flags.csv"
	expectedFlagsCsv := "out/soong/hiddenapi/hiddenapi-flags.csv"

	foo := result.ModuleForTests("foo", "android_common")

	hiddenAPI := result.SingletonForTests("hiddenapi")
	cpRule := hiddenAPI.Rule("Cp")
	actualCpInput := cpRule.BuildParams.Input
	actualCpOutput := cpRule.BuildParams.Output
	encodeDexRule := foo.Rule("hiddenAPIEncodeDex")
	actualFlagsCsv := encodeDexRule.BuildParams.Args["flagsCsv"]

	android.AssertPathRelativeToTopEquals(t, "hiddenapi cp rule input", expectedCpInput, actualCpInput)

	android.AssertPathRelativeToTopEquals(t, "hiddenapi cp rule output", expectedCpOutput, actualCpOutput)

	android.AssertStringEquals(t, "hiddenapi encode dex rule flags csv", expectedFlagsCsv, actualFlagsCsv)
}

func TestHiddenAPIEncoding_JavaSdkLibrary(t *testing.T) {

	result := android.GroupFixturePreparers(
		hiddenApiFixtureFactory,
		FixtureConfigureBootJars("platform:foo"),
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("foo"),

		// Make sure that the frameworks/base/Android.bp file exists as otherwise hidden API encoding
		// is disabled.
		android.FixtureAddTextFile("frameworks/base/Android.bp", ""),
	).RunTestWithBp(t, `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			shared_library: false,
			compile_dex: true,
			public: {enabled: true},
		}
	`)

	checkDexEncoded := func(t *testing.T, name, unencodedDexJar, encodedDexJar string) {
		moduleForTests := result.ModuleForTests(name, "android_common")

		encodeDexRule := moduleForTests.Rule("hiddenAPIEncodeDex")
		actualUnencodedDexJar := encodeDexRule.Input

		// Make sure that the module has its dex jar encoded.
		android.AssertStringEquals(t, "encode embedded java_library", unencodedDexJar, actualUnencodedDexJar.String())

		// Make sure that the encoded dex jar is the exported one.
		errCtx := moduleErrorfTestCtx{}
		exportedDexJar := moduleForTests.Module().(UsesLibraryDependency).DexJarBuildPath(errCtx).Path()
		android.AssertPathRelativeToTopEquals(t, "encode embedded java_library", encodedDexJar, exportedDexJar)
	}

	// The java_library embedded with the java_sdk_library must be dex encoded.
	t.Run("foo", func(t *testing.T) {
		expectedUnencodedDexJar := "out/soong/.intermediates/foo/android_common/aligned/foo.jar"
		expectedEncodedDexJar := "out/soong/.intermediates/foo/android_common/hiddenapi/foo.jar"
		checkDexEncoded(t, "foo", expectedUnencodedDexJar, expectedEncodedDexJar)
	})

	// The dex jar of the child implementation java_library of the java_sdk_library is not currently
	// dex encoded.
	t.Run("foo.impl", func(t *testing.T) {
		fooImpl := result.ModuleForTests("foo.impl", "android_common")
		encodeDexRule := fooImpl.MaybeRule("hiddenAPIEncodeDex")
		if encodeDexRule.Rule != nil {
			t.Errorf("foo.impl is not expected to be encoded")
		}
	})
}
