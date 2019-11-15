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

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func init() {
	android.RegisterSingletonType("vendor-snapshot", VendorSnapshotSingleton)
}

func VendorSnapshotSingleton() android.Singleton {
	return &vendorSnapshotSingleton{}
}

type vendorSnapshotSingleton struct {
	vendorSnapshotZipFile android.OptionalPath
}

var (
	// Modules under following directories are ignored. They are OEM's and vendor's
	// proprietary modules(device/, vendor/, and hardware/).
	// TODO(b/65377115): Clean up these with more maintainable way
	vendorProprietaryDirs = []string{
		"device",
		"vendor",
		"hardware",
	}

	// Modules under following directories are included as they are in AOSP,
	// although hardware/ is normally for vendor's own.
	// TODO(b/65377115): Clean up these with more maintainable way
	aospDirsUnderProprietary = []string{
		"hardware/interfaces",
		"hardware/libhardware",
		"hardware/libhardware_legacy",
		"hardware/ril",
	}
)

// Determine if a dir under source tree is an SoC-owned proprietary directory, such as
// device/, vendor/, etc.
func isVendorProprietaryPath(dir string) bool {
	for _, p := range vendorProprietaryDirs {
		if strings.HasPrefix(dir, p) {
			// filter out AOSP defined directories, e.g. hardware/interfaces/
			aosp := false
			for _, p := range aospDirsUnderProprietary {
				if strings.HasPrefix(dir, p) {
					aosp = true
					break
				}
			}
			if !aosp {
				return true
			}
		}
	}
	return false
}

// Determine if a module is going to be included in vendor snapshot or not.
//
// Targets of vendor snapshot are "vendor: true" or "vendor_available: true" modules in
// AOSP. They are not guaranteed to be compatible with older vendor images. (e.g. might
// depend on newer VNDK) So they are captured as vendor snapshot To build older vendor
// image and newer system image altogether.
func isVendorSnapshotModule(ctx android.SingletonContext, m *Module) bool {
	if !m.Enabled() {
		return false
	}
	// skip proprietary modules, but include all VNDK (static)
	if isVendorProprietaryPath(ctx.ModuleDir(m)) && !m.IsVndk() {
		return false
	}
	if m.Target().Os.Class != android.Device {
		return false
	}
	if m.Target().NativeBridge == android.NativeBridgeEnabled {
		return false
	}
	// the module must be installed in /vendor
	if !m.installable() || m.isSnapshotPrebuilt() || !m.inVendor() {
		return false
	}
	// exclude test modules
	if _, ok := m.linker.(interface{ gtest() bool }); ok {
		return false
	}
	// TODO(b/65377115): add full support for sanitizer
	if m.sanitize != nil && !m.sanitize.isUnsanitizedVariant() {
		return false
	}

	// Libraries
	if l, ok := m.linker.(snapshotLibraryInterface); ok {
		if l.static() {
			return proptools.BoolDefault(m.VendorProperties.Vendor_available, true)
		}
		if l.shared() {
			return !m.IsVndk()
		}
		return true
	}

	// Binaries
	_, ok := m.linker.(*binaryDecorator)
	if !ok {
		if _, ok := m.linker.(*prebuiltBinaryLinker); !ok {
			return false
		}
	}
	return proptools.BoolDefault(m.VendorProperties.Vendor_available, true)
}

