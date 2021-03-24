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
package android

import "testing"

func TestConvertAllModulesInPackage(t *testing.T) {
	testCases := []struct {
		prefixes   Bp2BuildConfig
		packageDir string
	}{
		{
			prefixes: Bp2BuildConfig{
				"a": Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a",
		},
		{
			prefixes: Bp2BuildConfig{
				"a/b": Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a/b",
		},
		{
			prefixes: Bp2BuildConfig{
				"a/b":   Bp2BuildDefaultTrueRecursively,
				"a/b/c": Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a/b",
		},
		{
			prefixes: Bp2BuildConfig{
				"a":     Bp2BuildDefaultTrueRecursively,
				"d/e/f": Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a/b",
		},
		{
			prefixes: Bp2BuildConfig{
				"a":     Bp2BuildDefaultFalse,
				"a/b":   Bp2BuildDefaultTrueRecursively,
				"a/b/c": Bp2BuildDefaultFalse,
			},
			packageDir: "a/b",
		},
		{
			prefixes: Bp2BuildConfig{
				"a":     Bp2BuildDefaultTrueRecursively,
				"a/b":   Bp2BuildDefaultFalse,
				"a/b/c": Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a",
		},
	}

	for _, test := range testCases {
		if !bp2buildDefaultTrueRecursively(test.packageDir, test.prefixes) {
			t.Errorf("Expected to convert all modules in %s based on %v, but failed.", test.packageDir, test.prefixes)
		}
	}
}

func TestModuleOptIn(t *testing.T) {
	testCases := []struct {
		prefixes   Bp2BuildConfig
		packageDir string
	}{
		{
			prefixes: Bp2BuildConfig{
				"a/b": Bp2BuildDefaultFalse,
			},
			packageDir: "a/b",
		},
		{
			prefixes: Bp2BuildConfig{
				"a":   Bp2BuildDefaultFalse,
				"a/b": Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a",
		},
		{
			prefixes: Bp2BuildConfig{
				"a/b": Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a", // opt-in by default
		},
		{
			prefixes: Bp2BuildConfig{
				"a/b/c": Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a/b",
		},
		{
			prefixes: Bp2BuildConfig{
				"a":     Bp2BuildDefaultTrueRecursively,
				"d/e/f": Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "foo/bar",
		},
		{
			prefixes: Bp2BuildConfig{
				"a":     Bp2BuildDefaultTrueRecursively,
				"a/b":   Bp2BuildDefaultFalse,
				"a/b/c": Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a/b",
		},
		{
			prefixes: Bp2BuildConfig{
				"a":     Bp2BuildDefaultFalse,
				"a/b":   Bp2BuildDefaultTrueRecursively,
				"a/b/c": Bp2BuildDefaultFalse,
			},
			packageDir: "a",
		},
	}

	for _, test := range testCases {
		if bp2buildDefaultTrueRecursively(test.packageDir, test.prefixes) {
			t.Errorf("Expected to allow module opt-in in %s based on %v, but failed.", test.packageDir, test.prefixes)
		}
	}
}
