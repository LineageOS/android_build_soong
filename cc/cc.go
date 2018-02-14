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
	"strconv"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc/config"
	"android/soong/genrule"
)

func init() {
	android.RegisterModuleType("cc_defaults", defaultsFactory)

	android.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("image", vendorMutator).Parallel()
		ctx.BottomUp("link", linkageMutator).Parallel()
		ctx.BottomUp("vndk", vndkMutator).Parallel()
		ctx.BottomUp("ndk_api", ndkApiMutator).Parallel()
		ctx.BottomUp("test_per_src", testPerSrcMutator).Parallel()
		ctx.BottomUp("begin", beginMutator).Parallel()
	})

	android.PostDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("asan_deps", sanitizerDepsMutator(asan))
		ctx.BottomUp("asan", sanitizerMutator(asan)).Parallel()

		ctx.TopDown("cfi_deps", sanitizerDepsMutator(cfi))
		ctx.BottomUp("cfi", sanitizerMutator(cfi)).Parallel()

		ctx.TopDown("tsan_deps", sanitizerDepsMutator(tsan))
		ctx.BottomUp("tsan", sanitizerMutator(tsan)).Parallel()

		ctx.BottomUp("coverage", coverageLinkingMutator).Parallel()
		ctx.TopDown("vndk_deps", sabiDepsMutator)

		ctx.TopDown("lto_deps", ltoDepsMutator)
		ctx.BottomUp("lto", ltoMutator).Parallel()
	})

	pctx.Import("android/soong/cc/config")
}

type Deps struct {
	SharedLibs, LateSharedLibs                  []string
	StaticLibs, LateStaticLibs, WholeStaticLibs []string
	HeaderLibs                                  []string

	ReexportSharedLibHeaders, ReexportStaticLibHeaders, ReexportHeaderLibHeaders []string

	ObjFiles []string

	GeneratedSources []string
	GeneratedHeaders []string

	ReexportGeneratedHeaders []string

	CrtBegin, CrtEnd string
	LinkerScript     string
}

type PathDeps struct {
	// Paths to .so files
	SharedLibs, LateSharedLibs android.Paths
	// Paths to the dependencies to use for .so files (.so.toc files)
	SharedLibsDeps, LateSharedLibsDeps android.Paths
	// Paths to .a files
	StaticLibs, LateStaticLibs, WholeStaticLibs android.Paths

	// Paths to .o files
	Objs               Objects
	StaticLibObjs      Objects
	WholeStaticLibObjs Objects

	// Paths to generated source files
	GeneratedSources android.Paths
	GeneratedHeaders android.Paths

	Flags, ReexportedFlags []string
	ReexportedFlagsDeps    android.Paths

	// Paths to crt*.o files
	CrtBegin, CrtEnd android.OptionalPath
	LinkerScript     android.OptionalPath
}

type Flags struct {
	GlobalFlags     []string // Flags that apply to C, C++, and assembly source files
	ArFlags         []string // Flags that apply to ar
	AsFlags         []string // Flags that apply to assembly source files
	CFlags          []string // Flags that apply to C and C++ source files
	ToolingCFlags   []string // Flags that apply to C and C++ source files parsed by clang LibTooling tools
	ConlyFlags      []string // Flags that apply to C source files
	CppFlags        []string // Flags that apply to C++ source files
	ToolingCppFlags []string // Flags that apply to C++ source files parsed by clang LibTooling tools
	YaccFlags       []string // Flags that apply to Yacc source files
	protoFlags      []string // Flags that apply to proto source files
	protoOutParams  []string // Flags that modify the output of proto generated files
	aidlFlags       []string // Flags that apply to aidl source files
	rsFlags         []string // Flags that apply to renderscript source files
	LdFlags         []string // Flags that apply to linker command lines
	libFlags        []string // Flags to add libraries early to the link order
	TidyFlags       []string // Flags that apply to clang-tidy
	SAbiFlags       []string // Flags that apply to header-abi-dumper
	YasmFlags       []string // Flags that apply to yasm assembly source files

	// Global include flags that apply to C, C++, and assembly source files
	// These must be after any module include flags, which will be in GlobalFlags.
	SystemIncludeFlags []string

	Toolchain config.Toolchain
	Clang     bool
	Tidy      bool
	Coverage  bool
	SAbiDump  bool

	RequiredInstructionSet string
	DynamicLinker          string

	CFlagsDeps  android.Paths // Files depended on by compiler flags
	LdFlagsDeps android.Paths // Files depended on by linker flags

	GroupStaticLibs bool
	ArGoldPlugin    bool // Whether LLVM gold plugin option is passed to llvm-ar
}

type ObjectLinkerProperties struct {
	// names of other cc_object modules to link into this module using partial linking
	Objs []string `android:"arch_variant"`

	// if set, add an extra objcopy --prefix-symbols= step
	Prefix_symbols *string
}

// Properties used to compile all C or C++ modules
type BaseProperties struct {
	// compile module with clang instead of gcc
	Clang *bool `android:"arch_variant"`

	// Minimum sdk version supported when compiling against the ndk
	Sdk_version *string

	AndroidMkSharedLibs []string `blueprint:"mutated"`
	HideFromMake        bool     `blueprint:"mutated"`
	PreventInstall      bool     `blueprint:"mutated"`

	UseVndk bool `blueprint:"mutated"`

	// *.logtags files, to combine together in order to generate the /system/etc/event-log-tags
	// file
	Logtags []string
}

type VendorProperties struct {
	// whether this module should be allowed to be directly depended by other
	// modules with `vendor: true`, `proprietary: true`, or `vendor_available:true`.
	// If set to true, two variants will be built separately, one like
	// normal, and the other limited to the set of libraries and headers
	// that are exposed to /vendor modules.
	//
	// The vendor variant may be used with a different (newer) /system,
	// so it shouldn't have any unversioned runtime dependencies, or
	// make assumptions about the system that may not be true in the
	// future.
	//
	// If set to false, this module becomes inaccessible from /vendor modules.
	//
	// Default value is true when vndk: {enabled: true} or vendor: true.
	//
	// Nothing happens if BOARD_VNDK_VERSION isn't set in the BoardConfig.mk
	Vendor_available *bool
}

type UnusedProperties struct {
	Tags []string
}

type ModuleContextIntf interface {
	static() bool
	staticBinary() bool
	clang() bool
	toolchain() config.Toolchain
	useSdk() bool
	sdkVersion() string
	useVndk() bool
	isVndk() bool
	isVndkSp() bool
	isVndkExt() bool
	createVndkSourceAbiDump() bool
	selectedStl() string
	baseModuleName() string
	getVndkExtendsModuleName() string
}

