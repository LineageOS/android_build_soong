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
	"android/soong"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"android/soong/glob"

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

type androidBaseContext interface {
	Arch() Arch
	HostOrDevice() HostOrDevice
	Host() bool
	Device() bool
	Darwin() bool
	Debug() bool
	AConfig() Config
}

type AndroidBaseContext interface {
	blueprint.BaseModuleContext
	androidBaseContext
}

type AndroidModuleContext interface {
	blueprint.ModuleContext
	androidBaseContext

	ExpandSources(srcFiles, excludes []string) []string
	Glob(outDir, globPattern string, excludes []string) []string

	InstallFile(installPath, srcPath string, deps ...string) string
	InstallFileName(installPath, name, srcPath string, deps ...string) string
	CheckbuildFile(srcPath string)
}

type AndroidModule interface {
	blueprint.Module

	GenerateAndroidBuildActions(AndroidModuleContext)

	base() *AndroidModuleBase
	Disabled() bool
	HostOrDevice() HostOrDevice
}

type commonProperties struct {
	Name string
	Deps []string
	Tags []string

	// don't emit any build rules for this module
	Disabled *bool `android:"arch_variant"`

	// control whether this module compiles for 32-bit, 64-bit, or both.  Possible values
	// are "32" (compile for 32-bit only), "64" (compile for 64-bit only), "both" (compile for both
	// architectures), or "first" (compile for 64-bit on a 64-bit platform, and 32-bit on a 32-bit
	// platform
	Compile_multilib string

	// Set by HostOrDeviceMutator
	CompileHostOrDevice HostOrDevice `blueprint:"mutated"`

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
	MultilibBoth   Multilib = "both"
	MultilibFirst  Multilib = "first"
	MultilibCommon Multilib = "common"
)

func InitAndroidModule(m AndroidModule,
	propertyStructs ...interface{}) (blueprint.Module, []interface{}) {

	base := m.base()
	base.module = m

	propertyStructs = append(propertyStructs, &base.commonProperties, &base.variableProperties)

	return m, propertyStructs
}

