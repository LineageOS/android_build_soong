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
	"path/filepath"
	"strings"

	"android/soong/glob"

	"github.com/google/blueprint"
)

var (
	DeviceSharedLibrary = "shared_library"
	DeviceStaticLibrary = "static_library"
	DeviceExecutable    = "executable"
	HostSharedLibrary   = "host_shared_library"
	HostStaticLibrary   = "host_static_library"
	HostExecutable      = "host_executable"
)

type ModuleBuildParams struct {
	Rule      blueprint.Rule
	Output    WritablePath
	Outputs   WritablePaths
	Input     Path
	Inputs    Paths
	Implicit  Path
	Implicits Paths
	OrderOnly Paths
	Default   bool
	Args      map[string]string
}

type androidBaseContext interface {
	Target() Target
	TargetPrimary() bool
	Arch() Arch
	Os() OsType
	Host() bool
	Device() bool
	Darwin() bool
	Debug() bool
	PrimaryArch() bool
	AConfig() Config
	DeviceConfig() DeviceConfig
}

type BaseContext interface {
	blueprint.BaseModuleContext
	androidBaseContext
}

type ModuleContext interface {
	blueprint.ModuleContext
	androidBaseContext

	// Similar to Build, but takes Paths instead of []string,
	// and performs more verification.
	ModuleBuild(pctx blueprint.PackageContext, params ModuleBuildParams)

	ExpandSources(srcFiles, excludes []string) Paths
	Glob(outDir, globPattern string, excludes []string) Paths

	InstallFile(installPath OutputPath, srcPath Path, deps ...Path) OutputPath
	InstallFileName(installPath OutputPath, name string, srcPath Path, deps ...Path) OutputPath
	InstallSymlink(installPath OutputPath, name string, srcPath OutputPath) OutputPath
	CheckbuildFile(srcPath Path)

	AddMissingDependencies(deps []string)

	Proprietary() bool
	InstallInData() bool
}

type Module interface {
	blueprint.Module

	GenerateAndroidBuildActions(ModuleContext)
	DepsMutator(BottomUpMutatorContext)

	base() *ModuleBase
	Enabled() bool
	Target() Target
	InstallInData() bool
}

type nameProperties struct {
	// The name of the module.  Must be unique across all modules.
	Name string
}

type commonProperties struct {
	Tags []string

	// emit build rules for this module
	Enabled *bool `android:"arch_variant"`

	// control whether this module compiles for 32-bit, 64-bit, or both.  Possible values
	// are "32" (compile for 32-bit only), "64" (compile for 64-bit only), "both" (compile for both
	// architectures), or "first" (compile for 64-bit on a 64-bit platform, and 32-bit on a 32-bit
	// platform
	Compile_multilib string `android:"arch_variant"`

	Target struct {
		Host struct {
			Compile_multilib string
		}
		Android struct {
			Compile_multilib string
		}
	}

	Default_multilib string `blueprint:"mutated"`

	// whether this is a proprietary vendor module, and should be installed into /vendor
	Proprietary bool

	// *.logtags files, to combine together in order to generate the /system/etc/event-log-tags
	// file
	Logtags []string

	// init.rc files to be installed if this module is installed
	Init_rc []string

	// names of other modules to install if this module is installed
	Required []string

	// Set by TargetMutator
	CompileTarget  Target `blueprint:"mutated"`
	CompilePrimary bool   `blueprint:"mutated"`

	// Set by InitAndroidModule
	HostOrDeviceSupported HostOrDeviceSupported `blueprint:"mutated"`
	ArchSpecific          bool                  `blueprint:"mutated"`

	SkipInstall bool `blueprint:"mutated"`
}

type hostAndDeviceProperties struct {
	Host_supported   *bool
	Device_supported *bool
}

type Multilib string

const (
	MultilibBoth    Multilib = "both"
	MultilibFirst   Multilib = "first"
	MultilibCommon  Multilib = "common"
	MultilibDefault Multilib = ""
)

type HostOrDeviceSupported int

const (
	_ HostOrDeviceSupported = iota
	HostSupported
	HostSupportedNoCross
	DeviceSupported
	HostAndDeviceSupported
	HostAndDeviceDefault
	NeitherHostNorDeviceSupported
)

