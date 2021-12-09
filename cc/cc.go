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
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/depset"
	"github.com/google/blueprint/proptools"

	"android/soong/aidl_library"
	"android/soong/android"
	"android/soong/cc/config"
	"android/soong/fuzz"
)

type CcMakeVarsInfo struct {
	WarningsAllowed string
	UsingWnoError   string
	MissingProfile  string
}

var CcMakeVarsInfoProvider = blueprint.NewProvider[*CcMakeVarsInfo]()

type CcObjectInfo struct {
	ObjFiles   android.Paths
	TidyFiles  android.Paths
	KytheFiles android.Paths
}

var CcObjectInfoProvider = blueprint.NewProvider[CcObjectInfo]()

type AidlInterfaceInfo struct {
	// list of aidl_interface sources
	Sources []string
	// root directory of AIDL sources
	AidlRoot string
	// AIDL backend language (e.g. "cpp", "ndk")
	Lang string
	// list of flags passed to AIDL generator
	Flags []string
}

type CompilerInfo struct {
	Srcs android.Paths
	// list of module-specific flags that will be used for C and C++ compiles.
	Cflags               []string
	AidlInterfaceInfo    AidlInterfaceInfo
	LibraryDecoratorInfo *LibraryDecoratorInfo
}

type LinkerInfo struct {
	WholeStaticLibs []string
	// list of modules that should be statically linked into this module.
	StaticLibs []string
	// list of modules that should be dynamically linked into this module.
	SharedLibs []string
	// list of modules that should only provide headers for this module.
	HeaderLibs               []string
	ImplementationModuleName *string

	BinaryDecoratorInfo       *BinaryDecoratorInfo
	LibraryDecoratorInfo      *LibraryDecoratorInfo
	TestBinaryInfo            *TestBinaryInfo
	BenchmarkDecoratorInfo    *BenchmarkDecoratorInfo
	ObjectLinkerInfo          *ObjectLinkerInfo
	StubDecoratorInfo         *StubDecoratorInfo
	PrebuiltLibraryLinkerInfo *PrebuiltLibraryLinkerInfo
}

type BinaryDecoratorInfo struct{}
type LibraryDecoratorInfo struct {
	ExportIncludeDirs []string
	InjectBsslHash    bool
	// Location of the static library in the sysroot. Empty if the library is
	// not included in the NDK.
	NdkSysrootPath android.Path
	VndkFileName   string
}

type SnapshotInfo struct {
	SnapshotAndroidMkSuffix string
}

type TestBinaryInfo struct {
	Gtest bool
}
type BenchmarkDecoratorInfo struct{}

type StubDecoratorInfo struct {
	AbiDumpPath  android.OutputPath
	HasAbiDump   bool
	AbiDiffPaths android.Paths
	InstallPath  android.Path
}

type ObjectLinkerInfo struct {
	// Location of the object in the sysroot. Empty if the object is not
	// included in the NDK.
	NdkSysrootPath android.Path
}

type PrebuiltLibraryLinkerInfo struct {
	VndkFileName string
}

type LibraryInfo struct {
	BuildStubs bool
}

type InstallerInfo struct {
	StubDecoratorInfo *StubDecoratorInfo
}

type LocalOrGlobalFlagsInfo struct {
	CommonFlags []string // Flags that apply to C, C++, and assembly source files
	CFlags      []string // Flags that apply to C and C++ source files
	ConlyFlags  []string // Flags that apply to C source files
	CppFlags    []string // Flags that apply to C++ source files
}

// Common info about the cc module.
type CcInfo struct {
	IsPrebuilt             bool
	CmakeSnapshotSupported bool
	HasLlndkStubs          bool
	DataPaths              []android.DataPath
	CompilerInfo           *CompilerInfo
	LinkerInfo             *LinkerInfo
	SnapshotInfo           *SnapshotInfo
	LibraryInfo            *LibraryInfo
	InstallerInfo          *InstallerInfo
}

var CcInfoProvider = blueprint.NewProvider[*CcInfo]()

type LinkableInfo struct {
	// StaticExecutable returns true if this is a binary module with "static_executable: true".
	StaticExecutable     bool
	Static               bool
	Shared               bool
	Header               bool
	HasStubsVariants     bool
	StubsVersion         string
	IsStubs              bool
	UnstrippedOutputFile android.Path
	OutputFile           android.OptionalPath
	CoverageFiles        android.Paths
	// CoverageOutputFile returns the output archive of gcno coverage information files.
	CoverageOutputFile android.OptionalPath
	SAbiDumpFiles      android.Paths
	// Partition returns the partition string for this module.
	Partition            string
	CcLibrary            bool
	CcLibraryInterface   bool
	RustLibraryInterface bool
	// CrateName returns the crateName for a Rust library
	CrateName string
	// DepFlags returns a slice of Rustc string flags
	ExportedCrateLinkDirs []string
	HasNonSystemVariants  bool
	IsLlndk               bool
	// True if the library is in the configs known NDK list.
	IsNdk             bool
	InVendorOrProduct bool
	// SubName returns the modules SubName, used for image and NDK/SDK variations.
	SubName             string
	InRamdisk           bool
	OnlyInRamdisk       bool
	InVendorRamdisk     bool
	OnlyInVendorRamdisk bool
	InRecovery          bool
	OnlyInRecovery      bool
	InVendor            bool
	Installable         *bool
	// RelativeInstallPath returns the relative install path for this module.
	RelativeInstallPath string
	// TODO(b/362509506): remove this once all apex_exclude uses are switched to stubs.
	RustApexExclude bool
	// Bootstrap tests if this module is allowed to use non-APEX version of libraries.
	Bootstrap                       bool
	Multilib                        string
	ImplementationModuleNameForMake string
	IsStubsImplementationRequired   bool
	// Symlinks returns a list of symlinks that should be created for this module.
	Symlinks               []string
	APIListCoverageXMLPath android.ModuleOutPath
	// FuzzSharedLibraries returns the shared library dependencies for this module.
	// Expects that IsFuzzModule returns true.
	FuzzSharedLibraries      android.RuleBuilderInstalls
	IsVndkPrebuiltLibrary    bool
	HasLLNDKStubs            bool
	IsLLNDKMovedToApex       bool
	ImplementationModuleName string
}

var LinkableInfoProvider = blueprint.NewProvider[*LinkableInfo]()

func init() {
	RegisterCCBuildComponents(android.InitRegistrationContext)

	pctx.Import("android/soong/android")
	pctx.Import("android/soong/cc/config")
}

func RegisterCCBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("cc_defaults", defaultsFactory)

	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.Transition("sdk", &sdkTransitionMutator{})
		ctx.BottomUp("llndk", llndkMutator)
		ctx.Transition("link", &linkageTransitionMutator{})
		ctx.Transition("version", &versionTransitionMutator{})
		ctx.BottomUp("begin", BeginMutator)
	})

	ctx.PostDepsMutators(func(ctx android.RegisterMutatorsContext) {
		for _, san := range Sanitizers {
			san.registerMutators(ctx)
		}

		ctx.BottomUp("sanitize_runtime_deps", sanitizerRuntimeDepsMutator)
		ctx.BottomUp("sanitize_runtime", sanitizerRuntimeMutator)

		ctx.Transition("fuzz", &fuzzTransitionMutator{})

		ctx.Transition("coverage", &coverageTransitionMutator{})

		ctx.Transition("afdo", &afdoTransitionMutator{})

		ctx.Transition("orderfile", &orderfileTransitionMutator{})

		ctx.Transition("lto", &ltoTransitionMutator{})

		ctx.BottomUp("check_linktype", checkLinkTypeMutator)
		ctx.BottomUp("double_loadable", checkDoubleLoadableLibraries)
	})

	ctx.PostApexMutators(func(ctx android.RegisterMutatorsContext) {
		// sabi mutator needs to be run after apex mutator finishes.
		ctx.Transition("sabi", &sabiTransitionMutator{})
	})

	ctx.RegisterParallelSingletonType("kythe_extract_all", kytheExtractAllFactory)
}

// Deps is a struct containing module names of dependencies, separated by the kind of dependency.
// Mutators should use `AddVariationDependencies` or its sibling methods to add actual dependency
// edges to these modules.
// This object is constructed in DepsMutator, by calling to various module delegates to set
// relevant fields. For example, `module.compiler.compilerDeps()` may append type-specific
// dependencies.
// This is then consumed by the same DepsMutator, which will call `ctx.AddVariationDependencies()`
// (or its sibling methods) to set real dependencies on the given modules.
type Deps struct {
	SharedLibs, LateSharedLibs                  []string
	StaticLibs, LateStaticLibs, WholeStaticLibs []string
	HeaderLibs                                  []string
	RuntimeLibs                                 []string

	// UnexportedStaticLibs are static libraries that are also passed to -Wl,--exclude-libs= to
	// prevent automatically exporting symbols.
	UnexportedStaticLibs []string

	// Used for data dependencies adjacent to tests
	DataLibs []string
	DataBins []string

	// Used by DepsMutator to pass system_shared_libs information to check_elf_file.py.
	SystemSharedLibs []string

	// Used by DepMutator to pass aidl_library modules to aidl compiler
	AidlLibs []string

	// If true, statically link the unwinder into native libraries/binaries.
	StaticUnwinderIfLegacy bool

	ReexportSharedLibHeaders, ReexportStaticLibHeaders, ReexportHeaderLibHeaders []string

	ObjFiles []string

	GeneratedSources            []string
	GeneratedHeaders            []string
	DeviceFirstGeneratedHeaders []string
	GeneratedDeps               []string

	ReexportGeneratedHeaders []string

	CrtBegin, CrtEnd []string

	// Used for host bionic
	DynamicLinker string

	// List of libs that need to be excluded for APEX variant
	ExcludeLibsForApex []string
	// List of libs that need to be excluded for non-APEX variant
	ExcludeLibsForNonApex []string

	// LLNDK headers for the ABI checker to check LLNDK implementation library.
	// An LLNDK implementation is the core variant. LLNDK header libs are reexported by the vendor variant.
	// The core variant cannot depend on the vendor variant because of the order of imageTransitionMutator.Split().
	// Instead, the LLNDK implementation depends on the LLNDK header libs.
	LlndkHeaderLibs []string
}

// A struct which to collect flags for rlib dependencies
type RustRlibDep struct {
	LibPath   android.Path // path to the rlib
	LinkDirs  []string     // flags required for dependency (e.g. -L flags)
	CrateName string       // crateNames associated with rlibDeps
}

func EqRustRlibDeps(a RustRlibDep, b RustRlibDep) bool {
	return a.LibPath == b.LibPath
}

