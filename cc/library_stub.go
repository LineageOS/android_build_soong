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
	"regexp"
	"strings"

	"android/soong/android"
	"android/soong/multitree"
)

var (
	ndkVariantRegex  = regexp.MustCompile("ndk\\.([a-zA-Z0-9]+)")
	stubVariantRegex = regexp.MustCompile("apex\\.([a-zA-Z0-9]+)")
)

func init() {
	RegisterLibraryStubBuildComponents(android.InitRegistrationContext)
}

func RegisterLibraryStubBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("cc_api_library", CcApiLibraryFactory)
	ctx.RegisterModuleType("cc_api_headers", CcApiHeadersFactory)
	ctx.RegisterModuleType("cc_api_variant", CcApiVariantFactory)
}

func updateImportedLibraryDependency(ctx android.BottomUpMutatorContext) {
	m, ok := ctx.Module().(*Module)
	if !ok {
		return
	}

	apiLibrary, ok := m.linker.(*apiLibraryDecorator)
	if !ok {
		return
	}

	if m.InVendorOrProduct() && apiLibrary.hasLLNDKStubs() {
		// Add LLNDK variant dependency
		if inList("llndk", apiLibrary.properties.Variants) {
			variantName := BuildApiVariantName(m.BaseModuleName(), "llndk", "")
			ctx.AddDependency(m, nil, variantName)
		}
	} else if m.IsSdkVariant() {
		// Add NDK variant dependencies
		targetVariant := "ndk." + m.StubsVersion()
		if inList(targetVariant, apiLibrary.properties.Variants) {
			variantName := BuildApiVariantName(m.BaseModuleName(), targetVariant, "")
			ctx.AddDependency(m, nil, variantName)
		}
	} else if m.IsStubs() {
		targetVariant := "apex." + m.StubsVersion()
		if inList(targetVariant, apiLibrary.properties.Variants) {
			variantName := BuildApiVariantName(m.BaseModuleName(), targetVariant, "")
			ctx.AddDependency(m, nil, variantName)
		}
	}
}

