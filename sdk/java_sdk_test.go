// Copyright (C) 2019 The Android Open Source Project
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
	"testing"

	"android/soong/android"
	"android/soong/dexpreopt"
	"android/soong/java"
)

var prepareForSdkTestWithJava = android.GroupFixturePreparers(
	java.PrepareForTestWithJavaBuildComponents,
	PrepareForTestWithSdkBuildComponents,
	dexpreopt.PrepareForTestWithFakeDex2oatd,

	// Ensure that all source paths are provided. This helps ensure that the snapshot generation is
	// consistent and all files referenced from the snapshot's Android.bp file have actually been
	// copied into the snapshot.
	android.PrepareForTestDisallowNonExistentPaths,

	// Files needs by most of the tests.
	android.MockFS{
		"Test.java":   nil,
		"art-profile": nil,
	}.AddToFixture(),
)

var prepareForSdkTestWithJavaSdkLibrary = android.GroupFixturePreparers(
	prepareForSdkTestWithJava,
	java.PrepareForTestWithJavaDefaultModules,
	java.PrepareForTestWithJavaSdkLibraryFiles,
	java.FixtureWithLastReleaseApis("myjavalib"),
	android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
		variables.BuildFlags = map[string]string{
			"RELEASE_HIDDEN_API_EXPORTABLE_STUBS": "true",
		}
	}),
)

// Contains tests for SDK members provided by the java package.

func TestSdkDependsOnSourceEvenWhenPrebuiltPreferred(t *testing.T) {
	result := android.GroupFixturePreparers(prepareForSdkTestWithJava).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_header_libs: ["sdkmember"],
		}

		java_library {
			name: "sdkmember",
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
		}
	`)

	// Make sure that the mysdk module depends on "sdkmember" and not "prebuilt_sdkmember".
	sdkChecker := func(t *testing.T, result *android.TestResult) {
		java.CheckModuleDependencies(t, result.TestContext, "mysdk", "android_common", []string{"sdkmember"})
	}

	CheckSnapshot(t, result, "mysdk", "",
		snapshotTestChecker(checkSnapshotWithSourcePreferred, sdkChecker),
		snapshotTestChecker(checkSnapshotPreferredWithSource, sdkChecker),
	)
}

func TestSnapshotWithJavaHeaderLibrary(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		android.FixtureAddFile("aidl/foo/bar/Test.aidl", nil),
	).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_header_libs: ["myjavalib"],
		}

		java_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			aidl: {
				export_include_dirs: ["aidl"],
			},
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
			host_supported: true,
			permitted_packages: ["pkg.myjavalib"],
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["java/myjavalib.jar"],
    permitted_packages: ["pkg.myjavalib"],
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib/android_common/turbine-combined/myjavalib.jar -> java/myjavalib.jar
aidl/foo/bar/Test.aidl -> aidl/aidl/foo/bar/Test.aidl
`),
	)
}

func TestHostSnapshotWithJavaHeaderLibrary(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		android.FixtureAddFile("aidl/foo/bar/Test.aidl", nil),
	).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			device_supported: false,
			host_supported: true,
			java_header_libs: ["myjavalib"],
		}

		java_library {
			name: "myjavalib",
			device_supported: false,
			host_supported: true,
			srcs: ["Test.java"],
			aidl: {
				export_include_dirs: ["aidl"],
			},
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    device_supported: false,
    host_supported: true,
    jars: ["java/myjavalib.jar"],
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib/linux_glibc_common/javac-header/myjavalib.jar -> java/myjavalib.jar
aidl/foo/bar/Test.aidl -> aidl/aidl/foo/bar/Test.aidl
`),
	)
}

func TestDeviceAndHostSnapshotWithJavaHeaderLibrary(t *testing.T) {
	result := android.GroupFixturePreparers(prepareForSdkTestWithJava).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			host_supported: true,
			java_header_libs: ["myjavalib"],
		}

		java_library {
			name: "myjavalib",
			host_supported: true,
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    host_supported: true,
    target: {
        android: {
            jars: ["java/android/myjavalib.jar"],
        },
        linux_glibc: {
            jars: ["java/linux_glibc/myjavalib.jar"],
        },
    },
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib/android_common/turbine-combined/myjavalib.jar -> java/android/myjavalib.jar
.intermediates/myjavalib/linux_glibc_common/javac-header/myjavalib.jar -> java/linux_glibc/myjavalib.jar
`),
	)
}

