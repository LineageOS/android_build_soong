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
	"path/filepath"
	"strings"

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
	ctx.RegisterModuleType("prebuilt_etc_cacerts", PrebuiltEtcCaCertsFactory)
	ctx.RegisterModuleType("prebuilt_avb", PrebuiltAvbFactory)
	ctx.RegisterModuleType("prebuilt_root", PrebuiltRootFactory)
	ctx.RegisterModuleType("prebuilt_root_host", PrebuiltRootHostFactory)
	ctx.RegisterModuleType("prebuilt_usr_share", PrebuiltUserShareFactory)
	ctx.RegisterModuleType("prebuilt_usr_share_host", PrebuiltUserShareHostFactory)
	ctx.RegisterModuleType("prebuilt_usr_hyphendata", PrebuiltUserHyphenDataFactory)
	ctx.RegisterModuleType("prebuilt_usr_keylayout", PrebuiltUserKeyLayoutFactory)
	ctx.RegisterModuleType("prebuilt_usr_keychars", PrebuiltUserKeyCharsFactory)
	ctx.RegisterModuleType("prebuilt_usr_idc", PrebuiltUserIdcFactory)
	ctx.RegisterModuleType("prebuilt_usr_srec", PrebuiltUserSrecFactory)
	ctx.RegisterModuleType("prebuilt_font", PrebuiltFontFactory)
	ctx.RegisterModuleType("prebuilt_overlay", PrebuiltOverlayFactory)
	ctx.RegisterModuleType("prebuilt_firmware", PrebuiltFirmwareFactory)
	ctx.RegisterModuleType("prebuilt_gpu", PrebuiltGPUFactory)
	ctx.RegisterModuleType("prebuilt_dsp", PrebuiltDSPFactory)
	ctx.RegisterModuleType("prebuilt_rfsa", PrebuiltRFSAFactory)
	ctx.RegisterModuleType("prebuilt_renderscript_bitcode", PrebuiltRenderScriptBitcodeFactory)
	ctx.RegisterModuleType("prebuilt_media", PrebuiltMediaFactory)
	ctx.RegisterModuleType("prebuilt_voicepack", PrebuiltVoicepackFactory)
	ctx.RegisterModuleType("prebuilt_bin", PrebuiltBinaryFactory)
	ctx.RegisterModuleType("prebuilt_wallpaper", PrebuiltWallpaperFactory)
	ctx.RegisterModuleType("prebuilt_priv_app", PrebuiltPrivAppFactory)
	ctx.RegisterModuleType("prebuilt_radio", PrebuiltRadioFactory)
	ctx.RegisterModuleType("prebuilt_rfs", PrebuiltRfsFactory)
	ctx.RegisterModuleType("prebuilt_framework", PrebuiltFrameworkFactory)
	ctx.RegisterModuleType("prebuilt_res", PrebuiltResFactory)
	ctx.RegisterModuleType("prebuilt_wlc_upt", PrebuiltWlcUptFactory)
	ctx.RegisterModuleType("prebuilt_odm", PrebuiltOdmFactory)
	ctx.RegisterModuleType("prebuilt_vendor_dlkm", PrebuiltVendorDlkmFactory)
	ctx.RegisterModuleType("prebuilt_vendor_overlay", PrebuiltVendorOverlayFactory)
	ctx.RegisterModuleType("prebuilt_bt_firmware", PrebuiltBtFirmwareFactory)
	ctx.RegisterModuleType("prebuilt_tvservice", PrebuiltTvServiceFactory)
	ctx.RegisterModuleType("prebuilt_optee", PrebuiltOpteeFactory)
	ctx.RegisterModuleType("prebuilt_tvconfig", PrebuiltTvConfigFactory)
	ctx.RegisterModuleType("prebuilt_vendor", PrebuiltVendorFactory)
	ctx.RegisterModuleType("prebuilt_sbin", PrebuiltSbinFactory)
	ctx.RegisterModuleType("prebuilt_system", PrebuiltSystemFactory)
	ctx.RegisterModuleType("prebuilt_first_stage_ramdisk", PrebuiltFirstStageRamdiskFactory)
	ctx.RegisterModuleType("prebuilt_any", PrebuiltAnyFactory)

	ctx.RegisterModuleType("prebuilt_defaults", defaultsFactory)

}

