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

func (mod *Module) DebugRamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return false
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

func (ctx *moduleContext) SocSpecific() bool {
	// Additionally check if this module is inVendor() that means it is a "vendor" variant of a
	// module. As well as SoC specific modules, vendor variants must be installed to /vendor
	// unless they have "odm_available: true".
	return ctx.ModuleContext.SocSpecific() || (ctx.RustModule().InVendor() && !ctx.RustModule().VendorVariantToOdm())
}

func (ctx *moduleContext) DeviceSpecific() bool {
	// Some vendor variants want to be installed to /odm by setting "odm_available: true".
	return ctx.ModuleContext.DeviceSpecific() || (ctx.RustModule().InVendor() && ctx.RustModule().VendorVariantToOdm())
}

// Returns true when this module creates a vendor variant and wants to install the vendor variant
// to the odm partition.
func (c *Module) VendorVariantToOdm() bool {
	return Bool(c.VendorProperties.Odm_available)
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

// Returns true if the module is "vendor" variant. Usually these modules are installed in /vendor
func (mod *Module) InVendor() bool {
	return mod.Properties.ImageVariationPrefix == cc.VendorVariationPrefix
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
	if Bool(mod.Properties.Vendor_ramdisk_available) {
		if lib, ok := mod.compiler.(libraryInterface); !ok || (ok && lib.buildShared()) {
			mctx.PropertyErrorf("vendor_ramdisk_available", "cannot be set for rust_ffi or rust_ffi_shared modules.")
		}
	}
	vendorSpecific := mctx.SocSpecific() || mctx.DeviceSpecific()
	if vendorSpecific {
		mctx.PropertyErrorf("vendor or soc_specific",
			"Rust modules do not yet support soc-specific modules")

	}

	cc.MutateImage(mctx, mod)

	if !mod.Properties.CoreVariantNeeded || mod.HasNonSystemVariants() {

		if _, ok := mod.compiler.(*prebuiltLibraryDecorator); ok {
			// Rust does not support prebuilt libraries on non-System images.
			mctx.ModuleErrorf("Rust prebuilt modules not supported for non-system images.")
		}
	}
}
