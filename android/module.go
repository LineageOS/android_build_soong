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

package android

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"

	"android/soong/bazel"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

var (
	DeviceSharedLibrary = "shared_library"
	DeviceStaticLibrary = "static_library"
	jarJarPrefixHandler func(ctx ModuleContext)
)

type Module interface {
	blueprint.Module

	// GenerateAndroidBuildActions is analogous to Blueprints' GenerateBuildActions,
	// but GenerateAndroidBuildActions also has access to Android-specific information.
	// For more information, see Module.GenerateBuildActions within Blueprint's module_ctx.go
	GenerateAndroidBuildActions(ModuleContext)

	// Add dependencies to the components of a module, i.e. modules that are created
	// by the module and which are considered to be part of the creating module.
	//
	// This is called before prebuilts are renamed so as to allow a dependency to be
	// added directly to a prebuilt child module instead of depending on a source module
	// and relying on prebuilt processing to switch to the prebuilt module if preferred.
	//
	// A dependency on a prebuilt must include the "prebuilt_" prefix.
	ComponentDepsMutator(ctx BottomUpMutatorContext)

	DepsMutator(BottomUpMutatorContext)

	base() *ModuleBase
	Disable()
	Enabled(ctx ConfigAndErrorContext) bool
	Target() Target
	MultiTargets() []Target

	// ImageVariation returns the image variation of this module.
	//
	// The returned structure has its Mutator field set to "image" and its Variation field set to the
	// image variation, e.g. recovery, ramdisk, etc.. The Variation field is "" for host modules and
	// device modules that have no image variation.
	ImageVariation() blueprint.Variation

	Owner() string
	InstallInData() bool
	InstallInTestcases() bool
	InstallInSanitizerDir() bool
	InstallInRamdisk() bool
	InstallInVendorRamdisk() bool
	InstallInDebugRamdisk() bool
	InstallInRecovery() bool
	InstallInRoot() bool
	InstallInOdm() bool
	InstallInProduct() bool
	InstallInVendor() bool
	InstallForceOS() (*OsType, *ArchType)
	PartitionTag(DeviceConfig) string
	HideFromMake()
	IsHideFromMake() bool
	IsSkipInstall() bool
	MakeUninstallable()
	ReplacedByPrebuilt()
	IsReplacedByPrebuilt() bool
	ExportedToMake() bool
	InitRc() Paths
	VintfFragments() Paths
	EffectiveLicenseKinds() []string
	EffectiveLicenseFiles() Paths

	AddProperties(props ...interface{})
	GetProperties() []interface{}

	BuildParamsForTests() []BuildParams
	RuleParamsForTests() map[blueprint.Rule]blueprint.RuleParams
	VariablesForTests() map[string]string

	// String returns a string that includes the module name and variants for printing during debugging.
	String() string

	// Get the qualified module id for this module.
	qualifiedModuleId(ctx BaseModuleContext) qualifiedModuleName

	// Get information about the properties that can contain visibility rules.
	visibilityProperties() []visibilityProperty

	RequiredModuleNames(ctx ConfigAndErrorContext) []string
	HostRequiredModuleNames() []string
	TargetRequiredModuleNames() []string

	FilesToInstall() InstallPaths
	PackagingSpecs() []PackagingSpec

	// TransitivePackagingSpecs returns the PackagingSpecs for this module and any transitive
	// dependencies with dependency tags for which IsInstallDepNeeded() returns true.
	TransitivePackagingSpecs() []PackagingSpec

	ConfigurableEvaluator(ctx ConfigAndErrorContext) proptools.ConfigurableEvaluator
}

// Qualified id for a module
type qualifiedModuleName struct {
	// The package (i.e. directory) in which the module is defined, without trailing /
	pkg string

	// The name of the module, empty string if package.
	name string
}

func (q qualifiedModuleName) String() string {
	if q.name == "" {
		return "//" + q.pkg
	}
	return "//" + q.pkg + ":" + q.name
}

func (q qualifiedModuleName) isRootPackage() bool {
	return q.pkg == "" && q.name == ""
}

// Get the id for the package containing this module.
func (q qualifiedModuleName) getContainingPackageId() qualifiedModuleName {
	pkg := q.pkg
	if q.name == "" {
		if pkg == "" {
			panic(fmt.Errorf("Cannot get containing package id of root package"))
		}

		index := strings.LastIndex(pkg, "/")
		if index == -1 {
			pkg = ""
		} else {
			pkg = pkg[:index]
		}
	}
	return newPackageId(pkg)
}

func newPackageId(pkg string) qualifiedModuleName {
	// A qualified id for a package module has no name.
	return qualifiedModuleName{pkg: pkg, name: ""}
}

type Dist struct {
	// Copy the output of this module to the $DIST_DIR when `dist` is specified on the
	// command line and any of these targets are also on the command line, or otherwise
	// built
	Targets []string `android:"arch_variant"`

	// The name of the output artifact. This defaults to the basename of the output of
	// the module.
	Dest *string `android:"arch_variant"`

	// The directory within the dist directory to store the artifact. Defaults to the
	// top level directory ("").
	Dir *string `android:"arch_variant"`

	// A suffix to add to the artifact file name (before any extension).
	Suffix *string `android:"arch_variant"`

	// If true, then the artifact file will be appended with _<product name>. For
	// example, if the product is coral and the module is an android_app module
	// of name foo, then the artifact would be foo_coral.apk. If false, there is
	// no change to the artifact file name.
	Append_artifact_with_product *bool `android:"arch_variant"`

	// A string tag to select the OutputFiles associated with the tag.
	//
	// If no tag is specified then it will select the default dist paths provided
	// by the module type. If a tag of "" is specified then it will return the
	// default output files provided by the modules, i.e. the result of calling
	// OutputFiles("").
	Tag *string `android:"arch_variant"`
}

// NamedPath associates a path with a name. e.g. a license text path with a package name
type NamedPath struct {
	Path Path
	Name string
}

// String returns an escaped string representing the `NamedPath`.
func (p NamedPath) String() string {
	if len(p.Name) > 0 {
		return p.Path.String() + ":" + url.QueryEscape(p.Name)
	}
	return p.Path.String()
}

// NamedPaths describes a list of paths each associated with a name.
type NamedPaths []NamedPath

// Strings returns a list of escaped strings representing each `NamedPath` in the list.
func (l NamedPaths) Strings() []string {
	result := make([]string, 0, len(l))
	for _, p := range l {
		result = append(result, p.String())
	}
	return result
}

// SortedUniqueNamedPaths modifies `l` in place to return the sorted unique subset.
func SortedUniqueNamedPaths(l NamedPaths) NamedPaths {
	if len(l) == 0 {
		return l
	}
	sort.Slice(l, func(i, j int) bool {
		return l[i].String() < l[j].String()
	})
	k := 0
	for i := 1; i < len(l); i++ {
		if l[i].String() == l[k].String() {
			continue
		}
		k++
		if k < i {
			l[k] = l[i]
		}
	}
	return l[:k+1]
}

// soongConfigTrace holds all references to VendorVars. Uses []string for blueprint:"mutated"
type soongConfigTrace struct {
	Bools   []string `json:",omitempty"`
	Strings []string `json:",omitempty"`
	IsSets  []string `json:",omitempty"`
}

func (c *soongConfigTrace) isEmpty() bool {
	return len(c.Bools) == 0 && len(c.Strings) == 0 && len(c.IsSets) == 0
}

// Returns hash of serialized trace records (empty string if there's no trace recorded)
func (c *soongConfigTrace) hash() string {
	// Use MD5 for speed. We don't care collision or preimage attack
	if c.isEmpty() {
		return ""
	}
	j, err := json.Marshal(c)
	if err != nil {
		panic(fmt.Errorf("json marshal of %#v failed: %#v", *c, err))
	}
	hash := md5.Sum(j)
	return hex.EncodeToString(hash[:])
}

type nameProperties struct {
	// The name of the module.  Must be unique across all modules.
	Name *string
}

