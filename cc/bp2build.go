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
	"android/soong/bazel"
)

// bp2build functions and helpers for converting cc_* modules to Bazel.

func init() {
	android.DepsBp2BuildMutators(RegisterDepsBp2Build)
}

func RegisterDepsBp2Build(ctx android.RegisterMutatorsContext) {
	ctx.BottomUp("cc_bp2build_deps", depsBp2BuildMutator)
}

// A naive deps mutator to add deps on all modules across all combinations of
// target props for cc modules. This is needed to make module -> bazel label
// resolution work in the bp2build mutator later. This is probably
// the wrong way to do it, but it works.
//
// TODO(jingwen): can we create a custom os mutator in depsBp2BuildMutator to do this?
func depsBp2BuildMutator(ctx android.BottomUpMutatorContext) {
	module, ok := ctx.Module().(*Module)
	if !ok {
		// Not a cc module
		return
	}

	if !module.ConvertWithBp2build(ctx) {
		return
	}

	var allDeps []string

	for _, p := range module.GetTargetProperties(&BaseLinkerProperties{}) {
		// arch specific linker props
		if baseLinkerProps, ok := p.(*BaseLinkerProperties); ok {
			allDeps = append(allDeps, baseLinkerProps.Header_libs...)
			allDeps = append(allDeps, baseLinkerProps.Export_header_lib_headers...)
		}
	}

	ctx.AddDependency(module, nil, android.SortedUniqueStrings(allDeps)...)
}

// bp2BuildParseHeaderLibs creates a label list attribute containing the header library deps of a module, including
// configurable attribute values.
func bp2BuildParseHeaderLibs(ctx android.TopDownMutatorContext, module *Module) bazel.LabelListAttribute {
	var ret bazel.LabelListAttribute
	for _, linkerProps := range module.linker.linkerProps() {
		if baseLinkerProps, ok := linkerProps.(*BaseLinkerProperties); ok {
			libs := baseLinkerProps.Header_libs
			libs = append(libs, baseLinkerProps.Export_header_lib_headers...)
			ret = bazel.MakeLabelListAttribute(
				android.BazelLabelForModuleDeps(ctx, android.SortedUniqueStrings(libs)))
			break
		}
	}

	for os, p := range module.GetTargetProperties(&BaseLinkerProperties{}) {
		if baseLinkerProps, ok := p.(*BaseLinkerProperties); ok {
			libs := baseLinkerProps.Header_libs
			libs = append(libs, baseLinkerProps.Export_header_lib_headers...)
			libs = android.SortedUniqueStrings(libs)
			ret.SetValueForOS(os.Name, android.BazelLabelForModuleDeps(ctx, libs))
		}
	}

	return ret
}

// bp2BuildParseExportedIncludes creates a label list attribute contains the
// exported included directories of a module.
func bp2BuildParseExportedIncludes(ctx android.TopDownMutatorContext, module *Module) (bazel.LabelListAttribute, bazel.LabelListAttribute) {
	libraryDecorator := module.linker.(*libraryDecorator)

	includeDirs := libraryDecorator.flagExporter.Properties.Export_system_include_dirs
	includeDirs = append(includeDirs, libraryDecorator.flagExporter.Properties.Export_include_dirs...)

	includeDirsLabels := android.BazelLabelForModuleSrc(ctx, includeDirs)

	var includeDirGlobs []string
	for _, includeDir := range includeDirs {
		includeDirGlobs = append(includeDirGlobs, includeDir+"/**/*.h")
		includeDirGlobs = append(includeDirGlobs, includeDir+"/**/*.inc")
		includeDirGlobs = append(includeDirGlobs, includeDir+"/**/*.hpp")
	}

	headersLabels := android.BazelLabelForModuleSrc(ctx, includeDirGlobs)
	return bazel.MakeLabelListAttribute(includeDirsLabels), bazel.MakeLabelListAttribute(headersLabels)
}
