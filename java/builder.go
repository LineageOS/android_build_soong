// Copyright 2015 Google Inc. All rights reserved.
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

// This file generates the final rules for compiling all Java.  All properties related to
// compiling should have been translated into javaBuilderFlags or another argument to the Transform*
// functions.

import (
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
	"android/soong/java/config"
)

var (
	pctx = android.NewPackageContext("android/soong/java")

	// Compiling java is not conducive to proper dependency tracking.  The path-matches-class-name
	// requirement leads to unpredictable generated source file names, and a single .java file
	// will get compiled into multiple .class files if it contains inner classes.  To work around
	// this, all java rules write into separate directories and then a post-processing step lists
	// the files in the the directory into a list file that later rules depend on (and sometimes
	// read from directly using @<listfile>)
	javac = pctx.AndroidGomaStaticRule("javac",
		blueprint.RuleParams{
			Command: `rm -rf "$outDir" "$annoDir" "$srcJarDir" && mkdir -p "$outDir" "$annoDir" "$srcJarDir" && ` +
				`${config.ExtractSrcJarsCmd} $srcJarDir $srcJarDir/list $srcJars && ` +
				`${config.SoongJavacWrapper} ${config.JavacWrapper}${config.JavacCmd} ${config.JavacHeapFlags} ${config.CommonJdkFlags} ` +
				`$javacFlags $bootClasspath $classpath ` +
				`-source $javaVersion -target $javaVersion ` +
				`-d $outDir -s $annoDir @$out.rsp @$srcJarDir/list && ` +
				`${config.SoongZipCmd} -jar -o $out -C $outDir -D $outDir`,
			CommandDeps: []string{
				"${config.JavacCmd}",
				"${config.SoongZipCmd}",
				"${config.ExtractSrcJarsCmd}",
			},
			CommandOrderOnly: []string{"${config.SoongJavacWrapper}"},
			Rspfile:          "$out.rsp",
			RspfileContent:   "$in",
		},
		"javacFlags", "bootClasspath", "classpath", "srcJars", "srcJarDir",
		"outDir", "annoDir", "javaVersion")

	kotlinc = pctx.AndroidGomaStaticRule("kotlinc",
		blueprint.RuleParams{
			// TODO(ccross): kotlinc doesn't support @ file for arguments, which will limit the
			// maximum number of input files, especially on darwin.
			Command: `rm -rf "$outDir" && mkdir -p "$outDir" && ` +
				`${config.KotlincCmd} $classpath $kotlincFlags ` +
				`-jvm-target $javaVersion -d $outDir $in && ` +
				`${config.SoongZipCmd} -jar -o $out -C $outDir -D $outDir`,
			CommandDeps: []string{
				"${config.KotlincCmd}",
				"${config.KotlinCompilerJar}",
				"${config.SoongZipCmd}",
			},
		},
		"kotlincFlags", "classpath", "outDir", "javaVersion")

	errorprone = pctx.AndroidStaticRule("errorprone",
		blueprint.RuleParams{
			Command: `rm -rf "$outDir" "$annoDir" "$srcJarDir" && mkdir -p "$outDir" "$annoDir" "$srcJarDir" && ` +
				`${config.ExtractSrcJarsCmd} $srcJarDir $srcJarDir/list $srcJars && ` +
				`${config.SoongJavacWrapper} ${config.ErrorProneCmd} ` +
				`$javacFlags $bootClasspath $classpath ` +
				`-source $javaVersion -target $javaVersion ` +
				`-d $outDir -s $annoDir @$out.rsp @$srcJarDir/list && ` +
				`${config.SoongZipCmd} -jar -o $out -C $outDir -D $outDir`,
			CommandDeps: []string{
				"${config.JavaCmd}",
				"${config.ErrorProneJavacJar}",
				"${config.ErrorProneJar}",
				"${config.SoongZipCmd}",
				"${config.ExtractSrcJarsCmd}",
			},
			CommandOrderOnly: []string{"${config.SoongJavacWrapper}"},
			Rspfile:          "$out.rsp",
			RspfileContent:   "$in",
		},
		"javacFlags", "bootClasspath", "classpath", "srcJars", "srcJarDir",
		"outDir", "annoDir", "javaVersion")

	turbine = pctx.AndroidStaticRule("turbine",
		blueprint.RuleParams{
			Command: `rm -rf "$outDir" "$srcJarDir" && mkdir -p "$outDir" "$srcJarDir" && ` +
				`${config.ExtractSrcJarsCmd} $srcJarDir $srcJarDir/list $srcJars && ` +
				`${config.JavaCmd} -jar ${config.TurbineJar} --output $out.tmp ` +
				`--temp_dir "$outDir" --sources @$out.rsp @$srcJarDir/list ` +
				`--javacopts ${config.CommonJdkFlags} ` +
				`$javacFlags -source $javaVersion -target $javaVersion $bootClasspath $classpath && ` +
				`${config.Ziptime} $out.tmp && ` +
				`(if cmp -s $out.tmp $out ; then rm $out.tmp ; else mv $out.tmp $out ; fi )`,
			CommandDeps: []string{
				"${config.TurbineJar}",
				"${config.JavaCmd}",
				"${config.Ziptime}",
				"${config.ExtractSrcJarsCmd}",
			},
			Rspfile:        "$out.rsp",
			RspfileContent: "$in",
			Restat:         true,
		},
		"javacFlags", "bootClasspath", "classpath", "srcJars", "srcJarDir",
		"outDir", "javaVersion")

	jar = pctx.AndroidStaticRule("jar",
		blueprint.RuleParams{
			Command:     `${config.SoongZipCmd} -jar -o $out $jarArgs`,
			CommandDeps: []string{"${config.SoongZipCmd}"},
		},
		"jarArgs")

	combineJar = pctx.AndroidStaticRule("combineJar",
		blueprint.RuleParams{
			Command:     `${config.MergeZipsCmd} -j $jarArgs $out $in`,
			CommandDeps: []string{"${config.MergeZipsCmd}"},
		},
		"jarArgs")

	desugar = pctx.AndroidStaticRule("desugar",
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

	dx = pctx.AndroidStaticRule("dx",
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

	jarjar = pctx.AndroidStaticRule("jarjar",
		blueprint.RuleParams{
			Command:     "${config.JavaCmd} -jar ${config.JarjarCmd} process $rulesFile $in $out",
			CommandDeps: []string{"${config.JavaCmd}", "${config.JarjarCmd}", "$rulesFile"},
		},
		"rulesFile")
)

func init() {
	pctx.Import("android/soong/java/config")
}

type javaBuilderFlags struct {
	javacFlags    string
	dxFlags       string
	bootClasspath classpath
	classpath     classpath
	systemModules classpath
	desugarFlags  string
	aidlFlags     string
	javaVersion   string

	kotlincFlags     string
	kotlincClasspath classpath

	protoFlags   string
	protoOutFlag string
}

func TransformKotlinToClasses(ctx android.ModuleContext, outputFile android.WritablePath,
	srcFiles, srcJars android.Paths,
	flags javaBuilderFlags) {

	classDir := android.PathForModuleOut(ctx, "kotlinc", "classes")

	inputs := append(android.Paths(nil), srcFiles...)
	inputs = append(inputs, srcJars...)

	ctx.Build(pctx, android.BuildParams{
		Rule:        kotlinc,
		Description: "kotlinc",
		Output:      outputFile,
		Inputs:      inputs,
		Args: map[string]string{
			"classpath":    flags.kotlincClasspath.FormJavaClassPath("-classpath"),
			"kotlincFlags": flags.kotlincFlags,
			"outDir":       classDir.String(),
			"javaVersion":  flags.javaVersion,
		},
	})
}

func TransformJavaToClasses(ctx android.ModuleContext, outputFile android.WritablePath,
	srcFiles, srcJars android.Paths,
	flags javaBuilderFlags, deps android.Paths) {

	transformJavaToClasses(ctx, outputFile, srcFiles, srcJars, flags, deps,
		"javac", "javac", javac)
}

func RunErrorProne(ctx android.ModuleContext, outputFile android.WritablePath,
	srcFiles, srcJars android.Paths, flags javaBuilderFlags) {

	if config.ErrorProneJar == "" {
		ctx.ModuleErrorf("cannot build with Error Prone, missing external/error_prone?")
	}

	transformJavaToClasses(ctx, outputFile, srcFiles, srcJars, flags, nil,
		"errorprone", "errorprone", errorprone)
}

func TransformJavaToHeaderClasses(ctx android.ModuleContext, outputFile android.WritablePath,
	srcFiles, srcJars android.Paths, flags javaBuilderFlags) {

	var deps android.Paths
	deps = append(deps, srcJars...)
	deps = append(deps, flags.bootClasspath...)
	deps = append(deps, flags.classpath...)

	var bootClasspath string
	if len(flags.bootClasspath) == 0 && ctx.Device() {
		// explicitly specify -bootclasspath "" if the bootclasspath is empty to
		// ensure java does not fall back to the default bootclasspath.
		bootClasspath = `--bootclasspath ""`
	} else {
		bootClasspath = flags.bootClasspath.FormJavaClassPath("--bootclasspath")
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        turbine,
		Description: "turbine",
		Output:      outputFile,
		Inputs:      srcFiles,
		Implicits:   deps,
		Args: map[string]string{
			"javacFlags":    flags.javacFlags,
			"bootClasspath": bootClasspath,
			"srcJars":       strings.Join(srcJars.Strings(), " "),
			"srcJarDir":     android.PathForModuleOut(ctx, "turbine", "srcjars").String(),
			"classpath":     flags.classpath.FormJavaClassPath("--classpath"),
			"outDir":        android.PathForModuleOut(ctx, "turbine", "classes").String(),
			"javaVersion":   flags.javaVersion,
		},
	})
}

// transformJavaToClasses takes source files and converts them to a jar containing .class files.
// srcFiles is a list of paths to sources, srcJars is a list of paths to jar files that contain
// sources.  flags contains various command line flags to be passed to the compiler.
//
// This method may be used for different compilers, including javac and Error Prone.  The rule
// argument specifies which command line to use and desc sets the description of the rule that will
// be printed at build time.  The stem argument provides the file name of the output jar, and
// suffix will be appended to various intermediate files and directories to avoid collisions when
// this function is called twice in the same module directory.
func transformJavaToClasses(ctx android.ModuleContext, outputFile android.WritablePath,
	srcFiles, srcJars android.Paths,
	flags javaBuilderFlags, deps android.Paths,
	intermediatesDir, desc string, rule blueprint.Rule) {

	deps = append(deps, srcJars...)

	var bootClasspath string
	if flags.javaVersion == "1.9" {
		deps = append(deps, flags.systemModules...)
		bootClasspath = flags.systemModules.FormJavaSystemModulesPath("--system=", ctx.Device())
	} else {
		deps = append(deps, flags.bootClasspath...)
		if len(flags.bootClasspath) == 0 && ctx.Device() {
			// explicitly specify -bootclasspath "" if the bootclasspath is empty to
			// ensure java does not fall back to the default bootclasspath.
			bootClasspath = `-bootclasspath ""`
		} else {
			bootClasspath = flags.bootClasspath.FormJavaClassPath("-bootclasspath")
		}
	}

	deps = append(deps, flags.classpath...)

	ctx.Build(pctx, android.BuildParams{
		Rule:        rule,
		Description: desc,
		Output:      outputFile,
		Inputs:      srcFiles,
		Implicits:   deps,
		Args: map[string]string{
			"javacFlags":    flags.javacFlags,
			"bootClasspath": bootClasspath,
			"classpath":     flags.classpath.FormJavaClassPath("-classpath"),
			"srcJars":       strings.Join(srcJars.Strings(), " "),
			"srcJarDir":     android.PathForModuleOut(ctx, intermediatesDir, "srcjars").String(),
			"outDir":        android.PathForModuleOut(ctx, intermediatesDir, "classes").String(),
			"annoDir":       android.PathForModuleOut(ctx, intermediatesDir, "anno").String(),
			"javaVersion":   flags.javaVersion,
		},
	})
}

func TransformResourcesToJar(ctx android.ModuleContext, outputFile android.WritablePath,
	jarArgs []string, deps android.Paths) {

	ctx.Build(pctx, android.BuildParams{
		Rule:        jar,
		Description: "jar",
		Output:      outputFile,
		Implicits:   deps,
		Args: map[string]string{
			"jarArgs": strings.Join(jarArgs, " "),
		},
	})
}

func TransformJarsToJar(ctx android.ModuleContext, outputFile android.WritablePath, desc string,
	jars android.Paths, manifest android.OptionalPath, stripDirs bool, dirsToStrip []string) {

	var deps android.Paths

	var jarArgs []string
	if manifest.Valid() {
		jarArgs = append(jarArgs, "-m ", manifest.String())
		deps = append(deps, manifest.Path())
	}

	if dirsToStrip != nil {
		for _, dir := range dirsToStrip {
			jarArgs = append(jarArgs, "-stripDir ", dir)
		}
	}

	if stripDirs {
		jarArgs = append(jarArgs, "-D")
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        combineJar,
		Description: desc,
		Output:      outputFile,
		Inputs:      jars,
		Implicits:   deps,
		Args: map[string]string{
			"jarArgs": strings.Join(jarArgs, " "),
		},
	})
}

