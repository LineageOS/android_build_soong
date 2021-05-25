// Copyright 2019 Google Inc. All rights reserved.
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
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/apex"
	"android/soong/cc"
	"android/soong/genrule"
	"android/soong/java"
)

// Prepare for running an sdk test with an apex.
var prepareForSdkTestWithApex = android.GroupFixturePreparers(
	apex.PrepareForTestWithApexBuildComponents,
	android.FixtureAddTextFile("sdk/tests/Android.bp", `
		apex_key {
			name: "myapex.key",
			public_key: "myapex.avbpubkey",
			private_key: "myapex.pem",
		}

		android_app_certificate {
			name: "myapex.cert",
			certificate: "myapex",
		}
	`),

	android.FixtureMergeMockFs(map[string][]byte{
		"apex_manifest.json":                           nil,
		"system/sepolicy/apex/myapex-file_contexts":    nil,
		"system/sepolicy/apex/myapex2-file_contexts":   nil,
		"system/sepolicy/apex/mysdkapex-file_contexts": nil,
		"sdk/tests/myapex.avbpubkey":                   nil,
		"sdk/tests/myapex.pem":                         nil,
		"sdk/tests/myapex.x509.pem":                    nil,
		"sdk/tests/myapex.pk8":                         nil,
	}),
)

// Legacy preparer used for running tests within the sdk package.
//
// This includes everything that was needed to run any test in the sdk package prior to the
// introduction of the test fixtures. Tests that are being converted to use fixtures directly
// rather than through the testSdkError() and testSdkWithFs() methods should avoid using this and
// instead should use the various preparers directly using android.GroupFixturePreparers(...) to
// group them when necessary.
//
// deprecated
var prepareForSdkTest = android.GroupFixturePreparers(
	cc.PrepareForTestWithCcDefaultModules,
	genrule.PrepareForTestWithGenRuleBuildComponents,
	java.PrepareForTestWithJavaBuildComponents,
	PrepareForTestWithSdkBuildComponents,

	prepareForSdkTestWithApex,

	cc.PrepareForTestOnWindows,
	android.FixtureModifyConfig(func(config android.Config) {
		// Add windows as a default disable OS to test behavior when some OS variants
		// are disabled.
		config.Targets[android.Windows] = []android.Target{
			{android.Windows, android.Arch{ArchType: android.X86_64}, android.NativeBridgeDisabled, "", "", true},
		}
	}),

	// Make sure that every test provides all the source files.
	android.PrepareForTestDisallowNonExistentPaths,
	android.MockFS{
		"Test.java": nil,
	}.AddToFixture(),
)

var PrepareForTestWithSdkBuildComponents = android.GroupFixturePreparers(
	android.FixtureRegisterWithContext(registerModuleExportsBuildComponents),
	android.FixtureRegisterWithContext(registerSdkBuildComponents),
)

func testSdkWithFs(t *testing.T, bp string, fs android.MockFS) *android.TestResult {
	t.Helper()
	return android.GroupFixturePreparers(
		prepareForSdkTest,
		fs.AddToFixture(),
	).RunTestWithBp(t, bp)
}

func testSdkError(t *testing.T, pattern, bp string) {
	t.Helper()
	prepareForSdkTest.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(pattern)).
		RunTestWithBp(t, bp)
}

func ensureListContains(t *testing.T, result []string, expected string) {
	t.Helper()
	if !android.InList(expected, result) {
		t.Errorf("%q is not found in %v", expected, result)
	}
}

func pathsToStrings(paths android.Paths) []string {
	var ret []string
	for _, p := range paths {
		ret = append(ret, p.String())
	}
	return ret
}

