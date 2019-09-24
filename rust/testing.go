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
				name: "libarena_x86_64-unknown-linux-gnu",
				srcs: [""],
				host_supported: true,
		}
		rust_prebuilt_dylib {
				name: "libfmt_macros_x86_64-unknown-linux-gnu",
				srcs: [""],
				host_supported: true,
		}
		rust_prebuilt_dylib {
				name: "libgraphviz_x86_64-unknown-linux-gnu",
				srcs: [""],
				host_supported: true,
		}
		rust_prebuilt_dylib {
				name: "libserialize_x86_64-unknown-linux-gnu",
				srcs: [""],
				host_supported: true,
		}
		rust_prebuilt_dylib {
				name: "libstd_x86_64-unknown-linux-gnu",
				srcs: [""],
				host_supported: true,
		}
		rust_prebuilt_dylib {
				name: "libsyntax_x86_64-unknown-linux-gnu",
				srcs: [""],
				host_supported: true,
		}
		rust_prebuilt_dylib {
				name: "libsyntax_ext_x86_64-unknown-linux-gnu",
				srcs: [""],
				host_supported: true,
		}
		rust_prebuilt_dylib {
				name: "libsyntax_pos_x86_64-unknown-linux-gnu",
				srcs: [""],
				host_supported: true,
		}
		rust_prebuilt_dylib {
				name: "libterm_x86_64-unknown-linux-gnu",
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

		toolchain_library {
			name: "libgcc",
			no_libcrt: true,
			nocrt: true,
			src: "",
			system_shared_libs: [],
		}
		cc_library {
			name: "libc",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
		}
		cc_library {
			name: "libm",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
		}
		cc_library {
			name: "libdl",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
		}
		cc_object {
			name: "crtbegin_dynamic",
		}

		cc_object {
			name: "crtend_android",
		}
		cc_library {
			name: "liblog",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
		}

		//////////////////////////////
		// cc module requirements

		toolchain_library {
			name: "libatomic",
			src: "",
		}
		toolchain_library {
			name: "libclang_rt.builtins-aarch64-android",
			src: "",
		}
		toolchain_library {
			name: "libgcc_stripped",
			src: "",
		}
		cc_library {
			name: "libc++_static",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
		}
		cc_library {
			name: "libc++demangle",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			host_supported: false,
		}
		cc_library {
			name: "libc++",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
		}
		cc_library {
			name: "libunwind_llvm",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
		}
		`
	return bp
}

func CreateTestContext(bp string) *android.TestContext {
	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("cc_library", android.ModuleFactoryAdaptor(cc.LibraryFactory))
	ctx.RegisterModuleType("cc_object", android.ModuleFactoryAdaptor(cc.ObjectFactory))
	ctx.RegisterModuleType("rust_binary", android.ModuleFactoryAdaptor(RustBinaryFactory))
	ctx.RegisterModuleType("rust_binary_host", android.ModuleFactoryAdaptor(RustBinaryHostFactory))
	ctx.RegisterModuleType("rust_library", android.ModuleFactoryAdaptor(RustLibraryFactory))
	ctx.RegisterModuleType("rust_library_host", android.ModuleFactoryAdaptor(RustLibraryHostFactory))
	ctx.RegisterModuleType("rust_library_host_rlib", android.ModuleFactoryAdaptor(RustLibraryRlibHostFactory))
	ctx.RegisterModuleType("rust_library_host_dylib", android.ModuleFactoryAdaptor(RustLibraryDylibHostFactory))
	ctx.RegisterModuleType("rust_library_rlib", android.ModuleFactoryAdaptor(RustLibraryRlibFactory))
	ctx.RegisterModuleType("rust_library_dylib", android.ModuleFactoryAdaptor(RustLibraryDylibFactory))
	ctx.RegisterModuleType("rust_proc_macro", android.ModuleFactoryAdaptor(ProcMacroFactory))
	ctx.RegisterModuleType("rust_prebuilt_dylib", android.ModuleFactoryAdaptor(PrebuiltDylibFactory))
	ctx.RegisterModuleType("toolchain_library", android.ModuleFactoryAdaptor(cc.ToolchainLibraryFactory))
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("rust_libraries", LibraryMutator).Parallel()

		ctx.BottomUp("image", cc.ImageMutator).Parallel()
		ctx.BottomUp("link", cc.LinkageMutator).Parallel()
		ctx.BottomUp("version", cc.VersionMutator).Parallel()
		ctx.BottomUp("begin", cc.BeginMutator).Parallel()
	})

	bp = bp + GatherRequiredDepsForTest()

	mockFS := map[string][]byte{
		"Android.bp": []byte(bp),
		"foo.rs":     nil,
		"src/bar.rs": nil,
		"liby.so":    nil,
		"libz.so":    nil,
	}

	ctx.MockFileSystem(mockFS)

	return ctx
}
