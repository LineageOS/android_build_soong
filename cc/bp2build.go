// Copyright 2021 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package cc

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"android/soong/android"
	"android/soong/bazel"
	"android/soong/cc/config"
	"android/soong/genrule"

	"github.com/google/blueprint"

	"github.com/google/blueprint/proptools"
)

const (
	cSrcPartition       = "c"
	asSrcPartition      = "as"
	asmSrcPartition     = "asm"
	lSrcPartition       = "l"
	llSrcPartition      = "ll"
	cppSrcPartition     = "cpp"
	protoSrcPartition   = "proto"
	aidlSrcPartition    = "aidl"
	syspropSrcPartition = "sysprop"

	yaccSrcPartition = "yacc"

	rScriptSrcPartition = "renderScript"

	xsdSrcPartition = "xsd"

	genrulePartition = "genrule"

	protoFromGenPartition = "proto_gen"

	hdrPartition = "hdr"

	stubsSuffix = "_stub_libs_current"
)

// staticOrSharedAttributes are the Bazel-ified versions of StaticOrSharedProperties --
// properties which apply to either the shared or static version of a cc_library module.
type staticOrSharedAttributes struct {
	Srcs      bazel.LabelListAttribute
	Srcs_c    bazel.LabelListAttribute
	Srcs_as   bazel.LabelListAttribute
	Srcs_aidl bazel.LabelListAttribute
	Hdrs      bazel.LabelListAttribute
	Copts     bazel.StringListAttribute

	Additional_compiler_inputs bazel.LabelListAttribute

	Deps                              bazel.LabelListAttribute
	Implementation_deps               bazel.LabelListAttribute
	Dynamic_deps                      bazel.LabelListAttribute
	Implementation_dynamic_deps       bazel.LabelListAttribute
	Whole_archive_deps                bazel.LabelListAttribute
	Implementation_whole_archive_deps bazel.LabelListAttribute
	Runtime_deps                      bazel.LabelListAttribute

	System_dynamic_deps bazel.LabelListAttribute

	Enabled bazel.BoolAttribute

	Native_coverage *bool

	Apex_available []string

	Features bazel.StringListAttribute

	sdkAttributes

	tidyAttributes
}

type tidyAttributes struct {
	Tidy                  *string
	Tidy_flags            []string
	Tidy_checks           []string
	Tidy_checks_as_errors []string
	Tidy_disabled_srcs    bazel.LabelListAttribute
	Tidy_timeout_srcs     bazel.LabelListAttribute
}

func (m *Module) convertTidyAttributes(ctx android.Bp2buildMutatorContext, moduleAttrs *tidyAttributes) {
	for _, f := range m.features {
		if tidy, ok := f.(*tidyFeature); ok {
			var tidyAttr *string
			if tidy.Properties.Tidy != nil {
				if *tidy.Properties.Tidy {
					tidyAttr = proptools.StringPtr("local")
				} else {
					tidyAttr = proptools.StringPtr("never")
				}
			}
			moduleAttrs.Tidy = tidyAttr
			moduleAttrs.Tidy_flags = tidy.Properties.Tidy_flags
			moduleAttrs.Tidy_checks = tidy.Properties.Tidy_checks
			moduleAttrs.Tidy_checks_as_errors = tidy.Properties.Tidy_checks_as_errors
		}

	}
	archVariantProps := m.GetArchVariantProperties(ctx, &BaseCompilerProperties{})
	for axis, configToProps := range archVariantProps {
		for cfg, _props := range configToProps {
			if archProps, ok := _props.(*BaseCompilerProperties); ok {
				archDisabledSrcs := android.BazelLabelForModuleSrc(ctx, archProps.Tidy_disabled_srcs)
				moduleAttrs.Tidy_disabled_srcs.SetSelectValue(axis, cfg, archDisabledSrcs)
				archTimeoutSrcs := android.BazelLabelForModuleSrc(ctx, archProps.Tidy_timeout_srcs)
				moduleAttrs.Tidy_timeout_srcs.SetSelectValue(axis, cfg, archTimeoutSrcs)
			}
		}
	}
}

// Returns an unchanged label and a bool indicating whether the dep is a genrule that produces .proto files
func protoFromGenPartitionMapper(pathCtx android.BazelConversionContext) bazel.LabelMapper {
	return func(ctx bazel.OtherModuleContext, label bazel.Label) (string, bool) {
		mod, exists := ctx.ModuleFromName(label.OriginalModuleName)
		if !exists {
			return label.Label, false
		}
		gen, isGen := mod.(*genrule.Module)
		if !isGen {
			return label.Label, false
		}
		if containsProto(ctx, gen.RawOutputFiles(pathCtx)) {
			// Return unmodified label + true
			// True will ensure that this module gets dropped from `srcs` of the generated cc_* target. `srcs` is reserved for .cpp files
			return label.Label, true
		}
		return label.Label, false
	}
}

// Returns true if srcs contains only .proto files
// Raises an exception if there is a combination of .proto and non .proto srcs
func containsProto(ctx bazel.OtherModuleContext, srcs []string) bool {
	onlyProtos := false
	for _, src := range srcs {
		if strings.HasSuffix(src, ".proto") {
			onlyProtos = true
		} else if onlyProtos {
			// This is not a proto file, and we have encountered a .proto file previously
			ctx.ModuleErrorf("TOOD: Add bp2build support combination of .proto and non .proto files in outs of genrule")
		}
	}
	return onlyProtos
}

// groupSrcsByExtension partitions `srcs` into groups based on file extension.
func groupSrcsByExtension(ctx android.BazelConversionPathContext, srcs bazel.LabelListAttribute) bazel.PartitionToLabelListAttribute {
	// Convert filegroup dependencies into extension-specific filegroups filtered in the filegroup.bzl
	// macro.
	addSuffixForFilegroup := func(suffix string) bazel.LabelMapper {
		return func(otherModuleCtx bazel.OtherModuleContext, label bazel.Label) (string, bool) {

			m, exists := otherModuleCtx.ModuleFromName(label.OriginalModuleName)
			labelStr := label.Label
			if !exists || !android.IsFilegroup(otherModuleCtx, m) {
				return labelStr, false
			}
			// If the filegroup is already converted to aidl_library or proto_library,
			// skip creating _c_srcs, _as_srcs, _cpp_srcs filegroups
			fg, _ := m.(android.FileGroupAsLibrary)
			if fg.ShouldConvertToAidlLibrary(ctx) || fg.ShouldConvertToProtoLibrary(ctx) {
				return labelStr, false
			}
			return labelStr + suffix, true
		}
	}

	// TODO(b/190006308): Handle language detection of sources in a Bazel rule.
	labels := bazel.LabelPartitions{
		protoSrcPartition: android.ProtoSrcLabelPartition,
		cSrcPartition:     bazel.LabelPartition{Extensions: []string{".c"}, LabelMapper: addSuffixForFilegroup("_c_srcs")},
		asSrcPartition:    bazel.LabelPartition{Extensions: []string{".s", ".S"}, LabelMapper: addSuffixForFilegroup("_as_srcs")},
		asmSrcPartition:   bazel.LabelPartition{Extensions: []string{".asm"}},
		aidlSrcPartition:  android.AidlSrcLabelPartition,
		// TODO(http://b/231968910): If there is ever a filegroup target that
		// 		contains .l or .ll files we will need to find a way to add a
		// 		LabelMapper for these that identifies these filegroups and
		//		converts them appropriately
		lSrcPartition:         bazel.LabelPartition{Extensions: []string{".l"}},
		llSrcPartition:        bazel.LabelPartition{Extensions: []string{".ll"}},
		rScriptSrcPartition:   bazel.LabelPartition{Extensions: []string{".fs", ".rscript"}},
		xsdSrcPartition:       bazel.LabelPartition{LabelMapper: android.XsdLabelMapper(xsdConfigCppTarget)},
		protoFromGenPartition: bazel.LabelPartition{LabelMapper: protoFromGenPartitionMapper(ctx)},
		// C++ is the "catch-all" group, and comprises generated sources because we don't
		// know the language of these sources until the genrule is executed.
		cppSrcPartition:     bazel.LabelPartition{Extensions: []string{".cpp", ".cc", ".cxx", ".mm"}, LabelMapper: addSuffixForFilegroup("_cpp_srcs"), Keep_remainder: true},
		syspropSrcPartition: bazel.LabelPartition{Extensions: []string{".sysprop"}},
		yaccSrcPartition:    bazel.LabelPartition{Extensions: []string{".y", "yy"}},
	}

	return bazel.PartitionLabelListAttribute(ctx, &srcs, labels)
}

func partitionHeaders(ctx android.BazelConversionPathContext, hdrs bazel.LabelListAttribute) bazel.PartitionToLabelListAttribute {
	labels := bazel.LabelPartitions{
		xsdSrcPartition:  bazel.LabelPartition{LabelMapper: android.XsdLabelMapper(xsdConfigCppTarget)},
		genrulePartition: bazel.LabelPartition{LabelMapper: genrule.GenruleCcHeaderLabelMapper},
		hdrPartition:     bazel.LabelPartition{Keep_remainder: true},
	}
	return bazel.PartitionLabelListAttribute(ctx, &hdrs, labels)
}

// bp2BuildParseLibProps returns the attributes for a variant of a cc_library.
func bp2BuildParseLibProps(ctx android.Bp2buildMutatorContext, module *Module, isStatic bool) staticOrSharedAttributes {
	lib, ok := module.compiler.(*libraryDecorator)
	if !ok {
		return staticOrSharedAttributes{}
	}
	return bp2buildParseStaticOrSharedProps(ctx, module, lib, isStatic)
}

// bp2buildParseSharedProps returns the attributes for the shared variant of a cc_library.
func bp2BuildParseSharedProps(ctx android.Bp2buildMutatorContext, module *Module) staticOrSharedAttributes {
	return bp2BuildParseLibProps(ctx, module, false)
}

// bp2buildParseStaticProps returns the attributes for the static variant of a cc_library.
func bp2BuildParseStaticProps(ctx android.Bp2buildMutatorContext, module *Module) staticOrSharedAttributes {
	return bp2BuildParseLibProps(ctx, module, true)
}

type depsPartition struct {
	export         bazel.LabelList
	implementation bazel.LabelList
}

type bazelLabelForDepsFn func(android.Bp2buildMutatorContext, []string) bazel.LabelList

func maybePartitionExportedAndImplementationsDeps(ctx android.Bp2buildMutatorContext, exportsDeps bool, allDeps, exportedDeps []string, fn bazelLabelForDepsFn) depsPartition {
	if !exportsDeps {
		return depsPartition{
			implementation: fn(ctx, allDeps),
		}
	}

	implementation, export := android.FilterList(allDeps, exportedDeps)

	return depsPartition{
		export:         fn(ctx, export),
		implementation: fn(ctx, implementation),
	}
}

type bazelLabelForDepsExcludesFn func(android.Bp2buildMutatorContext, []string, []string) bazel.LabelList

func maybePartitionExportedAndImplementationsDepsExcludes(ctx android.Bp2buildMutatorContext, exportsDeps bool, allDeps, excludes, exportedDeps []string, fn bazelLabelForDepsExcludesFn) depsPartition {
	if !exportsDeps {
		return depsPartition{
			implementation: fn(ctx, allDeps, excludes),
		}
	}
	implementation, export := android.FilterList(allDeps, exportedDeps)

	return depsPartition{
		export:         fn(ctx, export, excludes),
		implementation: fn(ctx, implementation, excludes),
	}
}

func bp2BuildPropParseHelper(ctx android.ArchVariantContext, module *Module, propsType interface{}, parseFunc func(axis bazel.ConfigurationAxis, config string, props interface{})) {
	for axis, configToProps := range module.GetArchVariantProperties(ctx, propsType) {
		for cfg, props := range configToProps {
			parseFunc(axis, cfg, props)
		}
	}
}