type commonProperties struct {
	// emit build rules for this module
	//
	// Disabling a module should only be done for those modules that cannot be built
	// in the current environment. Modules that can build in the current environment
	// but are not usually required (e.g. superceded by a prebuilt) should not be
	// disabled as that will prevent them from being built by the checkbuild target
	// and so prevent early detection of changes that have broken those modules.
	Enabled proptools.Configurable[bool] `android:"arch_variant,replace_instead_of_append"`

	// Controls the visibility of this module to other modules. Allowable values are one or more of
	// these formats:
	//
	//  ["//visibility:public"]: Anyone can use this module.
	//  ["//visibility:private"]: Only rules in the module's package (not its subpackages) can use
	//      this module.
	//  ["//visibility:override"]: Discards any rules inherited from defaults or a creating module.
	//      Can only be used at the beginning of a list of visibility rules.
	//  ["//some/package:__pkg__", "//other/package:__pkg__"]: Only modules in some/package and
	//      other/package (defined in some/package/*.bp and other/package/*.bp) have access to
	//      this module. Note that sub-packages do not have access to the rule; for example,
	//      //some/package/foo:bar or //other/package/testing:bla wouldn't have access. __pkg__
	//      is a special module and must be used verbatim. It represents all of the modules in the
	//      package.
	//  ["//project:__subpackages__", "//other:__subpackages__"]: Only modules in packages project
	//      or other or in one of their sub-packages have access to this module. For example,
	//      //project:rule, //project/library:lib or //other/testing/internal:munge are allowed
	//      to depend on this rule (but not //independent:evil)
	//  ["//project"]: This is shorthand for ["//project:__pkg__"]
	//  [":__subpackages__"]: This is shorthand for ["//project:__subpackages__"] where
	//      //project is the module's package. e.g. using [":__subpackages__"] in
	//      packages/apps/Settings/Android.bp is equivalent to
	//      //packages/apps/Settings:__subpackages__.
	//  ["//visibility:legacy_public"]: The default visibility, behaves as //visibility:public
	//      for now. It is an error if it is used in a module.
	//
	// If a module does not specify the `visibility` property then it uses the
	// `default_visibility` property of the `package` module in the module's package.
	//
	// If the `default_visibility` property is not set for the module's package then
	// it will use the `default_visibility` of its closest ancestor package for which
	// a `default_visibility` property is specified.
	//
	// If no `default_visibility` property can be found then the module uses the
	// global default of `//visibility:legacy_public`.
	//
	// The `visibility` property has no effect on a defaults module although it does
	// apply to any non-defaults module that uses it. To set the visibility of a
	// defaults module, use the `defaults_visibility` property on the defaults module;
	// not to be confused with the `default_visibility` property on the package module.
	//
	// See https://android.googlesource.com/platform/build/soong/+/main/README.md#visibility for
	// more details.
	Visibility []string

	// Describes the licenses applicable to this module. Must reference license modules.
	Licenses []string

	// Flattened from direct license dependencies. Equal to Licenses unless particular module adds more.
	Effective_licenses []string `blueprint:"mutated"`
	// Override of module name when reporting licenses
	Effective_package_name *string `blueprint:"mutated"`
	// Notice files
	Effective_license_text NamedPaths `blueprint:"mutated"`
	// License names
	Effective_license_kinds []string `blueprint:"mutated"`
	// License conditions
	Effective_license_conditions []string `blueprint:"mutated"`

	// control whether this module compiles for 32-bit, 64-bit, or both.  Possible values
	// are "32" (compile for 32-bit only), "64" (compile for 64-bit only), "both" (compile for both
	// architectures), or "first" (compile for 64-bit on a 64-bit platform, and 32-bit on a 32-bit
	// platform).
	Compile_multilib *string `android:"arch_variant"`

	Target struct {
		Host struct {
			Compile_multilib *string
		}
		Android struct {
			Compile_multilib *string
		}
	}

	// If set to true then the archMutator will create variants for each arch specific target
	// (e.g. 32/64) that the module is required to produce. If set to false then it will only
	// create a variant for the architecture and will list the additional arch specific targets
	// that the variant needs to produce in the CompileMultiTargets property.
	UseTargetVariants bool   `blueprint:"mutated"`
	Default_multilib  string `blueprint:"mutated"`

	// whether this is a proprietary vendor module, and should be installed into /vendor
	Proprietary *bool

	// vendor who owns this module
	Owner *string

	// whether this module is specific to an SoC (System-On-a-Chip). When set to true,
	// it is installed into /vendor (or /system/vendor if vendor partition does not exist).
	// Use `soc_specific` instead for better meaning.
	Vendor *bool

	// whether this module is specific to an SoC (System-On-a-Chip). When set to true,
	// it is installed into /vendor (or /system/vendor if vendor partition does not exist).
	Soc_specific *bool

	// whether this module is specific to a device, not only for SoC, but also for off-chip
	// peripherals. When set to true, it is installed into /odm (or /vendor/odm if odm partition
	// does not exist, or /system/vendor/odm if both odm and vendor partitions do not exist).
	// This implies `soc_specific:true`.
	Device_specific *bool

	// whether this module is specific to a software configuration of a product (e.g. country,
	// network operator, etc). When set to true, it is installed into /product (or
	// /system/product if product partition does not exist).
	Product_specific *bool

	// whether this module extends system. When set to true, it is installed into /system_ext
	// (or /system/system_ext if system_ext partition does not exist).
	System_ext_specific *bool

	// Whether this module is installed to recovery partition
	Recovery *bool

	// Whether this module is installed to ramdisk
	Ramdisk *bool

	// Whether this module is installed to vendor ramdisk
	Vendor_ramdisk *bool

	// Whether this module is installed to debug ramdisk
	Debug_ramdisk *bool

	// Whether this module is built for non-native architectures (also known as native bridge binary)
	Native_bridge_supported *bool `android:"arch_variant"`

	// init.rc files to be installed if this module is installed
	Init_rc []string `android:"arch_variant,path"`

	// VINTF manifest fragments to be installed if this module is installed
	Vintf_fragments proptools.Configurable[[]string] `android:"path"`

	// names of other modules to install if this module is installed
	Required proptools.Configurable[[]string] `android:"arch_variant"`

	// names of other modules to install on host if this module is installed
	Host_required []string `android:"arch_variant"`

	// names of other modules to install on target if this module is installed
	Target_required []string `android:"arch_variant"`

	// The OsType of artifacts that this module variant is responsible for creating.
	//
	// Set by osMutator
	CompileOS OsType `blueprint:"mutated"`

	// Set to true after the arch mutator has run on this module and set CompileTarget,
	// CompileMultiTargets, and CompilePrimary
	ArchReady bool `blueprint:"mutated"`

	// The Target of artifacts that this module variant is responsible for creating.
	//
	// Set by archMutator
	CompileTarget Target `blueprint:"mutated"`

	// The additional arch specific targets (e.g. 32/64 bit) that this module variant is
	// responsible for creating.
	//
	// By default this is nil as, where necessary, separate variants are created for the
	// different multilib types supported and that information is encapsulated in the
	// CompileTarget so the module variant simply needs to create artifacts for that.
	//
	// However, if UseTargetVariants is set to false (e.g. by
	// InitAndroidMultiTargetsArchModule)  then no separate variants are created for the
	// multilib targets. Instead a single variant is created for the architecture and
	// this contains the multilib specific targets that this variant should create.
	//
	// Set by archMutator
	CompileMultiTargets []Target `blueprint:"mutated"`

	// True if the module variant's CompileTarget is the primary target
	//
	// Set by archMutator
	CompilePrimary bool `blueprint:"mutated"`

	// Set by InitAndroidModule
	HostOrDeviceSupported HostOrDeviceSupported `blueprint:"mutated"`
	ArchSpecific          bool                  `blueprint:"mutated"`

	// If set to true then a CommonOS variant will be created which will have dependencies
	// on all its OsType specific variants. Used by sdk/module_exports to create a snapshot
	// that covers all os and architecture variants.
	//
	// The OsType specific variants can be retrieved by calling
	// GetOsSpecificVariantsOfCommonOSVariant
	//
	// Set at module initialization time by calling InitCommonOSAndroidMultiTargetsArchModule
	CreateCommonOSVariant bool `blueprint:"mutated"`

	// If set to true then this variant is the CommonOS variant that has dependencies on its
	// OsType specific variants.
	//
	// Set by osMutator.
	CommonOSVariant bool `blueprint:"mutated"`

	// When set to true, this module is not installed to the full install path (ex: under
	// out/target/product/<name>/<partition>). It can be installed only to the packaging
	// modules like android_filesystem.
	No_full_install *bool

	// When HideFromMake is set to true, no entry for this variant will be emitted in the
	// generated Android.mk file.
	HideFromMake bool `blueprint:"mutated"`

	// When SkipInstall is set to true, calls to ctx.InstallFile, ctx.InstallExecutable,
	// ctx.InstallSymlink and ctx.InstallAbsoluteSymlink act like calls to ctx.PackageFile
	// and don't create a rule to install the file.
	SkipInstall bool `blueprint:"mutated"`

	// UninstallableApexPlatformVariant is set by MakeUninstallable called by the apex
	// mutator.  MakeUninstallable also sets HideFromMake.  UninstallableApexPlatformVariant
	// is used to avoid adding install or packaging dependencies into libraries provided
	// by apexes.
	UninstallableApexPlatformVariant bool `blueprint:"mutated"`

	// Whether the module has been replaced by a prebuilt
	ReplacedByPrebuilt bool `blueprint:"mutated"`

	// Disabled by mutators. If set to true, it overrides Enabled property.
	ForcedDisabled bool `blueprint:"mutated"`

	NamespaceExportedToMake bool `blueprint:"mutated"`

	MissingDeps        []string `blueprint:"mutated"`
	CheckedMissingDeps bool     `blueprint:"mutated"`

	// Name and variant strings stored by mutators to enable Module.String()
	DebugName       string   `blueprint:"mutated"`
	DebugMutators   []string `blueprint:"mutated"`
	DebugVariations []string `blueprint:"mutated"`

	// ImageVariation is set by ImageMutator to specify which image this variation is for,
	// for example "" for core or "recovery" for recovery.  It will often be set to one of the
	// constants in image.go, but can also be set to a custom value by individual module types.
	ImageVariation string `blueprint:"mutated"`

	// SoongConfigTrace records accesses to VendorVars (soong_config). The trace will be hashed
	// and used as a subdir of PathForModuleOut.  Note that we mainly focus on incremental
	// builds among similar products (e.g. aosp_cf_x86_64_phone and aosp_cf_x86_64_foldable),
	// and there are variables other than soong_config, which isn't captured by soong config
	// trace, but influence modules among products.
	SoongConfigTrace     soongConfigTrace `blueprint:"mutated"`
	SoongConfigTraceHash string           `blueprint:"mutated"`

	// The team (defined by the owner/vendor) who owns the property.
	Team *string `android:"path"`
}

type distProperties struct {
	// configuration to distribute output files from this module to the distribution
	// directory (default: $OUT/dist, configurable with $DIST_DIR)
	Dist Dist `android:"arch_variant"`

	// a list of configurations to distribute output files from this module to the
	// distribution directory (default: $OUT/dist, configurable with $DIST_DIR)
	Dists []Dist `android:"arch_variant"`
}

type TeamDepTagType struct {
	blueprint.BaseDependencyTag
}

var teamDepTag = TeamDepTagType{}

// Dependency tag for required, host_required, and target_required modules.
var RequiredDepTag = struct {
	blueprint.BaseDependencyTag
	InstallAlwaysNeededDependencyTag
	// Requiring disabled module has been supported (as a side effect of this being implemented
	// in Make). We may want to make it an error, but for now, let's keep the existing behavior.
	AlwaysAllowDisabledModuleDependencyTag
}{}

// CommonTestOptions represents the common `test_options` properties in
// Android.bp.
type CommonTestOptions struct {
	// If the test is a hostside (no device required) unittest that shall be run
	// during presubmit check.
	Unit_test *bool

	// Tags provide additional metadata to customize test execution by downstream
	// test runners. The tags have no special meaning to Soong.
	Tags []string
}

// SetAndroidMkEntries sets AndroidMkEntries according to the value of base
// `test_options`.
func (t *CommonTestOptions) SetAndroidMkEntries(entries *AndroidMkEntries) {
	entries.SetBoolIfTrue("LOCAL_IS_UNIT_TEST", Bool(t.Unit_test))
	if len(t.Tags) > 0 {
		entries.AddStrings("LOCAL_TEST_OPTIONS_TAGS", t.Tags...)
	}
}

// The key to use in TaggedDistFiles when a Dist structure does not specify a
// tag property. This intentionally does not use "" as the default because that
// would mean that an empty tag would have a different meaning when used in a dist
// structure that when used to reference a specific set of output paths using the
// :module{tag} syntax, which passes tag to the OutputFiles(tag) method.
const DefaultDistTag = "<default-dist-tag>"

// A map of OutputFile tag keys to Paths, for disting purposes.
type TaggedDistFiles map[string]Paths

// addPathsForTag adds a mapping from the tag to the paths. If the map is nil
// then it will create a map, update it and then return it. If a mapping already
// exists for the tag then the paths are appended to the end of the current list
// of paths, ignoring any duplicates.
func (t TaggedDistFiles) addPathsForTag(tag string, paths ...Path) TaggedDistFiles {
	if t == nil {
		t = make(TaggedDistFiles)
	}

	for _, distFile := range paths {
		if distFile != nil && !t[tag].containsPath(distFile) {
			t[tag] = append(t[tag], distFile)
		}
	}

	return t
}

// merge merges the entries from the other TaggedDistFiles object into this one.
// If the TaggedDistFiles is nil then it will create a new instance, merge the
// other into it, and then return it.
func (t TaggedDistFiles) merge(other TaggedDistFiles) TaggedDistFiles {
	for tag, paths := range other {
		t = t.addPathsForTag(tag, paths...)
	}

	return t
}

func MakeDefaultDistFiles(paths ...Path) TaggedDistFiles {
	for _, p := range paths {
		if p == nil {
			panic("The path to a dist file cannot be nil.")
		}
	}

	// The default OutputFile tag is the empty "" string.
	return TaggedDistFiles{DefaultDistTag: paths}
}

type hostAndDeviceProperties struct {
	// If set to true, build a variant of the module for the host.  Defaults to false.
	Host_supported *bool

	// If set to true, build a variant of the module for the device.  Defaults to true.
	Device_supported *bool
}

type Multilib string

const (
	MultilibBoth        Multilib = "both"
	MultilibFirst       Multilib = "first"
	MultilibCommon      Multilib = "common"
	MultilibCommonFirst Multilib = "common_first"
)

type HostOrDeviceSupported int

const (
	hostSupported = 1 << iota
	hostCrossSupported
	deviceSupported
	hostDefault
	deviceDefault

	// Host and HostCross are built by default. Device is not supported.
	HostSupported = hostSupported | hostCrossSupported | hostDefault

	// Host is built by default. HostCross and Device are not supported.
	HostSupportedNoCross = hostSupported | hostDefault

	// Device is built by default. Host and HostCross are not supported.
	DeviceSupported = deviceSupported | deviceDefault

	// By default, _only_ device variant is built. Device variant can be disabled with `device_supported: false`
	// Host and HostCross are disabled by default and can be enabled with `host_supported: true`
	HostAndDeviceSupported = hostSupported | hostCrossSupported | deviceSupported | deviceDefault

	// Host, HostCross, and Device are built by default.
	// Building Device can be disabled with `device_supported: false`
	// Building Host and HostCross can be disabled with `host_supported: false`
	HostAndDeviceDefault = hostSupported | hostCrossSupported | hostDefault |
		deviceSupported | deviceDefault

	// Nothing is supported. This is not exposed to the user, but used to mark a
	// host only module as unsupported when the module type is not supported on
	// the host OS. E.g. benchmarks are supported on Linux but not Darwin.
	NeitherHostNorDeviceSupported = 0
)

type moduleKind int

const (
	platformModule moduleKind = iota
	deviceSpecificModule
	socSpecificModule
	productSpecificModule
	systemExtSpecificModule
)

func (k moduleKind) String() string {
	switch k {
	case platformModule:
		return "platform"
	case deviceSpecificModule:
		return "device-specific"
	case socSpecificModule:
		return "soc-specific"
	case productSpecificModule:
		return "product-specific"
	case systemExtSpecificModule:
		return "systemext-specific"
	default:
		panic(fmt.Errorf("unknown module kind %d", k))
	}
}

func initAndroidModuleBase(m Module) {
	m.base().module = m
}

// InitAndroidModule initializes the Module as an Android module that is not architecture-specific.
// It adds the common properties, for example "name" and "enabled".
func InitAndroidModule(m Module) {
	initAndroidModuleBase(m)
	base := m.base()

	m.AddProperties(
		&base.nameProperties,
		&base.commonProperties,
		&base.distProperties)

	initProductVariableModule(m)

	// The default_visibility property needs to be checked and parsed by the visibility module during
	// its checking and parsing phases so make it the primary visibility property.
	setPrimaryVisibilityProperty(m, "visibility", &base.commonProperties.Visibility)

	// The default_applicable_licenses property needs to be checked and parsed by the licenses module during
	// its checking and parsing phases so make it the primary licenses property.
	setPrimaryLicensesProperty(m, "licenses", &base.commonProperties.Licenses)
}

