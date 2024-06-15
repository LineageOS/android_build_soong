// Copyright 2019 Google Inc. All rights reserved.
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
	"github.com/google/blueprint"

	"android/soong/android"
)

var (
	hiddenAPIGenerateCSVRule = pctx.AndroidStaticRule("hiddenAPIGenerateCSV", blueprint.RuleParams{
		Command:     "${config.Class2NonSdkList} --stub-api-flags ${stubAPIFlags} $in $outFlag $out",
		CommandDeps: []string{"${config.Class2NonSdkList}"},
	}, "outFlag", "stubAPIFlags")

	hiddenAPIGenerateIndexRule = pctx.AndroidStaticRule("hiddenAPIGenerateIndex", blueprint.RuleParams{
		Command:     "${config.MergeCsvCommand} --zip_input --key_field signature --output=$out $in",
		CommandDeps: []string{"${config.MergeCsvCommand}"},
	})
)

type hiddenAPI struct {
	// True if the module containing this structure contributes to the hiddenapi information or has
	// that information encoded within it.
	active bool

	// The path to the dex jar that is in the boot class path. If this is unset then the associated
	// module is not a boot jar, but could be one of the <x>-hiddenapi modules that provide additional
	// annotations for the <x> boot dex jar but which do not actually provide a boot dex jar
	// themselves.
	//
	// This must be the path to the unencoded dex jar as the encoded dex jar indirectly depends on
	// this file so using the encoded dex jar here would result in a cycle in the ninja rules.
	bootDexJarPath    OptionalDexJarPath
	bootDexJarPathErr error

	// The paths to the classes jars that contain classes and class members annotated with
	// the UnsupportedAppUsage annotation that need to be extracted as part of the hidden API
	// processing.
	classesJarPaths android.Paths

	// The compressed state of the dex file being encoded. This is used to ensure that the encoded
	// dex file has the same state.
	uncompressDexState *bool
}

func (h *hiddenAPI) bootDexJar(ctx android.ModuleErrorfContext) OptionalDexJarPath {
	if h.bootDexJarPathErr != nil {
		ctx.ModuleErrorf(h.bootDexJarPathErr.Error())
	}
	return h.bootDexJarPath
}

func (h *hiddenAPI) classesJars() android.Paths {
	return h.classesJarPaths
}

func (h *hiddenAPI) uncompressDex() *bool {
	return h.uncompressDexState
}

// hiddenAPIModule is the interface a module that embeds the hiddenAPI structure must implement.
type hiddenAPIModule interface {
	android.Module
	hiddenAPIIntf

	MinSdkVersion(ctx android.EarlyModuleContext) android.ApiLevel
}

type hiddenAPIIntf interface {
	bootDexJar(ctx android.ModuleErrorfContext) OptionalDexJarPath
	classesJars() android.Paths
	uncompressDex() *bool
}

var _ hiddenAPIIntf = (*hiddenAPI)(nil)

// Initialize the hiddenapi structure
//
// uncompressedDexState should be nil when the module is a prebuilt and so does not require hidden
// API encoding.
func (h *hiddenAPI) initHiddenAPI(ctx android.ModuleContext, dexJar OptionalDexJarPath, classesJar android.Path, uncompressedDexState *bool) {

	// Save the classes jars even if this is not active as they may be used by modular hidden API
	// processing.
	classesJars := android.Paths{classesJar}
	ctx.VisitDirectDepsWithTag(hiddenApiAnnotationsTag, func(dep android.Module) {
		javaInfo, _ := android.OtherModuleProvider(ctx, dep, JavaInfoProvider)
		classesJars = append(classesJars, javaInfo.ImplementationJars...)
	})
	h.classesJarPaths = classesJars

	// Save the unencoded dex jar so it can be used when generating the
	// hiddenAPISingletonPathsStruct.stubFlags file.
	h.bootDexJarPath = dexJar

	h.uncompressDexState = uncompressedDexState

	// If hiddenapi processing is disabled treat this as inactive.
	if ctx.Config().DisableHiddenApiChecks() {
		return
	}

	// The context module must implement hiddenAPIModule.
	module := ctx.Module().(hiddenAPIModule)

	// If the frameworks/base directories does not exist and no prebuilt hidden API flag files have
	// been configured then it is not possible to do hidden API encoding.
	if !ctx.Config().FrameworksBaseDirExists(ctx) && ctx.Config().PrebuiltHiddenApiDir(ctx) == "" {
		return
	}

	// It is important that hiddenapi information is only gathered for/from modules that are actually
	// on the boot jars list because the runtime only enforces access to the hidden API for the
	// bootclassloader. If information is gathered for modules not on the list then that will cause
	// failures in the CtsHiddenApiBlocklist... tests.
	h.active = isModuleInBootClassPath(ctx, module)
}

