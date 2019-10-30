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
	"fmt"
	"path/filepath"
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

func (vndk *vndkdep) vndkCheckLinkType(ctx android.ModuleContext, to *Module, tag DependencyTag) {
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
	if !to.UseVndk() {
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
	vndkCoreLibrariesKey                = android.NewOnceKey("vndkCoreLibrarires")
	vndkSpLibrariesKey                  = android.NewOnceKey("vndkSpLibrarires")
	llndkLibrariesKey                   = android.NewOnceKey("llndkLibrarires")
	vndkPrivateLibrariesKey             = android.NewOnceKey("vndkPrivateLibrarires")
	vndkUsingCoreVariantLibrariesKey    = android.NewOnceKey("vndkUsingCoreVariantLibrarires")
	modulePathsKey                      = android.NewOnceKey("modulePaths")
	vndkSnapshotOutputsKey              = android.NewOnceKey("vndkSnapshotOutputs")
	vndkMustUseVendorVariantListKey     = android.NewOnceKey("vndkMustUseVendorVariantListKey")
	testVndkMustUseVendorVariantListKey = android.NewOnceKey("testVndkMustUseVendorVariantListKey")
	vndkLibrariesLock                   sync.Mutex

	headerExts = []string{".h", ".hh", ".hpp", ".hxx", ".h++", ".inl", ".inc", ".ipp", ".h.generic"}
)

func vndkCoreLibraries(config android.Config) map[string]string {
	return config.Once(vndkCoreLibrariesKey, func() interface{} {
		return make(map[string]string)
	}).(map[string]string)
}

func vndkSpLibraries(config android.Config) map[string]string {
	return config.Once(vndkSpLibrariesKey, func() interface{} {
		return make(map[string]string)
	}).(map[string]string)
}

func isLlndkLibrary(baseModuleName string, config android.Config) bool {
	_, ok := llndkLibraries(config)[baseModuleName]
	return ok
}

func llndkLibraries(config android.Config) map[string]string {
	return config.Once(llndkLibrariesKey, func() interface{} {
		return make(map[string]string)
	}).(map[string]string)
}

func isVndkPrivateLibrary(baseModuleName string, config android.Config) bool {
	_, ok := vndkPrivateLibraries(config)[baseModuleName]
	return ok
}

func vndkPrivateLibraries(config android.Config) map[string]string {
	return config.Once(vndkPrivateLibrariesKey, func() interface{} {
		return make(map[string]string)
	}).(map[string]string)
}

func vndkUsingCoreVariantLibraries(config android.Config) map[string]string {
	return config.Once(vndkUsingCoreVariantLibrariesKey, func() interface{} {
		return make(map[string]string)
	}).(map[string]string)
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

func vndkMustUseVendorVariantList(cfg android.Config) []string {
	return cfg.Once(vndkMustUseVendorVariantListKey, func() interface{} {
		override := cfg.Once(testVndkMustUseVendorVariantListKey, func() interface{} {
			return []string(nil)
		}).([]string)
		if override != nil {
			return override
		}
		return config.VndkMustUseVendorVariantList
	}).([]string)
}

// test may call this to override global configuration(config.VndkMustUseVendorVariantList)
// when it is called, it must be before the first call to vndkMustUseVendorVariantList()
func setVndkMustUseVendorVariantListForTest(config android.Config, mustUseVendorVariantList []string) {
	config.Once(testVndkMustUseVendorVariantListKey, func() interface{} {
		return mustUseVendorVariantList
	})
}

func processLlndkLibrary(mctx android.BottomUpMutatorContext, m *Module) {
	lib := m.linker.(*llndkStubDecorator)
	name := m.BaseModuleName()
	filename := m.BaseModuleName() + ".so"

	vndkLibrariesLock.Lock()
	defer vndkLibrariesLock.Unlock()

	llndkLibraries(mctx.Config())[name] = filename
	if !Bool(lib.Properties.Vendor_available) {
		vndkPrivateLibraries(mctx.Config())[name] = filename
	}
}

func processVndkLibrary(mctx android.BottomUpMutatorContext, m *Module) {
	name := m.BaseModuleName()
	filename, err := getVndkFileName(m)
	if err != nil {
		panic(err)
	}

	vndkLibrariesLock.Lock()
	defer vndkLibrariesLock.Unlock()

	modulePaths(mctx.Config())[name] = mctx.ModuleDir()

	if inList(name, vndkMustUseVendorVariantList(mctx.Config())) {
		m.Properties.MustUseVendorVariant = true
	}
	if mctx.DeviceConfig().VndkUseCoreVariant() && !m.Properties.MustUseVendorVariant {
		vndkUsingCoreVariantLibraries(mctx.Config())[name] = filename
	}

	if m.vndkdep.isVndkSp() {
		vndkSpLibraries(mctx.Config())[name] = filename
	} else {
		vndkCoreLibraries(mctx.Config())[name] = filename
	}
	if !Bool(m.VendorProperties.Vendor_available) {
		vndkPrivateLibraries(mctx.Config())[name] = filename
	}
}

func IsForVndkApex(mctx android.BottomUpMutatorContext, m *Module) bool {
	if !m.Enabled() {
		return false
	}

	if !mctx.Device() {
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
			mctx.DeviceConfig().VndkUseCoreVariant() && !m.MustUseVendorVariant()
		return lib.shared() && m.UseVndk() && m.IsVndk() && !m.isVndkExt() && !useCoreVariant
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

type vndkSnapshotSingleton struct {
	installedLlndkLibraries      []string
	llnkdLibrariesFile           android.Path
	vndkSpLibrariesFile          android.Path
	vndkCoreLibrariesFile        android.Path
	vndkPrivateLibrariesFile     android.Path
	vndkCoreVariantLibrariesFile android.Path
	vndkLibrariesFile            android.Path
}

func (c *vndkSnapshotSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	// build these files even if PlatformVndkVersion or BoardVndkVersion is not set
	c.buildVndkLibrariesTxtFiles(ctx)

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

	vndkCoreLibraries := android.SortedStringKeys(vndkCoreLibraries(ctx.Config()))
	vndkSpLibraries := android.SortedStringKeys(vndkSpLibraries(ctx.Config()))
	vndkPrivateLibraries := android.SortedStringKeys(vndkPrivateLibraries(ctx.Config()))

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
			prop.ExportedDirs = l.exportedDirs().Strings()
			prop.ExportedSystemDirs = l.exportedSystemDirs().Strings()
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
		if !m.UseVndk() || !m.IsForPlatform() || !m.installable() {
			return nil, "", false
		}
		l, ok := m.linker.(vndkSnapshotLibraryInterface)
		if !ok || !l.shared() {
			return nil, "", false
		}
		name := ctx.ModuleName(m)
		if inList(name, vndkCoreLibraries) {
			return l, filepath.Join("shared", "vndk-core"), true
		} else if inList(name, vndkSpLibraries) {
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
			includeDirs[dir.String()] = true
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

	installSnapshotFileFromContent(android.JoinWithSuffix(vndkCoreLibraries, ".so", "\\n"),
		filepath.Join(configsDir, "vndkcore.libraries.txt"))
	installSnapshotFileFromContent(android.JoinWithSuffix(vndkPrivateLibraries, ".so", "\\n"),
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

func getVndkFileName(m *Module) (string, error) {
	if library, ok := m.linker.(*libraryDecorator); ok {
		return library.getLibNameHelper(m.BaseModuleName(), true) + ".so", nil
	}
	if prebuilt, ok := m.linker.(*prebuiltLibraryLinker); ok {
		return prebuilt.libraryDecorator.getLibNameHelper(m.BaseModuleName(), true) + ".so", nil
	}
	return "", fmt.Errorf("VNDK library should have libraryDecorator or prebuiltLibraryLinker as linker: %T", m.linker)
}

func (c *vndkSnapshotSingleton) buildVndkLibrariesTxtFiles(ctx android.SingletonContext) {
	// Make uses LLNDK_LIBRARIES to determine which libraries to install.
	// HWASAN is only part of the LL-NDK in builds in which libc depends on HWASAN.
	// Therefore, by removing the library here, we cause it to only be installed if libc
	// depends on it.
	installedLlndkLibraries := make(map[string]string)
	for lib, filename := range llndkLibraries(ctx.Config()) {
		if strings.HasPrefix(lib, "libclang_rt.hwasan-") {
			continue
		}
		installedLlndkLibraries[lib] = filename
	}

	installListFile := func(list []string, fileName string) android.Path {
		out := android.PathForOutput(ctx, "vndk", fileName)
		ctx.Build(pctx, android.BuildParams{
			Rule:        android.WriteFile,
			Output:      out,
			Description: "Writing " + out.String(),
			Args: map[string]string{
				"content": strings.Join(list, "\\n"),
			},
		})
		return out
	}

	c.installedLlndkLibraries = android.SortedStringKeys(installedLlndkLibraries)

	llndk := android.SortedStringMapValues(installedLlndkLibraries)
	vndkcore := android.SortedStringMapValues(vndkCoreLibraries(ctx.Config()))
	vndksp := android.SortedStringMapValues(vndkSpLibraries(ctx.Config()))
	vndkprivate := android.SortedStringMapValues(vndkPrivateLibraries(ctx.Config()))
	vndkcorevariant := android.SortedStringMapValues(vndkUsingCoreVariantLibraries(ctx.Config()))

	c.llnkdLibrariesFile = installListFile(llndk, "llndk.libraries.txt")
	c.vndkCoreLibrariesFile = installListFile(vndkcore, "vndkcore.libraries.txt")
	c.vndkSpLibrariesFile = installListFile(vndksp, "vndksp.libraries.txt")
	c.vndkPrivateLibrariesFile = installListFile(vndkprivate, "vndkprivate.libraries.txt")
	c.vndkCoreVariantLibrariesFile = installListFile(vndkcorevariant, "vndkcorevariant.libraries.txt")

	// merged & tagged & filtered-out(libclang_rt)
	// Since each target have different set of libclang_rt.* files,
	// keep the common set of files in vndk.libraries.txt
	var merged []string
	filterOutLibClangRt := func(libList []string) (filtered []string) {
		for _, lib := range libList {
			if !strings.HasPrefix(lib, "libclang_rt.") {
				filtered = append(filtered, lib)
			}
		}
		return
	}
	merged = append(merged, addPrefix(filterOutLibClangRt(llndk), "LLNDK: ")...)
	merged = append(merged, addPrefix(vndksp, "VNDK-SP: ")...)
	merged = append(merged, addPrefix(filterOutLibClangRt(vndkcore), "VNDK-core: ")...)
	merged = append(merged, addPrefix(vndkprivate, "VNDK-private: ")...)
	c.vndkLibrariesFile = installListFile(merged, "vndk.libraries.txt")
}

func (c *vndkSnapshotSingleton) MakeVars(ctx android.MakeVarsContext) {
	// Make uses LLNDK_MOVED_TO_APEX_LIBRARIES to avoid installing libraries on /system if
	// they been moved to an apex.
	movedToApexLlndkLibraries := []string{}
	for _, lib := range c.installedLlndkLibraries {
		// Skip bionic libs, they are handled in different manner
		if android.DirectlyInAnyApex(&notOnHostContext{}, lib) && !isBionic(lib) {
			movedToApexLlndkLibraries = append(movedToApexLlndkLibraries, lib)
		}
	}
	ctx.Strict("LLNDK_MOVED_TO_APEX_LIBRARIES", strings.Join(movedToApexLlndkLibraries, " "))
	ctx.Strict("LLNDK_LIBRARIES", strings.Join(c.installedLlndkLibraries, " "))
	ctx.Strict("VNDK_CORE_LIBRARIES", strings.Join(android.SortedStringKeys(vndkCoreLibraries(ctx.Config())), " "))
	ctx.Strict("VNDK_SAMEPROCESS_LIBRARIES", strings.Join(android.SortedStringKeys(vndkSpLibraries(ctx.Config())), " "))
	ctx.Strict("VNDK_PRIVATE_LIBRARIES", strings.Join(android.SortedStringKeys(vndkPrivateLibraries(ctx.Config())), " "))
	ctx.Strict("VNDK_USING_CORE_VARIANT_LIBRARIES", strings.Join(android.SortedStringKeys(vndkUsingCoreVariantLibraries(ctx.Config())), " "))

	ctx.Strict("LLNDK_LIBRARIES_FILE", c.llnkdLibrariesFile.String())
	ctx.Strict("VNDKCORE_LIBRARIES_FILE", c.vndkCoreLibrariesFile.String())
	ctx.Strict("VNDKSP_LIBRARIES_FILE", c.vndkSpLibrariesFile.String())
	ctx.Strict("VNDKPRIVATE_LIBRARIES_FILE", c.vndkPrivateLibrariesFile.String())
	ctx.Strict("VNDKCOREVARIANT_LIBRARIES_FILE", c.vndkCoreVariantLibrariesFile.String())

	ctx.Strict("VNDK_LIBRARIES_FILE", c.vndkLibrariesFile.String())
}
