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
		"myapex.avbpubkey":                             nil,
		"myapex.pem":                                   nil,
		"myapex.x509.pem":                              nil,
		"myapex.pk8":                                   nil,
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
)

var PrepareForTestWithSdkBuildComponents = android.GroupFixturePreparers(
	android.FixtureRegisterWithContext(registerModuleExportsBuildComponents),
	android.FixtureRegisterWithContext(registerSdkBuildComponents),
)

func testSdkWithFs(t *testing.T, bp string, fs android.MockFS) *android.TestResult {
	t.Helper()
	return prepareForSdkTest.RunTest(t, fs.AddToFixture(), android.FixtureWithRootAndroidBp(bp))
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
		androidBpContents:            sdk.GetAndroidBpContentsForTests(),
		androidUnversionedBpContents: sdk.GetUnversionedAndroidBpContentsForTests(),
		androidVersionedBpContents:   sdk.GetVersionedAndroidBpContentsForTests(),
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
	android.AssertStringEquals(t, "Snapshot zip file in wrong place",
		fmt.Sprintf(".intermediates/%s%s/%s/%s-current.zip", dir, name, variant, name), actual)

	// Populate a mock filesystem with the files that would have been copied by
	// the rules.
	fs := android.MockFS{}
	snapshotSubDir := "snapshot"
	for _, dest := range snapshotBuildInfo.snapshotContents {
		fs[filepath.Join(snapshotSubDir, dest)] = nil
	}
	fs[filepath.Join(snapshotSubDir, "Android.bp")] = []byte(snapshotBuildInfo.androidBpContents)

	preparer := result.Preparer()

	// Process the generated bp file to make sure it is valid. Use the same preparer as was used to
	// produce this result.
	t.Run("snapshot without source", func(t *testing.T) {
		android.GroupFixturePreparers(
			preparer,
			// TODO(b/183184375): Set Config.TestAllowNonExistentPaths = false to verify that all the
			//  files the snapshot needs are actually copied into the snapshot.

			// Add the files (including bp) created for this snapshot to the test fixture.
			fs.AddToFixture(),

			// Remove the source Android.bp file to make sure it works without.
			// TODO(b/183184375): Add a test with the source.
			android.FixtureModifyMockFS(func(fs android.MockFS) {
				delete(fs, "Android.bp")
			}),
		).RunTest(t)
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

// Encapsulates information about the snapshot build structure in order to insulate tests from
// knowing too much about internal structures.
//
// All source/input paths are relative either the build directory. All dest/output paths are
// relative to the snapshot root directory.
type snapshotBuildInfo struct {
	t *testing.T

	// The result from RunTest()
	r *android.TestResult

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
}
