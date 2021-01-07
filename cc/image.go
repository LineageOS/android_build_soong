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
	"fmt"
	"reflect"
	"strings"

	"android/soong/android"
)

var _ android.ImageInterface = (*Module)(nil)

type imageVariantType string

const (
	coreImageVariant          imageVariantType = "core"
	vendorImageVariant        imageVariantType = "vendor"
	productImageVariant       imageVariantType = "product"
	ramdiskImageVariant       imageVariantType = "ramdisk"
	vendorRamdiskImageVariant imageVariantType = "vendor_ramdisk"
	recoveryImageVariant      imageVariantType = "recovery"
	hostImageVariant          imageVariantType = "host"
)

func (c *Module) getImageVariantType() imageVariantType {
	if c.Host() {
		return hostImageVariant
	} else if c.inVendor() {
		return vendorImageVariant
	} else if c.InProduct() {
		return productImageVariant
	} else if c.InRamdisk() {
		return ramdiskImageVariant
	} else if c.InVendorRamdisk() {
		return vendorRamdiskImageVariant
	} else if c.InRecovery() {
		return recoveryImageVariant
	} else {
		return coreImageVariant
	}
}

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
		(ctx.mod.HasProductVariant() && ctx.mod.InProduct())
}

func (ctx *moduleContext) SocSpecific() bool {
	return ctx.ModuleContext.SocSpecific() ||
		(ctx.mod.HasVendorVariant() && ctx.mod.inVendor())
}

func (ctx *moduleContextImpl) inProduct() bool {
	return ctx.mod.InProduct()
}

func (ctx *moduleContextImpl) inVendor() bool {
	return ctx.mod.inVendor()
}

func (ctx *moduleContextImpl) inRamdisk() bool {
	return ctx.mod.InRamdisk()
}

func (ctx *moduleContextImpl) inVendorRamdisk() bool {
	return ctx.mod.InVendorRamdisk()
}

func (ctx *moduleContextImpl) inRecovery() bool {
	return ctx.mod.InRecovery()
}

// Returns true when this module is configured to have core and vendor variants.
func (c *Module) HasVendorVariant() bool {
	// In case of a VNDK, 'vendor_available: false' still creates a vendor variant.
	return c.IsVndk() || Bool(c.VendorProperties.Vendor_available)
}

// Returns true when this module is configured to have core and product variants.
func (c *Module) HasProductVariant() bool {
	if c.VendorProperties.Product_available == nil {
		// Without 'product_available', product variant will not be created even for VNDKs.
		return false
	}
	// However, 'product_available: false' in a VNDK still creates a product variant.
	return c.IsVndk() || Bool(c.VendorProperties.Product_available)
}

// Returns true when this module is configured to have core and either product or vendor variants.
func (c *Module) HasNonSystemVariants() bool {
	return c.HasVendorVariant() || c.HasProductVariant()
}

// Returns true if the module is "product" variant. Usually these modules are installed in /product
func (c *Module) InProduct() bool {
	return c.Properties.ImageVariationPrefix == ProductVariationPrefix
}

// Returns true if the module is "vendor" variant. Usually these modules are installed in /vendor
func (c *Module) inVendor() bool {
	return c.Properties.ImageVariationPrefix == VendorVariationPrefix
}

func (c *Module) InRamdisk() bool {
	return c.ModuleBase.InRamdisk() || c.ModuleBase.InstallInRamdisk()
}

func (c *Module) InVendorRamdisk() bool {
	return c.ModuleBase.InVendorRamdisk() || c.ModuleBase.InstallInVendorRamdisk()
}

func (c *Module) InRecovery() bool {
	return c.ModuleBase.InRecovery() || c.ModuleBase.InstallInRecovery()
}

func (c *Module) OnlyInRamdisk() bool {
	return c.ModuleBase.InstallInRamdisk()
}

