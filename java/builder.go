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
			Command: `rm -rf "$outDir" "$annoDir" && mkdir -p "$outDir" "$annoDir" && ` +
				`${config.JavacWrapper}${config.JavacCmd} ${config.JavacHeapFlags} ${config.CommonJdkFlags} ` +
				`$javacFlags $bootClasspath $classpath ` +
				`-source $javaVersion -target $javaVersion ` +
				`-d $outDir -s $annoDir @$out.rsp && ` +
				`${config.SoongZipCmd} -jar -o $out -C $outDir -D $outDir`,
			CommandDeps:    []string{"${config.JavacCmd}", "${config.SoongZipCmd}"},
			Rspfile:        "$out.rsp",
			RspfileContent: "$in",
		},
		"javacFlags", "bootClasspath", "classpath", "outDir", "annoDir", "javaVersion")

	errorprone = pctx.AndroidStaticRule("errorprone",
		blueprint.RuleParams{
			Command: `rm -rf "$outDir" "$annoDir" && mkdir -p "$outDir" "$annoDir" && ` +
				`${config.ErrorProneCmd} ` +
				`$javacFlags $bootClasspath $classpath ` +
				`-source $javaVersion -target $javaVersion ` +
				`-d $outDir -s $annoDir @$out.rsp && ` +
				`${config.SoongZipCmd} -jar -o $out -C $outDir -D $outDir`,
			CommandDeps: []string{
				"${config.JavaCmd}",
				"${config.ErrorProneJavacJar}",
				"${config.ErrorProneJar}",
				"${config.SoongZipCmd}",
			},
			Rspfile:        "$out.rsp",
			RspfileContent: "$in",
		},
		"javacFlags", "bootClasspath", "classpath", "outDir", "annoDir", "javaVersion")

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
			CommandDeps: []string{"${config.DesugarJar}"},
		},
		"javaFlags", "classpathFlags", "desugarFlags", "dumpDir")

	dx = pctx.AndroidStaticRule("dx",
		blueprint.RuleParams{
			Command: `rm -rf "$outDir" && mkdir -p "$outDir" && ` +
				`${config.DxCmd} --dex --output=$outDir $dxFlags $in && ` +
				`${config.SoongZipCmd} -jar -o $out -C $outDir -D $outDir`,
			CommandDeps: []string{
				"${config.DxCmd}",
				"${config.SoongZipCmd}",
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
	desugarFlags  string
	aidlFlags     string
	javaVersion   string

	protoFlags   string
	protoOutFlag string
}

func TransformJavaToClasses(ctx android.ModuleContext, srcFiles, srcFileLists android.Paths,
	flags javaBuilderFlags, deps android.Paths) android.ModuleOutPath {

	classDir := android.PathForModuleOut(ctx, "classes")
	annoDir := android.PathForModuleOut(ctx, "anno")
	classJar := android.PathForModuleOut(ctx, "classes-compiled.jar")

	javacFlags := flags.javacFlags
	if len(srcFileLists) > 0 {
		javacFlags += " " + android.JoinWithPrefix(srcFileLists.Strings(), "@")
	}

	deps = append(deps, srcFileLists...)
	deps = append(deps, flags.bootClasspath...)
	deps = append(deps, flags.classpath...)

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:        javac,
		Description: "javac",
		Output:      classJar,
		Inputs:      srcFiles,
		Implicits:   deps,
		Args: map[string]string{
			"javacFlags":    javacFlags,
			"bootClasspath": flags.bootClasspath.JavaBootClasspath(ctx.Device()),
			"classpath":     flags.classpath.JavaClasspath(),
			"outDir":        classDir.String(),
			"annoDir":       annoDir.String(),
			"javaVersion":   flags.javaVersion,
		},
	})

	return classJar
}

func RunErrorProne(ctx android.ModuleContext, srcFiles, srcFileLists android.Paths,
	flags javaBuilderFlags) android.Path {

	if config.ErrorProneJar == "" {
		ctx.ModuleErrorf("cannot build with Error Prone, missing external/error_prone?")
		return nil
	}

	classDir := android.PathForModuleOut(ctx, "classes-errorprone")
	annoDir := android.PathForModuleOut(ctx, "anno-errorprone")
	classFileList := android.PathForModuleOut(ctx, "classes-errorprone.list")

	javacFlags := flags.javacFlags
	if len(srcFileLists) > 0 {
		javacFlags += " " + android.JoinWithPrefix(srcFileLists.Strings(), "@")
	}

	var deps android.Paths

	deps = append(deps, srcFileLists...)
	deps = append(deps, flags.bootClasspath...)
	deps = append(deps, flags.classpath...)

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:        errorprone,
		Description: "errorprone",
		Output:      classFileList,
		Inputs:      srcFiles,
		Implicits:   deps,
		Args: map[string]string{
			"javacFlags":    javacFlags,
			"bootClasspath": flags.bootClasspath.JavaBootClasspath(ctx.Device()),
			"classpath":     flags.classpath.JavaClasspath(),
			"outDir":        classDir.String(),
			"annoDir":       annoDir.String(),
			"javaVersion":   flags.javaVersion,
		},
	})

	return classFileList
}

