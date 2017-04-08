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
		ctx.BottomUp("link", linkageMutator).Parallel()
		ctx.BottomUp("ndk_api", ndkApiMutator).Parallel()
		ctx.BottomUp("test_per_src", testPerSrcMutator).Parallel()
		ctx.BottomUp("begin", beginMutator).Parallel()
	})

	android.PostDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("asan_deps", sanitizerDepsMutator(asan))
		ctx.BottomUp("asan", sanitizerMutator(asan)).Parallel()

		ctx.TopDown("tsan_deps", sanitizerDepsMutator(tsan))
		ctx.BottomUp("tsan", sanitizerMutator(tsan)).Parallel()

		ctx.BottomUp("coverage", coverageLinkingMutator).Parallel()
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
}

type Flags struct {
	GlobalFlags []string // Flags that apply to C, C++, and assembly source files
	ArFlags     []string // Flags that apply to ar
	AsFlags     []string // Flags that apply to assembly source files
	CFlags      []string // Flags that apply to C and C++ source files
	ConlyFlags  []string // Flags that apply to C source files
	CppFlags    []string // Flags that apply to C++ source files
	YaccFlags   []string // Flags that apply to Yacc source files
	protoFlags  []string // Flags that apply to proto source files
	aidlFlags   []string // Flags that apply to aidl source files
	LdFlags     []string // Flags that apply to linker command lines
	libFlags    []string // Flags to add libraries early to the link order
	TidyFlags   []string // Flags that apply to clang-tidy
	YasmFlags   []string // Flags that apply to yasm assembly source files

	// Global include flags that apply to C, C++, and assembly source files
	// These must be after any module include flags, which will be in GlobalFlags.
	SystemIncludeFlags []string

	Toolchain config.Toolchain
	Clang     bool
	Tidy      bool
	Coverage  bool

	RequiredInstructionSet string
	DynamicLinker          string

	CFlagsDeps android.Paths // Files depended on by compiler flags

	GroupStaticLibs bool
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
	PreventInstall      bool     `blueprint:"mutated"`
}

type UnusedProperties struct {
	Tags []string
}

type ModuleContextIntf interface {
	static() bool
	staticBinary() bool
	clang() bool
	toolchain() config.Toolchain
	noDefaultCompilerFlags() bool
	sdk() bool
	sdkVersion() string
	vndk() bool
	selectedStl() string
	baseModuleName() string
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
	compilerFlags(ctx ModuleContext, flags Flags) Flags
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
	reuseObjTag           = dependencyTag{name: "reuse objects"}
	ndkStubDepTag         = dependencyTag{name: "ndk stub", library: true}
	ndkLateStubDepTag     = dependencyTag{name: "ndk late stub", library: true}
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
	features  []feature
	compiler  compiler
	linker    linker
	installer installer
	stl       *stl
	sanitize  *sanitize
	coverage  *coverage

	androidMkSharedLibDeps []string

	outputFile android.OptionalPath

	cachedToolchain config.Toolchain

	subAndroidMkOnce map[subAndroidMkProvider]bool

	// Flags used to compile this module
	flags Flags
}

func (c *Module) Init() (blueprint.Module, []interface{}) {
	props := []interface{}{&c.Properties, &c.unused}
	if c.compiler != nil {
		props = append(props, c.compiler.compilerProps()...)
	}
	if c.linker != nil {
		props = append(props, c.linker.linkerProps()...)
	}
	if c.installer != nil {
		props = append(props, c.installer.installerProps()...)
	}
	if c.stl != nil {
		props = append(props, c.stl.props()...)
	}
	if c.sanitize != nil {
		props = append(props, c.sanitize.props()...)
	}
	if c.coverage != nil {
		props = append(props, c.coverage.props()...)
	}
	for _, feature := range c.features {
		props = append(props, feature.props()...)
	}

	_, props = android.InitAndroidArchModule(c, c.hod, c.multilib, props...)

	return android.InitDefaultableModule(c, c, props...)
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
	if static, ok := ctx.mod.linker.(interface {
		static() bool
	}); ok {
		return static.static()
	}
	return false
}

