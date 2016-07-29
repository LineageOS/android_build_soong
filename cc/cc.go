// Copyright 2015 Google Inc. All rights reserved.
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

// This file contains the module types for compiling C/C++ for Android, and converts the properties
// into the flags and filenames necessary to pass to the compiler.  The final creation of the rules
// is handled in builder.go

import (
	"fmt"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong"
	"android/soong/android"
	"android/soong/genrule"
)

func init() {
	soong.RegisterModuleType("cc_defaults", defaultsFactory)

	soong.RegisterModuleType("toolchain_library", toolchainLibraryFactory)

	// LinkageMutator must be registered after common.ArchMutator, but that is guaranteed by
	// the Go initialization order because this package depends on common, so common's init
	// functions will run first.
	android.RegisterBottomUpMutator("link", linkageMutator)
	android.RegisterBottomUpMutator("ndk_api", ndkApiMutator)
	android.RegisterBottomUpMutator("test_per_src", testPerSrcMutator)
	android.RegisterBottomUpMutator("deps", depsMutator)

	android.RegisterTopDownMutator("asan_deps", sanitizerDepsMutator(asan))
	android.RegisterBottomUpMutator("asan", sanitizerMutator(asan))

	android.RegisterTopDownMutator("tsan_deps", sanitizerDepsMutator(tsan))
	android.RegisterBottomUpMutator("tsan", sanitizerMutator(tsan))
}

type Deps struct {
	SharedLibs, LateSharedLibs                  []string
	StaticLibs, LateStaticLibs, WholeStaticLibs []string

	ReexportSharedLibHeaders, ReexportStaticLibHeaders []string

	ObjFiles []string

	GeneratedSources []string
	GeneratedHeaders []string

	CrtBegin, CrtEnd string
}

type PathDeps struct {
	SharedLibs, LateSharedLibs                  android.Paths
	StaticLibs, LateStaticLibs, WholeStaticLibs android.Paths

	ObjFiles               android.Paths
	WholeStaticLibObjFiles android.Paths

	GeneratedSources android.Paths
	GeneratedHeaders android.Paths

	Flags, ReexportedFlags []string

	CrtBegin, CrtEnd android.OptionalPath
}

type Flags struct {
	GlobalFlags []string // Flags that apply to C, C++, and assembly source files
	AsFlags     []string // Flags that apply to assembly source files
	CFlags      []string // Flags that apply to C and C++ source files
	ConlyFlags  []string // Flags that apply to C source files
	CppFlags    []string // Flags that apply to C++ source files
	YaccFlags   []string // Flags that apply to Yacc source files
	LdFlags     []string // Flags that apply to linker command lines
	libFlags    []string // Flags to add libraries early to the link order

	Nocrt     bool
	Toolchain Toolchain
	Clang     bool

	RequiredInstructionSet string
	DynamicLinker          string

	CFlagsDeps android.Paths // Files depended on by compiler flags
}

type ObjectLinkerProperties struct {
	// names of other cc_object modules to link into this module using partial linking
	Objs []string `android:"arch_variant"`
}

// Properties used to compile all C or C++ modules
type BaseProperties struct {
	// compile module with clang instead of gcc
	Clang *bool `android:"arch_variant"`

	// Minimum sdk version supported when compiling against the ndk
	Sdk_version string

	// don't insert default compiler flags into asflags, cflags,
	// cppflags, conlyflags, ldflags, or include_dirs
	No_default_compiler_flags *bool

	AndroidMkSharedLibs []string `blueprint:"mutated"`
	HideFromMake        bool     `blueprint:"mutated"`
}

type UnusedProperties struct {
	Native_coverage *bool
	Required        []string
	Tags            []string
}

type ModuleContextIntf interface {
	static() bool
	staticBinary() bool
	clang() bool
	toolchain() Toolchain
	noDefaultCompilerFlags() bool
	sdk() bool
	sdkVersion() string
	selectedStl() string
}

type ModuleContext interface {
	android.ModuleContext
	ModuleContextIntf
}

type BaseModuleContext interface {
	android.BaseContext
	ModuleContextIntf
}

