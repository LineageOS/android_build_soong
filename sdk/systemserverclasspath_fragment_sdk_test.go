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

func testSnapshotWithSystemServerClasspathFragment(t *testing.T, sdk string, targetBuildRelease string, expectedSdkSnapshot string) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		java.PrepareForTestWithJavaDefaultModules,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("mysdklibrary"),
		dexpreopt.FixtureSetApexSystemServerJars("myapex:mylib", "myapex:mysdklibrary"),
		android.FixtureModifyEnv(func(env map[string]string) {
			if targetBuildRelease != "latest" {
				env["SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE"] = targetBuildRelease
			}
		}),
		prepareForSdkTestWithApex,

		android.FixtureWithRootAndroidBp(sdk+`
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
				dex_preopt: {
					profile: "art-profile",
				},
			}

			java_sdk_library {
				name: "mysdklibrary",
				apex_available: ["myapex"],
				srcs: ["Test.java"],
				shared_library: false,
				public: {enabled: true},
				min_sdk_version: "2",
				dex_preopt: {
					profile: "art-profile",
				},
			}
		`),
	).RunTest(t)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(expectedSdkSnapshot),
	)
}

func TestSnapshotWithPartialSystemServerClasspathFragment(t *testing.T) {
	commonSdk := `
		apex {
			name: "myapex",
			key: "myapex.key",
			min_sdk_version: "Tiramisu",
			systemserverclasspath_fragments: ["mysystemserverclasspathfragment"],
		}
		systemserverclasspath_fragment {
			name: "mysystemserverclasspathfragment",
			apex_available: ["myapex"],
			contents: [
				"mysdklibrary",
				"mysdklibrary-future",
			],
		}
		java_sdk_library {
			name: "mysdklibrary",
			apex_available: ["myapex"],
			srcs: ["Test.java"],
			min_sdk_version: "33", // Tiramisu
		}
		java_sdk_library {
			name: "mysdklibrary-future",
			apex_available: ["myapex"],
			srcs: ["Test.java"],
			min_sdk_version: "34", // UpsideDownCake
		}
		sdk {
			name: "mysdk",
			apexes: ["myapex"],
		}
	`

	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		java.PrepareForTestWithJavaDefaultModules,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("mysdklibrary", "mysdklibrary-future"),
		dexpreopt.FixtureSetApexSystemServerJars("myapex:mysdklibrary", "myapex:mysdklibrary-future"),
		android.FixtureModifyEnv(func(env map[string]string) {
			// targeting Tiramisu here means that we won't export mysdklibrary-future
			env["SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE"] = "Tiramisu"
		}),
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.Platform_version_active_codenames = []string{"UpsideDownCake"}
		}),
		prepareForSdkTestWithApex,
		android.FixtureWithRootAndroidBp(commonSdk),
	).RunTest(t)

	CheckSnapshot(t, result, "mysdk", "", checkAndroidBpContents(
		`// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "mysdklibrary",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    shared_library: true,
    public: {
        jars: ["sdk_library/public/mysdklibrary-stubs.jar"],
        stub_srcs: ["sdk_library/public/mysdklibrary_stub_sources"],
        current_api: "sdk_library/public/mysdklibrary.txt",
        removed_api: "sdk_library/public/mysdklibrary-removed.txt",
        sdk_version: "current",
    },
    system: {
        jars: ["sdk_library/system/mysdklibrary-stubs.jar"],
        stub_srcs: ["sdk_library/system/mysdklibrary_stub_sources"],
        current_api: "sdk_library/system/mysdklibrary.txt",
        removed_api: "sdk_library/system/mysdklibrary-removed.txt",
        sdk_version: "system_current",
    },
    test: {
        jars: ["sdk_library/test/mysdklibrary-stubs.jar"],
        stub_srcs: ["sdk_library/test/mysdklibrary_stub_sources"],
        current_api: "sdk_library/test/mysdklibrary.txt",
        removed_api: "sdk_library/test/mysdklibrary-removed.txt",
        sdk_version: "test_current",
    },
}

prebuilt_systemserverclasspath_fragment {
    name: "mysystemserverclasspathfragment",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    contents: ["mysdklibrary"],
} `))
}

func TestSnapshotWithEmptySystemServerClasspathFragment(t *testing.T) {
	commonSdk := `
		apex {
			name: "myapex",
			key: "myapex.key",
			min_sdk_version: "Tiramisu",
			systemserverclasspath_fragments: ["mysystemserverclasspathfragment"],
		}
		systemserverclasspath_fragment {
			name: "mysystemserverclasspathfragment",
			apex_available: ["myapex"],
			contents: ["mysdklibrary"],
		}
		java_sdk_library {
			name: "mysdklibrary",
			apex_available: ["myapex"],
			srcs: ["Test.java"],
			min_sdk_version: "34", // UpsideDownCake
		}
		sdk {
			name: "mysdk",
			apexes: ["myapex"],
		}
	`

	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		java.PrepareForTestWithJavaDefaultModules,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("mysdklibrary"),
		dexpreopt.FixtureSetApexSystemServerJars("myapex:mysdklibrary"),
		android.FixtureModifyEnv(func(env map[string]string) {
			// targeting Tiramisu here means that we won't export mysdklibrary
			env["SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE"] = "Tiramisu"
		}),
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.Platform_version_active_codenames = []string{"UpsideDownCake"}
		}),
		prepareForSdkTestWithApex,
		android.FixtureWithRootAndroidBp(commonSdk),
	).RunTest(t)

	CheckSnapshot(t, result, "mysdk", "", checkAndroidBpContents(`// This is auto-generated. DO NOT EDIT.`))
}

func TestSnapshotWithSystemServerClasspathFragment(t *testing.T) {

	commonSdk := `
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
	`

	expectedLatestSnapshot := `
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "mysdklibrary",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    shared_library: false,
    dex_preopt: {
        profile_guided: true,
    },
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
    min_sdk_version: "2",
    permitted_packages: ["mylib"],
    dex_preopt: {
        profile_guided: true,
    },
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
`

	t.Run("target-s", func(t *testing.T) {
		testSnapshotWithSystemServerClasspathFragment(t, commonSdk, "S", `
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
`)
	})

	t.Run("target-t", func(t *testing.T) {
		testSnapshotWithSystemServerClasspathFragment(t, commonSdk, "Tiramisu", `
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
    min_sdk_version: "2",
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
`)
	})

	t.Run("target-u", func(t *testing.T) {
		testSnapshotWithSystemServerClasspathFragment(t, commonSdk, "UpsideDownCake", `
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "mysdklibrary",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    shared_library: false,
    dex_preopt: {
        profile_guided: true,
    },
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
    min_sdk_version: "2",
    permitted_packages: ["mylib"],
    dex_preopt: {
        profile_guided: true,
    },
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
`)
	})

	t.Run("added-directly", func(t *testing.T) {
		testSnapshotWithSystemServerClasspathFragment(t, commonSdk, `latest`, expectedLatestSnapshot)
	})

	t.Run("added-via-apex", func(t *testing.T) {
		testSnapshotWithSystemServerClasspathFragment(t, `
			sdk {
				name: "mysdk",
				apexes: ["myapex"],
			}
		`, `latest`, expectedLatestSnapshot)
	})
}
