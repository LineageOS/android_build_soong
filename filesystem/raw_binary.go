// Copyright (C) 2022 The Android Open Source Project
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

package filesystem

import (
	"fmt"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

var (
	toRawBinary = pctx.AndroidStaticRule("toRawBinary",
		blueprint.RuleParams{
			Command:     "${objcopy} --output-target=binary ${in} ${out}",
			CommandDeps: []string{"$objcopy"},
		},
		"objcopy")
)

func init() {
	pctx.Import("android/soong/cc/config")

	android.RegisterModuleType("raw_binary", rawBinaryFactory)
}

type rawBinary struct {
	android.ModuleBase

	properties rawBinaryProperties

	output     android.OutputPath
	installDir android.InstallPath
}

type rawBinaryProperties struct {
	// Set the name of the output. Defaults to <module_name>.bin.
	Stem *string

	// Name of input executable. Can be a name of a target.
	Src *string `android:"path,arch_variant"`
}

func rawBinaryFactory() android.Module {
	module := &rawBinary{}
	module.AddProperties(&module.properties)
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	return module
}

func (r *rawBinary) DepsMutator(ctx android.BottomUpMutatorContext) {
	// do nothing
}

func (r *rawBinary) installFileName() string {
	return proptools.StringDefault(r.properties.Stem, r.BaseModuleName()+".bin")
}

func (r *rawBinary) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	inputFile := android.PathForModuleSrc(ctx, proptools.String(r.properties.Src))
	outputFile := android.PathForModuleOut(ctx, r.installFileName()).OutputPath

	ctx.Build(pctx, android.BuildParams{
		Rule:        toRawBinary,
		Description: "prefix symbols " + outputFile.Base(),
		Output:      outputFile,
		Input:       inputFile,
		Args: map[string]string{
			"objcopy": "${config.ClangBin}/llvm-objcopy",
		},
	})

	r.output = outputFile
	r.installDir = android.PathForModuleInstall(ctx, "etc")
	ctx.InstallFile(r.installDir, r.installFileName(), r.output)
}

var _ android.AndroidMkEntriesProvider = (*rawBinary)(nil)

// Implements android.AndroidMkEntriesProvider
func (r *rawBinary) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(r.output),
	}}
}

var _ Filesystem = (*rawBinary)(nil)

func (r *rawBinary) OutputPath() android.Path {
	return r.output
}

func (r *rawBinary) SignedOutputPath() android.Path {
	return nil
}

var _ android.OutputFileProducer = (*rawBinary)(nil)

// Implements android.OutputFileProducer
func (r *rawBinary) OutputFiles(tag string) (android.Paths, error) {
	if tag == "" {
		return []android.Path{r.output}, nil
	}
	return nil, fmt.Errorf("unsupported module reference tag %q", tag)
}
