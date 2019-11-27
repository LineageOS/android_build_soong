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

const AAPT2_SHARD_SIZE = 100

// Convert input resource file path to output file path.
// values-[config]/<file>.xml -> values-[config]_<file>.arsc.flat;
// For other resource file, just replace the last "/" with "_" and
// add .flat extension.
func pathToAapt2Path(ctx android.ModuleContext, res android.Path) android.WritablePath {

	name := res.Base()
	subDir := filepath.Dir(res.String())
	subDir, lastDir := filepath.Split(subDir)
	if strings.HasPrefix(lastDir, "values") {
		name = strings.TrimSuffix(name, ".xml") + ".arsc"
	}
	name = lastDir + "_" + name + ".flat"
	return android.PathForModuleOut(ctx, "aapt2", subDir, name)
}

func pathsToAapt2Paths(ctx android.ModuleContext, resPaths android.Paths) android.WritablePaths {
	outPaths := make(android.WritablePaths, len(resPaths))

	for i, res := range resPaths {
		outPaths[i] = pathToAapt2Path(ctx, res)
	}

	return outPaths
}

var aapt2CompileRule = pctx.AndroidStaticRule("aapt2Compile",
	blueprint.RuleParams{
		Command:     `${config.Aapt2Cmd} compile -o $outDir $cFlags --legacy $in`,
		CommandDeps: []string{"${config.Aapt2Cmd}"},
	},
	"outDir", "cFlags")

func aapt2Compile(ctx android.ModuleContext, dir android.Path, paths android.Paths) android.WritablePaths {
	shards := android.ShardPaths(paths, AAPT2_SHARD_SIZE)

	ret := make(android.WritablePaths, 0, len(paths))

	for i, shard := range shards {
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
				"outDir": android.PathForModuleOut(ctx, "aapt2", dir.String()).String(),
				// Always set --pseudo-localize, it will be stripped out later for release
				// builds that don't want it.
				"cFlags": "--pseudo-localize",
			},
		})
	}

	sort.Slice(ret, func(i, j int) bool {
		return ret[i].String() < ret[j].String()
	})
	return ret
}

func aapt2CompileDirs(ctx android.ModuleContext, flata android.WritablePath, dirs android.Paths, deps android.Paths) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        aapt2CompileRule,
		Description: "aapt2 compile dirs",
		Implicits:   deps,
		Output:      flata,
		Args: map[string]string{
			"outDir": flata.String(),
			// Always set --pseudo-localize, it will be stripped out later for release
			// builds that don't want it.
			"cFlags": "--pseudo-localize " + android.JoinWithPrefix(dirs.Strings(), "--dir "),
		},
	})
}

var aapt2CompileZipRule = pctx.AndroidStaticRule("aapt2CompileZip",
	blueprint.RuleParams{
		Command: `${config.ZipSyncCmd} -d $resZipDir $in && ` +
			`${config.Aapt2Cmd} compile -o $out $cFlags --legacy --dir $resZipDir`,
		CommandDeps: []string{
			"${config.Aapt2Cmd}",
			"${config.ZipSyncCmd}",
		},
	}, "cFlags", "resZipDir")

func aapt2CompileZip(ctx android.ModuleContext, flata android.WritablePath, zip android.Path) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        aapt2CompileZipRule,
		Description: "aapt2 compile zip",
		Input:       zip,
		Output:      flata,
		Args: map[string]string{
			// Always set --pseudo-localize, it will be stripped out later for release
			// builds that don't want it.
			"cFlags":    "--pseudo-localize",
			"resZipDir": android.PathForModuleOut(ctx, "aapt2", "reszip", flata.Base()).String(),
		},
	})
}

var aapt2LinkRule = pctx.AndroidStaticRule("aapt2Link",
	blueprint.RuleParams{
		Command: `rm -rf $genDir && ` +
			`${config.Aapt2Cmd} link -o $out $flags --java $genDir --proguard $proguardOptions ` +
			`--output-text-symbols ${rTxt} $inFlags && ` +
			`${config.SoongZipCmd} -write_if_changed -jar -o $genJar -C $genDir -D $genDir &&` +
			`${config.ExtractJarPackagesCmd} -i $genJar -o $extraPackages --prefix '--extra-packages '`,

		CommandDeps: []string{
			"${config.Aapt2Cmd}",
			"${config.SoongZipCmd}",
			"${config.ExtractJarPackagesCmd}",
		},
		Restat: true,
	},
	"flags", "inFlags", "proguardOptions", "genDir", "genJar", "rTxt", "extraPackages")

var fileListToFileRule = pctx.AndroidStaticRule("fileListToFile",
	blueprint.RuleParams{
		Command:        `cp $out.rsp $out`,
		Rspfile:        "$out.rsp",
		RspfileContent: "$in",
	})

func aapt2Link(ctx android.ModuleContext,
	packageRes, genJar, proguardOptions, rTxt, extraPackages android.WritablePath,
	flags []string, deps android.Paths,
	compiledRes, compiledOverlay android.Paths, splitPackages android.WritablePaths) {

	genDir := android.PathForModuleGen(ctx, "aapt2", "R")

	var inFlags []string

	if len(compiledRes) > 0 {
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
		inFlags = append(inFlags, "@"+resFileList.String())
	}

	if len(compiledOverlay) > 0 {
		overlayFileList := android.PathForModuleOut(ctx, "aapt2", "overlay.list")
		ctx.Build(pctx, android.BuildParams{
			Rule:        fileListToFileRule,
			Description: "overlay resource file list",
			Inputs:      compiledOverlay,
			Output:      overlayFileList,
		})

		deps = append(deps, compiledOverlay...)
		deps = append(deps, overlayFileList)
		inFlags = append(inFlags, "-R", "@"+overlayFileList.String())
	}

	implicitOutputs := append(splitPackages, proguardOptions, genJar, rTxt, extraPackages)

	ctx.Build(pctx, android.BuildParams{
		Rule:            aapt2LinkRule,
		Description:     "aapt2 link",
		Implicits:       deps,
		Output:          packageRes,
		ImplicitOutputs: implicitOutputs,
		Args: map[string]string{
			"flags":           strings.Join(flags, " "),
			"inFlags":         strings.Join(inFlags, " "),
			"proguardOptions": proguardOptions.String(),
			"genDir":          genDir.String(),
			"genJar":          genJar.String(),
			"rTxt":            rTxt.String(),
			"extraPackages":   extraPackages.String(),
		},
	})
}

var aapt2ConvertRule = pctx.AndroidStaticRule("aapt2Convert",
	blueprint.RuleParams{
		Command:     `${config.Aapt2Cmd} convert --output-format proto $in -o $out`,
		CommandDeps: []string{"${config.Aapt2Cmd}"},
	})

func aapt2Convert(ctx android.ModuleContext, out android.WritablePath, in android.Path) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        aapt2ConvertRule,
		Input:       in,
		Output:      out,
		Description: "convert to proto",
	})
}
