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

func fixtureSetBootJarsProductVariable(bootJars ...string) android.FixturePreparer {
	return android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
		variables.BootJars = android.CreateTestConfiguredJarList(bootJars)
	})
}

func fixtureSetPrebuiltHiddenApiDirProductVariable(prebuiltHiddenApiDir *string) android.FixturePreparer {
	return android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
		variables.PrebuiltHiddenApiDir = prebuiltHiddenApiDir
	})
}

var hiddenApiFixtureFactory = android.GroupFixturePreparers(
	prepareForJavaTest, PrepareForTestWithHiddenApiBuildComponents)

func TestHiddenAPISingleton(t *testing.T) {
	result := hiddenApiFixtureFactory.Extend(
		fixtureSetBootJarsProductVariable("platform:foo"),
	).RunTestWithBp(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			compile_dex: true,
		}
	`)

	hiddenAPI := result.SingletonForTests("hiddenapi")
	hiddenapiRule := hiddenAPI.Rule("hiddenapi").RelativeToTop()
	want := "--boot-dex=out/soong/.intermediates/foo/android_common/aligned/foo.jar"
	android.AssertStringDoesContain(t, "hiddenapi command", hiddenapiRule.RuleParams.Command, want)
}

func TestHiddenAPIIndexSingleton(t *testing.T) {
	result := hiddenApiFixtureFactory.Extend(
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("bar"),
		fixtureSetBootJarsProductVariable("platform:foo", "platform:bar"),
	).RunTestWithBp(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			compile_dex: true,

			hiddenapi_additional_annotations: [
				"foo-hiddenapi-annotations",
			],
		}

		java_library {
			name: "foo-hiddenapi-annotations",
			srcs: ["a.java"],
			compile_dex: true,
		}

		java_import {
			name: "foo",
			jars: ["a.jar"],
			compile_dex: true,
			prefer: false,
		}

		java_sdk_library {
			name: "bar",
			srcs: ["a.java"],
			compile_dex: true,
		}
	`)

	hiddenAPIIndex := result.SingletonForTests("hiddenapi_index")
	indexRule := hiddenAPIIndex.Rule("singleton-merged-hiddenapi-index")
	CheckHiddenAPIRuleInputs(t, `
.intermediates/bar/android_common/hiddenapi/index.csv
.intermediates/foo/android_common/hiddenapi/index.csv
`,
		indexRule)

	// Make sure that the foo-hiddenapi-annotations.jar is included in the inputs to the rules that
	// creates the index.csv file.
	foo := result.ModuleForTests("foo", "android_common")
	indexParams := foo.Output("hiddenapi/index.csv")
	CheckHiddenAPIRuleInputs(t, `
.intermediates/foo-hiddenapi-annotations/android_common/javac/foo-hiddenapi-annotations.jar
.intermediates/foo/android_common/javac/foo.jar
`, indexParams)
}

func TestHiddenAPISingletonWithSourceAndPrebuiltPreferredButNoDex(t *testing.T) {
	expectedErrorMessage :=
		"hiddenapi has determined that the source module \"foo\" should be ignored as it has been" +
			" replaced by the prebuilt module \"prebuilt_foo\" but unfortunately it does not provide a" +
			" suitable boot dex jar"

	hiddenApiFixtureFactory.Extend(
		fixtureSetBootJarsProductVariable("platform:foo"),
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
	result := hiddenApiFixtureFactory.Extend(
		fixtureSetBootJarsProductVariable("platform:foo"),
	).RunTestWithBp(t, `
		java_import {
			name: "foo",
			jars: ["a.jar"],
			compile_dex: true,
	}
	`)

	hiddenAPI := result.SingletonForTests("hiddenapi")
	hiddenapiRule := hiddenAPI.Rule("hiddenapi").RelativeToTop()
	want := "--boot-dex=out/soong/.intermediates/foo/android_common/aligned/foo.jar"
	android.AssertStringDoesContain(t, "hiddenapi command", hiddenapiRule.RuleParams.Command, want)
}

func TestHiddenAPISingletonWithPrebuiltUseSource(t *testing.T) {
	result := hiddenApiFixtureFactory.Extend(
		fixtureSetBootJarsProductVariable("platform:foo"),
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

	hiddenAPI := result.SingletonForTests("hiddenapi")
	hiddenapiRule := hiddenAPI.Rule("hiddenapi").RelativeToTop()
	fromSourceJarArg := "--boot-dex=out/soong/.intermediates/foo/android_common/aligned/foo.jar"
	android.AssertStringDoesContain(t, "hiddenapi command", hiddenapiRule.RuleParams.Command, fromSourceJarArg)

	prebuiltJarArg := "--boot-dex=out/soong/.intermediates/foo/android_common/dex/foo.jar"
	android.AssertStringDoesNotContain(t, "hiddenapi command", hiddenapiRule.RuleParams.Command, prebuiltJarArg)
}

func TestHiddenAPISingletonWithPrebuiltOverrideSource(t *testing.T) {
	result := hiddenApiFixtureFactory.Extend(
		fixtureSetBootJarsProductVariable("platform:foo"),
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

	hiddenAPI := result.SingletonForTests("hiddenapi")
	hiddenapiRule := hiddenAPI.Rule("hiddenapi").RelativeToTop()
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
			publicStub:       "android_stubs_current",
			systemStub:       "android_system_stubs_current",
			testStub:         "android_test_stubs_current",
			corePlatformStub: "legacy.core.platform.api.stubs",
			preparer:         android.GroupFixturePreparers(),
		}, {
			name:             "testUnbundled",
			unbundledBuild:   true,
			publicStub:       "sdk_public_current_android",
			systemStub:       "sdk_system_current_android",
			testStub:         "sdk_test_current_android",
			corePlatformStub: "legacy.core.platform.api.stubs",
			preparer:         PrepareForTestWithPrebuiltsOfCurrentApi,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := hiddenApiFixtureFactory.Extend(
				tc.preparer,
				android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
					variables.Always_use_prebuilt_sdks = proptools.BoolPtr(tc.unbundledBuild)
				}),
			).RunTest(t)

			hiddenAPI := result.SingletonForTests("hiddenapi")
			hiddenapiRule := hiddenAPI.Rule("hiddenapi").RelativeToTop()
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

	result := hiddenApiFixtureFactory.Extend(
		fixtureSetBootJarsProductVariable("platform:foo"),
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
	encodeDexRule := foo.Rule("hiddenAPIEncodeDex").RelativeToTop()
	actualFlagsCsv := encodeDexRule.BuildParams.Args["flagsCsv"]

	android.AssertPathRelativeToTopEquals(t, "hiddenapi cp rule input", expectedCpInput, actualCpInput)

	android.AssertPathRelativeToTopEquals(t, "hiddenapi cp rule output", expectedCpOutput, actualCpOutput)

	android.AssertStringEquals(t, "hiddenapi encode dex rule flags csv", expectedFlagsCsv, actualFlagsCsv)
}
