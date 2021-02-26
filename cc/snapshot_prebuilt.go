// Copyright 2020 The Android Open Source Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
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
	"strings"

	"android/soong/android"

	"github.com/google/blueprint"
)

// Defines the specifics of different images to which the snapshot process is applicable, e.g.,
// vendor, recovery, ramdisk.
type snapshotImage interface {
	// Returns true if a snapshot should be generated for this image.
	shouldGenerateSnapshot(ctx android.SingletonContext) bool

	// Function that returns true if the module is included in this image.
	// Using a function return instead of a value to prevent early
	// evalution of a function that may be not be defined.
	inImage(m *Module) func() bool

	// Returns true if the module is private and must not be included in the
	// snapshot. For example VNDK-private modules must return true for the
	// vendor snapshots. But false for the recovery snapshots.
	private(m *Module) bool

	// Returns true if a dir under source tree is an SoC-owned proprietary
	// directory, such as device/, vendor/, etc.
	//
	// For a given snapshot (e.g., vendor, recovery, etc.) if
	// isProprietaryPath(dir) returns true, then the module in dir will be
	// built from sources.
	isProprietaryPath(dir string) bool

	// Whether to include VNDK in the snapshot for this image.
	includeVndk() bool

	// Whether a given module has been explicitly excluded from the
	// snapshot, e.g., using the exclude_from_vendor_snapshot or
	// exclude_from_recovery_snapshot properties.
	excludeFromSnapshot(m *Module) bool

	// Returns true if the build is using a snapshot for this image.
	isUsingSnapshot(cfg android.DeviceConfig) bool

	// Returns a version of which the snapshot should be used in this target.
	// This will only be meaningful when isUsingSnapshot is true.
	targetSnapshotVersion(cfg android.DeviceConfig) string

	// Whether to exclude a given module from the directed snapshot or not.
	// If the makefile variable DIRECTED_{IMAGE}_SNAPSHOT is true, directed snapshot is turned on,
	// and only modules listed in {IMAGE}_SNAPSHOT_MODULES will be captured.
	excludeFromDirectedSnapshot(cfg android.DeviceConfig, name string) bool

	// The image variant name for this snapshot image.
	// For example, recovery snapshot image will return "recovery", and vendor snapshot image will
	// return "vendor." + version.
	imageVariantName(cfg android.DeviceConfig) string

	// The variant suffix for snapshot modules. For example, vendor snapshot modules will have
	// ".vendor" as their suffix.
	moduleNameSuffix() string
}

type vendorSnapshotImage struct{}
type recoverySnapshotImage struct{}

func (vendorSnapshotImage) init(ctx android.RegistrationContext) {
	ctx.RegisterSingletonType("vendor-snapshot", VendorSnapshotSingleton)
	ctx.RegisterModuleType("vendor_snapshot", vendorSnapshotFactory)
	ctx.RegisterModuleType("vendor_snapshot_shared", VendorSnapshotSharedFactory)
	ctx.RegisterModuleType("vendor_snapshot_static", VendorSnapshotStaticFactory)
	ctx.RegisterModuleType("vendor_snapshot_header", VendorSnapshotHeaderFactory)
	ctx.RegisterModuleType("vendor_snapshot_binary", VendorSnapshotBinaryFactory)
	ctx.RegisterModuleType("vendor_snapshot_object", VendorSnapshotObjectFactory)

	ctx.RegisterSingletonType("vendor-fake-snapshot", VendorFakeSnapshotSingleton)
}

func (vendorSnapshotImage) shouldGenerateSnapshot(ctx android.SingletonContext) bool {
	// BOARD_VNDK_VERSION must be set to 'current' in order to generate a snapshot.
	return ctx.DeviceConfig().VndkVersion() == "current"
}

func (vendorSnapshotImage) inImage(m *Module) func() bool {
	return m.InVendor
}

func (vendorSnapshotImage) private(m *Module) bool {
	return m.IsVndkPrivate()
}

func (vendorSnapshotImage) isProprietaryPath(dir string) bool {
	return isVendorProprietaryPath(dir)
}

// vendor snapshot includes static/header libraries with vndk: {enabled: true}.
func (vendorSnapshotImage) includeVndk() bool {
	return true
}

func (vendorSnapshotImage) excludeFromSnapshot(m *Module) bool {
	return m.ExcludeFromVendorSnapshot()
}

