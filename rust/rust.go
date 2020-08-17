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
	"fmt"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/rust/config"
)

var pctx = android.NewPackageContext("android/soong/rust")

func init() {
	// Only allow rust modules to be defined for certain projects

	android.AddNeverAllowRules(
		android.NeverAllow().
			NotIn(config.RustAllowedPaths...).
			ModuleType(config.RustModuleTypes...))

	android.RegisterModuleType("rust_defaults", defaultsFactory)
	android.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("rust_libraries", LibraryMutator).Parallel()
		ctx.BottomUp("rust_begin", BeginMutator).Parallel()
	})
	pctx.Import("android/soong/rust/config")
	pctx.ImportAs("ccConfig", "android/soong/cc/config")
}

type Flags struct {
	GlobalRustFlags []string // Flags that apply globally to rust
	GlobalLinkFlags []string // Flags that apply globally to linker
	RustFlags       []string // Flags that apply to rust
	LinkFlags       []string // Flags that apply to linker
	ClippyFlags     []string // Flags that apply to clippy-driver, during the linting
	Toolchain       config.Toolchain
	Coverage        bool
	Clippy          bool
}

type BaseProperties struct {
	AndroidMkRlibs         []string
	AndroidMkDylibs        []string
	AndroidMkProcMacroLibs []string
	AndroidMkSharedLibs    []string
	AndroidMkStaticLibs    []string

	SubName string `blueprint:"mutated"`

	PreventInstall bool
	HideFromMake   bool
}

type Module struct {
	android.ModuleBase
	android.DefaultableModuleBase

	Properties BaseProperties

	hod      android.HostOrDeviceSupported
	multilib android.Multilib

	compiler         compiler
	coverage         *coverage
	clippy           *clippy
	cachedToolchain  config.Toolchain
	sourceProvider   SourceProvider
	subAndroidMkOnce map[SubAndroidMkProvider]bool

	outputFile    android.OptionalPath
	generatedFile android.OptionalPath
}

func (mod *Module) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		if mod.sourceProvider != nil && (mod.compiler == nil || mod.compiler.Disabled()) {
			return mod.sourceProvider.Srcs(), nil
		} else {
			if mod.outputFile.Valid() {
				return android.Paths{mod.outputFile.Path()}, nil
			}
			return android.Paths{}, nil
		}
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

var _ android.ImageInterface = (*Module)(nil)

func (mod *Module) ImageMutatorBegin(ctx android.BaseModuleContext) {}

func (mod *Module) CoreVariantNeeded(ctx android.BaseModuleContext) bool {
	return true
}

func (mod *Module) RamdiskVariantNeeded(android.BaseModuleContext) bool {
	return mod.InRamdisk()
}

func (mod *Module) RecoveryVariantNeeded(android.BaseModuleContext) bool {
	return mod.InRecovery()
}

func (mod *Module) ExtraImageVariations(android.BaseModuleContext) []string {
	return nil
}

func (c *Module) SetImageVariation(ctx android.BaseModuleContext, variant string, module android.Module) {
}

func (mod *Module) BuildStubs() bool {
	return false
}

func (mod *Module) HasStubsVariants() bool {
	return false
}

func (mod *Module) SelectedStl() string {
	return ""
}

func (mod *Module) NonCcVariants() bool {
	if mod.compiler != nil {
		if _, ok := mod.compiler.(libraryInterface); ok {
			return false
		}
	}
	panic(fmt.Errorf("NonCcVariants called on non-library module: %q", mod.BaseModuleName()))
}

func (mod *Module) ApiLevel() string {
	panic(fmt.Errorf("Called ApiLevel on Rust module %q; stubs libraries are not yet supported.", mod.BaseModuleName()))
}

func (mod *Module) Static() bool {
	if mod.compiler != nil {
		if library, ok := mod.compiler.(libraryInterface); ok {
			return library.static()
		}
	}
	return false
}

func (mod *Module) Shared() bool {
	if mod.compiler != nil {
		if library, ok := mod.compiler.(libraryInterface); ok {
			return library.shared()
		}
	}
	return false
}

func (mod *Module) Toc() android.OptionalPath {
	if mod.compiler != nil {
		if _, ok := mod.compiler.(libraryInterface); ok {
			return android.OptionalPath{}
		}
	}
	panic(fmt.Errorf("Toc() called on non-library module: %q", mod.BaseModuleName()))
}