func (c *Module) OnlyInVendorRamdisk() bool {
	return c.ModuleBase.InstallInVendorRamdisk()
}

func (c *Module) OnlyInRecovery() bool {
	return c.ModuleBase.InstallInRecovery()
}

func visitPropsAndCompareVendorAndProductProps(v reflect.Value) bool {
	if v.Kind() != reflect.Struct {
		return true
	}
	for i := 0; i < v.NumField(); i++ {
		prop := v.Field(i)
		if prop.Kind() == reflect.Struct && v.Type().Field(i).Name == "Target" {
			vendor_prop := prop.FieldByName("Vendor")
			product_prop := prop.FieldByName("Product")
			if vendor_prop.Kind() != reflect.Struct && product_prop.Kind() != reflect.Struct {
				// Neither Target.Vendor nor Target.Product is defined
				continue
			}
			if vendor_prop.Kind() != reflect.Struct || product_prop.Kind() != reflect.Struct ||
				!reflect.DeepEqual(vendor_prop.Interface(), product_prop.Interface()) {
				// If only one of either Target.Vendor or Target.Product is
				// defined or they have different values, it fails the build
				// since VNDK must have the same properties for both vendor
				// and product variants.
				return false
			}
		} else if !visitPropsAndCompareVendorAndProductProps(prop) {
			// Visit the substructures to find Target.Vendor and Target.Product
			return false
		}
	}
	return true
}

// In the case of VNDK, vendor and product variants must have the same properties.
// VNDK installs only one file and shares it for both vendor and product modules on
// runtime. We may not define different versions of a VNDK lib for each partition.
// This function is used only for the VNDK modules that is available to both vendor
// and product partitions.
func (c *Module) compareVendorAndProductProps() bool {
	if !c.IsVndk() && c.VendorProperties.Product_available != nil {
		panic(fmt.Errorf("This is only for product available VNDK libs. %q is not a VNDK library or not product available", c.Name()))
	}
	for _, properties := range c.GetProperties() {
		if !visitPropsAndCompareVendorAndProductProps(reflect.ValueOf(properties).Elem()) {
			return false
		}
	}
	return true
}

