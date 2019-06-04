// Copyright 2017 Google Inc. All rights reserved.
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

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"android/soong/android"
	"android/soong/cc/config"
)

type VndkProperties struct {
	Vndk struct {
		// declared as a VNDK or VNDK-SP module. The vendor variant
		// will be installed in /system instead of /vendor partition.
		//
		// `vendor_vailable` must be explicitly set to either true or
		// false together with `vndk: {enabled: true}`.
		Enabled *bool

		// declared as a VNDK-SP module, which is a subset of VNDK.
		//
		// `vndk: { enabled: true }` must set together.
		//
		// All these modules are allowed to link to VNDK-SP or LL-NDK
		// modules only. Other dependency will cause link-type errors.
		//
		// If `support_system_process` is not set or set to false,
		// the module is VNDK-core and can link to other VNDK-core,
		// VNDK-SP or LL-NDK modules only.
		Support_system_process *bool

		// Extending another module
		Extends *string

		// for vndk_prebuilt_shared, this is set by "version" property.
		// Otherwise, this is set as PLATFORM_VNDK_VERSION.
		Version string `blueprint:"mutated"`
	}
}

type vndkdep struct {
	Properties VndkProperties
}

func (vndk *vndkdep) props() []interface{} {
	return []interface{}{&vndk.Properties}
}

func (vndk *vndkdep) begin(ctx BaseModuleContext) {}

func (vndk *vndkdep) deps(ctx BaseModuleContext, deps Deps) Deps {
	return deps
}

func (vndk *vndkdep) isVndk() bool {
	return Bool(vndk.Properties.Vndk.Enabled)
}

func (vndk *vndkdep) isVndkSp() bool {
	return Bool(vndk.Properties.Vndk.Support_system_process)
}

func (vndk *vndkdep) isVndkExt() bool {
	return vndk.Properties.Vndk.Extends != nil
}

func (vndk *vndkdep) getVndkExtendsModuleName() string {
	return String(vndk.Properties.Vndk.Extends)
}

func (vndk *vndkdep) typeName() string {
	if !vndk.isVndk() {
		return "native:vendor"
	}
	if !vndk.isVndkExt() {
		if !vndk.isVndkSp() {
			return "native:vendor:vndk"
		}
		return "native:vendor:vndksp"
	}
	if !vndk.isVndkSp() {
		return "native:vendor:vndkext"
	}
	return "native:vendor:vndkspext"
}

func (vndk *vndkdep) vndkCheckLinkType(ctx android.ModuleContext, to *Module, tag dependencyTag) {
	if to.linker == nil {
		return
	}
	if !vndk.isVndk() {
		// Non-VNDK modules (those installed to /vendor) can't depend on modules marked with
		// vendor_available: false.
		violation := false
		if lib, ok := to.linker.(*llndkStubDecorator); ok && !Bool(lib.Properties.Vendor_available) {
			violation = true
		} else {
			if _, ok := to.linker.(libraryInterface); ok && to.VendorProperties.Vendor_available != nil && !Bool(to.VendorProperties.Vendor_available) {
				// Vendor_available == nil && !Bool(Vendor_available) should be okay since
				// it means a vendor-only library which is a valid dependency for non-VNDK
				// modules.
				violation = true
			}
		}
		if violation {
			ctx.ModuleErrorf("Vendor module that is not VNDK should not link to %q which is marked as `vendor_available: false`", to.Name())
		}
	}
	if lib, ok := to.linker.(*libraryDecorator); !ok || !lib.shared() {
		// Check only shared libraries.
		// Other (static and LL-NDK) libraries are allowed to link.
		return
	}
	if !to.Properties.UseVndk {
		ctx.ModuleErrorf("(%s) should not link to %q which is not a vendor-available library",
			vndk.typeName(), to.Name())
		return
	}
	if tag == vndkExtDepTag {
		// Ensure `extends: "name"` property refers a vndk module that has vendor_available
		// and has identical vndk properties.
		if to.vndkdep == nil || !to.vndkdep.isVndk() {
			ctx.ModuleErrorf("`extends` refers a non-vndk module %q", to.Name())
			return
		}
		if vndk.isVndkSp() != to.vndkdep.isVndkSp() {
			ctx.ModuleErrorf(
				"`extends` refers a module %q with mismatched support_system_process",
				to.Name())
			return
		}
		if !Bool(to.VendorProperties.Vendor_available) {
			ctx.ModuleErrorf(
				"`extends` refers module %q which does not have `vendor_available: true`",
				to.Name())
			return
		}
	}
	if to.vndkdep == nil {
		return
	}

	// Check the dependencies of VNDK shared libraries.
	if err := vndkIsVndkDepAllowed(vndk, to.vndkdep); err != nil {
		ctx.ModuleErrorf("(%s) should not link to %q (%s): %v",
			vndk.typeName(), to.Name(), to.vndkdep.typeName(), err)
		return
	}
}

