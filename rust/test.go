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
)

// A test module is a binary module with extra --test compiler flag
// and different default installation directory.
// In golang, inheriance is written as a component.
type testBinaryDecorator struct {
	*binaryDecorator
}

func NewRustTest(hod android.HostOrDeviceSupported) (*Module, *testBinaryDecorator) {
	module := newModule(hod, android.MultilibFirst)

	test := &testBinaryDecorator{
		binaryDecorator: &binaryDecorator{
			// TODO(chh): set up dir64?
			baseCompiler: NewBaseCompiler("testcases", ""),
		},
	}

	module.compiler = test

	return module, test
}

func (test *testBinaryDecorator) compilerFlags(ctx ModuleContext, flags Flags) Flags {
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

func (test *testBinaryDecorator) testPerSrc() bool {
	return true
}

func (test *testBinaryDecorator) srcs() []string {
	return test.Properties.Srcs
}

func (test *testBinaryDecorator) setSrc(name, src string) {
	test.Properties.Srcs = []string{src}
	test.baseCompiler.Properties.Stem = StringPtr(name)
}

func (test *testBinaryDecorator) unsetSrc() {
	test.Properties.Srcs = nil
	test.baseCompiler.Properties.Stem = StringPtr("")
}

type testPerSrc interface {
	testPerSrc() bool
	srcs() []string
	setSrc(string, string)
	unsetSrc()
}

var _ testPerSrc = (*testBinaryDecorator)(nil)

func TestPerSrcMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok {
		if test, ok := m.compiler.(testPerSrc); ok {
			numTests := len(test.srcs())
			if test.testPerSrc() && numTests > 0 {
				if duplicate, found := android.CheckDuplicate(test.srcs()); found {
					mctx.PropertyErrorf("srcs", "found a duplicate entry %q", duplicate)
					return
				}
				testNames := make([]string, numTests)
				for i, src := range test.srcs() {
					testNames[i] = strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
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
