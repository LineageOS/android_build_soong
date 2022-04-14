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

import (
	"android/soong/android/allowlists"
	"testing"
)

func TestConvertAllModulesInPackage(t *testing.T) {
	testCases := []struct {
		prefixes   allowlists.Bp2BuildConfig
		packageDir string
	}{
		{
			prefixes: allowlists.Bp2BuildConfig{
				"a": allowlists.Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a",
		},
		{
			prefixes: allowlists.Bp2BuildConfig{
				"a/b": allowlists.Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a/b",
		},
		{
			prefixes: allowlists.Bp2BuildConfig{
				"a/b":   allowlists.Bp2BuildDefaultTrueRecursively,
				"a/b/c": allowlists.Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a/b",
		},
		{
			prefixes: allowlists.Bp2BuildConfig{
				"a":     allowlists.Bp2BuildDefaultTrueRecursively,
				"d/e/f": allowlists.Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a/b",
		},
		{
			prefixes: allowlists.Bp2BuildConfig{
				"a":     allowlists.Bp2BuildDefaultFalse,
				"a/b":   allowlists.Bp2BuildDefaultTrueRecursively,
				"a/b/c": allowlists.Bp2BuildDefaultFalse,
			},
			packageDir: "a/b",
		},
		{
			prefixes: allowlists.Bp2BuildConfig{
				"a":     allowlists.Bp2BuildDefaultTrueRecursively,
				"a/b":   allowlists.Bp2BuildDefaultFalse,
				"a/b/c": allowlists.Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a",
		},
	}

	for _, test := range testCases {
		if ok, _ := bp2buildDefaultTrueRecursively(test.packageDir, test.prefixes); !ok {
			t.Errorf("Expected to convert all modules in %s based on %v, but failed.", test.packageDir, test.prefixes)
		}
	}
}

func TestModuleOptIn(t *testing.T) {
	testCases := []struct {
		prefixes   allowlists.Bp2BuildConfig
		packageDir string
	}{
		{
			prefixes: allowlists.Bp2BuildConfig{
				"a/b": allowlists.Bp2BuildDefaultFalse,
			},
			packageDir: "a/b",
		},
		{
			prefixes: allowlists.Bp2BuildConfig{
				"a":   allowlists.Bp2BuildDefaultFalse,
				"a/b": allowlists.Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a",
		},
		{
			prefixes: allowlists.Bp2BuildConfig{
				"a/b": allowlists.Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a", // opt-in by default
		},
		{
			prefixes: allowlists.Bp2BuildConfig{
				"a/b/c": allowlists.Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a/b",
		},
		{
			prefixes: allowlists.Bp2BuildConfig{
				"a":     allowlists.Bp2BuildDefaultTrueRecursively,
				"d/e/f": allowlists.Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "foo/bar",
		},
		{
			prefixes: allowlists.Bp2BuildConfig{
				"a":     allowlists.Bp2BuildDefaultTrueRecursively,
				"a/b":   allowlists.Bp2BuildDefaultFalse,
				"a/b/c": allowlists.Bp2BuildDefaultTrueRecursively,
			},
			packageDir: "a/b",
		},
		{
			prefixes: allowlists.Bp2BuildConfig{
				"a":     allowlists.Bp2BuildDefaultFalse,
				"a/b":   allowlists.Bp2BuildDefaultTrueRecursively,
				"a/b/c": allowlists.Bp2BuildDefaultFalse,
			},
			packageDir: "a",
		},
	}

	for _, test := range testCases {
		if ok, _ := bp2buildDefaultTrueRecursively(test.packageDir, test.prefixes); ok {
			t.Errorf("Expected to allow module opt-in in %s based on %v, but failed.", test.packageDir, test.prefixes)
		}
	}
}
