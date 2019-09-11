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
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/scanner"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"
	"github.com/google/blueprint/proptools"
)

var (
	DeviceSharedLibrary = "shared_library"
	DeviceStaticLibrary = "static_library"
	DeviceExecutable    = "executable"
	HostSharedLibrary   = "host_shared_library"
	HostStaticLibrary   = "host_static_library"
	HostExecutable      = "host_executable"
)

type BuildParams struct {
	Rule            blueprint.Rule
	Deps            blueprint.Deps
	Depfile         WritablePath
	Description     string
	Output          WritablePath
	Outputs         WritablePaths
	ImplicitOutput  WritablePath
	ImplicitOutputs WritablePaths
	Input           Path
	Inputs          Paths
	Implicit        Path
	Implicits       Paths
	OrderOnly       Paths
	Default         bool
	Args            map[string]string
}

type ModuleBuildParams BuildParams

type androidBaseContext interface {
	Target() Target
	TargetPrimary() bool
	MultiTargets() []Target
	Arch() Arch
	Os() OsType
	Host() bool
	Device() bool
	Darwin() bool
	Fuchsia() bool
	Windows() bool
	Debug() bool
	PrimaryArch() bool
	Platform() bool
	DeviceSpecific() bool
	SocSpecific() bool
	ProductSpecific() bool
	ProductServicesSpecific() bool
	AConfig() Config
	DeviceConfig() DeviceConfig
}

type BaseContext interface {
	BaseModuleContext
	androidBaseContext
}

// BaseModuleContext is the same as blueprint.BaseModuleContext except that Config() returns
// a Config instead of an interface{}.
type BaseModuleContext interface {
	ModuleName() string
	ModuleDir() string
	ModuleType() string
	Config() Config

	ContainsProperty(name string) bool
	Errorf(pos scanner.Position, fmt string, args ...interface{})
	ModuleErrorf(fmt string, args ...interface{})
	PropertyErrorf(property, fmt string, args ...interface{})
	Failed() bool

	// GlobWithDeps returns a list of files that match the specified pattern but do not match any
	// of the patterns in excludes.  It also adds efficient dependencies to rerun the primary
	// builder whenever a file matching the pattern as added or removed, without rerunning if a
	// file that does not match the pattern is added to a searched directory.
	GlobWithDeps(pattern string, excludes []string) ([]string, error)

	Fs() pathtools.FileSystem
	AddNinjaFileDeps(deps ...string)
}

type ModuleContext interface {
	androidBaseContext
	BaseModuleContext

	// Deprecated: use ModuleContext.Build instead.
	ModuleBuild(pctx PackageContext, params ModuleBuildParams)

	ExpandSources(srcFiles, excludes []string) Paths
	ExpandSource(srcFile, prop string) Path
	ExpandOptionalSource(srcFile *string, prop string) OptionalPath
	Glob(globPattern string, excludes []string) Paths
	GlobFiles(globPattern string, excludes []string) Paths

	InstallExecutable(installPath OutputPath, name string, srcPath Path, deps ...Path) OutputPath
	InstallFile(installPath OutputPath, name string, srcPath Path, deps ...Path) OutputPath
	InstallSymlink(installPath OutputPath, name string, srcPath OutputPath) OutputPath
	InstallAbsoluteSymlink(installPath OutputPath, name string, absPath string) OutputPath
	CheckbuildFile(srcPath Path)

	AddMissingDependencies(deps []string)

	InstallInData() bool
	InstallInTestcases() bool
	InstallInSanitizerDir() bool
	InstallInRecovery() bool

	RequiredModuleNames() []string

	// android.ModuleContext methods
	// These are duplicated instead of embedded so that can eventually be wrapped to take an
	// android.Module instead of a blueprint.Module
	OtherModuleName(m blueprint.Module) string
	OtherModuleErrorf(m blueprint.Module, fmt string, args ...interface{})
	OtherModuleDependencyTag(m blueprint.Module) blueprint.DependencyTag

	GetDirectDepWithTag(name string, tag blueprint.DependencyTag) blueprint.Module
	GetDirectDep(name string) (blueprint.Module, blueprint.DependencyTag)

	ModuleSubDir() string

	VisitDirectDepsBlueprint(visit func(blueprint.Module))
	VisitDirectDeps(visit func(Module))
	VisitDirectDepsWithTag(tag blueprint.DependencyTag, visit func(Module))
	VisitDirectDepsIf(pred func(Module) bool, visit func(Module))
	// Deprecated: use WalkDeps instead to support multiple dependency tags on the same module
	VisitDepsDepthFirst(visit func(Module))
	// Deprecated: use WalkDeps instead to support multiple dependency tags on the same module
	VisitDepsDepthFirstIf(pred func(Module) bool, visit func(Module))
	WalkDeps(visit func(Module, Module) bool)
	WalkDepsBlueprint(visit func(blueprint.Module, blueprint.Module) bool)

	Variable(pctx PackageContext, name, value string)
	Rule(pctx PackageContext, name string, params blueprint.RuleParams, argNames ...string) blueprint.Rule
	// Similar to blueprint.ModuleContext.Build, but takes Paths instead of []string,
	// and performs more verification.
	Build(pctx PackageContext, params BuildParams)

	PrimaryModule() Module
	FinalModule() Module
	VisitAllModuleVariants(visit func(Module))

	GetMissingDependencies() []string
	Namespace() blueprint.Namespace
}

type Module interface {
	blueprint.Module

	// GenerateAndroidBuildActions is analogous to Blueprints' GenerateBuildActions,
	// but GenerateAndroidBuildActions also has access to Android-specific information.
	// For more information, see Module.GenerateBuildActions within Blueprint's module_ctx.go
	GenerateAndroidBuildActions(ModuleContext)

	DepsMutator(BottomUpMutatorContext)

	base() *ModuleBase
	Enabled() bool
	Target() Target
	InstallInData() bool
	InstallInTestcases() bool
	InstallInSanitizerDir() bool
	InstallInRecovery() bool
	SkipInstall()
	ExportedToMake() bool
	NoticeFile() OptionalPath

	AddProperties(props ...interface{})
	GetProperties() []interface{}

	BuildParamsForTests() []BuildParams
	RuleParamsForTests() map[blueprint.Rule]blueprint.RuleParams
	VariablesForTests() map[string]string
}

