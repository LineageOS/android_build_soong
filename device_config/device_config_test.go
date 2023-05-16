// Copyright (C) 2019 The Android Open Source Project
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

package device_config

import (
	"os"
	"testing"

	"android/soong/android"
	"android/soong/java"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func test(t *testing.T, bp string) *android.TestResult {
	t.Helper()

	mockFS := android.MockFS{
		"config.aconfig": nil,
	}

	result := android.GroupFixturePreparers(
		java.PrepareForTestWithJavaDefaultModules,
		PrepareForTestWithSyspropBuildComponents,
		// TODO: Consider values files, although maybe in its own test
		// android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
		//	variables.ReleaseConfigValuesBasePaths = ...
		//})
		mockFS.AddToFixture(),
		android.FixtureWithRootAndroidBp(bp),
	).RunTest(t)

	return result
}

func TestOutputs(t *testing.T) {
	/*result := */ test(t, `
        device_config {
            name: "my_device_config",
            srcs: ["config.aconfig"],
        }
	`)

	// TODO: Make sure it exports a .srcjar, which is used by java libraries
	// TODO: Make sure it exports an intermediates file
	// TODO: Make sure the intermediates file is propagated to the Android.mk file
}
