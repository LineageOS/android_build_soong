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
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/depset"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	cc_config "android/soong/cc/config"
	"android/soong/fuzz"
	"android/soong/rust/config"
)

//go:generate go run ../../blueprint/gobtools/codegen

var pctx = android.NewPackageContext("android/soong/rust")

// @auto-generate: gob
type LibraryInfo struct {
	Rlib  bool
	Dylib bool
}

// @auto-generate: gob
type CompilerInfo struct {
	StdLinkageForDevice    StdLinkage
	StdLinkageForNonDevice StdLinkage
	NoStdlibs              bool
	CrateName              string
	Edition                string
	CargoOutDir            android.OptionalPath
	Features               []string
	CrateRootPath          android.Path
	LibraryInfo            *LibraryInfo
	BuildTarget            android.Path
	CheckTarget            android.OptionalPath
}

// @auto-generate: gob
type ProtobufDecoratorInfo struct{}

// @auto-generate: gob
type SourceProviderInfo struct {
	Srcs                  android.Paths
	ProtobufDecoratorInfo *ProtobufDecoratorInfo
}

// @auto-generate: gob
type ProcMacroInfo struct {
	Dylib android.Path
}

// @auto-generate: gob
type RustInfo struct {
	AndroidMkSuffix               string
	RustSubName                   string
	TransitiveAndroidMkSharedLibs depset.DepSet[string]
	CompilerInfo                  *CompilerInfo
	SnapshotInfo                  *cc.SnapshotInfo
	SourceProviderInfo            *SourceProviderInfo
	ProcMacroInfo                 *ProcMacroInfo
	XrefRustFiles                 android.Paths
	DocTimestampFile              android.OptionalPath
	CollectDoc                    bool
}

var RustInfoProvider = blueprint.NewProvider[*RustInfo]()

func init() {
	android.RegisterModuleType("rust_defaults", defaultsFactory)
	android.PreDepsMutators(registerPreDepsMutators)
	android.PostDepsMutators(registerPostDepsMutators)
	pctx.Import("android/soong/android")
	pctx.Import("android/soong/rust/config")
	pctx.ImportAs("cc_config", "android/soong/cc/config")
}

func registerPreDepsMutators(ctx android.RegisterMutatorsContext) {
	ctx.Transition("rust_libraries", &libraryTransitionMutator{})
	ctx.Transition("rust_stdlinkage", &libstdTransitionMutator{})
	ctx.BottomUp("rust_begin", BeginMutator)
}

func registerPostDepsMutators(ctx android.RegisterMutatorsContext) {
	ctx.BottomUp("rust_sanitizers", rustSanitizerRuntimeMutator)
}

type Flags struct {
	GlobalRustFlags   cc_config.FlagsWithDeps // Flags that apply globally to rust
	GlobalLinkFlags   cc_config.FlagsWithDeps // Flags that apply globally to linker
	RustFlags         []string                // Flags that apply to rust
	LinkFlags         cc_config.FlagsWithDeps // Flags that apply to linker
	LinkerScriptFlags []string                // Flags that should be visible to the android linker script
	ClippyFlags       []string                // Flags that apply to clippy-driver, during the linting
	RustdocFlags      []string                // Flags that apply to rustdoc
	Toolchain         config.Toolchain
	Coverage          bool
	Clippy            bool
	EmitXrefs         bool // If true, emit rules to aid cross-referencing
}

type BaseProperties struct {
	AndroidMkRlibs         []string `blueprint:"mutated"`
	AndroidMkDylibs        []string `blueprint:"mutated"`
	AndroidMkProcMacroLibs []string `blueprint:"mutated"`
	AndroidMkStaticLibs    []string `blueprint:"mutated"`
	AndroidMkHeaderLibs    []string `blueprint:"mutated"`

	ImageVariation string `blueprint:"mutated"`
	VndkVersion    string `blueprint:"mutated"`
	SubName        string `blueprint:"mutated"`

	// SubName is used by CC for tracking image variants / SDK versions. RustSubName is used for Rust-specific
	// subnaming which shouldn't be visible to CC modules (such as the rlib stdlinkage subname). This should be
	// appended before SubName.
	RustSubName string `blueprint:"mutated"`

	// Set by imageMutator
	ProductVariantNeeded       bool     `blueprint:"mutated"`
	VendorVariantNeeded        bool     `blueprint:"mutated"`
	CoreVariantNeeded          bool     `blueprint:"mutated"`
	VendorRamdiskVariantNeeded bool     `blueprint:"mutated"`
	RamdiskVariantNeeded       bool     `blueprint:"mutated"`
	RecoveryVariantNeeded      bool     `blueprint:"mutated"`
	ExtraVariants              []string `blueprint:"mutated"`

	// Allows this module to use non-APEX version of libraries. Useful
	// for building binaries that are started before APEXes are activated.
	Bootstrap *bool

	// Used by vendor snapshot to record dependencies from snapshot modules.
	SnapshotSharedLibs []string `blueprint:"mutated"`
	SnapshotStaticLibs []string `blueprint:"mutated"`
	SnapshotRlibs      []string `blueprint:"mutated"`
	SnapshotDylibs     []string `blueprint:"mutated"`

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
	// the recovery variant instead
	Vendor_ramdisk_available *bool

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

	// Make this module available when building for recovery
	Recovery_available *bool

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

	// If true, always create an sdk variant and don't create a platform variant.
	Sdk_variant_only *bool

	// Minimum OS API level supported by this C or C++ module. This property becomes the value
	// of the __ANDROID_API__ macro. When the C or C++ module is included in an APEX or an APK,
	// this property is also used to ensure that the min_sdk_version of the containing module is
	// not older (i.e. less) than this module's min_sdk_version. When not set, this property
	// defaults to the value of sdk_version.  When this is set to "apex_inherit", this tracks
	// min_sdk_version of the containing APEX. When the module
	// is not built for an APEX, "apex_inherit" defaults to sdk_version.
	Min_sdk_version *string

	// Variant is an SDK variant created by sdkMutator
	IsSdkVariant bool `blueprint:"mutated"`
	// Set when both SDK and platform variants are exported to Make to trigger renaming the SDK
	// variant to have a ".sdk" suffix.
	SdkAndPlatformVariantVisibleToMake bool `blueprint:"mutated"`

	// Set by factories of module types that can only be referenced from variants compiled against
	// the SDK.
	AlwaysSdk bool `blueprint:"mutated"`

	HideFromMake   bool `blueprint:"mutated"`
	PreventInstall bool `blueprint:"mutated"`

	Installable *bool
}

type Module struct {
	fuzz.FuzzModule

	VendorProperties cc.VendorProperties

	Properties BaseProperties

	hod        android.HostOrDeviceSupported
	multilib   android.Multilib
	testModule bool

	makeLinkType string

	afdo             *afdo
	compiler         compiler
	coverage         *coverage
	clippy           *clippy
	sanitize         *sanitize
	cachedToolchain  config.Toolchain
	sourceProvider   SourceProvider
	subAndroidMkOnce map[SubAndroidMkProvider]bool

	exportedLinkDirs     []string
	exportedLinkDirsDeps []android.Path

	// Output file to be installed, may be stripped or unstripped.
	outputFile android.OptionalPath

	// Cross-reference input file
	kytheFiles android.Paths

	docTimestampFile android.OptionalPath

	// For apex variants, this is set as apex.min_sdk_version
	apexSdkVersion android.ApiLevel

	transitiveAndroidMkSharedLibs depset.DepSet[string]

	// Shared flags among stubs build rules of this module
	sharedFlags cc.SharedFlags
}

func (mod *Module) Header() bool {
	//TODO: If Rust libraries provide header variants, this needs to be updated.
	return false
}

func (mod *Module) SetPreventInstall() {
	mod.Properties.PreventInstall = true
}

func (mod *Module) SetHideFromMake() {
	mod.Properties.HideFromMake = true
	mod.ModuleBase.HideFromMake()
}

func (mod *Module) HiddenFromMake() bool {
	return mod.Properties.HideFromMake
}

func (mod *Module) SanitizePropDefined() bool {
	// Because compiler is not set for some Rust modules where sanitize might be set, check that compiler is also not
	// nil since we need compiler to actually sanitize.
	return mod.sanitize != nil && mod.compiler != nil
}

func (mod *Module) IsPrebuilt() bool {
	if _, ok := mod.compiler.(*prebuiltLibraryDecorator); ok {
		return true
	}
	if _, ok := mod.compiler.(*prebuiltBinaryDecorator); ok {
		return true
	}
	return false
}

func (mod *Module) SelectedStl() string {
	return ""
}

