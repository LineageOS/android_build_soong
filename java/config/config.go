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

	DefaultLibraries = []string{"core-oj", "core-libart", "ext", "framework", "okhttp"}
)

func init() {
	pctx.Import("github.com/google/blueprint/bootstrap")

	pctx.StaticVariable("CommonJdkFlags", strings.Join([]string{
		`-J-Xmx2048M`,
		`-Xmaxerrs 9999999`,
		`-encoding UTF-8`,
		`-sourcepath ""`,
		`-g`,
	}, " "))

	pctx.StaticVariable("DefaultJavaVersion", "1.8")

	pctx.VariableConfigMethod("hostPrebuiltTag", android.Config.PrebuiltOS)

	pctx.SourcePathVariableWithEnvOverride("JavaHome",
		"prebuilts/jdk/jdk8/${hostPrebuiltTag}", "OVERRIDE_ANDROID_JAVA_HOME")
	pctx.SourcePathVariable("JavaToolchain", "${JavaHome}/bin")
	pctx.SourcePathVariableWithEnvOverride("JavacCmd",
		"${JavaToolchain}/javac", "ALTERNATE_JAVAC")
	pctx.SourcePathVariable("JavaCmd", "${JavaToolchain}/java")
	pctx.SourcePathVariable("JarCmd", "${JavaToolchain}/jar")
	pctx.SourcePathVariable("JavadocCmd", "${JavaToolchain}/javadoc")

	pctx.StaticVariable("SoongZipCmd", filepath.Join("${bootstrap.ToolDir}", "soong_zip"))
	pctx.HostBinToolVariable("DxCmd", "dx")
	pctx.HostJavaToolVariable("JarjarCmd", "jarjar.jar")

	pctx.VariableFunc("JavacWrapper", func(config interface{}) (string, error) {
		if override := config.(android.Config).Getenv("JAVAC_WRAPPER"); override != "" {
			return override + " ", nil
		}
		return "", nil
	})
}
