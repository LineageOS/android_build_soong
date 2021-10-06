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

	"github.com/google/blueprint"

	"github.com/google/blueprint/proptools"
)

// staticOrSharedAttributes are the Bazel-ified versions of StaticOrSharedProperties --
// properties which apply to either the shared or static version of a cc_library module.
type staticOrSharedAttributes struct {
	Srcs    bazel.LabelListAttribute
	Srcs_c  bazel.LabelListAttribute
	Srcs_as bazel.LabelListAttribute
	Copts   bazel.StringListAttribute

	Deps                        bazel.LabelListAttribute
	Implementation_deps         bazel.LabelListAttribute
	Dynamic_deps                bazel.LabelListAttribute
	Implementation_dynamic_deps bazel.LabelListAttribute
	Whole_archive_deps          bazel.LabelListAttribute

	System_dynamic_deps bazel.LabelListAttribute
}

func groupSrcsByExtension(ctx android.TopDownMutatorContext, srcs bazel.LabelListAttribute) (cppSrcs, cSrcs, asSrcs bazel.LabelListAttribute) {
	// Check that a module is a filegroup type named <label>.
	isFilegroupNamed := func(m android.Module, fullLabel string) bool {
		if ctx.OtherModuleType(m) != "filegroup" {
			return false
		}
		labelParts := strings.Split(fullLabel, ":")
		if len(labelParts) > 2 {
			// There should not be more than one colon in a label.
			ctx.ModuleErrorf("%s is not a valid Bazel label for a filegroup", fullLabel)
		}
		return m.Name() == labelParts[len(labelParts)-1]
	}

	// Convert filegroup dependencies into extension-specific filegroups filtered in the filegroup.bzl
	// macro.
	addSuffixForFilegroup := func(suffix string) bazel.LabelMapper {
		return func(ctx bazel.OtherModuleContext, label string) (string, bool) {
			m, exists := ctx.ModuleFromName(label)
			if !exists {
				return label, false
			}
			aModule, _ := m.(android.Module)
			if !isFilegroupNamed(aModule, label) {
				return label, false
			}
			return label + suffix, true
		}
	}

	// TODO(b/190006308): Handle language detection of sources in a Bazel rule.
	partitioned := bazel.PartitionLabelListAttribute(ctx, &srcs, bazel.LabelPartitions{
		"c":  bazel.LabelPartition{Extensions: []string{".c"}, LabelMapper: addSuffixForFilegroup("_c_srcs")},
		"as": bazel.LabelPartition{Extensions: []string{".s", ".S"}, LabelMapper: addSuffixForFilegroup("_as_srcs")},
		// C++ is the "catch-all" group, and comprises generated sources because we don't
		// know the language of these sources until the genrule is executed.
		"cpp": bazel.LabelPartition{Extensions: []string{".cpp", ".cc", ".cxx", ".mm"}, LabelMapper: addSuffixForFilegroup("_cpp_srcs"), Keep_remainder: true},
	})

	cSrcs = partitioned["c"]
	asSrcs = partitioned["as"]
	cppSrcs = partitioned["cpp"]
	return
}

// bp2BuildParseLibProps returns the attributes for a variant of a cc_library.
func bp2BuildParseLibProps(ctx android.TopDownMutatorContext, module *Module, isStatic bool) staticOrSharedAttributes {
	lib, ok := module.compiler.(*libraryDecorator)
	if !ok {
		return staticOrSharedAttributes{}
	}
	return bp2buildParseStaticOrSharedProps(ctx, module, lib, isStatic)
}

// bp2buildParseSharedProps returns the attributes for the shared variant of a cc_library.
func bp2BuildParseSharedProps(ctx android.TopDownMutatorContext, module *Module) staticOrSharedAttributes {
	return bp2BuildParseLibProps(ctx, module, false)
}

// bp2buildParseStaticProps returns the attributes for the static variant of a cc_library.
func bp2BuildParseStaticProps(ctx android.TopDownMutatorContext, module *Module) staticOrSharedAttributes {
	return bp2BuildParseLibProps(ctx, module, true)
}

type depsPartition struct {
	export         bazel.LabelList
	implementation bazel.LabelList
}

