// Copyright 2019 Google Inc. All rights reserved.
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

package sh

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/tradefed"
)

// sh_binary is for shell scripts (and batch files) that are installed as
// executable files into .../bin/
//
// Do not use them for prebuilt C/C++/etc files.  Use cc_prebuilt_binary
// instead.

var pctx = android.NewPackageContext("android/soong/sh")

func init() {
	pctx.Import("android/soong/android")

	android.RegisterModuleType("sh_binary", ShBinaryFactory)
	android.RegisterModuleType("sh_binary_host", ShBinaryHostFactory)
	android.RegisterModuleType("sh_test", ShTestFactory)
	android.RegisterModuleType("sh_test_host", ShTestHostFactory)
}

type shBinaryProperties struct {
	// Source file of this prebuilt.
	Src *string `android:"path,arch_variant"`

	// optional subdirectory under which this file is installed into
	Sub_dir *string `android:"arch_variant"`

	// optional name for the installed file. If unspecified, name of the module is used as the file name
	Filename *string `android:"arch_variant"`

	// when set to true, and filename property is not set, the name for the installed file
	// is the same as the file name of the source file.
	Filename_from_src *bool `android:"arch_variant"`

	// Whether this module is directly installable to one of the partitions. Default: true.
	Installable *bool

	// install symlinks to the binary
	Symlinks []string `android:"arch_variant"`
}

type TestProperties struct {
	// list of compatibility suites (for example "cts", "vts") that the module should be
	// installed into.
	Test_suites []string `android:"arch_variant"`

	// the name of the test configuration (for example "AndroidTest.xml") that should be
	// installed with the module.
	Test_config *string `android:"path,arch_variant"`

	// list of files or filegroup modules that provide data that should be installed alongside
	// the test.
	Data []string `android:"path,arch_variant"`

	// Add RootTargetPreparer to auto generated test config. This guarantees the test to run
	// with root permission.
	Require_root *bool

	// the name of the test configuration template (for example "AndroidTestTemplate.xml") that
	// should be installed with the module.
	Test_config_template *string `android:"path,arch_variant"`

	// Flag to indicate whether or not to create test config automatically. If AndroidTest.xml
	// doesn't exist next to the Android.bp, this attribute doesn't need to be set to true
	// explicitly.
	Auto_gen_config *bool

	// list of binary modules that should be installed alongside the test
	Data_bins []string `android:"path,arch_variant"`

	// list of library modules that should be installed alongside the test
	Data_libs []string `android:"path,arch_variant"`

	// list of device binary modules that should be installed alongside the test.
	// Only available for host sh_test modules.
	Data_device_bins []string `android:"path,arch_variant"`

	// list of device library modules that should be installed alongside the test.
	// Only available for host sh_test modules.
	Data_device_libs []string `android:"path,arch_variant"`
}

type ShBinary struct {
	android.ModuleBase

	properties shBinaryProperties

	sourceFilePath android.Path
	outputFilePath android.OutputPath
	installedFile  android.InstallPath
}

var _ android.HostToolProvider = (*ShBinary)(nil)

type ShTest struct {
	ShBinary

	testProperties TestProperties

	data       android.Paths
	testConfig android.Path

	dataModules map[string]android.Path
}

func (s *ShBinary) HostToolPath() android.OptionalPath {
	return android.OptionalPathForPath(s.installedFile)
}

func (s *ShBinary) DepsMutator(ctx android.BottomUpMutatorContext) {
	if s.properties.Src == nil {
		ctx.PropertyErrorf("src", "missing prebuilt source file")
	}
}

func (s *ShBinary) OutputFile() android.OutputPath {
	return s.outputFilePath
}

func (s *ShBinary) SubDir() string {
	return proptools.String(s.properties.Sub_dir)
}

func (s *ShBinary) Installable() bool {
	return s.properties.Installable == nil || proptools.Bool(s.properties.Installable)
}

func (s *ShBinary) Symlinks() []string {
	return s.properties.Symlinks
}

func (s *ShBinary) generateAndroidBuildActions(ctx android.ModuleContext) {
	s.sourceFilePath = android.PathForModuleSrc(ctx, proptools.String(s.properties.Src))
	filename := proptools.String(s.properties.Filename)
	filename_from_src := proptools.Bool(s.properties.Filename_from_src)
	if filename == "" {
		if filename_from_src {
			filename = s.sourceFilePath.Base()
		} else {
			filename = ctx.ModuleName()
		}
	} else if filename_from_src {
		ctx.PropertyErrorf("filename_from_src", "filename is set. filename_from_src can't be true")
		return
	}
	s.outputFilePath = android.PathForModuleOut(ctx, filename).OutputPath

	// This ensures that outputFilePath has the correct name for others to
	// use, as the source file may have a different name.
	ctx.Build(pctx, android.BuildParams{
		Rule:   android.CpExecutable,
		Output: s.outputFilePath,
		Input:  s.sourceFilePath,
	})
}

