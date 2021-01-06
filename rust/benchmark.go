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
)

type BenchmarkProperties struct {
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

	deps.Rustlibs = append(deps.Rustlibs, "libcriterion")

	return deps
}

func (benchmark *benchmarkDecorator) compilerProps() []interface{} {
	return append(benchmark.binaryDecorator.compilerProps(), &benchmark.Properties)
}