// InitAndroidArchModule initializes the Module as an Android module that is architecture-specific.
// It adds the common properties, for example "name" and "enabled", as well as runtime generated
// property structs for architecture-specific versions of generic properties tagged with
// `android:"arch_variant"`.
//
//	InitAndroidModule should not be called if InitAndroidArchModule was called.
func InitAndroidArchModule(m Module, hod HostOrDeviceSupported, defaultMultilib Multilib) {
	InitAndroidModule(m)

	base := m.base()
	base.commonProperties.HostOrDeviceSupported = hod
	base.commonProperties.Default_multilib = string(defaultMultilib)
	base.commonProperties.ArchSpecific = true
	base.commonProperties.UseTargetVariants = true

	if hod&hostSupported != 0 && hod&deviceSupported != 0 {
		m.AddProperties(&base.hostAndDeviceProperties)
	}

	initArchModule(m)
}

// InitAndroidMultiTargetsArchModule initializes the Module as an Android module that is
// architecture-specific, but will only have a single variant per OS that handles all the
// architectures simultaneously.  The list of Targets that it must handle will be available from
// ModuleContext.MultiTargets. It adds the common properties, for example "name" and "enabled", as
// well as runtime generated property structs for architecture-specific versions of generic
// properties tagged with `android:"arch_variant"`.
//
// InitAndroidModule or InitAndroidArchModule should not be called if
// InitAndroidMultiTargetsArchModule was called.
func InitAndroidMultiTargetsArchModule(m Module, hod HostOrDeviceSupported, defaultMultilib Multilib) {
	InitAndroidArchModule(m, hod, defaultMultilib)
	m.base().commonProperties.UseTargetVariants = false
}

// InitCommonOSAndroidMultiTargetsArchModule initializes the Module as an Android module that is
// architecture-specific, but will only have a single variant per OS that handles all the
// architectures simultaneously, and will also have an additional CommonOS variant that has
// dependencies on all the OS-specific variants.  The list of Targets that it must handle will be
// available from ModuleContext.MultiTargets.  It adds the common properties, for example "name" and
// "enabled", as well as runtime generated property structs for architecture-specific versions of
// generic properties tagged with `android:"arch_variant"`.
//
// InitAndroidModule, InitAndroidArchModule or InitAndroidMultiTargetsArchModule should not be
// called if InitCommonOSAndroidMultiTargetsArchModule was called.
func InitCommonOSAndroidMultiTargetsArchModule(m Module, hod HostOrDeviceSupported, defaultMultilib Multilib) {
	InitAndroidArchModule(m, hod, defaultMultilib)
	m.base().commonProperties.UseTargetVariants = false
	m.base().commonProperties.CreateCommonOSVariant = true
}

// A ModuleBase object contains the properties that are common to all Android
// modules.  It should be included as an anonymous field in every module
// struct definition.  InitAndroidModule should then be called from the module's
// factory function, and the return values from InitAndroidModule should be
// returned from the factory function.
//
// The ModuleBase type is responsible for implementing the GenerateBuildActions
// method to support the blueprint.Module interface. This method will then call
// the module's GenerateAndroidBuildActions method once for each build variant
// that is to be built. GenerateAndroidBuildActions is passed a ModuleContext
// rather than the usual blueprint.ModuleContext.
// ModuleContext exposes extra functionality specific to the Android build
// system including details about the particular build variant that is to be
// generated.
//
// For example:
//
//	import (
//	    "android/soong/android"
//	)
//
//	type myModule struct {
//	    android.ModuleBase
//	    properties struct {
//	        MyProperty string
//	    }
//	}
//
//	func NewMyModule() android.Module {
//	    m := &myModule{}
//	    m.AddProperties(&m.properties)
//	    android.InitAndroidModule(m)
//	    return m
//	}
//
//	func (m *myModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
//	    // Get the CPU architecture for the current build variant.
//	    variantArch := ctx.Arch()
//
//	    // ...
//	}
type ModuleBase struct {
	// Putting the curiously recurring thing pointing to the thing that contains
	// the thing pattern to good use.
	// TODO: remove this
	module Module

	nameProperties          nameProperties
	commonProperties        commonProperties
	distProperties          distProperties
	variableProperties      interface{}
	hostAndDeviceProperties hostAndDeviceProperties

	// Arch specific versions of structs in GetProperties() prior to
	// initialization in InitAndroidArchModule, lets call it `generalProperties`.
	// The outer index has the same order as generalProperties and the inner index
	// chooses the props specific to the architecture. The interface{} value is an
	// archPropRoot that is filled with arch specific values by the arch mutator.
	archProperties [][]interface{}

	// Properties specific to the Blueprint to BUILD migration.
	bazelTargetModuleProperties bazel.BazelTargetModuleProperties

	// Information about all the properties on the module that contains visibility rules that need
	// checking.
	visibilityPropertyInfo []visibilityProperty

	// The primary visibility property, may be nil, that controls access to the module.
	primaryVisibilityProperty visibilityProperty

	// The primary licenses property, may be nil, records license metadata for the module.
	primaryLicensesProperty applicableLicensesProperty

	noAddressSanitizer   bool
	installFiles         InstallPaths
	installFilesDepSet   *DepSet[InstallPath]
	checkbuildFiles      Paths
	packagingSpecs       []PackagingSpec
	packagingSpecsDepSet *DepSet[PackagingSpec]
	// katiInstalls tracks the install rules that were created by Soong but are being exported
	// to Make to convert to ninja rules so that Make can add additional dependencies.
	katiInstalls katiInstalls
	// katiInitRcInstalls and katiVintfInstalls track the install rules created by Soong that are
	// allowed to have duplicates across modules and variants.
	katiInitRcInstalls katiInstalls
	katiVintfInstalls  katiInstalls
	katiSymlinks       katiInstalls
	testData           []DataPath

	// The files to copy to the dist as explicitly specified in the .bp file.
	distFiles TaggedDistFiles

	// Used by buildTargetSingleton to create checkbuild and per-directory build targets
	// Only set on the final variant of each module
	installTarget    WritablePath
	checkbuildTarget WritablePath
	blueprintDir     string

	hooks hooks

	registerProps []interface{}

	// For tests
	buildParams []BuildParams
	ruleParams  map[blueprint.Rule]blueprint.RuleParams
	variables   map[string]string

	initRcPaths         Paths
	vintfFragmentsPaths Paths

	installedInitRcPaths         InstallPaths
	installedVintfFragmentsPaths InstallPaths

	// Merged Aconfig files for all transitive deps.
	aconfigFilePaths Paths

	// set of dependency module:location mappings used to populate the license metadata for
	// apex containers.
	licenseInstallMap []string

	// The path to the generated license metadata file for the module.
	licenseMetadataFile WritablePath

	// moduleInfoJSON can be filled out by GenerateAndroidBuildActions to write a JSON file that will
	// be included in the final module-info.json produced by Make.
	moduleInfoJSON *ModuleInfoJSON

	// outputFiles stores the output of a module by tag and is used to set
	// the OutputFilesProvider in GenerateBuildActions
	outputFiles OutputFilesInfo
}

func (m *ModuleBase) AddJSONData(d *map[string]interface{}) {
	(*d)["Android"] = map[string]interface{}{
		// Properties set in Blueprint or in blueprint of a defaults modules
		"SetProperties": m.propertiesWithValues(),
	}
}

type propInfo struct {
	Name   string
	Type   string
	Value  string
	Values []string
}

func (m *ModuleBase) propertiesWithValues() []propInfo {
	var info []propInfo
	props := m.GetProperties()

	var propsWithValues func(name string, v reflect.Value)
	propsWithValues = func(name string, v reflect.Value) {
		kind := v.Kind()
		switch kind {
		case reflect.Ptr, reflect.Interface:
			if v.IsNil() {
				return
			}
			propsWithValues(name, v.Elem())
		case reflect.Struct:
			if v.IsZero() {
				return
			}
			for i := 0; i < v.NumField(); i++ {
				namePrefix := name
				sTyp := v.Type().Field(i)
				if proptools.ShouldSkipProperty(sTyp) {
					continue
				}
				if name != "" && !strings.HasSuffix(namePrefix, ".") {
					namePrefix += "."
				}
				if !proptools.IsEmbedded(sTyp) {
					namePrefix += sTyp.Name
				}
				sVal := v.Field(i)
				propsWithValues(namePrefix, sVal)
			}
		case reflect.Array, reflect.Slice:
			if v.IsNil() {
				return
			}
			elKind := v.Type().Elem().Kind()
			info = append(info, propInfo{Name: name, Type: elKind.String() + " " + kind.String(), Values: sliceReflectionValue(v)})
		default:
			info = append(info, propInfo{Name: name, Type: kind.String(), Value: reflectionValue(v)})
		}
	}

	for _, p := range props {
		propsWithValues("", reflect.ValueOf(p).Elem())
	}
	sort.Slice(info, func(i, j int) bool {
		return info[i].Name < info[j].Name
	})
	return info
}

func reflectionValue(value reflect.Value) string {
	switch value.Kind() {
	case reflect.Bool:
		return fmt.Sprintf("%t", value.Bool())
	case reflect.Int64:
		return fmt.Sprintf("%d", value.Int())
	case reflect.String:
		return fmt.Sprintf("%s", value.String())
	case reflect.Struct:
		if value.IsZero() {
			return "{}"
		}
		length := value.NumField()
		vals := make([]string, length, length)
		for i := 0; i < length; i++ {
			sTyp := value.Type().Field(i)
			if proptools.ShouldSkipProperty(sTyp) {
				continue
			}
			name := sTyp.Name
			vals[i] = fmt.Sprintf("%s: %s", name, reflectionValue(value.Field(i)))
		}
		return fmt.Sprintf("%s{%s}", value.Type(), strings.Join(vals, ", "))
	case reflect.Array, reflect.Slice:
		vals := sliceReflectionValue(value)
		return fmt.Sprintf("[%s]", strings.Join(vals, ", "))
	}
	return ""
}

func sliceReflectionValue(value reflect.Value) []string {
	length := value.Len()
	vals := make([]string, length, length)
	for i := 0; i < length; i++ {
		vals[i] = reflectionValue(value.Index(i))
	}
	return vals
}

func (m *ModuleBase) ComponentDepsMutator(BottomUpMutatorContext) {}

func (m *ModuleBase) DepsMutator(BottomUpMutatorContext) {}

func (m *ModuleBase) baseDepsMutator(ctx BottomUpMutatorContext) {
	if m.Team() != "" {
		ctx.AddDependency(ctx.Module(), teamDepTag, m.Team())
	}

	// TODO(jiyong): remove below case. This is to work around build errors happening
	// on branches with reduced manifest like aosp_kernel-build-tools.
	// In the branch, a build error occurs as follows.
	// 1. aosp_kernel-build-tools is a reduced manifest branch. It doesn't have some git
	// projects like external/bouncycastle
	// 2. `boot_signer` is `required` by modules like `build_image` which is explicitly list as
	// the top-level build goal (in the shell file that invokes Soong).
	// 3. `boot_signer` depends on `bouncycastle-unbundled` which is in the missing git project.
	// 4. aosp_kernel-build-tools invokes soong with `--skip-make`. Therefore, the absence of
	// ALLOW_MISSING_DEPENDENCIES didn't cause a problem.
	// 5. Now, since Soong understands `required` deps, it tries to build `boot_signer` and the
	// absence of external/bouncycastle fails the build.
	//
	// Unfortunately, there's no way for Soong to correctly determine if it's running in a
	// reduced manifest branch. Instead, here, we use the absence of DeviceArch or DeviceName as
	// a strong signal, because that's very common across reduced manifest branches.
	pv := ctx.Config().productVariables
	fullManifest := pv.DeviceArch != nil && pv.DeviceName != nil
	if fullManifest {
		addRequiredDeps(ctx)
	}
}

