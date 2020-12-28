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
	"android/soong/android"
	"fmt"
	"strings"
	"testing"

	"github.com/google/blueprint/proptools"
)

func testConfigWithBootJars(bp string, bootJars []string) android.Config {
	config := testConfig(nil, bp, nil)
	config.TestProductVariables.BootJars = android.CreateTestConfiguredJarList(bootJars)
	return config
}

func testContextWithHiddenAPI(config android.Config) *android.TestContext {
	ctx := testContext(config)
	ctx.RegisterSingletonType("hiddenapi", hiddenAPISingletonFactory)
	return ctx
}

func testHiddenAPIWithConfig(t *testing.T, config android.Config) *android.TestContext {
	t.Helper()

	ctx := testContextWithHiddenAPI(config)

	run(t, ctx, config)
	return ctx
}

func testHiddenAPIBootJars(t *testing.T, bp string, bootJars []string) (*android.TestContext, android.Config) {
	config := testConfigWithBootJars(bp, bootJars)

	return testHiddenAPIWithConfig(t, config), config
}

func testHiddenAPIUnbundled(t *testing.T, unbundled bool) (*android.TestContext, android.Config) {
	config := testConfig(nil, ``, nil)
	config.TestProductVariables.Always_use_prebuilt_sdks = proptools.BoolPtr(unbundled)

	return testHiddenAPIWithConfig(t, config), config
}