type ModuleContext interface {
	android.ModuleContext
	ModuleContextIntf
}

type BaseModuleContext interface {
	android.BaseContext
	ModuleContextIntf
}

type DepsContext interface {
	android.BottomUpMutatorContext
	ModuleContextIntf
}

type feature interface {
	begin(ctx BaseModuleContext)
	deps(ctx DepsContext, deps Deps) Deps
	flags(ctx ModuleContext, flags Flags) Flags
	props() []interface{}
}

type compiler interface {
	compilerInit(ctx BaseModuleContext)
	compilerDeps(ctx DepsContext, deps Deps) Deps
	compilerFlags(ctx ModuleContext, flags Flags, deps PathDeps) Flags
	compilerProps() []interface{}

	appendCflags([]string)
	appendAsflags([]string)
	compile(ctx ModuleContext, flags Flags, deps PathDeps) Objects
}

type linker interface {
	linkerInit(ctx BaseModuleContext)
	linkerDeps(ctx DepsContext, deps Deps) Deps
	linkerFlags(ctx ModuleContext, flags Flags) Flags
	linkerProps() []interface{}

	link(ctx ModuleContext, flags Flags, deps PathDeps, objs Objects) android.Path
	appendLdflags([]string)
}

type installer interface {
	installerProps() []interface{}
	install(ctx ModuleContext, path android.Path)
	inData() bool
	inSanitizerDir() bool
	hostToolPath() android.OptionalPath
}

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name    string
	library bool

	reexportFlags bool
}

var (
	sharedDepTag          = dependencyTag{name: "shared", library: true}
	sharedExportDepTag    = dependencyTag{name: "shared", library: true, reexportFlags: true}
	lateSharedDepTag      = dependencyTag{name: "late shared", library: true}
	staticDepTag          = dependencyTag{name: "static", library: true}
	staticExportDepTag    = dependencyTag{name: "static", library: true, reexportFlags: true}
	lateStaticDepTag      = dependencyTag{name: "late static", library: true}
	wholeStaticDepTag     = dependencyTag{name: "whole static", library: true, reexportFlags: true}
	headerDepTag          = dependencyTag{name: "header", library: true}
	headerExportDepTag    = dependencyTag{name: "header", library: true, reexportFlags: true}
	genSourceDepTag       = dependencyTag{name: "gen source"}
	genHeaderDepTag       = dependencyTag{name: "gen header"}
	genHeaderExportDepTag = dependencyTag{name: "gen header", reexportFlags: true}
	objDepTag             = dependencyTag{name: "obj"}
	crtBeginDepTag        = dependencyTag{name: "crtbegin"}
	crtEndDepTag          = dependencyTag{name: "crtend"}
	linkerScriptDepTag    = dependencyTag{name: "linker script"}
	reuseObjTag           = dependencyTag{name: "reuse objects"}
	ndkStubDepTag         = dependencyTag{name: "ndk stub", library: true}
	ndkLateStubDepTag     = dependencyTag{name: "ndk late stub", library: true}
	vndkExtDepTag         = dependencyTag{name: "vndk extends", library: true}
)

// Module contains the properties and members used by all C/C++ module types, and implements
// the blueprint.Module interface.  It delegates to compiler, linker, and installer interfaces
// to construct the output file.  Behavior can be customized with a Customizer interface
type Module struct {
	android.ModuleBase
	android.DefaultableModuleBase

	Properties       BaseProperties
	VendorProperties VendorProperties
	unused           UnusedProperties

	// initialize before calling Init
	hod      android.HostOrDeviceSupported
	multilib android.Multilib

	// delegates, initialize before calling Init
	features  []feature
	compiler  compiler
	linker    linker
	installer installer
	stl       *stl
	sanitize  *sanitize
	coverage  *coverage
	sabi      *sabi
	vndkdep   *vndkdep
	lto       *lto
	pgo       *pgo

	androidMkSharedLibDeps []string

	outputFile android.OptionalPath

	cachedToolchain config.Toolchain

	subAndroidMkOnce map[subAndroidMkProvider]bool

	// Flags used to compile this module
	flags Flags

	// When calling a linker, if module A depends on module B, then A must precede B in its command
	// line invocation. depsInLinkOrder stores the proper ordering of all of the transitive
	// deps of this module
	depsInLinkOrder android.Paths

	// only non-nil when this is a shared library that reuses the objects of a static library
	staticVariant *Module
}

func (c *Module) Init() android.Module {
	c.AddProperties(&c.Properties, &c.VendorProperties, &c.unused)
	if c.compiler != nil {
		c.AddProperties(c.compiler.compilerProps()...)
	}
	if c.linker != nil {
		c.AddProperties(c.linker.linkerProps()...)
	}
	if c.installer != nil {
		c.AddProperties(c.installer.installerProps()...)
	}
	if c.stl != nil {
		c.AddProperties(c.stl.props()...)
	}
	if c.sanitize != nil {
		c.AddProperties(c.sanitize.props()...)
	}
	if c.coverage != nil {
		c.AddProperties(c.coverage.props()...)
	}
	if c.sabi != nil {
		c.AddProperties(c.sabi.props()...)
	}
	if c.vndkdep != nil {
		c.AddProperties(c.vndkdep.props()...)
	}
	if c.lto != nil {
		c.AddProperties(c.lto.props()...)
	}
	if c.pgo != nil {
		c.AddProperties(c.pgo.props()...)
	}
	for _, feature := range c.features {
		c.AddProperties(feature.props()...)
	}

	android.InitAndroidArchModule(c, c.hod, c.multilib)

	android.InitDefaultableModule(c)

	return c
}

// Returns true for dependency roots (binaries)
// TODO(ccross): also handle dlopenable libraries
func (c *Module) isDependencyRoot() bool {
	if root, ok := c.linker.(interface {
		isDependencyRoot() bool
	}); ok {
		return root.isDependencyRoot()
	}
	return false
}

func (c *Module) useVndk() bool {
	return c.Properties.UseVndk
}

func (c *Module) isVndk() bool {
	if vndkdep := c.vndkdep; vndkdep != nil {
		return vndkdep.isVndk()
	}
	return false
}

func (c *Module) isVndkSp() bool {
	if vndkdep := c.vndkdep; vndkdep != nil {
		return vndkdep.isVndkSp()
	}
	return false
}

