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

// This file contains image variant related things, including image mutator functions, utility
// functions to determine where a module is installed, etc.

import (
	"strings"

	"android/soong/android"
)

var _ android.ImageInterface = (*Module)(nil)

const (
	// VendorVariationPrefix is the variant prefix used for /vendor code that compiles
	// against the VNDK.
	VendorVariationPrefix = "vendor."

	// ProductVariationPrefix is the variant prefix used for /product code that compiles
	// against the VNDK.
	ProductVariationPrefix = "product."
)

func (ctx *moduleContext) ProductSpecific() bool {
	return ctx.ModuleContext.ProductSpecific() ||
		(ctx.mod.HasVendorVariant() && ctx.mod.inProduct() && !ctx.mod.IsVndk())
}

func (ctx *moduleContext) SocSpecific() bool {
	return ctx.ModuleContext.SocSpecific() ||
		(ctx.mod.HasVendorVariant() && ctx.mod.inVendor() && !ctx.mod.IsVndk())
}

func (ctx *moduleContextImpl) inProduct() bool {
	return ctx.mod.inProduct()
}

func (ctx *moduleContextImpl) inVendor() bool {
	return ctx.mod.inVendor()
}

func (ctx *moduleContextImpl) inRamdisk() bool {
	return ctx.mod.InRamdisk()
}

func (ctx *moduleContextImpl) inRecovery() bool {
	return ctx.mod.InRecovery()
}

// Returns true only when this module is configured to have core, product and vendor
// variants.
func (c *Module) HasVendorVariant() bool {
	return c.IsVndk() || Bool(c.VendorProperties.Vendor_available)
}

// Returns true if the module is "product" variant. Usually these modules are installed in /product
func (c *Module) inProduct() bool {
	return c.Properties.ImageVariationPrefix == ProductVariationPrefix
}

// Returns true if the module is "vendor" variant. Usually these modules are installed in /vendor
func (c *Module) inVendor() bool {
	return c.Properties.ImageVariationPrefix == VendorVariationPrefix
}

func (c *Module) InRamdisk() bool {
	return c.ModuleBase.InRamdisk() || c.ModuleBase.InstallInRamdisk()
}

func (c *Module) InRecovery() bool {
	return c.ModuleBase.InRecovery() || c.ModuleBase.InstallInRecovery()
}

func (c *Module) OnlyInRamdisk() bool {
	return c.ModuleBase.InstallInRamdisk()
}

func (c *Module) OnlyInRecovery() bool {
	return c.ModuleBase.InstallInRecovery()
}

