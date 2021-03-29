// Copyright (C) 2020 The Android Open Source Project
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

package linkerconfig

import (
	"os"
	"reflect"
	"testing"

	"android/soong/android"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

var prepareForLinkerConfigTest = android.GroupFixturePreparers(
	android.PrepareForTestWithAndroidBuildComponents,
	android.FixtureRegisterWithContext(registerLinkerConfigBuildComponent),
	android.FixtureAddFile("linker.config.json", nil),
)

func TestBaseLinkerConfig(t *testing.T) {
	result := prepareForLinkerConfigTest.RunTestWithBp(t, `
		linker_config {
			name: "linker-config-base",
			src: "linker.config.json",
		}
	`)

	expected := map[string][]string{
		"LOCAL_MODULE":                {"linker-config-base"},
		"LOCAL_MODULE_CLASS":          {"ETC"},
		"LOCAL_INSTALLED_MODULE_STEM": {"linker.config.pb"},
	}

	p := result.ModuleForTests("linker-config-base", "android_arm64_armv8-a").Module().(*linkerConfig)

	if p.outputFilePath.Base() != "linker.config.pb" {
		t.Errorf("expected linker.config.pb, got %q", p.outputFilePath.Base())
	}

	entries := android.AndroidMkEntriesForTest(t, result.TestContext, p)[0]
	for k, expectedValue := range expected {
		if value, ok := entries.EntryMap[k]; ok {
			if !reflect.DeepEqual(value, expectedValue) {
				t.Errorf("Value of %s is '%s', but expected as '%s'", k, value, expectedValue)
			}
		} else {
			t.Errorf("%s is not defined", k)
		}
	}

	if value, ok := entries.EntryMap["LOCAL_UNINSTALLABLE_MODULE"]; ok {
		t.Errorf("Value of LOCAL_UNINSTALLABLE_MODULE is %s, but expected as empty", value)
	}
}

func TestUninstallableLinkerConfig(t *testing.T) {
	result := prepareForLinkerConfigTest.RunTestWithBp(t, `
		linker_config {
			name: "linker-config-base",
			src: "linker.config.json",
			installable: false,
		}
	`)

	expected := []string{"true"}

	p := result.ModuleForTests("linker-config-base", "android_arm64_armv8-a").Module().(*linkerConfig)
	entries := android.AndroidMkEntriesForTest(t, result.TestContext, p)[0]
	if value, ok := entries.EntryMap["LOCAL_UNINSTALLABLE_MODULE"]; ok {
		if !reflect.DeepEqual(value, expected) {
			t.Errorf("LOCAL_UNINSTALLABLE_MODULE is expected to be true but %s", value)
		}
	} else {
		t.Errorf("LOCAL_UNINSTALLABLE_MODULE is not defined")
	}
}