func TestSnapshotWithJavaImplLibrary(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		android.FixtureAddFile("aidl/foo/bar/Test.aidl", nil),
		android.FixtureAddFile("resource.txt", nil),
	).RunTestWithBp(t, `
		module_exports {
			name: "myexports",
			java_libs: ["myjavalib"],
		}

		java_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			java_resources: ["resource.txt"],
			aidl: {
				export_include_dirs: ["aidl"],
			},
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
			host_supported: true,
		}
	`)

	CheckSnapshot(t, result, "myexports", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["java/myjavalib.jar"],
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib/android_common/withres/myjavalib.jar -> java/myjavalib.jar
aidl/foo/bar/Test.aidl -> aidl/aidl/foo/bar/Test.aidl
`),
	)
}

func TestSnapshotWithJavaBootLibrary(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		android.FixtureAddFile("aidl", nil),
		android.FixtureAddFile("resource.txt", nil),
	).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_boot_libs: ["myjavalib"],
		}

		java_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			java_resources: ["resource.txt"],
			// The aidl files should not be copied to the snapshot because a java_boot_libs member is not
			// intended to be used for compiling Java, only for accessing the dex implementation jar.
			aidl: {
				export_include_dirs: ["aidl"],
			},
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
			permitted_packages: ["pkg.myjavalib"],
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["java_boot_libs/snapshot/jars/are/invalid/myjavalib.jar"],
    permitted_packages: ["pkg.myjavalib"],
}
`),
		checkAllCopyRules(`
.intermediates/mysdk/common_os/empty -> java_boot_libs/snapshot/jars/are/invalid/myjavalib.jar
`),
	)
}

func TestSnapshotWithJavaBootLibrary_UpdatableMedia(t *testing.T) {
	runTest := func(t *testing.T, targetBuildRelease, expectedJarPath, expectedCopyRule string) {
		result := android.GroupFixturePreparers(
			prepareForSdkTestWithJava,
			android.FixtureMergeEnv(map[string]string{
				"SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE": targetBuildRelease,
			}),
		).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_boot_libs: ["updatable-media"],
		}

		java_library {
			name: "updatable-media",
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
			permitted_packages: ["pkg.media"],
			apex_available: ["com.android.media"],
		}
	`)

		CheckSnapshot(t, result, "mysdk", "",
			checkAndroidBpContents(fmt.Sprintf(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "updatable-media",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["com.android.media"],
    jars: ["%s"],
    permitted_packages: ["pkg.media"],
}
`, expectedJarPath)),
			checkAllCopyRules(expectedCopyRule),
		)
	}

	t.Run("updatable-media in S", func(t *testing.T) {
		runTest(t, "S", "java/updatable-media.jar", `
.intermediates/updatable-media/android_common/package-check/updatable-media.jar -> java/updatable-media.jar
`)
	})

	t.Run("updatable-media in T", func(t *testing.T) {
		runTest(t, "Tiramisu", "java_boot_libs/snapshot/jars/are/invalid/updatable-media.jar", `
.intermediates/mysdk/common_os/empty -> java_boot_libs/snapshot/jars/are/invalid/updatable-media.jar
`)
	})
}

func TestSnapshotWithJavaLibrary_MinSdkVersion(t *testing.T) {
	runTest := func(t *testing.T, targetBuildRelease, minSdkVersion, expectedMinSdkVersion string) {
		result := android.GroupFixturePreparers(
			prepareForSdkTestWithJava,
			android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				variables.Platform_version_active_codenames = []string{"S", "Tiramisu", "Unfinalized"}
			}),
			android.FixtureMergeEnv(map[string]string{
				"SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE": targetBuildRelease,
			}),
		).RunTestWithBp(t, fmt.Sprintf(`
		sdk {
			name: "mysdk",
			java_header_libs: ["mylib"],
		}

		java_library {
			name: "mylib",
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
			min_sdk_version: "%s",
		}
	`, minSdkVersion))

		expectedMinSdkVersionLine := ""
		if expectedMinSdkVersion != "" {
			expectedMinSdkVersionLine = fmt.Sprintf("    min_sdk_version: %q,\n", expectedMinSdkVersion)
		}

		CheckSnapshot(t, result, "mysdk", "",
			checkAndroidBpContents(fmt.Sprintf(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "mylib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["java/mylib.jar"],
%s}
`, expectedMinSdkVersionLine)),
		)
	}

	t.Run("min_sdk_version=S in S", func(t *testing.T) {
		// min_sdk_version was not added to java_import until Tiramisu.
		runTest(t, "S", "S", "")
	})

	t.Run("min_sdk_version=S in Tiramisu", func(t *testing.T) {
		// The canonical form of S is 31.
		runTest(t, "Tiramisu", "S", "31")
	})

	t.Run("min_sdk_version=24 in Tiramisu", func(t *testing.T) {
		// A numerical min_sdk_version is already in canonical form.
		runTest(t, "Tiramisu", "24", "24")
	})

	t.Run("min_sdk_version=Unfinalized in latest", func(t *testing.T) {
		// An unfinalized min_sdk_version has no numeric value yet.
		runTest(t, "", "Unfinalized", "Unfinalized")
	})
}

func TestSnapshotWithJavaSystemserverLibrary(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		android.FixtureAddFile("aidl", nil),
		android.FixtureAddFile("resource.txt", nil),
	).RunTestWithBp(t, `
		module_exports {
			name: "myexports",
			java_systemserver_libs: ["myjavalib"],
		}

		java_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			java_resources: ["resource.txt"],
			// The aidl files should not be copied to the snapshot because a java_systemserver_libs member
			// is not intended to be used for compiling Java, only for accessing the dex implementation
			// jar.
			aidl: {
				export_include_dirs: ["aidl"],
			},
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
			permitted_packages: ["pkg.myjavalib"],
		}
	`)

	CheckSnapshot(t, result, "myexports", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["java_systemserver_libs/snapshot/jars/are/invalid/myjavalib.jar"],
    permitted_packages: ["pkg.myjavalib"],
}
`),
		checkAllCopyRules(`
.intermediates/myexports/common_os/empty -> java_systemserver_libs/snapshot/jars/are/invalid/myjavalib.jar
`),
	)
}

