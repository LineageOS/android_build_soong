// Copyright 2016 Google Inc. All rights reserved.
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

package etc

import (
	"strconv"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

var pctx = android.NewPackageContext("android/soong/etc")

// TODO(jungw): Now that it handles more than the ones in etc/, consider renaming this file.

func init() {
	pctx.Import("android/soong/android")

	android.RegisterModuleType("prebuilt_etc", PrebuiltEtcFactory)
	android.RegisterModuleType("prebuilt_etc_host", PrebuiltEtcHostFactory)
	android.RegisterModuleType("prebuilt_usr_share", PrebuiltUserShareFactory)
	android.RegisterModuleType("prebuilt_usr_share_host", PrebuiltUserShareHostFactory)
	android.RegisterModuleType("prebuilt_font", PrebuiltFontFactory)
	android.RegisterModuleType("prebuilt_firmware", PrebuiltFirmwareFactory)
}

type prebuiltEtcProperties struct {
	// Source file of this prebuilt.
	Src *string `android:"path,arch_variant"`

	// optional subdirectory under which this file is installed into
	Sub_dir *string `android:"arch_variant"`

	// optional name for the installed file. If unspecified, name of the module is used as the file name
	Filename *string `android:"arch_variant"`

	// when set to true, and filename property is not set, the name for the installed file
	// is the same as the file name of the source file.
	Filename_from_src *bool `android:"arch_variant"`

	// Make this module available when building for ramdisk.
	Ramdisk_available *bool

	// Make this module available when building for recovery.
	Recovery_available *bool

	// Whether this module is directly installable to one of the partitions. Default: true.
	Installable *bool
}

type PrebuiltEtcModule interface {
	android.Module
	SubDir() string
	OutputFile() android.OutputPath
}

type PrebuiltEtc struct {
	android.ModuleBase

	properties prebuiltEtcProperties

	sourceFilePath android.Path
	outputFilePath android.OutputPath
	// The base install location, e.g. "etc" for prebuilt_etc, "usr/share" for prebuilt_usr_share.
	installDirBase string
	// The base install location when soc_specific property is set to true, e.g. "firmware" for prebuilt_firmware.
	socInstallDirBase      string
	installDirPath         android.InstallPath
	additionalDependencies *android.Paths
}

func (p *PrebuiltEtc) inRamdisk() bool {
	return p.ModuleBase.InRamdisk() || p.ModuleBase.InstallInRamdisk()
}

func (p *PrebuiltEtc) onlyInRamdisk() bool {
	return p.ModuleBase.InstallInRamdisk()
}

func (p *PrebuiltEtc) InstallInRamdisk() bool {
	return p.inRamdisk()
}

func (p *PrebuiltEtc) inRecovery() bool {
	return p.ModuleBase.InRecovery() || p.ModuleBase.InstallInRecovery()
}

func (p *PrebuiltEtc) onlyInRecovery() bool {
	return p.ModuleBase.InstallInRecovery()
}

func (p *PrebuiltEtc) InstallInRecovery() bool {
	return p.inRecovery()
}

var _ android.ImageInterface = (*PrebuiltEtc)(nil)

func (p *PrebuiltEtc) ImageMutatorBegin(ctx android.BaseModuleContext) {}

func (p *PrebuiltEtc) CoreVariantNeeded(ctx android.BaseModuleContext) bool {
	return !p.ModuleBase.InstallInRecovery() && !p.ModuleBase.InstallInRamdisk()
}

func (p *PrebuiltEtc) RamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return proptools.Bool(p.properties.Ramdisk_available) || p.ModuleBase.InstallInRamdisk()
}

func (p *PrebuiltEtc) RecoveryVariantNeeded(ctx android.BaseModuleContext) bool {
	return proptools.Bool(p.properties.Recovery_available) || p.ModuleBase.InstallInRecovery()
}

func (p *PrebuiltEtc) ExtraImageVariations(ctx android.BaseModuleContext) []string {
	return nil
}

func (p *PrebuiltEtc) SetImageVariation(ctx android.BaseModuleContext, variation string, module android.Module) {
}

func (p *PrebuiltEtc) DepsMutator(ctx android.BottomUpMutatorContext) {
	if p.properties.Src == nil {
		ctx.PropertyErrorf("src", "missing prebuilt source file")
	}
}

func (p *PrebuiltEtc) SourceFilePath(ctx android.ModuleContext) android.Path {
	return android.PathForModuleSrc(ctx, android.String(p.properties.Src))
}

func (p *PrebuiltEtc) InstallDirPath() android.InstallPath {
	return p.installDirPath
}

