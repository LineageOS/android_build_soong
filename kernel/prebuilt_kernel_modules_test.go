// Copyright 2021 Google Inc. All rights reserved.
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

package kernel

import (
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/cc"
)

func testKernelModules(t *testing.T, bp string, fs map[string][]byte) (*android.TestContext, android.Config) {
	bp = bp + `
		cc_binary_host {
			name: "depmod",
			srcs: ["depmod.cpp"],
			stl: "none",
			static_executable: true,
			system_shared_libs: [],
		}
	`
	bp = bp + cc.GatherRequiredDepsForTest(android.Android)

	fs["depmod.cpp"] = nil
	cc.GatherRequiredFilesForTest(fs)

	config := android.TestArchConfig(buildDir, nil, bp, fs)

	ctx := android.NewTestArchContext(config)
	ctx.RegisterModuleType("prebuilt_kernel_modules", prebuiltKernelModulesFactory)
	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	cc.RegisterRequiredBuildComponentsForTest(ctx)

	ctx.Register()
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	return ctx, config
}

func ensureListContains(t *testing.T, result []string, expected string) {
	t.Helper()
	if !android.InList(expected, result) {
		t.Errorf("%q is not found in %v", expected, result)
	}
}

func ensureContains(t *testing.T, result string, expected string) {
	t.Helper()
	if !strings.Contains(result, expected) {
		t.Errorf("%q is not found in %q", expected, result)
	}
}

func TestKernelModulesFilelist(t *testing.T) {
	ctx, _ := testKernelModules(t, `
		prebuilt_kernel_modules {
			name: "foo",
			srcs: ["*.ko"],
			kernel_version: "5.10",
		}
	`,
		map[string][]byte{
			"mod1.ko": nil,
			"mod2.ko": nil,
		})

	expected := []string{
		"lib/modules/5.10/mod1.ko",
		"lib/modules/5.10/mod2.ko",
		"lib/modules/5.10/modules.load",
		"lib/modules/5.10/modules.dep",
		"lib/modules/5.10/modules.softdep",
		"lib/modules/5.10/modules.alias",
	}

	var actual []string
	for _, ps := range ctx.ModuleForTests("foo", "android_arm64_armv8-a").Module().PackagingSpecs() {
		actual = append(actual, ps.RelPathInPackage())
	}
	actual = android.SortedUniqueStrings(actual)
	expected = android.SortedUniqueStrings(expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("\ngot: %v\nexpected: %v\n", actual, expected)
	}
}

var buildDir string

func setUp() {
	var err error
	buildDir, err = ioutil.TempDir("", "soong_kernel_test")
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
