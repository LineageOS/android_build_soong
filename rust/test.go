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
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/tradefed"
)

type TestProperties struct {
	// the name of the test configuration (for example "AndroidTest.xml") that should be
	// installed with the module.
	Test_config *string `android:"path,arch_variant"`

	// the name of the test configuration template (for example "AndroidTestTemplate.xml") that
	// should be installed with the module.
	Test_config_template *string `android:"path,arch_variant"`

	// list of compatibility suites (for example "cts", "vts") that the module should be
	// installed into.
	Test_suites []string `android:"arch_variant"`

	// Flag to indicate whether or not to create test config automatically. If AndroidTest.xml
	// doesn't exist next to the Android.bp, this attribute doesn't need to be set to true
	// explicitly.
	Auto_gen_config *bool
}

// A test module is a binary module with extra --test compiler flag
// and different default installation directory.
// In golang, inheriance is written as a component.
type testDecorator struct {
	*binaryDecorator
	Properties TestProperties
	testConfig android.Path
}

func NewRustTest(hod android.HostOrDeviceSupported) (*Module, *testDecorator) {
	module := newModule(hod, android.MultilibFirst)

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

func (test *testDecorator) getMutatedModuleSubName(moduleName string) string {
	stem := String(test.baseCompiler.Properties.Stem)
	if stem != "" && !strings.HasSuffix(moduleName, "_"+stem) {
		// Avoid repeated suffix in the module name.
		return "_" + stem
	}
	return ""
}

func (test *testDecorator) install(ctx ModuleContext, file android.Path) {
	name := ctx.ModuleName()
	path := test.baseCompiler.relativeInstallPath()
	// on device, use mutated module name
	name = name + test.getMutatedModuleSubName(name)
	if !ctx.Device() { // on host, use mutated module name + arch type + stem name
		stem := String(test.baseCompiler.Properties.Stem)
		if stem == "" {
			stem = name
		}
		name = filepath.Join(name, ctx.Arch().ArchType.String(), stem)
	}
	test.testConfig = tradefed.AutoGenRustTestConfig(ctx, name,
		test.Properties.Test_config,
		test.Properties.Test_config_template,
		test.Properties.Test_suites,
		test.Properties.Auto_gen_config)
	// default relative install path is module name
	if path == "" {
		test.baseCompiler.relative = ctx.ModuleName()
	}
	test.binaryDecorator.install(ctx, file)
}

func (test *testDecorator) compilerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = test.binaryDecorator.compilerFlags(ctx, flags)
	flags.RustFlags = append(flags.RustFlags, "--test")
	return flags
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

func (test *testDecorator) testPerSrc() bool {
	return true
}

func (test *testDecorator) srcs() []string {
	return test.binaryDecorator.Properties.Srcs
}

func (test *testDecorator) setSrc(name, src string) {
	test.binaryDecorator.Properties.Srcs = []string{src}
	test.baseCompiler.Properties.Stem = StringPtr(name)
}

func (test *testDecorator) unsetSrc() {
	test.binaryDecorator.Properties.Srcs = nil
	test.baseCompiler.Properties.Stem = StringPtr("")
}

type testPerSrc interface {
	testPerSrc() bool
	srcs() []string
	setSrc(string, string)
	unsetSrc()
}

var _ testPerSrc = (*testDecorator)(nil)

func TestPerSrcMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok {
		if test, ok := m.compiler.(testPerSrc); ok {
			numTests := len(test.srcs())
			if test.testPerSrc() && numTests > 0 {
				if duplicate, found := android.CheckDuplicate(test.srcs()); found {
					mctx.PropertyErrorf("srcs", "found a duplicate entry %q", duplicate)
					return
				}
				// Rust compiler always compiles one source file at a time and
				// uses the crate name as output file name.
				// Cargo uses the test source file name as default crate name,
				// but that can be redefined.
				// So when there are multiple source files, the source file names will
				// be the output file names, but when there is only one test file,
				// use the crate name.
				testNames := make([]string, numTests)
				for i, src := range test.srcs() {
					testNames[i] = strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
				}
				crateName := m.compiler.crateName()
				if numTests == 1 && crateName != "" {
					testNames[0] = crateName
				}
				// TODO(chh): Add an "all tests" variation like cc/test.go?
				tests := mctx.CreateLocalVariations(testNames...)
				for i, src := range test.srcs() {
					tests[i].(*Module).compiler.(testPerSrc).setSrc(testNames[i], src)
				}
			}
		}
	}
}
