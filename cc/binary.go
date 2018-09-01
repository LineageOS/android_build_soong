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
)

type BinaryLinkerProperties struct {
	// compile executable with -static
	Static_executable *bool `android:"arch_variant"`

	// set the name of the output
	Stem *string `android:"arch_variant"`

	// append to the name of the output
	Suffix *string `android:"arch_variant"`

	// if set, add an extra objcopy --prefix-symbols= step
	Prefix_symbols *string

	// local file name to pass to the linker as --version_script
	Version_script *string `android:"arch_variant"`

	// if set, install a symlink to the preferred architecture
	Symlink_preferred_arch *bool

	// install symlinks to the binary.  Symlink names will have the suffix and the binary
	// extension (if any) appended
	Symlinks []string `android:"arch_variant"`

	// do not pass -pie
	No_pie *bool `android:"arch_variant"`

	DynamicLinker string `blueprint:"mutated"`

	// Names of modules to be overridden. Listed modules can only be other binaries
	// (in Make or Soong).
	// This does not completely prevent installation of the overridden binaries, but if both
	// binaries would be installed by default (in PRODUCT_PACKAGES) the other binary will be removed
	// from PRODUCT_PACKAGES.
	Overrides []string

	Additional_deps []string
}

func init() {
	android.RegisterModuleType("cc_binary", binaryFactory)
	android.RegisterModuleType("cc_binary_host", binaryHostFactory)
}

// Module factory for binaries
func binaryFactory() android.Module {
	module, _ := NewBinary(android.HostAndDeviceSupported)
	return module.Init()
}

// Module factory for host binaries
func binaryHostFactory() android.Module {
	module, _ := NewBinary(android.HostSupported)
	return module.Init()
}

//
// Executables
//

type binaryDecorator struct {
	*baseLinker
	*baseInstaller
	stripper

	Properties BinaryLinkerProperties

	toolPath android.OptionalPath

	// Names of symlinks to be installed for use in LOCAL_MODULE_SYMLINKS
	symlinks []string

	// Output archive of gcno coverage information
	coverageOutputFile android.OptionalPath
}

var _ linker = (*binaryDecorator)(nil)

func (binary *binaryDecorator) linkerProps() []interface{} {
	return append(binary.baseLinker.linkerProps(),
		&binary.Properties,
		&binary.stripper.StripProperties)

}

func (binary *binaryDecorator) getStem(ctx BaseModuleContext) string {
	stem := ctx.baseModuleName()
	if String(binary.Properties.Stem) != "" {
		stem = String(binary.Properties.Stem)
	}

	return stem + String(binary.Properties.Suffix)
}

