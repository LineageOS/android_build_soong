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
	"android/soong/dexpreopt"
	"android/soong/java"
)

func testSnapshotWithSystemServerClasspathFragment(t *testing.T, targetBuildRelease string, expectedUnversionedSdkSnapshot string, expectedVersionedSdkSnapshot string) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		java.PrepareForTestWithJavaDefaultModules,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("mysdklibrary"),
		dexpreopt.FixtureSetApexSystemServerJars("myapex:mylib", "myapex:mysdklibrary"),
		android.FixtureModifyEnv(func(env map[string]string) {
			env["SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE"] = targetBuildRelease
		}),
		prepareForSdkTestWithApex,

		android.FixtureWithRootAndroidBp(`
			sdk {
				name: "mysdk",
				systemserverclasspath_fragments: ["mysystemserverclasspathfragment"],
				java_sdk_libs: [
					// This is not strictly needed as it should be automatically added to the sdk_snapshot as
					// a java_sdk_libs module because it is used in the mysystemserverclasspathfragment's
					// contents property. However, it is specified here to ensure that duplicates are
					// correctly deduped.
					"mysdklibrary",
				],
			}

			apex {
				name: "myapex",
				key: "myapex.key",
				min_sdk_version: "2",
				systemserverclasspath_fragments: ["mysystemserverclasspathfragment"],
			}

			systemserverclasspath_fragment {
				name: "mysystemserverclasspathfragment",
				apex_available: ["myapex"],
				contents: [
					"mylib",
					"mysdklibrary",
				],
			}

			java_library {
				name: "mylib",
				apex_available: ["myapex"],
				srcs: ["Test.java"],
				system_modules: "none",
				sdk_version: "none",
				min_sdk_version: "2",
				compile_dex: true,
				permitted_packages: ["mylib"],
			}

			java_sdk_library {
				name: "mysdklibrary",
				apex_available: ["myapex"],
				srcs: ["Test.java"],
				shared_library: false,
				public: {enabled: true},
				min_sdk_version: "2",
			}
		`),
	).RunTest(t)

	CheckSnapshot(t, result, "mysdk", "",
		checkUnversionedAndroidBpContents(expectedUnversionedSdkSnapshot),
		checkVersionedAndroidBpContents(expectedVersionedSdkSnapshot),
	)
}

func TestSnapshotWithSystemServerClasspathFragment(t *testing.T) {
	t.Run("target-s", func(t *testing.T) {
		testSnapshotWithSystemServerClasspathFragment(t, "S", `
// This is auto-generated. DO NOT EDIT.

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
`, `
// This is auto-generated. DO NOT EDIT.

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

sdk_snapshot {
    name: "mysdk@current",
    visibility: ["//visibility:public"],
    java_sdk_libs: ["mysdk_mysdklibrary@current"],
}
`)
	})

	t.Run("target-t", func(t *testing.T) {
		testSnapshotWithSystemServerClasspathFragment(t, "Tiramisu", `
// This is auto-generated. DO NOT EDIT.

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

java_import {
    name: "mylib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    jars: ["java_systemserver_libs/snapshot/jars/are/invalid/mylib.jar"],
    permitted_packages: ["mylib"],
}

prebuilt_systemserverclasspath_fragment {
    name: "mysystemserverclasspathfragment",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    contents: [
        "mylib",
        "mysdklibrary",
    ],
}
`, `
// This is auto-generated. DO NOT EDIT.

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

java_import {
    name: "mysdk_mylib@current",
    sdk_member_name: "mylib",
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    jars: ["java_systemserver_libs/snapshot/jars/are/invalid/mylib.jar"],
    permitted_packages: ["mylib"],
}

prebuilt_systemserverclasspath_fragment {
    name: "mysdk_mysystemserverclasspathfragment@current",
    sdk_member_name: "mysystemserverclasspathfragment",
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    contents: [
        "mysdk_mylib@current",
        "mysdk_mysdklibrary@current",
    ],
}

sdk_snapshot {
    name: "mysdk@current",
    visibility: ["//visibility:public"],
    java_sdk_libs: ["mysdk_mysdklibrary@current"],
    java_systemserver_libs: ["mysdk_mylib@current"],
    systemserverclasspath_fragments: ["mysdk_mysystemserverclasspathfragment@current"],
}
`)
	})
}