type nameProperties struct {
	// The name of the module.  Must be unique across all modules.
	Name *string
}

type commonProperties struct {
	// emit build rules for this module
	Enabled *bool `android:"arch_variant"`

	// control whether this module compiles for 32-bit, 64-bit, or both.  Possible values
	// are "32" (compile for 32-bit only), "64" (compile for 64-bit only), "both" (compile for both
	// architectures), or "first" (compile for 64-bit on a 64-bit platform, and 32-bit on a 32-bit
	// platform
	Compile_multilib *string `android:"arch_variant"`

	Target struct {
		Host struct {
			Compile_multilib *string
		}
		Android struct {
			Compile_multilib *string
		}
	}

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

	// whether this module provides services owned by the OS provider to the core platform. When set
	// to true, it is installed into  /product_services (or /system/product_services if
	// product_services partition does not exist).
	Product_services_specific *bool

	// Whether this module is installed to recovery partition
	Recovery *bool

	// init.rc files to be installed if this module is installed
	Init_rc []string `android:"path"`

	// VINTF manifest fragments to be installed if this module is installed
	Vintf_fragments []string `android:"path"`

	// names of other modules to install if this module is installed
	Required []string `android:"arch_variant"`

	// relative path to a file to include in the list of notices for the device
	Notice *string `android:"path"`

	Dist struct {
		// copy the output of this module to the $DIST_DIR when `dist` is specified on the
		// command line and  any of these targets are also on the command line, or otherwise
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
	} `android:"arch_variant"`

	// Set by TargetMutator
	CompileTarget       Target   `blueprint:"mutated"`
	CompileMultiTargets []Target `blueprint:"mutated"`
	CompilePrimary      bool     `blueprint:"mutated"`

	// Set by InitAndroidModule
	HostOrDeviceSupported HostOrDeviceSupported `blueprint:"mutated"`
	ArchSpecific          bool                  `blueprint:"mutated"`

	SkipInstall bool `blueprint:"mutated"`

	NamespaceExportedToMake bool `blueprint:"mutated"`
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
	MultilibDefault     Multilib = ""
)

type HostOrDeviceSupported int

const (
	_ HostOrDeviceSupported = iota

	// Host and HostCross are built by default. Device is not supported.
	HostSupported

	// Host is built by default. HostCross and Device are not supported.
	HostSupportedNoCross

	// Device is built by default. Host and HostCross are not supported.
	DeviceSupported

	// Device is built by default. Host and HostCross are supported.
	HostAndDeviceSupported

	// Host, HostCross, and Device are built by default.
	HostAndDeviceDefault

	// Nothing is supported. This is not exposed to the user, but used to mark a
	// host only module as unsupported when the module type is not supported on
	// the host OS. E.g. benchmarks are supported on Linux but not Darwin.
	NeitherHostNorDeviceSupported
)

type moduleKind int

const (
	platformModule moduleKind = iota
	deviceSpecificModule
	socSpecificModule
	productSpecificModule
	productServicesSpecificModule
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
	case productServicesSpecificModule:
		return "productservices-specific"
	default:
		panic(fmt.Errorf("unknown module kind %d", k))
	}
}

func InitAndroidModule(m Module) {
	base := m.base()
	base.module = m

	m.AddProperties(
		&base.nameProperties,
		&base.commonProperties,
		&base.variableProperties)
	base.generalProperties = m.GetProperties()
	base.customizableProperties = m.GetProperties()
}

func InitAndroidArchModule(m Module, hod HostOrDeviceSupported, defaultMultilib Multilib) {
	InitAndroidModule(m)

	base := m.base()
	base.commonProperties.HostOrDeviceSupported = hod
	base.commonProperties.Default_multilib = string(defaultMultilib)
	base.commonProperties.ArchSpecific = true
	base.commonProperties.UseTargetVariants = true

	switch hod {
	case HostAndDeviceSupported, HostAndDeviceDefault:
		m.AddProperties(&base.hostAndDeviceProperties)
	}

	InitArchModule(m)
}

