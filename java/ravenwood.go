// Copyright 2023 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package java

import (
	"android/soong/android"
	"android/soong/tradefed"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

func init() {
	RegisterRavenwoodBuildComponents(android.InitRegistrationContext)
}

func RegisterRavenwoodBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("android_ravenwood_test", ravenwoodTestFactory)
	ctx.RegisterModuleType("android_ravenwood_libgroup", ravenwoodLibgroupFactory)
}

var ravenwoodLibContentTag = dependencyTag{name: "ravenwoodlibcontent"}
var ravenwoodUtilsTag = dependencyTag{name: "ravenwoodutils"}
var ravenwoodRuntimeTag = dependencyTag{name: "ravenwoodruntime"}

const ravenwoodUtilsName = "ravenwood-utils"
const ravenwoodRuntimeName = "ravenwood-runtime"

type ravenwoodLibgroupJniDepProviderInfo struct {
	// All the jni_libs module names with transient dependencies.
	names map[string]bool
}

var ravenwoodLibgroupJniDepProvider = blueprint.NewProvider[ravenwoodLibgroupJniDepProviderInfo]()

func getLibPath(archType android.ArchType) string {
	if archType.Multilib == "lib64" {
		return "lib64"
	}
	return "lib"
}

type ravenwoodTestProperties struct {
	Jni_libs []string
}

type ravenwoodTest struct {
	Library

	ravenwoodTestProperties ravenwoodTestProperties

	testProperties testProperties
	testConfig     android.Path

	forceOSType   android.OsType
	forceArchType android.ArchType
}

func ravenwoodTestFactory() android.Module {
	module := &ravenwoodTest{}

	module.addHostAndDeviceProperties()
	module.AddProperties(&module.testProperties, &module.ravenwoodTestProperties)

	module.Module.dexpreopter.isTest = true
	module.Module.linter.properties.Lint.Test = proptools.BoolPtr(true)

	module.testProperties.Test_suites = []string{
		"general-tests",
		"ravenwood-tests",
	}
	module.testProperties.Test_options.Unit_test = proptools.BoolPtr(false)

	InitJavaModule(module, android.DeviceSupported)
	android.InitDefaultableModule(module)

	return module
}

func (r *ravenwoodTest) InstallInTestcases() bool { return true }
func (r *ravenwoodTest) InstallForceOS() (*android.OsType, *android.ArchType) {
	return &r.forceOSType, &r.forceArchType
}
func (r *ravenwoodTest) TestSuites() []string {
	return r.testProperties.Test_suites
}

func (r *ravenwoodTest) DepsMutator(ctx android.BottomUpMutatorContext) {
	r.Library.DepsMutator(ctx)

	// Generically depend on the runtime so that it's installed together with us
	ctx.AddVariationDependencies(nil, ravenwoodRuntimeTag, ravenwoodRuntimeName)

	// Directly depend on any utils so that we link against them
	utils := ctx.AddVariationDependencies(nil, ravenwoodUtilsTag, ravenwoodUtilsName)[0]
	if utils != nil {
		for _, lib := range utils.(*ravenwoodLibgroup).ravenwoodLibgroupProperties.Libs {
			ctx.AddVariationDependencies(nil, libTag, lib)
		}
	}

	// Add jni libs
	for _, lib := range r.ravenwoodTestProperties.Jni_libs {
		ctx.AddVariationDependencies(ctx.Config().BuildOSTarget.Variations(), jniLibTag, lib)
	}
}

func (r *ravenwoodTest) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	r.forceOSType = ctx.Config().BuildOS
	r.forceArchType = ctx.Config().BuildArch

	r.testConfig = tradefed.AutoGenTestConfig(ctx, tradefed.AutoGenTestConfigOptions{
		TestConfigProp:         r.testProperties.Test_config,
		TestConfigTemplateProp: r.testProperties.Test_config_template,
		TestSuites:             r.testProperties.Test_suites,
		AutoGenConfig:          r.testProperties.Auto_gen_config,
		DeviceTemplate:         "${RavenwoodTestConfigTemplate}",
		HostTemplate:           "${RavenwoodTestConfigTemplate}",
	})

	r.Library.GenerateAndroidBuildActions(ctx)

	// Start by depending on all files installed by dependencies
	var installDeps android.InstallPaths

	// All JNI libraries included in the runtime
	var runtimeJniModuleNames map[string]bool

	if utils := ctx.GetDirectDepsWithTag(ravenwoodUtilsTag)[0]; utils != nil {
		for _, installFile := range utils.FilesToInstall() {
			installDeps = append(installDeps, installFile)
		}
		jniDeps, ok := android.OtherModuleProvider(ctx, utils, ravenwoodLibgroupJniDepProvider)
		if ok {
			runtimeJniModuleNames = jniDeps.names
		}
	}

	if runtime := ctx.GetDirectDepsWithTag(ravenwoodRuntimeTag)[0]; runtime != nil {
		for _, installFile := range runtime.FilesToInstall() {
			installDeps = append(installDeps, installFile)
		}
		jniDeps, ok := android.OtherModuleProvider(ctx, runtime, ravenwoodLibgroupJniDepProvider)
		if ok {
			runtimeJniModuleNames = jniDeps.names
		}
	}

	// Also remember what JNI libs are in the runtime.

	// Also depend on our config
	installPath := android.PathForModuleInstall(ctx, r.BaseModuleName())
	installConfig := ctx.InstallFile(installPath, ctx.ModuleName()+".config", r.testConfig)
	installDeps = append(installDeps, installConfig)

	// Depend on the JNI libraries, but don't install the ones that the runtime already
	// contains.
	soInstallPath := installPath.Join(ctx, getLibPath(r.forceArchType))
	for _, jniLib := range collectTransitiveJniDeps(ctx) {
		if _, ok := runtimeJniModuleNames[jniLib.name]; ok {
			continue // Runtime already includes it.
		}
		installJni := ctx.InstallFile(soInstallPath, jniLib.path.Base(), jniLib.path)
		installDeps = append(installDeps, installJni)
	}

	// Install our JAR with all dependencies
	ctx.InstallFile(installPath, ctx.ModuleName()+".jar", r.outputFile, installDeps...)
}

