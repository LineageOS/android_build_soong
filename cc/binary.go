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
	"path/filepath"

	"github.com/google/blueprint"

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

	// if set, install a symlink to the preferred architecture
	Symlink_preferred_arch *bool `android:"arch_variant"`

	// install symlinks to the binary.  Symlink names will have the suffix and the binary
	// extension (if any) appended
	Symlinks []string `android:"arch_variant"`

	DynamicLinker string `blueprint:"mutated"`

	// Names of modules to be overridden. Listed modules can only be other binaries
	// (in Make or Soong).
	// This does not completely prevent installation of the overridden binaries, but if both
	// binaries would be installed by default (in PRODUCT_PACKAGES) the other binary will be removed
	// from PRODUCT_PACKAGES.
	Overrides []string
}

func init() {
	android.RegisterModuleType("cc_binary", BinaryFactory)
	android.RegisterModuleType("cc_binary_host", binaryHostFactory)
}

// cc_binary produces a binary that is runnable on a device.
func BinaryFactory() android.Module {
	module, _ := NewBinary(android.HostAndDeviceSupported)
	return module.Init()
}

// cc_binary_host produces a binary that is runnable on a host.
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

	// Location of the linked, unstripped binary
	unstrippedOutputFile android.Path

	// Names of symlinks to be installed for use in LOCAL_MODULE_SYMLINKS
	symlinks []string

	// Output archive of gcno coverage information
	coverageOutputFile android.OptionalPath

	// Location of the file that should be copied to dist dir when requested
	distFile android.OptionalPath

	post_install_cmds []string
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
			deps.DynamicLinker = "linker"
			deps.LinkerFlagsFile = "host_bionic_linker_flags"
		}
	}

	if !binary.static() && inList("libc", deps.StaticLibs) {
		ctx.ModuleErrorf("statically linking libc to dynamic executable, please remove libc\n" +
			"from static libs or set static_executable: true")
	}

	return deps
}

func (binary *binaryDecorator) isDependencyRoot() bool {
	return true
}

