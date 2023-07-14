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

package sdk

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/java"
)

// fixtureAddPlatformBootclasspathForBootclasspathFragment adds a platform_bootclasspath module that
// references the bootclasspath fragment.
func fixtureAddPlatformBootclasspathForBootclasspathFragment(apex, fragment string) android.FixturePreparer {
	return fixtureAddPlatformBootclasspathForBootclasspathFragmentWithExtra(apex, fragment, "")
}

// fixtureAddPlatformBootclasspathForBootclasspathFragmentWithExtra is the same as above, but also adds extra fragments.
func fixtureAddPlatformBootclasspathForBootclasspathFragmentWithExtra(apex, fragment, extraFragments string) android.FixturePreparer {
	return android.GroupFixturePreparers(
		// Add a platform_bootclasspath module.
		android.FixtureAddTextFile("frameworks/base/boot/Android.bp", fmt.Sprintf(`
			platform_bootclasspath {
				name: "platform-bootclasspath",
				fragments: [
					{
						apex: "%s",
						module: "%s",
					},
					%s
				],
			}
		`, apex, fragment, extraFragments)),
		android.FixtureAddFile("frameworks/base/config/boot-profile.txt", nil),
		android.FixtureAddFile("frameworks/base/config/boot-image-profile.txt", nil),
		android.FixtureAddFile("build/soong/scripts/check_boot_jars/package_allowed_list.txt", nil),
	)
}

// fixtureAddPrebuiltApexForBootclasspathFragment adds a prebuilt_apex that exports the fragment.
func fixtureAddPrebuiltApexForBootclasspathFragment(apex, fragment string) android.FixturePreparer {
	apexFile := fmt.Sprintf("%s.apex", apex)
	dir := "prebuilts/apex"
	return android.GroupFixturePreparers(
		// A preparer to add a prebuilt apex to the test fixture.
		android.FixtureAddTextFile(filepath.Join(dir, "Android.bp"), fmt.Sprintf(`
			prebuilt_apex {
				name: "%s",
				src: "%s",
				exported_bootclasspath_fragments: [
					"%s",
				],
			}
		`, apex, apexFile, fragment)),
		android.FixtureAddFile(filepath.Join(dir, apexFile), nil),
	)
}

func TestSnapshotWithBootclasspathFragment_ImageName(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		java.PrepareForTestWithDexpreopt,
		prepareForSdkTestWithApex,

		// Some additional files needed for the art apex.
		android.FixtureMergeMockFs(android.MockFS{
			"com.android.art.avbpubkey":                          nil,
			"com.android.art.pem":                                nil,
			"system/sepolicy/apex/com.android.art-file_contexts": nil,
		}),

		// Add a platform_bootclasspath that depends on the fragment.
		fixtureAddPlatformBootclasspathForBootclasspathFragmentWithExtra(
			"com.android.art", "art-bootclasspath-fragment", java.ApexBootJarFragmentsForPlatformBootclasspath),

		java.PrepareForBootImageConfigTest,
		java.PrepareApexBootJarConfigsAndModules,
		android.FixtureWithRootAndroidBp(`
			sdk {
				name: "mysdk",
				bootclasspath_fragments: ["art-bootclasspath-fragment"],
			}

			apex {
				name: "com.android.art",
				key: "com.android.art.key",
				bootclasspath_fragments: [
					"art-bootclasspath-fragment",
				],
				updatable: false,
			}

			bootclasspath_fragment {
				name: "art-bootclasspath-fragment",
				image_name: "art",
				contents: ["core1", "core2"],
				apex_available: ["com.android.art"],
				hidden_api: {
					split_packages: ["*"],
				},
			}

			apex_key {
				name: "com.android.art.key",
				public_key: "com.android.art.avbpubkey",
				private_key: "com.android.art.pem",
			}

			java_library {
				name: "core1",
				srcs: ["Test.java"],
				system_modules: "none",
				sdk_version: "none",
				compile_dex: true,
				apex_available: ["com.android.art"],
			}

			java_library {
				name: "core2",
				srcs: ["Test.java"],
				system_modules: "none",
				sdk_version: "none",
				compile_dex: true,
				apex_available: ["com.android.art"],
			}
`),
	).RunTest(t)

	// A preparer to update the test fixture used when processing an unpackage snapshot.
	preparerForSnapshot := fixtureAddPrebuiltApexForBootclasspathFragment("com.android.art", "art-bootclasspath-fragment")

	// Check that source on its own configures the bootImageConfig correctly.
	java.CheckMutatedArtBootImageConfig(t, result, "out/soong/.intermediates/default/java/dex_bootjars/android_common/meta_lic")
	java.CheckMutatedFrameworkBootImageConfig(t, result, "out/soong/.intermediates/default/java/dex_bootjars/android_common/meta_lic")

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_bootclasspath_fragment {
    name: "art-bootclasspath-fragment",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["com.android.art"],
    image_name: "art",
    contents: [
        "core1",
        "core2",
    ],
    hidden_api: {
        annotation_flags: "hiddenapi/annotation-flags.csv",
        metadata: "hiddenapi/metadata.csv",
        index: "hiddenapi/index.csv",
        signature_patterns: "hiddenapi/signature-patterns.csv",
        filtered_stub_flags: "hiddenapi/filtered-stub-flags.csv",
        filtered_flags: "hiddenapi/filtered-flags.csv",
    },
}

java_import {
    name: "core1",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["com.android.art"],
    jars: ["java_boot_libs/snapshot/jars/are/invalid/core1.jar"],
}

java_import {
    name: "core2",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["com.android.art"],
    jars: ["java_boot_libs/snapshot/jars/are/invalid/core2.jar"],
}
`),
		checkAllCopyRules(`