var PrepareForTestWithPrebuiltEtc = android.FixtureRegisterWithContext(RegisterPrebuiltEtcBuildComponents)

type PrebuiltEtcProperties struct {
	// Source file of this prebuilt. Can reference a genrule type module with the ":module" syntax.
	// Mutually exclusive with srcs.
	Src proptools.Configurable[string] `android:"path,arch_variant,replace_instead_of_append"`

	// Source files of this prebuilt. Can reference a genrule type module with the ":module" syntax.
	// Mutually exclusive with src. When used, filename_from_src is set to true unless dsts is also
	// set. May use globs in filenames.
	Srcs proptools.Configurable[[]string] `android:"path,arch_variant"`

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

	// Install to partition oem when set to true.
	Oem_specific *bool `android:"arch_variant"`
}

// Dsts is useful in that it allows prebuilt_* modules to easily map the source files to the
// install path within the partition. Dsts values are allowed to contain filepath separator
// so that the source files can be installed in subdirectories within the partition.
// However, this functionality should not be supported for prebuilt_root module type, as it
// allows the module to install to any arbitrary location. Thus, this property is defined in
// a separate struct so that it's not available to be set in prebuilt_root module type.
type PrebuiltDstsProperties struct {
	// Destination files of this prebuilt. Requires srcs to be used and causes srcs not to implicitly
	// set filename_from_src. This can be used to install each source file to a different directory
	// and/or change filenames when files are installed. Must be exactly one entry per source file,
	// which means care must be taken if srcs has globs.
	Dsts proptools.Configurable[[]string] `android:"path,arch_variant"`
}

type prebuiltSubdirProperties struct {
	// Optional subdirectory under which this file is installed into, cannot be specified with
	// relative_install_path, prefer relative_install_path.
	Sub_dir *string `android:"arch_variant"`

	// Optional subdirectory under which this file is installed into, cannot be specified with
	// sub_dir.
	Relative_install_path *string `android:"arch_variant"`
}

type prebuiltRootProperties struct {
	// Install this module to the root directory, without partition subdirs.  When this module is
	// added to PRODUCT_PACKAGES, this module will be installed to $PRODUCT_OUT/root, which will
	// then be copied to the root of system.img. When this module is packaged by other modules like
	// android_filesystem, this module will be installed to the root ("/"), unlike normal
	// prebuilt_root modules which are installed to the partition subdir (e.g. "/system/").
	Install_in_root *bool
}

type PrebuiltEtcModule interface {
	android.Module

	// Returns the base install directory, such as "etc", "usr/share".
	BaseDir() string

	// Returns the sub install directory relative to BaseDir().
	SubDir() string
}

type PrebuiltEtc struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties PrebuiltEtcProperties

	dstsProperties PrebuiltDstsProperties

	// rootProperties is used to return the value of the InstallInRoot() method. Currently, only
	// prebuilt_avb and prebuilt_root modules use this.
	rootProperties prebuiltRootProperties

	subdirProperties prebuiltSubdirProperties

	sourceFilePaths android.Paths
	outputFilePaths android.WritablePaths
	// The base install location, e.g. "etc" for prebuilt_etc, "usr/share" for prebuilt_usr_share.
	installDirBase               string
	installDirBase64             string
	installAvoidMultilibConflict bool
	// The base install location when soc_specific property is set to true, e.g. "firmware" for
	// prebuilt_firmware.
	socInstallDirBase      string
	installDirPaths        []android.InstallPath
	additionalDependencies *android.Paths

	usedSrcsProperty bool

	makeClass string
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

func (p *PrebuiltEtc) ImageMutatorBegin(ctx android.ImageInterfaceContext) {}

func (p *PrebuiltEtc) VendorVariantNeeded(ctx android.ImageInterfaceContext) bool {
	return false
}

func (p *PrebuiltEtc) ProductVariantNeeded(ctx android.ImageInterfaceContext) bool {
	return false
}

func (p *PrebuiltEtc) CoreVariantNeeded(ctx android.ImageInterfaceContext) bool {
	return !p.ModuleBase.InstallInRecovery() && !p.ModuleBase.InstallInRamdisk() &&
		!p.ModuleBase.InstallInVendorRamdisk() && !p.ModuleBase.InstallInDebugRamdisk()
}

