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
	"regexp"
	"strings"

	"android/soong/android"
	"android/soong/rust/config"
)

func init() {
	android.RegisterModuleType("rust_library", RustLibraryFactory)
	android.RegisterModuleType("rust_library_dylib", RustLibraryDylibFactory)
	android.RegisterModuleType("rust_library_rlib", RustLibraryRlibFactory)
	android.RegisterModuleType("rust_library_host", RustLibraryHostFactory)
	android.RegisterModuleType("rust_library_host_dylib", RustLibraryDylibHostFactory)
	android.RegisterModuleType("rust_library_host_rlib", RustLibraryRlibHostFactory)
	android.RegisterModuleType("rust_library_shared", RustLibrarySharedFactory)
	android.RegisterModuleType("rust_library_static", RustLibraryStaticFactory)
	android.RegisterModuleType("rust_library_host_shared", RustLibrarySharedHostFactory)
	android.RegisterModuleType("rust_library_host_static", RustLibraryStaticHostFactory)
}

type VariantLibraryProperties struct {
	Enabled *bool `android:"arch_variant"`
}

type LibraryCompilerProperties struct {
	Rlib   VariantLibraryProperties `android:"arch_variant"`
	Dylib  VariantLibraryProperties `android:"arch_variant"`
	Shared VariantLibraryProperties `android:"arch_variant"`
	Static VariantLibraryProperties `android:"arch_variant"`

	// path to the source file that is the main entry point of the program (e.g. src/lib.rs)
	Srcs []string `android:"path,arch_variant"`

	// path to include directories to pass to cc_* modules, only relevant for static/shared variants.
	Include_dirs []string `android:"path,arch_variant"`
}

type LibraryMutatedProperties struct {
	// Build a dylib variant
	BuildDylib bool `blueprint:"mutated"`
	// Build an rlib variant
	BuildRlib bool `blueprint:"mutated"`
	// Build a shared library variant
	BuildShared bool `blueprint:"mutated"`
	// Build a static library variant
	BuildStatic bool `blueprint:"mutated"`

	// This variant is a dylib
	VariantIsDylib bool `blueprint:"mutated"`
	// This variant is an rlib
	VariantIsRlib bool `blueprint:"mutated"`
	// This variant is a shared library
	VariantIsShared bool `blueprint:"mutated"`
	// This variant is a static library
	VariantIsStatic bool `blueprint:"mutated"`
}

type libraryDecorator struct {
	*baseCompiler

	Properties           LibraryCompilerProperties
	MutatedProperties    LibraryMutatedProperties
	distFile             android.OptionalPath
	unstrippedOutputFile android.Path
	includeDirs          android.Paths
}

type libraryInterface interface {
	rlib() bool
	dylib() bool
	static() bool
	shared() bool

	// Returns true if the build options for the module have selected a particular build type
	buildRlib() bool
	buildDylib() bool
	buildShared() bool
	buildStatic() bool

	// Sets a particular variant type
	setRlib()
	setDylib()
	setShared()
	setStatic()

	// Build a specific library variant
	BuildOnlyRlib()
	BuildOnlyDylib()
	BuildOnlyStatic()
	BuildOnlyShared()
}

func (library *libraryDecorator) exportedDirs() []string {
	return library.linkDirs
}

func (library *libraryDecorator) exportedDepFlags() []string {
	return library.depFlags
}

func (library *libraryDecorator) reexportDirs(dirs ...string) {
	library.linkDirs = android.FirstUniqueStrings(append(library.linkDirs, dirs...))
}

func (library *libraryDecorator) reexportDepFlags(flags ...string) {
	library.depFlags = android.FirstUniqueStrings(append(library.depFlags, flags...))
}

func (library *libraryDecorator) rlib() bool {
	return library.MutatedProperties.VariantIsRlib
}

func (library *libraryDecorator) dylib() bool {
	return library.MutatedProperties.VariantIsDylib
}

func (library *libraryDecorator) shared() bool {
	return library.MutatedProperties.VariantIsShared
}

func (library *libraryDecorator) static() bool {
	return library.MutatedProperties.VariantIsStatic
}

func (library *libraryDecorator) buildRlib() bool {
	return library.MutatedProperties.BuildRlib && BoolDefault(library.Properties.Rlib.Enabled, true)
}

func (library *libraryDecorator) buildDylib() bool {
	return library.MutatedProperties.BuildDylib && BoolDefault(library.Properties.Dylib.Enabled, true)
}

func (library *libraryDecorator) buildShared() bool {
	return library.MutatedProperties.BuildShared && BoolDefault(library.Properties.Shared.Enabled, true)
}

func (library *libraryDecorator) buildStatic() bool {
	return library.MutatedProperties.BuildStatic && BoolDefault(library.Properties.Static.Enabled, true)
}

