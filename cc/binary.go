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
	"github.com/google/blueprint"

	"android/soong"
	"android/soong/android"
)

type BinaryLinkerProperties struct {
	// compile executable with -static
	Static_executable *bool `android:"arch_variant"`

	// set the name of the output
	Stem string `android:"arch_variant"`

	// append to the name of the output
	Suffix string `android:"arch_variant"`

	// if set, add an extra objcopy --prefix-symbols= step
	Prefix_symbols string
}

func init() {
	soong.RegisterModuleType("cc_binary", binaryFactory)
	soong.RegisterModuleType("cc_binary_host", binaryHostFactory)
}

// Module factory for binaries
func binaryFactory() (blueprint.Module, []interface{}) {
	module := NewBinary(android.HostAndDeviceSupported)
	return module.Init()
}

// Module factory for host binaries
func binaryHostFactory() (blueprint.Module, []interface{}) {
	module := NewBinary(android.HostSupported)
	return module.Init()
}

//
// Executables
//

type binaryLinker struct {
	baseLinker
	stripper

	Properties BinaryLinkerProperties

	hostToolPath android.OptionalPath
}

var _ linker = (*binaryLinker)(nil)

func (binary *binaryLinker) props() []interface{} {
	return append(binary.baseLinker.props(),
		&binary.Properties,
		&binary.stripper.StripProperties)

}

func (binary *binaryLinker) buildStatic() bool {
	return binary.baseLinker.staticBinary()
}

func (binary *binaryLinker) buildShared() bool {
	return !binary.baseLinker.staticBinary()
}

func (binary *binaryLinker) getStem(ctx BaseModuleContext) string {
	stem := ctx.ModuleName()
	if binary.Properties.Stem != "" {
		stem = binary.Properties.Stem
	}

	return stem + binary.Properties.Suffix
}

func (binary *binaryLinker) deps(ctx BaseModuleContext, deps Deps) Deps {
	deps = binary.baseLinker.deps(ctx, deps)
	if ctx.Device() {
		if !Bool(binary.baseLinker.Properties.Nocrt) {
			if !ctx.sdk() {
				if binary.buildStatic() {
					deps.CrtBegin = "crtbegin_static"
				} else {
					deps.CrtBegin = "crtbegin_dynamic"
				}
				deps.CrtEnd = "crtend_android"
			} else {
				if binary.buildStatic() {
					deps.CrtBegin = "ndk_crtbegin_static." + ctx.sdkVersion()
				} else {
					if Bool(binary.Properties.Static_executable) {
						deps.CrtBegin = "ndk_crtbegin_static." + ctx.sdkVersion()
					} else {
						deps.CrtBegin = "ndk_crtbegin_dynamic." + ctx.sdkVersion()
					}
					deps.CrtEnd = "ndk_crtend_android." + ctx.sdkVersion()
				}
			}
		}

		if binary.buildStatic() {
			if inList("libc++_static", deps.StaticLibs) {
				deps.StaticLibs = append(deps.StaticLibs, "libm", "libc", "libdl")
			}
			// static libraries libcompiler_rt, libc and libc_nomalloc need to be linked with
			// --start-group/--end-group along with libgcc.  If they are in deps.StaticLibs,
			// move them to the beginning of deps.LateStaticLibs
			var groupLibs []string
			deps.StaticLibs, groupLibs = filterList(deps.StaticLibs,
				[]string{"libc", "libc_nomalloc", "libcompiler_rt"})
			deps.LateStaticLibs = append(groupLibs, deps.LateStaticLibs...)
		}
	}

	if binary.buildShared() && inList("libc", deps.StaticLibs) {
		ctx.ModuleErrorf("statically linking libc to dynamic executable, please remove libc\n" +
			"from static libs or set static_executable: true")
	}
	return deps
}

func (*binaryLinker) installable() bool {
	return true
}

func (binary *binaryLinker) isDependencyRoot() bool {
	return true
}

func NewBinary(hod android.HostOrDeviceSupported) *Module {
	module := newModule(hod, android.MultilibFirst)
	module.compiler = &baseCompiler{}
	module.linker = &binaryLinker{}
	module.installer = &baseInstaller{
		dir: "bin",
	}
	return module
}