func TestHostSnapshotWithJavaImplLibrary(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		android.FixtureAddFile("aidl/foo/bar/Test.aidl", nil),
	).RunTestWithBp(t, `
		module_exports {
			name: "myexports",
			device_supported: false,
			host_supported: true,
			java_libs: ["myjavalib"],
		}

		java_library {
			name: "myjavalib",
			device_supported: false,
			host_supported: true,
			srcs: ["Test.java"],
			aidl: {
				export_include_dirs: ["aidl"],
			},
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
		}
	`)

	CheckSnapshot(t, result, "myexports", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    device_supported: false,
    host_supported: true,
    jars: ["java/myjavalib.jar"],
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib/linux_glibc_common/javac/myjavalib.jar -> java/myjavalib.jar
aidl/foo/bar/Test.aidl -> aidl/aidl/foo/bar/Test.aidl
`),
	)
}

func TestSnapshotWithJavaTest(t *testing.T) {
	result := android.GroupFixturePreparers(prepareForSdkTestWithJava).RunTestWithBp(t, `
		module_exports {
			name: "myexports",
			java_tests: ["myjavatests"],
		}

		java_test {
			name: "myjavatests",
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
			host_supported: true,
		}
	`)

	CheckSnapshot(t, result, "myexports", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_test_import {
    name: "myjavatests",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["java/myjavatests.jar"],
    test_config: "java/myjavatests-AndroidTest.xml",
}
`),
		checkAllCopyRules(`
.intermediates/myjavatests/android_common/javac/myjavatests.jar -> java/myjavatests.jar
.intermediates/myjavatests/android_common/myjavatests.config -> java/myjavatests-AndroidTest.xml
`),
	)
}

