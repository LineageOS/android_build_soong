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

	"android/soong/android"
	"android/soong/tradefed"
)

type TestProperties struct {
	// if set, build against the gtest library. Defaults to true.
	Gtest *bool

	// if set, use the isolated gtest runner. Defaults to false.
	Isolated *bool
}

// Test option struct.
type TestOptions struct {
	// The UID that you want to run the test as on a device.
	Run_test_as *string
}

type TestBinaryProperties struct {
	// Create a separate binary for each source file.  Useful when there is
	// global state that can not be torn down and reset between each test suite.
	Test_per_src *bool

	// Disables the creation of a test-specific directory when used with
	// relative_install_path. Useful if several tests need to be in the same
	// directory, but test_per_src doesn't work.
	No_named_install_directory *bool

	// list of files or filegroup modules that provide data that should be installed alongside
	// the test
	Data []string `android:"path"`

	// list of compatibility suites (for example "cts", "vts") that the module should be
	// installed into.
	Test_suites []string `android:"arch_variant"`

	// the name of the test configuration (for example "AndroidTest.xml") that should be
	// installed with the module.
	Test_config *string `android:"path,arch_variant"`

	// the name of the test configuration template (for example "AndroidTestTemplate.xml") that
	// should be installed with the module.
	Test_config_template *string `android:"path,arch_variant"`

	// Test options.
	Test_options TestOptions

	// Add RootTargetPreparer to auto generated test config. This guarantees the test to run
	// with root permission.
	Require_root *bool
}

func init() {
	android.RegisterModuleType("cc_test", TestFactory)
	android.RegisterModuleType("cc_test_library", TestLibraryFactory)
	android.RegisterModuleType("cc_benchmark", BenchmarkFactory)
	android.RegisterModuleType("cc_test_host", TestHostFactory)
	android.RegisterModuleType("cc_benchmark_host", BenchmarkHostFactory)
}

// cc_test generates a test config file and an executable binary file to test
// specific functionality on a device. The executable binary gets an implicit
// static_libs dependency on libgtests unless the gtest flag is set to false.
func TestFactory() android.Module {
	module := NewTest(android.HostAndDeviceSupported)
	return module.Init()
}

// cc_test_library creates an archive of files (i.e. .o files) which is later
// referenced by another module (such as cc_test, cc_defaults or cc_test_library)
// for archiving or linking.
func TestLibraryFactory() android.Module {
	module := NewTestLibrary(android.HostAndDeviceSupported)
	return module.Init()
}

// cc_benchmark compiles an executable binary that performs benchmark testing
// of a specific component in a device. Additional files such as test suites
// and test configuration are installed on the side of the compiled executed
// binary.
func BenchmarkFactory() android.Module {
	module := NewBenchmark(android.HostAndDeviceSupported)
	return module.Init()
}

// cc_test_host compiles a test host binary.
func TestHostFactory() android.Module {
	module := NewTest(android.HostSupported)
	return module.Init()
}

// cc_benchmark_host compiles an executable binary that performs benchmark
// testing of a specific component in the host. Additional files such as
// test suites and test configuration are installed on the side of the
// compiled executed binary.
func BenchmarkHostFactory() android.Module {
	module := NewBenchmark(android.HostSupported)
	return module.Init()
}

type testPerSrc interface {
	testPerSrc() bool
	srcs() []string
	setSrc(string, string)
}

func (test *testBinary) testPerSrc() bool {
	return Bool(test.Properties.Test_per_src)
}

func (test *testBinary) srcs() []string {
	return test.baseCompiler.Properties.Srcs
}

func (test *testBinary) setSrc(name, src string) {
	test.baseCompiler.Properties.Srcs = []string{src}
	test.binaryDecorator.Properties.Stem = StringPtr(name)
}

var _ testPerSrc = (*testBinary)(nil)

func testPerSrcMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok {
		if test, ok := m.linker.(testPerSrc); ok {
			if test.testPerSrc() && len(test.srcs()) > 0 {
				if duplicate, found := checkDuplicate(test.srcs()); found {
					mctx.PropertyErrorf("srcs", "found a duplicate entry %q", duplicate)
					return
				}
				testNames := make([]string, len(test.srcs()))
				for i, src := range test.srcs() {
					testNames[i] = strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
				}
				tests := mctx.CreateLocalVariations(testNames...)
				for i, src := range test.srcs() {
					tests[i].(*Module).linker.(testPerSrc).setSrc(testNames[i], src)
				}
			}
		}
	}
}

func checkDuplicate(values []string) (duplicate string, found bool) {
	seen := make(map[string]string)
	for _, v := range values {
		if duplicate, found = seen[v]; found {
			return
		}
		seen[v] = v
	}
	return
}

type testDecorator struct {
	Properties TestProperties
	linker     *baseLinker
}