func (m *Module) ImageMutatorBegin(mctx android.BaseModuleContext) {
	// Validation check
	vendorSpecific := mctx.SocSpecific() || mctx.DeviceSpecific()
	productSpecific := mctx.ProductSpecific()

	if m.VendorProperties.Vendor_available != nil {
		if vendorSpecific {
			mctx.PropertyErrorf("vendor_available",
				"doesn't make sense at the same time as `vendor: true`, `proprietary: true`, or `device_specific:true`")
		}
	}

	if m.VendorProperties.Product_available != nil {
		if productSpecific {
			mctx.PropertyErrorf("product_available",
				"doesn't make sense at the same time as `product_specific: true`")
		}
		if vendorSpecific {
			mctx.PropertyErrorf("product_available",
				"cannot provide product variant from a vendor module. Please use `product_specific: true` with `vendor_available: true`")
		}
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
				} else if m.VendorProperties.Product_available != nil {
					mctx.PropertyErrorf("product_available",
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
				if m.VendorProperties.Product_available != nil {
					// If a VNDK module creates both product and vendor variants, they
					// must have the same properties since they share a single VNDK
					// library on runtime.
					if !m.compareVendorAndProductProps() {
						mctx.ModuleErrorf("product properties must have the same values with the vendor properties for VNDK modules")
					}
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
	var vendorRamdiskVariantNeeded bool = false
	var recoveryVariantNeeded bool = false

	var vendorVariants []string
	var productVariants []string

	platformVndkVersion := mctx.DeviceConfig().PlatformVndkVersion()
	boardVndkVersion := mctx.DeviceConfig().VndkVersion()
	productVndkVersion := mctx.DeviceConfig().ProductVndkVersion()
	recoverySnapshotVersion := mctx.DeviceConfig().RecoverySnapshotVersion()
	usingRecoverySnapshot := recoverySnapshotVersion != "current" &&
		recoverySnapshotVersion != ""
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
			if m.InstallInRecovery() {
				recoveryVariantNeeded = true
			} else {
				vendorVariants = append(vendorVariants, snapshot.version())
			}
		} else {
			mctx.ModuleErrorf("version is unknown for snapshot prebuilt")
		}
	} else if m.HasNonSystemVariants() && !m.IsVndkExt() {
		// This will be available to /system unless it is product_specific
		// which will be handled later.
		coreVariantNeeded = true

		// We assume that modules under proprietary paths are compatible for
		// BOARD_VNDK_VERSION. The other modules are regarded as AOSP, or
		// PLATFORM_VNDK_VERSION.
		if m.HasVendorVariant() {
			if isVendorProprietaryModule(mctx) {
				vendorVariants = append(vendorVariants, boardVndkVersion)
			} else {
				vendorVariants = append(vendorVariants, platformVndkVersion)
			}
		}

		// product_available modules are available to /product.
		if m.HasProductVariant() {
			productVariants = append(productVariants, platformVndkVersion)
			// VNDK is always PLATFORM_VNDK_VERSION
			if !m.IsVndk() {
				productVariants = append(productVariants, productVndkVersion)
			}
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
		} else if isVendorProprietaryModule(mctx) {
			vendorVariants = append(vendorVariants, boardVndkVersion)
		} else {
			vendorVariants = append(vendorVariants, platformVndkVersion)
		}
	} else if lib := moduleLibraryInterface(m); lib != nil && lib.hasLLNDKStubs() {
		// This is an LLNDK library.  The implementation of the library will be on /system,
		// and vendor and product variants will be created with LLNDK stubs.
		coreVariantNeeded = true
		vendorVariants = append(vendorVariants,
			platformVndkVersion,
			boardVndkVersion,
		)
		productVariants = append(productVariants,
			platformVndkVersion,
			productVndkVersion,
		)
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

	if Bool(m.Properties.Vendor_ramdisk_available) {
		vendorRamdiskVariantNeeded = true
	}

	if m.ModuleBase.InstallInVendorRamdisk() {
		vendorRamdiskVariantNeeded = true
		coreVariantNeeded = false
	}

	if Bool(m.Properties.Recovery_available) {
		recoveryVariantNeeded = true
	}

	if m.ModuleBase.InstallInRecovery() {
		recoveryVariantNeeded = true
		coreVariantNeeded = false
	}

	// If using a snapshot, the recovery variant under AOSP directories is not needed,
	// except for kernel headers, which needs all variants.
	if _, ok := m.linker.(*kernelHeadersDecorator); !ok &&
		!m.isSnapshotPrebuilt() &&
		usingRecoverySnapshot &&
		!isRecoveryProprietaryModule(mctx) {
		recoveryVariantNeeded = false
	}

	for _, variant := range android.FirstUniqueStrings(vendorVariants) {
		m.Properties.ExtraVariants = append(m.Properties.ExtraVariants, VendorVariationPrefix+variant)
	}

	for _, variant := range android.FirstUniqueStrings(productVariants) {
		m.Properties.ExtraVariants = append(m.Properties.ExtraVariants, ProductVariationPrefix+variant)
	}

	m.Properties.RamdiskVariantNeeded = ramdiskVariantNeeded
	m.Properties.VendorRamdiskVariantNeeded = vendorRamdiskVariantNeeded
	m.Properties.RecoveryVariantNeeded = recoveryVariantNeeded
	m.Properties.CoreVariantNeeded = coreVariantNeeded

	// Disable the module if no variants are needed.
	if !ramdiskVariantNeeded &&
		!recoveryVariantNeeded &&
		!coreVariantNeeded &&
		len(m.Properties.ExtraVariants) == 0 {
		m.Disable()
	}
}

func (c *Module) CoreVariantNeeded(ctx android.BaseModuleContext) bool {
	return c.Properties.CoreVariantNeeded
}

func (c *Module) RamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return c.Properties.RamdiskVariantNeeded
}

func (c *Module) VendorRamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return c.Properties.VendorRamdiskVariantNeeded
}

