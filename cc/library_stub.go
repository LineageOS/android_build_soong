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
	// cc_api_stub_library shares a lot of ndk_library, and this will be refactored later
	ctx.RegisterModuleType("cc_api_stub_library", CcApiStubLibraryFactory)
	ctx.RegisterModuleType("cc_api_contribution", CcApiContributionFactory)
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
