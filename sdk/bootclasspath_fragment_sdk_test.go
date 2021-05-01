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
	"testing"

	"android/soong/android"
	"android/soong/java"
)

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

		// platform_bootclasspath that depends on the fragment.
		android.FixtureAddTextFile("frameworks/base/boot/Android.bp", `
			platform_bootclasspath {
				name: "platform-bootclasspath",
				fragments: [
					{
						apex: "com.android.art",
						module: "mybootclasspathfragment",
					},
				],
			}
		`),
		// Needed for platform_bootclasspath
		android.FixtureAddFile("frameworks/base/config/boot-profile.txt", nil),

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

	// A preparer to add a prebuilt apex to the test fixture.
	prepareWithPrebuiltApex := android.GroupFixturePreparers(
		android.FixtureAddTextFile("prebuilts/apex/Android.bp", `
				prebuilt_apex {
					name: "com.android.art",
					src: "art.apex",
					exported_java_libs: [
						"mybootlib",
					],
					exported_bootclasspath_fragments: [
						"mybootclasspathfragment",
					],
				}
			`),
		android.FixtureAddFile("prebuilts/apex/art.apex", nil),
	)

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
		snapshotTestPreparer(checkSnapshotWithoutSource, prepareWithPrebuiltApex),
		snapshotTestPreparer(checkSnapshotWithSourcePreferred, prepareWithPrebuiltApex),
		snapshotTestPreparer(checkSnapshotPreferredWithSource, prepareWithPrebuiltApex),
	)
}

func TestSnapshotWithBootClasspathFragment_Contents(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		java.PrepareForTestWithJavaDefaultModules,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("mysdklibrary", "mycoreplatform"),
		android.FixtureWithRootAndroidBp(`
			sdk {
				name: "mysdk",
				bootclasspath_fragments: ["mybootclasspathfragment"],
				java_sdk_libs: ["mysdklibrary", "mycoreplatform"],
			}

			bootclasspath_fragment {
				name: "mybootclasspathfragment",
				contents: ["mybootlib"],
				api: {
					stub_libs: ["mysdklibrary"],
				},
				core_platform_api: {
					stub_libs: ["mycoreplatform"],
				},
			}

			java_library {
				name: "mybootlib",
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
			}

			java_sdk_library {
				name: "mycoreplatform",
				srcs: ["Test.java"],
				compile_dex: true,
				public: {enabled: true},
			}
		`),
	).RunTest(t)

	CheckSnapshot(t, result, "mysdk", "",
		checkUnversionedAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_bootclasspath_fragment {
    name: "mybootclasspathfragment",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    contents: ["mybootlib"],
    api: {
        stub_libs: ["mysdklibrary"],
    },
    core_platform_api: {
        stub_libs: ["mycoreplatform"],
    },
}

java_import {
    name: "mybootlib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
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

java_sdk_library_import {
    name: "mycoreplatform",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
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
		checkVersionedAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_bootclasspath_fragment {
    name: "mysdk_mybootclasspathfragment@current",
    sdk_member_name: "mybootclasspathfragment",
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    contents: ["mysdk_mybootlib@current"],
    api: {
        stub_libs: ["mysdk_mysdklibrary@current"],
    },
    core_platform_api: {
        stub_libs: ["mysdk_mycoreplatform@current"],
    },
}

java_import {
    name: "mysdk_mybootlib@current",
    sdk_member_name: "mybootlib",
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["java/mybootlib.jar"],
}

java_sdk_library_import {
    name: "mysdk_mysdklibrary@current",
    sdk_member_name: "mysdklibrary",
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

java_sdk_library_import {
    name: "mysdk_mycoreplatform@current",
    sdk_member_name: "mycoreplatform",
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
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

sdk_snapshot {
    name: "mysdk@current",
    visibility: ["//visibility:public"],
    bootclasspath_fragments: ["mysdk_mybootclasspathfragment@current"],
    java_boot_libs: ["mysdk_mybootlib@current"],
    java_sdk_libs: [
        "mysdk_mysdklibrary@current",
        "mysdk_mycoreplatform@current",
    ],
}
`),
		checkAllCopyRules(`
.intermediates/mybootlib/android_common/javac/mybootlib.jar -> java/mybootlib.jar
.intermediates/mysdklibrary.stubs/android_common/javac/mysdklibrary.stubs.jar -> sdk_library/public/mysdklibrary-stubs.jar
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_api.txt -> sdk_library/public/mysdklibrary.txt
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_removed.txt -> sdk_library/public/mysdklibrary-removed.txt
.intermediates/mycoreplatform.stubs/android_common/javac/mycoreplatform.stubs.jar -> sdk_library/public/mycoreplatform-stubs.jar
.intermediates/mycoreplatform.stubs.source/android_common/metalava/mycoreplatform.stubs.source_api.txt -> sdk_library/public/mycoreplatform.txt
.intermediates/mycoreplatform.stubs.source/android_common/metalava/mycoreplatform.stubs.source_removed.txt -> sdk_library/public/mycoreplatform-removed.txt
`))
}

// Test that bootclasspath_fragment works with sdk.
func TestBasicSdkWithBootclasspathFragment(t *testing.T) {
	android.GroupFixturePreparers(
		prepareForSdkTestWithApex,
		prepareForSdkTestWithJava,
		android.FixtureWithRootAndroidBp(`
		sdk {
			name: "mysdk",
			bootclasspath_fragments: ["mybootclasspathfragment"],
		}

		bootclasspath_fragment {
			name: "mybootclasspathfragment",
			image_name: "art",
			apex_available: ["myapex"],
		}

		sdk_snapshot {
			name: "mysdk@1",
			bootclasspath_fragments: ["mybootclasspathfragment_mysdk_1"],
		}

		prebuilt_bootclasspath_fragment {
			name: "mybootclasspathfragment_mysdk_1",
			sdk_member_name: "mybootclasspathfragment",
			prefer: false,
			visibility: ["//visibility:public"],
			apex_available: [
				"myapex",
			],
			image_name: "art",
		}
	`),
	).RunTest(t)
}

func TestSnapshotWithBootclasspathFragment_HiddenAPI(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
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

			bootclasspath_fragment {
				name: "mybootclasspathfragment",
				contents: ["mybootlib"],
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
				srcs: ["Test.java"],
				system_modules: "none",
				sdk_version: "none",
				compile_dex: true,
			}
		`),
	).RunTest(t)

	CheckSnapshot(t, result, "mysdk", "",
		checkUnversionedAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_bootclasspath_fragment {
    name: "mybootclasspathfragment",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    contents: ["mybootlib"],
    hidden_api: {
        unsupported: ["hiddenapi/my-unsupported.txt"],
        removed: ["hiddenapi/my-removed.txt"],
        max_target_r_low_priority: ["hiddenapi/my-max-target-r-low-priority.txt"],
        max_target_q: ["hiddenapi/my-max-target-q.txt"],
        max_target_p: ["hiddenapi/my-max-target-p.txt"],
        max_target_o_low_priority: ["hiddenapi/my-max-target-o-low-priority.txt"],
        blocked: ["hiddenapi/my-blocked.txt"],
        unsupported_packages: ["hiddenapi/my-unsupported-packages.txt"],
    },
}

java_import {
    name: "mybootlib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["java/mybootlib.jar"],
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
.intermediates/mybootlib/android_common/javac/mybootlib.jar -> java/mybootlib.jar
`),
	)
}