type CustomizerFlagsContext interface {
	BaseModuleContext
	AppendCflags(...string)
	AppendLdflags(...string)
	AppendAsflags(...string)
}

type Customizer interface {
	Flags(CustomizerFlagsContext)
	Properties() []interface{}
}

type feature interface {
	begin(ctx BaseModuleContext)
	deps(ctx BaseModuleContext, deps Deps) Deps
	flags(ctx ModuleContext, flags Flags) Flags
	props() []interface{}
}

type compiler interface {
	feature
	appendCflags([]string)
	appendAsflags([]string)
	compile(ctx ModuleContext, flags Flags, deps PathDeps) android.Paths
}

type linker interface {
	feature
	link(ctx ModuleContext, flags Flags, deps PathDeps, objFiles android.Paths) android.Path
	appendLdflags([]string)
	installable() bool
}

type installer interface {
	props() []interface{}
	install(ctx ModuleContext, path android.Path)
	inData() bool
}

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name    string
	library bool

	reexportFlags bool
}

var (
	sharedDepTag       = dependencyTag{name: "shared", library: true}
	sharedExportDepTag = dependencyTag{name: "shared", library: true, reexportFlags: true}
	lateSharedDepTag   = dependencyTag{name: "late shared", library: true}
	staticDepTag       = dependencyTag{name: "static", library: true}
	staticExportDepTag = dependencyTag{name: "static", library: true, reexportFlags: true}
	lateStaticDepTag   = dependencyTag{name: "late static", library: true}
	wholeStaticDepTag  = dependencyTag{name: "whole static", library: true, reexportFlags: true}
	genSourceDepTag    = dependencyTag{name: "gen source"}
	genHeaderDepTag    = dependencyTag{name: "gen header"}
	objDepTag          = dependencyTag{name: "obj"}
	crtBeginDepTag     = dependencyTag{name: "crtbegin"}
	crtEndDepTag       = dependencyTag{name: "crtend"}
	reuseObjTag        = dependencyTag{name: "reuse objects"}
	ndkStubDepTag      = dependencyTag{name: "ndk stub", library: true}
	ndkLateStubDepTag  = dependencyTag{name: "ndk late stub", library: true}
)

// Module contains the properties and members used by all C/C++ module types, and implements
// the blueprint.Module interface.  It delegates to compiler, linker, and installer interfaces
// to construct the output file.  Behavior can be customized with a Customizer interface
type Module struct {
	android.ModuleBase
	android.DefaultableModule

	Properties BaseProperties
	unused     UnusedProperties

	// initialize before calling Init
	hod      android.HostOrDeviceSupported
	multilib android.Multilib

	// delegates, initialize before calling Init
	Customizer Customizer
	features   []feature
	compiler   compiler
	linker     linker
	installer  installer
	stl        *stl
	sanitize   *sanitize

	androidMkSharedLibDeps []string

	outputFile android.OptionalPath

	cachedToolchain Toolchain
}

func (c *Module) Init() (blueprint.Module, []interface{}) {
	props := []interface{}{&c.Properties, &c.unused}
	if c.Customizer != nil {
		props = append(props, c.Customizer.Properties()...)
	}
	if c.compiler != nil {
		props = append(props, c.compiler.props()...)
	}
	if c.linker != nil {
		props = append(props, c.linker.props()...)
	}
	if c.installer != nil {
		props = append(props, c.installer.props()...)
	}
	if c.stl != nil {
		props = append(props, c.stl.props()...)
	}
	if c.sanitize != nil {
		props = append(props, c.sanitize.props()...)
	}
	for _, feature := range c.features {
		props = append(props, feature.props()...)
	}

	_, props = android.InitAndroidArchModule(c, c.hod, c.multilib, props...)

	return android.InitDefaultableModule(c, c, props...)
}

