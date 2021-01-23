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
	"sync"

	"android/soong/android"

	"github.com/google/blueprint/proptools"
)

// Defines the specifics of different images to which the snapshot process is applicable, e.g.,
// vendor, recovery, ramdisk.
type snapshotImage interface {
	// Used to register callbacks with the build system.
	init()

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

	// Returns the snapshotMap to be used for a given module and config, or nil if the
	// module is not included in this image.
	getSnapshotMap(m *Module, cfg android.Config) *snapshotMap

	// Returns mutex used for mutual exclusion when updating the snapshot maps.
	getMutex() *sync.Mutex

	// For a given arch, a maps of which modules are included in this image.
	suffixModules(config android.Config) map[string]bool

	// Whether to add a given module to the suffix map.
	shouldBeAddedToSuffixModules(m *Module) bool

	// Returns true if the build is using a snapshot for this image.
	isUsingSnapshot(cfg android.DeviceConfig) bool

	// Whether to skip the module mutator for a module in a given context.
	skipModuleMutator(ctx android.BottomUpMutatorContext) bool

	// Whether to skip the source mutator for a given module.
	skipSourceMutator(ctx android.BottomUpMutatorContext) bool

	// Whether to exclude a given module from the directed snapshot or not.
	// If the makefile variable DIRECTED_{IMAGE}_SNAPSHOT is true, directed snapshot is turned on,
	// and only modules listed in {IMAGE}_SNAPSHOT_MODULES will be captured.
	excludeFromDirectedSnapshot(cfg android.DeviceConfig, name string) bool
}

type vendorSnapshotImage struct{}
type recoverySnapshotImage struct{}

