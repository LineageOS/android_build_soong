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
	"strconv"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

var (
	pctx = android.NewPackageContext("android/soong/java")

	// Compiling java is not conducive to proper dependency tracking.  The path-matches-class-name
	// requirement leads to unpredictable generated source file names, and a single .java file
	// will get compiled into multiple .class files if it contains inner classes.  To work around
	// this, all java rules write into separate directories and then are combined into a .jar file
	// (if the rule produces .class files) or a .srcjar file (if the rule produces .java files).
	// .srcjar files are unzipped into a temporary directory when compiled with javac.
	javac = pctx.AndroidGomaStaticRule("javac",
		blueprint.RuleParams{
			Command: `rm -rf "$outDir" "$annoDir" "$srcJarDir" && mkdir -p "$outDir" "$annoDir" "$srcJarDir" && ` +
				`${config.ZipSyncCmd} -d $srcJarDir -l $srcJarDir/list -f "*.java" $srcJars && ` +
				`(if [ -s $srcJarDir/list ] || [ -s $out.rsp ] ; then ` +
				`${config.SoongJavacWrapper} ${config.JavacWrapper}${config.JavacCmd} ${config.JavacHeapFlags} ${config.CommonJdkFlags} ` +
				`$processorpath $processor $javacFlags $bootClasspath $classpath ` +
				`-source $javaVersion -target $javaVersion ` +
				`-d $outDir -s $annoDir @$out.rsp @$srcJarDir/list ; fi ) && ` +
				`${config.SoongZipCmd} -jar -o $out -C $outDir -D $outDir && ` +
				`rm -rf "$srcJarDir"`,
			CommandDeps: []string{
				"${config.JavacCmd}",
				"${config.SoongZipCmd}",
				"${config.ZipSyncCmd}",
			},
			CommandOrderOnly: []string{"${config.SoongJavacWrapper}"},
			Rspfile:          "$out.rsp",
			RspfileContent:   "$in",
		},
		"javacFlags", "bootClasspath", "classpath", "processorpath", "processor", "srcJars", "srcJarDir",
		"outDir", "annoDir", "javaVersion")

	turbine = pctx.AndroidStaticRule("turbine",
		blueprint.RuleParams{
			Command: `rm -rf "$outDir" && mkdir -p "$outDir" && ` +
				`${config.JavaCmd} -jar ${config.TurbineJar} --output $out.tmp ` +
				`--temp_dir "$outDir" --sources @$out.rsp  --source_jars $srcJars ` +
				`--javacopts ${config.CommonJdkFlags} ` +
				`$javacFlags -source $javaVersion -target $javaVersion -- $bootClasspath $classpath && ` +
				`${config.Ziptime} $out.tmp && ` +
				`(if cmp -s $out.tmp $out ; then rm $out.tmp ; else mv $out.tmp $out ; fi )`,
			CommandDeps: []string{
				"${config.TurbineJar}",
				"${config.JavaCmd}",
				"${config.Ziptime}",
			},
			Rspfile:        "$out.rsp",
			RspfileContent: "$in",
			Restat:         true,
		},
		"javacFlags", "bootClasspath", "classpath", "srcJars", "outDir", "javaVersion")

	jar = pctx.AndroidStaticRule("jar",
		blueprint.RuleParams{
			Command:        `${config.SoongZipCmd} -jar -o $out @$out.rsp`,
			CommandDeps:    []string{"${config.SoongZipCmd}"},
			Rspfile:        "$out.rsp",
			RspfileContent: "$jarArgs",
		},
		"jarArgs")

	zip = pctx.AndroidStaticRule("zip",
		blueprint.RuleParams{
			Command:        `${config.SoongZipCmd} -o $out @$out.rsp`,
			CommandDeps:    []string{"${config.SoongZipCmd}"},
			Rspfile:        "$out.rsp",
			RspfileContent: "$jarArgs",
		},
		"jarArgs")

	combineJar = pctx.AndroidStaticRule("combineJar",
		blueprint.RuleParams{
			Command:     `${config.MergeZipsCmd} --ignore-duplicates -j $jarArgs $out $in`,
			CommandDeps: []string{"${config.MergeZipsCmd}"},
		},
		"jarArgs")

	jarjar = pctx.AndroidStaticRule("jarjar",
		blueprint.RuleParams{
			Command:     "${config.JavaCmd} -jar ${config.JarjarCmd} process $rulesFile $in $out",
			CommandDeps: []string{"${config.JavaCmd}", "${config.JarjarCmd}", "$rulesFile"},
		},
		"rulesFile")

	packageCheck = pctx.AndroidStaticRule("packageCheck",
		blueprint.RuleParams{
			Command: "rm -f $out && " +
				"${config.PackageCheckCmd} $in $packages && " +
				"touch $out",
			CommandDeps: []string{"${config.PackageCheckCmd}"},
		},
		"packages")

	jetifier = pctx.AndroidStaticRule("jetifier",
		blueprint.RuleParams{
			Command:     "${config.JavaCmd} -jar ${config.JetifierJar} -l error -o $out -i $in",
			CommandDeps: []string{"${config.JavaCmd}", "${config.JetifierJar}"},
		},
	)

	zipalign = pctx.AndroidStaticRule("zipalign",
		blueprint.RuleParams{
			Command: "if ! ${config.ZipAlign} -c -p 4 $in > /dev/null; then " +
				"${config.ZipAlign} -f -p 4 $in $out; " +
				"else " +
				"cp -f $in $out; " +
				"fi",
			CommandDeps: []string{"${config.ZipAlign}"},
		},
	)
)