// addRequiredDeps adds required, target_required, and host_required as dependencies.
func addRequiredDeps(ctx BottomUpMutatorContext) {
	addDep := func(target Target, depName string) {
		if !ctx.OtherModuleExists(depName) {
			if ctx.Config().AllowMissingDependencies() {
				return
			}
		}

		// If Android native module requires another Android native module, ensure that
		// they have the same bitness. This mimics the policy in select-bitness-of-required-modules
		// in build/make/core/main.mk.
		// TODO(jiyong): the Make-side does this only when the required module is a shared
		// library or a native test.
		bothInAndroid := ctx.Device() && target.Os.Class == Device
		nativeArch := InList(ctx.Arch().ArchType.Multilib, []string{"lib32", "lib64"}) &&
			InList(target.Arch.ArchType.Multilib, []string{"lib32", "lib64"})
		sameBitness := ctx.Arch().ArchType.Multilib == target.Arch.ArchType.Multilib
		if bothInAndroid && nativeArch && !sameBitness {
			return
		}

		// ... also don't make a dependency between native bridge arch and non-native bridge
		// arches. b/342945184
		if ctx.Target().NativeBridge != target.NativeBridge {
			return
		}

		variation := target.Variations()
		if ctx.OtherModuleFarDependencyVariantExists(variation, depName) {
			ctx.AddFarVariationDependencies(variation, RequiredDepTag, depName)
		}
	}

	var deviceTargets []Target
	deviceTargets = append(deviceTargets, ctx.Config().Targets[Android]...)
	deviceTargets = append(deviceTargets, ctx.Config().AndroidCommonTarget)

	var hostTargets []Target
	hostTargets = append(hostTargets, ctx.Config().Targets[ctx.Config().BuildOS]...)
	hostTargets = append(hostTargets, ctx.Config().BuildOSCommonTarget)

	if ctx.Device() {
		for _, depName := range ctx.Module().RequiredModuleNames(ctx) {
			for _, target := range deviceTargets {
				addDep(target, depName)
			}
		}
		for _, depName := range ctx.Module().HostRequiredModuleNames() {
			for _, target := range hostTargets {
				addDep(target, depName)
			}
		}
	}

	if ctx.Host() {
		for _, depName := range ctx.Module().RequiredModuleNames(ctx) {
			for _, target := range hostTargets {
				// When a host module requires another host module, don't make a
				// dependency if they have different OSes (i.e. hostcross).
				if ctx.Target().HostCross != target.HostCross {
					continue
				}
				addDep(target, depName)
			}
		}
		for _, depName := range ctx.Module().TargetRequiredModuleNames() {
			for _, target := range deviceTargets {
				addDep(target, depName)
			}
		}
	}
}

// AddProperties "registers" the provided props
// each value in props MUST be a pointer to a struct
func (m *ModuleBase) AddProperties(props ...interface{}) {
	m.registerProps = append(m.registerProps, props...)
}

func (m *ModuleBase) GetProperties() []interface{} {
	return m.registerProps
}

func (m *ModuleBase) BuildParamsForTests() []BuildParams {
	// Expand the references to module variables like $flags[0-9]*,
	// so we do not need to change many existing unit tests.
	// This looks like undoing the shareFlags optimization in cc's
	// transformSourceToObj, and should only affects unit tests.
	vars := m.VariablesForTests()
	buildParams := append([]BuildParams(nil), m.buildParams...)
	for i := range buildParams {
		newArgs := make(map[string]string)
		for k, v := range buildParams[i].Args {
			newArgs[k] = v
			// Replaces both ${flags1} and $flags1 syntax.
			if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
				if value, found := vars[v[2:len(v)-1]]; found {
					newArgs[k] = value
				}
			} else if strings.HasPrefix(v, "$") {
				if value, found := vars[v[1:]]; found {
					newArgs[k] = value
				}
			}
		}
		buildParams[i].Args = newArgs
	}
	return buildParams
}

func (m *ModuleBase) RuleParamsForTests() map[blueprint.Rule]blueprint.RuleParams {
	return m.ruleParams
}

func (m *ModuleBase) VariablesForTests() map[string]string {
	return m.variables
}

// Name returns the name of the module.  It may be overridden by individual module types, for
// example prebuilts will prepend prebuilt_ to the name.
func (m *ModuleBase) Name() string {
	return String(m.nameProperties.Name)
}

// String returns a string that includes the module name and variants for printing during debugging.
func (m *ModuleBase) String() string {
	sb := strings.Builder{}
	sb.WriteString(m.commonProperties.DebugName)
	sb.WriteString("{")
	for i := range m.commonProperties.DebugMutators {
		if i != 0 {
			sb.WriteString(",")
		}
		sb.WriteString(m.commonProperties.DebugMutators[i])
		sb.WriteString(":")
		sb.WriteString(m.commonProperties.DebugVariations[i])
	}
	sb.WriteString("}")
	return sb.String()
}

// BaseModuleName returns the name of the module as specified in the blueprints file.
func (m *ModuleBase) BaseModuleName() string {
	return String(m.nameProperties.Name)
}

func (m *ModuleBase) base() *ModuleBase {
	return m
}

func (m *ModuleBase) qualifiedModuleId(ctx BaseModuleContext) qualifiedModuleName {
	return qualifiedModuleName{pkg: ctx.ModuleDir(), name: ctx.ModuleName()}
}

func (m *ModuleBase) visibilityProperties() []visibilityProperty {
	return m.visibilityPropertyInfo
}

func (m *ModuleBase) Dists() []Dist {
	if len(m.distProperties.Dist.Targets) > 0 {
		// Make a copy of the underlying Dists slice to protect against
		// backing array modifications with repeated calls to this method.
		distsCopy := append([]Dist(nil), m.distProperties.Dists...)
		return append(distsCopy, m.distProperties.Dist)
	} else {
		return m.distProperties.Dists
	}
}

func (m *ModuleBase) GenerateTaggedDistFiles(ctx BaseModuleContext) TaggedDistFiles {
	var distFiles TaggedDistFiles
	for _, dist := range m.Dists() {
		// If no tag is specified then it means to use the default dist paths so use
		// the special tag name which represents that.
		tag := proptools.StringDefault(dist.Tag, DefaultDistTag)

		if outputFileProducer, ok := m.module.(OutputFileProducer); ok {
			// Call the OutputFiles(tag) method to get the paths associated with the tag.
			distFilesForTag, err := outputFileProducer.OutputFiles(tag)

			// If the tag was not supported and is not DefaultDistTag then it is an error.
			// Failing to find paths for DefaultDistTag is not an error. It just means
			// that the module type requires the legacy behavior.
			if err != nil && tag != DefaultDistTag {
				ctx.PropertyErrorf("dist.tag", "%s", err.Error())
			}

			distFiles = distFiles.addPathsForTag(tag, distFilesForTag...)
		} else if tag != DefaultDistTag {
			// If the tag was specified then it is an error if the module does not
			// implement OutputFileProducer because there is no other way of accessing
			// the paths for the specified tag.
			ctx.PropertyErrorf("dist.tag",
				"tag %s not supported because the module does not implement OutputFileProducer", tag)
		}
	}

	return distFiles
}

func (m *ModuleBase) ArchReady() bool {
	return m.commonProperties.ArchReady
}

func (m *ModuleBase) Target() Target {
	return m.commonProperties.CompileTarget
}

func (m *ModuleBase) TargetPrimary() bool {
	return m.commonProperties.CompilePrimary
}

func (m *ModuleBase) MultiTargets() []Target {
	return m.commonProperties.CompileMultiTargets
}

func (m *ModuleBase) Os() OsType {
	return m.Target().Os
}

func (m *ModuleBase) Host() bool {
	return m.Os().Class == Host
}

func (m *ModuleBase) Device() bool {
	return m.Os().Class == Device
}

func (m *ModuleBase) Arch() Arch {
	return m.Target().Arch
}

func (m *ModuleBase) ArchSpecific() bool {
	return m.commonProperties.ArchSpecific
}

// True if the current variant is a CommonOS variant, false otherwise.
func (m *ModuleBase) IsCommonOSVariant() bool {
	return m.commonProperties.CommonOSVariant
}

// supportsTarget returns true if the given Target is supported by the current module.
func (m *ModuleBase) supportsTarget(target Target) bool {
	switch target.Os.Class {
	case Host:
		if target.HostCross {
			return m.HostCrossSupported()
		} else {
			return m.HostSupported()
		}
	case Device:
		return m.DeviceSupported()
	default:
		return false
	}
}

// DeviceSupported returns true if the current module is supported and enabled for device targets,
// i.e. the factory method set the HostOrDeviceSupported value to include device support and
// the device support is enabled by default or enabled by the device_supported property.
func (m *ModuleBase) DeviceSupported() bool {
	hod := m.commonProperties.HostOrDeviceSupported
	// deviceEnabled is true if the device_supported property is true or the HostOrDeviceSupported
	// value has the deviceDefault bit set.
	deviceEnabled := proptools.BoolDefault(m.hostAndDeviceProperties.Device_supported, hod&deviceDefault != 0)
	return hod&deviceSupported != 0 && deviceEnabled
}

// HostSupported returns true if the current module is supported and enabled for host targets,
// i.e. the factory method set the HostOrDeviceSupported value to include host support and
// the host support is enabled by default or enabled by the host_supported property.
func (m *ModuleBase) HostSupported() bool {
	hod := m.commonProperties.HostOrDeviceSupported
	// hostEnabled is true if the host_supported property is true or the HostOrDeviceSupported
	// value has the hostDefault bit set.
	hostEnabled := proptools.BoolDefault(m.hostAndDeviceProperties.Host_supported, hod&hostDefault != 0)
	return hod&hostSupported != 0 && hostEnabled
}

// HostCrossSupported returns true if the current module is supported and enabled for host cross
// targets, i.e. the factory method set the HostOrDeviceSupported value to include host cross
// support and the host cross support is enabled by default or enabled by the
// host_supported property.
func (m *ModuleBase) HostCrossSupported() bool {
	hod := m.commonProperties.HostOrDeviceSupported
	// hostEnabled is true if the host_supported property is true or the HostOrDeviceSupported
	// value has the hostDefault bit set.
	hostEnabled := proptools.BoolDefault(m.hostAndDeviceProperties.Host_supported, hod&hostDefault != 0)
	return hod&hostCrossSupported != 0 && hostEnabled
}

func (m *ModuleBase) Platform() bool {
	return !m.DeviceSpecific() && !m.SocSpecific() && !m.ProductSpecific() && !m.SystemExtSpecific()
}

func (m *ModuleBase) DeviceSpecific() bool {
	return Bool(m.commonProperties.Device_specific)
}

func (m *ModuleBase) SocSpecific() bool {
	return Bool(m.commonProperties.Vendor) || Bool(m.commonProperties.Proprietary) || Bool(m.commonProperties.Soc_specific)
}

func (m *ModuleBase) ProductSpecific() bool {
	return Bool(m.commonProperties.Product_specific)
}

func (m *ModuleBase) SystemExtSpecific() bool {
	return Bool(m.commonProperties.System_ext_specific)
}

// RequiresStableAPIs returns true if the module will be installed to a partition that may
// be updated separately from the system image.
func (m *ModuleBase) RequiresStableAPIs(ctx BaseModuleContext) bool {
	return m.SocSpecific() || m.DeviceSpecific() ||
		(m.ProductSpecific() && ctx.Config().EnforceProductPartitionInterface())
}

func (m *ModuleBase) PartitionTag(config DeviceConfig) string {
	partition := "system"
	if m.SocSpecific() {
		// A SoC-specific module could be on the vendor partition at
		// "vendor" or the system partition at "system/vendor".
		if config.VendorPath() == "vendor" {
			partition = "vendor"
		}
	} else if m.DeviceSpecific() {
		// A device-specific module could be on the odm partition at
		// "odm", the vendor partition at "vendor/odm", or the system
		// partition at "system/vendor/odm".
		if config.OdmPath() == "odm" {
			partition = "odm"
		} else if strings.HasPrefix(config.OdmPath(), "vendor/") {
			partition = "vendor"
		}
	} else if m.ProductSpecific() {
		// A product-specific module could be on the product partition
		// at "product" or the system partition at "system/product".
		if config.ProductPath() == "product" {
			partition = "product"
		}
	} else if m.SystemExtSpecific() {
		// A system_ext-specific module could be on the system_ext
		// partition at "system_ext" or the system partition at
		// "system/system_ext".
		if config.SystemExtPath() == "system_ext" {
			partition = "system_ext"
		}
	}
	return partition
}

func (m *ModuleBase) Enabled(ctx ConfigAndErrorContext) bool {
	if m.commonProperties.ForcedDisabled {
		return false
	}
	return m.commonProperties.Enabled.GetOrDefault(m.ConfigurableEvaluator(ctx), !m.Os().DefaultDisabled)
}

func (m *ModuleBase) Disable() {
	m.commonProperties.ForcedDisabled = true
}

// HideFromMake marks this variant so that it is not emitted in the generated Android.mk file.
func (m *ModuleBase) HideFromMake() {
	m.commonProperties.HideFromMake = true
}