func TransformResourcesToJar(ctx android.ModuleContext, jarArgs []string,
	deps android.Paths) android.Path {

	outputFile := android.PathForModuleOut(ctx, "res.jar")

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:        jar,
		Description: "jar",
		Output:      outputFile,
		Implicits:   deps,
		Args: map[string]string{
			"jarArgs": strings.Join(jarArgs, " "),
		},
	})

	return outputFile
}

func TransformJarsToJar(ctx android.ModuleContext, stem string, jars android.Paths,
	manifest android.OptionalPath, stripDirs bool) android.Path {

	outputFile := android.PathForModuleOut(ctx, stem)

	if len(jars) == 1 && !manifest.Valid() {
		return jars[0]
	}

	var deps android.Paths

	var jarArgs []string
	if manifest.Valid() {
		jarArgs = append(jarArgs, "-m "+manifest.String())
		deps = append(deps, manifest.Path())
	}

	if stripDirs {
		jarArgs = append(jarArgs, "-D")
	}

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:        combineJar,
		Description: "combine jars",
		Output:      outputFile,
		Inputs:      jars,
		Implicits:   deps,
		Args: map[string]string{
			"jarArgs": strings.Join(jarArgs, " "),
		},
	})

	return outputFile
}

func TransformDesugar(ctx android.ModuleContext, classesJar android.Path,
	flags javaBuilderFlags) android.Path {

	outputFile := android.PathForModuleOut(ctx, "classes-desugar.jar")
	dumpDir := android.PathForModuleOut(ctx, "desugar_dumped_classes")

	javaFlags := ""
	if ctx.AConfig().Getenv("EXPERIMENTAL_USE_OPENJDK9") != "" {
		javaFlags = "--add-opens java.base/java.lang.invoke=ALL-UNNAMED"
	}

	var desugarFlags []string
	desugarFlags = append(desugarFlags, flags.bootClasspath.DesugarBootClasspath()...)
	desugarFlags = append(desugarFlags, flags.classpath.DesugarClasspath()...)

	var deps android.Paths
	deps = append(deps, flags.bootClasspath...)
	deps = append(deps, flags.classpath...)

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
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

	return outputFile
}

func TransformClassesJarToDexJar(ctx android.ModuleContext, classesJar android.Path,
	flags javaBuilderFlags) android.Path {

	outDir := android.PathForModuleOut(ctx, "dex")
	outputFile := android.PathForModuleOut(ctx, "classes.dex.jar")

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:        dx,
		Description: "dx",
		Output:      outputFile,
		Input:       classesJar,
		Args: map[string]string{
			"dxFlags": flags.dxFlags,
			"outDir":  outDir.String(),
		},
	})

	return outputFile
}

func TransformJarJar(ctx android.ModuleContext, classesJar android.Path, rulesFile android.Path) android.ModuleOutPath {
	outputFile := android.PathForModuleOut(ctx, "classes-jarjar.jar")
	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:        jarjar,
		Description: "jarjar",
		Output:      outputFile,
		Input:       classesJar,
		Implicit:    rulesFile,
		Args: map[string]string{
			"rulesFile": rulesFile.String(),
		},
	})

	return outputFile
}

type classpath []android.Path

// Returns a -classpath argument in the form java or javac expects
func (x *classpath) JavaClasspath() string {
	if len(*x) > 0 {
		return "-classpath " + strings.Join(x.Strings(), ":")
	} else {
		return ""
	}
}

// Returns a -processorpath argument in the form java or javac expects
func (x *classpath) JavaProcessorpath() string {
	if len(*x) > 0 {
		return "-processorpath " + strings.Join(x.Strings(), ":")
	} else {
		return ""
	}
}

// Returns a -bootclasspath argument in the form java or javac expects.  If forceEmpty is true,
// returns -bootclasspath "" if the bootclasspath is empty to ensure javac does not fall back to the
// default bootclasspath.
func (x *classpath) JavaBootClasspath(forceEmpty bool) string {
	if len(*x) > 0 {
		return "-bootclasspath " + strings.Join(x.Strings(), ":")
	} else if forceEmpty {
		return `-bootclasspath ""`
	} else {
		return ""
	}
}

func (x *classpath) DesugarBootClasspath() []string {
	if x == nil || *x == nil {
		return nil
	}
	flags := make([]string, len(*x))
	for i, v := range *x {
		flags[i] = "--bootclasspath_entry " + v.String()
	}

	return flags
}

func (x *classpath) DesugarClasspath() []string {
	if x == nil || *x == nil {
		return nil
	}
	flags := make([]string, len(*x))
	for i, v := range *x {
		flags[i] = "--classpath_entry " + v.String()
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
