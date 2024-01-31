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

	"android/soong/android"
	"android/soong/cc/config"
	"android/soong/etc"
	"android/soong/snapshot"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

const (
	llndkLibrariesTxt                = "llndk.libraries.txt"
	llndkLibrariesTxtForApex         = "llndk.libraries.txt.apex"
	vndkCoreLibrariesTxt             = "vndkcore.libraries.txt"
	vndkSpLibrariesTxt               = "vndksp.libraries.txt"
	vndkPrivateLibrariesTxt          = "vndkprivate.libraries.txt"
	vndkProductLibrariesTxt          = "vndkproduct.libraries.txt"
	vndkUsingCoreVariantLibrariesTxt = "vndkcorevariant.libraries.txt"
)

func VndkLibrariesTxtModules(vndkVersion string, ctx android.BaseModuleContext) []string {
	// Return the list of vndk txt files for the vndk apex of the vndkVersion.
	if vndkVersion == "current" {
		// We can assume all txt files are snapshotted if we find one of them.
		currentVndkSnapshotted := ctx.OtherModuleExists(insertVndkVersion(llndkLibrariesTxt, ctx.DeviceConfig().PlatformVndkVersion()))
		if currentVndkSnapshotted {
			// If the current VNDK is already snapshotted (which can happen with
			// the `next` config), use the prebuilt txt files in the snapshot.
			// This is because the txt files built from source are probably be
			// for the in-development version.
			vndkVersion = ctx.DeviceConfig().PlatformVndkVersion()
		} else {
			// Use the txt files generated from the source
			return []string{
				llndkLibrariesTxtForApex,
				vndkCoreLibrariesTxt,
				vndkSpLibrariesTxt,
				vndkPrivateLibrariesTxt,
				vndkProductLibrariesTxt,
			}
		}
	}

	// Snapshot vndks have their own *.libraries.VER.txt files.
	// Note that snapshots don't have "vndkcorevariant.libraries.VER.txt"
	result := []string{
		insertVndkVersion(vndkCoreLibrariesTxt, vndkVersion),
		insertVndkVersion(vndkSpLibrariesTxt, vndkVersion),
		insertVndkVersion(vndkPrivateLibrariesTxt, vndkVersion),
		insertVndkVersion(vndkProductLibrariesTxt, vndkVersion),
		insertVndkVersion(llndkLibrariesTxt, vndkVersion),
	}

	return result
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

type moduleListerFunc func(ctx android.SingletonContext) (moduleNames, fileNames []string)

var (
	llndkLibraries                = vndkModuleLister(func(m *Module) bool { return m.VendorProperties.IsLLNDK && !m.Header() })
	vndkSPLibraries               = vndkModuleLister(func(m *Module) bool { return m.VendorProperties.IsVNDKSP })
	vndkCoreLibraries             = vndkModuleLister(func(m *Module) bool { return m.VendorProperties.IsVNDKCore })
	vndkPrivateLibraries          = vndkModuleLister(func(m *Module) bool { return m.VendorProperties.IsVNDKPrivate })
	vndkProductLibraries          = vndkModuleLister(func(m *Module) bool { return m.VendorProperties.IsVNDKProduct })
	vndkUsingCoreVariantLibraries = vndkModuleLister(func(m *Module) bool { return m.VendorProperties.IsVNDKUsingCoreVariant })
)

// vndkModuleLister takes a predicate that operates on a Module and returns a moduleListerFunc
// that produces a list of module names and output file names for which the predicate returns true.
func vndkModuleLister(predicate func(*Module) bool) moduleListerFunc {
	return func(ctx android.SingletonContext) (moduleNames, fileNames []string) {
		ctx.VisitAllModules(func(m android.Module) {
			if c, ok := m.(*Module); ok && predicate(c) && !c.IsVndkPrebuiltLibrary() {
				filename, err := getVndkFileName(c)
				if err != nil {
					ctx.ModuleErrorf(m, "%s", err)
				}
				moduleNames = append(moduleNames, ctx.ModuleName(m))
				fileNames = append(fileNames, filename)
			}
		})
		moduleNames = android.SortedUniqueStrings(moduleNames)
		fileNames = android.SortedUniqueStrings(fileNames)
		return
	}
}

// vndkModuleListRemover takes a moduleListerFunc and a prefix and returns a moduleListerFunc
// that returns the same lists as the input moduleListerFunc, but with  modules with the
// given prefix removed.
func vndkModuleListRemover(lister moduleListerFunc, prefix string) moduleListerFunc {
	return func(ctx android.SingletonContext) (moduleNames, fileNames []string) {
		moduleNames, fileNames = lister(ctx)
		filter := func(in []string) []string {
			out := make([]string, 0, len(in))
			for _, lib := range in {
				if strings.HasPrefix(lib, prefix) {
					continue
				}
				out = append(out, lib)
			}
			return out
		}
		return filter(moduleNames), filter(fileNames)
	}
}

var vndkMustUseVendorVariantListKey = android.NewOnceKey("vndkMustUseVendorVariantListKey")

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

func processVndkLibrary(mctx android.BottomUpMutatorContext, m *Module) {
	if m.InProduct() {
		// We may skip the steps for the product variants because they
		// are already covered by the vendor variants.
		return
	}

	name := m.BaseModuleName()

	if lib := m.library; lib != nil && lib.hasStubsVariants() && name != "libz" {
		// b/155456180 libz is the ONLY exception here. We don't want to make
		// libz an LLNDK library because we in general can't guarantee that
		// libz will behave consistently especially about the compression.
		// i.e. the compressed output might be different across releases.
		// As the library is an external one, it's risky to keep the compatibility
		// promise if it becomes an LLNDK.
		mctx.PropertyErrorf("vndk.enabled", "This library provides stubs. Shouldn't be VNDK. Consider making it as LLNDK")
	}

	if inList(name, vndkMustUseVendorVariantList(mctx.Config())) {
		m.Properties.MustUseVendorVariant = true
	}
	if mctx.DeviceConfig().VndkUseCoreVariant() && !m.Properties.MustUseVendorVariant {
		m.VendorProperties.IsVNDKUsingCoreVariant = true
	}

	if m.vndkdep.isVndkSp() {
		m.VendorProperties.IsVNDKSP = true
	} else {
		m.VendorProperties.IsVNDKCore = true
	}
	if m.IsVndkPrivate() {
		m.VendorProperties.IsVNDKPrivate = true
	}
	if Bool(m.VendorProperties.Product_available) {
		m.VendorProperties.IsVNDKProduct = true
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

	// TODO(b/142675459): Use enabled: to select target device in vndk_prebuilt_shared
	// When b/142675459 is landed, remove following check
	if p, ok := m.linker.(*vndkPrebuiltLibraryDecorator); ok {
		// prebuilt vndk modules should match with device
		if !p.MatchesWithDevice(mctx.DeviceConfig()) {
			return false
		}

		platformVndkVersion := mctx.DeviceConfig().PlatformVndkVersion()
		if platformVndkVersion != "" {
			// ignore prebuilt vndk modules that are newer than or equal to the platform vndk version
			platformVndkApiLevel := android.ApiLevelOrPanic(mctx, platformVndkVersion)
			if platformVndkApiLevel.LessThanOrEqualTo(android.ApiLevelOrPanic(mctx, p.Version())) {
				return false
			}
		}
	}

	if lib, ok := m.linker.(libraryInterface); ok {
		// VNDK APEX doesn't need stub variants
		if lib.buildStubs() {
			return false
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

	lib, isLib := m.linker.(*libraryDecorator)
	prebuiltLib, isPrebuiltLib := m.linker.(*prebuiltLibraryLinker)

	if m.InVendorOrProduct() && isLib && lib.hasLLNDKStubs() {
		m.VendorProperties.IsLLNDK = true
		m.VendorProperties.IsVNDKPrivate = Bool(lib.Properties.Llndk.Private)
	}
	if m.InVendorOrProduct() && isPrebuiltLib && prebuiltLib.hasLLNDKStubs() {
		m.VendorProperties.IsLLNDK = true
		m.VendorProperties.IsVNDKPrivate = Bool(prebuiltLib.Properties.Llndk.Private)
	}

	if m.IsVndkPrebuiltLibrary() && !m.IsVndk() {
		m.VendorProperties.IsLLNDK = true
		// TODO(b/280697209): copy "llndk.private" flag to vndk_prebuilt_shared
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
	android.RegisterParallelSingletonType("vndk-snapshot", VndkSnapshotSingleton)
}

func RegisterVndkLibraryTxtTypes(ctx android.RegistrationContext) {
	ctx.RegisterParallelSingletonModuleType("llndk_libraries_txt", llndkLibrariesTxtFactory)
	ctx.RegisterParallelSingletonModuleType("llndk_libraries_txt_for_apex", llndkLibrariesTxtApexOnlyFactory)
	ctx.RegisterParallelSingletonModuleType("vndksp_libraries_txt", vndkSPLibrariesTxtFactory)
	ctx.RegisterParallelSingletonModuleType("vndkcore_libraries_txt", vndkCoreLibrariesTxtFactory)
	ctx.RegisterParallelSingletonModuleType("vndkprivate_libraries_txt", vndkPrivateLibrariesTxtFactory)
	ctx.RegisterParallelSingletonModuleType("vndkproduct_libraries_txt", vndkProductLibrariesTxtFactory)
	ctx.RegisterParallelSingletonModuleType("vndkcorevariant_libraries_txt", vndkUsingCoreVariantLibrariesTxtFactory)
}

type vndkLibrariesTxt struct {
	android.SingletonModuleBase

	lister               moduleListerFunc
	makeVarName          string
	filterOutFromMakeVar string

	properties VndkLibrariesTxtProperties

	outputFile  android.OutputPath
	moduleNames []string
	fileNames   []string
}

type VndkLibrariesTxtProperties struct {
	Insert_vndk_version *bool
	Stem                *string
}

var _ etc.PrebuiltEtcModule = &vndkLibrariesTxt{}
var _ android.OutputFileProducer = &vndkLibrariesTxt{}

// llndk_libraries_txt is a singleton module whose content is a list of LLNDK libraries
// generated by Soong.
// Make uses LLNDK_LIBRARIES to determine which libraries to install.
// HWASAN is only part of the LLNDK in builds in which libc depends on HWASAN.
// Therefore, by removing the library here, we cause it to only be installed if libc
// depends on it.
func llndkLibrariesTxtFactory() android.SingletonModule {
	return newVndkLibrariesWithMakeVarFilter(llndkLibraries, "LLNDK_LIBRARIES", "libclang_rt.hwasan")
}

// llndk_libraries_txt_for_apex is a singleton module that provide the same LLNDK libraries list
// with the llndk_libraries_txt, but skips setting make variable LLNDK_LIBRARIES. So, it must not
// be used without installing llndk_libraries_txt singleton.
// We include llndk_libraries_txt by default to install the llndk.libraries.txt file to system/etc.
// This singleton module is to install the llndk.libraries.<ver>.txt file to vndk apex.
func llndkLibrariesTxtApexOnlyFactory() android.SingletonModule {
	return newVndkLibrariesWithMakeVarFilter(llndkLibraries, "", "libclang_rt.hwasan")
}

// vndksp_libraries_txt is a singleton module whose content is a list of VNDKSP libraries
// generated by Soong but can be referenced by other modules.
// For example, apex_vndk can depend on these files as prebuilt.
func vndkSPLibrariesTxtFactory() android.SingletonModule {
	return newVndkLibrariesTxt(vndkSPLibraries, "VNDK_SAMEPROCESS_LIBRARIES")
}

// vndkcore_libraries_txt is a singleton module whose content is a list of VNDK core libraries
// generated by Soong but can be referenced by other modules.
// For example, apex_vndk can depend on these files as prebuilt.
func vndkCoreLibrariesTxtFactory() android.SingletonModule {
	return newVndkLibrariesTxt(vndkCoreLibraries, "VNDK_CORE_LIBRARIES")
}

// vndkprivate_libraries_txt is a singleton module whose content is a list of VNDK private libraries
// generated by Soong but can be referenced by other modules.
// For example, apex_vndk can depend on these files as prebuilt.
func vndkPrivateLibrariesTxtFactory() android.SingletonModule {
	return newVndkLibrariesTxt(vndkPrivateLibraries, "VNDK_PRIVATE_LIBRARIES")
}

// vndkproduct_libraries_txt is a singleton module whose content is a list of VNDK product libraries
// generated by Soong but can be referenced by other modules.
// For example, apex_vndk can depend on these files as prebuilt.
func vndkProductLibrariesTxtFactory() android.SingletonModule {
	return newVndkLibrariesTxt(vndkProductLibraries, "VNDK_PRODUCT_LIBRARIES")
}

// vndkcorevariant_libraries_txt is a singleton module whose content is a list of VNDK libraries
// that are using the core variant, generated by Soong but can be referenced by other modules.
// For example, apex_vndk can depend on these files as prebuilt.
func vndkUsingCoreVariantLibrariesTxtFactory() android.SingletonModule {
	return newVndkLibrariesTxt(vndkUsingCoreVariantLibraries, "VNDK_USING_CORE_VARIANT_LIBRARIES")
}

func newVndkLibrariesWithMakeVarFilter(lister moduleListerFunc, makeVarName string, filter string) android.SingletonModule {
	m := &vndkLibrariesTxt{
		lister:               lister,
		makeVarName:          makeVarName,
		filterOutFromMakeVar: filter,
	}
	m.AddProperties(&m.properties)
	android.InitAndroidArchModule(m, android.DeviceSupported, android.MultilibCommon)
	return m
}

func newVndkLibrariesTxt(lister moduleListerFunc, makeVarName string) android.SingletonModule {
	return newVndkLibrariesWithMakeVarFilter(lister, makeVarName, "")
}

func insertVndkVersion(filename string, vndkVersion string) string {
	if index := strings.LastIndex(filename, "."); index != -1 {
		return filename[:index] + "." + vndkVersion + filename[index:]
	}
	return filename
}

func (txt *vndkLibrariesTxt) DepsMutator(mctx android.BottomUpMutatorContext) {
	versionedName := insertVndkVersion(txt.Name(), mctx.DeviceConfig().PlatformVndkVersion())
	if mctx.OtherModuleExists(versionedName) {
		// If the prebuilt vndk libraries txt files exist, install them instead.
		txt.HideFromMake()
		mctx.AddDependency(txt, nil, versionedName)
	}
}

func (txt *vndkLibrariesTxt) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	filename := proptools.StringDefault(txt.properties.Stem, txt.Name())

	if Bool(txt.properties.Insert_vndk_version) {
		filename = insertVndkVersion(filename, ctx.DeviceConfig().PlatformVndkVersion())
	}

	txt.outputFile = android.PathForModuleOut(ctx, filename).OutputPath

	installPath := android.PathForModuleInstall(ctx, "etc")
	ctx.InstallFile(installPath, filename, txt.outputFile)
}

func (txt *vndkLibrariesTxt) GenerateSingletonBuildActions(ctx android.SingletonContext) {
	txt.moduleNames, txt.fileNames = txt.lister(ctx)
	android.WriteFileRule(ctx, txt.outputFile, strings.Join(txt.fileNames, "\n"))
}

func (txt *vndkLibrariesTxt) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(txt.outputFile),
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_STEM", txt.outputFile.Base())
			},
		},
	}}
}

