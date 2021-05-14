// Copyright (C) 2021 The Android Open Source Project
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

package java

import (
	"strings"

	"android/soong/android"
	"github.com/google/blueprint"
)

// Contains support for processing hiddenAPI in a modular fashion.

type hiddenAPIStubsDependencyTag struct {
	blueprint.BaseDependencyTag
	sdkKind android.SdkKind
}

func (b hiddenAPIStubsDependencyTag) ExcludeFromApexContents() {
}

func (b hiddenAPIStubsDependencyTag) ReplaceSourceWithPrebuilt() bool {
	return false
}

func (b hiddenAPIStubsDependencyTag) SdkMemberType(child android.Module) android.SdkMemberType {
	// If the module is a java_sdk_library then treat it as if it was specific in the java_sdk_libs
	// property, otherwise treat if it was specified in the java_header_libs property.
	if javaSdkLibrarySdkMemberType.IsInstance(child) {
		return javaSdkLibrarySdkMemberType
	}

	return javaHeaderLibsSdkMemberType
}

func (b hiddenAPIStubsDependencyTag) ExportMember() bool {
	// Export the module added via this dependency tag from the sdk.
	return true
}

// Avoid having to make stubs content explicitly visible to dependent modules.
//
// This is a temporary workaround to make it easier to migrate to bootclasspath_fragment modules
// with proper dependencies.
// TODO(b/177892522): Remove this and add needed visibility.
func (b hiddenAPIStubsDependencyTag) ExcludeFromVisibilityEnforcement() {
}

var _ android.ExcludeFromVisibilityEnforcementTag = hiddenAPIStubsDependencyTag{}
var _ android.ReplaceSourceWithPrebuilt = hiddenAPIStubsDependencyTag{}
var _ android.ExcludeFromApexContentsTag = hiddenAPIStubsDependencyTag{}
var _ android.SdkMemberTypeDependencyTag = hiddenAPIStubsDependencyTag{}

// hiddenAPIRelevantSdkKinds lists all the android.SdkKind instances that are needed by the hidden
// API processing.
var hiddenAPIRelevantSdkKinds = []android.SdkKind{
	android.SdkPublic,
	android.SdkSystem,
	android.SdkTest,
	android.SdkCorePlatform,
}

// hiddenAPIComputeMonolithicStubLibModules computes the set of module names that provide stubs
// needed to produce the hidden API monolithic stub flags file.
func hiddenAPIComputeMonolithicStubLibModules(config android.Config) map[android.SdkKind][]string {
	var publicStubModules []string
	var systemStubModules []string
	var testStubModules []string
	var corePlatformStubModules []string

	if config.AlwaysUsePrebuiltSdks() {
		// Build configuration mandates using prebuilt stub modules
		publicStubModules = append(publicStubModules, "sdk_public_current_android")
		systemStubModules = append(systemStubModules, "sdk_system_current_android")
		testStubModules = append(testStubModules, "sdk_test_current_android")
	} else {
		// Use stub modules built from source
		publicStubModules = append(publicStubModules, "android_stubs_current")
		systemStubModules = append(systemStubModules, "android_system_stubs_current")
		testStubModules = append(testStubModules, "android_test_stubs_current")
	}
	// We do not have prebuilts of the core platform api yet
	corePlatformStubModules = append(corePlatformStubModules, "legacy.core.platform.api.stubs")

	// Allow products to define their own stubs for custom product jars that apps can use.
	publicStubModules = append(publicStubModules, config.ProductHiddenAPIStubs()...)
	systemStubModules = append(systemStubModules, config.ProductHiddenAPIStubsSystem()...)
	testStubModules = append(testStubModules, config.ProductHiddenAPIStubsTest()...)
	if config.IsEnvTrue("EMMA_INSTRUMENT") {
		publicStubModules = append(publicStubModules, "jacoco-stubs")
	}

	m := map[android.SdkKind][]string{}
	m[android.SdkPublic] = publicStubModules
	m[android.SdkSystem] = systemStubModules
	m[android.SdkTest] = testStubModules
	m[android.SdkCorePlatform] = corePlatformStubModules
	return m
}

