// Copyright 2021 Google Inc. All rights reserved.
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
	"android/soong/multitree"
)

func init() {
	RegisterLibraryStubBuildComponents(android.InitRegistrationContext)
}

func RegisterLibraryStubBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("cc_api_library", CcApiLibraryFactory)
	ctx.RegisterModuleType("cc_api_headers", CcApiHeadersFactory)
}

// 'cc_api_library' is a module type which is from the exported API surface
// with C shared library type. The module will replace original module, and
// offer a link to the module that generates shared library object from the
// map file.
type apiLibraryProperties struct {
	Src *string `android:"arch_variant"`
}

type apiLibraryDecorator struct {
	*libraryDecorator
	properties apiLibraryProperties
}

func CcApiLibraryFactory() android.Module {
	module, decorator := NewLibrary(android.DeviceSupported)
	apiLibraryDecorator := &apiLibraryDecorator{
		libraryDecorator: decorator,
	}
	apiLibraryDecorator.BuildOnlyShared()

	module.stl = nil
	module.sanitize = nil
	decorator.disableStripping()

	module.compiler = nil
	module.linker = apiLibraryDecorator
	module.installer = nil
	module.AddProperties(&module.Properties, &apiLibraryDecorator.properties)

	// Mark module as stub, so APEX would not include this stub in the package.
	module.library.setBuildStubs(true)

	// Prevent default system libs (libc, libm, and libdl) from being linked
	if apiLibraryDecorator.baseLinker.Properties.System_shared_libs == nil {
		apiLibraryDecorator.baseLinker.Properties.System_shared_libs = []string{}
	}

	apiLibraryDecorator.baseLinker.Properties.No_libcrt = BoolPtr(true)
	apiLibraryDecorator.baseLinker.Properties.Nocrt = BoolPtr(true)

	module.Init()

	return module
}

func (d *apiLibraryDecorator) Name(basename string) string {
	return basename + multitree.GetApiImportSuffix()
}

func (d *apiLibraryDecorator) link(ctx ModuleContext, flags Flags, deps PathDeps, objects Objects) android.Path {
	// Export headers as system include dirs if specified. Mostly for libc
	if Bool(d.libraryDecorator.Properties.Llndk.Export_headers_as_system) {
		d.libraryDecorator.flagExporter.Properties.Export_system_include_dirs = append(
			d.libraryDecorator.flagExporter.Properties.Export_system_include_dirs,
			d.libraryDecorator.flagExporter.Properties.Export_include_dirs...)
		d.libraryDecorator.flagExporter.Properties.Export_include_dirs = nil
	}

	// Flags reexported from dependencies. (e.g. vndk_prebuilt_shared)
	d.libraryDecorator.flagExporter.exportIncludes(ctx)
	d.libraryDecorator.reexportDirs(deps.ReexportedDirs...)
	d.libraryDecorator.reexportSystemDirs(deps.ReexportedSystemDirs...)
	d.libraryDecorator.reexportFlags(deps.ReexportedFlags...)
	d.libraryDecorator.reexportDeps(deps.ReexportedDeps...)
	d.libraryDecorator.addExportedGeneratedHeaders(deps.ReexportedGeneratedHeaders...)
	d.libraryDecorator.flagExporter.setProvider(ctx)

	in := android.PathForModuleSrc(ctx, *d.properties.Src)

	d.unstrippedOutputFile = in
	libName := d.libraryDecorator.getLibName(ctx) + flags.Toolchain.ShlibSuffix()

	tocFile := android.PathForModuleOut(ctx, libName+".toc")
	d.tocFile = android.OptionalPathForPath(tocFile)
	TransformSharedObjectToToc(ctx, in, tocFile)

	ctx.SetProvider(SharedLibraryInfoProvider, SharedLibraryInfo{
		SharedLibrary: in,
		Target:        ctx.Target(),

		TableOfContents: d.tocFile,
	})

	return in
}

func (d *apiLibraryDecorator) availableFor(what string) bool {
	// Stub from API surface should be available for any APEX.
	return true
}

// 'cc_api_headers' is similar with 'cc_api_library', but which replaces
// header libraries. The module will replace any dependencies to existing
// original header libraries.
type apiHeadersDecorator struct {
	*libraryDecorator
}

func CcApiHeadersFactory() android.Module {
	module, decorator := NewLibrary(android.DeviceSupported)
	apiHeadersDecorator := &apiHeadersDecorator{
		libraryDecorator: decorator,
	}
	apiHeadersDecorator.HeaderOnly()

	module.stl = nil
	module.sanitize = nil
	decorator.disableStripping()

	module.compiler = nil
	module.linker = apiHeadersDecorator
	module.installer = nil

	// Mark module as stub, so APEX would not include this stub in the package.
	module.library.setBuildStubs(true)

	// Prevent default system libs (libc, libm, and libdl) from being linked
	if apiHeadersDecorator.baseLinker.Properties.System_shared_libs == nil {
		apiHeadersDecorator.baseLinker.Properties.System_shared_libs = []string{}
	}

	apiHeadersDecorator.baseLinker.Properties.No_libcrt = BoolPtr(true)
	apiHeadersDecorator.baseLinker.Properties.Nocrt = BoolPtr(true)

	module.Init()

	return module
}

func (d *apiHeadersDecorator) Name(basename string) string {
	return basename + multitree.GetApiImportSuffix()
}

func (d *apiHeadersDecorator) availableFor(what string) bool {
	// Stub from API surface should be available for any APEX.
	return true
}