// IsHideFromMake returns true if HideFromMake was previously called.
func (m *ModuleBase) IsHideFromMake() bool {
	return m.commonProperties.HideFromMake == true
}

// SkipInstall marks this variant to not create install rules when ctx.Install* are called.
func (m *ModuleBase) SkipInstall() {
	m.commonProperties.SkipInstall = true
}

// IsSkipInstall returns true if this variant is marked to not create install
// rules when ctx.Install* are called.
func (m *ModuleBase) IsSkipInstall() bool {
	return m.commonProperties.SkipInstall
}

// Similar to HideFromMake, but if the AndroidMk entry would set
// LOCAL_UNINSTALLABLE_MODULE then this variant may still output that entry
// rather than leaving it out altogether. That happens in cases where it would
// have other side effects, in particular when it adds a NOTICE file target,
// which other install targets might depend on.
func (m *ModuleBase) MakeUninstallable() {
	m.commonProperties.UninstallableApexPlatformVariant = true
	m.HideFromMake()
}

func (m *ModuleBase) ReplacedByPrebuilt() {
	m.commonProperties.ReplacedByPrebuilt = true
	m.HideFromMake()
}

func (m *ModuleBase) IsReplacedByPrebuilt() bool {
	return m.commonProperties.ReplacedByPrebuilt
}

func (m *ModuleBase) ExportedToMake() bool {
	return m.commonProperties.NamespaceExportedToMake
}

func (m *ModuleBase) EffectiveLicenseKinds() []string {
	return m.commonProperties.Effective_license_kinds
}

func (m *ModuleBase) EffectiveLicenseFiles() Paths {
	result := make(Paths, 0, len(m.commonProperties.Effective_license_text))
	for _, p := range m.commonProperties.Effective_license_text {
		result = append(result, p.Path)
	}
	return result
}

// computeInstallDeps finds the installed paths of all dependencies that have a dependency
// tag that is annotated as needing installation via the isInstallDepNeeded method.
func (m *ModuleBase) computeInstallDeps(ctx ModuleContext) ([]*DepSet[InstallPath], []*DepSet[PackagingSpec]) {
	var installDeps []*DepSet[InstallPath]
	var packagingSpecs []*DepSet[PackagingSpec]
	ctx.VisitDirectDeps(func(dep Module) {
		if isInstallDepNeeded(dep, ctx.OtherModuleDependencyTag(dep)) {
			// Installation is still handled by Make, so anything hidden from Make is not
			// installable.
			if !dep.IsHideFromMake() && !dep.IsSkipInstall() {
				installDeps = append(installDeps, dep.base().installFilesDepSet)
			}
			// Add packaging deps even when the dependency is not installed so that uninstallable
			// modules can still be packaged.  Often the package will be installed instead.
			packagingSpecs = append(packagingSpecs, dep.base().packagingSpecsDepSet)
		}
	})

	return installDeps, packagingSpecs
}

// isInstallDepNeeded returns true if installing the output files of the current module
// should also install the output files of the given dependency and dependency tag.
func isInstallDepNeeded(dep Module, tag blueprint.DependencyTag) bool {
	// Don't add a dependency from the platform to a library provided by an apex.
	if dep.base().commonProperties.UninstallableApexPlatformVariant {
		return false
	}
	// Only install modules if the dependency tag is an InstallDepNeeded tag.
	return IsInstallDepNeededTag(tag)
}

func (m *ModuleBase) FilesToInstall() InstallPaths {
	return m.installFiles
}

func (m *ModuleBase) PackagingSpecs() []PackagingSpec {
	return m.packagingSpecs
}

func (m *ModuleBase) TransitivePackagingSpecs() []PackagingSpec {
	return m.packagingSpecsDepSet.ToList()
}

func (m *ModuleBase) NoAddressSanitizer() bool {
	return m.noAddressSanitizer
}

func (m *ModuleBase) InstallInData() bool {
	return false
}

func (m *ModuleBase) InstallInTestcases() bool {
	return false
}

func (m *ModuleBase) InstallInSanitizerDir() bool {
	return false
}

func (m *ModuleBase) InstallInRamdisk() bool {
	return Bool(m.commonProperties.Ramdisk)
}

func (m *ModuleBase) InstallInVendorRamdisk() bool {
	return Bool(m.commonProperties.Vendor_ramdisk)
}

func (m *ModuleBase) InstallInDebugRamdisk() bool {
	return Bool(m.commonProperties.Debug_ramdisk)
}

func (m *ModuleBase) InstallInRecovery() bool {
	return Bool(m.commonProperties.Recovery)
}

func (m *ModuleBase) InstallInOdm() bool {
	return false
}

func (m *ModuleBase) InstallInProduct() bool {
	return false
}

func (m *ModuleBase) InstallInVendor() bool {
	return Bool(m.commonProperties.Vendor) || Bool(m.commonProperties.Soc_specific) || Bool(m.commonProperties.Proprietary)
}

func (m *ModuleBase) InstallInRoot() bool {
	return false
}

func (m *ModuleBase) InstallForceOS() (*OsType, *ArchType) {
	return nil, nil
}

func (m *ModuleBase) Owner() string {
	return String(m.commonProperties.Owner)
}

func (m *ModuleBase) Team() string {
	return String(m.commonProperties.Team)
}

func (m *ModuleBase) setImageVariation(variant string) {
	m.commonProperties.ImageVariation = variant
}

func (m *ModuleBase) ImageVariation() blueprint.Variation {
	return blueprint.Variation{
		Mutator:   "image",
		Variation: m.base().commonProperties.ImageVariation,
	}
}

func (m *ModuleBase) getVariationByMutatorName(mutator string) string {
	for i, v := range m.commonProperties.DebugMutators {
		if v == mutator {
			return m.commonProperties.DebugVariations[i]
		}
	}

	return ""
}

func (m *ModuleBase) InRamdisk() bool {
	return m.base().commonProperties.ImageVariation == RamdiskVariation
}

func (m *ModuleBase) InVendorRamdisk() bool {
	return m.base().commonProperties.ImageVariation == VendorRamdiskVariation
}

func (m *ModuleBase) InDebugRamdisk() bool {
	return m.base().commonProperties.ImageVariation == DebugRamdiskVariation
}

func (m *ModuleBase) InRecovery() bool {
	return m.base().commonProperties.ImageVariation == RecoveryVariation
}

func (m *ModuleBase) RequiredModuleNames(ctx ConfigAndErrorContext) []string {
	return m.base().commonProperties.Required.GetOrDefault(m.ConfigurableEvaluator(ctx), nil)
}

func (m *ModuleBase) HostRequiredModuleNames() []string {
	return m.base().commonProperties.Host_required
}

func (m *ModuleBase) TargetRequiredModuleNames() []string {
	return m.base().commonProperties.Target_required
}

func (m *ModuleBase) InitRc() Paths {
	return append(Paths{}, m.initRcPaths...)
}

func (m *ModuleBase) VintfFragments() Paths {
	return append(Paths{}, m.vintfFragmentsPaths...)
}

func (m *ModuleBase) CompileMultilib() *string {
	return m.base().commonProperties.Compile_multilib
}

// SetLicenseInstallMap stores the set of dependency module:location mappings for files in an
// apex container for use when generation the license metadata file.
func (m *ModuleBase) SetLicenseInstallMap(installMap []string) {
	m.licenseInstallMap = append(m.licenseInstallMap, installMap...)
}

func (m *ModuleBase) generateModuleTarget(ctx ModuleContext) {
	var allInstalledFiles InstallPaths
	var allCheckbuildFiles Paths
	ctx.VisitAllModuleVariants(func(module Module) {
		a := module.base()
		allInstalledFiles = append(allInstalledFiles, a.installFiles...)
		// A module's -checkbuild phony targets should
		// not be created if the module is not exported to make.
		// Those could depend on the build target and fail to compile
		// for the current build target.
		if !ctx.Config().KatiEnabled() || !shouldSkipAndroidMkProcessing(ctx, a) {
			allCheckbuildFiles = append(allCheckbuildFiles, a.checkbuildFiles...)
		}
	})

	var deps Paths

	namespacePrefix := ctx.Namespace().id
	if namespacePrefix != "" {
		namespacePrefix = namespacePrefix + "-"
	}

	if len(allInstalledFiles) > 0 {
		name := namespacePrefix + ctx.ModuleName() + "-install"
		ctx.Phony(name, allInstalledFiles.Paths()...)
		m.installTarget = PathForPhony(ctx, name)
		deps = append(deps, m.installTarget)
	}

	if len(allCheckbuildFiles) > 0 {
		name := namespacePrefix + ctx.ModuleName() + "-checkbuild"
		ctx.Phony(name, allCheckbuildFiles...)
		m.checkbuildTarget = PathForPhony(ctx, name)
		deps = append(deps, m.checkbuildTarget)
	}

	if len(deps) > 0 {
		suffix := ""
		if ctx.Config().KatiEnabled() {
			suffix = "-soong"
		}

		ctx.Phony(namespacePrefix+ctx.ModuleName()+suffix, deps...)

		m.blueprintDir = ctx.ModuleDir()
	}
}

func determineModuleKind(m *ModuleBase, ctx blueprint.EarlyModuleContext) moduleKind {
	var socSpecific = Bool(m.commonProperties.Vendor) || Bool(m.commonProperties.Proprietary) || Bool(m.commonProperties.Soc_specific)
	var deviceSpecific = Bool(m.commonProperties.Device_specific)
	var productSpecific = Bool(m.commonProperties.Product_specific)
	var systemExtSpecific = Bool(m.commonProperties.System_ext_specific)

	msg := "conflicting value set here"
	if socSpecific && deviceSpecific {
		ctx.PropertyErrorf("device_specific", "a module cannot be specific to SoC and device at the same time.")
		if Bool(m.commonProperties.Vendor) {
			ctx.PropertyErrorf("vendor", msg)
		}
		if Bool(m.commonProperties.Proprietary) {
			ctx.PropertyErrorf("proprietary", msg)
		}
		if Bool(m.commonProperties.Soc_specific) {
			ctx.PropertyErrorf("soc_specific", msg)
		}
	}

	if productSpecific && systemExtSpecific {
		ctx.PropertyErrorf("product_specific", "a module cannot be specific to product and system_ext at the same time.")
		ctx.PropertyErrorf("system_ext_specific", msg)
	}

	if (socSpecific || deviceSpecific) && (productSpecific || systemExtSpecific) {
		if productSpecific {
			ctx.PropertyErrorf("product_specific", "a module cannot be specific to SoC or device and product at the same time.")
		} else {
			ctx.PropertyErrorf("system_ext_specific", "a module cannot be specific to SoC or device and system_ext at the same time.")
		}
		if deviceSpecific {
			ctx.PropertyErrorf("device_specific", msg)
		} else {
			if Bool(m.commonProperties.Vendor) {
				ctx.PropertyErrorf("vendor", msg)
			}
			if Bool(m.commonProperties.Proprietary) {
				ctx.PropertyErrorf("proprietary", msg)
			}
			if Bool(m.commonProperties.Soc_specific) {
				ctx.PropertyErrorf("soc_specific", msg)
			}
		}
	}

	if productSpecific {
		return productSpecificModule
	} else if systemExtSpecific {
		return systemExtSpecificModule
	} else if deviceSpecific {
		return deviceSpecificModule
	} else if socSpecific {
		return socSpecificModule
	} else {
		return platformModule
	}
}

func (m *ModuleBase) earlyModuleContextFactory(ctx blueprint.EarlyModuleContext) earlyModuleContext {
	return earlyModuleContext{
		EarlyModuleContext: ctx,
		kind:               determineModuleKind(m, ctx),
		config:             ctx.Config().(Config),
	}
}

func (m *ModuleBase) baseModuleContextFactory(ctx blueprint.BaseModuleContext) baseModuleContext {
	return baseModuleContext{
		bp:                 ctx,
		archModuleContext:  m.archModuleContextFactory(ctx),
		earlyModuleContext: m.earlyModuleContextFactory(ctx),
	}
}

func (m *ModuleBase) archModuleContextFactory(ctx blueprint.IncomingTransitionContext) archModuleContext {
	config := ctx.Config().(Config)
	target := m.Target()
	primaryArch := false
	if len(config.Targets[target.Os]) <= 1 {
		primaryArch = true
	} else {
		primaryArch = target.Arch.ArchType == config.Targets[target.Os][0].Arch.ArchType
	}

	return archModuleContext{
		ready:         m.commonProperties.ArchReady,
		os:            m.commonProperties.CompileOS,
		target:        m.commonProperties.CompileTarget,
		targetPrimary: m.commonProperties.CompilePrimary,
		multiTargets:  m.commonProperties.CompileMultiTargets,
		primaryArch:   primaryArch,
	}

}