func vndkIsVndkDepAllowed(from *vndkdep, to *vndkdep) error {
	// Check the dependencies of VNDK, VNDK-Ext, VNDK-SP, VNDK-SP-Ext and vendor modules.
	if from.isVndkExt() {
		if from.isVndkSp() {
			if to.isVndk() && !to.isVndkSp() {
				return errors.New("VNDK-SP extensions must not depend on VNDK or VNDK extensions")
			}
			return nil
		}
		// VNDK-Ext may depend on VNDK, VNDK-Ext, VNDK-SP, VNDK-SP-Ext, or vendor libs.
		return nil
	}
	if from.isVndk() {
		if to.isVndkExt() {
			return errors.New("VNDK-core and VNDK-SP must not depend on VNDK extensions")
		}
		if from.isVndkSp() {
			if !to.isVndkSp() {
				return errors.New("VNDK-SP must only depend on VNDK-SP")
			}
			return nil
		}
		if !to.isVndk() {
			return errors.New("VNDK-core must only depend on VNDK-core or VNDK-SP")
		}
		return nil
	}
	// Vendor modules may depend on VNDK, VNDK-Ext, VNDK-SP, VNDK-SP-Ext, or vendor libs.
	return nil
}

var (
	vndkCoreLibrariesKey             = android.NewOnceKey("vndkCoreLibrarires")
	vndkSpLibrariesKey               = android.NewOnceKey("vndkSpLibrarires")
	llndkLibrariesKey                = android.NewOnceKey("llndkLibrarires")
	vndkPrivateLibrariesKey          = android.NewOnceKey("vndkPrivateLibrarires")
	vndkUsingCoreVariantLibrariesKey = android.NewOnceKey("vndkUsingCoreVariantLibrarires")
	modulePathsKey                   = android.NewOnceKey("modulePaths")
	vndkSnapshotOutputsKey           = android.NewOnceKey("vndkSnapshotOutputs")
	vndkLibrariesLock                sync.Mutex
)

type vndkSnapshotOutputPaths struct {
	configs         android.Paths
	notices         android.Paths
	vndkCoreLibs    android.Paths
	vndkCoreLibs2nd android.Paths
	vndkSpLibs      android.Paths
	vndkSpLibs2nd   android.Paths
}

func vndkCoreLibraries(config android.Config) *[]string {
	return config.Once(vndkCoreLibrariesKey, func() interface{} {
		return &[]string{}
	}).(*[]string)
}

func vndkSpLibraries(config android.Config) *[]string {
	return config.Once(vndkSpLibrariesKey, func() interface{} {
		return &[]string{}
	}).(*[]string)
}

func llndkLibraries(config android.Config) *[]string {
	return config.Once(llndkLibrariesKey, func() interface{} {
		return &[]string{}
	}).(*[]string)
}

func vndkPrivateLibraries(config android.Config) *[]string {
	return config.Once(vndkPrivateLibrariesKey, func() interface{} {
		return &[]string{}
	}).(*[]string)
}

func vndkUsingCoreVariantLibraries(config android.Config) *[]string {
	return config.Once(vndkUsingCoreVariantLibrariesKey, func() interface{} {
		return &[]string{}
	}).(*[]string)
}

func modulePaths(config android.Config) map[string]string {
	return config.Once(modulePathsKey, func() interface{} {
		return make(map[string]string)
	}).(map[string]string)
}

func vndkSnapshotOutputs(config android.Config) *vndkSnapshotOutputPaths {
	return config.Once(vndkSnapshotOutputsKey, func() interface{} {
		return &vndkSnapshotOutputPaths{}
	}).(*vndkSnapshotOutputPaths)
}

