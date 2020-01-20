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
	"testing"
)

func testSdkWithJava(t *testing.T, bp string) *testSdkResult {
	t.Helper()

	fs := map[string][]byte{
		"Test.java":              nil,
		"aidl/foo/bar/Test.aidl": nil,
	}
	return testSdkWithFs(t, bp, fs)
}

// Contains tests for SDK members provided by the java package.

func TestBasicSdkWithJavaLibrary(t *testing.T) {
	result := testSdkWithJava(t, `
		sdk {
			name: "mysdk",
			java_header_libs: ["sdkmember"],
		}

		sdk_snapshot {
			name: "mysdk@1",
			java_header_libs: ["sdkmember_mysdk_1"],
		}

		sdk_snapshot {
			name: "mysdk@2",
			java_header_libs: ["sdkmember_mysdk_2"],
		}

		java_library {
			name: "sdkmember",
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
			host_supported: true,
			apex_available: [
				"//apex_available:platform",
				"//apex_available:anyapex",
			],
		}

		java_import {
			name: "sdkmember_mysdk_1",
			sdk_member_name: "sdkmember",
			host_supported: true,
			apex_available: [
				"//apex_available:platform",
				"//apex_available:anyapex",
			],
		}

		java_import {
			name: "sdkmember_mysdk_2",
			sdk_member_name: "sdkmember",
			host_supported: true,
			apex_available: [
				"//apex_available:platform",
				"//apex_available:anyapex",
			],
		}

		java_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			libs: ["sdkmember"],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
			host_supported: true,
			apex_available: [
				"myapex",
				"myapex2",
			],
		}

		apex {
			name: "myapex",
			java_libs: ["myjavalib"],
			uses_sdks: ["mysdk@1"],
			key: "myapex.key",
			certificate: ":myapex.cert",
		}

		apex {
			name: "myapex2",
			java_libs: ["myjavalib"],
			uses_sdks: ["mysdk@2"],
			key: "myapex.key",
			certificate: ":myapex.cert",
		}
	`)

	sdkMemberV1 := result.ctx.ModuleForTests("sdkmember_mysdk_1", "android_common_myapex").Rule("combineJar").Output
	sdkMemberV2 := result.ctx.ModuleForTests("sdkmember_mysdk_2", "android_common_myapex2").Rule("combineJar").Output

	javalibForMyApex := result.ctx.ModuleForTests("myjavalib", "android_common_myapex")
	javalibForMyApex2 := result.ctx.ModuleForTests("myjavalib", "android_common_myapex2")

	// Depending on the uses_sdks value, different libs are linked
	ensureListContains(t, pathsToStrings(javalibForMyApex.Rule("javac").Implicits), sdkMemberV1.String())
	ensureListContains(t, pathsToStrings(javalibForMyApex2.Rule("javac").Implicits), sdkMemberV2.String())
}