.intermediates/art-bootclasspath-fragment/android_common/modular-hiddenapi/annotation-flags.csv -> hiddenapi/annotation-flags.csv
.intermediates/art-bootclasspath-fragment/android_common/modular-hiddenapi/metadata.csv -> hiddenapi/metadata.csv
.intermediates/art-bootclasspath-fragment/android_common/modular-hiddenapi/index.csv -> hiddenapi/index.csv
.intermediates/art-bootclasspath-fragment/android_common/modular-hiddenapi/signature-patterns.csv -> hiddenapi/signature-patterns.csv
.intermediates/art-bootclasspath-fragment/android_common/modular-hiddenapi/filtered-stub-flags.csv -> hiddenapi/filtered-stub-flags.csv
.intermediates/art-bootclasspath-fragment/android_common/modular-hiddenapi/filtered-flags.csv -> hiddenapi/filtered-flags.csv
.intermediates/mysdk/common_os/empty -> java_boot_libs/snapshot/jars/are/invalid/core1.jar
.intermediates/mysdk/common_os/empty -> java_boot_libs/snapshot/jars/are/invalid/core2.jar
		`),
		snapshotTestPreparer(checkSnapshotWithoutSource, preparerForSnapshot),

		// Check the behavior of the snapshot without the source.
		snapshotTestChecker(checkSnapshotWithoutSource, func(t *testing.T, result *android.TestResult) {
			// Make sure that the boot jars package check rule includes the dex jars retrieved from the prebuilt apex.
			checkBootJarsPackageCheckRule(t, result,
				append(
					[]string{
						"out/soong/.intermediates/prebuilts/apex/com.android.art.deapexer/android_common/deapexer/javalib/core1.jar",
						"out/soong/.intermediates/prebuilts/apex/com.android.art.deapexer/android_common/deapexer/javalib/core2.jar",
						"out/soong/.intermediates/default/java/framework/android_common/aligned/framework.jar",
					},
					java.ApexBootJarDexJarPaths...,
				)...,
			)
			java.CheckMutatedArtBootImageConfig(t, result, "out/soong/.intermediates/default/java/dex_bootjars/android_common/meta_lic")
			java.CheckMutatedFrameworkBootImageConfig(t, result, "out/soong/.intermediates/default/java/dex_bootjars/android_common/meta_lic")
		}),

		snapshotTestPreparer(checkSnapshotWithSourcePreferred, preparerForSnapshot),

		// Check the behavior of the snapshot when the source is preferred.
		snapshotTestChecker(checkSnapshotWithSourcePreferred, func(t *testing.T, result *android.TestResult) {
			java.CheckMutatedArtBootImageConfig(t, result, "out/soong/.intermediates/default/java/dex_bootjars/android_common/meta_lic")
			java.CheckMutatedFrameworkBootImageConfig(t, result, "out/soong/.intermediates/default/java/dex_bootjars/android_common/meta_lic")
		}),

		snapshotTestPreparer(checkSnapshotPreferredWithSource, preparerForSnapshot),

		// Check the behavior of the snapshot when it is preferred.
		snapshotTestChecker(checkSnapshotPreferredWithSource, func(t *testing.T, result *android.TestResult) {
			java.CheckMutatedArtBootImageConfig(t, result, "out/soong/.intermediates/default/java/dex_bootjars/android_common/meta_lic")
			java.CheckMutatedFrameworkBootImageConfig(t, result, "out/soong/.intermediates/default/java/dex_bootjars/android_common/meta_lic")
		}),
	)

	// Make sure that the boot jars package check rule includes the dex jars created from the source.
	checkBootJarsPackageCheckRule(t, result,
		append(
			[]string{
				"out/soong/.intermediates/core1/android_common_apex10000/aligned/core1.jar",
				"out/soong/.intermediates/core2/android_common_apex10000/aligned/core2.jar",
				"out/soong/.intermediates/default/java/framework/android_common/aligned/framework.jar",
			},
			java.ApexBootJarDexJarPaths...,
		)...,
	)
}

// checkBootJarsPackageCheckRule checks that the supplied module is an input to the boot jars
// package check rule.
func checkBootJarsPackageCheckRule(t *testing.T, result *android.TestResult, expectedModules ...string) {
	t.Helper()
	platformBcp := result.ModuleForTests("platform-bootclasspath", "android_common")
	bootJarsCheckRule := platformBcp.Rule("boot_jars_package_check")
	command := bootJarsCheckRule.RuleParams.Command
	expectedCommandArgs := " build/soong/scripts/check_boot_jars/package_allowed_list.txt " + strings.Join(expectedModules, " ") + " &&"
	android.AssertStringDoesContain(t, "boot jars package check", command, expectedCommandArgs)
}

func testSnapshotWithBootClasspathFragment_Contents(t *testing.T, sdk string, copyRules string) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		java.PrepareForTestWithJavaDefaultModules,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("mysdklibrary", "myothersdklibrary", "mycoreplatform"),
		java.FixtureConfigureApexBootJars("myapex:mybootlib", "myapex:myothersdklibrary"),
		prepareForSdkTestWithApex,

		// Add a platform_bootclasspath that depends on the fragment.
		fixtureAddPlatformBootclasspathForBootclasspathFragment("myapex", "mybootclasspathfragment"),

		android.FixtureWithRootAndroidBp(sdk+`
			apex {
				name: "myapex",
				key: "myapex.key",
				min_sdk_version: "2",
				bootclasspath_fragments: ["mybootclasspathfragment"],
			}

			bootclasspath_fragment {
				name: "mybootclasspathfragment",
				apex_available: ["myapex"],
				contents: [
					// This should be automatically added to the sdk_snapshot as a java_boot_libs module.
					"mybootlib",
					// This should be automatically added to the sdk_snapshot as a java_sdk_libs module.
					"myothersdklibrary",
				],
				api: {
					stub_libs: ["mysdklibrary"],
				},
				core_platform_api: {
					// This should be automatically added to the sdk_snapshot as a java_sdk_libs module.
					stub_libs: ["mycoreplatform"],
				},
				hidden_api: {
					split_packages: ["*"],
				},
			}

			java_library {
				name: "mybootlib",
				apex_available: ["myapex"],
				srcs: ["Test.java"],
				system_modules: "none",
				sdk_version: "none",
				min_sdk_version: "2",
				compile_dex: true,
				permitted_packages: ["mybootlib"],
			}

			java_sdk_library {
				name: "mysdklibrary",
				apex_available: ["myapex"],
				srcs: ["Test.java"],
				shared_library: false,
				public: {enabled: true},
				min_sdk_version: "2",
			}

			java_sdk_library {
				name: "myothersdklibrary",
				apex_available: ["myapex"],
				srcs: ["Test.java"],
				compile_dex: true,
				public: {enabled: true},
				min_sdk_version: "2",
				permitted_packages: ["myothersdklibrary"],
			}

			java_sdk_library {
				name: "mycoreplatform",
				apex_available: ["myapex"],
				srcs: ["Test.java"],
				compile_dex: true,
				public: {enabled: true},
				min_sdk_version: "2",
			}
		`),
	).RunTest(t)

	// A preparer to update the test fixture used when processing an unpackage snapshot.
	preparerForSnapshot := fixtureAddPrebuiltApexForBootclasspathFragment("myapex", "mybootclasspathfragment")

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_bootclasspath_fragment {
    name: "mybootclasspathfragment",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    contents: [
        "mybootlib",
        "myothersdklibrary",
    ],
    api: {
        stub_libs: ["mysdklibrary"],
    },
    core_platform_api: {
        stub_libs: ["mycoreplatform"],
    },
    hidden_api: {
        annotation_flags: "hiddenapi/annotation-flags.csv",
        metadata: "hiddenapi/metadata.csv",
        index: "hiddenapi/index.csv",
        signature_patterns: "hiddenapi/signature-patterns.csv",
        filtered_stub_flags: "hiddenapi/filtered-stub-flags.csv",
        filtered_flags: "hiddenapi/filtered-flags.csv",
    },
}

java_import {
    name: "mybootlib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    jars: ["java_boot_libs/snapshot/jars/are/invalid/mybootlib.jar"],
    min_sdk_version: "2",
    permitted_packages: ["mybootlib"],
}

java_sdk_library_import {
    name: "myothersdklibrary",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    shared_library: true,
    compile_dex: true,
    permitted_packages: ["myothersdklibrary"],
    public: {
        jars: ["sdk_library/public/myothersdklibrary-stubs.jar"],
        stub_srcs: ["sdk_library/public/myothersdklibrary_stub_sources"],
        current_api: "sdk_library/public/myothersdklibrary.txt",
        removed_api: "sdk_library/public/myothersdklibrary-removed.txt",
        sdk_version: "current",
    },
}

java_sdk_library_import {
    name: "mysdklibrary",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    shared_library: false,
    public: {
        jars: ["sdk_library/public/mysdklibrary-stubs.jar"],
        stub_srcs: ["sdk_library/public/mysdklibrary_stub_sources"],
        current_api: "sdk_library/public/mysdklibrary.txt",
        removed_api: "sdk_library/public/mysdklibrary-removed.txt",
        sdk_version: "current",
    },
}

java_sdk_library_import {
    name: "mycoreplatform",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    shared_library: true,
    compile_dex: true,
    public: {
        jars: ["sdk_library/public/mycoreplatform-stubs.jar"],
        stub_srcs: ["sdk_library/public/mycoreplatform_stub_sources"],
        current_api: "sdk_library/public/mycoreplatform.txt",
        removed_api: "sdk_library/public/mycoreplatform-removed.txt",
        sdk_version: "current",
    },
}
		`),
		checkAllCopyRules(copyRules),
		snapshotTestPreparer(checkSnapshotWithoutSource, preparerForSnapshot),
		snapshotTestChecker(checkSnapshotWithoutSource, func(t *testing.T, result *android.TestResult) {
			module := result.ModuleForTests("platform-bootclasspath", "android_common")
			var rule android.TestingBuildParams
			rule = module.Output("out/soong/hiddenapi/hiddenapi-flags.csv")
			java.CheckHiddenAPIRuleInputs(t, "monolithic flags", `
				out/soong/.intermediates/frameworks/base/boot/platform-bootclasspath/android_common/hiddenapi-monolithic/annotation-flags-from-classes.csv
        out/soong/hiddenapi/hiddenapi-stub-flags.txt
        snapshot/hiddenapi/annotation-flags.csv
			`, rule)

			rule = module.Output("out/soong/hiddenapi/hiddenapi-unsupported.csv")
			java.CheckHiddenAPIRuleInputs(t, "monolithic metadata", `
				out/soong/.intermediates/frameworks/base/boot/platform-bootclasspath/android_common/hiddenapi-monolithic/metadata-from-classes.csv
        snapshot/hiddenapi/metadata.csv
			`, rule)

			rule = module.Output("out/soong/hiddenapi/hiddenapi-index.csv")
			java.CheckHiddenAPIRuleInputs(t, "monolithic index", `
				out/soong/.intermediates/frameworks/base/boot/platform-bootclasspath/android_common/hiddenapi-monolithic/index-from-classes.csv
        snapshot/hiddenapi/index.csv
			`, rule)

			rule = module.Output("out/soong/hiddenapi/hiddenapi-flags.csv.valid")
			android.AssertStringDoesContain(t, "verify-overlaps", rule.RuleParams.Command, " snapshot/hiddenapi/filtered-flags.csv:snapshot/hiddenapi/signature-patterns.csv ")
		}),
		snapshotTestPreparer(checkSnapshotWithSourcePreferred, preparerForSnapshot),
		snapshotTestChecker(checkSnapshotWithSourcePreferred, func(t *testing.T, result *android.TestResult) {
			module := result.ModuleForTests("platform-bootclasspath", "android_common")
			rule := module.Output("out/soong/hiddenapi/hiddenapi-flags.csv.valid")
			android.AssertStringDoesContain(t, "verify-overlaps", rule.RuleParams.Command, " out/soong/.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi/filtered-flags.csv:out/soong/.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi/signature-patterns.csv ")
		}),
		snapshotTestPreparer(checkSnapshotPreferredWithSource, preparerForSnapshot),
	)
}