// 'cc_api_library' is a module type which is from the exported API surface
// with C shared library type. The module will replace original module, and
// offer a link to the module that generates shared library object from the
// map file.
type apiLibraryProperties struct {
	Src      *string `android:"arch_variant"`
	Variants []string
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
	module.library = apiLibraryDecorator
	module.AddProperties(&module.Properties, &apiLibraryDecorator.properties)

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

// Export include dirs without checking for existence.
// The directories are not guaranteed to exist during Soong analysis.
func (d *apiLibraryDecorator) exportIncludes(ctx ModuleContext) {
	exporterProps := d.flagExporter.Properties
	for _, dir := range exporterProps.Export_include_dirs {
		d.dirs = append(d.dirs, android.MaybeExistentPathForSource(ctx, ctx.ModuleDir(), dir))
	}
	// system headers
	for _, dir := range exporterProps.Export_system_include_dirs {
		d.systemDirs = append(d.systemDirs, android.MaybeExistentPathForSource(ctx, ctx.ModuleDir(), dir))
	}
}

func (d *apiLibraryDecorator) linkerInit(ctx BaseModuleContext) {
	d.baseLinker.linkerInit(ctx)

	if d.hasNDKStubs() {
		// Set SDK version of module as current
		ctx.Module().(*Module).Properties.Sdk_version = StringPtr("current")

		// Add NDK stub as NDK known libs
		name := ctx.ModuleName()

		ndkKnownLibsLock.Lock()
		ndkKnownLibs := getNDKKnownLibs(ctx.Config())
		if !inList(name, *ndkKnownLibs) {
			*ndkKnownLibs = append(*ndkKnownLibs, name)
		}
		ndkKnownLibsLock.Unlock()
	}
}

func (d *apiLibraryDecorator) link(ctx ModuleContext, flags Flags, deps PathDeps, objects Objects) android.Path {
	m, _ := ctx.Module().(*Module)

	var in android.Path

	// src might not exist during the beginning of soong analysis in Multi-tree
	if src := String(d.properties.Src); src != "" {
		in = android.MaybeExistentPathForSource(ctx, ctx.ModuleDir(), src)
	}

	libName := m.BaseModuleName() + multitree.GetApiImportSuffix()

	load_cc_variant := func(apiVariantModule string) {
		var mod android.Module

		ctx.VisitDirectDeps(func(depMod android.Module) {
			if depMod.Name() == apiVariantModule {
				mod = depMod
				libName = apiVariantModule
			}
		})

		if mod != nil {
			variantMod, ok := mod.(*CcApiVariant)
			if ok {
				in = variantMod.Src()

				// Copy LLDNK properties to cc_api_library module
				d.libraryDecorator.flagExporter.Properties.Export_include_dirs = append(
					d.libraryDecorator.flagExporter.Properties.Export_include_dirs,
					variantMod.exportProperties.Export_include_dirs...)

				// Export headers as system include dirs if specified. Mostly for libc
				if Bool(variantMod.exportProperties.Export_headers_as_system) {
					d.libraryDecorator.flagExporter.Properties.Export_system_include_dirs = append(
						d.libraryDecorator.flagExporter.Properties.Export_system_include_dirs,
						d.libraryDecorator.flagExporter.Properties.Export_include_dirs...)
					d.libraryDecorator.flagExporter.Properties.Export_include_dirs = nil
				}
			}
		}
	}

	if m.InVendorOrProduct() && d.hasLLNDKStubs() {
		// LLNDK variant
		load_cc_variant(BuildApiVariantName(m.BaseModuleName(), "llndk", ""))
	} else if m.IsSdkVariant() {
		// NDK Variant
		load_cc_variant(BuildApiVariantName(m.BaseModuleName(), "ndk", m.StubsVersion()))
	} else if m.IsStubs() {
		// APEX Variant
		load_cc_variant(BuildApiVariantName(m.BaseModuleName(), "apex", m.StubsVersion()))
	}

	// Flags reexported from dependencies. (e.g. vndk_prebuilt_shared)
	d.exportIncludes(ctx)
	d.libraryDecorator.reexportDirs(deps.ReexportedDirs...)
	d.libraryDecorator.reexportSystemDirs(deps.ReexportedSystemDirs...)
	d.libraryDecorator.reexportFlags(deps.ReexportedFlags...)
	d.libraryDecorator.reexportDeps(deps.ReexportedDeps...)
	d.libraryDecorator.addExportedGeneratedHeaders(deps.ReexportedGeneratedHeaders...)

	if in == nil {
		ctx.PropertyErrorf("src", "Unable to locate source property")
		return nil
	}

	// Make the _compilation_ of rdeps have an order-only dep on cc_api_library.src (an .so file)
	// The .so file itself has an order-only dependency on the headers contributed by this library.
	// Creating this dependency ensures that the headers are assembled before compilation of rdeps begins.
	d.libraryDecorator.reexportDeps(in)
	d.libraryDecorator.flagExporter.setProvider(ctx)

	d.unstrippedOutputFile = in
	libName += flags.Toolchain.ShlibSuffix()

	tocFile := android.PathForModuleOut(ctx, libName+".toc")
	d.tocFile = android.OptionalPathForPath(tocFile)
	TransformSharedObjectToToc(ctx, in, tocFile)

	outputFile := android.PathForModuleOut(ctx, libName)

	// TODO(b/270485584) This copies with a new name, just to avoid conflict with prebuilts.
	// We can just use original input if there is any way to avoid name conflict without copy.
	ctx.Build(pctx, android.BuildParams{
		Rule:        android.Cp,
		Description: "API surface imported library",
		Input:       in,
		Output:      outputFile,
		Args: map[string]string{
			"cpFlags": "-L",
		},
	})

	android.SetProvider(ctx, SharedLibraryInfoProvider, SharedLibraryInfo{
		SharedLibrary: outputFile,
		Target:        ctx.Target(),

		TableOfContents: d.tocFile,
	})

	d.shareStubs(ctx)

	return outputFile
}

// Share additional information about stub libraries with provider
func (d *apiLibraryDecorator) shareStubs(ctx ModuleContext) {
	stubs := ctx.GetDirectDepsWithTag(stubImplDepTag)
	if len(stubs) > 0 {
		var stubsInfo []SharedStubLibrary
		for _, stub := range stubs {
			stubInfo, _ := android.OtherModuleProvider(ctx, stub, SharedLibraryInfoProvider)
			flagInfo, _ := android.OtherModuleProvider(ctx, stub, FlagExporterInfoProvider)
			stubsInfo = append(stubsInfo, SharedStubLibrary{
				Version:           moduleLibraryInterface(stub).stubsVersion(),
				SharedLibraryInfo: stubInfo,
				FlagExporterInfo:  flagInfo,
			})
		}
		android.SetProvider(ctx, SharedLibraryStubsProvider, SharedLibraryStubsInfo{
			SharedStubLibraries: stubsInfo,

			IsLLNDK: ctx.IsLlndk(),
		})
	}
}

func (d *apiLibraryDecorator) availableFor(what string) bool {
	// Stub from API surface should be available for any APEX.
	return true
}

func (d *apiLibraryDecorator) hasApexStubs() bool {
	for _, variant := range d.properties.Variants {
		if strings.HasPrefix(variant, "apex") {
			return true
		}
	}
	return false
}

func (d *apiLibraryDecorator) hasStubsVariants() bool {
	return d.hasApexStubs()
}

func (d *apiLibraryDecorator) stubsVersions(ctx android.BaseMutatorContext) []string {
	m, ok := ctx.Module().(*Module)

	if !ok {
		return nil
	}

	// TODO(b/244244438) Create more version information for NDK and APEX variations
	// NDK variants
	if m.IsSdkVariant() {
		// TODO(b/249193999) Do not check if module has NDK stubs once all NDK cc_api_library contains ndk variant of cc_api_variant.
		if d.hasNDKStubs() {
			return d.getNdkVersions()
		}
	}

	if d.hasLLNDKStubs() && m.InVendorOrProduct() {
		// LLNDK libraries only need a single stubs variant.
		return []string{android.FutureApiLevel.String()}
	}

	stubsVersions := d.getStubVersions()

	if len(stubsVersions) != 0 {
		return stubsVersions
	}

	if m.MinSdkVersion() == "" {
		return nil
	}

	firstVersion, err := nativeApiLevelFromUser(ctx,
		m.MinSdkVersion())

	if err != nil {
		return nil
	}

	return ndkLibraryVersions(ctx, firstVersion)
}

func (d *apiLibraryDecorator) hasLLNDKStubs() bool {
	return inList("llndk", d.properties.Variants)
}

func (d *apiLibraryDecorator) hasNDKStubs() bool {
	for _, variant := range d.properties.Variants {
		if ndkVariantRegex.MatchString(variant) {
			return true
		}
	}
	return false
}

func (d *apiLibraryDecorator) getNdkVersions() []string {
	ndkVersions := []string{}

	for _, variant := range d.properties.Variants {
		if match := ndkVariantRegex.FindStringSubmatch(variant); len(match) == 2 {
			ndkVersions = append(ndkVersions, match[1])
		}
	}

	return ndkVersions
}

func (d *apiLibraryDecorator) getStubVersions() []string {
	stubVersions := []string{}

	for _, variant := range d.properties.Variants {
		if match := stubVariantRegex.FindStringSubmatch(variant); len(match) == 2 {
			stubVersions = append(stubVersions, match[1])
		}
	}

	return stubVersions
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

type ccApiexportProperties struct {
	Src     *string `android:"arch_variant"`
	Variant *string
	Version *string
}

type variantExporterProperties struct {
	// Header directory to export
	Export_include_dirs []string `android:"arch_variant"`

	// Export all headers as system include
	Export_headers_as_system *bool
}

type CcApiVariant struct {
	android.ModuleBase

	properties       ccApiexportProperties
	exportProperties variantExporterProperties

	src android.Path
}

var _ android.Module = (*CcApiVariant)(nil)
var _ android.ImageInterface = (*CcApiVariant)(nil)

func CcApiVariantFactory() android.Module {
	module := &CcApiVariant{}

	module.AddProperties(&module.properties)
	module.AddProperties(&module.exportProperties)

	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibBoth)
	return module
}

func (v *CcApiVariant) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// No need to build

	if String(v.properties.Src) == "" {
		ctx.PropertyErrorf("src", "src is a required property")
	}

	// Skip the existence check of the stub prebuilt file.
	// The file is not guaranteed to exist during Soong analysis.
	// Build orchestrator will be responsible for creating a connected ninja graph.
	v.src = android.MaybeExistentPathForSource(ctx, ctx.ModuleDir(), String(v.properties.Src))
}

