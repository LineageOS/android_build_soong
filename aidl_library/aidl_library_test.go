// Copyright 2023 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aidl_library

import (
	"android/soong/android"
	"testing"
)

func TestAidlLibrary(t *testing.T) {
	t.Parallel()
	ctx := android.GroupFixturePreparers(
		PrepareForTestWithAidlLibrary,
		android.MockFS{
			"package_bar/Android.bp": []byte(`
			aidl_library {
					name: "bar",
					srcs: ["x/y/Bar.aidl"],
					strip_import_prefix: "x",
				}
			`),
		}.AddToFixture(),
		android.MockFS{
			"package_foo/Android.bp": []byte(`
			aidl_library {
					name: "foo",
					srcs: ["a/b/Foo.aidl"],
					hdrs: ["a/Header.aidl"],
					strip_import_prefix: "a",
					deps: ["bar"],
				}
			`),
		}.AddToFixture(),
	).RunTest(t).TestContext

	foo := ctx.ModuleForTests("foo", "").Module().(*AidlLibrary)
	actualInfo, _ := android.SingletonModuleProvider(ctx, foo, AidlLibraryProvider)

	android.AssertArrayString(
		t,
		"aidl include dirs",
		[]string{"package_foo/a", "package_bar/x"},
		android.Paths(actualInfo.IncludeDirs.ToList()).Strings(),
	)

	android.AssertPathsRelativeToTopEquals(
		t,
		"aidl srcs paths",
		[]string{"package_foo/a/b/Foo.aidl"},
		actualInfo.Srcs,
	)

	android.AssertPathsRelativeToTopEquals(
		t,
		"aidl hdrs paths",
		[]string{"package_foo/a/Header.aidl"},
		actualInfo.Hdrs.ToList(),
	)
}

func TestAidlLibraryWithoutStripImportPrefix(t *testing.T) {
	t.Parallel()
	ctx := android.GroupFixturePreparers(
		PrepareForTestWithAidlLibrary,
		android.MockFS{
			"package_bar/Android.bp": []byte(`
			aidl_library {
					name: "bar",
					srcs: ["x/y/Bar.aidl"],
					hdrs: ["BarHeader.aidl"],
				}
			`),
		}.AddToFixture(),
		android.MockFS{
			"package_foo/Android.bp": []byte(`
			aidl_library {
					name: "foo",
					srcs: ["a/b/Foo.aidl"],
					deps: ["bar"],
				}
			`),
		}.AddToFixture(),
	).RunTest(t).TestContext

	foo := ctx.ModuleForTests("foo", "").Module().(*AidlLibrary)
	actualInfo, _ := android.SingletonModuleProvider(ctx, foo, AidlLibraryProvider)

	android.AssertArrayString(
		t,
		"aidl include dirs",
		[]string{"package_foo", "package_bar"},
		android.Paths(actualInfo.IncludeDirs.ToList()).Strings(),
	)

	android.AssertPathsRelativeToTopEquals(
		t,
		"aidl srcs paths",
		[]string{"package_foo/a/b/Foo.aidl"},
		actualInfo.Srcs,
	)

	android.AssertPathsRelativeToTopEquals(
		t,
		"aidl hdrs paths",
		[]string{"package_bar/BarHeader.aidl"},
		actualInfo.Hdrs.ToList(),
	)
}

func TestAidlLibraryWithNoSrcsHdrsDeps(t *testing.T) {
	t.Parallel()
	android.GroupFixturePreparers(
		PrepareForTestWithAidlLibrary,
		android.MockFS{
			"package_bar/Android.bp": []byte(`
			aidl_library {
					name: "bar",
				}
			`),
		}.AddToFixture(),
	).
		ExtendWithErrorHandler(android.FixtureExpectsOneErrorPattern("at least srcs or hdrs prop must be non-empty")).
		RunTest(t)
}