func InitAndroidModule(m Module,
	propertyStructs ...interface{}) (blueprint.Module, []interface{}) {

	base := m.base()
	base.module = m

	propertyStructs = append(propertyStructs,
		&base.nameProperties,
		&base.commonProperties,
		&base.variableProperties)

	return m, propertyStructs
}

func InitAndroidArchModule(m Module, hod HostOrDeviceSupported, defaultMultilib Multilib,
	propertyStructs ...interface{}) (blueprint.Module, []interface{}) {

	_, propertyStructs = InitAndroidModule(m, propertyStructs...)

	base := m.base()
	base.commonProperties.HostOrDeviceSupported = hod
	base.commonProperties.Default_multilib = string(defaultMultilib)
	base.commonProperties.ArchSpecific = true

	switch hod {
	case HostAndDeviceSupported:
		// Default to module to device supported, host not supported, can override in module
		// properties
		base.hostAndDeviceProperties.Device_supported = boolPtr(true)
		fallthrough
	case HostAndDeviceDefault:
		propertyStructs = append(propertyStructs, &base.hostAndDeviceProperties)
	}

	return InitArchModule(m, propertyStructs...)
}

// A AndroidModuleBase object contains the properties that are common to all Android
// modules.  It should be included as an anonymous field in every module
// struct definition.  InitAndroidModule should then be called from the module's
// factory function, and the return values from InitAndroidModule should be
// returned from the factory function.
//
// The AndroidModuleBase type is responsible for implementing the
// GenerateBuildActions method to support the blueprint.Module interface. This
// method will then call the module's GenerateAndroidBuildActions method once
// for each build variant that is to be built. GenerateAndroidBuildActions is
// passed a AndroidModuleContext rather than the usual blueprint.ModuleContext.
// AndroidModuleContext exposes extra functionality specific to the Android build
// system including details about the particular build variant that is to be
// generated.
//
// For example:
//
//     import (
//         "android/soong/common"
//         "github.com/google/blueprint"
//     )
//
//     type myModule struct {
//         common.AndroidModuleBase
//         properties struct {
//             MyProperty string
//         }
//     }
//
//     func NewMyModule() (blueprint.Module, []interface{}) {
//         m := &myModule{}
//         return common.InitAndroidModule(m, &m.properties)
//     }
//
//     func (m *myModule) GenerateAndroidBuildActions(ctx common.AndroidModuleContext) {
//         // Get the CPU architecture for the current build variant.
//         variantArch := ctx.Arch()
//
//         // ...
//     }
type ModuleBase struct {
	// Putting the curiously recurring thing pointing to the thing that contains
	// the thing pattern to good use.
	module Module

	nameProperties          nameProperties
	commonProperties        commonProperties
	variableProperties      variableProperties
	hostAndDeviceProperties hostAndDeviceProperties
	generalProperties       []interface{}
	archProperties          []interface{}
	customizableProperties  []interface{}

	noAddressSanitizer bool
	installFiles       Paths
	checkbuildFiles    Paths

	// Used by buildTargetSingleton to create checkbuild and per-directory build targets
	// Only set on the final variant of each module
	installTarget    string
	checkbuildTarget string
	blueprintDir     string

	hooks hooks
}

// Name returns the name of the module.  It may be overridden by individual module types, for
// example prebuilts will prepend prebuilt_ to the name.
func (a *ModuleBase) Name() string {
	return a.nameProperties.Name
}

// BaseModuleName returns the name of the module as specified in the blueprints file.
func (a *ModuleBase) BaseModuleName() string {
	return a.nameProperties.Name
}

func (a *ModuleBase) base() *ModuleBase {
	return a
}

func (a *ModuleBase) SetTarget(target Target, primary bool) {
	a.commonProperties.CompileTarget = target
	a.commonProperties.CompilePrimary = primary
}

func (a *ModuleBase) Target() Target {
	return a.commonProperties.CompileTarget
}

