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

	"android/soong/android"
	"android/soong/cc"
)

func testSdkWithCc(t *testing.T, bp string) *testSdkResult {
	t.Helper()

	fs := map[string][]byte{
		"Test.cpp":                      nil,
		"include/Test.h":                nil,
		"include-android/AndroidTest.h": nil,
		"include-host/HostTest.h":       nil,
		"arm64/include/Arm64Test.h":     nil,
		"libfoo.so":                     nil,
		"aidl/foo/bar/Test.aidl":        nil,
		"some/where/stubslib.map.txt":   nil,
	}
	return testSdkWithFs(t, bp, fs)
}

// Contains tests for SDK members provided by the cc package.

func TestSdkIsCompileMultilibBoth(t *testing.T) {
	result := testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			native_shared_libs: ["sdkmember"],
		}

		cc_library_shared {
			name: "sdkmember",
			srcs: ["Test.cpp"],
			stl: "none",
		}
	`)

	armOutput := result.Module("sdkmember", "android_arm_armv7-a-neon_shared").(*cc.Module).OutputFile()
	arm64Output := result.Module("sdkmember", "android_arm64_armv8-a_shared").(*cc.Module).OutputFile()

	var inputs []string
	buildParams := result.Module("mysdk", android.CommonOS.Name).BuildParamsForTests()
	for _, bp := range buildParams {
		if bp.Input != nil {
			inputs = append(inputs, bp.Input.String())
		}
	}

	// ensure that both 32/64 outputs are inputs of the sdk snapshot
	ensureListContains(t, inputs, armOutput.String())
	ensureListContains(t, inputs, arm64Output.String())
}

func TestBasicSdkWithCc(t *testing.T) {
	result := testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			native_shared_libs: ["sdkmember"],
		}

		cc_library_shared {
			name: "sdkmember",
			system_shared_libs: [],
		}

		sdk_snapshot {
			name: "mysdk@1",
			native_shared_libs: ["sdkmember_mysdk_1"],
		}

		sdk_snapshot {
			name: "mysdk@2",
			native_shared_libs: ["sdkmember_mysdk_2"],
		}

		cc_prebuilt_library_shared {
			name: "sdkmember",
			srcs: ["libfoo.so"],
			prefer: false,
			system_shared_libs: [],
			stl: "none",
		}

		cc_prebuilt_library_shared {
			name: "sdkmember_mysdk_1",
			sdk_member_name: "sdkmember",
			srcs: ["libfoo.so"],
			system_shared_libs: [],
			stl: "none",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		cc_prebuilt_library_shared {
			name: "sdkmember_mysdk_2",
			sdk_member_name: "sdkmember",
			srcs: ["libfoo.so"],
			system_shared_libs: [],
			stl: "none",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex2",
			],
		}

		cc_library_shared {
			name: "mycpplib",
			srcs: ["Test.cpp"],
			shared_libs: ["sdkmember"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex",
				"myapex2",
			],
		}

		apex {
			name: "myapex",
			native_shared_libs: ["mycpplib"],
			uses_sdks: ["mysdk@1"],
			key: "myapex.key",
			certificate: ":myapex.cert",
		}

		apex {
			name: "myapex2",
			native_shared_libs: ["mycpplib"],
			uses_sdks: ["mysdk@2"],
			key: "myapex.key",
			certificate: ":myapex.cert",
		}
	`)

	sdkMemberV1 := result.ModuleForTests("sdkmember_mysdk_1", "android_arm64_armv8-a_shared_apex10000_mysdk_1").Rule("toc").Output
	sdkMemberV2 := result.ModuleForTests("sdkmember_mysdk_2", "android_arm64_armv8-a_shared_apex10000_mysdk_2").Rule("toc").Output

	cpplibForMyApex := result.ModuleForTests("mycpplib", "android_arm64_armv8-a_shared_apex10000_mysdk_1")
	cpplibForMyApex2 := result.ModuleForTests("mycpplib", "android_arm64_armv8-a_shared_apex10000_mysdk_2")

	// Depending on the uses_sdks value, different libs are linked
	ensureListContains(t, pathsToStrings(cpplibForMyApex.Rule("ld").Implicits), sdkMemberV1.String())
	ensureListContains(t, pathsToStrings(cpplibForMyApex2.Rule("ld").Implicits), sdkMemberV2.String())
}

// Make sure the sdk can use host specific cc libraries static/shared and both.
func TestHostSdkWithCc(t *testing.T) {
	testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			device_supported: false,
			host_supported: true,
			native_shared_libs: ["sdkshared"],
			native_static_libs: ["sdkstatic"],
		}

		cc_library_host_shared {
			name: "sdkshared",
			stl: "none",
		}

		cc_library_host_static {
			name: "sdkstatic",
			stl: "none",
		}
	`)
}

// Make sure the sdk can use cc libraries static/shared and both.
func TestSdkWithCc(t *testing.T) {
	testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			native_shared_libs: ["sdkshared", "sdkboth1"],
			native_static_libs: ["sdkstatic", "sdkboth2"],
		}

		cc_library_shared {
			name: "sdkshared",
			stl: "none",
		}

		cc_library_static {
			name: "sdkstatic",
			stl: "none",
		}

		cc_library {
			name: "sdkboth1",
			stl: "none",
		}

		cc_library {
			name: "sdkboth2",
			stl: "none",
		}
	`)
}

func TestSnapshotWithObject(t *testing.T) {
	result := testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			native_objects: ["crtobj"],
		}

		cc_object {
			name: "crtobj",
			stl: "none",
		}
	`)

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_object {
    name: "mysdk_crtobj@current",
    sdk_member_name: "crtobj",
    stl: "none",
    arch: {
        arm64: {
            srcs: ["arm64/lib/crtobj.o"],
        },
        arm: {
            srcs: ["arm/lib/crtobj.o"],
        },
    },
}

cc_prebuilt_object {
    name: "crtobj",
    prefer: false,
    stl: "none",
    arch: {
        arm64: {
            srcs: ["arm64/lib/crtobj.o"],
        },
        arm: {
            srcs: ["arm/lib/crtobj.o"],
        },
    },
}

sdk_snapshot {
    name: "mysdk@current",
    native_objects: ["mysdk_crtobj@current"],
}
`),
		checkAllCopyRules(`
.intermediates/crtobj/android_arm64_armv8-a/crtobj.o -> arm64/lib/crtobj.o
.intermediates/crtobj/android_arm_armv7-a-neon/crtobj.o -> arm/lib/crtobj.o
`),
	)
}

