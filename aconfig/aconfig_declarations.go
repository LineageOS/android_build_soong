// Copyright 2023 Google Inc. All rights reserved.
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

package aconfig

import (
	"strings"

	"android/soong/android"

	"github.com/google/blueprint"
)

type DeclarationsModule struct {
	android.ModuleBase
	android.DefaultableModuleBase

	// Properties for "aconfig_declarations"
	properties struct {
		// aconfig files, relative to this Android.bp file
		Srcs []string `android:"path"`

		// Release config flag package
		Package string

		// Values from TARGET_RELEASE / RELEASE_ACONFIG_VALUE_SETS
		Values []string `blueprint:"mutated"`

		// Container(system/vendor/apex) that this module belongs to
		Container string

		// The flags will only be repackaged if this prop is true.
		Exportable bool
	}
}

func DeclarationsFactory() android.Module {
	module := &DeclarationsModule{}

	android.InitAndroidModule(module)
	android.InitDefaultableModule(module)
	module.AddProperties(&module.properties)

	return module
}

type implicitValuesTagType struct {
	blueprint.BaseDependencyTag
}

var implicitValuesTag = implicitValuesTagType{}

func (module *DeclarationsModule) DepsMutator(ctx android.BottomUpMutatorContext) {
	// Validate Properties
	if len(module.properties.Srcs) == 0 {
		ctx.PropertyErrorf("srcs", "missing source files")
		return
	}
	if len(module.properties.Package) == 0 {
		ctx.PropertyErrorf("package", "missing package property")
	}
	if len(module.properties.Container) == 0 {
		ctx.PropertyErrorf("container", "missing container property")
	}

	// Add a dependency on the aconfig_value_sets defined in
	// RELEASE_ACONFIG_VALUE_SETS, and add any aconfig_values that
	// match our package.
	valuesFromConfig := ctx.Config().ReleaseAconfigValueSets()
	if len(valuesFromConfig) > 0 {
		ctx.AddDependency(ctx.Module(), implicitValuesTag, valuesFromConfig...)
	}
}

func joinAndPrefix(prefix string, values []string) string {
	var sb strings.Builder
	for _, v := range values {
		sb.WriteString(prefix)
		sb.WriteString(v)
	}
	return sb.String()
}

func optionalVariable(prefix string, value string) string {
	var sb strings.Builder
	if value != "" {
		sb.WriteString(prefix)
		sb.WriteString(value)
	}
	return sb.String()
}

func (module *DeclarationsModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Get the values that came from the global RELEASE_ACONFIG_VALUE_SETS flag
	valuesFiles := make([]android.Path, 0)
	ctx.VisitDirectDeps(func(dep android.Module) {
		if depData, ok := android.OtherModuleProvider(ctx, dep, valueSetProviderKey); ok {
			paths, ok := depData.AvailablePackages[module.properties.Package]
			if ok {
				valuesFiles = append(valuesFiles, paths...)
				for _, path := range paths {
					module.properties.Values = append(module.properties.Values, path.String())
				}
			}
		}
	})

	// Intermediate format
	declarationFiles := android.PathsForModuleSrc(ctx, module.properties.Srcs)
	intermediateCacheFilePath := android.PathForModuleOut(ctx, "intermediate.pb")
	defaultPermission := ctx.Config().ReleaseAconfigFlagDefaultPermission()
	inputFiles := make([]android.Path, len(declarationFiles))
	copy(inputFiles, declarationFiles)
	inputFiles = append(inputFiles, valuesFiles...)
	args := map[string]string{
		"release_version":    ctx.Config().ReleaseVersion(),
		"package":            module.properties.Package,
		"declarations":       android.JoinPathsWithPrefix(declarationFiles, "--declarations "),
		"values":             joinAndPrefix(" --values ", module.properties.Values),
		"default-permission": optionalVariable(" --default-permission ", defaultPermission),
	}
	if len(module.properties.Container) > 0 {
		args["container"] = "--container " + module.properties.Container
	}
	ctx.Build(pctx, android.BuildParams{
		Rule:        aconfigRule,
		Output:      intermediateCacheFilePath,
		Inputs:      inputFiles,
		Description: "aconfig_declarations",
		Args:        args,
	})

	intermediateDumpFilePath := android.PathForModuleOut(ctx, "intermediate.txt")
	ctx.Build(pctx, android.BuildParams{
		Rule:        aconfigTextRule,
		Output:      intermediateDumpFilePath,
		Inputs:      android.Paths{intermediateCacheFilePath},
		Description: "aconfig_text",
	})

	android.SetProvider(ctx, android.AconfigDeclarationsProviderKey, android.AconfigDeclarationsProviderData{
		Package:                     module.properties.Package,
		Container:                   module.properties.Container,
		Exportable:                  module.properties.Exportable,
		IntermediateCacheOutputPath: intermediateCacheFilePath,
		IntermediateDumpOutputPath:  intermediateDumpFilePath,
	})
}