func (mod *Module) OnlyInRamdisk() bool {
	return false
}

func (mod *Module) OnlyInRecovery() bool {
	return false
}

func (mod *Module) UseSdk() bool {
	return false
}

func (mod *Module) UseVndk() bool {
	return false
}

func (mod *Module) MustUseVendorVariant() bool {
	return false
}

func (mod *Module) IsVndk() bool {
	return false
}

func (mod *Module) HasVendorVariant() bool {
	return false
}

func (mod *Module) SdkVersion() string {
	return ""
}

func (mod *Module) AlwaysSdk() bool {
	return false
}

func (mod *Module) IsSdkVariant() bool {
	return false
}

func (mod *Module) ToolchainLibrary() bool {
	return false
}

func (mod *Module) NdkPrebuiltStl() bool {
	return false
}

func (mod *Module) StubDecorator() bool {
	return false
}

type Deps struct {
	Dylibs     []string
	Rlibs      []string
	Rustlibs   []string
	ProcMacros []string
	SharedLibs []string
	StaticLibs []string

	CrtBegin, CrtEnd string
}

type PathDeps struct {
	DyLibs     RustLibraries
	RLibs      RustLibraries
	SharedLibs android.Paths
	StaticLibs android.Paths
	ProcMacros RustLibraries
	linkDirs   []string
	depFlags   []string
	//ReexportedDeps android.Paths

	// Used by bindgen modules which call clang
	depClangFlags         []string
	depIncludePaths       android.Paths
	depSystemIncludePaths android.Paths

	coverageFiles android.Paths

	CrtBegin android.OptionalPath
	CrtEnd   android.OptionalPath

	// Paths to generated source files
	SrcDeps android.Paths
}

type RustLibraries []RustLibrary

type RustLibrary struct {
	Path      android.Path
	CrateName string
}

type compiler interface {
	compilerFlags(ctx ModuleContext, flags Flags) Flags
	compilerProps() []interface{}
	compile(ctx ModuleContext, flags Flags, deps PathDeps) android.Path
	compilerDeps(ctx DepsContext, deps Deps) Deps
	crateName() string

	inData() bool
	install(ctx ModuleContext, path android.Path)
	relativeInstallPath() string

	nativeCoverage() bool

	Disabled() bool
	SetDisabled()
}

type exportedFlagsProducer interface {
	exportedLinkDirs() []string
	exportedDepFlags() []string
	exportLinkDirs(...string)
	exportDepFlags(...string)
}

type flagExporter struct {
	depFlags []string
	linkDirs []string
}

func (flagExporter *flagExporter) exportedLinkDirs() []string {
	return flagExporter.linkDirs
}

func (flagExporter *flagExporter) exportedDepFlags() []string {
	return flagExporter.depFlags
}

func (flagExporter *flagExporter) exportLinkDirs(dirs ...string) {
	flagExporter.linkDirs = android.FirstUniqueStrings(append(flagExporter.linkDirs, dirs...))
}

func (flagExporter *flagExporter) exportDepFlags(flags ...string) {
	flagExporter.depFlags = android.FirstUniqueStrings(append(flagExporter.depFlags, flags...))
}

var _ exportedFlagsProducer = (*flagExporter)(nil)

func NewFlagExporter() *flagExporter {
	return &flagExporter{
		depFlags: []string{},
		linkDirs: []string{},
	}
}

func (mod *Module) isCoverageVariant() bool {
	return mod.coverage.Properties.IsCoverageVariant
}

var _ cc.Coverage = (*Module)(nil)

func (mod *Module) IsNativeCoverageNeeded(ctx android.BaseModuleContext) bool {
	return mod.coverage != nil && mod.coverage.Properties.NeedCoverageVariant
}

func (mod *Module) PreventInstall() {
	mod.Properties.PreventInstall = true
}

func (mod *Module) HideFromMake() {
	mod.Properties.HideFromMake = true
}

func (mod *Module) MarkAsCoverageVariant(coverage bool) {
	mod.coverage.Properties.IsCoverageVariant = coverage
}

func (mod *Module) EnableCoverageIfNeeded() {
	mod.coverage.Properties.CoverageEnabled = mod.coverage.Properties.NeedCoverageBuild
}