func TestSnapshotWithCcDuplicateHeaders(t *testing.T) {
	result := testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			native_shared_libs: ["mynativelib1", "mynativelib2"],
		}

		cc_library_shared {
			name: "mynativelib1",
			srcs: [
				"Test.cpp",
			],
			export_include_dirs: ["include"],
			stl: "none",
		}

		cc_library_shared {
			name: "mynativelib2",
			srcs: [
				"Test.cpp",
			],
			export_include_dirs: ["include"],
			stl: "none",
		}
	`)

	result.CheckSnapshot("mysdk", "",
		checkAllCopyRules(`
include/Test.h -> include/include/Test.h
.intermediates/mynativelib1/android_arm64_armv8-a_shared/mynativelib1.so -> arm64/lib/mynativelib1.so
.intermediates/mynativelib1/android_arm_armv7-a-neon_shared/mynativelib1.so -> arm/lib/mynativelib1.so
.intermediates/mynativelib2/android_arm64_armv8-a_shared/mynativelib2.so -> arm64/lib/mynativelib2.so
.intermediates/mynativelib2/android_arm_armv7-a-neon_shared/mynativelib2.so -> arm/lib/mynativelib2.so
`),
	)
}

// Verify that when the shared library has some common and some arch specific properties that the generated
// snapshot is optimized properly.
func TestSnapshotWithCcSharedLibraryCommonProperties(t *testing.T) {
	result := testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			native_shared_libs: ["mynativelib"],
		}

		cc_library_shared {
			name: "mynativelib",
			srcs: [
				"Test.cpp",
				"aidl/foo/bar/Test.aidl",
			],
			export_include_dirs: ["include"],
			arch: {
				arm64: {
					export_system_include_dirs: ["arm64/include"],
				},
			},
			stl: "none",
		}
	`)

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_shared {
    name: "mysdk_mynativelib@current",
    sdk_member_name: "mynativelib",
    installable: false,
    stl: "none",
    export_include_dirs: ["include/include"],
    arch: {
        arm64: {
            srcs: ["arm64/lib/mynativelib.so"],
            export_system_include_dirs: ["arm64/include/arm64/include"],
        },
        arm: {
            srcs: ["arm/lib/mynativelib.so"],
        },
    },
}

cc_prebuilt_library_shared {
    name: "mynativelib",
    prefer: false,
    stl: "none",
    export_include_dirs: ["include/include"],
    arch: {
        arm64: {
            srcs: ["arm64/lib/mynativelib.so"],
            export_system_include_dirs: ["arm64/include/arm64/include"],
        },
        arm: {
            srcs: ["arm/lib/mynativelib.so"],
        },
    },
}

sdk_snapshot {
    name: "mysdk@current",
    native_shared_libs: ["mysdk_mynativelib@current"],
}
`),
		checkAllCopyRules(`
include/Test.h -> include/include/Test.h
.intermediates/mynativelib/android_arm64_armv8-a_shared/mynativelib.so -> arm64/lib/mynativelib.so
arm64/include/Arm64Test.h -> arm64/include/arm64/include/Arm64Test.h
.intermediates/mynativelib/android_arm_armv7-a-neon_shared/mynativelib.so -> arm/lib/mynativelib.so`),
	)
}

func TestSnapshotWithCcBinary(t *testing.T) {
	result := testSdkWithCc(t, `
		module_exports {
			name: "mymodule_exports",
			native_binaries: ["mynativebinary"],
		}

		cc_binary {
			name: "mynativebinary",
			srcs: [
				"Test.cpp",
			],
			compile_multilib: "both",
			stl: "none",
		}
	`)

	result.CheckSnapshot("mymodule_exports", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_binary {
    name: "mymodule_exports_mynativebinary@current",
    sdk_member_name: "mynativebinary",
    installable: false,
    compile_multilib: "both",
    arch: {
        arm64: {
            srcs: ["arm64/bin/mynativebinary"],
        },
        arm: {
            srcs: ["arm/bin/mynativebinary"],
        },
    },
}

cc_prebuilt_binary {
    name: "mynativebinary",
    prefer: false,
    compile_multilib: "both",
    arch: {
        arm64: {
            srcs: ["arm64/bin/mynativebinary"],
        },
        arm: {
            srcs: ["arm/bin/mynativebinary"],
        },
    },
}

module_exports_snapshot {
    name: "mymodule_exports@current",
    native_binaries: ["mymodule_exports_mynativebinary@current"],
}
`),
		checkAllCopyRules(`
.intermediates/mynativebinary/android_arm64_armv8-a/mynativebinary -> arm64/bin/mynativebinary
.intermediates/mynativebinary/android_arm_armv7-a-neon/mynativebinary -> arm/bin/mynativebinary
`),
	)
}

