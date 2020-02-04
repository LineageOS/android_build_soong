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
		ctx.BottomUp("image", ImageMutator).Parallel()
		ctx.BottomUp("link", LinkageMutator).Parallel()
		ctx.BottomUp("vndk", VndkMutator).Parallel()
		ctx.BottomUp("ndk_api", ndkApiMutator).Parallel()
		ctx.BottomUp("test_per_src", testPerSrcMutator).Parallel()
		ctx.BottomUp("version", VersionMutator).Parallel()
		ctx.BottomUp("begin", BeginMutator).Parallel()
		ctx.BottomUp("sysprop", SyspropMutator).Parallel()
	})

	android.PostDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("asan_deps", sanitizerDepsMutator(asan))
		ctx.BottomUp("asan", sanitizerMutator(asan)).Parallel()

		ctx.TopDown("hwasan_deps", sanitizerDepsMutator(hwasan))
		ctx.BottomUp("hwasan", sanitizerMutator(hwasan)).Parallel()

		// cfi mutator shouldn't run before sanitizers that return true for
		// incompatibleWithCfi()
		ctx.TopDown("cfi_deps", sanitizerDepsMutator(cfi))
		ctx.BottomUp("cfi", sanitizerMutator(cfi)).Parallel()

		ctx.TopDown("scs_deps", sanitizerDepsMutator(scs))
		ctx.BottomUp("scs", sanitizerMutator(scs)).Parallel()

		ctx.TopDown("tsan_deps", sanitizerDepsMutator(tsan))
		ctx.BottomUp("tsan", sanitizerMutator(tsan)).Parallel()

		ctx.TopDown("sanitize_runtime_deps", sanitizerRuntimeDepsMutator)
		ctx.BottomUp("sanitize_runtime", sanitizerRuntimeMutator).Parallel()

		ctx.BottomUp("coverage", coverageMutator).Parallel()
		ctx.TopDown("vndk_deps", sabiDepsMutator)

		ctx.TopDown("lto_deps", ltoDepsMutator)
		ctx.BottomUp("lto", ltoMutator).Parallel()

		ctx.TopDown("double_loadable", checkDoubleLoadableLibraries).Parallel()
	})

	pctx.Import("android/soong/cc/config")
}

type Deps struct {
	SharedLibs, LateSharedLibs                  []string
	StaticLibs, LateStaticLibs, WholeStaticLibs []string
	HeaderLibs                                  []string
	RuntimeLibs                                 []string

	ReexportSharedLibHeaders, ReexportStaticLibHeaders, ReexportHeaderLibHeaders []string

	ObjFiles []string

	GeneratedSources []string
	GeneratedHeaders []string

	ReexportGeneratedHeaders []string

	CrtBegin, CrtEnd string

	// Used for host bionic
	LinkerFlagsFile string
	DynamicLinker   string
}

type PathDeps struct {
	// Paths to .so files
	SharedLibs, EarlySharedLibs, LateSharedLibs android.Paths
	// Paths to the dependencies to use for .so files (.so.toc files)
	SharedLibsDeps, EarlySharedLibsDeps, LateSharedLibsDeps android.Paths
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

	// Path to the file container flags to use with the linker
	LinkerFlagsFile android.OptionalPath

	// Path to the dynamic linker binary
	DynamicLinker android.OptionalPath
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
	Tidy      bool
	Coverage  bool
	SAbiDump  bool

	RequiredInstructionSet string
	DynamicLinker          string

	CFlagsDeps  android.Paths // Files depended on by compiler flags
	LdFlagsDeps android.Paths // Files depended on by linker flags

	GroupStaticLibs bool

	proto            android.ProtoFlags
	protoC           bool // Whether to use C instead of C++
	protoOptionsFile bool // Whether to look for a .options file next to the .proto
}

type ObjectLinkerProperties struct {
	// names of other cc_object modules to link into this module using partial linking
	Objs []string `android:"arch_variant"`

	// if set, add an extra objcopy --prefix-symbols= step
	Prefix_symbols *string
}

// Properties used to compile all C or C++ modules
type BaseProperties struct {
	// Deprecated. true is the default, false is invalid.
	Clang *bool `android:"arch_variant"`

	// Minimum sdk version supported when compiling against the ndk
	Sdk_version *string

	AndroidMkSharedLibs       []string `blueprint:"mutated"`
	AndroidMkStaticLibs       []string `blueprint:"mutated"`
	AndroidMkRuntimeLibs      []string `blueprint:"mutated"`
	AndroidMkWholeStaticLibs  []string `blueprint:"mutated"`
	HideFromMake              bool     `blueprint:"mutated"`
	PreventInstall            bool     `blueprint:"mutated"`
	ApexesProvidingSharedLibs []string `blueprint:"mutated"`

	UseVndk bool `blueprint:"mutated"`

	// *.logtags files, to combine together in order to generate the /system/etc/event-log-tags
	// file
	Logtags []string

	// Make this module available when building for recovery
	Recovery_available *bool

	InRecovery bool `blueprint:"mutated"`

	// Allows this module to use non-APEX version of libraries. Useful
	// for building binaries that are started before APEXes are activated.
	Bootstrap *bool
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

	// whether this module is capable of being loaded with other instance
	// (possibly an older version) of the same module in the same process.
	// Currently, a shared library that is a member of VNDK (vndk: {enabled: true})
	// can be double loaded in a vendor process if the library is also a
	// (direct and indirect) dependency of an LLNDK library. Such libraries must be
	// explicitly marked as `double_loadable: true` by the owner, or the dependency
	// from the LLNDK lib should be cut if the lib is not designed to be double loaded.
	Double_loadable *bool
}

