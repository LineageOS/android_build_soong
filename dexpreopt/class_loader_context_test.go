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
	//     ├── a
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
	implicit := true

	m := make(ClassLoaderContextMap)

	m.AddContext(ctx, AnySdkVersion, "a", optional, implicit, buildPath(ctx, "a"), installPath(ctx, "a"), nil)
	m.AddContext(ctx, AnySdkVersion, "b", optional, implicit, buildPath(ctx, "b"), installPath(ctx, "b"), nil)
	m.AddContext(ctx, AnySdkVersion, "c", optional, implicit, buildPath(ctx, "c"), installPath(ctx, "c"), nil)

	// Add some libraries with nested subcontexts.

	m1 := make(ClassLoaderContextMap)
	m1.AddContext(ctx, AnySdkVersion, "a1", optional, implicit, buildPath(ctx, "a1"), installPath(ctx, "a1"), nil)
	m1.AddContext(ctx, AnySdkVersion, "b1", optional, implicit, buildPath(ctx, "b1"), installPath(ctx, "b1"), nil)

	m2 := make(ClassLoaderContextMap)
	m2.AddContext(ctx, AnySdkVersion, "a2", optional, implicit, buildPath(ctx, "a2"), installPath(ctx, "a2"), nil)
	m2.AddContext(ctx, AnySdkVersion, "b2", optional, implicit, buildPath(ctx, "b2"), installPath(ctx, "b2"), nil)
	m2.AddContext(ctx, AnySdkVersion, "c2", optional, implicit, buildPath(ctx, "c2"), installPath(ctx, "c2"), m1)

	m3 := make(ClassLoaderContextMap)
	m3.AddContext(ctx, AnySdkVersion, "a3", optional, implicit, buildPath(ctx, "a3"), installPath(ctx, "a3"), nil)
	m3.AddContext(ctx, AnySdkVersion, "b3", optional, implicit, buildPath(ctx, "b3"), installPath(ctx, "b3"), nil)

	m.AddContext(ctx, AnySdkVersion, "d", optional, implicit, buildPath(ctx, "d"), installPath(ctx, "d"), m2)
	// When the same library is both in conditional and unconditional context, it should be removed
	// from conditional context.
	m.AddContext(ctx, 42, "f", optional, implicit, buildPath(ctx, "f"), installPath(ctx, "f"), nil)
	m.AddContext(ctx, AnySdkVersion, "f", optional, implicit, buildPath(ctx, "f"), installPath(ctx, "f"), nil)

	// Merge map with implicit root library that is among toplevel contexts => does nothing.
	m.AddContextMap(m1, "c")
	// Merge map with implicit root library that is not among toplevel contexts => all subcontexts
	// of the other map are added as toplevel contexts.
	m.AddContextMap(m3, "m_g")

	// Compatibility libraries with unknown install paths get default paths.
	m.AddContext(ctx, 29, AndroidHidlManager, optional, implicit, buildPath(ctx, AndroidHidlManager), nil, nil)
	m.AddContext(ctx, 29, AndroidHidlBase, optional, implicit, buildPath(ctx, AndroidHidlBase), nil, nil)

	// Add "android.test.mock" to conditional CLC, observe that is gets removed because it is only
	// needed as a compatibility library if "android.test.runner" is in CLC as well.
	m.AddContext(ctx, 30, AndroidTestMock, optional, implicit, buildPath(ctx, AndroidTestMock), nil, nil)

	valid, validationError := validateClassLoaderContext(m)

	fixClassLoaderContext(m)

	var haveStr string
	var havePaths android.Paths
	var haveUsesLibsReq, haveUsesLibsOpt []string
	if valid && validationError == nil {
		haveStr, havePaths = ComputeClassLoaderContext(m)
		haveUsesLibsReq, haveUsesLibsOpt = m.UsesLibs()
	}

	// Test that validation is successful (all paths are known).
	t.Run("validate", func(t *testing.T) {
		if !(valid && validationError == nil) {
			t.Errorf("invalid class loader context")
		}
	})

	// Test that class loader context structure is correct.
	t.Run("string", func(t *testing.T) {
		wantStr := " --host-context-for-sdk 29 " +
			"PCL[out/soong/" + AndroidHidlManager + ".jar]#" +
			"PCL[out/soong/" + AndroidHidlBase + ".jar]" +
			" --target-context-for-sdk 29 " +
			"PCL[/system/framework/" + AndroidHidlManager + ".jar]#" +
			"PCL[/system/framework/" + AndroidHidlBase + ".jar]" +
			" --host-context-for-sdk any " +
			"PCL[out/soong/a.jar]#PCL[out/soong/b.jar]#PCL[out/soong/c.jar]#PCL[out/soong/d.jar]" +
			"{PCL[out/soong/a2.jar]#PCL[out/soong/b2.jar]#PCL[out/soong/c2.jar]" +
			"{PCL[out/soong/a1.jar]#PCL[out/soong/b1.jar]}}#" +
			"PCL[out/soong/f.jar]#PCL[out/soong/a3.jar]#PCL[out/soong/b3.jar]" +
			" --target-context-for-sdk any " +
			"PCL[/system/a.jar]#PCL[/system/b.jar]#PCL[/system/c.jar]#PCL[/system/d.jar]" +
			"{PCL[/system/a2.jar]#PCL[/system/b2.jar]#PCL[/system/c2.jar]" +
			"{PCL[/system/a1.jar]#PCL[/system/b1.jar]}}#" +
			"PCL[/system/f.jar]#PCL[/system/a3.jar]#PCL[/system/b3.jar]"
		if wantStr != haveStr {
			t.Errorf("\nwant class loader context: %s\nhave class loader context: %s", wantStr, haveStr)
		}
	})

	// Test that all expected build paths are gathered.
	t.Run("paths", func(t *testing.T) {
		wantPaths := []string{
			"out/soong/android.hidl.manager-V1.0-java.jar", "out/soong/android.hidl.base-V1.0-java.jar",
			"out/soong/a.jar", "out/soong/b.jar", "out/soong/c.jar", "out/soong/d.jar",
			"out/soong/a2.jar", "out/soong/b2.jar", "out/soong/c2.jar",
			"out/soong/a1.jar", "out/soong/b1.jar",
			"out/soong/f.jar", "out/soong/a3.jar", "out/soong/b3.jar",
		}
		if !reflect.DeepEqual(wantPaths, havePaths.Strings()) {
			t.Errorf("\nwant paths: %s\nhave paths: %s", wantPaths, havePaths)
		}
	})

	// Test for libraries that are added by the manifest_fixer.
	t.Run("uses libs", func(t *testing.T) {
		wantUsesLibsReq := []string{"a", "b", "c", "d", "f", "a3", "b3"}
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
	implicit := true
	m := make(ClassLoaderContextMap)
	m.AddContext(ctx, 28, "a", optional, implicit, buildPath(ctx, "a"), installPath(ctx, "a"), nil)
	m.AddContext(ctx, 29, "b", optional, implicit, buildPath(ctx, "b"), installPath(ctx, "b"), nil)
	m.AddContext(ctx, 30, "c", optional, implicit, buildPath(ctx, "c"), installPath(ctx, "c"), nil)
	m.AddContext(ctx, AnySdkVersion, "d", optional, implicit, buildPath(ctx, "d"), installPath(ctx, "d"), nil)
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
	implicit := true

	m := make(ClassLoaderContextMap)
	if whichPath == "build" {
		m.AddContext(ctx, AnySdkVersion, "a", optional, implicit, nil, nil, nil)
	} else {
		m.AddContext(ctx, AnySdkVersion, "a", optional, implicit, buildPath(ctx, "a"), nil, nil)
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
//func TestCLCNestedConditional(t *testing.T) {
//	ctx := testContext()
//	optional := false
//	implicit := true
//	m1 := make(ClassLoaderContextMap)
//	m1.AddContext(ctx, 42, "a", optional, implicit, buildPath(ctx, "a"), installPath(ctx, "a"), nil)
//	m := make(ClassLoaderContextMap)
//	err := m.addContext(ctx, AnySdkVersion, "b", optional, implicit, buildPath(ctx, "b"), installPath(ctx, "b"), m1)
//	checkError(t, err, "nested class loader context shouldn't have conditional part")
//}

// Test for SDK version order in conditional CLC: no matter in what order the libraries are added,
// they end up in the order that agrees with PackageManager.
func TestCLCSdkVersionOrder(t *testing.T) {
	ctx := testContext()
	optional := false
	implicit := true
	m := make(ClassLoaderContextMap)
	m.AddContext(ctx, 28, "a", optional, implicit, buildPath(ctx, "a"), installPath(ctx, "a"), nil)
	m.AddContext(ctx, 29, "b", optional, implicit, buildPath(ctx, "b"), installPath(ctx, "b"), nil)
	m.AddContext(ctx, 30, "c", optional, implicit, buildPath(ctx, "c"), installPath(ctx, "c"), nil)
	m.AddContext(ctx, AnySdkVersion, "d", optional, implicit, buildPath(ctx, "d"), installPath(ctx, "d"), nil)

	valid, validationError := validateClassLoaderContext(m)

	fixClassLoaderContext(m)

	var haveStr string
	if valid && validationError == nil {
		haveStr, _ = ComputeClassLoaderContext(m)
	}

	// Test that validation is successful (all paths are known).
	t.Run("validate", func(t *testing.T) {
		if !(valid && validationError == nil) {
			t.Errorf("invalid class loader context")
		}
	})

	// Test that class loader context structure is correct.
	t.Run("string", func(t *testing.T) {
		wantStr := " --host-context-for-sdk 30 PCL[out/soong/c.jar]" +
			" --target-context-for-sdk 30 PCL[/system/c.jar]" +
			" --host-context-for-sdk 29 PCL[out/soong/b.jar]" +
			" --target-context-for-sdk 29 PCL[/system/b.jar]" +
			" --host-context-for-sdk 28 PCL[out/soong/a.jar]" +
			" --target-context-for-sdk 28 PCL[/system/a.jar]" +
			" --host-context-for-sdk any PCL[out/soong/d.jar]" +
			" --target-context-for-sdk any PCL[/system/d.jar]"
		if wantStr != haveStr {
			t.Errorf("\nwant class loader context: %s\nhave class loader context: %s", wantStr, haveStr)
		}
	})
}

func TestCLCMExcludeLibs(t *testing.T) {
	ctx := testContext()
	const optional = false
	const implicit = true

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
		m.AddContext(ctx, 28, "a", optional, implicit, buildPath(ctx, "a"), installPath(ctx, "a"), nil)

		a := excludeLibs(t, m)

		android.AssertStringEquals(t, "output CLCM ", `{
  "28": [
    {
      "Name": "a",
      "Optional": false,
      "Implicit": true,
      "Host": "out/soong/a.jar",
      "Device": "/system/a.jar",
      "Subcontexts": []
    }
  ]
}`, a.Dump())
	})

	t.Run("one item from list", func(t *testing.T) {
		m := make(ClassLoaderContextMap)
		m.AddContext(ctx, 28, "a", optional, implicit, buildPath(ctx, "a"), installPath(ctx, "a"), nil)
		m.AddContext(ctx, 28, "b", optional, implicit, buildPath(ctx, "b"), installPath(ctx, "b"), nil)

		a := excludeLibs(t, m, "a")

		expected := `{
  "28": [
    {
      "Name": "b",
      "Optional": false,
      "Implicit": true,
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
		m.AddContext(ctx, 28, "a", optional, implicit, buildPath(ctx, "a"), installPath(ctx, "a"), nil)
		m.AddContext(ctx, 28, "b", optional, implicit, buildPath(ctx, "b"), installPath(ctx, "b"), nil)

		a := excludeLibs(t, m, "a", "b")

		android.AssertStringEquals(t, "output CLCM ", `{}`, a.Dump())
	})

	t.Run("items from a subcontext", func(t *testing.T) {
		s := make(ClassLoaderContextMap)
		s.AddContext(ctx, AnySdkVersion, "b", optional, implicit, buildPath(ctx, "b"), installPath(ctx, "b"), nil)
		s.AddContext(ctx, AnySdkVersion, "c", optional, implicit, buildPath(ctx, "c"), installPath(ctx, "c"), nil)

		m := make(ClassLoaderContextMap)
		m.AddContext(ctx, 28, "a", optional, implicit, buildPath(ctx, "a"), installPath(ctx, "a"), s)

		a := excludeLibs(t, m, "b")

		android.AssertStringEquals(t, "output CLCM ", `{
  "28": [
    {
      "Name": "a",
      "Optional": false,
      "Implicit": true,
      "Host": "out/soong/a.jar",
      "Device": "/system/a.jar",
      "Subcontexts": [
        {
          "Name": "c",
          "Optional": false,
          "Implicit": true,
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
