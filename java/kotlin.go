// Copyright 2019 Google Inc. All rights reserved.
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
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"path/filepath"
	"strings"

	"android/soong/android"

	"github.com/google/blueprint"
)

var kotlinc = pctx.AndroidRemoteStaticRule("kotlinc", android.RemoteRuleSupports{Goma: true},
	blueprint.RuleParams{
		Command: `rm -rf "$classesDir" "$headerClassesDir" "$srcJarDir" "$kotlinBuildFile" "$emptyDir" && ` +
			`mkdir -p "$classesDir" "$headerClassesDir" "$srcJarDir" "$emptyDir" && ` +
			`${config.ZipSyncCmd} -d $srcJarDir -l $srcJarDir/list -f "*.java" $srcJars && ` +
			`${config.GenKotlinBuildFileCmd} --classpath "$classpath" --name "$name"` +
			` --out_dir "$classesDir" --srcs "$out.rsp" --srcs "$srcJarDir/list"` +
			` $commonSrcFilesArg --out "$kotlinBuildFile" && ` +
			`${config.KotlincCmd} ${config.KotlincGlobalFlags} ` +
			` ${config.KotlincSuppressJDK9Warnings} ${config.JavacHeapFlags} ` +
			` $kotlincFlags -jvm-target $kotlinJvmTarget -Xbuild-file=$kotlinBuildFile ` +
			` -kotlin-home $emptyDir ` +
			` -Xplugin=${config.KotlinAbiGenPluginJar} ` +
			` -P plugin:org.jetbrains.kotlin.jvm.abi:outputDir=$headerClassesDir && ` +
			`${config.SoongZipCmd} -jar -o $out -C $classesDir -D $classesDir -write_if_changed && ` +
			`${config.SoongZipCmd} -jar -o $headerJar -C $headerClassesDir -D $headerClassesDir -write_if_changed && ` +
			`rm -rf "$srcJarDir"`,
		CommandDeps: []string{
			"${config.KotlincCmd}",
			"${config.KotlinCompilerJar}",
			"${config.KotlinPreloaderJar}",
			"${config.KotlinReflectJar}",
			"${config.KotlinScriptRuntimeJar}",
			"${config.KotlinStdlibJar}",
			"${config.KotlinTrove4jJar}",
			"${config.KotlinAnnotationJar}",
			"${config.KotlinAbiGenPluginJar}",
			"${config.GenKotlinBuildFileCmd}",
			"${config.SoongZipCmd}",
			"${config.ZipSyncCmd}",
		},
		Rspfile:        "$out.rsp",
		RspfileContent: `$in`,
		Restat:         true,
	},
	"kotlincFlags", "classpath", "srcJars", "commonSrcFilesArg", "srcJarDir", "classesDir",
	"headerClassesDir", "headerJar", "kotlinJvmTarget", "kotlinBuildFile", "emptyDir", "name")

func kotlinCommonSrcsList(ctx android.ModuleContext, commonSrcFiles android.Paths) android.OptionalPath {
	if len(commonSrcFiles) > 0 {
		// The list of common_srcs may be too long to put on the command line, but
		// we can't use the rsp file because it is already being used for srcs.
		// Insert a second rule to write out the list of resources to a file.
		commonSrcsList := android.PathForModuleOut(ctx, "kotlinc_common_srcs.list")
		rule := android.NewRuleBuilder(pctx, ctx)
		rule.Command().Text("cp").
			FlagWithRspFileInputList("", commonSrcsList.ReplaceExtension(ctx, "rsp"), commonSrcFiles).
			Output(commonSrcsList)
		rule.Build("kotlin_common_srcs_list", "kotlin common_srcs list")
		return android.OptionalPathForPath(commonSrcsList)
	}
	return android.OptionalPath{}
}

