// Copyright 2020 The Android Open Source Project
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
	"sort"
	"testing"
)

func TestSourceProviderCollision(t *testing.T) {
	testRustError(t, "multiple source providers generate the same filename output: bindings.rs", `
		rust_binary {
			name: "source_collider",
			srcs: [
				"foo.rs",
				":libbindings1",
				":libbindings2",
			],
		}
		rust_bindgen {
			name: "libbindings1",
			source_stem: "bindings",
			crate_name: "bindings1",
			wrapper_src: "src/any.h",
		}
		rust_bindgen {
			name: "libbindings2",
			source_stem: "bindings",
			crate_name: "bindings2",
			wrapper_src: "src/any.h",
		}
	`)
}

func TestCompilationOutputFiles(t *testing.T) {
	ctx := testRust(t, `
		rust_library {
			name: "libfizz_buzz",
			crate_name:"fizz_buzz",
			srcs: ["lib.rs"],
		}
		rust_binary {
			name: "fizz_buzz",
			crate_name:"fizz_buzz",
			srcs: ["lib.rs"],
		}
		rust_ffi {
			name: "librust_ffi",
			crate_name: "rust_ffi",
			srcs: ["lib.rs"],
		}
	`)
	testcases := []struct {
		testName      string
		moduleName    string
		variant       string
		expectedFiles []string
	}{
		{
			testName:   "dylib",
			moduleName: "libfizz_buzz",
			variant:    "android_arm64_armv8-a_dylib",
			expectedFiles: []string{
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_dylib/libfizz_buzz.dylib.so",
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_dylib/libfizz_buzz.dylib.so.clippy",
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_dylib/unstripped/libfizz_buzz.dylib.so",
				"out/soong/target/product/test_device/system/lib64/libfizz_buzz.dylib.so",
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_dylib/meta_lic",
			},
		},
		{
			testName:   "rlib dylib-std",
			moduleName: "libfizz_buzz",
			variant:    "android_arm64_armv8-a_rlib_dylib-std",
			expectedFiles: []string{
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_rlib_dylib-std/libfizz_buzz.rlib",
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_rlib_dylib-std/libfizz_buzz.rlib.clippy",
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_rlib_dylib-std/meta_lic",
			},
		},
		{
			testName:   "rlib rlib-std",
			moduleName: "libfizz_buzz",
			variant:    "android_arm64_armv8-a_rlib_rlib-std",
			expectedFiles: []string{
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_rlib_rlib-std/libfizz_buzz.rlib",
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_rlib_rlib-std/libfizz_buzz.rlib.clippy",
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_rlib_rlib-std/meta_lic",
				"out/soong/.intermediates/libfizz_buzz/android_arm64_armv8-a_rlib_rlib-std/rustdoc.timestamp",
			},
		},
		{
			testName:   "rust_binary",
			moduleName: "fizz_buzz",
			variant:    "android_arm64_armv8-a",
			expectedFiles: []string{
				"out/soong/.intermediates/fizz_buzz/android_arm64_armv8-a/fizz_buzz",
				"out/soong/.intermediates/fizz_buzz/android_arm64_armv8-a/fizz_buzz.clippy",
				"out/soong/.intermediates/fizz_buzz/android_arm64_armv8-a/unstripped/fizz_buzz",
				"out/soong/target/product/test_device/system/bin/fizz_buzz",
				"out/soong/.intermediates/fizz_buzz/android_arm64_armv8-a/meta_lic",
			},
		},
		{
			testName:   "rust_ffi static",
			moduleName: "librust_ffi",
			variant:    "android_arm64_armv8-a_static",
			expectedFiles: []string{
				"out/soong/.intermediates/librust_ffi/android_arm64_armv8-a_static/librust_ffi.a",
				"out/soong/.intermediates/librust_ffi/android_arm64_armv8-a_static/librust_ffi.a.clippy",
				"out/soong/.intermediates/librust_ffi/android_arm64_armv8-a_static/meta_lic",
				"out/soong/.intermediates/librust_ffi/android_arm64_armv8-a_static/rustdoc.timestamp",
			},
		},
		{
			testName:   "rust_ffi shared",
			moduleName: "librust_ffi",
			variant:    "android_arm64_armv8-a_shared",
			expectedFiles: []string{
				"out/soong/.intermediates/librust_ffi/android_arm64_armv8-a_shared/librust_ffi.so",
				"out/soong/.intermediates/librust_ffi/android_arm64_armv8-a_shared/librust_ffi.so.clippy",
				"out/soong/.intermediates/librust_ffi/android_arm64_armv8-a_shared/unstripped/librust_ffi.so",
				"out/soong/.intermediates/librust_ffi/android_arm64_armv8-a_shared/unstripped/librust_ffi.so.toc",
				"out/soong/.intermediates/librust_ffi/android_arm64_armv8-a_shared/meta_lic",
				"out/soong/target/product/test_device/system/lib64/librust_ffi.so",
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.testName, func(t *testing.T) {
			modOutputs := ctx.ModuleForTests(tc.moduleName, tc.variant).AllOutputs()
			sort.Strings(tc.expectedFiles)
			sort.Strings(modOutputs)
			android.AssertStringPathsRelativeToTopEquals(
				t,
				"incorrect outputs from rust module",
				ctx.Config(),
				tc.expectedFiles,
				modOutputs,
			)
		})
	}
}
