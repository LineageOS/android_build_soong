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
			allDeps = append(allDeps, baseLinkerProps.Static_libs...)
			allDeps = append(allDeps, baseLinkerProps.Whole_static_libs...)
		}
	}

	for _, p := range module.GetArchProperties(ctx, &BaseLinkerProperties{}) {
		// arch specific linker props
		if baseLinkerProps, ok := p.(*BaseLinkerProperties); ok {
			allDeps = append(allDeps, baseLinkerProps.Header_libs...)
			allDeps = append(allDeps, baseLinkerProps.Export_header_lib_headers...)
			allDeps = append(allDeps, baseLinkerProps.Static_libs...)
			allDeps = append(allDeps, baseLinkerProps.Whole_static_libs...)
		}
	}

	// Deps in the static: { .. } and shared: { .. } props of a cc_library.
	if lib, ok := module.compiler.(*libraryDecorator); ok {
		allDeps = append(allDeps, lib.SharedProperties.Shared.Static_libs...)
		allDeps = append(allDeps, lib.SharedProperties.Shared.Whole_static_libs...)
		allDeps = append(allDeps, lib.SharedProperties.Shared.Shared_libs...)
		allDeps = append(allDeps, lib.SharedProperties.Shared.System_shared_libs...)

		allDeps = append(allDeps, lib.StaticProperties.Static.Static_libs...)
		allDeps = append(allDeps, lib.StaticProperties.Static.Whole_static_libs...)
		allDeps = append(allDeps, lib.StaticProperties.Static.Shared_libs...)
		allDeps = append(allDeps, lib.StaticProperties.Static.System_shared_libs...)
	}

	ctx.AddDependency(module, nil, android.SortedUniqueStrings(allDeps)...)
}

type sharedAttributes struct {
	copts            bazel.StringListAttribute
	srcs             bazel.LabelListAttribute
	staticDeps       bazel.LabelListAttribute
	dynamicDeps      bazel.LabelListAttribute
	wholeArchiveDeps bazel.LabelListAttribute
}

// bp2buildParseSharedProps returns the attributes for the shared variant of a cc_library.
func bp2BuildParseSharedProps(ctx android.TopDownMutatorContext, module *Module) sharedAttributes {
	lib, ok := module.compiler.(*libraryDecorator)
	if !ok {
		return sharedAttributes{}
	}

	copts := bazel.StringListAttribute{Value: lib.SharedProperties.Shared.Cflags}

	srcs := bazel.LabelListAttribute{
		Value: android.BazelLabelForModuleSrc(ctx, lib.SharedProperties.Shared.Srcs)}

	staticDeps := bazel.LabelListAttribute{
		Value: android.BazelLabelForModuleDeps(ctx, lib.SharedProperties.Shared.Static_libs)}

	dynamicDeps := bazel.LabelListAttribute{
		Value: android.BazelLabelForModuleDeps(ctx, lib.SharedProperties.Shared.Shared_libs)}

	wholeArchiveDeps := bazel.LabelListAttribute{
		Value: android.BazelLabelForModuleDeps(ctx, lib.SharedProperties.Shared.Whole_static_libs)}

	return sharedAttributes{
		copts:            copts,
		srcs:             srcs,
		staticDeps:       staticDeps,
		dynamicDeps:      dynamicDeps,
		wholeArchiveDeps: wholeArchiveDeps,
	}
}

type staticAttributes struct {
	copts            bazel.StringListAttribute
	srcs             bazel.LabelListAttribute
	staticDeps       bazel.LabelListAttribute
	dynamicDeps      bazel.LabelListAttribute
	wholeArchiveDeps bazel.LabelListAttribute
}

// bp2buildParseStaticProps returns the attributes for the static variant of a cc_library.
func bp2BuildParseStaticProps(ctx android.TopDownMutatorContext, module *Module) staticAttributes {
	lib, ok := module.compiler.(*libraryDecorator)
	if !ok {
		return staticAttributes{}
	}

	copts := bazel.StringListAttribute{Value: lib.StaticProperties.Static.Cflags}

	srcs := bazel.LabelListAttribute{
		Value: android.BazelLabelForModuleSrc(ctx, lib.StaticProperties.Static.Srcs)}

	staticDeps := bazel.LabelListAttribute{
		Value: android.BazelLabelForModuleDeps(ctx, lib.StaticProperties.Static.Static_libs)}

	dynamicDeps := bazel.LabelListAttribute{
		Value: android.BazelLabelForModuleDeps(ctx, lib.StaticProperties.Static.Shared_libs)}

	wholeArchiveDeps := bazel.LabelListAttribute{
		Value: android.BazelLabelForModuleDeps(ctx, lib.StaticProperties.Static.Whole_static_libs)}

	return staticAttributes{
		copts:            copts,
		srcs:             srcs,
		staticDeps:       staticDeps,
		dynamicDeps:      dynamicDeps,
		wholeArchiveDeps: wholeArchiveDeps,
	}
}

