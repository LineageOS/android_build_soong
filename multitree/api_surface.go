// Copyright 2021 Google Inc. All rights reserved.
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

package multitree

import (
	"android/soong/android"
	"fmt"

	"github.com/google/blueprint"
)

var (
	pctx = android.NewPackageContext("android/soong/multitree")
)

func init() {
	RegisterApiSurfaceBuildComponents(android.InitRegistrationContext)
}

var PrepareForTestWithApiSurface = android.FixtureRegisterWithContext(RegisterApiSurfaceBuildComponents)

func RegisterApiSurfaceBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("api_surface", ApiSurfaceFactory)
}

type ApiSurface struct {
	android.ModuleBase
	ExportableModuleBase
	properties apiSurfaceProperties

	allOutputs    android.Paths
	taggedOutputs map[string]android.Paths
}

type apiSurfaceProperties struct {
	Contributions []string
}

func ApiSurfaceFactory() android.Module {
	module := &ApiSurface{}
	module.AddProperties(&module.properties)
	android.InitAndroidModule(module)
	InitExportableModule(module)
	return module
}

func (surface *ApiSurface) DepsMutator(ctx android.BottomUpMutatorContext) {
	if surface.properties.Contributions != nil {
		ctx.AddVariationDependencies(nil, nil, surface.properties.Contributions...)
	}

}
func (surface *ApiSurface) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	contributionFiles := make(map[string]android.Paths)
	var allOutputs android.Paths
	ctx.WalkDeps(func(child, parent android.Module) bool {
		if contribution, ok := child.(ApiContribution); ok {
			copied := contribution.CopyFilesWithTag(ctx)
			for tag, files := range copied {
				contributionFiles[child.Name()+"#"+tag] = files
			}
			for _, paths := range copied {
				allOutputs = append(allOutputs, paths...)
			}
			return false // no transitive dependencies
		}
		return false
	})

	// phony target
	ctx.Build(pctx, android.BuildParams{
		Rule:   blueprint.Phony,
		Output: android.PathForPhony(ctx, ctx.ModuleName()),
		Inputs: allOutputs,
	})

	surface.allOutputs = allOutputs
	surface.taggedOutputs = contributionFiles
}

func (surface *ApiSurface) OutputFiles(tag string) (android.Paths, error) {
	if tag != "" {
		return nil, fmt.Errorf("unknown tag: %q", tag)
	}
	return surface.allOutputs, nil
}

func (surface *ApiSurface) TaggedOutputs() map[string]android.Paths {
	return surface.taggedOutputs
}

func (surface *ApiSurface) Exportable() bool {
	return true
}

var _ android.OutputFileProducer = (*ApiSurface)(nil)
var _ Exportable = (*ApiSurface)(nil)

type ApiContribution interface {
	// copy files necessaryt to construct an API surface
	// For C, it will be map.txt and .h files
	// For Java, it will be api.txt
	CopyFilesWithTag(ctx android.ModuleContext) map[string]android.Paths // output paths

	// Generate Android.bp in out/ to use the exported .txt files
	// GenerateBuildFiles(ctx ModuleContext) Paths //output paths
}