func (c *Module) isVndkExt() bool {
	if vndkdep := c.vndkdep; vndkdep != nil {
		return vndkdep.isVndkExt()
	}
	return false
}

func (c *Module) getVndkExtendsModuleName() string {
	if vndkdep := c.vndkdep; vndkdep != nil {
		return vndkdep.getVndkExtendsModuleName()
	}
	return ""
}

// Returns true only when this module is configured to have core and vendor
// variants.
func (c *Module) hasVendorVariant() bool {
	return c.isVndk() || Bool(c.VendorProperties.Vendor_available)
}

type baseModuleContext struct {
	android.BaseContext
	moduleContextImpl
}

type depsContext struct {
	android.BottomUpMutatorContext
	moduleContextImpl
}

type moduleContext struct {
	android.ModuleContext
	moduleContextImpl
}

func (ctx *moduleContext) SocSpecific() bool {
	return ctx.ModuleContext.SocSpecific() ||
		(ctx.mod.hasVendorVariant() && ctx.mod.useVndk() && !ctx.mod.isVndk())
}

type moduleContextImpl struct {
	mod *Module
	ctx BaseModuleContext
}

func (ctx *moduleContextImpl) clang() bool {
	return ctx.mod.clang(ctx.ctx)
}

func (ctx *moduleContextImpl) toolchain() config.Toolchain {
	return ctx.mod.toolchain(ctx.ctx)
}

func (ctx *moduleContextImpl) static() bool {
	return ctx.mod.static()
}

func (ctx *moduleContextImpl) staticBinary() bool {
	if static, ok := ctx.mod.linker.(interface {
		staticBinary() bool
	}); ok {
		return static.staticBinary()
	}
	return false
}

func (ctx *moduleContextImpl) useSdk() bool {
	if ctx.ctx.Device() && !ctx.useVndk() {
		return String(ctx.mod.Properties.Sdk_version) != ""
	}
	return false
}

func (ctx *moduleContextImpl) sdkVersion() string {
	if ctx.ctx.Device() {
		if ctx.useVndk() {
			return "current"
		} else {
			return String(ctx.mod.Properties.Sdk_version)
		}
	}
	return ""
}

func (ctx *moduleContextImpl) useVndk() bool {
	return ctx.mod.useVndk()
}

func (ctx *moduleContextImpl) isVndk() bool {
	return ctx.mod.isVndk()
}

func (ctx *moduleContextImpl) isVndkSp() bool {
	return ctx.mod.isVndkSp()
}

func (ctx *moduleContextImpl) isVndkExt() bool {
	return ctx.mod.isVndkExt()
}

// Create source abi dumps if the module belongs to the list of VndkLibraries.
func (ctx *moduleContextImpl) createVndkSourceAbiDump() bool {
	return ctx.ctx.Device() && ((ctx.useVndk() && ctx.isVndk()) || inList(ctx.baseModuleName(), llndkLibraries))
}

func (ctx *moduleContextImpl) selectedStl() string {
	if stl := ctx.mod.stl; stl != nil {
		return stl.Properties.SelectedStl
	}
	return ""
}

func (ctx *moduleContextImpl) baseModuleName() string {
	return ctx.mod.ModuleBase.BaseModuleName()
}

func (ctx *moduleContextImpl) getVndkExtendsModuleName() string {
	return ctx.mod.getVndkExtendsModuleName()
}

func newBaseModule(hod android.HostOrDeviceSupported, multilib android.Multilib) *Module {
	return &Module{
		hod:      hod,
		multilib: multilib,
	}
}

func newModule(hod android.HostOrDeviceSupported, multilib android.Multilib) *Module {
	module := newBaseModule(hod, multilib)
	module.features = []feature{
		&tidyFeature{},
	}
	module.stl = &stl{}
	module.sanitize = &sanitize{}
	module.coverage = &coverage{}
	module.sabi = &sabi{}
	module.vndkdep = &vndkdep{}
	module.lto = &lto{}
	module.pgo = &pgo{}
	return module
}

func (c *Module) Prebuilt() *android.Prebuilt {
	if p, ok := c.linker.(prebuiltLinkerInterface); ok {
		return p.prebuilt()
	}
	return nil
}

func (c *Module) Name() string {
	name := c.ModuleBase.Name()
	if p, ok := c.linker.(interface {
		Name(string) string
	}); ok {
		name = p.Name(name)
	}
	return name
}

// orderDeps reorders dependencies into a list such that if module A depends on B, then
// A will precede B in the resultant list.
// This is convenient for passing into a linker.
// Note that directSharedDeps should be the analogous static library for each shared lib dep
func orderDeps(directStaticDeps []android.Path, directSharedDeps []android.Path, allTransitiveDeps map[android.Path][]android.Path) (orderedAllDeps []android.Path, orderedDeclaredDeps []android.Path) {
	// If A depends on B, then
	//   Every list containing A will also contain B later in the list
	//   So, after concatenating all lists, the final instance of B will have come from the same
	//     original list as the final instance of A
	//   So, the final instance of B will be later in the concatenation than the final A
	//   So, keeping only the final instance of A and of B ensures that A is earlier in the output
	//     list than B
	for _, dep := range directStaticDeps {
		orderedAllDeps = append(orderedAllDeps, dep)
		orderedAllDeps = append(orderedAllDeps, allTransitiveDeps[dep]...)
	}
	for _, dep := range directSharedDeps {
		orderedAllDeps = append(orderedAllDeps, dep)
		orderedAllDeps = append(orderedAllDeps, allTransitiveDeps[dep]...)
	}

	orderedAllDeps = android.LastUniquePaths(orderedAllDeps)

	// We don't want to add any new dependencies into directStaticDeps (to allow the caller to
	// intentionally exclude or replace any unwanted transitive dependencies), so we limit the
	// resultant list to only what the caller has chosen to include in directStaticDeps
	_, orderedDeclaredDeps = android.FilterPathList(orderedAllDeps, directStaticDeps)

	return orderedAllDeps, orderedDeclaredDeps
}