// Analyse the sdk build rules to extract information about what it is doing.
//
// e.g. find the src/dest pairs from each cp command, the various zip files
// generated, etc.
func getSdkSnapshotBuildInfo(t *testing.T, result *android.TestResult, sdk *sdk) *snapshotBuildInfo {
	info := &snapshotBuildInfo{
		t:                            t,
		r:                            result,
		version:                      sdk.builderForTests.version,
		androidBpContents:            sdk.GetAndroidBpContentsForTests(),
		androidUnversionedBpContents: sdk.GetUnversionedAndroidBpContentsForTests(),
		androidVersionedBpContents:   sdk.GetVersionedAndroidBpContentsForTests(),
		snapshotTestCustomizations:   map[snapshotTest]*snapshotTestCustomization{},
	}

	buildParams := sdk.BuildParamsForTests()
	copyRules := &strings.Builder{}
	otherCopyRules := &strings.Builder{}
	snapshotDirPrefix := sdk.builderForTests.snapshotDir.String() + "/"
	for _, bp := range buildParams {
		switch bp.Rule.String() {
		case android.Cp.String():
			output := bp.Output
			// Get destination relative to the snapshot root
			dest := output.Rel()
			src := android.NormalizePathForTesting(bp.Input)
			// We differentiate between copy rules for the snapshot, and copy rules for the install file.
			if strings.HasPrefix(output.String(), snapshotDirPrefix) {
				// Get source relative to build directory.
				_, _ = fmt.Fprintf(copyRules, "%s -> %s\n", src, dest)
				info.snapshotContents = append(info.snapshotContents, dest)
			} else {
				_, _ = fmt.Fprintf(otherCopyRules, "%s -> %s\n", src, dest)
			}

		case repackageZip.String():
			// Add the destdir to the snapshot contents as that is effectively where
			// the content of the repackaged zip is copied.
			dest := bp.Args["destdir"]
			info.snapshotContents = append(info.snapshotContents, dest)

		case zipFiles.String():
			// This could be an intermediate zip file and not the actual output zip.
			// In that case this will be overridden when the rule to merge the zips
			// is processed.
			info.outputZip = android.NormalizePathForTesting(bp.Output)

		case mergeZips.String():
			// Copy the current outputZip to the intermediateZip.
			info.intermediateZip = info.outputZip
			mergeInput := android.NormalizePathForTesting(bp.Input)
			if info.intermediateZip != mergeInput {
				t.Errorf("Expected intermediate zip %s to be an input to merge zips but found %s instead",
					info.intermediateZip, mergeInput)
			}

			// Override output zip (which was actually the intermediate zip file) with the actual
			// output zip.
			info.outputZip = android.NormalizePathForTesting(bp.Output)

			// Save the zips to be merged into the intermediate zip.
			info.mergeZips = android.NormalizePathsForTesting(bp.Inputs)
		}
	}

	info.copyRules = copyRules.String()
	info.otherCopyRules = otherCopyRules.String()

	return info
}

// The enum of different sdk snapshot tests performed by CheckSnapshot.
type snapshotTest int

const (
	// The enumeration of the different test configurations.
	// A test with the snapshot/Android.bp file but without the original Android.bp file.
	checkSnapshotWithoutSource snapshotTest = iota

	// A test with both the original source and the snapshot, with the source preferred.
	checkSnapshotWithSourcePreferred

	// A test with both the original source and the snapshot, with the snapshot preferred.
	checkSnapshotPreferredWithSource

	// The directory into which the snapshot will be 'unpacked'.
	snapshotSubDir = "snapshot"
)

