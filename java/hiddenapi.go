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
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
)

var hiddenAPIGenerateCSVRule = pctx.AndroidStaticRule("hiddenAPIGenerateCSV", blueprint.RuleParams{
	Command:     "${config.Class2NonSdkList} --stub-api-flags ${stubAPIFlags} $in $outFlag $out",
	CommandDeps: []string{"${config.Class2NonSdkList}"},
}, "outFlag", "stubAPIFlags")

type hiddenAPI struct {
	// The name of the module as it would be used in the boot jars configuration, e.g. without any
	// prebuilt_ prefix (if it is a prebuilt), without any "-hiddenapi" suffix if it just provides
	// annotations and without any ".impl" suffix if it is a java_sdk_library implementation library.
	configurationName string

	// True if the module containing this structure contributes to the hiddenapi information or has
	// that information encoded within it.
	active bool

	// Identifies the active module variant which will be used as the source of hiddenapi information.
	//
	// A class may be compiled into a number of different module variants each of which will need the
	// hiddenapi information encoded into it and so will be marked as active. However, only one of
	// them must be used as a source of information by hiddenapi otherwise it will end up with
	// duplicate entries. That module will have primary=true.
	//
	// Note, that modules <x>-hiddenapi that provide additional annotation information for module <x>
	// that is on the bootclasspath are marked as primary=true as they are the primary source of that
	// annotation information.
	primary bool

	// True if the module only contains additional annotations and so does not require hiddenapi
	// information to be encoded in its dex file and should not be used to generate the
	// hiddenAPISingletonPathsStruct.stubFlags file.
	annotationsOnly bool

	// The path to the dex jar that is in the boot class path. If this is nil then the associated
	// module is not a boot jar, but could be one of the <x>-hiddenapi modules that provide additional
	// annotations for the <x> boot dex jar but which do not actually provide a boot dex jar
	// themselves.
	//
	// This must be the path to the unencoded dex jar as the encoded dex jar indirectly depends on
	// this file so using the encoded dex jar here would result in a cycle in the ninja rules.
	bootDexJarPath android.Path

	// The path to the CSV file that contains mappings from Java signature to various flags derived
	// from annotations in the source, e.g. whether it is public or the sdk version above which it
	// can no longer be used.
	//
	// It is created by the Class2NonSdkList tool which processes the .class files in the class
	// implementation jar looking for UnsupportedAppUsage and CovariantReturnType annotations. The
	// tool also consumes the hiddenAPISingletonPathsStruct.stubFlags file in order to perform
	// consistency checks on the information in the annotations and to filter out bridge methods
	// that are already part of the public API.
	flagsCSVPath android.Path

	// The path to the CSV file that contains mappings from Java signature to the value of properties
	// specified on UnsupportedAppUsage annotations in the source.
	//
	// Like the flagsCSVPath file this is also created by the Class2NonSdkList in the same way.
	// Although the two files could potentially be created in a single invocation of the
	// Class2NonSdkList at the moment they are created using their own invocation, with the behavior
	// being determined by the property that is used.
	metadataCSVPath android.Path

	// The path to the CSV file that contains mappings from Java signature to source location
	// information.
	//
	// It is created by the merge_csv tool which processes the class implementation jar, extracting
	// all the files ending in .uau (which are CSV files) and merges them together. The .uau files are
	// created by the unsupported app usage annotation processor during compilation of the class
	// implementation jar.
	indexCSVPath android.Path
}

func (h *hiddenAPI) flagsCSV() android.Path {
	return h.flagsCSVPath
}

func (h *hiddenAPI) metadataCSV() android.Path {
	return h.metadataCSVPath
}

func (h *hiddenAPI) bootDexJar() android.Path {
	return h.bootDexJarPath
}

func (h *hiddenAPI) indexCSV() android.Path {
	return h.indexCSVPath
}

type hiddenAPIIntf interface {
	bootDexJar() android.Path
	flagsCSV() android.Path
	indexCSV() android.Path
	metadataCSV() android.Path
}

var _ hiddenAPIIntf = (*hiddenAPI)(nil)