func (vendorSnapshotImage) isUsingSnapshot(cfg android.DeviceConfig) bool {
	vndkVersion := cfg.VndkVersion()
	return vndkVersion != "current" && vndkVersion != ""
}

func (vendorSnapshotImage) targetSnapshotVersion(cfg android.DeviceConfig) string {
	return cfg.VndkVersion()
}

// returns true iff a given module SHOULD BE EXCLUDED, false if included
func (vendorSnapshotImage) excludeFromDirectedSnapshot(cfg android.DeviceConfig, name string) bool {
	// If we're using full snapshot, not directed snapshot, capture every module
	if !cfg.DirectedVendorSnapshot() {
		return false
	}
	// Else, checks if name is in VENDOR_SNAPSHOT_MODULES.
	return !cfg.VendorSnapshotModules()[name]
}

func (vendorSnapshotImage) imageVariantName(cfg android.DeviceConfig) string {
	return VendorVariationPrefix + cfg.VndkVersion()
}

func (vendorSnapshotImage) moduleNameSuffix() string {
	return VendorSuffix
}

func (recoverySnapshotImage) init(ctx android.RegistrationContext) {
	ctx.RegisterSingletonType("recovery-snapshot", RecoverySnapshotSingleton)
	ctx.RegisterModuleType("recovery_snapshot", recoverySnapshotFactory)
	ctx.RegisterModuleType("recovery_snapshot_shared", RecoverySnapshotSharedFactory)
	ctx.RegisterModuleType("recovery_snapshot_static", RecoverySnapshotStaticFactory)
	ctx.RegisterModuleType("recovery_snapshot_header", RecoverySnapshotHeaderFactory)
	ctx.RegisterModuleType("recovery_snapshot_binary", RecoverySnapshotBinaryFactory)
	ctx.RegisterModuleType("recovery_snapshot_object", RecoverySnapshotObjectFactory)
}

func (recoverySnapshotImage) shouldGenerateSnapshot(ctx android.SingletonContext) bool {
	// RECOVERY_SNAPSHOT_VERSION must be set to 'current' in order to generate a
	// snapshot.
	return ctx.DeviceConfig().RecoverySnapshotVersion() == "current"
}

func (recoverySnapshotImage) inImage(m *Module) func() bool {
	return m.InRecovery
}

// recovery snapshot does not have private libraries.
func (recoverySnapshotImage) private(m *Module) bool {
	return false
}

func (recoverySnapshotImage) isProprietaryPath(dir string) bool {
	return isRecoveryProprietaryPath(dir)
}

// recovery snapshot does NOT treat vndk specially.
func (recoverySnapshotImage) includeVndk() bool {
	return false
}

func (recoverySnapshotImage) excludeFromSnapshot(m *Module) bool {
	return m.ExcludeFromRecoverySnapshot()
}

func (recoverySnapshotImage) isUsingSnapshot(cfg android.DeviceConfig) bool {
	recoverySnapshotVersion := cfg.RecoverySnapshotVersion()
	return recoverySnapshotVersion != "current" && recoverySnapshotVersion != ""
}

func (recoverySnapshotImage) targetSnapshotVersion(cfg android.DeviceConfig) string {
	return cfg.RecoverySnapshotVersion()
}

func (recoverySnapshotImage) excludeFromDirectedSnapshot(cfg android.DeviceConfig, name string) bool {
	// If we're using full snapshot, not directed snapshot, capture every module
	if !cfg.DirectedRecoverySnapshot() {
		return false
	}
	// Else, checks if name is in RECOVERY_SNAPSHOT_MODULES.
	return !cfg.RecoverySnapshotModules()[name]
}

func (recoverySnapshotImage) imageVariantName(cfg android.DeviceConfig) string {
	return android.RecoveryVariation
}

func (recoverySnapshotImage) moduleNameSuffix() string {
	return recoverySuffix
}

var vendorSnapshotImageSingleton vendorSnapshotImage
var recoverySnapshotImageSingleton recoverySnapshotImage

func init() {
	vendorSnapshotImageSingleton.init(android.InitRegistrationContext)
	recoverySnapshotImageSingleton.init(android.InitRegistrationContext)
}

const (
	snapshotHeaderSuffix = "_header."
	snapshotSharedSuffix = "_shared."
	snapshotStaticSuffix = "_static."
	snapshotBinarySuffix = "_binary."
	snapshotObjectSuffix = "_object."
)

