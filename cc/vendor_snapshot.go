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

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/snapshot"
)

// This file defines how to capture cc modules into snapshot package.

// Checks if the target image would contain VNDK
func includeVndk(image snapshot.SnapshotImage) bool {
	if image.ImageName() == snapshot.VendorSnapshotImageName {
		return true
	}

	return false
}

// Check if the module is VNDK private
func isPrivate(image snapshot.SnapshotImage, m LinkableInterface) bool {
	if image.ImageName() == snapshot.VendorSnapshotImageName && m.IsVndkPrivate() {
		return true
	}

	return false
}

// Checks if target image supports VNDK Ext
func supportsVndkExt(image snapshot.SnapshotImage) bool {
	if image.ImageName() == snapshot.VendorSnapshotImageName {
		return true
	}

	return false
}

// Determines if the module is a candidate for snapshot.
func isSnapshotAware(cfg android.DeviceConfig, m LinkableInterface, inProprietaryPath bool, apexInfo android.ApexInfo, image snapshot.SnapshotImage) bool {
	if !m.Enabled() || m.HiddenFromMake() {
		return false
	}
	// When android/prebuilt.go selects between source and prebuilt, it sets
	// HideFromMake on the other one to avoid duplicate install rules in make.
	if m.IsHideFromMake() {
		return false
	}
	// skip proprietary modules, but (for the vendor snapshot only)
	// include all VNDK (static)
	if inProprietaryPath && (!includeVndk(image) || !m.IsVndk()) {
		return false
	}
	// If the module would be included based on its path, check to see if
	// the module is marked to be excluded. If so, skip it.
	if image.ExcludeFromSnapshot(m) {
		return false
	}
	if m.Target().Os.Class != android.Device {
		return false
	}
	if m.Target().NativeBridge == android.NativeBridgeEnabled {
		return false
	}
	// the module must be installed in target image
	if !apexInfo.IsForPlatform() || m.IsSnapshotPrebuilt() || !image.InImage(m)() {
		return false
	}
	// skip kernel_headers which always depend on vendor
	if m.KernelHeadersDecorator() {
		return false
	}

	if m.IsLlndk() {
		return false
	}

	// Libraries
	if sanitizable, ok := m.(PlatformSanitizeable); ok && sanitizable.IsSnapshotLibrary() {
		if sanitizable.SanitizePropDefined() {
			// scs exports both sanitized and unsanitized variants for static and header
			// Always use unsanitized variant of it.
			if !sanitizable.Shared() && sanitizable.IsSanitizerEnabled(scs) {
				return false
			}
			// cfi and hwasan cannot be enabled at the same time.
			// Skip variants that have both cfi and hwasan enabled.
			if sanitizable.IsSanitizerEnabled(cfi) && sanitizable.IsSanitizerEnabled(Hwasan) {
				return false
			}
			// cfi and hwasan also export both variants. But for static, we capture both.
			// This is because cfi static libraries can't be linked from non-cfi modules,
			// and vice versa.
			// hwasan is captured as well to support hwasan build.
			if !sanitizable.Static() &&
				!sanitizable.Shared() &&
				(sanitizable.IsSanitizerEnabled(cfi) || sanitizable.IsSanitizerEnabled(Hwasan)) {
				return false
			}
		}
		if sanitizable.Static() || sanitizable.Rlib() {
			return sanitizable.OutputFile().Valid() && !isPrivate(image, m)
		}
		if sanitizable.Shared() || sanitizable.Dylib() {
			if !sanitizable.OutputFile().Valid() {
				return false
			}
			if includeVndk(image) {
				if !sanitizable.IsVndk() {
					return true
				}
				return sanitizable.IsVndkExt()
			}
		}
		return true
	}

	// Binaries and Objects
	if m.Binary() || m.Object() {
		return m.OutputFile().Valid()
	}

	return false
}

