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

package python

import (
	"fmt"

	"android/soong/testing"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/tradefed"
)

// This file contains the module types for building Python test.

func init() {
	registerPythonTestComponents(android.InitRegistrationContext)
}

func registerPythonTestComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("python_test_host", PythonTestHostFactory)
	ctx.RegisterModuleType("python_test", PythonTestFactory)
}

func NewTest(hod android.HostOrDeviceSupported) *PythonTestModule {
	return &PythonTestModule{PythonBinaryModule: *NewBinary(hod)}
}

func PythonTestHostFactory() android.Module {
	return NewTest(android.HostSupported).init()
}

func PythonTestFactory() android.Module {
	module := NewTest(android.HostAndDeviceSupported)
	module.multilib = android.MultilibBoth
	return module.init()
}

type TestProperties struct {
	// the name of the test configuration (for example "AndroidTest.xml") that should be
	// installed with the module.
	Test_config *string `android:"path,arch_variant"`

	// the name of the test configuration template (for example "AndroidTestTemplate.xml") that
	// should be installed with the module.
	Test_config_template *string `android:"path,arch_variant"`

	// list of files or filegroup modules that provide data that should be installed alongside
	// the test
	Data []string `android:"path,arch_variant"`

	// list of java modules that provide data that should be installed alongside the test.
	Java_data []string

	// Test options.
	Test_options TestOptions

	// list of device binary modules that should be installed alongside the test
	// This property adds 64bit AND 32bit variants of the dependency
	Data_device_bins_both []string `android:"arch_variant"`
}

type TestOptions struct {
	android.CommonTestOptions

	// Runner for the test. Supports "tradefed" and "mobly" (for multi-device tests). Default is "tradefed".
	Runner *string

	// Metadata to describe the test configuration.
	Metadata []Metadata
}

type Metadata struct {
	Name  string
	Value string
}

type PythonTestModule struct {
	PythonBinaryModule

	testProperties TestProperties
	testConfig     android.Path
	data           []android.DataPath
}

func (p *PythonTestModule) init() android.Module {
	p.AddProperties(&p.properties, &p.protoProperties)
	p.AddProperties(&p.binaryProperties)
	p.AddProperties(&p.testProperties)
	android.InitAndroidArchModule(p, p.hod, p.multilib)
	android.InitDefaultableModule(p)
	if p.isTestHost() && p.testProperties.Test_options.Unit_test == nil {
		p.testProperties.Test_options.Unit_test = proptools.BoolPtr(true)
	}
	return p
}

func (p *PythonTestModule) isTestHost() bool {
	return p.hod == android.HostSupported
}

var dataDeviceBinsTag = dependencyTag{name: "dataDeviceBins"}

// python_test_host DepsMutator uses this method to add multilib dependencies of
// data_device_bin_both
func (p *PythonTestModule) addDataDeviceBinsDeps(ctx android.BottomUpMutatorContext, filter string) {
	if len(p.testProperties.Data_device_bins_both) < 1 {
		return
	}

	var maybeAndroidTarget *android.Target
	androidTargetList := android.FirstTarget(ctx.Config().Targets[android.Android], filter)
	if len(androidTargetList) > 0 {
		maybeAndroidTarget = &androidTargetList[0]
	}

	if maybeAndroidTarget != nil {
		ctx.AddFarVariationDependencies(
			maybeAndroidTarget.Variations(),
			dataDeviceBinsTag,
			p.testProperties.Data_device_bins_both...,
		)
	}
}

func (p *PythonTestModule) DepsMutator(ctx android.BottomUpMutatorContext) {
	p.PythonBinaryModule.DepsMutator(ctx)
	if p.isTestHost() {
		p.addDataDeviceBinsDeps(ctx, "lib32")
		p.addDataDeviceBinsDeps(ctx, "lib64")
	}
}