// hiddenAPIAddStubLibDependencies adds dependencies onto the modules specified in
// sdkKindToStubLibModules. It adds them in a well known order and uses an SdkKind specific tag to
// identify the source of the dependency.
func hiddenAPIAddStubLibDependencies(ctx android.BottomUpMutatorContext, sdkKindToStubLibModules map[android.SdkKind][]string) {
	module := ctx.Module()
	for _, sdkKind := range hiddenAPIRelevantSdkKinds {
		modules := sdkKindToStubLibModules[sdkKind]
		ctx.AddDependency(module, hiddenAPIStubsDependencyTag{sdkKind: sdkKind}, modules...)
	}
}

// hiddenAPIGatherStubLibDexJarPaths gathers the paths to the dex jars from the dependencies added
// in hiddenAPIAddStubLibDependencies.
func hiddenAPIGatherStubLibDexJarPaths(ctx android.ModuleContext, contents []android.Module) map[android.SdkKind]android.Paths {
	m := map[android.SdkKind]android.Paths{}

	// If the contents includes any java_sdk_library modules then add them to the stubs.
	for _, module := range contents {
		if _, ok := module.(SdkLibraryDependency); ok {
			for _, kind := range []android.SdkKind{android.SdkPublic, android.SdkSystem, android.SdkTest} {
				dexJar := hiddenAPIRetrieveDexJarBuildPath(ctx, module, kind)
				if dexJar != nil {
					m[kind] = append(m[kind], dexJar)
				}
			}
		}
	}

	ctx.VisitDirectDepsIf(isActiveModule, func(module android.Module) {
		tag := ctx.OtherModuleDependencyTag(module)
		if hiddenAPIStubsTag, ok := tag.(hiddenAPIStubsDependencyTag); ok {
			kind := hiddenAPIStubsTag.sdkKind
			dexJar := hiddenAPIRetrieveDexJarBuildPath(ctx, module, kind)
			if dexJar != nil {
				m[kind] = append(m[kind], dexJar)
			}
		}
	})

	// Normalize the paths, i.e. remove duplicates and sort.
	for k, v := range m {
		m[k] = android.SortedUniquePaths(v)
	}

	return m
}

// hiddenAPIRetrieveDexJarBuildPath retrieves the DexJarBuildPath from the specified module, if
// available, or reports an error.
func hiddenAPIRetrieveDexJarBuildPath(ctx android.ModuleContext, module android.Module, kind android.SdkKind) android.Path {
	var dexJar android.Path
	if sdkLibrary, ok := module.(SdkLibraryDependency); ok {
		dexJar = sdkLibrary.SdkApiStubDexJar(ctx, kind)
	} else if j, ok := module.(UsesLibraryDependency); ok {
		dexJar = j.DexJarBuildPath()
	} else {
		ctx.ModuleErrorf("dependency %s of module type %s does not support providing a dex jar", module, ctx.OtherModuleType(module))
		return nil
	}

	if dexJar == nil {
		ctx.ModuleErrorf("dependency %s does not provide a dex jar, consider setting compile_dex: true", module)
	}
	return dexJar
}

var sdkKindToHiddenapiListOption = map[android.SdkKind]string{
	android.SdkPublic:       "public-stub-classpath",
	android.SdkSystem:       "system-stub-classpath",
	android.SdkTest:         "test-stub-classpath",
	android.SdkCorePlatform: "core-platform-stub-classpath",
}

