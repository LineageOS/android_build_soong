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

		// For java_sdk_library
		"api/current.txt":                                   nil,
		"api/removed.txt":                                   nil,
		"api/system-current.txt":                            nil,
		"api/system-removed.txt":                            nil,
		"api/test-current.txt":                              nil,
		"api/test-removed.txt":                              nil,
		"api/module-lib-current.txt":                        nil,
		"api/module-lib-removed.txt":                        nil,
		"api/system-server-current.txt":                     nil,
		"api/system-server-removed.txt":                     nil,
		"build/soong/scripts/gen-java-current-api-files.sh": nil,
	}

	// for java_sdk_library tests
	bp = `
java_system_modules_import {
	name: "core-current-stubs-system-modules",
}
java_system_modules_import {
	name: "core-platform-api-stubs-system-modules",
}
java_import {
	name: "core.platform.api.stubs",
}
java_import {
	name: "android_stubs_current",
}
java_import {
	name: "android_system_stubs_current",
}
java_import {
	name: "android_test_stubs_current",
}
java_import {
	name: "android_module_lib_stubs_current",
}
java_import {
	name: "android_system_server_stubs_current",
}
java_import {
	name: "core-lambda-stubs", 
	sdk_version: "none",
}
java_import {
	name: "ext", 
	sdk_version: "none",
}
java_import {
	name: "framework", 
	sdk_version: "none",
}
` + bp

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
		}

		java_import {
			name: "sdkmember_mysdk_1",
			sdk_member_name: "sdkmember",
			host_supported: true,
		}

		java_import {
			name: "sdkmember_mysdk_2",
			sdk_member_name: "sdkmember",
			host_supported: true,
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

	sdkMemberV1 := result.ctx.ModuleForTests("sdkmember_mysdk_1", "android_common").Rule("combineJar").Output
	sdkMemberV2 := result.ctx.ModuleForTests("sdkmember_mysdk_2", "android_common").Rule("combineJar").Output

	javalibForMyApex := result.ctx.ModuleForTests("myjavalib", "android_common_apex10000_mysdk_1")
	javalibForMyApex2 := result.ctx.ModuleForTests("myjavalib", "android_common_apex10000_mysdk_2")

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

	result.CheckSnapshot("mysdk", "",
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

	result.CheckSnapshot("mysdk", "",
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

func TestDeviceAndHostSnapshotWithJavaHeaderLibrary(t *testing.T) {
	// b/145598135 - Generating host snapshots for anything other than linux is not supported.
	SkipIfNotLinux(t)

	result := testSdkWithJava(t, `
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

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "mysdk_myjavalib@current",
    sdk_member_name: "myjavalib",
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

java_import {
    name: "myjavalib",
    prefer: false,
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

sdk_snapshot {
    name: "mysdk@current",
    host_supported: true,
    java_header_libs: ["mysdk_myjavalib@current"],
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib/android_common/turbine-combined/myjavalib.jar -> java/android/myjavalib.jar
.intermediates/myjavalib/linux_glibc_common/javac/myjavalib.jar -> java/linux_glibc/myjavalib.jar
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

	result.CheckSnapshot("myexports", "",
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

	result.CheckSnapshot("myexports", "",
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

	result.CheckSnapshot("myexports", "",
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

	result.CheckSnapshot("myexports", "",
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

	result.CheckSnapshot("myexports", "",
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
		checkMergeZips(".intermediates/myexports/common_os/tmp/java/myjavaapistubs_stubs_sources.zip"),
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

	result.CheckSnapshot("myexports", "",
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
		checkMergeZips(".intermediates/myexports/common_os/tmp/java/myjavaapistubs_stubs_sources.zip"),
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

	result.CheckSnapshot("mysdk", "",
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

	result.CheckSnapshot("mysdk", "",
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

func TestDeviceAndHostSnapshotWithOsSpecificMembers(t *testing.T) {
	// b/145598135 - Generating host snapshots for anything other than linux is not supported.
	SkipIfNotLinux(t)

	result := testSdkWithJava(t, `
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

	result.CheckSnapshot("myexports", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "myexports_hostjavalib@current",
    sdk_member_name: "hostjavalib",
    device_supported: false,
    host_supported: true,
    jars: ["java/hostjavalib.jar"],
}

java_import {
    name: "hostjavalib",
    prefer: false,
    device_supported: false,
    host_supported: true,
    jars: ["java/hostjavalib.jar"],
}

java_import {
    name: "myexports_androidjavalib@current",
    sdk_member_name: "androidjavalib",
    jars: ["java/androidjavalib.jar"],
}

java_import {
    name: "androidjavalib",
    prefer: false,
    jars: ["java/androidjavalib.jar"],
}

java_import {
    name: "myexports_myjavalib@current",
    sdk_member_name: "myjavalib",
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

java_import {
    name: "myjavalib",
    prefer: false,
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

module_exports_snapshot {
    name: "myexports@current",
    host_supported: true,
    java_libs: ["myexports_myjavalib@current"],
    target: {
        android: {
            java_header_libs: ["myexports_androidjavalib@current"],
        },
        linux_glibc: {
            java_header_libs: ["myexports_hostjavalib@current"],
        },
    },
}
`),
		checkAllCopyRules(`
.intermediates/hostjavalib/linux_glibc_common/javac/hostjavalib.jar -> java/hostjavalib.jar
.intermediates/androidjavalib/android_common/turbine-combined/androidjavalib.jar -> java/androidjavalib.jar
.intermediates/myjavalib/android_common/javac/myjavalib.jar -> java/android/myjavalib.jar
.intermediates/myjavalib/linux_glibc_common/javac/myjavalib.jar -> java/linux_glibc/myjavalib.jar
`),
	)
}

func TestSnapshotWithJavaSdkLibrary(t *testing.T) {
	result := testSdkWithJava(t, `
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
		}
	`)

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "mysdk_myjavalib@current",
    sdk_member_name: "myjavalib",
    apex_available: ["//apex_available:anyapex"],
    shared_library: false,
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

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
    apex_available: ["//apex_available:anyapex"],
    shared_library: false,
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

sdk_snapshot {
    name: "mysdk@current",
    java_sdk_libs: ["mysdk_myjavalib@current"],
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib.stubs/android_common/javac/myjavalib.stubs.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
.intermediates/myjavalib.stubs.system/android_common/javac/myjavalib.stubs.system.jar -> sdk_library/system/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source.system/android_common/myjavalib.stubs.source.system_api.txt -> sdk_library/system/myjavalib.txt
.intermediates/myjavalib.stubs.source.system/android_common/myjavalib.stubs.source.system_removed.txt -> sdk_library/system/myjavalib-removed.txt
.intermediates/myjavalib.stubs.test/android_common/javac/myjavalib.stubs.test.jar -> sdk_library/test/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source.test/android_common/myjavalib.stubs.source.test_api.txt -> sdk_library/test/myjavalib.txt
.intermediates/myjavalib.stubs.source.test/android_common/myjavalib.stubs.source.test_removed.txt -> sdk_library/test/myjavalib-removed.txt
`),
		checkMergeZips(
			".intermediates/mysdk/common_os/tmp/sdk_library/public/myjavalib_stub_sources.zip",
			".intermediates/mysdk/common_os/tmp/sdk_library/system/myjavalib_stub_sources.zip",
			".intermediates/mysdk/common_os/tmp/sdk_library/test/myjavalib_stub_sources.zip"),
	)
}

func TestSnapshotWithJavaSdkLibrary_SdkVersion_None(t *testing.T) {
	result := testSdkWithJava(t, `
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

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "mysdk_myjavalib@current",
    sdk_member_name: "myjavalib",
    shared_library: true,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "none",
    },
}

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
    shared_library: true,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "none",
    },
}

sdk_snapshot {
    name: "mysdk@current",
    java_sdk_libs: ["mysdk_myjavalib@current"],
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib.stubs/android_common/javac/myjavalib.stubs.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
`),
		checkMergeZips(
			".intermediates/mysdk/common_os/tmp/sdk_library/public/myjavalib_stub_sources.zip",
		),
	)
}

func TestSnapshotWithJavaSdkLibrary_SdkVersion_ForScope(t *testing.T) {
	result := testSdkWithJava(t, `
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

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "mysdk_myjavalib@current",
    sdk_member_name: "myjavalib",
    shared_library: true,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "module_current",
    },
}

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
    shared_library: true,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "module_current",
    },
}

sdk_snapshot {
    name: "mysdk@current",
    java_sdk_libs: ["mysdk_myjavalib@current"],
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib.stubs/android_common/javac/myjavalib.stubs.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
`),
		checkMergeZips(
			".intermediates/mysdk/common_os/tmp/sdk_library/public/myjavalib_stub_sources.zip",
		),
	)
}

func TestSnapshotWithJavaSdkLibrary_ApiScopes(t *testing.T) {
	result := testSdkWithJava(t, `
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

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "mysdk_myjavalib@current",
    sdk_member_name: "myjavalib",
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

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
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

sdk_snapshot {
    name: "mysdk@current",
    java_sdk_libs: ["mysdk_myjavalib@current"],
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib.stubs/android_common/javac/myjavalib.stubs.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
.intermediates/myjavalib.stubs.system/android_common/javac/myjavalib.stubs.system.jar -> sdk_library/system/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source.system/android_common/myjavalib.stubs.source.system_api.txt -> sdk_library/system/myjavalib.txt
.intermediates/myjavalib.stubs.source.system/android_common/myjavalib.stubs.source.system_removed.txt -> sdk_library/system/myjavalib-removed.txt
`),
		checkMergeZips(
			".intermediates/mysdk/common_os/tmp/sdk_library/public/myjavalib_stub_sources.zip",
			".intermediates/mysdk/common_os/tmp/sdk_library/system/myjavalib_stub_sources.zip",
		),
	)
}

