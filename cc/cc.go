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
	"io"
	"strconv"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc/config"
	"android/soong/genrule"
)

func init() {
	RegisterCCBuildComponents(android.InitRegistrationContext)

	pctx.Import("android/soong/cc/config")
}

func RegisterCCBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("cc_defaults", defaultsFactory)

	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("sdk", sdkMutator).Parallel()
		ctx.BottomUp("vndk", VndkMutator).Parallel()
		ctx.BottomUp("link", LinkageMutator).Parallel()
		ctx.BottomUp("ndk_api", NdkApiMutator).Parallel()
		ctx.BottomUp("test_per_src", TestPerSrcMutator).Parallel()
		ctx.BottomUp("version", VersionMutator).Parallel()
		ctx.BottomUp("begin", BeginMutator).Parallel()
		ctx.BottomUp("sysprop_cc", SyspropMutator).Parallel()
		ctx.BottomUp("vendor_snapshot", VendorSnapshotMutator).Parallel()
		ctx.BottomUp("vendor_snapshot_source", VendorSnapshotSourceMutator).Parallel()
	})

	ctx.PostDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("asan_deps", sanitizerDepsMutator(asan))
		ctx.BottomUp("asan", sanitizerMutator(asan)).Parallel()

		ctx.TopDown("hwasan_deps", sanitizerDepsMutator(hwasan))
		ctx.BottomUp("hwasan", sanitizerMutator(hwasan)).Parallel()

		ctx.TopDown("fuzzer_deps", sanitizerDepsMutator(fuzzer))
		ctx.BottomUp("fuzzer", sanitizerMutator(fuzzer)).Parallel()

		// cfi mutator shouldn't run before sanitizers that return true for
		// incompatibleWithCfi()
		ctx.TopDown("cfi_deps", sanitizerDepsMutator(cfi))
		ctx.BottomUp("cfi", sanitizerMutator(cfi)).Parallel()

		ctx.TopDown("scs_deps", sanitizerDepsMutator(scs))
		ctx.BottomUp("scs", sanitizerMutator(scs)).Parallel()

		ctx.TopDown("tsan_deps", sanitizerDepsMutator(tsan))
		ctx.BottomUp("tsan", sanitizerMutator(tsan)).Parallel()

		ctx.TopDown("sanitize_runtime_deps", sanitizerRuntimeDepsMutator).Parallel()
		ctx.BottomUp("sanitize_runtime", sanitizerRuntimeMutator).Parallel()

		ctx.BottomUp("coverage", coverageMutator).Parallel()
		ctx.TopDown("vndk_deps", sabiDepsMutator)

		ctx.TopDown("lto_deps", ltoDepsMutator)
		ctx.BottomUp("lto", ltoMutator).Parallel()

		ctx.TopDown("double_loadable", checkDoubleLoadableLibraries).Parallel()
	})

	android.RegisterSingletonType("kythe_extract_all", kytheExtractAllFactory)
}

type Deps struct {
	SharedLibs, LateSharedLibs                  []string
	StaticLibs, LateStaticLibs, WholeStaticLibs []string
	HeaderLibs                                  []string
	RuntimeLibs                                 []string

	StaticUnwinderIfLegacy bool

	ReexportSharedLibHeaders, ReexportStaticLibHeaders, ReexportHeaderLibHeaders []string

	ObjFiles []string

	GeneratedSources []string
	GeneratedHeaders []string
	GeneratedDeps    []string

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
	GeneratedDeps    android.Paths

	Flags                      []string
	IncludeDirs                android.Paths
	SystemIncludeDirs          android.Paths
	ReexportedDirs             android.Paths
	ReexportedSystemDirs       android.Paths
	ReexportedFlags            []string
	ReexportedGeneratedHeaders android.Paths
	ReexportedDeps             android.Paths

	// Paths to crt*.o files
	CrtBegin, CrtEnd android.OptionalPath

	// Path to the file container flags to use with the linker
	LinkerFlagsFile android.OptionalPath

	// Path to the dynamic linker binary
	DynamicLinker android.OptionalPath
}

// LocalOrGlobalFlags contains flags that need to have values set globally by the build system or locally by the module
// tracked separately, in order to maintain the required ordering (most of the global flags need to go first on the
// command line so they can be overridden by the local module flags).
type LocalOrGlobalFlags struct {
	CommonFlags     []string // Flags that apply to C, C++, and assembly source files
	AsFlags         []string // Flags that apply to assembly source files
	YasmFlags       []string // Flags that apply to yasm assembly source files
	CFlags          []string // Flags that apply to C and C++ source files
	ToolingCFlags   []string // Flags that apply to C and C++ source files parsed by clang LibTooling tools
	ConlyFlags      []string // Flags that apply to C source files
	CppFlags        []string // Flags that apply to C++ source files
	ToolingCppFlags []string // Flags that apply to C++ source files parsed by clang LibTooling tools
	LdFlags         []string // Flags that apply to linker command lines
}

type Flags struct {
	Local  LocalOrGlobalFlags
	Global LocalOrGlobalFlags

	aidlFlags     []string // Flags that apply to aidl source files
	rsFlags       []string // Flags that apply to renderscript source files
	libFlags      []string // Flags to add libraries early to the link order
	extraLibFlags []string // Flags to add libraries late in the link order after LdFlags
	TidyFlags     []string // Flags that apply to clang-tidy
	SAbiFlags     []string // Flags that apply to header-abi-dumper

	// Global include flags that apply to C, C++, and assembly source files
	// These must be after any module include flags, which will be in CommonFlags.
	SystemIncludeFlags []string

	Toolchain    config.Toolchain
	Tidy         bool
	GcovCoverage bool
	SAbiDump     bool
	EmitXrefs    bool // If true, generate Ninja rules to generate emitXrefs input files for Kythe

	RequiredInstructionSet string
	DynamicLinker          string

	CFlagsDeps  android.Paths // Files depended on by compiler flags
	LdFlagsDeps android.Paths // Files depended on by linker flags

	AssemblerWithCpp bool
	GroupStaticLibs  bool

	proto            android.ProtoFlags
	protoC           bool // Whether to use C instead of C++
	protoOptionsFile bool // Whether to look for a .options file next to the .proto

	Yacc *YaccProperties
}

// Properties used to compile all C or C++ modules
type BaseProperties struct {
	// Deprecated. true is the default, false is invalid.
	Clang *bool `android:"arch_variant"`

	// Minimum sdk version supported when compiling against the ndk. Setting this property causes
	// two variants to be built, one for the platform and one for apps.
	Sdk_version *string

	// Minimum sdk version that the artifact should support when it runs as part of mainline modules(APEX).
	Min_sdk_version *string

	// If true, always create an sdk variant and don't create a platform variant.
	Sdk_variant_only *bool

	AndroidMkSharedLibs       []string `blueprint:"mutated"`
	AndroidMkStaticLibs       []string `blueprint:"mutated"`
	AndroidMkRuntimeLibs      []string `blueprint:"mutated"`
	AndroidMkWholeStaticLibs  []string `blueprint:"mutated"`
	HideFromMake              bool     `blueprint:"mutated"`
	PreventInstall            bool     `blueprint:"mutated"`
	ApexesProvidingSharedLibs []string `blueprint:"mutated"`

	ImageVariationPrefix string `blueprint:"mutated"`
	VndkVersion          string `blueprint:"mutated"`
	SubName              string `blueprint:"mutated"`

	// *.logtags files, to combine together in order to generate the /system/etc/event-log-tags
	// file
	Logtags []string

	// Make this module available when building for ramdisk
	Ramdisk_available *bool

	// Make this module available when building for recovery
	Recovery_available *bool

	// Set by imageMutator
	CoreVariantNeeded     bool     `blueprint:"mutated"`
	RamdiskVariantNeeded  bool     `blueprint:"mutated"`
	RecoveryVariantNeeded bool     `blueprint:"mutated"`
	ExtraVariants         []string `blueprint:"mutated"`

	// Allows this module to use non-APEX version of libraries. Useful
	// for building binaries that are started before APEXes are activated.
	Bootstrap *bool

	// Even if DeviceConfig().VndkUseCoreVariant() is set, this module must use vendor variant.
	// see soong/cc/config/vndk.go
	MustUseVendorVariant bool `blueprint:"mutated"`

	// Used by vendor snapshot to record dependencies from snapshot modules.
	SnapshotSharedLibs  []string `blueprint:"mutated"`
	SnapshotRuntimeLibs []string `blueprint:"mutated"`

	Installable *bool

	// Set by factories of module types that can only be referenced from variants compiled against
	// the SDK.
	AlwaysSdk bool `blueprint:"mutated"`

	// Variant is an SDK variant created by sdkMutator
	IsSdkVariant bool `blueprint:"mutated"`
	// Set when both SDK and platform variants are exported to Make to trigger renaming the SDK
	// variant to have a ".sdk" suffix.
	SdkAndPlatformVariantVisibleToMake bool `blueprint:"mutated"`
}

type VendorProperties struct {
	// whether this module should be allowed to be directly depended by other
	// modules with `vendor: true`, `proprietary: true`, or `vendor_available:true`.
	// In addition, this module should be allowed to be directly depended by
	// product modules with `product_specific: true`.
	// If set to true, three variants will be built separately, one like
	// normal, another limited to the set of libraries and headers
	// that are exposed to /vendor modules, and the other to /product modules.
	//
	// The vendor and product variants may be used with a different (newer) /system,
	// so it shouldn't have any unversioned runtime dependencies, or
	// make assumptions about the system that may not be true in the
	// future.
	//
	// If set to false, this module becomes inaccessible from /vendor or /product
	// modules.
	//
	// Default value is true when vndk: {enabled: true} or vendor: true.
	//
	// Nothing happens if BOARD_VNDK_VERSION isn't set in the BoardConfig.mk
	// If PRODUCT_PRODUCT_VNDK_VERSION isn't set, product variant will not be used.
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
	binary() bool
	object() bool
	toolchain() config.Toolchain
	canUseSdk() bool
	useSdk() bool
	sdkVersion() string
	useVndk() bool
	isNdk() bool
	isLlndk(config android.Config) bool
	isLlndkPublic(config android.Config) bool
	isVndkPrivate(config android.Config) bool
	isVndk() bool
	isVndkSp() bool
	isVndkExt() bool
	inProduct() bool
	inVendor() bool
	inRamdisk() bool
	inRecovery() bool
	shouldCreateSourceAbiDump() bool
	selectedStl() string
	baseModuleName() string
	getVndkExtendsModuleName() string
	isPgoCompile() bool
	isNDKStubLibrary() bool
	useClangLld(actx ModuleContext) bool
	isForPlatform() bool
	apexVariationName() string
	apexSdkVersion() int
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
	android.BaseModuleContext
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

	// Get the deps that have been explicitly specified in the properties.
	// Only updates the
	linkerSpecifiedDeps(specifiedDeps specifiedDeps) specifiedDeps
}

