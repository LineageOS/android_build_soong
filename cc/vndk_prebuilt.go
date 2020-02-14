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
	"strings"

	"android/soong/android"
)

var (
	vndkSuffix     = ".vndk."
	binder32Suffix = ".binder32"
)

// Creates vndk prebuilts that include the VNDK version.
//
// Example:
//
// vndk_prebuilt_shared {
//     name: "libfoo",
//     version: "27",
//     target_arch: "arm64",
//     vendor_available: true,
//     vndk: {
//         enabled: true,
//     },
//     export_include_dirs: ["include/external/libfoo/vndk_include"],
//     arch: {
//         arm64: {
//             srcs: ["arm/lib64/libfoo.so"],
//         },
//         arm: {
//             srcs: ["arm/lib/libfoo.so"],
//         },
//     },
// }
//
type vndkPrebuiltProperties struct {
	// VNDK snapshot version.
	Version *string

	// Target arch name of the snapshot (e.g. 'arm64' for variant 'aosp_arm64_ab')
	Target_arch *string

	// If the prebuilt snapshot lib is built with 32 bit binder, this must be set to true.
	// The lib with 64 bit binder does not need to set this property.
	Binder32bit *bool

	// Prebuilt files for each arch.
	Srcs []string `android:"arch_variant"`

	// list of flags that will be used for any module that links against this module.
	Export_flags []string `android:"arch_variant"`

	// Check the prebuilt ELF files (e.g. DT_SONAME, DT_NEEDED, resolution of undefined symbols,
	// etc).
	Check_elf_files *bool
}

type vndkPrebuiltLibraryDecorator struct {
	*libraryDecorator
	properties vndkPrebuiltProperties
}

func (p *vndkPrebuiltLibraryDecorator) Name(name string) string {
	return name + p.NameSuffix()
}

func (p *vndkPrebuiltLibraryDecorator) NameSuffix() string {
	suffix := p.version()
	if p.arch() != "" {
		suffix += "." + p.arch()
	}
	if Bool(p.properties.Binder32bit) {
		suffix += binder32Suffix
	}
	return vndkSuffix + suffix
}

func (p *vndkPrebuiltLibraryDecorator) version() string {
	return String(p.properties.Version)
}

func (p *vndkPrebuiltLibraryDecorator) arch() string {
	return String(p.properties.Target_arch)
}

func (p *vndkPrebuiltLibraryDecorator) binderBit() string {
	if Bool(p.properties.Binder32bit) {
		return "32"
	}
	return "64"
}

func (p *vndkPrebuiltLibraryDecorator) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	p.libraryDecorator.libName = strings.TrimSuffix(ctx.ModuleName(), p.NameSuffix())
	return p.libraryDecorator.linkerFlags(ctx, flags)
}

func (p *vndkPrebuiltLibraryDecorator) singleSourcePath(ctx ModuleContext) android.Path {
	if len(p.properties.Srcs) == 0 {
		ctx.PropertyErrorf("srcs", "missing prebuilt source file")
		return nil
	}

	if len(p.properties.Srcs) > 1 {
		ctx.PropertyErrorf("srcs", "multiple prebuilt source files")
		return nil
	}

	return android.PathForModuleSrc(ctx, p.properties.Srcs[0])
}

func (p *vndkPrebuiltLibraryDecorator) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {

	if !p.matchesWithDevice(ctx.DeviceConfig()) {
		ctx.Module().SkipInstall()
		return nil
	}

	if len(p.properties.Srcs) > 0 && p.shared() {
		p.libraryDecorator.exportIncludes(ctx)
		p.libraryDecorator.reexportFlags(p.properties.Export_flags...)
		// current VNDK prebuilts are only shared libs.

		in := p.singleSourcePath(ctx)
		builderFlags := flagsToBuilderFlags(flags)
		p.unstrippedOutputFile = in
		libName := in.Base()
		if p.needsStrip(ctx) {
			stripped := android.PathForModuleOut(ctx, "stripped", libName)
			p.stripExecutableOrSharedLib(ctx, in, stripped, builderFlags)
			in = stripped
		}

		// Optimize out relinking against shared libraries whose interface hasn't changed by
		// depending on a table of contents file instead of the library itself.
		tocFile := android.PathForModuleOut(ctx, libName+".toc")
		p.tocFile = android.OptionalPathForPath(tocFile)
		TransformSharedObjectToToc(ctx, in, tocFile, builderFlags)

		return in
	}

	ctx.Module().SkipInstall()
	return nil
}

