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

func init() {
	android.RegisterMakeVarsProvider(pctx, makeVarsProvider)
}

func makeVarsProvider(ctx android.MakeVarsContext) {
	ctx.Strict("AUTOGEN_TEST_CONFIG_SCRIPT", "${AutoGenTestConfigScript}")
	ctx.Strict("INSTRUMENTATION_TEST_CONFIG_TEMPLATE", "${InstrumentationTestConfigTemplate}")
	ctx.Strict("JAVA_HOST_TEST_CONFIG_TEMPLATE", "${JavaHostTestConfigTemplate}")
	ctx.Strict("JAVA_TEST_CONFIG_TEMPLATE", "${JavaTestConfigTemplate}")
	ctx.Strict("NATIVE_BENCHMARK_TEST_CONFIG_TEMPLATE", "${NativeBenchmarkTestConfigTemplate}")
	ctx.Strict("NATIVE_HOST_TEST_CONFIG_TEMPLATE", "${NativeHostTestConfigTemplate}")
	ctx.Strict("NATIVE_TEST_CONFIG_TEMPLATE", "${NativeTestConfigTemplate}")
	ctx.Strict("PYTHON_BINARY_HOST_TEST_CONFIG_TEMPLATE", "${PythonBinaryHostTestConfigTemplate}")
	ctx.Strict("RUST_DEVICE_TEST_CONFIG_TEMPLATE", "${RustDeviceTestConfigTemplate}")
	ctx.Strict("RUST_HOST_TEST_CONFIG_TEMPLATE", "${RustHostTestConfigTemplate}")
	ctx.Strict("SHELL_TEST_CONFIG_TEMPLATE", "${ShellTestConfigTemplate}")

	ctx.Strict("EMPTY_TEST_CONFIG", "${EmptyTestConfig}")
}