type bazelLabelForDepsFn func(android.TopDownMutatorContext, []string) bazel.LabelList

func partitionExportedAndImplementationsDeps(ctx android.TopDownMutatorContext, allDeps, exportedDeps []string, fn bazelLabelForDepsFn) depsPartition {
	implementation, export := android.FilterList(allDeps, exportedDeps)

	return depsPartition{
		export:         fn(ctx, export),
		implementation: fn(ctx, implementation),
	}
}

type bazelLabelForDepsExcludesFn func(android.TopDownMutatorContext, []string, []string) bazel.LabelList

func partitionExportedAndImplementationsDepsExcludes(ctx android.TopDownMutatorContext, allDeps, excludes, exportedDeps []string, fn bazelLabelForDepsExcludesFn) depsPartition {
	implementation, export := android.FilterList(allDeps, exportedDeps)

	return depsPartition{
		export:         fn(ctx, export, excludes),
		implementation: fn(ctx, implementation, excludes),
	}
}

func bp2buildParseStaticOrSharedProps(ctx android.TopDownMutatorContext, module *Module, lib *libraryDecorator, isStatic bool) staticOrSharedAttributes {
	attrs := staticOrSharedAttributes{}

	setAttrs := func(axis bazel.ConfigurationAxis, config string, props StaticOrSharedProperties) {
		attrs.Copts.SetSelectValue(axis, config, props.Cflags)
		attrs.Srcs.SetSelectValue(axis, config, android.BazelLabelForModuleSrc(ctx, props.Srcs))
		attrs.System_dynamic_deps.SetSelectValue(axis, config, bazelLabelForSharedDeps(ctx, props.System_shared_libs))

		staticDeps := partitionExportedAndImplementationsDeps(ctx, props.Static_libs, props.Export_static_lib_headers, bazelLabelForStaticDeps)
		attrs.Deps.SetSelectValue(axis, config, staticDeps.export)
		attrs.Implementation_deps.SetSelectValue(axis, config, staticDeps.implementation)

		sharedDeps := partitionExportedAndImplementationsDeps(ctx, props.Shared_libs, props.Export_shared_lib_headers, bazelLabelForSharedDeps)
		attrs.Dynamic_deps.SetSelectValue(axis, config, sharedDeps.export)
		attrs.Implementation_dynamic_deps.SetSelectValue(axis, config, sharedDeps.implementation)

		attrs.Whole_archive_deps.SetSelectValue(axis, config, bazelLabelForWholeDeps(ctx, props.Whole_static_libs))
	}
	// system_dynamic_deps distinguishes between nil/empty list behavior:
	//    nil -> use default values
	//    empty list -> no values specified
	attrs.System_dynamic_deps.ForceSpecifyEmptyList = true

	if isStatic {
		for axis, configToProps := range module.GetArchVariantProperties(ctx, &StaticProperties{}) {
			for config, props := range configToProps {
				if staticOrSharedProps, ok := props.(*StaticProperties); ok {
					setAttrs(axis, config, staticOrSharedProps.Static)
				}
			}
		}
	} else {
		for axis, configToProps := range module.GetArchVariantProperties(ctx, &SharedProperties{}) {
			for config, props := range configToProps {
				if staticOrSharedProps, ok := props.(*SharedProperties); ok {
					setAttrs(axis, config, staticOrSharedProps.Shared)
				}
			}
		}
	}

	cppSrcs, cSrcs, asSrcs := groupSrcsByExtension(ctx, attrs.Srcs)
	attrs.Srcs = cppSrcs
	attrs.Srcs_c = cSrcs
	attrs.Srcs_as = asSrcs

	return attrs
}

// Convenience struct to hold all attributes parsed from prebuilt properties.
type prebuiltAttributes struct {
	Src bazel.LabelAttribute
}