type SnapshotProperties struct {
	Header_libs []string `android:"arch_variant"`
	Static_libs []string `android:"arch_variant"`
	Shared_libs []string `android:"arch_variant"`
	Vndk_libs   []string `android:"arch_variant"`
	Binaries    []string `android:"arch_variant"`
	Objects     []string `android:"arch_variant"`
}

type snapshot struct {
	android.ModuleBase

	properties SnapshotProperties

	baseSnapshot baseSnapshotDecorator

	image snapshotImage
}

func (s *snapshot) ImageMutatorBegin(ctx android.BaseModuleContext) {
	cfg := ctx.DeviceConfig()
	if !s.image.isUsingSnapshot(cfg) || s.image.targetSnapshotVersion(cfg) != s.baseSnapshot.version() {
		s.Disable()
	}
}

func (s *snapshot) CoreVariantNeeded(ctx android.BaseModuleContext) bool {
	return false
}

func (s *snapshot) RamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return false
}

func (s *snapshot) VendorRamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return false
}

func (s *snapshot) RecoveryVariantNeeded(ctx android.BaseModuleContext) bool {
	return false
}

func (s *snapshot) ExtraImageVariations(ctx android.BaseModuleContext) []string {
	return []string{s.image.imageVariantName(ctx.DeviceConfig())}
}

func (s *snapshot) SetImageVariation(ctx android.BaseModuleContext, variation string, module android.Module) {
}

func (s *snapshot) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Nothing, the snapshot module is only used to forward dependency information in DepsMutator.
}

func getSnapshotNameSuffix(moduleSuffix, version, arch string) string {
	versionSuffix := version
	if arch != "" {
		versionSuffix += "." + arch
	}
	return moduleSuffix + versionSuffix
}

func (s *snapshot) DepsMutator(ctx android.BottomUpMutatorContext) {
	collectSnapshotMap := func(names []string, snapshotSuffix, moduleSuffix string) map[string]string {
		snapshotMap := make(map[string]string)
		for _, name := range names {
			snapshotMap[name] = name +
				getSnapshotNameSuffix(snapshotSuffix+moduleSuffix,
					s.baseSnapshot.version(), ctx.Arch().ArchType.Name)
		}
		return snapshotMap
	}

	snapshotSuffix := s.image.moduleNameSuffix()
	headers := collectSnapshotMap(s.properties.Header_libs, snapshotSuffix, snapshotHeaderSuffix)
	binaries := collectSnapshotMap(s.properties.Binaries, snapshotSuffix, snapshotBinarySuffix)
	objects := collectSnapshotMap(s.properties.Objects, snapshotSuffix, snapshotObjectSuffix)
	staticLibs := collectSnapshotMap(s.properties.Static_libs, snapshotSuffix, snapshotStaticSuffix)
	sharedLibs := collectSnapshotMap(s.properties.Shared_libs, snapshotSuffix, snapshotSharedSuffix)
	vndkLibs := collectSnapshotMap(s.properties.Vndk_libs, "", vndkSuffix)
	for k, v := range vndkLibs {
		sharedLibs[k] = v
	}

	ctx.SetProvider(SnapshotInfoProvider, SnapshotInfo{
		HeaderLibs: headers,
		Binaries:   binaries,
		Objects:    objects,
		StaticLibs: staticLibs,
		SharedLibs: sharedLibs,
	})
}

type SnapshotInfo struct {
	HeaderLibs, Binaries, Objects, StaticLibs, SharedLibs map[string]string
}

var SnapshotInfoProvider = blueprint.NewMutatorProvider(SnapshotInfo{}, "deps")

var _ android.ImageInterface = (*snapshot)(nil)

func vendorSnapshotFactory() android.Module {
	return snapshotFactory(vendorSnapshotImageSingleton)
}

func recoverySnapshotFactory() android.Module {
	return snapshotFactory(recoverySnapshotImageSingleton)
}

func snapshotFactory(image snapshotImage) android.Module {
	snapshot := &snapshot{}
	snapshot.image = image
	snapshot.AddProperties(
		&snapshot.properties,
		&snapshot.baseSnapshot.baseProperties)
	android.InitAndroidArchModule(snapshot, android.DeviceSupported, android.MultilibBoth)
	return snapshot
}

