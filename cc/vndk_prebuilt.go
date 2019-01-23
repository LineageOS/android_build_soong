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
//     version: "27.1.0",
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
	if len(p.properties.Srcs) > 0 && p.shared() {
		// current VNDK prebuilts are only shared libs.
		return p.singleSourcePath(ctx)
	}
	return nil
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
	module.Properties.UseVndk = true

	prebuilt := &vndkPrebuiltLibraryDecorator{
		libraryDecorator: library,
	}

	prebuilt.properties.Check_elf_files = BoolPtr(false)

	module.compiler = nil
	module.linker = prebuilt
	module.installer = prebuilt

	module.AddProperties(
		&prebuilt.properties,
	)

	return module
}

func vndkPrebuiltSharedFactory() android.Module {
	module := vndkPrebuiltSharedLibrary()
	return module.Init()
}

func init() {
	android.RegisterModuleType("vndk_prebuilt_shared", vndkPrebuiltSharedFactory)
}
