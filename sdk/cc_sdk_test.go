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
		"Test.cpp":               nil,
		"include/Test.h":         nil,
		"libfoo.so":              nil,
		"aidl/foo/bar/Test.aidl": nil,
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

	armOutput := result.Module("sdkmember", "android_arm_armv7-a-neon_core_shared").(*cc.Module).OutputFile()
	arm64Output := result.Module("sdkmember", "android_arm64_armv8-a_core_shared").(*cc.Module).OutputFile()

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
		}

		cc_prebuilt_library_shared {
			name: "sdkmember_mysdk_2",
			sdk_member_name: "sdkmember",
			srcs: ["libfoo.so"],
			system_shared_libs: [],
			stl: "none",
		}

		cc_library_shared {
			name: "mycpplib",
			srcs: ["Test.cpp"],
			shared_libs: ["sdkmember"],
			system_shared_libs: [],
			stl: "none",
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

	sdkMemberV1 := result.ModuleForTests("sdkmember_mysdk_1", "android_arm64_armv8-a_core_shared_myapex").Rule("toc").Output
	sdkMemberV2 := result.ModuleForTests("sdkmember_mysdk_2", "android_arm64_armv8-a_core_shared_myapex2").Rule("toc").Output

	cpplibForMyApex := result.ModuleForTests("mycpplib", "android_arm64_armv8-a_core_shared_myapex")
	cpplibForMyApex2 := result.ModuleForTests("mycpplib", "android_arm64_armv8-a_core_shared_myapex2")

	// Depending on the uses_sdks value, different libs are linked
	ensureListContains(t, pathsToStrings(cpplibForMyApex.Rule("ld").Implicits), sdkMemberV1.String())
	ensureListContains(t, pathsToStrings(cpplibForMyApex2.Rule("ld").Implicits), sdkMemberV2.String())
}

func TestSnapshotWithCcShared(t *testing.T) {
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
    arch: {
        arm64: {
            srcs: ["arm64/lib/mynativelib.so"],
            export_include_dirs: [
                "arm64/include/include",
                "arm64/include_gen/mynativelib",
            ],
        },
        arm: {
            srcs: ["arm/lib/mynativelib.so"],
            export_include_dirs: [
                "arm/include/include",
                "arm/include_gen/mynativelib",
            ],
        },
    },
    stl: "none",
    system_shared_libs: [],
}

cc_prebuilt_library_shared {
    name: "mynativelib",
    prefer: false,
    arch: {
        arm64: {
            srcs: ["arm64/lib/mynativelib.so"],
            export_include_dirs: [
                "arm64/include/include",
                "arm64/include_gen/mynativelib",
            ],
        },
        arm: {
            srcs: ["arm/lib/mynativelib.so"],
            export_include_dirs: [
                "arm/include/include",
                "arm/include_gen/mynativelib",
            ],
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
.intermediates/mynativelib/android_arm64_armv8-a_core_shared/mynativelib.so -> arm64/lib/mynativelib.so
include/Test.h -> arm64/include/include/Test.h
.intermediates/mynativelib/android_arm64_armv8-a_core_shared/gen/aidl/aidl/foo/bar/Test.h -> arm64/include_gen/mynativelib/aidl/foo/bar/Test.h
.intermediates/mynativelib/android_arm64_armv8-a_core_shared/gen/aidl/aidl/foo/bar/BnTest.h -> arm64/include_gen/mynativelib/aidl/foo/bar/BnTest.h
.intermediates/mynativelib/android_arm64_armv8-a_core_shared/gen/aidl/aidl/foo/bar/BpTest.h -> arm64/include_gen/mynativelib/aidl/foo/bar/BpTest.h
.intermediates/mynativelib/android_arm_armv7-a-neon_core_shared/mynativelib.so -> arm/lib/mynativelib.so
include/Test.h -> arm/include/include/Test.h
.intermediates/mynativelib/android_arm_armv7-a-neon_core_shared/gen/aidl/aidl/foo/bar/Test.h -> arm/include_gen/mynativelib/aidl/foo/bar/Test.h
.intermediates/mynativelib/android_arm_armv7-a-neon_core_shared/gen/aidl/aidl/foo/bar/BnTest.h -> arm/include_gen/mynativelib/aidl/foo/bar/BnTest.h
.intermediates/mynativelib/android_arm_armv7-a-neon_core_shared/gen/aidl/aidl/foo/bar/BpTest.h -> arm/include_gen/mynativelib/aidl/foo/bar/BpTest.h
`),
	)
}

func TestHostSnapshotWithCcShared(t *testing.T) {
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
    arch: {
        x86_64: {
            srcs: ["x86_64/lib/mynativelib.so"],
            export_include_dirs: [
                "x86_64/include/include",
                "x86_64/include_gen/mynativelib",
            ],
        },
        x86: {
            srcs: ["x86/lib/mynativelib.so"],
            export_include_dirs: [
                "x86/include/include",
                "x86/include_gen/mynativelib",
            ],
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
    arch: {
        x86_64: {
            srcs: ["x86_64/lib/mynativelib.so"],
            export_include_dirs: [
                "x86_64/include/include",
                "x86_64/include_gen/mynativelib",
            ],
        },
        x86: {
            srcs: ["x86/lib/mynativelib.so"],
            export_include_dirs: [
                "x86/include/include",
                "x86/include_gen/mynativelib",
            ],
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
.intermediates/mynativelib/linux_glibc_x86_64_shared/mynativelib.so -> x86_64/lib/mynativelib.so
include/Test.h -> x86_64/include/include/Test.h
.intermediates/mynativelib/linux_glibc_x86_64_shared/gen/aidl/aidl/foo/bar/Test.h -> x86_64/include_gen/mynativelib/aidl/foo/bar/Test.h
.intermediates/mynativelib/linux_glibc_x86_64_shared/gen/aidl/aidl/foo/bar/BnTest.h -> x86_64/include_gen/mynativelib/aidl/foo/bar/BnTest.h
.intermediates/mynativelib/linux_glibc_x86_64_shared/gen/aidl/aidl/foo/bar/BpTest.h -> x86_64/include_gen/mynativelib/aidl/foo/bar/BpTest.h
.intermediates/mynativelib/linux_glibc_x86_shared/mynativelib.so -> x86/lib/mynativelib.so
include/Test.h -> x86/include/include/Test.h
.intermediates/mynativelib/linux_glibc_x86_shared/gen/aidl/aidl/foo/bar/Test.h -> x86/include_gen/mynativelib/aidl/foo/bar/Test.h
.intermediates/mynativelib/linux_glibc_x86_shared/gen/aidl/aidl/foo/bar/BnTest.h -> x86/include_gen/mynativelib/aidl/foo/bar/BnTest.h
.intermediates/mynativelib/linux_glibc_x86_shared/gen/aidl/aidl/foo/bar/BpTest.h -> x86/include_gen/mynativelib/aidl/foo/bar/BpTest.h
`),
	)
}