type specifiedDeps struct {
	sharedLibs       []string
	systemSharedLibs []string // Note nil and [] are semantically distinct.
}

type installer interface {
	installerProps() []interface{}
	install(ctx ModuleContext, path android.Path)
	everInstallable() bool
	inData() bool
	inSanitizerDir() bool
	hostToolPath() android.OptionalPath
	relativeInstallPath() string
	skipInstall(mod *Module)
}

type xref interface {
	XrefCcFiles() android.Paths
}

var (
	sharedExportDepTag    = DependencyTag{Name: "shared", Library: true, Shared: true, ReexportFlags: true}
	earlySharedDepTag     = DependencyTag{Name: "early_shared", Library: true, Shared: true}
	lateSharedDepTag      = DependencyTag{Name: "late shared", Library: true, Shared: true}
	staticExportDepTag    = DependencyTag{Name: "static", Library: true, ReexportFlags: true}
	lateStaticDepTag      = DependencyTag{Name: "late static", Library: true}
	staticUnwinderDepTag  = DependencyTag{Name: "static unwinder", Library: true}
	wholeStaticDepTag     = DependencyTag{Name: "whole static", Library: true, ReexportFlags: true}
	headerDepTag          = DependencyTag{Name: "header", Library: true}
	headerExportDepTag    = DependencyTag{Name: "header", Library: true, ReexportFlags: true}
	genSourceDepTag       = DependencyTag{Name: "gen source"}
	genHeaderDepTag       = DependencyTag{Name: "gen header"}
	genHeaderExportDepTag = DependencyTag{Name: "gen header", ReexportFlags: true}
	objDepTag             = DependencyTag{Name: "obj"}
	linkerFlagsDepTag     = DependencyTag{Name: "linker flags file"}
	dynamicLinkerDepTag   = DependencyTag{Name: "dynamic linker"}
	reuseObjTag           = DependencyTag{Name: "reuse objects"}
	staticVariantTag      = DependencyTag{Name: "static variant"}
	ndkStubDepTag         = DependencyTag{Name: "ndk stub", Library: true}
	ndkLateStubDepTag     = DependencyTag{Name: "ndk late stub", Library: true}
	vndkExtDepTag         = DependencyTag{Name: "vndk extends", Library: true}
	runtimeDepTag         = DependencyTag{Name: "runtime lib"}
	coverageDepTag        = DependencyTag{Name: "coverage"}
	testPerSrcDepTag      = DependencyTag{Name: "test_per_src"}
)

func IsSharedDepTag(depTag blueprint.DependencyTag) bool {
	ccDepTag, ok := depTag.(DependencyTag)
	return ok && ccDepTag.Shared
}

func IsRuntimeDepTag(depTag blueprint.DependencyTag) bool {
	ccDepTag, ok := depTag.(DependencyTag)
	return ok && ccDepTag == runtimeDepTag
}

func IsTestPerSrcDepTag(depTag blueprint.DependencyTag) bool {
	ccDepTag, ok := depTag.(DependencyTag)
	return ok && ccDepTag == testPerSrcDepTag
}

// Module contains the properties and members used by all C/C++ module types, and implements
// the blueprint.Module interface.  It delegates to compiler, linker, and installer interfaces
// to construct the output file.  Behavior can be customized with a Customizer interface
type Module struct {
	android.ModuleBase
	android.DefaultableModuleBase
	android.ApexModuleBase
	android.SdkBase

	Properties       BaseProperties
	VendorProperties VendorProperties

	// initialize before calling Init
	hod      android.HostOrDeviceSupported
	multilib android.Multilib

	// Allowable SdkMemberTypes of this module type.
	sdkMemberTypes []android.SdkMemberType

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
	staticVariant LinkableInterface

	makeLinkType string
	// Kythe (source file indexer) paths for this compilation module
	kytheFiles android.Paths

	// For apex variants, this is set as apex.min_sdk_version
	apexSdkVersion int
}

func (c *Module) Toc() android.OptionalPath {
	if c.linker != nil {
		if library, ok := c.linker.(libraryInterface); ok {
			return library.toc()
		}
	}
	panic(fmt.Errorf("Toc() called on non-library module: %q", c.BaseModuleName()))
}

func (c *Module) ApiLevel() string {
	if c.linker != nil {
		if stub, ok := c.linker.(*stubDecorator); ok {
			return stub.properties.ApiLevel
		}
	}
	panic(fmt.Errorf("ApiLevel() called on non-stub library module: %q", c.BaseModuleName()))
}

func (c *Module) Static() bool {
	if c.linker != nil {
		if library, ok := c.linker.(libraryInterface); ok {
			return library.static()
		}
	}
	panic(fmt.Errorf("Static() called on non-library module: %q", c.BaseModuleName()))
}

func (c *Module) Shared() bool {
	if c.linker != nil {
		if library, ok := c.linker.(libraryInterface); ok {
			return library.shared()
		}
	}
	panic(fmt.Errorf("Shared() called on non-library module: %q", c.BaseModuleName()))
}

func (c *Module) SelectedStl() string {
	if c.stl != nil {
		return c.stl.Properties.SelectedStl
	}
	return ""
}

func (c *Module) ToolchainLibrary() bool {
	if _, ok := c.linker.(*toolchainLibraryDecorator); ok {
		return true
	}
	return false
}

func (c *Module) NdkPrebuiltStl() bool {
	if _, ok := c.linker.(*ndkPrebuiltStlLinker); ok {
		return true
	}
	return false
}

func (c *Module) StubDecorator() bool {
	if _, ok := c.linker.(*stubDecorator); ok {
		return true
	}
	return false
}

func (c *Module) SdkVersion() string {
	return String(c.Properties.Sdk_version)
}

func (c *Module) MinSdkVersion() string {
	return String(c.Properties.Min_sdk_version)
}

func (c *Module) AlwaysSdk() bool {
	return c.Properties.AlwaysSdk || Bool(c.Properties.Sdk_variant_only)
}

func (c *Module) IncludeDirs() android.Paths {
	if c.linker != nil {
		if library, ok := c.linker.(exportedFlagsProducer); ok {
			return library.exportedDirs()
		}
	}
	panic(fmt.Errorf("IncludeDirs called on non-exportedFlagsProducer module: %q", c.BaseModuleName()))
}

func (c *Module) HasStaticVariant() bool {
	if c.staticVariant != nil {
		return true
	}
	return false
}

func (c *Module) GetStaticVariant() LinkableInterface {
	return c.staticVariant
}

func (c *Module) SetDepsInLinkOrder(depsInLinkOrder []android.Path) {
	c.depsInLinkOrder = depsInLinkOrder
}

func (c *Module) GetDepsInLinkOrder() []android.Path {
	return c.depsInLinkOrder
}

func (c *Module) StubsVersions() []string {
	if c.linker != nil {
		if library, ok := c.linker.(*libraryDecorator); ok {
			return library.Properties.Stubs.Versions
		}
	}
	panic(fmt.Errorf("StubsVersions called on non-library module: %q", c.BaseModuleName()))
}

func (c *Module) CcLibrary() bool {
	if c.linker != nil {
		if _, ok := c.linker.(*libraryDecorator); ok {
			return true
		}
	}
	return false
}

func (c *Module) CcLibraryInterface() bool {
	if _, ok := c.linker.(libraryInterface); ok {
		return true
	}
	return false
}

func (c *Module) NonCcVariants() bool {
	return false
}

func (c *Module) SetBuildStubs() {
	if c.linker != nil {
		if library, ok := c.linker.(*libraryDecorator); ok {
			library.MutatedProperties.BuildStubs = true
			c.Properties.HideFromMake = true
			c.sanitize = nil
			c.stl = nil
			c.Properties.PreventInstall = true
			return
		}
		if _, ok := c.linker.(*llndkStubDecorator); ok {
			c.Properties.HideFromMake = true
			return
		}
	}
	panic(fmt.Errorf("SetBuildStubs called on non-library module: %q", c.BaseModuleName()))
}

func (c *Module) BuildStubs() bool {
	if c.linker != nil {
		if library, ok := c.linker.(*libraryDecorator); ok {
			return library.buildStubs()
		}
	}
	panic(fmt.Errorf("BuildStubs called on non-library module: %q", c.BaseModuleName()))
}

func (c *Module) SetStubsVersions(version string) {
	if c.linker != nil {
		if library, ok := c.linker.(*libraryDecorator); ok {
			library.MutatedProperties.StubsVersion = version
			return
		}
		if llndk, ok := c.linker.(*llndkStubDecorator); ok {
			llndk.libraryDecorator.MutatedProperties.StubsVersion = version
			return
		}
	}
	panic(fmt.Errorf("SetStubsVersions called on non-library module: %q", c.BaseModuleName()))
}

func (c *Module) StubsVersion() string {
	if c.linker != nil {
		if library, ok := c.linker.(*libraryDecorator); ok {
			return library.MutatedProperties.StubsVersion
		}
		if llndk, ok := c.linker.(*llndkStubDecorator); ok {
			return llndk.libraryDecorator.MutatedProperties.StubsVersion
		}
	}
	panic(fmt.Errorf("StubsVersion called on non-library module: %q", c.BaseModuleName()))
}

func (c *Module) SetStatic() {
	if c.linker != nil {
		if library, ok := c.linker.(libraryInterface); ok {
			library.setStatic()
			return
		}
	}
	panic(fmt.Errorf("SetStatic called on non-library module: %q", c.BaseModuleName()))
}

func (c *Module) SetShared() {
	if c.linker != nil {
		if library, ok := c.linker.(libraryInterface); ok {
			library.setShared()
			return
		}
	}
	panic(fmt.Errorf("SetShared called on non-library module: %q", c.BaseModuleName()))
}

