// Copyright 2020 The Android Open Source Project
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

package rust

import (
	"strings"

	"android/soong/android"
	"android/soong/cc"
)

var _ android.ImageInterface = (*Module)(nil)

var _ cc.ImageMutatableModule = (*Module)(nil)

func (mod *Module) VendorAvailable() bool {
	return Bool(mod.VendorProperties.Vendor_available)
}

func (mod *Module) OdmAvailable() bool {
	return Bool(mod.VendorProperties.Odm_available)
}

func (mod *Module) ProductAvailable() bool {
	return false
}

func (mod *Module) RamdiskAvailable() bool {
	return false
}

func (mod *Module) VendorRamdiskAvailable() bool {
	return Bool(mod.Properties.Vendor_ramdisk_available)
}

func (mod *Module) AndroidModuleBase() *android.ModuleBase {
	return &mod.ModuleBase
}

func (mod *Module) RecoveryAvailable() bool {
	return false
}

func (mod *Module) ExtraVariants() []string {
	return mod.Properties.ExtraVariants
}

func (mod *Module) AppendExtraVariant(extraVariant string) {
	mod.Properties.ExtraVariants = append(mod.Properties.ExtraVariants, extraVariant)
}

func (mod *Module) SetRamdiskVariantNeeded(b bool) {
	if b {
		panic("Setting ramdisk variant needed for Rust module is unsupported: " + mod.BaseModuleName())
	}
}

func (mod *Module) SetVendorRamdiskVariantNeeded(b bool) {
	mod.Properties.VendorRamdiskVariantNeeded = b
}

func (mod *Module) SetRecoveryVariantNeeded(b bool) {
	if b {
		panic("Setting recovery variant needed for Rust module is unsupported: " + mod.BaseModuleName())
	}
}

func (mod *Module) SetCoreVariantNeeded(b bool) {
	mod.Properties.CoreVariantNeeded = b
}

func (mod *Module) SnapshotVersion(mctx android.BaseModuleContext) string {
	panic("Rust modules do not support snapshotting: " + mod.BaseModuleName())
}

func (mod *Module) VendorRamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return mod.Properties.VendorRamdiskVariantNeeded
}

func (mod *Module) CoreVariantNeeded(ctx android.BaseModuleContext) bool {
	return mod.Properties.CoreVariantNeeded
}

func (mod *Module) RamdiskVariantNeeded(android.BaseModuleContext) bool {
	return mod.InRamdisk()
}

func (mod *Module) RecoveryVariantNeeded(android.BaseModuleContext) bool {
	return mod.InRecovery()
}

func (mod *Module) ExtraImageVariations(android.BaseModuleContext) []string {
	return mod.Properties.ExtraVariants
}

func (mod *Module) IsSnapshotPrebuilt() bool {
	// Rust does not support prebuilts in its snapshots
	return false
}

func (ctx *moduleContext) ProductSpecific() bool {
	return false
}

func (mod *Module) InRecovery() bool {
	// TODO(b/165791368)
	return false
}

func (mod *Module) InVendorRamdisk() bool {
	return mod.ModuleBase.InVendorRamdisk() || mod.ModuleBase.InstallInVendorRamdisk()
}

func (mod *Module) OnlyInRamdisk() bool {
	// TODO(b/165791368)
	return false
}

func (mod *Module) OnlyInRecovery() bool {
	// TODO(b/165791368)
	return false
}

func (mod *Module) OnlyInVendorRamdisk() bool {
	return false
}

// Returns true when this module is configured to have core and vendor variants.
func (mod *Module) HasVendorVariant() bool {
	return Bool(mod.VendorProperties.Vendor_available) || Bool(mod.VendorProperties.Odm_available)
}

// Always returns false because rust modules do not support product variant.
func (mod *Module) HasProductVariant() bool {
	return Bool(mod.VendorProperties.Product_available)
}

func (mod *Module) HasNonSystemVariants() bool {
	return mod.HasVendorVariant() || mod.HasProductVariant()
}

func (mod *Module) InProduct() bool {
	return false
}

func (mod *Module) SetImageVariation(ctx android.BaseModuleContext, variant string, module android.Module) {
	m := module.(*Module)
	if variant == android.VendorRamdiskVariation {
		m.MakeAsPlatform()
	} else if strings.HasPrefix(variant, cc.VendorVariationPrefix) {
		m.Properties.ImageVariationPrefix = cc.VendorVariationPrefix
		m.Properties.VndkVersion = strings.TrimPrefix(variant, cc.VendorVariationPrefix)

		// Makefile shouldn't know vendor modules other than BOARD_VNDK_VERSION.
		// Hide other vendor variants to avoid collision.
		vndkVersion := ctx.DeviceConfig().VndkVersion()
		if vndkVersion != "current" && vndkVersion != "" && vndkVersion != m.Properties.VndkVersion {
			m.Properties.HideFromMake = true
			m.HideFromMake()
		}
	}
}

func (mod *Module) ImageMutatorBegin(mctx android.BaseModuleContext) {
	// Rust does not support installing to the product image yet.
	if Bool(mod.VendorProperties.Product_available) {
		mctx.PropertyErrorf("product_available",
			"Rust modules do not yet support being available to the product image")
	} else if mctx.ProductSpecific() {
		mctx.PropertyErrorf("product_specific",
			"Rust modules do not yet support installing to the product image.")
	} else if Bool(mod.VendorProperties.Double_loadable) {
		mctx.PropertyErrorf("double_loadable",
			"Rust modules do not yet support double loading")
	}

	if mod.HasVendorVariant() {
		if lib, ok := mod.compiler.(libraryInterface); ok && lib.buildShared() {
			// Explicitly disallow rust_ffi variants which produce shared libraries from setting vendor_available.
			// Vendor variants do not produce an error for dylibs, rlibs with dylib-std linkage are disabled in the respective library
			// mutators until support is added.
			//
			// We can't check shared() here because image mutator is called before the library mutator, so we need to
			// check buildShared()

			mctx.PropertyErrorf("vendor_available", "cannot be set for rust_ffi or rust_ffi_shared modules.")
		}
	}

	if Bool(mod.Properties.Vendor_ramdisk_available) {
		if lib, ok := mod.compiler.(libraryInterface); !ok || (ok && lib.buildShared()) {
			mctx.PropertyErrorf("vendor_ramdisk_available", "cannot be set for rust_ffi or rust_ffi_shared modules.")
		}
	}

	vendorSpecific := mctx.SocSpecific() || mctx.DeviceSpecific()
	if vendorSpecific {
		if lib, ok := mod.compiler.(libraryInterface); !ok || (ok && (lib.buildShared() || lib.buildDylib() || lib.buildRlib())) {
			mctx.ModuleErrorf("Rust vendor specific modules are currently only supported for rust_ffi_static modules.")
		}
	}

	cc.MutateImage(mctx, mod)
}
