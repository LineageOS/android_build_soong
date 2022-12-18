// Copyright 2022 Google Inc. All rights reserved.
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

package tradefed

import (
	"android/soong/android"
	"android/soong/bazel"

	"github.com/google/blueprint/proptools"
)

const (
	InstrumentationTestConfigTemplate  = "build/make/core/instrumentation_test_config_template.xml"
	JavaTestConfigTemplate             = "build/make/core/java_test_config_template.xml"
	JavaHostTestConfigTemplate         = "build/make/core/java_host_test_config_template.xml"
	JavaHostUnitTestConfigTemplate     = "build/make/core/java_host_unit_test_config_template.xml"
	NativeBenchmarkTestConfigTemplate  = "build/make/core/native_benchmark_test_config_template.xml"
	NativeHostTestConfigTemplate       = "build/make/core/native_host_test_config_template.xml"
	NativeTestConfigTemplate           = "build/make/core/native_test_config_template.xml"
	PythonBinaryHostTestConfigTemplate = "build/make/core/python_binary_host_test_config_template.xml"
	RustDeviceTestConfigTemplate       = "build/make/core/rust_device_test_config_template.xml"
	RustHostTestConfigTemplate         = "build/make/core/rust_host_test_config_template.xml"
	RustDeviceBenchmarkConfigTemplate  = "build/make/core/rust_device_benchmark_config_template.xml"
	RustHostBenchmarkConfigTemplate    = "build/make/core/rust_host_benchmark_config_template.xml"
	RobolectricTestConfigTemplate      = "build/make/core/robolectric_test_config_template.xml"
	ShellTestConfigTemplate            = "build/make/core/shell_test_config_template.xml"
)

type TestConfigAttributes struct {
	Test_config *bazel.Label

	Auto_generate_test_config *bool
	Template_test_config      *bazel.Label
	Template_configs          []string
	Template_install_base     *string
}

func GetTestConfigAttributes(
	ctx android.TopDownMutatorContext,
	testConfig *string,
	extraTestConfigs []string,
	autoGenConfig *bool,
	testSuites []string,
	template *string,
	templateConfigs []Config,
	templateInstallBase *string) TestConfigAttributes {

	attrs := TestConfigAttributes{}
	attrs.Test_config = GetTestConfig(ctx, testConfig)
	// do not generate a test config if
	// 1) test config already found
	// 2) autoGenConfig == false
	// 3) CTS tests and no template specified.
	// CTS Modules can be used for test data, so test config files must be explicitly specified.
	if (attrs.Template_test_config != nil) ||
		proptools.Bool(autoGenConfig) == false ||
		(template == nil && !android.InList("cts", testSuites)) {

		return attrs
	}

	// Add properties for the bazel rule to generate a test config
	// since a test config was not specified.
	templateLabel := android.BazelLabelForModuleSrcSingle(ctx, *template)
	attrs.Template_test_config = &templateLabel
	attrs.Auto_generate_test_config = autoGenConfig
	var configStrings []string
	for _, c := range templateConfigs {
		configString := proptools.NinjaAndShellEscape(c.Config())
		configStrings = append(configStrings, configString)
	}
	attrs.Template_configs = configStrings
	attrs.Template_install_base = templateInstallBase
	return attrs
}

func GetTestConfig(
	ctx android.TopDownMutatorContext,
	testConfig *string,
) *bazel.Label {

	if testConfig != nil {
		c, _ := android.BazelStringOrLabelFromProp(ctx, testConfig)
		if c.Value != nil {
			return c.Value
		}
	}

	// check for default AndroidTest.xml
	defaultTestConfigPath := ctx.ModuleDir() + "/AndroidTest.xml"
	c, _ := android.BazelStringOrLabelFromProp(ctx, &defaultTestConfigPath)
	return c.Value
}
