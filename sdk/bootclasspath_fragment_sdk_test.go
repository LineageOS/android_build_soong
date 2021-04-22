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
		prepareForSdkTestWithApex,

		// Some additional files needed for the art apex.
		android.FixtureMergeMockFs(android.MockFS{
			"com.android.art.avbpubkey":                          nil,
			"com.android.art.pem":                                nil,
			"system/sepolicy/apex/com.android.art-file_contexts": nil,
		}),
		java.FixtureConfigureBootJars("com.android.art:mybootlib"),
		android.FixtureWithRootAndroidBp(`
			sdk {
				name: "mysdk",
				bootclasspath_fragments: ["mybootclasspathfragment"],
				java_boot_libs: ["mybootlib"],
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

	CheckSnapshot(t, result, "mysdk", "",
		checkUnversionedAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_bootclasspath_fragment {
    name: "mybootclasspathfragment",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["com.android.art"],
    image_name: "art",
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
		snapshotTestPreparer(checkSnapshotPreferredWithSource, android.GroupFixturePreparers(
			android.FixtureAddTextFile("prebuilts/apex/Android.bp", `
				prebuilt_apex {
					name: "com.android.art",
					src: "art.apex",
					exported_java_libs: [
						"mybootlib",
					],
				}
			`),
			android.FixtureAddFile("prebuilts/apex/art.apex", nil),
		),
		),
	)
}

func TestSnapshotWithBootClasspathFragment_Contents(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		android.FixtureWithRootAndroidBp(`
			sdk {
				name: "mysdk",
				bootclasspath_fragments: ["mybootclasspathfragment"],
				java_boot_libs: ["mybootlib"],
			}

			bootclasspath_fragment {
				name: "mybootclasspathfragment",
				contents: ["mybootlib"],
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
}

java_import {
    name: "mybootlib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["java/mybootlib.jar"],
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
}

java_import {
    name: "mysdk_mybootlib@current",
    sdk_member_name: "mybootlib",
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
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
