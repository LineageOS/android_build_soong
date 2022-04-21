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
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/bazel"
	"android/soong/snapshot"
)

var pctx = android.NewPackageContext("android/soong/etc")

// TODO(jungw): Now that it handles more than the ones in etc/, consider renaming this file.

func init() {
	pctx.Import("android/soong/android")
	RegisterPrebuiltEtcBuildComponents(android.InitRegistrationContext)
	snapshot.RegisterSnapshotAction(generatePrebuiltSnapshot)
}

func RegisterPrebuiltEtcBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("prebuilt_etc", PrebuiltEtcFactory)
	ctx.RegisterModuleType("prebuilt_etc_host", PrebuiltEtcHostFactory)
	ctx.RegisterModuleType("prebuilt_root", PrebuiltRootFactory)
	ctx.RegisterModuleType("prebuilt_root_host", PrebuiltRootHostFactory)
	ctx.RegisterModuleType("prebuilt_usr_share", PrebuiltUserShareFactory)
	ctx.RegisterModuleType("prebuilt_usr_share_host", PrebuiltUserShareHostFactory)
	ctx.RegisterModuleType("prebuilt_font", PrebuiltFontFactory)
	ctx.RegisterModuleType("prebuilt_firmware", PrebuiltFirmwareFactory)
	ctx.RegisterModuleType("prebuilt_dsp", PrebuiltDSPFactory)
	ctx.RegisterModuleType("prebuilt_rfsa", PrebuiltRFSAFactory)

	ctx.RegisterModuleType("prebuilt_defaults", defaultsFactory)

}

var PrepareForTestWithPrebuiltEtc = android.FixtureRegisterWithContext(RegisterPrebuiltEtcBuildComponents)

type prebuiltEtcProperties struct {
	// Source file of this prebuilt. Can reference a genrule type module with the ":module" syntax.
	Src *string `android:"path,arch_variant"`

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

	// Make this module available when building for debug ramdisk.
	Debug_ramdisk_available *bool

	// Make this module available when building for recovery.
	Recovery_available *bool

	// Whether this module is directly installable to one of the partitions. Default: true.
	Installable *bool

	// Install symlinks to the installed file.
	Symlinks []string `android:"arch_variant"`
}

type prebuiltSubdirProperties struct {
	// Optional subdirectory under which this file is installed into, cannot be specified with
	// relative_install_path, prefer relative_install_path.
	Sub_dir *string `android:"arch_variant"`

	// Optional subdirectory under which this file is installed into, cannot be specified with
	// sub_dir.
	Relative_install_path *string `android:"arch_variant"`
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
	android.DefaultableModuleBase
	android.BazelModuleBase

	snapshot.VendorSnapshotModuleInterface
	snapshot.RecoverySnapshotModuleInterface

	properties       prebuiltEtcProperties
	subdirProperties prebuiltSubdirProperties

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

type Defaults struct {
	android.ModuleBase
	android.DefaultsModuleBase
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

func (p *PrebuiltEtc) inDebugRamdisk() bool {
	return p.ModuleBase.InDebugRamdisk() || p.ModuleBase.InstallInDebugRamdisk()
}

func (p *PrebuiltEtc) onlyInDebugRamdisk() bool {
	return p.ModuleBase.InstallInDebugRamdisk()
}

func (p *PrebuiltEtc) InstallInDebugRamdisk() bool {
	return p.inDebugRamdisk()
}

func (p *PrebuiltEtc) InRecovery() bool {
	return p.ModuleBase.InRecovery() || p.ModuleBase.InstallInRecovery()
}

func (p *PrebuiltEtc) onlyInRecovery() bool {
	return p.ModuleBase.InstallInRecovery()
}

func (p *PrebuiltEtc) InstallInRecovery() bool {
	return p.InRecovery()
}

var _ android.ImageInterface = (*PrebuiltEtc)(nil)

func (p *PrebuiltEtc) ImageMutatorBegin(ctx android.BaseModuleContext) {}

func (p *PrebuiltEtc) CoreVariantNeeded(ctx android.BaseModuleContext) bool {
	return !p.ModuleBase.InstallInRecovery() && !p.ModuleBase.InstallInRamdisk() &&
		!p.ModuleBase.InstallInVendorRamdisk() && !p.ModuleBase.InstallInDebugRamdisk()
}

func (p *PrebuiltEtc) RamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return proptools.Bool(p.properties.Ramdisk_available) || p.ModuleBase.InstallInRamdisk()
}

func (p *PrebuiltEtc) VendorRamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return proptools.Bool(p.properties.Vendor_ramdisk_available) || p.ModuleBase.InstallInVendorRamdisk()
}

