// Copyright 2016 Google Inc. All rights reserved.
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

package cc

import (
	"android/soong/android"
	"android/soong/cc/config"
)

func init() {
	android.RegisterModuleType("cc_fuzz", FuzzFactory)
}

// cc_fuzz creates a host/device fuzzer binary. Host binaries can be found at
// $ANDROID_HOST_OUT/fuzz/, and device binaries can be found at /data/fuzz on
// your device, or $ANDROID_PRODUCT_OUT/data/fuzz in your build tree.
func FuzzFactory() android.Module {
	module := NewFuzz(android.HostAndDeviceSupported)
	return module.Init()
}

func NewFuzzInstaller() *baseInstaller {
	return NewBaseInstaller("fuzz", "fuzz", InstallInData)
}

type fuzzBinary struct {
	*binaryDecorator
	*baseCompiler
}

func (fuzz *fuzzBinary) linkerProps() []interface{} {
	props := fuzz.binaryDecorator.linkerProps()
	return props
}

func (fuzz *fuzzBinary) linkerInit(ctx BaseModuleContext) {
	// Add ../lib[64] to rpath so that out/host/linux-x86/fuzz/<fuzzer> can
	// find out/host/linux-x86/lib[64]/library.so
	runpaths := []string{"../lib"}
	for _, runpath := range runpaths {
		if ctx.toolchain().Is64Bit() {
			runpath += "64"
		}
		fuzz.binaryDecorator.baseLinker.dynamicProperties.RunPaths = append(
			fuzz.binaryDecorator.baseLinker.dynamicProperties.RunPaths, runpath)
	}

	// add "" to rpath so that fuzzer binaries can find libraries in their own fuzz directory
	fuzz.binaryDecorator.baseLinker.dynamicProperties.RunPaths = append(
		fuzz.binaryDecorator.baseLinker.dynamicProperties.RunPaths, "")

	fuzz.binaryDecorator.linkerInit(ctx)
}

func (fuzz *fuzzBinary) linkerDeps(ctx DepsContext, deps Deps) Deps {
	deps.StaticLibs = append(deps.StaticLibs,
		config.LibFuzzerRuntimeLibrary(ctx.toolchain()))
	deps = fuzz.binaryDecorator.linkerDeps(ctx, deps)
	return deps
}

func (fuzz *fuzzBinary) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = fuzz.binaryDecorator.linkerFlags(ctx, flags)
	return flags
}

func (fuzz *fuzzBinary) install(ctx ModuleContext, file android.Path) {
	fuzz.binaryDecorator.baseInstaller.dir = "fuzz"
	fuzz.binaryDecorator.baseInstaller.dir64 = "fuzz"
	fuzz.binaryDecorator.baseInstaller.install(ctx, file)
}

func NewFuzz(hod android.HostOrDeviceSupported) *Module {
	module, binary := NewBinary(hod)

	// TODO(mitchp): The toolchain does not currently export the x86 (32-bit)
	// variant of libFuzzer for host. There is no way to only disable the host
	// 32-bit variant, so we specify cc_fuzz targets as 64-bit only. This doesn't
	// hurt anyone, as cc_fuzz is mostly for experimental targets as of this
	// moment.
	module.multilib = "64"

	binary.baseInstaller = NewFuzzInstaller()
	module.sanitize.SetSanitizer(fuzzer, true)

	fuzz := &fuzzBinary{
		binaryDecorator: binary,
		baseCompiler:    NewBaseCompiler(),
	}
	module.compiler = fuzz
	module.linker = fuzz
	module.installer = fuzz

	// The fuzzer runtime is not present for darwin host modules, disable cc_fuzz modules when targeting darwin.
	android.AddLoadHook(module, func(ctx android.LoadHookContext) {
		disableDarwinAndLinuxBionic := struct {
			Target struct {
				Darwin struct {
					Enabled *bool
				}
				Linux_bionic struct {
					Enabled *bool
				}
			}
		}{}
		disableDarwinAndLinuxBionic.Target.Darwin.Enabled = BoolPtr(false)
		disableDarwinAndLinuxBionic.Target.Linux_bionic.Enabled = BoolPtr(false)
		ctx.AppendProperties(&disableDarwinAndLinuxBionic)
	})

	return module
}