func TestHostSnapshotWithJavaTest(t *testing.T) {
	result := android.GroupFixturePreparers(prepareForSdkTestWithJava).RunTestWithBp(t, `
		module_exports {
			name: "myexports",
			device_supported: false,
			host_supported: true,
			java_tests: ["myjavatests"],
		}

		java_test {
			name: "myjavatests",
			device_supported: false,
			host_supported: true,
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
		}
	`)

	CheckSnapshot(t, result, "myexports", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_test_import {
    name: "myjavatests",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    device_supported: false,
    host_supported: true,
    jars: ["java/myjavatests.jar"],
    test_config: "java/myjavatests-AndroidTest.xml",
}
`),
		checkAllCopyRules(`
.intermediates/myjavatests/linux_glibc_common/javac/myjavatests.jar -> java/myjavatests.jar
.intermediates/myjavatests/linux_glibc_common/myjavatests.config -> java/myjavatests-AndroidTest.xml
`),
	)
}

func TestSnapshotWithJavaSystemModules(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		java.PrepareForTestWithJavaDefaultModules,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithPrebuiltApisAndExtensions(map[string][]string{
			"31":      {"myjavalib"},
			"32":      {"myjavalib"},
			"current": {"myjavalib"},
		}, map[string][]string{
			"1": {"myjavalib"},
			"2": {"myjavalib"},
		}),
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.BuildFlags = map[string]string{
				"RELEASE_HIDDEN_API_EXPORTABLE_STUBS": "true",
			}
		}),
	).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_header_libs: ["exported-system-module"],
			java_sdk_libs: ["myjavalib"],
			java_system_modules: ["my-system-modules"],
		}

		java_sdk_library {
			name: "myjavalib",
			apex_available: ["//apex_available:anyapex"],
			srcs: ["Test.java"],
			sdk_version: "current",
			shared_library: false,
			public: {
				enabled: true,
			},
		}

		java_system_modules {
			name: "my-system-modules",
			libs: ["system-module", "exported-system-module", "myjavalib.stubs"],
		}

		java_library {
			name: "system-module",
			srcs: ["Test.java"],
			sdk_version: "none",
			system_modules: "none",
		}

		java_library {
			name: "exported-system-module",
			srcs: ["Test.java"],
			sdk_version: "none",
			system_modules: "none",
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "exported-system-module",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["java/exported-system-module.jar"],
}

java_import {
    name: "mysdk_system-module",
    prefer: false,
    visibility: ["//visibility:private"],
    apex_available: ["//apex_available:platform"],
    jars: ["java/system-module.jar"],
}

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:anyapex"],
    shared_library: false,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "current",
    },
}

java_system_modules_import {
    name: "my-system-modules",
    prefer: false,
    visibility: ["//visibility:public"],
    libs: [
        "mysdk_system-module",
        "exported-system-module",
        "myjavalib.stubs",
    ],
}
`),
		checkAllCopyRules(`
.intermediates/exported-system-module/android_common/turbine-combined/exported-system-module.jar -> java/exported-system-module.jar
.intermediates/system-module/android_common/turbine-combined/system-module.jar -> java/system-module.jar
.intermediates/myjavalib.stubs.exportable/android_common/combined/myjavalib.stubs.exportable.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
`),
		checkInfoContents(result.Config, `
[
  {
    "@type": "sdk",
    "@name": "mysdk",
    "java_header_libs": [
      "exported-system-module",
      "system-module"
    ],
    "java_sdk_libs": [
      "myjavalib"
    ],
    "java_system_modules": [
      "my-system-modules"
    ]
  },
  {
    "@type": "java_library",
    "@name": "exported-system-module"
  },
  {
    "@type": "java_system_modules",
    "@name": "my-system-modules",
    "@deps": [
      "exported-system-module",
      "system-module"
    ]
  },
  {
    "@type": "java_sdk_library",
    "@name": "myjavalib",
    "dist_stem": "myjavalib",
    "scopes": {
      "public": {
        "current_api": "sdk_library/public/myjavalib.txt",
        "latest_api": "out/soong/.intermediates/prebuilts/sdk/myjavalib.api.public.latest/gen/myjavalib.api.public.latest",
        "latest_removed_api": "out/soong/.intermediates/prebuilts/sdk/myjavalib-removed.api.public.latest/gen/myjavalib-removed.api.public.latest",
        "removed_api": "sdk_library/public/myjavalib-removed.txt"
      }
    }
  },
  {
    "@type": "java_library",
    "@name": "system-module"
  }
]
`),
	)
}

func TestHostSnapshotWithJavaSystemModules(t *testing.T) {
	result := android.GroupFixturePreparers(prepareForSdkTestWithJava).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			device_supported: false,
			host_supported: true,
			java_system_modules: ["my-system-modules"],
		}

		java_system_modules {
			name: "my-system-modules",
			device_supported: false,
			host_supported: true,
			libs: ["system-module"],
		}

		java_library {
			name: "system-module",
			device_supported: false,
			host_supported: true,
			srcs: ["Test.java"],
			sdk_version: "none",
			system_modules: "none",
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "mysdk_system-module",
    prefer: false,
    visibility: ["//visibility:private"],
    apex_available: ["//apex_available:platform"],
    device_supported: false,
    host_supported: true,
    jars: ["java/system-module.jar"],
}

