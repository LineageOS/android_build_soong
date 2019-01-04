// Copyright 2018 Google Inc. All rights reserved.
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

package bpf

import (
	"io/ioutil"
	"os"
	"testing"

	"android/soong/android"
	cc2 "android/soong/cc"
)

var buildDir string

func setUp() {
	var err error
	buildDir, err = ioutil.TempDir("", "genrule_test")
	if err != nil {
		panic(err)
	}
}

func tearDown() {
	os.RemoveAll(buildDir)
}

func TestMain(m *testing.M) {
	run := func() int {
		setUp()
		defer tearDown()

		return m.Run()
	}

	os.Exit(run())
}

func testContext(bp string) *android.TestContext {
	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("bpf", android.ModuleFactoryAdaptor(bpfFactory))
	ctx.RegisterModuleType("cc_test", android.ModuleFactoryAdaptor(cc2.TestFactory))
	ctx.RegisterModuleType("cc_library", android.ModuleFactoryAdaptor(cc2.LibraryFactory))
	ctx.RegisterModuleType("cc_library_static", android.ModuleFactoryAdaptor(cc2.LibraryStaticFactory))
	ctx.RegisterModuleType("cc_object", android.ModuleFactoryAdaptor(cc2.ObjectFactory))
	ctx.RegisterModuleType("toolchain_library", android.ModuleFactoryAdaptor(cc2.ToolchainLibraryFactory))
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("link", cc2.LinkageMutator).Parallel()
	})
	ctx.Register()

	// Add some modules that are required by the compiler and/or linker
	bp = bp + `
		toolchain_library {
			name: "libatomic",
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
			name: "libgcc",
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

		cc_library {
			name: "libm",
			no_libgcc: true,
			nocrt: true,
			system_shared_libs: [],
			recovery_available: true,
		}

		cc_library {
			name: "libdl",
			no_libgcc: true,
			nocrt: true,
			system_shared_libs: [],
			recovery_available: true,
		}

		cc_library {
			name: "libgtest",
			host_supported: true,
			vendor_available: true,
		}

		cc_library {
			name: "libgtest_main",
			host_supported: true,
			vendor_available: true,
		}

		cc_object {
			name: "crtbegin_dynamic",
			recovery_available: true,
			vendor_available: true,
		}

		cc_object {
			name: "crtend_android",
			recovery_available: true,
			vendor_available: true,
		}

		cc_object {
			name: "crtbegin_so",
			recovery_available: true,
			vendor_available: true,
		}

		cc_object {
			name: "crtend_so",
			recovery_available: true,
			vendor_available: true,
		}
	`
	mockFS := map[string][]byte{
		"Android.bp":  []byte(bp),
		"bpf.c":       nil,
		"BpfTest.cpp": nil,
	}

	ctx.MockFileSystem(mockFS)

	return ctx
}

func TestBpfDataDependency(t *testing.T) {
	config := android.TestArchConfig(buildDir, nil)
	bp := `
		bpf {
			name: "bpf.o",
			srcs: ["bpf.c"],
		}

		cc_test {
			name: "vts_test_binary_bpf_module",
			srcs: ["BpfTest.cpp"],
			data: [":bpf.o"],
		}
	`

	ctx := testContext(bp)
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	if errs == nil {
		_, errs = ctx.PrepareBuildActions(config)
	}
	if errs != nil {
		t.Fatal(errs)
	}

	// We only verify the above BP configuration is processed successfully since the data property
	// value is not available for testing from this package.
	// TODO(jungjw): Add a check for data or move this test to the cc package.
}