type baseModuleContext struct {
	android.BaseContext
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

func (ctx *moduleContextImpl) AppendCflags(flags ...string) {
	CheckBadCompilerFlags(ctx.ctx, "", flags)
	ctx.mod.compiler.appendCflags(flags)
}

func (ctx *moduleContextImpl) AppendAsflags(flags ...string) {
	CheckBadCompilerFlags(ctx.ctx, "", flags)
	ctx.mod.compiler.appendAsflags(flags)
}

func (ctx *moduleContextImpl) AppendLdflags(flags ...string) {
	CheckBadLinkerFlags(ctx.ctx, "", flags)
	ctx.mod.linker.appendLdflags(flags)
}

func (ctx *moduleContextImpl) clang() bool {
	return ctx.mod.clang(ctx.ctx)
}

func (ctx *moduleContextImpl) toolchain() Toolchain {
	return ctx.mod.toolchain(ctx.ctx)
}

func (ctx *moduleContextImpl) static() bool {
	if ctx.mod.linker == nil {
		panic(fmt.Errorf("static called on module %q with no linker", ctx.ctx.ModuleName()))
	}
	if linker, ok := ctx.mod.linker.(baseLinkerInterface); ok {
		return linker.static()
	} else {
		panic(fmt.Errorf("static called on module %q that doesn't use base linker", ctx.ctx.ModuleName()))
	}
}

func (ctx *moduleContextImpl) staticBinary() bool {
	if ctx.mod.linker == nil {
		panic(fmt.Errorf("staticBinary called on module %q with no linker", ctx.ctx.ModuleName()))
	}
	if linker, ok := ctx.mod.linker.(baseLinkerInterface); ok {
		return linker.staticBinary()
	} else {
		panic(fmt.Errorf("staticBinary called on module %q that doesn't use base linker", ctx.ctx.ModuleName()))
	}
}

func (ctx *moduleContextImpl) noDefaultCompilerFlags() bool {
	return Bool(ctx.mod.Properties.No_default_compiler_flags)
}

func (ctx *moduleContextImpl) sdk() bool {
	if ctx.ctx.Device() {
		return ctx.mod.Properties.Sdk_version != ""
	}
	return false
}

func (ctx *moduleContextImpl) sdkVersion() string {
	if ctx.ctx.Device() {
		return ctx.mod.Properties.Sdk_version
	}
	return ""
}

func (ctx *moduleContextImpl) selectedStl() string {
	if stl := ctx.mod.stl; stl != nil {
		return stl.Properties.SelectedStl
	}
	return ""
}

func newBaseModule(hod android.HostOrDeviceSupported, multilib android.Multilib) *Module {
	return &Module{
		hod:      hod,
		multilib: multilib,
	}
}

func newModule(hod android.HostOrDeviceSupported, multilib android.Multilib) *Module {
	module := newBaseModule(hod, multilib)
	module.stl = &stl{}
	module.sanitize = &sanitize{}
	return module
}

func (c *Module) GenerateAndroidBuildActions(actx android.ModuleContext) {
	ctx := &moduleContext{
		ModuleContext: actx,
		moduleContextImpl: moduleContextImpl{
			mod: c,
		},
	}
	ctx.ctx = ctx

	if c.Customizer != nil {
		c.Customizer.Flags(ctx)
	}

	flags := Flags{
		Toolchain: c.toolchain(ctx),
		Clang:     c.clang(ctx),
	}
	if c.compiler != nil {
		flags = c.compiler.flags(ctx, flags)
	}
	if c.linker != nil {
		flags = c.linker.flags(ctx, flags)
	}
	if c.stl != nil {
		flags = c.stl.flags(ctx, flags)
	}
	if c.sanitize != nil {
		flags = c.sanitize.flags(ctx, flags)
	}
	for _, feature := range c.features {
		flags = feature.flags(ctx, flags)
	}
	if ctx.Failed() {
		return
	}

	flags.CFlags, _ = filterList(flags.CFlags, illegalFlags)
	flags.CppFlags, _ = filterList(flags.CppFlags, illegalFlags)
	flags.ConlyFlags, _ = filterList(flags.ConlyFlags, illegalFlags)

	// Optimization to reduce size of build.ninja
	// Replace the long list of flags for each file with a module-local variable
	ctx.Variable(pctx, "cflags", strings.Join(flags.CFlags, " "))
	ctx.Variable(pctx, "cppflags", strings.Join(flags.CppFlags, " "))
	ctx.Variable(pctx, "asflags", strings.Join(flags.AsFlags, " "))
	flags.CFlags = []string{"$cflags"}
	flags.CppFlags = []string{"$cppflags"}
	flags.AsFlags = []string{"$asflags"}

	deps := c.depsToPaths(ctx)
	if ctx.Failed() {
		return
	}

	flags.GlobalFlags = append(flags.GlobalFlags, deps.Flags...)

	var objFiles android.Paths
	if c.compiler != nil {
		objFiles = c.compiler.compile(ctx, flags, deps)
		if ctx.Failed() {
			return
		}
	}

	if c.linker != nil {
		outputFile := c.linker.link(ctx, flags, deps, objFiles)
		if ctx.Failed() {
			return
		}
		c.outputFile = android.OptionalPathForPath(outputFile)

		if c.installer != nil && c.linker.installable() {
			c.installer.install(ctx, outputFile)
			if ctx.Failed() {
				return
			}
		}
	}
}

func (c *Module) toolchain(ctx BaseModuleContext) Toolchain {
	if c.cachedToolchain == nil {
		arch := ctx.Arch()
		os := ctx.Os()
		factory := toolchainFactories[os][arch.ArchType]
		if factory == nil {
			ctx.ModuleErrorf("Toolchain not found for %s arch %q", os.String(), arch.String())
			return nil
		}
		c.cachedToolchain = factory(arch)
	}
	return c.cachedToolchain
}

func (c *Module) begin(ctx BaseModuleContext) {
	if c.compiler != nil {
		c.compiler.begin(ctx)
	}
	if c.linker != nil {
		c.linker.begin(ctx)
	}
	if c.stl != nil {
		c.stl.begin(ctx)
	}
	if c.sanitize != nil {
		c.sanitize.begin(ctx)
	}
	for _, feature := range c.features {
		feature.begin(ctx)
	}
}

func (c *Module) deps(ctx BaseModuleContext) Deps {
	deps := Deps{}

	if c.compiler != nil {
		deps = c.compiler.deps(ctx, deps)
	}
	if c.linker != nil {
		deps = c.linker.deps(ctx, deps)
	}
	if c.stl != nil {
		deps = c.stl.deps(ctx, deps)
	}
	if c.sanitize != nil {
		deps = c.sanitize.deps(ctx, deps)
	}
	for _, feature := range c.features {
		deps = feature.deps(ctx, deps)
	}

	deps.WholeStaticLibs = lastUniqueElements(deps.WholeStaticLibs)
	deps.StaticLibs = lastUniqueElements(deps.StaticLibs)
	deps.LateStaticLibs = lastUniqueElements(deps.LateStaticLibs)
	deps.SharedLibs = lastUniqueElements(deps.SharedLibs)
	deps.LateSharedLibs = lastUniqueElements(deps.LateSharedLibs)

	for _, lib := range deps.ReexportSharedLibHeaders {
		if !inList(lib, deps.SharedLibs) {
			ctx.PropertyErrorf("export_shared_lib_headers", "Shared library not in shared_libs: '%s'", lib)
		}
	}

	for _, lib := range deps.ReexportStaticLibHeaders {
		if !inList(lib, deps.StaticLibs) {
			ctx.PropertyErrorf("export_static_lib_headers", "Static library not in static_libs: '%s'", lib)
		}
	}

	return deps
}

func (c *Module) depsMutator(actx android.BottomUpMutatorContext) {
	ctx := &baseModuleContext{
		BaseContext: actx,
		moduleContextImpl: moduleContextImpl{
			mod: c,
		},
	}
	ctx.ctx = ctx

	c.begin(ctx)

	deps := c.deps(ctx)

	c.Properties.AndroidMkSharedLibs = append(c.Properties.AndroidMkSharedLibs, deps.SharedLibs...)
	c.Properties.AndroidMkSharedLibs = append(c.Properties.AndroidMkSharedLibs, deps.LateSharedLibs...)

	variantNdkLibs := []string{}
	variantLateNdkLibs := []string{}
	if ctx.sdk() {
		version := ctx.sdkVersion()

		// Rewrites the names of shared libraries into the names of the NDK
		// libraries where appropriate. This returns two slices.
		//
		// The first is a list of non-variant shared libraries (either rewritten
		// NDK libraries to the modules in prebuilts/ndk, or not rewritten
		// because they are not NDK libraries).
		//
		// The second is a list of ndk_library modules. These need to be
		// separated because they are a variation dependency and must be added
		// in a different manner.
		rewriteNdkLibs := func(list []string) ([]string, []string) {
			variantLibs := []string{}
			nonvariantLibs := []string{}
			for _, entry := range list {
				if inList(entry, ndkPrebuiltSharedLibraries) {
					if !inList(entry, ndkMigratedLibs) {
						nonvariantLibs = append(nonvariantLibs, entry+".ndk."+version)
					} else {
						variantLibs = append(variantLibs, entry+ndkLibrarySuffix)
					}
				} else {
					nonvariantLibs = append(variantLibs, entry)
				}
			}
			return nonvariantLibs, variantLibs
		}

		deps.SharedLibs, variantNdkLibs = rewriteNdkLibs(deps.SharedLibs)
		deps.LateSharedLibs, variantLateNdkLibs = rewriteNdkLibs(deps.LateSharedLibs)
	}

	actx.AddVariationDependencies([]blueprint.Variation{{"link", "static"}}, wholeStaticDepTag,
		deps.WholeStaticLibs...)

	for _, lib := range deps.StaticLibs {
		depTag := staticDepTag
		if inList(lib, deps.ReexportStaticLibHeaders) {
			depTag = staticExportDepTag
		}
		actx.AddVariationDependencies([]blueprint.Variation{{"link", "static"}}, depTag, lib)
	}

	actx.AddVariationDependencies([]blueprint.Variation{{"link", "static"}}, lateStaticDepTag,
		deps.LateStaticLibs...)

	for _, lib := range deps.SharedLibs {
		depTag := sharedDepTag
		if inList(lib, deps.ReexportSharedLibHeaders) {
			depTag = sharedExportDepTag
		}
		actx.AddVariationDependencies([]blueprint.Variation{{"link", "shared"}}, depTag, lib)
	}

	actx.AddVariationDependencies([]blueprint.Variation{{"link", "shared"}}, lateSharedDepTag,
		deps.LateSharedLibs...)

	actx.AddDependency(c, genSourceDepTag, deps.GeneratedSources...)
	actx.AddDependency(c, genHeaderDepTag, deps.GeneratedHeaders...)

	actx.AddDependency(c, objDepTag, deps.ObjFiles...)

	if deps.CrtBegin != "" {
		actx.AddDependency(c, crtBeginDepTag, deps.CrtBegin)
	}
	if deps.CrtEnd != "" {
		actx.AddDependency(c, crtEndDepTag, deps.CrtEnd)
	}

	version := ctx.sdkVersion()
	actx.AddVariationDependencies([]blueprint.Variation{
		{"ndk_api", version}, {"link", "shared"}}, ndkStubDepTag, variantNdkLibs...)
	actx.AddVariationDependencies([]blueprint.Variation{
		{"ndk_api", version}, {"link", "shared"}}, ndkLateStubDepTag, variantLateNdkLibs...)
}

func depsMutator(ctx android.BottomUpMutatorContext) {
	if c, ok := ctx.Module().(*Module); ok && c.Enabled() {
		c.depsMutator(ctx)
	}
}

func (c *Module) clang(ctx BaseModuleContext) bool {
	clang := Bool(c.Properties.Clang)

	if c.Properties.Clang == nil {
		if ctx.Host() {
			clang = true
		}

		if ctx.Device() && ctx.AConfig().DeviceUsesClang() {
			clang = true
		}
	}

	if !c.toolchain(ctx).ClangSupported() {
		clang = false
	}

	return clang
}

// Convert dependencies to paths.  Returns a PathDeps containing paths
func (c *Module) depsToPaths(ctx android.ModuleContext) PathDeps {
	var depPaths PathDeps

	// Whether a module can link to another module, taking into
	// account NDK linking.
	linkTypeOk := func(from, to *Module) bool {
		if from.Target().Os != android.Android {
			// Host code is not restricted
			return true
		}
		if from.Properties.Sdk_version == "" {
			// Platform code can link to anything
			return true
		}
		if _, ok := to.linker.(*toolchainLibraryLinker); ok {
			// These are always allowed
			return true
		}
		if _, ok := to.linker.(*ndkPrebuiltLibraryLinker); ok {
			// These are allowed, but don't set sdk_version
			return true
		}
		if _, ok := to.linker.(*ndkPrebuiltStlLinker); ok {
			// These are allowed, but don't set sdk_version
			return true
		}
		if _, ok := to.linker.(*stubLinker); ok {
			// These aren't real libraries, but are the stub shared libraries that are included in
			// the NDK.
			return true
		}
		return to.Properties.Sdk_version != ""
	}

	ctx.VisitDirectDeps(func(m blueprint.Module) {
		name := ctx.OtherModuleName(m)
		tag := ctx.OtherModuleDependencyTag(m)

		a, _ := m.(android.Module)
		if a == nil {
			ctx.ModuleErrorf("module %q not an android module", name)
			return
		}

		cc, _ := m.(*Module)
		if cc == nil {
			switch tag {
			case android.DefaultsDepTag:
			case genSourceDepTag:
				if genRule, ok := m.(genrule.SourceFileGenerator); ok {
					depPaths.GeneratedSources = append(depPaths.GeneratedSources,
						genRule.GeneratedSourceFiles()...)
				} else {
					ctx.ModuleErrorf("module %q is not a gensrcs or genrule", name)
				}
			case genHeaderDepTag:
				if genRule, ok := m.(genrule.SourceFileGenerator); ok {
					depPaths.GeneratedHeaders = append(depPaths.GeneratedHeaders,
						genRule.GeneratedSourceFiles()...)
					depPaths.Flags = append(depPaths.Flags,
						includeDirsToFlags(android.Paths{genRule.GeneratedHeaderDir()}))
				} else {
					ctx.ModuleErrorf("module %q is not a genrule", name)
				}
			default:
				ctx.ModuleErrorf("depends on non-cc module %q", name)
			}
			return
		}

		if !a.Enabled() {
			ctx.ModuleErrorf("depends on disabled module %q", name)
			return
		}

		if a.Target().Os != ctx.Os() {
			ctx.ModuleErrorf("OS mismatch between %q and %q", ctx.ModuleName(), name)
			return
		}

		if a.Target().Arch.ArchType != ctx.Arch().ArchType {
			ctx.ModuleErrorf("Arch mismatch between %q and %q", ctx.ModuleName(), name)
			return
		}

		if !cc.outputFile.Valid() {
			ctx.ModuleErrorf("module %q missing output file", name)
			return
		}

		if tag == reuseObjTag {
			depPaths.ObjFiles = append(depPaths.ObjFiles,
				cc.compiler.(*libraryCompiler).reuseObjFiles...)
			return
		}

		if t, ok := tag.(dependencyTag); ok && t.library {
			if i, ok := cc.linker.(exportedFlagsProducer); ok {
				flags := i.exportedFlags()
				depPaths.Flags = append(depPaths.Flags, flags...)

				if t.reexportFlags {
					depPaths.ReexportedFlags = append(depPaths.ReexportedFlags, flags...)
				}
			}

			if !linkTypeOk(c, cc) {
				ctx.ModuleErrorf("depends on non-NDK-built library %q", name)
			}
		}

		var depPtr *android.Paths

		switch tag {
		case ndkStubDepTag, sharedDepTag, sharedExportDepTag:
			depPtr = &depPaths.SharedLibs
		case lateSharedDepTag, ndkLateStubDepTag:
			depPtr = &depPaths.LateSharedLibs
		case staticDepTag, staticExportDepTag:
			depPtr = &depPaths.StaticLibs
		case lateStaticDepTag:
			depPtr = &depPaths.LateStaticLibs
		case wholeStaticDepTag:
			depPtr = &depPaths.WholeStaticLibs
			staticLib, _ := cc.linker.(libraryInterface)
			if staticLib == nil || !staticLib.static() {
				ctx.ModuleErrorf("module %q not a static library", name)
				return
			}

			if missingDeps := staticLib.getWholeStaticMissingDeps(); missingDeps != nil {
				postfix := " (required by " + ctx.OtherModuleName(m) + ")"
				for i := range missingDeps {
					missingDeps[i] += postfix
				}
				ctx.AddMissingDependencies(missingDeps)
			}
			depPaths.WholeStaticLibObjFiles =
				append(depPaths.WholeStaticLibObjFiles, staticLib.objs()...)
		case objDepTag:
			depPtr = &depPaths.ObjFiles
		case crtBeginDepTag:
			depPaths.CrtBegin = cc.outputFile
		case crtEndDepTag:
			depPaths.CrtEnd = cc.outputFile
		default:
			panic(fmt.Errorf("unknown dependency tag: %s", tag))
		}

		if depPtr != nil {
			*depPtr = append(*depPtr, cc.outputFile.Path())
		}
	})

	return depPaths
}

func (c *Module) InstallInData() bool {
	if c.installer == nil {
		return false
	}
	return c.installer.inData()
}

//
// Defaults
//
type Defaults struct {
	android.ModuleBase
	android.DefaultsModule
}

func (*Defaults) GenerateAndroidBuildActions(ctx android.ModuleContext) {
}

func defaultsFactory() (blueprint.Module, []interface{}) {
	module := &Defaults{}

	propertyStructs := []interface{}{
		&BaseProperties{},
		&BaseCompilerProperties{},
		&BaseLinkerProperties{},
		&LibraryCompilerProperties{},
		&FlagExporterProperties{},
		&LibraryLinkerProperties{},
		&BinaryLinkerProperties{},
		&TestLinkerProperties{},
		&UnusedProperties{},
		&StlProperties{},
		&SanitizeProperties{},
		&StripProperties{},
	}

	_, propertyStructs = android.InitAndroidArchModule(module, android.HostAndDeviceDefault,
		android.MultilibDefault, propertyStructs...)

	return android.InitDefaultsModule(module, module, propertyStructs...)
}

//
// Device libraries shipped with gcc
//

type toolchainLibraryLinker struct {
	baseLinker
}

var _ baseLinkerInterface = (*toolchainLibraryLinker)(nil)

func (*toolchainLibraryLinker) deps(ctx BaseModuleContext, deps Deps) Deps {
	// toolchain libraries can't have any dependencies
	return deps
}

func (*toolchainLibraryLinker) buildStatic() bool {
	return true
}

func (*toolchainLibraryLinker) buildShared() bool {
	return false
}

func toolchainLibraryFactory() (blueprint.Module, []interface{}) {
	module := newBaseModule(android.DeviceSupported, android.MultilibBoth)
	module.compiler = &baseCompiler{}
	module.linker = &toolchainLibraryLinker{}
	module.Properties.Clang = proptools.BoolPtr(false)
	return module.Init()
}

func (library *toolchainLibraryLinker) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objFiles android.Paths) android.Path {

	libName := ctx.ModuleName() + staticLibraryExtension
	outputFile := android.PathForModuleOut(ctx, libName)

	if flags.Clang {
		ctx.ModuleErrorf("toolchain_library must use GCC, not Clang")
	}

	CopyGccLib(ctx, libName, flagsToBuilderFlags(flags), outputFile)

	ctx.CheckbuildFile(outputFile)

	return outputFile
}

func (*toolchainLibraryLinker) installable() bool {
	return false
}

// lastUniqueElements returns all unique elements of a slice, keeping the last copy of each
// modifies the slice contents in place, and returns a subslice of the original slice
func lastUniqueElements(list []string) []string {
	totalSkip := 0
	for i := len(list) - 1; i >= totalSkip; i-- {
		skip := 0
		for j := i - 1; j >= totalSkip; j-- {
			if list[i] == list[j] {
				skip++
			} else {
				list[j+skip] = list[j]
			}
		}
		totalSkip += skip
	}
	return list[totalSkip:]
}

var Bool = proptools.Bool
