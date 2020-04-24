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
		rust_prebuilt_dylib {
				name: "libstd_x86_64-unknown-linux-gnu",
				srcs: [""],
				host_supported: true,
		}
		rust_prebuilt_dylib {
				name: "libtest_x86_64-unknown-linux-gnu",
				srcs: [""],
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
		rust_library_dylib {
			name: "libstd",
			crate_name: "std",
			srcs: ["foo.rs"],
			no_stdlibs: true,
		}
		rust_library_rlib {
			name: "libstd.static",
			crate_name: "std",
			srcs: ["foo.rs"],
			no_stdlibs: true,
		}
		rust_library_dylib {
			name: "libtest",
			crate_name: "test",
			srcs: ["foo.rs"],
			no_stdlibs: true,
		}
		rust_library_rlib {
			name: "libtest.static",
			crate_name: "test",
			srcs: ["foo.rs"],
			no_stdlibs: true,
		}

` + cc.GatherRequiredDepsForTest(android.NoOsType)
	return bp
}

func CreateTestContext() *android.TestContext {
	ctx := android.NewTestArchContext()
	cc.RegisterRequiredBuildComponentsForTest(ctx)
	ctx.RegisterModuleType("rust_binary", RustBinaryFactory)
	ctx.RegisterModuleType("rust_binary_host", RustBinaryHostFactory)
	ctx.RegisterModuleType("rust_test", RustTestFactory)
	ctx.RegisterModuleType("rust_test_host", RustTestHostFactory)
	ctx.RegisterModuleType("rust_library", RustLibraryFactory)
	ctx.RegisterModuleType("rust_library_host", RustLibraryHostFactory)
	ctx.RegisterModuleType("rust_library_host_rlib", RustLibraryRlibHostFactory)
	ctx.RegisterModuleType("rust_library_host_dylib", RustLibraryDylibHostFactory)
	ctx.RegisterModuleType("rust_library_rlib", RustLibraryRlibFactory)
	ctx.RegisterModuleType("rust_library_dylib", RustLibraryDylibFactory)
	ctx.RegisterModuleType("rust_library_shared", RustLibrarySharedFactory)
	ctx.RegisterModuleType("rust_library_static", RustLibraryStaticFactory)
	ctx.RegisterModuleType("rust_library_host_shared", RustLibrarySharedHostFactory)
	ctx.RegisterModuleType("rust_library_host_static", RustLibraryStaticHostFactory)
	ctx.RegisterModuleType("rust_proc_macro", ProcMacroFactory)
	ctx.RegisterModuleType("rust_prebuilt_dylib", PrebuiltDylibFactory)
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		// rust mutators
		ctx.BottomUp("rust_libraries", LibraryMutator).Parallel()
		ctx.BottomUp("rust_unit_tests", TestPerSrcMutator).Parallel()
	})

	return ctx
}