func (p *PrebuiltEtc) RamdiskVariantNeeded(ctx android.ImageInterfaceContext) bool {
	return proptools.Bool(p.properties.Ramdisk_available) || p.ModuleBase.InstallInRamdisk()
}

func (p *PrebuiltEtc) VendorRamdiskVariantNeeded(ctx android.ImageInterfaceContext) bool {
	return proptools.Bool(p.properties.Vendor_ramdisk_available) || p.ModuleBase.InstallInVendorRamdisk()
}

func (p *PrebuiltEtc) DebugRamdiskVariantNeeded(ctx android.ImageInterfaceContext) bool {
	return proptools.Bool(p.properties.Debug_ramdisk_available) || p.ModuleBase.InstallInDebugRamdisk()
}

func (p *PrebuiltEtc) InstallInRoot() bool {
	return proptools.Bool(p.rootProperties.Install_in_root)
}

func (p *PrebuiltEtc) RecoveryVariantNeeded(ctx android.ImageInterfaceContext) bool {
	return proptools.Bool(p.properties.Recovery_available) || p.ModuleBase.InstallInRecovery()
}

func (p *PrebuiltEtc) ExtraImageVariations(ctx android.ImageInterfaceContext) []string {
	return nil
}

func (p *PrebuiltEtc) SetImageVariation(ctx android.ImageInterfaceContext, variation string) {
}

func (p *PrebuiltEtc) SourceFilePath(ctx android.ModuleContext) android.Path {
	if len(p.properties.Srcs.GetOrDefault(ctx, nil)) > 0 {
		panic(fmt.Errorf("SourceFilePath not available on multi-source prebuilt %q", p.Name()))
	}
	return android.PathForModuleSrc(ctx, p.properties.Src.GetOrDefault(ctx, ""))
}

func (p *PrebuiltEtc) InstallDirPath() android.InstallPath {
	if len(p.installDirPaths) != 1 {
		panic(fmt.Errorf("InstallDirPath not available on multi-source prebuilt %q", p.Name()))
	}
	return p.installDirPaths[0]
}

// This allows other derivative modules (e.g. prebuilt_etc_xml) to perform
// additional steps (like validating the src) before the file is installed.
func (p *PrebuiltEtc) SetAdditionalDependencies(paths android.Paths) {
	p.additionalDependencies = &paths
}

