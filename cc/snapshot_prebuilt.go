// Copyright 2020 The Android Open Source Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package cc

// This file defines snapshot prebuilt modules, e.g. vendor snapshot and recovery snapshot. Such
// snapshot modules will override original source modules with setting BOARD_VNDK_VERSION, with
// snapshot mutators and snapshot information maps which are also defined in this file.

import (
	"fmt"
	"strings"

	"android/soong/android"
	"android/soong/snapshot"

	"github.com/google/blueprint"
)

// This interface overrides snapshot.SnapshotImage to implement cc module specific functions
type SnapshotImage interface {
	snapshot.SnapshotImage

	// The image variant name for this snapshot image.
	// For example, recovery snapshot image will return "recovery", and vendor snapshot image will
	// return "vendor." + version.
	imageVariantName(cfg android.DeviceConfig) string

	// The variant suffix for snapshot modules. For example, vendor snapshot modules will have
	// ".vendor" as their suffix.
	moduleNameSuffix() string
}

type vendorSnapshotImage struct {
	*snapshot.VendorSnapshotImage
}

type recoverySnapshotImage struct {
	*snapshot.RecoverySnapshotImage
}

func (vendorSnapshotImage) imageVariantName(cfg android.DeviceConfig) string {
	return VendorVariationPrefix + cfg.VndkVersion()
}

func (vendorSnapshotImage) moduleNameSuffix() string {
	return VendorSuffix
}

func (recoverySnapshotImage) imageVariantName(cfg android.DeviceConfig) string {
	return android.RecoveryVariation
}

func (recoverySnapshotImage) moduleNameSuffix() string {
	return RecoverySuffix
}

// Override existing vendor and recovery snapshot for cc module specific extra functions
var VendorSnapshotImageSingleton vendorSnapshotImage = vendorSnapshotImage{&snapshot.VendorSnapshotImageSingleton}
var RecoverySnapshotImageSingleton recoverySnapshotImage = recoverySnapshotImage{&snapshot.RecoverySnapshotImageSingleton}

func RegisterVendorSnapshotModules(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("vendor_snapshot", vendorSnapshotFactory)
	ctx.RegisterModuleType("vendor_snapshot_shared", VendorSnapshotSharedFactory)
	ctx.RegisterModuleType("vendor_snapshot_static", VendorSnapshotStaticFactory)
	ctx.RegisterModuleType("vendor_snapshot_header", VendorSnapshotHeaderFactory)
	ctx.RegisterModuleType("vendor_snapshot_binary", VendorSnapshotBinaryFactory)
	ctx.RegisterModuleType("vendor_snapshot_object", VendorSnapshotObjectFactory)
}

func RegisterRecoverySnapshotModules(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("recovery_snapshot", recoverySnapshotFactory)
	ctx.RegisterModuleType("recovery_snapshot_shared", RecoverySnapshotSharedFactory)
	ctx.RegisterModuleType("recovery_snapshot_static", RecoverySnapshotStaticFactory)
	ctx.RegisterModuleType("recovery_snapshot_header", RecoverySnapshotHeaderFactory)
	ctx.RegisterModuleType("recovery_snapshot_binary", RecoverySnapshotBinaryFactory)
	ctx.RegisterModuleType("recovery_snapshot_object", RecoverySnapshotObjectFactory)
}

func init() {
	RegisterVendorSnapshotModules(android.InitRegistrationContext)
	RegisterRecoverySnapshotModules(android.InitRegistrationContext)
	android.RegisterMakeVarsProvider(pctx, snapshotMakeVarsProvider)
}

const (
	snapshotHeaderSuffix = "_header."
	SnapshotSharedSuffix = "_shared."
	SnapshotStaticSuffix = "_static."
	snapshotBinarySuffix = "_binary."
	snapshotObjectSuffix = "_object."
	SnapshotRlibSuffix   = "_rlib."
	SnapshotDylibSuffix  = "_dylib."
)

