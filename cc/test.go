// Copyright 2016 Google Inc. All rights reserved.
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
	"path/filepath"
	"strings"

	"github.com/google/blueprint"

	"android/soong"
	"android/soong/android"
)

type TestLinkerProperties struct {
	// if set, build against the gtest library. Defaults to true.
	Gtest bool

	// Create a separate binary for each source file.  Useful when there is
	// global state that can not be torn down and reset between each test suite.
	Test_per_src *bool
}

func init() {
	soong.RegisterModuleType("cc_test", testFactory)
	soong.RegisterModuleType("cc_test_library", testLibraryFactory)
	soong.RegisterModuleType("cc_benchmark", benchmarkFactory)
	soong.RegisterModuleType("cc_test_host", testHostFactory)
	soong.RegisterModuleType("cc_benchmark_host", benchmarkHostFactory)
}

// Module factory for tests
func testFactory() (blueprint.Module, []interface{}) {
	module := NewTest(android.HostAndDeviceSupported)
	return module.Init()
}

// Module factory for test libraries
func testLibraryFactory() (blueprint.Module, []interface{}) {
	module := NewTestLibrary(android.HostAndDeviceSupported)
	return module.Init()
}

// Module factory for benchmarks
func benchmarkFactory() (blueprint.Module, []interface{}) {
	module := NewBenchmark(android.HostAndDeviceSupported)
	return module.Init()
}

// Module factory for host tests
func testHostFactory() (blueprint.Module, []interface{}) {
	module := NewTest(android.HostSupported)
	return module.Init()
}

// Module factory for host benchmarks
func benchmarkHostFactory() (blueprint.Module, []interface{}) {
	module := NewBenchmark(android.HostSupported)
	return module.Init()
}

func testPerSrcMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok {
		if test, ok := m.linker.(*testBinaryLinker); ok {
			if Bool(test.testLinker.Properties.Test_per_src) {
				testNames := make([]string, len(m.compiler.(*baseCompiler).Properties.Srcs))
				for i, src := range m.compiler.(*baseCompiler).Properties.Srcs {
					testNames[i] = strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
				}
				tests := mctx.CreateLocalVariations(testNames...)
				for i, src := range m.compiler.(*baseCompiler).Properties.Srcs {
					tests[i].(*Module).compiler.(*baseCompiler).Properties.Srcs = []string{src}
					tests[i].(*Module).linker.(*testBinaryLinker).binaryLinker.Properties.Stem = testNames[i]
				}
			}
		}
	}
}

type testLinker struct {
	Properties TestLinkerProperties
}

func (test *testLinker) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	if !test.Properties.Gtest {
		return flags
	}

	flags.CFlags = append(flags.CFlags, "-DGTEST_HAS_STD_STRING")
	if ctx.Host() {
		flags.CFlags = append(flags.CFlags, "-O0", "-g")

		switch ctx.Os() {
		case android.Windows:
			flags.CFlags = append(flags.CFlags, "-DGTEST_OS_WINDOWS")
		case android.Linux:
			flags.CFlags = append(flags.CFlags, "-DGTEST_OS_LINUX")
			flags.LdFlags = append(flags.LdFlags, "-lpthread")
		case android.Darwin:
			flags.CFlags = append(flags.CFlags, "-DGTEST_OS_MAC")
			flags.LdFlags = append(flags.LdFlags, "-lpthread")
		}
	} else {
		flags.CFlags = append(flags.CFlags, "-DGTEST_OS_LINUX_ANDROID")
	}

	return flags
}

func (test *testLinker) linkerDeps(ctx BaseModuleContext, deps Deps) Deps {
	if test.Properties.Gtest {
		if ctx.sdk() && ctx.Device() {
			switch ctx.selectedStl() {
			case "ndk_libc++_shared", "ndk_libc++_static":
				deps.StaticLibs = append(deps.StaticLibs, "libgtest_main_ndk_libcxx", "libgtest_ndk_libcxx")
			case "ndk_libgnustl_static":
				deps.StaticLibs = append(deps.StaticLibs, "libgtest_main_ndk_gnustl", "libgtest_ndk_gnustl")
			default:
				deps.StaticLibs = append(deps.StaticLibs, "libgtest_main_ndk", "libgtest_ndk")
			}
		} else {
			deps.StaticLibs = append(deps.StaticLibs, "libgtest_main", "libgtest")
		}
	}
	return deps
}