func InitAndroidArchModule(m AndroidModule, hod HostOrDeviceSupported, defaultMultilib Multilib,
	propertyStructs ...interface{}) (blueprint.Module, []interface{}) {

	_, propertyStructs = InitAndroidModule(m, propertyStructs...)

	base := m.base()
	base.commonProperties.HostOrDeviceSupported = hod
	base.commonProperties.Compile_multilib = string(defaultMultilib)

	if hod == HostAndDeviceSupported {
		// Default to module to device supported, host not supported, can override in module
		// properties
		base.hostAndDeviceProperties.Device_supported = true
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
type AndroidModuleBase struct {
	// Putting the curiously recurring thing pointing to the thing that contains
	// the thing pattern to good use.
	module AndroidModule

	commonProperties        commonProperties
	variableProperties      variableProperties
	hostAndDeviceProperties hostAndDeviceProperties
	generalProperties       []interface{}
	archProperties          []*archProperties

	noAddressSanitizer bool
	installFiles       []string
	checkbuildFiles    []string

	// Used by buildTargetSingleton to create checkbuild and per-directory build targets
	// Only set on the final variant of each module
	installTarget    string
	checkbuildTarget string
	blueprintDir     string
}

func (a *AndroidModuleBase) base() *AndroidModuleBase {
	return a
}

func (a *AndroidModuleBase) SetHostOrDevice(hod HostOrDevice) {
	a.commonProperties.CompileHostOrDevice = hod
}

func (a *AndroidModuleBase) SetArch(arch Arch) {
	a.commonProperties.CompileArch = arch
}

func (a *AndroidModuleBase) HostOrDevice() HostOrDevice {
	return a.commonProperties.CompileHostOrDevice
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
	return proptools.Bool(a.commonProperties.Disabled)
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

func (a *AndroidModuleBase) generateModuleTarget(ctx blueprint.ModuleContext) {
	if a != ctx.FinalModule().(AndroidModule).base() {
		return
	}

	allInstalledFiles := []string{}
	allCheckbuildFiles := []string{}
	ctx.VisitAllModuleVariants(func(module blueprint.Module) {
		a := module.(AndroidModule).base()
		allInstalledFiles = append(allInstalledFiles, a.installFiles...)
		allCheckbuildFiles = append(allCheckbuildFiles, a.checkbuildFiles...)
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
		a.installTarget = name
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
		a.checkbuildTarget = name
	}

	if len(deps) > 0 {
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      blueprint.Phony,
			Outputs:   []string{ctx.ModuleName()},
			Implicits: deps,
			Optional:  true,
		})

		a.blueprintDir = ctx.ModuleDir()
	}
}

func (a *AndroidModuleBase) androidBaseContextFactory(ctx blueprint.BaseModuleContext) androidBaseContextImpl {
	return androidBaseContextImpl{
		arch:   a.commonProperties.CompileArch,
		hod:    a.commonProperties.CompileHostOrDevice,
		config: ctx.Config().(Config),
	}
}

func (a *AndroidModuleBase) GenerateBuildActions(ctx blueprint.ModuleContext) {
	androidCtx := &androidModuleContext{
		ModuleContext:          ctx,
		androidBaseContextImpl: a.androidBaseContextFactory(ctx),
		installDeps:            a.computeInstallDeps(ctx),
		installFiles:           a.installFiles,
	}

	if proptools.Bool(a.commonProperties.Disabled) {
		return
	}

	a.module.GenerateAndroidBuildActions(androidCtx)
	if ctx.Failed() {
		return
	}

	a.installFiles = append(a.installFiles, androidCtx.installFiles...)
	a.checkbuildFiles = append(a.checkbuildFiles, androidCtx.checkbuildFiles...)

	a.generateModuleTarget(ctx)
	if ctx.Failed() {
		return
	}
}

type androidBaseContextImpl struct {
	arch   Arch
	hod    HostOrDevice
	debug  bool
	config Config
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

func (a *androidBaseContextImpl) HostOrDevice() HostOrDevice {
	return a.hod
}

func (a *androidBaseContextImpl) Host() bool {
	return a.hod.Host()
}

func (a *androidBaseContextImpl) Device() bool {
	return a.hod.Device()
}

func (a *androidBaseContextImpl) Darwin() bool {
	return a.hod.Host() && runtime.GOOS == "darwin"
}

func (a *androidBaseContextImpl) Debug() bool {
	return a.debug
}

func (a *androidBaseContextImpl) AConfig() Config {
	return a.config
}

func (a *androidModuleContext) InstallFileName(installPath, name, srcPath string,
	deps ...string) string {

	config := a.AConfig()
	var fullInstallPath string
	if a.hod.Device() {
		// TODO: replace unset with a device name once we have device targeting
		fullInstallPath = filepath.Join(config.DeviceOut(), "system",
			installPath, name)
	} else {
		fullInstallPath = filepath.Join(config.HostOut(), installPath, name)
	}

	deps = append(deps, a.installDeps...)

	a.ModuleContext.Build(pctx, blueprint.BuildParams{
		Rule:      Cp,
		Outputs:   []string{fullInstallPath},
		Inputs:    []string{srcPath},
		OrderOnly: deps,
	})

	a.installFiles = append(a.installFiles, fullInstallPath)
	a.checkbuildFiles = append(a.checkbuildFiles, srcPath)
	return fullInstallPath
}

func (a *androidModuleContext) InstallFile(installPath, srcPath string, deps ...string) string {
	return a.InstallFileName(installPath, filepath.Base(srcPath), srcPath, deps...)
}

func (a *androidModuleContext) CheckbuildFile(srcPath string) {
	a.checkbuildFiles = append(a.checkbuildFiles, srcPath)
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

func findStringInSlice(str string, slice []string) int {
	for i, s := range slice {
		if s == str {
			return i
		}
	}
	return -1
}

func (ctx *androidModuleContext) ExpandSources(srcFiles, excludes []string) []string {
	prefix := ModuleSrcDir(ctx)
	for i, e := range excludes {
		j := findStringInSlice(e, srcFiles)
		if j != -1 {
			srcFiles = append(srcFiles[:j], srcFiles[j+1:]...)
		}

		excludes[i] = filepath.Join(prefix, e)
	}

	for i, srcFile := range srcFiles {
		srcFiles[i] = filepath.Join(prefix, srcFile)
	}

	if !hasGlob(srcFiles) {
		return srcFiles
	}

	globbedSrcFiles := make([]string, 0, len(srcFiles))
	for _, s := range srcFiles {
		if glob.IsGlob(s) {
			globbedSrcFiles = append(globbedSrcFiles, ctx.Glob("src_glob", s, excludes)...)
		} else {
			globbedSrcFiles = append(globbedSrcFiles, s)
		}
	}

	return globbedSrcFiles
}

func (ctx *androidModuleContext) Glob(outDir, globPattern string, excludes []string) []string {
	ret, err := Glob(ctx, filepath.Join(ModuleOutDir(ctx), outDir), globPattern, excludes)
	if err != nil {
		ctx.ModuleErrorf("glob: %s", err.Error())
	}
	return ret
}

func init() {
	soong.RegisterSingletonType("buildtarget", BuildTargetSingleton)
}

func BuildTargetSingleton() blueprint.Singleton {
	return &buildTargetSingleton{}
}

type buildTargetSingleton struct{}

func (c *buildTargetSingleton) GenerateBuildActions(ctx blueprint.SingletonContext) {
	checkbuildDeps := []string{}

	dirModules := make(map[string][]string)
	hasBPFile := make(map[string]bool)
	bpFiles := []string{}

	ctx.VisitAllModules(func(module blueprint.Module) {
		if a, ok := module.(AndroidModule); ok {
			blueprintDir := a.base().blueprintDir
			installTarget := a.base().installTarget
			checkbuildTarget := a.base().checkbuildTarget
			bpFile := ctx.BlueprintFile(module)

			if checkbuildTarget != "" {
				checkbuildDeps = append(checkbuildDeps, checkbuildTarget)
				dirModules[blueprintDir] = append(dirModules[blueprintDir], checkbuildTarget)
			}

			if installTarget != "" {
				dirModules[blueprintDir] = append(dirModules[blueprintDir], installTarget)
			}

			if !hasBPFile[bpFile] {
				hasBPFile[bpFile] = true
				bpFiles = append(bpFiles, bpFile)
			}
		}
	})

	// Create a top-level checkbuild target that depends on all modules
	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      blueprint.Phony,
		Outputs:   []string{"checkbuild"},
		Implicits: checkbuildDeps,
		// HACK: checkbuild should be an optional build, but force it enabled for now
		//Optional:  true,
	})

	// Create a mm/<directory> target that depends on all modules in a directory
	dirs := sortedKeys(dirModules)
	for _, dir := range dirs {
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      blueprint.Phony,
			Outputs:   []string{filepath.Join("mm", dir)},
			Implicits: dirModules[dir],
			Optional:  true,
		})
	}

	// Create Android.bp->mk translation rules
	androidMks := []string{}
	srcDir := ctx.Config().(Config).SrcDir()
	intermediatesDir := filepath.Join(ctx.Config().(Config).IntermediatesDir(), "androidmk")
	sort.Strings(bpFiles)
	for _, origBp := range bpFiles {
		bpFile := filepath.Join(srcDir, origBp)
		mkFile := filepath.Join(srcDir, filepath.Dir(origBp), "Android.mk")

		files, err := Glob(ctx, intermediatesDir, mkFile, nil)
		if err != nil {
			ctx.Errorf("glob: %s", err.Error())
			continue
		}

		// Existing Android.mk file, use that instead
		if len(files) > 0 {
			for _, file := range files {
				ctx.AddNinjaFileDeps(file)
			}
			continue
		}

		transMk := filepath.Join("androidmk", "Android_"+strings.Replace(filepath.Dir(origBp), "/", "_", -1)+".mk")
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      androidbp,
			Outputs:   []string{transMk},
			Inputs:    []string{bpFile},
			Implicits: []string{androidbpCmd},
			Optional:  true,
		})

		androidMks = append(androidMks, transMk)
	}

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      blueprint.Phony,
		Outputs:   []string{"androidmk"},
		Implicits: androidMks,
		Optional:  true,
	})
}
