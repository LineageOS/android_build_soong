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

// This file contains image variant related things, including image mutator functions, utility
// functions to determine where a module is installed, etc.

import (
	"fmt"
	"reflect"
	"strings"

	"android/soong/android"
	"android/soong/snapshot"

	"github.com/google/blueprint/proptools"
)

var _ android.ImageInterface = (*Module)(nil)

type ImageVariantType string

const (
	coreImageVariant          ImageVariantType = "core"
	vendorImageVariant        ImageVariantType = "vendor"
	productImageVariant       ImageVariantType = "product"
	ramdiskImageVariant       ImageVariantType = "ramdisk"
	vendorRamdiskImageVariant ImageVariantType = "vendor_ramdisk"
	recoveryImageVariant      ImageVariantType = "recovery"
	hostImageVariant          ImageVariantType = "host"
)

const (
	// VendorVariation is the variant name used for /vendor code that does not
	// compile against the VNDK.
	VendorVariation = "vendor"

	// VendorVariationPrefix is the variant prefix used for /vendor code that compiles
	// against the VNDK.
	VendorVariationPrefix = "vendor."

	// ProductVariation is the variant name used for /product code that does not
	// compile against the VNDK.
	ProductVariation = "product"

	// ProductVariationPrefix is the variant prefix used for /product code that compiles
	// against the VNDK.
	ProductVariationPrefix = "product."
)

func (ctx *moduleContextImpl) inProduct() bool {
	return ctx.mod.InProduct()
}