func (s *ShBinary) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	s.generateAndroidBuildActions(ctx)
	installDir := android.PathForModuleInstall(ctx, "bin", proptools.String(s.properties.Sub_dir))
	s.installedFile = ctx.InstallExecutable(installDir, s.outputFilePath.Base(), s.outputFilePath)
}

func (s *ShBinary) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "EXECUTABLES",
		OutputFile: android.OptionalPathForPath(s.outputFilePath),
		Include:    "$(BUILD_SYSTEM)/soong_cc_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				s.customAndroidMkEntries(entries)
			},
		},
	}}
}

func (s *ShBinary) customAndroidMkEntries(entries *android.AndroidMkEntries) {
	entries.SetString("LOCAL_MODULE_RELATIVE_PATH", proptools.String(s.properties.Sub_dir))
	entries.SetString("LOCAL_MODULE_SUFFIX", "")
	entries.SetString("LOCAL_MODULE_STEM", s.outputFilePath.Rel())
	if len(s.properties.Symlinks) > 0 {
		entries.SetString("LOCAL_MODULE_SYMLINKS", strings.Join(s.properties.Symlinks, " "))
	}
}

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

var (
	shTestDataBinsTag       = dependencyTag{name: "dataBins"}
	shTestDataLibsTag       = dependencyTag{name: "dataLibs"}
	shTestDataDeviceBinsTag = dependencyTag{name: "dataDeviceBins"}
	shTestDataDeviceLibsTag = dependencyTag{name: "dataDeviceLibs"}
)

var sharedLibVariations = []blueprint.Variation{{Mutator: "link", Variation: "shared"}}

func (s *ShTest) DepsMutator(ctx android.BottomUpMutatorContext) {
	s.ShBinary.DepsMutator(ctx)

	ctx.AddFarVariationDependencies(ctx.Target().Variations(), shTestDataBinsTag, s.testProperties.Data_bins...)
	ctx.AddFarVariationDependencies(append(ctx.Target().Variations(), sharedLibVariations...),
		shTestDataLibsTag, s.testProperties.Data_libs...)
	if ctx.Target().Os.Class == android.Host && len(ctx.Config().Targets[android.Android]) > 0 {
		deviceVariations := ctx.Config().AndroidFirstDeviceTarget.Variations()
		ctx.AddFarVariationDependencies(deviceVariations, shTestDataDeviceBinsTag, s.testProperties.Data_device_bins...)
		ctx.AddFarVariationDependencies(append(deviceVariations, sharedLibVariations...),
			shTestDataDeviceLibsTag, s.testProperties.Data_device_libs...)
	} else if ctx.Target().Os.Class != android.Host {
		if len(s.testProperties.Data_device_bins) > 0 {
			ctx.PropertyErrorf("data_device_bins", "only available for host modules")
		}
		if len(s.testProperties.Data_device_libs) > 0 {
			ctx.PropertyErrorf("data_device_libs", "only available for host modules")
		}
	}
}

func (s *ShTest) addToDataModules(ctx android.ModuleContext, relPath string, path android.Path) {
	if _, exists := s.dataModules[relPath]; exists {
		ctx.ModuleErrorf("data modules have a conflicting installation path, %v - %s, %s",
			relPath, s.dataModules[relPath].String(), path.String())
		return
	}
	s.dataModules[relPath] = path
}