java_system_modules_import {
    name: "my-system-modules",
    prefer: false,
    visibility: ["//visibility:public"],
    device_supported: false,
    host_supported: true,
    libs: ["mysdk_system-module"],
}
`),
		checkAllCopyRules(".intermediates/system-module/linux_glibc_common/javac-header/system-module.jar -> java/system-module.jar"),
	)
}

func TestDeviceAndHostSnapshotWithOsSpecificMembers(t *testing.T) {
	result := android.GroupFixturePreparers(prepareForSdkTestWithJava).RunTestWithBp(t, `
		module_exports {
			name: "myexports",
			host_supported: true,
			java_libs: ["myjavalib"],
			target: {
				android: {
					java_header_libs: ["androidjavalib"],
				},
				host: {
					java_header_libs: ["hostjavalib"],
				},
			},
		}

		java_library {
			name: "myjavalib",
			host_supported: true,
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
		}

		java_library {
			name: "androidjavalib",
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
		}

		java_library_host {
			name: "hostjavalib",
			srcs: ["Test.java"],
		}
	`)

	CheckSnapshot(t, result, "myexports", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "hostjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    device_supported: false,
    host_supported: true,
    jars: ["java/hostjavalib.jar"],
}

java_import {
    name: "androidjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["java/androidjavalib.jar"],
}

java_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    host_supported: true,
    target: {
        android: {
            jars: ["java/android/myjavalib.jar"],
        },
        linux_glibc: {
            jars: ["java/linux_glibc/myjavalib.jar"],
        },
    },
}
`),
		checkAllCopyRules(`
.intermediates/hostjavalib/linux_glibc_common/javac-header/hostjavalib.jar -> java/hostjavalib.jar
.intermediates/androidjavalib/android_common/turbine-combined/androidjavalib.jar -> java/androidjavalib.jar
.intermediates/myjavalib/android_common/javac/myjavalib.jar -> java/android/myjavalib.jar
.intermediates/myjavalib/linux_glibc_common/javac/myjavalib.jar -> java/linux_glibc/myjavalib.jar
`),
	)
}

func TestSnapshotWithJavaSdkLibrary(t *testing.T) {
	result := android.GroupFixturePreparers(prepareForSdkTestWithJavaSdkLibrary).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_sdk_libs: ["myjavalib"],
		}

		java_sdk_library {
			name: "myjavalib",
			apex_available: ["//apex_available:anyapex"],
			srcs: ["Test.java"],
			sdk_version: "current",
			shared_library: false,
			stubs_library_visibility: ["//other"],
			stubs_source_visibility: ["//another"],
			permitted_packages: ["pkg.myjavalib"],
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:anyapex"],
    shared_library: false,
    permitted_packages: ["pkg.myjavalib"],
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "current",
    },
    system: {
        jars: ["sdk_library/system/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/system/myjavalib_stub_sources"],
        current_api: "sdk_library/system/myjavalib.txt",
        removed_api: "sdk_library/system/myjavalib-removed.txt",
        sdk_version: "system_current",
    },
    test: {
        jars: ["sdk_library/test/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/test/myjavalib_stub_sources"],
        current_api: "sdk_library/test/myjavalib.txt",
        removed_api: "sdk_library/test/myjavalib-removed.txt",
        sdk_version: "test_current",
    },
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib.stubs.exportable/android_common/combined/myjavalib.stubs.exportable.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
.intermediates/myjavalib.stubs.exportable.system/android_common/combined/myjavalib.stubs.exportable.system.jar -> sdk_library/system/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source.system/android_common/exportable/myjavalib.stubs.source.system_api.txt -> sdk_library/system/myjavalib.txt
.intermediates/myjavalib.stubs.source.system/android_common/exportable/myjavalib.stubs.source.system_removed.txt -> sdk_library/system/myjavalib-removed.txt
.intermediates/myjavalib.stubs.exportable.test/android_common/combined/myjavalib.stubs.exportable.test.jar -> sdk_library/test/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source.test/android_common/exportable/myjavalib.stubs.source.test_api.txt -> sdk_library/test/myjavalib.txt
.intermediates/myjavalib.stubs.source.test/android_common/exportable/myjavalib.stubs.source.test_removed.txt -> sdk_library/test/myjavalib-removed.txt
`),
		checkMergeZips(
			".intermediates/mysdk/common_os/tmp/sdk_library/public/myjavalib_stub_sources.zip",
			".intermediates/mysdk/common_os/tmp/sdk_library/system/myjavalib_stub_sources.zip",
			".intermediates/mysdk/common_os/tmp/sdk_library/test/myjavalib_stub_sources.zip",
		),
	)
}

func TestSnapshotWithJavaSdkLibrary_DistStem(t *testing.T) {
	result := android.GroupFixturePreparers(prepareForSdkTestWithJavaSdkLibrary).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_sdk_libs: ["myjavalib-foo"],
		}

		java_sdk_library {
			name: "myjavalib-foo",
			apex_available: ["//apex_available:anyapex"],
			srcs: ["Test.java"],
			sdk_version: "current",
			shared_library: false,
			public: {
				enabled: true,
			},
			dist_stem: "myjavalib",
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "myjavalib-foo",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:anyapex"],
    shared_library: false,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "current",
    },
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib-foo.stubs.exportable/android_common/combined/myjavalib-foo.stubs.exportable.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib-foo.stubs.source/android_common/exportable/myjavalib-foo.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib-foo.stubs.source/android_common/exportable/myjavalib-foo.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
`),
		checkMergeZips(
			".intermediates/mysdk/common_os/tmp/sdk_library/public/myjavalib_stub_sources.zip",
		),
	)
}