// kotlinCompile takes .java and .kt sources and srcJars, and compiles the .kt sources into a classes jar in outputFile.
func kotlinCompile(ctx android.ModuleContext, outputFile, headerOutputFile android.WritablePath,
	srcFiles, commonSrcFiles, srcJars android.Paths,
	flags javaBuilderFlags) {

	var deps android.Paths
	deps = append(deps, flags.kotlincClasspath...)
	deps = append(deps, flags.kotlincDeps...)
	deps = append(deps, srcJars...)
	deps = append(deps, commonSrcFiles...)

	kotlinName := filepath.Join(ctx.ModuleDir(), ctx.ModuleSubDir(), ctx.ModuleName())
	kotlinName = strings.ReplaceAll(kotlinName, "/", "__")

	commonSrcsList := kotlinCommonSrcsList(ctx, commonSrcFiles)
	commonSrcFilesArg := ""
	if commonSrcsList.Valid() {
		deps = append(deps, commonSrcsList.Path())
		commonSrcFilesArg = "--common_srcs " + commonSrcsList.String()
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:           kotlinc,
		Description:    "kotlinc",
		Output:         outputFile,
		ImplicitOutput: headerOutputFile,
		Inputs:         srcFiles,
		Implicits:      deps,
		Args: map[string]string{
			"classpath":         flags.kotlincClasspath.FormJavaClassPath(""),
			"kotlincFlags":      flags.kotlincFlags,
			"commonSrcFilesArg": commonSrcFilesArg,
			"srcJars":           strings.Join(srcJars.Strings(), " "),
			"classesDir":        android.PathForModuleOut(ctx, "kotlinc", "classes").String(),
			"headerClassesDir":  android.PathForModuleOut(ctx, "kotlinc", "header_classes").String(),
			"headerJar":         headerOutputFile.String(),
			"srcJarDir":         android.PathForModuleOut(ctx, "kotlinc", "srcJars").String(),
			"kotlinBuildFile":   android.PathForModuleOut(ctx, "kotlinc-build.xml").String(),
			"emptyDir":          android.PathForModuleOut(ctx, "kotlinc", "empty").String(),
			"kotlinJvmTarget":   flags.javaVersion.StringForKotlinc(),
			"name":              kotlinName,
		},
	})
}

var kapt = pctx.AndroidRemoteStaticRule("kapt", android.RemoteRuleSupports{Goma: true},
	blueprint.RuleParams{
		Command: `rm -rf "$srcJarDir" "$kotlinBuildFile" "$kaptDir" && ` +
			`mkdir -p "$srcJarDir" "$kaptDir/sources" "$kaptDir/classes" && ` +
			`${config.ZipSyncCmd} -d $srcJarDir -l $srcJarDir/list -f "*.java" $srcJars && ` +
			`${config.GenKotlinBuildFileCmd} --classpath "$classpath" --name "$name"` +
			` --srcs "$out.rsp" --srcs "$srcJarDir/list"` +
			` $commonSrcFilesArg --out "$kotlinBuildFile" && ` +
			`${config.KotlincCmd} ${config.KotlincGlobalFlags} ` +
			`${config.KaptSuppressJDK9Warnings} ${config.KotlincSuppressJDK9Warnings} ` +
			`${config.JavacHeapFlags} $kotlincFlags -Xplugin=${config.KotlinKaptJar} ` +
			`-P plugin:org.jetbrains.kotlin.kapt3:sources=$kaptDir/sources ` +
			`-P plugin:org.jetbrains.kotlin.kapt3:classes=$kaptDir/classes ` +
			`-P plugin:org.jetbrains.kotlin.kapt3:stubs=$kaptDir/stubs ` +
			`-P plugin:org.jetbrains.kotlin.kapt3:correctErrorTypes=true ` +
			`-P plugin:org.jetbrains.kotlin.kapt3:aptMode=stubsAndApt ` +
			`-P plugin:org.jetbrains.kotlin.kapt3:javacArguments=$encodedJavacFlags ` +
			`$kaptProcessorPath ` +
			`$kaptProcessor ` +
			`-Xbuild-file=$kotlinBuildFile && ` +
			`${config.SoongZipCmd} -jar -o $out -C $kaptDir/sources -D $kaptDir/sources && ` +
			`${config.SoongZipCmd} -jar -o $classesJarOut -C $kaptDir/classes -D $kaptDir/classes && ` +
			`rm -rf "$srcJarDir"`,
		CommandDeps: []string{
			"${config.KotlincCmd}",
			"${config.KotlinCompilerJar}",
			"${config.KotlinKaptJar}",
			"${config.GenKotlinBuildFileCmd}",
			"${config.SoongZipCmd}",
			"${config.ZipSyncCmd}",
		},
		Rspfile:        "$out.rsp",
		RspfileContent: `$in`,
	},
	"kotlincFlags", "encodedJavacFlags", "kaptProcessorPath", "kaptProcessor",
	"classpath", "srcJars", "commonSrcFilesArg", "srcJarDir", "kaptDir", "kotlinJvmTarget",
	"kotlinBuildFile", "name", "classesJarOut")