type ModuleContextIntf interface {
	static() bool
	staticBinary() bool
	header() bool
	toolchain() config.Toolchain
	useSdk() bool
	sdkVersion() string
	useVndk() bool
	isNdk() bool
	isLlndk() bool
	isLlndkPublic() bool
	isVndkPrivate() bool
	isVndk() bool
	isVndkSp() bool
	isVndkExt() bool
	inRecovery() bool
	shouldCreateVndkSourceAbiDump() bool
	selectedStl() string
	baseModuleName() string
	getVndkExtendsModuleName() string
	isPgoCompile() bool
	isNDKStubLibrary() bool
	useClangLld(actx ModuleContext) bool
	apexName() string
	hasStubsVariants() bool
	isStubs() bool
	bootstrap() bool
	mustUseVendorVariant() bool
	nativeCoverage() bool
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
	useClangLld(actx ModuleContext) bool

	link(ctx ModuleContext, flags Flags, deps PathDeps, objs Objects) android.Path
	appendLdflags([]string)
	unstrippedOutputFilePath() android.Path

	nativeCoverage() bool
	coverageOutputFilePath() android.OptionalPath
}

type installer interface {
	installerProps() []interface{}
	install(ctx ModuleContext, path android.Path)
	inData() bool
	inSanitizerDir() bool
	hostToolPath() android.OptionalPath
	relativeInstallPath() string
}

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name    string
	library bool

	reexportFlags bool

	explicitlyVersioned bool
}

var (
	sharedDepTag          = dependencyTag{name: "shared", library: true}
	sharedExportDepTag    = dependencyTag{name: "shared", library: true, reexportFlags: true}
	earlySharedDepTag     = dependencyTag{name: "early_shared", library: true}
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
	linkerFlagsDepTag     = dependencyTag{name: "linker flags file"}
	dynamicLinkerDepTag   = dependencyTag{name: "dynamic linker"}
	reuseObjTag           = dependencyTag{name: "reuse objects"}
	staticVariantTag      = dependencyTag{name: "static variant"}
	ndkStubDepTag         = dependencyTag{name: "ndk stub", library: true}
	ndkLateStubDepTag     = dependencyTag{name: "ndk late stub", library: true}
	vndkExtDepTag         = dependencyTag{name: "vndk extends", library: true}
	runtimeDepTag         = dependencyTag{name: "runtime lib"}
	coverageDepTag        = dependencyTag{name: "coverage"}
)

