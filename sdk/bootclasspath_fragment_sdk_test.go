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
	"testing"

	"android/soong/android"
	"android/soong/java"
)

// fixtureAddPlatformBootclasspathForBootclasspathFragment adds a platform_bootclasspath module that
// references the bootclasspath fragment.
func fixtureAddPlatformBootclasspathForBootclasspathFragment(apex, fragment string) android.FixturePreparer {
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
				],
			}
		`, apex, fragment)),
		android.FixtureAddFile("frameworks/base/config/boot-profile.txt", nil),
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
		java.PrepareForTestWithJavaDefaultModules,
		prepareForSdkTestWithApex,

		// Some additional files needed for the art apex.
		android.FixtureMergeMockFs(android.MockFS{
			"com.android.art.avbpubkey":                          nil,
			"com.android.art.pem":                                nil,
			"system/sepolicy/apex/com.android.art-file_contexts": nil,
		}),

		// Add a platform_bootclasspath that depends on the fragment.
		fixtureAddPlatformBootclasspathForBootclasspathFragment("com.android.art", "mybootclasspathfragment"),

		java.FixtureConfigureBootJars("com.android.art:mybootlib"),
		android.FixtureWithRootAndroidBp(`
			sdk {
				name: "mysdk",
				bootclasspath_fragments: ["mybootclasspathfragment"],
			}

			apex {
				name: "com.android.art",
				key: "com.android.art.key",
				bootclasspath_fragments: [
					"mybootclasspathfragment",
				],
				updatable: false,
			}

			bootclasspath_fragment {
				name: "mybootclasspathfragment",
				image_name: "art",
				contents: ["mybootlib"],
				apex_available: ["com.android.art"],
			}

			apex_key {
				name: "com.android.art.key",
				public_key: "com.android.art.avbpubkey",
				private_key: "com.android.art.pem",
			}

			java_library {
				name: "mybootlib",
				srcs: ["Test.java"],
				system_modules: "none",
				sdk_version: "none",
				compile_dex: true,
				apex_available: ["com.android.art"],
			}
		`),
	).RunTest(t)

	// A preparer to update the test fixture used when processing an unpackage snapshot.
	preparerForSnapshot := fixtureAddPrebuiltApexForBootclasspathFragment("com.android.art", "mybootclasspathfragment")

	CheckSnapshot(t, result, "mysdk", "",
		checkUnversionedAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_bootclasspath_fragment {
    name: "mybootclasspathfragment",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["com.android.art"],
    image_name: "art",
    contents: ["mybootlib"],
}

java_import {
    name: "mybootlib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["com.android.art"],
    jars: ["java/mybootlib.jar"],
}
`),
		checkVersionedAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_bootclasspath_fragment {
    name: "mysdk_mybootclasspathfragment@current",
    sdk_member_name: "mybootclasspathfragment",
    visibility: ["//visibility:public"],
    apex_available: ["com.android.art"],
    image_name: "art",
    contents: ["mysdk_mybootlib@current"],
}

java_import {
    name: "mysdk_mybootlib@current",
    sdk_member_name: "mybootlib",
    visibility: ["//visibility:public"],
    apex_available: ["com.android.art"],
    jars: ["java/mybootlib.jar"],
}

sdk_snapshot {
    name: "mysdk@current",
    visibility: ["//visibility:public"],
    bootclasspath_fragments: ["mysdk_mybootclasspathfragment@current"],
    java_boot_libs: ["mysdk_mybootlib@current"],
}
`),
		checkAllCopyRules(`