// Store any error encountered during the initialization of hiddenapi structure (e.g. unflagged co-existing prebuilt apexes)
func (h *hiddenAPI) initHiddenAPIError(err error) {
	h.bootDexJarPathErr = err
}

func isModuleInBootClassPath(ctx android.BaseModuleContext, module android.Module) bool {
	// Get the configured platform and apex boot jars.
	nonApexBootJars := ctx.Config().NonApexBootJars()
	apexBootJars := ctx.Config().ApexBootJars()
	active := isModuleInConfiguredList(ctx, module, nonApexBootJars) ||
		isModuleInConfiguredList(ctx, module, apexBootJars)
	return active
}

// hiddenAPIEncodeDex is called by any module that needs to encode dex files.
//
// It ignores any module that has not had initHiddenApi() called on it and which is not in the boot
// jar list. In that case it simply returns the supplied dex jar path.
//
// Otherwise, it creates a copy of the supplied dex file into which it has encoded the hiddenapi
// flags and returns this instead of the supplied dex jar.
func (h *hiddenAPI) hiddenAPIEncodeDex(ctx android.ModuleContext, dexJar android.OutputPath) android.OutputPath {

	if !h.active {
		return dexJar
	}

	// A nil uncompressDexState prevents the dex file from being encoded.
	if h.uncompressDexState == nil {
		ctx.ModuleErrorf("cannot encode dex file %s when uncompressDexState is nil", dexJar)
	}
	uncompressDex := *h.uncompressDexState

	// Create a copy of the dex jar which has been encoded with hiddenapi flags.
	flagsCSV := hiddenAPISingletonPaths(ctx).flags
	outputDir := android.PathForModuleOut(ctx, "hiddenapi").OutputPath
	encodedDex := hiddenAPIEncodeDex(ctx, dexJar, flagsCSV, uncompressDex, android.NoneApiLevel, outputDir)

	// Use the encoded dex jar from here onwards.
	return encodedDex
}

// buildRuleToGenerateAnnotationFlags builds a ninja rule to generate the annotation-flags.csv file
// from the classes jars and stub-flags.csv files.
//
// The annotation-flags.csv file contains mappings from Java signature to various flags derived from
// annotations in the source, e.g. whether it is public or the sdk version above which it can no
// longer be used.
//
// It is created by the Class2NonSdkList tool which processes the .class files in the class
// implementation jar looking for UnsupportedAppUsage and CovariantReturnType annotations. The
// tool also consumes the hiddenAPISingletonPathsStruct.stubFlags file in order to perform
// consistency checks on the information in the annotations and to filter out bridge methods
// that are already part of the public API.
func buildRuleToGenerateAnnotationFlags(ctx android.ModuleContext, desc string, classesJars android.Paths, stubFlagsCSV android.Path, outputPath android.WritablePath) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        hiddenAPIGenerateCSVRule,
		Description: desc,
		Inputs:      classesJars,
		Output:      outputPath,
		Implicit:    stubFlagsCSV,
		Args: map[string]string{
			"outFlag":      "--write-flags-csv",
			"stubAPIFlags": stubFlagsCSV.String(),
		},
	})
}

// buildRuleToGenerateMetadata builds a ninja rule to generate the metadata.csv file from
// the classes jars and stub-flags.csv files.
//
// The metadata.csv file contains mappings from Java signature to the value of properties specified
// on UnsupportedAppUsage annotations in the source.
//
// Like the annotation-flags.csv file this is also created by the Class2NonSdkList in the same way.
// Although the two files could potentially be created in a single invocation of the
// Class2NonSdkList at the moment they are created using their own invocation, with the behavior
// being determined by the property that is used.
func buildRuleToGenerateMetadata(ctx android.ModuleContext, desc string, classesJars android.Paths, stubFlagsCSV android.Path, metadataCSV android.WritablePath) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        hiddenAPIGenerateCSVRule,
		Description: desc,
		Inputs:      classesJars,
		Output:      metadataCSV,
		Implicit:    stubFlagsCSV,
		Args: map[string]string{
			"outFlag":      "--write-metadata-csv",
			"stubAPIFlags": stubFlagsCSV.String(),
		},
	})
}

// buildRuleToGenerateIndex builds a ninja rule to generate the index.csv file from the classes
// jars.
//
// The index.csv file contains mappings from Java signature to source location information.
//
// It is created by the merge_csv tool which processes the class implementation jar, extracting
// all the files ending in .uau (which are CSV files) and merges them together. The .uau files are
// created by the unsupported app usage annotation processor during compilation of the class
// implementation jar.
func buildRuleToGenerateIndex(ctx android.ModuleContext, desc string, classesJars android.Paths, indexCSV android.WritablePath) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        hiddenAPIGenerateIndexRule,
		Description: desc,
		Inputs:      classesJars,
		Output:      indexCSV,
	})
}