func processLlndkLibrary(mctx android.BottomUpMutatorContext, m *Module) {
	lib := m.linker.(*llndkStubDecorator)
	name := strings.TrimSuffix(m.Name(), llndkLibrarySuffix)

	vndkLibrariesLock.Lock()
	defer vndkLibrariesLock.Unlock()

	llndkLibraries := llndkLibraries(mctx.Config())
	if !inList(name, *llndkLibraries) {
		*llndkLibraries = append(*llndkLibraries, name)
		sort.Strings(*llndkLibraries)
	}
	if !Bool(lib.Properties.Vendor_available) {
		vndkPrivateLibraries := vndkPrivateLibraries(mctx.Config())
		if !inList(name, *vndkPrivateLibraries) {
			*vndkPrivateLibraries = append(*vndkPrivateLibraries, name)
			sort.Strings(*vndkPrivateLibraries)
		}
	}
}

func processVndkLibrary(mctx android.BottomUpMutatorContext, m *Module) {
	name := strings.TrimPrefix(m.Name(), "prebuilt_")

	vndkLibrariesLock.Lock()
	defer vndkLibrariesLock.Unlock()

	modulePaths := modulePaths(mctx.Config())
	if mctx.DeviceConfig().VndkUseCoreVariant() && !inList(name, config.VndkMustUseVendorVariantList) {
		vndkUsingCoreVariantLibraries := vndkUsingCoreVariantLibraries(mctx.Config())
		if !inList(name, *vndkUsingCoreVariantLibraries) {
			*vndkUsingCoreVariantLibraries = append(*vndkUsingCoreVariantLibraries, name)
			sort.Strings(*vndkUsingCoreVariantLibraries)
		}
	}
	if m.vndkdep.isVndkSp() {
		vndkSpLibraries := vndkSpLibraries(mctx.Config())
		if !inList(name, *vndkSpLibraries) {
			*vndkSpLibraries = append(*vndkSpLibraries, name)
			sort.Strings(*vndkSpLibraries)
			modulePaths[name] = mctx.ModuleDir()
		}
	} else {
		vndkCoreLibraries := vndkCoreLibraries(mctx.Config())
		if !inList(name, *vndkCoreLibraries) {
			*vndkCoreLibraries = append(*vndkCoreLibraries, name)
			sort.Strings(*vndkCoreLibraries)
			modulePaths[name] = mctx.ModuleDir()
		}
	}
	if !Bool(m.VendorProperties.Vendor_available) {
		vndkPrivateLibraries := vndkPrivateLibraries(mctx.Config())
		if !inList(name, *vndkPrivateLibraries) {
			*vndkPrivateLibraries = append(*vndkPrivateLibraries, name)
			sort.Strings(*vndkPrivateLibraries)
		}
	}
}

// gather list of vndk-core, vndk-sp, and ll-ndk libs
func VndkMutator(mctx android.BottomUpMutatorContext) {
	m, ok := mctx.Module().(*Module)
	if !ok {
		return
	}

	if !m.Enabled() {
		return
	}

	if m.isVndk() {
		if lib, ok := m.linker.(*vndkPrebuiltLibraryDecorator); ok {
			m.vndkdep.Properties.Vndk.Version = lib.version()
		} else {
			m.vndkdep.Properties.Vndk.Version = mctx.DeviceConfig().PlatformVndkVersion()
		}
	}

	if _, ok := m.linker.(*llndkStubDecorator); ok {
		processLlndkLibrary(mctx, m)
		return
	}

	lib, is_lib := m.linker.(*libraryDecorator)
	prebuilt_lib, is_prebuilt_lib := m.linker.(*prebuiltLibraryLinker)

	if (is_lib && lib.shared()) || (is_prebuilt_lib && prebuilt_lib.shared()) {
		if m.vndkdep.isVndk() && !m.vndkdep.isVndkExt() {
			processVndkLibrary(mctx, m)
			return
		}
	}
}