func (a *ModuleBase) TargetPrimary() bool {
	return a.commonProperties.CompilePrimary
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
	case HostAndDeviceSupported:
		var supported []OsClass
		if Bool(a.hostAndDeviceProperties.Host_supported) {
			supported = append(supported, Host, HostCross)
		}
		if Bool(a.hostAndDeviceProperties.Device_supported) {
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
			Bool(a.hostAndDeviceProperties.Device_supported)
}

func (a *ModuleBase) Enabled() bool {
	if a.commonProperties.Enabled == nil {
		return a.Os().Class != HostCross
	}
	return *a.commonProperties.Enabled
}

func (a *ModuleBase) SkipInstall() {
	a.commonProperties.SkipInstall = true
}

func (a *ModuleBase) computeInstallDeps(
	ctx blueprint.ModuleContext) Paths {

	result := Paths{}
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

func (a *ModuleBase) generateModuleTarget(ctx blueprint.ModuleContext) {
	allInstalledFiles := Paths{}
	allCheckbuildFiles := Paths{}
	ctx.VisitAllModuleVariants(func(module blueprint.Module) {
		a := module.(Module).base()
		allInstalledFiles = append(allInstalledFiles, a.installFiles...)
		allCheckbuildFiles = append(allCheckbuildFiles, a.checkbuildFiles...)
	})

	deps := []string{}

	if len(allInstalledFiles) > 0 {
		name := ctx.ModuleName() + "-install"
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      blueprint.Phony,
			Outputs:   []string{name},
			Implicits: allInstalledFiles.Strings(),
			Optional:  ctx.Config().(Config).EmbeddedInMake(),
		})
		deps = append(deps, name)
		a.installTarget = name
	}

	if len(allCheckbuildFiles) > 0 {
		name := ctx.ModuleName() + "-checkbuild"
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      blueprint.Phony,
			Outputs:   []string{name},
			Implicits: allCheckbuildFiles.Strings(),
			Optional:  true,
		})
		deps = append(deps, name)
		a.checkbuildTarget = name
	}

	if len(deps) > 0 {
		suffix := ""
		if ctx.Config().(Config).EmbeddedInMake() {
			suffix = "-soong"
		}

		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      blueprint.Phony,
			Outputs:   []string{ctx.ModuleName() + suffix},
			Implicits: deps,
			Optional:  true,
		})

		a.blueprintDir = ctx.ModuleDir()
	}
}

func (a *ModuleBase) androidBaseContextFactory(ctx blueprint.BaseModuleContext) androidBaseContextImpl {
	return androidBaseContextImpl{
		target:        a.commonProperties.CompileTarget,
		targetPrimary: a.commonProperties.CompilePrimary,
		config:        ctx.Config().(Config),
	}
}

func (a *ModuleBase) GenerateBuildActions(ctx blueprint.ModuleContext) {
	androidCtx := &androidModuleContext{
		module:                 a.module,
		ModuleContext:          ctx,
		androidBaseContextImpl: a.androidBaseContextFactory(ctx),
		installDeps:            a.computeInstallDeps(ctx),
		installFiles:           a.installFiles,
		missingDeps:            ctx.GetMissingDependencies(),
	}

	if a.Enabled() {
		a.module.GenerateAndroidBuildActions(androidCtx)
		if ctx.Failed() {
			return
		}

		a.installFiles = append(a.installFiles, androidCtx.installFiles...)
		a.checkbuildFiles = append(a.checkbuildFiles, androidCtx.checkbuildFiles...)
	}

	if a == ctx.FinalModule().(Module).base() {
		a.generateModuleTarget(ctx)
		if ctx.Failed() {
			return
		}
	}
}

type androidBaseContextImpl struct {
	target        Target
	targetPrimary bool
	debug         bool
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
}

func (a *androidModuleContext) ninjaError(outputs []string, err error) {
	a.ModuleContext.Build(pctx, blueprint.BuildParams{
		Rule:     ErrorRule,
		Outputs:  outputs,
		Optional: true,
		Args: map[string]string{
			"error": err.Error(),
		},
	})
	return
}

func (a *androidModuleContext) Build(pctx blueprint.PackageContext, params blueprint.BuildParams) {
	if a.missingDeps != nil && params.Rule != globRule {
		a.ninjaError(params.Outputs, fmt.Errorf("module %s missing dependencies: %s\n",
			a.ModuleName(), strings.Join(a.missingDeps, ", ")))
		return
	}

	params.Optional = true
	a.ModuleContext.Build(pctx, params)
}

