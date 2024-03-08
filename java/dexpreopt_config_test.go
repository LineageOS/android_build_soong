// Copyright 2022 Google Inc. All rights reserved.
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
	"runtime"
	"sort"
	"testing"

	"android/soong/android"
)

func TestBootImageConfig(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("Skipping as boot image config test is only supported on linux not %s", runtime.GOOS)
	}

	result := android.GroupFixturePreparers(
		PrepareForBootImageConfigTest,
		PrepareApexBootJarConfigs,
	).RunTest(t)

	CheckArtBootImageConfig(t, result)
	CheckFrameworkBootImageConfig(t, result)
	CheckMainlineBootImageConfig(t, result)
}

func TestImageNames(t *testing.T) {
	result := android.GroupFixturePreparers(
		PrepareForBootImageConfigTest,
	).RunTest(t)

	names := getImageNames()
	sort.Strings(names)

	ctx := &android.TestPathContext{TestResult: result}
	configs := genBootImageConfigs(ctx)
	namesFromConfigs := make([]string, 0, len(configs))
	for name, _ := range configs {
		namesFromConfigs = append(namesFromConfigs, name)
	}
	sort.Strings(namesFromConfigs)

	android.AssertArrayString(t, "getImageNames vs genBootImageConfigs", names, namesFromConfigs)
}