func (txt *vndkLibrariesTxt) MakeVars(ctx android.MakeVarsContext) {
	if txt.makeVarName == "" {
		return
	}

	filter := func(modules []string, prefix string) []string {
		if prefix == "" {
			return modules
		}
		var result []string
		for _, module := range modules {
			if strings.HasPrefix(module, prefix) {
				continue
			} else {
				result = append(result, module)
			}
		}
		return result
	}
	ctx.Strict(txt.makeVarName, strings.Join(filter(txt.moduleNames, txt.filterOutFromMakeVar), " "))
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

func isVndkSnapshotAware(config android.DeviceConfig, m LinkableInterface,
	apexInfo android.ApexInfo) (vndkType string, isVndkSnapshotLib bool) {

	if m.Target().NativeBridge == android.NativeBridgeEnabled {
		return "", false
	}
	// !inVendor: There's product/vendor variants for VNDK libs. We only care about vendor variants.
	// !installable: Snapshot only cares about "installable" modules.
	// !m.IsLlndk: llndk stubs are required for building against snapshots.
	// IsSnapshotPrebuilt: Snapshotting a snapshot doesn't make sense.
	// !outputFile.Valid: Snapshot requires valid output file.
	if !m.InVendor() || (!installable(m, apexInfo) && !m.IsLlndk()) || m.IsSnapshotPrebuilt() || !m.OutputFile().Valid() {
		return "", false
	}
	if !m.IsSnapshotLibrary() || !m.Shared() {
		return "", false
	}
	if m.VndkVersion() == config.PlatformVndkVersion() {
		if m.IsVndk() && !m.IsVndkExt() {
			if m.IsVndkSp() {
				return "vndk-sp", true
			} else {
				return "vndk-core", true
			}
		} else if m.HasLlndkStubs() && m.StubsVersion() == "" {
			// Use default version for the snapshot.
			return "llndk-stub", true
		}
	}

	return "", false
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
					llndk-stub/
						(LLNDK stub libraries)
			arch-{TARGET_2ND_ARCH}-{TARGET_2ND_ARCH_VARIANT}/
				shared/
					vndk-core/
						(VNDK-core libraries, e.g. libbinder.so)
					vndk-sp/
						(VNDK-SP libraries, e.g. libc++.so)
					llndk-stub/
						(LLNDK stub libraries)
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
		ret = append(ret, snapshot.CopyFileRule(pctx, ctx, libPath, snapshotLibOut))

		// json struct to export snapshot information
		prop := struct {
			MinSdkVersion       string   `json:",omitempty"`
			LicenseKinds        []string `json:",omitempty"`
			LicenseTexts        []string `json:",omitempty"`
			ExportedDirs        []string `json:",omitempty"`
			ExportedSystemDirs  []string `json:",omitempty"`
			ExportedFlags       []string `json:",omitempty"`
			RelativeInstallPath string   `json:",omitempty"`
		}{}

		prop.LicenseKinds = m.EffectiveLicenseKinds()
		prop.LicenseTexts = m.EffectiveLicenseFiles().Strings()
		prop.MinSdkVersion = m.MinSdkVersion()

		if ctx.Config().VndkSnapshotBuildArtifacts() {
			exportedInfo, _ := android.SingletonModuleProvider(ctx, m, FlagExporterInfoProvider)
			prop.ExportedFlags = exportedInfo.Flags
			prop.ExportedDirs = exportedInfo.IncludeDirs.Strings()
			prop.ExportedSystemDirs = exportedInfo.SystemIncludeDirs.Strings()
			prop.RelativeInstallPath = m.RelativeInstallPath()
		}

		propOut := snapshotLibOut + ".json"

		j, err := json.Marshal(prop)
		if err != nil {
			ctx.Errorf("json marshal to %q failed: %#v", propOut, err)
			return nil, false
		}
		ret = append(ret, snapshot.WriteStringToFileRule(ctx, string(j), propOut))

		return ret, true
	}

	ctx.VisitAllModules(func(module android.Module) {
		m, ok := module.(*Module)
		if !ok || !m.Enabled() {
			return
		}

		apexInfo, _ := android.SingletonModuleProvider(ctx, module, android.ApexInfoProvider)

		vndkType, ok := isVndkSnapshotAware(ctx.DeviceConfig(), m, apexInfo)
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

		for _, notice := range m.EffectiveLicenseFiles() {
			if _, ok := noticeBuilt[notice.String()]; !ok {
				noticeBuilt[notice.String()] = true
				snapshotOutputs = append(snapshotOutputs, snapshot.CopyFileRule(
					pctx, ctx, notice, filepath.Join(noticeDir, notice.String())))
			}
		}

		if ctx.Config().VndkSnapshotBuildArtifacts() {
			headers = append(headers, m.SnapshotHeaders()...)
		}
	})

	// install all headers after removing duplicates
	for _, header := range android.FirstUniquePaths(headers) {
		snapshotOutputs = append(snapshotOutputs, snapshot.CopyFileRule(
			pctx, ctx, header, filepath.Join(includeDir, header.String())))
	}

	// install *.libraries.txt except vndkcorevariant.libraries.txt
	ctx.VisitAllModules(func(module android.Module) {
		m, ok := module.(*vndkLibrariesTxt)
		if !ok || !m.Enabled() || m.Name() == vndkUsingCoreVariantLibrariesTxt {
			return
		}
		snapshotOutputs = append(snapshotOutputs, snapshot.CopyFileRule(
			pctx, ctx, m.OutputFile(), filepath.Join(configsDir, m.Name())))
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
	rspFile := snapshotOutputList.ReplaceExtension(ctx, "rsp")
	zipRule.Command().
		Text("tr").
		FlagWithArg("-d ", "\\'").
		FlagWithRspFileInputList("< ", rspFile, snapshotOutputs).
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
	// Build list of vndk libs as merged & tagged & filter-out(libclang_rt):
	// Since each target have different set of libclang_rt.* files,
	// keep the common set of files in vndk.libraries.txt
	_, llndk := vndkModuleListRemover(llndkLibraries, "libclang_rt.")(ctx)
	_, vndkcore := vndkModuleListRemover(vndkCoreLibraries, "libclang_rt.")(ctx)
	_, vndksp := vndkSPLibraries(ctx)
	_, vndkprivate := vndkPrivateLibraries(ctx)
	_, vndkproduct := vndkModuleListRemover(vndkProductLibraries, "libclang_rt.")(ctx)
	var merged []string
	merged = append(merged, addPrefix(llndk, "LLNDK: ")...)
	merged = append(merged, addPrefix(vndksp, "VNDK-SP: ")...)
	merged = append(merged, addPrefix(vndkcore, "VNDK-core: ")...)
	merged = append(merged, addPrefix(vndkprivate, "VNDK-private: ")...)
	merged = append(merged, addPrefix(vndkproduct, "VNDK-product: ")...)
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
		strings.Join(android.SortedKeys(movedToApexLlndkLibraries), " "))

	ctx.Strict("VNDK_LIBRARIES_FILE", c.vndkLibrariesFile.String())
	ctx.Strict("SOONG_VNDK_SNAPSHOT_ZIP", c.vndkSnapshotZipFile.String())
}