func (ctx *moduleContextImpl) staticBinary() bool {
	if static, ok := ctx.mod.linker.(interface {
		staticBinary() bool
	}); ok {
		return static.staticBinary()
	}
	return false
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
		if ctx.vndk() {
			return "current"
		} else {
			return ctx.mod.Properties.Sdk_version
		}
	}
	return ""
}

func (ctx *moduleContextImpl) vndk() bool {
	return ctx.ctx.Os() == android.Android && ctx.ctx.Vendor() && ctx.ctx.DeviceConfig().CompileVndk()
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
	if p, ok := c.linker.(prebuiltLinkerInterface); ok {
		name = p.Name(name)
	}
	return name
}

func (c *Module) GenerateAndroidBuildActions(actx android.ModuleContext) {
	ctx := &moduleContext{
		ModuleContext: actx,
		moduleContextImpl: moduleContextImpl{
			mod: c,
		},
	}
	ctx.ctx = ctx

	flags := Flags{
		Toolchain: c.toolchain(ctx),
		Clang:     c.clang(ctx),
	}
	if c.compiler != nil {
		flags = c.compiler.compilerFlags(ctx, flags)
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
	for _, feature := range c.features {
		flags = feature.flags(ctx, flags)
	}
	if ctx.Failed() {
		return
	}

	flags.CFlags, _ = filterList(flags.CFlags, config.IllegalFlags)
	flags.CppFlags, _ = filterList(flags.CppFlags, config.IllegalFlags)
	flags.ConlyFlags, _ = filterList(flags.ConlyFlags, config.IllegalFlags)

	deps := c.depsToPaths(ctx)
	if ctx.Failed() {
		return
	}
	flags.GlobalFlags = append(flags.GlobalFlags, deps.Flags...)
	c.flags = flags

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
	for _, feature := range c.features {
		feature.begin(ctx)
	}
	if ctx.sdk() {
		version, err := normalizeNdkApiLevel(ctx.sdkVersion(), ctx.Arch())
		if err != nil {
			ctx.PropertyErrorf("sdk_version", err.Error())
		}
		c.Properties.Sdk_version = version
	}
}

func (c *Module) deps(ctx DepsContext) Deps {
	deps := Deps{}

	if c.compiler != nil {
		deps = c.compiler.compilerDeps(ctx, deps)
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
	for _, feature := range c.features {
		deps = feature.deps(ctx, deps)
	}

	deps.WholeStaticLibs = lastUniqueElements(deps.WholeStaticLibs)
	deps.StaticLibs = lastUniqueElements(deps.StaticLibs)
	deps.LateStaticLibs = lastUniqueElements(deps.LateStaticLibs)
	deps.SharedLibs = lastUniqueElements(deps.SharedLibs)
	deps.LateSharedLibs = lastUniqueElements(deps.LateSharedLibs)
	deps.HeaderLibs = lastUniqueElements(deps.HeaderLibs)

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

	c.Properties.AndroidMkSharedLibs = append(c.Properties.AndroidMkSharedLibs, deps.SharedLibs...)
	c.Properties.AndroidMkSharedLibs = append(c.Properties.AndroidMkSharedLibs, deps.LateSharedLibs...)

	variantNdkLibs := []string{}
	variantLateNdkLibs := []string{}
	if ctx.Os() == android.Android {
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
				if ctx.sdk() && inList(entry, ndkPrebuiltSharedLibraries) {
					if !inList(entry, ndkMigratedLibs) {
						nonvariantLibs = append(nonvariantLibs, entry+".ndk."+version)
					} else {
						variantLibs = append(variantLibs, entry+ndkLibrarySuffix)
					}
				} else if ctx.vndk() && inList(entry, config.LLndkLibraries()) {
					nonvariantLibs = append(nonvariantLibs, entry+llndkLibrarySuffix)
				} else {
					nonvariantLibs = append(nonvariantLibs, entry)
				}
			}
			return nonvariantLibs, variantLibs
		}

		deps.SharedLibs, variantNdkLibs = rewriteNdkLibs(deps.SharedLibs)
		deps.LateSharedLibs, variantLateNdkLibs = rewriteNdkLibs(deps.LateSharedLibs)
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

	version := ctx.sdkVersion()
	actx.AddVariationDependencies([]blueprint.Variation{
		{"ndk_api", version}, {"link", "shared"}}, ndkStubDepTag, variantNdkLibs...)
	actx.AddVariationDependencies([]blueprint.Variation{
		{"ndk_api", version}, {"link", "shared"}}, ndkLateStubDepTag, variantLateNdkLibs...)
}

func beginMutator(ctx android.BottomUpMutatorContext) {
	if c, ok := ctx.Module().(*Module); ok && c.Enabled() {
		c.beginMutator(ctx)
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
	checkLinkType := func(from, to *Module) {
		if from.Target().Os != android.Android {
			// Host code is not restricted
			return
		}
		if from.Properties.Sdk_version == "" {
			// Platform code can link to anything
			return
		}
		if _, ok := to.linker.(*toolchainLibraryDecorator); ok {
			// These are always allowed
			return
		}
		if _, ok := to.linker.(*ndkPrebuiltLibraryLinker); ok {
			// These are allowed, but don't set sdk_version
			return
		}
		if _, ok := to.linker.(*ndkPrebuiltStlLinker); ok {
			// These are allowed, but don't set sdk_version
			return
		}
		if _, ok := to.linker.(*stubDecorator); ok {
			// These aren't real libraries, but are the stub shared libraries that are included in
			// the NDK.
			return
		}
		if to.Properties.Sdk_version == "" {
			// NDK code linking to platform code is never okay.
			ctx.ModuleErrorf("depends on non-NDK-built library %q",
				ctx.OtherModuleName(to))
		}

		// All this point we know we have two NDK libraries, but we need to
		// check that we're not linking against anything built against a higher
		// API level, as it is only valid to link against older or equivalent
		// APIs.

		if from.Properties.Sdk_version == "current" {
			// Current can link against anything.
			return
		} else if to.Properties.Sdk_version == "current" {
			// Current can't be linked against by anything else.
			ctx.ModuleErrorf("links %q built against newer API version %q",
				ctx.OtherModuleName(to), "current")
		}

		fromApi, err := strconv.Atoi(from.Properties.Sdk_version)
		if err != nil {
			ctx.PropertyErrorf("sdk_version",
				"Invalid sdk_version value (must be int): %q",
				from.Properties.Sdk_version)
		}
		toApi, err := strconv.Atoi(to.Properties.Sdk_version)
		if err != nil {
			ctx.PropertyErrorf("sdk_version",
				"Invalid sdk_version value (must be int): %q",
				to.Properties.Sdk_version)
		}

		if toApi > fromApi {
			ctx.ModuleErrorf("links %q built against newer API version %q",
				ctx.OtherModuleName(to), to.Properties.Sdk_version)
		}
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
			case android.DefaultsDepTag, android.SourceDepTag:
			case genSourceDepTag:
				if genRule, ok := m.(genrule.SourceFileGenerator); ok {
					depPaths.GeneratedSources = append(depPaths.GeneratedSources,
						genRule.GeneratedSourceFiles()...)
				} else {
					ctx.ModuleErrorf("module %q is not a gensrcs or genrule", name)
				}
			case genHeaderDepTag, genHeaderExportDepTag:
				if genRule, ok := m.(genrule.SourceFileGenerator); ok {
					depPaths.GeneratedHeaders = append(depPaths.GeneratedHeaders,
						genRule.GeneratedSourceFiles()...)
					flags := includeDirsToFlags(genRule.GeneratedHeaderDirs())
					depPaths.Flags = append(depPaths.Flags, flags)
					if tag == genHeaderExportDepTag {
						depPaths.ReexportedFlags = append(depPaths.ReexportedFlags, flags)
						depPaths.ReexportedFlagsDeps = append(depPaths.ReexportedFlagsDeps,
							genRule.GeneratedSourceFiles()...)
					}
				} else {
					ctx.ModuleErrorf("module %q is not a genrule", name)
				}
			default:
				ctx.ModuleErrorf("depends on non-cc module %q", name)
			}
			return
		}

		if !a.Enabled() {
			if ctx.AConfig().AllowMissingDependencies() {
				ctx.AddMissingDependencies([]string{name})
			} else {
				ctx.ModuleErrorf("depends on disabled module %q", name)
			}
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

		if tag == reuseObjTag {
			if l, ok := cc.compiler.(libraryInterface); ok {
				depPaths.Objs = depPaths.Objs.Append(l.reuseObjs())
				return
			}
		}

		if t, ok := tag.(dependencyTag); ok && t.library {
			if i, ok := cc.linker.(exportedFlagsProducer); ok {
				flags := i.exportedFlags()
				deps := i.exportedFlagsDeps()
				depPaths.Flags = append(depPaths.Flags, flags...)
				depPaths.GeneratedHeaders = append(depPaths.GeneratedHeaders, deps...)

				if t.reexportFlags {
					depPaths.ReexportedFlags = append(depPaths.ReexportedFlags, flags...)
					depPaths.ReexportedFlagsDeps = append(depPaths.ReexportedFlagsDeps, deps...)
				}
			}

			checkLinkType(c, cc)
		}

		var ptr *android.Paths
		var depPtr *android.Paths

		linkFile := cc.outputFile
		depFile := android.OptionalPath{}

		switch tag {
		case ndkStubDepTag, sharedDepTag, sharedExportDepTag:
			ptr = &depPaths.SharedLibs
			depPtr = &depPaths.SharedLibsDeps
			depFile = cc.linker.(libraryInterface).toc()
		case lateSharedDepTag, ndkLateStubDepTag:
			ptr = &depPaths.LateSharedLibs
			depPtr = &depPaths.LateSharedLibsDeps
			depFile = cc.linker.(libraryInterface).toc()
		case staticDepTag, staticExportDepTag:
			ptr = &depPaths.StaticLibs
		case lateStaticDepTag:
			ptr = &depPaths.LateStaticLibs
		case wholeStaticDepTag:
			ptr = &depPaths.WholeStaticLibs
			staticLib, ok := cc.linker.(libraryInterface)
			if !ok || !staticLib.static() {
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

		switch tag {
		case staticDepTag, staticExportDepTag, lateStaticDepTag:
			staticLib, ok := cc.linker.(libraryInterface)
			if !ok || !staticLib.static() {
				ctx.ModuleErrorf("module %q not a static library", name)
				return
			}

			// When combining coverage files for shared libraries and executables, coverage files
			// in static libraries act as if they were whole static libraries.
			depPaths.StaticLibObjs.coverageFiles = append(depPaths.StaticLibObjs.coverageFiles,
				staticLib.objs().coverageFiles...)
		}

		if ptr != nil {
			if !linkFile.Valid() {
				ctx.ModuleErrorf("module %q missing output file", name)
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
	})

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

//
// Defaults
//
type Defaults struct {
	android.ModuleBase
	android.DefaultsModule
}

func (*Defaults) GenerateAndroidBuildActions(ctx android.ModuleContext) {
}

func (d *Defaults) DepsMutator(ctx android.BottomUpMutatorContext) {
}

func defaultsFactory() (blueprint.Module, []interface{}) {
	return DefaultsFactory()
}

func DefaultsFactory(props ...interface{}) (blueprint.Module, []interface{}) {
	module := &Defaults{}

	props = append(props,
		&BaseProperties{},
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
	)

	return android.InitDefaultsModule(module, module, props...)
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