func TestMultipleHostOsTypesSnapshotWithCcBinary(t *testing.T) {
	// b/145598135 - Generating host snapshots for anything other than linux is not supported.
	SkipIfNotLinux(t)

	result := testSdkWithCc(t, `
		module_exports {
			name: "myexports",
			device_supported: false,
			host_supported: true,
			native_binaries: ["mynativebinary"],
			target: {
				windows: {
					enabled: true,
				},
			},
		}

		cc_binary {
			name: "mynativebinary",
			device_supported: false,
			host_supported: true,
			srcs: [
				"Test.cpp",
			],
			compile_multilib: "both",
			stl: "none",
			target: {
				windows: {
					enabled: true,
				},
			},
		}
	`)

	result.CheckSnapshot("myexports", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_binary {
    name: "myexports_mynativebinary@current",
    sdk_member_name: "mynativebinary",
    device_supported: false,
    host_supported: true,
    installable: false,
    target: {
        linux_glibc: {
            compile_multilib: "both",
        },
        linux_glibc_x86_64: {
            srcs: ["linux_glibc/x86_64/bin/mynativebinary"],
        },
        linux_glibc_x86: {
            srcs: ["linux_glibc/x86/bin/mynativebinary"],
        },
        windows: {
            compile_multilib: "64",
        },
        windows_x86_64: {
            srcs: ["windows/x86_64/bin/mynativebinary.exe"],
        },
    },
}

cc_prebuilt_binary {
    name: "mynativebinary",
    prefer: false,
    device_supported: false,
    host_supported: true,
    target: {
        linux_glibc: {
            compile_multilib: "both",
        },
        linux_glibc_x86_64: {
            srcs: ["linux_glibc/x86_64/bin/mynativebinary"],
        },
        linux_glibc_x86: {
            srcs: ["linux_glibc/x86/bin/mynativebinary"],
        },
        windows: {
            compile_multilib: "64",
        },
        windows_x86_64: {
            srcs: ["windows/x86_64/bin/mynativebinary.exe"],
        },
    },
}

module_exports_snapshot {
    name: "myexports@current",
    device_supported: false,
    host_supported: true,
    native_binaries: ["myexports_mynativebinary@current"],
    target: {
        windows: {
            compile_multilib: "64",
        },
    },
}
`),
		checkAllCopyRules(`
.intermediates/mynativebinary/linux_glibc_x86_64/mynativebinary -> linux_glibc/x86_64/bin/mynativebinary
.intermediates/mynativebinary/linux_glibc_x86/mynativebinary -> linux_glibc/x86/bin/mynativebinary
.intermediates/mynativebinary/windows_x86_64/mynativebinary.exe -> windows/x86_64/bin/mynativebinary.exe
`),
	)
}

func TestSnapshotWithCcSharedLibrary(t *testing.T) {
	result := testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			native_shared_libs: ["mynativelib"],
		}

		cc_library_shared {
			name: "mynativelib",
			srcs: [
				"Test.cpp",
				"aidl/foo/bar/Test.aidl",
			],
			apex_available: ["apex1", "apex2"],
			export_include_dirs: ["include"],
			aidl: {
				export_aidl_headers: true,
			},
			stl: "none",
		}
	`)

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_shared {
    name: "mysdk_mynativelib@current",
    sdk_member_name: "mynativelib",
    apex_available: [
        "apex1",
        "apex2",
    ],
    installable: false,
    stl: "none",
    export_include_dirs: ["include/include"],
    arch: {
        arm64: {
            srcs: ["arm64/lib/mynativelib.so"],
            export_include_dirs: ["arm64/include_gen/mynativelib"],
        },
        arm: {
            srcs: ["arm/lib/mynativelib.so"],
            export_include_dirs: ["arm/include_gen/mynativelib"],
        },
    },
}

cc_prebuilt_library_shared {
    name: "mynativelib",
    prefer: false,
    apex_available: [
        "apex1",
        "apex2",
    ],
    stl: "none",
    export_include_dirs: ["include/include"],
    arch: {
        arm64: {
            srcs: ["arm64/lib/mynativelib.so"],
            export_include_dirs: ["arm64/include_gen/mynativelib"],
        },
        arm: {
            srcs: ["arm/lib/mynativelib.so"],
            export_include_dirs: ["arm/include_gen/mynativelib"],
        },
    },
}

sdk_snapshot {
    name: "mysdk@current",
    native_shared_libs: ["mysdk_mynativelib@current"],
}
`),
		checkAllCopyRules(`
include/Test.h -> include/include/Test.h
.intermediates/mynativelib/android_arm64_armv8-a_shared/mynativelib.so -> arm64/lib/mynativelib.so
.intermediates/mynativelib/android_arm64_armv8-a_shared/gen/aidl/aidl/foo/bar/Test.h -> arm64/include_gen/mynativelib/aidl/foo/bar/Test.h
.intermediates/mynativelib/android_arm64_armv8-a_shared/gen/aidl/aidl/foo/bar/BnTest.h -> arm64/include_gen/mynativelib/aidl/foo/bar/BnTest.h
.intermediates/mynativelib/android_arm64_armv8-a_shared/gen/aidl/aidl/foo/bar/BpTest.h -> arm64/include_gen/mynativelib/aidl/foo/bar/BpTest.h
.intermediates/mynativelib/android_arm_armv7-a-neon_shared/mynativelib.so -> arm/lib/mynativelib.so
.intermediates/mynativelib/android_arm_armv7-a-neon_shared/gen/aidl/aidl/foo/bar/Test.h -> arm/include_gen/mynativelib/aidl/foo/bar/Test.h
.intermediates/mynativelib/android_arm_armv7-a-neon_shared/gen/aidl/aidl/foo/bar/BnTest.h -> arm/include_gen/mynativelib/aidl/foo/bar/BnTest.h
.intermediates/mynativelib/android_arm_armv7-a-neon_shared/gen/aidl/aidl/foo/bar/BpTest.h -> arm/include_gen/mynativelib/aidl/foo/bar/BpTest.h
`),
	)
}

func TestSnapshotWithCcSharedLibrarySharedLibs(t *testing.T) {
	result := testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			native_shared_libs: [
				"mynativelib",
				"myothernativelib",
				"mysystemnativelib",
			],
		}

		cc_library {
			name: "mysystemnativelib",
			srcs: [
				"Test.cpp",
			],
			stl: "none",
		}

		cc_library_shared {
			name: "myothernativelib",
			srcs: [
				"Test.cpp",
			],
			system_shared_libs: [
				// A reference to a library that is not an sdk member. Uses libm as that
				// is in the default set of modules available to this test and so is available
				// both here and also when the generated Android.bp file is tested in
				// CheckSnapshot(). This ensures that the system_shared_libs property correctly
				// handles references to modules that are not sdk members.
				"libm",
			],
			stl: "none",
		}

		cc_library {
			name: "mynativelib",
			srcs: [
				"Test.cpp",
			],
			shared_libs: [
				// A reference to another sdk member.
				"myothernativelib",
			],
			target: {
				android: {
					shared: {
						shared_libs: [
							// A reference to a library that is not an sdk member. The libc library
							// is used here to check that the shared_libs property is handled correctly
							// in a similar way to how libm is used to check system_shared_libs above.
							"libc",
						],
					},
				},
			},
			stl: "none",
		}
	`)

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_shared {
    name: "mysdk_mynativelib@current",
    sdk_member_name: "mynativelib",
    installable: false,
    stl: "none",
    shared_libs: [
        "mysdk_myothernativelib@current",
        "libc",
    ],
    arch: {
        arm64: {
            srcs: ["arm64/lib/mynativelib.so"],
        },
        arm: {
            srcs: ["arm/lib/mynativelib.so"],
        },
    },
}

