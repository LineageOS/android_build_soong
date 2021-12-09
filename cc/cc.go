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
	"android/soong/fuzz"
	"android/soong/genrule"
	"android/soong/snapshot"
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
		ctx.BottomUp("test_per_src", TestPerSrcMutator).Parallel()
		ctx.BottomUp("version_selector", versionSelectorMutator).Parallel()
		ctx.BottomUp("version", versionMutator).Parallel()
		ctx.BottomUp("begin", BeginMutator).Parallel()
		ctx.BottomUp("sysprop_cc", SyspropMutator).Parallel()
	})

	ctx.PostDepsMutators(func(ctx android.RegisterMutatorsContext) {
		for _, san := range Sanitizers {
			san.registerMutators(ctx)
		}

		ctx.TopDown("sanitize_runtime_deps", sanitizerRuntimeDepsMutator).Parallel()
		ctx.BottomUp("sanitize_runtime", sanitizerRuntimeMutator).Parallel()

		ctx.BottomUp("coverage", coverageMutator).Parallel()

		ctx.TopDown("afdo_deps", afdoDepsMutator)
		ctx.BottomUp("afdo", afdoMutator).Parallel()

		ctx.TopDown("lto_deps", ltoDepsMutator)
		ctx.BottomUp("lto", ltoMutator).Parallel()

		ctx.BottomUp("check_linktype", checkLinkTypeMutator).Parallel()
		ctx.TopDown("double_loadable", checkDoubleLoadableLibraries).Parallel()
	})

	ctx.FinalDepsMutators(func(ctx android.RegisterMutatorsContext) {
		// sabi mutator needs to be run after apex mutator finishes.
		ctx.TopDown("sabi_deps", sabiDepsMutator)
	})

	ctx.RegisterSingletonType("kythe_extract_all", kytheExtractAllFactory)
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

	// Used for data dependencies adjacent to tests
	DataLibs []string
	DataBins []string

	// Used by DepsMutator to pass system_shared_libs information to check_elf_file.py.
	SystemSharedLibs []string

	// If true, statically link the unwinder into native libraries/binaries.
	StaticUnwinderIfLegacy bool

	ReexportSharedLibHeaders, ReexportStaticLibHeaders, ReexportHeaderLibHeaders []string

	ObjFiles []string

	GeneratedSources []string
	GeneratedHeaders []string
	GeneratedDeps    []string

	ReexportGeneratedHeaders []string

	CrtBegin, CrtEnd []string

	// Used for host bionic
	DynamicLinker string

	// List of libs that need to be excluded for APEX variant
	ExcludeLibsForApex []string
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

	// Transitive static library dependencies of static libraries for use in ordering.
	TranstiveStaticLibrariesForOrdering *android.DepSet

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
	IncludeDirs                android.Paths
	SystemIncludeDirs          android.Paths
	ReexportedDirs             android.Paths
	ReexportedSystemDirs       android.Paths
	ReexportedFlags            []string
	ReexportedGeneratedHeaders android.Paths
	ReexportedDeps             android.Paths

	// Paths to crt*.o files
	CrtBegin, CrtEnd android.Paths

	// Path to the dynamic linker binary
	DynamicLinker android.OptionalPath

	// For Darwin builds, the path to the second architecture's output that should
	// be combined with this architectures's output into a FAT MachO file.
	DarwinSecondArchOutput android.OptionalPath
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

	Toolchain     config.Toolchain
	Tidy          bool // True if ninja .tidy rules should be generated.
	NeedTidyFiles bool // True if module link should depend on .tidy files
	GcovCoverage  bool // True if coverage files should be generated.
	SAbiDump      bool // True if header abi dumps should be generated.
	EmitXrefs     bool // If true, generate Ninja rules to generate emitXrefs input files for Kythe

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

	AndroidMkSharedLibs       []string `blueprint:"mutated"`
	AndroidMkStaticLibs       []string `blueprint:"mutated"`
	AndroidMkRuntimeLibs      []string `blueprint:"mutated"`
	AndroidMkWholeStaticLibs  []string `blueprint:"mutated"`
	AndroidMkHeaderLibs       []string `blueprint:"mutated"`
	HideFromMake              bool     `blueprint:"mutated"`
	PreventInstall            bool     `blueprint:"mutated"`
	ApexesProvidingSharedLibs []string `blueprint:"mutated"`

	// Set by DepsMutator.
	AndroidMkSystemSharedLibs []string `blueprint:"mutated"`

	// The name of the image this module is built for, suffixed with a '.'
	ImageVariationPrefix string `blueprint:"mutated"`

	// The VNDK version this module is built against. If empty, the module is not
	// build against the VNDK.
	VndkVersion string `blueprint:"mutated"`

	// Suffix for the name of Android.mk entries generated by this module
	SubName string `blueprint:"mutated"`

	// *.logtags files, to combine together in order to generate the /system/etc/event-log-tags
	// file
	Logtags []string

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

	// Even if DeviceConfig().VndkUseCoreVariant() is set, this module must use vendor variant.
	// see soong/cc/config/vndk.go
	MustUseVendorVariant bool `blueprint:"mutated"`

	// Used by vendor snapshot to record dependencies from snapshot modules.
	SnapshotSharedLibs  []string `blueprint:"mutated"`
	SnapshotStaticLibs  []string `blueprint:"mutated"`
	SnapshotRuntimeLibs []string `blueprint:"mutated"`

	Installable *bool `android:"arch_variant"`

	// Set by factories of module types that can only be referenced from variants compiled against
	// the SDK.
	AlwaysSdk bool `blueprint:"mutated"`

	// Variant is an SDK variant created by sdkMutator
	IsSdkVariant bool `blueprint:"mutated"`
	// Set when both SDK and platform variants are exported to Make to trigger renaming the SDK
	// variant to have a ".sdk" suffix.
	SdkAndPlatformVariantVisibleToMake bool `blueprint:"mutated"`

	// Normally Soong uses the directory structure to decide which modules
	// should be included (framework) or excluded (non-framework) from the
	// different snapshots (vendor, recovery, etc.), but this property
	// allows a partner to exclude a module normally thought of as a
	// framework module from the vendor snapshot.
	Exclude_from_vendor_snapshot *bool

	// Normally Soong uses the directory structure to decide which modules
	// should be included (framework) or excluded (non-framework) from the
	// different snapshots (vendor, recovery, etc.), but this property
	// allows a partner to exclude a module normally thought of as a
	// framework module from the recovery snapshot.
	Exclude_from_recovery_snapshot *bool

	// List of APEXes that this module has private access to for testing purpose. The module
	// can depend on libraries that are not exported by the APEXes and use private symbols
	// from the exported libraries.
	Test_for []string `android:"arch_variant"`

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

	// IsVNDKUsingCoreVariant is true for VNDK modules if the global VndkUseCoreVariant option is
	// set and the module is not listed in VndkMustUseVendorVariantList.
	IsVNDKUsingCoreVariant bool `blueprint:"mutated"`

	// IsVNDKCore is set if a VNDK module does not set the vndk.support_system_process property.
	IsVNDKCore bool `blueprint:"mutated"`

	// IsVNDKSP is set if a VNDK module sets the vndk.support_system_process property.
	IsVNDKSP bool `blueprint:"mutated"`

	// IsVNDKPrivate is set if a VNDK module sets the vndk.private property or an LLNDK
	// module sets the llndk.private property.
	IsVNDKPrivate bool `blueprint:"mutated"`

	// IsVNDKProduct is set if a VNDK module sets the product_available property.
	IsVNDKProduct bool `blueprint:"mutated"`

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
	testBinary() bool
	header() bool
	binary() bool
	object() bool
	toolchain() config.Toolchain
	canUseSdk() bool
	useSdk() bool
	sdkVersion() string
	minSdkVersion() string
	isSdkVariant() bool
	useVndk() bool
	isNdk(config android.Config) bool
	IsLlndk() bool
	IsLlndkPublic() bool
	isImplementationForLLNDKPublic() bool
	IsVndkPrivate() bool
	isVndk() bool
	isVndkSp() bool
	IsVndkExt() bool
	IsVendorPublicLibrary() bool
	inProduct() bool
	inVendor() bool
	inRamdisk() bool
	inVendorRamdisk() bool
	inRecovery() bool
	selectedStl() string
	baseModuleName() string
	getVndkExtendsModuleName() string
	isAfdoCompile() bool
	isPgoCompile() bool
	isNDKStubLibrary() bool
	useClangLld(actx ModuleContext) bool
	isForPlatform() bool
	apexVariationName() string
	apexSdkVersion() android.ApiLevel
	bootstrap() bool
	mustUseVendorVariant() bool
	nativeCoverage() bool
	directlyInAnyApex() bool
	isPreventInstall() bool
	isCfiAssemblySupportEnabled() bool
	getSharedFlags() *SharedFlags
}