// NOTE: Used outside of Soong repo project, in the clangprebuilts.go bootstrap_go_package
func Bp2BuildParsePrebuiltLibraryProps(ctx android.TopDownMutatorContext, module *Module) prebuiltAttributes {
	var srcLabelAttribute bazel.LabelAttribute

	for axis, configToProps := range module.GetArchVariantProperties(ctx, &prebuiltLinkerProperties{}) {
		for config, props := range configToProps {
			if prebuiltLinkerProperties, ok := props.(*prebuiltLinkerProperties); ok {
				if len(prebuiltLinkerProperties.Srcs) > 1 {
					ctx.ModuleErrorf("Bp2BuildParsePrebuiltLibraryProps: Expected at most once source file for %s %s\n", axis, config)
					continue
				} else if len(prebuiltLinkerProperties.Srcs) == 0 {
					continue
				}
				src := android.BazelLabelForModuleSrcSingle(ctx, prebuiltLinkerProperties.Srcs[0])
				srcLabelAttribute.SetSelectValue(axis, config, src)
			}
		}
	}

	return prebuiltAttributes{
		Src: srcLabelAttribute,
	}
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

	rtti bazel.BoolAttribute
	stl  *string

	localIncludes    bazel.StringListAttribute
	absoluteIncludes bazel.StringListAttribute
}

// bp2BuildParseCompilerProps returns copts, srcs and hdrs and other attributes.
func bp2BuildParseCompilerProps(ctx android.TopDownMutatorContext, module *Module) compilerAttributes {
	var srcs bazel.LabelListAttribute
	var copts bazel.StringListAttribute
	var asFlags bazel.StringListAttribute
	var conlyFlags bazel.StringListAttribute
	var cppFlags bazel.StringListAttribute
	var rtti bazel.BoolAttribute
	var localIncludes bazel.StringListAttribute
	var absoluteIncludes bazel.StringListAttribute

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

	// Parse srcs from an arch or OS's props value.
	parseSrcs := func(baseCompilerProps *BaseCompilerProperties) bazel.LabelList {
		// Add srcs-like dependencies such as generated files.
		// First create a LabelList containing these dependencies, then merge the values with srcs.
		generatedHdrsAndSrcs := baseCompilerProps.Generated_headers
		generatedHdrsAndSrcs = append(generatedHdrsAndSrcs, baseCompilerProps.Generated_sources...)
		generatedHdrsAndSrcsLabelList := android.BazelLabelForModuleDeps(ctx, generatedHdrsAndSrcs)

		allSrcsLabelList := android.BazelLabelForModuleSrcExcludes(ctx, baseCompilerProps.Srcs, baseCompilerProps.Exclude_srcs)
		return bazel.AppendBazelLabelLists(allSrcsLabelList, generatedHdrsAndSrcsLabelList)
	}

	archVariantCompilerProps := module.GetArchVariantProperties(ctx, &BaseCompilerProperties{})
	for axis, configToProps := range archVariantCompilerProps {
		for config, props := range configToProps {
			if baseCompilerProps, ok := props.(*BaseCompilerProperties); ok {
				// If there's arch specific srcs or exclude_srcs, generate a select entry for it.
				// TODO(b/186153868): do this for OS specific srcs and exclude_srcs too.
				if len(baseCompilerProps.Srcs) > 0 || len(baseCompilerProps.Exclude_srcs) > 0 {
					srcsList := parseSrcs(baseCompilerProps)
					srcs.SetSelectValue(axis, config, srcsList)
				}

				archVariantCopts := parseCommandLineFlags(baseCompilerProps.Cflags)
				archVariantAsflags := parseCommandLineFlags(baseCompilerProps.Asflags)

				localIncludeDirs := baseCompilerProps.Local_include_dirs
				if axis == bazel.NoConfigAxis && includeBuildDirectory(baseCompilerProps.Include_build_directory) {
					localIncludeDirs = append(localIncludeDirs, ".")
				}

				absoluteIncludes.SetSelectValue(axis, config, baseCompilerProps.Include_dirs)
				localIncludes.SetSelectValue(axis, config, localIncludeDirs)

				copts.SetSelectValue(axis, config, archVariantCopts)
				asFlags.SetSelectValue(axis, config, archVariantAsflags)
				conlyFlags.SetSelectValue(axis, config, parseCommandLineFlags(baseCompilerProps.Conlyflags))
				cppFlags.SetSelectValue(axis, config, parseCommandLineFlags(baseCompilerProps.Cppflags))
				rtti.SetSelectValue(axis, config, baseCompilerProps.Rtti)
			}
		}
	}

	srcs.ResolveExcludes()
	absoluteIncludes.DeduplicateAxesFromBase()
	localIncludes.DeduplicateAxesFromBase()

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
				attr.SetSelectValue(bazel.ProductVariableConfigurationAxis(prop.FullConfig), prop.FullConfig, newFlags)
			}
		}
	}

	srcs, cSrcs, asSrcs := groupSrcsByExtension(ctx, srcs)

	var stl *string = nil
	stlPropsByArch := module.GetArchVariantProperties(ctx, &StlProperties{})
	for _, configToProps := range stlPropsByArch {
		for _, props := range configToProps {
			if stlProps, ok := props.(*StlProperties); ok {
				if stlProps.Stl != nil {
					if stl == nil {
						stl = stlProps.Stl
					} else {
						if stl != stlProps.Stl {
							ctx.ModuleErrorf("Unsupported conversion: module with different stl for different variants: %s and %s", *stl, stlProps.Stl)
						}
					}
				}
			}
		}
	}

	return compilerAttributes{
		copts:            copts,
		srcs:             srcs,
		asFlags:          asFlags,
		asSrcs:           asSrcs,
		cSrcs:            cSrcs,
		conlyFlags:       conlyFlags,
		cppFlags:         cppFlags,
		rtti:             rtti,
		stl:              stl,
		localIncludes:    localIncludes,
		absoluteIncludes: absoluteIncludes,
	}
}

