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
	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

// OpenJDK 9 introduces the concept of "system modules", which replace the bootclasspath.  This
// file will produce the rules necessary to convert each unique set of bootclasspath jars into
// system modules in a runtime image using the jmod and jlink tools.

func init() {
	RegisterSystemModulesBuildComponents(android.InitRegistrationContext)

	pctx.SourcePathVariable("moduleInfoJavaPath", "build/soong/scripts/jars-to-module-info-java.sh")

	// Register sdk member types.
	android.RegisterSdkMemberType(&systemModulesSdkMemberType{
		android.SdkMemberTypeBase{
			PropertyName: "java_system_modules",
			SupportsSdk:  true,
		},
	})
}

func RegisterSystemModulesBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("java_system_modules", SystemModulesFactory)
	ctx.RegisterModuleType("java_system_modules_import", systemModulesImportFactory)
}

var (
	jarsTosystemModules = pctx.AndroidStaticRule("jarsTosystemModules", blueprint.RuleParams{
		Command: `rm -rf ${outDir} ${workDir} && mkdir -p ${workDir}/jmod && ` +
			`${moduleInfoJavaPath} java.base $in > ${workDir}/module-info.java && ` +
			`${config.JavacCmd} --system=none --patch-module=java.base=${classpath} ${workDir}/module-info.java && ` +
			`${config.SoongZipCmd} -jar -o ${workDir}/classes.jar -C ${workDir} -f ${workDir}/module-info.class && ` +
			`${config.MergeZipsCmd} -j ${workDir}/module.jar ${workDir}/classes.jar $in && ` +
			// Note: The version of the java.base module created must match the version
			// of the jlink tool which consumes it.
			// Use LINUX-OTHER to be compatible with JDK 21+ (b/294137077)
			`${config.JmodCmd} create --module-version ${config.JlinkVersion} --target-platform LINUX-OTHER ` +
			`  --class-path ${workDir}/module.jar ${workDir}/jmod/java.base.jmod && ` +
			`${config.JlinkCmd} --module-path ${workDir}/jmod --add-modules java.base --output ${outDir} ` +
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
		"classpath", "outDir", "workDir")

	// Dependency tag that causes the added dependencies to be added as java_header_libs
	// to the sdk/module_exports/snapshot. Dependencies that are added automatically via this tag are
	// not automatically exported.
	systemModulesLibsTag = android.DependencyTagForSdkMemberType(javaHeaderLibsSdkMemberType, false)
)

func TransformJarsToSystemModules(ctx android.ModuleContext, jars android.Paths) (android.Path, android.Paths) {
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
			"classpath": strings.Join(jars.Strings(), ":"),
			"workDir":   workDir.String(),
			"outDir":    outDir.String(),
		},
	})

	return outDir, outputs.Paths()
}

// java_system_modules creates a system module from a set of java libraries that can
// be referenced from the system_modules property. It must contain at a minimum the
// java.base module which must include classes from java.lang amongst other java packages.
func SystemModulesFactory() android.Module {
	module := &SystemModules{}
	module.AddProperties(&module.properties)
	android.InitAndroidArchModule(module, android.HostAndDeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

type SystemModulesProvider interface {
	HeaderJars() android.Paths
	OutputDirAndDeps() (android.Path, android.Paths)
}

var _ SystemModulesProvider = (*SystemModules)(nil)

var _ SystemModulesProvider = (*systemModulesImport)(nil)

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

func (system *SystemModules) HeaderJars() android.Paths {
	return system.headerJars
}

func (system *SystemModules) OutputDirAndDeps() (android.Path, android.Paths) {
	if system.outputDir == nil || len(system.outputDeps) == 0 {
		panic("Missing directory for system module dependency")
	}
	return system.outputDir, system.outputDeps
}

func (system *SystemModules) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	var jars android.Paths

	ctx.VisitDirectDepsWithTag(systemModulesLibsTag, func(module android.Module) {
		dep, _ := android.OtherModuleProvider(ctx, module, JavaInfoProvider)
		jars = append(jars, dep.HeaderJars...)
	})

	system.headerJars = jars

	system.outputDir, system.outputDeps = TransformJarsToSystemModules(ctx, jars)
}

// ComponentDepsMutator is called before prebuilt modules without a corresponding source module are
// renamed so unless the supplied libs specifically includes the prebuilt_ prefix this is guaranteed
// to only add dependencies on source modules.
//
// The systemModuleLibsTag will prevent the prebuilt mutators from replacing this dependency so it
// will never be changed to depend on a prebuilt either.
func (system *SystemModules) ComponentDepsMutator(ctx android.BottomUpMutatorContext) {
	ctx.AddVariationDependencies(nil, systemModulesLibsTag, system.properties.Libs...)
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
			// TODO(b/151177513): Licenses: Doesn't go through base_rules. May have to generate meta_lic and meta_module here.
		},
	}
}

