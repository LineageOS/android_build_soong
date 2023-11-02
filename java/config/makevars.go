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
	ctx.Strict("FRAMEWORK_LIBRARIES", strings.Join(FrameworkLibraries, " "))

	// These are used by make when LOCAL_PRIVATE_PLATFORM_APIS is set (equivalent to platform_apis in blueprint):
	ctx.Strict("LEGACY_CORE_PLATFORM_BOOTCLASSPATH_LIBRARIES",
		strings.Join(LegacyCorePlatformBootclasspathLibraries, " "))
	ctx.Strict("LEGACY_CORE_PLATFORM_SYSTEM_MODULES", LegacyCorePlatformSystemModules)

	ctx.Strict("ANDROID_JAVA_HOME", "${JavaHome}")
	ctx.Strict("ANDROID_JAVA8_HOME", "prebuilts/jdk/jdk8/${hostPrebuiltTag}")
	ctx.Strict("ANDROID_JAVA_TOOLCHAIN", "${JavaToolchain}")
	ctx.Strict("JAVA", "${JavaCmd} ${JavaVmFlags}")
	ctx.Strict("JAVAC", "${JavacCmd} ${JavacVmFlags}")
	ctx.Strict("JAR", "${JarCmd}")
	ctx.Strict("JAR_ARGS", "${JarArgsCmd}")
	ctx.Strict("JAVADOC", "${JavadocCmd}")
	ctx.Strict("COMMON_JDK_FLAGS", "${CommonJdkFlags}")

	ctx.Strict("D8", "${D8Cmd}")
	ctx.Strict("R8", "${R8Cmd}")
	ctx.Strict("D8_COMMAND", "${D8Cmd} ${D8Flags}")
	ctx.Strict("R8_COMMAND", "${R8Cmd} ${R8Flags}")

	ctx.Strict("TURBINE", "${TurbineJar}")

	if ctx.Config().RunErrorProne() {
		ctx.Strict("ERROR_PRONE_JARS", strings.Join(ErrorProneClasspath, " "))
		ctx.Strict("ERROR_PRONE_FLAGS", "${ErrorProneFlags}")
		ctx.Strict("ERROR_PRONE_CHECKS", "${ErrorProneChecks}")
	}

	ctx.Strict("TARGET_JAVAC", "${JavacCmd}  ${JavacVmFlags} ${CommonJdkFlags}")
	ctx.Strict("HOST_JAVAC", "${JavacCmd}  ${JavacVmFlags} ${CommonJdkFlags}")

	ctx.Strict("JLINK", "${JlinkCmd}")
	ctx.Strict("JMOD", "${JmodCmd}")

	ctx.Strict("SOONG_JAVAC_WRAPPER", "${SoongJavacWrapper}")
	ctx.Strict("DEXPREOPT_GEN", "${DexpreoptGen}")
	ctx.Strict("ZIPSYNC", "${ZipSyncCmd}")

	ctx.Strict("JACOCO_CLI_JAR", "${JacocoCLIJar}")
	ctx.Strict("DEFAULT_JACOCO_EXCLUDE_FILTER", strings.Join(DefaultMakeJacocoExcludeFilter, ","))

	ctx.Strict("EXTRACT_JAR_PACKAGES", "${ExtractJarPackagesCmd}")

	ctx.Strict("MANIFEST_CHECK", "${ManifestCheckCmd}")
	ctx.Strict("MANIFEST_FIXER", "${ManifestFixerCmd}")

	ctx.Strict("ANDROID_MANIFEST_MERGER", "${ManifestMergerCmd}")

	ctx.Strict("CLASS2NONSDKLIST", "${Class2NonSdkList}")
	ctx.Strict("HIDDENAPI", "${HiddenAPI}")

	ctx.Strict("AIDL", "${AidlCmd}")
	ctx.Strict("AAPT2", "${Aapt2Cmd}")
	ctx.Strict("ZIPALIGN", "${ZipAlign}")
	ctx.Strict("SIGNAPK_JAR", "${SignapkCmd}")
	ctx.Strict("SIGNAPK_JNI_LIBRARY_PATH", "${SignapkJniLibrary}")

	ctx.Strict("SOONG_ZIP", "${SoongZipCmd}")
	ctx.Strict("MERGE_ZIPS", "${MergeZipsCmd}")
	ctx.Strict("ZIP2ZIP", "${Zip2ZipCmd}")

	ctx.Strict("ZIPTIME", "${Ziptime}")

}
