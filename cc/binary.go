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

	"android/soong/android"
	"github.com/google/blueprint"
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

	// override the dynamic linker
	DynamicLinker string `blueprint:"mutated"`

	// Names of modules to be overridden. Listed modules can only be other binaries
	// (in Make or Soong).
	// This does not completely prevent installation of the overridden binaries, but if both
	// binaries would be installed by default (in PRODUCT_PACKAGES) the other binary will be removed
	// from PRODUCT_PACKAGES.
	Overrides []string

	// Inject boringssl hash into the shared library.  This is only intended for use by external/boringssl.
	Inject_bssl_hash *bool `android:"arch_variant"`
}

func init() {
	RegisterBinaryBuildComponents(android.InitRegistrationContext)
}

func RegisterBinaryBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("cc_binary", BinaryFactory)
	ctx.RegisterModuleType("cc_binary_host", BinaryHostFactory)
}

// cc_binary produces a binary that is runnable on a device.
func BinaryFactory() android.Module {
	module, _ := newBinary(android.HostAndDeviceSupported)
	return module.Init()
}

// cc_binary_host produces a binary that is runnable on a host.
func BinaryHostFactory() android.Module {
	module, _ := newBinary(android.HostSupported)
	return module.Init()
}

//
// Executables
//

// binaryDecorator is a decorator containing information for C++ binary modules.
type binaryDecorator struct {
	*baseLinker
	*baseInstaller
	stripper Stripper

	Properties BinaryLinkerProperties

	toolPath android.OptionalPath

	// Location of the linked, unstripped binary
	unstrippedOutputFile android.Path

	// Names of symlinks to be installed for use in LOCAL_MODULE_SYMLINKS
	symlinks []string

	// If the module has symlink_preferred_arch set, the name of the symlink to the
	// binary for the preferred arch.
	preferredArchSymlink string

	// Output archive of gcno coverage information
	coverageOutputFile android.OptionalPath

	// Location of the files that should be copied to dist dir when requested
	distFiles android.TaggedDistFiles

	// Action command lines to run directly after the binary is installed. For example,
	// may be used to symlink runtime dependencies (such as bionic) alongside installation.
	postInstallCmds []string
}

var _ linker = (*binaryDecorator)(nil)

// linkerProps returns the list of individual properties objects relevant
// for this binary.
func (binary *binaryDecorator) linkerProps() []interface{} {
	return append(binary.baseLinker.linkerProps(),
		&binary.Properties,
		&binary.stripper.StripProperties)

}

// getStemWithoutSuffix returns the main section of the name to use for the symlink of
// the main output file of this binary module. This may be derived from the module name
// or other property overrides.
// For the full symlink name, the `Suffix` property of a binary module must be appended.
func (binary *binaryDecorator) getStemWithoutSuffix(ctx BaseModuleContext) string {
	stem := ctx.baseModuleName()
	if String(binary.Properties.Stem) != "" {
		stem = String(binary.Properties.Stem)
	}

	return stem
}

// getStem returns the full name to use for the symlink of the main output file of this binary
// module. This may be derived from the module name and/or other property overrides.
func (binary *binaryDecorator) getStem(ctx BaseModuleContext) string {
	return binary.getStemWithoutSuffix(ctx) + String(binary.Properties.Suffix)
}

