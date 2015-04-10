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
	"path/filepath"
	"strings"

	"android/soong/common"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"
)

var (
	pctx = blueprint.NewPackageContext("android/soong/java")

	// Compiling java is not conducive to proper dependency tracking.  The path-matches-class-name
	// requirement leads to unpredictable generated source file names, and a single .java file
	// will get compiled into multiple .class files if it contains inner classes.  To work around
	// this, all java rules write into separate directories and then a post-processing step lists
	// the files in the the directory into a list file that later rules depend on (and sometimes
	// read from directly using @<listfile>)
	javac = pctx.StaticRule("javac",
		blueprint.RuleParams{
			Command: `rm -rf "$outDir" && mkdir -p "$outDir" && ` +
				`$javacCmd -encoding UTF-8 $javacFlags $bootClasspath $classpath ` +
				`-extdirs "" -d $outDir @$out.rsp || ( rm -rf "$outDir"; exit 41 ) && ` +
				`find $outDir -name "*.class" > $out`,
			Rspfile:        "$out.rsp",
			RspfileContent: "$in",
			Description:    "javac $outDir",
		},
		"javacCmd", "javacFlags", "bootClasspath", "classpath", "outDir")

	jar = pctx.StaticRule("jar",
		blueprint.RuleParams{
			Command:     `$jarCmd -o $out $jarArgs`,
			Description: "jar $out",
		},
		"jarCmd", "jarArgs")

	dx = pctx.StaticRule("dx",
		blueprint.RuleParams{
			Command:     "$dxCmd --dex --output=$out $dxFlags $in",
			Description: "dex $out",
		},
		"outDir", "dxFlags")

	jarjar = pctx.StaticRule("jarjar",
		blueprint.RuleParams{
			Command:     "java -jar $jarjarCmd process $rulesFile $in $out",
			Description: "jarjar $out",
		},
		"rulesFile")

	extractPrebuilt = pctx.StaticRule("extractPrebuilt",
		blueprint.RuleParams{
			Command: `rm -rf $outDir && unzip -qo $in -d $outDir && ` +
				`find $outDir -name "*.class" > $classFile && ` +
				`find $outDir -type f -a \! -name "*.class" -a \! -name "MANIFEST.MF" > $resourceFile || ` +
				`(rm -rf $outDir; exit 42)`,
			Description: "extract java prebuilt $outDir",
		},
		"outDir", "classFile", "resourceFile")
)

func init() {
	pctx.StaticVariable("commonJdkFlags", "-source 1.7 -target 1.7 -Xmaxerrs 9999999")
	pctx.StaticVariable("javacCmd", "javac -J-Xmx1024M $commonJdkFlags")
	pctx.StaticVariable("jarCmd", filepath.Join(bootstrap.BinDir, "soong_jar"))
	pctx.VariableFunc("dxCmd", func(c interface{}) (string, error) {
		return c.(common.Config).HostBinTool("dx")
	})
	pctx.VariableFunc("jarjarCmd", func(c interface{}) (string, error) {
		return c.(common.Config).HostJavaTool("jarjar.jar")
	})
}

type javaBuilderFlags struct {
	javacFlags    string
	dxFlags       string
	bootClasspath string
	classpath     string
	aidlFlags     string
}

type jarSpec struct {
	fileList, dir string
}

func (j jarSpec) soongJarArgs() string {
	return "-C " + j.dir + " -l " + j.fileList
}

func TransformJavaToClasses(ctx common.AndroidModuleContext, srcFiles []string,
	flags javaBuilderFlags, deps []string) jarSpec {

	classDir := filepath.Join(common.ModuleOutDir(ctx), "classes")
	classFileList := filepath.Join(common.ModuleOutDir(ctx), "classes.list")

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      javac,
		Outputs:   []string{classFileList},
		Inputs:    srcFiles,
		Implicits: deps,
		Args: map[string]string{
			"javacFlags":    flags.javacFlags,
			"bootClasspath": flags.bootClasspath,
			"classpath":     flags.classpath,
			"outDir":        classDir,
		},
	})

	return jarSpec{classFileList, classDir}
}

func TransformClassesToJar(ctx common.AndroidModuleContext, classes []jarSpec,
	manifest string) string {

	outputFile := filepath.Join(common.ModuleOutDir(ctx), "classes-full-debug.jar")

	deps := []string{}
	jarArgs := []string{}

	for _, j := range classes {
		deps = append(deps, j.fileList)
		jarArgs = append(jarArgs, j.soongJarArgs())
	}

	if manifest != "" {
		deps = append(deps, manifest)
		jarArgs = append(jarArgs, "-m "+manifest)
	}

	deps = append(deps, "$jarCmd")

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      jar,
		Outputs:   []string{outputFile},
		Implicits: deps,
		Args: map[string]string{
			"jarArgs": strings.Join(jarArgs, " "),
		},
	})

	return outputFile
}

func TransformClassesJarToDex(ctx common.AndroidModuleContext, classesJar string,
	flags javaBuilderFlags) string {

	outputFile := filepath.Join(common.ModuleOutDir(ctx), "classes.dex")

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      dx,
		Outputs:   []string{outputFile},
		Inputs:    []string{classesJar},
		Implicits: []string{"$dxCmd"},
		Args: map[string]string{
			"dxFlags": flags.dxFlags,
		},
	})

	return outputFile
}

func TransformDexToJavaLib(ctx common.AndroidModuleContext, resources []jarSpec,
	dexFile string) string {

	outputFile := filepath.Join(common.ModuleOutDir(ctx), "javalib.jar")
	var deps []string
	var jarArgs []string

	for _, j := range resources {
		deps = append(deps, j.fileList)
		jarArgs = append(jarArgs, j.soongJarArgs())
	}

	dexDir, _ := filepath.Split(dexFile)
	jarArgs = append(jarArgs, "-C "+dexDir+" -f "+dexFile)

	deps = append(deps, "$jarCmd", dexFile)

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      jar,
		Outputs:   []string{outputFile},
		Implicits: deps,
		Args: map[string]string{
			"jarArgs": strings.Join(jarArgs, " "),
		},
	})

	return outputFile
}

func TransformJarJar(ctx common.AndroidModuleContext, classesJar string, rulesFile string) string {
	outputFile := filepath.Join(common.ModuleOutDir(ctx), "classes-jarjar.jar")
	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      jarjar,
		Outputs:   []string{outputFile},
		Inputs:    []string{classesJar},
		Implicits: []string{"$jarjarCmd"},
		Args: map[string]string{
			"rulesFile": rulesFile,
		},
	})

	return outputFile
}

func TransformPrebuiltJarToClasses(ctx common.AndroidModuleContext,
	prebuilt string) (classJarSpec, resourceJarSpec jarSpec) {

	classDir := filepath.Join(common.ModuleOutDir(ctx), "classes")
	classFileList := filepath.Join(classDir, "classes.list")
	resourceFileList := filepath.Join(classDir, "resources.list")

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    extractPrebuilt,
		Outputs: []string{classFileList, resourceFileList},
		Inputs:  []string{prebuilt},
		Args: map[string]string{
			"outDir":       classDir,
			"classFile":    classFileList,
			"resourceFile": resourceFileList,
		},
	})

	return jarSpec{classFileList, classDir}, jarSpec{resourceFileList, classDir}
}
