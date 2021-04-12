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
	"android/soong/dexpreopt"
)

// Contains some simple tests for platform_bootclasspath.

var prepareForTestWithPlatformBootclasspath = android.GroupFixturePreparers(
	PrepareForTestWithJavaDefaultModules,
	dexpreopt.PrepareForTestByEnablingDexpreopt,
)

func TestPlatformBootclasspath(t *testing.T) {
	preparer := android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		dexpreopt.FixtureSetBootJars("platform:foo", "platform:bar"),
		android.FixtureWithRootAndroidBp(`
			platform_bootclasspath {
				name: "platform-bootclasspath",
			}

			java_library {
				name: "bar",
				srcs: ["a.java"],
				system_modules: "none",
				sdk_version: "none",
				compile_dex: true,
			}
		`),
	)

	var addSourceBootclassPathModule = android.FixtureAddTextFile("source/Android.bp", `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
		}
	`)

	var addPrebuiltBootclassPathModule = android.FixtureAddTextFile("prebuilt/Android.bp", `
		java_import {
			name: "foo",
			jars: ["a.jar"],
			compile_dex: true,
			prefer: false,
		}
	`)

	var addPrebuiltPreferredBootclassPathModule = android.FixtureAddTextFile("prebuilt/Android.bp", `
		java_import {
			name: "foo",
			jars: ["a.jar"],
			compile_dex: true,
			prefer: true,
		}
	`)

	t.Run("missing", func(t *testing.T) {
		preparer.
			ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(`"platform-bootclasspath" depends on undefined module "foo"`)).
			RunTest(t)
	})

	t.Run("source", func(t *testing.T) {
		result := android.GroupFixturePreparers(
			preparer,
			addSourceBootclassPathModule,
		).RunTest(t)

		CheckPlatformBootclasspathModules(t, result, "platform-bootclasspath", []string{
			"platform:foo",
			"platform:bar",
		})
	})

	t.Run("prebuilt", func(t *testing.T) {
		result := android.GroupFixturePreparers(
			preparer,
			addPrebuiltBootclassPathModule,
		).RunTest(t)

		CheckPlatformBootclasspathModules(t, result, "platform-bootclasspath", []string{
			"platform:prebuilt_foo",
			"platform:bar",
		})
	})

	t.Run("source+prebuilt - source preferred", func(t *testing.T) {
		result := android.GroupFixturePreparers(
			preparer,
			addSourceBootclassPathModule,
			addPrebuiltBootclassPathModule,
		).RunTest(t)

		CheckPlatformBootclasspathModules(t, result, "platform-bootclasspath", []string{
			"platform:foo",
			"platform:bar",
		})
	})

	t.Run("source+prebuilt - prebuilt preferred", func(t *testing.T) {
		result := android.GroupFixturePreparers(
			preparer,
			addSourceBootclassPathModule,
			addPrebuiltPreferredBootclassPathModule,
		).RunTest(t)

		CheckPlatformBootclasspathModules(t, result, "platform-bootclasspath", []string{
			"platform:prebuilt_foo",
			"platform:bar",
		})
	})
}

func TestPlatformBootclasspath_Dist(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		dexpreopt.FixtureSetBootJars("platform:foo", "platform:bar"),
		android.PrepareForTestWithAndroidMk,
		android.FixtureWithRootAndroidBp(`
			platform_bootclasspath {
				name: "platform-bootclasspath",
				dists: [
					{
						targets: ["droidcore"],
						tag: "hiddenapi-flags.csv",
					},
				],
			}

			java_library {
				name: "bar",
				srcs: ["a.java"],
				system_modules: "none",
				sdk_version: "none",
				compile_dex: true,
			}

			java_library {
				name: "foo",
				srcs: ["a.java"],
				system_modules: "none",
				sdk_version: "none",
				compile_dex: true,
			}
		`),
	).RunTest(t)

	platformBootclasspath := result.Module("platform-bootclasspath", "android_common").(*platformBootclasspathModule)
	entries := android.AndroidMkEntriesForTest(t, result.TestContext, platformBootclasspath)
	goals := entries[0].GetDistForGoals(platformBootclasspath)
	android.AssertStringEquals(t, "platform dist goals phony", ".PHONY: droidcore\n", goals[0])
	android.AssertStringEquals(t, "platform dist goals call", "$(call dist-for-goals,droidcore,out/soong/hiddenapi/hiddenapi-flags.csv:hiddenapi-flags.csv)\n", android.StringRelativeToTop(result.Config, goals[1]))
}