func NewBinary(hod android.HostOrDeviceSupported) (*Module, *binaryDecorator) {
	module := newModule(hod, android.MultilibFirst)
	binary := &binaryDecorator{
		baseLinker:    NewBaseLinker(module.sanitize),
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
		} else if !ctx.Fuchsia() {
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

	if ctx.Host() && !ctx.Windows() && !binary.static() {
		if !ctx.Config().IsEnvTrue("DISABLE_HOST_PIE") {
			flags.LdFlags = append(flags.LdFlags, "-pie")
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
						if ctx.bootstrap() && !ctx.inRecovery() {
							flags.DynamicLinker = "/system/bin/bootstrap/linker"
						} else {
							flags.DynamicLinker = "/system/bin/linker"
						}
						if flags.Toolchain.Is64Bit() {
							flags.DynamicLinker += "64"
						}
					case android.LinuxBionic:
						flags.DynamicLinker = ""
					default:
						ctx.ModuleErrorf("unknown dynamic linker")
					}
				}

				if ctx.Os() == android.LinuxBionic {
					// Use the dlwrap entry point, but keep _start around so
					// that it can be used by host_bionic_inject
					flags.LdFlags = append(flags.LdFlags,
						"-Wl,--entry=__dlwrap__start",
						"-Wl,--undefined=_start",
					)
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

	fileName := binary.getStem(ctx) + flags.Toolchain.ExecutableSuffix()
	outputFile := android.PathForModuleOut(ctx, fileName)
	ret := outputFile

	var linkerDeps android.Paths

	if deps.LinkerFlagsFile.Valid() {
		flags.LdFlags = append(flags.LdFlags, "$$(cat "+deps.LinkerFlagsFile.String()+")")
		linkerDeps = append(linkerDeps, deps.LinkerFlagsFile.Path())
	}

	if flags.DynamicLinker != "" {
		flags.LdFlags = append(flags.LdFlags, "-Wl,-dynamic-linker,"+flags.DynamicLinker)
	} else if ctx.toolchain().Bionic() && !binary.static() {
		flags.LdFlags = append(flags.LdFlags, "-Wl,--no-dynamic-linker")
	}

	builderFlags := flagsToBuilderFlags(flags)

	if binary.stripper.needsStrip(ctx) {
		if ctx.Darwin() {
			builderFlags.stripUseGnuStrip = true
		}
		strippedOutputFile := outputFile
		outputFile = android.PathForModuleOut(ctx, "unstripped", fileName)
		binary.stripper.strip(ctx, outputFile, strippedOutputFile, builderFlags)
	}

	binary.unstrippedOutputFile = outputFile

	if String(binary.Properties.Prefix_symbols) != "" {
		afterPrefixSymbols := outputFile
		outputFile = android.PathForModuleOut(ctx, "unprefixed", fileName)
		TransformBinaryPrefixSymbols(ctx, String(binary.Properties.Prefix_symbols), outputFile,
			flagsToBuilderFlags(flags), afterPrefixSymbols)
	}

	if Bool(binary.baseLinker.Properties.Use_version_lib) {
		if ctx.Host() {
			versionedOutputFile := outputFile
			outputFile = android.PathForModuleOut(ctx, "unversioned", fileName)
			binary.injectVersionSymbol(ctx, outputFile, versionedOutputFile)
		} else {
			versionedOutputFile := android.PathForModuleOut(ctx, "versioned", fileName)
			binary.distFile = android.OptionalPathForPath(versionedOutputFile)

			if binary.stripper.needsStrip(ctx) {
				out := android.PathForModuleOut(ctx, "versioned-stripped", fileName)
				binary.distFile = android.OptionalPathForPath(out)
				binary.stripper.strip(ctx, versionedOutputFile, out, builderFlags)
			}

			binary.injectVersionSymbol(ctx, outputFile, versionedOutputFile)
		}
	}

	if ctx.Os() == android.LinuxBionic && !binary.static() {
		injectedOutputFile := outputFile
		outputFile = android.PathForModuleOut(ctx, "prelinker", fileName)

		if !deps.DynamicLinker.Valid() {
			panic("Non-static host bionic modules must have a dynamic linker")
		}

		binary.injectHostBionicLinkerSymbols(ctx, outputFile, deps.DynamicLinker.Path(), injectedOutputFile)
	}

	var sharedLibs android.Paths
	// Ignore shared libs for static executables.
	if !binary.static() {
		sharedLibs = deps.EarlySharedLibs
		sharedLibs = append(sharedLibs, deps.SharedLibs...)
		sharedLibs = append(sharedLibs, deps.LateSharedLibs...)
		linkerDeps = append(linkerDeps, deps.EarlySharedLibsDeps...)
		linkerDeps = append(linkerDeps, deps.SharedLibsDeps...)
		linkerDeps = append(linkerDeps, deps.LateSharedLibsDeps...)
	}

	linkerDeps = append(linkerDeps, objs.tidyFiles...)
	linkerDeps = append(linkerDeps, flags.LdFlagsDeps...)

	TransformObjToDynamicBinary(ctx, objs.objFiles, sharedLibs, deps.StaticLibs,
		deps.LateStaticLibs, deps.WholeStaticLibs, linkerDeps, deps.CrtBegin, deps.CrtEnd, true,
		builderFlags, outputFile)

	objs.coverageFiles = append(objs.coverageFiles, deps.StaticLibObjs.coverageFiles...)
	objs.coverageFiles = append(objs.coverageFiles, deps.WholeStaticLibObjs.coverageFiles...)
	binary.coverageOutputFile = TransformCoverageFilesToZip(ctx, objs, binary.getStem(ctx))

	// Need to determine symlinks early since some targets (ie APEX) need this
	// information but will not call 'install'
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

	return ret
}

func (binary *binaryDecorator) unstrippedOutputFilePath() android.Path {
	return binary.unstrippedOutputFile
}

func (binary *binaryDecorator) symlinkList() []string {
	return binary.symlinks
}

func (binary *binaryDecorator) nativeCoverage() bool {
	return true
}

func (binary *binaryDecorator) coverageOutputFilePath() android.OptionalPath {
	return binary.coverageOutputFile
}

// /system/bin/linker -> /apex/com.android.runtime/bin/linker
func (binary *binaryDecorator) installSymlinkToRuntimeApex(ctx ModuleContext, file android.Path) {
	dir := binary.baseInstaller.installDir(ctx)
	dirOnDevice := android.InstallPathToOnDevicePath(ctx, dir)
	target := "/" + filepath.Join("apex", "com.android.runtime", dir.Base(), file.Base())

	ctx.InstallAbsoluteSymlink(dir, file.Base(), target)
	binary.post_install_cmds = append(binary.post_install_cmds, makeSymlinkCmd(dirOnDevice, file.Base(), target))

	for _, symlink := range binary.symlinks {
		ctx.InstallAbsoluteSymlink(dir, symlink, target)
		binary.post_install_cmds = append(binary.post_install_cmds, makeSymlinkCmd(dirOnDevice, symlink, target))
	}
}

func (binary *binaryDecorator) install(ctx ModuleContext, file android.Path) {
	// Bionic binaries (e.g. linker) is installed to the bootstrap subdirectory.
	// The original path becomes a symlink to the corresponding file in the
	// runtime APEX.
	if installToBootstrap(ctx.baseModuleName(), ctx.Config()) && ctx.Arch().Native && ctx.apexName() == "" && !ctx.inRecovery() {
		if ctx.Device() && isBionic(ctx.baseModuleName()) {
			binary.installSymlinkToRuntimeApex(ctx, file)
		}
		binary.baseInstaller.subDir = "bootstrap"
	}
	binary.baseInstaller.install(ctx, file)
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

func init() {
	pctx.HostBinToolVariable("hostBionicSymbolsInjectCmd", "host_bionic_inject")
}

var injectHostBionicSymbols = pctx.AndroidStaticRule("injectHostBionicSymbols",
	blueprint.RuleParams{
		Command:     "$hostBionicSymbolsInjectCmd -i $in -l $linker -o $out",
		CommandDeps: []string{"$hostBionicSymbolsInjectCmd"},
	}, "linker")

func (binary *binaryDecorator) injectHostBionicLinkerSymbols(ctx ModuleContext, in, linker android.Path, out android.WritablePath) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        injectHostBionicSymbols,
		Description: "inject host bionic symbols",
		Input:       in,
		Implicit:    linker,
		Output:      out,
		Args: map[string]string{
			"linker": linker.String(),
		},
	})
}
