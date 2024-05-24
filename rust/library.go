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
	"errors"
	"fmt"
	"regexp"
	"strings"

	"android/soong/android"
	"android/soong/cc"
)

var (
	RlibStdlibSuffix = ".rlib-std"
)

func init() {
	android.RegisterModuleType("rust_library", RustLibraryFactory)
	android.RegisterModuleType("rust_library_dylib", RustLibraryDylibFactory)
	android.RegisterModuleType("rust_library_rlib", RustLibraryRlibFactory)
	android.RegisterModuleType("rust_library_host", RustLibraryHostFactory)
	android.RegisterModuleType("rust_library_host_dylib", RustLibraryDylibHostFactory)
	android.RegisterModuleType("rust_library_host_rlib", RustLibraryRlibHostFactory)
	android.RegisterModuleType("rust_ffi", RustFFIFactory)
	android.RegisterModuleType("rust_ffi_shared", RustFFISharedFactory)
	android.RegisterModuleType("rust_ffi_rlib", RustFFIRlibFactory)
	android.RegisterModuleType("rust_ffi_host", RustFFIHostFactory)
	android.RegisterModuleType("rust_ffi_host_shared", RustFFISharedHostFactory)
	android.RegisterModuleType("rust_ffi_host_rlib", RustFFIRlibHostFactory)

	// TODO: Remove when all instances of rust_ffi_static have been switched to rust_ffi_rlib
	// Alias rust_ffi_static to the combined rust_ffi_rlib factory
	android.RegisterModuleType("rust_ffi_static", RustFFIStaticRlibFactory)
	android.RegisterModuleType("rust_ffi_host_static", RustFFIStaticRlibHostFactory)
}

type VariantLibraryProperties struct {
	Enabled *bool    `android:"arch_variant"`
	Srcs    []string `android:"path,arch_variant"`
}

type LibraryCompilerProperties struct {
	Rlib   VariantLibraryProperties `android:"arch_variant"`
	Dylib  VariantLibraryProperties `android:"arch_variant"`
	Shared VariantLibraryProperties `android:"arch_variant"`
	Static VariantLibraryProperties `android:"arch_variant"`

	// TODO: Remove this when all instances of Include_dirs have been removed from rust_ffi modules.
	// path to include directories to pass to cc_* modules, only relevant for static/shared variants (deprecated, use export_include_dirs instead).
	Include_dirs []string `android:"path,arch_variant"`

	// path to include directories to export to cc_* modules, only relevant for static/shared variants.
	Export_include_dirs []string `android:"path,arch_variant"`

	// Whether this library is part of the Rust toolchain sysroot.
	Sysroot *bool
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
	// This variant is a source provider
	VariantIsSource bool `blueprint:"mutated"`

	// This variant is disabled and should not be compiled
	// (used for SourceProvider variants that produce only source)
	VariantIsDisabled bool `blueprint:"mutated"`

	// Whether this library variant should be link libstd via rlibs
	VariantIsStaticStd bool `blueprint:"mutated"`
}

type libraryDecorator struct {
	*baseCompiler
	*flagExporter
	stripper Stripper

	Properties        LibraryCompilerProperties
	MutatedProperties LibraryMutatedProperties
	includeDirs       android.Paths
	sourceProvider    SourceProvider

	isFFI bool

	// table-of-contents file for cdylib crates to optimize out relinking when possible
	tocFile android.OptionalPath
}

type libraryInterface interface {
	rlib() bool
	dylib() bool
	static() bool
	shared() bool
	sysroot() bool
	source() bool

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
	setSource()

	// libstd linkage functions
	rlibStd() bool
	setRlibStd()
	setDylibStd()

	// Build a specific library variant
	BuildOnlyFFI()
	BuildOnlyRust()
	BuildOnlyRlib()
	BuildOnlyDylib()
	BuildOnlyStatic()
	BuildOnlyShared()

	toc() android.OptionalPath

	isFFILibrary() bool
}

func (library *libraryDecorator) nativeCoverage() bool {
	return true
}

func (library *libraryDecorator) toc() android.OptionalPath {
	return library.tocFile
}

func (library *libraryDecorator) rlib() bool {
	return library.MutatedProperties.VariantIsRlib
}