// ruleToGenerateHiddenAPIStubFlagsFile creates a rule to create a hidden API stub flags file.
//
// The rule is initialized but not built so that the caller can modify it and select an appropriate
// name.
func ruleToGenerateHiddenAPIStubFlagsFile(ctx android.BuilderContext, outputPath android.WritablePath, bootDexJars android.Paths, sdkKindToPathList map[android.SdkKind]android.Paths) *android.RuleBuilder {
	// Singleton rule which applies hiddenapi on all boot class path dex files.
	rule := android.NewRuleBuilder(pctx, ctx)

	tempPath := tempPathForRestat(ctx, outputPath)

	command := rule.Command().
		Tool(ctx.Config().HostToolPath(ctx, "hiddenapi")).
		Text("list").
		FlagForEachInput("--boot-dex=", bootDexJars)

	// Iterate over the sdk kinds in a fixed order.
	for _, sdkKind := range hiddenAPIRelevantSdkKinds {
		paths := sdkKindToPathList[sdkKind]
		if len(paths) > 0 {
			option := sdkKindToHiddenapiListOption[sdkKind]
			command.FlagWithInputList("--"+option+"=", paths, ":")
		}
	}

	// Add the output path.
	command.FlagWithOutput("--out-api-flags=", tempPath)

	commitChangeForRestat(rule, tempPath, outputPath)
	return rule
}

// HiddenAPIFlagFileProperties contains paths to the flag files that can be used to augment the
// information obtained from annotations within the source code in order to create the complete set
// of flags that should be applied to the dex implementation jars on the bootclasspath.
//
// Each property contains a list of paths. With the exception of the Unsupported_packages the paths
// of each property reference a plain text file that contains a java signature per line. The flags
// for each of those signatures will be updated in a property specific way.
//
// The Unsupported_packages property contains a list of paths, each of which is a plain text file
// with one Java package per line. All members of all classes within that package (but not nested
// packages) will be updated in a property specific way.
type HiddenAPIFlagFileProperties struct {
	// Marks each signature in the referenced files as being unsupported.
	Unsupported []string `android:"path"`

	// Marks each signature in the referenced files as being unsupported because it has been removed.
	// Any conflicts with other flags are ignored.
	Removed []string `android:"path"`

	// Marks each signature in the referenced files as being supported only for targetSdkVersion <= R
	// and low priority.
	Max_target_r_low_priority []string `android:"path"`

	// Marks each signature in the referenced files as being supported only for targetSdkVersion <= Q.
	Max_target_q []string `android:"path"`

	// Marks each signature in the referenced files as being supported only for targetSdkVersion <= P.
	Max_target_p []string `android:"path"`

	// Marks each signature in the referenced files as being supported only for targetSdkVersion <= O
	// and low priority. Any conflicts with other flags are ignored.
	Max_target_o_low_priority []string `android:"path"`

	// Marks each signature in the referenced files as being blocked.
	Blocked []string `android:"path"`

	// Marks each signature in every package in the referenced files as being unsupported.
	Unsupported_packages []string `android:"path"`
}

func (p *HiddenAPIFlagFileProperties) hiddenAPIFlagFileInfo(ctx android.ModuleContext) hiddenAPIFlagFileInfo {
	info := hiddenAPIFlagFileInfo{categoryToPaths: map[*hiddenAPIFlagFileCategory]android.Paths{}}
	for _, category := range hiddenAPIFlagFileCategories {
		paths := android.PathsForModuleSrc(ctx, category.propertyValueReader(p))
		info.categoryToPaths[category] = paths
	}
	return info
}

type hiddenAPIFlagFileCategory struct {
	// propertyName is the name of the property for this category.
	propertyName string

	// propertyValueReader retrieves the value of the property for this category from the set of
	// properties.
	propertyValueReader func(properties *HiddenAPIFlagFileProperties) []string

	// commandMutator adds the appropriate command line options for this category to the supplied
	// command
	commandMutator func(command *android.RuleBuilderCommand, path android.Path)
}