func defaultsFactory() android.Module {
	return DefaultsFactory()
}

type Defaults struct {
	android.ModuleBase
	android.DefaultsModuleBase
}

func DefaultsFactory(props ...interface{}) android.Module {
	module := &Defaults{}

	module.AddProperties(props...)
	module.AddProperties(
		&BaseProperties{},
		&BaseCompilerProperties{},
		&BinaryCompilerProperties{},
		&LibraryCompilerProperties{},
		&ProcMacroCompilerProperties{},
		&PrebuiltProperties{},
		&SourceProviderProperties{},
		&TestProperties{},
		&cc.CoverageProperties{},
		&ClippyProperties{},
	)

	android.InitDefaultsModule(module)
	return module
}

func (mod *Module) CrateName() string {
	return mod.compiler.crateName()
}

func (mod *Module) CcLibrary() bool {
	if mod.compiler != nil {
		if _, ok := mod.compiler.(*libraryDecorator); ok {
			return true
		}
	}
	return false
}

func (mod *Module) CcLibraryInterface() bool {
	if mod.compiler != nil {
		// use build{Static,Shared}() instead of {static,shared}() here because this might be called before
		// VariantIs{Static,Shared} is set.
		if lib, ok := mod.compiler.(libraryInterface); ok && (lib.buildShared() || lib.buildStatic()) {
			return true
		}
	}
	return false
}

func (mod *Module) IncludeDirs() android.Paths {
	if mod.compiler != nil {
		if library, ok := mod.compiler.(*libraryDecorator); ok {
			return library.includeDirs
		}
	}
	panic(fmt.Errorf("IncludeDirs called on non-library module: %q", mod.BaseModuleName()))
}

func (mod *Module) SetStatic() {
	if mod.compiler != nil {
		if library, ok := mod.compiler.(libraryInterface); ok {
			library.setStatic()
			return
		}
	}
	panic(fmt.Errorf("SetStatic called on non-library module: %q", mod.BaseModuleName()))
}

func (mod *Module) SetShared() {
	if mod.compiler != nil {
		if library, ok := mod.compiler.(libraryInterface); ok {
			library.setShared()
			return
		}
	}
	panic(fmt.Errorf("SetShared called on non-library module: %q", mod.BaseModuleName()))
}

func (mod *Module) SetBuildStubs() {
	panic("SetBuildStubs not yet implemented for rust modules")
}

func (mod *Module) SetStubsVersions(string) {
	panic("SetStubsVersions not yet implemented for rust modules")
}

func (mod *Module) StubsVersion() string {
	panic("SetStubsVersions not yet implemented for rust modules")
}

func (mod *Module) BuildStaticVariant() bool {
	if mod.compiler != nil {
		if library, ok := mod.compiler.(libraryInterface); ok {
			return library.buildStatic()
		}
	}
	panic(fmt.Errorf("BuildStaticVariant called on non-library module: %q", mod.BaseModuleName()))
}

func (mod *Module) BuildSharedVariant() bool {
	if mod.compiler != nil {
		if library, ok := mod.compiler.(libraryInterface); ok {
			return library.buildShared()
		}
	}
	panic(fmt.Errorf("BuildSharedVariant called on non-library module: %q", mod.BaseModuleName()))
}

// Rust module deps don't have a link order (?)
func (mod *Module) SetDepsInLinkOrder([]android.Path) {}

func (mod *Module) GetDepsInLinkOrder() []android.Path {
	return []android.Path{}
}

func (mod *Module) GetStaticVariant() cc.LinkableInterface {
	return nil
}

func (mod *Module) Module() android.Module {
	return mod
}

func (mod *Module) StubsVersions() []string {
	// For now, Rust has no stubs versions.
	if mod.compiler != nil {
		if _, ok := mod.compiler.(libraryInterface); ok {
			return []string{}
		}
	}
	panic(fmt.Errorf("StubsVersions called on non-library module: %q", mod.BaseModuleName()))
}

func (mod *Module) OutputFile() android.OptionalPath {
	return mod.outputFile
}

func (mod *Module) InRecovery() bool {
	// For now, Rust has no notion of the recovery image
	return false
}
func (mod *Module) HasStaticVariant() bool {
	if mod.GetStaticVariant() != nil {
		return true
	}
	return false
}