func (p *PythonTestModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// We inherit from only the library's GenerateAndroidBuildActions, and then
	// just use buildBinary() so that the binary is not installed into the location
	// it would be for regular binaries.
	p.PythonLibraryModule.GenerateAndroidBuildActions(ctx)
	p.buildBinary(ctx)

	var configs []tradefed.Option
	for _, metadata := range p.testProperties.Test_options.Metadata {
		configs = append(configs, tradefed.Option{Name: "config-descriptor:metadata", Key: metadata.Name, Value: metadata.Value})
	}

	runner := proptools.StringDefault(p.testProperties.Test_options.Runner, "tradefed")
	if runner == "tradefed" {
		p.testConfig = tradefed.AutoGenTestConfig(ctx, tradefed.AutoGenTestConfigOptions{
			TestConfigProp:          p.testProperties.Test_config,
			TestConfigTemplateProp:  p.testProperties.Test_config_template,
			TestSuites:              p.binaryProperties.Test_suites,
			OptionsForAutogenerated: configs,
			AutoGenConfig:           p.binaryProperties.Auto_gen_config,
			DeviceTemplate:          "${PythonBinaryHostTestConfigTemplate}",
			HostTemplate:            "${PythonBinaryHostTestConfigTemplate}",
		})
	} else if runner == "mobly" {
		if p.testProperties.Test_config != nil || p.testProperties.Test_config_template != nil || p.binaryProperties.Auto_gen_config != nil {
			panic(fmt.Errorf("cannot set test_config, test_config_template or auto_gen_config for mobly test"))
		}

		for _, testSuite := range p.binaryProperties.Test_suites {
			if testSuite == "cts" {
				configs = append(configs, tradefed.Option{Name: "test-suite-tag", Value: "cts"})
				break
			}
		}
		p.testConfig = tradefed.AutoGenTestConfig(ctx, tradefed.AutoGenTestConfigOptions{
			OptionsForAutogenerated: configs,
			DeviceTemplate:          "${PythonBinaryHostMoblyTestConfigTemplate}",
			HostTemplate:            "${PythonBinaryHostMoblyTestConfigTemplate}",
		})
	} else {
		panic(fmt.Errorf("unknown python test runner '%s', should be 'tradefed' or 'mobly'", runner))
	}

	for _, dataSrcPath := range android.PathsForModuleSrc(ctx, p.testProperties.Data) {
		p.data = append(p.data, android.DataPath{SrcPath: dataSrcPath})
	}

	if p.isTestHost() && len(p.testProperties.Data_device_bins_both) > 0 {
		ctx.VisitDirectDepsWithTag(dataDeviceBinsTag, func(dep android.Module) {
			p.data = append(p.data, android.DataPath{SrcPath: android.OutputFileForModule(ctx, dep, "")})
		})
	}

	// Emulate the data property for java_data dependencies.
	for _, javaData := range ctx.GetDirectDepsWithTag(javaDataTag) {
		for _, javaDataSrcPath := range android.OutputFilesForModule(ctx, javaData, "") {
			p.data = append(p.data, android.DataPath{SrcPath: javaDataSrcPath})
		}
	}

	installDir := installDir(ctx, "nativetest", "nativetest64", ctx.ModuleName())
	installedData := ctx.InstallTestData(installDir, p.data)
	p.installedDest = ctx.InstallFile(installDir, p.installSource.Base(), p.installSource, installedData...)

	ctx.SetProvider(testing.TestModuleProviderKey, testing.TestModuleProviderData{})
}

func (p *PythonTestModule) AndroidMkEntries() []android.AndroidMkEntries {
	entriesList := p.PythonBinaryModule.AndroidMkEntries()
	if len(entriesList) != 1 {
		panic("Expected 1 entry")
	}
	entries := &entriesList[0]

	entries.Class = "NATIVE_TESTS"

	entries.ExtraEntries = append(entries.ExtraEntries,
		func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
			//entries.AddCompatibilityTestSuites(p.binaryProperties.Test_suites...)
			if p.testConfig != nil {
				entries.SetString("LOCAL_FULL_TEST_CONFIG", p.testConfig.String())
			}

			entries.SetBoolIfTrue("LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG", !BoolDefault(p.binaryProperties.Auto_gen_config, true))

			p.testProperties.Test_options.SetAndroidMkEntries(entries)
		})

	return entriesList
}