func (c *Module) RecoveryVariantNeeded(ctx android.BaseModuleContext) bool {
	return c.Properties.RecoveryVariantNeeded
}

func (c *Module) ExtraImageVariations(ctx android.BaseModuleContext) []string {
	return c.Properties.ExtraVariants
}

func squashVendorSrcs(m *Module) {
	if lib, ok := m.compiler.(*libraryDecorator); ok {
		lib.baseCompiler.Properties.Srcs = append(lib.baseCompiler.Properties.Srcs,
			lib.baseCompiler.Properties.Target.Vendor.Srcs...)

		lib.baseCompiler.Properties.Exclude_srcs = append(lib.baseCompiler.Properties.Exclude_srcs,
			lib.baseCompiler.Properties.Target.Vendor.Exclude_srcs...)

		lib.baseCompiler.Properties.Exclude_generated_sources = append(lib.baseCompiler.Properties.Exclude_generated_sources,
			lib.baseCompiler.Properties.Target.Vendor.Exclude_generated_sources...)
	}
}

func squashProductSrcs(m *Module) {
	if lib, ok := m.compiler.(*libraryDecorator); ok {
		lib.baseCompiler.Properties.Srcs = append(lib.baseCompiler.Properties.Srcs,
			lib.baseCompiler.Properties.Target.Product.Srcs...)

		lib.baseCompiler.Properties.Exclude_srcs = append(lib.baseCompiler.Properties.Exclude_srcs,
			lib.baseCompiler.Properties.Target.Product.Exclude_srcs...)

		lib.baseCompiler.Properties.Exclude_generated_sources = append(lib.baseCompiler.Properties.Exclude_generated_sources,
			lib.baseCompiler.Properties.Target.Product.Exclude_generated_sources...)
	}
}

func squashRecoverySrcs(m *Module) {
	if lib, ok := m.compiler.(*libraryDecorator); ok {
		lib.baseCompiler.Properties.Srcs = append(lib.baseCompiler.Properties.Srcs,
			lib.baseCompiler.Properties.Target.Recovery.Srcs...)

		lib.baseCompiler.Properties.Exclude_srcs = append(lib.baseCompiler.Properties.Exclude_srcs,
			lib.baseCompiler.Properties.Target.Recovery.Exclude_srcs...)

		lib.baseCompiler.Properties.Exclude_generated_sources = append(lib.baseCompiler.Properties.Exclude_generated_sources,
			lib.baseCompiler.Properties.Target.Recovery.Exclude_generated_sources...)
	}
}

func squashVendorRamdiskSrcs(m *Module) {
	if lib, ok := m.compiler.(*libraryDecorator); ok {
		lib.baseCompiler.Properties.Exclude_srcs = append(lib.baseCompiler.Properties.Exclude_srcs, lib.baseCompiler.Properties.Target.Vendor_ramdisk.Exclude_srcs...)
	}
}

func (c *Module) SetImageVariation(ctx android.BaseModuleContext, variant string, module android.Module) {
	m := module.(*Module)
	if variant == android.RamdiskVariation {
		m.MakeAsPlatform()
	} else if variant == android.VendorRamdiskVariation {
		m.MakeAsPlatform()
		squashVendorRamdiskSrcs(m)
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
			m.HideFromMake()
		}
	} else if strings.HasPrefix(variant, ProductVariationPrefix) {
		m.Properties.ImageVariationPrefix = ProductVariationPrefix
		m.Properties.VndkVersion = strings.TrimPrefix(variant, ProductVariationPrefix)
		squashProductSrcs(m)
	}
}