func (library *libraryDecorator) setRlib() {
	library.MutatedProperties.VariantIsRlib = true
	library.MutatedProperties.VariantIsDylib = false
	library.MutatedProperties.VariantIsStatic = false
	library.MutatedProperties.VariantIsShared = false
}

func (library *libraryDecorator) setDylib() {
	library.MutatedProperties.VariantIsRlib = false
	library.MutatedProperties.VariantIsDylib = true
	library.MutatedProperties.VariantIsStatic = false
	library.MutatedProperties.VariantIsShared = false
}

func (library *libraryDecorator) setShared() {
	library.MutatedProperties.VariantIsStatic = false
	library.MutatedProperties.VariantIsShared = true
	library.MutatedProperties.VariantIsRlib = false
	library.MutatedProperties.VariantIsDylib = false
}

func (library *libraryDecorator) setStatic() {
	library.MutatedProperties.VariantIsStatic = true
	library.MutatedProperties.VariantIsShared = false
	library.MutatedProperties.VariantIsRlib = false
	library.MutatedProperties.VariantIsDylib = false
}

var _ compiler = (*libraryDecorator)(nil)
var _ libraryInterface = (*libraryDecorator)(nil)

// rust_library produces all variants.
func RustLibraryFactory() android.Module {
	module, _ := NewRustLibrary(android.HostAndDeviceSupported)
	return module.Init()
}

// rust_library_dylib produces a dylib.
func RustLibraryDylibFactory() android.Module {
	module, library := NewRustLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyDylib()
	return module.Init()
}

// rust_library_rlib produces an rlib.
func RustLibraryRlibFactory() android.Module {
	module, library := NewRustLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyRlib()
	return module.Init()
}

// rust_library_shared produces a shared library.
func RustLibrarySharedFactory() android.Module {
	module, library := NewRustLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyShared()
	return module.Init()
}

// rust_library_static produces a static library.
func RustLibraryStaticFactory() android.Module {
	module, library := NewRustLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyStatic()
	return module.Init()
}

// rust_library_host produces all variants.
func RustLibraryHostFactory() android.Module {
	module, _ := NewRustLibrary(android.HostSupported)
	return module.Init()
}

// rust_library_dylib_host produces a dylib.
func RustLibraryDylibHostFactory() android.Module {
	module, library := NewRustLibrary(android.HostSupported)
	library.BuildOnlyDylib()
	return module.Init()
}

// rust_library_rlib_host produces an rlib.
func RustLibraryRlibHostFactory() android.Module {
	module, library := NewRustLibrary(android.HostSupported)
	library.BuildOnlyRlib()
	return module.Init()
}

// rust_library_static_host produces a static library.
func RustLibraryStaticHostFactory() android.Module {
	module, library := NewRustLibrary(android.HostSupported)
	library.BuildOnlyStatic()
	return module.Init()
}

// rust_library_shared_host produces an shared library.
func RustLibrarySharedHostFactory() android.Module {
	module, library := NewRustLibrary(android.HostSupported)
	library.BuildOnlyShared()
	return module.Init()
}

func (library *libraryDecorator) BuildOnlyDylib() {
	library.MutatedProperties.BuildRlib = false
	library.MutatedProperties.BuildShared = false
	library.MutatedProperties.BuildStatic = false

}

func (library *libraryDecorator) BuildOnlyRlib() {
	library.MutatedProperties.BuildDylib = false
	library.MutatedProperties.BuildShared = false
	library.MutatedProperties.BuildStatic = false
}

func (library *libraryDecorator) BuildOnlyStatic() {
	library.MutatedProperties.BuildShared = false
	library.MutatedProperties.BuildRlib = false
	library.MutatedProperties.BuildDylib = false

}

func (library *libraryDecorator) BuildOnlyShared() {
	library.MutatedProperties.BuildStatic = false
	library.MutatedProperties.BuildRlib = false
	library.MutatedProperties.BuildDylib = false
}

func NewRustLibrary(hod android.HostOrDeviceSupported) (*Module, *libraryDecorator) {
	module := newModule(hod, android.MultilibFirst)

	library := &libraryDecorator{
		MutatedProperties: LibraryMutatedProperties{
			BuildDylib:  true,
			BuildRlib:   true,
			BuildShared: true,
			BuildStatic: true,
		},
		baseCompiler: NewBaseCompiler("lib", "lib64", InstallInSystem),
	}

	module.compiler = library

	return module, library
}

func (library *libraryDecorator) compilerProps() []interface{} {
	return append(library.baseCompiler.compilerProps(),
		&library.Properties,
		&library.MutatedProperties)
}

