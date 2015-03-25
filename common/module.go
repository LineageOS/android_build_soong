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

package common

import (
	"path/filepath"

	"github.com/google/blueprint"
)

type Config interface {
	CpPreserveSymlinksFlags() string
	SrcDir() string
	Getenv(string) string
	EnvDeps() map[string]string
}

var (
	DeviceSharedLibrary = "shared_library"
	DeviceStaticLibrary = "static_library"
	DeviceExecutable    = "executable"
	HostSharedLibrary   = "host_shared_library"
	HostStaticLibrary   = "host_static_library"
	HostExecutable      = "host_executable"
)

type androidBaseContext interface {
	Arch() Arch
	Host() bool
	Device() bool
	Debug() bool
}

type AndroidBaseContext interface {
	blueprint.BaseModuleContext
	androidBaseContext
}

type AndroidModuleContext interface {
	blueprint.ModuleContext
	androidBaseContext

	InstallFile(installPath, srcPath string)
	CheckbuildFile(srcPath string)
}

type AndroidModule interface {
	blueprint.Module

	GenerateAndroidBuildActions(AndroidModuleContext)

	base() *AndroidModuleBase
	Disabled() bool
	HostOrDevice() HostOrDevice
}

type AndroidDynamicDepender interface {
	AndroidDynamicDependencies(ctx AndroidDynamicDependerModuleContext) []string
}

type AndroidDynamicDependerModuleContext interface {
	blueprint.DynamicDependerModuleContext
	androidBaseContext
}

type commonProperties struct {
	Name         string
	Deps         []string
	ResourceDirs []string

	// disabled: don't emit any build rules for this module
	Disabled bool `android:"arch_variant"`

	// multilib: control whether this module compiles for 32-bit, 64-bit, or both.  Possible values
	// are "32" (compile for 32-bit only), "64" (compile for 64-bit only), "both" (compile for both
	// architectures), or "first" (compile for 64-bit on a 64-bit platform, and 32-bit on a 32-bit
	// platform
	Compile_multilib string

	// Set by ArchMutator
	CompileArch Arch `blueprint:"mutated"`

	// Set by InitAndroidModule
	HostOrDeviceSupported HostOrDeviceSupported `blueprint:"mutated"`
}

type hostAndDeviceProperties struct {
	Host_supported   bool
	Device_supported bool
}

type Multilib string

const (
	MultilibBoth  Multilib = "both"
	MultilibFirst Multilib = "first"
)

func InitAndroidModule(m AndroidModule,
	propertyStructs ...interface{}) (blueprint.Module, []interface{}) {

	base := m.base()
	base.module = m

	propertyStructs = append(propertyStructs, &base.commonProperties)

	return m, propertyStructs
}

func InitAndroidArchModule(m AndroidModule, hod HostOrDeviceSupported, defaultMultilib Multilib,
	propertyStructs ...interface{}) (blueprint.Module, []interface{}) {

	_, propertyStructs = InitAndroidModule(m, propertyStructs...)

	base := m.base()
	base.commonProperties.HostOrDeviceSupported = hod

	if hod == HostAndDeviceSupported {
		// Default to module to device supported, host not supported, can override in module
		// properties
		base.hostAndDeviceProperties.Device_supported = true
		propertyStructs = append(propertyStructs, &base.hostAndDeviceProperties)
	}

	return InitArchModule(m, defaultMultilib, propertyStructs...)
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
type AndroidModuleBase struct {
	// Putting the curiously recurring thing pointing to the thing that contains
	// the thing pattern to good use.
	module AndroidModule

	commonProperties        commonProperties
	hostAndDeviceProperties hostAndDeviceProperties
	generalProperties       []interface{}
	archProperties          []*archProperties

	noAddressSanitizer bool
	installFiles       []string
	checkbuildFiles    []string
}

func (a *AndroidModuleBase) base() *AndroidModuleBase {
	return a
}

func (a *AndroidModuleBase) SetArch(arch Arch) {
	a.commonProperties.CompileArch = arch
}

func (a *AndroidModuleBase) HostOrDevice() HostOrDevice {
	return a.commonProperties.CompileArch.HostOrDevice
}

func (a *AndroidModuleBase) HostSupported() bool {
	return a.commonProperties.HostOrDeviceSupported == HostSupported ||
		a.commonProperties.HostOrDeviceSupported == HostAndDeviceSupported &&
			a.hostAndDeviceProperties.Host_supported
}

func (a *AndroidModuleBase) DeviceSupported() bool {
	return a.commonProperties.HostOrDeviceSupported == DeviceSupported ||
		a.commonProperties.HostOrDeviceSupported == HostAndDeviceSupported &&
			a.hostAndDeviceProperties.Device_supported
}

func (a *AndroidModuleBase) Disabled() bool {
	return a.commonProperties.Disabled
}

func (a *AndroidModuleBase) computeInstallDeps(
	ctx blueprint.ModuleContext) []string {

	result := []string{}
	ctx.VisitDepsDepthFirstIf(isFileInstaller,
		func(m blueprint.Module) {
			fileInstaller := m.(fileInstaller)
			files := fileInstaller.filesToInstall()
			result = append(result, files...)
		})

	return result
}

func (a *AndroidModuleBase) filesToInstall() []string {
	return a.installFiles
}

func (p *AndroidModuleBase) NoAddressSanitizer() bool {
	return p.noAddressSanitizer
}

func (p *AndroidModuleBase) resourceDirs() []string {
	return p.commonProperties.ResourceDirs
}

func (a *AndroidModuleBase) generateModuleTarget(ctx blueprint.ModuleContext) {
	if a != ctx.FinalModule().(AndroidModule).base() {
		return
	}

	allInstalledFiles := []string{}
	allCheckbuildFiles := []string{}
	ctx.VisitAllModuleVariants(func(module blueprint.Module) {
		if androidModule, ok := module.(AndroidModule); ok {
			files := androidModule.base().installFiles
			allInstalledFiles = append(allInstalledFiles, files...)
			files = androidModule.base().checkbuildFiles
			allCheckbuildFiles = append(allCheckbuildFiles, files...)
		}
	})

	deps := []string{}

	if len(allInstalledFiles) > 0 {
		name := ctx.ModuleName() + "-install"
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      blueprint.Phony,
			Outputs:   []string{name},
			Implicits: allInstalledFiles,
		})
		deps = append(deps, name)
	}

	if len(allCheckbuildFiles) > 0 {
		name := ctx.ModuleName() + "-checkbuild"
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      blueprint.Phony,
			Outputs:   []string{name},
			Implicits: allCheckbuildFiles,
			Optional:  true,
		})
		deps = append(deps, name)
	}

	if len(deps) > 0 {
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      blueprint.Phony,
			Outputs:   []string{ctx.ModuleName()},
			Implicits: deps,
			Optional:  true,
		})
	}
}

