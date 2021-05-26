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
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/bazel"

	"github.com/google/blueprint/proptools"
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

	for _, osProps := range module.GetTargetProperties(ctx, &BaseCompilerProperties{}) {
		// os base compiler props
		if baseCompilerProps, ok := osProps.Properties.(*BaseCompilerProperties); ok {
			allDeps = append(allDeps, baseCompilerProps.Generated_headers...)
			allDeps = append(allDeps, baseCompilerProps.Generated_sources...)
		}
		// os + arch base compiler props
		for _, archProps := range osProps.ArchProperties {
			if baseCompilerProps, ok := archProps.(*BaseCompilerProperties); ok {
				allDeps = append(allDeps, baseCompilerProps.Generated_headers...)
				allDeps = append(allDeps, baseCompilerProps.Generated_sources...)
			}
		}
	}

	for _, props := range module.GetArchProperties(ctx, &BaseCompilerProperties{}) {
		// arch specific compiler props
		if baseCompilerProps, ok := props.(*BaseCompilerProperties); ok {
			allDeps = append(allDeps, baseCompilerProps.Generated_headers...)
			allDeps = append(allDeps, baseCompilerProps.Generated_sources...)
		}
	}

	for _, osProps := range module.GetTargetProperties(ctx, &BaseLinkerProperties{}) {
		// os specific linker props
		if baseLinkerProps, ok := osProps.Properties.(*BaseLinkerProperties); ok {
			allDeps = append(allDeps, baseLinkerProps.Header_libs...)
			allDeps = append(allDeps, baseLinkerProps.Export_header_lib_headers...)
			allDeps = append(allDeps, baseLinkerProps.Static_libs...)
			allDeps = append(allDeps, baseLinkerProps.Whole_static_libs...)
		}
		// os + arch base compiler props
		for _, archProps := range osProps.ArchProperties {
			if baseLinkerProps, ok := archProps.(*BaseLinkerProperties); ok {
				allDeps = append(allDeps, baseLinkerProps.Header_libs...)
				allDeps = append(allDeps, baseLinkerProps.Export_header_lib_headers...)
				allDeps = append(allDeps, baseLinkerProps.Static_libs...)
				allDeps = append(allDeps, baseLinkerProps.Whole_static_libs...)
			}
		}
	}

	for _, props := range module.GetArchProperties(ctx, &BaseLinkerProperties{}) {
		// arch specific linker props
		if baseLinkerProps, ok := props.(*BaseLinkerProperties); ok {
			allDeps = append(allDeps, baseLinkerProps.Header_libs...)
			allDeps = append(allDeps, baseLinkerProps.Export_header_lib_headers...)
			allDeps = append(allDeps, baseLinkerProps.Static_libs...)
			allDeps = append(allDeps, baseLinkerProps.Whole_static_libs...)
		}
	}

	// Deps in the static: { .. } and shared: { .. } props of a cc_library.
	if lib, ok := module.compiler.(*libraryDecorator); ok {
		appendDeps := func(deps []string, p StaticOrSharedProperties) []string {
			deps = append(deps, p.Static_libs...)
			deps = append(deps, p.Whole_static_libs...)
			deps = append(deps, p.Shared_libs...)
			return deps
		}

		allDeps = appendDeps(allDeps, lib.SharedProperties.Shared)
		allDeps = appendDeps(allDeps, lib.StaticProperties.Static)

		// TODO(b/186024507, b/186489250): Temporarily exclude adding
		// system_shared_libs deps until libc and libm builds.
		// allDeps = append(allDeps, lib.SharedProperties.Shared.System_shared_libs...)
		// allDeps = append(allDeps, lib.StaticProperties.Static.System_shared_libs...)

		// Deps in the target/arch nested static: { .. } and shared: { .. } props of a cc_library.
		// target: { <target>: shared: { ... } }
		for _, targetProps := range module.GetTargetProperties(ctx, &SharedProperties{}) {
			if p, ok := targetProps.Properties.(*SharedProperties); ok {
				allDeps = appendDeps(allDeps, p.Shared)
			}
			for _, archProperties := range targetProps.ArchProperties {
				if p, ok := archProperties.(*SharedProperties); ok {
					allDeps = appendDeps(allDeps, p.Shared)
				}
			}
		}
		// target: { <target>: static: { ... } }
		for _, targetProps := range module.GetTargetProperties(ctx, &StaticProperties{}) {
			if p, ok := targetProps.Properties.(*StaticProperties); ok {
				allDeps = appendDeps(allDeps, p.Static)
			}
			for _, archProperties := range targetProps.ArchProperties {
				if p, ok := archProperties.(*StaticProperties); ok {
					allDeps = appendDeps(allDeps, p.Static)
				}
			}
		}
		// arch: { <arch>: shared: { ... } }
		for _, properties := range module.GetArchProperties(ctx, &SharedProperties{}) {
			if p, ok := properties.(*SharedProperties); ok {
				allDeps = appendDeps(allDeps, p.Shared)
			}
		}
		// arch: { <arch>: static: { ... } }
		for _, properties := range module.GetArchProperties(ctx, &StaticProperties{}) {
			if p, ok := properties.(*StaticProperties); ok {
				allDeps = appendDeps(allDeps, p.Static)
			}
		}
	}

	ctx.AddDependency(module, nil, android.SortedUniqueStrings(allDeps)...)
}

