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
	"sort"
	"strings"
	"sync"

	"android/soong/android"
	"android/soong/cc/config"
	"android/soong/etc"

	"github.com/google/blueprint"
)

const (
	llndkLibrariesTxt                = "llndk.libraries.txt"
	vndkCoreLibrariesTxt             = "vndkcore.libraries.txt"
	vndkSpLibrariesTxt               = "vndksp.libraries.txt"
	vndkPrivateLibrariesTxt          = "vndkprivate.libraries.txt"
	vndkProductLibrariesTxt          = "vndkproduct.libraries.txt"
	vndkUsingCoreVariantLibrariesTxt = "vndkcorevariant.libraries.txt"
)

func VndkLibrariesTxtModules(vndkVersion string) []string {
	if vndkVersion == "current" {
		return []string{
			llndkLibrariesTxt,
			vndkCoreLibrariesTxt,
			vndkSpLibrariesTxt,
			vndkPrivateLibrariesTxt,
			vndkProductLibrariesTxt,
		}
	}
	// Snapshot vndks have their own *.libraries.VER.txt files.
	// Note that snapshots don't have "vndkcorevariant.libraries.VER.txt"
	return []string{
		insertVndkVersion(llndkLibrariesTxt, vndkVersion),
		insertVndkVersion(vndkCoreLibrariesTxt, vndkVersion),
		insertVndkVersion(vndkSpLibrariesTxt, vndkVersion),
		insertVndkVersion(vndkPrivateLibrariesTxt, vndkVersion),
		insertVndkVersion(vndkProductLibrariesTxt, vndkVersion),
	}
}