func (mod *Module) CoverageFiles() android.Paths {
	if mod.compiler != nil {
		if !mod.compiler.nativeCoverage() {
			return android.Paths{}
		}
		if library, ok := mod.compiler.(*libraryDecorator); ok {
			if library.coverageFile != nil {
				return android.Paths{library.coverageFile}
			}
			return android.Paths{}
		}
	}
	panic(fmt.Errorf("CoverageFiles called on non-library module: %q", mod.BaseModuleName()))
}

var _ cc.LinkableInterface = (*Module)(nil)

func (mod *Module) Init() android.Module {
	mod.AddProperties(&mod.Properties)

	if mod.compiler != nil {
		mod.AddProperties(mod.compiler.compilerProps()...)
	}
	if mod.coverage != nil {
		mod.AddProperties(mod.coverage.props()...)
	}
	if mod.clippy != nil {
		mod.AddProperties(mod.clippy.props()...)
	}
	if mod.sourceProvider != nil {
		mod.AddProperties(mod.sourceProvider.SourceProviderProps()...)
	}

	android.InitAndroidArchModule(mod, mod.hod, mod.multilib)

	android.InitDefaultableModule(mod)

	// Explicitly disable unsupported targets.
	android.AddLoadHook(mod, func(ctx android.LoadHookContext) {
		disableTargets := struct {
			Target struct {
				Linux_bionic struct {
					Enabled *bool
				}
			}
		}{}
		disableTargets.Target.Linux_bionic.Enabled = proptools.BoolPtr(false)

		ctx.AppendProperties(&disableTargets)
	})

	return mod
}

func newBaseModule(hod android.HostOrDeviceSupported, multilib android.Multilib) *Module {
	return &Module{
		hod:      hod,
		multilib: multilib,
	}
}
func newModule(hod android.HostOrDeviceSupported, multilib android.Multilib) *Module {
	module := newBaseModule(hod, multilib)
	module.coverage = &coverage{}
	module.clippy = &clippy{}
	return module
}

type ModuleContext interface {
	android.ModuleContext
	ModuleContextIntf
}

type BaseModuleContext interface {
	android.BaseModuleContext
	ModuleContextIntf
}

type DepsContext interface {
	android.BottomUpMutatorContext
	ModuleContextIntf
}

type ModuleContextIntf interface {
	RustModule() *Module
	toolchain() config.Toolchain
}

type depsContext struct {
	android.BottomUpMutatorContext
}

type moduleContext struct {
	android.ModuleContext
}

type baseModuleContext struct {
	android.BaseModuleContext
}

func (ctx *moduleContext) RustModule() *Module {
	return ctx.Module().(*Module)
}

func (ctx *moduleContext) toolchain() config.Toolchain {
	return ctx.RustModule().toolchain(ctx)
}

func (ctx *depsContext) RustModule() *Module {
	return ctx.Module().(*Module)
}

func (ctx *depsContext) toolchain() config.Toolchain {
	return ctx.RustModule().toolchain(ctx)
}

func (ctx *baseModuleContext) RustModule() *Module {
	return ctx.Module().(*Module)
}

func (ctx *baseModuleContext) toolchain() config.Toolchain {
	return ctx.RustModule().toolchain(ctx)
}

func (mod *Module) nativeCoverage() bool {
	return mod.compiler != nil && mod.compiler.nativeCoverage()
}

func (mod *Module) toolchain(ctx android.BaseModuleContext) config.Toolchain {
	if mod.cachedToolchain == nil {
		mod.cachedToolchain = config.FindToolchain(ctx.Os(), ctx.Arch())
	}
	return mod.cachedToolchain
}

func (d *Defaults) GenerateAndroidBuildActions(ctx android.ModuleContext) {
}