// Initialize the hiddenapi structure
func (h *hiddenAPI) initHiddenAPI(ctx android.BaseModuleContext, name string) {
	// If hiddenapi processing is disabled treat this as inactive.
	if ctx.Config().IsEnvTrue("UNSAFE_DISABLE_HIDDENAPI_FLAGS") {
		return
	}

	// Modules whose names are of the format <x>-hiddenapi provide hiddenapi information for the boot
	// jar module <x>. Otherwise, the module provides information for itself. Either way extract the
	// configurationName of the boot jar module.
	configurationName := strings.TrimSuffix(name, "-hiddenapi")
	h.configurationName = configurationName

	// It is important that hiddenapi information is only gathered for/from modules that are actually
	// on the boot jars list because the runtime only enforces access to the hidden API for the
	// bootclassloader. If information is gathered for modules not on the list then that will cause
	// failures in the CtsHiddenApiBlocklist... tests.
	h.active = inList(configurationName, ctx.Config().BootJars())
	if !h.active {
		// The rest of the properties will be ignored if active is false.
		return
	}

	// If this module has a suffix of -hiddenapi then it only provides additional annotation
	// information for a module on the boot jars list.
	h.annotationsOnly = strings.HasSuffix(name, "-hiddenapi")

	// Determine whether this module is the primary module or not.
	primary := true

	// A prebuilt module is only primary if it is preferred and conversely a source module is only
	// primary if it has not been replaced by a prebuilt module.
	module := ctx.Module()
	if pi, ok := module.(android.PrebuiltInterface); ok {
		if p := pi.Prebuilt(); p != nil {
			primary = p.UsePrebuilt()
		}
	} else {
		// The only module that will pass a different name to its module name to this method is the
		// implementation library of a java_sdk_library. It has a configuration name of <x> the same
		// as its parent java_sdk_library but a module name of <x>.impl. It is not the primary module,
		// the java_sdk_library with the name of <x> is.
		primary = name == ctx.ModuleName()

		// A source module that has been replaced by a prebuilt can never be the primary module.
		primary = primary && !module.IsReplacedByPrebuilt()
	}
	h.primary = primary
}

// hiddenAPIExtractAndEncode is called by any module that could contribute to the hiddenapi
// processing.
//
// It ignores any module that has not had initHiddenApi() called on it and which is not in the boot
// jar list.
//
// Otherwise, it generates ninja rules to do the following:
// 1. Extract information needed for hiddenapi processing from the module and output it into CSV
//    files.
// 2. Conditionally adds the supplied dex file to the list of files used to generate the
//    hiddenAPISingletonPathsStruct.stubsFlag file.
// 3. Conditionally creates a copy of the supplied dex file into which it has encoded the hiddenapi
//    flags and returns this instead of the supplied dex jar, otherwise simply returns the supplied
//    dex jar.
func (h *hiddenAPI) hiddenAPIExtractAndEncode(ctx android.ModuleContext, dexJar android.OutputPath,
	implementationJar android.Path, uncompressDex bool) android.OutputPath {

	if !h.active {
		return dexJar
	}

	h.hiddenAPIExtractInformation(ctx, dexJar, implementationJar)

	if !h.annotationsOnly {
		hiddenAPIJar := android.PathForModuleOut(ctx, "hiddenapi", h.configurationName+".jar").OutputPath

		// Create a copy of the dex jar which has been encoded with hiddenapi flags.
		hiddenAPIEncodeDex(ctx, hiddenAPIJar, dexJar, uncompressDex)

		// Use the encoded dex jar from here onwards.
		dexJar = hiddenAPIJar
	}

	return dexJar
}

// hiddenAPIExtractInformation generates ninja rules to extract the information from the classes
// jar, and outputs it to the appropriate module specific CSV file.
//
// It also makes the dex jar available for use when generating the
// hiddenAPISingletonPathsStruct.stubFlags.
func (h *hiddenAPI) hiddenAPIExtractInformation(ctx android.ModuleContext, dexJar, classesJar android.Path) {
	if !h.active {
		return
	}

	// More than one library with the same classes may need to be encoded but only one should be
	// used as a source of information for hidden API processing otherwise it will result in
	// duplicate entries in the files.
	if !h.primary {
		return
	}

	stubFlagsCSV := hiddenAPISingletonPaths(ctx).stubFlags

	flagsCSV := android.PathForModuleOut(ctx, "hiddenapi", "flags.csv")
	ctx.Build(pctx, android.BuildParams{
		Rule:        hiddenAPIGenerateCSVRule,
		Description: "hiddenapi flags",
		Input:       classesJar,
		Output:      flagsCSV,
		Implicit:    stubFlagsCSV,
		Args: map[string]string{
			"outFlag":      "--write-flags-csv",
			"stubAPIFlags": stubFlagsCSV.String(),
		},
	})
	h.flagsCSVPath = flagsCSV

	metadataCSV := android.PathForModuleOut(ctx, "hiddenapi", "metadata.csv")
	ctx.Build(pctx, android.BuildParams{
		Rule:        hiddenAPIGenerateCSVRule,
		Description: "hiddenapi metadata",
		Input:       classesJar,
		Output:      metadataCSV,
		Implicit:    stubFlagsCSV,
		Args: map[string]string{
			"outFlag":      "--write-metadata-csv",
			"stubAPIFlags": stubFlagsCSV.String(),
		},
	})
	h.metadataCSVPath = metadataCSV

	indexCSV := android.PathForModuleOut(ctx, "hiddenapi", "index.csv")
	rule := android.NewRuleBuilder(pctx, ctx)
	rule.Command().
		BuiltTool("merge_csv").
		FlagWithInput("--zip_input=", classesJar).
		FlagWithOutput("--output=", indexCSV)
	rule.Build("merged-hiddenapi-index", "Merged Hidden API index")
	h.indexCSVPath = indexCSV

	// Save the unencoded dex jar so it can be used when generating the
	// hiddenAPISingletonPathsStruct.stubFlags file.
	h.bootDexJarPath = dexJar
}