type VndkProperties struct {
	Vndk struct {
		// declared as a VNDK or VNDK-SP module. The vendor variant
		// will be installed in /system instead of /vendor partition.
		//
		// `vendor_available` and `product_available` must be explicitly
		// set to either true or false together with `vndk: {enabled: true}`.
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

		// declared as a VNDK-private module.
		// This module still creates the vendor and product variants refering
		// to the `vendor_available: true` and `product_available: true`
		// properties. However, it is only available to the other VNDK modules
		// but not to the non-VNDK vendor or product modules.
		Private *bool

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

// VNDK link type check from a module with UseVndk() == true.
func (vndk *vndkdep) vndkCheckLinkType(ctx android.BaseModuleContext, to *Module, tag blueprint.DependencyTag) {
	if to.linker == nil {
		return
	}
	if !vndk.isVndk() {
		// Non-VNDK modules those installed to /vendor, /system/vendor,
		// /product or /system/product cannot depend on VNDK-private modules
		// that include VNDK-core-private, VNDK-SP-private and LLNDK-private.
		if to.IsVndkPrivate() {
			ctx.ModuleErrorf("non-VNDK module should not link to %q which has `private: true`", to.Name())
		}
	}
	if lib, ok := to.linker.(*libraryDecorator); !ok || !lib.shared() {
		// Check only shared libraries.
		// Other (static) libraries are allowed to link.
		return
	}

	if to.IsLlndk() {
		// LL-NDK libraries are allowed to link
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
		if to.IsVndkPrivate() {
			ctx.ModuleErrorf(
				"`extends` refers module %q which has `private: true`",
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
	vndkCoreLibrariesKey             = android.NewOnceKey("vndkCoreLibraries")
	vndkSpLibrariesKey               = android.NewOnceKey("vndkSpLibraries")
	llndkLibrariesKey                = android.NewOnceKey("llndkLibraries")
	vndkPrivateLibrariesKey          = android.NewOnceKey("vndkPrivateLibraries")
	vndkProductLibrariesKey          = android.NewOnceKey("vndkProductLibraries")
	vndkUsingCoreVariantLibrariesKey = android.NewOnceKey("vndkUsingCoreVariantLibraries")
	vndkMustUseVendorVariantListKey  = android.NewOnceKey("vndkMustUseVendorVariantListKey")
	vndkLibrariesLock                sync.Mutex
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

func llndkLibraries(config android.Config) map[string]string {
	return config.Once(llndkLibrariesKey, func() interface{} {
		return make(map[string]string)
	}).(map[string]string)
}

func vndkPrivateLibraries(config android.Config) map[string]string {
	return config.Once(vndkPrivateLibrariesKey, func() interface{} {
		return make(map[string]string)
	}).(map[string]string)
}

func vndkProductLibraries(config android.Config) map[string]string {
	return config.Once(vndkProductLibrariesKey, func() interface{} {
		return make(map[string]string)
	}).(map[string]string)
}

func vndkUsingCoreVariantLibraries(config android.Config) map[string]string {
	return config.Once(vndkUsingCoreVariantLibrariesKey, func() interface{} {
		return make(map[string]string)
	}).(map[string]string)
}

func vndkMustUseVendorVariantList(cfg android.Config) []string {
	return cfg.Once(vndkMustUseVendorVariantListKey, func() interface{} {
		return config.VndkMustUseVendorVariantList
	}).([]string)
}

// test may call this to override global configuration(config.VndkMustUseVendorVariantList)
// when it is called, it must be before the first call to vndkMustUseVendorVariantList()
func setVndkMustUseVendorVariantListForTest(config android.Config, mustUseVendorVariantList []string) {
	config.Once(vndkMustUseVendorVariantListKey, func() interface{} {
		return mustUseVendorVariantList
	})
}

func processLlndkLibrary(mctx android.BottomUpMutatorContext, m *Module) {
	lib := m.linker.(*llndkStubDecorator)
	name := m.ImplementationModuleName(mctx)
	filename := name + ".so"

	vndkLibrariesLock.Lock()
	defer vndkLibrariesLock.Unlock()

	llndkLibraries(mctx.Config())[name] = filename
	m.VendorProperties.IsLLNDK = true
	if Bool(lib.Properties.Private) {
		vndkPrivateLibraries(mctx.Config())[name] = filename
		m.VendorProperties.IsLLNDKPrivate = true
	}
}

func processVndkLibrary(mctx android.BottomUpMutatorContext, m *Module) {
	if m.InProduct() {
		// We may skip the steps for the product variants because they
		// are already covered by the vendor variants.
		return
	}

	name := m.BaseModuleName()
	filename, err := getVndkFileName(m)
	if err != nil {
		panic(err)
	}

	if lib := m.library; lib != nil && lib.hasStubsVariants() && name != "libz" {
		// b/155456180 libz is the ONLY exception here. We don't want to make
		// libz an LLNDK library because we in general can't guarantee that
		// libz will behave consistently especially about the compression.
		// i.e. the compressed output might be different across releases.
		// As the library is an external one, it's risky to keep the compatibility
		// promise if it becomes an LLNDK.
		mctx.PropertyErrorf("vndk.enabled", "This library provides stubs. Shouldn't be VNDK. Consider making it as LLNDK")
	}

	vndkLibrariesLock.Lock()
	defer vndkLibrariesLock.Unlock()

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
	if m.IsVndkPrivate() {
		vndkPrivateLibraries(mctx.Config())[name] = filename
	}
	if Bool(m.VendorProperties.Product_available) {
		vndkProductLibraries(mctx.Config())[name] = filename
	}
}

// Check for modules that mustn't be VNDK
func shouldSkipVndkMutator(m *Module) bool {
	if !m.Enabled() {
		return true
	}
	if !m.Device() {
		// Skip non-device modules
		return true
	}
	if m.Target().NativeBridge == android.NativeBridgeEnabled {
		// Skip native_bridge modules
		return true
	}
	return false
}

func IsForVndkApex(mctx android.BottomUpMutatorContext, m *Module) bool {
	if shouldSkipVndkMutator(m) {
		return false
	}

	// prebuilt vndk modules should match with device
	// TODO(b/142675459): Use enabled: to select target device in vndk_prebuilt_shared
	// When b/142675459 is landed, remove following check
	if p, ok := m.linker.(*vndkPrebuiltLibraryDecorator); ok && !p.matchesWithDevice(mctx.DeviceConfig()) {
		return false
	}

	if lib, ok := m.linker.(libraryInterface); ok {
		// VNDK APEX for VNDK-Lite devices will have VNDK-SP libraries from core variants
		if mctx.DeviceConfig().VndkVersion() == "" {
			// b/73296261: filter out libz.so because it is considered as LLNDK for VNDK-lite devices
			if mctx.ModuleName() == "libz" {
				return false
			}
			return m.ImageVariation().Variation == android.CoreVariation && lib.shared() && m.isVndkSp()
		}

		useCoreVariant := m.VndkVersion() == mctx.DeviceConfig().PlatformVndkVersion() &&
			mctx.DeviceConfig().VndkUseCoreVariant() && !m.MustUseVendorVariant()
		return lib.shared() && m.InVendor() && m.IsVndk() && !m.IsVndkExt() && !useCoreVariant
	}
	return false
}

// gather list of vndk-core, vndk-sp, and ll-ndk libs
func VndkMutator(mctx android.BottomUpMutatorContext) {
	m, ok := mctx.Module().(*Module)
	if !ok {
		return
	}

	if shouldSkipVndkMutator(m) {
		return
	}

	if _, ok := m.linker.(*llndkStubDecorator); ok {
		processLlndkLibrary(mctx, m)
		return
	}

	// This is a temporary measure to copy the properties from an llndk_library into the cc_library
	// that will actually build the stubs.  It will be removed once the properties are moved into
	// the cc_library in the Android.bp files.
	mergeLLNDKToLib := func(llndk *Module, llndkProperties *llndkLibraryProperties, flagExporter *flagExporter) {
		if llndkLib := moduleLibraryInterface(llndk); llndkLib != nil {
			*llndkProperties = llndkLib.(*llndkStubDecorator).Properties
			flagExporter.Properties = llndkLib.(*llndkStubDecorator).flagExporter.Properties

			m.VendorProperties.IsLLNDK = llndk.VendorProperties.IsLLNDK
			m.VendorProperties.IsLLNDKPrivate = llndk.VendorProperties.IsLLNDKPrivate
		}
	}

	lib, isLib := m.linker.(*libraryDecorator)
	prebuiltLib, isPrebuiltLib := m.linker.(*prebuiltLibraryLinker)

	if m.UseVndk() && isLib && lib.hasLLNDKStubs() {
		llndk := mctx.AddVariationDependencies(nil, llndkStubDepTag, String(lib.Properties.Llndk_stubs))
		mergeLLNDKToLib(llndk[0].(*Module), &lib.Properties.Llndk, &lib.flagExporter)
	}
	if m.UseVndk() && isPrebuiltLib && prebuiltLib.hasLLNDKStubs() {
		llndk := mctx.AddVariationDependencies(nil, llndkStubDepTag, String(prebuiltLib.Properties.Llndk_stubs))
		mergeLLNDKToLib(llndk[0].(*Module), &prebuiltLib.Properties.Llndk, &prebuiltLib.flagExporter)
	}

	if (isLib && lib.buildShared()) || (isPrebuiltLib && prebuiltLib.buildShared()) {
		if m.vndkdep != nil && m.vndkdep.isVndk() && !m.vndkdep.isVndkExt() {
			processVndkLibrary(mctx, m)
			return
		}
	}
}

func init() {
	RegisterVndkLibraryTxtTypes(android.InitRegistrationContext)
	android.RegisterSingletonType("vndk-snapshot", VndkSnapshotSingleton)
}

func RegisterVndkLibraryTxtTypes(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("llndk_libraries_txt", VndkLibrariesTxtFactory(libclangRTRemover(llndkLibraries)))
	ctx.RegisterModuleType("vndksp_libraries_txt", VndkLibrariesTxtFactory(vndkSpLibraries))
	ctx.RegisterModuleType("vndkcore_libraries_txt", VndkLibrariesTxtFactory(vndkCoreLibraries))
	ctx.RegisterModuleType("vndkprivate_libraries_txt", VndkLibrariesTxtFactory(vndkPrivateLibraries))
	ctx.RegisterModuleType("vndkproduct_libraries_txt", VndkLibrariesTxtFactory(vndkProductLibraries))
	ctx.RegisterModuleType("vndkcorevariant_libraries_txt", VndkLibrariesTxtFactory(vndkUsingCoreVariantLibraries))
}

type vndkLibrariesTxt struct {
	android.ModuleBase

	lister     func(android.Config) map[string]string
	properties VndkLibrariesTxtProperties

	outputFile android.OutputPath
}

type VndkLibrariesTxtProperties struct {
	Insert_vndk_version *bool
}

var _ etc.PrebuiltEtcModule = &vndkLibrariesTxt{}
var _ android.OutputFileProducer = &vndkLibrariesTxt{}

// vndk_libraries_txt is a special kind of module type in that it name is one of
// - llndk.libraries.txt
// - vndkcore.libraries.txt
// - vndksp.libraries.txt
// - vndkprivate.libraries.txt
// - vndkproduct.libraries.txt
// - vndkcorevariant.libraries.txt
// A module behaves like a prebuilt_etc but its content is generated by soong.
// By being a soong module, these files can be referenced by other soong modules.
// For example, apex_vndk can depend on these files as prebuilt.
func VndkLibrariesTxtFactory(lister func(android.Config) map[string]string) android.ModuleFactory {
	return func() android.Module {
		m := &vndkLibrariesTxt{
			lister: lister,
		}
		m.AddProperties(&m.properties)
		android.InitAndroidModule(m)
		return m
	}
}

func insertVndkVersion(filename string, vndkVersion string) string {
	if index := strings.LastIndex(filename, "."); index != -1 {
		return filename[:index] + "." + vndkVersion + filename[index:]
	}
	return filename
}

func libclangRTRemover(lister func(android.Config) map[string]string) func(android.Config) map[string]string {
	return func(config android.Config) map[string]string {
		libs := lister(config)
		filteredLibs := make(map[string]string, len(libs))
		for lib, v := range libs {
			if strings.HasPrefix(lib, "libclang_rt.hwasan-") {
				continue
			}
			filteredLibs[lib] = v
		}
		return filteredLibs
	}
}

func (txt *vndkLibrariesTxt) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	list := android.SortedStringMapValues(txt.lister(ctx.Config()))

	var filename string
	if BoolDefault(txt.properties.Insert_vndk_version, true) {
		filename = insertVndkVersion(txt.Name(), ctx.DeviceConfig().PlatformVndkVersion())
	} else {
		filename = txt.Name()
	}

	txt.outputFile = android.PathForModuleOut(ctx, filename).OutputPath
	android.WriteFileRule(ctx, txt.outputFile, strings.Join(list, "\n"))

	installPath := android.PathForModuleInstall(ctx, "etc")
	ctx.InstallFile(installPath, filename, txt.outputFile)
}

func (txt *vndkLibrariesTxt) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(txt.outputFile),
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_STEM", txt.outputFile.Base())
			},
		},
	}}
}

