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

// This file implements module types that install prebuilt artifacts.
//
// There exist two classes of prebuilt modules in the Android tree. The first class are the ones
// based on `android.Prebuilt`, such as `cc_prebuilt_library` and `java_import`. This kind of
// modules may exist both as prebuilts and source at the same time, though only one would be
// installed and the other would be marked disabled. The `prebuilt_postdeps` mutator would select
// the actual modules to be installed. More details in android/prebuilt.go.
//
// The second class is described in this file. Unlike `android.Prebuilt` based module types,
// `prebuilt_etc` exist only as prebuilts and cannot have a same-named source module counterpart.
// This makes the logic of `prebuilt_etc` to be much simpler as they don't need to go through the
// various `prebuilt_*` mutators.

import (
	"fmt"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

var pctx = android.NewPackageContext("android/soong/etc")

// TODO(jungw): Now that it handles more than the ones in etc/, consider renaming this file.

func init() {
	pctx.Import("android/soong/android")
	RegisterPrebuiltEtcBuildComponents(android.InitRegistrationContext)
}

func RegisterPrebuiltEtcBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("prebuilt_etc", PrebuiltEtcFactory)
	ctx.RegisterModuleType("prebuilt_etc_host", PrebuiltEtcHostFactory)
	ctx.RegisterModuleType("prebuilt_usr_share", PrebuiltUserShareFactory)
	ctx.RegisterModuleType("prebuilt_usr_share_host", PrebuiltUserShareHostFactory)
	ctx.RegisterModuleType("prebuilt_font", PrebuiltFontFactory)
	ctx.RegisterModuleType("prebuilt_firmware", PrebuiltFirmwareFactory)
	ctx.RegisterModuleType("prebuilt_dsp", PrebuiltDSPFactory)
}

type prebuiltEtcProperties struct {
	// Source file of this prebuilt. Can reference a genrule type module with the ":module" syntax.
	Src *string `android:"path,arch_variant"`

	// Optional subdirectory under which this file is installed into, cannot be specified with
	// relative_install_path, prefer relative_install_path.
	Sub_dir *string `android:"arch_variant"`

	// Optional subdirectory under which this file is installed into, cannot be specified with
	// sub_dir.
	Relative_install_path *string `android:"arch_variant"`

	// Optional name for the installed file. If unspecified, name of the module is used as the file
	// name.
	Filename *string `android:"arch_variant"`

	// When set to true, and filename property is not set, the name for the installed file
	// is the same as the file name of the source file.
	Filename_from_src *bool `android:"arch_variant"`

	// Make this module available when building for ramdisk.
	// On device without a dedicated recovery partition, the module is only
	// available after switching root into
	// /first_stage_ramdisk. To expose the module before switching root, install
	// the recovery variant instead.
	Ramdisk_available *bool

	// Make this module available when building for vendor ramdisk.
	// On device without a dedicated recovery partition, the module is only
	// available after switching root into
	// /first_stage_ramdisk. To expose the module before switching root, install
	// the recovery variant instead.
	Vendor_ramdisk_available *bool

	// Make this module available when building for recovery.
	Recovery_available *bool

	// Whether this module is directly installable to one of the partitions. Default: true.
	Installable *bool

	// Install symlinks to the installed file.
	Symlinks []string `android:"arch_variant"`
}

type PrebuiltEtcModule interface {
	android.Module

	// Returns the base install directory, such as "etc", "usr/share".
	BaseDir() string

	// Returns the sub install directory relative to BaseDir().
	SubDir() string

	// Returns an android.OutputPath to the intermeidate file, which is the renamed prebuilt source
	// file.
	OutputFile() android.OutputPath
}

type PrebuiltEtc struct {
	android.ModuleBase

	properties prebuiltEtcProperties

	sourceFilePath android.Path
	outputFilePath android.OutputPath
	// The base install location, e.g. "etc" for prebuilt_etc, "usr/share" for prebuilt_usr_share.
	installDirBase string
	// The base install location when soc_specific property is set to true, e.g. "firmware" for
	// prebuilt_firmware.
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

func (p *PrebuiltEtc) inVendorRamdisk() bool {
	return p.ModuleBase.InVendorRamdisk() || p.ModuleBase.InstallInVendorRamdisk()
}

func (p *PrebuiltEtc) onlyInVendorRamdisk() bool {
	return p.ModuleBase.InstallInVendorRamdisk()
}

func (p *PrebuiltEtc) InstallInVendorRamdisk() bool {
	return p.inVendorRamdisk()
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
	return !p.ModuleBase.InstallInRecovery() && !p.ModuleBase.InstallInRamdisk() &&
		!p.ModuleBase.InstallInVendorRamdisk()
}

func (p *PrebuiltEtc) RamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return proptools.Bool(p.properties.Ramdisk_available) || p.ModuleBase.InstallInRamdisk()
}

func (p *PrebuiltEtc) VendorRamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return proptools.Bool(p.properties.Vendor_ramdisk_available) || p.ModuleBase.InstallInVendorRamdisk()
}

func (p *PrebuiltEtc) RecoveryVariantNeeded(ctx android.BaseModuleContext) bool {
	return proptools.Bool(p.properties.Recovery_available) || p.ModuleBase.InstallInRecovery()
}