// A prebuilt version of java_system_modules. It does not import the
// generated system module, it generates the system module from imported
// java libraries in the same way that java_system_modules does. It just
// acts as a prebuilt, i.e. can have the same base name as another module
// type and the one to use is selected at runtime.
func systemModulesImportFactory() android.Module {
	module := &systemModulesImport{}
	module.AddProperties(&module.properties, &module.prebuiltProperties)
	android.InitPrebuiltModule(module, &module.properties.Libs)
	android.InitAndroidArchModule(module, android.HostAndDeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}

type systemModulesImport struct {
	SystemModules
	prebuilt           android.Prebuilt
	prebuiltProperties prebuiltSystemModulesProperties
}

type prebuiltSystemModulesProperties struct {
	// Name of the source soong module that gets shadowed by this prebuilt
	// If unspecified, follows the naming convention that the source module of
	// the prebuilt is Name() without "prebuilt_" prefix
	Source_module_name *string
}

func (system *systemModulesImport) Name() string {
	return system.prebuilt.Name(system.ModuleBase.Name())
}

// BaseModuleName returns the source module that will get shadowed by this prebuilt
// e.g.
//
//	java_system_modules_import {
//	   name: "my_system_modules.v1",
//	   source_module_name: "my_system_modules",
//	}
//
//	java_system_modules_import {
//	   name: "my_system_modules.v2",
//	   source_module_name: "my_system_modules",
//	}
//
// `BaseModuleName` for both will return `my_system_modules`
func (system *systemModulesImport) BaseModuleName() string {
	return proptools.StringDefault(system.prebuiltProperties.Source_module_name, system.ModuleBase.Name())
}

func (system *systemModulesImport) Prebuilt() *android.Prebuilt {
	return &system.prebuilt
}

// ComponentDepsMutator is called before prebuilt modules without a corresponding source module are
// renamed so as this adds a prebuilt_ prefix this is guaranteed to only add dependencies on source
// modules.
func (system *systemModulesImport) ComponentDepsMutator(ctx android.BottomUpMutatorContext) {
	for _, lib := range system.properties.Libs {
		ctx.AddVariationDependencies(nil, systemModulesLibsTag, android.PrebuiltNameFromSource(lib))
	}
}

type systemModulesSdkMemberType struct {
	android.SdkMemberTypeBase
}

func (mt *systemModulesSdkMemberType) AddDependencies(ctx android.SdkDependencyContext, dependencyTag blueprint.DependencyTag, names []string) {
	ctx.AddVariationDependencies(nil, dependencyTag, names...)
}

func (mt *systemModulesSdkMemberType) IsInstance(module android.Module) bool {
	if _, ok := module.(*SystemModules); ok {
		// A prebuilt system module cannot be added as a member of an sdk because the source and
		// snapshot instances would conflict.
		_, ok := module.(*systemModulesImport)
		return !ok
	}
	return false
}

func (mt *systemModulesSdkMemberType) AddPrebuiltModule(ctx android.SdkMemberContext, member android.SdkMember) android.BpModule {
	return ctx.SnapshotBuilder().AddPrebuiltModule(member, "java_system_modules_import")
}

type systemModulesInfoProperties struct {
	android.SdkMemberPropertiesBase

	Libs []string
}

func (mt *systemModulesSdkMemberType) CreateVariantPropertiesStruct() android.SdkMemberProperties {
	return &systemModulesInfoProperties{}
}

func (p *systemModulesInfoProperties) PopulateFromVariant(ctx android.SdkMemberContext, variant android.Module) {
	systemModule := variant.(*SystemModules)
	p.Libs = systemModule.properties.Libs
}

func (p *systemModulesInfoProperties) AddToPropertySet(ctx android.SdkMemberContext, propertySet android.BpPropertySet) {
	if len(p.Libs) > 0 {
		// Add the references to the libraries that form the system module.
		propertySet.AddPropertyWithTag("libs", p.Libs, ctx.SnapshotBuilder().SdkMemberReferencePropertyTag(true))
	}
}
