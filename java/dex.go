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
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
)

var desugar = pctx.AndroidStaticRule("desugar",
	blueprint.RuleParams{
		Command: `rm -rf $dumpDir && mkdir -p $dumpDir && ` +
			`${config.JavaCmd} ` +
			`-Djdk.internal.lambda.dumpProxyClasses=$$(cd $dumpDir && pwd) ` +
			`$javaFlags ` +
			`-jar ${config.DesugarJar} $classpathFlags $desugarFlags ` +
			`-i $in -o $out`,
		CommandDeps: []string{"${config.DesugarJar}", "${config.JavaCmd}"},
	},
	"javaFlags", "classpathFlags", "desugarFlags", "dumpDir")

func (j *Module) desugar(ctx android.ModuleContext, flags javaBuilderFlags,
	classesJar android.Path, jarName string) android.Path {

	desugarFlags := []string{
		"--min_sdk_version " + j.minSdkVersionNumber(ctx),
		"--desugar_try_with_resources_if_needed=false",
		"--allow_empty_bootclasspath",
	}

	if inList("--core-library", j.deviceProperties.Dxflags) {
		desugarFlags = append(desugarFlags, "--core_library")
	}

	desugarJar := android.PathForModuleOut(ctx, "desugar", jarName)
	dumpDir := android.PathForModuleOut(ctx, "desugar", "classes")

	javaFlags := ""
	if ctx.Config().UseOpenJDK9() {
		javaFlags = "--add-opens java.base/java.lang.invoke=ALL-UNNAMED"
	}

	var classpathFlags []string
	classpathFlags = append(classpathFlags, flags.bootClasspath.FormDesugarClasspath("--bootclasspath_entry")...)
	classpathFlags = append(classpathFlags, flags.classpath.FormDesugarClasspath("--classpath_entry")...)

	var deps android.Paths
	deps = append(deps, flags.bootClasspath...)
	deps = append(deps, flags.classpath...)

	ctx.Build(pctx, android.BuildParams{
		Rule:        desugar,
		Description: "desugar",
		Output:      desugarJar,
		Input:       classesJar,
		Implicits:   deps,
		Args: map[string]string{
			"dumpDir":        dumpDir.String(),
			"javaFlags":      javaFlags,
			"classpathFlags": strings.Join(classpathFlags, " "),
			"desugarFlags":   strings.Join(desugarFlags, " "),
		},
	})

	return desugarJar
}

var dx = pctx.AndroidStaticRule("dx",
	blueprint.RuleParams{
		Command: `rm -rf "$outDir" && mkdir -p "$outDir" && ` +
			`${config.DxCmd} --dex --output=$outDir $dxFlags $in && ` +
			`${config.SoongZipCmd} -o $outDir/classes.dex.jar -C $outDir -D $outDir && ` +
			`${config.MergeZipsCmd} -D -stripFile "*.class" $out $outDir/classes.dex.jar $in`,
		CommandDeps: []string{
			"${config.DxCmd}",
			"${config.SoongZipCmd}",
			"${config.MergeZipsCmd}",
		},
	},
	"outDir", "dxFlags")

var d8 = pctx.AndroidStaticRule("d8",
	blueprint.RuleParams{
		Command: `rm -rf "$outDir" && mkdir -p "$outDir" && ` +
			`${config.D8Cmd} --output $outDir $dxFlags $in && ` +
			`${config.SoongZipCmd} -o $outDir/classes.dex.jar -C $outDir -D $outDir && ` +
			`${config.MergeZipsCmd} -D -stripFile "*.class" $out $outDir/classes.dex.jar $in`,
		CommandDeps: []string{
			"${config.D8Cmd}",
			"${config.SoongZipCmd}",
			"${config.MergeZipsCmd}",
		},
	},
	"outDir", "dxFlags")

func (j *Module) dxFlags(ctx android.ModuleContext, fullD8 bool) []string {
	flags := j.deviceProperties.Dxflags
	if fullD8 {
		// Translate all the DX flags to D8 ones until all the build files have been migrated
		// to D8 flags. See: b/69377755
		flags = android.RemoveListFromList(flags,
			[]string{"--core-library", "--dex", "--multi-dex"})
	}

	if ctx.Config().Getenv("NO_OPTIMIZE_DX") != "" {
		if fullD8 {
			flags = append(flags, "--debug")
		} else {
			flags = append(flags, "--no-optimize")
		}
	}

	if ctx.Config().Getenv("GENERATE_DEX_DEBUG") != "" {
		flags = append(flags,
			"--debug",
			"--verbose")
		if !fullD8 {
			flags = append(flags,
				"--dump-to="+android.PathForModuleOut(ctx, "classes.lst").String(),
				"--dump-width=1000")
		}
	}

	if fullD8 {
		flags = append(flags, "--min-api "+j.minSdkVersionNumber(ctx))
	} else {
		flags = append(flags, "--min-sdk-version="+j.minSdkVersionNumber(ctx))
	}
	return flags
}

func (j *Module) compileDex(ctx android.ModuleContext, flags javaBuilderFlags,
	classesJar android.Path, jarName string) android.Path {

	fullD8 := ctx.Config().UseD8Desugar()

	if !fullD8 {
		classesJar = j.desugar(ctx, flags, classesJar, jarName)
	}

	dxFlags := j.dxFlags(ctx, fullD8)

	// Compile classes.jar into classes.dex and then javalib.jar
	javalibJar := android.PathForModuleOut(ctx, "dex", jarName)
	outDir := android.PathForModuleOut(ctx, "dex")

	rule := dx
	desc := "dx"
	if fullD8 {
		rule = d8
		desc = "d8"
	}
	ctx.Build(pctx, android.BuildParams{
		Rule:        rule,
		Description: desc,
		Output:      javalibJar,
		Input:       classesJar,
		Args: map[string]string{
			"dxFlags": strings.Join(dxFlags, " "),
			"outDir":  outDir.String(),
		},
	})

	j.dexJarFile = javalibJar
	return javalibJar
}