func (test *testDecorator) gtest() bool {
	return BoolDefault(test.Properties.Gtest, true)
}

func (test *testDecorator) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	if !test.gtest() {
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
		case android.Darwin:
			flags.CFlags = append(flags.CFlags, "-DGTEST_OS_MAC")
		}
	} else {
		flags.CFlags = append(flags.CFlags, "-DGTEST_OS_LINUX_ANDROID")
	}

	return flags
}

func (test *testDecorator) linkerDeps(ctx BaseModuleContext, deps Deps) Deps {
	if test.gtest() {
		if ctx.useSdk() && ctx.Device() {
			deps.StaticLibs = append(deps.StaticLibs, "libgtest_main_ndk_c++", "libgtest_ndk_c++")
		} else if BoolDefault(test.Properties.Isolated, false) {
			deps.StaticLibs = append(deps.StaticLibs, "libgtest_isolated_main")
		} else {
			deps.StaticLibs = append(deps.StaticLibs, "libgtest_main", "libgtest")
		}
	}

	return deps
}

func (test *testDecorator) linkerInit(ctx BaseModuleContext, linker *baseLinker) {
	// 1. Add ../../lib[64] to rpath so that out/host/linux-x86/nativetest/<test dir>/<test> can
	// find out/host/linux-x86/lib[64]/library.so
	// 2. Add ../../../lib[64] to rpath so that out/host/linux-x86/testcases/<test dir>/<CPU>/<test> can
	// also find out/host/linux-x86/lib[64]/library.so
	runpaths := []string{"../../lib", "../../../lib"}
	for _, runpath := range runpaths {
		if ctx.toolchain().Is64Bit() {
			runpath += "64"
		}
		linker.dynamicProperties.RunPaths = append(linker.dynamicProperties.RunPaths, runpath)
	}

	// add "" to rpath so that test binaries can find libraries in their own test directory
	linker.dynamicProperties.RunPaths = append(linker.dynamicProperties.RunPaths, "")
}

func (test *testDecorator) linkerProps() []interface{} {
	return []interface{}{&test.Properties}
}

func NewTestInstaller() *baseInstaller {
	return NewBaseInstaller("nativetest", "nativetest64", InstallInData)
}

type testBinary struct {
	testDecorator
	*binaryDecorator
	*baseCompiler
	Properties TestBinaryProperties
	data       android.Paths
	testConfig android.Path
}

func (test *testBinary) linkerProps() []interface{} {
	props := append(test.testDecorator.linkerProps(), test.binaryDecorator.linkerProps()...)
	props = append(props, &test.Properties)
	return props
}

func (test *testBinary) linkerInit(ctx BaseModuleContext) {
	test.testDecorator.linkerInit(ctx, test.binaryDecorator.baseLinker)
	test.binaryDecorator.linkerInit(ctx)
}

func (test *testBinary) linkerDeps(ctx DepsContext, deps Deps) Deps {
	deps = test.testDecorator.linkerDeps(ctx, deps)
	deps = test.binaryDecorator.linkerDeps(ctx, deps)
	return deps
}

func (test *testBinary) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = test.binaryDecorator.linkerFlags(ctx, flags)
	flags = test.testDecorator.linkerFlags(ctx, flags)
	return flags
}

func (test *testBinary) install(ctx ModuleContext, file android.Path) {
	test.data = android.PathsForModuleSrc(ctx, test.Properties.Data)
	var configs []tradefed.Config
	if Bool(test.Properties.Require_root) {
		configs = append(configs, tradefed.Preparer{"com.android.tradefed.targetprep.RootTargetPreparer"})
	}
	if Bool(test.testDecorator.Properties.Isolated) {
		configs = append(configs, tradefed.Option{"not-shardable", "true"})
	}
	if test.Properties.Test_options.Run_test_as != nil {
		configs = append(configs, tradefed.Option{"run-test-as", String(test.Properties.Test_options.Run_test_as)})
	}

	test.testConfig = tradefed.AutoGenNativeTestConfig(ctx, test.Properties.Test_config,
		test.Properties.Test_config_template, test.Properties.Test_suites, configs)

	test.binaryDecorator.baseInstaller.dir = "nativetest"
	test.binaryDecorator.baseInstaller.dir64 = "nativetest64"

	if !Bool(test.Properties.No_named_install_directory) {
		test.binaryDecorator.baseInstaller.relative = ctx.ModuleName()
	} else if String(test.binaryDecorator.baseInstaller.Properties.Relative_install_path) == "" {
		ctx.PropertyErrorf("no_named_install_directory", "Module install directory may only be disabled if relative_install_path is set")
	}

	test.binaryDecorator.baseInstaller.install(ctx, file)
}