func (p *PrebuiltEtc) ExtraImageVariations(ctx android.BaseModuleContext) []string {
	return nil
}

func (p *PrebuiltEtc) SetImageVariation(ctx android.BaseModuleContext, variation string, module android.Module) {
}

func (p *PrebuiltEtc) SourceFilePath(ctx android.ModuleContext) android.Path {
	return android.PathForModuleSrc(ctx, proptools.String(p.properties.Src))
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

var _ android.OutputFileProducer = (*PrebuiltEtc)(nil)

func (p *PrebuiltEtc) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		return android.Paths{p.outputFilePath}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

func (p *PrebuiltEtc) SubDir() string {
	if subDir := proptools.String(p.properties.Sub_dir); subDir != "" {
		return subDir
	}
	return proptools.String(p.properties.Relative_install_path)
}

func (p *PrebuiltEtc) BaseDir() string {
	return p.installDirBase
}

func (p *PrebuiltEtc) Installable() bool {
	return p.properties.Installable == nil || proptools.Bool(p.properties.Installable)
}

func (p *PrebuiltEtc) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if p.properties.Src == nil {
		ctx.PropertyErrorf("src", "missing prebuilt source file")
		return
	}
	p.sourceFilePath = android.PathForModuleSrc(ctx, proptools.String(p.properties.Src))

	// Determine the output file basename.
	// If Filename is set, use the name specified by the property.
	// If Filename_from_src is set, use the source file name.
	// Otherwise use the module name.
	filename := proptools.String(p.properties.Filename)
	filenameFromSrc := proptools.Bool(p.properties.Filename_from_src)
	if filename != "" {
		if filenameFromSrc {
			ctx.PropertyErrorf("filename_from_src", "filename is set. filename_from_src can't be true")
			return
		}
	} else if filenameFromSrc {
		filename = p.sourceFilePath.Base()
	} else {
		filename = ctx.ModuleName()
	}
	p.outputFilePath = android.PathForModuleOut(ctx, filename).OutputPath

	// Check that `sub_dir` and `relative_install_path` are not set at the same time.
	if p.properties.Sub_dir != nil && p.properties.Relative_install_path != nil {
		ctx.PropertyErrorf("sub_dir", "relative_install_path is set. Cannot set sub_dir")
	}

	// If soc install dir was specified and SOC specific is set, set the installDirPath to the
	// specified socInstallDirBase.
	installBaseDir := p.installDirBase
	if p.SocSpecific() && p.socInstallDirBase != "" {
		installBaseDir = p.socInstallDirBase
	}
	p.installDirPath = android.PathForModuleInstall(ctx, installBaseDir, p.SubDir())

	// This ensures that outputFilePath has the correct name for others to
	// use, as the source file may have a different name.
	ctx.Build(pctx, android.BuildParams{
		Rule:   android.Cp,
		Output: p.outputFilePath,
		Input:  p.sourceFilePath,
	})

	if !p.Installable() {
		p.SkipInstall()
	}

	// Call InstallFile even when uninstallable to make the module included in the package
	installPath := ctx.InstallFile(p.installDirPath, p.outputFilePath.Base(), p.outputFilePath)
	for _, sl := range p.properties.Symlinks {
		ctx.InstallSymlink(p.installDirPath, sl, installPath)
	}
}

func (p *PrebuiltEtc) AndroidMkEntries() []android.AndroidMkEntries {
	nameSuffix := ""
	if p.inRamdisk() && !p.onlyInRamdisk() {
		nameSuffix = ".ramdisk"
	}
	if p.inVendorRamdisk() && !p.onlyInVendorRamdisk() {
		nameSuffix = ".vendor_ramdisk"
	}
	if p.inRecovery() && !p.onlyInRecovery() {
		nameSuffix = ".recovery"
	}
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		SubName:    nameSuffix,
		OutputFile: android.OptionalPathForPath(p.outputFilePath),
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_TAGS", "optional")
				entries.SetString("LOCAL_MODULE_PATH", p.installDirPath.ToMakePath().String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", p.outputFilePath.Base())
				if len(p.properties.Symlinks) > 0 {
					entries.AddStrings("LOCAL_MODULE_SYMLINKS", p.properties.Symlinks...)
				}
				entries.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", !p.Installable())
				if p.additionalDependencies != nil {
					entries.AddStrings("LOCAL_ADDITIONAL_DEPENDENCIES", p.additionalDependencies.Strings()...)
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

// prebuilt_firmware installs a firmware file to <partition>/etc/firmware directory for system
// image.
// If soc_specific property is set to true, the firmware file is installed to the
// vendor <partition>/firmware directory for vendor image.
func PrebuiltFirmwareFactory() android.Module {
	module := &PrebuiltEtc{}
	module.socInstallDirBase = "firmware"
	InitPrebuiltEtcModule(module, "etc/firmware")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	return module
}

// prebuilt_dsp installs a DSP related file to <partition>/etc/dsp directory for system image.
// If soc_specific property is set to true, the DSP related file is installed to the
// vendor <partition>/dsp directory for vendor image.
func PrebuiltDSPFactory() android.Module {
	module := &PrebuiltEtc{}
	module.socInstallDirBase = "dsp"
	InitPrebuiltEtcModule(module, "etc/dsp")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	return module
}