var hiddenAPIEncodeDexRule = pctx.AndroidStaticRule("hiddenAPIEncodeDex", blueprint.RuleParams{
	Command: `rm -rf $tmpDir && mkdir -p $tmpDir && mkdir $tmpDir/dex-input && mkdir $tmpDir/dex-output &&
		unzip -qoDD $in 'classes*.dex' -d $tmpDir/dex-input &&
		for INPUT_DEX in $$(find $tmpDir/dex-input -maxdepth 1 -name 'classes*.dex' | sort); do
		  echo "--input-dex=$${INPUT_DEX}";
		  echo "--output-dex=$tmpDir/dex-output/$$(basename $${INPUT_DEX})";
		done | xargs ${config.HiddenAPI} encode --api-flags=$flagsCsv $hiddenapiFlags &&
		${config.SoongZipCmd} $soongZipFlags -o $tmpDir/dex.jar -C $tmpDir/dex-output -f "$tmpDir/dex-output/classes*.dex" &&
		${config.MergeZipsCmd} -j -D -zipToNotStrip $tmpDir/dex.jar -stripFile "classes*.dex" -stripFile "**/*.uau" $out $tmpDir/dex.jar $in`,
	CommandDeps: []string{
		"${config.HiddenAPI}",
		"${config.SoongZipCmd}",
		"${config.MergeZipsCmd}",
	},
}, "flagsCsv", "hiddenapiFlags", "tmpDir", "soongZipFlags")

// hiddenAPIEncodeDex generates the build rule that will encode the supplied dex jar and place the
// encoded dex jar in a file of the same name in the output directory.
//
// The encode dex rule requires unzipping, encoding and rezipping the classes.dex files along with
// all the resources from the input jar. It also ensures that if it was uncompressed in the input
// it stays uncompressed in the output.
func hiddenAPIEncodeDex(ctx android.ModuleContext, dexInput, flagsCSV android.Path, uncompressDex bool, minSdkVersion android.ApiLevel, outputDir android.OutputPath) android.OutputPath {

	// The output file has the same name as the input file and is in the output directory.
	output := outputDir.Join(ctx, dexInput.Base())

	// Create a jar specific temporary directory in which to do the work just in case this is called
	// with the same output directory for multiple modules.
	tmpDir := outputDir.Join(ctx, dexInput.Base()+"-tmp")

	// If the input is uncompressed then generate the output of the encode rule to an intermediate
	// file as the final output will need further processing after encoding.
	soongZipFlags := ""
	encodeRuleOutput := output
	if uncompressDex {
		soongZipFlags = "-L 0"
		encodeRuleOutput = outputDir.Join(ctx, "unaligned", dexInput.Base())
	}

	// b/149353192: when a module is instrumented, jacoco adds synthetic members
	// $jacocoData and $jacocoInit. Since they don't exist when building the hidden API flags,
	// don't complain when we don't find hidden API flags for the synthetic members.
	hiddenapiFlags := ""
	if j, ok := ctx.Module().(interface {
		shouldInstrument(android.BaseModuleContext) bool
	}); ok && j.shouldInstrument(ctx) {
		hiddenapiFlags = "--no-force-assign-all"
	}

	// If the library is targeted for Q and/or R then make sure that they do not
	// have any S+ flags encoded as that will break the runtime.
	minApiLevel := minSdkVersion
	if !minApiLevel.IsNone() {
		if minApiLevel.LessThanOrEqualTo(android.ApiLevelOrPanic(ctx, "R")) {
			hiddenapiFlags = hiddenapiFlags + " --max-hiddenapi-level=max-target-r"
		}
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        hiddenAPIEncodeDexRule,
		Description: "hiddenapi encode dex",
		Input:       dexInput,
		Output:      encodeRuleOutput,
		Implicit:    flagsCSV,
		Args: map[string]string{
			"flagsCsv":       flagsCSV.String(),
			"tmpDir":         tmpDir.String(),
			"soongZipFlags":  soongZipFlags,
			"hiddenapiFlags": hiddenapiFlags,
		},
	})

	if uncompressDex {
		TransformZipAlign(ctx, output, encodeRuleOutput, nil)
	}

	return output
}

type hiddenApiAnnotationsDependencyTag struct {
	blueprint.BaseDependencyTag
	android.LicenseAnnotationSharedDependencyTag
}

// Tag used to mark dependencies on java_library instances that contains Java source files whose
// sole purpose is to provide additional hiddenapi annotations.
var hiddenApiAnnotationsTag hiddenApiAnnotationsDependencyTag

// Mark this tag so dependencies that use it are excluded from APEX contents.
func (t hiddenApiAnnotationsDependencyTag) ExcludeFromApexContents() {}

var _ android.ExcludeFromApexContentsTag = hiddenApiAnnotationsTag
