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
	"fmt"

	"android/soong/android"
	"android/soong/genrule"
	"android/soong/snapshot"
)

func init() {
	android.RegisterModuleType("cc_genrule", GenRuleFactory)
}

type GenruleExtraProperties struct {
	Vendor_available         *bool
	Odm_available            *bool
	Product_available        *bool
	Ramdisk_available        *bool
	Vendor_ramdisk_available *bool
	Recovery_available       *bool
	Sdk_version              *string
}

// cc_genrule is a genrule that can depend on other cc_* objects.
// The cmd may be run multiple times, once for each of the different arch/etc
// variations.  The following environment variables will be set when the command
// execute:
//
//	CC_ARCH           the name of the architecture the command is being executed for
//
//	CC_MULTILIB       "lib32" if the architecture the command is being executed for is 32-bit,
//	                  "lib64" if it is 64-bit.
//
//	CC_NATIVE_BRIDGE  the name of the subdirectory that native bridge libraries are stored in if
//	                  the architecture has native bridge enabled, empty if it is disabled.
//
//	CC_OS             the name of the OS the command is being executed for.
func GenRuleFactory() android.Module {
	module := genrule.NewGenRule()

	extra := &GenruleExtraProperties{}
	module.Extra = extra
	module.ImageInterface = extra
	module.CmdModifier = genruleCmdModifier
	module.AddProperties(module.Extra)

	android.InitAndroidArchModule(module, android.HostAndDeviceSupported, android.MultilibBoth)

	android.InitApexModule(module)

	return module
}

func genruleCmdModifier(ctx android.ModuleContext, cmd string) string {
	target := ctx.Target()
	arch := target.Arch.ArchType
	osName := target.Os.Name
	return fmt.Sprintf("CC_ARCH=%s CC_NATIVE_BRIDGE=%s CC_MULTILIB=%s CC_OS=%s && %s",
		arch.Name, target.NativeBridgeRelativePath, arch.Multilib, osName, cmd)
}

var _ android.ImageInterface = (*GenruleExtraProperties)(nil)

func (g *GenruleExtraProperties) ImageMutatorBegin(ctx android.BaseModuleContext) {}

func (g *GenruleExtraProperties) CoreVariantNeeded(ctx android.BaseModuleContext) bool {
	return !(ctx.SocSpecific() || ctx.DeviceSpecific() || ctx.ProductSpecific())
}

func (g *GenruleExtraProperties) RamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return Bool(g.Ramdisk_available)
}

func (g *GenruleExtraProperties) VendorRamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return Bool(g.Vendor_ramdisk_available)
}

func (g *GenruleExtraProperties) DebugRamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return false
}

func (g *GenruleExtraProperties) RecoveryVariantNeeded(ctx android.BaseModuleContext) bool {
	// If the build is using a snapshot, the recovery variant under AOSP directories
	// is not needed.
	recoverySnapshotVersion := ctx.DeviceConfig().RecoverySnapshotVersion()
	if recoverySnapshotVersion != "current" && recoverySnapshotVersion != "" &&
		!snapshot.IsRecoveryProprietaryModule(ctx) {
		return false
	} else {
		return Bool(g.Recovery_available)
	}
}

func (g *GenruleExtraProperties) ExtraImageVariations(ctx android.BaseModuleContext) []string {
	var variants []string
	vndkVersion := ctx.DeviceConfig().VndkVersion()
	vendorVariantRequired := Bool(g.Vendor_available) || Bool(g.Odm_available) || ctx.SocSpecific() || ctx.DeviceSpecific()
	productVariantRequired := Bool(g.Product_available) || ctx.ProductSpecific()

	if vndkVersion == "" {
		if vendorVariantRequired {
			variants = append(variants, VendorVariation)
		}
		if productVariantRequired {
			variants = append(variants, ProductVariation)
		}
	} else {
		if vendorVariantRequired {
			// If vndkVersion is current, we can always use PlatformVndkVersion.
			// If not, we assume modules under proprietary paths are compatible for
			// BOARD_VNDK_VERSION. The other modules are regarded as AOSP, that is
			// PLATFORM_VNDK_VERSION.
			if vndkVersion == "current" || !snapshot.IsVendorProprietaryModule(ctx) {
				variants = append(variants, VendorVariationPrefix+ctx.DeviceConfig().PlatformVndkVersion())
			} else {
				variants = append(variants, VendorVariationPrefix+vndkVersion)
			}
		}
		if productVariantRequired {
			variants = append(variants, ProductVariationPrefix+ctx.DeviceConfig().PlatformVndkVersion())
		}
	}

	return variants
}

func (g *GenruleExtraProperties) SetImageVariation(ctx android.BaseModuleContext, variation string, module android.Module) {
}