// PathDeps is a struct containing file paths to dependencies of a module.
// It's constructed in depsToPath() by traversing the direct dependencies of the current module.
// It's used to construct flags for various build statements (such as for compiling and linking).
// It is then passed to module decorator functions responsible for registering build statements
// (such as `module.compiler.compile()`).`
type PathDeps struct {
	// Paths to .so files
	SharedLibs, EarlySharedLibs, LateSharedLibs android.Paths
	// Paths to the dependencies to use for .so files (.so.toc files)
	SharedLibsDeps, EarlySharedLibsDeps, LateSharedLibsDeps android.Paths
	// Paths to .a files
	StaticLibs, LateStaticLibs, WholeStaticLibs android.Paths
	// Paths and crateNames for RustStaticLib dependencies
	RustRlibDeps []RustRlibDep

	// Transitive static library dependencies of static libraries for use in ordering.
	TranstiveStaticLibrariesForOrdering depset.DepSet[android.Path]

	// Paths to .o files
	Objs Objects
	// Paths to .o files in dependencies that provide them. Note that these lists
	// aren't complete since prebuilt modules don't provide the .o files.
	StaticLibObjs      Objects
	WholeStaticLibObjs Objects

	// Paths to .a files in prebuilts. Complements WholeStaticLibObjs to contain
	// the libs from all whole_static_lib dependencies.
	WholeStaticLibsFromPrebuilts android.Paths

	// Paths to generated source files
	GeneratedSources android.Paths
	GeneratedDeps    android.Paths

	Flags                      []string
	LdFlags                    []string
	IncludeDirs                android.Paths
	SystemIncludeDirs          android.Paths
	ReexportedDirs             android.Paths
	ReexportedSystemDirs       android.Paths
	ReexportedFlags            []string
	ReexportedGeneratedHeaders android.Paths
	ReexportedDeps             android.Paths
	ReexportedRustRlibDeps     []RustRlibDep

	// Paths to crt*.o files
	CrtBegin, CrtEnd android.Paths

	// Path to the dynamic linker binary
	DynamicLinker android.OptionalPath

	// For Darwin builds, the path to the second architecture's output that should
	// be combined with this architectures's output into a FAT MachO file.
	DarwinSecondArchOutput android.OptionalPath

	// Paths to direct srcs and transitive include dirs from direct aidl_library deps
	AidlLibraryInfos []aidl_library.AidlLibraryInfo

	// LLNDK headers for the ABI checker to check LLNDK implementation library.
	LlndkIncludeDirs       android.Paths
	LlndkSystemIncludeDirs android.Paths

	directImplementationDeps     android.Paths
	transitiveImplementationDeps []depset.DepSet[android.Path]
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

// Flags contains various types of command line flags (and settings) for use in building build
// statements related to C++.
type Flags struct {
	// Local flags (which individual modules are responsible for). These may override global flags.
	Local LocalOrGlobalFlags
	// Global flags (which build system or toolchain is responsible for).
	Global          LocalOrGlobalFlags
	NoOverrideFlags []string // Flags applied to the end of list of flags so they are not overridden

	aidlFlags     []string // Flags that apply to aidl source files
	rsFlags       []string // Flags that apply to renderscript source files
	libFlags      []string // Flags to add libraries early to the link order
	extraLibFlags []string // Flags to add libraries late in the link order after LdFlags
	TidyFlags     []string // Flags that apply to clang-tidy
	SAbiFlags     []string // Flags that apply to header-abi-dumper

	// Global include flags that apply to C, C++, and assembly source files
	// These must be after any module include flags, which will be in CommonFlags.
	SystemIncludeFlags []string

	Toolchain     config.Toolchain
	Tidy          bool // True if ninja .tidy rules should be generated.
	NeedTidyFiles bool // True if module link should depend on .tidy files
	GcovCoverage  bool // True if coverage files should be generated.
	SAbiDump      bool // True if header abi dumps should be generated.
	EmitXrefs     bool // If true, generate Ninja rules to generate emitXrefs input files for Kythe
	ClangVerify   bool // If true, append cflags "-Xclang -verify" and append "&& touch $out" to the clang command line.

	// The instruction set required for clang ("arm" or "thumb").
	RequiredInstructionSet string
	// The target-device system path to the dynamic linker.
	DynamicLinker string

	CFlagsDeps  android.Paths // Files depended on by compiler flags
	LdFlagsDeps android.Paths // Files depended on by linker flags

	// True if .s files should be processed with the c preprocessor.
	AssemblerWithCpp bool

	proto            android.ProtoFlags
	protoC           bool // Whether to use C instead of C++
	protoOptionsFile bool // Whether to look for a .options file next to the .proto

	Yacc *YaccProperties
	Lex  *LexProperties
}

// Properties used to compile all C or C++ modules
type BaseProperties struct {
	// Deprecated. true is the default, false is invalid.
	Clang *bool `android:"arch_variant"`

	// Aggresively trade performance for smaller binary size.
	// This should only be used for on-device binaries that are rarely executed and not
	// performance critical.
	Optimize_for_size *bool `android:"arch_variant"`

	// The API level that this module is built against. The APIs of this API level will be
	// visible at build time, but use of any APIs newer than min_sdk_version will render the
	// module unloadable on older devices.  In the future it will be possible to weakly-link new
	// APIs, making the behavior match Java: such modules will load on older devices, but
	// calling new APIs on devices that do not support them will result in a crash.
	//
	// This property has the same behavior as sdk_version does for Java modules. For those
	// familiar with Android Gradle, the property behaves similarly to how compileSdkVersion
	// does for Java code.
	//
	// In addition, setting this property causes two variants to be built, one for the platform
	// and one for apps.
	Sdk_version *string

	// Minimum OS API level supported by this C or C++ module. This property becomes the value
	// of the __ANDROID_API__ macro. When the C or C++ module is included in an APEX or an APK,
	// this property is also used to ensure that the min_sdk_version of the containing module is
	// not older (i.e. less) than this module's min_sdk_version. When not set, this property
	// defaults to the value of sdk_version.  When this is set to "apex_inherit", this tracks
	// min_sdk_version of the containing APEX. When the module
	// is not built for an APEX, "apex_inherit" defaults to sdk_version.
	Min_sdk_version *string

	// If true, always create an sdk variant and don't create a platform variant.
	Sdk_variant_only *bool

	AndroidMkSharedLibs      []string `blueprint:"mutated"`
	AndroidMkStaticLibs      []string `blueprint:"mutated"`
	AndroidMkRlibs           []string `blueprint:"mutated"`
	AndroidMkRuntimeLibs     []string `blueprint:"mutated"`
	AndroidMkWholeStaticLibs []string `blueprint:"mutated"`
	AndroidMkHeaderLibs      []string `blueprint:"mutated"`
	HideFromMake             bool     `blueprint:"mutated"`
	PreventInstall           bool     `blueprint:"mutated"`

	// Set by DepsMutator.
	AndroidMkSystemSharedLibs []string `blueprint:"mutated"`

	// The name of the image this module is built for
	ImageVariation string `blueprint:"mutated"`

	// The VNDK version this module is built against. If empty, the module is not
	// build against the VNDK.
	VndkVersion string `blueprint:"mutated"`

	// Suffix for the name of Android.mk entries generated by this module
	SubName string `blueprint:"mutated"`

	// *.logtags files, to combine together in order to generate the /system/etc/event-log-tags
	// file
	Logtags []string `android:"path"`

	// Make this module available when building for ramdisk.
	// On device without a dedicated recovery partition, the module is only
	// available after switching root into
	// /first_stage_ramdisk. To expose the module before switching root, install
	// the recovery variant instead.
	Ramdisk_available *bool

	// Make this module available when building for vendor ramdisk.
	// On device without a dedicated recovery partition, the module is only
	// available after switching root into
	// /first_stage_ramdisk. To expose the module before switching root, install
	// the recovery variant instead.
	Vendor_ramdisk_available *bool

	// Make this module available when building for recovery
	Recovery_available *bool

	// Used by imageMutator, set by ImageMutatorBegin()
	VendorVariantNeeded        bool `blueprint:"mutated"`
	ProductVariantNeeded       bool `blueprint:"mutated"`
	CoreVariantNeeded          bool `blueprint:"mutated"`
	RamdiskVariantNeeded       bool `blueprint:"mutated"`
	VendorRamdiskVariantNeeded bool `blueprint:"mutated"`
	RecoveryVariantNeeded      bool `blueprint:"mutated"`

	// A list of variations for the "image" mutator of the form
	//<image name> '.' <version char>, for example, 'vendor.S'
	ExtraVersionedImageVariations []string `blueprint:"mutated"`

	// Allows this module to use non-APEX version of libraries. Useful
	// for building binaries that are started before APEXes are activated.
	Bootstrap *bool

	// Allows this module to be included in CMake release snapshots to be built outside of Android
	// build system and source tree.
	Cmake_snapshot_supported *bool

	Installable *bool `android:"arch_variant"`

	// Set by factories of module types that can only be referenced from variants compiled against
	// the SDK.
	AlwaysSdk bool `blueprint:"mutated"`

	// Variant is an SDK variant created by sdkMutator
	IsSdkVariant bool `blueprint:"mutated"`
	// Set when both SDK and platform variants are exported to Make to trigger renaming the SDK
	// variant to have a ".sdk" suffix.
	SdkAndPlatformVariantVisibleToMake bool `blueprint:"mutated"`

	Target struct {
		Platform struct {
			// List of modules required by the core variant.
			Required []string `android:"arch_variant"`

			// List of modules not required by the core variant.
			Exclude_required []string `android:"arch_variant"`
		} `android:"arch_variant"`

		Recovery struct {
			// List of modules required by the recovery variant.
			Required []string `android:"arch_variant"`

			// List of modules not required by the recovery variant.
			Exclude_required []string `android:"arch_variant"`
		} `android:"arch_variant"`
	} `android:"arch_variant"`
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
	// The modules with vndk: {enabled: true} must define 'vendor_available'
	// to 'true'.
	//
	// Nothing happens if BOARD_VNDK_VERSION isn't set in the BoardConfig.mk
	Vendor_available *bool

	// This is the same as the "vendor_available" except that the install path
	// of the vendor variant is /odm or /vendor/odm.
	// By replacing "vendor_available: true" with "odm_available: true", the
	// module will install its vendor variant to the /odm partition or /vendor/odm.
	// As the modules with "odm_available: true" still create the vendor variants,
	// they can link to the other vendor modules as the vendor_available modules do.
	// Also, the vendor modules can link to odm_available modules.
	//
	// It may not be used for VNDK modules.
	Odm_available *bool

	// whether this module should be allowed to be directly depended by other
	// modules with `product_specific: true` or `product_available: true`.
	// If set to true, an additional product variant will be built separately
	// that is limited to the set of libraries and headers that are exposed to
	// /product modules.
	//
	// The product variant may be used with a different (newer) /system,
	// so it shouldn't have any unversioned runtime dependencies, or
	// make assumptions about the system that may not be true in the
	// future.
	//
	// If set to false, this module becomes inaccessible from /product modules.
	//
	// Different from the 'vendor_available' property, the modules with
	// vndk: {enabled: true} don't have to define 'product_available'. The VNDK
	// library without 'product_available' may not be depended on by any other
	// modules that has product variants including the product available VNDKs.
	//
	// Nothing happens if BOARD_VNDK_VERSION isn't set in the BoardConfig.mk
	// and PRODUCT_PRODUCT_VNDK_VERSION isn't set.
	Product_available *bool

	// whether this module is capable of being loaded with other instance
	// (possibly an older version) of the same module in the same process.
	// Currently, a shared library that is a member of VNDK (vndk: {enabled: true})
	// can be double loaded in a vendor process if the library is also a
	// (direct and indirect) dependency of an LLNDK library. Such libraries must be
	// explicitly marked as `double_loadable: true` by the owner, or the dependency
	// from the LLNDK lib should be cut if the lib is not designed to be double loaded.
	Double_loadable *bool

	// IsLLNDK is set to true for the vendor variant of a cc_library module that has LLNDK stubs.
	IsLLNDK bool `blueprint:"mutated"`

	// IsVendorPublicLibrary is set for the core and product variants of a library that has
	// vendor_public_library stubs.
	IsVendorPublicLibrary bool `blueprint:"mutated"`
}

// ModuleContextIntf is an interface (on a module context helper) consisting of functions related
// to understanding  details about the type of the current module.
// For example, one might call these functions to determine whether the current module is a static
// library and/or is installed in vendor directories.
type ModuleContextIntf interface {
	static() bool
	staticBinary() bool
	staticLibrary() bool
	testBinary() bool
	testLibrary() bool
	header() bool
	binary() bool
	object() bool
	toolchain() config.Toolchain
	useSdk() bool
	sdkVersion() string
	minSdkVersion() string
	isSdkVariant() bool
	useVndk() bool
	isNdk(config android.Config) bool
	IsLlndk() bool
	isImplementationForLLNDKPublic() bool
	IsVendorPublicLibrary() bool
	inProduct() bool
	inVendor() bool
	inRamdisk() bool
	inVendorRamdisk() bool
	inRecovery() bool
	InVendorOrProduct() bool
	selectedStl() string
	baseModuleName() string
	isAfdoCompile(ctx ModuleContext) bool
	isOrderfileCompile() bool
	isCfi() bool
	isFuzzer() bool
	isNDKStubLibrary() bool
	useClangLld(actx ModuleContext) bool
	apexVariationName() string
	bootstrap() bool
	nativeCoverage() bool
	isPreventInstall() bool
	isCfiAssemblySupportEnabled() bool
	getSharedFlags() *SharedFlags
	notInPlatform() bool
	optimizeForSize() bool
	getOrCreateMakeVarsInfo() *CcMakeVarsInfo
}

type SharedFlags struct {
	NumSharedFlags int
	FlagsMap       map[string]string
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

// feature represents additional (optional) steps to building cc-related modules, such as invocation
// of clang-tidy.
type feature interface {
	flags(ctx ModuleContext, flags Flags) Flags
	props() []interface{}
}

// Information returned from Generator about the source code it's generating
type GeneratedSource struct {
	IncludeDirs    android.Paths
	Sources        android.Paths
	Headers        android.Paths
	ReexportedDirs android.Paths
}

// generator allows injection of generated code
type Generator interface {
	GeneratorProps() []interface{}
	GeneratorInit(ctx BaseModuleContext)
	GeneratorDeps(ctx DepsContext, deps Deps) Deps
	GeneratorFlags(ctx ModuleContext, flags Flags, deps PathDeps) Flags
	GeneratorSources(ctx ModuleContext) GeneratedSource
	GeneratorBuildActions(ctx ModuleContext, flags Flags, deps PathDeps)
}

// compiler is the interface for a compiler helper object. Different module decorators may implement
// this helper differently.
type compiler interface {
	compilerInit(ctx BaseModuleContext)
	compilerDeps(ctx DepsContext, deps Deps) Deps
	compilerFlags(ctx ModuleContext, flags Flags, deps PathDeps) Flags
	compilerProps() []interface{}
	baseCompilerProps() BaseCompilerProperties

	appendCflags([]string)
	appendAsflags([]string)
	compile(ctx ModuleContext, flags Flags, deps PathDeps) Objects
}

// linker is the interface for a linker decorator object. Individual module types can provide
// their own implementation for this decorator, and thus specify custom logic regarding build
// statements pertaining to linking.
type linker interface {
	linkerInit(ctx BaseModuleContext)
	linkerDeps(ctx DepsContext, deps Deps) Deps
	linkerFlags(ctx ModuleContext, flags Flags) Flags
	linkerProps() []interface{}
	baseLinkerProps() BaseLinkerProperties
	useClangLld(actx ModuleContext) bool

	link(ctx ModuleContext, flags Flags, deps PathDeps, objs Objects) android.Path
	appendLdflags([]string)
	unstrippedOutputFilePath() android.Path
	strippedAllOutputFilePath() android.Path

	nativeCoverage() bool
	coverageOutputFilePath() android.OptionalPath

	// Get the deps that have been explicitly specified in the properties.
	linkerSpecifiedDeps(ctx android.ConfigurableEvaluatorContext, module *Module, specifiedDeps specifiedDeps) specifiedDeps

	// Gets a list of files that will be disted when using the dist property without specifying
	// an output file tag.
	defaultDistFiles() []android.Path

	moduleInfoJSON(ctx ModuleContext, moduleInfoJSON *android.ModuleInfoJSON)

	testSuiteInfo(ctx ModuleContext)
}

// specifiedDeps is a tuple struct representing dependencies of a linked binary owned by the linker.
type specifiedDeps struct {
	sharedLibs []string
	// Note nil and [] are semantically distinct. [] prevents linking against the defaults (usually
	// libc, libm, etc.)
	systemSharedLibs []string
}

// installer is the interface for an installer helper object. This helper is responsible for
// copying build outputs to the appropriate locations so that they may be installed on device.
type installer interface {
	installerProps() []interface{}
	install(ctx ModuleContext, path android.Path)
	everInstallable() bool
	inData() bool
	inSanitizerDir() bool
	hostToolPath() android.OptionalPath
	relativeInstallPath() string
	makeUninstallable(mod *Module)
	installInRoot() bool
}

type overridable interface {
	overriddenModules() []string
}

type libraryDependencyKind int

const (
	headerLibraryDependency = iota
	sharedLibraryDependency
	staticLibraryDependency
	rlibLibraryDependency
)

func (k libraryDependencyKind) String() string {
	switch k {
	case headerLibraryDependency:
		return "headerLibraryDependency"
	case sharedLibraryDependency:
		return "sharedLibraryDependency"
	case staticLibraryDependency:
		return "staticLibraryDependency"
	case rlibLibraryDependency:
		return "rlibLibraryDependency"
	default:
		panic(fmt.Errorf("unknown libraryDependencyKind %d", k))
	}
}

type libraryDependencyOrder int

const (
	earlyLibraryDependency  = -1
	normalLibraryDependency = 0
	lateLibraryDependency   = 1
)

func (o libraryDependencyOrder) String() string {
	switch o {
	case earlyLibraryDependency:
		return "earlyLibraryDependency"
	case normalLibraryDependency:
		return "normalLibraryDependency"
	case lateLibraryDependency:
		return "lateLibraryDependency"
	default:
		panic(fmt.Errorf("unknown libraryDependencyOrder %d", o))
	}
}

// libraryDependencyTag is used to tag dependencies on libraries.  Unlike many dependency
// tags that have a set of predefined tag objects that are reused for each dependency, a
// libraryDependencyTag is designed to contain extra metadata and is constructed as needed.
// That means that comparing a libraryDependencyTag for equality will only be equal if all
// of the metadata is equal.  Most usages will want to type assert to libraryDependencyTag and
// then check individual metadata fields instead.
type libraryDependencyTag struct {
	blueprint.BaseDependencyTag

	// These are exported so that fmt.Printf("%#v") can call their String methods.
	Kind  libraryDependencyKind
	Order libraryDependencyOrder

	wholeStatic bool

	reexportFlags       bool
	explicitlyVersioned bool
	explicitlyImpl      bool
	dataLib             bool
	ndk                 bool

	staticUnwinder bool

	makeSuffix string

	// Whether or not this dependency should skip the apex dependency check
	skipApexAllowedDependenciesCheck bool

	// Whether or not this dependency has to be followed for the apex variants
	excludeInApex bool
	// Whether or not this dependency has to be followed for the non-apex variants
	excludeInNonApex bool

	// If true, don't automatically export symbols from the static library into a shared library.
	unexportedSymbols bool
}

// header returns true if the libraryDependencyTag is tagging a header lib dependency.
func (d libraryDependencyTag) header() bool {
	return d.Kind == headerLibraryDependency
}

// shared returns true if the libraryDependencyTag is tagging a shared lib dependency.
func (d libraryDependencyTag) shared() bool {
	return d.Kind == sharedLibraryDependency
}

// shared returns true if the libraryDependencyTag is tagging a static lib dependency.
func (d libraryDependencyTag) static() bool {
	return d.Kind == staticLibraryDependency
}

func (d libraryDependencyTag) LicenseAnnotations() []android.LicenseAnnotation {
	if d.shared() {
		return []android.LicenseAnnotation{android.LicenseAnnotationSharedDependency}
	}
	return nil
}

var _ android.LicenseAnnotationsDependencyTag = libraryDependencyTag{}

// InstallDepNeeded returns true for shared libraries so that shared library dependencies of
// binaries or other shared libraries are installed as dependencies.
func (d libraryDependencyTag) InstallDepNeeded() bool {
	return d.shared()
}

var _ android.InstallNeededDependencyTag = libraryDependencyTag{}

func (d libraryDependencyTag) PropagateAconfigValidation() bool {
	return d.static()
}

var _ android.PropagateAconfigValidationDependencyTag = libraryDependencyTag{}

// dependencyTag is used for tagging miscellaneous dependency types that don't fit into
// libraryDependencyTag.  Each tag object is created globally and reused for multiple
// dependencies (although since the object contains no references, assigning a tag to a
// variable and modifying it will not modify the original).  Users can compare the tag
// returned by ctx.OtherModuleDependencyTag against the global original
type dependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

// installDependencyTag is used for tagging miscellaneous dependency types that don't fit into
// libraryDependencyTag, but where the dependency needs to be installed when the parent is
// installed.
type installDependencyTag struct {
	blueprint.BaseDependencyTag
	android.InstallAlwaysNeededDependencyTag
	name string
}

var (
	genSourceDepTag       = dependencyTag{name: "gen source"}
	genHeaderDepTag       = dependencyTag{name: "gen header"}
	genHeaderExportDepTag = dependencyTag{name: "gen header export"}
	objDepTag             = dependencyTag{name: "obj"}
	dynamicLinkerDepTag   = installDependencyTag{name: "dynamic linker"}
	reuseObjTag           = dependencyTag{name: "reuse objects"}
	staticVariantTag      = dependencyTag{name: "static variant"}
	vndkExtDepTag         = dependencyTag{name: "vndk extends"}
	dataLibDepTag         = dependencyTag{name: "data lib"}
	dataBinDepTag         = dependencyTag{name: "data bin"}
	runtimeDepTag         = installDependencyTag{name: "runtime lib"}
	StubImplDepTag        = dependencyTag{name: "stub_impl"}
	JniFuzzLibTag         = dependencyTag{name: "jni_fuzz_lib_tag"}
	FdoProfileTag         = dependencyTag{name: "fdo_profile"}
	aidlLibraryTag        = dependencyTag{name: "aidl_library"}
	llndkHeaderLibTag     = dependencyTag{name: "llndk_header_lib"}
)

func IsExplicitImplSharedDepTag(depTag blueprint.DependencyTag) bool {
	ccLibDepTag, ok := depTag.(libraryDependencyTag)
	return ok && ccLibDepTag.shared() && ccLibDepTag.explicitlyImpl
}

func IsSharedDepTag(depTag blueprint.DependencyTag) bool {
	ccLibDepTag, ok := depTag.(libraryDependencyTag)
	return ok && ccLibDepTag.shared()
}

func IsStaticDepTag(depTag blueprint.DependencyTag) bool {
	ccLibDepTag, ok := depTag.(libraryDependencyTag)
	return ok && ccLibDepTag.static()
}

func IsHeaderDepTag(depTag blueprint.DependencyTag) bool {
	ccLibDepTag, ok := depTag.(libraryDependencyTag)
	return ok && ccLibDepTag.header()
}

func IsRuntimeDepTag(depTag blueprint.DependencyTag) bool {
	return depTag == runtimeDepTag
}

func ExcludeInApexDepTag(depTag blueprint.DependencyTag) bool {
	ccLibDepTag, ok := depTag.(libraryDependencyTag)
	return ok && ccLibDepTag.excludeInApex
}

// Module contains the properties and members used by all C/C++ module types, and implements
// the blueprint.Module interface.  It delegates to compiler, linker, and installer interfaces
// to construct the output file.  Behavior can be customized with a Customizer, or "decorator",
// interface.
//
// To define a C/C++ related module, construct a new Module object and point its delegates to
// type-specific structs. These delegates will be invoked to register module-specific build
// statements which may be unique to the module type. For example, module.compiler.compile() should
// be defined so as to register build statements which are responsible for compiling the module.
//
// Another example: to construct a cc_binary module, one can create a `cc.binaryDecorator` struct
// which implements the `linker` and `installer` interfaces, and points the `linker` and `installer`
// members of the cc.Module to this decorator. Thus, a cc_binary module has custom linker and
// installer logic.
type Module struct {
	fuzz.FuzzModule

	VendorProperties VendorProperties
	Properties       BaseProperties
	sourceProperties android.SourceProperties

	// initialize before calling Init
	hod         android.HostOrDeviceSupported
	multilib    android.Multilib
	testModule  bool
	incremental bool

	// Allowable SdkMemberTypes of this module type.
	sdkMemberTypes []android.SdkMemberType

	// decorator delegates, initialize before calling Init
	// these may contain module-specific implementations, and effectively allow for custom
	// type-specific logic. These members may reference different objects or the same object.
	// Functions of these decorators will be invoked to initialize and register type-specific
	// build statements.
	generators []Generator
	compiler   compiler
	linker     linker
	installer  installer

	features  []feature
	stl       *stl
	sanitize  *sanitize
	coverage  *coverage
	fuzzer    *fuzzer
	sabi      *sabi
	lto       *lto
	afdo      *afdo
	orderfile *orderfile

	library libraryInterface

	outputFile android.OptionalPath

	cachedToolchain config.Toolchain

	subAndroidMkOnce map[subAndroidMkProviderInfoProducer]bool

	// Flags used to compile this module
	flags Flags

	// Shared flags among build rules of this module
	sharedFlags SharedFlags

	// only non-nil when this is a shared library that reuses the objects of a static library
	staticAnalogue *StaticLibraryInfo

	makeLinkType string

	// For apex variants, this is set as apex.min_sdk_version
	apexSdkVersion android.ApiLevel

	hideApexVariantFromMake bool

	logtagsPaths android.Paths

	WholeRustStaticlib bool

	hasAidl         bool
	hasLex          bool
	hasProto        bool
	hasRenderscript bool
	hasSysprop      bool
	hasWinMsg       bool
	hasYacc         bool

	makeVarsInfo *CcMakeVarsInfo
}

func (c *Module) IncrementalSupported() bool {
	return c.incremental
}

var _ blueprint.Incremental = (*Module)(nil)

func (c *Module) AddJSONData(d *map[string]interface{}) {
	c.AndroidModuleBase().AddJSONData(d)
	(*d)["Cc"] = map[string]interface{}{
		"SdkVersion":             c.SdkVersion(),
		"MinSdkVersion":          c.MinSdkVersion(),
		"VndkVersion":            c.VndkVersion(),
		"ProductSpecific":        c.ProductSpecific(),
		"SocSpecific":            c.SocSpecific(),
		"DeviceSpecific":         c.DeviceSpecific(),
		"InProduct":              c.InProduct(),
		"InVendor":               c.InVendor(),
		"InRamdisk":              c.InRamdisk(),
		"InVendorRamdisk":        c.InVendorRamdisk(),
		"InRecovery":             c.InRecovery(),
		"VendorAvailable":        c.VendorAvailable(),
		"ProductAvailable":       c.ProductAvailable(),
		"RamdiskAvailable":       c.RamdiskAvailable(),
		"VendorRamdiskAvailable": c.VendorRamdiskAvailable(),
		"RecoveryAvailable":      c.RecoveryAvailable(),
		"OdmAvailable":           c.OdmAvailable(),
		"InstallInData":          c.InstallInData(),
		"InstallInRamdisk":       c.InstallInRamdisk(),
		"InstallInSanitizerDir":  c.InstallInSanitizerDir(),
		"InstallInVendorRamdisk": c.InstallInVendorRamdisk(),
		"InstallInRecovery":      c.InstallInRecovery(),
		"InstallInRoot":          c.InstallInRoot(),
		"IsLlndk":                c.IsLlndk(),
		"IsVendorPublicLibrary":  c.IsVendorPublicLibrary(),
		"ApexSdkVersion":         c.apexSdkVersion,
		"AidlSrcs":               c.hasAidl,
		"LexSrcs":                c.hasLex,
		"ProtoSrcs":              c.hasProto,
		"RenderscriptSrcs":       c.hasRenderscript,
		"SyspropSrcs":            c.hasSysprop,
		"WinMsgSrcs":             c.hasWinMsg,
		"YaccSrsc":               c.hasYacc,
		"OnlyCSrcs":              !(c.hasAidl || c.hasLex || c.hasProto || c.hasRenderscript || c.hasSysprop || c.hasWinMsg || c.hasYacc),
		"OptimizeForSize":        c.OptimizeForSize(),
	}
}

func (c *Module) SetPreventInstall() {
	c.Properties.PreventInstall = true
}

func (c *Module) SetHideFromMake() {
	c.Properties.HideFromMake = true
}

func (c *Module) HiddenFromMake() bool {
	return c.Properties.HideFromMake
}

func (c *Module) RequiredModuleNames(ctx android.ConfigurableEvaluatorContext) []string {
	required := android.CopyOf(c.ModuleBase.RequiredModuleNames(ctx))
	if c.ImageVariation().Variation == android.CoreVariation {
		required = append(required, c.Properties.Target.Platform.Required...)
		required = removeListFromList(required, c.Properties.Target.Platform.Exclude_required)
	} else if c.InRecovery() {
		required = append(required, c.Properties.Target.Recovery.Required...)
		required = removeListFromList(required, c.Properties.Target.Recovery.Exclude_required)
	}
	return android.FirstUniqueStrings(required)
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
			return stub.apiLevel.String()
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

func (c *Module) StubDecorator() bool {
	if _, ok := c.linker.(*stubDecorator); ok {
		return true
	}
	return false
}

func (c *Module) OptimizeForSize() bool {
	return Bool(c.Properties.Optimize_for_size)
}

func (c *Module) SdkVersion() string {
	return String(c.Properties.Sdk_version)
}

func (c *Module) MinSdkVersion() string {
	return String(c.Properties.Min_sdk_version)
}

func (c *Module) SetSdkVersion(s string) {
	c.Properties.Sdk_version = StringPtr(s)
}

func (c *Module) SetMinSdkVersion(s string) {
	c.Properties.Min_sdk_version = StringPtr(s)
}

func (c *Module) SetStl(s string) {
	if c.stl != nil {
		c.stl.Properties.Stl = StringPtr(s)
	}
}

func (c *Module) IsCrt() bool {
	if linker, ok := c.linker.(*objectLinker); ok {
		return linker.isCrt()
	}
	return false
}

func (c *Module) SplitPerApiLevel() bool {
	return CanUseSdk(c) && c.IsCrt()
}

func (c *Module) AlwaysSdk() bool {
	return c.Properties.AlwaysSdk || Bool(c.Properties.Sdk_variant_only)
}

func (c *Module) CcLibrary() bool {
	if c.linker != nil {
		if _, ok := c.linker.(*libraryDecorator); ok {
			return true
		}
		if _, ok := c.linker.(*prebuiltLibraryLinker); ok {
			return true
		}
	}
	return false
}

func (c *Module) CcLibraryInterface() bool {
	if c.library != nil {
		return true
	}
	return false
}

func (c *Module) RlibStd() bool {
	panic(fmt.Errorf("RlibStd called on non-Rust module: %q", c.BaseModuleName()))
}

func (c *Module) RustLibraryInterface() bool {
	return false
}

func (c *Module) CrateName() string {
	panic(fmt.Errorf("CrateName called on non-Rust module: %q", c.BaseModuleName()))
}

func (c *Module) ExportedCrateLinkDirs() []string {
	panic(fmt.Errorf("ExportedCrateLinkDirs called on non-Rust module: %q", c.BaseModuleName()))
}

func (c *Module) IsFuzzModule() bool {
	if _, ok := c.compiler.(*fuzzBinary); ok {
		return true
	}
	return false
}

func (c *Module) FuzzModuleStruct() fuzz.FuzzModule {
	return c.FuzzModule
}

func (c *Module) FuzzPackagedModule() fuzz.FuzzPackagedModule {
	if fuzzer, ok := c.compiler.(*fuzzBinary); ok {
		return fuzzer.fuzzPackagedModule
	}
	panic(fmt.Errorf("FuzzPackagedModule called on non-fuzz module: %q", c.BaseModuleName()))
}

func (c *Module) FuzzSharedLibraries() android.RuleBuilderInstalls {
	if fuzzer, ok := c.compiler.(*fuzzBinary); ok {
		return fuzzer.sharedLibraries
	}
	panic(fmt.Errorf("FuzzSharedLibraries called on non-fuzz module: %q", c.BaseModuleName()))
}

func (c *Module) NonCcVariants() bool {
	return false
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

func (c *Module) BuildRlibVariant() bool {
	// cc modules can never build rlib variants
	return false
}

func (c *Module) Module() android.Module {
	return c
}

func (c *Module) OutputFile() android.OptionalPath {
	return c.outputFile
}

func (c *Module) CoverageFiles() android.Paths {
	if c.linker != nil {
		if library, ok := c.linker.(libraryInterface); ok {
			return library.objs().coverageFiles
		}
	}
	panic(fmt.Errorf("CoverageFiles called on non-library module: %q", c.BaseModuleName()))
}

var _ LinkableInterface = (*Module)(nil)

func (c *Module) VersionedInterface() VersionedInterface {
	if c.library != nil {
		return c.library
	}
	return nil
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

func (c *Module) VndkVersion() string {
	return c.Properties.VndkVersion
}

func (c *Module) Init() android.Module {
	c.AddProperties(&c.Properties, &c.VendorProperties)
	for _, generator := range c.generators {
		c.AddProperties(generator.GeneratorProps()...)
	}
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
	if c.fuzzer != nil {
		c.AddProperties(c.fuzzer.props()...)
	}
	if c.sabi != nil {
		c.AddProperties(c.sabi.props()...)
	}
	if c.lto != nil {
		c.AddProperties(c.lto.props()...)
	}
	if c.afdo != nil {
		c.AddProperties(c.afdo.props()...)
	}
	if c.orderfile != nil {
		c.AddProperties(c.orderfile.props()...)
	}
	for _, feature := range c.features {
		c.AddProperties(feature.props()...)
	}
	// Allow test-only on libraries that are not cc_test_library
	if c.library != nil && !c.testLibrary() {
		c.AddProperties(&c.sourceProperties)
	}

	android.InitAndroidArchModule(c, c.hod, c.multilib)
	android.InitApexModule(c)
	android.InitDefaultableModule(c)

	return c
}

// UseVndk() returns true if this module is built against VNDK.
// This means the vendor and product variants of a module.
func (c *Module) UseVndk() bool {
	return c.Properties.VndkVersion != ""
}

func CanUseSdk(c LinkableInterface) bool {
	return c.Module().Target().Os == android.Android && c.Target().NativeBridge == android.NativeBridgeDisabled &&
		!c.InVendorOrProduct() && !c.InRamdisk() && !c.InRecovery() && !c.InVendorRamdisk()
}

func (c *Module) UseSdk() bool {
	if CanUseSdk(c) {
		return String(c.Properties.Sdk_version) != ""
	}
	return false
}

func (c *Module) isCoverageVariant() bool {
	return c.coverage.Properties.IsCoverageVariant
}

func (c *Module) IsNdk(config android.Config) bool {
	return inList(c.BaseModuleName(), *getNDKKnownLibs(config))
}

func (c *Module) IsLlndk() bool {
	return c.VendorProperties.IsLLNDK
}

func (m *Module) NeedsLlndkVariants() bool {
	lib := moduleVersionedInterface(m)
	return lib != nil && (lib.HasLLNDKStubs() || lib.HasLLNDKHeaders())
}

func (m *Module) NeedsVendorPublicLibraryVariants() bool {
	lib := moduleVersionedInterface(m)
	return lib != nil && (lib.HasVendorPublicLibrary())
}

// IsVendorPublicLibrary returns true for vendor public libraries.
func (c *Module) IsVendorPublicLibrary() bool {
	return c.VendorProperties.IsVendorPublicLibrary
}

func (c *Module) IsVndkPrebuiltLibrary() bool {
	if _, ok := c.linker.(*vndkPrebuiltLibraryDecorator); ok {
		return true
	}
	return false
}

func (c *Module) SdkAndPlatformVariantVisibleToMake() bool {
	return c.Properties.SdkAndPlatformVariantVisibleToMake
}

func (c *Module) HasLlndkStubs() bool {
	lib := moduleVersionedInterface(c)
	return lib != nil && lib.HasLLNDKStubs()
}

func (c *Module) StubsVersion() string {
	if lib, ok := c.linker.(VersionedInterface); ok {
		return lib.StubsVersion()
	}
	panic(fmt.Errorf("StubsVersion called on non-versioned module: %q", c.BaseModuleName()))
}

// isImplementationForLLNDKPublic returns true for any variant of a cc_library that has LLNDK stubs
// and does not set llndk.vendor_available: false.
func (c *Module) isImplementationForLLNDKPublic() bool {
	library, _ := c.library.(*libraryDecorator)
	return library != nil && library.HasLLNDKStubs() &&
		!Bool(library.Properties.Llndk.Private)
}

func (c *Module) isAfdoCompile(ctx ModuleContext) bool {
	if afdo := c.afdo; afdo != nil {
		return afdo.isAfdoCompile(ctx)
	}
	return false
}

func (c *Module) isOrderfileCompile() bool {
	if orderfile := c.orderfile; orderfile != nil {
		return orderfile.Properties.OrderfileLoad
	}
	return false
}

func (c *Module) isCfi() bool {
	return c.sanitize.isSanitizerEnabled(cfi)
}

func (c *Module) isFuzzer() bool {
	return c.sanitize.isSanitizerEnabled(Fuzzer)
}

func (c *Module) isNDKStubLibrary() bool {
	if _, ok := c.compiler.(*stubDecorator); ok {
		return true
	}
	return false
}

func (c *Module) SubName() string {
	return c.Properties.SubName
}

func (c *Module) IsStubs() bool {
	if lib := c.library; lib != nil {
		return lib.BuildStubs()
	}
	return false
}

func (c *Module) HasStubsVariants() bool {
	if lib := c.library; lib != nil {
		return lib.HasStubsVariants()
	}
	return false
}

func (c *Module) RustApexExclude() bool {
	return false
}

func (c *Module) IsStubsImplementationRequired() bool {
	if lib := c.library; lib != nil {
		return lib.IsStubsImplementationRequired()
	}
	return false
}

// If this is a stubs library, ImplementationModuleName returns the name of the module that contains
// the implementation.  If it is an implementation library it returns its own name.
func (c *Module) ImplementationModuleName(ctx android.BaseModuleContext) string {
	name := ctx.OtherModuleName(c)
	if versioned, ok := c.linker.(VersionedInterface); ok {
		name = versioned.ImplementationModuleName(name)
	}
	return name
}

// Similar to ImplementationModuleNameByCtx, but uses the Make variant of the module
// name as base name, for use in AndroidMk output. E.g. for a prebuilt module
// where the Soong name is prebuilt_foo, this returns foo (which works in Make
// under the premise that the prebuilt module overrides its source counterpart
// if it is exposed to Make).
func (c *Module) ImplementationModuleNameForMake() string {
	name := c.BaseModuleName()
	if versioned, ok := c.linker.(VersionedInterface); ok {
		name = versioned.ImplementationModuleName(name)
	}
	return name
}

func (c *Module) Bootstrap() bool {
	return Bool(c.Properties.Bootstrap)
}

func (c *Module) nativeCoverage() bool {
	// Bug: http://b/137883967 - native-bridge modules do not currently work with coverage
	if c.Target().NativeBridge == android.NativeBridgeEnabled {
		return false
	}
	return c.linker != nil && c.linker.nativeCoverage()
}

func (c *Module) IsSnapshotPrebuilt() bool {
	if p, ok := c.linker.(SnapshotInterface); ok {
		return p.IsSnapshotPrebuilt()
	}
	return false
}

func isBionic(name string) bool {
	switch name {
	case "libc", "libm", "libdl", "libdl_android", "linker":
		return true
	}
	return false
}

func InstallToBootstrap(name string, config android.Config) bool {
	if name == "libclang_rt.hwasan" || name == "libc_hwasan" {
		return true
	}
	return isBionic(name)
}

func (c *Module) isCfiAssemblySupportEnabled() bool {
	return c.sanitize != nil &&
		Bool(c.sanitize.Properties.Sanitize.Config.Cfi_assembly_support)
}

func (c *Module) InstallInRoot() bool {
	return c.installer != nil && c.installer.installInRoot()
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

func (ctx *moduleContextImpl) staticLibrary() bool {
	return ctx.mod.staticLibrary()
}

func (ctx *moduleContextImpl) testBinary() bool {
	return ctx.mod.testBinary()
}

func (ctx *moduleContextImpl) testLibrary() bool {
	return ctx.mod.testLibrary()
}

func (ctx *moduleContextImpl) header() bool {
	return ctx.mod.Header()
}

func (ctx *moduleContextImpl) binary() bool {
	return ctx.mod.Binary()
}

func (ctx *moduleContextImpl) object() bool {
	return ctx.mod.Object()
}

func (ctx *moduleContextImpl) optimizeForSize() bool {
	return ctx.mod.OptimizeForSize()
}

func (ctx *moduleContextImpl) useSdk() bool {
	return ctx.mod.UseSdk()
}

func (ctx *moduleContextImpl) sdkVersion() string {
	if ctx.ctx.Device() {
		return String(ctx.mod.Properties.Sdk_version)
	}
	return ""
}

func MinSdkVersion(mod VersionedLinkableInterface, ctxIsForPlatform bool, device bool,
	platformSdkVersion string) string {

	ver := mod.MinSdkVersion()
	if ver == "apex_inherit" && !ctxIsForPlatform {
		ver = mod.ApexSdkVersion().String()
	}
	if ver == "apex_inherit" || ver == "" {
		ver = mod.SdkVersion()
	}

	if device {
		// When building for vendor/product, use the latest _stable_ API as "current".
		// This is passed to clang/aidl compilers so that compiled/generated code works
		// with the system.
		if (mod.InVendor() || mod.InProduct()) && (ver == "" || ver == "current") {
			ver = platformSdkVersion
		}
	}

	// For crt objects, the meaning of min_sdk_version is very different from other types of
	// module. For them, min_sdk_version defines the oldest version that the build system will
	// create versioned variants for. For example, if min_sdk_version is 16, then sdk variant of
	// the crt object has local variants of 16, 17, ..., up to the latest version. sdk_version
	// and min_sdk_version properties of the variants are set to the corresponding version
	// numbers. However, the non-sdk variant (for apex or platform) of the crt object is left
	// untouched.  min_sdk_version: 16 doesn't actually mean that the non-sdk variant has to
	// support such an old version. The version is set to the later version in case when the
	// non-sdk variant is for the platform, or the min_sdk_version of the containing APEX if
	// it's for an APEX.
	if mod.IsCrt() && !mod.IsSdkVariant() {
		if ctxIsForPlatform {
			ver = strconv.Itoa(android.FutureApiLevelInt)
		} else { // for apex
			ver = mod.ApexSdkVersion().String()
			if ver == "" { // in case when min_sdk_version was not set by the APEX
				ver = mod.SdkVersion()
			}
		}
	}

	// Also make sure that minSdkVersion is not greater than sdkVersion, if they are both numbers
	sdkVersionInt, err := strconv.Atoi(mod.SdkVersion())
	minSdkVersionInt, err2 := strconv.Atoi(ver)
	if err == nil && err2 == nil {
		if sdkVersionInt < minSdkVersionInt {
			return strconv.Itoa(sdkVersionInt)
		}
	}
	return ver
}

func (ctx *moduleContextImpl) minSdkVersion() string {
	platformSdkVersion := ""
	if ctx.ctx.Device() {
		platformSdkVersion = ctx.ctx.Config().PlatformSdkVersion().String()
	}
	return MinSdkVersion(ctx.mod, CtxIsForPlatform(ctx.ctx), ctx.ctx.Device(), platformSdkVersion)
}

func (ctx *moduleContextImpl) isSdkVariant() bool {
	return ctx.mod.IsSdkVariant()
}

func (ctx *moduleContextImpl) useVndk() bool {
	return ctx.mod.UseVndk()
}

func (ctx *moduleContextImpl) InVendorOrProduct() bool {
	return ctx.mod.InVendorOrProduct()
}

func (ctx *moduleContextImpl) isNdk(config android.Config) bool {
	return ctx.mod.IsNdk(config)
}

func (ctx *moduleContextImpl) IsLlndk() bool {
	return ctx.mod.IsLlndk()
}

func (ctx *moduleContextImpl) isImplementationForLLNDKPublic() bool {
	return ctx.mod.isImplementationForLLNDKPublic()
}

func (ctx *moduleContextImpl) isAfdoCompile(mctx ModuleContext) bool {
	return ctx.mod.isAfdoCompile(mctx)
}

func (ctx *moduleContextImpl) isOrderfileCompile() bool {
	return ctx.mod.isOrderfileCompile()
}

func (ctx *moduleContextImpl) isCfi() bool {
	return ctx.mod.isCfi()
}

func (ctx *moduleContextImpl) isFuzzer() bool {
	return ctx.mod.isFuzzer()
}

func (ctx *moduleContextImpl) isNDKStubLibrary() bool {
	return ctx.mod.isNDKStubLibrary()
}

func (ctx *moduleContextImpl) IsVendorPublicLibrary() bool {
	return ctx.mod.IsVendorPublicLibrary()
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
	return ctx.mod.BaseModuleName()
}

func CtxIsForPlatform(ctx android.BaseModuleContext) bool {
	apexInfo, _ := android.ModuleProvider(ctx, android.ApexInfoProvider)
	return apexInfo.IsForPlatform()
}

func (ctx *moduleContextImpl) apexVariationName() string {
	apexInfo, _ := android.ModuleProvider(ctx.ctx, android.ApexInfoProvider)
	return apexInfo.ApexVariationName
}

func (ctx *moduleContextImpl) bootstrap() bool {
	return ctx.mod.Bootstrap()
}

func (ctx *moduleContextImpl) nativeCoverage() bool {
	return ctx.mod.nativeCoverage()
}

func (ctx *moduleContextImpl) isPreventInstall() bool {
	return ctx.mod.Properties.PreventInstall
}

func (ctx *moduleContextImpl) getSharedFlags() *SharedFlags {
	shared := &ctx.mod.sharedFlags
	if shared.FlagsMap == nil {
		shared.NumSharedFlags = 0
		shared.FlagsMap = make(map[string]string)
	}
	return shared
}

func (ctx *moduleContextImpl) isCfiAssemblySupportEnabled() bool {
	return ctx.mod.isCfiAssemblySupportEnabled()
}

func (ctx *moduleContextImpl) notInPlatform() bool {
	return ctx.mod.NotInPlatform()
}

func (ctx *moduleContextImpl) getOrCreateMakeVarsInfo() *CcMakeVarsInfo {
	if ctx.mod.makeVarsInfo == nil {
		ctx.mod.makeVarsInfo = &CcMakeVarsInfo{}
	}
	return ctx.mod.makeVarsInfo
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
	module.fuzzer = &fuzzer{}
	module.sabi = &sabi{}
	module.lto = &lto{}
	module.afdo = &afdo{}
	module.orderfile = &orderfile{}
	return module
}

func (c *Module) Prebuilt() *android.Prebuilt {
	if p, ok := c.linker.(prebuiltLinkerInterface); ok {
		return p.prebuilt()
	}
	return nil
}

func (c *Module) IsPrebuilt() bool {
	return c.Prebuilt() != nil
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

func (c *Module) Multilib() string {
	return c.Arch().ArchType.Multilib
}

func (c *Module) ApexSdkVersion() android.ApiLevel {
	return c.apexSdkVersion
}

func (c *Module) Symlinks() []string {
	if p, ok := c.installer.(interface {
		symlinkList() []string
	}); ok {
		return p.symlinkList()
	}
	return nil
}

func (c *Module) DataPaths() []android.DataPath {
	if p, ok := c.installer.(interface {
		dataPaths() []android.DataPath
	}); ok {
		return p.dataPaths()
	}
	return nil
}

func getNameSuffixWithVndkVersion(ctx android.ModuleContext, c LinkableInterface) string {
	// Returns the name suffix for product and vendor variants. If the VNDK version is not
	// "current", it will append the VNDK version to the name suffix.
	var nameSuffix string
	if c.InProduct() {
		if c.ProductSpecific() {
			// If the module is product specific with 'product_specific: true',
			// do not add a name suffix because it is a base module.
			return ""
		}
		return ProductSuffix
	} else {
		nameSuffix = VendorSuffix
	}
	if c.VndkVersion() != "" {
		// add version suffix only if the module is using different vndk version than the
		// version in product or vendor partition.
		nameSuffix += "." + c.VndkVersion()
	}
	return nameSuffix
}

func GetSubnameProperty(actx android.ModuleContext, c LinkableInterface) string {
	var subName = ""

	if c.Target().NativeBridge == android.NativeBridgeEnabled {
		subName += NativeBridgeSuffix
	}

	llndk := c.IsLlndk()
	if llndk || (c.InVendorOrProduct() && c.HasNonSystemVariants()) {
		// .vendor.{version} suffix is added for vendor variant or .product.{version} suffix is
		// added for product variant only when we have vendor and product variants with core
		// variant. The suffix is not added for vendor-only or product-only module.
		subName += getNameSuffixWithVndkVersion(actx, c)
	} else if c.IsVendorPublicLibrary() {
		subName += vendorPublicLibrarySuffix
	} else if c.IsVndkPrebuiltLibrary() {
		// .vendor suffix is added for backward compatibility with VNDK snapshot whose names with
		// such suffixes are already hard-coded in prebuilts/vndk/.../Android.bp.
		subName += VendorSuffix
	} else if c.InRamdisk() && !c.OnlyInRamdisk() {
		subName += RamdiskSuffix
	} else if c.InVendorRamdisk() && !c.OnlyInVendorRamdisk() {
		subName += VendorRamdiskSuffix
	} else if c.InRecovery() && !c.OnlyInRecovery() {
		subName += RecoverySuffix
	} else if c.IsSdkVariant() && (c.SdkAndPlatformVariantVisibleToMake() || c.SplitPerApiLevel()) {
		subName += sdkSuffix
		if c.SplitPerApiLevel() {
			subName += "." + c.SdkVersion()
		}
	} else if c.IsStubs() && c.IsSdkVariant() {
		// Public API surface (NDK)
		// Add a suffix to this stub variant to distinguish it from the module-lib stub variant.
		subName = sdkSuffix
	}

	return subName
}

func moduleContextFromAndroidModuleContext(actx android.ModuleContext, c *Module) ModuleContext {
	ctx := &moduleContext{
		ModuleContext: actx,
		moduleContextImpl: moduleContextImpl{
			mod: c,
		},
	}
	ctx.ctx = ctx
	return ctx
}

// TODO (b/277651159): Remove this allowlist
var (
	skipStubLibraryMultipleApexViolation = map[string]bool{
		"libclang_rt.asan":   true,
		"libclang_rt.hwasan": true,
		// runtime apex
		"libc":          true,
		"libc_hwasan":   true,
		"libdl_android": true,
		"libm":          true,
		"libdl":         true,
		"libz":          true,
		// art apex
		// TODO(b/234351700): Remove this when com.android.art.debug is gone.
		"libandroidio":    true,
		"libdexfile":      true,
		"libdexfiled":     true, // com.android.art.debug only
		"libnativebridge": true,
		"libnativehelper": true,
		"libnativeloader": true,
		"libsigchain":     true,
	}
)

// Returns true if a stub library could be installed in multiple apexes
func (c *Module) stubLibraryMultipleApexViolation(ctx android.ModuleContext) bool {
	// If this is not an apex variant, no check necessary
	if info, ok := android.ModuleProvider(ctx, android.ApexInfoProvider); !ok || info.IsForPlatform() {
		return false
	}
	// If this is not a stub library, no check necessary
	if !c.HasStubsVariants() {
		return false
	}
	// Skip the allowlist
	// Use BaseModuleName so that this matches prebuilts.
	if _, exists := skipStubLibraryMultipleApexViolation[c.BaseModuleName()]; exists {
		return false
	}

	// Stub libraries should not have more than one apex_available
	apexAvailable := android.FirstUniqueStrings(c.ApexAvailable())
	if len(apexAvailable) > 1 {
		return true
	}
	// Stub libraries should not use the wildcard
	if apexAvailable[0] == android.AvailableToAnyApex {
		return true
	}
	// Default: no violation
	return false
}

func (c *Module) GenerateAndroidBuildActions(actx android.ModuleContext) {
	ctx := moduleContextFromAndroidModuleContext(actx, c)

	c.logtagsPaths = android.PathsForModuleSrc(actx, c.Properties.Logtags)
	android.SetProvider(ctx, android.LogtagsProviderKey, &android.LogtagsInfo{
		Logtags: c.logtagsPaths,
	})

	// If Test_only is set on a module in bp file, respect the setting, otherwise
	// see if is a known test module type.
	testOnly := c.testModule || c.testLibrary()
	if c.sourceProperties.Test_only != nil {
		testOnly = Bool(c.sourceProperties.Test_only)
	}
	// Keep before any early returns.
	android.SetProvider(ctx, android.TestOnlyProviderKey, android.TestModuleInformation{
		TestOnly:       testOnly,
		TopLevelTarget: c.testModule,
	})

	c.Properties.SubName = GetSubnameProperty(actx, c)
	apexInfo, _ := android.ModuleProvider(actx, android.ApexInfoProvider)
	if !apexInfo.IsForPlatform() {
		c.hideApexVariantFromMake = true
	}

	c.makeLinkType = GetMakeLinkType(actx, c)

	deps := c.depsToPaths(ctx)
	if ctx.Failed() {
		return
	}

	for _, generator := range c.generators {
		gen := generator.GeneratorSources(ctx)
		deps.IncludeDirs = append(deps.IncludeDirs, gen.IncludeDirs...)
		deps.ReexportedDirs = append(deps.ReexportedDirs, gen.ReexportedDirs...)
		deps.GeneratedDeps = append(deps.GeneratedDeps, gen.Headers...)
		deps.ReexportedGeneratedHeaders = append(deps.ReexportedGeneratedHeaders, gen.Headers...)
		deps.ReexportedDeps = append(deps.ReexportedDeps, gen.Headers...)
		if len(deps.Objs.objFiles) == 0 {
			// If we are reusuing object files (which happens when we're a shared library and we're
			// reusing our static variant's object files), then skip adding the actual source files,
			// because we already have the object for it.
			deps.GeneratedSources = append(deps.GeneratedSources, gen.Sources...)
		}
	}

	if ctx.Failed() {
		return
	}

	if c.stubLibraryMultipleApexViolation(actx) {
		actx.PropertyErrorf("apex_available",
			"Stub libraries should have a single apex_available (test apexes excluded). Got %v", c.ApexAvailable())
	}
	if c.Properties.Clang != nil && *c.Properties.Clang == false {
		ctx.PropertyErrorf("clang", "false (GCC) is no longer supported")
	} else if c.Properties.Clang != nil && !ctx.DeviceConfig().BuildBrokenClangProperty() {
		ctx.PropertyErrorf("clang", "property is deprecated, see Changes.md file")
	}

	flags := Flags{
		Toolchain: c.toolchain(ctx),
		EmitXrefs: ctx.Config().EmitXrefRules(),
	}
	for _, generator := range c.generators {
		flags = generator.GeneratorFlags(ctx, flags, deps)
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
	if c.fuzzer != nil {
		flags = c.fuzzer.flags(ctx, flags)
	}
	if c.lto != nil {
		flags = c.lto.flags(ctx, flags)
	}
	if c.afdo != nil {
		flags = c.afdo.flags(ctx, flags)
	}
	if c.orderfile != nil {
		flags = c.orderfile.flags(ctx, flags)
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

	flags.Local.LdFlags = append(flags.Local.LdFlags, deps.LdFlags...)

	c.flags = flags
	// We need access to all the flags seen by a source file.
	if c.sabi != nil {
		flags = c.sabi.flags(ctx, flags)
	}

	flags.AssemblerWithCpp = inList("-xassembler-with-cpp", flags.Local.AsFlags)

	for _, generator := range c.generators {
		generator.GeneratorBuildActions(ctx, flags, deps)
	}

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

		c.maybeUnhideFromMake()

		android.SetProvider(ctx, ImplementationDepInfoProvider, &ImplementationDepInfo{
			ImplementationDeps: depset.New(depset.PREORDER, deps.directImplementationDeps, deps.transitiveImplementationDeps),
		})
	}

	if Bool(c.Properties.Cmake_snapshot_supported) {
		android.SetProvider(ctx, cmakeSnapshotSourcesProvider, android.GlobFiles(ctx, ctx.ModuleDir()+"/**/*", nil))
	}

	c.maybeInstall(ctx, apexInfo)

	if c.linker != nil {
		moduleInfoJSON := ctx.ModuleInfoJSON()
		c.linker.moduleInfoJSON(ctx, moduleInfoJSON)
		moduleInfoJSON.SharedLibs = c.Properties.AndroidMkSharedLibs
		moduleInfoJSON.StaticLibs = c.Properties.AndroidMkStaticLibs
		moduleInfoJSON.SystemSharedLibs = c.Properties.AndroidMkSystemSharedLibs
		moduleInfoJSON.RuntimeDependencies = c.Properties.AndroidMkRuntimeLibs

		moduleInfoJSON.Dependencies = append(moduleInfoJSON.Dependencies, c.Properties.AndroidMkSharedLibs...)
		moduleInfoJSON.Dependencies = append(moduleInfoJSON.Dependencies, c.Properties.AndroidMkStaticLibs...)
		moduleInfoJSON.Dependencies = append(moduleInfoJSON.Dependencies, c.Properties.AndroidMkHeaderLibs...)
		moduleInfoJSON.Dependencies = append(moduleInfoJSON.Dependencies, c.Properties.AndroidMkWholeStaticLibs...)

		if c.sanitize != nil && len(moduleInfoJSON.Class) > 0 &&
			(moduleInfoJSON.Class[0] == "STATIC_LIBRARIES" || moduleInfoJSON.Class[0] == "HEADER_LIBRARIES") {
			if Bool(c.sanitize.Properties.SanitizeMutated.Cfi) {
				moduleInfoJSON.SubName += ".cfi"
			}
			if Bool(c.sanitize.Properties.SanitizeMutated.Hwaddress) {
				moduleInfoJSON.SubName += ".hwasan"
			}
			if Bool(c.sanitize.Properties.SanitizeMutated.Scs) {
				moduleInfoJSON.SubName += ".scs"
			}
		}
		moduleInfoJSON.SubName += c.Properties.SubName

		if c.Properties.IsSdkVariant && c.Properties.SdkAndPlatformVariantVisibleToMake {
			moduleInfoJSON.Uninstallable = true
		}
	}

	buildComplianceMetadataInfo(ctx, c, deps)

	if b, ok := c.compiler.(*baseCompiler); ok {
		c.hasAidl = b.hasSrcExt(ctx, ".aidl")
		c.hasLex = b.hasSrcExt(ctx, ".l") || b.hasSrcExt(ctx, ".ll")
		c.hasProto = b.hasSrcExt(ctx, ".proto")
		c.hasRenderscript = b.hasSrcExt(ctx, ".rscript") || b.hasSrcExt(ctx, ".fs")
		c.hasSysprop = b.hasSrcExt(ctx, ".sysprop")
		c.hasWinMsg = b.hasSrcExt(ctx, ".mc")
		c.hasYacc = b.hasSrcExt(ctx, ".y") || b.hasSrcExt(ctx, ".yy")
	}

	ccObjectInfo := CcObjectInfo{
		KytheFiles: objs.kytheFiles,
	}
	if !ctx.Config().KatiEnabled() || !android.ShouldSkipAndroidMkProcessing(ctx, c) {
		ccObjectInfo.ObjFiles = objs.objFiles
		ccObjectInfo.TidyFiles = objs.tidyFiles
	}
	if len(ccObjectInfo.KytheFiles)+len(ccObjectInfo.ObjFiles)+len(ccObjectInfo.TidyFiles) > 0 {
		android.SetProvider(ctx, CcObjectInfoProvider, ccObjectInfo)
	}

	linkableInfo := CreateCommonLinkableInfo(ctx, c)
	if lib, ok := c.linker.(VersionedInterface); ok {
		linkableInfo.StubsVersion = lib.StubsVersion()
	}
	if c.linker != nil {
		if library, ok := c.linker.(libraryInterface); ok {
			linkableInfo.Static = library.static()
			linkableInfo.Shared = library.shared()
			linkableInfo.CoverageFiles = library.objs().coverageFiles
			linkableInfo.SAbiDumpFiles = library.objs().sAbiDumpFiles
		}
	}
	android.SetProvider(ctx, LinkableInfoProvider, linkableInfo)

	ccInfo := CcInfo{
		IsPrebuilt:             c.IsPrebuilt(),
		CmakeSnapshotSupported: proptools.Bool(c.Properties.Cmake_snapshot_supported),
		HasLlndkStubs:          c.HasLlndkStubs(),
		DataPaths:              c.DataPaths(),
	}
	if c.compiler != nil {
		cflags := c.compiler.baseCompilerProps().Cflags
		ccInfo.CompilerInfo = &CompilerInfo{
			Srcs:   c.compiler.(CompiledInterface).Srcs(),
			Cflags: cflags.GetOrDefault(ctx, nil),
			AidlInterfaceInfo: AidlInterfaceInfo{
				Sources:  c.compiler.baseCompilerProps().AidlInterface.Sources,
				AidlRoot: c.compiler.baseCompilerProps().AidlInterface.AidlRoot,
				Lang:     c.compiler.baseCompilerProps().AidlInterface.Lang,
				Flags:    c.compiler.baseCompilerProps().AidlInterface.Flags,
			},
		}
		switch decorator := c.compiler.(type) {
		case *libraryDecorator:
			ccInfo.CompilerInfo.LibraryDecoratorInfo = &LibraryDecoratorInfo{
				ExportIncludeDirs: decorator.flagExporter.Properties.Export_include_dirs.GetOrDefault(ctx, nil),
			}
		}
	}
	if c.linker != nil {
		baseLinkerProps := c.linker.baseLinkerProps()
		ccInfo.LinkerInfo = &LinkerInfo{
			WholeStaticLibs: baseLinkerProps.Whole_static_libs.GetOrDefault(ctx, nil),
			StaticLibs:      baseLinkerProps.Static_libs.GetOrDefault(ctx, nil),
			SharedLibs:      baseLinkerProps.Shared_libs.GetOrDefault(ctx, nil),
			HeaderLibs:      baseLinkerProps.Header_libs.GetOrDefault(ctx, nil),
		}
		switch decorator := c.linker.(type) {
		case *binaryDecorator:
			ccInfo.LinkerInfo.BinaryDecoratorInfo = &BinaryDecoratorInfo{}
		case *libraryDecorator:
			lk := c.linker.(*libraryDecorator)
			ccInfo.LinkerInfo.LibraryDecoratorInfo = &LibraryDecoratorInfo{
				InjectBsslHash: Bool(lk.Properties.Inject_bssl_hash),
				NdkSysrootPath: lk.ndkSysrootPath,
				VndkFileName:   lk.getLibNameHelper(c.BaseModuleName(), true, false) + ".so",
			}
		case *testBinary:
			ccInfo.LinkerInfo.TestBinaryInfo = &TestBinaryInfo{
				Gtest: decorator.testDecorator.gtest(),
			}
		case *benchmarkDecorator:
			ccInfo.LinkerInfo.BenchmarkDecoratorInfo = &BenchmarkDecoratorInfo{}
		case *objectLinker:
			ccInfo.LinkerInfo.ObjectLinkerInfo = &ObjectLinkerInfo{
				NdkSysrootPath: c.linker.(*objectLinker).ndkSysrootPath,
			}
		case *stubDecorator:
			ccInfo.LinkerInfo.StubDecoratorInfo = &StubDecoratorInfo{}
		case *prebuiltLibraryLinker:
			ccInfo.LinkerInfo.PrebuiltLibraryLinkerInfo = &PrebuiltLibraryLinkerInfo{
				VndkFileName: c.linker.(*prebuiltLibraryLinker).getLibNameHelper(
					c.BaseModuleName(), true, false) + ".so",
			}
		}

		if s, ok := c.linker.(SnapshotInterface); ok {
			ccInfo.SnapshotInfo = &SnapshotInfo{
				SnapshotAndroidMkSuffix: s.SnapshotAndroidMkSuffix(),
			}
		}
		if v, ok := c.linker.(VersionedInterface); ok {
			name := v.ImplementationModuleName(ctx.OtherModuleName(c))
			ccInfo.LinkerInfo.ImplementationModuleName = &name
		}

		c.linker.testSuiteInfo(ctx)
	}
	if c.library != nil {
		ccInfo.LibraryInfo = &LibraryInfo{
			BuildStubs: c.library.BuildStubs(),
		}
	}
	if c.installer != nil {
		ccInfo.InstallerInfo = &InstallerInfo{}
		if installer, ok := c.installer.(*stubDecorator); ok {
			ccInfo.InstallerInfo.StubDecoratorInfo = &StubDecoratorInfo{
				HasAbiDump:   installer.hasAbiDump,
				AbiDumpPath:  installer.abiDumpPath,
				AbiDiffPaths: installer.abiDiffPaths,
				InstallPath:  installer.installPath,
			}
		}
	}
	android.SetProvider(ctx, CcInfoProvider, &ccInfo)

	c.setOutputFiles(ctx)

	if c.makeVarsInfo != nil {
		android.SetProvider(ctx, CcMakeVarsInfoProvider, c.makeVarsInfo)
	}
}

func (c *Module) CleanupAfterBuildActions() {
	// Clear as much of Module as possible to reduce memory usage.
	c.generators = nil
	c.compiler = nil
	c.installer = nil
	c.features = nil
	c.coverage = nil
	c.fuzzer = nil
	c.sabi = nil
	c.lto = nil
	c.afdo = nil
	c.orderfile = nil

	// TODO: these can be cleared after nativeBinaryInfoProperties and nativeLibInfoProperties are switched to
	//  using providers.
	// c.linker = nil
	// c.stl = nil
	// c.sanitize = nil
	// c.library = nil
}

func CreateCommonLinkableInfo(ctx android.ModuleContext, mod VersionedLinkableInterface) *LinkableInfo {
	info := &LinkableInfo{
		StaticExecutable:     mod.StaticExecutable(),
		HasStubsVariants:     mod.HasStubsVariants(),
		OutputFile:           mod.OutputFile(),
		UnstrippedOutputFile: mod.UnstrippedOutputFile(),
		CoverageOutputFile:   mod.CoverageOutputFile(),
		Partition:            mod.Partition(),
		IsStubs:              mod.IsStubs(),
		CcLibrary:            mod.CcLibrary(),
		CcLibraryInterface:   mod.CcLibraryInterface(),
		RustLibraryInterface: mod.RustLibraryInterface(),
		IsLlndk:              mod.IsLlndk(),
		IsNdk:                mod.IsNdk(ctx.Config()),
		HasNonSystemVariants: mod.HasNonSystemVariants(),
		SubName:              mod.SubName(),
		InVendorOrProduct:    mod.InVendorOrProduct(),
		InRamdisk:            mod.InRamdisk(),
		OnlyInRamdisk:        mod.OnlyInRamdisk(),
		InVendorRamdisk:      mod.InVendorRamdisk(),
		OnlyInVendorRamdisk:  mod.OnlyInVendorRamdisk(),
		InRecovery:           mod.InRecovery(),
		OnlyInRecovery:       mod.OnlyInRecovery(),
		InVendor:             mod.InVendor(),
		Installable:          mod.Installable(),
		RelativeInstallPath:  mod.RelativeInstallPath(),
		// TODO(b/362509506): remove this once all apex_exclude uses are switched to stubs.
		RustApexExclude:                 mod.RustApexExclude(),
		Bootstrap:                       mod.Bootstrap(),
		Multilib:                        mod.Multilib(),
		ImplementationModuleNameForMake: mod.ImplementationModuleNameForMake(),
		Symlinks:                        mod.Symlinks(),
		Header:                          mod.Header(),
		IsVndkPrebuiltLibrary:           mod.IsVndkPrebuiltLibrary(),
	}

	vi := mod.VersionedInterface()
	if vi != nil {
		info.IsStubsImplementationRequired = vi.IsStubsImplementationRequired()
		info.APIListCoverageXMLPath = vi.GetAPIListCoverageXMLPath()
		info.HasLLNDKStubs = vi.HasLLNDKStubs()
		info.IsLLNDKMovedToApex = vi.IsLLNDKMovedToApex()
		info.ImplementationModuleName = vi.ImplementationModuleName(mod.BaseModuleName())
	}

	if !mod.PreventInstall() && fuzz.IsValid(ctx, mod.FuzzModuleStruct()) && mod.IsFuzzModule() {
		info.FuzzSharedLibraries = mod.FuzzSharedLibraries()
		fm := mod.FuzzPackagedModule()
		fuzz.SetFuzzPackagedModuleInfo(ctx, &fm)
	}

	return info
}

func (c *Module) setOutputFiles(ctx ModuleContext) {
	if c.outputFile.Valid() {
		ctx.SetOutputFiles(android.Paths{c.outputFile.Path()}, "")
	} else {
		ctx.SetOutputFiles(android.Paths{}, "")
	}
	if c.linker != nil {
		ctx.SetOutputFiles(android.PathsIfNonNil(c.linker.unstrippedOutputFilePath()), "unstripped")
		ctx.SetOutputFiles(android.PathsIfNonNil(c.linker.strippedAllOutputFilePath()), "stripped_all")
		defaultDistFiles := c.linker.defaultDistFiles()
		if len(defaultDistFiles) > 0 {
			ctx.SetOutputFiles(defaultDistFiles, android.DefaultDistTag)
		}
	}
}

func buildComplianceMetadataInfo(ctx ModuleContext, c *Module, deps PathDeps) {
	// Dump metadata that can not be done in android/compliance-metadata.go
	complianceMetadataInfo := ctx.ComplianceMetadataInfo()
	complianceMetadataInfo.SetStringValue(android.ComplianceMetadataProp.IS_STATIC_LIB, strconv.FormatBool(ctx.static() || ctx.ModuleType() == "cc_object"))
	complianceMetadataInfo.SetStringValue(android.ComplianceMetadataProp.BUILT_FILES, c.outputFile.String())

	// Static deps
	staticDeps := ctx.GetDirectDepsProxyWithTag(StaticDepTag(false))
	staticDepNames := make([]string, 0, len(staticDeps))
	for _, dep := range staticDeps {
		staticDepNames = append(staticDepNames, dep.Name())
	}
	// Process CrtBegin and CrtEnd as static libs
	ctx.VisitDirectDepsProxy(func(dep android.ModuleProxy) {
		depName := ctx.OtherModuleName(dep)
		depTag := ctx.OtherModuleDependencyTag(dep)
		switch depTag {
		case CrtBeginDepTag:
			staticDepNames = append(staticDepNames, depName)
		case CrtEndDepTag:
			staticDepNames = append(staticDepNames, depName)
		}
	})

	staticDepPaths := make([]string, 0, len(deps.StaticLibs)+len(deps.CrtBegin)+len(deps.CrtEnd))
	for _, dep := range deps.StaticLibs {
		staticDepPaths = append(staticDepPaths, dep.String())
	}
	for _, dep := range deps.CrtBegin {
		staticDepPaths = append(staticDepPaths, dep.String())
	}
	for _, dep := range deps.CrtEnd {
		staticDepPaths = append(staticDepPaths, dep.String())
	}
	complianceMetadataInfo.SetListValue(android.ComplianceMetadataProp.STATIC_DEPS, android.FirstUniqueStrings(staticDepNames))
	complianceMetadataInfo.SetListValue(android.ComplianceMetadataProp.STATIC_DEP_FILES, android.FirstUniqueStrings(staticDepPaths))

	// Whole static deps
	wholeStaticDeps := ctx.GetDirectDepsProxyWithTag(StaticDepTag(true))
	wholeStaticDepNames := make([]string, 0, len(wholeStaticDeps))
	for _, dep := range wholeStaticDeps {
		wholeStaticDepNames = append(wholeStaticDepNames, dep.Name())
	}

	wholeStaticDepPaths := make([]string, 0, len(deps.WholeStaticLibs))
	for _, dep := range deps.WholeStaticLibs {
		wholeStaticDepPaths = append(wholeStaticDepPaths, dep.String())
	}
	complianceMetadataInfo.SetListValue(android.ComplianceMetadataProp.WHOLE_STATIC_DEPS, android.FirstUniqueStrings(wholeStaticDepNames))
	complianceMetadataInfo.SetListValue(android.ComplianceMetadataProp.WHOLE_STATIC_DEP_FILES, android.FirstUniqueStrings(wholeStaticDepPaths))

	// Header libs
	headerLibDeps := ctx.GetDirectDepsProxyWithTag(HeaderDepTag())
	headerLibDepNames := make([]string, 0, len(headerLibDeps))
	for _, dep := range headerLibDeps {
		headerLibDepNames = append(headerLibDepNames, dep.Name())
	}
	complianceMetadataInfo.SetListValue(android.ComplianceMetadataProp.HEADER_LIBS, android.FirstUniqueStrings(headerLibDepNames))
}

func (c *Module) maybeUnhideFromMake() {
	// If a lib is directly included in any of the APEXes or is not available to the
	// platform (which is often the case when the stub is provided as a prebuilt),
	// unhide the stubs variant having the latest version gets visible to make. In
	// addition, the non-stubs variant is renamed to <libname>.bootstrap. This is to
	// force anything in the make world to link against the stubs library.  (unless it
	// is explicitly referenced via .bootstrap suffix or the module is marked with
	// 'bootstrap: true').
	if c.HasStubsVariants() && c.NotInPlatform() && !c.InRamdisk() &&
		!c.InRecovery() && !c.InVendorOrProduct() && !c.static() && !c.isCoverageVariant() &&
		c.IsStubs() && !c.InVendorRamdisk() {
		c.Properties.HideFromMake = false // unhide
		// Note: this is still non-installable
	}
}

// maybeInstall is called at the end of both GenerateAndroidBuildActions to run the
// install hooks for installable modules, like binaries and tests.
func (c *Module) maybeInstall(ctx ModuleContext, apexInfo android.ApexInfo) {
	if !proptools.BoolDefault(c.Installable(), true) {
		// If the module has been specifically configure to not be installed then
		// hide from make as otherwise it will break when running inside make
		// as the output path to install will not be specified. Not all uninstallable
		// modules can be hidden from make as some are needed for resolving make side
		// dependencies.
		c.HideFromMake()
		c.SkipInstall()
	} else if !installable(c, apexInfo) {
		c.SkipInstall()
	}

	// Still call c.installer.install though, the installs will be stored as PackageSpecs
	// to allow using the outputs in a genrule.
	if c.installer != nil && c.outputFile.Valid() {
		c.installer.install(ctx, c.outputFile.Path())
		if ctx.Failed() {
			return
		}
	}
}

func (c *Module) toolchain(ctx android.BaseModuleContext) config.Toolchain {
	if c.cachedToolchain == nil {
		c.cachedToolchain = config.FindToolchainWithContext(ctx)
	}
	return c.cachedToolchain
}

func (c *Module) begin(ctx BaseModuleContext) {
	for _, generator := range c.generators {
		generator.GeneratorInit(ctx)
	}
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
	if c.afdo != nil {
		c.afdo.begin(ctx)
	}
	if c.lto != nil {
		c.lto.begin(ctx)
	}
	if c.orderfile != nil {
		c.orderfile.begin(ctx)
	}
	if ctx.useSdk() && c.IsSdkVariant() {
		version, err := NativeApiLevelFromUser(ctx, ctx.sdkVersion())
		if err != nil {
			ctx.PropertyErrorf("sdk_version", err.Error())
			c.Properties.Sdk_version = nil
		} else {
			c.Properties.Sdk_version = StringPtr(version.String())
		}
	}
}

func (c *Module) deps(ctx DepsContext) Deps {
	deps := Deps{}

	for _, generator := range c.generators {
		deps = generator.GeneratorDeps(ctx, deps)
	}
	if c.compiler != nil {
		deps = c.compiler.compilerDeps(ctx, deps)
	}
	if c.linker != nil {
		deps = c.linker.linkerDeps(ctx, deps)
	}
	if c.stl != nil {
		deps = c.stl.deps(ctx, deps)
	}
	if c.coverage != nil {
		deps = c.coverage.deps(ctx, deps)
	}

	deps.WholeStaticLibs = android.LastUniqueStrings(deps.WholeStaticLibs)
	deps.StaticLibs = android.LastUniqueStrings(deps.StaticLibs)
	deps.LateStaticLibs = android.LastUniqueStrings(deps.LateStaticLibs)
	deps.SharedLibs = android.LastUniqueStrings(deps.SharedLibs)
	deps.LateSharedLibs = android.LastUniqueStrings(deps.LateSharedLibs)
	deps.HeaderLibs = android.LastUniqueStrings(deps.HeaderLibs)
	deps.RuntimeLibs = android.LastUniqueStrings(deps.RuntimeLibs)
	deps.LlndkHeaderLibs = android.LastUniqueStrings(deps.LlndkHeaderLibs)

	if err := checkConflictingExplicitVersions(deps.SharedLibs); err != nil {
		ctx.PropertyErrorf("shared_libs", "%s", err.Error())
	}

	for _, lib := range deps.ReexportSharedLibHeaders {
		if !inList(lib, deps.SharedLibs) {
			ctx.PropertyErrorf("export_shared_lib_headers", "Shared library not in shared_libs: '%s'", lib)
		}
	}

	for _, lib := range deps.ReexportStaticLibHeaders {
		if !inList(lib, deps.StaticLibs) && !inList(lib, deps.WholeStaticLibs) {
			ctx.PropertyErrorf("export_static_lib_headers", "Static library not in static_libs or whole_static_libs: '%s'", lib)
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

func checkConflictingExplicitVersions(libs []string) error {
	withoutVersion := func(s string) string {
		name, _ := StubsLibNameAndVersion(s)
		return name
	}
	var errs []error
	for i, lib := range libs {
		libName := withoutVersion(lib)
		libsToCompare := libs[i+1:]
		j := slices.IndexFunc(libsToCompare, func(s string) bool {
			return withoutVersion(s) == libName
		})
		if j >= 0 {
			errs = append(errs, fmt.Errorf("duplicate shared libraries with different explicit versions: %q and %q",
				lib, libsToCompare[j]))
		}
	}
	return errors.Join(errs...)
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

func GetCrtVariations(ctx android.BottomUpMutatorContext,
	m LinkableInterface) []blueprint.Variation {
	if ctx.Os() != android.Android {
		return nil
	}
	if m.UseSdk() {
		// Choose the CRT that best satisfies the min_sdk_version requirement of this module
		minSdkVersion := m.MinSdkVersion()
		if minSdkVersion == "" || minSdkVersion == "apex_inherit" {
			minSdkVersion = m.SdkVersion()
		}
		apiLevel, err := android.ApiLevelFromUser(ctx, minSdkVersion)
		if err != nil {
			ctx.PropertyErrorf("min_sdk_version", err.Error())
		}

		// Raise the minSdkVersion to the minimum supported for the architecture.
		minApiForArch := MinApiForArch(ctx, m.Target().Arch.ArchType)
		if apiLevel.LessThan(minApiForArch) {
			apiLevel = minApiForArch
		}

		return []blueprint.Variation{
			{Mutator: "sdk", Variation: "sdk"},
			{Mutator: "version", Variation: apiLevel.String()},
		}
	}
	return []blueprint.Variation{
		{Mutator: "sdk", Variation: ""},
	}
}

func AddSharedLibDependenciesWithVersions(ctx android.BottomUpMutatorContext, mod LinkableInterface,
	variations []blueprint.Variation, depTag blueprint.DependencyTag, name, version string, far bool) {

	variations = append([]blueprint.Variation(nil), variations...)

	if version != "" && canBeOrLinkAgainstVersionVariants(mod) {
		// Version is explicitly specified. i.e. libFoo#30
		if version == "impl" {
			version = ""
		}
		variations = append(variations, blueprint.Variation{Mutator: "version", Variation: version})
		if tag, ok := depTag.(libraryDependencyTag); ok {
			tag.explicitlyVersioned = true
			if version == "" {
				tag.explicitlyImpl = true
			}
			// depTag is an interface that contains a concrete non-pointer struct.  That makes the local
			// tag variable a copy of the contents of depTag, and updating it doesn't change depTag.  Reassign
			// the modified copy to depTag.
			depTag = tag
		} else {
			panic(fmt.Errorf("Unexpected dependency tag: %T", depTag))
		}
	}

	if far {
		ctx.AddFarVariationDependencies(variations, depTag, name)
	} else {
		ctx.AddVariationDependencies(variations, depTag, name)
	}
}

func GetReplaceModuleName(lib string, replaceMap map[string]string) string {
	if snapshot, ok := replaceMap[lib]; ok {
		return snapshot
	}

	return lib
}

// FilterNdkLibs takes a list of names of shared libraries and scans it for two types
// of names:
//
// 1. Name of an NDK library that refers to an ndk_library module.
//
//	For each of these, it adds the name of the ndk_library module to the list of
//	variant libs.
//
// 2. Anything else (so anything that isn't an NDK library).
//
//	It adds these to the nonvariantLibs list.
//
// The caller can then know to add the variantLibs dependencies differently from the
// nonvariantLibs
func FilterNdkLibs(c LinkableInterface, config android.Config, list []string) (nonvariantLibs []string, variantLibs []string) {
	variantLibs = []string{}

	nonvariantLibs = []string{}
	for _, entry := range list {
		// strip #version suffix out
		name, _ := StubsLibNameAndVersion(entry)
		if c.UseSdk() && inList(name, *getNDKKnownLibs(config)) {
			variantLibs = append(variantLibs, name+ndkLibrarySuffix)
		} else {
			nonvariantLibs = append(nonvariantLibs, entry)
		}
	}
	return nonvariantLibs, variantLibs

}

func rewriteLibsForApiImports(c LinkableInterface, libs []string, replaceList map[string]string, config android.Config) ([]string, []string) {
	nonVariantLibs := []string{}
	variantLibs := []string{}

	for _, lib := range libs {
		replaceLibName := GetReplaceModuleName(lib, replaceList)
		if replaceLibName == lib {
			// Do not handle any libs which are not in API imports
			nonVariantLibs = append(nonVariantLibs, replaceLibName)
		} else if c.UseSdk() && inList(replaceLibName, *getNDKKnownLibs(config)) {
			variantLibs = append(variantLibs, replaceLibName)
		} else {
			nonVariantLibs = append(nonVariantLibs, replaceLibName)
		}
	}

	return nonVariantLibs, variantLibs
}

func (c *Module) shouldUseApiSurface() bool {
	if c.Os() == android.Android && c.Target().NativeBridge != android.NativeBridgeEnabled {
		if GetImageVariantType(c) == vendorImageVariant || GetImageVariantType(c) == productImageVariant {
			// LLNDK Variant
			return true
		}

		if c.Properties.IsSdkVariant {
			// NDK Variant
			return true
		}
	}

	return false
}

func (c *Module) DepsMutator(actx android.BottomUpMutatorContext) {
	if !c.Enabled(actx) {
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

	apiNdkLibs := []string{}
	apiLateNdkLibs := []string{}

	c.Properties.AndroidMkSystemSharedLibs = deps.SystemSharedLibs

	variantNdkLibs := []string{}
	variantLateNdkLibs := []string{}
	if ctx.Os() == android.Android {
		rewriteHeaderLibs := func(list []string) (newHeaderLibs []string) {
			newHeaderLibs = []string{}
			for _, entry := range list {
				// Replace device_kernel_headers with generated_kernel_headers
				// for inline kernel building
				if entry == "device_kernel_headers" || entry == "qti_kernel_headers" {
					newHeaderLibs = append(newHeaderLibs, "generated_kernel_headers")
					continue
				}
				newHeaderLibs = append(newHeaderLibs, entry)
			}
			return newHeaderLibs
		}
		deps.HeaderLibs = rewriteHeaderLibs(deps.HeaderLibs)

		deps.SharedLibs, variantNdkLibs = FilterNdkLibs(c, ctx.Config(), deps.SharedLibs)
		deps.LateSharedLibs, variantLateNdkLibs = FilterNdkLibs(c, ctx.Config(), deps.LateSharedLibs)
		deps.ReexportSharedLibHeaders, _ = FilterNdkLibs(c, ctx.Config(), deps.ReexportSharedLibHeaders)
	}

	for _, lib := range deps.HeaderLibs {
		depTag := libraryDependencyTag{Kind: headerLibraryDependency}
		if inList(lib, deps.ReexportHeaderLibHeaders) {
			depTag.reexportFlags = true
		}

		if c.isNDKStubLibrary() {
			variationExists := actx.OtherModuleDependencyVariantExists(nil, lib)
			if variationExists {
				actx.AddVariationDependencies(nil, depTag, lib)
			} else {
				// dependencies to ndk_headers fall here as ndk_headers do not have
				// any variants.
				actx.AddFarVariationDependencies([]blueprint.Variation{}, depTag, lib)
			}
		} else if c.IsStubs() {
			actx.AddFarVariationDependencies(append(ctx.Target().Variations(), c.ImageVariation()),
				depTag, lib)
		} else {
			actx.AddVariationDependencies(nil, depTag, lib)
		}
	}

	if c.isNDKStubLibrary() {
		// NDK stubs depend on their implementation because the ABI dumps are
		// generated from the implementation library.

		actx.AddFarVariationDependencies(append(ctx.Target().Variations(),
			c.ImageVariation(),
			blueprint.Variation{Mutator: "link", Variation: "shared"},
		), stubImplementation, c.BaseModuleName())
	}

	// If this module is an LLNDK implementation library, let it depend on LlndkHeaderLibs.
	if c.ImageVariation().Variation == android.CoreVariation && c.Device() &&
		c.Target().NativeBridge == android.NativeBridgeDisabled {
		actx.AddVariationDependencies(
			[]blueprint.Variation{{Mutator: "image", Variation: android.VendorVariation}},
			llndkHeaderLibTag,
			deps.LlndkHeaderLibs...)
	}

	for _, lib := range deps.WholeStaticLibs {
		depTag := libraryDependencyTag{Kind: staticLibraryDependency, wholeStatic: true, reexportFlags: true}

		actx.AddVariationDependencies([]blueprint.Variation{
			{Mutator: "link", Variation: "static"},
		}, depTag, lib)
	}

	for _, lib := range deps.StaticLibs {
		// Some dependencies listed in static_libs might actually be rust_ffi rlib variants.
		depTag := libraryDependencyTag{Kind: staticLibraryDependency}

		if inList(lib, deps.ReexportStaticLibHeaders) {
			depTag.reexportFlags = true
		}
		if inList(lib, deps.ExcludeLibsForApex) {
			depTag.excludeInApex = true
		}
		actx.AddVariationDependencies([]blueprint.Variation{
			{Mutator: "link", Variation: "static"},
		}, depTag, lib)
	}

	// staticUnwinderDep is treated as staticDep for Q apexes
	// so that native libraries/binaries are linked with static unwinder
	// because Q libc doesn't have unwinder APIs
	if deps.StaticUnwinderIfLegacy {
		depTag := libraryDependencyTag{Kind: staticLibraryDependency, staticUnwinder: true}
		actx.AddVariationDependencies([]blueprint.Variation{
			{Mutator: "link", Variation: "static"},
		}, depTag, staticUnwinder(actx))
	}

	// shared lib names without the #version suffix
	var sharedLibNames []string

	for _, lib := range deps.SharedLibs {
		depTag := libraryDependencyTag{Kind: sharedLibraryDependency}
		if inList(lib, deps.ReexportSharedLibHeaders) {
			depTag.reexportFlags = true
		}
		if inList(lib, deps.ExcludeLibsForApex) {
			depTag.excludeInApex = true
		}
		if inList(lib, deps.ExcludeLibsForNonApex) {
			depTag.excludeInNonApex = true
		}

		name, version := StubsLibNameAndVersion(lib)
		sharedLibNames = append(sharedLibNames, name)

		variations := []blueprint.Variation{
			{Mutator: "link", Variation: "shared"},
		}
		AddSharedLibDependenciesWithVersions(ctx, c, variations, depTag, name, version, false)
	}

	for _, lib := range deps.LateStaticLibs {
		depTag := libraryDependencyTag{Kind: staticLibraryDependency, Order: lateLibraryDependency}
		actx.AddVariationDependencies([]blueprint.Variation{
			{Mutator: "link", Variation: "static"},
		}, depTag, lib)
	}

	for _, lib := range deps.UnexportedStaticLibs {
		depTag := libraryDependencyTag{Kind: staticLibraryDependency, Order: lateLibraryDependency, unexportedSymbols: true}
		actx.AddVariationDependencies([]blueprint.Variation{
			{Mutator: "link", Variation: "static"},
		}, depTag, lib)
	}

	for _, lib := range deps.LateSharedLibs {
		if inList(lib, sharedLibNames) {
			// This is to handle the case that some of the late shared libs (libc, libdl, libm, ...)
			// are added also to SharedLibs with version (e.g., libc#10). If not skipped, we will be
			// linking against both the stubs lib and the non-stubs lib at the same time.
			continue
		}
		depTag := libraryDependencyTag{Kind: sharedLibraryDependency, Order: lateLibraryDependency}
		variations := []blueprint.Variation{
			{Mutator: "link", Variation: "shared"},
		}
		AddSharedLibDependenciesWithVersions(ctx, c, variations, depTag, lib, "", false)
	}

	actx.AddVariationDependencies([]blueprint.Variation{
		{Mutator: "link", Variation: "shared"},
	}, dataLibDepTag, deps.DataLibs...)

	actx.AddVariationDependencies(nil, dataBinDepTag, deps.DataBins...)

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

	for _, gen := range deps.DeviceFirstGeneratedHeaders {
		depTag := genHeaderDepTag
		actx.AddVariationDependencies(ctx.Config().AndroidFirstDeviceTarget.Variations(), depTag, gen)
	}

	crtVariations := GetCrtVariations(ctx, c)
	actx.AddVariationDependencies(crtVariations, objDepTag, deps.ObjFiles...)
	for _, crt := range deps.CrtBegin {
		actx.AddVariationDependencies(crtVariations, CrtBeginDepTag,
			crt)
	}
	for _, crt := range deps.CrtEnd {
		actx.AddVariationDependencies(crtVariations, CrtEndDepTag,
			crt)
	}
	if deps.DynamicLinker != "" {
		actx.AddDependency(c, dynamicLinkerDepTag, deps.DynamicLinker)
	}

	version := ctx.sdkVersion()

	ndkStubDepTag := libraryDependencyTag{Kind: sharedLibraryDependency, ndk: true, makeSuffix: "." + version}
	actx.AddVariationDependencies([]blueprint.Variation{
		{Mutator: "version", Variation: version},
		{Mutator: "link", Variation: "shared"},
	}, ndkStubDepTag, variantNdkLibs...)
	actx.AddVariationDependencies([]blueprint.Variation{
		{Mutator: "version", Variation: version},
		{Mutator: "link", Variation: "shared"},
	}, ndkStubDepTag, apiNdkLibs...)

	ndkLateStubDepTag := libraryDependencyTag{Kind: sharedLibraryDependency, Order: lateLibraryDependency, ndk: true, makeSuffix: "." + version}
	actx.AddVariationDependencies([]blueprint.Variation{
		{Mutator: "version", Variation: version},
		{Mutator: "link", Variation: "shared"},
	}, ndkLateStubDepTag, variantLateNdkLibs...)
	actx.AddVariationDependencies([]blueprint.Variation{
		{Mutator: "version", Variation: version},
		{Mutator: "link", Variation: "shared"},
	}, ndkLateStubDepTag, apiLateNdkLibs...)

	if len(deps.AidlLibs) > 0 {
		actx.AddDependency(
			c,
			aidlLibraryTag,
			deps.AidlLibs...,
		)
	}

}

func BeginMutator(ctx android.BottomUpMutatorContext) {
	if c, ok := ctx.Module().(*Module); ok && c.Enabled(ctx) {
		c.beginMutator(ctx)
	}
}

// Whether a module can link to another module, taking into
// account NDK linking.
func checkLinkType(ctx android.BaseModuleContext, from LinkableInterface, to LinkableInterface,
	tag blueprint.DependencyTag) {

	switch t := tag.(type) {
	case dependencyTag:
		if t != vndkExtDepTag {
			return
		}
	case libraryDependencyTag:
	default:
		return
	}

	if from.Target().Os != android.Android {
		// Host code is not restricted
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
	if from.InVendorRamdisk() {
		// Vendor ramdisk code is not NDK
		return
	}
	if from.InRecovery() {
		// Recovery code is not NDK
		return
	}
	// Change this to LinkableInterface if Rust gets NDK support, which stubDecorators are for
	if c, ok := to.(*Module); ok {
		if c.StubDecorator() {
			// These aren't real libraries, but are the stub shared libraries that are included in
			// the NDK.
			return
		}
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
			fromApi, err := android.ApiLevelFromUserWithConfig(ctx.Config(), from.SdkVersion())
			if err != nil {
				ctx.PropertyErrorf("sdk_version",
					"Invalid sdk_version value (must be int, preview or current): %q",
					from.SdkVersion())
			}
			toApi, err := android.ApiLevelFromUserWithConfig(ctx.Config(), to.SdkVersion())
			if err != nil {
				ctx.PropertyErrorf("sdk_version",
					"Invalid sdk_version value (must be int, preview or current): %q",
					to.SdkVersion())
			}

			if toApi.GreaterThan(fromApi) {
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

func checkLinkTypeMutator(ctx android.BottomUpMutatorContext) {
	if c, ok := ctx.Module().(*Module); ok {
		ctx.VisitDirectDeps(func(dep android.Module) {
			depTag := ctx.OtherModuleDependencyTag(dep)
			ccDep, ok := dep.(LinkableInterface)
			if ok {
				checkLinkType(ctx, c, ccDep, depTag)
			}
		})
	}
}

// Tests whether the dependent library is okay to be double loaded inside a single process.
// If a library has a vendor variant and is a (transitive) dependency of an LLNDK library,
// it is subject to be double loaded. Such lib should be explicitly marked as double_loadable: true
// or as vndk-sp (vndk: { enabled: true, support_system_process: true}).
func checkDoubleLoadableLibraries(ctx android.BottomUpMutatorContext) {
	check := func(child, parent android.Module) bool {
		to, ok := child.(*Module)
		if !ok {
			return false
		}

		if lib, ok := to.linker.(*libraryDecorator); !ok || !lib.shared() {
			return false
		}

		// These dependencies are not excercised at runtime. Tracking these will give us
		// false negative, so skip.
		depTag := ctx.OtherModuleDependencyTag(child)
		if IsHeaderDepTag(depTag) {
			return false
		}
		if depTag == staticVariantTag {
			return false
		}
		if depTag == StubImplDepTag {
			return false
		}
		if depTag == android.RequiredDepTag {
			return false
		}

		// Even if target lib has no vendor variant, keep checking dependency
		// graph in case it depends on vendor_available or product_available
		// but not double_loadable transtively.
		if !to.HasNonSystemVariants() {
			return true
		}

		// The happy path. Keep tracking dependencies until we hit a non double-loadable
		// one.
		if Bool(to.VendorProperties.Double_loadable) {
			return true
		}

		if to.IsLlndk() {
			return false
		}

		ctx.ModuleErrorf("links a library %q which is not LL-NDK, "+
			"VNDK-SP, or explicitly marked as 'double_loadable:true'. "+
			"Dependency list: %s", ctx.OtherModuleName(to), ctx.GetPathString(false))
		return false
	}
	if module, ok := ctx.Module().(*Module); ok {
		if lib, ok := module.linker.(*libraryDecorator); ok && lib.shared() {
			if lib.HasLLNDKStubs() {
				ctx.WalkDeps(check)
			}
		}
	}
}

func findApexSdkVersion(ctx android.BaseModuleContext, apexInfo android.ApexInfo) android.ApiLevel {
	// For the dependency from platform to apex, use the latest stubs
	apexSdkVersion := android.FutureApiLevel
	if !apexInfo.IsForPlatform() {
		apexSdkVersion = apexInfo.MinSdkVersion
	}

	if android.InList("hwaddress", ctx.Config().SanitizeDevice()) {
		// In hwasan build, we override apexSdkVersion to the FutureApiLevel(10000)
		// so that even Q(29/Android10) apexes could use the dynamic unwinder by linking the newer stubs(e.g libc(R+)).
		// (b/144430859)
		apexSdkVersion = android.FutureApiLevel
	}

	return apexSdkVersion
}

// Convert dependencies to paths.  Returns a PathDeps containing paths
func (c *Module) depsToPaths(ctx android.ModuleContext) PathDeps {
	var depPaths PathDeps

	var directStaticDeps []StaticLibraryInfo
	var directSharedDeps []SharedLibraryInfo

	reexportExporter := func(exporter FlagExporterInfo) {
		depPaths.ReexportedDirs = append(depPaths.ReexportedDirs, exporter.IncludeDirs...)
		depPaths.ReexportedSystemDirs = append(depPaths.ReexportedSystemDirs, exporter.SystemIncludeDirs...)
		depPaths.ReexportedFlags = append(depPaths.ReexportedFlags, exporter.Flags...)
		depPaths.ReexportedDeps = append(depPaths.ReexportedDeps, exporter.Deps...)
		depPaths.ReexportedGeneratedHeaders = append(depPaths.ReexportedGeneratedHeaders, exporter.GeneratedHeaders...)
	}

	apexInfo, _ := android.ModuleProvider(ctx, android.ApexInfoProvider)
	c.apexSdkVersion = findApexSdkVersion(ctx, apexInfo)

	skipModuleList := map[string]bool{}

	ctx.VisitDirectDepsProxy(func(dep android.ModuleProxy) {
		depName := ctx.OtherModuleName(dep)
		depTag := ctx.OtherModuleDependencyTag(dep)

		if _, ok := skipModuleList[depName]; ok {
			// skip this module because original module or API imported module matching with this should be used instead.
			return
		}

		var ccInfo *CcInfo
		v, hasCcInfo := android.OtherModuleProvider(ctx, dep, CcInfoProvider)
		if hasCcInfo {
			ccInfo = v
		}
		linkableInfo, hasLinkableInfo := android.OtherModuleProvider(ctx, dep, LinkableInfoProvider)
		if depTag == android.DarwinUniversalVariantTag {
			if !hasCcInfo {
				panic(fmt.Errorf("dep is not a cc module: %s", dep.String()))
			}
			depPaths.DarwinSecondArchOutput = linkableInfo.OutputFile
			return
		}

		if depTag == aidlLibraryTag {
			if aidlLibraryInfo, ok := android.OtherModuleProvider(ctx, dep, aidl_library.AidlLibraryProvider); ok {
				depPaths.AidlLibraryInfos = append(
					depPaths.AidlLibraryInfos,
					aidlLibraryInfo,
				)
			}
		}

		if !hasLinkableInfo {
			// handling for a few module types that aren't cc Module but that are also supported
			genRule, ok := android.OtherModuleProvider(ctx, dep, android.GeneratedSourceInfoProvider)
			switch depTag {
			case genSourceDepTag:
				if ok {
					depPaths.GeneratedSources = append(depPaths.GeneratedSources,
						genRule.GeneratedSourceFiles...)
				} else {
					ctx.ModuleErrorf("module %q is not a gensrcs or genrule", depName)
				}
				// Support exported headers from a generated_sources dependency
				fallthrough
			case genHeaderDepTag, genHeaderExportDepTag:
				if ok {
					depPaths.GeneratedDeps = append(depPaths.GeneratedDeps,
						genRule.GeneratedDeps...)
					dirs := genRule.GeneratedHeaderDirs
					depPaths.IncludeDirs = append(depPaths.IncludeDirs, dirs...)
					if depTag == genHeaderExportDepTag {
						depPaths.ReexportedDirs = append(depPaths.ReexportedDirs, dirs...)
						depPaths.ReexportedGeneratedHeaders = append(depPaths.ReexportedGeneratedHeaders,
							genRule.GeneratedSourceFiles...)
						depPaths.ReexportedDeps = append(depPaths.ReexportedDeps, genRule.GeneratedDeps...)
						// Add these re-exported flags to help header-abi-dumper to infer the abi exported by a library.
						c.sabi.Properties.ReexportedIncludes = append(c.sabi.Properties.ReexportedIncludes, dirs.Strings()...)
					}
				} else {
					ctx.ModuleErrorf("module %q is not a genrule", depName)
				}
			case CrtBeginDepTag:
				depPaths.CrtBegin = append(depPaths.CrtBegin, android.OutputFileForModule(ctx, dep, ""))
			case CrtEndDepTag:
				depPaths.CrtEnd = append(depPaths.CrtEnd, android.OutputFileForModule(ctx, dep, ""))
			}
			return
		}

		if depTag == android.ProtoPluginDepTag {
			return
		}

		if depTag == android.RequiredDepTag {
			return
		}

		commonInfo := android.OtherModulePointerProviderOrDefault(ctx, dep, android.CommonModuleInfoProvider)
		if commonInfo.Target.Os != ctx.Os() {
			ctx.ModuleErrorf("OS mismatch between %q (%s) and %q (%s)", ctx.ModuleName(), ctx.Os().Name, depName, dep.Target().Os.Name)
			return
		}
		if commonInfo.Target.Arch.ArchType != ctx.Arch().ArchType {
			ctx.ModuleErrorf("Arch mismatch between %q(%v) and %q(%v)",
				ctx.ModuleName(), ctx.Arch().ArchType, depName, commonInfo.Target.Arch.ArchType)
			return
		}

		if depTag == reuseObjTag {
			// Skip reused objects for stub libraries, they use their own stub object file instead.
			// The reuseObjTag dependency still exists because the LinkageMutator runs before the
			// version mutator, so the stubs variant is created from the shared variant that
			// already has the reuseObjTag dependency on the static variant.
			if !c.library.BuildStubs() {
				staticAnalogue, _ := android.OtherModuleProvider(ctx, dep, StaticLibraryInfoProvider)
				objs := staticAnalogue.ReuseObjects
				depPaths.Objs = depPaths.Objs.Append(objs)
				depExporterInfo, _ := android.OtherModuleProvider(ctx, dep, FlagExporterInfoProvider)
				reexportExporter(depExporterInfo)
			}
			return
		}

		if depTag == llndkHeaderLibTag {
			depExporterInfo, _ := android.OtherModuleProvider(ctx, dep, FlagExporterInfoProvider)
			depPaths.LlndkIncludeDirs = append(depPaths.LlndkIncludeDirs, depExporterInfo.IncludeDirs...)
			depPaths.LlndkSystemIncludeDirs = append(depPaths.LlndkSystemIncludeDirs, depExporterInfo.SystemIncludeDirs...)
		}

		linkFile := linkableInfo.OutputFile

		if libDepTag, ok := depTag.(libraryDependencyTag); ok {
			// Only use static unwinder for legacy (min_sdk_version = 29) apexes (b/144430859)
			if libDepTag.staticUnwinder && c.apexSdkVersion.GreaterThan(android.SdkVersion_Android10) {
				return
			}

			if !apexInfo.IsForPlatform() && libDepTag.excludeInApex {
				return
			}
			if apexInfo.IsForPlatform() && libDepTag.excludeInNonApex {
				return
			}

			depExporterInfo, _ := android.OtherModuleProvider(ctx, dep, FlagExporterInfoProvider)

			var ptr *android.Paths
			var depPtr *android.Paths

			depFile := android.OptionalPath{}

			switch {
			case libDepTag.header():
				if _, isHeaderLib := android.OtherModuleProvider(ctx, dep, HeaderLibraryInfoProvider); !isHeaderLib {
					if !ctx.Config().AllowMissingDependencies() {
						ctx.ModuleErrorf("module %q is not a header library", depName)
					} else {
						ctx.AddMissingDependencies([]string{depName})
					}
					return
				}
			case libDepTag.shared():
				if _, isSharedLib := android.OtherModuleProvider(ctx, dep, SharedLibraryInfoProvider); !isSharedLib {
					if !ctx.Config().AllowMissingDependencies() {
						ctx.ModuleErrorf("module %q is not a shared library", depName)
					} else {
						ctx.AddMissingDependencies([]string{depName})
					}
					return
				}

				sharedLibraryInfo, returnedDepExporterInfo := ChooseStubOrImpl(ctx, dep)
				depExporterInfo = returnedDepExporterInfo

				// Stubs lib doesn't link to the shared lib dependencies. Don't set
				// linkFile, depFile, and ptr.
				if c.IsStubs() {
					break
				}

				linkFile = android.OptionalPathForPath(sharedLibraryInfo.SharedLibrary)
				depFile = sharedLibraryInfo.TableOfContents

				if !sharedLibraryInfo.IsStubs {
					// TODO(b/362509506): remove this additional check once all apex_exclude uses are switched to stubs.
					if !linkableInfo.RustApexExclude {
						depPaths.directImplementationDeps = append(depPaths.directImplementationDeps, android.OutputFileForModule(ctx, dep, ""))
						if info, ok := android.OtherModuleProvider(ctx, dep, ImplementationDepInfoProvider); ok {
							depPaths.transitiveImplementationDeps = append(depPaths.transitiveImplementationDeps, info.ImplementationDeps)
						}
					}
				}

				ptr = &depPaths.SharedLibs
				switch libDepTag.Order {
				case earlyLibraryDependency:
					ptr = &depPaths.EarlySharedLibs
					depPtr = &depPaths.EarlySharedLibsDeps
				case normalLibraryDependency:
					ptr = &depPaths.SharedLibs
					depPtr = &depPaths.SharedLibsDeps
					directSharedDeps = append(directSharedDeps, sharedLibraryInfo)
				case lateLibraryDependency:
					ptr = &depPaths.LateSharedLibs
					depPtr = &depPaths.LateSharedLibsDeps
				default:
					panic(fmt.Errorf("unexpected library dependency order %d", libDepTag.Order))
				}

			case libDepTag.static():
				if linkableInfo.RustLibraryInterface {
					rlibDep := RustRlibDep{LibPath: linkFile.Path(), CrateName: linkableInfo.CrateName, LinkDirs: linkableInfo.ExportedCrateLinkDirs}
					depPaths.RustRlibDeps = append(depPaths.RustRlibDeps, rlibDep)
					depPaths.IncludeDirs = append(depPaths.IncludeDirs, depExporterInfo.IncludeDirs...)
					if libDepTag.wholeStatic {
						depPaths.ReexportedDirs = append(depPaths.ReexportedDirs, depExporterInfo.IncludeDirs...)
						depPaths.ReexportedRustRlibDeps = append(depPaths.ReexportedRustRlibDeps, rlibDep)

						// If whole_static, track this as we want to make sure that in a final linkage for a shared library,
						// exported functions from the rust generated staticlib still exported.
						if c.CcLibrary() && c.Shared() {
							c.WholeRustStaticlib = true
						}
					}

				} else {
					staticLibraryInfo, isStaticLib := android.OtherModuleProvider(ctx, dep, StaticLibraryInfoProvider)
					if !isStaticLib {
						if !ctx.Config().AllowMissingDependencies() {
							ctx.ModuleErrorf("module %q is not a static library", depName)
						} else {
							ctx.AddMissingDependencies([]string{depName})
						}
						return
					}

					// Stubs lib doesn't link to the static lib dependencies. Don't set
					// linkFile, depFile, and ptr.
					if c.IsStubs() {
						break
					}

					linkFile = android.OptionalPathForPath(staticLibraryInfo.StaticLibrary)
					if libDepTag.wholeStatic {
						ptr = &depPaths.WholeStaticLibs
						if len(staticLibraryInfo.Objects.objFiles) > 0 {
							depPaths.WholeStaticLibObjs = depPaths.WholeStaticLibObjs.Append(staticLibraryInfo.Objects)
						} else {
							// This case normally catches prebuilt static
							// libraries, but it can also occur when
							// AllowMissingDependencies is on and the
							// dependencies has no sources of its own
							// but has a whole_static_libs dependency
							// on a missing library.  We want to depend
							// on the .a file so that there is something
							// in the dependency tree that contains the
							// error rule for the missing transitive
							// dependency.
							depPaths.WholeStaticLibsFromPrebuilts = append(depPaths.WholeStaticLibsFromPrebuilts, linkFile.Path())
						}
						depPaths.WholeStaticLibsFromPrebuilts = append(depPaths.WholeStaticLibsFromPrebuilts,
							staticLibraryInfo.WholeStaticLibsFromPrebuilts...)
					} else {
						switch libDepTag.Order {
						case earlyLibraryDependency:
							panic(fmt.Errorf("early static libs not supported"))
						case normalLibraryDependency:
							// static dependencies will be handled separately so they can be ordered
							// using transitive dependencies.
							ptr = nil
							directStaticDeps = append(directStaticDeps, staticLibraryInfo)
						case lateLibraryDependency:
							ptr = &depPaths.LateStaticLibs
						default:
							panic(fmt.Errorf("unexpected library dependency order %d", libDepTag.Order))
						}
					}

					// Collect any exported Rust rlib deps from static libraries which have been included as whole_static_libs
					depPaths.RustRlibDeps = append(depPaths.RustRlibDeps, depExporterInfo.RustRlibDeps...)

					if libDepTag.unexportedSymbols {
						depPaths.LdFlags = append(depPaths.LdFlags,
							"-Wl,--exclude-libs="+staticLibraryInfo.StaticLibrary.Base())
					}
				}
			}

			if libDepTag.static() && !libDepTag.wholeStatic && !linkableInfo.RustLibraryInterface {
				if !linkableInfo.CcLibraryInterface || !linkableInfo.Static {
					ctx.ModuleErrorf("module %q not a static library", depName)
					return
				}

				// When combining coverage files for shared libraries and executables, coverage files
				// in static libraries act as if they were whole static libraries. The same goes for
				// source based Abi dump files.
				if hasCcInfo {
					depPaths.StaticLibObjs.coverageFiles = append(depPaths.StaticLibObjs.coverageFiles,
						linkableInfo.CoverageFiles...)
					depPaths.StaticLibObjs.sAbiDumpFiles = append(depPaths.StaticLibObjs.sAbiDumpFiles,
						linkableInfo.SAbiDumpFiles...)
				} else {
					// Handle non-CC modules here
					depPaths.StaticLibObjs.coverageFiles = append(depPaths.StaticLibObjs.coverageFiles,
						linkableInfo.CoverageFiles...)
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

			depPaths.IncludeDirs = append(depPaths.IncludeDirs, depExporterInfo.IncludeDirs...)
			depPaths.SystemIncludeDirs = append(depPaths.SystemIncludeDirs, depExporterInfo.SystemIncludeDirs...)
			depPaths.GeneratedDeps = append(depPaths.GeneratedDeps, depExporterInfo.Deps...)
			depPaths.Flags = append(depPaths.Flags, depExporterInfo.Flags...)
			depPaths.RustRlibDeps = append(depPaths.RustRlibDeps, depExporterInfo.RustRlibDeps...)

			// Only re-export RustRlibDeps for cc static libs
			if c.static() {
				depPaths.ReexportedRustRlibDeps = append(depPaths.ReexportedRustRlibDeps, depExporterInfo.RustRlibDeps...)
			}

			if libDepTag.reexportFlags {
				reexportExporter(depExporterInfo)
				// Add these re-exported flags to help header-abi-dumper to infer the abi exported by a library.
				// Re-exported shared library headers must be included as well since they can help us with type information
				// about template instantiations (instantiated from their headers).
				c.sabi.Properties.ReexportedIncludes = append(
					c.sabi.Properties.ReexportedIncludes, depExporterInfo.IncludeDirs.Strings()...)
				c.sabi.Properties.ReexportedSystemIncludes = append(
					c.sabi.Properties.ReexportedSystemIncludes, depExporterInfo.SystemIncludeDirs.Strings()...)
			}

			makeLibName := MakeLibName(ccInfo, linkableInfo, commonInfo, commonInfo.BaseModuleName) + libDepTag.makeSuffix
			switch {
			case libDepTag.header():
				c.Properties.AndroidMkHeaderLibs = append(
					c.Properties.AndroidMkHeaderLibs, makeLibName)
			case libDepTag.shared():
				// Note: the order of libs in this list is not important because
				// they merely serve as Make dependencies and do not affect this lib itself.
				c.Properties.AndroidMkSharedLibs = append(
					c.Properties.AndroidMkSharedLibs, makeLibName)
			case libDepTag.static():
				if !linkableInfo.RustLibraryInterface {
					if libDepTag.wholeStatic {
						c.Properties.AndroidMkWholeStaticLibs = append(
							c.Properties.AndroidMkWholeStaticLibs, makeLibName)
					} else {
						c.Properties.AndroidMkStaticLibs = append(
							c.Properties.AndroidMkStaticLibs, makeLibName)
					}
				}
			}
		} else if !c.IsStubs() {
			// Stubs lib doesn't link to the runtime lib, object, crt, etc. dependencies.

			switch depTag {
			case runtimeDepTag:
				c.Properties.AndroidMkRuntimeLibs = append(
					c.Properties.AndroidMkRuntimeLibs, MakeLibName(ccInfo, linkableInfo, commonInfo,
						commonInfo.BaseModuleName)+libDepTag.makeSuffix)
			case objDepTag:
				depPaths.Objs.objFiles = append(depPaths.Objs.objFiles, linkFile.Path())
			case CrtBeginDepTag:
				depPaths.CrtBegin = append(depPaths.CrtBegin, linkFile.Path())
			case CrtEndDepTag:
				depPaths.CrtEnd = append(depPaths.CrtEnd, linkFile.Path())
			case dynamicLinkerDepTag:
				depPaths.DynamicLinker = linkFile
			}
		}
	})

	// use the ordered dependencies as this module's dependencies
	orderedStaticPaths, transitiveStaticLibs := orderStaticModuleDeps(directStaticDeps, directSharedDeps)
	depPaths.TranstiveStaticLibrariesForOrdering = transitiveStaticLibs
	depPaths.StaticLibs = append(depPaths.StaticLibs, orderedStaticPaths...)

	// Dedup exported flags from dependencies
	depPaths.Flags = android.FirstUniqueStrings(depPaths.Flags)
	depPaths.IncludeDirs = android.FirstUniquePaths(depPaths.IncludeDirs)
	depPaths.SystemIncludeDirs = android.FirstUniquePaths(depPaths.SystemIncludeDirs)
	depPaths.GeneratedDeps = android.FirstUniquePaths(depPaths.GeneratedDeps)
	depPaths.RustRlibDeps = android.FirstUniqueFunc(depPaths.RustRlibDeps, EqRustRlibDeps)

	depPaths.ReexportedDirs = android.FirstUniquePaths(depPaths.ReexportedDirs)
	depPaths.ReexportedSystemDirs = android.FirstUniquePaths(depPaths.ReexportedSystemDirs)
	depPaths.ReexportedFlags = android.FirstUniqueStrings(depPaths.ReexportedFlags)
	depPaths.ReexportedDeps = android.FirstUniquePaths(depPaths.ReexportedDeps)
	depPaths.ReexportedGeneratedHeaders = android.FirstUniquePaths(depPaths.ReexportedGeneratedHeaders)
	depPaths.ReexportedRustRlibDeps = android.FirstUniqueFunc(depPaths.ReexportedRustRlibDeps, EqRustRlibDeps)

	if c.sabi != nil {
		c.sabi.Properties.ReexportedIncludes = android.FirstUniqueStrings(c.sabi.Properties.ReexportedIncludes)
		c.sabi.Properties.ReexportedSystemIncludes = android.FirstUniqueStrings(c.sabi.Properties.ReexportedSystemIncludes)
	}

	return depPaths
}

func ShouldUseStubForApex(ctx android.ModuleContext, parent android.Module, dep android.ModuleProxy) bool {
	inVendorOrProduct := false
	bootstrap := false
	if android.EqualModules(ctx.Module(), parent) {
		if linkable, ok := parent.(LinkableInterface); !ok {
			ctx.ModuleErrorf("Not a Linkable module: %q", ctx.ModuleName())
		} else {
			inVendorOrProduct = linkable.InVendorOrProduct()
			bootstrap = linkable.Bootstrap()
		}
	} else {
		if linkable, ok := android.OtherModuleProvider(ctx, parent, LinkableInfoProvider); !ok {
			ctx.ModuleErrorf("Not a Linkable module: %q", ctx.ModuleName())
		} else {
			inVendorOrProduct = linkable.InVendorOrProduct
			bootstrap = linkable.Bootstrap
		}
	}

	apexInfo, _ := android.OtherModuleProvider(ctx, parent, android.ApexInfoProvider)

	useStubs := false

	if android.OtherModuleProviderOrDefault(ctx, dep, LinkableInfoProvider).IsStubs && inVendorOrProduct { // LLNDK
		if !apexInfo.IsForPlatform() {
			// For platform libraries, use current version of LLNDK
			// If this is for use_vendor apex we will apply the same rules
			// of apex sdk enforcement below to choose right version.
			useStubs = true
		}
	} else if apexInfo.IsForPlatform() || apexInfo.UsePlatformApis {
		// If not building for APEX or the containing APEX allows the use of
		// platform APIs, use stubs only when it is from an APEX (and not from
		// platform) However, for host, ramdisk, vendor_ramdisk, recovery or
		// bootstrap modules, always link to non-stub variant
		isNotInPlatform := android.OtherModulePointerProviderOrDefault(ctx, dep, android.CommonModuleInfoProvider).NotInPlatform

		useStubs = isNotInPlatform && !bootstrap
	} else {
		// If building for APEX, always use stubs (can be bypassed by depending on <dep>#impl)
		useStubs = true
	}

	return useStubs
}

// ChooseStubOrImpl determines whether a given dependency should be redirected to the stub variant
// of the dependency or not, and returns the SharedLibraryInfo and FlagExporterInfo for the right
// dependency. The stub variant is selected when the dependency crosses a boundary where each side
// has different level of updatability. For example, if a library foo in an APEX depends on a
// library bar which provides stable interface and exists in the platform, foo uses the stub variant
// of bar. If bar doesn't provide a stable interface (i.e. buildStubs() == false) or is in the
// same APEX as foo, the non-stub variant of bar is used.
func ChooseStubOrImpl(ctx android.ModuleContext, dep android.ModuleProxy) (SharedLibraryInfo, FlagExporterInfo) {
	depTag := ctx.OtherModuleDependencyTag(dep)
	libDepTag, ok := depTag.(libraryDependencyTag)
	if !ok || !libDepTag.shared() {
		panic(fmt.Errorf("Unexpected dependency tag: %T", depTag))
	}

	sharedLibraryInfo, _ := android.OtherModuleProvider(ctx, dep, SharedLibraryInfoProvider)
	depExporterInfo, _ := android.OtherModuleProvider(ctx, dep, FlagExporterInfoProvider)
	sharedLibraryStubsInfo, _ := android.OtherModuleProvider(ctx, dep, SharedLibraryStubsProvider)

	if !libDepTag.explicitlyVersioned && len(sharedLibraryStubsInfo.SharedStubLibraries) > 0 {
		// when to use (unspecified) stubs, use the latest one.
		if ShouldUseStubForApex(ctx, ctx.Module(), dep) {
			stubs := sharedLibraryStubsInfo.SharedStubLibraries
			toUse := stubs[len(stubs)-1]
			sharedLibraryInfo = toUse.SharedLibraryInfo
			depExporterInfo = toUse.FlagExporterInfo
		}
	}
	return sharedLibraryInfo, depExporterInfo
}

// orderStaticModuleDeps rearranges the order of the static library dependencies of the module
// to match the topological order of the dependency tree, including any static analogues of
// direct shared libraries.  It returns the ordered static dependencies, and a depset.DepSet
// of the transitive dependencies.
func orderStaticModuleDeps(staticDeps []StaticLibraryInfo, sharedDeps []SharedLibraryInfo) (ordered android.Paths, transitive depset.DepSet[android.Path]) {
	transitiveStaticLibsBuilder := depset.NewBuilder[android.Path](depset.TOPOLOGICAL)
	var staticPaths android.Paths
	for _, staticDep := range staticDeps {
		staticPaths = append(staticPaths, staticDep.StaticLibrary)
		transitiveStaticLibsBuilder.Transitive(staticDep.TransitiveStaticLibrariesForOrdering)
	}
	for _, sharedDep := range sharedDeps {
		transitiveStaticLibsBuilder.Transitive(sharedDep.TransitiveStaticLibrariesForOrdering)
	}
	transitiveStaticLibs := transitiveStaticLibsBuilder.Build()

	orderedTransitiveStaticLibs := transitiveStaticLibs.ToList()

	// reorder the dependencies based on transitive dependencies
	staticPaths = android.FirstUniquePaths(staticPaths)
	_, orderedStaticPaths := android.FilterPathList(orderedTransitiveStaticLibs, staticPaths)

	if len(orderedStaticPaths) != len(staticPaths) {
		missing, _ := android.FilterPathList(staticPaths, orderedStaticPaths)
		panic(fmt.Errorf("expected %d ordered static paths , got %d, missing %q %q %q", len(staticPaths), len(orderedStaticPaths), missing, orderedStaticPaths, staticPaths))
	}

	return orderedStaticPaths, transitiveStaticLibs
}

// BaseLibName trims known prefixes and suffixes
func BaseLibName(depName string) string {
	libName := strings.TrimSuffix(depName, llndkLibrarySuffix)
	libName = strings.TrimSuffix(libName, vendorPublicLibrarySuffix)
	libName = android.RemoveOptionalPrebuiltPrefix(libName)
	return libName
}

func MakeLibName(ccInfo *CcInfo, linkableInfo *LinkableInfo, commonInfo *android.CommonModuleInfo, depName string) string {
	libName := BaseLibName(depName)
	isLLndk := ccInfo != nil && linkableInfo.IsLlndk
	nonSystemVariantsExist := linkableInfo.HasNonSystemVariants || isLLndk

	if ccInfo != nil {
		// Use base module name for snapshots when exporting to Makefile.
		if ccInfo.SnapshotInfo != nil {
			return commonInfo.BaseModuleName + ccInfo.SnapshotInfo.SnapshotAndroidMkSuffix
		}
	}

	if linkableInfo.InVendorOrProduct && nonSystemVariantsExist {
		// The vendor and product modules in Make will have been renamed to not conflict with the
		// core module, so update the dependency name here accordingly.
		return libName + linkableInfo.SubName
	} else if linkableInfo.InRamdisk && !linkableInfo.OnlyInRamdisk {
		return libName + RamdiskSuffix
	} else if linkableInfo.InVendorRamdisk && !linkableInfo.OnlyInVendorRamdisk {
		return libName + VendorRamdiskSuffix
	} else if linkableInfo.InRecovery && !linkableInfo.OnlyInRecovery {
		return libName + RecoverySuffix
	} else if commonInfo.Target.NativeBridge == android.NativeBridgeEnabled {
		return libName + NativeBridgeSuffix
	} else {
		return libName
	}
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

func (c *Module) InstallInVendorRamdisk() bool {
	return c.InVendorRamdisk()
}

func (c *Module) InstallInRecovery() bool {
	return c.InRecovery()
}

func (c *Module) MakeUninstallable() {
	if c.installer == nil {
		c.ModuleBase.MakeUninstallable()
		return
	}
	c.installer.makeUninstallable(c)
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

func (c *Module) static() bool {
	if static, ok := c.linker.(interface {
		static() bool
	}); ok {
		return static.static()
	}
	return false
}

func (c *Module) staticLibrary() bool {
	if static, ok := c.linker.(interface {
		staticLibrary() bool
	}); ok {
		return static.staticLibrary()
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

func (c *Module) testBinary() bool {
	if test, ok := c.linker.(interface {
		testBinary() bool
	}); ok {
		return test.testBinary()
	}
	return false
}

func (c *Module) testLibrary() bool {
	if test, ok := c.linker.(interface {
		testLibrary() bool
	}); ok {
		return test.testLibrary()
	}
	return false
}

func (c *Module) benchmarkBinary() bool {
	if b, ok := c.linker.(interface {
		benchmarkBinary() bool
	}); ok {
		return b.benchmarkBinary()
	}
	return false
}

func (c *Module) fuzzBinary() bool {
	if f, ok := c.linker.(interface {
		fuzzBinary() bool
	}); ok {
		return f.fuzzBinary()
	}
	return false
}

// Header returns true if the module is a header-only variant. (See cc/library.go header()).
func (c *Module) Header() bool {
	if h, ok := c.linker.(interface {
		header() bool
	}); ok {
		return h.header()
	}
	return false
}

func (c *Module) Binary() bool {
	if b, ok := c.linker.(interface {
		binary() bool
	}); ok {
		return b.binary()
	}
	return false
}

func (c *Module) ForceDisableSanitizers() {
	c.sanitize.Properties.ForceDisable = true
}

func (c *Module) StaticExecutable() bool {
	if b, ok := c.linker.(*binaryDecorator); ok {
		return b.static()
	}
	return false
}

func (c *Module) Object() bool {
	if o, ok := c.linker.(interface {
		object() bool
	}); ok {
		return o.object()
	}
	return false
}

func (m *Module) Dylib() bool {
	return false
}

func (m *Module) Rlib() bool {
	return false
}

func GetMakeLinkType(actx android.ModuleContext, c LinkableInterface) string {
	if c.InVendorOrProduct() {
		if c.IsLlndk() {
			return "native:vndk"
		}
		if c.InProduct() {
			return "native:product"
		}
		return "native:vendor"
	} else if c.InRamdisk() {
		return "native:ramdisk"
	} else if c.InVendorRamdisk() {
		return "native:vendor_ramdisk"
	} else if c.InRecovery() {
		return "native:recovery"
	} else if c.Target().Os == android.Android && c.SdkVersion() != "" {
		return "native:ndk:none:none"
		// TODO(b/114741097): use the correct ndk stl once build errors have been fixed
		//family, link := getNdkStlFamilyAndLinkType(c)
		//return fmt.Sprintf("native:ndk:%s:%s", family, link)
	} else {
		return "native:platform"
	}
}

// Overrides ApexModule.IsInstallabeToApex()
// Only shared/runtime libraries .
func (c *Module) IsInstallableToApex() bool {
	if lib := c.library; lib != nil {
		// Stub libs and prebuilt libs in a versioned SDK are not
		// installable to APEX even though they are shared libs.
		return lib.shared() && !lib.BuildStubs()
	}
	return false
}

func (c *Module) AvailableFor(what string) bool {
	return android.CheckAvailableForApex(what, c.ApexAvailableFor())
}

func (c *Module) ApexAvailableFor() []string {
	list := c.ApexModuleBase.ApexAvailable()
	if linker, ok := c.linker.(interface {
		apexAvailable() []string
	}); ok {
		list = append(list, linker.apexAvailable()...)
	}

	return android.FirstUniqueStrings(list)
}

func (c *Module) EverInstallable() bool {
	return c.installer != nil &&
		// Check to see whether the module is actually ever installable.
		c.installer.everInstallable()
}

func (c *Module) PreventInstall() bool {
	return c.Properties.PreventInstall
}

func (c *Module) Installable() *bool {
	if c.library != nil {
		if i := c.library.installable(); i != nil {
			return i
		}
	}
	return c.Properties.Installable
}

func installable(c LinkableInterface, apexInfo android.ApexInfo) bool {
	ret := c.EverInstallable() &&
		// Check to see whether the module has been configured to not be installed.
		proptools.BoolDefault(c.Installable(), true) &&
		!c.PreventInstall() && c.OutputFile().Valid()

	// The platform variant doesn't need further condition. Apex variants however might not
	// be installable because it will likely to be included in the APEX and won't appear
	// in the system partition.
	if apexInfo.IsForPlatform() {
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

var _ android.ApexModule = (*Module)(nil)

// Implements android.ApexModule
func (c *Module) GetDepInSameApexChecker() android.DepInSameApexChecker {
	return CcDepInSameApexChecker{
		Static:           c.static(),
		HasStubsVariants: c.HasStubsVariants(),
		IsLlndk:          c.IsLlndk(),
		Host:             c.Host(),
	}
}

type CcDepInSameApexChecker struct {
	Static           bool
	HasStubsVariants bool
	IsLlndk          bool
	Host             bool
}

func (c CcDepInSameApexChecker) OutgoingDepIsInSameApex(depTag blueprint.DependencyTag) bool {
	if depTag == StubImplDepTag {
		// We don't track from an implementation library to its stubs.
		return false
	}
	if depTag == staticVariantTag {
		// This dependency is for optimization (reuse *.o from the static lib). It doesn't
		// actually mean that the static lib (and its dependencies) are copied into the
		// APEX.
		return false
	}

	libDepTag, isLibDepTag := depTag.(libraryDependencyTag)
	if isLibDepTag && c.Static && libDepTag.shared() {
		// shared_lib dependency from a static lib is considered as crossing
		// the APEX boundary because the dependency doesn't actually is
		// linked; the dependency is used only during the compilation phase.
		return false
	}

	if isLibDepTag && libDepTag.excludeInApex {
		return false
	}

	return true
}

func (c CcDepInSameApexChecker) IncomingDepIsInSameApex(depTag blueprint.DependencyTag) bool {
	if c.Host {
		return false
	}
	if c.HasStubsVariants {
		if IsSharedDepTag(depTag) && !IsExplicitImplSharedDepTag(depTag) {
			// dynamic dep to a stubs lib crosses APEX boundary
			return false
		}
		if IsRuntimeDepTag(depTag) {
			// runtime dep to a stubs lib also crosses APEX boundary
			return false
		}
		if IsHeaderDepTag(depTag) {
			return false
		}
	}
	if c.IsLlndk {
		return false
	}

	return true
}

// Implements android.ApexModule
func (c *Module) MinSdkVersionSupported(ctx android.BaseModuleContext) android.ApiLevel {
	// We ignore libclang_rt.* prebuilt libs since they declare sdk_version: 14(b/121358700)
	if strings.HasPrefix(ctx.OtherModuleName(c), "libclang_rt") {
		return android.MinApiLevel
	}
	// We don't check for prebuilt modules
	if _, ok := c.linker.(prebuiltLinkerInterface); ok {
		return android.MinApiLevel
	}

	minSdkVersion := c.MinSdkVersion()
	if minSdkVersion == "apex_inherit" {
		return android.MinApiLevel
	}
	if minSdkVersion == "" {
		// JNI libs within APK-in-APEX fall into here
		// Those are okay to set sdk_version instead
		// We don't have to check if this is a SDK variant because
		// non-SDK variant resets sdk_version, which works too.
		minSdkVersion = c.SdkVersion()
	}

	if minSdkVersion == "" {
		return android.NoneApiLevel
	}

	// Not using nativeApiLevelFromUser because the context here is not
	// necessarily a native context.
	ver, err := android.ApiLevelFromUserWithConfig(ctx.Config(), minSdkVersion)
	if err != nil {
		return android.NoneApiLevel
	}

	// A dependency only needs to support a min_sdk_version at least
	// as high as  the api level that the architecture was introduced in.
	// This allows introducing new architectures in the platform that
	// need to be included in apexes that normally require an older
	// min_sdk_version.
	if c.Enabled(ctx) {
		minApiForArch := MinApiForArch(ctx, c.Target().Arch.ArchType)
		if ver.LessThanOrEqualTo(minApiForArch) {
			ver = android.MinApiLevel
		}
	}

	return ver
}

// Implements android.ApexModule
func (c *Module) AlwaysRequiresPlatformApexVariant() bool {
	// stub libraries and native bridge libraries are always available to platform
	return c.IsStubs() || c.Target().NativeBridge == android.NativeBridgeEnabled
}

func (c *Module) overriddenModules() []string {
	if o, ok := c.linker.(overridable); ok {
		return o.overriddenModules()
	}
	return nil
}

type moduleType int

const (
	unknownType moduleType = iota
	binary
	object
	fullLibrary
	staticLibrary
	sharedLibrary
	headerLibrary
	testBin // testBinary already declared
	ndkLibrary
)

func (c *Module) typ() moduleType {
	if c.testBinary() {
		// testBinary is also a binary, so this comes before the c.Binary()
		// conditional. A testBinary has additional implicit dependencies and
		// other test-only semantics.
		return testBin
	} else if c.Binary() {
		return binary
	} else if c.Object() {
		return object
	} else if c.testLibrary() {
		// TODO(b/244431896) properly convert cc_test_library to its own macro. This
		// will let them add implicit compile deps on gtest, for example.
		//
		// For now, treat them as regular libraries.
		return fullLibrary
	} else if c.CcLibrary() {
		static := false
		shared := false
		if library, ok := c.linker.(*libraryDecorator); ok {
			static = library.MutatedProperties.BuildStatic
			shared = library.MutatedProperties.BuildShared
		} else if library, ok := c.linker.(*prebuiltLibraryLinker); ok {
			static = library.MutatedProperties.BuildStatic
			shared = library.MutatedProperties.BuildShared
		}
		if static && shared {
			return fullLibrary
		} else if !static && !shared {
			return headerLibrary
		} else if static {
			return staticLibrary
		}
		return sharedLibrary
	} else if c.isNDKStubLibrary() {
		return ndkLibrary
	}
	return unknownType
}

// Defaults
type Defaults struct {
	android.ModuleBase
	android.DefaultsModuleBase
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
		&TestLinkerProperties{},
		&TestInstallerProperties{},
		&TestBinaryProperties{},
		&BenchmarkProperties{},
		&fuzz.FuzzProperties{},
		&StlProperties{},
		&SanitizeProperties{},
		&StripProperties{},
		&InstallerProperties{},
		&TidyProperties{},
		&CoverageProperties{},
		&SAbiProperties{},
		&LTOProperties{},
		&AfdoProperties{},
		&OrderfileProperties{},
		&android.ProtoProperties{},
		// RustBindgenProperties is included here so that cc_defaults can be used for rust_bindgen modules.
		&RustBindgenClangProperties{},
		&prebuiltLinkerProperties{},
	)

	android.InitDefaultsModule(module)

	return module
}

func (c *Module) IsSdkVariant() bool {
	return c.Properties.IsSdkVariant
}

func kytheExtractAllFactory() android.Singleton {
	return &kytheExtractAllSingleton{}
}

type kytheExtractAllSingleton struct {
}

func (ks *kytheExtractAllSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	var xrefTargets android.Paths
	ctx.VisitAllModuleProxies(func(module android.ModuleProxy) {
		files := android.OtherModuleProviderOrDefault(ctx, module, CcObjectInfoProvider).KytheFiles
		if len(files) > 0 {
			xrefTargets = append(xrefTargets, files...)
		}
	})
	// TODO(asmundak): Perhaps emit a rule to output a warning if there were no xrefTargets
	if len(xrefTargets) > 0 {
		ctx.Phony("xref_cxx", xrefTargets...)
	}
}

func (c *Module) Partition() string {
	if p, ok := c.installer.(interface {
		getPartition() string
	}); ok {
		return p.getPartition()
	}
	return ""
}

type sourceModuleName interface {
	sourceModuleName() string
}

func (c *Module) BaseModuleName() string {
	if smn, ok := c.linker.(sourceModuleName); ok && smn.sourceModuleName() != "" {
		// if the prebuilt module sets a source_module_name in Android.bp, use that
		return smn.sourceModuleName()
	}
	return c.ModuleBase.BaseModuleName()
}

func (c *Module) stubsSymbolFilePath() android.Path {
	if library, ok := c.linker.(*libraryDecorator); ok {
		return library.stubsSymbolFilePath
	}
	return android.OptionalPath{}.Path()
}

var Bool = proptools.Bool
var BoolDefault = proptools.BoolDefault
var BoolPtr = proptools.BoolPtr
var String = proptools.String
var StringPtr = proptools.StringPtr
