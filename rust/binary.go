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
	// Builds this binary as a static binary. Implies prefer_rlib true.
	//
	// Static executables currently only support for bionic targets. Non-bionic targets will not produce a fully static
	// binary, but will still implicitly imply prefer_rlib true.
	Static_executable *bool `android:"arch_variant"`
}

type binaryDecorator struct {
	*baseCompiler
	stripper Stripper

	Properties BinaryCompilerProperties
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

func (binary *binaryDecorator) compilerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = binary.baseCompiler.compilerFlags(ctx, flags)

	if ctx.toolchain().Bionic() {
		// no-undefined-version breaks dylib compilation since __rust_*alloc* functions aren't defined,
		// but we can apply this to binaries.
		flags.LinkFlags = append(flags.LinkFlags,
			"-Wl,--gc-sections",
			"-Wl,-z,nocopyreloc",
			"-Wl,--no-undefined-version")

		if Bool(binary.Properties.Static_executable) {
			flags.LinkFlags = append(flags.LinkFlags, "-static")
			flags.RustFlags = append(flags.RustFlags, "-C relocation-model=static")
		}
	}

	return flags
}

func (binary *binaryDecorator) compilerDeps(ctx DepsContext, deps Deps) Deps {
	deps = binary.baseCompiler.compilerDeps(ctx, deps)

	if ctx.toolchain().Bionic() {
		deps = bionicDeps(deps, Bool(binary.Properties.Static_executable))
		if Bool(binary.Properties.Static_executable) {
			deps.CrtBegin = "crtbegin_static"
		} else {
			deps.CrtBegin = "crtbegin_dynamic"
		}
		deps.CrtEnd = "crtend_android"
	}

	return deps
}

func (binary *binaryDecorator) compilerProps() []interface{} {
	return append(binary.baseCompiler.compilerProps(),
		&binary.Properties,
		&binary.stripper.StripProperties)
}

func (binary *binaryDecorator) nativeCoverage() bool {
	return true
}

func (binary *binaryDecorator) preferRlib() bool {
	return Bool(binary.baseCompiler.Properties.Prefer_rlib) || Bool(binary.Properties.Static_executable)
}

func (binary *binaryDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) android.Path {
	fileName := binary.getStem(ctx) + ctx.toolchain().ExecutableSuffix()
	srcPath, _ := srcPathFromModuleSrcs(ctx, binary.baseCompiler.Properties.Srcs)
	outputFile := android.PathForModuleOut(ctx, fileName)

	flags.RustFlags = append(flags.RustFlags, deps.depFlags...)
	flags.LinkFlags = append(flags.LinkFlags, deps.linkObjects...)

	outputs := TransformSrcToBinary(ctx, srcPath, deps, flags, outputFile, deps.linkDirs)

	if binary.stripper.NeedsStrip(ctx) {
		strippedOutputFile := android.PathForModuleOut(ctx, "stripped", fileName)
		binary.stripper.StripExecutableOrSharedLib(ctx, outputFile, strippedOutputFile)
		binary.strippedOutputFile = android.OptionalPathForPath(strippedOutputFile)
	}

	binary.coverageFile = outputs.coverageFile

	var coverageFiles android.Paths
	if outputs.coverageFile != nil {
		coverageFiles = append(coverageFiles, binary.coverageFile)
	}
	if len(deps.coverageFiles) > 0 {
		coverageFiles = append(coverageFiles, deps.coverageFiles...)
	}
	binary.coverageOutputZipFile = TransformCoverageFilesToZip(ctx, coverageFiles, binary.getStem(ctx))

	return outputFile
}

func (binary *binaryDecorator) coverageOutputZipPath() android.OptionalPath {
	return binary.coverageOutputZipFile
}

func (binary *binaryDecorator) autoDep(ctx BaseModuleContext) autoDep {
	// Binaries default to dylib dependencies for device, rlib for host.
	if binary.preferRlib() {
		return rlibAutoDep
	} else if ctx.Device() {
		return dylibAutoDep
	} else {
		return rlibAutoDep
	}
}

func (binary *binaryDecorator) stdLinkage(ctx *depsContext) RustLinkage {
	if binary.preferRlib() {
		return RlibLinkage
	}
	return binary.baseCompiler.stdLinkage(ctx)
}

func (binary *binaryDecorator) isDependencyRoot() bool {
	return true
}
