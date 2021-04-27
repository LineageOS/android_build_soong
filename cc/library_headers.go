// Copyright 2020 Google Inc. All rights reserved.
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
	"android/soong/bazel"
)

func init() {
	RegisterLibraryHeadersBuildComponents(android.InitRegistrationContext)

	// Register sdk member types.
	android.RegisterSdkMemberType(headersLibrarySdkMemberType)

	android.RegisterBp2BuildMutator("cc_library_headers", CcLibraryHeadersBp2Build)
}

var headersLibrarySdkMemberType = &librarySdkMemberType{
	SdkMemberTypeBase: android.SdkMemberTypeBase{
		PropertyName:    "native_header_libs",
		SupportsSdk:     true,
		HostOsDependent: true,
	},
	prebuiltModuleType: "cc_prebuilt_library_headers",
	noOutputFiles:      true,
}

func RegisterLibraryHeadersBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("cc_library_headers", LibraryHeaderFactory)
	ctx.RegisterModuleType("cc_prebuilt_library_headers", prebuiltLibraryHeaderFactory)
}

type libraryHeaderBazelHander struct {
	bazelHandler

	module  *Module
	library *libraryDecorator
}

func (h *libraryHeaderBazelHander) generateBazelBuildActions(ctx android.ModuleContext, label string) bool {
	bazelCtx := ctx.Config().BazelContext
	ccInfo, ok, err := bazelCtx.GetCcInfo(label, ctx.Arch().ArchType)
	if err != nil {
		ctx.ModuleErrorf("Error getting Bazel CcInfo: %s", err)
		return false
	}
	if !ok {
		return false
	}

	outputPaths := ccInfo.OutputFiles
	if len(outputPaths) != 1 {
		ctx.ModuleErrorf("expected exactly one output file for %q, but got %q", label, outputPaths)
		return false
	}

	outputPath := android.PathForBazelOut(ctx, outputPaths[0])
	h.module.outputFile = android.OptionalPathForPath(outputPath)

	// HeaderLibraryInfo is an empty struct to indicate to dependencies that this is a header library
	ctx.SetProvider(HeaderLibraryInfoProvider, HeaderLibraryInfo{})

	flagExporterInfo := flagExporterInfoFromCcInfo(ctx, ccInfo)
	// Store flag info to be passed along to androimk
	// TODO(b/184387147): Androidmk should be done in Bazel, not Soong.
	h.library.flagExporterInfo = &flagExporterInfo
	// flag exporters consolidates properties like includes, flags, dependencies that should be
	// exported from this module to other modules
	ctx.SetProvider(FlagExporterInfoProvider, flagExporterInfo)

	// Dependencies on this library will expect collectedSnapshotHeaders to be set, otherwise
	// validation will fail. For now, set this to an empty list.
	// TODO(cparsons): More closely mirror the collectHeadersForSnapshot implementation.
	h.library.collectedSnapshotHeaders = android.Paths{}

	return true
}

// cc_library_headers contains a set of c/c++ headers which are imported by
// other soong cc modules using the header_libs property. For best practices,
// use export_include_dirs property or LOCAL_EXPORT_C_INCLUDE_DIRS for
// Make.
func LibraryHeaderFactory() android.Module {
	module, library := NewLibrary(android.HostAndDeviceSupported)
	library.HeaderOnly()
	module.sdkMemberTypes = []android.SdkMemberType{headersLibrarySdkMemberType}
	module.bazelHandler = &libraryHeaderBazelHander{module: module, library: library}
	return module.Init()
}

// cc_prebuilt_library_headers is a prebuilt version of cc_library_headers
func prebuiltLibraryHeaderFactory() android.Module {
	module, library := NewPrebuiltLibrary(android.HostAndDeviceSupported)
	library.HeaderOnly()
	return module.Init()
}

type bazelCcLibraryHeadersAttributes struct {
	Copts    bazel.StringListAttribute
	Hdrs     bazel.LabelListAttribute
	Includes bazel.StringListAttribute
	Deps     bazel.LabelListAttribute
}

type bazelCcLibraryHeaders struct {
	android.BazelTargetModuleBase
	bazelCcLibraryHeadersAttributes
}

func BazelCcLibraryHeadersFactory() android.Module {
	module := &bazelCcLibraryHeaders{}
	module.AddProperties(&module.bazelCcLibraryHeadersAttributes)
	android.InitBazelTargetModule(module)
	return module
}

func CcLibraryHeadersBp2Build(ctx android.TopDownMutatorContext) {
	module, ok := ctx.Module().(*Module)
	if !ok {
		// Not a cc module
		return
	}

	if !module.ConvertWithBp2build(ctx) {
		return
	}

	if ctx.ModuleType() != "cc_library_headers" {
		return
	}

	exportedIncludes := bp2BuildParseExportedIncludes(ctx, module)
	compilerAttrs := bp2BuildParseCompilerProps(ctx, module)
	linkerAttrs := bp2BuildParseLinkerProps(ctx, module)

	attrs := &bazelCcLibraryHeadersAttributes{
		Copts:    compilerAttrs.copts,
		Includes: exportedIncludes,
		Deps:     linkerAttrs.deps,
	}

	props := bazel.BazelTargetModuleProperties{
		Rule_class:        "cc_library_headers",
		Bzl_load_location: "//build/bazel/rules:cc_library_headers.bzl",
	}

	ctx.CreateBazelTargetModule(BazelCcLibraryHeadersFactory, module.Name(), props, attrs)
}

func (m *bazelCcLibraryHeaders) Name() string {
	return m.BaseModuleName()
}

func (m *bazelCcLibraryHeaders) GenerateAndroidBuildActions(ctx android.ModuleContext) {}