func (p *PrebuiltEtc) DebugRamdiskVariantNeeded(ctx android.BaseModuleContext) bool {
	return proptools.Bool(p.properties.Debug_ramdisk_available) || p.ModuleBase.InstallInDebugRamdisk()
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
	if subDir := proptools.String(p.subdirProperties.Sub_dir); subDir != "" {
		return subDir
	}
	return proptools.String(p.subdirProperties.Relative_install_path)
}

func (p *PrebuiltEtc) BaseDir() string {
	return p.installDirBase
}

func (p *PrebuiltEtc) Installable() bool {
	return p.properties.Installable == nil || proptools.Bool(p.properties.Installable)
}

func (p *PrebuiltEtc) InVendor() bool {
	return p.ModuleBase.InstallInVendor()
}

func (p *PrebuiltEtc) ExcludeFromVendorSnapshot() bool {
	return false
}

func (p *PrebuiltEtc) ExcludeFromRecoverySnapshot() bool {
	return false
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

	if strings.Contains(filename, "/") {
		ctx.PropertyErrorf("filename", "filename cannot contain separator '/'")
		return
	}

	// Check that `sub_dir` and `relative_install_path` are not set at the same time.
	if p.subdirProperties.Sub_dir != nil && p.subdirProperties.Relative_install_path != nil {
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
	if p.inDebugRamdisk() && !p.onlyInDebugRamdisk() {
		nameSuffix = ".debug_ramdisk"
	}
	if p.InRecovery() && !p.onlyInRecovery() {
		nameSuffix = ".recovery"
	}
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		SubName:    nameSuffix,
		OutputFile: android.OptionalPathForPath(p.outputFilePath),
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_TAGS", "optional")
				entries.SetString("LOCAL_MODULE_PATH", p.installDirPath.String())
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
	p.AddProperties(&p.subdirProperties)
}

func InitPrebuiltRootModule(p *PrebuiltEtc) {
	p.installDirBase = "."
	p.AddProperties(&p.properties)
}

// prebuilt_etc is for a prebuilt artifact that is installed in
// <partition>/etc/<sub_dir> directory.
func PrebuiltEtcFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "etc")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	android.InitDefaultableModule(module)
	android.InitBazelModule(module)
	return module
}

func defaultsFactory() android.Module {
	return DefaultsFactory()
}

func DefaultsFactory(props ...interface{}) android.Module {
	module := &Defaults{}

	module.AddProperties(props...)
	module.AddProperties(
		&prebuiltEtcProperties{},
		&prebuiltSubdirProperties{},
	)

	android.InitDefaultsModule(module)

	return module
}

// prebuilt_etc_host is for a host prebuilt artifact that is installed in
// $(HOST_OUT)/etc/<sub_dir> directory.
func PrebuiltEtcHostFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "etc")
	// This module is host-only
	android.InitAndroidArchModule(module, android.HostSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitBazelModule(module)
	return module
}

