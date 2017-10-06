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
	"path/filepath"
	"strings"

	_ "github.com/google/blueprint/bootstrap"

	"android/soong/android"
)

var (
	pctx = android.NewPackageContext("android/soong/java/config")

	DefaultBootclasspathLibraries = []string{"core-oj", "core-libart"}
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
	}, " "))

	pctx.StaticVariable("DefaultJavaVersion", "1.8")

	pctx.VariableConfigMethod("hostPrebuiltTag", android.Config.PrebuiltOS)

	pctx.VariableFunc("JavaHome", func(config interface{}) (string, error) {
		if override := config.(android.Config).Getenv("OVERRIDE_ANDROID_JAVA_HOME"); override != "" {
			return override, nil
		}
		if jdk9 := config.(android.Config).Getenv("EXPERIMENTAL_USE_OPENJDK9"); jdk9 != "" {
			return "prebuilts/jdk/jdk9/${hostPrebuiltTag}", nil
		}
		return "prebuilts/jdk/jdk8/${hostPrebuiltTag}", nil
	})

	pctx.SourcePathVariable("JavaToolchain", "${JavaHome}/bin")
	pctx.SourcePathVariableWithEnvOverride("JavacCmd",
		"${JavaToolchain}/javac", "ALTERNATE_JAVAC")
	pctx.SourcePathVariable("JavaCmd", "${JavaToolchain}/java")
	pctx.SourcePathVariable("JarCmd", "${JavaToolchain}/jar")
	pctx.SourcePathVariable("JavadocCmd", "${JavaToolchain}/javadoc")
	pctx.SourcePathVariable("JlinkCmd", "${JavaToolchain}/jlink")
	pctx.SourcePathVariable("JmodCmd", "${JavaToolchain}/jmod")

	pctx.SourcePathVariable("JarArgsCmd", "build/soong/scripts/jar-args.sh")
	pctx.StaticVariable("SoongZipCmd", filepath.Join("${bootstrap.ToolDir}", "soong_zip"))
	pctx.StaticVariable("MergeZipsCmd", filepath.Join("${bootstrap.ToolDir}", "merge_zips"))
	pctx.HostBinToolVariable("DxCmd", "dx")
	pctx.HostJavaToolVariable("JarjarCmd", "jarjar.jar")
	pctx.HostJavaToolVariable("DesugarJar", "desugar.jar")

	pctx.VariableFunc("JavacWrapper", func(config interface{}) (string, error) {
		if override := config.(android.Config).Getenv("JAVAC_WRAPPER"); override != "" {
			return override + " ", nil
		}
		return "", nil
	})
}

func StripJavac9Flags(flags []string) []string {
	var ret []string
	for _, f := range flags {
		switch {
		case strings.HasPrefix(f, "-J--add-modules="):
			// drop
		default:
			ret = append(ret, f)
		}
	}

	return ret
}
