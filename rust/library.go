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
	android.RegisterModuleType("rust_library", RustLibraryFactory)
	android.RegisterModuleType("rust_library_dylib", RustLibraryDylibFactory)
	android.RegisterModuleType("rust_library_rlib", RustLibraryRlibFactory)
	android.RegisterModuleType("rust_library_host", RustLibraryHostFactory)
	android.RegisterModuleType("rust_library_host_dylib", RustLibraryDylibHostFactory)
	android.RegisterModuleType("rust_library_host_rlib", RustLibraryRlibHostFactory)

	//TODO: Add support for generating standard shared/static libraries.
}

type VariantLibraryProperties struct {
	Enabled *bool `android:"arch_variant"`
}

type LibraryCompilerProperties struct {
	Rlib  VariantLibraryProperties `android:"arch_variant"`
	Dylib VariantLibraryProperties `android:"arch_variant"`

	// path to the source file that is the main entry point of the program (e.g. src/lib.rs)
	Srcs []string `android:"path,arch_variant"`
}

type LibraryMutatedProperties struct {
	VariantName string `blueprint:"mutated"`

	// Build a dylib variant
	BuildDylib bool `blueprint:"mutated"`
	// Build an rlib variant
	BuildRlib bool `blueprint:"mutated"`

	// This variant is a dylib
	VariantIsDylib bool `blueprint:"mutated"`
	// This variant is an rlib
	VariantIsRlib bool `blueprint:"mutated"`
}

type libraryDecorator struct {
	*baseCompiler

	Properties           LibraryCompilerProperties
	MutatedProperties    LibraryMutatedProperties
	distFile             android.OptionalPath
	unstrippedOutputFile android.Path
}

type libraryInterface interface {
	rlib() bool
	dylib() bool

	// Returns true if the build options for the module have selected a particular build type
	buildRlib() bool
	buildDylib() bool

	// Sets a particular variant type
	setRlib()
	setDylib()
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

func (library *libraryDecorator) buildRlib() bool {
	return library.MutatedProperties.BuildRlib && BoolDefault(library.Properties.Rlib.Enabled, true)
}

func (library *libraryDecorator) buildDylib() bool {
	return library.MutatedProperties.BuildDylib && BoolDefault(library.Properties.Dylib.Enabled, true)
}

func (library *libraryDecorator) setRlib() {
	library.MutatedProperties.VariantIsRlib = true
	library.MutatedProperties.VariantIsDylib = false
}

func (library *libraryDecorator) setDylib() {
	library.MutatedProperties.VariantIsRlib = false
	library.MutatedProperties.VariantIsDylib = true
}

var _ compiler = (*libraryDecorator)(nil)

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

func (library *libraryDecorator) BuildOnlyDylib() {
	library.MutatedProperties.BuildRlib = false
}

func (library *libraryDecorator) BuildOnlyRlib() {
	library.MutatedProperties.BuildDylib = false
}

func NewRustLibrary(hod android.HostOrDeviceSupported) (*Module, *libraryDecorator) {
	module := newModule(hod, android.MultilibFirst)

	library := &libraryDecorator{
		MutatedProperties: LibraryMutatedProperties{
			BuildDylib: true,
			BuildRlib:  true,
		},
		baseCompiler: NewBaseCompiler("lib", "lib64"),
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
	deps = library.baseCompiler.compilerDeps(ctx, deps)

	if ctx.toolchain().Bionic() && library.dylib() {
		deps = library.baseCompiler.bionicDeps(ctx, deps)
	}

	return deps
}

func (library *libraryDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) android.Path {
	var outputFile android.WritablePath

	srcPath := srcPathFromModuleSrcs(ctx, library.Properties.Srcs)

	flags.RustFlags = append(flags.RustFlags, deps.depFlags...)

	if library.rlib() {
		fileName := library.getStem(ctx) + ctx.toolchain().RlibSuffix()
		outputFile = android.PathForModuleOut(ctx, fileName)

		TransformSrctoRlib(ctx, srcPath, deps, flags, outputFile, deps.linkDirs)
	} else if library.dylib() {
		fileName := library.getStem(ctx) + ctx.toolchain().DylibSuffix()
		outputFile = android.PathForModuleOut(ctx, fileName)

		// We need prefer-dynamic for now to avoid linking in the static stdlib. See:
		// https://github.com/rust-lang/rust/issues/19680
		// https://github.com/rust-lang/rust/issues/34909
		flags.RustFlags = append(flags.RustFlags, "-C prefer-dynamic")

		TransformSrctoDylib(ctx, srcPath, deps, flags, outputFile, deps.linkDirs)
	}

	library.reexportDirs(deps.linkDirs...)
	library.reexportDepFlags(deps.depFlags...)
	library.unstrippedOutputFile = outputFile

	return outputFile
}

func LibraryMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok && m.compiler != nil {
		switch library := m.compiler.(type) {
		case libraryInterface:
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