func orderStaticModuleDeps(module *Module, staticDeps []*Module, sharedDeps []*Module) (results []android.Path) {
	// convert Module to Path
	allTransitiveDeps := make(map[android.Path][]android.Path, len(staticDeps))
	staticDepFiles := []android.Path{}
	for _, dep := range staticDeps {
		allTransitiveDeps[dep.outputFile.Path()] = dep.depsInLinkOrder
		staticDepFiles = append(staticDepFiles, dep.outputFile.Path())
	}
	sharedDepFiles := []android.Path{}
	for _, sharedDep := range sharedDeps {
		staticAnalogue := sharedDep.staticVariant
		if staticAnalogue != nil {
			allTransitiveDeps[staticAnalogue.outputFile.Path()] = staticAnalogue.depsInLinkOrder
			sharedDepFiles = append(sharedDepFiles, staticAnalogue.outputFile.Path())
		}
	}

	// reorder the dependencies based on transitive dependencies
	module.depsInLinkOrder, results = orderDeps(staticDepFiles, sharedDepFiles, allTransitiveDeps)

	return results
}

func (c *Module) GenerateAndroidBuildActions(actx android.ModuleContext) {

	ctx := &moduleContext{
		ModuleContext: actx,
		moduleContextImpl: moduleContextImpl{
			mod: c,
		},
	}
	ctx.ctx = ctx

	deps := c.depsToPaths(ctx)
	if ctx.Failed() {
		return
	}

	flags := Flags{
		Toolchain: c.toolchain(ctx),
		Clang:     c.clang(ctx),
	}
	if c.compiler != nil {
		flags = c.compiler.compilerFlags(ctx, flags, deps)
	}
	if c.linker != nil {
		flags = c.linker.linkerFlags(ctx, flags)
	}
	if c.stl != nil {
		flags = c.stl.flags(ctx, flags)
	}
	if c.sanitize != nil {
		flags = c.sanitize.flags(ctx, flags)
	}
	if c.coverage != nil {
		flags = c.coverage.flags(ctx, flags)
	}
	if c.lto != nil {
		flags = c.lto.flags(ctx, flags)
	}
	if c.pgo != nil {
		flags = c.pgo.flags(ctx, flags)
	}
	for _, feature := range c.features {
		flags = feature.flags(ctx, flags)
	}
	if ctx.Failed() {
		return
	}

	flags.CFlags, _ = filterList(flags.CFlags, config.IllegalFlags)
	flags.CppFlags, _ = filterList(flags.CppFlags, config.IllegalFlags)
	flags.ConlyFlags, _ = filterList(flags.ConlyFlags, config.IllegalFlags)

	flags.GlobalFlags = append(flags.GlobalFlags, deps.Flags...)
	c.flags = flags
	// We need access to all the flags seen by a source file.
	if c.sabi != nil {
		flags = c.sabi.flags(ctx, flags)
	}
	// Optimization to reduce size of build.ninja
	// Replace the long list of flags for each file with a module-local variable
	ctx.Variable(pctx, "cflags", strings.Join(flags.CFlags, " "))
	ctx.Variable(pctx, "cppflags", strings.Join(flags.CppFlags, " "))
	ctx.Variable(pctx, "asflags", strings.Join(flags.AsFlags, " "))
	flags.CFlags = []string{"$cflags"}
	flags.CppFlags = []string{"$cppflags"}
	flags.AsFlags = []string{"$asflags"}

	var objs Objects
	if c.compiler != nil {
		objs = c.compiler.compile(ctx, flags, deps)
		if ctx.Failed() {
			return
		}
	}

	if c.linker != nil {
		outputFile := c.linker.link(ctx, flags, deps, objs)
		if ctx.Failed() {
			return
		}
		c.outputFile = android.OptionalPathForPath(outputFile)
	}

	if c.installer != nil && !c.Properties.PreventInstall && c.outputFile.Valid() {
		c.installer.install(ctx, c.outputFile.Path())
		if ctx.Failed() {
			return
		}
	}
}

func (c *Module) toolchain(ctx BaseModuleContext) config.Toolchain {
	if c.cachedToolchain == nil {
		c.cachedToolchain = config.FindToolchain(ctx.Os(), ctx.Arch())
	}
	return c.cachedToolchain
}

func (c *Module) begin(ctx BaseModuleContext) {
	if c.compiler != nil {
		c.compiler.compilerInit(ctx)
	}
	if c.linker != nil {
		c.linker.linkerInit(ctx)
	}
	if c.stl != nil {
		c.stl.begin(ctx)
	}
	if c.sanitize != nil {
		c.sanitize.begin(ctx)
	}
	if c.coverage != nil {
		c.coverage.begin(ctx)
	}
	if c.sabi != nil {
		c.sabi.begin(ctx)
	}
	if c.vndkdep != nil {
		c.vndkdep.begin(ctx)
	}
	if c.lto != nil {
		c.lto.begin(ctx)
	}
	if c.pgo != nil {
		c.pgo.begin(ctx)
	}
	for _, feature := range c.features {
		feature.begin(ctx)
	}
	if ctx.useSdk() {
		version, err := normalizeNdkApiLevel(ctx, ctx.sdkVersion(), ctx.Arch())
		if err != nil {
			ctx.PropertyErrorf("sdk_version", err.Error())
		}
		c.Properties.Sdk_version = StringPtr(version)
	}
}

func (c *Module) deps(ctx DepsContext) Deps {
	deps := Deps{}

	if c.compiler != nil {
		deps = c.compiler.compilerDeps(ctx, deps)
	}
	// Add the PGO dependency (the clang_rt.profile runtime library), which
	// sometimes depends on symbols from libgcc, before libgcc gets added
	// in linkerDeps().
	if c.pgo != nil {
		deps = c.pgo.deps(ctx, deps)
	}
	if c.linker != nil {
		deps = c.linker.linkerDeps(ctx, deps)
	}
	if c.stl != nil {
		deps = c.stl.deps(ctx, deps)
	}
	if c.sanitize != nil {
		deps = c.sanitize.deps(ctx, deps)
	}
	if c.coverage != nil {
		deps = c.coverage.deps(ctx, deps)
	}
	if c.sabi != nil {
		deps = c.sabi.deps(ctx, deps)
	}
	if c.vndkdep != nil {
		deps = c.vndkdep.deps(ctx, deps)
	}
	if c.lto != nil {
		deps = c.lto.deps(ctx, deps)
	}
	for _, feature := range c.features {
		deps = feature.deps(ctx, deps)
	}

	deps.WholeStaticLibs = android.LastUniqueStrings(deps.WholeStaticLibs)
	deps.StaticLibs = android.LastUniqueStrings(deps.StaticLibs)
	deps.LateStaticLibs = android.LastUniqueStrings(deps.LateStaticLibs)
	deps.SharedLibs = android.LastUniqueStrings(deps.SharedLibs)
	deps.LateSharedLibs = android.LastUniqueStrings(deps.LateSharedLibs)
	deps.HeaderLibs = android.LastUniqueStrings(deps.HeaderLibs)

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

	for _, lib := range deps.ReexportHeaderLibHeaders {
		if !inList(lib, deps.HeaderLibs) {
			ctx.PropertyErrorf("export_header_lib_headers", "Header library not in header_libs: '%s'", lib)
		}
	}

	for _, gen := range deps.ReexportGeneratedHeaders {
		if !inList(gen, deps.GeneratedHeaders) {
			ctx.PropertyErrorf("export_generated_headers", "Generated header module not in generated_headers: '%s'", gen)
		}
	}

	return deps
}

