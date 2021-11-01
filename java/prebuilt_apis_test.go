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

package java

import (
	"sort"
	"strings"
	"testing"

	"android/soong/android"
	"github.com/google/blueprint"
)

func TestPrebuiltApis_SystemModulesCreation(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForJavaTest,
		FixtureWithPrebuiltApis(map[string][]string{
			"31":      {},
			"current": {},
		}),
	).RunTest(t)

	sdkSystemModules := []string{}
	result.VisitAllModules(func(module blueprint.Module) {
		name := android.RemoveOptionalPrebuiltPrefix(module.Name())
		if strings.HasPrefix(name, "sdk_") && strings.HasSuffix(name, "_system_modules") {
			sdkSystemModules = append(sdkSystemModules, name)
		}
	})
	sort.Strings(sdkSystemModules)
	expected := []string{
		// 31 only has public system modules.
		"sdk_public_31_system_modules",

		// current only has public system modules.
		"sdk_public_current_system_modules",
	}
	sort.Strings(expected)
	android.AssertArrayString(t, "sdk system modules", expected, sdkSystemModules)
}