func TestSnapshotWithJavaSdkLibrary_UseSrcJar(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJavaSdkLibrary,
		android.FixtureMergeEnv(map[string]string{
			"SOONG_SDK_SNAPSHOT_USE_SRCJAR": "true",
		}),
	).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_sdk_libs: ["myjavalib"],
		}

		java_sdk_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			sdk_version: "current",
			shared_library: false,
			public: {
				enabled: true,
			},
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    shared_library: false,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib.srcjar"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "current",
    },
}
		`),
		checkAllCopyRules(`
.intermediates/myjavalib.stubs.exportable/android_common/combined/myjavalib.stubs.exportable.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source-stubs.srcjar -> sdk_library/public/myjavalib.srcjar
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
		`),
	)
}

func TestSnapshotWithJavaSdkLibrary_AnnotationsZip(t *testing.T) {
	result := android.GroupFixturePreparers(prepareForSdkTestWithJavaSdkLibrary).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_sdk_libs: ["myjavalib"],
		}

		java_sdk_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			sdk_version: "current",
			shared_library: false,
			annotations_enabled: true,
			public: {
				enabled: true,
			},
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    shared_library: false,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        annotations: "sdk_library/public/myjavalib_annotations.zip",
        sdk_version: "current",
    },
}
		`),
		checkAllCopyRules(`
.intermediates/myjavalib.stubs.exportable/android_common/combined/myjavalib.stubs.exportable.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_annotations.zip -> sdk_library/public/myjavalib_annotations.zip
		`),
		checkMergeZips(".intermediates/mysdk/common_os/tmp/sdk_library/public/myjavalib_stub_sources.zip"),
	)
}

func TestSnapshotWithJavaSdkLibrary_AnnotationsZip_PreT(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJavaSdkLibrary,
		android.FixtureMergeEnv(map[string]string{
			"SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE": "S",
		}),
	).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_sdk_libs: ["myjavalib"],
		}

		java_sdk_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			sdk_version: "S",
			shared_library: false,
			annotations_enabled: true,
			public: {
				enabled: true,
			},
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    shared_library: false,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "current",
    },
}
		`),
		checkAllCopyRules(`
.intermediates/myjavalib.stubs.exportable/android_common/combined/myjavalib.stubs.exportable.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
		`),
		checkMergeZips(".intermediates/mysdk/common_os/tmp/sdk_library/public/myjavalib_stub_sources.zip"),
	)
}

func TestSnapshotWithJavaSdkLibrary_CompileDex(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJavaSdkLibrary,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.BuildFlags = map[string]string{
				"RELEASE_HIDDEN_API_EXPORTABLE_STUBS": "true",
			}
		}),
	).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_sdk_libs: ["myjavalib"],
		}

		java_sdk_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			sdk_version: "current",
			shared_library: false,
			compile_dex: true,
			public: {
				enabled: true,
			},
			system: {
				enabled: true,
			},
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    shared_library: false,
    compile_dex: true,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "current",
    },
    system: {
        jars: ["sdk_library/system/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/system/myjavalib_stub_sources"],
        current_api: "sdk_library/system/myjavalib.txt",
        removed_api: "sdk_library/system/myjavalib-removed.txt",
        sdk_version: "system_current",
    },
}
`),
		snapshotTestChecker(checkSnapshotWithSourcePreferred, func(t *testing.T, result *android.TestResult) {
			ctx := android.ModuleInstallPathContextForTesting(result.Config)
			dexJarBuildPath := func(name string, kind android.SdkKind) string {
				dep := result.Module(name, "android_common").(java.SdkLibraryDependency)
				path := dep.SdkApiExportableStubDexJar(ctx, kind).Path()
				return path.RelativeToTop().String()
			}

			dexJarPath := dexJarBuildPath("myjavalib", android.SdkPublic)
			android.AssertStringEquals(t, "source dex public stubs jar build path", "out/soong/.intermediates/myjavalib.stubs.exportable/android_common/dex/myjavalib.stubs.exportable.jar", dexJarPath)

			dexJarPath = dexJarBuildPath("myjavalib", android.SdkSystem)
			systemDexJar := "out/soong/.intermediates/myjavalib.stubs.exportable.system/android_common/dex/myjavalib.stubs.exportable.system.jar"
			android.AssertStringEquals(t, "source dex system stubs jar build path", systemDexJar, dexJarPath)

			// This should fall back to system as module is not available.
			dexJarPath = dexJarBuildPath("myjavalib", android.SdkModule)
			android.AssertStringEquals(t, "source dex module stubs jar build path", systemDexJar, dexJarPath)

			// Prebuilt dex jar does not come from the exportable stubs.
			dexJarPath = dexJarBuildPath(android.PrebuiltNameFromSource("myjavalib"), android.SdkPublic)
			android.AssertStringEquals(t, "prebuilt dex public stubs jar build path", "out/soong/.intermediates/snapshot/prebuilt_myjavalib.stubs/android_common/dex/myjavalib.stubs.jar", dexJarPath)
		}),
	)
}

