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
	Command:     "${config.Class2Greylist} --stub-api-flags ${stubAPIFlags} $in $outFlag $out",
	CommandDeps: []string{"${config.Class2Greylist}"},
}, "outFlag", "stubAPIFlags")

type hiddenAPI struct {
	bootDexJarPath  android.Path
	flagsCSVPath    android.Path
	indexCSVPath    android.Path
	metadataCSVPath android.Path
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

func (h *hiddenAPI) hiddenAPI(ctx android.ModuleContext, name string, primary bool, dexJar android.ModuleOutPath,
	implementationJar android.Path, uncompressDex bool) android.ModuleOutPath {
	if !ctx.Config().IsEnvTrue("UNSAFE_DISABLE_HIDDENAPI_FLAGS") {

		// Modules whose names are of the format <x>-hiddenapi provide hiddenapi information
		// for the boot jar module <x>. Otherwise, the module provides information for itself.
		// Either way extract the name of the boot jar module.
		bootJarName := strings.TrimSuffix(name, "-hiddenapi")

		// If this module is on the boot jars list (or providing information for a module
		// on the list) then extract the hiddenapi information from it, and if necessary
		// encode that information in the generated dex file.
		//
		// It is important that hiddenapi information is only gathered for/from modules on
		// that are actually on the boot jars list because the runtime only enforces access
		// to the hidden API for the bootclassloader. If information is gathered for modules
		// not on the list then that will cause failures in the CtsHiddenApiBlacklist...
		// tests.
		isBootJarProvider := false
		ctx.VisitAllModuleVariants(func(module android.Module) {
			if m, ok := module.(interface{ BootJarProvider() bool }); ok &&
				m.BootJarProvider() {
				isBootJarProvider = true
			}
		})
		if isBootJarProvider && inList(bootJarName, ctx.Config().BootJars()) {
			// Derive the greylist from classes jar.
			flagsCSV := android.PathForModuleOut(ctx, "hiddenapi", "flags.csv")
			metadataCSV := android.PathForModuleOut(ctx, "hiddenapi", "metadata.csv")
			indexCSV := android.PathForModuleOut(ctx, "hiddenapi", "index.csv")
			h.hiddenAPIGenerateCSV(ctx, flagsCSV, metadataCSV, indexCSV, implementationJar)

			// If this module is actually on the boot jars list and not providing
			// hiddenapi information for a module on the boot jars list then encode
			// the gathered information in the generated dex file.
			if name == bootJarName {
				hiddenAPIJar := android.PathForModuleOut(ctx, "hiddenapi", name+".jar")

				// More than one library with the same classes can be encoded but only one can
				// be added to the global set of flags, otherwise it will result in duplicate
				// classes which is an error. Therefore, only add the dex jar of one of them
				// to the global set of flags.
				if primary {
					h.bootDexJarPath = dexJar
				}
				hiddenAPIEncodeDex(ctx, hiddenAPIJar, dexJar, uncompressDex)
				dexJar = hiddenAPIJar
			}
		}
	}

	return dexJar
}

func (h *hiddenAPI) hiddenAPIGenerateCSV(ctx android.ModuleContext, flagsCSV, metadataCSV, indexCSV android.WritablePath, classesJar android.Path) {
	stubFlagsCSV := hiddenAPISingletonPaths(ctx).stubFlags

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

	rule := android.NewRuleBuilder()
	rule.Command().
		BuiltTool(ctx, "merge_csv").
		FlagWithInput("--zip_input=", classesJar).
		FlagWithOutput("--output=", indexCSV)
	rule.Build(pctx, ctx, "merged-hiddenapi-index", "Merged Hidden API index")
	h.indexCSVPath = indexCSV
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