func (library *libraryDecorator) sysroot() bool {
	return Bool(library.Properties.Sysroot)
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

func (library *libraryDecorator) source() bool {
	return library.MutatedProperties.VariantIsSource
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

func (library *libraryDecorator) rlibStd() bool {
	return library.MutatedProperties.VariantIsStaticStd
}

func (library *libraryDecorator) setRlibStd() {
	library.MutatedProperties.VariantIsStaticStd = true
}

func (library *libraryDecorator) setDylibStd() {
	library.MutatedProperties.VariantIsStaticStd = false
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

func (library *libraryDecorator) setSource() {
	library.MutatedProperties.VariantIsSource = true
}

func (library *libraryDecorator) autoDep(ctx android.BottomUpMutatorContext) autoDep {
	if library.preferRlib() {
		return rlibAutoDep
	} else if library.rlib() || library.static() {
		return rlibAutoDep
	} else if library.dylib() || library.shared() {
		return dylibAutoDep
	} else {
		panic(fmt.Errorf("autoDep called on library %q that has no enabled variants.", ctx.ModuleName()))
	}
}

func (library *libraryDecorator) stdLinkage(ctx *depsContext) RustLinkage {
	if library.static() || library.MutatedProperties.VariantIsStaticStd || (library.rlib() && library.isFFILibrary()) {
		return RlibLinkage
	} else if library.baseCompiler.preferRlib() {
		return RlibLinkage
	}
	return DefaultLinkage
}

var _ compiler = (*libraryDecorator)(nil)
var _ libraryInterface = (*libraryDecorator)(nil)
var _ exportedFlagsProducer = (*libraryDecorator)(nil)

// rust_library produces all Rust variants (rust_library_dylib and
// rust_library_rlib).
func RustLibraryFactory() android.Module {
	module, library := NewRustLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyRust()
	return module.Init()
}

// rust_ffi produces all FFI variants (rust_ffi_shared, rust_ffi_static, and
// rust_ffi_rlib).
func RustFFIFactory() android.Module {
	module, library := NewRustLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyFFI()
	return module.Init()
}

// rust_library_dylib produces a Rust dylib (Rust crate type "dylib").
func RustLibraryDylibFactory() android.Module {
	module, library := NewRustLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyDylib()
	return module.Init()
}

// rust_library_rlib produces an rlib (Rust crate type "rlib").
func RustLibraryRlibFactory() android.Module {
	module, library := NewRustLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyRlib()
	return module.Init()
}

// rust_ffi_shared produces a shared library (Rust crate type
// "cdylib").
func RustFFISharedFactory() android.Module {
	module, library := NewRustLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyShared()
	return module.Init()
}

// rust_library_host produces all Rust variants for the host
// (rust_library_dylib_host and rust_library_rlib_host).
func RustLibraryHostFactory() android.Module {
	module, library := NewRustLibrary(android.HostSupported)
	library.BuildOnlyRust()
	return module.Init()
}

// rust_ffi_host produces all FFI variants for the host
// (rust_ffi_rlib_host, rust_ffi_static_host, and rust_ffi_shared_host).
func RustFFIHostFactory() android.Module {
	module, library := NewRustLibrary(android.HostSupported)
	library.BuildOnlyFFI()
	return module.Init()
}

// rust_library_dylib_host produces a dylib for the host (Rust crate
// type "dylib").
func RustLibraryDylibHostFactory() android.Module {
	module, library := NewRustLibrary(android.HostSupported)
	library.BuildOnlyDylib()
	return module.Init()
}

// rust_library_rlib_host produces an rlib for the host (Rust crate
// type "rlib").
func RustLibraryRlibHostFactory() android.Module {
	module, library := NewRustLibrary(android.HostSupported)
	library.BuildOnlyRlib()
	return module.Init()
}

// rust_ffi_shared_host produces an shared library for the host (Rust
// crate type "cdylib").
func RustFFISharedHostFactory() android.Module {
	module, library := NewRustLibrary(android.HostSupported)
	library.BuildOnlyShared()
	return module.Init()
}

// rust_ffi_rlib_host produces an rlib for the host (Rust crate
// type "rlib").
func RustFFIRlibHostFactory() android.Module {
	module, library := NewRustLibrary(android.HostSupported)
	library.BuildOnlyRlibStatic()

	library.isFFI = true
	return module.Init()
}

// rust_ffi_rlib produces an rlib (Rust crate type "rlib").
func RustFFIRlibFactory() android.Module {
	module, library := NewRustLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyRlib()

	library.isFFI = true
	return module.Init()
}

// rust_ffi_static produces a staticlib and an rlib variant
func RustFFIStaticRlibFactory() android.Module {
	module, library := NewRustLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyRlibStatic()

	library.isFFI = true
	return module.Init()
}

// rust_ffi_static_host produces a staticlib and an rlib variant for the host
func RustFFIStaticRlibHostFactory() android.Module {
	module, library := NewRustLibrary(android.HostSupported)
	library.BuildOnlyRlibStatic()

	library.isFFI = true
	return module.Init()
}

func (library *libraryDecorator) BuildOnlyFFI() {
	library.MutatedProperties.BuildDylib = false
	// we build rlibs for later static ffi linkage.
	library.MutatedProperties.BuildRlib = true
	library.MutatedProperties.BuildShared = true
	library.MutatedProperties.BuildStatic = true

	library.isFFI = true
}

func (library *libraryDecorator) BuildOnlyRust() {
	library.MutatedProperties.BuildDylib = true
	library.MutatedProperties.BuildRlib = true
	library.MutatedProperties.BuildShared = false
	library.MutatedProperties.BuildStatic = false
}

func (library *libraryDecorator) BuildOnlyDylib() {
	library.MutatedProperties.BuildDylib = true
	library.MutatedProperties.BuildRlib = false
	library.MutatedProperties.BuildShared = false
	library.MutatedProperties.BuildStatic = false
}

func (library *libraryDecorator) BuildOnlyRlib() {
	library.MutatedProperties.BuildDylib = false
	library.MutatedProperties.BuildRlib = true
	library.MutatedProperties.BuildShared = false
	library.MutatedProperties.BuildStatic = false
}

func (library *libraryDecorator) BuildOnlyRlibStatic() {
	library.MutatedProperties.BuildDylib = false
	library.MutatedProperties.BuildRlib = true
	library.MutatedProperties.BuildShared = false
	library.MutatedProperties.BuildStatic = true
	library.isFFI = true
}

func (library *libraryDecorator) BuildOnlyStatic() {
	library.MutatedProperties.BuildRlib = false
	library.MutatedProperties.BuildDylib = false
	library.MutatedProperties.BuildShared = false
	library.MutatedProperties.BuildStatic = true

	library.isFFI = true
}

func (library *libraryDecorator) BuildOnlyShared() {
	library.MutatedProperties.BuildRlib = false
	library.MutatedProperties.BuildDylib = false
	library.MutatedProperties.BuildStatic = false
	library.MutatedProperties.BuildShared = true

	library.isFFI = true
}

func (library *libraryDecorator) isFFILibrary() bool {
	return library.isFFI
}

func NewRustLibrary(hod android.HostOrDeviceSupported) (*Module, *libraryDecorator) {
	module := newModule(hod, android.MultilibBoth)

	library := &libraryDecorator{
		MutatedProperties: LibraryMutatedProperties{
			BuildDylib:  false,
			BuildRlib:   false,
			BuildShared: false,
			BuildStatic: false,
		},
		baseCompiler: NewBaseCompiler("lib", "lib64", InstallInSystem),
		flagExporter: NewFlagExporter(),
	}

	module.compiler = library

	return module, library
}

func (library *libraryDecorator) compilerProps() []interface{} {
	return append(library.baseCompiler.compilerProps(),
		&library.Properties,
		&library.MutatedProperties,
		&library.stripper.StripProperties)
}

func (library *libraryDecorator) compilerDeps(ctx DepsContext, deps Deps) Deps {
	deps = library.baseCompiler.compilerDeps(ctx, deps)

	if library.dylib() || library.shared() {
		if ctx.toolchain().Bionic() {
			deps = bionicDeps(ctx, deps, false)
			deps.CrtBegin = []string{"crtbegin_so"}
			deps.CrtEnd = []string{"crtend_so"}
		} else if ctx.Os() == android.LinuxMusl {
			deps = muslDeps(ctx, deps, false)
			deps.CrtBegin = []string{"libc_musl_crtbegin_so"}
			deps.CrtEnd = []string{"libc_musl_crtend_so"}
		}
	}

	return deps
}

func (library *libraryDecorator) sharedLibFilename(ctx ModuleContext) string {
	return library.getStem(ctx) + ctx.toolchain().SharedLibSuffix()
}

// Library cfg flags common to all variants
func CommonLibraryCfgFlags(ctx android.ModuleContext, flags Flags) Flags {
	return flags
}

func (library *libraryDecorator) cfgFlags(ctx ModuleContext, flags Flags) Flags {
	flags = library.baseCompiler.cfgFlags(ctx, flags)
	flags = CommonLibraryCfgFlags(ctx, flags)

	cfgs := library.baseCompiler.Properties.Cfgs.GetOrDefault(ctx, nil)

	if library.dylib() {
		// We need to add a dependency on std in order to link crates as dylibs.
		// The hack to add this dependency is guarded by the following cfg so
		// that we don't force a dependency when it isn't needed.
		cfgs = append(cfgs, "android_dylib")
	}

	cfgFlags := cfgsToFlags(cfgs)

	flags.RustFlags = append(flags.RustFlags, cfgFlags...)
	flags.RustdocFlags = append(flags.RustdocFlags, cfgFlags...)

	return flags
}

// Common flags applied to all libraries irrespective of properties or variant should be included here
func CommonLibraryCompilerFlags(ctx android.ModuleContext, flags Flags) Flags {
	flags.RustFlags = append(flags.RustFlags, "-C metadata="+ctx.ModuleName())

	return flags
}

func (library *libraryDecorator) compilerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = library.baseCompiler.compilerFlags(ctx, flags)

	flags = CommonLibraryCompilerFlags(ctx, flags)

	if library.isFFI {
		library.includeDirs = append(library.includeDirs, android.PathsForModuleSrc(ctx, library.Properties.Include_dirs)...)
		library.includeDirs = append(library.includeDirs, android.PathsForModuleSrc(ctx, library.Properties.Export_include_dirs)...)
	}

	if library.shared() {
		if ctx.Darwin() {
			flags.LinkFlags = append(
				flags.LinkFlags,
				"-dynamic_lib",
				"-install_name @rpath/"+library.sharedLibFilename(ctx),
			)
		} else {
			flags.LinkFlags = append(flags.LinkFlags, "-Wl,-soname="+library.sharedLibFilename(ctx))
		}
	}

	return flags
}

func (library *libraryDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) buildOutput {
	var outputFile android.ModuleOutPath
	var ret buildOutput
	var fileName string
	crateRootPath := crateRootPath(ctx, library)

	if library.sourceProvider != nil {
		deps.srcProviderFiles = append(deps.srcProviderFiles, library.sourceProvider.Srcs()...)
	}

	// Ensure link dirs are not duplicated
	deps.linkDirs = android.FirstUniqueStrings(deps.linkDirs)

	// Calculate output filename
	if library.rlib() {
		fileName = library.getStem(ctx) + ctx.toolchain().RlibSuffix()
		outputFile = android.PathForModuleOut(ctx, fileName)
		ret.outputFile = outputFile
	} else if library.dylib() {
		fileName = library.getStem(ctx) + ctx.toolchain().DylibSuffix()
		outputFile = android.PathForModuleOut(ctx, fileName)
		ret.outputFile = outputFile
	} else if library.static() {
		fileName = library.getStem(ctx) + ctx.toolchain().StaticLibSuffix()
		outputFile = android.PathForModuleOut(ctx, fileName)
		ret.outputFile = outputFile
	} else if library.shared() {
		fileName = library.sharedLibFilename(ctx)
		outputFile = android.PathForModuleOut(ctx, fileName)
		ret.outputFile = outputFile
	}

	if !library.rlib() && !library.static() && library.stripper.NeedsStrip(ctx) {
		strippedOutputFile := outputFile
		outputFile = android.PathForModuleOut(ctx, "unstripped", fileName)
		library.stripper.StripExecutableOrSharedLib(ctx, outputFile, strippedOutputFile)

		library.baseCompiler.strippedOutputFile = android.OptionalPathForPath(strippedOutputFile)
	}
	library.baseCompiler.unstrippedOutputFile = outputFile

	flags.RustFlags = append(flags.RustFlags, deps.depFlags...)
	flags.LinkFlags = append(flags.LinkFlags, deps.depLinkFlags...)
	flags.LinkFlags = append(flags.LinkFlags, deps.linkObjects...)

	if library.dylib() {
		// We need prefer-dynamic for now to avoid linking in the static stdlib. See:
		// https://github.com/rust-lang/rust/issues/19680
		// https://github.com/rust-lang/rust/issues/34909
		flags.RustFlags = append(flags.RustFlags, "-C prefer-dynamic")
	}

	// Call the appropriate builder for this library type
	if library.rlib() {
		ret.kytheFile = TransformSrctoRlib(ctx, crateRootPath, deps, flags, outputFile).kytheFile
	} else if library.dylib() {
		ret.kytheFile = TransformSrctoDylib(ctx, crateRootPath, deps, flags, outputFile).kytheFile
	} else if library.static() {
		ret.kytheFile = TransformSrctoStatic(ctx, crateRootPath, deps, flags, outputFile).kytheFile
	} else if library.shared() {
		ret.kytheFile = TransformSrctoShared(ctx, crateRootPath, deps, flags, outputFile).kytheFile
	}

	if library.rlib() || library.dylib() {
		library.flagExporter.exportLinkDirs(deps.linkDirs...)
		library.flagExporter.exportLinkObjects(deps.linkObjects...)
	}

	// Since we have FFI rlibs, we need to collect their includes as well
	if library.static() || library.shared() || library.rlib() {
		android.SetProvider(ctx, cc.FlagExporterInfoProvider, cc.FlagExporterInfo{
			IncludeDirs: android.FirstUniquePaths(library.includeDirs),
		})
	}

	if library.shared() {
		// Optimize out relinking against shared libraries whose interface hasn't changed by
		// depending on a table of contents file instead of the library itself.
		tocFile := outputFile.ReplaceExtension(ctx, flags.Toolchain.SharedLibSuffix()[1:]+".toc")
		library.tocFile = android.OptionalPathForPath(tocFile)
		cc.TransformSharedObjectToToc(ctx, outputFile, tocFile)

		android.SetProvider(ctx, cc.SharedLibraryInfoProvider, cc.SharedLibraryInfo{
			TableOfContents: android.OptionalPathForPath(tocFile),
			SharedLibrary:   outputFile,
			Target:          ctx.Target(),
		})
	}

	if library.static() {
		depSet := android.NewDepSetBuilder[android.Path](android.TOPOLOGICAL).Direct(outputFile).Build()
		android.SetProvider(ctx, cc.StaticLibraryInfoProvider, cc.StaticLibraryInfo{
			StaticLibrary: outputFile,

			TransitiveStaticLibrariesForOrdering: depSet,
		})
	}

	library.flagExporter.setProvider(ctx)

	return ret
}

func (library *libraryDecorator) checkedCrateRootPath() (android.Path, error) {
	if library.sourceProvider != nil {
		srcs := library.sourceProvider.Srcs()
		if len(srcs) == 0 {
			return nil, errors.New("Source provider generated 0 sources")
		}
		// Assume the first source from the source provider is the library entry point.
		return srcs[0], nil
	} else {
		return library.baseCompiler.checkedCrateRootPath()
	}
}

func (library *libraryDecorator) rustdoc(ctx ModuleContext, flags Flags,
	deps PathDeps) android.OptionalPath {
	// rustdoc has builtin support for documenting config specific information
	// regardless of the actual config it was given
	// (https://doc.rust-lang.org/rustdoc/advanced-features.html#cfgdoc-documenting-platform-specific-or-feature-specific-information),
	// so we generate the rustdoc for only the primary module so that we have a
	// single set of docs to refer to.
	if ctx.Module() != ctx.PrimaryModule() {
		return android.OptionalPath{}
	}

	return android.OptionalPathForPath(Rustdoc(ctx, crateRootPath(ctx, library),
		deps, flags))
}

func (library *libraryDecorator) getStem(ctx ModuleContext) string {
	stem := library.baseCompiler.getStemWithoutSuffix(ctx)
	validateLibraryStem(ctx, stem, library.crateName())

	return stem + String(library.baseCompiler.Properties.Suffix)
}

func (library *libraryDecorator) install(ctx ModuleContext) {
	// Only shared and dylib variants make sense to install.
	if library.shared() || library.dylib() {
		library.baseCompiler.install(ctx)
	}
}

func (library *libraryDecorator) Disabled() bool {
	return library.MutatedProperties.VariantIsDisabled
}

func (library *libraryDecorator) SetDisabled() {
	library.MutatedProperties.VariantIsDisabled = true
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

// LibraryMutator mutates the libraries into variants according to the
// build{Rlib,Dylib} attributes.
func LibraryMutator(mctx android.BottomUpMutatorContext) {
	// Only mutate on Rust libraries.
	m, ok := mctx.Module().(*Module)
	if !ok || m.compiler == nil {
		return
	}
	library, ok := m.compiler.(libraryInterface)
	if !ok {
		return
	}

	// Don't produce rlib/dylib/source variants for shared or static variants
	if library.shared() || library.static() {
		return
	}

	var variants []string
	// The source variant is used for SourceProvider modules. The other variants (i.e. rlib and dylib)
	// depend on this variant. It must be the first variant to be declared.
	sourceVariant := false
	if m.sourceProvider != nil {
		variants = append(variants, "source")
		sourceVariant = true
	}
	if library.buildRlib() {
		variants = append(variants, rlibVariation)
	}
	if library.buildDylib() {
		variants = append(variants, dylibVariation)
	}

	if len(variants) == 0 {
		return
	}
	modules := mctx.CreateLocalVariations(variants...)

	// The order of the variations (modules) matches the variant names provided. Iterate
	// through the new variation modules and set their mutated properties.
	for i, v := range modules {
		switch variants[i] {
		case rlibVariation:
			v.(*Module).compiler.(libraryInterface).setRlib()
		case dylibVariation:
			v.(*Module).compiler.(libraryInterface).setDylib()
			if v.(*Module).ModuleBase.ImageVariation().Variation == android.VendorRamdiskVariation {
				// TODO(b/165791368)
				// Disable dylib Vendor Ramdisk variations until we support these.
				v.(*Module).Disable()
			}

		case "source":
			v.(*Module).compiler.(libraryInterface).setSource()
			// The source variant does not produce any library.
			// Disable the compilation steps.
			v.(*Module).compiler.SetDisabled()
		case "":
			// if there's an empty variant, alias it so it is the default variant
			mctx.AliasVariation("")
		}
	}

	// If a source variant is created, add an inter-variant dependency
	// between the other variants and the source variant.
	if sourceVariant {
		sv := modules[0]
		for _, v := range modules[1:] {
			if !v.Enabled(mctx) {
				continue
			}
			mctx.AddInterVariantDependency(sourceDepTag, v, sv)
		}
		// Alias the source variation so it can be named directly in "srcs" properties.
		mctx.AliasVariation("source")
	}
}

func LibstdMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok && m.compiler != nil && !m.compiler.Disabled() {
		switch library := m.compiler.(type) {
		case libraryInterface:
			// Only create a variant if a library is actually being built.
			if library.rlib() && !library.sysroot() {
				// If this is a rust_ffi variant it only needs rlib-std
				if library.isFFILibrary() {
					variants := []string{"rlib-std"}
					modules := mctx.CreateLocalVariations(variants...)
					rlib := modules[0].(*Module)
					rlib.compiler.(libraryInterface).setRlibStd()
					rlib.Properties.RustSubName += RlibStdlibSuffix
				} else {
					variants := []string{"rlib-std", "dylib-std"}
					modules := mctx.CreateLocalVariations(variants...)

					rlib := modules[0].(*Module)
					dylib := modules[1].(*Module)
					rlib.compiler.(libraryInterface).setRlibStd()
					dylib.compiler.(libraryInterface).setDylibStd()
					if dylib.ModuleBase.ImageVariation().Variation == android.VendorRamdiskVariation {
						// TODO(b/165791368)
						// Disable rlibs that link against dylib-std on vendor ramdisk variations until those dylib
						// variants are properly supported.
						dylib.Disable()
					}
					rlib.Properties.RustSubName += RlibStdlibSuffix
				}
			}
		}
	}
}