// Convenience struct to hold all attributes parsed from compiler properties.
type compilerAttributes struct {
	copts    bazel.StringListAttribute
	srcs     bazel.LabelListAttribute
	includes bazel.StringListAttribute
}

// bp2BuildParseCompilerProps returns copts, srcs and hdrs and other attributes.
func bp2BuildParseCompilerProps(ctx android.TopDownMutatorContext, module *Module) compilerAttributes {
	var srcs bazel.LabelListAttribute
	var copts bazel.StringListAttribute

	// Creates the -I flag for a directory, while making the directory relative
	// to the exec root for Bazel to work.
	includeFlag := func(dir string) string {
		// filepath.Join canonicalizes the path, i.e. it takes care of . or .. elements.
		return "-I" + filepath.Join(ctx.ModuleDir(), dir)
	}

	// Parse the list of module-relative include directories (-I).
	parseLocalIncludeDirs := func(baseCompilerProps *BaseCompilerProperties) []string {
		// include_dirs are root-relative, not module-relative.
		includeDirs := bp2BuildMakePathsRelativeToModule(ctx, baseCompilerProps.Include_dirs)
		return append(includeDirs, baseCompilerProps.Local_include_dirs...)
	}

	// Parse the list of copts.
	parseCopts := func(baseCompilerProps *BaseCompilerProperties) []string {
		var copts []string
		for _, flag := range append(baseCompilerProps.Cflags, baseCompilerProps.Cppflags...) {
			// Soong's cflags can contain spaces, like `-include header.h`. For
			// Bazel's copts, split them up to be compatible with the
			// no_copts_tokenization feature.
			copts = append(copts, strings.Split(flag, " ")...)
		}
		for _, dir := range parseLocalIncludeDirs(baseCompilerProps) {
			copts = append(copts, includeFlag(dir))
		}
		return copts
	}

	// baseSrcs contain the list of src files that are used for every configuration.
	var baseSrcs []string
	// baseExcludeSrcs contain the list of src files that are excluded for every configuration.
	var baseExcludeSrcs []string
	// baseSrcsLabelList is a clone of the base srcs LabelList, used for computing the
	// arch or os specific srcs later.
	var baseSrcsLabelList bazel.LabelList

	// Parse srcs from an arch or OS's props value, taking the base srcs and
	// exclude srcs into account.
	parseSrcs := func(baseCompilerProps *BaseCompilerProperties) bazel.LabelList {
		// Combine the base srcs and arch-specific srcs
		allSrcs := append(baseSrcs, baseCompilerProps.Srcs...)
		// Combine the base exclude_srcs and configuration-specific exclude_srcs
		allExcludeSrcs := append(baseExcludeSrcs, baseCompilerProps.Exclude_srcs...)
		return android.BazelLabelForModuleSrcExcludes(ctx, allSrcs, allExcludeSrcs)
	}

	for _, props := range module.compiler.compilerProps() {
		if baseCompilerProps, ok := props.(*BaseCompilerProperties); ok {
			srcs.Value = parseSrcs(baseCompilerProps)
			copts.Value = parseCopts(baseCompilerProps)

			// Used for arch-specific srcs later.
			baseSrcs = baseCompilerProps.Srcs
			baseExcludeSrcs = baseCompilerProps.Exclude_srcs
			baseSrcsLabelList = parseSrcs(baseCompilerProps)
			break
		}
	}

	// Handle include_build_directory prop. If the property is true, then the
	// target has access to all headers recursively in the package, and has
	// "-I<module-dir>" in its copts.
	if c, ok := module.compiler.(*baseCompiler); ok && c.includeBuildDirectory() {
		copts.Value = append(copts.Value, includeFlag("."))
	} else if c, ok := module.compiler.(*libraryDecorator); ok && c.includeBuildDirectory() {
		copts.Value = append(copts.Value, includeFlag("."))
	}

	for arch, props := range module.GetArchProperties(ctx, &BaseCompilerProperties{}) {
		if baseCompilerProps, ok := props.(*BaseCompilerProperties); ok {
			// If there's arch specific srcs or exclude_srcs, generate a select entry for it.
			// TODO(b/186153868): do this for OS specific srcs and exclude_srcs too.
			if len(baseCompilerProps.Srcs) > 0 || len(baseCompilerProps.Exclude_srcs) > 0 {
				srcsList := parseSrcs(baseCompilerProps)
				srcs.SetValueForArch(arch.Name, srcsList)
				// The base srcs value should not contain any arch-specific excludes.
				srcs.Value = bazel.SubtractBazelLabelList(srcs.Value, bazel.LabelList{Includes: srcsList.Excludes})
			}

			copts.SetValueForArch(arch.Name, parseCopts(baseCompilerProps))
		}
	}

	// After going through all archs, delete the duplicate files in the arch
	// values that are already in the base srcs.Value.
	for arch, props := range module.GetArchProperties(ctx, &BaseCompilerProperties{}) {
		if _, ok := props.(*BaseCompilerProperties); ok {
			srcs.SetValueForArch(arch.Name, bazel.SubtractBazelLabelList(srcs.GetValueForArch(arch.Name), srcs.Value))
		}
	}

	// Now that the srcs.Value list is finalized, compare it with the original
	// list, and put the difference into the default condition for the arch
	// select.
	defaultsSrcs := bazel.SubtractBazelLabelList(baseSrcsLabelList, srcs.Value)
	// TODO(b/186153868): handle the case with multiple variant types, e.g. when arch and os are both used.
	srcs.SetValueForArch(bazel.CONDITIONS_DEFAULT, defaultsSrcs)

	// Handle OS specific props.
	for os, props := range module.GetTargetProperties(&BaseCompilerProperties{}) {
		if baseCompilerProps, ok := props.(*BaseCompilerProperties); ok {
			srcsList := parseSrcs(baseCompilerProps)
			// TODO(b/186153868): add support for os-specific srcs and exclude_srcs
			srcs.SetValueForOS(os.Name, bazel.SubtractBazelLabelList(srcsList, baseSrcsLabelList))
			copts.SetValueForOS(os.Name, parseCopts(baseCompilerProps))
		}
	}

	return compilerAttributes{
		srcs:  srcs,
		copts: copts,
	}
}

