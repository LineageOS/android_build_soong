// Copyright 2017 Google Inc. All rights reserved.
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
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
)

func isPathValueResource(res android.Path) bool {
	subDir := filepath.Dir(res.String())
	subDir, lastDir := filepath.Split(subDir)
	return strings.HasPrefix(lastDir, "values")
}

// Convert input resource file path to output file path.
// values-[config]/<file>.xml -> values-[config]_<file>.arsc.flat;
// For other resource file, just replace the last "/" with "_" and add .flat extension.
func pathToAapt2Path(ctx android.ModuleContext, res android.Path) android.WritablePath {

	name := res.Base()
	if isPathValueResource(res) {
		name = strings.TrimSuffix(name, ".xml") + ".arsc"
	}
	subDir := filepath.Dir(res.String())
	subDir, lastDir := filepath.Split(subDir)
	name = lastDir + "_" + name + ".flat"
	return android.PathForModuleOut(ctx, "aapt2", subDir, name)
}

// pathsToAapt2Paths Calls pathToAapt2Path on each entry of the given Paths, i.e. []Path.
func pathsToAapt2Paths(ctx android.ModuleContext, resPaths android.Paths) android.WritablePaths {
	outPaths := make(android.WritablePaths, len(resPaths))

	for i, res := range resPaths {
		outPaths[i] = pathToAapt2Path(ctx, res)
	}

	return outPaths
}

// Shard resource files for efficiency. See aapt2Compile for details.
const AAPT2_SHARD_SIZE = 100

var aapt2CompileRule = pctx.AndroidStaticRule("aapt2Compile",
	blueprint.RuleParams{
		Command:     `${config.Aapt2Cmd} compile -o $outDir $cFlags $in`,
		CommandDeps: []string{"${config.Aapt2Cmd}"},
	},
	"outDir", "cFlags")

// aapt2Compile compiles resources and puts the results in the requested directory.
func aapt2Compile(ctx android.ModuleContext, dir android.Path, paths android.Paths,
	flags []string, productToFilter string) android.WritablePaths {
	if productToFilter != "" && productToFilter != "default" {
		// --filter-product leaves only product-specific resources. Product-specific resources only exist
		// in value resources (values/*.xml), so filter value resource files only. Ignore other types of
		// resources as they don't need to be in product characteristics RRO (and they will cause aapt2
		// compile errors)
		filteredPaths := android.Paths{}
		for _, path := range paths {
			if isPathValueResource(path) {
				filteredPaths = append(filteredPaths, path)
			}
		}
		paths = filteredPaths
		flags = append([]string{"--filter-product " + productToFilter}, flags...)
	}

	// Shard the input paths so that they can be processed in parallel. If we shard them into too
	// small chunks, the additional cost of spinning up aapt2 outweighs the performance gain. The
	// current shard size, 100, seems to be a good balance between the added cost and the gain.
	// The aapt2 compile actions are trivially short, but each action in ninja takes on the order of
	// ~10 ms to run. frameworks/base/core/res/res has >10k resource files, so compiling each one
	// with an individual action could take 100 CPU seconds. Sharding them reduces the overhead of
	// starting actions by a factor of 100, at the expense of recompiling more files when one
	// changes.  Since the individual compiles are trivial it's a good tradeoff.
	shards := android.ShardPaths(paths, AAPT2_SHARD_SIZE)

	ret := make(android.WritablePaths, 0, len(paths))

	for i, shard := range shards {
		// This should be kept in sync with pathToAapt2Path. The aapt2 compile command takes an
		// output directory path, but not output file paths. So, outPaths is just where we expect
		// the output files will be located.
		outPaths := pathsToAapt2Paths(ctx, shard)
		ret = append(ret, outPaths...)

		shardDesc := ""
		if i != 0 {
			shardDesc = " " + strconv.Itoa(i+1)
		}

		ctx.Build(pctx, android.BuildParams{
			Rule:        aapt2CompileRule,
			Description: "aapt2 compile " + dir.String() + shardDesc,
			Inputs:      shard,
			Outputs:     outPaths,
			Args: map[string]string{
				// The aapt2 compile command takes an output directory path, but not output file paths.
				// outPaths specified above is only used for dependency management purposes. In order for
				// the outPaths values to match the actual outputs from aapt2, the dir parameter value
				// must be a common prefix path of the paths values, and the top-level path segment used
				// below, "aapt2", must always be kept in sync with the one in pathToAapt2Path.
				// TODO(b/174505750): Make this easier and robust to use.
				"outDir": android.PathForModuleOut(ctx, "aapt2", dir.String()).String(),
				"cFlags": strings.Join(flags, " "),
			},
		})
	}

	sort.Slice(ret, func(i, j int) bool {
		return ret[i].String() < ret[j].String()
	})
	return ret
}

