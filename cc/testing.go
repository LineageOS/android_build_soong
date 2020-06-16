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

package cc

import (
	"android/soong/android"
)

func RegisterRequiredBuildComponentsForTest(ctx android.RegistrationContext) {
	RegisterPrebuiltBuildComponents(ctx)
	android.RegisterPrebuiltMutators(ctx)

	RegisterCCBuildComponents(ctx)
	RegisterBinaryBuildComponents(ctx)
	RegisterLibraryBuildComponents(ctx)
	RegisterLibraryHeadersBuildComponents(ctx)

	ctx.RegisterModuleType("toolchain_library", ToolchainLibraryFactory)
	ctx.RegisterModuleType("llndk_library", LlndkLibraryFactory)
	ctx.RegisterModuleType("cc_object", ObjectFactory)
	ctx.RegisterModuleType("ndk_prebuilt_shared_stl", NdkPrebuiltSharedStlFactory)
	ctx.RegisterModuleType("ndk_prebuilt_object", NdkPrebuiltObjectFactory)
}

func GatherRequiredDepsForTest(oses ...android.OsType) string {
	ret := `
		toolchain_library {
			name: "libatomic",
			vendor_available: true,
			recovery_available: true,
			native_bridge_supported: true,
			src: "",
		}

		toolchain_library {
			name: "libcompiler_rt-extras",
			vendor_available: true,
			recovery_available: true,
			src: "",
		}

		toolchain_library {
			name: "libclang_rt.builtins-arm-android",
			vendor_available: true,
			recovery_available: true,
			native_bridge_supported: true,
			src: "",
		}

		toolchain_library {
			name: "libclang_rt.builtins-aarch64-android",
			vendor_available: true,
			recovery_available: true,
			native_bridge_supported: true,
			src: "",
		}

		cc_prebuilt_library_shared {
			name: "libclang_rt.hwasan-aarch64-android",
			nocrt: true,
			vendor_available: true,
			recovery_available: true,
			system_shared_libs: [],
			stl: "none",
			srcs: [""],
			check_elf_files: false,
			sanitize: {
				never: true,
			},
		}

		toolchain_library {
			name: "libclang_rt.builtins-i686-android",
			vendor_available: true,
			recovery_available: true,
			native_bridge_supported: true,
			src: "",
		}

		toolchain_library {
			name: "libclang_rt.builtins-x86_64-android",
			vendor_available: true,
			recovery_available: true,
			native_bridge_supported: true,
			src: "",
		}

		toolchain_library {
			name: "libclang_rt.fuzzer-arm-android",
			vendor_available: true,
			recovery_available: true,
			src: "",
		}

		toolchain_library {
			name: "libclang_rt.fuzzer-aarch64-android",
			vendor_available: true,
			recovery_available: true,
			src: "",
		}

		toolchain_library {
			name: "libclang_rt.fuzzer-i686-android",
			vendor_available: true,
			recovery_available: true,
			src: "",
		}

		toolchain_library {
			name: "libclang_rt.fuzzer-x86_64-android",
			vendor_available: true,
			recovery_available: true,
			src: "",
		}

		toolchain_library {
			name: "libclang_rt.fuzzer-x86_64",
			vendor_available: true,
			recovery_available: true,
			src: "",
		}

		// Needed for sanitizer
		cc_prebuilt_library_shared {
			name: "libclang_rt.ubsan_standalone-aarch64-android",
			vendor_available: true,
			recovery_available: true,
			system_shared_libs: [],
			srcs: [""],
		}

		toolchain_library {
			name: "libgcc",
			vendor_available: true,
			recovery_available: true,
			src: "",
		}

		toolchain_library {
			name: "libgcc_stripped",
			vendor_available: true,
			recovery_available: true,
			sdk_version: "current",
			src: "",
		}

		cc_library {
			name: "libc",
			no_libcrt: true,
			nocrt: true,
			stl: "none",
			system_shared_libs: [],
			recovery_available: true,
			stubs: {
				versions: ["27", "28", "29"],
			},
		}
		llndk_library {
			name: "libc",
			symbol_file: "",
			sdk_version: "current",
		}
		cc_library {
			name: "libm",
			no_libcrt: true,
			nocrt: true,
			stl: "none",
			system_shared_libs: [],
			recovery_available: true,
			stubs: {
				versions: ["27", "28", "29"],
			},
			apex_available: [
				"//apex_available:platform",
				"myapex"
			],
		}
		llndk_library {
			name: "libm",
			symbol_file: "",
			sdk_version: "current",
		}
		cc_library {
			name: "libdl",
			no_libcrt: true,
			nocrt: true,
			stl: "none",
			system_shared_libs: [],
			recovery_available: true,
			stubs: {
				versions: ["27", "28", "29"],
			},
			apex_available: [
				"//apex_available:platform",
				"myapex"
			],
		}
		llndk_library {
			name: "libdl",
			symbol_file: "",
			sdk_version: "current",
		}
		cc_library {
			name: "libft2",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			recovery_available: true,
		}
		llndk_library {
			name: "libft2",
			symbol_file: "",
			vendor_available: false,
			sdk_version: "current",
		}
		cc_library {
			name: "libc++_static",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			vendor_available: true,
			recovery_available: true,
			host_supported: true,
			apex_available: [
				"//apex_available:platform",
				"//apex_available:anyapex",
			],
		}
		cc_library {
			name: "libc++",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			vendor_available: true,
			recovery_available: true,
			host_supported: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			apex_available: [
				"//apex_available:platform",
				"myapex"
			],
		}
		cc_library {
			name: "libc++demangle",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			host_supported: false,
			vendor_available: true,
			recovery_available: true,
			apex_available: [
				"//apex_available:platform",
				"//apex_available:anyapex",
			],
		}
		cc_library {
			name: "libunwind_llvm",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			vendor_available: true,
			recovery_available: true,
		}

		cc_defaults {
			name: "crt_defaults",
			recovery_available: true,
			vendor_available: true,
			native_bridge_supported: true,
			stl: "none",
			apex_available: [
				"//apex_available:platform",
				"//apex_available:anyapex",
			],
		}

		cc_object {
			name: "crtbegin_so",
			defaults: ["crt_defaults"],
			recovery_available: true,
			vendor_available: true,
			native_bridge_supported: true,
			stl: "none",
		}

		cc_object {
			name: "crtbegin_dynamic",
			defaults: ["crt_defaults"],
			recovery_available: true,
			vendor_available: true,
			native_bridge_supported: true,
			stl: "none",
		}

		cc_object {
			name: "crtbegin_static",
			defaults: ["crt_defaults"],
			recovery_available: true,
			vendor_available: true,
			native_bridge_supported: true,
			stl: "none",
		}

		cc_object {
			name: "crtend_so",
			defaults: ["crt_defaults"],
			recovery_available: true,
			vendor_available: true,
			native_bridge_supported: true,
			stl: "none",
		}

		cc_object {
			name: "crtend_android",
			defaults: ["crt_defaults"],
			recovery_available: true,
			vendor_available: true,
			native_bridge_supported: true,
			stl: "none",
		}

		cc_library {
			name: "libprotobuf-cpp-lite",
		}

		cc_library {
			name: "ndk_libunwind",
			sdk_version: "current",
			stl: "none",
			system_shared_libs: [],
		}

		cc_library {
			name: "libc.ndk.current",
			sdk_version: "current",
			stl: "none",
			system_shared_libs: [],
		}

		cc_library {
			name: "libm.ndk.current",
			sdk_version: "current",
			stl: "none",
			system_shared_libs: [],
		}

		cc_library {
			name: "libdl.ndk.current",
			sdk_version: "current",
			stl: "none",
			system_shared_libs: [],
		}

		ndk_prebuilt_object {
			name: "ndk_crtbegin_so.27",
			sdk_version: "27",
		}

		ndk_prebuilt_object {
			name: "ndk_crtend_so.27",
			sdk_version: "27",
		}

		ndk_prebuilt_object {
			name: "ndk_crtbegin_dynamic.27",
			sdk_version: "27",
		}

		ndk_prebuilt_object {
			name: "ndk_crtend_android.27",
			sdk_version: "27",
		}

		ndk_prebuilt_shared_stl {
			name: "ndk_libc++_shared",
		}
	`

	for _, os := range oses {
		if os == android.Fuchsia {
			ret += `
		cc_library {
			name: "libbioniccompat",
			stl: "none",
		}
		cc_library {
			name: "libcompiler_rt",
			stl: "none",
		}
		`
		}
		if os == android.Windows {
			ret += `
		toolchain_library {
			name: "libwinpthread",
			host_supported: true,
			enabled: false,
			target: {
				windows: {
					enabled: true,
				},
			},
			src: "",
		}
		`
		}
	}
	return ret
}

