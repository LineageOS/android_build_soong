// Copyright 2023 Google Inc. All rights reserved.
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

package bp2build

import (
	"regexp"
	"testing"

	"android/soong/java"
)

func TestDroiddocExportedDir(t *testing.T) {
	bp := `
	droiddoc_exported_dir {
		name: "test-module",
		path: "docs",
	}
	`
	p := regexp.MustCompile(`\t*\|`)
	dedent := func(s string) string {
		return p.ReplaceAllString(s, "")
	}
	expectedBazelTargets := []string{
		MakeBazelTargetNoRestrictions(
			"droiddoc_exported_dir",
			"test-module",
			AttrNameToString{
				"dir": `"docs"`,
				"srcs": dedent(`[
				|        "docs/android/1.txt",
				|        "docs/android/nested-1/2.txt",
				|        "//docs/android/nested-2:3.txt",
				|        "//docs/android/nested-2:Android.bp",
				|    ]`),
			}),
		//note we are not excluding Android.bp files from subpackages for now
	}
	RunBp2BuildTestCase(t, java.RegisterDocsBuildComponents, Bp2buildTestCase{
		Blueprint:            bp,
		ExpectedBazelTargets: expectedBazelTargets,
		Filesystem: map[string]string{
			"docs/android/1.txt":               "",
			"docs/android/nested-1/2.txt":      "",
			"docs/android/nested-2/Android.bp": "",
			"docs/android/nested-2/3.txt":      "",
		},
	})
}