cc_prebuilt_library_shared {
    name: "mynativelib",
    prefer: false,
    stl: "none",
    shared_libs: [
        "myothernativelib",
        "libc",
    ],
    arch: {
        arm64: {
            srcs: ["arm64/lib/mynativelib.so"],
        },
        arm: {
            srcs: ["arm/lib/mynativelib.so"],
        },
    },
}

cc_prebuilt_library_shared {
    name: "mysdk_myothernativelib@current",
    sdk_member_name: "myothernativelib",
    installable: false,
    stl: "none",
    system_shared_libs: ["libm"],
    arch: {
        arm64: {
            srcs: ["arm64/lib/myothernativelib.so"],
        },
        arm: {
            srcs: ["arm/lib/myothernativelib.so"],
        },
    },
}

cc_prebuilt_library_shared {
    name: "myothernativelib",
    prefer: false,
    stl: "none",
    system_shared_libs: ["libm"],
    arch: {
        arm64: {
            srcs: ["arm64/lib/myothernativelib.so"],
        },
        arm: {
            srcs: ["arm/lib/myothernativelib.so"],
        },
    },
}

cc_prebuilt_library_shared {
    name: "mysdk_mysystemnativelib@current",
    sdk_member_name: "mysystemnativelib",
    installable: false,
    stl: "none",
    arch: {
        arm64: {
            srcs: ["arm64/lib/mysystemnativelib.so"],
        },
        arm: {
            srcs: ["arm/lib/mysystemnativelib.so"],
        },
    },
}

cc_prebuilt_library_shared {
    name: "mysystemnativelib",
    prefer: false,
    stl: "none",
    arch: {
        arm64: {
            srcs: ["arm64/lib/mysystemnativelib.so"],
        },
        arm: {
            srcs: ["arm/lib/mysystemnativelib.so"],
        },
    },
}

sdk_snapshot {
    name: "mysdk@current",
    native_shared_libs: [
        "mysdk_mynativelib@current",
        "mysdk_myothernativelib@current",
        "mysdk_mysystemnativelib@current",
    ],
}
`),
		checkAllCopyRules(`
.intermediates/mynativelib/android_arm64_armv8-a_shared/mynativelib.so -> arm64/lib/mynativelib.so
.intermediates/mynativelib/android_arm_armv7-a-neon_shared/mynativelib.so -> arm/lib/mynativelib.so
.intermediates/myothernativelib/android_arm64_armv8-a_shared/myothernativelib.so -> arm64/lib/myothernativelib.so
.intermediates/myothernativelib/android_arm_armv7-a-neon_shared/myothernativelib.so -> arm/lib/myothernativelib.so
.intermediates/mysystemnativelib/android_arm64_armv8-a_shared/mysystemnativelib.so -> arm64/lib/mysystemnativelib.so
.intermediates/mysystemnativelib/android_arm_armv7-a-neon_shared/mysystemnativelib.so -> arm/lib/mysystemnativelib.so
`),
	)
}

func TestHostSnapshotWithCcSharedLibrary(t *testing.T) {
	// b/145598135 - Generating host snapshots for anything other than linux is not supported.
	SkipIfNotLinux(t)

	result := testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			device_supported: false,
			host_supported: true,
			native_shared_libs: ["mynativelib"],
		}

		cc_library_shared {
			name: "mynativelib",
			device_supported: false,
			host_supported: true,
			srcs: [
				"Test.cpp",
				"aidl/foo/bar/Test.aidl",
			],
			export_include_dirs: ["include"],
			aidl: {
				export_aidl_headers: true,
			},
			stl: "none",
			sdk_version: "minimum",
		}
	`)

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_shared {
    name: "mysdk_mynativelib@current",
    sdk_member_name: "mynativelib",
    device_supported: false,
    host_supported: true,
    installable: false,
    sdk_version: "minimum",
    stl: "none",
    export_include_dirs: ["include/include"],
    arch: {
        x86_64: {
            srcs: ["x86_64/lib/mynativelib.so"],
            export_include_dirs: ["x86_64/include_gen/mynativelib"],
        },
        x86: {
            srcs: ["x86/lib/mynativelib.so"],
            export_include_dirs: ["x86/include_gen/mynativelib"],
        },
    },
}

cc_prebuilt_library_shared {
    name: "mynativelib",
    prefer: false,
    device_supported: false,
    host_supported: true,
    sdk_version: "minimum",
    stl: "none",
    export_include_dirs: ["include/include"],
    arch: {
        x86_64: {
            srcs: ["x86_64/lib/mynativelib.so"],
            export_include_dirs: ["x86_64/include_gen/mynativelib"],
        },
        x86: {
            srcs: ["x86/lib/mynativelib.so"],
            export_include_dirs: ["x86/include_gen/mynativelib"],
        },
    },
}

sdk_snapshot {
    name: "mysdk@current",
    device_supported: false,
    host_supported: true,
    native_shared_libs: ["mysdk_mynativelib@current"],
}
`),
		checkAllCopyRules(`
include/Test.h -> include/include/Test.h
.intermediates/mynativelib/linux_glibc_x86_64_shared/mynativelib.so -> x86_64/lib/mynativelib.so
.intermediates/mynativelib/linux_glibc_x86_64_shared/gen/aidl/aidl/foo/bar/Test.h -> x86_64/include_gen/mynativelib/aidl/foo/bar/Test.h
.intermediates/mynativelib/linux_glibc_x86_64_shared/gen/aidl/aidl/foo/bar/BnTest.h -> x86_64/include_gen/mynativelib/aidl/foo/bar/BnTest.h
.intermediates/mynativelib/linux_glibc_x86_64_shared/gen/aidl/aidl/foo/bar/BpTest.h -> x86_64/include_gen/mynativelib/aidl/foo/bar/BpTest.h
.intermediates/mynativelib/linux_glibc_x86_shared/mynativelib.so -> x86/lib/mynativelib.so
.intermediates/mynativelib/linux_glibc_x86_shared/gen/aidl/aidl/foo/bar/Test.h -> x86/include_gen/mynativelib/aidl/foo/bar/Test.h
.intermediates/mynativelib/linux_glibc_x86_shared/gen/aidl/aidl/foo/bar/BnTest.h -> x86/include_gen/mynativelib/aidl/foo/bar/BnTest.h
.intermediates/mynativelib/linux_glibc_x86_shared/gen/aidl/aidl/foo/bar/BpTest.h -> x86/include_gen/mynativelib/aidl/foo/bar/BpTest.h
`),
	)
}

func TestMultipleHostOsTypesSnapshotWithCcSharedLibrary(t *testing.T) {
	// b/145598135 - Generating host snapshots for anything other than linux is not supported.
	SkipIfNotLinux(t)

	result := testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			device_supported: false,
			host_supported: true,
			native_shared_libs: ["mynativelib"],
			target: {
				windows: {
					enabled: true,
				},
			},
		}

		cc_library_shared {
			name: "mynativelib",
			device_supported: false,
			host_supported: true,
			srcs: [
				"Test.cpp",
			],
			stl: "none",
			target: {
				windows: {
					enabled: true,
				},
			},
		}
	`)

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_shared {
    name: "mysdk_mynativelib@current",
    sdk_member_name: "mynativelib",
    device_supported: false,
    host_supported: true,
    installable: false,
    stl: "none",
    target: {
        linux_glibc_x86_64: {
            srcs: ["linux_glibc/x86_64/lib/mynativelib.so"],
        },
        linux_glibc_x86: {
            srcs: ["linux_glibc/x86/lib/mynativelib.so"],
        },
        windows_x86_64: {
            srcs: ["windows/x86_64/lib/mynativelib.dll"],
        },
    },
}