func TestSnapshotWithBootClasspathFragment_Contents(t *testing.T) {
	t.Run("added-directly", func(t *testing.T) {
		testSnapshotWithBootClasspathFragment_Contents(t, `
			sdk {
				name: "mysdk",
				bootclasspath_fragments: ["mybootclasspathfragment"],
				java_sdk_libs: [
					// This is not strictly needed as it should be automatically added to the sdk_snapshot as
					// a java_sdk_libs module because it is used in the mybootclasspathfragment's
					// api.stub_libs property. However, it is specified here to ensure that duplicates are
					// correctly deduped.
					"mysdklibrary",
				],
			}
		`, `
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/annotation-flags.csv -> hiddenapi/annotation-flags.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/metadata.csv -> hiddenapi/metadata.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/index.csv -> hiddenapi/index.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/signature-patterns.csv -> hiddenapi/signature-patterns.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/filtered-stub-flags.csv -> hiddenapi/filtered-stub-flags.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/filtered-flags.csv -> hiddenapi/filtered-flags.csv
.intermediates/mysdk/common_os/empty -> java_boot_libs/snapshot/jars/are/invalid/mybootlib.jar
.intermediates/myothersdklibrary.stubs/android_common/combined/myothersdklibrary.stubs.jar -> sdk_library/public/myothersdklibrary-stubs.jar
.intermediates/myothersdklibrary.stubs.source/android_common/metalava/myothersdklibrary.stubs.source_api.txt -> sdk_library/public/myothersdklibrary.txt
.intermediates/myothersdklibrary.stubs.source/android_common/metalava/myothersdklibrary.stubs.source_removed.txt -> sdk_library/public/myothersdklibrary-removed.txt
.intermediates/mysdklibrary.stubs/android_common/combined/mysdklibrary.stubs.jar -> sdk_library/public/mysdklibrary-stubs.jar
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_api.txt -> sdk_library/public/mysdklibrary.txt
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_removed.txt -> sdk_library/public/mysdklibrary-removed.txt
.intermediates/mycoreplatform.stubs/android_common/combined/mycoreplatform.stubs.jar -> sdk_library/public/mycoreplatform-stubs.jar
.intermediates/mycoreplatform.stubs.source/android_common/metalava/mycoreplatform.stubs.source_api.txt -> sdk_library/public/mycoreplatform.txt
.intermediates/mycoreplatform.stubs.source/android_common/metalava/mycoreplatform.stubs.source_removed.txt -> sdk_library/public/mycoreplatform-removed.txt
`)
	})

	copyBootclasspathFragmentFromApexVariantRules := `
.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi/annotation-flags.csv -> hiddenapi/annotation-flags.csv
.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi/metadata.csv -> hiddenapi/metadata.csv
.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi/index.csv -> hiddenapi/index.csv
.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi/signature-patterns.csv -> hiddenapi/signature-patterns.csv
.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi/filtered-stub-flags.csv -> hiddenapi/filtered-stub-flags.csv
.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi/filtered-flags.csv -> hiddenapi/filtered-flags.csv
.intermediates/mysdk/common_os/empty -> java_boot_libs/snapshot/jars/are/invalid/mybootlib.jar
.intermediates/myothersdklibrary.stubs/android_common/combined/myothersdklibrary.stubs.jar -> sdk_library/public/myothersdklibrary-stubs.jar
.intermediates/myothersdklibrary.stubs.source/android_common/metalava/myothersdklibrary.stubs.source_api.txt -> sdk_library/public/myothersdklibrary.txt
.intermediates/myothersdklibrary.stubs.source/android_common/metalava/myothersdklibrary.stubs.source_removed.txt -> sdk_library/public/myothersdklibrary-removed.txt
.intermediates/mysdklibrary.stubs/android_common/combined/mysdklibrary.stubs.jar -> sdk_library/public/mysdklibrary-stubs.jar
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_api.txt -> sdk_library/public/mysdklibrary.txt
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_removed.txt -> sdk_library/public/mysdklibrary-removed.txt
.intermediates/mycoreplatform.stubs/android_common/combined/mycoreplatform.stubs.jar -> sdk_library/public/mycoreplatform-stubs.jar
.intermediates/mycoreplatform.stubs.source/android_common/metalava/mycoreplatform.stubs.source_api.txt -> sdk_library/public/mycoreplatform.txt
.intermediates/mycoreplatform.stubs.source/android_common/metalava/mycoreplatform.stubs.source_removed.txt -> sdk_library/public/mycoreplatform-removed.txt
`
	t.Run("added-via-apex", func(t *testing.T) {
		testSnapshotWithBootClasspathFragment_Contents(t, `
			sdk {
				name: "mysdk",
				apexes: ["myapex"],
			}
		`, copyBootclasspathFragmentFromApexVariantRules)
	})

	t.Run("added-directly-and-indirectly", func(t *testing.T) {
		testSnapshotWithBootClasspathFragment_Contents(t, `
			sdk {
				name: "mysdk",
				apexes: ["myapex"],
				// This is not strictly needed as it should be automatically added to the sdk_snapshot as
				// a bootclasspath_fragments module because it is used in the myapex's
				// bootclasspath_fragments property. However, it is specified here to ensure that duplicates
				// are correctly deduped.
				bootclasspath_fragments: ["mybootclasspathfragment"],
				java_sdk_libs: [
					// This is not strictly needed as it should be automatically added to the sdk_snapshot as
					// a java_sdk_libs module because it is used in the mybootclasspathfragment's
					// api.stub_libs property. However, it is specified here to ensure that duplicates are
					// correctly deduped.
					"mysdklibrary",
				],
			}
		`, copyBootclasspathFragmentFromApexVariantRules)
	})
}

