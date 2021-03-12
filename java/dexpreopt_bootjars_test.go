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

package java

import (
	"path/filepath"
	"sort"
	"testing"

	"android/soong/android"
	"android/soong/dexpreopt"
)

func testDexpreoptBoot(t *testing.T, ruleFile string, expectedInputs, expectedOutputs []string) {
	bp := `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			api_packages: ["foo"],
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
			installable: true,
		}

		dex_import {
			name: "baz",
			jars: ["a.jar"],
		}
	`

	result := javaFixtureFactory.
		Extend(dexpreopt.FixtureSetBootJars("platform:foo", "platform:bar", "platform:baz")).
		RunTestWithBp(t, bp)

	dexpreoptBootJars := result.SingletonForTests("dex_bootjars")
	rule := dexpreoptBootJars.Output(ruleFile)

	for i := range expectedInputs {
		expectedInputs[i] = filepath.Join(buildDir, "test_device", expectedInputs[i])
	}

	for i := range expectedOutputs {
		expectedOutputs[i] = filepath.Join(buildDir, "test_device", expectedOutputs[i])
	}

	inputs := rule.Implicits.Strings()
	sort.Strings(inputs)
	sort.Strings(expectedInputs)

	outputs := append(android.WritablePaths{rule.Output}, rule.ImplicitOutputs...).Strings()
	sort.Strings(outputs)
	sort.Strings(expectedOutputs)

	result.AssertDeepEquals("inputs", expectedInputs, inputs)

	result.AssertDeepEquals("outputs", expectedOutputs, outputs)
}

func TestDexpreoptBootJars(t *testing.T) {
	ruleFile := "boot-foo.art"

	expectedInputs := []string{
		"dex_artjars/android/apex/art_boot_images/javalib/arm64/boot.art",
		"dex_bootjars_input/foo.jar",
		"dex_bootjars_input/bar.jar",
		"dex_bootjars_input/baz.jar",
	}

	expectedOutputs := []string{
		"dex_bootjars/android/system/framework/arm64/boot.invocation",
		"dex_bootjars/android/system/framework/arm64/boot-foo.art",
		"dex_bootjars/android/system/framework/arm64/boot-bar.art",
		"dex_bootjars/android/system/framework/arm64/boot-baz.art",
		"dex_bootjars/android/system/framework/arm64/boot-foo.oat",
		"dex_bootjars/android/system/framework/arm64/boot-bar.oat",
		"dex_bootjars/android/system/framework/arm64/boot-baz.oat",
		"dex_bootjars/android/system/framework/arm64/boot-foo.vdex",
		"dex_bootjars/android/system/framework/arm64/boot-bar.vdex",
		"dex_bootjars/android/system/framework/arm64/boot-baz.vdex",
		"dex_bootjars_unstripped/android/system/framework/arm64/boot-foo.oat",
		"dex_bootjars_unstripped/android/system/framework/arm64/boot-bar.oat",
		"dex_bootjars_unstripped/android/system/framework/arm64/boot-baz.oat",
	}

	testDexpreoptBoot(t, ruleFile, expectedInputs, expectedOutputs)
}

// Changes to the boot.zip structure may break the ART APK scanner.
func TestDexpreoptBootZip(t *testing.T) {
	ruleFile := "boot.zip"

	ctx := android.PathContextForTesting(testConfig(nil, "", nil))
	expectedInputs := []string{}
	for _, target := range ctx.Config().Targets[android.Android] {
		for _, ext := range []string{".art", ".oat", ".vdex"} {
			for _, jar := range []string{"foo", "bar", "baz"} {
				expectedInputs = append(expectedInputs,
					filepath.Join("dex_bootjars", target.Os.String(), "system/framework", target.Arch.ArchType.String(), "boot-"+jar+ext))
			}
		}
	}

	expectedOutputs := []string{
		"dex_bootjars/boot.zip",
	}

	testDexpreoptBoot(t, ruleFile, expectedInputs, expectedOutputs)
}