func TestSnapshotWithJavaSdkLibrary_ModuleLib(t *testing.T) {
	result := testSdkWithJava(t, `
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

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "mysdk_myjavalib@current",
    sdk_member_name: "myjavalib",
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

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
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

sdk_snapshot {
    name: "mysdk@current",
    java_sdk_libs: ["mysdk_myjavalib@current"],
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib.stubs/android_common/javac/myjavalib.stubs.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
.intermediates/myjavalib.stubs.system/android_common/javac/myjavalib.stubs.system.jar -> sdk_library/system/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source.system/android_common/myjavalib.stubs.source.system_api.txt -> sdk_library/system/myjavalib.txt
.intermediates/myjavalib.stubs.source.system/android_common/myjavalib.stubs.source.system_removed.txt -> sdk_library/system/myjavalib-removed.txt
.intermediates/myjavalib.stubs.module_lib/android_common/javac/myjavalib.stubs.module_lib.jar -> sdk_library/module-lib/myjavalib-stubs.jar
.intermediates/myjavalib.api.module_lib/android_common/myjavalib.api.module_lib_api.txt -> sdk_library/module-lib/myjavalib.txt
.intermediates/myjavalib.api.module_lib/android_common/myjavalib.api.module_lib_removed.txt -> sdk_library/module-lib/myjavalib-removed.txt
`),
		checkMergeZips(
			".intermediates/mysdk/common_os/tmp/sdk_library/public/myjavalib_stub_sources.zip",
			".intermediates/mysdk/common_os/tmp/sdk_library/system/myjavalib_stub_sources.zip",
			".intermediates/mysdk/common_os/tmp/sdk_library/module-lib/myjavalib_stub_sources.zip",
		),
	)
}