// TestSnapshotWithBootClasspathFragment_Fragments makes sure that the fragments property of a
// bootclasspath_fragment is correctly output to the sdk snapshot.
func TestSnapshotWithBootClasspathFragment_Fragments(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		java.PrepareForTestWithJavaDefaultModules,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("mysdklibrary", "myothersdklibrary"),
		java.FixtureConfigureApexBootJars("someapex:mysdklibrary", "myotherapex:myotherlib"),
		prepareForSdkTestWithApex,

		// Some additional files needed for the myotherapex.
		android.FixtureMergeMockFs(android.MockFS{
			"system/sepolicy/apex/myotherapex-file_contexts": nil,
			"myotherapex/apex_manifest.json":                 nil,
			"myotherapex/Test.java":                          nil,
		}),

		android.FixtureAddTextFile("myotherapex/Android.bp", `
			apex {
				name: "myotherapex",
				key: "myapex.key",
				min_sdk_version: "2",
				bootclasspath_fragments: ["myotherbootclasspathfragment"],
			}

			bootclasspath_fragment {
				name: "myotherbootclasspathfragment",
				apex_available: ["myotherapex"],
				contents: [
					"myotherlib",
				],
				hidden_api: {
					split_packages: ["*"],
				},
			}

			java_library {
				name: "myotherlib",
				apex_available: ["myotherapex"],
				srcs: ["Test.java"],
				min_sdk_version: "2",
				permitted_packages: ["myothersdklibrary"],
				compile_dex: true,
			}
		`),

		android.FixtureWithRootAndroidBp(`
			sdk {
				name: "mysdk",
				bootclasspath_fragments: ["mybootclasspathfragment"],
			}

			bootclasspath_fragment {
				name: "mybootclasspathfragment",
				contents: [
					"mysdklibrary",
				],
				fragments: [
					{
						apex: "myotherapex",
						module: "myotherbootclasspathfragment"
					},
				],
				hidden_api: {
					split_packages: ["*"],
				},
			}

			java_sdk_library {
				name: "mysdklibrary",
				srcs: ["Test.java"],
				shared_library: false,
				public: {enabled: true},
				min_sdk_version: "2",
			}
		`),
	).RunTest(t)

	// A preparer to update the test fixture used when processing an unpackage snapshot.
	preparerForSnapshot := fixtureAddPrebuiltApexForBootclasspathFragment("myapex", "mybootclasspathfragment")

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_bootclasspath_fragment {
    name: "mybootclasspathfragment",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    contents: ["mysdklibrary"],
    fragments: [
        {
            apex: "myotherapex",
            module: "myotherbootclasspathfragment",
        },
    ],
    hidden_api: {
        annotation_flags: "hiddenapi/annotation-flags.csv",
        metadata: "hiddenapi/metadata.csv",
        index: "hiddenapi/index.csv",
        signature_patterns: "hiddenapi/signature-patterns.csv",
        filtered_stub_flags: "hiddenapi/filtered-stub-flags.csv",
        filtered_flags: "hiddenapi/filtered-flags.csv",
    },
}