// prebuilt_root is for a prebuilt artifact that is installed in
// <partition>/ directory. Can't have any sub directories.
func PrebuiltRootFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltRootModule(module)
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_root_host is for a host prebuilt artifact that is installed in $(HOST_OUT)/<sub_dir>
// directory.
func PrebuiltRootHostFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, ".")
	// This module is host-only
	android.InitAndroidArchModule(module, android.HostSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_usr_share is for a prebuilt artifact that is installed in
// <partition>/usr/share/<sub_dir> directory.
func PrebuiltUserShareFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "usr/share")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	android.InitDefaultableModule(module)
	return module
}

// prebuild_usr_share_host is for a host prebuilt artifact that is installed in
// $(HOST_OUT)/usr/share/<sub_dir> directory.
func PrebuiltUserShareHostFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "usr/share")
	// This module is host-only
	android.InitAndroidArchModule(module, android.HostSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_font installs a font in <partition>/fonts directory.
func PrebuiltFontFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "fonts")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	android.InitDefaultableModule(module)
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
	android.InitDefaultableModule(module)
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
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_rfsa installs a firmware file that will be available through Qualcomm's RFSA
// to the <partition>/lib/rfsa directory.
func PrebuiltRFSAFactory() android.Module {
	module := &PrebuiltEtc{}
	// Ideally these would go in /vendor/dsp, but the /vendor/lib/rfsa paths are hardcoded in too
	// many places outside of the application processor.  They could be moved to /vendor/dsp once
	// that is cleaned up.
	InitPrebuiltEtcModule(module, "lib/rfsa")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	android.InitDefaultableModule(module)
	return module
}

// Copy file into the snapshot
func copyFile(ctx android.SingletonContext, path android.Path, out string, fake bool) android.OutputPath {
	if fake {
		// Create empty file instead for the fake snapshot
		return snapshot.WriteStringToFileRule(ctx, "", out)
	} else {
		return snapshot.CopyFileRule(pctx, ctx, path, out)
	}
}

// Check if the module is target of the snapshot
func isSnapshotAware(ctx android.SingletonContext, m *PrebuiltEtc, image snapshot.SnapshotImage) bool {
	if !m.Enabled() {
		return false
	}

	// Skip if the module is not included in the image
	if !image.InImage(m)() {
		return false
	}

	// When android/prebuilt.go selects between source and prebuilt, it sets
	// HideFromMake on the other one to avoid duplicate install rules in make.
	if m.IsHideFromMake() {
		return false
	}

	// There are some prebuilt_etc module with multiple definition of same name.
	// Check if the target would be included from the build
	if !m.ExportedToMake() {
		return false
	}

	// Skip if the module is in the predefined path list to skip
	if image.IsProprietaryPath(ctx.ModuleDir(m), ctx.DeviceConfig()) {
		return false
	}

	// Skip if the module should be excluded
	if image.ExcludeFromSnapshot(m) || image.ExcludeFromDirectedSnapshot(ctx.DeviceConfig(), m.BaseModuleName()) {
		return false
	}

	// Skip from other exceptional cases
	if m.Target().Os.Class != android.Device {
		return false
	}
	if m.Target().NativeBridge == android.NativeBridgeEnabled {
		return false
	}

	return true
}