// Check the snapshot build rules.
//
// Takes a list of functions which check different facets of the snapshot build rules.
// Allows each test to customize what is checked without duplicating lots of code
// or proliferating check methods of different flavors.
func CheckSnapshot(t *testing.T, result *android.TestResult, name string, dir string, checkers ...snapshotBuildInfoChecker) {
	t.Helper()

	// The sdk CommonOS variant is always responsible for generating the snapshot.
	variant := android.CommonOS.Name

	sdk := result.Module(name, variant).(*sdk)

	snapshotBuildInfo := getSdkSnapshotBuildInfo(t, result, sdk)

	// Check state of the snapshot build.
	for _, checker := range checkers {
		checker(snapshotBuildInfo)
	}

	// Make sure that the generated zip file is in the correct place.
	actual := snapshotBuildInfo.outputZip
	if dir != "" {
		dir = filepath.Clean(dir) + "/"
	}
	suffix := ""
	if snapshotBuildInfo.version != soongSdkSnapshotVersionUnversioned {
		suffix = "-" + snapshotBuildInfo.version
	}

	expectedZipPath := fmt.Sprintf(".intermediates/%s%s/%s/%s%s.zip", dir, name, variant, name, suffix)
	android.AssertStringEquals(t, "Snapshot zip file in wrong place", expectedZipPath, actual)

	// Populate a mock filesystem with the files that would have been copied by
	// the rules.
	fs := android.MockFS{}
	for _, dest := range snapshotBuildInfo.snapshotContents {
		fs[filepath.Join(snapshotSubDir, dest)] = nil
	}
	fs[filepath.Join(snapshotSubDir, "Android.bp")] = []byte(snapshotBuildInfo.androidBpContents)

	// The preparers from the original source fixture.
	sourcePreparers := result.Preparer()

	// Preparer to combine the snapshot and the source.
	snapshotPreparer := android.GroupFixturePreparers(sourcePreparers, fs.AddToFixture())

	var runSnapshotTestWithCheckers = func(t *testing.T, testConfig snapshotTest, extraPreparer android.FixturePreparer) {
		t.Helper()
		customization := snapshotBuildInfo.snapshotTestCustomization(testConfig)
		customizedPreparers := android.GroupFixturePreparers(customization.preparers...)

		// TODO(b/183184375): Set Config.TestAllowNonExistentPaths = false to verify that all the
		//  files the snapshot needs are actually copied into the snapshot.

		// Run the snapshot with the snapshot preparer and the extra preparer, which must come after as
		// it may need to modify parts of the MockFS populated by the snapshot preparer.
		result := android.GroupFixturePreparers(snapshotPreparer, extraPreparer, customizedPreparers).
			ExtendWithErrorHandler(customization.errorHandler).
			RunTest(t)

		// Perform any additional checks the test need on the result of processing the snapshot.
		for _, checker := range customization.checkers {
			checker(t, result)
		}
	}

	t.Run("snapshot without source", func(t *testing.T) {
		// Remove the source Android.bp file to make sure it works without.
		removeSourceAndroidBp := android.FixtureModifyMockFS(func(fs android.MockFS) {
			delete(fs, "Android.bp")
		})

		runSnapshotTestWithCheckers(t, checkSnapshotWithoutSource, removeSourceAndroidBp)
	})

	t.Run("snapshot with source preferred", func(t *testing.T) {
		runSnapshotTestWithCheckers(t, checkSnapshotWithSourcePreferred, android.NullFixturePreparer)
	})

	t.Run("snapshot preferred with source", func(t *testing.T) {
		// Replace the snapshot/Android.bp file with one where "prefer: false," has been replaced with
		// "prefer: true,"
		preferPrebuilts := android.FixtureModifyMockFS(func(fs android.MockFS) {
			snapshotBpFile := filepath.Join(snapshotSubDir, "Android.bp")
			unpreferred := string(fs[snapshotBpFile])
			fs[snapshotBpFile] = []byte(strings.ReplaceAll(unpreferred, "prefer: false,", "prefer: true,"))
		})

		runSnapshotTestWithCheckers(t, checkSnapshotPreferredWithSource, preferPrebuilts)
	})
}

type snapshotBuildInfoChecker func(info *snapshotBuildInfo)

// Check that the snapshot's generated Android.bp is correct.
//
// Both the expected and actual string are both trimmed before comparing.
func checkAndroidBpContents(expected string) snapshotBuildInfoChecker {
	return func(info *snapshotBuildInfo) {
		info.t.Helper()
		android.AssertTrimmedStringEquals(info.t, "Android.bp contents do not match", expected, info.androidBpContents)
	}
}

// Check that the snapshot's unversioned generated Android.bp is correct.
//
// This func should be used to check the general snapshot generation code.
//
// Both the expected and actual string are both trimmed before comparing.
func checkUnversionedAndroidBpContents(expected string) snapshotBuildInfoChecker {
	return func(info *snapshotBuildInfo) {
		info.t.Helper()
		android.AssertTrimmedStringEquals(info.t, "unversioned Android.bp contents do not match", expected, info.androidUnversionedBpContents)
	}
}

// Check that the snapshot's versioned generated Android.bp is correct.
//
// This func should only be used to check the version specific snapshot generation code,
// i.e. the encoding of version into module names and the generation of the _snapshot module. The
// general snapshot generation code should be checked using the checkUnversionedAndroidBpContents()
// func.
//
// Both the expected and actual string are both trimmed before comparing.
func checkVersionedAndroidBpContents(expected string) snapshotBuildInfoChecker {
	return func(info *snapshotBuildInfo) {
		info.t.Helper()
		android.AssertTrimmedStringEquals(info.t, "versioned Android.bp contents do not match", expected, info.androidVersionedBpContents)
	}
}

// Check that the snapshot's copy rules are correct.
//
// The copy rules are formatted as <src> -> <dest>, one per line and then compared
// to the supplied expected string. Both the expected and actual string are trimmed
// before comparing.
func checkAllCopyRules(expected string) snapshotBuildInfoChecker {
	return func(info *snapshotBuildInfo) {
		info.t.Helper()
		android.AssertTrimmedStringEquals(info.t, "Incorrect copy rules", expected, info.copyRules)
	}
}

func checkAllOtherCopyRules(expected string) snapshotBuildInfoChecker {
	return func(info *snapshotBuildInfo) {
		info.t.Helper()
		android.AssertTrimmedStringEquals(info.t, "Incorrect copy rules", expected, info.otherCopyRules)
	}
}

