// Copyright (C) 2021 The Android Open Source Project
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

package filesystem

import (
	"fmt"
	"strconv"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func init() {
	android.RegisterModuleType("bootimg", bootimgFactory)
}

type bootimg struct {
	android.ModuleBase

	properties bootimgProperties

	output     android.OutputPath
	installDir android.InstallPath
}

type bootimgProperties struct {
	// Path to the linux kernel prebuilt file
	Kernel_prebuilt *string `android:"arch_variant,path"`

	// Filesystem module that is used as ramdisk
	Ramdisk_module *string

	// Path to the device tree blob (DTB) prebuilt file to add to this boot image
	Dtb_prebuilt *string `android:"arch_variant,path"`

	// Header version number. Must be set to one of the version numbers that are currently
	// supported. Refer to
	// https://source.android.com/devices/bootloader/boot-image-header
	Header_version *string

	// Determines if this image is for the vendor_boot partition. Default is false. Refer to
	// https://source.android.com/devices/bootloader/partitions/vendor-boot-partitions
	Vendor_boot *bool

	// Optional kernel commandline
	Cmdline *string

	// When set to true, sign the image with avbtool. Default is false.
	Use_avb *bool

	// Name of the partition stored in vbmeta desc. Defaults to the name of this module.
	Partition_name *string

	// Path to the private key that avbtool will use to sign this filesystem image.
	// TODO(jiyong): allow apex_key to be specified here
	Avb_private_key *string `android:"path"`

	// Hash and signing algorithm for avbtool. Default is SHA256_RSA4096.
	Avb_algorithm *string
}

// bootimg is the image for the boot partition. It consists of header, kernel, ramdisk, and dtb.
func bootimgFactory() android.Module {
	module := &bootimg{}
	module.AddProperties(&module.properties)
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	return module
}

type bootimgDep struct {
	blueprint.BaseDependencyTag
	kind string
}

var bootimgRamdiskDep = bootimgDep{kind: "ramdisk"}

func (b *bootimg) DepsMutator(ctx android.BottomUpMutatorContext) {
	ramdisk := proptools.String(b.properties.Ramdisk_module)
	if ramdisk != "" {
		ctx.AddDependency(ctx.Module(), bootimgRamdiskDep, ramdisk)
	}
}

func (b *bootimg) installFileName() string {
	return b.BaseModuleName() + ".img"
}

func (b *bootimg) partitionName() string {
	return proptools.StringDefault(b.properties.Partition_name, b.BaseModuleName())
}

func (b *bootimg) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	var unsignedOutput android.OutputPath
	if proptools.Bool(b.properties.Vendor_boot) {
		unsignedOutput = b.buildVendorBootImage(ctx)
	} else {
		// TODO(jiyong): fix this
		ctx.PropertyErrorf("vendor_boot", "only vendor_boot:true is supported")
	}

	if proptools.Bool(b.properties.Use_avb) {
		b.output = b.signImage(ctx, unsignedOutput)
	} else {
		b.output = unsignedOutput
	}

	b.installDir = android.PathForModuleInstall(ctx, "etc")
	ctx.InstallFile(b.installDir, b.installFileName(), b.output)
}

func (b *bootimg) buildVendorBootImage(ctx android.ModuleContext) android.OutputPath {
	output := android.PathForModuleOut(ctx, "unsigned.img").OutputPath
	builder := android.NewRuleBuilder(pctx, ctx)
	cmd := builder.Command().BuiltTool("mkbootimg")

	kernel := android.OptionalPathForModuleSrc(ctx, b.properties.Kernel_prebuilt)
	if kernel.Valid() {
		ctx.PropertyErrorf("kernel_prebuilt", "vendor_boot partition can't have kernel")
		return output
	}

	dtbName := proptools.String(b.properties.Dtb_prebuilt)
	if dtbName == "" {
		ctx.PropertyErrorf("dtb_prebuilt", "must be set")
		return output
	}
	dtb := android.PathForModuleSrc(ctx, dtbName)
	cmd.FlagWithInput("--dtb ", dtb)

	cmdline := proptools.String(b.properties.Cmdline)
	if cmdline != "" {
		cmd.FlagWithArg("--vendor_cmdline ", "\""+cmdline+"\"")
	}

	headerVersion := proptools.String(b.properties.Header_version)
	if headerVersion == "" {
		ctx.PropertyErrorf("header_version", "must be set")
		return output
	}
	verNum, err := strconv.Atoi(headerVersion)
	if err != nil {
		ctx.PropertyErrorf("header_version", "%q is not a number", headerVersion)
		return output
	}
	if verNum < 3 {
		ctx.PropertyErrorf("header_version", "must be 3 or higher for vendor_boot")
		return output
	}
	cmd.FlagWithArg("--header_version ", headerVersion)

	ramdiskName := proptools.String(b.properties.Ramdisk_module)
	if ramdiskName == "" {
		ctx.PropertyErrorf("ramdisk_module", "must be set")
		return output
	}
	ramdisk := ctx.GetDirectDepWithTag(ramdiskName, bootimgRamdiskDep)
	if filesystem, ok := ramdisk.(*filesystem); ok {
		cmd.FlagWithInput("--vendor_ramdisk ", filesystem.OutputPath())
	} else {
		ctx.PropertyErrorf("ramdisk", "%q is not android_filesystem module", ramdisk.Name())
		return output
	}

	cmd.FlagWithOutput("--vendor_boot ", output)

	builder.Build("build_vendor_bootimg", fmt.Sprintf("Creating %s", b.BaseModuleName()))
	return output
}

func (b *bootimg) signImage(ctx android.ModuleContext, unsignedImage android.OutputPath) android.OutputPath {
	signedImage := android.PathForModuleOut(ctx, "signed.img").OutputPath
	key := android.PathForModuleSrc(ctx, proptools.String(b.properties.Avb_private_key))

	builder := android.NewRuleBuilder(pctx, ctx)
	builder.Command().Text("cp").Input(unsignedImage).Output(signedImage)
	builder.Command().
		BuiltTool("avbtool").
		Flag("add_hash_footer").
		FlagWithArg("--partition_name ", b.partitionName()).
		FlagWithInput("--key ", key).
		FlagWithOutput("--image ", signedImage)

	builder.Build("sign_bootimg", fmt.Sprintf("Signing %s", b.BaseModuleName()))

	return signedImage
}

var _ android.AndroidMkEntriesProvider = (*bootimg)(nil)

// Implements android.AndroidMkEntriesProvider
func (b *bootimg) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(b.output),
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_PATH", b.installDir.ToMakePath().String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", b.installFileName())
			},
		},
	}}
}

var _ Filesystem = (*bootimg)(nil)

func (b *bootimg) OutputPath() android.Path {
	return b.output
}