type SnapshotProperties struct {
	Header_libs []string `android:"arch_variant"`
	Static_libs []string `android:"arch_variant"`
	Shared_libs []string `android:"arch_variant"`
	Rlibs       []string `android:"arch_variant"`
	Dylibs      []string `android:"arch_variant"`
	Vndk_libs   []string `android:"arch_variant"`
	Binaries    []string `android:"arch_variant"`
	Objects     []string `android:"arch_variant"`
}
type snapshotModule struct {
	android.ModuleBase

	properties SnapshotProperties

	baseSnapshot BaseSnapshotDecorator

	image SnapshotImage
}

func (s *snapshotModule) ImageMutatorBegin(ctx android.BaseModuleContext) {
	cfg := ctx.DeviceConfig()
	if !s.image.IsUsingSnapshot(cfg) || s.image.TargetSnapshotVersion(cfg) != s.baseSnapshot.Version() {
		s.Disable()
	}
}

func (s *snapshotModule) CoreVariantNeeded(ctx android.BaseModuleContext) bool {
	return false
}

func (s *snapshotModule) RamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return false
}

func (s *snapshotModule) VendorRamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return false
}

func (s *snapshotModule) DebugRamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return false
}

func (s *snapshotModule) RecoveryVariantNeeded(ctx android.BaseModuleContext) bool {
	return false
}

func (s *snapshotModule) ExtraImageVariations(ctx android.BaseModuleContext) []string {
	return []string{s.image.imageVariantName(ctx.DeviceConfig())}
}

func (s *snapshotModule) SetImageVariation(ctx android.BaseModuleContext, variation string, module android.Module) {
}

func (s *snapshotModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Nothing, the snapshot module is only used to forward dependency information in DepsMutator.
}

func getSnapshotNameSuffix(moduleSuffix, version, arch string) string {
	versionSuffix := version
	if arch != "" {
		versionSuffix += "." + arch
	}
	return moduleSuffix + versionSuffix
}

func (s *snapshotModule) DepsMutator(ctx android.BottomUpMutatorContext) {
	collectSnapshotMap := func(names []string, snapshotSuffix, moduleSuffix string) map[string]string {
		snapshotMap := make(map[string]string)
		for _, name := range names {
			snapshotMap[name] = name +
				getSnapshotNameSuffix(snapshotSuffix+moduleSuffix,
					s.baseSnapshot.Version(),
					ctx.DeviceConfig().Arches()[0].ArchType.String())
		}
		return snapshotMap
	}

	snapshotSuffix := s.image.moduleNameSuffix()
	headers := collectSnapshotMap(s.properties.Header_libs, snapshotSuffix, snapshotHeaderSuffix)
	binaries := collectSnapshotMap(s.properties.Binaries, snapshotSuffix, snapshotBinarySuffix)
	objects := collectSnapshotMap(s.properties.Objects, snapshotSuffix, snapshotObjectSuffix)
	staticLibs := collectSnapshotMap(s.properties.Static_libs, snapshotSuffix, SnapshotStaticSuffix)
	sharedLibs := collectSnapshotMap(s.properties.Shared_libs, snapshotSuffix, SnapshotSharedSuffix)
	rlibs := collectSnapshotMap(s.properties.Rlibs, snapshotSuffix, SnapshotRlibSuffix)
	dylibs := collectSnapshotMap(s.properties.Dylibs, snapshotSuffix, SnapshotDylibSuffix)
	vndkLibs := collectSnapshotMap(s.properties.Vndk_libs, "", vndkSuffix)
	for k, v := range vndkLibs {
		sharedLibs[k] = v
	}

	android.SetProvider(ctx, SnapshotInfoProvider, SnapshotInfo{
		HeaderLibs: headers,
		Binaries:   binaries,
		Objects:    objects,
		StaticLibs: staticLibs,
		SharedLibs: sharedLibs,
		Rlibs:      rlibs,
		Dylibs:     dylibs,
	})
}

type SnapshotInfo struct {
	HeaderLibs, Binaries, Objects, StaticLibs, SharedLibs, Rlibs, Dylibs map[string]string
}

var SnapshotInfoProvider = blueprint.NewMutatorProvider[SnapshotInfo]("deps")