func (mod *Module) GenerateAndroidBuildActions(actx android.ModuleContext) {
	ctx := &moduleContext{
		ModuleContext: actx,
	}

	toolchain := mod.toolchain(ctx)

	if !toolchain.Supported() {
		// This toolchain's unsupported, there's nothing to do for this mod.
		return
	}

	deps := mod.depsToPaths(ctx)
	flags := Flags{
		Toolchain: toolchain,
	}

	if mod.compiler != nil {
		flags = mod.compiler.compilerFlags(ctx, flags)
	}
	if mod.coverage != nil {
		flags, deps = mod.coverage.flags(ctx, flags, deps)
	}
	if mod.clippy != nil {
		flags, deps = mod.clippy.flags(ctx, flags, deps)
	}

	// SourceProvider needs to call GenerateSource() before compiler calls compile() so it can provide the source.
	// TODO(b/162588681) This shouldn't have to run for every variant.
	if mod.sourceProvider != nil {
		generatedFile := mod.sourceProvider.GenerateSource(ctx, deps)
		mod.generatedFile = android.OptionalPathForPath(generatedFile)
		mod.sourceProvider.setSubName(ctx.ModuleSubDir())
	}

	if mod.compiler != nil && !mod.compiler.Disabled() {
		outputFile := mod.compiler.compile(ctx, flags, deps)

		mod.outputFile = android.OptionalPathForPath(outputFile)
		if mod.outputFile.Valid() && !mod.Properties.PreventInstall {
			mod.compiler.install(ctx, mod.outputFile.Path())
		}
	}
}

func (mod *Module) deps(ctx DepsContext) Deps {
	deps := Deps{}

	if mod.compiler != nil {
		deps = mod.compiler.compilerDeps(ctx, deps)
	}
	if mod.sourceProvider != nil {
		deps = mod.sourceProvider.SourceProviderDeps(ctx, deps)
	}

	if mod.coverage != nil {
		deps = mod.coverage.deps(ctx, deps)
	}

	deps.Rlibs = android.LastUniqueStrings(deps.Rlibs)
	deps.Dylibs = android.LastUniqueStrings(deps.Dylibs)
	deps.Rustlibs = android.LastUniqueStrings(deps.Rustlibs)
	deps.ProcMacros = android.LastUniqueStrings(deps.ProcMacros)
	deps.SharedLibs = android.LastUniqueStrings(deps.SharedLibs)
	deps.StaticLibs = android.LastUniqueStrings(deps.StaticLibs)

	return deps

}

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name       string
	library    bool
	proc_macro bool
}

var (
	customBindgenDepTag = dependencyTag{name: "customBindgenTag"}
	rlibDepTag          = dependencyTag{name: "rlibTag", library: true}
	dylibDepTag         = dependencyTag{name: "dylib", library: true}
	procMacroDepTag     = dependencyTag{name: "procMacro", proc_macro: true}
	testPerSrcDepTag    = dependencyTag{name: "rust_unit_tests"}
)

type autoDep struct {
	variation string
	depTag    dependencyTag
}

var (
	rlibAutoDep  = autoDep{variation: "rlib", depTag: rlibDepTag}
	dylibAutoDep = autoDep{variation: "dylib", depTag: dylibDepTag}
)

type autoDeppable interface {
	autoDep() autoDep
}

func (mod *Module) begin(ctx BaseModuleContext) {
	if mod.coverage != nil {
		mod.coverage.begin(ctx)
	}
}