func init() {
	android.RegisterSingletonType("vndk-snapshot", VndkSnapshotSingleton)
	android.RegisterMakeVarsProvider(pctx, func(ctx android.MakeVarsContext) {
		outputs := vndkSnapshotOutputs(ctx.Config())

		ctx.Strict("SOONG_VNDK_SNAPSHOT_CONFIGS", strings.Join(outputs.configs.Strings(), " "))
		ctx.Strict("SOONG_VNDK_SNAPSHOT_NOTICES", strings.Join(outputs.notices.Strings(), " "))
		ctx.Strict("SOONG_VNDK_SNAPSHOT_CORE_LIBS", strings.Join(outputs.vndkCoreLibs.Strings(), " "))
		ctx.Strict("SOONG_VNDK_SNAPSHOT_SP_LIBS", strings.Join(outputs.vndkSpLibs.Strings(), " "))
		ctx.Strict("SOONG_VNDK_SNAPSHOT_CORE_LIBS_2ND", strings.Join(outputs.vndkCoreLibs2nd.Strings(), " "))
		ctx.Strict("SOONG_VNDK_SNAPSHOT_SP_LIBS_2ND", strings.Join(outputs.vndkSpLibs2nd.Strings(), " "))
	})
}

func VndkSnapshotSingleton() android.Singleton {
	return &vndkSnapshotSingleton{}
}

type vndkSnapshotSingleton struct{}

func installVndkSnapshotLib(ctx android.SingletonContext, name string, module *Module, dir string) android.Path {
	if !module.outputFile.Valid() {
		panic(fmt.Errorf("module %s has no outputFile\n", name))
	}

	out := android.PathForOutput(ctx, dir, name+".so")

	ctx.Build(pctx, android.BuildParams{
		Rule:        android.Cp,
		Input:       module.outputFile.Path(),
		Output:      out,
		Description: "vndk snapshot " + dir + "/" + name + ".so",
		Args: map[string]string{
			"cpFlags": "-f -L",
		},
	})

	return out
}