// This allows other derivative modules (e.g. prebuilt_etc_xml) to perform
// additional steps (like validating the src) before the file is installed.
func (p *PrebuiltEtc) SetAdditionalDependencies(paths android.Paths) {
	p.additionalDependencies = &paths
}

func (p *PrebuiltEtc) OutputFile() android.OutputPath {
	return p.outputFilePath
}

func (p *PrebuiltEtc) SubDir() string {
	return android.String(p.properties.Sub_dir)
}

func (p *PrebuiltEtc) Installable() bool {
	return p.properties.Installable == nil || android.Bool(p.properties.Installable)
}

func (p *PrebuiltEtc) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	p.sourceFilePath = android.PathForModuleSrc(ctx, android.String(p.properties.Src))
	filename := android.String(p.properties.Filename)
	filename_from_src := android.Bool(p.properties.Filename_from_src)
	if filename == "" {
		if filename_from_src {
			filename = p.sourceFilePath.Base()
		} else {
			filename = ctx.ModuleName()
		}
	} else if filename_from_src {
		ctx.PropertyErrorf("filename_from_src", "filename is set. filename_from_src can't be true")
		return
	}
	p.outputFilePath = android.PathForModuleOut(ctx, filename).OutputPath

	// If soc install dir was specified and SOC specific is set, set the installDirPath to the specified
	// socInstallDirBase.
	installBaseDir := p.installDirBase
	if ctx.SocSpecific() && p.socInstallDirBase != "" {
		installBaseDir = p.socInstallDirBase
	}
	p.installDirPath = android.PathForModuleInstall(ctx, installBaseDir, proptools.String(p.properties.Sub_dir))

	// This ensures that outputFilePath has the correct name for others to
	// use, as the source file may have a different name.
	ctx.Build(pctx, android.BuildParams{
		Rule:   android.Cp,
		Output: p.outputFilePath,
		Input:  p.sourceFilePath,
	})
}

func (p *PrebuiltEtc) AndroidMkEntries() []android.AndroidMkEntries {
	nameSuffix := ""
	if p.inRamdisk() && !p.onlyInRamdisk() {
		nameSuffix = ".ramdisk"
	}
	if p.inRecovery() && !p.onlyInRecovery() {
		nameSuffix = ".recovery"
	}
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		SubName:    nameSuffix,
		OutputFile: android.OptionalPathForPath(p.outputFilePath),
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_TAGS", "optional")
				entries.SetString("LOCAL_MODULE_PATH", p.installDirPath.ToMakePath().String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", p.outputFilePath.Base())
				entries.SetString("LOCAL_UNINSTALLABLE_MODULE", strconv.FormatBool(!p.Installable()))
				if p.additionalDependencies != nil {
					for _, path := range *p.additionalDependencies {
						entries.SetString("LOCAL_ADDITIONAL_DEPENDENCIES", path.String())
					}
				}
			},
		},
	}}
}

func InitPrebuiltEtcModule(p *PrebuiltEtc, dirBase string) {
	p.installDirBase = dirBase
	p.AddProperties(&p.properties)
}

// prebuilt_etc is for a prebuilt artifact that is installed in
// <partition>/etc/<sub_dir> directory.
func PrebuiltEtcFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "etc")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	return module
}

// prebuilt_etc_host is for a host prebuilt artifact that is installed in
// $(HOST_OUT)/etc/<sub_dir> directory.
func PrebuiltEtcHostFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "etc")
	// This module is host-only
	android.InitAndroidArchModule(module, android.HostSupported, android.MultilibCommon)
	return module
}

// prebuilt_usr_share is for a prebuilt artifact that is installed in
// <partition>/usr/share/<sub_dir> directory.
func PrebuiltUserShareFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "usr/share")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	return module
}

// prebuild_usr_share_host is for a host prebuilt artifact that is installed in
// $(HOST_OUT)/usr/share/<sub_dir> directory.
func PrebuiltUserShareHostFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "usr/share")
	// This module is host-only
	android.InitAndroidArchModule(module, android.HostSupported, android.MultilibCommon)
	return module
}

// prebuilt_font installs a font in <partition>/fonts directory.
func PrebuiltFontFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "fonts")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	return module
}

// prebuilt_firmware installs a firmware file to <partition>/etc/firmware directory for system image.
// If soc_specific property is set to true, the firmware file is installed to the vendor <partition>/firmware
// directory for vendor image.
func PrebuiltFirmwareFactory() android.Module {
	module := &PrebuiltEtc{}
	module.socInstallDirBase = "firmware"
	InitPrebuiltEtcModule(module, "etc/firmware")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	return module
}