func (library *libraryDecorator) compilerDeps(ctx DepsContext, deps Deps) Deps {

	// TODO(b/144861059) Remove if C libraries support dylib linkage in the future.
	if !ctx.Host() && (library.static() || library.shared()) {
		library.setNoStdlibs()
		for _, stdlib := range config.Stdlibs {
			deps.Rlibs = append(deps.Rlibs, stdlib+".static")
		}
	}

	deps = library.baseCompiler.compilerDeps(ctx, deps)

	if ctx.toolchain().Bionic() && (library.dylib() || library.shared()) {
		deps = library.baseCompiler.bionicDeps(ctx, deps)
	}

	return deps
}
func (library *libraryDecorator) compilerFlags(ctx ModuleContext, flags Flags) Flags {
	flags.RustFlags = append(flags.RustFlags, "-C metadata="+ctx.baseModuleName())
	flags = library.baseCompiler.compilerFlags(ctx, flags)
	if library.shared() || library.static() {
		library.includeDirs = append(library.includeDirs, android.PathsForModuleSrc(ctx, library.Properties.Include_dirs)...)
	}
	return flags
}

func (library *libraryDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) android.Path {
	var outputFile android.WritablePath

	srcPath := srcPathFromModuleSrcs(ctx, library.Properties.Srcs)

	flags.RustFlags = append(flags.RustFlags, deps.depFlags...)

	if library.dylib() {
		// We need prefer-dynamic for now to avoid linking in the static stdlib. See:
		// https://github.com/rust-lang/rust/issues/19680
		// https://github.com/rust-lang/rust/issues/34909
		flags.RustFlags = append(flags.RustFlags, "-C prefer-dynamic")
	}

	if library.rlib() {
		fileName := library.getStem(ctx) + ctx.toolchain().RlibSuffix()
		outputFile = android.PathForModuleOut(ctx, fileName)

		TransformSrctoRlib(ctx, srcPath, deps, flags, outputFile, deps.linkDirs)
	} else if library.dylib() {
		fileName := library.getStem(ctx) + ctx.toolchain().DylibSuffix()
		outputFile = android.PathForModuleOut(ctx, fileName)

		TransformSrctoDylib(ctx, srcPath, deps, flags, outputFile, deps.linkDirs)
	} else if library.static() {
		fileName := library.getStem(ctx) + ctx.toolchain().StaticLibSuffix()
		outputFile = android.PathForModuleOut(ctx, fileName)

		TransformSrctoStatic(ctx, srcPath, deps, flags, outputFile, deps.linkDirs)
	} else if library.shared() {
		fileName := library.getStem(ctx) + ctx.toolchain().SharedLibSuffix()
		outputFile = android.PathForModuleOut(ctx, fileName)

		TransformSrctoShared(ctx, srcPath, deps, flags, outputFile, deps.linkDirs)
	}

	if library.rlib() || library.dylib() {
		library.reexportDirs(deps.linkDirs...)
		library.reexportDepFlags(deps.depFlags...)
	}
	library.unstrippedOutputFile = outputFile

	return outputFile
}

func (library *libraryDecorator) getStem(ctx ModuleContext) string {
	stem := library.baseCompiler.getStemWithoutSuffix(ctx)
	validateLibraryStem(ctx, stem, library.crateName())

	return stem + String(library.baseCompiler.Properties.Suffix)
}

var validCrateName = regexp.MustCompile("[^a-zA-Z0-9_]+")

func validateLibraryStem(ctx BaseModuleContext, filename string, crate_name string) {
	if crate_name == "" {
		ctx.PropertyErrorf("crate_name", "crate_name must be defined.")
	}

	// crate_names are used for the library output file, and rustc expects these
	// to be alphanumeric with underscores allowed.
	if validCrateName.MatchString(crate_name) {
		ctx.PropertyErrorf("crate_name",
			"library crate_names must be alphanumeric with underscores allowed")
	}

	// Libraries are expected to begin with "lib" followed by the crate_name
	if !strings.HasPrefix(filename, "lib"+crate_name) {
		ctx.ModuleErrorf("Invalid name or stem property; library filenames must start with lib<crate_name>")
	}
}

func LibraryMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok && m.compiler != nil {
		switch library := m.compiler.(type) {
		case libraryInterface:

			// We only build the rust library variants here. This assumes that
			// LinkageMutator runs first and there's an empty variant
			// if rust variants are required.
			if !library.static() && !library.shared() {
				if library.buildRlib() && library.buildDylib() {
					modules := mctx.CreateLocalVariations("rlib", "dylib")
					rlib := modules[0].(*Module)
					dylib := modules[1].(*Module)

					rlib.compiler.(libraryInterface).setRlib()
					dylib.compiler.(libraryInterface).setDylib()
				} else if library.buildRlib() {
					modules := mctx.CreateLocalVariations("rlib")
					modules[0].(*Module).compiler.(libraryInterface).setRlib()
				} else if library.buildDylib() {
					modules := mctx.CreateLocalVariations("dylib")
					modules[0].(*Module).compiler.(libraryInterface).setDylib()
				}
			}
		}
	}
}