func InitAndroidMultiTargetsArchModule(m Module, hod HostOrDeviceSupported, defaultMultilib Multilib) {
	InitAndroidArchModule(m, hod, defaultMultilib)
	m.base().commonProperties.UseTargetVariants = false
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
// that is to be built. GenerateAndroidBuildActions is passed a
// AndroidModuleContext rather than the usual blueprint.ModuleContext.
// AndroidModuleContext exposes extra functionality specific to the Android build
// system including details about the particular build variant that is to be
// generated.
//
// For example:
//
//     import (
//         "android/soong/android"
//     )
//
//     type myModule struct {
//         android.ModuleBase
//         properties struct {
//             MyProperty string
//         }
//     }
//
//     func NewMyModule() android.Module) {
//         m := &myModule{}
//         m.AddProperties(&m.properties)
//         android.InitAndroidModule(m)
//         return m
//     }
//
//     func (m *myModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
//         // Get the CPU architecture for the current build variant.
//         variantArch := ctx.Arch()
//
//         // ...
//     }
type ModuleBase struct {
	// Putting the curiously recurring thing pointing to the thing that contains
	// the thing pattern to good use.
	// TODO: remove this
	module Module

	nameProperties          nameProperties
	commonProperties        commonProperties
	variableProperties      variableProperties
	hostAndDeviceProperties hostAndDeviceProperties
	generalProperties       []interface{}
	archProperties          [][]interface{}
	customizableProperties  []interface{}

	noAddressSanitizer bool
	installFiles       Paths
	checkbuildFiles    Paths
	noticeFile         OptionalPath

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

	prefer32 func(ctx BaseModuleContext, base *ModuleBase, class OsClass) bool
}

func (a *ModuleBase) DepsMutator(BottomUpMutatorContext) {}

func (a *ModuleBase) AddProperties(props ...interface{}) {
	a.registerProps = append(a.registerProps, props...)
}

func (a *ModuleBase) GetProperties() []interface{} {
	return a.registerProps
}

func (a *ModuleBase) BuildParamsForTests() []BuildParams {
	return a.buildParams
}

func (a *ModuleBase) RuleParamsForTests() map[blueprint.Rule]blueprint.RuleParams {
	return a.ruleParams
}

func (a *ModuleBase) VariablesForTests() map[string]string {
	return a.variables
}

func (a *ModuleBase) Prefer32(prefer32 func(ctx BaseModuleContext, base *ModuleBase, class OsClass) bool) {
	a.prefer32 = prefer32
}

// Name returns the name of the module.  It may be overridden by individual module types, for
// example prebuilts will prepend prebuilt_ to the name.
func (a *ModuleBase) Name() string {
	return String(a.nameProperties.Name)
}

// BaseModuleName returns the name of the module as specified in the blueprints file.
func (a *ModuleBase) BaseModuleName() string {
	return String(a.nameProperties.Name)
}

func (a *ModuleBase) base() *ModuleBase {
	return a
}

func (a *ModuleBase) SetTarget(target Target, multiTargets []Target, primary bool) {
	a.commonProperties.CompileTarget = target
	a.commonProperties.CompileMultiTargets = multiTargets
	a.commonProperties.CompilePrimary = primary
}

func (a *ModuleBase) Target() Target {
	return a.commonProperties.CompileTarget
}

func (a *ModuleBase) TargetPrimary() bool {
	return a.commonProperties.CompilePrimary
}

func (a *ModuleBase) MultiTargets() []Target {
	return a.commonProperties.CompileMultiTargets
}

func (a *ModuleBase) Os() OsType {
	return a.Target().Os
}

func (a *ModuleBase) Host() bool {
	return a.Os().Class == Host || a.Os().Class == HostCross
}

func (a *ModuleBase) Arch() Arch {
	return a.Target().Arch
}

func (a *ModuleBase) ArchSpecific() bool {
	return a.commonProperties.ArchSpecific
}

func (a *ModuleBase) OsClassSupported() []OsClass {
	switch a.commonProperties.HostOrDeviceSupported {
	case HostSupported:
		return []OsClass{Host, HostCross}
	case HostSupportedNoCross:
		return []OsClass{Host}
	case DeviceSupported:
		return []OsClass{Device}
	case HostAndDeviceSupported, HostAndDeviceDefault:
		var supported []OsClass
		if Bool(a.hostAndDeviceProperties.Host_supported) ||
			(a.commonProperties.HostOrDeviceSupported == HostAndDeviceDefault &&
				a.hostAndDeviceProperties.Host_supported == nil) {
			supported = append(supported, Host, HostCross)
		}
		if a.hostAndDeviceProperties.Device_supported == nil ||
			*a.hostAndDeviceProperties.Device_supported {
			supported = append(supported, Device)
		}
		return supported
	default:
		return nil
	}
}

func (a *ModuleBase) DeviceSupported() bool {
	return a.commonProperties.HostOrDeviceSupported == DeviceSupported ||
		a.commonProperties.HostOrDeviceSupported == HostAndDeviceSupported &&
			(a.hostAndDeviceProperties.Device_supported == nil ||
				*a.hostAndDeviceProperties.Device_supported)
}

func (a *ModuleBase) Platform() bool {
	return !a.DeviceSpecific() && !a.SocSpecific() && !a.ProductSpecific() && !a.ProductServicesSpecific()
}

func (a *ModuleBase) DeviceSpecific() bool {
	return Bool(a.commonProperties.Device_specific)
}

func (a *ModuleBase) SocSpecific() bool {
	return Bool(a.commonProperties.Vendor) || Bool(a.commonProperties.Proprietary) || Bool(a.commonProperties.Soc_specific)
}

func (a *ModuleBase) ProductSpecific() bool {
	return Bool(a.commonProperties.Product_specific)
}

func (a *ModuleBase) ProductServicesSpecific() bool {
	return Bool(a.commonProperties.Product_services_specific)
}

func (a *ModuleBase) Enabled() bool {
	if a.commonProperties.Enabled == nil {
		return !a.Os().DefaultDisabled
	}
	return *a.commonProperties.Enabled
}

func (a *ModuleBase) SkipInstall() {
	a.commonProperties.SkipInstall = true
}

func (a *ModuleBase) ExportedToMake() bool {
	return a.commonProperties.NamespaceExportedToMake
}

func (a *ModuleBase) computeInstallDeps(
	ctx blueprint.ModuleContext) Paths {

	result := Paths{}
	// TODO(ccross): we need to use WalkDeps and have some way to know which dependencies require installation
	ctx.VisitDepsDepthFirstIf(isFileInstaller,
		func(m blueprint.Module) {
			fileInstaller := m.(fileInstaller)
			files := fileInstaller.filesToInstall()
			result = append(result, files...)
		})

	return result
}

func (a *ModuleBase) filesToInstall() Paths {
	return a.installFiles
}

func (p *ModuleBase) NoAddressSanitizer() bool {
	return p.noAddressSanitizer
}

func (p *ModuleBase) InstallInData() bool {
	return false
}

func (p *ModuleBase) InstallInTestcases() bool {
	return false
}

func (p *ModuleBase) InstallInSanitizerDir() bool {
	return false
}

func (p *ModuleBase) InstallInRecovery() bool {
	return Bool(p.commonProperties.Recovery)
}

func (a *ModuleBase) Owner() string {
	return String(a.commonProperties.Owner)
}

func (a *ModuleBase) NoticeFile() OptionalPath {
	return a.noticeFile
}

func (a *ModuleBase) generateModuleTarget(ctx ModuleContext) {
	allInstalledFiles := Paths{}
	allCheckbuildFiles := Paths{}
	ctx.VisitAllModuleVariants(func(module Module) {
		a := module.base()
		allInstalledFiles = append(allInstalledFiles, a.installFiles...)
		allCheckbuildFiles = append(allCheckbuildFiles, a.checkbuildFiles...)
	})

	var deps Paths

	namespacePrefix := ctx.Namespace().(*Namespace).id
	if namespacePrefix != "" {
		namespacePrefix = namespacePrefix + "-"
	}

	if len(allInstalledFiles) > 0 {
		name := PathForPhony(ctx, namespacePrefix+ctx.ModuleName()+"-install")
		ctx.Build(pctx, BuildParams{
			Rule:      blueprint.Phony,
			Output:    name,
			Implicits: allInstalledFiles,
			Default:   !ctx.Config().EmbeddedInMake(),
		})
		deps = append(deps, name)
		a.installTarget = name
	}

	if len(allCheckbuildFiles) > 0 {
		name := PathForPhony(ctx, namespacePrefix+ctx.ModuleName()+"-checkbuild")
		ctx.Build(pctx, BuildParams{
			Rule:      blueprint.Phony,
			Output:    name,
			Implicits: allCheckbuildFiles,
		})
		deps = append(deps, name)
		a.checkbuildTarget = name
	}

	if len(deps) > 0 {
		suffix := ""
		if ctx.Config().EmbeddedInMake() {
			suffix = "-soong"
		}

		name := PathForPhony(ctx, namespacePrefix+ctx.ModuleName()+suffix)
		ctx.Build(pctx, BuildParams{
			Rule:      blueprint.Phony,
			Outputs:   []WritablePath{name},
			Implicits: deps,
		})

		a.blueprintDir = ctx.ModuleDir()
	}
}

func determineModuleKind(a *ModuleBase, ctx blueprint.BaseModuleContext) moduleKind {
	var socSpecific = Bool(a.commonProperties.Vendor) || Bool(a.commonProperties.Proprietary) || Bool(a.commonProperties.Soc_specific)
	var deviceSpecific = Bool(a.commonProperties.Device_specific)
	var productSpecific = Bool(a.commonProperties.Product_specific)
	var productServicesSpecific = Bool(a.commonProperties.Product_services_specific)

	msg := "conflicting value set here"
	if socSpecific && deviceSpecific {
		ctx.PropertyErrorf("device_specific", "a module cannot be specific to SoC and device at the same time.")
		if Bool(a.commonProperties.Vendor) {
			ctx.PropertyErrorf("vendor", msg)
		}
		if Bool(a.commonProperties.Proprietary) {
			ctx.PropertyErrorf("proprietary", msg)
		}
		if Bool(a.commonProperties.Soc_specific) {
			ctx.PropertyErrorf("soc_specific", msg)
		}
	}

	if productSpecific && productServicesSpecific {
		ctx.PropertyErrorf("product_specific", "a module cannot be specific to product and product_services at the same time.")
		ctx.PropertyErrorf("product_services_specific", msg)
	}

	if (socSpecific || deviceSpecific) && (productSpecific || productServicesSpecific) {
		if productSpecific {
			ctx.PropertyErrorf("product_specific", "a module cannot be specific to SoC or device and product at the same time.")
		} else {
			ctx.PropertyErrorf("product_services_specific", "a module cannot be specific to SoC or device and product_services at the same time.")
		}
		if deviceSpecific {
			ctx.PropertyErrorf("device_specific", msg)
		} else {
			if Bool(a.commonProperties.Vendor) {
				ctx.PropertyErrorf("vendor", msg)
			}
			if Bool(a.commonProperties.Proprietary) {
				ctx.PropertyErrorf("proprietary", msg)
			}
			if Bool(a.commonProperties.Soc_specific) {
				ctx.PropertyErrorf("soc_specific", msg)
			}
		}
	}

	if productSpecific {
		return productSpecificModule
	} else if productServicesSpecific {
		return productServicesSpecificModule
	} else if deviceSpecific {
		return deviceSpecificModule
	} else if socSpecific {
		return socSpecificModule
	} else {
		return platformModule
	}
}

func (a *ModuleBase) androidBaseContextFactory(ctx blueprint.BaseModuleContext) androidBaseContextImpl {
	return androidBaseContextImpl{
		target:        a.commonProperties.CompileTarget,
		targetPrimary: a.commonProperties.CompilePrimary,
		multiTargets:  a.commonProperties.CompileMultiTargets,
		kind:          determineModuleKind(a, ctx),
		config:        ctx.Config().(Config),
	}
}

func (a *ModuleBase) GenerateBuildActions(blueprintCtx blueprint.ModuleContext) {
	ctx := &androidModuleContext{
		module:                 a.module,
		ModuleContext:          blueprintCtx,
		androidBaseContextImpl: a.androidBaseContextFactory(blueprintCtx),
		installDeps:            a.computeInstallDeps(blueprintCtx),
		installFiles:           a.installFiles,
		missingDeps:            blueprintCtx.GetMissingDependencies(),
		variables:              make(map[string]string),
	}

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

	ctx.Variable(pctx, "moduleDesc", desc)

	s := ""
	if len(suffix) > 0 {
		s = " [" + strings.Join(suffix, " ") + "]"
	}
	ctx.Variable(pctx, "moduleDescSuffix", s)

	// Some common property checks for properties that will be used later in androidmk.go
	if a.commonProperties.Dist.Dest != nil {
		_, err := validateSafePath(*a.commonProperties.Dist.Dest)
		if err != nil {
			ctx.PropertyErrorf("dist.dest", "%s", err.Error())
		}
	}
	if a.commonProperties.Dist.Dir != nil {
		_, err := validateSafePath(*a.commonProperties.Dist.Dir)
		if err != nil {
			ctx.PropertyErrorf("dist.dir", "%s", err.Error())
		}
	}
	if a.commonProperties.Dist.Suffix != nil {
		if strings.Contains(*a.commonProperties.Dist.Suffix, "/") {
			ctx.PropertyErrorf("dist.suffix", "Suffix may not contain a '/' character.")
		}
	}

	if a.Enabled() {
		notice := proptools.StringDefault(a.commonProperties.Notice, "NOTICE")
		if m := SrcIsModule(notice); m != "" {
			a.noticeFile = ctx.ExpandOptionalSource(&notice, "notice")
		} else {
			noticePath := filepath.Join(ctx.ModuleDir(), notice)
			a.noticeFile = ExistentPathForSource(ctx, noticePath)
		}

		a.module.GenerateAndroidBuildActions(ctx)
		if ctx.Failed() {
			return
		}

		a.installFiles = append(a.installFiles, ctx.installFiles...)
		a.checkbuildFiles = append(a.checkbuildFiles, ctx.checkbuildFiles...)
	}

	if a == ctx.FinalModule().(Module).base() {
		a.generateModuleTarget(ctx)
		if ctx.Failed() {
			return
		}
	}

	a.buildParams = ctx.buildParams
	a.ruleParams = ctx.ruleParams
	a.variables = ctx.variables
}

type androidBaseContextImpl struct {
	target        Target
	multiTargets  []Target
	targetPrimary bool
	debug         bool
	kind          moduleKind
	config        Config
}

type androidModuleContext struct {
	blueprint.ModuleContext
	androidBaseContextImpl
	installDeps     Paths
	installFiles    Paths
	checkbuildFiles Paths
	missingDeps     []string
	module          Module

	// For tests
	buildParams []BuildParams
	ruleParams  map[blueprint.Rule]blueprint.RuleParams
	variables   map[string]string
}

func (a *androidModuleContext) ninjaError(desc string, outputs []string, err error) {
	a.ModuleContext.Build(pctx.PackageContext, blueprint.BuildParams{
		Rule:        ErrorRule,
		Description: desc,
		Outputs:     outputs,
		Optional:    true,
		Args: map[string]string{
			"error": err.Error(),
		},
	})
	return
}

func (a *androidModuleContext) Config() Config {
	return a.ModuleContext.Config().(Config)
}

func (a *androidModuleContext) ModuleBuild(pctx PackageContext, params ModuleBuildParams) {
	a.Build(pctx, BuildParams(params))
}

func convertBuildParams(params BuildParams) blueprint.BuildParams {
	bparams := blueprint.BuildParams{
		Rule:            params.Rule,
		Description:     params.Description,
		Deps:            params.Deps,
		Outputs:         params.Outputs.Strings(),
		ImplicitOutputs: params.ImplicitOutputs.Strings(),
		Inputs:          params.Inputs.Strings(),
		Implicits:       params.Implicits.Strings(),
		OrderOnly:       params.OrderOnly.Strings(),
		Args:            params.Args,
		Optional:        !params.Default,
	}

	if params.Depfile != nil {
		bparams.Depfile = params.Depfile.String()
	}
	if params.Output != nil {
		bparams.Outputs = append(bparams.Outputs, params.Output.String())
	}
	if params.ImplicitOutput != nil {
		bparams.ImplicitOutputs = append(bparams.ImplicitOutputs, params.ImplicitOutput.String())
	}
	if params.Input != nil {
		bparams.Inputs = append(bparams.Inputs, params.Input.String())
	}
	if params.Implicit != nil {
		bparams.Implicits = append(bparams.Implicits, params.Implicit.String())
	}

	bparams.Outputs = proptools.NinjaEscapeList(bparams.Outputs)
	bparams.ImplicitOutputs = proptools.NinjaEscapeList(bparams.ImplicitOutputs)
	bparams.Inputs = proptools.NinjaEscapeList(bparams.Inputs)
	bparams.Implicits = proptools.NinjaEscapeList(bparams.Implicits)
	bparams.OrderOnly = proptools.NinjaEscapeList(bparams.OrderOnly)
	bparams.Depfile = proptools.NinjaEscapeList([]string{bparams.Depfile})[0]

	return bparams
}

func (a *androidModuleContext) Variable(pctx PackageContext, name, value string) {
	if a.config.captureBuild {
		a.variables[name] = value
	}

	a.ModuleContext.Variable(pctx.PackageContext, name, value)
}

func (a *androidModuleContext) Rule(pctx PackageContext, name string, params blueprint.RuleParams,
	argNames ...string) blueprint.Rule {

	rule := a.ModuleContext.Rule(pctx.PackageContext, name, params, argNames...)

	if a.config.captureBuild {
		a.ruleParams[rule] = params
	}

	return rule
}

func (a *androidModuleContext) Build(pctx PackageContext, params BuildParams) {
	if a.config.captureBuild {
		a.buildParams = append(a.buildParams, params)
	}

	bparams := convertBuildParams(params)

	if bparams.Description != "" {
		bparams.Description = "${moduleDesc}" + params.Description + "${moduleDescSuffix}"
	}

	if a.missingDeps != nil {
		a.ninjaError(bparams.Description, bparams.Outputs,
			fmt.Errorf("module %s missing dependencies: %s\n",
				a.ModuleName(), strings.Join(a.missingDeps, ", ")))
		return
	}

	a.ModuleContext.Build(pctx.PackageContext, bparams)
}

func (a *androidModuleContext) GetMissingDependencies() []string {
	return a.missingDeps
}

func (a *androidModuleContext) AddMissingDependencies(deps []string) {
	if deps != nil {
		a.missingDeps = append(a.missingDeps, deps...)
		a.missingDeps = FirstUniqueStrings(a.missingDeps)
	}
}

func (a *androidModuleContext) validateAndroidModule(module blueprint.Module) Module {
	aModule, _ := module.(Module)
	if aModule == nil {
		a.ModuleErrorf("module %q not an android module", a.OtherModuleName(aModule))
		return nil
	}

	if !aModule.Enabled() {
		if a.Config().AllowMissingDependencies() {
			a.AddMissingDependencies([]string{a.OtherModuleName(aModule)})
		} else {
			a.ModuleErrorf("depends on disabled module %q", a.OtherModuleName(aModule))
		}
		return nil
	}

	return aModule
}

func (a *androidModuleContext) getDirectDepInternal(name string, tag blueprint.DependencyTag) (blueprint.Module, blueprint.DependencyTag) {
	type dep struct {
		mod blueprint.Module
		tag blueprint.DependencyTag
	}
	var deps []dep
	a.VisitDirectDepsBlueprint(func(m blueprint.Module) {
		if aModule, _ := m.(Module); aModule != nil && aModule.base().BaseModuleName() == name {
			returnedTag := a.ModuleContext.OtherModuleDependencyTag(aModule)
			if tag == nil || returnedTag == tag {
				deps = append(deps, dep{aModule, returnedTag})
			}
		}
	})
	if len(deps) == 1 {
		return deps[0].mod, deps[0].tag
	} else if len(deps) >= 2 {
		panic(fmt.Errorf("Multiple dependencies having same BaseModuleName() %q found from %q",
			name, a.ModuleName()))
	} else {
		return nil, nil
	}
}

func (a *androidModuleContext) GetDirectDepWithTag(name string, tag blueprint.DependencyTag) blueprint.Module {
	m, _ := a.getDirectDepInternal(name, tag)
	return m
}

func (a *androidModuleContext) GetDirectDep(name string) (blueprint.Module, blueprint.DependencyTag) {
	return a.getDirectDepInternal(name, nil)
}

func (a *androidModuleContext) VisitDirectDepsBlueprint(visit func(blueprint.Module)) {
	a.ModuleContext.VisitDirectDeps(visit)
}

func (a *androidModuleContext) VisitDirectDeps(visit func(Module)) {
	a.ModuleContext.VisitDirectDeps(func(module blueprint.Module) {
		if aModule := a.validateAndroidModule(module); aModule != nil {
			visit(aModule)
		}
	})
}

func (a *androidModuleContext) VisitDirectDepsWithTag(tag blueprint.DependencyTag, visit func(Module)) {
	a.ModuleContext.VisitDirectDeps(func(module blueprint.Module) {
		if aModule := a.validateAndroidModule(module); aModule != nil {
			if a.ModuleContext.OtherModuleDependencyTag(aModule) == tag {
				visit(aModule)
			}
		}
	})
}

func (a *androidModuleContext) VisitDirectDepsIf(pred func(Module) bool, visit func(Module)) {
	a.ModuleContext.VisitDirectDepsIf(
		// pred
		func(module blueprint.Module) bool {
			if aModule := a.validateAndroidModule(module); aModule != nil {
				return pred(aModule)
			} else {
				return false
			}
		},
		// visit
		func(module blueprint.Module) {
			visit(module.(Module))
		})
}

func (a *androidModuleContext) VisitDepsDepthFirst(visit func(Module)) {
	a.ModuleContext.VisitDepsDepthFirst(func(module blueprint.Module) {
		if aModule := a.validateAndroidModule(module); aModule != nil {
			visit(aModule)
		}
	})
}

func (a *androidModuleContext) VisitDepsDepthFirstIf(pred func(Module) bool, visit func(Module)) {
	a.ModuleContext.VisitDepsDepthFirstIf(
		// pred
		func(module blueprint.Module) bool {
			if aModule := a.validateAndroidModule(module); aModule != nil {
				return pred(aModule)
			} else {
				return false
			}
		},
		// visit
		func(module blueprint.Module) {
			visit(module.(Module))
		})
}

func (a *androidModuleContext) WalkDepsBlueprint(visit func(blueprint.Module, blueprint.Module) bool) {
	a.ModuleContext.WalkDeps(visit)
}

func (a *androidModuleContext) WalkDeps(visit func(Module, Module) bool) {
	a.ModuleContext.WalkDeps(func(child, parent blueprint.Module) bool {
		childAndroidModule := a.validateAndroidModule(child)
		parentAndroidModule := a.validateAndroidModule(parent)
		if childAndroidModule != nil && parentAndroidModule != nil {
			return visit(childAndroidModule, parentAndroidModule)
		} else {
			return false
		}
	})
}

func (a *androidModuleContext) VisitAllModuleVariants(visit func(Module)) {
	a.ModuleContext.VisitAllModuleVariants(func(module blueprint.Module) {
		visit(module.(Module))
	})
}

func (a *androidModuleContext) PrimaryModule() Module {
	return a.ModuleContext.PrimaryModule().(Module)
}

func (a *androidModuleContext) FinalModule() Module {
	return a.ModuleContext.FinalModule().(Module)
}

func (a *androidBaseContextImpl) Target() Target {
	return a.target
}

func (a *androidBaseContextImpl) TargetPrimary() bool {
	return a.targetPrimary
}

func (a *androidBaseContextImpl) MultiTargets() []Target {
	return a.multiTargets
}

func (a *androidBaseContextImpl) Arch() Arch {
	return a.target.Arch
}

func (a *androidBaseContextImpl) Os() OsType {
	return a.target.Os
}

func (a *androidBaseContextImpl) Host() bool {
	return a.target.Os.Class == Host || a.target.Os.Class == HostCross
}

func (a *androidBaseContextImpl) Device() bool {
	return a.target.Os.Class == Device
}

func (a *androidBaseContextImpl) Darwin() bool {
	return a.target.Os == Darwin
}

func (a *androidBaseContextImpl) Fuchsia() bool {
	return a.target.Os == Fuchsia
}

func (a *androidBaseContextImpl) Windows() bool {
	return a.target.Os == Windows
}

func (a *androidBaseContextImpl) Debug() bool {
	return a.debug
}

func (a *androidBaseContextImpl) PrimaryArch() bool {
	if len(a.config.Targets[a.target.Os]) <= 1 {
		return true
	}
	return a.target.Arch.ArchType == a.config.Targets[a.target.Os][0].Arch.ArchType
}

func (a *androidBaseContextImpl) AConfig() Config {
	return a.config
}

func (a *androidBaseContextImpl) DeviceConfig() DeviceConfig {
	return DeviceConfig{a.config.deviceConfig}
}

func (a *androidBaseContextImpl) Platform() bool {
	return a.kind == platformModule
}

func (a *androidBaseContextImpl) DeviceSpecific() bool {
	return a.kind == deviceSpecificModule
}

func (a *androidBaseContextImpl) SocSpecific() bool {
	return a.kind == socSpecificModule
}

func (a *androidBaseContextImpl) ProductSpecific() bool {
	return a.kind == productSpecificModule
}

func (a *androidBaseContextImpl) ProductServicesSpecific() bool {
	return a.kind == productServicesSpecificModule
}

// Makes this module a platform module, i.e. not specific to soc, device,
// product, or product_services.
func (a *ModuleBase) MakeAsPlatform() {
	a.commonProperties.Vendor = boolPtr(false)
	a.commonProperties.Proprietary = boolPtr(false)
	a.commonProperties.Soc_specific = boolPtr(false)
	a.commonProperties.Product_specific = boolPtr(false)
	a.commonProperties.Product_services_specific = boolPtr(false)
}

func (a *androidModuleContext) InstallInData() bool {
	return a.module.InstallInData()
}

func (a *androidModuleContext) InstallInTestcases() bool {
	return a.module.InstallInTestcases()
}

func (a *androidModuleContext) InstallInSanitizerDir() bool {
	return a.module.InstallInSanitizerDir()
}

func (a *androidModuleContext) InstallInRecovery() bool {
	return a.module.InstallInRecovery()
}

func (a *androidModuleContext) skipInstall(fullInstallPath OutputPath) bool {
	if a.module.base().commonProperties.SkipInstall {
		return true
	}

	// We'll need a solution for choosing which of modules with the same name in different
	// namespaces to install.  For now, reuse the list of namespaces exported to Make as the
	// list of namespaces to install in a Soong-only build.
	if !a.module.base().commonProperties.NamespaceExportedToMake {
		return true
	}

	if a.Device() {
		if a.Config().SkipDeviceInstall() {
			return true
		}

		if a.Config().SkipMegaDeviceInstall(fullInstallPath.String()) {
			return true
		}
	}

	return false
}

func (a *androidModuleContext) InstallFile(installPath OutputPath, name string, srcPath Path,
	deps ...Path) OutputPath {
	return a.installFile(installPath, name, srcPath, Cp, deps)
}

func (a *androidModuleContext) InstallExecutable(installPath OutputPath, name string, srcPath Path,
	deps ...Path) OutputPath {
	return a.installFile(installPath, name, srcPath, CpExecutable, deps)
}

func (a *androidModuleContext) installFile(installPath OutputPath, name string, srcPath Path,
	rule blueprint.Rule, deps []Path) OutputPath {

	fullInstallPath := installPath.Join(a, name)
	a.module.base().hooks.runInstallHooks(a, fullInstallPath, false)

	if !a.skipInstall(fullInstallPath) {

		deps = append(deps, a.installDeps...)

		var implicitDeps, orderOnlyDeps Paths

		if a.Host() {
			// Installed host modules might be used during the build, depend directly on their
			// dependencies so their timestamp is updated whenever their dependency is updated
			implicitDeps = deps
		} else {
			orderOnlyDeps = deps
		}

		a.Build(pctx, BuildParams{
			Rule:        rule,
			Description: "install " + fullInstallPath.Base(),
			Output:      fullInstallPath,
			Input:       srcPath,
			Implicits:   implicitDeps,
			OrderOnly:   orderOnlyDeps,
			Default:     !a.Config().EmbeddedInMake(),
		})

		a.installFiles = append(a.installFiles, fullInstallPath)
	}
	a.checkbuildFiles = append(a.checkbuildFiles, srcPath)
	return fullInstallPath
}

func (a *androidModuleContext) InstallSymlink(installPath OutputPath, name string, srcPath OutputPath) OutputPath {
	fullInstallPath := installPath.Join(a, name)
	a.module.base().hooks.runInstallHooks(a, fullInstallPath, true)

	if !a.skipInstall(fullInstallPath) {

		relPath, err := filepath.Rel(path.Dir(fullInstallPath.String()), srcPath.String())
		if err != nil {
			panic(fmt.Sprintf("Unable to generate symlink between %q and %q: %s", fullInstallPath.Base(), srcPath.Base(), err))
		}
		a.Build(pctx, BuildParams{
			Rule:        Symlink,
			Description: "install symlink " + fullInstallPath.Base(),
			Output:      fullInstallPath,
			OrderOnly:   Paths{srcPath},
			Default:     !a.Config().EmbeddedInMake(),
			Args: map[string]string{
				"fromPath": relPath,
			},
		})

		a.installFiles = append(a.installFiles, fullInstallPath)
		a.checkbuildFiles = append(a.checkbuildFiles, srcPath)
	}
	return fullInstallPath
}

// installPath/name -> absPath where absPath might be a path that is available only at runtime
// (e.g. /apex/...)
func (a *androidModuleContext) InstallAbsoluteSymlink(installPath OutputPath, name string, absPath string) OutputPath {
	fullInstallPath := installPath.Join(a, name)
	a.module.base().hooks.runInstallHooks(a, fullInstallPath, true)

	if !a.skipInstall(fullInstallPath) {
		a.Build(pctx, BuildParams{
			Rule:        Symlink,
			Description: "install symlink " + fullInstallPath.Base() + " -> " + absPath,
			Output:      fullInstallPath,
			Default:     !a.Config().EmbeddedInMake(),
			Args: map[string]string{
				"fromPath": absPath,
			},
		})

		a.installFiles = append(a.installFiles, fullInstallPath)
	}
	return fullInstallPath
}

func (a *androidModuleContext) CheckbuildFile(srcPath Path) {
	a.checkbuildFiles = append(a.checkbuildFiles, srcPath)
}

type fileInstaller interface {
	filesToInstall() Paths
}

func isFileInstaller(m blueprint.Module) bool {
	_, ok := m.(fileInstaller)
	return ok
}

func isAndroidModule(m blueprint.Module) bool {
	_, ok := m.(Module)
	return ok
}

func findStringInSlice(str string, slice []string) int {
	for i, s := range slice {
		if s == str {
			return i
		}
	}
	return -1
}

func SrcIsModule(s string) string {
	if len(s) > 1 && s[0] == ':' {
		return s[1:]
	}
	return ""
}

type sourceDependencyTag struct {
	blueprint.BaseDependencyTag
}

var SourceDepTag sourceDependencyTag

// Adds necessary dependencies to satisfy filegroup or generated sources modules listed in srcFiles
// using ":module" syntax, if any.
//
// Deprecated: tag the property with `android:"path"` instead.
func ExtractSourcesDeps(ctx BottomUpMutatorContext, srcFiles []string) {
	var deps []string
	set := make(map[string]bool)

	for _, s := range srcFiles {
		if m := SrcIsModule(s); m != "" {
			if _, found := set[m]; found {
				ctx.ModuleErrorf("found source dependency duplicate: %q!", m)
			} else {
				set[m] = true
				deps = append(deps, m)
			}
		}
	}

	ctx.AddDependency(ctx.Module(), SourceDepTag, deps...)
}

// Adds necessary dependencies to satisfy filegroup or generated sources modules specified in s
// using ":module" syntax, if any.
//
// Deprecated: tag the property with `android:"path"` instead.
func ExtractSourceDeps(ctx BottomUpMutatorContext, s *string) {
	if s != nil {
		if m := SrcIsModule(*s); m != "" {
			ctx.AddDependency(ctx.Module(), SourceDepTag, m)
		}
	}
}

type SourceFileProducer interface {
	Srcs() Paths
}

type HostToolProvider interface {
	HostToolPath() OptionalPath
}

// Returns a list of paths expanded from globs and modules referenced using ":module" syntax.  The property must
// be tagged with `android:"path" to support automatic source module dependency resolution.
//
// Deprecated: use PathsForModuleSrc or PathsForModuleSrcExcludes instead.
func (ctx *androidModuleContext) ExpandSources(srcFiles, excludes []string) Paths {
	return PathsForModuleSrcExcludes(ctx, srcFiles, excludes)
}

// Returns a single path expanded from globs and modules referenced using ":module" syntax.  The property must
// be tagged with `android:"path" to support automatic source module dependency resolution.
//
// Deprecated: use PathForModuleSrc instead.
func (ctx *androidModuleContext) ExpandSource(srcFile, prop string) Path {
	return PathForModuleSrc(ctx, srcFile)
}

// Returns an optional single path expanded from globs and modules referenced using ":module" syntax if
// the srcFile is non-nil.  The property must be tagged with `android:"path" to support automatic source module
// dependency resolution.
func (ctx *androidModuleContext) ExpandOptionalSource(srcFile *string, prop string) OptionalPath {
	if srcFile != nil {
		return OptionalPathForPath(PathForModuleSrc(ctx, *srcFile))
	}
	return OptionalPath{}
}

func (ctx *androidModuleContext) RequiredModuleNames() []string {
	return ctx.module.base().commonProperties.Required
}

func (ctx *androidModuleContext) Glob(globPattern string, excludes []string) Paths {
	ret, err := ctx.GlobWithDeps(globPattern, excludes)
	if err != nil {
		ctx.ModuleErrorf("glob: %s", err.Error())
	}
	return pathsForModuleSrcFromFullPath(ctx, ret, true)
}

func (ctx *androidModuleContext) GlobFiles(globPattern string, excludes []string) Paths {
	ret, err := ctx.GlobWithDeps(globPattern, excludes)
	if err != nil {
		ctx.ModuleErrorf("glob: %s", err.Error())
	}
	return pathsForModuleSrcFromFullPath(ctx, ret, false)
}

func init() {
	RegisterSingletonType("buildtarget", BuildTargetSingleton)
}

func BuildTargetSingleton() Singleton {
	return &buildTargetSingleton{}
}

func parentDir(dir string) string {
	dir, _ = filepath.Split(dir)
	return filepath.Clean(dir)
}

type buildTargetSingleton struct{}

func (c *buildTargetSingleton) GenerateBuildActions(ctx SingletonContext) {
	var checkbuildDeps Paths

	mmTarget := func(dir string) WritablePath {
		return PathForPhony(ctx,
			"MODULES-IN-"+strings.Replace(filepath.Clean(dir), "/", "-", -1))
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
	if ctx.Config().EmbeddedInMake() {
		suffix = "-soong"
	}

	// Create a top-level checkbuild target that depends on all modules
	ctx.Build(pctx, BuildParams{
		Rule:      blueprint.Phony,
		Output:    PathForPhony(ctx, "checkbuild"+suffix),
		Implicits: checkbuildDeps,
	})

	// Make will generate the MODULES-IN-* targets
	if ctx.Config().EmbeddedInMake() {
		return
	}

	sortedKeys := func(m map[string]Paths) []string {
		s := make([]string, 0, len(m))
		for k := range m {
			s = append(s, k)
		}
		sort.Strings(s)
		return s
	}

	// Ensure ancestor directories are in modulesInDir
	dirs := sortedKeys(modulesInDir)
	for _, dir := range dirs {
		dir := parentDir(dir)
		for dir != "." && dir != "/" {
			if _, exists := modulesInDir[dir]; exists {
				break
			}
			modulesInDir[dir] = nil
			dir = parentDir(dir)
		}
	}

	// Make directories build their direct subdirectories
	dirs = sortedKeys(modulesInDir)
	for _, dir := range dirs {
		p := parentDir(dir)
		if p != "." && p != "/" {
			modulesInDir[p] = append(modulesInDir[p], mmTarget(dir))
		}
	}

	// Create a MODULES-IN-<directory> target that depends on all modules in a directory, and
	// depends on the MODULES-IN-* targets of all of its subdirectories that contain Android.bp
	// files.
	for _, dir := range dirs {
		ctx.Build(pctx, BuildParams{
			Rule:      blueprint.Phony,
			Output:    mmTarget(dir),
			Implicits: modulesInDir[dir],
			// HACK: checkbuild should be an optional build, but force it
			// enabled for now in standalone builds
			Default: !ctx.Config().EmbeddedInMake(),
		})
	}

	// Create (host|host-cross|target)-<OS> phony rules to build a reduced checkbuild.
	osDeps := map[OsType]Paths{}
	ctx.VisitAllModules(func(module Module) {
		if module.Enabled() {
			os := module.Target().Os
			osDeps[os] = append(osDeps[os], module.base().checkbuildFiles...)
		}
	})

	osClass := make(map[string]Paths)
	for os, deps := range osDeps {
		var className string

		switch os.Class {
		case Host:
			className = "host"
		case HostCross:
			className = "host-cross"
		case Device:
			className = "target"
		default:
			continue
		}

		name := PathForPhony(ctx, className+"-"+os.Name)
		osClass[className] = append(osClass[className], name)

		ctx.Build(pctx, BuildParams{
			Rule:      blueprint.Phony,
			Output:    name,
			Implicits: deps,
		})
	}

	// Wrap those into host|host-cross|target phony rules
	osClasses := sortedKeys(osClass)
	for _, class := range osClasses {
		ctx.Build(pctx, BuildParams{
			Rule:      blueprint.Phony,
			Output:    PathForPhony(ctx, class),
			Implicits: osClass[class],
		})
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
}