var hiddenAPIEncodeDexRule = pctx.AndroidStaticRule("hiddenAPIEncodeDex", blueprint.RuleParams{
	Command: `rm -rf $tmpDir && mkdir -p $tmpDir && mkdir $tmpDir/dex-input && mkdir $tmpDir/dex-output &&
		unzip -qoDD $in 'classes*.dex' -d $tmpDir/dex-input &&
		for INPUT_DEX in $$(find $tmpDir/dex-input -maxdepth 1 -name 'classes*.dex' | sort); do
		  echo "--input-dex=$${INPUT_DEX}";
		  echo "--output-dex=$tmpDir/dex-output/$$(basename $${INPUT_DEX})";
		done | xargs ${config.HiddenAPI} encode --api-flags=$flagsCsv $hiddenapiFlags &&
		${config.SoongZipCmd} $soongZipFlags -o $tmpDir/dex.jar -C $tmpDir/dex-output -f "$tmpDir/dex-output/classes*.dex" &&
		${config.MergeZipsCmd} -D -zipToNotStrip $tmpDir/dex.jar -stripFile "classes*.dex" -stripFile "**/*.uau" $out $tmpDir/dex.jar $in`,
	CommandDeps: []string{
		"${config.HiddenAPI}",
		"${config.SoongZipCmd}",
		"${config.MergeZipsCmd}",
	},
}, "flagsCsv", "hiddenapiFlags", "tmpDir", "soongZipFlags")

func hiddenAPIEncodeDex(ctx android.ModuleContext, output android.WritablePath, dexInput android.Path,
	uncompressDex bool) {

	flagsCSV := hiddenAPISingletonPaths(ctx).flags

	// The encode dex rule requires unzipping and rezipping the classes.dex files, ensure that if it was uncompressed
	// in the input it stays uncompressed in the output.
	soongZipFlags := ""
	hiddenapiFlags := ""
	tmpOutput := output
	tmpDir := android.PathForModuleOut(ctx, "hiddenapi", "dex")
	if uncompressDex {
		soongZipFlags = "-L 0"
		tmpOutput = android.PathForModuleOut(ctx, "hiddenapi", "unaligned", "unaligned.jar")
		tmpDir = android.PathForModuleOut(ctx, "hiddenapi", "unaligned")
	}

	enforceHiddenApiFlagsToAllMembers := true
	// If frameworks/base doesn't exist we must be building with the 'master-art' manifest.
	// Disable assertion that all methods/fields have hidden API flags assigned.
	if !ctx.Config().FrameworksBaseDirExists(ctx) {
		enforceHiddenApiFlagsToAllMembers = false
	}
	// b/149353192: when a module is instrumented, jacoco adds synthetic members
	// $jacocoData and $jacocoInit. Since they don't exist when building the hidden API flags,
	// don't complain when we don't find hidden API flags for the synthetic members.
	if j, ok := ctx.Module().(interface {
		shouldInstrument(android.BaseModuleContext) bool
	}); ok && j.shouldInstrument(ctx) {
		enforceHiddenApiFlagsToAllMembers = false
	}

	if !enforceHiddenApiFlagsToAllMembers {
		hiddenapiFlags = "--no-force-assign-all"
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        hiddenAPIEncodeDexRule,
		Description: "hiddenapi encode dex",
		Input:       dexInput,
		Output:      tmpOutput,
		Implicit:    flagsCSV,
		Args: map[string]string{
			"flagsCsv":       flagsCSV.String(),
			"tmpDir":         tmpDir.String(),
			"soongZipFlags":  soongZipFlags,
			"hiddenapiFlags": hiddenapiFlags,
		},
	})

	if uncompressDex {
		TransformZipAlign(ctx, output, tmpOutput)
	}
}
