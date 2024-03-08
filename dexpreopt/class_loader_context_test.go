// Copyright 2020 Google Inc. All rights reserved.
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

package dexpreopt

// This file contains unit tests for class loader context structure.
// For class loader context tests involving .bp files, see TestUsesLibraries in java package.

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"android/soong/android"
)

func TestCLC(t *testing.T) {
	// Construct class loader context with the following structure:
	// .
	// ├── 29
	// │   ├── android.hidl.manager
	// │   └── android.hidl.base
	// │
	// └── any
	//     ├── a'  (a single quotation mark (') is there to test escaping)
	//     ├── b
	//     ├── c
	//     ├── d
	//     │   ├── a2
	//     │   ├── b2
	//     │   └── c2
	//     │       ├── a1
	//     │       └── b1
	//     ├── f
	//     ├── a3
	//     └── b3
	//
	ctx := testContext()

	optional := false

	m := make(ClassLoaderContextMap)

	m.AddContext(ctx, AnySdkVersion, "a'", optional, buildPath(ctx, "a"), installPath(ctx, "a"), nil)
	m.AddContext(ctx, AnySdkVersion, "b", optional, buildPath(ctx, "b"), installPath(ctx, "b"), nil)
	m.AddContext(ctx, AnySdkVersion, "c", optional, buildPath(ctx, "c"), installPath(ctx, "c"), nil)

	// Add some libraries with nested subcontexts.

	m1 := make(ClassLoaderContextMap)
	m1.AddContext(ctx, AnySdkVersion, "a1", optional, buildPath(ctx, "a1"), installPath(ctx, "a1"), nil)
	m1.AddContext(ctx, AnySdkVersion, "b1", optional, buildPath(ctx, "b1"), installPath(ctx, "b1"), nil)

	m2 := make(ClassLoaderContextMap)
	m2.AddContext(ctx, AnySdkVersion, "a2", optional, buildPath(ctx, "a2"), installPath(ctx, "a2"), nil)
	m2.AddContext(ctx, AnySdkVersion, "b2", optional, buildPath(ctx, "b2"), installPath(ctx, "b2"), nil)
	m2.AddContext(ctx, AnySdkVersion, "c2", optional, buildPath(ctx, "c2"), installPath(ctx, "c2"), m1)

	m3 := make(ClassLoaderContextMap)
	m3.AddContext(ctx, AnySdkVersion, "a3", optional, buildPath(ctx, "a3"), installPath(ctx, "a3"), nil)
	m3.AddContext(ctx, AnySdkVersion, "b3", optional, buildPath(ctx, "b3"), installPath(ctx, "b3"), nil)

	m.AddContext(ctx, AnySdkVersion, "d", optional, buildPath(ctx, "d"), installPath(ctx, "d"), m2)
	// When the same library is both in conditional and unconditional context, it should be removed
	// from conditional context.
	m.AddContext(ctx, 42, "f", optional, buildPath(ctx, "f"), installPath(ctx, "f"), nil)
	m.AddContext(ctx, AnySdkVersion, "f", optional, buildPath(ctx, "f"), installPath(ctx, "f"), nil)

	// Merge map with implicit root library that is among toplevel contexts => does nothing.
	m.AddContextMap(m1, "c")
	// Merge map with implicit root library that is not among toplevel contexts => all subcontexts
	// of the other map are added as toplevel contexts.
	m.AddContextMap(m3, "m_g")

	// Compatibility libraries with unknown install paths get default paths.
	m.AddContext(ctx, 29, AndroidHidlManager, optional, buildPath(ctx, AndroidHidlManager), nil, nil)
	m.AddContext(ctx, 29, AndroidHidlBase, optional, buildPath(ctx, AndroidHidlBase), nil, nil)

	// Add "android.test.mock" to conditional CLC, observe that is gets removed because it is only
	// needed as a compatibility library if "android.test.runner" is in CLC as well.
	m.AddContext(ctx, 30, AndroidTestMock, optional, buildPath(ctx, AndroidTestMock), nil, nil)

	valid, validationError := validateClassLoaderContext(m)

	fixClassLoaderContext(m)

	var actualNames []string
	var actualPaths android.Paths
	var haveUsesLibsReq, haveUsesLibsOpt []string
	if valid && validationError == nil {
		actualNames, actualPaths = ComputeClassLoaderContextDependencies(m)
		haveUsesLibsReq, haveUsesLibsOpt = m.UsesLibs()
	}

	// Test that validation is successful (all paths are known).
	t.Run("validate", func(t *testing.T) {
		if !(valid && validationError == nil) {
			t.Errorf("invalid class loader context")
		}
	})

	// Test that all expected build paths are gathered.
	t.Run("names and paths", func(t *testing.T) {
		expectedNames := []string{
			"a'", "a1", "a2", "a3", "android.hidl.base-V1.0-java", "android.hidl.manager-V1.0-java", "b",
			"b1", "b2", "b3", "c", "c2", "d", "f",
		}
		expectedPaths := []string{
			"out/soong/android.hidl.manager-V1.0-java.jar", "out/soong/android.hidl.base-V1.0-java.jar",
			"out/soong/a.jar", "out/soong/b.jar", "out/soong/c.jar", "out/soong/d.jar",
			"out/soong/a2.jar", "out/soong/b2.jar", "out/soong/c2.jar",
			"out/soong/a1.jar", "out/soong/b1.jar",
			"out/soong/f.jar", "out/soong/a3.jar", "out/soong/b3.jar",
		}
		actualPathsStrs := actualPaths.Strings()
		// The order does not matter.
		sort.Strings(expectedNames)
		sort.Strings(actualNames)
		android.AssertArrayString(t, "", expectedNames, actualNames)
		sort.Strings(expectedPaths)
		sort.Strings(actualPathsStrs)
		android.AssertArrayString(t, "", expectedPaths, actualPathsStrs)
	})

	// Test the JSON passed to construct_context.py.
	t.Run("json", func(t *testing.T) {
		// The tree structure within each SDK version should be kept exactly the same when serialized
		// to JSON. The order matters because the Python script keeps the order within each SDK version
		// as is.
		// The JSON is passed to the Python script as a commandline flag, so quotation ('') and escaping
		// must be performed.
		android.AssertStringEquals(t, "", strings.TrimSpace(`
'{"29":[{"Name":"android.hidl.manager-V1.0-java","Optional":false,"Host":"out/soong/android.hidl.manager-V1.0-java.jar","Device":"/system/framework/android.hidl.manager-V1.0-java.jar","Subcontexts":[]},{"Name":"android.hidl.base-V1.0-java","Optional":false,"Host":"out/soong/android.hidl.base-V1.0-java.jar","Device":"/system/framework/android.hidl.base-V1.0-java.jar","Subcontexts":[]}],"30":[],"42":[],"any":[{"Name":"a'\''","Optional":false,"Host":"out/soong/a.jar","Device":"/system/a.jar","Subcontexts":[]},{"Name":"b","Optional":false,"Host":"out/soong/b.jar","Device":"/system/b.jar","Subcontexts":[]},{"Name":"c","Optional":false,"Host":"out/soong/c.jar","Device":"/system/c.jar","Subcontexts":[]},{"Name":"d","Optional":false,"Host":"out/soong/d.jar","Device":"/system/d.jar","Subcontexts":[{"Name":"a2","Optional":false,"Host":"out/soong/a2.jar","Device":"/system/a2.jar","Subcontexts":[]},{"Name":"b2","Optional":false,"Host":"out/soong/b2.jar","Device":"/system/b2.jar","Subcontexts":[]},{"Name":"c2","Optional":false,"Host":"out/soong/c2.jar","Device":"/system/c2.jar","Subcontexts":[{"Name":"a1","Optional":false,"Host":"out/soong/a1.jar","Device":"/system/a1.jar","Subcontexts":[]},{"Name":"b1","Optional":false,"Host":"out/soong/b1.jar","Device":"/system/b1.jar","Subcontexts":[]}]}]},{"Name":"f","Optional":false,"Host":"out/soong/f.jar","Device":"/system/f.jar","Subcontexts":[]},{"Name":"a3","Optional":false,"Host":"out/soong/a3.jar","Device":"/system/a3.jar","Subcontexts":[]},{"Name":"b3","Optional":false,"Host":"out/soong/b3.jar","Device":"/system/b3.jar","Subcontexts":[]}]}'
`), m.DumpForFlag())
	})

	// Test for libraries that are added by the manifest_fixer.
	t.Run("uses libs", func(t *testing.T) {
		wantUsesLibsReq := []string{"a'", "b", "c", "d", "f", "a3", "b3"}
		wantUsesLibsOpt := []string{}
		if !reflect.DeepEqual(wantUsesLibsReq, haveUsesLibsReq) {
			t.Errorf("\nwant required uses libs: %s\nhave required uses libs: %s", wantUsesLibsReq, haveUsesLibsReq)
		}
		if !reflect.DeepEqual(wantUsesLibsOpt, haveUsesLibsOpt) {
			t.Errorf("\nwant optional uses libs: %s\nhave optional uses libs: %s", wantUsesLibsOpt, haveUsesLibsOpt)
		}
	})
}

