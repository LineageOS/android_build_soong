// Copyright 2020 The Android Open Source Project
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
	"android/soong/android"
	"android/soong/tradefed"
)

type BenchmarkProperties struct {
	// Disables the creation of a test-specific directory when used with
	// relative_install_path. Useful if several tests need to be in the same
	// directory, but test_per_src doesn't work.
	No_named_install_directory *bool

	// the name of the test configuration (for example "AndroidBenchmark.xml") that should be
	// installed with the module.
	Test_config *string `android:"path,arch_variant"`

	// the name of the test configuration template (for example "AndroidBenchmarkTemplate.xml") that
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

type benchmarkDecorator struct {
	*binaryDecorator
	Properties BenchmarkProperties
	testConfig android.Path
}

func NewRustBenchmark(hod android.HostOrDeviceSupported) (*Module, *benchmarkDecorator) {
	// Build both 32 and 64 targets for device benchmarks.
	// Cannot build both for host benchmarks yet if the benchmark depends on
	// something like proc-macro2 that cannot be built for both.
	multilib := android.MultilibBoth
	if hod != android.DeviceSupported && hod != android.HostAndDeviceSupported {
		multilib = android.MultilibFirst
	}
	module := newModule(hod, multilib)

	benchmark := &benchmarkDecorator{
		binaryDecorator: &binaryDecorator{
			baseCompiler: NewBaseCompiler("nativebench", "nativebench64", InstallInData),
		},
	}

	module.compiler = benchmark
	module.AddProperties(&benchmark.Properties)
	return module, benchmark
}

func init() {
	android.RegisterModuleType("rust_benchmark", RustBenchmarkFactory)
	android.RegisterModuleType("rust_benchmark_host", RustBenchmarkHostFactory)
}

func RustBenchmarkFactory() android.Module {
	module, _ := NewRustBenchmark(android.HostAndDeviceSupported)
	return module.Init()
}

func RustBenchmarkHostFactory() android.Module {
	module, _ := NewRustBenchmark(android.HostSupported)
	return module.Init()
}

func (benchmark *benchmarkDecorator) autoDep(ctx android.BottomUpMutatorContext) autoDep {
	return rlibAutoDep
}

func (benchmark *benchmarkDecorator) stdLinkage(ctx *depsContext) RustLinkage {
	return RlibLinkage
}

func (benchmark *benchmarkDecorator) compilerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = benchmark.binaryDecorator.compilerFlags(ctx, flags)
	return flags
}

func (benchmark *benchmarkDecorator) compilerDeps(ctx DepsContext, deps Deps) Deps {
	deps = benchmark.binaryDecorator.compilerDeps(ctx, deps)

	deps.Rustlibs = append(deps.Rustlibs, "libtest")
	deps.Rustlibs = append(deps.Rustlibs, "libcriterion")

	return deps
}

func (benchmark *benchmarkDecorator) compilerProps() []interface{} {
	return append(benchmark.binaryDecorator.compilerProps(), &benchmark.Properties)
}

func (benchmark *benchmarkDecorator) install(ctx ModuleContext) {
	benchmark.testConfig = tradefed.AutoGenRustBenchmarkConfig(ctx,
		benchmark.Properties.Test_config,
		benchmark.Properties.Test_config_template,
		benchmark.Properties.Test_suites,
		nil,
		benchmark.Properties.Auto_gen_config)

	// default relative install path is module name
	if !Bool(benchmark.Properties.No_named_install_directory) {
		benchmark.baseCompiler.relative = ctx.ModuleName()
	} else if String(benchmark.baseCompiler.Properties.Relative_install_path) == "" {
		ctx.PropertyErrorf("no_named_install_directory", "Module install directory may only be disabled if relative_install_path is set")
	}

	benchmark.binaryDecorator.install(ctx)
}