// PrebuiltEtcModule interface
func (txt *vndkLibrariesTxt) OutputFile() android.OutputPath {
	return txt.outputFile
}

// PrebuiltEtcModule interface
func (txt *vndkLibrariesTxt) BaseDir() string {
	return "etc"
}

// PrebuiltEtcModule interface
func (txt *vndkLibrariesTxt) SubDir() string {
	return ""
}

func (txt *vndkLibrariesTxt) OutputFiles(tag string) (android.Paths, error) {
	return android.Paths{txt.outputFile}, nil
}

func VndkSnapshotSingleton() android.Singleton {
	return &vndkSnapshotSingleton{}
}

type vndkSnapshotSingleton struct {
	vndkLibrariesFile   android.OutputPath
	vndkSnapshotZipFile android.OptionalPath
}

func isVndkSnapshotAware(config android.DeviceConfig, m *Module,
	apexInfo android.ApexInfo) (i snapshotLibraryInterface, vndkType string, isVndkSnapshotLib bool) {

	if m.Target().NativeBridge == android.NativeBridgeEnabled {
		return nil, "", false
	}
	// !inVendor: There's product/vendor variants for VNDK libs. We only care about vendor variants.
	// !installable: Snapshot only cares about "installable" modules.
	// isSnapshotPrebuilt: Snapshotting a snapshot doesn't make sense.
	if !m.InVendor() || !m.installable(apexInfo) || m.isSnapshotPrebuilt() {
		return nil, "", false
	}
	l, ok := m.linker.(snapshotLibraryInterface)
	if !ok || !l.shared() {
		return nil, "", false
	}
	if m.VndkVersion() == config.PlatformVndkVersion() && m.IsVndk() && !m.IsVndkExt() {
		if m.isVndkSp() {
			return l, "vndk-sp", true
		} else {
			return l, "vndk-core", true
		}
	}

	return nil, "", false
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

	var snapshotOutputs android.Paths

	/*
		VNDK snapshot zipped artifacts directory structure:
		{SNAPSHOT_ARCH}/
			arch-{TARGET_ARCH}-{TARGET_ARCH_VARIANT}/
				shared/
					vndk-core/
						(VNDK-core libraries, e.g. libbinder.so)
					vndk-sp/
						(VNDK-SP libraries, e.g. libc++.so)
			arch-{TARGET_2ND_ARCH}-{TARGET_2ND_ARCH_VARIANT}/
				shared/
					vndk-core/
						(VNDK-core libraries, e.g. libbinder.so)
					vndk-sp/
						(VNDK-SP libraries, e.g. libc++.so)
			binder32/
				(This directory is newly introduced in v28 (Android P) to hold
				prebuilts built for 32-bit binder interface.)
				arch-{TARGET_ARCH}-{TARGE_ARCH_VARIANT}/
					...
			configs/
				(various *.txt configuration files)
			include/
				(header files of same directory structure with source tree)
			NOTICE_FILES/
				(notice files of libraries, e.g. libcutils.so.txt)
	*/

	snapshotDir := "vndk-snapshot"
	snapshotArchDir := filepath.Join(snapshotDir, ctx.DeviceConfig().DeviceArch())

	configsDir := filepath.Join(snapshotArchDir, "configs")
	noticeDir := filepath.Join(snapshotArchDir, "NOTICE_FILES")
	includeDir := filepath.Join(snapshotArchDir, "include")

	// set of notice files copied.
	noticeBuilt := make(map[string]bool)

	// paths of VNDK modules for GPL license checking
	modulePaths := make(map[string]string)

	// actual module names of .so files
	// e.g. moduleNames["libprotobuf-cpp-full-3.9.1.so"] = "libprotobuf-cpp-full"
	moduleNames := make(map[string]string)

	var headers android.Paths

	// installVndkSnapshotLib copies built .so file from the module.
	// Also, if the build artifacts is on, write a json file which contains all exported flags
	// with FlagExporterInfo.
	installVndkSnapshotLib := func(m *Module, vndkType string) (android.Paths, bool) {
		var ret android.Paths

		targetArch := "arch-" + m.Target().Arch.ArchType.String()
		if m.Target().Arch.ArchVariant != "" {
			targetArch += "-" + m.Target().Arch.ArchVariant
		}

		libPath := m.outputFile.Path()
		snapshotLibOut := filepath.Join(snapshotArchDir, targetArch, "shared", vndkType, libPath.Base())
		ret = append(ret, copyFileRule(ctx, libPath, snapshotLibOut))

		if ctx.Config().VndkSnapshotBuildArtifacts() {
			prop := struct {
				ExportedDirs        []string `json:",omitempty"`
				ExportedSystemDirs  []string `json:",omitempty"`
				ExportedFlags       []string `json:",omitempty"`
				RelativeInstallPath string   `json:",omitempty"`
			}{}
			exportedInfo := ctx.ModuleProvider(m, FlagExporterInfoProvider).(FlagExporterInfo)
			prop.ExportedFlags = exportedInfo.Flags
			prop.ExportedDirs = exportedInfo.IncludeDirs.Strings()
			prop.ExportedSystemDirs = exportedInfo.SystemIncludeDirs.Strings()
			prop.RelativeInstallPath = m.RelativeInstallPath()

			propOut := snapshotLibOut + ".json"

			j, err := json.Marshal(prop)
			if err != nil {
				ctx.Errorf("json marshal to %q failed: %#v", propOut, err)
				return nil, false
			}
			ret = append(ret, writeStringToFileRule(ctx, string(j), propOut))
		}
		return ret, true
	}

	ctx.VisitAllModules(func(module android.Module) {
		m, ok := module.(*Module)
		if !ok || !m.Enabled() {
			return
		}

		apexInfo := ctx.ModuleProvider(module, android.ApexInfoProvider).(android.ApexInfo)

		l, vndkType, ok := isVndkSnapshotAware(ctx.DeviceConfig(), m, apexInfo)
		if !ok {
			return
		}

		// For all snapshot candidates, the followings are captured.
		//   - .so files
		//   - notice files
		//
		// The followings are also captured if VNDK_SNAPSHOT_BUILD_ARTIFACTS.
		//   - .json files containing exported flags
		//   - exported headers from collectHeadersForSnapshot()
		//
		// Headers are deduplicated after visiting all modules.

		// install .so files for appropriate modules.
		// Also install .json files if VNDK_SNAPSHOT_BUILD_ARTIFACTS
		libs, ok := installVndkSnapshotLib(m, vndkType)
		if !ok {
			return
		}
		snapshotOutputs = append(snapshotOutputs, libs...)

		// These are for generating module_names.txt and module_paths.txt
		stem := m.outputFile.Path().Base()
		moduleNames[stem] = ctx.ModuleName(m)
		modulePaths[stem] = ctx.ModuleDir(m)

		if len(m.NoticeFiles()) > 0 {
			noticeName := stem + ".txt"
			// skip already copied notice file
			if _, ok := noticeBuilt[noticeName]; !ok {
				noticeBuilt[noticeName] = true
				snapshotOutputs = append(snapshotOutputs, combineNoticesRule(
					ctx, m.NoticeFiles(), filepath.Join(noticeDir, noticeName)))
			}
		}

		if ctx.Config().VndkSnapshotBuildArtifacts() {
			headers = append(headers, l.snapshotHeaders()...)
		}
	})

	// install all headers after removing duplicates
	for _, header := range android.FirstUniquePaths(headers) {
		snapshotOutputs = append(snapshotOutputs, copyFileRule(
			ctx, header, filepath.Join(includeDir, header.String())))
	}

	// install *.libraries.txt except vndkcorevariant.libraries.txt
	ctx.VisitAllModules(func(module android.Module) {
		m, ok := module.(*vndkLibrariesTxt)
		if !ok || !m.Enabled() || m.Name() == vndkUsingCoreVariantLibrariesTxt {
			return
		}
		snapshotOutputs = append(snapshotOutputs, copyFileRule(
			ctx, m.OutputFile(), filepath.Join(configsDir, m.Name())))
	})

	/*
		module_paths.txt contains paths on which VNDK modules are defined.
		e.g.,
			libbase.so system/libbase
			libc.so bionic/libc
			...
	*/
	snapshotOutputs = append(snapshotOutputs, installMapListFileRule(ctx, modulePaths, filepath.Join(configsDir, "module_paths.txt")))

	/*
		module_names.txt contains names as which VNDK modules are defined,
		because output filename and module name can be different with stem and suffix properties.

		e.g.,
			libcutils.so libcutils
			libprotobuf-cpp-full-3.9.2.so libprotobuf-cpp-full
			...
	*/
	snapshotOutputs = append(snapshotOutputs, installMapListFileRule(ctx, moduleNames, filepath.Join(configsDir, "module_names.txt")))

	// All artifacts are ready. Sort them to normalize ninja and then zip.
	sort.Slice(snapshotOutputs, func(i, j int) bool {
		return snapshotOutputs[i].String() < snapshotOutputs[j].String()
	})

	zipPath := android.PathForOutput(ctx, snapshotDir, "android-vndk-"+ctx.DeviceConfig().DeviceArch()+".zip")
	zipRule := android.NewRuleBuilder(pctx, ctx)

	// filenames in rspfile from FlagWithRspFileInputList might be single-quoted. Remove it with tr
	snapshotOutputList := android.PathForOutput(ctx, snapshotDir, "android-vndk-"+ctx.DeviceConfig().DeviceArch()+"_list")
	zipRule.Command().
		Text("tr").
		FlagWithArg("-d ", "\\'").
		FlagWithRspFileInputList("< ", snapshotOutputs).
		FlagWithOutput("> ", snapshotOutputList)

	zipRule.Temporary(snapshotOutputList)

	zipRule.Command().
		BuiltTool("soong_zip").
		FlagWithOutput("-o ", zipPath).
		FlagWithArg("-C ", android.PathForOutput(ctx, snapshotDir).String()).
		FlagWithInput("-l ", snapshotOutputList)

	zipRule.Build(zipPath.String(), "vndk snapshot "+zipPath.String())
	zipRule.DeleteTemporaryFiles()
	c.vndkSnapshotZipFile = android.OptionalPathForPath(zipPath)
}

