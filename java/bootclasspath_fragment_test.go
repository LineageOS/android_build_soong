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

// Contains some simple tests for bootclasspath_fragment logic, additional tests can be found in
// apex/bootclasspath_fragment_test.go as the ART boot image requires modules from the ART apex.

var prepareForTestWithBootclasspathFragment = android.GroupFixturePreparers(
	PrepareForTestWithJavaDefaultModules,
	dexpreopt.PrepareForTestByEnablingDexpreopt,
)

func TestBootclasspathFragment_UnknownImageName(t *testing.T) {
	prepareForTestWithBootclasspathFragment.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(
			`\Qimage_name: unknown image name "unknown", expected "art"\E`)).
		RunTestWithBp(t, `
			bootclasspath_fragment {
				name: "unknown-bootclasspath-fragment",
				image_name: "unknown",
				contents: ["foo"],
			}
		`)
}

func TestPrebuiltBootclasspathFragment_UnknownImageName(t *testing.T) {
	prepareForTestWithBootclasspathFragment.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(
			`\Qimage_name: unknown image name "unknown", expected "art"\E`)).
		RunTestWithBp(t, `
			prebuilt_bootclasspath_fragment {
				name: "unknown-bootclasspath-fragment",
				image_name: "unknown",
				contents: ["foo"],
			}
		`)
}

func TestBootclasspathFragmentInconsistentArtConfiguration_Platform(t *testing.T) {
	android.GroupFixturePreparers(
		prepareForTestWithBootclasspathFragment,
		dexpreopt.FixtureSetArtBootJars("platform:foo", "apex:bar"),
	).
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(
			`\QArtApexJars is invalid as it requests a platform variant of "foo"\E`)).
		RunTestWithBp(t, `
			bootclasspath_fragment {
				name: "bootclasspath-fragment",
				image_name: "art",
				contents: ["foo", "bar"],
				apex_available: [
					"apex",
				],
			}
		`)
}

func TestBootclasspathFragmentInconsistentArtConfiguration_ApexMixture(t *testing.T) {
	android.GroupFixturePreparers(
		prepareForTestWithBootclasspathFragment,
		dexpreopt.FixtureSetArtBootJars("apex1:foo", "apex2:bar"),
	).
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(
			`\QArtApexJars configuration is inconsistent, expected all jars to be in the same apex but it specifies apex "apex1" and "apex2"\E`)).
		RunTestWithBp(t, `
			bootclasspath_fragment {
				name: "bootclasspath-fragment",
				image_name: "art",
				contents: ["foo", "bar"],
				apex_available: [
					"apex1",
					"apex2",
				],
			}
		`)
}

func TestBootclasspathFragment_Coverage(t *testing.T) {
	prepareForTestWithFrameworkCoverage := android.FixtureMergeEnv(map[string]string{
		"EMMA_INSTRUMENT":           "true",
		"EMMA_INSTRUMENT_FRAMEWORK": "true",
	})

	prepareWithBp := android.FixtureWithRootAndroidBp(`
		bootclasspath_fragment {
			name: "myfragment",
			contents: [
				"mybootlib",
			],
			api: {
				stub_libs: [
					"mysdklibrary",
				],
			},
			coverage: {
				contents: [
					"coveragelib",
				],
				api: {
					stub_libs: [
						"mycoveragestubs",
					],
				},
			},
		}

		java_library {
			name: "mybootlib",
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
		}

		java_library {
			name: "coveragelib",
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
		}

		java_sdk_library {
			name: "mysdklibrary",
			srcs: ["Test.java"],
			compile_dex: true,
			public: {enabled: true},
			system: {enabled: true},
		}

		java_sdk_library {
			name: "mycoveragestubs",
			srcs: ["Test.java"],
			compile_dex: true,
			public: {enabled: true},
		}
	`)

	checkContents := func(t *testing.T, result *android.TestResult, expected ...string) {
		module := result.Module("myfragment", "android_common").(*BootclasspathFragmentModule)
		android.AssertArrayString(t, "contents property", expected, module.properties.Contents)
	}

	preparer := android.GroupFixturePreparers(
		prepareForTestWithBootclasspathFragment,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("mysdklibrary", "mycoveragestubs"),
		FixtureConfigureApexBootJars("someapex:mybootlib"),
		prepareWithBp,
	)

	t.Run("without coverage", func(t *testing.T) {
		result := preparer.RunTest(t)
		checkContents(t, result, "mybootlib")
	})

	t.Run("with coverage", func(t *testing.T) {
		result := android.GroupFixturePreparers(
			prepareForTestWithFrameworkCoverage,
			preparer,
		).RunTest(t)
		checkContents(t, result, "mybootlib", "coveragelib")
	})
}

