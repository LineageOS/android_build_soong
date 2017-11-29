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
)

var (
	signapk = pctx.AndroidStaticRule("signapk",
		blueprint.RuleParams{
			Command:     `java -jar $signapkCmd $certificates $in $out`,
			CommandDeps: []string{"$signapkCmd"},
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
}

var combineApk = pctx.AndroidStaticRule("combineApk",
	blueprint.RuleParams{
		Command:     `${config.MergeZipsCmd} $out $in`,
		CommandDeps: []string{"${config.MergeZipsCmd}"},
	})

func CreateAppPackage(ctx android.ModuleContext, outputFile android.WritablePath,
	resJarFile, dexJarFile android.Path, certificates []string) {

	// TODO(ccross): JNI libs

	unsignedApk := android.PathForModuleOut(ctx, "unsigned.apk")

	inputs := android.Paths{resJarFile}
	if dexJarFile != nil {
		inputs = append(inputs, dexJarFile)
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:   combineApk,
		Inputs: inputs,
		Output: unsignedApk,
	})

	var certificateArgs []string
	for _, c := range certificates {
		certificateArgs = append(certificateArgs, c+".x509.pem", c+".pk8")
	}

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
