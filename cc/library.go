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
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"

	"android/soong/android"
)

type LibraryProperties struct {
	Static struct {
		Srcs   []string `android:"arch_variant"`
		Cflags []string `android:"arch_variant"`

		Enabled           *bool    `android:"arch_variant"`
		Whole_static_libs []string `android:"arch_variant"`
		Static_libs       []string `android:"arch_variant"`
		Shared_libs       []string `android:"arch_variant"`
	} `android:"arch_variant"`
	Shared struct {
		Srcs   []string `android:"arch_variant"`
		Cflags []string `android:"arch_variant"`

		Enabled           *bool    `android:"arch_variant"`
		Whole_static_libs []string `android:"arch_variant"`
		Static_libs       []string `android:"arch_variant"`
		Shared_libs       []string `android:"arch_variant"`
	} `android:"arch_variant"`

	// local file name to pass to the linker as --version_script
	Version_script *string `android:"arch_variant"`
	// local file name to pass to the linker as -unexported_symbols_list
	Unexported_symbols_list *string `android:"arch_variant"`
	// local file name to pass to the linker as -force_symbols_not_weak_list
	Force_symbols_not_weak_list *string `android:"arch_variant"`
	// local file name to pass to the linker as -force_symbols_weak_list
	Force_symbols_weak_list *string `android:"arch_variant"`

	// rename host libraries to prevent overlap with system installed libraries
	Unique_host_soname *bool

	Aidl struct {
		// export headers generated from .aidl sources
		Export_aidl_headers bool
	}

	Proto struct {
		// export headers generated from .proto sources
		Export_proto_headers bool
	}
}

type LibraryMutatedProperties struct {
	VariantName string `blueprint:"mutated"`

	// Build a static variant
	BuildStatic bool `blueprint:"mutated"`
	// Build a shared variant
	BuildShared bool `blueprint:"mutated"`
	// This variant is shared
	VariantIsShared bool `blueprint:"mutated"`
	// This variant is static
	VariantIsStatic bool `blueprint:"mutated"`
}

type FlagExporterProperties struct {
	// list of directories relative to the Blueprints file that will
	// be added to the include path (using -I) for this module and any module that links
	// against this module
	Export_include_dirs []string `android:"arch_variant"`
}

func init() {
	android.RegisterModuleType("cc_library_static", libraryStaticFactory)
	android.RegisterModuleType("cc_library_shared", librarySharedFactory)
	android.RegisterModuleType("cc_library", libraryFactory)
	android.RegisterModuleType("cc_library_host_static", libraryHostStaticFactory)
	android.RegisterModuleType("cc_library_host_shared", libraryHostSharedFactory)
	android.RegisterModuleType("cc_library_headers", libraryHeaderFactory)
}

// Module factory for combined static + shared libraries, device by default but with possible host
// support
func libraryFactory() (blueprint.Module, []interface{}) {
	module, _ := NewLibrary(android.HostAndDeviceSupported)
	return module.Init()
}

// Module factory for static libraries
func libraryStaticFactory() (blueprint.Module, []interface{}) {
	module, library := NewLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyStatic()
	return module.Init()
}

// Module factory for shared libraries
func librarySharedFactory() (blueprint.Module, []interface{}) {
	module, library := NewLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyShared()
	return module.Init()
}

// Module factory for host static libraries
func libraryHostStaticFactory() (blueprint.Module, []interface{}) {
	module, library := NewLibrary(android.HostSupported)
	library.BuildOnlyStatic()
	return module.Init()
}

// Module factory for host shared libraries
func libraryHostSharedFactory() (blueprint.Module, []interface{}) {
	module, library := NewLibrary(android.HostSupported)
	library.BuildOnlyShared()
	return module.Init()
}

// Module factory for header-only libraries
func libraryHeaderFactory() (blueprint.Module, []interface{}) {
	module, library := NewLibrary(android.HostAndDeviceSupported)
	library.HeaderOnly()
	return module.Init()
}

