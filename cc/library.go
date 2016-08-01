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

	"android/soong"
	"android/soong/android"
)

type LibraryCompilerProperties struct {
	Static struct {
		Srcs         []string `android:"arch_variant"`
		Exclude_srcs []string `android:"arch_variant"`
		Cflags       []string `android:"arch_variant"`
	} `android:"arch_variant"`
	Shared struct {
		Srcs         []string `android:"arch_variant"`
		Exclude_srcs []string `android:"arch_variant"`
		Cflags       []string `android:"arch_variant"`
	} `android:"arch_variant"`
}

type FlagExporterProperties struct {
	// list of directories relative to the Blueprints file that will
	// be added to the include path using -I for any module that links against this module
	Export_include_dirs []string `android:"arch_variant"`
}

type LibraryLinkerProperties struct {
	Static struct {
		Enabled           *bool    `android:"arch_variant"`
		Whole_static_libs []string `android:"arch_variant"`
		Static_libs       []string `android:"arch_variant"`
		Shared_libs       []string `android:"arch_variant"`
	} `android:"arch_variant"`
	Shared struct {
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

	VariantName string `blueprint:"mutated"`
}

func init() {
	soong.RegisterModuleType("cc_library_static", libraryStaticFactory)
	soong.RegisterModuleType("cc_library_shared", librarySharedFactory)
	soong.RegisterModuleType("cc_library", libraryFactory)
	soong.RegisterModuleType("cc_library_host_static", libraryHostStaticFactory)
	soong.RegisterModuleType("cc_library_host_shared", libraryHostSharedFactory)
}

// Module factory for combined static + shared libraries, device by default but with possible host
// support
func libraryFactory() (blueprint.Module, []interface{}) {
	module := NewLibrary(android.HostAndDeviceSupported, true, true)
	return module.Init()
}

// Module factory for static libraries
func libraryStaticFactory() (blueprint.Module, []interface{}) {
	module := NewLibrary(android.HostAndDeviceSupported, false, true)
	return module.Init()
}

// Module factory for shared libraries
func librarySharedFactory() (blueprint.Module, []interface{}) {
	module := NewLibrary(android.HostAndDeviceSupported, true, false)
	return module.Init()
}

// Module factory for host static libraries
func libraryHostStaticFactory() (blueprint.Module, []interface{}) {
	module := NewLibrary(android.HostSupported, false, true)
	return module.Init()
}

// Module factory for host shared libraries
func libraryHostSharedFactory() (blueprint.Module, []interface{}) {
	module := NewLibrary(android.HostSupported, true, false)
	return module.Init()
}

type flagExporter struct {
	Properties FlagExporterProperties

	flags []string
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

func (f *flagExporter) exportedFlags() []string {
	return f.flags
}

type exportedFlagsProducer interface {
	exportedFlags() []string
}

var _ exportedFlagsProducer = (*flagExporter)(nil)

type libraryCompiler struct {
	baseCompiler

	linker     *libraryLinker
	Properties LibraryCompilerProperties

	// For reusing static library objects for shared library
	reuseObjFiles android.Paths
}

var _ compiler = (*libraryCompiler)(nil)

func (library *libraryCompiler) compilerProps() []interface{} {
	props := library.baseCompiler.compilerProps()
	return append(props, &library.Properties)
}

func (library *libraryCompiler) compilerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = library.baseCompiler.compilerFlags(ctx, flags)

	// MinGW spits out warnings about -fPIC even for -fpie?!) being ignored because
	// all code is position independent, and then those warnings get promoted to
	// errors.
	if ctx.Os() != android.Windows {
		flags.CFlags = append(flags.CFlags, "-fPIC")
	}

	if library.linker.static() {
		flags.CFlags = append(flags.CFlags, library.Properties.Static.Cflags...)
	} else {
		flags.CFlags = append(flags.CFlags, library.Properties.Shared.Cflags...)
	}

	return flags
}

func (library *libraryCompiler) compile(ctx ModuleContext, flags Flags, deps PathDeps) android.Paths {
	var objFiles android.Paths

	objFiles = library.baseCompiler.compile(ctx, flags, deps)
	library.reuseObjFiles = objFiles

	pathDeps := deps.GeneratedHeaders
	pathDeps = append(pathDeps, ndkPathDeps(ctx)...)

	if library.linker.static() {
		objFiles = append(objFiles, library.compileObjs(ctx, flags, android.DeviceStaticLibrary,
			library.Properties.Static.Srcs, library.Properties.Static.Exclude_srcs,
			nil, pathDeps)...)
	} else {
		objFiles = append(objFiles, library.compileObjs(ctx, flags, android.DeviceSharedLibrary,
			library.Properties.Shared.Srcs, library.Properties.Shared.Exclude_srcs,
			nil, pathDeps)...)
	}

	return objFiles
}

type libraryLinker struct {
	baseLinker
	flagExporter
	stripper

	Properties LibraryLinkerProperties

	dynamicProperties struct {
		BuildStatic bool `blueprint:"mutated"`
		BuildShared bool `blueprint:"mutated"`
	}

	// If we're used as a whole_static_lib, our missing dependencies need
	// to be given
	wholeStaticMissingDeps []string

	// For whole_static_libs
	objFiles android.Paths

	// Uses the module's name if empty, but can be overridden. Does not include
	// shlib suffix.
	libName string
}

var _ linker = (*libraryLinker)(nil)

type libraryInterface interface {
	getWholeStaticMissingDeps() []string
	static() bool
	objs() android.Paths
}

func (library *libraryLinker) linkerProps() []interface{} {
	props := library.baseLinker.linkerProps()
	return append(props,
		&library.Properties,
		&library.dynamicProperties,
		&library.flagExporter.Properties,
		&library.stripper.StripProperties)
}

func (library *libraryLinker) getLibName(ctx ModuleContext) string {
	name := library.libName
	if name == "" {
		name = ctx.ModuleName()
	}

	if ctx.Host() && Bool(library.Properties.Unique_host_soname) {
		if !strings.HasSuffix(name, "-host") {
			name = name + "-host"
		}
	}

	return name + library.Properties.VariantName
}

func (library *libraryLinker) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = library.baseLinker.linkerFlags(ctx, flags)

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
				//"-read_only_relocs suppress",
				"-install_name @rpath/"+libName+flags.Toolchain.ShlibSuffix(),
			)
		} else {
			f = append(f,
				sharedFlag,
				"-Wl,-soname,"+libName+flags.Toolchain.ShlibSuffix())
		}

		flags.LdFlags = append(f, flags.LdFlags...)
	}

	return flags
}