func (m *ModuleBase) GenerateBuildActions(blueprintCtx blueprint.ModuleContext) {
	ctx := &moduleContext{
		module:            m.module,
		bp:                blueprintCtx,
		baseModuleContext: m.baseModuleContextFactory(blueprintCtx),
		variables:         make(map[string]string),
	}

	m.licenseMetadataFile = PathForModuleOut(ctx, "meta_lic")

	dependencyInstallFiles, dependencyPackagingSpecs := m.computeInstallDeps(ctx)
	// set m.installFilesDepSet to only the transitive dependencies to be used as the dependencies
	// of installed files of this module.  It will be replaced by a depset including the installed
	// files of this module at the end for use by modules that depend on this one.
	m.installFilesDepSet = NewDepSet[InstallPath](TOPOLOGICAL, nil, dependencyInstallFiles)

	// Temporarily continue to call blueprintCtx.GetMissingDependencies() to maintain the previous behavior of never
	// reporting missing dependency errors in Blueprint when AllowMissingDependencies == true.
	// TODO: This will be removed once defaults modules handle missing dependency errors
	blueprintCtx.GetMissingDependencies()

	// For the final GenerateAndroidBuildActions pass, require that all visited dependencies Soong modules and
	// are enabled. Unless the module is a CommonOS variant which may have dependencies on disabled variants
	// (because the dependencies are added before the modules are disabled). The
	// GetOsSpecificVariantsOfCommonOSVariant(...) method will ensure that the disabled variants are
	// ignored.
	ctx.baseModuleContext.strictVisitDeps = !m.IsCommonOSVariant()

	if ctx.config.captureBuild {
		ctx.ruleParams = make(map[blueprint.Rule]blueprint.RuleParams)
	}

	desc := "//" + ctx.ModuleDir() + ":" + ctx.ModuleName() + " "
	var suffix []string
	if ctx.Os().Class != Device && ctx.Os().Class != Generic {
		suffix = append(suffix, ctx.Os().String())
	}
	if !ctx.PrimaryArch() {
		suffix = append(suffix, ctx.Arch().ArchType.String())
	}
	if apexInfo, _ := ModuleProvider(ctx, ApexInfoProvider); !apexInfo.IsForPlatform() {
		suffix = append(suffix, apexInfo.ApexVariationName)
	}

	ctx.Variable(pctx, "moduleDesc", desc)

	s := ""
	if len(suffix) > 0 {
		s = " [" + strings.Join(suffix, " ") + "]"
	}
	ctx.Variable(pctx, "moduleDescSuffix", s)

	// Some common property checks for properties that will be used later in androidmk.go
	checkDistProperties(ctx, "dist", &m.distProperties.Dist)
	for i := range m.distProperties.Dists {
		checkDistProperties(ctx, fmt.Sprintf("dists[%d]", i), &m.distProperties.Dists[i])
	}

	if m.Enabled(ctx) {
		// ensure all direct android.Module deps are enabled
		ctx.VisitDirectDepsBlueprint(func(bm blueprint.Module) {
			if m, ok := bm.(Module); ok {
				ctx.validateAndroidModule(bm, ctx.OtherModuleDependencyTag(m), ctx.baseModuleContext.strictVisitDeps, false)
			}
		})

		if m.Device() {
			// Handle any init.rc and vintf fragment files requested by the module.  All files installed by this
			// module will automatically have a dependency on the installed init.rc or vintf fragment file.
			// The same init.rc or vintf fragment file may be requested by multiple modules or variants,
			// so instead of installing them now just compute the install path and store it for later.
			// The full list of all init.rc and vintf fragment install rules will be deduplicated later
			// so only a single rule is created for each init.rc or vintf fragment file.

			if !m.InVendorRamdisk() {
				m.initRcPaths = PathsForModuleSrc(ctx, m.commonProperties.Init_rc)
				rcDir := PathForModuleInstall(ctx, "etc", "init")
				for _, src := range m.initRcPaths {
					installedInitRc := rcDir.Join(ctx, src.Base())
					m.katiInitRcInstalls = append(m.katiInitRcInstalls, katiInstall{
						from: src,
						to:   installedInitRc,
					})
					ctx.PackageFile(rcDir, src.Base(), src)
					m.installedInitRcPaths = append(m.installedInitRcPaths, installedInitRc)
				}
			}

			m.vintfFragmentsPaths = PathsForModuleSrc(ctx, m.commonProperties.Vintf_fragments.GetOrDefault(ctx, nil))
			vintfDir := PathForModuleInstall(ctx, "etc", "vintf", "manifest")
			for _, src := range m.vintfFragmentsPaths {
				installedVintfFragment := vintfDir.Join(ctx, src.Base())
				m.katiVintfInstalls = append(m.katiVintfInstalls, katiInstall{
					from: src,
					to:   installedVintfFragment,
				})
				ctx.PackageFile(vintfDir, src.Base(), src)
				m.installedVintfFragmentsPaths = append(m.installedVintfFragmentsPaths, installedVintfFragment)
			}
		}

		licensesPropertyFlattener(ctx)
		if ctx.Failed() {
			return
		}

		if jarJarPrefixHandler != nil {
			jarJarPrefixHandler(ctx)
			if ctx.Failed() {
				return
			}
		}

		// Call aconfigUpdateAndroidBuildActions to collect merged aconfig files before being used
		// in m.module.GenerateAndroidBuildActions
		aconfigUpdateAndroidBuildActions(ctx)
		if ctx.Failed() {
			return
		}

		m.module.GenerateAndroidBuildActions(ctx)
		if ctx.Failed() {
			return
		}

		// Create the set of tagged dist files after calling GenerateAndroidBuildActions
		// as GenerateTaggedDistFiles() calls OutputFiles(tag) and so relies on the
		// output paths being set which must be done before or during
		// GenerateAndroidBuildActions.
		m.distFiles = m.GenerateTaggedDistFiles(ctx)
		if ctx.Failed() {
			return
		}

		m.installFiles = append(m.installFiles, ctx.installFiles...)
		m.checkbuildFiles = append(m.checkbuildFiles, ctx.checkbuildFiles...)
		m.packagingSpecs = append(m.packagingSpecs, ctx.packagingSpecs...)
		m.katiInstalls = append(m.katiInstalls, ctx.katiInstalls...)
		m.katiSymlinks = append(m.katiSymlinks, ctx.katiSymlinks...)
		m.testData = append(m.testData, ctx.testData...)
	} else if ctx.Config().AllowMissingDependencies() {
		// If the module is not enabled it will not create any build rules, nothing will call
		// ctx.GetMissingDependencies(), and blueprint will consider the missing dependencies to be unhandled
		// and report them as an error even when AllowMissingDependencies = true.  Call
		// ctx.GetMissingDependencies() here to tell blueprint not to handle them.
		ctx.GetMissingDependencies()
	}

	if m == ctx.FinalModule().(Module).base() {
		m.generateModuleTarget(ctx)
		if ctx.Failed() {
			return
		}
	}

	m.installFilesDepSet = NewDepSet[InstallPath](TOPOLOGICAL, m.installFiles, dependencyInstallFiles)
	m.packagingSpecsDepSet = NewDepSet[PackagingSpec](TOPOLOGICAL, m.packagingSpecs, dependencyPackagingSpecs)

	buildLicenseMetadata(ctx, m.licenseMetadataFile)

	if m.moduleInfoJSON != nil {
		var installed InstallPaths
		installed = append(installed, m.katiInstalls.InstallPaths()...)
		installed = append(installed, m.katiSymlinks.InstallPaths()...)
		installed = append(installed, m.katiInitRcInstalls.InstallPaths()...)
		installed = append(installed, m.katiVintfInstalls.InstallPaths()...)
		installedStrings := installed.Strings()

		var targetRequired, hostRequired []string
		if ctx.Host() {
			targetRequired = m.commonProperties.Target_required
		} else {
			hostRequired = m.commonProperties.Host_required
		}

		var data []string
		for _, d := range m.testData {
			data = append(data, d.ToRelativeInstallPath())
		}

		if m.moduleInfoJSON.Uninstallable {
			installedStrings = nil
			if len(m.moduleInfoJSON.CompatibilitySuites) == 1 && m.moduleInfoJSON.CompatibilitySuites[0] == "null-suite" {
				m.moduleInfoJSON.CompatibilitySuites = nil
				m.moduleInfoJSON.TestConfig = nil
				m.moduleInfoJSON.AutoTestConfig = nil
				data = nil
			}
		}

		m.moduleInfoJSON.core = CoreModuleInfoJSON{
			RegisterName:       m.moduleInfoRegisterName(ctx, m.moduleInfoJSON.SubName),
			Path:               []string{ctx.ModuleDir()},
			Installed:          installedStrings,
			ModuleName:         m.BaseModuleName() + m.moduleInfoJSON.SubName,
			SupportedVariants:  []string{m.moduleInfoVariant(ctx)},
			TargetDependencies: targetRequired,
			HostDependencies:   hostRequired,
			Data:               data,
			Required:           m.RequiredModuleNames(ctx),
		}
		SetProvider(ctx, ModuleInfoJSONProvider, m.moduleInfoJSON)
	}

	m.buildParams = ctx.buildParams
	m.ruleParams = ctx.ruleParams
	m.variables = ctx.variables

	if m.outputFiles.DefaultOutputFiles != nil || m.outputFiles.TaggedOutputFiles != nil {
		SetProvider(ctx, OutputFilesProvider, m.outputFiles)
	}
}

func SetJarJarPrefixHandler(handler func(ModuleContext)) {
	if jarJarPrefixHandler != nil {
		panic("jarJarPrefixHandler already set")
	}
	jarJarPrefixHandler = handler
}

func (m *ModuleBase) moduleInfoRegisterName(ctx ModuleContext, subName string) string {
	name := m.BaseModuleName()

	prefix := ""
	if ctx.Host() {
		if ctx.Os() != ctx.Config().BuildOS {
			prefix = "host_cross_"
		}
	}
	suffix := ""
	arches := slices.Clone(ctx.Config().Targets[ctx.Os()])
	arches = slices.DeleteFunc(arches, func(target Target) bool {
		return target.NativeBridge != ctx.Target().NativeBridge
	})
	if len(arches) > 0 && ctx.Arch().ArchType != arches[0].Arch.ArchType {
		if ctx.Arch().ArchType.Multilib == "lib32" {
			suffix = "_32"
		} else {
			suffix = "_64"
		}
	}
	return prefix + name + subName + suffix
}

func (m *ModuleBase) moduleInfoVariant(ctx ModuleContext) string {
	variant := "DEVICE"
	if ctx.Host() {
		if ctx.Os() != ctx.Config().BuildOS {
			variant = "HOST_CROSS"
		} else {
			variant = "HOST"
		}
	}
	return variant
}

// Check the supplied dist structure to make sure that it is valid.
//
// property - the base property, e.g. dist or dists[1], which is combined with the
// name of the nested property to produce the full property, e.g. dist.dest or
// dists[1].dir.
func checkDistProperties(ctx *moduleContext, property string, dist *Dist) {
	if dist.Dest != nil {
		_, err := validateSafePath(*dist.Dest)
		if err != nil {
			ctx.PropertyErrorf(property+".dest", "%s", err.Error())
		}
	}
	if dist.Dir != nil {
		_, err := validateSafePath(*dist.Dir)
		if err != nil {
			ctx.PropertyErrorf(property+".dir", "%s", err.Error())
		}
	}
	if dist.Suffix != nil {
		if strings.Contains(*dist.Suffix, "/") {
			ctx.PropertyErrorf(property+".suffix", "Suffix may not contain a '/' character.")
		}
	}

}

// katiInstall stores a request from Soong to Make to create an install rule.
type katiInstall struct {
	from          Path
	to            InstallPath
	implicitDeps  Paths
	orderOnlyDeps Paths
	executable    bool
	extraFiles    *extraFilesZip

	absFrom string
}

type extraFilesZip struct {
	zip Path
	dir InstallPath
}

type katiInstalls []katiInstall

// BuiltInstalled returns the katiInstalls in the form used by $(call copy-many-files) in Make, a
// space separated list of from:to tuples.
func (installs katiInstalls) BuiltInstalled() string {
	sb := strings.Builder{}
	for i, install := range installs {
		if i != 0 {
			sb.WriteRune(' ')
		}
		sb.WriteString(install.from.String())
		sb.WriteRune(':')
		sb.WriteString(install.to.String())
	}
	return sb.String()
}