// Module contains the properties and members used by all C/C++ module types, and implements
// the blueprint.Module interface.  It delegates to compiler, linker, and installer interfaces
// to construct the output file.  Behavior can be customized with a Customizer interface
type Module struct {
	android.ModuleBase
	android.DefaultableModuleBase
	android.ApexModuleBase

	Properties       BaseProperties
	VendorProperties VendorProperties

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
	xom       *xom

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

func (c *Module) OutputFile() android.OptionalPath {
	return c.outputFile
}

func (c *Module) UnstrippedOutputFile() android.Path {
	if c.linker != nil {
		return c.linker.unstrippedOutputFilePath()
	}
	return nil
}

func (c *Module) CoverageOutputFile() android.OptionalPath {
	if c.linker != nil {
		return c.linker.coverageOutputFilePath()
	}
	return android.OptionalPath{}
}

func (c *Module) RelativeInstallPath() string {
	if c.installer != nil {
		return c.installer.relativeInstallPath()
	}
	return ""
}

func (c *Module) Init() android.Module {
	c.AddProperties(&c.Properties, &c.VendorProperties)
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
	if c.xom != nil {
		c.AddProperties(c.xom.props()...)
	}
	for _, feature := range c.features {
		c.AddProperties(feature.props()...)
	}

	c.Prefer32(func(ctx android.BaseModuleContext, base *android.ModuleBase, class android.OsClass) bool {
		switch class {
		case android.Device:
			return ctx.Config().DevicePrefer32BitExecutables()
		case android.HostCross:
			// Windows builds always prefer 32-bit
			return true
		default:
			return false
		}
	})
	android.InitAndroidArchModule(c, c.hod, c.multilib)

	android.InitDefaultableModule(c)

	android.InitApexModule(c)

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

func (c *Module) isCoverageVariant() bool {
	return c.coverage.Properties.IsCoverageVariant
}

func (c *Module) isNdk() bool {
	return inList(c.Name(), ndkMigratedLibs)
}

func (c *Module) isLlndk() bool {
	// Returns true for both LLNDK (public) and LLNDK-private libs.
	return inList(c.Name(), llndkLibraries)
}

func (c *Module) isLlndkPublic() bool {
	// Returns true only for LLNDK (public) libs.
	return c.isLlndk() && !c.isVndkPrivate()
}

func (c *Module) isVndkPrivate() bool {
	// Returns true for LLNDK-private, VNDK-SP-private, and VNDK-core-private.
	return inList(c.Name(), vndkPrivateLibraries)
}

func (c *Module) isVndk() bool {
	if vndkdep := c.vndkdep; vndkdep != nil {
		return vndkdep.isVndk()
	}
	return false
}

func (c *Module) isPgoCompile() bool {
	if pgo := c.pgo; pgo != nil {
		return pgo.Properties.PgoCompile
	}
	return false
}

func (c *Module) isNDKStubLibrary() bool {
	if _, ok := c.compiler.(*stubDecorator); ok {
		return true
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

func (c *Module) mustUseVendorVariant() bool {
	return c.isVndkSp() || inList(c.Name(), config.VndkMustUseVendorVariantList)
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

func (c *Module) inRecovery() bool {
	return c.Properties.InRecovery || c.ModuleBase.InstallInRecovery()
}

func (c *Module) onlyInRecovery() bool {
	return c.ModuleBase.InstallInRecovery()
}

func (c *Module) IsStubs() bool {
	if library, ok := c.linker.(*libraryDecorator); ok {
		return library.buildStubs()
	} else if _, ok := c.linker.(*llndkStubDecorator); ok {
		return true
	}
	return false
}

func (c *Module) HasStubsVariants() bool {
	if library, ok := c.linker.(*libraryDecorator); ok {
		return len(library.Properties.Stubs.Versions) > 0
	}
	if library, ok := c.linker.(*prebuiltLibraryLinker); ok {
		return len(library.Properties.Stubs.Versions) > 0
	}
	return false
}

func (c *Module) bootstrap() bool {
	return Bool(c.Properties.Bootstrap)
}

func (c *Module) nativeCoverage() bool {
	return c.linker != nil && c.linker.nativeCoverage()
}

func isBionic(name string) bool {
	switch name {
	case "libc", "libm", "libdl", "linker":
		return true
	}
	return false
}

func installToBootstrap(name string, config android.Config) bool {
	if name == "libclang_rt.hwasan-aarch64-android" {
		return inList("hwaddress", config.SanitizeDevice())
	}
	return isBionic(name)
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

func (ctx *moduleContextImpl) toolchain() config.Toolchain {
	return ctx.mod.toolchain(ctx.ctx)
}

func (ctx *moduleContextImpl) static() bool {
	return ctx.mod.static()
}

func (ctx *moduleContextImpl) staticBinary() bool {
	return ctx.mod.staticBinary()
}

func (ctx *moduleContextImpl) header() bool {
	return ctx.mod.header()
}

func (ctx *moduleContextImpl) useSdk() bool {
	if ctx.ctx.Device() && !ctx.useVndk() && !ctx.inRecovery() && !ctx.ctx.Fuchsia() {
		return String(ctx.mod.Properties.Sdk_version) != ""
	}
	return false
}

func (ctx *moduleContextImpl) sdkVersion() string {
	if ctx.ctx.Device() {
		if ctx.useVndk() {
			vndk_ver := ctx.ctx.DeviceConfig().VndkVersion()
			if vndk_ver == "current" {
				platform_vndk_ver := ctx.ctx.DeviceConfig().PlatformVndkVersion()
				if inList(platform_vndk_ver, ctx.ctx.Config().PlatformVersionCombinedCodenames()) {
					return "current"
				}
				return platform_vndk_ver
			}
			return vndk_ver
		}
		return String(ctx.mod.Properties.Sdk_version)
	}
	return ""
}

func (ctx *moduleContextImpl) useVndk() bool {
	return ctx.mod.useVndk()
}

func (ctx *moduleContextImpl) isNdk() bool {
	return ctx.mod.isNdk()
}

func (ctx *moduleContextImpl) isLlndk() bool {
	return ctx.mod.isLlndk()
}

func (ctx *moduleContextImpl) isLlndkPublic() bool {
	return ctx.mod.isLlndkPublic()
}

func (ctx *moduleContextImpl) isVndkPrivate() bool {
	return ctx.mod.isVndkPrivate()
}

func (ctx *moduleContextImpl) isVndk() bool {
	return ctx.mod.isVndk()
}

func (ctx *moduleContextImpl) isPgoCompile() bool {
	return ctx.mod.isPgoCompile()
}

func (ctx *moduleContextImpl) isNDKStubLibrary() bool {
	return ctx.mod.isNDKStubLibrary()
}

func (ctx *moduleContextImpl) isVndkSp() bool {
	return ctx.mod.isVndkSp()
}

func (ctx *moduleContextImpl) isVndkExt() bool {
	return ctx.mod.isVndkExt()
}

func (ctx *moduleContextImpl) mustUseVendorVariant() bool {
	return ctx.mod.mustUseVendorVariant()
}

func (ctx *moduleContextImpl) inRecovery() bool {
	return ctx.mod.inRecovery()
}

// Check whether ABI dumps should be created for this module.
func (ctx *moduleContextImpl) shouldCreateVndkSourceAbiDump() bool {
	if ctx.ctx.Config().IsEnvTrue("SKIP_ABI_CHECKS") {
		return false
	}

	if ctx.ctx.Fuchsia() {
		return false
	}

	if sanitize := ctx.mod.sanitize; sanitize != nil {
		if !sanitize.isVariantOnProductionDevice() {
			return false
		}
	}
	if !ctx.ctx.Device() {
		// Host modules do not need ABI dumps.
		return false
	}
	if !ctx.mod.IsForPlatform() {
		// APEX variants do not need ABI dumps.
		return false
	}
	if ctx.isNdk() {
		return true
	}
	if ctx.isLlndkPublic() {
		return true
	}
	if ctx.useVndk() && ctx.isVndk() && !ctx.isVndkPrivate() {
		// Return true if this is VNDK-core, VNDK-SP, or VNDK-Ext and this is not
		// VNDK-private.
		return true
	}
	return false
}

func (ctx *moduleContextImpl) selectedStl() string {
	if stl := ctx.mod.stl; stl != nil {
		return stl.Properties.SelectedStl
	}
	return ""
}

func (ctx *moduleContextImpl) useClangLld(actx ModuleContext) bool {
	return ctx.mod.linker.useClangLld(actx)
}

func (ctx *moduleContextImpl) baseModuleName() string {
	return ctx.mod.ModuleBase.BaseModuleName()
}

func (ctx *moduleContextImpl) getVndkExtendsModuleName() string {
	return ctx.mod.getVndkExtendsModuleName()
}

func (ctx *moduleContextImpl) apexName() string {
	return ctx.mod.ApexName()
}

func (ctx *moduleContextImpl) hasStubsVariants() bool {
	return ctx.mod.HasStubsVariants()
}

func (ctx *moduleContextImpl) isStubs() bool {
	return ctx.mod.IsStubs()
}

func (ctx *moduleContextImpl) bootstrap() bool {
	return ctx.mod.bootstrap()
}

func (ctx *moduleContextImpl) nativeCoverage() bool {
	return ctx.mod.nativeCoverage()
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
	module.xom = &xom{}
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

func (c *Module) Symlinks() []string {
	if p, ok := c.installer.(interface {
		symlinkList() []string
	}); ok {
		return p.symlinkList()
	}
	return nil
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

	if c.Properties.Clang != nil && *c.Properties.Clang == false {
		ctx.PropertyErrorf("clang", "false (GCC) is no longer supported")
	}

	flags := Flags{
		Toolchain: c.toolchain(ctx),
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
		flags, deps = c.coverage.flags(ctx, flags, deps)
	}
	if c.lto != nil {
		flags = c.lto.flags(ctx, flags)
	}
	if c.pgo != nil {
		flags = c.pgo.flags(ctx, flags)
	}
	if c.xom != nil {
		flags = c.xom.flags(ctx, flags)
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

		// If a lib is directly included in any of the APEXes, unhide the stubs
		// variant having the latest version gets visible to make. In addition,
		// the non-stubs variant is renamed to <libname>.bootstrap. This is to
		// force anything in the make world to link against the stubs library.
		// (unless it is explicitly referenced via .bootstrap suffix or the
		// module is marked with 'bootstrap: true').
		if c.HasStubsVariants() &&
			android.DirectlyInAnyApex(ctx, ctx.baseModuleName()) &&
			!c.inRecovery() && !c.useVndk() && !c.static() && !c.isCoverageVariant() &&
			c.IsStubs() {
			c.Properties.HideFromMake = false // unhide
			// Note: this is still non-installable
		}
	}

	if c.installer != nil && !c.Properties.PreventInstall && c.IsForPlatform() && c.outputFile.Valid() {
		c.installer.install(ctx, c.outputFile.Path())
		if ctx.Failed() {
			return
		}
	}
}

func (c *Module) toolchain(ctx android.BaseContext) config.Toolchain {
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
	deps.RuntimeLibs = android.LastUniqueStrings(deps.RuntimeLibs)

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

// Split name#version into name and version
func stubsLibNameAndVersion(name string) (string, string) {
	if sharp := strings.LastIndex(name, "#"); sharp != -1 && sharp != len(name)-1 {
		version := name[sharp+1:]
		libname := name[:sharp]
		return libname, version
	}
	return name, ""
}

func (c *Module) DepsMutator(actx android.BottomUpMutatorContext) {
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
				// strip #version suffix out
				name, _ := stubsLibNameAndVersion(entry)
				if ctx.useSdk() && inList(name, ndkPrebuiltSharedLibraries) {
					if !inList(name, ndkMigratedLibs) {
						nonvariantLibs = append(nonvariantLibs, name+".ndk."+version)
					} else {
						variantLibs = append(variantLibs, name+ndkLibrarySuffix)
					}
				} else if ctx.useVndk() && inList(name, llndkLibraries) {
					nonvariantLibs = append(nonvariantLibs, name+llndkLibrarySuffix)
				} else if (ctx.Platform() || ctx.ProductSpecific()) && inList(name, vendorPublicLibraries) {
					vendorPublicLib := name + vendorPublicLibrarySuffix
					if actx.OtherModuleExists(vendorPublicLib) {
						nonvariantLibs = append(nonvariantLibs, vendorPublicLib)
					} else {
						// This can happen if vendor_public_library module is defined in a
						// namespace that isn't visible to the current module. In that case,
						// link to the original library.
						nonvariantLibs = append(nonvariantLibs, name)
					}
				} else {
					// put name#version back
					nonvariantLibs = append(nonvariantLibs, entry)
				}
			}
			return nonvariantLibs, variantLibs
		}

		deps.SharedLibs, variantNdkLibs = rewriteNdkLibs(deps.SharedLibs)
		deps.LateSharedLibs, variantLateNdkLibs = rewriteNdkLibs(deps.LateSharedLibs)
		deps.ReexportSharedLibHeaders, _ = rewriteNdkLibs(deps.ReexportSharedLibHeaders)
	}

	buildStubs := false
	if c.linker != nil {
		if library, ok := c.linker.(*libraryDecorator); ok {
			if library.buildStubs() {
				buildStubs = true
			}
		}
	}

	for _, lib := range deps.HeaderLibs {
		depTag := headerDepTag
		if inList(lib, deps.ReexportHeaderLibHeaders) {
			depTag = headerExportDepTag
		}
		if buildStubs {
			actx.AddFarVariationDependencies([]blueprint.Variation{
				{Mutator: "arch", Variation: ctx.Target().String()},
				{Mutator: "image", Variation: c.imageVariation()},
			}, depTag, lib)
		} else {
			actx.AddVariationDependencies(nil, depTag, lib)
		}
	}

	if buildStubs {
		// Stubs lib does not have dependency to other static/shared libraries.
		// Don't proceed.
		return
	}

	syspropImplLibraries := syspropImplLibraries(actx.Config())

	for _, lib := range deps.WholeStaticLibs {
		depTag := wholeStaticDepTag
		if impl, ok := syspropImplLibraries[lib]; ok {
			lib = impl
		}
		actx.AddVariationDependencies([]blueprint.Variation{
			{Mutator: "link", Variation: "static"},
		}, depTag, lib)
	}

	for _, lib := range deps.StaticLibs {
		depTag := staticDepTag
		if inList(lib, deps.ReexportStaticLibHeaders) {
			depTag = staticExportDepTag
		}

		if impl, ok := syspropImplLibraries[lib]; ok {
			lib = impl
		}

		actx.AddVariationDependencies([]blueprint.Variation{
			{Mutator: "link", Variation: "static"},
		}, depTag, lib)
	}

	actx.AddVariationDependencies([]blueprint.Variation{
		{Mutator: "link", Variation: "static"},
	}, lateStaticDepTag, deps.LateStaticLibs...)

	addSharedLibDependencies := func(depTag dependencyTag, name string, version string) {
		var variations []blueprint.Variation
		variations = append(variations, blueprint.Variation{Mutator: "link", Variation: "shared"})
		versionVariantAvail := !ctx.useVndk() && !c.inRecovery()
		if version != "" && versionVariantAvail {
			// Version is explicitly specified. i.e. libFoo#30
			variations = append(variations, blueprint.Variation{Mutator: "version", Variation: version})
			depTag.explicitlyVersioned = true
		}
		actx.AddVariationDependencies(variations, depTag, name)

		// If the version is not specified, add dependency to the latest stubs library.
		// The stubs library will be used when the depending module is built for APEX and
		// the dependent module is not in the same APEX.
		latestVersion := latestStubsVersionFor(actx.Config(), name)
		if version == "" && latestVersion != "" && versionVariantAvail {
			actx.AddVariationDependencies([]blueprint.Variation{
				{Mutator: "link", Variation: "shared"},
				{Mutator: "version", Variation: latestVersion},
			}, depTag, name)
			// Note that depTag.explicitlyVersioned is false in this case.
		}
	}

	// shared lib names without the #version suffix
	var sharedLibNames []string

	for _, lib := range deps.SharedLibs {
		depTag := sharedDepTag
		if inList(lib, deps.ReexportSharedLibHeaders) {
			depTag = sharedExportDepTag
		}

		if impl, ok := syspropImplLibraries[lib]; ok {
			lib = impl
		}

		name, version := stubsLibNameAndVersion(lib)
		sharedLibNames = append(sharedLibNames, name)

		addSharedLibDependencies(depTag, name, version)
	}

	for _, lib := range deps.LateSharedLibs {
		if inList(lib, sharedLibNames) {
			// This is to handle the case that some of the late shared libs (libc, libdl, libm, ...)
			// are added also to SharedLibs with version (e.g., libc#10). If not skipped, we will be
			// linking against both the stubs lib and the non-stubs lib at the same time.
			continue
		}
		addSharedLibDependencies(lateSharedDepTag, lib, "")
	}

	actx.AddVariationDependencies([]blueprint.Variation{
		{Mutator: "link", Variation: "shared"},
	}, runtimeDepTag, deps.RuntimeLibs...)

	actx.AddDependency(c, genSourceDepTag, deps.GeneratedSources...)

	for _, gen := range deps.GeneratedHeaders {
		depTag := genHeaderDepTag
		if inList(gen, deps.ReexportGeneratedHeaders) {
			depTag = genHeaderExportDepTag
		}
		actx.AddDependency(c, depTag, gen)
	}

	actx.AddVariationDependencies(nil, objDepTag, deps.ObjFiles...)

	if deps.CrtBegin != "" {
		actx.AddVariationDependencies(nil, crtBeginDepTag, deps.CrtBegin)
	}
	if deps.CrtEnd != "" {
		actx.AddVariationDependencies(nil, crtEndDepTag, deps.CrtEnd)
	}
	if deps.LinkerFlagsFile != "" {
		actx.AddDependency(c, linkerFlagsDepTag, deps.LinkerFlagsFile)
	}
	if deps.DynamicLinker != "" {
		actx.AddDependency(c, dynamicLinkerDepTag, deps.DynamicLinker)
	}

	version := ctx.sdkVersion()
	actx.AddVariationDependencies([]blueprint.Variation{
		{Mutator: "ndk_api", Variation: version},
		{Mutator: "link", Variation: "shared"},
	}, ndkStubDepTag, variantNdkLibs...)
	actx.AddVariationDependencies([]blueprint.Variation{
		{Mutator: "ndk_api", Variation: version},
		{Mutator: "link", Variation: "shared"},
	}, ndkLateStubDepTag, variantLateNdkLibs...)

	if vndkdep := c.vndkdep; vndkdep != nil {
		if vndkdep.isVndkExt() {
			baseModuleMode := vendorMode
			if actx.DeviceConfig().VndkVersion() == "" {
				baseModuleMode = coreMode
			}
			actx.AddVariationDependencies([]blueprint.Variation{
				{Mutator: "image", Variation: baseModuleMode},
				{Mutator: "link", Variation: "shared"},
			}, vndkExtDepTag, vndkdep.getVndkExtendsModuleName())
		}
	}
}

func BeginMutator(ctx android.BottomUpMutatorContext) {
	if c, ok := ctx.Module().(*Module); ok && c.Enabled() {
		c.beginMutator(ctx)
	}
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
	if from.inRecovery() {
		// Recovery code is not NDK
		return
	}
	if _, ok := to.linker.(*toolchainLibraryDecorator); ok {
		// These are always allowed
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

	if strings.HasPrefix(ctx.ModuleName(), "libclang_rt.") && to.Name() == "libc++" {
		// Bug: http://b/121358700 - Allow libclang_rt.* shared libraries (with sdk_version)
		// to link to libc++ (non-NDK and without sdk_version).
		return
	}

	if String(to.Properties.Sdk_version) == "" {
		// NDK code linking to platform code is never okay.
		ctx.ModuleErrorf("depends on non-NDK-built library %q",
			ctx.OtherModuleName(to))
		return
	}

	// At this point we know we have two NDK libraries, but we need to
	// check that we're not linking against anything built against a higher
	// API level, as it is only valid to link against older or equivalent
	// APIs.

	// Current can link against anything.
	if String(from.Properties.Sdk_version) != "current" {
		// Otherwise we need to check.
		if String(to.Properties.Sdk_version) == "current" {
			// Current can't be linked against by anything else.
			ctx.ModuleErrorf("links %q built against newer API version %q",
				ctx.OtherModuleName(to), "current")
		} else {
			fromApi, err := strconv.Atoi(String(from.Properties.Sdk_version))
			if err != nil {
				ctx.PropertyErrorf("sdk_version",
					"Invalid sdk_version value (must be int or current): %q",
					String(from.Properties.Sdk_version))
			}
			toApi, err := strconv.Atoi(String(to.Properties.Sdk_version))
			if err != nil {
				ctx.PropertyErrorf("sdk_version",
					"Invalid sdk_version value (must be int or current): %q",
					String(to.Properties.Sdk_version))
			}

			if toApi > fromApi {
				ctx.ModuleErrorf("links %q built against newer API version %q",
					ctx.OtherModuleName(to), String(to.Properties.Sdk_version))
			}
		}
	}

	// Also check that the two STL choices are compatible.
	fromStl := from.stl.Properties.SelectedStl
	toStl := to.stl.Properties.SelectedStl
	if fromStl == "" || toStl == "" {
		// Libraries that don't use the STL are unrestricted.
	} else if fromStl == "ndk_system" || toStl == "ndk_system" {
		// We can be permissive with the system "STL" since it is only the C++
		// ABI layer, but in the future we should make sure that everyone is
		// using either libc++ or nothing.
	} else if getNdkStlFamily(from) != getNdkStlFamily(to) {
		ctx.ModuleErrorf("uses %q and depends on %q which uses incompatible %q",
			from.stl.Properties.SelectedStl, ctx.OtherModuleName(to),
			to.stl.Properties.SelectedStl)
	}
}

// Tests whether the dependent library is okay to be double loaded inside a single process.
// If a library has a vendor variant and is a (transitive) dependency of an LLNDK library,
// it is subject to be double loaded. Such lib should be explicitly marked as double_loadable: true
// or as vndk-sp (vndk: { enabled: true, support_system_process: true}).
func checkDoubleLoadableLibraries(ctx android.TopDownMutatorContext) {
	check := func(child, parent android.Module) bool {
		to, ok := child.(*Module)
		if !ok {
			// follow thru cc.Defaults, etc.
			return true
		}

		if lib, ok := to.linker.(*libraryDecorator); !ok || !lib.shared() {
			return false
		}

		// if target lib has no vendor variant, keep checking dependency graph
		if !to.hasVendorVariant() {
			return true
		}

		if to.isVndkSp() || inList(child.Name(), llndkLibraries) || Bool(to.VendorProperties.Double_loadable) {
			return false
		}

		var stringPath []string
		for _, m := range ctx.GetWalkPath() {
			stringPath = append(stringPath, m.Name())
		}
		ctx.ModuleErrorf("links a library %q which is not LL-NDK, "+
			"VNDK-SP, or explicitly marked as 'double_loadable:true'. "+
			"(dependency: %s)", ctx.OtherModuleName(to), strings.Join(stringPath, " -> "))
		return false
	}
	if module, ok := ctx.Module().(*Module); ok {
		if lib, ok := module.linker.(*libraryDecorator); ok && lib.shared() {
			if inList(ctx.ModuleName(), llndkLibraries) || Bool(module.VendorProperties.Double_loadable) {
				ctx.WalkDeps(check)
			}
		}
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
						genRule.GeneratedDeps()...)
					flags := includeDirsToFlags(genRule.GeneratedHeaderDirs())
					depPaths.Flags = append(depPaths.Flags, flags)
					if depTag == genHeaderExportDepTag {
						depPaths.ReexportedFlags = append(depPaths.ReexportedFlags, flags)
						depPaths.ReexportedFlagsDeps = append(depPaths.ReexportedFlagsDeps,
							genRule.GeneratedDeps()...)
						// Add these re-exported flags to help header-abi-dumper to infer the abi exported by a library.
						c.sabi.Properties.ReexportedIncludeFlags = append(c.sabi.Properties.ReexportedIncludeFlags, flags)

					}
				} else {
					ctx.ModuleErrorf("module %q is not a genrule", depName)
				}
			case linkerFlagsDepTag:
				if genRule, ok := dep.(genrule.SourceFileGenerator); ok {
					files := genRule.GeneratedSourceFiles()
					if len(files) == 1 {
						depPaths.LinkerFlagsFile = android.OptionalPathForPath(files[0])
					} else if len(files) > 1 {
						ctx.ModuleErrorf("module %q can only generate a single file if used for a linker flag file", depName)
					}
				} else {
					ctx.ModuleErrorf("module %q is not a genrule", depName)
				}
			}
			return
		}

		if depTag == android.ProtoPluginDepTag {
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

		if depTag == staticVariantTag {
			if _, ok := ccDep.compiler.(libraryInterface); ok {
				c.staticVariant = ccDep
				return
			}
		}

		// Extract explicitlyVersioned field from the depTag and reset it inside the struct.
		// Otherwise, sharedDepTag and lateSharedDepTag with explicitlyVersioned set to true
		// won't be matched to sharedDepTag and lateSharedDepTag.
		explicitlyVersioned := false
		if t, ok := depTag.(dependencyTag); ok {
			explicitlyVersioned = t.explicitlyVersioned
			t.explicitlyVersioned = false
			depTag = t
		}

		if t, ok := depTag.(dependencyTag); ok && t.library {
			depIsStatic := false
			switch depTag {
			case staticDepTag, staticExportDepTag, lateStaticDepTag, wholeStaticDepTag:
				depIsStatic = true
			}
			if dependentLibrary, ok := ccDep.linker.(*libraryDecorator); ok && !depIsStatic {
				depIsStubs := dependentLibrary.buildStubs()
				depHasStubs := ccDep.HasStubsVariants()
				depInSameApex := android.DirectlyInApex(c.ApexName(), depName)
				depInPlatform := !android.DirectlyInAnyApex(ctx, depName)

				var useThisDep bool
				if depIsStubs && explicitlyVersioned {
					// Always respect dependency to the versioned stubs (i.e. libX#10)
					useThisDep = true
				} else if !depHasStubs {
					// Use non-stub variant if that is the only choice
					// (i.e. depending on a lib without stubs.version property)
					useThisDep = true
				} else if c.IsForPlatform() {
					// If not building for APEX, use stubs only when it is from
					// an APEX (and not from platform)
					useThisDep = (depInPlatform != depIsStubs)
					if c.inRecovery() || c.bootstrap() {
						// However, for recovery or bootstrap modules,
						// always link to non-stub variant
						useThisDep = !depIsStubs
					}
				} else {
					// If building for APEX, use stubs only when it is not from
					// the same APEX
					useThisDep = (depInSameApex != depIsStubs)
				}

				if !useThisDep {
					return // stop processing this dep
				}
			}

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
		case earlySharedDepTag:
			ptr = &depPaths.EarlySharedLibs
			depPtr = &depPaths.EarlySharedLibsDeps
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
		case dynamicLinkerDepTag:
			depPaths.DynamicLinker = linkFile
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

		makeLibName := func(depName string) string {
			libName := strings.TrimSuffix(depName, llndkLibrarySuffix)
			libName = strings.TrimSuffix(libName, vendorPublicLibrarySuffix)
			libName = strings.TrimPrefix(libName, "prebuilt_")
			isLLndk := inList(libName, llndkLibraries)
			isVendorPublicLib := inList(libName, vendorPublicLibraries)
			bothVendorAndCoreVariantsExist := ccDep.hasVendorVariant() || isLLndk

			if ctx.DeviceConfig().VndkUseCoreVariant() && ccDep.isVndk() && !ccDep.mustUseVendorVariant() {
				// The vendor module is a no-vendor-variant VNDK library.  Depend on the
				// core module instead.
				return libName
			} else if c.useVndk() && bothVendorAndCoreVariantsExist {
				// The vendor module in Make will have been renamed to not conflict with the core
				// module, so update the dependency name here accordingly.
				return libName + vendorSuffix
			} else if (ctx.Platform() || ctx.ProductSpecific()) && isVendorPublicLib {
				return libName + vendorPublicLibrarySuffix
			} else if ccDep.inRecovery() && !ccDep.onlyInRecovery() {
				return libName + recoverySuffix
			} else {
				return libName
			}
		}

		// Export the shared libs to Make.
		switch depTag {
		case sharedDepTag, sharedExportDepTag, lateSharedDepTag, earlySharedDepTag:
			if dependentLibrary, ok := ccDep.linker.(*libraryDecorator); ok {
				if dependentLibrary.buildStubs() && android.InAnyApex(depName) {
					// Add the dependency to the APEX(es) providing the library so that
					// m <module> can trigger building the APEXes as well.
					for _, an := range android.GetApexesForModule(depName) {
						c.Properties.ApexesProvidingSharedLibs = append(
							c.Properties.ApexesProvidingSharedLibs, an)
					}
				}
			}

			// Note: the order of libs in this list is not important because
			// they merely serve as Make dependencies and do not affect this lib itself.
			c.Properties.AndroidMkSharedLibs = append(
				c.Properties.AndroidMkSharedLibs, makeLibName(depName))
		case ndkStubDepTag, ndkLateStubDepTag:
			ndkStub := ccDep.linker.(*stubDecorator)
			c.Properties.AndroidMkSharedLibs = append(
				c.Properties.AndroidMkSharedLibs,
				depName+"."+ndkStub.properties.ApiLevel)
		case staticDepTag, staticExportDepTag, lateStaticDepTag:
			c.Properties.AndroidMkStaticLibs = append(
				c.Properties.AndroidMkStaticLibs, makeLibName(depName))
		case runtimeDepTag:
			c.Properties.AndroidMkRuntimeLibs = append(
				c.Properties.AndroidMkRuntimeLibs, makeLibName(depName))
		case wholeStaticDepTag:
			c.Properties.AndroidMkWholeStaticLibs = append(
				c.Properties.AndroidMkWholeStaticLibs, makeLibName(depName))
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

func (c *Module) InstallInRecovery() bool {
	return c.inRecovery()
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

func (c *Module) staticBinary() bool {
	if static, ok := c.linker.(interface {
		staticBinary() bool
	}); ok {
		return static.staticBinary()
	}
	return false
}

func (c *Module) header() bool {
	if h, ok := c.linker.(interface {
		header() bool
	}); ok {
		return h.header()
	}
	return false
}

func (c *Module) getMakeLinkType() string {
	if c.useVndk() {
		if inList(c.Name(), vndkCoreLibraries) || inList(c.Name(), vndkSpLibraries) || inList(c.Name(), llndkLibraries) {
			if inList(c.Name(), vndkPrivateLibraries) {
				return "native:vndk_private"
			} else {
				return "native:vndk"
			}
		} else {
			return "native:vendor"
		}
	} else if c.inRecovery() {
		return "native:recovery"
	} else if c.Target().Os == android.Android && String(c.Properties.Sdk_version) != "" {
		return "native:ndk:none:none"
		// TODO(b/114741097): use the correct ndk stl once build errors have been fixed
		//family, link := getNdkStlFamilyAndLinkType(c)
		//return fmt.Sprintf("native:ndk:%s:%s", family, link)
	} else if inList(c.Name(), vndkUsingCoreVariantLibraries) {
		return "native:platform_vndk"
	} else {
		return "native:platform"
	}
}

// Overrides ApexModule.IsInstallabeToApex()
// Only shared libraries are installable to APEX.
func (c *Module) IsInstallableToApex() bool {
	if shared, ok := c.linker.(interface {
		shared() bool
	}); ok {
		return shared.shared()
	}
	return false
}

func (c *Module) imageVariation() string {
	variation := "core"
	if c.useVndk() {
		variation = "vendor"
	} else if c.inRecovery() {
		variation = "recovery"
	}
	return variation
}

func (c *Module) IDEInfo(dpInfo *android.IdeInfo) {
	dpInfo.Srcs = append(dpInfo.Srcs, c.Srcs().Strings()...)
}

//
// Defaults
//
type Defaults struct {
	android.ModuleBase
	android.DefaultsModuleBase
	android.ApexModuleBase
}

func (*Defaults) GenerateAndroidBuildActions(ctx android.ModuleContext) {
}

// cc_defaults provides a set of properties that can be inherited by other cc
// modules. A module can use the properties from a cc_defaults using
// `defaults: ["<:default_module_name>"]`. Properties of both modules are
// merged (when possible) by prepending the default module's values to the
// depending module's values.
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
		&XomProperties{},
		&android.ProtoProperties{},
	)

	android.InitDefaultsModule(module)
	android.InitApexModule(module)

	return module
}

const (
	// coreMode is the variant used for framework-private libraries, or
	// SDK libraries. (which framework-private libraries can use)
	coreMode = "core"

	// vendorMode is the variant used for /vendor code that compiles
	// against the VNDK.
	vendorMode = "vendor"

	recoveryMode = "recovery"
)

func squashVendorSrcs(m *Module) {
	if lib, ok := m.compiler.(*libraryDecorator); ok {
		lib.baseCompiler.Properties.Srcs = append(lib.baseCompiler.Properties.Srcs,
			lib.baseCompiler.Properties.Target.Vendor.Srcs...)

		lib.baseCompiler.Properties.Exclude_srcs = append(lib.baseCompiler.Properties.Exclude_srcs,
			lib.baseCompiler.Properties.Target.Vendor.Exclude_srcs...)
	}
}

func squashRecoverySrcs(m *Module) {
	if lib, ok := m.compiler.(*libraryDecorator); ok {
		lib.baseCompiler.Properties.Srcs = append(lib.baseCompiler.Properties.Srcs,
			lib.baseCompiler.Properties.Target.Recovery.Srcs...)

		lib.baseCompiler.Properties.Exclude_srcs = append(lib.baseCompiler.Properties.Exclude_srcs,
			lib.baseCompiler.Properties.Target.Recovery.Exclude_srcs...)
	}
}

func ImageMutator(mctx android.BottomUpMutatorContext) {
	if mctx.Os() != android.Android {
		return
	}

	if g, ok := mctx.Module().(*genrule.Module); ok {
		if props, ok := g.Extra.(*GenruleExtraProperties); ok {
			var coreVariantNeeded bool = false
			var vendorVariantNeeded bool = false
			var recoveryVariantNeeded bool = false
			if mctx.DeviceConfig().VndkVersion() == "" {
				coreVariantNeeded = true
			} else if Bool(props.Vendor_available) {
				coreVariantNeeded = true
				vendorVariantNeeded = true
			} else if mctx.SocSpecific() || mctx.DeviceSpecific() {
				vendorVariantNeeded = true
			} else {
				coreVariantNeeded = true
			}
			if Bool(props.Recovery_available) {
				recoveryVariantNeeded = true
			}

			if recoveryVariantNeeded {
				primaryArch := mctx.Config().DevicePrimaryArchType()
				moduleArch := g.Target().Arch.ArchType
				if moduleArch != primaryArch {
					recoveryVariantNeeded = false
				}
			}

			var variants []string
			if coreVariantNeeded {
				variants = append(variants, coreMode)
			}
			if vendorVariantNeeded {
				variants = append(variants, vendorMode)
			}
			if recoveryVariantNeeded {
				variants = append(variants, recoveryMode)
			}
			mod := mctx.CreateVariations(variants...)
			for i, v := range variants {
				if v == recoveryMode {
					m := mod[i].(*genrule.Module)
					m.Extra.(*GenruleExtraProperties).InRecovery = true
				}
			}
		}
	}

	m, ok := mctx.Module().(*Module)
	if !ok {
		return
	}

	// Sanity check
	vendorSpecific := mctx.SocSpecific() || mctx.DeviceSpecific()
	productSpecific := mctx.ProductSpecific()

	if m.VendorProperties.Vendor_available != nil && vendorSpecific {
		mctx.PropertyErrorf("vendor_available",
			"doesn't make sense at the same time as `vendor: true`, `proprietary: true`, or `device_specific:true`")
		return
	}

	if vndkdep := m.vndkdep; vndkdep != nil {
		if vndkdep.isVndk() {
			if productSpecific {
				mctx.PropertyErrorf("product_specific",
					"product_specific must not be true when `vndk: {enabled: true}`")
				return
			}
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

	var coreVariantNeeded bool = false
	var vendorVariantNeeded bool = false
	var recoveryVariantNeeded bool = false

	if mctx.DeviceConfig().VndkVersion() == "" {
		// If the device isn't compiling against the VNDK, we always
		// use the core mode.
		coreVariantNeeded = true
	} else if _, ok := m.linker.(*llndkStubDecorator); ok {
		// LL-NDK stubs only exist in the vendor variant, since the
		// real libraries will be used in the core variant.
		vendorVariantNeeded = true
	} else if _, ok := m.linker.(*llndkHeadersDecorator); ok {
		// ... and LL-NDK headers as well
		vendorVariantNeeded = true
	} else if _, ok := m.linker.(*vndkPrebuiltLibraryDecorator); ok {
		// Make vendor variants only for the versions in BOARD_VNDK_VERSION and
		// PRODUCT_EXTRA_VNDK_VERSIONS.
		vendorVariantNeeded = true
	} else if m.hasVendorVariant() && !vendorSpecific {
		// This will be available in both /system and /vendor
		// or a /system directory that is available to vendor.
		coreVariantNeeded = true
		vendorVariantNeeded = true
	} else if vendorSpecific && String(m.Properties.Sdk_version) == "" {
		// This will be available in /vendor (or /odm) only
		vendorVariantNeeded = true
	} else {
		// This is either in /system (or similar: /data), or is a
		// modules built with the NDK. Modules built with the NDK
		// will be restricted using the existing link type checks.
		coreVariantNeeded = true
	}

	if Bool(m.Properties.Recovery_available) {
		recoveryVariantNeeded = true
	}

	if m.ModuleBase.InstallInRecovery() {
		recoveryVariantNeeded = true
		coreVariantNeeded = false
	}

	if recoveryVariantNeeded {
		primaryArch := mctx.Config().DevicePrimaryArchType()
		moduleArch := m.Target().Arch.ArchType
		if moduleArch != primaryArch {
			recoveryVariantNeeded = false
		}
	}

	var variants []string
	if coreVariantNeeded {
		variants = append(variants, coreMode)
	}
	if vendorVariantNeeded {
		variants = append(variants, vendorMode)
	}
	if recoveryVariantNeeded {
		variants = append(variants, recoveryMode)
	}
	mod := mctx.CreateVariations(variants...)
	for i, v := range variants {
		if v == vendorMode {
			m := mod[i].(*Module)
			m.Properties.UseVndk = true
			squashVendorSrcs(m)
		} else if v == recoveryMode {
			m := mod[i].(*Module)
			m.Properties.InRecovery = true
			m.MakeAsPlatform()
			squashRecoverySrcs(m)
		}
	}
}

func getCurrentNdkPrebuiltVersion(ctx DepsContext) string {
	if ctx.Config().PlatformSdkVersionInt() > config.NdkMaxPrebuiltVersionInt {
		return strconv.Itoa(config.NdkMaxPrebuiltVersionInt)
	}
	return ctx.Config().PlatformSdkVersion()
}

var Bool = proptools.Bool
var BoolDefault = proptools.BoolDefault
var BoolPtr = proptools.BoolPtr
var String = proptools.String
var StringPtr = proptools.StringPtr