// Parses properties common to static and shared libraries. Also used for prebuilt libraries.
func bp2buildParseStaticOrSharedProps(ctx android.Bp2buildMutatorContext, module *Module, lib *libraryDecorator, isStatic bool) staticOrSharedAttributes {
	attrs := staticOrSharedAttributes{}

	setAttrs := func(axis bazel.ConfigurationAxis, config string, props StaticOrSharedProperties) {
		attrs.Copts.SetSelectValue(axis, config, parseCommandLineFlags(props.Cflags, filterOutStdFlag, filterOutHiddenVisibility))
		attrs.Srcs.SetSelectValue(axis, config, android.BazelLabelForModuleSrc(ctx, props.Srcs))
		attrs.System_dynamic_deps.SetSelectValue(axis, config, bazelLabelForSharedDeps(ctx, props.System_shared_libs))

		staticDeps := maybePartitionExportedAndImplementationsDeps(ctx, true, props.Static_libs, props.Export_static_lib_headers, bazelLabelForStaticDeps)
		attrs.Deps.SetSelectValue(axis, config, staticDeps.export)
		attrs.Implementation_deps.SetSelectValue(axis, config, staticDeps.implementation)

		sharedDeps := maybePartitionExportedAndImplementationsDeps(ctx, true, props.Shared_libs, props.Export_shared_lib_headers, bazelLabelForSharedDeps)
		attrs.Dynamic_deps.SetSelectValue(axis, config, sharedDeps.export)
		attrs.Implementation_dynamic_deps.SetSelectValue(axis, config, sharedDeps.implementation)

		attrs.Whole_archive_deps.SetSelectValue(axis, config, bazelLabelForWholeDeps(ctx, props.Whole_static_libs))
		attrs.Enabled.SetSelectValue(axis, config, props.Enabled)
	}
	// system_dynamic_deps distinguishes between nil/empty list behavior:
	//    nil -> use default values
	//    empty list -> no values specified
	attrs.System_dynamic_deps.ForceSpecifyEmptyList = true

	var apexAvailable []string
	if isStatic {
		apexAvailable = lib.StaticProperties.Static.Apex_available
		bp2BuildPropParseHelper(ctx, module, &StaticProperties{}, func(axis bazel.ConfigurationAxis, config string, props interface{}) {
			if staticOrSharedProps, ok := props.(*StaticProperties); ok {
				setAttrs(axis, config, staticOrSharedProps.Static)
			}
		})
	} else {
		apexAvailable = lib.SharedProperties.Shared.Apex_available
		bp2BuildPropParseHelper(ctx, module, &SharedProperties{}, func(axis bazel.ConfigurationAxis, config string, props interface{}) {
			if staticOrSharedProps, ok := props.(*SharedProperties); ok {
				setAttrs(axis, config, staticOrSharedProps.Shared)
			}
		})
	}

	partitionedSrcs := groupSrcsByExtension(ctx, attrs.Srcs)
	attrs.Srcs = partitionedSrcs[cppSrcPartition]
	attrs.Srcs_c = partitionedSrcs[cSrcPartition]
	attrs.Srcs_as = partitionedSrcs[asSrcPartition]

	attrs.Apex_available = android.ConvertApexAvailableToTagsWithoutTestApexes(ctx, apexAvailable)

	attrs.Features.Append(convertHiddenVisibilityToFeatureStaticOrShared(ctx, module, isStatic))

	if !partitionedSrcs[protoSrcPartition].IsEmpty() {
		// TODO(b/208815215): determine whether this is used and add support if necessary
		ctx.ModuleErrorf("Migrating static/shared only proto srcs is not currently supported")
	}

	return attrs
}

// Convenience struct to hold all attributes parsed from prebuilt properties.
type prebuiltAttributes struct {
	Src     bazel.LabelAttribute
	Enabled bazel.BoolAttribute
}

func parseSrc(ctx android.Bp2buildMutatorContext, srcLabelAttribute *bazel.LabelAttribute, axis bazel.ConfigurationAxis, config string, srcs []string) {
	srcFileError := func() {
		ctx.ModuleErrorf("parseSrc: Expected at most one source file for %s %s\n", axis, config)
	}
	if len(srcs) > 1 {
		srcFileError()
		return
	} else if len(srcs) == 0 {
		return
	}
	if srcLabelAttribute.SelectValue(axis, config) != nil {
		srcFileError()
		return
	}
	srcLabelAttribute.SetSelectValue(axis, config, android.BazelLabelForModuleSrcSingle(ctx, srcs[0]))
}

// NOTE: Used outside of Soong repo project, in the clangprebuilts.go bootstrap_go_package
func Bp2BuildParsePrebuiltLibraryProps(ctx android.Bp2buildMutatorContext, module *Module, isStatic bool) prebuiltAttributes {

	var srcLabelAttribute bazel.LabelAttribute
	bp2BuildPropParseHelper(ctx, module, &prebuiltLinkerProperties{}, func(axis bazel.ConfigurationAxis, config string, props interface{}) {
		if prebuiltLinkerProperties, ok := props.(*prebuiltLinkerProperties); ok {
			parseSrc(ctx, &srcLabelAttribute, axis, config, prebuiltLinkerProperties.Srcs)
		}
	})

	var enabledLabelAttribute bazel.BoolAttribute
	parseAttrs := func(axis bazel.ConfigurationAxis, config string, props StaticOrSharedProperties) {
		if props.Enabled != nil {
			enabledLabelAttribute.SetSelectValue(axis, config, props.Enabled)
		}
		parseSrc(ctx, &srcLabelAttribute, axis, config, props.Srcs)
	}

	if isStatic {
		bp2BuildPropParseHelper(ctx, module, &StaticProperties{}, func(axis bazel.ConfigurationAxis, config string, props interface{}) {
			if staticProperties, ok := props.(*StaticProperties); ok {
				parseAttrs(axis, config, staticProperties.Static)
			}
		})
	} else {
		bp2BuildPropParseHelper(ctx, module, &SharedProperties{}, func(axis bazel.ConfigurationAxis, config string, props interface{}) {
			if sharedProperties, ok := props.(*SharedProperties); ok {
				parseAttrs(axis, config, sharedProperties.Shared)
			}
		})
	}

	return prebuiltAttributes{
		Src:     srcLabelAttribute,
		Enabled: enabledLabelAttribute,
	}
}

func bp2BuildParsePrebuiltBinaryProps(ctx android.Bp2buildMutatorContext, module *Module) prebuiltAttributes {
	var srcLabelAttribute bazel.LabelAttribute
	bp2BuildPropParseHelper(ctx, module, &prebuiltLinkerProperties{}, func(axis bazel.ConfigurationAxis, config string, props interface{}) {
		if props, ok := props.(*prebuiltLinkerProperties); ok {
			parseSrc(ctx, &srcLabelAttribute, axis, config, props.Srcs)
		}
	})

	return prebuiltAttributes{
		Src: srcLabelAttribute,
	}
}

func bp2BuildParsePrebuiltObjectProps(ctx android.Bp2buildMutatorContext, module *Module) prebuiltAttributes {
	var srcLabelAttribute bazel.LabelAttribute
	bp2BuildPropParseHelper(ctx, module, &prebuiltObjectProperties{}, func(axis bazel.ConfigurationAxis, config string, props interface{}) {
		if props, ok := props.(*prebuiltObjectProperties); ok {
			parseSrc(ctx, &srcLabelAttribute, axis, config, props.Srcs)
		}
	})

	return prebuiltAttributes{
		Src: srcLabelAttribute,
	}
}

type baseAttributes struct {
	compilerAttributes
	linkerAttributes

	// A combination of compilerAttributes.features and linkerAttributes.features, as well as sanitizer features
	features        bazel.StringListAttribute
	protoDependency *bazel.LabelAttribute
	aidlDependency  *bazel.LabelAttribute
	Native_coverage *bool
}

// Convenience struct to hold all attributes parsed from compiler properties.
type compilerAttributes struct {
	// Options for all languages
	copts bazel.StringListAttribute
	// Assembly options and sources
	asFlags bazel.StringListAttribute
	asSrcs  bazel.LabelListAttribute
	asmSrcs bazel.LabelListAttribute
	// C options and sources
	conlyFlags bazel.StringListAttribute
	cSrcs      bazel.LabelListAttribute
	// C++ options and sources
	cppFlags bazel.StringListAttribute
	srcs     bazel.LabelListAttribute

	// xsd config sources
	xsdSrcs       bazel.LabelListAttribute
	exportXsdSrcs bazel.LabelListAttribute

	// genrule headers
	genruleHeaders       bazel.LabelListAttribute
	exportGenruleHeaders bazel.LabelListAttribute

	// Lex sources and options
	lSrcs   bazel.LabelListAttribute
	llSrcs  bazel.LabelListAttribute
	lexopts bazel.StringListAttribute

	// Sysprop sources
	syspropSrcs bazel.LabelListAttribute

	// Yacc sources
	yaccSrc               *bazel.LabelAttribute
	yaccFlags             bazel.StringListAttribute
	yaccGenLocationHeader bazel.BoolAttribute
	yaccGenPositionHeader bazel.BoolAttribute

	rsSrcs bazel.LabelListAttribute

	hdrs bazel.LabelListAttribute

	rtti bazel.BoolAttribute

	// Not affected by arch variants
	stl    *string
	cStd   *string
	cppStd *string

	localIncludes    bazel.StringListAttribute
	absoluteIncludes bazel.StringListAttribute

	includes BazelIncludes

	protoSrcs   bazel.LabelListAttribute
	aidlSrcs    bazel.LabelListAttribute
	rscriptSrcs bazel.LabelListAttribute

	stubsSymbolFile *string
	stubsVersions   bazel.StringListAttribute

	features bazel.StringListAttribute

	stem   bazel.StringAttribute
	suffix bazel.StringAttribute

	fdoProfile bazel.LabelAttribute

	additionalCompilerInputs bazel.LabelListAttribute
}

type filterOutFn func(string) bool

// filterOutHiddenVisibility removes the flag specifying hidden visibility as
// this flag is converted to a toolchain feature
func filterOutHiddenVisibility(flag string) bool {
	return flag == config.VisibilityHiddenFlag
}

func filterOutStdFlag(flag string) bool {
	return strings.HasPrefix(flag, "-std=")
}

func filterOutClangUnknownCflags(flag string) bool {
	for _, f := range config.ClangUnknownCflags {
		if f == flag {
			return true
		}
	}
	return false
}

func parseCommandLineFlags(soongFlags []string, filterOut ...filterOutFn) []string {
	var result []string
	for _, flag := range soongFlags {
		skipFlag := false
		for _, filter := range filterOut {
			if filter != nil && filter(flag) {
				skipFlag = true
			}
		}
		if skipFlag {
			continue
		}
		// Soong's cflags can contain spaces, like `-include header.h`. For
		// Bazel's copts, split them up to be compatible with the
		// no_copts_tokenization feature.
		result = append(result, strings.Split(flag, " ")...)
	}
	return result
}

func (ca *compilerAttributes) bp2buildForAxisAndConfig(ctx android.Bp2buildMutatorContext, axis bazel.ConfigurationAxis, config string, props *BaseCompilerProperties) {
	// If there's arch specific srcs or exclude_srcs, generate a select entry for it.
	// TODO(b/186153868): do this for OS specific srcs and exclude_srcs too.
	srcsList, ok := parseSrcs(ctx, props)

	if ok {
		ca.srcs.SetSelectValue(axis, config, srcsList)
	}

	localIncludeDirs := props.Local_include_dirs
	if axis == bazel.NoConfigAxis {
		ca.cStd, ca.cppStd = bp2buildResolveCppStdValue(props.C_std, props.Cpp_std, props.Gnu_extensions)
		if includeBuildDirectory(props.Include_build_directory) {
			localIncludeDirs = append(localIncludeDirs, ".")
		}
	}

	ca.absoluteIncludes.SetSelectValue(axis, config, props.Include_dirs)
	ca.localIncludes.SetSelectValue(axis, config, localIncludeDirs)

	instructionSet := proptools.StringDefault(props.Instruction_set, "")
	if instructionSet == "arm" {
		ca.features.SetSelectValue(axis, config, []string{"arm_isa_arm"})
	} else if instructionSet != "" && instructionSet != "thumb" {
		ctx.ModuleErrorf("Unknown value for instruction_set: %s", instructionSet)
	}

	// In Soong, cflags occur on the command line before -std=<val> flag, resulting in the value being
	// overridden. In Bazel we always allow overriding, via flags; however, this can cause
	// incompatibilities, so we remove "-std=" flags from Cflag properties while leaving it in other
	// cases.
	ca.copts.SetSelectValue(axis, config, parseCommandLineFlags(props.Cflags, filterOutStdFlag, filterOutClangUnknownCflags, filterOutHiddenVisibility))
	ca.asFlags.SetSelectValue(axis, config, parseCommandLineFlags(props.Asflags, nil))
	ca.conlyFlags.SetSelectValue(axis, config, parseCommandLineFlags(props.Conlyflags, filterOutClangUnknownCflags))
	ca.cppFlags.SetSelectValue(axis, config, parseCommandLineFlags(props.Cppflags, filterOutClangUnknownCflags))
	ca.rtti.SetSelectValue(axis, config, props.Rtti)
}