func (library *libraryLinker) linkerDeps(ctx BaseModuleContext, deps Deps) Deps {
	deps = library.baseLinker.linkerDeps(ctx, deps)
	if library.static() {
		deps.WholeStaticLibs = append(deps.WholeStaticLibs, library.Properties.Static.Whole_static_libs...)
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

func (library *libraryLinker) linkStatic(ctx ModuleContext,
	flags Flags, deps PathDeps, objFiles android.Paths) android.Path {

	library.objFiles = append(android.Paths{}, deps.WholeStaticLibObjFiles...)
	library.objFiles = append(library.objFiles, objFiles...)

	outputFile := android.PathForModuleOut(ctx,
		ctx.ModuleName()+library.Properties.VariantName+staticLibraryExtension)

	if ctx.Darwin() {
		TransformDarwinObjToStaticLib(ctx, library.objFiles, flagsToBuilderFlags(flags), outputFile)
	} else {
		TransformObjToStaticLib(ctx, library.objFiles, flagsToBuilderFlags(flags), outputFile)
	}

	library.wholeStaticMissingDeps = ctx.GetMissingDependencies()

	ctx.CheckbuildFile(outputFile)

	return outputFile
}

func (library *libraryLinker) linkShared(ctx ModuleContext,
	flags Flags, deps PathDeps, objFiles android.Paths) android.Path {

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

	if library.stripper.needsStrip(ctx) {
		strippedOutputFile := outputFile
		outputFile = android.PathForModuleOut(ctx, "unstripped", fileName)
		library.stripper.strip(ctx, outputFile, strippedOutputFile, builderFlags)
	}

	sharedLibs := deps.SharedLibs
	sharedLibs = append(sharedLibs, deps.LateSharedLibs...)

	TransformObjToDynamicBinary(ctx, objFiles, sharedLibs,
		deps.StaticLibs, deps.LateStaticLibs, deps.WholeStaticLibs,
		linkerDeps, deps.CrtBegin, deps.CrtEnd, false, builderFlags, outputFile)

	return ret
}

func (library *libraryLinker) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objFiles android.Paths) android.Path {

	objFiles = append(objFiles, deps.ObjFiles...)

	var out android.Path
	if library.static() {
		out = library.linkStatic(ctx, flags, deps, objFiles)
	} else {
		out = library.linkShared(ctx, flags, deps, objFiles)
	}

	library.exportIncludes(ctx, "-I")
	library.reexportFlags(deps.ReexportedFlags)

	return out
}

func (library *libraryLinker) buildStatic() bool {
	return library.dynamicProperties.BuildStatic &&
		(library.Properties.Static.Enabled == nil || *library.Properties.Static.Enabled)
}

func (library *libraryLinker) buildShared() bool {
	return library.dynamicProperties.BuildShared &&
		(library.Properties.Shared.Enabled == nil || *library.Properties.Shared.Enabled)
}

func (library *libraryLinker) getWholeStaticMissingDeps() []string {
	return library.wholeStaticMissingDeps
}

func (library *libraryLinker) installable() bool {
	return !library.static()
}

func (library *libraryLinker) objs() android.Paths {
	return library.objFiles
}

type libraryInstaller struct {
	baseInstaller

	linker   *libraryLinker
	sanitize *sanitize
}

func (library *libraryInstaller) install(ctx ModuleContext, file android.Path) {
	if !library.linker.static() {
		library.baseInstaller.install(ctx, file)
	}
}

func (library *libraryInstaller) inData() bool {
	return library.baseInstaller.inData() || library.sanitize.inData()
}

func NewLibrary(hod android.HostOrDeviceSupported, shared, static bool) *Module {
	module := newModule(hod, android.MultilibBoth)

	linker := &libraryLinker{}
	linker.dynamicProperties.BuildShared = shared
	linker.dynamicProperties.BuildStatic = static
	module.linker = linker

	module.compiler = &libraryCompiler{
		linker: linker,
	}
	module.installer = &libraryInstaller{
		baseInstaller: baseInstaller{
			dir:   "lib",
			dir64: "lib64",
		},
		linker:   linker,
		sanitize: module.sanitize,
	}

	return module
}
