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
			no_libgcc: true,
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
			no_libgcc: true,
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
			no_libgcc: true,
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
			no_libgcc: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			vendor_available: true,
			recovery_available: true,
		}
		cc_library {
			name: "libc++",
			no_libgcc: true,
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
			no_libgcc: true,
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
