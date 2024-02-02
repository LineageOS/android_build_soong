// Copyright 2024 Google Inc. All rights reserved.
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
package tradefed_modules

import (
	"android/soong/android"
	"android/soong/java"
	"strings"
	"testing"
)

const bp = `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			sdk_version: "current",
		}

                android_test_helper_app {
                        name: "HelperApp",
                        srcs: ["helper.java"],
                }

		android_test {
			name: "base",
			sdk_version: "current",
                        data: [":HelperApp", "data/testfile"],
		}

                test_module_config {
                        name: "derived_test",
                        base: "base",
                        exclude_filters: ["android.test.example.devcodelab.DevCodelabTest#testHelloFail"],
                        include_annotations: ["android.platform.test.annotations.LargeTest"],
                }

`

// Ensure we create files needed and set the AndroidMkEntries needed
func TestModuleConfigAndroidTest(t *testing.T) {

	ctx := android.GroupFixturePreparers(
		java.PrepareForTestWithJavaDefaultModules,
		android.FixtureRegisterWithContext(RegisterTestModuleConfigBuildComponents),
	).RunTestWithBp(t, bp)

	derived := ctx.ModuleForTests("derived_test", "android_common")
	// Assert there are rules to create these files.
	derived.Output("test_module_config.manifest")
	derived.Output("test_config_fixer/derived_test.config")

	// Ensure some basic rules exist.
	ctx.ModuleForTests("base", "android_common").Output("package-res.apk")
	entries := android.AndroidMkEntriesForTest(t, ctx.TestContext, derived.Module())[0]

	// Ensure some entries from base are there, specifically support files for data and helper apps.
	assertEntryPairValues(t, entries.EntryMap["LOCAL_COMPATIBILITY_SUPPORT_FILES"], []string{"HelperApp.apk", "data/testfile"})

	// And some new derived entries are there.
	android.AssertArrayString(t, "", entries.EntryMap["LOCAL_MODULE_TAGS"], []string{"tests"})

	// And ones we override
	android.AssertArrayString(t, "", entries.EntryMap["LOCAL_SOONG_JNI_LIBS_SYMBOLS"], []string{""})

	android.AssertStringMatches(t, "", entries.EntryMap["LOCAL_FULL_TEST_CONFIG"][0], "derived_test/android_common/test_config_fixer/derived_test.config")
}

// Make sure we call test-config-fixer with the right args.
func TestModuleConfigOptions(t *testing.T) {

	ctx := android.GroupFixturePreparers(
		java.PrepareForTestWithJavaDefaultModules,
		android.FixtureRegisterWithContext(RegisterTestModuleConfigBuildComponents),
	).RunTestWithBp(t, bp)

	// Check that we generate a rule to make a new AndroidTest.xml/Module.config file.
	derived := ctx.ModuleForTests("derived_test", "android_common")
	rule_cmd := derived.Rule("fix_test_config").RuleParams.Command
	android.AssertStringDoesContain(t, "Bad FixConfig rule inputs", rule_cmd,
		`--test-file-name=derived_test.apk --orig-test-file-name=base.apk --test-runner-options='[{"Name":"exclude-filter","Key":"","Value":"android.test.example.devcodelab.DevCodelabTest#testHelloFail"},{"Name":"include-annotation","Key":"","Value":"android.platform.test.annotations.LargeTest"}]'`)
}

// Ensure we error for a base we don't support.
func TestModuleConfigBadBaseShouldFail(t *testing.T) {
	badBp := `
		java_test_host {
			name: "base",
                        srcs: ["a.java"],
		}

                test_module_config {
                        name: "derived_test",
                        base: "base",
                        exclude_filters: ["android.test.example.devcodelab.DevCodelabTest#testHelloFail"],
                        include_annotations: ["android.platform.test.annotations.LargeTest"],
                }`

	ctx := android.GroupFixturePreparers(
		java.PrepareForTestWithJavaDefaultModules,
		android.FixtureRegisterWithContext(RegisterTestModuleConfigBuildComponents),
	).ExtendWithErrorHandler(
		android.FixtureExpectsAtLeastOneErrorMatchingPattern("does not provide test BaseTestProviderData")).
		RunTestWithBp(t, badBp)

	ctx.ModuleForTests("derived_test", "android_common")
}