func (a *androidModuleContext) ModuleBuild(pctx blueprint.PackageContext, params ModuleBuildParams) {
	bparams := blueprint.BuildParams{
		Rule:      params.Rule,
		Outputs:   params.Outputs.Strings(),
		Inputs:    params.Inputs.Strings(),
		Implicits: params.Implicits.Strings(),
		OrderOnly: params.OrderOnly.Strings(),
		Args:      params.Args,
		Optional:  !params.Default,
	}

	if params.Output != nil {
		bparams.Outputs = append(bparams.Outputs, params.Output.String())
	}
	if params.Input != nil {
		bparams.Inputs = append(bparams.Inputs, params.Input.String())
	}
	if params.Implicit != nil {
		bparams.Implicits = append(bparams.Implicits, params.Implicit.String())
	}

	if a.missingDeps != nil {
		a.ninjaError(bparams.Outputs, fmt.Errorf("module %s missing dependencies: %s\n",
			a.ModuleName(), strings.Join(a.missingDeps, ", ")))
		return
	}

	a.ModuleContext.Build(pctx, bparams)
}

func (a *androidModuleContext) GetMissingDependencies() []string {
	return a.missingDeps
}

func (a *androidModuleContext) AddMissingDependencies(deps []string) {
	if deps != nil {
		a.missingDeps = append(a.missingDeps, deps...)
	}
}

func (a *androidBaseContextImpl) Target() Target {
	return a.target
}