type flagExporter struct {
	Properties FlagExporterProperties

	flags     []string
	flagsDeps android.Paths
}

func (f *flagExporter) exportIncludes(ctx ModuleContext, inc string) {
	includeDirs := android.PathsForModuleSrc(ctx, f.Properties.Export_include_dirs)
	for _, dir := range includeDirs.Strings() {
		f.flags = append(f.flags, inc+dir)
	}
}

func (f *flagExporter) reexportFlags(flags []string) {
	f.flags = append(f.flags, flags...)
}

func (f *flagExporter) reexportDeps(deps android.Paths) {
	f.flagsDeps = append(f.flagsDeps, deps...)
}

func (f *flagExporter) exportedFlags() []string {
	return f.flags
}

func (f *flagExporter) exportedFlagsDeps() android.Paths {
	return f.flagsDeps
}

type exportedFlagsProducer interface {
	exportedFlags() []string
	exportedFlagsDeps() android.Paths
}

var _ exportedFlagsProducer = (*flagExporter)(nil)

// libraryDecorator wraps baseCompiler, baseLinker and baseInstaller to provide library-specific
// functionality: static vs. shared linkage, reusing object files for shared libraries
type libraryDecorator struct {
	Properties        LibraryProperties
	MutatedProperties LibraryMutatedProperties

	// For reusing static library objects for shared library
	reuseObjects Objects
	// table-of-contents file to optimize out relinking when possible
	tocFile android.OptionalPath

	flagExporter
	stripper
	relocationPacker

	// If we're used as a whole_static_lib, our missing dependencies need
	// to be given
	wholeStaticMissingDeps []string

	// For whole_static_libs
	objects Objects

	// Uses the module's name if empty, but can be overridden. Does not include
	// shlib suffix.
	libName string

	sanitize *sanitize

	// Output archive of gcno coverage information files
	coverageOutputFile android.OptionalPath

	// Decorated interafaces
	*baseCompiler
	*baseLinker
	*baseInstaller
}

func (library *libraryDecorator) linkerProps() []interface{} {
	var props []interface{}
	props = append(props, library.baseLinker.linkerProps()...)
	return append(props,
		&library.Properties,
		&library.MutatedProperties,
		&library.flagExporter.Properties,
		&library.stripper.StripProperties,
		&library.relocationPacker.Properties)
}

func (library *libraryDecorator) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = library.baseLinker.linkerFlags(ctx, flags)

	// MinGW spits out warnings about -fPIC even for -fpie?!) being ignored because
	// all code is position independent, and then those warnings get promoted to
	// errors.
	if !ctx.Windows() {
		flags.CFlags = append(flags.CFlags, "-fPIC")
	}

	if library.static() {
		flags.CFlags = append(flags.CFlags, library.Properties.Static.Cflags...)
	} else if library.shared() {
		flags.CFlags = append(flags.CFlags, library.Properties.Shared.Cflags...)
	}

	if library.shared() {
		libName := library.getLibName(ctx)
		// GCC for Android assumes that -shared means -Bsymbolic, use -Wl,-shared instead
		sharedFlag := "-Wl,-shared"
		if flags.Clang || ctx.Host() {
			sharedFlag = "-shared"
		}
		var f []string
		if ctx.toolchain().Bionic() {
			f = append(f,
				"-nostdlib",
				"-Wl,--gc-sections",
			)
		}

		if ctx.Darwin() {
			f = append(f,
				"-dynamiclib",
				"-single_module",
				"-install_name @rpath/"+libName+flags.Toolchain.ShlibSuffix(),
			)
			if ctx.Arch().ArchType == android.X86 {
				f = append(f,
					"-read_only_relocs suppress",
				)
			}
		} else {
			f = append(f,
				sharedFlag,
				"-Wl,-soname,"+libName+flags.Toolchain.ShlibSuffix())
		}

		flags.LdFlags = append(f, flags.LdFlags...)
	}

	return flags
}

