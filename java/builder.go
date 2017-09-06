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
				`find $outDir -type f | sort | ${config.JarArgsCmd} $outDir > $out`,
			CommandDeps:    []string{"${config.JavacCmd}", "${config.JarArgsCmd}"},
			Rspfile:        "$out.rsp",
			RspfileContent: "$in",
		},
		"javacFlags", "bootClasspath", "classpath", "outDir", "annoDir", "javaVersion")

	errorprone = pctx.AndroidStaticRule("errorprone",
		blueprint.RuleParams{
			Command: `rm -rf "$outDir" "$annoDir" && mkdir -p "$outDir" "$annoDir" && ` +
				`${config.ErrorProneCmd}` +
				`$javacFlags $bootClasspath $classpath ` +
				`-source $javaVersion -target $javaVersion ` +
				`-d $outDir -s $annoDir @$out.rsp && ` +
				`find $outDir -type f | sort | ${config.JarArgsCmd} $outDir > $out`,
			CommandDeps: []string{
				"${config.JavaCmd}",
				"${config.ErrorProneJavacJar}",
				"${config.ErrorProneJar}",
				"${config.JarArgsCmd}",
			},
			Rspfile:        "$out.rsp",
			RspfileContent: "$in",
		},
		"javacFlags", "bootClasspath", "classpath", "outDir", "annoDir", "javaVersion")

	jar = pctx.AndroidStaticRule("jar",
		blueprint.RuleParams{
			Command:     `${config.JarCmd} $operation ${out}.tmp $manifest $jarArgs && ${config.Zip2ZipCmd} -t -i ${out}.tmp -o ${out} && rm ${out}.tmp`,
			CommandDeps: []string{"${config.JarCmd}"},
		},
		"operation", "manifest", "jarArgs")

	dx = pctx.AndroidStaticRule("dx",
		blueprint.RuleParams{
			Command: `rm -rf "$outDir" && mkdir -p "$outDir" && ` +
				`${config.DxCmd} --dex --output=$outDir $dxFlags $in && ` +
				`find "$outDir" -name "classes*.dex" | sort | ${config.JarArgsCmd} ${outDir} > $out`,
			CommandDeps: []string{"${config.DxCmd}", "${config.JarArgsCmd}"},
		},
		"outDir", "dxFlags")

	jarjar = pctx.AndroidStaticRule("jarjar",
		blueprint.RuleParams{
			Command:     "${config.JavaCmd} -jar ${config.JarjarCmd} process $rulesFile $in $out",
			CommandDeps: []string{"${config.JavaCmd}", "${config.JarjarCmd}", "$rulesFile"},
		},
		"rulesFile")

	extractPrebuilt = pctx.AndroidStaticRule("extractPrebuilt",
		blueprint.RuleParams{
			Command: `rm -rf $outDir && unzip -qo $in -d $outDir && ` +
				`find $outDir -name "*.class" | sort | ${config.JarArgsCmd} ${outDir} > $classFile && ` +
				`find $outDir -type f -a \! -name "*.class" -a \! -name "MANIFEST.MF" | sort | ${config.JarArgsCmd} ${outDir} > $resourceFile`,
			CommandDeps: []string{"${config.JarArgsCmd}"},
		},
		"outDir", "classFile", "resourceFile")

	fileListToJarArgs = pctx.AndroidStaticRule("fileListToJarArgs",
		blueprint.RuleParams{
			Command:     `${config.JarArgsCmd} -f $in -p ${outDir} -o $out`,
			CommandDeps: []string{"${config.JarjarCmd}"},
		},
		"outDir")
)

func init() {
	pctx.Import("android/soong/java/config")
}

type javaBuilderFlags struct {
	javacFlags    string
	dxFlags       string
	bootClasspath string
	classpath     string
	aidlFlags     string
	javaVersion   string
}

type jarSpec struct {
	android.ModuleOutPath
}

func (j jarSpec) jarArgs() string {
	return "@" + j.String()
}

func (j jarSpec) path() android.Path {
	return j.ModuleOutPath
}

func TransformJavaToClasses(ctx android.ModuleContext, srcFiles android.Paths, srcFileLists android.Paths,
	flags javaBuilderFlags, deps android.Paths) jarSpec {

	classDir := android.PathForModuleOut(ctx, "classes")
	annoDir := android.PathForModuleOut(ctx, "anno")
	classFileList := android.PathForModuleOut(ctx, "classes.list")

	javacFlags := flags.javacFlags + android.JoinWithPrefix(srcFileLists.Strings(), "@")

	deps = append(deps, srcFileLists...)

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:        javac,
		Description: "javac",
		Output:      classFileList,
		Inputs:      srcFiles,
		Implicits:   deps,
		Args: map[string]string{
			"javacFlags":    javacFlags,
			"bootClasspath": flags.bootClasspath,
			"classpath":     flags.classpath,
			"outDir":        classDir.String(),
			"annoDir":       annoDir.String(),
			"javaVersion":   flags.javaVersion,
		},
	})

	return jarSpec{classFileList}
}