func (ca *compilerAttributes) convertStlProps(ctx android.ArchVariantContext, module *Module) {
	bp2BuildPropParseHelper(ctx, module, &StlProperties{}, func(axis bazel.ConfigurationAxis, config string, props interface{}) {
		if stlProps, ok := props.(*StlProperties); ok {
			if stlProps.Stl == nil {
				return
			}
			if ca.stl == nil {
				stl := deduplicateStlInput(*stlProps.Stl)
				ca.stl = &stl
			} else if ca.stl != stlProps.Stl {
				ctx.ModuleErrorf("Unsupported conversion: module with different stl for different variants: %s and %s", *ca.stl, stlProps.Stl)
			}
		}
	})
}

func (ca *compilerAttributes) convertProductVariables(ctx android.BazelConversionPathContext, productVariableProps android.ProductConfigProperties) {
	productVarPropNameToAttribute := map[string]*bazel.StringListAttribute{
		"Cflags":   &ca.copts,
		"Asflags":  &ca.asFlags,
		"Cppflags": &ca.cppFlags,
	}
	for propName, attr := range productVarPropNameToAttribute {
		if productConfigProps, exists := productVariableProps[propName]; exists {
			for productConfigProp, prop := range productConfigProps {
				flags, ok := prop.([]string)
				if !ok {
					ctx.ModuleErrorf("Could not convert product variable %s property", proptools.PropertyNameForField(propName))
				}
				newFlags, _ := bazel.TryVariableSubstitutions(flags, productConfigProp.Name())
				attr.SetSelectValue(productConfigProp.ConfigurationAxis(), productConfigProp.SelectKey(), newFlags)
			}
		}
	}
}

func (ca *compilerAttributes) finalize(ctx android.BazelConversionPathContext, implementationHdrs, exportHdrs bazel.LabelListAttribute) {
	ca.srcs.ResolveExcludes()
	partitionedSrcs := groupSrcsByExtension(ctx, ca.srcs)
	partitionedImplHdrs := partitionHeaders(ctx, implementationHdrs)
	partitionedHdrs := partitionHeaders(ctx, exportHdrs)

	ca.protoSrcs = partitionedSrcs[protoSrcPartition]
	ca.protoSrcs.Append(partitionedSrcs[protoFromGenPartition])
	ca.aidlSrcs = partitionedSrcs[aidlSrcPartition]

	for p, lla := range partitionedSrcs {
		// if there are no sources, there is no need for headers
		if lla.IsEmpty() {
			continue
		}
		lla.Append(partitionedImplHdrs[hdrPartition])
		partitionedSrcs[p] = lla
	}

	ca.hdrs = partitionedHdrs[hdrPartition]

	ca.includesFromHeaders(ctx, partitionedImplHdrs[hdrPartition], partitionedHdrs[hdrPartition])

	xsdSrcs := bazel.SubtractBazelLabelListAttribute(partitionedSrcs[xsdSrcPartition], partitionedHdrs[xsdSrcPartition])
	xsdSrcs.Append(partitionedImplHdrs[xsdSrcPartition])
	ca.exportXsdSrcs = partitionedHdrs[xsdSrcPartition]
	ca.xsdSrcs = bazel.FirstUniqueBazelLabelListAttribute(xsdSrcs)

	ca.genruleHeaders = partitionedImplHdrs[genrulePartition]
	ca.exportGenruleHeaders = partitionedHdrs[genrulePartition]

	ca.srcs = partitionedSrcs[cppSrcPartition]
	ca.cSrcs = partitionedSrcs[cSrcPartition]
	ca.asSrcs = partitionedSrcs[asSrcPartition]
	ca.asmSrcs = partitionedSrcs[asmSrcPartition]
	ca.lSrcs = partitionedSrcs[lSrcPartition]
	ca.llSrcs = partitionedSrcs[llSrcPartition]
	if yacc := partitionedSrcs[yaccSrcPartition]; !yacc.IsEmpty() {
		if len(yacc.Value.Includes) > 1 {
			ctx.PropertyErrorf("srcs", "Found multiple yacc (.y/.yy) files in library")
		}
		ca.yaccSrc = bazel.MakeLabelAttribute(yacc.Value.Includes[0].Label)
	}
	ca.syspropSrcs = partitionedSrcs[syspropSrcPartition]
	ca.rscriptSrcs = partitionedSrcs[rScriptSrcPartition]

	ca.absoluteIncludes.DeduplicateAxesFromBase()
	ca.localIncludes.DeduplicateAxesFromBase()
}

// Parse srcs from an arch or OS's props value.
func parseSrcs(ctx android.Bp2buildMutatorContext, props *BaseCompilerProperties) (bazel.LabelList, bool) {
	anySrcs := false
	// Add srcs-like dependencies such as generated files.
	// First create a LabelList containing these dependencies, then merge the values with srcs.
	genSrcs := props.Generated_sources
	generatedSrcsLabelList := android.BazelLabelForModuleDepsExcludes(ctx, genSrcs, props.Exclude_generated_sources)
	if len(props.Generated_sources) > 0 || len(props.Exclude_generated_sources) > 0 {
		anySrcs = true
	}

	allSrcsLabelList := android.BazelLabelForModuleSrcExcludes(ctx, props.Srcs, props.Exclude_srcs)

	if len(props.Srcs) > 0 || len(props.Exclude_srcs) > 0 {
		anySrcs = true
	}

	return bazel.AppendBazelLabelLists(allSrcsLabelList, generatedSrcsLabelList), anySrcs
}

func bp2buildStdVal(std *string, prefix string, useGnu bool) *string {
	defaultVal := prefix + "_std_default"
	// If c{,pp}std properties are not specified, don't generate them in the BUILD file.
	// Defaults are handled by the toolchain definition.
	// However, if gnu_extensions is false, then the default gnu-to-c version must be specified.
	stdVal := proptools.StringDefault(std, defaultVal)
	if stdVal == "experimental" || stdVal == defaultVal {
		if stdVal == "experimental" {
			stdVal = prefix + "_std_experimental"
		}
		if !useGnu {
			stdVal += "_no_gnu"
		}
	} else if !useGnu {
		stdVal = gnuToCReplacer.Replace(stdVal)
	}

	if stdVal == defaultVal {
		return nil
	}
	return &stdVal
}

func bp2buildResolveCppStdValue(c_std *string, cpp_std *string, gnu_extensions *bool) (*string, *string) {
	useGnu := useGnuExtensions(gnu_extensions)

	return bp2buildStdVal(c_std, "c", useGnu), bp2buildStdVal(cpp_std, "cpp", useGnu)
}

// packageFromLabel extracts package from a fully-qualified or relative Label and whether the label
// is fully-qualified.
// e.g. fully-qualified "//a/b:foo" -> "a/b", true, relative: ":bar" -> ".", false
func packageFromLabel(label string) (string, bool) {
	split := strings.Split(label, ":")
	if len(split) != 2 {
		return "", false
	}
	if split[0] == "" {
		return ".", false
	}
	// remove leading "//"
	return split[0][2:], true
}

// includesFromHeaders gets the include directories needed from generated headers
func (ca *compilerAttributes) includesFromHeaders(ctx android.BazelConversionPathContext, implHdrs, hdrs bazel.LabelListAttribute) {
	local, absolute := includesFromLabelListAttribute(implHdrs, ca.localIncludes, ca.absoluteIncludes)
	localExport, absoluteExport := includesFromLabelListAttribute(hdrs, ca.includes.Includes, ca.includes.AbsoluteIncludes)

	ca.localIncludes = local
	ca.absoluteIncludes = absolute

	ca.includes.Includes = localExport
	ca.includes.AbsoluteIncludes = absoluteExport
}

// includesFromLabelList extracts the packages from a LabelListAttribute that should be includes and
// combines them with existing local/absolute includes.
func includesFromLabelListAttribute(attr bazel.LabelListAttribute, existingLocal, existingAbsolute bazel.StringListAttribute) (bazel.StringListAttribute, bazel.StringListAttribute) {
	localAttr := existingLocal.Clone()
	absoluteAttr := existingAbsolute.Clone()
	if !attr.Value.IsEmpty() {
		l, a := includesFromLabelList(attr.Value, existingLocal.Value, existingAbsolute.Value)
		localAttr.SetSelectValue(bazel.NoConfigAxis, "", l)
		absoluteAttr.SetSelectValue(bazel.NoConfigAxis, "", a)
	}
	for axis, configToLabels := range attr.ConfigurableValues {
		for c, labels := range configToLabels {
			local := existingLocal.SelectValue(axis, c)
			absolute := existingAbsolute.SelectValue(axis, c)
			l, a := includesFromLabelList(labels, local, absolute)
			localAttr.SetSelectValue(axis, c, l)
			absoluteAttr.SetSelectValue(axis, c, a)
		}
	}
	return *localAttr, *absoluteAttr
}

// includesFromLabelList extracts relative/absolute includes from a bazel.LabelList.
func includesFromLabelList(labelList bazel.LabelList, existingRel, existingAbs []string) ([]string, []string) {
	var relative, absolute []string
	for _, hdr := range labelList.Includes {
		if pkg, hasPkg := packageFromLabel(hdr.Label); hasPkg {
			absolute = append(absolute, pkg)
		} else if pkg != "" {
			relative = append(relative, pkg)
		}
	}
	if len(relative)+len(existingRel) != 0 {
		relative = android.FirstUniqueStrings(append(append([]string{}, existingRel...), relative...))
	}
	if len(absolute)+len(existingAbs) != 0 {
		absolute = android.FirstUniqueStrings(append(append([]string{}, existingAbs...), absolute...))
	}
	return relative, absolute
}

type YasmAttributes struct {
	Srcs         bazel.LabelListAttribute
	Flags        bazel.StringListAttribute
	Include_dirs bazel.StringListAttribute
}

func bp2BuildYasm(ctx android.Bp2buildMutatorContext, m *Module, ca compilerAttributes) *bazel.LabelAttribute {
	if ca.asmSrcs.IsEmpty() {
		return nil
	}

	// Yasm needs the include directories from both local_includes and
	// export_include_dirs. We don't care about actually exporting them from the
	// yasm rule though, because they will also be present on the cc_ rule that
	// wraps this yasm rule.
	includes := ca.localIncludes.Clone()
	bp2BuildPropParseHelper(ctx, m, &FlagExporterProperties{}, func(axis bazel.ConfigurationAxis, config string, props interface{}) {
		if flagExporterProperties, ok := props.(*FlagExporterProperties); ok {
			if len(flagExporterProperties.Export_include_dirs) > 0 {
				x := bazel.StringListAttribute{}
				x.SetSelectValue(axis, config, flagExporterProperties.Export_include_dirs)
				includes.Append(x)
			}
		}
	})

	ctx.CreateBazelTargetModule(
		bazel.BazelTargetModuleProperties{
			Rule_class:        "yasm",
			Bzl_load_location: "//build/bazel/rules/cc:yasm.bzl",
		},
		android.CommonAttributes{Name: m.Name() + "_yasm"},
		&YasmAttributes{
			Srcs:         ca.asmSrcs,
			Flags:        ca.asFlags,
			Include_dirs: *includes,
		})

	// We only want to add a dependency on the _yasm target if there are asm
	// sources in the current configuration. If there are unconfigured asm
	// sources, always add the dependency. Otherwise, add the dependency only
	// on the configuration axes and values that had asm sources.
	if len(ca.asmSrcs.Value.Includes) > 0 {
		return bazel.MakeLabelAttribute(":" + m.Name() + "_yasm")
	}

	ret := &bazel.LabelAttribute{}
	for _, axis := range ca.asmSrcs.SortedConfigurationAxes() {
		for cfg := range ca.asmSrcs.ConfigurableValues[axis] {
			ret.SetSelectValue(axis, cfg, bazel.Label{Label: ":" + m.Name() + "_yasm"})
		}
	}
	return ret
}

