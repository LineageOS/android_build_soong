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
	"android/soong/android"
	"android/soong/genrule"
)

func init() {
	android.RegisterModuleType("cc_genrule", genRuleFactory)
}

type GenruleExtraProperties struct {
	Vendor_available   *bool
	Ramdisk_available  *bool
	Recovery_available *bool
	Sdk_version        *string
}

// cc_genrule is a genrule that can depend on other cc_* objects.
// The cmd may be run multiple times, once for each of the different arch/etc
// variations.
func genRuleFactory() android.Module {
	module := genrule.NewGenRule()

	extra := &GenruleExtraProperties{}
	module.Extra = extra
	module.ImageInterface = extra
	module.AddProperties(module.Extra)

	android.InitAndroidArchModule(module, android.HostAndDeviceSupported, android.MultilibBoth)

	android.InitApexModule(module)

	return module
}

var _ android.ImageInterface = (*GenruleExtraProperties)(nil)

func (g *GenruleExtraProperties) ImageMutatorBegin(ctx android.BaseModuleContext) {}

func (g *GenruleExtraProperties) CoreVariantNeeded(ctx android.BaseModuleContext) bool {
	if ctx.DeviceConfig().VndkVersion() == "" {
		return true
	}

	if ctx.DeviceConfig().ProductVndkVersion() != "" && ctx.ProductSpecific() {
		return false
	}

	return Bool(g.Vendor_available) || !(ctx.SocSpecific() || ctx.DeviceSpecific())
}

func (g *GenruleExtraProperties) RamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return Bool(g.Ramdisk_available)
}

func (g *GenruleExtraProperties) RecoveryVariantNeeded(ctx android.BaseModuleContext) bool {
	return Bool(g.Recovery_available)
}

func (g *GenruleExtraProperties) ExtraImageVariations(ctx android.BaseModuleContext) []string {
	if ctx.DeviceConfig().VndkVersion() == "" {
		return nil
	}

	var variants []string
	if Bool(g.Vendor_available) || ctx.SocSpecific() || ctx.DeviceSpecific() {
		vndkVersion := ctx.DeviceConfig().VndkVersion()
		// If vndkVersion is current, we can always use PlatformVndkVersion.
		// If not, we assume modules under proprietary paths are compatible for
		// BOARD_VNDK_VERSION. The other modules are regarded as AOSP, that is
		// PLATFORM_VNDK_VERSION.
		if vndkVersion == "current" || !isVendorProprietaryModule(ctx) {
			variants = append(variants, VendorVariationPrefix+ctx.DeviceConfig().PlatformVndkVersion())
		} else {
			variants = append(variants, VendorVariationPrefix+vndkVersion)
		}
	}

	if ctx.DeviceConfig().ProductVndkVersion() == "" {
		return variants
	}

	if Bool(g.Vendor_available) || ctx.ProductSpecific() {
		variants = append(variants, ProductVariationPrefix+ctx.DeviceConfig().PlatformVndkVersion())
		if vndkVersion := ctx.DeviceConfig().ProductVndkVersion(); vndkVersion != "current" {
			variants = append(variants, ProductVariationPrefix+vndkVersion)
		}
	}

	return variants
}

func (g *GenruleExtraProperties) SetImageVariation(ctx android.BaseModuleContext, variation string, module android.Module) {
}