var _ android.ImageInterface = (*snapshotModule)(nil)

func snapshotMakeVarsProvider(ctx android.MakeVarsContext) {
	snapshotSet := map[string]struct{}{}
	ctx.VisitAllModules(func(m android.Module) {
		if s, ok := m.(*snapshotModule); ok {
			if _, ok := snapshotSet[s.Name()]; ok {
				// arch variant generates duplicated modules
				// skip this as we only need to know the path of the module.
				return
			}
			snapshotSet[s.Name()] = struct{}{}
			imageNameVersion := strings.Split(s.image.imageVariantName(ctx.DeviceConfig()), ".")
			ctx.Strict(
				strings.Join([]string{strings.ToUpper(imageNameVersion[0]), s.baseSnapshot.Version(), "SNAPSHOT_DIR"}, "_"),
				ctx.ModuleDir(s))
		}
	})
}

func vendorSnapshotFactory() android.Module {
	return snapshotFactory(VendorSnapshotImageSingleton)
}

func recoverySnapshotFactory() android.Module {
	return snapshotFactory(RecoverySnapshotImageSingleton)
}

func snapshotFactory(image SnapshotImage) android.Module {
	snapshotModule := &snapshotModule{}
	snapshotModule.image = image
	snapshotModule.AddProperties(
		&snapshotModule.properties,
		&snapshotModule.baseSnapshot.baseProperties)
	android.InitAndroidArchModule(snapshotModule, android.DeviceSupported, android.MultilibBoth)
	return snapshotModule
}

type BaseSnapshotDecoratorProperties struct {
	// snapshot version.
	Version string

	// Target arch name of the snapshot (e.g. 'arm64' for variant 'aosp_arm64')
	Target_arch string

	// Suffix to be added to the module name when exporting to Android.mk, e.g. ".vendor".
	Androidmk_suffix string `blueprint:"mutated"`

	// Suffix to be added to the module name, e.g., vendor_shared,
	// recovery_shared, etc.
	ModuleSuffix string `blueprint:"mutated"`
}

// BaseSnapshotDecorator provides common basic functions for all snapshot modules, such as snapshot
// version, snapshot arch, etc. It also adds a special suffix to Soong module name, so it doesn't
// collide with source modules. e.g. the following example module,
//
//	vendor_snapshot_static {
//	    name: "libbase",
//	    arch: "arm64",
//	    version: 30,
//	    ...
//	}
//
// will be seen as "libbase.vendor_static.30.arm64" by Soong.
type BaseSnapshotDecorator struct {
	baseProperties BaseSnapshotDecoratorProperties
	Image          SnapshotImage
}

func (p *BaseSnapshotDecorator) Name(name string) string {
	return name + p.NameSuffix()
}

func (p *BaseSnapshotDecorator) NameSuffix() string {
	return getSnapshotNameSuffix(p.moduleSuffix(), p.Version(), p.Arch())
}

func (p *BaseSnapshotDecorator) Version() string {
	return p.baseProperties.Version
}

func (p *BaseSnapshotDecorator) Arch() string {
	return p.baseProperties.Target_arch
}

func (p *BaseSnapshotDecorator) moduleSuffix() string {
	return p.baseProperties.ModuleSuffix
}

func (p *BaseSnapshotDecorator) IsSnapshotPrebuilt() bool {
	return true
}

func (p *BaseSnapshotDecorator) SnapshotAndroidMkSuffix() string {
	return p.baseProperties.Androidmk_suffix
}