func TestSnapshotWithJavaSdkLibrary_SdkVersion_None(t *testing.T) {
	result := android.GroupFixturePreparers(prepareForSdkTestWithJavaSdkLibrary).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_sdk_libs: ["myjavalib"],
		}

		java_sdk_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			sdk_version: "none",
			system_modules: "none",
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    shared_library: true,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "none",
    },
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib.stubs.exportable/android_common/combined/myjavalib.stubs.exportable.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
`),
		checkMergeZips(
			".intermediates/mysdk/common_os/tmp/sdk_library/public/myjavalib_stub_sources.zip",
		),
	)
}

func TestSnapshotWithJavaSdkLibrary_SdkVersion_ForScope(t *testing.T) {
	result := android.GroupFixturePreparers(prepareForSdkTestWithJavaSdkLibrary).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_sdk_libs: ["myjavalib"],
		}

		java_sdk_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			sdk_version: "module_current",
			public: {
				enabled: true,
				sdk_version: "module_current",
			},
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    shared_library: true,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "module_current",
    },
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib.stubs.exportable/android_common/combined/myjavalib.stubs.exportable.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
`),
		checkMergeZips(
			".intermediates/mysdk/common_os/tmp/sdk_library/public/myjavalib_stub_sources.zip",
		),
	)
}

func TestSnapshotWithJavaSdkLibrary_ApiScopes(t *testing.T) {
	result := android.GroupFixturePreparers(prepareForSdkTestWithJavaSdkLibrary).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_sdk_libs: ["myjavalib"],
		}

		java_sdk_library {
			name: "myjavalib",
			apex_available: ["//apex_available:anyapex"],
			srcs: ["Test.java"],
			sdk_version: "current",
			public: {
				enabled: true,
			},
			system: {
				enabled: true,
			},
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:anyapex"],
    shared_library: true,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "current",
    },
    system: {
        jars: ["sdk_library/system/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/system/myjavalib_stub_sources"],
        current_api: "sdk_library/system/myjavalib.txt",
        removed_api: "sdk_library/system/myjavalib-removed.txt",
        sdk_version: "system_current",
    },
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib.stubs.exportable/android_common/combined/myjavalib.stubs.exportable.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
.intermediates/myjavalib.stubs.exportable.system/android_common/combined/myjavalib.stubs.exportable.system.jar -> sdk_library/system/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source.system/android_common/exportable/myjavalib.stubs.source.system_api.txt -> sdk_library/system/myjavalib.txt
.intermediates/myjavalib.stubs.source.system/android_common/exportable/myjavalib.stubs.source.system_removed.txt -> sdk_library/system/myjavalib-removed.txt
`),
		checkMergeZips(
			".intermediates/mysdk/common_os/tmp/sdk_library/public/myjavalib_stub_sources.zip",
			".intermediates/mysdk/common_os/tmp/sdk_library/system/myjavalib_stub_sources.zip",
		),
	)
}

func TestSnapshotWithJavaSdkLibrary_ModuleLib(t *testing.T) {
	result := android.GroupFixturePreparers(prepareForSdkTestWithJavaSdkLibrary).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_sdk_libs: ["myjavalib"],
		}

		java_sdk_library {
			name: "myjavalib",
			apex_available: ["//apex_available:anyapex"],
			srcs: ["Test.java"],
			sdk_version: "current",
			public: {
				enabled: true,
			},
			system: {
				enabled: true,
			},
			module_lib: {
				enabled: true,
			},
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:anyapex"],
    shared_library: true,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "current",
    },
    system: {
        jars: ["sdk_library/system/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/system/myjavalib_stub_sources"],
        current_api: "sdk_library/system/myjavalib.txt",
        removed_api: "sdk_library/system/myjavalib-removed.txt",
        sdk_version: "system_current",
    },
    module_lib: {
        jars: ["sdk_library/module-lib/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/module-lib/myjavalib_stub_sources"],
        current_api: "sdk_library/module-lib/myjavalib.txt",
        removed_api: "sdk_library/module-lib/myjavalib-removed.txt",
        sdk_version: "module_current",
    },
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib.stubs.exportable/android_common/combined/myjavalib.stubs.exportable.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
.intermediates/myjavalib.stubs.exportable.system/android_common/combined/myjavalib.stubs.exportable.system.jar -> sdk_library/system/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source.system/android_common/exportable/myjavalib.stubs.source.system_api.txt -> sdk_library/system/myjavalib.txt
.intermediates/myjavalib.stubs.source.system/android_common/exportable/myjavalib.stubs.source.system_removed.txt -> sdk_library/system/myjavalib-removed.txt
.intermediates/myjavalib.stubs.exportable.module_lib/android_common/combined/myjavalib.stubs.exportable.module_lib.jar -> sdk_library/module-lib/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source.module_lib/android_common/exportable/myjavalib.stubs.source.module_lib_api.txt -> sdk_library/module-lib/myjavalib.txt
.intermediates/myjavalib.stubs.source.module_lib/android_common/exportable/myjavalib.stubs.source.module_lib_removed.txt -> sdk_library/module-lib/myjavalib-removed.txt
`),
		checkMergeZips(
			".intermediates/mysdk/common_os/tmp/sdk_library/module-lib/myjavalib_stub_sources.zip",
			".intermediates/mysdk/common_os/tmp/sdk_library/public/myjavalib_stub_sources.zip",
			".intermediates/mysdk/common_os/tmp/sdk_library/system/myjavalib_stub_sources.zip",
		),
	)
}