func TestSnapshotWithJavaHeaderLibrary(t *testing.T) {
	result := testSdkWithJava(t, `
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
		}
	`)

	result.CheckSnapshot("mysdk", "android_common", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "mysdk_myjavalib@current",
    sdk_member_name: "myjavalib",
    jars: ["java/myjavalib.jar"],
}

java_import {
    name: "myjavalib",
    prefer: false,
    jars: ["java/myjavalib.jar"],
}

sdk_snapshot {
    name: "mysdk@current",
    java_header_libs: ["mysdk_myjavalib@current"],
}

`),
		checkAllCopyRules(`
.intermediates/myjavalib/android_common/turbine-combined/myjavalib.jar -> java/myjavalib.jar
aidl/foo/bar/Test.aidl -> aidl/aidl/foo/bar/Test.aidl
`),
	)
}

func TestHostSnapshotWithJavaHeaderLibrary(t *testing.T) {
	// b/145598135 - Generating host snapshots for anything other than linux is not supported.
	SkipIfNotLinux(t)

	result := testSdkWithJava(t, `
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

	result.CheckSnapshot("mysdk", "linux_glibc_common", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "mysdk_myjavalib@current",
    sdk_member_name: "myjavalib",
    device_supported: false,
    host_supported: true,
    jars: ["java/myjavalib.jar"],
}

java_import {
    name: "myjavalib",
    prefer: false,
    device_supported: false,
    host_supported: true,
    jars: ["java/myjavalib.jar"],
}

sdk_snapshot {
    name: "mysdk@current",
    device_supported: false,
    host_supported: true,
    java_header_libs: ["mysdk_myjavalib@current"],
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib/linux_glibc_common/javac/myjavalib.jar -> java/myjavalib.jar
aidl/foo/bar/Test.aidl -> aidl/aidl/foo/bar/Test.aidl
`),
	)
}

func TestSnapshotWithJavaImplLibrary(t *testing.T) {
	result := testSdkWithJava(t, `
		module_exports {
			name: "myexports",
			java_libs: ["myjavalib"],
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
		}
	`)

	result.CheckSnapshot("myexports", "android_common", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "myexports_myjavalib@current",
    sdk_member_name: "myjavalib",
    jars: ["java/myjavalib.jar"],
}

java_import {
    name: "myjavalib",
    prefer: false,
    jars: ["java/myjavalib.jar"],
}

module_exports_snapshot {
    name: "myexports@current",
    java_libs: ["myexports_myjavalib@current"],
}

`),
		checkAllCopyRules(`
.intermediates/myjavalib/android_common/javac/myjavalib.jar -> java/myjavalib.jar
aidl/foo/bar/Test.aidl -> aidl/aidl/foo/bar/Test.aidl
`),
	)
}

func TestHostSnapshotWithJavaImplLibrary(t *testing.T) {
	// b/145598135 - Generating host snapshots for anything other than linux is not supported.
	SkipIfNotLinux(t)

	result := testSdkWithJava(t, `
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

	result.CheckSnapshot("myexports", "linux_glibc_common", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "myexports_myjavalib@current",
    sdk_member_name: "myjavalib",
    device_supported: false,
    host_supported: true,
    jars: ["java/myjavalib.jar"],
}

java_import {
    name: "myjavalib",
    prefer: false,
    device_supported: false,
    host_supported: true,
    jars: ["java/myjavalib.jar"],
}

module_exports_snapshot {
    name: "myexports@current",
    device_supported: false,
    host_supported: true,
    java_libs: ["myexports_myjavalib@current"],
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib/linux_glibc_common/javac/myjavalib.jar -> java/myjavalib.jar
aidl/foo/bar/Test.aidl -> aidl/aidl/foo/bar/Test.aidl
`),
	)
}

func TestSnapshotWithJavaTest(t *testing.T) {
	result := testSdkWithJava(t, `
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

	result.CheckSnapshot("myexports", "android_common", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_test_import {
    name: "myexports_myjavatests@current",
    sdk_member_name: "myjavatests",
    jars: ["java/myjavatests.jar"],
    test_config: "java/myjavatests-AndroidTest.xml",
}

java_test_import {
    name: "myjavatests",
    prefer: false,
    jars: ["java/myjavatests.jar"],
    test_config: "java/myjavatests-AndroidTest.xml",
}

module_exports_snapshot {
    name: "myexports@current",
    java_tests: ["myexports_myjavatests@current"],
}
`),
		checkAllCopyRules(`
.intermediates/myjavatests/android_common/javac/myjavatests.jar -> java/myjavatests.jar
.intermediates/myjavatests/android_common/myjavatests.config -> java/myjavatests-AndroidTest.xml
`),
	)
}