func (mod *Module) depsToPaths(ctx android.ModuleContext) PathDeps {
	var depPaths PathDeps

	directRlibDeps := []*Module{}
	directDylibDeps := []*Module{}
	directProcMacroDeps := []*Module{}
	directSharedLibDeps := [](cc.LinkableInterface){}
	directStaticLibDeps := [](cc.LinkableInterface){}
	directSrcProvidersDeps := []*Module{}
	directSrcDeps := [](android.SourceFileProducer){}

	ctx.VisitDirectDeps(func(dep android.Module) {
		depName := ctx.OtherModuleName(dep)
		depTag := ctx.OtherModuleDependencyTag(dep)
		if rustDep, ok := dep.(*Module); ok && !rustDep.CcLibraryInterface() {
			//Handle Rust Modules

			switch depTag {
			case dylibDepTag:
				dylib, ok := rustDep.compiler.(libraryInterface)
				if !ok || !dylib.dylib() {
					ctx.ModuleErrorf("mod %q not an dylib library", depName)
					return
				}
				directDylibDeps = append(directDylibDeps, rustDep)
				mod.Properties.AndroidMkDylibs = append(mod.Properties.AndroidMkDylibs, depName)
			case rlibDepTag:
				rlib, ok := rustDep.compiler.(libraryInterface)
				if !ok || !rlib.rlib() {
					ctx.ModuleErrorf("mod %q not an rlib library", depName)
					return
				}
				depPaths.coverageFiles = append(depPaths.coverageFiles, rustDep.CoverageFiles()...)
				directRlibDeps = append(directRlibDeps, rustDep)
				mod.Properties.AndroidMkRlibs = append(mod.Properties.AndroidMkRlibs, depName)
			case procMacroDepTag:
				directProcMacroDeps = append(directProcMacroDeps, rustDep)
				mod.Properties.AndroidMkProcMacroLibs = append(mod.Properties.AndroidMkProcMacroLibs, depName)
			case android.SourceDepTag:
				// Since these deps are added in path_properties.go via AddDependencies, we need to ensure the correct
				// OS/Arch variant is used.
				var helper string
				if ctx.Host() {
					helper = "missing 'host_supported'?"
				} else {
					helper = "device module defined?"
				}

				if dep.Target().Os != ctx.Os() {
					ctx.ModuleErrorf("OS mismatch on dependency %q (%s)", dep.Name(), helper)
					return
				} else if dep.Target().Arch.ArchType != ctx.Arch().ArchType {
					ctx.ModuleErrorf("Arch mismatch on dependency %q (%s)", dep.Name(), helper)
					return
				}
				directSrcProvidersDeps = append(directSrcProvidersDeps, rustDep)
			}

			//Append the dependencies exportedDirs, except for proc-macros which target a different arch/OS
			if lib, ok := rustDep.compiler.(exportedFlagsProducer); ok && depTag != procMacroDepTag {
				depPaths.linkDirs = append(depPaths.linkDirs, lib.exportedLinkDirs()...)
				depPaths.depFlags = append(depPaths.depFlags, lib.exportedDepFlags()...)
			}

			if depTag == dylibDepTag || depTag == rlibDepTag || depTag == procMacroDepTag {
				linkFile := rustDep.outputFile
				if !linkFile.Valid() {
					ctx.ModuleErrorf("Invalid output file when adding dep %q to %q",
						depName, ctx.ModuleName())
					return
				}
				linkDir := linkPathFromFilePath(linkFile.Path())
				if lib, ok := mod.compiler.(exportedFlagsProducer); ok {
					lib.exportLinkDirs(linkDir)
				}
			}

		} else if ccDep, ok := dep.(cc.LinkableInterface); ok {
			//Handle C dependencies
			if _, ok := ccDep.(*Module); !ok {
				if ccDep.Module().Target().Os != ctx.Os() {
					ctx.ModuleErrorf("OS mismatch between %q and %q", ctx.ModuleName(), depName)
					return
				}
				if ccDep.Module().Target().Arch.ArchType != ctx.Arch().ArchType {
					ctx.ModuleErrorf("Arch mismatch between %q and %q", ctx.ModuleName(), depName)
					return
				}
			}
			linkFile := ccDep.OutputFile()
			linkPath := linkPathFromFilePath(linkFile.Path())
			libName := libNameFromFilePath(linkFile.Path())
			depFlag := "-l" + libName

			if !linkFile.Valid() {
				ctx.ModuleErrorf("Invalid output file when adding dep %q to %q", depName, ctx.ModuleName())
			}

			exportDep := false
			switch {
			case cc.IsStaticDepTag(depTag):
				depFlag = "-lstatic=" + libName
				depPaths.linkDirs = append(depPaths.linkDirs, linkPath)
				depPaths.depFlags = append(depPaths.depFlags, depFlag)
				depPaths.depIncludePaths = append(depPaths.depIncludePaths, ccDep.IncludeDirs()...)
				if mod, ok := ccDep.(*cc.Module); ok {
					depPaths.depSystemIncludePaths = append(depPaths.depSystemIncludePaths, mod.ExportedSystemIncludeDirs()...)
					depPaths.depClangFlags = append(depPaths.depClangFlags, mod.ExportedFlags()...)
				}
				depPaths.coverageFiles = append(depPaths.coverageFiles, ccDep.CoverageFiles()...)
				directStaticLibDeps = append(directStaticLibDeps, ccDep)
				mod.Properties.AndroidMkStaticLibs = append(mod.Properties.AndroidMkStaticLibs, depName)
			case cc.IsSharedDepTag(depTag):
				depFlag = "-ldylib=" + libName
				depPaths.linkDirs = append(depPaths.linkDirs, linkPath)
				depPaths.depFlags = append(depPaths.depFlags, depFlag)
				depPaths.depIncludePaths = append(depPaths.depIncludePaths, ccDep.IncludeDirs()...)
				if mod, ok := ccDep.(*cc.Module); ok {
					depPaths.depSystemIncludePaths = append(depPaths.depSystemIncludePaths, mod.ExportedSystemIncludeDirs()...)
					depPaths.depClangFlags = append(depPaths.depClangFlags, mod.ExportedFlags()...)
				}
				directSharedLibDeps = append(directSharedLibDeps, ccDep)
				mod.Properties.AndroidMkSharedLibs = append(mod.Properties.AndroidMkSharedLibs, depName)
				exportDep = true
			case depTag == cc.CrtBeginDepTag:
				depPaths.CrtBegin = linkFile
			case depTag == cc.CrtEndDepTag:
				depPaths.CrtEnd = linkFile
			}

			// Make sure these dependencies are propagated
			if lib, ok := mod.compiler.(exportedFlagsProducer); ok && exportDep {
				lib.exportLinkDirs(linkPath)
				lib.exportDepFlags(depFlag)
			}
		}

		if srcDep, ok := dep.(android.SourceFileProducer); ok {
			switch depTag {
			case android.SourceDepTag:
				// These are usually genrules which don't have per-target variants.
				directSrcDeps = append(directSrcDeps, srcDep)
			}
		}
	})

	var rlibDepFiles RustLibraries
	for _, dep := range directRlibDeps {
		rlibDepFiles = append(rlibDepFiles, RustLibrary{Path: dep.outputFile.Path(), CrateName: dep.CrateName()})
	}
	var dylibDepFiles RustLibraries
	for _, dep := range directDylibDeps {
		dylibDepFiles = append(dylibDepFiles, RustLibrary{Path: dep.outputFile.Path(), CrateName: dep.CrateName()})
	}
	var procMacroDepFiles RustLibraries
	for _, dep := range directProcMacroDeps {
		procMacroDepFiles = append(procMacroDepFiles, RustLibrary{Path: dep.outputFile.Path(), CrateName: dep.CrateName()})
	}

	var staticLibDepFiles android.Paths
	for _, dep := range directStaticLibDeps {
		staticLibDepFiles = append(staticLibDepFiles, dep.OutputFile().Path())
	}

	var sharedLibDepFiles android.Paths
	for _, dep := range directSharedLibDeps {
		sharedLibDepFiles = append(sharedLibDepFiles, dep.OutputFile().Path())
	}

	var srcProviderDepFiles android.Paths
	for _, dep := range directSrcProvidersDeps {
		srcs, _ := dep.OutputFiles("")
		srcProviderDepFiles = append(srcProviderDepFiles, srcs...)
	}
	for _, dep := range directSrcDeps {
		srcs := dep.Srcs()
		srcProviderDepFiles = append(srcProviderDepFiles, srcs...)
	}

	depPaths.RLibs = append(depPaths.RLibs, rlibDepFiles...)
	depPaths.DyLibs = append(depPaths.DyLibs, dylibDepFiles...)
	depPaths.SharedLibs = append(depPaths.SharedLibs, sharedLibDepFiles...)
	depPaths.StaticLibs = append(depPaths.StaticLibs, staticLibDepFiles...)
	depPaths.ProcMacros = append(depPaths.ProcMacros, procMacroDepFiles...)
	depPaths.SrcDeps = append(depPaths.SrcDeps, srcProviderDepFiles...)

	// Dedup exported flags from dependencies
	depPaths.linkDirs = android.FirstUniqueStrings(depPaths.linkDirs)
	depPaths.depFlags = android.FirstUniqueStrings(depPaths.depFlags)
	depPaths.depClangFlags = android.FirstUniqueStrings(depPaths.depClangFlags)
	depPaths.depIncludePaths = android.FirstUniquePaths(depPaths.depIncludePaths)
	depPaths.depSystemIncludePaths = android.FirstUniquePaths(depPaths.depSystemIncludePaths)

	return depPaths
}

