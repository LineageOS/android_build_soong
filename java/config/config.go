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
	"android/soong/remoteexec"
)

var (
	pctx = android.NewPackageContext("android/soong/java/config")

	LegacyCorePlatformBootclasspathLibraries = []string{"legacy.core.platform.api.stubs", "core-lambda-stubs"}
	LegacyCorePlatformSystemModules          = "legacy-core-platform-api-stubs-system-modules"
	StableCorePlatformBootclasspathLibraries = []string{"stable.core.platform.api.stubs", "core-lambda-stubs"}
	StableCorePlatformSystemModules          = "stable-core-platform-api-stubs-system-modules"
	FrameworkLibraries                       = []string{"ext", "framework"}
	DefaultLambdaStubsLibrary                = "core-lambda-stubs"
	SdkLambdaStubsPath                       = "prebuilts/sdk/tools/core-lambda-stubs.jar"

	DefaultMakeJacocoExcludeFilter = []string{"org.junit.*", "org.jacoco.*", "org.mockito.*"}
	DefaultJacocoExcludeFilter     = []string{"org.junit.**", "org.jacoco.**", "org.mockito.**"}

	InstrumentFrameworkModules = []string{
		"framework",
		"framework-minus-apex",
		"telephony-common",
		"services",
		"android.car",
		"android.car7",
		"conscrypt",
		"core-icu4j",
		"core-oj",
		"core-libart",
		// TODO: Could this be all updatable bootclasspath jars?
		"updatable-media",
		"framework-mediaprovider",
		"framework-sdkextensions",
		"android.net.ipsec.ike",
	}
)