// linkerDeps augments and returns the given `deps` to contain dependencies on
// modules common to most binaries, such as bionic libraries.
func (binary *binaryDecorator) linkerDeps(ctx DepsContext, deps Deps) Deps {
	deps = binary.baseLinker.linkerDeps(ctx, deps)
	if binary.baseLinker.Properties.crt() {
		if binary.static() {
			deps.CrtBegin = ctx.toolchain().CrtBeginStaticBinary()
			deps.CrtEnd = ctx.toolchain().CrtEndStaticBinary()
		} else {
			deps.CrtBegin = ctx.toolchain().CrtBeginSharedBinary()
			deps.CrtEnd = ctx.toolchain().CrtEndSharedBinary()
		}
	}

	if binary.static() {
		deps.StaticLibs = append(deps.StaticLibs, deps.SystemSharedLibs...)
	}

	if ctx.toolchain().Bionic() {
		if binary.static() {
			if ctx.selectedStl() == "libc++_static" {
				deps.StaticLibs = append(deps.StaticLibs, "libm", "libc")
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
		}
	}

	if !binary.static() && inList("libc", deps.StaticLibs) {
		ctx.ModuleErrorf("statically linking libc to dynamic executable, please remove libc\n" +
			"from static libs or set static_executable: true")
	}

	return deps
}

// NewBinary builds and returns a new Module corresponding to a C++ binary.
// Individual module implementations which comprise a C++ binary should call this function,
// set some fields on the result, and then call the Init function.
func NewBinary(hod android.HostOrDeviceSupported) (*Module, *binaryDecorator) {
	return newBinary(hod)
}

func newBinary(hod android.HostOrDeviceSupported) (*Module, *binaryDecorator) {
	module := newModule(hod, android.MultilibFirst)
	binary := &binaryDecorator{
		baseLinker:    NewBaseLinker(module.sanitize),
		baseInstaller: NewBaseInstaller("bin", "", InstallInSystem),
	}
	module.compiler = NewBaseCompiler()
	module.linker = binary
	module.installer = binary

	// Allow module to be added as member of an sdk/module_exports.
	module.sdkMemberTypes = []android.SdkMemberType{
		ccBinarySdkMemberType,
	}
	return module, binary
}

// linkerInit initializes dynamic properties of the linker (such as runpath) based
// on properties of this binary.
func (binary *binaryDecorator) linkerInit(ctx BaseModuleContext) {
	binary.baseLinker.linkerInit(ctx)

	if ctx.Os().Linux() && ctx.Host() {
		// Unless explicitly specified otherwise, host static binaries are built with -static
		// if HostStaticBinaries is true for the product configuration.
		if binary.Properties.Static_executable == nil && ctx.Config().HostStaticBinaries() {
			binary.Properties.Static_executable = BoolPtr(true)
		}
	}

	if ctx.Darwin() || ctx.Windows() {
		// Static executables are not supported on Darwin or Windows
		binary.Properties.Static_executable = nil
	}
}

func (binary *binaryDecorator) static() bool {
	return Bool(binary.Properties.Static_executable)
}

func (binary *binaryDecorator) staticBinary() bool {
	return binary.static()
}

func (binary *binaryDecorator) binary() bool {
	return true
}

// linkerFlags returns a Flags object containing linker flags that are defined
// by this binary, or that are implied by attributes of this binary. These flags are
// combined with the given flags.
func (binary *binaryDecorator) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = binary.baseLinker.linkerFlags(ctx, flags)

	// Passing -pie to clang for Windows binaries causes a warning that -pie is unused.
	if ctx.Host() && !ctx.Windows() && !binary.static() {
		if !ctx.Config().IsEnvTrue("DISABLE_HOST_PIE") {
			flags.Global.LdFlags = append(flags.Global.LdFlags, "-pie")
		}
	}

	// MinGW spits out warnings about -fPIC even for -fpie?!) being ignored because
	// all code is position independent, and then those warnings get promoted to
	// errors.
	if !ctx.Windows() {
		flags.Global.CFlags = append(flags.Global.CFlags, "-fPIE")
	}

	if ctx.toolchain().Bionic() {
		if binary.static() {
			// Clang driver needs -static to create static executable.
			// However, bionic/linker uses -shared to overwrite.
			// Linker for x86 targets does not allow coexistance of -static and -shared,
			// so we add -static only if -shared is not used.
			if !inList("-shared", flags.Local.LdFlags) {
				flags.Global.LdFlags = append(flags.Global.LdFlags, "-static")
			}

			flags.Global.LdFlags = append(flags.Global.LdFlags,
				"-nostdlib",
				"-Bstatic",
				"-Wl,--gc-sections",
			)
		} else { // not static
			if flags.DynamicLinker == "" {
				if binary.Properties.DynamicLinker != "" {
					flags.DynamicLinker = binary.Properties.DynamicLinker
				} else {
					switch ctx.Os() {
					case android.Android:
						if ctx.bootstrap() && !ctx.inRecovery() && !ctx.inRamdisk() && !ctx.inVendorRamdisk() {
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
					flags.Global.LdFlags = append(flags.Global.LdFlags,
						"-Wl,--entry=__dlwrap__start",
						"-Wl,--undefined=_start",
					)
				}
			}

			flags.Global.LdFlags = append(flags.Global.LdFlags,
				"-pie",
				"-nostdlib",
				"-Bdynamic",
				"-Wl,--gc-sections",
				"-Wl,-z,nocopyreloc",
			)
		}
	} else { // not bionic
		if binary.static() {
			flags.Global.LdFlags = append(flags.Global.LdFlags, "-static")
		}
		if ctx.Darwin() {
			flags.Global.LdFlags = append(flags.Global.LdFlags, "-Wl,-headerpad_max_install_names")
		}
	}

	return flags
}

// link registers actions to link this binary, and sets various fields
// on this binary to reflect information that should be exported up the build
// tree (for example, exported flags and include paths).
func (binary *binaryDecorator) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {

	fileName := binary.getStem(ctx) + flags.Toolchain.ExecutableSuffix()
	outputFile := android.PathForModuleOut(ctx, fileName)
	ret := outputFile

	var linkerDeps android.Paths

	if flags.DynamicLinker != "" {
		flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,-dynamic-linker,"+flags.DynamicLinker)
	} else if (ctx.toolchain().Bionic() || ctx.toolchain().Musl()) && !binary.static() {
		flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,--no-dynamic-linker")
	}

	if ctx.Darwin() && deps.DarwinSecondArchOutput.Valid() {
		fatOutputFile := outputFile
		outputFile = android.PathForModuleOut(ctx, "pre-fat", fileName)
		transformDarwinUniversalBinary(ctx, fatOutputFile, outputFile, deps.DarwinSecondArchOutput.Path())
	}

	builderFlags := flagsToBuilderFlags(flags)
	stripFlags := flagsToStripFlags(flags)
	if binary.stripper.NeedsStrip(ctx) {
		if ctx.Darwin() {
			stripFlags.StripUseGnuStrip = true
		}
		strippedOutputFile := outputFile
		outputFile = android.PathForModuleOut(ctx, "unstripped", fileName)
		binary.stripper.StripExecutableOrSharedLib(ctx, outputFile, strippedOutputFile, stripFlags)
	}

	binary.unstrippedOutputFile = outputFile

	if String(binary.Properties.Prefix_symbols) != "" {
		afterPrefixSymbols := outputFile
		outputFile = android.PathForModuleOut(ctx, "unprefixed", fileName)
		transformBinaryPrefixSymbols(ctx, String(binary.Properties.Prefix_symbols), outputFile,
			builderFlags, afterPrefixSymbols)
	}

	outputFile = maybeInjectBoringSSLHash(ctx, outputFile, binary.Properties.Inject_bssl_hash, fileName)

	// If use_version_lib is true, make an android::build::GetBuildNumber() function available.
	if Bool(binary.baseLinker.Properties.Use_version_lib) {
		if ctx.Host() {
			versionedOutputFile := outputFile
			outputFile = android.PathForModuleOut(ctx, "unversioned", fileName)
			binary.injectVersionSymbol(ctx, outputFile, versionedOutputFile)
		} else {
			// When dist'ing a library or binary that has use_version_lib set, always
			// distribute the stamped version, even for the device.
			versionedOutputFile := android.PathForModuleOut(ctx, "versioned", fileName)
			binary.distFiles = android.MakeDefaultDistFiles(versionedOutputFile)

			if binary.stripper.NeedsStrip(ctx) {
				out := android.PathForModuleOut(ctx, "versioned-stripped", fileName)
				binary.distFiles = android.MakeDefaultDistFiles(out)
				binary.stripper.StripExecutableOrSharedLib(ctx, versionedOutputFile, out, stripFlags)
			}

			binary.injectVersionSymbol(ctx, outputFile, versionedOutputFile)
		}
	}

	var validations android.Paths

	// Handle host bionic linker symbols.
	if ctx.Os() == android.LinuxBionic && !binary.static() {
		verifyFile := android.PathForModuleOut(ctx, "host_bionic_verify.stamp")

		if !deps.DynamicLinker.Valid() {
			panic("Non-static host bionic modules must have a dynamic linker")
		}

		binary.verifyHostBionicLinker(ctx, outputFile, deps.DynamicLinker.Path(), verifyFile)
		validations = append(validations, verifyFile)
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
		linkerDeps = append(linkerDeps, ndkSharedLibDeps(ctx)...)
	}

	validations = append(validations, objs.tidyDepFiles...)
	linkerDeps = append(linkerDeps, flags.LdFlagsDeps...)

	// Register link action.
	transformObjToDynamicBinary(ctx, objs.objFiles, sharedLibs, deps.StaticLibs,
		deps.LateStaticLibs, deps.WholeStaticLibs, linkerDeps, deps.CrtBegin, deps.CrtEnd, true,
		builderFlags, outputFile, nil, validations)

	objs.coverageFiles = append(objs.coverageFiles, deps.StaticLibObjs.coverageFiles...)
	objs.coverageFiles = append(objs.coverageFiles, deps.WholeStaticLibObjs.coverageFiles...)
	binary.coverageOutputFile = transformCoverageFilesToZip(ctx, objs, binary.getStem(ctx))

	// Need to determine symlinks early since some targets (ie APEX) need this
	// information but will not call 'install'
	binary.setSymlinkList(ctx)

	return ret
}

func (binary *binaryDecorator) unstrippedOutputFilePath() android.Path {
	return binary.unstrippedOutputFile
}

func (binary *binaryDecorator) strippedAllOutputFilePath() android.Path {
	panic("Not implemented.")
}

func (binary *binaryDecorator) setSymlinkList(ctx ModuleContext) {
	for _, symlink := range binary.Properties.Symlinks {
		binary.symlinks = append(binary.symlinks,
			symlink+String(binary.Properties.Suffix)+ctx.toolchain().ExecutableSuffix())
	}

	if Bool(binary.Properties.Symlink_preferred_arch) {
		if String(binary.Properties.Suffix) == "" {
			ctx.PropertyErrorf("symlink_preferred_arch", "must also specify suffix")
		}
		if ctx.TargetPrimary() {
			// Install a symlink to the preferred architecture
			symlinkName := binary.getStemWithoutSuffix(ctx)
			binary.symlinks = append(binary.symlinks, symlinkName)
			binary.preferredArchSymlink = symlinkName
		}
	}
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
	binary.postInstallCmds = append(binary.postInstallCmds, makeSymlinkCmd(dirOnDevice, file.Base(), target))

	for _, symlink := range binary.symlinks {
		ctx.InstallAbsoluteSymlink(dir, symlink, target)
		binary.postInstallCmds = append(binary.postInstallCmds, makeSymlinkCmd(dirOnDevice, symlink, target))
	}
}

func (binary *binaryDecorator) install(ctx ModuleContext, file android.Path) {
	// Bionic binaries (e.g. linker) is installed to the bootstrap subdirectory.
	// The original path becomes a symlink to the corresponding file in the
	// runtime APEX.
	translatedArch := ctx.Target().NativeBridge == android.NativeBridgeEnabled
	if InstallToBootstrap(ctx.baseModuleName(), ctx.Config()) && !ctx.Host() && ctx.directlyInAnyApex() &&
		!translatedArch && ctx.apexVariationName() == "" && !ctx.inRamdisk() && !ctx.inRecovery() &&
		!ctx.inVendorRamdisk() {

		if ctx.Device() && isBionic(ctx.baseModuleName()) {
			binary.installSymlinkToRuntimeApex(ctx, file)
		}
		binary.baseInstaller.subDir = "bootstrap"
	}
	binary.baseInstaller.install(ctx, file)

	var preferredArchSymlinkPath android.OptionalPath
	for _, symlink := range binary.symlinks {
		installedSymlink := ctx.InstallSymlink(binary.baseInstaller.installDir(ctx), symlink,
			binary.baseInstaller.path)
		if symlink == binary.preferredArchSymlink {
			// If this is the preferred arch symlink, save the installed path for use as the
			// tool path.
			preferredArchSymlinkPath = android.OptionalPathForPath(installedSymlink)
		}
	}

	if ctx.Os().Class == android.Host {
		// If the binary is multilib with a symlink to the preferred architecture, use the
		// symlink instead of the binary because that's the more "canonical" name.
		if preferredArchSymlinkPath.Valid() {
			binary.toolPath = preferredArchSymlinkPath
		} else {
			binary.toolPath = android.OptionalPathForPath(binary.baseInstaller.path)
		}
	}
}

func (binary *binaryDecorator) hostToolPath() android.OptionalPath {
	return binary.toolPath
}

func (binary *binaryDecorator) overriddenModules() []string {
	return binary.Properties.Overrides
}

func (binary *binaryDecorator) moduleInfoJSON(ctx ModuleContext, moduleInfoJSON *android.ModuleInfoJSON) {
	moduleInfoJSON.Class = []string{"EXECUTABLES"}
	binary.baseLinker.moduleInfoJSON(ctx, moduleInfoJSON)
}

var _ overridable = (*binaryDecorator)(nil)

func init() {
	pctx.HostBinToolVariable("verifyHostBionicCmd", "host_bionic_verify")
}

var verifyHostBionic = pctx.AndroidStaticRule("verifyHostBionic",
	blueprint.RuleParams{
		Command:     "$verifyHostBionicCmd -i $in -l $linker && touch $out",
		CommandDeps: []string{"$verifyHostBionicCmd"},
	}, "linker")

func (binary *binaryDecorator) verifyHostBionicLinker(ctx ModuleContext, in, linker android.Path, out android.WritablePath) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        verifyHostBionic,
		Description: "verify host bionic",
		Input:       in,
		Implicit:    linker,
		Output:      out,
		Args: map[string]string{
			"linker": linker.String(),
		},
	})
}
