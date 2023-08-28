// Copyright 2021 The Android Open Source Project
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

package rust

import (
	"fmt"

	"android/soong/android"
	"android/soong/cc"

	"github.com/google/blueprint/proptools"
)

type snapshotLibraryDecorator struct {
	cc.BaseSnapshotDecorator
	*libraryDecorator
	properties          cc.SnapshotLibraryProperties
	sanitizerProperties struct {
		SanitizerVariation cc.SanitizerType `blueprint:"mutated"`

		//TODO: Library flags for cfi variant when CFI is supported.
		//Cfi cc.SnapshotLibraryProperties `android:"arch_variant"`

		// Library flags for hwasan variant.
		Hwasan cc.SnapshotLibraryProperties `android:"arch_variant"`
	}
}

var _ cc.SnapshotSanitizer = (*snapshotLibraryDecorator)(nil)

func (library *snapshotLibraryDecorator) IsSanitizerAvailable(t cc.SanitizerType) bool {
	switch t {
	//TODO: When CFI is supported, add a check here as well
	case cc.Hwasan:
		return library.sanitizerProperties.Hwasan.Src != nil
	default:
		return false
	}
}

func (library *snapshotLibraryDecorator) SetSanitizerVariation(t cc.SanitizerType, enabled bool) {
	if !enabled || library.IsSanitizerEnabled(t) {
		return
	}
	if !library.IsUnsanitizedVariant() {
		panic(fmt.Errorf("snapshot Sanitizer must be one of Cfi or Hwasan but not both"))
	}
	library.sanitizerProperties.SanitizerVariation = t
}

func (library *snapshotLibraryDecorator) IsSanitizerEnabled(t cc.SanitizerType) bool {
	return library.sanitizerProperties.SanitizerVariation == t
}

func (library *snapshotLibraryDecorator) IsUnsanitizedVariant() bool {
	//TODO: When CFI is supported, add a check here as well
	return !library.IsSanitizerEnabled(cc.Hwasan)
}

func init() {
	registerRustSnapshotModules(android.InitRegistrationContext)
}

func (mod *Module) IsSnapshotSanitizerAvailable(t cc.SanitizerType) bool {
	if ss, ok := mod.compiler.(cc.SnapshotSanitizer); ok {
		return ss.IsSanitizerAvailable(t)
	}
	return false
}

func (mod *Module) SetSnapshotSanitizerVariation(t cc.SanitizerType, enabled bool) {
	if ss, ok := mod.compiler.(cc.SnapshotSanitizer); ok {
		ss.SetSanitizerVariation(t, enabled)
	} else {
		panic(fmt.Errorf("Calling SetSnapshotSanitizerVariation on a non-snapshotLibraryDecorator: %s", mod.Name()))
	}
}

func (mod *Module) IsSnapshotUnsanitizedVariant() bool {
	if ss, ok := mod.compiler.(cc.SnapshotSanitizer); ok {
		return ss.IsUnsanitizedVariant()
	}
	return false
}

func (mod *Module) IsSnapshotSanitizer() bool {
	if _, ok := mod.compiler.(cc.SnapshotSanitizer); ok {
		return true
	}
	return false
}

func registerRustSnapshotModules(ctx android.RegistrationContext) {
	cc.VendorSnapshotImageSingleton.RegisterAdditionalModule(ctx,
		"vendor_snapshot_rlib", VendorSnapshotRlibFactory)
	cc.VendorSnapshotImageSingleton.RegisterAdditionalModule(ctx,
		"vendor_snapshot_dylib", VendorSnapshotDylibFactory)
	cc.RecoverySnapshotImageSingleton.RegisterAdditionalModule(ctx,
		"recovery_snapshot_rlib", RecoverySnapshotRlibFactory)
}

func snapshotLibraryFactory(image cc.SnapshotImage, moduleSuffix string) (*Module, *snapshotLibraryDecorator) {
	module, library := NewRustLibrary(android.DeviceSupported)

	module.sanitize = nil
	library.stripper.StripProperties.Strip.None = proptools.BoolPtr(true)

	prebuilt := &snapshotLibraryDecorator{
		libraryDecorator: library,
	}

	module.compiler = prebuilt

	prebuilt.Init(module, image, moduleSuffix)
	module.AddProperties(
		&prebuilt.properties,
		&prebuilt.sanitizerProperties,
	)

	return module, prebuilt
}

func (library *snapshotLibraryDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) buildOutput {
	var variant string
	if library.static() {
		variant = cc.SnapshotStaticSuffix
	} else if library.shared() {
		variant = cc.SnapshotSharedSuffix
	} else if library.rlib() {
		variant = cc.SnapshotRlibSuffix
	} else if library.dylib() {
		variant = cc.SnapshotDylibSuffix
	}

	library.SetSnapshotAndroidMkSuffix(ctx, variant)

	if library.IsSanitizerEnabled(cc.Hwasan) {
		library.properties = library.sanitizerProperties.Hwasan
	}
	if !library.MatchesWithDevice(ctx.DeviceConfig()) {
		return buildOutput{}
	}
	outputFile := android.PathForModuleSrc(ctx, *library.properties.Src)
	library.unstrippedOutputFile = outputFile
	return buildOutput{outputFile: outputFile}
}

func (library *snapshotLibraryDecorator) rustdoc(ctx ModuleContext, flags Flags, deps PathDeps) android.OptionalPath {
	return android.OptionalPath{}
}

// vendor_snapshot_rlib is a special prebuilt rlib library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of vendor snapshot, vendor_snapshot_rlib
// overrides the vendor variant of the rust rlib library with the same name, if BOARD_VNDK_VERSION
// is set.
func VendorSnapshotRlibFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(cc.VendorSnapshotImageSingleton, cc.SnapshotRlibSuffix)
	prebuilt.libraryDecorator.BuildOnlyRlib()
	prebuilt.libraryDecorator.setNoStdlibs()
	return module.Init()
}

// vendor_snapshot_dylib is a special prebuilt dylib library which is auto-generated by
// development/vendor_snapshot/update.py. As a part of vendor snapshot, vendor_snapshot_dylib
// overrides the vendor variant of the rust dylib library with the same name, if BOARD_VNDK_VERSION
// is set.
func VendorSnapshotDylibFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(cc.VendorSnapshotImageSingleton, cc.SnapshotDylibSuffix)
	prebuilt.libraryDecorator.BuildOnlyDylib()
	prebuilt.libraryDecorator.setNoStdlibs()
	return module.Init()
}

func RecoverySnapshotRlibFactory() android.Module {
	module, prebuilt := snapshotLibraryFactory(cc.RecoverySnapshotImageSingleton, cc.SnapshotRlibSuffix)
	prebuilt.libraryDecorator.BuildOnlyRlib()
	prebuilt.libraryDecorator.setNoStdlibs()
	return module.Init()
}

func (library *snapshotLibraryDecorator) MatchesWithDevice(config android.DeviceConfig) bool {
	arches := config.Arches()
	if len(arches) == 0 || arches[0].ArchType.String() != library.Arch() {
		return false
	}
	if library.properties.Src == nil {
		return false
	}
	return true
}

func (library *snapshotLibraryDecorator) IsSnapshotPrebuilt() bool {
	return true
}

var _ cc.SnapshotInterface = (*snapshotLibraryDecorator)(nil)
