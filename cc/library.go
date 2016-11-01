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

	Proto struct {
		// export headers generated from .proto sources
		Export_proto_headers bool
	}

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
	// be added to the include path using -I for any module that links against this module
	Export_include_dirs []string `android:"arch_variant"`
}

func init() {
	android.RegisterModuleType("cc_library_static", libraryStaticFactory)
	android.RegisterModuleType("cc_library_shared", librarySharedFactory)
	android.RegisterModuleType("cc_library", libraryFactory)
	android.RegisterModuleType("cc_library_host_static", libraryHostStaticFactory)
	android.RegisterModuleType("cc_library_host_shared", libraryHostSharedFactory)
}

// Module factory for combined static + shared libraries, device by default but with possible host
// support
func libraryFactory() (blueprint.Module, []interface{}) {
	module, _ := NewLibrary(android.HostAndDeviceSupported, true, true)
	return module.Init()
}

// Module factory for static libraries
func libraryStaticFactory() (blueprint.Module, []interface{}) {
	module, _ := NewLibrary(android.HostAndDeviceSupported, false, true)
	return module.Init()
}

// Module factory for shared libraries
func librarySharedFactory() (blueprint.Module, []interface{}) {
	module, _ := NewLibrary(android.HostAndDeviceSupported, true, false)
	return module.Init()
}

// Module factory for host static libraries
func libraryHostStaticFactory() (blueprint.Module, []interface{}) {
	module, _ := NewLibrary(android.HostSupported, false, true)
	return module.Init()
}

// Module factory for host shared libraries
func libraryHostSharedFactory() (blueprint.Module, []interface{}) {
	module, _ := NewLibrary(android.HostSupported, true, false)
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
	Properties LibraryProperties

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
		&library.flagExporter.Properties,
		&library.stripper.StripProperties,
		&library.relocationPacker.Properties)
}

func (library *libraryDecorator) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = library.baseLinker.linkerFlags(ctx, flags)

	// MinGW spits out warnings about -fPIC even for -fpie?!) being ignored because
	// all code is position independent, and then those warnings get promoted to
	// errors.
	if ctx.Os() != android.Windows {
		flags.CFlags = append(flags.CFlags, "-fPIC")
	}

	if library.static() {
		flags.CFlags = append(flags.CFlags, library.Properties.Static.Cflags...)
	} else {
		flags.CFlags = append(flags.CFlags, library.Properties.Shared.Cflags...)
	}

	if !library.static() {
		libName := library.getLibName(ctx)
		// GCC for Android assumes that -shared means -Bsymbolic, use -Wl,-shared instead
		sharedFlag := "-Wl,-shared"
		if flags.Clang || ctx.Host() {
			sharedFlag = "-shared"
		}
		var f []string
		if ctx.Device() {
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

func (library *libraryDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) Objects {
	objs := library.baseCompiler.compile(ctx, flags, deps)
	library.reuseObjects = objs
	buildFlags := flagsToBuilderFlags(flags)

	if library.static() {
		srcs := android.PathsForModuleSrc(ctx, library.Properties.Static.Srcs)
		objs = objs.Append(compileObjs(ctx, buildFlags, android.DeviceStaticLibrary,
			srcs, library.baseCompiler.deps))
	} else {
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
	setStatic(bool)
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

	return name + library.Properties.VariantName
}

func (library *libraryDecorator) linkerInit(ctx BaseModuleContext) {
	location := InstallInSystem
	if library.sanitize.inData() {
		location = InstallInData
	}
	library.baseInstaller.location = location

	library.baseLinker.linkerInit(ctx)

	library.relocationPacker.packingInit(ctx)
}

func (library *libraryDecorator) linkerDeps(ctx BaseModuleContext, deps Deps) Deps {
	deps = library.baseLinker.linkerDeps(ctx, deps)

	if library.static() {
		deps.WholeStaticLibs = append(deps.WholeStaticLibs,
			library.Properties.Static.Whole_static_libs...)
		deps.StaticLibs = append(deps.StaticLibs, library.Properties.Static.Static_libs...)
		deps.SharedLibs = append(deps.SharedLibs, library.Properties.Static.Shared_libs...)
	} else {
		if ctx.Device() && !Bool(library.baseLinker.Properties.Nocrt) {
			if !ctx.sdk() {
				deps.CrtBegin = "crtbegin_so"
				deps.CrtEnd = "crtend_so"
			} else {
				deps.CrtBegin = "ndk_crtbegin_so." + ctx.sdkVersion()
				deps.CrtEnd = "ndk_crtend_so." + ctx.sdkVersion()
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
		ctx.ModuleName()+library.Properties.VariantName+staticLibraryExtension)

	if ctx.Darwin() {
		TransformDarwinObjToStaticLib(ctx, library.objects.objFiles, flagsToBuilderFlags(flags), outputFile, objs.tidyFiles)
	} else {
		TransformObjToStaticLib(ctx, library.objects.objFiles, flagsToBuilderFlags(flags), outputFile, objs.tidyFiles)
	}

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

	if !ctx.Darwin() {
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

	return ret
}

func (library *libraryDecorator) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {

	objs = objs.Append(deps.Objs)

	var out android.Path
	if library.static() {
		out = library.linkStatic(ctx, flags, deps, objs)
	} else {
		out = library.linkShared(ctx, flags, deps, objs)
	}

	library.exportIncludes(ctx, "-I")
	library.reexportFlags(deps.ReexportedFlags)
	library.reexportDeps(deps.ReexportedFlagsDeps)

	if library.baseCompiler.hasProto() {
		if library.Properties.Proto.Export_proto_headers {
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
	return library.Properties.BuildStatic &&
		(library.Properties.Static.Enabled == nil || *library.Properties.Static.Enabled)
}

func (library *libraryDecorator) buildShared() bool {
	return library.Properties.BuildShared &&
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
	return library.Properties.VariantIsStatic
}

func (library *libraryDecorator) setStatic(static bool) {
	library.Properties.VariantIsStatic = static
}

func NewLibrary(hod android.HostOrDeviceSupported, shared, static bool) (*Module, *libraryDecorator) {
	module := newModule(hod, android.MultilibBoth)

	library := &libraryDecorator{
		Properties: LibraryProperties{
			BuildShared: shared,
			BuildStatic: static,
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

				static.linker.(libraryInterface).setStatic(true)
				shared.linker.(libraryInterface).setStatic(false)

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
				modules[0].(*Module).linker.(libraryInterface).setStatic(true)
			} else if library.buildShared() {
				modules = mctx.CreateLocalVariations("shared")
				modules[0].(*Module).linker.(libraryInterface).setStatic(false)
			}
		}
	}
}
