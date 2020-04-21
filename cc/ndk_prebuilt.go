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

package cc

import (
	"fmt"
	"strings"

	"android/soong/android"
	"android/soong/cc/config"
)

func init() {
	android.RegisterModuleType("ndk_prebuilt_object", NdkPrebuiltObjectFactory)
	android.RegisterModuleType("ndk_prebuilt_static_stl", NdkPrebuiltStaticStlFactory)
	android.RegisterModuleType("ndk_prebuilt_shared_stl", NdkPrebuiltSharedStlFactory)
}

// NDK prebuilt libraries.
//
// These differ from regular prebuilts in that they aren't stripped and usually aren't installed
// either (with the exception of the shared STLs, which are installed to the app's directory rather
// than to the system image).

func getNdkLibDir(ctx android.ModuleContext, toolchain config.Toolchain, version string) android.SourcePath {
	suffix := ""
	// Most 64-bit NDK prebuilts store libraries in "lib64", except for arm64 which is not a
	// multilib toolchain and stores the libraries in "lib".
	if toolchain.Is64Bit() && ctx.Arch().ArchType != android.Arm64 {
		suffix = "64"
	}
	return android.PathForSource(ctx, fmt.Sprintf("prebuilts/ndk/current/platforms/android-%s/arch-%s/usr/lib%s",
		version, toolchain.Name(), suffix))
}

func ndkPrebuiltModuleToPath(ctx android.ModuleContext, toolchain config.Toolchain,
	ext string, version string) android.Path {

	// NDK prebuilts are named like: ndk_NAME.EXT.SDK_VERSION.
	// We want to translate to just NAME.EXT
	name := strings.Split(strings.TrimPrefix(ctx.ModuleName(), "ndk_"), ".")[0]
	dir := getNdkLibDir(ctx, toolchain, version)
	return dir.Join(ctx, name+ext)
}

type ndkPrebuiltObjectLinker struct {
	objectLinker
}

func (*ndkPrebuiltObjectLinker) linkerDeps(ctx DepsContext, deps Deps) Deps {
	// NDK objects can't have any dependencies
	return deps
}

// ndk_prebuilt_object exports a precompiled ndk object file for linking
// operations. Soong's module name format is ndk_<NAME>.o.<sdk_version> where
// the object is located under
// ./prebuilts/ndk/current/platforms/android-<sdk_version>/arch-$(HOST_ARCH)/usr/lib/<NAME>.o.
func NdkPrebuiltObjectFactory() android.Module {
	module := newBaseModule(android.DeviceSupported, android.MultilibBoth)
	module.ModuleBase.EnableNativeBridgeSupportByDefault()
	module.linker = &ndkPrebuiltObjectLinker{
		objectLinker: objectLinker{
			baseLinker: NewBaseLinker(nil),
		},
	}
	module.Properties.HideFromMake = true
	return module.Init()
}

func (c *ndkPrebuiltObjectLinker) link(ctx ModuleContext, flags Flags,
	deps PathDeps, objs Objects) android.Path {
	// A null build step, but it sets up the output path.
	if !strings.HasPrefix(ctx.ModuleName(), "ndk_crt") {
		ctx.ModuleErrorf("NDK prebuilt objects must have an ndk_crt prefixed name")
	}

	return ndkPrebuiltModuleToPath(ctx, flags.Toolchain, objectExtension, ctx.sdkVersion())
}

func (*ndkPrebuiltObjectLinker) availableFor(what string) bool {
	// ndk prebuilt objects are available to everywhere
	return true
}

type ndkPrebuiltStlLinker struct {
	*libraryDecorator
}

func (ndk *ndkPrebuiltStlLinker) linkerProps() []interface{} {
	return append(ndk.libraryDecorator.linkerProps(), &ndk.Properties, &ndk.flagExporter.Properties)
}

func (*ndkPrebuiltStlLinker) linkerDeps(ctx DepsContext, deps Deps) Deps {
	// NDK libraries can't have any dependencies
	return deps
}

func (*ndkPrebuiltStlLinker) availableFor(what string) bool {
	// ndk prebuilt objects are available to everywhere
	return true
}

// ndk_prebuilt_shared_stl exports a precompiled ndk shared standard template
// library (stl) library for linking operation. The soong's module name format
// is ndk_<NAME>.so where the library is located under
// ./prebuilts/ndk/current/sources/cxx-stl/llvm-libc++/libs/$(HOST_ARCH)/<NAME>.so.
func NdkPrebuiltSharedStlFactory() android.Module {
	module, library := NewLibrary(android.DeviceSupported)
	library.BuildOnlyShared()
	module.compiler = nil
	module.linker = &ndkPrebuiltStlLinker{
		libraryDecorator: library,
	}
	module.installer = nil
	minVersionString := "minimum"
	noStlString := "none"
	module.Properties.Sdk_version = &minVersionString
	module.stl.Properties.Stl = &noStlString
	return module.Init()
}

// ndk_prebuilt_static_stl exports a precompiled ndk static standard template
// library (stl) library for linking operation. The soong's module name format
// is ndk_<NAME>.a where the library is located under
// ./prebuilts/ndk/current/sources/cxx-stl/llvm-libc++/libs/$(HOST_ARCH)/<NAME>.a.
func NdkPrebuiltStaticStlFactory() android.Module {
	module, library := NewLibrary(android.DeviceSupported)
	library.BuildOnlyStatic()
	module.compiler = nil
	module.linker = &ndkPrebuiltStlLinker{
		libraryDecorator: library,
	}
	module.installer = nil
	module.Properties.HideFromMake = true
	module.ModuleBase.EnableNativeBridgeSupportByDefault()
	return module.Init()
}

func getNdkStlLibDir(ctx android.ModuleContext) android.SourcePath {
	libDir := "prebuilts/ndk/current/sources/cxx-stl/llvm-libc++/libs"
	return android.PathForSource(ctx, libDir).Join(ctx, ctx.Arch().Abi[0])
}

func (ndk *ndkPrebuiltStlLinker) link(ctx ModuleContext, flags Flags,
	deps PathDeps, objs Objects) android.Path {
	// A null build step, but it sets up the output path.
	if !strings.HasPrefix(ctx.ModuleName(), "ndk_lib") {
		ctx.ModuleErrorf("NDK prebuilt libraries must have an ndk_lib prefixed name")
	}

	ndk.exportIncludesAsSystem(ctx)

	libName := strings.TrimPrefix(ctx.ModuleName(), "ndk_")
	libExt := flags.Toolchain.ShlibSuffix()
	if ndk.static() {
		libExt = staticLibraryExtension
	}

	libDir := getNdkStlLibDir(ctx)
	return libDir.Join(ctx, libName+libExt)
}
