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

package java

import (
	"testing"

	"android/soong/android"
)

var prepareForTestWithSystemServerClasspath = android.GroupFixturePreparers(
	PrepareForTestWithJavaDefaultModules,
)

func TestPlatformSystemServerClasspathVariant(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithSystemServerClasspath,
		android.FixtureWithRootAndroidBp(`
			platform_systemserverclasspath {
				name: "platform-systemserverclasspath",
			}
		`),
	).RunTest(t)

	variants := result.ModuleVariantsForTests("platform-systemserverclasspath")
	android.AssertIntEquals(t, "expect 1 variant", 1, len(variants))
}

func TestPlatformSystemServerClasspath_ClasspathFragmentPaths(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithSystemServerClasspath,
		android.FixtureWithRootAndroidBp(`
			platform_systemserverclasspath {
				name: "platform-systemserverclasspath",
			}
		`),
	).RunTest(t)

	p := result.Module("platform-systemserverclasspath", "android_common").(*platformSystemServerClasspathModule)
	android.AssertStringEquals(t, "output filepath", "systemserverclasspath.pb", p.ClasspathFragmentBase.outputFilepath.Base())
	android.AssertPathRelativeToTopEquals(t, "install filepath", "out/soong/target/product/test_device/system/etc/classpaths", p.ClasspathFragmentBase.installDirPath)
}

func TestPlatformSystemServerClasspathModule_AndroidMkEntries(t *testing.T) {
	preparer := android.GroupFixturePreparers(
		prepareForTestWithSystemServerClasspath,
		android.FixtureWithRootAndroidBp(`
			platform_systemserverclasspath {
				name: "platform-systemserverclasspath",
			}
		`),
	)

	t.Run("AndroidMkEntries", func(t *testing.T) {
		result := preparer.RunTest(t)

		p := result.Module("platform-systemserverclasspath", "android_common").(*platformSystemServerClasspathModule)

		entries := android.AndroidMkEntriesForTest(t, result.TestContext, p)
		android.AssertIntEquals(t, "AndroidMkEntries count", 1, len(entries))
	})

	t.Run("classpath-fragment-entry", func(t *testing.T) {
		result := preparer.RunTest(t)

		want := map[string][]string{
			"LOCAL_MODULE":                {"platform-systemserverclasspath"},
			"LOCAL_MODULE_CLASS":          {"ETC"},
			"LOCAL_INSTALLED_MODULE_STEM": {"systemserverclasspath.pb"},
			// Output and Install paths are tested separately in TestPlatformSystemServerClasspath_ClasspathFragmentPaths
		}

		p := result.Module("platform-systemserverclasspath", "android_common").(*platformSystemServerClasspathModule)

		entries := android.AndroidMkEntriesForTest(t, result.TestContext, p)
		got := entries[0]
		for k, expectedValue := range want {
			if value, ok := got.EntryMap[k]; ok {
				android.AssertDeepEquals(t, k, expectedValue, value)
			} else {
				t.Errorf("No %s defined, saw %q", k, got.EntryMap)
			}
		}
	})
}

func TestSystemServerClasspathFragmentWithoutContents(t *testing.T) {
	prepareForTestWithSystemServerClasspath.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(
			`\Qempty contents are not allowed\E`)).
		RunTestWithBp(t, `
			systemserverclasspath_fragment {
				name: "systemserverclasspath-fragment",
			}
		`)
}