func (m *Module) ImageMutatorBegin(mctx android.BaseModuleContext) {
	// Validation check
	vendorSpecific := mctx.SocSpecific() || mctx.DeviceSpecific()
	productSpecific := mctx.ProductSpecific()

	if m.VendorProperties.Vendor_available != nil && vendorSpecific {
		mctx.PropertyErrorf("vendor_available",
			"doesn't make sense at the same time as `vendor: true`, `proprietary: true`, or `device_specific:true`")
	}

	if vndkdep := m.vndkdep; vndkdep != nil {
		if vndkdep.isVndk() {
			if vendorSpecific || productSpecific {
				if !vndkdep.isVndkExt() {
					mctx.PropertyErrorf("vndk",
						"must set `extends: \"...\"` to vndk extension")
				} else if m.VendorProperties.Vendor_available != nil {
					mctx.PropertyErrorf("vendor_available",
						"must not set at the same time as `vndk: {extends: \"...\"}`")
				}
			} else {
				if vndkdep.isVndkExt() {
					mctx.PropertyErrorf("vndk",
						"must set `vendor: true` or `product_specific: true` to set `extends: %q`",
						m.getVndkExtendsModuleName())
				}
				if m.VendorProperties.Vendor_available == nil {
					mctx.PropertyErrorf("vndk",
						"vendor_available must be set to either true or false when `vndk: {enabled: true}`")
				}
			}
		} else {
			if vndkdep.isVndkSp() {
				mctx.PropertyErrorf("vndk",
					"must set `enabled: true` to set `support_system_process: true`")
			}
			if vndkdep.isVndkExt() {
				mctx.PropertyErrorf("vndk",
					"must set `enabled: true` to set `extends: %q`",
					m.getVndkExtendsModuleName())
			}
		}
	}

	var coreVariantNeeded bool = false
	var ramdiskVariantNeeded bool = false
	var recoveryVariantNeeded bool = false

	var vendorVariants []string
	var productVariants []string

	platformVndkVersion := mctx.DeviceConfig().PlatformVndkVersion()
	boardVndkVersion := mctx.DeviceConfig().VndkVersion()
	productVndkVersion := mctx.DeviceConfig().ProductVndkVersion()
	if boardVndkVersion == "current" {
		boardVndkVersion = platformVndkVersion
	}
	if productVndkVersion == "current" {
		productVndkVersion = platformVndkVersion
	}

	if boardVndkVersion == "" {
		// If the device isn't compiling against the VNDK, we always
		// use the core mode.
		coreVariantNeeded = true
	} else if _, ok := m.linker.(*llndkStubDecorator); ok {
		// LL-NDK stubs only exist in the vendor and product variants,
		// since the real libraries will be used in the core variant.
		vendorVariants = append(vendorVariants,
			platformVndkVersion,
			boardVndkVersion,
		)
		productVariants = append(productVariants,
			platformVndkVersion,
			productVndkVersion,
		)
	} else if _, ok := m.linker.(*llndkHeadersDecorator); ok {
		// ... and LL-NDK headers as well
		vendorVariants = append(vendorVariants,
			platformVndkVersion,
			boardVndkVersion,
		)
		productVariants = append(productVariants,
			platformVndkVersion,
			productVndkVersion,
		)
	} else if m.isSnapshotPrebuilt() {
		// Make vendor variants only for the versions in BOARD_VNDK_VERSION and
		// PRODUCT_EXTRA_VNDK_VERSIONS.
		if snapshot, ok := m.linker.(interface {
			version() string
		}); ok {
			vendorVariants = append(vendorVariants, snapshot.version())
		} else {
			mctx.ModuleErrorf("version is unknown for snapshot prebuilt")
		}
	} else if m.HasVendorVariant() && !m.isVndkExt() {
		// This will be available in /system, /vendor and /product
		// or a /system directory that is available to vendor and product.
		coreVariantNeeded = true

		// We assume that modules under proprietary paths are compatible for
		// BOARD_VNDK_VERSION. The other modules are regarded as AOSP, or
		// PLATFORM_VNDK_VERSION.
		if isVendorProprietaryPath(mctx.ModuleDir()) {
			vendorVariants = append(vendorVariants, boardVndkVersion)
		} else {
			vendorVariants = append(vendorVariants, platformVndkVersion)
		}

		// vendor_available modules are also available to /product.
		productVariants = append(productVariants, platformVndkVersion)
		// VNDK is always PLATFORM_VNDK_VERSION
		if !m.IsVndk() {
			productVariants = append(productVariants, productVndkVersion)
		}
	} else if vendorSpecific && String(m.Properties.Sdk_version) == "" {
		// This will be available in /vendor (or /odm) only

		// kernel_headers is a special module type whose exported headers
		// are coming from DeviceKernelHeaders() which is always vendor
		// dependent. They'll always have both vendor variants.
		// For other modules, we assume that modules under proprietary
		// paths are compatible for BOARD_VNDK_VERSION. The other modules
		// are regarded as AOSP, which is PLATFORM_VNDK_VERSION.
		if _, ok := m.linker.(*kernelHeadersDecorator); ok {
			vendorVariants = append(vendorVariants,
				platformVndkVersion,
				boardVndkVersion,
			)
		} else if isVendorProprietaryPath(mctx.ModuleDir()) {
			vendorVariants = append(vendorVariants, boardVndkVersion)
		} else {
			vendorVariants = append(vendorVariants, platformVndkVersion)
		}
	} else {
		// This is either in /system (or similar: /data), or is a
		// modules built with the NDK. Modules built with the NDK
		// will be restricted using the existing link type checks.
		coreVariantNeeded = true
	}

	if boardVndkVersion != "" && productVndkVersion != "" {
		if coreVariantNeeded && productSpecific && String(m.Properties.Sdk_version) == "" {
			// The module has "product_specific: true" that does not create core variant.
			coreVariantNeeded = false
			productVariants = append(productVariants, productVndkVersion)
		}
	} else {
		// Unless PRODUCT_PRODUCT_VNDK_VERSION is set, product partition has no
		// restriction to use system libs.
		// No product variants defined in this case.
		productVariants = []string{}
	}

	if Bool(m.Properties.Ramdisk_available) {
		ramdiskVariantNeeded = true
	}

	if m.ModuleBase.InstallInRamdisk() {
		ramdiskVariantNeeded = true
		coreVariantNeeded = false
	}

	if Bool(m.Properties.Recovery_available) {
		recoveryVariantNeeded = true
	}

	if m.ModuleBase.InstallInRecovery() {
		recoveryVariantNeeded = true
		coreVariantNeeded = false
	}

	for _, variant := range android.FirstUniqueStrings(vendorVariants) {
		m.Properties.ExtraVariants = append(m.Properties.ExtraVariants, VendorVariationPrefix+variant)
	}

	for _, variant := range android.FirstUniqueStrings(productVariants) {
		m.Properties.ExtraVariants = append(m.Properties.ExtraVariants, ProductVariationPrefix+variant)
	}

	m.Properties.RamdiskVariantNeeded = ramdiskVariantNeeded
	m.Properties.RecoveryVariantNeeded = recoveryVariantNeeded
	m.Properties.CoreVariantNeeded = coreVariantNeeded
}

