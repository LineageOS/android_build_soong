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

package tradefed

import (
	"android/soong/android"
)

var (
	pctx = android.NewPackageContext("android/soong/tradefed")
)

func init() {
	pctx.SourcePathVariable("AutoGenTestConfigScript", "build/make/tools/auto_gen_test_config.py")
	pctx.SourcePathVariable("InstrumentationTestConfigTemplate", "build/make/core/instrumentation_test_config_template.xml")
	pctx.SourcePathVariable("JavaTestConfigTemplate", "build/make/core/java_test_config_template.xml")
	pctx.SourcePathVariable("JavaHostTestConfigTemplate", "build/make/core/java_host_test_config_template.xml")
	pctx.SourcePathVariable("JavaHostUnitTestConfigTemplate", "build/make/core/java_host_unit_test_config_template.xml")
	pctx.SourcePathVariable("NativeBenchmarkTestConfigTemplate", "build/make/core/native_benchmark_test_config_template.xml")
	pctx.SourcePathVariable("NativeHostTestConfigTemplate", "build/make/core/native_host_test_config_template.xml")
	pctx.SourcePathVariable("NativeTestConfigTemplate", "build/make/core/native_test_config_template.xml")
	pctx.SourcePathVariable("PythonBinaryHostMoblyTestConfigTemplate", "build/make/core/python_binary_host_mobly_test_config_template.xml")
	pctx.SourcePathVariable("PythonBinaryHostTestConfigTemplate", "build/make/core/python_binary_host_test_config_template.xml")
	pctx.SourcePathVariable("RavenwoodTestConfigTemplate", "build/make/core/ravenwood_test_config_template.xml")
	pctx.SourcePathVariable("RustDeviceTestConfigTemplate", "build/make/core/rust_device_test_config_template.xml")
	pctx.SourcePathVariable("RustHostTestConfigTemplate", "build/make/core/rust_host_test_config_template.xml")
	pctx.SourcePathVariable("RustDeviceBenchmarkConfigTemplate", "build/make/core/rust_device_benchmark_config_template.xml")
	pctx.SourcePathVariable("RustHostBenchmarkConfigTemplate", "build/make/core/rust_host_benchmark_config_template.xml")
	pctx.SourcePathVariable("RobolectricTestConfigTemplate", "build/make/core/robolectric_test_config_template.xml")
	pctx.SourcePathVariable("ShellTestConfigTemplate", "build/make/core/shell_test_config_template.xml")

	pctx.SourcePathVariable("EmptyTestConfig", "build/make/core/empty_test_config.xml")
}