func (mod *Module) NonCcVariants() bool {
	if mod.compiler != nil {
		if library, ok := mod.compiler.(libraryInterface); ok {
			return library.buildRlib() || library.buildDylib()
		}
	}
	panic(fmt.Errorf("NonCcVariants called on non-library module: %q", mod.BaseModuleName()))
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

func (mod *Module) Dylib() bool {
	if mod.compiler != nil {
		if library, ok := mod.compiler.(libraryInterface); ok {
			return library.dylib()
		}
	}
	return false
}

func (mod *Module) Source() bool {
	if mod.compiler != nil {
		if library, ok := mod.compiler.(libraryInterface); ok && mod.sourceProvider != nil {
			return library.source()
		}
	}
	return false
}

func (mod *Module) RlibStd() bool {
	if mod.compiler != nil {
		if library, ok := mod.compiler.(libraryInterface); ok && library.rlib() {
			return library.rlibStd()
		}
	}
	panic(fmt.Errorf("RlibStd() called on non-rlib module: %q", mod.BaseModuleName()))
}

func (mod *Module) Rlib() bool {
	if mod.compiler != nil {
		if library, ok := mod.compiler.(libraryInterface); ok {
			return library.rlib()
		}
	}
	return false
}

func (mod *Module) Binary() bool {
	if binary, ok := mod.compiler.(binaryInterface); ok {
		return binary.binary()
	}
	return false
}

func (mod *Module) StaticExecutable() bool {
	if !mod.Binary() {
		return false
	}
	return mod.StaticallyLinked()
}

func (mod *Module) Xom() *bool {
	if mod.compiler == nil {
		return nil
	}
	return mod.compiler.Xom()
}

func (mod *Module) ApexExclude() bool {
	if mod.compiler != nil {
		if library, ok := mod.compiler.(libraryInterface); ok {
			return library.apexExclude()
		}
	}
	return false
}

func (mod *Module) Object() bool {
	// Rust has no modules which produce only object files.
	return false
}

func (mod *Module) Toc() android.OptionalPath {
	if mod.compiler != nil {
		if lib, ok := mod.compiler.(libraryInterface); ok {
			return lib.toc()
		}
	}
	panic(fmt.Errorf("Toc() called on non-library module: %q", mod.BaseModuleName()))
}

func (mod *Module) UseSdk() bool {
	if cc.CanUseSdk(mod) {
		return String(mod.Properties.Sdk_version) != ""
	}
	return false
}

func (mod *Module) RelativeInstallPath() string {
	if mod.compiler != nil {
		return mod.compiler.relativeInstallPath()
	}
	return ""
}

func (mod *Module) UseVndk() bool {
	return mod.Properties.VndkVersion != ""
}

func (mod *Module) Bootstrap() bool {
	return Bool(mod.Properties.Bootstrap)
}

func (mod *Module) SubName() string {
	return mod.Properties.SubName
}

func (mod *Module) IsVndkPrebuiltLibrary() bool {
	// Rust modules do not provide VNDK prebuilts
	return false
}

func (mod *Module) IsVendorPublicLibrary() bool {
	// Rust modules do not currently support vendor_public_library
	return false
}

func (mod *Module) SdkAndPlatformVariantVisibleToMake() bool {
	return mod.Properties.SdkAndPlatformVariantVisibleToMake
}

func (c *Module) IsVndkPrivate() bool {
	// Rust modules do not currently support VNDK variants
	return false
}

func (c *Module) IsLlndk() bool {
	// Rust modules do not currently support LLNDK variants
	return false
}

func (mod *Module) KernelHeadersDecorator() bool {
	return false
}

func (m *Module) NeedsLlndkVariants() bool {
	// Rust modules do not currently support LLNDK variants
	return false
}

func (m *Module) NeedsVendorPublicLibraryVariants() bool {
	// Rust modules do not currently support vendor_public_library
	return false
}

func (mod *Module) HasLlndkStubs() bool {
	// Rust modules do not currently support LLNDK stubs
	return false
}

func (mod *Module) SdkVersion() string {
	return String(mod.Properties.Sdk_version)
}

func (mod *Module) AlwaysSdk() bool {
	return mod.Properties.AlwaysSdk || Bool(mod.Properties.Sdk_variant_only)
}

func (mod *Module) IsSdkVariant() bool {
	return mod.Properties.IsSdkVariant
}

func (mod *Module) SplitPerApiLevel() bool {
	return cc.CanUseSdk(mod) && mod.IsCrt()
}

func (mod *Module) XrefRustFiles() android.Paths {
	return mod.kytheFiles
}

type Deps struct {
	Dylibs          []string
	Rlibs           []string
	Rustlibs        []string
	NoStdRlibs      []string
	Stdlibs         []string
	ProcMacros      []string
	SharedLibs      []string
	StaticLibs      []string
	WholeStaticLibs []string
	HeaderLibs      []string

	// Used for data dependencies adjacent to tests
	DataLibs []string
	DataBins []string

	CrtBegin, CrtEnd []string
}

type PathDeps struct {
	DyLibs        RustLibraries
	RLibs         RustLibraries
	SharedLibs    android.Paths
	SharedLibDeps android.Paths
	StaticLibs    android.Paths
	ProcMacros    RustLibraries
	AfdoProfiles  android.Paths
	LinkerDeps    android.Paths

	// depFlags and depLinkFlags are rustc and linker (clang) flags.
	depFlags     []string
	depLinkFlags []string

	// track cc static-libs that have Rlib dependencies
	reexportedCcRlibDeps      []cc.RustRlibDep
	reexportedWholeCcRlibDeps []cc.RustRlibDep
	ccRlibDeps                []cc.RustRlibDep

	// linkDirs are link paths passed via -L to rustc. linkObjects are objects passed directly to the linker
	// Both of these are exported and propagate to dependencies.
	linkDirs              []string
	linkDirsDeps          []android.Path
	rustLibObjects        []string
	staticLibObjects      []string
	wholeStaticLibObjects []string
	sharedLibObjects      []string

	// exportedLinkDirs are exported linkDirs for direct rlib dependencies to
	// cc_library_static dependants of rlibs.
	// Track them separately from linkDirs so superfluous -L flags don't get emitted.
	exportedLinkDirs     []string
	exportedLinkDirsDeps []android.Path

	// Used by bindgen modules which call clang
	depClangFlags         []string
	depIncludePaths       android.Paths
	depGeneratedHeaders   android.Paths
	depSystemIncludePaths android.Paths

	CrtBegin android.Paths
	CrtEnd   android.Paths

	SrcFiles android.Paths

	// Paths to generated source files
	SrcDeps          android.Paths
	srcProviderFiles android.Paths

	directApexImplementationDeps        android.Paths
	transitiveApexImplementationDeps    []depset.DepSet[android.Path]
	directNonApexImplementationDeps     android.Paths
	transitiveNonApexImplementationDeps []depset.DepSet[android.Path]
}

type RustLibraries []RustLibrary

type RustLibrary struct {
	Path      android.Path
	CrateName string
}

type exportedFlagsProducer interface {
	exportLinkDirs(dirs []string, deps []android.Path)
	exportRustLibs(...string)
	exportStaticLibs(...string)
	exportWholeStaticLibs(...string)
	exportSharedLibs(...string)
}

type xref interface {
	XrefRustFiles() android.Paths
}

type flagExporter struct {
	linkDirs              []string
	linkDirsDeps          []android.Path
	ccLinkDirs            []string
	rustLibPaths          []string
	staticLibObjects      []string
	sharedLibObjects      []string
	wholeStaticLibObjects []string
	wholeRustRlibDeps     []cc.RustRlibDep
}

func (flagExporter *flagExporter) exportLinkDirs(dirs []string, deps []android.Path) {
	flagExporter.linkDirs = android.FirstUniqueStrings(append(flagExporter.linkDirs, dirs...))
	flagExporter.linkDirsDeps = android.FirstUniquePaths(append(flagExporter.linkDirsDeps, deps...))
}

func (flagExporter *flagExporter) exportRustLibs(flags ...string) {
	flagExporter.rustLibPaths = android.FirstUniqueStrings(append(flagExporter.rustLibPaths, flags...))
}

func (flagExporter *flagExporter) exportStaticLibs(flags ...string) {
	flagExporter.staticLibObjects = android.FirstUniqueStrings(append(flagExporter.staticLibObjects, flags...))
}

func (flagExporter *flagExporter) exportSharedLibs(flags ...string) {
	flagExporter.sharedLibObjects = android.FirstUniqueStrings(append(flagExporter.sharedLibObjects, flags...))
}

func (flagExporter *flagExporter) exportWholeStaticLibs(flags ...string) {
	flagExporter.wholeStaticLibObjects = android.FirstUniqueStrings(append(flagExporter.wholeStaticLibObjects, flags...))
}

func (flagExporter *flagExporter) setRustProvider(ctx ModuleContext) {
	android.SetProvider(ctx, RustFlagExporterInfoProvider, RustFlagExporterInfo{
		LinkDirs:              flagExporter.linkDirs,
		LinkDirsDeps:          flagExporter.linkDirsDeps,
		RustLibObjects:        flagExporter.rustLibPaths,
		StaticLibObjects:      flagExporter.staticLibObjects,
		WholeStaticLibObjects: flagExporter.wholeStaticLibObjects,
		SharedLibPaths:        flagExporter.sharedLibObjects,
		WholeRustRlibDeps:     flagExporter.wholeRustRlibDeps,
	})
}

var _ exportedFlagsProducer = (*flagExporter)(nil)

func NewFlagExporter() *flagExporter {
	return &flagExporter{}
}

// @auto-generate: gob
type RustFlagExporterInfo struct {
	Flags                 []string
	LinkDirs              []string
	LinkDirsDeps          []android.Path
	RustLibObjects        []string
	StaticLibObjects      []string
	WholeStaticLibObjects []string
	SharedLibPaths        []string
	WholeRustRlibDeps     []cc.RustRlibDep
}

var RustFlagExporterInfoProvider = blueprint.NewProvider[RustFlagExporterInfo]()

var _ cc.Coverage = (*Module)(nil)

func (mod *Module) IsNativeCoverageNeeded(ctx cc.IsNativeCoverageNeededContext) bool {
	return mod.coverage != nil && mod.coverage.Properties.NeedCoverageVariant
}

func (mod *Module) VndkVersion() string {
	return mod.Properties.VndkVersion
}

func (mod *Module) ExportedCrateLinkDirs() ([]string, android.Paths) {
	return mod.exportedLinkDirs, mod.exportedLinkDirsDeps
}

func (mod *Module) PreventInstall() bool {
	return mod.Properties.PreventInstall
}
func (c *Module) ForceDisableSanitizers() {
	c.sanitize.Properties.ForceDisable = true
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

func DefaultsFactory(props ...any) android.Module {
	module := &Defaults{}

	module.AddProperties(props...)
	module.AddProperties(
		&BaseProperties{},
		&cc.AfdoProperties{},
		&cc.VendorProperties{},
		&BenchmarkProperties{},
		&BindgenProperties{},
		&BaseCompilerProperties{},
		&ObjectProperties{},
		&BinaryCompilerProperties{},
		&LibraryCompilerProperties{},
		&ProcMacroCompilerProperties{},
		&PrebuiltProperties{},
		&SourceProviderProperties{},
		&TestProperties{},
		&cc.CoverageProperties{},
		&cc.RustBindgenClangProperties{},
		&ClippyProperties{},
		&SanitizeProperties{},
		&cc.StripProperties{},
		&fuzz.FuzzProperties{},
	)

	android.InitDefaultsModule(module)
	return module
}

func (mod *Module) CrateName() string {
	return mod.compiler.crateName()
}

func (mod *Module) CcLibrary() bool {
	if mod.compiler != nil {
		if _, ok := mod.compiler.(libraryInterface); ok {
			return true
		}
	}
	return false
}

func (mod *Module) CcLibraryInterface() bool {
	if mod.compiler != nil {
		// use build{Static,Shared}() instead of {static,shared}() here because this might be called before
		// VariantIs{Static,Shared} is set.
		if lib, ok := mod.compiler.(libraryInterface); ok && (lib.buildShared() || lib.buildStatic() || lib.buildRlib()) {
			return true
		}
	}
	return false
}

func (mod *Module) RustLibraryInterface() bool {
	if mod.compiler != nil {
		if _, ok := mod.compiler.(libraryInterface); ok {
			return true
		}
	}
	return false
}

func (mod *Module) IsFuzzModule() bool {
	if _, ok := mod.compiler.(*fuzzDecorator); ok {
		return true
	}
	return false
}

func (mod *Module) FuzzModuleStruct() fuzz.FuzzModule {
	return mod.FuzzModule
}

func (mod *Module) FuzzPackagedModule() fuzz.FuzzPackagedModule {
	if fuzzer, ok := mod.compiler.(*fuzzDecorator); ok {
		return fuzzer.fuzzPackagedModule
	}
	panic(fmt.Errorf("FuzzPackagedModule called on non-fuzz module: %q", mod.BaseModuleName()))
}

func (mod *Module) FuzzSharedLibraries() cc.InstallPairs {
	if fuzzer, ok := mod.compiler.(*fuzzDecorator); ok {
		return fuzzer.sharedLibraries
	}
	panic(fmt.Errorf("FuzzSharedLibraries called on non-fuzz module: %q", mod.BaseModuleName()))
}

func (mod *Module) UnstrippedOutputFile() android.Path {
	if mod.compiler != nil {
		return mod.compiler.unstrippedOutputFilePath()
	}
	return nil
}

func (mod *Module) CheckJsonFilePath() android.OptionalPath {
	if mod.compiler != nil {
		return mod.compiler.checkJsonFilePath()
	}
	return android.OptionalPath{}
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

func (mod *Module) BuildStaticVariant() bool {
	if mod.compiler != nil {
		if library, ok := mod.compiler.(libraryInterface); ok {
			return library.buildStatic()
		}
	}
	panic(fmt.Errorf("BuildStaticVariant called on non-library module: %q", mod.BaseModuleName()))
}

func (mod *Module) BuildRlibVariant() bool {
	if mod.compiler != nil {
		if library, ok := mod.compiler.(libraryInterface); ok {
			return library.buildRlib()
		}
	}
	panic(fmt.Errorf("BuildRlibVariant called on non-library module: %q", mod.BaseModuleName()))
}

func (mod *Module) BuildSharedVariant() bool {
	if mod.compiler != nil {
		if library, ok := mod.compiler.(libraryInterface); ok {
			return library.buildShared()
		}
	}
	panic(fmt.Errorf("BuildSharedVariant called on non-library module: %q", mod.BaseModuleName()))
}

func (mod *Module) Module() android.Module {
	return mod
}

func (mod *Module) OutputFile() android.OptionalPath {
	return mod.outputFile
}

func (mod *Module) CoverageFiles() android.Paths {
	if mod.compiler != nil {
		return android.Paths{}
	}
	panic(fmt.Errorf("CoverageFiles called on non-library module: %q", mod.BaseModuleName()))
}

// Rust does not produce gcno files, and therefore does not produce a coverage archive.
func (mod *Module) CoverageOutputFile() android.OptionalPath {
	return android.OptionalPath{}
}

func (c *Module) LinkCoverage() bool {
	return false
}

func (mod *Module) IsNdk(config android.Config) bool {
	return false
}

func (mod *Module) IsStubs() bool {
	if lib, ok := mod.compiler.(libraryInterface); ok {
		return lib.BuildStubs()
	}
	return false
}

func (mod *Module) HasStubsVariants() bool {
	if lib, ok := mod.compiler.(libraryInterface); ok {
		return lib.HasStubsVariants()
	}
	return false
}

func (mod *Module) ApexSdkVersion() android.ApiLevel {
	return mod.apexSdkVersion
}

func (mod *Module) RustApexExclude() bool {
	return mod.ApexExclude()
}

func (mod *Module) getSharedFlags() *cc.SharedFlags {
	shared := &mod.sharedFlags
	if shared.FlagsMap == nil {
		shared.NumSharedFlags = 0
		shared.FlagsMap = make(map[string]string)
	}
	return shared
}

func (mod *Module) ImplementationModuleNameForMake() string {
	name := mod.BaseModuleName()
	if versioned, ok := mod.compiler.(cc.VersionedInterface); ok {
		name = versioned.ImplementationModuleName(name)
	}
	return name
}

func (mod *Module) Multilib() string {
	return mod.Arch().ArchType.Multilib
}

func (mod *Module) IsCrt() bool {
	if obj, ok := mod.compiler.(objectInterface); ok {
		return obj.crt()
	}
	return false
}

func (mod *Module) installable(apexInfo android.ApexInfo) bool {
	if !proptools.BoolDefault(mod.Installable(), mod.EverInstallable()) {
		return false
	}
	if mod.PreventInstall() {
		return false
	}
	// The apex variant is not installable because it is included in the APEX and won't appear
	// in the system partition as a standalone file.
	if !apexInfo.IsForPlatform() {
		return false
	}

	return mod.OutputFile().Valid() && !mod.Properties.PreventInstall
}

func (ctx moduleContext) apexVariationName() string {
	apexInfo, _ := android.ModuleProvider(ctx, android.ApexInfoProvider)
	return apexInfo.ApexVariationName
}

var _ cc.LinkableInterface = (*Module)(nil)
var _ cc.VersionedLinkableInterface = (*Module)(nil)

func (mod *Module) Init() android.Module {
	mod.AddProperties(&mod.Properties)
	mod.AddProperties(&mod.VendorProperties)

	if mod.afdo != nil {
		mod.AddProperties(mod.afdo.props()...)
	}
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
	if mod.sanitize != nil {
		mod.AddProperties(mod.sanitize.props()...)
	}

	android.InitAndroidArchModule(mod, mod.hod, mod.multilib)
	android.InitApexModule(mod)

	android.InitDefaultableModule(mod)
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
	module.afdo = &afdo{}
	module.coverage = &coverage{}
	module.clippy = &clippy{}
	module.sanitize = &sanitize{}
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
	// Bug: http://b/137883967 - native-bridge modules do not currently work with coverage
	if mod.Target().NativeBridge == android.NativeBridgeEnabled {
		return false
	}
	return mod.compiler != nil && mod.compiler.nativeCoverage()
}

func (mod *Module) SetStl(s string) {
	// STL is a CC concept; do nothing for Rust
}

func (mod *Module) SetSdkVersion(s *string) {
	mod.Properties.Sdk_version = s
}

func (mod *Module) SetSdkAndPlatformVariantVisibleToMake() {
	mod.Properties.SdkAndPlatformVariantVisibleToMake = true
}

func (mod *Module) SetSdkVariant() {
	mod.Properties.IsSdkVariant = true
}

func (mod *Module) SetMinSdkVersion(s string) {
	mod.Properties.Min_sdk_version = StringPtr(s)
}

func (mod *Module) VersionedInterface() cc.VersionedInterface {
	if _, ok := mod.compiler.(cc.VersionedInterface); ok {
		return mod.compiler.(cc.VersionedInterface)
	}
	return nil
}

func (mod *Module) EverInstallable() bool {
	return mod.compiler != nil &&
		// Check to see whether the module is actually ever installable.
		mod.compiler.everInstallable()
}

func (mod *Module) Installable() *bool {
	return mod.Properties.Installable
}

func (mod *Module) ProcMacro() bool {
	if pm, ok := mod.compiler.(procMacroInterface); ok {
		return pm.ProcMacro()
	}
	return false
}

func (mod *Module) toolchain(ctx android.BaseModuleContext) config.Toolchain {
	if mod.cachedToolchain == nil {
		mod.cachedToolchain = config.FindToolchain(ctx.Os(), ctx.Arch())
	}
	return mod.cachedToolchain
}

func (mod *Module) ccToolchain(ctx android.BaseModuleContext) cc_config.Toolchain {
	return cc_config.FindToolchain(ctx.Os(), ctx.Arch(), false)
}

func (d *Defaults) GenerateAndroidBuildActions(ctx android.ModuleContext) {
}

func (mod *Module) GenerateAndroidBuildActions(actx android.ModuleContext) {
	ctx := &moduleContext{
		ModuleContext: actx,
	}

	apexInfo, _ := android.ModuleProvider(actx, android.ApexInfoProvider)
	if !apexInfo.IsForPlatform() {
		mod.SetHideFromMake()
	}

	toolchain := mod.toolchain(ctx)
	mod.makeLinkType = cc.GetMakeLinkType(actx, mod)

	mod.Properties.SubName = cc.GetSubnameProperty(actx, mod)

	if !toolchain.Supported() {
		// This toolchain's unsupported, there's nothing to do for this mod.
		return
	}

	deps, testSuiteSharedLibs := mod.depsToPaths(ctx)
	// Export linkDirs for CC rust generatedlibs
	mod.exportedLinkDirs = append(mod.exportedLinkDirs, deps.exportedLinkDirs...)
	mod.exportedLinkDirs = append(mod.exportedLinkDirs, deps.linkDirs...)
	mod.exportedLinkDirsDeps = append(mod.exportedLinkDirsDeps, deps.exportedLinkDirsDeps...)
	mod.exportedLinkDirsDeps = append(mod.exportedLinkDirsDeps, deps.linkDirsDeps...)

	flags := Flags{
		Toolchain: toolchain,
	}

	// Calculate rustc flags
	if mod.afdo != nil {
		flags, deps = mod.afdo.flags(actx, flags, deps)
	}
	if mod.compiler != nil {
		flags = mod.compiler.compilerFlags(ctx, flags)
		flags = mod.compiler.cfgFlags(ctx, flags)
		flags = mod.compiler.featureFlags(ctx, flags)
	}
	if mod.coverage != nil {
		flags, deps = mod.coverage.flags(ctx, flags, deps)
	}
	if mod.clippy != nil {
		flags, deps = mod.clippy.flags(ctx, flags, deps)
	}
	if mod.sanitize != nil {
		flags, deps = mod.sanitize.flags(ctx, flags, deps)
	}

	// SourceProvider needs to call GenerateSource() before compiler calls
	// compile() so it can provide the source. A SourceProvider has
	// multiple variants (e.g. source, rlib, dylib). Only the "source"
	// variant is responsible for effectively generating the source. The
	// remaining variants relies on the "source" variant output.
	if mod.sourceProvider != nil {
		if mod.compiler.(libraryInterface).source() {
			mod.sourceProvider.GenerateSource(ctx, deps)
			mod.sourceProvider.setSubName(ctx.ModuleSubDir())
		} else {
			sourceMod := actx.GetDirectDepProxyWithTag(mod.Name(), sourceDepTag)
			sourceLib := android.OtherModuleProviderOrDefault(ctx, sourceMod, RustInfoProvider).SourceProviderInfo
			mod.sourceProvider.setOutputFiles(sourceLib.Srcs)
		}
		ctx.CheckbuildFile(mod.sourceProvider.Srcs()...)
	}

	if mod.compiler != nil && !mod.compiler.Disabled() {
		buildOutput := mod.compiler.compile(ctx, flags, deps)
		if ctx.Failed() {
			return
		}
		mod.outputFile = android.OptionalPathForPath(buildOutput.outputFile)
		ctx.CheckbuildFile(buildOutput.outputFile)
		if buildOutput.kytheFile != nil {
			mod.kytheFiles = append(mod.kytheFiles, buildOutput.kytheFile)
		}

		deps.SrcFiles = append(deps.SrcFiles, mod.compiler.crateSources(ctx)...)
		mod.docTimestampFile = mod.compiler.rustdoc(ctx, flags, deps)

		apexInfo, _ := android.ModuleProvider(actx, android.ApexInfoProvider)
		if !proptools.BoolDefault(mod.Installable(), mod.EverInstallable()) && !mod.ProcMacro() {
			// If the module has been specifically configure to not be installed then
			// hide from make as otherwise it will break when running inside make as the
			// output path to install will not be specified. Not all uninstallable
			// modules can be hidden from make as some are needed for resolving make
			// side dependencies. In particular, proc-macros need to be captured in the
			// host snapshot.
			mod.HideFromMake()
			mod.SkipInstall()
		} else if !mod.installable(apexInfo) {
			mod.SkipInstall()
		}

		// Still call install though, the installs will be stored as PackageSpecs to allow
		// using the outputs in a genrule.
		if mod.OutputFile().Valid() {
			mod.compiler.install(ctx)
			if ctx.Failed() {
				return
			}
			// Export your own directory as a linkDir
			mod.exportedLinkDirs = append(mod.exportedLinkDirs, linkPathFromFilePath(mod.OutputFile().Path()))
			mod.exportedLinkDirsDeps = append(mod.exportedLinkDirsDeps, mod.OutputFile().Path())
		}

		android.SetProvider(ctx, cc.ImplementationDepInfoProvider, &cc.ImplementationDepInfo{
			ImplementationDeps: depset.New(depset.PREORDER, deps.directApexImplementationDeps, deps.transitiveApexImplementationDeps),
		})
		android.SetProvider(ctx, RustImplementationDepInfoProvider, &RustImplementationDepInfo{
			NonApexImplementationDeps: depset.New(depset.PREORDER, deps.directNonApexImplementationDeps, deps.transitiveNonApexImplementationDeps),
		})

		ctx.Phony("rust", ctx.RustModule().OutputFile().Path())
	}

	linkableInfo := cc.CreateCommonLinkableInfo(ctx, mod)
	linkableInfo.Static = mod.Static()
	linkableInfo.Shared = mod.Shared()
	linkableInfo.Rlib = mod.Rlib()
	linkableInfo.CrateName = mod.CrateName()
	exportedLinkDirs, exportedLinkDirsDeps := mod.ExportedCrateLinkDirs()
	linkableInfo.ExportedCrateLinkDirs = android.FirstUniqueStrings(exportedLinkDirs)
	linkableInfo.ExportedCrateLinkDirsDeps = android.FirstUniquePaths(exportedLinkDirsDeps)
	if lib, ok := mod.compiler.(cc.VersionedInterface); ok {
		linkableInfo.StubsVersion = lib.StubsVersion()
	}
	linkableInfo.FuzzDependencies = cc.PropagateSharedLibraryFuzzerDependencies(ctx, android.OptionalPath{}, false)

	android.SetProvider(ctx, cc.LinkableInfoProvider, linkableInfo)

	rustInfo := &RustInfo{
		AndroidMkSuffix:               mod.AndroidMkSuffix(),
		RustSubName:                   mod.Properties.RustSubName,
		TransitiveAndroidMkSharedLibs: mod.transitiveAndroidMkSharedLibs,
		XrefRustFiles:                 mod.XrefRustFiles(),
		DocTimestampFile:              mod.docTimestampFile,
		CollectDoc:                    config.DocConfigForDir(ctx.ModuleDir()),
	}
	if mod.compiler != nil {
		rustInfo.CompilerInfo = &CompilerInfo{
			NoStdlibs:              mod.compiler.noStdlibs(),
			CrateName:              mod.compiler.crateName(),
			Edition:                mod.compiler.edition(),
			CargoOutDir:            mod.compiler.cargoOutDir(ctx),
			Features:               mod.compiler.features(ctx),
			CrateRootPath:          mod.compiler.crateRootPath(ctx),
			StdLinkageForDevice:    mod.compiler.stdLinkage(true),
			StdLinkageForNonDevice: mod.compiler.stdLinkage(false),
			BuildTarget:            mod.compiler.unstrippedOutputFilePath(),
			CheckTarget:            mod.compiler.checkJsonFilePath(),
		}
		if lib, ok := mod.compiler.(libraryInterface); ok {
			rustInfo.CompilerInfo.LibraryInfo = &LibraryInfo{
				Dylib: lib.dylib(),
				Rlib:  lib.rlib(),
			}
		}
		if lib, ok := mod.compiler.(cc.SnapshotInterface); ok {
			rustInfo.SnapshotInfo = &cc.SnapshotInfo{
				SnapshotAndroidMkSuffix: lib.SnapshotAndroidMkSuffix(),
			}
		}
	}
	if mod.sourceProvider != nil {
		rustInfo.SourceProviderInfo = &SourceProviderInfo{
			Srcs: mod.sourceProvider.Srcs(),
		}
		if _, ok := mod.sourceProvider.(*protobufDecorator); ok {
			rustInfo.SourceProviderInfo.ProtobufDecoratorInfo = &ProtobufDecoratorInfo{}
		}
	}
	if _, ok := mod.compiler.(*procMacroDecorator); ok {
		rustInfo.ProcMacroInfo = &ProcMacroInfo{
			Dylib: mod.compiler.unstrippedOutputFilePath(),
		}
	}
	android.SetProvider(ctx, RustInfoProvider, rustInfo)

	// TODO: Refactor rustMakeLibName so we don't have to fake CommonModuleInfo like this
	myCommonInfo := android.CommonModuleInfo{
		BaseModuleName: mod.BaseModuleName(),
		Target:         ctx.Target(),
	}
	ctx.SetMakeNamesInfo(&android.MakeNamesInfo{
		SharedLibsMakeNames: testSuiteSharedLibs,
		MakeName:            rustMakeLibName(rustInfo, linkableInfo, &myCommonInfo, ctx.ModuleName()),
	})

	mod.setOutputFiles(ctx)

	buildComplianceMetadataInfo(ctx, mod, deps)

	moduleInfoJSON := ctx.ModuleInfoJSON()
	if mod.compiler != nil {
		mod.compiler.moduleInfoJSON(ctx, moduleInfoJSON)
	}

	mod.setSymbolsInfoProvider(ctx)
}

func (mod *Module) baseSymbolInfo(ctx android.ModuleContext) *cc.SymbolInfo {
	return &cc.SymbolInfo{
		Name:          mod.BaseModuleName() + mod.Properties.SubName,
		ModuleDir:     ctx.ModuleDir(),
		Uninstallable: mod.IsSkipInstall() || !proptools.BoolDefault(mod.Properties.Installable, true) || mod.NoFullInstall(),
	}
}

func (mod *Module) getSymbolInfo(ctx android.ModuleContext, t any, info *cc.SymbolInfo) *cc.SymbolInfo {
	switch tt := t.(type) {
	case *binaryDecorator:
		mod.getSymbolInfo(ctx, tt.baseCompiler, info)
	case *testDecorator:
		mod.getSymbolInfo(ctx, tt.binaryDecorator, info)
	case *benchmarkDecorator:
		mod.getSymbolInfo(ctx, tt.binaryDecorator, info)
	case *libraryDecorator:
		mod.getSymbolInfo(ctx, tt.baseCompiler, info)
	case *procMacroDecorator:
		mod.getSymbolInfo(ctx, tt.baseCompiler, info)
	case *BaseSourceProvider:
		outFile := tt.OutputFiles[0]
		_, file := filepath.Split(outFile.String())
		stem, suffix, _ := android.SplitFileExt(file)
		info.Suffix = suffix
		info.Stem = stem
		info.Uninstallable = true
	case *bindgenDecorator:
		mod.getSymbolInfo(ctx, tt.BaseSourceProvider, info)
	case *protobufDecorator:
		mod.getSymbolInfo(ctx, tt.BaseSourceProvider, info)
	case *baseCompiler:
		if tt.path != (android.InstallPath{}) {
			info.UnstrippedBinaryPath = tt.unstrippedOutputFile
			path, file := filepath.Split(tt.path.String())
			stem, suffix, _ := android.SplitFileExt(file)
			info.Suffix = suffix
			info.ModuleDir = path
			info.Stem = stem
		}
	case *fuzzDecorator:
		mod.getSymbolInfo(ctx, tt.binaryDecorator, info)
	case *prebuiltLibraryDecorator:
		mod.getSymbolInfo(ctx, tt.baseCompiler, info)
	case *prebuiltBinaryDecorator:
		mod.getSymbolInfo(ctx, tt.baseCompiler, info)
	case *toolchainLibraryDecorator:
		mod.getSymbolInfo(ctx, tt.baseCompiler, info)
	}
	return info
}

func (mod *Module) setSymbolsInfoProvider(ctx android.ModuleContext) {
	if !mod.Properties.HideFromMake {
		infos := &cc.SymbolInfos{}
		if mod.compiler != nil && !mod.compiler.Disabled() {
			infos.AppendSymbols(mod.getSymbolInfo(ctx, mod.compiler, mod.baseSymbolInfo(ctx)))
		} else if mod.sourceProvider != nil {
			infos.AppendSymbols(mod.getSymbolInfo(ctx, mod.sourceProvider, mod.baseSymbolInfo(ctx)))
		}

		if mod.sanitize != nil {
			infos.AppendSymbols(mod.getSymbolInfo(ctx, mod.sanitize, mod.baseSymbolInfo(ctx)))
		}

		cc.CopySymbolsAndSetSymbolsInfoProvider(ctx, infos)
	}
}

func (mod *Module) setOutputFiles(ctx ModuleContext) {
	if mod.sourceProvider != nil && (mod.compiler == nil || mod.compiler.Disabled()) {
		ctx.SetOutputFiles(mod.sourceProvider.Srcs(), "")
	} else if mod.OutputFile().Valid() {
		ctx.SetOutputFiles(android.Paths{mod.OutputFile().Path()}, "")
	} else {
		ctx.SetOutputFiles(android.Paths{}, "")
	}
	if mod.compiler != nil {
		ctx.SetOutputFiles(android.PathsIfNonNil(mod.compiler.unstrippedOutputFilePath()), "unstripped")
	}
}

func buildComplianceMetadataInfo(ctx *moduleContext, mod *Module, deps PathDeps) {
	// Dump metadata that can not be done in android/compliance-metadata.go
	metadataInfo := ctx.ComplianceMetadataInfo()
	metadataInfo.SetStringValue(android.ComplianceMetadataProp.IS_STATIC_LIB, strconv.FormatBool(mod.Static()))
	metadataInfo.AddBuiltFiles(mod.outputFile.String())

	// Static libs
	staticDeps := ctx.GetDirectDepsProxyWithTag(rlibDepTag)
	staticDepNames := make([]string, 0, len(staticDeps))
	for _, dep := range staticDeps {
		staticDepNames = append(staticDepNames, dep.Name())
	}
	ccStaticDeps := ctx.GetDirectDepsProxyWithTag(cc.StaticDepTag(false))
	for _, dep := range ccStaticDeps {
		staticDepNames = append(staticDepNames, dep.Name())
	}

	staticDepPaths := make([]string, 0, len(deps.StaticLibs)+len(deps.RLibs))
	// C static libraries
	for _, dep := range deps.StaticLibs {
		staticDepPaths = append(staticDepPaths, dep.String())
	}
	// Rust static libraries
	for _, dep := range deps.RLibs {
		staticDepPaths = append(staticDepPaths, dep.Path.String())
	}
	metadataInfo.SetListValue(android.ComplianceMetadataProp.STATIC_DEP_FILES, android.FirstUniqueStrings(staticDepPaths))

	// C Whole static libs
	ccWholeStaticDeps := ctx.GetDirectDepsProxyWithTag(cc.StaticDepTag(true))
	wholeStaticDepNames := make([]string, 0, len(ccWholeStaticDeps))
	for _, dep := range ccStaticDeps {
		wholeStaticDepNames = append(wholeStaticDepNames, dep.Name())
	}

	allStaticDepNames := append(staticDepNames, wholeStaticDepNames...)
	metadataInfo.SetListValue(android.ComplianceMetadataProp.STATIC_DEPS, android.FirstUniqueStrings(allStaticDepNames))
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
	deps.NoStdRlibs = android.LastUniqueStrings(deps.NoStdRlibs)
	deps.ProcMacros = android.LastUniqueStrings(deps.ProcMacros)
	deps.SharedLibs = android.LastUniqueStrings(deps.SharedLibs)
	deps.StaticLibs = android.LastUniqueStrings(deps.StaticLibs)
	deps.Stdlibs = android.LastUniqueStrings(deps.Stdlibs)
	deps.WholeStaticLibs = android.LastUniqueStrings(deps.WholeStaticLibs)
	return deps

}

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name      string
	library   bool
	procMacro bool
	dynamic   bool
}

// InstallDepNeeded returns true for rlibs, dylibs, and proc macros so that they or their transitive
// dependencies (especially C/C++ shared libs) are installed as dependencies of a rust binary.
func (d dependencyTag) InstallDepNeeded() bool {
	return d.library || d.procMacro
}

var _ android.InstallNeededDependencyTag = dependencyTag{}

func (d dependencyTag) LicenseAnnotations() []android.LicenseAnnotation {
	if d.library && d.dynamic {
		return []android.LicenseAnnotation{android.LicenseAnnotationSharedDependency}
	}
	return nil
}

func (d dependencyTag) PropagateAconfigValidation() bool {
	return d == rlibDepTag || d == sourceDepTag
}

func (d dependencyTag) IsNativeCoverageNeededDepTag(ctx cc.IsNativeCoverageNeededContext) bool {
	return d.dynamic
}

var _ cc.UseCoverageDeptag = dependencyTag{}

var _ android.PropagateAconfigValidationDependencyTag = dependencyTag{}

var _ android.LicenseAnnotationsDependencyTag = dependencyTag{}

var (
	customBindgenDepTag = dependencyTag{name: "customBindgenTag"}
	rlibDepTag          = dependencyTag{name: "rlibTag", library: true}
	dylibDepTag         = dependencyTag{name: "dylib", library: true, dynamic: true}
	procMacroDepTag     = dependencyTag{name: "procMacro", procMacro: true}
	sourceDepTag        = dependencyTag{name: "source"}
	dataLibDepTag       = dependencyTag{name: "data lib"}
	dataBinDepTag       = dependencyTag{name: "data bin"}
)

func IsDylibDepTag(depTag blueprint.DependencyTag) bool {
	tag, ok := depTag.(dependencyTag)
	return ok && tag == dylibDepTag
}

func IsRlibDepTag(depTag blueprint.DependencyTag) bool {
	tag, ok := depTag.(dependencyTag)
	return ok && tag == rlibDepTag
}

type autoDep struct {
	variation string
	depTag    dependencyTag
}

var (
	sourceVariation = "source"
	rlibVariation   = "rlib"
	dylibVariation  = "dylib"
	rlibAutoDep     = autoDep{variation: rlibVariation, depTag: rlibDepTag}
	dylibAutoDep    = autoDep{variation: dylibVariation, depTag: dylibDepTag}
)

type autoDeppable interface {
	autoDep(ctx android.BottomUpMutatorContext) autoDep
}

func (mod *Module) begin(ctx BaseModuleContext) {
	if mod.coverage != nil {
		mod.coverage.begin(ctx, mod.Binary(), mod.testModule)
	}
	if mod.sanitize != nil {
		mod.sanitize.begin(ctx)
	}
	if mod.compiler != nil {
		mod.compiler.begin(ctx)
	}

	if mod.UseSdk() && mod.IsSdkVariant() {
		sdkVersion := ""
		if ctx.Device() {
			sdkVersion = mod.SdkVersion()
		}
		version, err := cc.NativeApiLevelFromUser(ctx, sdkVersion)
		if err != nil {
			ctx.PropertyErrorf("sdk_version", err.Error())
			mod.Properties.Sdk_version = nil
		} else {
			mod.Properties.Sdk_version = StringPtr(version.String())
		}
	}

}

func (mod *Module) Prebuilt() *android.Prebuilt {
	if p, ok := mod.compiler.(rustPrebuilt); ok {
		return p.prebuilt()
	}
	return nil
}

func (mod *Module) Symlinks() []string {
	// TODO update this to return the list of symlinks when Rust supports defining symlinks
	return nil
}

func rustMakeLibName(rustInfo *RustInfo, linkableInfo *cc.LinkableInfo, commonInfo *android.CommonModuleInfo, depName string) string {
	if rustInfo != nil {
		// Use base module name for snapshots when exporting to Makefile.
		if rustInfo.SnapshotInfo != nil {
			baseName := commonInfo.BaseModuleName
			return baseName + rustInfo.SnapshotInfo.SnapshotAndroidMkSuffix + rustInfo.AndroidMkSuffix
		}
	}
	return cc.MakeLibName(nil, linkableInfo, commonInfo, depName)
}

func collectIncludedProtos(mod *Module, rustInfo *RustInfo, linkableInfo *cc.LinkableInfo) {
	if protoMod, ok := mod.sourceProvider.(*protobufDecorator); ok {
		if rustInfo.SourceProviderInfo.ProtobufDecoratorInfo != nil {
			protoMod.additionalCrates = append(protoMod.additionalCrates, linkableInfo.CrateName)
		}
	}
}

func (mod *Module) depsToPaths(ctx android.ModuleContext) (PathDeps, []string) {
	var depPaths PathDeps

	directRlibDeps := []*cc.LinkableInfo{}
	directDylibDeps := []*cc.LinkableInfo{}
	directProcMacroDeps := []*cc.LinkableInfo{}
	directSharedLibDeps := []cc.SharedLibraryInfo{}
	directStaticLibDeps := [](*cc.LinkableInfo){}
	directSrcProvidersDeps := []*android.ModuleProxy{}
	directSrcDeps := []android.SourceFilesInfo{}

	// For the dependency from platform to apex, use the latest stubs
	mod.apexSdkVersion = android.FutureApiLevel
	apexInfo, _ := android.ModuleProvider(ctx, android.ApexInfoProvider)
	if !apexInfo.IsForPlatform() {
		mod.apexSdkVersion = apexInfo.MinSdkVersion
	}

	if android.InList("hwaddress", ctx.Config().SanitizeDevice()) {
		// In hwasan build, we override apexSdkVersion to the FutureApiLevel(10000)
		// so that even Q(29/Android10) apexes could use the dynamic unwinder by linking the newer stubs(e.g libc(R+)).
		// (b/144430859)
		mod.apexSdkVersion = android.FutureApiLevel
	}

	skipModuleList := map[string]bool{}

	var transitiveAndroidMkSharedLibs []depset.DepSet[string]
	var directAndroidMkSharedLibs []string

	ctx.VisitDirectDepsProxy(func(dep android.ModuleProxy) {
		depName := ctx.OtherModuleName(dep)
		depTag := ctx.OtherModuleDependencyTag(dep)
		modStdLinkage := mod.compiler.stdLinkage(ctx.Device())

		if _, exists := skipModuleList[depName]; exists {
			return
		}

		if depTag == android.DarwinUniversalVariantTag {
			return
		}

		rustInfo, hasRustInfo := android.OtherModuleProvider(ctx, dep, RustInfoProvider)
		ccInfo, _ := android.OtherModuleProvider(ctx, dep, cc.CcInfoProvider)
		linkableInfo, hasLinkableInfo := android.OtherModuleProvider(ctx, dep, cc.LinkableInfoProvider)
		commonInfo := android.OtherModulePointerProviderOrDefault(ctx, dep, android.CommonModuleInfoProvider)
		if hasRustInfo && !linkableInfo.Static && !linkableInfo.Shared {
			//Handle Rust Modules
			makeLibName := rustMakeLibName(rustInfo, linkableInfo, commonInfo, depName+rustInfo.RustSubName)

			switch {
			case depTag == dylibDepTag:
				dylib := rustInfo.CompilerInfo.LibraryInfo
				if dylib == nil || !dylib.Dylib {
					ctx.ModuleErrorf("mod %q not an dylib library", depName)
					return
				}
				directDylibDeps = append(directDylibDeps, linkableInfo)
				mod.Properties.AndroidMkDylibs = append(mod.Properties.AndroidMkDylibs, makeLibName)
				mod.Properties.SnapshotDylibs = append(mod.Properties.SnapshotDylibs, cc.BaseLibName(depName))

				depPaths.directApexImplementationDeps = append(depPaths.directApexImplementationDeps, android.OutputFileForModule(ctx, dep, ""))
				if info, ok := android.OtherModuleProvider(ctx, dep, cc.ImplementationDepInfoProvider); ok {
					depPaths.transitiveApexImplementationDeps = append(depPaths.transitiveApexImplementationDeps, info.ImplementationDeps)
				}
				if info, ok := android.OtherModuleProvider(ctx, dep, RustImplementationDepInfoProvider); ok {
					depPaths.transitiveNonApexImplementationDeps = append(depPaths.transitiveNonApexImplementationDeps, info.NonApexImplementationDeps)
				}

				if !rustInfo.CompilerInfo.NoStdlibs {
					rustDepStdLinkage := rustInfo.CompilerInfo.StdLinkageForNonDevice
					if ctx.Device() {
						rustDepStdLinkage = rustInfo.CompilerInfo.StdLinkageForDevice
					}
					if !slices.Contains(modStdLinkage.compatChoices(), rustDepStdLinkage) {
						ctx.ModuleErrorf("Rust dependency %q has the wrong StdLinkage; expected %#v, got %#v", depName, modStdLinkage, rustDepStdLinkage)
						return
					}
				}

			case depTag == rlibDepTag:
				rlib := rustInfo.CompilerInfo.LibraryInfo
				if rlib == nil || !rlib.Rlib {
					ctx.ModuleErrorf("mod %q not an rlib library", makeLibName)
					return
				}
				directRlibDeps = append(directRlibDeps, linkableInfo)
				mod.Properties.AndroidMkRlibs = append(mod.Properties.AndroidMkRlibs, makeLibName)
				mod.Properties.SnapshotRlibs = append(mod.Properties.SnapshotRlibs, cc.BaseLibName(depName))

				// rust_ffi rlibs may export include dirs, so collect those here.
				exportedInfo, _ := android.OtherModuleProvider(ctx, dep, cc.FlagExporterInfoProvider)
				depPaths.depIncludePaths = append(depPaths.depIncludePaths, exportedInfo.IncludeDirs...)
				depPaths.exportedLinkDirs = append(depPaths.exportedLinkDirs, linkPathFromFilePath(linkableInfo.OutputFile.Path()))
				depPaths.exportedLinkDirsDeps = append(depPaths.exportedLinkDirsDeps, linkableInfo.OutputFile.Path())

				// rlibs are not installed, so don't add the output file to apexDirectImplementationDeps. Track them for RBE however.
				depPaths.directNonApexImplementationDeps = append(depPaths.directNonApexImplementationDeps, android.OutputFileForModule(ctx, dep, ""))
				if info, ok := android.OtherModuleProvider(ctx, dep, cc.ImplementationDepInfoProvider); ok {
					depPaths.transitiveApexImplementationDeps = append(depPaths.transitiveApexImplementationDeps, info.ImplementationDeps)
				}
				if info, ok := android.OtherModuleProvider(ctx, dep, RustImplementationDepInfoProvider); ok {
					depPaths.transitiveNonApexImplementationDeps = append(depPaths.transitiveNonApexImplementationDeps, info.NonApexImplementationDeps)
				}

				if !rustInfo.CompilerInfo.NoStdlibs {
					rustDepStdLinkage := rustInfo.CompilerInfo.StdLinkageForNonDevice
					if ctx.Device() {
						rustDepStdLinkage = rustInfo.CompilerInfo.StdLinkageForDevice
					}
					if !slices.Contains(modStdLinkage.compatChoices(), rustDepStdLinkage) {
						ctx.ModuleErrorf("Rust dependency %q has the wrong StdLinkage; expected %#v, got %#v", depName, modStdLinkage, rustDepStdLinkage)
						return
					}
				}

				if !mod.Rlib() {
					depPaths.ccRlibDeps = append(depPaths.ccRlibDeps, exportedInfo.RustRlibDeps...)
				} else {
					// rlibs need to reexport these
					depPaths.reexportedCcRlibDeps = append(depPaths.reexportedCcRlibDeps, exportedInfo.RustRlibDeps...)
				}

			case depTag == procMacroDepTag:
				directProcMacroDeps = append(directProcMacroDeps, linkableInfo)
				mod.Properties.AndroidMkProcMacroLibs = append(mod.Properties.AndroidMkProcMacroLibs, makeLibName)
				// proc_macro link dirs need to be exported, so collect those here.
				depPaths.exportedLinkDirs = append(depPaths.exportedLinkDirs, linkPathFromFilePath(linkableInfo.OutputFile.Path()))
				depPaths.exportedLinkDirsDeps = append(depPaths.exportedLinkDirsDeps, linkableInfo.OutputFile.Path())

				depPaths.directNonApexImplementationDeps = append(depPaths.directNonApexImplementationDeps, android.OutputFileForModule(ctx, dep, ""))
				if info, ok := android.OtherModuleProvider(ctx, dep, RustImplementationDepInfoProvider); ok {
					depPaths.transitiveNonApexImplementationDeps = append(depPaths.transitiveNonApexImplementationDeps, info.NonApexImplementationDeps)
				}

			case depTag == sourceDepTag:
				if _, ok := mod.sourceProvider.(*protobufDecorator); ok {
					collectIncludedProtos(mod, rustInfo, linkableInfo)
				}
			case cc.IsStaticDepTag(depTag):
				// Rust FFI rlibs should not be declared in a Rust modules
				// "static_libs" list as we can't handle them properly at the
				// moment (for example, they only produce an rlib-std variant).
				// Instead, a normal rust_library variant should be used.
				ctx.PropertyErrorf("static_libs",
					"found '%s' in static_libs; use a rust_library module in rustlibs instead of a rust_ffi module in static_libs",
					depName)

			}

			transitiveAndroidMkSharedLibs = append(transitiveAndroidMkSharedLibs, rustInfo.TransitiveAndroidMkSharedLibs)

			if android.IsSourceDepTagWithOutputTag(depTag, "") {
				// Since these deps are added in path_properties.go via AddDependencies, we need to ensure the correct
				// OS/Arch variant is used.
				var helper string
				if ctx.Host() {
					helper = "missing 'host_supported'?"
				} else {
					helper = "device module defined?"
				}

				if commonInfo.Target.Os != ctx.Os() {
					ctx.ModuleErrorf("OS mismatch on dependency %q (%s)", dep.Name(), helper)
					return
				} else if commonInfo.Target.Arch.ArchType != ctx.Arch().ArchType {
					ctx.ModuleErrorf("Arch mismatch on dependency %q (%s)", dep.Name(), helper)
					return
				}
				directSrcProvidersDeps = append(directSrcProvidersDeps, &dep)
			}

			exportedRustInfo, _ := android.OtherModuleProvider(ctx, dep, RustFlagExporterInfoProvider)
			exportedInfo, _ := android.OtherModuleProvider(ctx, dep, RustFlagExporterInfoProvider)
			//Append the dependencies exported objects, except for proc-macros which target a different arch/OS
			if depTag != procMacroDepTag {
				depPaths.depFlags = append(depPaths.depFlags, exportedInfo.Flags...)
				depPaths.rustLibObjects = append(depPaths.rustLibObjects, exportedInfo.RustLibObjects...)
				depPaths.sharedLibObjects = append(depPaths.sharedLibObjects, exportedInfo.SharedLibPaths...)
				depPaths.staticLibObjects = append(depPaths.staticLibObjects, exportedInfo.StaticLibObjects...)
				depPaths.wholeStaticLibObjects = append(depPaths.wholeStaticLibObjects, exportedInfo.WholeStaticLibObjects...)
				depPaths.linkDirs = append(depPaths.linkDirs, exportedInfo.LinkDirs...)
				depPaths.linkDirsDeps = append(depPaths.linkDirsDeps, exportedInfo.LinkDirsDeps...)

				depPaths.reexportedWholeCcRlibDeps = append(depPaths.reexportedWholeCcRlibDeps, exportedRustInfo.WholeRustRlibDeps...)
				if !mod.Rlib() {
					depPaths.ccRlibDeps = append(depPaths.ccRlibDeps, exportedRustInfo.WholeRustRlibDeps...)
				}
			}

			if depTag == dylibDepTag || depTag == rlibDepTag || depTag == procMacroDepTag {
				linkFile := linkableInfo.UnstrippedOutputFile
				linkDir := linkPathFromFilePath(linkFile)
				if lib, ok := mod.compiler.(exportedFlagsProducer); ok {
					lib.exportLinkDirs([]string{linkDir}, []android.Path{linkFile})
				}
			}

			if depTag == sourceDepTag {
				if _, ok := mod.sourceProvider.(*protobufDecorator); ok && mod.Source() {
					if rustInfo.SourceProviderInfo.ProtobufDecoratorInfo != nil {
						exportedInfo, _ := android.OtherModuleProvider(ctx, dep, cc.FlagExporterInfoProvider)
						depPaths.depIncludePaths = append(depPaths.depIncludePaths, exportedInfo.IncludeDirs...)
					}
				}
			}
		} else if hasLinkableInfo {
			//Handle C dependencies
			makeLibName := cc.MakeLibName(ccInfo, linkableInfo, commonInfo, depName)
			if !hasRustInfo {
				if commonInfo.Target.Os != ctx.Os() {
					ctx.ModuleErrorf("OS mismatch between %q (%s) and %q (%s)",
						ctx.ModuleName(), ctx.Os().Name, depName, commonInfo.Target.Os.Name)
					return
				}
				if commonInfo.Target.Arch.ArchType != ctx.Arch().ArchType {
					ctx.ModuleErrorf("Arch mismatch between %q(%v) and %q(%v)",
						ctx.ModuleName(), ctx.Arch().ArchType, depName, commonInfo.Target.Arch.ArchType)
					return
				}
			}
			ccLibPath := linkableInfo.OutputFile
			if !ccLibPath.Valid() {
				if !ctx.Config().AllowMissingDependencies() {
					ctx.ModuleErrorf("Invalid output file when adding dep %q to %q", depName, ctx.ModuleName())
				} else {
					ctx.AddMissingDependencies([]string{depName})
				}
				return
			}

			linkPath := linkPathFromFilePath(ccLibPath.Path())

			exportDep := false
			switch {
			case cc.IsStaticDepTag(depTag):
				if cc.IsWholeStaticLib(depTag) {
					// rustc will bundle static libraries when they're passed with "-lstatic=<lib>". This will fail
					// if the library is not prefixed by "lib".
					if mod.Binary() {
						// Since binaries don't need to 'rebundle' these like libraries and only use these for the
						// final linkage, pass the args directly to the linker to handle these cases.
						depPaths.depLinkFlags = append(depPaths.depLinkFlags, []string{"-Wl,--whole-archive", ccLibPath.Path().String(), "-Wl,--no-whole-archive"}...)
					} else if libName, ok := libNameFromFilePath(ccLibPath.Path()); ok {
						depPaths.depFlags = append(depPaths.depFlags, "-lstatic:+whole-archive="+libName)
						depPaths.depLinkFlags = append(depPaths.depLinkFlags, ccLibPath.Path().String())
					} else {
						ctx.ModuleErrorf("'%q' cannot be listed as a whole_static_library in Rust modules unless the output is prefixed by 'lib'", depName, ctx.ModuleName())
					}
				}

				exportedInfo, _ := android.OtherModuleProvider(ctx, dep, cc.FlagExporterInfoProvider)
				if cc.IsWholeStaticLib(depTag) {
					// Add whole staticlibs to wholeStaticLibObjects to propagate to Rust all dependents.
					depPaths.wholeStaticLibObjects = append(depPaths.wholeStaticLibObjects, ccLibPath.String())

					// We also propagate forward whole-static'd cc staticlibs with rust_ffi_rlib dependencies
					// We don't need to check a hypothetical exportedRustInfo.WholeRustRlibDeps because we
					// wouldn't expect a rust_ffi_rlib to be listed in `static_libs` (Soong explicitly disallows this)
					depPaths.reexportedWholeCcRlibDeps = append(depPaths.reexportedWholeCcRlibDeps, exportedInfo.RustRlibDeps...)
				} else {
					// If not whole_static, add to staticLibObjects, which only propagate through rlibs to their dependents.
					depPaths.staticLibObjects = append(depPaths.staticLibObjects, ccLibPath.String())

					if mod.Rlib() {
						// rlibs propagate their inherited rust_ffi_rlibs forward.
						depPaths.reexportedCcRlibDeps = append(depPaths.reexportedCcRlibDeps, exportedInfo.RustRlibDeps...)
					}
				}

				depPaths.linkDirs = append(depPaths.linkDirs, linkPath)
				depPaths.linkDirsDeps = append(depPaths.linkDirsDeps, ccLibPath.Path())
				depPaths.depIncludePaths = append(depPaths.depIncludePaths, exportedInfo.IncludeDirs...)
				depPaths.depSystemIncludePaths = append(depPaths.depSystemIncludePaths, exportedInfo.SystemIncludeDirs...)
				depPaths.depClangFlags = append(depPaths.depClangFlags, exportedInfo.Flags...)
				depPaths.depGeneratedHeaders = append(depPaths.depGeneratedHeaders, exportedInfo.GeneratedHeaders...)

				if !mod.Rlib() {
					// rlibs don't need to build the generated static library, so they don't need to track these.
					depPaths.ccRlibDeps = append(depPaths.ccRlibDeps, exportedInfo.RustRlibDeps...)
				}

				directStaticLibDeps = append(directStaticLibDeps, linkableInfo)

				// Record baseLibName for snapshots.
				mod.Properties.SnapshotStaticLibs = append(mod.Properties.SnapshotStaticLibs, cc.BaseLibName(depName))

				mod.Properties.AndroidMkStaticLibs = append(mod.Properties.AndroidMkStaticLibs, makeLibName)
			case cc.IsSharedDepTag(depTag):
				// For the shared lib dependencies, we may link to the stub variant
				// of the dependency depending on the context (e.g. if this
				// dependency crosses the APEX boundaries).
				sharedLibraryInfo, exportedInfo := cc.ChooseStubOrImpl(ctx, dep)

				if !sharedLibraryInfo.IsStubs {
					// TODO(b/362509506): remove this additional check once all apex_exclude uses are switched to stubs.
					if !linkableInfo.RustApexExclude {
						depPaths.directApexImplementationDeps = append(depPaths.directApexImplementationDeps, android.OutputFileForModule(ctx, dep, ""))
						if info, ok := android.OtherModuleProvider(ctx, dep, cc.ImplementationDepInfoProvider); ok {
							depPaths.transitiveApexImplementationDeps = append(depPaths.transitiveApexImplementationDeps, info.ImplementationDeps)
						}
					}
				}

				// Re-get linkObject as ChooseStubOrImpl actually tells us which
				// object (either from stub or non-stub) to use.
				ccLibPath = android.OptionalPathForPath(sharedLibraryInfo.SharedLibrary)
				if !ccLibPath.Valid() {
					if !ctx.Config().AllowMissingDependencies() {
						ctx.ModuleErrorf("Invalid output file when adding dep %q to %q", depName, ctx.ModuleName())
					} else {
						ctx.AddMissingDependencies([]string{depName})
					}
					return
				}
				linkPath = linkPathFromFilePath(ccLibPath.Path())

				depPaths.linkDirs = append(depPaths.linkDirs, linkPath)
				depPaths.linkDirsDeps = append(depPaths.linkDirsDeps, ccLibPath.Path())
				depPaths.sharedLibObjects = append(depPaths.sharedLibObjects, ccLibPath.String())
				depPaths.depIncludePaths = append(depPaths.depIncludePaths, exportedInfo.IncludeDirs...)
				depPaths.depSystemIncludePaths = append(depPaths.depSystemIncludePaths, exportedInfo.SystemIncludeDirs...)
				depPaths.depClangFlags = append(depPaths.depClangFlags, exportedInfo.Flags...)
				depPaths.depGeneratedHeaders = append(depPaths.depGeneratedHeaders, exportedInfo.GeneratedHeaders...)
				directSharedLibDeps = append(directSharedLibDeps, sharedLibraryInfo)

				// Record baseLibName for snapshots.
				mod.Properties.SnapshotSharedLibs = append(mod.Properties.SnapshotSharedLibs, cc.BaseLibName(depName))

				directAndroidMkSharedLibs = append(directAndroidMkSharedLibs, makeLibName)
				exportDep = true
			case cc.IsHeaderDepTag(depTag):
				exportedInfo, _ := android.OtherModuleProvider(ctx, dep, cc.FlagExporterInfoProvider)
				depPaths.depIncludePaths = append(depPaths.depIncludePaths, exportedInfo.IncludeDirs...)
				depPaths.depSystemIncludePaths = append(depPaths.depSystemIncludePaths, exportedInfo.SystemIncludeDirs...)
				depPaths.depGeneratedHeaders = append(depPaths.depGeneratedHeaders, exportedInfo.GeneratedHeaders...)
				mod.Properties.AndroidMkHeaderLibs = append(mod.Properties.AndroidMkHeaderLibs, makeLibName)
			case depTag == cc.CrtBeginDepTag:
				depPaths.CrtBegin = append(depPaths.CrtBegin, ccLibPath.Path())
			case depTag == cc.CrtEndDepTag:
				depPaths.CrtEnd = append(depPaths.CrtEnd, ccLibPath.Path())
			}

			// Make sure shared dependencies are propagated
			if lib, ok := mod.compiler.(exportedFlagsProducer); ok && exportDep {
				lib.exportLinkDirs([]string{linkPath}, []android.Path{ccLibPath.Path()})
				lib.exportSharedLibs(ccLibPath.String())
			}
		} else {
			switch depTag {
			case cc.CrtBeginDepTag:
				depPaths.CrtBegin = append(depPaths.CrtBegin, android.OutputFileForModule(ctx, dep, ""))
				depPaths.directNonApexImplementationDeps = append(depPaths.directNonApexImplementationDeps, android.OutputFileForModule(ctx, dep, ""))
			case cc.CrtEndDepTag:
				depPaths.directNonApexImplementationDeps = append(depPaths.directNonApexImplementationDeps, android.OutputFileForModule(ctx, dep, ""))
				depPaths.CrtEnd = append(depPaths.CrtEnd, android.OutputFileForModule(ctx, dep, ""))
			}
		}

		if srcDep := commonInfo.SourceFiles; srcDep != nil {
			if android.IsSourceDepTagWithOutputTag(depTag, "") {
				// These are usually genrules which don't have per-target variants.
				directSrcDeps = append(directSrcDeps, *srcDep)
			}
		}
	})

	mod.transitiveAndroidMkSharedLibs = depset.New(depset.PREORDER, directAndroidMkSharedLibs, transitiveAndroidMkSharedLibs)

	var rlibDepFiles RustLibraries
	aliases := mod.compiler.Aliases()
	for _, dep := range directRlibDeps {
		crateName := dep.CrateName
		if alias, aliased := aliases[crateName]; aliased {
			crateName = alias
		}
		rlibDepFiles = append(rlibDepFiles, RustLibrary{Path: dep.UnstrippedOutputFile, CrateName: crateName})
	}
	var dylibDepFiles RustLibraries
	for _, dep := range directDylibDeps {
		crateName := dep.CrateName
		if alias, aliased := aliases[crateName]; aliased {
			crateName = alias
		}
		dylibDepFiles = append(dylibDepFiles, RustLibrary{Path: dep.UnstrippedOutputFile, CrateName: crateName})
	}
	var procMacroDepFiles RustLibraries
	for _, dep := range directProcMacroDeps {
		crateName := dep.CrateName
		if alias, aliased := aliases[crateName]; aliased {
			crateName = alias
		}
		procMacroDepFiles = append(procMacroDepFiles, RustLibrary{Path: dep.UnstrippedOutputFile, CrateName: crateName})
	}

	var staticLibDepFiles android.Paths
	for _, dep := range directStaticLibDeps {
		staticLibDepFiles = append(staticLibDepFiles, dep.OutputFile.Path())
	}

	var sharedLibFiles android.Paths
	var sharedLibDepFiles android.Paths
	for _, dep := range directSharedLibDeps {
		sharedLibFiles = append(sharedLibFiles, dep.SharedLibrary)
		if dep.TableOfContents.Valid() {
			sharedLibDepFiles = append(sharedLibDepFiles, dep.TableOfContents.Path())
		} else {
			sharedLibDepFiles = append(sharedLibDepFiles, dep.SharedLibrary)
		}
	}

	var srcProviderDepFiles android.Paths
	for _, dep := range directSrcProvidersDeps {
		srcs := android.OutputFilesForModule(ctx, *dep, "")
		srcProviderDepFiles = append(srcProviderDepFiles, srcs...)
	}
	for _, dep := range directSrcDeps {
		srcs := dep.Srcs
		outDir := android.PathForOutput(ctx).String() + string(os.PathSeparator)
		for _, srcFile := range srcs {
			if strings.HasPrefix(srcFile.String(), outDir) {
				// Only append generated files from genrules and other source file
				// producers. This is to ensure filegroups aren't included in this
				// list and later copied
				srcProviderDepFiles = append(srcProviderDepFiles, srcFile)
			}
		}
	}

	depPaths.RLibs = append(depPaths.RLibs, rlibDepFiles...)
	depPaths.DyLibs = append(depPaths.DyLibs, dylibDepFiles...)
	depPaths.SharedLibs = append(depPaths.SharedLibs, sharedLibFiles...)
	depPaths.SharedLibDeps = append(depPaths.SharedLibDeps, sharedLibDepFiles...)
	depPaths.StaticLibs = append(depPaths.StaticLibs, staticLibDepFiles...)
	depPaths.ProcMacros = append(depPaths.ProcMacros, procMacroDepFiles...)
	depPaths.SrcDeps = append(depPaths.SrcDeps, srcProviderDepFiles...)

	// Dedup exported flags from dependencies
	depPaths.linkDirs = android.FirstUniqueStrings(depPaths.linkDirs)
	depPaths.linkDirsDeps = android.FirstUniquePaths(depPaths.linkDirsDeps)
	depPaths.rustLibObjects = android.FirstUniqueStrings(depPaths.rustLibObjects)
	depPaths.staticLibObjects = android.FirstUniqueStrings(depPaths.staticLibObjects)
	depPaths.wholeStaticLibObjects = android.FirstUniqueStrings(depPaths.wholeStaticLibObjects)
	depPaths.sharedLibObjects = android.FirstUniqueStrings(depPaths.sharedLibObjects)
	depPaths.depFlags = android.FirstUniqueStrings(depPaths.depFlags)
	depPaths.depClangFlags = android.FirstUniqueStrings(depPaths.depClangFlags)
	depPaths.depIncludePaths = android.FirstUniquePaths(depPaths.depIncludePaths)
	depPaths.depSystemIncludePaths = android.FirstUniquePaths(depPaths.depSystemIncludePaths)
	depPaths.depLinkFlags = android.FirstUniqueStrings(depPaths.depLinkFlags)
	depPaths.reexportedCcRlibDeps = android.FirstUniqueFunc(depPaths.reexportedCcRlibDeps, cc.EqRustRlibDeps)
	depPaths.reexportedWholeCcRlibDeps = android.FirstUniqueFunc(depPaths.reexportedWholeCcRlibDeps, cc.EqRustRlibDeps)
	depPaths.ccRlibDeps = android.FirstUniqueFunc(depPaths.ccRlibDeps, cc.EqRustRlibDeps)

	makeNames := append(mod.transitiveAndroidMkSharedLibs.ToList(), mod.Properties.AndroidMkDylibs...)
	return depPaths, makeNames
}

func (mod *Module) InstallInData() bool {
	if mod.compiler == nil {
		return false
	}
	return mod.compiler.inData()
}

func (mod *Module) InstallInRamdisk() bool {
	return mod.InRamdisk()
}

func (mod *Module) InstallInVendorRamdisk() bool {
	return mod.InVendorRamdisk()
}

func (mod *Module) InstallInRecovery() bool {
	return mod.InRecovery()
}

func linkPathFromFilePath(filepath android.Path) string {
	return strings.Split(filepath.String(), filepath.Base())[0]
}

func (mod *Module) StdLinkageIsRlibLinkage(device bool) bool {
	if mod.compiler != nil {
		switch mod.compiler.stdLinkage(device) {
		case NoCore, RlibCore, RlibStd:
			return true
		}
	}
	return false
}

// Go remains uncivilized for not having this as a default method on their slice.
func sliceMap[E1 any, E2 any](base []E1, f func(E1) E2) []E2 {
	out := make([]E2, len(base))
	for i, v := range base {
		out[i] = f(v)
	}
	return out
}

func (linkage StdLinkage) compatChoices() []StdLinkage {
	switch linkage {
	case DylibStd:
		// dylib-std should only takes its own linkage, but there are cases in the build
		// today that are depending on no-std modules.
		// TODO migrate so that dylib-std only pulls in dylib-std
		return []StdLinkage{linkage, RlibCore}
	case RlibStd:
		// rlib-std can also accept rlib-core libraries, but should prefer std libraries.
		return []StdLinkage{linkage, RlibCore}
	case RlibCore:
		// rlib-core should only support its own linkage, but there are a variety of cases
		// in the build today where a no-std build is depending on a std build, so we need
		// to allow it, at least for now.
		// TODO migrate existing builds to not have rlib-core libraries depend on rlib-std
		return []StdLinkage{linkage, RlibStd}
	case NoCore:
		// Sysroots can only accept other sysroot (e.g. non-mutated) libraries
		return []StdLinkage{linkage}
	}
	panic(fmt.Errorf("unrecognized linkage %v", linkage))
}

func (mod *Module) stdLinkageOptions(ctx DepsContext) [][]blueprint.Variation {
	stdLinkage := mod.compiler.stdLinkage(ctx.Device())
	switch stdLinkage {
	case NoCore:
		// NoCore is currently unmutated, so there's no variation here
		return [][]blueprint.Variation{{}}
	default:
		return sliceMap(stdLinkage.compatChoices(), func(choice StdLinkage) []blueprint.Variation { return []blueprint.Variation{choice.variation()} })
	}
}

func (mod *Module) addNoStdDep(ctx DepsContext, lib string) {
	variations := []blueprint.Variation{RlibCore.variation(), rlibDepTag.libraryVariation()}

	if ctx.OtherModuleDependencyVariantExists(variations, lib) {
		ctx.AddVariationDependencies(variations, rlibDepTag, lib)
		return
	}
	// To allow migration of custom nostd modules to no_std variants, temporarily support
	// stripping _nostd suffixes if the library is not present.
	// This is made safe by the enforcement that no_std libraries can no longer depend on
	// stdful libraries, which works transitively.
	if strings.HasSuffix(lib, "_nostd") {
		mod.addNoStdDep(ctx, strings.TrimSuffix(lib, "_nostd"))
		return
	}
	if !ctx.Config().AllowMissingDependencies() {
		ctx.ModuleErrorf("unable to find no_std variation for lib %#v", lib)
	}
}

func (mod *Module) addVariantDep(ctx DepsContext, depTags []dependencyTag, lib string) {
	// Preference order is to get the preferred depTag, then to get preferred stdLinkage.
	for _, depTag := range depTags {
		for _, stdLinkage := range mod.stdLinkageOptions(ctx) {
			variations := append(stdLinkage, depTag.libraryVariation())
			// If the stdlinkage + depTag choice exists, select it and return
			if ctx.OtherModuleDependencyVariantExists(variations, lib) {
				ctx.AddVariationDependencies(variations, depTag, lib)
				return
			}
		}
	}
	// To allow migration of custom nostd modules to no_std variants, temporarily support
	// stripping _nostd suffixes if the library is not present.
	// This is made safe by the enforcement that no_std libraries can no longer depend on
	// stdful libraries, which works transitively.
	if strings.HasSuffix(lib, "_nostd") {
		mod.addVariantDep(ctx, depTags, strings.TrimSuffix(lib, "_nostd"))
		return
	}
	if !ctx.Config().AllowMissingDependencies() {
		// Intentionally add a missing dependency to trigger a more user-friendly error
		depTag := depTags[0]
		stdLinkage := mod.stdLinkageOptions(ctx)[0]
		variations := append(stdLinkage, depTag.libraryVariation())
		ctx.AddVariationDependencies(variations, depTag, lib)
	}
}

func (mod *Module) DepsMutator(actx android.BottomUpMutatorContext) {
	ctx := &depsContext{
		BottomUpMutatorContext: actx,
	}

	deps := mod.deps(ctx)
	var commonDepVariations []blueprint.Variation

	variantNdkLibs := []string{}
	if ctx.Os() == android.Android {
		deps.SharedLibs, variantNdkLibs = cc.FilterNdkLibs(mod, ctx.Config(), deps.SharedLibs)
	}

	// rlibs
	for _, lib := range deps.Rlibs {
		mod.addVariantDep(ctx, []dependencyTag{rlibDepTag}, lib)
	}

	// dylibs
	for _, lib := range deps.Dylibs {
		mod.addVariantDep(ctx, []dependencyTag{dylibDepTag}, lib)
	}

	// rustlibs
	if deps.Rustlibs != nil {
		if !mod.compiler.Disabled() {
			for _, lib := range deps.Rustlibs {
				autoDep := mod.compiler.(autoDeppable).autoDep(ctx)
				switch autoDep.depTag {
				case rlibDepTag:
					mod.addVariantDep(ctx, []dependencyTag{rlibDepTag}, lib)
				case dylibDepTag:
					// autoDep.depTag is a dylib depTag. Not all rustlibs may be available as a dylib however.
					// Check for the existence of the dylib deptag variant. Select it if available,
					// otherwise select the rlib variant.
					mod.addVariantDep(ctx, []dependencyTag{dylibDepTag, rlibDepTag}, lib)
				default:
					panic(fmt.Errorf("unknown depTag: %v", autoDep.depTag))
				}
			}
		} else if _, ok := mod.sourceProvider.(*protobufDecorator); ok {
			for _, lib := range deps.Rustlibs {
				srcProviderVariations := append(commonDepVariations,
					blueprint.Variation{Mutator: "rust_libraries", Variation: sourceVariation})

				// Only add rustlib dependencies if they're source providers themselves.
				// This is used to track which crate names need to be added to the source generated
				// in the rust_protobuf mod.rs.
				if actx.OtherModuleDependencyVariantExists(srcProviderVariations, lib) {
					actx.AddVariationDependencies(srcProviderVariations, sourceDepTag, lib)
				}
			}
		}
	}

	if deps.NoStdRlibs != nil {
		for _, lib := range deps.NoStdRlibs {
			mod.addNoStdDep(ctx, lib)
		}
	}

	// stdlibs
	if deps.Stdlibs != nil {
		stdLinkage := mod.compiler.stdLinkage(ctx.Device())
		for _, lib := range deps.Stdlibs {
			actx.AddVariationDependencies(stdLinkage.libraryVariations(), stdLinkage.depTag(), lib)
		}
	}

	for _, lib := range deps.SharedLibs {
		depTag := cc.SharedDepTag()
		name, version := cc.StubsLibNameAndVersion(lib)

		variations := []blueprint.Variation{
			{Mutator: "link", Variation: "shared"},
		}
		cc.AddSharedLibDependenciesWithVersions(ctx, mod, variations, depTag, name, version, false)
	}

	for _, lib := range deps.WholeStaticLibs {
		depTag := cc.StaticDepTag(true)

		actx.AddVariationDependencies([]blueprint.Variation{
			{Mutator: "link", Variation: "static"},
		}, depTag, lib)
	}

	for _, lib := range deps.StaticLibs {
		depTag := cc.StaticDepTag(false)

		actx.AddVariationDependencies([]blueprint.Variation{
			{Mutator: "link", Variation: "static"},
		}, depTag, lib)
	}

	actx.AddVariationDependencies(nil, cc.HeaderDepTag(), deps.HeaderLibs...)

	crtVariations := cc.GetCrtVariations(ctx, mod)
	for _, crt := range deps.CrtBegin {
		actx.AddVariationDependencies(crtVariations, cc.CrtBeginDepTag, crt)
	}
	for _, crt := range deps.CrtEnd {
		actx.AddVariationDependencies(crtVariations, cc.CrtEndDepTag, crt)
	}

	if mod.sourceProvider != nil {
		if bindgen, ok := mod.sourceProvider.(*bindgenDecorator); ok &&
			bindgen.Properties.Custom_bindgen != "" {
			actx.AddFarVariationDependencies(ctx.Config().BuildOSTarget.Variations(), customBindgenDepTag,
				bindgen.Properties.Custom_bindgen)
		}
	}

	actx.AddVariationDependencies([]blueprint.Variation{
		{Mutator: "link", Variation: "shared"},
	}, dataLibDepTag, deps.DataLibs...)

	version := ""
	if ctx.Device() {
		version = String(mod.Properties.Sdk_version)
	}

	ndkStubDepTag := cc.NdkSharedLibDepTag(version)
	actx.AddVariationDependencies([]blueprint.Variation{
		{Mutator: "version", Variation: version},
		{Mutator: "link", Variation: "shared"},
	}, ndkStubDepTag, variantNdkLibs...)

	actx.AddVariationDependencies(nil, dataBinDepTag, deps.DataBins...)

	// proc_macros are compiler plugins, and so we need the host arch variant as a dependendcy.
	actx.AddFarVariationDependencies(ctx.Config().BuildOSTarget.Variations(), procMacroDepTag, deps.ProcMacros...)

	mod.afdo.addDep(ctx, actx)
}

func BeginMutator(ctx android.BottomUpMutatorContext) {
	if mod, ok := ctx.Module().(*Module); ok && mod.Enabled(ctx) {
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
		return android.OptionalPathForPath(binary.path)
	} else if pm, ok := mod.compiler.(*procMacroDecorator); ok {
		// Even though proc-macros aren't strictly "tools", since they target the compiler
		// and act as compiler plugins, we treat them similarly.
		return android.OptionalPathForPath(pm.path)
	}
	return android.OptionalPath{}
}

var _ android.ApexModule = (*Module)(nil)

// If a module is marked for exclusion from apexes, don't provide apex variants.
// TODO(b/362509506): remove this once all apex_exclude usages are removed.
func (m *Module) CanHaveApexVariants() bool {
	if m.ApexExclude() {
		return false
	} else {
		return m.ApexModuleBase.CanHaveApexVariants()
	}
}

func (mod *Module) MinSdkVersion(ctx android.ConfigurableEvaluatorContext) string {
	return String(mod.Properties.Min_sdk_version)
}

// Implements android.ApexModule
func (mod *Module) MinSdkVersionSupported(ctx android.BaseModuleContext) android.ApiLevel {
	minSdkVersion := mod.MinSdkVersion(ctx)
	if minSdkVersion == "apex_inherit" {
		return android.MinApiLevel
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

	return ver
}

// Implements android.ApexModule
func (mod *Module) AlwaysRequiresPlatformApexVariant() bool {
	// stub libraries and native bridge libraries are always available to platform
	// TODO(b/362509506): remove the ApexExclude() check once all apex_exclude uses are switched to stubs.
	return mod.IsStubs() || mod.Target().NativeBridge == android.NativeBridgeEnabled || mod.ApexExclude()
}

// Implements android.ApexModule
// @auto-generate: gob
type RustDepInSameApexChecker struct {
	Static           bool
	HasStubsVariants bool
	ApexExclude      bool
	Host             bool
}

func (mod *Module) GetDepInSameApexChecker() android.DepInSameApexChecker {
	return RustDepInSameApexChecker{
		Static:           mod.Static(),
		HasStubsVariants: mod.HasStubsVariants(),
		ApexExclude:      mod.ApexExclude(),
		Host:             mod.Host(),
	}
}

func (r RustDepInSameApexChecker) OutgoingDepIsInSameApex(depTag blueprint.DependencyTag) bool {
	if depTag == procMacroDepTag || depTag == customBindgenDepTag {
		return false
	}

	if r.Static && cc.IsSharedDepTag(depTag) {
		// shared_lib dependency from a static lib is considered as crossing
		// the APEX boundary because the dependency doesn't actually is
		// linked; the dependency is used only during the compilation phase.
		return false
	}

	if depTag == cc.StubImplDepTag {
		// We don't track from an implementation library to its stubs.
		return false
	}

	if cc.ExcludeInApexDepTag(depTag) {
		return false
	}

	// TODO(b/362509506): remove once all apex_exclude uses are switched to stubs.
	if r.ApexExclude {
		return false
	}

	return true
}

func (r RustDepInSameApexChecker) IncomingDepIsInSameApex(depTag blueprint.DependencyTag) bool {
	if r.Host {
		return false
	}
	// TODO(b/362509506): remove once all apex_exclude uses are switched to stubs.
	if r.ApexExclude {
		return false
	}

	if r.HasStubsVariants {
		if cc.IsSharedDepTag(depTag) && !cc.IsExplicitImplSharedDepTag(depTag) {
			// dynamic dep to a stubs lib crosses APEX boundary
			return false
		}
		if cc.IsRuntimeDepTag(depTag) {
			// runtime dep to a stubs lib also crosses APEX boundary
			return false
		}
		if cc.IsHeaderDepTag(depTag) {
			return false
		}
	}
	return true
}

// Overrides ApexModule.IsInstallabeToApex()
func (mod *Module) IsInstallableToApex() bool {
	// TODO(b/362509506): remove once all apex_exclude uses are switched to stubs.
	if mod.ApexExclude() {
		return false
	}

	if mod.compiler != nil {
		if lib, ok := mod.compiler.(libraryInterface); ok {
			return (lib.shared() || lib.dylib()) && !lib.BuildStubs()
		}
		if _, ok := mod.compiler.(*binaryDecorator); ok {
			return true
		}
	}
	return false
}

// If a library file has a "lib" prefix, extract the library name without the prefix.
func libNameFromFilePath(filepath android.Path) (string, bool) {
	libName := strings.TrimSuffix(filepath.Base(), filepath.Ext())
	if strings.HasPrefix(libName, "lib") {
		libName = libName[3:]
		return libName, true
	}
	return "", false
}

func (c *Module) Partition() string {
	return ""
}

// @auto-generate: gob
type RustImplementationDepInfo struct {
	NonApexImplementationDeps depset.DepSet[android.Path]
}

func (linkage StdLinkage) libraryVariations() []blueprint.Variation {
	switch linkage {
	case RlibCore, RlibStd:
		return []blueprint.Variation{{Mutator: "rust_libraries", Variation: rlibVariation}}
	case DylibStd:
		return []blueprint.Variation{{Mutator: "rust_libraries", Variation: dylibVariation}}
	case NoCore:
		panic("trying to link stdlibs for no-core library")
	default:
		panic(fmt.Errorf("unknown linkage: %v", linkage))
	}
}

func (linkage StdLinkage) depTag() dependencyTag {
	switch linkage {
	case RlibCore, RlibStd:
		return rlibDepTag
	case DylibStd:
		return dylibDepTag
	default:
		panic(fmt.Errorf("unknown linkage: %v", linkage))
	}
}

func (depTag dependencyTag) libraryVariation() blueprint.Variation {
	switch depTag {
	case rlibDepTag:
		return blueprint.Variation{Mutator: "rust_libraries", Variation: rlibVariation}
	case dylibDepTag:
		return blueprint.Variation{Mutator: "rust_libraries", Variation: dylibVariation}
	default:
		panic(fmt.Errorf("creating variation for unknown depTag: %v", depTag))
	}
}

var RustImplementationDepInfoProvider = blueprint.NewProvider[*RustImplementationDepInfo]()

var Bool = proptools.Bool
var BoolDefault = proptools.BoolDefault
var String = proptools.String
var StringPtr = proptools.StringPtr