// Check that the specified paths match the list of zips to merge with the intermediate zip.
func checkMergeZips(expected ...string) snapshotBuildInfoChecker {
	return func(info *snapshotBuildInfo) {
		info.t.Helper()
		if info.intermediateZip == "" {
			info.t.Errorf("No intermediate zip file was created")
		}

		android.AssertDeepEquals(info.t, "mismatching merge zip files", expected, info.mergeZips)
	}
}

type resultChecker func(t *testing.T, result *android.TestResult)

// snapshotTestPreparer registers a preparer that will be used to customize the specified
// snapshotTest.
func snapshotTestPreparer(snapshotTest snapshotTest, preparer android.FixturePreparer) snapshotBuildInfoChecker {
	return func(info *snapshotBuildInfo) {
		customization := info.snapshotTestCustomization(snapshotTest)
		customization.preparers = append(customization.preparers, preparer)
	}
}

// snapshotTestChecker registers a checker that will be run against the result of processing the
// generated snapshot for the specified snapshotTest.
func snapshotTestChecker(snapshotTest snapshotTest, checker resultChecker) snapshotBuildInfoChecker {
	return func(info *snapshotBuildInfo) {
		customization := info.snapshotTestCustomization(snapshotTest)
		customization.checkers = append(customization.checkers, checker)
	}
}

// snapshotTestErrorHandler registers an error handler to use when processing the snapshot
// in the specific test case.
//
// Generally, the snapshot should work with all the test cases but some do not and just in case
// there are a lot of issues to resolve, or it will take a lot of time this is a
// get-out-of-jail-free card that allows progress to be made.
//
// deprecated: should only be used as a temporary workaround with an attached to do and bug.
func snapshotTestErrorHandler(snapshotTest snapshotTest, handler android.FixtureErrorHandler) snapshotBuildInfoChecker {
	return func(info *snapshotBuildInfo) {
		customization := info.snapshotTestCustomization(snapshotTest)
		customization.errorHandler = handler
	}
}

// Encapsulates information provided by each test to customize a specific snapshotTest.
type snapshotTestCustomization struct {
	// Preparers that are used to customize the test fixture before running the test.
	preparers []android.FixturePreparer

	// Checkers that are run on the result of processing the preferred snapshot in a specific test
	// case.
	checkers []resultChecker

	// Specify an error handler for when processing a specific test case.
	//
	// In some cases the generated snapshot cannot be used in a test configuration. Those cases are
	// invariably bugs that need to be resolved but sometimes that can take a while. This provides a
	// mechanism to temporarily ignore that error.
	errorHandler android.FixtureErrorHandler
}

// Encapsulates information about the snapshot build structure in order to insulate tests from
// knowing too much about internal structures.
//
// All source/input paths are relative either the build directory. All dest/output paths are
// relative to the snapshot root directory.
type snapshotBuildInfo struct {
	t *testing.T

	// The result from RunTest()
	r *android.TestResult

	// The version of the generated snapshot.
	//
	// See snapshotBuilder.version for more information about this field.
	version string

	// The contents of the generated Android.bp file
	androidBpContents string

	// The contents of the unversioned Android.bp file
	androidUnversionedBpContents string

	// The contents of the versioned Android.bp file
	androidVersionedBpContents string

	// The paths, relative to the snapshot root, of all files and directories copied into the
	// snapshot.
	snapshotContents []string

	// A formatted representation of the src/dest pairs for a snapshot, one pair per line,
	// of the format src -> dest
	copyRules string

	// A formatted representation of the src/dest pairs for files not in a snapshot, one pair
	// per line, of the format src -> dest
	otherCopyRules string

	// The path to the intermediate zip, which is a zip created from the source files copied
	// into the snapshot directory and which will be merged with other zips to form the final output.
	// Is am empty string if there is no intermediate zip because there are no zips to merge in.
	intermediateZip string

	// The paths to the zips to merge into the output zip, does not include the intermediate
	// zip.
	mergeZips []string

	// The final output zip.
	outputZip string

	// The test specific customizations for each snapshot test.
	snapshotTestCustomizations map[snapshotTest]*snapshotTestCustomization
}

// snapshotTestCustomization gets the test specific customization for the specified snapshotTest.
//
// If no customization was created previously then it creates a default customization.
func (i *snapshotBuildInfo) snapshotTestCustomization(snapshotTest snapshotTest) *snapshotTestCustomization {
	customization := i.snapshotTestCustomizations[snapshotTest]
	if customization == nil {
		customization = &snapshotTestCustomization{
			errorHandler: android.FixtureExpectsNoErrors,
		}
		i.snapshotTestCustomizations[snapshotTest] = customization
	}
	return customization
}