func (binary *binaryLinker) begin(ctx BaseModuleContext) {
	binary.baseLinker.begin(ctx)

	static := Bool(binary.Properties.Static_executable)
	if ctx.Host() {
		if ctx.Os() == android.Linux {
			if binary.Properties.Static_executable == nil && Bool(ctx.AConfig().ProductVariables.HostStaticBinaries) {
				static = true
			}
		} else {
			// Static executables are not supported on Darwin or Windows
			static = false
		}
	}
	if static {
		binary.dynamicProperties.VariantIsStatic = true
		binary.dynamicProperties.VariantIsStaticBinary = true
	}
}

func (binary *binaryLinker) flags(ctx ModuleContext, flags Flags) Flags {
	flags = binary.baseLinker.flags(ctx, flags)

	if ctx.Host() && !binary.staticBinary() {
		flags.LdFlags = append(flags.LdFlags, "-pie")
		if ctx.Os() == android.Windows {
			flags.LdFlags = append(flags.LdFlags, "-Wl,-e_mainCRTStartup")
		}
	}

	// MinGW spits out warnings about -fPIC even for -fpie?!) being ignored because
	// all code is position independent, and then those warnings get promoted to
	// errors.
	if ctx.Os() != android.Windows {
		flags.CFlags = append(flags.CFlags, "-fpie")
	}

	if ctx.Device() {
		if binary.buildStatic() {
			// Clang driver needs -static to create static executable.
			// However, bionic/linker uses -shared to overwrite.
			// Linker for x86 targets does not allow coexistance of -static and -shared,
			// so we add -static only if -shared is not used.
			if !inList("-shared", flags.LdFlags) {
				flags.LdFlags = append(flags.LdFlags, "-static")
			}

			flags.LdFlags = append(flags.LdFlags,
				"-nostdlib",
				"-Bstatic",
				"-Wl,--gc-sections",
			)

		} else {
			if flags.DynamicLinker == "" {
				flags.DynamicLinker = "/system/bin/linker"
				if flags.Toolchain.Is64Bit() {
					flags.DynamicLinker += "64"
				}
			}

			flags.LdFlags = append(flags.LdFlags,
				"-pie",
				"-nostdlib",
				"-Bdynamic",
				"-Wl,--gc-sections",
				"-Wl,-z,nocopyreloc",
			)
		}
	} else {
		if binary.staticBinary() {
			flags.LdFlags = append(flags.LdFlags, "-static")
		}
		if ctx.Darwin() {
			flags.LdFlags = append(flags.LdFlags, "-Wl,-headerpad_max_install_names")
		}
	}

	return flags
}

func (binary *binaryLinker) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objFiles android.Paths) android.Path {

	fileName := binary.getStem(ctx) + flags.Toolchain.ExecutableSuffix()
	outputFile := android.PathForModuleOut(ctx, fileName)
	ret := outputFile
	if ctx.Os().Class == android.Host {
		binary.hostToolPath = android.OptionalPathForPath(outputFile)
	}

	var linkerDeps android.Paths

	sharedLibs := deps.SharedLibs
	sharedLibs = append(sharedLibs, deps.LateSharedLibs...)

	if flags.DynamicLinker != "" {
		flags.LdFlags = append(flags.LdFlags, " -Wl,-dynamic-linker,"+flags.DynamicLinker)
	}

	builderFlags := flagsToBuilderFlags(flags)

	if binary.stripper.needsStrip(ctx) {
		strippedOutputFile := outputFile
		outputFile = android.PathForModuleOut(ctx, "unstripped", fileName)
		binary.stripper.strip(ctx, outputFile, strippedOutputFile, builderFlags)
	}

	if binary.Properties.Prefix_symbols != "" {
		afterPrefixSymbols := outputFile
		outputFile = android.PathForModuleOut(ctx, "unprefixed", fileName)
		TransformBinaryPrefixSymbols(ctx, binary.Properties.Prefix_symbols, outputFile,
			flagsToBuilderFlags(flags), afterPrefixSymbols)
	}

	TransformObjToDynamicBinary(ctx, objFiles, sharedLibs, deps.StaticLibs,
		deps.LateStaticLibs, deps.WholeStaticLibs, linkerDeps, deps.CrtBegin, deps.CrtEnd, true,
		builderFlags, outputFile)

	return ret
}

func (binary *binaryLinker) HostToolPath() android.OptionalPath {
	return binary.hostToolPath
}
