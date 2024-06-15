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
	ctx.RegisterModuleType("prebuilt_etc_cacerts", PrebuiltEtcCaCertsFactory)
	ctx.RegisterModuleType("prebuilt_root", PrebuiltRootFactory)
	ctx.RegisterModuleType("prebuilt_root_host", PrebuiltRootHostFactory)
	ctx.RegisterModuleType("prebuilt_usr_share", PrebuiltUserShareFactory)
	ctx.RegisterModuleType("prebuilt_usr_share_host", PrebuiltUserShareHostFactory)
	ctx.RegisterModuleType("prebuilt_font", PrebuiltFontFactory)
	ctx.RegisterModuleType("prebuilt_firmware", PrebuiltFirmwareFactory)
	ctx.RegisterModuleType("prebuilt_dsp", PrebuiltDSPFactory)
	ctx.RegisterModuleType("prebuilt_rfsa", PrebuiltRFSAFactory)
	ctx.RegisterModuleType("prebuilt_renderscript_bitcode", PrebuiltRenderScriptBitcodeFactory)

	ctx.RegisterModuleType("prebuilt_defaults", defaultsFactory)

}

var PrepareForTestWithPrebuiltEtc = android.FixtureRegisterWithContext(RegisterPrebuiltEtcBuildComponents)

type prebuiltEtcProperties struct {
	// Source file of this prebuilt. Can reference a genrule type module with the ":module" syntax.
	// Mutually exclusive with srcs.
	Src *string `android:"path,arch_variant"`

	// Source files of this prebuilt. Can reference a genrule type module with the ":module" syntax.
	// Mutually exclusive with src. When used, filename_from_src is set to true.
	Srcs []string `android:"path,arch_variant"`

	// Optional name for the installed file. If unspecified, name of the module is used as the file
	// name. Only available when using a single source (src).
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

	// Returns an android.OutputPath to the intermediate file, which is the renamed prebuilt source
	// file.
	OutputFiles(tag string) (android.Paths, error)
}

type PrebuiltEtc struct {
	android.ModuleBase
	android.DefaultableModuleBase

	snapshot.VendorSnapshotModuleInterface
	snapshot.RecoverySnapshotModuleInterface

	properties       prebuiltEtcProperties
	subdirProperties prebuiltSubdirProperties

	sourceFilePaths android.Paths
	outputFilePaths android.OutputPaths
	// The base install location, e.g. "etc" for prebuilt_etc, "usr/share" for prebuilt_usr_share.
	installDirBase               string
	installDirBase64             string
	installAvoidMultilibConflict bool
	// The base install location when soc_specific property is set to true, e.g. "firmware" for
	// prebuilt_firmware.
	socInstallDirBase      string
	installDirPath         android.InstallPath
	additionalDependencies *android.Paths

	makeClass string

	// Aconfig files for all transitive deps.  Also exposed via TransitiveDeclarationsInfo
	mergedAconfigFiles map[string]android.Paths
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
	if len(p.properties.Srcs) > 0 {
		panic(fmt.Errorf("SourceFilePath not available on multi-source prebuilt %q", p.Name()))
	}
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
	if len(p.properties.Srcs) > 0 {
		panic(fmt.Errorf("OutputFile not available on multi-source prebuilt %q", p.Name()))
	}
	return p.outputFilePaths[0]
}

var _ android.OutputFileProducer = (*PrebuiltEtc)(nil)

func (p *PrebuiltEtc) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		return p.outputFilePaths.Paths(), nil
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

func (p *PrebuiltEtc) installBaseDir(ctx android.ModuleContext) string {
	// If soc install dir was specified and SOC specific is set, set the installDirPath to the
	// specified socInstallDirBase.
	installBaseDir := p.installDirBase
	if p.Target().Arch.ArchType.Multilib == "lib64" && p.installDirBase64 != "" {
		installBaseDir = p.installDirBase64
	}
	if p.SocSpecific() && p.socInstallDirBase != "" {
		installBaseDir = p.socInstallDirBase
	}
	if p.installAvoidMultilibConflict && !ctx.Host() && ctx.Config().HasMultilibConflict(ctx.Arch().ArchType) {
		installBaseDir = filepath.Join(installBaseDir, ctx.Arch().ArchType.String())
	}
	return installBaseDir
}

