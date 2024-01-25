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
	"strings"
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

			java_library {
				name: "foo",
				srcs: ["foo.java"],
				installable: true,
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

			java_import {
				name: "foo",
				jars: ["foo.jar"],
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

			java_library {
				name: "foo",
				srcs: ["foo.java"],
				installable: true,
			}

			java_library {
				name: "bar",
				srcs: ["bar.java"],
				installable: true,
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

			java_library {
				name: "foo",
				srcs: ["foo.java"],
				installable: true,
			}

			java_library {
				name: "bar",
				srcs: ["bar.java"],
				installable: true,
			}
		`)
}

func TestBootclasspathFragment_Coverage(t *testing.T) {
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
			hidden_api: {
				split_packages: ["*"],
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
			prepareForTestWithFrameworkJacocoInstrumentation,
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
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.BuildFlags = map[string]string{
				"RELEASE_HIDDEN_API_EXPORTABLE_STUBS": "true",
			}
		}),
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
			hidden_api: {
				split_packages: ["*"],
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
	info, _ := android.SingletonModuleProvider(result, fragment, HiddenAPIInfoProvider)

	stubsJar := "out/soong/.intermediates/mystublib/android_common/dex/mystublib.jar"

	// Stubs jars for mysdklibrary
	publicStubsJar := "out/soong/.intermediates/mysdklibrary.stubs.exportable/android_common/dex/mysdklibrary.stubs.exportable.jar"
	systemStubsJar := "out/soong/.intermediates/mysdklibrary.stubs.exportable.system/android_common/dex/mysdklibrary.stubs.exportable.system.jar"

	// Stubs jars for myothersdklibrary
	otherPublicStubsJar := "out/soong/.intermediates/myothersdklibrary.stubs.exportable/android_common/dex/myothersdklibrary.stubs.exportable.jar"

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

func TestFromTextWidestApiScope(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithBootclasspathFragment,
		PrepareForTestWithJavaSdkLibraryFiles,
		android.FixtureModifyConfig(func(config android.Config) {
			config.SetBuildFromTextStub(true)
		}),
		FixtureWithLastReleaseApis("mysdklibrary", "android-non-updatable"),
		FixtureConfigureApexBootJars("someapex:mysdklibrary"),
	).RunTestWithBp(t, `
		bootclasspath_fragment {
			name: "myfragment",
			contents: ["mysdklibrary"],
			additional_stubs: [
				"android-non-updatable",
			],
			hidden_api: {
				split_packages: ["*"],
			},
		}
		java_sdk_library {
			name: "mysdklibrary",
			srcs: ["a.java"],
			shared_library: false,
			public: {enabled: true},
			system: {enabled: true},
		}
		java_sdk_library {
			name: "android-non-updatable",
			srcs: ["b.java"],
			compile_dex: true,
			public: {
				enabled: true,
			},
			system: {
				enabled: true,
			},
			test: {
				enabled: true,
			},
			module_lib: {
				enabled: true,
			},
		}
	`)

	fragment := result.ModuleForTests("myfragment", "android_common")
	dependencyStubDexFlag := "--dependency-stub-dex=out/soong/.intermediates/default/java/android-non-updatable.stubs.test_module_lib/android_common/dex/android-non-updatable.stubs.test_module_lib.jar"
	stubFlagsCommand := fragment.Output("modular-hiddenapi/stub-flags.csv").RuleParams.Command
	android.AssertStringDoesContain(t,
		"Stub flags generating command does not include the expected dependency stub dex file",
		stubFlagsCommand, dependencyStubDexFlag)
}

func TestSnapshotWithBootclasspathFragment_HiddenAPI(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithBootclasspathFragment,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("mysdklibrary", "mynewlibrary"),
		FixtureConfigureApexBootJars("myapex:mybootlib", "myapex:mynewlibrary"),
		android.MockFS{
			"my-blocked.txt":                   nil,
			"my-max-target-o-low-priority.txt": nil,
			"my-max-target-p.txt":              nil,
			"my-max-target-q.txt":              nil,
			"my-max-target-r-low-priority.txt": nil,
			"my-removed.txt":                   nil,
			"my-unsupported-packages.txt":      nil,
			"my-unsupported.txt":               nil,
			"my-new-max-target-q.txt":          nil,
		}.AddToFixture(),
		android.FixtureWithRootAndroidBp(`
			bootclasspath_fragment {
				name: "mybootclasspathfragment",
				apex_available: ["myapex"],
				contents: ["mybootlib", "mynewlibrary"],
				hidden_api: {
					unsupported: [
							"my-unsupported.txt",
					],
					removed: [
							"my-removed.txt",
					],
					max_target_r_low_priority: [
							"my-max-target-r-low-priority.txt",
					],
					max_target_q: [
							"my-max-target-q.txt",
					],
					max_target_p: [
							"my-max-target-p.txt",
					],
					max_target_o_low_priority: [
							"my-max-target-o-low-priority.txt",
					],
					blocked: [
							"my-blocked.txt",
					],
					unsupported_packages: [
							"my-unsupported-packages.txt",
					],
					split_packages: ["sdklibrary"],
					package_prefixes: ["sdklibrary.all.mine"],
					single_packages: ["sdklibrary.mine"],
				},
			}

			java_library {
				name: "mybootlib",
				apex_available: ["myapex"],
				srcs: ["Test.java"],
				system_modules: "none",
				sdk_version: "none",
				min_sdk_version: "1",
				compile_dex: true,
				permitted_packages: ["mybootlib"],
			}

			java_sdk_library {
				name: "mynewlibrary",
				apex_available: ["myapex"],
				srcs: ["Test.java"],
				min_sdk_version: "10",
				compile_dex: true,
				public: {enabled: true},
				permitted_packages: ["mysdklibrary"],
				hidden_api: {
					max_target_q: [
							"my-new-max-target-q.txt",
					],
					split_packages: ["sdklibrary", "newlibrary"],
					package_prefixes: ["newlibrary.all.mine"],
					single_packages: ["newlibrary.mine"],
				},
			}
		`),
	).RunTest(t)

	// Make sure that the library exports hidden API properties for use by the bootclasspath_fragment.
	library := result.Module("mynewlibrary", "android_common")
	info, _ := android.SingletonModuleProvider(result, library, hiddenAPIPropertyInfoProvider)
	android.AssertArrayString(t, "split packages", []string{"sdklibrary", "newlibrary"}, info.SplitPackages)
	android.AssertArrayString(t, "package prefixes", []string{"newlibrary.all.mine"}, info.PackagePrefixes)
	android.AssertArrayString(t, "single packages", []string{"newlibrary.mine"}, info.SinglePackages)
	for _, c := range HiddenAPIFlagFileCategories {
		expectedMaxTargetQPaths := []string(nil)
		if c.PropertyName() == "max_target_q" {
			expectedMaxTargetQPaths = []string{"my-new-max-target-q.txt"}
		}
		android.AssertPathsRelativeToTopEquals(t, c.PropertyName(), expectedMaxTargetQPaths, info.FlagFilesByCategory[c])
	}

	// Make sure that the signature-patterns.csv is passed all the appropriate package properties
	// from the bootclasspath_fragment and its contents.
	fragment := result.ModuleForTests("mybootclasspathfragment", "android_common")
	rule := fragment.Output("modular-hiddenapi/signature-patterns.csv")
	expectedCommand := strings.Join([]string{
		"--split-package newlibrary",
		"--split-package sdklibrary",
		"--package-prefix newlibrary.all.mine",
		"--package-prefix sdklibrary.all.mine",
		"--single-package newlibrary.mine",
		"--single-package sdklibrary",
	}, " ")
	android.AssertStringDoesContain(t, "signature patterns command", rule.RuleParams.Command, expectedCommand)
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
