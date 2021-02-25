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
	"android/soong/bazel"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/scanner"

	"github.com/google/blueprint"
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
	SymlinkOutput   WritablePath
	SymlinkOutputs  WritablePaths
	ImplicitOutput  WritablePath
	ImplicitOutputs WritablePaths
	Input           Path
	Inputs          Paths
	Implicit        Path
	Implicits       Paths
	OrderOnly       Paths
	Validation      Path
	Validations     Paths
	Default         bool
	Args            map[string]string
}

type ModuleBuildParams BuildParams

// EarlyModuleContext provides methods that can be called early, as soon as the properties have
// been parsed into the module and before any mutators have run.
type EarlyModuleContext interface {
	// Module returns the current module as a Module.  It should rarely be necessary, as the module already has a
	// reference to itself.
	Module() Module

	// ModuleName returns the name of the module.  This is generally the value that was returned by Module.Name() when
	// the module was created, but may have been modified by calls to BaseMutatorContext.Rename.
	ModuleName() string

	// ModuleDir returns the path to the directory that contains the definition of the module.
	ModuleDir() string

	// ModuleType returns the name of the module type that was used to create the module, as specified in
	// RegisterModuleType.
	ModuleType() string

	// BlueprintFile returns the name of the blueprint file that contains the definition of this
	// module.
	BlueprintsFile() string

	// ContainsProperty returns true if the specified property name was set in the module definition.
	ContainsProperty(name string) bool

	// Errorf reports an error at the specified position of the module definition file.
	Errorf(pos scanner.Position, fmt string, args ...interface{})

	// ModuleErrorf reports an error at the line number of the module type in the module definition.
	ModuleErrorf(fmt string, args ...interface{})

	// PropertyErrorf reports an error at the line number of a property in the module definition.
	PropertyErrorf(property, fmt string, args ...interface{})

	// Failed returns true if any errors have been reported.  In most cases the module can continue with generating
	// build rules after an error, allowing it to report additional errors in a single run, but in cases where the error
	// has prevented the module from creating necessary data it can return early when Failed returns true.
	Failed() bool

	// AddNinjaFileDeps adds dependencies on the specified files to the rule that creates the ninja manifest.  The
	// primary builder will be rerun whenever the specified files are modified.
	AddNinjaFileDeps(deps ...string)

	DeviceSpecific() bool
	SocSpecific() bool
	ProductSpecific() bool
	SystemExtSpecific() bool
	Platform() bool

	Config() Config
	DeviceConfig() DeviceConfig

	// Deprecated: use Config()
	AConfig() Config

	// GlobWithDeps returns a list of files that match the specified pattern but do not match any
	// of the patterns in excludes.  It also adds efficient dependencies to rerun the primary
	// builder whenever a file matching the pattern as added or removed, without rerunning if a
	// file that does not match the pattern is added to a searched directory.
	GlobWithDeps(pattern string, excludes []string) ([]string, error)

	Glob(globPattern string, excludes []string) Paths
	GlobFiles(globPattern string, excludes []string) Paths
	IsSymlink(path Path) bool
	Readlink(path Path) string

	// Namespace returns the Namespace object provided by the NameInterface set by Context.SetNameInterface, or the
	// default SimpleNameInterface if Context.SetNameInterface was not called.
	Namespace() *Namespace
}