func (a *androidBaseContextImpl) TargetPrimary() bool {
	return a.targetPrimary
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

func (a *androidBaseContextImpl) Debug() bool {
	return a.debug
}

func (a *androidBaseContextImpl) PrimaryArch() bool {
	return a.target.Arch.ArchType == a.config.Targets[a.target.Os.Class][0].Arch.ArchType
}

func (a *androidBaseContextImpl) AConfig() Config {
	return a.config
}

func (a *androidBaseContextImpl) DeviceConfig() DeviceConfig {
	return DeviceConfig{a.config.deviceConfig}
}

func (a *androidModuleContext) Proprietary() bool {
	return a.module.base().commonProperties.Proprietary
}

func (a *androidModuleContext) InstallInData() bool {
	return a.module.InstallInData()
}

func (a *androidModuleContext) InstallFileName(installPath OutputPath, name string, srcPath Path,
	deps ...Path) OutputPath {

	fullInstallPath := installPath.Join(a, name)
	a.module.base().hooks.runInstallHooks(a, fullInstallPath, false)

	if !a.module.base().commonProperties.SkipInstall &&
		(a.Host() || !a.AConfig().SkipDeviceInstall()) {

		deps = append(deps, a.installDeps...)

		var implicitDeps, orderOnlyDeps Paths

		if a.Host() {
			// Installed host modules might be used during the build, depend directly on their
			// dependencies so their timestamp is updated whenever their dependency is updated
			implicitDeps = deps
		} else {
			orderOnlyDeps = deps
		}

		a.ModuleBuild(pctx, ModuleBuildParams{
			Rule:      Cp,
			Output:    fullInstallPath,
			Input:     srcPath,
			Implicits: implicitDeps,
			OrderOnly: orderOnlyDeps,
			Default:   !a.AConfig().EmbeddedInMake(),
		})

		a.installFiles = append(a.installFiles, fullInstallPath)
	}
	a.checkbuildFiles = append(a.checkbuildFiles, srcPath)
	return fullInstallPath
}

func (a *androidModuleContext) InstallFile(installPath OutputPath, srcPath Path, deps ...Path) OutputPath {
	return a.InstallFileName(installPath, filepath.Base(srcPath.String()), srcPath, deps...)
}

func (a *androidModuleContext) InstallSymlink(installPath OutputPath, name string, srcPath OutputPath) OutputPath {
	fullInstallPath := installPath.Join(a, name)
	a.module.base().hooks.runInstallHooks(a, fullInstallPath, true)

	if !a.module.base().commonProperties.SkipInstall &&
		(a.Host() || !a.AConfig().SkipDeviceInstall()) {

		a.ModuleBuild(pctx, ModuleBuildParams{
			Rule:      Symlink,
			Output:    fullInstallPath,
			OrderOnly: Paths{srcPath},
			Default:   !a.AConfig().EmbeddedInMake(),
			Args: map[string]string{
				"fromPath": srcPath.String(),
			},
		})

		a.installFiles = append(a.installFiles, fullInstallPath)
		a.checkbuildFiles = append(a.checkbuildFiles, srcPath)
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

func (ctx *androidModuleContext) ExpandSources(srcFiles, excludes []string) Paths {
	prefix := PathForModuleSrc(ctx).String()
	for i, e := range excludes {
		j := findStringInSlice(e, srcFiles)
		if j != -1 {
			srcFiles = append(srcFiles[:j], srcFiles[j+1:]...)
		}

		excludes[i] = filepath.Join(prefix, e)
	}

	globbedSrcFiles := make(Paths, 0, len(srcFiles))
	for _, s := range srcFiles {
		if glob.IsGlob(s) {
			globbedSrcFiles = append(globbedSrcFiles, ctx.Glob("src_glob", filepath.Join(prefix, s), excludes)...)
		} else {
			globbedSrcFiles = append(globbedSrcFiles, PathForModuleSrc(ctx, s))
		}
	}

	return globbedSrcFiles
}

func (ctx *androidModuleContext) Glob(outDir, globPattern string, excludes []string) Paths {
	ret, err := Glob(ctx, PathForModuleOut(ctx, outDir).String(), globPattern, excludes)
	if err != nil {
		ctx.ModuleErrorf("glob: %s", err.Error())
	}
	return pathsForModuleSrcFromFullPath(ctx, ret)
}

func init() {
	RegisterSingletonType("buildtarget", BuildTargetSingleton)
}

func BuildTargetSingleton() blueprint.Singleton {
	return &buildTargetSingleton{}
}

type buildTargetSingleton struct{}

func (c *buildTargetSingleton) GenerateBuildActions(ctx blueprint.SingletonContext) {
	checkbuildDeps := []string{}

	dirModules := make(map[string][]string)

	ctx.VisitAllModules(func(module blueprint.Module) {
		if a, ok := module.(Module); ok {
			blueprintDir := a.base().blueprintDir
			installTarget := a.base().installTarget
			checkbuildTarget := a.base().checkbuildTarget

			if checkbuildTarget != "" {
				checkbuildDeps = append(checkbuildDeps, checkbuildTarget)
				dirModules[blueprintDir] = append(dirModules[blueprintDir], checkbuildTarget)
			}

			if installTarget != "" {
				dirModules[blueprintDir] = append(dirModules[blueprintDir], installTarget)
			}
		}
	})

	suffix := ""
	if ctx.Config().(Config).EmbeddedInMake() {
		suffix = "-soong"
	}

	// Create a top-level checkbuild target that depends on all modules
	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      blueprint.Phony,
		Outputs:   []string{"checkbuild" + suffix},
		Implicits: checkbuildDeps,
		Optional:  true,
	})

	// Create a mm/<directory> target that depends on all modules in a directory
	dirs := sortedKeys(dirModules)
	for _, dir := range dirs {
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      blueprint.Phony,
			Outputs:   []string{filepath.Join("mm", dir)},
			Implicits: dirModules[dir],
			// HACK: checkbuild should be an optional build, but force it
			// enabled for now in standalone builds
			Optional: ctx.Config().(Config).EmbeddedInMake(),
		})
	}
}

type AndroidModulesByName struct {
	slice []Module
	ctx   interface {
		ModuleName(blueprint.Module) string
		ModuleSubDir(blueprint.Module) string
	}
}

func (s AndroidModulesByName) Len() int { return len(s.slice) }
func (s AndroidModulesByName) Less(i, j int) bool {
	mi, mj := s.slice[i], s.slice[j]
	ni, nj := s.ctx.ModuleName(mi), s.ctx.ModuleName(mj)

	if ni != nj {
		return ni < nj
	} else {
		return s.ctx.ModuleSubDir(mi) < s.ctx.ModuleSubDir(mj)
	}
}
func (s AndroidModulesByName) Swap(i, j int) { s.slice[i], s.slice[j] = s.slice[j], s.slice[i] }