func getVndkFileName(m *Module) (string, error) {
	if library, ok := m.linker.(*libraryDecorator); ok {
		return library.getLibNameHelper(m.BaseModuleName(), true, false) + ".so", nil
	}
	if prebuilt, ok := m.linker.(*prebuiltLibraryLinker); ok {
		return prebuilt.libraryDecorator.getLibNameHelper(m.BaseModuleName(), true, false) + ".so", nil
	}
	return "", fmt.Errorf("VNDK library should have libraryDecorator or prebuiltLibraryLinker as linker: %T", m.linker)
}

func (c *vndkSnapshotSingleton) buildVndkLibrariesTxtFiles(ctx android.SingletonContext) {
	llndk := android.SortedStringMapValues(llndkLibraries(ctx.Config()))
	vndkcore := android.SortedStringMapValues(vndkCoreLibraries(ctx.Config()))
	vndksp := android.SortedStringMapValues(vndkSpLibraries(ctx.Config()))
	vndkprivate := android.SortedStringMapValues(vndkPrivateLibraries(ctx.Config()))
	vndkproduct := android.SortedStringMapValues(vndkProductLibraries(ctx.Config()))

	// Build list of vndk libs as merged & tagged & filter-out(libclang_rt):
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
	merged = append(merged, addPrefix(filterOutLibClangRt(vndkproduct), "VNDK-product: ")...)
	c.vndkLibrariesFile = android.PathForOutput(ctx, "vndk", "vndk.libraries.txt")
	android.WriteFileRule(ctx, c.vndkLibrariesFile, strings.Join(merged, "\n"))
}

