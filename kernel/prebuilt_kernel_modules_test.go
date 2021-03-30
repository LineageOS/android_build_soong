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
	"os"
	"testing"

	"android/soong/android"
	"android/soong/cc"
)

func TestKernelModulesFilelist(t *testing.T) {
	ctx := android.GroupFixturePreparers(
		cc.PrepareForTestWithCcDefaultModules,
		android.FixtureRegisterWithContext(registerKernelBuildComponents),
		android.MockFS{
			"depmod.cpp": nil,
			"mod1.ko":    nil,
			"mod2.ko":    nil,
		}.AddToFixture(),
	).RunTestWithBp(t, `
		prebuilt_kernel_modules {
			name: "foo",
			srcs: ["*.ko"],
			kernel_version: "5.10",
		}
	`)

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
	android.AssertDeepEquals(t, "foo packaging specs", expected, actual)
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