// InstallPaths returns the install path of each entry.
func (installs katiInstalls) InstallPaths() InstallPaths {
	paths := make(InstallPaths, 0, len(installs))
	for _, install := range installs {
		paths = append(paths, install.to)
	}
	return paths
}

// Makes this module a platform module, i.e. not specific to soc, device,
// product, or system_ext.
func (m *ModuleBase) MakeAsPlatform() {
	m.commonProperties.Vendor = boolPtr(false)
	m.commonProperties.Proprietary = boolPtr(false)
	m.commonProperties.Soc_specific = boolPtr(false)
	m.commonProperties.Product_specific = boolPtr(false)
	m.commonProperties.System_ext_specific = boolPtr(false)
}

func (m *ModuleBase) MakeAsSystemExt() {
	m.commonProperties.Vendor = boolPtr(false)
	m.commonProperties.Proprietary = boolPtr(false)
	m.commonProperties.Soc_specific = boolPtr(false)
	m.commonProperties.Product_specific = boolPtr(false)
	m.commonProperties.System_ext_specific = boolPtr(true)
}

// IsNativeBridgeSupported returns true if "native_bridge_supported" is explicitly set as "true"
func (m *ModuleBase) IsNativeBridgeSupported() bool {
	return proptools.Bool(m.commonProperties.Native_bridge_supported)
}

type ConfigAndErrorContext interface {
	Config() Config
	OtherModulePropertyErrorf(module Module, property string, fmt string, args ...interface{})
}

type configurationEvalutor struct {
	ctx ConfigAndErrorContext
	m   Module
}

func (m *ModuleBase) ConfigurableEvaluator(ctx ConfigAndErrorContext) proptools.ConfigurableEvaluator {
	return configurationEvalutor{
		ctx: ctx,
		m:   m.module,
	}
}

func (e configurationEvalutor) PropertyErrorf(property string, fmt string, args ...interface{}) {
	e.ctx.OtherModulePropertyErrorf(e.m, property, fmt, args...)
}

func (e configurationEvalutor) EvaluateConfiguration(condition proptools.ConfigurableCondition, property string) proptools.ConfigurableValue {
	ctx := e.ctx
	m := e.m
	switch condition.FunctionName() {
	case "release_flag":
		if condition.NumArgs() != 1 {
			ctx.OtherModulePropertyErrorf(m, property, "release_flag requires 1 argument, found %d", condition.NumArgs())
			return proptools.ConfigurableValueUndefined()
		}
		if ty, ok := ctx.Config().productVariables.BuildFlagTypes[condition.Arg(0)]; ok {
			v := ctx.Config().productVariables.BuildFlags[condition.Arg(0)]
			switch ty {
			case "unspecified", "obsolete":
				return proptools.ConfigurableValueUndefined()
			case "string":
				return proptools.ConfigurableValueString(v)
			case "bool":
				return proptools.ConfigurableValueBool(v == "true")
			default:
				panic("unhandled release flag type: " + ty)
			}
		}
		return proptools.ConfigurableValueUndefined()
	case "product_variable":
		if condition.NumArgs() != 1 {
			ctx.OtherModulePropertyErrorf(m, property, "product_variable requires 1 argument, found %d", condition.NumArgs())
			return proptools.ConfigurableValueUndefined()
		}
		variable := condition.Arg(0)
		switch variable {
		case "debuggable":
			return proptools.ConfigurableValueBool(ctx.Config().Debuggable())
		case "use_debug_art":
			// TODO(b/234351700): Remove once ART does not have separated debug APEX
			return proptools.ConfigurableValueBool(ctx.Config().UseDebugArt())
		default:
			// TODO(b/323382414): Might add these on a case-by-case basis
			ctx.OtherModulePropertyErrorf(m, property, fmt.Sprintf("TODO(b/323382414): Product variable %q is not yet supported in selects", variable))
			return proptools.ConfigurableValueUndefined()
		}
	case "soong_config_variable":
		if condition.NumArgs() != 2 {
			ctx.OtherModulePropertyErrorf(m, property, "soong_config_variable requires 2 arguments, found %d", condition.NumArgs())
			return proptools.ConfigurableValueUndefined()
		}
		namespace := condition.Arg(0)
		variable := condition.Arg(1)
		if n, ok := ctx.Config().productVariables.VendorVars[namespace]; ok {
			if v, ok := n[variable]; ok {
				ty := ""
				if namespaces, ok := ctx.Config().productVariables.VendorVarTypes[namespace]; ok {
					ty = namespaces[variable]
				}
				switch ty {
				case "":
					// strings are the default, we don't bother writing them to the soong variables json file
					return proptools.ConfigurableValueString(v)
				case "bool":
					return proptools.ConfigurableValueBool(v == "true")
				default:
					panic("unhandled soong config variable type: " + ty)
				}

			}
		}
		return proptools.ConfigurableValueUndefined()
	case "arch":
		if condition.NumArgs() != 0 {
			ctx.OtherModulePropertyErrorf(m, property, "arch requires no arguments, found %d", condition.NumArgs())
			return proptools.ConfigurableValueUndefined()
		}
		if !m.base().ArchReady() {
			ctx.OtherModulePropertyErrorf(m, property, "A select on arch was attempted before the arch mutator ran")
			return proptools.ConfigurableValueUndefined()
		}
		return proptools.ConfigurableValueString(m.base().Arch().ArchType.Name)
	case "os":
		if condition.NumArgs() != 0 {
			ctx.OtherModulePropertyErrorf(m, property, "os requires no arguments, found %d", condition.NumArgs())
			return proptools.ConfigurableValueUndefined()
		}
		// the arch mutator runs after the os mutator, we can just use this to enforce that os is ready.
		if !m.base().ArchReady() {
			ctx.OtherModulePropertyErrorf(m, property, "A select on os was attempted before the arch mutator ran (arch runs after os, we use it to lazily detect that os is ready)")
			return proptools.ConfigurableValueUndefined()
		}
		return proptools.ConfigurableValueString(m.base().Os().Name)
	case "boolean_var_for_testing":
		// We currently don't have any other boolean variables (we should add support for typing
		// the soong config variables), so add this fake one for testing the boolean select
		// functionality.
		if condition.NumArgs() != 0 {
			ctx.OtherModulePropertyErrorf(m, property, "boolean_var_for_testing requires 0 arguments, found %d", condition.NumArgs())
			return proptools.ConfigurableValueUndefined()
		}

		if n, ok := ctx.Config().productVariables.VendorVars["boolean_var"]; ok {
			if v, ok := n["for_testing"]; ok {
				switch v {
				case "true":
					return proptools.ConfigurableValueBool(true)
				case "false":
					return proptools.ConfigurableValueBool(false)
				default:
					ctx.OtherModulePropertyErrorf(m, property, "testing:my_boolean_var can only be true or false, found %q", v)
				}
			}
		}
		return proptools.ConfigurableValueUndefined()
	default:
		ctx.OtherModulePropertyErrorf(m, property, "Unknown select condition %s", condition.FunctionName)
		return proptools.ConfigurableValueUndefined()
	}
}

// ModuleNameWithPossibleOverride returns the name of the OverrideModule that overrides the current
// variant of this OverridableModule, or ctx.ModuleName() if this module is not an OverridableModule
// or if this variant is not overridden.
func ModuleNameWithPossibleOverride(ctx BaseModuleContext) string {
	if overridable, ok := ctx.Module().(OverridableModule); ok {
		if o := overridable.GetOverriddenBy(); o != "" {
			return o
		}
	}
	return ctx.ModuleName()
}

// SrcIsModule decodes module references in the format ":unqualified-name" or "//namespace:name"
// into the module name, or empty string if the input was not a module reference.
func SrcIsModule(s string) (module string) {
	if len(s) > 1 {
		if s[0] == ':' {
			module = s[1:]
			if !isUnqualifiedModuleName(module) {
				// The module name should be unqualified but is not so do not treat it as a module.
				module = ""
			}
		} else if s[0] == '/' && s[1] == '/' {
			module = s
		}
	}
	return module
}

// SrcIsModuleWithTag decodes module references in the format ":unqualified-name{.tag}" or
// "//namespace:name{.tag}" into the module name and tag, ":unqualified-name" or "//namespace:name"
// into the module name and an empty string for the tag, or empty strings if the input was not a
// module reference.
func SrcIsModuleWithTag(s string) (module, tag string) {
	if len(s) > 1 {
		if s[0] == ':' {
			module = s[1:]
		} else if s[0] == '/' && s[1] == '/' {
			module = s
		}

		if module != "" {
			if tagStart := strings.IndexByte(module, '{'); tagStart > 0 {
				if module[len(module)-1] == '}' {
					tag = module[tagStart+1 : len(module)-1]
					module = module[:tagStart]
				}
			}

			if s[0] == ':' && !isUnqualifiedModuleName(module) {
				// The module name should be unqualified but is not so do not treat it as a module.
				module = ""
				tag = ""
			}
		}
	}

	return module, tag
}

// isUnqualifiedModuleName makes sure that the supplied module is an unqualified module name, i.e.
// does not contain any /.
func isUnqualifiedModuleName(module string) bool {
	return strings.IndexByte(module, '/') == -1
}

// sourceOrOutputDependencyTag is the dependency tag added automatically by pathDepsMutator for any
// module reference in a property annotated with `android:"path"` or passed to ExtractSourceDeps
// or ExtractSourcesDeps.
//
// If uniquely identifies the dependency that was added as it contains both the module name used to
// add the dependency as well as the tag. That makes it very simple to find the matching dependency
// in GetModuleFromPathDep as all it needs to do is find the dependency whose tag matches the tag
// used to add it. It does not need to check that the module name as returned by one of
// Module.Name(), BaseModuleContext.OtherModuleName() or ModuleBase.BaseModuleName() matches the
// name supplied in the tag. That means it does not need to handle differences in module names
// caused by prebuilt_ prefix, or fully qualified module names.
type sourceOrOutputDependencyTag struct {
	blueprint.BaseDependencyTag
	AlwaysPropagateAconfigValidationDependencyTag

	// The name of the module.
	moduleName string

	// The tag that will be passed to the module's OutputFileProducer.OutputFiles(tag) method.
	tag string
}

func sourceOrOutputDepTag(moduleName, tag string) blueprint.DependencyTag {
	return sourceOrOutputDependencyTag{moduleName: moduleName, tag: tag}
}

// IsSourceDepTagWithOutputTag returns true if the supplied blueprint.DependencyTag is one that was
// used to add dependencies by either ExtractSourceDeps, ExtractSourcesDeps or automatically for
// properties tagged with `android:"path"` AND it was added using a module reference of
// :moduleName{outputTag}.
func IsSourceDepTagWithOutputTag(depTag blueprint.DependencyTag, outputTag string) bool {
	t, ok := depTag.(sourceOrOutputDependencyTag)
	return ok && t.tag == outputTag
}

// Adds necessary dependencies to satisfy filegroup or generated sources modules listed in srcFiles
// using ":module" syntax, if any.
//
// Deprecated: tag the property with `android:"path"` instead.
func ExtractSourcesDeps(ctx BottomUpMutatorContext, srcFiles []string) {
	set := make(map[string]bool)

	for _, s := range srcFiles {
		if m, t := SrcIsModuleWithTag(s); m != "" {
			if _, found := set[s]; found {
				ctx.ModuleErrorf("found source dependency duplicate: %q!", s)
			} else {
				set[s] = true
				ctx.AddDependency(ctx.Module(), sourceOrOutputDepTag(m, t), m)
			}
		}
	}
}

// Adds necessary dependencies to satisfy filegroup or generated sources modules specified in s
// using ":module" syntax, if any.
//
// Deprecated: tag the property with `android:"path"` instead.
func ExtractSourceDeps(ctx BottomUpMutatorContext, s *string) {
	if s != nil {
		if m, t := SrcIsModuleWithTag(*s); m != "" {
			ctx.AddDependency(ctx.Module(), sourceOrOutputDepTag(m, t), m)
		}
	}
}

// A module that implements SourceFileProducer can be referenced from any property that is tagged with `android:"path"`
// using the ":module" syntax and provides a list of paths to be used as if they were listed in the property.
type SourceFileProducer interface {
	Srcs() Paths
}

// A module that implements OutputFileProducer can be referenced from any property that is tagged with `android:"path"`
// using the ":module" syntax or ":module{.tag}" syntax and provides a list of output files to be used as if they were
// listed in the property.
type OutputFileProducer interface {
	OutputFiles(tag string) (Paths, error)
}

