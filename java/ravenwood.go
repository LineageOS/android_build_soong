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

	"github.com/google/blueprint/proptools"
)

func init() {
	RegisterRavenwoodBuildComponents(android.InitRegistrationContext)
}

func RegisterRavenwoodBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("android_ravenwood_test", ravenwoodTestFactory)
	ctx.RegisterModuleType("android_ravenwood_libgroup", ravenwoodLibgroupFactory)
}

var ravenwoodTag = dependencyTag{name: "ravenwood"}

const ravenwoodUtilsName = "ravenwood-utils"
const ravenwoodRuntimeName = "ravenwood-runtime"

type ravenwoodTest struct {
	Library

	testProperties testProperties
	testConfig     android.Path

	forceOSType   android.OsType
	forceArchType android.ArchType
}

func ravenwoodTestFactory() android.Module {
	module := &ravenwoodTest{}

	module.addHostAndDeviceProperties()
	module.AddProperties(&module.testProperties)

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
	ctx.AddVariationDependencies(nil, ravenwoodTag, ravenwoodRuntimeName)

	// Directly depend on any utils so that we link against them
	utils := ctx.AddVariationDependencies(nil, ravenwoodTag, ravenwoodUtilsName)[0]
	if utils != nil {
		for _, lib := range utils.(*ravenwoodLibgroup).ravenwoodLibgroupProperties.Libs {
			ctx.AddVariationDependencies(nil, libTag, lib)
		}
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

	// Start by depending on all files installed by dependancies
	var installDeps android.InstallPaths
	for _, dep := range ctx.GetDirectDepsWithTag(ravenwoodTag) {
		for _, installFile := range dep.FilesToInstall() {
			installDeps = append(installDeps, installFile)
		}
	}

	// Also depend on our config
	installPath := android.PathForModuleInstall(ctx, r.BaseModuleName())
	installConfig := ctx.InstallFile(installPath, ctx.ModuleName()+".config", r.testConfig)
	installDeps = append(installDeps, installConfig)

	// Finally install our JAR with all dependencies
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
		ctx.AddVariationDependencies(nil, ravenwoodTag, lib)
	}
}

func (r *ravenwoodLibgroup) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	r.forceOSType = ctx.Config().BuildOS
	r.forceArchType = ctx.Config().BuildArch

	// Install our runtime into expected location for packaging
	installPath := android.PathForModuleInstall(ctx, r.BaseModuleName())
	for _, lib := range r.ravenwoodLibgroupProperties.Libs {
		libModule := ctx.GetDirectDepWithTag(lib, ravenwoodTag)
		libJar := android.OutputFileForModule(ctx, libModule, "")
		ctx.InstallFile(installPath, lib+".jar", libJar)
	}

	// Normal build should perform install steps
	ctx.Phony(r.BaseModuleName(), android.PathForPhony(ctx, r.BaseModuleName()+"-install"))
}