func (vendorSnapshotImage) init() {
	android.RegisterSingletonType("vendor-snapshot", VendorSnapshotSingleton)
	android.RegisterModuleType("vendor_snapshot_shared", VendorSnapshotSharedFactory)
	android.RegisterModuleType("vendor_snapshot_static", VendorSnapshotStaticFactory)
	android.RegisterModuleType("vendor_snapshot_header", VendorSnapshotHeaderFactory)
	android.RegisterModuleType("vendor_snapshot_binary", VendorSnapshotBinaryFactory)
	android.RegisterModuleType("vendor_snapshot_object", VendorSnapshotObjectFactory)

	android.RegisterSingletonType("vendor-fake-snapshot", VendorFakeSnapshotSingleton)
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

func (vendorSnapshotImage) getSnapshotMap(m *Module, cfg android.Config) *snapshotMap {
	if lib, ok := m.linker.(libraryInterface); ok {
		if lib.static() {
			return vendorSnapshotStaticLibs(cfg)
		} else if lib.shared() {
			return vendorSnapshotSharedLibs(cfg)
		} else {
			// header
			return vendorSnapshotHeaderLibs(cfg)
		}
	} else if m.binary() {
		return vendorSnapshotBinaries(cfg)
	} else if m.object() {
		return vendorSnapshotObjects(cfg)
	} else {
		return nil
	}
}

func (vendorSnapshotImage) getMutex() *sync.Mutex {
	return &vendorSnapshotsLock
}

func (vendorSnapshotImage) suffixModules(config android.Config) map[string]bool {
	return vendorSuffixModules(config)
}

func (vendorSnapshotImage) shouldBeAddedToSuffixModules(module *Module) bool {
	// vendor suffix should be added to snapshots if the source module isn't vendor: true.
	if module.SocSpecific() {
		return false
	}

	// But we can't just check SocSpecific() since we already passed the image mutator.
	// Check ramdisk and recovery to see if we are real "vendor: true" module.
	ramdiskAvailable := module.InRamdisk() && !module.OnlyInRamdisk()
	vendorRamdiskAvailable := module.InVendorRamdisk() && !module.OnlyInVendorRamdisk()
	recoveryAvailable := module.InRecovery() && !module.OnlyInRecovery()

	return !ramdiskAvailable && !recoveryAvailable && !vendorRamdiskAvailable
}

func (vendorSnapshotImage) isUsingSnapshot(cfg android.DeviceConfig) bool {
	vndkVersion := cfg.VndkVersion()
	return vndkVersion != "current" && vndkVersion != ""
}

func (vendorSnapshotImage) skipModuleMutator(ctx android.BottomUpMutatorContext) bool {
	vndkVersion := ctx.DeviceConfig().VndkVersion()
	module, ok := ctx.Module().(*Module)
	return !ok || module.VndkVersion() != vndkVersion
}

func (vendorSnapshotImage) skipSourceMutator(ctx android.BottomUpMutatorContext) bool {
	vndkVersion := ctx.DeviceConfig().VndkVersion()
	module, ok := ctx.Module().(*Module)
	if !ok {
		return true
	}
	if module.VndkVersion() != vndkVersion {
		return true
	}
	// .. and also filter out llndk library
	if module.IsLlndk() {
		return true
	}
	return false
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

func (recoverySnapshotImage) init() {
	android.RegisterSingletonType("recovery-snapshot", RecoverySnapshotSingleton)
	android.RegisterModuleType("recovery_snapshot_shared", RecoverySnapshotSharedFactory)
	android.RegisterModuleType("recovery_snapshot_static", RecoverySnapshotStaticFactory)
	android.RegisterModuleType("recovery_snapshot_header", RecoverySnapshotHeaderFactory)
	android.RegisterModuleType("recovery_snapshot_binary", RecoverySnapshotBinaryFactory)
	android.RegisterModuleType("recovery_snapshot_object", RecoverySnapshotObjectFactory)
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

func (recoverySnapshotImage) getSnapshotMap(m *Module, cfg android.Config) *snapshotMap {
	if lib, ok := m.linker.(libraryInterface); ok {
		if lib.static() {
			return recoverySnapshotStaticLibs(cfg)
		} else if lib.shared() {
			return recoverySnapshotSharedLibs(cfg)
		} else {
			// header
			return recoverySnapshotHeaderLibs(cfg)
		}
	} else if m.binary() {
		return recoverySnapshotBinaries(cfg)
	} else if m.object() {
		return recoverySnapshotObjects(cfg)
	} else {
		return nil
	}
}

func (recoverySnapshotImage) getMutex() *sync.Mutex {
	return &recoverySnapshotsLock
}

func (recoverySnapshotImage) suffixModules(config android.Config) map[string]bool {
	return recoverySuffixModules(config)
}

func (recoverySnapshotImage) shouldBeAddedToSuffixModules(module *Module) bool {
	return proptools.BoolDefault(module.Properties.Recovery_available, false)
}

func (recoverySnapshotImage) isUsingSnapshot(cfg android.DeviceConfig) bool {
	recoverySnapshotVersion := cfg.RecoverySnapshotVersion()
	return recoverySnapshotVersion != "current" && recoverySnapshotVersion != ""
}

func (recoverySnapshotImage) skipModuleMutator(ctx android.BottomUpMutatorContext) bool {
	module, ok := ctx.Module().(*Module)
	return !ok || !module.InRecovery()
}

func (recoverySnapshotImage) skipSourceMutator(ctx android.BottomUpMutatorContext) bool {
	module, ok := ctx.Module().(*Module)
	return !ok || !module.InRecovery()
}

func (recoverySnapshotImage) excludeFromDirectedSnapshot(cfg android.DeviceConfig, name string) bool {
	// directed recovery snapshot is not implemented yet
	return false
}

var vendorSnapshotImageSingleton vendorSnapshotImage
var recoverySnapshotImageSingleton recoverySnapshotImage

func init() {
	vendorSnapshotImageSingleton.init()
	recoverySnapshotImageSingleton.init()
}

const (
	vendorSnapshotHeaderSuffix = ".vendor_header."
	vendorSnapshotSharedSuffix = ".vendor_shared."
	vendorSnapshotStaticSuffix = ".vendor_static."
	vendorSnapshotBinarySuffix = ".vendor_binary."
	vendorSnapshotObjectSuffix = ".vendor_object."
)

const (
	recoverySnapshotHeaderSuffix = ".recovery_header."
	recoverySnapshotSharedSuffix = ".recovery_shared."
	recoverySnapshotStaticSuffix = ".recovery_static."
	recoverySnapshotBinarySuffix = ".recovery_binary."
	recoverySnapshotObjectSuffix = ".recovery_object."
)

var (
	vendorSnapshotsLock         sync.Mutex
	vendorSuffixModulesKey      = android.NewOnceKey("vendorSuffixModules")
	vendorSnapshotHeaderLibsKey = android.NewOnceKey("vendorSnapshotHeaderLibs")
	vendorSnapshotStaticLibsKey = android.NewOnceKey("vendorSnapshotStaticLibs")
	vendorSnapshotSharedLibsKey = android.NewOnceKey("vendorSnapshotSharedLibs")
	vendorSnapshotBinariesKey   = android.NewOnceKey("vendorSnapshotBinaries")
	vendorSnapshotObjectsKey    = android.NewOnceKey("vendorSnapshotObjects")
)

var (
	recoverySnapshotsLock         sync.Mutex
	recoverySuffixModulesKey      = android.NewOnceKey("recoverySuffixModules")
	recoverySnapshotHeaderLibsKey = android.NewOnceKey("recoverySnapshotHeaderLibs")
	recoverySnapshotStaticLibsKey = android.NewOnceKey("recoverySnapshotStaticLibs")
	recoverySnapshotSharedLibsKey = android.NewOnceKey("recoverySnapshotSharedLibs")
	recoverySnapshotBinariesKey   = android.NewOnceKey("recoverySnapshotBinaries")
	recoverySnapshotObjectsKey    = android.NewOnceKey("recoverySnapshotObjects")
)

// vendorSuffixModules holds names of modules whose vendor variants should have the vendor suffix.
// This is determined by source modules, and then this will be used when exporting snapshot modules
// to Makefile.
//
// For example, if libbase has "vendor_available: true", the name of core variant will be "libbase"
// while the name of vendor variant will be "libbase.vendor". In such cases, the vendor snapshot of
// "libbase" should be exported with the name "libbase.vendor".
//
// Refer to VendorSnapshotSourceMutator and makeLibName which use this.
func vendorSuffixModules(config android.Config) map[string]bool {
	return config.Once(vendorSuffixModulesKey, func() interface{} {
		return make(map[string]bool)
	}).(map[string]bool)
}

// these are vendor snapshot maps holding names of vendor snapshot modules
func vendorSnapshotHeaderLibs(config android.Config) *snapshotMap {
	return config.Once(vendorSnapshotHeaderLibsKey, func() interface{} {
		return newSnapshotMap()
	}).(*snapshotMap)
}

func vendorSnapshotSharedLibs(config android.Config) *snapshotMap {
	return config.Once(vendorSnapshotSharedLibsKey, func() interface{} {
		return newSnapshotMap()
	}).(*snapshotMap)
}

func vendorSnapshotStaticLibs(config android.Config) *snapshotMap {
	return config.Once(vendorSnapshotStaticLibsKey, func() interface{} {
		return newSnapshotMap()
	}).(*snapshotMap)
}

func vendorSnapshotBinaries(config android.Config) *snapshotMap {
	return config.Once(vendorSnapshotBinariesKey, func() interface{} {
		return newSnapshotMap()
	}).(*snapshotMap)
}

func vendorSnapshotObjects(config android.Config) *snapshotMap {
	return config.Once(vendorSnapshotObjectsKey, func() interface{} {
		return newSnapshotMap()
	}).(*snapshotMap)
}

func recoverySuffixModules(config android.Config) map[string]bool {
	return config.Once(recoverySuffixModulesKey, func() interface{} {
		return make(map[string]bool)
	}).(map[string]bool)
}

func recoverySnapshotHeaderLibs(config android.Config) *snapshotMap {
	return config.Once(recoverySnapshotHeaderLibsKey, func() interface{} {
		return newSnapshotMap()
	}).(*snapshotMap)
}

func recoverySnapshotSharedLibs(config android.Config) *snapshotMap {
	return config.Once(recoverySnapshotSharedLibsKey, func() interface{} {
		return newSnapshotMap()
	}).(*snapshotMap)
}

func recoverySnapshotStaticLibs(config android.Config) *snapshotMap {
	return config.Once(recoverySnapshotStaticLibsKey, func() interface{} {
		return newSnapshotMap()
	}).(*snapshotMap)
}

func recoverySnapshotBinaries(config android.Config) *snapshotMap {
	return config.Once(recoverySnapshotBinariesKey, func() interface{} {
		return newSnapshotMap()
	}).(*snapshotMap)
}

func recoverySnapshotObjects(config android.Config) *snapshotMap {
	return config.Once(recoverySnapshotObjectsKey, func() interface{} {
		return newSnapshotMap()
	}).(*snapshotMap)
}

type baseSnapshotDecoratorProperties struct {
	// snapshot version.
	Version string

	// Target arch name of the snapshot (e.g. 'arm64' for variant 'aosp_arm64')
	Target_arch string

	// Suffix to be added to the module name, e.g., vendor_shared,
	// recovery_shared, etc.
	Module_suffix string
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
	versionSuffix := p.version()
	if p.arch() != "" {
		versionSuffix += "." + p.arch()
	}

	return p.baseProperties.Module_suffix + versionSuffix
}

func (p *baseSnapshotDecorator) version() string {
	return p.baseProperties.Version
}

func (p *baseSnapshotDecorator) arch() string {
	return p.baseProperties.Target_arch
}

func (p *baseSnapshotDecorator) module_suffix() string {
	return p.baseProperties.Module_suffix
}

func (p *baseSnapshotDecorator) isSnapshotPrebuilt() bool {
	return true
}

// Call this with a module suffix after creating a snapshot module, such as
// vendorSnapshotSharedSuffix, recoverySnapshotBinarySuffix, etc.
func (p *baseSnapshotDecorator) init(m *Module, suffix string) {
	p.baseProperties.Module_suffix = suffix
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
	androidMkSuffix string
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
	m := ctx.Module().(*Module)

	if m.InVendor() && vendorSuffixModules(ctx.Config())[m.BaseModuleName()] {
		p.androidMkSuffix = vendorSuffix
	} else if m.InRecovery() && recoverySuffixModules(ctx.Config())[m.BaseModuleName()] {
		p.androidMkSuffix = recoverySuffix
	}

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

func snapshotLibraryFactory(suffix string) (*Module, *snapshotLibraryDecorator) {
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

	prebuilt.init(module, suffix)
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
	module, prebuilt := snapshotLibraryFactory(vendorSnapshotSharedSuffix)
	prebuilt.libraryDecorator.BuildOnlyShared()
	return module.Init()
}

// recovery_snapshot_shared is a special prebuilt shared library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of recovery snapshot, recovery_snapshot_shared
// overrides the recovery variant of the cc shared library with the same name, if BOARD_VNDK_VERSION
// is set.
func RecoverySnapshotSharedFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(recoverySnapshotSharedSuffix)
	prebuilt.libraryDecorator.BuildOnlyShared()
	return module.Init()
}

// vendor_snapshot_static is a special prebuilt static library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of vendor snapshot, vendor_snapshot_static
// overrides the vendor variant of the cc static library with the same name, if BOARD_VNDK_VERSION
// is set.
func VendorSnapshotStaticFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(vendorSnapshotStaticSuffix)
	prebuilt.libraryDecorator.BuildOnlyStatic()
	return module.Init()
}

// recovery_snapshot_static is a special prebuilt static library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of recovery snapshot, recovery_snapshot_static
// overrides the recovery variant of the cc static library with the same name, if BOARD_VNDK_VERSION
// is set.
func RecoverySnapshotStaticFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(recoverySnapshotStaticSuffix)
	prebuilt.libraryDecorator.BuildOnlyStatic()
	return module.Init()
}

// vendor_snapshot_header is a special header library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of vendor snapshot, vendor_snapshot_header
// overrides the vendor variant of the cc header library with the same name, if BOARD_VNDK_VERSION
// is set.
func VendorSnapshotHeaderFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(vendorSnapshotHeaderSuffix)
	prebuilt.libraryDecorator.HeaderOnly()
	return module.Init()
}