func (library *libraryDecorator) compilerFlags(ctx ModuleContext, flags Flags) Flags {
	exportIncludeDirs := android.PathsForModuleSrc(ctx, library.flagExporter.Properties.Export_include_dirs)
	if len(exportIncludeDirs) > 0 {
		flags.GlobalFlags = append(flags.GlobalFlags, includeDirsToFlags(exportIncludeDirs))
	}

	return library.baseCompiler.compilerFlags(ctx, flags)
}

func (library *libraryDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) Objects {
	if !library.buildShared() && !library.buildStatic() {
		if len(library.baseCompiler.Properties.Srcs) > 0 {
			ctx.PropertyErrorf("srcs", "cc_library_headers must not have any srcs")
		}
		if len(library.Properties.Static.Srcs) > 0 {
			ctx.PropertyErrorf("static.srcs", "cc_library_headers must not have any srcs")
		}
		if len(library.Properties.Shared.Srcs) > 0 {
			ctx.PropertyErrorf("shared.srcs", "cc_library_headers must not have any srcs")
		}
		return Objects{}
	}

	objs := library.baseCompiler.compile(ctx, flags, deps)
	library.reuseObjects = objs
	buildFlags := flagsToBuilderFlags(flags)

	if library.static() {
		srcs := android.PathsForModuleSrc(ctx, library.Properties.Static.Srcs)
		objs = objs.Append(compileObjs(ctx, buildFlags, android.DeviceStaticLibrary,
			srcs, library.baseCompiler.deps))
	} else if library.shared() {
		srcs := android.PathsForModuleSrc(ctx, library.Properties.Shared.Srcs)
		objs = objs.Append(compileObjs(ctx, buildFlags, android.DeviceSharedLibrary,
			srcs, library.baseCompiler.deps))
	}

	return objs
}

type libraryInterface interface {
	getWholeStaticMissingDeps() []string
	static() bool
	objs() Objects
	reuseObjs() Objects
	toc() android.OptionalPath

	// Returns true if the build options for the module have selected a static or shared build
	buildStatic() bool
	buildShared() bool

	// Sets whether a specific variant is static or shared
	setStatic()
	setShared()
}

func (library *libraryDecorator) getLibName(ctx ModuleContext) string {
	name := library.libName
	if name == "" {
		name = ctx.baseModuleName()
	}

	if ctx.Host() && Bool(library.Properties.Unique_host_soname) {
		if !strings.HasSuffix(name, "-host") {
			name = name + "-host"
		}
	}

	return name + library.MutatedProperties.VariantName
}

func (library *libraryDecorator) linkerInit(ctx BaseModuleContext) {
	location := InstallInSystem
	if library.sanitize.inSanitizerDir() {
		location = InstallInSanitizerDir
	}
	library.baseInstaller.location = location

	library.baseLinker.linkerInit(ctx)

	library.relocationPacker.packingInit(ctx)
}

func (library *libraryDecorator) linkerDeps(ctx DepsContext, deps Deps) Deps {
	deps = library.baseLinker.linkerDeps(ctx, deps)

	if library.static() {
		deps.WholeStaticLibs = append(deps.WholeStaticLibs,
			library.Properties.Static.Whole_static_libs...)
		deps.StaticLibs = append(deps.StaticLibs, library.Properties.Static.Static_libs...)
		deps.SharedLibs = append(deps.SharedLibs, library.Properties.Static.Shared_libs...)
	} else if library.shared() {
		if ctx.toolchain().Bionic() && !Bool(library.baseLinker.Properties.Nocrt) {
			if !ctx.sdk() && !ctx.vndk() {
				deps.CrtBegin = "crtbegin_so"
				deps.CrtEnd = "crtend_so"
			} else {
				// TODO(danalbert): Add generation of crt objects.
				// For `sdk_version: "current"`, we don't actually have a
				// freshly generated set of CRT objects. Use the last stable
				// version.
				version := ctx.sdkVersion()
				if version == "current" {
					version = ctx.AConfig().PlatformSdkVersion()
				}
				deps.CrtBegin = "ndk_crtbegin_so." + version
				deps.CrtEnd = "ndk_crtend_so." + version
			}
		}
		deps.WholeStaticLibs = append(deps.WholeStaticLibs, library.Properties.Shared.Whole_static_libs...)
		deps.StaticLibs = append(deps.StaticLibs, library.Properties.Shared.Static_libs...)
		deps.SharedLibs = append(deps.SharedLibs, library.Properties.Shared.Shared_libs...)
	}

	return deps
}