// Extend the snapshot.SnapshotJsonFlags to include cc specific fields.
type snapshotJsonFlags struct {
	snapshot.SnapshotJsonFlags
	// library flags
	ExportedDirs       []string `json:",omitempty"`
	ExportedSystemDirs []string `json:",omitempty"`
	ExportedFlags      []string `json:",omitempty"`
	Sanitize           string   `json:",omitempty"`
	SanitizeMinimalDep bool     `json:",omitempty"`
	SanitizeUbsanDep   bool     `json:",omitempty"`

	// binary flags
	Symlinks         []string `json:",omitempty"`
	StaticExecutable bool     `json:",omitempty"`
	InstallInRoot    bool     `json:",omitempty"`

	// dependencies
	SharedLibs  []string `json:",omitempty"`
	StaticLibs  []string `json:",omitempty"`
	RuntimeLibs []string `json:",omitempty"`
	Dylibs      []string `json:",omitempty"`
	Rlibs       []string `json:",omitempty"`

	// extra config files
	InitRc         []string `json:",omitempty"`
	VintfFragments []string `json:",omitempty"`
	MinSdkVersion  string   `json:",omitempty"`
}

var ccSnapshotAction snapshot.GenerateSnapshotAction = func(s snapshot.SnapshotSingleton, ctx android.SingletonContext, snapshotArchDir string) snapshot.SnapshotPaths {
	/*
		Vendor snapshot zipped artifacts directory structure for cc modules:
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
				object/
					(.o object files)
			arch-{TARGET_2ND_ARCH}-{TARGET_2ND_ARCH_VARIANT}/
				shared/
					(.so shared libraries)
				static/
					(.a static libraries)
				header/
					(header only libraries)
				binary/
					(executable binaries)
				object/
					(.o object files)
			NOTICE_FILES/
				(notice files, e.g. libbase.txt)
			configs/
				(config files, e.g. init.rc files, vintf_fragments.xml files, etc.)
			include/
				(header files of same directory structure with source tree)
	*/

	var snapshotOutputs android.Paths
	var snapshotNotices android.Paths

	includeDir := filepath.Join(snapshotArchDir, "include")
	configsDir := filepath.Join(snapshotArchDir, "configs")

	installedNotices := make(map[string]bool)
	installedConfigs := make(map[string]bool)

	var headers android.Paths

	copyFile := func(ctx android.SingletonContext, path android.Path, out string, fake bool) android.OutputPath {
		if fake {
			// All prebuilt binaries and headers are installed by copyFile function. This makes a fake
			// snapshot just touch prebuilts and headers, rather than installing real files.
			return snapshot.WriteStringToFileRule(ctx, "", out)
		} else {
			return snapshot.CopyFileRule(pctx, ctx, path, out)
		}
	}

	// installSnapshot function copies prebuilt file (.so, .a, or executable) and json flag file.
	// For executables, init_rc and vintf_fragments files are also copied.
	installSnapshot := func(m LinkableInterface, fake bool) android.Paths {
		targetArch := "arch-" + m.Target().Arch.ArchType.String()
		if m.Target().Arch.ArchVariant != "" {
			targetArch += "-" + m.Target().Arch.ArchVariant
		}

		var ret android.Paths

		prop := snapshotJsonFlags{}

		// Common properties among snapshots.
		prop.InitBaseSnapshotPropsWithName(m, ctx.ModuleName(m))
		if supportsVndkExt(s.Image) && m.IsVndkExt() {
			// vndk exts are installed to /vendor/lib(64)?/vndk(-sp)?
			if m.IsVndkSp() {
				prop.RelativeInstallPath = "vndk-sp"
			} else {
				prop.RelativeInstallPath = "vndk"
			}
		} else {
			prop.RelativeInstallPath = m.RelativeInstallPath()
		}
		prop.RuntimeLibs = m.SnapshotRuntimeLibs()
		prop.Required = m.RequiredModuleNames()
		if o, ok := m.(overridable); ok {
			prop.Overrides = o.overriddenModules()
		}
		for _, path := range m.InitRc() {
			prop.InitRc = append(prop.InitRc, filepath.Join("configs", path.Base()))
		}
		for _, path := range m.VintfFragments() {
			prop.VintfFragments = append(prop.VintfFragments, filepath.Join("configs", path.Base()))
		}
		if m.IsPrebuilt() {
			prop.MinSdkVersion = "apex_inherit"
		} else {
			prop.MinSdkVersion = m.MinSdkVersion()
		}

		// install config files. ignores any duplicates.
		for _, path := range append(m.InitRc(), m.VintfFragments()...) {
			out := filepath.Join(configsDir, path.Base())
			if !installedConfigs[out] {
				installedConfigs[out] = true
				ret = append(ret, copyFile(ctx, path, out, fake))
			}
		}

		var propOut string

		if m.IsSnapshotLibrary() {
			exporterInfo, _ := android.SingletonModuleProvider(ctx, m.Module(), FlagExporterInfoProvider)

			// library flags
			prop.ExportedFlags = exporterInfo.Flags
			for _, dir := range exporterInfo.IncludeDirs {
				prop.ExportedDirs = append(prop.ExportedDirs, filepath.Join("include", dir.String()))
			}
			for _, dir := range exporterInfo.SystemIncludeDirs {
				prop.ExportedSystemDirs = append(prop.ExportedSystemDirs, filepath.Join("include", dir.String()))
			}

			// shared libs dependencies aren't meaningful on static or header libs
			if m.Shared() {
				prop.SharedLibs = m.SnapshotSharedLibs()
			}

			// dylibs collect both shared and dylib dependencies.
			if m.Dylib() {
				prop.SharedLibs = m.SnapshotSharedLibs()
				prop.Dylibs = m.SnapshotDylibs()
			}

			// static and rlib libs dependencies are required to collect the NOTICE files.
			prop.StaticLibs = m.SnapshotStaticLibs()
			prop.Rlibs = m.SnapshotRlibs()

			if sanitizable, ok := m.(PlatformSanitizeable); ok {
				if sanitizable.Static() && sanitizable.SanitizePropDefined() {
					prop.SanitizeMinimalDep = sanitizable.MinimalRuntimeDep() || sanitizable.MinimalRuntimeNeeded()
					prop.SanitizeUbsanDep = sanitizable.UbsanRuntimeDep() || sanitizable.UbsanRuntimeNeeded()
				}
			}

			var libType string
			if m.Static() {
				libType = "static"
			} else if m.Shared() {
				libType = "shared"
			} else if m.Rlib() {
				libType = "rlib"
			} else if m.Dylib() {
				libType = "dylib"
			} else {
				libType = "header"
			}

			var stem string

			// install .a, .rlib, .dylib.so, or .so
			if libType != "header" {
				libPath := m.OutputFile().Path()
				stem = libPath.Base()
				if sanitizable, ok := m.(PlatformSanitizeable); ok {
					if (sanitizable.Static() || sanitizable.Rlib()) && sanitizable.SanitizePropDefined() {
						if sanitizable.IsSanitizerEnabled(cfi) {
							// both cfi and non-cfi variant for static libraries can exist.
							// attach .cfi to distinguish between cfi and non-cfi.
							// e.g. libbase.a -> libbase.cfi.a
							ext := filepath.Ext(stem)
							stem = strings.TrimSuffix(stem, ext) + ".cfi" + ext
							prop.Sanitize = "cfi"
							prop.ModuleName += ".cfi"
						} else if sanitizable.IsSanitizerEnabled(Hwasan) {
							// Same for the hwasan
							ext := filepath.Ext(stem)
							stem = strings.TrimSuffix(stem, ext) + ".hwasan" + ext
							prop.Sanitize = "hwasan"
							prop.ModuleName += ".hwasan"
						}
					}
				}
				if m.Rlib() && m.RlibStd() {
					// rlibs produce both rlib-std and dylib-std variants
					ext := filepath.Ext(stem)
					stem = strings.TrimSuffix(stem, ext) + ".rlib-std" + ext
					prop.ModuleName += ".rlib-std"
				}
				snapshotLibOut := filepath.Join(snapshotArchDir, targetArch, libType, m.RelativeInstallPath(), stem)
				ret = append(ret, copyFile(ctx, libPath, snapshotLibOut, fake))
			} else {
				stem = ctx.ModuleName(m)
			}

			propOut = filepath.Join(snapshotArchDir, targetArch, libType, m.RelativeInstallPath(), stem+".json")
		} else if m.Binary() {
			// binary flags
			prop.Symlinks = m.Symlinks()
			prop.StaticExecutable = m.StaticExecutable()
			prop.InstallInRoot = m.InstallInRoot()
			prop.SharedLibs = m.SnapshotSharedLibs()
			prop.Dylibs = m.SnapshotDylibs()

			// static and rlib dependencies are required to collect the NOTICE files.
			prop.StaticLibs = m.SnapshotStaticLibs()
			prop.Rlibs = m.SnapshotRlibs()

			// install bin
			binPath := m.OutputFile().Path()
			snapshotBinOut := filepath.Join(snapshotArchDir, targetArch, "binary", binPath.Base())
			ret = append(ret, copyFile(ctx, binPath, snapshotBinOut, fake))
			propOut = snapshotBinOut + ".json"
		} else if m.Object() {
			// object files aren't installed to the device, so their names can conflict.
			// Use module name as stem.
			objPath := m.OutputFile().Path()
			snapshotObjOut := filepath.Join(snapshotArchDir, targetArch, "object",
				ctx.ModuleName(m)+filepath.Ext(objPath.Base()))
			ret = append(ret, copyFile(ctx, objPath, snapshotObjOut, fake))
			propOut = snapshotObjOut + ".json"
		} else {
			ctx.Errorf("unknown module %q in vendor snapshot", m.String())
			return nil
		}

		j, err := json.Marshal(prop)
		if err != nil {
			ctx.Errorf("json marshal to %q failed: %#v", propOut, err)
			return nil
		}
		ret = append(ret, snapshot.WriteStringToFileRule(ctx, string(j), propOut))

		return ret
	}

	ctx.VisitAllModules(func(module android.Module) {
		m, ok := module.(LinkableInterface)
		if !ok {
			return
		}

		moduleDir := ctx.ModuleDir(module)
		inProprietaryPath := s.Image.IsProprietaryPath(moduleDir, ctx.DeviceConfig())
		apexInfo, _ := android.SingletonModuleProvider(ctx, module, android.ApexInfoProvider)

		if s.Image.ExcludeFromSnapshot(m) {
			if inProprietaryPath {
				// Error: exclude_from_vendor_snapshot applies
				// to framework-path modules only.
				ctx.Errorf("module %q in vendor proprietary path %q may not use \"exclude_from_vendor_snapshot: true\"", m.String(), moduleDir)
				return
			}
		}

		if !isSnapshotAware(ctx.DeviceConfig(), m, inProprietaryPath, apexInfo, s.Image) {
			return
		}

		// If we are using directed snapshot and a module is not included in the
		// list, we will still include the module as if it was a fake module.
		// The reason is that soong needs all the dependencies to be present, even
		// if they are not using during the build.
		installAsFake := s.Fake
		if s.Image.ExcludeFromDirectedSnapshot(ctx.DeviceConfig(), m.BaseModuleName()) {
			installAsFake = true
		}

		// installSnapshot installs prebuilts and json flag files
		snapshotOutputs = append(snapshotOutputs, installSnapshot(m, installAsFake)...)
		// just gather headers and notice files here, because they are to be deduplicated
		if m.IsSnapshotLibrary() {
			headers = append(headers, m.SnapshotHeaders()...)
		}

		for _, notice := range m.EffectiveLicenseFiles() {
			if _, ok := installedNotices[notice.String()]; !ok {
				installedNotices[notice.String()] = true
				snapshotNotices = append(snapshotNotices, notice)
			}
		}
	})

	// install all headers after removing duplicates
	for _, header := range android.FirstUniquePaths(headers) {
		snapshotOutputs = append(snapshotOutputs, copyFile(ctx, header, filepath.Join(includeDir, header.String()), s.Fake))
	}

	return snapshot.SnapshotPaths{OutputFiles: snapshotOutputs, NoticeFiles: snapshotNotices}
}

func init() {
	snapshot.RegisterSnapshotAction(ccSnapshotAction)
}
