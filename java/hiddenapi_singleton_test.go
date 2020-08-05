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
	"strings"
	"testing"
)

func testConfigWithBootJars(bp string, bootJars []string) android.Config {
	config := testConfig(nil, bp, nil)
	config.TestProductVariables.BootJars = bootJars
	return config
}

func testContextWithHiddenAPI() *android.TestContext {
	ctx := testContext()
	ctx.RegisterSingletonType("hiddenapi", hiddenAPISingletonFactory)
	return ctx
}

func testHiddenAPI(t *testing.T, bp string, bootJars []string) (*android.TestContext, android.Config) {
	t.Helper()

	config := testConfigWithBootJars(bp, bootJars)
	ctx := testContextWithHiddenAPI()

	run(t, ctx, config)

	return ctx, config
}

func TestHiddenAPISingleton(t *testing.T) {
	ctx, _ := testHiddenAPI(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			compile_dex: true,
	}
	`, []string{"foo"})

	hiddenAPI := ctx.SingletonForTests("hiddenapi")
	hiddenapiRule := hiddenAPI.Rule("hiddenapi")
	want := "--boot-dex=" + buildDir + "/.intermediates/foo/android_common/aligned/foo.jar"
	if !strings.Contains(hiddenapiRule.RuleParams.Command, want) {
		t.Errorf("Expected %s in hiddenapi command, but it was not present: %s", want, hiddenapiRule.RuleParams.Command)
	}
}

func TestHiddenAPISingletonWithPrebuilt(t *testing.T) {
	ctx, _ := testHiddenAPI(t, `
		java_import {
			name: "foo",
			jars: ["a.jar"],
			compile_dex: true,
	}
	`, []string{"foo"})

	hiddenAPI := ctx.SingletonForTests("hiddenapi")
	hiddenapiRule := hiddenAPI.Rule("hiddenapi")
	want := "--boot-dex=" + buildDir + "/.intermediates/foo/android_common/dex/foo.jar"
	if !strings.Contains(hiddenapiRule.RuleParams.Command, want) {
		t.Errorf("Expected %s in hiddenapi command, but it was not present: %s", want, hiddenapiRule.RuleParams.Command)
	}
}

func TestHiddenAPISingletonWithPrebuiltUseSource(t *testing.T) {
	ctx, _ := testHiddenAPI(t, `
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
	`, []string{"foo"})

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
	ctx, _ := testHiddenAPI(t, `
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
	`, []string{"foo"})

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
