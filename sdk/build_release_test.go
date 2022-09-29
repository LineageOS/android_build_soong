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

package sdk

import (
	"encoding/json"
	"fmt"
	"testing"

	"android/soong/android"
)

// Tests for build_release.go

var (
	// Some additional test specific releases that are added after the currently supported ones and
	// so are treated as being for future releases.
	buildReleaseFuture1 = initBuildRelease("F1")
	buildReleaseFuture2 = initBuildRelease("F2")
)

func TestNameToRelease(t *testing.T) {
	t.Run("single release", func(t *testing.T) {
		release, err := nameToRelease("S")
		android.AssertDeepEquals(t, "errors", nil, err)
		android.AssertDeepEquals(t, "release", buildReleaseS, release)
	})
	t.Run("invalid release", func(t *testing.T) {
		release, err := nameToRelease("A")
		android.AssertDeepEquals(t, "release", (*buildRelease)(nil), release)
		// Uses a wildcard in the error message to allow for additional build releases to be added to
		// the supported set without breaking this test.
		android.FailIfNoMatchingErrors(t, `unknown release "A", expected one of \[S,Tiramisu,UpsideDownCake,F1,F2,current\]`, []error{err})
	})
}

func TestParseBuildReleaseSet(t *testing.T) {
	t.Run("single release", func(t *testing.T) {
		set, err := parseBuildReleaseSet("S")
		android.AssertDeepEquals(t, "errors", nil, err)
		android.AssertStringEquals(t, "set", "[S]", set.String())
	})
	t.Run("open range", func(t *testing.T) {
		set, err := parseBuildReleaseSet("F1+")
		android.AssertDeepEquals(t, "errors", nil, err)
		android.AssertStringEquals(t, "set", "[F1,F2,current]", set.String())
	})
	t.Run("closed range", func(t *testing.T) {
		set, err := parseBuildReleaseSet("S-F1")
		android.AssertDeepEquals(t, "errors", nil, err)
		android.AssertStringEquals(t, "set", "[S,Tiramisu,UpsideDownCake,F1]", set.String())
	})
	invalidAReleaseMessage := `unknown release "A", expected one of ` + allBuildReleaseSet.String()
	t.Run("invalid release", func(t *testing.T) {
		set, err := parseBuildReleaseSet("A")
		android.AssertDeepEquals(t, "set", (*buildReleaseSet)(nil), set)
		android.AssertStringDoesContain(t, "errors", fmt.Sprint(err), invalidAReleaseMessage)
	})
	t.Run("invalid release in open range", func(t *testing.T) {
		set, err := parseBuildReleaseSet("A+")
		android.AssertDeepEquals(t, "set", (*buildReleaseSet)(nil), set)
		android.AssertStringDoesContain(t, "errors", fmt.Sprint(err), invalidAReleaseMessage)
	})
	t.Run("invalid release in closed range start", func(t *testing.T) {
		set, err := parseBuildReleaseSet("A-S")
		android.AssertDeepEquals(t, "set", (*buildReleaseSet)(nil), set)
		android.AssertStringDoesContain(t, "errors", fmt.Sprint(err), invalidAReleaseMessage)
	})
	t.Run("invalid release in closed range end", func(t *testing.T) {
		set, err := parseBuildReleaseSet("Tiramisu-A")
		android.AssertDeepEquals(t, "set", (*buildReleaseSet)(nil), set)
		android.AssertStringDoesContain(t, "errors", fmt.Sprint(err), invalidAReleaseMessage)
	})
	t.Run("invalid closed range reversed", func(t *testing.T) {
		set, err := parseBuildReleaseSet("F1-S")
		android.AssertDeepEquals(t, "set", (*buildReleaseSet)(nil), set)
		android.AssertStringDoesContain(t, "errors", fmt.Sprint(err), `invalid closed range, start release "F1" is later than end release "S"`)
	})
}

func TestBuildReleaseSetContains(t *testing.T) {
	t.Run("contains", func(t *testing.T) {
		set, _ := parseBuildReleaseSet("F1-F2")
		android.AssertBoolEquals(t, "set contains F1", true, set.contains(buildReleaseFuture1))
		android.AssertBoolEquals(t, "set does not contain S", false, set.contains(buildReleaseS))
		android.AssertBoolEquals(t, "set contains F2", true, set.contains(buildReleaseFuture2))
		android.AssertBoolEquals(t, "set does not contain T", false, set.contains(buildReleaseT))
	})
}

