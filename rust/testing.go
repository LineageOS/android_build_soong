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
	"android/soong/genrule"
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
		}
		//////////////////////////////
		// Device module requirements

		cc_library {
			name: "liblog",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
		}
		rust_library {
			name: "libstd",
			crate_name: "std",
			srcs: ["foo.rs"],
			no_stdlibs: true,
			host_supported: true,
                        native_coverage: false,
		}
		rust_library {
			name: "libtest",
			crate_name: "test",
			srcs: ["foo.rs"],
			no_stdlibs: true,
			host_supported: true,
                        native_coverage: false,
		}

` + cc.GatherRequiredDepsForTest(android.NoOsType)
	return bp
}

func CreateTestContext() *android.TestContext {
	ctx := android.NewTestArchContext()
	android.RegisterPrebuiltMutators(ctx)
	cc.RegisterRequiredBuildComponentsForTest(ctx)
	ctx.RegisterModuleType("genrule", genrule.GenRuleFactory)
	ctx.RegisterModuleType("rust_binary", RustBinaryFactory)
	ctx.RegisterModuleType("rust_binary_host", RustBinaryHostFactory)
	ctx.RegisterModuleType("rust_bindgen", RustBindgenFactory)
	ctx.RegisterModuleType("rust_test", RustTestFactory)
	ctx.RegisterModuleType("rust_test_host", RustTestHostFactory)
	ctx.RegisterModuleType("rust_library", RustLibraryFactory)
	ctx.RegisterModuleType("rust_library_dylib", RustLibraryDylibFactory)
	ctx.RegisterModuleType("rust_library_rlib", RustLibraryRlibFactory)
	ctx.RegisterModuleType("rust_library_host", RustLibraryHostFactory)
	ctx.RegisterModuleType("rust_library_host_dylib", RustLibraryDylibHostFactory)
	ctx.RegisterModuleType("rust_library_host_rlib", RustLibraryRlibHostFactory)
	ctx.RegisterModuleType("rust_ffi", RustFFIFactory)
	ctx.RegisterModuleType("rust_ffi_shared", RustFFISharedFactory)
	ctx.RegisterModuleType("rust_ffi_static", RustFFIStaticFactory)
	ctx.RegisterModuleType("rust_ffi_host", RustFFIHostFactory)
	ctx.RegisterModuleType("rust_ffi_host_shared", RustFFISharedHostFactory)
	ctx.RegisterModuleType("rust_ffi_host_static", RustFFIStaticHostFactory)
	ctx.RegisterModuleType("rust_proc_macro", ProcMacroFactory)
	ctx.RegisterModuleType("rust_prebuilt_library", PrebuiltLibraryFactory)
	ctx.RegisterModuleType("rust_prebuilt_dylib", PrebuiltDylibFactory)
	ctx.RegisterModuleType("rust_prebuilt_rlib", PrebuiltRlibFactory)
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		// rust mutators
		ctx.BottomUp("rust_libraries", LibraryMutator).Parallel()
		ctx.BottomUp("rust_begin", BeginMutator).Parallel()
	})
	ctx.RegisterSingletonType("rust_project_generator", rustProjectGeneratorSingleton)

	return ctx
}
