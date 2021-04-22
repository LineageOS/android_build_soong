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
	"path/filepath"
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
			allDeps = append(allDeps, baseLinkerProps.Static_libs...)
			allDeps = append(allDeps, baseLinkerProps.Whole_static_libs...)
		}
	}

	for _, p := range module.GetArchProperties(&BaseLinkerProperties{}) {
		// arch specific linker props
		if baseLinkerProps, ok := p.(*BaseLinkerProperties); ok {
			allDeps = append(allDeps, baseLinkerProps.Header_libs...)
			allDeps = append(allDeps, baseLinkerProps.Export_header_lib_headers...)
			allDeps = append(allDeps, baseLinkerProps.Static_libs...)
			allDeps = append(allDeps, baseLinkerProps.Whole_static_libs...)
		}
	}

	ctx.AddDependency(module, nil, android.SortedUniqueStrings(allDeps)...)
}

// Convenience struct to hold all attributes parsed from compiler properties.
type compilerAttributes struct {
	copts    bazel.StringListAttribute
	srcs     bazel.LabelListAttribute
	includes bazel.StringListAttribute
}

// bp2BuildParseCompilerProps returns copts, srcs and hdrs and other attributes.
func bp2BuildParseCompilerProps(ctx android.TopDownMutatorContext, module *Module) compilerAttributes {
	var localHdrs, srcs bazel.LabelListAttribute
	var copts bazel.StringListAttribute

	// Creates the -I flag for a directory, while making the directory relative
	// to the exec root for Bazel to work.
	includeFlag := func(dir string) string {
		// filepath.Join canonicalizes the path, i.e. it takes care of . or .. elements.
		return "-I" + filepath.Join(ctx.ModuleDir(), dir)
	}

	// Parse the list of srcs, excluding files from exclude_srcs.
	parseSrcs := func(baseCompilerProps *BaseCompilerProperties) bazel.LabelList {
		return android.BazelLabelForModuleSrcExcludes(ctx, baseCompilerProps.Srcs, baseCompilerProps.Exclude_srcs)
	}

	// Parse the list of module-relative include directories (-I).
	parseLocalIncludeDirs := func(baseCompilerProps *BaseCompilerProperties) []string {
		// include_dirs are root-relative, not module-relative.
		includeDirs := bp2BuildMakePathsRelativeToModule(ctx, baseCompilerProps.Include_dirs)
		return append(includeDirs, baseCompilerProps.Local_include_dirs...)
	}

	// Parse the list of copts.
	parseCopts := func(baseCompilerProps *BaseCompilerProperties) []string {
		copts := append([]string{}, baseCompilerProps.Cflags...)
		for _, dir := range parseLocalIncludeDirs(baseCompilerProps) {
			copts = append(copts, includeFlag(dir))
		}
		return copts
	}

	for _, props := range module.compiler.compilerProps() {
		if baseCompilerProps, ok := props.(*BaseCompilerProperties); ok {
			srcs.Value = parseSrcs(baseCompilerProps)
			copts.Value = parseCopts(baseCompilerProps)
			break
		}
	}

	if c, ok := module.compiler.(*baseCompiler); ok && c.includeBuildDirectory() {
		copts.Value = append(copts.Value, includeFlag("."))
		localHdrs.Value = bp2BuildListHeadersInDir(ctx, ".")
	} else if c, ok := module.compiler.(*libraryDecorator); ok && c.includeBuildDirectory() {
		copts.Value = append(copts.Value, includeFlag("."))
		localHdrs.Value = bp2BuildListHeadersInDir(ctx, ".")
	}

	for arch, props := range module.GetArchProperties(&BaseCompilerProperties{}) {
		if baseCompilerProps, ok := props.(*BaseCompilerProperties); ok {
			srcsList := parseSrcs(baseCompilerProps)
			srcs.SetValueForArch(arch.Name, bazel.SubtractBazelLabelList(srcsList, srcs.Value))
			copts.SetValueForArch(arch.Name, parseCopts(baseCompilerProps))
		}
	}

	for os, props := range module.GetTargetProperties(&BaseCompilerProperties{}) {
		if baseCompilerProps, ok := props.(*BaseCompilerProperties); ok {
			srcsList := parseSrcs(baseCompilerProps)
			srcs.SetValueForOS(os.Name, bazel.SubtractBazelLabelList(srcsList, srcs.Value))
			copts.SetValueForOS(os.Name, parseCopts(baseCompilerProps))
		}
	}

	// Combine local, non-exported hdrs into srcs
	srcs.Append(localHdrs)

	return compilerAttributes{
		srcs:  srcs,
		copts: copts,
	}
}