// BaseModuleContext is the same as blueprint.BaseModuleContext except that Config() returns
// a Config instead of an interface{}, and some methods have been wrapped to use an android.Module
// instead of a blueprint.Module, plus some extra methods that return Android-specific information
// about the current module.
type BaseModuleContext interface {
	EarlyModuleContext

	blueprintBaseModuleContext() blueprint.BaseModuleContext

	// OtherModuleName returns the name of another Module.  See BaseModuleContext.ModuleName for more information.
	// It is intended for use inside the visit functions of Visit* and WalkDeps.
	OtherModuleName(m blueprint.Module) string

	// OtherModuleDir returns the directory of another Module.  See BaseModuleContext.ModuleDir for more information.
	// It is intended for use inside the visit functions of Visit* and WalkDeps.
	OtherModuleDir(m blueprint.Module) string

	// OtherModuleErrorf reports an error on another Module.  See BaseModuleContext.ModuleErrorf for more information.
	// It is intended for use inside the visit functions of Visit* and WalkDeps.
	OtherModuleErrorf(m blueprint.Module, fmt string, args ...interface{})

	// OtherModuleDependencyTag returns the dependency tag used to depend on a module, or nil if there is no dependency
	// on the module.  When called inside a Visit* method with current module being visited, and there are multiple
	// dependencies on the module being visited, it returns the dependency tag used for the current dependency.
	OtherModuleDependencyTag(m blueprint.Module) blueprint.DependencyTag

	// OtherModuleExists returns true if a module with the specified name exists, as determined by the NameInterface
	// passed to Context.SetNameInterface, or SimpleNameInterface if it was not called.
	OtherModuleExists(name string) bool

	// OtherModuleDependencyVariantExists returns true if a module with the
	// specified name and variant exists. The variant must match the given
	// variations. It must also match all the non-local variations of the current
	// module. In other words, it checks for the module AddVariationDependencies
	// would add a dependency on with the same arguments.
	OtherModuleDependencyVariantExists(variations []blueprint.Variation, name string) bool

	// OtherModuleReverseDependencyVariantExists returns true if a module with the
	// specified name exists with the same variations as the current module. In
	// other words, it checks for the module AddReverseDependency would add a
	// dependency on with the same argument.
	OtherModuleReverseDependencyVariantExists(name string) bool

	// OtherModuleType returns the type of another Module.  See BaseModuleContext.ModuleType for more information.
	// It is intended for use inside the visit functions of Visit* and WalkDeps.
	OtherModuleType(m blueprint.Module) string

	// OtherModuleProvider returns the value for a provider for the given module.  If the value is
	// not set it returns the zero value of the type of the provider, so the return value can always
	// be type asserted to the type of the provider.  The value returned may be a deep copy of the
	// value originally passed to SetProvider.
	OtherModuleProvider(m blueprint.Module, provider blueprint.ProviderKey) interface{}

	// OtherModuleHasProvider returns true if the provider for the given module has been set.
	OtherModuleHasProvider(m blueprint.Module, provider blueprint.ProviderKey) bool

	// Provider returns the value for a provider for the current module.  If the value is
	// not set it returns the zero value of the type of the provider, so the return value can always
	// be type asserted to the type of the provider.  It panics if called before the appropriate
	// mutator or GenerateBuildActions pass for the provider.  The value returned may be a deep
	// copy of the value originally passed to SetProvider.
	Provider(provider blueprint.ProviderKey) interface{}

	// HasProvider returns true if the provider for the current module has been set.
	HasProvider(provider blueprint.ProviderKey) bool

	// SetProvider sets the value for a provider for the current module.  It panics if not called
	// during the appropriate mutator or GenerateBuildActions pass for the provider, if the value
	// is not of the appropriate type, or if the value has already been set.  The value should not
	// be modified after being passed to SetProvider.
	SetProvider(provider blueprint.ProviderKey, value interface{})

	GetDirectDepsWithTag(tag blueprint.DependencyTag) []Module

	// GetDirectDepWithTag returns the Module the direct dependency with the specified name, or nil if
	// none exists.  It panics if the dependency does not have the specified tag.  It skips any
	// dependencies that are not an android.Module.
	GetDirectDepWithTag(name string, tag blueprint.DependencyTag) blueprint.Module

	// GetDirectDep returns the Module and DependencyTag for the  direct dependency with the specified
	// name, or nil if none exists.  If there are multiple dependencies on the same module it returns
	// the first DependencyTag.  It skips any dependencies that are not an android.Module.
	GetDirectDep(name string) (blueprint.Module, blueprint.DependencyTag)

	// VisitDirectDepsBlueprint calls visit for each direct dependency.  If there are multiple
	// direct dependencies on the same module visit will be called multiple times on that module
	// and OtherModuleDependencyTag will return a different tag for each.
	//
	// The Module passed to the visit function should not be retained outside of the visit
	// function, it may be invalidated by future mutators.
	VisitDirectDepsBlueprint(visit func(blueprint.Module))

	// VisitDirectDeps calls visit for each direct dependency.  If there are multiple
	// direct dependencies on the same module visit will be called multiple times on that module
	// and OtherModuleDependencyTag will return a different tag for each.  It skips any
	// dependencies that are not an android.Module.
	//
	// The Module passed to the visit function should not be retained outside of the visit
	// function, it may be invalidated by future mutators.
	VisitDirectDeps(visit func(Module))

	VisitDirectDepsWithTag(tag blueprint.DependencyTag, visit func(Module))

	// VisitDirectDepsIf calls pred for each direct dependency, and if pred returns true calls visit.  If there are
	// multiple direct dependencies on the same module pred and visit will be called multiple times on that module and
	// OtherModuleDependencyTag will return a different tag for each.  It skips any
	// dependencies that are not an android.Module.
	//
	// The Module passed to the visit function should not be retained outside of the visit function, it may be
	// invalidated by future mutators.
	VisitDirectDepsIf(pred func(Module) bool, visit func(Module))
	// Deprecated: use WalkDeps instead to support multiple dependency tags on the same module
	VisitDepsDepthFirst(visit func(Module))
	// Deprecated: use WalkDeps instead to support multiple dependency tags on the same module
	VisitDepsDepthFirstIf(pred func(Module) bool, visit func(Module))

	// WalkDeps calls visit for each transitive dependency, traversing the dependency tree in top down order.  visit may
	// be called multiple times for the same (child, parent) pair if there are multiple direct dependencies between the
	// child and parent with different tags.  OtherModuleDependencyTag will return the tag for the currently visited
	// (child, parent) pair.  If visit returns false WalkDeps will not continue recursing down to child.  It skips
	// any dependencies that are not an android.Module.
	//
	// The Modules passed to the visit function should not be retained outside of the visit function, they may be
	// invalidated by future mutators.
	WalkDeps(visit func(Module, Module) bool)

	// WalkDepsBlueprint calls visit for each transitive dependency, traversing the dependency
	// tree in top down order.  visit may be called multiple times for the same (child, parent)
	// pair if there are multiple direct dependencies between the child and parent with different
	// tags.  OtherModuleDependencyTag will return the tag for the currently visited
	// (child, parent) pair.  If visit returns false WalkDeps will not continue recursing down
	// to child.
	//
	// The Modules passed to the visit function should not be retained outside of the visit function, they may be
	// invalidated by future mutators.
	WalkDepsBlueprint(visit func(blueprint.Module, blueprint.Module) bool)

	// GetWalkPath is supposed to be called in visit function passed in WalkDeps()
	// and returns a top-down dependency path from a start module to current child module.
	GetWalkPath() []Module

	// PrimaryModule returns the first variant of the current module.  Variants of a module are always visited in
	// order by mutators and GenerateBuildActions, so the data created by the current mutator can be read from the
	// Module returned by PrimaryModule without data races.  This can be used to perform singleton actions that are
	// only done once for all variants of a module.
	PrimaryModule() Module

	// FinalModule returns the last variant of the current module.  Variants of a module are always visited in
	// order by mutators and GenerateBuildActions, so the data created by the current mutator can be read from all
	// variants using VisitAllModuleVariants if the current module == FinalModule().  This can be used to perform
	// singleton actions that are only done once for all variants of a module.
	FinalModule() Module

	// VisitAllModuleVariants calls visit for each variant of the current module.  Variants of a module are always
	// visited in order by mutators and GenerateBuildActions, so the data created by the current mutator can be read
	// from all variants if the current module == FinalModule().  Otherwise, care must be taken to not access any
	// data modified by the current mutator.
	VisitAllModuleVariants(visit func(Module))

	// GetTagPath is supposed to be called in visit function passed in WalkDeps()
	// and returns a top-down dependency tags path from a start module to current child module.
	// It has one less entry than GetWalkPath() as it contains the dependency tags that
	// exist between each adjacent pair of modules in the GetWalkPath().
	// GetTagPath()[i] is the tag between GetWalkPath()[i] and GetWalkPath()[i+1]
	GetTagPath() []blueprint.DependencyTag

	// GetPathString is supposed to be called in visit function passed in WalkDeps()
	// and returns a multi-line string showing the modules and dependency tags
	// among them along the top-down dependency path from a start module to current child module.
	// skipFirst when set to true, the output doesn't include the start module,
	// which is already printed when this function is used along with ModuleErrorf().
	GetPathString(skipFirst bool) string

	AddMissingDependencies(missingDeps []string)

	Target() Target
	TargetPrimary() bool

	// The additional arch specific targets (e.g. 32/64 bit) that this module variant is
	// responsible for creating.
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
}

// Deprecated: use EarlyModuleContext instead
type BaseContext interface {
	EarlyModuleContext
}

type ModuleContext interface {
	BaseModuleContext

	blueprintModuleContext() blueprint.ModuleContext

	// Deprecated: use ModuleContext.Build instead.
	ModuleBuild(pctx PackageContext, params ModuleBuildParams)

	ExpandSources(srcFiles, excludes []string) Paths
	ExpandSource(srcFile, prop string) Path
	ExpandOptionalSource(srcFile *string, prop string) OptionalPath

	// InstallExecutable creates a rule to copy srcPath to name in the installPath directory,
	// with the given additional dependencies.  The file is marked executable after copying.
	//
	// The installed file will be returned by FilesToInstall(), and the PackagingSpec for the
	// installed file will be returned by PackagingSpecs() on this module or by
	// TransitivePackagingSpecs() on modules that depend on this module through dependency tags
	// for which IsInstallDepNeeded returns true.
	InstallExecutable(installPath InstallPath, name string, srcPath Path, deps ...Path) InstallPath

	// InstallFile creates a rule to copy srcPath to name in the installPath directory,
	// with the given additional dependencies.
	//
	// The installed file will be returned by FilesToInstall(), and the PackagingSpec for the
	// installed file will be returned by PackagingSpecs() on this module or by
	// TransitivePackagingSpecs() on modules that depend on this module through dependency tags
	// for which IsInstallDepNeeded returns true.
	InstallFile(installPath InstallPath, name string, srcPath Path, deps ...Path) InstallPath

	// InstallSymlink creates a rule to create a symlink from src srcPath to name in the installPath
	// directory.
	//
	// The installed symlink will be returned by FilesToInstall(), and the PackagingSpec for the
	// installed file will be returned by PackagingSpecs() on this module or by
	// TransitivePackagingSpecs() on modules that depend on this module through dependency tags
	// for which IsInstallDepNeeded returns true.
	InstallSymlink(installPath InstallPath, name string, srcPath InstallPath) InstallPath

	// InstallAbsoluteSymlink creates a rule to create an absolute symlink from src srcPath to name
	// in the installPath directory.
	//
	// The installed symlink will be returned by FilesToInstall(), and the PackagingSpec for the
	// installed file will be returned by PackagingSpecs() on this module or by
	// TransitivePackagingSpecs() on modules that depend on this module through dependency tags
	// for which IsInstallDepNeeded returns true.
	InstallAbsoluteSymlink(installPath InstallPath, name string, absPath string) InstallPath

	// PackageFile creates a PackagingSpec as if InstallFile was called, but without creating
	// the rule to copy the file.  This is useful to define how a module would be packaged
	// without installing it into the global installation directories.
	//
	// The created PackagingSpec for the will be returned by PackagingSpecs() on this module or by
	// TransitivePackagingSpecs() on modules that depend on this module through dependency tags
	// for which IsInstallDepNeeded returns true.
	PackageFile(installPath InstallPath, name string, srcPath Path) PackagingSpec

	CheckbuildFile(srcPath Path)

	InstallInData() bool
	InstallInTestcases() bool
	InstallInSanitizerDir() bool
	InstallInRamdisk() bool
	InstallInVendorRamdisk() bool
	InstallInRecovery() bool
	InstallInRoot() bool
	InstallBypassMake() bool
	InstallForceOS() (*OsType, *ArchType)

	RequiredModuleNames() []string
	HostRequiredModuleNames() []string
	TargetRequiredModuleNames() []string

	ModuleSubDir() string

	Variable(pctx PackageContext, name, value string)
	Rule(pctx PackageContext, name string, params blueprint.RuleParams, argNames ...string) blueprint.Rule
	// Similar to blueprint.ModuleContext.Build, but takes Paths instead of []string,
	// and performs more verification.
	Build(pctx PackageContext, params BuildParams)
	// Phony creates a Make-style phony rule, a rule with no commands that can depend on other
	// phony rules or real files.  Phony can be called on the same name multiple times to add
	// additional dependencies.
	Phony(phony string, deps ...Path)

	// GetMissingDependencies returns the list of dependencies that were passed to AddDependencies or related methods,
	// but do not exist.
	GetMissingDependencies() []string
}

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
	Enabled() bool
	Target() Target
	Owner() string
	InstallInData() bool
	InstallInTestcases() bool
	InstallInSanitizerDir() bool
	InstallInRamdisk() bool
	InstallInVendorRamdisk() bool
	InstallInRecovery() bool
	InstallInRoot() bool
	InstallBypassMake() bool
	InstallForceOS() (*OsType, *ArchType)
	HideFromMake()
	IsHideFromMake() bool
	IsSkipInstall() bool
	MakeUninstallable()
	ReplacedByPrebuilt()
	IsReplacedByPrebuilt() bool
	ExportedToMake() bool
	InitRc() Paths
	VintfFragments() Paths
	NoticeFiles() Paths

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

	RequiredModuleNames() []string
	HostRequiredModuleNames() []string
	TargetRequiredModuleNames() []string

	FilesToInstall() InstallPaths
	PackagingSpecs() []PackagingSpec

	// TransitivePackagingSpecs returns the PackagingSpecs for this module and any transitive
	// dependencies with dependency tags for which IsInstallDepNeeded() returns true.
	TransitivePackagingSpecs() []PackagingSpec
}