type baseSnapshotDecoratorProperties struct {
	// snapshot version.
	Version string

	// Target arch name of the snapshot (e.g. 'arm64' for variant 'aosp_arm64')
	Target_arch string

	// Suffix to be added to the module name when exporting to Android.mk, e.g. ".vendor".
	Androidmk_suffix string

	// Suffix to be added to the module name, e.g., vendor_shared,
	// recovery_shared, etc.
	ModuleSuffix string `blueprint:"mutated"`
}

// baseSnapshotDecorator provides common basic functions for all snapshot modules, such as snapshot
// version, snapshot arch, etc. It also adds a special suffix to Soong module name, so it doesn't
// collide with source modules. e.g. the following example module,
//
// vendor_snapshot_static {
//     name: "libbase",
//     arch: "arm64",
//     version: 30,
//     ...
// }
//
// will be seen as "libbase.vendor_static.30.arm64" by Soong.
type baseSnapshotDecorator struct {
	baseProperties baseSnapshotDecoratorProperties
}

func (p *baseSnapshotDecorator) Name(name string) string {
	return name + p.NameSuffix()
}

func (p *baseSnapshotDecorator) NameSuffix() string {
	return getSnapshotNameSuffix(p.moduleSuffix(), p.version(), p.arch())
}

func (p *baseSnapshotDecorator) version() string {
	return p.baseProperties.Version
}

func (p *baseSnapshotDecorator) arch() string {
	return p.baseProperties.Target_arch
}

func (p *baseSnapshotDecorator) moduleSuffix() string {
	return p.baseProperties.ModuleSuffix
}

func (p *baseSnapshotDecorator) isSnapshotPrebuilt() bool {
	return true
}

func (p *baseSnapshotDecorator) snapshotAndroidMkSuffix() string {
	return p.baseProperties.Androidmk_suffix
}

// Call this with a module suffix after creating a snapshot module, such as
// vendorSnapshotSharedSuffix, recoverySnapshotBinarySuffix, etc.
func (p *baseSnapshotDecorator) init(m *Module, snapshotSuffix, moduleSuffix string) {
	p.baseProperties.ModuleSuffix = snapshotSuffix + moduleSuffix
	m.AddProperties(&p.baseProperties)
	android.AddLoadHook(m, func(ctx android.LoadHookContext) {
		vendorSnapshotLoadHook(ctx, p)
	})
}

// vendorSnapshotLoadHook disables snapshots if it's not BOARD_VNDK_VERSION.
// As vendor snapshot is only for vendor, such modules won't be used at all.
func vendorSnapshotLoadHook(ctx android.LoadHookContext, p *baseSnapshotDecorator) {
	if p.version() != ctx.DeviceConfig().VndkVersion() {
		ctx.Module().Disable()
		return
	}
}

//
// Module definitions for snapshots of libraries (shared, static, header).
//
// Modules (vendor|recovery)_snapshot_(shared|static|header) are defined here. Shared libraries and
// static libraries have their prebuilt library files (.so for shared, .a for static) as their src,
// which can be installed or linked against. Also they export flags needed when linked, such as
// include directories, c flags, sanitize dependency information, etc.
//
// These modules are auto-generated by development/vendor_snapshot/update.py.
type snapshotLibraryProperties struct {
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

type snapshotSanitizer interface {
	isSanitizerEnabled(t SanitizerType) bool
	setSanitizerVariation(t SanitizerType, enabled bool)
}

type snapshotLibraryDecorator struct {
	baseSnapshotDecorator
	*libraryDecorator
	properties          snapshotLibraryProperties
	sanitizerProperties struct {
		CfiEnabled bool `blueprint:"mutated"`

		// Library flags for cfi variant.
		Cfi snapshotLibraryProperties `android:"arch_variant"`
	}
}

func (p *snapshotLibraryDecorator) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	p.libraryDecorator.libName = strings.TrimSuffix(ctx.ModuleName(), p.NameSuffix())
	return p.libraryDecorator.linkerFlags(ctx, flags)
}