func TestCLCJson(t *testing.T) {
	ctx := testContext()
	optional := false
	m := make(ClassLoaderContextMap)
	m.AddContext(ctx, 28, "a", optional, buildPath(ctx, "a"), installPath(ctx, "a"), nil)
	m.AddContext(ctx, 29, "b", optional, buildPath(ctx, "b"), installPath(ctx, "b"), nil)
	m.AddContext(ctx, 30, "c", optional, buildPath(ctx, "c"), installPath(ctx, "c"), nil)
	m.AddContext(ctx, AnySdkVersion, "d", optional, buildPath(ctx, "d"), installPath(ctx, "d"), nil)
	jsonCLC := toJsonClassLoaderContext(m)
	restored := fromJsonClassLoaderContext(ctx, jsonCLC)
	android.AssertIntEquals(t, "The size of the maps should be the same.", len(m), len(restored))
	for k := range m {
		a, _ := m[k]
		b, ok := restored[k]
		android.AssertBoolEquals(t, "The both maps should have the same keys.", ok, true)
		android.AssertIntEquals(t, "The size of the elements should be the same.", len(a), len(b))
		for i, elemA := range a {
			before := fmt.Sprintf("%v", *elemA)
			after := fmt.Sprintf("%v", *b[i])
			android.AssertStringEquals(t, "The content should be the same.", before, after)
		}
	}
}