func RunErrorProne(ctx android.ModuleContext, srcFiles android.Paths, srcFileLists android.Paths,
	flags javaBuilderFlags, deps android.Paths) android.Path {

	if config.ErrorProneJar == "" {
		ctx.ModuleErrorf("cannot build with Error Prone, missing external/error_prone?")
		return nil
	}

	classDir := android.PathForModuleOut(ctx, "classes-errorprone")
	annoDir := android.PathForModuleOut(ctx, "anno-errorprone")
	classFileList := android.PathForModuleOut(ctx, "classes-errorprone.list")

	javacFlags := flags.javacFlags + android.JoinWithPrefix(srcFileLists.Strings(), "@")

	deps = append(deps, srcFileLists...)

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:        errorprone,
		Description: "errorprone",
		Output:      classFileList,
		Inputs:      srcFiles,
		Implicits:   deps,
		Args: map[string]string{
			"javacFlags":    javacFlags,
			"bootClasspath": flags.bootClasspath,
			"classpath":     flags.classpath,
			"outDir":        classDir.String(),
			"annoDir":       annoDir.String(),
			"javaVersion":   flags.javaVersion,
		},
	})

	return classFileList
}

func TransformClassesToJar(ctx android.ModuleContext, classes []jarSpec,
	manifest android.OptionalPath, deps android.Paths) android.Path {

	outputFile := android.PathForModuleOut(ctx, "classes-full-debug.jar")

	jarArgs := []string{}

	for _, j := range classes {
		deps = append(deps, j.path())
		jarArgs = append(jarArgs, j.jarArgs())
	}

	operation := "cf"
	if manifest.Valid() {
		operation = "cfm"
		deps = append(deps, manifest.Path())
	}

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:        jar,
		Description: "jar",
		Output:      outputFile,
		Implicits:   deps,
		Args: map[string]string{
			"jarArgs":   strings.Join(jarArgs, " "),
			"operation": operation,
			"manifest":  manifest.String(),
		},
	})

	return outputFile
}

func TransformClassesJarToDex(ctx android.ModuleContext, classesJar android.Path,
	flags javaBuilderFlags) jarSpec {

	outDir := android.PathForModuleOut(ctx, "dex")
	outputFile := android.PathForModuleOut(ctx, "dex.filelist")

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

	return jarSpec{outputFile}
}

func TransformDexToJavaLib(ctx android.ModuleContext, resources []jarSpec,
	dexJarSpec jarSpec) android.Path {

	outputFile := android.PathForModuleOut(ctx, "javalib.jar")
	var deps android.Paths
	var jarArgs []string

	for _, j := range resources {
		deps = append(deps, j.path())
		jarArgs = append(jarArgs, j.jarArgs())
	}

	deps = append(deps, dexJarSpec.path())
	jarArgs = append(jarArgs, dexJarSpec.jarArgs())

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:        jar,
		Description: "jar",
		Output:      outputFile,
		Implicits:   deps,
		Args: map[string]string{
			"operation": "cf",
			"jarArgs":   strings.Join(jarArgs, " "),
		},
	})

	return outputFile
}

func TransformJarJar(ctx android.ModuleContext, classesJar android.Path, rulesFile android.Path) android.Path {
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

func TransformPrebuiltJarToClasses(ctx android.ModuleContext,
	subdir string, prebuilt android.Path) (classJarSpec, resourceJarSpec jarSpec) {

	classDir := android.PathForModuleOut(ctx, subdir, "classes")
	classFileList := android.PathForModuleOut(ctx, subdir, "classes.list")
	resourceFileList := android.PathForModuleOut(ctx, subdir, "resources.list")

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:        extractPrebuilt,
		Description: "extract classes",
		Outputs:     android.WritablePaths{classFileList, resourceFileList},
		Input:       prebuilt,
		Args: map[string]string{
			"outDir":       classDir.String(),
			"classFile":    classFileList.String(),
			"resourceFile": resourceFileList.String(),
		},
	})

	return jarSpec{classFileList}, jarSpec{resourceFileList}
}

func TransformFileListToJarSpec(ctx android.ModuleContext, dir, fileListFile android.Path) jarSpec {
	outputFile := android.PathForModuleOut(ctx, fileListFile.Base()+".jarArgs")

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:        fileListToJarArgs,
		Description: "file list to jar args",
		Output:      outputFile,
		Input:       fileListFile,
		Args: map[string]string{
			"outDir": dir.String(),
		},
	})

	return jarSpec{outputFile}
}