func TestPropertyPrunerInvalidTag(t *testing.T) {
	type brokenStruct struct {
		Broken string `supported_build_releases:"A"`
	}
	type containingStruct struct {
		Nested brokenStruct
	}

	t.Run("broken struct", func(t *testing.T) {
		android.AssertPanicMessageContains(t, "error", "invalid `supported_build_releases` tag on Broken of *sdk.brokenStruct: unknown release \"A\"", func() {
			newPropertyPrunerByBuildRelease(&brokenStruct{}, buildReleaseS)
		})
	})

	t.Run("nested broken struct", func(t *testing.T) {
		android.AssertPanicMessageContains(t, "error", "invalid `supported_build_releases` tag on Nested.Broken of *sdk.containingStruct: unknown release \"A\"", func() {
			newPropertyPrunerByBuildRelease(&containingStruct{}, buildReleaseS)
		})
	})
}

func TestPropertyPrunerByBuildRelease(t *testing.T) {
	type nested struct {
		F1_only string `supported_build_releases:"F1"`
	}

	type mapped struct {
		Default string
		T_only  string `supported_build_releases:"Tiramisu"`
	}

	type testBuildReleasePruner struct {
		Default      string
		S_and_T_only string `supported_build_releases:"S-Tiramisu"`
		T_later      string `supported_build_releases:"Tiramisu+"`
		Nested       nested
		Mapped       map[string]*mapped
	}

	inputFactory := func() testBuildReleasePruner {
		return testBuildReleasePruner{
			Default:      "Default",
			S_and_T_only: "S_and_T_only",
			T_later:      "T_later",
			Nested: nested{
				F1_only: "F1_only",
			},
			Mapped: map[string]*mapped{
				"one": {
					Default: "one-default",
					T_only:  "one-t-only",
				},
				"two": {
					Default: "two-default",
					T_only:  "two-t-only",
				},
			},
		}
	}

	marshal := func(t interface{}) string {
		bytes, err := json.MarshalIndent(t, "", "  ")
		if err != nil {
			panic(err)
		}
		return string(bytes)
	}

	assertJsonEquals := func(t *testing.T, expected, actual interface{}) {
		t.Helper()
		expectedJson := marshal(expected)
		actualJson := marshal(actual)
		if actualJson != expectedJson {
			t.Errorf("test struct: expected:\n%s\n got:\n%s", expectedJson, actualJson)
		}
	}

	t.Run("target S", func(t *testing.T) {
		testStruct := inputFactory()
		pruner := newPropertyPrunerByBuildRelease(&testStruct, buildReleaseS)
		pruner.pruneProperties(&testStruct)

		expected := inputFactory()
		expected.T_later = ""
		expected.Nested.F1_only = ""
		expected.Mapped["one"].T_only = ""
		expected.Mapped["two"].T_only = ""
		assertJsonEquals(t, expected, testStruct)
	})

	t.Run("target T", func(t *testing.T) {
		testStruct := inputFactory()
		pruner := newPropertyPrunerByBuildRelease(&testStruct, buildReleaseT)
		pruner.pruneProperties(&testStruct)

		expected := inputFactory()
		expected.Nested.F1_only = ""
		assertJsonEquals(t, expected, testStruct)
	})

	t.Run("target F1", func(t *testing.T) {
		testStruct := inputFactory()
		pruner := newPropertyPrunerByBuildRelease(&testStruct, buildReleaseFuture1)
		pruner.pruneProperties(&testStruct)

		expected := inputFactory()
		expected.S_and_T_only = ""
		expected.Mapped["one"].T_only = ""
		expected.Mapped["two"].T_only = ""
		assertJsonEquals(t, expected, testStruct)
	})

	t.Run("target F2", func(t *testing.T) {
		testStruct := inputFactory()
		pruner := newPropertyPrunerByBuildRelease(&testStruct, buildReleaseFuture2)
		pruner.pruneProperties(&testStruct)

		expected := inputFactory()
		expected.S_and_T_only = ""
		expected.Nested.F1_only = ""
		expected.Mapped["one"].T_only = ""
		expected.Mapped["two"].T_only = ""
		assertJsonEquals(t, expected, testStruct)
	})
}