func (p *PrebuiltEtc) OutputFile() android.Path {
	if p.usedSrcsProperty {
		panic(fmt.Errorf("OutputFile not available on multi-source prebuilt %q", p.Name()))
	}
	return p.outputFilePaths[0]
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

	srcProperty := p.properties.Src.Get(ctx)
	srcsProperty := p.properties.Srcs.GetOrDefault(ctx, nil)
	if srcProperty.IsPresent() && len(srcsProperty) > 0 {
		ctx.PropertyErrorf("src", "src is set. Cannot set srcs")
	}
	dstsProperty := p.dstsProperties.Dsts.GetOrDefault(ctx, nil)
	if len(dstsProperty) > 0 && len(srcsProperty) == 0 {
		ctx.PropertyErrorf("dsts", "dsts is set. Must use srcs")
	}

	// Check that `sub_dir` and `relative_install_path` are not set at the same time.
	if p.subdirProperties.Sub_dir != nil && p.subdirProperties.Relative_install_path != nil {
		ctx.PropertyErrorf("sub_dir", "relative_install_path is set. Cannot set sub_dir")
	}
	baseInstallDirPath := android.PathForModuleInstall(ctx, p.installBaseDir(ctx), p.SubDir())
	// TODO(b/377304441)
	if android.Bool(p.properties.Oem_specific) {
		baseInstallDirPath = android.PathForModuleInPartitionInstall(ctx, ctx.DeviceConfig().OemPath(), p.installBaseDir(ctx), p.SubDir())
	}

	filename := proptools.String(p.properties.Filename)
	filenameFromSrc := proptools.Bool(p.properties.Filename_from_src)
	if srcProperty.IsPresent() {
		p.sourceFilePaths = android.PathsForModuleSrc(ctx, []string{srcProperty.Get()})
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
		p.outputFilePaths = android.WritablePaths{android.PathForModuleOut(ctx, filename)}

		ip := installProperties{
			filename:       filename,
			sourceFilePath: p.sourceFilePaths[0],
			outputFilePath: p.outputFilePaths[0],
			installDirPath: baseInstallDirPath,
			symlinks:       p.properties.Symlinks,
		}
		installs = append(installs, ip)
		p.installDirPaths = append(p.installDirPaths, baseInstallDirPath)
	} else if len(srcsProperty) > 0 {
		p.usedSrcsProperty = true
		if filename != "" {
			ctx.PropertyErrorf("filename", "filename cannot be set when using srcs")
		}
		if len(p.properties.Symlinks) > 0 {
			ctx.PropertyErrorf("symlinks", "symlinks cannot be set when using srcs")
		}
		if p.properties.Filename_from_src != nil {
			if len(dstsProperty) > 0 {
				ctx.PropertyErrorf("filename_from_src", "dsts is set. Cannot set filename_from_src")
			} else {
				ctx.PropertyErrorf("filename_from_src", "filename_from_src is implicitly set to true when using srcs")
			}
		}
		p.sourceFilePaths = android.PathsForModuleSrc(ctx, srcsProperty)
		if len(dstsProperty) > 0 && len(p.sourceFilePaths) != len(dstsProperty) {
			ctx.PropertyErrorf("dsts", "Must have one entry in dsts per source file")
		}
		for i, src := range p.sourceFilePaths {
			var filename string
			var installDirPath android.InstallPath

			if len(dstsProperty) > 0 {
				var dstdir string

				dstdir, filename = filepath.Split(dstsProperty[i])
				installDirPath = baseInstallDirPath.Join(ctx, dstdir)
			} else {
				filename = src.Base()
				installDirPath = baseInstallDirPath
			}
			output := android.PathForModuleOut(ctx, filename)
			ip := installProperties{
				filename:       filename,
				sourceFilePath: src,
				outputFilePath: output,
				installDirPath: installDirPath,
			}
			p.outputFilePaths = append(p.outputFilePaths, output)
			installs = append(installs, ip)
			p.installDirPaths = append(p.installDirPaths, installDirPath)
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
		p.outputFilePaths = android.WritablePaths{android.PathForModuleOut(ctx, filename)}
		ip := installProperties{
			filename:       filename,
			sourceFilePath: p.sourceFilePaths[0],
			outputFilePath: p.outputFilePaths[0],
			installDirPath: baseInstallDirPath,
		}
		installs = append(installs, ip)
		p.installDirPaths = append(p.installDirPaths, baseInstallDirPath)
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

	ctx.SetOutputFiles(p.outputFilePaths.Paths(), "")
}

type installProperties struct {
	filename       string
	sourceFilePath android.Path
	outputFilePath android.WritablePath
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
				entries.SetString("LOCAL_MODULE_PATH", p.installDirPaths[0].String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", p.outputFilePaths[0].Base())
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

func (p *PrebuiltEtc) AndroidModuleBase() *android.ModuleBase {
	return &p.ModuleBase
}

func InitPrebuiltEtcModule(p *PrebuiltEtc, dirBase string) {
	p.installDirBase = dirBase
	p.AddProperties(&p.properties)
	p.AddProperties(&p.subdirProperties)
	p.AddProperties(&p.rootProperties)
	p.AddProperties(&p.dstsProperties)
}

func InitPrebuiltRootModule(p *PrebuiltEtc) {
	p.installDirBase = "."
	p.AddProperties(&p.properties)
	p.AddProperties(&p.rootProperties)
}

func InitPrebuiltAvbModule(p *PrebuiltEtc) {
	p.installDirBase = "avb"
	p.AddProperties(&p.properties)
	p.AddProperties(&p.dstsProperties)
	p.rootProperties.Install_in_root = proptools.BoolPtr(true)
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
		&PrebuiltEtcProperties{},
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

// prebuilt_any is a special module where the module can define the subdirectory that the files
// are installed to. This is only used for converting the PRODUCT_COPY_FILES entries to Soong
// modules, and should never be defined in the bp files. If none of the existing prebuilt_*
// modules allow installing the file at the desired location, introduce a new prebuilt_* module
// type instead.
func PrebuiltAnyFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, ".")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
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

// Generally, a <partition> directory will contain a `system` subdirectory, but the <partition> of
// `prebuilt_avb` will not have a `system` subdirectory.
// Ultimately, prebuilt_avb will install the prebuilt artifact to the `avb` subdirectory under the
// root directory of the partition: <partition_root>/avb.
// prebuilt_avb does not allow adding any other subdirectories.
func PrebuiltAvbFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltAvbModule(module)
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	android.InitDefaultableModule(module)
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

// prebuilt_usr_hyphendata is for a prebuilt artifact that is installed in
// <partition>/usr/hyphen-data/<sub_dir> directory.
func PrebuiltUserHyphenDataFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "usr/hyphen-data")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_usr_keylayout is for a prebuilt artifact that is installed in
// <partition>/usr/keylayout/<sub_dir> directory.
func PrebuiltUserKeyLayoutFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "usr/keylayout")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_usr_keychars is for a prebuilt artifact that is installed in
// <partition>/usr/keychars/<sub_dir> directory.
func PrebuiltUserKeyCharsFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "usr/keychars")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_usr_idc is for a prebuilt artifact that is installed in
// <partition>/usr/idc/<sub_dir> directory.
func PrebuiltUserIdcFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "usr/idc")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_usr_srec is for a prebuilt artifact that is installed in
// <partition>/usr/srec/<sub_dir> directory.
func PrebuiltUserSrecFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "usr/srec")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_font installs a font in <partition>/fonts directory.
func PrebuiltFontFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "fonts")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_overlay is for a prebuilt artifact in <partition>/overlay directory.
func PrebuiltOverlayFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "overlay")
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
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_gpu is for a prebuilt artifact in <partition>/gpu directory.
func PrebuiltGPUFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "gpu")
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