func init() {
	pctx.Import("android/soong/android")
	pctx.Import("android/soong/java/config")
}

type javaBuilderFlags struct {
	javacFlags    string
	bootClasspath classpath
	classpath     classpath
	processorPath classpath
	processor     string
	systemModules classpath
	aidlFlags     string
	aidlDeps      android.Paths
	javaVersion   string

	errorProneExtraJavacFlags string
	errorProneProcessorPath   classpath

	kotlincFlags     string
	kotlincClasspath classpath

	proto android.ProtoFlags
}

func TransformJavaToClasses(ctx android.ModuleContext, outputFile android.WritablePath, shardIdx int,
	srcFiles, srcJars android.Paths, flags javaBuilderFlags, deps android.Paths) {

	// Compile java sources into .class files
	desc := "javac"
	if shardIdx >= 0 {
		desc += strconv.Itoa(shardIdx)
	}

	transformJavaToClasses(ctx, outputFile, shardIdx, srcFiles, srcJars, flags, deps, "javac", desc)
}

func RunErrorProne(ctx android.ModuleContext, outputFile android.WritablePath,
	srcFiles, srcJars android.Paths, flags javaBuilderFlags) {

	flags.processorPath = append(flags.errorProneProcessorPath, flags.processorPath...)

	if len(flags.errorProneExtraJavacFlags) > 0 {
		if len(flags.javacFlags) > 0 {
			flags.javacFlags += " " + flags.errorProneExtraJavacFlags
		} else {
			flags.javacFlags = flags.errorProneExtraJavacFlags
		}
	}

	transformJavaToClasses(ctx, outputFile, -1, srcFiles, srcJars, flags, nil,
		"errorprone", "errorprone")
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
		bootClasspath = strings.Join(flags.bootClasspath.FormTurbineClasspath("--bootclasspath "), " ")
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
			"classpath":     strings.Join(flags.classpath.FormTurbineClasspath("--classpath "), " "),
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
	shardIdx int, srcFiles, srcJars android.Paths,
	flags javaBuilderFlags, deps android.Paths,
	intermediatesDir, desc string) {

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
	deps = append(deps, flags.processorPath...)

	processor := "-proc:none"
	if flags.processor != "" {
		processor = "-processor " + flags.processor
	}

	srcJarDir := "srcjars"
	outDir := "classes"
	annoDir := "anno"
	if shardIdx >= 0 {
		shardDir := "shard" + strconv.Itoa(shardIdx)
		srcJarDir = filepath.Join(shardDir, srcJarDir)
		outDir = filepath.Join(shardDir, outDir)
		annoDir = filepath.Join(shardDir, annoDir)
	}
	ctx.Build(pctx, android.BuildParams{
		Rule:        javac,
		Description: desc,
		Output:      outputFile,
		Inputs:      srcFiles,
		Implicits:   deps,
		Args: map[string]string{
			"javacFlags":    flags.javacFlags,
			"bootClasspath": bootClasspath,
			"classpath":     flags.classpath.FormJavaClassPath("-classpath"),
			"processorpath": flags.processorPath.FormJavaClassPath("-processorpath"),
			"processor":     processor,
			"srcJars":       strings.Join(srcJars.Strings(), " "),
			"srcJarDir":     android.PathForModuleOut(ctx, intermediatesDir, srcJarDir).String(),
			"outDir":        android.PathForModuleOut(ctx, intermediatesDir, outDir).String(),
			"annoDir":       android.PathForModuleOut(ctx, intermediatesDir, annoDir).String(),
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
			"jarArgs": strings.Join(proptools.NinjaAndShellEscapeList(jarArgs), " "),
		},
	})
}