func (c *vendorSnapshotSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	// BOARD_VNDK_VERSION must be set to 'current' in order to generate a vendor snapshot.
	if ctx.DeviceConfig().VndkVersion() != "current" {
		return
	}

	var snapshotOutputs android.Paths

	/*
		Vendor snapshot zipped artifacts directory structure:
		{SNAPSHOT_ARCH}/
			arch-{TARGET_ARCH}-{TARGET_ARCH_VARIANT}/
				shared/
					(.so shared libraries)
				static/
					(.a static libraries)
				header/
					(header only libraries)
				binary/
					(executable binaries)
			arch-{TARGET_2ND_ARCH}-{TARGET_2ND_ARCH_VARIANT}/
				shared/
					(.so shared libraries)
				static/
					(.a static libraries)
				header/
					(header only libraries)
				binary/
					(executable binaries)
			NOTICE_FILES/
				(notice files, e.g. libbase.txt)
			configs/
				(config files, e.g. init.rc files, vintf_fragments.xml files, etc.)
			include/
				(header files of same directory structure with source tree)
	*/

	snapshotDir := "vendor-snapshot"
	snapshotArchDir := filepath.Join(snapshotDir, ctx.DeviceConfig().DeviceArch())

	includeDir := filepath.Join(snapshotArchDir, "include")
	configsDir := filepath.Join(snapshotArchDir, "configs")
	noticeDir := filepath.Join(snapshotArchDir, "NOTICE_FILES")

	installedNotices := make(map[string]bool)
	installedConfigs := make(map[string]bool)

	var headers android.Paths

	type vendorSnapshotLibraryInterface interface {
		exportedFlagsProducer
		libraryInterface
	}

	var _ vendorSnapshotLibraryInterface = (*prebuiltLibraryLinker)(nil)
	var _ vendorSnapshotLibraryInterface = (*libraryDecorator)(nil)

	installSnapshot := func(m *Module) android.Paths {
		targetArch := "arch-" + m.Target().Arch.ArchType.String()
		if m.Target().Arch.ArchVariant != "" {
			targetArch += "-" + m.Target().Arch.ArchVariant
		}

		var ret android.Paths

		prop := struct {
			ModuleName          string `json:",omitempty"`
			RelativeInstallPath string `json:",omitempty"`

			// library flags
			ExportedDirs       []string `json:",omitempty"`
			ExportedSystemDirs []string `json:",omitempty"`
			ExportedFlags      []string `json:",omitempty"`
			SanitizeMinimalDep bool     `json:",omitempty"`
			SanitizeUbsanDep   bool     `json:",omitempty"`

			// binary flags
			Symlinks []string `json:",omitempty"`

			// dependencies
			SharedLibs  []string `json:",omitempty"`
			RuntimeLibs []string `json:",omitempty"`
			Required    []string `json:",omitempty"`

			// extra config files
			InitRc         []string `json:",omitempty"`
			VintfFragments []string `json:",omitempty"`
		}{}

		// Common properties among snapshots.
		prop.ModuleName = ctx.ModuleName(m)
		prop.RelativeInstallPath = m.RelativeInstallPath()
		prop.RuntimeLibs = m.Properties.SnapshotRuntimeLibs
		prop.Required = m.RequiredModuleNames()
		for _, path := range m.InitRc() {
			prop.InitRc = append(prop.InitRc, filepath.Join("configs", path.Base()))
		}
		for _, path := range m.VintfFragments() {
			prop.VintfFragments = append(prop.VintfFragments, filepath.Join("configs", path.Base()))
		}

		// install config files. ignores any duplicates.
		for _, path := range append(m.InitRc(), m.VintfFragments()...) {
			out := filepath.Join(configsDir, path.Base())
			if !installedConfigs[out] {
				installedConfigs[out] = true
				ret = append(ret, copyFile(ctx, path, out))
			}
		}

		var propOut string

		if l, ok := m.linker.(vendorSnapshotLibraryInterface); ok {
			// library flags
			prop.ExportedFlags = l.exportedFlags()
			for _, dir := range l.exportedDirs() {
				prop.ExportedDirs = append(prop.ExportedDirs, filepath.Join("include", dir.String()))
			}
			for _, dir := range l.exportedSystemDirs() {
				prop.ExportedSystemDirs = append(prop.ExportedSystemDirs, filepath.Join("include", dir.String()))
			}
			// shared libs dependencies aren't meaningful on static or header libs
			if l.shared() {
				prop.SharedLibs = m.Properties.SnapshotSharedLibs
			}
			if l.static() && m.sanitize != nil {
				prop.SanitizeMinimalDep = m.sanitize.Properties.MinimalRuntimeDep || enableMinimalRuntime(m.sanitize)
				prop.SanitizeUbsanDep = m.sanitize.Properties.UbsanRuntimeDep || enableUbsanRuntime(m.sanitize)
			}

			var libType string
			if l.static() {
				libType = "static"
			} else if l.shared() {
				libType = "shared"
			} else {
				libType = "header"
			}

			var stem string

			// install .a or .so
			if libType != "header" {
				libPath := m.outputFile.Path()
				stem = libPath.Base()
				snapshotLibOut := filepath.Join(snapshotArchDir, targetArch, libType, stem)
				ret = append(ret, copyFile(ctx, libPath, snapshotLibOut))
			} else {
				stem = ctx.ModuleName(m)
			}

			propOut = filepath.Join(snapshotArchDir, targetArch, libType, stem+".json")
		} else {
			// binary flags
			prop.Symlinks = m.Symlinks()
			prop.SharedLibs = m.Properties.SnapshotSharedLibs

			// install bin
			binPath := m.outputFile.Path()
			snapshotBinOut := filepath.Join(snapshotArchDir, targetArch, "binary", binPath.Base())
			ret = append(ret, copyFile(ctx, binPath, snapshotBinOut))
			propOut = snapshotBinOut + ".json"
		}

		j, err := json.Marshal(prop)
		if err != nil {
			ctx.Errorf("json marshal to %q failed: %#v", propOut, err)
			return nil
		}
		ret = append(ret, writeStringToFile(ctx, string(j), propOut))

		return ret
	}

	ctx.VisitAllModules(func(module android.Module) {
		m, ok := module.(*Module)
		if !ok || !isVendorSnapshotModule(ctx, m) {
			return
		}

		snapshotOutputs = append(snapshotOutputs, installSnapshot(m)...)
		if l, ok := m.linker.(vendorSnapshotLibraryInterface); ok {
			headers = append(headers, exportedHeaders(ctx, l)...)
		}

		if m.NoticeFile().Valid() {
			noticeName := ctx.ModuleName(m) + ".txt"
			noticeOut := filepath.Join(noticeDir, noticeName)
			// skip already copied notice file
			if !installedNotices[noticeOut] {
				installedNotices[noticeOut] = true
				snapshotOutputs = append(snapshotOutputs, copyFile(
					ctx, m.NoticeFile().Path(), noticeOut))
			}
		}
	})

	// install all headers after removing duplicates
	for _, header := range android.FirstUniquePaths(headers) {
		snapshotOutputs = append(snapshotOutputs, copyFile(
			ctx, header, filepath.Join(includeDir, header.String())))
	}

	// All artifacts are ready. Sort them to normalize ninja and then zip.
	sort.Slice(snapshotOutputs, func(i, j int) bool {
		return snapshotOutputs[i].String() < snapshotOutputs[j].String()
	})

	zipPath := android.PathForOutput(ctx, snapshotDir, "vendor-"+ctx.Config().DeviceName()+".zip")
	zipRule := android.NewRuleBuilder()

	// filenames in rspfile from FlagWithRspFileInputList might be single-quoted. Remove it with tr
	snapshotOutputList := android.PathForOutput(ctx, snapshotDir, "vendor-"+ctx.Config().DeviceName()+"_list")
	zipRule.Command().
		Text("tr").
		FlagWithArg("-d ", "\\'").
		FlagWithRspFileInputList("< ", snapshotOutputs).
		FlagWithOutput("> ", snapshotOutputList)

	zipRule.Temporary(snapshotOutputList)

	zipRule.Command().
		BuiltTool(ctx, "soong_zip").
		FlagWithOutput("-o ", zipPath).
		FlagWithArg("-C ", android.PathForOutput(ctx, snapshotDir).String()).
		FlagWithInput("-l ", snapshotOutputList)

	zipRule.Build(pctx, ctx, zipPath.String(), "vendor snapshot "+zipPath.String())
	zipRule.DeleteTemporaryFiles()
	c.vendorSnapshotZipFile = android.OptionalPathForPath(zipPath)
}

func (c *vendorSnapshotSingleton) MakeVars(ctx android.MakeVarsContext) {
	ctx.Strict("SOONG_VENDOR_SNAPSHOT_ZIP", c.vendorSnapshotZipFile.String())
}