func (c *Module) BuildStaticVariant() bool {
	if c.linker != nil {
		if library, ok := c.linker.(libraryInterface); ok {
			return library.buildStatic()
		}
	}
	panic(fmt.Errorf("BuildStaticVariant called on non-library module: %q", c.BaseModuleName()))
}

func (c *Module) BuildSharedVariant() bool {
	if c.linker != nil {
		if library, ok := c.linker.(libraryInterface); ok {
			return library.buildShared()
		}
	}
	panic(fmt.Errorf("BuildSharedVariant called on non-library module: %q", c.BaseModuleName()))
}

func (c *Module) Module() android.Module {
	return c
}

func (c *Module) OutputFile() android.OptionalPath {
	return c.outputFile
}

var _ LinkableInterface = (*Module)(nil)

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

func (c *Module) VndkVersion() string {
	return c.Properties.VndkVersion
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
	android.InitApexModule(c)
	android.InitSdkAwareModule(c)
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

// Returns true if the module is using VNDK libraries instead of the libraries in /system/lib or /system/lib64.
// "product" and "vendor" variant modules return true for this function.
// When BOARD_VNDK_VERSION is set, vendor variants of "vendor_available: true", "vendor: true",
// "soc_specific: true" and more vendor installed modules are included here.
// When PRODUCT_PRODUCT_VNDK_VERSION is set, product variants of "vendor_available: true" or
// "product_specific: true" modules are included here.
func (c *Module) UseVndk() bool {
	return c.Properties.VndkVersion != ""
}

func (c *Module) canUseSdk() bool {
	return c.Os() == android.Android && !c.UseVndk() && !c.InRamdisk() && !c.InRecovery()
}

func (c *Module) UseSdk() bool {
	if c.canUseSdk() {
		return String(c.Properties.Sdk_version) != ""
	}
	return false
}

func (c *Module) isCoverageVariant() bool {
	return c.coverage.Properties.IsCoverageVariant
}

func (c *Module) IsNdk() bool {
	return inList(c.Name(), ndkMigratedLibs)
}

func (c *Module) isLlndk(config android.Config) bool {
	// Returns true for both LLNDK (public) and LLNDK-private libs.
	return isLlndkLibrary(c.BaseModuleName(), config)
}

func (c *Module) isLlndkPublic(config android.Config) bool {
	// Returns true only for LLNDK (public) libs.
	name := c.BaseModuleName()
	return isLlndkLibrary(name, config) && !isVndkPrivateLibrary(name, config)
}

func (c *Module) isVndkPrivate(config android.Config) bool {
	// Returns true for LLNDK-private, VNDK-SP-private, and VNDK-core-private.
	return isVndkPrivateLibrary(c.BaseModuleName(), config)
}

func (c *Module) IsVndk() bool {
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

func (c *Module) MustUseVendorVariant() bool {
	return c.isVndkSp() || c.Properties.MustUseVendorVariant
}

func (c *Module) getVndkExtendsModuleName() string {
	if vndkdep := c.vndkdep; vndkdep != nil {
		return vndkdep.getVndkExtendsModuleName()
	}
	return ""
}

// Returns true only when this module is configured to have core, product and vendor
// variants.
func (c *Module) HasVendorVariant() bool {
	return c.IsVndk() || Bool(c.VendorProperties.Vendor_available)
}

const (
	// VendorVariationPrefix is the variant prefix used for /vendor code that compiles
	// against the VNDK.
	VendorVariationPrefix = "vendor."

	// ProductVariationPrefix is the variant prefix used for /product code that compiles
	// against the VNDK.
	ProductVariationPrefix = "product."
)

// Returns true if the module is "product" variant. Usually these modules are installed in /product
func (c *Module) inProduct() bool {
	return c.Properties.ImageVariationPrefix == ProductVariationPrefix
}

// Returns true if the module is "vendor" variant. Usually these modules are installed in /vendor
func (c *Module) inVendor() bool {
	return c.Properties.ImageVariationPrefix == VendorVariationPrefix
}

func (c *Module) InRamdisk() bool {
	return c.ModuleBase.InRamdisk() || c.ModuleBase.InstallInRamdisk()
}

func (c *Module) InRecovery() bool {
	return c.ModuleBase.InRecovery() || c.ModuleBase.InstallInRecovery()
}

func (c *Module) OnlyInRamdisk() bool {
	return c.ModuleBase.InstallInRamdisk()
}

func (c *Module) OnlyInRecovery() bool {
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
	// Bug: http://b/137883967 - native-bridge modules do not currently work with coverage
	if c.Target().NativeBridge == android.NativeBridgeEnabled {
		return false
	}
	return c.linker != nil && c.linker.nativeCoverage()
}

func (c *Module) isSnapshotPrebuilt() bool {
	if p, ok := c.linker.(interface{ isSnapshotPrebuilt() bool }); ok {
		return p.isSnapshotPrebuilt()
	}
	return false
}

func (c *Module) ExportedIncludeDirs() android.Paths {
	if flagsProducer, ok := c.linker.(exportedFlagsProducer); ok {
		return flagsProducer.exportedDirs()
	}
	return nil
}

func (c *Module) ExportedSystemIncludeDirs() android.Paths {
	if flagsProducer, ok := c.linker.(exportedFlagsProducer); ok {
		return flagsProducer.exportedSystemDirs()
	}
	return nil
}

func (c *Module) ExportedFlags() []string {
	if flagsProducer, ok := c.linker.(exportedFlagsProducer); ok {
		return flagsProducer.exportedFlags()
	}
	return nil
}

func (c *Module) ExportedDeps() android.Paths {
	if flagsProducer, ok := c.linker.(exportedFlagsProducer); ok {
		return flagsProducer.exportedDeps()
	}
	return nil
}

func (c *Module) ExportedGeneratedHeaders() android.Paths {
	if flagsProducer, ok := c.linker.(exportedFlagsProducer); ok {
		return flagsProducer.exportedGeneratedHeaders()
	}
	return nil
}

func isBionic(name string) bool {
	switch name {
	case "libc", "libm", "libdl", "libdl_android", "linker":
		return true
	}
	return false
}

func InstallToBootstrap(name string, config android.Config) bool {
	if name == "libclang_rt.hwasan-aarch64-android" {
		return inList("hwaddress", config.SanitizeDevice())
	}
	return isBionic(name)
}

func (c *Module) XrefCcFiles() android.Paths {
	return c.kytheFiles
}

type baseModuleContext struct {
	android.BaseModuleContext
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

func (ctx *moduleContext) ProductSpecific() bool {
	return ctx.ModuleContext.ProductSpecific() ||
		(ctx.mod.HasVendorVariant() && ctx.mod.inProduct() && !ctx.mod.IsVndk())
}

func (ctx *moduleContext) SocSpecific() bool {
	return ctx.ModuleContext.SocSpecific() ||
		(ctx.mod.HasVendorVariant() && ctx.mod.inVendor() && !ctx.mod.IsVndk())
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

func (ctx *moduleContextImpl) binary() bool {
	return ctx.mod.binary()
}

func (ctx *moduleContextImpl) object() bool {
	return ctx.mod.object()
}

func (ctx *moduleContextImpl) canUseSdk() bool {
	return ctx.mod.canUseSdk()
}

func (ctx *moduleContextImpl) useSdk() bool {
	return ctx.mod.UseSdk()
}

func (ctx *moduleContextImpl) sdkVersion() string {
	if ctx.ctx.Device() {
		if ctx.useVndk() {
			vndkVer := ctx.mod.VndkVersion()
			if inList(vndkVer, ctx.ctx.Config().PlatformVersionActiveCodenames()) {
				return "current"
			}
			return vndkVer
		}
		return String(ctx.mod.Properties.Sdk_version)
	}
	return ""
}

func (ctx *moduleContextImpl) useVndk() bool {
	return ctx.mod.UseVndk()
}

func (ctx *moduleContextImpl) isNdk() bool {
	return ctx.mod.IsNdk()
}

func (ctx *moduleContextImpl) isLlndk(config android.Config) bool {
	return ctx.mod.isLlndk(config)
}

func (ctx *moduleContextImpl) isLlndkPublic(config android.Config) bool {
	return ctx.mod.isLlndkPublic(config)
}

func (ctx *moduleContextImpl) isVndkPrivate(config android.Config) bool {
	return ctx.mod.isVndkPrivate(config)
}

func (ctx *moduleContextImpl) isVndk() bool {
	return ctx.mod.IsVndk()
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
	return ctx.mod.MustUseVendorVariant()
}

func (ctx *moduleContextImpl) inProduct() bool {
	return ctx.mod.inProduct()
}

func (ctx *moduleContextImpl) inVendor() bool {
	return ctx.mod.inVendor()
}

func (ctx *moduleContextImpl) inRamdisk() bool {
	return ctx.mod.InRamdisk()
}

func (ctx *moduleContextImpl) inRecovery() bool {
	return ctx.mod.InRecovery()
}

// Check whether ABI dumps should be created for this module.
func (ctx *moduleContextImpl) shouldCreateSourceAbiDump() bool {
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
	if ctx.isStubs() || ctx.isNDKStubLibrary() {
		// Stubs do not need ABI dumps.
		return false
	}
	return true
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

func (ctx *moduleContextImpl) isForPlatform() bool {
	return ctx.mod.IsForPlatform()
}

func (ctx *moduleContextImpl) apexVariationName() string {
	return ctx.mod.ApexVariationName()
}

func (ctx *moduleContextImpl) apexSdkVersion() int {
	return ctx.mod.apexSdkVersion
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

func orderStaticModuleDeps(module LinkableInterface, staticDeps []LinkableInterface, sharedDeps []LinkableInterface) (results []android.Path) {
	// convert Module to Path
	var depsInLinkOrder []android.Path
	allTransitiveDeps := make(map[android.Path][]android.Path, len(staticDeps))
	staticDepFiles := []android.Path{}
	for _, dep := range staticDeps {
		// The OutputFile may not be valid for a variant not present, and the AllowMissingDependencies flag is set.
		if dep.OutputFile().Valid() {
			allTransitiveDeps[dep.OutputFile().Path()] = dep.GetDepsInLinkOrder()
			staticDepFiles = append(staticDepFiles, dep.OutputFile().Path())
		}
	}
	sharedDepFiles := []android.Path{}
	for _, sharedDep := range sharedDeps {
		if sharedDep.HasStaticVariant() {
			staticAnalogue := sharedDep.GetStaticVariant()
			allTransitiveDeps[staticAnalogue.OutputFile().Path()] = staticAnalogue.GetDepsInLinkOrder()
			sharedDepFiles = append(sharedDepFiles, staticAnalogue.OutputFile().Path())
		}
	}

	// reorder the dependencies based on transitive dependencies
	depsInLinkOrder, results = orderDeps(staticDepFiles, sharedDepFiles, allTransitiveDeps)
	module.SetDepsInLinkOrder(depsInLinkOrder)

	return results
}

func (c *Module) IsTestPerSrcAllTestsVariation() bool {
	test, ok := c.linker.(testPerSrc)
	return ok && test.isAllTestsVariation()
}

func (c *Module) getNameSuffixWithVndkVersion(ctx android.ModuleContext) string {
	// Returns the name suffix for product and vendor variants. If the VNDK version is not
	// "current", it will append the VNDK version to the name suffix.
	var vndkVersion string
	var nameSuffix string
	if c.inProduct() {
		vndkVersion = ctx.DeviceConfig().ProductVndkVersion()
		nameSuffix = productSuffix
	} else {
		vndkVersion = ctx.DeviceConfig().VndkVersion()
		nameSuffix = vendorSuffix
	}
	if vndkVersion == "current" {
		vndkVersion = ctx.DeviceConfig().PlatformVndkVersion()
	}
	if c.Properties.VndkVersion != vndkVersion {
		// add version suffix only if the module is using different vndk version than the
		// version in product or vendor partition.
		nameSuffix += "." + c.Properties.VndkVersion
	}
	return nameSuffix
}

func (c *Module) GenerateAndroidBuildActions(actx android.ModuleContext) {
	// Handle the case of a test module split by `test_per_src` mutator.
	//
	// The `test_per_src` mutator adds an extra variation named "", depending on all the other
	// `test_per_src` variations of the test module. Set `outputFile` to an empty path for this
	// module and return early, as this module does not produce an output file per se.
	if c.IsTestPerSrcAllTestsVariation() {
		c.outputFile = android.OptionalPath{}
		return
	}

	c.makeLinkType = c.getMakeLinkType(actx)

	c.Properties.SubName = ""

	if c.Target().NativeBridge == android.NativeBridgeEnabled {
		c.Properties.SubName += nativeBridgeSuffix
	}

	_, llndk := c.linker.(*llndkStubDecorator)
	_, llndkHeader := c.linker.(*llndkHeadersDecorator)
	if llndk || llndkHeader || (c.UseVndk() && c.HasVendorVariant()) {
		// .vendor.{version} suffix is added for vendor variant or .product.{version} suffix is
		// added for product variant only when we have vendor and product variants with core
		// variant. The suffix is not added for vendor-only or product-only module.
		c.Properties.SubName += c.getNameSuffixWithVndkVersion(actx)
	} else if _, ok := c.linker.(*vndkPrebuiltLibraryDecorator); ok {
		// .vendor suffix is added for backward compatibility with VNDK snapshot whose names with
		// such suffixes are already hard-coded in prebuilts/vndk/.../Android.bp.
		c.Properties.SubName += vendorSuffix
	} else if c.InRamdisk() && !c.OnlyInRamdisk() {
		c.Properties.SubName += ramdiskSuffix
	} else if c.InRecovery() && !c.OnlyInRecovery() {
		c.Properties.SubName += recoverySuffix
	} else if c.Properties.IsSdkVariant && c.Properties.SdkAndPlatformVariantVisibleToMake {
		c.Properties.SubName += sdkSuffix
	}

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
		EmitXrefs: ctx.Config().EmitXrefRules(),
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
	for _, feature := range c.features {
		flags = feature.flags(ctx, flags)
	}
	if ctx.Failed() {
		return
	}

	flags.Local.CFlags, _ = filterList(flags.Local.CFlags, config.IllegalFlags)
	flags.Local.CppFlags, _ = filterList(flags.Local.CppFlags, config.IllegalFlags)
	flags.Local.ConlyFlags, _ = filterList(flags.Local.ConlyFlags, config.IllegalFlags)

	flags.Local.CommonFlags = append(flags.Local.CommonFlags, deps.Flags...)

	for _, dir := range deps.IncludeDirs {
		flags.Local.CommonFlags = append(flags.Local.CommonFlags, "-I"+dir.String())
	}
	for _, dir := range deps.SystemIncludeDirs {
		flags.Local.CommonFlags = append(flags.Local.CommonFlags, "-isystem "+dir.String())
	}

	c.flags = flags
	// We need access to all the flags seen by a source file.
	if c.sabi != nil {
		flags = c.sabi.flags(ctx, flags)
	}

	flags.AssemblerWithCpp = inList("-xassembler-with-cpp", flags.Local.AsFlags)

	// Optimization to reduce size of build.ninja
	// Replace the long list of flags for each file with a module-local variable
	ctx.Variable(pctx, "cflags", strings.Join(flags.Local.CFlags, " "))
	ctx.Variable(pctx, "cppflags", strings.Join(flags.Local.CppFlags, " "))
	ctx.Variable(pctx, "asflags", strings.Join(flags.Local.AsFlags, " "))
	flags.Local.CFlags = []string{"$cflags"}
	flags.Local.CppFlags = []string{"$cppflags"}
	flags.Local.AsFlags = []string{"$asflags"}

	var objs Objects
	if c.compiler != nil {
		objs = c.compiler.compile(ctx, flags, deps)
		if ctx.Failed() {
			return
		}
		c.kytheFiles = objs.kytheFiles
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
			android.DirectlyInAnyApex(ctx, ctx.baseModuleName()) && !c.InRamdisk() &&
			!c.InRecovery() && !c.UseVndk() && !c.static() && !c.isCoverageVariant() &&
			c.IsStubs() {
			c.Properties.HideFromMake = false // unhide
			// Note: this is still non-installable
		}

		// glob exported headers for snapshot, if BOARD_VNDK_VERSION is current.
		if i, ok := c.linker.(snapshotLibraryInterface); ok && ctx.DeviceConfig().VndkVersion() == "current" {
			if isSnapshotAware(ctx, c) {
				i.collectHeadersForSnapshot(ctx)
			}
		}
	}

	if c.installable() {
		c.installer.install(ctx, c.outputFile.Path())
		if ctx.Failed() {
			return
		}
	} else if !proptools.BoolDefault(c.Properties.Installable, true) {
		// If the module has been specifically configure to not be installed then
		// skip the installation as otherwise it will break when running inside make
		// as the output path to install will not be specified. Not all uninstallable
		// modules can skip installation as some are needed for resolving make side
		// dependencies.
		c.SkipInstall()
	}
}

func (c *Module) toolchain(ctx android.BaseModuleContext) config.Toolchain {
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
		BaseModuleContext: actx,
		moduleContextImpl: moduleContextImpl{
			mod: c,
		},
	}
	ctx.ctx = ctx

	c.begin(ctx)
}

// Split name#version into name and version
func StubsLibNameAndVersion(name string) (string, string) {
	if sharp := strings.LastIndex(name, "#"); sharp != -1 && sharp != len(name)-1 {
		version := name[sharp+1:]
		libname := name[:sharp]
		return libname, version
	}
	return name, ""
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

		// rewriteLibs takes a list of names of shared libraries and scans it for three types
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

		vendorPublicLibraries := vendorPublicLibraries(actx.Config())
		vendorSnapshotSharedLibs := vendorSnapshotSharedLibs(actx.Config())

		rewriteVendorLibs := func(lib string) string {
			if isLlndkLibrary(lib, ctx.Config()) {
				return lib + llndkLibrarySuffix
			}

			// only modules with BOARD_VNDK_VERSION uses snapshot.
			if c.VndkVersion() != actx.DeviceConfig().VndkVersion() {
				return lib
			}

			if snapshot, ok := vendorSnapshotSharedLibs.get(lib, actx.Arch().ArchType); ok {
				return snapshot
			}

			return lib
		}

		rewriteLibs := func(list []string) (nonvariantLibs []string, variantLibs []string) {
			variantLibs = []string{}
			nonvariantLibs = []string{}
			for _, entry := range list {
				// strip #version suffix out
				name, _ := StubsLibNameAndVersion(entry)
				if ctx.useSdk() && inList(name, ndkPrebuiltSharedLibraries) {
					if !inList(name, ndkMigratedLibs) {
						nonvariantLibs = append(nonvariantLibs, name+".ndk."+version)
					} else {
						variantLibs = append(variantLibs, name+ndkLibrarySuffix)
					}
				} else if ctx.useVndk() {
					nonvariantLibs = append(nonvariantLibs, rewriteVendorLibs(entry))
				} else if (ctx.Platform() || ctx.ProductSpecific()) && inList(name, *vendorPublicLibraries) {
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

		deps.SharedLibs, variantNdkLibs = rewriteLibs(deps.SharedLibs)
		deps.LateSharedLibs, variantLateNdkLibs = rewriteLibs(deps.LateSharedLibs)
		deps.ReexportSharedLibHeaders, _ = rewriteLibs(deps.ReexportSharedLibHeaders)
		if ctx.useVndk() {
			for idx, lib := range deps.RuntimeLibs {
				deps.RuntimeLibs[idx] = rewriteVendorLibs(lib)
			}
		}
	}

	buildStubs := false
	if c.linker != nil {
		if library, ok := c.linker.(*libraryDecorator); ok {
			if library.buildStubs() {
				buildStubs = true
			}
		}
	}

	rewriteSnapshotLibs := func(lib string, snapshotMap *snapshotMap) string {
		// only modules with BOARD_VNDK_VERSION uses snapshot.
		if c.VndkVersion() != actx.DeviceConfig().VndkVersion() {
			return lib
		}

		if snapshot, ok := snapshotMap.get(lib, actx.Arch().ArchType); ok {
			return snapshot
		}

		return lib
	}

	vendorSnapshotHeaderLibs := vendorSnapshotHeaderLibs(actx.Config())
	for _, lib := range deps.HeaderLibs {
		depTag := headerDepTag
		if inList(lib, deps.ReexportHeaderLibHeaders) {
			depTag = headerExportDepTag
		}

		lib = rewriteSnapshotLibs(lib, vendorSnapshotHeaderLibs)

		if buildStubs {
			actx.AddFarVariationDependencies(append(ctx.Target().Variations(), c.ImageVariation()),
				depTag, lib)
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
	vendorSnapshotStaticLibs := vendorSnapshotStaticLibs(actx.Config())

	for _, lib := range deps.WholeStaticLibs {
		depTag := wholeStaticDepTag
		if impl, ok := syspropImplLibraries[lib]; ok {
			lib = impl
		}

		lib = rewriteSnapshotLibs(lib, vendorSnapshotStaticLibs)

		actx.AddVariationDependencies([]blueprint.Variation{
			{Mutator: "link", Variation: "static"},
		}, depTag, lib)
	}

	for _, lib := range deps.StaticLibs {
		depTag := StaticDepTag
		if inList(lib, deps.ReexportStaticLibHeaders) {
			depTag = staticExportDepTag
		}

		if impl, ok := syspropImplLibraries[lib]; ok {
			lib = impl
		}

		lib = rewriteSnapshotLibs(lib, vendorSnapshotStaticLibs)

		actx.AddVariationDependencies([]blueprint.Variation{
			{Mutator: "link", Variation: "static"},
		}, depTag, lib)
	}

	// staticUnwinderDep is treated as staticDep for Q apexes
	// so that native libraries/binaries are linked with static unwinder
	// because Q libc doesn't have unwinder APIs
	if deps.StaticUnwinderIfLegacy {
		actx.AddVariationDependencies([]blueprint.Variation{
			{Mutator: "link", Variation: "static"},
		}, staticUnwinderDepTag, rewriteSnapshotLibs(staticUnwinder(actx), vendorSnapshotStaticLibs))
	}

	for _, lib := range deps.LateStaticLibs {
		actx.AddVariationDependencies([]blueprint.Variation{
			{Mutator: "link", Variation: "static"},
		}, lateStaticDepTag, rewriteSnapshotLibs(lib, vendorSnapshotStaticLibs))
	}

	addSharedLibDependencies := func(depTag DependencyTag, name string, version string) {
		var variations []blueprint.Variation
		variations = append(variations, blueprint.Variation{Mutator: "link", Variation: "shared"})
		if version != "" && VersionVariantAvailable(c) {
			// Version is explicitly specified. i.e. libFoo#30
			variations = append(variations, blueprint.Variation{Mutator: "version", Variation: version})
			depTag.ExplicitlyVersioned = true
		}
		actx.AddVariationDependencies(variations, depTag, name)

		// If the version is not specified, add dependency to all stubs libraries.
		// The stubs library will be used when the depending module is built for APEX and
		// the dependent module is not in the same APEX.
		if version == "" && VersionVariantAvailable(c) {
			for _, ver := range stubsVersionsFor(actx.Config())[name] {
				// Note that depTag.ExplicitlyVersioned is false in this case.
				actx.AddVariationDependencies([]blueprint.Variation{
					{Mutator: "link", Variation: "shared"},
					{Mutator: "version", Variation: ver},
				}, depTag, name)
			}
		}
	}

	// shared lib names without the #version suffix
	var sharedLibNames []string

	for _, lib := range deps.SharedLibs {
		depTag := SharedDepTag
		if c.static() {
			depTag = SharedFromStaticDepTag
		}
		if inList(lib, deps.ReexportSharedLibHeaders) {
			depTag = sharedExportDepTag
		}

		if impl, ok := syspropImplLibraries[lib]; ok {
			lib = impl
		}

		name, version := StubsLibNameAndVersion(lib)
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

	vendorSnapshotObjects := vendorSnapshotObjects(actx.Config())

	if deps.CrtBegin != "" {
		actx.AddVariationDependencies(nil, CrtBeginDepTag, rewriteSnapshotLibs(deps.CrtBegin, vendorSnapshotObjects))
	}
	if deps.CrtEnd != "" {
		actx.AddVariationDependencies(nil, CrtEndDepTag, rewriteSnapshotLibs(deps.CrtEnd, vendorSnapshotObjects))
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
			actx.AddVariationDependencies([]blueprint.Variation{
				c.ImageVariation(),
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
func checkLinkType(ctx android.ModuleContext, from LinkableInterface, to LinkableInterface, tag DependencyTag) {
	if from.Module().Target().Os != android.Android {
		// Host code is not restricted
		return
	}

	// VNDK is cc.Module supported only for now.
	if ccFrom, ok := from.(*Module); ok && from.UseVndk() {
		// Though vendor code is limited by the vendor mutator,
		// each vendor-available module needs to check
		// link-type for VNDK.
		if ccTo, ok := to.(*Module); ok {
			if ccFrom.vndkdep != nil {
				ccFrom.vndkdep.vndkCheckLinkType(ctx, ccTo, tag)
			}
		} else {
			ctx.ModuleErrorf("Attempting to link VNDK cc.Module with unsupported module type")
		}
		return
	}
	if from.SdkVersion() == "" {
		// Platform code can link to anything
		return
	}
	if from.InRamdisk() {
		// Ramdisk code is not NDK
		return
	}
	if from.InRecovery() {
		// Recovery code is not NDK
		return
	}
	if to.ToolchainLibrary() {
		// These are always allowed
		return
	}
	if to.NdkPrebuiltStl() {
		// These are allowed, but they don't set sdk_version
		return
	}
	if to.StubDecorator() {
		// These aren't real libraries, but are the stub shared libraries that are included in
		// the NDK.
		return
	}

	if strings.HasPrefix(ctx.ModuleName(), "libclang_rt.") && to.Module().Name() == "libc++" {
		// Bug: http://b/121358700 - Allow libclang_rt.* shared libraries (with sdk_version)
		// to link to libc++ (non-NDK and without sdk_version).
		return
	}

	if to.SdkVersion() == "" {
		// NDK code linking to platform code is never okay.
		ctx.ModuleErrorf("depends on non-NDK-built library %q",
			ctx.OtherModuleName(to.Module()))
		return
	}

	// At this point we know we have two NDK libraries, but we need to
	// check that we're not linking against anything built against a higher
	// API level, as it is only valid to link against older or equivalent
	// APIs.

	// Current can link against anything.
	if from.SdkVersion() != "current" {
		// Otherwise we need to check.
		if to.SdkVersion() == "current" {
			// Current can't be linked against by anything else.
			ctx.ModuleErrorf("links %q built against newer API version %q",
				ctx.OtherModuleName(to.Module()), "current")
		} else {
			fromApi, err := strconv.Atoi(from.SdkVersion())
			if err != nil {
				ctx.PropertyErrorf("sdk_version",
					"Invalid sdk_version value (must be int or current): %q",
					from.SdkVersion())
			}
			toApi, err := strconv.Atoi(to.SdkVersion())
			if err != nil {
				ctx.PropertyErrorf("sdk_version",
					"Invalid sdk_version value (must be int or current): %q",
					to.SdkVersion())
			}

			if toApi > fromApi {
				ctx.ModuleErrorf("links %q built against newer API version %q",
					ctx.OtherModuleName(to.Module()), to.SdkVersion())
			}
		}
	}

	// Also check that the two STL choices are compatible.
	fromStl := from.SelectedStl()
	toStl := to.SelectedStl()
	if fromStl == "" || toStl == "" {
		// Libraries that don't use the STL are unrestricted.
	} else if fromStl == "ndk_system" || toStl == "ndk_system" {
		// We can be permissive with the system "STL" since it is only the C++
		// ABI layer, but in the future we should make sure that everyone is
		// using either libc++ or nothing.
	} else if getNdkStlFamily(from) != getNdkStlFamily(to) {
		ctx.ModuleErrorf("uses %q and depends on %q which uses incompatible %q",
			from.SelectedStl(), ctx.OtherModuleName(to.Module()),
			to.SelectedStl())
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
		if !to.HasVendorVariant() {
			return true
		}

		if to.isVndkSp() || to.isLlndk(ctx.Config()) || Bool(to.VendorProperties.Double_loadable) {
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
			if module.isLlndk(ctx.Config()) || Bool(module.VendorProperties.Double_loadable) {
				ctx.WalkDeps(check)
			}
		}
	}
}

// Convert dependencies to paths.  Returns a PathDeps containing paths
func (c *Module) depsToPaths(ctx android.ModuleContext) PathDeps {
	var depPaths PathDeps

	directStaticDeps := []LinkableInterface{}
	directSharedDeps := []LinkableInterface{}

	vendorPublicLibraries := vendorPublicLibraries(ctx.Config())

	reexportExporter := func(exporter exportedFlagsProducer) {
		depPaths.ReexportedDirs = append(depPaths.ReexportedDirs, exporter.exportedDirs()...)
		depPaths.ReexportedSystemDirs = append(depPaths.ReexportedSystemDirs, exporter.exportedSystemDirs()...)
		depPaths.ReexportedFlags = append(depPaths.ReexportedFlags, exporter.exportedFlags()...)
		depPaths.ReexportedDeps = append(depPaths.ReexportedDeps, exporter.exportedDeps()...)
		depPaths.ReexportedGeneratedHeaders = append(depPaths.ReexportedGeneratedHeaders, exporter.exportedGeneratedHeaders()...)
	}

	ctx.VisitDirectDeps(func(dep android.Module) {
		depName := ctx.OtherModuleName(dep)
		depTag := ctx.OtherModuleDependencyTag(dep)

		ccDep, ok := dep.(LinkableInterface)
		if !ok {

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
						genRule.GeneratedSourceFiles()...)
					depPaths.GeneratedDeps = append(depPaths.GeneratedDeps,
						genRule.GeneratedDeps()...)
					dirs := genRule.GeneratedHeaderDirs()
					depPaths.IncludeDirs = append(depPaths.IncludeDirs, dirs...)
					if depTag == genHeaderExportDepTag {
						depPaths.ReexportedDirs = append(depPaths.ReexportedDirs, dirs...)
						depPaths.ReexportedGeneratedHeaders = append(depPaths.ReexportedGeneratedHeaders,
							genRule.GeneratedSourceFiles()...)
						depPaths.ReexportedDeps = append(depPaths.ReexportedDeps, genRule.GeneratedDeps()...)
						// Add these re-exported flags to help header-abi-dumper to infer the abi exported by a library.
						c.sabi.Properties.ReexportedIncludes = append(c.sabi.Properties.ReexportedIncludes, dirs.Strings()...)

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
		if depTag == llndkImplDep {
			return
		}

		if dep.Target().Os != ctx.Os() {
			ctx.ModuleErrorf("OS mismatch between %q and %q", ctx.ModuleName(), depName)
			return
		}
		if dep.Target().Arch.ArchType != ctx.Arch().ArchType {
			ctx.ModuleErrorf("Arch mismatch between %q(%v) and %q(%v)",
				ctx.ModuleName(), ctx.Arch().ArchType, depName, dep.Target().Arch.ArchType)
			return
		}

		// re-exporting flags
		if depTag == reuseObjTag {
			// reusing objects only make sense for cc.Modules.
			if ccReuseDep, ok := ccDep.(*Module); ok && ccDep.CcLibraryInterface() {
				c.staticVariant = ccDep
				objs, exporter := ccReuseDep.compiler.(libraryInterface).reuseObjs()
				depPaths.Objs = depPaths.Objs.Append(objs)
				reexportExporter(exporter)
				return
			}
		}

		if depTag == staticVariantTag {
			// staticVariants are a cc.Module specific concept.
			if _, ok := ccDep.(*Module); ok && ccDep.CcLibraryInterface() {
				c.staticVariant = ccDep
				return
			}
		}

		// For the dependency from platform to apex, use the latest stubs
		c.apexSdkVersion = android.FutureApiLevel
		if !c.IsForPlatform() {
			c.apexSdkVersion = c.ApexProperties.Info.MinSdkVersion
		}

		if android.InList("hwaddress", ctx.Config().SanitizeDevice()) {
			// In hwasan build, we override apexSdkVersion to the FutureApiLevel(10000)
			// so that even Q(29/Android10) apexes could use the dynamic unwinder by linking the newer stubs(e.g libc(R+)).
			// (b/144430859)
			c.apexSdkVersion = android.FutureApiLevel
		}

		if depTag == staticUnwinderDepTag {
			// Use static unwinder for legacy (min_sdk_version = 29) apexes (b/144430859)
			if c.apexSdkVersion <= android.SdkVersion_Android10 {
				depTag = StaticDepTag
			} else {
				return
			}
		}

		// Extract ExplicitlyVersioned field from the depTag and reset it inside the struct.
		// Otherwise, SharedDepTag and lateSharedDepTag with ExplicitlyVersioned set to true
		// won't be matched to SharedDepTag and lateSharedDepTag.
		explicitlyVersioned := false
		if t, ok := depTag.(DependencyTag); ok {
			explicitlyVersioned = t.ExplicitlyVersioned
			t.ExplicitlyVersioned = false
			depTag = t
		}

		if t, ok := depTag.(DependencyTag); ok && t.Library {
			depIsStatic := false
			switch depTag {
			case StaticDepTag, staticExportDepTag, lateStaticDepTag, wholeStaticDepTag:
				depIsStatic = true
			}
			if ccDep.CcLibrary() && !depIsStatic {
				depIsStubs := ccDep.BuildStubs()
				depHasStubs := VersionVariantAvailable(c) && ccDep.HasStubsVariants()
				depInSameApexes := android.DirectlyInAllApexes(c.InApexes(), depName)
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
					if c.bootstrap() {
						// However, for host, ramdisk, recovery or bootstrap modules,
						// always link to non-stub variant
						useThisDep = !depIsStubs
					}
					for _, testFor := range c.TestFor() {
						// Another exception: if this module is bundled with an APEX, then
						// it is linked with the non-stub variant of a module in the APEX
						// as if this is part of the APEX.
						if android.DirectlyInApex(testFor, depName) {
							useThisDep = !depIsStubs
							break
						}
					}
				} else {
					// If building for APEX, use stubs when the parent is in any APEX that
					// the child is not in.
					useThisDep = (depInSameApexes != depIsStubs)
				}

				// when to use (unspecified) stubs, check min_sdk_version and choose the right one
				if useThisDep && depIsStubs && !explicitlyVersioned {
					versionToUse, err := c.ChooseSdkVersion(ccDep.StubsVersions(), c.apexSdkVersion)
					if err != nil {
						ctx.OtherModuleErrorf(dep, err.Error())
						return
					}
					if versionToUse != ccDep.StubsVersion() {
						useThisDep = false
					}
				}

				if !useThisDep {
					return // stop processing this dep
				}
			}
			if c.UseVndk() {
				if m, ok := ccDep.(*Module); ok && m.IsStubs() { // LLNDK
					// by default, use current version of LLNDK
					versionToUse := ""
					versions := stubsVersionsFor(ctx.Config())[depName]
					if c.ApexVariationName() != "" && len(versions) > 0 {
						// if this is for use_vendor apex && dep has stubsVersions
						// apply the same rule of apex sdk enforcement to choose right version
						var err error
						versionToUse, err = c.ChooseSdkVersion(versions, c.apexSdkVersion)
						if err != nil {
							ctx.OtherModuleErrorf(dep, err.Error())
							return
						}
					}
					if versionToUse != ccDep.StubsVersion() {
						return
					}
				}
			}

			depPaths.IncludeDirs = append(depPaths.IncludeDirs, ccDep.IncludeDirs()...)

			// Exporting flags only makes sense for cc.Modules
			if _, ok := ccDep.(*Module); ok {
				if i, ok := ccDep.(*Module).linker.(exportedFlagsProducer); ok {
					depPaths.SystemIncludeDirs = append(depPaths.SystemIncludeDirs, i.exportedSystemDirs()...)
					depPaths.GeneratedHeaders = append(depPaths.GeneratedHeaders, i.exportedGeneratedHeaders()...)
					depPaths.GeneratedDeps = append(depPaths.GeneratedDeps, i.exportedDeps()...)
					depPaths.Flags = append(depPaths.Flags, i.exportedFlags()...)

					if t.ReexportFlags {
						reexportExporter(i)
						// Add these re-exported flags to help header-abi-dumper to infer the abi exported by a library.
						// Re-exported shared library headers must be included as well since they can help us with type information
						// about template instantiations (instantiated from their headers).
						// -isystem headers are not included since for bionic libraries, abi-filtering is taken care of by version
						// scripts.
						c.sabi.Properties.ReexportedIncludes = append(
							c.sabi.Properties.ReexportedIncludes, i.exportedDirs().Strings()...)
					}
				}
			}
			checkLinkType(ctx, c, ccDep, t)
		}

		var ptr *android.Paths
		var depPtr *android.Paths

		linkFile := ccDep.OutputFile()
		depFile := android.OptionalPath{}

		switch depTag {
		case ndkStubDepTag, SharedDepTag, SharedFromStaticDepTag, sharedExportDepTag:
			ptr = &depPaths.SharedLibs
			depPtr = &depPaths.SharedLibsDeps
			depFile = ccDep.Toc()
			directSharedDeps = append(directSharedDeps, ccDep)

		case earlySharedDepTag:
			ptr = &depPaths.EarlySharedLibs
			depPtr = &depPaths.EarlySharedLibsDeps
			depFile = ccDep.Toc()
			directSharedDeps = append(directSharedDeps, ccDep)
		case lateSharedDepTag, ndkLateStubDepTag:
			ptr = &depPaths.LateSharedLibs
			depPtr = &depPaths.LateSharedLibsDeps
			depFile = ccDep.Toc()
		case StaticDepTag, staticExportDepTag:
			ptr = nil
			directStaticDeps = append(directStaticDeps, ccDep)
		case lateStaticDepTag:
			ptr = &depPaths.LateStaticLibs
		case wholeStaticDepTag:
			ptr = &depPaths.WholeStaticLibs
			if !ccDep.CcLibraryInterface() || !ccDep.Static() {
				ctx.ModuleErrorf("module %q not a static library", depName)
				return
			}

			// Because the static library objects are included, this only makes sense
			// in the context of proper cc.Modules.
			if ccWholeStaticLib, ok := ccDep.(*Module); ok {
				staticLib := ccWholeStaticLib.linker.(libraryInterface)
				if missingDeps := staticLib.getWholeStaticMissingDeps(); missingDeps != nil {
					postfix := " (required by " + ctx.OtherModuleName(dep) + ")"
					for i := range missingDeps {
						missingDeps[i] += postfix
					}
					ctx.AddMissingDependencies(missingDeps)
				}
				depPaths.WholeStaticLibObjs = depPaths.WholeStaticLibObjs.Append(staticLib.objs())
			} else {
				ctx.ModuleErrorf(
					"non-cc.Modules cannot be included as whole static libraries.", depName)
				return
			}
		case headerDepTag:
			// Nothing
		case objDepTag:
			depPaths.Objs.objFiles = append(depPaths.Objs.objFiles, linkFile.Path())
		case CrtBeginDepTag:
			depPaths.CrtBegin = linkFile
		case CrtEndDepTag:
			depPaths.CrtEnd = linkFile
		case dynamicLinkerDepTag:
			depPaths.DynamicLinker = linkFile
		}

		switch depTag {
		case StaticDepTag, staticExportDepTag, lateStaticDepTag:
			if !ccDep.CcLibraryInterface() || !ccDep.Static() {
				ctx.ModuleErrorf("module %q not a static library", depName)
				return
			}

			// When combining coverage files for shared libraries and executables, coverage files
			// in static libraries act as if they were whole static libraries. The same goes for
			// source based Abi dump files.
			// This should only be done for cc.Modules
			if c, ok := ccDep.(*Module); ok {
				staticLib := c.linker.(libraryInterface)
				depPaths.StaticLibObjs.coverageFiles = append(depPaths.StaticLibObjs.coverageFiles,
					staticLib.objs().coverageFiles...)
				depPaths.StaticLibObjs.sAbiDumpFiles = append(depPaths.StaticLibObjs.sAbiDumpFiles,
					staticLib.objs().sAbiDumpFiles...)
			}
		}

		if ptr != nil {
			if !linkFile.Valid() {
				if !ctx.Config().AllowMissingDependencies() {
					ctx.ModuleErrorf("module %q missing output file", depName)
				} else {
					ctx.AddMissingDependencies([]string{depName})
				}
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

		vendorSuffixModules := vendorSuffixModules(ctx.Config())

		baseLibName := func(depName string) string {
			libName := strings.TrimSuffix(depName, llndkLibrarySuffix)
			libName = strings.TrimSuffix(libName, vendorPublicLibrarySuffix)
			libName = strings.TrimPrefix(libName, "prebuilt_")
			return libName
		}

		makeLibName := func(depName string) string {
			libName := baseLibName(depName)
			isLLndk := isLlndkLibrary(libName, ctx.Config())
			isVendorPublicLib := inList(libName, *vendorPublicLibraries)
			bothVendorAndCoreVariantsExist := ccDep.HasVendorVariant() || isLLndk

			if c, ok := ccDep.(*Module); ok {
				// Use base module name for snapshots when exporting to Makefile.
				if c.isSnapshotPrebuilt() {
					baseName := c.BaseModuleName()

					if c.IsVndk() {
						return baseName + ".vendor"
					}

					if vendorSuffixModules[baseName] {
						return baseName + ".vendor"
					} else {
						return baseName
					}
				}
			}

			if ctx.DeviceConfig().VndkUseCoreVariant() && ccDep.IsVndk() && !ccDep.MustUseVendorVariant() && !c.InRamdisk() && !c.InRecovery() {
				// The vendor module is a no-vendor-variant VNDK library.  Depend on the
				// core module instead.
				return libName
			} else if c.UseVndk() && bothVendorAndCoreVariantsExist {
				// The vendor module in Make will have been renamed to not conflict with the core
				// module, so update the dependency name here accordingly.
				return libName + c.getNameSuffixWithVndkVersion(ctx)
			} else if (ctx.Platform() || ctx.ProductSpecific()) && isVendorPublicLib {
				return libName + vendorPublicLibrarySuffix
			} else if ccDep.InRamdisk() && !ccDep.OnlyInRamdisk() {
				return libName + ramdiskSuffix
			} else if ccDep.InRecovery() && !ccDep.OnlyInRecovery() {
				return libName + recoverySuffix
			} else if ccDep.Module().Target().NativeBridge == android.NativeBridgeEnabled {
				return libName + nativeBridgeSuffix
			} else {
				return libName
			}
		}

		// Export the shared libs to Make.
		switch depTag {
		case SharedDepTag, sharedExportDepTag, lateSharedDepTag, earlySharedDepTag:
			if ccDep.CcLibrary() {
				if ccDep.BuildStubs() && android.InAnyApex(depName) {
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
			// Record baseLibName for snapshots.
			c.Properties.SnapshotSharedLibs = append(c.Properties.SnapshotSharedLibs, baseLibName(depName))
		case ndkStubDepTag, ndkLateStubDepTag:
			c.Properties.AndroidMkSharedLibs = append(
				c.Properties.AndroidMkSharedLibs,
				depName+"."+ccDep.ApiLevel())
		case StaticDepTag, staticExportDepTag, lateStaticDepTag:
			c.Properties.AndroidMkStaticLibs = append(
				c.Properties.AndroidMkStaticLibs, makeLibName(depName))
		case runtimeDepTag:
			c.Properties.AndroidMkRuntimeLibs = append(
				c.Properties.AndroidMkRuntimeLibs, makeLibName(depName))
			// Record baseLibName for snapshots.
			c.Properties.SnapshotRuntimeLibs = append(c.Properties.SnapshotRuntimeLibs, baseLibName(depName))
		case wholeStaticDepTag:
			c.Properties.AndroidMkWholeStaticLibs = append(
				c.Properties.AndroidMkWholeStaticLibs, makeLibName(depName))
		}
	})

	// use the ordered dependencies as this module's dependencies
	depPaths.StaticLibs = append(depPaths.StaticLibs, orderStaticModuleDeps(c, directStaticDeps, directSharedDeps)...)

	// Dedup exported flags from dependencies
	depPaths.Flags = android.FirstUniqueStrings(depPaths.Flags)
	depPaths.IncludeDirs = android.FirstUniquePaths(depPaths.IncludeDirs)
	depPaths.SystemIncludeDirs = android.FirstUniquePaths(depPaths.SystemIncludeDirs)
	depPaths.GeneratedHeaders = android.FirstUniquePaths(depPaths.GeneratedHeaders)
	depPaths.GeneratedDeps = android.FirstUniquePaths(depPaths.GeneratedDeps)
	depPaths.ReexportedDirs = android.FirstUniquePaths(depPaths.ReexportedDirs)
	depPaths.ReexportedSystemDirs = android.FirstUniquePaths(depPaths.ReexportedSystemDirs)
	depPaths.ReexportedFlags = android.FirstUniqueStrings(depPaths.ReexportedFlags)
	depPaths.ReexportedDeps = android.FirstUniquePaths(depPaths.ReexportedDeps)
	depPaths.ReexportedGeneratedHeaders = android.FirstUniquePaths(depPaths.ReexportedGeneratedHeaders)

	if c.sabi != nil {
		c.sabi.Properties.ReexportedIncludes = android.FirstUniqueStrings(c.sabi.Properties.ReexportedIncludes)
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

func (c *Module) InstallInRamdisk() bool {
	return c.InRamdisk()
}

func (c *Module) InstallInRecovery() bool {
	return c.InRecovery()
}

func (c *Module) SkipInstall() {
	if c.installer == nil {
		c.ModuleBase.SkipInstall()
		return
	}
	c.installer.skipInstall(c)
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

func (c *Module) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		if c.outputFile.Valid() {
			return android.Paths{c.outputFile.Path()}, nil
		}
		return android.Paths{}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
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

func (c *Module) binary() bool {
	if b, ok := c.linker.(interface {
		binary() bool
	}); ok {
		return b.binary()
	}
	return false
}

func (c *Module) object() bool {
	if o, ok := c.linker.(interface {
		object() bool
	}); ok {
		return o.object()
	}
	return false
}

func (c *Module) getMakeLinkType(actx android.ModuleContext) string {
	if c.UseVndk() {
		if lib, ok := c.linker.(*llndkStubDecorator); ok {
			if Bool(lib.Properties.Vendor_available) {
				return "native:vndk"
			}
			return "native:vndk_private"
		}
		if c.IsVndk() && !c.isVndkExt() {
			if Bool(c.VendorProperties.Vendor_available) {
				return "native:vndk"
			}
			return "native:vndk_private"
		}
		if c.inProduct() {
			return "native:product"
		}
		return "native:vendor"
	} else if c.InRamdisk() {
		return "native:ramdisk"
	} else if c.InRecovery() {
		return "native:recovery"
	} else if c.Target().Os == android.Android && String(c.Properties.Sdk_version) != "" {
		return "native:ndk:none:none"
		// TODO(b/114741097): use the correct ndk stl once build errors have been fixed
		//family, link := getNdkStlFamilyAndLinkType(c)
		//return fmt.Sprintf("native:ndk:%s:%s", family, link)
	} else if actx.DeviceConfig().VndkUseCoreVariant() && !c.MustUseVendorVariant() {
		return "native:platform_vndk"
	} else {
		return "native:platform"
	}
}

// Overrides ApexModule.IsInstallabeToApex()
// Only shared/runtime libraries and "test_per_src" tests are installable to APEX.
func (c *Module) IsInstallableToApex() bool {
	if shared, ok := c.linker.(interface {
		shared() bool
	}); ok {
		// Stub libs and prebuilt libs in a versioned SDK are not
		// installable to APEX even though they are shared libs.
		return shared.shared() && !c.IsStubs() && c.ContainingSdk().Unversioned()
	} else if _, ok := c.linker.(testPerSrc); ok {
		return true
	}
	return false
}

func (c *Module) AvailableFor(what string) bool {
	if linker, ok := c.linker.(interface {
		availableFor(string) bool
	}); ok {
		return c.ApexModuleBase.AvailableFor(what) || linker.availableFor(what)
	} else {
		return c.ApexModuleBase.AvailableFor(what)
	}
}

func (c *Module) TestFor() []string {
	if test, ok := c.linker.(interface {
		testFor() []string
	}); ok {
		return test.testFor()
	} else {
		return c.ApexModuleBase.TestFor()
	}
}

func (c *Module) UniqueApexVariations() bool {
	if u, ok := c.compiler.(interface {
		uniqueApexVariations() bool
	}); ok {
		return u.uniqueApexVariations()
	} else {
		return false
	}
}

// Return true if the module is ever installable.
func (c *Module) EverInstallable() bool {
	return c.installer != nil &&
		// Check to see whether the module is actually ever installable.
		c.installer.everInstallable()
}

func (c *Module) installable() bool {
	ret := c.EverInstallable() &&
		// Check to see whether the module has been configured to not be installed.
		proptools.BoolDefault(c.Properties.Installable, true) &&
		!c.Properties.PreventInstall && c.outputFile.Valid()

	// The platform variant doesn't need further condition. Apex variants however might not
	// be installable because it will likely to be included in the APEX and won't appear
	// in the system partition.
	if c.IsForPlatform() {
		return ret
	}

	// Special case for modules that are configured to be installed to /data, which includes
	// test modules. For these modules, both APEX and non-APEX variants are considered as
	// installable. This is because even the APEX variants won't be included in the APEX, but
	// will anyway be installed to /data/*.
	// See b/146995717
	if c.InstallInData() {
		return ret
	}

	return false
}

func (c *Module) AndroidMkWriteAdditionalDependenciesForSourceAbiDiff(w io.Writer) {
	if c.linker != nil {
		if library, ok := c.linker.(*libraryDecorator); ok {
			library.androidMkWriteAdditionalDependenciesForSourceAbiDiff(w)
		}
	}
}

func (c *Module) DepIsInSameApex(ctx android.BaseModuleContext, dep android.Module) bool {
	if depTag, ok := ctx.OtherModuleDependencyTag(dep).(DependencyTag); ok {
		if cc, ok := dep.(*Module); ok {
			if cc.HasStubsVariants() {
				if depTag.Shared && depTag.Library {
					// dynamic dep to a stubs lib crosses APEX boundary
					return false
				}
				if IsRuntimeDepTag(depTag) {
					// runtime dep to a stubs lib also crosses APEX boundary
					return false
				}
			}
			if depTag.FromStatic {
				// shared_lib dependency from a static lib is considered as crossing
				// the APEX boundary because the dependency doesn't actually is
				// linked; the dependency is used only during the compilation phase.
				return false
			}
		}
	}
	return true
}

//
// Defaults
//
type Defaults struct {
	android.ModuleBase
	android.DefaultsModuleBase
	android.ApexModuleBase
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
		&ObjectLinkerProperties{},
		&LibraryProperties{},
		&StaticProperties{},
		&SharedProperties{},
		&FlagExporterProperties{},
		&BinaryLinkerProperties{},
		&TestProperties{},
		&TestBinaryProperties{},
		&FuzzProperties{},
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
		&android.ProtoProperties{},
	)

	android.InitDefaultsModule(module)

	return module
}

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

var _ android.ImageInterface = (*Module)(nil)

func (m *Module) ImageMutatorBegin(mctx android.BaseModuleContext) {
	// Sanity check
	vendorSpecific := mctx.SocSpecific() || mctx.DeviceSpecific()
	productSpecific := mctx.ProductSpecific()

	if m.VendorProperties.Vendor_available != nil && vendorSpecific {
		mctx.PropertyErrorf("vendor_available",
			"doesn't make sense at the same time as `vendor: true`, `proprietary: true`, or `device_specific:true`")
	}

	if vndkdep := m.vndkdep; vndkdep != nil {
		if vndkdep.isVndk() {
			if vendorSpecific || productSpecific {
				if !vndkdep.isVndkExt() {
					mctx.PropertyErrorf("vndk",
						"must set `extends: \"...\"` to vndk extension")
				} else if m.VendorProperties.Vendor_available != nil {
					mctx.PropertyErrorf("vendor_available",
						"must not set at the same time as `vndk: {extends: \"...\"}`")
				}
			} else {
				if vndkdep.isVndkExt() {
					mctx.PropertyErrorf("vndk",
						"must set `vendor: true` or `product_specific: true` to set `extends: %q`",
						m.getVndkExtendsModuleName())
				}
				if m.VendorProperties.Vendor_available == nil {
					mctx.PropertyErrorf("vndk",
						"vendor_available must be set to either true or false when `vndk: {enabled: true}`")
				}
			}
		} else {
			if vndkdep.isVndkSp() {
				mctx.PropertyErrorf("vndk",
					"must set `enabled: true` to set `support_system_process: true`")
			}
			if vndkdep.isVndkExt() {
				mctx.PropertyErrorf("vndk",
					"must set `enabled: true` to set `extends: %q`",
					m.getVndkExtendsModuleName())
			}
		}
	}

	var coreVariantNeeded bool = false
	var ramdiskVariantNeeded bool = false
	var recoveryVariantNeeded bool = false

	var vendorVariants []string
	var productVariants []string

	platformVndkVersion := mctx.DeviceConfig().PlatformVndkVersion()
	boardVndkVersion := mctx.DeviceConfig().VndkVersion()
	productVndkVersion := mctx.DeviceConfig().ProductVndkVersion()
	if boardVndkVersion == "current" {
		boardVndkVersion = platformVndkVersion
	}
	if productVndkVersion == "current" {
		productVndkVersion = platformVndkVersion
	}

	if boardVndkVersion == "" {
		// If the device isn't compiling against the VNDK, we always
		// use the core mode.
		coreVariantNeeded = true
	} else if _, ok := m.linker.(*llndkStubDecorator); ok {
		// LL-NDK stubs only exist in the vendor and product variants,
		// since the real libraries will be used in the core variant.
		vendorVariants = append(vendorVariants,
			platformVndkVersion,
			boardVndkVersion,
		)
		productVariants = append(productVariants,
			platformVndkVersion,
			productVndkVersion,
		)
	} else if _, ok := m.linker.(*llndkHeadersDecorator); ok {
		// ... and LL-NDK headers as well
		vendorVariants = append(vendorVariants,
			platformVndkVersion,
			boardVndkVersion,
		)
		productVariants = append(productVariants,
			platformVndkVersion,
			productVndkVersion,
		)
	} else if m.isSnapshotPrebuilt() {
		// Make vendor variants only for the versions in BOARD_VNDK_VERSION and
		// PRODUCT_EXTRA_VNDK_VERSIONS.
		if snapshot, ok := m.linker.(interface {
			version() string
		}); ok {
			vendorVariants = append(vendorVariants, snapshot.version())
		} else {
			mctx.ModuleErrorf("version is unknown for snapshot prebuilt")
		}
	} else if m.HasVendorVariant() && !m.isVndkExt() {
		// This will be available in /system, /vendor and /product
		// or a /system directory that is available to vendor and product.
		coreVariantNeeded = true

		// We assume that modules under proprietary paths are compatible for
		// BOARD_VNDK_VERSION. The other modules are regarded as AOSP, or
		// PLATFORM_VNDK_VERSION.
		if isVendorProprietaryPath(mctx.ModuleDir()) {
			vendorVariants = append(vendorVariants, boardVndkVersion)
		} else {
			vendorVariants = append(vendorVariants, platformVndkVersion)
		}

		// vendor_available modules are also available to /product.
		productVariants = append(productVariants, platformVndkVersion)
		// VNDK is always PLATFORM_VNDK_VERSION
		if !m.IsVndk() {
			productVariants = append(productVariants, productVndkVersion)
		}
	} else if vendorSpecific && String(m.Properties.Sdk_version) == "" {
		// This will be available in /vendor (or /odm) only
		// We assume that modules under proprietary paths are compatible for
		// BOARD_VNDK_VERSION. The other modules are regarded as AOSP, or
		// PLATFORM_VNDK_VERSION.
		if isVendorProprietaryPath(mctx.ModuleDir()) {
			vendorVariants = append(vendorVariants, boardVndkVersion)
		} else {
			vendorVariants = append(vendorVariants, platformVndkVersion)
		}
	} else {
		// This is either in /system (or similar: /data), or is a
		// modules built with the NDK. Modules built with the NDK
		// will be restricted using the existing link type checks.
		coreVariantNeeded = true
	}

	if boardVndkVersion != "" && productVndkVersion != "" {
		if coreVariantNeeded && productSpecific && String(m.Properties.Sdk_version) == "" {
			// The module has "product_specific: true" that does not create core variant.
			coreVariantNeeded = false
			productVariants = append(productVariants, productVndkVersion)
		}
	} else {
		// Unless PRODUCT_PRODUCT_VNDK_VERSION is set, product partition has no
		// restriction to use system libs.
		// No product variants defined in this case.
		productVariants = []string{}
	}

	if Bool(m.Properties.Ramdisk_available) {
		ramdiskVariantNeeded = true
	}

	if m.ModuleBase.InstallInRamdisk() {
		ramdiskVariantNeeded = true
		coreVariantNeeded = false
	}

	if Bool(m.Properties.Recovery_available) {
		recoveryVariantNeeded = true
	}

	if m.ModuleBase.InstallInRecovery() {
		recoveryVariantNeeded = true
		coreVariantNeeded = false
	}

	for _, variant := range android.FirstUniqueStrings(vendorVariants) {
		m.Properties.ExtraVariants = append(m.Properties.ExtraVariants, VendorVariationPrefix+variant)
	}

	for _, variant := range android.FirstUniqueStrings(productVariants) {
		m.Properties.ExtraVariants = append(m.Properties.ExtraVariants, ProductVariationPrefix+variant)
	}

	m.Properties.RamdiskVariantNeeded = ramdiskVariantNeeded
	m.Properties.RecoveryVariantNeeded = recoveryVariantNeeded
	m.Properties.CoreVariantNeeded = coreVariantNeeded
}

func (c *Module) CoreVariantNeeded(ctx android.BaseModuleContext) bool {
	return c.Properties.CoreVariantNeeded
}

func (c *Module) RamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return c.Properties.RamdiskVariantNeeded
}

func (c *Module) RecoveryVariantNeeded(ctx android.BaseModuleContext) bool {
	return c.Properties.RecoveryVariantNeeded
}

func (c *Module) ExtraImageVariations(ctx android.BaseModuleContext) []string {
	return c.Properties.ExtraVariants
}

func (c *Module) SetImageVariation(ctx android.BaseModuleContext, variant string, module android.Module) {
	m := module.(*Module)
	if variant == android.RamdiskVariation {
		m.MakeAsPlatform()
	} else if variant == android.RecoveryVariation {
		m.MakeAsPlatform()
		squashRecoverySrcs(m)
	} else if strings.HasPrefix(variant, VendorVariationPrefix) {
		m.Properties.ImageVariationPrefix = VendorVariationPrefix
		m.Properties.VndkVersion = strings.TrimPrefix(variant, VendorVariationPrefix)
		squashVendorSrcs(m)

		// Makefile shouldn't know vendor modules other than BOARD_VNDK_VERSION.
		// Hide other vendor variants to avoid collision.
		vndkVersion := ctx.DeviceConfig().VndkVersion()
		if vndkVersion != "current" && vndkVersion != "" && vndkVersion != m.Properties.VndkVersion {
			m.Properties.HideFromMake = true
			m.SkipInstall()
		}
	} else if strings.HasPrefix(variant, ProductVariationPrefix) {
		m.Properties.ImageVariationPrefix = ProductVariationPrefix
		m.Properties.VndkVersion = strings.TrimPrefix(variant, ProductVariationPrefix)
		squashVendorSrcs(m)
	}
}

func getCurrentNdkPrebuiltVersion(ctx DepsContext) string {
	if ctx.Config().PlatformSdkVersionInt() > config.NdkMaxPrebuiltVersionInt {
		return strconv.Itoa(config.NdkMaxPrebuiltVersionInt)
	}
	return ctx.Config().PlatformSdkVersion()
}

func kytheExtractAllFactory() android.Singleton {
	return &kytheExtractAllSingleton{}
}

type kytheExtractAllSingleton struct {
}

func (ks *kytheExtractAllSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	var xrefTargets android.Paths
	ctx.VisitAllModules(func(module android.Module) {
		if ccModule, ok := module.(xref); ok {
			xrefTargets = append(xrefTargets, ccModule.XrefCcFiles()...)
		}
	})
	// TODO(asmundak): Perhaps emit a rule to output a warning if there were no xrefTargets
	if len(xrefTargets) > 0 {
		ctx.Phony("xref_cxx", xrefTargets...)
	}
}

var Bool = proptools.Bool
var BoolDefault = proptools.BoolDefault
var BoolPtr = proptools.BoolPtr
var String = proptools.String
var StringPtr = proptools.StringPtr