func TransformDesugar(ctx android.ModuleContext, outputFile android.WritablePath,
	classesJar android.Path, flags javaBuilderFlags) {

	dumpDir := android.PathForModuleOut(ctx, "desugar", "classes")

	javaFlags := ""
	if ctx.AConfig().UseOpenJDK9() {
		javaFlags = "--add-opens java.base/java.lang.invoke=ALL-UNNAMED"
	}

	var desugarFlags []string
	desugarFlags = append(desugarFlags, flags.bootClasspath.FormDesugarClasspath("--bootclasspath_entry")...)
	desugarFlags = append(desugarFlags, flags.classpath.FormDesugarClasspath("--classpath_entry")...)

	var deps android.Paths
	deps = append(deps, flags.bootClasspath...)
	deps = append(deps, flags.classpath...)

	ctx.Build(pctx, android.BuildParams{
		Rule:        desugar,
		Description: "desugar",
		Output:      outputFile,
		Input:       classesJar,
		Implicits:   deps,
		Args: map[string]string{
			"dumpDir":        dumpDir.String(),
			"javaFlags":      javaFlags,
			"classpathFlags": strings.Join(desugarFlags, " "),
			"desugarFlags":   flags.desugarFlags,
		},
	})
}

// Converts a classes.jar file to classes*.dex, then combines the dex files with any resources
// in the classes.jar file into a dex jar.
func TransformClassesJarToDexJar(ctx android.ModuleContext, outputFile android.WritablePath,
	classesJar android.Path, flags javaBuilderFlags) {

	outDir := android.PathForModuleOut(ctx, "dex")

	ctx.Build(pctx, android.BuildParams{
		Rule:        dx,
		Description: "dx",
		Output:      outputFile,
		Input:       classesJar,
		Args: map[string]string{
			"dxFlags": flags.dxFlags,
			"outDir":  outDir.String(),
		},
	})
}