func TestHiddenAPISingleton(t *testing.T) {
	ctx, _ := testHiddenAPIBootJars(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			compile_dex: true,
	}
	`, []string{":foo"})

	hiddenAPI := ctx.SingletonForTests("hiddenapi")
	hiddenapiRule := hiddenAPI.Rule("hiddenapi")
	want := "--boot-dex=" + buildDir + "/.intermediates/foo/android_common/aligned/foo.jar"
	if !strings.Contains(hiddenapiRule.RuleParams.Command, want) {
		t.Errorf("Expected %s in hiddenapi command, but it was not present: %s", want, hiddenapiRule.RuleParams.Command)
	}
}

func TestHiddenAPISingletonWithPrebuilt(t *testing.T) {
	ctx, _ := testHiddenAPIBootJars(t, `
		java_import {
			name: "foo",
			jars: ["a.jar"],
			compile_dex: true,
	}
	`, []string{":foo"})

	hiddenAPI := ctx.SingletonForTests("hiddenapi")
	hiddenapiRule := hiddenAPI.Rule("hiddenapi")
	want := "--boot-dex=" + buildDir + "/.intermediates/foo/android_common/aligned/foo.jar"
	if !strings.Contains(hiddenapiRule.RuleParams.Command, want) {
		t.Errorf("Expected %s in hiddenapi command, but it was not present: %s", want, hiddenapiRule.RuleParams.Command)
	}
}

func TestHiddenAPISingletonWithPrebuiltUseSource(t *testing.T) {
	ctx, _ := testHiddenAPIBootJars(t, `
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
	`, []string{":foo"})

	hiddenAPI := ctx.SingletonForTests("hiddenapi")
	hiddenapiRule := hiddenAPI.Rule("hiddenapi")
	fromSourceJarArg := "--boot-dex=" + buildDir + "/.intermediates/foo/android_common/aligned/foo.jar"
	if !strings.Contains(hiddenapiRule.RuleParams.Command, fromSourceJarArg) {
		t.Errorf("Expected %s in hiddenapi command, but it was not present: %s", fromSourceJarArg, hiddenapiRule.RuleParams.Command)
	}

	prebuiltJarArg := "--boot-dex=" + buildDir + "/.intermediates/foo/android_common/dex/foo.jar"
	if strings.Contains(hiddenapiRule.RuleParams.Command, prebuiltJarArg) {
		t.Errorf("Did not expect %s in hiddenapi command, but it was present: %s", prebuiltJarArg, hiddenapiRule.RuleParams.Command)
	}
}

func TestHiddenAPISingletonWithPrebuiltOverrideSource(t *testing.T) {
	ctx, _ := testHiddenAPIBootJars(t, `
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
	`, []string{":foo"})

	hiddenAPI := ctx.SingletonForTests("hiddenapi")
	hiddenapiRule := hiddenAPI.Rule("hiddenapi")
	prebuiltJarArg := "--boot-dex=" + buildDir + "/.intermediates/prebuilt_foo/android_common/dex/foo.jar"
	if !strings.Contains(hiddenapiRule.RuleParams.Command, prebuiltJarArg) {
		t.Errorf("Expected %s in hiddenapi command, but it was not present: %s", prebuiltJarArg, hiddenapiRule.RuleParams.Command)
	}

	fromSourceJarArg := "--boot-dex=" + buildDir + "/.intermediates/foo/android_common/aligned/foo.jar"
	if strings.Contains(hiddenapiRule.RuleParams.Command, fromSourceJarArg) {
		t.Errorf("Did not expect %s in hiddenapi command, but it was present: %s", fromSourceJarArg, hiddenapiRule.RuleParams.Command)
	}
}

func TestHiddenAPISingletonSdks(t *testing.T) {
	testCases := []struct {
		name             string
		unbundledBuild   bool
		publicStub       string
		systemStub       string
		testStub         string
		corePlatformStub string
	}{
		{
			name:             "testBundled",
			unbundledBuild:   false,
			publicStub:       "android_stubs_current",
			systemStub:       "android_system_stubs_current",
			testStub:         "android_test_stubs_current",
			corePlatformStub: "legacy.core.platform.api.stubs",
		}, {
			name:             "testUnbundled",
			unbundledBuild:   true,
			publicStub:       "sdk_public_current_android",
			systemStub:       "sdk_system_current_android",
			testStub:         "sdk_test_current_android",
			corePlatformStub: "legacy.core.platform.api.stubs",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := testHiddenAPIUnbundled(t, tc.unbundledBuild)

			hiddenAPI := ctx.SingletonForTests("hiddenapi")
			hiddenapiRule := hiddenAPI.Rule("hiddenapi")
			wantPublicStubs := "--public-stub-classpath=" + generateSdkDexPath(tc.publicStub, tc.unbundledBuild)
			if !strings.Contains(hiddenapiRule.RuleParams.Command, wantPublicStubs) {
				t.Errorf("Expected %s in hiddenapi command, but it was not present: %s", wantPublicStubs, hiddenapiRule.RuleParams.Command)
			}

			wantSystemStubs := "--system-stub-classpath=" + generateSdkDexPath(tc.systemStub, tc.unbundledBuild)
			if !strings.Contains(hiddenapiRule.RuleParams.Command, wantSystemStubs) {
				t.Errorf("Expected %s in hiddenapi command, but it was not present: %s", wantSystemStubs, hiddenapiRule.RuleParams.Command)
			}

			wantTestStubs := "--test-stub-classpath=" + generateSdkDexPath(tc.testStub, tc.unbundledBuild)
			if !strings.Contains(hiddenapiRule.RuleParams.Command, wantTestStubs) {
				t.Errorf("Expected %s in hiddenapi command, but it was not present: %s", wantTestStubs, hiddenapiRule.RuleParams.Command)
			}

			wantCorePlatformStubs := "--core-platform-stub-classpath=" + generateDexPath(tc.corePlatformStub)
			if !strings.Contains(hiddenapiRule.RuleParams.Command, wantCorePlatformStubs) {
				t.Errorf("Expected %s in hiddenapi command, but it was not present: %s", wantCorePlatformStubs, hiddenapiRule.RuleParams.Command)
			}
		})
	}
}

func generateDexedPath(subDir, dex, module string) string {
	return fmt.Sprintf("%s/.intermediates/%s/android_common/%s/%s.jar", buildDir, subDir, dex, module)
}

func generateDexPath(module string) string {
	return generateDexedPath(module, "dex", module)
}

func generateSdkDexPath(module string, unbundled bool) string {
	if unbundled {
		return generateDexedPath("prebuilts/sdk/"+module, "dex", module)
	}
	return generateDexPath(module)
}