func (ctx *moduleContextImpl) inVendor() bool {
	return ctx.mod.InVendor()
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

func (c *Module) InstallInProduct() bool {
	// Additionally check if this module is inProduct() that means it is a "product" variant of a
	// module. As well as product specific modules, product variants must be installed to /product.
	return c.InProduct()
}

func (c *Module) InstallInVendor() bool {
	// Additionally check if this module is inVendor() that means it is a "vendor" variant of a
	// module. As well as SoC specific modules, vendor variants must be installed to /vendor
	// unless they have "odm_available: true".
	return c.HasVendorVariant() && c.InVendor() && !c.VendorVariantToOdm()
}

func (c *Module) InstallInOdm() bool {
	// Some vendor variants want to be installed to /odm by setting "odm_available: true".
	return c.InVendor() && c.VendorVariantToOdm()
}

// Returns true when this module is configured to have core and vendor variants.
func (c *Module) HasVendorVariant() bool {
	return Bool(c.VendorProperties.Vendor_available) || Bool(c.VendorProperties.Odm_available)
}

// Returns true when this module creates a vendor variant and wants to install the vendor variant
// to the odm partition.
func (c *Module) VendorVariantToOdm() bool {
	return Bool(c.VendorProperties.Odm_available)
}

// Returns true when this module is configured to have core and product variants.
func (c *Module) HasProductVariant() bool {
	return Bool(c.VendorProperties.Product_available)
}

// Returns true when this module is configured to have core and either product or vendor variants.
func (c *Module) HasNonSystemVariants() bool {
	return c.HasVendorVariant() || c.HasProductVariant()
}

// Returns true if the module is "product" variant. Usually these modules are installed in /product
func (c *Module) InProduct() bool {
	return c.Properties.ImageVariation == ProductVariation
}

// Returns true if the module is "vendor" variant. Usually these modules are installed in /vendor
func (c *Module) InVendor() bool {
	return c.Properties.ImageVariation == VendorVariation
}

// Returns true if the module is "vendor" or "product" variant. This replaces previous UseVndk usages
// which were misused to check if the module variant is vendor or product.
func (c *Module) InVendorOrProduct() bool {
	return c.InVendor() || c.InProduct()
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
	if !c.IsVndk() && !Bool(c.VendorProperties.Product_available) {
		panic(fmt.Errorf("This is only for product available VNDK libs. %q is not a VNDK library or not product available", c.Name()))
	}
	for _, properties := range c.GetProperties() {
		if !visitPropsAndCompareVendorAndProductProps(reflect.ValueOf(properties).Elem()) {
			return false
		}
	}
	return true
}

// ImageMutatableModule provides a common image mutation interface for  LinkableInterface modules.
type ImageMutatableModule interface {
	android.Module
	LinkableInterface

	// AndroidModuleBase returns the android.ModuleBase for this module
	AndroidModuleBase() *android.ModuleBase

	// VendorAvailable returns true if this module is available on the vendor image.
	VendorAvailable() bool

	// OdmAvailable returns true if this module is available on the odm image.
	OdmAvailable() bool

	// ProductAvailable returns true if this module is available on the product image.
	ProductAvailable() bool

	// RamdiskAvailable returns true if this module is available on the ramdisk image.
	RamdiskAvailable() bool

	// RecoveryAvailable returns true if this module is available on the recovery image.
	RecoveryAvailable() bool

	// VendorRamdiskAvailable returns true if this module is available on the vendor ramdisk image.
	VendorRamdiskAvailable() bool

	// IsSnapshotPrebuilt returns true if this module is a snapshot prebuilt.
	IsSnapshotPrebuilt() bool

	// SnapshotVersion returns the snapshot version for this module.
	SnapshotVersion(mctx android.BaseModuleContext) string

	// SdkVersion returns the SDK version for this module.
	SdkVersion() string

	// ExtraVariants returns the list of extra variants this module requires.
	ExtraVariants() []string

	// AppendExtraVariant returns an extra variant to the list of extra variants this module requires.
	AppendExtraVariant(extraVariant string)

	// SetRamdiskVariantNeeded sets whether the Ramdisk Variant is needed.
	SetRamdiskVariantNeeded(b bool)

	// SetVendorRamdiskVariantNeeded sets whether the Vendor Ramdisk Variant is needed.
	SetVendorRamdiskVariantNeeded(b bool)

	// SetRecoveryVariantNeeded sets whether the Recovery Variant is needed.
	SetRecoveryVariantNeeded(b bool)

	// SetCoreVariantNeeded sets whether the Core Variant is needed.
	SetCoreVariantNeeded(b bool)
}

var _ ImageMutatableModule = (*Module)(nil)

func (m *Module) ImageMutatorBegin(mctx android.BaseModuleContext) {
	m.CheckVndkProperties(mctx)
	MutateImage(mctx, m)
}

// CheckVndkProperties checks whether the VNDK-related properties are set correctly.
// If properties are not set correctly, results in a module context property error.
func (m *Module) CheckVndkProperties(mctx android.BaseModuleContext) {
	vendorSpecific := mctx.SocSpecific() || mctx.DeviceSpecific()
	productSpecific := mctx.ProductSpecific()

	if vndkdep := m.vndkdep; vndkdep != nil {
		if vndkdep.isVndk() {
			if vendorSpecific || productSpecific {
				if !vndkdep.isVndkExt() {
					mctx.PropertyErrorf("vndk",
						"must set `extends: \"...\"` to vndk extension")
				} else if Bool(m.VendorProperties.Vendor_available) {
					mctx.PropertyErrorf("vendor_available",
						"must not set at the same time as `vndk: {extends: \"...\"}`")
				} else if Bool(m.VendorProperties.Product_available) {
					mctx.PropertyErrorf("product_available",
						"must not set at the same time as `vndk: {extends: \"...\"}`")
				}
			} else {
				if vndkdep.isVndkExt() {
					mctx.PropertyErrorf("vndk",
						"must set `vendor: true` or `product_specific: true` to set `extends: %q`",
						m.getVndkExtendsModuleName())
				}
				if !Bool(m.VendorProperties.Vendor_available) {
					mctx.PropertyErrorf("vndk",
						"vendor_available must be set to true when `vndk: {enabled: true}`")
				}
				if Bool(m.VendorProperties.Product_available) {
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
}

func (m *Module) VendorAvailable() bool {
	return Bool(m.VendorProperties.Vendor_available)
}

func (m *Module) OdmAvailable() bool {
	return Bool(m.VendorProperties.Odm_available)
}

func (m *Module) ProductAvailable() bool {
	return Bool(m.VendorProperties.Product_available)
}

func (m *Module) RamdiskAvailable() bool {
	return Bool(m.Properties.Ramdisk_available)
}

func (m *Module) VendorRamdiskAvailable() bool {
	return Bool(m.Properties.Vendor_ramdisk_available)
}

func (m *Module) AndroidModuleBase() *android.ModuleBase {
	return &m.ModuleBase
}

func (m *Module) RecoveryAvailable() bool {
	return Bool(m.Properties.Recovery_available)
}

func (m *Module) ExtraVariants() []string {
	return m.Properties.ExtraVersionedImageVariations
}

func (m *Module) AppendExtraVariant(extraVariant string) {
	m.Properties.ExtraVersionedImageVariations = append(m.Properties.ExtraVersionedImageVariations, extraVariant)
}

func (m *Module) SetRamdiskVariantNeeded(b bool) {
	m.Properties.RamdiskVariantNeeded = b
}

func (m *Module) SetVendorRamdiskVariantNeeded(b bool) {
	m.Properties.VendorRamdiskVariantNeeded = b
}

func (m *Module) SetRecoveryVariantNeeded(b bool) {
	m.Properties.RecoveryVariantNeeded = b
}

func (m *Module) SetCoreVariantNeeded(b bool) {
	m.Properties.CoreVariantNeeded = b
}

func (m *Module) SnapshotVersion(mctx android.BaseModuleContext) string {
	if snapshot, ok := m.linker.(SnapshotInterface); ok {
		return snapshot.Version()
	} else {
		mctx.ModuleErrorf("version is unknown for snapshot prebuilt")
		// Should we be panicking here instead?
		return ""
	}
}

func (m *Module) KernelHeadersDecorator() bool {
	if _, ok := m.linker.(*kernelHeadersDecorator); ok {
		return true
	}
	return false
}

// MutateImage handles common image mutations for ImageMutatableModule interfaces.
func MutateImage(mctx android.BaseModuleContext, m ImageMutatableModule) {
	// Validation check
	vendorSpecific := mctx.SocSpecific() || mctx.DeviceSpecific()
	productSpecific := mctx.ProductSpecific()

	if m.VendorAvailable() {
		if vendorSpecific {
			mctx.PropertyErrorf("vendor_available",
				"doesn't make sense at the same time as `vendor: true`, `proprietary: true`, or `device_specific: true`")
		}
		if m.OdmAvailable() {
			mctx.PropertyErrorf("vendor_available",
				"doesn't make sense at the same time as `odm_available: true`")
		}
	}

	if m.OdmAvailable() {
		if vendorSpecific {
			mctx.PropertyErrorf("odm_available",
				"doesn't make sense at the same time as `vendor: true`, `proprietary: true`, or `device_specific: true`")
		}
	}

	if m.ProductAvailable() {
		if productSpecific {
			mctx.PropertyErrorf("product_available",
				"doesn't make sense at the same time as `product_specific: true`")
		}
		if vendorSpecific {
			mctx.PropertyErrorf("product_available",
				"cannot provide product variant from a vendor module. Please use `product_specific: true` with `vendor_available: true`")
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
	recoverySnapshotVersion := mctx.DeviceConfig().RecoverySnapshotVersion()
	usingRecoverySnapshot := recoverySnapshotVersion != "current" &&
		recoverySnapshotVersion != ""
	needVndkVersionVendorVariantForLlndk := false
	if boardVndkVersion != "" {
		boardVndkApiLevel, err := android.ApiLevelFromUser(mctx, boardVndkVersion)
		if err == nil && !boardVndkApiLevel.IsPreview() {
			// VNDK snapshot newer than v30 has LLNDK stub libraries.
			// Only the VNDK version less than or equal to v30 requires generating the vendor
			// variant of the VNDK version from the source tree.
			needVndkVersionVendorVariantForLlndk = boardVndkApiLevel.LessThanOrEqualTo(android.ApiLevelOrPanic(mctx, "30"))
		}
	}
	if boardVndkVersion == "current" {
		boardVndkVersion = platformVndkVersion
	}

	if m.NeedsLlndkVariants() {
		// This is an LLNDK library.  The implementation of the library will be on /system,
		// and vendor and product variants will be created with LLNDK stubs.
		// The LLNDK libraries need vendor variants even if there is no VNDK.
		coreVariantNeeded = true
		vendorVariants = append(vendorVariants, platformVndkVersion)
		productVariants = append(productVariants, platformVndkVersion)
		// Generate vendor variants for boardVndkVersion only if the VNDK snapshot does not
		// provide the LLNDK stub libraries.
		if needVndkVersionVendorVariantForLlndk {
			vendorVariants = append(vendorVariants, boardVndkVersion)
		}
	} else if m.NeedsVendorPublicLibraryVariants() {
		// A vendor public library has the implementation on /vendor, with stub variants
		// for system and product.
		coreVariantNeeded = true
		vendorVariants = append(vendorVariants, boardVndkVersion)
		productVariants = append(productVariants, platformVndkVersion)
	} else if m.IsSnapshotPrebuilt() {
		// Make vendor variants only for the versions in BOARD_VNDK_VERSION and
		// PRODUCT_EXTRA_VNDK_VERSIONS.
		if m.InstallInRecovery() {
			recoveryVariantNeeded = true
		} else {
			vendorVariants = append(vendorVariants, m.SnapshotVersion(mctx))
		}
	} else if m.HasNonSystemVariants() && !m.IsVndkExt() {
		// This will be available to /system unless it is product_specific
		// which will be handled later.
		coreVariantNeeded = true

		// We assume that modules under proprietary paths are compatible for
		// BOARD_VNDK_VERSION. The other modules are regarded as AOSP, or
		// PLATFORM_VNDK_VERSION.
		if m.HasVendorVariant() {
			if snapshot.IsVendorProprietaryModule(mctx) {
				vendorVariants = append(vendorVariants, boardVndkVersion)
			} else {
				vendorVariants = append(vendorVariants, platformVndkVersion)
			}
		}

		// product_available modules are available to /product.
		if m.HasProductVariant() {
			productVariants = append(productVariants, platformVndkVersion)
		}
	} else if vendorSpecific && m.SdkVersion() == "" {
		// This will be available in /vendor (or /odm) only

		// kernel_headers is a special module type whose exported headers
		// are coming from DeviceKernelHeaders() which is always vendor
		// dependent. They'll always have both vendor variants.
		// For other modules, we assume that modules under proprietary
		// paths are compatible for BOARD_VNDK_VERSION. The other modules
		// are regarded as AOSP, which is PLATFORM_VNDK_VERSION.
		if m.KernelHeadersDecorator() {
			vendorVariants = append(vendorVariants,
				platformVndkVersion,
				boardVndkVersion,
			)
		} else if snapshot.IsVendorProprietaryModule(mctx) {
			vendorVariants = append(vendorVariants, boardVndkVersion)
		} else {
			vendorVariants = append(vendorVariants, platformVndkVersion)
		}
	} else {
		// This is either in /system (or similar: /data), or is a
		// module built with the NDK. Modules built with the NDK
		// will be restricted using the existing link type checks.
		coreVariantNeeded = true
	}

	if coreVariantNeeded && productSpecific && m.SdkVersion() == "" {
		// The module has "product_specific: true" that does not create core variant.
		coreVariantNeeded = false
		productVariants = append(productVariants, platformVndkVersion)
	}

	if m.RamdiskAvailable() {
		ramdiskVariantNeeded = true
	}

	if m.AndroidModuleBase().InstallInRamdisk() {
		ramdiskVariantNeeded = true
		coreVariantNeeded = false
	}

	if m.VendorRamdiskAvailable() {
		vendorRamdiskVariantNeeded = true
	}

	if m.AndroidModuleBase().InstallInVendorRamdisk() {
		vendorRamdiskVariantNeeded = true
		coreVariantNeeded = false
	}

	if m.RecoveryAvailable() {
		recoveryVariantNeeded = true
	}

	if m.AndroidModuleBase().InstallInRecovery() {
		recoveryVariantNeeded = true
		coreVariantNeeded = false
	}

	// If using a snapshot, the recovery variant under AOSP directories is not needed,
	// except for kernel headers, which needs all variants.
	if !m.KernelHeadersDecorator() &&
		!m.IsSnapshotPrebuilt() &&
		usingRecoverySnapshot &&
		!snapshot.IsRecoveryProprietaryModule(mctx) {
		recoveryVariantNeeded = false
	}

	for _, variant := range android.FirstUniqueStrings(vendorVariants) {
		if variant == "" {
			m.AppendExtraVariant(VendorVariation)
		} else {
			m.AppendExtraVariant(VendorVariationPrefix + variant)
		}
	}

	for _, variant := range android.FirstUniqueStrings(productVariants) {
		if variant == "" {
			m.AppendExtraVariant(ProductVariation)
		} else {
			m.AppendExtraVariant(ProductVariationPrefix + variant)
		}
	}

	m.SetRamdiskVariantNeeded(ramdiskVariantNeeded)
	m.SetVendorRamdiskVariantNeeded(vendorRamdiskVariantNeeded)
	m.SetRecoveryVariantNeeded(recoveryVariantNeeded)
	m.SetCoreVariantNeeded(coreVariantNeeded)

	// Disable the module if no variants are needed.
	if !ramdiskVariantNeeded &&
		!recoveryVariantNeeded &&
		!coreVariantNeeded &&
		len(m.ExtraVariants()) == 0 {
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

func (c *Module) DebugRamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return false
}

func (c *Module) RecoveryVariantNeeded(ctx android.BaseModuleContext) bool {
	return c.Properties.RecoveryVariantNeeded
}

func (c *Module) ExtraImageVariations(ctx android.BaseModuleContext) []string {
	return c.Properties.ExtraVersionedImageVariations
}

func squashVendorSrcs(m *Module) {
	if lib, ok := m.compiler.(*libraryDecorator); ok {
		lib.baseCompiler.Properties.Srcs = append(lib.baseCompiler.Properties.Srcs,
			lib.baseCompiler.Properties.Target.Vendor.Srcs...)

		lib.baseCompiler.Properties.Exclude_srcs = append(lib.baseCompiler.Properties.Exclude_srcs,
			lib.baseCompiler.Properties.Target.Vendor.Exclude_srcs...)

		lib.baseCompiler.Properties.Exclude_generated_sources = append(lib.baseCompiler.Properties.Exclude_generated_sources,
			lib.baseCompiler.Properties.Target.Vendor.Exclude_generated_sources...)

		if lib.Properties.Target.Vendor.No_stubs {
			proptools.Clear(&lib.Properties.Stubs)
		}
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

		if lib.Properties.Target.Product.No_stubs {
			proptools.Clear(&lib.Properties.Stubs)
		}
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

func squashRamdiskSrcs(m *Module) {
	if lib, ok := m.compiler.(*libraryDecorator); ok {
		lib.baseCompiler.Properties.Exclude_srcs = append(lib.baseCompiler.Properties.Exclude_srcs, lib.baseCompiler.Properties.Target.Ramdisk.Exclude_srcs...)
	}
}

func (c *Module) SetImageVariation(ctx android.BaseModuleContext, variant string, module android.Module) {
	m := module.(*Module)
	if variant == android.RamdiskVariation {
		m.MakeAsPlatform()
		squashRamdiskSrcs(m)
	} else if variant == android.VendorRamdiskVariation {
		m.MakeAsPlatform()
		squashVendorRamdiskSrcs(m)
	} else if variant == android.RecoveryVariation {
		m.MakeAsPlatform()
		squashRecoverySrcs(m)
	} else if strings.HasPrefix(variant, VendorVariation) {
		m.Properties.ImageVariation = VendorVariation

		if strings.HasPrefix(variant, VendorVariationPrefix) {
			m.Properties.VndkVersion = strings.TrimPrefix(variant, VendorVariationPrefix)
		}
		squashVendorSrcs(m)

		// Makefile shouldn't know vendor modules other than BOARD_VNDK_VERSION.
		// Hide other vendor variants to avoid collision.
		vndkVersion := ctx.DeviceConfig().VndkVersion()
		if vndkVersion != "current" && vndkVersion != "" && vndkVersion != m.Properties.VndkVersion {
			m.Properties.HideFromMake = true
			m.HideFromMake()
		}
	} else if strings.HasPrefix(variant, ProductVariation) {
		m.Properties.ImageVariation = ProductVariation
		if strings.HasPrefix(variant, ProductVariationPrefix) {
			m.Properties.VndkVersion = strings.TrimPrefix(variant, ProductVariationPrefix)
		}
		squashProductSrcs(m)
	}

	if c.NeedsVendorPublicLibraryVariants() &&
		(variant == android.CoreVariation || strings.HasPrefix(variant, ProductVariationPrefix)) {
		c.VendorProperties.IsVendorPublicLibrary = true
	}
}