var aapt2CompileZipRule = pctx.AndroidStaticRule("aapt2CompileZip",
	blueprint.RuleParams{
		Command: `${config.ZipSyncCmd} -d $resZipDir $zipSyncFlags $in && ` +
			`${config.Aapt2Cmd} compile -o $out $cFlags --dir $resZipDir`,
		CommandDeps: []string{
			"${config.Aapt2Cmd}",
			"${config.ZipSyncCmd}",
		},
	}, "cFlags", "resZipDir", "zipSyncFlags")

// Unzips the given compressed file and compiles the resource source files in it. The zipPrefix
// parameter points to the subdirectory in the zip file where the resource files are located.
func aapt2CompileZip(ctx android.ModuleContext, flata android.WritablePath, zip android.Path, zipPrefix string,
	flags []string) {

	if zipPrefix != "" {
		zipPrefix = "--zip-prefix " + zipPrefix
	}
	ctx.Build(pctx, android.BuildParams{
		Rule:        aapt2CompileZipRule,
		Description: "aapt2 compile zip",
		Input:       zip,
		Output:      flata,
		Args: map[string]string{
			"cFlags":       strings.Join(flags, " "),
			"resZipDir":    android.PathForModuleOut(ctx, "aapt2", "reszip", flata.Base()).String(),
			"zipSyncFlags": zipPrefix,
		},
	})
}

var aapt2LinkRule = pctx.AndroidStaticRule("aapt2Link",
	blueprint.RuleParams{
		Command: `$preamble` +
			`${config.Aapt2Cmd} link -o $out $flags --proguard $proguardOptions ` +
			`--output-text-symbols ${rTxt} $inFlags` +
			`$postamble`,

		CommandDeps: []string{
			"${config.Aapt2Cmd}",
			"${config.SoongZipCmd}",
		},
		Restat: true,
	},
	"flags", "inFlags", "proguardOptions", "rTxt", "extraPackages", "preamble", "postamble")

var aapt2ExtractExtraPackagesRule = pctx.AndroidStaticRule("aapt2ExtractExtraPackages",
	blueprint.RuleParams{
		Command:     `${config.ExtractJarPackagesCmd} -i $in -o $out --prefix '--extra-packages '`,
		CommandDeps: []string{"${config.ExtractJarPackagesCmd}"},
		Restat:      true,
	})

var fileListToFileRule = pctx.AndroidStaticRule("fileListToFile",
	blueprint.RuleParams{
		Command:        `cp $out.rsp $out`,
		Rspfile:        "$out.rsp",
		RspfileContent: "$in",
	})

var mergeAssetsRule = pctx.AndroidStaticRule("mergeAssets",
	blueprint.RuleParams{
		Command:     `${config.MergeZipsCmd} ${out} ${in}`,
		CommandDeps: []string{"${config.MergeZipsCmd}"},
	})