func (v *CcApiVariant) Name() string {
	version := String(v.properties.Version)
	return BuildApiVariantName(v.BaseModuleName(), *v.properties.Variant, version)
}

func (v *CcApiVariant) Src() android.Path {
	return v.src
}

func BuildApiVariantName(baseName string, variant string, version string) string {
	names := []string{baseName, variant}
	if version != "" {
		names = append(names, version)
	}

	return strings.Join(names[:], ".") + multitree.GetApiImportSuffix()
}

// Implement ImageInterface to generate image variants
func (v *CcApiVariant) ImageMutatorBegin(ctx android.BaseModuleContext) {}
func (v *CcApiVariant) CoreVariantNeeded(ctx android.BaseModuleContext) bool {
	return inList(String(v.properties.Variant), []string{"ndk", "apex"})
}
func (v *CcApiVariant) RamdiskVariantNeeded(ctx android.BaseModuleContext) bool       { return false }
func (v *CcApiVariant) VendorRamdiskVariantNeeded(ctx android.BaseModuleContext) bool { return false }
func (v *CcApiVariant) DebugRamdiskVariantNeeded(ctx android.BaseModuleContext) bool  { return false }
func (v *CcApiVariant) RecoveryVariantNeeded(ctx android.BaseModuleContext) bool      { return false }
func (v *CcApiVariant) ExtraImageVariations(ctx android.BaseModuleContext) []string {
	var variations []string
	platformVndkVersion := ctx.DeviceConfig().PlatformVndkVersion()

	if String(v.properties.Variant) == "llndk" {
		variations = append(variations, VendorVariationPrefix+platformVndkVersion)
		variations = append(variations, ProductVariationPrefix+platformVndkVersion)
	}

	return variations
}
func (v *CcApiVariant) SetImageVariation(ctx android.BaseModuleContext, variation string, module android.Module) {
}