func GatherRequiredFilesForTest(fs map[string][]byte) {
}

func TestConfig(buildDir string, os android.OsType, env map[string]string,
	bp string, fs map[string][]byte) android.Config {

	// add some modules that are required by the compiler and/or linker
	bp = bp + GatherRequiredDepsForTest(os)

	mockFS := map[string][]byte{}

	GatherRequiredFilesForTest(mockFS)

	for k, v := range fs {
		mockFS[k] = v
	}

	var config android.Config
	if os == android.Fuchsia {
		config = android.TestArchConfigFuchsia(buildDir, env, bp, mockFS)
	} else {
		config = android.TestArchConfig(buildDir, env, bp, mockFS)
	}

	return config
}

func CreateTestContext() *android.TestContext {
	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("cc_fuzz", FuzzFactory)
	ctx.RegisterModuleType("cc_test", TestFactory)
	ctx.RegisterModuleType("llndk_headers", llndkHeadersFactory)
	ctx.RegisterModuleType("ndk_library", NdkLibraryFactory)
	ctx.RegisterModuleType("vendor_public_library", vendorPublicLibraryFactory)
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	ctx.RegisterModuleType("vndk_prebuilt_shared", VndkPrebuiltSharedFactory)
	ctx.RegisterModuleType("vndk_libraries_txt", VndkLibrariesTxtFactory)
	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	RegisterRequiredBuildComponentsForTest(ctx)
	ctx.RegisterSingletonType("vndk-snapshot", VndkSnapshotSingleton)
	ctx.RegisterSingletonType("vendor-snapshot", VendorSnapshotSingleton)

	return ctx
}
