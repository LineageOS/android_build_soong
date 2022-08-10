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
	"strings"
)

func init() {
	RegisterLibraryStubBuildComponents(android.InitRegistrationContext)
}

func RegisterLibraryStubBuildComponents(ctx android.RegistrationContext) {
	// cc_api_stub_library shares a lot of ndk_library, and this will be refactored later
	ctx.RegisterModuleType("cc_api_library", CcApiLibraryFactory)
	ctx.RegisterModuleType("cc_api_stub_library", CcApiStubLibraryFactory)
	ctx.RegisterModuleType("cc_api_contribution", CcApiContributionFactory)
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

	module.Init()

	return module
}

func (d *apiLibraryDecorator) Name(basename string) string {
	return basename + multitree.GetApiImportSuffix()
}

func (d *apiLibraryDecorator) link(ctx ModuleContext, flags Flags, deps PathDeps, objects Objects) android.Path {
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

func (d *apiLibraryDecorator) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	d.libraryDecorator.libName = strings.TrimSuffix(ctx.ModuleName(), multitree.GetApiImportSuffix())
	return d.libraryDecorator.linkerFlags(ctx, flags)
}

func CcApiStubLibraryFactory() android.Module {
	module, decorator := NewLibrary(android.DeviceSupported)
	apiStubDecorator := &apiStubDecorator{
		libraryDecorator: decorator,
	}
	apiStubDecorator.BuildOnlyShared()

	module.compiler = apiStubDecorator
	module.linker = apiStubDecorator
	module.installer = nil
	module.library = apiStubDecorator
	module.Properties.HideFromMake = true // TODO: remove

	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibBoth)
	module.AddProperties(&module.Properties,
		&apiStubDecorator.properties,
		&apiStubDecorator.MutatedProperties,
		&apiStubDecorator.apiStubLibraryProperties)
	return module
}

type apiStubLiraryProperties struct {
	Imported_includes []string `android:"path"`
}

type apiStubDecorator struct {
	*libraryDecorator
	properties               libraryProperties
	apiStubLibraryProperties apiStubLiraryProperties
}

func (compiler *apiStubDecorator) stubsVersions(ctx android.BaseMutatorContext) []string {
	firstVersion := String(compiler.properties.First_version)
	return ndkLibraryVersions(ctx, android.ApiLevelOrPanic(ctx, firstVersion))
}

func (decorator *apiStubDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) Objects {
	if decorator.stubsVersion() == "" {
		decorator.setStubsVersion("current")
	} // TODO: fix
	symbolFile := String(decorator.properties.Symbol_file)
	nativeAbiResult := parseNativeAbiDefinition(ctx, symbolFile,
		android.ApiLevelOrPanic(ctx, decorator.stubsVersion()),
		"")
	return compileStubLibrary(ctx, flags, nativeAbiResult.stubSrc)
}

func (decorator *apiStubDecorator) link(ctx ModuleContext, flags Flags, deps PathDeps, objects Objects) android.Path {
	decorator.reexportDirs(android.PathsForModuleSrc(ctx, decorator.apiStubLibraryProperties.Imported_includes)...)
	return decorator.libraryDecorator.link(ctx, flags, deps, objects)
}

func init() {
	pctx.HostBinToolVariable("gen_api_surface_build_files", "gen_api_surface_build_files")
}

type CcApiContribution struct {
	android.ModuleBase
	properties ccApiContributionProperties
}

type ccApiContributionProperties struct {
	Symbol_file        *string `android:"path"`
	First_version      *string
	Export_include_dir *string
}

func CcApiContributionFactory() android.Module {
	module := &CcApiContribution{}
	module.AddProperties(&module.properties)
	android.InitAndroidModule(module)
	return module
}

// Do some simple validations
// Majority of the build rules will be created in the ctx of the api surface this module contributes to
func (contrib *CcApiContribution) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if contrib.properties.Symbol_file == nil {
		ctx.PropertyErrorf("symbol_file", "%v does not have symbol file", ctx.ModuleName())
	}
	if contrib.properties.First_version == nil {
		ctx.PropertyErrorf("first_version", "%v does not have first_version for stub variants", ctx.ModuleName())
	}
}

// Path is out/soong/.export/ but will be different in final multi-tree layout
func outPathApiSurface(ctx android.ModuleContext, myModuleName string, pathComponent string) android.OutputPath {
	return android.PathForOutput(ctx, ".export", ctx.ModuleName(), myModuleName, pathComponent)
}

func (contrib *CcApiContribution) CopyFilesWithTag(apiSurfaceContext android.ModuleContext) map[string]android.Paths {
	// copy map.txt for now
	// hardlinks cannot be created since nsjail creates a different mountpoint for out/
	myDir := apiSurfaceContext.OtherModuleDir(contrib)
	genMapTxt := outPathApiSurface(apiSurfaceContext, contrib.Name(), String(contrib.properties.Symbol_file))
	apiSurfaceContext.Build(pctx, android.BuildParams{
		Rule:        android.Cp,
		Description: "import map.txt file",
		Input:       android.PathForSource(apiSurfaceContext, myDir, String(contrib.properties.Symbol_file)),
		Output:      genMapTxt,
	})

	outputs := make(map[string]android.Paths)
	outputs["map"] = []android.Path{genMapTxt}

	if contrib.properties.Export_include_dir != nil {
		includeDir := android.PathForSource(apiSurfaceContext, myDir, String(contrib.properties.Export_include_dir))
		outputs["export_include_dir"] = []android.Path{includeDir}
	}
	return outputs
}

var _ multitree.ApiContribution = (*CcApiContribution)(nil)

/*
func (contrib *CcApiContribution) GenerateBuildFiles(apiSurfaceContext android.ModuleContext) android.Paths {
	genAndroidBp := outPathApiSurface(apiSurfaceContext, contrib.Name(), "Android.bp")

	// generate Android.bp
	apiSurfaceContext.Build(pctx, android.BuildParams{
		Rule:        genApiSurfaceBuildFiles,
		Description: "generate API surface build files",
		Outputs:     []android.WritablePath{genAndroidBp},
		Args: map[string]string{
			"name":          contrib.Name() + "." + apiSurfaceContext.ModuleName(), //e.g. liblog.ndk
			"symbol_file":   String(contrib.properties.Symbol_file),
			"first_version": String(contrib.properties.First_version),
		},
	})
	return []android.Path{genAndroidBp}
}
*/