func TestSnapshotWithJavaSdkLibrary_SystemServer(t *testing.T) {
	result := testSdkWithJava(t, `
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

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "mysdk_myjavalib@current",
    sdk_member_name: "myjavalib",
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

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
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

sdk_snapshot {
    name: "mysdk@current",
    java_sdk_libs: ["mysdk_myjavalib@current"],
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib.stubs/android_common/javac/myjavalib.stubs.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source/android_common/myjavalib.stubs.source_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib.stubs.source/android_common/myjavalib.stubs.source_removed.txt -> sdk_library/public/myjavalib-removed.txt
.intermediates/myjavalib.stubs.system_server/android_common/javac/myjavalib.stubs.system_server.jar -> sdk_library/system-server/myjavalib-stubs.jar
.intermediates/myjavalib.stubs.source.system_server/android_common/myjavalib.stubs.source.system_server_api.txt -> sdk_library/system-server/myjavalib.txt
.intermediates/myjavalib.stubs.source.system_server/android_common/myjavalib.stubs.source.system_server_removed.txt -> sdk_library/system-server/myjavalib-removed.txt
`),
		checkMergeZips(
			".intermediates/mysdk/common_os/tmp/sdk_library/public/myjavalib_stub_sources.zip",
			".intermediates/mysdk/common_os/tmp/sdk_library/system-server/myjavalib_stub_sources.zip",
		),
	)
}

func TestSnapshotWithJavaSdkLibrary_NamingScheme(t *testing.T) {
	result := testSdkWithJava(t, `
		sdk {
			name: "mysdk",
			java_sdk_libs: ["myjavalib"],
		}

		java_sdk_library {
			name: "myjavalib",
			apex_available: ["//apex_available:anyapex"],
			srcs: ["Test.java"],
			sdk_version: "current",
			naming_scheme: "framework-modules",
			public: {
				enabled: true,
			},
		}
	`)

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_sdk_library_import {
    name: "mysdk_myjavalib@current",
    sdk_member_name: "myjavalib",
    apex_available: ["//apex_available:anyapex"],
    naming_scheme: "framework-modules",
    shared_library: true,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "current",
    },
}

java_sdk_library_import {
    name: "myjavalib",
    prefer: false,
    apex_available: ["//apex_available:anyapex"],
    naming_scheme: "framework-modules",
    shared_library: true,
    public: {
        jars: ["sdk_library/public/myjavalib-stubs.jar"],
        stub_srcs: ["sdk_library/public/myjavalib_stub_sources"],
        current_api: "sdk_library/public/myjavalib.txt",
        removed_api: "sdk_library/public/myjavalib-removed.txt",
        sdk_version: "current",
    },
}

sdk_snapshot {
    name: "mysdk@current",
    java_sdk_libs: ["mysdk_myjavalib@current"],
}
`),
		checkAllCopyRules(`
.intermediates/myjavalib-stubs-publicapi/android_common/javac/myjavalib-stubs-publicapi.jar -> sdk_library/public/myjavalib-stubs.jar
.intermediates/myjavalib-stubs-srcs-publicapi/android_common/myjavalib-stubs-srcs-publicapi_api.txt -> sdk_library/public/myjavalib.txt
.intermediates/myjavalib-stubs-srcs-publicapi/android_common/myjavalib-stubs-srcs-publicapi_removed.txt -> sdk_library/public/myjavalib-removed.txt
`),
		checkMergeZips(
			".intermediates/mysdk/common_os/tmp/sdk_library/public/myjavalib_stub_sources.zip",
		),
	)
}