// Convenience struct to hold all attributes parsed from linker properties.
type linkerAttributes struct {
	deps                      bazel.LabelListAttribute
	implementationDeps        bazel.LabelListAttribute
	dynamicDeps               bazel.LabelListAttribute
	implementationDynamicDeps bazel.LabelListAttribute
	wholeArchiveDeps          bazel.LabelListAttribute
	systemDynamicDeps         bazel.LabelListAttribute

	linkCrt                       bazel.BoolAttribute
	useLibcrt                     bazel.BoolAttribute
	linkopts                      bazel.StringListAttribute
	versionScript                 bazel.LabelAttribute
	stripKeepSymbols              bazel.BoolAttribute
	stripKeepSymbolsAndDebugFrame bazel.BoolAttribute
	stripKeepSymbolsList          bazel.StringListAttribute
	stripAll                      bazel.BoolAttribute
	stripNone                     bazel.BoolAttribute
	features                      bazel.StringListAttribute
}

// bp2BuildParseLinkerProps parses the linker properties of a module, including
// configurable attribute values.
func bp2BuildParseLinkerProps(ctx android.TopDownMutatorContext, module *Module) linkerAttributes {

	var headerDeps bazel.LabelListAttribute
	var implementationHeaderDeps bazel.LabelListAttribute
	var deps bazel.LabelListAttribute
	var implementationDeps bazel.LabelListAttribute
	var dynamicDeps bazel.LabelListAttribute
	var implementationDynamicDeps bazel.LabelListAttribute
	var wholeArchiveDeps bazel.LabelListAttribute
	systemSharedDeps := bazel.LabelListAttribute{ForceSpecifyEmptyList: true}

	var linkopts bazel.StringListAttribute
	var versionScript bazel.LabelAttribute
	var linkCrt bazel.BoolAttribute
	var useLibcrt bazel.BoolAttribute

	var stripKeepSymbols bazel.BoolAttribute
	var stripKeepSymbolsAndDebugFrame bazel.BoolAttribute
	var stripKeepSymbolsList bazel.StringListAttribute
	var stripAll bazel.BoolAttribute
	var stripNone bazel.BoolAttribute

	var features bazel.StringListAttribute

	for axis, configToProps := range module.GetArchVariantProperties(ctx, &StripProperties{}) {
		for config, props := range configToProps {
			if stripProperties, ok := props.(*StripProperties); ok {
				stripKeepSymbols.SetSelectValue(axis, config, stripProperties.Strip.Keep_symbols)
				stripKeepSymbolsList.SetSelectValue(axis, config, stripProperties.Strip.Keep_symbols_list)
				stripKeepSymbolsAndDebugFrame.SetSelectValue(axis, config, stripProperties.Strip.Keep_symbols_and_debug_frame)
				stripAll.SetSelectValue(axis, config, stripProperties.Strip.All)
				stripNone.SetSelectValue(axis, config, stripProperties.Strip.None)
			}
		}
	}

	// Use a single variable to capture usage of nocrt in arch variants, so there's only 1 error message for this module
	var disallowedArchVariantCrt bool

	for axis, configToProps := range module.GetArchVariantProperties(ctx, &BaseLinkerProperties{}) {
		for config, props := range configToProps {
			if baseLinkerProps, ok := props.(*BaseLinkerProperties); ok {
				var axisFeatures []string

				// Excludes to parallel Soong:
				// https://cs.android.com/android/platform/superproject/+/master:build/soong/cc/linker.go;l=247-249;drc=088b53577dde6e40085ffd737a1ae96ad82fc4b0
				staticLibs := android.FirstUniqueStrings(baseLinkerProps.Static_libs)
				staticDeps := partitionExportedAndImplementationsDepsExcludes(ctx, staticLibs, baseLinkerProps.Exclude_static_libs, baseLinkerProps.Export_static_lib_headers, bazelLabelForStaticDepsExcludes)
				deps.SetSelectValue(axis, config, staticDeps.export)
				implementationDeps.SetSelectValue(axis, config, staticDeps.implementation)

				wholeStaticLibs := android.FirstUniqueStrings(baseLinkerProps.Whole_static_libs)
				wholeArchiveDeps.SetSelectValue(axis, config, bazelLabelForWholeDepsExcludes(ctx, wholeStaticLibs, baseLinkerProps.Exclude_static_libs))

				systemSharedLibs := baseLinkerProps.System_shared_libs
				// systemSharedLibs distinguishes between nil/empty list behavior:
				//    nil -> use default values
				//    empty list -> no values specified
				if len(systemSharedLibs) > 0 {
					systemSharedLibs = android.FirstUniqueStrings(systemSharedLibs)
				}
				systemSharedDeps.SetSelectValue(axis, config, bazelLabelForSharedDeps(ctx, systemSharedLibs))

				sharedLibs := android.FirstUniqueStrings(baseLinkerProps.Shared_libs)
				sharedDeps := partitionExportedAndImplementationsDepsExcludes(ctx, sharedLibs, baseLinkerProps.Exclude_shared_libs, baseLinkerProps.Export_shared_lib_headers, bazelLabelForSharedDepsExcludes)
				dynamicDeps.SetSelectValue(axis, config, sharedDeps.export)
				implementationDynamicDeps.SetSelectValue(axis, config, sharedDeps.implementation)

				headerLibs := android.FirstUniqueStrings(baseLinkerProps.Header_libs)
				hDeps := partitionExportedAndImplementationsDeps(ctx, headerLibs, baseLinkerProps.Export_header_lib_headers, bazelLabelForHeaderDeps)

				headerDeps.SetSelectValue(axis, config, hDeps.export)
				implementationHeaderDeps.SetSelectValue(axis, config, hDeps.implementation)

				linkopts.SetSelectValue(axis, config, baseLinkerProps.Ldflags)
				if !BoolDefault(baseLinkerProps.Pack_relocations, packRelocationsDefault) {
					axisFeatures = append(axisFeatures, "disable_pack_relocations")
				}

				if Bool(baseLinkerProps.Allow_undefined_symbols) {
					axisFeatures = append(axisFeatures, "-no_undefined_symbols")
				}

				if baseLinkerProps.Version_script != nil {
					versionScript.SetSelectValue(axis, config, android.BazelLabelForModuleSrcSingle(ctx, *baseLinkerProps.Version_script))
				}
				useLibcrt.SetSelectValue(axis, config, baseLinkerProps.libCrt())

				// it's very unlikely for nocrt to be arch variant, so bp2build doesn't support it.
				if baseLinkerProps.crt() != nil {
					if axis == bazel.NoConfigAxis {
						linkCrt.SetSelectValue(axis, config, baseLinkerProps.crt())
					} else if axis == bazel.ArchConfigurationAxis {
						disallowedArchVariantCrt = true
					}
				}

				if axisFeatures != nil {
					features.SetSelectValue(axis, config, axisFeatures)
				}
			}
		}
	}

	if disallowedArchVariantCrt {
		ctx.ModuleErrorf("nocrt is not supported for arch variants")
	}

	type productVarDep struct {
		// the name of the corresponding excludes field, if one exists
		excludesField string
		// reference to the bazel attribute that should be set for the given product variable config
		attribute *bazel.LabelListAttribute

		depResolutionFunc func(ctx android.TopDownMutatorContext, modules, excludes []string) bazel.LabelList
	}

	productVarToDepFields := map[string]productVarDep{
		// product variables do not support exclude_shared_libs
		"Shared_libs":       productVarDep{attribute: &implementationDynamicDeps, depResolutionFunc: bazelLabelForSharedDepsExcludes},
		"Static_libs":       productVarDep{"Exclude_static_libs", &implementationDeps, bazelLabelForStaticDepsExcludes},
		"Whole_static_libs": productVarDep{"Exclude_static_libs", &wholeArchiveDeps, bazelLabelForWholeDepsExcludes},
	}

	productVariableProps := android.ProductVariableProperties(ctx)
	for name, dep := range productVarToDepFields {
		props, exists := productVariableProps[name]
		excludeProps, excludesExists := productVariableProps[dep.excludesField]
		// if neither an include or excludes property exists, then skip it
		if !exists && !excludesExists {
			continue
		}
		// collect all the configurations that an include or exclude property exists for.
		// we want to iterate all configurations rather than either the include or exclude because for a
		// particular configuration we may have only and include or only an exclude to handle
		configs := make(map[string]bool, len(props)+len(excludeProps))
		for config := range props {
			configs[config] = true
		}
		for config := range excludeProps {
			configs[config] = true
		}

		for config := range configs {
			prop, includesExists := props[config]
			excludesProp, excludesExists := excludeProps[config]
			var includes, excludes []string
			var ok bool
			// if there was no includes/excludes property, casting fails and that's expected
			if includes, ok = prop.Property.([]string); includesExists && !ok {
				ctx.ModuleErrorf("Could not convert product variable %s property", name)
			}
			if excludes, ok = excludesProp.Property.([]string); excludesExists && !ok {
				ctx.ModuleErrorf("Could not convert product variable %s property", dep.excludesField)
			}

			dep.attribute.SetSelectValue(bazel.ProductVariableConfigurationAxis(config), config, dep.depResolutionFunc(ctx, android.FirstUniqueStrings(includes), excludes))
		}
	}

	headerDeps.Append(deps)
	implementationHeaderDeps.Append(implementationDeps)

	headerDeps.ResolveExcludes()
	implementationHeaderDeps.ResolveExcludes()
	dynamicDeps.ResolveExcludes()
	implementationDynamicDeps.ResolveExcludes()
	wholeArchiveDeps.ResolveExcludes()

	return linkerAttributes{
		deps:                      headerDeps,
		implementationDeps:        implementationHeaderDeps,
		dynamicDeps:               dynamicDeps,
		implementationDynamicDeps: implementationDynamicDeps,
		wholeArchiveDeps:          wholeArchiveDeps,
		systemDynamicDeps:         systemSharedDeps,

		linkCrt:       linkCrt,
		linkopts:      linkopts,
		useLibcrt:     useLibcrt,
		versionScript: versionScript,

		// Strip properties
		stripKeepSymbols:              stripKeepSymbols,
		stripKeepSymbolsAndDebugFrame: stripKeepSymbolsAndDebugFrame,
		stripKeepSymbolsList:          stripKeepSymbolsList,
		stripAll:                      stripAll,
		stripNone:                     stripNone,

		features: features,
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

// BazelIncludes contains information about -I and -isystem paths from a module converted to Bazel
// attributes.
type BazelIncludes struct {
	Includes       bazel.StringListAttribute
	SystemIncludes bazel.StringListAttribute
}

func bp2BuildParseExportedIncludes(ctx android.TopDownMutatorContext, module *Module) BazelIncludes {
	libraryDecorator := module.linker.(*libraryDecorator)
	return bp2BuildParseExportedIncludesHelper(ctx, module, libraryDecorator)
}

// Bp2buildParseExportedIncludesForPrebuiltLibrary returns a BazelIncludes with Bazel-ified values
// to export includes from the underlying module's properties.
func Bp2BuildParseExportedIncludesForPrebuiltLibrary(ctx android.TopDownMutatorContext, module *Module) BazelIncludes {
	prebuiltLibraryLinker := module.linker.(*prebuiltLibraryLinker)
	libraryDecorator := prebuiltLibraryLinker.libraryDecorator
	return bp2BuildParseExportedIncludesHelper(ctx, module, libraryDecorator)
}

// bp2BuildParseExportedIncludes creates a string list attribute contains the
// exported included directories of a module.
func bp2BuildParseExportedIncludesHelper(ctx android.TopDownMutatorContext, module *Module, libraryDecorator *libraryDecorator) BazelIncludes {
	exported := BazelIncludes{}
	for axis, configToProps := range module.GetArchVariantProperties(ctx, &FlagExporterProperties{}) {
		for config, props := range configToProps {
			if flagExporterProperties, ok := props.(*FlagExporterProperties); ok {
				if len(flagExporterProperties.Export_include_dirs) > 0 {
					exported.Includes.SetSelectValue(axis, config, flagExporterProperties.Export_include_dirs)
				}
				if len(flagExporterProperties.Export_system_include_dirs) > 0 {
					exported.SystemIncludes.SetSelectValue(axis, config, flagExporterProperties.Export_system_include_dirs)
				}
			}
		}
	}
	exported.Includes.DeduplicateAxesFromBase()
	exported.SystemIncludes.DeduplicateAxesFromBase()

	return exported
}

func bazelLabelForStaticModule(ctx android.TopDownMutatorContext, m blueprint.Module) string {
	label := android.BazelModuleLabel(ctx, m)
	if aModule, ok := m.(android.Module); ok {
		if ctx.OtherModuleType(aModule) == "cc_library" && !android.GenerateCcLibraryStaticOnly(m.Name()) {
			label += "_bp2build_cc_library_static"
		}
	}
	return label
}

func bazelLabelForSharedModule(ctx android.TopDownMutatorContext, m blueprint.Module) string {
	// cc_library, at it's root name, propagates the shared library, which depends on the static
	// library.
	return android.BazelModuleLabel(ctx, m)
}

func bazelLabelForStaticWholeModuleDeps(ctx android.TopDownMutatorContext, m blueprint.Module) string {
	label := bazelLabelForStaticModule(ctx, m)
	if aModule, ok := m.(android.Module); ok {
		if android.IsModulePrebuilt(aModule) {
			label += "_alwayslink"
		}
	}
	return label
}

func bazelLabelForWholeDeps(ctx android.TopDownMutatorContext, modules []string) bazel.LabelList {
	return android.BazelLabelForModuleDepsWithFn(ctx, modules, bazelLabelForStaticWholeModuleDeps)
}

func bazelLabelForWholeDepsExcludes(ctx android.TopDownMutatorContext, modules, excludes []string) bazel.LabelList {
	return android.BazelLabelForModuleDepsExcludesWithFn(ctx, modules, excludes, bazelLabelForStaticWholeModuleDeps)
}

func bazelLabelForStaticDepsExcludes(ctx android.TopDownMutatorContext, modules, excludes []string) bazel.LabelList {
	return android.BazelLabelForModuleDepsExcludesWithFn(ctx, modules, excludes, bazelLabelForStaticModule)
}

func bazelLabelForStaticDeps(ctx android.TopDownMutatorContext, modules []string) bazel.LabelList {
	return android.BazelLabelForModuleDepsWithFn(ctx, modules, bazelLabelForStaticModule)
}

func bazelLabelForSharedDeps(ctx android.TopDownMutatorContext, modules []string) bazel.LabelList {
	return android.BazelLabelForModuleDepsWithFn(ctx, modules, bazelLabelForSharedModule)
}

func bazelLabelForHeaderDeps(ctx android.TopDownMutatorContext, modules []string) bazel.LabelList {
	// This is not elegant, but bp2build's shared library targets only propagate
	// their header information as part of the normal C++ provider.
	return bazelLabelForSharedDeps(ctx, modules)
}

func bazelLabelForSharedDepsExcludes(ctx android.TopDownMutatorContext, modules, excludes []string) bazel.LabelList {
	return android.BazelLabelForModuleDepsExcludesWithFn(ctx, modules, excludes, bazelLabelForSharedModule)
}