func (library *libraryDecorator) linkStatic(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {

	library.objects = deps.WholeStaticLibObjs.Copy()
	library.objects = library.objects.Append(objs)

	outputFile := android.PathForModuleOut(ctx,
		ctx.ModuleName()+library.MutatedProperties.VariantName+staticLibraryExtension)
	builderFlags := flagsToBuilderFlags(flags)

	TransformObjToStaticLib(ctx, library.objects.objFiles, builderFlags, outputFile, objs.tidyFiles)

	library.coverageOutputFile = TransformCoverageFilesToLib(ctx, library.objects, builderFlags,
		ctx.ModuleName()+library.MutatedProperties.VariantName)

	library.wholeStaticMissingDeps = ctx.GetMissingDependencies()

	ctx.CheckbuildFile(outputFile)

	return outputFile
}

func (library *libraryDecorator) linkShared(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {

	var linkerDeps android.Paths

	versionScript := android.OptionalPathForModuleSrc(ctx, library.Properties.Version_script)
	unexportedSymbols := android.OptionalPathForModuleSrc(ctx, library.Properties.Unexported_symbols_list)
	forceNotWeakSymbols := android.OptionalPathForModuleSrc(ctx, library.Properties.Force_symbols_not_weak_list)
	forceWeakSymbols := android.OptionalPathForModuleSrc(ctx, library.Properties.Force_symbols_weak_list)
	if !ctx.Darwin() {
		if versionScript.Valid() {
			flags.LdFlags = append(flags.LdFlags, "-Wl,--version-script,"+versionScript.String())
			linkerDeps = append(linkerDeps, versionScript.Path())
		}
		if unexportedSymbols.Valid() {
			ctx.PropertyErrorf("unexported_symbols_list", "Only supported on Darwin")
		}
		if forceNotWeakSymbols.Valid() {
			ctx.PropertyErrorf("force_symbols_not_weak_list", "Only supported on Darwin")
		}
		if forceWeakSymbols.Valid() {
			ctx.PropertyErrorf("force_symbols_weak_list", "Only supported on Darwin")
		}
	} else {
		if versionScript.Valid() {
			ctx.PropertyErrorf("version_script", "Not supported on Darwin")
		}
		if unexportedSymbols.Valid() {
			flags.LdFlags = append(flags.LdFlags, "-Wl,-unexported_symbols_list,"+unexportedSymbols.String())
			linkerDeps = append(linkerDeps, unexportedSymbols.Path())
		}
		if forceNotWeakSymbols.Valid() {
			flags.LdFlags = append(flags.LdFlags, "-Wl,-force_symbols_not_weak_list,"+forceNotWeakSymbols.String())
			linkerDeps = append(linkerDeps, forceNotWeakSymbols.Path())
		}
		if forceWeakSymbols.Valid() {
			flags.LdFlags = append(flags.LdFlags, "-Wl,-force_symbols_weak_list,"+forceWeakSymbols.String())
			linkerDeps = append(linkerDeps, forceWeakSymbols.Path())
		}
	}

	fileName := library.getLibName(ctx) + flags.Toolchain.ShlibSuffix()
	outputFile := android.PathForModuleOut(ctx, fileName)
	ret := outputFile

	builderFlags := flagsToBuilderFlags(flags)

	if !ctx.Darwin() && !ctx.Windows() {
		// Optimize out relinking against shared libraries whose interface hasn't changed by
		// depending on a table of contents file instead of the library itself.
		tocPath := outputFile.RelPathString()
		tocPath = pathtools.ReplaceExtension(tocPath, flags.Toolchain.ShlibSuffix()[1:]+".toc")
		tocFile := android.PathForOutput(ctx, tocPath)
		library.tocFile = android.OptionalPathForPath(tocFile)
		TransformSharedObjectToToc(ctx, outputFile, tocFile, builderFlags)
	}

	if library.relocationPacker.needsPacking(ctx) {
		packedOutputFile := outputFile
		outputFile = android.PathForModuleOut(ctx, "unpacked", fileName)
		library.relocationPacker.pack(ctx, outputFile, packedOutputFile, builderFlags)
	}

	if library.stripper.needsStrip(ctx) {
		strippedOutputFile := outputFile
		outputFile = android.PathForModuleOut(ctx, "unstripped", fileName)
		library.stripper.strip(ctx, outputFile, strippedOutputFile, builderFlags)
	}

	sharedLibs := deps.SharedLibs
	sharedLibs = append(sharedLibs, deps.LateSharedLibs...)

	// TODO(danalbert): Clean this up when soong supports prebuilts.
	if strings.HasPrefix(ctx.selectedStl(), "ndk_libc++") {
		libDir := getNdkStlLibDir(ctx, flags.Toolchain, "libc++")

		if strings.HasSuffix(ctx.selectedStl(), "_shared") {
			deps.StaticLibs = append(deps.StaticLibs,
				libDir.Join(ctx, "libandroid_support.a"))
		} else {
			deps.StaticLibs = append(deps.StaticLibs,
				libDir.Join(ctx, "libc++abi.a"),
				libDir.Join(ctx, "libandroid_support.a"))
		}

		if ctx.Arch().ArchType == android.Arm {
			deps.StaticLibs = append(deps.StaticLibs,
				libDir.Join(ctx, "libunwind.a"))
		}
	}

	linkerDeps = append(linkerDeps, deps.SharedLibsDeps...)
	linkerDeps = append(linkerDeps, deps.LateSharedLibsDeps...)
	linkerDeps = append(linkerDeps, objs.tidyFiles...)

	TransformObjToDynamicBinary(ctx, objs.objFiles, sharedLibs,
		deps.StaticLibs, deps.LateStaticLibs, deps.WholeStaticLibs,
		linkerDeps, deps.CrtBegin, deps.CrtEnd, false, builderFlags, outputFile)

	objs.coverageFiles = append(objs.coverageFiles, deps.StaticLibObjs.coverageFiles...)
	objs.coverageFiles = append(objs.coverageFiles, deps.WholeStaticLibObjs.coverageFiles...)
	library.coverageOutputFile = TransformCoverageFilesToLib(ctx, objs, builderFlags, library.getLibName(ctx))

	return ret
}

func (library *libraryDecorator) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {

	objs = objs.Append(deps.Objs)

	var out android.Path
	if library.static() || library.header() {
		out = library.linkStatic(ctx, flags, deps, objs)
	} else {
		out = library.linkShared(ctx, flags, deps, objs)
	}

	library.exportIncludes(ctx, "-I")
	library.reexportFlags(deps.ReexportedFlags)
	library.reexportDeps(deps.ReexportedFlagsDeps)

	if library.Properties.Aidl.Export_aidl_headers {
		if library.baseCompiler.hasSrcExt(".aidl") {
			library.reexportFlags([]string{
				"-I" + android.PathForModuleGen(ctx, "aidl").String(),
			})
			library.reexportDeps(library.baseCompiler.deps) // TODO: restrict to aidl deps
		}
	}

	if library.Properties.Proto.Export_proto_headers {
		if library.baseCompiler.hasSrcExt(".proto") {
			library.reexportFlags([]string{
				"-I" + protoSubDir(ctx).String(),
				"-I" + protoDir(ctx).String(),
			})
			library.reexportDeps(library.baseCompiler.deps) // TODO: restrict to proto deps
		}
	}

	return out
}

func (library *libraryDecorator) buildStatic() bool {
	return library.MutatedProperties.BuildStatic &&
		(library.Properties.Static.Enabled == nil || *library.Properties.Static.Enabled)
}

func (library *libraryDecorator) buildShared() bool {
	return library.MutatedProperties.BuildShared &&
		(library.Properties.Shared.Enabled == nil || *library.Properties.Shared.Enabled)
}

func (library *libraryDecorator) getWholeStaticMissingDeps() []string {
	return library.wholeStaticMissingDeps
}

func (library *libraryDecorator) objs() Objects {
	return library.objects
}

func (library *libraryDecorator) reuseObjs() Objects {
	return library.reuseObjects
}

func (library *libraryDecorator) toc() android.OptionalPath {
	return library.tocFile
}

func (library *libraryDecorator) install(ctx ModuleContext, file android.Path) {
	if !ctx.static() {
		library.baseInstaller.install(ctx, file)
	}
}

func (library *libraryDecorator) static() bool {
	return library.MutatedProperties.VariantIsStatic
}

func (library *libraryDecorator) shared() bool {
	return library.MutatedProperties.VariantIsShared
}

func (library *libraryDecorator) header() bool {
	return !library.static() && !library.shared()
}

func (library *libraryDecorator) setStatic() {
	library.MutatedProperties.VariantIsStatic = true
	library.MutatedProperties.VariantIsShared = false
}

func (library *libraryDecorator) setShared() {
	library.MutatedProperties.VariantIsStatic = false
	library.MutatedProperties.VariantIsShared = true
}

func (library *libraryDecorator) BuildOnlyStatic() {
	library.MutatedProperties.BuildShared = false
}

func (library *libraryDecorator) BuildOnlyShared() {
	library.MutatedProperties.BuildStatic = false
}

func (library *libraryDecorator) HeaderOnly() {
	library.MutatedProperties.BuildShared = false
	library.MutatedProperties.BuildStatic = false
}

func NewLibrary(hod android.HostOrDeviceSupported) (*Module, *libraryDecorator) {
	module := newModule(hod, android.MultilibBoth)

	library := &libraryDecorator{
		MutatedProperties: LibraryMutatedProperties{
			BuildShared: true,
			BuildStatic: true,
		},
		baseCompiler:  NewBaseCompiler(),
		baseLinker:    NewBaseLinker(),
		baseInstaller: NewBaseInstaller("lib", "lib64", InstallInSystem),
		sanitize:      module.sanitize,
	}

	module.compiler = library
	module.linker = library
	module.installer = library

	return module, library
}

func linkageMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok && m.linker != nil {
		if library, ok := m.linker.(libraryInterface); ok {
			var modules []blueprint.Module
			if library.buildStatic() && library.buildShared() {
				modules = mctx.CreateLocalVariations("static", "shared")
				static := modules[0].(*Module)
				shared := modules[1].(*Module)

				static.linker.(libraryInterface).setStatic()
				shared.linker.(libraryInterface).setShared()

				if staticCompiler, ok := static.compiler.(*libraryDecorator); ok {
					sharedCompiler := shared.compiler.(*libraryDecorator)
					if len(staticCompiler.Properties.Static.Cflags) == 0 &&
						len(sharedCompiler.Properties.Shared.Cflags) == 0 {
						// Optimize out compiling common .o files twice for static+shared libraries
						mctx.AddInterVariantDependency(reuseObjTag, shared, static)
						sharedCompiler.baseCompiler.Properties.Srcs = nil
						sharedCompiler.baseCompiler.Properties.Generated_sources = nil
					}
				}
			} else if library.buildStatic() {
				modules = mctx.CreateLocalVariations("static")
				modules[0].(*Module).linker.(libraryInterface).setStatic()
			} else if library.buildShared() {
				modules = mctx.CreateLocalVariations("shared")
				modules[0].(*Module).linker.(libraryInterface).setShared()
			}
		}
	}
}
