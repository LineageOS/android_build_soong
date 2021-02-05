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
	"android/soong/cc"
	"android/soong/rust/config"
)

func init() {
	android.RegisterModuleType("rust_fuzz", RustFuzzFactory)
}

type fuzzDecorator struct {
	*binaryDecorator

	Properties            cc.FuzzProperties
	dictionary            android.Path
	corpus                android.Paths
	corpusIntermediateDir android.Path
	config                android.Path
	data                  android.Paths
	dataIntermediateDir   android.Path
}

var _ compiler = (*binaryDecorator)(nil)

// rust_binary produces a binary that is runnable on a device.
func RustFuzzFactory() android.Module {
	module, _ := NewRustFuzz(android.HostAndDeviceSupported)
	return module.Init()
}

func NewRustFuzz(hod android.HostOrDeviceSupported) (*Module, *fuzzDecorator) {
	module, binary := NewRustBinary(hod)
	fuzz := &fuzzDecorator{
		binaryDecorator: binary,
	}

	// Change the defaults for the binaryDecorator's baseCompiler
	fuzz.binaryDecorator.baseCompiler.dir = "fuzz"
	fuzz.binaryDecorator.baseCompiler.dir64 = "fuzz"
	fuzz.binaryDecorator.baseCompiler.location = InstallInData
	module.sanitize.SetSanitizer(cc.Fuzzer, true)
	module.compiler = fuzz
	return module, fuzz
}

func (fuzzer *fuzzDecorator) compilerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = fuzzer.binaryDecorator.compilerFlags(ctx, flags)

	// `../lib` for installed fuzz targets (both host and device), and `./lib` for fuzz target packages.
	flags.LinkFlags = append(flags.LinkFlags, `-Wl,-rpath,\$$ORIGIN/../lib`)
	flags.LinkFlags = append(flags.LinkFlags, `-Wl,-rpath,\$$ORIGIN/lib`)

	return flags
}

func (fuzzer *fuzzDecorator) compilerDeps(ctx DepsContext, deps Deps) Deps {
	deps.StaticLibs = append(deps.StaticLibs,
		config.LibFuzzerRuntimeLibrary(ctx.toolchain()))
	deps.SharedLibs = append(deps.SharedLibs,
		config.LibclangRuntimeLibrary(ctx.toolchain(), "asan"))
	deps.SharedLibs = append(deps.SharedLibs, "libc++")
	deps.Rlibs = append(deps.Rlibs, "liblibfuzzer_sys")

	deps = fuzzer.binaryDecorator.compilerDeps(ctx, deps)

	return deps
}

func (fuzzer *fuzzDecorator) compilerProps() []interface{} {
	return append(fuzzer.binaryDecorator.compilerProps(),
		&fuzzer.Properties)
}

func (fuzzer *fuzzDecorator) stdLinkage(ctx *depsContext) RustLinkage {
	return RlibLinkage
}

func (fuzzer *fuzzDecorator) autoDep(ctx android.BottomUpMutatorContext) autoDep {
	return rlibAutoDep
}
