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
	"github.com/google/blueprint"
)

// Properties for "device_config_value_set"
type ValueSetModule struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties struct {
		// device_config_values modules
		Values []string
	}
}

func ValueSetFactory() android.Module {
	module := &ValueSetModule{}

	android.InitAndroidModule(module)
	android.InitDefaultableModule(module)
	module.AddProperties(&module.properties)
	// TODO: bp2build
	//android.InitBazelModule(module)

	return module
}

// Dependency tag for values property
type valueSetType struct {
	blueprint.BaseDependencyTag
}

var valueSetTag = valueSetType{}

// Provider published by device_config_value_set
type valueSetProviderData struct {
	// The namespace of each of the
	// (map of namespace --> device_config_module)
	AvailableNamespaces map[string]android.Paths
}

var valueSetProviderKey = blueprint.NewProvider(valueSetProviderData{})

func (module *ValueSetModule) DepsMutator(ctx android.BottomUpMutatorContext) {
	deps := ctx.AddDependency(ctx.Module(), valueSetTag, module.properties.Values...)
	for _, dep := range deps {
		_, ok := dep.(*ValuesModule)
		if !ok {
			ctx.PropertyErrorf("values", "values must be a device_config_values module")
			return
		}
	}
}

func (module *ValueSetModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Accumulate the namespaces of the values modules listed, and set that as an
	// valueSetProviderKey provider that device_config modules can read and use
	// to append values to their aconfig actions.
	namespaces := make(map[string]android.Paths)
	ctx.VisitDirectDeps(func(dep android.Module) {
		if !ctx.OtherModuleHasProvider(dep, valuesProviderKey) {
			// Other modules get injected as dependencies too, for example the license modules
			return
		}
		depData := ctx.OtherModuleProvider(dep, valuesProviderKey).(valuesProviderData)

		srcs := make([]android.Path, len(depData.Values))
		copy(srcs, depData.Values)
		namespaces[depData.Namespace] = srcs

	})
	ctx.SetProvider(valueSetProviderKey, valueSetProviderData{
		AvailableNamespaces: namespaces,
	})
}
