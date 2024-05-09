// Copyright 2024 Google Inc. All rights reserved.
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

package cc

import (
	"strings"
	"testing"

	"android/soong/android"
)

func wasGenerated(t *testing.T, m *android.TestingModule, fileName string, ruleType string) {
	t.Helper()
	ruleName := m.Output(fileName).Rule.String()
	if !strings.HasSuffix(ruleName, ruleType) {
		t.Errorf("Main Cmake file wasn't generated, expected rule %v, found %v", ruleType, ruleName)
	}
}

func TestEmptyCmakeSnapshot(t *testing.T) {
	t.Parallel()
	result := PrepareForIntegrationTestWithCc.RunTestWithBp(t, `
		cc_cmake_snapshot {
			name: "foo",
			modules: [],
			prebuilts: ["libc++"],
			include_sources: true,
		}`)

	snapshotModule := result.ModuleForTests("foo", "")

	wasGenerated(t, &snapshotModule, "CMakeLists.txt", "rawFileCopy")
	wasGenerated(t, &snapshotModule, "foo.zip", "")
}

func TestCmakeSnapshotWithBinary(t *testing.T) {
	t.Parallel()
	xtra := android.FixtureAddTextFile("some/module/Android.bp", `
		cc_binary {
			name: "foo_binary",
			host_supported: true,
			cmake_snapshot_supported: true,
		}
	`)
	result := android.GroupFixturePreparers(PrepareForIntegrationTestWithCc, xtra).RunTestWithBp(t, `
		cc_cmake_snapshot {
			name: "foo",
			modules: [
				"foo_binary",
			],
			include_sources: true,
		}`)

	snapshotModule := result.ModuleForTests("foo", "")

	wasGenerated(t, &snapshotModule, "some/module/CMakeLists.txt", "rawFileCopy")
}