func TestSnapshotWithJavaSdkLibrary_SystemServer(t *testing.T) {
	result := android.GroupFixturePreparers(prepareForSdkTestWithJavaSdkLibrary).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_sdk_libs: ["myjavalib"],
		}

		java_sdk_library {
			name: "myjavalib",
			apex_available: ["//apex_available:anyapex"],
			srcs: ["Test.java"],
			sdk_version: "current",
			public: {
				enabled: true,
			},
			system_server: {
				enabled: true,
			},
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:anyapex"],
    shared_library: true,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "current",
    },
    system_server: {
        jars: ["sdk_library/system-server/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/system-server/myjavalib_stub_sources"],
        current_api: "sdk_library/system-server/myjavalib.txt",
        removed_api: "sdk_library/system-server/myjavalib-removed.txt",
        sdk_version: "system_server_current",
    },
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib.stubs.exportable/android_common/combined/myjavalib.stubs.exportable.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
.intermediates/myjavalib.stubs.exportable.system_server/android_common/combined/myjavalib.stubs.exportable.system_server.jar -> sdk_library/system-server/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source.system_server/android_common/exportable/myjavalib.stubs.source.system_server_api.txt -> sdk_library/system-server/myjavalib.txt
.intermediates/myjavalib.stubs.source.system_server/android_common/exportable/myjavalib.stubs.source.system_server_removed.txt -> sdk_library/system-server/myjavalib-removed.txt
`),
		checkMergeZips(
			".intermediates/mysdk/common_os/tmp/sdk_library/public/myjavalib_stub_sources.zip",
			".intermediates/mysdk/common_os/tmp/sdk_library/system-server/myjavalib_stub_sources.zip",
		),
	)
}

func TestSnapshotWithJavaSdkLibrary_NamingScheme(t *testing.T) {
	result := android.GroupFixturePreparers(prepareForSdkTestWithJavaSdkLibrary).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_sdk_libs: ["myjavalib"],
		}

		java_sdk_library {
			name: "myjavalib",
			apex_available: ["//apex_available:anyapex"],
			srcs: ["Test.java"],
			sdk_version: "current",
			naming_scheme: "default",
			public: {
				enabled: true,
			},
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:anyapex"],
    naming_scheme: "default",
    shared_library: true,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "current",
    },
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib.stubs.exportable/android_common/combined/myjavalib.stubs.exportable.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
`),
		checkMergeZips(
			".intermediates/mysdk/common_os/tmp/sdk_library/public/myjavalib_stub_sources.zip",
		),
	)
}

func TestSnapshotWithJavaSdkLibrary_DoctagFiles(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJavaSdkLibrary,
		android.FixtureAddFile("docs/known_doctags", nil),
	).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			java_sdk_libs: ["myjavalib"],
		}

		java_sdk_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			sdk_version: "current",
			public: {
				enabled: true,
			},
			doctag_files: ["docs/known_doctags"],
		}

		filegroup {
			name: "mygroup",
			srcs: [":myjavalib{.doctags}"],
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    shared_library: true,
    doctag_files: ["doctags/docs/known_doctags"],
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "current",
    },
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib.stubs.exportable/android_common/combined/myjavalib.stubs.exportable.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/exportable/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
docs/known_doctags -> doctags/docs/known_doctags
`),
	)
}
