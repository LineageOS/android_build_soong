// Copyright 2020 Google Inc. All rights reserved.
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
	"path/filepath"

	"github.com/google/blueprint/proptools"
)

func init() {
	RegisterModuleType("makefile_goal", MakefileGoalFactory)
}

type makefileGoalProperties struct {
	// Sources.

	// Makefile goal output file path, relative to PRODUCT_OUT.
	Product_out_path *string
}

type makefileGoal struct {
	ModuleBase

	properties makefileGoalProperties

	// Destination. Output file path of this module.
	outputFilePath OutputPath
}

var _ AndroidMkEntriesProvider = (*makefileGoal)(nil)
var _ OutputFileProducer = (*makefileGoal)(nil)

// Input file of this makefile_goal module. Nil if none specified. May use variable names in makefiles.
func (p *makefileGoal) inputPath() *string {
	if p.properties.Product_out_path != nil {
		return proptools.StringPtr(filepath.Join("$(PRODUCT_OUT)", proptools.String(p.properties.Product_out_path)))
	}
	return nil
}

// OutputFileProducer
func (p *makefileGoal) OutputFiles(tag string) (Paths, error) {
	if tag != "" {
		return nil, fmt.Errorf("unsupported tag %q", tag)
	}
	return Paths{p.outputFilePath}, nil
}

// AndroidMkEntriesProvider
func (p *makefileGoal) DepsMutator(ctx BottomUpMutatorContext) {
	if p.inputPath() == nil {
		ctx.PropertyErrorf("product_out_path", "Path relative to PRODUCT_OUT required")
	}
}

func (p *makefileGoal) GenerateAndroidBuildActions(ctx ModuleContext) {
	filename := filepath.Base(proptools.String(p.inputPath()))
	p.outputFilePath = PathForModuleOut(ctx, filename).OutputPath

	ctx.InstallFile(PathForModuleInstall(ctx, "etc"), ctx.ModuleName(), p.outputFilePath)
}

func (p *makefileGoal) AndroidMkEntries() []AndroidMkEntries {
	return []AndroidMkEntries{AndroidMkEntries{
		Class:      "ETC",
		OutputFile: OptionalPathForPath(p.outputFilePath),
		ExtraFooters: []AndroidMkExtraFootersFunc{
			func(w io.Writer, name, prefix, moduleDir string) {
				// Can't use Cp because inputPath() is not a valid Path.
				fmt.Fprintf(w, "$(eval $(call copy-one-file,%s,%s))\n", proptools.String(p.inputPath()), p.outputFilePath)
			},
		},
	}}
}

// Import a Makefile goal to Soong by copying the file built by
// the goal to a path visible to Soong. This rule only works on boot images.
func MakefileGoalFactory() Module {
	module := &makefileGoal{}
	module.AddProperties(&module.properties)
	InitAndroidModule(module)
	return module
}
