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
	"android/soong/android"
)

//
// Device libraries shipped with gcc
//

func init() {
	android.RegisterModuleType("toolchain_library", ToolchainLibraryFactory)
}

type toolchainLibraryProperties struct {
	// the prebuilt toolchain library, as a path from the top of the source tree
	Src *string `android:"arch_variant"`

	// Repack the archive with only the selected objects.
	Repack_objects_to_keep []string `android:"arch_variant"`
}

type toolchainLibraryDecorator struct {
	*libraryDecorator

	stripper

	Properties toolchainLibraryProperties
}

func (*toolchainLibraryDecorator) linkerDeps(ctx DepsContext, deps Deps) Deps {
	// toolchain libraries can't have any dependencies
	return deps
}

func (library *toolchainLibraryDecorator) linkerProps() []interface{} {
	var props []interface{}
	props = append(props, library.libraryDecorator.linkerProps()...)
	return append(props, &library.Properties, &library.stripper.StripProperties)
}

// toolchain_library is used internally by the build tool to link the specified
// static library in src property to the device libraries that are shipped with
// gcc.
func ToolchainLibraryFactory() android.Module {
	module, library := NewLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyStatic()
	toolchainLibrary := &toolchainLibraryDecorator{
		libraryDecorator: library,
	}
	module.compiler = toolchainLibrary
	module.linker = toolchainLibrary
	module.stl = nil
	module.sanitize = nil
	module.installer = nil
	module.Properties.Sdk_version = StringPtr("current")
	return module.Init()
}

func (library *toolchainLibraryDecorator) compile(ctx ModuleContext, flags Flags,
	deps PathDeps) Objects {
	return Objects{}
}

func (library *toolchainLibraryDecorator) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {

	if library.Properties.Src == nil {
		ctx.PropertyErrorf("src", "No library source specified")
		return android.PathForSource(ctx, "")
	}

	srcPath := android.PathForSource(ctx, *library.Properties.Src)

	if library.stripper.StripProperties.Strip.Keep_symbols_list != nil {
		fileName := ctx.ModuleName() + staticLibraryExtension
		outputFile := android.PathForModuleOut(ctx, fileName)
		buildFlags := flagsToBuilderFlags(flags)
		library.stripper.stripStaticLib(ctx, srcPath, outputFile, buildFlags)
		return outputFile
	}

	if library.Properties.Repack_objects_to_keep != nil {
		fileName := ctx.ModuleName() + staticLibraryExtension
		outputFile := android.PathForModuleOut(ctx, fileName)
		TransformArchiveRepack(ctx, srcPath, outputFile, library.Properties.Repack_objects_to_keep)

		return outputFile
	}

	return srcPath
}

func (library *toolchainLibraryDecorator) nativeCoverage() bool {
	return false
}
