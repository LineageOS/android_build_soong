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
	"io"
)

// sh_binary is for shell scripts (and batch files) that are installed as
// executable files into .../bin/
//
// Do not use them for prebuilt C/C++/etc files.  Use cc_prebuilt_binary
// instead.

func init() {
	RegisterModuleType("sh_binary", ShBinaryFactory)
	RegisterModuleType("sh_binary_host", ShBinaryHostFactory)
}

type shBinaryProperties struct {
	// Source file of this prebuilt.
	Src *string `android:"arch_variant"`

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

type ShBinary struct {
	ModuleBase

	properties shBinaryProperties

	sourceFilePath Path
	outputFilePath OutputPath
}

func (s *ShBinary) DepsMutator(ctx BottomUpMutatorContext) {
	if s.properties.Src == nil {
		ctx.PropertyErrorf("src", "missing prebuilt source file")
	}

	// To support ":modulename" in src
	ExtractSourceDeps(ctx, s.properties.Src)
}

func (s *ShBinary) SourceFilePath(ctx ModuleContext) Path {
	return ctx.ExpandSource(String(s.properties.Src), "src")
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
	s.sourceFilePath = ctx.ExpandSource(String(s.properties.Src), "src")
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

func (s *ShBinary) AndroidMk() AndroidMkData {
	return AndroidMkData{
		Class:      "EXECUTABLES",
		OutputFile: OptionalPathForPath(s.outputFilePath),
		Include:    "$(BUILD_SYSTEM)/soong_cc_prebuilt.mk",
		Extra: []AndroidMkExtraFunc{
			func(w io.Writer, outputFile Path) {
				fmt.Fprintln(w, "LOCAL_MODULE_RELATIVE_PATH :=", String(s.properties.Sub_dir))
				fmt.Fprintln(w, "LOCAL_MODULE_SUFFIX :=")
				fmt.Fprintln(w, "LOCAL_MODULE_STEM :=", s.outputFilePath.Rel())
			},
		},
	}
}

func InitShBinaryModule(s *ShBinary) {
	s.AddProperties(&s.properties)
}

func ShBinaryFactory() Module {
	module := &ShBinary{}
	InitShBinaryModule(module)
	InitAndroidArchModule(module, HostAndDeviceSupported, MultilibFirst)
	return module
}

func ShBinaryHostFactory() Module {
	module := &ShBinary{}
	InitShBinaryModule(module)
	InitAndroidArchModule(module, HostSupported, MultilibFirst)
	return module
}