type testBinaryLinker struct {
	testLinker
	binaryLinker
}

func (test *testBinaryLinker) linkerInit(ctx BaseModuleContext) {
	test.binaryLinker.linkerInit(ctx)
	runpath := "../../lib"
	if ctx.toolchain().Is64Bit() {
		runpath += "64"
	}
	test.dynamicProperties.RunPaths = append([]string{runpath}, test.dynamicProperties.RunPaths...)
}

func (test *testBinaryLinker) linkerProps() []interface{} {
	return append(test.binaryLinker.linkerProps(), &test.testLinker.Properties)
}

func (test *testBinaryLinker) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = test.binaryLinker.linkerFlags(ctx, flags)
	flags = test.testLinker.linkerFlags(ctx, flags)
	return flags
}

func (test *testBinaryLinker) linkerDeps(ctx BaseModuleContext, deps Deps) Deps {
	deps = test.testLinker.linkerDeps(ctx, deps)
	deps = test.binaryLinker.linkerDeps(ctx, deps)
	return deps
}

type testLibraryLinker struct {
	testLinker
	*libraryLinker
}

func (test *testLibraryLinker) linkerProps() []interface{} {
	return append(test.libraryLinker.linkerProps(), &test.testLinker.Properties)
}

func (test *testLibraryLinker) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = test.libraryLinker.linkerFlags(ctx, flags)
	flags = test.testLinker.linkerFlags(ctx, flags)
	return flags
}

func (test *testLibraryLinker) linkerDeps(ctx BaseModuleContext, deps Deps) Deps {
	deps = test.testLinker.linkerDeps(ctx, deps)
	deps = test.libraryLinker.linkerDeps(ctx, deps)
	return deps
}

type testInstaller struct {
	baseInstaller
}

func (installer *testInstaller) install(ctx ModuleContext, file android.Path) {
	installer.dir = filepath.Join(installer.dir, ctx.ModuleName())
	installer.dir64 = filepath.Join(installer.dir64, ctx.ModuleName())
	installer.baseInstaller.install(ctx, file)
}

func NewTest(hod android.HostOrDeviceSupported) *Module {
	module := newModule(hod, android.MultilibBoth)
	module.compiler = &baseCompiler{}
	linker := &testBinaryLinker{}
	linker.testLinker.Properties.Gtest = true
	module.linker = linker
	module.installer = &testInstaller{
		baseInstaller: baseInstaller{
			dir:   "nativetest",
			dir64: "nativetest64",
			data:  true,
		},
	}
	return module
}

func NewTestLibrary(hod android.HostOrDeviceSupported) *Module {
	module := NewLibrary(android.HostAndDeviceSupported, false, true)
	linker := &testLibraryLinker{
		libraryLinker: module.linker.(*libraryLinker),
	}
	linker.testLinker.Properties.Gtest = true
	module.linker = linker
	module.installer = &testInstaller{
		baseInstaller: baseInstaller{
			dir:   "nativetest",
			dir64: "nativetest64",
			data:  true,
		},
	}
	return module
}

type benchmarkLinker struct {
	testBinaryLinker
}

func (benchmark *benchmarkLinker) linkerDeps(ctx BaseModuleContext, deps Deps) Deps {
	deps = benchmark.testBinaryLinker.linkerDeps(ctx, deps)
	deps.StaticLibs = append(deps.StaticLibs, "libgoogle-benchmark")
	return deps
}

func NewBenchmark(hod android.HostOrDeviceSupported) *Module {
	module := newModule(hod, android.MultilibFirst)
	module.compiler = &baseCompiler{}
	module.linker = &benchmarkLinker{}
	module.installer = &testInstaller{
		baseInstaller: baseInstaller{
			dir:   "nativetest",
			dir64: "nativetest64",
			data:  true,
		},
	}
	return module
}