// Ensure we error for a base we don't support.
func TestModuleConfigNoFiltersOrAnnotationsShouldFail(t *testing.T) {
	badBp := `
		android_test {
			name: "base",
			sdk_version: "current",
                        srcs: ["a.java"],
		}

                test_module_config {
                        name: "derived_test",
                        base: "base",
                }`

	ctx := android.GroupFixturePreparers(
		java.PrepareForTestWithJavaDefaultModules,
		android.FixtureRegisterWithContext(RegisterTestModuleConfigBuildComponents),
	).ExtendWithErrorHandler(
		android.FixtureExpectsAtLeastOneErrorMatchingPattern("Test options must be given")).
		RunTestWithBp(t, badBp)

	ctx.ModuleForTests("derived_test", "android_common")
}

func TestModuleConfigMultipleDerivedTestsWriteDistinctMakeEntries(t *testing.T) {
	multiBp := `
		android_test {
			name: "base",
			sdk_version: "current",
                        srcs: ["a.java"],
		}

                test_module_config {
                        name: "derived_test",
                        base: "base",
                        include_annotations: ["android.platform.test.annotations.LargeTest"],
                }

                test_module_config {
                        name: "another_derived_test",
                        base: "base",
                        include_annotations: ["android.platform.test.annotations.LargeTest"],
                }`

	ctx := android.GroupFixturePreparers(
		java.PrepareForTestWithJavaDefaultModules,
		android.FixtureRegisterWithContext(RegisterTestModuleConfigBuildComponents),
	).RunTestWithBp(t, multiBp)

	{
		derived := ctx.ModuleForTests("derived_test", "android_common")
		entries := android.AndroidMkEntriesForTest(t, ctx.TestContext, derived.Module())[0]
		// All these should be the same in both derived tests
		assertEntryPairValues(t, entries.EntryMap["LOCAL_COMPATIBILITY_SUPPORT_FILES"], []string{"HelperApp.apk", "data/testfile"})
		android.AssertArrayString(t, "", entries.EntryMap["LOCAL_SOONG_JNI_LIBS_SYMBOLS"], []string{""})
		// Except this one, which points to the updated tradefed xml file.
		android.AssertStringMatches(t, "", entries.EntryMap["LOCAL_FULL_TEST_CONFIG"][0], "derived_test/android_common/test_config_fixer/derived_test.config")
		// And this one, the module name.
		android.AssertArrayString(t, "", entries.EntryMap["LOCAL_MODULE"], []string{"derived_test"})
	}

	{
		derived := ctx.ModuleForTests("another_derived_test", "android_common")
		entries := android.AndroidMkEntriesForTest(t, ctx.TestContext, derived.Module())[0]
		// All these should be the same in both derived tests
		assertEntryPairValues(t, entries.EntryMap["LOCAL_COMPATIBILITY_SUPPORT_FILES"], []string{"HelperApp.apk", "data/testfile"})
		android.AssertArrayString(t, "", entries.EntryMap["LOCAL_SOONG_JNI_LIBS_SYMBOLS"], []string{""})
		// Except this one, which points to the updated tradefed xml file.
		android.AssertStringMatches(t, "", entries.EntryMap["LOCAL_FULL_TEST_CONFIG"][0], "another_derived_test/android_common/test_config_fixer/another_derived_test.config")
		// And this one, the module name.
		android.AssertArrayString(t, "", entries.EntryMap["LOCAL_MODULE"], []string{"another_derived_test"})
	}
}

// Use for situations where the entries map contains pairs:  [srcPath:installedPath1, srcPath2:installedPath2]
// and we want to compare the RHS of the pairs, i.e. installedPath1, installedPath2
func assertEntryPairValues(t *testing.T, actual []string, expected []string) {
	for i, e := range actual {
		parts := strings.Split(e, ":")
		if len(parts) != 2 {
			t.Errorf("Expected entry to have a value delimited by :, received: %s", e)
			return
		}
		android.AssertStringEquals(t, "", parts[1], expected[i])
	}
}