func (p *vndkPrebuiltLibraryDecorator) matchesWithDevice(config android.DeviceConfig) bool {
	arches := config.Arches()
	if len(arches) == 0 || arches[0].ArchType.String() != p.arch() {
		return false
	}
	if config.BinderBitness() != p.binderBit() {
		return false
	}
	if len(p.properties.Srcs) == 0 {
		return false
	}
	return true
}

func (p *vndkPrebuiltLibraryDecorator) nativeCoverage() bool {
	return false
}

func (p *vndkPrebuiltLibraryDecorator) install(ctx ModuleContext, file android.Path) {
	arches := ctx.DeviceConfig().Arches()
	if len(arches) == 0 || arches[0].ArchType.String() != p.arch() {
		return
	}
	if ctx.DeviceConfig().BinderBitness() != p.binderBit() {
		return
	}
	if p.shared() {
		if ctx.isVndkSp() {
			p.baseInstaller.subDir = "vndk-sp-" + p.version()
		} else if ctx.isVndk() {
			p.baseInstaller.subDir = "vndk-" + p.version()
		}
		p.baseInstaller.install(ctx, file)
	}
}

func vndkPrebuiltSharedLibrary() *Module {
	module, library := NewLibrary(android.DeviceSupported)
	library.BuildOnlyShared()
	module.stl = nil
	module.sanitize = nil
	library.StripProperties.Strip.None = BoolPtr(true)

	prebuilt := &vndkPrebuiltLibraryDecorator{
		libraryDecorator: library,
	}

	prebuilt.properties.Check_elf_files = BoolPtr(false)
	prebuilt.baseLinker.Properties.No_libcrt = BoolPtr(true)
	prebuilt.baseLinker.Properties.Nocrt = BoolPtr(true)

	// Prevent default system libs (libc, libm, and libdl) from being linked
	if prebuilt.baseLinker.Properties.System_shared_libs == nil {
		prebuilt.baseLinker.Properties.System_shared_libs = []string{}
	}

	module.compiler = nil
	module.linker = prebuilt
	module.installer = prebuilt

	module.AddProperties(
		&prebuilt.properties,
	)

	android.AddLoadHook(module, func(ctx android.LoadHookContext) {
		// Only vndk snapshots of BOARD_VNDK_VERSION will be used when building.
		if prebuilt.version() != ctx.DeviceConfig().VndkVersion() {
			module.SkipInstall()
			module.Properties.HideFromMake = true
			return
		}
	})

	return module
}

// vndk_prebuilt_shared installs Vendor Native Development kit (VNDK) snapshot
// shared libraries for system build. Example:
//
//    vndk_prebuilt_shared {
//        name: "libfoo",
//        version: "27",
//        target_arch: "arm64",
//        vendor_available: true,
//        vndk: {
//            enabled: true,
//        },
//        export_include_dirs: ["include/external/libfoo/vndk_include"],
//        arch: {
//            arm64: {
//                srcs: ["arm/lib64/libfoo.so"],
//            },
//            arm: {
//                srcs: ["arm/lib/libfoo.so"],
//            },
//        },
//    }
func VndkPrebuiltSharedFactory() android.Module {
	module := vndkPrebuiltSharedLibrary()
	return module.Init()
}

func init() {
	android.RegisterModuleType("vndk_prebuilt_shared", VndkPrebuiltSharedFactory)
}