// prebuilt_media installs media files in <partition>/media directory.
func PrebuiltMediaFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "media")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_voicepack installs voice pack files in <partition>/tts directory.
func PrebuiltVoicepackFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "tts")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_bin installs files in <partition>/bin directory.
func PrebuiltBinaryFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "bin")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_wallpaper installs image files in <partition>/wallpaper directory.
func PrebuiltWallpaperFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "wallpaper")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_priv_app installs files in <partition>/priv-app directory.
func PrebuiltPrivAppFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "priv-app")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_radio installs files in <partition>/radio directory.
func PrebuiltRadioFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "radio")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_rfs installs files in <partition>/rfs directory.
func PrebuiltRfsFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "rfs")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_framework installs files in <partition>/framework directory.
func PrebuiltFrameworkFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "framework")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_res installs files in <partition>/res directory.
func PrebuiltResFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "res")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_wlc_upt installs files in <partition>/wlc_upt directory.
func PrebuiltWlcUptFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "wlc_upt")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_odm installs files in <partition>/odm directory.
func PrebuiltOdmFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "odm")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_vendor_dlkm installs files in <partition>/vendor_dlkm directory.
func PrebuiltVendorDlkmFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "vendor_dlkm")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_bt_firmware installs files in <partition>/bt_firmware directory.
func PrebuiltBtFirmwareFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "bt_firmware")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_tvservice installs files in <partition>/tvservice directory.
func PrebuiltTvServiceFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "tvservice")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_optee installs files in <partition>/optee directory.
func PrebuiltOpteeFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "optee")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_tvconfig installs files in <partition>/tvconfig directory.
func PrebuiltTvConfigFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "tvconfig")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_vendor installs files in <partition>/vendor directory.
func PrebuiltVendorFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "vendor")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_vendor_overlay is for a prebuilt artifact in <partition>/vendor_overlay directory.
func PrebuiltVendorOverlayFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "vendor_overlay")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_sbin installs files in <partition>/sbin directory.
func PrebuiltSbinFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "sbin")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_system installs files in <partition>/system directory.
func PrebuiltSystemFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "system")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

// prebuilt_first_stage_ramdisk installs files in <partition>/first_stage_ramdisk directory.
func PrebuiltFirstStageRamdiskFactory() android.Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module, "first_stage_ramdisk")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}