java_sdk_library_import {
    name: "mysdklibrary",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    shared_library: false,
    public: {
        jars: ["sdk_library/public/mysdklibrary-stubs.jar"],
        stub_srcs: ["sdk_library/public/mysdklibrary_stub_sources"],
        current_api: "sdk_library/public/mysdklibrary.txt",
        removed_api: "sdk_library/public/mysdklibrary-removed.txt",
        sdk_version: "current",
    },
}
		`),
		snapshotTestPreparer(checkSnapshotWithoutSource, preparerForSnapshot),
		snapshotTestPreparer(checkSnapshotWithSourcePreferred, preparerForSnapshot),
		snapshotTestPreparer(checkSnapshotPreferredWithSource, preparerForSnapshot),
	)
}

// Test that bootclasspath_fragment works with sdk.
func TestBasicSdkWithBootclasspathFragment(t *testing.T) {
	android.GroupFixturePreparers(
		prepareForSdkTestWithApex,
		prepareForSdkTestWithJava,
		android.FixtureMergeMockFs(android.MockFS{
			"java/mybootlib.jar":                nil,
			"hiddenapi/annotation-flags.csv":    nil,
			"hiddenapi/index.csv":               nil,
			"hiddenapi/metadata.csv":            nil,
			"hiddenapi/signature-patterns.csv":  nil,
			"hiddenapi/filtered-stub-flags.csv": nil,
			"hiddenapi/filtered-flags.csv":      nil,
		}),
		android.FixtureWithRootAndroidBp(`
		sdk {
			name: "mysdk",
			bootclasspath_fragments: ["mybootclasspathfragment"],
		}

		bootclasspath_fragment {
			name: "mybootclasspathfragment",
			image_name: "art",
			contents: ["mybootlib"],
			apex_available: ["myapex"],
			hidden_api: {
				split_packages: ["*"],
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
		}

		prebuilt_bootclasspath_fragment {
			name: "mybootclasspathfragment",
			prefer: false,
			visibility: ["//visibility:public"],
			apex_available: [
				"myapex",
			],
			image_name: "art",
			contents: ["mybootlib"],
			hidden_api: {
				annotation_flags: "hiddenapi/annotation-flags.csv",
				metadata: "hiddenapi/metadata.csv",
				index: "hiddenapi/index.csv",
				signature_patterns: "hiddenapi/signature-patterns.csv",
				filtered_stub_flags: "hiddenapi/filtered-stub-flags.csv",
				filtered_flags: "hiddenapi/filtered-flags.csv",
			},
		}

		java_import {
			name: "mybootlib",
			visibility: ["//visibility:public"],
			apex_available: ["com.android.art"],
			jars: ["java/mybootlib.jar"],
		}
	`),
	).RunTest(t)
}

func TestSnapshotWithBootclasspathFragment_HiddenAPI(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		java.PrepareForTestWithJavaDefaultModules,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("mysdklibrary", "mynewlibrary"),
		java.FixtureConfigureApexBootJars("myapex:mybootlib", "myapex:mynewlibrary"),
		prepareForSdkTestWithApex,

		// Add a platform_bootclasspath that depends on the fragment.
		fixtureAddPlatformBootclasspathForBootclasspathFragment("myapex", "mybootclasspathfragment"),

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
			sdk {
				name: "mysdk",
				bootclasspath_fragments: ["mybootclasspathfragment"],
			}

			apex {
				name: "myapex",
				key: "myapex.key",
				min_sdk_version: "1",
				bootclasspath_fragments: ["mybootclasspathfragment"],
			}

			bootclasspath_fragment {
				name: "mybootclasspathfragment",
				apex_available: ["myapex"],
				contents: ["mybootlib", "mynewlibrary"],
				api: {
					stub_libs: ["mysdklibrary"],
				},
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
				name: "mysdklibrary",
				srcs: ["Test.java"],
				compile_dex: true,
				public: {enabled: true},
				permitted_packages: ["mysdklibrary"],
				min_sdk_version: "current",
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

	// A preparer to update the test fixture used when processing an unpackage snapshot.
	preparerForSnapshot := fixtureAddPrebuiltApexForBootclasspathFragment("myapex", "mybootclasspathfragment")

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_bootclasspath_fragment {
    name: "mybootclasspathfragment",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    contents: [
        "mybootlib",
        "mynewlibrary",
    ],
    api: {
        stub_libs: ["mysdklibrary"],
    },
    hidden_api: {
        unsupported: ["hiddenapi/my-unsupported.txt"],
        removed: ["hiddenapi/my-removed.txt"],
        max_target_r_low_priority: ["hiddenapi/my-max-target-r-low-priority.txt"],
        max_target_q: [
            "hiddenapi/my-max-target-q.txt",
            "hiddenapi/my-new-max-target-q.txt",
        ],
        max_target_p: ["hiddenapi/my-max-target-p.txt"],
        max_target_o_low_priority: ["hiddenapi/my-max-target-o-low-priority.txt"],
        blocked: ["hiddenapi/my-blocked.txt"],
        unsupported_packages: ["hiddenapi/my-unsupported-packages.txt"],
        annotation_flags: "hiddenapi/annotation-flags.csv",
        metadata: "hiddenapi/metadata.csv",
        index: "hiddenapi/index.csv",
        signature_patterns: "hiddenapi/signature-patterns.csv",
        filtered_stub_flags: "hiddenapi/filtered-stub-flags.csv",
        filtered_flags: "hiddenapi/filtered-flags.csv",
    },
}

java_import {
    name: "mybootlib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    jars: ["java_boot_libs/snapshot/jars/are/invalid/mybootlib.jar"],
    min_sdk_version: "1",
    permitted_packages: ["mybootlib"],
}

java_sdk_library_import {
    name: "mynewlibrary",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    shared_library: true,
    compile_dex: true,
    permitted_packages: ["mysdklibrary"],
    public: {
        jars: ["sdk_library/public/mynewlibrary-stubs.jar"],
        stub_srcs: ["sdk_library/public/mynewlibrary_stub_sources"],
        current_api: "sdk_library/public/mynewlibrary.txt",
        removed_api: "sdk_library/public/mynewlibrary-removed.txt",
        sdk_version: "current",
    },
}

java_sdk_library_import {
    name: "mysdklibrary",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    shared_library: true,
    compile_dex: true,
    permitted_packages: ["mysdklibrary"],
    public: {
        jars: ["sdk_library/public/mysdklibrary-stubs.jar"],
        stub_srcs: ["sdk_library/public/mysdklibrary_stub_sources"],
        current_api: "sdk_library/public/mysdklibrary.txt",
        removed_api: "sdk_library/public/mysdklibrary-removed.txt",
        sdk_version: "current",
    },
}
`),
		checkAllCopyRules(`
my-unsupported.txt -> hiddenapi/my-unsupported.txt
my-removed.txt -> hiddenapi/my-removed.txt
my-max-target-r-low-priority.txt -> hiddenapi/my-max-target-r-low-priority.txt
my-max-target-q.txt -> hiddenapi/my-max-target-q.txt
my-new-max-target-q.txt -> hiddenapi/my-new-max-target-q.txt
my-max-target-p.txt -> hiddenapi/my-max-target-p.txt
my-max-target-o-low-priority.txt -> hiddenapi/my-max-target-o-low-priority.txt
my-blocked.txt -> hiddenapi/my-blocked.txt
my-unsupported-packages.txt -> hiddenapi/my-unsupported-packages.txt
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/annotation-flags.csv -> hiddenapi/annotation-flags.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/metadata.csv -> hiddenapi/metadata.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/index.csv -> hiddenapi/index.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/signature-patterns.csv -> hiddenapi/signature-patterns.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/filtered-stub-flags.csv -> hiddenapi/filtered-stub-flags.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/filtered-flags.csv -> hiddenapi/filtered-flags.csv
.intermediates/mysdk/common_os/empty -> java_boot_libs/snapshot/jars/are/invalid/mybootlib.jar
.intermediates/mynewlibrary.stubs/android_common/combined/mynewlibrary.stubs.jar -> sdk_library/public/mynewlibrary-stubs.jar
.intermediates/mynewlibrary.stubs.source/android_common/metalava/mynewlibrary.stubs.source_api.txt -> sdk_library/public/mynewlibrary.txt
.intermediates/mynewlibrary.stubs.source/android_common/metalava/mynewlibrary.stubs.source_removed.txt -> sdk_library/public/mynewlibrary-removed.txt
.intermediates/mysdklibrary.stubs/android_common/combined/mysdklibrary.stubs.jar -> sdk_library/public/mysdklibrary-stubs.jar
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_api.txt -> sdk_library/public/mysdklibrary.txt
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_removed.txt -> sdk_library/public/mysdklibrary-removed.txt
`),
		snapshotTestPreparer(checkSnapshotWithoutSource, preparerForSnapshot),
		snapshotTestPreparer(checkSnapshotWithSourcePreferred, preparerForSnapshot),
		snapshotTestPreparer(checkSnapshotPreferredWithSource, preparerForSnapshot),
	)
}

