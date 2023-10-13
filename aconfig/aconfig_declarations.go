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
	"fmt"
	"strings"

	"android/soong/android"
	"android/soong/bazel"
	"github.com/google/blueprint"
)

type DeclarationsModule struct {
	android.ModuleBase
	android.DefaultableModuleBase
	android.BazelModuleBase

	// Properties for "aconfig_declarations"
	properties struct {
		// aconfig files, relative to this Android.bp file
		Srcs []string `android:"path"`

		// Release config flag package
		Package string

		// Values from TARGET_RELEASE / RELEASE_ACONFIG_VALUE_SETS
		Values []string `blueprint:"mutated"`
	}

	intermediatePath android.WritablePath
}

func DeclarationsFactory() android.Module {
	module := &DeclarationsModule{}

	android.InitAndroidModule(module)
	android.InitDefaultableModule(module)
	module.AddProperties(&module.properties)
	android.InitBazelModule(module)

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

	// Add a dependency on the aconfig_value_sets defined in
	// RELEASE_ACONFIG_VALUE_SETS, and add any aconfig_values that
	// match our package.
	valuesFromConfig := ctx.Config().ReleaseAconfigValueSets()
	if len(valuesFromConfig) > 0 {
		ctx.AddDependency(ctx.Module(), implicitValuesTag, valuesFromConfig...)
	}
}

func (module *DeclarationsModule) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		// The default output of this module is the intermediates format, which is
		// not installable and in a private format that no other rules can handle
		// correctly.
		return []android.Path{module.intermediatePath}, nil
	default:
		return nil, fmt.Errorf("unsupported aconfig_declarations module reference tag %q", tag)
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

// Provider published by aconfig_value_set
type declarationsProviderData struct {
	Package          string
	IntermediatePath android.WritablePath
}

var declarationsProviderKey = blueprint.NewProvider(declarationsProviderData{})

func (module *DeclarationsModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Get the values that came from the global RELEASE_ACONFIG_VALUE_SETS flag
	valuesFiles := make([]android.Path, 0)
	ctx.VisitDirectDeps(func(dep android.Module) {
		if !ctx.OtherModuleHasProvider(dep, valueSetProviderKey) {
			// Other modules get injected as dependencies too, for example the license modules
			return
		}
		depData := ctx.OtherModuleProvider(dep, valueSetProviderKey).(valueSetProviderData)
		paths, ok := depData.AvailablePackages[module.properties.Package]
		if ok {
			valuesFiles = append(valuesFiles, paths...)
			for _, path := range paths {
				module.properties.Values = append(module.properties.Values, path.String())
			}
		}
	})

	// Intermediate format
	declarationFiles := android.PathsForModuleSrc(ctx, module.properties.Srcs)
	intermediatePath := android.PathForModuleOut(ctx, "intermediate.pb")
	defaultPermission := ctx.Config().ReleaseAconfigFlagDefaultPermission()
	inputFiles := make([]android.Path, len(declarationFiles))
	copy(inputFiles, declarationFiles)
	inputFiles = append(inputFiles, valuesFiles...)
	ctx.Build(pctx, android.BuildParams{
		Rule:        aconfigRule,
		Output:      intermediatePath,
		Inputs:      inputFiles,
		Description: "aconfig_declarations",
		Args: map[string]string{
			"release_version":    ctx.Config().ReleaseVersion(),
			"package":            module.properties.Package,
			"declarations":       android.JoinPathsWithPrefix(declarationFiles, "--declarations "),
			"values":             joinAndPrefix(" --values ", module.properties.Values),
			"default-permission": optionalVariable(" --default-permission ", defaultPermission),
		},
	})

	ctx.SetProvider(declarationsProviderKey, declarationsProviderData{
		Package:          module.properties.Package,
		IntermediatePath: intermediatePath,
	})

}

type bazelAconfigDeclarationsAttributes struct {
	Srcs    bazel.LabelListAttribute
	Package string
}

func (module *DeclarationsModule) ConvertWithBp2build(ctx android.Bp2buildMutatorContext) {
	if ctx.ModuleType() != "aconfig_declarations" {
		return
	}
	srcs := bazel.MakeLabelListAttribute(android.BazelLabelForModuleSrc(ctx, module.properties.Srcs))

	attrs := bazelAconfigDeclarationsAttributes{
		Srcs:    srcs,
		Package: module.properties.Package,
	}
	props := bazel.BazelTargetModuleProperties{
		Rule_class:        "aconfig_declarations",
		Bzl_load_location: "//build/bazel/rules/aconfig:aconfig_declarations.bzl",
	}

	ctx.CreateBazelTargetModule(props, android.CommonAttributes{Name: module.Name()}, &attrs)
}
