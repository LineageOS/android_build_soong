// Copyright 2017 Google Inc. All rights reserved.
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

package config

import (
	"strings"

	_ "github.com/google/blueprint/bootstrap"

	"android/soong/android"
)

var (
	pctx = android.NewPackageContext("android/soong/java/config")

	DefaultBootclasspathLibraries = []string{"core-oj", "core-libart"}
	DefaultSystemModules          = "core-system-modules"
	DefaultLibraries              = []string{"ext", "framework", "okhttp"}
)

func init() {
	pctx.Import("github.com/google/blueprint/bootstrap")

	pctx.StaticVariable("JavacHeapSize", "2048M")
	pctx.StaticVariable("JavacHeapFlags", "-J-Xmx${JavacHeapSize}")

	pctx.StaticVariable("CommonJdkFlags", strings.Join([]string{
		`-Xmaxerrs 9999999`,
		`-encoding UTF-8`,
		`-sourcepath ""`,
		`-g`,
		// Turbine leaves out bridges which can cause javac to unnecessarily insert them into
		// subclasses (b/65645120).  Setting this flag causes our custom javac to assume that
		// the missing bridges will exist at runtime and not recreate them in subclasses.
		// If a different javac is used the flag will be ignored and extra bridges will be inserted.
		// The flag is implemented by https://android-review.googlesource.com/c/486427
		`-XDskipDuplicateBridges=true`,

		// b/65004097: prevent using java.lang.invoke.StringConcatFactory when using -target 1.9
		`-XDstringConcat=inline`,
	}, " "))

	pctx.VariableConfigMethod("hostPrebuiltTag", android.Config.PrebuiltOS)

	pctx.VariableFunc("JavaHome", func(config interface{}) (string, error) {
		// This is set up and guaranteed by soong_ui
		return config.(android.Config).Getenv("ANDROID_JAVA_HOME"), nil
	})

	pctx.SourcePathVariable("JavaToolchain", "${JavaHome}/bin")
	pctx.SourcePathVariableWithEnvOverride("JavacCmd",
		"${JavaToolchain}/javac", "ALTERNATE_JAVAC")
	pctx.SourcePathVariable("JavaCmd", "${JavaToolchain}/java")
	pctx.SourcePathVariable("JarCmd", "${JavaToolchain}/jar")
	pctx.SourcePathVariable("JavadocCmd", "${JavaToolchain}/javadoc")
	pctx.SourcePathVariable("JlinkCmd", "${JavaToolchain}/jlink")
	pctx.SourcePathVariable("JmodCmd", "${JavaToolchain}/jmod")
	pctx.SourcePathVariable("JrtFsJar", "${JavaHome}/lib/jrt-fs.jar")
	pctx.SourcePathVariable("Ziptime", "prebuilts/build-tools/${hostPrebuiltTag}/bin/ziptime")

	pctx.SourcePathVariable("ExtractSrcJarsCmd", "build/soong/scripts/extract-srcjars.sh")
	pctx.SourcePathVariable("JarArgsCmd", "build/soong/scripts/jar-args.sh")
	pctx.HostBinToolVariable("SoongZipCmd", "soong_zip")
	pctx.HostBinToolVariable("MergeZipsCmd", "merge_zips")
	pctx.VariableFunc("DxCmd", func(config interface{}) (string, error) {
		if config.(android.Config).IsEnvFalse("USE_D8") {
			if config.(android.Config).UnbundledBuild() || config.(android.Config).IsPdkBuild() {
				return "prebuilts/build-tools/common/bin/dx", nil
			} else {
				path, err := pctx.HostBinToolPath(config, "dx")
				if err != nil {
					return "", err
				}
				return path.String(), nil
			}
		} else {
			path, err := pctx.HostBinToolPath(config, "d8-compat-dx")
			if err != nil {
				return "", err
			}
			return path.String(), nil
		}
	})
	pctx.VariableFunc("TurbineJar", func(config interface{}) (string, error) {
		turbine := "turbine.jar"
		if config.(android.Config).UnbundledBuild() {
			return "prebuilts/build-tools/common/framework/" + turbine, nil
		} else {
			path, err := pctx.HostJavaToolPath(config, turbine)
			if err != nil {
				return "", err
			}
			return path.String(), nil
		}
	})

	pctx.HostJavaToolVariable("JarjarCmd", "jarjar.jar")
	pctx.HostJavaToolVariable("DesugarJar", "desugar.jar")

	pctx.HostBinToolVariable("SoongJavacWrapper", "soong_javac_wrapper")

	pctx.VariableFunc("JavacWrapper", func(config interface{}) (string, error) {
		if override := config.(android.Config).Getenv("JAVAC_WRAPPER"); override != "" {
			return override + " ", nil
		}
		return "", nil
	})
}