func testSnapshotWithBootClasspathFragment_MinSdkVersion(t *testing.T, targetBuildRelease string,
	expectedSdkSnapshot string,
	expectedCopyRules string,
	expectedStubFlagsInputs []string,
	suffix string) {

	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		java.PrepareForTestWithJavaDefaultModules,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("mysdklibrary", "mynewsdklibrary"),
		java.FixtureConfigureApexBootJars("myapex:mysdklibrary", "myapex:mynewsdklibrary"),
		prepareForSdkTestWithApex,

		// Add a platform_bootclasspath that depends on the fragment.
		fixtureAddPlatformBootclasspathForBootclasspathFragment("myapex", "mybootclasspathfragment"),

		android.FixtureMergeEnv(map[string]string{
			"SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE": targetBuildRelease,
		}),

		android.FixtureWithRootAndroidBp(`
			sdk {
				name: "mysdk",
				apexes: ["myapex"],
			}

			apex {
				name: "myapex",
				key: "myapex.key",
				min_sdk_version: "S",
				bootclasspath_fragments: ["mybootclasspathfragment"],
			}

			bootclasspath_fragment {
				name: "mybootclasspathfragment",
				apex_available: ["myapex"],
				contents: [
					"mysdklibrary",
					"mynewsdklibrary",
				],

				hidden_api: {
					split_packages: [],
				},
			}

			java_sdk_library {
				name: "mysdklibrary",
				apex_available: ["myapex"],
				srcs: ["Test.java"],
				shared_library: false,
				public: {enabled: true},
				min_sdk_version: "S",
			}

			java_sdk_library {
				name: "mynewsdklibrary",
				apex_available: ["myapex"],
				srcs: ["Test.java"],
				compile_dex: true,
				public: {enabled: true},
				min_sdk_version: "Tiramisu",
				permitted_packages: ["mynewsdklibrary"],
			}
		`),
	).RunTest(t)

	bcpf := result.ModuleForTests("mybootclasspathfragment", "android_common")
	rule := bcpf.Output("out/soong/.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi" + suffix + "/stub-flags.csv")
	android.AssertPathsRelativeToTopEquals(t, "stub flags inputs", expectedStubFlagsInputs, rule.Implicits)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(expectedSdkSnapshot),
		checkAllCopyRules(expectedCopyRules),
	)
}