// Convenience struct to hold all attributes parsed from linker properties.
type linkerAttributes struct {
	deps             bazel.LabelListAttribute
	dynamicDeps      bazel.LabelListAttribute
	wholeArchiveDeps bazel.LabelListAttribute
	linkopts         bazel.StringListAttribute
	versionScript    bazel.LabelAttribute
}

// FIXME(b/187655838): Use the existing linkerFlags() function instead of duplicating logic here
func getBp2BuildLinkerFlags(linkerProperties *BaseLinkerProperties) []string {
	flags := linkerProperties.Ldflags
	if !BoolDefault(linkerProperties.Pack_relocations, true) {
		flags = append(flags, "-Wl,--pack-dyn-relocs=none")
	}
	return flags
}

// bp2BuildParseLinkerProps parses the linker properties of a module, including
// configurable attribute values.
func bp2BuildParseLinkerProps(ctx android.TopDownMutatorContext, module *Module) linkerAttributes {
	var deps bazel.LabelListAttribute
	var dynamicDeps bazel.LabelListAttribute
	var wholeArchiveDeps bazel.LabelListAttribute
	var linkopts bazel.StringListAttribute
	var versionScript bazel.LabelAttribute

	for _, linkerProps := range module.linker.linkerProps() {
		if baseLinkerProps, ok := linkerProps.(*BaseLinkerProperties); ok {
			libs := baseLinkerProps.Header_libs
			libs = append(libs, baseLinkerProps.Export_header_lib_headers...)
			libs = append(libs, baseLinkerProps.Static_libs...)
			wholeArchiveLibs := baseLinkerProps.Whole_static_libs
			libs = android.SortedUniqueStrings(libs)
			deps = bazel.MakeLabelListAttribute(android.BazelLabelForModuleDeps(ctx, libs))
			linkopts.Value = getBp2BuildLinkerFlags(baseLinkerProps)
			wholeArchiveDeps = bazel.MakeLabelListAttribute(android.BazelLabelForModuleDeps(ctx, wholeArchiveLibs))

			if baseLinkerProps.Version_script != nil {
				versionScript.Value = android.BazelLabelForModuleSrcSingle(ctx, *baseLinkerProps.Version_script)
			}

			sharedLibs := baseLinkerProps.Shared_libs
			dynamicDeps = bazel.MakeLabelListAttribute(android.BazelLabelForModuleDeps(ctx, sharedLibs))

			break
		}
	}

	for arch, p := range module.GetArchProperties(ctx, &BaseLinkerProperties{}) {
		if baseLinkerProps, ok := p.(*BaseLinkerProperties); ok {
			libs := baseLinkerProps.Header_libs
			libs = append(libs, baseLinkerProps.Export_header_lib_headers...)
			libs = append(libs, baseLinkerProps.Static_libs...)
			wholeArchiveLibs := baseLinkerProps.Whole_static_libs
			libs = android.SortedUniqueStrings(libs)
			deps.SetValueForArch(arch.Name, android.BazelLabelForModuleDeps(ctx, libs))
			linkopts.SetValueForArch(arch.Name, getBp2BuildLinkerFlags(baseLinkerProps))
			wholeArchiveDeps.SetValueForArch(arch.Name, android.BazelLabelForModuleDeps(ctx, wholeArchiveLibs))

			if baseLinkerProps.Version_script != nil {
				versionScript.SetValueForArch(arch.Name,
					android.BazelLabelForModuleSrcSingle(ctx, *baseLinkerProps.Version_script))
			}

			sharedLibs := baseLinkerProps.Shared_libs
			dynamicDeps.SetValueForArch(arch.Name, android.BazelLabelForModuleDeps(ctx, sharedLibs))
		}
	}

	for os, p := range module.GetTargetProperties(&BaseLinkerProperties{}) {
		if baseLinkerProps, ok := p.(*BaseLinkerProperties); ok {
			libs := baseLinkerProps.Header_libs
			libs = append(libs, baseLinkerProps.Export_header_lib_headers...)
			libs = append(libs, baseLinkerProps.Static_libs...)
			wholeArchiveLibs := baseLinkerProps.Whole_static_libs
			libs = android.SortedUniqueStrings(libs)
			wholeArchiveDeps.SetValueForOS(os.Name, android.BazelLabelForModuleDeps(ctx, wholeArchiveLibs))
			deps.SetValueForOS(os.Name, android.BazelLabelForModuleDeps(ctx, libs))

			linkopts.SetValueForOS(os.Name, getBp2BuildLinkerFlags(baseLinkerProps))

			sharedLibs := baseLinkerProps.Shared_libs
			dynamicDeps.SetValueForOS(os.Name, android.BazelLabelForModuleDeps(ctx, sharedLibs))
		}
	}

	return linkerAttributes{
		deps:             deps,
		dynamicDeps:      dynamicDeps,
		wholeArchiveDeps: wholeArchiveDeps,
		linkopts:         linkopts,
		versionScript:    versionScript,
	}
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
// exported included directories of a module.
func bp2BuildParseExportedIncludes(ctx android.TopDownMutatorContext, module *Module) bazel.StringListAttribute {
	libraryDecorator := module.linker.(*libraryDecorator)

	// Export_system_include_dirs and export_include_dirs are already module dir
	// relative, so they don't need to be relativized like include_dirs, which
	// are root-relative.
	includeDirs := libraryDecorator.flagExporter.Properties.Export_system_include_dirs
	includeDirs = append(includeDirs, libraryDecorator.flagExporter.Properties.Export_include_dirs...)
	includeDirsAttribute := bazel.MakeStringListAttribute(includeDirs)

	for arch, props := range module.GetArchProperties(ctx, &FlagExporterProperties{}) {
		if flagExporterProperties, ok := props.(*FlagExporterProperties); ok {
			archIncludeDirs := flagExporterProperties.Export_system_include_dirs
			archIncludeDirs = append(archIncludeDirs, flagExporterProperties.Export_include_dirs...)

			// To avoid duplicate includes when base includes + arch includes are combined
			// FIXME: This doesn't take conflicts between arch and os includes into account
			archIncludeDirs = bazel.SubtractStrings(archIncludeDirs, includeDirs)

			if len(archIncludeDirs) > 0 {
				includeDirsAttribute.SetValueForArch(arch.Name, archIncludeDirs)
			}
		}
	}

	for os, props := range module.GetTargetProperties(&FlagExporterProperties{}) {
		if flagExporterProperties, ok := props.(*FlagExporterProperties); ok {
			osIncludeDirs := flagExporterProperties.Export_system_include_dirs
			osIncludeDirs = append(osIncludeDirs, flagExporterProperties.Export_include_dirs...)

			// To avoid duplicate includes when base includes + os includes are combined
			// FIXME: This doesn't take conflicts between arch and os includes into account
			osIncludeDirs = bazel.SubtractStrings(osIncludeDirs, includeDirs)

			if len(osIncludeDirs) > 0 {
				includeDirsAttribute.SetValueForOS(os.Name, osIncludeDirs)
			}
		}
	}

	return includeDirsAttribute
}