func (p *BaseSnapshotDecorator) SetSnapshotAndroidMkSuffix(ctx android.ModuleContext, variant string) {
	// If there are any 2 or more variations among {core, product, vendor, recovery}
	// we have to add the androidmk suffix to avoid duplicate modules with the same
	// name.
	variations := append(ctx.Target().Variations(), blueprint.Variation{
		Mutator:   "image",
		Variation: android.CoreVariation})

	if ctx.OtherModuleFarDependencyVariantExists(variations, ctx.Module().(LinkableInterface).BaseModuleName()) {
		p.baseProperties.Androidmk_suffix = p.Image.moduleNameSuffix()
		return
	}

	variations = append(ctx.Target().Variations(), blueprint.Variation{
		Mutator:   "image",
		Variation: ProductVariationPrefix + ctx.DeviceConfig().PlatformVndkVersion()})

	if ctx.OtherModuleFarDependencyVariantExists(variations, ctx.Module().(LinkableInterface).BaseModuleName()) {
		p.baseProperties.Androidmk_suffix = p.Image.moduleNameSuffix()
		return
	}

	images := []SnapshotImage{VendorSnapshotImageSingleton, RecoverySnapshotImageSingleton}

	for _, image := range images {
		if p.Image == image {
			continue
		}
		variations = append(ctx.Target().Variations(), blueprint.Variation{
			Mutator:   "image",
			Variation: image.imageVariantName(ctx.DeviceConfig())})

		if ctx.OtherModuleFarDependencyVariantExists(variations,
			ctx.Module().(LinkableInterface).BaseModuleName()+
				getSnapshotNameSuffix(
					image.moduleNameSuffix()+variant,
					p.Version(),
					ctx.DeviceConfig().Arches()[0].ArchType.String())) {
			p.baseProperties.Androidmk_suffix = p.Image.moduleNameSuffix()
			return
		}
	}

	p.baseProperties.Androidmk_suffix = ""
}

// Call this with a module suffix after creating a snapshot module, such as
// vendorSnapshotSharedSuffix, recoverySnapshotBinarySuffix, etc.
func (p *BaseSnapshotDecorator) Init(m LinkableInterface, image SnapshotImage, moduleSuffix string) {
	p.Image = image
	p.baseProperties.ModuleSuffix = image.moduleNameSuffix() + moduleSuffix
	m.AddProperties(&p.baseProperties)
	android.AddLoadHook(m, func(ctx android.LoadHookContext) {
		vendorSnapshotLoadHook(ctx, p)
	})
}

// vendorSnapshotLoadHook disables snapshots if it's not BOARD_VNDK_VERSION.
// As vendor snapshot is only for vendor, such modules won't be used at all.
func vendorSnapshotLoadHook(ctx android.LoadHookContext, p *BaseSnapshotDecorator) {
	if p.Version() != ctx.DeviceConfig().VndkVersion() {
		ctx.Module().Disable()
		return
	}
}

// Module definitions for snapshots of libraries (shared, static, header).
//
// Modules (vendor|recovery)_snapshot_(shared|static|header) are defined here. Shared libraries and
// static libraries have their prebuilt library files (.so for shared, .a for static) as their src,
// which can be installed or linked against. Also they export flags needed when linked, such as
// include directories, c flags, sanitize dependency information, etc.
//
// These modules are auto-generated by development/vendor_snapshot/update.py.
type SnapshotLibraryProperties struct {
	// Prebuilt file for each arch.
	Src *string `android:"arch_variant"`

	// list of directories that will be added to the include path (using -I).
	Export_include_dirs []string `android:"arch_variant"`

	// list of directories that will be added to the system path (using -isystem).
	Export_system_include_dirs []string `android:"arch_variant"`

	// list of flags that will be used for any module that links against this module.
	Export_flags []string `android:"arch_variant"`

	// Whether this prebuilt needs to depend on sanitize ubsan runtime or not.
	Sanitize_ubsan_dep *bool `android:"arch_variant"`

	// Whether this prebuilt needs to depend on sanitize minimal runtime or not.
	Sanitize_minimal_dep *bool `android:"arch_variant"`
}

type SnapshotSanitizer interface {
	IsSanitizerAvailable(t SanitizerType) bool
	SetSanitizerVariation(t SanitizerType, enabled bool)
	IsSanitizerEnabled(t SanitizerType) bool
	IsUnsanitizedVariant() bool
}

