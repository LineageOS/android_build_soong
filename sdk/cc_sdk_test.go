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

	"android/soong/cc"
)

func testSdkWithCc(t *testing.T, bp string) *testSdkResult {
	t.Helper()

	fs := map[string][]byte{
		"Test.cpp":                  nil,
		"include/Test.h":            nil,
		"arm64/include/Arm64Test.h": nil,
		"libfoo.so":                 nil,
		"aidl/foo/bar/Test.aidl":    nil,
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
			system_shared_libs: [],
			stl: "none",
		}
	`)

	armOutput := result.Module("sdkmember", "android_arm_armv7-a-neon_shared").(*cc.Module).OutputFile()
	arm64Output := result.Module("sdkmember", "android_arm64_armv8-a_shared").(*cc.Module).OutputFile()

	var inputs []string
	buildParams := result.Module("mysdk", "android_common").BuildParamsForTests()
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

	sdkMemberV1 := result.ModuleForTests("sdkmember_mysdk_1", "android_arm64_armv8-a_shared_myapex").Rule("toc").Output
	sdkMemberV2 := result.ModuleForTests("sdkmember_mysdk_2", "android_arm64_armv8-a_shared_myapex2").Rule("toc").Output

	cpplibForMyApex := result.ModuleForTests("mycpplib", "android_arm64_armv8-a_shared_myapex")
	cpplibForMyApex2 := result.ModuleForTests("mycpplib", "android_arm64_armv8-a_shared_myapex2")

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
			system_shared_libs: [],
			stl: "none",
		}

		cc_library_host_static {
			name: "sdkstatic",
			system_shared_libs: [],
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
			system_shared_libs: [],
			stl: "none",
		}

		cc_library_static {
			name: "sdkstatic",
			system_shared_libs: [],
			stl: "none",
		}

		cc_library {
			name: "sdkboth1",
			system_shared_libs: [],
			stl: "none",
		}

		cc_library {
			name: "sdkboth2",
			system_shared_libs: [],
			stl: "none",
		}
	`)
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
			system_shared_libs: [],
			stl: "none",
		}

		cc_library_shared {
			name: "mynativelib2",
			srcs: [
				"Test.cpp",
			],
			export_include_dirs: ["include"],
			system_shared_libs: [],
			stl: "none",
		}
	`)

	result.CheckSnapshot("mysdk", "android_common", "",
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
			system_shared_libs: [],
			stl: "none",
		}
	`)

	result.CheckSnapshot("mysdk", "android_common", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_shared {
    name: "mysdk_mynativelib@current",
    sdk_member_name: "mynativelib",
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
    stl: "none",
    system_shared_libs: [],
}

cc_prebuilt_library_shared {
    name: "mynativelib",
    prefer: false,
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
    stl: "none",
    system_shared_libs: [],
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
			system_shared_libs: [],
			stl: "none",
		}
	`)

	result.CheckSnapshot("mymodule_exports", "android_common", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_binary {
    name: "mymodule_exports_mynativebinary@current",
    sdk_member_name: "mynativebinary",
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
			export_include_dirs: ["include"],
			aidl: {
				export_aidl_headers: true,
			},
			system_shared_libs: [],
			stl: "none",
		}
	`)

	result.CheckSnapshot("mysdk", "android_common", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_shared {
    name: "mysdk_mynativelib@current",
    sdk_member_name: "mynativelib",
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
    stl: "none",
    system_shared_libs: [],
}

cc_prebuilt_library_shared {
    name: "mynativelib",
    prefer: false,
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
    stl: "none",
    system_shared_libs: [],
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
			system_shared_libs: [],
			stl: "none",
		}
	`)

	result.CheckSnapshot("mysdk", "linux_glibc_common", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_shared {
    name: "mysdk_mynativelib@current",
    sdk_member_name: "mynativelib",
    device_supported: false,
    host_supported: true,
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
    stl: "none",
    system_shared_libs: [],
}

cc_prebuilt_library_shared {
    name: "mynativelib",
    prefer: false,
    device_supported: false,
    host_supported: true,
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
    stl: "none",
    system_shared_libs: [],
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
			system_shared_libs: [],
			stl: "none",
		}
	`)

	result.CheckSnapshot("myexports", "android_common", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_static {
    name: "myexports_mynativelib@current",
    sdk_member_name: "mynativelib",
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
    stl: "none",
    system_shared_libs: [],
}

cc_prebuilt_library_static {
    name: "mynativelib",
    prefer: false,
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
    stl: "none",
    system_shared_libs: [],
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
			system_shared_libs: [],
			stl: "none",
		}
	`)

	result.CheckSnapshot("myexports", "linux_glibc_common", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

cc_prebuilt_library_static {
    name: "myexports_mynativelib@current",
    sdk_member_name: "mynativelib",
    device_supported: false,
    host_supported: true,
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
    stl: "none",
    system_shared_libs: [],
}

cc_prebuilt_library_static {
    name: "mynativelib",
    prefer: false,
    device_supported: false,
    host_supported: true,
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
    stl: "none",
    system_shared_libs: [],
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