.intermediates/mybootlib/android_common/javac/mybootlib.jar -> java/mybootlib.jar
`),
		snapshotTestPreparer(checkSnapshotWithoutSource, preparerForSnapshot),
		snapshotTestPreparer(checkSnapshotWithSourcePreferred, preparerForSnapshot),
		snapshotTestPreparer(checkSnapshotPreferredWithSource, preparerForSnapshot),
	)
}

func TestSnapshotWithBootClasspathFragment_Contents(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		java.PrepareForTestWithJavaDefaultModules,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("mysdklibrary", "myothersdklibrary", "mycoreplatform"),
		java.FixtureConfigureUpdatableBootJars("myapex:mybootlib", "myapex:myothersdklibrary"),
		prepareForSdkTestWithApex,

		// Add a platform_bootclasspath that depends on the fragment.
		fixtureAddPlatformBootclasspathForBootclasspathFragment("myapex", "mybootclasspathfragment"),

		android.FixtureWithRootAndroidBp(`
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
				shared_library: false,
				public: {enabled: true},
				min_sdk_version: "2",
				permitted_packages: ["myothersdklibrary"],
			}

			java_sdk_library {
				name: "mycoreplatform",
				apex_available: ["myapex"],
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
		checkUnversionedAndroidBpContents(`
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
        stub_flags: "hiddenapi/stub-flags.csv",
        annotation_flags: "hiddenapi/annotation-flags.csv",
        metadata: "hiddenapi/metadata.csv",
        index: "hiddenapi/index.csv",
        all_flags: "hiddenapi/all-flags.csv",
    },
}

java_import {
    name: "mybootlib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    jars: ["java/mybootlib.jar"],
}

java_sdk_library_import {
    name: "myothersdklibrary",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    shared_library: false,
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
    shared_library: false,
    public: {
        jars: ["sdk_library/public/mycoreplatform-stubs.jar"],
        stub_srcs: ["sdk_library/public/mycoreplatform_stub_sources"],
        current_api: "sdk_library/public/mycoreplatform.txt",
        removed_api: "sdk_library/public/mycoreplatform-removed.txt",
        sdk_version: "current",
    },
}
		`),
		checkVersionedAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_bootclasspath_fragment {
    name: "mysdk_mybootclasspathfragment@current",
    sdk_member_name: "mybootclasspathfragment",
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    contents: [
        "mysdk_mybootlib@current",
        "mysdk_myothersdklibrary@current",
    ],
    api: {
        stub_libs: ["mysdk_mysdklibrary@current"],
    },
    core_platform_api: {
        stub_libs: ["mysdk_mycoreplatform@current"],
    },
    hidden_api: {
        stub_flags: "hiddenapi/stub-flags.csv",
        annotation_flags: "hiddenapi/annotation-flags.csv",
        metadata: "hiddenapi/metadata.csv",
        index: "hiddenapi/index.csv",
        all_flags: "hiddenapi/all-flags.csv",
    },
}

java_import {
    name: "mysdk_mybootlib@current",
    sdk_member_name: "mybootlib",
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    jars: ["java/mybootlib.jar"],
}

java_sdk_library_import {
    name: "mysdk_myothersdklibrary@current",
    sdk_member_name: "myothersdklibrary",
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    shared_library: false,
    public: {
        jars: ["sdk_library/public/myothersdklibrary-stubs.jar"],
        stub_srcs: ["sdk_library/public/myothersdklibrary_stub_sources"],
        current_api: "sdk_library/public/myothersdklibrary.txt",
        removed_api: "sdk_library/public/myothersdklibrary-removed.txt",
        sdk_version: "current",
    },
}

java_sdk_library_import {
    name: "mysdk_mysdklibrary@current",
    sdk_member_name: "mysdklibrary",
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
    name: "mysdk_mycoreplatform@current",
    sdk_member_name: "mycoreplatform",
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    shared_library: false,
    public: {
        jars: ["sdk_library/public/mycoreplatform-stubs.jar"],
        stub_srcs: ["sdk_library/public/mycoreplatform_stub_sources"],
        current_api: "sdk_library/public/mycoreplatform.txt",
        removed_api: "sdk_library/public/mycoreplatform-removed.txt",
        sdk_version: "current",
    },
}

sdk_snapshot {
    name: "mysdk@current",
    visibility: ["//visibility:public"],
    bootclasspath_fragments: ["mysdk_mybootclasspathfragment@current"],
    java_boot_libs: ["mysdk_mybootlib@current"],
    java_sdk_libs: [
        "mysdk_myothersdklibrary@current",
        "mysdk_mysdklibrary@current",
        "mysdk_mycoreplatform@current",
    ],
}
		`),
		checkAllCopyRules(`
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/stub-flags.csv -> hiddenapi/stub-flags.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/annotation-flags.csv -> hiddenapi/annotation-flags.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/metadata.csv -> hiddenapi/metadata.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/index.csv -> hiddenapi/index.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/all-flags.csv -> hiddenapi/all-flags.csv
.intermediates/mybootlib/android_common/javac/mybootlib.jar -> java/mybootlib.jar
.intermediates/myothersdklibrary.stubs/android_common/javac/myothersdklibrary.stubs.jar -> sdk_library/public/myothersdklibrary-stubs.jar
.intermediates/myothersdklibrary.stubs.source/android_common/metalava/myothersdklibrary.stubs.source_api.txt -> sdk_library/public/myothersdklibrary.txt
.intermediates/myothersdklibrary.stubs.source/android_common/metalava/myothersdklibrary.stubs.source_removed.txt -> sdk_library/public/myothersdklibrary-removed.txt
.intermediates/mysdklibrary.stubs/android_common/javac/mysdklibrary.stubs.jar -> sdk_library/public/mysdklibrary-stubs.jar
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_api.txt -> sdk_library/public/mysdklibrary.txt
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_removed.txt -> sdk_library/public/mysdklibrary-removed.txt
.intermediates/mycoreplatform.stubs/android_common/javac/mycoreplatform.stubs.jar -> sdk_library/public/mycoreplatform-stubs.jar
.intermediates/mycoreplatform.stubs.source/android_common/metalava/mycoreplatform.stubs.source_api.txt -> sdk_library/public/mycoreplatform.txt
.intermediates/mycoreplatform.stubs.source/android_common/metalava/mycoreplatform.stubs.source_removed.txt -> sdk_library/public/mycoreplatform-removed.txt
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
		android.FixtureAddFile("java/mybootlib.jar", nil),
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

		sdk_snapshot {
			name: "mysdk@1",
			bootclasspath_fragments: ["mysdk_mybootclasspathfragment@1"],
		}

		prebuilt_bootclasspath_fragment {
			name: "mysdk_mybootclasspathfragment@1",
			sdk_member_name: "mybootclasspathfragment",
			prefer: false,
			visibility: ["//visibility:public"],
			apex_available: [
				"myapex",
			],
			image_name: "art",
			contents: ["mysdk_mybootlib@1"],
		}

		java_import {
			name: "mysdk_mybootlib@1",
			sdk_member_name: "mybootlib",
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
		java.FixtureWithLastReleaseApis("mysdklibrary"),
		java.FixtureConfigureUpdatableBootJars("myapex:mybootlib"),
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
				contents: ["mybootlib"],
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
			}
		`),
	).RunTest(t)

	// A preparer to update the test fixture used when processing an unpackage snapshot.
	preparerForSnapshot := fixtureAddPrebuiltApexForBootclasspathFragment("myapex", "mybootclasspathfragment")

	CheckSnapshot(t, result, "mysdk", "",
		checkUnversionedAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_bootclasspath_fragment {
    name: "mybootclasspathfragment",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    contents: ["mybootlib"],
    api: {
        stub_libs: ["mysdklibrary"],
    },
    hidden_api: {
        unsupported: ["hiddenapi/my-unsupported.txt"],
        removed: ["hiddenapi/my-removed.txt"],
        max_target_r_low_priority: ["hiddenapi/my-max-target-r-low-priority.txt"],
        max_target_q: ["hiddenapi/my-max-target-q.txt"],
        max_target_p: ["hiddenapi/my-max-target-p.txt"],
        max_target_o_low_priority: ["hiddenapi/my-max-target-o-low-priority.txt"],
        blocked: ["hiddenapi/my-blocked.txt"],
        unsupported_packages: ["hiddenapi/my-unsupported-packages.txt"],
        stub_flags: "hiddenapi/stub-flags.csv",
        annotation_flags: "hiddenapi/annotation-flags.csv",
        metadata: "hiddenapi/metadata.csv",
        index: "hiddenapi/index.csv",
        all_flags: "hiddenapi/all-flags.csv",
    },
}

java_import {
    name: "mybootlib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    jars: ["java/mybootlib.jar"],
}

java_sdk_library_import {
    name: "mysdklibrary",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    shared_library: true,
    compile_dex: true,
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
my-max-target-p.txt -> hiddenapi/my-max-target-p.txt
my-max-target-o-low-priority.txt -> hiddenapi/my-max-target-o-low-priority.txt
my-blocked.txt -> hiddenapi/my-blocked.txt
my-unsupported-packages.txt -> hiddenapi/my-unsupported-packages.txt
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/stub-flags.csv -> hiddenapi/stub-flags.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/annotation-flags.csv -> hiddenapi/annotation-flags.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/metadata.csv -> hiddenapi/metadata.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/index.csv -> hiddenapi/index.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/all-flags.csv -> hiddenapi/all-flags.csv
.intermediates/mybootlib/android_common/javac/mybootlib.jar -> java/mybootlib.jar
.intermediates/mysdklibrary.stubs/android_common/javac/mysdklibrary.stubs.jar -> sdk_library/public/mysdklibrary-stubs.jar
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_api.txt -> sdk_library/public/mysdklibrary.txt
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_removed.txt -> sdk_library/public/mysdklibrary-removed.txt
`),
		snapshotTestPreparer(checkSnapshotWithoutSource, preparerForSnapshot),
		snapshotTestPreparer(checkSnapshotWithSourcePreferred, preparerForSnapshot),
		snapshotTestPreparer(checkSnapshotPreferredWithSource, preparerForSnapshot),
	)
}
