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
	"android/soong/remoteexec"
)

var (
	pctx = android.NewPackageContext("android/soong/java")

	// Compiling java is not conducive to proper dependency tracking.  The path-matches-class-name
	// requirement leads to unpredictable generated source file names, and a single .java file
	// will get compiled into multiple .class files if it contains inner classes.  To work around
	// this, all java rules write into separate directories and then are combined into a .jar file
	// (if the rule produces .class files) or a .srcjar file (if the rule produces .java files).
	// .srcjar files are unzipped into a temporary directory when compiled with javac.
	// TODO(b/143658984): goma can't handle the --system argument to javac.
	javac, javacRE = pctx.MultiCommandRemoteStaticRules("javac",
		blueprint.RuleParams{
			Command: `rm -rf "$outDir" "$annoDir" "$srcJarDir" "$out" && mkdir -p "$outDir" "$annoDir" "$srcJarDir" && ` +
				`${config.ZipSyncCmd} -d $srcJarDir -l $srcJarDir/list -f "*.java" $srcJars && ` +
				`(if [ -s $srcJarDir/list ] || [ -s $out.rsp ] ; then ` +
				`${config.SoongJavacWrapper} $javaTemplate${config.JavacCmd} ` +
				`${config.JavacHeapFlags} ${config.JavacVmFlags} ${config.CommonJdkFlags} ` +
				`$processorpath $processor $javacFlags $bootClasspath $classpath ` +
				`-source $javaVersion -target $javaVersion ` +
				`-d $outDir -s $annoDir @$out.rsp @$srcJarDir/list ; fi ) && ` +
				`$zipTemplate${config.SoongZipCmd} -jar -o $out -C $outDir -D $outDir && ` +
				`rm -rf "$srcJarDir"`,
			CommandDeps: []string{
				"${config.JavacCmd}",
				"${config.SoongZipCmd}",
				"${config.ZipSyncCmd}",
			},
			CommandOrderOnly: []string{"${config.SoongJavacWrapper}"},
			Rspfile:          "$out.rsp",
			RspfileContent:   "$in",
		}, map[string]*remoteexec.REParams{
			"$javaTemplate": &remoteexec.REParams{
				Labels:       map[string]string{"type": "compile", "lang": "java", "compiler": "javac"},
				ExecStrategy: "${config.REJavacExecStrategy}",
				Platform:     map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
			},
			"$zipTemplate": &remoteexec.REParams{
				Labels:       map[string]string{"type": "tool", "name": "soong_zip"},
				Inputs:       []string{"${config.SoongZipCmd}", "$outDir"},
				OutputFiles:  []string{"$out"},
				ExecStrategy: "${config.REJavacExecStrategy}",
				Platform:     map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
			},
		}, []string{"javacFlags", "bootClasspath", "classpath", "processorpath", "processor", "srcJars", "srcJarDir",
			"outDir", "annoDir", "javaVersion"}, nil)

	_ = pctx.VariableFunc("kytheCorpus",
		func(ctx android.PackageVarContext) string { return ctx.Config().XrefCorpusName() })
	_ = pctx.VariableFunc("kytheCuEncoding",
		func(ctx android.PackageVarContext) string { return ctx.Config().XrefCuEncoding() })
	_ = pctx.VariableFunc("kytheCuJavaSourceMax",
		func(ctx android.PackageVarContext) string { return ctx.Config().XrefCuJavaSourceMax() })
	_ = pctx.SourcePathVariable("kytheVnames", "build/soong/vnames.json")
	// Run it with several --add-exports to allow the classes in the
	// com.google.devtools.kythe.extractors.java.standalone package access the packages in the
	// jdk.compiler compiler module. Long live Java modules.
	kytheExtract = pctx.AndroidStaticRule("kythe",
		blueprint.RuleParams{
			Command: `${config.ZipSyncCmd} -d $srcJarDir ` +
				`-l $srcJarDir/list -f "*.java" $srcJars && ` +
				`( [ ! -s $srcJarDir/list -a ! -s $out.rsp ] || ` +
				`KYTHE_ROOT_DIRECTORY=. KYTHE_OUTPUT_FILE=$out ` +
				`KYTHE_CORPUS=${kytheCorpus} ` +
				`KYTHE_VNAMES=${kytheVnames} ` +
				`KYTHE_KZIP_ENCODING=${kytheCuEncoding} ` +
				`KYTHE_JAVA_SOURCE_BATCH_SIZE=${kytheCuJavaSourceMax} ` +
				`${config.SoongJavacWrapper} ${config.JavaCmd} ` +
				// Avoid JDK9's warning about "Illegal reflective access by com.google.protobuf.Utf8$UnsafeProcessor ...
				// to field java.nio.Buffer.address"
				`--add-opens=java.base/java.nio=ALL-UNNAMED ` +
				// Allow the classes in the com.google.devtools.kythe.extractors.java.standalone package
				// access the packages in the jdk.compiler compiler module
				`--add-opens=java.base/java.nio=ALL-UNNAMED ` +
				`--add-exports=jdk.compiler/com.sun.tools.javac.util=ALL-UNNAMED ` +
				`--add-exports=jdk.compiler/com.sun.tools.javac.main=ALL-UNNAMED ` +
				`--add-exports=jdk.compiler/com.sun.tools.javac.file=ALL-UNNAMED ` +
				`--add-exports=jdk.compiler/com.sun.tools.javac.api=ALL-UNNAMED ` +
				`--add-exports=jdk.compiler/com.sun.tools.javac.code=ALL-UNNAMED ` +
				`-jar ${config.JavaKytheExtractorJar} ` +
				`${config.JavacHeapFlags} ${config.CommonJdkFlags} ` +
				`$processorpath $processor $javacFlags $bootClasspath $classpath ` +
				`-source $javaVersion -target $javaVersion ` +
				`-d $outDir -s $annoDir @$out.rsp @$srcJarDir/list)`,
			CommandDeps: []string{
				"${config.JavaCmd}",
				"${config.JavaKytheExtractorJar}",
				"${kytheVnames}",
				"${config.ZipSyncCmd}",
			},
			CommandOrderOnly: []string{"${config.SoongJavacWrapper}"},
			Rspfile:          "$out.rsp",
			RspfileContent:   "$in",
		},
		"javacFlags", "bootClasspath", "classpath", "processorpath", "processor", "srcJars", "srcJarDir",
		"outDir", "annoDir", "javaVersion")

	extractMatchingApks = pctx.StaticRule(
		"extractMatchingApks",
		blueprint.RuleParams{
			Command: `rm -rf "$out" && ` +
				`${config.ExtractApksCmd} -o "${out}" -zip "${zip}" -allow-prereleased=${allow-prereleased} ` +
				`-sdk-version=${sdk-version} -skip-sdk-check=${skip-sdk-check} -abis=${abis} ` +
				`--screen-densities=${screen-densities} --stem=${stem} ` +
				`-apkcerts=${apkcerts} -partition=${partition} ` +
				`${in}`,
			CommandDeps: []string{"${config.ExtractApksCmd}"},
		},
		"abis", "allow-prereleased", "screen-densities", "sdk-version", "skip-sdk-check", "stem", "apkcerts", "partition", "zip")

	turbine, turbineRE = pctx.RemoteStaticRules("turbine",
		blueprint.RuleParams{
			Command: `$reTemplate${config.JavaCmd} ${config.JavaVmFlags} -jar ${config.TurbineJar} $outputFlags ` +
				`--sources @$out.rsp  --source_jars $srcJars ` +
				`--javacopts ${config.CommonJdkFlags} ` +
				`$javacFlags -source $javaVersion -target $javaVersion -- $turbineFlags && ` +
				`(for o in $outputs; do if cmp -s $${o}.tmp $${o} ; then rm $${o}.tmp ; else mv $${o}.tmp $${o} ; fi; done )`,
			CommandDeps: []string{
				"${config.TurbineJar}",
				"${config.JavaCmd}",
			},
			Rspfile:        "$out.rsp",
			RspfileContent: "$in",
			Restat:         true,
		},
		&remoteexec.REParams{Labels: map[string]string{"type": "tool", "name": "turbine"},
			ExecStrategy:    "${config.RETurbineExecStrategy}",
			Inputs:          []string{"${config.TurbineJar}", "${out}.rsp", "$implicits"},
			RSPFiles:        []string{"${out}.rsp"},
			OutputFiles:     []string{"$rbeOutputs"},
			ToolchainInputs: []string{"${config.JavaCmd}"},
			Platform:        map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
		[]string{"javacFlags", "turbineFlags", "outputFlags", "javaVersion", "outputs", "rbeOutputs", "srcJars"}, []string{"implicits"})

	jar, jarRE = pctx.RemoteStaticRules("jar",
		blueprint.RuleParams{
			Command:        `$reTemplate${config.SoongZipCmd} -jar -o $out @$out.rsp`,
			CommandDeps:    []string{"${config.SoongZipCmd}"},
			Rspfile:        "$out.rsp",
			RspfileContent: "$jarArgs",
		},
		&remoteexec.REParams{
			ExecStrategy: "${config.REJarExecStrategy}",
			Inputs:       []string{"${config.SoongZipCmd}", "${out}.rsp"},
			RSPFiles:     []string{"${out}.rsp"},
			OutputFiles:  []string{"$out"},
			Platform:     map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		}, []string{"jarArgs"}, nil)

	zip, zipRE = pctx.RemoteStaticRules("zip",
		blueprint.RuleParams{
			Command:        `${config.SoongZipCmd} -o $out @$out.rsp`,
			CommandDeps:    []string{"${config.SoongZipCmd}"},
			Rspfile:        "$out.rsp",
			RspfileContent: "$jarArgs",
		},
		&remoteexec.REParams{
			ExecStrategy: "${config.REZipExecStrategy}",
			Inputs:       []string{"${config.SoongZipCmd}", "${out}.rsp", "$implicits"},
			RSPFiles:     []string{"${out}.rsp"},
			OutputFiles:  []string{"$out"},
			Platform:     map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		}, []string{"jarArgs"}, []string{"implicits"})

	combineJar = pctx.AndroidStaticRule("combineJar",
		blueprint.RuleParams{
			Command:     `${config.MergeZipsCmd} --ignore-duplicates -j $jarArgs $out $in`,
			CommandDeps: []string{"${config.MergeZipsCmd}"},
		},
		"jarArgs")

	jarjar = pctx.AndroidStaticRule("jarjar",
		blueprint.RuleParams{
			Command: "" +
				// Jarjar doesn't exit with an error when the rules file contains a syntax error,
				// leading to stale or missing files later in the build.  Remove the output file
				// before running jarjar.
				"rm -f ${out} && " +
				"${config.JavaCmd} ${config.JavaVmFlags}" +
				// b/146418363 Enable Android specific jarjar transformer to drop compat annotations
				// for newly repackaged classes. Dropping @UnsupportedAppUsage on repackaged classes
				// avoids adding new hiddenapis after jarjar'ing.
				" -DremoveAndroidCompatAnnotations=true" +
				" -jar ${config.JarjarCmd} process $rulesFile $in $out && " +
				// Turn a missing output file into a ninja error
				`[ -e ${out} ] || (echo "Missing output file"; exit 1)`,
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
			Command:     "${config.JavaCmd}  ${config.JavaVmFlags} -jar ${config.JetifierJar} -l error -o $out -i $in -t epoch",
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

	convertImplementationJarToHeaderJarRule = pctx.AndroidStaticRule("convertImplementationJarToHeaderJar",
		blueprint.RuleParams{
			Command:     `${config.Zip2ZipCmd} -i ${in} -o ${out} -x 'META-INF/services/**/*'`,
			CommandDeps: []string{"${config.Zip2ZipCmd}"},
		})
)

func init() {
	pctx.Import("android/soong/android")
	pctx.Import("android/soong/java/config")
}

type javaBuilderFlags struct {
	javacFlags string

	// bootClasspath is the list of jars that form the boot classpath (generally the java.* and
	// android.* classes) for tools that still use it.  javac targeting 1.9 or higher uses
	// systemModules and java9Classpath instead.
	bootClasspath classpath

	// classpath is the list of jars that form the classpath for javac and kotlinc rules.  It
	// contains header jars for all static and non-static dependencies.
	classpath classpath

	// dexClasspath is the list of jars that form the classpath for d8 and r8 rules.  It contains
	// header jars for all non-static dependencies.  Static dependencies have already been
	// combined into the program jar.
	dexClasspath classpath

	// java9Classpath is the list of jars that will be added to the classpath when targeting
	// 1.9 or higher.  It generally contains the android.* classes, while the java.* classes
	// are provided by systemModules.
	java9Classpath classpath

	processorPath classpath
	processors    []string
	systemModules *systemModules
	aidlFlags     string
	aidlDeps      android.Paths
	javaVersion   javaVersion

	errorProneExtraJavacFlags string
	errorProneProcessorPath   classpath

	kotlincFlags     string
	kotlincClasspath classpath
	kotlincDeps      android.Paths

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

// Emits the rule to generate Xref input file (.kzip file) for the given set of source files and source jars
// to compile with given set of builder flags, etc.
func emitXrefRule(ctx android.ModuleContext, xrefFile android.WritablePath, idx int,
	srcFiles, srcJars android.Paths,
	flags javaBuilderFlags, deps android.Paths) {

	deps = append(deps, srcJars...)
	classpath := flags.classpath

	var bootClasspath string
	if flags.javaVersion.usesJavaModules() {
		var systemModuleDeps android.Paths
		bootClasspath, systemModuleDeps = flags.systemModules.FormJavaSystemModulesPath(ctx.Device())
		deps = append(deps, systemModuleDeps...)
		classpath = append(flags.java9Classpath, classpath...)
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

	deps = append(deps, classpath...)
	deps = append(deps, flags.processorPath...)

	processor := "-proc:none"
	if len(flags.processors) > 0 {
		processor = "-processor " + strings.Join(flags.processors, ",")
	}

	intermediatesDir := "xref"
	if idx >= 0 {
		intermediatesDir += strconv.Itoa(idx)
	}

	ctx.Build(pctx,
		android.BuildParams{
			Rule:        kytheExtract,
			Description: "Xref Java extractor",
			Output:      xrefFile,
			Inputs:      srcFiles,
			Implicits:   deps,
			Args: map[string]string{
				"annoDir":       android.PathForModuleOut(ctx, intermediatesDir, "anno").String(),
				"bootClasspath": bootClasspath,
				"classpath":     classpath.FormJavaClassPath("-classpath"),
				"javacFlags":    flags.javacFlags,
				"javaVersion":   flags.javaVersion.String(),
				"outDir":        android.PathForModuleOut(ctx, "javac", "classes.xref").String(),
				"processorpath": flags.processorPath.FormJavaClassPath("-processorpath"),
				"processor":     processor,
				"srcJarDir":     android.PathForModuleOut(ctx, intermediatesDir, "srcjars.xref").String(),
				"srcJars":       strings.Join(srcJars.Strings(), " "),
			},
		})
}

func turbineFlags(ctx android.ModuleContext, flags javaBuilderFlags) (string, android.Paths) {
	var deps android.Paths

	classpath := flags.classpath

	var bootClasspath string
	if flags.javaVersion.usesJavaModules() {
		var systemModuleDeps android.Paths
		bootClasspath, systemModuleDeps = flags.systemModules.FormTurbineSystemModulesPath(ctx.Device())
		deps = append(deps, systemModuleDeps...)
		classpath = append(flags.java9Classpath, classpath...)
	} else {
		deps = append(deps, flags.bootClasspath...)
		if len(flags.bootClasspath) == 0 && ctx.Device() {
			// explicitly specify -bootclasspath "" if the bootclasspath is empty to
			// ensure turbine does not fall back to the default bootclasspath.
			bootClasspath = `--bootclasspath ""`
		} else {
			bootClasspath = flags.bootClasspath.FormTurbineClassPath("--bootclasspath ")
		}
	}

	deps = append(deps, classpath...)
	turbineFlags := bootClasspath + " " + classpath.FormTurbineClassPath("--classpath ")

	return turbineFlags, deps
}

func TransformJavaToHeaderClasses(ctx android.ModuleContext, outputFile android.WritablePath,
	srcFiles, srcJars android.Paths, flags javaBuilderFlags) {

	turbineFlags, deps := turbineFlags(ctx, flags)

	deps = append(deps, srcJars...)

	rule := turbine
	args := map[string]string{
		"javacFlags":   flags.javacFlags,
		"srcJars":      strings.Join(srcJars.Strings(), " "),
		"javaVersion":  flags.javaVersion.String(),
		"turbineFlags": turbineFlags,
		"outputFlags":  "--output " + outputFile.String() + ".tmp",
		"outputs":      outputFile.String(),
	}
	if ctx.Config().UseRBE() && ctx.Config().IsEnvTrue("RBE_TURBINE") {
		rule = turbineRE
		args["implicits"] = strings.Join(deps.Strings(), ",")
		args["rbeOutputs"] = outputFile.String() + ".tmp"
	}
	ctx.Build(pctx, android.BuildParams{
		Rule:        rule,
		Description: "turbine",
		Output:      outputFile,
		Inputs:      srcFiles,
		Implicits:   deps,
		Args:        args,
	})
}

// TurbineApt produces a rule to run annotation processors using turbine.
func TurbineApt(ctx android.ModuleContext, outputSrcJar, outputResJar android.WritablePath,
	srcFiles, srcJars android.Paths, flags javaBuilderFlags) {

	turbineFlags, deps := turbineFlags(ctx, flags)

	deps = append(deps, srcJars...)

	deps = append(deps, flags.processorPath...)
	turbineFlags += " " + flags.processorPath.FormTurbineClassPath("--processorpath ")
	turbineFlags += " --processors " + strings.Join(flags.processors, " ")

	outputs := android.WritablePaths{outputSrcJar, outputResJar}
	outputFlags := "--gensrc_output " + outputSrcJar.String() + ".tmp " +
		"--resource_output " + outputResJar.String() + ".tmp"

	rule := turbine
	args := map[string]string{
		"javacFlags":   flags.javacFlags,
		"srcJars":      strings.Join(srcJars.Strings(), " "),
		"javaVersion":  flags.javaVersion.String(),
		"turbineFlags": turbineFlags,
		"outputFlags":  outputFlags,
		"outputs":      strings.Join(outputs.Strings(), " "),
	}
	if ctx.Config().UseRBE() && ctx.Config().IsEnvTrue("RBE_TURBINE") {
		rule = turbineRE
		args["implicits"] = strings.Join(deps.Strings(), ",")
		args["rbeOutputs"] = outputSrcJar.String() + ".tmp," + outputResJar.String() + ".tmp"
	}
	ctx.Build(pctx, android.BuildParams{
		Rule:            rule,
		Description:     "turbine apt",
		Output:          outputs[0],
		ImplicitOutputs: outputs[1:],
		Inputs:          srcFiles,
		Implicits:       deps,
		Args:            args,
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

	classpath := flags.classpath

	var bootClasspath string
	if flags.javaVersion.usesJavaModules() {
		var systemModuleDeps android.Paths
		bootClasspath, systemModuleDeps = flags.systemModules.FormJavaSystemModulesPath(ctx.Device())
		deps = append(deps, systemModuleDeps...)
		classpath = append(flags.java9Classpath, classpath...)
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

	deps = append(deps, classpath...)
	deps = append(deps, flags.processorPath...)

	processor := "-proc:none"
	if len(flags.processors) > 0 {
		processor = "-processor " + strings.Join(flags.processors, ",")
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
	rule := javac
	if ctx.Config().UseRBE() && ctx.Config().IsEnvTrue("RBE_JAVAC") {
		rule = javacRE
	}
	ctx.Build(pctx, android.BuildParams{
		Rule:        rule,
		Description: desc,
		Output:      outputFile,
		Inputs:      srcFiles,
		Implicits:   deps,
		Args: map[string]string{
			"javacFlags":    flags.javacFlags,
			"bootClasspath": bootClasspath,
			"classpath":     classpath.FormJavaClassPath("-classpath"),
			"processorpath": flags.processorPath.FormJavaClassPath("-processorpath"),
			"processor":     processor,
			"srcJars":       strings.Join(srcJars.Strings(), " "),
			"srcJarDir":     android.PathForModuleOut(ctx, intermediatesDir, srcJarDir).String(),
			"outDir":        android.PathForModuleOut(ctx, intermediatesDir, outDir).String(),
			"annoDir":       android.PathForModuleOut(ctx, intermediatesDir, annoDir).String(),
			"javaVersion":   flags.javaVersion.String(),
		},
	})
}

func TransformResourcesToJar(ctx android.ModuleContext, outputFile android.WritablePath,
	jarArgs []string, deps android.Paths) {

	rule := jar
	if ctx.Config().UseRBE() && ctx.Config().IsEnvTrue("RBE_JAR") {
		rule = jarRE
	}
	ctx.Build(pctx, android.BuildParams{
		Rule:        rule,
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

func convertImplementationJarToHeaderJar(ctx android.ModuleContext, implementationJarFile android.Path,
	headerJarFile android.WritablePath) {
	ctx.Build(pctx, android.BuildParams{
		Rule:   convertImplementationJarToHeaderJarRule,
		Input:  implementationJarFile,
		Output: headerJarFile,
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
	android.WriteFileRule(ctx, outputFile, "Main-Class: "+mainClass+"\n")
}

func TransformZipAlign(ctx android.ModuleContext, outputFile android.WritablePath, inputFile android.Path) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        zipalign,
		Description: "align",
		Input:       inputFile,
		Output:      outputFile,
	})
}

type classpath android.Paths

func (x *classpath) formJoinedClassPath(optName string, sep string) string {
	if optName != "" && !strings.HasSuffix(optName, "=") && !strings.HasSuffix(optName, " ") {
		optName += " "
	}
	if len(*x) > 0 {
		return optName + strings.Join(x.Strings(), sep)
	} else {
		return ""
	}
}
func (x *classpath) FormJavaClassPath(optName string) string {
	return x.formJoinedClassPath(optName, ":")
}

func (x *classpath) FormTurbineClassPath(optName string) string {
	return x.formJoinedClassPath(optName, " ")
}

// FormRepeatedClassPath returns a list of arguments with the given optName prefixed to each element of the classpath.
func (x *classpath) FormRepeatedClassPath(optName string) []string {
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

type systemModules struct {
	dir  android.Path
	deps android.Paths
}

// Returns a --system argument in the form javac expects with -source 1.9 and the list of files to
// depend on.  If forceEmpty is true, returns --system=none if the list is empty to ensure javac
// does not fall back to the default system modules.
func (x *systemModules) FormJavaSystemModulesPath(forceEmpty bool) (string, android.Paths) {
	if x != nil {
		return "--system=" + x.dir.String(), x.deps
	} else if forceEmpty {
		return "--system=none", nil
	} else {
		return "", nil
	}
}

// Returns a --system argument in the form turbine expects with -source 1.9 and the list of files to
// depend on.  If forceEmpty is true, returns --bootclasspath "" if the list is empty to ensure turbine
// does not fall back to the default bootclasspath.
func (x *systemModules) FormTurbineSystemModulesPath(forceEmpty bool) (string, android.Paths) {
	if x != nil {
		return "--system " + x.dir.String(), x.deps
	} else if forceEmpty {
		return `--bootclasspath ""`, nil
	} else {
		return "--system ${config.JavaHome}", nil
	}
}
