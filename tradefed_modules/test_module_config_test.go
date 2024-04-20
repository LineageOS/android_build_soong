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
	"strconv"
	"strings"
	"testing"

	"github.com/google/blueprint"
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
                        test_suites: ["general-tests"],
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
func TestModuleConfigWithHostBaseShouldFailWithExplicitMessage(t *testing.T) {
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
                        test_suites: ["general-tests"],
                }`

	android.GroupFixturePreparers(
		java.PrepareForTestWithJavaDefaultModules,
		android.FixtureRegisterWithContext(RegisterTestModuleConfigBuildComponents),
	).ExtendWithErrorHandler(
		android.FixtureExpectsAtLeastOneErrorMatchingPattern("'java_test_host' module used as base, but 'android_test' expected")).
		RunTestWithBp(t, badBp)
}

func TestModuleConfigBadBaseShouldFailWithGeneralMessage(t *testing.T) {
	badBp := `
		java_library {
			name: "base",
                        srcs: ["a.java"],
		}

                test_module_config {
                        name: "derived_test",
                        base: "base",
                        exclude_filters: ["android.test.example.devcodelab.DevCodelabTest#testHelloFail"],
                        include_annotations: ["android.platform.test.annotations.LargeTest"],
                        test_suites: ["general-tests"],
                }`

	android.GroupFixturePreparers(
		java.PrepareForTestWithJavaDefaultModules,
		android.FixtureRegisterWithContext(RegisterTestModuleConfigBuildComponents),
	).ExtendWithErrorHandler(
		android.FixtureExpectsOneErrorPattern("'base' module used as base but it is not a 'android_test' module.")).
		RunTestWithBp(t, badBp)
}

func TestModuleConfigNoBaseShouldFail(t *testing.T) {
	badBp := `
		java_library {
			name: "base",
                        srcs: ["a.java"],
		}

                test_module_config {
                        name: "derived_test",
                        exclude_filters: ["android.test.example.devcodelab.DevCodelabTest#testHelloFail"],
                        include_annotations: ["android.platform.test.annotations.LargeTest"],
                        test_suites: ["general-tests"],
                }`

	android.GroupFixturePreparers(
		java.PrepareForTestWithJavaDefaultModules,
		android.FixtureRegisterWithContext(RegisterTestModuleConfigBuildComponents),
	).ExtendWithErrorHandler(
		android.FixtureExpectsOneErrorPattern("'base' field must be set to a 'android_test' module.")).
		RunTestWithBp(t, badBp)
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
                        test_suites: ["general-tests"],
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
                        test_suites: ["general-tests"],
                }

                test_module_config {
                        name: "another_derived_test",
                        base: "base",
                        include_annotations: ["android.platform.test.annotations.LargeTest"],
                        test_suites: ["general-tests"],
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

// Test_module_config_host rule is allowed to depend on java_test_host
func TestModuleConfigHostBasics(t *testing.T) {
	bp := `
               java_test_host {
                        name: "base",
                        srcs: ["a.java"],
                        test_suites: ["suiteA", "general-tests",  "suiteB"],
               }

                test_module_config_host {
                        name: "derived_test",
                        base: "base",
                        exclude_filters: ["android.test.example.devcodelab.DevCodelabTest#testHelloFail"],
                        include_annotations: ["android.platform.test.annotations.LargeTest"],
                        test_suites: ["general-tests"],
                }`

	ctx := android.GroupFixturePreparers(
		java.PrepareForTestWithJavaDefaultModules,
		android.FixtureRegisterWithContext(RegisterTestModuleConfigBuildComponents),
	).RunTestWithBp(t, bp)

	variant := ctx.Config.BuildOS.String() + "_common"
	derived := ctx.ModuleForTests("derived_test", variant)
	mod := derived.Module().(*testModuleConfigHostModule)
	allEntries := android.AndroidMkEntriesForTest(t, ctx.TestContext, mod)
	entries := allEntries[0]
	android.AssertArrayString(t, "", entries.EntryMap["LOCAL_MODULE"], []string{"derived_test"})

	if !mod.Host() {
		t.Errorf("host bit is not set for a java_test_host module.")
	}
	actualData, _ := strconv.ParseBool(entries.EntryMap["LOCAL_IS_UNIT_TEST"][0])
	android.AssertBoolEquals(t, "LOCAL_IS_UNIT_TEST", true, actualData)

}

// When you pass an 'android_test' as base, the warning message is a bit obscure,
// talking about variants, but it is something.  Ideally we could do better.
func TestModuleConfigHostBadBaseShouldFailWithVariantWarning(t *testing.T) {
	badBp := `
		android_test {
			name: "base",
			sdk_version: "current",
                        srcs: ["a.java"],
		}

                test_module_config_host {
                        name: "derived_test",
                        base: "base",
                        exclude_filters: ["android.test.example.devcodelab.DevCodelabTest#testHelloFail"],
                        include_annotations: ["android.platform.test.annotations.LargeTest"],
                }`

	android.GroupFixturePreparers(
		java.PrepareForTestWithJavaDefaultModules,
		android.FixtureRegisterWithContext(RegisterTestModuleConfigBuildComponents),
	).ExtendWithErrorHandler(
		android.FixtureExpectsAtLeastOneErrorMatchingPattern("missing variant")).
		RunTestWithBp(t, badBp)
}

func TestModuleConfigHostNeedsATestSuite(t *testing.T) {
	badBp := `
		java_test_host {
			name: "base",
                        srcs: ["a.java"],
		}

                test_module_config_host {
                        name: "derived_test",
                        base: "base",
                        exclude_filters: ["android.test.example.devcodelab.DevCodelabTest#testHelloFail"],
                        include_annotations: ["android.platform.test.annotations.LargeTest"],
                }`

	android.GroupFixturePreparers(
		java.PrepareForTestWithJavaDefaultModules,
		android.FixtureRegisterWithContext(RegisterTestModuleConfigBuildComponents),
	).ExtendWithErrorHandler(
		android.FixtureExpectsAtLeastOneErrorMatchingPattern("At least one test-suite must be set")).
		RunTestWithBp(t, badBp)
}

func TestModuleConfigHostDuplicateTestSuitesGiveErrors(t *testing.T) {
	badBp := `
		java_test_host {
			name: "base",
                        srcs: ["a.java"],
                        test_suites: ["general-tests", "some-compat"],
		}

                test_module_config_host {
                        name: "derived_test",
                        base: "base",
                        exclude_filters: ["android.test.example.devcodelab.DevCodelabTest#testHelloFail"],
                        include_annotations: ["android.platform.test.annotations.LargeTest"],
                        test_suites: ["general-tests", "some-compat"],
                }`

	android.GroupFixturePreparers(
		java.PrepareForTestWithJavaDefaultModules,
		android.FixtureRegisterWithContext(RegisterTestModuleConfigBuildComponents),
	).ExtendWithErrorHandler(
		android.FixtureExpectsAtLeastOneErrorMatchingPattern("TestSuite some-compat exists in the base")).
		RunTestWithBp(t, badBp)
}

func TestTestOnlyProvider(t *testing.T) {
	t.Parallel()
	ctx := android.GroupFixturePreparers(
		java.PrepareForTestWithJavaDefaultModules,
		android.FixtureRegisterWithContext(RegisterTestModuleConfigBuildComponents),
	).RunTestWithBp(t, `
                // These should be test-only
                test_module_config_host {
                        name: "host-derived-test",
                        base: "host-base",
                        exclude_filters: ["android.test.example.devcodelab.DevCodelabTest#testHelloFail"],
                        include_annotations: ["android.platform.test.annotations.LargeTest"],
                        test_suites: ["general-tests"],
                }

                test_module_config {
                        name: "derived-test",
                        base: "base",
                        exclude_filters: ["android.test.example.devcodelab.DevCodelabTest#testHelloFail"],
                        include_annotations: ["android.platform.test.annotations.LargeTest"],
                        test_suites: ["general-tests"],
                }

		android_test {
			name: "base",
			sdk_version: "current",
                        data: ["data/testfile"],
		}

		java_test_host {
			name: "host-base",
                        srcs: ["a.java"],
                        test_suites: ["general-tests"],
		}`,
	)

	// Visit all modules and ensure only the ones that should
	// marked as test-only are marked as test-only.

	actualTestOnly := []string{}
	ctx.VisitAllModules(func(m blueprint.Module) {
		if provider, ok := android.OtherModuleProvider(ctx.TestContext.OtherModuleProviderAdaptor(), m, android.TestOnlyProviderKey); ok {
			if provider.TestOnly {
				actualTestOnly = append(actualTestOnly, m.Name())
			}
		}
	})
	expectedTestOnlyModules := []string{
		"host-derived-test",
		"derived-test",
		// android_test and java_test_host are tests too.
		"host-base",
		"base",
	}

	notEqual, left, right := android.ListSetDifference(expectedTestOnlyModules, actualTestOnly)
	if notEqual {
		t.Errorf("test-only: Expected but not found: %v, Found but not expected: %v", left, right)
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