type snapshotLibraryDecorator struct {
	BaseSnapshotDecorator
	*libraryDecorator
	properties          SnapshotLibraryProperties
	sanitizerProperties struct {
		SanitizerVariation SanitizerType `blueprint:"mutated"`

		// Library flags for cfi variant.
		Cfi SnapshotLibraryProperties `android:"arch_variant"`

		// Library flags for hwasan variant.
		Hwasan SnapshotLibraryProperties `android:"arch_variant"`
	}
}

func (p *snapshotLibraryDecorator) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	p.libraryDecorator.libName = strings.TrimSuffix(ctx.ModuleName(), p.NameSuffix())
	return p.libraryDecorator.linkerFlags(ctx, flags)
}

func (p *snapshotLibraryDecorator) MatchesWithDevice(config android.DeviceConfig) bool {
	arches := config.Arches()
	if len(arches) == 0 || arches[0].ArchType.String() != p.Arch() {
		return false
	}
	if !p.header() && p.properties.Src == nil {
		return false
	}
	return true
}

// cc modules' link functions are to link compiled objects into final binaries.
// As snapshots are prebuilts, this just returns the prebuilt binary after doing things which are
// done by normal library decorator, e.g. exporting flags.
func (p *snapshotLibraryDecorator) link(ctx ModuleContext, flags Flags, deps PathDeps, objs Objects) android.Path {
	var variant string
	if p.shared() {
		variant = SnapshotSharedSuffix
	} else if p.static() {
		variant = SnapshotStaticSuffix
	} else {
		variant = snapshotHeaderSuffix
	}

	p.SetSnapshotAndroidMkSuffix(ctx, variant)

	if p.header() {
		return p.libraryDecorator.link(ctx, flags, deps, objs)
	}

	if p.IsSanitizerEnabled(cfi) {
		p.properties = p.sanitizerProperties.Cfi
	} else if p.IsSanitizerEnabled(Hwasan) {
		p.properties = p.sanitizerProperties.Hwasan
	}

	if !p.MatchesWithDevice(ctx.DeviceConfig()) {
		return nil
	}

	// Flags specified directly to this module.
	p.libraryDecorator.reexportDirs(android.PathsForModuleSrc(ctx, p.properties.Export_include_dirs)...)
	p.libraryDecorator.reexportSystemDirs(android.PathsForModuleSrc(ctx, p.properties.Export_system_include_dirs)...)
	p.libraryDecorator.reexportFlags(p.properties.Export_flags...)

	// Flags reexported from dependencies. (e.g. vndk_prebuilt_shared)
	p.libraryDecorator.reexportDirs(deps.ReexportedDirs...)
	p.libraryDecorator.reexportSystemDirs(deps.ReexportedSystemDirs...)
	p.libraryDecorator.reexportFlags(deps.ReexportedFlags...)
	p.libraryDecorator.reexportDeps(deps.ReexportedDeps...)
	p.libraryDecorator.addExportedGeneratedHeaders(deps.ReexportedGeneratedHeaders...)

	in := android.PathForModuleSrc(ctx, *p.properties.Src)
	p.unstrippedOutputFile = in

	if p.shared() {
		libName := in.Base()

		// Optimize out relinking against shared libraries whose interface hasn't changed by
		// depending on a table of contents file instead of the library itself.
		tocFile := android.PathForModuleOut(ctx, libName+".toc")
		p.tocFile = android.OptionalPathForPath(tocFile)
		TransformSharedObjectToToc(ctx, in, tocFile)

		android.SetProvider(ctx, SharedLibraryInfoProvider, SharedLibraryInfo{
			SharedLibrary: in,
			Target:        ctx.Target(),

			TableOfContents: p.tocFile,
		})
	}

	if p.static() {
		depSet := android.NewDepSetBuilder[android.Path](android.TOPOLOGICAL).Direct(in).Build()
		android.SetProvider(ctx, StaticLibraryInfoProvider, StaticLibraryInfo{
			StaticLibrary: in,

			TransitiveStaticLibrariesForOrdering: depSet,
		})
	}

	p.libraryDecorator.flagExporter.setProvider(ctx)

	return in
}