var hiddenAPIFlagFileCategories = []*hiddenAPIFlagFileCategory{
	// See HiddenAPIFlagFileProperties.Unsupported
	{
		propertyName: "unsupported",
		propertyValueReader: func(properties *HiddenAPIFlagFileProperties) []string {
			return properties.Unsupported
		},
		commandMutator: func(command *android.RuleBuilderCommand, path android.Path) {
			command.FlagWithInput("--unsupported ", path)
		},
	},
	// See HiddenAPIFlagFileProperties.Removed
	{
		propertyName: "removed",
		propertyValueReader: func(properties *HiddenAPIFlagFileProperties) []string {
			return properties.Removed
		},
		commandMutator: func(command *android.RuleBuilderCommand, path android.Path) {
			command.FlagWithInput("--unsupported ", path).Flag("--ignore-conflicts ").FlagWithArg("--tag ", "removed")
		},
	},
	// See HiddenAPIFlagFileProperties.Max_target_r_low_priority
	{
		propertyName: "max_target_r_low_priority",
		propertyValueReader: func(properties *HiddenAPIFlagFileProperties) []string {
			return properties.Max_target_r_low_priority
		},
		commandMutator: func(command *android.RuleBuilderCommand, path android.Path) {
			command.FlagWithInput("--max-target-r ", path).FlagWithArg("--tag ", "lo-prio")
		},
	},
	// See HiddenAPIFlagFileProperties.Max_target_q
	{
		propertyName: "max_target_q",
		propertyValueReader: func(properties *HiddenAPIFlagFileProperties) []string {
			return properties.Max_target_q
		},
		commandMutator: func(command *android.RuleBuilderCommand, path android.Path) {
			command.FlagWithInput("--max-target-q ", path)
		},
	},
	// See HiddenAPIFlagFileProperties.Max_target_p
	{
		propertyName: "max_target_p",
		propertyValueReader: func(properties *HiddenAPIFlagFileProperties) []string {
			return properties.Max_target_p
		},
		commandMutator: func(command *android.RuleBuilderCommand, path android.Path) {
			command.FlagWithInput("--max-target-p ", path)
		},
	},
	// See HiddenAPIFlagFileProperties.Max_target_o_low_priority
	{
		propertyName: "max_target_o_low_priority",
		propertyValueReader: func(properties *HiddenAPIFlagFileProperties) []string {
			return properties.Max_target_o_low_priority
		},
		commandMutator: func(command *android.RuleBuilderCommand, path android.Path) {
			command.FlagWithInput("--max-target-o ", path).Flag("--ignore-conflicts ").FlagWithArg("--tag ", "lo-prio")
		},
	},
	// See HiddenAPIFlagFileProperties.Blocked
	{
		propertyName: "blocked",
		propertyValueReader: func(properties *HiddenAPIFlagFileProperties) []string {
			return properties.Blocked
		},
		commandMutator: func(command *android.RuleBuilderCommand, path android.Path) {
			command.FlagWithInput("--blocked ", path)
		},
	},
	// See HiddenAPIFlagFileProperties.Unsupported_packages
	{
		propertyName: "unsupported_packages",
		propertyValueReader: func(properties *HiddenAPIFlagFileProperties) []string {
			return properties.Unsupported_packages
		},
		commandMutator: func(command *android.RuleBuilderCommand, path android.Path) {
			command.FlagWithInput("--unsupported ", path).Flag("--packages ")
		},
	},
}

// hiddenAPIFlagFileInfo contains paths resolved from HiddenAPIFlagFileProperties and also generated
// by hidden API processing.
//
// This is used both for an individual bootclasspath_fragment to provide it to other modules and
// for a module to collate the files from the fragments it depends upon. That is why the fields are
// all Paths even though they are initialized with a single path.
type hiddenAPIFlagFileInfo struct {
	// categoryToPaths maps from the flag file category to the paths containing information for that
	// category.
	categoryToPaths map[*hiddenAPIFlagFileCategory]android.Paths

	// The paths to the generated stub-flags.csv files.
	StubFlagsPaths android.Paths

	// The paths to the generated annotation-flags.csv files.
	AnnotationFlagsPaths android.Paths

	// The paths to the generated metadata.csv files.
	MetadataPaths android.Paths

	// The paths to the generated index.csv files.
	IndexPaths android.Paths

	// The paths to the generated all-flags.csv files.
	AllFlagsPaths android.Paths
}