func (a *AndroidModuleBase) DynamicDependencies(ctx blueprint.DynamicDependerModuleContext) []string {
	actx := &androidDynamicDependerContext{
		DynamicDependerModuleContext: ctx,
		androidBaseContextImpl: androidBaseContextImpl{
			arch: a.commonProperties.CompileArch,
		},
	}

	if dynamic, ok := a.module.(AndroidDynamicDepender); ok {
		return dynamic.AndroidDynamicDependencies(actx)
	}

	return nil
}

func (a *AndroidModuleBase) GenerateBuildActions(ctx blueprint.ModuleContext) {
	androidCtx := &androidModuleContext{
		ModuleContext: ctx,
		androidBaseContextImpl: androidBaseContextImpl{
			arch: a.commonProperties.CompileArch,
		},
		installDeps:  a.computeInstallDeps(ctx),
		installFiles: a.installFiles,
	}

	if a.commonProperties.Disabled {
		return
	}

	a.module.GenerateAndroidBuildActions(androidCtx)
	if ctx.Failed() {
		return
	}

	a.generateModuleTarget(ctx)
	if ctx.Failed() {
		return
	}

	a.installFiles = append(a.installFiles, androidCtx.installFiles...)
	a.checkbuildFiles = append(a.checkbuildFiles, androidCtx.checkbuildFiles...)
}

type androidBaseContextImpl struct {
	arch  Arch
	debug bool
}

type androidModuleContext struct {
	blueprint.ModuleContext
	androidBaseContextImpl
	installDeps     []string
	installFiles    []string
	checkbuildFiles []string
}

func (a *androidModuleContext) Build(pctx *blueprint.PackageContext, params blueprint.BuildParams) {
	params.Optional = true
	a.ModuleContext.Build(pctx, params)
}

func (a *androidBaseContextImpl) Arch() Arch {
	return a.arch
}

func (a *androidBaseContextImpl) Host() bool {
	return a.arch.HostOrDevice.Host()
}

func (a *androidBaseContextImpl) Device() bool {
	return a.arch.HostOrDevice.Device()
}

func (a *androidBaseContextImpl) Debug() bool {
	return a.debug
}

func (a *androidModuleContext) InstallFile(installPath, srcPath string) {
	var fullInstallPath string
	if a.arch.HostOrDevice.Device() {
		// TODO: replace unset with a device name once we have device targeting
		fullInstallPath = filepath.Join("out/target/product/unset/system", installPath,
			filepath.Base(srcPath))
	} else {
		// TODO: replace unset with a host name
		fullInstallPath = filepath.Join("out/host/unset/", installPath, filepath.Base(srcPath))
	}

	a.ModuleContext.Build(pctx, blueprint.BuildParams{
		Rule:      Cp,
		Outputs:   []string{fullInstallPath},
		Inputs:    []string{srcPath},
		OrderOnly: a.installDeps,
	})

	a.installFiles = append(a.installFiles, fullInstallPath)
}

func (a *androidModuleContext) CheckbuildFile(srcPath string) {
	a.checkbuildFiles = append(a.checkbuildFiles, srcPath)
}

type androidDynamicDependerContext struct {
	blueprint.DynamicDependerModuleContext
	androidBaseContextImpl
}

type fileInstaller interface {
	filesToInstall() []string
}

func isFileInstaller(m blueprint.Module) bool {
	_, ok := m.(fileInstaller)
	return ok
}

func isAndroidModule(m blueprint.Module) bool {
	_, ok := m.(AndroidModule)
	return ok
}