func (p *snapshotLibraryDecorator) install(ctx ModuleContext, file android.Path) {
	if p.MatchesWithDevice(ctx.DeviceConfig()) && p.shared() {
		p.baseInstaller.install(ctx, file)
	}
}

func (p *snapshotLibraryDecorator) nativeCoverage() bool {
	return false
}

var _ SnapshotSanitizer = (*snapshotLibraryDecorator)(nil)

func (p *snapshotLibraryDecorator) IsSanitizerAvailable(t SanitizerType) bool {
	switch t {
	case cfi:
		return p.sanitizerProperties.Cfi.Src != nil
	case Hwasan:
		return p.sanitizerProperties.Hwasan.Src != nil
	default:
		return false
	}
}

func (p *snapshotLibraryDecorator) SetSanitizerVariation(t SanitizerType, enabled bool) {
	if !enabled || p.IsSanitizerEnabled(t) {
		return
	}
	if !p.IsUnsanitizedVariant() {
		panic(fmt.Errorf("snapshot Sanitizer must be one of Cfi or Hwasan but not both"))
	}
	p.sanitizerProperties.SanitizerVariation = t
}

func (p *snapshotLibraryDecorator) IsSanitizerEnabled(t SanitizerType) bool {
	return p.sanitizerProperties.SanitizerVariation == t
}

func (p *snapshotLibraryDecorator) IsUnsanitizedVariant() bool {
	return !p.IsSanitizerEnabled(Asan) &&
		!p.IsSanitizerEnabled(Hwasan)
}

func snapshotLibraryFactory(image SnapshotImage, moduleSuffix string) (*Module, *snapshotLibraryDecorator) {
	module, library := NewLibrary(android.DeviceSupported)

	module.stl = nil
	module.sanitize = nil
	library.disableStripping()

	prebuilt := &snapshotLibraryDecorator{
		libraryDecorator: library,
	}

	prebuilt.baseLinker.Properties.No_libcrt = BoolPtr(true)
	prebuilt.baseLinker.Properties.Nocrt = BoolPtr(true)

	// Prevent default system libs (libc, libm, and libdl) from being linked
	if prebuilt.baseLinker.Properties.System_shared_libs == nil {
		prebuilt.baseLinker.Properties.System_shared_libs = []string{}
	}

	module.compiler = nil
	module.linker = prebuilt
	module.installer = prebuilt

	prebuilt.Init(module, image, moduleSuffix)
	module.AddProperties(
		&prebuilt.properties,
		&prebuilt.sanitizerProperties,
	)

	return module, prebuilt
}

// vendor_snapshot_shared is a special prebuilt shared library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of vendor snapshot, vendor_snapshot_shared
// overrides the vendor variant of the cc shared library with the same name, if BOARD_VNDK_VERSION
// is set.
func VendorSnapshotSharedFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(VendorSnapshotImageSingleton, SnapshotSharedSuffix)
	prebuilt.libraryDecorator.BuildOnlyShared()
	return module.Init()
}

// recovery_snapshot_shared is a special prebuilt shared library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of recovery snapshot, recovery_snapshot_shared
// overrides the recovery variant of the cc shared library with the same name, if BOARD_VNDK_VERSION
// is set.
func RecoverySnapshotSharedFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(RecoverySnapshotImageSingleton, SnapshotSharedSuffix)
	prebuilt.libraryDecorator.BuildOnlyShared()
	return module.Init()
}

// vendor_snapshot_static is a special prebuilt static library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of vendor snapshot, vendor_snapshot_static
// overrides the vendor variant of the cc static library with the same name, if BOARD_VNDK_VERSION
// is set.
func VendorSnapshotStaticFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(VendorSnapshotImageSingleton, SnapshotStaticSuffix)
	prebuilt.libraryDecorator.BuildOnlyStatic()
	return module.Init()
}

// recovery_snapshot_static is a special prebuilt static library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of recovery snapshot, recovery_snapshot_static
// overrides the recovery variant of the cc static library with the same name, if BOARD_VNDK_VERSION
// is set.
func RecoverySnapshotStaticFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(RecoverySnapshotImageSingleton, SnapshotStaticSuffix)
	prebuilt.libraryDecorator.BuildOnlyStatic()
	return module.Init()
}

