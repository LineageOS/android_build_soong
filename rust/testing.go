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

package rust

import (
	"android/soong/android"
	"android/soong/cc"
)

func GatherRequiredDepsForTest() string {
	bp := `
		rust_prebuilt_library {
				name: "libstd_x86_64-unknown-linux-gnu",
                                crate_name: "std",
                                rlib: {
                                    srcs: ["libstd.rlib"],
                                },
                                dylib: {
                                    srcs: ["libstd.so"],
                                },
				host_supported: true,
				sysroot: true,
		}
		rust_prebuilt_library {
				name: "libtest_x86_64-unknown-linux-gnu",
                                crate_name: "test",
                                rlib: {
                                    srcs: ["libtest.rlib"],
                                },
                                dylib: {
                                    srcs: ["libtest.so"],
                                },
				host_supported: true,
				sysroot: true,
		}
		rust_prebuilt_library {
				name: "libstd_i686-unknown-linux-gnu",
                                crate_name: "std",
                                rlib: {
                                    srcs: ["libstd.rlib"],
                                },
                                dylib: {
                                    srcs: ["libstd.so"],
                                },
				host_supported: true,
				sysroot: true,
		}
		rust_prebuilt_library {
				name: "libtest_i686-unknown-linux-gnu",
                                crate_name: "test",
                                rlib: {
                                    srcs: ["libtest.rlib"],
                                },
                                dylib: {
                                    srcs: ["libtest.so"],
                                },
				host_supported: true,
				sysroot: true,
		}
		rust_prebuilt_library {
				name: "libstd_x86_64-apple-darwin",
                                crate_name: "std",
                                rlib: {
                                    srcs: ["libstd.rlib"],
                                },
                                dylib: {
                                    srcs: ["libstd.so"],
                                },
				host_supported: true,
				sysroot: true,
		}
		rust_prebuilt_library {
				name: "libtest_x86_64-apple-darwin",
                                crate_name: "test",
                                rlib: {
                                    srcs: ["libtest.rlib"],
                                },
                                dylib: {
                                    srcs: ["libtest.so"],
                                },
				host_supported: true,
				sysroot: true,
		}
		//////////////////////////////
		// Device module requirements

		cc_library {
			name: "liblog",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			apex_available: ["//apex_available:platform", "//apex_available:anyapex"],
			min_sdk_version: "29",
		}
		cc_library {
			name: "libprotobuf-cpp-full",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			export_include_dirs: ["libprotobuf-cpp-full-includes"],
		}
		cc_library {
			name: "libclang_rt.asan-aarch64-android",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			export_include_dirs: ["libprotobuf-cpp-full-includes"],
		}
		rust_library {
			name: "libstd",
			crate_name: "std",
			srcs: ["foo.rs"],
			no_stdlibs: true,
			host_supported: true,
			vendor_available: true,
			vendor_ramdisk_available: true,
                        native_coverage: false,
			sysroot: true,
			apex_available: ["//apex_available:platform", "//apex_available:anyapex"],
			min_sdk_version: "29",
		}
		rust_library {
			name: "libtest",
			crate_name: "test",
			srcs: ["foo.rs"],
			no_stdlibs: true,
			host_supported: true,
			vendor_available: true,
			vendor_ramdisk_available: true,
                        native_coverage: false,
			sysroot: true,
			apex_available: ["//apex_available:platform", "//apex_available:anyapex"],
			min_sdk_version: "29",
		}
		rust_library {
			name: "libprotobuf",
			crate_name: "protobuf",
			srcs: ["foo.rs"],
			host_supported: true,
		}
		rust_library {
			name: "libgrpcio",
			crate_name: "grpcio",
			srcs: ["foo.rs"],
			host_supported: true,
		}
		rust_library {
			name: "libfutures",
			crate_name: "futures",
			srcs: ["foo.rs"],
			host_supported: true,
		}
		rust_library {
			name: "liblibfuzzer_sys",
			crate_name: "libfuzzer_sys",
			srcs:["foo.rs"],
			host_supported: true,
		}
`
	return bp
}

func RegisterRequiredBuildComponentsForTest(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("rust_binary", RustBinaryFactory)
	ctx.RegisterModuleType("rust_binary_host", RustBinaryHostFactory)
	ctx.RegisterModuleType("rust_bindgen", RustBindgenFactory)
	ctx.RegisterModuleType("rust_bindgen_host", RustBindgenHostFactory)
	ctx.RegisterModuleType("rust_test", RustTestFactory)
	ctx.RegisterModuleType("rust_test_host", RustTestHostFactory)
	ctx.RegisterModuleType("rust_library", RustLibraryFactory)
	ctx.RegisterModuleType("rust_library_dylib", RustLibraryDylibFactory)
	ctx.RegisterModuleType("rust_library_rlib", RustLibraryRlibFactory)
	ctx.RegisterModuleType("rust_library_host", RustLibraryHostFactory)
	ctx.RegisterModuleType("rust_library_host_dylib", RustLibraryDylibHostFactory)
	ctx.RegisterModuleType("rust_library_host_rlib", RustLibraryRlibHostFactory)
	ctx.RegisterModuleType("rust_fuzz", RustFuzzFactory)
	ctx.RegisterModuleType("rust_ffi", RustFFIFactory)
	ctx.RegisterModuleType("rust_ffi_shared", RustFFISharedFactory)
	ctx.RegisterModuleType("rust_ffi_static", RustFFIStaticFactory)
	ctx.RegisterModuleType("rust_ffi_host", RustFFIHostFactory)
	ctx.RegisterModuleType("rust_ffi_host_shared", RustFFISharedHostFactory)
	ctx.RegisterModuleType("rust_ffi_host_static", RustFFIStaticHostFactory)
	ctx.RegisterModuleType("rust_proc_macro", ProcMacroFactory)
	ctx.RegisterModuleType("rust_protobuf", RustProtobufFactory)
	ctx.RegisterModuleType("rust_protobuf_host", RustProtobufHostFactory)
	ctx.RegisterModuleType("rust_prebuilt_library", PrebuiltLibraryFactory)
	ctx.RegisterModuleType("rust_prebuilt_dylib", PrebuiltDylibFactory)
	ctx.RegisterModuleType("rust_prebuilt_rlib", PrebuiltRlibFactory)
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		// rust mutators
		ctx.BottomUp("rust_libraries", LibraryMutator).Parallel()
		ctx.BottomUp("rust_stdlinkage", LibstdMutator).Parallel()
		ctx.BottomUp("rust_begin", BeginMutator).Parallel()
	})
	ctx.RegisterSingletonType("rust_project_generator", rustProjectGeneratorSingleton)
}

func CreateTestContext(config android.Config) *android.TestContext {
	ctx := android.NewTestArchContext(config)
	android.RegisterPrebuiltMutators(ctx)
	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	cc.RegisterRequiredBuildComponentsForTest(ctx)
	RegisterRequiredBuildComponentsForTest(ctx)

	return ctx
}