// BazelTargetModule is a lightweight wrapper interface around Module for
// bp2build conversion purposes.
//
// In bp2build's bootstrap.Main execution, Soong runs an alternate pipeline of
// mutators that creates BazelTargetModules from regular Module objects,
// performing the mapping from Soong properties to Bazel rule attributes in the
// process. This process may optionally create additional BazelTargetModules,
// resulting in a 1:many mapping.
//
// bp2build.Codegen is then responsible for visiting all modules in the graph,
// filtering for BazelTargetModules, and code-generating BUILD targets from
// them.
type BazelTargetModule interface {
	Module

	bazelTargetModuleProperties() *bazel.BazelTargetModuleProperties
	SetBazelTargetModuleProperties(props bazel.BazelTargetModuleProperties)

	RuleClass() string
	BzlLoadLocation() string
}

// InitBazelTargetModule is a wrapper function that decorates BazelTargetModule
// with property structs containing metadata for bp2build conversion.
func InitBazelTargetModule(module BazelTargetModule) {
	module.AddProperties(module.bazelTargetModuleProperties())
	InitAndroidModule(module)
}

// BazelTargetModuleBase contains the property structs with metadata for
// bp2build conversion.
type BazelTargetModuleBase struct {
	ModuleBase
	Properties bazel.BazelTargetModuleProperties
}

// bazelTargetModuleProperties getter.
func (btmb *BazelTargetModuleBase) bazelTargetModuleProperties() *bazel.BazelTargetModuleProperties {
	return &btmb.Properties
}

// SetBazelTargetModuleProperties setter for BazelTargetModuleProperties
func (btmb *BazelTargetModuleBase) SetBazelTargetModuleProperties(props bazel.BazelTargetModuleProperties) {
	btmb.Properties = props
}

// RuleClass returns the rule class for this Bazel target
func (b *BazelTargetModuleBase) RuleClass() string {
	return b.bazelTargetModuleProperties().Rule_class
}

// BzlLoadLocation returns the rule class for this Bazel target
func (b *BazelTargetModuleBase) BzlLoadLocation() string {
	return b.bazelTargetModuleProperties().Bzl_load_location
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

	// A string tag to select the OutputFiles associated with the tag.
	//
	// If no tag is specified then it will select the default dist paths provided
	// by the module type. If a tag of "" is specified then it will return the
	// default output files provided by the modules, i.e. the result of calling
	// OutputFiles("").
	Tag *string `android:"arch_variant"`
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
	Enabled *bool `android:"arch_variant"`

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
	// See https://android.googlesource.com/platform/build/soong/+/master/README.md#visibility for
	// more details.
	Visibility []string

	// Describes the licenses applicable to this module. Must reference license modules.
	Licenses []string

	// Flattened from direct license dependencies. Equal to Licenses unless particular module adds more.
	Effective_licenses []string `blueprint:"mutated"`
	// Override of module name when reporting licenses
	Effective_package_name *string `blueprint:"mutated"`
	// Notice files
	Effective_license_text []string `blueprint:"mutated"`
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

	// Whether this module is built for non-native architecures (also known as native bridge binary)
	Native_bridge_supported *bool `android:"arch_variant"`

	// init.rc files to be installed if this module is installed
	Init_rc []string `android:"arch_variant,path"`

	// VINTF manifest fragments to be installed if this module is installed
	Vintf_fragments []string `android:"path"`

	// names of other modules to install if this module is installed
	Required []string `android:"arch_variant"`

	// names of other modules to install on host if this module is installed
	Host_required []string `android:"arch_variant"`

	// names of other modules to install on target if this module is installed
	Target_required []string `android:"arch_variant"`

	// relative path to a file to include in the list of notices for the device
	Notice *string `android:"path"`

	// The OsType of artifacts that this module variant is responsible for creating.
	//
	// Set by osMutator
	CompileOS OsType `blueprint:"mutated"`

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

	// When HideFromMake is set to true, no entry for this variant will be emitted in the
	// generated Android.mk file.
	HideFromMake bool `blueprint:"mutated"`

	// When SkipInstall is set to true, calls to ctx.InstallFile, ctx.InstallExecutable,
	// ctx.InstallSymlink and ctx.InstallAbsoluteSymlink act like calls to ctx.PackageFile
	// and don't create a rule to install the file.
	SkipInstall bool `blueprint:"mutated"`

	// Whether the module has been replaced by a prebuilt
	ReplacedByPrebuilt bool `blueprint:"mutated"`

	// Disabled by mutators. If set to true, it overrides Enabled property.
	ForcedDisabled bool `blueprint:"mutated"`

	NamespaceExportedToMake bool `blueprint:"mutated"`

	MissingDeps []string `blueprint:"mutated"`

	// Name and variant strings stored by mutators to enable Module.String()
	DebugName       string   `blueprint:"mutated"`
	DebugMutators   []string `blueprint:"mutated"`
	DebugVariations []string `blueprint:"mutated"`

	// ImageVariation is set by ImageMutator to specify which image this variation is for,
	// for example "" for core or "recovery" for recovery.  It will often be set to one of the
	// constants in image.go, but can also be set to a custom value by individual module types.
	ImageVariation string `blueprint:"mutated"`
}