func (i *hiddenAPIFlagFileInfo) append(other hiddenAPIFlagFileInfo) {
	for _, category := range hiddenAPIFlagFileCategories {
		i.categoryToPaths[category] = append(i.categoryToPaths[category], other.categoryToPaths[category]...)
	}
	i.StubFlagsPaths = append(i.StubFlagsPaths, other.StubFlagsPaths...)
	i.AnnotationFlagsPaths = append(i.AnnotationFlagsPaths, other.AnnotationFlagsPaths...)
	i.MetadataPaths = append(i.MetadataPaths, other.MetadataPaths...)
	i.IndexPaths = append(i.IndexPaths, other.IndexPaths...)
	i.AllFlagsPaths = append(i.AllFlagsPaths, other.AllFlagsPaths...)
}

var hiddenAPIFlagFileInfoProvider = blueprint.NewProvider(hiddenAPIFlagFileInfo{})

// pathForValidation creates a path of the same type as the supplied type but with a name of
// <path>.valid.
//
// e.g. If path is an OutputPath for out/soong/hiddenapi/hiddenapi-flags.csv then this will return
// an OutputPath for out/soong/hiddenapi/hiddenapi-flags.csv.valid
func pathForValidation(ctx android.PathContext, path android.WritablePath) android.WritablePath {
	extWithoutLeadingDot := strings.TrimPrefix(path.Ext(), ".")
	return path.ReplaceExtension(ctx, extWithoutLeadingDot+".valid")
}

// buildRuleToGenerateHiddenApiFlags creates a rule to create the monolithic hidden API flags from
// the flags from all the modules, the stub flags, augmented with some additional configuration
// files.
//
// baseFlagsPath is the path to the flags file containing all the information from the stubs plus
// an entry for every single member in the dex implementation jars of the individual modules. Every
// signature in any of the other files MUST be included in this file.
//
// moduleSpecificFlagsPaths are the paths to the flags files generated by each module using
// information from the baseFlagsPath as well as from annotations within the source.
//
// augmentationInfo is a struct containing paths to files that augment the information provided by
// the moduleSpecificFlagsPaths.
func buildRuleToGenerateHiddenApiFlags(ctx android.BuilderContext, name, desc string, outputPath android.WritablePath, baseFlagsPath android.Path, moduleSpecificFlagsPaths android.Paths, flagFileInfo *hiddenAPIFlagFileInfo) {

	// The file which is used to record that the flags file is valid.
	var validFile android.WritablePath

	// If there are flag files that have been generated by fragments on which this depends then use
	// them to validate the flag file generated by the rules created by this method.
	if allFlagsPaths := flagFileInfo.AllFlagsPaths; len(allFlagsPaths) > 0 {
		// The flags file generated by the rule created by this method needs to be validated to ensure
		// that it is consistent with the flag files generated by the individual fragments.

		validFile = pathForValidation(ctx, outputPath)

		// Create a rule to validate the output from the following rule.
		rule := android.NewRuleBuilder(pctx, ctx)
		rule.Command().
			BuiltTool("verify_overlaps").
			Input(outputPath).
			Inputs(allFlagsPaths).
			// If validation passes then update the file that records that.
			Text("&& touch").Output(validFile)
		rule.Build(name+"Validation", desc+" validation")
	}

	// Create the rule that will generate the flag files.
	tempPath := tempPathForRestat(ctx, outputPath)
	rule := android.NewRuleBuilder(pctx, ctx)
	command := rule.Command().
		BuiltTool("generate_hiddenapi_lists").
		FlagWithInput("--csv ", baseFlagsPath).
		Inputs(moduleSpecificFlagsPaths).
		FlagWithOutput("--output ", tempPath)

	// Add the options for the different categories of flag files.
	for _, category := range hiddenAPIFlagFileCategories {
		paths := flagFileInfo.categoryToPaths[category]
		for _, path := range paths {
			category.commandMutator(command, path)
		}
	}

	commitChangeForRestat(rule, tempPath, outputPath)

	if validFile != nil {
		// Add the file that indicates that the file generated by this is valid.
		//
		// This will cause the validation rule above to be run any time that the output of this rule
		// changes but the validation will run in parallel with other rules that depend on this file.
		command.Validation(validFile)
	}

	rule.Build(name, desc)
}