func NewTest(hod android.HostOrDeviceSupported) *Module {
	module, binary := NewBinary(hod)
	module.multilib = android.MultilibBoth
	binary.baseInstaller = NewTestInstaller()

	test := &testBinary{
		testDecorator: testDecorator{
			linker: binary.baseLinker,
		},
		binaryDecorator: binary,
		baseCompiler:    NewBaseCompiler(),
	}
	module.compiler = test
	module.linker = test
	module.installer = test
	return module
}

type testLibrary struct {
	testDecorator
	*libraryDecorator
}

func (test *testLibrary) linkerProps() []interface{} {
	return append(test.testDecorator.linkerProps(), test.libraryDecorator.linkerProps()...)
}

func (test *testLibrary) linkerInit(ctx BaseModuleContext) {
	test.testDecorator.linkerInit(ctx, test.libraryDecorator.baseLinker)
	test.libraryDecorator.linkerInit(ctx)
}

func (test *testLibrary) linkerDeps(ctx DepsContext, deps Deps) Deps {
	deps = test.testDecorator.linkerDeps(ctx, deps)
	deps = test.libraryDecorator.linkerDeps(ctx, deps)
	return deps
}

func (test *testLibrary) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = test.libraryDecorator.linkerFlags(ctx, flags)
	flags = test.testDecorator.linkerFlags(ctx, flags)
	return flags
}

func NewTestLibrary(hod android.HostOrDeviceSupported) *Module {
	module, library := NewLibrary(android.HostAndDeviceSupported)
	library.baseInstaller = NewTestInstaller()
	test := &testLibrary{
		testDecorator: testDecorator{
			linker: library.baseLinker,
		},
		libraryDecorator: library,
	}
	module.linker = test
	return module
}

type BenchmarkProperties struct {
	// list of files or filegroup modules that provide data that should be installed alongside
	// the test
	Data []string `android:"path"`

	// list of compatibility suites (for example "cts", "vts") that the module should be
	// installed into.
	Test_suites []string `android:"arch_variant"`

	// the name of the test configuration (for example "AndroidTest.xml") that should be
	// installed with the module.
	Test_config *string `android:"path,arch_variant"`

	// the name of the test configuration template (for example "AndroidTestTemplate.xml") that
	// should be installed with the module.
	Test_config_template *string `android:"path,arch_variant"`

	// Add RootTargetPreparer to auto generated test config. This guarantees the test to run
	// with root permission.
	Require_root *bool
}

type benchmarkDecorator struct {
	*binaryDecorator
	Properties BenchmarkProperties
	data       android.Paths
	testConfig android.Path
}

func (benchmark *benchmarkDecorator) linkerInit(ctx BaseModuleContext) {
	runpath := "../../lib"
	if ctx.toolchain().Is64Bit() {
		runpath += "64"
	}
	benchmark.baseLinker.dynamicProperties.RunPaths = append(benchmark.baseLinker.dynamicProperties.RunPaths, runpath)
	benchmark.binaryDecorator.linkerInit(ctx)
}

func (benchmark *benchmarkDecorator) linkerProps() []interface{} {
	props := benchmark.binaryDecorator.linkerProps()
	props = append(props, &benchmark.Properties)
	return props
}

func (benchmark *benchmarkDecorator) linkerDeps(ctx DepsContext, deps Deps) Deps {
	deps = benchmark.binaryDecorator.linkerDeps(ctx, deps)
	deps.StaticLibs = append(deps.StaticLibs, "libgoogle-benchmark")
	return deps
}

func (benchmark *benchmarkDecorator) install(ctx ModuleContext, file android.Path) {
	benchmark.data = android.PathsForModuleSrc(ctx, benchmark.Properties.Data)
	var configs []tradefed.Config
	if Bool(benchmark.Properties.Require_root) {
		configs = append(configs, tradefed.Preparer{"com.android.tradefed.targetprep.RootTargetPreparer"})
	}
	benchmark.testConfig = tradefed.AutoGenNativeBenchmarkTestConfig(ctx, benchmark.Properties.Test_config,
		benchmark.Properties.Test_config_template, benchmark.Properties.Test_suites, configs)

	benchmark.binaryDecorator.baseInstaller.dir = filepath.Join("benchmarktest", ctx.ModuleName())
	benchmark.binaryDecorator.baseInstaller.dir64 = filepath.Join("benchmarktest64", ctx.ModuleName())
	benchmark.binaryDecorator.baseInstaller.install(ctx, file)
}

func NewBenchmark(hod android.HostOrDeviceSupported) *Module {
	module, binary := NewBinary(hod)
	module.multilib = android.MultilibBoth
	binary.baseInstaller = NewBaseInstaller("benchmarktest", "benchmarktest64", InstallInData)

	benchmark := &benchmarkDecorator{
		binaryDecorator: binary,
	}
	module.linker = benchmark
	module.installer = benchmark
	return module
}