func (binary *binaryDecorator) linkerDeps(ctx DepsContext, deps Deps) Deps {
	deps = binary.baseLinker.linkerDeps(ctx, deps)
	if ctx.toolchain().Bionic() {
		if !Bool(binary.baseLinker.Properties.Nocrt) {
			if !ctx.useSdk() {
				if binary.static() {
					deps.CrtBegin = "crtbegin_static"
				} else {
					deps.CrtBegin = "crtbegin_dynamic"
				}
				deps.CrtEnd = "crtend_android"
			} else {
				// TODO(danalbert): Add generation of crt objects.
				// For `sdk_version: "current"`, we don't actually have a
				// freshly generated set of CRT objects. Use the last stable
				// version.
				version := ctx.sdkVersion()
				if version == "current" {
					version = getCurrentNdkPrebuiltVersion(ctx)
				}

				if binary.static() {
					deps.CrtBegin = "ndk_crtbegin_static." + version
				} else {
					if binary.static() {
						deps.CrtBegin = "ndk_crtbegin_static." + version
					} else {
						deps.CrtBegin = "ndk_crtbegin_dynamic." + version
					}
					deps.CrtEnd = "ndk_crtend_android." + version
				}
			}
		}

		if binary.static() {
			if ctx.selectedStl() == "libc++_static" {
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

		if ctx.Os() == android.LinuxBionic && !binary.static() {
			deps.LinkerScript = "host_bionic_linker_script"
		}
	}

	if !binary.static() && inList("libc", deps.StaticLibs) {
		ctx.ModuleErrorf("statically linking libc to dynamic executable, please remove libc\n" +
			"from static libs or set static_executable: true")
	}

	android.ExtractSourceDeps(ctx, binary.Properties.Version_script)

	return deps
}

func (binary *binaryDecorator) isDependencyRoot() bool {
	return true
}

func NewBinary(hod android.HostOrDeviceSupported) (*Module, *binaryDecorator) {
	module := newModule(hod, android.MultilibFirst)
	binary := &binaryDecorator{
		baseLinker:    NewBaseLinker(),
		baseInstaller: NewBaseInstaller("bin", "", InstallInSystem),
	}
	module.compiler = NewBaseCompiler()
	module.linker = binary
	module.installer = binary
	return module, binary
}

func (binary *binaryDecorator) linkerInit(ctx BaseModuleContext) {
	binary.baseLinker.linkerInit(ctx)

	if !ctx.toolchain().Bionic() {
		if ctx.Os() == android.Linux {
			if binary.Properties.Static_executable == nil && ctx.Config().HostStaticBinaries() {
				binary.Properties.Static_executable = BoolPtr(true)
			}
		} else {
			// Static executables are not supported on Darwin or Windows
			binary.Properties.Static_executable = nil
		}
	}
}

func (binary *binaryDecorator) static() bool {
	return Bool(binary.Properties.Static_executable)
}

func (binary *binaryDecorator) staticBinary() bool {
	return binary.static()
}

func (binary *binaryDecorator) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = binary.baseLinker.linkerFlags(ctx, flags)

	if ctx.Host() && !binary.static() {
		if !ctx.Config().IsEnvTrue("DISABLE_HOST_PIE") {
			flags.LdFlags = append(flags.LdFlags, "-pie")
			if ctx.Windows() {
				flags.LdFlags = append(flags.LdFlags, "-Wl,-e_mainCRTStartup")
			}
		}
	}

	// MinGW spits out warnings about -fPIC even for -fpie?!) being ignored because
	// all code is position independent, and then those warnings get promoted to
	// errors.
	if !ctx.Windows() {
		flags.CFlags = append(flags.CFlags, "-fPIE")
	}

	if ctx.toolchain().Bionic() {
		if binary.static() {
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
				if binary.Properties.DynamicLinker != "" {
					flags.DynamicLinker = binary.Properties.DynamicLinker
				} else {
					switch ctx.Os() {
					case android.Android:
						flags.DynamicLinker = "/system/bin/linker"
					case android.LinuxBionic:
						flags.DynamicLinker = ""
					default:
						ctx.ModuleErrorf("unknown dynamic linker")
					}
					if flags.Toolchain.Is64Bit() {
						flags.DynamicLinker += "64"
					}
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
		if binary.static() {
			flags.LdFlags = append(flags.LdFlags, "-static")
		}
		if ctx.Darwin() {
			flags.LdFlags = append(flags.LdFlags, "-Wl,-headerpad_max_install_names")
		}
	}

	return flags
}

func (binary *binaryDecorator) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {

	versionScript := ctx.ExpandOptionalSource(binary.Properties.Version_script, "version_script")
	fileName := binary.getStem(ctx) + flags.Toolchain.ExecutableSuffix()
	outputFile := android.PathForModuleOut(ctx, fileName)
	ret := outputFile

	var linkerDeps android.Paths

	sharedLibs := deps.SharedLibs
	sharedLibs = append(sharedLibs, deps.LateSharedLibs...)

	if versionScript.Valid() {
		if ctx.Darwin() {
			ctx.PropertyErrorf("version_script", "Not supported on Darwin")
		} else {
			flags.LdFlags = append(flags.LdFlags, "-Wl,--version-script,"+versionScript.String())
			linkerDeps = append(linkerDeps, versionScript.Path())
		}
	}

	if deps.LinkerScript.Valid() {
		flags.LdFlags = append(flags.LdFlags, "-Wl,-T,"+deps.LinkerScript.String())
		linkerDeps = append(linkerDeps, deps.LinkerScript.Path())
	}

	if flags.DynamicLinker != "" {
		flags.LdFlags = append(flags.LdFlags, " -Wl,-dynamic-linker,"+flags.DynamicLinker)
	}

	builderFlags := flagsToBuilderFlags(flags)

	if binary.stripper.needsStrip(ctx) {
		strippedOutputFile := outputFile
		outputFile = android.PathForModuleOut(ctx, "unstripped", fileName)
		binary.stripper.strip(ctx, outputFile, strippedOutputFile, builderFlags)
	}

	if String(binary.Properties.Prefix_symbols) != "" {
		afterPrefixSymbols := outputFile
		outputFile = android.PathForModuleOut(ctx, "unprefixed", fileName)
		TransformBinaryPrefixSymbols(ctx, String(binary.Properties.Prefix_symbols), outputFile,
			flagsToBuilderFlags(flags), afterPrefixSymbols)
	}

	if Bool(binary.baseLinker.Properties.Use_version_lib) && ctx.Host() {
		versionedOutputFile := outputFile
		outputFile = android.PathForModuleOut(ctx, "unversioned", fileName)
		binary.injectVersionSymbol(ctx, outputFile, versionedOutputFile)
	}

	linkerDeps = append(linkerDeps, deps.SharedLibsDeps...)
	linkerDeps = append(linkerDeps, deps.LateSharedLibsDeps...)
	linkerDeps = append(linkerDeps, objs.tidyFiles...)
	linkerDeps = append(linkerDeps, flags.LdFlagsDeps...)

	TransformObjToDynamicBinary(ctx, objs.objFiles, sharedLibs, deps.StaticLibs,
		deps.LateStaticLibs, deps.WholeStaticLibs, linkerDeps, deps.CrtBegin, deps.CrtEnd, true,
		builderFlags, outputFile)

	objs.coverageFiles = append(objs.coverageFiles, deps.StaticLibObjs.coverageFiles...)
	objs.coverageFiles = append(objs.coverageFiles, deps.WholeStaticLibObjs.coverageFiles...)
	binary.coverageOutputFile = TransformCoverageFilesToLib(ctx, objs, builderFlags, binary.getStem(ctx))

	return ret
}

func (binary *binaryDecorator) install(ctx ModuleContext, file android.Path) {
	binary.baseInstaller.install(ctx, file)
	for _, symlink := range binary.Properties.Symlinks {
		binary.symlinks = append(binary.symlinks,
			symlink+String(binary.Properties.Suffix)+ctx.toolchain().ExecutableSuffix())
	}

	if Bool(binary.Properties.Symlink_preferred_arch) {
		if String(binary.Properties.Stem) == "" && String(binary.Properties.Suffix) == "" {
			ctx.PropertyErrorf("symlink_preferred_arch", "must also specify stem or suffix")
		}
		if ctx.TargetPrimary() {
			binary.symlinks = append(binary.symlinks, ctx.baseModuleName())
		}
	}

	for _, symlink := range binary.symlinks {
		ctx.InstallSymlink(binary.baseInstaller.installDir(ctx), symlink, binary.baseInstaller.path)
	}

	if ctx.Os().Class == android.Host {
		binary.toolPath = android.OptionalPathForPath(binary.baseInstaller.path)
	}
}

func (binary *binaryDecorator) hostToolPath() android.OptionalPath {
	return binary.toolPath
}