// hiddenAPIGenerateAllFlagsForBootclasspathFragment will generate all the flags for a fragment
// of the bootclasspath.
//
// It takes:
// * Map from android.SdkKind to stub dex jar paths defining the API for that sdk kind.
// * The list of modules that are the contents of the fragment.
// * The additional manually curated flag files to use.
//
// It generates:
// * stub-flags.csv
// * annotation-flags.csv
// * metadata.csv
// * index.csv
// * all-flags.csv
func hiddenAPIGenerateAllFlagsForBootclasspathFragment(ctx android.ModuleContext, contents []android.Module, stubJarsByKind map[android.SdkKind]android.Paths, flagFileInfo *hiddenAPIFlagFileInfo) {

	hiddenApiSubDir := "modular-hiddenapi"

	bootDexJars := android.Paths{}
	classesJars := android.Paths{}
	for _, module := range contents {
		if hiddenAPI, ok := module.(hiddenAPIIntf); ok {
			classesJars = append(classesJars, hiddenAPI.classesJars()...)
			bootDexJar := hiddenAPI.bootDexJar()
			if bootDexJar == nil {
				ctx.ModuleErrorf("module %s does not provide a dex jar", module)
			} else {
				bootDexJars = append(bootDexJars, bootDexJar)
			}
		} else {
			ctx.ModuleErrorf("module %s does not implement hiddenAPIIntf", module)
		}
	}

	// Generate the stub-flags.csv.
	stubFlagsCSV := android.PathForModuleOut(ctx, hiddenApiSubDir, "stub-flags.csv")
	rule := ruleToGenerateHiddenAPIStubFlagsFile(ctx, stubFlagsCSV, bootDexJars, stubJarsByKind)
	rule.Build("modularHiddenAPIStubFlagsFile", "modular hiddenapi stub flags")

	// Generate the set of flags from the annotations in the source code.
	annotationFlagsCSV := android.PathForModuleOut(ctx, hiddenApiSubDir, "annotation-flags.csv")
	buildRuleToGenerateAnnotationFlags(ctx, "modular hiddenapi annotation flags", classesJars, stubFlagsCSV, annotationFlagsCSV)

	// Generate the metadata from the annotations in the source code.
	metadataCSV := android.PathForModuleOut(ctx, hiddenApiSubDir, "metadata.csv")
	buildRuleToGenerateMetadata(ctx, "modular hiddenapi metadata", classesJars, stubFlagsCSV, metadataCSV)

	// Generate the index file from the annotations in the source code.
	indexCSV := android.PathForModuleOut(ctx, hiddenApiSubDir, "index.csv")
	buildRuleToGenerateIndex(ctx, "modular hiddenapi index", classesJars, indexCSV)

	// Removed APIs need to be marked and in order to do that the flagFileInfo needs to specify files
	// containing dex signatures of all the removed APIs. In the monolithic files that is done by
	// manually combining all the removed.txt files for each API and then converting them to dex
	// signatures, see the combined-removed-dex module. That will all be done automatically in future.
	// For now removed APIs are ignored.
	// TODO(b/179354495): handle removed apis automatically.

	// Generate the all-flags.csv which are the flags that will, in future, be encoded into the dex
	// files.
	outputPath := android.PathForModuleOut(ctx, hiddenApiSubDir, "all-flags.csv")
	buildRuleToGenerateHiddenApiFlags(ctx, "modularHiddenApiAllFlags", "modular hiddenapi all flags", outputPath, stubFlagsCSV, android.Paths{annotationFlagsCSV}, flagFileInfo)

	// Store the paths in the info for use by other modules and sdk snapshot generation.
	flagFileInfo.StubFlagsPaths = android.Paths{stubFlagsCSV}
	flagFileInfo.AnnotationFlagsPaths = android.Paths{annotationFlagsCSV}
	flagFileInfo.MetadataPaths = android.Paths{metadataCSV}
	flagFileInfo.IndexPaths = android.Paths{indexCSV}
	flagFileInfo.AllFlagsPaths = android.Paths{outputPath}
}