func (r *ravenwoodTest) AndroidMkEntries() []android.AndroidMkEntries {
	entriesList := r.Library.AndroidMkEntries()
	entries := &entriesList[0]
	entries.ExtraEntries = append(entries.ExtraEntries,
		func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
			entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
			entries.AddStrings("LOCAL_COMPATIBILITY_SUITE",
				"general-tests", "ravenwood-tests")
			if r.testConfig != nil {
				entries.SetPath("LOCAL_FULL_TEST_CONFIG", r.testConfig)
			}
		})
	return entriesList
}

type ravenwoodLibgroupProperties struct {
	Libs []string

	Jni_libs []string
}

type ravenwoodLibgroup struct {
	android.ModuleBase

	ravenwoodLibgroupProperties ravenwoodLibgroupProperties

	forceOSType   android.OsType
	forceArchType android.ArchType
}

func ravenwoodLibgroupFactory() android.Module {
	module := &ravenwoodLibgroup{}
	module.AddProperties(&module.ravenwoodLibgroupProperties)

	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	return module
}

func (r *ravenwoodLibgroup) InstallInTestcases() bool { return true }
func (r *ravenwoodLibgroup) InstallForceOS() (*android.OsType, *android.ArchType) {
	return &r.forceOSType, &r.forceArchType
}
func (r *ravenwoodLibgroup) TestSuites() []string {
	return []string{
		"general-tests",
		"ravenwood-tests",
	}
}

func (r *ravenwoodLibgroup) DepsMutator(ctx android.BottomUpMutatorContext) {
	// Always depends on our underlying libs
	for _, lib := range r.ravenwoodLibgroupProperties.Libs {
		ctx.AddVariationDependencies(nil, ravenwoodLibContentTag, lib)
	}
	for _, lib := range r.ravenwoodLibgroupProperties.Jni_libs {
		ctx.AddVariationDependencies(ctx.Config().BuildOSTarget.Variations(), jniLibTag, lib)
	}
}

func (r *ravenwoodLibgroup) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	r.forceOSType = ctx.Config().BuildOS
	r.forceArchType = ctx.Config().BuildArch

	// Collect the JNI dependencies, including the transitive deps.
	jniDepNames := make(map[string]bool)
	jniLibs := collectTransitiveJniDeps(ctx)

	for _, jni := range jniLibs {
		jniDepNames[jni.name] = true
	}
	android.SetProvider(ctx, ravenwoodLibgroupJniDepProvider, ravenwoodLibgroupJniDepProviderInfo{
		names: jniDepNames,
	})

	// Install our runtime into expected location for packaging
	installPath := android.PathForModuleInstall(ctx, r.BaseModuleName())
	for _, lib := range r.ravenwoodLibgroupProperties.Libs {
		libModule := ctx.GetDirectDepWithTag(lib, ravenwoodLibContentTag)
		libJar := android.OutputFileForModule(ctx, libModule, "")
		ctx.InstallFile(installPath, lib+".jar", libJar)
	}
	soInstallPath := android.PathForModuleInstall(ctx, r.BaseModuleName()).Join(ctx, getLibPath(r.forceArchType))

	for _, jniLib := range jniLibs {
		ctx.InstallFile(soInstallPath, jniLib.path.Base(), jniLib.path)
	}

	// Normal build should perform install steps
	ctx.Phony(r.BaseModuleName(), android.PathForPhony(ctx, r.BaseModuleName()+"-install"))
}

// collectTransitiveJniDeps returns all JNI dependencies, including transitive
// ones, including NDK / stub libs. (Because Ravenwood has no "preinstalled" libraries)
func collectTransitiveJniDeps(ctx android.ModuleContext) []jniLib {
	libs, _ := collectJniDeps(ctx, true, false, nil)
	return libs
}