func (mod *Module) InstallInData() bool {
	if mod.compiler == nil {
		return false
	}
	return mod.compiler.inData()
}

func linkPathFromFilePath(filepath android.Path) string {
	return strings.Split(filepath.String(), filepath.Base())[0]
}

func libNameFromFilePath(filepath android.Path) string {
	libName := strings.TrimSuffix(filepath.Base(), filepath.Ext())
	if strings.HasPrefix(libName, "lib") {
		libName = libName[3:]
	}
	return libName
}

func (mod *Module) DepsMutator(actx android.BottomUpMutatorContext) {
	ctx := &depsContext{
		BottomUpMutatorContext: actx,
	}

	deps := mod.deps(ctx)
	commonDepVariations := []blueprint.Variation{}
	if cc.VersionVariantAvailable(mod) {
		commonDepVariations = append(commonDepVariations,
			blueprint.Variation{Mutator: "version", Variation: ""})
	}
	if !mod.Host() {
		commonDepVariations = append(commonDepVariations,
			blueprint.Variation{Mutator: "image", Variation: android.CoreVariation})
	}
	actx.AddVariationDependencies(
		append(commonDepVariations, []blueprint.Variation{
			{Mutator: "rust_libraries", Variation: "rlib"}}...),
		rlibDepTag, deps.Rlibs...)
	actx.AddVariationDependencies(
		append(commonDepVariations, []blueprint.Variation{
			{Mutator: "rust_libraries", Variation: "dylib"}}...),
		dylibDepTag, deps.Dylibs...)

	if deps.Rustlibs != nil {
		autoDep := mod.compiler.(autoDeppable).autoDep()
		actx.AddVariationDependencies(
			append(commonDepVariations, []blueprint.Variation{
				{Mutator: "rust_libraries", Variation: autoDep.variation}}...),
			autoDep.depTag, deps.Rustlibs...)
	}

	actx.AddVariationDependencies(append(commonDepVariations,
		blueprint.Variation{Mutator: "link", Variation: "shared"}),
		cc.SharedDepTag(), deps.SharedLibs...)
	actx.AddVariationDependencies(append(commonDepVariations,
		blueprint.Variation{Mutator: "link", Variation: "static"}),
		cc.StaticDepTag(), deps.StaticLibs...)

	crtVariations := append(cc.GetCrtVariations(ctx, mod), commonDepVariations...)
	if deps.CrtBegin != "" {
		actx.AddVariationDependencies(crtVariations, cc.CrtBeginDepTag, deps.CrtBegin)
	}
	if deps.CrtEnd != "" {
		actx.AddVariationDependencies(crtVariations, cc.CrtEndDepTag, deps.CrtEnd)
	}

	if mod.sourceProvider != nil {
		if bindgen, ok := mod.sourceProvider.(*bindgenDecorator); ok &&
			bindgen.Properties.Custom_bindgen != "" {
			actx.AddFarVariationDependencies(ctx.Config().BuildOSTarget.Variations(), customBindgenDepTag,
				bindgen.Properties.Custom_bindgen)
		}
	}
	// proc_macros are compiler plugins, and so we need the host arch variant as a dependendcy.
	actx.AddFarVariationDependencies(ctx.Config().BuildOSTarget.Variations(), procMacroDepTag, deps.ProcMacros...)
}