// OutputFilesForModule returns the paths from an OutputFileProducer with the given tag.  On error, including if the
// module produced zero paths, it reports errors to the ctx and returns nil.
func OutputFilesForModule(ctx PathContext, module blueprint.Module, tag string) Paths {
	paths, err := outputFilesForModule(ctx, module, tag)
	if err != nil {
		reportPathError(ctx, err)
		return nil
	}
	return paths
}

// OutputFileForModule returns the path from an OutputFileProducer with the given tag.  On error, including if the
// module produced zero or multiple paths, it reports errors to the ctx and returns nil.
func OutputFileForModule(ctx PathContext, module blueprint.Module, tag string) Path {
	paths, err := outputFilesForModule(ctx, module, tag)
	if err != nil {
		reportPathError(ctx, err)
		return nil
	}
	if len(paths) == 0 {
		type addMissingDependenciesIntf interface {
			AddMissingDependencies([]string)
			OtherModuleName(blueprint.Module) string
		}
		if mctx, ok := ctx.(addMissingDependenciesIntf); ok && ctx.Config().AllowMissingDependencies() {
			mctx.AddMissingDependencies([]string{mctx.OtherModuleName(module)})
		} else {
			ReportPathErrorf(ctx, "failed to get output files from module %q", pathContextName(ctx, module))
		}
		// Return a fake output file to avoid nil dereferences of Path objects later.
		// This should never get used for an actual build as the error or missing
		// dependency has already been reported.
		p, err := pathForSource(ctx, filepath.Join("missing_output_file", pathContextName(ctx, module)))
		if err != nil {
			reportPathError(ctx, err)
			return nil
		}
		return p
	}
	if len(paths) > 1 {
		ReportPathErrorf(ctx, "got multiple output files from module %q, expected exactly one",
			pathContextName(ctx, module))
	}
	return paths[0]
}

func outputFilesForModule(ctx PathContext, module blueprint.Module, tag string) (Paths, error) {
	outputFilesFromProvider, err := outputFilesForModuleFromProvider(ctx, module, tag)
	if outputFilesFromProvider != nil || err != nil {
		return outputFilesFromProvider, err
	}
	if outputFileProducer, ok := module.(OutputFileProducer); ok {
		paths, err := outputFileProducer.OutputFiles(tag)
		if err != nil {
			return nil, fmt.Errorf("failed to get output file from module %q at tag %q: %s",
				pathContextName(ctx, module), tag, err.Error())
		}
		return paths, nil
	} else if sourceFileProducer, ok := module.(SourceFileProducer); ok {
		if tag != "" {
			return nil, fmt.Errorf("module %q is a SourceFileProducer, not an OutputFileProducer, and so does not support tag %q", pathContextName(ctx, module), tag)
		}
		paths := sourceFileProducer.Srcs()
		return paths, nil
	} else {
		return nil, fmt.Errorf("module %q is not an OutputFileProducer or SourceFileProducer", pathContextName(ctx, module))
	}
}

// This method uses OutputFilesProvider for output files
// *inter-module-communication*.
// If mctx module is the same as the param module the output files are obtained
// from outputFiles property of module base, to avoid both setting and
// reading OutputFilesProvider before  GenerateBuildActions is finished. Also
// only empty-string-tag is supported in this case.
// If a module doesn't have the OutputFilesProvider, nil is returned.
func outputFilesForModuleFromProvider(ctx PathContext, module blueprint.Module, tag string) (Paths, error) {
	// TODO: support OutputFilesProvider for singletons
	mctx, ok := ctx.(ModuleContext)
	if !ok {
		return nil, nil
	}
	if mctx.Module() != module {
		if outputFilesProvider, ok := OtherModuleProvider(mctx, module, OutputFilesProvider); ok {
			if tag == "" {
				return outputFilesProvider.DefaultOutputFiles, nil
			} else if taggedOutputFiles, hasTag := outputFilesProvider.TaggedOutputFiles[tag]; hasTag {
				return taggedOutputFiles, nil
			} else {
				return nil, fmt.Errorf("unsupported module reference tag %q", tag)
			}
		}
	} else {
		if tag == "" {
			return mctx.Module().base().outputFiles.DefaultOutputFiles, nil
		} else {
			return nil, fmt.Errorf("unsupported tag %q for module getting its own output files", tag)
		}
	}
	// TODO: Add a check for param module not having OutputFilesProvider set
	return nil, nil
}

type OutputFilesInfo struct {
	// default output files when tag is an empty string ""
	DefaultOutputFiles Paths

	// the corresponding output files for given tags
	TaggedOutputFiles map[string]Paths
}

var OutputFilesProvider = blueprint.NewProvider[OutputFilesInfo]()

// Modules can implement HostToolProvider and return a valid OptionalPath from HostToolPath() to
// specify that they can be used as a tool by a genrule module.
type HostToolProvider interface {
	Module
	// HostToolPath returns the path to the host tool for the module if it is one, or an invalid
	// OptionalPath.
	HostToolPath() OptionalPath
}

func init() {
	RegisterParallelSingletonType("buildtarget", BuildTargetSingleton)
	RegisterParallelSingletonType("soongconfigtrace", soongConfigTraceSingletonFunc)
	FinalDepsMutators(registerSoongConfigTraceMutator)
}

func BuildTargetSingleton() Singleton {
	return &buildTargetSingleton{}
}

func parentDir(dir string) string {
	dir, _ = filepath.Split(dir)
	return filepath.Clean(dir)
}

type buildTargetSingleton struct{}

func AddAncestors(ctx SingletonContext, dirMap map[string]Paths, mmName func(string) string) ([]string, []string) {
	// Ensure ancestor directories are in dirMap
	// Make directories build their direct subdirectories
	// Returns a slice of all directories and a slice of top-level directories.
	dirs := SortedKeys(dirMap)
	for _, dir := range dirs {
		dir := parentDir(dir)
		for dir != "." && dir != "/" {
			if _, exists := dirMap[dir]; exists {
				break
			}
			dirMap[dir] = nil
			dir = parentDir(dir)
		}
	}
	dirs = SortedKeys(dirMap)
	var topDirs []string
	for _, dir := range dirs {
		p := parentDir(dir)
		if p != "." && p != "/" {
			dirMap[p] = append(dirMap[p], PathForPhony(ctx, mmName(dir)))
		} else if dir != "." && dir != "/" && dir != "" {
			topDirs = append(topDirs, dir)
		}
	}
	return SortedKeys(dirMap), topDirs
}

func (c *buildTargetSingleton) GenerateBuildActions(ctx SingletonContext) {
	var checkbuildDeps Paths

	mmTarget := func(dir string) string {
		return "MODULES-IN-" + strings.Replace(filepath.Clean(dir), "/", "-", -1)
	}

	modulesInDir := make(map[string]Paths)

	ctx.VisitAllModules(func(module Module) {
		blueprintDir := module.base().blueprintDir
		installTarget := module.base().installTarget
		checkbuildTarget := module.base().checkbuildTarget

		if checkbuildTarget != nil {
			checkbuildDeps = append(checkbuildDeps, checkbuildTarget)
			modulesInDir[blueprintDir] = append(modulesInDir[blueprintDir], checkbuildTarget)
		}

		if installTarget != nil {
			modulesInDir[blueprintDir] = append(modulesInDir[blueprintDir], installTarget)
		}
	})

	suffix := ""
	if ctx.Config().KatiEnabled() {
		suffix = "-soong"
	}

	// Create a top-level checkbuild target that depends on all modules
	ctx.Phony("checkbuild"+suffix, checkbuildDeps...)

	// Make will generate the MODULES-IN-* targets
	if ctx.Config().KatiEnabled() {
		return
	}

	dirs, _ := AddAncestors(ctx, modulesInDir, mmTarget)

	// Create a MODULES-IN-<directory> target that depends on all modules in a directory, and
	// depends on the MODULES-IN-* targets of all of its subdirectories that contain Android.bp
	// files.
	for _, dir := range dirs {
		ctx.Phony(mmTarget(dir), modulesInDir[dir]...)
	}

	// Create (host|host-cross|target)-<OS> phony rules to build a reduced checkbuild.
	type osAndCross struct {
		os        OsType
		hostCross bool
	}
	osDeps := map[osAndCross]Paths{}
	ctx.VisitAllModules(func(module Module) {
		if module.Enabled(ctx) {
			key := osAndCross{os: module.Target().Os, hostCross: module.Target().HostCross}
			osDeps[key] = append(osDeps[key], module.base().checkbuildFiles...)
		}
	})

	osClass := make(map[string]Paths)
	for key, deps := range osDeps {
		var className string

		switch key.os.Class {
		case Host:
			if key.hostCross {
				className = "host-cross"
			} else {
				className = "host"
			}
		case Device:
			className = "target"
		default:
			continue
		}

		name := className + "-" + key.os.Name
		osClass[className] = append(osClass[className], PathForPhony(ctx, name))

		ctx.Phony(name, deps...)
	}

	// Wrap those into host|host-cross|target phony rules
	for _, class := range SortedKeys(osClass) {
		ctx.Phony(class, osClass[class]...)
	}
}

// Collect information for opening IDE project files in java/jdeps.go.
type IDEInfo interface {
	IDEInfo(ideInfo *IdeInfo)
	BaseModuleName() string
}

// Extract the base module name from the Import name.
// Often the Import name has a prefix "prebuilt_".
// Remove the prefix explicitly if needed
// until we find a better solution to get the Import name.
type IDECustomizedModuleName interface {
	IDECustomizedModuleName() string
}

type IdeInfo struct {
	Deps              []string `json:"dependencies,omitempty"`
	Srcs              []string `json:"srcs,omitempty"`
	Aidl_include_dirs []string `json:"aidl_include_dirs,omitempty"`
	Jarjar_rules      []string `json:"jarjar_rules,omitempty"`
	Jars              []string `json:"jars,omitempty"`
	Classes           []string `json:"class,omitempty"`
	Installed_paths   []string `json:"installed,omitempty"`
	SrcJars           []string `json:"srcjars,omitempty"`
	Paths             []string `json:"path,omitempty"`
	Static_libs       []string `json:"static_libs,omitempty"`
	Libs              []string `json:"libs,omitempty"`
}

func CheckBlueprintSyntax(ctx BaseModuleContext, filename string, contents string) []error {
	bpctx := ctx.blueprintBaseModuleContext()
	return blueprint.CheckBlueprintSyntax(bpctx.ModuleFactories(), filename, contents)
}

func registerSoongConfigTraceMutator(ctx RegisterMutatorsContext) {
	ctx.BottomUp("soongconfigtrace", soongConfigTraceMutator).Parallel()
}

// soongConfigTraceMutator accumulates recorded soong_config trace from children. Also it normalizes
// SoongConfigTrace to make it consistent.
func soongConfigTraceMutator(ctx BottomUpMutatorContext) {
	trace := &ctx.Module().base().commonProperties.SoongConfigTrace
	ctx.VisitDirectDeps(func(m Module) {
		childTrace := &m.base().commonProperties.SoongConfigTrace
		trace.Bools = append(trace.Bools, childTrace.Bools...)
		trace.Strings = append(trace.Strings, childTrace.Strings...)
		trace.IsSets = append(trace.IsSets, childTrace.IsSets...)
	})
	trace.Bools = SortedUniqueStrings(trace.Bools)
	trace.Strings = SortedUniqueStrings(trace.Strings)
	trace.IsSets = SortedUniqueStrings(trace.IsSets)

	ctx.Module().base().commonProperties.SoongConfigTraceHash = trace.hash()
}

// soongConfigTraceSingleton writes a map from each module's config hash value to trace data.
func soongConfigTraceSingletonFunc() Singleton {
	return &soongConfigTraceSingleton{}
}

type soongConfigTraceSingleton struct {
}

func (s *soongConfigTraceSingleton) GenerateBuildActions(ctx SingletonContext) {
	outFile := PathForOutput(ctx, "soong_config_trace.json")

	traces := make(map[string]*soongConfigTrace)
	ctx.VisitAllModules(func(module Module) {
		trace := &module.base().commonProperties.SoongConfigTrace
		if !trace.isEmpty() {
			hash := module.base().commonProperties.SoongConfigTraceHash
			traces[hash] = trace
		}
	})

	j, err := json.Marshal(traces)
	if err != nil {
		ctx.Errorf("json marshal to %q failed: %#v", outFile, err)
		return
	}

	WriteFileRule(ctx, outFile, string(j))
	ctx.Phony("soong_config_trace", outFile)
}