// Test that unknown library paths cause a validation error.
func testCLCUnknownPath(t *testing.T, whichPath string) {
	ctx := testContext()
	optional := false

	m := make(ClassLoaderContextMap)
	if whichPath == "build" {
		m.AddContext(ctx, AnySdkVersion, "a", optional, nil, nil, nil)
	} else {
		m.AddContext(ctx, AnySdkVersion, "a", optional, buildPath(ctx, "a"), nil, nil)
	}

	// The library should be added to <uses-library> tags by the manifest_fixer.
	t.Run("uses libs", func(t *testing.T) {
		haveUsesLibsReq, haveUsesLibsOpt := m.UsesLibs()
		wantUsesLibsReq := []string{"a"}
		wantUsesLibsOpt := []string{}
		if !reflect.DeepEqual(wantUsesLibsReq, haveUsesLibsReq) {
			t.Errorf("\nwant required uses libs: %s\nhave required uses libs: %s", wantUsesLibsReq, haveUsesLibsReq)
		}
		if !reflect.DeepEqual(wantUsesLibsOpt, haveUsesLibsOpt) {
			t.Errorf("\nwant optional uses libs: %s\nhave optional uses libs: %s", wantUsesLibsOpt, haveUsesLibsOpt)
		}
	})

	// But CLC cannot be constructed: there is a validation error.
	_, err := validateClassLoaderContext(m)
	checkError(t, err, fmt.Sprintf("invalid %s path for <uses-library> \"a\"", whichPath))
}

// Test that unknown build path is an error.
func TestCLCUnknownBuildPath(t *testing.T) {
	testCLCUnknownPath(t, "build")
}

// Test that unknown install path is an error.
func TestCLCUnknownInstallPath(t *testing.T) {
	testCLCUnknownPath(t, "install")
}

// An attempt to add conditional nested subcontext should fail.
func TestCLCNestedConditional(t *testing.T) {
	ctx := testContext()
	optional := false
	m1 := make(ClassLoaderContextMap)
	m1.AddContext(ctx, 42, "a", optional, buildPath(ctx, "a"), installPath(ctx, "a"), nil)
	m := make(ClassLoaderContextMap)
	err := m.addContext(ctx, AnySdkVersion, "b", optional, buildPath(ctx, "b"), installPath(ctx, "b"), m1)
	checkError(t, err, "nested class loader context shouldn't have conditional part")
}