// bp2BuildParseBaseProps returns all compiler, linker, library attributes of a cc module.
func bp2BuildParseBaseProps(ctx android.Bp2buildMutatorContext, module *Module) baseAttributes {
	archVariantCompilerProps := module.GetArchVariantProperties(ctx, &BaseCompilerProperties{})
	archVariantLinkerProps := module.GetArchVariantProperties(ctx, &BaseLinkerProperties{})
	archVariantLibraryProperties := module.GetArchVariantProperties(ctx, &LibraryProperties{})

	axisToConfigs := map[bazel.ConfigurationAxis]map[string]bool{}
	allAxesAndConfigs := func(cp android.ConfigurationAxisToArchVariantProperties) {
		for axis, configMap := range cp {
			if _, ok := axisToConfigs[axis]; !ok {
				axisToConfigs[axis] = map[string]bool{}
			}
			for cfg := range configMap {
				axisToConfigs[axis][cfg] = true
			}
		}
	}
	allAxesAndConfigs(archVariantCompilerProps)
	allAxesAndConfigs(archVariantLinkerProps)
	allAxesAndConfigs(archVariantLibraryProperties)

	compilerAttrs := compilerAttributes{}
	linkerAttrs := linkerAttributes{}

	var aidlLibs bazel.LabelList
	var implementationHdrs, exportHdrs bazel.LabelListAttribute

	// Iterate through these axes in a deterministic order. This is required
	// because processing certain dependencies may result in concatenating
	// elements along other axes. (For example, processing NoConfig may result
	// in elements being added to InApex). This is thus the only way to ensure
	// that the order of entries in each list is in a predictable order.
	for _, axis := range bazel.SortedConfigurationAxes(axisToConfigs) {
		configs := axisToConfigs[axis]
		for cfg := range configs {
			var allHdrs []string
			if baseCompilerProps, ok := archVariantCompilerProps[axis][cfg].(*BaseCompilerProperties); ok {
				allHdrs = baseCompilerProps.Generated_headers

				if baseCompilerProps.Lex != nil {
					compilerAttrs.lexopts.SetSelectValue(axis, cfg, baseCompilerProps.Lex.Flags)
				}
				if baseCompilerProps.Yacc != nil {
					compilerAttrs.yaccFlags.SetSelectValue(axis, cfg, baseCompilerProps.Yacc.Flags)
					compilerAttrs.yaccGenLocationHeader.SetSelectValue(axis, cfg, baseCompilerProps.Yacc.Gen_location_hh)
					compilerAttrs.yaccGenPositionHeader.SetSelectValue(axis, cfg, baseCompilerProps.Yacc.Gen_position_hh)
				}
				(&compilerAttrs).bp2buildForAxisAndConfig(ctx, axis, cfg, baseCompilerProps)
				aidlLibs.Append(android.BazelLabelForModuleDeps(ctx, baseCompilerProps.Aidl.Libs))
			}

			var exportedHdrs []string

			if baseLinkerProps, ok := archVariantLinkerProps[axis][cfg].(*BaseLinkerProperties); ok {
				exportedHdrs = baseLinkerProps.Export_generated_headers
				(&linkerAttrs).bp2buildForAxisAndConfig(ctx, module, axis, cfg, baseLinkerProps)
			}

			headers := maybePartitionExportedAndImplementationsDeps(ctx, !module.Binary(), allHdrs, exportedHdrs, android.BazelLabelForModuleDeps)

			implementationHdrs.SetSelectValue(axis, cfg, headers.implementation)
			exportHdrs.SetSelectValue(axis, cfg, headers.export)

			if libraryProps, ok := archVariantLibraryProperties[axis][cfg].(*LibraryProperties); ok {
				if axis == bazel.NoConfigAxis {
					if libraryProps.Stubs.Symbol_file != nil {
						compilerAttrs.stubsSymbolFile = libraryProps.Stubs.Symbol_file
						versions := android.CopyOf(libraryProps.Stubs.Versions)
						normalizeVersions(ctx, versions)
						versions = addCurrentVersionIfNotPresent(versions)
						compilerAttrs.stubsVersions.SetSelectValue(axis, cfg, versions)
					}
				}
				if stem := libraryProps.Stem; stem != nil {
					compilerAttrs.stem.SetSelectValue(axis, cfg, stem)
				}
				if suffix := libraryProps.Suffix; suffix != nil {
					compilerAttrs.suffix.SetSelectValue(axis, cfg, suffix)
				}
			}

		}
	}

	compilerAttrs.convertStlProps(ctx, module)
	(&linkerAttrs).convertStripProps(ctx, module)

	var nativeCoverage *bool
	if module.coverage != nil && module.coverage.Properties.Native_coverage != nil &&
		!Bool(module.coverage.Properties.Native_coverage) {
		nativeCoverage = BoolPtr(false)
	}

	productVariableProps, errs := android.ProductVariableProperties(ctx, ctx.Module())
	for _, err := range errs {
		ctx.ModuleErrorf("ProductVariableProperties error: %s", err)
	}

	(&compilerAttrs).convertProductVariables(ctx, productVariableProps)
	(&linkerAttrs).convertProductVariables(ctx, productVariableProps)

	(&compilerAttrs).finalize(ctx, implementationHdrs, exportHdrs)
	(&linkerAttrs).finalize(ctx)

	(&compilerAttrs.srcs).Add(bp2BuildYasm(ctx, module, compilerAttrs))

	(&linkerAttrs).deps.Append(compilerAttrs.exportGenruleHeaders)
	(&linkerAttrs).implementationDeps.Append(compilerAttrs.genruleHeaders)

	(&linkerAttrs).wholeArchiveDeps.Append(compilerAttrs.exportXsdSrcs)
	(&linkerAttrs).implementationWholeArchiveDeps.Append(compilerAttrs.xsdSrcs)

	protoDep := bp2buildProto(ctx, module, compilerAttrs.protoSrcs, linkerAttrs)

	// bp2buildProto will only set wholeStaticLib or implementationWholeStaticLib, but we don't know
	// which. This will add the newly generated proto library to the appropriate attribute and nothing
	// to the other
	(&linkerAttrs).wholeArchiveDeps.Add(protoDep.wholeStaticLib)
	(&linkerAttrs).implementationWholeArchiveDeps.Add(protoDep.implementationWholeStaticLib)

	aidlDep := bp2buildCcAidlLibrary(
		ctx, module,
		compilerAttrs.aidlSrcs,
		bazel.LabelListAttribute{
			Value: aidlLibs,
		},
		linkerAttrs,
		compilerAttrs,
	)
	if aidlDep != nil {
		if lib, ok := module.linker.(*libraryDecorator); ok {
			if proptools.Bool(lib.Properties.Aidl.Export_aidl_headers) {
				(&linkerAttrs).wholeArchiveDeps.Add(aidlDep)
			} else {
				(&linkerAttrs).implementationWholeArchiveDeps.Add(aidlDep)
			}
		}
	}

	// Create a cc_yacc_static_library if srcs contains .y/.yy files
	// This internal target will produce an .a file that will be statically linked to the parent library
	if yaccDep := bp2buildCcYaccLibrary(ctx, compilerAttrs, linkerAttrs); yaccDep != nil {
		(&linkerAttrs).implementationWholeArchiveDeps.Add(yaccDep)
	}

	convertedLSrcs := bp2BuildLex(ctx, module.Name(), compilerAttrs)
	(&compilerAttrs).srcs.Add(&convertedLSrcs.srcName)
	(&compilerAttrs).cSrcs.Add(&convertedLSrcs.cSrcName)

	if module.afdo != nil && module.afdo.Properties.Afdo {
		fdoProfileDep := bp2buildFdoProfile(ctx, module)
		if fdoProfileDep != nil {
			// TODO(b/276287371): Only set fdo_profile for android platform
			// https://cs.android.com/android/platform/superproject/main/+/main:build/soong/cc/afdo.go;l=105;drc=2dbe160d1af445de32725098570ec594e3944fc5
			(&compilerAttrs).fdoProfile.SetValue(*fdoProfileDep)
		}
	}

	if !compilerAttrs.syspropSrcs.IsEmpty() {
		(&linkerAttrs).wholeArchiveDeps.Add(bp2buildCcSysprop(ctx, module.Name(), module.Properties.Min_sdk_version, compilerAttrs.syspropSrcs))
	}

	linkerAttrs.wholeArchiveDeps.Prepend = true
	linkerAttrs.deps.Prepend = true
	compilerAttrs.localIncludes.Prepend = true
	compilerAttrs.absoluteIncludes.Prepend = true
	compilerAttrs.hdrs.Prepend = true

	convertedRsSrcs, rsAbsIncludes, rsLocalIncludes := bp2buildRScript(ctx, module, compilerAttrs)
	(&compilerAttrs).srcs.Add(&convertedRsSrcs)
	(&compilerAttrs).absoluteIncludes.Append(rsAbsIncludes)
	(&compilerAttrs).localIncludes.Append(rsLocalIncludes)
	(&compilerAttrs).localIncludes.Value = android.FirstUniqueStrings(compilerAttrs.localIncludes.Value)

	sanitizerValues := bp2buildSanitizerFeatures(ctx, module)

	features := compilerAttrs.features.Clone().Append(linkerAttrs.features).Append(sanitizerValues.features)
	features = features.Append(bp2buildLtoFeatures(ctx, module))
	features = features.Append(convertHiddenVisibilityToFeatureBase(ctx, module))
	features.DeduplicateAxesFromBase()

	compilerAttrs.copts = *compilerAttrs.copts.Append(sanitizerValues.copts)
	compilerAttrs.additionalCompilerInputs = *compilerAttrs.additionalCompilerInputs.Append(sanitizerValues.additionalCompilerInputs)

	addMuslSystemDynamicDeps(ctx, linkerAttrs)

	// Dedupe all deps.
	(&linkerAttrs).deps.Value = bazel.FirstUniqueBazelLabelList((&linkerAttrs).deps.Value)
	(&linkerAttrs).implementationDeps.Value = bazel.FirstUniqueBazelLabelList((&linkerAttrs).implementationDeps.Value)
	(&linkerAttrs).implementationDynamicDeps.Value = bazel.FirstUniqueBazelLabelList((&linkerAttrs).implementationDynamicDeps.Value)
	(&linkerAttrs).wholeArchiveDeps.Value = bazel.FirstUniqueBazelLabelList((&linkerAttrs).wholeArchiveDeps.Value)
	(&linkerAttrs).implementationWholeArchiveDeps.Value = bazel.FirstUniqueBazelLabelList((&linkerAttrs).implementationWholeArchiveDeps.Value)

	return baseAttributes{
		compilerAttrs,
		linkerAttrs,
		*features,
		protoDep.protoDep,
		aidlDep,
		nativeCoverage,
	}
}

type ccYaccLibraryAttributes struct {
	Src                         bazel.LabelAttribute
	Flags                       bazel.StringListAttribute
	Gen_location_hh             bazel.BoolAttribute
	Gen_position_hh             bazel.BoolAttribute
	Local_includes              bazel.StringListAttribute
	Implementation_deps         bazel.LabelListAttribute
	Implementation_dynamic_deps bazel.LabelListAttribute
}

func bp2buildCcYaccLibrary(ctx android.Bp2buildMutatorContext, ca compilerAttributes, la linkerAttributes) *bazel.LabelAttribute {
	if ca.yaccSrc == nil {
		return nil
	}
	yaccLibraryLabel := ctx.Module().Name() + "_yacc"
	ctx.CreateBazelTargetModule(
		bazel.BazelTargetModuleProperties{
			Rule_class:        "cc_yacc_static_library",
			Bzl_load_location: "//build/bazel/rules/cc:cc_yacc_library.bzl",
		},
		android.CommonAttributes{
			Name: yaccLibraryLabel,
		},
		&ccYaccLibraryAttributes{
			Src:                         *ca.yaccSrc,
			Flags:                       ca.yaccFlags,
			Gen_location_hh:             ca.yaccGenLocationHeader,
			Gen_position_hh:             ca.yaccGenPositionHeader,
			Local_includes:              ca.localIncludes,
			Implementation_deps:         la.implementationDeps,
			Implementation_dynamic_deps: la.implementationDynamicDeps,
		},
	)

	yaccLibrary := &bazel.LabelAttribute{
		Value: &bazel.Label{
			Label: ":" + yaccLibraryLabel,
		},
	}
	return yaccLibrary
}

