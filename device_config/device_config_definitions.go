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

package device_config

import (
	"android/soong/android"
	"fmt"
	"github.com/google/blueprint"
	"strings"
)

type DefinitionsModule struct {
	android.ModuleBase
	android.DefaultableModuleBase

	// Properties for "device_config_definitions"
	properties struct {
		// aconfig files, relative to this Android.bp file
		Srcs []string `android:"path"`

		// Release config flag namespace
		Namespace string

		// Values from TARGET_RELEASE / RELEASE_DEVICE_CONFIG_VALUE_SETS
		Values []string `blueprint:"mutated"`
	}

	intermediatePath android.WritablePath
}

func DefinitionsFactory() android.Module {
	module := &DefinitionsModule{}

	android.InitAndroidModule(module)
	android.InitDefaultableModule(module)
	module.AddProperties(&module.properties)
	// TODO: bp2build
	//android.InitBazelModule(module)

	return module
}

type implicitValuesTagType struct {
	blueprint.BaseDependencyTag
}

var implicitValuesTag = implicitValuesTagType{}

func (module *DefinitionsModule) DepsMutator(ctx android.BottomUpMutatorContext) {
	// Validate Properties
	if len(module.properties.Srcs) == 0 {
		ctx.PropertyErrorf("srcs", "missing source files")
		return
	}
	if len(module.properties.Namespace) == 0 {
		ctx.PropertyErrorf("namespace", "missing namespace property")
	}

	// Add a dependency on the device_config_value_sets defined in
	// RELEASE_DEVICE_CONFIG_VALUE_SETS, and add any device_config_values that
	// match our namespace.
	valuesFromConfig := ctx.Config().ReleaseDeviceConfigValueSets()
	ctx.AddDependency(ctx.Module(), implicitValuesTag, valuesFromConfig...)
}

func (module *DefinitionsModule) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		// The default output of this module is the intermediates format, which is
		// not installable and in a private format that no other rules can handle
		// correctly.
		return []android.Path{module.intermediatePath}, nil
	default:
		return nil, fmt.Errorf("unsupported device_config_definitions module reference tag %q", tag)
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

// Provider published by device_config_value_set
type definitionsProviderData struct {
	namespace        string
	intermediatePath android.WritablePath
}

var definitionsProviderKey = blueprint.NewProvider(definitionsProviderData{})

func (module *DefinitionsModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Get the values that came from the global RELEASE_DEVICE_CONFIG_VALUE_SETS flag
	ctx.VisitDirectDeps(func(dep android.Module) {
		if !ctx.OtherModuleHasProvider(dep, valueSetProviderKey) {
			// Other modules get injected as dependencies too, for example the license modules
			return
		}
		depData := ctx.OtherModuleProvider(dep, valueSetProviderKey).(valueSetProviderData)
		valuesFiles, ok := depData.AvailableNamespaces[module.properties.Namespace]
		if ok {
			for _, path := range valuesFiles {
				module.properties.Values = append(module.properties.Values, path.String())
			}
		}
	})

	// Intermediate format
	inputFiles := android.PathsForModuleSrc(ctx, module.properties.Srcs)
	intermediatePath := android.PathForModuleOut(ctx, "intermediate.json")
	ctx.Build(pctx, android.BuildParams{
		Rule:        aconfigRule,
		Inputs:      inputFiles,
		Output:      intermediatePath,
		Description: "device_config_definitions",
		Args: map[string]string{
			"release_version": ctx.Config().ReleaseVersion(),
			"namespace":       module.properties.Namespace,
			"values":          joinAndPrefix(" --values ", module.properties.Values),
		},
	})

	ctx.SetProvider(definitionsProviderKey, definitionsProviderData{
		namespace:        module.properties.Namespace,
		intermediatePath: intermediatePath,
	})

}