type SharedFlags struct {
	numSharedFlags int
	flagsMap       map[string]string
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

// compiler is the interface for a compiler helper object. Different module decorators may implement
// this helper differently.
type compiler interface {
	compilerInit(ctx BaseModuleContext)
	compilerDeps(ctx DepsContext, deps Deps) Deps
	compilerFlags(ctx ModuleContext, flags Flags, deps PathDeps) Flags
	compilerProps() []interface{}

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
	useClangLld(actx ModuleContext) bool

	link(ctx ModuleContext, flags Flags, deps PathDeps, objs Objects) android.Path
	appendLdflags([]string)
	unstrippedOutputFilePath() android.Path

	nativeCoverage() bool
	coverageOutputFilePath() android.OptionalPath

	// Get the deps that have been explicitly specified in the properties.
	linkerSpecifiedDeps(specifiedDeps specifiedDeps) specifiedDeps
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

type xref interface {
	XrefCcFiles() android.Paths
}

type libraryDependencyKind int

const (
	headerLibraryDependency = iota
	sharedLibraryDependency
	staticLibraryDependency
)

func (k libraryDependencyKind) String() string {
	switch k {
	case headerLibraryDependency:
		return "headerLibraryDependency"
	case sharedLibraryDependency:
		return "sharedLibraryDependency"
	case staticLibraryDependency:
		return "staticLibraryDependency"
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
	dataLib             bool
	ndk                 bool

	staticUnwinder bool

	makeSuffix string

	// Whether or not this dependency should skip the apex dependency check
	skipApexAllowedDependenciesCheck bool

	// Whether or not this dependency has to be followed for the apex variants
	excludeInApex bool
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
	testPerSrcDepTag      = dependencyTag{name: "test_per_src"}
	stubImplDepTag        = dependencyTag{name: "stub_impl"}
)

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

func IsTestPerSrcDepTag(depTag blueprint.DependencyTag) bool {
	ccDepTag, ok := depTag.(dependencyTag)
	return ok && ccDepTag == testPerSrcDepTag
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

	android.SdkBase
	android.BazelModuleBase

	VendorProperties VendorProperties
	Properties       BaseProperties

	// initialize before calling Init
	hod       android.HostOrDeviceSupported
	multilib  android.Multilib
	bazelable bool

	// Allowable SdkMemberTypes of this module type.
	sdkMemberTypes []android.SdkMemberType

	// decorator delegates, initialize before calling Init
	// these may contain module-specific implementations, and effectively allow for custom
	// type-specific logic. These members may reference different objects or the same object.
	// Functions of these decorators will be invoked to initialize and register type-specific
	// build statements.
	compiler     compiler
	linker       linker
	installer    installer
	bazelHandler android.BazelHandler

	features []feature
	stl      *stl
	sanitize *sanitize
	coverage *coverage
	sabi     *sabi
	vndkdep  *vndkdep
	lto      *lto
	afdo     *afdo
	pgo      *pgo

	library libraryInterface

	outputFile android.OptionalPath

	cachedToolchain config.Toolchain

	subAndroidMkOnce map[subAndroidMkProvider]bool

	// Flags used to compile this module
	flags Flags

	// Shared flags among build rules of this module
	sharedFlags SharedFlags

	// only non-nil when this is a shared library that reuses the objects of a static library
	staticAnalogue *StaticLibraryInfo

	makeLinkType string
	// Kythe (source file indexer) paths for this compilation module
	kytheFiles android.Paths
	// Object .o file output paths for this compilation module
	objFiles android.Paths
	// Tidy .tidy file output paths for this compilation module
	tidyFiles android.Paths

	// For apex variants, this is set as apex.min_sdk_version
	apexSdkVersion android.ApiLevel

	hideApexVariantFromMake bool
}

func (c *Module) AddJSONData(d *map[string]interface{}) {
	var hasAidl, hasLex, hasProto, hasRenderscript, hasSysprop, hasWinMsg, hasYacc bool
	if b, ok := c.compiler.(*baseCompiler); ok {
		hasAidl = b.hasSrcExt(".aidl")
		hasLex = b.hasSrcExt(".l") || b.hasSrcExt(".ll")
		hasProto = b.hasSrcExt(".proto")
		hasRenderscript = b.hasSrcExt(".rscript") || b.hasSrcExt(".fs")
		hasSysprop = b.hasSrcExt(".sysprop")
		hasWinMsg = b.hasSrcExt(".mc")
		hasYacc = b.hasSrcExt(".y") || b.hasSrcExt(".yy")
	}
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
		"IsVndk":                 c.IsVndk(),
		"IsVndkExt":              c.IsVndkExt(),
		"IsVndkPrivate":          c.IsVndkPrivate(),
		"IsVndkSp":               c.IsVndkSp(),
		"IsLlndk":                c.IsLlndk(),
		"IsLlndkPublic":          c.IsLlndkPublic(),
		"IsSnapshotLibrary":      c.IsSnapshotLibrary(),
		"IsSnapshotPrebuilt":     c.IsSnapshotPrebuilt(),
		"IsVendorPublicLibrary":  c.IsVendorPublicLibrary(),
		"ApexSdkVersion":         c.apexSdkVersion,
		"TestFor":                c.TestFor(),
		"AidlSrcs":               hasAidl,
		"LexSrcs":                hasLex,
		"ProtoSrcs":              hasProto,
		"RenderscriptSrcs":       hasRenderscript,
		"SyspropSrcs":            hasSysprop,
		"WinMsgSrcs":             hasWinMsg,
		"YaccSrsc":               hasYacc,
		"OnlyCSrcs":              !(hasAidl || hasLex || hasProto || hasRenderscript || hasSysprop || hasWinMsg || hasYacc),
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

func (c *Module) RequiredModuleNames() []string {
	required := android.CopyOf(c.ModuleBase.RequiredModuleNames())
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

func (c *Module) isCrt() bool {
	if linker, ok := c.linker.(*objectLinker); ok {
		return linker.isCrt()
	}
	return false
}

func (c *Module) SplitPerApiLevel() bool {
	return c.canUseSdk() && c.isCrt()
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
	if _, ok := c.linker.(libraryInterface); ok {
		return true
	}
	return false
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
	if c.afdo != nil {
		c.AddProperties(c.afdo.props()...)
	}
	if c.pgo != nil {
		c.AddProperties(c.pgo.props()...)
	}
	for _, feature := range c.features {
		c.AddProperties(feature.props()...)
	}

	android.InitAndroidArchModule(c, c.hod, c.multilib)
	if c.bazelable {
		android.InitBazelModule(c)
	}
	android.InitApexModule(c)
	android.InitSdkAwareModule(c)
	android.InitDefaultableModule(c)

	return c
}

func (c *Module) UseVndk() bool {
	return c.Properties.VndkVersion != ""
}

func (c *Module) canUseSdk() bool {
	return c.Os() == android.Android && c.Target().NativeBridge == android.NativeBridgeDisabled &&
		!c.UseVndk() && !c.InRamdisk() && !c.InRecovery() && !c.InVendorRamdisk()
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

func (c *Module) IsNdk(config android.Config) bool {
	return inList(c.BaseModuleName(), *getNDKKnownLibs(config))
}

func (c *Module) IsLlndk() bool {
	return c.VendorProperties.IsLLNDK
}

func (c *Module) IsLlndkPublic() bool {
	return c.VendorProperties.IsLLNDK && !c.VendorProperties.IsVNDKPrivate
}

func (m *Module) NeedsLlndkVariants() bool {
	lib := moduleLibraryInterface(m)
	return lib != nil && (lib.hasLLNDKStubs() || lib.hasLLNDKHeaders())
}

func (m *Module) NeedsVendorPublicLibraryVariants() bool {
	lib := moduleLibraryInterface(m)
	return lib != nil && (lib.hasVendorPublicLibrary())
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
	lib := moduleLibraryInterface(c)
	return lib != nil && lib.hasLLNDKStubs()
}

func (c *Module) StubsVersion() string {
	if lib, ok := c.linker.(versionedInterface); ok {
		return lib.stubsVersion()
	}
	panic(fmt.Errorf("StubsVersion called on non-versioned module: %q", c.BaseModuleName()))
}

// isImplementationForLLNDKPublic returns true for any variant of a cc_library that has LLNDK stubs
// and does not set llndk.vendor_available: false.
func (c *Module) isImplementationForLLNDKPublic() bool {
	library, _ := c.library.(*libraryDecorator)
	return library != nil && library.hasLLNDKStubs() &&
		!Bool(library.Properties.Llndk.Private)
}

// Returns true for LLNDK-private, VNDK-SP-private, and VNDK-core-private.
func (c *Module) IsVndkPrivate() bool {
	// Check if VNDK-core-private or VNDK-SP-private
	if c.IsVndk() {
		return Bool(c.vndkdep.Properties.Vndk.Private)
	}

	// Check if LLNDK-private
	if library, ok := c.library.(*libraryDecorator); ok && c.IsLlndk() {
		return Bool(library.Properties.Llndk.Private)
	}

	return false
}

func (c *Module) IsVndk() bool {
	if vndkdep := c.vndkdep; vndkdep != nil {
		return vndkdep.isVndk()
	}
	return false
}

func (c *Module) isAfdoCompile() bool {
	if afdo := c.afdo; afdo != nil {
		return afdo.Properties.AfdoTarget != nil
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

func (c *Module) IsVndkSp() bool {
	if vndkdep := c.vndkdep; vndkdep != nil {
		return vndkdep.isVndkSp()
	}
	return false
}

func (c *Module) IsVndkExt() bool {
	if vndkdep := c.vndkdep; vndkdep != nil {
		return vndkdep.isVndkExt()
	}
	return false
}

func (c *Module) SubName() string {
	return c.Properties.SubName
}

func (c *Module) MustUseVendorVariant() bool {
	return c.IsVndkSp() || c.Properties.MustUseVendorVariant
}

func (c *Module) getVndkExtendsModuleName() string {
	if vndkdep := c.vndkdep; vndkdep != nil {
		return vndkdep.getVndkExtendsModuleName()
	}
	return ""
}

func (c *Module) IsStubs() bool {
	if lib := c.library; lib != nil {
		return lib.buildStubs()
	}
	return false
}

func (c *Module) HasStubsVariants() bool {
	if lib := c.library; lib != nil {
		return lib.hasStubsVariants()
	}
	return false
}

// If this is a stubs library, ImplementationModuleName returns the name of the module that contains
// the implementation.  If it is an implementation library it returns its own name.
func (c *Module) ImplementationModuleName(ctx android.BaseModuleContext) string {
	name := ctx.OtherModuleName(c)
	if versioned, ok := c.linker.(versionedInterface); ok {
		name = versioned.implementationModuleName(name)
	}
	return name
}

// Similar to ImplementationModuleName, but uses the Make variant of the module
// name as base name, for use in AndroidMk output. E.g. for a prebuilt module
// where the Soong name is prebuilt_foo, this returns foo (which works in Make
// under the premise that the prebuilt module overrides its source counterpart
// if it is exposed to Make).
func (c *Module) ImplementationModuleNameForMake(ctx android.BaseModuleContext) string {
	name := c.BaseModuleName()
	if versioned, ok := c.linker.(versionedInterface); ok {
		name = versioned.implementationModuleName(name)
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

func (c *Module) ExcludeFromVendorSnapshot() bool {
	return Bool(c.Properties.Exclude_from_vendor_snapshot)
}

func (c *Module) ExcludeFromRecoverySnapshot() bool {
	return Bool(c.Properties.Exclude_from_recovery_snapshot)
}

func isBionic(name string) bool {
	switch name {
	case "libc", "libm", "libdl", "libdl_android", "linker", "linkerconfig":
		return true
	}
	return false
}

func InstallToBootstrap(name string, config android.Config) bool {
	if name == "libclang_rt.hwasan" {
		return true
	}
	return isBionic(name)
}

func (c *Module) XrefCcFiles() android.Paths {
	return c.kytheFiles
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

func (ctx *moduleContextImpl) testBinary() bool {
	return ctx.mod.testBinary()
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

func (ctx *moduleContextImpl) minSdkVersion() string {
	ver := ctx.mod.MinSdkVersion()
	if ver == "apex_inherit" && !ctx.isForPlatform() {
		ver = ctx.apexSdkVersion().String()
	}
	if ver == "apex_inherit" || ver == "" {
		ver = ctx.sdkVersion()
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
	if ctx.mod.isCrt() && !ctx.isSdkVariant() {
		if ctx.isForPlatform() {
			ver = strconv.Itoa(android.FutureApiLevelInt)
		} else { // for apex
			ver = ctx.apexSdkVersion().String()
			if ver == "" { // in case when min_sdk_version was not set by the APEX
				ver = ctx.sdkVersion()
			}
		}
	}

	// Also make sure that minSdkVersion is not greater than sdkVersion, if they are both numbers
	sdkVersionInt, err := strconv.Atoi(ctx.sdkVersion())
	minSdkVersionInt, err2 := strconv.Atoi(ver)
	if err == nil && err2 == nil {
		if sdkVersionInt < minSdkVersionInt {
			return strconv.Itoa(sdkVersionInt)
		}
	}
	return ver
}

func (ctx *moduleContextImpl) isSdkVariant() bool {
	return ctx.mod.IsSdkVariant()
}

func (ctx *moduleContextImpl) useVndk() bool {
	return ctx.mod.UseVndk()
}

func (ctx *moduleContextImpl) isNdk(config android.Config) bool {
	return ctx.mod.IsNdk(config)
}

func (ctx *moduleContextImpl) IsLlndk() bool {
	return ctx.mod.IsLlndk()
}

func (ctx *moduleContextImpl) IsLlndkPublic() bool {
	return ctx.mod.IsLlndkPublic()
}

func (ctx *moduleContextImpl) isImplementationForLLNDKPublic() bool {
	return ctx.mod.isImplementationForLLNDKPublic()
}

func (ctx *moduleContextImpl) IsVndkPrivate() bool {
	return ctx.mod.IsVndkPrivate()
}

func (ctx *moduleContextImpl) isVndk() bool {
	return ctx.mod.IsVndk()
}

func (ctx *moduleContextImpl) isAfdoCompile() bool {
	return ctx.mod.isAfdoCompile()
}

func (ctx *moduleContextImpl) isPgoCompile() bool {
	return ctx.mod.isPgoCompile()
}

func (ctx *moduleContextImpl) isNDKStubLibrary() bool {
	return ctx.mod.isNDKStubLibrary()
}

func (ctx *moduleContextImpl) isVndkSp() bool {
	return ctx.mod.IsVndkSp()
}

func (ctx *moduleContextImpl) IsVndkExt() bool {
	return ctx.mod.IsVndkExt()
}

func (ctx *moduleContextImpl) IsVendorPublicLibrary() bool {
	return ctx.mod.IsVendorPublicLibrary()
}

func (ctx *moduleContextImpl) mustUseVendorVariant() bool {
	return ctx.mod.MustUseVendorVariant()
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
	return ctx.ctx.Provider(android.ApexInfoProvider).(android.ApexInfo).IsForPlatform()
}

func (ctx *moduleContextImpl) apexVariationName() string {
	return ctx.ctx.Provider(android.ApexInfoProvider).(android.ApexInfo).ApexVariationName
}

func (ctx *moduleContextImpl) apexSdkVersion() android.ApiLevel {
	return ctx.mod.apexSdkVersion
}

func (ctx *moduleContextImpl) bootstrap() bool {
	return ctx.mod.Bootstrap()
}

func (ctx *moduleContextImpl) nativeCoverage() bool {
	return ctx.mod.nativeCoverage()
}

func (ctx *moduleContextImpl) directlyInAnyApex() bool {
	return ctx.mod.DirectlyInAnyApex()
}

func (ctx *moduleContextImpl) isPreventInstall() bool {
	return ctx.mod.Properties.PreventInstall
}

func (ctx *moduleContextImpl) getSharedFlags() *SharedFlags {
	shared := &ctx.mod.sharedFlags
	if shared.flagsMap == nil {
		shared.numSharedFlags = 0
		shared.flagsMap = make(map[string]string)
	}
	return shared
}

func (ctx *moduleContextImpl) isCfiAssemblySupportEnabled() bool {
	return ctx.mod.isCfiAssemblySupportEnabled()
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
	module.afdo = &afdo{}
	module.pgo = &pgo{}
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

func (c *Module) Symlinks() []string {
	if p, ok := c.installer.(interface {
		symlinkList() []string
	}); ok {
		return p.symlinkList()
	}
	return nil
}

func (c *Module) IsTestPerSrcAllTestsVariation() bool {
	test, ok := c.linker.(testPerSrc)
	return ok && test.isAllTestsVariation()
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
	var vndkVersion string
	var nameSuffix string
	if c.InProduct() {
		if c.ProductSpecific() {
			// If the module is product specific with 'product_specific: true',
			// do not add a name suffix because it is a base module.
			return ""
		}
		vndkVersion = ctx.DeviceConfig().ProductVndkVersion()
		nameSuffix = ProductSuffix
	} else {
		vndkVersion = ctx.DeviceConfig().VndkVersion()
		nameSuffix = VendorSuffix
	}
	if vndkVersion == "current" {
		vndkVersion = ctx.DeviceConfig().PlatformVndkVersion()
	}
	if c.VndkVersion() != vndkVersion && c.VndkVersion() != "" {
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
	if llndk || (c.UseVndk() && c.HasNonSystemVariants()) {
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
	}

	return subName
}

// Returns true if Bazel was successfully used for the analysis of this module.
func (c *Module) maybeGenerateBazelActions(actx android.ModuleContext) bool {
	var bazelModuleLabel string
	if c.typ() == fullLibrary && c.static() {
		// cc_library is a special case in bp2build; two targets are generated -- one for each
		// of the shared and static variants. The shared variant keeps the module name, but the
		// static variant uses a different suffixed name.
		bazelModuleLabel = bazelLabelForStaticModule(actx, c)
	} else {
		bazelModuleLabel = c.GetBazelLabel(actx, c)
	}

	bazelActionsUsed := false
	// Mixed builds mode is disabled for modules outside of device OS.
	// TODO(b/200841190): Support non-device OS in mixed builds.
	if c.MixedBuildsEnabled(actx) && c.bazelHandler != nil {
		bazelActionsUsed = c.bazelHandler.GenerateBazelBuildActions(actx, bazelModuleLabel)
	}
	return bazelActionsUsed
}

func (c *Module) GenerateAndroidBuildActions(actx android.ModuleContext) {
	// TODO(cparsons): Any logic in this method occurring prior to querying Bazel should be
	// requested from Bazel instead.

	// Handle the case of a test module split by `test_per_src` mutator.
	//
	// The `test_per_src` mutator adds an extra variation named "", depending on all the other
	// `test_per_src` variations of the test module. Set `outputFile` to an empty path for this
	// module and return early, as this module does not produce an output file per se.
	if c.IsTestPerSrcAllTestsVariation() {
		c.outputFile = android.OptionalPath{}
		return
	}

	c.Properties.SubName = GetSubnameProperty(actx, c)
	apexInfo := actx.Provider(android.ApexInfoProvider).(android.ApexInfo)
	if !apexInfo.IsForPlatform() {
		c.hideApexVariantFromMake = true
	}

	c.makeLinkType = GetMakeLinkType(actx, c)

	ctx := &moduleContext{
		ModuleContext: actx,
		moduleContextImpl: moduleContextImpl{
			mod: c,
		},
	}
	ctx.ctx = ctx

	if c.maybeGenerateBazelActions(actx) {
		c.maybeInstall(ctx, apexInfo)
		return
	}

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
	if c.afdo != nil {
		flags = c.afdo.flags(ctx, flags)
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

	var objs Objects
	if c.compiler != nil {
		objs = c.compiler.compile(ctx, flags, deps)
		if ctx.Failed() {
			return
		}
		c.kytheFiles = objs.kytheFiles
		c.objFiles = objs.objFiles
		c.tidyFiles = objs.tidyFiles
	}

	if c.linker != nil {
		outputFile := c.linker.link(ctx, flags, deps, objs)
		if ctx.Failed() {
			return
		}
		c.outputFile = android.OptionalPathForPath(outputFile)

		c.maybeUnhideFromMake()

		// glob exported headers for snapshot, if BOARD_VNDK_VERSION is current or
		// RECOVERY_SNAPSHOT_VERSION is current.
		if i, ok := c.linker.(snapshotLibraryInterface); ok {
			if ShouldCollectHeadersForSnapshot(ctx, c, apexInfo) {
				i.collectHeadersForSnapshot(ctx)
			}
		}
	}

	c.maybeInstall(ctx, apexInfo)
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
		!c.InRecovery() && !c.UseVndk() && !c.static() && !c.isCoverageVariant() &&
		c.IsStubs() && !c.InVendorRamdisk() {
		c.Properties.HideFromMake = false // unhide
		// Note: this is still non-installable
	}
}

func (c *Module) maybeInstall(ctx ModuleContext, apexInfo android.ApexInfo) {
	if !proptools.BoolDefault(c.Installable(), true) {
		// If the module has been specifically configure to not be installed then
		// hide from make as otherwise it will break when running inside make
		// as the output path to install will not be specified. Not all uninstallable
		// modules can be hidden from make as some are needed for resolving make side
		// dependencies.
		c.HideFromMake()
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
	if c.lto != nil {
		c.lto.begin(ctx)
	}
	if c.afdo != nil {
		c.afdo.begin(ctx)
	}
	if c.pgo != nil {
		c.pgo.begin(ctx)
	}
	if ctx.useSdk() && c.IsSdkVariant() {
		version, err := nativeApiLevelFromUser(ctx, ctx.sdkVersion())
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

	// In Bazel conversion mode, we dependency and build validations will occur in Bazel, so there is
	// no need to do so in Soong.
	if ctx.BazelConversionMode() {
		return deps
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

	if version != "" && CanBeOrLinkAgainstVersionVariants(mod) {
		// Version is explicitly specified. i.e. libFoo#30
		variations = append(variations, blueprint.Variation{Mutator: "version", Variation: version})
		if tag, ok := depTag.(libraryDependencyTag); ok {
			tag.explicitlyVersioned = true
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

func GetSnapshot(c LinkableInterface, snapshotInfo **SnapshotInfo, actx android.BottomUpMutatorContext) SnapshotInfo {
	// Only device modules with BOARD_VNDK_VERSION uses snapshot.  Others use the zero value of
	// SnapshotInfo, which provides no mappings.
	if *snapshotInfo == nil && c.Device() {
		// Only retrieve the snapshot on demand in order to avoid circular dependencies
		// between the modules in the snapshot and the snapshot itself.
		var snapshotModule []blueprint.Module
		if c.InVendor() && c.VndkVersion() == actx.DeviceConfig().VndkVersion() {
			snapshotModule = actx.AddVariationDependencies(nil, nil, "vendor_snapshot")
		} else if recoverySnapshotVersion := actx.DeviceConfig().RecoverySnapshotVersion(); recoverySnapshotVersion != "current" && recoverySnapshotVersion != "" && c.InRecovery() {
			snapshotModule = actx.AddVariationDependencies(nil, nil, "recovery_snapshot")
		}
		if len(snapshotModule) > 0 && snapshotModule[0] != nil {
			snapshot := actx.OtherModuleProvider(snapshotModule[0], SnapshotInfoProvider).(SnapshotInfo)
			*snapshotInfo = &snapshot
			// republish the snapshot for use in later mutators on this module
			actx.SetProvider(SnapshotInfoProvider, snapshot)
		}
	}
	if *snapshotInfo == nil {
		*snapshotInfo = &SnapshotInfo{}
	}
	return **snapshotInfo
}

func RewriteSnapshotLib(lib string, snapshotMap map[string]string) string {
	if snapshot, ok := snapshotMap[lib]; ok {
		return snapshot
	}

	return lib
}

// RewriteLibs takes a list of names of shared libraries and scans it for three types
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
func RewriteLibs(c LinkableInterface, snapshotInfo **SnapshotInfo, actx android.BottomUpMutatorContext, config android.Config, list []string) (nonvariantLibs []string, variantLibs []string) {
	variantLibs = []string{}

	nonvariantLibs = []string{}
	for _, entry := range list {
		// strip #version suffix out
		name, _ := StubsLibNameAndVersion(entry)
		if c.InRecovery() {
			nonvariantLibs = append(nonvariantLibs, RewriteSnapshotLib(entry, GetSnapshot(c, snapshotInfo, actx).SharedLibs))
		} else if c.UseSdk() && inList(name, *getNDKKnownLibs(config)) {
			variantLibs = append(variantLibs, name+ndkLibrarySuffix)
		} else if c.UseVndk() {
			nonvariantLibs = append(nonvariantLibs, RewriteSnapshotLib(entry, GetSnapshot(c, snapshotInfo, actx).SharedLibs))
		} else {
			// put name#version back
			nonvariantLibs = append(nonvariantLibs, entry)
		}
	}
	return nonvariantLibs, variantLibs
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

	c.Properties.AndroidMkSystemSharedLibs = deps.SystemSharedLibs

	var snapshotInfo *SnapshotInfo

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

		deps.SharedLibs, variantNdkLibs = RewriteLibs(c, &snapshotInfo, actx, ctx.Config(), deps.SharedLibs)
		deps.LateSharedLibs, variantLateNdkLibs = RewriteLibs(c, &snapshotInfo, actx, ctx.Config(), deps.LateSharedLibs)
		deps.ReexportSharedLibHeaders, _ = RewriteLibs(c, &snapshotInfo, actx, ctx.Config(), deps.ReexportSharedLibHeaders)

		for idx, lib := range deps.RuntimeLibs {
			deps.RuntimeLibs[idx] = RewriteSnapshotLib(lib, GetSnapshot(c, &snapshotInfo, actx).SharedLibs)
		}
	}

	for _, lib := range deps.HeaderLibs {
		depTag := libraryDependencyTag{Kind: headerLibraryDependency}
		if inList(lib, deps.ReexportHeaderLibHeaders) {
			depTag.reexportFlags = true
		}

		lib = RewriteSnapshotLib(lib, GetSnapshot(c, &snapshotInfo, actx).HeaderLibs)

		if c.IsStubs() {
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

	// sysprop_library has to support both C++ and Java. So sysprop_library internally creates one
	// C++ implementation library and one Java implementation library. When a module links against
	// sysprop_library, the C++ implementation library has to be linked. syspropImplLibraries is a
	// map from sysprop_library to implementation library; it will be used in whole_static_libs,
	// static_libs, and shared_libs.
	syspropImplLibraries := syspropImplLibraries(actx.Config())

	for _, lib := range deps.WholeStaticLibs {
		depTag := libraryDependencyTag{Kind: staticLibraryDependency, wholeStatic: true, reexportFlags: true}
		if impl, ok := syspropImplLibraries[lib]; ok {
			lib = impl
		}

		lib = RewriteSnapshotLib(lib, GetSnapshot(c, &snapshotInfo, actx).StaticLibs)

		actx.AddVariationDependencies([]blueprint.Variation{
			{Mutator: "link", Variation: "static"},
		}, depTag, lib)
	}

	for _, lib := range deps.StaticLibs {
		depTag := libraryDependencyTag{Kind: staticLibraryDependency}
		if inList(lib, deps.ReexportStaticLibHeaders) {
			depTag.reexportFlags = true
		}
		if inList(lib, deps.ExcludeLibsForApex) {
			depTag.excludeInApex = true
		}

		if impl, ok := syspropImplLibraries[lib]; ok {
			lib = impl
		}

		lib = RewriteSnapshotLib(lib, GetSnapshot(c, &snapshotInfo, actx).StaticLibs)

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
		}, depTag, RewriteSnapshotLib(staticUnwinder(actx), GetSnapshot(c, &snapshotInfo, actx).StaticLibs))
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

		if impl, ok := syspropImplLibraries[lib]; ok {
			lib = impl
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
		}, depTag, RewriteSnapshotLib(lib, GetSnapshot(c, &snapshotInfo, actx).StaticLibs))
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

	crtVariations := GetCrtVariations(ctx, c)
	actx.AddVariationDependencies(crtVariations, objDepTag, deps.ObjFiles...)
	for _, crt := range deps.CrtBegin {
		actx.AddVariationDependencies(crtVariations, CrtBeginDepTag,
			RewriteSnapshotLib(crt, GetSnapshot(c, &snapshotInfo, actx).Objects))
	}
	for _, crt := range deps.CrtEnd {
		actx.AddVariationDependencies(crtVariations, CrtEndDepTag,
			RewriteSnapshotLib(crt, GetSnapshot(c, &snapshotInfo, actx).Objects))
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

	ndkLateStubDepTag := libraryDependencyTag{Kind: sharedLibraryDependency, Order: lateLibraryDependency, ndk: true, makeSuffix: "." + version}
	actx.AddVariationDependencies([]blueprint.Variation{
		{Mutator: "version", Variation: version},
		{Mutator: "link", Variation: "shared"},
	}, ndkLateStubDepTag, variantLateNdkLibs...)

	if vndkdep := c.vndkdep; vndkdep != nil {
		if vndkdep.isVndkExt() {
			actx.AddVariationDependencies([]blueprint.Variation{
				c.ImageVariation(),
				{Mutator: "link", Variation: "shared"},
			}, vndkExtDepTag, RewriteSnapshotLib(vndkdep.getVndkExtendsModuleName(), GetSnapshot(c, &snapshotInfo, actx).SharedLibs))
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

	// VNDK is cc.Module supported only for now.
	if ccFrom, ok := from.(*Module); ok && from.UseVndk() {
		// Though allowed dependency is limited by the image mutator,
		// each vendor and product module needs to check link-type
		// for VNDK.
		if ccTo, ok := to.(*Module); ok {
			if ccFrom.vndkdep != nil {
				ccFrom.vndkdep.vndkCheckLinkType(ctx, ccTo, tag)
			}
		} else if _, ok := to.(LinkableInterface); !ok {
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
	if from.InVendorRamdisk() {
		// Vendor ramdisk code is not NDK
		return
	}
	if from.InRecovery() {
		// Recovery code is not NDK
		return
	}
	if c, ok := to.(*Module); ok {
		if c.NdkPrebuiltStl() {
			// These are allowed, but they don't set sdk_version
			return
		}
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
func checkDoubleLoadableLibraries(ctx android.TopDownMutatorContext) {
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
		if depTag == stubImplDepTag {
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

		if to.IsVndkSp() || to.IsLlndk() {
			return false
		}

		ctx.ModuleErrorf("links a library %q which is not LL-NDK, "+
			"VNDK-SP, or explicitly marked as 'double_loadable:true'. "+
			"Dependency list: %s", ctx.OtherModuleName(to), ctx.GetPathString(false))
		return false
	}
	if module, ok := ctx.Module().(*Module); ok {
		if lib, ok := module.linker.(*libraryDecorator); ok && lib.shared() {
			if lib.hasLLNDKStubs() {
				ctx.WalkDeps(check)
			}
		}
	}
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

	// For the dependency from platform to apex, use the latest stubs
	c.apexSdkVersion = android.FutureApiLevel
	apexInfo := ctx.Provider(android.ApexInfoProvider).(android.ApexInfo)
	if !apexInfo.IsForPlatform() {
		c.apexSdkVersion = apexInfo.MinSdkVersion
	}

	if android.InList("hwaddress", ctx.Config().SanitizeDevice()) {
		// In hwasan build, we override apexSdkVersion to the FutureApiLevel(10000)
		// so that even Q(29/Android10) apexes could use the dynamic unwinder by linking the newer stubs(e.g libc(R+)).
		// (b/144430859)
		c.apexSdkVersion = android.FutureApiLevel
	}

	ctx.VisitDirectDeps(func(dep android.Module) {
		depName := ctx.OtherModuleName(dep)
		depTag := ctx.OtherModuleDependencyTag(dep)

		if depTag == android.DarwinUniversalVariantTag {
			depPaths.DarwinSecondArchOutput = dep.(*Module).OutputFile()
			return
		}

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

		if dep.Target().Os != ctx.Os() {
			ctx.ModuleErrorf("OS mismatch between %q and %q", ctx.ModuleName(), depName)
			return
		}
		if dep.Target().Arch.ArchType != ctx.Arch().ArchType {
			ctx.ModuleErrorf("Arch mismatch between %q(%v) and %q(%v)",
				ctx.ModuleName(), ctx.Arch().ArchType, depName, dep.Target().Arch.ArchType)
			return
		}

		if depTag == reuseObjTag {
			// Skip reused objects for stub libraries, they use their own stub object file instead.
			// The reuseObjTag dependency still exists because the LinkageMutator runs before the
			// version mutator, so the stubs variant is created from the shared variant that
			// already has the reuseObjTag dependency on the static variant.
			if !c.library.buildStubs() {
				staticAnalogue := ctx.OtherModuleProvider(dep, StaticLibraryInfoProvider).(StaticLibraryInfo)
				objs := staticAnalogue.ReuseObjects
				depPaths.Objs = depPaths.Objs.Append(objs)
				depExporterInfo := ctx.OtherModuleProvider(dep, FlagExporterInfoProvider).(FlagExporterInfo)
				reexportExporter(depExporterInfo)
			}
			return
		}

		linkFile := ccDep.OutputFile()

		if libDepTag, ok := depTag.(libraryDependencyTag); ok {
			// Only use static unwinder for legacy (min_sdk_version = 29) apexes (b/144430859)
			if libDepTag.staticUnwinder && c.apexSdkVersion.GreaterThan(android.SdkVersion_Android10) {
				return
			}

			if !apexInfo.IsForPlatform() && libDepTag.excludeInApex {
				return
			}

			depExporterInfo := ctx.OtherModuleProvider(dep, FlagExporterInfoProvider).(FlagExporterInfo)

			var ptr *android.Paths
			var depPtr *android.Paths

			depFile := android.OptionalPath{}

			switch {
			case libDepTag.header():
				if !ctx.OtherModuleHasProvider(dep, HeaderLibraryInfoProvider) {
					if !ctx.Config().AllowMissingDependencies() {
						ctx.ModuleErrorf("module %q is not a header library", depName)
					} else {
						ctx.AddMissingDependencies([]string{depName})
					}
					return
				}
			case libDepTag.shared():
				if !ctx.OtherModuleHasProvider(dep, SharedLibraryInfoProvider) {
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
				if !ctx.OtherModuleHasProvider(dep, StaticLibraryInfoProvider) {
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

				staticLibraryInfo := ctx.OtherModuleProvider(dep, StaticLibraryInfoProvider).(StaticLibraryInfo)
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
						panic(fmt.Errorf("early static libs not suppported"))
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
			}

			if libDepTag.static() && !libDepTag.wholeStatic {
				if !ccDep.CcLibraryInterface() || !ccDep.Static() {
					ctx.ModuleErrorf("module %q not a static library", depName)
					return
				}

				// When combining coverage files for shared libraries and executables, coverage files
				// in static libraries act as if they were whole static libraries. The same goes for
				// source based Abi dump files.
				if c, ok := ccDep.(*Module); ok {
					staticLib := c.linker.(libraryInterface)
					depPaths.StaticLibObjs.coverageFiles = append(depPaths.StaticLibObjs.coverageFiles,
						staticLib.objs().coverageFiles...)
					depPaths.StaticLibObjs.sAbiDumpFiles = append(depPaths.StaticLibObjs.sAbiDumpFiles,
						staticLib.objs().sAbiDumpFiles...)
				} else {
					// Handle non-CC modules here
					depPaths.StaticLibObjs.coverageFiles = append(depPaths.StaticLibObjs.coverageFiles,
						ccDep.CoverageFiles()...)
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

			if libDepTag.reexportFlags {
				reexportExporter(depExporterInfo)
				// Add these re-exported flags to help header-abi-dumper to infer the abi exported by a library.
				// Re-exported shared library headers must be included as well since they can help us with type information
				// about template instantiations (instantiated from their headers).
				// -isystem headers are not included since for bionic libraries, abi-filtering is taken care of by version
				// scripts.
				c.sabi.Properties.ReexportedIncludes = append(
					c.sabi.Properties.ReexportedIncludes, depExporterInfo.IncludeDirs.Strings()...)
			}

			makeLibName := MakeLibName(ctx, c, ccDep, depName) + libDepTag.makeSuffix
			switch {
			case libDepTag.header():
				c.Properties.AndroidMkHeaderLibs = append(
					c.Properties.AndroidMkHeaderLibs, makeLibName)
			case libDepTag.shared():
				if lib := moduleLibraryInterface(dep); lib != nil {
					if lib.buildStubs() && dep.(android.ApexModule).InAnyApex() {
						// Add the dependency to the APEX(es) providing the library so that
						// m <module> can trigger building the APEXes as well.
						depApexInfo := ctx.OtherModuleProvider(dep, android.ApexInfoProvider).(android.ApexInfo)
						for _, an := range depApexInfo.InApexVariants {
							c.Properties.ApexesProvidingSharedLibs = append(
								c.Properties.ApexesProvidingSharedLibs, an)
						}
					}
				}

				// Note: the order of libs in this list is not important because
				// they merely serve as Make dependencies and do not affect this lib itself.
				c.Properties.AndroidMkSharedLibs = append(
					c.Properties.AndroidMkSharedLibs, makeLibName)
				// Record BaseLibName for snapshots.
				c.Properties.SnapshotSharedLibs = append(c.Properties.SnapshotSharedLibs, BaseLibName(depName))
			case libDepTag.static():
				if libDepTag.wholeStatic {
					c.Properties.AndroidMkWholeStaticLibs = append(
						c.Properties.AndroidMkWholeStaticLibs, makeLibName)
				} else {
					c.Properties.AndroidMkStaticLibs = append(
						c.Properties.AndroidMkStaticLibs, makeLibName)
				}
				// Record BaseLibName for snapshots.
				c.Properties.SnapshotStaticLibs = append(c.Properties.SnapshotStaticLibs, BaseLibName(depName))
			}
		} else if !c.IsStubs() {
			// Stubs lib doesn't link to the runtime lib, object, crt, etc. dependencies.

			switch depTag {
			case runtimeDepTag:
				c.Properties.AndroidMkRuntimeLibs = append(
					c.Properties.AndroidMkRuntimeLibs, MakeLibName(ctx, c, ccDep, depName)+libDepTag.makeSuffix)
				// Record BaseLibName for snapshots.
				c.Properties.SnapshotRuntimeLibs = append(c.Properties.SnapshotRuntimeLibs, BaseLibName(depName))
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

// ChooseStubOrImpl determines whether a given dependency should be redirected to the stub variant
// of the dependency or not, and returns the SharedLibraryInfo and FlagExporterInfo for the right
// dependency. The stub variant is selected when the dependency crosses a boundary where each side
// has different level of updatability. For example, if a library foo in an APEX depends on a
// library bar which provides stable interface and exists in the platform, foo uses the stub variant
// of bar. If bar doesn't provide a stable interface (i.e. buildStubs() == false) or is in the
// same APEX as foo, the non-stub variant of bar is used.
func ChooseStubOrImpl(ctx android.ModuleContext, dep android.Module) (SharedLibraryInfo, FlagExporterInfo) {
	depName := ctx.OtherModuleName(dep)
	depTag := ctx.OtherModuleDependencyTag(dep)
	libDepTag, ok := depTag.(libraryDependencyTag)
	if !ok || !libDepTag.shared() {
		panic(fmt.Errorf("Unexpected dependency tag: %T", depTag))
	}

	thisModule, ok := ctx.Module().(android.ApexModule)
	if !ok {
		panic(fmt.Errorf("Not an APEX module: %q", ctx.ModuleName()))
	}

	useVndk := false
	bootstrap := false
	if linkable, ok := ctx.Module().(LinkableInterface); !ok {
		panic(fmt.Errorf("Not a Linkable module: %q", ctx.ModuleName()))
	} else {
		useVndk = linkable.UseVndk()
		bootstrap = linkable.Bootstrap()
	}

	sharedLibraryInfo := ctx.OtherModuleProvider(dep, SharedLibraryInfoProvider).(SharedLibraryInfo)
	depExporterInfo := ctx.OtherModuleProvider(dep, FlagExporterInfoProvider).(FlagExporterInfo)
	sharedLibraryStubsInfo := ctx.OtherModuleProvider(dep, SharedLibraryStubsProvider).(SharedLibraryStubsInfo)
	apexInfo := ctx.Provider(android.ApexInfoProvider).(android.ApexInfo)

	if !libDepTag.explicitlyVersioned && len(sharedLibraryStubsInfo.SharedStubLibraries) > 0 {
		useStubs := false

		if lib := moduleLibraryInterface(dep); lib.buildStubs() && useVndk { // LLNDK
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
			useStubs = dep.(android.ApexModule).NotInPlatform() && !bootstrap
			if useStubs {
				// Another exception: if this module is a test for an APEX, then
				// it is linked with the non-stub variant of a module in the APEX
				// as if this is part of the APEX.
				testFor := ctx.Provider(android.ApexTestForInfoProvider).(android.ApexTestForInfo)
				for _, apexContents := range testFor.ApexContents {
					if apexContents.DirectlyInApex(depName) {
						useStubs = false
						break
					}
				}
			}
			if useStubs {
				// Yet another exception: If this module and the dependency are
				// available to the same APEXes then skip stubs between their
				// platform variants. This complements the test_for case above,
				// which avoids the stubs on a direct APEX library dependency, by
				// avoiding stubs for indirect test dependencies as well.
				//
				// TODO(b/183882457): This doesn't work if the two libraries have
				// only partially overlapping apex_available. For that test_for
				// modules would need to be split into APEX variants and resolved
				// separately for each APEX they have access to.
				if android.AvailableToSameApexes(thisModule, dep.(android.ApexModule)) {
					useStubs = false
				}
			}
		} else {
			// If building for APEX, use stubs when the parent is in any APEX that
			// the child is not in.
			useStubs = !android.DirectlyInAllApexes(apexInfo, depName)
		}

		// when to use (unspecified) stubs, use the latest one.
		if useStubs {
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
// direct shared libraries.  It returns the ordered static dependencies, and an android.DepSet
// of the transitive dependencies.
func orderStaticModuleDeps(staticDeps []StaticLibraryInfo, sharedDeps []SharedLibraryInfo) (ordered android.Paths, transitive *android.DepSet) {
	transitiveStaticLibsBuilder := android.NewDepSetBuilder(android.TOPOLOGICAL)
	var staticPaths android.Paths
	for _, staticDep := range staticDeps {
		staticPaths = append(staticPaths, staticDep.StaticLibrary)
		transitiveStaticLibsBuilder.Transitive(staticDep.TransitiveStaticLibrariesForOrdering)
	}
	for _, sharedDep := range sharedDeps {
		if sharedDep.TransitiveStaticLibrariesForOrdering != nil {
			transitiveStaticLibsBuilder.Transitive(sharedDep.TransitiveStaticLibrariesForOrdering)
		}
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

func MakeLibName(ctx android.ModuleContext, c LinkableInterface, ccDep LinkableInterface, depName string) string {
	libName := BaseLibName(depName)
	ccDepModule, _ := ccDep.(*Module)
	isLLndk := ccDepModule != nil && ccDepModule.IsLlndk()
	nonSystemVariantsExist := ccDep.HasNonSystemVariants() || isLLndk

	if ccDepModule != nil {
		// TODO(ivanlozano) Support snapshots for Rust-produced C library variants.
		// Use base module name for snapshots when exporting to Makefile.
		if snapshotPrebuilt, ok := ccDepModule.linker.(SnapshotInterface); ok {
			baseName := ccDepModule.BaseModuleName()

			return baseName + snapshotPrebuilt.SnapshotAndroidMkSuffix()
		}
	}

	if ctx.DeviceConfig().VndkUseCoreVariant() && ccDep.IsVndk() && !ccDep.MustUseVendorVariant() &&
		!c.InRamdisk() && !c.InVendorRamdisk() && !c.InRecovery() {
		// The vendor module is a no-vendor-variant VNDK library.  Depend on the
		// core module instead.
		return libName
	} else if ccDep.UseVndk() && nonSystemVariantsExist {
		// The vendor and product modules in Make will have been renamed to not conflict with the
		// core module, so update the dependency name here accordingly.
		return libName + ccDep.SubName()
	} else if ccDep.InRamdisk() && !ccDep.OnlyInRamdisk() {
		return libName + RamdiskSuffix
	} else if ccDep.InVendorRamdisk() && !ccDep.OnlyInVendorRamdisk() {
		return libName + VendorRamdiskSuffix
	} else if ccDep.InRecovery() && !ccDep.OnlyInRecovery() {
		return libName + RecoverySuffix
	} else if ccDep.Target().NativeBridge == android.NativeBridgeEnabled {
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

func (c *Module) testBinary() bool {
	if test, ok := c.linker.(interface {
		testBinary() bool
	}); ok {
		return test.testBinary()
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

func GetMakeLinkType(actx android.ModuleContext, c LinkableInterface) string {
	if c.UseVndk() {
		if c.IsLlndk() {
			if !c.IsLlndkPublic() {
				return "native:vndk_private"
			}
			return "native:vndk"
		}
		if c.IsVndk() && !c.IsVndkExt() {
			if c.IsVndkPrivate() {
				return "native:vndk_private"
			}
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
	} else if actx.DeviceConfig().VndkUseCoreVariant() && !c.MustUseVendorVariant() {
		return "native:platform_vndk"
	} else {
		return "native:platform"
	}
}

// Overrides ApexModule.IsInstallabeToApex()
// Only shared/runtime libraries and "test_per_src" tests are installable to APEX.
func (c *Module) IsInstallableToApex() bool {
	if lib := c.library; lib != nil {
		// Stub libs and prebuilt libs in a versioned SDK are not
		// installable to APEX even though they are shared libs.
		return lib.shared() && !lib.buildStubs() && c.ContainingSdk().Unversioned()
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
	return c.Properties.Test_for
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

var _ android.ApexModule = (*Module)(nil)

// Implements android.ApexModule
func (c *Module) DepIsInSameApex(ctx android.BaseModuleContext, dep android.Module) bool {
	depTag := ctx.OtherModuleDependencyTag(dep)
	libDepTag, isLibDepTag := depTag.(libraryDependencyTag)

	if cc, ok := dep.(*Module); ok {
		if cc.HasStubsVariants() {
			if isLibDepTag && libDepTag.shared() {
				// dynamic dep to a stubs lib crosses APEX boundary
				return false
			}
			if IsRuntimeDepTag(depTag) {
				// runtime dep to a stubs lib also crosses APEX boundary
				return false
			}
		}
		if cc.IsLlndk() {
			return false
		}
		if isLibDepTag && c.static() && libDepTag.shared() {
			// shared_lib dependency from a static lib is considered as crossing
			// the APEX boundary because the dependency doesn't actually is
			// linked; the dependency is used only during the compilation phase.
			return false
		}

		if isLibDepTag && libDepTag.excludeInApex {
			return false
		}
	}
	if depTag == stubImplDepTag {
		// We don't track from an implementation library to its stubs.
		return false
	}
	if depTag == staticVariantTag {
		// This dependency is for optimization (reuse *.o from the static lib). It doesn't
		// actually mean that the static lib (and its dependencies) are copied into the
		// APEX.
		return false
	}
	return true
}

// Implements android.ApexModule
func (c *Module) ShouldSupportSdkVersion(ctx android.BaseModuleContext,
	sdkVersion android.ApiLevel) error {
	// We ignore libclang_rt.* prebuilt libs since they declare sdk_version: 14(b/121358700)
	if strings.HasPrefix(ctx.OtherModuleName(c), "libclang_rt") {
		return nil
	}
	// We don't check for prebuilt modules
	if _, ok := c.linker.(prebuiltLinkerInterface); ok {
		return nil
	}
	minSdkVersion := c.MinSdkVersion()
	if minSdkVersion == "apex_inherit" {
		return nil
	}
	if minSdkVersion == "" {
		// JNI libs within APK-in-APEX fall into here
		// Those are okay to set sdk_version instead
		// We don't have to check if this is a SDK variant because
		// non-SDK variant resets sdk_version, which works too.
		minSdkVersion = c.SdkVersion()
	}
	if minSdkVersion == "" {
		return fmt.Errorf("neither min_sdk_version nor sdk_version specificed")
	}
	// Not using nativeApiLevelFromUser because the context here is not
	// necessarily a native context.
	ver, err := android.ApiLevelFromUser(ctx, minSdkVersion)
	if err != nil {
		return err
	}

	if ver.GreaterThan(sdkVersion) {
		return fmt.Errorf("newer SDK(%v)", ver)
	}
	return nil
}

// Implements android.ApexModule
func (c *Module) AlwaysRequiresPlatformApexVariant() bool {
	// stub libraries and native bridge libraries are always available to platform
	return c.IsStubs() || c.Target().NativeBridge == android.NativeBridgeEnabled
}

// Overrides android.ApexModuleBase.UniqueApexVariations
func (c *Module) UniqueApexVariations() bool {
	// When a vendor APEX needs a VNDK lib in it (use_vndk_as_stable: false), it should be a unique
	// APEX variation. Otherwise, another vendor APEX with use_vndk_as_stable:true may use a wrong
	// variation of the VNDK lib because APEX variations are merged/grouped.
	return c.UseVndk() && c.IsVndk()
}

var _ snapshot.RelativeInstallPath = (*Module)(nil)

type moduleType int

const (
	unknownType moduleType = iota
	binary
	object
	fullLibrary
	staticLibrary
	sharedLibrary
	headerLibrary
)

func (c *Module) typ() moduleType {
	if c.Binary() {
		return binary
	} else if c.Object() {
		return object
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
	}
	return unknownType
}

// ConvertWithBp2build converts Module to Bazel for bp2build.
func (c *Module) ConvertWithBp2build(ctx android.TopDownMutatorContext) {
	prebuilt := c.IsPrebuilt()
	switch c.typ() {
	case binary:
		if !prebuilt {
			binaryBp2build(ctx, c, ctx.ModuleType())
		}
	case object:
		if !prebuilt {
			objectBp2Build(ctx, c)
		}
	case fullLibrary:
		if !prebuilt {
			libraryBp2Build(ctx, c)
		} else {
			prebuiltLibraryBp2Build(ctx, c)
		}
	case headerLibrary:
		libraryHeadersBp2Build(ctx, c)
	case staticLibrary:
		if prebuilt {
			prebuiltLibraryStaticBp2Build(ctx, c, false)
		} else {
			sharedOrStaticLibraryBp2Build(ctx, c, true)
		}
	case sharedLibrary:
		if prebuilt {
			prebuiltLibrarySharedBp2Build(ctx, c)
		} else {
			sharedOrStaticLibraryBp2Build(ctx, c, false)
		}
	}
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
		&VndkProperties{},
		&LTOProperties{},
		&AfdoProperties{},
		&PgoProperties{},
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