const (
	JavaVmFlags  = `-XX:OnError="cat hs_err_pid%p.log" -XX:CICompilerCount=6 -XX:+UseDynamicNumberOfGCThreads`
	JavacVmFlags = `-J-XX:OnError="cat hs_err_pid%p.log" -J-XX:CICompilerCount=6 -J-XX:+UseDynamicNumberOfGCThreads -J-XX:+TieredCompilation -J-XX:TieredStopAtLevel=1`
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

	pctx.StaticVariable("JavaVmFlags", JavaVmFlags)
	pctx.StaticVariable("JavacVmFlags", JavacVmFlags)

	pctx.VariableConfigMethod("hostPrebuiltTag", android.Config.PrebuiltOS)

	pctx.VariableFunc("JavaHome", func(ctx android.PackageVarContext) string {
		// This is set up and guaranteed by soong_ui
		return ctx.Config().Getenv("ANDROID_JAVA_HOME")
	})
	pctx.VariableFunc("JlinkVersion", func(ctx android.PackageVarContext) string {
		if override := ctx.Config().Getenv("OVERRIDE_JLINK_VERSION_NUMBER"); override != "" {
			return override
		}
		return "11"
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
	pctx.SourcePathVariable("JavaKytheExtractorJar", "prebuilts/build-tools/common/framework/javac_extractor.jar")
	pctx.SourcePathVariable("Ziptime", "prebuilts/build-tools/${hostPrebuiltTag}/bin/ziptime")

	pctx.HostBinToolVariable("GenKotlinBuildFileCmd", "gen-kotlin-build-file.py")

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
	pctx.HostBinToolVariable("ExtractApksCmd", "extract_apks")
	pctx.VariableFunc("TurbineJar", func(ctx android.PackageVarContext) string {
		turbine := "turbine.jar"
		if ctx.Config().AlwaysUsePrebuiltSdks() {
			return "prebuilts/build-tools/common/framework/" + turbine
		} else {
			return ctx.Config().HostJavaToolPath(ctx, turbine).String()
		}
	})

	pctx.HostJavaToolVariable("JarjarCmd", "jarjar.jar")
	pctx.HostJavaToolVariable("JsilverJar", "jsilver.jar")
	pctx.HostJavaToolVariable("DoclavaJar", "doclava.jar")
	pctx.HostJavaToolVariable("MetalavaJar", "metalava.jar")
	pctx.HostJavaToolVariable("DokkaJar", "dokka.jar")
	pctx.HostJavaToolVariable("JetifierJar", "jetifier.jar")
	pctx.HostJavaToolVariable("R8Jar", "r8-compat-proguard.jar")
	pctx.HostJavaToolVariable("D8Jar", "d8.jar")

	pctx.HostBinToolVariable("SoongJavacWrapper", "soong_javac_wrapper")
	pctx.HostBinToolVariable("DexpreoptGen", "dexpreopt_gen")

	pctx.StaticVariableWithEnvOverride("REJavaPool", "RBE_JAVA_POOL", "java16")
	pctx.StaticVariableWithEnvOverride("REJavacExecStrategy", "RBE_JAVAC_EXEC_STRATEGY", remoteexec.RemoteLocalFallbackExecStrategy)
	pctx.StaticVariableWithEnvOverride("RED8ExecStrategy", "RBE_D8_EXEC_STRATEGY", remoteexec.RemoteLocalFallbackExecStrategy)
	pctx.StaticVariableWithEnvOverride("RER8ExecStrategy", "RBE_R8_EXEC_STRATEGY", remoteexec.RemoteLocalFallbackExecStrategy)
	pctx.StaticVariableWithEnvOverride("RETurbineExecStrategy", "RBE_TURBINE_EXEC_STRATEGY", remoteexec.LocalExecStrategy)
	pctx.StaticVariableWithEnvOverride("RESignApkExecStrategy", "RBE_SIGNAPK_EXEC_STRATEGY", remoteexec.LocalExecStrategy)
	pctx.StaticVariableWithEnvOverride("REJarExecStrategy", "RBE_JAR_EXEC_STRATEGY", remoteexec.LocalExecStrategy)
	pctx.StaticVariableWithEnvOverride("REZipExecStrategy", "RBE_ZIP_EXEC_STRATEGY", remoteexec.LocalExecStrategy)

	pctx.HostJavaToolVariable("JacocoCLIJar", "jacoco-cli.jar")

	pctx.HostBinToolVariable("ManifestCheckCmd", "manifest_check")
	pctx.HostBinToolVariable("ManifestFixerCmd", "manifest_fixer")

	pctx.HostBinToolVariable("ManifestMergerCmd", "manifest-merger")

	pctx.HostBinToolVariable("Class2NonSdkList", "class2nonsdklist")
	pctx.HostBinToolVariable("HiddenAPI", "hiddenapi")

	hostBinToolVariableWithSdkToolsPrebuilt("Aapt2Cmd", "aapt2")
	hostBinToolVariableWithBuildToolsPrebuilt("AidlCmd", "aidl")
	hostBinToolVariableWithBuildToolsPrebuilt("ZipAlign", "zipalign")

	hostJavaToolVariableWithSdkToolsPrebuilt("SignapkCmd", "signapk")
	// TODO(ccross): this should come from the signapk dependencies, but we don't have any way
	// to express host JNI dependencies yet.
	hostJNIToolVariableWithSdkToolsPrebuilt("SignapkJniLibrary", "libconscrypt_openjdk_jni")
}

func hostBinToolVariableWithSdkToolsPrebuilt(name, tool string) {
	pctx.VariableFunc(name, func(ctx android.PackageVarContext) string {
		if ctx.Config().AlwaysUsePrebuiltSdks() {
			return filepath.Join("prebuilts/sdk/tools", runtime.GOOS, "bin", tool)
		} else {
			return ctx.Config().HostToolPath(ctx, tool).String()
		}
	})
}

func hostJavaToolVariableWithSdkToolsPrebuilt(name, tool string) {
	pctx.VariableFunc(name, func(ctx android.PackageVarContext) string {
		if ctx.Config().AlwaysUsePrebuiltSdks() {
			return filepath.Join("prebuilts/sdk/tools/lib", tool+".jar")
		} else {
			return ctx.Config().HostJavaToolPath(ctx, tool+".jar").String()
		}
	})
}

func hostJNIToolVariableWithSdkToolsPrebuilt(name, tool string) {
	pctx.VariableFunc(name, func(ctx android.PackageVarContext) string {
		if ctx.Config().AlwaysUsePrebuiltSdks() {
			ext := ".so"
			if runtime.GOOS == "darwin" {
				ext = ".dylib"
			}
			return filepath.Join("prebuilts/sdk/tools", runtime.GOOS, "lib64", tool+ext)
		} else {
			return ctx.Config().HostJNIToolPath(ctx, tool).String()
		}
	})
}

func hostBinToolVariableWithBuildToolsPrebuilt(name, tool string) {
	pctx.VariableFunc(name, func(ctx android.PackageVarContext) string {
		if ctx.Config().AlwaysUsePrebuiltSdks() {
			return filepath.Join("prebuilts/build-tools", ctx.Config().PrebuiltOS(), "bin", tool)
		} else {
			return ctx.Config().HostToolPath(ctx, tool).String()
		}
	})
}

// JavaCmd returns a SourcePath object with the path to the java command.
func JavaCmd(ctx android.PathContext) android.SourcePath {
	return javaTool(ctx, "java")
}

// JavadocCmd returns a SourcePath object with the path to the java command.
func JavadocCmd(ctx android.PathContext) android.SourcePath {
	return javaTool(ctx, "javadoc")
}

func javaTool(ctx android.PathContext, tool string) android.SourcePath {
	type javaToolKey string

	key := android.NewCustomOnceKey(javaToolKey(tool))

	return ctx.Config().OnceSourcePath(key, func() android.SourcePath {
		return javaToolchain(ctx).Join(ctx, tool)
	})

}

var javaToolchainKey = android.NewOnceKey("javaToolchain")

func javaToolchain(ctx android.PathContext) android.SourcePath {
	return ctx.Config().OnceSourcePath(javaToolchainKey, func() android.SourcePath {
		return javaHome(ctx).Join(ctx, "bin")
	})
}

var javaHomeKey = android.NewOnceKey("javaHome")

func javaHome(ctx android.PathContext) android.SourcePath {
	return ctx.Config().OnceSourcePath(javaHomeKey, func() android.SourcePath {
		// This is set up and guaranteed by soong_ui
		return android.PathForSource(ctx, ctx.Config().Getenv("ANDROID_JAVA_HOME"))
	})
}