// As a workaround for b/261657184, we are manually adding the default value
// of system_dynamic_deps for the linux_musl os.
// TODO: Solve this properly
func addMuslSystemDynamicDeps(ctx android.Bp2buildMutatorContext, attrs linkerAttributes) {
	systemDynamicDeps := attrs.systemDynamicDeps.SelectValue(bazel.OsConfigurationAxis, "linux_musl")
	if attrs.systemDynamicDeps.HasAxisSpecificValues(bazel.OsConfigurationAxis) && systemDynamicDeps.IsNil() {
		attrs.systemDynamicDeps.SetSelectValue(bazel.OsConfigurationAxis, "linux_musl", android.BazelLabelForModuleDeps(ctx, config.MuslDefaultSharedLibraries))
	}
}

type fdoProfileAttributes struct {
	Absolute_path_profile string
}

func bp2buildFdoProfile(
	ctx android.Bp2buildMutatorContext,
	m *Module,
) *bazel.Label {
	// TODO(b/267229066): Convert to afdo boolean attribute and let Bazel handles finding
	// fdo_profile target from AfdoProfiles product var
	for _, project := range globalAfdoProfileProjects {
		// Ensure it's a Soong package
		bpPath := android.ExistentPathForSource(ctx, project, "Android.bp")
		if bpPath.Valid() {
			// TODO(b/260714900): Handle arch-specific afdo profiles (e.g. `<module-name>-arm<64>.afdo`)
			path := android.ExistentPathForSource(ctx, project, m.Name()+".afdo")
			if path.Valid() {
				fdoProfileLabel := "//" + strings.TrimSuffix(project, "/") + ":" + m.Name()
				return &bazel.Label{
					Label: fdoProfileLabel,
				}
			}
		}
	}

	return nil
}

func bp2buildCcAidlLibrary(
	ctx android.Bp2buildMutatorContext,
	m *Module,
	aidlSrcs bazel.LabelListAttribute,
	aidlLibs bazel.LabelListAttribute,
	linkerAttrs linkerAttributes,
	compilerAttrs compilerAttributes,
) *bazel.LabelAttribute {
	var aidlLibsFromSrcs, aidlFiles bazel.LabelListAttribute
	apexAvailableTags := android.ApexAvailableTagsWithoutTestApexes(ctx, ctx.Module())

	if !aidlSrcs.IsEmpty() {
		aidlLibsFromSrcs, aidlFiles = aidlSrcs.Partition(func(src bazel.Label) bool {
			if fg, ok := android.ToFileGroupAsLibrary(ctx, src.OriginalModuleName); ok &&
				fg.ShouldConvertToAidlLibrary(ctx) {
				return true
			}
			return false
		})

		if !aidlFiles.IsEmpty() {
			aidlLibName := m.Name() + "_aidl_library"
			ctx.CreateBazelTargetModule(
				bazel.BazelTargetModuleProperties{
					Rule_class:        "aidl_library",
					Bzl_load_location: "//build/bazel/rules/aidl:aidl_library.bzl",
				},
				android.CommonAttributes{
					Name: aidlLibName,
					Tags: apexAvailableTags,
				},
				&aidlLibraryAttributes{
					Srcs: aidlFiles,
				},
			)
			aidlLibsFromSrcs.Add(&bazel.LabelAttribute{Value: &bazel.Label{Label: ":" + aidlLibName}})
		}
	}

	allAidlLibs := aidlLibs.Clone()
	allAidlLibs.Append(aidlLibsFromSrcs)

	if !allAidlLibs.IsEmpty() {
		ccAidlLibrarylabel := m.Name() + "_cc_aidl_library"
		// Since parent cc_library already has these dependencies, we can add them as implementation
		// deps so that they don't re-export
		implementationDeps := linkerAttrs.deps.Clone()
		implementationDeps.Append(linkerAttrs.implementationDeps)
		implementationDynamicDeps := linkerAttrs.dynamicDeps.Clone()
		implementationDynamicDeps.Append(linkerAttrs.implementationDynamicDeps)

		sdkAttrs := bp2BuildParseSdkAttributes(m)

		exportedIncludes := bp2BuildParseExportedIncludes(ctx, m, &compilerAttrs.includes)
		includeAttrs := includesAttributes{
			Export_includes:          exportedIncludes.Includes,
			Export_absolute_includes: exportedIncludes.AbsoluteIncludes,
			Export_system_includes:   exportedIncludes.SystemIncludes,
			Local_includes:           compilerAttrs.localIncludes,
			Absolute_includes:        compilerAttrs.absoluteIncludes,
		}

		ctx.CreateBazelTargetModule(
			bazel.BazelTargetModuleProperties{
				Rule_class:        "cc_aidl_library",
				Bzl_load_location: "//build/bazel/rules/cc:cc_aidl_library.bzl",
			},
			android.CommonAttributes{Name: ccAidlLibrarylabel},
			&ccAidlLibraryAttributes{
				Deps:                        *allAidlLibs,
				Implementation_deps:         *implementationDeps,
				Implementation_dynamic_deps: *implementationDynamicDeps,
				Tags:                        apexAvailableTags,
				sdkAttributes:               sdkAttrs,
				includesAttributes:          includeAttrs,
			},
		)
		label := &bazel.LabelAttribute{
			Value: &bazel.Label{
				Label: ":" + ccAidlLibrarylabel,
			},
		}
		return label
	}

	return nil
}

func bp2BuildParseSdkAttributes(module *Module) sdkAttributes {
	return sdkAttributes{
		Sdk_version:     module.Properties.Sdk_version,
		Min_sdk_version: module.Properties.Min_sdk_version,
	}
}

type sdkAttributes struct {
	Sdk_version     *string
	Min_sdk_version *string
}

// Convenience struct to hold all attributes parsed from linker properties.
type linkerAttributes struct {
	deps                             bazel.LabelListAttribute
	implementationDeps               bazel.LabelListAttribute
	dynamicDeps                      bazel.LabelListAttribute
	implementationDynamicDeps        bazel.LabelListAttribute
	runtimeDeps                      bazel.LabelListAttribute
	wholeArchiveDeps                 bazel.LabelListAttribute
	implementationWholeArchiveDeps   bazel.LabelListAttribute
	systemDynamicDeps                bazel.LabelListAttribute
	usedSystemDynamicDepAsStaticDep  map[string]bool
	usedSystemDynamicDepAsDynamicDep map[string]bool

	useVersionLib                 bazel.BoolAttribute
	linkopts                      bazel.StringListAttribute
	additionalLinkerInputs        bazel.LabelListAttribute
	stripKeepSymbols              bazel.BoolAttribute
	stripKeepSymbolsAndDebugFrame bazel.BoolAttribute
	stripKeepSymbolsList          bazel.StringListAttribute
	stripAll                      bazel.BoolAttribute
	stripNone                     bazel.BoolAttribute
	features                      bazel.StringListAttribute
}

var (
	soongSystemSharedLibs = []string{"libc", "libm", "libdl"}
	versionLib            = "libbuildversion"
)

// resolveTargetApex re-adds the shared and static libs in target.apex.exclude_shared|static_libs props to non-apex variant
// since all libs are already excluded by default
func (la *linkerAttributes) resolveTargetApexProp(ctx android.Bp2buildMutatorContext, props *BaseLinkerProperties) {
	excludeSharedLibs := bazelLabelForSharedDeps(ctx, props.Target.Apex.Exclude_shared_libs)
	sharedExcludes := bazel.LabelList{Excludes: excludeSharedLibs.Includes}
	sharedExcludesLabelList := bazel.LabelListAttribute{}
	sharedExcludesLabelList.SetSelectValue(bazel.InApexAxis, bazel.InApex, sharedExcludes)

	la.dynamicDeps.Append(sharedExcludesLabelList)
	la.implementationDynamicDeps.Append(sharedExcludesLabelList)

	excludeStaticLibs := bazelLabelForStaticDeps(ctx, props.Target.Apex.Exclude_static_libs)
	staticExcludes := bazel.LabelList{Excludes: excludeStaticLibs.Includes}
	staticExcludesLabelList := bazel.LabelListAttribute{}
	staticExcludesLabelList.SetSelectValue(bazel.InApexAxis, bazel.InApex, staticExcludes)

	la.deps.Append(staticExcludesLabelList)
	la.implementationDeps.Append(staticExcludesLabelList)
}

func (la *linkerAttributes) bp2buildForAxisAndConfig(ctx android.Bp2buildMutatorContext, module *Module, axis bazel.ConfigurationAxis, config string, props *BaseLinkerProperties) {
	isBinary := module.Binary()
	// Use a single variable to capture usage of nocrt in arch variants, so there's only 1 error message for this module
	var axisFeatures []string

	wholeStaticLibs := android.FirstUniqueStrings(props.Whole_static_libs)
	staticLibs := android.FirstUniqueStrings(android.RemoveListFromList(props.Static_libs, wholeStaticLibs))
	if axis == bazel.NoConfigAxis {
		la.useVersionLib.SetSelectValue(axis, config, props.Use_version_lib)
		if proptools.Bool(props.Use_version_lib) {
			versionLibAlreadyInDeps := android.InList(versionLib, wholeStaticLibs)
			// remove from static libs so there is no duplicate dependency
			_, staticLibs = android.RemoveFromList(versionLib, staticLibs)
			// only add the dep if it is not in progress
			if !versionLibAlreadyInDeps {
				wholeStaticLibs = append(wholeStaticLibs, versionLib)
			}
		}
	}

	// Excludes to parallel Soong:
	// https://cs.android.com/android/platform/superproject/+/master:build/soong/cc/linker.go;l=247-249;drc=088b53577dde6e40085ffd737a1ae96ad82fc4b0
	la.wholeArchiveDeps.SetSelectValue(axis, config, bazelLabelForWholeDepsExcludes(ctx, wholeStaticLibs, props.Exclude_static_libs))

	if isBinary && module.StaticExecutable() {
		usedSystemStatic := android.FilterListPred(staticLibs, func(s string) bool {
			return android.InList(s, soongSystemSharedLibs) && !android.InList(s, props.Exclude_static_libs)
		})

		for _, el := range usedSystemStatic {
			if la.usedSystemDynamicDepAsStaticDep == nil {
				la.usedSystemDynamicDepAsStaticDep = map[string]bool{}
			}
			la.usedSystemDynamicDepAsStaticDep[el] = true
		}
	}
	staticDeps := maybePartitionExportedAndImplementationsDepsExcludes(
		ctx,
		!isBinary,
		staticLibs,
		props.Exclude_static_libs,
		props.Export_static_lib_headers,
		bazelLabelForStaticDepsExcludes,
	)

	headerLibs := android.FirstUniqueStrings(props.Header_libs)
	hDeps := maybePartitionExportedAndImplementationsDeps(ctx, !isBinary, headerLibs, props.Export_header_lib_headers, bazelLabelForHeaderDeps)

	(&hDeps.export).Append(staticDeps.export)
	la.deps.SetSelectValue(axis, config, hDeps.export)

	(&hDeps.implementation).Append(staticDeps.implementation)
	la.implementationDeps.SetSelectValue(axis, config, hDeps.implementation)

	systemSharedLibs := props.System_shared_libs
	// systemSharedLibs distinguishes between nil/empty list behavior:
	//    nil -> use default values
	//    empty list -> no values specified
	if len(systemSharedLibs) > 0 {
		systemSharedLibs = android.FirstUniqueStrings(systemSharedLibs)
	}
	la.systemDynamicDeps.SetSelectValue(axis, config, bazelLabelForSharedDeps(ctx, systemSharedLibs))

	sharedLibs := android.FirstUniqueStrings(props.Shared_libs)
	excludeSharedLibs := props.Exclude_shared_libs
	usedSystem := android.FilterListPred(sharedLibs, func(s string) bool {
		return android.InList(s, soongSystemSharedLibs) && !android.InList(s, excludeSharedLibs)
	})

	for _, el := range usedSystem {
		if la.usedSystemDynamicDepAsDynamicDep == nil {
			la.usedSystemDynamicDepAsDynamicDep = map[string]bool{}
		}
		la.usedSystemDynamicDepAsDynamicDep[el] = true
	}

	sharedDeps := maybePartitionExportedAndImplementationsDepsExcludes(
		ctx,
		!isBinary,
		sharedLibs,
		props.Exclude_shared_libs,
		props.Export_shared_lib_headers,
		bazelLabelForSharedDepsExcludes,
	)
	la.dynamicDeps.SetSelectValue(axis, config, sharedDeps.export)
	la.implementationDynamicDeps.SetSelectValue(axis, config, sharedDeps.implementation)
	la.resolveTargetApexProp(ctx, props)

	if axis == bazel.NoConfigAxis || (axis == bazel.OsConfigurationAxis && config == bazel.OsAndroid) {
		// If a dependency in la.implementationDynamicDeps or la.dynamicDeps has stubs, its
		// stub variant should be used when the dependency is linked in a APEX. The
		// dependencies in NoConfigAxis and OsConfigurationAxis/OsAndroid are grouped by
		// having stubs or not, so Bazel select() statement can be used to choose
		// source/stub variants of them.
		apexAvailable := module.ApexAvailable()
		SetStubsForDynamicDeps(ctx, axis, config, apexAvailable, sharedDeps.export, &la.dynamicDeps, &la.deps, 0, false)
		SetStubsForDynamicDeps(ctx, axis, config, apexAvailable, sharedDeps.implementation, &la.implementationDynamicDeps, &la.deps, 1, false)
		if len(systemSharedLibs) > 0 {
			SetStubsForDynamicDeps(ctx, axis, config, apexAvailable, bazelLabelForSharedDeps(ctx, systemSharedLibs), &la.systemDynamicDeps, &la.deps, 2, true)
		}
	}

	if !BoolDefault(props.Pack_relocations, packRelocationsDefault) {
		axisFeatures = append(axisFeatures, "disable_pack_relocations")
	}

	if Bool(props.Allow_undefined_symbols) {
		axisFeatures = append(axisFeatures, "-no_undefined_symbols")
	}

	var linkerFlags []string
	if len(props.Ldflags) > 0 {
		linkerFlags = append(linkerFlags, proptools.NinjaEscapeList(props.Ldflags)...)
		// binaries remove static flag if -shared is in the linker flags
		if isBinary && android.InList("-shared", linkerFlags) {
			axisFeatures = append(axisFeatures, "-static_flag")
		}
	}

	if !props.libCrt() {
		axisFeatures = append(axisFeatures, "-use_libcrt")
	}
	if !props.crt() {
		axisFeatures = append(axisFeatures, "-link_crt")
	}

	// This must happen before the addition of flags for Version Script and
	// Dynamic List, as these flags must be split on spaces and those must not
	linkerFlags = parseCommandLineFlags(linkerFlags, filterOutClangUnknownCflags)

	additionalLinkerInputs := bazel.LabelList{}
	if props.Version_script != nil {
		label := android.BazelLabelForModuleSrcSingle(ctx, *props.Version_script)
		additionalLinkerInputs.Add(&label)
		linkerFlags = append(linkerFlags, fmt.Sprintf("-Wl,--version-script,$(location %s)", label.Label))
		axisFeatures = append(axisFeatures, "android_cfi_exports_map")
	}

	if props.Dynamic_list != nil {
		label := android.BazelLabelForModuleSrcSingle(ctx, *props.Dynamic_list)
		additionalLinkerInputs.Add(&label)
		linkerFlags = append(linkerFlags, fmt.Sprintf("-Wl,--dynamic-list,$(location %s)", label.Label))
	}

	la.additionalLinkerInputs.SetSelectValue(axis, config, additionalLinkerInputs)
	if axis == bazel.OsConfigurationAxis && (config == bazel.OsDarwin || config == bazel.OsLinux || config == bazel.OsWindows) {
		linkerFlags = append(linkerFlags, props.Host_ldlibs...)
	}
	la.linkopts.SetSelectValue(axis, config, linkerFlags)

	if axisFeatures != nil {
		la.features.SetSelectValue(axis, config, axisFeatures)
	}

	runtimeDeps := android.BazelLabelForModuleDepsExcludes(ctx, props.Runtime_libs, props.Exclude_runtime_libs)
	if !runtimeDeps.IsEmpty() {
		la.runtimeDeps.SetSelectValue(axis, config, runtimeDeps)
	}
}

