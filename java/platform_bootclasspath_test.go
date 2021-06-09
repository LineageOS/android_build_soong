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
		FixtureConfigureBootJars("platform:foo", "system_ext:bar"),
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
				system_ext_specific: true,
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

	t.Run("dex import", func(t *testing.T) {
		result := android.GroupFixturePreparers(
			preparer,
			android.FixtureAddTextFile("deximport/Android.bp", `
				dex_import {
					name: "foo",
					jars: ["a.jar"],
				}
			`),
		).RunTest(t)

		CheckPlatformBootclasspathModules(t, result, "platform-bootclasspath", []string{
			"platform:prebuilt_foo",
			"platform:bar",
		})
	})
}

func TestPlatformBootclasspathVariant(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		android.FixtureWithRootAndroidBp(`
			platform_bootclasspath {
				name: "platform-bootclasspath",
			}
		`),
	).RunTest(t)

	variants := result.ModuleVariantsForTests("platform-bootclasspath")
	android.AssertIntEquals(t, "expect 1 variant", 1, len(variants))
}

func TestPlatformBootclasspath_ClasspathFragmentPaths(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		android.FixtureWithRootAndroidBp(`
			platform_bootclasspath {
				name: "platform-bootclasspath",
			}
		`),
	).RunTest(t)

	p := result.Module("platform-bootclasspath", "android_common").(*platformBootclasspathModule)
	android.AssertStringEquals(t, "output filepath", "bootclasspath.pb", p.ClasspathFragmentBase.outputFilepath.Base())
	android.AssertPathRelativeToTopEquals(t, "install filepath", "out/soong/target/product/test_device/system/etc/classpaths", p.ClasspathFragmentBase.installDirPath)
}

func TestPlatformBootclasspathModule_AndroidMkEntries(t *testing.T) {
	preparer := android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		android.FixtureWithRootAndroidBp(`
			platform_bootclasspath {
				name: "platform-bootclasspath",
			}
		`),
	)

	t.Run("AndroidMkEntries", func(t *testing.T) {
		result := preparer.RunTest(t)

		p := result.Module("platform-bootclasspath", "android_common").(*platformBootclasspathModule)

		entries := android.AndroidMkEntriesForTest(t, result.TestContext, p)
		android.AssertIntEquals(t, "AndroidMkEntries count", 2, len(entries))
	})

	t.Run("hiddenapi-flags-entry", func(t *testing.T) {
		result := preparer.RunTest(t)

		p := result.Module("platform-bootclasspath", "android_common").(*platformBootclasspathModule)

		entries := android.AndroidMkEntriesForTest(t, result.TestContext, p)
		got := entries[0].OutputFile
		android.AssertBoolEquals(t, "valid output path", true, got.Valid())
		android.AssertSame(t, "output filepath", p.hiddenAPIFlagsCSV, got.Path())
	})

	t.Run("classpath-fragment-entry", func(t *testing.T) {
		result := preparer.RunTest(t)

		want := map[string][]string{
			"LOCAL_MODULE":                {"platform-bootclasspath"},
			"LOCAL_MODULE_CLASS":          {"ETC"},
			"LOCAL_INSTALLED_MODULE_STEM": {"bootclasspath.pb"},
			// Output and Install paths are tested separately in TestPlatformBootclasspath_ClasspathFragmentPaths
		}

		p := result.Module("platform-bootclasspath", "android_common").(*platformBootclasspathModule)

		entries := android.AndroidMkEntriesForTest(t, result.TestContext, p)
		got := entries[1]
		for k, expectedValue := range want {
			if value, ok := got.EntryMap[k]; ok {
				android.AssertDeepEquals(t, k, expectedValue, value)
			} else {
				t.Errorf("No %s defined, saw %q", k, got.EntryMap)
			}
		}
	})
}

func TestPlatformBootclasspath_Dist(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		FixtureConfigureBootJars("platform:foo", "platform:bar"),
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

func TestPlatformBootclasspath_HiddenAPIMonolithicFiles(t *testing.T) {
	result := android.GroupFixturePreparers(
		hiddenApiFixtureFactory,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("bar"),
		FixtureConfigureBootJars("platform:foo", "platform:bar"),
	).RunTestWithBp(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			compile_dex: true,

			hiddenapi_additional_annotations: [
				"foo-hiddenapi-annotations",
			],
		}

		java_library {
			name: "foo-hiddenapi-annotations",
			srcs: ["a.java"],
			compile_dex: true,
		}

		java_import {
			name: "foo",
			jars: ["a.jar"],
			compile_dex: true,
			prefer: false,
		}

		java_sdk_library {
			name: "bar",
			srcs: ["a.java"],
			compile_dex: true,
		}

		platform_bootclasspath {
			name: "myplatform-bootclasspath",
		}
	`)

	// Make sure that the foo-hiddenapi-annotations.jar is included in the inputs to the rules that
	// creates the index.csv file.
	platformBootclasspath := result.ModuleForTests("myplatform-bootclasspath", "android_common")
	indexRule := platformBootclasspath.Rule("monolithic_hidden_API_index")
	CheckHiddenAPIRuleInputs(t, `
.intermediates/bar/android_common/javac/bar.jar
.intermediates/foo-hiddenapi-annotations/android_common/javac/foo-hiddenapi-annotations.jar
.intermediates/foo/android_common/javac/foo.jar
`,
		indexRule)
}