// staticOrSharedAttributes are the Bazel-ified versions of StaticOrSharedProperties --
// properties which apply to either the shared or static version of a cc_library module.
type staticOrSharedAttributes struct {
	copts            bazel.StringListAttribute
	srcs             bazel.LabelListAttribute
	staticDeps       bazel.LabelListAttribute
	dynamicDeps      bazel.LabelListAttribute
	wholeArchiveDeps bazel.LabelListAttribute
}

// bp2buildParseSharedProps returns the attributes for the shared variant of a cc_library.
func bp2BuildParseSharedProps(ctx android.TopDownMutatorContext, module *Module) staticOrSharedAttributes {
	lib, ok := module.compiler.(*libraryDecorator)
	if !ok {
		return staticOrSharedAttributes{}
	}

	return bp2buildParseStaticOrSharedProps(ctx, module, lib, false)
}

// bp2buildParseStaticProps returns the attributes for the static variant of a cc_library.
func bp2BuildParseStaticProps(ctx android.TopDownMutatorContext, module *Module) staticOrSharedAttributes {
	lib, ok := module.compiler.(*libraryDecorator)
	if !ok {
		return staticOrSharedAttributes{}
	}

	return bp2buildParseStaticOrSharedProps(ctx, module, lib, true)
}

func bp2buildParseStaticOrSharedProps(ctx android.TopDownMutatorContext, module *Module, lib *libraryDecorator, isStatic bool) staticOrSharedAttributes {
	var props StaticOrSharedProperties
	if isStatic {
		props = lib.StaticProperties.Static
	} else {
		props = lib.SharedProperties.Shared
	}

	attrs := staticOrSharedAttributes{
		copts:            bazel.StringListAttribute{Value: props.Cflags},
		srcs:             bazel.LabelListAttribute{Value: android.BazelLabelForModuleSrc(ctx, props.Srcs)},
		staticDeps:       bazel.LabelListAttribute{Value: android.BazelLabelForModuleDeps(ctx, props.Static_libs)},
		dynamicDeps:      bazel.LabelListAttribute{Value: android.BazelLabelForModuleDeps(ctx, props.Shared_libs)},
		wholeArchiveDeps: bazel.LabelListAttribute{Value: android.BazelLabelForModuleDeps(ctx, props.Whole_static_libs)},
	}

	setArchAttrs := func(arch string, props StaticOrSharedProperties) {
		attrs.copts.SetValueForArch(arch, props.Cflags)
		attrs.srcs.SetValueForArch(arch, android.BazelLabelForModuleSrc(ctx, props.Srcs))
		attrs.staticDeps.SetValueForArch(arch, android.BazelLabelForModuleDeps(ctx, props.Static_libs))
		attrs.dynamicDeps.SetValueForArch(arch, android.BazelLabelForModuleDeps(ctx, props.Shared_libs))
		attrs.wholeArchiveDeps.SetValueForArch(arch, android.BazelLabelForModuleDeps(ctx, props.Whole_static_libs))
	}

	setTargetAttrs := func(target string, props StaticOrSharedProperties) {
		attrs.copts.SetOsValueForTarget(target, props.Cflags)
		attrs.srcs.SetOsValueForTarget(target, android.BazelLabelForModuleSrc(ctx, props.Srcs))
		attrs.staticDeps.SetOsValueForTarget(target, android.BazelLabelForModuleDeps(ctx, props.Static_libs))
		attrs.dynamicDeps.SetOsValueForTarget(target, android.BazelLabelForModuleDeps(ctx, props.Shared_libs))
		attrs.wholeArchiveDeps.SetOsValueForTarget(target, android.BazelLabelForModuleDeps(ctx, props.Whole_static_libs))
	}

	setTargetArchAttrs := func(target, arch string, props StaticOrSharedProperties) {
		attrs.copts.SetOsArchValueForTarget(target, arch, props.Cflags)
		attrs.srcs.SetOsArchValueForTarget(target, arch, android.BazelLabelForModuleSrc(ctx, props.Srcs))
		attrs.staticDeps.SetOsArchValueForTarget(target, arch, android.BazelLabelForModuleDeps(ctx, props.Static_libs))
		attrs.dynamicDeps.SetOsArchValueForTarget(target, arch, android.BazelLabelForModuleDeps(ctx, props.Shared_libs))
		attrs.wholeArchiveDeps.SetOsArchValueForTarget(target, arch, android.BazelLabelForModuleDeps(ctx, props.Whole_static_libs))
	}

	if isStatic {
		for arch, properties := range module.GetArchProperties(ctx, &StaticProperties{}) {
			if staticOrSharedProps, ok := properties.(*StaticProperties); ok {
				setArchAttrs(arch.Name, staticOrSharedProps.Static)
			}
		}
		for target, p := range module.GetTargetProperties(ctx, &StaticProperties{}) {
			if staticOrSharedProps, ok := p.Properties.(*StaticProperties); ok {
				setTargetAttrs(target.Name, staticOrSharedProps.Static)
			}
			for arch, archProperties := range p.ArchProperties {
				if staticOrSharedProps, ok := archProperties.(*StaticProperties); ok {
					setTargetArchAttrs(target.Name, arch.Name, staticOrSharedProps.Static)
				}
			}
		}
	} else {
		for arch, p := range module.GetArchProperties(ctx, &SharedProperties{}) {
			if staticOrSharedProps, ok := p.(*SharedProperties); ok {
				setArchAttrs(arch.Name, staticOrSharedProps.Shared)
			}
		}
		for target, p := range module.GetTargetProperties(ctx, &SharedProperties{}) {
			if staticOrSharedProps, ok := p.Properties.(*SharedProperties); ok {
				setTargetAttrs(target.Name, staticOrSharedProps.Shared)
			}
			for arch, archProperties := range p.ArchProperties {
				if staticOrSharedProps, ok := archProperties.(*SharedProperties); ok {
					setTargetArchAttrs(target.Name, arch.Name, staticOrSharedProps.Shared)
				}
			}
		}
	}

	return attrs
}