func generatePrebuiltSnapshot(s snapshot.SnapshotSingleton, ctx android.SingletonContext, snapshotArchDir string) android.Paths {
	/*
		Snapshot zipped artifacts directory structure for etc modules:
		{SNAPSHOT_ARCH}/
			arch-{TARGET_ARCH}-{TARGET_ARCH_VARIANT}/
				etc/
					(prebuilt etc files)
			arch-{TARGET_2ND_ARCH}-{TARGET_2ND_ARCH_VARIANT}/
				etc/
					(prebuilt etc files)
			NOTICE_FILES/
				(notice files)
	*/
	var snapshotOutputs android.Paths
	noticeDir := filepath.Join(snapshotArchDir, "NOTICE_FILES")
	installedNotices := make(map[string]bool)

	ctx.VisitAllModules(func(module android.Module) {
		m, ok := module.(*PrebuiltEtc)
		if !ok {
			return
		}

		if !isSnapshotAware(ctx, m, s.Image) {
			return
		}

		targetArch := "arch-" + m.Target().Arch.ArchType.String()

		snapshotLibOut := filepath.Join(snapshotArchDir, targetArch, "etc", m.BaseModuleName())
		snapshotOutputs = append(snapshotOutputs, copyFile(ctx, m.OutputFile(), snapshotLibOut, s.Fake))

		prop := snapshot.SnapshotJsonFlags{}
		propOut := snapshotLibOut + ".json"
		prop.ModuleName = m.BaseModuleName()
		if m.subdirProperties.Relative_install_path != nil {
			prop.RelativeInstallPath = *m.subdirProperties.Relative_install_path
		}

		if m.properties.Filename != nil {
			prop.Filename = *m.properties.Filename
		}

		j, err := json.Marshal(prop)
		if err != nil {
			ctx.Errorf("json marshal to %q failed: %#v", propOut, err)
			return
		}
		snapshotOutputs = append(snapshotOutputs, snapshot.WriteStringToFileRule(ctx, string(j), propOut))

		if len(m.EffectiveLicenseFiles()) > 0 {
			noticeName := ctx.ModuleName(m) + ".txt"
			noticeOut := filepath.Join(noticeDir, noticeName)
			// skip already copied notice file
			if !installedNotices[noticeOut] {
				installedNotices[noticeOut] = true

				noticeOutPath := android.PathForOutput(ctx, noticeOut)
				ctx.Build(pctx, android.BuildParams{
					Rule:        android.Cat,
					Inputs:      m.EffectiveLicenseFiles(),
					Output:      noticeOutPath,
					Description: "combine notices for " + noticeOut,
				})
				snapshotOutputs = append(snapshotOutputs, noticeOutPath)
			}
		}

	})

	return snapshotOutputs
}

// For Bazel / bp2build

type bazelPrebuiltEtcAttributes struct {
	Src         bazel.LabelAttribute
	Filename    string
	Sub_dir     string
	Installable bazel.BoolAttribute
}

// ConvertWithBp2build performs bp2build conversion of PrebuiltEtc
func (p *PrebuiltEtc) ConvertWithBp2build(ctx android.TopDownMutatorContext) {
	// All prebuilt_* modules are PrebuiltEtc, but at this time, we only convert prebuilt_etc modules.
	if p.installDirBase != "etc" {
		return
	}

	prebuiltEtcBp2BuildInternal(ctx, p)
}

func prebuiltEtcBp2BuildInternal(ctx android.TopDownMutatorContext, module *PrebuiltEtc) {
	var srcLabelAttribute bazel.LabelAttribute
	for axis, configToProps := range module.GetArchVariantProperties(ctx, &prebuiltEtcProperties{}) {
		for config, p := range configToProps {
			props, ok := p.(*prebuiltEtcProperties)
			if !ok {
				continue
			}
			if props.Src != nil {
				label := android.BazelLabelForModuleSrcSingle(ctx, *props.Src)
				srcLabelAttribute.SetSelectValue(axis, config, label)
			}
		}
	}

	var filename string
	if module.properties.Filename != nil {
		filename = *module.properties.Filename
	}

	var subDir string
	if module.subdirProperties.Sub_dir != nil {
		subDir = *module.subdirProperties.Sub_dir
	}

	var installableBoolAttribute bazel.BoolAttribute
	if module.properties.Installable != nil {
		installableBoolAttribute.Value = module.properties.Installable
	}

	attrs := &bazelPrebuiltEtcAttributes{
		Src:         srcLabelAttribute,
		Filename:    filename,
		Sub_dir:     subDir,
		Installable: installableBoolAttribute,
	}

	props := bazel.BazelTargetModuleProperties{
		Rule_class:        "prebuilt_file",
		Bzl_load_location: "//build/bazel/rules:prebuilt_file.bzl",
	}

	ctx.CreateBazelTargetModule(props, android.CommonAttributes{Name: module.Name()}, attrs)
}
