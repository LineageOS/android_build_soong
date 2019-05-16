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

package android

import (
	"fmt"
	"strings"
)

// sh_binary is for shell scripts (and batch files) that are installed as
// executable files into .../bin/
//
// Do not use them for prebuilt C/C++/etc files.  Use cc_prebuilt_binary
// instead.

func init() {
	RegisterModuleType("sh_binary", ShBinaryFactory)
	RegisterModuleType("sh_binary_host", ShBinaryHostFactory)
	RegisterModuleType("sh_test", ShTestFactory)
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
}

type TestProperties struct {
	// list of compatibility suites (for example "cts", "vts") that the module should be
	// installed into.
	Test_suites []string `android:"arch_variant"`

	// the name of the test configuration (for example "AndroidTest.xml") that should be
	// installed with the module.
	Test_config *string `android:"arch_variant"`

	// list of files or filegroup modules that provide data that should be installed alongside
	// the test.
	Data []string `android:"path,arch_variant"`
}

type ShBinary struct {
	ModuleBase

	properties shBinaryProperties

	sourceFilePath Path
	outputFilePath OutputPath
}

type ShTest struct {
	ShBinary

	testProperties TestProperties

	data Paths
}

func (s *ShBinary) DepsMutator(ctx BottomUpMutatorContext) {
	if s.properties.Src == nil {
		ctx.PropertyErrorf("src", "missing prebuilt source file")
	}
}

func (s *ShBinary) SourceFilePath(ctx ModuleContext) Path {
	return PathForModuleSrc(ctx, String(s.properties.Src))
}

func (s *ShBinary) OutputFile() OutputPath {
	return s.outputFilePath
}

func (s *ShBinary) SubDir() string {
	return String(s.properties.Sub_dir)
}

func (s *ShBinary) Installable() bool {
	return s.properties.Installable == nil || Bool(s.properties.Installable)
}

func (s *ShBinary) GenerateAndroidBuildActions(ctx ModuleContext) {
	s.sourceFilePath = PathForModuleSrc(ctx, String(s.properties.Src))
	filename := String(s.properties.Filename)
	filename_from_src := Bool(s.properties.Filename_from_src)
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
	s.outputFilePath = PathForModuleOut(ctx, filename).OutputPath

	// This ensures that outputFilePath has the correct name for others to
	// use, as the source file may have a different name.
	ctx.Build(pctx, BuildParams{
		Rule:   CpExecutable,
		Output: s.outputFilePath,
		Input:  s.sourceFilePath,
	})
}

func (s *ShBinary) AndroidMkEntries() AndroidMkEntries {
	return AndroidMkEntries{
		Class:      "EXECUTABLES",
		OutputFile: OptionalPathForPath(s.outputFilePath),
		Include:    "$(BUILD_SYSTEM)/soong_cc_prebuilt.mk",
		AddCustomEntries: func(name, prefix, moduleDir string, entries *AndroidMkEntries) {
			s.customAndroidMkEntries(entries)
		},
	}
}

func (s *ShBinary) customAndroidMkEntries(entries *AndroidMkEntries) {
	entries.SetString("LOCAL_MODULE_RELATIVE_PATH", String(s.properties.Sub_dir))
	entries.SetString("LOCAL_MODULE_SUFFIX", "")
	entries.SetString("LOCAL_MODULE_STEM", s.outputFilePath.Rel())
}

func (s *ShTest) GenerateAndroidBuildActions(ctx ModuleContext) {
	s.ShBinary.GenerateAndroidBuildActions(ctx)

	s.data = PathsForModuleSrc(ctx, s.testProperties.Data)
}

func (s *ShTest) AndroidMkEntries() AndroidMkEntries {
	return AndroidMkEntries{
		Class:      "NATIVE_TESTS",
		OutputFile: OptionalPathForPath(s.outputFilePath),
		Include:    "$(BUILD_SYSTEM)/soong_cc_prebuilt.mk",
		AddCustomEntries: func(name, prefix, moduleDir string, entries *AndroidMkEntries) {
			s.customAndroidMkEntries(entries)

			entries.AddStrings("LOCAL_COMPATIBILITY_SUITE", s.testProperties.Test_suites...)
			entries.SetString("LOCAL_TEST_CONFIG", String(s.testProperties.Test_config))
			for _, d := range s.data {
				rel := d.Rel()
				path := d.String()
				if !strings.HasSuffix(path, rel) {
					panic(fmt.Errorf("path %q does not end with %q", path, rel))
				}
				path = strings.TrimSuffix(path, rel)
				entries.AddStrings("LOCAL_TEST_DATA", path+":"+rel)
			}
		},
	}
}

func InitShBinaryModule(s *ShBinary) {
	s.AddProperties(&s.properties)
}

// sh_binary is for a shell script or batch file to be installed as an
// executable binary to <partition>/bin.
func ShBinaryFactory() Module {
	module := &ShBinary{}
	InitShBinaryModule(module)
	InitAndroidArchModule(module, HostAndDeviceSupported, MultilibFirst)
	return module
}

// sh_binary_host is for a shell script to be installed as an executable binary
// to $(HOST_OUT)/bin.
func ShBinaryHostFactory() Module {
	module := &ShBinary{}
	InitShBinaryModule(module)
	InitAndroidArchModule(module, HostSupported, MultilibFirst)
	return module
}

func ShTestFactory() Module {
	module := &ShTest{}
	InitShBinaryModule(&module.ShBinary)
	module.AddProperties(&module.testProperties)

	InitAndroidArchModule(module, HostAndDeviceSupported, MultilibFirst)
	return module
}
