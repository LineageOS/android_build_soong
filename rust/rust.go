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
		ctx.BottomUp("rust_unit_tests", TestPerSrcMutator).Parallel()
	})
	pctx.Import("android/soong/rust/config")
}

type Flags struct {
	GlobalRustFlags []string      // Flags that apply globally to rust
	GlobalLinkFlags []string      // Flags that apply globally to linker
	RustFlags       []string      // Flags that apply to rust
	LinkFlags       []string      // Flags that apply to linker
	RustFlagsDeps   android.Paths // Files depended on by compiler flags
	Toolchain       config.Toolchain
}

type BaseProperties struct {
	AndroidMkRlibs         []string
	AndroidMkDylibs        []string
	AndroidMkProcMacroLibs []string
	AndroidMkSharedLibs    []string
	AndroidMkStaticLibs    []string
	SubName                string `blueprint:"mutated"`
}

type Module struct {
	android.ModuleBase
	android.DefaultableModuleBase

	Properties BaseProperties

	hod      android.HostOrDeviceSupported
	multilib android.Multilib

	compiler         compiler
	cachedToolchain  config.Toolchain
	subAndroidMkOnce map[subAndroidMkProvider]bool
	outputFile       android.OptionalPath
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
		if library, ok := mod.compiler.(libraryInterface); ok {
			if library.buildRlib() || library.buildDylib() {
				return true
			} else {
				return false
			}
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
	panic(fmt.Errorf("Static called on non-library module: %q", mod.BaseModuleName()))
}

func (mod *Module) Shared() bool {
	if mod.compiler != nil {
		if library, ok := mod.compiler.(libraryInterface); ok {
			return library.static()
		}
	}
	panic(fmt.Errorf("Shared called on non-library module: %q", mod.BaseModuleName()))
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

	CrtBegin android.OptionalPath
	CrtEnd   android.OptionalPath
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
		&TestProperties{},
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
		if _, ok := mod.compiler.(libraryInterface); ok {
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
		if _, ok := mod.compiler.(*libraryDecorator); ok {
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

var _ cc.LinkableInterface = (*Module)(nil)

func (mod *Module) Init() android.Module {
	mod.AddProperties(&mod.Properties)

	if mod.compiler != nil {
		mod.AddProperties(mod.compiler.compilerProps()...)
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
	toolchain() config.Toolchain
	baseModuleName() string
	CrateName() string
}

type depsContext struct {
	android.BottomUpMutatorContext
	moduleContextImpl
}

type moduleContext struct {
	android.ModuleContext
	moduleContextImpl
}

type moduleContextImpl struct {
	mod *Module
	ctx BaseModuleContext
}

func (ctx *moduleContextImpl) toolchain() config.Toolchain {
	return ctx.mod.toolchain(ctx.ctx)
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
		moduleContextImpl: moduleContextImpl{
			mod: mod,
		},
	}
	ctx.ctx = ctx

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
		outputFile := mod.compiler.compile(ctx, flags, deps)
		mod.outputFile = android.OptionalPathForPath(outputFile)
		mod.compiler.install(ctx, mod.outputFile.Path())
	}
}

func (mod *Module) deps(ctx DepsContext) Deps {
	deps := Deps{}

	if mod.compiler != nil {
		deps = mod.compiler.compilerDeps(ctx, deps)
	}

	deps.Rlibs = android.LastUniqueStrings(deps.Rlibs)
	deps.Dylibs = android.LastUniqueStrings(deps.Dylibs)
	deps.ProcMacros = android.LastUniqueStrings(deps.ProcMacros)
	deps.SharedLibs = android.LastUniqueStrings(deps.SharedLibs)
	deps.StaticLibs = android.LastUniqueStrings(deps.StaticLibs)

	return deps

}

func (ctx *moduleContextImpl) baseModuleName() string {
	return ctx.mod.ModuleBase.BaseModuleName()
}

func (ctx *moduleContextImpl) CrateName() string {
	return ctx.mod.CrateName()
}

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name       string
	library    bool
	proc_macro bool
}

var (
	rlibDepTag       = dependencyTag{name: "rlibTag", library: true}
	dylibDepTag      = dependencyTag{name: "dylib", library: true}
	procMacroDepTag  = dependencyTag{name: "procMacro", proc_macro: true}
	testPerSrcDepTag = dependencyTag{name: "rust_unit_tests"}
)

func (mod *Module) depsToPaths(ctx android.ModuleContext) PathDeps {
	var depPaths PathDeps

	directRlibDeps := []*Module{}
	directDylibDeps := []*Module{}
	directProcMacroDeps := []*Module{}
	directSharedLibDeps := [](cc.LinkableInterface){}
	directStaticLibDeps := [](cc.LinkableInterface){}

	ctx.VisitDirectDeps(func(dep android.Module) {
		depName := ctx.OtherModuleName(dep)
		depTag := ctx.OtherModuleDependencyTag(dep)
		if rustDep, ok := dep.(*Module); ok {
			//Handle Rust Modules

			linkFile := rustDep.outputFile
			if !linkFile.Valid() {
				ctx.ModuleErrorf("Invalid output file when adding dep %q to %q", depName, ctx.ModuleName())
			}

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
				directRlibDeps = append(directRlibDeps, rustDep)
				mod.Properties.AndroidMkRlibs = append(mod.Properties.AndroidMkRlibs, depName)
			case procMacroDepTag:
				directProcMacroDeps = append(directProcMacroDeps, rustDep)
				mod.Properties.AndroidMkProcMacroLibs = append(mod.Properties.AndroidMkProcMacroLibs, depName)
			}

			//Append the dependencies exportedDirs
			if lib, ok := rustDep.compiler.(*libraryDecorator); ok {
				depPaths.linkDirs = append(depPaths.linkDirs, lib.exportedDirs()...)
				depPaths.depFlags = append(depPaths.depFlags, lib.exportedDepFlags()...)
			}

			// Append this dependencies output to this mod's linkDirs so they can be exported to dependencies
			// This can be probably be refactored by defining a common exporter interface similar to cc's
			if depTag == dylibDepTag || depTag == rlibDepTag || depTag == procMacroDepTag {
				linkDir := linkPathFromFilePath(linkFile.Path())
				if lib, ok := mod.compiler.(*libraryDecorator); ok {
					lib.linkDirs = append(lib.linkDirs, linkDir)
				} else if procMacro, ok := mod.compiler.(*procMacroDecorator); ok {
					procMacro.linkDirs = append(procMacro.linkDirs, linkDir)
				}
			}

		}

		if ccDep, ok := dep.(cc.LinkableInterface); ok {
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
			switch depTag {
			case cc.StaticDepTag:
				depFlag = "-lstatic=" + libName
				depPaths.linkDirs = append(depPaths.linkDirs, linkPath)
				depPaths.depFlags = append(depPaths.depFlags, depFlag)
				directStaticLibDeps = append(directStaticLibDeps, ccDep)
				mod.Properties.AndroidMkStaticLibs = append(mod.Properties.AndroidMkStaticLibs, depName)
			case cc.SharedDepTag:
				depFlag = "-ldylib=" + libName
				depPaths.linkDirs = append(depPaths.linkDirs, linkPath)
				depPaths.depFlags = append(depPaths.depFlags, depFlag)
				directSharedLibDeps = append(directSharedLibDeps, ccDep)
				mod.Properties.AndroidMkSharedLibs = append(mod.Properties.AndroidMkSharedLibs, depName)
				exportDep = true
			case cc.CrtBeginDepTag:
				depPaths.CrtBegin = linkFile
			case cc.CrtEndDepTag:
				depPaths.CrtEnd = linkFile
			}

			// Make sure these dependencies are propagated
			if lib, ok := mod.compiler.(*libraryDecorator); ok && exportDep {
				lib.linkDirs = append(lib.linkDirs, linkPath)
				lib.depFlags = append(lib.depFlags, depFlag)
			} else if procMacro, ok := mod.compiler.(*procMacroDecorator); ok && exportDep {
				procMacro.linkDirs = append(procMacro.linkDirs, linkPath)
				procMacro.depFlags = append(procMacro.depFlags, depFlag)
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

	depPaths.RLibs = append(depPaths.RLibs, rlibDepFiles...)
	depPaths.DyLibs = append(depPaths.DyLibs, dylibDepFiles...)
	depPaths.SharedLibs = append(depPaths.SharedLibs, sharedLibDepFiles...)
	depPaths.StaticLibs = append(depPaths.StaticLibs, staticLibDepFiles...)
	depPaths.ProcMacros = append(depPaths.ProcMacros, procMacroDepFiles...)

	// Dedup exported flags from dependencies
	depPaths.linkDirs = android.FirstUniqueStrings(depPaths.linkDirs)
	depPaths.depFlags = android.FirstUniqueStrings(depPaths.depFlags)

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
		moduleContextImpl: moduleContextImpl{
			mod: mod,
		},
	}
	ctx.ctx = ctx

	deps := mod.deps(ctx)
	commonDepVariations := []blueprint.Variation{}
	commonDepVariations = append(commonDepVariations,
		blueprint.Variation{Mutator: "version", Variation: ""})
	if !mod.Host() {
		commonDepVariations = append(commonDepVariations,
			blueprint.Variation{Mutator: "image", Variation: android.CoreVariation})
	}

	actx.AddVariationDependencies(
		append(commonDepVariations, []blueprint.Variation{
			{Mutator: "rust_libraries", Variation: "rlib"},
			{Mutator: "link", Variation: ""}}...),
		rlibDepTag, deps.Rlibs...)
	actx.AddVariationDependencies(
		append(commonDepVariations, []blueprint.Variation{
			{Mutator: "rust_libraries", Variation: "dylib"},
			{Mutator: "link", Variation: ""}}...),
		dylibDepTag, deps.Dylibs...)

	actx.AddVariationDependencies(append(commonDepVariations,
		blueprint.Variation{Mutator: "link", Variation: "shared"}),
		cc.SharedDepTag, deps.SharedLibs...)
	actx.AddVariationDependencies(append(commonDepVariations,
		blueprint.Variation{Mutator: "link", Variation: "static"}),
		cc.StaticDepTag, deps.StaticLibs...)

	if deps.CrtBegin != "" {
		actx.AddVariationDependencies(commonDepVariations, cc.CrtBeginDepTag, deps.CrtBegin)
	}
	if deps.CrtEnd != "" {
		actx.AddVariationDependencies(commonDepVariations, cc.CrtEndDepTag, deps.CrtEnd)
	}

	// proc_macros are compiler plugins, and so we need the host arch variant as a dependendcy.
	actx.AddFarVariationDependencies(ctx.Config().BuildOSTarget.Variations(), procMacroDepTag, deps.ProcMacros...)
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

var Bool = proptools.Bool
var BoolDefault = proptools.BoolDefault
var String = proptools.String
var StringPtr = proptools.StringPtr