var (
	apiSurfaceModuleLibCurrentPackage = "@api_surfaces//" + android.ModuleLibApi.String() + "/current:"
)

func availableToSameApexes(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	differ, _, _ := android.ListSetDifference(a, b)
	return !differ
}

var (
	apiDomainConfigSettingKey  = android.NewOnceKey("apiDomainConfigSettingKey")
	apiDomainConfigSettingLock sync.Mutex
)

func getApiDomainConfigSettingMap(config android.Config) *map[string]bool {
	return config.Once(apiDomainConfigSettingKey, func() interface{} {
		return &map[string]bool{}
	}).(*map[string]bool)
}

var (
	testApexNameToApiDomain = map[string]string{
		"test_broken_com.android.art": "com.android.art",
	}
)

// GetApiDomain returns the canonical name of the apex. This is synonymous to the apex_name definition.
// https://cs.android.com/android/_/android/platform/build/soong/+/e3f0281b8897da1fe23b2f4f3a05f1dc87bcc902:apex/prebuilt.go;l=81-83;drc=2dc7244af985a6ad701b22f1271e606cabba527f;bpv=1;bpt=0
// For test apexes, it uses a naming convention heuristic to determine the api domain.
// TODO (b/281548611): Move this build/soong/android
func GetApiDomain(apexName string) string {
	if apiDomain, exists := testApexNameToApiDomain[apexName]; exists {
		return apiDomain
	}
	// Remove `test_` prefix
	return strings.TrimPrefix(apexName, "test_")
}

// Create a config setting for this apex in build/bazel/rules/apex
// The use case for this is stub/impl selection in cc libraries
// Long term, these config_setting(s) should be colocated with the respective apex definitions.
// Note that this is an anti-pattern: The config_setting should be created from the apex definition
// and not from a cc_library.
// This anti-pattern is needed today since not all apexes have been allowlisted.
func createInApexConfigSetting(ctx android.Bp2buildMutatorContext, apexName string) {
	if apexName == android.AvailableToPlatform || apexName == android.AvailableToAnyApex {
		// These correspond to android-non_apex and android-in_apex
		return
	}
	apiDomainConfigSettingLock.Lock()
	defer apiDomainConfigSettingLock.Unlock()

	// Return if a config_setting has already been created
	apiDomain := GetApiDomain(apexName)
	acsm := getApiDomainConfigSettingMap(ctx.Config())
	if _, exists := (*acsm)[apiDomain]; exists {
		return
	}
	(*acsm)[apiDomain] = true

	csa := bazel.ConfigSettingAttributes{
		Flag_values: bazel.StringMapAttribute{
			"//build/bazel/rules/apex:api_domain": apiDomain,
		},
		// Constraint this to android
		Constraint_values: bazel.MakeLabelListAttribute(
			bazel.MakeLabelList(
				[]bazel.Label{
					bazel.Label{Label: "//build/bazel/platforms/os:android"},
				},
			),
		),
	}
	ca := android.CommonAttributes{
		Name: apiDomain,
	}
	ctx.CreateBazelConfigSetting(
		csa,
		ca,
		"build/bazel/rules/apex",
	)
}

func inApexConfigSetting(apexAvailable string) string {
	if apexAvailable == android.AvailableToPlatform {
		return bazel.AndroidPlatform
	}
	if apexAvailable == android.AvailableToAnyApex {
		return bazel.AndroidAndInApex
	}
	apiDomain := GetApiDomain(apexAvailable)
	return "//build/bazel/rules/apex:" + apiDomain
}

// Inputs to stub vs impl selection.
type stubSelectionInfo struct {
	// Label of the implementation library (e.g. //bionic/libc:libc)
	impl bazel.Label
	// Axis containing the implementation library
	axis bazel.ConfigurationAxis
	// Axis key containing the implementation library
	config string
	// API domain of the apex
	// For test apexes (test_com.android.foo), this will be the source apex (com.android.foo)
	apiDomain string
	// List of dep labels
	dynamicDeps *bazel.LabelListAttribute
	// Boolean value for determining if the dep is in the same api domain
	// If false, the label will be rewritten to to the stub label
	sameApiDomain bool
}

func useStubOrImplInApexWithName(ssi stubSelectionInfo) {
	lib := ssi.impl
	if !ssi.sameApiDomain {
		lib = bazel.Label{
			Label: apiSurfaceModuleLibCurrentPackage + strings.TrimPrefix(lib.OriginalModuleName, ":"),
		}
	}
	// Create a select statement specific to this apex
	inApexSelectValue := ssi.dynamicDeps.SelectValue(bazel.OsAndInApexAxis, inApexConfigSetting(ssi.apiDomain))
	(&inApexSelectValue).Append(bazel.MakeLabelList([]bazel.Label{lib}))
	ssi.dynamicDeps.SetSelectValue(bazel.OsAndInApexAxis, inApexConfigSetting(ssi.apiDomain), bazel.FirstUniqueBazelLabelList(inApexSelectValue))
	// Delete the library from the common config for this apex
	implDynamicDeps := ssi.dynamicDeps.SelectValue(ssi.axis, ssi.config)
	implDynamicDeps = bazel.SubtractBazelLabelList(implDynamicDeps, bazel.MakeLabelList([]bazel.Label{ssi.impl}))
	ssi.dynamicDeps.SetSelectValue(ssi.axis, ssi.config, implDynamicDeps)
	if ssi.axis == bazel.NoConfigAxis {
		// Set defaults. Defaults (i.e. host) should use impl and not stubs.
		defaultSelectValue := ssi.dynamicDeps.SelectValue(bazel.OsAndInApexAxis, bazel.ConditionsDefaultConfigKey)
		(&defaultSelectValue).Append(bazel.MakeLabelList([]bazel.Label{ssi.impl}))
		ssi.dynamicDeps.SetSelectValue(bazel.OsAndInApexAxis, bazel.ConditionsDefaultConfigKey, bazel.FirstUniqueBazelLabelList(defaultSelectValue))
	}
}

// hasNdkStubs returns true for libfoo if there exists a libfoo.ndk of type ndk_library
func hasNdkStubs(ctx android.BazelConversionPathContext, c *Module) bool {
	mod, exists := ctx.ModuleFromName(c.Name() + ndkLibrarySuffix)
	return exists && ctx.OtherModuleType(mod) == "ndk_library"
}

