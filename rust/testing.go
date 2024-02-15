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
	"android/soong/bloaty"
	"android/soong/cc"
)

// Preparer that will define all cc module types and a limited set of mutators and singletons that
// make those module types usable.
var PrepareForTestWithRustBuildComponents = android.GroupFixturePreparers(
	android.FixtureRegisterWithContext(registerRequiredBuildComponentsForTest),
)

// The directory in which rust test default modules will be defined.
//
// Placing them here ensures that their location does not conflict with default test modules
// defined by other packages.
const rustDefaultsDir = "defaults/rust/"

// Preparer that will define default rust modules, e.g. standard prebuilt modules.
var PrepareForTestWithRustDefaultModules = android.GroupFixturePreparers(
	cc.PrepareForTestWithCcDefaultModules,
	bloaty.PrepareForTestWithBloatyDefaultModules,
	PrepareForTestWithRustBuildComponents,
	android.FixtureAddTextFile(rustDefaultsDir+"Android.bp", GatherRequiredDepsForTest()),
)

// Preparer that will allow use of all rust modules fully.
var PrepareForIntegrationTestWithRust = android.GroupFixturePreparers(
	PrepareForTestWithRustDefaultModules,
)

var PrepareForTestWithRustIncludeVndk = android.GroupFixturePreparers(
	PrepareForIntegrationTestWithRust,
	cc.PrepareForTestWithCcIncludeVndk,
)

func GatherRequiredDepsForTest() string {
	bp := `
		rust_prebuilt_library {
				name: "libstd",
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
		//////////////////////////////
		// Device module requirements

		cc_library {
			name: "liblog",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			apex_available: ["//apex_available:platform", "//apex_available:anyapex"],
			min_sdk_version: "29",
			vendor_available: true,
			host_supported: true,
			recovery_available: true,
			llndk: {
				symbol_file: "liblog.map.txt",
			},
		}
		cc_library {
			name: "libprotobuf-cpp-full",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			export_include_dirs: ["libprotobuf-cpp-full-includes"],
		}
		cc_library {
			name: "libclang_rt.asan",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
		}
		cc_library {
			name: "libclang_rt.hwasan_static",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
		}
		rust_library {
			name: "libstd",
			crate_name: "std",
			srcs: ["foo.rs"],
			no_stdlibs: true,
			product_available: true,
			host_supported: true,
			vendor_available: true,
			vendor_ramdisk_available: true,
			recovery_available: true,
			native_coverage: false,
			sysroot: true,
			apex_available: ["//apex_available:platform", "//apex_available:anyapex"],
			min_sdk_version: "29",
		}
		rust_library {
			name: "libtest",
			crate_name: "test",
			srcs: ["foo.rs"],
			host_supported: true,
			vendor_available: true,
			vendor_ramdisk_available: true,
			recovery_available: true,
			native_coverage: false,
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
		rust_library {
			name: "libcriterion",
			crate_name: "criterion",
			srcs:["foo.rs"],
			host_supported: true,
		}
`
	return bp
}

func registerRequiredBuildComponentsForTest(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("rust_benchmark", RustBenchmarkFactory)
	ctx.RegisterModuleType("rust_benchmark_host", RustBenchmarkHostFactory)
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
	ctx.RegisterModuleType("rust_fuzz_host", RustFuzzHostFactory)
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
	ctx.RegisterParallelSingletonType("rust_project_generator", rustProjectGeneratorSingleton)
	ctx.RegisterParallelSingletonType("kythe_rust_extract", kytheExtractRustFactory)
	ctx.PostDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("rust_sanitizers", rustSanitizerRuntimeMutator).Parallel()
	})
	registerRustSnapshotModules(ctx)
}