cc_prebuilt_library_shared {
    name: "mynativelib",
    prefer: false,
    device_supported: false,
    host_supported: true,
    stl: "none",
    target: {
        linux_glibc_x86_64: {
            srcs: ["linux_glibc/x86_64/lib/mynativelib.so"],
        },
        linux_glibc_x86: {
            srcs: ["linux_glibc/x86/lib/mynativelib.so"],
        },
        windows_x86_64: {
            srcs: ["windows/x86_64/lib/mynativelib.dll"],
        },
    },
}

sdk_snapshot {
    name: "mysdk@current",
    device_supported: false,
    host_supported: true,
    native_shared_libs: ["mysdk_mynativelib@current"],
    target: {
        windows: {
            compile_multilib: "64",
        },
    },
}
`),
		checkAllCopyRules(`
.intermediates/mynativelib/linux_glibc_x86_64_shared/mynativelib.so -> linux_glibc/x86_64/lib/mynativelib.so
.intermediates/mynativelib/linux_glibc_x86_shared/mynativelib.so -> linux_glibc/x86/lib/mynativelib.so
.intermediates/mynativelib/windows_x86_64_shared/mynativelib.dll -> windows/x86_64/lib/mynativelib.dll
`),
	)
}

func TestSnapshotWithCcStaticLibrary(t *testing.T) {
	result := testSdkWithCc(t, `
		module_exports {
			name: "myexports",
			native_static_libs: ["mynativelib"],
		}

		cc_library_static {
			name: "mynativelib",
			srcs: [
				"Test.cpp",
				"aidl/foo/bar/Test.aidl",
			],
			export_include_dirs: ["include"],
			aidl: {
				export_aidl_headers: true,
			},
			stl: "none",
		}
	`)

	result.CheckSnapshot("myexports", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_static {
    name: "myexports_mynativelib@current",
    sdk_member_name: "mynativelib",
    installable: false,
    stl: "none",
    export_include_dirs: ["include/include"],
    arch: {
        arm64: {
            srcs: ["arm64/lib/mynativelib.a"],
            export_include_dirs: ["arm64/include_gen/mynativelib"],
        },
        arm: {
            srcs: ["arm/lib/mynativelib.a"],
            export_include_dirs: ["arm/include_gen/mynativelib"],
        },
    },
}

cc_prebuilt_library_static {
    name: "mynativelib",
    prefer: false,
    stl: "none",
    export_include_dirs: ["include/include"],
    arch: {
        arm64: {
            srcs: ["arm64/lib/mynativelib.a"],
            export_include_dirs: ["arm64/include_gen/mynativelib"],
        },
        arm: {
            srcs: ["arm/lib/mynativelib.a"],
            export_include_dirs: ["arm/include_gen/mynativelib"],
        },
    },
}

module_exports_snapshot {
    name: "myexports@current",
    native_static_libs: ["myexports_mynativelib@current"],
}
`),
		checkAllCopyRules(`
include/Test.h -> include/include/Test.h
.intermediates/mynativelib/android_arm64_armv8-a_static/mynativelib.a -> arm64/lib/mynativelib.a
.intermediates/mynativelib/android_arm64_armv8-a_static/gen/aidl/aidl/foo/bar/Test.h -> arm64/include_gen/mynativelib/aidl/foo/bar/Test.h
.intermediates/mynativelib/android_arm64_armv8-a_static/gen/aidl/aidl/foo/bar/BnTest.h -> arm64/include_gen/mynativelib/aidl/foo/bar/BnTest.h
.intermediates/mynativelib/android_arm64_armv8-a_static/gen/aidl/aidl/foo/bar/BpTest.h -> arm64/include_gen/mynativelib/aidl/foo/bar/BpTest.h
.intermediates/mynativelib/android_arm_armv7-a-neon_static/mynativelib.a -> arm/lib/mynativelib.a
.intermediates/mynativelib/android_arm_armv7-a-neon_static/gen/aidl/aidl/foo/bar/Test.h -> arm/include_gen/mynativelib/aidl/foo/bar/Test.h
.intermediates/mynativelib/android_arm_armv7-a-neon_static/gen/aidl/aidl/foo/bar/BnTest.h -> arm/include_gen/mynativelib/aidl/foo/bar/BnTest.h
.intermediates/mynativelib/android_arm_armv7-a-neon_static/gen/aidl/aidl/foo/bar/BpTest.h -> arm/include_gen/mynativelib/aidl/foo/bar/BpTest.h
`),
	)
}

