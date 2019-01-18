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
	"android/soong/android"
	"strings"

	"github.com/google/blueprint"
)

var kotlinc = pctx.AndroidGomaStaticRule("kotlinc",
	blueprint.RuleParams{
		Command: `rm -rf "$classesDir" "$srcJarDir" "$kotlinBuildFile" && mkdir -p "$classesDir" "$srcJarDir" && ` +
			`${config.ZipSyncCmd} -d $srcJarDir -l $srcJarDir/list -f "*.java" $srcJars && ` +
			`${config.GenKotlinBuildFileCmd} $classpath $classesDir $out.rsp $srcJarDir/list > $kotlinBuildFile &&` +
			`${config.KotlincCmd} $kotlincFlags ` +
			`-jvm-target $kotlinJvmTarget -Xbuild-file=$kotlinBuildFile && ` +
			`${config.SoongZipCmd} -jar -o $out -C $classesDir -D $classesDir`,
		CommandDeps: []string{
			"${config.KotlincCmd}",
			"${config.KotlinCompilerJar}",
			"${config.GenKotlinBuildFileCmd}",
			"${config.SoongZipCmd}",
			"${config.ZipSyncCmd}",
		},
		Rspfile:        "$out.rsp",
		RspfileContent: `$in`,
	},
	"kotlincFlags", "classpath", "srcJars", "srcJarDir", "classesDir", "kotlinJvmTarget", "kotlinBuildFile")

func kotlinCompile(ctx android.ModuleContext, outputFile android.WritablePath,
	srcFiles, srcJars android.Paths,
	flags javaBuilderFlags) {

	inputs := append(android.Paths(nil), srcFiles...)

	var deps android.Paths
	deps = append(deps, flags.kotlincClasspath...)
	deps = append(deps, srcJars...)

	ctx.Build(pctx, android.BuildParams{
		Rule:        kotlinc,
		Description: "kotlinc",
		Output:      outputFile,
		Inputs:      inputs,
		Implicits:   deps,
		Args: map[string]string{
			"classpath":       flags.kotlincClasspath.FormJavaClassPath("-classpath"),
			"kotlincFlags":    flags.kotlincFlags,
			"srcJars":         strings.Join(srcJars.Strings(), " "),
			"classesDir":      android.PathForModuleOut(ctx, "kotlinc", "classes").String(),
			"srcJarDir":       android.PathForModuleOut(ctx, "kotlinc", "srcJars").String(),
			"kotlinBuildFile": android.PathForModuleOut(ctx, "kotlinc-build.xml").String(),
			// http://b/69160377 kotlinc only supports -jvm-target 1.6 and 1.8
			"kotlinJvmTarget": "1.8",
		},
	})
}