func (p *PrebuiltEtc) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	var installs []installProperties

	if p.properties.Src != nil && len(p.properties.Srcs) > 0 {
		ctx.PropertyErrorf("src", "src is set. Cannot set srcs")
	}

	// Check that `sub_dir` and `relative_install_path` are not set at the same time.
	if p.subdirProperties.Sub_dir != nil && p.subdirProperties.Relative_install_path != nil {
		ctx.PropertyErrorf("sub_dir", "relative_install_path is set. Cannot set sub_dir")
	}
	p.installDirPath = android.PathForModuleInstall(ctx, p.installBaseDir(ctx), p.SubDir())

	filename := proptools.String(p.properties.Filename)
	filenameFromSrc := proptools.Bool(p.properties.Filename_from_src)
	if p.properties.Src != nil {
		p.sourceFilePaths = android.PathsForModuleSrc(ctx, []string{proptools.String(p.properties.Src)})
		// If the source was not found, set a fake source path to
		// support AllowMissingDependencies executions.
		if len(p.sourceFilePaths) == 0 {
			p.sourceFilePaths = android.Paths{android.PathForModuleSrc(ctx)}
		}

		// Determine the output file basename.
		// If Filename is set, use the name specified by the property.
		// If Filename_from_src is set, use the source file name.
		// Otherwise use the module name.
		if filename != "" {
			if filenameFromSrc {
				ctx.PropertyErrorf("filename_from_src", "filename is set. filename_from_src can't be true")
				return
			}
		} else if filenameFromSrc {
			filename = p.sourceFilePaths[0].Base()
		} else {
			filename = ctx.ModuleName()
		}
		if strings.Contains(filename, "/") {
			ctx.PropertyErrorf("filename", "filename cannot contain separator '/'")
			return
		}
		p.outputFilePaths = android.OutputPaths{android.PathForModuleOut(ctx, filename).OutputPath}

		ip := installProperties{
			filename:       filename,
			sourceFilePath: p.sourceFilePaths[0],
			outputFilePath: p.outputFilePaths[0],
			installDirPath: p.installDirPath,
			symlinks:       p.properties.Symlinks,
		}
		installs = append(installs, ip)
	} else if len(p.properties.Srcs) > 0 {
		if filename != "" {
			ctx.PropertyErrorf("filename", "filename cannot be set when using srcs")
		}
		if len(p.properties.Symlinks) > 0 {
			ctx.PropertyErrorf("symlinks", "symlinks cannot be set when using srcs")
		}
		if p.properties.Filename_from_src != nil {
			ctx.PropertyErrorf("filename_from_src", "filename_from_src is implicitly set to true when using srcs")
		}
		p.sourceFilePaths = android.PathsForModuleSrc(ctx, p.properties.Srcs)
		for _, src := range p.sourceFilePaths {
			filename := src.Base()
			output := android.PathForModuleOut(ctx, filename).OutputPath
			ip := installProperties{
				filename:       filename,
				sourceFilePath: src,
				outputFilePath: output,
				installDirPath: p.installDirPath,
			}
			p.outputFilePaths = append(p.outputFilePaths, output)
			installs = append(installs, ip)
		}
	} else if ctx.Config().AllowMissingDependencies() {
		// If no srcs was set and AllowMissingDependencies is enabled then
		// mark the module as missing dependencies and set a fake source path
		// and file name.
		ctx.AddMissingDependencies([]string{"MISSING_PREBUILT_SRC_FILE"})
		p.sourceFilePaths = android.Paths{android.PathForModuleSrc(ctx)}
		if filename == "" {
			filename = ctx.ModuleName()
		}
		p.outputFilePaths = android.OutputPaths{android.PathForModuleOut(ctx, filename).OutputPath}
		ip := installProperties{
			filename:       filename,
			sourceFilePath: p.sourceFilePaths[0],
			outputFilePath: p.outputFilePaths[0],
			installDirPath: p.installDirPath,
		}
		installs = append(installs, ip)
	} else {
		ctx.PropertyErrorf("src", "missing prebuilt source file")
		return
	}

	// Call InstallFile even when uninstallable to make the module included in the package.
	if !p.Installable() {
		p.SkipInstall()
	}
	for _, ip := range installs {
		ip.addInstallRules(ctx)
	}
	android.CollectDependencyAconfigFiles(ctx, &p.mergedAconfigFiles)
}