func (c *vndkSnapshotSingleton) MakeVars(ctx android.MakeVarsContext) {
	// Make uses LLNDK_MOVED_TO_APEX_LIBRARIES to avoid installing libraries on /system if
	// they been moved to an apex.
	movedToApexLlndkLibraries := make(map[string]bool)
	ctx.VisitAllModules(func(module android.Module) {
		if library := moduleLibraryInterface(module); library != nil && library.hasLLNDKStubs() {
			// Skip bionic libs, they are handled in different manner
			name := library.implementationModuleName(module.(*Module).BaseModuleName())
			if module.(android.ApexModule).DirectlyInAnyApex() && !isBionic(name) {
				movedToApexLlndkLibraries[name] = true
			}
		}
	})

	ctx.Strict("LLNDK_MOVED_TO_APEX_LIBRARIES",
		strings.Join(android.SortedStringKeys(movedToApexLlndkLibraries), " "))

	// Make uses LLNDK_LIBRARIES to determine which libraries to install.
	// HWASAN is only part of the LL-NDK in builds in which libc depends on HWASAN.
	// Therefore, by removing the library here, we cause it to only be installed if libc
	// depends on it.
	installedLlndkLibraries := []string{}
	for lib := range llndkLibraries(ctx.Config()) {
		if strings.HasPrefix(lib, "libclang_rt.hwasan-") {
			continue
		}
		installedLlndkLibraries = append(installedLlndkLibraries, lib)
	}
	sort.Strings(installedLlndkLibraries)
	ctx.Strict("LLNDK_LIBRARIES", strings.Join(installedLlndkLibraries, " "))

	ctx.Strict("VNDK_CORE_LIBRARIES", strings.Join(android.SortedStringKeys(vndkCoreLibraries(ctx.Config())), " "))
	ctx.Strict("VNDK_SAMEPROCESS_LIBRARIES", strings.Join(android.SortedStringKeys(vndkSpLibraries(ctx.Config())), " "))
	ctx.Strict("VNDK_PRIVATE_LIBRARIES", strings.Join(android.SortedStringKeys(vndkPrivateLibraries(ctx.Config())), " "))
	ctx.Strict("VNDK_USING_CORE_VARIANT_LIBRARIES", strings.Join(android.SortedStringKeys(vndkUsingCoreVariantLibraries(ctx.Config())), " "))

	ctx.Strict("VNDK_LIBRARIES_FILE", c.vndkLibrariesFile.String())
	ctx.Strict("SOONG_VNDK_SNAPSHOT_ZIP", c.vndkSnapshotZipFile.String())
}