func SetStubsForDynamicDeps(ctx android.Bp2buildMutatorContext, axis bazel.ConfigurationAxis,
	config string, apexAvailable []string, dynamicLibs bazel.LabelList, dynamicDeps *bazel.LabelListAttribute, deps *bazel.LabelListAttribute, ind int, buildNonApexWithStubs bool) {

	// Create a config_setting for each apex_available.
	// This will be used to select impl of a dep if dep is available to the same apex.
	for _, aa := range apexAvailable {
		createInApexConfigSetting(ctx, aa)
	}

	apiDomainForSelects := []string{}
	for _, apex := range apexAvailable {
		apiDomainForSelects = append(apiDomainForSelects, GetApiDomain(apex))
	}
	// Always emit a select statement for the platform variant.
	// This ensures that b build //foo --config=android works
	// Soong always creates a platform variant even when the library might not be available to platform.
	if !android.InList(android.AvailableToPlatform, apiDomainForSelects) {
		apiDomainForSelects = append(apiDomainForSelects, android.AvailableToPlatform)
	}
	apiDomainForSelects = android.SortedUniqueStrings(apiDomainForSelects)

	// Create a select for each apex this library could be included in.
	for _, l := range dynamicLibs.Includes {
		dep, _ := ctx.ModuleFromName(l.OriginalModuleName)
		if c, ok := dep.(*Module); !ok || !c.HasStubsVariants() {
			continue
		}
		// TODO (b/280339069): Decrease the verbosity of the generated BUILD files
		for _, apiDomain := range apiDomainForSelects {
			var sameApiDomain bool
			if apiDomain == android.AvailableToPlatform {
				// Platform variants in Soong use equality of apex_available for stub/impl selection.
				// https://cs.android.com/android/_/android/platform/build/soong/+/316b0158fe57ee7764235923e7c6f3d530da39c6:cc/cc.go;l=3393-3404;drc=176271a426496fa2688efe2b40d5c74340c63375;bpv=1;bpt=0
				// One of the factors behind this design choice is cc_test
				// Tests only have a platform variant, and using equality of apex_available ensures
				// that tests of an apex library gets its implementation and not stubs.
				// TODO (b/280343104): Discuss if we can drop this special handling for platform variants.
				sameApiDomain = availableToSameApexes(apexAvailable, dep.(*Module).ApexAvailable())
				if linkable, ok := ctx.Module().(LinkableInterface); ok && linkable.Bootstrap() {
					sameApiDomain = true
				}
				// If dependency has `apex_available: ["//apex_available:platform]`, then the platform variant of rdep should link against its impl.
				// https://cs.android.com/android/_/android/platform/build/soong/+/main:cc/cc.go;l=3617;bpv=1;bpt=0;drc=c6a93d853b37ec90786e745b8d282145e6d3b589
				if depApexAvailable := dep.(*Module).ApexAvailable(); len(depApexAvailable) == 1 && depApexAvailable[0] == android.AvailableToPlatform {
					sameApiDomain = true
				}
			} else {
				sameApiDomain = android.InList(apiDomain, dep.(*Module).ApexAvailable())
			}
			ssi := stubSelectionInfo{
				impl:          l,
				axis:          axis,
				config:        config,
				apiDomain:     apiDomain,
				dynamicDeps:   dynamicDeps,
				sameApiDomain: sameApiDomain,
			}
			useStubOrImplInApexWithName(ssi)
		}
	}

	// If the library has an sdk variant, create additional selects to build this variant against the ndk
	// The config setting for this variant will be //build/bazel/rules/apex:unbundled_app
	if c, ok := ctx.Module().(*Module); ok && c.Properties.Sdk_version != nil {
		for _, l := range dynamicLibs.Includes {
			dep, _ := ctx.ModuleFromName(l.OriginalModuleName)
			label := l // use the implementation by default
			if depC, ok := dep.(*Module); ok && hasNdkStubs(ctx, depC) {
				// If the dependency has ndk stubs, build against the ndk stubs
				// https://cs.android.com/android/_/android/platform/build/soong/+/main:cc/cc.go;l=2642-2643;drc=e12d252e22dd8afa654325790d3298a0d67bd9d6;bpv=1;bpt=0
				ver := proptools.String(c.Properties.Sdk_version)
				// TODO - b/298085502: Add bp2build support for sdk_version: "minimum"
				ndkLibModule, _ := ctx.ModuleFromName(dep.Name() + ndkLibrarySuffix)
				label = bazel.Label{
					Label: "//" + ctx.OtherModuleDir(ndkLibModule) + ":" + ndkLibModule.Name() + "_stub_libs-" + ver,
				}
			}
			// add the ndk lib label to this axis
			existingValue := dynamicDeps.SelectValue(bazel.OsAndInApexAxis, "unbundled_app")
			existingValue.Append(bazel.MakeLabelList([]bazel.Label{label}))
			dynamicDeps.SetSelectValue(bazel.OsAndInApexAxis, "unbundled_app", bazel.FirstUniqueBazelLabelList(existingValue))
		}

		// Add ndk_sysroot to deps.
		// ndk_sysroot has a dependency edge on all ndk_headers, and will provide the .h files of _every_ ndk library
		existingValue := deps.SelectValue(bazel.OsAndInApexAxis, "unbundled_app")
		existingValue.Append(bazel.MakeLabelList([]bazel.Label{ndkSysrootLabel}))
		deps.SetSelectValue(bazel.OsAndInApexAxis, "unbundled_app", bazel.FirstUniqueBazelLabelList(existingValue))
	}
}

var (
	ndkSysrootLabel = bazel.Label{
		Label: "//build/bazel/rules/cc:ndk_sysroot",
	}
)

func (la *linkerAttributes) convertStripProps(ctx android.BazelConversionPathContext, module *Module) {
	bp2BuildPropParseHelper(ctx, module, &StripProperties{}, func(axis bazel.ConfigurationAxis, config string, props interface{}) {
		if stripProperties, ok := props.(*StripProperties); ok {
			la.stripKeepSymbols.SetSelectValue(axis, config, stripProperties.Strip.Keep_symbols)
			la.stripKeepSymbolsList.SetSelectValue(axis, config, stripProperties.Strip.Keep_symbols_list)
			la.stripKeepSymbolsAndDebugFrame.SetSelectValue(axis, config, stripProperties.Strip.Keep_symbols_and_debug_frame)
			la.stripAll.SetSelectValue(axis, config, stripProperties.Strip.All)
			la.stripNone.SetSelectValue(axis, config, stripProperties.Strip.None)
		}
	})
}

func (la *linkerAttributes) convertProductVariables(ctx android.Bp2buildMutatorContext, productVariableProps android.ProductConfigProperties) {

	type productVarDep struct {
		// the name of the corresponding excludes field, if one exists
		excludesField string
		// reference to the bazel attribute that should be set for the given product variable config
		attribute *bazel.LabelListAttribute

		depResolutionFunc func(ctx android.Bp2buildMutatorContext, modules, excludes []string) bazel.LabelList
	}

	// an intermediate attribute that holds Header_libs info, and will be appended to
	// implementationDeps at the end, to solve the confliction that both header_libs
	// and static_libs use implementationDeps.
	var headerDeps bazel.LabelListAttribute

	productVarToDepFields := map[string]productVarDep{
		// product variables do not support exclude_shared_libs
		"Shared_libs":       {attribute: &la.implementationDynamicDeps, depResolutionFunc: bazelLabelForSharedDepsExcludes},
		"Static_libs":       {"Exclude_static_libs", &la.implementationDeps, bazelLabelForStaticDepsExcludes},
		"Whole_static_libs": {"Exclude_static_libs", &la.wholeArchiveDeps, bazelLabelForWholeDepsExcludes},
		"Header_libs":       {attribute: &headerDeps, depResolutionFunc: bazelLabelForHeaderDepsExcludes},
	}

	for name, dep := range productVarToDepFields {
		props, exists := productVariableProps[name]
		excludeProps, excludesExists := productVariableProps[dep.excludesField]
		// if neither an include nor excludes property exists, then skip it
		if !exists && !excludesExists {
			continue
		}
		// Collect all the configurations that an include or exclude property exists for.
		// We want to iterate all configurations rather than either the include or exclude because, for a
		// particular configuration, we may have either only an include or an exclude to handle.
		productConfigProps := make(map[android.ProductConfigOrSoongConfigProperty]bool, len(props)+len(excludeProps))
		for p := range props {
			productConfigProps[p] = true
		}
		for p := range excludeProps {
			productConfigProps[p] = true
		}

		for productConfigProp := range productConfigProps {
			prop, includesExists := props[productConfigProp]
			excludesProp, excludesExists := excludeProps[productConfigProp]
			var includes, excludes []string
			var ok bool
			// if there was no includes/excludes property, casting fails and that's expected
			if includes, ok = prop.([]string); includesExists && !ok {
				ctx.ModuleErrorf("Could not convert product variable %s property", name)
			}
			if excludes, ok = excludesProp.([]string); excludesExists && !ok {
				ctx.ModuleErrorf("Could not convert product variable %s property", dep.excludesField)
			}

			dep.attribute.EmitEmptyList = productConfigProp.AlwaysEmit()
			dep.attribute.SetSelectValue(
				productConfigProp.ConfigurationAxis(),
				productConfigProp.SelectKey(),
				dep.depResolutionFunc(ctx, android.FirstUniqueStrings(includes), excludes),
			)
		}
	}
	la.implementationDeps.Append(headerDeps)
}

func (la *linkerAttributes) finalize(ctx android.Bp2buildMutatorContext) {
	// if system dynamic deps have the default value, any use of a system dynamic library used will
	// result in duplicate library errors for bionic OSes. Here, we explicitly exclude those libraries
	// from bionic OSes and the no config case as these libraries only build for bionic OSes.
	if la.systemDynamicDeps.IsNil() && len(la.usedSystemDynamicDepAsDynamicDep) > 0 {
		toRemove := bazelLabelForSharedDeps(ctx, android.SortedKeys(la.usedSystemDynamicDepAsDynamicDep))
		la.dynamicDeps.Exclude(bazel.NoConfigAxis, "", toRemove)
		la.dynamicDeps.Exclude(bazel.OsConfigurationAxis, "android", toRemove)
		la.dynamicDeps.Exclude(bazel.OsConfigurationAxis, "linux_bionic", toRemove)
		la.implementationDynamicDeps.Exclude(bazel.NoConfigAxis, "", toRemove)
		la.implementationDynamicDeps.Exclude(bazel.OsConfigurationAxis, "android", toRemove)
		la.implementationDynamicDeps.Exclude(bazel.OsConfigurationAxis, "linux_bionic", toRemove)

		la.implementationDynamicDeps.Exclude(bazel.OsAndInApexAxis, bazel.ConditionsDefaultConfigKey, toRemove)
		stubsToRemove := make([]bazel.Label, 0, len(la.usedSystemDynamicDepAsDynamicDep))
		for _, lib := range toRemove.Includes {
			stubLabelInApiSurfaces := bazel.Label{
				Label: apiSurfaceModuleLibCurrentPackage + lib.OriginalModuleName,
			}
			stubsToRemove = append(stubsToRemove, stubLabelInApiSurfaces)
		}
		// system libraries (e.g. libc, libm, libdl) belong the com.android.runtime api domain
		// dedupe the stubs of these libraries from the other api domains (platform, other_apexes...)
		for _, aa := range ctx.Module().(*Module).ApexAvailable() {
			la.implementationDynamicDeps.Exclude(bazel.OsAndInApexAxis, inApexConfigSetting(aa), bazel.MakeLabelList(stubsToRemove))
		}
		la.implementationDynamicDeps.Exclude(bazel.OsAndInApexAxis, bazel.AndroidPlatform, bazel.MakeLabelList(stubsToRemove))
	}
	if la.systemDynamicDeps.IsNil() && len(la.usedSystemDynamicDepAsStaticDep) > 0 {
		toRemove := bazelLabelForStaticDeps(ctx, android.SortedKeys(la.usedSystemDynamicDepAsStaticDep))
		la.deps.Exclude(bazel.NoConfigAxis, "", toRemove)
		la.deps.Exclude(bazel.OsConfigurationAxis, "android", toRemove)
		la.deps.Exclude(bazel.OsConfigurationAxis, "linux_bionic", toRemove)
		la.implementationDeps.Exclude(bazel.NoConfigAxis, "", toRemove)
		la.implementationDeps.Exclude(bazel.OsConfigurationAxis, "android", toRemove)
		la.implementationDeps.Exclude(bazel.OsConfigurationAxis, "linux_bionic", toRemove)
	}

	la.deps.ResolveExcludes()
	la.implementationDeps.ResolveExcludes()
	la.dynamicDeps.ResolveExcludes()
	la.implementationDynamicDeps.ResolveExcludes()
	la.wholeArchiveDeps.ResolveExcludes()
	la.systemDynamicDeps.ForceSpecifyEmptyList = true

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
	AbsoluteIncludes bazel.StringListAttribute
	Includes         bazel.StringListAttribute
	SystemIncludes   bazel.StringListAttribute
}

func bp2BuildParseExportedIncludes(ctx android.BazelConversionPathContext, module *Module, includes *BazelIncludes) BazelIncludes {
	var exported BazelIncludes
	if includes != nil {
		exported = *includes
	} else {
		exported = BazelIncludes{}
	}

	// cc library Export_include_dirs and Export_system_include_dirs are marked
	// "variant_prepend" in struct tag, set their prepend property to true to make
	// sure bp2build generates correct result.
	exported.Includes.Prepend = true
	exported.SystemIncludes.Prepend = true

	bp2BuildPropParseHelper(ctx, module, &FlagExporterProperties{}, func(axis bazel.ConfigurationAxis, config string, props interface{}) {
		if flagExporterProperties, ok := props.(*FlagExporterProperties); ok {
			if len(flagExporterProperties.Export_include_dirs) > 0 {
				exported.Includes.SetSelectValue(axis, config, android.FirstUniqueStrings(append(exported.Includes.SelectValue(axis, config), flagExporterProperties.Export_include_dirs...)))
			}
			if len(flagExporterProperties.Export_system_include_dirs) > 0 {
				exported.SystemIncludes.SetSelectValue(axis, config, android.FirstUniqueStrings(append(exported.SystemIncludes.SelectValue(axis, config), flagExporterProperties.Export_system_include_dirs...)))
			}
		}
	})
	exported.AbsoluteIncludes.DeduplicateAxesFromBase()
	exported.Includes.DeduplicateAxesFromBase()
	exported.SystemIncludes.DeduplicateAxesFromBase()

	return exported
}

func BazelLabelNameForStaticModule(baseLabel string) string {
	return baseLabel + "_bp2build_cc_library_static"
}