// Convenience struct to hold all attributes parsed from linker properties.
type linkerAttributes struct {
	deps     bazel.LabelListAttribute
	linkopts bazel.StringListAttribute
}

// bp2BuildParseLinkerProps creates a label list attribute containing the header library deps of a module, including
// configurable attribute values.
func bp2BuildParseLinkerProps(ctx android.TopDownMutatorContext, module *Module) linkerAttributes {
	var deps bazel.LabelListAttribute
	var linkopts bazel.StringListAttribute

	for _, linkerProps := range module.linker.linkerProps() {
		if baseLinkerProps, ok := linkerProps.(*BaseLinkerProperties); ok {
			libs := baseLinkerProps.Header_libs
			libs = append(libs, baseLinkerProps.Export_header_lib_headers...)
			libs = append(libs, baseLinkerProps.Static_libs...)
			libs = append(libs, baseLinkerProps.Whole_static_libs...)
			libs = android.SortedUniqueStrings(libs)
			deps = bazel.MakeLabelListAttribute(android.BazelLabelForModuleDeps(ctx, libs))
			linkopts.Value = baseLinkerProps.Ldflags
			break
		}
	}

	for arch, p := range module.GetArchProperties(&BaseLinkerProperties{}) {
		if baseLinkerProps, ok := p.(*BaseLinkerProperties); ok {
			libs := baseLinkerProps.Header_libs
			libs = append(libs, baseLinkerProps.Export_header_lib_headers...)
			libs = append(libs, baseLinkerProps.Static_libs...)
			libs = append(libs, baseLinkerProps.Whole_static_libs...)
			libs = android.SortedUniqueStrings(libs)
			deps.SetValueForArch(arch.Name, android.BazelLabelForModuleDeps(ctx, libs))
			linkopts.SetValueForArch(arch.Name, baseLinkerProps.Ldflags)
		}
	}

	for os, p := range module.GetTargetProperties(&BaseLinkerProperties{}) {
		if baseLinkerProps, ok := p.(*BaseLinkerProperties); ok {
			libs := baseLinkerProps.Header_libs
			libs = append(libs, baseLinkerProps.Export_header_lib_headers...)
			libs = append(libs, baseLinkerProps.Static_libs...)
			libs = append(libs, baseLinkerProps.Whole_static_libs...)
			libs = android.SortedUniqueStrings(libs)
			deps.SetValueForOS(os.Name, android.BazelLabelForModuleDeps(ctx, libs))
			linkopts.SetValueForOS(os.Name, baseLinkerProps.Ldflags)
		}
	}

	return linkerAttributes{
		deps:     deps,
		linkopts: linkopts,
	}
}

func bp2BuildListHeadersInDir(ctx android.TopDownMutatorContext, includeDir string) bazel.LabelList {
	globs := bazel.GlobsInDir(includeDir, true, headerExts)
	return android.BazelLabelForModuleSrc(ctx, globs)
}

// Relativize a list of root-relative paths with respect to the module's
// directory.
//
// include_dirs Soong prop are root-relative (b/183742505), but
// local_include_dirs, export_include_dirs and export_system_include_dirs are
// module dir relative. This function makes a list of paths entirely module dir
// relative.
//
// For the `include` attribute, Bazel wants the paths to be relative to the
// module.
func bp2BuildMakePathsRelativeToModule(ctx android.BazelConversionPathContext, paths []string) []string {
	var relativePaths []string
	for _, path := range paths {
		// Semantics of filepath.Rel: join(ModuleDir, rel(ModuleDir, path)) == path
		relativePath, err := filepath.Rel(ctx.ModuleDir(), path)
		if err != nil {
			panic(err)
		}
		relativePaths = append(relativePaths, relativePath)
	}
	return relativePaths
}

// bp2BuildParseExportedIncludes creates a string list attribute contains the
// exported included directories of a module, and a label list attribute
// containing the exported headers of a module.
func bp2BuildParseExportedIncludes(ctx android.TopDownMutatorContext, module *Module) (bazel.StringListAttribute, bazel.LabelListAttribute) {
	libraryDecorator := module.linker.(*libraryDecorator)

	// Export_system_include_dirs and export_include_dirs are already module dir
	// relative, so they don't need to be relativized like include_dirs, which
	// are root-relative.
	includeDirs := libraryDecorator.flagExporter.Properties.Export_system_include_dirs
	includeDirs = append(includeDirs, libraryDecorator.flagExporter.Properties.Export_include_dirs...)
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