// vendor_snapshot_header is a special header library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of vendor snapshot, vendor_snapshot_header
// overrides the vendor variant of the cc header library with the same name, if BOARD_VNDK_VERSION
// is set.
func VendorSnapshotHeaderFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(VendorSnapshotImageSingleton, snapshotHeaderSuffix)
	prebuilt.libraryDecorator.HeaderOnly()
	return module.Init()
}

// recovery_snapshot_header is a special header library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of recovery snapshot, recovery_snapshot_header
// overrides the recovery variant of the cc header library with the same name, if BOARD_VNDK_VERSION
// is set.
func RecoverySnapshotHeaderFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(RecoverySnapshotImageSingleton, snapshotHeaderSuffix)
	prebuilt.libraryDecorator.HeaderOnly()
	return module.Init()
}

// Module definitions for snapshots of executable binaries.
//
// Modules (vendor|recovery)_snapshot_binary are defined here. They have their prebuilt executable
// binaries (e.g. toybox, sh) as their src, which can be installed.
//
// These modules are auto-generated by development/vendor_snapshot/update.py.
type snapshotBinaryProperties struct {
	// Prebuilt file for each arch.
	Src *string `android:"arch_variant"`
}

type snapshotBinaryDecorator struct {
	BaseSnapshotDecorator
	*binaryDecorator
	properties snapshotBinaryProperties
}

func (p *snapshotBinaryDecorator) MatchesWithDevice(config android.DeviceConfig) bool {
	if config.DeviceArch() != p.Arch() {
		return false
	}
	if p.properties.Src == nil {
		return false
	}
	return true
}

// cc modules' link functions are to link compiled objects into final binaries.
// As snapshots are prebuilts, this just returns the prebuilt binary
func (p *snapshotBinaryDecorator) link(ctx ModuleContext, flags Flags, deps PathDeps, objs Objects) android.Path {
	p.SetSnapshotAndroidMkSuffix(ctx, snapshotBinarySuffix)

	if !p.MatchesWithDevice(ctx.DeviceConfig()) {
		return nil
	}

	in := android.PathForModuleSrc(ctx, *p.properties.Src)
	p.unstrippedOutputFile = in
	binName := in.Base()

	// use cpExecutable to make it executable
	outputFile := android.PathForModuleOut(ctx, binName)
	ctx.Build(pctx, android.BuildParams{
		Rule:        android.CpExecutable,
		Description: "prebuilt",
		Output:      outputFile,
		Input:       in,
	})

	// binary snapshots need symlinking
	p.setSymlinkList(ctx)

	return outputFile
}

func (p *snapshotBinaryDecorator) nativeCoverage() bool {
	return false
}

// vendor_snapshot_binary is a special prebuilt executable binary which is auto-generated by
// development/vendor_snapshot/update.py. As a part of vendor snapshot, vendor_snapshot_binary
// overrides the vendor variant of the cc binary with the same name, if BOARD_VNDK_VERSION is set.
func VendorSnapshotBinaryFactory() android.Module {
	return snapshotBinaryFactory(VendorSnapshotImageSingleton, snapshotBinarySuffix)
}

// recovery_snapshot_binary is a special prebuilt executable binary which is auto-generated by
// development/vendor_snapshot/update.py. As a part of recovery snapshot, recovery_snapshot_binary
// overrides the recovery variant of the cc binary with the same name, if BOARD_VNDK_VERSION is set.
func RecoverySnapshotBinaryFactory() android.Module {
	return snapshotBinaryFactory(RecoverySnapshotImageSingleton, snapshotBinarySuffix)
}

