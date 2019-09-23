// Copyright 2017 Google Inc. All rights reserved.
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
	"fmt"
	"io"
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
)

// OpenJDK 9 introduces the concept of "system modules", which replace the bootclasspath.  This
// file will produce the rules necessary to convert each unique set of bootclasspath jars into
// system modules in a runtime image using the jmod and jlink tools.

func init() {
	android.RegisterModuleType("java_system_modules", SystemModulesFactory)

	pctx.SourcePathVariable("moduleInfoJavaPath", "build/soong/scripts/jars-to-module-info-java.sh")
}

var (
	jarsTosystemModules = pctx.AndroidStaticRule("jarsTosystemModules", blueprint.RuleParams{
		Command: `rm -rf ${outDir} ${workDir} && mkdir -p ${workDir}/jmod && ` +
			`${moduleInfoJavaPath} ${moduleName} $in > ${workDir}/module-info.java && ` +
			`${config.JavacCmd} --system=none --patch-module=java.base=${classpath} ${workDir}/module-info.java && ` +
			`${config.SoongZipCmd} -jar -o ${workDir}/classes.jar -C ${workDir} -f ${workDir}/module-info.class && ` +
			`${config.MergeZipsCmd} -j ${workDir}/module.jar ${workDir}/classes.jar $in && ` +
			`${config.JmodCmd} create --module-version 9 --target-platform android ` +
			`  --class-path ${workDir}/module.jar ${workDir}/jmod/${moduleName}.jmod && ` +
			`${config.JlinkCmd} --module-path ${workDir}/jmod --add-modules ${moduleName} --output ${outDir} ` +
			// Note: The system-modules jlink plugin is disabled because (a) it is not
			// useful on Android, and (b) it causes errors with later versions of jlink
			// when the jdk.internal.module is absent from java.base (as it is here).
			`  --disable-plugin system-modules && ` +
			`cp ${config.JrtFsJar} ${outDir}/lib/`,
		CommandDeps: []string{
			"${moduleInfoJavaPath}",
			"${config.JavacCmd}",
			"${config.SoongZipCmd}",
			"${config.MergeZipsCmd}",
			"${config.JmodCmd}",
			"${config.JlinkCmd}",
			"${config.JrtFsJar}",
		},
	},
		"moduleName", "classpath", "outDir", "workDir")
)

func TransformJarsToSystemModules(ctx android.ModuleContext, moduleName string, jars android.Paths) (android.Path, android.Paths) {
	outDir := android.PathForModuleOut(ctx, "system")
	workDir := android.PathForModuleOut(ctx, "modules")
	outputFile := android.PathForModuleOut(ctx, "system/lib/modules")
	outputs := android.WritablePaths{
		outputFile,
		android.PathForModuleOut(ctx, "system/lib/jrt-fs.jar"),
		android.PathForModuleOut(ctx, "system/release"),
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        jarsTosystemModules,
		Description: "system modules",
		Outputs:     outputs,
		Inputs:      jars,
		Args: map[string]string{
			"moduleName": moduleName,
			"classpath":  strings.Join(jars.Strings(), ":"),
			"workDir":    workDir.String(),
			"outDir":     outDir.String(),
		},
	})

	return outDir, outputs.Paths()
}

func SystemModulesFactory() android.Module {
	module := &SystemModules{}
	module.AddProperties(&module.properties)
	android.InitAndroidArchModule(module, android.HostAndDeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

type SystemModules struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties SystemModulesProperties

	// The aggregated header jars from all jars specified in the libs property.
	// Used when system module is added as a dependency to bootclasspath.
	headerJars android.Paths
	outputDir  android.Path
	outputDeps android.Paths
}

type SystemModulesProperties struct {
	// List of java library modules that should be included in the system modules
	Libs []string
}

func (system *SystemModules) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	var jars android.Paths

	ctx.VisitDirectDepsWithTag(libTag, func(module android.Module) {
		dep, _ := module.(Dependency)
		jars = append(jars, dep.HeaderJars()...)
	})

	system.headerJars = jars

	system.outputDir, system.outputDeps = TransformJarsToSystemModules(ctx, "java.base", jars)
}

func (system *SystemModules) DepsMutator(ctx android.BottomUpMutatorContext) {
	ctx.AddVariationDependencies(nil, libTag, system.properties.Libs...)
}

func (system *SystemModules) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
			fmt.Fprintln(w)

			makevar := "SOONG_SYSTEM_MODULES_" + name
			fmt.Fprintln(w, makevar, ":=$=", system.outputDir.String())
			fmt.Fprintln(w)

			makevar = "SOONG_SYSTEM_MODULES_LIBS_" + name
			fmt.Fprintln(w, makevar, ":=$=", strings.Join(system.properties.Libs, " "))
			fmt.Fprintln(w)

			makevar = "SOONG_SYSTEM_MODULES_DEPS_" + name
			fmt.Fprintln(w, makevar, ":=$=", strings.Join(system.outputDeps.Strings(), " "))
			fmt.Fprintln(w)

			fmt.Fprintln(w, name+":", "$("+makevar+")")
			fmt.Fprintln(w, ".PHONY:", name)
		},
	}
}