func TestHostSnapshotWithCcStaticLibrary(t *testing.T) {
	// b/145598135 - Generating host snapshots for anything other than linux is not supported.
	SkipIfNotLinux(t)

	result := testSdkWithCc(t, `
		module_exports {
			name: "myexports",
			device_supported: false,
			host_supported: true,
			native_static_libs: ["mynativelib"],
		}

		cc_library_static {
			name: "mynativelib",
			device_supported: false,
			host_supported: true,
			srcs: [
				"Test.cpp",
				"aidl/foo/bar/Test.aidl",
			],
			export_include_dirs: ["include"],
			aidl: {
				export_aidl_headers: true,
			},
			stl: "none",
		}
	`)

	result.CheckSnapshot("myexports", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_static {
    name: "myexports_mynativelib@current",
    sdk_member_name: "mynativelib",
    device_supported: false,
    host_supported: true,
    installable: false,
    stl: "none",
    export_include_dirs: ["include/include"],
    arch: {
        x86_64: {
            srcs: ["x86_64/lib/mynativelib.a"],
            export_include_dirs: ["x86_64/include_gen/mynativelib"],
        },
        x86: {
            srcs: ["x86/lib/mynativelib.a"],
            export_include_dirs: ["x86/include_gen/mynativelib"],
        },
    },
}

cc_prebuilt_library_static {
    name: "mynativelib",
    prefer: false,
    device_supported: false,
    host_supported: true,
    stl: "none",
    export_include_dirs: ["include/include"],
    arch: {
        x86_64: {
            srcs: ["x86_64/lib/mynativelib.a"],
            export_include_dirs: ["x86_64/include_gen/mynativelib"],
        },
        x86: {
            srcs: ["x86/lib/mynativelib.a"],
            export_include_dirs: ["x86/include_gen/mynativelib"],
        },
    },
}

module_exports_snapshot {
    name: "myexports@current",
    device_supported: false,
    host_supported: true,
    native_static_libs: ["myexports_mynativelib@current"],
}
`),
		checkAllCopyRules(`
include/Test.h -> include/include/Test.h
.intermediates/mynativelib/linux_glibc_x86_64_static/mynativelib.a -> x86_64/lib/mynativelib.a
.intermediates/mynativelib/linux_glibc_x86_64_static/gen/aidl/aidl/foo/bar/Test.h -> x86_64/include_gen/mynativelib/aidl/foo/bar/Test.h
.intermediates/mynativelib/linux_glibc_x86_64_static/gen/aidl/aidl/foo/bar/BnTest.h -> x86_64/include_gen/mynativelib/aidl/foo/bar/BnTest.h
.intermediates/mynativelib/linux_glibc_x86_64_static/gen/aidl/aidl/foo/bar/BpTest.h -> x86_64/include_gen/mynativelib/aidl/foo/bar/BpTest.h
.intermediates/mynativelib/linux_glibc_x86_static/mynativelib.a -> x86/lib/mynativelib.a
.intermediates/mynativelib/linux_glibc_x86_static/gen/aidl/aidl/foo/bar/Test.h -> x86/include_gen/mynativelib/aidl/foo/bar/Test.h
.intermediates/mynativelib/linux_glibc_x86_static/gen/aidl/aidl/foo/bar/BnTest.h -> x86/include_gen/mynativelib/aidl/foo/bar/BnTest.h
.intermediates/mynativelib/linux_glibc_x86_static/gen/aidl/aidl/foo/bar/BpTest.h -> x86/include_gen/mynativelib/aidl/foo/bar/BpTest.h
`),
	)
}

func TestSnapshotWithCcLibrary(t *testing.T) {
	result := testSdkWithCc(t, `
		module_exports {
			name: "myexports",
			native_libs: ["mynativelib"],
		}

		cc_library {
			name: "mynativelib",
			srcs: [
				"Test.cpp",
			],
			export_include_dirs: ["include"],
			stl: "none",
		}
	`)

	result.CheckSnapshot("myexports", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library {
    name: "myexports_mynativelib@current",
    sdk_member_name: "mynativelib",
    installable: false,
    stl: "none",
    export_include_dirs: ["include/include"],
    arch: {
        arm64: {
            static: {
                srcs: ["arm64/lib/mynativelib.a"],
            },
            shared: {
                srcs: ["arm64/lib/mynativelib.so"],
            },
        },
        arm: {
            static: {
                srcs: ["arm/lib/mynativelib.a"],
            },
            shared: {
                srcs: ["arm/lib/mynativelib.so"],
            },
        },
    },
}

cc_prebuilt_library {
    name: "mynativelib",
    prefer: false,
    stl: "none",
    export_include_dirs: ["include/include"],
    arch: {
        arm64: {
            static: {
                srcs: ["arm64/lib/mynativelib.a"],
            },
            shared: {
                srcs: ["arm64/lib/mynativelib.so"],
            },
        },
        arm: {
            static: {
                srcs: ["arm/lib/mynativelib.a"],
            },
            shared: {
                srcs: ["arm/lib/mynativelib.so"],
            },
        },
    },
}

module_exports_snapshot {
    name: "myexports@current",
    native_libs: ["myexports_mynativelib@current"],
}
`),
		checkAllCopyRules(`
include/Test.h -> include/include/Test.h
.intermediates/mynativelib/android_arm64_armv8-a_static/mynativelib.a -> arm64/lib/mynativelib.a
.intermediates/mynativelib/android_arm64_armv8-a_shared/mynativelib.so -> arm64/lib/mynativelib.so
.intermediates/mynativelib/android_arm_armv7-a-neon_static/mynativelib.a -> arm/lib/mynativelib.a
.intermediates/mynativelib/android_arm_armv7-a-neon_shared/mynativelib.so -> arm/lib/mynativelib.so`),
	)
}