func (p *snapshotLibraryDecorator) matchesWithDevice(config android.DeviceConfig) bool {
	arches := config.Arches()
	if len(arches) == 0 || arches[0].ArchType.String() != p.arch() {
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
	if p.header() {
		return p.libraryDecorator.link(ctx, flags, deps, objs)
	}

	if p.sanitizerProperties.CfiEnabled {
		p.properties = p.sanitizerProperties.Cfi
	}

	if !p.matchesWithDevice(ctx.DeviceConfig()) {
		return nil
	}

	p.libraryDecorator.reexportDirs(android.PathsForModuleSrc(ctx, p.properties.Export_include_dirs)...)
	p.libraryDecorator.reexportSystemDirs(android.PathsForModuleSrc(ctx, p.properties.Export_system_include_dirs)...)
	p.libraryDecorator.reexportFlags(p.properties.Export_flags...)

	in := android.PathForModuleSrc(ctx, *p.properties.Src)
	p.unstrippedOutputFile = in

	if p.shared() {
		libName := in.Base()
		builderFlags := flagsToBuilderFlags(flags)

		// Optimize out relinking against shared libraries whose interface hasn't changed by
		// depending on a table of contents file instead of the library itself.
		tocFile := android.PathForModuleOut(ctx, libName+".toc")
		p.tocFile = android.OptionalPathForPath(tocFile)
		transformSharedObjectToToc(ctx, in, tocFile, builderFlags)

		ctx.SetProvider(SharedLibraryInfoProvider, SharedLibraryInfo{
			SharedLibrary:           in,
			UnstrippedSharedLibrary: p.unstrippedOutputFile,

			TableOfContents: p.tocFile,
		})
	}

	if p.static() {
		depSet := android.NewDepSetBuilder(android.TOPOLOGICAL).Direct(in).Build()
		ctx.SetProvider(StaticLibraryInfoProvider, StaticLibraryInfo{
			StaticLibrary: in,

			TransitiveStaticLibrariesForOrdering: depSet,
		})
	}

	p.libraryDecorator.flagExporter.setProvider(ctx)

	return in
}

func (p *snapshotLibraryDecorator) install(ctx ModuleContext, file android.Path) {
	if p.matchesWithDevice(ctx.DeviceConfig()) && (p.shared() || p.static()) {
		p.baseInstaller.install(ctx, file)
	}
}

func (p *snapshotLibraryDecorator) nativeCoverage() bool {
	return false
}

func (p *snapshotLibraryDecorator) isSanitizerEnabled(t SanitizerType) bool {
	switch t {
	case cfi:
		return p.sanitizerProperties.Cfi.Src != nil
	default:
		return false
	}
}

func (p *snapshotLibraryDecorator) setSanitizerVariation(t SanitizerType, enabled bool) {
	if !enabled {
		return
	}
	switch t {
	case cfi:
		p.sanitizerProperties.CfiEnabled = true
	default:
		return
	}
}

func snapshotLibraryFactory(snapshotSuffix, moduleSuffix string) (*Module, *snapshotLibraryDecorator) {
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

	prebuilt.init(module, snapshotSuffix, moduleSuffix)
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
	module, prebuilt := snapshotLibraryFactory(vendorSnapshotImageSingleton.moduleNameSuffix(), snapshotSharedSuffix)
	prebuilt.libraryDecorator.BuildOnlyShared()
	return module.Init()
}

// recovery_snapshot_shared is a special prebuilt shared library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of recovery snapshot, recovery_snapshot_shared
// overrides the recovery variant of the cc shared library with the same name, if BOARD_VNDK_VERSION
// is set.
func RecoverySnapshotSharedFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(recoverySnapshotImageSingleton.moduleNameSuffix(), snapshotSharedSuffix)
	prebuilt.libraryDecorator.BuildOnlyShared()
	return module.Init()
}

// vendor_snapshot_static is a special prebuilt static library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of vendor snapshot, vendor_snapshot_static
// overrides the vendor variant of the cc static library with the same name, if BOARD_VNDK_VERSION
// is set.
func VendorSnapshotStaticFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(vendorSnapshotImageSingleton.moduleNameSuffix(), snapshotStaticSuffix)
	prebuilt.libraryDecorator.BuildOnlyStatic()
	return module.Init()
}

// recovery_snapshot_static is a special prebuilt static library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of recovery snapshot, recovery_snapshot_static
// overrides the recovery variant of the cc static library with the same name, if BOARD_VNDK_VERSION
// is set.
func RecoverySnapshotStaticFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(recoverySnapshotImageSingleton.moduleNameSuffix(), snapshotStaticSuffix)
	prebuilt.libraryDecorator.BuildOnlyStatic()
	return module.Init()
}