func (c *Module) beginMutator(actx android.BottomUpMutatorContext) {
	ctx := &baseModuleContext{
		BaseContext: actx,
		moduleContextImpl: moduleContextImpl{
			mod: c,
		},
	}
	ctx.ctx = ctx

	c.begin(ctx)
}

func (c *Module) DepsMutator(actx android.BottomUpMutatorContext) {
	if !c.Enabled() {
		return
	}

	ctx := &depsContext{
		BottomUpMutatorContext: actx,
		moduleContextImpl: moduleContextImpl{
			mod: c,
		},
	}
	ctx.ctx = ctx

	deps := c.deps(ctx)

	variantNdkLibs := []string{}
	variantLateNdkLibs := []string{}
	if ctx.Os() == android.Android {
		version := ctx.sdkVersion()

		// rewriteNdkLibs takes a list of names of shared libraries and scans it for three types
		// of names:
		//
		// 1. Name of an NDK library that refers to a prebuilt module.
		//    For each of these, it adds the name of the prebuilt module (which will be in
		//    prebuilts/ndk) to the list of nonvariant libs.
		// 2. Name of an NDK library that refers to an ndk_library module.
		//    For each of these, it adds the name of the ndk_library module to the list of
		//    variant libs.
		// 3. Anything else (so anything that isn't an NDK library).
		//    It adds these to the nonvariantLibs list.
		//
		// The caller can then know to add the variantLibs dependencies differently from the
		// nonvariantLibs
		rewriteNdkLibs := func(list []string) (nonvariantLibs []string, variantLibs []string) {
			variantLibs = []string{}
			nonvariantLibs = []string{}
			for _, entry := range list {
				if ctx.useSdk() && inList(entry, ndkPrebuiltSharedLibraries) {
					if !inList(entry, ndkMigratedLibs) {
						nonvariantLibs = append(nonvariantLibs, entry+".ndk."+version)
					} else {
						variantLibs = append(variantLibs, entry+ndkLibrarySuffix)
					}
				} else if ctx.useVndk() && inList(entry, llndkLibraries) {
					nonvariantLibs = append(nonvariantLibs, entry+llndkLibrarySuffix)
				} else {
					nonvariantLibs = append(nonvariantLibs, entry)
				}
			}
			return nonvariantLibs, variantLibs
		}

		deps.SharedLibs, variantNdkLibs = rewriteNdkLibs(deps.SharedLibs)
		deps.LateSharedLibs, variantLateNdkLibs = rewriteNdkLibs(deps.LateSharedLibs)
		deps.ReexportSharedLibHeaders, _ = rewriteNdkLibs(deps.ReexportSharedLibHeaders)
	}

	for _, lib := range deps.HeaderLibs {
		depTag := headerDepTag
		if inList(lib, deps.ReexportHeaderLibHeaders) {
			depTag = headerExportDepTag
		}
		actx.AddVariationDependencies(nil, depTag, lib)
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

	for _, gen := range deps.GeneratedHeaders {
		depTag := genHeaderDepTag
		if inList(gen, deps.ReexportGeneratedHeaders) {
			depTag = genHeaderExportDepTag
		}
		actx.AddDependency(c, depTag, gen)
	}

	actx.AddDependency(c, objDepTag, deps.ObjFiles...)

	if deps.CrtBegin != "" {
		actx.AddDependency(c, crtBeginDepTag, deps.CrtBegin)
	}
	if deps.CrtEnd != "" {
		actx.AddDependency(c, crtEndDepTag, deps.CrtEnd)
	}
	if deps.LinkerScript != "" {
		actx.AddDependency(c, linkerScriptDepTag, deps.LinkerScript)
	}

	version := ctx.sdkVersion()
	actx.AddVariationDependencies([]blueprint.Variation{
		{"ndk_api", version}, {"link", "shared"}}, ndkStubDepTag, variantNdkLibs...)
	actx.AddVariationDependencies([]blueprint.Variation{
		{"ndk_api", version}, {"link", "shared"}}, ndkLateStubDepTag, variantLateNdkLibs...)

	if vndkdep := c.vndkdep; vndkdep != nil {
		if vndkdep.isVndkExt() {
			baseModuleMode := vendorMode
			if actx.DeviceConfig().VndkVersion() == "" {
				baseModuleMode = coreMode
			}
			actx.AddVariationDependencies([]blueprint.Variation{
				{"image", baseModuleMode}, {"link", "shared"}}, vndkExtDepTag,
				vndkdep.getVndkExtendsModuleName())
		}
	}
}

func beginMutator(ctx android.BottomUpMutatorContext) {
	if c, ok := ctx.Module().(*Module); ok && c.Enabled() {
		c.beginMutator(ctx)
	}
}

func (c *Module) clang(ctx BaseModuleContext) bool {
	clang := Bool(c.Properties.Clang)

	if c.Properties.Clang == nil {
		clang = true
	}

	if !c.toolchain(ctx).ClangSupported() {
		clang = false
	}

	return clang
}

// Whether a module can link to another module, taking into
// account NDK linking.
func checkLinkType(ctx android.ModuleContext, from *Module, to *Module, tag dependencyTag) {
	if from.Target().Os != android.Android {
		// Host code is not restricted
		return
	}
	if from.Properties.UseVndk {
		// Though vendor code is limited by the vendor mutator,
		// each vendor-available module needs to check
		// link-type for VNDK.
		if from.vndkdep != nil {
			from.vndkdep.vndkCheckLinkType(ctx, to, tag)
		}
		return
	}
	if String(from.Properties.Sdk_version) == "" {
		// Platform code can link to anything
		return
	}
	if _, ok := to.linker.(*toolchainLibraryDecorator); ok {
		// These are always allowed
		return
	}
	if _, ok := to.linker.(*ndkPrebuiltLibraryLinker); ok {
		// These are allowed, but they don't set sdk_version
		return
	}
	if _, ok := to.linker.(*ndkPrebuiltStlLinker); ok {
		// These are allowed, but they don't set sdk_version
		return
	}
	if _, ok := to.linker.(*stubDecorator); ok {
		// These aren't real libraries, but are the stub shared libraries that are included in
		// the NDK.
		return
	}
	if String(to.Properties.Sdk_version) == "" {
		// NDK code linking to platform code is never okay.
		ctx.ModuleErrorf("depends on non-NDK-built library %q",
			ctx.OtherModuleName(to))
	}

	// At this point we know we have two NDK libraries, but we need to
	// check that we're not linking against anything built against a higher
	// API level, as it is only valid to link against older or equivalent
	// APIs.

	if String(from.Properties.Sdk_version) == "current" {
		// Current can link against anything.
		return
	} else if String(to.Properties.Sdk_version) == "current" {
		// Current can't be linked against by anything else.
		ctx.ModuleErrorf("links %q built against newer API version %q",
			ctx.OtherModuleName(to), "current")
	}

	fromApi, err := strconv.Atoi(String(from.Properties.Sdk_version))
	if err != nil {
		ctx.PropertyErrorf("sdk_version",
			"Invalid sdk_version value (must be int): %q",
			String(from.Properties.Sdk_version))
	}
	toApi, err := strconv.Atoi(String(to.Properties.Sdk_version))
	if err != nil {
		ctx.PropertyErrorf("sdk_version",
			"Invalid sdk_version value (must be int): %q",
			String(to.Properties.Sdk_version))
	}

	if toApi > fromApi {
		ctx.ModuleErrorf("links %q built against newer API version %q",
			ctx.OtherModuleName(to), String(to.Properties.Sdk_version))
	}

	// Also check that the two STL choices are compatible.
	fromStl := from.stl.Properties.SelectedStl
	toStl := to.stl.Properties.SelectedStl
	if fromStl == "" || toStl == "" {
		// Libraries that don't use the STL are unrestricted.
		return
	}

	if fromStl == "ndk_system" || toStl == "ndk_system" {
		// We can be permissive with the system "STL" since it is only the C++
		// ABI layer, but in the future we should make sure that everyone is
		// using either libc++ or nothing.
		return
	}

	if getNdkStlFamily(ctx, from) != getNdkStlFamily(ctx, to) {
		ctx.ModuleErrorf("uses %q and depends on %q which uses incompatible %q",
			from.stl.Properties.SelectedStl, ctx.OtherModuleName(to),
			to.stl.Properties.SelectedStl)
	}
}

// Convert dependencies to paths.  Returns a PathDeps containing paths
func (c *Module) depsToPaths(ctx android.ModuleContext) PathDeps {
	var depPaths PathDeps

	directStaticDeps := []*Module{}
	directSharedDeps := []*Module{}

	ctx.VisitDirectDeps(func(dep android.Module) {
		depName := ctx.OtherModuleName(dep)
		depTag := ctx.OtherModuleDependencyTag(dep)

		ccDep, _ := dep.(*Module)
		if ccDep == nil {
			// handling for a few module types that aren't cc Module but that are also supported
			switch depTag {
			case android.DefaultsDepTag, android.SourceDepTag:
				// Nothing to do
			case genSourceDepTag:
				if genRule, ok := dep.(genrule.SourceFileGenerator); ok {
					depPaths.GeneratedSources = append(depPaths.GeneratedSources,
						genRule.GeneratedSourceFiles()...)
				} else {
					ctx.ModuleErrorf("module %q is not a gensrcs or genrule", depName)
				}
				// Support exported headers from a generated_sources dependency
				fallthrough
			case genHeaderDepTag, genHeaderExportDepTag:
				if genRule, ok := dep.(genrule.SourceFileGenerator); ok {
					depPaths.GeneratedHeaders = append(depPaths.GeneratedHeaders,
						genRule.GeneratedSourceFiles()...)
					flags := includeDirsToFlags(genRule.GeneratedHeaderDirs())
					depPaths.Flags = append(depPaths.Flags, flags)
					if depTag == genHeaderExportDepTag {
						depPaths.ReexportedFlags = append(depPaths.ReexportedFlags, flags)
						depPaths.ReexportedFlagsDeps = append(depPaths.ReexportedFlagsDeps,
							genRule.GeneratedSourceFiles()...)
						// Add these re-exported flags to help header-abi-dumper to infer the abi exported by a library.
						c.sabi.Properties.ReexportedIncludeFlags = append(c.sabi.Properties.ReexportedIncludeFlags, flags)

					}
				} else {
					ctx.ModuleErrorf("module %q is not a genrule", depName)
				}
			case linkerScriptDepTag:
				if genRule, ok := dep.(genrule.SourceFileGenerator); ok {
					files := genRule.GeneratedSourceFiles()
					if len(files) == 1 {
						depPaths.LinkerScript = android.OptionalPathForPath(files[0])
					} else if len(files) > 1 {
						ctx.ModuleErrorf("module %q can only generate a single file if used for a linker script", depName)
					}
				} else {
					ctx.ModuleErrorf("module %q is not a genrule", depName)
				}
			default:
				ctx.ModuleErrorf("depends on non-cc module %q", depName)
			}
			return
		}

		if dep.Target().Os != ctx.Os() {
			ctx.ModuleErrorf("OS mismatch between %q and %q", ctx.ModuleName(), depName)
			return
		}
		if dep.Target().Arch.ArchType != ctx.Arch().ArchType {
			ctx.ModuleErrorf("Arch mismatch between %q and %q", ctx.ModuleName(), depName)
			return
		}

		// re-exporting flags
		if depTag == reuseObjTag {
			if l, ok := ccDep.compiler.(libraryInterface); ok {
				c.staticVariant = ccDep
				objs, flags, deps := l.reuseObjs()
				depPaths.Objs = depPaths.Objs.Append(objs)
				depPaths.ReexportedFlags = append(depPaths.ReexportedFlags, flags...)
				depPaths.ReexportedFlagsDeps = append(depPaths.ReexportedFlagsDeps, deps...)
				return
			}
		}
		if t, ok := depTag.(dependencyTag); ok && t.library {
			if i, ok := ccDep.linker.(exportedFlagsProducer); ok {
				flags := i.exportedFlags()
				deps := i.exportedFlagsDeps()
				depPaths.Flags = append(depPaths.Flags, flags...)
				depPaths.GeneratedHeaders = append(depPaths.GeneratedHeaders, deps...)

				if t.reexportFlags {
					depPaths.ReexportedFlags = append(depPaths.ReexportedFlags, flags...)
					depPaths.ReexportedFlagsDeps = append(depPaths.ReexportedFlagsDeps, deps...)
					// Add these re-exported flags to help header-abi-dumper to infer the abi exported by a library.
					// Re-exported shared library headers must be included as well since they can help us with type information
					// about template instantiations (instantiated from their headers).
					c.sabi.Properties.ReexportedIncludeFlags = append(c.sabi.Properties.ReexportedIncludeFlags, flags...)
				}
			}

			checkLinkType(ctx, c, ccDep, t)
		}

		var ptr *android.Paths
		var depPtr *android.Paths

		linkFile := ccDep.outputFile
		depFile := android.OptionalPath{}

		switch depTag {
		case ndkStubDepTag, sharedDepTag, sharedExportDepTag:
			ptr = &depPaths.SharedLibs
			depPtr = &depPaths.SharedLibsDeps
			depFile = ccDep.linker.(libraryInterface).toc()
			directSharedDeps = append(directSharedDeps, ccDep)
		case lateSharedDepTag, ndkLateStubDepTag:
			ptr = &depPaths.LateSharedLibs
			depPtr = &depPaths.LateSharedLibsDeps
			depFile = ccDep.linker.(libraryInterface).toc()
		case staticDepTag, staticExportDepTag:
			ptr = nil
			directStaticDeps = append(directStaticDeps, ccDep)
		case lateStaticDepTag:
			ptr = &depPaths.LateStaticLibs
		case wholeStaticDepTag:
			ptr = &depPaths.WholeStaticLibs
			staticLib, ok := ccDep.linker.(libraryInterface)
			if !ok || !staticLib.static() {
				ctx.ModuleErrorf("module %q not a static library", depName)
				return
			}

			if missingDeps := staticLib.getWholeStaticMissingDeps(); missingDeps != nil {
				postfix := " (required by " + ctx.OtherModuleName(dep) + ")"
				for i := range missingDeps {
					missingDeps[i] += postfix
				}
				ctx.AddMissingDependencies(missingDeps)
			}
			depPaths.WholeStaticLibObjs = depPaths.WholeStaticLibObjs.Append(staticLib.objs())
		case headerDepTag:
			// Nothing
		case objDepTag:
			depPaths.Objs.objFiles = append(depPaths.Objs.objFiles, linkFile.Path())
		case crtBeginDepTag:
			depPaths.CrtBegin = linkFile
		case crtEndDepTag:
			depPaths.CrtEnd = linkFile
		}

		switch depTag {
		case staticDepTag, staticExportDepTag, lateStaticDepTag:
			staticLib, ok := ccDep.linker.(libraryInterface)
			if !ok || !staticLib.static() {
				ctx.ModuleErrorf("module %q not a static library", depName)
				return
			}

			// When combining coverage files for shared libraries and executables, coverage files
			// in static libraries act as if they were whole static libraries. The same goes for
			// source based Abi dump files.
			depPaths.StaticLibObjs.coverageFiles = append(depPaths.StaticLibObjs.coverageFiles,
				staticLib.objs().coverageFiles...)
			depPaths.StaticLibObjs.sAbiDumpFiles = append(depPaths.StaticLibObjs.sAbiDumpFiles,
				staticLib.objs().sAbiDumpFiles...)

		}

		if ptr != nil {
			if !linkFile.Valid() {
				ctx.ModuleErrorf("module %q missing output file", depName)
				return
			}
			*ptr = append(*ptr, linkFile.Path())
		}

		if depPtr != nil {
			dep := depFile
			if !dep.Valid() {
				dep = linkFile
			}
			*depPtr = append(*depPtr, dep.Path())
		}

		// Export the shared libs to Make.
		switch depTag {
		case sharedDepTag, sharedExportDepTag, lateSharedDepTag:
			libName := strings.TrimSuffix(depName, llndkLibrarySuffix)
			libName = strings.TrimPrefix(libName, "prebuilt_")
			isLLndk := inList(libName, llndkLibraries)
			var makeLibName string
			bothVendorAndCoreVariantsExist := ccDep.hasVendorVariant() || isLLndk
			if c.useVndk() && bothVendorAndCoreVariantsExist {
				// The vendor module in Make will have been renamed to not conflict with the core
				// module, so update the dependency name here accordingly.
				makeLibName = libName + vendorSuffix
			} else {
				makeLibName = libName
			}
			// Note: the order of libs in this list is not important because
			// they merely serve as Make dependencies and do not affect this lib itself.
			c.Properties.AndroidMkSharedLibs = append(c.Properties.AndroidMkSharedLibs, makeLibName)
		}
	})

	// use the ordered dependencies as this module's dependencies
	depPaths.StaticLibs = append(depPaths.StaticLibs, orderStaticModuleDeps(c, directStaticDeps, directSharedDeps)...)

	// Dedup exported flags from dependencies
	depPaths.Flags = android.FirstUniqueStrings(depPaths.Flags)
	depPaths.GeneratedHeaders = android.FirstUniquePaths(depPaths.GeneratedHeaders)
	depPaths.ReexportedFlags = android.FirstUniqueStrings(depPaths.ReexportedFlags)
	depPaths.ReexportedFlagsDeps = android.FirstUniquePaths(depPaths.ReexportedFlagsDeps)

	if c.sabi != nil {
		c.sabi.Properties.ReexportedIncludeFlags = android.FirstUniqueStrings(c.sabi.Properties.ReexportedIncludeFlags)
	}

	return depPaths
}

func (c *Module) InstallInData() bool {
	if c.installer == nil {
		return false
	}
	return c.installer.inData()
}

func (c *Module) InstallInSanitizerDir() bool {
	if c.installer == nil {
		return false
	}
	if c.sanitize != nil && c.sanitize.inSanitizerDir() {
		return true
	}
	return c.installer.inSanitizerDir()
}

func (c *Module) HostToolPath() android.OptionalPath {
	if c.installer == nil {
		return android.OptionalPath{}
	}
	return c.installer.hostToolPath()
}

func (c *Module) IntermPathForModuleOut() android.OptionalPath {
	return c.outputFile
}

func (c *Module) Srcs() android.Paths {
	if c.outputFile.Valid() {
		return android.Paths{c.outputFile.Path()}
	}
	return android.Paths{}
}

func (c *Module) static() bool {
	if static, ok := c.linker.(interface {
		static() bool
	}); ok {
		return static.static()
	}
	return false
}

//
// Defaults
//
type Defaults struct {
	android.ModuleBase
	android.DefaultsModuleBase
}

func (*Defaults) GenerateAndroidBuildActions(ctx android.ModuleContext) {
}

func (d *Defaults) DepsMutator(ctx android.BottomUpMutatorContext) {
}

func defaultsFactory() android.Module {
	return DefaultsFactory()
}

func DefaultsFactory(props ...interface{}) android.Module {
	module := &Defaults{}

	module.AddProperties(props...)
	module.AddProperties(
		&BaseProperties{},
		&VendorProperties{},
		&BaseCompilerProperties{},
		&BaseLinkerProperties{},
		&LibraryProperties{},
		&FlagExporterProperties{},
		&BinaryLinkerProperties{},
		&TestProperties{},
		&TestBinaryProperties{},
		&UnusedProperties{},
		&StlProperties{},
		&SanitizeProperties{},
		&StripProperties{},
		&InstallerProperties{},
		&TidyProperties{},
		&CoverageProperties{},
		&SAbiProperties{},
		&VndkProperties{},
		&LTOProperties{},
		&PgoProperties{},
	)

	android.InitDefaultsModule(module)

	return module
}

const (
	// coreMode is the variant used for framework-private libraries, or
	// SDK libraries. (which framework-private libraries can use)
	coreMode = "core"

	// vendorMode is the variant used for /vendor code that compiles
	// against the VNDK.
	vendorMode = "vendor"
)

func squashVendorSrcs(m *Module) {
	if lib, ok := m.compiler.(*libraryDecorator); ok {
		lib.baseCompiler.Properties.Srcs = append(lib.baseCompiler.Properties.Srcs,
			lib.baseCompiler.Properties.Target.Vendor.Srcs...)

		lib.baseCompiler.Properties.Exclude_srcs = append(lib.baseCompiler.Properties.Exclude_srcs,
			lib.baseCompiler.Properties.Target.Vendor.Exclude_srcs...)
	}
}

func vendorMutator(mctx android.BottomUpMutatorContext) {
	if mctx.Os() != android.Android {
		return
	}

	if genrule, ok := mctx.Module().(*genrule.Module); ok {
		if props, ok := genrule.Extra.(*VendorProperties); ok {
			if mctx.DeviceConfig().VndkVersion() == "" {
				mctx.CreateVariations(coreMode)
			} else if Bool(props.Vendor_available) {
				mctx.CreateVariations(coreMode, vendorMode)
			} else if mctx.SocSpecific() || mctx.DeviceSpecific() {
				mctx.CreateVariations(vendorMode)
			} else {
				mctx.CreateVariations(coreMode)
			}
		}
	}

	m, ok := mctx.Module().(*Module)
	if !ok {
		return
	}

	// Sanity check
	vendorSpecific := mctx.SocSpecific() || mctx.DeviceSpecific()

	if m.VendorProperties.Vendor_available != nil && vendorSpecific {
		mctx.PropertyErrorf("vendor_available",
			"doesn't make sense at the same time as `vendor: true`, `proprietary: true`, or `device_specific:true`")
		return
	}

	if vndkdep := m.vndkdep; vndkdep != nil {
		if vndkdep.isVndk() {
			if vendorSpecific {
				if !vndkdep.isVndkExt() {
					mctx.PropertyErrorf("vndk",
						"must set `extends: \"...\"` to vndk extension")
					return
				}
			} else {
				if vndkdep.isVndkExt() {
					mctx.PropertyErrorf("vndk",
						"must set `vendor: true` to set `extends: %q`",
						m.getVndkExtendsModuleName())
					return
				}
				if m.VendorProperties.Vendor_available == nil {
					mctx.PropertyErrorf("vndk",
						"vendor_available must be set to either true or false when `vndk: {enabled: true}`")
					return
				}
			}
		} else {
			if vndkdep.isVndkSp() {
				mctx.PropertyErrorf("vndk",
					"must set `enabled: true` to set `support_system_process: true`")
				return
			}
			if vndkdep.isVndkExt() {
				mctx.PropertyErrorf("vndk",
					"must set `enabled: true` to set `extends: %q`",
					m.getVndkExtendsModuleName())
				return
			}
		}
	}

	if mctx.DeviceConfig().VndkVersion() == "" {
		// If the device isn't compiling against the VNDK, we always
		// use the core mode.
		mctx.CreateVariations(coreMode)
	} else if _, ok := m.linker.(*llndkStubDecorator); ok {
		// LL-NDK stubs only exist in the vendor variant, since the
		// real libraries will be used in the core variant.
		mctx.CreateVariations(vendorMode)
	} else if _, ok := m.linker.(*llndkHeadersDecorator); ok {
		// ... and LL-NDK headers as well
		mod := mctx.CreateVariations(vendorMode)
		vendor := mod[0].(*Module)
		vendor.Properties.UseVndk = true
	} else if _, ok := m.linker.(*vndkPrebuiltLibraryDecorator); ok {
		// Make vendor variants only for the versions in BOARD_VNDK_VERSION and
		// PRODUCT_EXTRA_VNDK_VERSIONS.
		mod := mctx.CreateVariations(vendorMode)
		vendor := mod[0].(*Module)
		vendor.Properties.UseVndk = true
	} else if m.hasVendorVariant() && !vendorSpecific {
		// This will be available in both /system and /vendor
		// or a /system directory that is available to vendor.
		mod := mctx.CreateVariations(coreMode, vendorMode)
		vendor := mod[1].(*Module)
		vendor.Properties.UseVndk = true
		squashVendorSrcs(vendor)
	} else if vendorSpecific && String(m.Properties.Sdk_version) == "" {
		// This will be available in /vendor (or /odm) only
		mod := mctx.CreateVariations(vendorMode)
		vendor := mod[0].(*Module)
		vendor.Properties.UseVndk = true
		squashVendorSrcs(vendor)
	} else {
		// This is either in /system (or similar: /data), or is a
		// modules built with the NDK. Modules built with the NDK
		// will be restricted using the existing link type checks.
		mctx.CreateVariations(coreMode)
	}
}

func getCurrentNdkPrebuiltVersion(ctx DepsContext) string {
	if ctx.Config().PlatformSdkVersionInt() > config.NdkMaxPrebuiltVersionInt {
		return strconv.Itoa(config.NdkMaxPrebuiltVersionInt)
	}
	return ctx.Config().PlatformSdkVersion()
}

var Bool = proptools.Bool
var BoolPtr = proptools.BoolPtr
var String = proptools.String
var StringPtr = proptools.StringPtr