func TestHostSnapshotWithMultiLib64(t *testing.T) {
	// b/145598135 - Generating host snapshots for anything other than linux is not supported.
	SkipIfNotLinux(t)

	result := testSdkWithCc(t, `
		module_exports {
			name: "myexports",
			device_supported: false,
			host_supported: true,
			target: {
				host: {
					compile_multilib: "64",
				},
			},
			native_static_libs: ["mynativelib"],
		}

		cc_library_static {
			name: "mynativelib",
			device_supported: false,
			host_supported: true,
			srcs: [
				"Test.cpp",
				"aidl/foo/bar/Test.aidl",
			],
			export_include_dirs: ["include"],
			aidl: {
				export_aidl_headers: true,
			},
			stl: "none",
		}
	`)

	result.CheckSnapshot("myexports", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_static {
    name: "myexports_mynativelib@current",
    sdk_member_name: "mynativelib",
    device_supported: false,
    host_supported: true,
    installable: false,
    stl: "none",
    export_include_dirs: ["include/include"],
    arch: {
        x86_64: {
            srcs: ["x86_64/lib/mynativelib.a"],
            export_include_dirs: ["x86_64/include_gen/mynativelib"],
        },
    },
}

cc_prebuilt_library_static {
    name: "mynativelib",
    prefer: false,
    device_supported: false,
    host_supported: true,
    stl: "none",
    export_include_dirs: ["include/include"],
    arch: {
        x86_64: {
            srcs: ["x86_64/lib/mynativelib.a"],
            export_include_dirs: ["x86_64/include_gen/mynativelib"],
        },
    },
}

module_exports_snapshot {
    name: "myexports@current",
    device_supported: false,
    host_supported: true,
    native_static_libs: ["myexports_mynativelib@current"],
    target: {
        linux_glibc: {
            compile_multilib: "64",
        },
    },
}`),
		checkAllCopyRules(`
include/Test.h -> include/include/Test.h
.intermediates/mynativelib/linux_glibc_x86_64_static/mynativelib.a -> x86_64/lib/mynativelib.a
.intermediates/mynativelib/linux_glibc_x86_64_static/gen/aidl/aidl/foo/bar/Test.h -> x86_64/include_gen/mynativelib/aidl/foo/bar/Test.h
.intermediates/mynativelib/linux_glibc_x86_64_static/gen/aidl/aidl/foo/bar/BnTest.h -> x86_64/include_gen/mynativelib/aidl/foo/bar/BnTest.h
.intermediates/mynativelib/linux_glibc_x86_64_static/gen/aidl/aidl/foo/bar/BpTest.h -> x86_64/include_gen/mynativelib/aidl/foo/bar/BpTest.h
`),
	)
}

func TestSnapshotWithCcHeadersLibrary(t *testing.T) {
	result := testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			native_header_libs: ["mynativeheaders"],
		}

		cc_library_headers {
			name: "mynativeheaders",
			export_include_dirs: ["include"],
			stl: "none",
		}
	`)

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_headers {
    name: "mysdk_mynativeheaders@current",
    sdk_member_name: "mynativeheaders",
    stl: "none",
    export_include_dirs: ["include/include"],
}

cc_prebuilt_library_headers {
    name: "mynativeheaders",
    prefer: false,
    stl: "none",
    export_include_dirs: ["include/include"],
}

sdk_snapshot {
    name: "mysdk@current",
    native_header_libs: ["mysdk_mynativeheaders@current"],
}
`),
		checkAllCopyRules(`
include/Test.h -> include/include/Test.h
`),
	)
}

func TestHostSnapshotWithCcHeadersLibrary(t *testing.T) {
	// b/145598135 - Generating host snapshots for anything other than linux is not supported.
	SkipIfNotLinux(t)

	result := testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			device_supported: false,
			host_supported: true,
			native_header_libs: ["mynativeheaders"],
		}

		cc_library_headers {
			name: "mynativeheaders",
			device_supported: false,
			host_supported: true,
			export_include_dirs: ["include"],
			stl: "none",
		}
	`)

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_headers {
    name: "mysdk_mynativeheaders@current",
    sdk_member_name: "mynativeheaders",
    device_supported: false,
    host_supported: true,
    stl: "none",
    export_include_dirs: ["include/include"],
}

cc_prebuilt_library_headers {
    name: "mynativeheaders",
    prefer: false,
    device_supported: false,
    host_supported: true,
    stl: "none",
    export_include_dirs: ["include/include"],
}

sdk_snapshot {
    name: "mysdk@current",
    device_supported: false,
    host_supported: true,
    native_header_libs: ["mysdk_mynativeheaders@current"],
}
`),
		checkAllCopyRules(`
include/Test.h -> include/include/Test.h
`),
	)
}

func TestDeviceAndHostSnapshotWithCcHeadersLibrary(t *testing.T) {
	// b/145598135 - Generating host snapshots for anything other than linux is not supported.
	SkipIfNotLinux(t)

	result := testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			host_supported: true,
			native_header_libs: ["mynativeheaders"],
		}

		cc_library_headers {
			name: "mynativeheaders",
			host_supported: true,
			stl: "none",
			export_system_include_dirs: ["include"],
			target: {
				android: {
					export_include_dirs: ["include-android"],
				},
				host: {
					export_include_dirs: ["include-host"],
				},
			},
		}
	`)

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_headers {
    name: "mysdk_mynativeheaders@current",
    sdk_member_name: "mynativeheaders",
    host_supported: true,
    stl: "none",
    export_system_include_dirs: ["include/include"],
    target: {
        android: {
            export_include_dirs: ["include/include-android"],
        },
        linux_glibc: {
            export_include_dirs: ["include/include-host"],
        },
    },
}

cc_prebuilt_library_headers {
    name: "mynativeheaders",
    prefer: false,
    host_supported: true,
    stl: "none",
    export_system_include_dirs: ["include/include"],
    target: {
        android: {
            export_include_dirs: ["include/include-android"],
        },
        linux_glibc: {
            export_include_dirs: ["include/include-host"],
        },
    },
}

sdk_snapshot {
    name: "mysdk@current",
    host_supported: true,
    native_header_libs: ["mysdk_mynativeheaders@current"],
}
`),
		checkAllCopyRules(`
include/Test.h -> include/include/Test.h
include-android/AndroidTest.h -> include/include-android/AndroidTest.h
include-host/HostTest.h -> include/include-host/HostTest.h
`),
	)
}

func TestSystemSharedLibPropagation(t *testing.T) {
	// b/145598135 - Generating host snapshots for anything other than linux is not supported.
	SkipIfNotLinux(t)

	result := testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			native_shared_libs: ["sslnil", "sslempty", "sslnonempty"],
		}

		cc_library {
			name: "sslnil",
			host_supported: true,
		}

		cc_library {
			name: "sslempty",
			system_shared_libs: [],
		}

		cc_library {
			name: "sslnonempty",
			system_shared_libs: ["sslnil"],
		}
	`)

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_shared {
    name: "mysdk_sslnil@current",
    sdk_member_name: "sslnil",
    installable: false,
    arch: {
        arm64: {
            srcs: ["arm64/lib/sslnil.so"],
        },
        arm: {
            srcs: ["arm/lib/sslnil.so"],
        },
    },
}

cc_prebuilt_library_shared {
    name: "sslnil",
    prefer: false,
    arch: {
        arm64: {
            srcs: ["arm64/lib/sslnil.so"],
        },
        arm: {
            srcs: ["arm/lib/sslnil.so"],
        },
    },
}

