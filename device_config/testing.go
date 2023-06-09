// Copyright (C) 2021 The Android Open Source Project
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
	"testing"

	"android/soong/android"
)

var PrepareForTestWithDeviceConfigBuildComponents = android.FixtureRegisterWithContext(registerBuildComponents)

func runTest(t *testing.T, errorHandler android.FixtureErrorHandler, bp string) *android.TestResult {
	return android.GroupFixturePreparers(PrepareForTestWithDeviceConfigBuildComponents).
		ExtendWithErrorHandler(errorHandler).
		RunTestWithBp(t, bp)
}