func TestSnapshotWithBootClasspathFragment_MinSdkVersion(t *testing.T) {
	t.Run("target S build", func(t *testing.T) {
		expectedSnapshot := `
// This is auto-generated. DO NOT EDIT.

prebuilt_bootclasspath_fragment {
    name: "mybootclasspathfragment",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    contents: ["mysdklibrary"],
    hidden_api: {
        annotation_flags: "hiddenapi/annotation-flags.csv",
        metadata: "hiddenapi/metadata.csv",
        index: "hiddenapi/index.csv",
        stub_flags: "hiddenapi/stub-flags.csv",
        all_flags: "hiddenapi/all-flags.csv",
    },
}

java_sdk_library_import {
    name: "mysdklibrary",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    shared_library: false,
    public: {
        jars: ["sdk_library/public/mysdklibrary-stubs.jar"],
        stub_srcs: ["sdk_library/public/mysdklibrary_stub_sources"],
        current_api: "sdk_library/public/mysdklibrary.txt",
        removed_api: "sdk_library/public/mysdklibrary-removed.txt",
        sdk_version: "current",
    },
}
`
		expectedCopyRules := `
.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi-for-sdk-snapshot/annotation-flags.csv -> hiddenapi/annotation-flags.csv
.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi-for-sdk-snapshot/metadata.csv -> hiddenapi/metadata.csv
.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi-for-sdk-snapshot/index.csv -> hiddenapi/index.csv
.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi-for-sdk-snapshot/stub-flags.csv -> hiddenapi/stub-flags.csv
.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi-for-sdk-snapshot/all-flags.csv -> hiddenapi/all-flags.csv
.intermediates/mysdklibrary.stubs/android_common/combined/mysdklibrary.stubs.jar -> sdk_library/public/mysdklibrary-stubs.jar
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_api.txt -> sdk_library/public/mysdklibrary.txt
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_removed.txt -> sdk_library/public/mysdklibrary-removed.txt
`

		// On S the stub flags should only be generated from mysdklibrary as mynewsdklibrary is not part
		// of the snapshot.
		expectedStubFlagsInputs := []string{
			"out/soong/.intermediates/mysdklibrary.stubs/android_common/dex/mysdklibrary.stubs.jar",
			"out/soong/.intermediates/mysdklibrary/android_common/aligned/mysdklibrary.jar",
		}

		testSnapshotWithBootClasspathFragment_MinSdkVersion(t, "S",
			expectedSnapshot, expectedCopyRules, expectedStubFlagsInputs, "-for-sdk-snapshot")
	})

	t.Run("target-Tiramisu-build", func(t *testing.T) {
		expectedSnapshot := `
// This is auto-generated. DO NOT EDIT.

prebuilt_bootclasspath_fragment {
    name: "mybootclasspathfragment",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    contents: [
        "mysdklibrary",
        "mynewsdklibrary",
    ],
    hidden_api: {
        annotation_flags: "hiddenapi/annotation-flags.csv",
        metadata: "hiddenapi/metadata.csv",
        index: "hiddenapi/index.csv",
        signature_patterns: "hiddenapi/signature-patterns.csv",
        filtered_stub_flags: "hiddenapi/filtered-stub-flags.csv",
        filtered_flags: "hiddenapi/filtered-flags.csv",
    },
}

java_sdk_library_import {
    name: "mysdklibrary",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    shared_library: false,
    public: {
        jars: ["sdk_library/public/mysdklibrary-stubs.jar"],
        stub_srcs: ["sdk_library/public/mysdklibrary_stub_sources"],
        current_api: "sdk_library/public/mysdklibrary.txt",
        removed_api: "sdk_library/public/mysdklibrary-removed.txt",
        sdk_version: "current",
    },
}

java_sdk_library_import {
    name: "mynewsdklibrary",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    shared_library: true,
    compile_dex: true,
    permitted_packages: ["mynewsdklibrary"],
    public: {
        jars: ["sdk_library/public/mynewsdklibrary-stubs.jar"],
        stub_srcs: ["sdk_library/public/mynewsdklibrary_stub_sources"],
        current_api: "sdk_library/public/mynewsdklibrary.txt",
        removed_api: "sdk_library/public/mynewsdklibrary-removed.txt",
        sdk_version: "current",
    },
}
`
		expectedCopyRules := `
.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi/annotation-flags.csv -> hiddenapi/annotation-flags.csv
.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi/metadata.csv -> hiddenapi/metadata.csv
.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi/index.csv -> hiddenapi/index.csv
.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi/signature-patterns.csv -> hiddenapi/signature-patterns.csv
.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi/filtered-stub-flags.csv -> hiddenapi/filtered-stub-flags.csv
.intermediates/mybootclasspathfragment/android_common_myapex/modular-hiddenapi/filtered-flags.csv -> hiddenapi/filtered-flags.csv
.intermediates/mysdklibrary.stubs/android_common/combined/mysdklibrary.stubs.jar -> sdk_library/public/mysdklibrary-stubs.jar
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_api.txt -> sdk_library/public/mysdklibrary.txt
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_removed.txt -> sdk_library/public/mysdklibrary-removed.txt
.intermediates/mynewsdklibrary.stubs/android_common/combined/mynewsdklibrary.stubs.jar -> sdk_library/public/mynewsdklibrary-stubs.jar
.intermediates/mynewsdklibrary.stubs.source/android_common/metalava/mynewsdklibrary.stubs.source_api.txt -> sdk_library/public/mynewsdklibrary.txt
.intermediates/mynewsdklibrary.stubs.source/android_common/metalava/mynewsdklibrary.stubs.source_removed.txt -> sdk_library/public/mynewsdklibrary-removed.txt
`

		// On tiramisu the stub flags should be generated from both mynewsdklibrary and mysdklibrary as
		// they are both part of the snapshot.
		expectedStubFlagsInputs := []string{
			"out/soong/.intermediates/mynewsdklibrary.stubs/android_common/dex/mynewsdklibrary.stubs.jar",
			"out/soong/.intermediates/mynewsdklibrary/android_common/aligned/mynewsdklibrary.jar",
			"out/soong/.intermediates/mysdklibrary.stubs/android_common/dex/mysdklibrary.stubs.jar",
			"out/soong/.intermediates/mysdklibrary/android_common/aligned/mysdklibrary.jar",
		}

		testSnapshotWithBootClasspathFragment_MinSdkVersion(t, "Tiramisu",
			expectedSnapshot, expectedCopyRules, expectedStubFlagsInputs, "")
	})
}