func bazelLabelForStaticModule(ctx android.BazelConversionPathContext, m blueprint.Module) string {
	label := android.BazelModuleLabel(ctx, m)
	if ccModule, ok := m.(*Module); ok && ccModule.typ() == fullLibrary {
		return BazelLabelNameForStaticModule(label)
	}
	return label
}

func bazelLabelForSharedModule(ctx android.BazelConversionPathContext, m blueprint.Module) string {
	// cc_library, at it's root name, propagates the shared library, which depends on the static
	// library.
	return android.BazelModuleLabel(ctx, m)
}

func bazelLabelForStaticWholeModuleDeps(ctx android.BazelConversionPathContext, m blueprint.Module) string {
	label := bazelLabelForStaticModule(ctx, m)
	if aModule, ok := m.(android.Module); ok {
		if android.IsModulePrebuilt(aModule) {
			label += "_alwayslink"
		}
	}
	return label
}

func xsdConfigCppTarget(xsd android.XsdConfigBp2buildTargets) string {
	return xsd.CppBp2buildTargetName()
}

func bazelLabelForWholeDeps(ctx android.Bp2buildMutatorContext, modules []string) bazel.LabelList {
	return android.BazelLabelForModuleDepsWithFn(ctx, modules, bazelLabelForStaticWholeModuleDeps, /*markAsDeps=*/true)
}

func bazelLabelForWholeDepsExcludes(ctx android.Bp2buildMutatorContext, modules, excludes []string) bazel.LabelList {
	return android.BazelLabelForModuleDepsExcludesWithFn(ctx, modules, excludes, bazelLabelForStaticWholeModuleDeps)
}

func bazelLabelForStaticDepsExcludes(ctx android.Bp2buildMutatorContext, modules, excludes []string) bazel.LabelList {
	return android.BazelLabelForModuleDepsExcludesWithFn(ctx, modules, excludes, bazelLabelForStaticModule)
}

func bazelLabelForStaticDeps(ctx android.Bp2buildMutatorContext, modules []string) bazel.LabelList {
	return android.BazelLabelForModuleDepsWithFn(ctx, modules, bazelLabelForStaticModule, /*markAsDeps=*/true)
}

func bazelLabelForSharedDeps(ctx android.Bp2buildMutatorContext, modules []string) bazel.LabelList {
	return android.BazelLabelForModuleDepsWithFn(ctx, modules, bazelLabelForSharedModule, /*markAsDeps=*/true)
}

func bazelLabelForHeaderDeps(ctx android.Bp2buildMutatorContext, modules []string) bazel.LabelList {
	// This is not elegant, but bp2build's shared library targets only propagate
	// their header information as part of the normal C++ provider.
	return bazelLabelForSharedDeps(ctx, modules)
}

func bazelLabelForHeaderDepsExcludes(ctx android.Bp2buildMutatorContext, modules, excludes []string) bazel.LabelList {
	// This is only used when product_variable header_libs is processed, to follow
	// the pattern of depResolutionFunc
	return android.BazelLabelForModuleDepsExcludesWithFn(ctx, modules, excludes, bazelLabelForSharedModule)
}

func bazelLabelForSharedDepsExcludes(ctx android.Bp2buildMutatorContext, modules, excludes []string) bazel.LabelList {
	return android.BazelLabelForModuleDepsExcludesWithFn(ctx, modules, excludes, bazelLabelForSharedModule)
}

type binaryLinkerAttrs struct {
	Linkshared *bool
	Stem       bazel.StringAttribute
	Suffix     bazel.StringAttribute
}

func bp2buildBinaryLinkerProps(ctx android.BazelConversionPathContext, m *Module) binaryLinkerAttrs {
	attrs := binaryLinkerAttrs{}
	bp2BuildPropParseHelper(ctx, m, &BinaryLinkerProperties{}, func(axis bazel.ConfigurationAxis, config string, props interface{}) {
		linkerProps := props.(*BinaryLinkerProperties)
		staticExecutable := linkerProps.Static_executable
		if axis == bazel.NoConfigAxis {
			if linkBinaryShared := !proptools.Bool(staticExecutable); !linkBinaryShared {
				attrs.Linkshared = &linkBinaryShared
			}
		} else if staticExecutable != nil {
			// TODO(b/202876379): Static_executable is arch-variant; however, linkshared is a
			// nonconfigurable attribute. Only 4 AOSP modules use this feature, defer handling
			ctx.ModuleErrorf("bp2build cannot migrate a module with arch/target-specific static_executable values")
		}
		if stem := linkerProps.Stem; stem != nil {
			attrs.Stem.SetSelectValue(axis, config, stem)
		}
		if suffix := linkerProps.Suffix; suffix != nil {
			attrs.Suffix.SetSelectValue(axis, config, suffix)
		}
	})

	return attrs
}

type sanitizerValues struct {
	features                 bazel.StringListAttribute
	copts                    bazel.StringListAttribute
	additionalCompilerInputs bazel.LabelListAttribute
}

func bp2buildSanitizerFeatures(ctx android.BazelConversionPathContext, m *Module) sanitizerValues {
	sanitizerFeatures := bazel.StringListAttribute{}
	sanitizerCopts := bazel.StringListAttribute{}
	sanitizerCompilerInputs := bazel.LabelListAttribute{}
	memtagFeatures := bazel.StringListAttribute{}
	memtagFeature := ""
	bp2BuildPropParseHelper(ctx, m, &SanitizeProperties{}, func(axis bazel.ConfigurationAxis, config string, props interface{}) {
		var features []string
		if sanitizerProps, ok := props.(*SanitizeProperties); ok {
			if sanitizerProps.Sanitize.Integer_overflow != nil && *sanitizerProps.Sanitize.Integer_overflow {
				features = append(features, "ubsan_integer_overflow")
			}
			for _, sanitizer := range sanitizerProps.Sanitize.Misc_undefined {
				features = append(features, "ubsan_"+sanitizer)
			}
			blocklist := sanitizerProps.Sanitize.Blocklist
			if blocklist != nil {
				// TODO: b/294868620 - Change this not to use the special axis when completing the bug
				coptValue := fmt.Sprintf("-fsanitize-ignorelist=$(location %s)", *blocklist)
				sanitizerCopts.SetSelectValue(bazel.SanitizersEnabledAxis, bazel.SanitizersEnabled, []string{coptValue})
				sanitizerCompilerInputs.SetSelectValue(bazel.SanitizersEnabledAxis, bazel.SanitizersEnabled, bazel.MakeLabelListFromTargetNames([]string{*blocklist}))
			}
			if sanitizerProps.Sanitize.Cfi != nil && !proptools.Bool(sanitizerProps.Sanitize.Cfi) {
				features = append(features, "-android_cfi")
			} else if proptools.Bool(sanitizerProps.Sanitize.Cfi) {
				features = append(features, "android_cfi")
				if proptools.Bool(sanitizerProps.Sanitize.Config.Cfi_assembly_support) {
					features = append(features, "android_cfi_assembly_support")
				}
			}

			if sanitizerProps.Sanitize.Memtag_heap != nil {
				if (axis == bazel.NoConfigAxis && memtagFeature == "") ||
					(axis == bazel.OsArchConfigurationAxis && config == bazel.OsArchAndroidArm64) {
					memtagFeature = setMemtagValue(sanitizerProps, &memtagFeatures)
				}
			}
			sanitizerFeatures.SetSelectValue(axis, config, features)
		}
	})
	sanitizerFeatures.Append(memtagFeatures)

	return sanitizerValues{
		features:                 sanitizerFeatures,
		copts:                    sanitizerCopts,
		additionalCompilerInputs: sanitizerCompilerInputs,
	}
}

func setMemtagValue(sanitizerProps *SanitizeProperties, memtagFeatures *bazel.StringListAttribute) string {
	var features []string
	if proptools.Bool(sanitizerProps.Sanitize.Memtag_heap) {
		features = append(features, "memtag_heap")
	} else {
		features = append(features, "-memtag_heap")
	}
	// Logic comes from: https://cs.android.com/android/platform/superproject/main/+/32ea1afbd1148b0b78553f24fa61116c999eb968:build/soong/cc/sanitize.go;l=910
	if sanitizerProps.Sanitize.Diag.Memtag_heap != nil {
		if proptools.Bool(sanitizerProps.Sanitize.Diag.Memtag_heap) {
			features = append(features, "diag_memtag_heap")
		} else {
			features = append(features, "-diag_memtag_heap")
		}
	}
	memtagFeatures.SetSelectValue(bazel.OsArchConfigurationAxis, bazel.OsArchAndroidArm64, features)

	return features[0]
}

func bp2buildLtoFeatures(ctx android.BazelConversionPathContext, m *Module) bazel.StringListAttribute {
	lto_feature_name := "android_thin_lto"
	ltoBoolFeatures := bazel.BoolAttribute{}
	bp2BuildPropParseHelper(ctx, m, &LTOProperties{}, func(axis bazel.ConfigurationAxis, config string, props interface{}) {
		if ltoProps, ok := props.(*LTOProperties); ok {
			thinProp := ltoProps.Lto.Thin != nil && *ltoProps.Lto.Thin
			thinPropSetToFalse := ltoProps.Lto.Thin != nil && !*ltoProps.Lto.Thin
			neverProp := ltoProps.Lto.Never != nil && *ltoProps.Lto.Never
			if thinProp {
				ltoBoolFeatures.SetSelectValue(axis, config, BoolPtr(true))
				return
			}
			if neverProp || thinPropSetToFalse {
				if thinProp {
					ctx.ModuleErrorf("lto.thin and lto.never are mutually exclusive but were specified together")
				} else {
					ltoBoolFeatures.SetSelectValue(axis, config, BoolPtr(false))
				}
				return
			}
		}
		ltoBoolFeatures.SetSelectValue(axis, config, nil)
	})

	props := m.GetArchVariantProperties(ctx, &LTOProperties{})
	ltoStringFeatures, err := ltoBoolFeatures.ToStringListAttribute(func(boolPtr *bool, axis bazel.ConfigurationAxis, config string) []string {
		if boolPtr == nil {
			return []string{}
		}
		if !*boolPtr {
			return []string{"-" + lto_feature_name}
		}
		features := []string{lto_feature_name}
		if ltoProps, ok := props[axis][config].(*LTOProperties); ok {
			if ltoProps.Whole_program_vtables != nil && *ltoProps.Whole_program_vtables {
				features = append(features, "android_thin_lto_whole_program_vtables")
			}
		}
		return features
	})
	if err != nil {
		ctx.ModuleErrorf("Error processing LTO attributes: %s", err)
	}
	return ltoStringFeatures
}

func convertHiddenVisibilityToFeatureBase(ctx android.BazelConversionPathContext, m *Module) bazel.StringListAttribute {
	visibilityHiddenFeature := bazel.StringListAttribute{}
	bp2BuildPropParseHelper(ctx, m, &BaseCompilerProperties{}, func(axis bazel.ConfigurationAxis, configString string, props interface{}) {
		if baseCompilerProps, ok := props.(*BaseCompilerProperties); ok {
			convertHiddenVisibilityToFeatureHelper(&visibilityHiddenFeature, axis, configString, baseCompilerProps.Cflags)
		}
	})
	return visibilityHiddenFeature
}

func convertHiddenVisibilityToFeatureStaticOrShared(ctx android.BazelConversionPathContext, m *Module, isStatic bool) bazel.StringListAttribute {
	visibilityHiddenFeature := bazel.StringListAttribute{}
	if isStatic {
		bp2BuildPropParseHelper(ctx, m, &StaticProperties{}, func(axis bazel.ConfigurationAxis, configString string, props interface{}) {
			if staticProps, ok := props.(*StaticProperties); ok {
				convertHiddenVisibilityToFeatureHelper(&visibilityHiddenFeature, axis, configString, staticProps.Static.Cflags)
			}
		})
	} else {
		bp2BuildPropParseHelper(ctx, m, &SharedProperties{}, func(axis bazel.ConfigurationAxis, configString string, props interface{}) {
			if sharedProps, ok := props.(*SharedProperties); ok {
				convertHiddenVisibilityToFeatureHelper(&visibilityHiddenFeature, axis, configString, sharedProps.Shared.Cflags)
			}
		})
	}

	return visibilityHiddenFeature
}

func convertHiddenVisibilityToFeatureHelper(feature *bazel.StringListAttribute, axis bazel.ConfigurationAxis, configString string, cflags []string) {
	if inList(config.VisibilityHiddenFlag, cflags) {
		feature.SetSelectValue(axis, configString, []string{"visibility_hidden"})
	}
}
