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

package java

import (
	"reflect"
	"testing"

	"android/soong/android"
)

func TestAndroidAppSet(t *testing.T) {
	ctx, config := testJava(t, `
		android_app_set {
			name: "foo",
			set: "prebuilts/apks/app.apks",
			prerelease: true,
		}`)
	module := ctx.ModuleForTests("foo", "android_common")
	const packedSplitApks = "foo.zip"
	params := module.Output(packedSplitApks)
	if params.Rule == nil {
		t.Errorf("expected output %s is missing", packedSplitApks)
	}
	if s := params.Args["allow-prereleased"]; s != "true" {
		t.Errorf("wrong allow-prereleased value: '%s', expected 'true'", s)
	}
	if s := params.Args["partition"]; s != "system" {
		t.Errorf("wrong partition value: '%s', expected 'system'", s)
	}
	mkEntries := android.AndroidMkEntriesForTest(t, config, "", module.Module())[0]
	actualInstallFile := mkEntries.EntryMap["LOCAL_APK_SET_INSTALL_FILE"]
	expectedInstallFile := []string{"foo.apk"}
	if !reflect.DeepEqual(actualInstallFile, expectedInstallFile) {
		t.Errorf("Unexpected LOCAL_APK_SET_INSTALL_FILE value: '%s', expected: '%s',",
			actualInstallFile, expectedInstallFile)
	}
}

func TestAndroidAppSet_Variants(t *testing.T) {
	bp := `
		android_app_set {
			name: "foo",
			set: "prebuilts/apks/app.apks",
		}`
	testCases := []struct {
		name            string
		targets         []android.Target
		aaptPrebuiltDPI []string
		sdkVersion      int
		expected        map[string]string
	}{
		{
			name: "One",
			targets: []android.Target{
				{Os: android.Android, Arch: android.Arch{ArchType: android.X86}},
			},
			aaptPrebuiltDPI: []string{"ldpi", "xxhdpi"},
			sdkVersion:      29,
			expected: map[string]string{
				"abis":              "X86",
				"allow-prereleased": "false",
				"screen-densities":  "LDPI,XXHDPI",
				"sdk-version":       "29",
				"stem":              "foo",
			},
		},
		{
			name: "Two",
			targets: []android.Target{
				{Os: android.Android, Arch: android.Arch{ArchType: android.X86_64}},
				{Os: android.Android, Arch: android.Arch{ArchType: android.X86}},
			},
			aaptPrebuiltDPI: nil,
			sdkVersion:      30,
			expected: map[string]string{
				"abis":              "X86_64,X86",
				"allow-prereleased": "false",
				"screen-densities":  "all",
				"sdk-version":       "30",
				"stem":              "foo",
			},
		},
	}

	for _, test := range testCases {
		config := testAppConfig(nil, bp, nil)
		config.TestProductVariables.AAPTPrebuiltDPI = test.aaptPrebuiltDPI
		config.TestProductVariables.Platform_sdk_version = &test.sdkVersion
		config.Targets[android.Android] = test.targets
		ctx := testContext(config)
		run(t, ctx, config)
		module := ctx.ModuleForTests("foo", "android_common")
		const packedSplitApks = "foo.zip"
		params := module.Output(packedSplitApks)
		for k, v := range test.expected {
			if actual := params.Args[k]; actual != v {
				t.Errorf("%s: bad build arg value for '%s': '%s', expected '%s'",
					test.name, k, actual, v)
			}
		}
	}
}
