// Copyright 2017 Google Inc. All rights reserved.
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

import "testing"

func TestJacocoFilterToSpecs(t *testing.T) {
	testCases := []struct {
		name, in, out string
	}{
		{
			name: "class",
			in:   "package.Class",
			out:  "package/Class.class",
		},
		{
			name: "class wildcard",
			in:   "package.Class*",
			out:  "package/Class*.class",
		},
		{
			name: "package wildcard",
			in:   "package.*",
			out:  "package/*.class",
		},
		{
			name: "package recursive wildcard",
			in:   "package.**",
			out:  "package/**/*.class",
		},
		{
			name: "recursive wildcard only",
			in:   "**",
			out:  "**/*.class",
		},
		{
			name: "single wildcard only",
			in:   "*",
			out:  "*.class",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := jacocoFilterToSpec(testCase.in)
			if err != nil {
				t.Error(err)
			}
			if got != testCase.out {
				t.Errorf("expected %q got %q", testCase.out, got)
			}
		})
	}
}

func TestJacocoFiltersToZipCommand(t *testing.T) {
	testCases := []struct {
		name               string
		includes, excludes []string
		out                string
	}{
		{
			name:     "implicit wildcard",
			includes: []string{},
			out:      "**/*.class",
		},
		{
			name:     "only include",
			includes: []string{"package/Class.class"},
			out:      "package/Class.class",
		},
		{
			name:     "multiple includes",
			includes: []string{"package/Class.class", "package2/Class.class"},
			out:      "package/Class.class package2/Class.class",
		},
		{
			name:     "excludes",
			includes: []string{"package/**/*.class"},
			excludes: []string{"package/Class.class"},
			out:      "-x package/Class.class package/**/*.class",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := jacocoFiltersToZipCommand(testCase.includes, testCase.excludes)
			if got != testCase.out {
				t.Errorf("expected %q got %q", testCase.out, got)
			}
		})
	}
}