func TestBootclasspathFragment_StubLibs(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithBootclasspathFragment,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("mysdklibrary", "myothersdklibrary", "mycoreplatform"),
		FixtureConfigureApexBootJars("someapex:mysdklibrary"),
	).RunTestWithBp(t, `
		bootclasspath_fragment {
			name: "myfragment",
			contents: ["mysdklibrary"],
			api: {
				stub_libs: [
					"mystublib",
					"myothersdklibrary",
				],
			},
			core_platform_api: {
				stub_libs: ["mycoreplatform.stubs"],
			},
		}

		java_library {
			name: "mystublib",
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
		}

		java_sdk_library {
			name: "mysdklibrary",
			srcs: ["a.java"],
			shared_library: false,
			public: {enabled: true},
			system: {enabled: true},
		}

		java_sdk_library {
			name: "myothersdklibrary",
			srcs: ["a.java"],
			shared_library: false,
			public: {enabled: true},
		}

		java_sdk_library {
			name: "mycoreplatform",
			srcs: ["a.java"],
			shared_library: false,
			public: {enabled: true},
		}
	`)

	fragment := result.Module("myfragment", "android_common")
	info := result.ModuleProvider(fragment, HiddenAPIInfoProvider).(HiddenAPIInfo)

	stubsJar := "out/soong/.intermediates/mystublib/android_common/dex/mystublib.jar"

	// Stubs jars for mysdklibrary
	publicStubsJar := "out/soong/.intermediates/mysdklibrary.stubs/android_common/dex/mysdklibrary.stubs.jar"
	systemStubsJar := "out/soong/.intermediates/mysdklibrary.stubs.system/android_common/dex/mysdklibrary.stubs.system.jar"

	// Stubs jars for myothersdklibrary
	otherPublicStubsJar := "out/soong/.intermediates/myothersdklibrary.stubs/android_common/dex/myothersdklibrary.stubs.jar"

	// Check that SdkPublic uses public stubs for all sdk libraries.
	android.AssertPathsRelativeToTopEquals(t, "public dex stubs jar", []string{otherPublicStubsJar, publicStubsJar, stubsJar}, info.TransitiveStubDexJarsByScope.StubDexJarsForScope(PublicHiddenAPIScope))

	// Check that SdkSystem uses system stubs for mysdklibrary and public stubs for myothersdklibrary
	// as it does not provide system stubs.
	android.AssertPathsRelativeToTopEquals(t, "system dex stubs jar", []string{otherPublicStubsJar, systemStubsJar, stubsJar}, info.TransitiveStubDexJarsByScope.StubDexJarsForScope(SystemHiddenAPIScope))

	// Check that SdkTest also uses system stubs for mysdklibrary as it does not provide test stubs
	// and public stubs for myothersdklibrary as it does not provide test stubs either.
	android.AssertPathsRelativeToTopEquals(t, "test dex stubs jar", []string{otherPublicStubsJar, systemStubsJar, stubsJar}, info.TransitiveStubDexJarsByScope.StubDexJarsForScope(TestHiddenAPIScope))

	// Check that SdkCorePlatform uses public stubs from the mycoreplatform library.
	corePlatformStubsJar := "out/soong/.intermediates/mycoreplatform.stubs/android_common/dex/mycoreplatform.stubs.jar"
	android.AssertPathsRelativeToTopEquals(t, "core platform dex stubs jar", []string{corePlatformStubsJar}, info.TransitiveStubDexJarsByScope.StubDexJarsForScope(CorePlatformHiddenAPIScope))

	// Check the widest stubs.. The list contains the widest stub dex jar provided by each module.
	expectedWidestPaths := []string{
		// mycoreplatform's widest API is core platform.
		corePlatformStubsJar,

		// myothersdklibrary's widest API is public.
		otherPublicStubsJar,

		// sdklibrary's widest API is system.
		systemStubsJar,

		// mystublib's only provides one API and so it must be the widest.
		stubsJar,
	}

	android.AssertPathsRelativeToTopEquals(t, "widest dex stubs jar", expectedWidestPaths, info.TransitiveStubDexJarsByScope.StubDexJarsForWidestAPIScope())
}

func TestBootclasspathFragment_Test(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithBootclasspathFragment,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("mysdklibrary"),
	).RunTestWithBp(t, `
		bootclasspath_fragment {
			name: "myfragment",
			contents: ["mysdklibrary"],
			hidden_api: {
				split_packages: [],
			},
		}

		bootclasspath_fragment_test {
			name: "a_test_fragment",
			contents: ["mysdklibrary"],
			hidden_api: {
				split_packages: [],
			},
		}


		java_sdk_library {
			name: "mysdklibrary",
			srcs: ["a.java"],
			shared_library: false,
			public: {enabled: true},
			system: {enabled: true},
		}
	`)

	fragment := result.Module("myfragment", "android_common").(*BootclasspathFragmentModule)
	android.AssertBoolEquals(t, "not a test fragment", false, fragment.isTestFragment())

	fragment = result.Module("a_test_fragment", "android_common").(*BootclasspathFragmentModule)
	android.AssertBoolEquals(t, "is a test fragment by type", true, fragment.isTestFragment())
}