func aapt2Link(ctx android.ModuleContext,
	packageRes, genJar, proguardOptions, rTxt android.WritablePath,
	flags []string, deps android.Paths,
	compiledRes, compiledOverlay, assetPackages android.Paths, splitPackages android.WritablePaths,
	featureFlagsPaths android.Paths) {

	var inFlags []string

	if len(compiledRes) > 0 {
		// Create a file that contains the list of all compiled resource file paths.
		resFileList := android.PathForModuleOut(ctx, "aapt2", "res.list")
		// Write out file lists to files
		ctx.Build(pctx, android.BuildParams{
			Rule:        fileListToFileRule,
			Description: "resource file list",
			Inputs:      compiledRes,
			Output:      resFileList,
		})

		deps = append(deps, compiledRes...)
		deps = append(deps, resFileList)
		// aapt2 filepath arguments that start with "@" mean file-list files.
		inFlags = append(inFlags, "@"+resFileList.String())
	}

	if len(compiledOverlay) > 0 {
		// Compiled overlay files are processed the same way as compiled resources.
		overlayFileList := android.PathForModuleOut(ctx, "aapt2", "overlay.list")
		ctx.Build(pctx, android.BuildParams{
			Rule:        fileListToFileRule,
			Description: "overlay resource file list",
			Inputs:      compiledOverlay,
			Output:      overlayFileList,
		})

		deps = append(deps, compiledOverlay...)
		deps = append(deps, overlayFileList)
		// Compiled overlay files are passed over to aapt2 using -R option.
		inFlags = append(inFlags, "-R", "@"+overlayFileList.String())
	}

	// Set auxiliary outputs as implicit outputs to establish correct dependency chains.
	implicitOutputs := append(splitPackages, proguardOptions, rTxt)
	linkOutput := packageRes

	// AAPT2 ignores assets in overlays. Merge them after linking.
	if len(assetPackages) > 0 {
		linkOutput = android.PathForModuleOut(ctx, "aapt2", "package-res.apk")
		inputZips := append(android.Paths{linkOutput}, assetPackages...)
		ctx.Build(pctx, android.BuildParams{
			Rule:        mergeAssetsRule,
			Inputs:      inputZips,
			Output:      packageRes,
			Description: "merge assets from dependencies",
		})
	}

	for _, featureFlagsPath := range featureFlagsPaths {
		deps = append(deps, featureFlagsPath)
		inFlags = append(inFlags, "--feature-flags", "@"+featureFlagsPath.String())
	}

	// Note the absence of splitPackages. The caller is supposed to compose and provide --split flag
	// values via the flags parameter when it wants to split outputs.
	// TODO(b/174509108): Perhaps we can process it in this func while keeping the code reasonably
	// tidy.
	args := map[string]string{
		"flags":           strings.Join(flags, " "),
		"inFlags":         strings.Join(inFlags, " "),
		"proguardOptions": proguardOptions.String(),
		"rTxt":            rTxt.String(),
	}

	if genJar != nil {
		// Generating java source files from aapt2 was requested, use aapt2LinkAndGenRule and pass it
		// genJar and genDir args.
		genDir := android.PathForModuleGen(ctx, "aapt2", "R")
		ctx.Variable(pctx, "aapt2GenDir", genDir.String())
		ctx.Variable(pctx, "aapt2GenJar", genJar.String())
		implicitOutputs = append(implicitOutputs, genJar)
		args["preamble"] = `rm -rf $aapt2GenDir && `
		args["postamble"] = `&& ${config.SoongZipCmd} -write_if_changed -jar -o $aapt2GenJar -C $aapt2GenDir -D $aapt2GenDir && ` +
			`rm -rf $aapt2GenDir`
		args["flags"] += " --java $aapt2GenDir"
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:            aapt2LinkRule,
		Description:     "aapt2 link",
		Implicits:       deps,
		Output:          linkOutput,
		ImplicitOutputs: implicitOutputs,
		Args:            args,
	})
}

// aapt2ExtractExtraPackages takes a srcjar generated by aapt2 or a classes jar generated by ResourceProcessorBusyBox
// and converts it to a text file containing a list of --extra_package arguments for passing to Make modules so they
// correctly generate R.java entries for packages provided by transitive dependencies.
func aapt2ExtractExtraPackages(ctx android.ModuleContext, out android.WritablePath, in android.Path) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        aapt2ExtractExtraPackagesRule,
		Description: "aapt2 extract extra packages",
		Input:       in,
		Output:      out,
	})
}

var aapt2ConvertRule = pctx.AndroidStaticRule("aapt2Convert",
	blueprint.RuleParams{
		Command: `${config.Aapt2Cmd} convert --enable-compact-entries ` +
			`--output-format $format $in -o $out`,
		CommandDeps: []string{"${config.Aapt2Cmd}"},
	}, "format",
)

// Converts xml files and resource tables (resources.arsc) in the given jar/apk file to a proto
// format. The proto definition is available at frameworks/base/tools/aapt2/Resources.proto.
func aapt2Convert(ctx android.ModuleContext, out android.WritablePath, in android.Path, format string) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        aapt2ConvertRule,
		Input:       in,
		Output:      out,
		Description: "convert to " + format,
		Args: map[string]string{
			"format": format,
		},
	})
}