func BeginMutator(ctx android.BottomUpMutatorContext) {
	if mod, ok := ctx.Module().(*Module); ok && mod.Enabled() {
		mod.beginMutator(ctx)
	}
}

func (mod *Module) beginMutator(actx android.BottomUpMutatorContext) {
	ctx := &baseModuleContext{
		BaseModuleContext: actx,
	}

	mod.begin(ctx)
}

func (mod *Module) Name() string {
	name := mod.ModuleBase.Name()
	if p, ok := mod.compiler.(interface {
		Name(string) string
	}); ok {
		name = p.Name(name)
	}
	return name
}

func (mod *Module) disableClippy() {
	if mod.clippy != nil {
		mod.clippy.Properties.Clippy_lints = proptools.StringPtr("none")
	}
}

var _ android.HostToolProvider = (*Module)(nil)

func (mod *Module) HostToolPath() android.OptionalPath {
	if !mod.Host() {
		return android.OptionalPath{}
	}
	if binary, ok := mod.compiler.(*binaryDecorator); ok {
		return android.OptionalPathForPath(binary.baseCompiler.path)
	}
	return android.OptionalPath{}
}

var Bool = proptools.Bool
var BoolDefault = proptools.BoolDefault
var String = proptools.String
var StringPtr = proptools.StringPtr

var _ android.OutputFileProducer = (*Module)(nil)
