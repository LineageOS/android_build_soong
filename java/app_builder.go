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

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

var (
	signapk = pctx.AndroidStaticRule("signapk",
		blueprint.RuleParams{
			Command: `${config.JavaCmd} -Djava.library.path=$$(dirname $signapkJniLibrary) ` +
				`-jar $signapkCmd $certificates $in $out`,
			CommandDeps: []string{"$signapkCmd", "$signapkJniLibrary"},
		},
		"certificates")

	androidManifestMerger = pctx.AndroidStaticRule("androidManifestMerger",
		blueprint.RuleParams{
			Command: "java -classpath $androidManifestMergerCmd com.android.manifmerger.Main merge " +
				"--main $in --libs $libsManifests --out $out",
			CommandDeps: []string{"$androidManifestMergerCmd"},
			Description: "merge manifest files",
		},
		"libsManifests")
)

func init() {
	pctx.SourcePathVariable("androidManifestMergerCmd", "prebuilts/devtools/tools/lib/manifest-merger.jar")
	pctx.HostBinToolVariable("aaptCmd", "aapt")
	pctx.HostJavaToolVariable("signapkCmd", "signapk.jar")
	// TODO(ccross): this should come from the signapk dependencies, but we don't have any way
	// to express host JNI dependencies yet.
	pctx.HostJNIToolVariable("signapkJniLibrary", "libconscrypt_openjdk_jni")
}

var combineApk = pctx.AndroidStaticRule("combineApk",
	blueprint.RuleParams{
		Command:     `${config.MergeZipsCmd} $out $in`,
		CommandDeps: []string{"${config.MergeZipsCmd}"},
	})

func CreateAppPackage(ctx android.ModuleContext, outputFile android.WritablePath,
	resJarFile, jniJarFile, dexJarFile android.Path, certificates []certificate) {

	unsignedApk := android.PathForModuleOut(ctx, "unsigned.apk")

	var inputs android.Paths
	if dexJarFile != nil {
		inputs = append(inputs, dexJarFile)
	}
	inputs = append(inputs, resJarFile)
	if jniJarFile != nil {
		inputs = append(inputs, jniJarFile)
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:   combineApk,
		Inputs: inputs,
		Output: unsignedApk,
	})

	var certificateArgs []string
	for _, c := range certificates {
		certificateArgs = append(certificateArgs, c.pem.String(), c.key.String())
	}

	// TODO(ccross): sometimes uncompress dex
	// TODO(ccross): sometimes strip dex

	ctx.Build(pctx, android.BuildParams{
		Rule:        signapk,
		Description: "signapk",
		Output:      outputFile,
		Input:       unsignedApk,
		Args: map[string]string{
			"certificates": strings.Join(certificateArgs, " "),
		},
	})
}

var buildAAR = pctx.AndroidStaticRule("buildAAR",
	blueprint.RuleParams{
		Command: `rm -rf ${outDir} && mkdir -p ${outDir} && ` +
			`cp ${manifest} ${outDir}/AndroidManifest.xml && ` +
			`cp ${classesJar} ${outDir}/classes.jar && ` +
			`cp ${rTxt} ${outDir}/R.txt && ` +
			`${config.SoongZipCmd} -jar -o $out -C ${outDir} -D ${outDir}`,
		CommandDeps: []string{"${config.SoongZipCmd}"},
	},
	"manifest", "classesJar", "rTxt", "outDir")

func BuildAAR(ctx android.ModuleContext, outputFile android.WritablePath,
	classesJar, manifest, rTxt android.Path, res android.Paths) {

	// TODO(ccross): uniquify and copy resources with dependencies

	deps := android.Paths{manifest, rTxt}
	classesJarPath := ""
	if classesJar != nil {
		deps = append(deps, classesJar)
		classesJarPath = classesJar.String()
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:      buildAAR,
		Implicits: deps,
		Output:    outputFile,
		Args: map[string]string{
			"manifest":   manifest.String(),
			"classesJar": classesJarPath,
			"rTxt":       rTxt.String(),
			"outDir":     android.PathForModuleOut(ctx, "aar").String(),
		},
	})
}

func TransformJniLibsToJar(ctx android.ModuleContext, outputFile android.WritablePath,
	jniLibs []jniLib) {

	var deps android.Paths
	jarArgs := []string{
		"-j", // junk paths, they will be added back with -P arguments
	}

	if !ctx.Config().UnbundledBuild() {
		jarArgs = append(jarArgs, "-L 0")
	}

	for _, j := range jniLibs {
		deps = append(deps, j.path)
		jarArgs = append(jarArgs,
			"-P "+targetToJniDir(j.target),
			"-f "+j.path.String())
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        zip,
		Description: "zip jni libs",
		Output:      outputFile,
		Implicits:   deps,
		Args: map[string]string{
			"jarArgs": strings.Join(proptools.NinjaAndShellEscape(jarArgs), " "),
		},
	})
}

func targetToJniDir(target android.Target) string {
	return filepath.Join("lib", target.Arch.Abi[0])
}
