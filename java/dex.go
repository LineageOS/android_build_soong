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
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/remoteexec"
)

var d8, d8RE = remoteexec.MultiCommandStaticRules(pctx, "d8",
	blueprint.RuleParams{
		Command: `rm -rf "$outDir" && mkdir -p "$outDir" && ` +
			`$d8Template${config.D8Cmd} ${config.DexFlags} --output $outDir $d8Flags $in && ` +
			`$zipTemplate${config.SoongZipCmd} $zipFlags -o $outDir/classes.dex.jar -C $outDir -f "$outDir/classes*.dex" && ` +
			`${config.MergeZipsCmd} -D -stripFile "**/*.class" $out $outDir/classes.dex.jar $in`,
		CommandDeps: []string{
			"${config.D8Cmd}",
			"${config.SoongZipCmd}",
			"${config.MergeZipsCmd}",
		},
	}, map[string]*remoteexec.REParams{
		"$d8Template": &remoteexec.REParams{
			Labels:          map[string]string{"type": "compile", "compiler": "d8"},
			Inputs:          []string{"${config.D8Jar}"},
			ExecStrategy:    "${config.RED8ExecStrategy}",
			ToolchainInputs: []string{"${config.JavaCmd}"},
			Platform:        map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
		"$zipTemplate": &remoteexec.REParams{
			Labels:       map[string]string{"type": "tool", "name": "soong_zip"},
			Inputs:       []string{"${config.SoongZipCmd}", "$outDir"},
			OutputFiles:  []string{"$outDir/classes.dex.jar"},
			ExecStrategy: "${config.RED8ExecStrategy}",
			Platform:     map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
	}, []string{"outDir", "d8Flags", "zipFlags"}, nil)

var r8, r8RE = remoteexec.MultiCommandStaticRules(pctx, "r8",
	blueprint.RuleParams{
		Command: `rm -rf "$outDir" && mkdir -p "$outDir" && ` +
			`rm -f "$outDict" && ` +
			`$r8Template${config.R8Cmd} ${config.DexFlags} -injars $in --output $outDir ` +
			`--force-proguard-compatibility ` +
			`--no-data-resources ` +
			`-printmapping $outDict ` +
			`$r8Flags && ` +
			`touch "$outDict" && ` +
			`$zipTemplate${config.SoongZipCmd} $zipFlags -o $outDir/classes.dex.jar -C $outDir -f "$outDir/classes*.dex" && ` +
			`${config.MergeZipsCmd} -D -stripFile "**/*.class" $out $outDir/classes.dex.jar $in`,
		CommandDeps: []string{
			"${config.R8Cmd}",
			"${config.SoongZipCmd}",
			"${config.MergeZipsCmd}",
		},
	}, map[string]*remoteexec.REParams{
		"$r8Template": &remoteexec.REParams{
			Labels:          map[string]string{"type": "compile", "compiler": "r8"},
			Inputs:          []string{"$implicits", "${config.R8Jar}"},
			ExecStrategy:    "${config.RER8ExecStrategy}",
			ToolchainInputs: []string{"${config.JavaCmd}"},
			Platform:        map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
		"$zipTemplate": &remoteexec.REParams{
			Labels:       map[string]string{"type": "tool", "name": "soong_zip"},
			Inputs:       []string{"${config.SoongZipCmd}", "$outDir"},
			OutputFiles:  []string{"$outDir/classes.dex.jar"},
			ExecStrategy: "${config.RER8ExecStrategy}",
			Platform:     map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
	}, []string{"outDir", "outDict", "r8Flags", "zipFlags"}, []string{"implicits"})

func (j *Module) dexCommonFlags(ctx android.ModuleContext) []string {
	flags := j.deviceProperties.Dxflags
	// Translate all the DX flags to D8 ones until all the build files have been migrated
	// to D8 flags. See: b/69377755
	flags = android.RemoveListFromList(flags,
		[]string{"--core-library", "--dex", "--multi-dex"})

	if ctx.Config().Getenv("NO_OPTIMIZE_DX") != "" {
		flags = append(flags, "--debug")
	}

	if ctx.Config().Getenv("GENERATE_DEX_DEBUG") != "" {
		flags = append(flags,
			"--debug",
			"--verbose")
	}

	minSdkVersion, err := j.minSdkVersion().effectiveVersion(ctx)
	if err != nil {
		ctx.PropertyErrorf("min_sdk_version", "%s", err)
	}

	flags = append(flags, "--min-api "+minSdkVersion.asNumberString())
	return flags
}

func (j *Module) d8Flags(ctx android.ModuleContext, flags javaBuilderFlags) ([]string, android.Paths) {
	d8Flags := j.dexCommonFlags(ctx)

	d8Flags = append(d8Flags, flags.bootClasspath.FormRepeatedClassPath("--lib ")...)
	d8Flags = append(d8Flags, flags.classpath.FormRepeatedClassPath("--lib ")...)

	var d8Deps android.Paths
	d8Deps = append(d8Deps, flags.bootClasspath...)
	d8Deps = append(d8Deps, flags.classpath...)

	return d8Flags, d8Deps
}

func (j *Module) r8Flags(ctx android.ModuleContext, flags javaBuilderFlags) (r8Flags []string, r8Deps android.Paths) {
	opt := j.deviceProperties.Optimize

	// When an app contains references to APIs that are not in the SDK specified by
	// its LOCAL_SDK_VERSION for example added by support library or by runtime
	// classes added by desugaring, we artifically raise the "SDK version" "linked" by
	// ProGuard, to
	// - suppress ProGuard warnings of referencing symbols unknown to the lower SDK version.
	// - prevent ProGuard stripping subclass in the support library that extends class added in the higher SDK version.
	// See b/20667396
	var proguardRaiseDeps classpath
	ctx.VisitDirectDepsWithTag(proguardRaiseTag, func(dep android.Module) {
		proguardRaiseDeps = append(proguardRaiseDeps, dep.(Dependency).HeaderJars()...)
	})

	r8Flags = append(r8Flags, j.dexCommonFlags(ctx)...)

	r8Flags = append(r8Flags, proguardRaiseDeps.FormJavaClassPath("-libraryjars"))
	r8Flags = append(r8Flags, flags.bootClasspath.FormJavaClassPath("-libraryjars"))
	r8Flags = append(r8Flags, flags.classpath.FormJavaClassPath("-libraryjars"))

	r8Deps = append(r8Deps, proguardRaiseDeps...)
	r8Deps = append(r8Deps, flags.bootClasspath...)
	r8Deps = append(r8Deps, flags.classpath...)

	flagFiles := android.Paths{
		android.PathForSource(ctx, "build/make/core/proguard.flags"),
	}

	if j.shouldInstrumentStatic(ctx) {
		flagFiles = append(flagFiles,
			android.PathForSource(ctx, "build/make/core/proguard.jacoco.flags"))
	}

	flagFiles = append(flagFiles, j.extraProguardFlagFiles...)
	// TODO(ccross): static android library proguard files

	flagFiles = append(flagFiles, android.PathsForModuleSrc(ctx, j.deviceProperties.Optimize.Proguard_flags_files)...)

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
	// TODO(ccross): if this is an instrumentation test of an obfuscated app, use the
	// dictionary of the app and move the app from libraryjars to injars.

	// Don't strip out debug information for eng builds.
	if ctx.Config().Eng() {
		r8Flags = append(r8Flags, "--debug")
	}

	return r8Flags, r8Deps
}

func (j *Module) compileDex(ctx android.ModuleContext, flags javaBuilderFlags,
	classesJar android.Path, jarName string) android.ModuleOutPath {

	useR8 := j.deviceProperties.EffectiveOptimizeEnabled()

	// Compile classes.jar into classes.dex and then javalib.jar
	javalibJar := android.PathForModuleOut(ctx, "dex", jarName)
	outDir := android.PathForModuleOut(ctx, "dex")

	zipFlags := "--ignore_missing_files"
	if proptools.Bool(j.deviceProperties.Uncompress_dex) {
		zipFlags += " -L 0"
	}

	if useR8 {
		proguardDictionary := android.PathForModuleOut(ctx, "proguard_dictionary")
		j.proguardDictionary = proguardDictionary
		r8Flags, r8Deps := j.r8Flags(ctx, flags)
		rule := r8
		args := map[string]string{
			"r8Flags":  strings.Join(r8Flags, " "),
			"zipFlags": zipFlags,
			"outDict":  j.proguardDictionary.String(),
			"outDir":   outDir.String(),
		}
		if ctx.Config().IsEnvTrue("RBE_R8") {
			rule = r8RE
			args["implicits"] = strings.Join(r8Deps.Strings(), ",")
		}
		ctx.Build(pctx, android.BuildParams{
			Rule:           rule,
			Description:    "r8",
			Output:         javalibJar,
			ImplicitOutput: proguardDictionary,
			Input:          classesJar,
			Implicits:      r8Deps,
			Args:           args,
		})
	} else {
		d8Flags, d8Deps := j.d8Flags(ctx, flags)
		rule := d8
		if ctx.Config().IsEnvTrue("RBE_D8") {
			rule = d8RE
		}
		ctx.Build(pctx, android.BuildParams{
			Rule:        rule,
			Description: "d8",
			Output:      javalibJar,
			Input:       classesJar,
			Implicits:   d8Deps,
			Args: map[string]string{
				"d8Flags":  strings.Join(d8Flags, " "),
				"zipFlags": zipFlags,
				"outDir":   outDir.String(),
			},
		})
	}
	if proptools.Bool(j.deviceProperties.Uncompress_dex) {
		alignedJavalibJar := android.PathForModuleOut(ctx, "aligned", jarName)
		TransformZipAlign(ctx, alignedJavalibJar, javalibJar)
		javalibJar = alignedJavalibJar
	}

	return javalibJar
}
