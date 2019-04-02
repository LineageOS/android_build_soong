// Copyright 2019 Google Inc. All rights reserved.
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

package main

import (
	"testing"
)

func TestMatch(t *testing.T) {
	testCases := []struct {
		pattern, name string
		match         bool
	}{
		{"a/*", "b/", false},
		{"a/*", "b/a", false},
		{"a/*", "b/b/", false},
		{"a/*", "b/b/c", false},
		{"a/**/*", "b/", false},
		{"a/**/*", "b/a", false},
		{"a/**/*", "b/b/", false},
		{"a/**/*", "b/b/c", false},

		{"a/*", "a/", false},
		{"a/*", "a/a", true},
		{"a/*", "a/b/", false},
		{"a/*", "a/b/c", false},

		{"a/*/", "a/", false},
		{"a/*/", "a/a", false},
		{"a/*/", "a/b/", true},
		{"a/*/", "a/b/c", false},

		{"a/**/*", "a/", false},
		{"a/**/*", "a/a", true},
		{"a/**/*", "a/b/", false},
		{"a/**/*", "a/b/c", true},

		{"a/**/*/", "a/", false},
		{"a/**/*/", "a/a", false},
		{"a/**/*/", "a/b/", true},
		{"a/**/*/", "a/b/c", false},

		{"**/*", "a/", false},
		{"**/*", "a/a", true},
		{"**/*", "a/b/", false},
		{"**/*", "a/b/c", true},

		{"**/*/", "a/", true},
		{"**/*/", "a/a", false},
		{"**/*/", "a/b/", true},
		{"**/*/", "a/b/c", false},

		{`a/\*\*/\*`, `a/**/*`, true},
		{`a/\*\*/\*`, `a/a/*`, false},
		{`a/\*\*/\*`, `a/**/a`, false},
		{`a/\*\*/\*`, `a/a/a`, false},

		{`a/**/\*`, `a/**/*`, true},
		{`a/**/\*`, `a/a/*`, true},
		{`a/**/\*`, `a/**/a`, false},
		{`a/**/\*`, `a/a/a`, false},

		{`a/\*\*/*`, `a/**/*`, true},
		{`a/\*\*/*`, `a/a/*`, false},
		{`a/\*\*/*`, `a/**/a`, true},
		{`a/\*\*/*`, `a/a/a`, false},

		{`*/**/a`, `a/a/a`, true},
		{`*/**/a`, `*/a/a`, true},
		{`*/**/a`, `a/**/a`, true},
		{`*/**/a`, `*/**/a`, true},

		{`\*/\*\*/a`, `a/a/a`, false},
		{`\*/\*\*/a`, `*/a/a`, false},
		{`\*/\*\*/a`, `a/**/a`, false},
		{`\*/\*\*/a`, `*/**/a`, true},

		{`a/?`, `a/?`, true},
		{`a/?`, `a/a`, true},
		{`a/\?`, `a/?`, true},
		{`a/\?`, `a/a`, false},

		{`a/?`, `a/?`, true},
		{`a/?`, `a/a`, true},
		{`a/\?`, `a/?`, true},
		{`a/\?`, `a/a`, false},

		{`a/[a-c]`, `a/b`, true},
		{`a/[abc]`, `a/b`, true},

		{`a/\[abc]`, `a/b`, false},
		{`a/\[abc]`, `a/[abc]`, true},

		{`a/\[abc\]`, `a/b`, false},
		{`a/\[abc\]`, `a/[abc]`, true},

		{`a/?`, `a/?`, true},
		{`a/?`, `a/a`, true},
		{`a/\?`, `a/?`, true},
		{`a/\?`, `a/a`, false},

		{"/a/*", "/a/", false},
		{"/a/*", "/a/a", true},
		{"/a/*", "/a/b/", false},
		{"/a/*", "/a/b/c", false},

		{"/a/*/", "/a/", false},
		{"/a/*/", "/a/a", false},
		{"/a/*/", "/a/b/", true},
		{"/a/*/", "/a/b/c", false},

		{"/a/**/*", "/a/", false},
		{"/a/**/*", "/a/a", true},
		{"/a/**/*", "/a/b/", false},
		{"/a/**/*", "/a/b/c", true},

		{"/**/*", "/a/", false},
		{"/**/*", "/a/a", true},
		{"/**/*", "/a/b/", false},
		{"/**/*", "/a/b/c", true},

		{"/**/*/", "/a/", true},
		{"/**/*/", "/a/a", false},
		{"/**/*/", "/a/b/", true},
		{"/**/*/", "/a/b/c", false},

		{`a`, `/a`, false},
		{`/a`, `a`, false},
		{`*`, `/a`, false},
		{`/*`, `a`, false},
		{`**/*`, `/a`, false},
		{`/**/*`, `a`, false},
	}

	for _, test := range testCases {
		t.Run(test.pattern+","+test.name, func(t *testing.T) {
			match, err := Match(test.pattern, test.name)
			if err != nil {
				t.Fatal(err)
			}
			if match != test.match {
				t.Errorf("want: %v, got %v", test.match, match)
			}
		})
	}
}