func (c *vndkSnapshotSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	// BOARD_VNDK_VERSION must be set to 'current' in order to generate a VNDK snapshot.
	if ctx.DeviceConfig().VndkVersion() != "current" {
		return
	}

	if ctx.DeviceConfig().PlatformVndkVersion() == "" {
		return
	}

	if ctx.DeviceConfig().BoardVndkRuntimeDisable() {
		return
	}

	outputs := vndkSnapshotOutputs(ctx.Config())

	snapshotDir := "vndk-snapshot"

	var vndkLibPath, vndkLib2ndPath string

	snapshotVariantPath := filepath.Join(snapshotDir, ctx.DeviceConfig().DeviceArch())
	if ctx.DeviceConfig().BinderBitness() == "32" {
		vndkLibPath = filepath.Join(snapshotVariantPath, "binder32", fmt.Sprintf(
			"arch-%s-%s", ctx.DeviceConfig().DeviceArch(), ctx.DeviceConfig().DeviceArchVariant()))
		vndkLib2ndPath = filepath.Join(snapshotVariantPath, "binder32", fmt.Sprintf(
			"arch-%s-%s", ctx.DeviceConfig().DeviceSecondaryArch(), ctx.DeviceConfig().DeviceSecondaryArchVariant()))
	} else {
		vndkLibPath = filepath.Join(snapshotVariantPath, fmt.Sprintf(
			"arch-%s-%s", ctx.DeviceConfig().DeviceArch(), ctx.DeviceConfig().DeviceArchVariant()))
		vndkLib2ndPath = filepath.Join(snapshotVariantPath, fmt.Sprintf(
			"arch-%s-%s", ctx.DeviceConfig().DeviceSecondaryArch(), ctx.DeviceConfig().DeviceSecondaryArchVariant()))
	}

	vndkCoreLibPath := filepath.Join(vndkLibPath, "shared", "vndk-core")
	vndkSpLibPath := filepath.Join(vndkLibPath, "shared", "vndk-sp")
	vndkCoreLib2ndPath := filepath.Join(vndkLib2ndPath, "shared", "vndk-core")
	vndkSpLib2ndPath := filepath.Join(vndkLib2ndPath, "shared", "vndk-sp")
	noticePath := filepath.Join(snapshotVariantPath, "NOTICE_FILES")
	noticeBuilt := make(map[string]bool)

	tryBuildNotice := func(m *Module) {
		name := ctx.ModuleName(m)

		if _, ok := noticeBuilt[name]; ok {
			return
		}

		noticeBuilt[name] = true

		if m.NoticeFile().Valid() {
			out := android.PathForOutput(ctx, noticePath, name+".so.txt")
			ctx.Build(pctx, android.BuildParams{
				Rule:        android.Cp,
				Input:       m.NoticeFile().Path(),
				Output:      out,
				Description: "vndk snapshot notice " + name + ".so.txt",
				Args: map[string]string{
					"cpFlags": "-f -L",
				},
			})
			outputs.notices = append(outputs.notices, out)
		}
	}

	vndkCoreLibraries := vndkCoreLibraries(ctx.Config())
	vndkSpLibraries := vndkSpLibraries(ctx.Config())
	vndkPrivateLibraries := vndkPrivateLibraries(ctx.Config())

	ctx.VisitAllModules(func(module android.Module) {
		m, ok := module.(*Module)
		if !ok || !m.Enabled() || !m.useVndk() || !m.installable() {
			return
		}

		if m.Target().NativeBridge == android.NativeBridgeEnabled {
			return
		}

		lib, is_lib := m.linker.(*libraryDecorator)
		prebuilt_lib, is_prebuilt_lib := m.linker.(*prebuiltLibraryLinker)

		if !(is_lib && lib.shared()) && !(is_prebuilt_lib && prebuilt_lib.shared()) {
			return
		}

		is_2nd := m.Target().Arch.ArchType != ctx.Config().DevicePrimaryArchType()

		name := ctx.ModuleName(module)

		if inList(name, *vndkCoreLibraries) {
			if is_2nd {
				out := installVndkSnapshotLib(ctx, name, m, vndkCoreLib2ndPath)
				outputs.vndkCoreLibs2nd = append(outputs.vndkCoreLibs2nd, out)
			} else {
				out := installVndkSnapshotLib(ctx, name, m, vndkCoreLibPath)
				outputs.vndkCoreLibs = append(outputs.vndkCoreLibs, out)
			}
			tryBuildNotice(m)
		} else if inList(name, *vndkSpLibraries) {
			if is_2nd {
				out := installVndkSnapshotLib(ctx, name, m, vndkSpLib2ndPath)
				outputs.vndkSpLibs2nd = append(outputs.vndkSpLibs2nd, out)
			} else {
				out := installVndkSnapshotLib(ctx, name, m, vndkSpLibPath)
				outputs.vndkSpLibs = append(outputs.vndkSpLibs, out)
			}
			tryBuildNotice(m)
		}
	})

	configsPath := filepath.Join(snapshotVariantPath, "configs")
	vndkCoreTxt := android.PathForOutput(ctx, configsPath, "vndkcore.libraries.txt")
	vndkPrivateTxt := android.PathForOutput(ctx, configsPath, "vndkprivate.libraries.txt")
	modulePathTxt := android.PathForOutput(ctx, configsPath, "module_paths.txt")

	ctx.Build(pctx, android.BuildParams{
		Rule:        android.WriteFile,
		Output:      vndkCoreTxt,
		Description: "vndk snapshot vndkcore.libraries.txt",
		Args: map[string]string{
			"content": android.JoinWithSuffix(*vndkCoreLibraries, ".so", "\\n"),
		},
	})
	outputs.configs = append(outputs.configs, vndkCoreTxt)

	ctx.Build(pctx, android.BuildParams{
		Rule:        android.WriteFile,
		Output:      vndkPrivateTxt,
		Description: "vndk snapshot vndkprivate.libraries.txt",
		Args: map[string]string{
			"content": android.JoinWithSuffix(*vndkPrivateLibraries, ".so", "\\n"),
		},
	})
	outputs.configs = append(outputs.configs, vndkPrivateTxt)

	var modulePathTxtBuilder strings.Builder

	modulePaths := modulePaths(ctx.Config())
	var libs []string
	for lib := range modulePaths {
		libs = append(libs, lib)
	}
	sort.Strings(libs)

	first := true
	for _, lib := range libs {
		if first {
			first = false
		} else {
			modulePathTxtBuilder.WriteString("\\n")
		}
		modulePathTxtBuilder.WriteString(lib)
		modulePathTxtBuilder.WriteString(".so ")
		modulePathTxtBuilder.WriteString(modulePaths[lib])
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        android.WriteFile,
		Output:      modulePathTxt,
		Description: "vndk snapshot module_paths.txt",
		Args: map[string]string{
			"content": modulePathTxtBuilder.String(),
		},
	})
	outputs.configs = append(outputs.configs, modulePathTxt)
}
