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
	"runtime"
	"strings"

	_ "github.com/google/blueprint/bootstrap"

	"android/soong/android"
)

var (
	pctx = android.NewPackageContext("android/soong/java/config")

	DefaultBootclasspathLibraries = []string{"core.platform.api.stubs", "core-lambda-stubs"}
	DefaultSystemModules          = "core-platform-api-stubs-system-modules"
	DefaultLibraries              = []string{"ext", "framework", "updatable_media_stubs"}
	DefaultLambdaStubsLibrary     = "core-lambda-stubs"
	SdkLambdaStubsPath            = "prebuilts/sdk/tools/core-lambda-stubs.jar"

	DefaultJacocoExcludeFilter = []string{"org.junit.*", "org.jacoco.*", "org.mockito.*"}

	InstrumentFrameworkModules = []string{
		"framework",
		"telephony-common",
		"services",
		"android.car",
		"android.car7",
		"conscrypt",
		"core-oj",
		"core-libart",
		"updatable-media",
	}
)

func init() {
	pctx.Import("github.com/google/blueprint/bootstrap")

	pctx.StaticVariable("JavacHeapSize", "2048M")
	pctx.StaticVariable("JavacHeapFlags", "-J-Xmx${JavacHeapSize}")
	pctx.StaticVariable("DexFlags", "-JXX:OnError='cat hs_err_pid%p.log' -JXX:CICompilerCount=6 -JXX:+UseDynamicNumberOfGCThreads")

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

	pctx.VariableFunc("JavaHome", func(ctx android.PackageVarContext) string {
		// This is set up and guaranteed by soong_ui
		return ctx.Config().Getenv("ANDROID_JAVA_HOME")
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

	pctx.SourcePathVariable("GenKotlinBuildFileCmd", "build/soong/scripts/gen-kotlin-build-file.sh")

	pctx.SourcePathVariable("JarArgsCmd", "build/soong/scripts/jar-args.sh")
	pctx.SourcePathVariable("PackageCheckCmd", "build/soong/scripts/package-check.sh")
	pctx.HostBinToolVariable("ExtractJarPackagesCmd", "extract_jar_packages")
	pctx.HostBinToolVariable("SoongZipCmd", "soong_zip")
	pctx.HostBinToolVariable("MergeZipsCmd", "merge_zips")
	pctx.HostBinToolVariable("Zip2ZipCmd", "zip2zip")
	pctx.HostBinToolVariable("ZipSyncCmd", "zipsync")
	pctx.HostBinToolVariable("ApiCheckCmd", "apicheck")
	pctx.HostBinToolVariable("D8Cmd", "d8")
	pctx.HostBinToolVariable("R8Cmd", "r8-compat-proguard")
	pctx.HostBinToolVariable("HiddenAPICmd", "hiddenapi")

	pctx.VariableFunc("TurbineJar", func(ctx android.PackageVarContext) string {
		turbine := "turbine.jar"
		if ctx.Config().UnbundledBuild() {
			return "prebuilts/build-tools/common/framework/" + turbine
		} else {
			return pctx.HostJavaToolPath(ctx, turbine).String()
		}
	})

	pctx.HostJavaToolVariable("JarjarCmd", "jarjar.jar")
	pctx.HostJavaToolVariable("JsilverJar", "jsilver.jar")
	pctx.HostJavaToolVariable("DoclavaJar", "doclava.jar")
	pctx.HostJavaToolVariable("MetalavaJar", "metalava.jar")
	pctx.HostJavaToolVariable("DokkaJar", "dokka.jar")
	pctx.HostJavaToolVariable("JetifierJar", "jetifier.jar")

	pctx.HostBinToolVariable("SoongJavacWrapper", "soong_javac_wrapper")
	pctx.HostBinToolVariable("DexpreoptGen", "dexpreopt_gen")

	pctx.VariableFunc("JavacWrapper", func(ctx android.PackageVarContext) string {
		if override := ctx.Config().Getenv("JAVAC_WRAPPER"); override != "" {
			return override + " "
		}
		return ""
	})

	pctx.HostJavaToolVariable("JacocoCLIJar", "jacoco-cli.jar")

	hostBinToolVariableWithPrebuilt := func(name, prebuiltDir, tool string) {
		pctx.VariableFunc(name, func(ctx android.PackageVarContext) string {
			if ctx.Config().UnbundledBuild() || ctx.Config().IsPdkBuild() {
				return filepath.Join(prebuiltDir, runtime.GOOS, "bin", tool)
			} else {
				return pctx.HostBinToolPath(ctx, tool).String()
			}
		})
	}

	hostBinToolVariableWithPrebuilt("Aapt2Cmd", "prebuilts/sdk/tools", "aapt2")

	pctx.SourcePathVariable("ManifestFixerCmd", "build/soong/scripts/manifest_fixer.py")

	pctx.HostBinToolVariable("ManifestMergerCmd", "manifest-merger")

	pctx.HostBinToolVariable("ZipAlign", "zipalign")

	pctx.HostBinToolVariable("Class2Greylist", "class2greylist")
	pctx.HostBinToolVariable("HiddenAPI", "hiddenapi")
}
