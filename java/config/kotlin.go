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

import "strings"

var (
	KotlinStdlibJar     = "external/kotlinc/lib/kotlin-stdlib.jar"
	KotlincIllegalFlags = []string{
		"-no-jdk",
		"-no-stdlib",
	}
)

func init() {
	pctx.SourcePathVariable("KotlincCmd", "external/kotlinc/bin/kotlinc")
	pctx.SourcePathVariable("KotlinCompilerJar", "external/kotlinc/lib/kotlin-compiler.jar")
	pctx.SourcePathVariable("KotlinPreloaderJar", "external/kotlinc/lib/kotlin-preloader.jar")
	pctx.SourcePathVariable("KotlinReflectJar", "external/kotlinc/lib/kotlin-reflect.jar")
	pctx.SourcePathVariable("KotlinScriptRuntimeJar", "external/kotlinc/lib/kotlin-script-runtime.jar")
	pctx.SourcePathVariable("KotlinTrove4jJar", "external/kotlinc/lib/trove4j.jar")
	pctx.SourcePathVariable("KotlinKaptJar", "external/kotlinc/lib/kotlin-annotation-processing.jar")
	pctx.SourcePathVariable("KotlinAnnotationJar", "external/kotlinc/lib/annotations-13.0.jar")
	pctx.SourcePathVariable("KotlinStdlibJar", KotlinStdlibJar)
	pctx.SourcePathVariable("KotlinAbiGenPluginJar", "external/kotlinc/lib/jvm-abi-gen.jar")

	// These flags silence "Illegal reflective access" warnings when running kapt in OpenJDK9+
	pctx.StaticVariable("KaptSuppressJDK9Warnings", strings.Join([]string{
		"-J--add-exports=jdk.compiler/com.sun.tools.javac.file=ALL-UNNAMED",
		"-J--add-exports=jdk.compiler/com.sun.tools.javac.tree=ALL-UNNAMED",
		"-J--add-exports=jdk.compiler/com.sun.tools.javac.main=ALL-UNNAMED",
		"-J--add-opens=java.base/sun.net.www.protocol.jar=ALL-UNNAMED",
	}, " "))

	// These flags silence "Illegal reflective access" warnings when running kotlinc in OpenJDK9+
	pctx.StaticVariable("KotlincSuppressJDK9Warnings", strings.Join([]string{
		"-J--add-opens=java.base/java.util=ALL-UNNAMED", // https://youtrack.jetbrains.com/issue/KT-43704
	}, " "))

	pctx.StaticVariable("KotlincGlobalFlags", strings.Join([]string{}, " "))
}