type installProperties struct {
	filename       string
	sourceFilePath android.Path
	outputFilePath android.OutputPath
	installDirPath android.InstallPath
	symlinks       []string
}

// utility function to add install rules to the build graph.
// Reduces code duplication between Soong and Mixed build analysis
func (ip *installProperties) addInstallRules(ctx android.ModuleContext) {
	// Copy the file from src to a location in out/ with the correct `filename`
	// This ensures that outputFilePath has the correct name for others to
	// use, as the source file may have a different name.
	ctx.Build(pctx, android.BuildParams{
		Rule:   android.Cp,
		Output: ip.outputFilePath,
		Input:  ip.sourceFilePath,
	})

	installPath := ctx.InstallFile(ip.installDirPath, ip.filename, ip.outputFilePath)
	for _, sl := range ip.symlinks {
		ctx.InstallSymlink(ip.installDirPath, sl, installPath)
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

	class := p.makeClass
	if class == "" {
		class = "ETC"
	}

	return []android.AndroidMkEntries{{
		Class:      class,
		SubName:    nameSuffix,
		OutputFile: android.OptionalPathForPath(p.outputFilePaths[0]),
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_TAGS", "optional")
				entries.SetString("LOCAL_MODULE_PATH", p.installDirPath.String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", p.outputFilePaths[0].Base())
				if len(p.properties.Symlinks) > 0 {
					entries.AddStrings("LOCAL_MODULE_SYMLINKS", p.properties.Symlinks...)
				}
				entries.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", !p.Installable())
				if p.additionalDependencies != nil {
					entries.AddStrings("LOCAL_ADDITIONAL_DEPENDENCIES", p.additionalDependencies.Strings()...)
				}
				android.SetAconfigFileMkEntries(p.AndroidModuleBase(), entries, p.mergedAconfigFiles)
			},
		},
	}}
}

func (p *PrebuiltEtc) AndroidModuleBase() *android.ModuleBase {
	return &p.ModuleBase
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
	return module
}

// prebuilt_etc_host is for a host prebuilt artifact that is installed in
// <partition>/etc/<sub_dir> directory.
func PrebuiltEtcCaCertsFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "cacerts")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
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

// prebuilt_renderscript_bitcode installs a *.bc file into /system/lib or /system/lib64.
func PrebuiltRenderScriptBitcodeFactory() android.Module {
	module := &PrebuiltEtc{}
	module.makeClass = "RENDERSCRIPT_BITCODE"
	module.installDirBase64 = "lib64"
	module.installAvoidMultilibConflict = true
	InitPrebuiltEtcModule(module, "lib")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibBoth)
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

func generatePrebuiltSnapshot(s snapshot.SnapshotSingleton, ctx android.SingletonContext, snapshotArchDir string) snapshot.SnapshotPaths {
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
	var snapshotNotices android.Paths
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
		outputs, _ := m.OutputFiles("")
		for _, output := range outputs {
			cp := copyFile(ctx, output, snapshotLibOut, s.Fake)
			snapshotOutputs = append(snapshotOutputs, cp)
		}

		prop := snapshot.SnapshotJsonFlags{}
		propOut := snapshotLibOut + ".json"
		prop.InitBaseSnapshotProps(m)
		prop.RelativeInstallPath = m.SubDir()

		if m.properties.Filename != nil {
			prop.Filename = *m.properties.Filename
		}

		j, err := json.Marshal(prop)
		if err != nil {
			ctx.Errorf("json marshal to %q failed: %#v", propOut, err)
			return
		}
		snapshotOutputs = append(snapshotOutputs, snapshot.WriteStringToFileRule(ctx, string(j), propOut))

		for _, notice := range m.EffectiveLicenseFiles() {
			if _, ok := installedNotices[notice.String()]; !ok {
				installedNotices[notice.String()] = true
				snapshotNotices = append(snapshotNotices, notice)
			}
		}

	})

	return snapshot.SnapshotPaths{OutputFiles: snapshotOutputs, NoticeFiles: snapshotNotices}
}
