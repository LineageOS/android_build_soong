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

var r8 = pctx.AndroidStaticRule("r8",
	blueprint.RuleParams{
		Command: `rm -rf "$outDir" && mkdir -p "$outDir" && ` +
			`${config.R8Cmd} -injars $in --output $outDir ` +
			`--force-proguard-compatibility ` +
			`-printmapping $outDict ` +
			`$dxFlags $r8Flags && ` +
			`${config.SoongZipCmd} -o $outDir/classes.dex.jar -C $outDir -D $outDir && ` +
			`${config.MergeZipsCmd} -D -stripFile "*.class" $out $outDir/classes.dex.jar $in`,
		CommandDeps: []string{
			"${config.R8Cmd}",
			"${config.SoongZipCmd}",
			"${config.MergeZipsCmd}",
		},
	},
	"outDir", "outDict", "dxFlags", "r8Flags")

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

func (j *Module) r8Flags(ctx android.ModuleContext, flags javaBuilderFlags) (r8Flags []string, r8Deps android.Paths) {
	opt := j.deviceProperties.Optimize

	// When an app contains references to APIs that are not in the SDK specified by
	// its LOCAL_SDK_VERSION for example added by support library or by runtime
	// classes added by desugar, we artifically raise the "SDK version" "linked" by
	// ProGuard, to
	// - suppress ProGuard warnings of referencing symbols unknown to the lower SDK version.
	// - prevent ProGuard stripping subclass in the support library that extends class added in the higher SDK version.
	// See b/20667396
	var proguardRaiseDeps classpath
	ctx.VisitDirectDepsWithTag(proguardRaiseTag, func(dep android.Module) {
		proguardRaiseDeps = append(proguardRaiseDeps, dep.(Dependency).HeaderJars()...)
	})

	r8Flags = append(r8Flags, proguardRaiseDeps.FormJavaClassPath("-libraryjars"))
	r8Flags = append(r8Flags, flags.bootClasspath.FormJavaClassPath("-libraryjars"))
	r8Flags = append(r8Flags, flags.classpath.FormJavaClassPath("-libraryjars"))
	r8Flags = append(r8Flags, "-forceprocessing")

	flagFiles := android.Paths{
		android.PathForSource(ctx, "build/make/core/proguard.flags"),
	}

	if j.shouldInstrumentStatic(ctx) {
		flagFiles = append(flagFiles,
			android.PathForSource(ctx, "build/make/core/proguard.jacoco.flags"))
	}

	flagFiles = append(flagFiles, j.extraProguardFlagFiles...)
	// TODO(ccross): static android library proguard files

	r8Flags = append(r8Flags, android.JoinWithPrefix(flagFiles.Strings(), "-include "))
	r8Deps = append(r8Deps, flagFiles...)

	// TODO(b/70942988): This is included from build/make/core/proguard.flags
	r8Deps = append(r8Deps, android.PathForSource(ctx,
		"build/make/core/proguard_basic_keeps.flags"))

	r8Flags = append(r8Flags, j.deviceProperties.Optimize.Proguard_flags...)

	// TODO(ccross): Don't shrink app instrumentation tests by default.
	if !Bool(opt.Shrink) {
		r8Flags = append(r8Flags, "-dontshrink")
	}

	if !Bool(opt.Optimize) {
		r8Flags = append(r8Flags, "-dontoptimize")
	}

	// TODO(ccross): error if obufscation + app instrumentation test.
	if !Bool(opt.Obfuscate) {
		r8Flags = append(r8Flags, "-dontobfuscate")
	}

	return r8Flags, r8Deps
}

func (j *Module) compileDex(ctx android.ModuleContext, flags javaBuilderFlags,
	classesJar android.Path, jarName string) android.Path {

	useR8 := Bool(j.deviceProperties.Optimize.Enabled)
	fullD8 := useR8 || ctx.Config().UseD8Desugar()

	if !fullD8 {
		classesJar = j.desugar(ctx, flags, classesJar, jarName)
	}

	dxFlags := j.dxFlags(ctx, fullD8)

	// Compile classes.jar into classes.dex and then javalib.jar
	javalibJar := android.PathForModuleOut(ctx, "dex", jarName)
	outDir := android.PathForModuleOut(ctx, "dex")

	if useR8 {
		// TODO(ccross): if this is an instrumentation test of an obfuscated app, use the
		// dictionary of the app and move the app from libraryjars to injars.
		proguardDictionary := android.PathForModuleOut(ctx, "proguard_dictionary")
		j.proguardDictionary = proguardDictionary
		r8Flags, r8Deps := j.r8Flags(ctx, flags)
		ctx.Build(pctx, android.BuildParams{
			Rule:           r8,
			Description:    "r8",
			Output:         javalibJar,
			ImplicitOutput: proguardDictionary,
			Input:          classesJar,
			Implicits:      r8Deps,
			Args: map[string]string{
				"dxFlags": strings.Join(dxFlags, " "),
				"r8Flags": strings.Join(r8Flags, " "),
				"outDict": j.proguardDictionary.String(),
				"outDir":  outDir.String(),
			},
		})
	} else {
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
	}

	j.dexJarFile = javalibJar
	return javalibJar
}
