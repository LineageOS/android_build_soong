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
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/bazel"
)

func init() {
	android.RegisterModuleType("ndk_prebuilt_static_stl", NdkPrebuiltStaticStlFactory)
	android.RegisterModuleType("ndk_prebuilt_shared_stl", NdkPrebuiltSharedStlFactory)
}

// NDK prebuilt libraries.
//
// These differ from regular prebuilts in that they aren't stripped and usually aren't installed
// either (with the exception of the shared STLs, which are installed to the app's directory rather
// than to the system image).

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
	module.Properties.Sdk_version = StringPtr("minimum")
	module.Properties.AlwaysSdk = true
	module.stl.Properties.Stl = StringPtr("none")
	module.bazelable = true
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
	module.Properties.Sdk_version = StringPtr("minimum")
	module.Properties.HideFromMake = true
	module.Properties.AlwaysSdk = true
	module.Properties.Sdk_version = StringPtr("current")
	module.stl.Properties.Stl = StringPtr("none")
	module.bazelable = true
	return module.Init()
}

const (
	libDir = "current/sources/cxx-stl/llvm-libc++/libs"
)

func getNdkStlLibDir(ctx android.ModuleContext) android.SourcePath {
	return android.PathForSource(ctx, ctx.ModuleDir(), libDir).Join(ctx, ctx.Arch().Abi[0])
}

func (ndk *ndkPrebuiltStlLinker) link(ctx ModuleContext, flags Flags,
	deps PathDeps, objs Objects) android.Path {
	// A null build step, but it sets up the output path.
	if !strings.HasPrefix(ctx.ModuleName(), "ndk_lib") {
		ctx.ModuleErrorf("NDK prebuilt libraries must have an ndk_lib prefixed name")
	}

	ndk.libraryDecorator.flagExporter.exportIncludesAsSystem(ctx)

	libName := strings.TrimPrefix(ctx.ModuleName(), "ndk_")
	libExt := flags.Toolchain.ShlibSuffix()
	if ndk.static() {
		libExt = staticLibraryExtension
	}

	libDir := getNdkStlLibDir(ctx)
	lib := libDir.Join(ctx, libName+libExt)

	ndk.libraryDecorator.flagExporter.setProvider(ctx)

	if ndk.static() {
		depSet := android.NewDepSetBuilder[android.Path](android.TOPOLOGICAL).Direct(lib).Build()
		ctx.SetProvider(StaticLibraryInfoProvider, StaticLibraryInfo{
			StaticLibrary: lib,

			TransitiveStaticLibrariesForOrdering: depSet,
		})
	} else {
		ctx.SetProvider(SharedLibraryInfoProvider, SharedLibraryInfo{
			SharedLibrary: lib,
			Target:        ctx.Target(),
		})
	}

	return lib
}

var (
	archToAbiDirMap = map[string]string{
		"android_arm":     "armeabi-v7a",
		"android_arm64":   "arm64-v8a",
		"android_riscv64": "riscv64",
		"android_x86":     "x86",
		"android_x86_64":  "x86_64",
	}
)

// stlSrcBp2build returns a bazel label for the checked-in .so/.a file
// It contains a select statement for each ABI
func stlSrcBp2build(ctx android.Bp2buildMutatorContext, c *Module) bazel.LabelAttribute {
	libName := strings.TrimPrefix(c.Name(), "ndk_")
	libExt := ".so" // TODO - b/201079053: Support windows
	if ctx.ModuleType() == "ndk_prebuilt_static_stl" {
		libExt = ".a"
	}
	src := bazel.LabelAttribute{}
	for arch, abiDir := range archToAbiDirMap {
		srcPath := filepath.Join(libDir, abiDir, libName+libExt)
		src.SetSelectValue(
			bazel.OsArchConfigurationAxis,
			arch,
			android.BazelLabelForModuleSrcSingle(ctx, srcPath),
		)
	}
	return src
}

// stlIncludesBp2build returns the includes exported by the STL
func stlIncludesBp2build(c *Module) bazel.StringListAttribute {
	linker, _ := c.linker.(*ndkPrebuiltStlLinker)
	includeDirs := append(
		[]string{},
		linker.libraryDecorator.flagExporter.Properties.Export_include_dirs...,
	)
	includeDirs = append(
		includeDirs,
		linker.libraryDecorator.flagExporter.Properties.Export_system_include_dirs...,
	)
	return bazel.MakeStringListAttribute(android.FirstUniqueStrings(includeDirs))
}

func ndkPrebuiltStlBp2build(ctx android.Bp2buildMutatorContext, c *Module) {
	if ctx.ModuleType() == "ndk_prebuilt_static_stl" {
		ndkPrebuiltStaticStlBp2build(ctx, c)
	} else {
		ndkPrebuiltSharedStlBp2build(ctx, c)
	}
}

func ndkPrebuiltStaticStlBp2build(ctx android.Bp2buildMutatorContext, c *Module) {
	props := bazel.BazelTargetModuleProperties{
		Rule_class:        "cc_prebuilt_library_static",
		Bzl_load_location: "//build/bazel/rules/cc:cc_prebuilt_library_static.bzl",
	}
	attrs := &bazelPrebuiltLibraryStaticAttributes{
		Static_library:         stlSrcBp2build(ctx, c),
		Export_system_includes: stlIncludesBp2build(c), // The exports are always as system
	}
	// TODO: min_sdk_version
	ctx.CreateBazelTargetModule(props, android.CommonAttributes{Name: c.Name()}, attrs)
}

func ndkPrebuiltSharedStlBp2build(ctx android.Bp2buildMutatorContext, c *Module) {
	props := bazel.BazelTargetModuleProperties{
		Rule_class:        "cc_prebuilt_library_shared",
		Bzl_load_location: "//build/bazel/rules/cc:cc_prebuilt_library_shared.bzl",
	}
	attrs := &bazelPrebuiltLibrarySharedAttributes{
		Shared_library:         stlSrcBp2build(ctx, c),
		Export_system_includes: stlIncludesBp2build(c), // The exports are always as system
	}
	// TODO: min_sdk_version
	ctx.CreateBazelTargetModule(props, android.CommonAttributes{Name: c.Name()}, attrs)
}