func (c *Module) CoreVariantNeeded(ctx android.BaseModuleContext) bool {
	return c.Properties.CoreVariantNeeded
}

func (c *Module) RamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return c.Properties.RamdiskVariantNeeded
}

func (c *Module) RecoveryVariantNeeded(ctx android.BaseModuleContext) bool {
	return c.Properties.RecoveryVariantNeeded
}

func (c *Module) ExtraImageVariations(ctx android.BaseModuleContext) []string {
	return c.Properties.ExtraVariants
}

func (c *Module) SetImageVariation(ctx android.BaseModuleContext, variant string, module android.Module) {
	m := module.(*Module)
	if variant == android.RamdiskVariation {
		m.MakeAsPlatform()
	} else if variant == android.RecoveryVariation {
		m.MakeAsPlatform()
		squashRecoverySrcs(m)
	} else if strings.HasPrefix(variant, VendorVariationPrefix) {
		m.Properties.ImageVariationPrefix = VendorVariationPrefix
		m.Properties.VndkVersion = strings.TrimPrefix(variant, VendorVariationPrefix)
		squashVendorSrcs(m)

		// Makefile shouldn't know vendor modules other than BOARD_VNDK_VERSION.
		// Hide other vendor variants to avoid collision.
		vndkVersion := ctx.DeviceConfig().VndkVersion()
		if vndkVersion != "current" && vndkVersion != "" && vndkVersion != m.Properties.VndkVersion {
			m.Properties.HideFromMake = true
			m.SkipInstall()
		}
	} else if strings.HasPrefix(variant, ProductVariationPrefix) {
		m.Properties.ImageVariationPrefix = ProductVariationPrefix
		m.Properties.VndkVersion = strings.TrimPrefix(variant, ProductVariationPrefix)
		squashVendorSrcs(m)
	}
}