type distProperties struct {
	// configuration to distribute output files from this module to the distribution
	// directory (default: $OUT/dist, configurable with $DIST_DIR)
	Dist Dist `android:"arch_variant"`

	// a list of configurations to distribute output files from this module to the
	// distribution directory (default: $OUT/dist, configurable with $DIST_DIR)
	Dists []Dist `android:"arch_variant"`
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
	for _, path := range paths {
		if path == nil {
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
	MultilibDefault     Multilib = ""
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

	// Device is built by default. Host and HostCross are supported.
	HostAndDeviceSupported = hostSupported | hostCrossSupported | deviceSupported | deviceDefault

	// Host, HostCross, and Device are built by default.
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

	base.generalProperties = m.GetProperties()
	base.customizableProperties = m.GetProperties()

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
//  InitAndroidModule should not be called if InitAndroidArchModule was called.
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
	distProperties          distProperties
	variableProperties      interface{}
	hostAndDeviceProperties hostAndDeviceProperties
	generalProperties       []interface{}
	archProperties          [][]interface{}
	customizableProperties  []interface{}

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
	installFilesDepSet   *installPathsDepSet
	checkbuildFiles      Paths
	packagingSpecs       []PackagingSpec
	packagingSpecsDepSet *packagingSpecsDepSet
	noticeFiles          Paths
	phonies              map[string]Paths

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

	prefer32 func(ctx BaseModuleContext, base *ModuleBase, os OsType) bool
}

func (m *ModuleBase) ComponentDepsMutator(BottomUpMutatorContext) {}

func (m *ModuleBase) DepsMutator(BottomUpMutatorContext) {}

func (m *ModuleBase) AddProperties(props ...interface{}) {
	m.registerProps = append(m.registerProps, props...)
}

func (m *ModuleBase) GetProperties() []interface{} {
	return m.registerProps
}

func (m *ModuleBase) BuildParamsForTests() []BuildParams {
	return m.buildParams
}

func (m *ModuleBase) RuleParamsForTests() map[blueprint.Rule]blueprint.RuleParams {
	return m.ruleParams
}

func (m *ModuleBase) VariablesForTests() map[string]string {
	return m.variables
}

func (m *ModuleBase) Prefer32(prefer32 func(ctx BaseModuleContext, base *ModuleBase, os OsType) bool) {
	m.prefer32 = prefer32
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

func (m *ModuleBase) Enabled() bool {
	if m.commonProperties.ForcedDisabled {
		return false
	}
	if m.commonProperties.Enabled == nil {
		return !m.Os().DefaultDisabled
	}
	return *m.commonProperties.Enabled
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

// computeInstallDeps finds the installed paths of all dependencies that have a dependency
// tag that is annotated as needing installation via the IsInstallDepNeeded method.
func (m *ModuleBase) computeInstallDeps(ctx ModuleContext) ([]*installPathsDepSet, []*packagingSpecsDepSet) {
	var installDeps []*installPathsDepSet
	var packagingSpecs []*packagingSpecsDepSet
	ctx.VisitDirectDeps(func(dep Module) {
		if IsInstallDepNeeded(ctx.OtherModuleDependencyTag(dep)) {
			installDeps = append(installDeps, dep.base().installFilesDepSet)
			packagingSpecs = append(packagingSpecs, dep.base().packagingSpecsDepSet)
		}
	})

	return installDeps, packagingSpecs
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

func (m *ModuleBase) InstallInRecovery() bool {
	return Bool(m.commonProperties.Recovery)
}

func (m *ModuleBase) InstallInRoot() bool {
	return false
}

func (m *ModuleBase) InstallBypassMake() bool {
	return false
}

func (m *ModuleBase) InstallForceOS() (*OsType, *ArchType) {
	return nil, nil
}

func (m *ModuleBase) Owner() string {
	return String(m.commonProperties.Owner)
}

func (m *ModuleBase) NoticeFiles() Paths {
	return m.noticeFiles
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

func (m *ModuleBase) InRecovery() bool {
	return m.base().commonProperties.ImageVariation == RecoveryVariation
}

func (m *ModuleBase) RequiredModuleNames() []string {
	return m.base().commonProperties.Required
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

func (m *ModuleBase) generateModuleTarget(ctx ModuleContext) {
	var allInstalledFiles InstallPaths
	var allCheckbuildFiles Paths
	ctx.VisitAllModuleVariants(func(module Module) {
		a := module.base()
		allInstalledFiles = append(allInstalledFiles, a.installFiles...)
		allCheckbuildFiles = append(allCheckbuildFiles, a.checkbuildFiles...)
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
		earlyModuleContext: m.earlyModuleContextFactory(ctx),
		os:                 m.commonProperties.CompileOS,
		target:             m.commonProperties.CompileTarget,
		targetPrimary:      m.commonProperties.CompilePrimary,
		multiTargets:       m.commonProperties.CompileMultiTargets,
	}
}

func (m *ModuleBase) GenerateBuildActions(blueprintCtx blueprint.ModuleContext) {
	ctx := &moduleContext{
		module:            m.module,
		bp:                blueprintCtx,
		baseModuleContext: m.baseModuleContextFactory(blueprintCtx),
		variables:         make(map[string]string),
	}

	dependencyInstallFiles, dependencyPackagingSpecs := m.computeInstallDeps(ctx)
	// set m.installFilesDepSet to only the transitive dependencies to be used as the dependencies
	// of installed files of this module.  It will be replaced by a depset including the installed
	// files of this module at the end for use by modules that depend on this one.
	m.installFilesDepSet = newInstallPathsDepSet(nil, dependencyInstallFiles)

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
	if apexInfo := ctx.Provider(ApexInfoProvider).(ApexInfo); !apexInfo.IsForPlatform() {
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
	for i, _ := range m.distProperties.Dists {
		checkDistProperties(ctx, fmt.Sprintf("dists[%d]", i), &m.distProperties.Dists[i])
	}

	if m.Enabled() {
		// ensure all direct android.Module deps are enabled
		ctx.VisitDirectDepsBlueprint(func(bm blueprint.Module) {
			if _, ok := bm.(Module); ok {
				ctx.validateAndroidModule(bm, ctx.baseModuleContext.strictVisitDeps)
			}
		})

		m.noticeFiles = make([]Path, 0)
		optPath := OptionalPath{}
		notice := proptools.StringDefault(m.commonProperties.Notice, "")
		if module := SrcIsModule(notice); module != "" {
			optPath = ctx.ExpandOptionalSource(&notice, "notice")
		} else if notice != "" {
			noticePath := filepath.Join(ctx.ModuleDir(), notice)
			optPath = ExistentPathForSource(ctx, noticePath)
		}
		if optPath.Valid() {
			m.noticeFiles = append(m.noticeFiles, optPath.Path())
		} else {
			for _, notice = range []string{"LICENSE", "LICENCE", "NOTICE"} {
				noticePath := filepath.Join(ctx.ModuleDir(), notice)
				optPath = ExistentPathForSource(ctx, noticePath)
				if optPath.Valid() {
					m.noticeFiles = append(m.noticeFiles, optPath.Path())
				}
			}
		}

		licensesPropertyFlattener(ctx)
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
		m.initRcPaths = PathsForModuleSrc(ctx, m.commonProperties.Init_rc)
		m.vintfFragmentsPaths = PathsForModuleSrc(ctx, m.commonProperties.Vintf_fragments)
		for k, v := range ctx.phonies {
			m.phonies[k] = append(m.phonies[k], v...)
		}
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

	m.installFilesDepSet = newInstallPathsDepSet(m.installFiles, dependencyInstallFiles)
	m.packagingSpecsDepSet = newPackagingSpecsDepSet(m.packagingSpecs, dependencyPackagingSpecs)

	m.buildParams = ctx.buildParams
	m.ruleParams = ctx.ruleParams
	m.variables = ctx.variables
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

type earlyModuleContext struct {
	blueprint.EarlyModuleContext

	kind   moduleKind
	config Config
}

func (e *earlyModuleContext) Glob(globPattern string, excludes []string) Paths {
	return Glob(e, globPattern, excludes)
}

func (e *earlyModuleContext) GlobFiles(globPattern string, excludes []string) Paths {
	return GlobFiles(e, globPattern, excludes)
}

func (b *earlyModuleContext) IsSymlink(path Path) bool {
	fileInfo, err := b.config.fs.Lstat(path.String())
	if err != nil {
		b.ModuleErrorf("os.Lstat(%q) failed: %s", path.String(), err)
	}
	return fileInfo.Mode()&os.ModeSymlink == os.ModeSymlink
}

func (b *earlyModuleContext) Readlink(path Path) string {
	dest, err := b.config.fs.Readlink(path.String())
	if err != nil {
		b.ModuleErrorf("os.Readlink(%q) failed: %s", path.String(), err)
	}
	return dest
}

func (e *earlyModuleContext) Module() Module {
	module, _ := e.EarlyModuleContext.Module().(Module)
	return module
}

func (e *earlyModuleContext) Config() Config {
	return e.EarlyModuleContext.Config().(Config)
}

func (e *earlyModuleContext) AConfig() Config {
	return e.config
}

func (e *earlyModuleContext) DeviceConfig() DeviceConfig {
	return DeviceConfig{e.config.deviceConfig}
}

func (e *earlyModuleContext) Platform() bool {
	return e.kind == platformModule
}

func (e *earlyModuleContext) DeviceSpecific() bool {
	return e.kind == deviceSpecificModule
}

func (e *earlyModuleContext) SocSpecific() bool {
	return e.kind == socSpecificModule
}

func (e *earlyModuleContext) ProductSpecific() bool {
	return e.kind == productSpecificModule
}

func (e *earlyModuleContext) SystemExtSpecific() bool {
	return e.kind == systemExtSpecificModule
}

func (e *earlyModuleContext) Namespace() *Namespace {
	return e.EarlyModuleContext.Namespace().(*Namespace)
}

type baseModuleContext struct {
	bp blueprint.BaseModuleContext
	earlyModuleContext
	os            OsType
	target        Target
	multiTargets  []Target
	targetPrimary bool
	debug         bool

	walkPath []Module
	tagPath  []blueprint.DependencyTag

	strictVisitDeps bool // If true, enforce that all dependencies are enabled
}

func (b *baseModuleContext) OtherModuleName(m blueprint.Module) string {
	return b.bp.OtherModuleName(m)
}
func (b *baseModuleContext) OtherModuleDir(m blueprint.Module) string { return b.bp.OtherModuleDir(m) }
func (b *baseModuleContext) OtherModuleErrorf(m blueprint.Module, fmt string, args ...interface{}) {
	b.bp.OtherModuleErrorf(m, fmt, args...)
}
func (b *baseModuleContext) OtherModuleDependencyTag(m blueprint.Module) blueprint.DependencyTag {
	return b.bp.OtherModuleDependencyTag(m)
}
func (b *baseModuleContext) OtherModuleExists(name string) bool { return b.bp.OtherModuleExists(name) }
func (b *baseModuleContext) OtherModuleDependencyVariantExists(variations []blueprint.Variation, name string) bool {
	return b.bp.OtherModuleDependencyVariantExists(variations, name)
}
func (b *baseModuleContext) OtherModuleReverseDependencyVariantExists(name string) bool {
	return b.bp.OtherModuleReverseDependencyVariantExists(name)
}
func (b *baseModuleContext) OtherModuleType(m blueprint.Module) string {
	return b.bp.OtherModuleType(m)
}
func (b *baseModuleContext) OtherModuleProvider(m blueprint.Module, provider blueprint.ProviderKey) interface{} {
	return b.bp.OtherModuleProvider(m, provider)
}
func (b *baseModuleContext) OtherModuleHasProvider(m blueprint.Module, provider blueprint.ProviderKey) bool {
	return b.bp.OtherModuleHasProvider(m, provider)
}
func (b *baseModuleContext) Provider(provider blueprint.ProviderKey) interface{} {
	return b.bp.Provider(provider)
}
func (b *baseModuleContext) HasProvider(provider blueprint.ProviderKey) bool {
	return b.bp.HasProvider(provider)
}
func (b *baseModuleContext) SetProvider(provider blueprint.ProviderKey, value interface{}) {
	b.bp.SetProvider(provider, value)
}

func (b *baseModuleContext) GetDirectDepWithTag(name string, tag blueprint.DependencyTag) blueprint.Module {
	return b.bp.GetDirectDepWithTag(name, tag)
}

func (b *baseModuleContext) blueprintBaseModuleContext() blueprint.BaseModuleContext {
	return b.bp
}

type moduleContext struct {
	bp blueprint.ModuleContext
	baseModuleContext
	packagingSpecs  []PackagingSpec
	installFiles    InstallPaths
	checkbuildFiles Paths
	module          Module
	phonies         map[string]Paths

	// For tests
	buildParams []BuildParams
	ruleParams  map[blueprint.Rule]blueprint.RuleParams
	variables   map[string]string
}

func (m *moduleContext) ninjaError(params BuildParams, err error) (PackageContext, BuildParams) {
	return pctx, BuildParams{
		Rule:            ErrorRule,
		Description:     params.Description,
		Output:          params.Output,
		Outputs:         params.Outputs,
		ImplicitOutput:  params.ImplicitOutput,
		ImplicitOutputs: params.ImplicitOutputs,
		Args: map[string]string{
			"error": err.Error(),
		},
	}
}

func (m *moduleContext) ModuleBuild(pctx PackageContext, params ModuleBuildParams) {
	m.Build(pctx, BuildParams(params))
}

func validateBuildParams(params blueprint.BuildParams) error {
	// Validate that the symlink outputs are declared outputs or implicit outputs
	allOutputs := map[string]bool{}
	for _, output := range params.Outputs {
		allOutputs[output] = true
	}
	for _, output := range params.ImplicitOutputs {
		allOutputs[output] = true
	}
	for _, symlinkOutput := range params.SymlinkOutputs {
		if !allOutputs[symlinkOutput] {
			return fmt.Errorf(
				"Symlink output %s is not a declared output or implicit output",
				symlinkOutput)
		}
	}
	return nil
}

// Convert build parameters from their concrete Android types into their string representations,
// and combine the singular and plural fields of the same type (e.g. Output and Outputs).
func convertBuildParams(params BuildParams) blueprint.BuildParams {
	bparams := blueprint.BuildParams{
		Rule:            params.Rule,
		Description:     params.Description,
		Deps:            params.Deps,
		Outputs:         params.Outputs.Strings(),
		ImplicitOutputs: params.ImplicitOutputs.Strings(),
		SymlinkOutputs:  params.SymlinkOutputs.Strings(),
		Inputs:          params.Inputs.Strings(),
		Implicits:       params.Implicits.Strings(),
		OrderOnly:       params.OrderOnly.Strings(),
		Validations:     params.Validations.Strings(),
		Args:            params.Args,
		Optional:        !params.Default,
	}

	if params.Depfile != nil {
		bparams.Depfile = params.Depfile.String()
	}
	if params.Output != nil {
		bparams.Outputs = append(bparams.Outputs, params.Output.String())
	}
	if params.SymlinkOutput != nil {
		bparams.SymlinkOutputs = append(bparams.SymlinkOutputs, params.SymlinkOutput.String())
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
	if params.Validation != nil {
		bparams.Validations = append(bparams.Validations, params.Validation.String())
	}

	bparams.Outputs = proptools.NinjaEscapeList(bparams.Outputs)
	bparams.ImplicitOutputs = proptools.NinjaEscapeList(bparams.ImplicitOutputs)
	bparams.SymlinkOutputs = proptools.NinjaEscapeList(bparams.SymlinkOutputs)
	bparams.Inputs = proptools.NinjaEscapeList(bparams.Inputs)
	bparams.Implicits = proptools.NinjaEscapeList(bparams.Implicits)
	bparams.OrderOnly = proptools.NinjaEscapeList(bparams.OrderOnly)
	bparams.Validations = proptools.NinjaEscapeList(bparams.Validations)
	bparams.Depfile = proptools.NinjaEscape(bparams.Depfile)

	return bparams
}

func (m *moduleContext) Variable(pctx PackageContext, name, value string) {
	if m.config.captureBuild {
		m.variables[name] = value
	}

	m.bp.Variable(pctx.PackageContext, name, value)
}

func (m *moduleContext) Rule(pctx PackageContext, name string, params blueprint.RuleParams,
	argNames ...string) blueprint.Rule {

	if m.config.UseRemoteBuild() {
		if params.Pool == nil {
			// When USE_GOMA=true or USE_RBE=true are set and the rule is not supported by goma/RBE, restrict
			// jobs to the local parallelism value
			params.Pool = localPool
		} else if params.Pool == remotePool {
			// remotePool is a fake pool used to identify rule that are supported for remoting. If the rule's
			// pool is the remotePool, replace with nil so that ninja runs it at NINJA_REMOTE_NUM_JOBS
			// parallelism.
			params.Pool = nil
		}
	}

	rule := m.bp.Rule(pctx.PackageContext, name, params, argNames...)

	if m.config.captureBuild {
		m.ruleParams[rule] = params
	}

	return rule
}

func (m *moduleContext) Build(pctx PackageContext, params BuildParams) {
	if params.Description != "" {
		params.Description = "${moduleDesc}" + params.Description + "${moduleDescSuffix}"
	}

	if missingDeps := m.GetMissingDependencies(); len(missingDeps) > 0 {
		pctx, params = m.ninjaError(params, fmt.Errorf("module %s missing dependencies: %s\n",
			m.ModuleName(), strings.Join(missingDeps, ", ")))
	}

	if m.config.captureBuild {
		m.buildParams = append(m.buildParams, params)
	}

	bparams := convertBuildParams(params)
	err := validateBuildParams(bparams)
	if err != nil {
		m.ModuleErrorf(
			"%s: build parameter validation failed: %s",
			m.ModuleName(),
			err.Error())
	}
	m.bp.Build(pctx.PackageContext, bparams)
}

func (m *moduleContext) Phony(name string, deps ...Path) {
	addPhony(m.config, name, deps...)
}

func (m *moduleContext) GetMissingDependencies() []string {
	var missingDeps []string
	missingDeps = append(missingDeps, m.Module().base().commonProperties.MissingDeps...)
	missingDeps = append(missingDeps, m.bp.GetMissingDependencies()...)
	missingDeps = FirstUniqueStrings(missingDeps)
	return missingDeps
}

func (b *baseModuleContext) AddMissingDependencies(deps []string) {
	if deps != nil {
		missingDeps := &b.Module().base().commonProperties.MissingDeps
		*missingDeps = append(*missingDeps, deps...)
		*missingDeps = FirstUniqueStrings(*missingDeps)
	}
}

func (b *baseModuleContext) validateAndroidModule(module blueprint.Module, strict bool) Module {
	aModule, _ := module.(Module)

	if !strict {
		return aModule
	}

	if aModule == nil {
		b.ModuleErrorf("module %q not an android module", b.OtherModuleName(module))
		return nil
	}

	if !aModule.Enabled() {
		if b.Config().AllowMissingDependencies() {
			b.AddMissingDependencies([]string{b.OtherModuleName(aModule)})
		} else {
			b.ModuleErrorf("depends on disabled module %q", b.OtherModuleName(aModule))
		}
		return nil
	}
	return aModule
}

func (b *baseModuleContext) getDirectDepInternal(name string, tag blueprint.DependencyTag) (blueprint.Module, blueprint.DependencyTag) {
	type dep struct {
		mod blueprint.Module
		tag blueprint.DependencyTag
	}
	var deps []dep
	b.VisitDirectDepsBlueprint(func(module blueprint.Module) {
		if aModule, _ := module.(Module); aModule != nil {
			if aModule.base().BaseModuleName() == name {
				returnedTag := b.bp.OtherModuleDependencyTag(aModule)
				if tag == nil || returnedTag == tag {
					deps = append(deps, dep{aModule, returnedTag})
				}
			}
		} else if b.bp.OtherModuleName(module) == name {
			returnedTag := b.bp.OtherModuleDependencyTag(module)
			if tag == nil || returnedTag == tag {
				deps = append(deps, dep{module, returnedTag})
			}
		}
	})
	if len(deps) == 1 {
		return deps[0].mod, deps[0].tag
	} else if len(deps) >= 2 {
		panic(fmt.Errorf("Multiple dependencies having same BaseModuleName() %q found from %q",
			name, b.ModuleName()))
	} else {
		return nil, nil
	}
}

func (b *baseModuleContext) GetDirectDepsWithTag(tag blueprint.DependencyTag) []Module {
	var deps []Module
	b.VisitDirectDepsBlueprint(func(module blueprint.Module) {
		if aModule, _ := module.(Module); aModule != nil {
			if b.bp.OtherModuleDependencyTag(aModule) == tag {
				deps = append(deps, aModule)
			}
		}
	})
	return deps
}

func (m *moduleContext) GetDirectDepWithTag(name string, tag blueprint.DependencyTag) blueprint.Module {
	module, _ := m.getDirectDepInternal(name, tag)
	return module
}

func (b *baseModuleContext) GetDirectDep(name string) (blueprint.Module, blueprint.DependencyTag) {
	return b.getDirectDepInternal(name, nil)
}

func (b *baseModuleContext) VisitDirectDepsBlueprint(visit func(blueprint.Module)) {
	b.bp.VisitDirectDeps(visit)
}

func (b *baseModuleContext) VisitDirectDeps(visit func(Module)) {
	b.bp.VisitDirectDeps(func(module blueprint.Module) {
		if aModule := b.validateAndroidModule(module, b.strictVisitDeps); aModule != nil {
			visit(aModule)
		}
	})
}

func (b *baseModuleContext) VisitDirectDepsWithTag(tag blueprint.DependencyTag, visit func(Module)) {
	b.bp.VisitDirectDeps(func(module blueprint.Module) {
		if aModule := b.validateAndroidModule(module, b.strictVisitDeps); aModule != nil {
			if b.bp.OtherModuleDependencyTag(aModule) == tag {
				visit(aModule)
			}
		}
	})
}

func (b *baseModuleContext) VisitDirectDepsIf(pred func(Module) bool, visit func(Module)) {
	b.bp.VisitDirectDepsIf(
		// pred
		func(module blueprint.Module) bool {
			if aModule := b.validateAndroidModule(module, b.strictVisitDeps); aModule != nil {
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

func (b *baseModuleContext) VisitDepsDepthFirst(visit func(Module)) {
	b.bp.VisitDepsDepthFirst(func(module blueprint.Module) {
		if aModule := b.validateAndroidModule(module, b.strictVisitDeps); aModule != nil {
			visit(aModule)
		}
	})
}

func (b *baseModuleContext) VisitDepsDepthFirstIf(pred func(Module) bool, visit func(Module)) {
	b.bp.VisitDepsDepthFirstIf(
		// pred
		func(module blueprint.Module) bool {
			if aModule := b.validateAndroidModule(module, b.strictVisitDeps); aModule != nil {
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

func (b *baseModuleContext) WalkDepsBlueprint(visit func(blueprint.Module, blueprint.Module) bool) {
	b.bp.WalkDeps(visit)
}

func (b *baseModuleContext) WalkDeps(visit func(Module, Module) bool) {
	b.walkPath = []Module{b.Module()}
	b.tagPath = []blueprint.DependencyTag{}
	b.bp.WalkDeps(func(child, parent blueprint.Module) bool {
		childAndroidModule, _ := child.(Module)
		parentAndroidModule, _ := parent.(Module)
		if childAndroidModule != nil && parentAndroidModule != nil {
			// record walkPath before visit
			for b.walkPath[len(b.walkPath)-1] != parentAndroidModule {
				b.walkPath = b.walkPath[0 : len(b.walkPath)-1]
				b.tagPath = b.tagPath[0 : len(b.tagPath)-1]
			}
			b.walkPath = append(b.walkPath, childAndroidModule)
			b.tagPath = append(b.tagPath, b.OtherModuleDependencyTag(childAndroidModule))
			return visit(childAndroidModule, parentAndroidModule)
		} else {
			return false
		}
	})
}

func (b *baseModuleContext) GetWalkPath() []Module {
	return b.walkPath
}

func (b *baseModuleContext) GetTagPath() []blueprint.DependencyTag {
	return b.tagPath
}

func (b *baseModuleContext) VisitAllModuleVariants(visit func(Module)) {
	b.bp.VisitAllModuleVariants(func(module blueprint.Module) {
		visit(module.(Module))
	})
}

func (b *baseModuleContext) PrimaryModule() Module {
	return b.bp.PrimaryModule().(Module)
}

func (b *baseModuleContext) FinalModule() Module {
	return b.bp.FinalModule().(Module)
}

// IsMetaDependencyTag returns true for cross-cutting metadata dependencies.
func IsMetaDependencyTag(tag blueprint.DependencyTag) bool {
	if tag == licenseKindTag {
		return true
	} else if tag == licensesTag {
		return true
	}
	return false
}

// A regexp for removing boilerplate from BaseDependencyTag from the string representation of
// a dependency tag.
var tagCleaner = regexp.MustCompile(`\QBaseDependencyTag:{}\E(, )?`)

// PrettyPrintTag returns string representation of the tag, but prefers
// custom String() method if available.
func PrettyPrintTag(tag blueprint.DependencyTag) string {
	// Use tag's custom String() method if available.
	if stringer, ok := tag.(fmt.Stringer); ok {
		return stringer.String()
	}

	// Otherwise, get a default string representation of the tag's struct.
	tagString := fmt.Sprintf("%T: %+v", tag, tag)

	// Remove the boilerplate from BaseDependencyTag as it adds no value.
	tagString = tagCleaner.ReplaceAllString(tagString, "")
	return tagString
}

func (b *baseModuleContext) GetPathString(skipFirst bool) string {
	sb := strings.Builder{}
	tagPath := b.GetTagPath()
	walkPath := b.GetWalkPath()
	if !skipFirst {
		sb.WriteString(walkPath[0].String())
	}
	for i, m := range walkPath[1:] {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("           via tag %s\n", PrettyPrintTag(tagPath[i])))
		sb.WriteString(fmt.Sprintf("    -> %s", m.String()))
	}
	return sb.String()
}

func (m *moduleContext) ModuleSubDir() string {
	return m.bp.ModuleSubDir()
}

func (b *baseModuleContext) Target() Target {
	return b.target
}

func (b *baseModuleContext) TargetPrimary() bool {
	return b.targetPrimary
}

func (b *baseModuleContext) MultiTargets() []Target {
	return b.multiTargets
}

func (b *baseModuleContext) Arch() Arch {
	return b.target.Arch
}

func (b *baseModuleContext) Os() OsType {
	return b.os
}

func (b *baseModuleContext) Host() bool {
	return b.os.Class == Host
}

func (b *baseModuleContext) Device() bool {
	return b.os.Class == Device
}

func (b *baseModuleContext) Darwin() bool {
	return b.os == Darwin
}

func (b *baseModuleContext) Fuchsia() bool {
	return b.os == Fuchsia
}

func (b *baseModuleContext) Windows() bool {
	return b.os == Windows
}

func (b *baseModuleContext) Debug() bool {
	return b.debug
}

func (b *baseModuleContext) PrimaryArch() bool {
	if len(b.config.Targets[b.target.Os]) <= 1 {
		return true
	}
	return b.target.Arch.ArchType == b.config.Targets[b.target.Os][0].Arch.ArchType
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

func (m *moduleContext) InstallInData() bool {
	return m.module.InstallInData()
}

func (m *moduleContext) InstallInTestcases() bool {
	return m.module.InstallInTestcases()
}

func (m *moduleContext) InstallInSanitizerDir() bool {
	return m.module.InstallInSanitizerDir()
}

func (m *moduleContext) InstallInRamdisk() bool {
	return m.module.InstallInRamdisk()
}

func (m *moduleContext) InstallInVendorRamdisk() bool {
	return m.module.InstallInVendorRamdisk()
}

func (m *moduleContext) InstallInRecovery() bool {
	return m.module.InstallInRecovery()
}

func (m *moduleContext) InstallInRoot() bool {
	return m.module.InstallInRoot()
}

func (m *moduleContext) InstallBypassMake() bool {
	return m.module.InstallBypassMake()
}

func (m *moduleContext) InstallForceOS() (*OsType, *ArchType) {
	return m.module.InstallForceOS()
}

func (m *moduleContext) skipInstall() bool {
	if m.module.base().commonProperties.SkipInstall {
		return true
	}

	if m.module.base().commonProperties.HideFromMake {
		return true
	}

	// We'll need a solution for choosing which of modules with the same name in different
	// namespaces to install.  For now, reuse the list of namespaces exported to Make as the
	// list of namespaces to install in a Soong-only build.
	if !m.module.base().commonProperties.NamespaceExportedToMake {
		return true
	}

	if m.Device() {
		if m.Config().KatiEnabled() && !m.InstallBypassMake() {
			return true
		}
	}

	return false
}

func (m *moduleContext) InstallFile(installPath InstallPath, name string, srcPath Path,
	deps ...Path) InstallPath {
	return m.installFile(installPath, name, srcPath, deps, false)
}

func (m *moduleContext) InstallExecutable(installPath InstallPath, name string, srcPath Path,
	deps ...Path) InstallPath {
	return m.installFile(installPath, name, srcPath, deps, true)
}

func (m *moduleContext) PackageFile(installPath InstallPath, name string, srcPath Path) PackagingSpec {
	fullInstallPath := installPath.Join(m, name)
	return m.packageFile(fullInstallPath, srcPath, false)
}

func (m *moduleContext) packageFile(fullInstallPath InstallPath, srcPath Path, executable bool) PackagingSpec {
	spec := PackagingSpec{
		relPathInPackage: Rel(m, fullInstallPath.PartitionDir(), fullInstallPath.String()),
		srcPath:          srcPath,
		symlinkTarget:    "",
		executable:       executable,
	}
	m.packagingSpecs = append(m.packagingSpecs, spec)
	return spec
}

func (m *moduleContext) installFile(installPath InstallPath, name string, srcPath Path, deps []Path, executable bool) InstallPath {

	fullInstallPath := installPath.Join(m, name)
	m.module.base().hooks.runInstallHooks(m, srcPath, fullInstallPath, false)

	if !m.skipInstall() {
		deps = append(deps, m.module.base().installFilesDepSet.ToList().Paths()...)

		var implicitDeps, orderOnlyDeps Paths

		if m.Host() {
			// Installed host modules might be used during the build, depend directly on their
			// dependencies so their timestamp is updated whenever their dependency is updated
			implicitDeps = deps
		} else {
			orderOnlyDeps = deps
		}

		rule := Cp
		if executable {
			rule = CpExecutable
		}

		m.Build(pctx, BuildParams{
			Rule:        rule,
			Description: "install " + fullInstallPath.Base(),
			Output:      fullInstallPath,
			Input:       srcPath,
			Implicits:   implicitDeps,
			OrderOnly:   orderOnlyDeps,
			Default:     !m.Config().KatiEnabled(),
		})

		m.installFiles = append(m.installFiles, fullInstallPath)
	}

	m.packageFile(fullInstallPath, srcPath, executable)

	m.checkbuildFiles = append(m.checkbuildFiles, srcPath)

	return fullInstallPath
}

func (m *moduleContext) InstallSymlink(installPath InstallPath, name string, srcPath InstallPath) InstallPath {
	fullInstallPath := installPath.Join(m, name)
	m.module.base().hooks.runInstallHooks(m, srcPath, fullInstallPath, true)

	relPath, err := filepath.Rel(path.Dir(fullInstallPath.String()), srcPath.String())
	if err != nil {
		panic(fmt.Sprintf("Unable to generate symlink between %q and %q: %s", fullInstallPath.Base(), srcPath.Base(), err))
	}
	if !m.skipInstall() {

		m.Build(pctx, BuildParams{
			Rule:        Symlink,
			Description: "install symlink " + fullInstallPath.Base(),
			Output:      fullInstallPath,
			Input:       srcPath,
			Default:     !m.Config().KatiEnabled(),
			Args: map[string]string{
				"fromPath": relPath,
			},
		})

		m.installFiles = append(m.installFiles, fullInstallPath)
		m.checkbuildFiles = append(m.checkbuildFiles, srcPath)
	}

	m.packagingSpecs = append(m.packagingSpecs, PackagingSpec{
		relPathInPackage: Rel(m, fullInstallPath.PartitionDir(), fullInstallPath.String()),
		srcPath:          nil,
		symlinkTarget:    relPath,
		executable:       false,
	})

	return fullInstallPath
}

// installPath/name -> absPath where absPath might be a path that is available only at runtime
// (e.g. /apex/...)
func (m *moduleContext) InstallAbsoluteSymlink(installPath InstallPath, name string, absPath string) InstallPath {
	fullInstallPath := installPath.Join(m, name)
	m.module.base().hooks.runInstallHooks(m, nil, fullInstallPath, true)

	if !m.skipInstall() {
		m.Build(pctx, BuildParams{
			Rule:        Symlink,
			Description: "install symlink " + fullInstallPath.Base() + " -> " + absPath,
			Output:      fullInstallPath,
			Default:     !m.Config().KatiEnabled(),
			Args: map[string]string{
				"fromPath": absPath,
			},
		})

		m.installFiles = append(m.installFiles, fullInstallPath)
	}

	m.packagingSpecs = append(m.packagingSpecs, PackagingSpec{
		relPathInPackage: Rel(m, fullInstallPath.PartitionDir(), fullInstallPath.String()),
		srcPath:          nil,
		symlinkTarget:    absPath,
		executable:       false,
	})

	return fullInstallPath
}

func (m *moduleContext) CheckbuildFile(srcPath Path) {
	m.checkbuildFiles = append(m.checkbuildFiles, srcPath)
}

func (m *moduleContext) blueprintModuleContext() blueprint.ModuleContext {
	return m.bp
}

// SrcIsModule decodes module references in the format ":name" into the module name, or empty string if the input
// was not a module reference.
func SrcIsModule(s string) (module string) {
	if len(s) > 1 && s[0] == ':' {
		return s[1:]
	}
	return ""
}

// SrcIsModule decodes module references in the format ":name{.tag}" into the module name and tag, ":name" into the
// module name and an empty string for the tag, or empty strings if the input was not a module reference.
func SrcIsModuleWithTag(s string) (module, tag string) {
	if len(s) > 1 && s[0] == ':' {
		module = s[1:]
		if tagStart := strings.IndexByte(module, '{'); tagStart > 0 {
			if module[len(module)-1] == '}' {
				tag = module[tagStart+1 : len(module)-1]
				module = module[:tagStart]
				return module, tag
			}
		}
		return module, ""
	}
	return "", ""
}

type sourceOrOutputDependencyTag struct {
	blueprint.BaseDependencyTag
	tag string
}

func sourceOrOutputDepTag(tag string) blueprint.DependencyTag {
	return sourceOrOutputDependencyTag{tag: tag}
}

var SourceDepTag = sourceOrOutputDepTag("")

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
				ctx.AddDependency(ctx.Module(), sourceOrOutputDepTag(t), m)
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
			ctx.AddDependency(ctx.Module(), sourceOrOutputDepTag(t), m)
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
	if len(paths) > 1 {
		ReportPathErrorf(ctx, "got multiple output files from module %q, expected exactly one",
			pathContextName(ctx, module))
		return nil
	}
	return paths[0]
}

func outputFilesForModule(ctx PathContext, module blueprint.Module, tag string) (Paths, error) {
	if outputFileProducer, ok := module.(OutputFileProducer); ok {
		paths, err := outputFileProducer.OutputFiles(tag)
		if err != nil {
			return nil, fmt.Errorf("failed to get output file from module %q: %s",
				pathContextName(ctx, module), err.Error())
		}
		if len(paths) == 0 {
			return nil, fmt.Errorf("failed to get output files from module %q", pathContextName(ctx, module))
		}
		return paths, nil
	} else if sourceFileProducer, ok := module.(SourceFileProducer); ok {
		if tag != "" {
			return nil, fmt.Errorf("module %q is a SourceFileProducer, not an OutputFileProducer, and so does not support tag %q", pathContextName(ctx, module), tag)
		}
		paths := sourceFileProducer.Srcs()
		if len(paths) == 0 {
			return nil, fmt.Errorf("failed to get output files from module %q", pathContextName(ctx, module))
		}
		return paths, nil
	} else {
		return nil, fmt.Errorf("module %q is not an OutputFileProducer", pathContextName(ctx, module))
	}
}

// Modules can implement HostToolProvider and return a valid OptionalPath from HostToolPath() to
// specify that they can be used as a tool by a genrule module.
type HostToolProvider interface {
	Module
	// HostToolPath returns the path to the host tool for the module if it is one, or an invalid
	// OptionalPath.
	HostToolPath() OptionalPath
}

// Returns a list of paths expanded from globs and modules referenced using ":module" syntax.  The property must
// be tagged with `android:"path" to support automatic source module dependency resolution.
//
// Deprecated: use PathsForModuleSrc or PathsForModuleSrcExcludes instead.
func (m *moduleContext) ExpandSources(srcFiles, excludes []string) Paths {
	return PathsForModuleSrcExcludes(m, srcFiles, excludes)
}

// Returns a single path expanded from globs and modules referenced using ":module" syntax.  The property must
// be tagged with `android:"path" to support automatic source module dependency resolution.
//
// Deprecated: use PathForModuleSrc instead.
func (m *moduleContext) ExpandSource(srcFile, prop string) Path {
	return PathForModuleSrc(m, srcFile)
}

// Returns an optional single path expanded from globs and modules referenced using ":module" syntax if
// the srcFile is non-nil.  The property must be tagged with `android:"path" to support automatic source module
// dependency resolution.
func (m *moduleContext) ExpandOptionalSource(srcFile *string, prop string) OptionalPath {
	if srcFile != nil {
		return OptionalPathForPath(PathForModuleSrc(m, *srcFile))
	}
	return OptionalPath{}
}

func (m *moduleContext) RequiredModuleNames() []string {
	return m.module.RequiredModuleNames()
}

func (m *moduleContext) HostRequiredModuleNames() []string {
	return m.module.HostRequiredModuleNames()
}

func (m *moduleContext) TargetRequiredModuleNames() []string {
	return m.module.TargetRequiredModuleNames()
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

	// Ensure ancestor directories are in modulesInDir
	dirs := SortedStringKeys(modulesInDir)
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
	for _, dir := range dirs {
		p := parentDir(dir)
		if p != "." && p != "/" {
			modulesInDir[p] = append(modulesInDir[p], PathForPhony(ctx, mmTarget(dir)))
		}
	}

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
		if module.Enabled() {
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
	for _, class := range SortedStringKeys(osClass) {
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
}

func CheckBlueprintSyntax(ctx BaseModuleContext, filename string, contents string) []error {
	bpctx := ctx.blueprintBaseModuleContext()
	return blueprint.CheckBlueprintSyntax(bpctx.ModuleFactories(), filename, contents)
}

// installPathsDepSet is a thin type-safe wrapper around the generic depSet.  It always uses
// topological order.
type installPathsDepSet struct {
	depSet
}

// newInstallPathsDepSet returns an immutable packagingSpecsDepSet with the given direct and
// transitive contents.
func newInstallPathsDepSet(direct InstallPaths, transitive []*installPathsDepSet) *installPathsDepSet {
	return &installPathsDepSet{*newDepSet(TOPOLOGICAL, direct, transitive)}
}

// ToList returns the installPathsDepSet flattened to a list in topological order.
func (d *installPathsDepSet) ToList() InstallPaths {
	if d == nil {
		return nil
	}
	return d.depSet.ToList().(InstallPaths)
}
