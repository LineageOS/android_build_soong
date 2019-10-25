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
	"encoding/json"
	"errors"
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
		// `vendor_available` must be explicitly set to either true or
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
	if !to.useVndk() {
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

	headerExts = []string{".h", ".hh", ".hpp", ".hxx", ".h++", ".inl", ".inc", ".ipp", ".h.generic"}
)

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

func vndkSnapshotOutputs(config android.Config) *android.RuleBuilderInstalls {
	return config.Once(vndkSnapshotOutputsKey, func() interface{} {
		return &android.RuleBuilderInstalls{}
	}).(*android.RuleBuilderInstalls)
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

func IsForVndkApex(mctx android.BottomUpMutatorContext, m *Module) bool {
	if !m.Enabled() {
		return false
	}

	if m.Target().NativeBridge == android.NativeBridgeEnabled {
		return false
	}

	// prebuilt vndk modules should match with device
	// TODO(b/142675459): Use enabled: to select target device in vndk_prebuilt_shared
	// When b/142675459 is landed, remove following check
	if p, ok := m.linker.(*vndkPrebuiltLibraryDecorator); ok && !p.matchesWithDevice(mctx.DeviceConfig()) {
		return false
	}

	if lib, ok := m.linker.(libraryInterface); ok {
		useCoreVariant := m.vndkVersion() == mctx.DeviceConfig().PlatformVndkVersion() &&
			mctx.DeviceConfig().VndkUseCoreVariant() &&
			!inList(m.BaseModuleName(), config.VndkMustUseVendorVariantList)
		return lib.shared() && m.useVndk() && m.isVndk() && !m.isVndkExt() && !useCoreVariant
	}
	return false
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
	if m.Target().NativeBridge == android.NativeBridgeEnabled {
		// Skip native_bridge modules
		return
	}

	if _, ok := m.linker.(*llndkStubDecorator); ok {
		processLlndkLibrary(mctx, m)
		return
	}

	lib, is_lib := m.linker.(*libraryDecorator)
	prebuilt_lib, is_prebuilt_lib := m.linker.(*prebuiltLibraryLinker)

	if (is_lib && lib.buildShared()) || (is_prebuilt_lib && prebuilt_lib.buildShared()) {
		if m.vndkdep != nil && m.vndkdep.isVndk() && !m.vndkdep.isVndkExt() {
			processVndkLibrary(mctx, m)
			return
		}
	}
}

func init() {
	android.RegisterSingletonType("vndk-snapshot", VndkSnapshotSingleton)
	android.RegisterMakeVarsProvider(pctx, func(ctx android.MakeVarsContext) {
		outputs := vndkSnapshotOutputs(ctx.Config())
		ctx.Strict("SOONG_VNDK_SNAPSHOT_FILES", outputs.String())
	})
}

func VndkSnapshotSingleton() android.Singleton {
	return &vndkSnapshotSingleton{}
}

type vndkSnapshotSingleton struct{}

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

	vndkLibDir := make(map[android.ArchType]string)

	snapshotVariantDir := ctx.DeviceConfig().DeviceArch()
	for _, target := range ctx.Config().Targets[android.Android] {
		dir := snapshotVariantDir
		if ctx.DeviceConfig().BinderBitness() == "32" {
			dir = filepath.Join(dir, "binder32")
		}
		arch := "arch-" + target.Arch.ArchType.String()
		if target.Arch.ArchVariant != "" {
			arch += "-" + target.Arch.ArchVariant
		}
		dir = filepath.Join(dir, arch)
		vndkLibDir[target.Arch.ArchType] = dir
	}
	configsDir := filepath.Join(snapshotVariantDir, "configs")
	noticeDir := filepath.Join(snapshotVariantDir, "NOTICE_FILES")
	includeDir := filepath.Join(snapshotVariantDir, "include")
	noticeBuilt := make(map[string]bool)

	installSnapshotFileFromPath := func(path android.Path, out string) {
		ctx.Build(pctx, android.BuildParams{
			Rule:        android.Cp,
			Input:       path,
			Output:      android.PathForOutput(ctx, snapshotDir, out),
			Description: "vndk snapshot " + out,
			Args: map[string]string{
				"cpFlags": "-f -L",
			},
		})
		*outputs = append(*outputs, android.RuleBuilderInstall{
			From: android.PathForOutput(ctx, snapshotDir, out),
			To:   out,
		})
	}
	installSnapshotFileFromContent := func(content, out string) {
		ctx.Build(pctx, android.BuildParams{
			Rule:        android.WriteFile,
			Output:      android.PathForOutput(ctx, snapshotDir, out),
			Description: "vndk snapshot " + out,
			Args: map[string]string{
				"content": content,
			},
		})
		*outputs = append(*outputs, android.RuleBuilderInstall{
			From: android.PathForOutput(ctx, snapshotDir, out),
			To:   out,
		})
	}

	tryBuildNotice := func(m *Module) {
		name := ctx.ModuleName(m) + ".so.txt"

		if _, ok := noticeBuilt[name]; ok {
			return
		}

		noticeBuilt[name] = true

		if m.NoticeFile().Valid() {
			installSnapshotFileFromPath(m.NoticeFile().Path(), filepath.Join(noticeDir, name))
		}
	}

	vndkCoreLibraries := vndkCoreLibraries(ctx.Config())
	vndkSpLibraries := vndkSpLibraries(ctx.Config())
	vndkPrivateLibraries := vndkPrivateLibraries(ctx.Config())

	var generatedHeaders android.Paths
	includeDirs := make(map[string]bool)

	type vndkSnapshotLibraryInterface interface {
		exportedFlagsProducer
		libraryInterface
	}

	var _ vndkSnapshotLibraryInterface = (*prebuiltLibraryLinker)(nil)
	var _ vndkSnapshotLibraryInterface = (*libraryDecorator)(nil)

	installVndkSnapshotLib := func(m *Module, l vndkSnapshotLibraryInterface, dir string) bool {
		name := ctx.ModuleName(m)
		libOut := filepath.Join(dir, name+".so")

		installSnapshotFileFromPath(m.outputFile.Path(), libOut)
		tryBuildNotice(m)

		if ctx.Config().VndkSnapshotBuildArtifacts() {
			prop := struct {
				ExportedDirs        []string `json:",omitempty"`
				ExportedSystemDirs  []string `json:",omitempty"`
				ExportedFlags       []string `json:",omitempty"`
				RelativeInstallPath string   `json:",omitempty"`
			}{}
			prop.ExportedFlags = l.exportedFlags()
			prop.ExportedDirs = l.exportedDirs()
			prop.ExportedSystemDirs = l.exportedSystemDirs()
			prop.RelativeInstallPath = m.RelativeInstallPath()

			propOut := libOut + ".json"

			j, err := json.Marshal(prop)
			if err != nil {
				ctx.Errorf("json marshal to %q failed: %#v", propOut, err)
				return false
			}

			installSnapshotFileFromContent(string(j), propOut)
		}
		return true
	}

	isVndkSnapshotLibrary := func(m *Module) (i vndkSnapshotLibraryInterface, libDir string, isVndkSnapshotLib bool) {
		if m.Target().NativeBridge == android.NativeBridgeEnabled {
			return nil, "", false
		}
		if !m.useVndk() || !m.IsForPlatform() || !m.installable() {
			return nil, "", false
		}
		l, ok := m.linker.(vndkSnapshotLibraryInterface)
		if !ok || !l.shared() {
			return nil, "", false
		}
		name := ctx.ModuleName(m)
		if inList(name, *vndkCoreLibraries) {
			return l, filepath.Join("shared", "vndk-core"), true
		} else if inList(name, *vndkSpLibraries) {
			return l, filepath.Join("shared", "vndk-sp"), true
		} else {
			return nil, "", false
		}
	}

	ctx.VisitAllModules(func(module android.Module) {
		m, ok := module.(*Module)
		if !ok || !m.Enabled() {
			return
		}

		baseDir, ok := vndkLibDir[m.Target().Arch.ArchType]
		if !ok {
			return
		}

		l, libDir, ok := isVndkSnapshotLibrary(m)
		if !ok {
			return
		}

		if !installVndkSnapshotLib(m, l, filepath.Join(baseDir, libDir)) {
			return
		}

		generatedHeaders = append(generatedHeaders, l.exportedDeps()...)
		for _, dir := range append(l.exportedDirs(), l.exportedSystemDirs()...) {
			includeDirs[dir] = true
		}
	})

	if ctx.Config().VndkSnapshotBuildArtifacts() {
		headers := make(map[string]bool)

		for _, dir := range android.SortedStringKeys(includeDirs) {
			// workaround to determine if dir is under output directory
			if strings.HasPrefix(dir, android.PathForOutput(ctx).String()) {
				continue
			}
			exts := headerExts
			// Glob all files under this special directory, because of C++ headers.
			if strings.HasPrefix(dir, "external/libcxx/include") {
				exts = []string{""}
			}
			for _, ext := range exts {
				glob, err := ctx.GlobWithDeps(dir+"/**/*"+ext, nil)
				if err != nil {
					ctx.Errorf("%#v\n", err)
					return
				}
				for _, header := range glob {
					if strings.HasSuffix(header, "/") {
						continue
					}
					headers[header] = true
				}
			}
		}

		for _, header := range android.SortedStringKeys(headers) {
			installSnapshotFileFromPath(android.PathForSource(ctx, header),
				filepath.Join(includeDir, header))
		}

		isHeader := func(path string) bool {
			for _, ext := range headerExts {
				if strings.HasSuffix(path, ext) {
					return true
				}
			}
			return false
		}

		for _, path := range android.PathsToDirectorySortedPaths(android.FirstUniquePaths(generatedHeaders)) {
			header := path.String()

			if !isHeader(header) {
				continue
			}

			installSnapshotFileFromPath(path, filepath.Join(includeDir, header))
		}
	}

	installSnapshotFileFromContent(android.JoinWithSuffix(*vndkCoreLibraries, ".so", "\\n"),
		filepath.Join(configsDir, "vndkcore.libraries.txt"))
	installSnapshotFileFromContent(android.JoinWithSuffix(*vndkPrivateLibraries, ".so", "\\n"),
		filepath.Join(configsDir, "vndkprivate.libraries.txt"))

	var modulePathTxtBuilder strings.Builder

	modulePaths := modulePaths(ctx.Config())

	first := true
	for _, lib := range android.SortedStringKeys(modulePaths) {
		if first {
			first = false
		} else {
			modulePathTxtBuilder.WriteString("\\n")
		}
		modulePathTxtBuilder.WriteString(lib)
		modulePathTxtBuilder.WriteString(".so ")
		modulePathTxtBuilder.WriteString(modulePaths[lib])
	}

	installSnapshotFileFromContent(modulePathTxtBuilder.String(),
		filepath.Join(configsDir, "module_paths.txt"))
}
