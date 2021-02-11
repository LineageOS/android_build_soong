// Copyright 2019 The Android Open Source Project
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
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/tradefed"
)

// Test option struct.
type TestOptions struct {
	// If the test is a hostside(no device required) unittest that shall be run during presubmit check.
	Unit_test *bool
}

type TestProperties struct {
	// Disables the creation of a test-specific directory when used with
	// relative_install_path. Useful if several tests need to be in the same
	// directory, but test_per_src doesn't work.
	No_named_install_directory *bool

	// the name of the test configuration (for example "AndroidTest.xml") that should be
	// installed with the module.
	Test_config *string `android:"path,arch_variant"`

	// the name of the test configuration template (for example "AndroidTestTemplate.xml") that
	// should be installed with the module.
	Test_config_template *string `android:"path,arch_variant"`

	// list of compatibility suites (for example "cts", "vts") that the module should be
	// installed into.
	Test_suites []string `android:"arch_variant"`

	// list of files or filegroup modules that provide data that should be installed alongside
	// the test
	Data []string `android:"path,arch_variant"`

	// Flag to indicate whether or not to create test config automatically. If AndroidTest.xml
	// doesn't exist next to the Android.bp, this attribute doesn't need to be set to true
	// explicitly.
	Auto_gen_config *bool

	// if set, build with the standard Rust test harness. Defaults to true.
	Test_harness *bool

	// Test options.
	Test_options TestOptions
}

// A test module is a binary module with extra --test compiler flag
// and different default installation directory.
// In golang, inheriance is written as a component.
type testDecorator struct {
	*binaryDecorator
	Properties TestProperties
	testConfig android.Path

	data []android.DataPath
}

func (test *testDecorator) dataPaths() []android.DataPath {
	return test.data
}

func (test *testDecorator) nativeCoverage() bool {
	return true
}

func (test *testDecorator) testHarness() bool {
	return BoolDefault(test.Properties.Test_harness, true)
}

func NewRustTest(hod android.HostOrDeviceSupported) (*Module, *testDecorator) {
	// Build both 32 and 64 targets for device tests.
	// Cannot build both for host tests yet if the test depends on
	// something like proc-macro2 that cannot be built for both.
	multilib := android.MultilibBoth
	if hod != android.DeviceSupported && hod != android.HostAndDeviceSupported {
		multilib = android.MultilibFirst
	}
	module := newModule(hod, multilib)

	test := &testDecorator{
		binaryDecorator: &binaryDecorator{
			baseCompiler: NewBaseCompiler("nativetest", "nativetest64", InstallInData),
		},
	}

	module.compiler = test
	return module, test
}

func (test *testDecorator) compilerProps() []interface{} {
	return append(test.binaryDecorator.compilerProps(), &test.Properties)
}

func (test *testDecorator) install(ctx ModuleContext) {
	test.testConfig = tradefed.AutoGenRustTestConfig(ctx,
		test.Properties.Test_config,
		test.Properties.Test_config_template,
		test.Properties.Test_suites,
		nil,
		test.Properties.Auto_gen_config)

	dataSrcPaths := android.PathsForModuleSrc(ctx, test.Properties.Data)

	for _, dataSrcPath := range dataSrcPaths {
		test.data = append(test.data, android.DataPath{SrcPath: dataSrcPath})
	}

	// default relative install path is module name
	if !Bool(test.Properties.No_named_install_directory) {
		test.baseCompiler.relative = ctx.ModuleName()
	} else if String(test.baseCompiler.Properties.Relative_install_path) == "" {
		ctx.PropertyErrorf("no_named_install_directory", "Module install directory may only be disabled if relative_install_path is set")
	}

	if ctx.Host() && test.Properties.Test_options.Unit_test == nil {
		test.Properties.Test_options.Unit_test = proptools.BoolPtr(true)
	}
	test.binaryDecorator.install(ctx)
}

func (test *testDecorator) compilerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = test.binaryDecorator.compilerFlags(ctx, flags)
	if test.testHarness() {
		flags.RustFlags = append(flags.RustFlags, "--test")
	}
	if ctx.Device() {
		flags.RustFlags = append(flags.RustFlags, "-Z panic_abort_tests")
	}
	return flags
}

func (test *testDecorator) autoDep(ctx android.BottomUpMutatorContext) autoDep {
	return rlibAutoDep
}

func init() {
	// Rust tests are binary files built with --test.
	android.RegisterModuleType("rust_test", RustTestFactory)
	android.RegisterModuleType("rust_test_host", RustTestHostFactory)
}

func RustTestFactory() android.Module {
	module, _ := NewRustTest(android.HostAndDeviceSupported)
	return module.Init()
}

func RustTestHostFactory() android.Module {
	module, _ := NewRustTest(android.HostSupported)
	return module.Init()
}

func (test *testDecorator) stdLinkage(ctx *depsContext) RustLinkage {
	return RlibLinkage
}