// Convenience struct to hold all attributes parsed from compiler properties.
type compilerAttributes struct {
	// Options for all languages
	copts bazel.StringListAttribute
	// Assembly options and sources
	asFlags bazel.StringListAttribute
	asSrcs  bazel.LabelListAttribute
	// C options and sources
	conlyFlags bazel.StringListAttribute
	cSrcs      bazel.LabelListAttribute
	// C++ options and sources
	cppFlags bazel.StringListAttribute
	srcs     bazel.LabelListAttribute
}

// bp2BuildParseCompilerProps returns copts, srcs and hdrs and other attributes.
func bp2BuildParseCompilerProps(ctx android.TopDownMutatorContext, module *Module) compilerAttributes {
	var srcs bazel.LabelListAttribute
	var copts bazel.StringListAttribute
	var asFlags bazel.StringListAttribute
	var conlyFlags bazel.StringListAttribute
	var cppFlags bazel.StringListAttribute

	// Creates the -I flags for a directory, while making the directory relative
	// to the exec root for Bazel to work.
	includeFlags := func(dir string) []string {
		// filepath.Join canonicalizes the path, i.e. it takes care of . or .. elements.
		moduleDirRootedPath := filepath.Join(ctx.ModuleDir(), dir)
		return []string{
			"-I" + moduleDirRootedPath,
			// Include the bindir-rooted path (using make variable substitution). This most
			// closely matches Bazel's native include path handling, which allows for dependency
			// on generated headers in these directories.
			// TODO(b/188084383): Handle local include directories in Bazel.
			"-I$(BINDIR)/" + moduleDirRootedPath,
		}
	}

	// Parse the list of module-relative include directories (-I).
	parseLocalIncludeDirs := func(baseCompilerProps *BaseCompilerProperties) []string {
		// include_dirs are root-relative, not module-relative.
		includeDirs := bp2BuildMakePathsRelativeToModule(ctx, baseCompilerProps.Include_dirs)
		return append(includeDirs, baseCompilerProps.Local_include_dirs...)
	}

	parseCommandLineFlags := func(soongFlags []string) []string {
		var result []string
		for _, flag := range soongFlags {
			// Soong's cflags can contain spaces, like `-include header.h`. For
			// Bazel's copts, split them up to be compatible with the
			// no_copts_tokenization feature.
			result = append(result, strings.Split(flag, " ")...)
		}
		return result
	}

	// Parse the list of copts.
	parseCopts := func(baseCompilerProps *BaseCompilerProperties) []string {
		var copts []string
		copts = append(copts, parseCommandLineFlags(baseCompilerProps.Cflags)...)
		for _, dir := range parseLocalIncludeDirs(baseCompilerProps) {
			copts = append(copts, includeFlags(dir)...)
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
		// Add srcs-like dependencies such as generated files.
		// First create a LabelList containing these dependencies, then merge the values with srcs.
		generatedHdrsAndSrcs := baseCompilerProps.Generated_headers
		generatedHdrsAndSrcs = append(generatedHdrsAndSrcs, baseCompilerProps.Generated_sources...)

		generatedHdrsAndSrcsLabelList := android.BazelLabelForModuleDeps(ctx, generatedHdrsAndSrcs)

		// Combine the base exclude_srcs and configuration-specific exclude_srcs
		allExcludeSrcs := append(baseExcludeSrcs, baseCompilerProps.Exclude_srcs...)
		allSrcsLabelList := android.BazelLabelForModuleSrcExcludes(ctx, allSrcs, allExcludeSrcs)
		return bazel.AppendBazelLabelLists(allSrcsLabelList, generatedHdrsAndSrcsLabelList)
	}

	for _, props := range module.compiler.compilerProps() {
		if baseCompilerProps, ok := props.(*BaseCompilerProperties); ok {
			srcs.Value = parseSrcs(baseCompilerProps)
			copts.Value = parseCopts(baseCompilerProps)
			asFlags.Value = parseCommandLineFlags(baseCompilerProps.Asflags)
			conlyFlags.Value = parseCommandLineFlags(baseCompilerProps.Conlyflags)
			cppFlags.Value = parseCommandLineFlags(baseCompilerProps.Cppflags)

			// Used for arch-specific srcs later.
			baseSrcs = baseCompilerProps.Srcs
			baseSrcsLabelList = parseSrcs(baseCompilerProps)
			baseExcludeSrcs = baseCompilerProps.Exclude_srcs
			break
		}
	}

	// Handle include_build_directory prop. If the property is true, then the
	// target has access to all headers recursively in the package, and has
	// "-I<module-dir>" in its copts.
	if c, ok := module.compiler.(*baseCompiler); ok && c.includeBuildDirectory() {
		copts.Value = append(copts.Value, includeFlags(".")...)
	} else if c, ok := module.compiler.(*libraryDecorator); ok && c.includeBuildDirectory() {
		copts.Value = append(copts.Value, includeFlags(".")...)
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
			asFlags.SetValueForArch(arch.Name, parseCommandLineFlags(baseCompilerProps.Asflags))
			conlyFlags.SetValueForArch(arch.Name, parseCommandLineFlags(baseCompilerProps.Conlyflags))
			cppFlags.SetValueForArch(arch.Name, parseCommandLineFlags(baseCompilerProps.Cppflags))
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

	// Handle target specific properties.
	for os, osProps := range module.GetTargetProperties(ctx, &BaseCompilerProperties{}) {
		if baseCompilerProps, ok := osProps.Properties.(*BaseCompilerProperties); ok {
			srcsList := parseSrcs(baseCompilerProps)
			// TODO(b/186153868): add support for os-specific srcs and exclude_srcs
			if len(baseCompilerProps.Srcs) > 0 || len(baseCompilerProps.Exclude_srcs) > 0 {
				srcs.SetOsValueForTarget(os.Name, bazel.SubtractBazelLabelList(srcsList, baseSrcsLabelList))
			}
			copts.SetOsValueForTarget(os.Name, parseCopts(baseCompilerProps))
			asFlags.SetOsValueForTarget(os.Name, parseCommandLineFlags(baseCompilerProps.Asflags))
			conlyFlags.SetOsValueForTarget(os.Name, parseCommandLineFlags(baseCompilerProps.Conlyflags))
			cppFlags.SetOsValueForTarget(os.Name, parseCommandLineFlags(baseCompilerProps.Cppflags))
		}
		for arch, archProps := range osProps.ArchProperties {
			if baseCompilerProps, ok := archProps.(*BaseCompilerProperties); ok {
				srcsList := parseSrcs(baseCompilerProps)
				// TODO(b/186153868): add support for os-specific srcs and exclude_srcs
				if len(baseCompilerProps.Srcs) > 0 || len(baseCompilerProps.Exclude_srcs) > 0 {
					srcs.SetOsArchValueForTarget(os.Name, arch.Name, bazel.SubtractBazelLabelList(srcsList, baseSrcsLabelList))
				}
				copts.SetOsArchValueForTarget(os.Name, arch.Name, parseCopts(baseCompilerProps))
				asFlags.SetOsArchValueForTarget(os.Name, arch.Name, parseCommandLineFlags(baseCompilerProps.Asflags))
				conlyFlags.SetOsArchValueForTarget(os.Name, arch.Name, parseCommandLineFlags(baseCompilerProps.Conlyflags))
				cppFlags.SetOsArchValueForTarget(os.Name, arch.Name, parseCommandLineFlags(baseCompilerProps.Cppflags))
			}
		}
	}

	productVarPropNameToAttribute := map[string]*bazel.StringListAttribute{
		"Cflags":   &copts,
		"Asflags":  &asFlags,
		"CppFlags": &cppFlags,
	}
	productVariableProps := android.ProductVariableProperties(ctx)
	for propName, attr := range productVarPropNameToAttribute {
		if props, exists := productVariableProps[propName]; exists {
			for _, prop := range props {
				flags, ok := prop.Property.([]string)
				if !ok {
					ctx.ModuleErrorf("Could not convert product variable %s property", proptools.PropertyNameForField(propName))
				}
				newFlags, _ := bazel.TryVariableSubstitutions(flags, prop.ProductConfigVariable)
				attr.ProductValues = append(attr.ProductValues, bazel.ProductVariableValues{
					ProductVariable: prop.ProductConfigVariable,
					Values:          newFlags,
				})
			}
		}
	}

	// Branch srcs into three language-specific groups.
	// C++ is the "catch-all" group, and comprises generated sources because we don't
	// know the language of these sources until the genrule is executed.
	// TODO(b/): Handle language detection of sources in a Bazel rule.
	isCSrc := func(s string) bool {
		return strings.HasSuffix(s, ".c")
	}
	isAsmSrc := func(s string) bool {
		return strings.HasSuffix(s, ".S") || strings.HasSuffix(s, ".s")
	}
	cSrcs := bazel.FilterLabelListAttribute(srcs, isCSrc)
	asSrcs := bazel.FilterLabelListAttribute(srcs, isAsmSrc)
	srcs = bazel.SubtractBazelLabelListAttribute(srcs, cSrcs)
	srcs = bazel.SubtractBazelLabelListAttribute(srcs, asSrcs)
	return compilerAttributes{
		copts:      copts,
		srcs:       srcs,
		asFlags:    asFlags,
		asSrcs:     asSrcs,
		cSrcs:      cSrcs,
		conlyFlags: conlyFlags,
		cppFlags:   cppFlags,
	}
}

// Convenience struct to hold all attributes parsed from linker properties.
type linkerAttributes struct {
	deps             bazel.LabelListAttribute
	dynamicDeps      bazel.LabelListAttribute
	wholeArchiveDeps bazel.LabelListAttribute
	exportedDeps     bazel.LabelListAttribute
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
	var exportedDeps bazel.LabelListAttribute
	var dynamicDeps bazel.LabelListAttribute
	var wholeArchiveDeps bazel.LabelListAttribute
	var linkopts bazel.StringListAttribute
	var versionScript bazel.LabelAttribute

	getLibs := func(baseLinkerProps *BaseLinkerProperties) []string {
		libs := baseLinkerProps.Header_libs
		libs = append(libs, baseLinkerProps.Static_libs...)
		libs = android.SortedUniqueStrings(libs)
		return libs
	}

	for _, linkerProps := range module.linker.linkerProps() {
		if baseLinkerProps, ok := linkerProps.(*BaseLinkerProperties); ok {
			libs := getLibs(baseLinkerProps)
			exportedLibs := baseLinkerProps.Export_header_lib_headers
			wholeArchiveLibs := baseLinkerProps.Whole_static_libs
			deps = bazel.MakeLabelListAttribute(android.BazelLabelForModuleDeps(ctx, libs))
			exportedDeps = bazel.MakeLabelListAttribute(android.BazelLabelForModuleDeps(ctx, exportedLibs))
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

	for arch, props := range module.GetArchProperties(ctx, &BaseLinkerProperties{}) {
		if baseLinkerProps, ok := props.(*BaseLinkerProperties); ok {
			libs := getLibs(baseLinkerProps)
			exportedLibs := baseLinkerProps.Export_header_lib_headers
			wholeArchiveLibs := baseLinkerProps.Whole_static_libs
			deps.SetValueForArch(arch.Name, android.BazelLabelForModuleDeps(ctx, libs))
			exportedDeps.SetValueForArch(arch.Name, android.BazelLabelForModuleDeps(ctx, exportedLibs))
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

	for os, targetProperties := range module.GetTargetProperties(ctx, &BaseLinkerProperties{}) {
		if baseLinkerProps, ok := targetProperties.Properties.(*BaseLinkerProperties); ok {
			libs := getLibs(baseLinkerProps)
			exportedLibs := baseLinkerProps.Export_header_lib_headers
			wholeArchiveLibs := baseLinkerProps.Whole_static_libs
			wholeArchiveDeps.SetOsValueForTarget(os.Name, android.BazelLabelForModuleDeps(ctx, wholeArchiveLibs))
			deps.SetOsValueForTarget(os.Name, android.BazelLabelForModuleDeps(ctx, libs))
			exportedDeps.SetOsValueForTarget(os.Name, android.BazelLabelForModuleDeps(ctx, exportedLibs))

			linkopts.SetOsValueForTarget(os.Name, getBp2BuildLinkerFlags(baseLinkerProps))

			if baseLinkerProps.Version_script != nil {
				versionScript.SetOsValueForTarget(os.Name, android.BazelLabelForModuleSrcSingle(ctx, *baseLinkerProps.Version_script))
			}

			sharedLibs := baseLinkerProps.Shared_libs
			dynamicDeps.SetOsValueForTarget(os.Name, android.BazelLabelForModuleDeps(ctx, sharedLibs))
		}
		for arch, archProperties := range targetProperties.ArchProperties {
			if baseLinkerProps, ok := archProperties.(*BaseLinkerProperties); ok {
				libs := getLibs(baseLinkerProps)
				exportedLibs := baseLinkerProps.Export_header_lib_headers
				wholeArchiveLibs := baseLinkerProps.Whole_static_libs
				wholeArchiveDeps.SetOsArchValueForTarget(os.Name, arch.Name, android.BazelLabelForModuleDeps(ctx, wholeArchiveLibs))
				deps.SetOsArchValueForTarget(os.Name, arch.Name, android.BazelLabelForModuleDeps(ctx, libs))
				exportedDeps.SetOsArchValueForTarget(os.Name, arch.Name, android.BazelLabelForModuleDeps(ctx, exportedLibs))

				linkopts.SetOsArchValueForTarget(os.Name, arch.Name, getBp2BuildLinkerFlags(baseLinkerProps))

				if baseLinkerProps.Version_script != nil {
					versionScript.SetOsArchValueForTarget(os.Name, arch.Name, android.BazelLabelForModuleSrcSingle(ctx, *baseLinkerProps.Version_script))
				}

				sharedLibs := baseLinkerProps.Shared_libs
				dynamicDeps.SetOsArchValueForTarget(os.Name, arch.Name, android.BazelLabelForModuleDeps(ctx, sharedLibs))
			}
		}
	}

	return linkerAttributes{
		deps:             deps,
		exportedDeps:     exportedDeps,
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

	getVariantIncludeDirs := func(includeDirs []string, flagExporterProperties *FlagExporterProperties) []string {
		variantIncludeDirs := flagExporterProperties.Export_system_include_dirs
		variantIncludeDirs = append(variantIncludeDirs, flagExporterProperties.Export_include_dirs...)

		// To avoid duplicate includes when base includes + arch includes are combined
		// TODO: This doesn't take conflicts between arch and os includes into account
		variantIncludeDirs = bazel.SubtractStrings(variantIncludeDirs, includeDirs)
		return variantIncludeDirs
	}

	for arch, props := range module.GetArchProperties(ctx, &FlagExporterProperties{}) {
		if flagExporterProperties, ok := props.(*FlagExporterProperties); ok {
			archIncludeDirs := getVariantIncludeDirs(includeDirs, flagExporterProperties)
			if len(archIncludeDirs) > 0 {
				includeDirsAttribute.SetValueForArch(arch.Name, archIncludeDirs)
			}
		}
	}

	for os, targetProperties := range module.GetTargetProperties(ctx, &FlagExporterProperties{}) {
		if flagExporterProperties, ok := targetProperties.Properties.(*FlagExporterProperties); ok {
			targetIncludeDirs := getVariantIncludeDirs(includeDirs, flagExporterProperties)
			if len(targetIncludeDirs) > 0 {
				includeDirsAttribute.SetOsValueForTarget(os.Name, targetIncludeDirs)
			}
		}
		for arch, archProperties := range targetProperties.ArchProperties {
			if flagExporterProperties, ok := archProperties.(*FlagExporterProperties); ok {
				targetIncludeDirs := getVariantIncludeDirs(includeDirs, flagExporterProperties)
				if len(targetIncludeDirs) > 0 {
					includeDirsAttribute.SetOsArchValueForTarget(os.Name, arch.Name, targetIncludeDirs)
				}
			}
		}
	}

	return includeDirsAttribute
}
