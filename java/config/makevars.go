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

	"android/soong/android"
)

func init() {
	android.RegisterMakeVarsProvider(pctx, makeVarsProvider)
}

func makeVarsProvider(ctx android.MakeVarsContext) {
	ctx.Strict("TARGET_DEFAULT_JAVA_LIBRARIES", strings.Join(DefaultLibraries, " "))
	ctx.Strict("TARGET_DEFAULT_BOOTCLASSPATH_LIBRARIES", strings.Join(DefaultBootclasspathLibraries, " "))
	ctx.Strict("DEFAULT_SYSTEM_MODULES", DefaultSystemModules)

	if ctx.Config().TargetOpenJDK9() {
		ctx.Strict("DEFAULT_JAVA_LANGUAGE_VERSION", "1.9")
	} else {
		ctx.Strict("DEFAULT_JAVA_LANGUAGE_VERSION", "1.8")
	}

	ctx.Strict("ANDROID_JAVA_HOME", "${JavaHome}")
	ctx.Strict("ANDROID_JAVA8_HOME", "prebuilts/jdk/jdk8/${hostPrebuiltTag}")
	ctx.Strict("ANDROID_JAVA9_HOME", "prebuilts/jdk/jdk9/${hostPrebuiltTag}")
	ctx.Strict("ANDROID_JAVA_TOOLCHAIN", "${JavaToolchain}")
	ctx.Strict("JAVA", "${JavaCmd}")
	ctx.Strict("JAVAC", "${JavacCmd}")
	ctx.Strict("JAR", "${JarCmd}")
	ctx.Strict("JAR_ARGS", "${JarArgsCmd}")
	ctx.Strict("JAVADOC", "${JavadocCmd}")
	ctx.Strict("COMMON_JDK_FLAGS", "${CommonJdkFlags}")

	if ctx.Config().UseD8Desugar() {
		ctx.Strict("DX", "${D8Cmd}")
		ctx.Strict("DX_COMMAND", "${D8Cmd} -JXms16M -JXmx2048M")
		ctx.Strict("USE_D8_DESUGAR", "true")
	} else {
		ctx.Strict("DX", "${DxCmd}")
		ctx.Strict("DX_COMMAND", "${DxCmd} -JXms16M -JXmx2048M")
		ctx.Strict("USE_D8_DESUGAR", "false")
	}
	ctx.Strict("R8_COMPAT_PROGUARD", "${R8Cmd}")

	ctx.Strict("TURBINE", "${TurbineJar}")

	if ctx.Config().IsEnvTrue("RUN_ERROR_PRONE") {
		ctx.Strict("TARGET_JAVAC", "${ErrorProneCmd}")
		ctx.Strict("HOST_JAVAC", "${ErrorProneCmd}")
	} else {
		ctx.Strict("TARGET_JAVAC", "${JavacCmd} ${CommonJdkFlags}")
		ctx.Strict("HOST_JAVAC", "${JavacCmd} ${CommonJdkFlags}")
	}

	if ctx.Config().UseOpenJDK9() {
		ctx.Strict("JLINK", "${JlinkCmd}")
		ctx.Strict("JMOD", "${JmodCmd}")
	}

	ctx.Strict("SOONG_JAVAC_WRAPPER", "${SoongJavacWrapper}")
	ctx.Strict("ZIPSYNC", "${ZipSyncCmd}")

	ctx.Strict("JACOCO_CLI_JAR", "${JacocoCLIJar}")
	ctx.Strict("DEFAULT_JACOCO_EXCLUDE_FILTER", strings.Join(DefaultJacocoExcludeFilter, ","))

	ctx.Strict("EXTRACT_JAR_PACKAGES", "${ExtractJarPackagesCmd}")

	ctx.Strict("DEX_FLAGS", "${DexFlags}")
}