cc_prebuilt_library_shared {
    name: "mysdk_sslempty@current",
    sdk_member_name: "sslempty",
    installable: false,
    system_shared_libs: [],
    arch: {
        arm64: {
            srcs: ["arm64/lib/sslempty.so"],
        },
        arm: {
            srcs: ["arm/lib/sslempty.so"],
        },
    },
}

cc_prebuilt_library_shared {
    name: "sslempty",
    prefer: false,
    system_shared_libs: [],
    arch: {
        arm64: {
            srcs: ["arm64/lib/sslempty.so"],
        },
        arm: {
            srcs: ["arm/lib/sslempty.so"],
        },
    },
}

cc_prebuilt_library_shared {
    name: "mysdk_sslnonempty@current",
    sdk_member_name: "sslnonempty",
    installable: false,
    system_shared_libs: ["mysdk_sslnil@current"],
    arch: {
        arm64: {
            srcs: ["arm64/lib/sslnonempty.so"],
        },
        arm: {
            srcs: ["arm/lib/sslnonempty.so"],
        },
    },
}

cc_prebuilt_library_shared {
    name: "sslnonempty",
    prefer: false,
    system_shared_libs: ["sslnil"],
    arch: {
        arm64: {
            srcs: ["arm64/lib/sslnonempty.so"],
        },
        arm: {
            srcs: ["arm/lib/sslnonempty.so"],
        },
    },
}

sdk_snapshot {
    name: "mysdk@current",
    native_shared_libs: [
        "mysdk_sslnil@current",
        "mysdk_sslempty@current",
        "mysdk_sslnonempty@current",
    ],
}
`))

	result = testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			host_supported: true,
			native_shared_libs: ["sslvariants"],
		}

		cc_library {
			name: "sslvariants",
			host_supported: true,
			target: {
				android: {
					system_shared_libs: [],
				},
			},
		}
	`)

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_shared {
    name: "mysdk_sslvariants@current",
    sdk_member_name: "sslvariants",
    host_supported: true,
    installable: false,
    target: {
        android: {
            system_shared_libs: [],
        },
        android_arm64: {
            srcs: ["android/arm64/lib/sslvariants.so"],
        },
        android_arm: {
            srcs: ["android/arm/lib/sslvariants.so"],
        },
        linux_glibc_x86_64: {
            srcs: ["linux_glibc/x86_64/lib/sslvariants.so"],
        },
        linux_glibc_x86: {
            srcs: ["linux_glibc/x86/lib/sslvariants.so"],
        },
    },
}

cc_prebuilt_library_shared {
    name: "sslvariants",
    prefer: false,
    host_supported: true,
    target: {
        android: {
            system_shared_libs: [],
        },
        android_arm64: {
            srcs: ["android/arm64/lib/sslvariants.so"],
        },
        android_arm: {
            srcs: ["android/arm/lib/sslvariants.so"],
        },
        linux_glibc_x86_64: {
            srcs: ["linux_glibc/x86_64/lib/sslvariants.so"],
        },
        linux_glibc_x86: {
            srcs: ["linux_glibc/x86/lib/sslvariants.so"],
        },
    },
}

sdk_snapshot {
    name: "mysdk@current",
    host_supported: true,
    native_shared_libs: ["mysdk_sslvariants@current"],
}
`))
}

func TestStubsLibrary(t *testing.T) {
	result := testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			native_shared_libs: ["stubslib"],
		}

		cc_library {
			name: "internaldep",
		}

		cc_library {
			name: "stubslib",
			shared_libs: ["internaldep"],
			stubs: {
				symbol_file: "some/where/stubslib.map.txt",
				versions: ["1", "2", "3"],
			},
		}
	`)

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_shared {
    name: "mysdk_stubslib@current",
    sdk_member_name: "stubslib",
    installable: false,
    stubs: {
        versions: ["3"],
    },
    arch: {
        arm64: {
            srcs: ["arm64/lib/stubslib.so"],
        },
        arm: {
            srcs: ["arm/lib/stubslib.so"],
        },
    },
}

cc_prebuilt_library_shared {
    name: "stubslib",
    prefer: false,
    stubs: {
        versions: ["3"],
    },
    arch: {
        arm64: {
            srcs: ["arm64/lib/stubslib.so"],
        },
        arm: {
            srcs: ["arm/lib/stubslib.so"],
        },
    },
}

sdk_snapshot {
    name: "mysdk@current",
    native_shared_libs: ["mysdk_stubslib@current"],
}
`))
}

func TestDeviceAndHostSnapshotWithStubsLibrary(t *testing.T) {
	// b/145598135 - Generating host snapshots for anything other than linux is not supported.
	SkipIfNotLinux(t)

	result := testSdkWithCc(t, `
		sdk {
			name: "mysdk",
			host_supported: true,
			native_shared_libs: ["stubslib"],
		}

		cc_library {
			name: "internaldep",
			host_supported: true,
		}

		cc_library {
			name: "stubslib",
			host_supported: true,
			shared_libs: ["internaldep"],
			stubs: {
				symbol_file: "some/where/stubslib.map.txt",
				versions: ["1", "2", "3"],
			},
		}
	`)

	result.CheckSnapshot("mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_shared {
    name: "mysdk_stubslib@current",
    sdk_member_name: "stubslib",
    host_supported: true,
    installable: false,
    stubs: {
        versions: ["3"],
    },
    target: {
        android_arm64: {
            srcs: ["android/arm64/lib/stubslib.so"],
        },
        android_arm: {
            srcs: ["android/arm/lib/stubslib.so"],
        },
        linux_glibc_x86_64: {
            srcs: ["linux_glibc/x86_64/lib/stubslib.so"],
        },
        linux_glibc_x86: {
            srcs: ["linux_glibc/x86/lib/stubslib.so"],
        },
    },
}

cc_prebuilt_library_shared {
    name: "stubslib",
    prefer: false,
    host_supported: true,
    stubs: {
        versions: ["3"],
    },
    target: {
        android_arm64: {
            srcs: ["android/arm64/lib/stubslib.so"],
        },
        android_arm: {
            srcs: ["android/arm/lib/stubslib.so"],
        },
        linux_glibc_x86_64: {
            srcs: ["linux_glibc/x86_64/lib/stubslib.so"],
        },
        linux_glibc_x86: {
            srcs: ["linux_glibc/x86/lib/stubslib.so"],
        },
    },
}

sdk_snapshot {
    name: "mysdk@current",
    host_supported: true,
    native_shared_libs: ["mysdk_stubslib@current"],
}
`))
}
