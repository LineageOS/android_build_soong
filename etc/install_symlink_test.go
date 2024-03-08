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

package etc

import (
	"android/soong/android"
	"strings"
	"testing"
)

var prepareForInstallSymlinkTest = android.GroupFixturePreparers(
	android.PrepareForTestWithArchMutator,
	android.FixtureRegisterWithContext(RegisterInstallSymlinkBuildComponents),
)

func TestInstallSymlinkBasic(t *testing.T) {
	result := prepareForInstallSymlinkTest.RunTestWithBp(t, `
		install_symlink {
			name: "foo",
			installed_location: "bin/foo",
			symlink_target: "/system/system_ext/bin/foo",
		}
	`)

	foo_variants := result.ModuleVariantsForTests("foo")
	if len(foo_variants) != 1 {
		t.Fatalf("expected 1 variant, got %#v", foo_variants)
	}

	foo := result.ModuleForTests("foo", "android_common").Module()
	androidMkEntries := android.AndroidMkEntriesForTest(t, result.TestContext, foo)
	if len(androidMkEntries) != 1 {
		t.Fatalf("expected 1 androidmkentry, got %d", len(androidMkEntries))
	}

	symlinks := androidMkEntries[0].EntryMap["LOCAL_SOONG_INSTALL_SYMLINKS"]
	if len(symlinks) != 1 {
		t.Fatalf("Expected 1 symlink, got %d", len(symlinks))
	}

	if !strings.HasSuffix(symlinks[0], "system/bin/foo") {
		t.Fatalf("Expected symlink install path to end in system/bin/foo, got: %s", symlinks[0])
	}
}

func TestInstallSymlinkToRecovery(t *testing.T) {
	result := prepareForInstallSymlinkTest.RunTestWithBp(t, `
		install_symlink {
			name: "foo",
			installed_location: "bin/foo",
			symlink_target: "/system/system_ext/bin/foo",
			recovery: true,
		}
	`)

	foo_variants := result.ModuleVariantsForTests("foo")
	if len(foo_variants) != 1 {
		t.Fatalf("expected 1 variant, got %#v", foo_variants)
	}

	foo := result.ModuleForTests("foo", "android_common").Module()
	androidMkEntries := android.AndroidMkEntriesForTest(t, result.TestContext, foo)
	if len(androidMkEntries) != 1 {
		t.Fatalf("expected 1 androidmkentry, got %d", len(androidMkEntries))
	}

	symlinks := androidMkEntries[0].EntryMap["LOCAL_SOONG_INSTALL_SYMLINKS"]
	if len(symlinks) != 1 {
		t.Fatalf("Expected 1 symlink, got %d", len(symlinks))
	}

	if !strings.HasSuffix(symlinks[0], "recovery/root/system/bin/foo") {
		t.Fatalf("Expected symlink install path to end in recovery/root/system/bin/foo, got: %s", symlinks[0])
	}
}

func TestErrorOnNonCleanTarget(t *testing.T) {
	prepareForInstallSymlinkTest.
		ExtendWithErrorHandler(android.FixtureExpectsOneErrorPattern("Should be a clean filepath")).
		RunTestWithBp(t, `
		install_symlink {
			name: "foo",
			installed_location: "bin/foo",
			symlink_target: "/system/system_ext/../bin/foo",
		}
	`)
}

func TestErrorOnNonCleanInstalledLocation(t *testing.T) {
	prepareForInstallSymlinkTest.
		ExtendWithErrorHandler(android.FixtureExpectsOneErrorPattern("Should be a clean filepath")).
		RunTestWithBp(t, `
		install_symlink {
			name: "foo",
			installed_location: "bin/../foo",
			symlink_target: "/system/system_ext/bin/foo",
		}
	`)
}

func TestErrorOnInstalledPathStartingWithDotDot(t *testing.T) {
	prepareForInstallSymlinkTest.
		ExtendWithErrorHandler(android.FixtureExpectsOneErrorPattern("Should not start with / or \\.\\./")).
		RunTestWithBp(t, `
		install_symlink {
			name: "foo",
			installed_location: "../bin/foo",
			symlink_target: "/system/system_ext/bin/foo",
		}
	`)
}

func TestErrorOnInstalledPathStartingWithSlash(t *testing.T) {
	prepareForInstallSymlinkTest.
		ExtendWithErrorHandler(android.FixtureExpectsOneErrorPattern("Should not start with / or \\.\\./")).
		RunTestWithBp(t, `
		install_symlink {
			name: "foo",
			installed_location: "/bin/foo",
			symlink_target: "/system/system_ext/bin/foo",
		}
	`)
}
