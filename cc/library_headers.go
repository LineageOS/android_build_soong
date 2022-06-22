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
	"android/soong/bazel/cquery"
)

func init() {
	RegisterLibraryHeadersBuildComponents(android.InitRegistrationContext)

	// Register sdk member types.
	android.RegisterSdkMemberType(headersLibrarySdkMemberType)

}

var headersLibrarySdkMemberType = &librarySdkMemberType{
	SdkMemberTypeBase: android.SdkMemberTypeBase{
		PropertyName:    "native_header_libs",
		SupportsSdk:     true,
		HostOsDependent: true,
		Traits: []android.SdkMemberTrait{
			nativeBridgeSdkTrait,
			ramdiskImageRequiredSdkTrait,
			recoveryImageRequiredSdkTrait,
		},
	},
	prebuiltModuleType: "cc_prebuilt_library_headers",
	noOutputFiles:      true,
}

func RegisterLibraryHeadersBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("cc_library_headers", LibraryHeaderFactory)
	ctx.RegisterModuleType("cc_prebuilt_library_headers", prebuiltLibraryHeaderFactory)
}

type libraryHeaderBazelHandler struct {
	module  *Module
	library *libraryDecorator
}

var _ BazelHandler = (*libraryHeaderBazelHandler)(nil)

func (handler *libraryHeaderBazelHandler) QueueBazelCall(ctx android.BaseModuleContext, label string) {
	bazelCtx := ctx.Config().BazelContext
	bazelCtx.QueueBazelRequest(label, cquery.GetCcInfo, android.GetConfigKey(ctx))
}

func (h *libraryHeaderBazelHandler) ProcessBazelQueryResponse(ctx android.ModuleContext, label string) {
	bazelCtx := ctx.Config().BazelContext
	ccInfo, err := bazelCtx.GetCcInfo(label, android.GetConfigKey(ctx))
	if err != nil {
		ctx.ModuleErrorf(err.Error())
		return
	}

	outputPaths := ccInfo.OutputFiles
	if len(outputPaths) != 1 {
		ctx.ModuleErrorf("expected exactly one output file for %q, but got %q", label, outputPaths)
		return
	}

	outputPath := android.PathForBazelOut(ctx, outputPaths[0])
	h.module.outputFile = android.OptionalPathForPath(outputPath)

	// HeaderLibraryInfo is an empty struct to indicate to dependencies that this is a header library
	ctx.SetProvider(HeaderLibraryInfoProvider, HeaderLibraryInfo{})

	h.library.setFlagExporterInfoFromCcInfo(ctx, ccInfo)

	// Dependencies on this library will expect collectedSnapshotHeaders to be set, otherwise
	// validation will fail. For now, set this to an empty list.
	// TODO(cparsons): More closely mirror the collectHeadersForSnapshot implementation.
	h.library.collectedSnapshotHeaders = android.Paths{}
}

// cc_library_headers contains a set of c/c++ headers which are imported by
// other soong cc modules using the header_libs property. For best practices,
// use export_include_dirs property or LOCAL_EXPORT_C_INCLUDE_DIRS for
// Make.
func LibraryHeaderFactory() android.Module {
	module, library := NewLibrary(android.HostAndDeviceSupported)
	library.HeaderOnly()
	module.sdkMemberTypes = []android.SdkMemberType{headersLibrarySdkMemberType}
	module.bazelable = true
	module.bazelHandler = &libraryHeaderBazelHandler{module: module, library: library}
	return module.Init()
}

// cc_prebuilt_library_headers is a prebuilt version of cc_library_headers
func prebuiltLibraryHeaderFactory() android.Module {
	module, library := NewPrebuiltLibrary(android.HostAndDeviceSupported, "")
	library.HeaderOnly()
	module.bazelable = true
	module.bazelHandler = &ccLibraryBazelHandler{module: module}
	return module.Init()
}

type bazelCcLibraryHeadersAttributes struct {
	Hdrs                     bazel.LabelListAttribute
	Export_includes          bazel.StringListAttribute
	Export_absolute_includes bazel.StringListAttribute
	Export_system_includes   bazel.StringListAttribute
	Deps                     bazel.LabelListAttribute
	Implementation_deps      bazel.LabelListAttribute
	System_dynamic_deps      bazel.LabelListAttribute
	sdkAttributes
}

func libraryHeadersBp2Build(ctx android.TopDownMutatorContext, module *Module) {
	baseAttributes := bp2BuildParseBaseProps(ctx, module)
	exportedIncludes := bp2BuildParseExportedIncludes(ctx, module, &baseAttributes.includes)
	linkerAttrs := baseAttributes.linkerAttributes

	attrs := &bazelCcLibraryHeadersAttributes{
		Export_includes:          exportedIncludes.Includes,
		Export_absolute_includes: exportedIncludes.AbsoluteIncludes,
		Export_system_includes:   exportedIncludes.SystemIncludes,
		Deps:                     linkerAttrs.deps,
		System_dynamic_deps:      linkerAttrs.systemDynamicDeps,
		Hdrs:                     baseAttributes.hdrs,
		sdkAttributes:            bp2BuildParseSdkAttributes(module),
	}

	props := bazel.BazelTargetModuleProperties{
		Rule_class:        "cc_library_headers",
		Bzl_load_location: "//build/bazel/rules/cc:cc_library_headers.bzl",
	}

	ctx.CreateBazelTargetModule(props, android.CommonAttributes{Name: module.Name()}, attrs)
}