func TestHostSnapshotWithJavaTest(t *testing.T) {
	// b/145598135 - Generating host snapshots for anything other than linux is not supported.
	SkipIfNotLinux(t)

	result := testSdkWithJava(t, `
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

	result.CheckSnapshot("myexports", "linux_glibc_common", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_test_import {
    name: "myexports_myjavatests@current",
    sdk_member_name: "myjavatests",
    device_supported: false,
    host_supported: true,
    jars: ["java/myjavatests.jar"],
    test_config: "java/myjavatests-AndroidTest.xml",
}

java_test_import {
    name: "myjavatests",
    prefer: false,
    device_supported: false,
    host_supported: true,
    jars: ["java/myjavatests.jar"],
    test_config: "java/myjavatests-AndroidTest.xml",
}

module_exports_snapshot {
    name: "myexports@current",
    device_supported: false,
    host_supported: true,
    java_tests: ["myexports_myjavatests@current"],
}
`),
		checkAllCopyRules(`
.intermediates/myjavatests/linux_glibc_common/javac/myjavatests.jar -> java/myjavatests.jar
.intermediates/myjavatests/linux_glibc_common/myjavatests.config -> java/myjavatests-AndroidTest.xml
`),
	)
}

func testSdkWithDroidstubs(t *testing.T, bp string) *testSdkResult {
	t.Helper()

	fs := map[string][]byte{
		"foo/bar/Foo.java":               nil,
		"stubs-sources/foo/bar/Foo.java": nil,
	}
	return testSdkWithFs(t, bp, fs)
}

// Note: This test does not verify that a droidstubs can be referenced, either
// directly or indirectly from an APEX as droidstubs can never be a part of an
// apex.
func TestBasicSdkWithDroidstubs(t *testing.T) {
	testSdkWithDroidstubs(t, `
		sdk {
				name: "mysdk",
				stubs_sources: ["mystub"],
		}
		sdk_snapshot {
				name: "mysdk@10",
				stubs_sources: ["mystub_mysdk@10"],
		}
		prebuilt_stubs_sources {
				name: "mystub_mysdk@10",
				sdk_member_name: "mystub",
				srcs: ["stubs-sources/foo/bar/Foo.java"],
		}
		droidstubs {
				name: "mystub",
				srcs: ["foo/bar/Foo.java"],
				sdk_version: "none",
				system_modules: "none",
		}
		java_library {
				name: "myjavalib",
				srcs: [":mystub"],
				sdk_version: "none",
				system_modules: "none",
		}
	`)
}

func TestSnapshotWithDroidstubs(t *testing.T) {
	result := testSdkWithDroidstubs(t, `
		module_exports {
			name: "myexports",
			stubs_sources: ["myjavaapistubs"],
		}

		droidstubs {
			name: "myjavaapistubs",
			srcs: ["foo/bar/Foo.java"],
			system_modules: "none",
			sdk_version: "none",
		}
	`)

	result.CheckSnapshot("myexports", "android_common", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_stubs_sources {
    name: "myexports_myjavaapistubs@current",
    sdk_member_name: "myjavaapistubs",
    srcs: ["java/myjavaapistubs_stubs_sources"],
}

prebuilt_stubs_sources {
    name: "myjavaapistubs",
    prefer: false,
    srcs: ["java/myjavaapistubs_stubs_sources"],
}

module_exports_snapshot {
    name: "myexports@current",
    stubs_sources: ["myexports_myjavaapistubs@current"],
}

`),
		checkAllCopyRules(""),
		checkMergeZip(".intermediates/myexports/android_common/tmp/java/myjavaapistubs_stubs_sources.zip"),
	)
}

func TestHostSnapshotWithDroidstubs(t *testing.T) {
	// b/145598135 - Generating host snapshots for anything other than linux is not supported.
	SkipIfNotLinux(t)

	result := testSdkWithDroidstubs(t, `
		module_exports {
			name: "myexports",
			device_supported: false,
			host_supported: true,
			stubs_sources: ["myjavaapistubs"],
		}

		droidstubs {
			name: "myjavaapistubs",
			device_supported: false,
			host_supported: true,
			srcs: ["foo/bar/Foo.java"],
			system_modules: "none",
			sdk_version: "none",
		}
	`)

	result.CheckSnapshot("myexports", "linux_glibc_common", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_stubs_sources {
    name: "myexports_myjavaapistubs@current",
    sdk_member_name: "myjavaapistubs",
    device_supported: false,
    host_supported: true,
    srcs: ["java/myjavaapistubs_stubs_sources"],
}

prebuilt_stubs_sources {
    name: "myjavaapistubs",
    prefer: false,
    device_supported: false,
    host_supported: true,
    srcs: ["java/myjavaapistubs_stubs_sources"],
}

module_exports_snapshot {
    name: "myexports@current",
    device_supported: false,
    host_supported: true,
    stubs_sources: ["myexports_myjavaapistubs@current"],
}
`),
		checkAllCopyRules(""),
		checkMergeZip(".intermediates/myexports/linux_glibc_common/tmp/java/myjavaapistubs_stubs_sources.zip"),
	)
}