// recovery_snapshot_header is a special header library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of recovery snapshot, recovery_snapshot_header
// overrides the recovery variant of the cc header library with the same name, if BOARD_VNDK_VERSION
// is set.
func RecoverySnapshotHeaderFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(recoverySnapshotHeaderSuffix)
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
	properties      snapshotBinaryProperties
	androidMkSuffix string
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

	m := ctx.Module().(*Module)
	if m.InVendor() && vendorSuffixModules(ctx.Config())[m.BaseModuleName()] {
		p.androidMkSuffix = vendorSuffix
	} else if m.InRecovery() && recoverySuffixModules(ctx.Config())[m.BaseModuleName()] {
		p.androidMkSuffix = recoverySuffix

	}

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
	return snapshotBinaryFactory(vendorSnapshotBinarySuffix)
}

// recovery_snapshot_binary is a special prebuilt executable binary which is auto-generated by
// development/vendor_snapshot/update.py. As a part of recovery snapshot, recovery_snapshot_binary
// overrides the recovery variant of the cc binary with the same name, if BOARD_VNDK_VERSION is set.
func RecoverySnapshotBinaryFactory() android.Module {
	return snapshotBinaryFactory(recoverySnapshotBinarySuffix)
}

func snapshotBinaryFactory(suffix string) android.Module {
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

	prebuilt.init(module, suffix)
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
	properties      vendorSnapshotObjectProperties
	androidMkSuffix string
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

	m := ctx.Module().(*Module)

	if m.InVendor() && vendorSuffixModules(ctx.Config())[m.BaseModuleName()] {
		p.androidMkSuffix = vendorSuffix
	} else if m.InRecovery() && recoverySuffixModules(ctx.Config())[m.BaseModuleName()] {
		p.androidMkSuffix = recoverySuffix
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

	prebuilt.init(module, vendorSnapshotObjectSuffix)
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

	prebuilt.init(module, recoverySnapshotObjectSuffix)
	module.AddProperties(&prebuilt.properties)
	return module.Init()
}

type snapshotInterface interface {
	matchesWithDevice(config android.DeviceConfig) bool
}

var _ snapshotInterface = (*vndkPrebuiltLibraryDecorator)(nil)
var _ snapshotInterface = (*snapshotLibraryDecorator)(nil)
var _ snapshotInterface = (*snapshotBinaryDecorator)(nil)
var _ snapshotInterface = (*snapshotObjectLinker)(nil)

//
// Mutators that helps vendor snapshot modules override source modules.
//

// VendorSnapshotMutator gathers all snapshots for vendor, and disable all snapshots which don't
// match with device, e.g.
//   - snapshot version is different with BOARD_VNDK_VERSION
//   - snapshot arch is different with device's arch (e.g. arm vs x86)
//
// This also handles vndk_prebuilt_shared, except for they won't be disabled in any cases, given
// that any versions of VNDK might be packed into vndk APEX.
//
// TODO(b/145966707): remove mutator and utilize android.Prebuilt to override source modules
func VendorSnapshotMutator(ctx android.BottomUpMutatorContext) {
	snapshotMutator(ctx, vendorSnapshotImageSingleton)
}

func RecoverySnapshotMutator(ctx android.BottomUpMutatorContext) {
	snapshotMutator(ctx, recoverySnapshotImageSingleton)
}

func snapshotMutator(ctx android.BottomUpMutatorContext, image snapshotImage) {
	if !image.isUsingSnapshot(ctx.DeviceConfig()) {
		return
	}
	module, ok := ctx.Module().(*Module)
	if !ok || !module.Enabled() {
		return
	}
	if image.skipModuleMutator(ctx) {
		return
	}
	if !module.isSnapshotPrebuilt() {
		return
	}

	// isSnapshotPrebuilt ensures snapshotInterface
	if !module.linker.(snapshotInterface).matchesWithDevice(ctx.DeviceConfig()) {
		// Disable unnecessary snapshot module, but do not disable
		// vndk_prebuilt_shared because they might be packed into vndk APEX
		if !module.IsVndk() {
			module.Disable()
		}
		return
	}

	var snapshotMap *snapshotMap = image.getSnapshotMap(module, ctx.Config())
	if snapshotMap == nil {
		return
	}

	mutex := image.getMutex()
	mutex.Lock()
	defer mutex.Unlock()
	snapshotMap.add(module.BaseModuleName(), ctx.Arch().ArchType, ctx.ModuleName())
}

// VendorSnapshotSourceMutator disables source modules which have corresponding snapshots.
func VendorSnapshotSourceMutator(ctx android.BottomUpMutatorContext) {
	snapshotSourceMutator(ctx, vendorSnapshotImageSingleton)
}

func RecoverySnapshotSourceMutator(ctx android.BottomUpMutatorContext) {
	snapshotSourceMutator(ctx, recoverySnapshotImageSingleton)
}

func snapshotSourceMutator(ctx android.BottomUpMutatorContext, image snapshotImage) {
	if !ctx.Device() {
		return
	}
	if !image.isUsingSnapshot(ctx.DeviceConfig()) {
		return
	}

	module, ok := ctx.Module().(*Module)
	if !ok {
		return
	}

	if image.shouldBeAddedToSuffixModules(module) {
		mutex := image.getMutex()
		mutex.Lock()
		defer mutex.Unlock()

		image.suffixModules(ctx.Config())[ctx.ModuleName()] = true
	}

	if module.isSnapshotPrebuilt() {
		return
	}
	if image.skipSourceMutator(ctx) {
		return
	}

	var snapshotMap *snapshotMap = image.getSnapshotMap(module, ctx.Config())
	if snapshotMap == nil {
		return
	}

	if _, ok := snapshotMap.get(ctx.ModuleName(), ctx.Arch().ArchType); !ok {
		// Corresponding snapshot doesn't exist
		return
	}

	// Disables source modules if corresponding snapshot exists.
	if lib, ok := module.linker.(libraryInterface); ok && lib.buildStatic() && lib.buildShared() {
		// But do not disable because the shared variant depends on the static variant.
		module.HideFromMake()
		module.Properties.HideFromMake = true
	} else {
		module.Disable()
	}
}
