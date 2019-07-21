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

func GatherRequiredDepsForTest(os android.OsType) string {
	ret := `
		toolchain_library {
			name: "libatomic",
			vendor_available: true,
			recovery_available: true,
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
			src: "",
		}

		toolchain_library {
			name: "libclang_rt.builtins-aarch64-android",
			vendor_available: true,
			recovery_available: true,
			src: "",
		}

		toolchain_library {
			name: "libclang_rt.builtins-i686-android",
			vendor_available: true,
			recovery_available: true,
			src: "",
		}

		toolchain_library {
			name: "libclang_rt.builtins-x86_64-android",
			vendor_available: true,
			recovery_available: true,
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
			src: "",
		}

		cc_library {
			name: "libc",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			recovery_available: true,
		}
		llndk_library {
			name: "libc",
			symbol_file: "",
		}
		cc_library {
			name: "libm",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			recovery_available: true,
		}
		llndk_library {
			name: "libm",
			symbol_file: "",
		}
		cc_library {
			name: "libdl",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			recovery_available: true,
		}
		llndk_library {
			name: "libdl",
			symbol_file: "",
		}
		cc_library {
			name: "libc++_static",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			vendor_available: true,
			recovery_available: true,
		}
		cc_library {
			name: "libc++",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			vendor_available: true,
			recovery_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
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

		cc_object {
			name: "crtbegin_so",
			recovery_available: true,
			vendor_available: true,
		}

		cc_object {
			name: "crtbegin_dynamic",
			recovery_available: true,
			vendor_available: true,
		}

		cc_object {
			name: "crtbegin_static",
			recovery_available: true,
			vendor_available: true,
		}

		cc_object {
			name: "crtend_so",
			recovery_available: true,
			vendor_available: true,
		}

		cc_object {
			name: "crtend_android",
			recovery_available: true,
			vendor_available: true,
		}

		cc_library {
			name: "libprotobuf-cpp-lite",
		}
		`
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
	return ret
}

func CreateTestContext(bp string, fs map[string][]byte,
	os android.OsType) *android.TestContext {

	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("cc_binary", android.ModuleFactoryAdaptor(BinaryFactory))
	ctx.RegisterModuleType("cc_binary_host", android.ModuleFactoryAdaptor(binaryHostFactory))
	ctx.RegisterModuleType("cc_fuzz", android.ModuleFactoryAdaptor(FuzzFactory))
	ctx.RegisterModuleType("cc_library", android.ModuleFactoryAdaptor(LibraryFactory))
	ctx.RegisterModuleType("cc_library_shared", android.ModuleFactoryAdaptor(LibrarySharedFactory))
	ctx.RegisterModuleType("cc_library_static", android.ModuleFactoryAdaptor(LibraryStaticFactory))
	ctx.RegisterModuleType("cc_library_headers", android.ModuleFactoryAdaptor(LibraryHeaderFactory))
	ctx.RegisterModuleType("cc_test", android.ModuleFactoryAdaptor(TestFactory))
	ctx.RegisterModuleType("toolchain_library", android.ModuleFactoryAdaptor(ToolchainLibraryFactory))
	ctx.RegisterModuleType("llndk_library", android.ModuleFactoryAdaptor(LlndkLibraryFactory))
	ctx.RegisterModuleType("llndk_headers", android.ModuleFactoryAdaptor(llndkHeadersFactory))
	ctx.RegisterModuleType("vendor_public_library", android.ModuleFactoryAdaptor(vendorPublicLibraryFactory))
	ctx.RegisterModuleType("cc_object", android.ModuleFactoryAdaptor(ObjectFactory))
	ctx.RegisterModuleType("filegroup", android.ModuleFactoryAdaptor(android.FileGroupFactory))
	ctx.RegisterModuleType("vndk_prebuilt_shared", android.ModuleFactoryAdaptor(vndkPrebuiltSharedFactory))
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("image", ImageMutator).Parallel()
		ctx.BottomUp("link", LinkageMutator).Parallel()
		ctx.BottomUp("vndk", VndkMutator).Parallel()
		ctx.BottomUp("version", VersionMutator).Parallel()
		ctx.BottomUp("begin", BeginMutator).Parallel()
	})
	ctx.PostDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("double_loadable", checkDoubleLoadableLibraries).Parallel()
	})
	ctx.RegisterSingletonType("vndk-snapshot", android.SingletonFactoryAdaptor(VndkSnapshotSingleton))

	// add some modules that are required by the compiler and/or linker
	bp = bp + GatherRequiredDepsForTest(os)

	mockFS := map[string][]byte{
		"Android.bp":  []byte(bp),
		"foo.c":       nil,
		"bar.c":       nil,
		"a.proto":     nil,
		"b.aidl":      nil,
		"sub/c.aidl":  nil,
		"my_include":  nil,
		"foo.map.txt": nil,
		"liba.so":     nil,
	}

	for k, v := range fs {
		mockFS[k] = v
	}

	ctx.MockFileSystem(mockFS)

	return ctx
}