func TransformJarsToJar(ctx android.ModuleContext, outputFile android.WritablePath, desc string,
	jars android.Paths, manifest android.OptionalPath, stripDirEntries bool, filesToStrip []string,
	dirsToStrip []string) {

	var deps android.Paths

	var jarArgs []string
	if manifest.Valid() {
		jarArgs = append(jarArgs, "-m ", manifest.String())
		deps = append(deps, manifest.Path())
	}

	for _, dir := range dirsToStrip {
		jarArgs = append(jarArgs, "-stripDir ", dir)
	}

	for _, file := range filesToStrip {
		jarArgs = append(jarArgs, "-stripFile ", file)
	}

	// Remove any module-info.class files that may have come from prebuilt jars, they cause problems
	// for downstream tools like desugar.
	jarArgs = append(jarArgs, "-stripFile module-info.class")

	if stripDirEntries {
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

func CheckJarPackages(ctx android.ModuleContext, outputFile android.WritablePath,
	classesJar android.Path, permittedPackages []string) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        packageCheck,
		Description: "packageCheck",
		Output:      outputFile,
		Input:       classesJar,
		Args: map[string]string{
			"packages": strings.Join(permittedPackages, " "),
		},
	})
}

func TransformJetifier(ctx android.ModuleContext, outputFile android.WritablePath,
	inputFile android.Path) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        jetifier,
		Description: "jetifier",
		Output:      outputFile,
		Input:       inputFile,
	})
}

func GenerateMainClassManifest(ctx android.ModuleContext, outputFile android.WritablePath, mainClass string) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        android.WriteFile,
		Description: "manifest",
		Output:      outputFile,
		Args: map[string]string{
			"content": "Main-Class: " + mainClass + "\n",
		},
	})
}

func TransformZipAlign(ctx android.ModuleContext, outputFile android.WritablePath, inputFile android.Path) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        zipalign,
		Description: "align",
		Input:       inputFile,
		Output:      outputFile,
	})
}

type classpath []android.Path

func (x *classpath) FormJavaClassPath(optName string) string {
	if optName != "" && !strings.HasSuffix(optName, "=") && !strings.HasSuffix(optName, " ") {
		optName += " "
	}
	if len(*x) > 0 {
		return optName + strings.Join(x.Strings(), ":")
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

func (x *classpath) FormTurbineClasspath(optName string) []string {
	if x == nil || *x == nil {
		return nil
	}
	flags := make([]string, len(*x))
	for i, v := range *x {
		flags[i] = optName + v.String()
	}

	return flags
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