// kotlinKapt performs Kotlin-compatible annotation processing.  It takes .kt and .java sources and srcjars, and runs
// annotation processors over all of them, producing a srcjar of generated code in outputFile.  The srcjar should be
// added as an additional input to kotlinc and javac rules, and the javac rule should have annotation processing
// disabled.
func kotlinKapt(ctx android.ModuleContext, srcJarOutputFile, resJarOutputFile android.WritablePath,
	srcFiles, commonSrcFiles, srcJars android.Paths,
	flags javaBuilderFlags) {

	srcFiles = append(android.Paths(nil), srcFiles...)

	var deps android.Paths
	deps = append(deps, flags.kotlincClasspath...)
	deps = append(deps, flags.kotlincDeps...)
	deps = append(deps, srcJars...)
	deps = append(deps, flags.processorPath...)
	deps = append(deps, commonSrcFiles...)

	commonSrcsList := kotlinCommonSrcsList(ctx, commonSrcFiles)
	commonSrcFilesArg := ""
	if commonSrcsList.Valid() {
		deps = append(deps, commonSrcsList.Path())
		commonSrcFilesArg = "--common_srcs " + commonSrcsList.String()
	}

	kaptProcessorPath := flags.processorPath.FormRepeatedClassPath("-P plugin:org.jetbrains.kotlin.kapt3:apclasspath=")

	kaptProcessor := ""
	for i, p := range flags.processors {
		if i > 0 {
			kaptProcessor += " "
		}
		kaptProcessor += "-P plugin:org.jetbrains.kotlin.kapt3:processors=" + p
	}

	encodedJavacFlags := kaptEncodeFlags([][2]string{
		{"-source", flags.javaVersion.String()},
		{"-target", flags.javaVersion.String()},
	})

	kotlinName := filepath.Join(ctx.ModuleDir(), ctx.ModuleSubDir(), ctx.ModuleName())
	kotlinName = strings.ReplaceAll(kotlinName, "/", "__")

	ctx.Build(pctx, android.BuildParams{
		Rule:           kapt,
		Description:    "kapt",
		Output:         srcJarOutputFile,
		ImplicitOutput: resJarOutputFile,
		Inputs:         srcFiles,
		Implicits:      deps,
		Args: map[string]string{
			"classpath":         flags.kotlincClasspath.FormJavaClassPath(""),
			"kotlincFlags":      flags.kotlincFlags,
			"commonSrcFilesArg": commonSrcFilesArg,
			"srcJars":           strings.Join(srcJars.Strings(), " "),
			"srcJarDir":         android.PathForModuleOut(ctx, "kapt", "srcJars").String(),
			"kotlinBuildFile":   android.PathForModuleOut(ctx, "kapt", "build.xml").String(),
			"kaptProcessorPath": strings.Join(kaptProcessorPath, " "),
			"kaptProcessor":     kaptProcessor,
			"kaptDir":           android.PathForModuleOut(ctx, "kapt/gen").String(),
			"encodedJavacFlags": encodedJavacFlags,
			"name":              kotlinName,
			"classesJarOut":     resJarOutputFile.String(),
		},
	})
}

// kapt converts a list of key, value pairs into a base64 encoded Java serialization, which is what kapt expects.
func kaptEncodeFlags(options [][2]string) string {
	buf := &bytes.Buffer{}

	binary.Write(buf, binary.BigEndian, uint32(len(options)))
	for _, option := range options {
		binary.Write(buf, binary.BigEndian, uint16(len(option[0])))
		buf.WriteString(option[0])
		binary.Write(buf, binary.BigEndian, uint16(len(option[1])))
		buf.WriteString(option[1])
	}

	header := &bytes.Buffer{}
	header.Write([]byte{0xac, 0xed, 0x00, 0x05}) // java serialization header

	if buf.Len() < 256 {
		header.WriteByte(0x77) // blockdata
		header.WriteByte(byte(buf.Len()))
	} else {
		header.WriteByte(0x7a) // blockdatalong
		binary.Write(header, binary.BigEndian, uint32(buf.Len()))
	}

	return base64.StdEncoding.EncodeToString(append(header.Bytes(), buf.Bytes()...))
}