func (s *ShTest) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	s.ShBinary.generateAndroidBuildActions(ctx)
	testDir := "nativetest"
	if ctx.Target().Arch.ArchType.Multilib == "lib64" {
		testDir = "nativetest64"
	}
	if ctx.Target().NativeBridge == android.NativeBridgeEnabled {
		testDir = filepath.Join(testDir, ctx.Target().NativeBridgeRelativePath)
	} else if !ctx.Host() && ctx.Config().HasMultilibConflict(ctx.Arch().ArchType) {
		testDir = filepath.Join(testDir, ctx.Arch().ArchType.String())
	}
	installDir := android.PathForModuleInstall(ctx, testDir, proptools.String(s.properties.Sub_dir))
	s.installedFile = ctx.InstallExecutable(installDir, s.outputFilePath.Base(), s.outputFilePath)

	s.data = android.PathsForModuleSrc(ctx, s.testProperties.Data)

	var configs []tradefed.Config
	if Bool(s.testProperties.Require_root) {
		configs = append(configs, tradefed.Object{"target_preparer", "com.android.tradefed.targetprep.RootTargetPreparer", nil})
	} else {
		options := []tradefed.Option{{Name: "force-root", Value: "false"}}
		configs = append(configs, tradefed.Object{"target_preparer", "com.android.tradefed.targetprep.RootTargetPreparer", options})
	}
	s.testConfig = tradefed.AutoGenShellTestConfig(ctx, s.testProperties.Test_config,
		s.testProperties.Test_config_template, s.testProperties.Test_suites, configs, s.testProperties.Auto_gen_config, s.outputFilePath.Base())

	s.dataModules = make(map[string]android.Path)
	ctx.VisitDirectDeps(func(dep android.Module) {
		depTag := ctx.OtherModuleDependencyTag(dep)
		switch depTag {
		case shTestDataBinsTag, shTestDataDeviceBinsTag:
			if cc, isCc := dep.(*cc.Module); isCc {
				s.addToDataModules(ctx, cc.OutputFile().Path().Base(), cc.OutputFile().Path())
				return
			}
			property := "data_bins"
			if depTag == shTestDataDeviceBinsTag {
				property = "data_device_bins"
			}
			ctx.PropertyErrorf(property, "%q of type %q is not supported", dep.Name(), ctx.OtherModuleType(dep))
		case shTestDataLibsTag, shTestDataDeviceLibsTag:
			if cc, isCc := dep.(*cc.Module); isCc {
				// Copy to an intermediate output directory to append "lib[64]" to the path,
				// so that it's compatible with the default rpath values.
				var relPath string
				if cc.Arch().ArchType.Multilib == "lib64" {
					relPath = filepath.Join("lib64", cc.OutputFile().Path().Base())
				} else {
					relPath = filepath.Join("lib", cc.OutputFile().Path().Base())
				}
				if _, exist := s.dataModules[relPath]; exist {
					return
				}
				relocatedLib := android.PathForModuleOut(ctx, "relocated", relPath)
				ctx.Build(pctx, android.BuildParams{
					Rule:   android.Cp,
					Input:  cc.OutputFile().Path(),
					Output: relocatedLib,
				})
				s.addToDataModules(ctx, relPath, relocatedLib)
				return
			}
			property := "data_libs"
			if depTag == shTestDataDeviceBinsTag {
				property = "data_device_libs"
			}
			ctx.PropertyErrorf(property, "%q of type %q is not supported", dep.Name(), ctx.OtherModuleType(dep))
		}
	})
}

func (s *ShTest) InstallInData() bool {
	return true
}

func (s *ShTest) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "NATIVE_TESTS",
		OutputFile: android.OptionalPathForPath(s.outputFilePath),
		Include:    "$(BUILD_SYSTEM)/soong_cc_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				s.customAndroidMkEntries(entries)

				entries.AddStrings("LOCAL_COMPATIBILITY_SUITE", s.testProperties.Test_suites...)
				if s.testConfig != nil {
					entries.SetPath("LOCAL_FULL_TEST_CONFIG", s.testConfig)
				}
				for _, d := range s.data {
					rel := d.Rel()
					path := d.String()
					if !strings.HasSuffix(path, rel) {
						panic(fmt.Errorf("path %q does not end with %q", path, rel))
					}
					path = strings.TrimSuffix(path, rel)
					entries.AddStrings("LOCAL_TEST_DATA", path+":"+rel)
				}
				relPaths := make([]string, 0)
				for relPath, _ := range s.dataModules {
					relPaths = append(relPaths, relPath)
				}
				sort.Strings(relPaths)
				for _, relPath := range relPaths {
					dir := strings.TrimSuffix(s.dataModules[relPath].String(), relPath)
					entries.AddStrings("LOCAL_TEST_DATA", dir+":"+relPath)
				}
			},
		},
	}}
}

func InitShBinaryModule(s *ShBinary) {
	s.AddProperties(&s.properties)
}

// sh_binary is for a shell script or batch file to be installed as an
// executable binary to <partition>/bin.
func ShBinaryFactory() android.Module {
	module := &ShBinary{}
	module.Prefer32(func(ctx android.BaseModuleContext, base *android.ModuleBase, class android.OsClass) bool {
		return class == android.Device && ctx.Config().DevicePrefer32BitExecutables()
	})
	InitShBinaryModule(module)
	android.InitAndroidArchModule(module, android.HostAndDeviceSupported, android.MultilibFirst)
	return module
}

// sh_binary_host is for a shell script to be installed as an executable binary
// to $(HOST_OUT)/bin.
func ShBinaryHostFactory() android.Module {
	module := &ShBinary{}
	InitShBinaryModule(module)
	android.InitAndroidArchModule(module, android.HostSupported, android.MultilibFirst)
	return module
}

// sh_test defines a shell script based test module.
func ShTestFactory() android.Module {
	module := &ShTest{}
	InitShBinaryModule(&module.ShBinary)
	module.AddProperties(&module.testProperties)

	android.InitAndroidArchModule(module, android.HostAndDeviceSupported, android.MultilibFirst)
	return module
}

// sh_test_host defines a shell script based test module that runs on a host.
func ShTestHostFactory() android.Module {
	module := &ShTest{}
	InitShBinaryModule(&module.ShBinary)
	module.AddProperties(&module.testProperties)

	android.InitAndroidArchModule(module, android.HostSupported, android.MultilibFirst)
	return module
}

var Bool = proptools.Bool