// vendor_snapshot_header is a special header library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of vendor snapshot, vendor_snapshot_header
// overrides the vendor variant of the cc header library with the same name, if BOARD_VNDK_VERSION
// is set.
func VendorSnapshotHeaderFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(vendorSnapshotImageSingleton.moduleNameSuffix(), snapshotHeaderSuffix)
	prebuilt.libraryDecorator.HeaderOnly()
	return module.Init()
}

// recovery_snapshot_header is a special header library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of recovery snapshot, recovery_snapshot_header
// overrides the recovery variant of the cc header library with the same name, if BOARD_VNDK_VERSION
// is set.
func RecoverySnapshotHeaderFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(recoverySnapshotImageSingleton.moduleNameSuffix(), snapshotHeaderSuffix)
	prebuilt.libraryDecorator.HeaderOnly()
	return module.Init()
}

var _ snapshotSanitizer = (*snapshotLibraryDecorator)(nil)

//
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
	baseSnapshotDecorator
	*binaryDecorator
	properties snapshotBinaryProperties
}

func (p *snapshotBinaryDecorator) matchesWithDevice(config android.DeviceConfig) bool {
	if config.DeviceArch() != p.arch() {
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
	if !p.matchesWithDevice(ctx.DeviceConfig()) {
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

	return outputFile
}

func (p *snapshotBinaryDecorator) nativeCoverage() bool {
	return false
}

// vendor_snapshot_binary is a special prebuilt executable binary which is auto-generated by
// development/vendor_snapshot/update.py. As a part of vendor snapshot, vendor_snapshot_binary
// overrides the vendor variant of the cc binary with the same name, if BOARD_VNDK_VERSION is set.
func VendorSnapshotBinaryFactory() android.Module {
	return snapshotBinaryFactory(vendorSnapshotImageSingleton.moduleNameSuffix(), snapshotBinarySuffix)
}

// recovery_snapshot_binary is a special prebuilt executable binary which is auto-generated by
// development/vendor_snapshot/update.py. As a part of recovery snapshot, recovery_snapshot_binary
// overrides the recovery variant of the cc binary with the same name, if BOARD_VNDK_VERSION is set.
func RecoverySnapshotBinaryFactory() android.Module {
	return snapshotBinaryFactory(recoverySnapshotImageSingleton.moduleNameSuffix(), snapshotBinarySuffix)
}

func snapshotBinaryFactory(snapshotSuffix, moduleSuffix string) android.Module {
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

	prebuilt.init(module, snapshotSuffix, moduleSuffix)
	module.AddProperties(&prebuilt.properties)
	return module.Init()
}

//
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
	baseSnapshotDecorator
	objectLinker
	properties vendorSnapshotObjectProperties
}

func (p *snapshotObjectLinker) matchesWithDevice(config android.DeviceConfig) bool {
	if config.DeviceArch() != p.arch() {
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
	if !p.matchesWithDevice(ctx.DeviceConfig()) {
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
	module := newObject()

	prebuilt := &snapshotObjectLinker{
		objectLinker: objectLinker{
			baseLinker: NewBaseLinker(nil),
		},
	}
	module.linker = prebuilt

	prebuilt.init(module, vendorSnapshotImageSingleton.moduleNameSuffix(), snapshotObjectSuffix)
	module.AddProperties(&prebuilt.properties)
	return module.Init()
}

// recovery_snapshot_object is a special prebuilt compiled object file which is auto-generated by
// development/vendor_snapshot/update.py. As a part of recovery snapshot, recovery_snapshot_object
// overrides the recovery variant of the cc object with the same name, if BOARD_VNDK_VERSION is set.
func RecoverySnapshotObjectFactory() android.Module {
	module := newObject()

	prebuilt := &snapshotObjectLinker{
		objectLinker: objectLinker{
			baseLinker: NewBaseLinker(nil),
		},
	}
	module.linker = prebuilt

	prebuilt.init(module, recoverySnapshotImageSingleton.moduleNameSuffix(), snapshotObjectSuffix)
	module.AddProperties(&prebuilt.properties)
	return module.Init()
}

type snapshotInterface interface {
	matchesWithDevice(config android.DeviceConfig) bool
	isSnapshotPrebuilt() bool
	version() string
	snapshotAndroidMkSuffix() string
}

var _ snapshotInterface = (*vndkPrebuiltLibraryDecorator)(nil)
var _ snapshotInterface = (*snapshotLibraryDecorator)(nil)
var _ snapshotInterface = (*snapshotBinaryDecorator)(nil)
var _ snapshotInterface = (*snapshotObjectLinker)(nil)
