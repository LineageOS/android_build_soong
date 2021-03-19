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

// cc_library_headers contains a set of c/c++ headers which are imported by
// other soong cc modules using the header_libs property. For best practices,
// use export_include_dirs property or LOCAL_EXPORT_C_INCLUDE_DIRS for
// Make.
func LibraryHeaderFactory() android.Module {
	module, library := NewLibrary(android.HostAndDeviceSupported)
	library.HeaderOnly()
	module.sdkMemberTypes = []android.SdkMemberType{headersLibrarySdkMemberType}
	return module.Init()
}

// cc_prebuilt_library_headers is a prebuilt version of cc_library_headers
func prebuiltLibraryHeaderFactory() android.Module {
	module, library := NewPrebuiltLibrary(android.HostAndDeviceSupported)
	library.HeaderOnly()
	return module.Init()
}

type bazelCcLibraryHeadersAttributes struct {
	Hdrs     bazel.LabelList
	Includes bazel.LabelList
	Deps     bazel.LabelList
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

	if !module.ConvertWithBp2build() {
		return
	}

	if ctx.ModuleType() != "cc_library_headers" {
		return
	}

	lib, _ := module.linker.(*libraryDecorator)

	// list of directories that will be added to the include path (using -I) for this
	// module and any module that links against this module.
	includeDirs := lib.flagExporter.Properties.Export_system_include_dirs
	includeDirs = append(includeDirs, lib.flagExporter.Properties.Export_include_dirs...)
	includeDirLabels := android.BazelLabelForModuleSrc(ctx, includeDirs)

	var includeDirGlobs []string
	for _, includeDir := range includeDirs {
		includeDirGlobs = append(includeDirGlobs, includeDir+"/**/*.h")
	}

	headerLabels := android.BazelLabelForModuleSrc(ctx, includeDirGlobs)

	// list of modules that should only provide headers for this module.
	var headerLibs []string
	for _, linkerProps := range lib.linkerProps() {
		if baseLinkerProps, ok := linkerProps.(*BaseLinkerProperties); ok {
			headerLibs = baseLinkerProps.Export_header_lib_headers
			break
		}
	}
	headerLibLabels := android.BazelLabelForModuleDeps(ctx, headerLibs)

	attrs := &bazelCcLibraryHeadersAttributes{
		Includes: includeDirLabels,
		Hdrs:     headerLabels,
		Deps:     headerLibLabels,
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