func TestSnapshotWithEmptyBootClasspathFragment(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		java.PrepareForTestWithJavaDefaultModules,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("mysdklibrary", "mynewsdklibrary"),
		java.FixtureConfigureApexBootJars("myapex:mysdklibrary", "myapex:mynewsdklibrary"),
		prepareForSdkTestWithApex,
		// Add a platform_bootclasspath that depends on the fragment.
		fixtureAddPlatformBootclasspathForBootclasspathFragment("myapex", "mybootclasspathfragment"),
		android.FixtureMergeEnv(map[string]string{
			"SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE": "S",
		}),
		android.FixtureWithRootAndroidBp(`
			sdk {
				name: "mysdk",
				apexes: ["myapex"],
			}
			apex {
				name: "myapex",
				key: "myapex.key",
				min_sdk_version: "S",
				bootclasspath_fragments: ["mybootclasspathfragment"],
			}
			bootclasspath_fragment {
				name: "mybootclasspathfragment",
				apex_available: ["myapex"],
				contents: ["mysdklibrary", "mynewsdklibrary"],
				hidden_api: {
					split_packages: [],
				},
			}
			java_sdk_library {
				name: "mysdklibrary",
				apex_available: ["myapex"],
				srcs: ["Test.java"],
				shared_library: false,
				public: {enabled: true},
				min_sdk_version: "Tiramisu",
			}
			java_sdk_library {
				name: "mynewsdklibrary",
				apex_available: ["myapex"],
				srcs: ["Test.java"],
				compile_dex: true,
				public: {enabled: true},
				min_sdk_version: "Tiramisu",
				permitted_packages: ["mynewsdklibrary"],
			}
		`),
	).RunTest(t)

	CheckSnapshot(t, result, "mysdk", "", checkAndroidBpContents(`// This is auto-generated. DO NOT EDIT.`))
}