func TestCLCMExcludeLibs(t *testing.T) {
	ctx := testContext()
	const optional = false

	excludeLibs := func(t *testing.T, m ClassLoaderContextMap, excluded_libs ...string) ClassLoaderContextMap {
		// Dump the CLCM before creating a new copy that excludes a specific set of libraries.
		before := m.Dump()

		// Create a new CLCM that excludes some libraries.
		c := m.ExcludeLibs(excluded_libs)

		// Make sure that the original CLCM was not changed.
		after := m.Dump()
		android.AssertStringEquals(t, "input CLCM modified", before, after)

		return c
	}

	t.Run("exclude nothing", func(t *testing.T) {
		m := make(ClassLoaderContextMap)
		m.AddContext(ctx, 28, "a", optional, buildPath(ctx, "a"), installPath(ctx, "a"), nil)

		a := excludeLibs(t, m)

		android.AssertStringEquals(t, "output CLCM ", `{
  "28": [
    {
      "Name": "a",
      "Optional": false,
      "Host": "out/soong/a.jar",
      "Device": "/system/a.jar",
      "Subcontexts": []
    }
  ]
}`, a.Dump())
	})

	t.Run("one item from list", func(t *testing.T) {
		m := make(ClassLoaderContextMap)
		m.AddContext(ctx, 28, "a", optional, buildPath(ctx, "a"), installPath(ctx, "a"), nil)
		m.AddContext(ctx, 28, "b", optional, buildPath(ctx, "b"), installPath(ctx, "b"), nil)

		a := excludeLibs(t, m, "a")

		expected := `{
  "28": [
    {
      "Name": "b",
      "Optional": false,
      "Host": "out/soong/b.jar",
      "Device": "/system/b.jar",
      "Subcontexts": []
    }
  ]
}`
		android.AssertStringEquals(t, "output CLCM ", expected, a.Dump())
	})

	t.Run("all items from a list", func(t *testing.T) {
		m := make(ClassLoaderContextMap)
		m.AddContext(ctx, 28, "a", optional, buildPath(ctx, "a"), installPath(ctx, "a"), nil)
		m.AddContext(ctx, 28, "b", optional, buildPath(ctx, "b"), installPath(ctx, "b"), nil)

		a := excludeLibs(t, m, "a", "b")

		android.AssertStringEquals(t, "output CLCM ", `{}`, a.Dump())
	})

	t.Run("items from a subcontext", func(t *testing.T) {
		s := make(ClassLoaderContextMap)
		s.AddContext(ctx, AnySdkVersion, "b", optional, buildPath(ctx, "b"), installPath(ctx, "b"), nil)
		s.AddContext(ctx, AnySdkVersion, "c", optional, buildPath(ctx, "c"), installPath(ctx, "c"), nil)

		m := make(ClassLoaderContextMap)
		m.AddContext(ctx, 28, "a", optional, buildPath(ctx, "a"), installPath(ctx, "a"), s)

		a := excludeLibs(t, m, "b")

		android.AssertStringEquals(t, "output CLCM ", `{
  "28": [
    {
      "Name": "a",
      "Optional": false,
      "Host": "out/soong/a.jar",
      "Device": "/system/a.jar",
      "Subcontexts": [
        {
          "Name": "c",
          "Optional": false,
          "Host": "out/soong/c.jar",
          "Device": "/system/c.jar",
          "Subcontexts": []
        }
      ]
    }
  ]
}`, a.Dump())
	})
}

// Test that CLC is correctly serialized to JSON.
func TestCLCtoJSON(t *testing.T) {
	ctx := testContext()
	optional := false
	m := make(ClassLoaderContextMap)
	m.AddContext(ctx, 28, "a", optional, buildPath(ctx, "a"), installPath(ctx, "a"), nil)
	m.AddContext(ctx, AnySdkVersion, "b", optional, buildPath(ctx, "b"), installPath(ctx, "b"), nil)
	android.AssertStringEquals(t, "output CLCM ", `{
  "28": [
    {
      "Name": "a",
      "Optional": false,
      "Host": "out/soong/a.jar",
      "Device": "/system/a.jar",
      "Subcontexts": []
    }
  ],
  "any": [
    {
      "Name": "b",
      "Optional": false,
      "Host": "out/soong/b.jar",
      "Device": "/system/b.jar",
      "Subcontexts": []
    }
  ]
}`, m.Dump())
}

func checkError(t *testing.T, have error, want string) {
	if have == nil {
		t.Errorf("\nwant error: '%s'\nhave: none", want)
	} else if msg := have.Error(); !strings.HasPrefix(msg, want) {
		t.Errorf("\nwant error: '%s'\nhave error: '%s'\n", want, msg)
	}
}

func testContext() android.ModuleInstallPathContext {
	config := android.TestConfig("out", nil, "", nil)
	return android.ModuleInstallPathContextForTesting(config)
}

func buildPath(ctx android.PathContext, lib string) android.Path {
	return android.PathForOutput(ctx, lib+".jar")
}

func installPath(ctx android.ModuleInstallPathContext, lib string) android.InstallPath {
	return android.PathForModuleInstall(ctx, lib+".jar")
}
