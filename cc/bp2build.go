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
	"strings"
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

// bp2BuildParseCompilerProps returns copts, srcs and hdrs and other attributes.
func bp2BuildParseCompilerProps(
	ctx android.TopDownMutatorContext,
	module *Module) (
	copts bazel.StringListAttribute,
	srcs bazel.LabelListAttribute,
	hdrs bazel.LabelListAttribute) {

	hdrsAndSrcs := func(baseCompilerProps *BaseCompilerProperties) (bazel.LabelList, bazel.LabelList) {
		srcsList := android.BazelLabelForModuleSrcExcludes(
			ctx, baseCompilerProps.Srcs, baseCompilerProps.Exclude_srcs)
		hdrsList := android.BazelLabelForModuleSrc(ctx, srcsList.LooseHdrsGlobs(headerExts))
		return hdrsList, srcsList
	}

	for _, props := range module.compiler.compilerProps() {
		if baseCompilerProps, ok := props.(*BaseCompilerProperties); ok {
			hdrs.Value, srcs.Value = hdrsAndSrcs(baseCompilerProps)
			copts.Value = baseCompilerProps.Cflags
			break
		}
	}

	for arch, props := range module.GetArchProperties(&BaseCompilerProperties{}) {
		if baseCompilerProps, ok := props.(*BaseCompilerProperties); ok {
			hdrsList, srcsList := hdrsAndSrcs(baseCompilerProps)
			hdrs.SetValueForArch(arch.Name, bazel.SubtractBazelLabelList(hdrsList, hdrs.Value))
			srcs.SetValueForArch(arch.Name, srcsList)
			copts.SetValueForArch(arch.Name, baseCompilerProps.Cflags)
		}
	}

	for os, props := range module.GetTargetProperties(&BaseCompilerProperties{}) {
		if baseCompilerProps, ok := props.(*BaseCompilerProperties); ok {
			hdrsList, srcsList := hdrsAndSrcs(baseCompilerProps)
			hdrs.SetValueForOS(os.Name, bazel.SubtractBazelLabelList(hdrsList, hdrs.Value))
			srcs.SetValueForOS(os.Name, srcsList)
			copts.SetValueForOS(os.Name, baseCompilerProps.Cflags)
		}
	}

	return copts, srcs, hdrs
}

// bp2BuildParseLinkerProps creates a label list attribute containing the header library deps of a module, including
// configurable attribute values.
func bp2BuildParseLinkerProps(
	ctx android.TopDownMutatorContext, module *Module) (bazel.LabelListAttribute, bazel.StringListAttribute) {

	var deps bazel.LabelListAttribute
	var linkopts bazel.StringListAttribute

	for _, linkerProps := range module.linker.linkerProps() {
		if baseLinkerProps, ok := linkerProps.(*BaseLinkerProperties); ok {
			libs := baseLinkerProps.Header_libs
			libs = append(libs, baseLinkerProps.Export_header_lib_headers...)
			deps = bazel.MakeLabelListAttribute(
				android.BazelLabelForModuleDeps(ctx, android.SortedUniqueStrings(libs)))
			linkopts.Value = baseLinkerProps.Ldflags
			break
		}
	}

	for arch, p := range module.GetArchProperties(&BaseLinkerProperties{}) {
		if baseLinkerProps, ok := p.(*BaseLinkerProperties); ok {
			libs := baseLinkerProps.Header_libs
			libs = append(libs, baseLinkerProps.Export_header_lib_headers...)
			libs = android.SortedUniqueStrings(libs)
			deps.SetValueForArch(arch.Name, android.BazelLabelForModuleDeps(ctx, libs))
			linkopts.SetValueForArch(arch.Name, baseLinkerProps.Ldflags)
		}
	}

	for os, p := range module.GetTargetProperties(&BaseLinkerProperties{}) {
		if baseLinkerProps, ok := p.(*BaseLinkerProperties); ok {
			libs := baseLinkerProps.Header_libs
			libs = append(libs, baseLinkerProps.Export_header_lib_headers...)
			libs = android.SortedUniqueStrings(libs)
			deps.SetValueForOS(os.Name, android.BazelLabelForModuleDeps(ctx, libs))
			linkopts.SetValueForOS(os.Name, baseLinkerProps.Ldflags)
		}
	}

	return deps, linkopts
}

func bp2BuildListHeadersInDir(ctx android.TopDownMutatorContext, includeDir string) bazel.LabelList {
	globs := bazel.GlobsInDir(includeDir, includeDir != ".", headerExts)
	return android.BazelLabelForModuleSrc(ctx, globs)
}

// Bazel wants include paths to be relative to the module
func bp2BuildMakePathsRelativeToModule(ctx android.TopDownMutatorContext, paths []string) []string {
	var relativePaths []string
	for _, path := range paths {
		relativePath := strings.TrimPrefix(path, ctx.ModuleDir()+"/")
		relativePaths = append(relativePaths, relativePath)
	}
	return relativePaths
}

// bp2BuildParseExportedIncludes creates a label list attribute contains the
// exported included directories of a module.
func bp2BuildParseExportedIncludes(ctx android.TopDownMutatorContext, module *Module) (bazel.StringListAttribute, bazel.LabelListAttribute) {
	libraryDecorator := module.linker.(*libraryDecorator)

	includeDirs := libraryDecorator.flagExporter.Properties.Export_system_include_dirs
	includeDirs = append(includeDirs, libraryDecorator.flagExporter.Properties.Export_include_dirs...)
	includeDirs = bp2BuildMakePathsRelativeToModule(ctx, includeDirs)
	includeDirsAttribute := bazel.MakeStringListAttribute(includeDirs)

	var headersAttribute bazel.LabelListAttribute
	var headers bazel.LabelList
	for _, includeDir := range includeDirs {
		headers.Append(bp2BuildListHeadersInDir(ctx, includeDir))
	}
	headers = bazel.UniqueBazelLabelList(headers)
	headersAttribute.Value = headers

	for arch, props := range module.GetArchProperties(&FlagExporterProperties{}) {
		if flagExporterProperties, ok := props.(*FlagExporterProperties); ok {
			archIncludeDirs := flagExporterProperties.Export_system_include_dirs
			archIncludeDirs = append(archIncludeDirs, flagExporterProperties.Export_include_dirs...)
			archIncludeDirs = bp2BuildMakePathsRelativeToModule(ctx, archIncludeDirs)

			// To avoid duplicate includes when base includes + arch includes are combined
			archIncludeDirs = bazel.SubtractStrings(archIncludeDirs, includeDirs)

			if len(archIncludeDirs) > 0 {
				includeDirsAttribute.SetValueForArch(arch.Name, archIncludeDirs)
			}

			var archHeaders bazel.LabelList
			for _, archIncludeDir := range archIncludeDirs {
				archHeaders.Append(bp2BuildListHeadersInDir(ctx, archIncludeDir))
			}
			archHeaders = bazel.UniqueBazelLabelList(archHeaders)

			// To avoid duplicate headers when base headers + arch headers are combined
			archHeaders = bazel.SubtractBazelLabelList(archHeaders, headers)

			if len(archHeaders.Includes) > 0 || len(archHeaders.Excludes) > 0 {
				headersAttribute.SetValueForArch(arch.Name, archHeaders)
			}
		}
	}

	return includeDirsAttribute, headersAttribute
}