func TestSnapshotWithJavaSystemModules(t *testing.T) {
	result := testSdkWithJava(t, `
		sdk {
			name: "mysdk",
			java_header_libs: ["exported-system-module"],
			java_system_modules: ["my-system-modules"],
		}

		java_system_modules {
			name: "my-system-modules",
			libs: ["system-module", "exported-system-module"],
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

	result.CheckSnapshot("mysdk", "android_common", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "mysdk_exported-system-module@current",
    sdk_member_name: "exported-system-module",
    jars: ["java/exported-system-module.jar"],
}

java_import {
    name: "exported-system-module",
    prefer: false,
    jars: ["java/exported-system-module.jar"],
}

java_import {
    name: "mysdk_system-module@current",
    sdk_member_name: "system-module",
    visibility: ["//visibility:private"],
    jars: ["java/system-module.jar"],
}

java_import {
    name: "mysdk_system-module",
    prefer: false,
    visibility: ["//visibility:private"],
    jars: ["java/system-module.jar"],
}

java_system_modules_import {
    name: "mysdk_my-system-modules@current",
    sdk_member_name: "my-system-modules",
    libs: [
        "mysdk_system-module@current",
        "mysdk_exported-system-module@current",
    ],
}

java_system_modules_import {
    name: "my-system-modules",
    prefer: false,
    libs: [
        "mysdk_system-module",
        "exported-system-module",
    ],
}

sdk_snapshot {
    name: "mysdk@current",
    java_header_libs: ["mysdk_exported-system-module@current"],
    java_system_modules: ["mysdk_my-system-modules@current"],
}
`),
		checkAllCopyRules(`
.intermediates/exported-system-module/android_common/turbine-combined/exported-system-module.jar -> java/exported-system-module.jar
.intermediates/system-module/android_common/turbine-combined/system-module.jar -> java/system-module.jar
`),
	)
}

func TestHostSnapshotWithJavaSystemModules(t *testing.T) {
	// b/145598135 - Generating host snapshots for anything other than linux is not supported.
	SkipIfNotLinux(t)

	result := testSdkWithJava(t, `
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

	result.CheckSnapshot("mysdk", "linux_glibc_common", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "mysdk_system-module@current",
    sdk_member_name: "system-module",
    visibility: ["//visibility:private"],
    device_supported: false,
    host_supported: true,
    jars: ["java/system-module.jar"],
}

java_import {
    name: "mysdk_system-module",
    prefer: false,
    visibility: ["//visibility:private"],
    device_supported: false,
    host_supported: true,
    jars: ["java/system-module.jar"],
}

java_system_modules_import {
    name: "mysdk_my-system-modules@current",
    sdk_member_name: "my-system-modules",
    device_supported: false,
    host_supported: true,
    libs: ["mysdk_system-module@current"],
}

java_system_modules_import {
    name: "my-system-modules",
    prefer: false,
    device_supported: false,
    host_supported: true,
    libs: ["mysdk_system-module"],
}

sdk_snapshot {
    name: "mysdk@current",
    device_supported: false,
    host_supported: true,
    java_system_modules: ["mysdk_my-system-modules@current"],
}
`),
		checkAllCopyRules(".intermediates/system-module/linux_glibc_common/javac/system-module.jar -> java/system-module.jar"),
	)
}
