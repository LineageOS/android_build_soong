// Copyright 2017 Google Inc. All rights reserved.
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

package python

import (
	"android/soong/android"
	"android/soong/tradefed"
)

// This file contains the module types for building Python test.

func init() {
	android.RegisterModuleType("python_test_host", PythonTestHostFactory)
	android.RegisterModuleType("python_test", PythonTestFactory)
}

type TestProperties struct {
	// the name of the test configuration (for example "AndroidTest.xml") that should be
	// installed with the module.
	Test_config *string `android:"path,arch_variant"`

	// the name of the test configuration template (for example "AndroidTestTemplate.xml") that
	// should be installed with the module.
	Test_config_template *string `android:"path,arch_variant"`
}

type testDecorator struct {
	*binaryDecorator

	testProperties TestProperties

	testConfig android.Path
}

func (test *testDecorator) bootstrapperProps() []interface{} {
	return append(test.binaryDecorator.bootstrapperProps(), &test.testProperties)
}

func (test *testDecorator) install(ctx android.ModuleContext, file android.Path) {
	test.testConfig = tradefed.AutoGenPythonBinaryHostTestConfig(ctx, test.testProperties.Test_config,
		test.testProperties.Test_config_template, test.binaryDecorator.binaryProperties.Test_suites,
		test.binaryDecorator.binaryProperties.Auto_gen_config)

	test.binaryDecorator.pythonInstaller.dir = "nativetest"
	test.binaryDecorator.pythonInstaller.dir64 = "nativetest64"

	test.binaryDecorator.pythonInstaller.relative = ctx.ModuleName()

	test.binaryDecorator.pythonInstaller.install(ctx, file)
}

func NewTest(hod android.HostOrDeviceSupported) *Module {
	module, binary := NewBinary(hod)

	binary.pythonInstaller = NewPythonInstaller("nativetest", "nativetest64")

	test := &testDecorator{binaryDecorator: binary}

	module.bootstrapper = test
	module.installer = test

	return module
}

func PythonTestHostFactory() android.Module {
	module := NewTest(android.HostSupportedNoCross)

	return module.Init()
}

func PythonTestFactory() android.Module {
	module := NewTest(android.HostAndDeviceSupported)
	module.multilib = android.MultilibBoth

	return module.Init()
}
