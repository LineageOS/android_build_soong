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
	"android/soong/android"
)

func init() {
	android.RegisterModuleType("rust_binary", RustBinaryFactory)
	android.RegisterModuleType("rust_binary_host", RustBinaryHostFactory)
}

type BinaryCompilerProperties struct {
	// path to the main source file that contains the program entry point (e.g. src/main.rs)
	Srcs []string `android:"path,arch_variant"`

	// passes -C prefer-dynamic to rustc, which tells it to dynamically link the stdlib
	// (assuming it has no dylib dependencies already)
	Prefer_dynamic *bool
}

type binaryDecorator struct {
	*baseCompiler

	Properties           BinaryCompilerProperties
	distFile             android.OptionalPath
	unstrippedOutputFile android.Path
}

var _ compiler = (*binaryDecorator)(nil)

// rust_binary produces a binary that is runnable on a device.
func RustBinaryFactory() android.Module {
	module, _ := NewRustBinary(android.HostAndDeviceSupported)
	return module.Init()
}

func RustBinaryHostFactory() android.Module {
	module, _ := NewRustBinary(android.HostSupported)
	return module.Init()
}

func NewRustBinary(hod android.HostOrDeviceSupported) (*Module, *binaryDecorator) {
	module := newModule(hod, android.MultilibFirst)

	binary := &binaryDecorator{
		baseCompiler: NewBaseCompiler("bin", "", InstallInSystem),
	}

	module.compiler = binary

	return module, binary
}

func (binary *binaryDecorator) preferDynamic() bool {
	return Bool(binary.Properties.Prefer_dynamic)
}

func (binary *binaryDecorator) compilerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = binary.baseCompiler.compilerFlags(ctx, flags)

	if ctx.toolchain().Bionic() {
		// no-undefined-version breaks dylib compilation since __rust_*alloc* functions aren't defined,
		// but we can apply this to binaries.
		flags.LinkFlags = append(flags.LinkFlags,
			"-Wl,--gc-sections",
			"-Wl,-z,nocopyreloc",
			"-Wl,--no-undefined-version")
	}

	if binary.preferDynamic() {
		flags.RustFlags = append(flags.RustFlags, "-C prefer-dynamic")
	}
	return flags
}

func (binary *binaryDecorator) compilerDeps(ctx DepsContext, deps Deps) Deps {
	deps = binary.baseCompiler.compilerDeps(ctx, deps)

	if ctx.toolchain().Bionic() {
		deps = binary.baseCompiler.bionicDeps(ctx, deps)
		deps.CrtBegin = "crtbegin_dynamic"
		deps.CrtEnd = "crtend_android"
	}

	return deps
}

func (binary *binaryDecorator) compilerProps() []interface{} {
	return append(binary.baseCompiler.compilerProps(),
		&binary.Properties)
}

func (binary *binaryDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) android.Path {
	fileName := binary.getStem(ctx) + ctx.toolchain().ExecutableSuffix()

	srcPath := srcPathFromModuleSrcs(ctx, binary.Properties.Srcs)

	outputFile := android.PathForModuleOut(ctx, fileName)
	binary.unstrippedOutputFile = outputFile

	flags.RustFlags = append(flags.RustFlags, deps.depFlags...)

	TransformSrcToBinary(ctx, srcPath, deps, flags, outputFile, deps.linkDirs)

	return outputFile
}
