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

	"android/soong/android"

	"github.com/google/blueprint"
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
				`${config.JavacWrapper}${config.JavacCmd} ${config.CommonJdkFlags} ` +
				`$javacFlags $bootClasspath $classpath ` +
				`-source $javaVersion -target $javaVersion ` +
				`-d $outDir -s $annoDir @$out.rsp || ( rm -rf "$outDir"; exit 41 ) && ` +
				`find $outDir -name "*.class" > $out`,
			Rspfile:        "$out.rsp",
			RspfileContent: "$in",
		},
		"javacFlags", "bootClasspath", "classpath", "outDir", "annoDir", "javaVersion")

	jar = pctx.AndroidStaticRule("jar",
		blueprint.RuleParams{
			Command:     `${config.SoongZipCmd} -o $out -d $jarArgs`,
			CommandDeps: []string{"${config.SoongZipCmd}"},
		},
		"jarCmd", "jarArgs")

	dx = pctx.AndroidStaticRule("dx",
		blueprint.RuleParams{
			Command: `rm -rf "$outDir" && mkdir -p "$outDir" && ` +
				`${config.DxCmd} --dex --output=$outDir $dxFlags $in || ( rm -rf "$outDir"; exit 41 ) && ` +
				`find "$outDir" -name "classes*.dex" | sort > $out`,
			CommandDeps: []string{"${config.DxCmd}"},
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
				`find $outDir -name "*.class" > $classFile && ` +
				`find $outDir -type f -a \! -name "*.class" -a \! -name "MANIFEST.MF" > $resourceFile || ` +
				`(rm -rf $outDir; exit 42)`,
		},
		"outDir", "classFile", "resourceFile")
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
	fileList, dir android.Path
}

func (j jarSpec) soongJarArgs() string {
	return "-C " + j.dir.String() + " -l " + j.fileList.String()
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

	return jarSpec{classFileList, classDir}
}

func TransformClassesToJar(ctx android.ModuleContext, classes []jarSpec,
	manifest android.OptionalPath) android.Path {

	outputFile := android.PathForModuleOut(ctx, "classes-full-debug.jar")

	deps := android.Paths{}
	jarArgs := []string{}

	for _, j := range classes {
		deps = append(deps, j.fileList)
		jarArgs = append(jarArgs, j.soongJarArgs())
	}

	if manifest.Valid() {
		deps = append(deps, manifest.Path())
		jarArgs = append(jarArgs, "-m "+manifest.String())
	}

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

	return jarSpec{outputFile, outDir}
}

func TransformDexToJavaLib(ctx android.ModuleContext, resources []jarSpec,
	dexJarSpec jarSpec) android.Path {

	outputFile := android.PathForModuleOut(ctx, "javalib.jar")
	var deps android.Paths
	var jarArgs []string

	for _, j := range resources {
		deps = append(deps, j.fileList)
		jarArgs = append(jarArgs, j.soongJarArgs())
	}

	deps = append(deps, dexJarSpec.fileList)
	jarArgs = append(jarArgs, dexJarSpec.soongJarArgs())

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

	return jarSpec{classFileList, classDir}, jarSpec{resourceFileList, classDir}
}