func snapshotBinaryFactory(image SnapshotImage, moduleSuffix string) android.Module {
	module, binary := NewBinary(android.DeviceSupported)
	binary.baseLinker.Properties.No_libcrt = BoolPtr(true)
	binary.baseLinker.Properties.Nocrt = BoolPtr(true)

	// Prevent default system libs (libc, libm, and libdl) from being linked
	if binary.baseLinker.Properties.System_shared_libs == nil {
		binary.baseLinker.Properties.System_shared_libs = []string{}
	}

	prebuilt := &snapshotBinaryDecorator{
		binaryDecorator: binary,
	}

	module.compiler = nil
	module.sanitize = nil
	module.stl = nil
	module.linker = prebuilt

	prebuilt.Init(module, image, moduleSuffix)
	module.AddProperties(&prebuilt.properties)
	return module.Init()
}

// Module definitions for snapshots of object files (*.o).
//
// Modules (vendor|recovery)_snapshot_object are defined here. They have their prebuilt object
// files (*.o) as their src.
//
// These modules are auto-generated by development/vendor_snapshot/update.py.
type vendorSnapshotObjectProperties struct {
	// Prebuilt file for each arch.
	Src *string `android:"arch_variant"`
}

type snapshotObjectLinker struct {
	BaseSnapshotDecorator
	objectLinker
	properties vendorSnapshotObjectProperties
}

func (p *snapshotObjectLinker) MatchesWithDevice(config android.DeviceConfig) bool {
	if config.DeviceArch() != p.Arch() {
		return false
	}
	if p.properties.Src == nil {
		return false
	}
	return true
}

// cc modules' link functions are to link compiled objects into final binaries.
// As snapshots are prebuilts, this just returns the prebuilt binary
func (p *snapshotObjectLinker) link(ctx ModuleContext, flags Flags, deps PathDeps, objs Objects) android.Path {
	p.SetSnapshotAndroidMkSuffix(ctx, snapshotObjectSuffix)

	if !p.MatchesWithDevice(ctx.DeviceConfig()) {
		return nil
	}

	return android.PathForModuleSrc(ctx, *p.properties.Src)
}

func (p *snapshotObjectLinker) nativeCoverage() bool {
	return false
}

// vendor_snapshot_object is a special prebuilt compiled object file which is auto-generated by
// development/vendor_snapshot/update.py. As a part of vendor snapshot, vendor_snapshot_object
// overrides the vendor variant of the cc object with the same name, if BOARD_VNDK_VERSION is set.
func VendorSnapshotObjectFactory() android.Module {
	module := newObject(android.DeviceSupported)

	prebuilt := &snapshotObjectLinker{
		objectLinker: objectLinker{
			baseLinker: NewBaseLinker(nil),
		},
	}
	module.linker = prebuilt

	prebuilt.Init(module, VendorSnapshotImageSingleton, snapshotObjectSuffix)
	module.AddProperties(&prebuilt.properties)

	// vendor_snapshot_object module does not provide sanitizer variants
	module.sanitize.Properties.Sanitize.Never = BoolPtr(true)

	return module.Init()
}

// recovery_snapshot_object is a special prebuilt compiled object file which is auto-generated by
// development/vendor_snapshot/update.py. As a part of recovery snapshot, recovery_snapshot_object
// overrides the recovery variant of the cc object with the same name, if BOARD_VNDK_VERSION is set.
func RecoverySnapshotObjectFactory() android.Module {
	module := newObject(android.DeviceSupported)

	prebuilt := &snapshotObjectLinker{
		objectLinker: objectLinker{
			baseLinker: NewBaseLinker(nil),
		},
	}
	module.linker = prebuilt

	prebuilt.Init(module, RecoverySnapshotImageSingleton, snapshotObjectSuffix)
	module.AddProperties(&prebuilt.properties)
	return module.Init()
}

type SnapshotInterface interface {
	MatchesWithDevice(config android.DeviceConfig) bool
	IsSnapshotPrebuilt() bool
	Version() string
	SnapshotAndroidMkSuffix() string
}

var _ SnapshotInterface = (*vndkPrebuiltLibraryDecorator)(nil)
var _ SnapshotInterface = (*snapshotLibraryDecorator)(nil)
var _ SnapshotInterface = (*snapshotBinaryDecorator)(nil)
var _ SnapshotInterface = (*snapshotObjectLinker)(nil)