func TransformJarJar(ctx android.ModuleContext, outputFile android.WritablePath,
	classesJar android.Path, rulesFile android.Path) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        jarjar,
		Description: "jarjar",
		Output:      outputFile,
		Input:       classesJar,
		Implicit:    rulesFile,
		Args: map[string]string{
			"rulesFile": rulesFile.String(),
		},
	})
}

type classpath []android.Path

func (x *classpath) FormJavaClassPath(optName string) string {
	if len(*x) > 0 {
		return optName + " " + strings.Join(x.Strings(), ":")
	} else {
		return ""
	}
}

// Returns a --system argument in the form javac expects with -source 1.9.  If forceEmpty is true,
// returns --system=none if the list is empty to ensure javac does not fall back to the default
// system modules.
func (x *classpath) FormJavaSystemModulesPath(optName string, forceEmpty bool) string {
	if len(*x) > 1 {
		panic("more than one system module")
	} else if len(*x) == 1 {
		return optName + strings.TrimSuffix((*x)[0].String(), "lib/modules")
	} else if forceEmpty {
		return optName + "none"
	} else {
		return ""
	}
}

func (x *classpath) FormDesugarClasspath(optName string) []string {
	if x == nil || *x == nil {
		return nil
	}
	flags := make([]string, len(*x))
	for i, v := range *x {
		flags[i] = optName + " " + v.String()
	}

	return flags
}

// Append an android.Paths to the end of the classpath list
func (x *classpath) AddPaths(paths android.Paths) {
	for _, path := range paths {
		*x = append(*x, path)
	}
}

// Convert a classpath to an android.Paths
func (x *classpath) Paths() android.Paths {
	return append(android.Paths(nil), (*x)...)
}

func (x *classpath) Strings() []string {
	if x == nil {
		return nil
	}
	ret := make([]string, len(*x))
	for i, path := range *x {
		ret[i] = path.String()
	}
	return ret
}
